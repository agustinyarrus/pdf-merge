package pdf

import (
	"bytes"
	"testing"
)

// mergeTwice combina el mismo PDF parseado DOS veces por separado (dos
// Documents distintos, así el copiador por documento no los comparte): todo lo
// que no sea página debería fusionarse en la deduplicación.
func mergeTwice(t *testing.T, raw []byte, dedupe bool) (*Builder, DedupeStats) {
	t.Helper()
	var sources []Source
	for i := 0; i < 2; i++ {
		doc, err := Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		pages, err := doc.Pages()
		if err != nil {
			t.Fatal(err)
		}
		sources = append(sources, Source{Doc: doc, Pages: pages})
	}
	out := NewBuilder()
	if _, err := Merge(out, sources, MergeOptions{}); err != nil {
		t.Fatal(err)
	}
	var st DedupeStats
	if dedupe {
		st = out.Dedupe()
	}
	return out, st
}

func reparsePages(t *testing.T, b *Builder) int {
	t.Helper()
	var buf bytes.Buffer
	if _, err := b.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	doc, err := Parse(buf.Bytes())
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	n, err := doc.PageCount()
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// TestDedupeEntreDocumentos: el mismo contenido en dos documentos distintos se
// escribe una sola vez, y las páginas siguen siendo todas.
func TestDedupeEntreDocumentos(t *testing.T) {
	raw := read(t, fxDir(t), "d.pdf") // 2 páginas, con una imagen
	sin, _ := mergeTwice(t, raw, false)
	con, st := mergeTwice(t, raw, true)
	if st.Merged == 0 || st.BytesSaved == 0 {
		t.Fatalf("no fusionó nada: %+v", st)
	}
	if len(con.objects) >= len(sin.objects) {
		t.Errorf("objetos con dedupe = %d, sin dedupe = %d: tendría que haber menos", len(con.objects)-1, len(sin.objects)-1)
	}
	if n := reparsePages(t, con); n != 4 {
		t.Errorf("páginas tras dedupe = %d, esperaba 4 (las páginas nunca se fusionan)", n)
	}
}

// TestDedupeMismaPaginaDosVeces: pedir la misma página dos veces produce dos
// diccionarios de página idénticos; tienen identidad y deben quedar dos.
func TestDedupeMismaPaginaDosVeces(t *testing.T) {
	doc, err := Parse(read(t, fxDir(t), "a.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	all, _ := doc.Pages()
	out := NewBuilder()
	if _, err := Merge(out, []Source{{Doc: doc, Pages: []Page{all[0], all[0]}}}, MergeOptions{}); err != nil {
		t.Fatal(err)
	}
	out.Dedupe()
	if n := reparsePages(t, out); n != 2 {
		t.Errorf("páginas = %d, esperaba 2", n)
	}
}

// TestTarjanCiclo: en el grafo 1→2→3→1 y 4→5, el ciclo es una sola componente y
// sale DESPUÉS de lo que no apunta a nadie (orden topológico inverso).
func TestTarjanCiclo(t *testing.T) {
	adj := [][]int{nil, {2}, {3}, {1}, {5}, nil}
	sccs := tarjanSCC(adj, 6)
	pos := map[int]int{}
	for i, comp := range sccs {
		for _, v := range comp {
			pos[v] = i
		}
	}
	if pos[1] != pos[2] || pos[2] != pos[3] {
		t.Errorf("1, 2 y 3 forman un ciclo y deberían compartir componente: %v", sccs)
	}
	if pos[5] > pos[4] {
		t.Errorf("5 (sin salidas) debería salir antes que 4, que lo apunta: %v", sccs)
	}
}

// TestDedupeCicloNoSeFusiona: dos ciclos idénticos (A↔B y C↔D) no se fusionan
// entre sí: dentro de un ciclo, cada objeto conserva su identidad.
func TestDedupeCicloNoSeFusiona(t *testing.T) {
	b := NewBuilder()
	a := b.Reserve()
	bb := b.Reserve()
	c := b.Reserve()
	d := b.Reserve()
	b.Set(a, Dict{"X": Ref{Num: bb}})
	b.Set(bb, Dict{"X": Ref{Num: a}})
	b.Set(c, Dict{"X": Ref{Num: d}})
	b.Set(d, Dict{"X": Ref{Num: c}})
	root := b.Add(Dict{"Type": Name("Catalog"), "A": Ref{Num: a}, "C": Ref{Num: c}})
	b.SetRoot(root)
	st := b.Dedupe()
	if st.Merged != 0 {
		t.Errorf("fusionó %d objetos de ciclos: no debería fusionar ninguno", st.Merged)
	}
}

// TestDedupeHojasIguales: dos diccionarios hoja idénticos se fusionan y la
// referencia del que desaparece apunta al que queda.
func TestDedupeHojasIguales(t *testing.T) {
	b := NewBuilder()
	x := b.Add(Dict{"Type": Name("ExtGState"), "ca": Real(0.5)})
	y := b.Add(Dict{"Type": Name("ExtGState"), "ca": Real(0.5)})
	root := b.Add(Dict{"Type": Name("Catalog"), "G1": Ref{Num: x}, "G2": Ref{Num: y}})
	b.SetRoot(root)
	st := b.Dedupe()
	if st.Merged != 1 {
		t.Fatalf("fusionados = %d, esperaba 1", st.Merged)
	}
	cat := b.objects[b.root].(Dict)
	if cat["G1"] != cat["G2"] {
		t.Errorf("G1 y G2 deberían apuntar al mismo objeto: %v vs %v", cat["G1"], cat["G2"])
	}
}
