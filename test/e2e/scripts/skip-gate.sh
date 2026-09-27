#!/usr/bin/env bash
# skip-gate.sh: hold a complete Docker run's skips to the declared ones.
#
# Sourced by run-docker-e2e.sh, and by scripts/e2e_skip_gate_test.py, which
# drives these functions with a stand-in go. It defines functions and runs
# nothing, so sourcing it changes no state.
#
# The judgement is cmd/audit_e2e_coverage -check-skips, whose table of
# declared skips is cmd/audit_e2e_coverage/skip_declarations.go. What is here
# is only how a run reaches it and what it does to the run's status.

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
