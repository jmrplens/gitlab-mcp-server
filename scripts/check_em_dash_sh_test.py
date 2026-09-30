#!/usr/bin/env python3
"""Tests the description half of scripts/check-em-dash.sh (`make check-pr-description`).

This repository squash merges, so a pull request's title and body become the
commit message on main. The gate refuses three things in them: an em dash, a
block a review bot injected, and a command that makes GitHub skip workflows.
The third is the newest. GitHub skips every workflow a push triggers when the
commit it lands on carries one of those commands anywhere in its message, so a
description that merely explains a fixture committing with one turned off CI on
main for its merge, and the release for the tag put on that commit (7baa4a40b,
the merge of pull request 1094, and v3.1.0).

Most cases run the real script over a title and a body written to disk, which
is the rehearsal path the script offers (PR_TITLE_FILE and PR_BODY_FILE). One
drives the path CI takes, PR_NUMBER through `gh api` and `jq`, against a
stand-in `gh` that answers with a body carrying Windows line endings, as the
API returns a description edited in the browser.

Run with:

    python3 -m unittest discover -s scripts -p 'check_em_dash_sh_test.py'
"""

import os
import shutil
import stat
import subprocess
import tempfile
import unittest

ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
SCRIPT = os.path.join(ROOT, "scripts", "check-em-dash.sh")

# Built from its code point so this file does not carry the character the diff
# half of the same gate refuses.
EM_DASH = chr(0x2014)

# The five commands GitHub documents for skipping workflows on push and
# pull_request events.
SKIP_COMMANDS = ["[skip ci]", "[ci skip]", "[no ci]", "[skip actions]", "[actions skip]"]

SKIP_FAILURE = "carries a command that makes GitHub skip"


def require_tool(test, name):
    """Returns the path of a tool, failing under CI and skipping elsewhere when it is missing."""
    path = shutil.which(name)
    if path is None:
        # A skip keeps the job green, so on a runner without the tool this
        # check would stop running and nobody would be told. GitHub sets CI on
        # every step; there a missing tool is a failure.
        if os.environ.get("CI"):
            test.fail(name + " is not installed, and CI must run this check rather than skip it")
        test.skipTest(name + " is not installed")
    return path


def judge(title, body):
    """Runs the description gate over title and body from disk, returning (status, output)."""
    with tempfile.TemporaryDirectory() as tmp:
        title_file = os.path.join(tmp, "title")
        body_file = os.path.join(tmp, "body")
        with open(title_file, "w", encoding="utf-8") as fh:
            fh.write(title)
        with open(body_file, "w", encoding="utf-8") as fh:
            fh.write(body)
        env = dict(os.environ, PR_TITLE_FILE=title_file, PR_BODY_FILE=body_file)
        env.pop("PR_NUMBER", None)
        run = subprocess.run(
            ["bash", SCRIPT, "description"],
            env=env,
            capture_output=True,
            text=True,
            check=False,
        )
    return run.returncode, run.stdout + run.stderr


