#!/usr/bin/env bash
# publish-npm.sh assembles and publishes the npm distribution for one release:
# the six per-platform binary packages first, then the launcher that depends on
# them. The order is load-bearing — a consumer installing between the two
# publishes must not find the launcher referencing platform packages that are
# not on the registry yet.
#
# Usage:
#   scripts/publish-npm.sh <binaries-dir> <version> [--dry-run]
#
# <binaries-dir> holds the release assets under their published names. Auth is
# whatever `npm` already has: an NPM_TOKEN in CI (with an .npmrc that reads it)
# or an interactive `npm login` locally. In CI with id-token permission, npm
# attaches build provenance automatically; --dry-run packs and validates
# without publishing anything.
set -euo pipefail

BINARIES_DIR="${1:?Usage: $0 <binaries-dir> <version> [--dry-run]}"
VERSION="${2:?Usage: $0 <binaries-dir> <version> [--dry-run]}"
DRY_RUN=""
if [ "${3:-}" = "--dry-run" ]; then
  DRY_RUN="--dry-run"
fi

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="$ROOT/npm/packages"

echo "Assembling npm distribution for v$VERSION"
node "$ROOT/scripts/build-npm.mjs" --binaries "$BINARIES_DIR" --version "$VERSION" --out "$OUT"

publish() {
  dir="$1"
  name="$(node -p "require('$dir/package.json').name")"
  # Skip a version already on the registry so re-running a release job (which
  # happens — a later step can fail and get retried) does not die on npm's 409
  # for the packages that already went out. `npm view` prints the version when
  # it exists and errors (nothing on stdout) when it does not.
  if [ -z "$DRY_RUN" ] && [ "$(npm view "$name@$VERSION" version 2>/dev/null || true)" = "$VERSION" ]; then
    echo "Skipping $name@$VERSION — already published."
    return 0
  fi
  echo "Publishing $name@$VERSION…"
  # No --provenance flag on purpose. In CI this publishes through npm's OIDC
  # trusted publisher, which generates and attaches provenance automatically;
  # the local bootstrap publish (token auth, no OIDC) simply has none. Auth is
  # left to the environment — the workflow's setup-node registry config for the
  # OIDC exchange, or a developer's `npm login` / .npmrc for the bootstrap — so
  # this script never sees a credential.
  # shellcheck disable=SC2086 -- $DRY_RUN is intentionally word-split
  npm publish "$dir" --access public $DRY_RUN
}

for key in linux-x64 linux-arm64 darwin-x64 darwin-arm64 win32-x64 win32-arm64; do
  publish "$OUT/$key"
done

# The launcher last, once every platform package it pins is live.
publish "$ROOT/npm/gitlab-mcp-server"

echo "npm publish complete for v$VERSION"
