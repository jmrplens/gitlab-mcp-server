#!/usr/bin/env bash
# Build the Claude Desktop extension bundle (gitlab-mcp-server.mcpb).
#
# Assembles a MCPB bundle directory from the checked-in manifest
# (mcpb/manifest.json), the 512x512 icon (mcpb/icon.png), and the release
# binaries produced by GoReleaser, then packs it as a zip:
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

for f in "$MANIFEST" "$ICON"; do
  if [[ ! -f "$f" ]]; then
    echo "ERROR: $f not found (run from the repository root)" >&2
    exit 1
  fi
done

for tool in jq zip; do
  if ! command -v "$tool" &> /dev/null; then
    echo "ERROR: $tool is required but not installed" >&2
    exit 1
  fi
done

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
rm -f "$OUTPUT"

# A .mcpb is a plain zip with manifest.json at its root — the layout above is
# the whole specification, and `zip` produces it. This used to shell out to
# `npx --yes @anthropic-ai/mcpb@<pin>`, which pinned the CLI's own version but
# resolved its nine caret-ranged dependencies fresh from the registry on every
# release, inside the job that holds the repository's signing and publishing
# identities. Nothing about packing a zip justifies that.
#
# Fixed entry timestamps so the same inputs produce the same bytes: server.json
# carries this file's SHA256, and a hash that changes because the clock moved
# tells a verifier nothing.
# 198001010000 is the zip epoch, and the -t form is the one both GNU and
# BSD/macOS touch accept (`make mcpb` runs on macOS for lipo).
find "$BUNDLE_DIR" -exec touch -t 198001010000 {} +
(
  cd "$BUNDLE_DIR"
  # manifest.json first: a reader that streams the archive finds the manifest
  # before the multi-megabyte binaries.
  zip -q -X -D "../$(basename "$OUTPUT")" manifest.json
  zip -q -X -D -r "../$(basename "$OUTPUT")" . -x manifest.json
)

if ! zip -sf "$OUTPUT" | grep -q "^  manifest.json$"; then
  echo "ERROR: $OUTPUT has no manifest.json at its root" >&2
  exit 1
fi

echo "Built $OUTPUT (version $VERSION)"
