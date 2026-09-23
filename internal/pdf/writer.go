package pdf

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
)

// writer.go emite un PDF nuevo: numera los objetos de 1 a N, los serializa, y
// arma una tabla xref clásica y el trailer. No comprime ni usa object streams:
// la salida es simple, válida y abrible por cualquier lector.

// Builder acumula los objetos del documento de salida y los escribe.
type Builder struct {
	objects []Object // índice 0 no se usa (los objetos van de 1 a N)
	root    int      // número del objeto catálogo
}

// NewBuilder crea un constructor vacío.
func NewBuilder() *Builder {
	return &Builder{objects: make([]Object, 1)} // reservar el índice 0
}

// Add registra un objeto y devuelve el número que le tocó.
func (b *Builder) Add(o Object) int {
	b.objects = append(b.objects, o)
	return len(b.objects) - 1
}

// Reserve aparta un número de objeto para rellenarlo después (útil para ciclos
// como el árbol de páginas, donde el padre referencia hijos y viceversa).
func (b *Builder) Reserve() int {
	return b.Add(Null{})
}

// Set completa un número reservado.
func (b *Builder) Set(num int, o Object) { b.objects[num] = o }

// SetRoot marca cuál objeto es el catálogo.
func (b *Builder) SetRoot(num int) { b.root = num }

// WriteTo serializa todo el documento a w.
func (b *Builder) WriteTo(w io.Writer) (int64, error) {
	bw := bufio.NewWriterSize(w, 1<<16)
	cw := &countWriter{w: bw}

	// Encabezado con un comentario binario, como recomienda el estándar para
	// que las herramientas traten el archivo como binario.
	fmt.Fprint(cw, "%PDF-1.7\n%\xe2\xe3\xcf\xd3\n")

	offsets := make([]int64, len(b.objects))
	for num := 1; num < len(b.objects); num++ {
		offsets[num] = cw.n
		fmt.Fprintf(cw, "%d 0 obj\n", num)
		writeObject(cw, b.objects[num])
		fmt.Fprint(cw, "\nendobj\n")
	}

	xrefPos := cw.n
	fmt.Fprintf(cw, "xref\n0 %d\n", len(b.objects))
	fmt.Fprint(cw, "0000000000 65535 f \n") // objeto 0, cabeza de la lista libre
	for num := 1; num < len(b.objects); num++ {
		fmt.Fprintf(cw, "%010d 00000 n \n", offsets[num])
	}

	trailer := Dict{
		Name("Size"): Integer(len(b.objects)),
		Name("Root"): Ref{Num: b.root, Gen: 0},
	}
	fmt.Fprint(cw, "trailer\n")
	writeObject(cw, trailer)
	fmt.Fprintf(cw, "\nstartxref\n%d\n%%%%EOF\n", xrefPos)

	if err := bw.Flush(); err != nil {
		return cw.n, err
	}
	return cw.n, nil
}

// countWriter cuenta bytes escritos para calcular offsets del xref.
type countWriter struct {
	w io.Writer
	n int64
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// writeObject serializa un objeto en sintaxis PDF.
func writeObject(w io.Writer, o Object) {
	switch v := o.(type) {
	case nil, Null:
		io.WriteString(w, "null")
	case Bool:
		if v {
			io.WriteString(w, "true")
		} else {
			io.WriteString(w, "false")
		}
	case Integer:
		io.WriteString(w, strconv.FormatInt(int64(v), 10))
	case Real:
		io.WriteString(w, formatReal(float64(v)))
	case Name:
		writeName(w, string(v))
	case String:
		writeString(w, v)
	case Ref:
		fmt.Fprintf(w, "%d %d R", v.Num, v.Gen)
	case Array:
		io.WriteString(w, "[")
		for i, e := range v {
			if i > 0 {
				io.WriteString(w, " ")
			}
			writeObject(w, e)
		}
		io.WriteString(w, "]")
	case Dict:
		writeDict(w, v)
	case *Stream:
		// /Length directo y siempre igual a los bytes que se escriben: nunca se
		// confía en el valor copiado del origen.
		d := make(Dict, len(v.Dict)+1)
		for k, val := range v.Dict {
			d[k] = val
		}
		d[Name("Length")] = Integer(len(v.Raw))
		writeDict(w, d)
		io.WriteString(w, "\nstream\n")
		w.Write(v.Raw)
		io.WriteString(w, "\nendstream")
	default:
		io.WriteString(w, "null")
	}
}

func writeDict(w io.Writer, d Dict) {
	io.WriteString(w, "<<")
	for _, k := range d.sortedKeys() {
		io.WriteString(w, " ")
		writeName(w, string(k))
		io.WriteString(w, " ")
		writeObject(w, d[k])
	}
	io.WriteString(w, " >>")
}

// writeName escribe /Nombre escapando lo que no sea imprimible regular.
func writeName(w io.Writer, name string) {
	io.WriteString(w, "/")
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c < '!' || c > '~' || isDelim(c) || c == '#' {
			fmt.Fprintf(w, "#%02X", c)
		} else {
			w.Write([]byte{c})
		}
	}
}

// writeString escribe una cadena en hexadecimal cuando trae bytes no
// imprimibles, y como literal cuando es texto limpio.
func writeString(w io.Writer, s String) {
	if s.Hex || !isPrintableString(s.Value) {
		io.WriteString(w, "<")
		const hexdigits = "0123456789ABCDEF"
		buf := make([]byte, 0, len(s.Value)*2)
		for _, b := range s.Value {
			buf = append(buf, hexdigits[b>>4], hexdigits[b&0xf])
		}
		w.Write(buf)
		io.WriteString(w, ">")
		return
	}
	io.WriteString(w, "(")
	for _, b := range s.Value {
		switch b {
		case '(', ')', '\\':
			w.Write([]byte{'\\', b})
		case '\n':
			io.WriteString(w, "\\n")
		case '\r':
			io.WriteString(w, "\\r")
		default:
			w.Write([]byte{b})
		}
	}
	io.WriteString(w, ")")
}

func isPrintableString(b []byte) bool {
	for _, c := range b {
		if c != '\n' && c != '\r' && c != '\t' && (c < 0x20 || c > 0x7e) {
			return false
		}
	}
	return true
}
