package pdf

import "fmt"

// xrefstream.go entiende las tablas de referencias cruzadas en formato stream
// (PDF 1.5+): un objeto con /Type /XRef cuyo contenido binario, gobernado por
// /W [a b c], describe cada entrada. También expande los object streams, donde
// varios objetos viajan comprimidos juntos.

// readXRefStream lee un xref en forma de objeto stream ubicado en offset.
func (d *Document) readXRefStream(offset int) (Dict, int, int, error) {
	_, _, obj, err := d.parseIndirectAt(offset)
	if err != nil {
		return nil, 0, 0, err
	}
	stm, ok := obj.(*Stream)
	if !ok {
		return nil, 0, 0, fmt.Errorf("en %d esperaba un xref stream", offset)
	}
	data, err := d.decodeStream(stm)
	if err != nil {
		return nil, 0, 0, err
	}
	w, ok := stm.Dict[Name("W")].(Array)
	if !ok || len(w) != 3 {
		return nil, 0, 0, fmt.Errorf("xref stream sin /W válido")
	}
	w0, _ := intValue(w[0])
	w1, _ := intValue(w[1])
	w2, _ := intValue(w[2])
	rowLen := w0 + w1 + w2
	if rowLen == 0 {
		return nil, 0, 0, fmt.Errorf("xref stream con /W en cero")
	}

	size, _ := intValue(stm.Dict[Name("Size")])
	index := d.xrefIndex(stm.Dict, size)

	pos := 0
	read := func(n int) int64 { // los campos son enteros big-endian de n bytes
		var v int64
		for i := 0; i < n; i++ {
			v = v<<8 | int64(data[pos])
			pos++
		}
		return v
	}
	for s := 0; s+1 < len(index); s += 2 {
		startObj, count := index[s], index[s+1]
		for i := 0; i < count; i++ {
			if pos+rowLen > len(data) {
				break
			}
			f0 := int64(1) // tipo por defecto = 1 cuando /W[0]=0
			if w0 > 0 {
				f0 = read(w0)
			}
			f1 := read(w1)
			f2 := read(w2)
			num := startObj + i
			if _, exists := d.xref[num]; exists {
				continue // ya hay una versión más nueva
			}
			switch f0 {
			case 1:
				d.xref[num] = xrefEntry{offset: int(f1)}
			case 2:
				d.xref[num] = xrefEntry{inStream: true, stmNum: int(f1), stmIdx: int(f2)}
			}
		}
	}

	prev, xrefStm := -1, -1
	if p, ok := intValue(stm.Dict[Name("Prev")]); ok {
		prev = p
	}
	return stm.Dict, prev, xrefStm, nil
}

// xrefIndex devuelve el /Index del xref stream, o [0 Size] si no está.
func (d *Document) xrefIndex(dict Dict, size int) []int {
	if idx, ok := dict[Name("Index")].(Array); ok {
		out := make([]int, 0, len(idx))
		for _, o := range idx {
			if v, ok := intValue(o); ok {
				out = append(out, v)
			}
		}
		if len(out)%2 == 0 && len(out) > 0 {
			return out
		}
	}
	return []int{0, size}
}

// objectStream es un /Type /ObjStm ya expandido: los objetos que contiene y sus
// offsets internos.
type objectStream struct {
	data    []byte
	offsets map[int]int // número de objeto → offset dentro de data
	first   int
	order   []int
}

// loadObjectStream expande (una vez) el object stream número stmNum.
func (d *Document) loadObjectStream(stmNum int) (*objectStream, error) {
	if os, ok := d.objStm[stmNum]; ok {
		return os, nil
	}
	obj, err := d.getObject(stmNum)
	if err != nil {
		return nil, err
	}
	stm, ok := obj.(*Stream)
	if !ok {
		return nil, fmt.Errorf("objeto %d no es un object stream", stmNum)
	}
	data, err := d.decodeStream(stm)
	if err != nil {
		return nil, err
	}
	n, _ := intValue(stm.Dict[Name("N")])
	first, _ := intValue(stm.Dict[Name("First")])
	os := &objectStream{data: data, first: first, offsets: map[int]int{}}

	// El encabezado son N pares "objNum offset".
	hs := newScanner(data)
	for i := 0; i < n; i++ {
		num, ok1 := hs.readInt()
		off, ok2 := hs.readInt()
		if !ok1 || !ok2 {
			return nil, fmt.Errorf("encabezado de object stream %d corrupto", stmNum)
		}
		os.offsets[num] = first + off
		os.order = append(os.order, num)
	}
	d.objStm[stmNum] = os
	return os, nil
}
