#!/usr/bin/env python3
"""Validate the PyPI wheelhouse before anything is uploaded.

A published PyPI file name is burned forever (deleting a release does not
free its file names), so everything checkable is checked before twine or the
publish action ever sees the wheels:

- all six platform wheels exist for the given version, and nothing else;
- every RECORD hash and size matches the archived bytes;
- WHEEL/METADATA/entry_points agree with the file name, the version, and the
  console script contract;
- METADATA carries the mcp-name ownership token the MCP Registry validates;
- each wheel holds exactly one binary with the right magic number and
  machine type for its tag, the executable bit set, and a size floor;
- every embedded binary is the exact one build_pypi.py verified against the
  release's cosign-signed checksums.txt — the checks above are shape checks,
  and a wrong-but-plausible file of the right size with the right first bytes
  passes all of them for the five platforms this host cannot execute;
- Linux binaries demand no glibc symbol newer than 2.17, which is what the
  manylinux_2_17 tag promises;
- the wheel matching the host is installed into a throwaway venv and the
  console script must answer an MCP initialize handshake over stdio with
  pure JSON-RPC and the right server version (skipped with --no-install).

Standard library only. Usage:
    python3 scripts/validate_pypi.py --wheels pypi/dist --version 2.7.5
"""

import argparse
import base64
import hashlib
import json
import os
import platform
import re
import struct
import subprocess
import sys
import tempfile
import venv
import zipfile

DIST_NAME = "jmrplens-gitlab-mcp-server"
DIST = DIST_NAME.replace("-", "_")
COMMAND = "gitlab-mcp-server"
MCP_NAME_TOKEN = "mcp-name: io.github.jmrplens/gitlab-mcp-server"
MIN_BINARY_BYTES = 20 * 1024 * 1024
GLIBC_CEILING = (2, 17)

# tag -> (goos, expected magic checker)
EXPECTED_TAGS = [
    "manylinux_2_17_x86_64.manylinux2014_x86_64",
    "manylinux_2_17_aarch64.manylinux2014_aarch64",
    "macosx_11_0_x86_64",
    "macosx_11_0_arm64",
    "win_amd64",
    "win_arm64",
]

failures = []


def fail(msg):
    failures.append(msg)
    print("FAIL:", msg)


def check_magic(tag, data):
    if tag.startswith("manylinux"):
        if data[:4] != b"\x7fELF":
            return "not an ELF binary"
        machine = struct.unpack_from("<H", data, 18)[0]
        want = 0x3E if "x86_64" in tag else 0xB7
        if machine != want:
            return "ELF machine 0x{:x}, want 0x{:x}".format(machine, want)
    elif tag.startswith("macosx"):
        if data[:4] != b"\xcf\xfa\xed\xfe":
            return "not a 64-bit Mach-O binary"
        cputype = struct.unpack_from("<I", data, 4)[0]
        want = 0x01000007 if "x86_64" in tag else 0x0100000C
        if cputype != want:
            return "Mach-O cputype 0x{:x}, want 0x{:x}".format(cputype, want)
    else:
        if data[:2] != b"MZ":
            return "not a PE binary"
        pe_off = struct.unpack_from("<I", data, 0x3C)[0]
        if data[pe_off:pe_off + 4] != b"PE\0\0":
            return "MZ without PE header"
        machine = struct.unpack_from("<H", data, pe_off + 4)[0]
        want = 0x8664 if tag == "win_amd64" else 0xAA64
        if machine != want:
            return "PE machine 0x{:x}, want 0x{:x}".format(machine, want)
    return None


def check_glibc_floor(name, data):
    worst = (0, 0)
    for m in re.finditer(rb"GLIBC_(\d+)\.(\d+)", data):
        ver = (int(m.group(1)), int(m.group(2)))
        worst = max(worst, ver)
    if worst > GLIBC_CEILING:
        fail("{}: binary demands glibc {}.{} but the tag promises {}.{}".format(
            name, worst[0], worst[1], GLIBC_CEILING[0], GLIBC_CEILING[1]))


VERIFIED_MANIFEST = "verified-binaries.json"

# Wheel platform tag -> the plat_key build_pypi.py records digests under.
TAG_PLAT_KEYS = {
    "manylinux_2_17_x86_64.manylinux2014_x86_64": "linux-amd64",
    "manylinux_2_17_aarch64.manylinux2014_aarch64": "linux-arm64",
    "macosx_11_0_x86_64": "darwin-amd64",
    "macosx_11_0_arm64": "darwin-arm64",
    "win_amd64": "windows-amd64",
    "win_arm64": "windows-arm64",
}


