package pdf

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rc4"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"errors"
	"fmt"
)

// crypt.go implementa el manejador de seguridad estándar del PDF (/Filter
// /Standard) para DESCIFRAR: RC4 de 40 y 128 bits (revisiones 2 a 4), AES-128
// (revisión 4, /AESV2) y AES-256 (revisiones 5 y 6, /AESV3, con el Algorithm
// 2.B de ISO 32000-2). Siempre se prueba primero la contraseña vacía: es el
// caso de los PDF "protegidos" que se abren sin pedir nada pero traen
// restricciones del propietario.
//
// Qué NO se descifra (lo dice el estándar): las cadenas del propio /Encrypt,
// los xref streams, los objetos DENTRO de un object stream (viajan en claro
// una vez descifrado el stream contenedor), y los /Metadata cuando
// /EncryptMetadata es false.

// ErrPassword se devuelve cuando el PDF pide una contraseña que no tenemos.
var ErrPassword = errors.New("el PDF pide contraseña para abrirse")

// padding es la cadena de relleno del Algorithm 2 (32 bytes fijos del estándar).
var padding = []byte{
	0x28, 0xBF, 0x4E, 0x5E, 0x4E, 0x75, 0x8A, 0x41, 0x64, 0x00, 0x4E, 0x56, 0xFF, 0xFA, 0x01, 0x08,
	0x2E, 0x2E, 0x00, 0xB6, 0xD0, 0x68, 0x3E, 0x80, 0x2F, 0x0C, 0xA9, 0xFE, 0x64, 0x53, 0x69, 0x7A,
}

// cryptMethod es el algoritmo de un filtro de cifrado.
type cryptMethod uint8

const (
	cryptNone cryptMethod = iota
	cryptRC4
	cryptAESV2 // AES-128 con clave por objeto
	cryptAESV3 // AES-256 con la clave de archivo directa
)

// securityHandler guarda la clave de archivo y los métodos para streams y
// cadenas (pueden diferir en V4/V5).
type securityHandler struct {
	key          []byte
	stmMethod    cryptMethod
	strMethod    cryptMethod
	encryptMeta  bool
	encryptNum   int // el /Encrypt no se descifra a sí mismo
	restrictions bool
}

// setupEncryption arma el manejador probando la contraseña vacía y después las
// dadas. Se llama con el cifrado todavía apagado (el /Encrypt va en claro).
func (d *Document) setupEncryption(passwords []string) error {
	encRef := d.trailer[Name("Encrypt")]
	enc, err := d.resolveDict(encRef)
	if err != nil || enc == nil {
		return fmt.Errorf("el /Encrypt no se puede leer")
	}
	if f := enc.GetName("Filter"); f != "Standard" {
		return fmt.Errorf("cifrado %q no soportado (solo el estándar con contraseña)", f)
	}
	h := &securityHandler{encryptMeta: true}
	if r, ok := encRef.(Ref); ok {
		h.encryptNum = r.Num
	}
	if b, ok := enc[Name("EncryptMetadata")].(Bool); ok {
		h.encryptMeta = bool(b)
	}
	v, _ := intValue(enc[Name("V")])
	r, _ := intValue(enc[Name("R")])
	if err := h.methods(enc, v); err != nil {
		return err
	}
	if p, ok := intValue(enc[Name("P")]); ok {
		// Todos los bits de permiso en 1 = sin restricciones.
		h.restrictions = uint32(int32(p))&0xF3C != 0xF3C
	}

	id := d.firstID()
	candidates := append([]string{""}, passwords...)
	for _, pw := range candidates {
		for _, raw := range passwordBytes(pw, r) {
			var key []byte
			if r >= 5 {
				key = authenticateAES256(enc, r, raw)
			} else {
				key = authenticateLegacy(enc, r, v, id, raw, h.encryptMeta)
			}
			if key == nil {
				continue
			}
			h.key = key
			d.crypt = h
			// Lo que se leyó antes de conocer la clave quedó sin descifrar.
			d.cache = map[int]Object{}
			d.objStm = map[int]*objectStream{}
			return nil
		}
	}
	return ErrPassword
}

// passwordBytes devuelve las codificaciones a probar para una contraseña. En
// R5/R6 el estándar dice UTF-8. En R2–R4 dice PDFDocEncoding (una "ñ" es el
// byte 0xF1, no los dos de UTF-8); se prueba esa y, por tolerancia con los
// generadores que la ignoraron, también UTF-8.
func passwordBytes(pw string, r int) [][]byte {
	utf := []byte(pw)
	if r >= 5 {
		return [][]byte{utf}
	}
	if doc, ok := pdfDocEncode(pw); ok && !bytes.Equal(doc, utf) {
		return [][]byte{doc, utf}
	}
	return [][]byte{utf}
}

