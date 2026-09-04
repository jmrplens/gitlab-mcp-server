#!/usr/bin/env bash
# go-test-crash-aware.sh — run a test suite through gotestsum and tell a Go
# runtime crash apart from a failing test.
#
# Usage:
#   go-test-crash-aware.sh <json-dir> [-flag=value ...] <packages ...>
#
# Flags are passed to `go test` as written and must carry their value in the
# same word (-timeout=30m, not -timeout 30m), because the packages are told
# from the flags by their first character and run again on their own.
#
# The first run is `gotestsum --jsonfile <json-dir>/first.json -- <args>`.
# Its go test -json stream is then read back. A package whose output carries
# one of the signatures below is a crashed package; a package with a fail
# event and no signature is a failing package, whether a test failed, the
# build broke or TestMain gave up. Failing packages fail the run, as they
# always did, and are listed with their failing tests. Crashed packages are
# run once more, alone: one that passes is reported as a runtime crash in
# the log and in the job summary and does not fail the run, one that crashes
# or fails again does.
#
# Why. The Windows leg of the cross-platform matrix dies inside the Go
# runtime now and then (issue 467). golang/go#81238 explains it: on hosts with
# Intel AMX, recovering a hardware exception on a goroutine stack lets Windows
# write its exception context below the stack into the heap, and the next
# garbage collection faults. The hosted pool is heterogeneous, so the same
# commit crashes on one runner and passes on the next. Rerunning the whole job
# by hand told nobody anything and cost twenty minutes; failing on the crash
# blocked merges on a bug nobody here can fix; ignoring the leg let a real
# Windows regression pass as one more flake. This keeps the distinction, and
# keeps a record of every crash where a reader looks for one.
#
# The signatures are the runtime's own words for a corrupted heap or stack,
# and only those. "panic: runtime error" is not one: an unrecovered nil
# dereference is a bug in the code under test. Neither is a deadlock nor a
# concurrent map write, which the runtime also reports as a fatal error and
# which are bugs too.
set -uo pipefail

JSON_DIR="${1:?Usage: $0 <json-dir> [-flag=value ...] <packages ...>}"
shift

CRASH_SIGNATURES='fatal error: (fault|found pointer to free object|found bad pointer in Go heap|unexpected signal|bad pointer in frame)|runtime: marked free object in span|traceback did not unwind completely'

flags=()
packages=()
for arg in "$@"; do
  case "$arg" in
    -*) flags+=("$arg") ;;
    *) packages+=("$arg") ;;
  esac
done
if [ "${#packages[@]}" -eq 0 ]; then
  echo "ERROR: no packages given" >&2
  exit 2
fi

mkdir -p "$JSON_DIR"

# crashed_packages lists the packages whose output carries a crash signature.
crashed_packages() {
  jq -r --arg re "$CRASH_SIGNATURES" \
    'select(.Action == "output" and (.Output | test($re))) | .Package' "$1" | sort -u
}

# failed_packages lists the packages with a package-level fail event.
failed_packages() {
  jq -r 'select(.Action == "fail" and .Test == null) | .Package' "$1" | sort -u
}

# failed_tests lists "package<TAB>test" for every test-level fail event.
failed_tests() {
  jq -r 'select(.Action == "fail" and .Test != null) | "\(.Package)\t\(.Test)"' "$1" | sort -u
}

# crash_lines lists "package<TAB>first signature line" per crashed package.
crash_lines() {
  jq -r --arg re "$CRASH_SIGNATURES" \
    'select(.Action == "output" and (.Output | test($re))) | "\(.Package)\t\(.Output | rtrimstr("\n"))"' "$1" \
    | sort -u -k1,1
}

# without prints the lines of $1 whose first field is not listed in $2.
without() {
  awk -F'\t' -v list="$2" '
    BEGIN { n = split(list, a, "\n"); for (i = 1; i <= n; i++) if (a[i] != "") c[a[i]] = 1 }
    $1 != "" && !($1 in c)' <<< "$1"
}

# summary appends to the job summary when there is one, and always to stdout.
summary() {
  printf '%s\n' "$@"
  if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
    printf '%s\n' "$@" >> "$GITHUB_STEP_SUMMARY"
  fi
}

first="$JSON_DIR/first.json"
gotestsum --jsonfile "$first" -- "${flags[@]}" "${packages[@]}"
first_status=$?
if [ ! -s "$first" ]; then
  echo "ERROR: gotestsum wrote no JSON stream to $first (exit $first_status)" >&2
  exit "${first_status:-1}"
fi

crashed=$(crashed_packages "$first")
failing=$(without "$(failed_packages "$first")" "$crashed")

if [ -n "$failing" ]; then
  summary "### Test suite: failing packages" ""
  while IFS= read -r pkg; do
    summary "- \`$pkg\`"
    while IFS=$'\t' read -r p t; do
      [ "$p" = "$pkg" ] && summary "  - $t"
    done <<< "$(failed_tests "$first")"
  done <<< "$failing"
  if [ -n "$crashed" ]; then
    summary "" "Also crashed inside the Go runtime, not rerun because the failures above decide the run:"
    while IFS=$'\t' read -r p line; do summary "- \`$p\`: $line"; done <<< "$(crash_lines "$first")"
  fi
  exit 1
fi

if [ -z "$crashed" ]; then
  exit "$first_status"
fi

echo
echo "The Go runtime crashed in the following package(s); rerunning them alone once (issue 467, golang/go#81238):"
while IFS=$'\t' read -r p line; do echo "  $p: $line"; done <<< "$(crash_lines "$first")"
echo

rerun="$JSON_DIR/rerun.json"
# shellcheck disable=SC2086 # import paths, one per line, never contain spaces
gotestsum --jsonfile "$rerun" -- "${flags[@]}" $crashed
rerun_status=$?

crashed_again=$(crashed_packages "$rerun")
failed_again=$(failed_packages "$rerun")

summary "### Test suite: Go runtime crash" "" \
  "The first run crashed inside the Go runtime (issue 467, golang/go#81238) and the crashed packages were run once more on their own." "" \
  "| Package | First run | Rerun |" "| --- | --- | --- |"
while IFS=$'\t' read -r p line; do
  outcome="passed"
  if grep -qxF "$p" <<< "$crashed_again"; then outcome="crashed again"
  elif grep -qxF "$p" <<< "$failed_again"; then outcome="failed"; fi
  summary "| \`$p\` | \`$line\` | $outcome |"
done <<< "$(crash_lines "$first")"

if [ -n "$crashed_again" ] || [ -n "$failed_again" ] || [ "$rerun_status" -ne 0 ]; then
  summary "" "The rerun did not pass, so the run fails."
  exit 1
fi
echo "::warning title=Go runtime crash::the Go runtime crashed in $(wc -l <<< "$crashed" | tr -d ' ') package(s) and every one passed when rerun alone; see the job summary (issue 467)"
exit 0
