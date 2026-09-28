# Arquitectura

## Principios

- **Go puro, sin CGO y sin módulos externos.** pdf-merge compila a un único `.exe` estático. El lector, el escritor, el cifrado y la deduplicación son biblioteca estándar y código propio.
- **Una responsabilidad por paquete.** `pdf` no sabe nada de consolas: recibe bytes y devuelve documentos, y combina fuentes en un documento nuevo. La herramienta (`internal/pdfmerge`) es la interfaz: argumentos, lectura en paralelo, presentación.
- **Validar en la frontera.** Los flags y las selecciones de páginas se interpretan una sola vez al parsear; la salida se valida antes de leer nada (que termine en `.pdf`, que no sea una de las entradas, que no exista sin `--force`).
- **Escritura atómica.** La salida va a un temporal en la misma carpeta y se renombra al final: un corte o un Ctrl+C nunca deja un PDF a medias ni pisa el anterior.
- **Salida visual que degrada.** Colores pastel sobre negro, progreso vivo y tarjeta de resumen en la consola; texto plano sin un solo escape cuando la salida es un pipe o un archivo.

## El recorrido de una corrida

1. `main.go` abre la consola (`tui.Open`), llama a `pdfmerge.Main` con la versión y sale con el código que devuelve.
2. Cada argumento se separa en patrón y selección (`informe.pdf@1-3,5`). Se parte en la **última** `@` y solo si lo que sigue es una selección válida; si el argumento entero existe como archivo, no se parte (`factura@2026.pdf`).
3. `fsx.Expand` resuelve patrones y carpetas (con `-r`, recursivo) conservando el orden de los argumentos; dentro de una carpeta, orden natural.
4. `validarSalida` frena antes de leer si la salida pisaría una entrada o un archivo existente.
5. `batch.Run` lee y valida los PDF en paralelo (una lectura por CPU, o `--jobs`), probando las contraseñas en cada uno. Si alguno falla, no se combina nada, salvo con `--skip-errors`.
6. `pdf.Merge` combina en el orden de los argumentos; la deduplicación y la escritura van detrás.
7. La tarjeta final cuenta archivos, páginas, bytes de entrada y salida, lo que se fusionó (los objetos y, si hubo streams, los bytes ahorrados), marcadores, campos y el tiempo, y abajo van los avisos, partidos al ancho de la ventana (restricciones del propietario que no se conservan, campos renombrados, firmas que dejan de validar, `/XFA` descartado).

## pdf: lectura

- **Lexer**: descendente recursivo sobre el archivo en memoria.
- **Números**: se escriben con la representación más corta que vuelve exacta al mismo float64. Un entero que desborda int64 se conserva como real.
- **Referencias cruzadas**:
  - tablas clásicas leídas por tokens (tolera filas de 19 o 21 bytes);
  - xref streams con `/W`, `/Index` y predictores PNG/TIFF;
  - object streams;
  - la cadena `/Prev` de la más nueva a la más vieja;
  - xref híbridos con `/XRefStm`.
- **Recuperación**:
  - si la tabla falla, se reconstruye barriendo el archivo por `N G obj`;
  - si un objeto no está donde dice la tabla (offsets corridos), la reconstrucción se hace una sola vez, al vuelo;
  - el trailer se busca en orden: el último `trailer`, después el xref stream más reciente, después el catálogo a mano.
- **Cifrado**: se configura antes de validar el catálogo, porque si el catálogo está en un object stream cifrado, sin clave ni se podría leer.
  - Las cadenas se descifran al cargar cada objeto directo.
  - Los streams se descifran al leer sus bytes, con memo.
  - Excepciones del estándar: el propio `/Encrypt`, los xref streams, los objetos dentro de un object stream (ya salen en claro) y los `/Metadata` con `EncryptMetadata false`.
  - RC4 de 40 y 128 bits, AES-128 y AES-256. Para la revisión 6 usa el Algorithm 2.B de ISO 32000-2. En las revisiones 2 a 4 la contraseña va en PDFDocEncoding (una "ñ" es el byte 0xF1, no los dos de UTF-8).
  - Primero se prueba la contraseña vacía (los PDF que solo tienen restricciones se abren sin pedir nada) y después cada `--password`, como de usuario y como de propietario.

## pdf: combinación

- **Un copiador por documento**: el mismo PDF pedido dos veces copia sus fuentes una sola vez.
- **Barreras del recorrido**: una referencia a una página no seleccionada pasa a null, y el árbol de páginas viejo y el catálogo de origen no se siguen. Así no se arrastran páginas huérfanas.
- **Dos pasadas**: primero se reservan los números de todas las páginas de salida y después se copia. Un enlace hacia adelante ya encuentra su destino.
- **Atributos heredados**: `MediaBox`, `CropBox`, `Resources` y `Rotate` se materializan en cada página.
- **Selecciones**: `1-3,5`, `8-`, `impares`, `pares` y `reverso`, 1-basadas y validadas contra la cantidad real de páginas de cada documento.

## pdf: deduplicación