// pdfDocSpecial son los caracteres de PDFDocEncoding que NO coinciden con
// Latin-1 (los rangos 0x18–0x1F y 0x80–0xA0).
var pdfDocSpecial = map[rune]byte{
	'˘': 0x18, 'ˇ': 0x19, 'ˆ': 0x1A, '˙': 0x1B, '˝': 0x1C, '˛': 0x1D, '˚': 0x1E, '˜': 0x1F,
	'•': 0x80, '†': 0x81, '‡': 0x82, '…': 0x83, '—': 0x84, '–': 0x85, 'ƒ': 0x86, '⁄': 0x87,
	'‹': 0x88, '›': 0x89, '−': 0x8A, '‰': 0x8B, '„': 0x8C, '“': 0x8D, '”': 0x8E, '‘': 0x8F,
	'’': 0x90, '‚': 0x91, '™': 0x92, 'ﬁ': 0x93, 'ﬂ': 0x94, 'Ł': 0x95, 'Œ': 0x96, 'Š': 0x97,
	'Ÿ': 0x98, 'Ž': 0x99, 'ı': 0x9A, 'ł': 0x9B, 'œ': 0x9C, 'š': 0x9D, 'ž': 0x9E, '€': 0xA0,
}

// pdfDocEncode codifica en PDFDocEncoding; false si algún carácter no existe ahí.
func pdfDocEncode(s string) ([]byte, bool) {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		switch {
		case r < 0x18 || (r >= 0x20 && r < 0x7F):
			out = append(out, byte(r))
		case r >= 0xA1 && r <= 0xFF && r != 0xAD:
			out = append(out, byte(r))
		default:
			b, ok := pdfDocSpecial[r]
			if !ok {
				return nil, false
			}
			out = append(out, b)
		}
	}
	return out, true
}

// methods elige los algoritmos según /V (y los filtros /CF en V4/V5).
func (h *securityHandler) methods(enc Dict, v int) error {
	switch v {
	case 1, 2:
		h.stmMethod, h.strMethod = cryptRC4, cryptRC4
		return nil
	case 4, 5:
		cf, _ := enc[Name("CF")].(Dict)
		pick := func(key Name) cryptMethod {
			name, _ := enc[key].(Name)
			if name == "" || name == "Identity" {
				return cryptNone
			}
			filter, _ := cf[name].(Dict)
			switch filter.GetName("CFM") {
			case "V2":
				return cryptRC4
			case "AESV2":
				return cryptAESV2
			case "AESV3":
				return cryptAESV3
			}
			return cryptNone
		}
		h.stmMethod, h.strMethod = pick("StmF"), pick("StrF")
		return nil
	}
	return fmt.Errorf("versión de cifrado /V %d no soportada", v)
}

func (d *Document) firstID() []byte {
	if ids, ok := d.trailer[Name("ID")].(Array); ok && len(ids) > 0 {
		if s, ok := ids[0].(String); ok {
			return s.Value
		}
	}
	return nil
}

func encBytes(enc Dict, key Name) []byte {
	if s, ok := enc[key].(String); ok {
		return s.Value
	}
	return nil
}

// keyLength devuelve el largo de la clave en bytes (R2 = 5; si no, /Length/8).
func keyLength(enc Dict, r int) int {
	if r == 2 {
		return 5
	}
	if bits, ok := intValue(enc[Name("Length")]); ok && bits >= 40 && bits <= 128 {
		return bits / 8
	}
	return 16
}

// padPassword completa o corta la contraseña a 32 bytes con el relleno.
func padPassword(pw []byte) []byte {
	out := make([]byte, 32)
	n := copy(out, pw)
	copy(out[n:], padding)
	return out
}

// fileKeyLegacy es el Algorithm 2: la clave de archivo a partir de una
// contraseña de USUARIO.
func fileKeyLegacy(enc Dict, r int, id, userPW []byte, encryptMeta bool) []byte {
	n := keyLength(enc, r)
	h := md5.New()
	h.Write(padPassword(userPW))
	h.Write(encBytes(enc, "O"))
	p, _ := intValue(enc[Name("P")])
	var pb [4]byte
	binary.LittleEndian.PutUint32(pb[:], uint32(int32(p)))
	h.Write(pb[:])
	h.Write(id)
	if r >= 4 && !encryptMeta {
		h.Write([]byte{0xFF, 0xFF, 0xFF, 0xFF})
	}
	sum := h.Sum(nil)
	if r >= 3 {
		for i := 0; i < 50; i++ {
			s := md5.Sum(sum[:n])
			sum = s[:]
		}
	}
	return sum[:n]
}

