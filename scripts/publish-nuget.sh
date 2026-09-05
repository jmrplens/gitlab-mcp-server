#!/usr/bin/env bash
# publish-nuget.sh assembles, validates and pushes the NuGet distribution for
# one release: the six runtime-identifier packages first, then the pointer
# package that names them. The order is load-bearing, exactly as it is for
# npm: `dotnet tool install` resolves the pointer and then the package for the
# host, so a pointer visible before its runtime packages installs nothing.
#
# Usage:
#   scripts/publish-nuget.sh <binaries-dir> <version> [--dry-run] [--no-assemble]
#
# <binaries-dir> holds the release assets under their published names. Auth is
# the NUGET_API_KEY environment variable: in the release workflow it is the
# one-hour key NuGet/login minted from the job's OIDC identity under the
# nuget.org trusted publishing policy, and out of band it is an API key from
# nuget.org (`make publish-nuget NUGET_BINARIES=dist`), the only tokened path.
# `dotnet nuget push --skip-duplicate` turns the registry's 409 for a version
# already there into a warning, so re-running a release job is safe.
#
# --dry-run builds and validates, then prints the pushes it would make and
# stops; the key is never read. --no-assemble pushes the tree already in
# nuget/dist (or $NUGET_OUT) instead of building it again, so the bytes the
# validation step opened are the bytes that go out, and assert_assembled
# refuses a tree that is missing or built for another version rather than
# letting the flag turn a skipped build into a stale publish.
#
# Needs python3 for the packer and validator and the .NET 10 SDK on PATH for
# the install check and the push. Without an SDK on the machine, run the whole
# thing from the SDK container: `make validate-nuget NUGET_BINARIES=dist`
# covers everything but the push.
set -euo pipefail

BINARIES_DIR="${1:?Usage: $0 <binaries-dir> <version> [--dry-run] [--no-assemble]}"
VERSION="${2:?Usage: $0 <binaries-dir> <version> [--dry-run] [--no-assemble]}"
shift 2
DRY_RUN=0
ASSEMBLE=1
for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=1 ;;
    --no-assemble) ASSEMBLE=0 ;;
    *) echo "unknown option: $arg" >&2; exit 2 ;;
  esac
done

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="${NUGET_OUT:-$ROOT/nuget/dist}"
PKG_ID="gitlab-mcp-server"
SOURCE_URL="https://api.nuget.org/v3/index.json"
RIDS=(linux-x64 linux-arm64 osx-x64 osx-arm64 win-x64 win-arm64)

# The key reaches `dotnet nuget push` and nothing else: the packer, the
# validator and the SDK install it drives all spawn child processes that would
# otherwise inherit it from the environment. Capture it into a plain shell
# variable and drop the exported name before the first child starts.
NUGET_API_KEY_VALUE="${NUGET_API_KEY:-}"
unset NUGET_API_KEY

if ! command -v dotnet >/dev/null 2>&1; then
  echo "ERROR: dotnet is not on PATH; the .NET 10 SDK is needed to validate and push" >&2
  exit 1
fi

# The seven packages, runtime packages first and the pointer last.
packages=()
for rid in "${RIDS[@]}"; do
  packages+=("$OUT/$PKG_ID.$rid.$VERSION.nupkg")
done
packages+=("$OUT/$PKG_ID.$VERSION.nupkg")

# assert_assembled checks that every package --no-assemble is about to push
# exists and carries the version being released, which the file name states.
assert_assembled() {
  local pkg
  for pkg in "${packages[@]}"; do
    if [ ! -f "$pkg" ]; then
      echo "ERROR: --no-assemble was passed but $pkg is missing." >&2
      echo "       Run 'make validate-nuget-local NUGET_BINARIES=<dir>' first, or drop the flag." >&2
      exit 1
    fi
  done
  local stray
  stray="$(find "$OUT" -maxdepth 1 -name '*.nupkg' ! -name "*.$VERSION.nupkg" | head -n 1)"
  if [ -n "$stray" ]; then
    echo "ERROR: $OUT holds $stray, which is not version $VERSION." >&2
    echo "       The tree predates this release; re-run the validation step or drop --no-assemble." >&2
    exit 1
  fi
}

if [ "$ASSEMBLE" -eq 1 ]; then
  echo "Assembling NuGet distribution for v$VERSION"
  python3 "$ROOT/scripts/build_nuget.py" --binaries "$BINARIES_DIR" --version "$VERSION" --out "$OUT"
  python3 "$ROOT/scripts/validate_nuget.py" --packages "$OUT" --version "$VERSION"
else
  echo "Publishing the NuGet distribution already assembled in $OUT"
  assert_assembled
fi

if [ "$DRY_RUN" -eq 1 ]; then
  echo "publish-nuget: dry run, these pushes would be made in this order:"
  for pkg in "${packages[@]}"; do
    echo "  dotnet nuget push $(basename "$pkg") --source $SOURCE_URL --skip-duplicate"
  done
  echo "publish-nuget: dry run complete, nothing pushed"
  exit 0
fi

if [ -z "$NUGET_API_KEY_VALUE" ]; then
  echo "ERROR: NUGET_API_KEY is not set" >&2
  exit 1
fi

for pkg in "${packages[@]}"; do
  echo "Pushing $(basename "$pkg")"
  dotnet nuget push "$pkg" --api-key "$NUGET_API_KEY_VALUE" --source "$SOURCE_URL" --skip-duplicate
done

echo "nuget publish complete for v$VERSION"
