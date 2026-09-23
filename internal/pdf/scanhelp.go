package pdf

import (
	"bytes"
	"strconv"
)

// scanhelp.go junta las utilidades de bajo nivel del scanner: reconocer
// tokens, límites de token, y localizar delimitadores dentro de un stream.

// scanNumberToken consume un número entero o real y devuelve la posición donde
// empezó y si tenía punto decimal. Deja el cursor al final del número.
func (s *scanner) scanNumberToken() (start int, isReal bool) {
	start = s.pos
	if s.pos < len(s.buf) && (s.buf[s.pos] == '+' || s.buf[s.pos] == '-') {
		s.pos++
	}
	for s.pos < len(s.buf) {
		c := s.buf[s.pos]
		switch {
		case c >= '0' && c <= '9':
			s.pos++
		case c == '.':
			isReal = true
			s.pos++
		default:
			return start, isReal
		}
	}
	return start, isReal
}

// hasPrefix dice si en la posición actual comienza la palabra p.
func (s *scanner) hasPrefix(p string) bool {
	return s.pos+len(p) <= len(s.buf) && string(s.buf[s.pos:s.pos+len(p)]) == p
}

// tokenBoundaryAt dice si en la posición i hay fin de token (espacio,
// delimitador o fin de datos): sirve para no confundir "R" con "Rotate".
func (s *scanner) tokenBoundaryAt(i int) bool {
	return i >= len(s.buf) || isWhite(s.buf[i]) || isDelim(s.buf[i])
}

// readInt lee un entero decimal en la posición actual (saltando espacios
// previos). Devuelve false si no hay dígitos.
func (s *scanner) readInt() (int, bool) {
	s.skipWhite()
	start := s.pos
	if s.pos < len(s.buf) && (s.buf[s.pos] == '+' || s.buf[s.pos] == '-') {
		s.pos++
	}
	for s.pos < len(s.buf) && s.buf[s.pos] >= '0' && s.buf[s.pos] <= '9' {
		s.pos++
	}
	if s.pos == start {
		return 0, false
	}
	n, err := strconv.Atoi(string(s.buf[start:s.pos]))
	return n, err == nil
}

// skipSingleEOL salta un fin de línea (CRLF, LF o CR) y los espacios previos.
func (s *scanner) skipSingleEOL() {
	for s.pos < len(s.buf) && (s.buf[s.pos] == ' ' || s.buf[s.pos] == '\t') {
		s.pos++
	}
	if s.pos < len(s.buf) && s.buf[s.pos] == '\r' {
		s.pos++
	}
	if s.pos < len(s.buf) && s.buf[s.pos] == '\n' {
		s.pos++
	}
}

func hexVal(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

// indexEndstream busca el primer "endstream" dentro de buf y devuelve su índice
// relativo, o -1. Se usa cuando /Length mintió o era indirecto.
func indexEndstream(buf []byte) int {
	return bytes.Index(buf, []byte("endstream"))
}

// trimEOL saca un único salto de línea final (CRLF o LF) que separa los datos
// del "endstream", sin tocar el resto del contenido del stream.
func trimEOL(b []byte) []byte {
	if n := len(b); n >= 2 && b[n-2] == '\r' && b[n-1] == '\n' {
		return b[:n-2]
	} else if n >= 1 && (b[n-1] == '\n' || b[n-1] == '\r') {
		return b[:n-1]
	}
	return b
}
