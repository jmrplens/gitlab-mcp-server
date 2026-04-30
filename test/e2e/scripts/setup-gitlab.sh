#!/usr/bin/env bash
set -euo pipefail

# Provision a test user and Personal Access Token on the ephemeral GitLab instance.
# Writes credentials and Docker fixture endpoints to test/e2e/.env.docker.
# Usage: ./test/e2e/scripts/setup-gitlab.sh [GITLAB_URL]

GITLAB_URL="${1:-http://localhost:8929}"
ROOT_PASSWORD="E2e_R0ot!xK9mZ#2026"
TEST_USER="e2e-tester"
TEST_EMAIL="e2e-tester@example.com"
TEST_PASSWORD="E2e_T3st!vQ7nW#2026"
ENV_FILE="test/e2e/.env.docker"

echo "=== Setting up GitLab E2E test environment ==="
echo "GitLab URL: ${GITLAB_URL}"

# Extract object field from JSON safely. Returns empty string on invalid JSON.
json_field() {
    local json_input="$1"
    local field="$2"
    JSON_INPUT="$json_input" FIELD="$field" python3 -c 'import json, os
data = os.environ.get("JSON_INPUT", "")
field = os.environ.get("FIELD", "")
try:
    obj = json.loads(data)
    if isinstance(obj, dict):
        value = obj.get(field, "")
        print("" if value is None else value)
    else:
        print("")
except Exception:
    print("")
' 2>/dev/null || true
}

# Extract field from first element in JSON array safely.
# Returns empty string on invalid JSON, non-array payloads, or empty arrays.
json_first_array_field() {
    local json_input="$1"
    local field="$2"
    JSON_INPUT="$json_input" FIELD="$field" python3 -c 'import json, os
data = os.environ.get("JSON_INPUT", "")
field = os.environ.get("FIELD", "")
try:
    arr = json.loads(data)
    if isinstance(arr, list) and arr and isinstance(arr[0], dict):
        value = arr[0].get(field, "")
        print("" if value is None else value)
    else:
        print("")
except Exception:
    print("")
' 2>/dev/null || true
}

# 1. Get root OAuth token (with retry — GitLab may still be warming up)
echo "  [1/4] Authenticating as root..."
ROOT_TOKEN=""
for attempt in 1 2 3 4 5; do
    OAUTH_RESPONSE=$(curl -sS "${GITLAB_URL}/oauth/token" \
        --data-urlencode "grant_type=password" \
        --data-urlencode "username=root" \
        --data-urlencode "password=${ROOT_PASSWORD}" \
        --retry 3 --retry-delay 2 --retry-all-errors \
        --connect-timeout 5 --max-time 30 2>/dev/null || true)

    if [ -n "$OAUTH_RESPONSE" ]; then
        ROOT_TOKEN=$(json_field "$OAUTH_RESPONSE" "access_token")
    fi

    if [ -n "$ROOT_TOKEN" ]; then
        break
    fi
    echo "    Attempt ${attempt}/5 failed, retrying in 3s..."
    sleep 3
done

if [ -z "$ROOT_TOKEN" ]; then
    echo "ERROR: Failed to authenticate as root after 5 attempts"
    exit 1
fi
echo "    Root OAuth token obtained"

# 1b. Disable default branch protection so E2E tests can push to main,
# reduce deletion_adjourned_period to 1 day (minimum) so permanent deletes work immediately,
# and disable all rate limiting to avoid 429 errors during parallel E2E tests.
curl -sf "${GITLAB_URL}/api/v4/application/settings" \
    -X PUT \
    -H "Authorization: Bearer ${ROOT_TOKEN}" \
    -d "default_branch_protection=0" \
    -d "deletion_adjourned_period=1" \
    -d "allow_local_requests_from_web_hooks_and_services=true" \
    -d "allow_local_requests_from_system_hooks=true" \
    -d "throttle_authenticated_api_enabled=false" \
    -d "throttle_authenticated_web_enabled=false" \
    -d "throttle_unauthenticated_api_enabled=false" \
    -d "throttle_unauthenticated_web_enabled=false" \
    -d "throttle_authenticated_packages_api_enabled=false" \
    -d "throttle_authenticated_git_lfs_enabled=false" \
    -d "throttle_authenticated_files_api_enabled=false" \
    -d "throttle_unauthenticated_files_api_enabled=false" \
    -d "throttle_authenticated_deprecated_api_enabled=false" \
    -d "throttle_unauthenticated_deprecated_api_enabled=false" > /dev/null 2>&1
