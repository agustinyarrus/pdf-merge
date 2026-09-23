package pdf

// outline.go arma los marcadores (outline) del documento combinado:
//   - un marcador por archivo de entrada, que lleva a su primera página;
//   - debajo, los marcadores originales de ese archivo, con los destinos
//     remapeados a las páginas nuevas;
//   - los marcadores cuyo destino no entró en la selección se podan (salvo que
//     tengan hijos que sí), para no dejar entradas que no llevan a ningún lado.

// BookmarkMode elige qué marcadores lleva la salida.
type BookmarkMode uint8

const (
	BookmarksAuto  BookmarkMode = iota // uno por archivo + los originales anidados
	BookmarksFiles                     // solo uno por archivo
	BookmarksKeep                      // solo los originales, sin el nivel por archivo
	BookmarksNone                      // sin marcadores
)

// ParseBookmarkMode interpreta el valor de --bookmarks.
func ParseBookmarkMode(s string) (BookmarkMode, bool) {
	switch s {
	case "auto", "":
		return BookmarksAuto, true
	case "files", "archivos":
		return BookmarksFiles, true
	case "keep", "originales":
		return BookmarksKeep, true
	case "none", "ninguno":
		return BookmarksNone, true
	}
	return 0, false
}

// maxOpenOutline: un archivo con hasta estos marcadores propios arranca
// desplegado; con más, plegado (no inundar el panel al abrir).
const maxOpenOutline = 12

type outlineItem struct {
	title  Object
	dest   Object // array explícito ya copiado a la salida
	action Object // acción copiada (URI, etc.) cuando no hay destino
	color  Object
	flags  Object
	open   bool
	kids   []*outlineItem
}

// readOutline lee los marcadores del documento de origen, con los destinos ya
// copiados a la salida (página remapeada) por el copiador.
func (c *copier) readOutline() []*outlineItem {
	root, _ := c.doc.resolveDict(c.doc.trailer[Name("Root")])
	if root == nil {
		return nil
	}
	outlines, _ := c.doc.resolveDict(root[Name("Outlines")])
	if outlines == nil {
		return nil
	}
	return c.readSiblings(outlines[Name("First")], 0, map[int]bool{})
}

// readSiblings recorre una cadena /First → /Next. Defensivo contra ciclos y
// profundidades absurdas (outlines corruptos existen).
func (c *copier) readSiblings(first Object, depth int, seen map[int]bool) []*outlineItem {
	var items []*outlineItem
	for cur := first; cur != nil && depth <= 64; {
		ref, isRef := cur.(Ref)
		if !isRef || seen[ref.Num] {
			break
		}
		seen[ref.Num] = true
		node, err := c.doc.resolveDict(cur)
		if err != nil || node == nil {
			break
		}
		it := &outlineItem{}
		if t, err := c.doc.Resolve(node[Name("Title")]); err == nil {
			if s, ok := t.(String); ok {
				it.title = s
			}
		}
		switch {
		case node[Name("Dest")] != nil:
			if dest, ok := c.copyDest(node[Name("Dest")]); ok {
				it.dest = dest
			}
		case node[Name("A")] != nil:
			ad, _ := c.doc.resolveDict(node[Name("A")])
			if ad != nil && ad.GetName("S") == "GoTo" {
				if dest, ok := c.copyDest(ad[Name("D")]); ok {
					it.dest = dest
				}
			} else if ad != nil {
				if cp, err := c.copyValue(node[Name("A")]); err == nil {
					it.action = cp
				}
			}
		}
		it.color, _ = c.copyValue(node[Name("C")])
		it.flags, _ = c.copyValue(node[Name("F")])
		count, _ := intValue(node[Name("Count")])
		it.open = count > 0
		it.kids = c.readSiblings(node[Name("First")], depth+1, seen)
		items = append(items, it)
		cur = node[Name("Next")]
	}
	return pruneOutline(items)
}

// pruneOutline saca los marcadores sin destino, sin acción y sin hijos útiles.
func pruneOutline(items []*outlineItem) []*outlineItem {
	out := items[:0]
	for _, it := range items {
		if it.dest != nil || it.action != nil || len(it.kids) > 0 {
			if it.title == nil {
				it.title = String{Value: []byte("(sin título)")}
			}
			out = append(out, it)
		}
	}
	return out
}

// countOutline cuenta los marcadores de todo el árbol.
func countOutline(items []*outlineItem) int {
	n := len(items)
	for _, it := range items {
		n += countOutline(it.kids)
	}
	return n
}

// writeOutline escribe el árbol en el Builder y devuelve la referencia al
// diccionario /Outlines, o false si no hay nada que escribir.
func writeOutline(b *Builder, items []*outlineItem) (Ref, bool) {
	if len(items) == 0 {
		return Ref{}, false
	}
	root := b.Reserve()
	first, last, visible := writeOutlineLevel(b, items, root)
	b.Set(root, Dict{
		Name("Type"):  Name("Outlines"),
		Name("First"): first,
		Name("Last"):  last,
		Name("Count"): Integer(visible),
	})
	return Ref{Num: root}, true
}

// writeOutlineLevel escribe una lista de hermanos con sus enlaces /Prev /Next
// /Parent y devuelve el primero, el último y cuántos quedan visibles (para el
// /Count del padre).
func writeOutlineLevel(b *Builder, items []*outlineItem, parent int) (first, last Ref, visible int) {
	nums := make([]int, len(items))
	for i := range items {
		nums[i] = b.Reserve()
	}
	for i, it := range items {
		d := Dict{Name("Title"): it.title, Name("Parent"): Ref{Num: parent}}
		if i > 0 {
			d[Name("Prev")] = Ref{Num: nums[i-1]}
		}
		if i < len(items)-1 {
			d[Name("Next")] = Ref{Num: nums[i+1]}
		}
		if it.dest != nil {
			d[Name("Dest")] = it.dest
		} else if it.action != nil {
			d[Name("A")] = it.action
		}
		if it.color != nil && !isNull(it.color) {
			d[Name("C")] = it.color
		}
		if it.flags != nil && !isNull(it.flags) {
			d[Name("F")] = it.flags
		}
		visible++
		if len(it.kids) > 0 {
			f, l, kidsVisible := writeOutlineLevel(b, it.kids, nums[i])
			d[Name("First")], d[Name("Last")] = f, l
			// /Count: positivo = desplegado con esa cantidad de visibles;
			// negativo = plegado, con los que se verían al desplegarlo.
			if it.open {
				d[Name("Count")] = Integer(kidsVisible)
				visible += kidsVisible
			} else {
				d[Name("Count")] = Integer(-kidsVisible)
			}
		}
		b.Set(nums[i], d)
	}
	return Ref{Num: nums[0]}, Ref{Num: nums[len(nums)-1]}, visible
}

func isNull(o Object) bool {
	_, ok := o.(Null)
	return ok
}
