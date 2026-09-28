# demo.tape: la demo del README de pdf-merge, grabada de verdad con VHS.
#
#   cd demo
#   vhs demo.tape            # escribe demo.gif y demo.webm
#
# Hace falta VHS 0.12.1 o más nuevo (la 0.12.0 no llega a llamar a ffmpeg),
# ttyd, ffmpeg, Go y Python con reportlab. Todo lo que se ve corre de verdad:
# preparar.ps1 compila pdf-merge desde este repo, genera los PDF de prueba con
# generar.py y monta la carpeta como X:\docs\obra. Al final se desarma sola.
# Detalles: LEEME.md.

Output demo.gif
Output demo.webm

Require pwsh
Require go
Require python

Set Shell "pwsh"
Set FontFamily "Cascadia Mono"
Set FontSize 16
Set Width 1078
Set Height 644
Set Padding 24
# 1078x644 con Cascadia Mono 16 y 24 de margen: 100 columnas x 30 filas.
Set Framerate 25
Set CursorBlink false
Set TypingSpeed 80ms
Set WaitTimeout 90s
# La paleta de pdf-merge (internal/tui/color.go) sobre #0b0b0f.
Set Theme { "name": "pdf-merge", "background": "#0b0b0f", "foreground": "#cdd6f4", "cursor": "#c4b5fd", "selection": "#2c2e38", "black": "#2c2e38", "red": "#f2a7b8", "green": "#b5dfa8", "yellow": "#eedfb8", "blue": "#c4b5fd", "magenta": "#f3b9d2", "cyan": "#8fd6cc", "white": "#cdd6f4", "brightBlack": "#55586b", "brightRed": "#f2a7b8", "brightGreen": "#b5dfa8", "brightYellow": "#eedfb8", "brightBlue": "#c4b5fd", "brightMagenta": "#f3b9d2", "brightCyan": "#8fd6cc", "brightWhite": "#ffffff" }

# Fuera de cámara: la escena, un prompt ">" limpio, sin sugerencias del
# historial (serían del que graba) y los colores de la línea de comandos en
# la misma paleta.
Hide
Type@5ms "$demo = $PWD.Path; $obra = ./preparar.ps1; $env:Path = (Join-Path $demo '.escena\bin') + ';' + $env:Path; Set-Location $obra"
Enter
Wait
Type@5ms 'Set-PSReadLineOption -PredictionSource None -Colors @{ Command = "`e[38;2;196;181;253m"; Parameter = "`e[38;2;138;143;168m"; String = "`e[38;2;143;214;204m"; Number = "`e[38;2;246;192;160m"; Operator = "`e[38;2;138;143;168m"; Default = "`e[38;2;205;214;244m" }; function prompt { "`e[38;2;196;181;253m>`e[0m " }; Clear-Host'
Enter
Wait
Show

Sleep 400ms

# Las primeras cuatro páginas del informe, las impares del escaneo y la
# carpeta de facturas, en un solo PDF: cada archivo leído y la tarjeta.
# Con estos 44 MB pdf-merge tarda unos 50 ms: la barra de lectura pasa en
# un cuadro.
Type@70ms "pdf-merge"
Type@60ms " informe.pdf@1-4"
Type@55ms " escaneo.pdf@impares"
Type@60ms " facturas"
Type@65ms " -o legajo.pdf"
Sleep 150ms
Enter
Wait+Screen@20s /Tiempo/
Sleep 4s

# Fuera de cámara: se desarma la escena.
Hide
Type@5ms "Set-Location $demo; ./preparar.ps1 -Limpiar"
Enter
Wait
