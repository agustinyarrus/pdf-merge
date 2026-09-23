package pdf

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
)

// resolve.go obtiene objetos por número (desde un offset del archivo o desde un
// object stream), sigue referencias indirectas y decodifica el contenido de los
// streams cuando hace falta leerlos (xref y object streams van con FlateDecode).

// getObject devuelve el objeto num, cacheado. Un objeto ausente es Null (como
// manda el estándar: una referencia a un objeto inexistente vale null).
//
// Si el xref apunta a un lugar donde no está ESE objeto (offsets corridos por
// basura antes del encabezado, o una tabla mal escrita), se reconstruye la
// tabla barriendo el archivo, una sola vez, y se reintenta.
func (d *Document) getObject(num int) (Object, error) {
	if o, ok := d.cache[num]; ok {
		return o, nil
	}
	obj, err := d.loadObject(num)
	if err != nil && !d.rebuilt {
		d.rebuilt = true
		if rerr := d.rebuildXref(); rerr == nil {
			obj, err = d.loadObject(num)
		}
	}
	if err != nil {
		return nil, err
	}
	d.cache[num] = obj
	return obj, nil
}

func (d *Document) loadObject(num int) (Object, error) {
	entry, ok := d.xref[num]
	if !ok {
		return Null{}, nil
	}
	if entry.inStream {
		return d.objectFromStream(entry.stmNum, num)
	}
	got, gen, obj, err := d.parseIndirectAt(entry.offset)
	if err != nil {
		return nil, err
	}
	if got != num {
		return nil, fmt.Errorf("el xref dice que el objeto %d está en %d, pero ahí está el %d", num, entry.offset, got)
	}
	// Solo los objetos DIRECTOS del archivo están cifrados; los de un object
	// stream ya salen en claro al descifrar el stream que los contiene.
	return d.decryptObject(obj, num, gen), nil
}

// parseIndirectAt lee "num gen obj <objeto> endobj" en offset y devuelve el
// número y la generación que encontró, y el objeto interno.
func (d *Document) parseIndirectAt(offset int) (int, int, Object, error) {
	if offset < 0 || offset >= len(d.raw) {
		return 0, 0, nil, fmt.Errorf("offset %d fuera de rango", offset)
	}
	s := newScanner(d.raw)
	s.pos = offset
	num, ok := s.readInt()
	if !ok {
		return 0, 0, nil, fmt.Errorf("en %d no arranca un objeto indirecto", offset)
	}
	gen, ok := s.readInt()
	if !ok {
		return 0, 0, nil, fmt.Errorf("en %d falta la generación", offset)
	}
	s.skipWhite()
	if !s.hasPrefix("obj") {
		return 0, 0, nil, fmt.Errorf("en %d falta 'obj'", offset)
	}
	s.pos += len("obj")
	obj, err := s.parseObject()
	return num, gen, obj, err
}

// streamData devuelve los bytes exactos (y descifrados, si hace falta) de un
// stream. Con /Length directo ya vienen bien del scanner; con /Length indirecto
// se resuelve la referencia y se recorta con la longitud verdadera.
func (d *Document) streamData(stm *Stream) []byte {
	if stm.plain != nil {
		return stm.plain
	}
	raw := d.exactStreamBytes(stm)
	if stm.encrypted && d.crypt != nil && d.crypt.streamNeedsDecrypt(stm) {
		stm.plain = d.crypt.decrypt(raw, stm.cryptNum, stm.cryptGen, d.crypt.stmMethod)
		return stm.plain
	}
	return raw
}

func (d *Document) exactStreamBytes(stm *Stream) []byte {
	if stm.lenRef == nil {
		return stm.Raw
	}
	lo, err := d.getObject(stm.lenRef.Num)
	if err != nil {
		return stm.Raw
	}
	n, ok := intValue(lo)
	if !ok || n < 0 || stm.dataStart+n > len(stm.file) {
		return stm.Raw
	}
	s := newScanner(stm.file)
	s.pos = stm.dataStart + n
	s.skipWhite()
	if !s.hasPrefix("endstream") {
		return stm.Raw // la longitud declarada tampoco cierra: quedarse con el corte
	}
	return stm.file[stm.dataStart : stm.dataStart+n]
}

// objectFromStream saca el objeto num de un object stream ya expandido.
func (d *Document) objectFromStream(stmNum, num int) (Object, error) {
	os, err := d.loadObjectStream(stmNum)
	if err != nil {
		return nil, err
	}
	off, ok := os.offsets[num]
	if !ok {
		return Null{}, nil
	}
	s := newScanner(os.data)
	s.pos = off
	return s.parseObject()
}

