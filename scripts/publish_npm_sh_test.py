#!/usr/bin/env python3
# Copyright (c) 2026 José M. Requena Plens
# SPDX-License-Identifier: MIT

"""Tests scripts/publish-npm.sh against a stand-in npm.

The script publishes the six platform packages and then the launcher that
pins them. Without a dist-tag npm moves `latest` to whatever it publishes, so
a backport released after a newer version rolled every `npm install` back to
it (issue 1323). The release workflow now hands the script the dist-tag
the order check of cmd/release_checks decides, and these cases hold the
script to passing it to every publish, the launcher's included, and to
publishing under npm's default when it is given none.

They run a copy of the script in a scratch tree holding the seven
package.json files --no-assemble expects, with a stand-in for npm first on
PATH that records every call and answers `npm view` as for a version the
registry does not hold yet. node reads the package names, as the script
does.

Run with:

    python3 -m unittest discover -s scripts -p 'publish_npm_sh_test.py'
"""

import json
import os
import shutil
import subprocess
import tempfile
import unittest

ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
SCRIPT = os.path.join(ROOT, "scripts", "publish-npm.sh")
VERSION = "9.8.7"
PLATFORM_KEYS = ["linux-x64", "linux-arm64", "darwin-x64", "darwin-arm64", "win32-x64", "win32-arm64"]

NPM = """#!/usr/bin/env bash
printf '%s\\n' "$*" >> "$NPM_CALLS"
case "$1" in
  view) exit 1 ;;
  *) exit 0 ;;
esac
"""


class PublishNpmTest(unittest.TestCase):
    """Runs a copy of the real script against a stand-in npm."""

    def setUp(self):
        for tool in ("bash", "node"):
            if shutil.which(tool) is None:
                # GitHub sets CI on every step; there a missing tool is a failure.
                if os.environ.get("CI"):
                    self.fail("the script needs " + tool + ", and CI must run these cases rather than skip them")
                self.skipTest("the script needs " + tool)
        self.work = tempfile.mkdtemp(prefix="publish-npm-")
        self.addCleanup(shutil.rmtree, self.work, True)
        os.makedirs(os.path.join(self.work, "scripts"))
        shutil.copy(SCRIPT, os.path.join(self.work, "scripts", "publish-npm.sh"))
        for key in PLATFORM_KEYS:
            self.package(os.path.join("npm", "packages", key), "@jmrp.io/gitlab-mcp-server-" + key)
        self.package(os.path.join("npm", "gitlab-mcp-server"), "@jmrp.io/gitlab-mcp-server")
        bin_dir = os.path.join(self.work, "bin")
        os.makedirs(bin_dir)
        npm = os.path.join(bin_dir, "npm")
        with open(npm, "w", encoding="utf-8") as fh:
            fh.write(NPM)
        os.chmod(npm, 0o755)
        self.calls = os.path.join(self.work, "npm-calls.txt")
        self.env = dict(os.environ, PATH=bin_dir + os.pathsep + os.environ.get("PATH", ""), NPM_CALLS=self.calls)

    def package(self, relative, name):
        directory = os.path.join(self.work, relative)
        os.makedirs(directory)
        with open(os.path.join(directory, "package.json"), "w", encoding="utf-8") as fh:
            json.dump({"name": name, "version": VERSION}, fh)

    def run_script(self, *args):
        return subprocess.run(
            ["bash", os.path.join(self.work, "scripts", "publish-npm.sh"), "dist", VERSION, "--no-assemble", *args],
            env=self.env, capture_output=True, text=True, timeout=120, check=False,
        )

    def publishes(self):
        if not os.path.exists(self.calls):
            return []
        with open(self.calls, encoding="utf-8") as fh:
            return [line.rstrip("\n") for line in fh if line.startswith("publish ")]

    def expected(self, suffix):
        directories = [os.path.join(self.work, "npm", "packages", key) for key in PLATFORM_KEYS]
        directories.append(os.path.join(self.work, "npm", "gitlab-mcp-server"))
        return ["publish " + directory + " --access public" + suffix for directory in directories]

    def test_without_a_dist_tag_every_package_publishes_under_npms_default(self):
        result = self.run_script()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.publishes(), self.expected(""))

    def test_a_dist_tag_reaches_every_publish_the_launcher_last(self):
        result = self.run_script("--tag", "release-3.1")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.publishes(), self.expected(" --tag release-3.1"))
        self.assertIn("under the dist-tag release-3.1", result.stdout)

    def test_a_dist_tag_reaches_a_dry_run_too(self):
        result = self.run_script("--dry-run", "--tag", "release-3.1")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.publishes(), self.expected(" --dry-run --tag release-3.1"))

    def test_a_dist_tag_with_no_name_is_refused_before_anything_publishes(self):
        for args in (["--tag"], ["--tag", ""], ["--tag", "--dry-run"]):
            with self.subTest(args):
                result = self.run_script(*args)
                self.assertEqual(result.returncode, 2, result.stdout + result.stderr)
                self.assertIn("--tag needs the name of a dist-tag", result.stderr)
                self.assertEqual(self.publishes(), [])

    def test_an_unknown_option_is_still_refused(self):
        result = self.run_script("--latest")
        self.assertEqual(result.returncode, 2, result.stdout + result.stderr)
        self.assertIn("unknown option: --latest", result.stderr)
        self.assertEqual(self.publishes(), [])


if __name__ == "__main__":
    unittest.main()
