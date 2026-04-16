#!/usr/bin/env bash
# Update server.json with version, version-pinned download URLs,
# and SHA256 hashes from GoReleaser's checksums.txt.
#
# Usage: update-server-json-sha.sh <checksums-file> <version>
#
# Steps:
#   1. Sets .version to the given version
#   2. Replaces /releases/latest/download/ with /releases/download/v<version>/
#   3. Sets .fileSha256 for each package matching a checksum entry

set -euo pipefail

CHECKSUMS_FILE="${1:?Usage: $0 <checksums-file> <version>}"
VERSION="${2:?Usage: $0 <checksums-file> <version>}"
SERVER_JSON="server.json"

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

# 1. Update version
jq --arg v "$VERSION" '.version = $v' "$SERVER_JSON" > tmp.$$.json && mv tmp.$$.json "$SERVER_JSON"
echo "Version set to $VERSION"

# 2. Pin download URLs to this release version
jq --arg v "$VERSION" \
  '(.packages[].identifier) |= gsub("releases/latest/download"; "releases/download/v" + $v)' \
  "$SERVER_JSON" > tmp.$$.json && mv tmp.$$.json "$SERVER_JSON"
echo "Identifiers pinned to v$VERSION"

# 3. Update fileSha256 for each entry in checksums
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
