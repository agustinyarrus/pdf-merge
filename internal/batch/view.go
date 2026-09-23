package batch

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agustinyarrus/pdf-merge/internal/tui"
)

func defaultWorkers() int {
	n := runtime.NumCPU()
	if n < 1 {
		return 1
	}
	return n
}

type panicErr struct{ v any }

func (e panicErr) Error() string { return fmt.Sprintf("%v", e.v) }

func errPanic(v any) error { return panicErr{v} }

// activeSet lleva la cuenta de qué tareas están corriendo ahora mismo, para
// mostrarlas en la línea del medio de la barra. Protegido por mutex porque lo
// tocan todos los workers.
type activeSet struct {
	mu    sync.Mutex
	items map[string]int // etiqueta → orden de llegada, para mostrarlas estable
	seq   int
}

func newActiveSet() *activeSet { return &activeSet{items: map[string]int{}} }

func (a *activeSet) add(label string) {
	a.mu.Lock()
	a.seq++
	a.items[label] = a.seq
	a.mu.Unlock()
}

func (a *activeSet) remove(label string) {
	a.mu.Lock()
	delete(a.items, label)
	a.mu.Unlock()
}

func (a *activeSet) snapshot() []string {
	a.mu.Lock()
	type it struct {
		label string
		order int
	}
	list := make([]it, 0, len(a.items))
	for l, o := range a.items {
		list = append(list, it{l, o})
	}
	a.mu.Unlock()
	sort.Slice(list, func(i, j int) bool { return list[i].order < list[j].order })
	out := make([]string, len(list))
	for i, x := range list {
		out[i] = x.label
	}
	return out
}

// renderBar dibuja las dos líneas de la barra viva: progreso con conteo, y la
// lista de archivos en curso.
func renderBar(t *tui.Term, title string, done, total, okN, skipN, failN int, active []string, width int) []string {
	inner := width - len(tui.Margin)*2
	frac := 0.0
	if total > 0 {
		frac = float64(done) / float64(total)
	}

	counts := t.Paint(tui.Sage, fmt.Sprintf("%d ✓", okN))
	if skipN > 0 {
		counts += "  " + t.Paint(tui.Cream, fmt.Sprintf("%d ↷", skipN))
	}
	if failN > 0 {
		counts += "  " + t.Paint(tui.Rose, fmt.Sprintf("%d ✗", failN))
	}
	head := t.Paint(tui.Lavender, tui.Spinner(time.Now())+" "+title)
	progreso := t.Paint(tui.Subtle, fmt.Sprintf("%d/%d", done, total))
	right := progreso + "   " + counts
	gap := inner - tui.Width(head) - tui.Width(right)
	if gap < 1 {
		gap = 1
	}
	line1 := tui.Margin + head + strings.Repeat(" ", gap) + right

	barW := inner - 8
	pct := fmt.Sprintf(" %3.0f%%", frac*100)
	line2 := tui.Margin + t.Bar(frac, barW) + t.Paint(tui.Subtle, pct)

	lines := []string{line1, line2}
	if len(active) > 0 {
		etiqueta := "· " + strings.Join(active, "   · ")
		lines = append(lines, tui.Margin+t.Italic(tui.Faint, tui.Truncate(etiqueta, inner)))
	}
	return lines
}

// Tally resume una lista de resultados.
type Tally struct {
	OK, Skip, Fail int
	Total          int
	Elapsed        time.Duration
}

// Summarize cuenta los resultados por estado.
func Summarize(results []Result, elapsed time.Duration) Tally {
	tl := Tally{Total: len(results), Elapsed: elapsed}
	for _, r := range results {
		switch r.Status {
		case OK:
			tl.OK++
		case Skip:
			tl.Skip++
		case Fail:
			tl.Fail++
		}
	}
	return tl
}
