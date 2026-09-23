package pdf

import (
	"crypto/sha256"
	"encoding/binary"
	"hash"
	"math"
	"sort"
)

// dedupe.go fusiona los objetos IDÉNTICOS del documento de salida: la misma
// imagen o la misma fuente embebida en diez PDF se escribe una sola vez. Es una
// pasada separada sobre el Builder, después del merge.
//
// El método es hashing de Merkle sobre el grafo de objetos:
//   - el hash de un objeto se calcula sobre su forma canónica, donde cada
//     referencia se reemplaza por el hash del objeto apuntado; así dos fuentes
//     iguales en archivos distintos, con numeración distinta, dan el mismo hash;
//   - para que los hijos estén listos antes que los padres, se recorren las
//     componentes fuertemente conexas de Tarjan, que salen en orden topológico
//     inverso;
//   - un objeto dentro de un ciclo (una página y su /Parent, una anotación y su
//     /P) o con identidad propia (páginas, anotaciones, campos, capas) recibe un
//     hash único y nunca se fusiona.
// Al final se marca lo alcanzable desde el catálogo y se renumera compacto.
//
// Complejidad: O(V + E) para el grafo y Tarjan, O(bytes) para el hashing
// (SHA-256 con instrucciones SHA-NI en el procesador).

// DedupeStats resume lo que ahorró la deduplicación.
type DedupeStats struct {
	Merged     int   // objetos eliminados por ser copia exacta de otro
	BytesSaved int64 // bytes de streams que ya no se escriben
	Kept       int   // objetos finales
}

type digest = [sha256.Size]byte

// Dedupe fusiona los objetos idénticos y compacta la numeración.
func (b *Builder) Dedupe() DedupeStats {
	n := len(b.objects)
	adj := make([][]int, n)
	identity := make([]bool, n)
	for num := 1; num < n; num++ {
		adj[num] = collectRefs(b.objects[num], n)
		identity[num] = hasIdentity(b.objects[num])
	}

	hashes := make([]digest, n)
	h := sha256.New()
	for _, comp := range tarjanSCC(adj, n) {
		unique := len(comp) > 1 || identity[comp[0]] || selfLoop(adj, comp[0])
		for _, v := range comp {
			if unique {
				hashes[v] = uniqueDigest(v)
				continue
			}
			h.Reset()
			writeCanonical(h, b.objects[v], hashes)
			copy(hashes[v][:], h.Sum(nil))
		}
	}

	// Representante de cada hash: el de menor número (el primero en aparecer),
	// así el orden de salida sigue el de las páginas.
	canon := make([]int, n)
	first := make(map[digest]int, n)
	for num := 1; num < n; num++ {
		if rep, ok := first[hashes[num]]; ok {
			canon[num] = rep
		} else {
			first[hashes[num]] = num
			canon[num] = num
		}
	}
	return b.compact(adj, canon)
}

// Collect es la recolección de basura sin deduplicar: descarta lo que quedó
// inalcanzable desde el catálogo (por ejemplo, los widgets de un campo cuyas
// páginas no entraron en la selección) y renumera compacto. O(V + E).
func (b *Builder) Collect() int {
	n := len(b.objects)
	adj := make([][]int, n)
	canon := make([]int, n)
	for num := 1; num < n; num++ {
		adj[num] = collectRefs(b.objects[num], n)
		canon[num] = num
	}
	before := n - 1
	st := b.compact(adj, canon)
	return before - st.Kept
}

