<p align="right"><sub><b>English</b> · <a href="ARQUITECTURA.md">Español</a></sub></p>

# Architecture

## Principles

- **Pure Go, no CGO and no external modules.** pdf-merge builds into a single static `.exe`. The reader, the writer, the encryption and the deduplication are the standard library and our own code.
- **One responsibility per package.** `pdf` knows nothing about consoles: it takes bytes and returns documents, and it merges sources into a new document. The tool (`internal/pdfmerge`) is the interface: arguments, parallel reading, presentation.
- **Validate at the boundary.** Flags and page selections are interpreted once, when parsing; the output is validated before reading anything (that it ends in `.pdf`, that it isn't one of the inputs, that it doesn't exist without `--force`).
- **Atomic writes.** The output goes to a temporary file in the same folder and is renamed at the end: a crash or a Ctrl+C never leaves a half-written PDF or overwrites the previous one.
- **Visual output that degrades.** Pastel colors on black, live progress and a summary card in the console; plain text without a single escape code when the output is a pipe or a file.

## The path of a run

1. `main.go` opens the console (`tui.Open`), calls `pdfmerge.Main` with the version and exits with the code it returns.
2. Each argument is split into pattern and selection (`informe.pdf@1-3,5`). It's split at the **last** `@` and only if what follows is a valid selection; if the whole argument exists as a file, it isn't split (`factura@2026.pdf`).
3. `fsx.Expand` resolves patterns and folders (with `-r`, recursively) keeping the order of the arguments; inside a folder, natural order.
4. `validarSalida` stops before reading if the output would overwrite an input or an existing file.
5. `batch.Run` reads and validates the PDFs in parallel (one read per CPU, or `--jobs`), trying the passwords on each one. If any fails, nothing is merged, except with `--skip-errors`.
6. `pdf.Merge` merges in the order of the arguments; deduplication and writing come after.
7. The final card counts files, pages, input and output bytes, what was merged (the objects and, if there were streams, the bytes saved), bookmarks, fields and the time, and below go the warnings, wrapped at the window's width (owner restrictions that aren't kept, renamed fields, signatures that stop validating, `/XFA` dropped).

## pdf: reading

- **Lexer**: recursive descent over the file in memory.
- **Numbers**: written with the shortest representation that round-trips to exactly the same float64. An integer that overflows int64 is kept as a real.
- **Cross-references**:
  - classic tables read by tokens (19- or 21-byte rows are tolerated);
  - xref streams with `/W`, `/Index` and PNG/TIFF predictors;
  - object streams;
  - the `/Prev` chain from newest to oldest;
  - hybrid xrefs with `/XRefStm`.
- **Recovery**:
  - if the table fails, it's rebuilt by sweeping the file for `N G obj`;
  - if an object isn't where the table says (shifted offsets), the rebuild is done just once, on the fly;
  - the trailer is looked for in order: the last `trailer`, then the most recent xref stream, then the catalog by hand.
- **Encryption**: it's set up before validating the catalog, because if the catalog is in an encrypted object stream, without the key it couldn't even be read.
  - Strings are decrypted when each direct object is loaded.
  - Streams are decrypted when their bytes are read, with a memo.
  - The standard's exceptions: `/Encrypt` itself, xref streams, objects inside an object stream (they already come out in the clear) and `/Metadata` with `EncryptMetadata false`.
  - 40- and 128-bit RC4, AES-128 and AES-256. For revision 6 it uses ISO 32000-2's Algorithm 2.B. In revisions 2 to 4 the password goes in PDFDocEncoding (an "ñ" is the byte 0xF1, not UTF-8's two bytes).
  - The empty password is tried first (PDFs that only have restrictions open without asking for anything) and then each `--password`, as a user password and as an owner password.

## pdf: merging

- **One copier per document**: the same PDF requested twice copies its sources only once.
- **Traversal barriers**: a reference to an unselected page becomes null, and the old page tree and the source catalog aren't followed. That way no orphan pages get dragged along.
- **Two passes**: first the numbers of every output page are reserved and then the copy happens. A forward link already finds its target.
- **Inherited attributes**: `MediaBox`, `CropBox`, `Resources` and `Rotate` are materialized on each page.
- **Selections**: `1-3,5`, `8-`, `impares` (odd), `pares` (even) and `reverso` (reverse), 1-based and validated against each document's real page count.

