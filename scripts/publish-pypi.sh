#!/usr/bin/env bash
# publish-pypi.sh assembles, validates and publishes the PyPI distribution for
# one release: the six platform wheels scripts/build_pypi.py writes.
#
# Usage:
#   scripts/publish-pypi.sh <binaries-dir> <version> [--dry-run] [--no-assemble] [--trusted-publishing]
#
# Two ways to authenticate, never both.
#
# --trusted-publishing is how release.yml's pypi job publishes. The job's OIDC
# token is exchanged for a PyPI upload token the way PyPI documents trusted
# publishing by hand: the audience is the one https://pypi.org/_/oidc/audience
# publishes, the token GitHub issues the job for it is posted to
# https://pypi.org/_/oidc/mint-token, and what PyPI mints uploads for this
# project's trusted publisher (this repository, release.yml and the release
# environment) and expires within minutes. Each wheel is first given a PEP 740
# publish attestation, signed with the same identity through Sigstore by
# pypi-attestations, and twine uploads the two together; the upload token is
# minted last, right before twine, so a signing failure mints nothing. These
# steps replace pypa/gh-action-pypi-publish, whose generated container action
# runs an image pypa names by a registry tag (issue 1243).
#
# Without it the upload token is PYPI_TOKEN, a pypi.org API token, for a
# publish from a maintainer's machine (make publish-pypi). No attestation is
# made then, since signing one needs the identity of a workflow.
#
# --dry-run builds or checks the wheelhouse, installs the tools and runs twine
# check --strict over the wheels, and stops there: nothing is minted, signed or
# uploaded. A release rehearsal runs exactly that.
#
# --no-assemble publishes the wheelhouse already in pypi/dist, which the
# workflow's previous step validated, instead of building an equivalent one,
# and refuses a wheelhouse holding anything but wheels of <version>.
#
# twine and pypi-attestations, with everything they import, are installed into
# a throwaway venv from .github/requirements/pypi-publish.txt in hash-checking
# mode and from wheels alone, so nothing beside the token runs that the file
# does not name by its digest, and nothing is built. --skip-existing makes a
# re-run land on the files PyPI already holds without failing, as
# publish-npm.sh skips a version npm already has.
#
# Each credential reaches the one process that needs it. The exported names are
# dropped before the first child starts; the upload token is handed to twine
# alone, and the job's token request credential to the identity request and to
# the signer; and none of them is ever a command-line argument, which every
# process on the machine can read.
set -euo pipefail

# Read first and dropped from the environment before anything else runs, the
# dirname that finds ROOT below included, so that no child inherits them.
PYPI_TOKEN_VALUE="${PYPI_TOKEN:-}"
REQUEST_URL="${ACTIONS_ID_TOKEN_REQUEST_URL:-}"
REQUEST_TOKEN="${ACTIONS_ID_TOKEN_REQUEST_TOKEN:-}"
unset PYPI_TOKEN ACTIONS_ID_TOKEN_REQUEST_URL ACTIONS_ID_TOKEN_REQUEST_TOKEN

USAGE="Usage: $0 <binaries-dir> <version> [--dry-run] [--no-assemble] [--trusted-publishing]"
BINARIES="${1:?$USAGE}"
VERSION="${2:?$USAGE}"
shift 2
DRY_RUN=0
ASSEMBLE=1
TRUSTED=0
while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run) DRY_RUN=1 ;;
    --no-assemble) ASSEMBLE=0 ;;
    --trusted-publishing) TRUSTED=1 ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
  shift
done

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WHEELHOUSE="$ROOT/pypi/dist"
REQUIREMENTS="$ROOT/.github/requirements/pypi-publish.txt"
DIST="jmrplens_gitlab_mcp_server"
UPLOAD_URL="https://upload.pypi.org/legacy/"
AUDIENCE_URL="https://pypi.org/_/oidc/audience"
MINT_URL="https://pypi.org/_/oidc/mint-token"

if [ "$TRUSTED" = 1 ]; then
  if [ -n "$PYPI_TOKEN_VALUE" ]; then
    echo "ERROR: PYPI_TOKEN is set, but --trusted-publishing mints the upload token from the job's identity; unset it" >&2
    exit 2
  fi
  if [ -z "$REQUEST_URL" ] || [ -z "$REQUEST_TOKEN" ]; then
    echo "ERROR: --trusted-publishing needs the job's OIDC identity, and ACTIONS_ID_TOKEN_REQUEST_URL or" \
      "ACTIONS_ID_TOKEN_REQUEST_TOKEN is not set: the job has to hold id-token: write" >&2
    exit 1
  fi
elif [ "$DRY_RUN" = 0 ] && [ -z "$PYPI_TOKEN_VALUE" ]; then
  echo "ERROR: PYPI_TOKEN is not set (a workflow holding id-token: write passes --trusted-publishing instead)" >&2
  exit 1
fi

# assert_wheelhouse holds pypi/dist to what is about to be uploaded: one wheel
# or more, each a wheel of $VERSION, and nothing else, a hidden file included.
# An attestation left behind would be uploaded beside a wheel it does not
# describe, and a wheel of another version would be published with this one.
assert_wheelhouse() {
  local file name count=0
  if [ ! -d "$WHEELHOUSE" ]; then
    echo "ERROR: $WHEELHOUSE does not exist; run 'make validate-pypi-local PYPI_BINARIES=<dir>' first, or drop --no-assemble" >&2
    exit 1
  fi
  shopt -s nullglob dotglob
  for file in "$WHEELHOUSE"/*; do
    name="${file##*/}"
    case "$name" in
      "$DIST-$VERSION-"*.whl) count=$((count + 1)) ;;
      *)
        echo "ERROR: $WHEELHOUSE holds $name, which is not a wheel of $VERSION; rebuild the wheelhouse" >&2
        exit 1
        ;;
    esac
  done
  shopt -u dotglob
  if [ "$count" = 0 ]; then
    echo "ERROR: $WHEELHOUSE holds no wheel of $VERSION; rebuild the wheelhouse" >&2
    exit 1
  fi
}

