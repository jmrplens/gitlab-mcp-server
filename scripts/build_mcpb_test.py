#!/usr/bin/env python3
"""Tests scripts/build-mcpb.sh, which packs the Claude Desktop bundle and checks what it packed.

After packing, the script reads the archive back and removes a bundle that
fails any of its checks, so that no later step and no developer picks it up.
Two things are tested here. The rules on the packed manifest refuse each shape
that would ship a bundle Claude Desktop cannot start on one of its platforms:
a platform listed with no override (it would be handed the macOS binary, the
base command), a platform left out of the list (Desktop marks the bundle
incompatible there), an override for an unlisted platform, an override
carrying `env` (it replaces the base env and drops `GITLAB_TOKEN`), and a path
the archive does not carry. And every refusal ends with the bundle removed,
including one where an entry never reached the archive, which `zip` allows by
exiting 0 when one of its inputs is missing.

Each case runs the real script from a scratch tree laid out the way it expects,
the repository root with mcpb/ and a dist/ of per-target builds, holding small
stand-ins for the four release binaries.

Run with:

    python3 -m unittest discover -s scripts -p 'build_mcpb_test.py'
"""

import copy
import json
import os
import shutil
import subprocess
import tempfile
import unittest
import zipfile

ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
SCRIPT = os.path.join(ROOT, "scripts", "build-mcpb.sh")
VERSION = "9.8.7"

# The per-target build outputs the script looks for, with the first bytes of
# the format each one is. Nothing here executes them.
BINARIES = {
    "local_darwin_all/gitlab-mcp-server": b"\xcf\xfa\xed\xfe",
    "local_windows_amd64/gitlab-mcp-server.exe": b"MZ",
    "local_linux_amd64/gitlab-mcp-server": b"\x7fELF",
    "local_linux_arm64/gitlab-mcp-server": b"\x7fELF",
}

ENTRIES = [
    "manifest.json",
    "icon.png",
    "server/gitlab-mcp-server",
    "server/gitlab-mcp-server.exe",
    "server/linux/launch.sh",
    "server/linux/gitlab-mcp-server-linux-amd64",
    "server/linux/gitlab-mcp-server-linux-arm64",
]
NOT_EXECUTABLE = {"manifest.json", "icon.png"}

# Stands in for zip and leaves out the entry DROP_ENTRY names, which is what
# zip itself does, with exit status 0, when one of its inputs is missing.
ZIP_DROPPING_AN_ENTRY = """#!/bin/sh
for arg; do
  shift
  [ "$arg" = "$DROP_ENTRY" ] || set -- "$@" "$arg"
done
exec "$REAL_ZIP" "$@"
"""


def without_platform(manifest, platform, keep_override=False):
    manifest["compatibility"]["platforms"].remove(platform)
    if not keep_override:
        manifest["server"]["mcp_config"]["platform_overrides"].pop(platform, None)
    return manifest


def without_override(manifest, platform):
    del manifest["server"]["mcp_config"]["platform_overrides"][platform]
    return manifest


def with_linux_override(manifest, **fields):
    manifest["server"]["mcp_config"]["platform_overrides"]["linux"].update(fields)
    return manifest


def without_platform_list(manifest):
    del manifest["compatibility"]["platforms"]
    return manifest


