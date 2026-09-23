# Casos de marcadores y enlaces para pdf-merge, verificados con qpdf (pikepdf).
# Uso: & outline_cases.ps1 -Exe <pdf-merge.exe> -Fx <carpeta de fixtures>
param(
    [Parameter(Mandatory)] [string] $Exe,
    [Parameter(Mandatory)] [string] $Fx
)
$ErrorActionPreference = 'Stop'
$here = Split-Path -Parent $MyInvocation.MyCommand.Path
$check = Join-Path $here 'outline_check.py'
python $check gen $Fx | Out-Null

function Mark([string] $t, [int] $n, $p) { [ordered]@{ titulo = $t; nivel = $n; pagina = $p } }
function Link([int] $d, $h) { [ordered]@{ desde = $d; hacia = $h } }

$cap1 = 'Capítulo 1'; $cap2 = 'Capítulo 2 · diseño'; $sec21 = 'Sección 2.1 — ñandú'; $cap3 = 'Capítulo 3'
$casos = @(
    @{
        nombre = 'dos archivos completos: nivel por archivo + originales'
        args   = @("$Fx\links.pdf", "$Fx\a.pdf")
        esperado = @{
            marcadores = @((Mark 'links' 0 1), (Mark $cap1 1 1), (Mark $cap2 1 3), (Mark $sec21 2 4), (Mark $cap3 1 5), (Mark 'a' 0 6))
            enlaces    = @((Link 1 3), (Link 2 5), (Link 3 5), (Link 4 1))
            pagemode   = '/UseOutlines'
        }
    },
    @{
        nombre = 'rango @3-5: se poda el Capítulo 1 y el enlace a la página 1 queda inerte'
        args   = @("$Fx\links.pdf@3-5", "$Fx\a.pdf")
        esperado = @{
            marcadores = @((Mark 'links' 0 1), (Mark $cap2 1 1), (Mark $sec21 2 2), (Mark $cap3 1 3), (Mark 'a' 0 4))
            enlaces    = @((Link 1 3), (Link 2 'inerte'))
            pagemode   = '/UseOutlines'
        }
    },
    @{
        nombre = 'un solo archivo: solo los marcadores originales'
        args   = @("$Fx\links.pdf")
        esperado = @{
            marcadores = @((Mark $cap1 0 1), (Mark $cap2 0 3), (Mark $sec21 1 4), (Mark $cap3 0 5))
            enlaces    = @((Link 1 3), (Link 2 5), (Link 3 5), (Link 4 1))
            pagemode   = '/UseOutlines'
        }
    },
    @{
        nombre = 'destinos CON NOMBRE (árbol /Names con /Kids, /Dests 1.1, GoTo): mismo resultado'
        args   = @("$Fx\links_named.pdf", "$Fx\a.pdf")
        esperado = @{
            marcadores = @((Mark 'links_named' 0 1), (Mark $cap1 1 1), (Mark $cap2 1 3), (Mark $sec21 2 4), (Mark $cap3 1 5), (Mark 'a' 0 6))
            enlaces    = @((Link 1 3), (Link 2 5), (Link 3 5), (Link 4 1))
            pagemode   = '/UseOutlines'
        }
    },
    @{
        nombre = 'destinos CON NOMBRE y rango @3-5'
        args   = @("$Fx\links_named.pdf@3-5", "$Fx\a.pdf")
        esperado = @{
            marcadores = @((Mark 'links_named' 0 1), (Mark $cap2 1 1), (Mark $sec21 2 2), (Mark $cap3 1 3), (Mark 'a' 0 4))
            enlaces    = @((Link 1 3), (Link 2 'inerte'))
            pagemode   = '/UseOutlines'
        }
    },
    @{
        nombre = '--bookmarks none'
        args   = @("$Fx\links.pdf", "$Fx\a.pdf", '--bookmarks', 'none')
        esperado = @{
            marcadores = @()
            enlaces    = @((Link 1 3), (Link 2 5), (Link 3 5), (Link 4 1))
            pagemode   = ''
        }
    }
)

$fallas = 0
$i = 0
foreach ($c in $casos) {
    $i++
    $out = Join-Path $Fx "outline_$i.pdf"
    $exp = Join-Path $Fx "outline_$i.json"
    if (Test-Path $out) { [IO.File]::Delete($out) }
    & $Exe @($c.args) -o $out --no-color | Out-Null
    if ($LASTEXITCODE -ne 0) { "  ✗ caso $i ($($c.nombre)): pdf-merge salió con $LASTEXITCODE"; $fallas++; continue }
    [IO.File]::WriteAllText($exp, (ConvertTo-Json $c.esperado -Depth 6), [Text.UTF8Encoding]::new($false))
    "caso ${i}: $($c.nombre)"
    python $check check $out $exp
    if ($LASTEXITCODE -ne 0) { $fallas++ }
}
""
if ($fallas -eq 0) { "  resultado: los $i casos OK" } else { "  resultado: $fallas casos con fallas" }
exit $fallas
