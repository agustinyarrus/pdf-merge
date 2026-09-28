package pdfmerge

import (
	"strings"
	"testing"

	"github.com/agustinyarrus/pdf-merge/internal/pdf"
	"github.com/agustinyarrus/pdf-merge/internal/tui"
)

// Un aviso largo se parte en palabras al ancho de la ventana, con las líneas
// de más alineadas después del "!". Antes iba en una sola línea y, a 100
// columnas, la consola lo cortaba en mitad de una palabra ("no las con" /
// "serva").
func TestAvisoSeParteEnPalabras(t *testing.T) {
	term := &tui.Term{}
	term.SetWidth(100)
	w := "1 archivo tenía restricciones del propietario (imprimir, copiar, editar); la salida no las conserva"
	lineas := aviso(term, w)
	if len(lineas) != 2 {
		t.Fatalf("esperaba dos líneas, salieron %d: %q", len(lineas), lineas)
	}
	if !strings.HasPrefix(lineas[0], "    ! 1 archivo") {
		t.Errorf("primera línea: %q", lineas[0])
	}
	if !strings.HasPrefix(lineas[1], "      ") || strings.HasPrefix(lineas[1], "       ") {
		t.Errorf("la segunda línea tiene que arrancar alineada después del \"! \": %q", lineas[1])
	}
	var palabras []string
	for _, l := range lineas {
		if n := tui.Width(l); n > 100-len(tui.Margin) {
			t.Errorf("línea de %d columnas (el tope es %d): %q", n, 100-len(tui.Margin), l)
		}
		palabras = append(palabras, strings.Fields(strings.TrimPrefix(strings.TrimSpace(l), "! "))...)
	}
	if got := strings.Join(palabras, " "); got != w {
		t.Errorf("el aviso cambió al partirse:\n%q\n%q", got, w)
	}

	// Uno corto queda en una línea.
	if l := aviso(term, "la salida no va cifrada"); len(l) != 1 {
		t.Errorf("un aviso corto se partió: %q", l)
	}
}

// Los objetos sin stream (un diccionario de fuente) se fusionan sin ahorrar
// bytes de stream: la tarjeta dice los objetos solos, no "−0 B".
func TestRepetidosSinBytesNoDiceCero(t *testing.T) {
	if got := repetidos(pdf.DedupeStats{Merged: 3}); got != "3 objetos" {
		t.Errorf("sin bytes ahorrados: %q", got)
	}
	if got := repetidos(pdf.DedupeStats{Merged: 1}); got != "1 objeto" {
		t.Errorf("uno solo: %q", got)
	}
	got := repetidos(pdf.DedupeStats{Merged: 16, BytesSaved: 554_000})
	if !strings.HasPrefix(got, "16 objetos · −") || strings.Contains(got, "−0 B") {
		t.Errorf("con bytes ahorrados: %q", got)
	}
}
