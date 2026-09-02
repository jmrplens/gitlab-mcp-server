#!/usr/bin/env python3
"""Assemble the PyPI distribution from released binaries.

Builds one platform wheel per release binary, the esbuild/uv/ruff model
adapted to Python packaging: each wheel carries the native gitlab-mcp-server
binary plus a launcher module exposing a console script, and pip selects the
wheel whose platform tag matches the host. Nothing is compiled here and
nothing is downloaded at install time.

Standard library only, so the release job and a contributor's machine need a
Python interpreter and nothing else.

Usage:
    python3 scripts/build_pypi.py --binaries dist --version 2.7.5 [--out pypi/dist]

The binaries directory must hold the GoReleaser release assets under their
exact release names (gitlab-mcp-server-<os>-<arch>[.exe]) together with the
release's own checksums.txt, which is cosign-signed at build time. Every
binary is checked against it before it goes into a wheel: the wheels used to
be assembled from an unverified copy of the build directory, and a published
PyPI file name is burned forever. Pass --allow-unverified only when there is
genuinely no manifest (never in CI).

Wheels land in --out (default pypi/dist), which is wiped first so a rebuild
cannot mix versions.
"""

import argparse
import base64
import hashlib
import json
import os
import re
import shutil
import sys
import zipfile

# The unprefixed name gitlab-mcp-server is registered empty on PyPI by an
# unrelated account and is under PEP 541 reclamation; until that resolves,
# the distribution ships under the author-prefixed name. The IMPORT package
# and the native command keep the unprefixed name, so client configurations
# never change: uvx matches the distribution name through a console-script
# wrapper, and the reclaimed name will slot in as a rename of DIST_NAME.
DIST_NAME = "jmrplens-gitlab-mcp-server"
NORM_NAME = DIST_NAME.replace("-", "_")
PKG_NAME = "gitlab_mcp_server"
COMMAND = "gitlab-mcp-server"

# Release-asset name fragments mapped to wheel platform tags. The Linux
# binaries are PIE executables linked against glibc, so the wheels carry
# manylinux tags (validate_pypi.py enforces the glibc_2_17 floor on the
# binary itself); Alpine/musl users are served by the container image, the
# same boundary the npm distribution draws.
PLATFORMS = {
    "linux-amd64": "manylinux_2_17_x86_64.manylinux2014_x86_64",
    "linux-arm64": "manylinux_2_17_aarch64.manylinux2014_aarch64",
    "darwin-amd64": "macosx_11_0_x86_64",
    "darwin-arm64": "macosx_11_0_arm64",
    "windows-amd64": "win_amd64",
    "windows-arm64": "win_arm64",
}

# Deterministic zip entry timestamp (zip's epoch): rebuilding the same
# inputs yields byte-identical wheels.
ZIP_DATE = (1980, 1, 1, 0, 0, 0)

LAUNCHER = '''\
"""Locator for the gitlab-mcp-server binary installed by this wheel.

The binary itself ships in the wheel's .data/scripts directory, so the
installer places it on the scripts path (bin/ or Scripts/) with the
executable bit set and it IS the `gitlab-mcp-server` command; no Python
runs in that path. This module exists for `python -m gitlab_mcp_server`
and for programmatic lookup, the same shape uv's find_uv_bin takes.
"""

import os
import subprocess
import sys
import sysconfig

__version__ = "{version}"

_BIN = "gitlab-mcp-server.exe" if os.name == "nt" else "gitlab-mcp-server"


def find_binary():
    """Return the installed binary's path, searching the scripts
    directories of the running environment."""
    candidates = [
        sysconfig.get_path("scripts"),
        os.path.join(sys.prefix, "Scripts" if os.name == "nt" else "bin"),
        os.path.dirname(sys.executable),
    ]
    for scripts_dir in candidates:
        if not scripts_dir:
            continue
        path = os.path.join(scripts_dir, _BIN)
        if os.path.isfile(path):
            return path
    raise FileNotFoundError(
        "gitlab-mcp-server binary not found next to " + sys.executable)


def main():
    path = find_binary()
    argv = [path] + sys.argv[1:]
    if os.name == "nt":
        try:
            raise SystemExit(subprocess.call(argv))
        except KeyboardInterrupt:
            raise SystemExit(130) from None
    os.execv(path, argv)
'''

