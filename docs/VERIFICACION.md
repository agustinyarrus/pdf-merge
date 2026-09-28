# Verificación

La regla: pdf-merge se contrasta con algo **independiente**. Un parser que lee lo que escribió su propio escritor no prueba nada; qpdf, pypdf y PDFium sí.

## Tests de Go

```powershell
go test ./...
.\build.ps1 -Test     # go vet + go test y después compila
```

61 tests en `cli`, `fsx`, `textdist`, `tui`, `win/desk`, `pdf` (parser, selección de páginas, combinación, deduplicación y las componentes de Tarjan) y `pdfmerge` (los avisos partidos al ancho de la ventana y el valor de "Repetidos fusionados"). Siete de `pdf` leen PDF de prueba: buscan la carpeta de `$env:PDF_FIXTURES` o, si no está definida, `%TEMP%\pdf-merge-fx`, y si no existe se saltean sin fallar. Para generarla:

```powershell
.\scripts\fixtures.ps1                    # necesita Python con pillow, pikepdf y reportlab
.\scripts\fixtures.ps1 -Dir D:\fx\pdf     # en otra carpeta; después $env:PDF_FIXTURES = 'D:\fx\pdf'
```

Hay también un test de dependencias: `go list -deps` sobre `tui` y el exe no puede traer `net`, `net/netip` ni `os/exec`.

## Oráculos

Requisitos: Python 3 con `pip install -r internal/pdf/_oracle/requirements.txt` (numpy, pikepdf, Pillow, pypdf, pypdfium2 y reportlab, en las versiones con que se verificó).

```powershell
.\internal\pdf\_oracle\all.ps1 [-Corpus lista.txt] [-Fx carpeta] [-Work carpeta]
```

Compila pdf-merge, corre todo y termina con una tarjeta de resumen. `-Fx` son los fixtures de `scripts\fixtures.ps1` (por defecto, los mismos que leen los tests); `-Work` es donde quedan el exe y las salidas (por defecto `%TEMP%\pdf-merge-oracle`). El corpus es un archivo de texto con una ruta de PDF por línea, por ejemplo:

```powershell
Get-ChildItem $HOME\Documents -Recurse -Filter *.pdf | ForEach-Object FullName > corpus.txt
```

Sin corpus se corren solo las etapas con casos armados.

| Etapa | Script | Qué comprueba |
|---|---|---|
| tests de Go | `go test ./internal/pdf/` | parser, merge, deduplicación, Tarjan |
| fixtures | `verify.py` | rangos, reverso, object streams, una página con imagen |
| marcadores y enlaces | `outline_cases.ps1` + `outline_check.py` | 6 casos, incluidos destinos con nombre en todas sus formas |
| cifrado | `crypt_check.py` | 13 casos: R2–R6, contraseña de usuario y de propietario, object streams cifrados |
| formularios | `list_forms.py` + `form_check.py` | campos únicos, sin widgets huérfanos, cada uno con su valor original |
| barrido | `sweep.py` | cada PDF del corpus combinado solo |
| estrés | `bigmerge.py` | todo el corpus en un único PDF |

`verify.py` es el oráculo triple que usan las demás etapas:

- **qpdf** (vía pikepdf): chequeo estructural estricto. Solo cuentan las advertencias nuevas, no las que ya traía el origen.
- **pypdf**: la cantidad de páginas y el texto de cada una, en orden.
- **PDFium** (el motor de Chrome, vía pypdfium2): cada página combinada renderizada y comparada píxel a píxel con la original.

Con un corpus de 56 PDF reales (generados por Chrome en varias versiones, iTextSharp, SAP NetWeaver, reportlab y otros), los 56 pasan. Combinados todos juntos dan 352 páginas en unos 2 s, y la deduplicación baja de 94,6 MB a 21,8 MB.

`streamdiff.py` y `dictdiff.py` son herramientas de diagnóstico: comparan streams y diccionarios de origen y salida para encontrar dónde difieren.

### Lecciones

