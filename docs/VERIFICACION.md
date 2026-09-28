# Verificación

La regla: pdf-merge se contrasta con algo **independiente**. Un parser que lee lo que escribió su propio escritor no prueba nada; qpdf, pypdf y PDFium sí.

## Tests de Go

```powershell
go test ./...
.\build.ps1 -Test     # go vet + go test y después compila
```

57 tests en `cli`, `fsx`, `textdist`, `tui`, `win/desk` y `pdf` (parser, selección de páginas, combinación, deduplicación y las componentes de Tarjan). Siete de `pdf` leen PDF de prueba: buscan la carpeta de `$env:PDF_FIXTURES` o, si no está definida, `%TEMP%\pdf-merge-fx`, y si no existe se saltean sin fallar. Para generarla:

```powershell
.\scripts\fixtures.ps1                    # necesita Python con pillow, pikepdf y reportlab
.\scripts\fixtures.ps1 -Dir D:\fx\pdf     # en otra carpeta; después $env:PDF_FIXTURES = 'D:\fx\pdf'
```

Hay también un test de dependencias: `go list -deps` sobre `tui` y el exe no puede traer `net`, `net/netip` ni `os/exec`.

## Oráculos

Requisitos: Python 3 con `pip install pillow numpy pikepdf pypdf pypdfium2 reportlab`.

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

## Corrida del 28/09/2026, ya como proyecto propio

Con el pdf-merge que compila este repo (1.0.0), en Windows 11 con Go 1.26.4, pikepdf 10.13 (qpdf), pypdf y pypdfium2, y los fixtures recién generados por `scripts\fixtures.ps1`:

| Qué | Resultado |
|---|---|
| `gofmt -l .`, `go vet ./...` | limpios |
| `go test ./...` con fixtures | 57 de 57, ninguno salteado |
| `all.ps1`: fixtures con el oráculo triple | 10 páginas: qpdf sin advertencias nuevas, texto en orden, 0 píxeles distintos |
| `all.ps1`: marcadores y enlaces | 6 de 6 |
| `all.ps1`: cifrado | 13 de 13 |
| `all.ps1` con un corpus armado de 10 PDF (los fixtures y tres formularios de reportlab con un campo `fecha` repetido) | formularios: 12 de 12 campos con su valor y nombres únicos (`fecha_2`, `fecha_3`); barrido 10 de 10; los 10 en un solo PDF de 33 páginas |

El corpus de 56 PDF reales no se volvió a correr: son documentos privados y no viajan con el repo. Para repetirlo con los propios, `-Corpus`.
