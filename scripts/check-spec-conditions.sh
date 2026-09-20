#!/usr/bin/env bash
# Condition coverage over the action_specs.go of the packages a branch
# touches, which is the one shape mutation testing keeps finding and no gate
# reads: a per-action metadata table plus a dispatch on the tool name, where
# every entry of the table fills every field, so each `if meta.x != ""` arm is
# always taken and its fallback can never publish anything. The condition is a
# decoration, and the mapping from action to metadata is stated nowhere a
# reader can check.
#
# gobco is what sees it: it instruments every boolean condition and reports
# the ones no test ever evaluated both ways. It exits 0 whatever it finds, so
# a gate has to read its output, which is what this does.
#
# Three deliberate bounds, each because the alternative is worse:
#
#   Changed packages only. gobco instruments into a temporary tree and
#   caches nothing, so it costs 60 to 80 seconds per package: the 169
#   packages that carry an action_specs.go would be about three hours.
#
#   action_specs.go only. A condition elsewhere in a package is the mutation
#   sweep's subject and is reported by `make coverage-conditions`; the claim
#   here is narrow on purpose, so that it can gate.
#
#   A package gobco cannot instrument is reported and does not fail. It
#   copies every .go file ignoring //go:build, so a package with
#   build-constrained files dies with a redeclaration panic, and it resolves
#   the external test package against the non-test files alone, so a package
#   whose export_test.go hands a symbol to its _test package dies with
#   "undefined". Twenty-five of the packages carrying an action_specs.go are
#   in the second state today. Failing on them would fail for the tool's
#   limits rather than for anything about this repository.
#
# A condition that genuinely cannot take the other value is declared on its
# own line, and the declaration is held to the discipline every declaration
# table here is held to: one that answers nothing fails.
#
#   if meta.usage != "" { // gobco: always true because every entry sets it
#
# Usage:
#
#   scripts/check-spec-conditions.sh [base-ref]     # default origin/main

set -euo pipefail

readonly GOBCO="github.com/rillig/gobco@v1.3.4"
readonly DECLARATION='// gobco:'

base="${1:-origin/main}"
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

if ! git rev-parse --verify --quiet "$base" >/dev/null; then
  echo "check-spec-conditions: no such ref: $base" >&2
  exit 2
fi

mapfile -t changed < <(git diff --name-only "$base"...HEAD -- 'internal/tools/*/action_specs.go' | sort -u)
if [ "${#changed[@]}" -eq 0 ]; then
  echo "check-spec-conditions: no action_specs.go changed against $base"
  exit 0
fi

failures=0

# report prints one failing line and counts it.
report() {
  echo "  $1" >&2
  failures=$((failures + 1))
}

# declared reports whether the source line gobco named, or the line above it,
# carries a declaration.
declared() {
  local file="$1" line="$2" above=""
  if [ "$line" -gt 1 ]; then
    above="$(sed -n "$((line - 1))p" "$file")"
  fi
  case "$(sed -n "${line}p" "$file")$above" in
  *"$DECLARATION"*) return 0 ;;
  *) return 1 ;;
  esac
}

for file in "${changed[@]}"; do
  if [ ! -f "$file" ]; then
    echo "check-spec-conditions: $file was removed, nothing to measure"
    continue
  fi
  pkg="$(dirname "$file")"
  echo "check-spec-conditions: measuring $pkg"

  if ! output="$(cd "$pkg" && go run "$GOBCO" 2>&1)"; then
    echo "  not measured: gobco could not instrument this package: $(printf '%s' "$output" | head -n 1)"
    continue
  fi

  printf '%s\n' "$output" | grep -E '^Condition coverage: ' || true

  # gobco names a position it could not decide in one of three shapes:
  #   action_specs.go:91:5: condition "meta.usage != \"\"" was 3 times true but never false
  #   action_specs.go:91:5: condition "meta.usage != \"\"" was once false but never true
  #   action_specs.go:91:5: condition "meta.usage != \"\"" was never evaluated
  # The third is a gap as much as the other two: a condition no test reaches
  # is one no test has decided either way.
  reported=()
  mapfile -t reported < <(printf '%s\n' "$output" | grep -E '^action_specs\.go:[0-9]+:[0-9]+: condition .*(but never (true|false)|was never evaluated)$' || true)

  answered=0
  for line in "${reported[@]}"; do
    number="$(printf '%s' "$line" | cut -d: -f2)"
    if declared "$file" "$number"; then
      answered=$((answered + 1))
      continue
    fi
    report "$pkg/$line"
  done

  # A declaration that answers nothing has stopped describing the package.
  declarations="$(grep -c -- "$DECLARATION" "$file" || true)"
  if [ "$declarations" -gt "$answered" ]; then
    report "$pkg: $declarations declaration(s) and $answered answered condition(s); the rest are stale"
  fi
done

if [ "$failures" -gt 0 ]; then
  echo "check-spec-conditions: $failures finding(s)" >&2
  exit 1
fi
echo "check-spec-conditions: every measured condition is evaluated both ways"
