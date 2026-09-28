# La demo de pdf-merge

`demo.gif` y `demo.webm` son una grabación de verdad, no un mockup: una terminal de 100×30 corre
pdf-merge compilado desde este repo sobre una carpeta con PDF de prueba. Lo que se ve es la salida
real de esa corrida.

| Momento | Qué se ve |
|---|---|
| 0 a 4,9 s | se escribe `pdf-merge informe.pdf@1-4 escaneo.pdf@impares facturas -o legajo.pdf`: las cuatro primeras páginas del informe, las impares del escaneo y la carpeta de facturas, en ese orden |
| 4,9 s al final | cada archivo leído (en el orden en que terminan de leerse) y la tarjeta: 6 archivos, 16 páginas, 43,8 MB que quedan en 21,7 MB, 16 objetos repetidos fusionados, 8 marcadores y el tiempo |

**La barra pasa en un cuadro.** pdf-merge lee los PDF en paralelo con la misma barra que img, y
después narra las etapas (combinar, buscar repetidos, escribir) en una línea viva. Con estos 44 MB
todo tarda unos 50 ms: la barra de lectura sale en un solo cuadro del GIF (a los 4,92 s) y las
etapas, en ninguno. No se infló la escena ni se frenó el programa para que se vea: es lo que tarda.

La salida es correcta, no solo rápida: el `legajo.pdf` de esta escena, pasado por el oráculo triple
de [`verify.py`](../internal/pdf/_oracle/verify.py), da estructura válida para qpdf, las 16 páginas
con su texto en orden para pypdf y 0 píxeles distintos contra sus originales para PDFium.

## Qué hay acá

| Archivo | Para qué |
|---|---|
| `demo.tape` | el guion de [VHS](https://github.com/charmbracelet/vhs): arma la escena fuera de cámara, graba y la desarma |
| `preparar.ps1` | arma la escena (o la desarma con `-Limpiar`): compila pdf-merge y genera los PDF |
| `generar.py` | los PDF de prueba, con reportlab |
| `demo.gif` · `demo.webm` | la grabación: 8,9 s a 25 cuadros por segundo, 1078×644, 120 KB y 96 KB |
| `demo.png` | el último cuadro, con la tarjeta (para quien pide menos movimiento) |

## Los PDF

Todos de prueba, generados por `generar.py` con semilla fija y reportlab en modo invariante (sin
fecha ni identificador al azar): los mismos bytes en cada corrida.

| Archivo | Qué es |
|---|---|
| `informe.pdf` | 12 páginas de texto, con un marcador por capítulo (de a tres páginas) |
| `escaneo.pdf` | 16 páginas "escaneadas": una imagen JPEG de papel con renglones, A4 a 300 ppp, distinta en cada página (43 MB) |
| `facturas\factura-01.pdf` … `04` | una página cada una, con el mismo logo: pdf-merge lo guarda una sola vez (los 554 KB de "repetidos fusionados") |

## Regrabar con VHS

Hace falta Windows, PowerShell 7, Go y Python con los paquetes de los oráculos
(`pip install -r internal\pdf\_oracle\requirements.txt`: `generar.py` usa numpy, pillow y
reportlab), y además:

```powershell
winget install --scope user charmbracelet.vhs tsl0922.ttyd Gyan.FFmpeg
```

**Ojo con la versión de VHS.** La 0.12.0 (la de winget al 28/9/2026) graba los cuadros pero no
genera ni el GIF ni el WebM, sin dar error: cancela el contexto de la grabación y se lo pasa a
ffmpeg. Está arreglado en la 0.12.1:

```powershell
go install github.com/charmbracelet/vhs@v0.12.1     # pide Go 1.26.7+; Go baja solo esa versión
```

VHS dibuja la terminal en un Chrome o Edge sin ventana: usa el que esté instalado, no baja
Chromium. Después, desde esta carpeta:

```powershell
Remove-Item Env:NO_COLOR -ErrorAction SilentlyContinue     # si tu entorno lo define, la demo sale sin color
vhs demo.tape                                             # demo.gif y demo.webm
ffmpeg -y -sseof -0.5 -i demo.gif -frames:v 1 -vf "split[a][b];[a]palettegen[p];[b][p]paletteuse=dither=none" demo.png
```

Tarda medio minuto. Mirá siempre el resultado antes de publicarlo: cuadros sueltos con
`ffmpeg -ss 6 -i demo.gif -frames:v 1 cuadro.png`.

### Qué hace la escena, y por qué

- **Nada de tu máquina en pantalla.** Los PDF son de prueba y la carpeta es `X:\docs\obra`, no tu
  ruta: `preparar.ps1` genera todo en `demo\.escena\docs\obra` y monta `demo\.escena` como unidad
  con `subst` (la primera libre de X, Y, Z, W, V). pdf-merge muestra los nombres tal como se los
  pasaron, relativos a la carpeta. `-Limpiar` desmonta la unidad y borra `.escena` (está en
  `.gitignore`).
- **pdf-merge de este repo.** `preparar.ps1` lo compila sin commit estampado: en la cabecera dice
  `v1.0.0`, no `v1.0.0+abc1234`.
- **El prompt es `>` y nada más**, sin sugerencias del historial (serían las de quien graba), con
  los colores de la línea de comandos en la paleta de pdf-merge. El tape los fija fuera de cámara.
- **La espera es la de verdad.** El tape no duerme un tiempo fijo después del Enter: espera a que la
  tarjeta diga `Tiempo` y recién ahí cuenta los 4 s de lectura.

### Estilo

El de las demás demos de la familia: fondo `#0b0b0f`, texto `#cdd6f4` y los acentos de
`internal/tui/color.go` como colores ANSI del tema; Cascadia Mono 16; 1078×644 px, que con 24 px de
margen dan 100×30. Tipeo de 55 a 70 ms por tecla. 25 cuadros por segundo: a 50, el navegador no
llega a capturar cada cuadro a tiempo y el video sale acelerado.

## Sin VHS

La misma sesión se puede grabar a mano, con un grabador de pantalla como
[ScreenToGif](https://www.screentogif.com) sobre una terminal de 100×30 con fondo `#0b0b0f` y
Cascadia Mono (`wt --size 100,30 pwsh -NoProfile`). Desde esta carpeta:

```powershell
$demo = $PWD.Path
$env:Path = "$demo\.escena\bin;$env:Path"
Set-Location (.\preparar.ps1)            # arma la escena y entra
pdf-merge informe.pdf@1-4 escaneo.pdf@impares facturas -o legajo.pdf
Set-Location $demo; .\preparar.ps1 -Limpiar
```

## Desarmar a mano

Si una grabación se cortó y quedó la unidad `X:`:

```powershell
.\preparar.ps1 -Limpiar
```
