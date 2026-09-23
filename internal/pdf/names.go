package pdf

// names.go resuelve destinos: un enlace o un marcador puede apuntar a una
// página con un array explícito ([página /XYZ x y zoom]) o con un NOMBRE que se
// busca en el catálogo (/Dests en PDF 1.1, o el árbol /Names /Dests desde 1.2).
//
// Al combinar, los nombres de dos archivos pueden chocar ("section.1" existe en
// los dos). En vez de fusionar árboles de nombres y renombrar, cada destino con
// nombre se resuelve acá y se escribe como array explícito: sin tabla de nombres
// en la salida, sin choques posibles.

const maxDestDepth = 8 // un /D que apunta a un nombre que apunta a un /D… acotado

// namedDests aplana (una vez, memoizado) todos los destinos con nombre del
// documento en un mapa nombre → destino. O(nombres).
func (d *Document) namedDests() map[string]Object {
	if d.dests != nil {
		return d.dests
	}
	d.dests = map[string]Object{}
	root, _ := d.resolveDict(d.trailer[Name("Root")])
	if root == nil {
		return d.dests
	}
	if old, _ := d.resolveDict(root[Name("Dests")]); old != nil {
		for k, v := range old {
			d.dests[string(k)] = v
		}
	}
	if names, _ := d.resolveDict(root[Name("Names")]); names != nil {
		d.walkNameTree(names[Name("Dests")], d.dests, 0, map[int]bool{})
	}
	return d.dests
}

// walkNameTree recorre un árbol de nombres (hojas con /Names [clave valor …],
// nodos internos con /Kids) cargando todas las entradas en out.
func (d *Document) walkNameTree(node Object, out map[string]Object, depth int, seen map[int]bool) {
	if depth > 32 {
		return
	}
	if ref, ok := node.(Ref); ok {
		if seen[ref.Num] {
			return
		}
		seen[ref.Num] = true
	}
	n, _ := d.resolveDict(node)
	if n == nil {
		return
	}
	if arr, ok := d.resolveArray(n[Name("Names")]); ok {
		for i := 0; i+1 < len(arr); i += 2 {
			if key, ok := d.keyString(arr[i]); ok {
				if _, exists := out[key]; !exists {
					out[key] = arr[i+1]
				}
			}
		}
	}
	if kids, ok := d.resolveArray(n[Name("Kids")]); ok {
		for _, k := range kids {
			d.walkNameTree(k, out, depth+1, seen)
		}
	}
}

func (d *Document) resolveArray(o Object) (Array, bool) {
	v, err := d.Resolve(o)
	if err != nil {
		return nil, false
	}
	arr, ok := v.(Array)
	return arr, ok
}

func (d *Document) keyString(o Object) (string, bool) {
	v, err := d.Resolve(o)
	if err != nil {
		return "", false
	}
	switch t := v.(type) {
	case String:
		return string(t.Value), true
	case Name:
		return string(t), true
	}
	return "", false
}

// explicitDest lleva cualquier forma de destino (array, nombre, cadena, o un
// diccionario con /D) a su array explícito [página vista …].
func (d *Document) explicitDest(dest Object) (Array, bool) {
	return d.explicitDestDepth(dest, 0)
}

func (d *Document) explicitDestDepth(dest Object, depth int) (Array, bool) {
	if depth > maxDestDepth {
		return nil, false
	}
	v, err := d.Resolve(dest)
	if err != nil || v == nil {
		return nil, false
	}
	switch t := v.(type) {
	case Array:
		return t, len(t) > 0
	case Name:
		return d.lookupDest(string(t), depth)
	case String:
		return d.lookupDest(string(t.Value), depth)
	case Dict:
		return d.explicitDestDepth(t[Name("D")], depth+1)
	}
	return nil, false
}

func (d *Document) lookupDest(name string, depth int) (Array, bool) {
	v, ok := d.namedDests()[name]
	if !ok {
		return nil, false
	}
	return d.explicitDestDepth(v, depth+1)
}

// textString codifica un texto de la interfaz (título de marcador) como cadena
// de texto PDF: ASCII tal cual; si hay acentos o eñes, UTF-16BE con BOM, que es
// la forma que todo lector entiende.
func textString(s string) String {
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii {
		return String{Value: []byte(s)}
	}
	out := []byte{0xFE, 0xFF}
	for _, r := range s {
		if r > 0xFFFF { // fuera del plano básico: par sustituto
			r -= 0x10000
			hi, lo := 0xD800+(r>>10), 0xDC00+(r&0x3FF)
			out = append(out, byte(hi>>8), byte(hi), byte(lo>>8), byte(lo))
			continue
		}
		out = append(out, byte(r>>8), byte(r))
	}
	return String{Value: out, Hex: true}
}
