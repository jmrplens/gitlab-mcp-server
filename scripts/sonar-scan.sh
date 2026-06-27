#!/usr/bin/env bash
#
# sonar-scan.sh — run the SonarCloud analysis pipeline locally, mirroring CI.
#
# Steps:
#   1. Run unit tests with coverage exactly as the CI "test" job does
#      (go test -coverpkg=./cmd/...,./internal/... -coverprofile=coverage.out).
#   2. Upload the analysis with sonar-scanner (config from sonar-project.properties).
#   3. Poll the SonarCloud Compute Engine task until the analysis is processed.
#   4. Print the quality gate status, the per-condition results, and key measures.
#
# Reads SONARQUBE_TOKEN (required) and optional SONARQUBE_ORG from .env.
# The branch defaults to the current git branch; override with SONAR_BRANCH.
#
# Flags / env:
#   --no-scan            skip steps 1-3 and only fetch the latest gate result
#   SONAR_BRANCH=<name>  analyze/report a specific branch (default: current)
#   SONAR_HOST_URL=<url> SonarQube host (default: https://sonarcloud.io)
#   SONAR_POLL_TIMEOUT   seconds to wait for the CE task (default: 240)
#   SONAR_POLL_INTERVAL  seconds between CE task polls (default: 5)
#   COVERAGE_MIN         minimum total coverage %, mirrors CI (default: 90)
#
# Exits non-zero if the quality gate is not OK (or on any pipeline error).

set -euo pipefail

HOST="${SONAR_HOST_URL:-https://sonarcloud.io}"
POLL_TIMEOUT="${SONAR_POLL_TIMEOUT:-240}"
POLL_INTERVAL="${SONAR_POLL_INTERVAL:-5}"
COVERAGE_MIN="${COVERAGE_MIN:-90}"
NO_SCAN=0
[ "${1:-}" = "--no-scan" ] && NO_SCAN=1

cd "$(git rev-parse --show-toplevel)"

if [ ! -f .env ]; then
	echo "ERROR: .env not found in repo root (needs SONARQUBE_TOKEN)" >&2
	exit 1
fi
set -a
# shellcheck disable=SC1091
. ./.env
set +a

TOKEN="${SONARQUBE_TOKEN:-}"
if [ -z "$TOKEN" ]; then
	echo "ERROR: SONARQUBE_TOKEN is not set in .env" >&2
	exit 1
fi

PROJECT_KEY="$(grep -E '^sonar\.projectKey=' sonar-project.properties | cut -d= -f2-)"
BRANCH="${SONAR_BRANCH:-$(git rev-parse --abbrev-ref HEAD)}"

# api <path> <query...> — authenticated GET against the SonarCloud web API.
api() {
	local path="$1"
	shift
	local args=()
	local kv
	for kv in "$@"; do
		args+=(--data-urlencode "$kv")
	done
	curl -fsS -u "$TOKEN:" -G "${args[@]}" "$HOST$path"
}

if [ "$NO_SCAN" -eq 0 ]; then
	echo "==> [1/4] Running tests with coverage (branch: $BRANCH)"
	go test -count=1 -coverpkg=./cmd/...,./internal/... -coverprofile=coverage.out ./cmd/... ./internal/...
	total="$(go tool cover -func=coverage.out | awk '/^total:/ {print $3}' | tr -d '%')"
	echo "    total coverage: ${total}% (minimum ${COVERAGE_MIN}%)"
	if awk "BEGIN {exit !(${total} + 0 < ${COVERAGE_MIN} + 0)}"; then
		echo "ERROR: coverage ${total}% is below minimum ${COVERAGE_MIN}%" >&2
		exit 1
	fi

	echo "==> [2/4] Uploading analysis to SonarCloud"
	sonar-scanner \
		-Dsonar.token="$TOKEN" \
		-Dsonar.host.url="$HOST" \
		-Dsonar.branch.name="$BRANCH"

	echo "==> [3/4] Waiting for the Compute Engine task to finish"
	CE_URL="$(grep -E '^ceTaskUrl=' .scannerwork/report-task.txt | cut -d= -f2-)"
	deadline=$(( $(date +%s) + POLL_TIMEOUT ))
	while :; do
		status="$(curl -fsS -u "$TOKEN:" "$CE_URL" | jq -r '.task.status')"
		case "$status" in
		SUCCESS) echo "    analysis processed"; break ;;
		FAILED | CANCELED) echo "ERROR: Compute Engine task $status" >&2; exit 1 ;;
		*) printf '    status: %s\r' "$status" ;;
		esac
		if [ "$(date +%s)" -ge "$deadline" ]; then
			echo "ERROR: timed out after ${POLL_TIMEOUT}s waiting for CE task" >&2
			exit 1
		fi
		sleep "$POLL_INTERVAL"
	done
fi

echo "==> [4/4] Fetching quality gate result"
GATE="$(api /api/qualitygates/project_status "projectKey=$PROJECT_KEY" "branch=$BRANCH")"
MEASURES="$(api /api/measures/component "component=$PROJECT_KEY" "branch=$BRANCH" \
	"metricKeys=new_coverage,new_duplicated_lines_density,new_code_smells,coverage,duplicated_lines_density")"

GATE_STATUS="$(echo "$GATE" | jq -r '.projectStatus.status')"

echo ""
echo "=============== SonarCloud — $PROJECT_KEY @ $BRANCH ==============="
echo "Quality gate: $GATE_STATUS"
echo "-- new-code conditions --"
echo "$GATE" | jq -r '
	.projectStatus.conditions[]
	| "  [\(.status)] \(.metricKey): \(.actualValue) (error threshold: \(.errorThreshold))"'
echo "-- measures --"
echo "$MEASURES" | jq -r '
	.component.measures[]
	| "  \(.metric): \(.value // (.period.value) // (.periods[0].value))"'
echo "Dashboard: $HOST/summary/new_code?id=$PROJECT_KEY&branch=$BRANCH"
echo "=================================================================="

[ "$GATE_STATUS" = "OK" ]
