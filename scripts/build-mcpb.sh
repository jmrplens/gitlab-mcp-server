#!/usr/bin/env bash
# Build the Claude Desktop extension bundle (gitlab-mcp-server.mcpb).
#
# Assembles a MCPB bundle directory from the checked-in manifest
# (mcpb/manifest.json), the 512x512 icon (mcpb/icon.png), and the release
# binaries produced by GoReleaser, then packs it with the official
# @anthropic-ai/mcpb CLI:
#
#   bundle/
#   ├── manifest.json                  (version stamped to <version>)
#   ├── icon.png
#   └── server/
#       ├── gitlab-mcp-server          (darwin universal: arm64 + amd64)
#       └── gitlab-mcp-server.exe      (windows amd64)
#
# Usage: build-mcpb.sh <version> [dist-dir]
#
#   <version>   Release version without the leading v (e.g. 2.5.0)
#   [dist-dir]  GoReleaser output directory (default: dist)
#
# Output: <dist-dir>/gitlab-mcp-server.mcpb

set -euo pipefail

VERSION="${1:?Usage: $0 <version> [dist-dir]}"
DIST_DIR="${2:-dist}"
MANIFEST="mcpb/manifest.json"
ICON="mcpb/icon.png"
# Pin the packer for supply-chain integrity; bump deliberately.
MCPB_VERSION="2.1.2"

for f in "$MANIFEST" "$ICON"; do
  if [[ ! -f "$f" ]]; then
    echo "ERROR: $f not found (run from the repository root)" >&2
    exit 1
  fi
done

if ! command -v jq &> /dev/null; then
  echo "ERROR: jq is required but not installed" >&2
  exit 1
fi

# Locate the GoReleaser artifacts. Binary paths live in per-target build
# directories (dist/<id>_<goos>_<goarch>[_<goamd64>]/); the darwin universal
# binary comes from the universal_binaries step (goarch "all").
find_binary() {
  local pattern="$1" name="$2" found
  found=$(find "$DIST_DIR" -type f -path "$pattern" -name "$name" | head -n1)
  if [[ -z "$found" ]]; then
    echo "ERROR: no $name matching $pattern under $DIST_DIR — run GoReleaser first" >&2
    exit 1
  fi
  echo "$found"
}

DARWIN_BIN=$(find_binary "*darwin_all*" "gitlab-mcp-server")
WINDOWS_BIN=$(find_binary "*windows_amd64*" "gitlab-mcp-server.exe")

BUNDLE_DIR="$DIST_DIR/mcpb-bundle"
rm -rf "$BUNDLE_DIR"
mkdir -p "$BUNDLE_DIR/server"

jq --arg v "$VERSION" '.version = $v' "$MANIFEST" > "$BUNDLE_DIR/manifest.json"
cp "$ICON" "$BUNDLE_DIR/icon.png"
cp "$DARWIN_BIN" "$BUNDLE_DIR/server/gitlab-mcp-server"
cp "$WINDOWS_BIN" "$BUNDLE_DIR/server/gitlab-mcp-server.exe"
chmod +x "$BUNDLE_DIR/server/gitlab-mcp-server"

OUTPUT="$DIST_DIR/gitlab-mcp-server.mcpb"
npx --yes "@anthropic-ai/mcpb@${MCPB_VERSION}" pack "$BUNDLE_DIR" "$OUTPUT"

echo "Built $OUTPUT (version $VERSION)"