Los oráculos encontraron seis defectos que las pruebas internas no veían:

- Los números reales se redondeaban a 6 decimales. La `/FontMatrix` de las fuentes Type 3 que genera Chrome (1/2048) quedaba en 0.000488 y la negrita se dibujaba un 0,06 % más chica. Ahora se usa la representación más corta que vuelve exacta al mismo float64.
- Los xref streams llevan el predictor PNG "Up" y hay que revertirlo antes de leerlos.
- Cuando `/Length` es una referencia indirecta, hay que resolverla para cortar el stream exacto.
- Puede venir basura antes del `%PDF-` (el estándar la tolera en el primer KB).
- En las revisiones 2 a 4, las contraseñas van en PDFDocEncoding: una "ñ" es el byte 0xF1, no los dos bytes de UTF-8.
- Advertencias que ya traía el PDF de origen aparecían como si fueran de la combinación. El oráculo ahora las compara contra las del origen.

## En la CI

Cada push corre [`.github/workflows/ci.yml`](../.github/workflows/ci.yml) en `windows-latest`, con el Go mínimo del `go.mod` (1.26.0): Python 3.12 con los paquetes de [`requirements.txt`](../internal/pdf/_oracle/requirements.txt), `gofmt`, `go mod tidy -diff`, `go vet ./...` (también con `GOOS=linux`), los PDF de prueba con `scripts\fixtures.ps1`, todas las pruebas sin salteadas con el resumen de [`.github/pruebas.ps1`](../.github/pruebas.ps1), `build.ps1` y el `.exe` con su versión y su commit, `all.ps1` sin corpus (las etapas de casos armados: pruebas de `pdf`, fixtures con el oráculo triple, marcadores y enlaces, cifrado) y, al final, `SHA256SUMS` y el `.exe` como artefacto. El barrido de PDF reales no corre ahí: son documentos privados y no viajan con el repo.

La CI no corrió todavía en GitHub (el repo no se publicó). Se validó con [actionlint](https://github.com/rhysd/actionlint) y se simuló en la PC de desarrollo: un clon limpio, Go 1.26.0, cachés vacías, un Python 3.12 recién creado y los pasos `run:` del workflow con el mismo envoltorio de PowerShell que usa el runner. Todos en verde.

## Corrida del 28/09/2026, ya como proyecto propio

Con el pdf-merge que compila este repo (1.0.0), en Windows 11 con Go 1.27.1 y Python 3.12 con pikepdf 10.13 (qpdf 12.3.2), pypdf 6.19 y pypdfium2 5.13, y los fixtures recién generados por `scripts\fixtures.ps1`:

| Qué | Resultado |
|---|---|
| `gofmt -l .`, `go vet ./...` (Windows y Linux) | limpios |
| `go test ./...` con fixtures | 61 de 61, ninguno salteado |
| `all.ps1`: fixtures con el oráculo triple | 10 páginas: qpdf sin advertencias nuevas, texto en orden, 0 píxeles distintos |
| `all.ps1`: marcadores y enlaces | 6 de 6 |
| `all.ps1`: cifrado | 13 de 13 |
| el `legajo.pdf` de la demo (`informe.pdf@1-4 escaneo.pdf@impares facturas`), con `verify.py` | qpdf sin advertencias nuevas, 16 páginas con su texto en orden, 0 píxeles distintos |
| la CI simulada con Go 1.26.0 | todos los pasos en verde |

La corrida anterior, cuando pdf-merge ya era proyecto propio pero antes de esta, agregó a `all.ps1` un corpus armado de 10 PDF (los fixtures y tres formularios de reportlab con un campo `fecha` repetido): formularios 12 de 12 campos con su valor y nombres únicos (`fecha_2`, `fecha_3`), barrido 10 de 10 y los 10 en un solo PDF de 33 páginas.

El corpus de 56 PDF reales no se volvió a correr: son documentos privados y no viajan con el repo. Para repetirlo con los propios, `-Corpus`.
