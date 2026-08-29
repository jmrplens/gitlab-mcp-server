#!/usr/bin/env bash
# Validate that every package server.json declares is actually published and is
# actually what it claims to be.
#
# `make check-server-json` validates the manifest against the registry schema.
# A schema cannot see that a package declared as registryType "mcpb" points at a
# raw ELF binary, that an OCI tag was never pushed, or that an OCI entry carries
# a field the registry rejects only at publish time. Those are exactly the
# mistakes that leave a directory listing scoring the server on an artifact no
# client can install, so they get their own gate.
#
# Usage: validate-server-json-packages.sh [server.json]
#
# Requires network access: it downloads each declared artifact.

set -euo pipefail

SERVER_JSON="${1:-server.json}"

for tool in jq curl python3 sha256sum; do
  if ! command -v "$tool" &> /dev/null; then
    echo "ERROR: $tool is required but not installed" >&2
    exit 1
  fi
done

if [[ ! -f "$SERVER_JSON" ]]; then
  echo "ERROR: $SERVER_JSON not found" >&2
  exit 1
fi

WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT

failures=0
checked=0

fail() {
  echo "  FAIL: $1" >&2
  failures=$((failures + 1))
}

server_name=$(jq -r '.name' "$SERVER_JSON")

# --- mcpb packages -----------------------------------------------------------
# A .mcpb is a zip carrying manifest.json plus the server binaries. Declaring a
# bare executable under this registryType parses fine and installs nowhere.
while read -r identifier; do
  [[ -z "$identifier" ]] && continue
  checked=$((checked + 1))
  echo "mcpb: $identifier"

  declared_hash=$(jq -r --arg id "$identifier" \
    '.packages[] | select(.identifier == $id) | .fileSha256 // ""' "$SERVER_JSON")

  if [[ "$identifier" != *.mcpb ]]; then
    fail "identifier does not name a .mcpb bundle"
    continue
  fi

  if [[ -z "$declared_hash" ]]; then
    fail "no fileSha256 declared"
    continue
  fi

  bundle="$WORKDIR/bundle.mcpb"
  if ! curl -fsSL -o "$bundle" "$identifier"; then
    fail "not downloadable"
    continue
  fi

  actual_hash=$(sha256sum "$bundle" | cut -d' ' -f1)
  if [[ "$actual_hash" != "$declared_hash" ]]; then
    fail "fileSha256 mismatch: declared ${declared_hash:0:16}..., actual ${actual_hash:0:16}..."
    continue
  fi

  if ! python3 -c '
import sys, zipfile
path = sys.argv[1]
if not zipfile.is_zipfile(path):
    with open(path, "rb") as fh:
        magic = fh.read(4).hex(" ")
    sys.exit("not a zip archive (magic bytes: %s)" % magic)
with zipfile.ZipFile(path) as bundle:
    if "manifest.json" not in bundle.namelist():
        sys.exit("zip archive carries no manifest.json")
' "$bundle"; then
    fail "not a valid MCP bundle"
    continue
  fi

  echo "  OK: ${actual_hash:0:16}..., valid bundle"
done < <(jq -r '.packages[] | select(.registryType == "mcpb") | .identifier' "$SERVER_JSON")

# --- oci packages ------------------------------------------------------------
# The registry validates ownership through an image label, and rejects version,
# registryBaseUrl and fileSha256 on an OCI entry — server-side, at publish time,
# long after any local schema check has passed.
while read -r identifier; do
  [[ -z "$identifier" ]] && continue
  checked=$((checked + 1))
  echo "oci: $identifier"

  banned=$(jq -r --arg id "$identifier" \
    '[.packages[] | select(.identifier == $id) | to_entries[]
      | select(.key == "version" or .key == "registryBaseUrl" or .key == "fileSha256")
      | .key] | join(", ")' "$SERVER_JSON")
  if [[ -n "$banned" ]]; then
    fail "carries field(s) the registry rejects for OCI packages: $banned"
  fi

  registry="${identifier%%/*}"
  if [[ "$registry" != "ghcr.io" ]]; then
    echo "  NOTE: $registry is not ghcr.io, skipping the image inspection"
    continue
  fi

  repo_and_tag="${identifier#*/}"
  repo="${repo_and_tag%%:*}"
  tag="${repo_and_tag##*:}"
  if [[ "$tag" == "$repo_and_tag" ]]; then
    fail "identifier carries no tag"
    continue
  fi

  token=$(curl -fsS "https://ghcr.io/token?scope=repository:${repo}:pull" \
    | jq -r '.token')
  accept='application/vnd.oci.image.index.v1+json,application/vnd.docker.distribution.manifest.list.v2+json,application/vnd.oci.image.manifest.v1+json'

  if ! index=$(curl -fsS -H "Authorization: Bearer $token" -H "Accept: $accept" \
    "https://ghcr.io/v2/${repo}/manifests/${tag}"); then
    fail "image tag not found in the registry"
    continue
  fi

  # A multi-arch tag resolves to an index; follow it to any one platform, since
  # the labels this checks are identical across them.
  child=$(echo "$index" | jq -r '.manifests[0].digest // ""')
  if [[ -n "$child" ]]; then
    manifest=$(curl -fsS -H "Authorization: Bearer $token" -H "Accept: $accept" \
      "https://ghcr.io/v2/${repo}/manifests/${child}")
  else
    manifest="$index"
  fi

  config_digest=$(echo "$manifest" | jq -r '.config.digest')
  config=$(curl -fsSL -H "Authorization: Bearer $token" \
    "https://ghcr.io/v2/${repo}/blobs/${config_digest}")

  label=$(echo "$config" | jq -r '.config.Labels["io.modelcontextprotocol.server.name"] // ""')
  if [[ "$label" != "$server_name" ]]; then
    fail "ownership label is \"$label\", expected \"$server_name\""
    continue
  fi

  # The image's own CMD starts an HTTP listener. An MCP client speaks stdio to
  # the container, so an entry that does not override it hangs at initialize.
  cmd=$(echo "$config" | jq -r '.config.Cmd // [] | join(" ")')
  args=$(jq -r --arg id "$identifier" \
    '[.packages[] | select(.identifier == $id) | .packageArguments // [] | .[].value] | join(" ")' \
    "$SERVER_JSON")
  if [[ "$cmd" == *"--http"* && "$args" != *"--http=false"* ]]; then
    fail "image CMD is \"$cmd\" but the package declares no --http=false override"
    continue
  fi

  echo "  OK: ownership label matches, stdio override declared"
done < <(jq -r '.packages[] | select(.registryType == "oci") | .identifier' "$SERVER_JSON")

if [[ "$checked" -eq 0 ]]; then
  echo "ERROR: no mcpb or oci packages found in $SERVER_JSON" >&2
  exit 1
fi

if [[ "$failures" -gt 0 ]]; then
  echo "$failures of $checked declared package(s) failed validation" >&2
  exit 1
fi

echo "All $checked declared package(s) validated"
