#!/usr/bin/env python3
# Copyright (c) 2026 José M. Requena Plens
# SPDX-License-Identifier: MIT

"""Tests scripts/publish-nuget.sh against a stand-in dotnet.

The cases held the script inside the suite of the Python packer and
validator, which cmd/release_packages replaced; they stay with the script,
which stays a shell wrapper around `dotnet nuget push`. They run the script
with --no-assemble over a directory of the seven package files named as the
nuget subcommand names them, with a stand-in dotnet first on PATH that
records every call.

Run with:

    python3 -m unittest discover -s scripts -p 'publish_nuget_sh_test.py'
"""

import os
import shutil
import subprocess
import tempfile
import unittest

ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
PUBLISHER = os.path.join(ROOT, "scripts", "publish-nuget.sh")
VERSION = "9.8.7"
RIDS = ["linux-x64", "linux-arm64", "osx-x64", "osx-arm64", "win-x64", "win-arm64"]


class PublishNugetTest(unittest.TestCase):
    """The runtime packages are pushed before the pointer, with the key alone."""

    def setUp(self):
        if shutil.which("bash") is None:
            if os.environ.get("CI"):
                self.fail("the script needs bash, and CI must run these cases rather than skip them")
            self.skipTest("the script needs bash")
        self.work = tempfile.mkdtemp(prefix="publish-nuget-")
        self.addCleanup(shutil.rmtree, self.work, True)
        self.out = os.path.join(self.work, "out")
        self.bin = os.path.join(self.work, "bin")
        os.makedirs(self.bin)
        self.log = os.path.join(self.work, "dotnet.log")
        fake = os.path.join(self.bin, "dotnet")
        with open(fake, "w", encoding="utf-8") as fh:
            fh.write('#!/bin/sh\nprintf \'%s\\n\' "$*" >> "{}"\n'.format(self.log))
        os.chmod(fake, 0o755)
        self.pack()

    def package(self, rid=None):
        name = "gitlab-mcp-server.{}.nupkg".format(VERSION) if rid is None else \
            "gitlab-mcp-server.{}.{}.nupkg".format(rid, VERSION)
        return os.path.join(self.out, name)

    def pack(self):
        os.makedirs(self.out, exist_ok=True)
        for rid in RIDS + [None]:
            with open(self.package(rid), "wb") as fh:
                fh.write(b"a package")

    def run_publish(self, *flags, key=None):
        env = {**os.environ, "PATH": self.bin + os.pathsep + os.environ["PATH"], "NUGET_OUT": self.out}
        env.pop("NUGET_API_KEY", None)
        env.pop("RELEASE_PACKAGES", None)
        if key is not None:
            env["NUGET_API_KEY"] = key
        return subprocess.run(
            ["bash", PUBLISHER, os.path.join(self.work, "dist"), VERSION, *flags],
            cwd=ROOT, capture_output=True, text=True, check=False, env=env,
        )

    def pushes(self):
        if not os.path.exists(self.log):
            return []
        with open(self.log, encoding="utf-8") as fh:
            return [line.split() for line in fh.read().splitlines() if line.startswith("nuget push")]

    def test_pushes_runtime_packages_before_the_pointer(self):
        result = self.run_publish("--no-assemble", key="secret-key")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        pushed = [os.path.basename(args[2]) for args in self.pushes()]
        want = ["gitlab-mcp-server.{}.{}.nupkg".format(rid, VERSION) for rid in RIDS]
        want.append("gitlab-mcp-server.{}.nupkg".format(VERSION))
        self.assertEqual(pushed, want)
        for args in self.pushes():
            with self.subTest(args[2]):
                self.assertIn("--skip-duplicate", args)
                self.assertIn("secret-key", args)
                self.assertIn("https://api.nuget.org/v3/index.json", args)

    def test_a_dry_run_pushes_nothing_and_needs_no_key(self):
        result = self.run_publish("--no-assemble", "--dry-run")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.pushes(), [])
        self.assertIn("nothing pushed", result.stdout)

    def test_refuses_to_push_without_a_key(self):
        result = self.run_publish("--no-assemble")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("NUGET_API_KEY", result.stderr)
        self.assertEqual(self.pushes(), [])

    def test_assembling_needs_the_release_packages_tool(self):
        result = self.run_publish("--dry-run")
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn("set RELEASE_PACKAGES to a build of cmd/release_packages", result.stderr)
        self.assertEqual(self.pushes(), [])

    def test_refuses_a_stale_tree(self):
        cases = [
            ("a package is missing", lambda: os.remove(self.package("win-x64")), "is missing"),
            ("another version sits beside the release",
             lambda: shutil.copy(self.package(), os.path.join(self.out, "gitlab-mcp-server.0.0.1.nupkg")),
             "not version"),
        ]
        for name, tamper, want in cases:
            with self.subTest(name):
                shutil.rmtree(self.out, ignore_errors=True)
                self.pack()
                tamper()
                result = self.run_publish("--no-assemble", "--dry-run")
                self.assertNotEqual(result.returncode, 0)
                self.assertIn(want, result.stderr)
                self.assertEqual(self.pushes(), [])


if __name__ == "__main__":
    unittest.main()
