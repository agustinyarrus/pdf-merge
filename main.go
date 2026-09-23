// pdf-merge combina varios PDF en uno, local, con selección de páginas.
package main

import (
	"os"

	"github.com/agustinyarrus/pdf-merge/internal/pdfmerge"
	"github.com/agustinyarrus/pdf-merge/internal/tui"
	"github.com/agustinyarrus/pdf-merge/internal/version"
)

func main() {
	t := tui.Open()
	defer t.Close()
	os.Exit(pdfmerge.Main(t, version.String(), os.Args[1:]))
}
