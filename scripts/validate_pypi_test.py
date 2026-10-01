#!/usr/bin/env python3
"""Tests scripts/build_pypi.py and scripts/validate_pypi.py on wheels built
from fake binaries.

A published PyPI file name is burned forever, so the validator is what
decides whether a wheel reaches the registry. These cases hold it to the
licence declaration core metadata 2.4 defines (PEP 639): an SPDX
License-Expression, never the legacy License field beside it, a
License-File for each text the wheel carries under .dist-info/licenses/,
and those texts byte for byte the repository's own. They also hold the
well-known Issues and Security project URLs and the file type of every
archive entry.

The binaries carry the header bytes the validator inspects, so a case fails
for the reason it names and not for a header the fixture never had; the
size floor is lowered for the duration, since twenty megabytes per platform
would test nothing the floor's own check does not.

Run with:

    python3 -m unittest discover -s scripts -p 'validate_pypi_test.py'
"""

import hashlib
import importlib.util
import io
import os
import shutil
import struct
import subprocess
import sys
import tempfile
import unittest
import zipfile
from contextlib import redirect_stdout
from unittest import mock

ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
BUILDER = os.path.join(ROOT, "scripts", "build_pypi.py")


def load_module(name):
    """Import a script by path; scripts/ is not a package."""
    spec = importlib.util.spec_from_file_location(name, os.path.join(ROOT, "scripts", name + ".py"))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


validate_pypi = load_module("validate_pypi")
build_pypi = load_module("build_pypi")

VERSION = "9.9.9"
DIST_INFO = "{}-{}.dist-info".format(validate_pypi.DIST, VERSION)


def fake_binary(plat_key):
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
    return head + plat_key.encode() * 64


def asset_name(plat_key):
    return "gitlab-mcp-server-" + plat_key + (".exe" if plat_key.startswith("windows") else "")


def write_fixture(binaries_dir):
    """The six release assets and a matching checksums.txt."""
    os.makedirs(binaries_dir, exist_ok=True)
    lines = []
    for plat_key in build_pypi.PLATFORMS:
        payload = fake_binary(plat_key)
        with open(os.path.join(binaries_dir, asset_name(plat_key)), "wb") as fh:
            fh.write(payload)
        lines.append("{}  {}".format(hashlib.sha256(payload).hexdigest(), asset_name(plat_key)))
    with open(os.path.join(binaries_dir, "checksums.txt"), "w", encoding="utf-8") as fh:
        fh.write("\n".join(lines) + "\n")


def rewrite(path, replace=None, drop=(), add=None, modes=None):
    """Rewrite a wheel with entries replaced, dropped or added, and its RECORD
    recomputed, so a case fails for what it changed rather than for a RECORD
    that no longer matches."""
    replace = dict(replace or {})
    add = dict(add or {})
    modes = dict(modes or {})
    entries = []
    with zipfile.ZipFile(path) as src:
        for info in src.infolist():
            if info.filename in drop or info.filename.endswith("/RECORD"):
                continue
            data = replace.get(info.filename, src.read(info.filename))
            entries.append((info.filename, data, info.external_attr >> 16))
    for name, data in add.items():
        entries.append((name, data, 0o100644))
    records = []
    with zipfile.ZipFile(path, "w") as dst:
        for name, data, mode in entries:
            if isinstance(data, str):
                data = data.encode("utf-8")
            info = zipfile.ZipInfo(name, date_time=build_pypi.ZIP_DATE)
            info.external_attr = modes.get(name, mode) << 16
            dst.writestr(info, data)
            records.append("{},{},{}".format(name, build_pypi.record_hash(data), len(data)))
        record_name = DIST_INFO + "/RECORD"
        records.append(record_name + ",,")
        info = zipfile.ZipInfo(record_name, date_time=build_pypi.ZIP_DATE)
        info.external_attr = modes.get(record_name, 0o100644) << 16
        dst.writestr(info, "\n".join(records) + "\n")


