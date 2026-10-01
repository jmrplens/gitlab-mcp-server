#!/usr/bin/env python3
"""Hold each Linux release binary to the ELF interpreter its channels promise.

The release binaries are built with -buildmode=pie, which makes them
dynamically linked, so the kernel starts each one through the loader its
PT_INTERP program header names. Go's linker picks that path by stat-ing the
host that builds the release (cmd/link/internal/ld/elf.go), and a binary
naming a loader the target system does not have dies at exec with "not
found" before a line of Go runs: the image's linux/arm64 binary shipped that
way once.

Every Linux channel promises glibc: npm's libc field, PyPI's manylinux tags,
NuGet's linux RIDs and the .mcpb launcher all hand out the same two
binaries. .goreleaser.yml therefore names the glibc loaders in per-target
ldflags overrides, and this script holds the built binaries to them once,
right after GoReleaser, which covers every channel at the one point they
share rather than one channel's validator at a time.

Standard library only. Usage:
    python3 scripts/check_elf_interp.py dist/gitlab-mcp-server-linux-amd64 \\
        dist/gitlab-mcp-server-linux-arm64

Exits 0 when every binary names the interpreter its machine expects, 1 when
any does not or cannot be read, and 2 when no binary is named.
"""

import struct
import sys

# ELF e_machine -> (name, interpreter). The paths are the ones the
# per-target overrides in .goreleaser.yml pass with -I; a test holds the two
# lists to each other.
EXPECTED = {
    0x3E: ("x86-64", "/lib64/ld-linux-x86-64.so.2"),
    0xB7: ("aarch64", "/lib/ld-linux-aarch64.so.1"),
}

PT_INTERP = 3
ELF64_HEADER_SIZE = 64
ELF64_PHDR_SIZE = 56


def read_interpreter(data):
    """Return (e_machine, interpreter) of a 64-bit little-endian ELF image.

    The interpreter is None when the image has no PT_INTERP, which is what a
    statically linked binary looks like. Anything that is not a well-formed
    64-bit little-endian ELF raises ValueError, as does an image declaring
    more than one PT_INTERP, which the kernel refuses to start.
    """
    if len(data) < ELF64_HEADER_SIZE or data[:4] != b"\x7fELF":
        raise ValueError("not an ELF file")
    if data[4] != 2:
        raise ValueError("not a 64-bit ELF file")
    if data[5] != 1:
        raise ValueError("not a little-endian ELF file")
    machine = struct.unpack_from("<H", data, 18)[0]
    phoff = struct.unpack_from("<Q", data, 32)[0]
    phentsize, phnum = struct.unpack_from("<HH", data, 54)
    if phnum and phentsize < ELF64_PHDR_SIZE:
        raise ValueError("program header entries are {} bytes, under {}".format(phentsize, ELF64_PHDR_SIZE))
    if phoff + phnum * phentsize > len(data):
        raise ValueError("program headers run past the end of the file")
    interpreters = []
    for index in range(phnum):
        start = phoff + index * phentsize
        p_type = struct.unpack_from("<I", data, start)[0]
        if p_type != PT_INTERP:
            continue
        offset = struct.unpack_from("<Q", data, start + 8)[0]
        size = struct.unpack_from("<Q", data, start + 32)[0]
        if offset + size > len(data):
            raise ValueError("PT_INTERP runs past the end of the file")
        interpreters.append(data[offset:offset + size].split(b"\0", 1)[0].decode("utf-8", "replace"))
    if len(interpreters) > 1:
        raise ValueError("{} PT_INTERP headers".format(len(interpreters)))
    return machine, (interpreters[0] if interpreters else None)


def check(path):
    """Return what is wrong with the binary at path, or None."""
    try:
        with open(path, "rb") as fh:
            data = fh.read()
        machine, interp = read_interpreter(data)
    except (OSError, ValueError) as err:
        return "{}: {}".format(path, err)
    if machine not in EXPECTED:
        return "{}: ELF machine 0x{:x} is not one the release builds".format(path, machine)
    name, want = EXPECTED[machine]
    if interp is None:
        return "{}: {} binary names no interpreter, want {}".format(path, name, want)
    if interp != want:
        return "{}: {} binary names interpreter {}, want {}".format(path, name, interp, want)
    return None


def main(argv):
    if not argv:
        sys.stderr.write("usage: check_elf_interp.py BINARY [BINARY...]\n")
        return 2
    problems = [p for p in (check(path) for path in argv) if p]
    for problem in problems:
        sys.stderr.write("check_elf_interp: " + problem + "\n")
    if problems:
        return 1
    for path in argv:
        print("check_elf_interp: {} names the expected interpreter".format(path))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