def read_verified(wheels_dir, version):
    """Load the digests build_pypi.py recorded, or fail if there are none.

    The manifest lives in the wheelhouse and is removed once read, so the
    publish step never sees a non-wheel file in packages-dir.
    """
    path = os.path.join(wheels_dir, VERIFIED_MANIFEST)
    if not os.path.isfile(path):
        fail("wheels were assembled without {} — build_pypi.py did not check them "
             "against the release's checksums.txt".format(VERIFIED_MANIFEST))
        return {}
    with open(path, encoding="utf-8") as fh:
        manifest = json.load(fh)
    os.remove(path)
    if not manifest.get("verified"):
        fail("{} says the wheels were built with --allow-unverified".format(VERIFIED_MANIFEST))
    if manifest.get("version") != version:
        fail("{} records version {} but this is {}".format(
            VERIFIED_MANIFEST, manifest.get("version"), version))
    return manifest.get("binaries") or {}


def validate_wheel(path, version, tag, verified=None):
    name = os.path.basename(path)
    with zipfile.ZipFile(path) as zf:
        names = zf.namelist()
        dist_info = "{}-{}.dist-info".format(DIST, version)

        record = zf.read(dist_info + "/RECORD").decode("utf-8")
        recorded = {}
        for line in record.strip().splitlines():
            arc, digest, size = line.rsplit(",", 2)
            recorded[arc] = (digest, size)
        if set(recorded) != set(names):
            fail("{}: RECORD names differ from archive contents".format(name))
        for arc in names:
            data = zf.read(arc)
            digest, size = recorded.get(arc, ("", ""))
            if arc.endswith("/RECORD"):
                continue
            want = "sha256=" + base64.urlsafe_b64encode(
                hashlib.sha256(data).digest()).decode("ascii").rstrip("=")
            if digest != want or size != str(len(data)):
                fail("{}: RECORD mismatch for {}".format(name, arc))

        metadata = zf.read(dist_info + "/METADATA").decode("utf-8")
        for needle in ("Name: " + DIST_NAME, "Version: " + version,
                       "Description-Content-Type: text/markdown"):
            if needle not in metadata:
                fail("{}: METADATA missing {!r}".format(name, needle))
        if not re.search(r"(^|\s)" + re.escape(MCP_NAME_TOKEN) + r"(\s|$)", metadata):
            fail("{}: METADATA description lost the MCP Registry ownership token".format(name))

        entry = zf.read(dist_info + "/entry_points.txt").decode("utf-8")
        if DIST_NAME + " = gitlab_mcp_server:main" not in entry:
            fail("{}: console-script wrapper named after the distribution missing (uvx name matching)".format(name))

        wheel_meta = zf.read(dist_info + "/WHEEL").decode("utf-8")
        if "Tag: py3-none-" + tag not in wheel_meta:
            fail("{}: WHEEL tag disagrees with the file name".format(name))
        if "Root-Is-Purelib: false" not in wheel_meta:
            fail("{}: wheel must be platlib (Root-Is-Purelib: false)".format(name))

        bin_name = "gitlab-mcp-server.exe" if tag.startswith("win") else "gitlab-mcp-server"
        arc = "{}-{}.data/scripts/{}".format(DIST, version, bin_name)
        if arc not in names:
            fail("{}: bundled binary {} missing".format(name, arc))
            return
        info = zf.getinfo(arc)
        data = zf.read(arc)
        if len(data) < MIN_BINARY_BYTES:
            fail("{}: binary is {} bytes, under the {} floor".format(name, len(data), MIN_BINARY_BYTES))
        mode = info.external_attr >> 16
        if not tag.startswith("win") and not ((mode & 0o170000) == 0o100000 and mode & 0o111):
            fail("{}: binary must be a regular file with the executable bit "
                 "(pip's zip_item_is_executable requires S_ISREG), got mode {:o}".format(name, mode))
        problem = check_magic(tag, data)
        if problem:
            fail("{}: {}".format(name, problem))
        if tag.startswith("manylinux"):
            check_glibc_floor(name, data)
        if verified:
            plat_key = TAG_PLAT_KEYS.get(tag)
            want = verified.get(plat_key)
            got = hashlib.sha256(data).hexdigest()
            if want != got:
                fail("{}: the embedded binary is sha256 {}, but the release's signed "
                     "checksums.txt named {}".format(name, got, want))


