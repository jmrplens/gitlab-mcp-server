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

Run with:

    python3 -m unittest discover -s scripts -p 'mcpb_manifest_test.py'
"""

import json
import os
import unittest

ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
MANIFEST = os.path.join(ROOT, "mcpb", "manifest.json")

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


if __name__ == "__main__":
    unittest.main()
