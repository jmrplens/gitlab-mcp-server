#!/bin/sh
# install.sh — download the gitlab-mcp-server binary for this OS/arch from the
# latest GitHub Release, verify its checksum, and install it onto PATH.
#
#   curl -fsSL https://raw.githubusercontent.com/jmrplens/gitlab-mcp-server/main/scripts/install.sh | sh
#
# Environment overrides:
#   INSTALL_DIR        target directory (default: $HOME/.local/bin)
#   VERSION            release tag to install (default: latest)
#   REPO               owner/repo (default: jmrplens/gitlab-mcp-server)
#   REQUIRE_SIGNATURE  set to 1 to abort when no signature can be verified
#   RELEASE_BASE_URL   override the release download base (used by the tests)
#   ALLOW_UNVERIFIED   set to 1 to skip verification entirely (not recommended)
#
# After install, register the server with Claude Code:
#   claude mcp add gitlab --env GITLAB_TOKEN=glpat-xxxx -- gitlab-mcp-server
set -eu

REPO="${REPO:-jmrplens/gitlab-mcp-server}"
VERSION="${VERSION:-latest}"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
BIN_NAME="gitlab-mcp-server"

info() { printf '==> %s\n' "$1" >&2; }
err() {
	printf 'error: %s\n' "$1" >&2
	exit 1
}

# --- detect platform -------------------------------------------------------
os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
linux) os=linux ;;
darwin) os=darwin ;;
*) err "unsupported OS '$os'; on Windows use scripts/install.ps1 (PowerShell) instead" ;;
esac

arch=$(uname -m)
case "$arch" in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) err "unsupported architecture '$arch'" ;;
esac

asset="${BIN_NAME}-${os}-${arch}"

# --- resolve download base -------------------------------------------------
# RELEASE_BASE_URL exists so the installer can be driven against a local
# fixture server in tests; leave it unset for real installs.
if [ -n "${RELEASE_BASE_URL:-}" ]; then
	base="$RELEASE_BASE_URL"
elif [ "$VERSION" = "latest" ]; then
	base="https://github.com/$REPO/releases/latest/download"
else
	base="https://github.com/$REPO/releases/download/$VERSION"
fi

# --- pick a downloader -----------------------------------------------------
if command -v curl >/dev/null 2>&1; then
	dl() { curl -fsSL "$1" -o "$2"; }
elif command -v wget >/dev/null 2>&1; then
	dl() { wget -qO "$2" "$1"; }
else
	err "need curl or wget to download"
fi

# --- pick a checksum tool --------------------------------------------------
# Integrity verification is mandatory by default (fail closed): a download that
# cannot be verified is rejected rather than installed. Set ALLOW_UNVERIFIED=1
# to bypass on systems without a sha256 tool — at your own risk.
if command -v sha256sum >/dev/null 2>&1; then
	sha256() { sha256sum "$1" | cut -d' ' -f1; }
elif command -v shasum >/dev/null 2>&1; then
	sha256() { shasum -a 256 "$1" | cut -d' ' -f1; }
else
	sha256() { echo ""; }
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

info "downloading $asset ($VERSION)"
dl "$base/$asset" "$tmp/$asset" || err "download failed: $base/$asset"

if [ "${ALLOW_UNVERIFIED:-0}" = "1" ]; then
	info "WARNING: ALLOW_UNVERIFIED=1 — skipping checksum verification"
