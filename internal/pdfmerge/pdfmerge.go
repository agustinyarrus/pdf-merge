// Package pdfmerge implementa `pdf-merge`: combina varios PDF en uno, en el
// orden dado, con selección de páginas por archivo. Todo local, sin subir nada.
package pdfmerge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/agustinyarrus/pdf-merge/internal/batch"
	"github.com/agustinyarrus/pdf-merge/internal/cli"
	"github.com/agustinyarrus/pdf-merge/internal/fsx"
	"github.com/agustinyarrus/pdf-merge/internal/pdf"
	"github.com/agustinyarrus/pdf-merge/internal/tui"
)

type opciones struct {
	salida     string
	forzar     bool
	recursivo  bool
	saltarMal  bool
	sinDedupe  bool
	marcadores string
	claves     []string
	trabajos   int
}

// entrada es un archivo concreto a combinar, con su selección.
type entrada struct {
	path  string
	pages *pdf.PageRange
	sel   string
}

// leido es lo que deja la lectura de una entrada.
type leido struct {
	source   pdf.Source
	total    int
	elegidas int
	bytes    int64
}

// Main es el punto de entrada de pdf-merge: interpreta args, hace el trabajo y
// devuelve el código de salida.
func Main(t *tui.Term, version string, args []string) int {
	o := opciones{salida: "merged.pdf", marcadores: "auto"}
	app := cli.New("pdf-merge", version, "combina varios PDF en uno, local")
	app.Usage = []string{
		"pdf-merge <archivo.pdf[@páginas]|carpeta|patrón>… [-o salida.pdf]",
		"pdf-merge a.pdf b.pdf c.pdf -o todo.pdf",
		"pdf-merge informe.pdf@1-3 anexo.pdf@5,8- -o recorte.pdf",
	}
	app.Section("salida")
	app.String(&o.salida, "out", 'o', "ARCHIVO", "PDF de salida")
	app.Bool(&o.forzar, "force", 'f', "sobrescribir la salida si ya existe")
	app.Bool(&o.sinDedupe, "no-dedupe", 0, "no fusionar imágenes y fuentes repetidas entre archivos")
	app.Enum(&o.marcadores, "bookmarks", 'b', "marcadores: uno por archivo con los originales adentro (auto), solo por archivo (files), solo los originales (keep) o ninguno (none)", "auto", "files", "keep", "none")
	app.Section("entrada")
	app.Bool(&o.recursivo, "recursive", 'r', "entrar en subcarpetas al pasar una carpeta")
	app.Bool(&o.saltarMal, "skip-errors", 0, "seguir sin los PDF que no se puedan leer (por defecto se aborta)")
	app.Strings(&o.claves, "password", 'p', "CLAVE", "contraseña para abrir PDF cifrados (repetible; se prueba en cada uno)")
	app.Int(&o.trabajos, "jobs", 'j', "N", "lecturas en paralelo (0 = una por CPU)", 0, 256)
	app.Examples = []cli.Example{
		{Cmd: "pdf-merge *.pdf -o todo.pdf", Desc: "todos los PDF de la carpeta, en orden natural"},
		{Cmd: "pdf-merge contrato.pdf@1-2 firmas.pdf", Desc: "las dos primeras del contrato y después las firmas"},
		{Cmd: "pdf-merge libro.pdf@impares -o impares.pdf", Desc: "también: pares, reverso, 8- (hasta el final)"},
		{Cmd: "pdf-merge escaneos/ -o legajo.pdf", Desc: "una carpeta entera (orden natural: pag2 antes que pag10)"},
	}
	app.Notes = []string{
		"Las páginas se copian tal cual: no se recomprime ni se pierde calidad.",
		"Las imágenes y fuentes idénticas se guardan una sola vez aunque vengan de archivos distintos (típico al juntar facturas o catálogos del mismo sistema).",
		"Los enlaces internos se conservan cuando apuntan a páginas incluidas.",
		"Los PDF cifrados que se abren sin contraseña (solo restricciones) se combinan directo; los que piden una, con --password. La salida no va cifrada.",
		"Si un nombre de archivo tiene '@' (factura@2026.pdf) se toma entero, no como selección.",
	}

	pos, done, code := app.Start(t, args)
	if done {
		return code
	}
	if len(pos) == 0 {
		t.Lines(app.Help(t))
		return cli.ExitUsage
	}

	entradas, problemas, err := resolverEntradas(pos, o)
	if err != nil {
		return fallo(t, err)
	}

	t.Lines(t.Header("pdf-merge", "combina varios PDF en uno, local", version))
	for _, p := range problemas {
		t.Line(t.Status(tui.Warn, fsx.Display(p.Pattern), p.Err.Error()))
	}
	if len(entradas) == 0 {
		t.Blank()
		t.Line(t.Status(tui.Fail, "No hay PDF para combinar", "revisá los nombres o los patrones"))
		return cli.ExitFailure
	}
	if err := validarSalida(o, entradas); err != nil {
		return fallo(t, err)
	}

	ctx, stop := cli.Interrupt(nil)
	defer stop()
	inicio := time.Now()

	t.Lines([]string{"", t.Chips(
		tui.Chip{Text: tui.Count(int64(len(entradas)), "archivo", "archivos"), Dot: tui.Teal},
		tui.Chip{Text: "salida " + fsx.Display(o.salida), Dot: tui.Lavender},
	), ""})

	resultados := batch.Run(ctx, t, tareasDeLectura(entradas, o.claves), batch.Options{Title: "Leyendo PDF", Workers: o.trabajos})
	if ctx.Err() != nil {
		return cli.ExitInterrupted
	}

	sources, leidos, fallidos := juntar(resultados)
	if fallidos > 0 && !o.saltarMal {
		t.Blank()
		t.Line(t.Status(tui.Fail, "No se combinó nada", fmt.Sprintf("%s con error (usá --skip-errors para seguir sin ellos)", tui.Count(int64(fallidos), "archivo", "archivos"))))
		return cli.ExitFailure
	}
	if len(sources) == 0 {
		return fallo(t, errors.New("ningún PDF se pudo leer"))
	}

	modo, _ := pdf.ParseBookmarkMode(o.marcadores) // ya validado por el Enum
	res, dd, bytesOut, err := escribir(t, o.salida, sources, pdf.MergeOptions{Bookmarks: modo}, !o.sinDedupe)
	if err != nil {
		return fallo(t, err)
	}

	t.Lines(tarjeta(t, leidos, res, dd, bytesOut, fallidos, o.salida, time.Since(inicio)))
	restringidos := 0
	for _, l := range leidos {
		if l.source.Doc.Restricted() {
			restringidos++
		}
	}
	if restringidos > 0 {
		res.Warnings = append(res.Warnings, fmt.Sprintf("%s tenía restricciones del propietario (imprimir, copiar, editar); la salida no las conserva", tui.Count(int64(restringidos), "archivo", "archivos")))
	}
	for _, w := range res.Warnings {
		t.Lines(aviso(t, w))
	}
	if len(res.Warnings) > 0 {
		t.Blank()
	}
	if fallidos > 0 {
		return cli.ExitPartial
	}
	return cli.ExitOK
}