- **Hash de Merkle**: el hash de cada objeto se calcula sobre su forma canónica (cada valor con su etiqueta de tipo, los largos prefijados, los diccionarios con las claves ordenadas), con cada referencia reemplazada por el hash del objeto apuntado. `/Length` no entra: el escritor la recalcula.
- **Orden**: los hijos quedan listos antes que los padres porque se recorren las componentes fuertemente conexas de Tarjan (versión iterativa), que salen en orden topológico inverso.
- **Objetos que conservan identidad**: los que están dentro de un ciclo, y las páginas, anotaciones, campos y capas. Reciben un hash que ningún contenido puede producir, así nunca se fusionan.
- **Compactación**: al final se marca lo alcanzable desde el catálogo y se renumera. Todo es O(V + E), más el hashing de los bytes.

## pdf: marcadores y enlaces

- Se lee el árbol de marcadores de cada origen y se remapean sus destinos.
- Se podan los que no llevan a nada y se escribe un `/Count` correcto (positivo si está desplegado, negativo si está plegado).
- Los destinos con nombre se resuelven a destinos explícitos: tanto el `/Dests` de PDF 1.1 como el árbol `/Names` con sus `/Kids`. Así dos archivos con un destino del mismo nombre no chocan, y un enlace a una página que quedó afuera queda inerte en vez de roto.
- `--bookmarks`: `auto` (uno por archivo con los originales adentro), `files`, `keep` o `none`.

## pdf: formularios

- Entran los campos que tienen algún widget en una página incluida, y se podan sus `/Kids`.
- Los nombres raíz que chocan entre archivos distintos se renombran (`fecha`, `fecha_2`, `fecha_3`).
- Se fusionan `/DR`, `/DA`, `/NeedAppearances` y `/SigFlags`; si había firmas, se avisa que dejan de validar. El `/XFA` se descarta, con aviso.

## pdf: escritura

- Tabla xref clásica.
- `/Length` siempre directo e igual a los bytes escritos.
- Claves ordenadas, para que la salida sea determinista.

**Diagnóstico**: `go run ./internal/pdf/_dbg/dbg.go archivo.pdf` muestra el xref y el trailer que ve el parser, usando `pdf.Debug`.

## El núcleo de consola

### tui

- **Región viva**: el progreso se redibuja en el lugar a 20 cuadros por segundo y las líneas permanentes se imprimen por encima, con un orden de candados fijo para que no haya deadlock.
- **Recorte seguro**: `ClipANSI` recorta una línea con escapes a N columnas visibles sin romper los colores.
- **Renglones que no entran**: los errores (`✗`) y los avisos del pie (`!`) se parten en palabras al ancho de la ventana, con las líneas de más alineadas después de la marca (`Term.Marked`); a un pipe van enteros. Antes la consola los cortaba donde caían.
- **Barra**: resolución de 1/8 de columna (bloques `▏▎▍▌▋▊▉█`) con degradé; en Windows Terminal el avance también se publica en el ícono y la pestaña (OSC 9;4).
- **Tarjeta** sin bordes y **formato es-AR**: miles con punto, decimales con coma, bytes en unidades decimales.

### cli

- Flags al estilo GNU: `-o x`, `--out=x`, `-rf`, `--no-x`, `--`; binders tipados con validación (enteros acotados, enumerados, repetibles como `--password`).
- La ayuda se genera con el mismo lenguaje visual que el resto de la salida, al ancho real de la ventana.
- Un flag mal escrito sugiere el más parecido por distancia de Damerau–Levenshtein (`textdist`).

### fsx

- Patrones con `*`, `?`, `[]` y `**`, resueltos segmento a segmento y sin distinguir mayúsculas, como Windows. Ni cmd ni PowerShell expanden comodines para un exe nativo: lo hace la herramienta.
- Orden natural: `pag2` antes que `pag10`, comparando tramos de dígitos por valor y sin límite de largo.
- `WriteAtomic` reintenta el renombre con espera exponencial (un antivirus o un visor de PDF pueden tener el destino abierto un instante).

### batch

- Pool de workers; los resultados se guardan por índice, así el resumen es determinista aunque terminen desordenados.
- Un pánico en una tarea se convierte en un fallo de esa tarea, sin tirar abajo la tanda: un PDF patológico no se lleva puesta la corrida.

### win/desk

- El modo y el tamaño de la consola por `syscall`, con `//go:uintptrescapes` en el envoltorio de las llamadas. Solo carga `kernel32.dll`, una KnownDLL que Windows toma siempre de System32 (con otra DLL nombrada sin ruta, `LoadLibrary` buscaría primero junto al exe).
- Es chico a propósito: un test (`internal/tui/deps_test.go`) comprueba con `go list -deps` que ni `tui` ni el exe cargan `net`, `net/netip` u `os/exec`.

### version

`Version` y `Commit` son variables que `build.ps1` pisa con `-ldflags -X`: `pdf-merge --version` dice `1.0.0+abc1234`. Compilado con `go install`, dice `1.0.0`.
