#!/usr/bin/env bash
# run-docker-e2e.sh: the Docker lifecycle of one end-to-end run, in one place.
#
# Usage: run-docker-e2e.sh <ce|ee> [--] <go test arguments>
#
#   ce  starts gitlab/gitlab-ce with the CI runner, the fixture service and
#       the Bitbucket import fixture, and runs the arguments as an unlicensed
#       runtime.
#   ee  starts gitlab/gitlab-ee, activates it with the cached license or the
#       activation code the repository .env carries, registers the runner and
#       runs the arguments as a licensed runtime. No Bitbucket: the importer
#       scenarios are Free and run under ce.
#
# Everything after the runtime, with an optional -- in front of it, is handed
# to go test through gotestsum: the packages, the -run filter, the timeout.
# The script adds the tags and the two flags every run needs, -p 1 because
# capability locks are process-local and -count=1 because a cached PASS
# records no calls.
#
# It exists because the two Docker targets in the Makefile duplicated this
# lifecycle line for line, and a fix to one did not reach the other. The
# provisioning steps are the same scripts the targets called: wait, setup
# with three attempts, runner, and the Bitbucket fixture. The stack is torn
# down whatever happened, and the run's status is the exit status; a
# teardown that failed is reported only when the run passed, so a failing
# run is never mistaken for a failing teardown.
#
# Docker follows DOCKER_HOST or the active context, so the stack can run on
# another machine: nothing here bind-mounts a local path, copies into a
# container, or assumes the daemon shares this filesystem. Point
# E2E_DOCKER_GITLAB_URL at the address the fixture is reached from.
#
# Environment, all optional:
#   E2E_DOCKER_GITLAB_URL          http://localhost:8929
#   E2E_DOCKER_BITBUCKET_URL       http://localhost:7990
#   E2E_BITBUCKET                  true under ce; false skips the Bitbucket fixture
#   E2E_KEEP_STACK                 true leaves the stack up after the run, for a look
#   E2E_REPORT_DIR                 dist/e2e-reports, resolved from the repository root
#   E2E_REPORT_NAME                stem of the junit, json and output files: e2e-<runtime>
#   GITLAB_MCP_TEST_E2E_CALLS_DIR  where the suite records its calls; cleared first, must be absolute
#   GITLAB_IMAGE                   the image for the runtime; the defaults above
#   GOTESTSUM                      the gotestsum binary; the one on PATH
#   E2E_SERVER_BINARY, E2E_COMMIT  forwarded to the run as they are
#   E2E_GITLAB_EXTERNAL_URL, E2E_REGISTRY_EXTERNAL_URL, E2E_BITBUCKET_BIND
#                                  what the compose file publishes; derived from the URL above
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"
COMPOSE_FILE="${REPO_ROOT}/test/e2e/docker-compose.yml"

usage() {
    echo "usage: $0 <ce|ee> [--] <go test arguments>" >&2
    exit 2
}

RUNTIME="${1:-}"
case "${RUNTIME}" in
    ce|ee) ;;
    *) usage ;;
esac
shift
if [ "${1:-}" = "--" ]; then
    shift
fi
if [ "$#" -eq 0 ]; then
    echo "run-docker-e2e.sh: no go test arguments; name at least one package" >&2
    usage
fi

