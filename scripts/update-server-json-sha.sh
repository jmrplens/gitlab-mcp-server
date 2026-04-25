#!/usr/bin/env bash
# Update server.json (MCP Registry manifest) and .plugin/plugin.json
# (Open Plugins manifest) with the release version, version-pinned download
# URLs, and SHA256 hashes from GoReleaser's checksums.txt.
#
# Usage: update-server-json-sha.sh <checksums-file> <version>
#
# Steps for server.json:
#   1. Sets top-level .version to the given version
#   2. Sets .packages[].version to the given version
#   3. Pins .packages[].identifier URLs to /releases/download/v<version>/,
#      handling both /releases/latest/download/ and prior /releases/download/vX.Y.Z/
#   4. Sets .fileSha256 for each package matching a checksum entry
#
# Steps for .plugin/plugin.json:
#   5. Sets top-level .version to the given version (if file exists)

set -euo pipefail

CHECKSUMS_FILE="${1:?Usage: $0 <checksums-file> <version>}"
VERSION="${2:?Usage: $0 <checksums-file> <version>}"
SERVER_JSON="server.json"
PLUGIN_JSON=".plugin/plugin.json"

if [[ ! -f "$CHECKSUMS_FILE" ]]; then
  echo "ERROR: checksums file not found: $CHECKSUMS_FILE" >&2
  exit 1
fi

if [[ ! -f "$SERVER_JSON" ]]; then
  echo "ERROR: $SERVER_JSON not found in current directory" >&2
  exit 1
fi

if ! command -v jq &> /dev/null; then
  echo "ERROR: jq is required but not installed" >&2
  exit 1
fi

# 1. Update top-level version
jq --arg v "$VERSION" '.version = $v' "$SERVER_JSON" > tmp.$$.json && mv tmp.$$.json "$SERVER_JSON"
echo "Top-level version set to $VERSION"

# 2. Update per-package version field (only for packages that already declare one)
jq --arg v "$VERSION" \
  '.packages |= map(if has("version") then .version = $v else . end)' \
  "$SERVER_JSON" > tmp.$$.json && mv tmp.$$.json "$SERVER_JSON"
echo "Per-package version fields set to $VERSION"

# 3. Pin identifier URLs to this release version.
# Handles both /releases/latest/download/ and previously-pinned /releases/download/vX.Y.Z/
jq --arg v "$VERSION" '
  (.packages[].identifier) |=
    (sub("releases/latest/download"; "releases/download/v" + $v)
     | sub("releases/download/v[0-9]+\\.[0-9]+\\.[0-9]+"; "releases/download/v" + $v))
' "$SERVER_JSON" > tmp.$$.json && mv tmp.$$.json "$SERVER_JSON"
echo "Identifiers pinned to v$VERSION"

# 4. Update fileSha256 for each entry in checksums
updated=0
while read -r hash filename; do
  [[ -z "${hash:-}" || -z "${filename:-}" ]] && continue

  match=$(jq --arg name "$filename" \
    '[.packages[] | select(.identifier | endswith($name))] | length' \
    "$SERVER_JSON")

  if [[ "$match" -gt 0 ]]; then
    jq --arg hash "$hash" --arg name "$filename" \
      '(.packages[] | select(.identifier | endswith($name))).fileSha256 = $hash' \
      "$SERVER_JSON" > tmp.$$.json && mv tmp.$$.json "$SERVER_JSON"
    echo "SHA256 for $filename: ${hash:0:16}..."
    ((updated++)) || true
  fi
done < "$CHECKSUMS_FILE"

total=$(jq '.packages | length' "$SERVER_JSON")
echo "Updated $updated of $total package entries"

if [[ "$updated" -eq 0 ]]; then
  echo "WARNING: no checksums matched any package identifier" >&2
  exit 1
fi

# 5. Update Open Plugins manifest version (if present)
if [[ -f "$PLUGIN_JSON" ]]; then
  jq --arg v "$VERSION" '.version = $v' "$PLUGIN_JSON" > tmp.$$.json && mv tmp.$$.json "$PLUGIN_JSON"
  echo "$PLUGIN_JSON version set to $VERSION"
else
  echo "NOTE: $PLUGIN_JSON not found, skipping Open Plugins manifest update"
fi