// compact marca lo alcanzable desde el catálogo (siguiendo ya las referencias
// canónicas), renumera 1..M en el orden original y reescribe las referencias.
func (b *Builder) compact(adj [][]int, canon []int) DedupeStats {
	n := len(b.objects)
	keep := make([]bool, n)
	stack := []int{canon[b.root]}
	for len(stack) > 0 {
		v := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if keep[v] {
			continue
		}
		keep[v] = true
		for _, w := range adj[v] {
			if c := canon[w]; !keep[c] {
				stack = append(stack, c)
			}
		}
	}

	var stats DedupeStats
	newNum := make([]int, n)
	next := 1
	for num := 1; num < n; num++ {
		if keep[num] {
			newNum[num] = next
			next++
			continue
		}
		if canon[num] != num {
			stats.Merged++
			if stm, ok := b.objects[num].(*Stream); ok {
				stats.BytesSaved += int64(len(stm.Raw))
			}
		}
	}
	remap := func(num int) int {
		if num <= 0 || num >= n {
			return 0
		}
		return newNum[canon[num]]
	}
	out := make([]Object, next)
	for num := 1; num < n; num++ {
		if keep[num] {
			out[newNum[num]] = rewriteRefs(b.objects[num], remap)
		}
	}
	b.objects = out
	b.root = newNum[canon[b.root]]
	stats.Kept = next - 1
	return stats
}

// collectRefs junta los números de objeto que referencia o (sin repetir).
func collectRefs(o Object, n int) []int {
	var out []int
	seen := map[int]bool{}
	var walk func(Object)
	walk = func(o Object) {
		switch v := o.(type) {
		case Ref:
			if v.Num > 0 && v.Num < n && !seen[v.Num] {
				seen[v.Num] = true
				out = append(out, v.Num)
			}
		case Array:
			for _, e := range v {
				walk(e)
			}
		case Dict:
			for _, e := range v {
				walk(e)
			}
		case *Stream:
			for _, e := range v.Dict {
				walk(e)
			}
		}
	}
	walk(o)
	sort.Ints(out) // orden estable: Tarjan y el recorrido dan siempre lo mismo
	return out
}

// hasIdentity marca los objetos que son "alguien" y no "algo": dos páginas o
// dos anotaciones iguales siguen siendo dos, y fusionarlas rompería el
// documento (una anotación pertenece a UNA página; una capa se prende y apaga
// por separado; un campo de formulario tiene su propio valor).
func hasIdentity(o Object) bool {
	d, ok := asDict(o)
	if !ok {
		return false
	}
	switch d.GetName("Type") {
	case "Page", "Pages", "Catalog", "Annot", "OCG", "OCMD", "Outlines", "StructTreeRoot", "StructElem", "Sig":
		return true
	}
	if d[Name("Rect")] != nil && d[Name("Subtype")] != nil {
		return true // anotación escrita sin /Type
	}
	if d[Name("FT")] != nil {
		return true // campo de formulario
	}
	if d[Name("T")] != nil && (d[Name("Kids")] != nil || d[Name("Parent")] != nil) {
		return true // nodo del árbol de campos
	}
	return false
}

func selfLoop(adj [][]int, v int) bool {
	for _, w := range adj[v] {
		if w == v {
			return true
		}
	}
	return false
}

// uniqueDigest es un hash que ningún contenido puede producir (prefijo propio).
func uniqueDigest(num int) digest {
	var d digest
	copy(d[:], "\x00navaja-identidad\x00")
	binary.BigEndian.PutUint64(d[sha256.Size-8:], uint64(num))
	return d
}