def host_tag():
    system = platform.system()
    machine = platform.machine().lower()
    if system == "Linux":
        return EXPECTED_TAGS[0] if machine in ("x86_64", "amd64") else EXPECTED_TAGS[1]
    if system == "Darwin":
        return EXPECTED_TAGS[2] if machine == "x86_64" else EXPECTED_TAGS[3]
    if system == "Windows":
        return EXPECTED_TAGS[4] if machine in ("amd64", "x86_64") else EXPECTED_TAGS[5]
    return None


def handshake(wheel_path, version, commands=(COMMAND, DIST_NAME)):
    tmp = tempfile.mkdtemp(prefix="pypi-validate-")
    env_dir = os.path.join(tmp, "venv")
    venv.create(env_dir, with_pip=True)
    bin_dir = "Scripts" if os.name == "nt" else "bin"
    pip = os.path.join(env_dir, bin_dir, "pip")
    subprocess.run([pip, "install", "--quiet", "--no-index", wheel_path], check=True)

    for command in commands:
        run_handshake(os.path.join(env_dir, bin_dir, command), version, tmp, command)


def run_handshake(script, version, tmp, label):
    request = json.dumps({
        "jsonrpc": "2.0", "id": 1, "method": "initialize",
        "params": {"protocolVersion": "2025-11-25", "capabilities": {},
                   "clientInfo": {"name": "validate-pypi", "version": "1"}},
    }) + "\n"
    stderr_path = os.path.join(tmp, "stderr.log")
    stderr_file = open(stderr_path, "wb")
    proc = subprocess.Popen(
        [script],
        stdin=subprocess.PIPE, stdout=subprocess.PIPE,
        stderr=stderr_file,
        env={**os.environ, "GITLAB_URL": "http://127.0.0.1:1",
             "GITLAB_TOKEN": "glpat-validate-pypi", "LOG_LEVEL": "error",
             # The fake URL is unreachable by design; skipping scope
             # detection avoids its retry loop in the handshake path.
             "GITLAB_IGNORE_SCOPES": "true"},
    )
    # stdin stays open while the response is read: closing it signals
    # shutdown to a stdio MCP server, and a server told to shut down before
    # it answered is allowed to just go.
    import threading
    line_holder = []
    reader = threading.Thread(target=lambda: line_holder.append(proc.stdout.readline()))
    reader.daemon = True
    proc.stdin.write(request.encode("utf-8"))
    proc.stdin.flush()
    reader.start()
    reader.join(timeout=60)
    proc.stdin.close()
    proc.terminate()
    proc.wait(timeout=15)
    stderr_file.close()
    if not line_holder or not line_holder[0]:
        with open(stderr_path, "rb") as fh:
            tail = fh.read()[-800:]
        fail("handshake ({}): no answer within 60s; stderr tail: {!r}".format(label, tail))
        return
    line = line_holder[0].rstrip(b"\n")
    try:
        response = json.loads(line)
    except ValueError:
        fail("handshake ({}): stdout is not pure JSON-RPC: {!r}".format(label, line[:120]))
        return
    server_info = response.get("result", {}).get("serverInfo", {})
    if server_info.get("version") != version:
        fail("handshake ({}): serverInfo.version = {!r}, want {!r}".format(
            label, server_info.get("version"), version))
    else:
        print("handshake:", label, "answered initialize with version", version)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--wheels", default="pypi/dist")
    parser.add_argument("--version", required=True)
    parser.add_argument("--no-install", action="store_true",
                        help="skip the venv install + MCP handshake")
    args = parser.parse_args()

    expected = {"{}-{}-py3-none-{}.whl".format(DIST, args.version, tag): tag
                for tag in EXPECTED_TAGS}
    verified = read_verified(args.wheels, args.version)

    present = sorted(f for f in os.listdir(args.wheels) if f.endswith(".whl"))
    if set(present) != set(expected):
        fail("wheelhouse holds {} but the release needs exactly {}".format(present, sorted(expected)))

    leftovers = sorted(f for f in os.listdir(args.wheels) if not f.endswith(".whl"))
    if leftovers:
        fail("wheelhouse holds non-wheel files the publish step would try to upload: {}".format(leftovers))

    for fname in present:
        if fname in expected:
            validate_wheel(os.path.join(args.wheels, fname), args.version, expected[fname], verified)

    if not args.no_install and not failures:
        tag = host_tag()
        if tag is None:
            print("handshake: unrecognized host platform, skipping install test")
        else:
            wheel = os.path.join(args.wheels, "{}-{}-py3-none-{}.whl".format(DIST, args.version, tag))
            handshake(wheel, args.version)

    if failures:
        sys.exit("validate_pypi: {} failure(s)".format(len(failures)))
    print("validate_pypi: all wheels valid for version", args.version)


if __name__ == "__main__":
    main()
