#!/usr/bin/env python3
"""Tests scripts/build-npm.mjs and scripts/validate-npm.mjs on packages
assembled from stand-in binaries.

A published npm version is permanent, so the validator decides what the
registry receives. These cases hold the seven packages to carrying the
repository's LICENSE, byte for byte, the launcher included, and the
validator to refusing a package that lost it or carries another text. The
same holds for THIRD_PARTY_NOTICES, which the release generates beside the
binaries: every package carries the copy checksums.txt names, and the
builder refuses a release without it.

Both scripts are copied into a scratch tree with the files they read, so
the build's write of the launcher package is a write into that tree and
never into this checkout. The validator runs with --no-install: the
structural checks are what these cases are about, and the stand-in
binaries carry only the header bytes those checks read.

Run with:

    python3 -m unittest discover -s scripts -p 'validate_npm_test.py'
"""

import hashlib
import json
import os
import shutil
import struct
import subprocess
import tempfile
import unittest

ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))

# The validator's size floor (MIN_BINARY_BYTES in validate-npm.mjs): a
# stand-in under it would fail for its size, not for what a case changes.
MIN_BINARY_BYTES = 5_000_000

ASSETS = {
    "linux-amd64": "gitlab-mcp-server-linux-amd64",
    "linux-arm64": "gitlab-mcp-server-linux-arm64",
    "darwin-amd64": "gitlab-mcp-server-darwin-amd64",
    "darwin-arm64": "gitlab-mcp-server-darwin-arm64",
    "windows-amd64": "gitlab-mcp-server-windows-amd64.exe",
    "windows-arm64": "gitlab-mcp-server-windows-arm64.exe",
}
PLATFORM_KEYS = ["linux-x64", "linux-arm64", "darwin-x64", "darwin-arm64", "win32-x64", "win32-arm64"]

# A stand-in for what cmd/gen_third_party_notices writes: the validator
# reads its header line and its digest, nothing else.
NOTICES = "THIRD_PARTY_NOTICES"
NOTICES_TEXT = b"Third-party notices for gitlab-mcp-server\n\nstand-in body\n"


def stand_in(plat_key):
    """The magic bytes the validator reads, padded past its size floor."""
    if plat_key.startswith("linux"):
        head = b"\x7fELF"
    elif plat_key.startswith("darwin"):
        head = b"\xcf\xfa\xed\xfe" + struct.pack("<I", 0x01000007)
    else:
        head = b"MZ"
    return head + plat_key.encode() + b"\0" * MIN_BINARY_BYTES


