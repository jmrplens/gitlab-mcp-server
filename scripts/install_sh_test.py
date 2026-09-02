#!/usr/bin/env python3
"""Tests scripts/install.sh against a local fixture release.

The installer downloads a binary and a checksums.txt from the same GitHub
release and compares hashes. That is fail-closed and correct against
corruption, and worth nothing against a principal who can replace release
assets: releases are mutable, so both files get replaced together and the
comparison still passes. Every release publishes a keyless cosign signature
over checksums.txt, which the installer used to ignore entirely.

These tests drive the real script over HTTP against a directory of fixture
files, with a controlled PATH so the presence or absence of cosign and gh is
part of the case rather than a property of the machine.

Run with:

    python3 -m unittest discover -s scripts -p 'install_sh_test.py'
"""

import hashlib
import http.server
import os
import platform
import shutil
import subprocess
import tempfile
import threading
import unittest

ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
INSTALLER = os.path.join(ROOT, "scripts", "install.sh")

# Everything install.sh reaches for, minus the signature tools, which each case
# decides about. A curated PATH is the point: otherwise "no cosign installed"
# means "no cosign on this developer's machine today".
BASE_TOOLS = [
    "sh", "uname", "tr", "curl", "wget", "sha256sum", "shasum", "mktemp",
    "grep", "cut", "rm", "mkdir", "install", "cp", "chmod", "cat", "printf",
]


class FixtureRelease:
    """A directory of release assets served over HTTP on localhost."""

    def __init__(self, directory):
        self.directory = directory
        handler = http.server.SimpleHTTPRequestHandler

        class Handler(handler):
            def __init__(self, *args, **kwargs):
                super().__init__(*args, directory=directory, **kwargs)

            def log_message(self, *args):  # keep the test output readable
                pass

        self.server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()

    @property
    def base_url(self):
        host, port = self.server.server_address[:2]
        return f"http://{host}:{port}"

    def close(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=5)


def host_asset():
    machine = platform.machine().lower()
    arch = "amd64" if machine in ("x86_64", "amd64") else "arm64"
    return f"gitlab-mcp-server-{platform.system().lower()}-{arch}"


