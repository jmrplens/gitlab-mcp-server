# build-release.ps1 — Cross-compile release binaries for all platforms.
# Usage: powershell -ExecutionPolicy Bypass -File scripts/build-release.ps1
# Called by: make release (on Windows)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$BinaryName = "gitlab-mcp-server"
$CmdPath = "./cmd/server"
$Version = (Get-Content -Path "VERSION" -Raw).Trim()
$Commit = git rev-parse --short HEAD 2>$null
if (-not $Commit) { $Commit = "none" }

$LdFlags = "-s -w -X main.version=$Version -X main.commit=$Commit"
$OutDir = "dist"

$Targets = @(
    @{ GOOS = "linux";   GOARCH = "amd64"; Ext = "" },
    @{ GOOS = "linux";   GOARCH = "arm64"; Ext = "" },
    @{ GOOS = "windows"; GOARCH = "amd64"; Ext = ".exe" },
    @{ GOOS = "windows"; GOARCH = "arm64"; Ext = ".exe" },
    @{ GOOS = "darwin";  GOARCH = "amd64"; Ext = "" },
    @{ GOOS = "darwin";  GOARCH = "arm64"; Ext = "" }
)

Write-Host "=== Building release v$Version (commit $Commit) ===" -ForegroundColor Cyan
Write-Host "Output directory: $OutDir"
Write-Host ""

if (Test-Path $OutDir) {
    Remove-Item -Recurse -Force $OutDir
}
New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

$env:CGO_ENABLED = "0"
$failed = 0

foreach ($t in $Targets) {
    $outFile = "$BinaryName-$($t.GOOS)-$($t.GOARCH)$($t.Ext)"
    $outPath = "$OutDir/$outFile"
    Write-Host "  Building $outFile ..." -NoNewline

    $env:GOOS = $t.GOOS
    $env:GOARCH = $t.GOARCH

    go build -ldflags="$LdFlags" -o $outPath $CmdPath 2>&1
    if ($LASTEXITCODE -ne 0) {
        Write-Host " FAILED" -ForegroundColor Red
        $failed++
    } else {
        $size = [math]::Round((Get-Item $outPath).Length / 1MB, 1)
        Write-Host " OK (${size} MB)" -ForegroundColor Green
    }
}

# Reset environment
Remove-Item Env:GOOS -ErrorAction SilentlyContinue
Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue

# Generate SHA256 checksums
Write-Host ""
Write-Host "=== Generating checksums ===" -ForegroundColor Cyan
$checksumFile = "$OutDir/checksums.txt"
$binaries = Get-ChildItem -Path $OutDir -File | Where-Object { $_.Name -ne "checksums.txt" }
$checksums = @()
foreach ($bin in $binaries) {
    $hash = (Get-FileHash -Path $bin.FullName -Algorithm SHA256).Hash.ToLower()
    $checksums += "$hash  $($bin.Name)"
}
$checksums | Out-File -FilePath $checksumFile -Encoding utf8

Write-Host "Checksums written to $checksumFile"

Write-Host ""

# Summary
$total = $Targets.Count
$ok = $total - $failed
Write-Host "=== Release build complete ===" -ForegroundColor Cyan
Write-Host "  Version : v$Version"
Write-Host "  Commit  : $Commit"
Write-Host "  Binaries: $ok/$total succeeded"
Write-Host "  Output  : $OutDir/"
Write-Host ""

if ($failed -gt 0) {
    Write-Host "$failed build(s) failed!" -ForegroundColor Red
    exit 1
}

Get-Content $checksumFile
