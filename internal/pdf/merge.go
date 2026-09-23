package pdf

import (
	"fmt"
	"io"
)

// merge.go combina varios PDF en uno. Por cada página se copia el cierre
// transitivo de los objetos que referencia (recursos, fuentes, imágenes,
// streams de contenido, anotaciones) al documento de salida, remapeando los
// números para que no choquen entre archivos. Después se arman el árbol de
// páginas, los marcadores y el catálogo.
//
// Barreras del recorrido (lo que NO se sigue al copiar):
//   - una página NO seleccionada del mismo documento → null (si no, un enlace o
//     una anotación arrastraría esa página entera como objeto huérfano);
//   - un nodo /Pages del árbol viejo y el catálogo de origen → null (se
//     reconstruyen; seguirlos copiaría el documento completo).
// Una página SÍ seleccionada que es referenciada (anotación /P, enlace /Dest)
// se remapea a su página nueva: los enlaces internos siguen funcionando. Los
// destinos con nombre se resuelven a destinos explícitos (ver names.go).

// Source es un PDF de entrada ya parseado, con las páginas a tomar.
type Source struct {
	Doc   *Document
	Pages []Page // páginas ya seleccionadas, en el orden deseado
	Title string // título del marcador de este archivo (normalmente, el nombre)
}

// MergeOptions ajusta la combinación.
type MergeOptions struct {
	Bookmarks BookmarkMode
}

// MergeResult resume lo que se combinó.
type MergeResult struct {
	TotalPages int
	Objects    int
	Bookmarks  int
	FormFields int
	Warnings   []string // cosas que el user tiene que saber (firmas, XFA, renombres)
}

// copier lleva el mapeo "objeto del origen" → "número en el destino" para UN
// documento fuente, de modo que cada objeto se copie una sola vez.
type copier struct {
	doc      *Document
	out      *Builder
	remap    map[int]int  // número en el origen → número en el destino
	pageNums map[int]bool // todas las páginas del documento (seleccionadas o no)
	rootNum  int          // catálogo de origen (barrera)
}

func newCopier(doc *Document, out *Builder) (*copier, error) {
	all, err := doc.Pages()
	if err != nil {
		return nil, err
	}
	c := &copier{doc: doc, out: out, remap: map[int]int{}, pageNums: map[int]bool{}, rootNum: doc.RootNum()}
	for _, p := range all {
		if p.Num != 0 {
			c.pageNums[p.Num] = true
		}
	}
	return c, nil
}

// Merge combina las fuentes en el Builder. Dos pasadas: primero se reservan los
// números de TODAS las páginas de salida (así una referencia hacia adelante,
// como un enlace de la página 1 a la 7, ya encuentra su destino), después se
// copia el contenido.
func Merge(out *Builder, sources []Source, opts MergeOptions) (MergeResult, error) {
	pagesRoot := out.Reserve() // el nodo raíz de páginas, se completa al final

	var plan []placedPage
	var pageRefs []Object
	firstPage := make([]placedPage, len(sources))

	// Un copiador por DOCUMENTO, no por entrada: si el mismo PDF aparece dos
	// veces con rangos distintos, sus fuentes e imágenes se copian una sola vez.
	copiers := map[*Document]*copier{}
	for si, src := range sources {
		c, ok := copiers[src.Doc]
		if !ok {
			var err error
			if c, err = newCopier(src.Doc, out); err != nil {
				return MergeResult{}, fmt.Errorf("archivo %d: %w", si+1, err)
			}
			copiers[src.Doc] = c
		}
		for pi, page := range src.Pages {
			n := out.Reserve()
			if page.Num != 0 {
				if _, dup := c.remap[page.Num]; !dup {
					// Si la misma página se pide dos veces, las referencias a la
					// original apuntan a su primera aparición.
					c.remap[page.Num] = n
				}
			}
			p := placedPage{c: c, page: page, newNum: n, src: si}
			if pi == 0 {
				firstPage[si] = p
			}
			plan = append(plan, p)
			pageRefs = append(pageRefs, Ref{Num: n, Gen: 0})
		}
	}

	for _, p := range plan {
		newPage := Dict{}
		for k, v := range p.page.Dict {
			if k == "Parent" {
				continue
			}
			copied, err := p.c.copyValue(v)
			if err != nil {
				return MergeResult{}, fmt.Errorf("archivo %d: %w", p.src+1, err)
			}
			newPage[k] = copied
		}
		newPage[Name("Type")] = Name("Page")
		newPage[Name("Parent")] = Ref{Num: pagesRoot, Gen: 0}
		out.Set(p.newNum, newPage)
	}

	out.Set(pagesRoot, Dict{
		Name("Type"):  Name("Pages"),
		Name("Kids"):  Array(pageRefs),
		Name("Count"): Integer(len(pageRefs)),
	})

	catalog := Dict{
		Name("Type"):  Name("Catalog"),
		Name("Pages"): Ref{Num: pagesRoot, Gen: 0},
	}
	res := MergeResult{TotalPages: len(pageRefs)}

	outline := buildOutline(sources, copiers, firstPage, opts)
	if ref, ok := writeOutline(out, outline); ok {
		catalog[Name("Outlines")] = ref
		catalog[Name("PageMode")] = Name("UseOutlines") // abrir con el panel de marcadores
		res.Bookmarks = countOutline(outline)
	}

	pageNums := make([]int, len(plan))
	for i, p := range plan {
		pageNums[i] = p.newNum
	}
	if af, warnings := mergeAcroForms(out, sources, copiers, pageNums); af != nil {
		catalog[Name("AcroForm")] = Ref{Num: out.Add(af)}
		res.FormFields = len(af[Name("Fields")].(Array))
		res.Warnings = append(res.Warnings, warnings...)
	}

	out.SetRoot(out.Add(catalog))
	res.Objects = len(out.objects) - 1
	return res, nil
}

