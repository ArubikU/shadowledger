# compile-all.ps1 — build the ShadowLedger CIIS 2026 paper (modular, with bibliography).
# Requires MiKTeX (scoop) or TeX Live. Usage: .\compile-all.ps1
Set-StrictMode -Version Latest
$ErrorActionPreference = "Continue"

# Put MiKTeX (scoop) on PATH if present.
$miktexBin = "$env:USERPROFILE\scoop\apps\latex\current\texmfs\install\miktex\bin\x64"
if (Test-Path $miktexBin) { $env:PATH = "$miktexBin;$env:PATH" }
$tinytexBin = "$env:APPDATA\TinyTeX\bin\windows"
if ((Test-Path $tinytexBin) -and -not (Get-Command pdflatex -ErrorAction SilentlyContinue)) {
    $env:PATH = "$tinytexBin;$env:PATH"
}
if (-not (Get-Command pdflatex -ErrorAction SilentlyContinue)) {
    Write-Error "pdflatex not found. Install MiKTeX: scoop install latex"; exit 1
}
Write-Host "pdflatex: $((Get-Command pdflatex).Source)" -ForegroundColor Green

$root = $PSScriptRoot
$outDir = Join-Path $root "output"
if (-not (Test-Path $outDir)) { New-Item -ItemType Directory -Path $outDir | Out-Null }

# Paper lives in paper/main.tex with sections in paper/secciones/ and references.bib.
$paperDir = Join-Path $root "paper"
Push-Location $paperDir
try {
    Write-Host "`n=== Building paper (pdflatex -> bibtex -> pdflatex x2) ===" -ForegroundColor Cyan
    & pdflatex -interaction=nonstopmode -output-directory="$outDir" main.tex *> $null
    # bibtex reads the .aux in outDir and needs the .bib alongside it.
    Copy-Item (Join-Path $paperDir "references.bib") (Join-Path $outDir "references.bib") -Force
    Push-Location $outDir; & bibtex main *> $null; Pop-Location
    & pdflatex -interaction=nonstopmode -output-directory="$outDir" main.tex *> $null
    & pdflatex -interaction=nonstopmode -output-directory="$outDir" main.tex *> $null

    $pdf = Join-Path $outDir "main.pdf"
    if (Test-Path $pdf) {
        Copy-Item $pdf (Join-Path $outDir "paper_shadowledger.pdf") -Force
        $log = Get-Content (Join-Path $outDir "main.log") -Raw
        $pages = ([regex]::Match($log, "Output written.*?\((\d+) pages")).Groups[1].Value
        $uc = ([regex]::Matches($log, "Citation .* undefined")).Count
        Write-Host "  [OK] paper_shadowledger.pdf  pages=$pages  undefined-citations=$uc" -ForegroundColor Green
    } else {
        Write-Host "  [ERROR] no PDF produced; see $outDir\main.log" -ForegroundColor Red
    }
} finally { Pop-Location }

Write-Host "`n=== Done. PDFs in $outDir ===" -ForegroundColor Cyan
Get-ChildItem $outDir -Filter "*.pdf" | ForEach-Object {
    Write-Host ("  {0}  ({1:N1} KB)" -f $_.Name, ($_.Length/1KB)) -ForegroundColor White
}
