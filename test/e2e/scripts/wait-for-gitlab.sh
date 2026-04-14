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
        echo "GitLab is ready after ${elapsed}s"
        exit 0
    fi
    echo "  ...not ready yet (${elapsed}s elapsed)"
    sleep "$INTERVAL"
    elapsed=$((elapsed + INTERVAL))
done

echo "ERROR: GitLab did not become ready within ${TIMEOUT}s"
exit 1
