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
import io
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

# A stand-in for what cmd/gen_third_party_notices writes: the validator reads
# its header line and its digest, nothing else.
NOTICES = "THIRD_PARTY_NOTICES"
NOTICES_TEXT = b"Third-party notices for gitlab-mcp-server\n\nstand-in body\n"


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
    """The six release assets, the notices and, optionally, a matching
    checksums.txt."""
    os.makedirs(binaries_dir, exist_ok=True)
    lines = []
    for plat_key in RIDS:
        payload = fake_binary(plat_key)
        with open(os.path.join(binaries_dir, asset_name(plat_key)), "wb") as fh:
            fh.write(payload)
        lines.append("{}  {}".format(hashlib.sha256(payload).hexdigest(), asset_name(plat_key)))
    with open(os.path.join(binaries_dir, NOTICES), "wb") as fh:
        fh.write(NOTICES_TEXT)
    lines.append("{}  {}".format(hashlib.sha256(NOTICES_TEXT).hexdigest(), NOTICES))
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


class Unseekable(io.RawIOBase):
    """A write-only stream that can say where it is but cannot seek, which is
    what makes zipfile write every entry with a trailing data descriptor."""

    def __init__(self):
        super().__init__()
        self.buffer = io.BytesIO()

    def writable(self):
        return True

    def write(self, data):
        return self.buffer.write(data)

    def tell(self):
        return self.buffer.tell()

    def seek(self, *args):
        raise OSError("not seekable")


def rewrite(path, stream_data_descriptors=False, zip64_entries=(), comment=b""):
    """Rewrite a .nupkg entry by entry, keeping names, modes and contents, in
    one of the layouts nuget.org's signing would not leave intact."""
    sink = Unseekable() if stream_data_descriptors else io.BytesIO()
    with zipfile.ZipFile(path) as src, zipfile.ZipFile(sink, "w") as dst:
        for info in src.infolist():
            # Read before writing: dst.open rewrites the ZipInfo it is given,
            # and src looks the entry up through that same object.
            data = src.read(info.filename)
            with dst.open(info, "w", force_zip64=info.filename in zip64_entries) as fh:
                fh.write(data)
        dst.comment = comment
    blob = sink.buffer.getvalue() if stream_data_descriptors else sink.getvalue()
    with open(path, "wb") as fh:
        fh.write(blob)


