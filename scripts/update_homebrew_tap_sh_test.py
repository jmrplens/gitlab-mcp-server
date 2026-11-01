#!/usr/bin/env python3
# Copyright (c) 2026 José M. Requena Plens
# SPDX-License-Identifier: MIT

"""Tests scripts/update-homebrew-tap.sh in its dry-run mode.

The script renders the tap formula from a release's checksums.txt, checks it
with ruby -c and commits it to a clone of the tap. These cases run it against
a local git repository standing in for that clone, with --dry-run, so nothing
is cloned or pushed, and read the formula it wrote. They hold the parts the
release depends on: each binary pinned to its own checksum, and the licence
and the third-party notices installed beside the binary, the notices pinned to
the checksum the release signed and the licence to the hash of this
repository's LICENSE, fetched from the tag's tree. They also hold the
copyright and license header the formula opens with, since the tap publishes
it as source, with its magic comments still read below it.

Run with:

    python3 -m unittest discover -s scripts -p 'update_homebrew_tap_sh_test.py'
"""

import hashlib
import os
import shutil
import subprocess
import tempfile
import unittest

ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
SCRIPT = os.path.join(ROOT, "scripts", "update-homebrew-tap.sh")
VERSION = "9.8.7"
RELEASE = "https://github.com/jmrplens/gitlab-mcp-server/releases/download/v" + VERSION

ASSETS = {
    "gitlab-mcp-server-darwin-arm64": "a1" * 32,
    "gitlab-mcp-server-darwin-amd64": "a2" * 32,
    "gitlab-mcp-server-linux-arm64": "a3" * 32,
    "gitlab-mcp-server-linux-amd64": "a4" * 32,
    "gitlab-mcp-server-windows-amd64.exe": "a5" * 32,
    "THIRD_PARTY_NOTICES": "b6" * 32,
}