// userCheck es el Algorithm 4/5: lo que tendría que valer /U con esa clave.
func userCheck(key []byte, r int, id []byte) []byte {
	if r == 2 {
		return rc4Bytes(key, padding)
	}
	h := md5.New()
	h.Write(padding)
	h.Write(id)
	x := rc4Bytes(key, h.Sum(nil))
	for i := 1; i <= 19; i++ {
		x = rc4Bytes(xorKey(key, byte(i)), x)
	}
	return x
}

// authenticateLegacy prueba pw como contraseña de usuario y como de
// propietario (Algorithm 7: del /O se recupera la de usuario). Devuelve la
// clave de archivo o nil.
func authenticateLegacy(enc Dict, r, v int, id, pw []byte, encryptMeta bool) []byte {
	u := encBytes(enc, "U")
	tryUser := func(userPW []byte) []byte {
		key := fileKeyLegacy(enc, r, id, userPW, encryptMeta)
		want := userCheck(key, r, id)
		cmp := 32
		if r >= 3 {
			cmp = 16 // en R3+ solo los primeros 16 bytes de /U son significativos
		}
		if len(u) >= cmp && bytes.Equal(want[:cmp], u[:cmp]) {
			return key
		}
		return nil
	}
	if key := tryUser(pw); key != nil {
		return key
	}
	// Como contraseña de propietario.
	n := keyLength(enc, r)
	sum := md5.Sum(padPassword(pw))
	ownerKey := sum[:]
	if r >= 3 {
		for i := 0; i < 50; i++ {
			s := md5.Sum(ownerKey)
			ownerKey = s[:]
		}
	}
	ownerKey = ownerKey[:n]
	userPW := encBytes(enc, "O")
	if r == 2 {
		userPW = rc4Bytes(ownerKey, userPW)
	} else {
		for i := 19; i >= 0; i-- {
			userPW = rc4Bytes(xorKey(ownerKey, byte(i)), userPW)
		}
	}
	return tryUser(userPW)
}

// authenticateAES256 implementa la validación de R5/R6 para usuario y
// propietario y devuelve la clave de archivo (32 bytes) o nil.
func authenticateAES256(enc Dict, r int, pw []byte) []byte {
	if len(pw) > 127 {
		pw = pw[:127]
	}
	u, o := encBytes(enc, "U"), encBytes(enc, "O")
	ue, oe := encBytes(enc, "UE"), encBytes(enc, "OE")
	if len(u) < 48 || len(o) < 48 {
		return nil
	}
	hash := func(salt, udata []byte) []byte {
		if r == 5 {
			h := sha256.New()
			h.Write(pw)
			h.Write(salt)
			h.Write(udata)
			return h.Sum(nil)
		}
		return hash2B(pw, salt, udata)
	}
	if bytes.Equal(hash(u[32:40], nil), u[:32]) {
		return aesUnwrap(hash(u[40:48], nil), ue)
	}
	if bytes.Equal(hash(o[32:40], u[:48]), o[:32]) {
		return aesUnwrap(hash(o[40:48], u[:48]), oe)
	}
	return nil
}

// hash2B es el Algorithm 2.B de ISO 32000-2 (revisión 6): un hash iterado que
// alterna SHA-256/384/512 según los datos, con al menos 64 rondas.
func hash2B(pw, salt, udata []byte) []byte {
	h := sha256.New()
	h.Write(pw)
	h.Write(salt)
	h.Write(udata)
	k := h.Sum(nil)
	for round := 0; ; round++ {
		block := make([]byte, 0, len(pw)+len(k)+len(udata))
		block = append(block, pw...)
		block = append(block, k...)
		block = append(block, udata...)
		k1 := bytes.Repeat(block, 64)
		c, err := aes.NewCipher(k[:16])
		if err != nil {
			return nil
		}
		e := make([]byte, len(k1))
		cipher.NewCBCEncrypter(c, k[16:32]).CryptBlocks(e, k1)
		// Los 16 primeros bytes como entero big-endian, módulo 3: como 256 ≡ 1
		// (mód 3), alcanza con sumar los bytes.
		mod := 0
		for _, b := range e[:16] {
			mod += int(b)
		}
		switch mod % 3 {
		case 0:
			s := sha256.Sum256(e)
			k = s[:]
		case 1:
			s := sha512.Sum384(e)
			k = s[:]
		default:
			s := sha512.Sum512(e)
			k = s[:]
		}
		if round >= 63 && int(e[len(e)-1]) <= round+1-32 {
			break
		}
	}
	return k[:32]
}

