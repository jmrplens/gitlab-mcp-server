#!/usr/bin/env python3
"""Compare what npm and PyPI actually serve with the release's signed checksums.

Usage:
    python3 scripts/verify_published_packages.py <version> <checksums.txt>

<checksums.txt> is the release's own manifest, after its cosign signature has
been verified — in CI that is what scripts/fetch-release-assets.sh leaves
behind. Nothing here trusts the local build tree, and the binaries themselves
never have to be downloaded: the signed manifest already names their hashes.

Why this exists. The npm and PyPI packages are validated before publishing and
then **re-assembled** by the publish step, so the bytes that were validated are
not the bytes that went out; and of the six per-platform binaries, only the
runner's own is ever executed by any check. A stale or swapped binary for the
other five reaches two immutable registries with nothing in the pipeline
looking at it again. This runs after both publishes, in a job holding no
publishing credential, and reads the packages back out of the registries.

Standard library only, and anonymous: it talks to registry.npmjs.org and
pypi.org and needs no credential of any kind, so it is also the out-of-band
check to run days later.
"""

import argparse
import hashlib
import io
import json
import os
import sys
import tarfile
import urllib.error
import urllib.parse
import urllib.request
import zipfile

NPM_SCOPE = "@jmrp.io"
PYPI_DIST = "jmrplens-gitlab-mcp-server"
PYPI_NORM = PYPI_DIST.replace("-", "_")

# npm package suffix -> release asset name. The same table lives in
# scripts/build-npm.mjs; it is repeated rather than parsed because this script
# is meant to run against a checkout that may be older or newer than the
# release it is auditing.
NPM_ASSETS = {
    "linux-x64": "gitlab-mcp-server-linux-amd64",
    "linux-arm64": "gitlab-mcp-server-linux-arm64",
    "darwin-x64": "gitlab-mcp-server-darwin-amd64",
    "darwin-arm64": "gitlab-mcp-server-darwin-arm64",
    "win32-x64": "gitlab-mcp-server-windows-amd64.exe",
    "win32-arm64": "gitlab-mcp-server-windows-arm64.exe",
}

# Wheel platform tag fragment -> release asset name.
WHEEL_ASSETS = {
    "manylinux_2_17_x86_64": "gitlab-mcp-server-linux-amd64",
    "manylinux_2_17_aarch64": "gitlab-mcp-server-linux-arm64",
    "macosx_11_0_x86_64": "gitlab-mcp-server-darwin-amd64",
    "macosx_11_0_arm64": "gitlab-mcp-server-darwin-arm64",
    "win_amd64": "gitlab-mcp-server-windows-amd64.exe",
    "win_arm64": "gitlab-mcp-server-windows-arm64.exe",
}

TIMEOUT = 120


def fetch(url):
    request = urllib.request.Request(url, headers={"User-Agent": "gitlab-mcp-server-release-audit"})
    with urllib.request.urlopen(request, timeout=TIMEOUT) as response:  # noqa: S310 - fixed https hosts
        return response.read()


def sha256(data):
    return hashlib.sha256(data).hexdigest()


def released_digests(checksums_path):
    """Parse `sha256  name` lines from the release's signed checksums.txt."""
    wanted = set(NPM_ASSETS.values()) | set(WHEEL_ASSETS.values())
    digests = {}
    with open(checksums_path, encoding="utf-8") as fh:
        for line in fh:
            parts = line.replace("\r", "").strip().split(None, 1)
            if len(parts) != 2:
                continue
            digest, name = parts[0], parts[1].lstrip("*")
            if name in wanted:
                digests[name] = digest
    return digests


def npm_binary(suffix, version):
    """Download the published npm platform package and return its binary bytes."""
    name = f"{NPM_SCOPE}/gitlab-mcp-server-{suffix}"
    meta = json.loads(fetch(f"https://registry.npmjs.org/{urllib.parse.quote(name, safe='')}/{version}"))
    tarball = meta["dist"]["tarball"]
    blob = fetch(tarball)
    with tarfile.open(fileobj=io.BytesIO(blob), mode="r:gz") as tar:
        for member in tar.getmembers():
            base = os.path.basename(member.name)
            if member.isfile() and base in ("gitlab-mcp-server", "gitlab-mcp-server.exe"):
                return tar.extractfile(member).read(), tarball
    raise LookupError(f"{name}@{version} ships no gitlab-mcp-server binary")


