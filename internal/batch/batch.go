// Package batch corre muchas tareas independientes en paralelo con una barra
// de progreso viva: narra qué se está haciendo, tilda cada una al terminar y
// deja el conteo a la vista. Lo comparten las herramientas que procesan varios
// archivos (img, pdf-merge, vidsquash), para no reescribir el mismo pool y la
// misma barra en cada una.
package batch

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/agustinyarrus/pdf-merge/internal/tui"
)

// Status es el desenlace de una tarea.
type Status uint8

const (
	OK Status = iota
	Skip
	Fail
)

func (s Status) kind() tui.Kind {
	switch s {
	case Skip:
		return tui.Skip
	case Fail:
		return tui.Fail
	}
	return tui.OK
}

// Task es una unidad de trabajo. Run recibe un contexto que se cancela con
// Ctrl+C y devuelve el desenlace, una línea de detalle, un dato opaco para que
// el que llama arme su resumen, y un error opcional.
type Task struct {
	Label string
	Run   func(ctx context.Context) (Status, string, any, error)
}

// Result es lo que dejó una tarea, en el orden original.
type Result struct {
	Index    int
	Label    string
	Status   Status
	Detail   string
	Err      error
	Data     any
	Duration time.Duration
}

// Options ajusta la corrida.
type Options struct {
	Title   string // título de la barra
	Workers int    // paralelismo; <=0 usa la cantidad de CPUs
	Verb    string // "convertido", "comprimido"… para el detalle por defecto
}

// Run ejecuta las tareas y devuelve los resultados en el orden de entrada.
// Mantiene el orden con un arreglo indexado (no un canal de resultados) para
// que el resumen sea determinista aunque los workers terminen desordenados.
func Run(ctx context.Context, t *tui.Term, tasks []Task, opt Options) []Result {
	results := make([]Result, len(tasks))
	if len(tasks) == 0 {
		return results
	}
	workers := opt.Workers
	if workers <= 0 {
		workers = defaultWorkers()
	}
	workers = min(workers, len(tasks))

	var done int64
	var okN, skipN, failN int64
	active := newActiveSet()

	live := t.StartLive(func(width int) []string {
		return renderBar(t, opt.Title, int(atomic.LoadInt64(&done)), len(tasks),
			int(atomic.LoadInt64(&okN)), int(atomic.LoadInt64(&skipN)), int(atomic.LoadInt64(&failN)),
			active.snapshot(), width)
	})

	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				task := tasks[i]
				active.add(task.Label)
				start := time.Now()

				status, detail, data, err := runSafely(ctx, task)

				res := Result{Index: i, Label: task.Label, Status: status, Detail: detail, Err: err, Data: data, Duration: time.Since(start)}
				results[i] = res

				switch status {
				case OK:
					atomic.AddInt64(&okN, 1)
				case Skip:
					atomic.AddInt64(&skipN, 1)
				case Fail:
					atomic.AddInt64(&failN, 1)
				}
				atomic.AddInt64(&done, 1)
				active.remove(task.Label)
				t.TaskProgress(taskState(status), float64(atomic.LoadInt64(&done))/float64(len(tasks)))
				live.Println(t.Status(status.kind(), task.Label, detail))
			}
		}()
	}

	go func() {
		defer close(jobs)
		for i := range tasks {
			select {
			case <-ctx.Done():
				return
			case jobs <- i:
			}
		}
	}()

	wg.Wait()
	live.Stop(false)
	t.TaskProgressClear()
	return results
}

// runSafely corre una tarea recuperando un pánico como fallo, para que un
// archivo corrupto no tire abajo toda la tanda.
func runSafely(ctx context.Context, task Task) (status Status, detail string, data any, err error) {
	defer func() {
		if r := recover(); r != nil {
			status, detail, data, err = Fail, "error interno procesando el archivo", nil, errPanic(r)
		}
	}()
	return task.Run(ctx)
}

func taskState(s Status) int {
	if s == Fail {
		return 2 // rojo en la barra de tareas de Windows Terminal
	}
	return 1
}
