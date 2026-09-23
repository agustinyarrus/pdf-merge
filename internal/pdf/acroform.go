package pdf

import (
	"bytes"
	"fmt"
)

// acroform.go arma el formulario (/AcroForm) del documento combinado. Los
// widgets de los campos ya viajan con sus páginas (son anotaciones); lo que se
// reconstruye acá es el árbol de campos que los hace interactivos:
//   - entran solo los campos con algún widget en una página incluida, y de sus
//     /Kids se sacan los widgets de páginas que quedaron afuera;
//   - dos campos raíz de ARCHIVOS DISTINTOS con el mismo nombre se renombran
//     ("fecha" y "fecha_2"): con el mismo nombre completo, un lector los trata
//     como un único campo y sincroniza sus valores entre documentos;
//   - los recursos por defecto (/DR) se fusionan; /NeedAppearances y /SigFlags
//     se combinan; /XFA se descarta (no se puede fusionar) con aviso.

// sigFlagSignaturesExist y sigFlagAppendOnly son los bits de /SigFlags.
const (
	sigFlagSignaturesExist = 1
	sigFlagAppendOnly      = 2
)

// mergeAcroForms devuelve el /AcroForm de salida (nil si no hay campos) y los
// avisos para el user.
func mergeAcroForms(out *Builder, sources []Source, copiers map[*Document]*copier, pageNums []int) (Dict, []string) {
	alive := aliveWidgets(out, pageNums)
	if len(alive.order) == 0 {
		return nil, nil
	}

	// Subir desde cada widget vivo hasta su campo raíz, marcando los ancestros.
	var roots []int
	isRoot := map[int]bool{}
	for _, w := range alive.order {
		cur := w
		for depth := 0; depth < 64; depth++ {
			alive.set[cur] = true
			d, _ := out.objects[cur].(Dict)
			parent, hasParent := d[Name("Parent")].(Ref)
			if !hasParent || parent.Num <= 0 || parent.Num >= len(out.objects) {
				if (d[Name("T")] != nil || d[Name("FT")] != nil) && !isRoot[cur] {
					isRoot[cur] = true
					roots = append(roots, cur)
				}
				break
			}
			cur = parent.Num
		}
	}
	if len(roots) == 0 {
		return nil, nil
	}

	// Podar /Kids: fuera los widgets y subcampos que no llegaron a la salida.
	for num := range alive.set {
		d, ok := out.objects[num].(Dict)
		if !ok {
			continue
		}
		if kids, ok := d[Name("Kids")].(Array); ok {
			kept := kids[:0:0]
			for _, k := range kids {
				if r, ok := k.(Ref); ok && alive.set[r.Num] {
					kept = append(kept, k)
				}
			}
			d[Name("Kids")] = kept
		}
	}

	var warnings []string
	renamed := renameConflictingRoots(out, roots, rootOrigin(out, roots, copiers))
	if renamed > 0 {
		warnings = append(warnings, fmt.Sprintf("%s de formulario se renombraron para que no se mezclen entre archivos (sufijo _2, _3…)", countNoun(renamed, "campo", "campos")))
	}

	af := Dict{}
	fields := make(Array, len(roots))
	for i, r := range roots {
		fields[i] = Ref{Num: r}
	}
	af[Name("Fields")] = fields

	dr := Dict{}
	var sigFlags int
	need := false
	for si, src := range sources {
		srcAF := src.Doc.acroForm()
		if srcAF == nil {
			continue
		}
		c := copiers[src.Doc]
		if b, ok := srcAF[Name("NeedAppearances")].(Bool); ok && bool(b) {
			need = true
		}
		if f, ok := intValue(srcAF[Name("SigFlags")]); ok {
			sigFlags |= f
		}
		if da, ok := srcAF[Name("DA")]; ok && af[Name("DA")] == nil {
			if cp, err := c.copyValue(da); err == nil {
				af[Name("DA")] = cp
			}
		}
		if q, ok := srcAF[Name("Q")]; ok && af[Name("Q")] == nil {
			af[Name("Q")] = q
		}
		mergeDR(dr, srcAF[Name("DR")], c)
		if srcAF[Name("XFA")] != nil {
			warnings = append(warnings, fmt.Sprintf("el archivo %d tenía un formulario XFA: se conserva su versión AcroForm", si+1))
		}
	}
	if len(dr) > 0 {
		af[Name("DR")] = dr
	}
	if need {
		af[Name("NeedAppearances")] = Bool(true)
	}
	if sigFlags&sigFlagSignaturesExist != 0 {
		// Los campos de firma siguen ahí, pero el documento cambió: la firma
		// ya no valida, y "solo agregar" deja de tener sentido.
		af[Name("SigFlags")] = Integer(sigFlagSignaturesExist)
		warnings = append(warnings, "había firmas digitales: el documento combinado es otro archivo, así que esas firmas dejan de validar (se ven, pero no certifican)")
	}
	return af, warnings
}

