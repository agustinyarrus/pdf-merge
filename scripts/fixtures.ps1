<#
.SYNOPSIS
  Genera los PDF de prueba de pdf-merge (los que leen los tests de internal/pdf
  y los oráculos de internal/pdf/_oracle).

.DESCRIPTION
  Los tests de internal/pdf leen la carpeta de $env:PDF_FIXTURES o, si no está
  definida, %TEMP%\pdf-merge-fx. Si no existe, esos tests se saltean sin fallar;
  este script la llena:
    a.pdf  3 páginas A4      b.pdf  2 páginas carta      c.pdf  5 páginas A4
    d.pdf  2 páginas, una con imagen (streams binarios y recursos)
    c_objstm.pdf   c.pdf reescrito con object streams y xref stream (PDF 1.5+)
    links.pdf y links_named.pdf: marcadores anidados y enlaces por nombre
                   (los genera internal\pdf\_oracle\outline_check.py)

  Requisito: Python 3 con pip install pillow pikepdf reportlab.

.EXAMPLE
  .\scripts\fixtures.ps1                   # a %TEMP%\pdf-merge-fx
  .\scripts\fixtures.ps1 -Dir D:\fx\pdf    # a otra carpeta; los tests la leen con:
  $env:PDF_FIXTURES = 'D:\fx\pdf'; go test ./internal/pdf/
#>
[CmdletBinding()]
param([string] $Dir)
$ErrorActionPreference = 'Stop'

$e = [char]27
function Ok([string] $s) { Write-Host "  $e[38;2;181;223;168m✓$e[0m $s" }
function Fail([string] $s) { Write-Host "  $e[38;2;242;167;184m✗$e[0m $s"; exit 1 }

$default = Join-Path ([IO.Path]::GetTempPath()) 'pdf-merge-fx'
if (-not $Dir) {
    $Dir = if ($env:PDF_FIXTURES) { $env:PDF_FIXTURES } else { $default }
}
if (-not (Get-Command python -ErrorAction SilentlyContinue)) { Fail 'falta Python 3' }
python -c "import PIL, pikepdf, reportlab" 2>$null
if ($LASTEXITCODE -ne 0) { Fail 'faltan módulos de Python: pip install pillow pikepdf reportlab' }

New-Item -ItemType Directory -Force $Dir | Out-Null
python (Join-Path $PSScriptRoot 'fixtures_pdf.py') $Dir
if ($LASTEXITCODE -ne 0) { Fail 'no pude generar los PDF de prueba' }
python (Join-Path $PSScriptRoot '..\internal\pdf\_oracle\outline_check.py') gen $Dir | Out-Null
if ($LASTEXITCODE -ne 0) { Fail 'no pude generar los PDF con marcadores' }
Ok "PDF en $Dir ($((Get-ChildItem $Dir -Filter *.pdf).Count) archivos)"
if ($Dir -ne $default -and $env:PDF_FIXTURES -ne $Dir) {
    Write-Host "  para los tests: `$env:PDF_FIXTURES = '$Dir'"
}
