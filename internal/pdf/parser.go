package pdf

import (
	"bytes"
	"fmt"
	"strconv"
)

// parser.go arma un Document a partir de los bytes de un PDF: localiza la tabla
// de referencias cruzadas (clásica o en stream), sigue la cadena de
// actualizaciones incrementales (/Prev) y resuelve objetos a pedido, incluidos
// los que viven comprimidos dentro de object streams.

// xrefEntry ubica un objeto: en un offset del archivo, o dentro de un object
// stream comprimido.
type xrefEntry struct {
	inStream bool
	offset   int // si inStream=false: byte donde arranca "num gen obj"
	stmNum   int // si inStream=true: número del object stream contenedor
	stmIdx   int // índice dentro del object stream
}

// Document es un PDF parseado y navegable.
type Document struct {
	raw     []byte
	xref    map[int]xrefEntry
	trailer Dict
	cache   map[int]Object
	objStm  map[int]*objectStream // cache de object streams ya expandidos
	pages   []Page                // árbol de páginas ya recorrido (memoizado)
	dests   map[string]Object     // destinos con nombre aplanados (memoizado)
	rebuilt bool                  // el xref ya se reconstruyó por barrido (una sola vez)
	crypt   *securityHandler      // nil si el documento no está cifrado
}

// Restricted dice si el documento venía cifrado con restricciones del
// propietario (impresión, copia, edición), que la salida no conserva.
func (d *Document) Restricted() bool { return d.crypt != nil && d.crypt.restrictions }

// Encrypted dice si el documento venía cifrado.
func (d *Document) Encrypted() bool { return d.crypt != nil }

// headerWindow: el estándar (y Acrobat) toleran basura antes de "%PDF-" dentro
// del primer kilobyte; aparece cuando un gateway de correo o un envoltorio de
// descarga le antepone algo al archivo.
const headerWindow = 1024

// Parse lee un PDF completo desde sus bytes. Si está cifrado, prueba la
// contraseña vacía (el caso de los PDF "protegidos" que se abren sin pedir nada).
func Parse(raw []byte) (*Document, error) { return ParseWithPasswords(raw, nil) }

// ParseWithPasswords es Parse probando además las contraseñas dadas, como de
// usuario o de propietario.
func ParseWithPasswords(raw []byte, passwords []string) (*Document, error) {
	hdr := bytes.Index(raw[:min(len(raw), headerWindow)], []byte("%PDF-"))
	if hdr < 0 {
		return nil, fmt.Errorf("no parece un PDF (no hay encabezado %%PDF- en el primer KB)")
	}
	// Si la basura se agregó después de escribir el PDF, los offsets cuentan
	// desde "%PDF-": se descarta el prefijo. Si el generador los contó desde el
	// byte 0, la verificación de números de getObject lo detecta y el xref se
	// reconstruye por barrido.
	raw = raw[hdr:]
	d := &Document{
		raw:    raw,
		xref:   map[int]xrefEntry{},
		cache:  map[int]Object{},
		objStm: map[int]*objectStream{},
	}

	var chainErr error
	if start, err := lastStartXref(raw); err != nil {
		chainErr = err
	} else if err := d.readXRefChain(start); err != nil {
		chainErr = err
	}
	if chainErr != nil {
		d.rebuilt = true
		d.xref = map[int]xrefEntry{}
		if err := d.rebuildXref(); err != nil {
			return nil, fmt.Errorf("%v; tampoco pude reconstruir la tabla: %w", chainErr, err)
		}
	}
	// El cifrado se configura ANTES de validar el catálogo: si el catálogo vive
	// en un object stream cifrado, sin la clave no se podría ni leer.
	if err := d.ensureEncryption(passwords); err != nil {
		return nil, err
	}
	if !d.rootLooksValid() {
		if !d.rebuilt {
			d.rebuilt = true
			if err := d.rebuildXref(); err != nil {
				return nil, err
			}
		}
		if err := d.rebuildTrailer(); err != nil {
			return nil, err
		}
		if err := d.ensureEncryption(passwords); err != nil {
			return nil, err
		}
	}
	return d, nil
}

// ensureEncryption arma el descifrado si el trailer declara /Encrypt y todavía
// no está armado.
func (d *Document) ensureEncryption(passwords []string) error {
	if d.crypt != nil || d.trailer == nil || d.trailer[Name("Encrypt")] == nil {
		return nil
	}
	return d.setupEncryption(passwords)
}

