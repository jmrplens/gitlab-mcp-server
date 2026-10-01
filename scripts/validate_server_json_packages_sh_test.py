#!/usr/bin/env python3
"""Tests the Claude Desktop bundle checks of scripts/validate-server-json-packages.sh.

`make check-server-json-packages` downloads every package server.json declares.
For the mcpb entries it holds each bundle to its fileSha256, opens it as a zip
and checks every path its manifest names, and then asks one question of the
entries together: is each of darwin, win32 and linux served by exactly one
declared bundle. A registry entry has no platform field, so a system no bundle
serves is offered nothing it can install, and a system two serve is offered
two bundles with nothing to tell them apart, which is what declaring the
universal bundle beside the per-OS ones would do. Both shapes the repository
has used pass: the single universal bundle main declared until the first
release that builds per-OS bundles, and the three per-OS bundles after it.

Each case runs the real script over a scratch server.json whose identifiers
are file:// URLs of bundles built here, which curl downloads like any other,
so the cases need no network. The scratch server.json declares no npm entry,
which is what the script reads the release to check for immutability from, so
nothing here reaches GitHub either.

Run with:

    python3 -m unittest discover -s scripts -p 'validate_server_json_packages_sh_test.py'
"""

import hashlib
import json
import os
import shutil
import subprocess
import tempfile
import unittest
import zipfile

ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
SCRIPT = os.path.join(ROOT, "scripts", "validate-server-json-packages.sh")
SERVER_NAME = "io.github.jmrplens/gitlab-mcp-server"

# What each bundle carries and how its manifest starts it, as the build packs
# them: the server entries and the base command line of each.
LAYOUTS = {
    "darwin": (["server/gitlab-mcp-server"], "${__dirname}/server/gitlab-mcp-server", []),
    "win32": (["server/gitlab-mcp-server.exe"], "${__dirname}/server/gitlab-mcp-server.exe", []),
    "linux": (["server/linux/launch.sh", "server/linux/gitlab-mcp-server-linux-amd64"],
              "/bin/sh", ["${__dirname}/server/linux/launch.sh"]),
}


