#!/usr/bin/env python3
# Copyright (c) 2026 José M. Requena Plens
# SPDX-License-Identifier: MIT

"""Tests how a Docker e2e run reaches the release gate (issue 1155).

A Docker run against a GitLab newer than the release the fine-grained action
table was recorded from stops before go test: test/e2e/scripts/run-docker-e2e.sh
hands the version the instance answered to cmd/audit_e2e_coverage
-check-release. What the gate decides is tested with the command and with the
rule in internal/testutil/e2erelease; what is tested here is the part in the
shell, which lives in test/e2e/scripts/release-gate.sh so that it can be driven
with a stand-in go: which command runs, from where, with which version, what
its status does, and that the lifecycle script asks it after the instance
answered and before the tests, on every run.

Run with:

    python3 -m unittest discover -s scripts -p 'e2e_release_gate_test.py'
"""

import os
import re
import subprocess
import tempfile
import unittest

ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
SCRIPT = os.path.join(ROOT, "test", "e2e", "scripts", "release-gate.sh")
RUNNER = os.path.join(ROOT, "test", "e2e", "scripts", "run-docker-e2e.sh")

# The stand-in go writes where it ran and what it was asked to GO_LOG, one
# line each, and answers a build the way go build does: with nothing on
# stdout and a program at the -o path, or with GO_STATUS and no program when
# that is not zero. The program it writes is a real one, which prints where it
# ran and what it was asked and exits with AUDIT_STATUS, so the status the
# gate answers with is one a process exited with and not one a shell function
# returned. The driver sources the script under the options run-docker-e2e.sh
# runs with, calls the function the way that script does, and prints the
# status last.
DRIVER = r"""
set -euo pipefail
. "$1"
shift
go() {
    printf 'go ran in %s\ngo %s\n' "${PWD}" "$*" >>"${GO_LOG}"
    if [ "$1" != build ] || [ "$2" != -o ]; then
        echo "the stand-in go builds with -o and does nothing else" >&2
        return 1
    fi
    if [ "${GO_STATUS}" != 0 ]; then
        echo "a build error on stderr" >&2
        return "${GO_STATUS}"
    fi
    printf '#!/usr/bin/env bash\necho "audit ran in ${PWD}"\necho "audit $*"\nexit %s\n' "${AUDIT_STATUS}" >"$3"
    chmod +x "$3"
}
status=0
"$@" || status=$?
echo "status=${status}"
"""


def drive(*command, go_status=0, audit_status=0, repo_root=None):
    """Runs one function of the script and returns its stdout lines, what the
    stand-in go was asked, one line each, with the directory the gate built
    into written as $BIN_DIR, and what the gate left in its temporary
    directory."""
    with tempfile.TemporaryDirectory() as scratch:
        log = os.path.join(scratch, "go.log")
        tmp = os.path.join(scratch, "tmp")
        os.mkdir(tmp)
        env = dict(os.environ)
        env.update(GO_STATUS=str(go_status), AUDIT_STATUS=str(audit_status), GO_LOG=log, TMPDIR=tmp)
        if repo_root is not None:
            env["REPO_ROOT"] = repo_root
        result = subprocess.run(
            ["bash", "-c", DRIVER, "driver", SCRIPT, *command],
            env=env,
            capture_output=True,
            text=True,
            check=True,
        )
        asked = []
        if os.path.exists(log):
            with open(log, encoding="utf-8") as handle:
                asked = [re.sub(re.escape(tmp) + r"/[^/ ]+", "$BIN_DIR", line) for line in handle.read().splitlines()]
        return result.stdout.splitlines(), asked, os.listdir(tmp)


