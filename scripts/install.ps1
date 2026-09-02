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

    or configure another MCP client by hand:

        https://jmrp.io/docs/gitlab-mcp-server/configuration/

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
    [string]$InstallDir = $(
        if ($env:INSTALL_DIR) { $env:INSTALL_DIR }
        elseif ($env:LOCALAPPDATA) { Join-Path $env:LOCALAPPDATA 'Programs\gitlab-mcp-server' }
        elseif ($env:USERPROFILE) { Join-Path $env:USERPROFILE 'AppData\Local\Programs\gitlab-mcp-server' }
        else { throw 'Cannot determine an install directory: set INSTALL_DIR, or ensure LOCALAPPDATA/USERPROFILE is defined.' }
    ),
    [string]$Repo = $(if ($env:REPO) { $env:REPO } else { 'jmrplens/gitlab-mcp-server' })
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Write-Info { param([string]$Message) Write-Host "==> $Message" -ForegroundColor Cyan }

$binName = 'gitlab-mcp-server'

# --- detect architecture ---------------------------------------------------
# PROCESSOR_ARCHITECTURE reports the *process* architecture, so a 32-bit
# PowerShell on 64-bit Windows says 'x86'. PROCESSOR_ARCHITEW6432 carries the
# real OS architecture in that case, so prefer it when present.
$procArch = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
$arch = switch ($procArch) {
    'AMD64' { 'amd64' }
    'ARM64' { 'arm64' }
    'x86' { throw 'unsupported architecture x86 (32-bit); no 32-bit build is published' }
    default { throw "unsupported architecture '$procArch'" }
}

$asset = "$binName-windows-$arch.exe"

# --- resolve download base -------------------------------------------------
# RELEASE_BASE_URL exists so the installer can be driven against a local
# fixture server in tests; leave it unset for real installs.
$base = if ($env:RELEASE_BASE_URL) {
    $env:RELEASE_BASE_URL
}
elseif ($Version -eq 'latest') {
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
    # Integrity verification is mandatory by default (fail closed). Set the
    # environment variable ALLOW_UNVERIFIED=1 to bypass, at your own risk.
    if ($env:ALLOW_UNVERIFIED -eq '1') {
        Write-Info 'WARNING: ALLOW_UNVERIFIED=1 - skipping checksum verification'
    }
    else {
        Write-Info 'verifying checksum'
        $sumsPath = Join-Path $tmp 'checksums.txt'
        try {
            Invoke-WebRequest -Uri "$base/checksums.txt" -OutFile $sumsPath -UseBasicParsing
        }
        catch {
            throw "could not fetch checksums.txt to verify the download; aborting (set ALLOW_UNVERIFIED=1 to bypass)"
        }
        # checksums.txt is fetched from the same release as the binary, so on
        # its own it only proves the two agree — whoever can replace release
        # assets replaces both. Every release also publishes a keyless cosign
        # signature over checksums.txt, and GitHub holds a build-provenance
        # attestation for the binary; verify whichever tool is present.
        $verifiedSignature = $false
        if (Get-Command cosign -ErrorAction SilentlyContinue) {
            $bundlePath = Join-Path $tmp 'checksums.txt.sigstore.json'
            $haveBundle = $true
            try {
                Invoke-WebRequest -Uri "$base/checksums.txt.sigstore.json" -OutFile $bundlePath -UseBasicParsing
            }
            catch {
                $haveBundle = $false
                Write-Info 'WARNING: this release publishes no checksums.txt.sigstore.json'
            }
            if ($haveBundle) {
                Write-Info 'verifying the cosign signature over checksums.txt'
                # /releases/latest/download never reveals the tag, so pin the
                # workflow identity and let the tag be any release version.
                $identityArgs = if ($Version -eq 'latest') {
                    @('--certificate-identity-regexp',
                        ('^https://github\.com/' + $Repo + '/\.github/workflows/release\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+$'))
                }
                else {
                    @('--certificate-identity',
                        "https://github.com/$Repo/.github/workflows/release.yml@refs/tags/$Version")
                }
                & cosign verify-blob --bundle $bundlePath @identityArgs `
                    --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' $sumsPath | Out-Null
                if ($LASTEXITCODE -ne 0) {
                    throw 'checksums.txt failed cosign verification - the release assets do not carry this project''s signature'
                }
                $verifiedSignature = $true
                Write-Info 'signature OK'
            }
        }
        elseif (Get-Command gh -ErrorAction SilentlyContinue) {
            Write-Info 'verifying the build-provenance attestation with gh'
            & gh attestation verify $assetPath --repo $Repo 2>&1 | Out-Null
            if ($LASTEXITCODE -eq 0) {
                $verifiedSignature = $true
                Write-Info 'attestation OK'
            }
            else {
                Write-Info "WARNING: gh found no valid build-provenance attestation for $asset"
            }
        }
        if (-not $verifiedSignature) {
            $msg = "no signature was verified - install cosign or the gh CLI to check that these bytes came from $Repo's release workflow"
            if ($env:REQUIRE_SIGNATURE -eq '1') { throw "$msg (REQUIRE_SIGNATURE=1)" }
            Write-Info "WARNING: $msg"
        }

        $want = (Get-Content $sumsPath |
                Where-Object { $_ -match "\s$([regex]::Escape($asset))$" } |
                ForEach-Object { ($_ -split '\s+')[0] } |
                Select-Object -First 1)
        if (-not $want) {
            throw "$asset is not listed in checksums.txt; aborting (set ALLOW_UNVERIFIED=1 to bypass)"
        }
        $got = (Get-FileHash -Algorithm SHA256 -Path $assetPath).Hash.ToLower()
        if ($want.ToLower() -ne $got) {
            throw "checksum mismatch for $asset (want $want, got $got)"
        }
        Write-Info 'checksum OK'
    }

    # --- install -----------------------------------------------------------
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    $dest = Join-Path $InstallDir "$binName.exe"
    Copy-Item -Path $assetPath -Destination $dest -Force
    Write-Info "installed $dest"

    # --- ensure on PATH (user scope) ---------------------------------------
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    $pathList = if ($userPath) { $userPath -split ';' } else { @() }
    if ($pathList -notcontains $InstallDir) {
        $newPath = if ($userPath) { "$userPath;$InstallDir" } else { $InstallDir }
        [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
        $env:Path = "$env:Path;$InstallDir"
        Write-Info "added $InstallDir to your user PATH (restart your terminal to pick it up)"
    }
}
finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}

Write-Host @"

Next steps:
  1. Register with Claude Code:
       claude mcp add gitlab --env GITLAB_TOKEN=glpat-xxxx -- $binName
     (self-managed GitLab: add  --env GITLAB_URL=https://gitlab.example.com)
  2. Or configure another MCP client by hand:
       https://jmrp.io/docs/gitlab-mcp-server/configuration/
"@
