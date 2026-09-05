#!/usr/bin/env python3
"""Tests scripts/build_nuget.py, scripts/validate_nuget.py and
scripts/publish-nuget.sh against packages packed from fake binaries.

A pushed NuGet version can never be replaced, so the validator is the gate
that decides what reaches the registry, and the publish script decides in
which order. Both are exercised here without a .NET SDK: the packer needs
none, the validator's offline checks need none, and the publish script is run
against a fake `dotnet` on PATH that records what it was asked to push.

The fake binaries carry the header bytes the validator inspects (ELF machine,
Mach-O cputype, PE machine) so a case that tampers with one of them fails for
the reason the case names and not for a header the fixture never had.

Run with:

    python3 -m unittest discover -s scripts -p 'validate_nuget_test.py'
"""

import hashlib
import importlib.util
import json
import os
import shutil
import stat
import struct
import subprocess
import sys
import tempfile
import unittest
import zipfile

ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
BUILDER = os.path.join(ROOT, "scripts", "build_nuget.py")
PUBLISHER = os.path.join(ROOT, "scripts", "publish-nuget.sh")


def load_module(name):
    """Import a script by path; scripts/ is not a package."""
    spec = importlib.util.spec_from_file_location(name, os.path.join(ROOT, "scripts", name + ".py"))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


validate_nuget = load_module("validate_nuget")
build_nuget = load_module("build_nuget")

VERSION = "9.9.9"
RIDS = build_nuget.RIDS


def fake_binary(plat_key, tail=b""):
    """Bytes with the header the validator checks for this platform."""
    if plat_key.startswith("linux"):
        machine = 0x3E if plat_key.endswith("amd64") else 0xB7
        head = b"\x7fELF" + b"\0" * 14 + struct.pack("<H", machine)
    elif plat_key.startswith("darwin"):
        cputype = 0x01000007 if plat_key.endswith("amd64") else 0x0100000C
        head = b"\xcf\xfa\xed\xfe" + struct.pack("<I", cputype)
    else:
        machine = 0x8664 if plat_key.endswith("amd64") else 0xAA64
        head = b"MZ" + b"\0" * (0x3C - 2) + struct.pack("<I", 0x40)
        head += b"\0" * (0x40 - len(head)) + b"PE\0\0" + struct.pack("<H", machine)
    return head + (plat_key.encode() * 64) + tail


def asset_name(plat_key):
    return "gitlab-mcp-server-" + plat_key + (".exe" if plat_key.startswith("windows") else "")


def write_fixture(binaries_dir, with_checksums=True):
    """The six release assets and, optionally, a matching checksums.txt."""
    os.makedirs(binaries_dir, exist_ok=True)
    lines = []
    for plat_key in RIDS:
        payload = fake_binary(plat_key)
        with open(os.path.join(binaries_dir, asset_name(plat_key)), "wb") as fh:
            fh.write(payload)
        lines.append("{}  {}".format(hashlib.sha256(payload).hexdigest(), asset_name(plat_key)))
    if with_checksums:
        with open(os.path.join(binaries_dir, "checksums.txt"), "w", encoding="utf-8") as fh:
            fh.write("\n".join(lines) + "\n")


def repack(path, replace=None, drop=()):
    """Rewrite a .nupkg with some entries replaced or dropped, keeping the
    other entries' bytes and modes."""
    replace = replace or {}
    tmp = path + ".tmp"
    with zipfile.ZipFile(path) as src, zipfile.ZipFile(tmp, "w") as dst:
        for info in src.infolist():
            if info.filename in drop:
                continue
            data = replace.get(info.filename, src.read(info.filename))
            if isinstance(data, str):
                data = data.encode("utf-8")
            dst.writestr(info, data)
    os.replace(tmp, path)


