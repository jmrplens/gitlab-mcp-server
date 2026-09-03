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
#   RELEASE_LATEST_URL override the /releases/latest URL (used by the tests)
#   FETCH_ATTEMPTS     tries per download before giving up (default: 3)
#   FETCH_RETRY_DELAY  seconds between those tries (default: 2)
#   ALLOW_UNVERIFIED   set to 1 to skip verification entirely (not recommended)
#
# After install, register the server with Claude Code:
#   claude mcp add gitlab --env GITLAB_TOKEN=glpat-xxxx -- gitlab-mcp-server
set -eu

REPO="${REPO:-jmrplens/gitlab-mcp-server}"
VERSION="${VERSION:-latest}"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
BIN_NAME="gitlab-mcp-server"
FETCH_ATTEMPTS="${FETCH_ATTEMPTS:-3}"
FETCH_RETRY_DELAY="${FETCH_RETRY_DELAY:-2}"

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

# --- pick a downloader -----------------------------------------------------
# Real installs stay on https end to end, redirects included: a chain that
# dropped to http would otherwise be followed without a word. RELEASE_BASE_URL
# is the test hook and points at a local http fixture, so it opts out.
#
# final_url reports where a URL ends up after redirects; wget prints each hop as
# a `Location:` header rather than the final URL, so the last one is the answer.
last_location() {
	sed -n 's/^[[:space:]]*[Ll]ocation:[[:space:]]*\([^[:space:]]*\).*/\1/p' | tr -d '\r' | tail -n 1
}

# http_status pulls the last HTTP status out of a downloader's own diagnostics:
# GNU wget -S prints "  HTTP/1.1 404 Not Found" header lines, BusyBox wget
# prints "wget: server returned error: HTTP/1.1 404 Not Found", and GNU wget
# without -S prints "ERROR 404: Not Found." Anything unrecognised yields the
# empty string, which every caller reads as "could not tell".
http_status() {
	sed -n \
		-e 's|.*HTTP/[0-9.]*[[:space:]]\{1,\}\([0-9][0-9][0-9]\).*|\1|p' \
		-e 's|.*ERROR[[:space:]]\{1,\}\([0-9][0-9][0-9]\).*|\1|p' |
		tail -n 1
}

# wget_get FLAG URL DEST is the shared body of the wget branches: -S so the
# response headers reach http_status, and no -q so a BusyBox wget's error line
# reaches it too. FLAG is empty or a single extra wget option, since POSIX sh
# has no arrays to hold one.
wget_get() {
	if [ -n "$1" ]; then
		_wget_out=$(wget "$1" -S -O "$3" "$2" 2>&1) && {
			echo 200
			return 0
		}
	else
		_wget_out=$(wget -S -O "$3" "$2" 2>&1) && {
			echo 200
			return 0
		}
	fi
	printf '%s\n' "$_wget_out" | http_status
}

# http_get URL DEST writes the body to DEST and prints the HTTP status the
# server gave, or nothing at all when no response was obtained. That
# distinction is the whole reason this is not just `dl`: "this release
# publishes no signature" and "the server did not answer" are different facts,
# and only the first one may be fatal.
if command -v curl >/dev/null 2>&1; then
	if [ -n "${RELEASE_BASE_URL:-}" ]; then
		http_get() { curl -fsSL "$1" -o "$2" -w '%{http_code}' 2>/dev/null || true; }
		final_url() { curl -fsSLI -o /dev/null -w '%{url_effective}' "$1" 2>/dev/null || true; }
	else
		http_get() {
			curl -fsSL --proto '=https' --proto-redir '=https' --tlsv1.2 \
				"$1" -o "$2" -w '%{http_code}' 2>/dev/null || true
		}
		final_url() {
			curl -fsSLI --proto '=https' --proto-redir '=https' --tlsv1.2 \
				-o /dev/null -w '%{url_effective}' "$1" 2>/dev/null || true
		}
	fi
elif command -v wget >/dev/null 2>&1; then
	if [ -n "${RELEASE_BASE_URL:-}" ]; then
		http_get() { wget_get "" "$1" "$2"; }
		final_url() { wget --max-redirect=10 -S --spider "$1" 2>&1 | last_location; }
	elif wget --help 2>&1 | grep -q -- '--https-only'; then
		http_get() { wget_get --https-only "$1" "$2"; }
		final_url() { wget --https-only --max-redirect=10 -S --spider "$1" 2>&1 | last_location; }
	else
		# BusyBox's wget (Alpine) has neither --https-only nor --spider, and
		# breaking those installs to gain the restriction would be a poor trade:
		# the URLs are https literals, and "latest" stays unresolved. It has no
		# -S either, so the status comes from whatever it wrote about the
		# failure; when that says nothing recognisable the caller is told it
		# could not tell, rather than being handed a guess.
		http_get() { wget_get "" "$1" "$2"; }
		final_url() { echo ""; }
	fi