def pypi_wheels(version):
    """Yield (filename, url) for every wheel of this release."""
    meta = json.loads(fetch(f"https://pypi.org/pypi/{PYPI_DIST}/{version}/json"))
    for entry in meta.get("urls", []):
        if entry.get("packagetype") == "bdist_wheel":
            yield entry["filename"], entry["url"]


def wheel_binary(url, version):
    blob = fetch(url)
    with zipfile.ZipFile(io.BytesIO(blob)) as zf:
        prefix = f"{PYPI_NORM}-{version}.data/scripts/"
        for name in zf.namelist():
            if name.startswith(prefix) and os.path.basename(name).startswith("gitlab-mcp-server"):
                return zf.read(name)
    raise LookupError(f"{url} ships no binary under {PYPI_NORM}-{version}.data/scripts/")


def check_npm(version, digests, problems):
    for suffix, asset in NPM_ASSETS.items():
        want = digests.get(asset)
        if want is None:
            problems.append(f"npm {suffix}: checksums.txt does not name the release asset {asset}")
            continue
        try:
            binary, tarball = npm_binary(suffix, version)
        except (urllib.error.URLError, LookupError, KeyError) as exc:
            problems.append(f"npm {suffix}: could not read the published package: {exc}")
            continue
        got = sha256(binary)
        if got != want:
            problems.append(
                f"npm {suffix}: {tarball} carries sha256 {got}, but the signed checksums.txt says {asset} is {want}"
            )
        else:
            print(f"  ok  npm {suffix:<13} matches {asset}")


def check_pypi(version, digests, problems):
    seen = set()
    try:
        wheels = list(pypi_wheels(version))
    except (urllib.error.URLError, KeyError) as exc:
        problems.append(f"pypi: could not list the published wheels: {exc}")
        return
    for filename, url in wheels:
        asset = next((a for frag, a in WHEEL_ASSETS.items() if frag in filename), None)
        if asset is None:
            problems.append(f"pypi {filename}: no release asset corresponds to this platform tag")
            continue
        seen.add(asset)
        want = digests.get(asset)
        if want is None:
            problems.append(f"pypi {filename}: checksums.txt does not name the release asset {asset}")
            continue
        try:
            binary = wheel_binary(url, version)
        except (urllib.error.URLError, LookupError) as exc:
            problems.append(f"pypi {filename}: could not read the wheel: {exc}")
            continue
        got = sha256(binary)
        if got != want:
            problems.append(
                f"pypi {filename}: carries sha256 {got}, but the signed checksums.txt says {asset} is {want}"
            )
        else:
            print(f"  ok  pypi {filename:<60} matches {asset}")
    missing = sorted(set(WHEEL_ASSETS.values()) - seen)
    if missing:
        problems.append(f"pypi: no wheel published for {', '.join(missing)}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("version", help="release version without the leading v, e.g. 2.7.6")
    parser.add_argument("checksums", help="path to the release's checksums.txt (or a directory holding it)")
    parser.add_argument("--skip-npm", action="store_true", help="check PyPI only")
    parser.add_argument("--skip-pypi", action="store_true", help="check npm only")
    args = parser.parse_args()

    checksums = args.checksums
    if os.path.isdir(checksums):
        checksums = os.path.join(checksums, "checksums.txt")
    digests = released_digests(checksums)
    if not digests:
        sys.exit(f"verify_published_packages: {checksums} names none of the release binaries")

    print(f"Comparing published packages for v{args.version} against {len(digests)} entries in {checksums}")
    problems = []
    if not args.skip_npm:
        check_npm(args.version, digests, problems)
    if not args.skip_pypi:
        check_pypi(args.version, digests, problems)

    if problems:
        print(f"\nFAILED ({len(problems)}):")
        for problem in problems:
            print(f"  x {problem}")
        sys.exit(1)
    print("\nEvery published package carries the binary the release signed.")


if __name__ == "__main__":
    main()
