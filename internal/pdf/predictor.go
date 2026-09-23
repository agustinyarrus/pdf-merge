package pdf

import "fmt"

// predictor.go deshace los predictores de PNG y TIFF que se aplican antes de
// FlateDecode. Los xref streams casi siempre vienen con el predictor PNG "Up"
// (12): sin revertirlo, los bytes de la tabla salen corridos y no se puede
// resolver ningún objeto. Es exactamente el filtro que rompía la lectura.

// predictorParams son los parámetros de DecodeParms que gobiernan el predictor.
type predictorParams struct {
	predictor int
	colors    int
	bpc       int // bits por componente
	columns   int
}

// readPredictorParams extrae los parámetros de un DecodeParms (Dict), con los
// valores por defecto del estándar.
func readPredictorParams(dp Dict) predictorParams {
	p := predictorParams{predictor: 1, colors: 1, bpc: 8, columns: 1}
	if v, ok := intValue(dp[Name("Predictor")]); ok {
		p.predictor = v
	}
	if v, ok := intValue(dp[Name("Colors")]); ok {
		p.colors = v
	}
	if v, ok := intValue(dp[Name("BitsPerComponent")]); ok {
		p.bpc = v
	}
	if v, ok := intValue(dp[Name("Columns")]); ok {
		p.columns = v
	}
	return p
}

// bytesPerPixel es el desplazamiento del vecino izquierdo, en bytes.
func (p predictorParams) bytesPerPixel() int {
	bpp := (p.colors*p.bpc + 7) / 8
	if bpp < 1 {
		return 1
	}
	return bpp
}

// rowBytes es el ancho de una fila de datos, en bytes.
func (p predictorParams) rowBytes() int {
	return (p.colors*p.bpc*p.columns + 7) / 8
}

// applyPredictor revierte el predictor sobre data ya descomprimida.
func applyPredictor(data []byte, p predictorParams) ([]byte, error) {
	switch {
	case p.predictor <= 1:
		return data, nil
	case p.predictor == 2:
		return tiffPredictor(data, p)
	case p.predictor >= 10:
		return pngPredictor(data, p)
	}
	return nil, fmt.Errorf("predictor %d no soportado", p.predictor)
}

// pngPredictor revierte los filtros de PNG (por fila, con un byte de tipo al
// frente de cada una): None, Sub, Up, Average, Paeth.
func pngPredictor(data []byte, p predictorParams) ([]byte, error) {
	row := p.rowBytes()
	if row <= 0 {
		return nil, fmt.Errorf("predictor con ancho de fila inválido")
	}
	bpp := p.bytesPerPixel()
	stride := row + 1 // +1 por el byte de tipo de filtro
	if len(data)%stride != 0 {
		// Algunos generadores dejan la última fila sin el byte de tipo; se
		// tolera recortando lo que sobra.
		data = data[:len(data)/stride*stride]
	}
	rows := len(data) / stride
	out := make([]byte, 0, rows*row)
	prev := make([]byte, row)
	for r := 0; r < rows; r++ {
		ft := data[r*stride]
		cur := append([]byte(nil), data[r*stride+1:r*stride+1+row]...)
		for i := 0; i < row; i++ {
			var a, b, c int
			if i >= bpp {
				a = int(cur[i-bpp])
				c = int(prev[i-bpp])
			}
			b = int(prev[i])
			switch ft {
			case 0: // None
			case 1: // Sub
				cur[i] += byte(a)
			case 2: // Up
				cur[i] += byte(b)
			case 3: // Average
				cur[i] += byte((a + b) / 2)
			case 4: // Paeth
				cur[i] += byte(paeth(a, b, c))
			default:
				return nil, fmt.Errorf("tipo de filtro PNG %d desconocido", ft)
			}
		}
		out = append(out, cur...)
		prev = cur
	}
	return out, nil
}

// paeth es el predictor Paeth del estándar PNG.
func paeth(a, b, c int) int {
	p := a + b - c
	pa, pb, pc := abs(p-a), abs(p-b), abs(p-c)
	switch {
	case pa <= pb && pa <= pc:
		return a
	case pb <= pc:
		return b
	default:
		return c
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// tiffPredictor revierte el predictor TIFF 2 (diferencia horizontal).
func tiffPredictor(data []byte, p predictorParams) ([]byte, error) {
	if p.bpc != 8 {
		return nil, fmt.Errorf("predictor TIFF solo soportado con 8 bits por componente")
	}
	row := p.rowBytes()
	bpp := p.bytesPerPixel()
	out := append([]byte(nil), data...)
	for r := 0; r+row <= len(out); r += row {
		for i := bpp; i < row; i++ {
			out[r+i] += out[r+i-bpp]
		}
	}
	return out, nil
}
