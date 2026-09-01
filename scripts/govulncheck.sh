#!/usr/bin/env bash
# Run govulncheck, tolerating a small, documented allowlist of accepted
# advisories that have no available fix.
#
# govulncheck has no native ignore mechanism. This wrapper runs it and, when it
# reports vulnerabilities, passes only if EVERY reported advisory ID is on the
# ALLOWLIST below. Any advisory not on the list fails the build, so newly
# introduced (fixable) vulnerabilities are never silently ignored.
#
# What this gates on, precisely: whether OUR CODE CALLS a vulnerable symbol.
# That is what govulncheck's own exit status reports, and this wrapper defers to
# it rather than re-deriving it from the printed text. The distinction is not
# academic. govulncheck also reports advisories against modules that are merely
# in the build graph, and it prints those only at higher -show levels, so a
# wrapper that scraped every advisory ID out of the output would pass or fail
# depending on a flag its caller happened to pass. That is what this one used to
# do, and `scripts/govulncheck.sh -show verbose ./...` failed while the same
# scan without the flag passed.
#
# Accepted advisories (keep in sync with docs/development/static-analysis.md):
#   None. The list was emptied when the self-update subsystem was removed. It
#   held only GO-2026-5932, "the golang.org/x/crypto/openpgp package is
#   unmaintained, unsafe by design, and has known security issues", which
#   reached our code through github.com/creativeprojects/go-selfupdate. Dropping
#   that dependency removed the call path, so govulncheck now reports "Your code
#   is affected by 0 vulnerabilities" where it previously named ours.
#
#   Be precise about what did NOT happen: the advisory is keyed to the module
#   golang.org/x/crypto, not to the openpgp package, and that module is still a
#   direct requirement because cmd/eval_mcp_surfaces imports
#   golang.org/x/crypto/ssh. So `govulncheck -show verbose ./...` still lists
#   GO-2026-5932 under module results, and always will: it covers every version
#   ("introduced: 0", "Fixed in: N/A"). What the removal cleared is the reachable
#   path, which is the thing that mattered and the thing this gate checks.
#
#   Keep the list empty. An entry here is a vulnerability shipped on purpose in
#   code we actually call. If one is ever added it needs the same standard of
#   argument: why no bump can fix it, and why the reachable path is inert.
#
# Usage: scripts/govulncheck.sh [-tags <tags>] [packages...]
set -uo pipefail

# Space-separated OSV IDs accepted with documented justification above.
ALLOWLIST=""

echo "=== govulncheck ==="
out="$(govulncheck "$@" 2>&1)"
status=$?
printf '%s\n' "$out"

if [ "$status" -eq 0 ]; then
	# Nothing our code calls is vulnerable. Advisories against modules that are
	# only in the build graph may still appear above, at -show levels that print
	# them; they are informational and do not gate, because a module we require
	# for one package does not become a risk through a package we never import.
	exit 0
fi

# Past here our code is affected, or govulncheck itself failed. Advisory IDs
# govulncheck reported (deduplicated).
ids="$(printf '%s\n' "$out" | grep -oE 'GO-[0-9]{4}-[0-9]+' | sort -u)"

if [ -z "$ids" ]; then
	# A non-zero exit with no advisory parsed is a tool or build error. Propagate
	# it verbatim rather than reporting a clean scan.
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
