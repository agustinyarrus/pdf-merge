package pdf

import (
	"fmt"
	"strconv"
)

// lexer.go convierte bytes de PDF en objetos. El PDF es un formato de tokens
// simples (números, nombres, cadenas, arreglos, diccionarios) con referencias
// indirectas; el analizador es recursivo-descendente y trabaja sobre un buffer
// en memoria con un cursor.

// scanner recorre un slice de bytes con un cursor.
type scanner struct {
	buf []byte
	pos int
}

func newScanner(buf []byte) *scanner { return &scanner{buf: buf} }

func isWhite(c byte) bool {
	return c == 0 || c == '\t' || c == '\n' || c == '\f' || c == '\r' || c == ' '
}

func isDelim(c byte) bool {
	switch c {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return false
}

// skipWhite avanza sobre espacios y comentarios (% hasta fin de línea).
func (s *scanner) skipWhite() {
	for s.pos < len(s.buf) {
		c := s.buf[s.pos]
		if isWhite(c) {
			s.pos++
			continue
		}
		if c == '%' {
			for s.pos < len(s.buf) && s.buf[s.pos] != '\n' && s.buf[s.pos] != '\r' {
				s.pos++
			}
			continue
		}
		break
	}
}

// parseObject lee un objeto en la posición actual. resolveRefs=false hace que
// "n g R" se devuelva como Ref (no se sigue).
func (s *scanner) parseObject() (Object, error) {
	s.skipWhite()
	if s.pos >= len(s.buf) {
		return nil, fmt.Errorf("fin de datos inesperado")
	}
	c := s.buf[s.pos]
	switch {
	case c == '/':
		return s.parseName()
	case c == '(':
		return s.parseLiteralString()
	case c == '<':
		if s.pos+1 < len(s.buf) && s.buf[s.pos+1] == '<' {
			return s.parseDict()
		}
		return s.parseHexString()
	case c == '[':
		return s.parseArray()
	case c == '+' || c == '-' || c == '.' || (c >= '0' && c <= '9'):
		return s.parseNumberOrRef()
	default:
		return s.parseKeyword()
	}
}

func (s *scanner) parseName() (Object, error) {
	s.pos++ // '/'
	start := s.pos
	var out []byte
	for s.pos < len(s.buf) {
		c := s.buf[s.pos]
		if isWhite(c) || isDelim(c) {
			break
		}
		if c == '#' && s.pos+2 < len(s.buf) {
			hi, ok1 := hexVal(s.buf[s.pos+1])
			lo, ok2 := hexVal(s.buf[s.pos+2])
			if ok1 && ok2 {
				out = append(out, hi<<4|lo)
				s.pos += 3
				continue
			}
		}
		out = append(out, c)
		s.pos++
	}
	if out == nil {
		return Name(s.buf[start:s.pos]), nil
	}
	return Name(out), nil
}

func (s *scanner) parseLiteralString() (Object, error) {
	s.pos++ // '('
	depth := 1
	var out []byte
	for s.pos < len(s.buf) {
		c := s.buf[s.pos]
		s.pos++
		switch c {
		case '\\':
			if s.pos >= len(s.buf) {
				break
			}
			e := s.buf[s.pos]
			s.pos++
			switch e {
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case 'b':
				out = append(out, '\b')
			case 'f':
				out = append(out, '\f')
			case '(', ')', '\\':
				out = append(out, e)
			case '\r':
				if s.pos < len(s.buf) && s.buf[s.pos] == '\n' {
					s.pos++
				}
			case '\n':
				// barra invertida seguida de fin de línea: continuación, nada
			default:
				if e >= '0' && e <= '7' { // octal de 1 a 3 dígitos
					val := int(e - '0')
					for k := 0; k < 2 && s.pos < len(s.buf) && s.buf[s.pos] >= '0' && s.buf[s.pos] <= '7'; k++ {
						val = val*8 + int(s.buf[s.pos]-'0')
						s.pos++
					}
					out = append(out, byte(val))
				} else {
					out = append(out, e)
				}
			}
		case '(':
			depth++
			out = append(out, c)
		case ')':
			depth--
			if depth == 0 {
				return String{Value: out}, nil
			}
			out = append(out, c)
		default:
			out = append(out, c)
		}
	}
	return String{Value: out}, nil
}

func (s *scanner) parseHexString() (Object, error) {
	s.pos++ // '<'
	var nibbles []byte
	for s.pos < len(s.buf) && s.buf[s.pos] != '>' {
		if v, ok := hexVal(s.buf[s.pos]); ok {
			nibbles = append(nibbles, v)
		}
		s.pos++
	}
	if s.pos < len(s.buf) {
		s.pos++ // '>'
	}
	if len(nibbles)%2 == 1 {
		nibbles = append(nibbles, 0) // el último nibble impar se completa con 0
	}
	out := make([]byte, len(nibbles)/2)
	for i := range out {
		out[i] = nibbles[2*i]<<4 | nibbles[2*i+1]
	}
	return String{Value: out, Hex: true}, nil
}

func (s *scanner) parseArray() (Object, error) {
	s.pos++ // '['
	arr := Array{}
	for {
		s.skipWhite()
		if s.pos >= len(s.buf) {
			return nil, fmt.Errorf("arreglo sin cierre")
		}
		if s.buf[s.pos] == ']' {
			s.pos++
			return arr, nil
		}
		o, err := s.parseObject()
		if err != nil {
			return nil, err
		}
		arr = append(arr, o)
	}
}

func (s *scanner) parseDict() (Object, error) {
	s.pos += 2 // '<<'
	d := Dict{}
	for {
		s.skipWhite()
		if s.pos+1 < len(s.buf) && s.buf[s.pos] == '>' && s.buf[s.pos+1] == '>' {
			s.pos += 2
			break
		}
		if s.pos >= len(s.buf) {
			return nil, fmt.Errorf("diccionario sin cierre")
		}
		if s.buf[s.pos] != '/' {
			return nil, fmt.Errorf("clave de diccionario esperada en pos %d", s.pos)
		}
		key, err := s.parseName()
		if err != nil {
			return nil, err
		}
		val, err := s.parseObject()
		if err != nil {
			return nil, err
		}
		d[key.(Name)] = val
	}
	// ¿Le sigue un stream?
	save := s.pos
	s.skipWhite()
	if s.hasPrefix("stream") {
		return s.parseStream(d)
	}
	s.pos = save
	return d, nil
}

func (s *scanner) parseStream(d Dict) (Object, error) {
	s.pos += len("stream")
	// Tras "stream" viene CRLF o LF.
	if s.pos < len(s.buf) && s.buf[s.pos] == '\r' {
		s.pos++
	}
	if s.pos < len(s.buf) && s.buf[s.pos] == '\n' {
		s.pos++
	}
	start := s.pos
	// La longitud declarada manda; si no es un entero directo (es una Ref), se
	// busca "endstream" como respaldo.
	if length, ok := intValue(d[Name("Length")]); ok && start+length <= len(s.buf) {
		raw := s.buf[start : start+length]
		s.pos = start + length
		s.skipWhite()
		if s.hasPrefix("endstream") {
			s.pos += len("endstream")
			return &Stream{Dict: d, Raw: raw}, nil
		}
		// La longitud mintió: caer al escaneo por "endstream".
		s.pos = start
	}
	idx := indexEndstream(s.buf[start:])
	if idx < 0 {
		return nil, fmt.Errorf("stream sin endstream")
	}
	raw := s.buf[start : start+idx]
	s.pos = start + idx + len("endstream")
	stm := &Stream{Dict: d, Raw: trimEOL(raw)}
	if ref, ok := d[Name("Length")].(Ref); ok {
		// El corte por "endstream" es provisorio: el Document lo corrige con la
		// longitud real cuando resuelva la referencia (Document.streamData).
		stm.lenRef, stm.dataStart, stm.file = &ref, start, s.buf
	}
	return stm, nil
}

func (s *scanner) parseNumberOrRef() (Object, error) {
	startNum, isReal := s.scanNumberToken()
	if isReal {
		f, _ := strconv.ParseFloat(string(s.buf[startNum:s.pos]), 64)
		return Real(f), nil
	}
	firstEnd := s.pos
	first, err := strconv.ParseInt(string(s.buf[startNum:firstEnd]), 10, 64)
	if err != nil {
		// Un entero que desborda int64 (algunos generadores rotos los escriben)
		// se conserva como real en vez de convertirse en 0 en silencio.
		f, ferr := strconv.ParseFloat(string(s.buf[startNum:firstEnd]), 64)
		if ferr != nil {
			return Integer(0), nil
		}
		return Real(f), nil
	}

	// Puede ser "n g R" (referencia). Se mira adelante: otro entero seguido de
	// 'R' con frontera de token. Si no encaja, se retrocede y queda el entero.
	save := s.pos
	s.skipWhite()
	if s.pos < len(s.buf) && s.buf[s.pos] >= '0' && s.buf[s.pos] <= '9' {
		genStart, gIsReal := s.scanNumberToken()
		if !gIsReal {
			gen, _ := strconv.Atoi(string(s.buf[genStart:s.pos]))
			afterGen := s.pos
			s.skipWhite()
			if s.pos < len(s.buf) && s.buf[s.pos] == 'R' && s.tokenBoundaryAt(s.pos+1) {
				s.pos++ // 'R'
				return Ref{Num: int(first), Gen: gen}, nil
			}
			s.pos = afterGen
		}
	}
	s.pos = save
	return Integer(first), nil
}

func (s *scanner) parseKeyword() (Object, error) {
	start := s.pos
	for s.pos < len(s.buf) && !isWhite(s.buf[s.pos]) && !isDelim(s.buf[s.pos]) {
		s.pos++
	}
	switch string(s.buf[start:s.pos]) {
	case "true":
		return Bool(true), nil
	case "false":
		return Bool(false), nil
	case "null":
		return Null{}, nil
	case "":
		return nil, fmt.Errorf("token vacío en pos %d", start)
	}
	return Null{}, nil // palabra desconocida: tratar como null y seguir
}
