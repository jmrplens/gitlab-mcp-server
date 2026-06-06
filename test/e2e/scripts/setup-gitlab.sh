#!/usr/bin/env bash
set -euo pipefail

# Provision a test user and Personal Access Token on the ephemeral GitLab instance.
# Writes credentials and Docker fixture endpoints to test/e2e/.env.docker.
# Usage: ./test/e2e/scripts/setup-gitlab.sh [GITLAB_URL]

GITLAB_URL="${1:-http://localhost:8929}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"
ROOT_PASSWORD="E2e_R0ot!xK9mZ#2026"
TEST_USER="e2e-tester"
TEST_EMAIL="e2e-tester@example.com"
TEST_PASSWORD="E2e_T3st!vQ7nW#2026"
ENV_FILE="${REPO_ROOT}/test/e2e/.env.docker"
ENV_FILE_DISPLAY="test/e2e/.env.docker"
ROOT_AUTH_ATTEMPTS="${E2E_ROOT_AUTH_ATTEMPTS:-30}"
ROOT_AUTH_INTERVAL="${E2E_ROOT_AUTH_INTERVAL:-10}"
COMPOSE_FILE="${E2E_DOCKER_COMPOSE_FILE:-${REPO_ROOT}/test/e2e/docker-compose.yml}"
ENTERPRISE_LICENSE_FILE="${E2E_ENTERPRISE_LICENSE_FILE:-${REPO_ROOT}/test/e2e/.enterprise-license}"
ENTERPRISE_LICENSE_FILE_DISPLAY="${E2E_ENTERPRISE_LICENSE_FILE:-test/e2e/.enterprise-license}"
if [[ "$ENTERPRISE_LICENSE_FILE" != /* ]]; then
    ENTERPRISE_LICENSE_FILE="${REPO_ROOT}/${ENTERPRISE_LICENSE_FILE}"
fi

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

# Read one key from the repository .env file without shell-evaluating secrets.
dotenv_value() {
    local key="$1"
    local file="${REPO_ROOT}/.env"
    if [ ! -f "$file" ]; then
        return 0
    fi
    KEY="$key" ENV_PATH="$file" python3 -c 'import os
key = os.environ.get("KEY", "")
path = os.environ.get("ENV_PATH", "")
try:
    with open(path, "r", encoding="utf-8") as handle:
        for raw in handle:
            line = raw.strip()
            if not line or line.startswith("#") or "=" not in line:
                continue
            name, value = line.split("=", 1)
            name = name.strip()
            if name.startswith("export "):
                name = name.split(None, 1)[1].strip()
            if name != key:
                continue
            value = value.strip().strip("\"").strip(chr(39))
            print(value)
            break
except OSError:
    pass
' 2>/dev/null || true
}

env_or_dotenv() {
    local key="$1"
    local value="${!key:-}"
    if [ -n "$value" ]; then
        printf '%s' "$value"
        return 0
    fi
    dotenv_value "$key"
}

enterprise_license_file_value() {
    local file="$ENTERPRISE_LICENSE_FILE"
    if [ ! -f "$file" ]; then
        return 0
    fi
    LICENSE_FILE="$file" python3 -c 'import os
path = os.environ.get("LICENSE_FILE", "")
try:
    with open(path, "r", encoding="utf-8") as handle:
        print(handle.read().strip(), end="")
except OSError:
    pass
' 2>/dev/null || true
}

cache_current_enterprise_license() {
    local response_file status license_key tmp_file
    response_file=$(mktemp)
    status=$(curl -sS -o "$response_file" -w '%{http_code}' "${GITLAB_URL}/api/v4/license/usage_export.csv" \
        -H "Authorization: Bearer ${ROOT_TOKEN}" \
        --retry 3 --retry-delay 2 --retry-all-errors \
        --connect-timeout 5 --max-time 60 2>/dev/null || true)
    if [ "$status" != "200" ]; then
        rm -f "$response_file"
        echo "WARN: Could not export Enterprise license for reuse (HTTP ${status})" >&2
        return 1
    fi

    license_key=$(python3 - "$response_file" <<'PY'
import csv
import io
import sys

with open(sys.argv[1], "r", encoding="utf-8", errors="replace", newline="") as handle:
    content = handle.read()

for row in csv.reader(io.StringIO(content)):
    if row and row[0] == "License Key" and len(row) > 1:
        print(row[1].strip(), end="")
        break
PY
)
    rm -f "$response_file"
    if [ -z "$license_key" ]; then
        echo "WARN: Enterprise license export did not include a reusable license key" >&2
        return 1
    fi

    mkdir -p "$(dirname "$ENTERPRISE_LICENSE_FILE")"
    umask 077
    tmp_file="${ENTERPRISE_LICENSE_FILE}.tmp.$$"
    printf '%s' "$license_key" > "$tmp_file"
    mv "$tmp_file" "$ENTERPRISE_LICENSE_FILE"
    chmod 600 "$ENTERPRISE_LICENSE_FILE" 2>/dev/null || true
    echo "    Enterprise license cached at ${ENTERPRISE_LICENSE_FILE_DISPLAY}"
}

normalize_bool() {
    case "$(printf '%s' "${1:-}" | tr '[:upper:]' '[:lower:]')" in
        1|true|yes|y|on) printf 'true' ;;
        *) printf 'false' ;;
    esac
}

looks_like_activation_code() {
    [[ "${1:-}" =~ ^[[:alnum:]]{24}$ ]]
}

license_plan() {
    local response status body plan
    response=$(curl -sS -w '\n%{http_code}' "${GITLAB_URL}/api/v4/license" \
        -H "Authorization: Bearer ${ROOT_TOKEN}" \
        --retry 3 --retry-delay 2 --retry-all-errors \
        --connect-timeout 5 --max-time 30 2>/dev/null || true)
    status=$(printf '%s' "$response" | tail -n 1)
    body=$(printf '%s' "$response" | sed '$d')
    if [ "$status" != "200" ]; then
        return 1
    fi
    plan=$(json_field "$body" "plan")
    if [ -z "$plan" ]; then
        return 1
    fi
    printf '%s' "$plan"
}

wait_for_enterprise_license() {
    local reason="$1"
    local plan
    for attempt in 1 2 3 4 5 6 7 8 9 10 11 12; do
        plan=$(license_plan || true)
        if [ -n "$plan" ]; then
            echo "    Enterprise license active (plan: ${plan})"
            return 0
        fi
        echo "    Waiting for Enterprise license from ${reason} (${attempt}/12), retrying in 10s..."
        sleep 10
    done
    return 1
}

bootstrap_root_pat_with_rails() {
    if ! command -v docker >/dev/null 2>&1; then
        return 1
    fi
    if ! docker compose -f "${COMPOSE_FILE}" ps -q gitlab >/dev/null 2>&1; then
        return 1
    fi

    local runner_output
    runner_output=$(docker compose -f "${COMPOSE_FILE}" exec -T gitlab gitlab-rails runner '
require "securerandom"

user = User.find_by_username!("root")
token_value = "glpat-#{SecureRandom.hex(24)}"
token = user.personal_access_tokens.create!(
  name: "e2e-root-bootstrap-#{Time.now.to_i}",
  scopes: ["api"],
  expires_at: Date.today + 1
)
token.set_token(token_value)
token.save!
puts "E2E_ROOT_TOKEN=#{token_value}"
' 2>&1) || {
        printf '    Rails bootstrap failed: %.300s\n' "${runner_output}" >&2
        return 1
    }

    printf '%s\n' "${runner_output}" | awk -F= '/^E2E_ROOT_TOKEN=/{print $2; found=1} END{exit !found}'
}

# 1. Get root OAuth token. The readiness endpoint can go green before the
# password grant is usable, especially on fresh GitLab containers.
echo "  [1/4] Authenticating as root..."
ROOT_TOKEN=""
LAST_AUTH_STATUS=""
LAST_AUTH_ERROR=""
for attempt in $(seq 1 "${ROOT_AUTH_ATTEMPTS}"); do
    OAUTH_ERROR=""
    OAUTH_RESPONSE=$(curl -sS -w '\n%{http_code}' "${GITLAB_URL}/oauth/token" \
        --data-urlencode "grant_type=password" \
        --data-urlencode "username=root" \
        --data-urlencode "password=${ROOT_PASSWORD}" \
        --retry 3 --retry-delay 2 --retry-all-errors \
        --connect-timeout 5 --max-time 30 2>/dev/null || true)

    if [ -n "$OAUTH_RESPONSE" ]; then
        LAST_AUTH_STATUS=$(printf '%s' "$OAUTH_RESPONSE" | tail -n 1)
        OAUTH_BODY=$(printf '%s' "$OAUTH_RESPONSE" | sed '$d')
        ROOT_TOKEN=$(json_field "$OAUTH_BODY" "access_token")
        OAUTH_ERROR=$(json_field "$OAUTH_BODY" "error")
        OAUTH_ERROR_DESCRIPTION=$(json_field "$OAUTH_BODY" "error_description")
        if [ -n "$OAUTH_ERROR" ] || [ -n "$OAUTH_ERROR_DESCRIPTION" ]; then
            LAST_AUTH_ERROR="${OAUTH_ERROR}${OAUTH_ERROR_DESCRIPTION:+: ${OAUTH_ERROR_DESCRIPTION}}"
        else
            LAST_AUTH_ERROR=""
        fi
    fi

    if [ "$OAUTH_ERROR" = "unsupported_grant_type" ]; then
        echo "    Root password grant unsupported; bootstrapping root PAT via GitLab Rails..."
        ROOT_TOKEN=$(bootstrap_root_pat_with_rails || true)
        if [ -n "$ROOT_TOKEN" ]; then
            break
        fi
    fi

    if [ -n "$ROOT_TOKEN" ]; then
        break
    fi
    echo "    Attempt ${attempt}/${ROOT_AUTH_ATTEMPTS} failed (HTTP ${LAST_AUTH_STATUS:-000}${LAST_AUTH_ERROR:+, ${LAST_AUTH_ERROR}}), retrying in ${ROOT_AUTH_INTERVAL}s..."
    sleep "${ROOT_AUTH_INTERVAL}"
done

if [ -z "$ROOT_TOKEN" ]; then
    echo "ERROR: Failed to authenticate as root after ${ROOT_AUTH_ATTEMPTS} attempts"
    exit 1
fi
echo "    Root API token obtained"

ENTERPRISE_MODE="$(normalize_bool "$(env_or_dotenv GITLAB_ENTERPRISE)")"
ENTERPRISE_LICENSE_RAW="$(env_or_dotenv ENTERPRISE_LICENSE)"
ENTERPRISE_ACTIVATION_CODE="$(env_or_dotenv GITLAB_ACTIVATION_CODE)"
ENTERPRISE_LICENSE_CACHED="$(enterprise_license_file_value)"
ENTERPRISE_LICENSE=""
ENTERPRISE_LICENSE_SOURCE=""
if [ -n "$ENTERPRISE_LICENSE_RAW" ] && ! looks_like_activation_code "$ENTERPRISE_LICENSE_RAW" ]; then
    ENTERPRISE_LICENSE="$ENTERPRISE_LICENSE_RAW"
    ENTERPRISE_LICENSE_SOURCE="ENTERPRISE_LICENSE"
elif [ -n "$ENTERPRISE_LICENSE_CACHED" ]; then
    ENTERPRISE_LICENSE="$ENTERPRISE_LICENSE_CACHED"
    ENTERPRISE_LICENSE_SOURCE="$ENTERPRISE_LICENSE_FILE_DISPLAY"
elif [ -z "$ENTERPRISE_ACTIVATION_CODE" ] && looks_like_activation_code "$ENTERPRISE_LICENSE_RAW" ]; then
    ENTERPRISE_ACTIVATION_CODE="$ENTERPRISE_LICENSE_RAW"
fi
if [ "$ENTERPRISE_MODE" = "true" ]; then
    if [ -z "$ENTERPRISE_LICENSE" ] && [ -z "$ENTERPRISE_ACTIVATION_CODE" ]; then
        echo "ERROR: GITLAB_ENTERPRISE=true requires ENTERPRISE_LICENSE, GITLAB_ACTIVATION_CODE, or ${ENTERPRISE_LICENSE_FILE_DISPLAY}" >&2
        exit 1
    fi
    if [ -n "$ENTERPRISE_LICENSE" ]; then
        EXISTING_PLAN=$(license_plan || true)
        if [ -n "$EXISTING_PLAN" ]; then
            echo "    Enterprise license already active (plan: ${EXISTING_PLAN})"
        else
            echo "    Installing Enterprise license from ${ENTERPRISE_LICENSE_SOURCE}"
            LICENSE_RESPONSE=$(curl -sS -w '\n%{http_code}' "${GITLAB_URL}/api/v4/license" \
                -X POST \
                -H "Authorization: Bearer ${ROOT_TOKEN}" \
                --data-urlencode "license=${ENTERPRISE_LICENSE}" \
                --retry 3 --retry-delay 2 --retry-all-errors \
                --connect-timeout 5 --max-time 60 2>/dev/null || true)
            LICENSE_STATUS=$(printf '%s' "$LICENSE_RESPONSE" | tail -n 1)
            LICENSE_BODY=$(printf '%s' "$LICENSE_RESPONSE" | sed '$d')
            case "$LICENSE_STATUS" in
                200|201)
                    echo "    Enterprise license installed"
                    ;;
                *)
                    echo "ERROR: Failed to install Enterprise license (HTTP ${LICENSE_STATUS})" >&2
                    if [ -n "$LICENSE_BODY" ]; then
                        printf '%.300s\n' "$LICENSE_BODY" >&2
                    fi
                    exit 1
                    ;;
            esac
            if ! wait_for_enterprise_license "license key"; then
                echo "ERROR: Enterprise license was installed but could not be verified" >&2
                exit 1
            fi
        fi
    else
        echo "    Verifying Enterprise activation code applied during installation"
        if ! wait_for_enterprise_license "activation code"; then
            echo "ERROR: Enterprise activation code was not applied. Start the GitLab EE container with GITLAB_ACTIVATION_CODE set, or provide a legacy .gitlab-license key in ENTERPRISE_LICENSE." >&2
            exit 1
        fi
        cache_current_enterprise_license || true
    fi
fi

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

# 1c. Enable iterations feature flags for EE (Premium/Ultimate).
# Feature.enable(:iterations) and Feature.enable(:iteration_license) are required
# because the iterations API returns 403 when the feature flag is not enabled,
# even with a valid Ultimate license and correct token permissions.
if [ "$ENTERPRISE_MODE" = "true" ]; then
    # Run the feature flag enablement and capture both output and exit status.
    # The '|| true' prevents `set -e` from aborting the script under any
    # command-substitution failure mode (exit code, pipefail, etc.).
    set +e
    ff_output=$(docker compose -f "${COMPOSE_FILE}" exec -T gitlab gitlab-rails runner \
        "Feature.enable(:iterations); Feature.enable(:iteration_license); puts 'OK'" 2>&1)
    ff_status=$?
    set -e
    if [ "$ff_status" -eq 0 ]; then
        echo "    Iterations feature flags enabled"
    else
        echo "    WARN: could not enable iterations feature flags (exit $ff_status, output: ${ff_output:-unknown})" >&2
    fi
fi

# 1d. Optionally configure LDAP against the e2e-ldap container. GitLab
# only honors LDAP if the integration is enabled; for EE test runs the
# setup detects the e2e-ldap container and PUTs /api/v4/ldap/ldapmain
# (which creates or updates the LDAP integration). The PUT response
# is validated and a non-2xx status is reported as a warning so that
# silent failures do not leave the groupldap_ee tests running
# against an unconfigured integration. When the container is not
# present, the groupldap_ee tests fall back to the error-path branch.
if [ "$ENTERPRISE_MODE" = "true" ] && docker compose -f "${COMPOSE_FILE}" ps -q e2e-ldap >/dev/null 2>&1; then
    set +e
    ldap_output=$(curl -sS -w '\n%{http_code}' "${GITLAB_URL}/api/v4/ldap/ldapmain" \
        -H "Authorization: Bearer ${ROOT_TOKEN}" \
        --retry 5 --retry-delay 2 --retry-all-errors \
        --connect-timeout 5 --max-time 30 2>&1)
    set -e
    ldap_code=$(printf '%s' "$ldap_output" | tail -n 1)
    ldap_body=$(printf '%s' "$ldap_output" | sed '$d')
    if [ "$ldap_code" = "404" ]; then
        # Create the LDAP integration. Capture the response and HTTP
        # status so we can validate success (2xx) before reporting it.
        set +e
        # Keep the LDAP bind password in sync with docker-compose.yml's
        # E2E_LDAP_ADMIN_PASSWORD (default: "ldapadmin") so groupldap_ee
        # auth succeeds when the test compose file overrides the env.
        LDAP_ADMIN_PASSWORD="${E2E_LDAP_ADMIN_PASSWORD:-ldapadmin}"
        ldap_put_output=$(curl -sS -w '\n%{http_code}' -X PUT "${GITLAB_URL}/api/v4/ldap/ldapmain" \
            -H "Authorization: Bearer ${ROOT_TOKEN}" \
            -d "host=ldap-e2e" \
            -d "port=389" \
            -d "uid=uid" \
            -d "bind_dn=cn=admin,dc=example,dc=com" \
            -d "password=${LDAP_ADMIN_PASSWORD}" \
            -d "encryption=no" \
            -d "verify_certificates=false" \
            -d "active_directory=false" \
            -d "block_auto_created_users=false" \
            --retry 3 --retry-delay 2 --retry-all-errors \
            --connect-timeout 5 --max-time 30 2>&1)
        set -e
        ldap_put_code=$(printf '%s' "$ldap_put_output" | tail -n 1)
        ldap_put_body=$(printf '%s' "$ldap_put_output" | sed '$d')
        case "$ldap_put_code" in
            2*)
                echo "    LDAP integration 'ldapmain' configured against e2e-ldap"
                ;;
            *)
                echo "    WARN: PUT ${GITLAB_URL}/api/v4/ldap/ldapmain returned HTTP ${ldap_put_code}, body: ${ldap_put_body:-unknown}" >&2
                ;;
        esac
    elif [ "$ldap_code" = "200" ]; then
        echo "    LDAP integration 'ldapmain' already configured"
    else
        echo "    WARN: could not query LDAP (HTTP ${ldap_code}, body: ${ldap_body:-unknown})" >&2
    fi
fi

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

# Pre-warm the GraphQL endpoint so the first work_items query in E2E
# does not trigger a cold Puma worker boot under tight context deadlines.
echo "  [3.5/4] Warming up GraphQL endpoint..."
for _ in 1 2 3; do
    curl -sS -o /dev/null -X POST "${GITLAB_URL}/api/graphql" \
        -H "Authorization: Bearer ${PAT}" \
        -H "Content-Type: application/json" \
        --data '{"query":"{ currentUser { id username } }"}' \
        --connect-timeout 5 --max-time 30 2>/dev/null || true
done

# 4. Write .env.docker
echo "  [4/4] Writing ${ENV_FILE_DISPLAY}..."
cat > "${ENV_FILE}" <<EOF
GITLAB_URL=${GITLAB_URL}
GITLAB_TOKEN=${PAT}
GITLAB_SKIP_TLS_VERIFY=true
GITLAB_ENTERPRISE=${ENTERPRISE_MODE}
E2E_MODE=docker
E2E_FIXTURE_URL=http://e2e-fixture:8080
E2E_GITLAB_INTERNAL_URL=http://gitlab-e2e
EOF

echo ""
echo "=== Setup complete ==="
echo "  User: ${TEST_USER} (admin)"
echo "  Token: ${PAT:0:10}..."
echo "  Enterprise: ${ENTERPRISE_MODE}"
echo "  Config: ${ENV_FILE_DISPLAY}"
echo ""
echo "To run E2E tests:"
echo "  set -a && source ${ENV_FILE_DISPLAY} && set +a"
echo "  go test -v -tags e2e -timeout 600s ./test/e2e/"
