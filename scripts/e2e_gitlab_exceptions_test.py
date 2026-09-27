#!/usr/bin/env python3
"""Tests how a Docker e2e run keeps what GitLab logged as an exception.

A scenario that meets a 500 learns only that GitLab raised something: the
exception is written inside the container, on a volume the teardown of
test/e2e/scripts/run-docker-e2e.sh deletes. The saved view create answered
that way on every surface of both complete runs of de1ab3b49, and nothing was
left to say which exception it was. The teardown now copies GitLab's
exceptions log into the reports first; the copy lives in
test/e2e/scripts/gitlab-exceptions.sh so that these tests can drive it with a
stand-in docker, without a daemon.

Run with:

    python3 -m unittest discover -s scripts -p 'e2e_gitlab_exceptions_test.py'
"""

import os
import subprocess
import tempfile
import unittest

ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
SCRIPT = os.path.join(ROOT, "test", "e2e", "scripts", "gitlab-exceptions.sh")
RUNNER = os.path.join(ROOT, "test", "e2e", "scripts", "run-docker-e2e.sh")

LOG = "/var/log/gitlab/gitlab-rails/exceptions_json.log"

# Two lines of the shape GitLab writes, one exception each.
EXCEPTIONS = (
    '{"severity":"ERROR","exception.class":"ArgumentError","exception.message":"\'x\' is not a valid sort"}\n'
    '{"severity":"ERROR","exception.class":"NoMethodError","exception.message":"undefined method"}\n'
)

# The stand-in docker records every call in $STATE/calls and answers the
# exec with $EXCEPTIONS, or fails it when EXEC_FAILS is set. The driver sources
# the script under the options run-docker-e2e.sh runs with, runs the command
# it is handed, and prints its exit status last.
DRIVER = r"""
set -euo pipefail
. "$1"
shift
docker() {
    printf '%s\n' "docker $*" >> "${STATE}/calls"
    [ -z "${EXEC_FAILS}" ] || return 1
    printf '%s' "${EXCEPTIONS}"
}
status=0
"$@" || status=$?
echo "status=${status}"
"""


def drive(destination, state, exec_fails=False):
    """Saves the log to destination through the stand-in compose command."""
    env = dict(os.environ)
    env["STATE"] = state
    env["EXCEPTIONS"] = EXCEPTIONS
    env["EXEC_FAILS"] = "yes" if exec_fails else ""
    result = subprocess.run(
        ["bash", "-c", DRIVER, "driver", SCRIPT, "save_gitlab_exceptions", destination, "docker", "compose", "-f", "stack.yml"],
        env=env,
        capture_output=True,
        text=True,
        check=True,
    )
    calls_path = os.path.join(state, "calls")
    calls = []
    if os.path.exists(calls_path):
        with open(calls_path, encoding="utf-8") as handle:
            calls = handle.read().splitlines()
    lines = result.stdout.splitlines()
    return {"stdout": lines[:-1], "status": lines[-1], "stderr": result.stderr, "calls": calls}


class SaveGitLabExceptionsTest(unittest.TestCase):
    def test_the_log_is_saved_as_gitlab_wrote_it(self):
        with tempfile.TemporaryDirectory() as state:
            destination = os.path.join(state, "reports", "nested", "e2e-ce-gitlab-exceptions.json")
            got = drive(destination, state)
            with open(destination, encoding="utf-8") as handle:
                saved = handle.read()
            leftovers = os.path.exists(destination + ".part")
        self.assertEqual(saved, EXCEPTIONS)
        self.assertFalse(leftovers)
        self.assertEqual(got["status"], "status=0")
        self.assertEqual(got["stdout"], [f"    GitLab's exceptions log: 2 entries, saved to {destination}"])
        self.assertEqual(got["calls"], [f"docker compose -f stack.yml exec -T gitlab cat {LOG}"])

    def test_a_log_that_cannot_be_read_warns_leaves_nothing_and_never_fails_the_run(self):
        with tempfile.TemporaryDirectory() as state:
            destination = os.path.join(state, "e2e-ee-gitlab-exceptions.json")
            # A file an earlier run left under the name would read as this
            # run's, so it goes whether or not the copy succeeds.
            with open(destination, "w", encoding="utf-8") as handle:
                handle.write("an earlier run's log\n")
            got = drive(destination, state, exec_fails=True)
            present = os.path.exists(destination), os.path.exists(destination + ".part")
        self.assertEqual(present, (False, False))
        self.assertEqual(got["status"], "status=0")
        self.assertEqual(got["stdout"], [])
        self.assertEqual(got["stderr"], "WARN: could not read GitLab's exceptions log from the gitlab service; none saved\n")

    def test_a_destination_that_cannot_be_made_asks_nothing(self):
        with tempfile.TemporaryDirectory() as state:
            blocker = os.path.join(state, "reports")
            with open(blocker, "w", encoding="utf-8") as handle:
                handle.write("a file where the directory would go\n")
            got = drive(os.path.join(blocker, "e2e-ce-gitlab-exceptions.json"), state)
        self.assertEqual(got["status"], "status=0")
        self.assertEqual(got["calls"], [])
        self.assertIn("none saved", got["stderr"])


class RunnerWiringTest(unittest.TestCase):
    """The teardown saves the log before the down that deletes it, into the
    reports under the run's own name."""

    def test_the_log_is_saved_before_the_stack_goes(self):
        with open(RUNNER, encoding="utf-8") as handle:
            text = handle.read()
        self.assertIn('. "${SCRIPT_DIR}/gitlab-exceptions.sh"', text)
        teardown = text.index("teardown() {")
        saved = text.index(
            'save_gitlab_exceptions "${E2E_REPORT_DIR}/${E2E_REPORT_NAME}-gitlab-exceptions.json" "${COMPOSE[@]}"',
            teardown,
        )
        kept = text.index('if [ "${E2E_KEEP_STACK:-false}" = "true" ]; then', teardown)
        down = text.index('"${DOWN[@]}" || teardown_status=$?', teardown)
        self.assertLess(saved, kept)
        self.assertLess(saved, down)


if __name__ == "__main__":
    unittest.main()
