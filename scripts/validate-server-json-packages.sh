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

# Every request here talks to a third-party registry, so each one is bounded:
# a stalled response would otherwise hang this gate until the workflow's own
# timeout fires, with no indication of which host went quiet. The bundle needs
# a longer transfer window than the metadata calls because it is tens of MB.
CURL_CONNECT_TIMEOUT=10
CURL_MAX_TIME=60
CURL_DOWNLOAD_MAX_TIME=300
CURL_META=(--connect-timeout "$CURL_CONNECT_TIMEOUT" --max-time "$CURL_MAX_TIME" --retry 2 --retry-connrefused)
CURL_DOWNLOAD=(--connect-timeout "$CURL_CONNECT_TIMEOUT" --max-time "$CURL_DOWNLOAD_MAX_TIME" --retry 2 --retry-connrefused)

failures=0
checked=0

fail() {
  echo "  FAIL: $1" >&2
  failures=$((failures + 1))
}

warn() {
  echo "  WARNING: $1" >&2
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
  if ! curl "${CURL_DOWNLOAD[@]}" -fsSL -o "$bundle" "$identifier"; then
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

  # The registry accepts repo:tag, repo@digest and repo:tag@digest. This
  # manifest uses the third form: the tag stays readable, the digest is what a
  # client resolves.
  ref="${identifier#*/}"
  digest=""
  if [[ "$ref" == *"@"* ]]; then
    digest="${ref##*@}"
    ref="${ref%@*}"
  fi
  repo="${ref%%:*}"
  tag=""
  [[ "$ref" == *":"* ]] && tag="${ref##*:}"
  if [[ -z "$tag" && -z "$digest" ]]; then
    fail "identifier carries neither a tag nor a digest"
    continue
  fi
  if [[ -n "$digest" && ! "$digest" =~ ^sha256:[0-9a-f]{64}$ ]]; then
    fail "digest is not a sha256:<64 hex> reference: $digest"
    continue
  fi

  token=$(curl "${CURL_META[@]}" -fsS "https://ghcr.io/token?scope=repository:${repo}:pull" \
    | jq -r '.token')
  accept='application/vnd.oci.image.index.v1+json,application/vnd.docker.distribution.manifest.list.v2+json,application/vnd.oci.image.manifest.v1+json'

  # Resolve the digest when there is one — that is what a client installs.
  if ! index=$(curl "${CURL_META[@]}" -fsS -H "Authorization: Bearer $token" -H "Accept: $accept" \
    "https://ghcr.io/v2/${repo}/manifests/${digest:-$tag}"); then
    fail "image ${digest:-$tag} not found in the registry"
    continue
  fi

  # A published version tag must never move. If it no longer resolves to the
  # pinned digest, either the tag was re-pushed or the manifest is stale — both
  # mean the reference no longer describes what this release shipped.
  if [[ -n "$digest" && -n "$tag" ]]; then
    tag_digest=$(curl "${CURL_META[@]}" -fsSI -H "Authorization: Bearer $token" -H "Accept: $accept" \
      "https://ghcr.io/v2/${repo}/manifests/${tag}" \
      | awk 'BEGIN { IGNORECASE = 1 } /^docker-content-digest:/ { print $2 }' | tr -d '\r')
    if [[ -z "$tag_digest" ]]; then
      fail "tag $tag does not resolve in the registry"
      continue
    fi
    if [[ "$tag_digest" != "$digest" ]]; then
      fail "tag $tag resolves to $tag_digest but the manifest pins $digest"
      continue
    fi
  fi

  # A multi-arch tag resolves to an index. Check EVERY runnable manifest, not
  # just the first: reading .manifests[0] means only linux/amd64 was ever
  # inspected, which is how a linux/arm64 image that could not exec at all
  # stayed published and digest-pinned here across a whole release. Attestation
  # manifests (buildkit's provenance/SBOM entries, platform unknown/unknown)
  # carry no runtime config and are skipped.
  mapfile -t children < <(echo "$index" | jq -r '
    (.manifests // []) | .[]
    | select((.platform.os // "") != "unknown")
    | "\(.digest)\t\(.platform.os // "?")/\(.platform.architecture // "?")"')
  if [[ ${#children[@]} -eq 0 ]]; then
    # $'\t' outside the quotes: inside them it is a backslash and a t, which the
    # tab-splitting below never finds, so both halves came out as the whole
    # string and the empty-digest guard could not fire.
    children=("$(echo "$index" | jq -r '.config.digest // ""')"$'\t'"single")
    manifests_are_index=0
  else
    manifests_are_index=1
  fi

  platforms_checked=0
  entry_failed=0
  for child_entry in "${children[@]}"; do
    child=${child_entry%%$'\t'*}
    child_platform=${child_entry##*$'\t'}
    if [[ "$manifests_are_index" -eq 1 ]]; then
      [[ -n "$child" ]] || continue
      manifest=$(curl "${CURL_META[@]}" -fsS -H "Authorization: Bearer $token" -H "Accept: $accept" \
        "https://ghcr.io/v2/${repo}/manifests/${child}")
    else
      manifest="$index"
    fi

    # A manifest with no config declares no labels either, and fetching
    # blobs/null aborts the whole run under `set -e` without a FAIL line naming
    # the package that failed.
    config_digest=$(echo "$manifest" | jq -r '.config.digest // ""')
    if [[ -z "$config_digest" || "$config_digest" == "null" ]]; then
      fail "$child_platform: the image manifest declares no config digest"
      entry_failed=1
      continue
    fi
    if ! config=$(curl "${CURL_META[@]}" -fsSL -H "Authorization: Bearer $token" \
      "https://ghcr.io/v2/${repo}/blobs/${config_digest}"); then
      fail "$child_platform: image config blob $config_digest could not be fetched"
      entry_failed=1
      continue
    fi

    label=$(echo "$config" | jq -r '.config.Labels["io.modelcontextprotocol.server.name"] // ""')
    if [[ "$label" != "$server_name" ]]; then
      fail "$child_platform: ownership label is \"$label\", expected \"$server_name\""
      entry_failed=1
      continue
    fi
    platforms_checked=$((platforms_checked + 1))
  done
  if [[ "$entry_failed" -eq 1 ]]; then
    continue
  fi
  if [[ "$platforms_checked" -eq 0 ]]; then
    fail "the index declares no runnable platform manifest"
    continue
  fi
  echo "  checked $platforms_checked platform manifest(s)"

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

  if [[ -n "$digest" ]]; then
    echo "  OK: digest resolves, tag agrees, ownership label matches, stdio override declared"
  else
    echo "  OK: ownership label matches, stdio override declared"
  fi
done < <(jq -r '.packages[] | select(.registryType == "oci") | .identifier' "$SERVER_JSON")

# --- npm packages ------------------------------------------------------------
# The registry validates npm ownership server-side: it fetches the published
# version's package.json and requires its mcpName to equal the server name; it
# also requires a version and rejects fileSha256. release.yml publishes npm
# before mcp-publisher runs, so the committed launcher manifest is the one live
# at registry-publish time — the lockstep below is therefore the real invariant.
NPM_MAIN="npm/gitlab-mcp-server/package.json"
while IFS=$'\t' read -r identifier version; do
  [[ -z "$identifier" ]] && continue
  checked=$((checked + 1))
  echo "npm: $identifier@$version"

  banned=$(jq -r --arg id "$identifier" \
    '[.packages[] | select(.registryType == "npm" and .identifier == $id) | to_entries[]
      | select(.key == "fileSha256") | .key] | join(", ")' "$SERVER_JSON")
  if [[ -n "$banned" ]]; then
    fail "carries field(s) the registry rejects for npm packages: $banned"
  fi

  if [[ -z "$version" || "$version" == "null" ]]; then
    fail "declares no version (the registry requires one for npm entries)"
    continue
  fi

  if [[ ! -f "$NPM_MAIN" ]]; then
    fail "$NPM_MAIN not found"
    continue
  fi

  launcher_name=$(jq -r '.name' "$NPM_MAIN")
  if [[ "$identifier" != "$launcher_name" ]]; then
    fail "identifier is \"$identifier\" but the committed launcher is \"$launcher_name\""
    continue
  fi

  launcher_mcp=$(jq -r '.mcpName // ""' "$NPM_MAIN")
  if [[ "$launcher_mcp" != "$server_name" ]]; then
    fail "launcher mcpName is \"$launcher_mcp\", expected \"$server_name\" (the registry's npm ownership check fails without it)"
    continue
  fi

  launcher_version=$(jq -r '.version' "$NPM_MAIN")
  if [[ "$version" != "$launcher_version" ]]; then
    fail "declared version $version does not match the committed launcher version $launcher_version"
    continue
  fi

  encoded=${identifier//\//%2F}
  if ! meta=$(curl "${CURL_META[@]}" -fsSL "https://registry.npmjs.org/${encoded}/${version}"); then
    fail "version $version not found on registry.npmjs.org"
    continue
  fi
  published_mcp=$(echo "$meta" | jq -r '.mcpName // ""')
  if [[ -z "$published_mcp" ]]; then
    echo "  NOTE: published $version predates mcpName; the registry accepts this entry starting with the release that publishes it"
  elif [[ "$published_mcp" != "$server_name" ]]; then
    fail "published mcpName is \"$published_mcp\", expected \"$server_name\""
  else
    echo "  OK: name, mcpName and version in lockstep; published mcpName matches"
  fi
done < <(jq -r '.packages[] | select(.registryType == "npm") | [.identifier, (.version // "null")] | @tsv' "$SERVER_JSON")

# --- pypi packages: the registry validates ownership through an
# "mcp-name: <server-name>" token in the published version's README
# (long_description), fetched from the PyPI JSON API. fileSha256 is
# rejected server-side like the other non-mcpb types.
while IFS=$'\t' read -r identifier version; do
  checked=$((checked + 1))
  echo "pypi package: $identifier@$version"

  banned=$(jq -r --arg id "$identifier" \
    '[.packages[] | select(.registryType == "pypi" and .identifier == $id) | to_entries[]
      | select(.key == "fileSha256") | .key] | join(", ")' "$SERVER_JSON")
  if [[ -n "$banned" ]]; then
    fail "carries field(s) the registry rejects for pypi packages: $banned"
  fi

  if [[ -z "$version" || "$version" == "null" ]]; then
    fail "declares no version (the registry requires one for pypi entries)"
    continue
  fi

  if ! meta=$(curl "${CURL_META[@]}" -fsSL "https://pypi.org/pypi/${identifier}/${version}/json"); then
    fail "version $version not found on pypi.org"
    continue
  fi
  published_desc=$(echo "$meta" | jq -r '.info.description // ""')
  if ! grep -qE "(^|[[:space:]])mcp-name: ${server_name}([[:space:]]|$)" <<<"$published_desc"; then
    fail "published README lacks the \"mcp-name: ${server_name}\" ownership token the registry validates"
  else
    echo "  OK: published version carries the mcp-name ownership token"
  fi
done < <(jq -r '.packages[] | select(.registryType == "pypi") | [.identifier, (.version // "null")] | @tsv' "$SERVER_JSON")

# --- release immutability ----------------------------------------------------
# Every other artefact this script checks is pinned by a hash or an immutable
# registry version. The GitHub Release is not: unless immutable releases are
# enabled on the repository, anyone holding contents: write can replace a
# published binary AND its checksums.txt together, and both installers accept
# the pair. This is a drift alarm, not a merge gate — the setting lives in the
# repository, releases published before it was turned on stay mutable forever,
# and the installer-side signature check is the part that travels.
release_repo="${GITHUB_REPOSITORY:-jmrplens/gitlab-mcp-server}"
release_version=$(jq -r '[.packages[] | select(.registryType == "npm") | .version] | first // ""' "$SERVER_JSON")
if [[ -n "$release_version" && "$release_version" != "null" ]]; then
  echo "release: v${release_version}"
  gh_auth=()
  if [[ -n "${GH_TOKEN:-}" ]]; then
    gh_auth=(-H "Authorization: Bearer ${GH_TOKEN}")
  fi
  release_meta=$(curl "${CURL_META[@]}" -fsSL \
    -H "Accept: application/vnd.github+json" \
    "${gh_auth[@]}" \
    "https://api.github.com/repos/${release_repo}/releases/tags/v${release_version}" || true)
  if [[ -z "$release_meta" ]]; then
    warn "could not read the v${release_version} release metadata; immutability not checked"
  elif [[ "$(echo "$release_meta" | jq -r '.immutable // false')" != "true" ]]; then
    warn "release v${release_version} is mutable — its assets and checksums.txt can be replaced together. Enable immutable releases in the repository settings; the workflow already publishes drafts, which the feature requires."
  else
    echo "  OK: release v${release_version} is immutable"
  fi
fi

if [[ "$checked" -eq 0 ]]; then
  echo "ERROR: no mcpb, oci, npm or pypi packages found in $SERVER_JSON" >&2
  exit 1
fi

if [[ "$failures" -gt 0 ]]; then
  echo "$failures of $checked declared package(s) failed validation" >&2
  exit 1
fi

echo "All $checked declared package(s) validated"