def sign_like_nuget(path):
    """Append a stored .signature.p7s the way nuget.org signs a package: a
    local entry after the last one and a central record after the last one,
    every byte before them unchanged (zipfile's append mode does exactly that)."""
    with zipfile.ZipFile(path, "a") as zf:
        info = zipfile.ZipInfo(".signature.p7s", date_time=(2026, 1, 1, 0, 0, 0))
        info.compress_type = zipfile.ZIP_STORED
        zf.writestr(info, b"0\x82signature")


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

    def forget_notices_digest(self):
        """Rewrite the manifest the packer left without the notices digest."""
        path = os.path.join(self.out, "verified-binaries.json")
        with open(path, encoding="utf-8") as fh:
            manifest = json.load(fh)
        del manifest["notices"]
        with open(path, "w", encoding="utf-8") as fh:
            json.dump(manifest, fh)

    def relicense_mode(self, rid, mode):
        """Rewrite one package with its LICENSE entry carrying `mode`."""
        path = self.package(rid)
        tmp = path + ".tmp"
        with zipfile.ZipFile(path) as src, zipfile.ZipFile(tmp, "w") as dst:
            for info in src.infolist():
                data = src.read(info.filename)
                if info.filename == "LICENSE":
                    info.external_attr = mode << 16
                dst.writestr(info, data)
        os.replace(tmp, path)


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
        self.assertEqual(manifest["notices"], hashlib.sha256(NOTICES_TEXT).hexdigest())

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

    def test_every_package_carries_the_licence(self):
        with open(os.path.join(ROOT, "LICENSE"), "rb") as fh:
            want = fh.read()
        for rid in [None] + list(RIDS.values()):
            with self.subTest(rid or "pointer"):
                with zipfile.ZipFile(self.package(rid)) as zf:
                    self.assertEqual(zf.read("LICENSE"), want)
                    self.assertEqual(zf.getinfo("LICENSE").external_attr >> 16, 0o100644)

    def test_every_package_carries_the_notices(self):
        for rid in [None] + list(RIDS.values()):
            with self.subTest(rid or "pointer"):
                with zipfile.ZipFile(self.package(rid)) as zf:
                    self.assertEqual(zf.read(NOTICES), NOTICES_TEXT)
                    self.assertEqual(zf.getinfo(NOTICES).external_attr >> 16, 0o100644)

    def test_the_notices_must_come_with_the_release(self):
        cases = [
            ("no notices", "remove", "THIRD_PARTY_NOTICES not found"),
            ("notices checksums.txt does not name", "replace", "THIRD_PARTY_NOTICES is sha256"),
        ]
        for name, action, want in cases:
            with self.subTest(name):
                path = os.path.join(self.binaries, NOTICES)
                if action == "remove":
                    os.remove(path)
                else:
                    with open(path, "wb") as fh:
                        fh.write(b"Third-party notices for gitlab-mcp-server\nanother body\n")
                try:
                    result = subprocess.run(
                        [sys.executable, BUILDER, "--binaries", self.binaries, "--version", VERSION,
                         "--out", os.path.join(self.work, "again")],
                        cwd=ROOT, capture_output=True, text=True, check=False,
                    )
                finally:
                    with open(path, "wb") as fh:
                        fh.write(NOTICES_TEXT)
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertIn(want, result.stderr)

    def test_a_missing_licence_stops_the_pack(self):
        with self.assertRaises(SystemExit) as caught:
            build_nuget.read_licenses([("LICENSE", os.path.join(self.work, "absent"))])
        self.assertIn("licence file LICENSE not found", str(caught.exception))

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

        def license_url_dropped():
            # The push of 2.7.5 was refused with a 400 for exactly this: an MIT
            # expression whose <licenseUrl> is missing. Nothing before the push
            # reported it, so the validator has to.
            with zipfile.ZipFile(self.package()) as zf:
                nuspec = zf.read("gitlab-mcp-server.nuspec").decode()
            nuspec = nuspec.replace("    <licenseUrl>https://licenses.nuget.org/MIT</licenseUrl>\n", "")
            repack(self.package(), {"gitlab-mcp-server.nuspec": nuspec})

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
            ("the license expression lost its licenseUrl", license_url_dropped, "licenseUrl"),
            ("a stray file sits beside the packages", leftover_file, "would try to push"),
            ("the build skipped verification", unverified_build, "--allow-unverified"),
            ("the executable bit was lost", executable_bit_lost, "executable bit"),
            ("the pointer is written with data descriptors",
             lambda: rewrite(self.package(), stream_data_descriptors=True), "data descriptor"),
            ("a binary is stored in zip64 form",
             lambda: rewrite(self.package("win-arm64"), zip64_entries=("tools/any/win-arm64/gitlab-mcp-server.exe",)),
             "zip64 form"),
            ("a package carries an archive comment",
             lambda: rewrite(self.package("linux-x64"), comment=b"built by hand"), "archive comment"),
            ("a package is already signed", lambda: sign_like_nuget(self.package("osx-arm64")),
             "already carries .signature.p7s"),
            ("a runtime package lost its licence",
             lambda: repack(self.package("linux-arm64"), drop=("LICENSE",)), "no LICENSE at the package root"),
            ("the pointer carries another licence text",
             lambda: repack(self.package(), {"LICENSE": "Not the MIT License\n"}),
             "LICENSE is not the repository's LICENSE"),
            ("a licence entry without its file type", lambda: self.relicense_mode("win-x64", 0o644),
             "LICENSE is not a regular file (mode 644)"),
            ("the pointer lost the notices",
             lambda: repack(self.package(), drop=(NOTICES,)), "no THIRD_PARTY_NOTICES at the package root"),
            ("a runtime package carries other notices",
             lambda: repack(self.package("osx-x64"),
                            {NOTICES: b"Third-party notices for gitlab-mcp-server\nanother body\n"}),
             "THIRD_PARTY_NOTICES is sha256"),
            ("a runtime package carries a file without the header",
             lambda: repack(self.package("linux-x64"), {NOTICES: "some other text\n"}),
             "THIRD_PARTY_NOTICES does not open with the generator's header"),
            ("the manifest records no notices digest", self.forget_notices_digest,
             "records no digest for THIRD_PARTY_NOTICES"),
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


