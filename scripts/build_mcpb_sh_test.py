#!/usr/bin/env python3
"""Tests scripts/build-mcpb.sh, which packs the Claude Desktop bundles and checks what it packed.

One run packs four bundles: one per operating system, which server.json
declares, and the universal one, kept under its old name for existing links.
After packing, the script reads each archive back and removes the whole set
when any bundle fails any of its checks, so that no later step and no
developer picks up a bundle that failed, or the rest of a set one failed.
Three things are tested here. Each bundle carries its own system's servers, the
licence and a manifest that lists only the platforms it serves; a per-OS
manifest is the committed one with that platform's command promoted to the
base command. The rules on the packed manifest refuse each shape that would
ship a bundle Claude Desktop cannot start on one of its platforms: a platform
listed with no override (it would be handed the macOS binary, the base
command), a platform left out of the list (Desktop marks the bundle
incompatible there), an override for an unlisted platform, an override
carrying `env` (it replaces the base env and drops `GITLAB_TOKEN`), and a path
the archive does not carry. And every refusal ends with the bundles removed,
including one where an entry never reached the archive, which `zip` allows by
exiting 0 when one of its inputs is missing, and one before anything was
packed, which must not leave the previous run's bundles behind.

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

# The four bundles one run packs, by the target name the script gives each.
TARGETS = ("darwin", "windows", "linux", "universal")


def bundle_name(target):
    if target == "universal":
        return "gitlab-mcp-server.mcpb"
    return "gitlab-mcp-server-" + target + ".mcpb"


SERVER_ENTRIES = {
    "darwin": ["server/gitlab-mcp-server"],
    "windows": ["server/gitlab-mcp-server.exe"],
    "linux": [
        "server/linux/launch.sh",
        "server/linux/gitlab-mcp-server-linux-amd64",
        "server/linux/gitlab-mcp-server-linux-arm64",
    ],
}
SERVER_ENTRIES["universal"] = SERVER_ENTRIES["darwin"] + SERVER_ENTRIES["windows"] + SERVER_ENTRIES["linux"]
ENTRIES = {target: ["manifest.json", "icon.png", "LICENSE"] + SERVER_ENTRIES[target] for target in TARGETS}
NOT_EXECUTABLE = {"manifest.json", "icon.png", "LICENSE"}
# The process.platform value each per-OS bundle serves.
PLATFORM = {"darwin": "darwin", "windows": "win32", "linux": "linux"}

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
            reason = "the build script needs " + ", ".join(missing)
            # A skip keeps the job green, so on a runner without these tools
            # every case here would stop running and nobody would be told.
            # GitHub sets CI on every step; there a missing tool is a failure.
            if os.environ.get("CI"):
                self.fail(reason + ", and CI must run these cases rather than skip them")
            self.skipTest(reason)
        self.work = tempfile.mkdtemp(prefix="build-mcpb-")
        self.addCleanup(shutil.rmtree, self.work, True)
        os.makedirs(os.path.join(self.work, "mcpb", "linux"))
        shutil.copyfile(os.path.join(ROOT, "mcpb", "icon.png"), os.path.join(self.work, "mcpb", "icon.png"))
        shutil.copyfile(os.path.join(ROOT, "mcpb", "platform.jq"), os.path.join(self.work, "mcpb", "platform.jq"))
        self.licence = os.path.join(ROOT, "LICENSE")
        shutil.copyfile(self.licence, os.path.join(self.work, "LICENSE"))
        self.launcher = os.path.join(ROOT, "mcpb", "linux", "launch.sh")
        shutil.copyfile(self.launcher, os.path.join(self.work, "mcpb", "linux", "launch.sh"))
        with open(os.path.join(ROOT, "mcpb", "manifest.json"), encoding="utf-8") as fh:
            self.manifest = json.load(fh)
        self.dist = os.path.join(self.work, "dist")
        self.lay_out_dist(MAKE_MCPB_DIST)
        self.outputs = {target: os.path.join(self.dist, bundle_name(target)) for target in TARGETS}
        self.output = self.outputs["universal"]

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
        """Each bundle's servers are the dist/ files sources maps them to."""
        for target in TARGETS:
            with zipfile.ZipFile(self.outputs[target]) as bundle:
                for entry, rel in sources.items():
                    if entry not in SERVER_ENTRIES[target]:
                        continue
                    with self.subTest(bundle=target, entry=entry):
                        self.assertEqual(bundle.read(entry), stand_in(rel), f"{entry} was not packed from dist/{rel}")

    def install_stand_in(self, name, body, env):
        """Puts a stand-in for the tool name first on env's PATH."""
        bin_dir = os.path.join(self.work, "bin")
        os.makedirs(bin_dir, exist_ok=True)
        wrapper = os.path.join(bin_dir, name)
        with open(wrapper, "w", encoding="utf-8") as fh:
            fh.write(body)
        os.chmod(wrapper, 0o755)
        if not env.get("PATH", "").startswith(bin_dir + os.pathsep):
            env["PATH"] = bin_dir + os.pathsep + env.get("PATH", "")

    def build(self, manifest=None, drop_entry=None):
        with open(os.path.join(self.work, "mcpb", "manifest.json"), "w", encoding="utf-8") as fh:
            json.dump(self.manifest if manifest is None else manifest, fh, indent=2, ensure_ascii=False)
        env = dict(os.environ)
        if drop_entry is not None:
            env.update(REAL_ZIP=shutil.which("zip"), DROP_ENTRY=drop_entry)
            self.install_stand_in("zip", ZIP_DROPPING_AN_ENTRY, env)
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
        for target, output in self.outputs.items():
            self.assertFalse(os.path.exists(output), f"the {target} bundle was left in dist/ after a refusal")

    def assert_bundles(self, sources):
        """Every bundle carries its own entries, modes, licence, launcher and
        manifest, and its servers come from the dist/ files sources names."""
        for target in TARGETS:
            with self.subTest(bundle=target), zipfile.ZipFile(self.outputs[target]) as bundle:
                self.assertEqual(bundle.namelist(), ENTRIES[target])
                for info in bundle.infolist():
                    # Claude Desktop restores the execute bit only from an
                    # owner-execute bit recorded under Unix attributes.
                    self.assertEqual(info.create_system, 3, f"{info.filename} not recorded with Unix attributes")
                    expected = 0o100644 if info.filename in NOT_EXECUTABLE else 0o100755
                    self.assertEqual(oct(info.external_attr >> 16), oct(expected), info.filename)
                with open(self.licence, "rb") as fh:
                    self.assertEqual(bundle.read("LICENSE"), fh.read())
                if "server/linux/launch.sh" in ENTRIES[target]:
                    with open(self.launcher, "rb") as fh:
                        self.assertEqual(bundle.read("server/linux/launch.sh"), fh.read())
                packed = json.loads(bundle.read("manifest.json"))
            self.assert_manifest(target, packed)
        self.assert_packed_from(sources)

    def assert_manifest(self, target, packed):
        """The universal bundle packs the committed manifest; a per-OS bundle
        packs it with that platform's command as the base command, no
        override, the path that command names as its entry point, and only
        that platform listed. Every other field is the committed one."""
        expected = copy.deepcopy(self.manifest)
        expected["version"] = VERSION
        if target != "universal":
            platform = PLATFORM[target]
            config = expected["server"]["mcp_config"]
            override = config.pop("platform_overrides").get(platform, {})
            config.update({key: override[key] for key in ("command", "args") if key in override})
            launch = [config["command"]] + config["args"]
            expected["server"]["entry_point"] = next(
                item[len("${__dirname}/"):] for item in launch if item.startswith("${__dirname}/"))
            expected["compatibility"]["platforms"] = [platform]
        with self.subTest(bundle=target, check="manifest"):
            self.assertEqual(packed, expected)

    def test_packs_each_bundle_from_the_make_mcpb_dist(self):
        result = self.build()
        self.assertEqual(result.returncode, 0, result.stderr.decode())
        self.assert_bundles(MAKE_MCPB_SOURCES)

    def test_each_per_os_bundle_serves_its_own_system_only(self):
        # Spelled out rather than derived, so a change to the derivation that
        # still agrees with assert_manifest's reading of it is caught here.
        expected = {
            "darwin": ("server/gitlab-mcp-server", "${__dirname}/server/gitlab-mcp-server", []),
            "windows": ("server/gitlab-mcp-server.exe", "${__dirname}/server/gitlab-mcp-server.exe", []),
            "linux": ("server/linux/launch.sh", "/bin/sh", ["${__dirname}/server/linux/launch.sh"]),
        }
        result = self.build()
        self.assertEqual(result.returncode, 0, result.stderr.decode())
        for target, (entry_point, command, args) in expected.items():
            with self.subTest(bundle=target), zipfile.ZipFile(self.outputs[target]) as bundle:
                packed = json.loads(bundle.read("manifest.json"))
                self.assertEqual(packed["compatibility"]["platforms"], [PLATFORM[target]])
                self.assertEqual(packed["server"]["entry_point"], entry_point)
                self.assertEqual(packed["server"]["mcp_config"]["command"], command)
                self.assertEqual(packed["server"]["mcp_config"]["args"], args)
                self.assertNotIn("platform_overrides", packed["server"]["mcp_config"])
                self.assertEqual(packed["name"], self.manifest["name"], "a per-OS bundle must install as the same extension")

    def test_packs_each_server_from_the_release_jobs_dist(self):
        # Every staged root copy and every build the bundles do not carry sits
        # beside the four they do, so a discovery pattern that matched one of
        # them too would be refused as a duplicate, and one that matched none
        # of the four would be refused as missing.
        self.lay_out_dist(RELEASE_DIST)
        result = self.build()
        self.assertEqual(result.returncode, 0, result.stderr.decode())
        self.assert_bundles(RELEASE_SOURCES)

    def test_a_refusal_before_packing_removes_the_previous_bundles(self):
        # make mcpb after make release leaves both layouts in dist/, which is
        # the duplicate the first case refuses; the previous run's bundles must
        # not survive the refusal under their old version.
        def with_a_second_linux_amd64_build():
            self.add_to_dist("gitlab-mcp-server_linux_amd64_v1/gitlab-mcp-server")

        def without_the_icon():
            os.remove(os.path.join(self.work, "mcpb", "icon.png"))

        def without_the_licence():
            os.remove(os.path.join(self.work, "LICENSE"))

        def without_the_derivation():
            os.remove(os.path.join(self.work, "mcpb", "platform.jq"))

        cases = [
            ("a binary found twice", with_a_second_linux_amd64_build, "remove the stale ones"),
            ("a missing input", without_the_icon, "mcpb/icon.png not found"),
            ("a missing licence", without_the_licence, "LICENSE not found"),
            ("a missing derivation", without_the_derivation, "mcpb/platform.jq not found"),
        ]
        for name, break_the_tree, message in cases:
            with self.subTest(case=name):
                self.lay_out_dist(MAKE_MCPB_DIST)
                shutil.copyfile(os.path.join(ROOT, "mcpb", "icon.png"), os.path.join(self.work, "mcpb", "icon.png"))
                shutil.copyfile(self.licence, os.path.join(self.work, "LICENSE"))
                shutil.copyfile(os.path.join(ROOT, "mcpb", "platform.jq"), os.path.join(self.work, "mcpb", "platform.jq"))
                first = self.build()
                self.assertEqual(first.returncode, 0, first.stderr.decode())
                for output in self.outputs.values():
                    self.assertTrue(os.path.exists(output), output)
                break_the_tree()
                result = self.build()
                stderr = result.stderr.decode()
                self.assertEqual(result.returncode, 1, stderr)
                self.assertIn(message, stderr)
                for target, output in self.outputs.items():
                    self.assertFalse(os.path.exists(output), f"the previous run's {target} bundle was left in dist/")

    def test_refuses_a_platform_the_universal_bundle_has_no_server_for(self):
        manifest = copy.deepcopy(self.manifest)
        manifest["compatibility"]["platforms"].append("freebsd")
        self.assert_refused(
            self.build(manifest=manifest),
            "compatibility.platforms lists freebsd, although the archive carries no server for it",
        )

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