// rootLooksValid dice si el trailer lleva a un catálogo utilizable.
func (d *Document) rootLooksValid() bool {
	if d.trailer == nil {
		return false
	}
	root, err := d.resolveDict(d.trailer[Name("Root")])
	return err == nil && root != nil && (root.GetName("Type") == "Catalog" || root[Name("Pages")] != nil)
}

// lastStartXref busca el último "startxref" y devuelve el offset que declara.
func lastStartXref(raw []byte) (int, error) {
	idx := bytes.LastIndex(raw, []byte("startxref"))
	if idx < 0 {
		return 0, fmt.Errorf("falta startxref")
	}
	s := newScanner(raw)
	s.pos = idx + len("startxref")
	s.skipWhite()
	numStart := s.pos
	for s.pos < len(raw) && raw[s.pos] >= '0' && raw[s.pos] <= '9' {
		s.pos++
	}
	n, err := strconv.Atoi(string(raw[numStart:s.pos]))
	if err != nil {
		return 0, fmt.Errorf("startxref sin offset")
	}
	return n, nil
}

// readXRefChain lee la sección xref en `offset` y toda la cadena /Prev.
// Las entradas ya presentes NO se pisan: se recorre de la más nueva a la más
// vieja, y la primera aparición de cada objeto es la vigente.
func (d *Document) readXRefChain(offset int) error {
	seen := map[int]bool{}
	for offset > 0 && offset < len(d.raw) {
		if seen[offset] {
			break // ciclo defensivo
		}
		seen[offset] = true

		trailer, prev, xrefStm, err := d.readXRefSection(offset)
		if err != nil {
			return err
		}
		if d.trailer == nil {
			d.trailer = trailer
		} else {
			// Completar claves del catálogo que solo estén en trailers viejos.
			for k, v := range trailer {
				if _, ok := d.trailer[k]; !ok {
					d.trailer[k] = v
				}
			}
		}
		// Un xref híbrido apunta también a un xref stream con /XRefStm.
		if xrefStm > 0 && !seen[xrefStm] {
			if _, _, _, err := d.readXRefSection(xrefStm); err == nil {
				seen[xrefStm] = true
			}
		}
		offset = prev
	}
	if len(d.xref) == 0 {
		return fmt.Errorf("no encontré ninguna entrada xref")
	}
	return nil
}

// readXRefSection lee una sección en offset: puede ser una tabla clásica
// ("xref ... trailer") o un xref stream (un objeto con /Type /XRef).
func (d *Document) readXRefSection(offset int) (trailer Dict, prev, xrefStm int, err error) {
	s := newScanner(d.raw)
	s.pos = offset
	s.skipWhite()
	if s.hasPrefix("xref") {
		return d.readClassicXRef(s)
	}
	return d.readXRefStream(offset)
}

// readClassicXRef lee "xref\n<sub-secciones>\ntrailer<<...>>".
func (d *Document) readClassicXRef(s *scanner) (Dict, int, int, error) {
	s.pos += len("xref")
	for {
		s.skipWhite()
		if s.hasPrefix("trailer") {
			s.pos += len("trailer")
			s.skipWhite()
			obj, err := s.parseObject()
			if err != nil {
				return nil, 0, 0, err
			}
			tr, ok := obj.(Dict)
			if !ok {
				return nil, 0, 0, fmt.Errorf("trailer no es un diccionario")
			}
			prev, xrefStm := -1, -1
			if p, ok := intValue(tr[Name("Prev")]); ok {
				prev = p
			}
			if x, ok := intValue(tr[Name("XRefStm")]); ok {
				xrefStm = x
			}
			return tr, prev, xrefStm, nil
		}
		// Sub-sección: "start count" y luego count entradas "offset gen n|f".
		// Se leen por tokens y no por ancho fijo: el estándar dice 20 bytes por
		// línea, pero hay generadores que escriben 19 o 21 y con ancho fijo
		// todo lo que sigue quedaría corrido.
		startObj, ok1 := s.readInt()
		count, ok2 := s.readInt()
		if !ok1 || !ok2 {
			return nil, 0, 0, fmt.Errorf("sub-sección xref inválida")
		}
		for i := 0; i < count; i++ {
			off, ok1 := s.readInt()
			_, ok2 := s.readInt() // generación
			s.skipWhite()
			if !ok1 || !ok2 || s.pos >= len(d.raw) {
				return nil, 0, 0, fmt.Errorf("xref truncada en la entrada %d", startObj+i)
			}
			typ := d.raw[s.pos]
			s.pos++
			num := startObj + i
			if typ == 'n' {
				if _, exists := d.xref[num]; !exists {
					d.xref[num] = xrefEntry{offset: off}
				}
			}
		}
	}
}