# json_member prints the string member $1 of the JSON object on standard input,
# or fails saying what the object holds instead: PyPI's errors, each code with
# its description, when it names any. The value only ever travels through a
# pipe and standard output.
json_member() {
  python3 -c '
import json
import sys

name = sys.argv[1]
try:
    document = json.load(sys.stdin)
except ValueError as exc:
    sys.exit(f"ERROR: the answer is not JSON: {exc}")
document = document if isinstance(document, dict) else {}
value = document.get(name)
if isinstance(value, str) and value:
    print(value)
    sys.exit(0)
errors = document.get("errors")
if isinstance(errors, list) and errors:
    reasons = "; ".join("%s: %s" % (e.get("code"), e.get("description")) for e in errors if isinstance(e, dict))
    sys.exit("ERROR: %s: %s" % (document.get("message", "the request was refused"), reasons))
sys.exit("ERROR: the answer carries no " + name)
' "$1"
}

# curl_https makes one request to an https URL, refusing every other protocol
# and following no redirect, and fails on an HTTP error with the answer still
# written out, so json_member can say why.
curl_https() {
  curl --silent --show-error --fail-with-body --proto '=https' --max-time 30 "$@"
}

# mask asks the runner to hide a token from the job's log, as PyPI's
# instructions for minting one by hand do. Off GitHub Actions it prints nothing.
mask() {
  if [ "${GITHUB_ACTIONS:-}" = "true" ]; then
    echo "::add-mask::$1"
  fi
}

if [ "$ASSEMBLE" = 1 ]; then
  python3 "$ROOT/scripts/build_pypi.py" --binaries "$BINARIES" --version "$VERSION" --out "$WHEELHOUSE"
  python3 "$ROOT/scripts/validate_pypi.py" --wheels "$WHEELHOUSE" --version "$VERSION"
fi
assert_wheelhouse
WHEELS=("$WHEELHOUSE"/*.whl)

VENVDIR="$(mktemp -d)"
trap 'rm -rf "$VENVDIR"' EXIT
python3 -m venv "$VENVDIR/venv"
PY="$VENVDIR/venv/bin/python"
"$PY" -m pip install --require-hashes --only-binary :all: --quiet --disable-pip-version-check -r "$REQUIREMENTS"
"$PY" -m twine check --strict "${WHEELS[@]}"

if [ "$DRY_RUN" = 1 ]; then
  echo "publish-pypi: dry run complete: ${#WHEELS[@]} wheels checked with the hash-pinned twine; nothing was minted, signed or uploaded"
  exit 0
fi

if [ "$TRUSTED" = 0 ]; then
  TWINE_USERNAME="__token__" TWINE_PASSWORD="$PYPI_TOKEN_VALUE" \
    "$PY" -m twine upload --non-interactive --skip-existing --repository-url "$UPLOAD_URL" "${WHEELS[@]}"
  echo "publish-pypi: published version $VERSION"
  exit 0
fi

# Sigstore reads the job's identity from the two variables, through the id
# library, and asks for a token of its own audience.
ACTIONS_ID_TOKEN_REQUEST_URL="$REQUEST_URL" ACTIONS_ID_TOKEN_REQUEST_TOKEN="$REQUEST_TOKEN" \
  "$PY" -m pypi_attestations sign "${WHEELS[@]}"
ATTESTATIONS=("${WHEELS[@]/%/.publish.attestation}")
for attestation in "${ATTESTATIONS[@]}"; do
  if [ ! -f "$attestation" ]; then
    echo "ERROR: pypi-attestations wrote no $attestation" >&2
    exit 1
  fi
done

AUDIENCE="$(curl_https "$AUDIENCE_URL" | json_member audience)"
case "$AUDIENCE" in
  *[!A-Za-z0-9._:-]*)
    echo "ERROR: $AUDIENCE_URL answered $AUDIENCE, which is not an audience" >&2
    exit 1
    ;;
esac
case "$REQUEST_URL" in
  *\?*) SEPARATOR="&" ;;
  *) SEPARATOR="?" ;;
esac
OIDC_TOKEN="$(printf 'header = "Authorization: bearer %s"\n' "$REQUEST_TOKEN" |
  curl_https --config - "$REQUEST_URL${SEPARATOR}audience=$AUDIENCE" | json_member value)"
mask "$OIDC_TOKEN"
case "$OIDC_TOKEN" in
  *[!A-Za-z0-9._-]*)
    echo "ERROR: the job's OIDC token is not a JWT" >&2
    exit 1
    ;;
esac
UPLOAD_TOKEN="$(printf '{"token": "%s"}' "$OIDC_TOKEN" |
  curl_https --header "Content-Type: application/json" --data-binary @- "$MINT_URL" | json_member token)"
mask "$UPLOAD_TOKEN"

TWINE_USERNAME="__token__" TWINE_PASSWORD="$UPLOAD_TOKEN" \
  "$PY" -m twine upload --non-interactive --skip-existing --attestations --repository-url "$UPLOAD_URL" \
  "${WHEELS[@]}" "${ATTESTATIONS[@]}"
echo "publish-pypi: published version $VERSION, each wheel with its publish attestation"
