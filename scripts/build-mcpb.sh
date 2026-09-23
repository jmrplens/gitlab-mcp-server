#!/usr/bin/env bash
# Build the Claude Desktop extension bundle (gitlab-mcp-server.mcpb).
#
# Assembles a MCPB bundle directory from the checked-in manifest
# (mcpb/manifest.json), the 512x512 icon (mcpb/icon.png), the Linux launcher
# (mcpb/linux/launch.sh) and the release binaries produced by GoReleaser, then
# packs it as a zip:
#
#   bundle/
#   ├── manifest.json                  (version stamped to <version>)
#   ├── icon.png
#   └── server/
#       ├── gitlab-mcp-server          (darwin universal: arm64 + amd64)
#       ├── gitlab-mcp-server.exe      (windows amd64)
#       └── linux/
#           ├── launch.sh              (picks the binary below by uname -m)
#           ├── gitlab-mcp-server-linux-amd64
#           └── gitlab-mcp-server-linux-arm64
#
# The manifest's platform_overrides are keyed by operating system only, so
# macOS runs the universal binary, Windows the amd64 executable, and Linux the
# launcher through /bin/sh, which chooses between the two Linux binaries.
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
OUTPUT="$DIST_DIR/gitlab-mcp-server.mcpb"
# The previous run's bundle is removed before anything can refuse this run, so
# a refusal on any path, from a missing input to a binary found twice, does not
# leave it in dist/ under the old version for a later step or a developer to
# take for this one. From here until every check below has passed, any exit
# removes the bundle, including one set -e forces on a command nobody expected
# to fail: a bundle that did not pass its own checks must not be left behind.
rm -f "$OUTPUT"
trap 'rm -f "$OUTPUT"' EXIT

MANIFEST="mcpb/manifest.json"
ICON="mcpb/icon.png"
LAUNCHER="mcpb/linux/launch.sh"

for f in "$MANIFEST" "$ICON" "$LAUNCHER"; do
  if [[ ! -f "$f" ]]; then
    echo "ERROR: $f not found (run from the repository root)" >&2
    exit 1
  fi
done

for tool in jq zip unzip; do
  if ! command -v "$tool" &> /dev/null; then
    echo "ERROR: $tool is required but not installed" >&2
    exit 1
  fi
done

# Locate the GoReleaser artifacts. Binary paths live in per-target build
# directories (dist/<id>_<goos>_<goarch>[_<variant>]/, such as
# gitlab-mcp-server_linux_amd64_v1); `make mcpb` writes dist/local_<goos>_<goarch>/
# instead. The darwin universal binary comes from the universal_binaries step
# (goarch "all"). Exactly one match is accepted: with two, which one find lists
# first is up to the filesystem, and the bundle would carry whichever it was.
find_binary() {
  local pattern="$1" name="$2" found="" count=0 path
  while IFS= read -r path; do
    found="$path"
    count=$((count + 1))
  done < <(find "$DIST_DIR" -type f -path "$pattern" -name "$name")
  if [[ $count -eq 0 ]]; then
    echo "ERROR: no $name matching $pattern under $DIST_DIR; run GoReleaser or make mcpb first" >&2
    exit 1
  fi
  if [[ $count -gt 1 ]]; then
    echo "ERROR: $count files named $name match $pattern under $DIST_DIR; remove the stale ones:" >&2
    find "$DIST_DIR" -type f -path "$pattern" -name "$name" >&2
    exit 1
  fi
  echo "$found"
}

DARWIN_BIN=$(find_binary "*darwin_all*" "gitlab-mcp-server")
WINDOWS_BIN=$(find_binary "*windows_amd64*" "gitlab-mcp-server.exe")
LINUX_AMD64_BIN=$(find_binary "*linux_amd64*" "gitlab-mcp-server")
LINUX_ARM64_BIN=$(find_binary "*linux_arm64*" "gitlab-mcp-server")