MAIN_MODULE = '''\
from gitlab_mcp_server import main

if __name__ == "__main__":
    main()
'''


def build_metadata(version, readme):
    headers = [
        ("Metadata-Version", "2.1"),
        ("Name", DIST_NAME),
        ("Version", version),
        ("Summary", "GitLab MCP server: GitLab REST v4 and GraphQL as Model Context Protocol tools (native Go binary)"),
        ("Author", "jmrplens"),
        ("License", "MIT"),
        ("Project-URL", "Homepage, https://github.com/jmrplens/gitlab-mcp-server"),
        ("Project-URL", "Documentation, https://jmrp.io/docs/gitlab-mcp-server/"),
        ("Project-URL", "Repository, https://github.com/jmrplens/gitlab-mcp-server"),
        ("Project-URL", "Changelog, https://github.com/jmrplens/gitlab-mcp-server/releases"),
        ("Classifier", "License :: OSI Approved :: MIT License"),
        ("Classifier", "Development Status :: 5 - Production/Stable"),
        ("Classifier", "Intended Audience :: Developers"),
        ("Classifier", "Topic :: Software Development"),
        ("Classifier", "Programming Language :: Go"),
        ("Requires-Python", ">=3.9"),
        ("Description-Content-Type", "text/markdown"),
    ]
    lines = ["{}: {}".format(k, v) for k, v in headers]
    return "\n".join(lines) + "\n\n" + readme


def wheel_file(tag):
    return (
        "Wheel-Version: 1.0\n"
        "Generator: gitlab-mcp-server build_pypi.py\n"
        "Root-Is-Purelib: false\n"
        "Tag: py3-none-{}\n".format(tag)
    )


def read_checksums(binaries_dir):
    """Parse `<sha256>  <name>` lines from the release's checksums.txt.

    Returns None when the file is absent, so the caller decides whether an
    unverified build is acceptable.
    """
    path = os.path.join(binaries_dir, "checksums.txt")
    if not os.path.isfile(path):
        return None
    entries = {}
    with open(path, encoding="utf-8") as fh:
        for line in fh:
            parts = line.replace("\r", "").strip().split(None, 1)
            if len(parts) == 2 and re.fullmatch(r"[0-9a-f]{64}", parts[0]):
                entries[parts[1].lstrip("*")] = parts[0]
    return entries


def verify_binary(binary_path, checksums):
    """Abort unless the file matches the digest the signed manifest names."""
    name = os.path.basename(binary_path)
    want = checksums.get(name)
    if want is None:
        sys.exit("build_pypi: {} is not listed in checksums.txt".format(name))
    with open(binary_path, "rb") as fh:
        got = hashlib.sha256(fh.read()).hexdigest()
    if got != want:
        sys.exit("build_pypi: {} is sha256 {}, but checksums.txt says {}".format(name, got, want))
    return got


def record_hash(data):
    digest = hashlib.sha256(data).digest()
    return "sha256=" + base64.urlsafe_b64encode(digest).decode("ascii").rstrip("=")


def add_file(zf, records, arcname, data, executable=False):
    if isinstance(data, str):
        data = data.encode("utf-8")
    info = zipfile.ZipInfo(arcname, date_time=ZIP_DATE)
    # S_IFREG matters: pip's zip_item_is_executable requires S_ISREG(mode)
    # before honoring the 0o111 bits, so permission bits alone install the
    # binary without +x.
    mode = 0o100755 if executable else 0o100644
    info.external_attr = mode << 16
    info.compress_type = zipfile.ZIP_DEFLATED
    zf.writestr(info, data)
    records.append("{},{},{}".format(arcname, record_hash(data), len(data)))