class InstallShTest(unittest.TestCase):
    """Verifies install.sh's verification behaviour end to end.

    Each case builds a fixture release, points the installer at it with
    RELEASE_BASE_URL, and asserts on the exit status and the messages the user
    actually sees.
    """

    def setUp(self):
        self.work = tempfile.mkdtemp(prefix="install-sh-")
        self.addCleanup(shutil.rmtree, self.work, True)
        self.release_dir = os.path.join(self.work, "release")
        self.bin_dir = os.path.join(self.work, "bin")
        self.install_dir = os.path.join(self.work, "target")
        os.makedirs(self.release_dir)
        os.makedirs(self.bin_dir)

        self.asset = host_asset()
        self.payload = b"\x7fELF" + b"fixture binary" * 32
        with open(os.path.join(self.release_dir, self.asset), "wb") as fh:
            fh.write(self.payload)

        for tool in BASE_TOOLS:
            real = shutil.which(tool)
            if real:
                os.symlink(real, os.path.join(self.bin_dir, tool))

        self.release = FixtureRelease(self.release_dir)
        self.addCleanup(self.release.close)

    def write_checksums(self, digest=None):
        digest = digest or hashlib.sha256(self.payload).hexdigest()
        with open(os.path.join(self.release_dir, "checksums.txt"), "w", encoding="utf-8") as fh:
            fh.write(f"{digest}  {self.asset}\n")

    def write_bundle(self):
        with open(os.path.join(self.release_dir, "checksums.txt.sigstore.json"), "w", encoding="utf-8") as fh:
            fh.write('{"fixture": true}\n')

    def fake_tool(self, name, exit_code):
        path = os.path.join(self.bin_dir, name)
        with open(path, "w", encoding="utf-8") as fh:
            fh.write(f'#!/bin/sh\necho "{name} fixture invoked: $*" >&2\nexit {exit_code}\n')
        os.chmod(path, 0o755)

    def run_installer(self, **env):
        environment = {
            "PATH": self.bin_dir,
            "HOME": self.work,
            "INSTALL_DIR": self.install_dir,
            "RELEASE_BASE_URL": self.release.base_url,
        }
        environment.update(env)
        return subprocess.run(
            ["sh", INSTALLER], env=environment, capture_output=True, text=True, check=False, cwd=self.work
        )

    def test_checksum_failures_still_abort(self):
        cases = [
            ("hash does not match", "0" * 64, "checksum mismatch"),
            ("asset absent from the manifest", None, None),
        ]
        for name, digest, message in cases:
            with self.subTest(name):
                if digest:
                    self.write_checksums(digest)
                else:
                    with open(os.path.join(self.release_dir, "checksums.txt"), "w", encoding="utf-8") as fh:
                        fh.write("deadbeef  some-other-asset\n")
                result = self.run_installer()
                self.assertNotEqual(result.returncode, 0, result.stderr)
                if message:
                    self.assertIn(message, result.stderr)

    def test_no_signature_tool_warns_and_can_be_made_fatal(self):
        """Without cosign or gh the install proceeds, loudly — unless asked not to."""
        self.write_checksums()
        cases = [
            ("default: warn and install", {}, 0, "no signature was verified"),
            ("REQUIRE_SIGNATURE=1: abort", {"REQUIRE_SIGNATURE": "1"}, 1, "REQUIRE_SIGNATURE=1"),
        ]
        for name, env, want_code, message in cases:
            with self.subTest(name):
                result = self.run_installer(**env)
                self.assertEqual(result.returncode, want_code, result.stderr)
                self.assertIn(message, result.stderr)

    def test_cosign_verifies_the_sigstore_bundle(self):
        """A bundle is fetched and cosign's verdict decides the install."""
        self.write_checksums()
        self.write_bundle()
        cases = [
            ("cosign accepts", 0, 0, "signature OK"),
            ("cosign rejects", 1, 1, "failed cosign verification"),
        ]
        for name, cosign_exit, want_code, message in cases:
            with self.subTest(name):
                self.fake_tool("cosign", cosign_exit)
                result = self.run_installer(REQUIRE_SIGNATURE="1")
                self.assertEqual(result.returncode, want_code, result.stderr)
                self.assertIn(message, result.stderr)
                self.assertIn("cosign fixture invoked", result.stderr)

    def test_missing_bundle_is_reported_not_ignored(self):
        """A release with no signature is called out rather than passed over."""
        self.write_checksums()
        self.fake_tool("cosign", 0)
        result = self.run_installer()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("publishes no checksums.txt.sigstore.json", result.stderr)
        self.assertIn("no signature was verified", result.stderr)

    def test_gh_attestation_is_the_fallback(self):
        """With no cosign but a gh CLI, the build-provenance attestation counts."""
        self.write_checksums()
        cases = [
            ("gh verifies", 0, 0, "attestation OK"),
            ("gh cannot verify", 1, 1, "REQUIRE_SIGNATURE=1"),
        ]
        for name, gh_exit, want_code, message in cases:
            with self.subTest(name):
                self.fake_tool("gh", gh_exit)
                result = self.run_installer(REQUIRE_SIGNATURE="1")
                self.assertEqual(result.returncode, want_code, result.stderr)
                self.assertIn(message, result.stderr)

    def test_a_verified_install_lands_the_binary(self):
        """The happy path still installs, executable, at INSTALL_DIR."""
        self.write_checksums()
        self.write_bundle()
        self.fake_tool("cosign", 0)
        result = self.run_installer(REQUIRE_SIGNATURE="1")
        self.assertEqual(result.returncode, 0, result.stderr)
        installed = os.path.join(self.install_dir, "gitlab-mcp-server")
        self.assertTrue(os.path.isfile(installed))
        self.assertTrue(os.access(installed, os.X_OK))
        with open(installed, "rb") as fh:
            self.assertEqual(fh.read(), self.payload)


if __name__ == "__main__":
    unittest.main()
