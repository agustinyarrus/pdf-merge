# pdf-merge

Combina varios PDF en uno, en el orden que le digas y con las páginas que elijas, desde la consola de Windows. Un único `pdf-merge.exe` portable escrito en Go puro, con un lector y un escritor de PDF propios: sin instalador, sin dependencias, sin subir los documentos a ningún servicio.

```powershell
pdf-merge a.pdf b.pdf c.pdf -o todo.pdf
pdf-merge contrato.pdf@1-2 firmas.pdf -o firmado.pdf
pdf-merge libro.pdf@impares -o impares.pdf     # también: pares, reverso, 8- (hasta el final)
pdf-merge escaneos\ -o legajo.pdf              # una carpeta entera, en orden natural
pdf-merge protegido.pdf --password clave       # PDF que pide contraseña
```

Una corrida real (salida sin colores):

```
  pdf-merge  ·  combina varios PDF en uno, local                                             1.0.0


  ● 7 archivos   ● salida todo.pdf

  ✓ c.pdf@2-4   3 de 5 páginas · 3,22 KB
  ✓ b.pdf@reverso   2 páginas · 1,80 KB
  ✓ a.pdf   3 páginas · 2,29 KB
  ✓ formulario_b.pdf   3 páginas · 8,87 KB
  ✓ links.pdf   5 páginas · 4,63 KB
  ✓ formulario_a.pdf   2 páginas · 6,70 KB
  ✓ d.pdf   2 páginas · 2,60 KB

  ── pdf-merge ──
    Archivos              7
    Páginas               20
    Entrada               30,1 KB
    Salida                19,1 KB
    Repetidos fusionados  28 objetos · −791 B
    Marcadores            11
    Campos de formulario  10
    Archivo               todo.pdf
    Tiempo                3 ms
    ! 2 campos de formulario se renombraron para que no se mezclen entre archivos (sufijo _2, _3…)
```

Los archivos se leen en paralelo (por eso se tildan en el orden en que terminan), pero la salida respeta el orden de la línea de comandos.

## Qué hace

- **Parser propio y tolerante**: tablas xref clásicas y en stream, object streams, predictores PNG/TIFF, actualizaciones incrementales, basura antes del encabezado, offsets corridos (reconstruye la tabla barriendo el archivo) y `/Length` indirectos.
- **Selección de páginas** pegada a cada archivo con `@`: `1-3,5`, `8-` (hasta el final), `impares`, `pares`, `reverso`. Un archivo que se llama `factura@2026.pdf` se toma entero, no como selección.
- **Las páginas se copian tal cual**: no se recomprime nada ni se pierde calidad.
- **Deduplicación**: las imágenes y fuentes idénticas se guardan una vez aunque vengan de archivos distintos (hash de Merkle sobre el grafo de objetos, con las componentes fuertemente conexas de Tarjan). En una prueba con 56 PDF reales, la salida bajó de 94,6 MB a 21,8 MB.
- **Marcadores**: uno por archivo, con los marcadores originales anidados y apuntando a las páginas nuevas. Se podan los que apuntan a páginas no incluidas. `--bookmarks auto|files|keep|none`.
- **Enlaces internos**: los destinos con nombre se resuelven a destinos explícitos, así no chocan nombres entre archivos. Un enlace a una página que quedó afuera queda inerte, no roto.
- **Formularios**: los campos siguen siendo rellenables; si dos archivos tienen un campo con el mismo nombre, se renombra (`fecha` y `fecha_2`) para que no compartan valor. Avisa que una firma digital deja de validar al combinar.
- **Cifrado**: RC4 de 40 y 128 bits, AES-128 y AES-256 (revisiones 2 a 6 del manejador estándar). Prueba primero la contraseña vacía (los PDF "protegidos" que se abren sin pedir nada) y después las de `--password`, como de usuario o de propietario. La salida no va cifrada y lo avisa.
- **No pisa nada**: la salida se escribe de forma atómica y no se sobrescribe sin `--force`.

## Instalación