// widgetSet son los widgets alcanzables desde las páginas de salida.
type widgetSet struct {
	order []int
	set   map[int]bool
}

func aliveWidgets(out *Builder, pageNums []int) widgetSet {
	ws := widgetSet{set: map[int]bool{}}
	for _, p := range pageNums {
		page, _ := out.objects[p].(Dict)
		annots, _ := page[Name("Annots")].(Array)
		for _, a := range annots {
			r, ok := a.(Ref)
			if !ok || r.Num <= 0 || r.Num >= len(out.objects) || ws.set[r.Num] {
				continue
			}
			if d, ok := out.objects[r.Num].(Dict); ok && d.GetName("Subtype") == "Widget" {
				ws.set[r.Num] = true
				ws.order = append(ws.order, r.Num)
			}
		}
	}
	return ws
}

// rootOrigin dice de qué copiador (documento) viene cada campo raíz, buscando
// su número en los mapas de remapeo.
func rootOrigin(out *Builder, roots []int, copiers map[*Document]*copier) map[int]*copier {
	owner := map[int]*copier{}
	want := map[int]bool{}
	for _, r := range roots {
		want[r] = true
	}
	for _, c := range copiers {
		for _, dst := range c.remap {
			if want[dst] {
				owner[dst] = c
			}
		}
	}
	return owner
}

// renameConflictingRoots agrega _2, _3… al nombre (/T) de un campo raíz cuando
// otro de un documento distinto ya usó ese nombre. Devuelve cuántos renombró.
func renameConflictingRoots(out *Builder, roots []int, owner map[int]*copier) int {
	type claim struct{ owner *copier }
	taken := map[string]claim{}
	renamed := 0
	for _, r := range roots {
		d := out.objects[r].(Dict)
		t, ok := d[Name("T")].(String)
		if !ok {
			continue
		}
		key := string(t.Value)
		prev, used := taken[key]
		if !used {
			taken[key] = claim{owner[r]}
			continue
		}
		if prev.owner == owner[r] {
			continue // mismo documento: su nombre ya era así en el original
		}
		for n := 2; ; n++ {
			candidate := withSuffix(t, fmt.Sprintf("_%d", n))
			if _, clash := taken[string(candidate.Value)]; !clash {
				d[Name("T")] = candidate
				taken[string(candidate.Value)] = claim{owner[r]}
				renamed++
				break
			}
		}
	}
	return renamed
}

// withSuffix agrega un sufijo ASCII a una cadena de texto PDF respetando su
// codificación (UTF-16BE con BOM, o PDFDocEncoding).
func withSuffix(s String, suffix string) String {
	v := append([]byte(nil), s.Value...)
	if bytes.HasPrefix(v, []byte{0xFE, 0xFF}) {
		for _, r := range suffix {
			v = append(v, 0, byte(r))
		}
		return String{Value: v, Hex: true}
	}
	return String{Value: append(v, suffix...), Hex: s.Hex}
}

// mergeDR fusiona un /DR de origen en el de salida: por categoría (/Font,
// /XObject…), la primera entrada con cada nombre gana.
func mergeDR(dst Dict, srcDR Object, c *copier) {
	src, _ := c.doc.resolveDict(srcDR)
	for cat, sub := range src {
		subDict, _ := c.doc.resolveDict(sub)
		if subDict == nil {
			continue
		}
		target, _ := dst[cat].(Dict)
		if target == nil {
			target = Dict{}
			dst[cat] = target
		}
		for name, val := range subDict {
			if _, exists := target[name]; exists {
				continue
			}
			if cp, err := c.copyValue(val); err == nil {
				target[name] = cp
			}
		}
	}
}

// acroForm devuelve el /AcroForm del catálogo, o nil.
func (d *Document) acroForm() Dict {
	root, _ := d.resolveDict(d.trailer[Name("Root")])
	if root == nil {
		return nil
	}
	af, _ := d.resolveDict(root[Name("AcroForm")])
	return af
}

func countNoun(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
