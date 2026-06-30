#!/usr/bin/env pwsh
<#
.SYNOPSIS
    Download the gitlab-mcp-server binary for Windows from the latest GitHub
    Release, verify its checksum, and install it onto your PATH.

.DESCRIPTION
    Run from PowerShell:

        irm https://raw.githubusercontent.com/jmrplens/gitlab-mcp-server/main/scripts/install.ps1 | iex

    After install, register the server with Claude Code:

        claude mcp add gitlab --env GITLAB_TOKEN=glpat-xxxx -- gitlab-mcp-server

    or run the guided setup wizard:

        gitlab-mcp-server --setup

.PARAMETER Version
    Release tag to install (default: latest).

.PARAMETER InstallDir
    Target directory (default: %LOCALAPPDATA%\Programs\gitlab-mcp-server).

.PARAMETER Repo
    owner/repo (default: jmrplens/gitlab-mcp-server).
#>
[CmdletBinding()]
# Write-Host is intentional: this is an interactive installer (run via irm | iex)
# whose colored progress output is the user-facing UX, matching the rustup/deno/scoop convention.
[Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSAvoidUsingWriteHost', '', Justification = 'Interactive installer UX')]
param(
    [string]$Version = $(if ($env:VERSION) { $env:VERSION } else { 'latest' }),
    [string]$InstallDir = $(if ($env:INSTALL_DIR) { $env:INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\gitlab-mcp-server' }),
    [string]$Repo = $(if ($env:REPO) { $env:REPO } else { 'jmrplens/gitlab-mcp-server' })
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Write-Info { param([string]$Message) Write-Host "==> $Message" -ForegroundColor Cyan }

$binName = 'gitlab-mcp-server'

# --- detect architecture ---------------------------------------------------
$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    'AMD64' { 'amd64' }
    'ARM64' { 'arm64' }
    'x86' { throw 'unsupported architecture x86 (32-bit); no 32-bit build is published' }
    default { throw "unsupported architecture '$($env:PROCESSOR_ARCHITECTURE)'" }
}

$asset = "$binName-windows-$arch.exe"

# --- resolve download base -------------------------------------------------
$base = if ($Version -eq 'latest') {
    "https://github.com/$Repo/releases/latest/download"
}
else {
    "https://github.com/$Repo/releases/download/$Version"
}

$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ([System.IO.Path]::GetRandomFileName())
New-Item -ItemType Directory -Path $tmp -Force | Out-Null
try {
    $assetPath = Join-Path $tmp $asset
    Write-Info "downloading $asset ($Version)"
    Invoke-WebRequest -Uri "$base/$asset" -OutFile $assetPath -UseBasicParsing

    # --- verify checksum ---------------------------------------------------
    Write-Info 'verifying checksum'
    $sumsPath = Join-Path $tmp 'checksums.txt'
    try {
        Invoke-WebRequest -Uri "$base/checksums.txt" -OutFile $sumsPath -UseBasicParsing
        $want = (Get-Content $sumsPath |
                Where-Object { $_ -match "\s$([regex]::Escape($asset))$" } |
                ForEach-Object { ($_ -split '\s+')[0] } |
                Select-Object -First 1)
        $got = (Get-FileHash -Algorithm SHA256 -Path $assetPath).Hash.ToLower()
        if (-not $want) {
            Write-Info "WARNING: $asset not listed in checksums.txt; skipping verification"
        }
        elseif ($want.ToLower() -ne $got) {
            throw "checksum mismatch for $asset (want $want, got $got)"
        }
        else {
            Write-Info 'checksum OK'
        }
    }
    catch [System.Net.WebException] {
        Write-Info 'WARNING: could not fetch checksums.txt; skipping verification'
    }

    # --- install -----------------------------------------------------------
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    $dest = Join-Path $InstallDir "$binName.exe"
    Copy-Item -Path $assetPath -Destination $dest -Force
    Write-Info "installed $dest"

    # --- ensure on PATH (user scope) ---------------------------------------
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if (($userPath -split ';') -notcontains $InstallDir) {
        [Environment]::SetEnvironmentVariable('Path', "$userPath;$InstallDir", 'User')
        $env:Path = "$env:Path;$InstallDir"
        Write-Info "added $InstallDir to your user PATH (restart your terminal to pick it up)"
    }
}
finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}

Write-Host @"

Next steps:
  1. Guided setup (collects your GitLab token, configures your MCP client):
       $binName --setup
  2. Or register with Claude Code directly:
       claude mcp add gitlab --env GITLAB_TOKEN=glpat-xxxx -- $binName
     (self-managed GitLab: add  --env GITLAB_URL=https://gitlab.example.com)
"@
