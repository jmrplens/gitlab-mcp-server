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
- the licence is declared as core metadata 2.4 defines it (PEP 639): an
  SPDX License-Expression, no legacy License field and no License ::
  classifier beside it (PyPI refuses that pair), and a License-File for
  each text under .dist-info/licenses/, which hold the repository's own
  LICENSE byte for byte, the release's THIRD_PARTY_NOTICES (the
  generator's header, and the digest build_pypi.py verified against
  checksums.txt), and nothing undeclared;
- METADATA links the issue tracker and the security policy under the
  well-known Issues and Security labels;
- every archive entry, RECORD included, is a regular file (S_IFREG);
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
  pure JSON-RPC and the right server version (skipped with --no-install);
- `twine check --strict` passes over every wheel, when twine is installed
  in the interpreter running this (the publish action runs its own).

Standard library only. Usage:
    python3 scripts/validate_pypi.py --wheels pypi/dist --version 2.7.5
"""

import argparse
import base64
import email.parser
import hashlib
import importlib.util
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
ROOT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..")

METADATA_VERSION = "2.4"
LICENSE_EXPRESSION = "MIT"

# License-File value -> the file in this repository whose bytes it must hold.
LICENSE_SOURCES = {"LICENSE": os.path.join(ROOT, "LICENSE")}

# The License-File generated with the release rather than kept in the
# repository: held to the generator's header and to the digest build_pypi.py
# recorded after checking it against the release's checksums.txt.
NOTICES = "THIRD_PARTY_NOTICES"
NOTICES_HEADER = b"Third-party notices for gitlab-mcp-server\n"
LICENSE_FILES = sorted(list(LICENSE_SOURCES) + [NOTICES])

# Well-known project URL labels the wheel must carry, with their targets.
REQUIRED_PROJECT_URLS = {
    "Issues": "https://github.com/jmrplens/gitlab-mcp-server/issues",
    "Security": "https://github.com/jmrplens/gitlab-mcp-server/security/policy",
}

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
    """Load the digests build_pypi.py recorded, or fail if there are none:
    the binaries' by platform, and the notices'.

    The manifest lives in the wheelhouse and is removed once read, so the
    publish step never sees a non-wheel file in packages-dir.
    """
    path = os.path.join(wheels_dir, VERIFIED_MANIFEST)
    if not os.path.isfile(path):
        fail("wheels were assembled without {} — build_pypi.py did not check them "
             "against the release's checksums.txt".format(VERIFIED_MANIFEST))
        return {}, None
    with open(path, encoding="utf-8") as fh:
        manifest = json.load(fh)
    os.remove(path)
    if not manifest.get("verified"):
        fail("{} says the wheels were built with --allow-unverified".format(VERIFIED_MANIFEST))
    if manifest.get("version") != version:
        fail("{} records version {} but this is {}".format(
            VERIFIED_MANIFEST, manifest.get("version"), version))
    if not manifest.get("notices"):
        fail("{} records no digest for {}".format(VERIFIED_MANIFEST, NOTICES))
    return manifest.get("binaries") or {}, manifest.get("notices")


def metadata_headers(metadata):
    """Parse the header block of a METADATA file (the part before the
    description) the way packaging tools read it: RFC 822 style fields."""
    return email.parser.HeaderParser().parsestr(metadata)


def check_licensing(zf, name, dist_info, metadata, notices_digest=None):
    """Hold the wheel's licence declaration to core metadata 2.4 (PEP 639).

    The legacy License field and License-Expression are mutually exclusive
    and PyPI refuses a file carrying both; a License :: classifier is
    deprecated beside an expression. Every License-File must be in
    .dist-info/licenses/ under the path the field names, nothing may sit
    there undeclared, and each repository text must be the repository's own
    file byte for byte, so a wheel can never carry a licence the source does
    not. The third-party notices open with the generator's header and, when
    build_pypi.py recorded their digest, are those exact bytes.
    """
    headers = metadata_headers(metadata)
    if headers.get("Metadata-Version") != METADATA_VERSION:
        fail("{}: Metadata-Version is {!r}, want {!r} (License-Expression needs it)".format(
            name, headers.get("Metadata-Version"), METADATA_VERSION))
    if headers.get("License-Expression") != LICENSE_EXPRESSION:
        fail("{}: License-Expression is {!r}, want {!r}".format(
            name, headers.get("License-Expression"), LICENSE_EXPRESSION))
    if headers.get("License") is not None:
        fail("{}: METADATA carries the legacy License field beside License-Expression, "
             "which PyPI refuses".format(name))
    for classifier in headers.get_all("Classifier") or []:
        if classifier.startswith("License ::"):
            fail("{}: METADATA carries the classifier {!r}, deprecated beside License-Expression".format(
                name, classifier))

    declared = headers.get_all("License-File") or []
    prefix = dist_info + "/licenses/"
    shipped = sorted(n[len(prefix):] for n in zf.namelist() if n.startswith(prefix))
    if sorted(declared) != LICENSE_FILES:
        fail("{}: License-File declares {}, want {}".format(name, sorted(declared), LICENSE_FILES))
    if shipped != sorted(declared):
        fail("{}: {} holds {} but METADATA declares {}".format(name, prefix, shipped, sorted(declared)))
    for license_file, source in LICENSE_SOURCES.items():
        arc = prefix + license_file
        if arc not in zf.namelist():
            continue
        with open(source, "rb") as fh:
            want = fh.read()
        if zf.read(arc) != want:
            fail("{}: {} is not the repository's {}".format(name, arc, os.path.relpath(source, ROOT)))
    arc = prefix + NOTICES
    if arc in zf.namelist():
        notices = zf.read(arc)
        if not notices.startswith(NOTICES_HEADER):
            fail("{}: {} does not open with the generator's header".format(name, arc))
        got = hashlib.sha256(notices).hexdigest()
        if notices_digest and got != notices_digest:
            fail("{}: {} is sha256 {}, but the release's signed checksums.txt named {}".format(
                name, arc, got, notices_digest))


def check_project_urls(name, metadata):
    """The issue tracker and the security policy under well-known labels."""
    urls = {}
    for value in metadata_headers(metadata).get_all("Project-URL") or []:
        label, _, url = value.partition(",")
        urls[label.strip()] = url.strip()
    for label, url in REQUIRED_PROJECT_URLS.items():
        if urls.get(label) != url:
            fail("{}: Project-URL {!r} is {!r}, want {!r}".format(name, label, urls.get(label), url))


def validate_wheel(path, version, tag, verified=None, notices_digest=None):
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
        check_licensing(zf, name, dist_info, metadata, notices_digest)
        check_project_urls(name, metadata)

        for info in zf.infolist():
            if (info.external_attr >> 16) & 0o170000 != 0o100000:
                fail("{}: {} is not a regular file (mode {:o})".format(
                    name, info.filename, info.external_attr >> 16))

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


def twine_check(wheels):
    """Run `twine check --strict` over the wheels when twine is importable.

    This is the reading PyPI's own upload path makes of the metadata, so a
    field it would refuse fails here instead of after a file name is burned.
    It is skipped, and says so, where twine is not installed: the
    validator stays standard library only, and the publish action runs the
    same check before it uploads.
    """
    if importlib.util.find_spec("twine") is None:
        print("twine: not installed here, skipping twine check (the publish action runs its own)")
        return
    result = subprocess.run([sys.executable, "-m", "twine", "check", "--strict"] + list(wheels),
                            capture_output=True, text=True, check=False)
    if result.returncode != 0:
        fail("twine check --strict refused the wheels:\n{}{}".format(result.stdout, result.stderr))
    else:
        print("twine: check --strict passed for", len(wheels), "wheels")


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
             "GITLAB_TOKEN": "glpat-validate-pypi", "GITLAB_MCP_LOG_LEVEL": "error",
             # The fake URL is unreachable by design; skipping scope
             # detection avoids its retry loop in the handshake path.
             "GITLAB_MCP_IGNORE_SCOPES": "true"},
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
    verified, notices_digest = read_verified(args.wheels, args.version)

    present = sorted(f for f in os.listdir(args.wheels) if f.endswith(".whl"))
    if set(present) != set(expected):
        fail("wheelhouse holds {} but the release needs exactly {}".format(present, sorted(expected)))

    leftovers = sorted(f for f in os.listdir(args.wheels) if not f.endswith(".whl"))
    if leftovers:
        fail("wheelhouse holds non-wheel files the publish step would try to upload: {}".format(leftovers))

    for fname in present:
        if fname in expected:
            validate_wheel(os.path.join(args.wheels, fname), args.version, expected[fname], verified,
                           notices_digest)
    if present:
        twine_check([os.path.join(args.wheels, fname) for fname in present])

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
