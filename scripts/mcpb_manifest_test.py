#!/usr/bin/env python3
"""Tests mcpb/manifest.json, the Claude Desktop bundle's manifest, for what its schema cannot say.

`make check-mcpb` validates the manifest against the MCPB schema, and the
schema accepts a manifest that lists a platform nothing can start on. Claude
Desktop (read from its 2.7032.0 Linux build) marks a bundle incompatible on a
platform `compatibility.platforms` leaves out, takes `platform_overrides`
keyed by `process.platform`, and otherwise starts the base command, which here
is the macOS universal binary. It also applies an override's `env` and `args`
in place of the base ones rather than merging them. So a manifest that drops
the `linux` entry, or keeps `linux` listed with no override, or gives an
override `env`, passes the schema and ships a bundle that is refused, starts a
Mach-O binary, or starts the server without `GITLAB_TOKEN`. These tests pin the
values the bundle's layout depends on.

The committed manifest describes the universal bundle. The three per-OS
bundles carry a manifest derived from it by mcpb/platform.jq, and the second
class here holds that derivation to what each of them needs: only its own
platform listed, its own command as the base command, no override, and an
entry point inside the bundle. It also holds the derivation's two refusals,
which no bundle the build packs can reach: a platform Claude Desktop does not
report, and a command line that names no path inside the bundle.

Run with:

    python3 -m unittest discover -s scripts -p 'mcpb_manifest_test.py'
"""

import json
import os
import shutil
import subprocess
import unittest

ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
MANIFEST = os.path.join(ROOT, "mcpb", "manifest.json")
PLATFORM_JQ = os.path.join(ROOT, "mcpb", "platform.jq")

# Where scripts/build-mcpb.sh puts the launcher, relative to the extension
# directory, and where that file comes from in this tree.
LAUNCHER_ENTRY = "server/linux/launch.sh"
LAUNCHER_SOURCE = os.path.join(ROOT, "mcpb", "linux", "launch.sh")


class McpbManifestTest(unittest.TestCase):
    """Reads the committed manifest, not a packed copy of it."""

    @classmethod
    def setUpClass(cls):
        with open(MANIFEST, encoding="utf-8") as fh:
            cls.manifest = json.load(fh)
        cls.config = cls.manifest["server"]["mcp_config"]
        cls.overrides = cls.config.get("platform_overrides") or {}

    def test_lists_exactly_the_three_platforms_the_bundle_serves(self):
        platforms = self.manifest["compatibility"]["platforms"]
        self.assertEqual(sorted(platforms), ["darwin", "linux", "win32"])
        self.assertEqual(len(platforms), len(set(platforms)), "a platform is listed twice")

    def test_every_listed_platform_but_darwin_has_an_override(self):
        # The base command is the macOS universal binary, so any other listed
        # platform without an override would be handed a Mach-O file.
        for platform in self.manifest["compatibility"]["platforms"]:
            if platform == "darwin":
                continue
            with self.subTest(platform=platform):
                self.assertIn(platform, self.overrides)

    def test_every_override_is_a_listed_platform(self):
        for platform in self.overrides:
            with self.subTest(platform=platform):
                self.assertIn(platform, self.manifest["compatibility"]["platforms"])

    def test_the_base_command_is_the_macos_universal_binary_with_no_args(self):
        self.assertEqual(self.config["command"], "${__dirname}/server/gitlab-mcp-server")
        # Empty, so the linux override's args replace nothing.
        self.assertEqual(self.config["args"], [])

    def test_linux_runs_the_launcher_through_bin_sh(self):
        self.assertEqual(
            self.overrides.get("linux"),
            {"command": "/bin/sh", "args": ["${__dirname}/" + LAUNCHER_ENTRY]},
        )

    def test_windows_runs_the_amd64_executable(self):
        self.assertEqual(self.overrides.get("win32"), {"command": "${__dirname}/server/gitlab-mcp-server.exe"})

    def test_no_override_declares_env(self):
        # An override's env replaces the base env, which carries GITLAB_TOKEN.
        for platform, override in self.overrides.items():
            with self.subTest(platform=platform):
                self.assertNotIn("env", override)
        self.assertIn("GITLAB_TOKEN", self.config["env"])
        self.assertIn("GITLAB_URL", self.config["env"])

    def test_the_launcher_the_linux_override_names_is_in_the_tree(self):
        # scripts/build-mcpb.sh packs this file as the entry the override
        # names; build_mcpb_sh_test.py checks that the packed bytes are these.
        self.assertTrue(os.path.isfile(LAUNCHER_SOURCE), LAUNCHER_SOURCE)