# Every entry the bundle carries, in archive order. The same list packs the
# archive and is what the archive is checked against afterwards.
ENTRIES=(
  manifest.json
  icon.png
  server/gitlab-mcp-server
  server/gitlab-mcp-server.exe
  server/linux/launch.sh
  server/linux/gitlab-mcp-server-linux-amd64
  server/linux/gitlab-mcp-server-linux-arm64
)
# The entries a host must be able to execute. Claude Desktop extracts every
# file 0600 and restores the execute bit only for entries whose recorded mode
# has owner-execute, so these are stored 0755 and checked after packing.
EXECUTABLES=(
  server/gitlab-mcp-server
  server/linux/launch.sh
  server/linux/gitlab-mcp-server-linux-amd64
  server/linux/gitlab-mcp-server-linux-arm64
)

BUNDLE_DIR="$DIST_DIR/mcpb-bundle"
rm -rf "$BUNDLE_DIR"
mkdir -p "$BUNDLE_DIR/server/linux"

jq --arg v "$VERSION" '.version = $v' "$MANIFEST" > "$BUNDLE_DIR/manifest.json"
cp "$ICON" "$BUNDLE_DIR/icon.png"
cp "$DARWIN_BIN" "$BUNDLE_DIR/server/gitlab-mcp-server"
cp "$WINDOWS_BIN" "$BUNDLE_DIR/server/gitlab-mcp-server.exe"
cp "$LAUNCHER" "$BUNDLE_DIR/server/linux/launch.sh"
cp "$LINUX_AMD64_BIN" "$BUNDLE_DIR/server/linux/gitlab-mcp-server-linux-amd64"
cp "$LINUX_ARM64_BIN" "$BUNDLE_DIR/server/linux/gitlab-mcp-server-linux-arm64"
# Every mode is set rather than inherited, since the zip records it and a mode
# that follows the builder's umask would make the same inputs pack differently.
(
  cd "$BUNDLE_DIR"
  chmod 0644 "${ENTRIES[@]}"
  chmod 0755 server/gitlab-mcp-server.exe "${EXECUTABLES[@]}"
)

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
  # The entries are named rather than recursed into, so their order is the one
  # ENTRIES gives and not the order the filesystem happens to list them in.
  # manifest.json comes first: a reader that streams the archive finds the
  # manifest before the multi-megabyte binaries.
  zip -q -X -D "../$(basename "$OUTPUT")" "${ENTRIES[@]}"
)

# --- What was packed -----------------------------------------------------------
# The checks below read the archive, not the staging directory, because the
# archive is what ships.
failures=0
fail() {
  echo "ERROR: $OUTPUT: $1" >&2
  failures=$((failures + 1))
}

# Exactly the expected entries, in order.
expected_list=$(printf '%s\n' "${ENTRIES[@]}")
actual_list=$(unzip -Z1 "$OUTPUT")
if [[ "$actual_list" != "$expected_list" ]]; then
  fail "the archive does not carry exactly the expected entries"
  diff <(echo "$expected_list") <(echo "$actual_list") >&2 || true
fi

# The executables are recorded as Unix files with mode 0755. A zip written
# without Unix attributes extracts every file 0600 in Claude Desktop.
for entry in "${EXECUTABLES[@]}"; do
  # An entry the archive lacks was reported by the entry-list check, and
  # zipinfo exits non-zero on it, which would end the script before the
  # removal below. zip itself exits 0 when an input is missing.
  grep -qxF "$entry" <<< "$actual_list" || continue
  recorded=$(unzip -Z "$OUTPUT" "$entry" | awk -v name="$entry" '$NF == name { print $1, $3 }' || true)
  if [[ "$recorded" != "-rwxr-xr-x unx" ]]; then
    fail "$entry is recorded as '${recorded:-nothing}', expected '-rwxr-xr-x unx'"
  fi
done