// aesUnwrap descifra /UE u /OE: AES-256-CBC, IV en cero, sin relleno.
func aesUnwrap(key, wrapped []byte) []byte {
	if len(wrapped) != 32 {
		return nil
	}
	c, err := aes.NewCipher(key)
	if err != nil {
		return nil
	}
	out := make([]byte, 32)
	cipher.NewCBCDecrypter(c, make([]byte, 16)).CryptBlocks(out, wrapped)
	return out
}

func rc4Bytes(key, data []byte) []byte {
	c, err := rc4.NewCipher(key)
	if err != nil {
		return nil
	}
	out := make([]byte, len(data))
	c.XORKeyStream(out, data)
	return out
}

func xorKey(key []byte, v byte) []byte {
	out := make([]byte, len(key))
	for i, b := range key {
		out[i] = b ^ v
	}
	return out
}

// objectKey es el Algorithm 1: la clave de un objeto concreto (RC4 y AESV2).
func (h *securityHandler) objectKey(num, gen int, method cryptMethod) []byte {
	if method == cryptAESV3 {
		return h.key
	}
	m := md5.New()
	m.Write(h.key)
	m.Write([]byte{byte(num), byte(num >> 8), byte(num >> 16), byte(gen), byte(gen >> 8)})
	if method == cryptAESV2 {
		m.Write([]byte("sAlT"))
	}
	sum := m.Sum(nil)
	return sum[:min(len(h.key)+5, 16)]
}

// decrypt descifra datos de un objeto con el método dado.
func (h *securityHandler) decrypt(data []byte, num, gen int, method cryptMethod) []byte {
	switch method {
	case cryptRC4:
		return rc4Bytes(h.objectKey(num, gen, method), data)
	case cryptAESV2, cryptAESV3:
		return aesDecryptCBC(h.objectKey(num, gen, method), data)
	}
	return data
}

// aesDecryptCBC: los primeros 16 bytes son el IV; al final, relleno PKCS#7.
// Un bloque incompleto o un relleno inválido se tolera devolviendo lo que haya
// (hay generadores que escriben cadenas vacías sin IV).
func aesDecryptCBC(key, data []byte) []byte {
	if len(data) < 16 {
		return nil
	}
	c, err := aes.NewCipher(key)
	if err != nil {
		return data
	}
	body := data[16:]
	body = body[:len(body)/aes.BlockSize*aes.BlockSize]
	out := make([]byte, len(body))
	cipher.NewCBCDecrypter(c, data[:16]).CryptBlocks(out, body)
	if n := len(out); n > 0 {
		pad := int(out[n-1])
		if pad >= 1 && pad <= aes.BlockSize && pad <= n && bytes.Equal(out[n-pad:], bytes.Repeat([]byte{byte(pad)}, pad)) {
			out = out[:n-pad]
		}
	}
	return out
}

// decryptObject descifra en el lugar las cadenas de un objeto recién leído del
// archivo (num gen), y marca sus streams para descifrar al leer sus bytes.
func (d *Document) decryptObject(o Object, num, gen int) Object {
	h := d.crypt
	if h == nil || num == h.encryptNum {
		return o
	}
	var walk func(Object) Object
	walk = func(o Object) Object {
		switch v := o.(type) {
		case String:
			return String{Value: h.decrypt(v.Value, num, gen, h.strMethod), Hex: v.Hex}
		case Array:
			for i := range v {
				v[i] = walk(v[i])
			}
			return v
		case Dict:
			for k, e := range v {
				v[k] = walk(e)
			}
			return v
		case *Stream:
			for k, e := range v.Dict {
				v.Dict[k] = walk(e)
			}
			v.cryptNum, v.cryptGen, v.encrypted = num, gen, true
			return v
		}
		return o
	}
	return walk(o)
}

// streamNeedsDecrypt aplica las excepciones del estándar para streams.
func (h *securityHandler) streamNeedsDecrypt(stm *Stream) bool {
	switch stm.Dict.GetName("Type") {
	case "XRef":
		return false
	case "Metadata":
		if !h.encryptMeta {
			return false
		}
	}
	// Un filtro /Crypt con /Name /Identity pide explícitamente no descifrar.
	if f, ok := stm.Dict[Name("Filter")].(Array); ok && len(f) > 0 {
		if n, ok := f[0].(Name); ok && n == "Crypt" {
			return false
		}
	}
	return h.stmMethod != cryptNone
}
