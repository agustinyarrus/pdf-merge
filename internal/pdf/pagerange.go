package pdf

import (
	"fmt"
	"strconv"
	"strings"
)

// pagerange.go interpreta selecciones de páginas al estilo de una impresora:
// "1-3,5,8-" y también "impares"/"pares"/"reverso". Los números son 1-basados y
// se validan contra la cantidad real de páginas del documento.

// PageRange es una selección de páginas ya interpretada.
type PageRange struct {
	spec string
	segs []segment
}

type segment struct {
	from, to int  // 1-basados; to=0 significa "hasta el final"
	kind     kind // normal, pares, impares
	reverse  bool
}

type kind uint8

const (
	kAll kind = iota
	kOdd
	kEven
)

// ParsePageRange interpreta una especificación. Vacía = todas las páginas.
func ParsePageRange(spec string) (*PageRange, error) {
	spec = strings.TrimSpace(spec)
	pr := &PageRange{spec: spec}
	if spec == "" {
		pr.segs = []segment{{from: 1, to: 0}}
		return pr, nil
	}
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(strings.ToLower(part))
		if part == "" {
			continue
		}
		switch part {
		case "impares", "odd":
			pr.segs = append(pr.segs, segment{from: 1, to: 0, kind: kOdd})
			continue
		case "pares", "even":
			pr.segs = append(pr.segs, segment{from: 1, to: 0, kind: kEven})
			continue
		case "reverso", "reverse", "rev":
			pr.segs = append(pr.segs, segment{from: 1, to: 0, reverse: true})
			continue
		}
		seg, err := parseSegment(part)
		if err != nil {
			return nil, err
		}
		pr.segs = append(pr.segs, seg)
	}
	if len(pr.segs) == 0 {
		return nil, fmt.Errorf("selección de páginas vacía: %q", spec)
	}
	return pr, nil
}

func parseSegment(part string) (segment, error) {
	if lo, hi, ok := strings.Cut(part, "-"); ok {
		from := 1
		to := 0 // abierto = hasta el final
		if strings.TrimSpace(lo) != "" {
			v, err := strconv.Atoi(strings.TrimSpace(lo))
			if err != nil || v < 1 {
				return segment{}, fmt.Errorf("página inicial inválida en %q", part)
			}
			from = v
		}
		if strings.TrimSpace(hi) != "" {
			v, err := strconv.Atoi(strings.TrimSpace(hi))
			if err != nil || v < 1 {
				return segment{}, fmt.Errorf("página final inválida en %q", part)
			}
			to = v
		}
		return segment{from: from, to: to}, nil
	}
	v, err := strconv.Atoi(part)
	if err != nil || v < 1 {
		return segment{}, fmt.Errorf("número de página inválido: %q", part)
	}
	return segment{from: v, to: v}, nil
}

// Select aplica la selección a las páginas de un documento (1-basado).
func (pr *PageRange) Select(all []Page) ([]Page, error) {
	n := len(all)
	var out []Page
	for _, seg := range pr.segs {
		to := seg.to
		if to == 0 || to > n {
			to = n
		}
		from := seg.from
		if from > n {
			return nil, fmt.Errorf("la página %d no existe (el archivo tiene %d)", from, n)
		}
		idxs := make([]int, 0, to-from+1)
		for i := from; i <= to; i++ {
			switch seg.kind {
			case kOdd:
				if i%2 == 1 {
					idxs = append(idxs, i)
				}
			case kEven:
				if i%2 == 0 {
					idxs = append(idxs, i)
				}
			default:
				idxs = append(idxs, i)
			}
		}
		if seg.reverse {
			for l, r := 0, len(idxs)-1; l < r; l, r = l+1, r-1 {
				idxs[l], idxs[r] = idxs[r], idxs[l]
			}
		}
		for _, i := range idxs {
			out = append(out, all[i-1])
		}
	}
	return out, nil
}

// String devuelve la especificación original.
func (pr *PageRange) String() string {
	if pr.spec == "" {
		return "todas"
	}
	return pr.spec
}
