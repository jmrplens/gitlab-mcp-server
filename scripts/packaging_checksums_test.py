#!/usr/bin/env python3
"""Tests that the npm, PyPI and NuGet builders refuse unverified release binaries.

The first two distributions used to be assembled from an unverified copy of
the build directory: six files of the right size with the right first bytes
passed every check either validator made, and five of the six were never
executed by anything before reaching an immutable registry. The builders now
compare each binary with the release's own cosign-signed checksums.txt, and
the NuGet builder was written to the same contract.

Run with:

    python3 -m unittest discover -s scripts -p 'packaging_checksums_test.py'
"""

import hashlib
import json
import os
import shutil
import subprocess
import sys
import tempfile
import unittest

ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))

# One fake binary per release asset. The magic bytes are what the validators
# look at; the builders themselves only care about the digests.
ASSETS = {
    "gitlab-mcp-server-linux-amd64": b"\x7fELF",
    "gitlab-mcp-server-linux-arm64": b"\x7fELF",
    "gitlab-mcp-server-darwin-amd64": b"\xcf\xfa\xed\xfe",
    "gitlab-mcp-server-darwin-arm64": b"\xcf\xfa\xed\xfe",
    "gitlab-mcp-server-windows-amd64.exe": b"MZ",
    "gitlab-mcp-server-windows-arm64.exe": b"MZ",
}


def repo_version():
    with open(os.path.join(ROOT, "VERSION"), encoding="utf-8") as fh:
        return fh.read().strip()


def write_fixture(binaries_dir, with_checksums=True, tamper=None):
    """Create the six release assets and, optionally, a matching checksums.txt.

    `tamper` names an asset to rewrite *after* its digest is recorded, which is
    exactly the shape of the defect: a manifest that no longer describes the
    bytes beside it.
    """
    os.makedirs(binaries_dir, exist_ok=True)
    lines = []
    for name, magic in ASSETS.items():
        payload = magic + (name.encode() * 64)
        path = os.path.join(binaries_dir, name)
        with open(path, "wb") as fh:
            fh.write(payload)
        lines.append("{}  {}".format(hashlib.sha256(payload).hexdigest(), name))
    if tamper:
        with open(os.path.join(binaries_dir, tamper), "wb") as fh:
            fh.write(ASSETS[tamper] + b"swapped payload")
    if with_checksums:
        with open(os.path.join(binaries_dir, "checksums.txt"), "w", encoding="utf-8") as fh:
            fh.write("\n".join(lines) + "\n")


class BuilderChecksumTestCase(unittest.TestCase):
    """Shared fixture: a scratch binaries dir and a scratch output dir."""

    def setUp(self):
        self.work = tempfile.mkdtemp(prefix="packaging-checksums-")
        self.addCleanup(shutil.rmtree, self.work, True)
        self.binaries = os.path.join(self.work, "dist")
        self.out = os.path.join(self.work, "out")
        self.version = repo_version()

    def manifest(self):
        with open(os.path.join(self.out, "verified-binaries.json"), encoding="utf-8") as fh:
            return json.load(fh)


class BuildNpmTest(BuilderChecksumTestCase):
    """Verifies scripts/build-npm.mjs will not package an unverified binary.

    build-npm.mjs also rewrites the committed launcher package.json, so the
    test pins the repository's own VERSION (making that write a no-op) and
    restores the file regardless.
    """

    def setUp(self):
        super().setUp()
        launcher = os.path.join(ROOT, "npm", "gitlab-mcp-server", "package.json")
        with open(launcher, "rb") as fh:
            original = fh.read()

        def restore():
            with open(launcher, "wb") as fh:
                fh.write(original)

        self.addCleanup(restore)

    def run_builder(self, *extra):
        return subprocess.run(
            [
                "node", os.path.join(ROOT, "scripts", "build-npm.mjs"),
                "--binaries", self.binaries,
                "--version", self.version,
                "--out", self.out,
                *extra,
            ],
            cwd=ROOT, capture_output=True, text=True, check=False,
        )

    def test_refuses_binaries_it_cannot_verify(self):
        cases = [
            ("no checksums.txt at all", dict(with_checksums=False), (), 1, "checksums.txt"),
            ("a binary that does not match", dict(tamper="gitlab-mcp-server-linux-arm64"), (),
             1, "gitlab-mcp-server-linux-arm64"),
            ("matching checksums", dict(), (), 0, ""),
            ("explicit opt-out", dict(with_checksums=False), ("--allow-unverified",), 0, ""),
        ]
        for name, fixture, extra, want_code, want_text in cases:
            with self.subTest(name):
                shutil.rmtree(self.binaries, ignore_errors=True)
                shutil.rmtree(self.out, ignore_errors=True)
                write_fixture(self.binaries, **fixture)
                result = self.run_builder(*extra)
                self.assertEqual(result.returncode != 0, want_code != 0, result.stderr)
                if want_text:
                    self.assertIn(want_text, result.stderr)

    def test_records_what_it_verified(self):
        write_fixture(self.binaries)
        self.assertEqual(self.run_builder().returncode, 0)
        manifest = self.manifest()
        self.assertTrue(manifest["verified"])
        self.assertEqual(manifest["version"], self.version)
        self.assertEqual(len(manifest["binaries"]), len(ASSETS))

    def test_opting_out_is_recorded_as_unverified(self):
        write_fixture(self.binaries, with_checksums=False)
        self.assertEqual(self.run_builder("--allow-unverified").returncode, 0)
        self.assertFalse(self.manifest()["verified"])


