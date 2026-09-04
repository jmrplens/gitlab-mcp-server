#!/usr/bin/env bash
# Decide a fan-in job's verdict from the results of the jobs it needs.
#
# Reads RESULTS, the JSON of the `needs` context, and prints one line per job.
# The verdict is failure if any job did not succeed, with two exceptions the
# caller names by job id:
#
#   INFORMATIONAL  jobs whose result is printed but never decides. A job goes
#                  here when it reports something outside this repository's
#                  control, such as somebody else's advisory database, or a
#                  platform whose runtime crashes on its own now and then.
#   MAY_SKIP       jobs for which "skipped" is a pass, because they skip
#                  themselves on purpose for some events. For every other job
#                  a skip means an upstream failure took it down, and that is
#                  a failure: it is how a parent that no fan-in lists directly
#                  still fails the run through its children.
#
# Both are space-separated lists and both may be empty.
set -euo pipefail

: "${RESULTS:?RESULTS must hold the needs context as JSON}"
informational=" ${INFORMATIONAL:-} "
may_skip=" ${MAY_SKIP:-} "
failed=0

for job in $(printf '%s' "$RESULTS" | jq -r 'keys[]'); do
  result=$(printf '%s' "$RESULTS" | jq -r --arg j "$job" '.[$j].result')
  case "$informational" in
    *" $job "*) printf '%-26s %-10s (informational)\n' "$job" "$result"; continue ;;
  esac
  printf '%-26s %s\n' "$job" "$result"
  case "$result" in
    success) ;;
    skipped)
      case "$may_skip" in
        *" $job "*) ;;
        *) failed=1 ;;
      esac
      ;;
    *) failed=1 ;;
  esac
done

if [ "$failed" -ne 0 ]; then
  echo "::error::a required job did not succeed; see the table above"
  exit 1
fi