## pdf: deduplication

- **Merkle hash**: each object's hash is computed over its canonical form (each value with its type tag, lengths prefixed, dictionaries with sorted keys), with every reference replaced by the hash of the object it points to. `/Length` doesn't go in: the writer recomputes it.
- **Order**: children are ready before parents because Tarjan's strongly connected components are traversed (iterative version), and they come out in reverse topological order.
- **Objects that keep their identity**: the ones inside a cycle, and pages, annotations, fields and layers. They get a hash no content can produce, so they're never merged.
- **Compaction**: at the end, what's reachable from the catalog is marked and renumbered. It's all O(V + E), plus hashing the bytes.

## pdf: bookmarks and links

- Each source's bookmark tree is read and its destinations are remapped.
- The ones that lead nowhere are pruned and a correct `/Count` is written (positive if expanded, negative if collapsed).
- Named destinations are resolved to explicit destinations: both PDF 1.1's `/Dests` and the `/Names` tree with its `/Kids`. That way two files with a destination of the same name don't clash, and a link to a page that was left out goes inert instead of broken.
- `--bookmarks`: `auto` (one per file with the originals inside), `files`, `keep` or `none`.

## pdf: forms

- Fields that have some widget on an included page get in, and their `/Kids` are pruned.
- Root names that clash across different files are renamed (`fecha`, `fecha_2`, `fecha_3`).
- `/DR`, `/DA`, `/NeedAppearances` and `/SigFlags` are merged; if there were signatures, it warns that they stop validating. `/XFA` is dropped, with a warning.

## pdf: writing

- A classic xref table.
- `/Length` always direct and equal to the bytes written.
- Sorted keys, so the output is deterministic.

**Diagnostics**: `go run ./internal/pdf/_dbg/dbg.go archivo.pdf` shows the xref and the trailer the parser sees, using `pdf.Debug`.

## The console core

### tui

- **Live region**: progress is redrawn in place at 20 frames per second and permanent lines are printed above it, with a fixed lock order so there's no deadlock.
- **Safe clipping**: `ClipANSI` cuts a line with escape codes to N visible columns without breaking the colors.
- **Lines that don't fit**: errors (`✗`) and the footer's warnings (`!`) are wrapped by words at the window's width, with the extra lines aligned after the mark (`Term.Marked`); into a pipe they go whole. Before, the console cut them wherever they fell.
- **Bar**: 1/8-column resolution (`▏▎▍▌▋▊▉█` blocks) with a gradient; in Windows Terminal the progress is also published on the icon and the tab (OSC 9;4).
- Borderless **card** and **es-AR formatting**: thousands with a period, decimals with a comma, bytes in decimal units.

### cli

- GNU-style flags: `-o x`, `--out=x`, `-rf`, `--no-x`, `--`; typed binders with validation (bounded integers, enums, repeatables like `--password`).
- The help is generated with the same visual language as the rest of the output, at the window's real width.
- A misspelled flag suggests the closest one by Damerau–Levenshtein distance (`textdist`).

### fsx

- Patterns with `*`, `?`, `[]` and `**`, resolved segment by segment and case-insensitively, like Windows. Neither cmd nor PowerShell expand wildcards for a native exe: the tool does it.
- Natural order: `pag2` before `pag10`, comparing runs of digits by value and with no length limit.
- `WriteAtomic` retries the rename with exponential backoff (an antivirus or a PDF viewer may hold the destination open for a moment).

### batch

- A worker pool; results are stored by index, so the summary is deterministic even if they finish out of order.
- A panic in a task becomes a failure of that task, without taking the batch down: a pathological PDF doesn't take the whole run with it.

### win/desk

- The console's mode and size through `syscall`, with `//go:uintptrescapes` on the wrapper for the calls. It only loads `kernel32.dll`, a KnownDLL that Windows always takes from System32 (with any other DLL named without a path, `LoadLibrary` would look next to the exe first).
- It's small on purpose: a test (`internal/tui/deps_test.go`) checks with `go list -deps` that neither `tui` nor the exe load `net`, `net/netip` or `os/exec`.

### version

`Version` and `Commit` are variables that `build.ps1` overrides with `-ldflags -X`: `pdf-merge --version` says `1.0.0+abc1234`. Built with `go install`, it says `1.0.0`.

<!-- i18n source=ARQUITECTURA.md sha256=63052c90facd -->
