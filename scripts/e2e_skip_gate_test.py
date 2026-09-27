#!/usr/bin/env python3
"""Tests how a complete Docker e2e run reaches the skip gate.

A complete run, make test-e2e-ce or make test-e2e-ee, fails on a skip nobody
declared: test/e2e/scripts/run-docker-e2e.sh hands the go test -json stream
it just wrote to cmd/audit_e2e_coverage -check-skips. What the gate judges is
tested with the command, against the streams of two real runs; what is tested
here is the part in the shell, which lives in test/e2e/scripts/skip-gate.sh so
that it can be driven with a stand-in go: which command runs, from where, that
its verdict lands in the saved output beside the tests' own, what it does to
the status the run ends with, and what a run asking to be complete is started
with, or refused for, before anything starts.

Run with:

    python3 -m unittest discover -s scripts -p 'e2e_skip_gate_test.py'
"""

import os
import subprocess
import tempfile
import unittest

# The variables the functions read, cleared before every case so the
# developer's own environment is never part of one.
CLEARED = ("E2E_EXTERNAL_NETWORK", "E2E_GATE_SKIPS", "E2E_BITBUCKET", "E2E_ENV_FILE")

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


def drive(*command, go_status=0, repo_root=None, **settings):
    """Runs one function of the script and returns its stdout lines."""
    env = {key: value for key, value in os.environ.items() if key not in CLEARED}
    env["GO_STATUS"] = str(go_status)
    if repo_root is not None:
        env["REPO_ROOT"] = repo_root
    env.update(settings)
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


def write_env_file(directory, name, text):
    """Writes a dotenv file into the directory and returns its path."""
    path = os.path.join(directory, name)
    with open(path, "w", encoding="utf-8") as handle:
        handle.write(text)
    return path


class CompleteRunExternalNetworkTest(unittest.TestCase):
    """What a complete run hands the harness as E2E_EXTERNAL_NETWORK: the
    caller's value wherever the harness would have read it, in its order, and
    true when the caller gave none."""

    def network(self, *files, **settings):
        lines = drive("complete_run_external_network", *files, **settings)
        self.assertEqual(lines[-1], "status=0")
        return lines[:-1]

    def test_no_value_anywhere_is_true(self):
        with tempfile.TemporaryDirectory() as root:
            env_file = write_env_file(root, ".env", "GH_TOKEN=x\n")
            self.assertEqual(self.network("", env_file, os.path.join(root, "missing")), ["true"])
        self.assertEqual(self.network(), ["true"])

    def test_the_environment_wins_over_every_file(self):
        with tempfile.TemporaryDirectory() as root:
            env_file = write_env_file(root, ".env", "E2E_EXTERNAL_NETWORK=true\n")
            self.assertEqual(self.network(env_file, E2E_EXTERNAL_NETWORK="false"), ["false"])

    def test_a_false_written_in_env_is_honoured(self):
        # The file .env.example points a user to for this key: the default must
        # not outrank it, which is what exporting true unconditionally did.
        with tempfile.TemporaryDirectory() as root:
            env_file = write_env_file(root, ".env", 'GH_TOKEN=x\nE2E_EXTERNAL_NETWORK="false"\n')
            self.assertEqual(self.network("", env_file), ["false"])

    def test_the_named_file_is_asked_before_the_repository_env(self):
        with tempfile.TemporaryDirectory() as root:
            named = write_env_file(root, "named.env", "E2E_EXTERNAL_NETWORK=no\n")
            repo_env = write_env_file(root, ".env", "E2E_EXTERNAL_NETWORK=true\n")
            self.assertEqual(self.network(named, repo_env), ["no"])
            self.assertEqual(self.network(repo_env, named), ["true"])

    def test_an_empty_value_gives_none(self):
        with tempfile.TemporaryDirectory() as root:
            empty = write_env_file(root, "named.env", "E2E_EXTERNAL_NETWORK=\n")
            repo_env = write_env_file(root, ".env", "E2E_EXTERNAL_NETWORK=false\n")
            self.assertEqual(self.network(empty, repo_env, E2E_EXTERNAL_NETWORK=""), ["false"])
            self.assertEqual(self.network(empty), ["true"])

    def test_a_file_the_shell_cannot_read_through_gives_none(self):
        # run-docker-e2e.sh runs under set -u, which the subshell reading the
        # file inherits: a reference to a variable nobody set ends the read
        # before the key, and the next file is asked.
        with tempfile.TemporaryDirectory() as root:
            broken = write_env_file(root, "named.env", "A=$NOT_SET_ANYWHERE_E2E\nE2E_EXTERNAL_NETWORK=false\n")
            self.assertEqual(self.network(broken), ["true"])


class CompleteRunRefusalTest(unittest.TestCase):
    """Why a run asked to be complete cannot be, one reason per line."""

    def refusal(self, network, bitbucket):
        lines = drive("complete_run_refusal", network, bitbucket)
        self.assertEqual(lines[-1], "status=0")
        return lines[:-1]

    def test_a_run_that_can_be_complete_is_refused_nothing(self):
        # The harness compares the switch with true whatever its case.
        for network in ("true", "TRUE", "True"):
            with self.subTest(network=network):
                self.assertEqual(self.refusal(network, "true"), [])

    def test_no_external_network_is_refused(self):
        for network in ("false", "yes", "1", ""):
            with self.subTest(network=network):
                self.assertEqual(
                    self.refusal(network, "true"),
                    [
                        f"E2E_EXTERNAL_NETWORK is '{network}', so the GitHub, Gists and Bitbucket Cloud importers, "
                        "which call a public URL, would skip"
                    ],
                )

    def test_no_bitbucket_fixture_is_refused(self):
        # run-docker-e2e.sh starts the fixture on the exact word true.
        for bitbucket in ("false", "TRUE"):
            with self.subTest(bitbucket=bitbucket):
                self.assertEqual(
                    self.refusal("true", bitbucket),
                    [
                        f"E2E_BITBUCKET is '{bitbucket}', so the Bitbucket fixture would not start and the "
                        "Bitbucket Server import would skip"
                    ],
                )

    def test_both_reasons_are_given(self):
        self.assertEqual(len(self.refusal("false", "false")), 2)


