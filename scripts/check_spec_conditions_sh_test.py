#!/usr/bin/env python3
"""Tests scripts/check-spec-conditions.sh, the gate behind `make check-spec-conditions`.

The gate runs gobco, through scripts/coverage-conditions.sh, over every
action_specs.go a branch changes, and fails on a condition no test evaluates
both ways that no declaration answers. A package the recipe will not measure
is reported and does not fail, with one line saying why, and that line is what
these cases hold first: the recipe opens a staged run with a notice, and ends
a run it will not pass on with a verdict of its own (issue 1017), so the first
line of its output is the wrong reason in exactly the cases a reader needs one.

These cases run the real gate in a throwaway repository with the recipe
replaced by a stand-in that prints, per package, the output and the status a
case configures, and a base commit the branch changes each action_specs.go
against. Nothing here runs gobco or go.

Run with:

    python3 -m unittest discover -s scripts -p 'check_spec_conditions_sh_test.py'
"""

import json
import os
import shutil
import subprocess
import sys
import tempfile
import unittest

ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
SCRIPT = os.path.join(ROOT, "scripts", "check-spec-conditions.sh")

# Stands in for scripts/coverage-conditions.sh: prints what STUB_CONDITIONS
# configures for the package it is given and exits with its status.
STUB_RECIPE = r'''import json
import os
import sys

with open(os.environ["STUB_CONDITIONS"], encoding="utf-8") as fh:
    answer = json.load(fh)[sys.argv[1]]
sys.stdout.write(answer["output"])
sys.exit(answer["status"])
'''

# What the recipe prints before a staged run, which is its first line and
# never the reason a run was not measured.
NOTICE = ("gobco: ./internal/tools/{pkg} holds 1 .go file(s) the go command does not build on linux/amd64 "
          "under build tags (none) (x_windows.go), which gobco would parse with the rest (issue 1017); "
          "measuring it in a staged copy of the module without them.\n")

SPECS = "package {pkg}\n\nfunc f(c int) bool {{\n\treturn c == 1\n}}\n"