def derive(platform, manifest=None):
    """Runs mcpb/platform.jq for platform over manifest (the committed one by
    default) and returns jq's completed process."""
    if manifest is None:
        with open(MANIFEST, encoding="utf-8") as fh:
            manifest = json.load(fh)
    return subprocess.run(
        ["jq", "--arg", "platform", platform, "-f", PLATFORM_JQ],
        input=json.dumps(manifest).encode(),
        capture_output=True,
        timeout=30,
        check=False,
    )


class McpbPlatformManifestTest(unittest.TestCase):
    """Reads what mcpb/platform.jq derives from the committed manifest."""

    def setUp(self):
        if shutil.which("jq") is None:
            # A skip keeps the job green, so on a runner without jq every case
            # here would stop running and nobody would be told. GitHub sets CI
            # on every step; there a missing tool is a failure.
            if os.environ.get("CI"):
                self.fail("the derivation needs jq, and CI must run these cases rather than skip them")
            self.skipTest("the derivation needs jq")
        with open(MANIFEST, encoding="utf-8") as fh:
            self.manifest = json.load(fh)

    def derived(self, platform):
        result = derive(platform)
        self.assertEqual(result.returncode, 0, result.stderr.decode())
        return json.loads(result.stdout)

    def test_each_platform_gets_its_own_command_and_nothing_else(self):
        cases = {
            "darwin": ("${__dirname}/server/gitlab-mcp-server", [], "server/gitlab-mcp-server"),
            "win32": ("${__dirname}/server/gitlab-mcp-server.exe", [], "server/gitlab-mcp-server.exe"),
            "linux": ("/bin/sh", ["${__dirname}/" + LAUNCHER_ENTRY], LAUNCHER_ENTRY),
        }
        for platform, (command, args, entry_point) in cases.items():
            with self.subTest(platform=platform):
                derived = self.derived(platform)
                config = derived["server"]["mcp_config"]
                self.assertEqual(derived["compatibility"]["platforms"], [platform])
                self.assertEqual(config["command"], command)
                self.assertEqual(config["args"], args)
                self.assertNotIn("platform_overrides", config)
                self.assertEqual(derived["server"]["entry_point"], entry_point)
                # The env is the base env, whole: an override never carried one.
                self.assertEqual(config["env"], self.manifest["server"]["mcp_config"]["env"])

    def test_everything_but_the_launch_and_the_platforms_is_the_committed_manifest(self):
        # The same name above all: the per-OS bundles are one extension, so
        # installing one replaces the universal bundle rather than adding a
        # second copy beside it.
        for platform in ("darwin", "win32", "linux"):
            with self.subTest(platform=platform):
                derived = self.derived(platform)
                for key in set(self.manifest) | set(derived):
                    if key in ("server", "compatibility"):
                        continue
                    self.assertEqual(derived.get(key), self.manifest.get(key), key)
                self.assertEqual(
                    {k: v for k, v in derived["compatibility"].items() if k != "platforms"},
                    {k: v for k, v in self.manifest["compatibility"].items() if k != "platforms"},
                )
                self.assertEqual(
                    {k: v for k, v in derived["server"].items() if k not in ("entry_point", "mcp_config")},
                    {k: v for k, v in self.manifest["server"].items() if k not in ("entry_point", "mcp_config")},
                )

    def test_refuses_a_platform_claude_desktop_does_not_report(self):
        for platform in ("windows", "macos", "freebsd", ""):
            with self.subTest(platform=platform):
                result = derive(platform)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("platform must be darwin, win32 or linux", result.stderr.decode())
                self.assertEqual(result.stdout, b"")

    def test_refuses_a_command_line_that_names_nothing_inside_the_bundle(self):
        manifest = json.loads(json.dumps(self.manifest))
        manifest["server"]["mcp_config"]["platform_overrides"]["linux"] = {"command": "/usr/bin/gitlab-mcp-server"}
        result = derive("linux", manifest)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("the linux command line names no path inside the bundle", result.stderr.decode())

    def test_an_override_without_args_keeps_the_base_args(self):
        # The universal manifest's base args are empty and the Windows override
        # names none, so this is the case where the base args survive.
        manifest = json.loads(json.dumps(self.manifest))
        manifest["server"]["mcp_config"]["args"] = ["--log-level", "debug"]
        result = derive("win32", manifest)
        self.assertEqual(result.returncode, 0, result.stderr.decode())
        self.assertEqual(json.loads(result.stdout)["server"]["mcp_config"]["args"], ["--log-level", "debug"])


if __name__ == "__main__":
    unittest.main()