// writeCanonical escribe una codificación sin ambigüedad del objeto: cada valor
// lleva una etiqueta de tipo y los largos van prefijados; los diccionarios se
// recorren con las claves ordenadas; las referencias se reemplazan por el hash
// del objeto apuntado. /Length no entra (el escritor la recalcula).
func writeCanonical(h hash.Hash, o Object, hashes []digest) {
	var buf [9]byte
	u64 := func(tag byte, v uint64) {
		buf[0] = tag
		binary.BigEndian.PutUint64(buf[1:], v)
		h.Write(buf[:])
	}
	bytesTagged := func(tag byte, b []byte) {
		u64(tag, uint64(len(b)))
		h.Write(b)
	}
	switch v := o.(type) {
	case nil, Null:
		h.Write([]byte{'n'})
	case Bool:
		if v {
			h.Write([]byte{'t'})
		} else {
			h.Write([]byte{'f'})
		}
	case Integer:
		u64('i', uint64(v))
	case Real:
		u64('r', math.Float64bits(float64(v)))
	case Name:
		bytesTagged('N', []byte(v))
	case String:
		bytesTagged('S', v.Value)
	case Ref:
		if v.Num > 0 && v.Num < len(hashes) {
			h.Write([]byte{'R'})
			h.Write(hashes[v.Num][:])
		} else {
			h.Write([]byte{'n'})
		}
	case Array:
		u64('A', uint64(len(v)))
		for _, e := range v {
			writeCanonical(h, e, hashes)
		}
	case Dict:
		writeCanonicalDict(h, v, hashes, u64, bytesTagged)
	case *Stream:
		h.Write([]byte{'T'})
		writeCanonicalDict(h, v.Dict, hashes, u64, bytesTagged)
		bytesTagged('B', v.Raw)
	}
}

func writeCanonicalDict(h hash.Hash, d Dict, hashes []digest, u64 func(byte, uint64), bytesTagged func(byte, []byte)) {
	keys := d.sortedKeys()
	count := 0
	for _, k := range keys {
		if k != "Length" {
			count++
		}
	}
	u64('D', uint64(count))
	for _, k := range keys {
		if k == "Length" {
			continue
		}
		bytesTagged('K', []byte(k))
		writeCanonical(h, d[k], hashes)
	}
}

// rewriteRefs copia el valor reemplazando cada referencia por su número nuevo
// (0 = el objeto desapareció: se escribe null, como manda el estándar).
func rewriteRefs(o Object, remap func(int) int) Object {
	switch v := o.(type) {
	case Ref:
		if n := remap(v.Num); n > 0 {
			return Ref{Num: n, Gen: 0}
		}
		return Null{}
	case Array:
		out := make(Array, len(v))
		for i, e := range v {
			out[i] = rewriteRefs(e, remap)
		}
		return out
	case Dict:
		out := make(Dict, len(v))
		for k, e := range v {
			out[k] = rewriteRefs(e, remap)
		}
		return out
	case *Stream:
		nd := rewriteRefs(v.Dict, remap).(Dict)
		return &Stream{Dict: nd, Raw: v.Raw}
	}
	return o
}

// tarjanSCC devuelve las componentes fuertemente conexas en orden topológico
// inverso (primero las que no apuntan a nadie). Versión iterativa: un PDF con
// cientos de miles de objetos encadenados no debe depender de la profundidad
// de la pila de llamadas.
func tarjanSCC(adj [][]int, n int) [][]int {
	index := make([]int, n)
	low := make([]int, n)
	onStack := make([]bool, n)
	for i := range index {
		index[i] = -1
	}
	var stack []int
	var sccs [][]int
	next := 0
	type frame struct{ v, edge int }

	for root := 1; root < n; root++ {
		if index[root] != -1 {
			continue
		}
		index[root], low[root] = next, next
		next++
		stack = append(stack, root)
		onStack[root] = true
		call := []frame{{v: root}}

		for len(call) > 0 {
			f := &call[len(call)-1]
			v := f.v
			if f.edge < len(adj[v]) {
				w := adj[v][f.edge]
				f.edge++
				switch {
				case index[w] == -1:
					index[w], low[w] = next, next
					next++
					stack = append(stack, w)
					onStack[w] = true
					call = append(call, frame{v: w})
				case onStack[w]:
					low[v] = min(low[v], index[w])
				}
				continue
			}
			if low[v] == index[v] {
				var comp []int
				for {
					w := stack[len(stack)-1]
					stack = stack[:len(stack)-1]
					onStack[w] = false
					comp = append(comp, w)
					if w == v {
						break
					}
				}
				sccs = append(sccs, comp)
			}
			call = call[:len(call)-1]
			if len(call) > 0 {
				u := call[len(call)-1].v
				low[u] = min(low[u], low[v])
			}
		}
	}
	return sccs
}
