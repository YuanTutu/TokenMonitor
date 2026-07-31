# ============================================================
# TokenMonitor - Build Script
# ============================================================
# Usage:
#   powershell -ExecutionPolicy Bypass -File .\build.ps1
#   powershell -ExecutionPolicy Bypass -File .\build.ps1 -Version 0.2.0
#
# Default version 0.1.0, build date = today (YYYYMMDD).
# Output goes to dist/ and is zipped for release.
# ASCII-only to avoid PowerShell 5.1 encoding issues.
# ============================================================

param(
    [string]$Version = "0.1.0",
    [string]$BuildDate = (Get-Date -Format "yyyyMMdd")
)

$ErrorActionPreference = "Stop"

$VersionTag = "v$Version"
$DistDir    = "$PSScriptRoot\dist"
$ExeName    = "tokenmonitor.exe"
$ZipName    = "TokenMonitor-$VersionTag-windows-amd64.zip"

Write-Host "=== TokenMonitor Build ===" -ForegroundColor Cyan
Write-Host "Version:     $VersionTag" -ForegroundColor Yellow
Write-Host "Build date:  $BuildDate" -ForegroundColor Yellow
Write-Host "Target:      windows/amd64" -ForegroundColor Yellow

# 1. Clean and create dist dir
Write-Host "`n[1/4] Prepare output dir..." -ForegroundColor Yellow
if (Test-Path $DistDir) { Remove-Item -Recurse -Force $DistDir }
New-Item -ItemType Directory -Path $DistDir | Out-Null
Write-Host "OK: $DistDir"

# 2. Cross-compile windows/amd64 with ldflags injecting version + date
Write-Host "`n[2/4] Compile (GOOS=windows GOARCH=amd64)..." -ForegroundColor Yellow
$env:GOOS   = "windows"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "0"

$ldflags = "-s -w -X main.version=$VersionTag -X main.buildDate=$BuildDate"
$output  = "$DistDir\$ExeName"

go build -ldflags $ldflags -o $output .
if ($LASTEXITCODE -ne 0) {
    Write-Host "FAIL: build failed" -ForegroundColor Red
    exit 1
}
Write-Host "OK: $output"

# 3. Verify version injection
Write-Host "`n[3/4] Verify version injection..." -ForegroundColor Yellow
$versionOut = (& $output -v)
Write-Host "  $versionOut"
$expected = "TokenMonitor $VersionTag build $BuildDate"
if ($versionOut -ne $expected) {
    Write-Host "FAIL: version mismatch" -ForegroundColor Red
    Write-Host "  expected: $expected" -ForegroundColor Red
    Write-Host "  actual:   $versionOut" -ForegroundColor Red
    exit 1
}
Write-Host "OK: version injected" -ForegroundColor Green

# 4. Zip
Write-Host "`n[4/4] Zip..." -ForegroundColor Yellow
Copy-Item "$PSScriptRoot\README.md" "$DistDir\README.md" -ErrorAction SilentlyContinue

$zipPath = "$PSScriptRoot\$ZipName"
if (Test-Path $zipPath) { Remove-Item -Force $zipPath }
Compress-Archive -Path "$DistDir\$ExeName", "$DistDir\README.md" -DestinationPath $zipPath
Write-Host "OK: $zipPath" -ForegroundColor Green

Write-Host "`n=== Build Done ===" -ForegroundColor Green
Write-Host "Exe:      $output" -ForegroundColor Cyan
Write-Host "Zip:      $zipPath" -ForegroundColor Cyan
Write-Host "Version:  $VersionTag build $BuildDate" -ForegroundColor Cyan