class RunnerWiringTest(unittest.TestCase):
    """The lifecycle script sources the gate and calls it after the tests,
    only when E2E_GATE_SKIPS asks, and folds the gate's status in; and a run
    asking to be complete is refused before anything starts when it cannot
    be."""

    def test_the_gate_runs_after_the_tests_and_only_when_asked(self):
        with open(RUNNER, encoding="utf-8") as handle:
            text = handle.read()
        self.assertIn('. "${SCRIPT_DIR}/skip-gate.sh"', text)
        tests = text.index('RUN_STATUS="${PIPESTATUS[0]}"')
        asked = text.index('if [ "${E2E_GATE_SKIPS:-false}" = "true" ]; then', tests)
        gate = text.index("gate_skips \"${RUNTIME}\"")
        folded = text.index('RUN_STATUS="$(run_status_after_gate "${RUN_STATUS}" "${GATE_STATUS}")"')
        self.assertLess(tests, asked)
        self.assertLess(asked, gate)
        self.assertLess(gate, folded)

    def test_the_complete_run_is_settled_before_anything_starts(self):
        with open(RUNNER, encoding="utf-8") as handle:
            text = handle.read()
        settled = text.index('E2E_EXTERNAL_NETWORK="$(complete_run_external_network "${E2E_ENV_FILE:-}" "${REPO_ROOT}/.env")"')
        exported = text.index("export E2E_EXTERNAL_NETWORK", settled)
        refused = text.index('complete_run_refusal "${E2E_EXTERNAL_NETWORK}" "${E2E_BITBUCKET:-true}"', exported)
        self.assertLess(refused, text.index("trap teardown EXIT"))

    def refused_run(self, **settings):
        """Runs the lifecycle script as a complete run with a stand-in
        docker that records being called, and returns the result and whether
        docker was."""
        with tempfile.TemporaryDirectory() as root:
            bin_dir = os.path.join(root, "bin")
            os.mkdir(bin_dir)
            marker = os.path.join(root, "docker-called")
            docker = os.path.join(bin_dir, "docker")
            with open(docker, "w", encoding="utf-8") as handle:
                handle.write(f'#!/bin/sh\n: > "{marker}"\n')
            os.chmod(docker, 0o755)
            env = {key: value for key, value in os.environ.items() if key not in CLEARED}
            env["PATH"] = bin_dir + os.pathsep + env.get("PATH", "")
            env["E2E_GATE_SKIPS"] = "true"
            env["E2E_REPORT_DIR"] = os.path.join(root, "reports")
            env.update(settings)
            result = subprocess.run(
                ["bash", RUNNER, "ce", "--", "./test/e2e/gitlab/common/"],
                env=env,
                capture_output=True,
                text=True,
                timeout=60,
            )
            return result, os.path.exists(marker)

    def test_a_complete_run_without_the_external_network_is_refused_before_docker(self):
        result, docker_called = self.refused_run(E2E_EXTERNAL_NETWORK="false")
        self.assertEqual(result.returncode, 2, result.stderr)
        self.assertFalse(docker_called)
        self.assertIn("ERROR: this run was asked to be complete (E2E_GATE_SKIPS=true) and cannot be:", result.stderr)
        self.assertIn("  E2E_EXTERNAL_NETWORK is 'false'", result.stderr)
        self.assertIn("E2E_GATE_SKIPS=false, which is not a complete run", result.stderr)

    def test_a_complete_run_told_so_in_the_named_file_is_refused_before_docker(self):
        with tempfile.TemporaryDirectory() as root:
            named = write_env_file(root, "named.env", "E2E_EXTERNAL_NETWORK=false\n")
            result, docker_called = self.refused_run(E2E_ENV_FILE=named)
        self.assertEqual(result.returncode, 2, result.stderr)
        self.assertFalse(docker_called)
        self.assertIn("  E2E_EXTERNAL_NETWORK is 'false'", result.stderr)

    def test_a_complete_run_without_bitbucket_is_refused_before_docker(self):
        result, docker_called = self.refused_run(E2E_EXTERNAL_NETWORK="true", E2E_BITBUCKET="false")
        self.assertEqual(result.returncode, 2, result.stderr)
        self.assertFalse(docker_called)
        self.assertIn("  E2E_BITBUCKET is 'false'", result.stderr)
        self.assertNotIn("E2E_EXTERNAL_NETWORK is", result.stderr)

    def test_the_complete_run_targets_ask_for_it_and_leave_the_network_to_the_script(self):
        with open(os.path.join(ROOT, "Makefile"), encoding="utf-8") as handle:
            makefile = handle.read()
        self.assertIn('e2e_complete_run_env = E2E_GATE_SKIPS="$${E2E_GATE_SKIPS:-true}"\n', makefile)
        for target in ("test-e2e-ce:", "test-e2e-ee:"):
            with self.subTest(target=target):
                recipe = makefile[makefile.index("\n" + target) :]
                recipe = recipe[: recipe.index("\n\n")]
                self.assertIn("\t$(e2e_complete_run_env) \\\n", recipe)
                # An assignment here would put a value into the process
                # environment, which outranks the .env a caller wrote false in.
                self.assertNotIn("E2E_EXTERNAL_NETWORK", recipe)


if __name__ == "__main__":
    unittest.main()
