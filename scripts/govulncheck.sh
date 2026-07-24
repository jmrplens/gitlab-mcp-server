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
#   - GO-2026-5932: golang.org/x/crypto/openpgp is unmaintained and unsafe by
#     design. The advisory covers every version of the module
#     ("introduced: 0" / "Fixed in: N/A"), so no dependency bump can ever clear
#     it. It enters the build through github.com/creativeprojects/go-selfupdate,
#     whose validate.go imports the package unconditionally (no build tag) for
#     its PGPValidator type, which links openpgp into any binary importing the
#     selfupdate package.
#
#     We never execute that code. internal/autoupdate configures
#     selfupdate.ChecksumValidator (checksums.txt), never a PGP validator, and
#     our releases are signed with cosign/sigstore
#     (checksums.txt.sigstore.json), not GPG. Every govulncheck trace is a
#     package init() call; not one reaches an openpgp cryptographic function.
#     Reachability is therefore linkage only. Accepted risk.
#
# Usage: scripts/govulncheck.sh [-tags <tags>] [packages...]
set -uo pipefail

# Space-separated OSV IDs accepted with documented justification above.
ALLOWLIST="GO-2026-5932"

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
