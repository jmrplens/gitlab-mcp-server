#!/usr/bin/env bash
# Copyright (c) 2026 José M. Requena Plens
# SPDX-License-Identifier: MIT

# skip-gate.sh: hold a complete Docker run's skips to the declared ones.
#
# Sourced by run-docker-e2e.sh, and by scripts/e2e_skip_gate_test.py, which
# drives these functions with a stand-in go. It defines functions and runs
# nothing, so sourcing it changes no state.
#
# The judgement is cmd/audit_e2e_coverage -check-skips, whose table of
# declared skips is cmd/audit_e2e_coverage/skip_declarations.go. What is here
# is how a run reaches it, what it does to the run's status, what a run asking
# to be complete has to be started with so that the gate can pass, and which
# scenario such a run runs last, in a final pass of its own.

# shellcheck source=test/e2e/scripts/audit-command.sh
. "$(dirname "${BASH_SOURCE[0]}")/audit-command.sh"

# gate_skips runs the skip gate over the go test -json stream of one run, from
# the repository root named by REPO_ROOT, printing what it finds and appending
# the same to the run's saved output, and returns the gate's status: 0 when
# every skip is declared and every declaration was needed, 1 when not, 2 when
# the stream could not be judged. The command is built and run by
# run_audit_e2e_coverage, so the 2 reaches the run as a 2.
#
# A run that made a final pass names that pass's stream as well, which the
# gate reads as -final-results: a runs-last declaration excuses the main
# pass's skip of its scenario only when that stream records the scenario
# passed, and a path with no stream there is a final pass that never ran.
#
#   gate_skips <ce|ee> <go test -json stream> <saved output> [final pass's stream]
gate_skips() {
    local runtime="$1" results="$2" output="$3" final="${4:-}"
    local final_pass=()
    if [ -n "${final}" ]; then
        final_pass=(-final-results "${final}")
    fi
    (
        cd "${REPO_ROOT}" || exit 1
        # The guarded expansion: bash before 4.4 calls an empty array unbound
        # under set -u.
        run_audit_e2e_coverage -check-skips -runtime "${runtime}" -results "${results}" ${final_pass[@]+"${final_pass[@]}"}
    ) 2>&1 | tee -a "${output}"
    return "${PIPESTATUS[0]}"
}

# final_pass_test prints the scenario a complete run of the runtime runs last,
# as its package under test/e2e/gitlab, its test and the switch that lets it
# run, and nothing for a runtime that has none. On ce that is the throttle
# probe of issue 954, which turns GitLab's unauthenticated API throttle on for
# the whole instance: run beside the other packages, a probe cut short before
# its cleanup would leave the throttle on for every one after it. So the main
# pass skips it with the switch off, and run-docker-e2e.sh runs it alone, with
# the switch on, in a final pass on the stack the main pass ran on and before
# the one teardown: nothing runs after it on the instance it changed, and no
# second stack is booted for it. Each line answers a runs-last declaration of
# cmd/audit_e2e_coverage/skip_declarations.go, and that table's test holds the
# two to each other.
#
#   final_pass_test <ce|ee>
final_pass_test() {
    case "$1" in
        ce) echo "ce TestAuthThrottle_InvalidTokens_SpendTheAddressUnauthenticatedAllowance E2E_AUTH_THROTTLE_PROBE" ;;
    esac
}

# complete_run_value prints the value the caller gave a key, looked for where
# the harness looks and in its order (the process environment, then each
# dotenv file named, which run-docker-e2e.sh passes as the file E2E_ENV_FILE
# names and the repository .env), and nothing when none of them gives one. An
# empty value gives none. test/e2e/.env.docker, which the harness reads
# between the two files, is not asked: the setup script writes it afresh and
# never with a key a complete run asks about. The key's name is held in a
# variable no dotenv file of this repository spells, since the file is sourced
# in the same shell that reads it back.
#
#   complete_run_value <key> [dotenv file...]
complete_run_value() {
    local _complete_run_key="$1" file value
    shift
    if [ -n "${!_complete_run_key:-}" ]; then
        printf '%s\n' "${!_complete_run_key}"
        return 0
    fi
    for file in "$@"; do
        if [ -z "${file}" ] || [ ! -f "${file}" ]; then
            continue
        fi
        # shellcheck source=/dev/null
        value="$(set -a && . "${file}" >/dev/null 2>&1; printf '%s' "${!_complete_run_key:-}")" || value=""
        if [ -n "${value}" ]; then
            printf '%s\n' "${value}"
            return 0
        fi
    done
}

# complete_run_external_network prints the E2E_EXTERNAL_NETWORK a complete
# run is started with: the value the caller gave, wherever complete_run_value
# finds it, and true when the caller gave none, since the host running a
# complete run has Internet. The script exports what this prints, so the
# harness reads that value whatever the files say, and a false written in
# .env is honoured rather than outranked by a default.
#
#   complete_run_external_network [dotenv file...]
complete_run_external_network() {
    local value
    value="$(complete_run_value E2E_EXTERNAL_NETWORK "$@")"
    printf '%s\n' "${value:-true}"
}

# complete_run_refusal prints, one per line, why a run asked to be complete
# cannot be, and nothing when it can. A complete run is held to the skips its
# runtime declares, and none is declared for a scenario the run chose to leave
# out, so the gate would fail such a run after an hour of tests for a reason
# known before the first. The external network is read the way the harness
# reads it, case aside; E2E_BITBUCKET the way run-docker-e2e.sh reads it.
#
# The throttle probe's switch, E2E_AUTH_THROTTLE_PROBE, is refused for a
# different reason: the probe changes an instance-wide setting, so a complete
# run turns the switch on for its final pass alone (final_pass_test), and a
# switch a person set would have the main pass run the probe beside the other
# packages, where a probe cut short would leave the throttle on for the ones
# after it. It is read the way the probe reads it, case aside, and may be left
# out.
#
#   complete_run_refusal <E2E_EXTERNAL_NETWORK> <E2E_BITBUCKET> [E2E_AUTH_THROTTLE_PROBE]
complete_run_refusal() {
    if [[ ! $1 =~ ^[Tt][Rr][Uu][Ee]$ ]]; then
        echo "E2E_EXTERNAL_NETWORK is '$1', so the GitHub, Gists and Bitbucket Cloud importers, which call a public URL, would skip"
    fi
    if [ "$2" != "true" ]; then
        echo "E2E_BITBUCKET is '$2', so the Bitbucket fixture would not start and the Bitbucket Server import would skip"
    fi
    if [[ ${3:-} =~ ^[Tt][Rr][Uu][Ee]$ ]]; then
        echo "E2E_AUTH_THROTTLE_PROBE is '$3', so the main pass would run the throttle probe beside the other packages, where a probe cut short would leave GitLab's unauthenticated API throttle on for the ones after it; a complete run turns the switch on for its final pass alone, so leave it unset"
    fi
}

# run_status_after_final_pass prints the status a run's tests end with once
# its final pass has run: the main pass's when it failed, so the failure the
# run reports is the one that came first, and the final pass's when the main
# pass passed. A final pass that could not run leaves its status at 0, and the
# gate fails the run for the skip it leaves unproven.
#
#   run_status_after_final_pass <main pass's status> <final pass's status>
run_status_after_final_pass() {
    if [ "$1" -ne 0 ]; then
        echo "$1"
    else
        echo "$2"
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
