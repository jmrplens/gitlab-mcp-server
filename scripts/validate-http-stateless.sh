#!/usr/bin/env bash
# validate-http-stateless.sh — smoke-validates stateless streamable HTTP mode.
#
# Usage:
#   scripts/validate-http-stateless.sh [binary|docker]
#
# Requires GITLAB_URL and GITLAB_TOKEN in the environment (falls back to .env).
# In docker mode GITLAB_URL must be reachable from inside the container
# (e.g. http://host.docker.internal:8929 for the Docker E2E GitLab).
set -euo pipefail

MODE="${1:-binary}"
PORT="${VALIDATE_PORT:-18080}"
BASE="http://127.0.0.1:${PORT}"

if [[ -z "${GITLAB_URL:-}" || -z "${GITLAB_TOKEN:-}" ]] && [[ -f .env ]]; then
  set -a
  # shellcheck disable=SC1091
  source .env
  set +a
fi
: "${GITLAB_URL:?GITLAB_URL is required}"
: "${GITLAB_TOKEN:?GITLAB_TOKEN is required}"

# shellcheck disable=SC2329 # invoked indirectly via the EXIT trap
cleanup() { :; }
trap 'cleanup' EXIT

case "$MODE" in
  binary)
    go build -o /tmp/gitlab-mcp-server-validate ./cmd/server
    /tmp/gitlab-mcp-server-validate --http --http-addr="127.0.0.1:${PORT}" \
      --gitlab-url="${GITLAB_URL}" --stateless --json-response &
    SERVER_PID=$!
    cleanup() { kill "${SERVER_PID}" 2>/dev/null || true; }
    ;;
  docker)
    docker build -t gitlab-mcp-server:stateless-validate .
    CONTAINER_ID=$(docker run -d --rm -p "${PORT}:8080" \
      gitlab-mcp-server:stateless-validate \
      --http --http-addr=0.0.0.0:8080 --gitlab-url="${GITLAB_URL}" \
      --stateless --json-response)
    cleanup() { docker stop "${CONTAINER_ID}" >/dev/null 2>&1 || true; }
    ;;
  *)
    echo "unknown mode: ${MODE} (expected binary|docker)" >&2
    exit 2
    ;;
esac

echo "waiting for ${BASE}/health ..."
for _ in $(seq 1 60); do
  if curl -fsS "${BASE}/health" >/dev/null 2>&1; then break; fi
  sleep 1
done
curl -fsS "${BASE}/health" >/dev/null || { echo "FAIL: server never became healthy" >&2; exit 1; }

echo "1) tools/list without session must return 200, JSON, no Mcp-Session-Id"
HEADERS=$(mktemp)
BODY=$(curl -fsS -D "${HEADERS}" -X POST "${BASE}/" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "PRIVATE-TOKEN: ${GITLAB_TOKEN}" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}')
grep -qi "^content-type: application/json" "${HEADERS}" || { echo "FAIL: response is not application/json" >&2; exit 1; }
if grep -qi "^mcp-session-id:" "${HEADERS}"; then
  echo "FAIL: Mcp-Session-Id present in stateless mode" >&2
  exit 1
fi
echo "${BODY}" | grep -q "gitlab_find_action" || { echo "FAIL: tools/list missing gitlab_find_action" >&2; exit 1; }

echo "2) tools/call gitlab_find_action must return matches"
BODY=$(curl -fsS -X POST "${BASE}/" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "PRIVATE-TOKEN: ${GITLAB_TOKEN}" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"gitlab_find_action","arguments":{"query":"list projects"}}}')
echo "${BODY}" | grep -Eq "gitlab_execute_action|project" || { echo "FAIL: find_action returned no matches" >&2; exit 1; }

echo "3) GET on MCP endpoint must return 405"
STATUS=$(curl -s -o /dev/null -w "%{http_code}" -H "PRIVATE-TOKEN: ${GITLAB_TOKEN}" "${BASE}/")
[[ "${STATUS}" == "405" ]] || { echo "FAIL: GET returned ${STATUS}, want 405" >&2; exit 1; }

rm -f "${HEADERS}"
echo "PASS: stateless HTTP validation (${MODE} mode) succeeded"
