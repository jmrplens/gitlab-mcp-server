#!/usr/bin/env bash
# skip-gate.sh: hold a complete Docker run's skips to the declared ones.
#
# Sourced by run-docker-e2e.sh, and by scripts/e2e_skip_gate_test.py, which
# drives these functions with a stand-in go. It defines functions and runs
# nothing, so sourcing it changes no state.
#
# The judgement is cmd/audit_e2e_coverage -check-skips, whose table of
# declared skips is cmd/audit_e2e_coverage/skip_declarations.go. What is here
# is how a run reaches it, what it does to the run's status, and what a run
# asking to be complete has to be started with so that the gate can pass.

# gate_skips runs the skip gate over the go test -json stream of one run, from
# the repository root named by REPO_ROOT, printing what it finds and appending
# the same to the run's saved output, and returns the gate's status: 0 when
# every skip is declared and every declaration was needed, 1 when not, 2 when
# the stream could not be judged.
#
#   gate_skips <ce|ee> <go test -json stream> <saved output>
gate_skips() {
    local runtime="$1" results="$2" output="$3"
    (
        cd "${REPO_ROOT}" || exit 1
        go run ./cmd/audit_e2e_coverage/ -check-skips -runtime "${runtime}" -results "${results}"
    ) 2>&1 | tee -a "${output}"
    return "${PIPESTATUS[0]}"
}

# complete_run_external_network prints the E2E_EXTERNAL_NETWORK a complete
# run is started with: the value the caller gave, looked for where the harness
# looks and in its order (the process environment, then each dotenv file
# named, which run-docker-e2e.sh passes as the file E2E_ENV_FILE names and the
# repository .env), and true when none of them gives one, since the host
# running a complete run has Internet. An empty value gives none. The script
# exports what this prints, so the harness reads that value whatever the files
# say, and a false written in .env is honoured rather than outranked by a
# default. test/e2e/.env.docker, which the harness reads between the two, is
# not asked: the setup script writes it afresh and never with this key.
#
#   complete_run_external_network [dotenv file...]
complete_run_external_network() {
    local file value
    if [ -n "${E2E_EXTERNAL_NETWORK:-}" ]; then
        printf '%s\n' "${E2E_EXTERNAL_NETWORK}"
        return 0
    fi
    for file in "$@"; do
        if [ -z "${file}" ] || [ ! -f "${file}" ]; then
            continue
        fi
        # shellcheck source=/dev/null
        value="$(set -a && . "${file}" >/dev/null 2>&1; printf '%s' "${E2E_EXTERNAL_NETWORK:-}")" || value=""
        if [ -n "${value}" ]; then
            printf '%s\n' "${value}"
            return 0
        fi
    done
    echo true
}

# complete_run_refusal prints, one per line, why a run asked to be complete
# cannot be, and nothing when it can. A complete run is held to the skips its
# runtime declares, and none is declared for a scenario the run chose to leave
# out, so the gate would fail such a run after an hour of tests for a reason
# known before the first. The external network is read the way the harness
# reads it, case aside; E2E_BITBUCKET the way run-docker-e2e.sh reads it.
#
#   complete_run_refusal <E2E_EXTERNAL_NETWORK> <E2E_BITBUCKET>
complete_run_refusal() {
    if [[ ! $1 =~ ^[Tt][Rr][Uu][Ee]$ ]]; then
        echo "E2E_EXTERNAL_NETWORK is '$1', so the GitHub, Gists and Bitbucket Cloud importers, which call a public URL, would skip"
    fi
    if [ "$2" != "true" ]; then
        echo "E2E_BITBUCKET is '$2', so the Bitbucket fixture would not start and the Bitbucket Server import would skip"
    fi
}

# run_status_after_gate prints the status a run ends with once its skips are
# judged: the tests' own when they failed, so a failing run is never reported
# as a skip finding, and the gate's when they passed.
#
#   run_status_after_gate <tests' status> <gate's status>
run_status_after_gate() {
    if [ "$1" -ne 0 ]; then
        echo "$1"
    else
        echo "$2"
    fi
}
