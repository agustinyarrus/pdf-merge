<p align="right"><sub><b>English</b> · <a href="release-v1.0.0.md">Español</a></sub></p>

# pdf-merge 1.0.0

The first version of pdf-merge as a project of its own. It used to live in navaja, the suite of console tools for Windows; the code is the one that was tested there (plus three presentation changes, each with its test: the header says the version with its `v`; errors and warnings that don't fit in the window are wrapped by words instead of being cut wherever they fall; and if what was merged are only objects without a stream, the card says the objects and not "−0 B"), now with its own repo, its version number, its CI and its demo.

pdf-merge is for merging PDFs without uploading them to a "merge PDF for free" site: a contract with its annex, the month's invoices in a single file, the odd pages of a scan. From the console, in the order you tell it and with the pages you pick.

## What it brings

**Merging, with page selection.** `pdf-merge a.pdf b.pdf c.pdf -o todo.pdf`, and each file can carry its selection right after it: `contrato.pdf@1-2`, `anexo.pdf@5,8-` (from 8 to the end), `libro.pdf@impares` (odd), `pares` (even), `reverso` (reverse). A whole folder goes in natural order (`pag2` before `pag10`). Files are read in parallel, with a live bar, and the output follows the order of the command line. At the end, a card with files, pages, sizes, what was merged, bookmarks and fields.

**Pages are copied as they are.** Nothing is recompressed and no quality is lost.

**Its own tolerant reader.** Classic and stream xref tables, object streams, PNG/TIFF predictors, incremental updates, garbage before the header, shifted offsets (it rebuilds the table by sweeping the file) and indirect `/Length`. Pure Go, no dependencies.

**Deduplication.** Identical images and fonts are stored only once even if they come from different files: a Merkle hash over the object graph, with Tarjan's strongly connected components. In a test with 56 real PDFs, the output went down from 94.6 MB to 21.8 MB.

**Bookmarks, links and forms.** One bookmark per file with the originals nested inside, pointing to the new pages (`--bookmarks auto|files|keep|none`). Named destinations are resolved, so they don't clash across files, and a link to a page that was left out goes inert, not broken. Form fields stay fillable; if two files have a field with the same name, it's renamed (`fecha` and `fecha_2`) so they don't share a value.

**Encryption.** 40- and 128-bit RC4, AES-128 and AES-256. It tries the empty password first ("protected" PDFs that open without asking for anything) and then the ones from `--password`, as a user or owner password. The output isn't encrypted: each decrypted file is marked as such and, if it had owner restrictions, a warning says they don't carry over to the output.

**It doesn't overwrite anything.** The output is written atomically and isn't overwritten without `--force`; if it would overwrite one of the inputs, it stops before reading. Exit codes: `0` all good, `1` merged skipping unreadable PDFs (`--skip-errors`), `2` invalid command line, `3` it couldn't merge, `130` canceled.

## Download

`pdf-merge.exe` is the whole program: 64-bit Windows 10 or 11 (it was tested on Windows 11), a single file, no installer. Copy it to a folder on your `PATH`. Or, with Go:

```powershell
go install github.com/agustinyarrus/pdf-merge@v1.0.0
```

To check the download, in the folder where the two files ended up:

```powershell
(Get-FileHash .\pdf-merge.exe -Algorithm SHA256).Hash -eq ((Get-Content .\SHA256SUMS) -split '\s+')[0]   # True
```

In Git Bash or WSL, `sha256sum -c SHA256SUMS`. The `.exe` is reproducible: the same tag with the same Go gives the same bytes, so anyone can build it and compare ([how](https://github.com/agustinyarrus/pdf-merge/blob/v1.0.0/docs/RELEASE.md#5-el-exe-es-reproducible)).

## How it was verified

- pdf-merge isn't checked against itself: every merged PDF goes through three independent readers. qpdf (strict structure), pypdf (pages and text in order) and PDFium, Chrome's engine (each page rendered and compared pixel by pixel with its original).
- With the hand-built cases: ranges, reverse and object streams with 0 pixels different; bookmarks and links, 6 of 6; encryption from revision 2 to 6, 13 of 13; forms, each field with its value and unique names. With a corpus of 56 real PDFs (Chrome, iTextSharp, SAP NetWeaver, reportlab…), 56 of 56.
- The oracles found six defects the internal tests didn't see, among them rounded real numbers that shrank Chrome's bold and passwords in PDFDocEncoding.
- 61 Go tests, all of them running: the ones in `pdf` read generated test PDFs.
- CI configured for `windows-latest` with the minimum Go from the `go.mod`: formatting, `go vet` (for Linux too), all the tests with none skipped, the triple oracle on the hand-built cases and the `.exe` with its version and its SHA256. This version was verified with the same sequence on a clean clone, before publishing it.

The details, in [docs/VERIFICACION.md](https://github.com/agustinyarrus/pdf-merge/blob/v1.0.0/docs/VERIFICACION.md); the architecture and the algorithms, in [docs/ARQUITECTURA.md](https://github.com/agustinyarrus/pdf-merge/blob/v1.0.0/docs/ARQUITECTURA.md).

## What's missing

- **What's lost today when merging:** each file's own page numbering (`/PageLabels`), layers (`/OCProperties`) and the accessibility structure of a tagged PDF.
- **The output doesn't use object streams:** it's the most compatible form, not the smallest.
- The corpus of 56 real PDFs was run when pdf-merge lived in the suite; they're private documents and don't travel with the repo, so as a project of its own it was run again with a hand-built corpus of 10.

The list, in [docs/PENDIENTE.md](https://github.com/agustinyarrus/pdf-merge/blob/v1.0.0/docs/PENDIENTE.md).

<!-- i18n source=release-v1.0.0.md sha256=bb89b5df7d32 -->
