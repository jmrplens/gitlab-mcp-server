#!/usr/bin/env python3
"""Tests scripts/check_elf_interp.py, the release's PT_INTERP check.

The images are built here, byte by byte, rather than read from a release:
the cases worth holding are the ones no correct build produces (a musl
loader on a glibc channel, a static binary, a machine the release does not
build, a truncated file), and a real Go binary would need the toolchain this
suite deliberately does without. One case does read a real binary, the
interpreter running these tests, where the host makes that meaningful.

Run with:

    python3 -m unittest discover -s scripts -p 'check_elf_interp_test.py'
"""

import io
import os
import platform
import re
import shutil
import struct
import sys
import tempfile
import unittest
from contextlib import redirect_stderr, redirect_stdout

ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
sys.path.insert(0, os.path.join(ROOT, "scripts"))

import check_elf_interp  # noqa: E402

AMD64 = 0x3E
ARM64 = 0xB7
GLIBC_AMD64 = "/lib64/ld-linux-x86-64.so.2"
GLIBC_ARM64 = "/lib/ld-linux-aarch64.so.1"
PT_LOAD = 1


def elf(machine=AMD64, interpreters=(GLIBC_AMD64,), elf_class=2, encoding=1,
        leading_loads=1, phentsize=56, truncate_interp=False):
    """Build a minimal 64-bit ELF image.

    The program header table starts right after the 64-byte file header and
    holds `leading_loads` PT_LOAD entries before one PT_INTERP per entry of
    `interpreters`, so the check has to walk past headers that are not the
    one it wants. The interpreter strings follow the table, NUL-terminated
    the way the linker writes them.
    """
    phnum = leading_loads + len(interpreters)
    header = bytearray(64)
    header[0:4] = b"\x7fELF"
    header[4] = elf_class
    header[5] = encoding
    header[6] = 1
    struct.pack_into("<H", header, 18, machine)
    struct.pack_into("<Q", header, 32, 64)
    struct.pack_into("<HH", header, 54, phentsize, phnum)

    table = bytearray(phentsize * phnum)
    strings = bytearray()
    strings_at = 64 + len(table)
    for index in range(phnum):
        entry = index * phentsize
        if index < leading_loads:
            struct.pack_into("<I", table, entry, PT_LOAD)
            continue
        text = interpreters[index - leading_loads].encode() + b"\0"
        size = len(text) + (4096 if truncate_interp else 0)
        struct.pack_into("<I", table, entry, check_elf_interp.PT_INTERP)
        struct.pack_into("<Q", table, entry + 8, strings_at + len(strings))
        struct.pack_into("<Q", table, entry + 32, size)
        strings += text
    return bytes(header + table + strings)


class ReadInterpreterTest(unittest.TestCase):
    """Holds the parser to what the kernel reads, and to refusing the rest."""

    def test_reads_machine_and_interpreter(self):
        cases = [
            ("amd64 glibc", elf(AMD64, (GLIBC_AMD64,)), (AMD64, GLIBC_AMD64)),
            ("arm64 glibc", elf(ARM64, (GLIBC_ARM64,)), (ARM64, GLIBC_ARM64)),
            ("interpreter first in the table", elf(ARM64, (GLIBC_ARM64,), leading_loads=0), (ARM64, GLIBC_ARM64)),
            ("static: no PT_INTERP", elf(AMD64, ()), (AMD64, None)),
            ("wider entries than the minimum", elf(AMD64, (GLIBC_AMD64,), phentsize=64), (AMD64, GLIBC_AMD64)),
        ]
        for name, data, want in cases:
            with self.subTest(name):
                self.assertEqual(check_elf_interp.read_interpreter(data), want)

    def test_refuses_what_is_not_a_readable_elf64_le(self):
        cases = [
            ("empty", b"", "not an ELF file"),
            ("short of a header", b"\x7fELF" + b"\0" * 59, "not an ELF file"),
            ("wrong magic", b"MZ" + elf()[2:], "not an ELF file"),
            ("32-bit", elf(elf_class=1), "not a 64-bit ELF file"),
            ("big-endian", elf(encoding=2), "not a little-endian ELF file"),
            ("short entries", elf(phentsize=55), "under 56"),
            ("table past the end", elf()[:100], "program headers run past the end"),
            ("interpreter past the end", elf(truncate_interp=True), "PT_INTERP runs past the end"),
            ("two interpreters", elf(AMD64, (GLIBC_AMD64, GLIBC_AMD64)), "2 PT_INTERP headers"),
        ]
        for name, data, want in cases:
            with self.subTest(name):
                with self.assertRaises(ValueError) as caught:
                    check_elf_interp.read_interpreter(data)
                self.assertIn(want, str(caught.exception))

    def test_a_table_ending_exactly_at_the_file_end_is_read(self):
        # The bounds check is "past the end", not "at the end": a table that
        # fills the file to its last byte is whole. A static binary whose
        # headers are its last bytes is that case.
        data = elf(AMD64, ())
        self.assertEqual(len(data), 64 + 56)
        self.assertEqual(check_elf_interp.read_interpreter(data), (AMD64, None))

    def test_an_interpreter_ending_exactly_at_the_file_end_is_read(self):
        data = elf(ARM64, (GLIBC_ARM64,))
        self.assertTrue(data.endswith(GLIBC_ARM64.encode() + b"\0"))
        self.assertEqual(check_elf_interp.read_interpreter(data), (ARM64, GLIBC_ARM64))

    @unittest.skipUnless(sys.platform.startswith("linux") and platform.machine() in ("x86_64", "aarch64"),
                         "the running interpreter is a glibc ELF only on a Linux x86-64 or aarch64 host")
    def test_reads_a_real_binary(self):
        # The interpreter running this suite is a dynamically linked ELF on a
        # glibc host, so the parser must find a loader in it, which an
        # off-by-one in the header offsets would not.
        with open(os.path.realpath(sys.executable), "rb") as fh:
            machine, interp = check_elf_interp.read_interpreter(fh.read())
        self.assertIn(machine, check_elf_interp.EXPECTED)
        if interp is None:
            self.skipTest("this Python is statically linked")
        self.assertTrue(interp.startswith("/lib"), interp)


