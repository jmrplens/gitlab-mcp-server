#!/usr/bin/env bash
# Build the Claude Desktop extension bundles (.mcpb).
#
# One run builds four bundles from the same release binaries produced by
# GoReleaser, the checked-in manifest (mcpb/manifest.json), the derivation of
# its per-platform form (mcpb/platform.jq), the 512x512 icon (mcpb/icon.png),
# the Linux launcher (mcpb/linux/launch.sh), the repository's LICENSE, and
# <dist-dir>/THIRD_PARTY_NOTICES, the license, notice and patent texts of what
# the binaries link, which GoReleaser generates beside them in a release and
# `make mcpb` generates for its own builds (cmd/gen_third_party_notices):
#
#   gitlab-mcp-server-darwin.mcpb    macOS: the universal binary (arm64 + amd64)
#   gitlab-mcp-server-windows.mcpb   Windows: the amd64 executable
#   gitlab-mcp-server-linux.mcpb     Linux: the launcher and both Linux binaries
#   gitlab-mcp-server.mcpb           all three servers, the universal bundle
#
# The three per-OS bundles are what server.json declares, one registry entry
# each. A user downloads only the server their system runs, and each manifest
# lists only its own platform, so Claude Desktop refuses one opened on another
# system with a clear message instead of starting a binary that is not there.
# The universal bundle keeps the name it has always had and is still published,
# because links to it exist outside this repository; server.json does not
# declare it, since a registry entry has no platform field and a client could
# not tell it from the three that each serve one system.
#
# Every bundle uses the same layout, carrying the entries its system needs, so
# a path means the same thing in all four:
#
#   bundle/
#   ├── manifest.json                  (version stamped to <version>)
#   ├── icon.png
#   ├── LICENSE
#   ├── THIRD_PARTY_NOTICES
#   └── server/
#       ├── gitlab-mcp-server          (darwin universal: arm64 + amd64)
#       ├── gitlab-mcp-server.exe      (windows amd64)
#       └── linux/
#           ├── launch.sh              (picks the binary below by uname -m)
#           ├── gitlab-mcp-server-linux-amd64
#           └── gitlab-mcp-server-linux-arm64
#
# The universal manifest's platform_overrides are keyed by operating system
# only, so macOS runs the universal binary, Windows the amd64 executable, and
# Linux the launcher through /bin/sh, which chooses between the two Linux
# binaries. A per-OS manifest makes its own platform's command the base command
# and carries no override (mcpb/platform.jq).
#
# Usage: build-mcpb.sh <version> [dist-dir]
#
#   <version>   Release version without the leading v (e.g. 2.5.0)
#   [dist-dir]  GoReleaser output directory (default: dist)
#
# Output: the four bundles above in <dist-dir>. Each one's download and
# unpacked size is printed, and written to the job summary in GitHub Actions.

set -euo pipefail

VERSION="${1:?Usage: $0 <version> [dist-dir]}"
DIST_DIR="${2:-dist}"

# Each bundle this run builds, by the name its target goes by below. The
# per-OS ones come first so their sizes lead the report.
TARGETS=(darwin windows linux universal)
output_of() {
  if [[ "$1" == universal ]]; then
    echo "$DIST_DIR/gitlab-mcp-server.mcpb"
  else
    echo "$DIST_DIR/gitlab-mcp-server-$1.mcpb"
  fi
}
OUTPUTS=()
for target in "${TARGETS[@]}"; do
  OUTPUTS+=("$(output_of "$target")")
done
# The previous run's bundles are removed before anything can refuse this run,
# so a refusal on any path, from a missing input to a binary found twice, does
# not leave them in dist/ under the old version for a later step or a
# developer to take for this one. From here until every check below has
# passed, any exit removes them, including one set -e forces on a command
# nobody expected to fail: a bundle that did not pass its own checks must not
# be left behind. They go as a set, since the release declares three of them
# and a later step handed two would publish a registry entry short of a system.
rm -f "${OUTPUTS[@]}"
trap 'rm -f "${OUTPUTS[@]}"' EXIT