class NpmLicenceTest(unittest.TestCase):
    """Runs the real build and validator over stand-ins in a scratch tree."""

    def setUp(self):
        missing = [tool for tool in ("node", "npm", "tar") if shutil.which(tool) is None]
        if missing:
            reason = "the npm scripts need " + ", ".join(missing)
            # GitHub sets CI on every step; there a missing tool is a failure.
            if os.environ.get("CI"):
                self.fail(reason + ", and CI must run these cases rather than skip them")
            self.skipTest(reason)
        self.work = tempfile.mkdtemp(prefix="validate-npm-")
        self.addCleanup(shutil.rmtree, self.work, True)
        self.tree = os.path.join(self.work, "tree")
        os.makedirs(os.path.join(self.tree, "scripts"))
        for script in ("build-npm.mjs", "validate-npm.mjs"):
            shutil.copyfile(os.path.join(ROOT, "scripts", script), os.path.join(self.tree, "scripts", script))
        shutil.copyfile(os.path.join(ROOT, "LICENSE"), os.path.join(self.tree, "LICENSE"))
        launcher = os.path.join(self.tree, "npm", "gitlab-mcp-server")
        os.makedirs(launcher)
        for name in ("package.json", "cli.js", "README.md"):
            shutil.copyfile(os.path.join(ROOT, "npm", "gitlab-mcp-server", name), os.path.join(launcher, name))
        with open(os.path.join(ROOT, "VERSION"), encoding="utf-8") as fh:
            self.version = fh.read().strip()

        self.binaries = os.path.join(self.work, "dist")
        os.makedirs(self.binaries)
        lines = []
        for plat_key, asset in ASSETS.items():
            payload = stand_in(plat_key)
            with open(os.path.join(self.binaries, asset), "wb") as fh:
                fh.write(payload)
            lines.append("{}  {}".format(hashlib.sha256(payload).hexdigest(), asset))
        with open(os.path.join(self.binaries, NOTICES), "wb") as fh:
            fh.write(NOTICES_TEXT)
        lines.append("{}  {}".format(hashlib.sha256(NOTICES_TEXT).hexdigest(), NOTICES))
        with open(os.path.join(self.binaries, "checksums.txt"), "w", encoding="utf-8") as fh:
            fh.write("\n".join(lines) + "\n")

        self.packages = os.path.join(self.tree, "npm", "packages")
        self.launcher = launcher
        build = self.build()
        self.assertEqual(build.returncode, 0, build.stdout + build.stderr)
        with open(os.path.join(ROOT, "LICENSE"), "rb") as fh:
            self.licence = fh.read()

    def build(self):
        return subprocess.run(
            ["node", os.path.join(self.tree, "scripts", "build-npm.mjs"),
             "--binaries", self.binaries, "--version", self.version, "--out", self.packages],
            cwd=self.tree, capture_output=True, text=True, check=False,
        )

    def validate(self):
        return subprocess.run(
            ["node", os.path.join(self.tree, "scripts", "validate-npm.mjs"),
             "--packages", self.packages, "--main", self.launcher,
             "--version", self.version, "--no-install"],
            cwd=self.tree, capture_output=True, text=True, check=False,
        )

    def test_every_package_carries_the_licence(self):
        dirs = [os.path.join(self.packages, key) for key in PLATFORM_KEYS] + [self.launcher]
        for directory in dirs:
            with self.subTest(os.path.basename(directory)):
                with open(os.path.join(directory, "LICENSE"), "rb") as fh:
                    self.assertEqual(fh.read(), self.licence)
                with open(os.path.join(directory, "package.json"), encoding="utf-8") as fh:
                    self.assertIn("LICENSE", json.load(fh)["files"])
                self.assertEqual(os.stat(os.path.join(directory, "LICENSE")).st_mode & 0o777, 0o644)

    def test_every_package_carries_the_notices(self):
        dirs = [os.path.join(self.packages, key) for key in PLATFORM_KEYS] + [self.launcher]
        for directory in dirs:
            with self.subTest(os.path.basename(directory)):
                with open(os.path.join(directory, NOTICES), "rb") as fh:
                    self.assertEqual(fh.read(), NOTICES_TEXT)
                with open(os.path.join(directory, "package.json"), encoding="utf-8") as fh:
                    self.assertIn(NOTICES, json.load(fh)["files"])
                self.assertEqual(os.stat(os.path.join(directory, NOTICES)).st_mode & 0o777, 0o644)
        with open(os.path.join(self.packages, "verified-binaries.json"), encoding="utf-8") as fh:
            self.assertEqual(json.load(fh)["notices"], hashlib.sha256(NOTICES_TEXT).hexdigest())

    def test_the_builder_refuses_a_release_without_its_notices(self):
        cases = [
            ("no notices at all", "remove", "THIRD_PARTY_NOTICES not found"),
            ("notices checksums.txt does not name", "replace",
             "THIRD_PARTY_NOTICES is sha256"),
        ]
        for name, action, want in cases:
            with self.subTest(name):
                path = os.path.join(self.binaries, NOTICES)
                if action == "remove":
                    os.remove(path)
                else:
                    with open(path, "wb") as fh:
                        fh.write(b"Third-party notices for gitlab-mcp-server\nanother body\n")
                try:
                    result = self.build()
                finally:
                    with open(path, "wb") as fh:
                        fh.write(NOTICES_TEXT)
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertIn(want, result.stderr)

    def test_refuses_a_package_without_the_notices_or_with_others(self):
        cases = [
            ("platform package lost them", "darwin-x64", b"", "darwin-x64: tarball ships"),
            ("platform package carries another copy", "linux-x64",
             b"Third-party notices for gitlab-mcp-server\nanother body\n",
             "linux-x64: THIRD_PARTY_NOTICES in the tarball is sha256"),
            ("launcher carries a file without the header", None, b"some other text\n",
             "main: THIRD_PARTY_NOTICES in the tarball does not open with the generator's header"),
        ]
        for name, key, body, want in cases:
            with self.subTest(name):
                directory = self.launcher if key is None else os.path.join(self.packages, key)
                path = os.path.join(directory, NOTICES)
                if body:
                    with open(path, "wb") as fh:
                        fh.write(body)
                else:
                    os.remove(path)
                try:
                    result = self.validate()
                finally:
                    with open(path, "wb") as fh:
                        fh.write(NOTICES_TEXT)
                    os.chmod(path, 0o644)
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertIn(want, result.stdout)

    def test_the_built_packages_validate(self):
        result = self.validate()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("runtime: skipped (--no-install)", result.stdout)

    def test_refuses_a_package_without_the_licence_or_with_another(self):
        cases = [
            ("platform package lost it", "linux-arm64", "remove",
             'linux-arm64: tarball ships'),
            ("platform package carries another text", "win32-x64", "replace",
             "win32-x64: LICENSE in the tarball is not the repository's LICENSE"),
            ("launcher carries another text", None, "replace",
             "main: LICENSE in the tarball is not the repository's LICENSE"),
        ]
        for name, key, action, want in cases:
            with self.subTest(name):
                directory = self.launcher if key is None else os.path.join(self.packages, key)
                path = os.path.join(directory, "LICENSE")
                if action == "remove":
                    os.remove(path)
                else:
                    with open(path, "wb") as fh:
                        fh.write(b"Not the MIT License\n")
                try:
                    result = self.validate()
                finally:
                    with open(path, "wb") as fh:
                        fh.write(self.licence)
                    os.chmod(path, 0o644)
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertIn(want, result.stdout)


if __name__ == "__main__":
    unittest.main()
