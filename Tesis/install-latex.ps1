# install-latex.ps1
# Instala TinyTeX (distribución LaTeX ligera) y los paquetes necesarios para compilar el proyecto AVE
# Ejecutar como: .\install-latex.ps1

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

Write-Host "=== Instalador de TinyTeX para Proyecto AVE ===" -ForegroundColor Cyan

# 1. Descargar e instalar TinyTeX
Write-Host "`n[1/4] Descargando TinyTeX..." -ForegroundColor Yellow
$installer = "$env:TEMP\install-tinytex.ps1"
Invoke-WebRequest -Uri "https://yihui.org/tinytex/install-windows.ps1" -OutFile $installer
. $installer

# 2. Agregar TinyTeX al PATH de esta sesión
$tinytexBin = "$env:APPDATA\TinyTeX\bin\windows"
$env:PATH = "$tinytexBin;$env:PATH"

# Verificar instalación
if (-not (Get-Command pdflatex -ErrorAction SilentlyContinue)) {
    Write-Error "TinyTeX no se instaló correctamente. Verifique la instalación manualmente."
    exit 1
}
Write-Host "TinyTeX instalado correctamente en: $tinytexBin" -ForegroundColor Green

# 3. Instalar paquetes LaTeX necesarios
Write-Host "`n[2/4] Instalando paquetes LaTeX requeridos..." -ForegroundColor Yellow
$packages = @(
    "babel-spanish",
    "booktabs",
    "multirow",
    "enumitem",
    "hyperref",
    "cite",
    "float",
    "caption",
    "amsmath",
    "amssymb",
    "graphicx",
    "geometry",
    "mathptmx",
    "authblk",
    "abstract",
    "algorithm2e",
    "pgfplots",
    "subfig",
    "longtable"
)

foreach ($pkg in $packages) {
    Write-Host "  Instalando: $pkg" -NoNewline
    & tlmgr install $pkg 2>$null
    Write-Host " [OK]" -ForegroundColor Green
}

Write-Host "`n[3/4] TinyTeX y paquetes instalados correctamente." -ForegroundColor Green
Write-Host "[4/4] Ejecute 'compile-all.ps1' para compilar todos los documentos." -ForegroundColor Cyan
