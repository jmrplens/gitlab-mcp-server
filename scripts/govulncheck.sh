#!/usr/bin/env bash
# Run govulncheck, tolerating a small, documented allowlist of accepted
# advisories that have no available fix.
#
# govulncheck has no native ignore mechanism. This wrapper runs it and, when it
# reports vulnerabilities, passes only if EVERY reported advisory ID is on the
# ALLOWLIST below. Any advisory not on the list fails the build, so newly
# introduced (fixable) vulnerabilities are never silently ignored.
#
# Accepted advisories (keep in sync with docs/development/static-analysis.md):
#   None. The list was emptied when the self-update subsystem was removed: it
#   held only GO-2026-5932, for golang.org/x/crypto/openpgp, which entered the
#   build through github.com/creativeprojects/go-selfupdate. That advisory
#   covers every version of the module ("introduced: 0", "Fixed in: N/A"), so
#   no dependency bump could ever have cleared it; dropping the dependency did.
#
#   Keep it empty. An entry here is a vulnerability shipped on purpose, and the
#   last one was only defensible because the code was never executed. If a new
#   one is ever added, it needs the same standard of argument: why no bump can
#   fix it, and why the reachable path is inert.
#
# Usage: scripts/govulncheck.sh [-tags <tags>] [packages...]
set -uo pipefail

# Space-separated OSV IDs accepted with documented justification above.
ALLOWLIST=""

echo "=== govulncheck ==="
out="$(govulncheck "$@" 2>&1)"
status=$?
printf '%s\n' "$out"

# Advisory IDs govulncheck reported (deduplicated).
ids="$(printf '%s\n' "$out" | grep -oE 'GO-[0-9]{4}-[0-9]+' | sort -u)"

if [ -z "$ids" ]; then
	# No advisories parsed: propagate govulncheck's own exit status verbatim so
	# tool/build errors still fail.
	exit "$status"
fi

unaccepted=""
for id in $ids; do
	case " $ALLOWLIST " in
	*" $id "*) ;;
	*) unaccepted="$unaccepted $id" ;;
	esac
done

echo ""
if [ -n "$unaccepted" ]; then
	echo "govulncheck: FAIL — unaccepted advisories:$unaccepted"
	exit 1
fi

ids_oneline="$(printf '%s\n' "$ids" | paste -sd' ' -)"
echo "govulncheck: PASS — only accepted advisories present: ${ids_oneline} (see docs/development/static-analysis.md)"
exit 0
