#!/usr/bin/env python3
"""Tests scripts/update-homebrew-tap.sh in its dry-run mode.

The script renders the tap formula from a release's checksums.txt, checks it
with ruby -c and commits it to a clone of the tap. These cases run it against
a local git repository standing in for that clone, with --dry-run, so nothing
is cloned or pushed, and read the formula it wrote. They hold the parts the
release depends on: each binary pinned to its own checksum, and the licence
and the third-party notices installed beside the binary, the notices pinned to
the checksum the release signed and the licence to the hash of this
repository's LICENSE, fetched from the tag's tree.

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

    def test_refuses_a_release_whose_checksums_name_no_notices(self):
        assets = {name: sha for name, sha in ASSETS.items() if name != "THIRD_PARTY_NOTICES"}
        result = self.run_script(assets)
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn("no checksum for THIRD_PARTY_NOTICES", result.stderr)
        self.assertFalse(os.path.exists(os.path.join(self.tap, "Formula", "gitlab-mcp-server.rb")))


if __name__ == "__main__":
    unittest.main()
