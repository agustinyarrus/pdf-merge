# pdf-merge 1.0.0

La primera versión de pdf-merge como proyecto propio. Antes vivía en navaja, la suite de herramientas de consola para Windows; el código es el que se probó ahí (más un cambio de presentación: la cabecera dice la versión con su `v`), ahora con su repo, su número de versión, su CI y su demo.

pdf-merge es para juntar PDF sin subirlos a un sitio de "combinar PDF gratis": un contrato con su anexo, las facturas del mes en un solo archivo, las páginas impares de un escaneo. Desde la consola, en el orden que le digas y con las páginas que elijas.

## Qué trae

**Combinar, con selección de páginas.** `pdf-merge a.pdf b.pdf c.pdf -o todo.pdf`, y cada archivo puede llevar pegada su selección: `contrato.pdf@1-2`, `anexo.pdf@5,8-` (del 8 al final), `libro.pdf@impares`, `pares`, `reverso`. Una carpeta entera va en orden natural (`pag2` antes que `pag10`). Los archivos se leen en paralelo, con una barra viva, y la salida respeta el orden de la línea de comandos. Al final, una tarjeta con archivos, páginas, pesos, lo que se fusionó, marcadores y campos.

**Las páginas se copian tal cual.** No se recomprime nada ni se pierde calidad.

**Un lector propio y tolerante.** Tablas xref clásicas y en stream, object streams, predictores PNG/TIFF, actualizaciones incrementales, basura antes del encabezado, offsets corridos (reconstruye la tabla barriendo el archivo) y `/Length` indirectos. Go puro, sin dependencias.

**Deduplicación.** Las imágenes y fuentes idénticas se guardan una sola vez aunque vengan de archivos distintos: hash de Merkle sobre el grafo de objetos, con las componentes fuertemente conexas de Tarjan. En una prueba con 56 PDF reales, la salida bajó de 94,6 MB a 21,8 MB.

**Marcadores, enlaces y formularios.** Un marcador por archivo con los originales anidados adentro, apuntando a las páginas nuevas (`--bookmarks auto|files|keep|none`). Los destinos con nombre se resuelven, así no chocan entre archivos, y un enlace a una página que quedó afuera queda inerte, no roto. Los campos de formulario siguen siendo rellenables; si dos archivos tienen un campo con el mismo nombre, se renombra (`fecha` y `fecha_2`) para que no compartan valor.

**Cifrado.** RC4 de 40 y 128 bits, AES-128 y AES-256. Prueba primero la contraseña vacía (los PDF "protegidos" que se abren sin pedir nada) y después las de `--password`, como de usuario o de propietario. La salida no va cifrada, y lo avisa.

**No pisa nada.** La salida se escribe de forma atómica y no se sobrescribe sin `--force`; si pisaría una de las entradas, frena antes de leer. Códigos de salida: `0` todo bien, `1` se combinó salteando PDF ilegibles (`--skip-errors`), `2` línea de comandos inválida, `3` no se pudo combinar, `130` cancelado.

## Descargar

`pdf-merge.exe` es el programa entero: Windows 10 u 11 de 64 bits (se probó en Windows 11), un solo archivo, sin instalador. Copialo a una carpeta del `PATH`. O, con Go:

```powershell
go install github.com/agustinyarrus/pdf-merge@v1.0.0
```

Para comprobar la descarga, en la carpeta donde quedaron los dos archivos:

```powershell
(Get-FileHash .\pdf-merge.exe -Algorithm SHA256).Hash -eq ((Get-Content .\SHA256SUMS) -split '\s+')[0]   # True
```

En Git Bash o WSL, `sha256sum -c SHA256SUMS`. El `.exe` es reproducible: la misma etiqueta con el mismo Go da los mismos bytes, así que cualquiera puede compilarla y comparar ([cómo](https://github.com/agustinyarrus/pdf-merge/blob/v1.0.0/docs/RELEASE.md#5-el-exe-es-reproducible)).

## Cómo se verificó

- pdf-merge no se contrasta consigo misma: cada PDF combinado pasa por tres lectores independientes. qpdf (estructura estricta), pypdf (páginas y texto en orden) y PDFium, el motor de Chrome (cada página renderizada y comparada píxel a píxel con su original).
- Con los casos armados: rangos, reverso y object streams con 0 píxeles distintos; marcadores y enlaces, 6 de 6; cifrado de la revisión 2 a la 6, 13 de 13; formularios, cada campo con su valor y nombres únicos. Con un corpus de 56 PDF reales (Chrome, iTextSharp, SAP NetWeaver, reportlab…), 56 de 56.
- Los oráculos encontraron seis defectos que las pruebas internas no veían, entre ellos números reales redondeados que achicaban la negrita de Chrome y contraseñas en PDFDocEncoding.
- 58 pruebas de Go, todas corriendo: las de `pdf` leen PDF de prueba generados.
- CI en `windows-latest` con el Go mínimo del `go.mod`: formato, `go vet` (también para Linux), todas las pruebas sin salteadas, el oráculo triple sobre los casos armados y el `.exe` con su versión y su SHA256.

El detalle, en [docs/VERIFICACION.md](https://github.com/agustinyarrus/pdf-merge/blob/v1.0.0/docs/VERIFICACION.md); la arquitectura y los algoritmos, en [docs/ARQUITECTURA.md](https://github.com/agustinyarrus/pdf-merge/blob/v1.0.0/docs/ARQUITECTURA.md).

## Lo que falta

- **Lo que hoy se pierde al combinar:** la numeración de páginas propia de cada archivo (`/PageLabels`), las capas (`/OCProperties`) y la estructura de accesibilidad de un PDF etiquetado.
- **La salida no usa object streams:** es la forma más compatible, no la más chica.
- El corpus de 56 PDF reales se corrió cuando pdf-merge vivía en la suite; son documentos privados y no viajan con el repo, así que como proyecto propio se volvió a correr con un corpus armado de 10.

La lista, en [docs/PENDIENTE.md](https://github.com/agustinyarrus/pdf-merge/blob/v1.0.0/docs/PENDIENTE.md).