MANIFEST="mcpb/manifest.json"
PLATFORM_JQ="mcpb/platform.jq"
ICON="mcpb/icon.png"
LAUNCHER="mcpb/linux/launch.sh"
# The licence travels with the binaries it covers: a bundle is a redistribution
# of the server, and MIT asks for its notice to accompany every copy.
LICENSE_FILE="LICENSE"
# The notices of the modules those binaries link travel with them for the
# same reason, and are generated with the binaries rather than kept here.
NOTICES_FILE="$DIST_DIR/THIRD_PARTY_NOTICES"
NOTICES_HEADER="Third-party notices for gitlab-mcp-server"

for f in "$MANIFEST" "$PLATFORM_JQ" "$ICON" "$LAUNCHER" "$LICENSE_FILE"; do
  if [[ ! -f "$f" ]]; then
    echo "ERROR: $f not found (run from the repository root)" >&2
    exit 1
  fi
done
if [[ ! -f "$NOTICES_FILE" ]]; then
  echo "ERROR: $NOTICES_FILE not found: GoReleaser writes it beside the release binaries and make mcpb beside its own (cmd/gen_third_party_notices)" >&2
  exit 1
fi
if [[ "$(head -n 1 "$NOTICES_FILE")" != "$NOTICES_HEADER" ]]; then
  echo "ERROR: $NOTICES_FILE does not open with '$NOTICES_HEADER', so it is not what cmd/gen_third_party_notices writes" >&2
  exit 1
fi

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

# The entries the bundles carry, in archive order. The list a target selects
# below packs its archive and is what that archive is checked against
# afterwards. manifest.json comes first: a reader that streams the archive
# finds the manifest before the multi-megabyte binaries.
COMMON_ENTRIES=(manifest.json icon.png LICENSE THIRD_PARTY_NOTICES)
DARWIN_ENTRIES=(server/gitlab-mcp-server)
WINDOWS_ENTRIES=(server/gitlab-mcp-server.exe)
LINUX_ENTRIES=(
  server/linux/launch.sh
  server/linux/gitlab-mcp-server-linux-amd64
  server/linux/gitlab-mcp-server-linux-arm64
)
# The entries a host must be able to execute. Claude Desktop extracts every
# file 0600 and restores the execute bit only for entries whose recorded mode
# has owner-execute, so these are stored 0755 and checked after packing. The
# Windows executable is stored 0755 too, which Windows ignores.
EXECUTABLES=(
  server/gitlab-mcp-server
  server/linux/launch.sh
  server/linux/gitlab-mcp-server-linux-amd64
  server/linux/gitlab-mcp-server-linux-arm64
)

# Selects what one target carries and serves. PLATFORM is the value its
# per-OS manifest is derived for, and empty for the universal bundle, whose
# manifest is the committed one. PLATFORMS is the compatibility.platforms list
# the packed manifest has to declare, exactly. BASE_PLATFORM is the platform
# the manifest's base command serves, the one listed platform that needs no
# override.
select_target() {
  case "$1" in
    darwin)
      ENTRIES=("${COMMON_ENTRIES[@]}" "${DARWIN_ENTRIES[@]}")
      PLATFORM=darwin
      PLATFORMS='["darwin"]'
      ;;
    windows)
      ENTRIES=("${COMMON_ENTRIES[@]}" "${WINDOWS_ENTRIES[@]}")
      PLATFORM=win32
      PLATFORMS='["win32"]'
      ;;
    linux)
      ENTRIES=("${COMMON_ENTRIES[@]}" "${LINUX_ENTRIES[@]}")
      PLATFORM=linux
      PLATFORMS='["linux"]'
      ;;
    universal)
      ENTRIES=("${COMMON_ENTRIES[@]}" "${DARWIN_ENTRIES[@]}" "${WINDOWS_ENTRIES[@]}" "${LINUX_ENTRIES[@]}")
      PLATFORM=""
      PLATFORMS='["darwin","win32","linux"]'
      ;;
  esac
  BASE_PLATFORM=${PLATFORM:-darwin}
}

# Where each entry other than the manifest is copied from.
source_of() {
  case "$1" in
    icon.png) echo "$ICON" ;;
    LICENSE) echo "$LICENSE_FILE" ;;
    THIRD_PARTY_NOTICES) echo "$NOTICES_FILE" ;;
    server/gitlab-mcp-server) echo "$DARWIN_BIN" ;;
    server/gitlab-mcp-server.exe) echo "$WINDOWS_BIN" ;;
    server/linux/launch.sh) echo "$LAUNCHER" ;;
    server/linux/gitlab-mcp-server-linux-amd64) echo "$LINUX_AMD64_BIN" ;;
    server/linux/gitlab-mcp-server-linux-arm64) echo "$LINUX_ARM64_BIN" ;;
  esac
}