// resolverEntradas convierte los argumentos en archivos concretos, conservando
// el orden de los argumentos y la selección de cada uno.
func resolverEntradas(pos []string, o opciones) ([]entrada, []fsx.Problem, error) {
	var out []entrada
	var problemas []fsx.Problem
	for _, arg := range pos {
		spec, err := parseSpec(arg)
		if err != nil {
			return nil, nil, err
		}
		files, probs := fsx.Expand([]string{spec.pattern}, fsx.ExpandOptions{
			Recursive: o.recursivo,
			Accept:    func(p string) bool { return strings.EqualFold(filepath.Ext(p), ".pdf") },
		})
		problemas = append(problemas, probs...)
		for _, f := range files {
			out = append(out, entrada{path: f, pages: spec.pages, sel: spec.raw})
		}
	}
	return out, problemas, nil
}

// validarSalida frena antes de leer nada si la salida pisaría una entrada o un
// archivo existente sin --force.
func validarSalida(o opciones, entradas []entrada) error {
	if !strings.EqualFold(filepath.Ext(o.salida), ".pdf") {
		return fmt.Errorf("la salida tiene que terminar en .pdf (vino %q)", o.salida)
	}
	for _, e := range entradas {
		if fsx.SameFile(e.path, o.salida) {
			return fmt.Errorf("la salida %s es también una de las entradas: elegí otro nombre con -o", fsx.Display(o.salida))
		}
	}
	if !o.forzar && fsx.Exists(o.salida) {
		return fmt.Errorf("%s ya existe (usá --force para sobrescribirlo)", fsx.Display(o.salida))
	}
	return nil
}