class BuildPypiTest(BuilderChecksumTestCase):
    """Verifies scripts/build_pypi.py will not put an unverified binary in a wheel."""

    def run_builder(self, *extra):
        return subprocess.run(
            [
                sys.executable, os.path.join(ROOT, "scripts", "build_pypi.py"),
                "--binaries", self.binaries,
                "--version", self.version,
                "--out", self.out,
                *extra,
            ],
            cwd=ROOT, capture_output=True, text=True, check=False,
        )

    def test_refuses_binaries_it_cannot_verify(self):
        cases = [
            ("no checksums.txt at all", dict(with_checksums=False), (), 1, "checksums.txt"),
            ("a binary that does not match", dict(tamper="gitlab-mcp-server-windows-arm64.exe"), (),
             1, "gitlab-mcp-server-windows-arm64.exe"),
            ("matching checksums", dict(), (), 0, ""),
            ("explicit opt-out", dict(with_checksums=False), ("--allow-unverified",), 0, ""),
        ]
        for name, fixture, extra, want_code, want_text in cases:
            with self.subTest(name):
                shutil.rmtree(self.binaries, ignore_errors=True)
                shutil.rmtree(self.out, ignore_errors=True)
                write_fixture(self.binaries, **fixture)
                result = self.run_builder(*extra)
                output = result.stdout + result.stderr
                self.assertEqual(result.returncode != 0, want_code != 0, output)
                if want_text:
                    self.assertIn(want_text, output)

    def test_records_what_it_verified(self):
        write_fixture(self.binaries)
        self.assertEqual(self.run_builder().returncode, 0)
        manifest = self.manifest()
        self.assertTrue(manifest["verified"])
        self.assertEqual(manifest["version"], self.version)
        self.assertEqual(len(manifest["binaries"]), len(ASSETS))


class BuildNugetTest(BuilderChecksumTestCase):
    """Verifies scripts/build_nuget.py will not put an unverified binary in a package."""

    def run_builder(self, *extra):
        return subprocess.run(
            [
                sys.executable, os.path.join(ROOT, "scripts", "build_nuget.py"),
                "--binaries", self.binaries,
                "--version", self.version,
                "--out", self.out,
                *extra,
            ],
            cwd=ROOT, capture_output=True, text=True, check=False,
        )

    def test_refuses_binaries_it_cannot_verify(self):
        cases = [
            ("no checksums.txt at all", dict(with_checksums=False), (), 1, "checksums.txt"),
            ("a binary that does not match", dict(tamper="gitlab-mcp-server-darwin-arm64"), (),
             1, "gitlab-mcp-server-darwin-arm64"),
            ("matching checksums", dict(), (), 0, ""),
            ("explicit opt-out", dict(with_checksums=False), ("--allow-unverified",), 0, ""),
        ]
        for name, fixture, extra, want_code, want_text in cases:
            with self.subTest(name):
                shutil.rmtree(self.binaries, ignore_errors=True)
                shutil.rmtree(self.out, ignore_errors=True)
                write_fixture(self.binaries, **fixture)
                result = self.run_builder(*extra)
                output = result.stdout + result.stderr
                self.assertEqual(result.returncode != 0, want_code != 0, output)
                if want_text:
                    self.assertIn(want_text, output)

    def test_records_what_it_verified(self):
        write_fixture(self.binaries)
        self.assertEqual(self.run_builder().returncode, 0)
        manifest = self.manifest()
        self.assertTrue(manifest["verified"])
        self.assertEqual(manifest["version"], self.version)
        self.assertEqual(len(manifest["binaries"]), len(ASSETS))


if __name__ == "__main__":
    unittest.main()