class SignableLayoutTest(unittest.TestCase):
    """check_signable_layout over hand-damaged bytes, one record at a time.

    The end-to-end cases above reach it through zipfile, which can only write
    well-formed archives; these reach the branches that judge a record
    zipfile would never produce, each named by the problem it must report.
    """

    def archive(self, names=("[Content_Types].xml", "tools/any/linux-x64/gitlab-mcp-server")):
        blob = io.BytesIO()
        with zipfile.ZipFile(blob, "w") as zf:
            for name in names:
                info = zipfile.ZipInfo(name, date_time=build_nuget.ZIP_DATE)
                info.compress_type = zipfile.ZIP_DEFLATED
                zf.writestr(info, name.encode() * 8)
        return bytearray(blob.getvalue())

    def offsets(self, data):
        """The end record's offset and the central directory's offset."""
        eocd = bytes(data).rfind(validate_nuget.ZIP_EOCD)
        return eocd, struct.unpack_from("<I", data, eocd + 16)[0]

    def problems(self, data):
        problems = []
        validate_nuget.check_signable_layout(bytes(data), "pkg.nupkg", problems)
        return problems

    def assert_one(self, data, want):
        problems = self.problems(data)
        self.assertEqual(len(problems), 1, problems)
        self.assertIn(want, problems[0])

    def test_an_archive_packed_like_build_nuget_passes(self):
        self.assertEqual(self.problems(self.archive()), [])

    def test_damaged_records_are_named(self):
        def no_end_record(data):
            data[-22:-18] = b"XXXX"

        def truncated_end_record(data):
            del data[-4:]

        def comment(data):
            data[-2:] = struct.pack("<H", 5)
            data.extend(b"hello")

        def zip64_locator(data):
            eocd, _ = self.offsets(data)
            data[eocd:eocd] = validate_nuget.ZIP64_EOCD_LOCATOR + b"\0" * 16

        def zip64_entry_count(data):
            eocd, _ = self.offsets(data)
            struct.pack_into("<H", data, eocd + 10, validate_nuget.ZIP64_SENTINEL_16)

        def zip64_directory_offset(data):
            eocd, _ = self.offsets(data)
            struct.pack_into("<I", data, eocd + 16, validate_nuget.ZIP64_SENTINEL_32)

        def directory_past_the_end(data):
            eocd, _ = self.offsets(data)
            struct.pack_into("<I", data, eocd + 16, eocd)

        def damaged_central_record(data):
            _, cd = self.offsets(data)
            data[cd:cd + 4] = b"XXXX"

        def local_offset_elsewhere(data):
            _, cd = self.offsets(data)
            struct.pack_into("<I", data, cd + 42, 1)

        def descriptor_in_the_local_header_only(data):
            struct.pack_into("<H", data, 6, validate_nuget.ZIP_DATA_DESCRIPTOR_FLAG)

        def descriptor_in_the_central_record_only(data):
            _, cd = self.offsets(data)
            struct.pack_into("<H", data, cd + 8, validate_nuget.ZIP_DATA_DESCRIPTOR_FLAG)

        def zip64_compressed_size(data):
            _, cd = self.offsets(data)
            struct.pack_into("<I", data, cd + 20, validate_nuget.ZIP64_SENTINEL_32)

        def zip64_uncompressed_size(data):
            _, cd = self.offsets(data)
            struct.pack_into("<I", data, cd + 24, validate_nuget.ZIP64_SENTINEL_32)

        cases = [
            ("no end record", no_end_record, "no end-of-central-directory record"),
            ("an end record cut short", truncated_end_record, "no end-of-central-directory record"),
            ("an archive comment", comment, "5 byte(s) of archive comment"),
            ("a zip64 end locator", zip64_locator, "ends in zip64 form"),
            ("a zip64 entry count", zip64_entry_count, "ends in zip64 form"),
            ("a zip64 directory offset", zip64_directory_offset, "ends in zip64 form"),
            ("a directory past the end record", directory_past_the_end, "runs past the end"),
            ("a damaged central record", damaged_central_record, "no central directory record at offset"),
            ("a local offset that names no local header", local_offset_elsewhere, "that is not one"),
            ("a data descriptor in the local header only", descriptor_in_the_local_header_only,
             "[Content_Types].xml is written with a data descriptor"),
            ("a data descriptor in the central record only", descriptor_in_the_central_record_only,
             "[Content_Types].xml is written with a data descriptor"),
            ("a zip64 compressed size", zip64_compressed_size, "[Content_Types].xml is stored in zip64 form"),
            ("a zip64 uncompressed size", zip64_uncompressed_size, "[Content_Types].xml is stored in zip64 form"),
        ]
        for name, damage, want in cases:
            with self.subTest(name):
                data = self.archive()
                damage(data)
                self.assert_one(data, want)

    def test_a_zip64_record_is_named_as_zip64_before_its_local_offset_is_read(self):
        """A zip64 record keeps its real local offset in its extra field, so
        a sentinel offset, or a zip64 extra field, is reported as zip64 and
        never sent to look for a header where none was promised."""
        sentinel = self.archive()
        _, cd = self.offsets(sentinel)
        struct.pack_into("<I", sentinel, cd + 42, validate_nuget.ZIP64_SENTINEL_32)

        # The first record's extra field becomes a zip64 one in place: its
        # four-byte tail is turned into a header with an empty body, after
        # which the name, the offsets and every later record stay where they
        # were. zipfile writes no extra field of its own here, so one of four
        # bytes is carved out of the record's name length instead.
        extra = self.archive()
        _, cd = self.offsets(extra)
        name_len = struct.unpack_from("<H", extra, cd + 28)[0]
        struct.pack_into("<HH", extra, cd + 28, name_len - 4, 4)
        struct.pack_into("<HH", extra, cd + 46 + name_len - 4, validate_nuget.ZIP64_EXTRA_ID, 0)

        for name, data in (("a sentinel local offset", sentinel), ("a zip64 extra field", extra)):
            with self.subTest(name):
                problems = self.problems(data)
                self.assertEqual(len(problems), 1, problems)
                self.assertIn("is stored in zip64 form", problems[0])

    def test_a_zip64_extra_in_the_local_header_is_named(self):
        blob = io.BytesIO()
        with zipfile.ZipFile(blob, "w") as zf:
            with zf.open("tools/any/linux-x64/gitlab-mcp-server", "w", force_zip64=True) as fh:
                fh.write(b"binary")
        self.assert_one(bytearray(blob.getvalue()), "gitlab-mcp-server is stored in zip64 form")

    def test_a_signature_entry_is_named_in_any_case(self):
        for name in (".signature.p7s", ".SIGNATURE.P7S"):
            with self.subTest(name):
                self.assert_one(self.archive(("README.md", name)), "already carries " + name)

    def test_extra_field_ids_reads_every_header_and_stops_at_a_short_tail(self):
        extra = struct.pack("<HH", 0x5455, 2) + b"ab" + struct.pack("<HH", 0x0001, 0) + b"\x07"
        self.assertEqual(validate_nuget.extra_field_ids(extra), [0x5455, 0x0001])
        self.assertEqual(validate_nuget.extra_field_ids(b""), [])


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