class UpdateHomebrewTapTest(unittest.TestCase):
    """Runs the real script against a local stand-in for the tap clone."""

    def setUp(self):
        for tool in ("bash", "git", "ruby"):
            if shutil.which(tool) is None:
                # GitHub sets CI on every step; there a missing tool is a failure.
                if os.environ.get("CI"):
                    self.fail("the script needs " + tool + ", and CI must run these cases rather than skip them")
                self.skipTest("the script needs " + tool)
        self.work = tempfile.mkdtemp(prefix="update-homebrew-tap-")
        self.addCleanup(shutil.rmtree, self.work, True)
        self.tap = os.path.join(self.work, "tap")
        self.env = dict(os.environ)
        # The clone's commits must not reach for the developer's signing key or
        # identity: the script sets the tap's own identity, and nothing else.
        self.env.update(GIT_CONFIG_GLOBAL=os.devnull, GIT_CONFIG_NOSYSTEM="1")
        # The tap has history, and the dry run shows the formula's diff against
        # the commit before its own, so the stand-in starts with one commit.
        seed = dict(self.env, GIT_AUTHOR_NAME="fixture", GIT_AUTHOR_EMAIL="fixture@example.com",
                    GIT_COMMITTER_NAME="fixture", GIT_COMMITTER_EMAIL="fixture@example.com")
        subprocess.run(["git", "init", "-q", self.tap], env=seed, check=True)
        with open(os.path.join(self.tap, "README.md"), "w", encoding="utf-8") as fh:
            fh.write("tap\n")
        subprocess.run(["git", "-C", self.tap, "add", "README.md"], env=seed, check=True)
        subprocess.run(["git", "-C", self.tap, "commit", "-q", "-m", "seed"], env=seed, check=True)

    def checksums(self, assets):
        path = os.path.join(self.work, "checksums.txt")
        with open(path, "w", encoding="utf-8") as fh:
            for name, sha in assets.items():
                fh.write(sha + "  " + name + "\n")
        return path

    def run_script(self, assets):
        return subprocess.run(
            ["bash", SCRIPT, self.checksums(assets), VERSION, self.tap, "--dry-run"],
            env=self.env, capture_output=True, text=True, timeout=120, check=False,
        )

    def formula(self):
        with open(os.path.join(self.tap, "Formula", "gitlab-mcp-server.rb"), encoding="utf-8") as fh:
            return fh.read()

    def test_the_formula_installs_the_licence_and_notices_beside_the_binary(self):
        result = self.run_script(ASSETS)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("Dry run: the tap would receive this commit, nothing was pushed", result.stdout)

        with open(os.path.join(ROOT, "LICENSE"), "rb") as fh:
            licence_sha = hashlib.sha256(fh.read()).hexdigest()
        formula = self.formula()
        expected = [
            ('url "' + RELEASE + '/gitlab-mcp-server-darwin-arm64"\n      sha256 "' + "a1" * 32 + '"'),
            ('url "' + RELEASE + '/gitlab-mcp-server-linux-amd64"\n      sha256 "' + "a4" * 32 + '"'),
            ('resource "license" do\n'
             '    url "https://raw.githubusercontent.com/jmrplens/gitlab-mcp-server/v' + VERSION + '/LICENSE"\n'
             '    sha256 "' + licence_sha + '"\n  end'),
            ('resource "third-party-notices" do\n'
             '    url "' + RELEASE + '/THIRD_PARTY_NOTICES"\n'
             '    sha256 "' + "b6" * 32 + '"\n  end'),
            'resource("license").stage { prefix.install "LICENSE" }',
            'resource("third-party-notices").stage { prefix.install "THIRD_PARTY_NOTICES" }',
        ]
        for fragment in expected:
            with self.subTest(fragment.splitlines()[0]):
                self.assertIn(fragment, formula)

    def test_the_formula_opens_with_the_file_header(self):
        # The header this script opens with below its shebang, which make
        # check-file-headers holds to cmd/internal/fileheader, so a change of
        # holder or year rewrites the oracle along with the tree.
        with open(SCRIPT, encoding="utf-8") as fh:
            header = "".join(fh.readlines()[1:3])
        self.assertTrue(header.startswith("# Copyright (c) "), header)
        result = self.run_script(ASSETS)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        formula = self.formula()
        # First and followed by a blank line, where the gate puts it in a Ruby
        # file of the tree with no shebang and no encoding declaration.
        self.assertTrue(formula.startswith(header + "\n"), formula[:300])

        # The magic comments below it still hold: Ruby reads one in any
        # comment before the first token, so the lines above the class are
        # comments alone, and run with a probe they freeze its literal.
        leading = formula.split("class GitlabMcpServer < Formula\n", 1)[0]
        for line in leading.splitlines():
            with self.subTest(line):
                self.assertTrue(line == "" or line.startswith("#"), line)
        self.assertIn("\n# typed: false\n# frozen_string_literal: true\n", leading)
        probe = os.path.join(self.work, "probe.rb")
        with open(probe, "w", encoding="utf-8") as fh:
            fh.write(leading + 'print "".frozen?\n')
        ran = subprocess.run(["ruby", probe], env=self.env, capture_output=True, text=True, timeout=60, check=False)
        self.assertEqual((ran.returncode, ran.stdout), (0, "true"), ran.stderr)

    def seed_formula(self, version):
        """Commits a formula at version to the stand-in tap, as an earlier release left it."""
        seed = dict(self.env, GIT_AUTHOR_NAME="fixture", GIT_AUTHOR_EMAIL="fixture@example.com",
                    GIT_COMMITTER_NAME="fixture", GIT_COMMITTER_EMAIL="fixture@example.com")
        os.makedirs(os.path.join(self.tap, "Formula"), exist_ok=True)
        path = os.path.join(self.tap, "Formula", "gitlab-mcp-server.rb")
        with open(path, "w", encoding="utf-8") as fh:
            fh.write('class GitlabMcpServer < Formula\n  version "' + version + '"\nend\n')
        subprocess.run(["git", "-C", self.tap, "add", "Formula"], env=seed, check=True)
        subprocess.run(["git", "-C", self.tap, "commit", "-q", "-m", "seed " + version], env=seed, check=True)
        return path

    # Issue 1323: a backport tagged after a newer release wrote its older
    # version into the formula, so `brew upgrade` downgraded. The workflow runs
    # this script only for the newest release now, and the script refuses on
    # its own as well, since the tap is the one place that says which version
    # it holds.
    def test_refuses_to_move_the_formula_back_to_an_older_version(self):
        for newer in ("9.8.8", "9.9.0", "10.0.0", "9.10.0"):
            with self.subTest(newer):
                path = self.seed_formula(newer)
                result = self.run_script(ASSETS)
                self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
                self.assertIn("the tap already holds " + newer + ", newer than " + VERSION, result.stderr)
                with open(path, encoding="utf-8") as fh:
                    self.assertIn('version "' + newer + '"', fh.read())

    def test_moves_the_formula_forward_and_rewrites_the_same_version(self):
        for earlier in ("9.8.6", "9.7.10", "8.99.99", VERSION):
            with self.subTest(earlier):
                self.seed_formula(earlier)
                result = self.run_script(ASSETS)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                self.assertIn('version "' + VERSION + '"', self.formula())

    def test_refuses_a_release_whose_checksums_name_no_notices(self):
        assets = {name: sha for name, sha in ASSETS.items() if name != "THIRD_PARTY_NOTICES"}
        result = self.run_script(assets)
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn("no checksum for THIRD_PARTY_NOTICES", result.stderr)
        self.assertFalse(os.path.exists(os.path.join(self.tap, "Formula", "gitlab-mcp-server.rb")))


if __name__ == "__main__":
    unittest.main()
