#!/usr/bin/env bash
set -euo pipefail

# Register a GitLab Runner against the ephemeral GitLab instance.
# Requires: GitLab already healthy, .env.docker exists with root-level access.
# Usage: ./test/e2e/scripts/register-runner.sh [GITLAB_URL]

GITLAB_URL="${1:-http://localhost:8929}"
# Runner needs GitLab's internal Docker hostname (not localhost)
GITLAB_INTERNAL_URL="${2:-http://gitlab-e2e:80}"
ROOT_PASSWORD="E2e_R0ot!xK9mZ#2026"

echo "=== Registering GitLab Runner ==="

# 1. Get root OAuth token
echo "  [1/3] Authenticating as root..."
ROOT_TOKEN=$(curl -sf "${GITLAB_URL}/oauth/token" \
    --data-urlencode "grant_type=password" \
    --data-urlencode "username=root" \
    --data-urlencode "password=${ROOT_PASSWORD}" | python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])")

if [ -z "$ROOT_TOKEN" ]; then
    echo "ERROR: Failed to authenticate as root"
    exit 1
fi

# 2. Create a runner via the API (GitLab 16+ method)
echo "  [2/3] Creating runner via API..."
RUNNER_RESPONSE=$(curl -sf "${GITLAB_URL}/api/v4/user/runners" \
    -H "Authorization: Bearer ${ROOT_TOKEN}" \
    -d "runner_type=instance_type" \
    -d "description=e2e-docker-runner" \
    -d "tag_list=e2e,docker" \
    -d "run_untagged=true" 2>/dev/null || true)

RUNNER_TOKEN=$(echo "$RUNNER_RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin).get('token',''))" 2>/dev/null || true)

if [ -z "$RUNNER_TOKEN" ]; then
    echo "  WARN: New runner API failed, trying legacy registration token..."
    # Fallback: get registration token from admin settings
    REG_TOKEN=$(curl -sf "${GITLAB_URL}/api/v4/runners/reset_registration_token" \
        -H "Authorization: Bearer ${ROOT_TOKEN}" \
        -X POST | python3 -c "import sys,json; print(json.load(sys.stdin).get('token',''))" 2>/dev/null || true)

    if [ -z "$REG_TOKEN" ]; then
        echo "  WARN: Could not get registration token. Runner registration skipped."
        echo "  Pipeline/job tests will be skipped."
        exit 0
    fi
    RUNNER_TOKEN="$REG_TOKEN"
fi

# 3. Register in the runner container
echo "  [3/3] Configuring runner container..."
RUNNER_CONTAINER=$(docker compose ps -q gitlab-runner 2>/dev/null || true)

if [ -z "$RUNNER_CONTAINER" ]; then
    echo "  WARN: gitlab-runner container not found. Skipping runner registration."
    exit 0
fi

# Detect the Docker network created by compose (varies by directory name)
COMPOSE_NETWORK=$(docker compose ps --format json 2>/dev/null | python3 -c "
import sys, json
for line in sys.stdin:
    obj = json.loads(line)
    for net in obj.get('Networks', '').split(','):
        net = net.strip()
        if net:
            print(net)
            sys.exit(0)
" 2>/dev/null || echo "e2e_default")

docker exec "$RUNNER_CONTAINER" gitlab-runner register \
    --non-interactive \
    --url "${GITLAB_INTERNAL_URL}" \
    --token "${RUNNER_TOKEN}" \
    --executor docker \
    --docker-image "alpine:latest" \
    --docker-network-mode "${COMPOSE_NETWORK}" \
    --description "e2e-docker-runner" 2>/dev/null || \
docker exec "$RUNNER_CONTAINER" gitlab-runner register \
    --non-interactive \
    --url "${GITLAB_INTERNAL_URL}" \
    --registration-token "${RUNNER_TOKEN}" \
    --executor docker \
    --docker-image "alpine:latest" \
    --docker-network-mode "${COMPOSE_NETWORK}" \
    --description "e2e-docker-runner" 2>/dev/null || true

echo ""
echo "=== Runner registration complete ==="
echo "  Verify: curl -s ${GITLAB_URL}/api/v4/runners/all -H 'Authorization: Bearer ...'"
