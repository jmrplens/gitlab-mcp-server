# docker-publish.ps1 — Build and push Docker image to GitHub Container Registry.
# Usage: powershell -ExecutionPolicy Bypass -File scripts/docker-publish.ps1 [-Registry <url>]
#
# Parameters:
#   -Registry   Full container registry URL. Default: reads from DOCKER_REGISTRY env var.
#
# Environment variables:
#   DOCKER_REGISTRY   Override the container registry URL
#   GITHUB_USER       GitHub username for registry login
#   GITHUB_TOKEN      GitHub token for registry login

param(
    [string]$Registry
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

# Load .env if present (supports both project root and scripts/ invocation)
$EnvFile = Join-Path $PSScriptRoot "..\.env"
if (Test-Path $EnvFile) {
    Get-Content $EnvFile | ForEach-Object {
        if ($_ -match '^\s*([A-Z_][A-Z0-9_]*)\s*=\s*(.*)$' -and $_ -notmatch '^\s*#') {
            [Environment]::SetEnvironmentVariable($Matches[1], $Matches[2], "Process")
        }
    }
}

$Version = (Get-Content -Path "VERSION" -Raw).Trim()
$Commit = git rev-parse --short HEAD 2>$null
if (-not $Commit) { $Commit = "none" }

# Determine registry URL
if (-not $Registry) {
    $Registry = $env:DOCKER_REGISTRY
}
if (-not $Registry) {
    Write-Host "Error: No registry URL provided." -ForegroundColor Red
    Write-Host "Usage: .\scripts\docker-publish.ps1 -Registry <registry_url>"
    Write-Host "   or: `$env:DOCKER_REGISTRY = 'registry.example.com/group/project'; .\scripts\docker-publish.ps1"
    exit 1
}

Write-Host "=== Docker Publish v$Version (commit $Commit) ===" -ForegroundColor Cyan
Write-Host "Registry: $Registry"
Write-Host ""

# Login to registry
$RegistryHost = ($Registry -split "/")[0]
$User = if ($env:GITHUB_USER) { $env:GITHUB_USER } else { $env:CI_REGISTRY_USER }
$Pass = if ($env:GITHUB_TOKEN) { $env:GITHUB_TOKEN } else { $env:CI_REGISTRY_PASSWORD }

if ($User -and $Pass) {
    Write-Host "Logging in to $RegistryHost..."
    $Pass | docker login $RegistryHost -u $User --password-stdin
    Write-Host ""
}

# Build image
Write-Host "Building image..." -ForegroundColor Cyan
$env:DOCKER_BUILDKIT = '1'
docker build `
    --build-arg "VERSION=$Version" `
    --build-arg "COMMIT=$Commit" `
    -t "${Registry}:${Version}" `
    -t "${Registry}:latest" `
    .

if ($LASTEXITCODE -ne 0) {
    Write-Host "Docker build failed" -ForegroundColor Red
    exit 1
}

Write-Host ""

# Push images
Write-Host "Pushing ${Registry}:${Version}..." -ForegroundColor Cyan
docker push "${Registry}:${Version}"
if ($LASTEXITCODE -ne 0) {
    Write-Host "Push failed" -ForegroundColor Red
    exit 1
}

Write-Host "Pushing ${Registry}:latest..." -ForegroundColor Cyan
docker push "${Registry}:latest"
if ($LASTEXITCODE -ne 0) {
    Write-Host "Push failed" -ForegroundColor Red
    exit 1
}

Write-Host ""
Write-Host "=== Done ===" -ForegroundColor Green
Write-Host "  ${Registry}:${Version}"
Write-Host "  ${Registry}:latest"
