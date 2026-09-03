#!/usr/bin/env bash
# publish-npm.sh assembles and publishes the npm distribution for one release:
# the six per-platform binary packages first, then the launcher that depends on
# them. The order is load-bearing — a consumer installing between the two
# publishes must not find the launcher referencing platform packages that are
# not on the registry yet.
#
# Usage:
#   scripts/publish-npm.sh <binaries-dir> <version> [--dry-run] [--no-assemble]
#
# <binaries-dir> holds the release assets under their published names. Auth is
# whatever `npm` already has: an NPM_TOKEN in CI (with an .npmrc that reads it)
# or an interactive `npm login` locally. In CI with id-token permission, npm
# attaches build provenance automatically; --dry-run packs and validates
# without publishing anything.
#
# --no-assemble publishes the tree that is already in npm/packages instead of
# building it again. It exists because the release workflow validates the
# packages and then called this script, which rebuilt them: the bytes that went
# to the registry were therefore never the bytes the validator examined, only
# bytes an equally-configured run of the same generator had produced. Both runs
# verify each binary against the same cosign-signed checksums.txt, so the gap
# was narrow, but "we validated something else" is not a property worth keeping
# in a release path. With the flag the validated directory is what ships, and
# assert_assembled refuses a tree that is missing or built for another version
# rather than letting the flag turn a skipped build into a stale publish.
set -euo pipefail

BINARIES_DIR="${1:?Usage: $0 <binaries-dir> <version> [--dry-run] [--no-assemble]}"
VERSION="${2:?Usage: $0 <binaries-dir> <version> [--dry-run] [--no-assemble]}"
shift 2
DRY_RUN=""
ASSEMBLE=1
for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN="--dry-run" ;;
    --no-assemble) ASSEMBLE=0 ;;
    *) echo "unknown option: $arg" >&2; exit 2 ;;
  esac
done

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="$ROOT/npm/packages"

PLATFORM_KEYS=(linux-x64 linux-arm64 darwin-x64 darwin-arm64 win32-x64 win32-arm64)

# assert_assembled checks that every package --no-assemble is about to publish
# exists and carries the version being released.
assert_assembled() {
  local dir name assembled
  for dir in "${PLATFORM_KEYS[@]/#/$OUT/}" "$ROOT/npm/gitlab-mcp-server"; do
    if [ ! -f "$dir/package.json" ]; then
      echo "ERROR: --no-assemble was passed but $dir/package.json is missing." >&2
      echo "       Run 'make validate-npm-local NPM_BINARIES=<dir>' first, or drop the flag." >&2
      exit 1
    fi
    name="$(node -p "require('$dir/package.json').name")"
    assembled="$(node -p "require('$dir/package.json').version")"
    if [ "$assembled" != "$VERSION" ]; then
      echo "ERROR: $name in $dir is assembled at $assembled, not $VERSION." >&2
      echo "       The tree predates this release; re-run the validation step or drop --no-assemble." >&2
      exit 1
    fi
  done
}

if [ "$ASSEMBLE" -eq 1 ]; then
  echo "Assembling npm distribution for v$VERSION"
  node "$ROOT/scripts/build-npm.mjs" --binaries "$BINARIES_DIR" --version "$VERSION" --out "$OUT"
else
  echo "Publishing the npm distribution already assembled in $OUT"
  assert_assembled
fi

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
  # $DRY_RUN is empty or "--dry-run" and must word-split, not stay one argument.
  # shellcheck disable=SC2086
  npm publish "$dir" --access public $DRY_RUN
}

for key in "${PLATFORM_KEYS[@]}"; do
  publish "$OUT/$key"
done

# The launcher last, once every platform package it pins is live.
publish "$ROOT/npm/gitlab-mcp-server"

echo "npm publish complete for v$VERSION"