else
	info "verifying checksum"
	got=$(sha256 "$tmp/$asset")
	[ -n "$got" ] || err "no sha256 tool (need sha256sum or shasum) to verify the download; install one or re-run with ALLOW_UNVERIFIED=1"
	dl "$base/checksums.txt" "$tmp/checksums.txt" 2>/dev/null || err "could not fetch checksums.txt to verify the download; aborting (set ALLOW_UNVERIFIED=1 to bypass)"

	# checksums.txt comes from the same release as the binary, so on its own it
	# only proves the two files agree — a principal who can replace release
	# assets replaces both. Every release also publishes a keyless cosign
	# signature over checksums.txt, and GitHub holds a build-provenance
	# attestation for the binary itself; verify whichever the machine can.
	verified_signature=0
	if command -v cosign >/dev/null 2>&1; then
		if dl "$base/checksums.txt.sigstore.json" "$tmp/checksums.txt.sigstore.json" 2>/dev/null; then
			info "verifying the cosign signature over checksums.txt"
			# /releases/latest/download never reveals the tag, so pin the
			# workflow identity and let the tag itself be any release version.
			if [ "$VERSION" = "latest" ]; then
				identity_arg="--certificate-identity-regexp"
				identity="^https://github\\.com/${REPO}/\\.github/workflows/release\\.yml@refs/tags/v[0-9]+\\.[0-9]+\\.[0-9]+\$"
			else
				identity_arg="--certificate-identity"
				identity="https://github.com/${REPO}/.github/workflows/release.yml@refs/tags/${VERSION}"
			fi
			cosign verify-blob \
				--bundle "$tmp/checksums.txt.sigstore.json" \
				"$identity_arg" "$identity" \
				--certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
				"$tmp/checksums.txt" >/dev/null ||
				err "checksums.txt failed cosign verification — the release assets do not carry this project's signature"
			verified_signature=1
			info "signature OK"
		else
			info "WARNING: this release publishes no checksums.txt.sigstore.json"
		fi
	elif command -v gh >/dev/null 2>&1; then
		info "verifying the build-provenance attestation with gh"
		if gh attestation verify "$tmp/$asset" --repo "$REPO" >/dev/null 2>&1; then
			verified_signature=1
			info "attestation OK"
		else
			info "WARNING: gh found no valid build-provenance attestation for $asset"
		fi
	fi
	if [ "$verified_signature" -eq 0 ]; then
		msg="no signature was verified — install cosign or the gh CLI to check that these bytes came from ${REPO}'s release workflow"
		if [ "${REQUIRE_SIGNATURE:-0}" = "1" ]; then
			err "$msg (REQUIRE_SIGNATURE=1)"
		fi
		info "WARNING: $msg"
	fi

	# Strip CR so a checksums.txt saved with CRLF still matches the $-anchored grep.
	want=$(tr -d '\r' <"$tmp/checksums.txt" | grep " ${asset}\$" | cut -d' ' -f1 || true)
	[ -n "$want" ] || err "$asset is not listed in checksums.txt; aborting (set ALLOW_UNVERIFIED=1 to bypass)"
	[ "$want" = "$got" ] || err "checksum mismatch for $asset (want $want, got $got)"
	info "checksum OK"
fi

# --- install ---------------------------------------------------------------
mkdir -p "$INSTALL_DIR"
# Unlink an existing binary first so re-installing over a running server does not
# fail with "Text file busy".
rm -f "$INSTALL_DIR/$BIN_NAME" 2>/dev/null || true
install -m 0755 "$tmp/$asset" "$INSTALL_DIR/$BIN_NAME" 2>/dev/null ||
	{ cp "$tmp/$asset" "$INSTALL_DIR/$BIN_NAME" && chmod 0755 "$INSTALL_DIR/$BIN_NAME"; }

info "installed $INSTALL_DIR/$BIN_NAME"
case ":$PATH:" in
*":$INSTALL_DIR:"*) on_path=1 ;;
*) on_path=0 ;;
esac
if [ "$on_path" -eq 0 ]; then
	info "NOTE: $INSTALL_DIR is not on your PATH. Add it, e.g.:"
	# The literal $PATH must reach the user verbatim, so it stays single-quoted.
	# shellcheck disable=SC2016
	printf '       export PATH="%s:$PATH"\n' "$INSTALL_DIR" >&2
fi

cat >&2 <<EOF

Next steps:
  1. Register with Claude Code:
       claude mcp add gitlab --env GITLAB_TOKEN=glpat-xxxx -- $BIN_NAME
     (self-managed GitLab: add  --env GITLAB_URL=https://gitlab.example.com)
  2. Or configure another MCP client by hand:
       https://jmrp.io/docs/gitlab-mcp-server/configuration/

Running $BIN_NAME with no token prints what it needs and waits, so you can
check the install without configuring anything.
EOF