else
	err "need curl or wget to download"
fi

# fetch_asset URL DEST downloads one release asset and sets fetch_outcome:
#   ok           the bytes are at DEST
#   absent       the server answered, and this asset is not published
#   unavailable  no usable answer after FETCH_ATTEMPTS tries
#
# It always returns 0, so a caller reads the outcome rather than juggling exit
# statuses under `set -e`. Only a transport failure is retried; a server that
# answered 404 has told us what we asked, and asking again would only delay
# saying so.
fetch_outcome=""
fetch_asset() {
	_attempt=1
	while :; do
		_code=$(http_get "$1" "$2")
		case "$_code" in
		2??)
			fetch_outcome=ok
			return 0
			;;
		404 | 410)
			fetch_outcome=absent
			return 0
			;;
		esac
		if [ "$_attempt" -ge "$FETCH_ATTEMPTS" ]; then
			fetch_outcome=unavailable
			return 0
		fi
		info "${1##*/}: no answer (${_code:-no response}); retrying in ${FETCH_RETRY_DELAY}s (attempt $_attempt of $FETCH_ATTEMPTS)"
		_attempt=$((_attempt + 1))
		sleep "$FETCH_RETRY_DELAY"
	done
}

# --- resolve which release to install --------------------------------------
# The asset names carry no version (gitlab-mcp-server-<os>-<arch>) and neither
# does checksums.txt, so "latest" left unresolved lets whoever can replace
# release assets serve a consistent, correctly signed triple from an older,
# vulnerable release. Following the /releases/latest redirect names the tag
# before anything is downloaded, which is what pins the signature identity to
# the version being installed.
if [ -n "${RELEASE_BASE_URL:-}" ]; then
	latest_url="${RELEASE_LATEST_URL:-}"
else
	latest_url="https://github.com/$REPO/releases/latest"
fi
if [ "$VERSION" = "latest" ] && [ -n "$latest_url" ]; then
	resolved=$(final_url "$latest_url")
	case "$resolved" in
	*/releases/tag/*)
		VERSION="${resolved##*/releases/tag/}"
		info "latest is $VERSION"
		;;
	*)
		info "WARNING: could not resolve which release is latest; the signature check cannot name a version"
		;;
	esac
fi

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
fetch_asset "$base/$asset" "$tmp/$asset"
case "$fetch_outcome" in
absent) err "release $VERSION publishes no $asset: $base/$asset" ;;
unavailable) err "download failed after $FETCH_ATTEMPTS attempts: $base/$asset" ;;
esac

# The two knobs pull in opposite directions, so a configuration that sets both
# is a mistake worth naming rather than resolving silently in either direction.
if [ "${ALLOW_UNVERIFIED:-0}" = "1" ] && [ "${REQUIRE_SIGNATURE:-0}" = "1" ]; then
	err "ALLOW_UNVERIFIED=1 and REQUIRE_SIGNATURE=1 contradict each other; unset one"
fi

if [ "${ALLOW_UNVERIFIED:-0}" = "1" ]; then
	info "WARNING: ALLOW_UNVERIFIED=1 — skipping checksum verification"
