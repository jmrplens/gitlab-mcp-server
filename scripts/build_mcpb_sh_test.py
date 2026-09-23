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
exiting 0 when one of its inputs is missing, and one before anything was
packed, which must not leave the previous run's bundle behind.

Each case runs the real script from a scratch tree laid out the way it expects,
the repository root with mcpb/ and a dist/ of per-target builds, holding small
stand-ins for the release binaries, each with bytes of its own. Two dist/
layouts are built: the one `make mcpb` leaves, and the one the release job
builds from, so that a change to the script's discovery patterns that would
pick the wrong file, or none, out of the release job's dist/ fails here and not
in a release.

Run with:

    python3 -m unittest discover -s scripts -p 'build_mcpb_sh_test.py'
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

# The dist/ `make mcpb` leaves: one local_<goos>_<goarch> directory per target
# it cross-compiles, the darwin one holding the lipo'd universal binary.
MAKE_MCPB_DIST = [
    "local_darwin_all/gitlab-mcp-server",
    "local_windows_amd64/gitlab-mcp-server.exe",
    "local_linux_amd64/gitlab-mcp-server",
    "local_linux_arm64/gitlab-mcp-server",
]
# The file of that dist/ each server entry of the bundle is packed from.
MAKE_MCPB_SOURCES = {
    "server/gitlab-mcp-server": "local_darwin_all/gitlab-mcp-server",
    "server/gitlab-mcp-server.exe": "local_windows_amd64/gitlab-mcp-server.exe",
    "server/linux/gitlab-mcp-server-linux-amd64": "local_linux_amd64/gitlab-mcp-server",
    "server/linux/gitlab-mcp-server-linux-arm64": "local_linux_arm64/gitlab-mcp-server",
}

# The dist/ the release job runs the script over, as `ls -1 dist/` printed it
# in the v3.1.0 release rehearsal (run 35228958688), after GoReleaser and the
# step that stages each binary at the root under its release asset name: the
# per-target directories, including the darwin and windows arm64 builds the
# bundle does not carry and the per-arch darwin builds beside the universal
# one, the staged copies with their SBOMs, and GoReleaser's own metadata.
RELEASE_DIST = [
    "artifacts.json",
    "checksums.txt",
    "config.yaml",
    "metadata.json",
    "gitlab-mcp-server-universal_darwin_all/gitlab-mcp-server",
    "gitlab-mcp-server_darwin_amd64_v1/gitlab-mcp-server",
    "gitlab-mcp-server_darwin_arm64_v8.0/gitlab-mcp-server",
    "gitlab-mcp-server_linux_amd64_v1/gitlab-mcp-server",
    "gitlab-mcp-server_linux_arm64_v8.0/gitlab-mcp-server",
    "gitlab-mcp-server_windows_amd64_v1/gitlab-mcp-server.exe",
    "gitlab-mcp-server_windows_arm64_v8.0/gitlab-mcp-server.exe",
] + [
    staged + suffix
    for staged in (
        "gitlab-mcp-server-darwin-all",
        "gitlab-mcp-server-darwin-amd64",
        "gitlab-mcp-server-darwin-arm64",
        "gitlab-mcp-server-linux-amd64",
        "gitlab-mcp-server-linux-arm64",
        "gitlab-mcp-server-windows-amd64.exe",
        "gitlab-mcp-server-windows-arm64.exe",
    )
    for suffix in ("", ".sbom.json")
]
RELEASE_SOURCES = {
    "server/gitlab-mcp-server": "gitlab-mcp-server-universal_darwin_all/gitlab-mcp-server",
    "server/gitlab-mcp-server.exe": "gitlab-mcp-server_windows_amd64_v1/gitlab-mcp-server.exe",
    "server/linux/gitlab-mcp-server-linux-amd64": "gitlab-mcp-server_linux_amd64_v1/gitlab-mcp-server",
    "server/linux/gitlab-mcp-server-linux-arm64": "gitlab-mcp-server_linux_arm64_v8.0/gitlab-mcp-server",
}


def stand_in(rel):
    """The bytes the scratch dist/ holds at rel: different for every file, so
    a bundle packed from the wrong one is told apart. Nothing executes them."""
    return b"stand-in for dist/" + rel.encode() + b"\n" * 64

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
        self.dist = os.path.join(self.work, "dist")
        self.lay_out_dist(MAKE_MCPB_DIST)
        self.output = os.path.join(self.dist, "gitlab-mcp-server.mcpb")

    def lay_out_dist(self, files):
        """Replaces dist/ with the given files, each holding its stand-in bytes."""
        shutil.rmtree(self.dist, ignore_errors=True)
        for rel in files:
            self.add_to_dist(rel)

    def add_to_dist(self, rel):
        """Writes one file into dist/ and leaves everything else there alone."""
        path = os.path.join(self.dist, rel)
        os.makedirs(os.path.dirname(path), exist_ok=True)
        with open(path, "wb") as fh:
            fh.write(stand_in(rel))

    def assert_packed_from(self, sources):
        with zipfile.ZipFile(self.output) as bundle:
            for entry, rel in sources.items():
                with self.subTest(entry=entry):
                    self.assertEqual(bundle.read(entry), stand_in(rel), f"{entry} was not packed from dist/{rel}")

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
        self.assert_packed_from(MAKE_MCPB_SOURCES)

    def test_packs_each_server_from_the_release_jobs_dist(self):
        # Every staged root copy and every build the bundle does not carry sits
        # beside the four it does, so a discovery pattern that matched one of
        # them too would be refused as a duplicate, and one that matched none
        # of the four would be refused as missing.
        self.lay_out_dist(RELEASE_DIST)
        result = self.build()
        self.assertEqual(result.returncode, 0, result.stderr.decode())
        with zipfile.ZipFile(self.output) as bundle:
            self.assertEqual(bundle.namelist(), ENTRIES)
        self.assert_packed_from(RELEASE_SOURCES)

    def test_a_refusal_before_packing_removes_the_previous_bundle(self):
        # make mcpb after make release leaves both layouts in dist/, which is
        # the duplicate the first case refuses; the previous run's bundle must
        # not survive the refusal under its old version.
        def with_a_second_linux_amd64_build():
            self.add_to_dist("gitlab-mcp-server_linux_amd64_v1/gitlab-mcp-server")

        def without_the_icon():
            os.remove(os.path.join(self.work, "mcpb", "icon.png"))

        cases = [
            ("a binary found twice", with_a_second_linux_amd64_build, "remove the stale ones"),
            ("a missing input", without_the_icon, "mcpb/icon.png not found"),
        ]
        for name, break_the_tree, message in cases:
            with self.subTest(case=name):
                self.lay_out_dist(MAKE_MCPB_DIST)
                shutil.copyfile(os.path.join(ROOT, "mcpb", "icon.png"), os.path.join(self.work, "mcpb", "icon.png"))
                first = self.build()
                self.assertEqual(first.returncode, 0, first.stderr.decode())
                self.assertTrue(os.path.exists(self.output))
                break_the_tree()
                result = self.build()
                stderr = result.stderr.decode()
                self.assertEqual(result.returncode, 1, stderr)
                self.assertIn(message, stderr)
                self.assertFalse(os.path.exists(self.output), "the previous run's bundle was left in dist/")

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
