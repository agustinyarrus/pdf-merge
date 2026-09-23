<#
.SYNOPSIS
  Regenera los fixtures de prueba en %TEMP%, para correr la suite en cualquier PC.

.DESCRIPTION
  Los tests de imaging y pdf leen archivos de %TEMP%\navaja-fx y %TEMP%\navaja-pdf
  (si no están, esos tests se saltean sin fallar). Este script los recrea:
    navaja-fx   imágenes: webp con y sin pérdida, webp con alfa, gif estático y
                animado, jpg, bmp (generadas con ffmpeg)
    navaja-pdf  PDF: clásicos, con object streams, con imagen, con marcadores y
                enlaces por nombre (reportlab + pikepdf)

  Requisitos: ffmpeg (winget install Gyan.FFmpeg) y Python con
  pip install pillow pikepdf reportlab

.EXAMPLE
  .\scripts\fixtures.ps1
#>
[CmdletBinding()]
param([string] $Root = $env:TEMP)
$ErrorActionPreference = 'Stop'

$e = [char]27
function Ok([string] $s) { Write-Host "  $e[38;2;181;223;168m✓$e[0m $s" }
function Fail([string] $s) { Write-Host "  $e[38;2;242;167;184m✗$e[0m $s"; exit 1 }

if (-not (Get-Command ffmpeg -ErrorAction SilentlyContinue)) { Fail 'falta ffmpeg (winget install Gyan.FFmpeg)' }
if (-not (Get-Command python -ErrorAction SilentlyContinue)) { Fail 'falta Python 3' }
python -c "import PIL, pikepdf, reportlab" 2>$null
if ($LASTEXITCODE -ne 0) { Fail 'faltan módulos de Python: pip install pillow pikepdf reportlab' }

# Imágenes ---------------------------------------------------------------------
$fx = Join-Path $Root 'navaja-fx'
New-Item -ItemType Directory -Force $fx | Out-Null
Push-Location $fx
try {
    $jobs = @(
        @('-f', 'lavfi', '-i', 'gradients=s=320x200:c0=0x8fd6cc:c1=0xc4b5fd:d=1', '-frames:v', '1', 'base.png'),
        @('-i', 'base.png', '-c:v', 'libwebp', '-lossless', '1', 'lossless.webp'),
        @('-i', 'base.png', '-c:v', 'libwebp', '-q:v', '80', 'lossy.webp'),
        @('-f', 'lavfi', '-i', 'color=c=0x8fd6cc@0.5:s=160x120,format=rgba', '-frames:v', '1', 'alpha.png'),
        @('-i', 'alpha.png', '-c:v', 'libwebp', '-lossless', '1', 'alpha.webp'),
        @('-i', 'base.png', '-frames:v', '1', 'static.gif'),
        @('-f', 'lavfi', '-i', 'testsrc=s=120x120:d=1:r=8', 'anim.gif'),
        @('-i', 'base.png', 'q.jpg'),
        @('-i', 'base.png', 'b.bmp')
    )
    foreach ($j in $jobs) {
        ffmpeg -y -loglevel error @j
        if ($LASTEXITCODE -ne 0) { Fail "ffmpeg falló generando $($j[-1])" }
    }
} finally { Pop-Location }
Ok "imágenes en $fx ($((Get-ChildItem $fx).Count) archivos)"

# PDF --------------------------------------------------------------------------
$pdf = Join-Path $Root 'navaja-pdf'
New-Item -ItemType Directory -Force $pdf | Out-Null
python (Join-Path $PSScriptRoot 'fixtures_pdf.py') $pdf
if ($LASTEXITCODE -ne 0) { Fail 'no pude generar los PDF de prueba' }
python (Join-Path $PSScriptRoot '..\internal\pdf\_oracle\outline_check.py') gen $pdf | Out-Null
if ($LASTEXITCODE -ne 0) { Fail 'no pude generar los PDF con marcadores' }
Ok "PDF en $pdf ($((Get-ChildItem $pdf -Filter *.pdf).Count) archivos)"
