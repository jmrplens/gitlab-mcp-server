#!/usr/bin/env bash
# Assemble, validate and publish the PyPI distribution out of band.
#
# The release workflow does NOT use this script: it publishes through the
# pypa/gh-action-pypi-publish action under an OIDC trusted publisher, with
# PEP 740 attestations and no stored credential. This script is for the
# bootstrap publish (a package cannot carry a trusted publisher until it
# exists... it can as a "pending publisher", but the bootstrap of 2.7.5 was
# done with a token) and for out-of-band re-publishes from a maintainer
# machine.
#
# Usage: scripts/publish-pypi.sh <binaries-dir> <version> [--dry-run]
# Auth:  PYPI_TOKEN environment variable (a pypi.org API token).
#
# twine is version-pinned and installed into a throwaway venv so nothing is
# added to the host Python. --skip-existing makes a re-run land on PyPI's
# 400 for existing files silently, mirroring publish-npm.sh's 409 handling.
set -euo pipefail

BINARIES="${1:?Usage: $0 <binaries-dir> <version> [--dry-run]}"
VERSION="${2:?Usage: $0 <binaries-dir> <version> [--dry-run]}"
DRY_RUN="${3:-}"

TWINE_VERSION="7.0.0"
WHEELHOUSE="pypi/dist"

# The token reaches twine and nothing else: the build, the validator and pip
# all spawn child processes that would otherwise inherit it from the
# environment. Capture it into a plain shell variable and drop the exported
# name before the first child starts.
PYPI_TOKEN_VALUE="${PYPI_TOKEN:-}"
unset PYPI_TOKEN

python3 scripts/build_pypi.py --binaries "$BINARIES" --version "$VERSION"
python3 scripts/validate_pypi.py --wheels "$WHEELHOUSE" --version "$VERSION"

if [[ "$DRY_RUN" == "--dry-run" ]]; then
  echo "publish-pypi: dry run complete, nothing uploaded"
  exit 0
fi

if [[ -z "$PYPI_TOKEN_VALUE" ]]; then
  echo "ERROR: PYPI_TOKEN is not set" >&2
  exit 1
fi

VENVDIR="$(mktemp -d)"
trap 'rm -rf "$VENVDIR"' EXIT
python3 -m venv "$VENVDIR/venv"
"$VENVDIR/venv/bin/pip" install --quiet "twine==${TWINE_VERSION}"

TWINE_USERNAME="__token__" TWINE_PASSWORD="$PYPI_TOKEN_VALUE" \
  "$VENVDIR/venv/bin/twine" upload --non-interactive --skip-existing "$WHEELHOUSE"/*.whl

echo "publish-pypi: published version $VERSION"