// placedPage es una página ya ubicada en la salida: de qué copiador viene, su
// diccionario de origen y su número nuevo.
type placedPage struct {
	c      *copier
	page   Page
	newNum int
	src    int
}

// buildOutline arma la lista de marcadores de primer nivel según el modo.
func buildOutline(sources []Source, copiers map[*Document]*copier, firstPage []placedPage, opts MergeOptions) []*outlineItem {
	if opts.Bookmarks == BookmarksNone {
		return nil
	}
	perFile := opts.Bookmarks == BookmarksAuto || opts.Bookmarks == BookmarksFiles
	if opts.Bookmarks == BookmarksAuto && len(sources) == 1 {
		perFile = false // un solo archivo: el nivel "por archivo" no aporta nada
	}
	keepOriginals := opts.Bookmarks == BookmarksAuto || opts.Bookmarks == BookmarksKeep

	var items []*outlineItem
	for si, src := range sources {
		fp := firstPage[si]
		if fp.newNum == 0 {
			continue // la selección de este archivo quedó vacía
		}
		var kids []*outlineItem
		if keepOriginals {
			kids = copiers[src.Doc].readOutline()
		}
		if !perFile {
			items = append(items, kids...)
			continue
		}
		title := src.Title
		if title == "" {
			title = fmt.Sprintf("Archivo %d", si+1)
		}
		items = append(items, &outlineItem{
			title: textString(title),
			dest:  fp.c.pageTopDest(fp.newNum, fp.page),
			open:  countOutline(kids) <= maxOpenOutline,
			kids:  kids,
		})
	}
	return items
}

// pageTopDest arma un destino al borde superior de la página, sin tocar el
// zoom del lector: [página /XYZ null tope null]. Si la página no declara su
// caja, /Fit.
func (c *copier) pageTopDest(num int, page Page) Array {
	for _, key := range []Name{"CropBox", "MediaBox"} {
		box, err := c.doc.Resolve(page.Dict[key])
		if err != nil {
			continue
		}
		if arr, ok := box.(Array); ok && len(arr) == 4 {
			top, err := c.doc.Resolve(arr[3])
			if err != nil {
				continue
			}
			switch top.(type) {
			case Integer, Real:
				return Array{Ref{Num: num}, Name("XYZ"), Null{}, top, Null{}}
			}
		}
	}
	return Array{Ref{Num: num}, Name("Fit")}
}

// copyDest copia un destino (en cualquiera de sus formas) como array explícito
// con la página remapeada. Devuelve false si la página no quedó en la salida o
// si el destino no se puede resolver: el que llama omite el destino y el
// enlace queda inerte en vez de apuntar a la nada.
func (c *copier) copyDest(dest Object) (Object, bool) {
	arr, ok := c.doc.explicitDest(dest)
	if !ok || len(arr) == 0 {
		return nil, false
	}
	target := arr[0]
	// Algunos generadores ponen el índice de página (entero) en vez de la
	// referencia: se traduce con la lista de páginas del origen.
	if idx, isInt := target.(Integer); isInt {
		pages, err := c.doc.Pages()
		if err != nil || idx < 0 || int(idx) >= len(pages) || pages[idx].Num == 0 {
			return nil, false
		}
		target = Ref{Num: pages[idx].Num}
	}
	ref, isRef := target.(Ref)
	if !isRef || !c.pageNums[ref.Num] {
		return nil, false
	}
	newNum, selected := c.remap[ref.Num]
	if !selected {
		return nil, false
	}
	out := Array{Ref{Num: newNum}}
	for _, v := range arr[1:] {
		cp, err := c.copyValue(v)
		if err != nil {
			return nil, false
		}
		out = append(out, cp)
	}
	return out, true
}