# What the packed manifest says, checked against what the archive carries.
# Claude Desktop resolves platform_overrides[process.platform] and substitutes
# ${__dirname} with the extension directory, so a path the manifest names and
# the archive lacks is a server that cannot start on that platform. Two rules
# the schema cannot express come from the same resolver: an override's env
# replaces the base env rather than merging with it, which would drop
# GITLAB_TOKEN and every other setting, and an override's args replace the
# base args.
#
# The platform rules run both ways. Every override has to be listed in
# compatibility.platforms, and every listed platform other than darwin has to
# have an override, since the base command is the macOS universal binary and a
# platform without an override would be handed that. The list has to name all
# three platforms the archive carries a server for: Desktop marks the bundle
# incompatible on a platform the list leaves out, and on one it lists with no
# override it would start the Mach-O binary.
entries_json=$(printf '%s\n' "${ENTRIES[@]}" | jq -R . | jq -s .)
check_manifest() {
  unzip -p "$OUTPUT" manifest.json | jq -r --arg v "$VERSION" --argjson entries "$entries_json" '
  (.server.mcp_config.platform_overrides // {}) as $overrides
  | (.compatibility.platforms // []) as $platforms
  | ([ .server.mcp_config.command, (.server.mcp_config.args // [])[],
       ($overrides[] | .command, (.args // [])[]) ]
     | map(select(type == "string" and contains("${__dirname}")))) as $named
  | [
      (select(.version != $v) | "manifest version is \(.version), expected \($v)"),
      (.server.entry_point as $e | select(($e | IN($entries[])) | not)
        | "server.entry_point \($e) is not in the archive"),
      (.icon as $i | select($i != null and (($i | IN($entries[])) | not))
        | "icon \($i) is not in the archive"),
      ($named[]
        | if startswith("${__dirname}/") then
            ltrimstr("${__dirname}/") as $rel
            | select(($rel | IN($entries[])) | not)
            | "\(.) names \($rel), which is not in the archive"
          else
            "\(.) uses ${__dirname} other than as a leading path component, so it cannot be checked"
          end),
      ($overrides | keys[]
        | select(IN("darwin", "win32", "linux") | not)
        | "platform_overrides.\(.) is not a platform Claude Desktop reports (darwin, win32 or linux)"),
      ($overrides | keys[]
        | select(IN($platforms[]) | not)
        | "platform_overrides.\(.) is not listed in compatibility.platforms"),
      ($platforms[]
        | select(. != "darwin")
        | select(IN($overrides | keys[]) | not)
        | "compatibility.platforms lists \(.) with no platform_overrides entry, so it would start the base command, the macOS binary"),
      ("darwin", "win32", "linux"
        | select(IN($platforms[]) | not)
        | "compatibility.platforms does not list \(.), although the archive carries its server"),
      ($overrides | to_entries[] | select(.value | has("env"))
        | "platform_overrides.\(.key) declares env, which would replace the base env"),
      (select((.server.mcp_config.args // []) | length > 0)
        | $overrides | to_entries[] | select(.value | has("args"))
        | "platform_overrides.\(.key) declares args, which would replace the non-empty base args")
    ]
  | .[]'
}
# A manifest the entry-list check already found missing is not read again:
# unzip would exit non-zero on it and end the script before the removal below.
if grep -qxF manifest.json <<< "$actual_list"; then
  if manifest_errors=$(check_manifest); then
    if [[ -n "$manifest_errors" ]]; then
      while IFS= read -r line; do
        fail "$line"
      done <<< "$manifest_errors"
    fi
  else
    fail "the packed manifest.json could not be read and checked"
  fi
fi

if [[ $failures -gt 0 ]]; then
  # Removed so that no later step, and no developer, picks up a bundle that
  # failed its own checks.
  rm -f "$OUTPUT"
  echo "ERROR: $OUTPUT failed $failures check(s) and was removed" >&2
  exit 1
fi

trap - EXIT
echo "Built $OUTPUT (version $VERSION)"
unzip -Z -l "$OUTPUT"
