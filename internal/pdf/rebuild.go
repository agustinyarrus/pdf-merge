package pdf

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
)

// rebuild.go es la red de seguridad: si el xref está roto, corrido o falta, se
// barre el archivo entero buscando "N G obj" y se reconstruye la tabla; si el
// trailer no lleva a un catálogo, se busca uno que sí. Muchos PDF del mundo
// real tienen el xref corrupto pero los objetos intactos.

// objHeader captura "12 0 obj" al comienzo de línea o tras un delimitador.
var objHeader = regexp.MustCompile(`(?m)(?:^|[\s>])(\d+)\s+(\d+)\s+obj\b`)

// rebuildXref reconstruye la tabla recorriendo todo el archivo. La última
// definición de cada número gana (respeta las actualizaciones incrementales).
// O(tamaño del archivo), una sola pasada de la expresión regular.
func (d *Document) rebuildXref() error {
	matches := objHeader.FindAllSubmatchIndex(d.raw, -1)
	if len(matches) == 0 {
		return fmt.Errorf("no encontré objetos para reconstruir el xref")
	}
	d.xref = map[int]xrefEntry{}
	d.objStm = map[int]*objectStream{}
	for _, m := range matches {
		num, err := strconv.Atoi(string(d.raw[m[2]:m[3]]))
		if err != nil {
			continue
		}
		// El offset es donde empieza el número, no el match (que puede incluir
		// el espacio o el '>' previo).
		d.xref[num] = xrefEntry{offset: m[2]}
	}
	d.indexObjectStreams()
	return nil
}

// indexObjectStreams recorre los /Type /ObjStm hallados y registra sus objetos
// internos en el xref (sin pisar los que están directos en el archivo).
func (d *Document) indexObjectStreams() {
	nums := make([]int, 0, len(d.xref))
	for num, e := range d.xref {
		if !e.inStream {
			nums = append(nums, num)
		}
	}
	for _, num := range nums {
		obj, err := d.loadObject(num)
		if err != nil {
			continue
		}
		stm, ok := obj.(*Stream)
		if !ok || stm.Dict.GetName("Type") != "ObjStm" {
			continue
		}
		os, err := d.loadObjectStream(num)
		if err != nil {
			continue
		}
		for idx, inner := range os.order {
			if _, exists := d.xref[inner]; !exists {
				d.xref[inner] = xrefEntry{inStream: true, stmNum: num, stmIdx: idx}
			}
		}
	}
}

// rebuildTrailer busca un trailer que lleve a un catálogo, en este orden:
//  1. el último "trailer << … >>" del archivo (conserva /Encrypt, /Info, /ID);
//  2. el diccionario del xref stream más reciente (hace de trailer en PDF 1.5+);
//  3. el catálogo más reciente, a mano.
func (d *Document) rebuildTrailer() error {
	for idx := bytes.LastIndex(d.raw, []byte("trailer")); idx >= 0; idx = bytes.LastIndex(d.raw[:idx], []byte("trailer")) {
		s := newScanner(d.raw)
		s.pos = idx + len("trailer")
		if obj, err := s.parseObject(); err == nil {
			if tr, ok := obj.(Dict); ok && d.tryTrailer(tr) {
				return nil
			}
		}
	}
	if num, ok := d.latestObject(func(o Object) bool {
		stm, ok := o.(*Stream)
		return ok && stm.Dict.GetName("Type") == "XRef"
	}); ok {
		if obj, err := d.loadObject(num); err == nil && d.tryTrailer(obj.(*Stream).Dict) {
			return nil
		}
	}
	if num, ok := d.latestObject(func(o Object) bool {
		dict, ok := asDict(o)
		return ok && dict.GetName("Type") == "Catalog"
	}); ok {
		d.trailer = Dict{Name("Root"): Ref{Num: num, Gen: 0}}
		return nil
	}
	return fmt.Errorf("no encontré el catálogo del documento (/Type /Catalog)")
}

// tryTrailer adopta tr como trailer si lleva a un catálogo válido.
func (d *Document) tryTrailer(tr Dict) bool {
	prev := d.trailer
	d.trailer = tr
	if d.rootLooksValid() {
		return true
	}
	d.trailer = prev
	return false
}

// latestObject devuelve, entre los objetos directos que cumplen match, el que
// está más adelante en el archivo (la versión más nueva tras actualizaciones
// incrementales). Recorre el xref en orden de offset para ser determinista.
func (d *Document) latestObject(match func(Object) bool) (int, bool) {
	best, bestOff := 0, -1
	for num, e := range d.xref {
		if e.inStream || e.offset <= bestOff {
			continue
		}
		obj, err := d.loadObject(num)
		if err != nil || !match(obj) {
			continue
		}
		best, bestOff = num, e.offset
	}
	return best, bestOff >= 0
}

// asDict extrae el diccionario de un Dict o un Stream.
func asDict(o Object) (Dict, bool) {
	switch t := o.(type) {
	case Dict:
		return t, true
	case *Stream:
		return t.Dict, true
	}
	return nil, false
}
