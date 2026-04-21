#!/usr/bin/env bash
set -euo pipefail

# Wait for GitLab to become ready
# Usage: ./test/e2e/scripts/wait-for-gitlab.sh [URL] [TIMEOUT_SECONDS]

GITLAB_URL="${1:-http://localhost:8929}"
TIMEOUT="${2:-600}"
INTERVAL=10

echo "Waiting for GitLab at ${GITLAB_URL} (timeout: ${TIMEOUT}s)..."

elapsed=0
while [ "$elapsed" -lt "$TIMEOUT" ]; do
    if curl -sf "${GITLAB_URL}/-/readiness?all=1" > /dev/null 2>&1; then
        echo "GitLab readiness probe OK after ${elapsed}s"
        break
    fi
    echo "  ...not ready yet (${elapsed}s elapsed)"
    sleep "$INTERVAL"
    elapsed=$((elapsed + INTERVAL))
done

if [ "$elapsed" -ge "$TIMEOUT" ]; then
    echo "ERROR: GitLab did not become ready within ${TIMEOUT}s"
    exit 1
fi

# Verify the REST API is actually accepting connections (the readiness probe
# can return OK before nginx/puma workers are fully warmed up).
API_TIMEOUT=60
api_elapsed=0
echo "Verifying API endpoint responds (timeout: ${API_TIMEOUT}s)..."
while [ "$api_elapsed" -lt "$API_TIMEOUT" ]; do
    # Accept any HTTP response (even 401) — it means the API is accepting connections.
    http_code=$(curl -s -o /dev/null -w "%{http_code}" "${GITLAB_URL}/api/v4/version" 2>/dev/null || true)
    if [ -n "$http_code" ] && [ "$http_code" -gt 0 ] 2>/dev/null; then
        echo "API verified after ${api_elapsed}s (HTTP ${http_code}) — GitLab is ready"
        exit 0
    fi
    sleep 2
    api_elapsed=$((api_elapsed + 2))
done

echo "ERROR: GitLab API did not respond within ${API_TIMEOUT}s after readiness"
exit 1