class CheckEmDashDescriptionTest(unittest.TestCase):
    def test_clean_description_passes(self):
        status, output = judge(
            "Refuse a skip command in a pull request description",
            "Plain prose, no em dash, no generated block.\n",
        )
        self.assertEqual(status, 0, output)
        self.assertIn("OK: the title and body carry no em dash, no injected block and no skip command.", output)

    def test_each_skip_command_in_the_body_fails_and_names_its_line(self):
        for command in SKIP_COMMANDS:
            with self.subTest(command=command):
                status, output = judge("A title", "first line\nthe fixture commits with " + command + "\n")
                self.assertEqual(status, 1, output)
                self.assertIn("body:2:the fixture commits with " + command, output)
                self.assertIn(SKIP_FAILURE, output)

    def test_skip_command_in_the_title_fails_in_the_em_dash_check_format(self):
        status, output = judge("Commit fixtures with [skip ci]", "body\n")
        self.assertEqual(status, 1, output)
        self.assertIn("title: Commit fixtures with [skip ci]", output)
        self.assertIn(SKIP_FAILURE, output)

    def test_skip_command_is_matched_in_any_case_spacing_and_inside_backticks(self):
        for text in ["commits with `[skip ci]`", "commits with [Skip CI]", "commits with [NO CI]",
                     "commits with [skip  ci]", "commits with [actions\tskip]", "commits with [ skip ci ]"]:
            with self.subTest(text=text):
                status, output = judge("A title", text + "\n")
                self.assertEqual(status, 1, output)
                self.assertIn("body:1:" + text, output)
                self.assertIn(SKIP_FAILURE, output)

    def test_command_named_without_its_brackets_passes(self):
        status, output = judge("A title", "the fixture commits with skip ci in square brackets\n")
        self.assertEqual(status, 0, output)

    def test_skip_checks_trailer_at_the_start_of_a_line_fails(self):
        cases = {
            "no space": ("body\n\n\nskip-checks:true\n", "body:4:"),
            "one space": ("body\n\n\nskip-checks: true\n", "body:4:"),
            "any case": ("body\n\n\nSkip-Checks: TRUE\n", "body:4:"),
            "in backticks": ("body\n`skip-checks: true`\n", "body:2:"),
            "before another trailer": ("body\n\nskip-checks: true\nCo-authored-by: A <a@example.com>\n", "body:3:"),
        }
        for name, (body, location) in cases.items():
            with self.subTest(name=name):
                status, output = judge("A title", body)
                self.assertEqual(status, 1, output)
                self.assertIn(location, output)
                self.assertIn(SKIP_FAILURE, output)

    def test_skip_checks_named_inside_a_sentence_passes(self):
        status, output = judge("A title", "GitHub also reads a skip-checks: true trailer at the end.\n")
        self.assertEqual(status, 0, output)

    def test_skip_command_beside_a_byte_invalid_in_utf8_is_still_judged(self):
        # grep would call the input binary and print nothing on stdout, so the
        # gate would pass a line it never read; -a keeps it text.
        with tempfile.TemporaryDirectory() as tmp:
            body_file = os.path.join(tmp, "body")
            with open(body_file, "wb") as fh:
                fh.write(b"x\ncaf\xe9 commits with [skip ci]\n")
            env = dict(os.environ, PR_BODY_FILE=body_file, LC_ALL="C.UTF-8")
            env.pop("PR_TITLE_FILE", None)
            env.pop("PR_NUMBER", None)
            run = subprocess.run(["bash", SCRIPT, "description"], env=env, capture_output=True, check=False)
        output = (run.stdout + run.stderr).decode("utf-8", "replace")
        self.assertEqual(run.returncode, 1, output)
        self.assertIn("body:2:", output)

    def test_line_carrying_both_forms_is_reported_once(self):
        status, output = judge("A title", "skip-checks: true [skip ci]\n")
        self.assertEqual(status, 1, output)
        self.assertEqual(output.count("body:1:"), 1, output)

    def test_em_dash_still_fails(self):
        status, output = judge("A title", "a sentence " + EM_DASH + " with a dash\n")
        self.assertEqual(status, 1, output)
        self.assertIn("carries an em dash", output)

    def test_injected_block_still_fails(self):
        status, output = judge("A title", "body\n<!-- This is an auto-generated comment: summary -->\n")
        self.assertEqual(status, 1, output)
        self.assertIn("carries a block a review bot generated", output)

    def test_every_finding_is_reported_in_one_run(self):
        status, output = judge("A title " + EM_DASH, "commits with [skip ci]\n## Summary by CodeRabbit\n")
        self.assertEqual(status, 1, output)
        self.assertIn("carries an em dash", output)
        self.assertIn("carries a block a review bot generated", output)
        self.assertIn(SKIP_FAILURE, output)

    def test_description_read_through_the_api_with_crlf_line_endings_is_judged(self):
        require_tool(self, "jq")
        with tempfile.TemporaryDirectory() as tmp:
            payload = os.path.join(tmp, "payload.json")
            with open(payload, "w", encoding="utf-8") as fh:
                fh.write('{"title": "A title", "body": "first line\\r\\nthe fixture commits with [skip ci]\\r\\n'
                         '\\r\\nskip-checks: true\\r\\n"}')
            # A stand-in gh that answers the one call the script makes, and
            # records what it was asked so the test can hold the script to it.
            stub = os.path.join(tmp, "gh")
            with open(stub, "w", encoding="utf-8") as fh:
                fh.write('#!/usr/bin/env bash\nprintf "%s\\n" "$*" >> "' + os.path.join(tmp, "calls") + '"\n'
                         '[[ "$1" == "api" ]] && exec cat "' + payload + '"\nexit 1\n')
            os.chmod(stub, os.stat(stub).st_mode | stat.S_IXUSR)
            env = dict(os.environ, PATH=tmp + os.pathsep + os.environ.get("PATH", ""),
                       PR_NUMBER="7", GITHUB_REPOSITORY="owner/repo")
            env.pop("PR_TITLE_FILE", None)
            env.pop("PR_BODY_FILE", None)
            run = subprocess.run(["bash", SCRIPT, "description"], env=env, capture_output=True, text=True,
                                 check=False)
            with open(os.path.join(tmp, "calls"), encoding="utf-8") as fh:
                calls = fh.read()
        output = run.stdout + run.stderr
        self.assertEqual(calls, "api repos/owner/repo/pulls/7\n")
        self.assertIn("owner/repo#7, read from the API just now", output)
        self.assertEqual(run.returncode, 1, output)
        self.assertIn("body:2:the fixture commits with [skip ci]", output)
        # The trailer ends in a carriage return here, which an anchor on the
        # end of the value would miss.
        self.assertIn("body:4:skip-checks: true", output)
        self.assertIn(SKIP_FAILURE, output)

    def test_script_passes_shellcheck(self):
        shellcheck = require_tool(self, "shellcheck")
        result = subprocess.run([shellcheck, SCRIPT], capture_output=True, timeout=60, check=False)
        self.assertEqual(result.returncode, 0, result.stdout.decode() + result.stderr.decode())


if __name__ == "__main__":
    unittest.main()
