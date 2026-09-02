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

# --- resolve which release to install --------------------------------------
# The asset names carry no version (gitlab-mcp-server-windows-<arch>.exe) and
# neither does checksums.txt, so "latest" left unresolved lets whoever can
# replace release assets serve a consistent, correctly signed triple from an
# older, vulnerable release. Following the /releases/latest redirect names the
# tag before anything is downloaded, which is what pins the signature identity
# to the version being installed.
function Resolve-LatestTag {
    param([string]$Url)
    try {
        $response = Invoke-WebRequest -Uri $Url -UseBasicParsing -MaximumRedirection 5
        # Windows PowerShell 5.1 hands back an HttpWebResponse and PowerShell 7
        # an HttpResponseMessage; each spells "where this ended up" its own way.
        $inner = $response.BaseResponse
        $final = if ($inner.PSObject.Properties['ResponseUri']) { $inner.ResponseUri.AbsoluteUri }
        elseif ($inner.PSObject.Properties['RequestMessage']) { $inner.RequestMessage.RequestUri.AbsoluteUri }
        else { '' }
        if ($final -match '/releases/tag/(?<tag>[^/]+)$') { return $Matches['tag'] }
    }
    catch {
        return $null
    }
    return $null
}

# RELEASE_BASE_URL is the test hook and points at a local fixture with no
# /releases/latest of its own, so resolution follows it.
$latestUrl = if ($env:RELEASE_BASE_URL) { $env:RELEASE_LATEST_URL } else { "https://github.com/$Repo/releases/latest" }
if ($Version -eq 'latest' -and $latestUrl) {
    $latestTag = Resolve-LatestTag -Url $latestUrl
    if ($latestTag) {
        $Version = $latestTag
        Write-Info "latest is $Version"
    }
    else {
        Write-Info 'WARNING: could not resolve which release is latest; the signature check cannot name a version'
    }
}

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
    #
    # The two knobs pull in opposite directions, so a configuration that sets
    # both is a mistake worth naming rather than resolving silently in either
    # direction.
    if ($env:ALLOW_UNVERIFIED -eq '1' -and $env:REQUIRE_SIGNATURE -eq '1') {
        throw 'ALLOW_UNVERIFIED=1 and REQUIRE_SIGNATURE=1 contradict each other; unset one'
    }
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
        $bundleMissing = $false

        # Both verifiers are told which release they are looking at. An
        # unresolved "latest" is the only case left with no tag to name, and its
        # regexp is satisfied by any release version - which is precisely the
        # rollback this cannot otherwise see.
        if ($Version -eq 'latest') {
            $identity = '^https://github\.com/' + $Repo + '/\.github/workflows/release\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+$'
            $identityArgs = @('--certificate-identity-regexp', $identity)
            $ghIdentityArgs = @('--cert-identity-regex', $identity)
        }
        else {
            $identity = "https://github.com/$Repo/.github/workflows/release.yml@refs/tags/$Version"
            $identityArgs = @('--certificate-identity', $identity)
            $ghIdentityArgs = @('--cert-identity', $identity)
        }

        if (Get-Command cosign -ErrorAction SilentlyContinue) {
            $bundlePath = Join-Path $tmp 'checksums.txt.sigstore.json'
            try {
                Invoke-WebRequest -Uri "$base/checksums.txt.sigstore.json" -OutFile $bundlePath -UseBasicParsing
            }
            catch {
                $bundleMissing = $true
                Write-Info 'WARNING: this release publishes no checksums.txt.sigstore.json'
            }
            if (-not $bundleMissing) {
                Write-Info 'verifying the cosign signature over checksums.txt'
                & cosign verify-blob --bundle $bundlePath @identityArgs `
                    --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' $sumsPath | Out-Null
                if ($LASTEXITCODE -ne 0) {
                    throw 'checksums.txt failed cosign verification - the release assets do not carry this project''s signature'
                }
                $verifiedSignature = $true
                Write-Info 'signature OK'
            }
        }
        # Not an elseif: when the bundle is the asset that went missing, gh
        # still has something to say, because the build-provenance attestation
        # lives in GitHub's own store rather than among the release assets.
        if (-not $verifiedSignature -and (Get-Command gh -ErrorAction SilentlyContinue)) {
            Write-Info 'verifying the build-provenance attestation with gh'
            # No `2>&1` here: on Windows PowerShell 5.1 - the host the documented
            # one-liner lands in - merging a native command's stderr while
            # $ErrorActionPreference is 'Stop' turns gh's first diagnostic line
            # into a terminating error, so neither the warning below nor the
            # REQUIRE_SIGNATURE gate was reachable on a failed attestation. The
            # exit code decides, and gh's own message reaches the user.
            & gh attestation verify $assetPath --repo $Repo @ghIdentityArgs | Out-Null
            if ($LASTEXITCODE -eq 0) {
                $verifiedSignature = $true
                Write-Info 'attestation OK'
            }
            else {
                Write-Info "WARNING: gh found no valid build-provenance attestation for $asset"
            }
        }
        if (-not $verifiedSignature) {
            # Every release since signing began publishes the bundle, so on a
            # machine that can check one its absence is not an old release: it
            # is the single asset whoever replaced the binary and checksums.txt
            # also has to remove.
            if ($bundleMissing) {
                throw 'no checksums.txt.sigstore.json is served for this release and no attestation could be verified; refusing to install unverified bytes (set ALLOW_UNVERIFIED=1 to bypass at your own risk)'
            }
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
