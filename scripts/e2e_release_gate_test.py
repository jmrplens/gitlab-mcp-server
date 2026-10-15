#!/usr/bin/env python3
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

# The stand-in go prints where it ran and what it was asked, one line each,
# and exits with GO_STATUS. The driver sources the script under the options
# run-docker-e2e.sh runs with, calls the function the way that script does,
# and prints the status last.
DRIVER = r"""
set -euo pipefail
. "$1"
shift
go() {
    echo "go ran in ${PWD}"
    echo "go $*"
    return "${GO_STATUS}"
}
status=0
"$@" || status=$?
echo "status=${status}"
"""


def drive(*command, go_status=0, repo_root=None):
    """Runs one function of the script and returns its stdout lines."""
    env = dict(os.environ)
    env["GO_STATUS"] = str(go_status)
    if repo_root is not None:
        env["REPO_ROOT"] = repo_root
    result = subprocess.run(
        ["bash", "-c", DRIVER, "driver", SCRIPT, *command],
        env=env,
        capture_output=True,
        text=True,
        check=True,
    )
    return result.stdout.splitlines()


class GateReleaseTest(unittest.TestCase):
    def gate(self, tested, go_status=0):
        with tempfile.TemporaryDirectory() as root:
            return root, drive("gate_release", tested, go_status=go_status, repo_root=root)

    def test_the_gate_asks_the_command_from_the_repository_root_with_the_version_alone(self):
        # tested_gitlab_version prints the revision beside the version; the
        # command is handed the version GitLab answered and nothing else.
        root, lines = self.gate("19.4.1-ee (revision 0a1b2c3d)")
        self.assertEqual(
            lines,
            [
                f"go ran in {root}",
                "go run ./cmd/audit_e2e_coverage/ -check-release -gitlab-version 19.4.1-ee",
                "status=0",
            ],
        )

    def test_an_instance_that_did_not_answer_hands_the_command_unknown(self):
        # The script's fallback names a reason after the word; the word is what
        # the command reads, and it does not parse, so the run stops.
        _, lines = self.gate("unknown (GET http://localhost:8929/api/v4/version did not answer)")
        self.assertIn("go run ./cmd/audit_e2e_coverage/ -check-release -gitlab-version unknown", lines)

    def test_a_prerelease_is_handed_over_whole(self):
        _, lines = self.gate("19.5.0-pre (revision abc)")
        self.assertIn("go run ./cmd/audit_e2e_coverage/ -check-release -gitlab-version 19.5.0-pre", lines)

    def test_the_gate_answers_with_the_commands_status(self):
        for go_status in (0, 1, 2):
            with self.subTest(go_status=go_status):
                _, lines = self.gate("19.4.1-ee (revision abc)", go_status=go_status)
                self.assertEqual(lines[-1], f"status={go_status}")

    def test_a_root_that_is_not_there_fails_the_gate(self):
        with tempfile.TemporaryDirectory() as root:
            lines = drive("gate_release", "19.4.1-ee (revision abc)", repo_root=os.path.join(root, "missing"))
        self.assertNotIn("go run", "\n".join(lines))
        self.assertEqual(lines[-1], "status=1")


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