class BuildMcpbTest(unittest.TestCase):
    """Runs the real build script over stand-in binaries in a scratch tree."""

    def setUp(self):
        missing = [tool for tool in ("bash", "jq", "zip", "unzip") if shutil.which(tool) is None]
        if missing:
            self.skipTest("the build script needs " + ", ".join(missing))
        self.work = tempfile.mkdtemp(prefix="build-mcpb-")
        self.addCleanup(shutil.rmtree, self.work, True)
        os.makedirs(os.path.join(self.work, "mcpb", "linux"))
        shutil.copyfile(os.path.join(ROOT, "mcpb", "icon.png"), os.path.join(self.work, "mcpb", "icon.png"))
        self.launcher = os.path.join(ROOT, "mcpb", "linux", "launch.sh")
        shutil.copyfile(self.launcher, os.path.join(self.work, "mcpb", "linux", "launch.sh"))
        with open(os.path.join(ROOT, "mcpb", "manifest.json"), encoding="utf-8") as fh:
            self.manifest = json.load(fh)
        for rel, magic in BINARIES.items():
            path = os.path.join(self.work, "dist", rel)
            os.makedirs(os.path.dirname(path), exist_ok=True)
            with open(path, "wb") as fh:
                fh.write(magic + rel.encode() * 16)
        self.output = os.path.join(self.work, "dist", "gitlab-mcp-server.mcpb")

    def build(self, manifest=None, drop_entry=None):
        with open(os.path.join(self.work, "mcpb", "manifest.json"), "w", encoding="utf-8") as fh:
            json.dump(self.manifest if manifest is None else manifest, fh, indent=2, ensure_ascii=False)
        env = dict(os.environ)
        if drop_entry is not None:
            bin_dir = os.path.join(self.work, "bin")
            os.makedirs(bin_dir, exist_ok=True)
            wrapper = os.path.join(bin_dir, "zip")
            with open(wrapper, "w", encoding="utf-8") as fh:
                fh.write(ZIP_DROPPING_AN_ENTRY)
            os.chmod(wrapper, 0o755)
            env.update(REAL_ZIP=shutil.which("zip"), DROP_ENTRY=drop_entry,
                       PATH=bin_dir + os.pathsep + env.get("PATH", ""))
        return subprocess.run(
            ["bash", SCRIPT, VERSION, "dist"],
            cwd=self.work,
            env=env,
            capture_output=True,
            timeout=120,
            check=False,
        )

    def assert_refused(self, result, *messages):
        stderr = result.stderr.decode()
        self.assertEqual(result.returncode, 1, stderr)
        for message in messages:
            self.assertIn(message, stderr)
        self.assertIn("was removed", stderr, "the script ended before its removal step")
        self.assertFalse(os.path.exists(self.output), "a refused bundle was left in dist/")

    def test_packs_the_committed_manifest_and_launcher(self):
        result = self.build()
        self.assertEqual(result.returncode, 0, result.stderr.decode())
        with zipfile.ZipFile(self.output) as bundle:
            self.assertEqual(bundle.namelist(), ENTRIES)
            for info in bundle.infolist():
                with self.subTest(entry=info.filename):
                    # Claude Desktop restores the execute bit only from an
                    # owner-execute bit recorded under Unix attributes.
                    self.assertEqual(info.create_system, 3, "not recorded with Unix attributes")
                    expected = 0o100644 if info.filename in NOT_EXECUTABLE else 0o100755
                    self.assertEqual(oct(info.external_attr >> 16), oct(expected))
            packed = json.loads(bundle.read("manifest.json"))
            with open(self.launcher, "rb") as fh:
                self.assertEqual(bundle.read("server/linux/launch.sh"), fh.read())
        self.assertEqual(packed["version"], VERSION)
        expected_manifest = copy.deepcopy(self.manifest)
        expected_manifest["version"] = VERSION
        self.assertEqual(packed, expected_manifest)

    def test_refuses_a_manifest_that_cannot_start_on_one_of_its_platforms(self):
        cases = [
            ("linux listed without its override", lambda m: without_override(m, "linux"),
             ["compatibility.platforms lists linux with no platform_overrides entry"]),
            ("win32 listed without its override", lambda m: without_override(m, "win32"),
             ["compatibility.platforms lists win32 with no platform_overrides entry"]),
            ("linux left out", lambda m: without_platform(m, "linux"),
             ["compatibility.platforms does not list linux"]),
            ("win32 left out", lambda m: without_platform(m, "win32"),
             ["compatibility.platforms does not list win32"]),
            ("darwin left out", lambda m: without_platform(m, "darwin"),
             ["compatibility.platforms does not list darwin"]),
            ("no platform list", without_platform_list,
             ["compatibility.platforms does not list darwin",
              "compatibility.platforms does not list win32",
              "compatibility.platforms does not list linux"]),
            ("an override for an unlisted platform", lambda m: without_platform(m, "linux", keep_override=True),
             ["platform_overrides.linux is not listed in compatibility.platforms"]),
            ("an override carrying env", lambda m: with_linux_override(m, env={"EXTRA": "x"}),
             ["platform_overrides.linux declares env"]),
            ("a launcher the archive lacks",
             lambda m: with_linux_override(m, args=["${__dirname}/server/linux/start.sh"]),
             ["names server/linux/start.sh, which is not in the archive"]),
        ]
        for name, mutate, messages in cases:
            with self.subTest(case=name):
                self.assert_refused(self.build(manifest=mutate(copy.deepcopy(self.manifest))), *messages)

    def test_removes_a_bundle_an_entry_never_reached(self):
        for entry in ("server/linux/launch.sh", "server/linux/gitlab-mcp-server-linux-arm64", "manifest.json"):
            with self.subTest(entry=entry):
                self.assert_refused(
                    self.build(drop_entry=entry),
                    "the archive does not carry exactly the expected entries",
                )


if __name__ == "__main__":
    unittest.main()