class CheckTest(unittest.TestCase):
    """Holds each binary to the loader its machine is promised."""

    def setUp(self):
        self.work = tempfile.mkdtemp(prefix="check-elf-interp-")
        self.addCleanup(shutil.rmtree, self.work, True)

    def write(self, name, data):
        path = os.path.join(self.work, name)
        with open(path, "wb") as fh:
            fh.write(data)
        return path

    def test_judges_each_binary(self):
        cases = [
            ("amd64 glibc", elf(AMD64, (GLIBC_AMD64,)), None),
            ("arm64 glibc", elf(ARM64, (GLIBC_ARM64,)), None),
            ("amd64 musl", elf(AMD64, ("/lib/ld-musl-x86_64.so.1",)),
             "x86-64 binary names interpreter /lib/ld-musl-x86_64.so.1, want " + GLIBC_AMD64),
            ("arm64 with the amd64 loader", elf(ARM64, (GLIBC_AMD64,)),
             "aarch64 binary names interpreter " + GLIBC_AMD64 + ", want " + GLIBC_ARM64),
            ("static", elf(AMD64, ()), "x86-64 binary names no interpreter, want " + GLIBC_AMD64),
            ("riscv64", elf(0xF3, ("/lib/ld-linux-riscv64-lp64d.so.1",)),
             "ELF machine 0xf3 is not one the release builds"),
            ("not ELF", b"MZ" + b"\0" * 100, "not an ELF file"),
        ]
        for name, data, want in cases:
            with self.subTest(name):
                path = self.write(name.replace(" ", "-"), data)
                got = check_elf_interp.check(path)
                if want is None:
                    self.assertIsNone(got)
                else:
                    self.assertEqual(got, path + ": " + want)

    def test_a_missing_file_is_reported_not_raised(self):
        path = os.path.join(self.work, "absent")
        got = check_elf_interp.check(path)
        self.assertTrue(got.startswith(path + ": "), got)

    def test_main_exit_codes(self):
        good = self.write("good", elf(AMD64, (GLIBC_AMD64,)))
        bad = self.write("bad", elf(ARM64, ("/lib/ld-musl-aarch64.so.1",)))
        cases = [
            ("no binary named", [], 2, "", "usage:"),
            ("every binary right", [good], 0, "names the expected interpreter", ""),
            ("one wrong among right", [good, bad], 1, "", "bad: aarch64 binary names interpreter"),
        ]
        for name, argv, want_code, want_out, want_err in cases:
            with self.subTest(name):
                out, err = io.StringIO(), io.StringIO()
                with redirect_stdout(out), redirect_stderr(err):
                    code = check_elf_interp.main(argv)
                self.assertEqual(code, want_code)
                self.assertIn(want_out, out.getvalue())
                self.assertIn(want_err, err.getvalue())
                if want_code == 1:
                    # A failing run names nothing as right: the summary of
                    # what passed is printed only when everything did.
                    self.assertEqual(out.getvalue(), "")


class GoreleaserAgreementTest(unittest.TestCase):
    """Holds the table above to the overrides .goreleaser.yml builds with."""

    def test_overrides_pass_the_interpreters_this_script_expects(self):
        with open(os.path.join(ROOT, ".goreleaser.yml"), encoding="utf-8") as fh:
            config = fh.read()
        named = re.findall(r"^\s*- -I (\S+)\s*$", config, re.MULTILINE)
        expected = sorted(interp for _, interp in check_elf_interp.EXPECTED.values())
        self.assertEqual(sorted(named), expected)
        for goarch, interp in (("amd64", GLIBC_AMD64), ("arm64", GLIBC_ARM64)):
            with self.subTest(goarch):
                block = re.search(
                    r"- goos: linux\n\s+goarch: " + goarch + r"\n\s+ldflags:\n\s+- \*ldflags\n\s+- -I (\S+)",
                    config,
                )
                self.assertIsNotNone(block, "no linux/{} override in .goreleaser.yml".format(goarch))
                self.assertEqual(block.group(1), interp)


if __name__ == "__main__":
    unittest.main()
