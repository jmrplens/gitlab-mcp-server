#!/usr/bin/env bash
set -euo pipefail

# Register a GitLab Runner against the ephemeral GitLab instance.
# Requires: GitLab already healthy. Uses test/e2e/.env.docker when available.
# Usage: ./test/e2e/scripts/register-runner.sh [GITLAB_URL]

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
E2E_DIR="$(dirname "$SCRIPT_DIR")"
COMPOSE_FILE="${E2E_DOCKER_COMPOSE_FILE:-${E2E_DIR}/docker-compose.yml}"
ENV_FILE="${E2E_DIR}/.env.docker"

CLI_GITLAB_URL="${1:-}"
CLI_GITLAB_INTERNAL_URL="${2:-}"

if [ -f "${ENV_FILE}" ]; then
    set -a
    . "${ENV_FILE}"
    set +a
fi

GITLAB_URL="${CLI_GITLAB_URL:-${GITLAB_URL:-http://localhost:8929}}"
# Runner needs GitLab's internal Docker hostname (not localhost)
GITLAB_INTERNAL_URL="${CLI_GITLAB_INTERNAL_URL:-${E2E_GITLAB_INTERNAL_URL:-http://gitlab-e2e:80}}"
ROOT_PASSWORD="E2e_R0ot!xK9mZ#2026"
ROOT_AUTH_ATTEMPTS="${E2E_ROOT_AUTH_ATTEMPTS:-30}"
ROOT_AUTH_INTERVAL="${E2E_ROOT_AUTH_INTERVAL:-10}"
RUNNER_INTERNAL_ATTEMPTS="${E2E_RUNNER_INTERNAL_ATTEMPTS:-60}"
RUNNER_INTERNAL_INTERVAL="${E2E_RUNNER_INTERNAL_INTERVAL:-5}"

echo "=== Registering GitLab Runner ==="

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
  name: "e2e-runner-bootstrap-#{Time.now.to_i}",
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

authenticate_root() {
    local oauth_response oauth_body oauth_error oauth_token last_status last_error

    for attempt in $(seq 1 "${ROOT_AUTH_ATTEMPTS}"); do
        oauth_response=$(curl -sS -w '\n%{http_code}' "${GITLAB_URL}/oauth/token" \
            --data-urlencode "grant_type=password" \
            --data-urlencode "username=root" \
            --data-urlencode "password=${ROOT_PASSWORD}" \
            --retry 3 --retry-delay 2 --retry-all-errors \
            --connect-timeout 5 --max-time 30 2>/dev/null || true)

        if [ -n "$oauth_response" ]; then
            last_status=$(printf '%s' "$oauth_response" | tail -n 1)
            oauth_body=$(printf '%s' "$oauth_response" | sed '$d')
            oauth_token=$(json_field "$oauth_body" "access_token")
            oauth_error=$(json_field "$oauth_body" "error")
            last_error=$(json_field "$oauth_body" "error_description")

            if [ -n "$oauth_token" ]; then
                API_AUTH_HEADER="Authorization: Bearer ${oauth_token}"
                return 0
            fi

            if [ "$oauth_error" = "unsupported_grant_type" ]; then
                echo "    Root password grant unsupported; bootstrapping root PAT via GitLab Rails..."
                local root_pat
                root_pat=$(bootstrap_root_pat_with_rails || true)
                if [ -n "$root_pat" ]; then
                    API_AUTH_HEADER="PRIVATE-TOKEN: ${root_pat}"
                    return 0
                fi
            fi
        fi

        echo "    Attempt ${attempt}/${ROOT_AUTH_ATTEMPTS} failed (HTTP ${last_status:-000}${last_error:+, ${last_error}}), retrying in ${ROOT_AUTH_INTERVAL}s..."
        sleep "${ROOT_AUTH_INTERVAL}"
    done

    return 1
}

create_runner() {
    local auth_header="$1"
    local response body status

    response=$(curl -sS -w '\n%{http_code}' "${GITLAB_URL}/api/v4/user/runners" \
        -H "$auth_header" \
        -d "runner_type=instance_type" \
        -d "description=e2e-docker-runner" \
        -d "tag_list=e2e,docker" \
        -d "run_untagged=true" \
        --retry 3 --retry-delay 2 --retry-all-errors \
        --connect-timeout 5 --max-time 30 2>/dev/null || true)

    status=$(printf '%s' "$response" | tail -n 1)
    body=$(printf '%s' "$response" | sed '$d')
    RUNNER_TOKEN=$(json_field "$body" "token")

    if [ -n "$RUNNER_TOKEN" ]; then
        return 0
    fi

    RUNNER_ERROR="HTTP ${status:-000}: $(printf '%.300s' "$body")"
    return 1
}

wait_for_internal_gitlab() {
    local runner_container="$1"
    local internal_url="${GITLAB_INTERNAL_URL%/}"

    for attempt in $(seq 1 "${RUNNER_INTERNAL_ATTEMPTS}"); do
        if docker exec "$runner_container" sh -c '
url="$1"
if command -v curl >/dev/null 2>&1; then
    curl -sf "${url}/-/readiness?all=1" >/dev/null
elif command -v wget >/dev/null 2>&1; then
    wget -q --spider "${url}/-/readiness?all=1"
else
    exit 2
fi
' sh "$internal_url" >/dev/null 2>&1; then
            return 0
        fi

        echo "    Internal GitLab URL not reachable from runner (${attempt}/${RUNNER_INTERNAL_ATTEMPTS}), retrying in ${RUNNER_INTERNAL_INTERVAL}s..."
        sleep "${RUNNER_INTERNAL_INTERVAL}"
    done

    return 1
}

