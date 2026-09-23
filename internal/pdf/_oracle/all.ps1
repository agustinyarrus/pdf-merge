# Regresión completa de pdf-merge contra oráculos externos (qpdf, pypdf, PDFium).
# Uso: & all.ps1 [-Corpus lista.txt]   (lista de PDF reales, uno por línea)
param([string] $Corpus = (Join-Path $env:TEMP 'navaja-sweep-list.txt'))
$ErrorActionPreference = 'Continue'
$repo = Resolve-Path (Join-Path $PSScriptRoot '..\..\..')
$oracle = $PSScriptRoot
$fx = Join-Path $env:TEMP 'navaja-pdf'
$exe = Join-Path $env:TEMP 'pdf-merge.exe'

# ESC como [char]27 y no `e: así corre también en Windows PowerShell 5.1.
$e = [char]27
$sage = "$e[38;2;181;223;168m"; $rose = "$e[38;2;242;167;184m"
$sub = "$e[38;2;138;143;168m"; $lav = "$e[38;2;196;181;253m"; $bg = "$e[48;2;16;17;22m"; $x = "$e[0m"

Push-Location $repo
$env:CGO_ENABLED = '0'
go build -ldflags="-s -w" -trimpath -o $exe ./cmd/pdf-merge
if ($LASTEXITCODE -ne 0) { Pop-Location; throw 'no compila pdf-merge' }

$etapas = [ordered]@{}
function Etapa([string] $nombre, [scriptblock] $cuerpo) {
    Write-Host "`n  $lav› $nombre$x"
    $t0 = [Diagnostics.Stopwatch]::StartNew()
    & $cuerpo | ForEach-Object { Write-Host "    $_" }
    $ok = ($LASTEXITCODE -eq 0)
    $etapas[$nombre] = @{ ok = $ok; ms = [int]$t0.Elapsed.TotalMilliseconds }
}

Etapa 'tests de Go (parser, merge, dedupe, Tarjan)' { go test ./internal/pdf/ 2>&1 | Select-Object -Last 1 }
Etapa 'fixtures: rango, reverso, object streams' {
    $out = Join-Path $env:TEMP 'navaja-merged.pdf'
    & $exe "$fx\a.pdf" "$fx\c_objstm.pdf@2-4" "$fx\d.pdf" "$fx\b.pdf@reverso" -o $out --force --no-color | Out-Null
    python (Join-Path $oracle 'verify.py') $out "$fx\a.pdf:1,2,3" "$fx\c_objstm.pdf:2,3,4" "$fx\d.pdf:1,2" "$fx\b.pdf:2,1" 2>$null
}
Etapa 'marcadores y enlaces (6 casos)' { & (Join-Path $oracle 'outline_cases.ps1') -Exe $exe -Fx $fx 2>&1 | Select-Object -Last 1 }
Etapa 'cifrado (13 casos)' { python (Join-Path $oracle 'crypt_check.py') $exe $fx 2>$null | Select-Object -Last 1 }
if (Test-Path $Corpus) {
    # Los PDF del corpus con formulario, detectados por contenido (no por nombre).
    $forms = @(python (Join-Path $oracle 'list_forms.py') $Corpus 2>$null)
    if ($forms.Count -gt 0) {
        Etapa "formularios reales ($($forms.Count) PDF con AcroForm)" {
            $out = Join-Path $env:TEMP 'navaja-forms.pdf'
            & $exe @forms -o $out --force --no-color | Out-Null
            python (Join-Path $oracle 'form_check.py') $out @forms 2>$null
        }
    }
    Etapa "barrido de PDF reales ($((Get-Content $Corpus).Count))" {
        python (Join-Path $oracle 'sweep.py') $exe (Join-Path $env:TEMP 'navaja-sweep') $Corpus 2>$null | Select-Object -Last 1
    }
    Etapa 'estrés: todos en un solo PDF' {
        python (Join-Path $oracle 'bigmerge.py') $exe $Corpus (Join-Path $env:TEMP 'navaja-big.pdf') 2>$null | Select-Object -First 1
    }
}
Pop-Location

$okN = @($etapas.Values | Where-Object { $_.ok }).Count
$total = $etapas.Count
$w = 76
Write-Host ""
Write-Host "  $bg$(' ' * $w)$x"
Write-Host "  $bg   $sub$('resumen de la regresión'.PadRight($w - 3))$x"
Write-Host "  $bg$(' ' * $w)$x"
foreach ($k in $etapas.Keys) {
    $e = $etapas[$k]
    $dot = if ($e.ok) { "$sage●" } else { "$rose●" }
    $ms = $e.ms.ToString('N0', [Globalization.CultureInfo]::GetCultureInfo('es-AR'))
    $txt = "$($k.PadRight(52)) $($ms.PadLeft(7)) ms"
    Write-Host "  $bg   $dot $sub$($txt.PadRight($w - 5))$x"
}
Write-Host "  $bg$(' ' * $w)$x"
$final = if ($okN -eq $total) { "$sage$okN de $total etapas OK" } else { "$rose$($total - $okN) etapas fallaron" }
Write-Host "  $bg   $final$(' ' * ($w - 3 - "$okN de $total etapas OK".Length))$x"
Write-Host "  $bg$(' ' * $w)$x"
exit ($total - $okN)