class WheelTestCase(unittest.TestCase):
    """Builds the six wheels once per test in a scratch directory."""

    def setUp(self):
        self.work = tempfile.mkdtemp(prefix="validate-pypi-")
        self.addCleanup(shutil.rmtree, self.work, True)
        self.binaries = os.path.join(self.work, "dist")
        self.out = os.path.join(self.work, "wheels")
        write_fixture(self.binaries)
        result = subprocess.run(
            [sys.executable, BUILDER, "--binaries", self.binaries, "--version", VERSION, "--out", self.out],
            cwd=ROOT, capture_output=True, text=True, check=False,
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        validate_pypi.failures.clear()
        self.addCleanup(validate_pypi.failures.clear)
        patcher = mock.patch.object(validate_pypi, "MIN_BINARY_BYTES", 0)
        patcher.start()
        self.addCleanup(patcher.stop)

    def wheel(self, tag="manylinux_2_17_x86_64.manylinux2014_x86_64"):
        return os.path.join(self.out, "{}-{}-py3-none-{}.whl".format(validate_pypi.DIST, VERSION, tag)), tag

    def validate(self, path, tag):
        with redirect_stdout(io.StringIO()):
            validate_pypi.validate_wheel(path, VERSION, tag)
        return list(validate_pypi.failures)

    def metadata(self, path):
        with zipfile.ZipFile(path) as zf:
            return zf.read(DIST_INFO + "/METADATA").decode("utf-8")


class BuiltWheelTest(WheelTestCase):
    """What build_pypi.py writes passes, and carries what PEP 639 asks."""

    def test_every_built_wheel_validates(self):
        for tag in validate_pypi.EXPECTED_TAGS:
            with self.subTest(tag):
                validate_pypi.failures.clear()
                self.assertEqual(self.validate(*self.wheel(tag)), [])

    def test_metadata_declares_the_licence_the_pep_639_way(self):
        path, _ = self.wheel()
        headers = validate_pypi.metadata_headers(self.metadata(path))
        self.assertEqual(headers["Metadata-Version"], "2.4")
        self.assertEqual(headers["License-Expression"], "MIT")
        self.assertEqual(headers.get_all("License-File"), ["LICENSE"])
        self.assertIsNone(headers.get("License"))
        self.assertFalse([c for c in headers.get_all("Classifier") if c.startswith("License ::")])
        urls = headers.get_all("Project-URL")
        self.assertIn("Issues, https://github.com/jmrplens/gitlab-mcp-server/issues", urls)
        self.assertIn("Security, https://github.com/jmrplens/gitlab-mcp-server/security/policy", urls)

    def test_the_licence_text_is_the_repository_licence(self):
        path, _ = self.wheel("win_arm64")
        with open(os.path.join(ROOT, "LICENSE"), "rb") as fh:
            want = fh.read()
        with zipfile.ZipFile(path) as zf:
            self.assertEqual(zf.read(DIST_INFO + "/licenses/LICENSE"), want)
            record = zf.read(DIST_INFO + "/RECORD").decode("utf-8")
            self.assertIn(DIST_INFO + "/licenses/LICENSE,sha256=", record)

    def test_every_entry_is_a_regular_file(self):
        path, _ = self.wheel()
        with zipfile.ZipFile(path) as zf:
            for info in zf.infolist():
                with self.subTest(info.filename):
                    self.assertEqual((info.external_attr >> 16) & 0o170000, 0o100000)

    def test_a_missing_licence_file_stops_the_build(self):
        with self.assertRaises(SystemExit) as caught:
            build_pypi.read_licenses([("LICENSE", os.path.join(self.work, "absent"))])
        self.assertIn("licence file LICENSE not found", str(caught.exception))


class LicensingRefusalTest(WheelTestCase):
    """Each way a licence declaration goes wrong is refused, by name."""

    def test_refuses_each_departure(self):
        metadata_path = DIST_INFO + "/METADATA"
        license_path = DIST_INFO + "/licenses/LICENSE"
        record_path = DIST_INFO + "/RECORD"

        def edited_metadata(old, new):
            path, _ = self.wheel()
            return {metadata_path: self.metadata(path).replace(old, new, 1)}

        cases = [
            ("metadata 2.1", dict(replace=edited_metadata("Metadata-Version: 2.4", "Metadata-Version: 2.1")),
             "Metadata-Version is '2.1'"),
            ("another expression", dict(replace=edited_metadata("License-Expression: MIT", "License-Expression: Apache-2.0")),
             "License-Expression is 'Apache-2.0'"),
            ("legacy field beside the expression",
             dict(replace=edited_metadata("License-Expression: MIT", "License-Expression: MIT\nLicense: MIT")),
             "legacy License field"),
            ("licence classifier",
             dict(replace=edited_metadata("Classifier: Development", "Classifier: License :: OSI Approved :: MIT License\nClassifier: Development")),
             "License :: OSI Approved :: MIT License"),
            ("no License-File", dict(replace=edited_metadata("License-File: LICENSE\n", "")),
             "License-File declares []"),
            ("declared and not shipped", dict(drop=(license_path,)),
             "holds [] but METADATA declares ['LICENSE']"),
            ("shipped and not declared", dict(add={DIST_INFO + "/licenses/EXTRA": "extra"}),
             "holds ['EXTRA', 'LICENSE']"),
            ("another text", dict(replace={license_path: "Not the MIT License\n"}),
             "is not the repository's LICENSE"),
            ("RECORD without its type bits", dict(modes={record_path: 0o644}),
             "RECORD is not a regular file (mode 644)"),
            ("no Issues URL", dict(replace=edited_metadata("Project-URL: Issues, ", "Project-URL: Tracker, ")),
             "Project-URL 'Issues' is None"),
            ("another Security URL",
             dict(replace=edited_metadata("security/policy", "security")),
             "Project-URL 'Security' is 'https://github.com/jmrplens/gitlab-mcp-server/security'"),
        ]
        path, tag = self.wheel()
        pristine = os.path.join(self.work, "pristine.whl")
        shutil.copyfile(path, pristine)
        for name, change, want in cases:
            with self.subTest(name):
                shutil.copyfile(pristine, path)
                rewrite(path, **change)
                validate_pypi.failures.clear()
                failures = self.validate(path, tag)
                self.assertTrue(any(want in f for f in failures), failures)


class TwineCheckTest(unittest.TestCase):
    """twine runs when it is installed and its refusal is a failure."""

    def setUp(self):
        validate_pypi.failures.clear()
        self.addCleanup(validate_pypi.failures.clear)

    def test_skipped_and_said_so_without_twine(self):
        out = io.StringIO()
        with mock.patch.object(validate_pypi.importlib.util, "find_spec", return_value=None), \
                mock.patch.object(validate_pypi.subprocess, "run") as run, redirect_stdout(out):
            validate_pypi.twine_check(["a.whl"])
        run.assert_not_called()
        self.assertIn("skipping twine check", out.getvalue())
        self.assertEqual(validate_pypi.failures, [])

    def test_runs_strict_and_reports_a_refusal(self):
        cases = [
            ("passes", 0, False),
            ("refuses", 1, True),
        ]
        for name, code, want_failure in cases:
            with self.subTest(name):
                validate_pypi.failures.clear()
                done = subprocess.CompletedProcess([], code, stdout="Checking a.whl: FAILED\n", stderr="")
                with mock.patch.object(validate_pypi.importlib.util, "find_spec", return_value=object()), \
                        mock.patch.object(validate_pypi.subprocess, "run", return_value=done) as run, \
                        redirect_stdout(io.StringIO()):
                    validate_pypi.twine_check(["a.whl", "b.whl"])
                argv = run.call_args.args[0]
                self.assertEqual(argv[1:], ["-m", "twine", "check", "--strict", "a.whl", "b.whl"])
                self.assertEqual(bool(validate_pypi.failures), want_failure)
                if want_failure:
                    self.assertIn("Checking a.whl: FAILED", validate_pypi.failures[0])


if __name__ == "__main__":
    unittest.main()