// Resolve sigue una referencia indirecta hasta un valor concreto. Un valor que
// no es Ref se devuelve tal cual.
func (d *Document) Resolve(o Object) (Object, error) {
	seen := 0
	for {
		ref, ok := o.(Ref)
		if !ok {
			return o, nil
		}
		if seen++; seen > 100 {
			return nil, fmt.Errorf("cadena de referencias demasiado larga")
		}
		v, err := d.getObject(ref.Num)
		if err != nil {
			return nil, err
		}
		o = v
	}
}

// resolveDict resuelve una referencia y espera un diccionario (o un stream, del
// que devuelve su diccionario).
func (d *Document) resolveDict(o Object) (Dict, error) {
	v, err := d.Resolve(o)
	if err != nil {
		return nil, err
	}
	switch t := v.(type) {
	case Dict:
		return t, nil
	case *Stream:
		return t.Dict, nil
	case Null:
		return nil, nil
	}
	return nil, fmt.Errorf("esperaba un diccionario, vino %T", v)
}

// decodeStream devuelve el contenido de un stream aplicando el filtro que
// declare (solo FlateDecode y sin filtro; alcanza para xref y object streams).
// Para copiar páginas NO se decodifica: se preservan los bytes crudos.
func (d *Document) decodeStream(stm *Stream) ([]byte, error) {
	raw := d.streamData(stm)
	filter := stm.Dict[Name("Filter")]
	parms := d.decodeParms(stm.Dict)
	switch f := filter.(type) {
	case nil:
		return raw, nil
	case Name:
		return d.filterWithPredictor(string(f), raw, parms[0])
	case Array:
		out := raw
		for i, item := range f {
			name, _ := item.(Name)
			var dp Dict
			if i < len(parms) {
				dp = parms[i]
			}
			var err error
			out, err = d.filterWithPredictor(string(name), out, dp)
			if err != nil {
				return nil, err
			}
		}
		return out, nil
	}
	return raw, nil
}

// filterWithPredictor aplica un filtro y, si el DecodeParms lo pide, deshace el
// predictor (PNG/TIFF) que precede a FlateDecode.
func (d *Document) filterWithPredictor(name string, data []byte, dp Dict) ([]byte, error) {
	out, err := applyFilter(name, data)
	if err != nil {
		return nil, err
	}
	if dp == nil {
		return out, nil
	}
	params := readPredictorParams(dp)
	if params.predictor <= 1 {
		return out, nil
	}
	return applyPredictor(out, params)
}

// decodeParms devuelve los DecodeParms de un stream como una lista alineada con
// la lista de filtros (o un único elemento). Resuelve referencias indirectas.
func (d *Document) decodeParms(dict Dict) []Dict {
	raw := dict[Name("DecodeParms")]
	if raw == nil {
		raw = dict[Name("DP")]
	}
	resolved, _ := d.Resolve(raw)
	switch v := resolved.(type) {
	case Dict:
		return []Dict{v}
	case Array:
		out := make([]Dict, len(v))
		for i, item := range v {
			if dd, err := d.resolveDict(item); err == nil {
				out[i] = dd
			}
		}
		return out
	}
	return []Dict{nil}
}

func applyFilter(name string, data []byte) ([]byte, error) {
	switch name {
	case "FlateDecode", "Fl":
		return inflate(data)
	case "", "Identity":
		return data, nil
	}
	return nil, fmt.Errorf("filtro %q no soportado para xref/objstm", name)
}

// inflate descomprime datos FlateDecode con la tolerancia que piden los PDF del
// mundo real: si el checksum Adler-32 del final está roto (o el flujo termina
// antes) pero los datos salieron, se aceptan; si falta el encabezado zlib, se
// prueba deflate crudo.
func inflate(data []byte) ([]byte, error) {
	if r, err := zlib.NewReader(bytes.NewReader(data)); err == nil {
		out, rerr := io.ReadAll(r)
		r.Close()
		if rerr == nil || (len(out) > 0 && (errors.Is(rerr, zlib.ErrChecksum) || errors.Is(rerr, io.ErrUnexpectedEOF))) {
			return out, nil
		}
	}
	out, err := io.ReadAll(flate.NewReader(bytes.NewReader(data)))
	if err == nil || (len(out) > 0 && errors.Is(err, io.ErrUnexpectedEOF)) {
		return out, nil
	}
	return nil, fmt.Errorf("no pude descomprimir el flujo Flate")
}
