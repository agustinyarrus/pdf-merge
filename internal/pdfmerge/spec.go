package pdfmerge

import (
	"fmt"
	"os"
	"strings"

	"github.com/agustinyarrus/pdf-merge/internal/pdf"
)

// spec.go interpreta cada argumento de entrada: un patrón (archivo, carpeta o
// comodín) con una selección de páginas opcional pegada con '@'.

// inputSpec es un argumento ya separado en patrón + selección.
type inputSpec struct {
	pattern string
	pages   *pdf.PageRange // nil = todas
	raw     string         // selección tal como la escribió el user ("" = todas)
}

// parseSpec separa "informe.pdf@1-3,5" en patrón y selección. Si el argumento
// completo existe como archivo, NO se parte: un nombre como "factura@2026.pdf"
// es un archivo, no una selección. Se parte en la ÚLTIMA '@' y solo si lo que
// sigue es una selección válida.
func parseSpec(arg string) (inputSpec, error) {
	if _, err := os.Stat(arg); err == nil {
		return inputSpec{pattern: arg}, nil
	}
	at := strings.LastIndex(arg, "@")
	if at <= 0 || at == len(arg)-1 {
		return inputSpec{pattern: arg}, nil
	}
	pattern, sel := arg[:at], arg[at+1:]
	pr, err := pdf.ParsePageRange(sel)
	if err != nil {
		// Lo que sigue a la '@' no es una selección: se toma el argumento
		// entero como patrón y el error aparecerá como "no existe".
		return inputSpec{pattern: arg}, nil
	}
	if !strings.HasSuffix(strings.ToLower(pattern), ".pdf") && !strings.ContainsAny(pattern, "*?[") {
		if st, err := os.Stat(pattern); err != nil || st.IsDir() {
			return inputSpec{}, fmt.Errorf("%q: la selección de páginas va pegada a un .pdf (ej.: informe.pdf@1-3)", arg)
		}
	}
	return inputSpec{pattern: pattern, pages: pr, raw: sel}, nil
}
