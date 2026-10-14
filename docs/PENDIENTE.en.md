<p align="right"><sub><b>English</b> · <a href="PENDIENTE.md">Español</a></sub></p>

# To do

pdf-merge is finished and verified ([VERIFICACION.en.md](VERIFICACION.en.md)). What follows are improvements, in the order it's best to do them.

## Output

- **Object streams and an xref stream in the output**: today a classic xref table is written, which is the most compatible form but not the smallest. With object streams the structure would take quite a bit less space in documents with many pages.
- **Recompressing the streams that come unfiltered** (some generators write the page content in the clear).

## What's lost today when merging

- **Each file's own page numbering** (`/PageLabels`): today the output numbers straight through.
- **Layers** (`/OCProperties`): each source's optional content groups would have to be merged.
- **Accessibility structure** (tagged PDF, `/StructTreeRoot`): today it's dropped. Merging it properly is the biggest job on the list.

## Distribution

- Done: the CI ([`ci.yml`](../.github/workflows/ci.yml)) runs all the tests on every push, with the test PDFs, and `all.ps1` without a corpus; the 1.0.0 release is ready to publish ([RELEASE.en.md](RELEASE.en.md)).
- Missing: publishing it, and seeing the CI's first run on GitHub (it was simulated on the PC, it didn't run on a real runner).

<!-- i18n source=PENDIENTE.md sha256=6e51e14a8bdf -->