class GateReleaseTest(unittest.TestCase):
    def gate(self, tested, go_status=0, audit_status=0):
        with tempfile.TemporaryDirectory() as root:
            return (root, *drive("gate_release", tested, go_status=go_status, audit_status=audit_status, repo_root=root))

    def test_the_gate_builds_the_command_and_asks_it_from_the_repository_root_with_the_version_alone(self):
        # tested_gitlab_version prints the revision beside the version; the
        # command is handed the version GitLab answered and nothing else.
        root, lines, asked, _ = self.gate("19.4.1-ee (revision 0a1b2c3d)")
        self.assertEqual(
            asked,
            [
                f"go ran in {root}",
                "go build -o $BIN_DIR/audit_e2e_coverage ./cmd/audit_e2e_coverage/",
            ],
        )
        self.assertEqual(
            lines,
            [
                f"audit ran in {root}",
                "audit -check-release -gitlab-version 19.4.1-ee",
                "status=0",
            ],
        )

    def test_an_instance_that_did_not_answer_hands_the_command_unknown(self):
        # The script's fallback names a reason after the word; the word is what
        # the command reads, and it does not parse, so the run stops.
        _, lines, _, _ = self.gate("unknown (GET http://localhost:8929/api/v4/version did not answer)")
        self.assertIn("audit -check-release -gitlab-version unknown", lines)

    def test_a_prerelease_is_handed_over_whole(self):
        _, lines, _, _ = self.gate("19.5.0-pre (revision abc)")
        self.assertIn("audit -check-release -gitlab-version 19.5.0-pre", lines)

    def test_the_gate_answers_with_the_status_the_command_exits_with(self):
        # 2 is the one go run would lose: it reports a program that failed as
        # its own status 1, which is the gate's answer for a newer release.
        for audit_status in (0, 1, 2):
            with self.subTest(audit_status=audit_status):
                _, lines, _, _ = self.gate("19.4.1-ee (revision abc)", audit_status=audit_status)
                self.assertEqual(lines[-1], f"status={audit_status}")

    def test_the_built_command_is_removed_whatever_it_answers(self):
        for audit_status in (0, 1, 2):
            with self.subTest(audit_status=audit_status):
                _, _, _, left = self.gate("19.4.1-ee (revision abc)", audit_status=audit_status)
                self.assertEqual(left, [])

    def test_a_command_that_does_not_build_stops_the_run_and_runs_nothing(self):
        _, lines, asked, left = self.gate("19.4.1-ee (revision abc)", go_status=1)
        self.assertEqual(lines, ["status=1"])
        self.assertEqual(len(asked), 2)
        self.assertEqual(left, [])

    def test_a_root_that_is_not_there_fails_the_gate(self):
        with tempfile.TemporaryDirectory() as root:
            lines, asked, left = drive("gate_release", "19.4.1-ee (revision abc)", repo_root=os.path.join(root, "missing"))
        self.assertEqual(asked, [])
        self.assertEqual(lines, ["status=1"])
        self.assertEqual(left, [])


class RunnerWiringTest(unittest.TestCase):
    """The lifecycle script sources the gate, asks it on every run once the
    instance has answered its version and before the tests start, and ends the
    run with the gate's status when it is not zero, leaving the teardown to the
    trap that is already set."""

    def runner(self):
        with open(RUNNER, encoding="utf-8") as handle:
            return handle.read()

    def test_the_gate_is_sourced(self):
        self.assertIn('. "${SCRIPT_DIR}/release-gate.sh"', self.runner())

    def test_the_gate_runs_after_the_version_is_read_and_before_the_tests(self):
        text = self.runner()
        trap = text.index("trap teardown EXIT")
        version = text.index('TESTED_VERSION="$(tested_gitlab_version ')
        gate = text.index('gate_release "${TESTED_VERSION}" || RELEASE_STATUS=$?')
        stopped = text.index('exit "${RELEASE_STATUS}"', gate)
        tests = text.index('"${GOTESTSUM}"')
        self.assertLess(trap, version)
        self.assertLess(version, gate)
        self.assertLess(gate, stopped)
        self.assertLess(stopped, tests)

    def test_the_gate_runs_on_every_run(self):
        # At the top level of the script, not under E2E_GATE_SKIPS or any other
        # switch: a run that is not complete is still a run whose fine-grained
        # sessions would all be in phase A.
        self.assertRegex(self.runner(), re.compile(r'^gate_release "\$\{TESTED_VERSION\}"', re.MULTILINE))


if __name__ == "__main__":
    unittest.main()
