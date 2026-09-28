<#
.SYNOPSIS
  Arma (o desarma con -Limpiar) la escena de la demo de pdf-merge: una
  carpeta con PDF de prueba y el pdf-merge.exe de este repo.

.DESCRIPTION
  1. Compila pdf-merge desde este repo a demo\.escena\bin\pdf-merge.exe con
     la versión limpia (sin commit estampado): en la cabecera dice v1.0.0.
  2. Genera con generar.py, en demo\.escena\docs\obra, los PDF de la demo:
     un informe de 12 páginas con marcadores, un escaneo de 16 páginas a
     300 ppp y cuatro facturas con el mismo logo. Son de prueba, con semilla
     fija: los mismos bytes en cada corrida.
  3. Monta demo\.escena como una unidad con subst (la primera libre de X, Y,
     Z, W, V): en pantalla la carpeta es X:\docs\obra y no la ruta real de
     quien graba.

  Imprime la carpeta de los PDF. -Limpiar desmonta la unidad y borra
  demo\.escena (está en .gitignore).

  Requisitos: Go y Python 3 con
  pip install -r internal\pdf\_oracle\requirements.txt (numpy, pillow y
  reportlab son los que usa generar.py).

.EXAMPLE
  .\preparar.ps1            # arma la escena y dice dónde quedó
  .\preparar.ps1 -Limpiar   # la desarma
#>
[CmdletBinding()]
param(
    [switch] $Limpiar
)
$ErrorActionPreference = 'Stop'

$demo    = $PSScriptRoot
$repo    = Split-Path $demo -Parent
$escena  = Join-Path $demo '.escena'
$unidadF = Join-Path $escena 'unidad.txt'

function Stop-Escena {
    if (Test-Path -LiteralPath $unidadF) {
        $u = (Get-Content -LiteralPath $unidadF -Raw).Trim()
        if ($u) { subst.exe "${u}:" /d 2>$null | Out-Null }
    }
    if (Test-Path -LiteralPath $escena) { Remove-Item -LiteralPath $escena -Recurse -Force }
}

if ($Limpiar) {
    Stop-Escena
    Write-Host 'escena desarmada'
    exit 0
}

# Una escena vieja (una grabación cortada) se desarma antes.
Stop-Escena

foreach ($cmd in 'go', 'python') {
    if (-not (Get-Command $cmd -ErrorAction SilentlyContinue)) { Write-Host "falta $cmd en el PATH"; exit 1 }
}
python -c 'import numpy, PIL, reportlab' 2>$null
if ($LASTEXITCODE -ne 0) { Write-Host 'faltan módulos de Python: pip install -r internal\pdf\_oracle\requirements.txt'; exit 1 }

# 1. pdf-merge de este repo, con la versión limpia (sin +commit).
$bin = Join-Path $escena 'bin'
New-Item -ItemType Directory -Path $bin -Force | Out-Null
Push-Location $repo
$cgoAntes = $env:CGO_ENABLED
try {
    $env:CGO_ENABLED = '0'
    go build -trimpath -ldflags '-s -w' -o (Join-Path $bin 'pdf-merge.exe') .
    if ($LASTEXITCODE -ne 0) { throw 'no compiló pdf-merge' }
} finally {
    $env:CGO_ENABLED = $cgoAntes
    Pop-Location
}

# 2. Los PDF.
$obra = Join-Path $escena 'docs\obra'
New-Item -ItemType Directory -Path $obra -Force | Out-Null
python (Join-Path $demo 'generar.py') $obra
if ($LASTEXITCODE -ne 0) { throw 'generar.py falló' }

# 3. La unidad.
$libres = 'X', 'Y', 'Z', 'W', 'V' | Where-Object { -not (Test-Path "${_}:\") }
if (-not $libres) { throw 'no hay una unidad libre entre X, Y, Z, W y V para montar la escena' }
$u = @($libres)[0]
subst.exe "${u}:" $escena
if ($LASTEXITCODE -ne 0) { throw "subst ${u}: no anduvo" }
Set-Content -LiteralPath $unidadF -Value $u -Encoding ascii
"${u}:\docs\obra"