STAGING="$DIST_DIR/mcpb-bundle"
rm -rf "$STAGING"
DIST_ABS=$(cd "$DIST_DIR" && pwd)

# Stages and packs the selected target into $1.
pack() {
  local output="$1" stage entry
  stage="$STAGING/$(basename "$output" .mcpb)"
  mkdir -p "$stage"
  if [[ -z "$PLATFORM" ]]; then
    jq --arg v "$VERSION" '.version = $v' "$MANIFEST" > "$stage/manifest.json"
  else
    jq --arg platform "$PLATFORM" -f "$PLATFORM_JQ" "$MANIFEST" \
      | jq --arg v "$VERSION" '.version = $v' > "$stage/manifest.json"
  fi
  for entry in "${ENTRIES[@]}"; do
    [[ "$entry" == manifest.json ]] && continue
    mkdir -p "$stage/$(dirname "$entry")"
    cp "$(source_of "$entry")" "$stage/$entry"
  done
  # Every mode is set rather than inherited, since the zip records it and a
  # mode that follows the builder's umask would make the same inputs pack
  # differently.
  (
    cd "$stage"
    chmod 0644 "${ENTRIES[@]}"
    for entry in "${ENTRIES[@]}"; do
      case "$entry" in
        server/*) chmod 0755 "$entry" ;;
      esac
    done
  )

  # A .mcpb is a plain zip with manifest.json at its root: the layout above
  # is the whole specification, and `zip` produces it. This used to shell out
  # to `npx --yes @anthropic-ai/mcpb@<pin>`, which pinned the CLI's own version
  # but resolved its nine caret-ranged dependencies fresh from the registry on
  # every release, inside the job that holds the repository's signing and
  # publishing identities. Nothing about packing a zip justifies that.
  #
  # Fixed entry timestamps so the same inputs produce the same bytes:
  # server.json carries each declared bundle's SHA256, and a hash that changes
  # because the clock moved tells a verifier nothing.
  # 198001010000 is the zip epoch, and the -t form is the one both GNU and
  # BSD/macOS touch accept (`make mcpb` runs on macOS for lipo).
  find "$stage" -exec touch -t 198001010000 {} +
  (
    cd "$stage"
    # The entries are named rather than recursed into, so their order is the
    # one ENTRIES gives and not the order the filesystem happens to list them
    # in.
    zip -q -X -D "$DIST_ABS/$(basename "$output")" "${ENTRIES[@]}"
  )
}

# --- What was packed -----------------------------------------------------------
# The checks below read each archive, not its staging directory, because the
# archive is what ships.
failures=0
fail() {
  echo "ERROR: $1" >&2
  failures=$((failures + 1))
}

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
# compatibility.platforms, and every listed platform other than the one the
# base command serves has to have an override, since a platform without one
# would be handed the base command: the macOS binary in the universal bundle.
# The list has to name exactly the platforms the archive carries a server for:
# Desktop marks the bundle incompatible on a platform the list leaves out, and
# on one it lists that the archive has no server for it would start a command
# that is not there.
check_manifest() {
  local output="$1" entries_json
  entries_json=$(printf '%s\n' "${ENTRIES[@]}" | jq -R . | jq -s .)
  unzip -p "$output" manifest.json | jq -r --arg v "$VERSION" --arg base "$BASE_PLATFORM" \
    --argjson entries "$entries_json" --argjson expected "$PLATFORMS" '
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
        | select(. != $base)
        | select(IN($overrides | keys[]) | not)
        | "compatibility.platforms lists \(.) with no platform_overrides entry, so it would start the base command, which serves \($base)"),
      ($expected[]
        | select(IN($platforms[]) | not)
        | "compatibility.platforms does not list \(.), although the archive carries its server"),
      ($platforms[]
        | select(IN($expected[]) | not)
        | "compatibility.platforms lists \(.), although the archive carries no server for it"),
      ($overrides | to_entries[] | select(.value | has("env"))
        | "platform_overrides.\(.key) declares env, which would replace the base env"),
      (select((.server.mcp_config.args // []) | length > 0)
        | $overrides | to_entries[] | select(.value | has("args"))
        | "platform_overrides.\(.key) declares args, which would replace the non-empty base args")
    ]
  | .[]'
}

# Checks the entries, their modes and the manifest of the selected target's
# archive $1.
check_bundle() {
  local output="$1" entry expected_list actual_list recorded manifest_errors line
  # Exactly the expected entries, in order.
  expected_list=$(printf '%s\n' "${ENTRIES[@]}")
  actual_list=$(unzip -Z1 "$output")
  if [[ "$actual_list" != "$expected_list" ]]; then
    fail "$output: the archive does not carry exactly the expected entries"
    diff <(echo "$expected_list") <(echo "$actual_list") >&2 || true
  fi

  # The executables are recorded as Unix files with mode 0755. A zip written
  # without Unix attributes extracts every file 0600 in Claude Desktop.
  for entry in "${EXECUTABLES[@]}"; do
    # Only the executables this bundle carries. An entry the archive lacks was
    # reported by the entry-list check, and zipinfo exits non-zero on it,
    # which would end the script before the removal below. zip itself exits 0
    # when an input is missing.
    grep -qxF "$entry" <<< "$expected_list" || continue
    grep -qxF "$entry" <<< "$actual_list" || continue
    recorded=$(unzip -Z "$output" "$entry" | awk -v name="$entry" '$NF == name { print $1, $3 }' || true)
    if [[ "$recorded" != "-rwxr-xr-x unx" ]]; then
      fail "$output: $entry is recorded as '${recorded:-nothing}', expected '-rwxr-xr-x unx'"
    fi
  done

  # A manifest the entry-list check already found missing is not read again:
  # unzip would exit non-zero on it and end the script before the removal
  # below.
  if grep -qxF manifest.json <<< "$actual_list"; then
    if manifest_errors=$(check_manifest "$output"); then
      if [[ -n "$manifest_errors" ]]; then
        while IFS= read -r line; do
          fail "$output: $line"
        done <<< "$manifest_errors"
      fi
    else
      fail "$output: the packed manifest.json could not be read and checked"
    fi
  fi
}

# --- How big each bundle is ----------------------------------------------------
# Claude Desktop's own limits, read from its extension runtime (2.7032.0, the
# extraction policy it applies to an extension): it refuses an archive with
# more than 100,000 entries, any entry that unpacks past 512 MiB, more than
# 2048 MiB unpacked in all, and an archive that unpacks to more than 50 times
# its own size. A bundle past any of them cannot be installed at all, so each
# one fails the build.
DESKTOP_MAX_ENTRIES=100000
DESKTOP_MAX_ENTRY_BYTES=$((512 * 1024 * 1024))
DESKTOP_MAX_UNPACKED_BYTES=$((2048 * 1024 * 1024))
DESKTOP_MAX_RATIO=50
# Directories that score MCP servers stop reading a bundle past 50 MiB to
# download or 256 MiB unpacked (verifymcp.io's inspection limits), and then
# report its provenance, licence and maintenance as unverified; 3.1.0's single
# bundle, at 73 MiB and 285 MiB, lost all three that way. Every user also
# downloads the whole file. Neither is a reason to refuse a release, so a
# declared bundle past either size is a warning. The universal bundle is not
# declared anywhere a directory reads, and carries all three systems' servers
# by design, so it is measured and not warned about.
DIRECTORY_WARN_DOWNLOAD_BYTES=$((50 * 1024 * 1024))
DIRECTORY_WARN_UNPACKED_BYTES=$((256 * 1024 * 1024))

warn() {
  # A workflow command on stdout becomes an annotation on the run's summary.
  if [[ "${GITHUB_ACTIONS:-}" == true ]]; then
    echo "::warning::$1"
  else
    echo "WARNING: $1" >&2
  fi
}

# Bytes as MiB with two decimals, rounded half up.
mib() {
  local hundredths=$((($1 * 100 + 524288) / 1048576))
  printf '%d.%02d' $((hundredths / 100)) $((hundredths % 100))
}

# One row per bundle for the job summary, filled in as each one is measured.
REPORT_ROWS=()

# Measures the archive $1 of target $2, reports its sizes and checks them.
check_size() {
  local output="$1" target="$2" name listing size archive unpacked=0 largest=0 count=0 tenths
  name=$(basename "$output")
  # Every figure comes from one zipinfo listing of the archive: its header
  # gives the archive's size in bytes, and each regular file's line starts
  # with '-' and gives its unpacked size in the fourth column. The sums are
  # taken in the shell, whose integers are 64 bits wide, since some awk builds
  # print a sum past 2^31 in exponent form.
  listing=$(unzip -Zl "$output")
  archive=$(awk '/^Zip file size:/ { print $4; exit }' <<< "$listing")
  if [[ ! "$archive" =~ ^[1-9][0-9]*$ ]]; then
    fail "$output: zipinfo gave no archive size, so the bundle could not be measured"
    return 0
  fi
  while read -r size; do
    count=$((count + 1))
    unpacked=$((unpacked + size))
    if ((size > largest)); then
      largest=$size
    fi
  done < <(awk '$1 ~ /^-/ { print $4 }' <<< "$listing")
  tenths=$((unpacked * 10 / archive))

  echo "$name: $(mib "$archive") MiB to download, $(mib "$unpacked") MiB unpacked, $count entries, largest $(mib "$largest") MiB, ratio $((tenths / 10)).$((tenths % 10)):1"
  local declared=yes
  [[ "$target" == universal ]] && declared="no, kept for existing links"
  REPORT_ROWS+=("| \`$name\` | $(mib "$archive") MiB | $(mib "$unpacked") MiB | $declared |")

  if ((count > DESKTOP_MAX_ENTRIES)); then
    fail "$output: $count entries; Claude Desktop refuses an extension with more than $DESKTOP_MAX_ENTRIES"
  fi
  if ((largest > DESKTOP_MAX_ENTRY_BYTES)); then
    fail "$output: an entry unpacks to $largest bytes; Claude Desktop refuses an entry over $DESKTOP_MAX_ENTRY_BYTES (512 MiB)"
  fi
  if ((unpacked > DESKTOP_MAX_UNPACKED_BYTES)); then
    fail "$output: unpacks to $unpacked bytes; Claude Desktop refuses more than $DESKTOP_MAX_UNPACKED_BYTES (2048 MiB)"
  fi
  if ((unpacked > DESKTOP_MAX_RATIO * archive)); then
    fail "$output: unpacks to more than $DESKTOP_MAX_RATIO times its $archive bytes; Claude Desktop refuses it as a zip bomb"
  fi

  [[ "$target" == universal ]] && return 0
  if ((archive > DIRECTORY_WARN_DOWNLOAD_BYTES)); then
    warn "$name is $(mib "$archive") MiB to download, over the 50 MiB past which directories stop reading a bundle and report its provenance, licence and maintenance as unverified"
  fi
  if ((unpacked > DIRECTORY_WARN_UNPACKED_BYTES)); then
    warn "$name unpacks to $(mib "$unpacked") MiB, over the 256 MiB past which directories stop reading a bundle and report its provenance, licence and maintenance as unverified"
  fi
  return 0
}

for target in "${TARGETS[@]}"; do
  output=$(output_of "$target")
  select_target "$target"
  pack "$output"
  before=$failures
  check_bundle "$output"
  check_size "$output" "$target"
  if ((failures > before)); then
    echo "ERROR: $output failed $((failures - before)) check(s)" >&2
  fi
done

if [[ $failures -gt 0 ]]; then
  # Removed so that no later step, and no developer, picks up a bundle that
  # failed its own checks, or the rest of a set one of them failed.
  rm -f "${OUTPUTS[@]}"
  echo "ERROR: the bundles failed $failures check(s) in all, and the whole set was removed" >&2
  exit 1
fi

trap - EXIT
echo "Built ${#OUTPUTS[@]} bundles (version $VERSION)"
for output in "${OUTPUTS[@]}"; do
  unzip -Z -l "$output"
done

if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
  {
    echo "### Claude Desktop extension bundles $VERSION"
    echo
    echo "| Bundle | Download | Unpacked | Declared in server.json |"
    echo "| --- | ---: | ---: | --- |"
    printf '%s\n' "${REPORT_ROWS[@]}"
  } >> "$GITHUB_STEP_SUMMARY"
fi