E2E_DOCKER_GITLAB_URL="${E2E_DOCKER_GITLAB_URL:-http://localhost:8929}"
E2E_DOCKER_BITBUCKET_URL="${E2E_DOCKER_BITBUCKET_URL:-http://localhost:7990}"
E2E_REPORT_DIR="${E2E_REPORT_DIR:-dist/e2e-reports}"
E2E_REPORT_NAME="${E2E_REPORT_NAME:-e2e-${RUNTIME}}"
GOTESTSUM="${GOTESTSUM:-gotestsum}"
case "${E2E_REPORT_DIR}" in
    /*) ;;
    *) E2E_REPORT_DIR="${REPO_ROOT}/${E2E_REPORT_DIR}" ;;
esac

# What the compose file publishes follows the address the fixture is reached
# from, so the web_url fields GitLab answers with are the ones the tests reach.
export E2E_GITLAB_EXTERNAL_URL="${E2E_GITLAB_EXTERNAL_URL:-${E2E_DOCKER_GITLAB_URL}}"
if [ -z "${E2E_REGISTRY_EXTERNAL_URL:-}" ] && [ "${E2E_DOCKER_GITLAB_URL%:8929}" != "${E2E_DOCKER_GITLAB_URL}" ]; then
    export E2E_REGISTRY_EXTERNAL_URL="${E2E_DOCKER_GITLAB_URL%:8929}:5050"
fi
export E2E_BITBUCKET_BIND="${E2E_BITBUCKET_BIND:-127.0.0.1}"
export E2E_COMMIT="${E2E_COMMIT:-$(git -C "${REPO_ROOT}" rev-parse HEAD 2>/dev/null || echo unknown)}"

# Every down names the Bitbucket profile, whatever this run starts: a
# container an earlier run started under the profile is not an orphan to a
# down that does not name it, and would be left running beside the new stack.
COMPOSE=(docker compose -f "${COMPOSE_FILE}")
DOWN=("${COMPOSE[@]}" --profile bitbucket down -v --remove-orphans)
WITH_BITBUCKET=false
if [ "${RUNTIME}" = "ce" ]; then
    export GITLAB_IMAGE="${GITLAB_IMAGE:-gitlab/gitlab-ce:latest}"
    if [ "${E2E_BITBUCKET:-true}" = "true" ]; then
        WITH_BITBUCKET=true
    fi
else
    export GITLAB_IMAGE="${GITLAB_IMAGE:-gitlab/gitlab-ee:latest}"
fi

# The status the run ended with, read by the teardown. Unset until the tests
# have run, so a failure during provisioning exits with the shell's own.
RUN_STATUS=""

teardown() {
    local exit_status=$?
    if [ "${E2E_KEEP_STACK:-false}" = "true" ]; then
        echo "=== E2E_KEEP_STACK is set; the stack stays up ==="
        [ -n "${RUN_STATUS}" ] && exit "${RUN_STATUS}"
        exit "${exit_status}"
    fi
    echo "=== Tearing down ==="
    local teardown_status=0
    "${DOWN[@]}" || teardown_status=$?
    echo "=== E2E reports saved to ${E2E_REPORT_DIR}/ ==="
    if [ -n "${RUN_STATUS}" ]; then
        if [ "${RUN_STATUS}" -ne 0 ]; then exit "${RUN_STATUS}"; fi
        exit "${teardown_status}"
    fi
    exit "${exit_status}"
}
trap teardown EXIT

echo "=== Cleaning up previous containers (if any) ==="
"${DOWN[@]}" 2>/dev/null || true

if [ "${WITH_BITBUCKET}" = "true" ]; then
    echo "=== Starting ephemeral GitLab CE and Bitbucket fixture ==="
    E2E_BITBUCKET_ADMIN_PASSWORD="$(openssl rand -hex 16)"
    export E2E_BITBUCKET_ADMIN_PASSWORD
    "${COMPOSE[@]}" --profile bitbucket up -d
elif [ "${RUNTIME}" = "ce" ]; then
    echo "=== Starting ephemeral GitLab CE ==="
    "${COMPOSE[@]}" up -d
else
    echo "=== Starting ephemeral GitLab EE ==="
    activation_code="$("${SCRIPT_DIR}/enterprise-activation-code.sh")"
    if [ -n "${activation_code}" ]; then
        echo "    Passing Enterprise activation code to GitLab EE container"
    elif [ -s "${E2E_ENTERPRISE_LICENSE_FILE:-${REPO_ROOT}/test/e2e/.enterprise-license}" ]; then
        echo "    Reusing cached Enterprise license during setup"
    fi
    GITLAB_ACTIVATION_CODE="${activation_code}" "${COMPOSE[@]}" up -d
fi

echo "=== Waiting for GitLab readiness ==="
"${SCRIPT_DIR}/wait-for-gitlab.sh" "${E2E_DOCKER_GITLAB_URL}" 600

echo "=== Setting up test user and token ==="
for attempt in 1 2 3; do
    if [ "${RUNTIME}" = "ee" ]; then
        GITLAB_ENTERPRISE=true "${SCRIPT_DIR}/setup-gitlab.sh" "${E2E_DOCKER_GITLAB_URL}" && break
    else
        "${SCRIPT_DIR}/setup-gitlab.sh" "${E2E_DOCKER_GITLAB_URL}" && break
    fi
    if [ "${attempt}" -eq 3 ]; then
        echo "ERROR: setup-gitlab.sh failed after 3 attempts" >&2
        exit 1
    fi
    echo "WARN: setup-gitlab.sh failed (attempt ${attempt}/3), retrying in 5s..."
    sleep 5
done

echo "=== Registering GitLab Runner ==="
"${SCRIPT_DIR}/register-runner.sh" "${E2E_DOCKER_GITLAB_URL}"

if [ "${WITH_BITBUCKET}" = "true" ]; then
    echo "=== Provisioning Bitbucket import fixture ==="
    "${SCRIPT_DIR}/setup-bitbucket.sh" "${E2E_DOCKER_BITBUCKET_URL}"
fi

echo "=== Running E2E tests (${RUNTIME}) ==="
mkdir -p "${E2E_REPORT_DIR}"
if [ -n "${GITLAB_MCP_TEST_E2E_CALLS_DIR:-}" ]; then
    case "${GITLAB_MCP_TEST_E2E_CALLS_DIR}" in
        /*) ;;
        *)
            echo "ERROR: GITLAB_MCP_TEST_E2E_CALLS_DIR must be absolute, and is ${GITLAB_MCP_TEST_E2E_CALLS_DIR}" >&2
            exit 1
            ;;
    esac
    rm -rf "${GITLAB_MCP_TEST_E2E_CALLS_DIR}"
    mkdir -p "${GITLAB_MCP_TEST_E2E_CALLS_DIR}"
fi

set +e
(
    set -a
    # shellcheck disable=SC1091
    . "${REPO_ROOT}/test/e2e/.env.docker"
    set +a
    cd "${REPO_ROOT}" || exit 1
    E2E_MODE=docker "${GOTESTSUM}" \
        --format testdox \
        --junitfile "${E2E_REPORT_DIR}/${E2E_REPORT_NAME}-junit.xml" \
        --jsonfile "${E2E_REPORT_DIR}/${E2E_REPORT_NAME}-log.json" \
        -- -tags e2e -p 1 -count=1 "$@"
) 2>&1 | tee "${E2E_REPORT_DIR}/${E2E_REPORT_NAME}-output.txt"
RUN_STATUS="${PIPESTATUS[0]}"
set -e