API_AUTH_HEADER=""
RUNNER_TOKEN=""
RUNNER_ERROR=""

echo "  [1/3] Selecting API credentials..."
if [ -n "${GITLAB_TOKEN:-}" ]; then
    API_AUTH_HEADER="PRIVATE-TOKEN: ${GITLAB_TOKEN}"
    echo "    Using admin PAT from ${ENV_FILE}"
else
    echo "    ${ENV_FILE} did not provide GITLAB_TOKEN; falling back to root bootstrap"
    if ! authenticate_root; then
        echo "ERROR: Failed to authenticate as root after ${ROOT_AUTH_ATTEMPTS} attempts"
        exit 1
    fi
fi

echo "  [2/3] Creating runner via API..."
if ! create_runner "$API_AUTH_HEADER"; then
    echo "    Runner creation failed with selected token (${RUNNER_ERROR})"
    echo "    Bootstrapping root PAT via GitLab Rails and retrying..."
    ROOT_PAT=$(bootstrap_root_pat_with_rails || true)
    if [ -n "$ROOT_PAT" ]; then
        API_AUTH_HEADER="PRIVATE-TOKEN: ${ROOT_PAT}"
    elif ! authenticate_root; then
        echo "ERROR: Failed to obtain root API token for runner creation"
        exit 1
    fi

    if ! create_runner "$API_AUTH_HEADER"; then
        echo "ERROR: Failed to create runner via API (${RUNNER_ERROR})"
        exit 1
    fi
fi

echo "    Runner authentication token obtained"

echo "  [3/3] Configuring runner container..."
RUNNER_CONTAINER=$(docker compose -f "$COMPOSE_FILE" ps -q gitlab-runner 2>/dev/null || true)

if [ -z "$RUNNER_CONTAINER" ]; then
    echo "  WARN: gitlab-runner container not found. Skipping runner registration."
    exit 0
fi

# Detect the Docker network created by compose (varies by directory name)
COMPOSE_NETWORK=$(docker compose -f "$COMPOSE_FILE" ps --format json 2>/dev/null | python3 -c "
import sys, json
for line in sys.stdin:
    obj = json.loads(line)
    for net in obj.get('Networks', '').split(','):
        net = net.strip()
        if net:
            print(net)
            sys.exit(0)
" 2>/dev/null || echo "e2e_default")

echo "    Waiting for ${GITLAB_INTERNAL_URL} from runner container..."
if ! wait_for_internal_gitlab "$RUNNER_CONTAINER"; then
    echo "ERROR: GitLab internal URL ${GITLAB_INTERNAL_URL} is not reachable from gitlab-runner after ${RUNNER_INTERNAL_ATTEMPTS} attempts"
    exit 1
fi

docker exec "$RUNNER_CONTAINER" gitlab-runner register \
    --non-interactive \
    --url "${GITLAB_INTERNAL_URL}" \
    --token "${RUNNER_TOKEN}" \
    --executor docker \
    --docker-image "alpine:latest" \
    --docker-network-mode "${COMPOSE_NETWORK}" \
    --description "e2e-docker-runner"

# Bump concurrent jobs and prefer cached images so the vulnerability
# SAST pipeline (which uses a heavy analyzer image) doesn't have to
# re-pull on every run. Without this, single-job runners and cold pulls
# are the dominant source of E2E flake on the vulnerability lifecycle.
# The edits below are idempotent so repeated runs don't accumulate
# duplicate keys. The runner image is minimal (no bash/tomlq), so we
# use sed.
docker exec "$RUNNER_CONTAINER" sh -c '
set -e
# Match any concurrent value, not only "1", and replace it with 4.
sed -i "s|^concurrent = [0-9]*$|concurrent = 4|" /etc/gitlab-runner/config.toml
# Remove any existing pull_policy line(s) under [runners.docker] so the
# next sed only inserts the canonical one. (sed ranges are processed
# line by line; this strips the line itself.)
sed -i "/^  pull_policy = /d" /etc/gitlab-runner/config.toml
# Insert a single canonical pull_policy after the [runners.docker] header.
sed -i "s|^  \[runners\.docker\]$|&\n  pull_policy = [\"if-not-present\"]|" /etc/gitlab-runner/config.toml
'

# Reload the runner so the new config takes effect. The runner
# image is minimal (no bash, no full process tree) and the
# restart can fail for non-fatal reasons even when the runner is
# already running with the new config (written by the register
# step above). Treat the restart as best-effort: log a warning
# and continue with the test suite. The next pipeline will pick
# up the new concurrent/pull_policy settings on the next poll.
if ! docker exec "$RUNNER_CONTAINER" gitlab-runner restart 2>/dev/null; then
    echo "WARN: gitlab-runner restart failed inside $RUNNER_CONTAINER (runner is already running; the concurrent/pull_policy config edit will apply on the next poll)" >&2
fi

echo ""
echo "=== Runner registration complete ==="
echo "  Verify: curl -s ${GITLAB_URL}/api/v4/runners/all -H 'PRIVATE-TOKEN: ...'"
