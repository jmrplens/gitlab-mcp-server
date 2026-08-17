#!/usr/bin/env bash
# setup-bitbucket.sh — provision the ephemeral Bitbucket Data Center fixture
# for the import_bitbucket_server e2e coverage.
#
# Waits for the e2e-bitbucket compose service (unattended setup driven by
# docker-compose.yml) to reach RUNNING, then provisions over REST:
#   1. Project E2E
#   2. Repository "fixture" with an initial commit (browse edit API — no git
#      client involvement needed)
#   3. An admin HTTP access token for GitLab's importer
# and appends the BITBUCKET_SERVER_* variables to test/e2e/.env.docker.
# The URL written is the compose-network address (http://e2e-bitbucket:7990)
# because GitLab — not the test process — dereferences it.
#
# Best-effort by design: any failure prints a WARNING and exits 0 so the
# suite falls back to the documented skip instead of failing the whole run.
set -uo pipefail

BITBUCKET_URL="${1:-http://localhost:7990}"
INTERNAL_URL="${E2E_BITBUCKET_INTERNAL_URL:-http://e2e-bitbucket:7990}"
ADMIN_USER="admin"
ADMIN_PASSWORD="${E2E_BITBUCKET_ADMIN_PASSWORD:-}"
PROJECT_KEY="E2E"
REPO_SLUG="fixture"
WAIT_SECONDS="${2:-420}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"
ENV_FILE="${REPO_ROOT}/test/e2e/.env.docker"

warn_skip() {
    echo "      WARNING: $1; import_bitbucket_server e2e coverage will skip"
    exit 0
}

[ -n "${ADMIN_PASSWORD}" ] || warn_skip "E2E_BITBUCKET_ADMIN_PASSWORD is not set (the Makefile generates an ephemeral one per run)"

echo "  Waiting for Bitbucket at ${BITBUCKET_URL} (up to ${WAIT_SECONDS}s)..."
deadline=$((SECONDS + WAIT_SECONDS))
while true; do
    state=$(curl -sf --connect-timeout 5 --max-time 10 "${BITBUCKET_URL}/status" 2>/dev/null || true)
    case "${state}" in
        *RUNNING*) break ;;
    esac
    if [ "${SECONDS}" -ge "${deadline}" ]; then
        warn_skip "Bitbucket did not reach RUNNING within ${WAIT_SECONDS}s (last state: ${state:-unreachable})"
    fi
    sleep 10
done
echo "  Bitbucket is RUNNING"

bb_api() {
    curl -sS -u "${ADMIN_USER}:${ADMIN_PASSWORD}" --connect-timeout 5 --max-time 60 "$@"
}

# Project and repository creation are idempotent for this script's purpose:
# a 409 from a previous provisioning attempt is as good as a 201.
echo "  Creating project ${PROJECT_KEY} and repository ${REPO_SLUG}..."
status=$(bb_api -o /dev/null -w '%{http_code}' -X POST "${BITBUCKET_URL}/rest/api/1.0/projects" \
    -H 'Content-Type: application/json' \
    -d "{\"key\":\"${PROJECT_KEY}\",\"name\":\"E2E Import Fixtures\"}")
case "${status}" in
    201|409) ;;
    *) warn_skip "project creation failed (HTTP ${status})" ;;
esac

status=$(bb_api -o /dev/null -w '%{http_code}' -X POST "${BITBUCKET_URL}/rest/api/1.0/projects/${PROJECT_KEY}/repos" \
    -H 'Content-Type: application/json' \
    -d "{\"name\":\"${REPO_SLUG}\",\"scmId\":\"git\",\"defaultBranch\":\"main\"}")
case "${status}" in
    201|409) ;;
    *) warn_skip "repository creation failed (HTTP ${status})" ;;
esac

# Seed the initial commit through the browse edit API so the imported project
# has real content. A 409 means the file already exists from a previous run.
status=$(bb_api -o /dev/null -w '%{http_code}' -X PUT \
    "${BITBUCKET_URL}/rest/api/1.0/projects/${PROJECT_KEY}/repos/${REPO_SLUG}/browse/README.md" \
    -F content="# e2e import fixture" -F message="Initial commit" -F branch=main)
case "${status}" in
    200|201|409) ;;
    *) warn_skip "initial commit failed (HTTP ${status})" ;;
esac

echo "  Minting HTTP access token for the importer..."
token_json=$(bb_api -X PUT "${BITBUCKET_URL}/rest/access-tokens/1.0/users/${ADMIN_USER}" \
    -H 'Content-Type: application/json' \
    -d "{\"name\":\"e2e-import-$(date +%s)\",\"permissions\":[\"REPO_ADMIN\",\"PROJECT_ADMIN\"]}" 2>/dev/null)
token=$(printf '%s' "${token_json}" | python3 -c 'import json,sys
try:
    print(json.load(sys.stdin).get("token", ""))
except Exception:
    pass')
[ -n "${token}" ] || warn_skip "access token creation failed"

{
    echo "BITBUCKET_SERVER_URL=${INTERNAL_URL}"
    echo "BITBUCKET_SERVER_USERNAME=${ADMIN_USER}"
    echo "BITBUCKET_SERVER_TOKEN=${token}"
    echo "BITBUCKET_SERVER_PROJECT_KEY=${PROJECT_KEY}"
    echo "BITBUCKET_SERVER_REPO_SLUG=${REPO_SLUG}"
} >> "${ENV_FILE}"
echo "  Bitbucket fixture ready: ${INTERNAL_URL} ${PROJECT_KEY}/${REPO_SLUG} (vars appended to .env.docker)"