class CheckSpecConditionsTest(unittest.TestCase):

    def setUp(self):
        self.scratch = tempfile.mkdtemp(prefix="check-spec-conditions-")
        self.addCleanup(shutil.rmtree, self.scratch, True)
        self.repo = os.path.join(self.scratch, "repo")
        os.makedirs(os.path.join(self.repo, "scripts"))
        shutil.copy(SCRIPT, os.path.join(self.repo, "scripts", "check-spec-conditions.sh"))
        stub = os.path.join(self.repo, "scripts", "coverage-conditions.sh")
        with open(stub, "w", encoding="utf-8") as fh:
            fh.write("#!" + sys.executable + " -S\n" + STUB_RECIPE)
        os.chmod(stub, 0o755)
        self.conditions = os.path.join(self.scratch, "conditions.json")
        # The fixture repository reads no configuration of the person running
        # the tests, so a signing key or a hook of theirs changes nothing.
        self.env = {k: v for k, v in os.environ.items() if not k.startswith(("GIT_", "STUB_"))}
        self.env.update({
            "GIT_CONFIG_GLOBAL": os.devnull,
            "GIT_CONFIG_NOSYSTEM": "1",
            "GIT_AUTHOR_NAME": "fixture",
            "GIT_AUTHOR_EMAIL": "fixture@example.com",
            "GIT_COMMITTER_NAME": "fixture",
            "GIT_COMMITTER_EMAIL": "fixture@example.com",
            "STUB_CONDITIONS": self.conditions,
        })
        self.git("init", "-q", "-b", "main")

    def git(self, *args):
        return subprocess.run(["git", *args], cwd=self.repo, env=self.env, capture_output=True, text=True,
                              check=True).stdout.strip()

    def commit_specs(self, packages, message):
        for pkg, text in packages.items():
            path = os.path.join(self.repo, "internal", "tools", pkg, "action_specs.go")
            os.makedirs(os.path.dirname(path), exist_ok=True)
            with open(path, "w", encoding="utf-8") as fh:
                fh.write(text)
        self.git("add", "-A")
        self.git("commit", "-q", "-m", message)
        return self.git("rev-parse", "HEAD")

    def run_gate(self, answers):
        """Changes one action_specs.go per package the answers name, on a
        branch off a base that holds them unchanged, and runs the gate with
        the recipe answering each as configured."""
        base = self.commit_specs({pkg: SPECS.format(pkg=pkg) for pkg in answers}, "base")
        self.commit_specs({pkg: SPECS.format(pkg=pkg) + "\n// changed\n" for pkg in answers}, "change")
        with open(self.conditions, "w", encoding="utf-8") as fh:
            json.dump({"./internal/tools/" + pkg: answer for pkg, answer in answers.items()}, fh)
        return subprocess.run([os.path.join(self.repo, "scripts", "check-spec-conditions.sh"), base],
                              cwd=self.repo, env=self.env, capture_output=True, text=True, timeout=60,
                              check=False)

    def test_a_package_not_measured_is_reported_with_the_recipes_reason(self):
        # The reason is the recipe's own verdict, whichever it gave, or the
        # line gobco panicked with, and never the staging notice above it.
        cases = [
            ("gobco panicked", NOTICE + "panic: x_unix.go:5:6: f redeclared in this block\n"
             "gobco: gobco exited 2 on the staged copy of ./internal/tools/{pkg}, so any figure above is not a "
             "measurement: its go test failed there, or it could not instrument the copy\n", 2,
             "panic: x_unix.go:5:6: f redeclared in this block"),
            ("a staged run failed", NOTICE + "--- FAIL: TestHeaders (0.00s)\n\nCondition coverage: 3/4\n"
             "gobco: gobco exited 1 on the staged copy of ./internal/tools/{pkg}, so any figure above is not a "
             "measurement: its go test failed there, or it could not instrument the copy\n"
             "gobco: the copy blanked the build constraint lines of x.go, so a test that reads its own files' "
             "headers finds them blank there and fails\n", 1,
             "gobco: gobco exited 1 on the staged copy of ./internal/tools/{pkg}, so any figure above is not a "
             "measurement"),
            ("a run where it is failed", "--- FAIL: TestX (0.00s)\n\nCondition coverage: 3/4\n"
             "gobco: gobco exited 1 on ./internal/tools/{pkg}, so any figure above is not a measurement: its go "
             "test failed, or it could not instrument the package\n", 1,
             "gobco: gobco exited 1 on ./internal/tools/{pkg}, so any figure above is not a measurement"),
            ("no condition measured", NOTICE + "\nCondition coverage: 0/0\n"
             "gobco: the report above measured no condition of ./internal/tools/{pkg} (Condition coverage: 0/0, "
             "or no figure at all), which is what gobco prints when no test wrote its counts or when it declined "
             "to instrument every file; refusing to pass it on as a measurement\n", 1,
             "refusing to pass it on as a measurement"),
            ("refused before gobco ran", "gobco: ./internal/tools/{pkg} keeps no test file on linux/amd64 under "
             "build tags (none): its tests are among the files the go command leaves out here (x_test.go), so no "
             "test would run and gobco would measure nothing; refusing to measure\n"
             "gobco: a package behind a build tag is measured with TAGS=<tag>, which reaches go list and gobco's "
             "go test alike\n", 1, "keeps no test file"),
            # A recipe that said nothing a reader could pick out still gets a
            # reason: its first line.
            ("no verdict at all", "something went wrong\nand kept going\n", 1, "something went wrong"),
        ]
        for name, output, status, reason in cases:
            with self.subTest(name):
                self.setUp()
                proc = self.run_gate({"alpha": {"output": output.format(pkg="alpha"), "status": status}})
                self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
                lines = [line for line in proc.stdout.splitlines() if "not measured" in line]
                self.assertEqual(len(lines), 1, proc.stdout)
                self.assertIn(reason.format(pkg="alpha"), lines[0])
                self.assertNotIn("holds 1 .go file(s)", lines[0])
                self.assertIn("every measured condition is evaluated both ways", proc.stdout)

    def test_a_condition_no_test_decides_fails_unless_declared(self):
        undecided = 'action_specs.go:4:9: condition "c == 1" was once true but never false\n'
        cases = [
            ("undeclared", SPECS, 1),
            ("declared", SPECS.replace("return c == 1", "return c == 1 // gobco: always true here"), 0),
        ]
        for name, text, status in cases:
            with self.subTest(name):
                self.setUp()
                base = self.commit_specs({"beta": text}, "base")
                self.commit_specs({"beta": text + "\n// changed\n"}, "change")
                with open(self.conditions, "w", encoding="utf-8") as fh:
                    json.dump({"./internal/tools/beta": {"output": NOTICE.format(pkg="beta")
                                                        + "\nCondition coverage: 3/4\n" + undecided,
                                                        "status": 0}}, fh)
                proc = subprocess.run([os.path.join(self.repo, "scripts", "check-spec-conditions.sh"), base],
                                      cwd=self.repo, env=self.env, capture_output=True, text=True, timeout=60,
                                      check=False)
                self.assertEqual(proc.returncode, status, proc.stdout + proc.stderr)
                self.assertIn("Condition coverage: 3/4", proc.stdout)
                self.assertEqual("internal/tools/beta/" + undecided.strip() in proc.stderr, status == 1,
                                 proc.stderr)

    def test_script_passes_shellcheck(self):
        shellcheck = shutil.which("shellcheck")
        if shellcheck is None:
            # A skip keeps the job green, so on a runner without shellcheck
            # this check would stop running and nobody would be told.
            # GitHub sets CI on every step; there a missing tool is a failure.
            if os.environ.get("CI"):
                self.fail("shellcheck is not installed, and CI must run this check rather than skip it")
            self.skipTest("shellcheck is not installed")
        result = subprocess.run([shellcheck, SCRIPT], capture_output=True, timeout=60, check=False)
        self.assertEqual(result.returncode, 0, result.stdout.decode() + result.stderr.decode())


if __name__ == "__main__":
    unittest.main()
