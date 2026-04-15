#!/usr/bin/env bash
set -euo pipefail

# Provision a test user and Personal Access Token on the ephemeral GitLab instance.
# Writes credentials to .env.docker in the project root.
# Usage: ./test/e2e/scripts/setup-gitlab.sh [GITLAB_URL]

GITLAB_URL="${1:-http://localhost:8929}"
ROOT_PASSWORD="E2e_R0ot!xK9mZ#2026"
TEST_USER="e2e-tester"
TEST_EMAIL="e2e-tester@example.com"
TEST_PASSWORD="E2e_T3st!vQ7nW#2026"
ENV_FILE=".env.docker"

echo "=== Setting up GitLab E2E test environment ==="
echo "GitLab URL: ${GITLAB_URL}"

# 1. Get root OAuth token
echo "  [1/4] Authenticating as root..."
ROOT_TOKEN=$(curl -sf "${GITLAB_URL}/oauth/token" \
    --data-urlencode "grant_type=password" \
    --data-urlencode "username=root" \
    --data-urlencode "password=${ROOT_PASSWORD}" | python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])")

if [ -z "$ROOT_TOKEN" ]; then
    echo "ERROR: Failed to authenticate as root"
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
echo "    Default branch protection disabled, deletion_adjourned_period=1, rate limiting disabled"

# 2. Create test user
echo "  [2/4] Creating test user '${TEST_USER}'..."
USER_RESPONSE=$(curl -sf "${GITLAB_URL}/api/v4/users" \
    -H "Authorization: Bearer ${ROOT_TOKEN}" \
    -d "email=${TEST_EMAIL}" \
    -d "username=${TEST_USER}" \
    -d "name=E2E Test User" \
    -d "password=${TEST_PASSWORD}" \
    -d "skip_confirmation=true" \
    -d "admin=true" 2>/dev/null || true)

USER_ID=$(echo "$USER_RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin).get('id',''))" 2>/dev/null || true)

if [ -z "$USER_ID" ]; then
    # User may already exist from a previous run
    USER_ID=$(curl -sf "${GITLAB_URL}/api/v4/users?username=${TEST_USER}" \
        -H "Authorization: Bearer ${ROOT_TOKEN}" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d[0]['id'] if d else '')")
    if [ -z "$USER_ID" ]; then
        echo "ERROR: Failed to create or find test user"
        exit 1
    fi
    echo "    User already exists (ID: ${USER_ID})"
else
    echo "    User created (ID: ${USER_ID})"
fi

# 3. Create Personal Access Token for test user
echo "  [3/4] Creating Personal Access Token..."
TOKEN_RESPONSE=$(curl -sf "${GITLAB_URL}/api/v4/users/${USER_ID}/personal_access_tokens" \
    -H "Authorization: Bearer ${ROOT_TOKEN}" \
    -d "name=e2e-token" \
    -d "scopes[]=api" \
    -d "scopes[]=read_user" \
    -d "scopes[]=read_repository" \
    -d "scopes[]=write_repository")

PAT=$(echo "$TOKEN_RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin)['token'])")

if [ -z "$PAT" ]; then
    echo "ERROR: Failed to create Personal Access Token"
    exit 1
fi
echo "    PAT created successfully"

# 4. Write .env.docker
echo "  [4/4] Writing ${ENV_FILE}..."
cat > "${ENV_FILE}" <<EOF
GITLAB_URL=${GITLAB_URL}
GITLAB_TOKEN=${PAT}
GITLAB_USER=${TEST_USER}
GITLAB_SKIP_TLS_VERIFY=true
E2E_MODE=docker
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