class ValidateMcpbPackagesTest(unittest.TestCase):
    """Runs the real validator over bundles served from file:// URLs."""

    def setUp(self):
        missing = [tool for tool in ("bash", "jq", "curl", "python3", "sha256sum") if shutil.which(tool) is None]
        if missing:
            reason = "the validator needs " + ", ".join(missing)
            # A skip keeps the job green, so on a runner without these tools
            # every case here would stop running and nobody would be told.
            # GitHub sets CI on every step; there a missing tool is a failure.
            if os.environ.get("CI"):
                self.fail(reason + ", and CI must run these cases rather than skip them")
            self.skipTest(reason)
        self.work = tempfile.mkdtemp(prefix="validate-server-json-")
        self.addCleanup(shutil.rmtree, self.work, True)

    def bundle(self, name, platform=None, platforms=None):
        """Writes a bundle for one platform's layout, listing platforms (that
        platform alone by default), or the universal bundle when platform is
        None; returns its path."""
        path = os.path.join(self.work, name)
        if platform is None:
            entries = [entry for layout in LAYOUTS.values() for entry in layout[0]]
            config = {"command": LAYOUTS["darwin"][1], "args": [], "platform_overrides": {
                "win32": {"command": LAYOUTS["win32"][1]},
                "linux": {"command": LAYOUTS["linux"][1], "args": LAYOUTS["linux"][2]},
            }}
            entry_point = LAYOUTS["darwin"][0][0]
            listed = ["darwin", "win32", "linux"] if platforms is None else platforms
        else:
            entries, command, args = LAYOUTS[platform]
            config = {"command": command, "args": args}
            entry_point = entries[0]
            listed = [platform] if platforms is None else platforms
        manifest = {"server": {"entry_point": entry_point, "mcp_config": config},
                    "compatibility": {"platforms": listed}}
        with zipfile.ZipFile(path, "w") as archive:
            archive.writestr("manifest.json", json.dumps(manifest))
            for entry in entries:
                archive.writestr(entry, entry)
        return path

    def declare(self, *bundles, wrong_hash_for=None):
        """Writes a server.json declaring each bundle by its file:// URL."""
        packages = []
        for path in bundles:
            with open(path, "rb") as fh:
                digest = hashlib.sha256(fh.read()).hexdigest()
            if path == wrong_hash_for:
                digest = "0" * 64
            packages.append({"registryType": "mcpb", "identifier": "file://" + path,
                             "version": "9.8.7", "fileSha256": digest, "transport": {"type": "stdio"}})
        server_json = os.path.join(self.work, "server.json")
        with open(server_json, "w", encoding="utf-8") as fh:
            json.dump({"name": SERVER_NAME, "packages": packages}, fh)
        return server_json

    def validate(self, server_json):
        env = dict(os.environ)
        env.pop("GH_TOKEN", None)
        return subprocess.run(
            ["bash", SCRIPT, server_json],
            cwd=self.work,
            env=env,
            capture_output=True,
            timeout=120,
            check=False,
        )

    def per_os(self):
        return [self.bundle("gitlab-mcp-server-" + suffix + ".mcpb", platform)
                for suffix, platform in (("darwin", "darwin"), ("windows", "win32"), ("linux", "linux"))]

    def test_accepts_one_bundle_per_system(self):
        result = self.validate(self.declare(*self.per_os()))
        self.assertEqual(result.returncode, 0, result.stderr.decode())
        stdout = result.stdout.decode()
        self.assertIn("valid bundle for darwin", stdout)
        self.assertIn("valid bundle for win32", stdout)
        self.assertIn("valid bundle for linux", stdout)
        self.assertIn("OK: darwin, win32 and linux are each served by exactly one declared bundle", stdout)
        self.assertIn("All 3 declared package(s) validated", stdout)

    def test_accepts_the_universal_bundle_alone(self):
        # The shape main declares until the first release that builds per-OS
        # bundles: one bundle serving all three systems.
        result = self.validate(self.declare(self.bundle("gitlab-mcp-server.mcpb")))
        self.assertEqual(result.returncode, 0, result.stderr.decode())
        self.assertIn("valid bundle for darwin win32 linux", result.stdout.decode())
        self.assertIn("each served by exactly one declared bundle", result.stdout.decode())

    def test_refuses_a_system_served_twice_or_not_at_all(self):
        darwin, windows, linux = self.per_os()
        universal = self.bundle("gitlab-mcp-server.mcpb")
        freebsd = self.bundle("gitlab-mcp-server-freebsd.mcpb", "linux", platforms=["freebsd"])
        cases = [
            ("the universal bundle beside the per-OS ones", [darwin, windows, linux, universal], [
                "darwin is served by 2 declared bundles (file://" + darwin + " file://" + universal + ")",
                "win32 is served by 2 declared bundles",
                "linux is served by 2 declared bundles",
            ]),
            ("a system no bundle serves", [darwin, windows], [
                "no declared bundle serves linux, so a client there is offered nothing it can install",
            ]),
            ("a platform Claude Desktop does not report", [darwin, windows, linux, freebsd], [
                "file://" + freebsd + " lists freebsd, which is not a platform Claude Desktop reports",
            ]),
        ]
        for name, bundles, messages in cases:
            with self.subTest(case=name):
                result = self.validate(self.declare(*bundles))
                stderr = result.stderr.decode()
                self.assertEqual(result.returncode, 1, stderr)
                for message in messages:
                    self.assertIn(message, stderr)
                self.assertNotIn("each served by exactly one", result.stdout.decode())

    def test_judges_coverage_only_once_every_bundle_passed(self):
        # A bundle that failed its own checks has no platforms to count, and
        # its failure is already reported: coverage would only add a second,
        # misleading line about the system it was meant to serve.
        darwin, windows, linux = self.per_os()
        result = self.validate(self.declare(darwin, windows, linux, wrong_hash_for=linux))
        stderr = result.stderr.decode()
        self.assertEqual(result.returncode, 1, stderr)
        self.assertIn("fileSha256 mismatch", stderr)
        self.assertNotIn("no declared bundle serves linux", stderr)
        self.assertNotIn("platforms served", result.stdout.decode())

    def test_refuses_a_per_os_bundle_that_names_a_server_it_does_not_carry(self):
        # A Windows bundle whose manifest still starts the macOS binary, as a
        # derivation that forgot to promote the override would pack it. It
        # replaces the good one per_os wrote under the same name.
        darwin, _, linux = self.per_os()
        path = os.path.join(self.work, "gitlab-mcp-server-windows.mcpb")
        manifest = {"server": {"entry_point": "server/gitlab-mcp-server.exe",
                               "mcp_config": {"command": "${__dirname}/server/gitlab-mcp-server", "args": []}},
                    "compatibility": {"platforms": ["win32"]}}
        with zipfile.ZipFile(path, "w") as archive:
            archive.writestr("manifest.json", json.dumps(manifest))
            archive.writestr("server/gitlab-mcp-server.exe", "exe")
        result = self.validate(self.declare(darwin, path, linux))
        stderr = result.stderr.decode()
        self.assertEqual(result.returncode, 1, stderr)
        self.assertIn("mcp_config: server/gitlab-mcp-server is not in the archive", stderr)


if __name__ == "__main__":
    unittest.main()