class PackedFixture(unittest.TestCase):
    """A scratch binaries dir, packed once per test into a scratch output dir."""

    def setUp(self):
        self.work = tempfile.mkdtemp(prefix="validate-nuget-")
        self.addCleanup(shutil.rmtree, self.work, True)
        self.binaries = os.path.join(self.work, "dist")
        self.out = os.path.join(self.work, "out")
        write_fixture(self.binaries)
        self.pack()
        # The real floor is 20 MiB; the fixtures are a few KB.
        self.addCleanup(setattr, validate_nuget, "MIN_BINARY_BYTES", validate_nuget.MIN_BINARY_BYTES)
        validate_nuget.MIN_BINARY_BYTES = 64

    def pack(self, *extra):
        result = subprocess.run(
            [sys.executable, BUILDER, "--binaries", self.binaries, "--version", VERSION, "--out", self.out, *extra],
            cwd=ROOT, capture_output=True, text=True, check=False,
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        return result

    def package(self, rid=None):
        if rid is None:
            return os.path.join(self.out, "{}.{}.nupkg".format(build_nuget.PKG_ID, VERSION))
        return os.path.join(self.out, "{}.{}.{}.nupkg".format(build_nuget.PKG_ID, rid, VERSION))

    def problems(self):
        return validate_nuget.validate_packages(self.out, VERSION)


class BuildNugetTest(PackedFixture):
    """What the packer emits, checked by opening the packages directly."""

    def test_emits_seven_packages_and_the_manifest(self):
        names = sorted(os.listdir(self.out))
        want = sorted(list(validate_nuget.expected_packages(VERSION)) + ["verified-binaries.json"])
        self.assertEqual(names, want)
        with open(os.path.join(self.out, "verified-binaries.json"), encoding="utf-8") as fh:
            manifest = json.load(fh)
        self.assertTrue(manifest["verified"])
        self.assertEqual(manifest["version"], VERSION)
        self.assertEqual(sorted(manifest["binaries"]), sorted(RIDS))

    def test_pointer_layout(self):
        with zipfile.ZipFile(self.package()) as zf:
            names = zf.namelist()
            self.assertIn("tools/net10.0/any/DotnetToolSettings.xml", names)
            self.assertIn(".mcp/server.json", names)
            self.assertIn("README.md", names)
            doc = json.loads(zf.read(".mcp/server.json"))
            self.assertEqual(doc["version"], VERSION)
            self.assertEqual(doc["packages"][0]["identifier"], build_nuget.PKG_ID)
            self.assertEqual(doc["packages"][0]["version"], VERSION)
            self.assertEqual(doc["packages"][0]["registryType"], "nuget")
            settings = zf.read("tools/net10.0/any/DotnetToolSettings.xml").decode()
            for rid in RIDS.values():
                self.assertIn('RuntimeIdentifier="{}" Id="gitlab-mcp-server.{}"'.format(rid, rid), settings)
            nuspec = zf.read("gitlab-mcp-server.nuspec").decode()
            self.assertIn('<packageType name="DotnetTool" />', nuspec)
            self.assertIn('<packageType name="McpServer" />', nuspec)
            self.assertIn("<readme>README.md</readme>", nuspec)

    def test_rid_package_layout(self):
        cases = [(plat_key, rid) for plat_key, rid in RIDS.items()]
        for plat_key, rid in cases:
            with self.subTest(rid):
                bin_name = "gitlab-mcp-server" + (".exe" if plat_key.startswith("windows") else "")
                with zipfile.ZipFile(self.package(rid)) as zf:
                    arc = "tools/any/{}/{}".format(rid, bin_name)
                    self.assertIn(arc, zf.namelist())
                    info = zf.getinfo(arc)
                    mode = info.external_attr >> 16
                    self.assertTrue(stat.S_ISREG(mode), "binary entry must be a regular file")
                    self.assertTrue(mode & 0o111, "binary entry must carry the executable bit")
                    self.assertEqual(zf.read(arc), fake_binary(plat_key))
                    settings = zf.read("tools/any/{}/DotnetToolSettings.xml".format(rid)).decode()
                    self.assertIn('EntryPoint="{}" Runner="executable"'.format(bin_name), settings)
                    nuspec = zf.read("gitlab-mcp-server.{}.nuspec".format(rid)).decode()
                    self.assertIn('<packageType name="DotnetToolRidPackage" />', nuspec)

    def test_is_deterministic(self):
        first = {}
        for name in os.listdir(self.out):
            with open(os.path.join(self.out, name), "rb") as fh:
                first[name] = hashlib.sha256(fh.read()).hexdigest()
        self.pack()
        for name, digest in first.items():
            with self.subTest(name):
                with open(os.path.join(self.out, name), "rb") as fh:
                    self.assertEqual(hashlib.sha256(fh.read()).hexdigest(), digest)


class ValidateNugetTest(PackedFixture):
    """The offline checks: a clean build passes, and each tampering is named."""

    def test_a_clean_build_passes(self):
        self.assertEqual(self.problems(), [])
        self.assertFalse(os.path.exists(os.path.join(self.out, "verified-binaries.json")),
                         "the manifest is consumed so the publish step never pushes it")

    def test_each_defect_is_reported(self):
        pointer_settings = "tools/net10.0/any/DotnetToolSettings.xml"

        def missing_rid_package():
            os.remove(self.package("osx-arm64"))

        def swapped_binary():
            repack(self.package("linux-x64"),
                   {"tools/any/linux-x64/gitlab-mcp-server": fake_binary("linux-amd64", b"swapped")})

        def wrong_machine():
            repack(self.package("linux-arm64"),
                   {"tools/any/linux-arm64/gitlab-mcp-server": fake_binary("linux-amd64")})

        def lost_token():
            repack(self.package(), {"README.md": "# no token here\n"})

        def glued_token():
            repack(self.package(), {"README.md": "mcp-name: io.github.jmrplens/gitlab-mcp-server-other\n"})

        def pointer_forgets_a_rid():
            with zipfile.ZipFile(self.package()) as zf:
                settings = zf.read(pointer_settings).decode()
            settings = settings.replace(
                '    <RuntimeIdentifierPackage RuntimeIdentifier="win-arm64" Id="gitlab-mcp-server.win-arm64" />\n', "")
            repack(self.package(), {pointer_settings: settings})

        def stale_server_json():
            with zipfile.ZipFile(self.package()) as zf:
                doc = json.loads(zf.read(".mcp/server.json"))
            doc["version"] = "0.0.1"
            repack(self.package(), {".mcp/server.json": json.dumps(doc)})

        def leftover_file():
            with open(os.path.join(self.out, "notes.txt"), "w", encoding="utf-8") as fh:
                fh.write("scratch\n")

        def unverified_build():
            os.remove(os.path.join(self.binaries, "checksums.txt"))
            self.pack("--allow-unverified")

        def executable_bit_lost():
            path = self.package("osx-x64")
            tmp = path + ".tmp"
            with zipfile.ZipFile(path) as src, zipfile.ZipFile(tmp, "w") as dst:
                for info in src.infolist():
                    data = src.read(info.filename)
                    if info.filename.endswith("/gitlab-mcp-server"):
                        info.external_attr = 0o100644 << 16
                    dst.writestr(info, data)
            os.replace(tmp, path)

        cases = [
            ("a runtime package is missing", missing_rid_package, "needs exactly"),
            ("a binary was swapped after verification", swapped_binary, "signed checksums.txt named"),
            ("a binary is for another machine", wrong_machine, "ELF machine"),
            ("the README lost the ownership token", lost_token, "ownership token"),
            ("the token is glued to a longer name", glued_token, "ownership token"),
            ("the pointer forgets a runtime identifier", pointer_forgets_a_rid, "RuntimeIdentifierPackages"),
            (".mcp/server.json carries another version", stale_server_json, ".mcp/server.json carries version"),
            ("a stray file sits beside the packages", leftover_file, "would try to push"),
            ("the build skipped verification", unverified_build, "--allow-unverified"),
            ("the executable bit was lost", executable_bit_lost, "executable bit"),
        ]
        for name, tamper, want in cases:
            with self.subTest(name):
                shutil.rmtree(self.out, ignore_errors=True)
                write_fixture(self.binaries)
                self.pack()
                tamper()
                problems = self.problems()
                self.assertTrue(problems, "the defect went unreported")
                self.assertTrue(any(want in p for p in problems), problems)


class PublishNugetTest(PackedFixture):
    """The publish script's ordering and its refusal to push a stale tree,
    driven against a fake dotnet that records every invocation."""

    def setUp(self):
        super().setUp()
        self.bin = os.path.join(self.work, "bin")
        os.makedirs(self.bin)
        self.log = os.path.join(self.work, "dotnet.log")
        fake = os.path.join(self.bin, "dotnet")
        with open(fake, "w", encoding="utf-8") as fh:
            fh.write('#!/bin/sh\nprintf \'%s\\n\' "$*" >> "{}"\n'.format(self.log))
        os.chmod(fake, 0o755)
        # The validator consumed the manifest; the publish script must not
        # need it back, since the workflow runs the two in separate steps.
        self.assertEqual(self.problems(), [])

    def run_publish(self, *flags, key=None):
        env = {**os.environ, "PATH": self.bin + os.pathsep + os.environ["PATH"], "NUGET_OUT": self.out}
        env.pop("NUGET_API_KEY", None)
        if key is not None:
            env["NUGET_API_KEY"] = key
        return subprocess.run(
            ["bash", PUBLISHER, self.binaries, VERSION, *flags],
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
        want = ["gitlab-mcp-server.{}.{}.nupkg".format(rid, VERSION) for rid in RIDS.values()]
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
                self.problems()
                tamper()
                result = self.run_publish("--no-assemble", "--dry-run")
                self.assertNotEqual(result.returncode, 0)
                self.assertIn(want, result.stderr)
                self.assertEqual(self.pushes(), [])


if __name__ == "__main__":
    unittest.main()