// tareasDeLectura arma una tarea por entrada: leer, parsear y seleccionar.
func tareasDeLectura(entradas []entrada, claves []string) []batch.Task {
	tareas := make([]batch.Task, len(entradas))
	for i, e := range entradas {
		e := e
		label := fsx.Display(e.path)
		if e.sel != "" {
			label += "@" + e.sel
		}
		tareas[i] = batch.Task{
			Label: label,
			Run: func(ctx context.Context) (batch.Status, string, any, error) {
				l, err := leer(e, claves)
				if err != nil {
					return batch.Fail, explicar(err), nil, err
				}
				det := fmt.Sprintf("%s · %s", tui.Count(int64(l.elegidas), "página", "páginas"), tui.Bytes(l.bytes))
				if l.elegidas != l.total {
					det = fmt.Sprintf("%d de %d páginas · %s", l.elegidas, l.total, tui.Bytes(l.bytes))
				}
				if l.source.Doc.Encrypted() {
					det += " · descifrado"
				}
				return batch.OK, det, l, nil
			},
		}
	}
	return tareas
}

func leer(e entrada, claves []string) (leido, error) {
	raw, err := os.ReadFile(e.path)
	if err != nil {
		return leido{}, err
	}
	doc, err := pdf.ParseWithPasswords(raw, claves)
	if err != nil {
		return leido{}, err
	}
	all, err := doc.Pages()
	if err != nil {
		return leido{}, err
	}
	sel := all
	if e.pages != nil {
		if sel, err = e.pages.Select(all); err != nil {
			return leido{}, err
		}
	}
	if len(sel) == 0 {
		return leido{}, errors.New("la selección no deja ninguna página")
	}
	titulo := strings.TrimSuffix(filepath.Base(e.path), filepath.Ext(e.path))
	return leido{
		source:   pdf.Source{Doc: doc, Pages: sel, Title: titulo},
		total:    len(all),
		elegidas: len(sel),
		bytes:    int64(len(raw)),
	}, nil
}

// explicar traduce errores frecuentes a algo accionable.
func explicar(err error) string {
	switch {
	case errors.Is(err, pdf.ErrPassword):
		return "pide contraseña para abrirse: pasala con --password"
	case errors.Is(err, os.ErrNotExist):
		return "no existe"
	}
	return err.Error()
}

// juntar separa las lecturas buenas (en el orden original) de las fallidas.
func juntar(resultados []batch.Result) ([]pdf.Source, []leido, int) {
	var sources []pdf.Source
	var leidos []leido
	fallidos := 0
	for _, r := range resultados {
		l, ok := r.Data.(leido)
		if r.Status != batch.OK || !ok {
			fallidos++
			continue
		}
		sources = append(sources, l.source)
		leidos = append(leidos, l)
	}
	return sources, leidos, fallidos
}

