#!/usr/bin/env pwsh
<#
.SYNOPSIS
    Download the gitlab-mcp-server binary for Windows from the latest GitHub
    Release, verify its checksum, and install it onto your PATH.

.DESCRIPTION
    Run from PowerShell:

        irm https://raw.githubusercontent.com/jmrplens/gitlab-mcp-server/main/scripts/install.ps1 | iex

    After install, put the token where the server reads it and register with Claude Code:

        Set-Content -Path "$env:USERPROFILE\.gitlab-mcp-server.env" -Value 'GITLAB_TOKEN=glpat-xxxx'
        claude mcp add gitlab -- gitlab-mcp-server

    or configure another MCP client by hand:

        https://jmrp.io/docs/gitlab-mcp-server/configuration/

.PARAMETER Version
    Release tag to install (default: latest).

.PARAMETER InstallDir
    Target directory (default: %LOCALAPPDATA%\Programs\gitlab-mcp-server).

.PARAMETER Repo
    owner/repo (default: jmrplens/gitlab-mcp-server).

.NOTES
    Environment overrides, matching scripts/install.sh:

        REQUIRE_SIGNATURE   set to 1 to abort when no signature can be verified
        ALLOW_UNVERIFIED    set to 1 to skip verification entirely (not recommended;
                            refused when REQUIRE_SIGNATURE is also 1)
        FETCH_ATTEMPTS      tries per download before giving up (default: 3)
        FETCH_RETRY_DELAY   seconds between those tries (default: 2)
        RELEASE_BASE_URL    override the release download base (used by the tests)
        RELEASE_LATEST_URL  override the /releases/latest URL (used by the tests)
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

# PowerShell 7.4 made a native command's non-zero exit a terminating error
# whenever $ErrorActionPreference is 'Stop'. Every verifier below is consulted
# for its exit code instead - "cosign says no", "gh cannot reach GitHub" and
# "gh says no" are three decisions this script has to tell apart, and an
# exception unwinding past them collapses all three into one abort. Setting it
# here scopes it to this script, and it is harmless on Windows PowerShell 5.1,
# which has no such variable to read.
$PSNativeCommandUseErrorActionPreference = $false

function Write-Info { param([string]$Message) Write-Host "==> $Message" -ForegroundColor Cyan }

$binName = 'gitlab-mcp-server'

$fetchAttempts = if ($env:FETCH_ATTEMPTS) { [int]$env:FETCH_ATTEMPTS } else { 3 }
$fetchRetryDelay = if ($env:FETCH_RETRY_DELAY) { [int]$env:FETCH_RETRY_DELAY } else { 2 }