echo "    Default branch protection disabled, local outbound requests enabled, deletion_adjourned_period=1, rate limiting disabled"

# 2. Create test user
echo "  [2/4] Creating test user '${TEST_USER}'..."
USER_ID=""
USER_CREATED="false"
for attempt in 1 2 3 4 5 6; do
    USER_RESPONSE=$(curl -sS "${GITLAB_URL}/api/v4/users" \
        -H "Authorization: Bearer ${ROOT_TOKEN}" \
        -d "email=${TEST_EMAIL}" \
        -d "username=${TEST_USER}" \
        -d "name=E2E Test User" \
        -d "password=${TEST_PASSWORD}" \
        -d "skip_confirmation=true" \
        -d "admin=true" \
        --retry 3 --retry-delay 2 --retry-all-errors \
        --connect-timeout 5 --max-time 30 2>/dev/null || true)

    USER_ID=$(json_field "$USER_RESPONSE" "id")
    if [ -n "$USER_ID" ]; then
        USER_CREATED="true"
        break
    fi

    # User may already exist from a previous run. Query and parse defensively.
    LOOKUP_RESPONSE=$(curl -sS "${GITLAB_URL}/api/v4/users?username=${TEST_USER}" \
        -H "Authorization: Bearer ${ROOT_TOKEN}" \
        --retry 3 --retry-delay 2 --retry-all-errors \
        --connect-timeout 5 --max-time 30 2>/dev/null || true)
    USER_ID=$(json_first_array_field "$LOOKUP_RESPONSE" "id")
    if [ -n "$USER_ID" ]; then
        break
    fi

    echo "    Attempt ${attempt}/6: user not available yet, retrying in 3s..."
    sleep 3
done

if [ -z "$USER_ID" ]; then
    echo "ERROR: Failed to create or find test user after 6 attempts"
    exit 1
fi

if [ "$USER_CREATED" = "true" ]; then
    echo "    User created (ID: ${USER_ID})"
else
    echo "    User already exists (ID: ${USER_ID})"
fi

# 3. Create Personal Access Token for test user (with retry for timing issues)
echo "  [3/4] Creating Personal Access Token..."
PAT=""
for attempt in 1 2 3; do
    TOKEN_RESPONSE=$(curl -sS "${GITLAB_URL}/api/v4/users/${USER_ID}/personal_access_tokens" \
        -H "Authorization: Bearer ${ROOT_TOKEN}" \
        -d "name=e2e-token" \
        -d "scopes[]=api" \
        -d "scopes[]=read_user" \
        -d "scopes[]=read_repository" \
        -d "scopes[]=write_repository" \
        --retry 3 --retry-delay 2 --retry-all-errors \
        --connect-timeout 5 --max-time 30 2>/dev/null || true)

    PAT=$(json_field "$TOKEN_RESPONSE" "token")

    if [ -n "$PAT" ]; then
        break
    fi
    echo "    Attempt ${attempt}/3 failed, retrying in 3s..."
    sleep 3
done

if [ -z "$PAT" ]; then
    echo "ERROR: Failed to create Personal Access Token after 3 attempts"
    exit 1
fi
echo "    PAT created successfully"

# 4. Write .env.docker
echo "  [4/4] Writing ${ENV_FILE}..."
cat > "${ENV_FILE}" <<EOF
GITLAB_URL=${GITLAB_URL}
GITLAB_TOKEN=${PAT}
GITLAB_SKIP_TLS_VERIFY=true
E2E_MODE=docker
E2E_FIXTURE_URL=http://e2e-fixture:8080
E2E_GITLAB_INTERNAL_URL=http://gitlab-e2e
EOF

echo ""
echo "=== Setup complete ==="
echo "  User: ${TEST_USER} (admin)"
echo "  Token: ${PAT:0:10}..."
echo "  Config: ${ENV_FILE}"
echo ""
echo "To run E2E tests:"
echo "  set -a && source ${ENV_FILE} && set +a"
echo "  go test -v -tags e2e -timeout 600s ./test/e2e/"