// copyValue copia un objeto al documento de salida, siguiendo referencias y
// copiando cada objeto referenciado una sola vez (memoizado en remap). Es un
// recorrido en profundidad del grafo de objetos: O(objetos alcanzables).
func (c *copier) copyValue(o Object) (Object, error) {
	switch v := o.(type) {
	case Ref:
		return c.copyRef(v)
	case Array:
		out := make(Array, len(v))
		for i, e := range v {
			cp, err := c.copyValue(e)
			if err != nil {
				return nil, err
			}
			out[i] = cp
		}
		return out, nil
	case Dict:
		return c.copyDict(v)
	case *Stream:
		// /Length no se copia (si era indirecto arrastraría un objeto suelto):
		// el escritor la pone siempre con el largo real de los bytes.
		src := cloneShallow(v.Dict)
		delete(src, Name("Length"))
		nd, err := c.copyDict(src)
		if err != nil {
			return nil, err
		}
		return &Stream{Dict: nd, Raw: c.doc.streamData(v)}, nil
	default:
		return o, nil // escalares: se copian por valor
	}
}

// copyDict copia un diccionario. Los destinos de los enlaces (/Dest de un
// /Link) y de las acciones GoTo (/D) pasan por copyDest: nombres resueltos,
// páginas remapeadas, y omitidos si su página no quedó en la salida.
func (c *copier) copyDict(d Dict) (Dict, error) {
	out := make(Dict, len(d))
	goTo := d.GetName("S") == "GoTo"
	link := d.GetName("Subtype") == "Link"
	for k, v := range d {
		if (goTo && k == "D") || (link && k == "Dest") {
			if dest, ok := c.copyDest(v); ok {
				out[k] = dest
			}
			continue
		}
		cp, err := c.copyValue(v)
		if err != nil {
			return nil, err
		}
		out[k] = cp
	}
	return out, nil
}

// copyRef copia el objeto apuntado por una referencia (una sola vez) y devuelve
// la referencia nueva. Rompe los ciclos reservando el número ANTES de copiar el
// contenido (una anotación que apunta a su página, que apunta a la anotación).
func (c *copier) copyRef(ref Ref) (Object, error) {
	if newNum, ok := c.remap[ref.Num]; ok {
		return Ref{Num: newNum, Gen: 0}, nil
	}
	if c.pageNums[ref.Num] || ref.Num == c.rootNum {
		return Null{}, nil // página no seleccionada o catálogo de origen: barrera
	}
	target, err := c.doc.getObject(ref.Num)
	if err != nil {
		return nil, err
	}
	if dict, ok := asDict(target); ok && dict.GetName("Type") == "Pages" {
		return Null{}, nil // nodo del árbol viejo: barrera
	}

	newNum := c.out.Reserve()
	c.remap[ref.Num] = newNum
	copied, err := c.copyValue(target)
	if err != nil {
		return nil, err
	}
	c.out.Set(newNum, copied)
	return Ref{Num: newNum, Gen: 0}, nil
}

// MergeFiles es la función de alto nivel: parsea cada archivo, selecciona sus
// páginas y escribe el resultado.
func MergeFiles(inputs [][]byte, ranges []*PageRange, w io.Writer) (MergeResult, error) {
	sources := make([]Source, 0, len(inputs))
	for i, raw := range inputs {
		doc, err := Parse(raw)
		if err != nil {
			return MergeResult{}, fmt.Errorf("archivo %d: %w", i+1, err)
		}
		all, err := doc.Pages()
		if err != nil {
			return MergeResult{}, fmt.Errorf("archivo %d: %w", i+1, err)
		}
		sel := all
		if i < len(ranges) && ranges[i] != nil {
			sel, err = ranges[i].Select(all)
			if err != nil {
				return MergeResult{}, fmt.Errorf("archivo %d: %w", i+1, err)
			}
		}
		sources = append(sources, Source{Doc: doc, Pages: sel})
	}
	out := NewBuilder()
	res, err := Merge(out, sources, MergeOptions{})
	if err != nil {
		return res, err
	}
	if _, err := out.WriteTo(w); err != nil {
		return res, err
	}
	return res, nil
}
