#!/usr/bin/env python3
# Copyright (c) 2026 José M. Requena Plens
# SPDX-License-Identifier: MIT

"""Tests scripts/release-order.sh, the release workflow's decision about latest.

Every v* tag used to publish the image as :latest, mark the GitHub release
Latest, stamp main's manifests, move npm's latest dist-tag and update the
Homebrew tap, whatever version it named. A backport tagged after a newer
release therefore rolled every one of those back, and a prerelease tag went
out as the default image before the npm job refused it (issue 1323).

The script is what preflight asks instead. It reads the tags the repository
already holds on stdin, as `git ls-remote --tags --refs` prints them or one
name per line, and answers for the tag being released: refused unless it is a
plain vX.Y.Z, newest when no stable tag is higher, and the npm dist-tag that
follows from that. These cases run it over lists written here, so nothing
reaches a remote.

Run with:

    python3 -m unittest discover -s scripts -p 'release_order_sh_test.py'
"""

import os
import shutil
import subprocess
import unittest

ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
SCRIPT = os.path.join(ROOT, "scripts", "release-order.sh")

LS_REMOTE = "".join(
    "0123456789abcdef0123456789abcdef0123456{0}\trefs/tags/{1}\n".format(index, tag)
    for index, tag in enumerate(["v3.0.0", "v3.1.0", "v3.2.0", "v3.3.0-rc.1", "v2.7.5"])
)


class ReleaseOrderTest(unittest.TestCase):
    """Runs the real script over tag lists written here."""

    def setUp(self):
        if shutil.which("bash") is None:
            # GitHub sets CI on every step; there a missing tool is a failure.
            if os.environ.get("CI"):
                self.fail("the script needs bash, and CI must run these cases rather than skip them")
            self.skipTest("the script needs bash")

    def run_script(self, tag, tags):
        return subprocess.run(
            ["bash", SCRIPT, tag],
            input=tags, capture_output=True, text=True, timeout=60, check=False,
        )

    def outputs(self, result):
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        lines = result.stdout.splitlines()
        # Every line of stdout is a key=value pair $GITHUB_OUTPUT accepts.
        for line in lines:
            self.assertRegex(line, r"^[a-z_]+=[^\s=]+$")
        return dict(line.split("=", 1) for line in lines)

    def test_the_highest_stable_tag_is_the_newest(self):
        got = self.outputs(self.run_script("v3.2.0", LS_REMOTE))
        self.assertEqual(got, {"stable": "true", "newest": "true", "npm_dist_tag": "latest"})

    def test_a_tag_the_remote_does_not_hold_yet_is_compared_as_itself(self):
        # A rehearsal names the version VERSION carries, which is not tagged
        # until the release is cut.
        got = self.outputs(self.run_script("v3.3.0", LS_REMOTE))
        self.assertEqual(got["newest"], "true")

    def test_a_backport_is_not_the_newest_and_keeps_npm_latest(self):
        got = self.outputs(self.run_script("v3.1.1", LS_REMOTE + "fedcba\trefs/tags/v3.1.1\n"))
        self.assertEqual(got, {"stable": "true", "newest": "false", "npm_dist_tag": "release-3.1"})

    def test_versions_are_ordered_by_number_not_by_text(self):
        tags = "v3.9.0\nv3.10.0\n"
        self.assertEqual(self.outputs(self.run_script("v3.10.0", tags))["newest"], "true")
        self.assertEqual(self.outputs(self.run_script("v3.9.1", tags))["newest"], "false")

    def test_a_prerelease_in_the_remote_does_not_outrank_a_release(self):
        # v3.3.0-rc.1 sorts above v3.2.0 by version, and must not stop v3.2.1
        # from being the newest stable release.
        got = self.outputs(self.run_script("v3.2.1", LS_REMOTE))
        self.assertEqual(got["newest"], "true")

    def test_an_empty_remote_makes_the_first_release_the_newest(self):
        self.assertEqual(self.outputs(self.run_script("v0.1.0", ""))["newest"], "true")

    def test_a_prerelease_tag_is_refused_before_anything_publishes(self):
        for tag in ("v3.3.0-rc.1", "v3.3.0+build.5", "3.3.0", "v3.3", "v3.3.0.1", "release"):
            with self.subTest(tag):
                result = self.run_script(tag, LS_REMOTE)
                self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
                self.assertIn("::error::" + tag + " is not a release version", result.stderr)
                # stdout is appended to $GITHUB_OUTPUT whole, so it carries
                # nothing on a refusal.
                self.assertEqual(result.stdout, "")

    def test_a_missing_tag_argument_is_a_usage_error(self):
        result = subprocess.run(["bash", SCRIPT], input="", capture_output=True, text=True, timeout=60, check=False)
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn("newest=", result.stdout)


if __name__ == "__main__":
    unittest.main()
