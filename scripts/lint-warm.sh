#!/usr/bin/env bash
# Rebuild the golangci-lint analysis cache with visible progress.
#
# A full `golangci-lint run ./...` on a cold cache takes many minutes on this
# repository (226 packages, 55 active linters) and prints nothing at all until
# it finishes, so there is no way to tell a slow run from a hung one. golangci-
# lint has no progress output of its own — `-v` reports phases, never items —
# so the only way to get a real percentage is to drive the packages in batches
# and count them.
#
# That costs wall-clock time: batching loses cross-package parallelism and pays
# process startup once per batch. Measured on a warm cache, 12 batches took 9s
# against 3.5s for a single run (~2.5x). The penalty is smaller on a cold cache,
# where analysis dominates startup — which is the only case this script is for.
#
# Use it when the cache has been invalidated wholesale (Go or golangci-lint
# upgrade, .golangci.yml change) and you want to rebuild it deliberately rather
# than pay for it inside your next `make golangci-lint`. For everyday linting
# use `make golangci-lint` directly: with a warm cache it takes seconds.
#
# Usage: scripts/lint-warm.sh [batch-count]   (default: 12)

set -euo pipefail

cd "$(dirname "$0")/.."

BATCHES="${1:-12}"
# Build tags come from .golangci.yml (run.build-tags); `go list` needs them
# passed explicitly so the package set matches what golangci-lint will analyze.
TAGS="${GO_ANALYSIS_TAGS:-e2e}"

if ! command -v golangci-lint >/dev/null 2>&1; then
	echo "golangci-lint not found on PATH" >&2
	exit 1
fi

case "$BATCHES" in
'' | *[!0-9]*) echo "batch-count must be a positive integer, got: $BATCHES" >&2; exit 1 ;;
0) echo "batch-count must be greater than zero" >&2; exit 1 ;;
esac

WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT

echo "=== golangci-lint cache warm-up ==="
golangci-lint cache status
echo

# Warming the Go build cache first is cheap and removes compilation from the
# per-batch timings, so the reported progress reflects analysis work.
echo "--- warming Go build cache ---"
go build -tags "$TAGS" ./...
echo

# `-f {{.Dir}}` because golangci-lint resolves its arguments as filesystem paths,
# not import paths. Passing the import path makes it look for a directory of that
# name under the working directory, which fails typechecking with exit 7 while
# still looking like it ran.
go list -tags "$TAGS" -f '{{.Dir}}' ./... > "$WORKDIR/packages"
TOTAL="$(wc -l < "$WORKDIR/packages" | tr -d ' ')"
if [ "$TOTAL" -eq 0 ]; then
	echo "no packages to analyze" >&2
	exit 1
fi
# Ceiling division so the final batch is never empty.
PER_BATCH=$(((TOTAL + BATCHES - 1) / BATCHES))

echo "--- analyzing $TOTAL packages in batches of $PER_BATCH ---"
split -l "$PER_BATCH" "$WORKDIR/packages" "$WORKDIR/chunk."

START="$(date +%s)"
INDEX=0
FAILED=0
for chunk in "$WORKDIR"/chunk.*; do
	INDEX=$((INDEX + 1))
	# Read into an array rather than relying on word splitting, so a package path
	# is passed as one argument no matter what it contains. A `while read` loop
	# keeps this working on the bash 3.2 that stock macOS still ships.
	PKGS=()
	while IFS= read -r pkg; do
		[ -n "$pkg" ] && PKGS+=("$pkg")
	done < "$chunk"
	# A batch that reports findings must not abort the warm-up: the cache entries
	# it produced are still valid, and `make golangci-lint` is what reports issues.
	if ! golangci-lint run --build-tags "$TAGS" "${PKGS[@]}" >/dev/null 2>&1; then
		FAILED=$((FAILED + 1))
	fi
	DONE=$((INDEX * PER_BATCH))
	[ "$DONE" -gt "$TOTAL" ] && DONE="$TOTAL"
	ELAPSED=$(($(date +%s) - START))
	printf '  [%2d/%2d] %3d%%  %3d/%d packages  %02d:%02d elapsed\n' \
		"$INDEX" "$BATCHES" $((DONE * 100 / TOTAL)) "$DONE" "$TOTAL" \
		$((ELAPSED / 60)) $((ELAPSED % 60))
done

echo
echo "--- cache warmed in $(( ($(date +%s) - START) / 60 ))m $(( ($(date +%s) - START) % 60 ))s ---"
golangci-lint cache status
if [ "$FAILED" -gt 0 ]; then
	echo
	echo "note: $FAILED batch(es) reported findings or errors; the cache is still warm."
	echo "      run 'make golangci-lint' to see them."
fi