// escribir combina, deduplica y escribe de forma atómica, con una línea viva
// que narra la etapa en curso.
func escribir(t *tui.Term, salida string, sources []pdf.Source, opts pdf.MergeOptions, dedupe bool) (pdf.MergeResult, pdf.DedupeStats, int64, error) {
	var etapa atomic.Value
	etapa.Store("Combinando páginas…")
	live := t.StartLive(func(width int) []string {
		return []string{tui.Margin + t.Paint(tui.Lavender, tui.Spinner(time.Now())+" "+etapa.Load().(string))}
	})
	var res pdf.MergeResult
	var dd pdf.DedupeStats
	var n int64
	err := fsx.WriteAtomic(salida, func(w io.Writer) error {
		out := pdf.NewBuilder()
		r, err := pdf.Merge(out, sources, opts)
		if err != nil {
			return err
		}
		res = r
		if dedupe {
			etapa.Store("Buscando imágenes y fuentes repetidas…")
			dd = out.Dedupe() // incluye la recolección de lo inalcanzable
		} else {
			out.Collect()
		}
		etapa.Store("Escribiendo…")
		n, err = out.WriteTo(w)
		return err
	})
	live.Stop(false)
	return res, dd, n, err
}

func tarjeta(t *tui.Term, leidos []leido, res pdf.MergeResult, dd pdf.DedupeStats, bytesOut int64, fallidos int, salida string, d time.Duration) []string {
	var bytesIn int64
	for _, l := range leidos {
		bytesIn += l.bytes
	}
	items := []tui.Item{
		{Label: "Archivos", Value: tui.Int(int64(len(leidos))), Dot: tui.Teal},
		{Label: "Páginas", Value: tui.Int(int64(res.TotalPages)), Dot: tui.Lavender},
		{Label: "Entrada", Value: tui.Bytes(bytesIn), Dot: tui.Sky},
		{Label: "Salida", Value: tui.Bytes(bytesOut), Dot: tui.Peach},
	}
	if dd.Merged > 0 {
		items = append(items, tui.Item{
			Label: "Repetidos fusionados",
			Value: repetidos(dd),
			Dot:   tui.Cream,
		})
	}
	if res.Bookmarks > 0 {
		items = append(items, tui.Item{Label: "Marcadores", Value: tui.Int(int64(res.Bookmarks)), Dot: tui.Sky})
	}
	if res.FormFields > 0 {
		items = append(items, tui.Item{Label: "Campos de formulario", Value: tui.Int(int64(res.FormFields)), Dot: tui.Lavender})
	}
	items = append(items,
		tui.Item{Label: "Archivo", Value: fsx.Display(salida), Dot: tui.Sage},
		tui.Item{Label: "Tiempo", Value: tui.Duration(d), Dot: tui.Pink},
	)
	if fallidos > 0 {
		items = append(items, tui.Item{Label: "Omitidos", Value: tui.Int(int64(fallidos)), Dot: tui.Rose})
	}
	return t.Card("pdf-merge", items)
}

func fallo(t *tui.Term, err error) int {
	t.Blank()
	t.Lines(t.Marked("✗ ", tui.Rose, err.Error()))
	t.Blank()
	return cli.ExitFailure
}

// repetidos es el valor de "Repetidos fusionados": cuántos objetos y cuánto
// se ahorró. Los objetos sin stream (un diccionario de fuente, un recurso
// compartido) no suman bytes de stream: si solo se fusionaron de esos, van
// los objetos solos y no un "−0 B".
func repetidos(dd pdf.DedupeStats) string {
	s := tui.Count(int64(dd.Merged), "objeto", "objetos")
	if dd.BytesSaved > 0 {
		s += " · −" + tui.Bytes(dd.BytesSaved)
	}
	return s
}

// aviso arma las líneas de un aviso al pie ("! …"), partido en palabras al
// ancho de la ventana: si no, la consola lo corta en mitad de una palabra.
// Las líneas de más quedan alineadas después del "!". A un pipe o a un
// archivo sin ancho, el aviso va entero en una línea.
func aviso(t *tui.Term, w string) []string {
	sangria := strings.Repeat(" ", tui.Width(tui.Margin+t.Status(tui.Warn, "", "")))
	partes := []string{w}
	if t.Bounded() {
		partes = tui.Wrap(w, t.Width()-len(sangria)-len(tui.Margin))
	}
	out := []string{tui.Margin + t.Status(tui.Warn, partes[0], "")}
	for _, p := range partes[1:] {
		out = append(out, sangria+t.Paint(tui.Text, p))
	}
	return out
}
