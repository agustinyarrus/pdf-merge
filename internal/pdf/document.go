package pdf

import "fmt"

// document.go navega la estructura lógica del PDF: del catálogo al árbol de
// páginas, y de ahí a la lista de páginas en orden, resolviendo los atributos
// heredados (MediaBox, Resources, etc.) que un nodo página puede tomar de sus
// ancestros.

// atributos que una página hereda de los nodos Pages ancestros (tabla 30 del
// estándar). Al copiar una página los materializamos para no depender del árbol.
var inheritable = []Name{"MediaBox", "CropBox", "Resources", "Rotate"}

// Page es una página con su diccionario ya resuelto y sus atributos heredados
// incorporados. Num es su número de objeto original (0 si era un diccionario
// directo dentro de /Kids): con él se remapean las referencias a la página que
// hacen las anotaciones (/P) y los enlaces internos (/Dest).
type Page struct {
	Dict Dict
	Num  int
}

// Pages devuelve las páginas del documento en orden de lectura. El recorrido se
// hace una vez y se memoiza: el merge lo pide para seleccionar y para conocer
// el conjunto completo de páginas (barrera de referencias).
func (d *Document) Pages() ([]Page, error) {
	if d.pages != nil {
		return d.pages, nil
	}
	pages, err := d.walkAllPages()
	if err != nil {
		return nil, err
	}
	d.pages = pages
	return pages, nil
}

// RootNum es el número de objeto del catálogo (0 si era directo).
func (d *Document) RootNum() int {
	if r, ok := d.trailer[Name("Root")].(Ref); ok {
		return r.Num
	}
	return 0
}

func (d *Document) walkAllPages() ([]Page, error) {
	root, err := d.resolveDict(d.trailer[Name("Root")])
	if err != nil {
		return nil, fmt.Errorf("no pude leer el catálogo: %w", err)
	}
	if root == nil {
		return nil, fmt.Errorf("el documento no tiene catálogo")
	}
	pagesRoot, err := d.resolveDict(root[Name("Pages")])
	if err != nil {
		return nil, fmt.Errorf("no pude leer el árbol de páginas: %w", err)
	}
	if pagesRoot == nil {
		return nil, fmt.Errorf("el catálogo no apunta a un árbol de páginas")
	}
	var pages []Page
	seen := map[int]bool{}
	if err := d.walkPages(pagesRoot, 0, Dict{}, seen, &pages); err != nil {
		return nil, err
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("el documento no tiene páginas")
	}
	return pages, nil
}

// walkPages recorre el árbol en profundidad acumulando los atributos heredados.
// num es el número de objeto del nodo actual (0 si vino como diccionario directo).
func (d *Document) walkPages(node Dict, num int, inherited Dict, seen map[int]bool, out *[]Page) error {
	// Acumular los heredables presentes en este nodo para los descendientes.
	next := cloneShallow(inherited)
	for _, key := range inheritable {
		if v, ok := node[key]; ok {
			next[key] = v
		}
	}

	switch node.GetName("Type") {
	case "Pages", "":
		kids, err := d.Resolve(node[Name("Kids")])
		if err != nil {
			return err
		}
		arr, ok := kids.(Array)
		if !ok {
			// Un nodo sin Kids pero con Contents es en la práctica una página.
			if node[Name("Contents")] != nil || node.GetName("Type") == "Page" {
				return d.appendPage(node, num, next, out)
			}
			return fmt.Errorf("nodo Pages sin Kids")
		}
		for _, kid := range arr {
			childNum := 0
			if ref, isRef := kid.(Ref); isRef {
				if seen[ref.Num] {
					continue // ciclo defensivo
				}
				seen[ref.Num] = true
				childNum = ref.Num
			}
			child, err := d.resolveDict(kid)
			if err != nil {
				return err
			}
			if child == nil {
				continue
			}
			if err := d.walkPages(child, childNum, next, seen, out); err != nil {
				return err
			}
		}
		return nil
	default: // "Page"
		return d.appendPage(node, num, next, out)
	}
}

func (d *Document) appendPage(node Dict, num int, inherited Dict, out *[]Page) error {
	page := cloneShallow(node)
	for _, key := range inheritable {
		if _, ok := page[key]; !ok {
			if v, ok := inherited[key]; ok {
				page[key] = v
			}
		}
	}
	delete(page, Name("Parent")) // se reconstruye al escribir
	*out = append(*out, Page{Dict: page, Num: num})
	return nil
}

// cloneShallow copia el primer nivel de un diccionario (los valores se comparten).
func cloneShallow(d Dict) Dict {
	out := make(Dict, len(d))
	for k, v := range d {
		out[k] = v
	}
	return out
}

// PageCount cuenta las páginas sin materializarlas del todo (usa el /Count del
// catálogo si es confiable, con respaldo al recorrido).
func (d *Document) PageCount() (int, error) {
	pages, err := d.Pages()
	if err != nil {
		return 0, err
	}
	return len(pages), nil
}