Con [Go](https://go.dev/dl) 1.24 o más nuevo (el `go.mod` pide 1.26 y Go descarga sola esa versión la primera vez):

```powershell
go install github.com/agustinyarrus/pdf-merge@latest
```

O desde el código, con la versión y el commit estampados en el exe:

```powershell
git clone https://github.com/agustinyarrus/pdf-merge
cd pdf-merge
.\build.ps1          # compila a dist\pdf-merge.exe
.\build.ps1 -Test    # antes corre go vet y todos los tests
```

No tiene módulos externos: el lector, el escritor, el cifrado y la deduplicación son biblioteca estándar de Go y código propio.

## Uso

`pdf-merge --help` (salida real, sin colores):

```
  pdf-merge  ·  combina varios PDF en uno, local                                             1.0.0

  uso
    pdf-merge <archivo.pdf[@páginas]|carpeta|patrón>… [-o salida.pdf]
    pdf-merge a.pdf b.pdf c.pdf -o todo.pdf
    pdf-merge informe.pdf@1-3 anexo.pdf@5,8- -o recorte.pdf

  salida
    -o, --out ARCHIVO                PDF de salida (merged.pdf)
    -f, --force                      sobrescribir la salida si ya existe
        --no-dedupe                  no fusionar imágenes y fuentes repetidas entre archivos
    -b, --bookmarks auto|files|keep|none
                                     marcadores: uno por archivo con los originales adentro
                                     (auto), solo por archivo (files), solo los originales (keep)
                                     o ninguno (none) (auto)

  entrada
    -r, --recursive                  entrar en subcarpetas al pasar una carpeta
        --skip-errors                seguir sin los PDF que no se puedan leer (por defecto se
                                     aborta)
    -p, --password CLAVE             contraseña para abrir PDF cifrados (repetible; se prueba en
                                     cada uno)
    -j, --jobs N                     lecturas en paralelo (0 = una por CPU) (0)

  general
    -h, --help                       muestra esta ayuda
    -V, --version                    muestra la versión
        --no-color                   salida sin colores (también respeta NO_COLOR)
```

Flags al estilo GNU (`-o x`, `--out=x`, `-rf`, `--`); un flag mal escrito sugiere el más parecido. Los patrones (`*`, `?`, `[]`, `**`) los expande la herramienta, porque ni cmd ni PowerShell lo hacen para un exe nativo, y las carpetas se recorren en orden natural (`pag2` antes que `pag10`).

Códigos de salida: `0` todo bien · `1` se combinó salteando PDF ilegibles (`--skip-errors`) · `2` línea de comandos inválida · `3` no se pudo combinar · `130` cancelado con Ctrl+C.

## Cómo se verifica

pdf-merge no se contrasta consigo misma: un parser que lee lo que escribió su propio escritor no prueba nada. Cada salida pasa por tres lectores independientes:

- **qpdf** (vía pikepdf): chequeo estructural estricto; solo cuentan las advertencias nuevas, no las que ya traía el origen.
- **pypdf**: la cantidad de páginas y el texto de cada una, en orden.
- **PDFium** (el motor de Chrome, vía pypdfium2): cada página combinada renderizada y comparada píxel a píxel con su original.

| Etapa | Resultado |
|---|---|
| fixtures: rangos, reverso, object streams, imágenes | qpdf sin advertencias, texto en orden, 0 píxeles distintos |
| marcadores y enlaces, incluidos destinos con nombre en todas sus formas | 6 de 6 casos |
| cifrado: R2 a R6, contraseña de usuario y de propietario, object streams cifrados | 13 de 13 casos |
| formularios: campos únicos, sin widgets huérfanos, cada uno con su valor | todos |
| barrido: 56 PDF reales (Chrome en varias versiones, iTextSharp, SAP NetWeaver, reportlab…) | 56 de 56; todos juntos, 352 páginas en unos 2 s |

Los oráculos encontraron seis defectos que las pruebas internas no veían (números reales redondeados que achicaban la negrita de Chrome, el predictor de los xref streams, `/Length` indirecto, basura antes del `%PDF-`, contraseñas en PDFDocEncoding y advertencias heredadas del origen); están contados en [docs/VERIFICACION.md](docs/VERIFICACION.md), con cómo correr todo con un solo comando.

Además hay 57 tests de Go (`go test ./...`); siete leen PDF de prueba que genera `scripts/fixtures.ps1` y, si no están, se saltean sin fallar.

## Cómo está hecho

```
main.go              el punto de entrada: abre la consola, llama a pdfmerge.Main y sale con su código
internal/
  pdfmerge/          la herramienta: flags, archivo@páginas, lectura en paralelo, tarjeta final
  pdf/               lexer, parser tolerante, xref y object streams, cifrado, combinación,
                     deduplicación, marcadores, destinos con nombre, formularios, escritor
    _oracle/         verify.py (qpdf + pypdf + PDFium) y los guiones de cada etapa; all.ps1 corre todo
    _dbg/            muestra el xref y el trailer que ve el parser
  batch/             tareas en paralelo con barra viva y resultados en orden
  fsx/               patrones con **, orden natural, escritura atómica
  cli/               flags estilo GNU, ayuda, sugerencias por distancia de edición
  tui/               consola: paleta, progreso vivo, tarjetas, formato es-AR
  textdist/          distancia de Damerau–Levenshtein
  win/desk/          modo y tamaño de la consola por syscall
  version/           versión y commit, estampados por build.ps1
scripts/             fixtures.ps1 y fixtures_pdf.py: los PDF de prueba
```

La arquitectura y los algoritmos (lectura, combinación en dos pasadas, deduplicación por Merkle + Tarjan, cifrado): [docs/ARQUITECTURA.md](docs/ARQUITECTURA.md). Lo que falta: [docs/PENDIENTE.md](docs/PENDIENTE.md).

## Origen

pdf-merge nació dentro de navaja, una suite de herramientas de consola para Windows que compartían un núcleo (la consola, los flags, los patrones de archivos, el trabajo en paralelo). Desde la 1.0.0 es un proyecto propio: se llevó ese núcleo y la historia de git de sus archivos.

## Licencia

[MIT](LICENSE).