else
	info "verifying checksum"
	got=$(sha256 "$tmp/$asset")
	[ -n "$got" ] || err "no sha256 tool (need sha256sum or shasum) to verify the download; install one or re-run with ALLOW_UNVERIFIED=1"
	fetch_asset "$base/checksums.txt" "$tmp/checksums.txt"
	[ "$fetch_outcome" = ok ] ||
		err "could not fetch checksums.txt to verify the download ($fetch_outcome); aborting (set ALLOW_UNVERIFIED=1 to bypass)"

	# checksums.txt comes from the same release as the binary, so on its own it
	# only proves the two files agree — a principal who can replace release
	# assets replaces both. Every release also publishes a keyless cosign
	# signature over checksums.txt, and GitHub holds a build-provenance
	# attestation for the binary itself; verify whichever the machine can.
	verified_signature=0
	bundle_missing=0
	bundle_unreachable=0

	# Both verifiers are told which release they are looking at. An unresolved
	# "latest" is the only case left with no tag to name, and its regexp is
	# satisfied by any release version, which is precisely the rollback this
	# cannot otherwise see.
	if [ "$VERSION" = "latest" ]; then
		identity_arg="--certificate-identity-regexp"
		gh_identity_arg="--cert-identity-regex"
		identity="^https://github\\.com/${REPO}/\\.github/workflows/release\\.yml@refs/tags/v[0-9]+\\.[0-9]+\\.[0-9]+\$"
	else
		identity_arg="--certificate-identity"
		gh_identity_arg="--cert-identity"
		identity="https://github.com/${REPO}/.github/workflows/release.yml@refs/tags/${VERSION}"
	fi

	if command -v cosign >/dev/null 2>&1; then
		fetch_asset "$base/checksums.txt.sigstore.json" "$tmp/checksums.txt.sigstore.json"
		case "$fetch_outcome" in
		ok)
			info "verifying the cosign signature over checksums.txt"
			cosign verify-blob \
				--bundle "$tmp/checksums.txt.sigstore.json" \
				"$identity_arg" "$identity" \
				--certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
				"$tmp/checksums.txt" >/dev/null ||
				err "checksums.txt failed cosign verification — the release assets do not carry this project's signature"
			verified_signature=1
			info "signature OK"
			;;
		absent)
			bundle_missing=1
			info "WARNING: this release publishes no checksums.txt.sigstore.json"
			;;
		unavailable)
			# The server never said whether the bundle is there, so neither can
			# this. Treating an unanswered request as "the release has no
			# signature" would make a proxy hiccup indistinguishable from the
			# one asset an attacker has to remove, and would abort an install
			# that has nothing wrong with it.
			bundle_unreachable=1
			info "WARNING: checksums.txt.sigstore.json could not be fetched after $FETCH_ATTEMPTS attempts; whether this release is signed is unknown"
			;;
		esac
	fi
	# Not an elif: when the bundle is the asset that went missing, gh still has
	# something to say, because the build-provenance attestation lives in
	# GitHub's own store rather than among the release assets.
	if [ "$verified_signature" -eq 0 ] && command -v gh >/dev/null 2>&1; then
		# gh attestation verify has to ask GitHub, so a non-zero exit conflates
		# two very different answers: "these bytes are not what that workflow
		# built" and "I could not ask". Only the first is a verdict, and only a
		# verdict may be fatal, so the ability to ask is established first
		# rather than guessed at from gh's failure text afterwards.
		if gh auth status >/dev/null 2>&1; then
			info "verifying the build-provenance attestation with gh"
			if gh attestation verify "$tmp/$asset" --repo "$REPO" "$gh_identity_arg" "$identity" >/dev/null 2>&1; then
				verified_signature=1
				info "attestation OK"
			else
				# A verifier that ran and said no is the strongest signal
				# either tool produces — strictly stronger than the absence of
				# the bundle a few lines above, which is already fatal. It
				# cannot be the warning while that is the error.
				err "gh found no valid build-provenance attestation for $asset from ${REPO}'s release workflow — these bytes are not what that workflow published, or the attestation that would prove it has been removed (set ALLOW_UNVERIFIED=1 to bypass at your own risk)"
			fi
		else
			info "WARNING: gh is installed but cannot reach GitHub (try 'gh auth login'), so no build-provenance attestation could be checked"
		fi
	fi
	if [ "$verified_signature" -eq 0 ]; then
		# Every release since signing began publishes the bundle, so on a machine
		# that can check one its absence is not an old release: it is the single
		# asset whoever replaced the binary and checksums.txt also has to remove.
		if [ "$bundle_missing" -eq 1 ]; then
			err "no checksums.txt.sigstore.json is served for this release and no attestation could be verified; refusing to install unverified bytes (set ALLOW_UNVERIFIED=1 to bypass at your own risk)"
		fi
		# An unanswered request makes no claim about the release, so it lands
		# where every other "nothing could be checked" lands: loud by default,
		# fatal under REQUIRE_SIGNATURE=1.
		if [ "$bundle_unreachable" -eq 1 ]; then
			msg="the signature could not be fetched, so nothing about these bytes was verified — retry, or install from a network that can reach ${base}"
		else
			msg="no signature was verified — install cosign or the gh CLI to check that these bytes came from ${REPO}'s release workflow"
		fi
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
