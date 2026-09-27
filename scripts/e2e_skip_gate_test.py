#!/usr/bin/env python3
"""Tests how a complete Docker e2e run reaches the skip gate.

A complete run, make test-e2e-ce or make test-e2e-ee, fails on a skip nobody
declared: test/e2e/scripts/run-docker-e2e.sh hands the go test -json stream
it just wrote to cmd/audit_e2e_coverage -check-skips. What the gate judges is
tested with the command, against the streams of two real runs; what is tested
here is the part in the shell, which lives in test/e2e/scripts/skip-gate.sh so
that it can be driven with a stand-in go: which command runs, from where, that
its verdict lands in the saved output beside the tests' own, and what it does
to the status the run ends with.

Run with:

    python3 -m unittest discover -s scripts -p 'e2e_skip_gate_test.py'
"""

import os
import subprocess
import tempfile
import unittest

ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
SCRIPT = os.path.join(ROOT, "test", "e2e", "scripts", "skip-gate.sh")
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
    echo "a finding on stderr" >&2
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


class GateSkipsTest(unittest.TestCase):
    def gate(self, go_status):
        with tempfile.TemporaryDirectory() as root:
            output = os.path.join(root, "e2e-ce-output.txt")
            with open(output, "w", encoding="utf-8") as handle:
                handle.write("the tests' own output\n")
            lines = drive("gate_skips", "ce", "/reports/e2e-ce-log.json", output, go_status=go_status, repo_root=root)
            with open(output, encoding="utf-8") as handle:
                saved = handle.read()
            return root, lines, saved

    def test_the_gate_runs_the_command_from_the_repository_root(self):
        root, lines, _ = self.gate(0)
        self.assertEqual(
            lines,
            [
                f"go ran in {root}",
                "go run ./cmd/audit_e2e_coverage/ -check-skips -runtime ce -results /reports/e2e-ce-log.json",
                "a finding on stderr",
                "status=0",
            ],
        )

    def test_the_verdict_is_appended_to_the_saved_output(self):
        root, _, saved = self.gate(1)
        self.assertEqual(
            saved,
            "the tests' own output\n"
            f"go ran in {root}\n"
            "go run ./cmd/audit_e2e_coverage/ -check-skips -runtime ce -results /reports/e2e-ce-log.json\n"
            "a finding on stderr\n",
        )

    def test_the_gate_answers_with_the_commands_status(self):
        for go_status in (0, 1, 2):
            with self.subTest(go_status=go_status):
                _, lines, _ = self.gate(go_status)
                self.assertEqual(lines[-1], f"status={go_status}")

    def test_a_root_that_is_not_there_fails_the_gate(self):
        with tempfile.TemporaryDirectory() as root:
            output = os.path.join(root, "out.txt")
            lines = drive("gate_skips", "ee", "/r.json", output, repo_root=os.path.join(root, "missing"))
        self.assertNotIn("go run", "\n".join(lines))
        self.assertEqual(lines[-1], "status=1")


class RunStatusAfterGateTest(unittest.TestCase):
    def test_the_tests_status_wins_when_they_failed(self):
        for run, gate, want in (("0", "0", "0"), ("0", "1", "1"), ("0", "2", "2"), ("1", "0", "1"), ("1", "1", "1"), ("2", "1", "2")):
            with self.subTest(run=run, gate=gate):
                lines = drive("run_status_after_gate", run, gate)
                self.assertEqual(lines, [want, "status=0"])


class RunnerWiringTest(unittest.TestCase):
    """The lifecycle script sources the gate and calls it after the tests,
    only when E2E_GATE_SKIPS asks, and folds the gate's status in."""

    def test_the_gate_runs_after_the_tests_and_only_when_asked(self):
        with open(RUNNER, encoding="utf-8") as handle:
            text = handle.read()
        self.assertIn('. "${SCRIPT_DIR}/skip-gate.sh"', text)
        tests = text.index('RUN_STATUS="${PIPESTATUS[0]}"')
        asked = text.index('if [ "${E2E_GATE_SKIPS:-false}" = "true" ]; then')
        gate = text.index("gate_skips \"${RUNTIME}\"")
        folded = text.index('RUN_STATUS="$(run_status_after_gate "${RUN_STATUS}" "${GATE_STATUS}")"')
        self.assertLess(tests, asked)
        self.assertLess(asked, gate)
        self.assertLess(gate, folded)

    def test_the_complete_run_targets_ask_for_it(self):
        with open(os.path.join(ROOT, "Makefile"), encoding="utf-8") as handle:
            makefile = handle.read()
        self.assertIn('e2e_complete_run_env = E2E_EXTERNAL_NETWORK="$${E2E_EXTERNAL_NETWORK:-true}" E2E_GATE_SKIPS=true', makefile)
        for target in ("test-e2e-ce:", "test-e2e-ee:"):
            with self.subTest(target=target):
                recipe = makefile[makefile.index("\n" + target) :]
                recipe = recipe[: recipe.index("\n\n")]
                self.assertIn("\t$(e2e_complete_run_env) \\\n", recipe)


if __name__ == "__main__":
    unittest.main()
