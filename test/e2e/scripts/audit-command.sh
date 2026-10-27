#!/usr/bin/env bash
# Copyright (c) 2026 José M. Requena Plens
# SPDX-License-Identifier: MIT

# audit-command.sh: run cmd/audit_e2e_coverage the way the Docker run's gates
# need it run.
#
# Sourced by release-gate.sh and skip-gate.sh, and through them by
# run-docker-e2e.sh and the tests that drive those gates with a stand-in go. It
# defines a function and runs nothing, so sourcing it changes no state.

# run_audit_e2e_coverage builds cmd/audit_e2e_coverage, from the current
# directory, into a directory of its own, runs it with the arguments given, and
# returns the status it exits with, removing the directory whatever happened.
# The gates answer with that status and give 2 a meaning of its own, an input
# they could not judge, which go run would lose: it reports a program that
# failed as its own status 1, the gates' answer for a finding. A command that
# does not build returns go's status and runs nothing.
#
#   run_audit_e2e_coverage <flag>...
run_audit_e2e_coverage() {
    local dir status=0
    dir="$(mktemp -d)" || return 1
    go build -o "${dir}/audit_e2e_coverage" ./cmd/audit_e2e_coverage/ || status=$?
    if [ "${status}" -eq 0 ]; then
        "${dir}/audit_e2e_coverage" "$@" || status=$?
    fi
    rm -rf "${dir}"
    return "${status}"
}
