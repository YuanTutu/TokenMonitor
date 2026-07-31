# ============================================================
# TokenMonitor - Build Script (Wails)
# ============================================================
# Usage:
#   powershell -ExecutionPolicy Bypass -File .\build.ps1
#   powershell -ExecutionPolicy Bypass -File .\build.ps1 -Version 1.0.0
#
# Default version 1.0.0, build date = today (YYYYMMDD).
# Output: build/bin/tokenmonitor.exe (single exe, frontend embedded).
# ASCII-only to avoid PowerShell 5.1 encoding issues.
# ============================================================

param(
    [string]$Version = "1.0.0",
    [string]$BuildDate = (Get-Date -Format "yyyyMMdd"),
    [string]$ExeName = "tokenmonitor"
)

$ErrorActionPreference = "Stop"

$VersionTag = "v$Version"
$BuildDir   = "$PSScriptRoot\build\bin"
$ExePath    = "$BuildDir\$ExeName.exe"
$ZipName    = "TokenMonitor-$VersionTag-windows-amd64.zip"

Write-Host "=== TokenMonitor Build (Wails) ===" -ForegroundColor Cyan
Write-Host "Version:     $VersionTag" -ForegroundColor Yellow
Write-Host "Build date:  $BuildDate" -ForegroundColor Yellow
Write-Host "Target:      windows/amd64" -ForegroundColor Yellow

# 1. Build via wails (compiles frontend + Go, embeds assets)
Write-Host "`n[1/4] Wails build..." -ForegroundColor Yellow
$ldflags = "-X main.version=$VersionTag -X main.buildDate=$BuildDate"
# 直接调用（不用管道，避免 $LASTEXITCODE 被管道覆盖）
& wails build -ldflags $ldflags -o "$ExeName.exe" 2>&1 | ForEach-Object {
    if ($_ -match "Built '|Compiling application|Compiling frontend|Done|FAIL|Error") { Write-Host "  $_" }
}
# wails outputs to build/bin/<name>.exe；用产物存在性判断成功（管道会干扰 $LASTEXITCODE）
$builtExe = "$BuildDir\$ExeName.exe"
if (-not (Test-Path $builtExe)) {
    Write-Host "FAIL: wails build failed (exe not found at $builtExe)" -ForegroundColor Red
    exit 1
}
Write-Host "OK: $builtExe"

# 2. Verify version injection
Write-Host "`n[2/4] Verify version injection..." -ForegroundColor Yellow
$versionOut = (& $builtExe -v)
Write-Host "  $versionOut"
$expected = "TokenMonitor $VersionTag build $BuildDate"
if ($versionOut -ne $expected) {
    Write-Host "FAIL: version mismatch" -ForegroundColor Red
    Write-Host "  expected: $expected" -ForegroundColor Red
    Write-Host "  actual:   $versionOut" -ForegroundColor Red
    exit 1
}
Write-Host "OK: version injected" -ForegroundColor Green

# 3. Copy README into build dir for zipping
Write-Host "`n[3/4] Prepare zip contents..." -ForegroundColor Yellow
Copy-Item "$PSScriptRoot\README.md" "$BuildDir\README.md" -ErrorAction SilentlyContinue
Write-Host "OK: README copied"

# 4. Zip
Write-Host "`n[4/4] Zip..." -ForegroundColor Yellow
$zipPath = "$PSScriptRoot\$ZipName"
if (Test-Path $zipPath) { Remove-Item -Force $zipPath }
Compress-Archive -Path $builtExe, "$BuildDir\README.md" -DestinationPath $zipPath
Write-Host "OK: $zipPath" -ForegroundColor Green

Write-Host "`n=== Build Done ===" -ForegroundColor Green
Write-Host "Exe:      $builtExe" -ForegroundColor Cyan
Write-Host "Zip:      $zipPath" -ForegroundColor Cyan
Write-Host "Version:  $VersionTag build $BuildDate" -ForegroundColor Cyan