# Get-ReleaseAsset downloads one release asset and reports which of three
# things happened:
#   ok           the bytes are at OutFile
#   absent       the server answered, and this asset is not published
#   unavailable  no usable answer after $fetchAttempts tries
#
# The distinction is the point. "This release publishes no signature" and "the
# server did not answer" are different facts, and only the first one may be
# fatal: a proxy that returns 503 for one asset must not read as the one asset
# an attacker has to remove. Only the second is retried, because a server that
# answered 404 has already told us what we asked.
function Get-ReleaseAsset {
    param([string]$Url, [string]$OutFile)
    for ($attempt = 1; ; $attempt++) {
        try {
            Invoke-WebRequest -Uri $Url -OutFile $OutFile -UseBasicParsing
            return 'ok'
        }
        catch {
            # Windows PowerShell 5.1 throws a WebException and PowerShell 7 an
            # HttpResponseException; both carry the response, and a connection
            # that never got one carries nothing, which is exactly the case
            # worth retrying.
            $status = 0
            $exception = $_.Exception
            if ($exception.PSObject.Properties['Response'] -and $exception.Response) {
                if ($exception.Response.PSObject.Properties['StatusCode']) {
                    $status = [int]$exception.Response.StatusCode
                }
            }
            if ($status -eq 404 -or $status -eq 410) { return 'absent' }
            if ($attempt -ge $fetchAttempts) { return 'unavailable' }
            $seen = if ($status) { $status } else { 'no response' }
            $name = $Url.Substring($Url.LastIndexOf('/') + 1)
            Write-Info "${name}: no answer ($seen); retrying in ${fetchRetryDelay}s (attempt $attempt of $fetchAttempts)"
            Start-Sleep -Seconds $fetchRetryDelay
        }
    }
}

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
    $outcome = Get-ReleaseAsset -Url "$base/$asset" -OutFile $assetPath
    if ($outcome -eq 'absent') { throw "release $Version publishes no ${asset}: $base/$asset" }
    if ($outcome -ne 'ok') { throw "download failed after $fetchAttempts attempts: $base/$asset" }

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
        $outcome = Get-ReleaseAsset -Url "$base/checksums.txt" -OutFile $sumsPath
        if ($outcome -ne 'ok') {
            throw "could not fetch checksums.txt to verify the download ($outcome); aborting (set ALLOW_UNVERIFIED=1 to bypass)"
        }
        # checksums.txt is fetched from the same release as the binary, so on
        # its own it only proves the two agree — whoever can replace release
        # assets replaces both. Every release also publishes a keyless cosign
        # signature over checksums.txt, and GitHub holds a build-provenance
        # attestation for the binary; verify whichever tool is present.
        $verifiedSignature = $false
        $bundleMissing = $false
        $bundleUnreachable = $false

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
            $outcome = Get-ReleaseAsset -Url "$base/checksums.txt.sigstore.json" -OutFile $bundlePath
            if ($outcome -eq 'ok') {
                Write-Info 'verifying the cosign signature over checksums.txt'
                & cosign verify-blob --bundle $bundlePath @identityArgs `
                    --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' $sumsPath | Out-Null
                if ($LASTEXITCODE -ne 0) {
                    throw 'checksums.txt failed cosign verification - the release assets do not carry this project''s signature'
                }
                $verifiedSignature = $true
                Write-Info 'signature OK'
            }
            elseif ($outcome -eq 'absent') {
                $bundleMissing = $true
                Write-Info 'WARNING: this release publishes no checksums.txt.sigstore.json'
            }
            else {
                # The server never said whether the bundle is there, so neither
                # can this. Calling an unanswered request "the release has no
                # signature" would abort an install that has nothing wrong with
                # it, and would hide the case where that claim is true.
                $bundleUnreachable = $true
                Write-Info "WARNING: checksums.txt.sigstore.json could not be fetched after $fetchAttempts attempts; whether this release is signed is unknown"
            }
        }
        # Not an elseif: when the bundle is the asset that went missing, gh
        # still has something to say, because the build-provenance attestation
        # lives in GitHub's own store rather than among the release assets.
        if (-not $verifiedSignature -and (Get-Command gh -ErrorAction SilentlyContinue)) {
            # No `2>&1` on any gh call here: on Windows PowerShell 5.1 - the host
            # the documented one-liner lands in - merging a native command's
            # stderr while $ErrorActionPreference is 'Stop' turns gh's first
            # diagnostic line into a terminating error, so neither the warning
            # below nor the REQUIRE_SIGNATURE gate was reachable on a failed
            # attestation. The exit code decides, and gh's own message reaches
            # the user.
            #
            # gh attestation verify has to ask GitHub, so a non-zero exit
            # conflates "these bytes are not what that workflow built" with "I
            # could not ask". Only the first is a verdict, and only a verdict
            # may be fatal, so the ability to ask is established first.
            & gh auth status | Out-Null
            if ($LASTEXITCODE -eq 0) {
                Write-Info 'verifying the build-provenance attestation with gh'
                & gh attestation verify $assetPath --repo $Repo @ghIdentityArgs | Out-Null
                if ($LASTEXITCODE -eq 0) {
                    $verifiedSignature = $true
                    Write-Info 'attestation OK'
                }
                else {
                    # A verifier that ran and said no is the strongest signal
                    # either tool produces - stronger than the absence of the
                    # bundle a few lines above, which is already fatal. It
                    # cannot be the warning while that is the error.
                    throw "gh found no valid build-provenance attestation for $asset from $Repo's release workflow - these bytes are not what that workflow published, or the attestation that would prove it has been removed (set ALLOW_UNVERIFIED=1 to bypass at your own risk)"
                }
            }
            else {
                Write-Info 'WARNING: gh is installed but cannot reach GitHub (try ''gh auth login''), so no build-provenance attestation could be checked'
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
            # An unanswered request makes no claim about the release, so it
            # lands where every other "nothing could be checked" lands: loud by
            # default, fatal under REQUIRE_SIGNATURE=1.
            $msg = if ($bundleUnreachable) {
                "the signature could not be fetched, so nothing about these bytes was verified - retry, or install from a network that can reach $base"
            }
            else {
                "no signature was verified - install cosign or the gh CLI to check that these bytes came from $Repo's release workflow"
            }
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
  1. Write the token where the server reads it, then register with Claude Code:
       Set-Content -Path "$env:USERPROFILE\.gitlab-mcp-server.env" -Value 'GITLAB_TOKEN=glpat-xxxx'
       claude mcp add gitlab -- $binName
     (self-managed GitLab: add  GITLAB_URL=https://gitlab.example.com  to that file)
     Nothing names the token on the command line, so it stays out of the process
     arguments and out of the client's own configuration file.
  2. Or configure another MCP client by hand:
       https://jmrp.io/docs/gitlab-mcp-server/configuration/
"@