def build_wheel(out_dir, version, plat_key, tag, binary_path, readme):
    dist_info = "{}-{}.dist-info".format(NORM_NAME, version)
    wheel_name = "{}-{}-py3-none-{}.whl".format(NORM_NAME, version, tag)
    wheel_path = os.path.join(out_dir, wheel_name)

    with open(binary_path, "rb") as fh:
        binary = fh.read()
    bin_name = "gitlab-mcp-server.exe" if plat_key.startswith("windows") else "gitlab-mcp-server"

    # The binary lives in .data/scripts, which the wheel spec obliges the
    # installer to place on the environment's scripts path with the
    # executable bit set. A binary inside the package directory does not get
    # that guarantee: pip installed it without +x and the launcher died with
    # PermissionError, which is exactly why uv and ruff ship theirs this way.
    data_scripts = "{}-{}.data/scripts".format(NORM_NAME, version)

    records = []
    with zipfile.ZipFile(wheel_path, "w") as zf:
        add_file(zf, records, PKG_NAME + "/__init__.py", LAUNCHER.format(version=version))
        add_file(zf, records, PKG_NAME + "/__main__.py", MAIN_MODULE)
        add_file(zf, records, data_scripts + "/" + bin_name, binary, executable=True)
        add_file(zf, records, dist_info + "/METADATA", build_metadata(version, readme))
        add_file(zf, records, dist_info + "/WHEEL", wheel_file(tag))
        add_file(zf, records, dist_info + "/entry_points.txt",
                 "[console_scripts]\n{} = {}:main\n".format(DIST_NAME, PKG_NAME))
        record_name = dist_info + "/RECORD"
        records.append("{},,".format(record_name))
        record_data = "\n".join(records) + "\n"
        info = zipfile.ZipInfo(record_name, date_time=ZIP_DATE)
        info.external_attr = 0o644 << 16
        info.compress_type = zipfile.ZIP_DEFLATED
        zf.writestr(info, record_data)
    return wheel_name


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binaries", required=True, help="directory holding the release binaries")
    parser.add_argument("--version", required=True, help="release version, e.g. 2.7.5")
    parser.add_argument("--out", default="pypi/dist", help="wheelhouse output directory")
    parser.add_argument(
        "--allow-unverified",
        action="store_true",
        help="build without a checksums.txt manifest (never in CI)",
    )
    args = parser.parse_args()

    if not re.fullmatch(r"\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?", args.version):
        sys.exit("build_pypi: version {!r} does not look like a release version".format(args.version))

    readme_path = os.path.join(os.path.dirname(__file__), "..", "pypi", "README.md")
    with open(readme_path, encoding="utf-8") as fh:
        readme = fh.read()
    if "mcp-name: io.github.jmrplens/gitlab-mcp-server" not in readme:
        sys.exit("build_pypi: pypi/README.md lost the mcp-name ownership token the MCP Registry validates")

    checksums = read_checksums(args.binaries)
    if checksums is None and not args.allow_unverified:
        sys.exit(
            "build_pypi: {} not found — refusing to package unverified binaries "
            "(pass --allow-unverified to override)".format(os.path.join(args.binaries, "checksums.txt"))
        )
    if checksums is None:
        sys.stderr.write("WARNING: --allow-unverified — wheels are being built without a checksum manifest\n")

    if os.path.isdir(args.out):
        shutil.rmtree(args.out)
    os.makedirs(args.out)

    built = []
    digests = {}
    for plat_key, tag in PLATFORMS.items():
        suffix = ".exe" if plat_key.startswith("windows") else ""
        binary_path = os.path.join(args.binaries, "{}-{}{}".format(COMMAND, plat_key, suffix))
        if not os.path.isfile(binary_path):
            sys.exit("build_pypi: missing release binary {}".format(binary_path))
        if checksums is not None:
            digests[plat_key] = verify_binary(binary_path, checksums)
        built.append(build_wheel(args.out, args.version, plat_key, tag, binary_path, readme))

    # Record what was verified so validate_pypi.py can confirm the wheels still
    # carry those exact bytes. Written beside the wheels, never inside one.
    with open(os.path.join(args.out, "verified-binaries.json"), "w", encoding="utf-8") as fh:
        json.dump({"version": args.version, "verified": checksums is not None, "binaries": digests}, fh, indent=2)
        fh.write("\n")

    for name in built:
        print("built", os.path.join(args.out, name))
    print("build_pypi: {} wheels for version {}".format(len(built), args.version))


if __name__ == "__main__":
    main()
