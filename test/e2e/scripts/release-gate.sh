#!/usr/bin/env bash
# release-gate.sh: stop a Docker run before go test when the GitLab it booted is
# newer than the release the fine-grained action table was recorded from.
#
# Sourced by run-docker-e2e.sh, and by scripts/e2e_release_gate_test.py, which
# drives these functions with a stand-in go. It defines functions and runs
# nothing, so sourcing it changes no state.
#
# The judgement is cmd/audit_e2e_coverage -check-release, which takes its rule
# from internal/testutil/e2erelease, the package the e2e harness refuses a
# self-hosted run with in its environment setup (issue 1155), so the script and
# the harness stop on the same releases with the same words. What is here is
# how a run reaches it and what it hands it. Asking after the boot costs a stale
# table the few minutes the boot took rather than the whole suite, which would
# otherwise pass with every fine-grained session in phase A.

# shellcheck source=test/e2e/scripts/audit-command.sh
. "$(dirname "${BASH_SOURCE[0]}")/audit-command.sh"

# gate_release asks the release gate about the version the instance reported,
# from the repository root named by REPO_ROOT, and returns the gate's status:
# 0 when the run may go ahead (the table's release, the prerelease right after
# it, or an older one, which the gate names in one line), 1 when the release is
# newer than both states a grant is judged in, 2 when the version could not be
# read. It is handed what tested_gitlab_version printed, "<version> (revision
# <revision>)" or "unknown (<reason>)", and passes the first word, so an
# instance that did not answer its version stops the run rather than passing it.
# The command is built and run by run_audit_e2e_coverage, so the 2 reaches the
# run as a 2.
#
#   gate_release <what tested_gitlab_version printed>
gate_release() {
    local tested="$1"
    (
        cd "${REPO_ROOT}" || exit 1
        run_audit_e2e_coverage -check-release -gitlab-version "${tested%% *}"
    )
}
