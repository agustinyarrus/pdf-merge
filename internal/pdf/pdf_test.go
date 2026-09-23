package pdf

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func fxDir(t *testing.T) string {
	dir := filepath.Join(os.TempDir(), "navaja-pdf")
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("no hay fixtures de PDF en %s", dir)
	}
	return dir
}

func read(t *testing.T, dir, name string) []byte {
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("leer %s: %v", name, err)
	}
	return b
}

// TestParsePageCounts: el parser cuenta bien las páginas, tanto en PDF con xref
// clásico como con object streams.
func TestParsePageCounts(t *testing.T) {
	dir := fxDir(t)
	casos := map[string]int{
		"a.pdf": 3, "b.pdf": 2, "c.pdf": 5, "c_objstm.pdf": 5, "d.pdf": 2,
	}
	for name, want := range casos {
		t.Run(name, func(t *testing.T) {
			doc, err := Parse(read(t, dir, name))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			n, err := doc.PageCount()
			if err != nil {
				t.Fatalf("pagecount: %v", err)
			}
			if n != want {
				t.Errorf("páginas = %d, esperaba %d", n, want)
			}
		})
	}
}

// TestMergeAll: combinar a+b+c da la suma de páginas y produce un PDF re-parseable.
func TestMergeAll(t *testing.T) {
	dir := fxDir(t)
	inputs := [][]byte{read(t, dir, "a.pdf"), read(t, dir, "b.pdf"), read(t, dir, "c.pdf")}
	var buf bytes.Buffer
	res, err := MergeFiles(inputs, nil, &buf)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if res.TotalPages != 10 {
		t.Errorf("páginas combinadas = %d, esperaba 10", res.TotalPages)
	}
	// El resultado debe re-parsearse con la misma cuenta.
	doc, err := Parse(buf.Bytes())
	if err != nil {
		t.Fatalf("re-parse del merge: %v", err)
	}
	n, err := doc.PageCount()
	if err != nil {
		t.Fatalf("pagecount del merge: %v", err)
	}
	if n != 10 {
		t.Errorf("re-parse: páginas = %d, esperaba 10", n)
	}
}

// TestMergeObjStm: combinar un PDF con object streams funciona igual.
func TestMergeObjStm(t *testing.T) {
	dir := fxDir(t)
	inputs := [][]byte{read(t, dir, "c_objstm.pdf"), read(t, dir, "a.pdf")}
	var buf bytes.Buffer
	res, err := MergeFiles(inputs, nil, &buf)
	if err != nil {
		t.Fatalf("merge objstm: %v", err)
	}
	if res.TotalPages != 8 {
		t.Errorf("páginas = %d, esperaba 8", res.TotalPages)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF-")) {
		t.Error("la salida no tiene encabezado PDF")
	}
}

// TestPageRange: selección de páginas.
func TestPageRange(t *testing.T) {
	dir := fxDir(t)
	doc, err := Parse(read(t, dir, "c.pdf")) // 5 páginas
	if err != nil {
		t.Fatal(err)
	}
	all, err := doc.Pages()
	if err != nil {
		t.Fatal(err)
	}
	casos := map[string]int{
		"1-3":     3,
		"2,4":     2,
		"3-":      3, // 3,4,5
		"impares": 3, // 1,3,5
		"pares":   2, // 2,4
	}
	for spec, want := range casos {
		pr, err := ParsePageRange(spec)
		if err != nil {
			t.Fatalf("range %q: %v", spec, err)
		}
		sel, err := pr.Select(all)
		if err != nil {
			t.Fatalf("select %q: %v", spec, err)
		}
		if len(sel) != want {
			t.Errorf("range %q → %d páginas, esperaba %d", spec, len(sel), want)
		}
	}
}

// TestPageRangeFueraDeRango: pedir una página inexistente da error claro.
func TestPageRangeFueraDeRango(t *testing.T) {
	dir := fxDir(t)
	doc, _ := Parse(read(t, dir, "b.pdf")) // 2 páginas
	all, _ := doc.Pages()
	pr, _ := ParsePageRange("5")
	if _, err := pr.Select(all); err == nil {
		t.Error("esperaba error al pedir la página 5 de un PDF de 2")
	}
}
