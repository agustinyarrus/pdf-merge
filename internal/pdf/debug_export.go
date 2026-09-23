package pdf

import (
	"fmt"
	"sort"
	"strings"
)

// Debug devuelve un resumen legible del estado interno de un documento parseado.
// Es para diagnóstico durante el desarrollo; no forma parte de la API estable.
func Debug(d *Document) string {
	var b strings.Builder
	fmt.Fprintf(&b, "xref: %d entradas\n", len(d.xref))
	nums := make([]int, 0, len(d.xref))
	for k := range d.xref {
		nums = append(nums, k)
	}
	sort.Ints(nums)
	shown := nums
	if len(shown) > 20 {
		shown = shown[:20]
	}
	for _, n := range shown {
		e := d.xref[n]
		if e.inStream {
			fmt.Fprintf(&b, "  %d: en objstm %d idx %d\n", n, e.stmNum, e.stmIdx)
		} else {
			fmt.Fprintf(&b, "  %d: offset %d\n", n, e.offset)
		}
	}
	fmt.Fprintf(&b, "trailer keys: ")
	if d.trailer == nil {
		b.WriteString("(nil)\n")
	} else {
		for _, k := range d.trailer.sortedKeys() {
			fmt.Fprintf(&b, "%s=%v ", k, d.trailer[k])
		}
		b.WriteString("\n")
	}
	if root := d.trailer[Name("Root")]; root != nil {
		rd, err := d.resolveDict(root)
		fmt.Fprintf(&b, "Root resuelto: %v (err %v)\n", rd != nil, err)
		if rd != nil {
			fmt.Fprintf(&b, "  Root/Type=%s Pages=%v\n", rd.GetName("Type"), rd[Name("Pages")])
		}
	}
	return b.String()
}
