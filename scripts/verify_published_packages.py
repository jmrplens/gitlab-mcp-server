#!/usr/bin/env python3
"""Compare what npm, PyPI and NuGet actually serve with the release's signed checksums.

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
other five reaches immutable registries with nothing in the pipeline looking
at it again. This runs after the publishes, in a job holding no publishing
credential, and reads the packages back out of the registries. NuGet joined
later with the same shape: six runtime-identifier packages each carrying one
binary, read back from the flat container the SDK itself installs from.

Standard library only, and anonymous: it talks to registry.npmjs.org, pypi.org
and api.nuget.org and needs no credential of any kind, so it is also the
out-of-band check to run days later.

Retries. This job gates the ones that advertise the release, so it is the
strictest gate in the pipeline, and it runs minutes after the uploads it reads
back. A version that has not propagated to a registry CDN yet answers 404 —
the same not-yet-visible state the mcp-registry job already waits out at 60s
intervals. Every download therefore draws on a shared retry budget
(--retry-budget, --retry-delay; pass 0 for the out-of-band run, where there is
no lag left to wait for).

What is never retried is a mismatch. Retrying lives strictly inside fetch(),
so by the time a digest is compared the bytes are already in hand and a failed
comparison is reported once, immediately. Waiting can turn "not there yet"
into a pass, which is the point, and can never turn "these are the wrong
bytes" into one, which is the finding this job exists to make.
"""

import argparse
import hashlib
import io
import json
import os
import sys
import tarfile
import time
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

# NuGet: the pointer package id, and runtime identifier -> release asset name.
# The same table lives in scripts/build_nuget.py, repeated for the reason the
# npm one is.
NUGET_ID = "gitlab-mcp-server"
NUGET_FLAT = "https://api.nuget.org/v3-flatcontainer"
NUGET_ASSETS = {
    "linux-x64": "gitlab-mcp-server-linux-amd64",
    "linux-arm64": "gitlab-mcp-server-linux-arm64",
    "osx-x64": "gitlab-mcp-server-darwin-amd64",
    "osx-arm64": "gitlab-mcp-server-darwin-arm64",
    "win-x64": "gitlab-mcp-server-windows-amd64.exe",
    "win-arm64": "gitlab-mcp-server-windows-arm64.exe",
}

TIMEOUT = 120

# Seconds of retry sleep the whole run may spend, and the pause between
# attempts. The budget is shared rather than granted per URL because the
# failure modes here are correlated: either a registry has this version or it
# does not, and letting each of six npm packages wait out a budget of its own
# would turn one unpublished release into an hour of CI. A slow registry is
# absorbed by the first download that waits for it; a genuinely absent version
# exhausts the budget once and then fails the remaining checks promptly.
RETRY_BUDGET = 600.0
RETRY_DELAY = 20.0


def describe(exc):
    """Render a fetch failure the way a release engineer needs to read it."""
    if isinstance(exc, urllib.error.HTTPError):
        return f"HTTP {exc.code} {exc.reason}"
    return f"{type(exc).__name__}: {getattr(exc, 'reason', exc)}"


class FetchError(urllib.error.URLError):
    """A download that never succeeded, carrying what it took to know that.

    It subclasses URLError so every caller's existing except clause keeps
    catching it, and it reports the attempt count so the log distinguishes a
    blip that was waited out from a version that was never there.
    """

    def __init__(self, url, attempts, cause):
        super().__init__(describe(cause))
        self.url = url
        self.attempts = attempts
        self.cause = cause

    def __str__(self):
        return f"{self.reason} (still failing after {self.attempts} attempt(s))"


class RetryBudget:
    """A shared allowance, in seconds, of sleeping between download attempts.

    sleep is injected so a test can exercise the retry path without spending
    the wall-clock time it describes.
    """

    def __init__(self, seconds=RETRY_BUDGET, delay=RETRY_DELAY, sleep=time.sleep):
        self.remaining = float(seconds)
        self.delay = float(delay)
        self.waits = 0
        self._sleep = sleep

    def wait(self):
        """Pause before another attempt, and report whether one was allowed."""
        if self.delay <= 0 or self.remaining < self.delay:
            return False
        self.remaining -= self.delay
        self.waits += 1
        self._sleep(self.delay)
        return True


def fetch(url, budget=None):
    """GET url, spending the shared budget on anything that fails in transit.

    A 404 is retried like any other failure: minutes after an upload it is
    indistinguishable from propagation lag. It is not forgiven — once the
    budget is spent the FetchError becomes a reported problem and the job
    fails — it is only waited for.
    """
    request = urllib.request.Request(url, headers={"User-Agent": "gitlab-mcp-server-release-audit"})
    attempts = 0
    while True:
        attempts += 1
        try:
            with urllib.request.urlopen(request, timeout=TIMEOUT) as response:  # noqa: S310 - fixed https hosts
                return response.read()
        except urllib.error.URLError as exc:
            if budget is None or not budget.wait():
                raise FetchError(url, attempts, exc) from exc
            print(f"  .. {url}: {describe(exc)}; retrying (attempt {attempts + 1})", flush=True)


def sha256(data):
    return hashlib.sha256(data).hexdigest()


def released_digests(checksums_path):
    """Parse `sha256  name` lines from the release's signed checksums.txt."""
    wanted = set(NPM_ASSETS.values()) | set(WHEEL_ASSETS.values()) | set(NUGET_ASSETS.values())
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


def npm_binary(suffix, version, budget=None):
    """Download the published npm platform package and return its binary bytes."""
    name = f"{NPM_SCOPE}/gitlab-mcp-server-{suffix}"
    meta = json.loads(fetch(f"https://registry.npmjs.org/{urllib.parse.quote(name, safe='')}/{version}", budget))
    tarball = meta["dist"]["tarball"]
    blob = fetch(tarball, budget)
    with tarfile.open(fileobj=io.BytesIO(blob), mode="r:gz") as tar:
        for member in tar.getmembers():
            base = os.path.basename(member.name)
            if member.isfile() and base in ("gitlab-mcp-server", "gitlab-mcp-server.exe"):
                return tar.extractfile(member).read(), tarball
    raise LookupError(f"{name}@{version} ships no gitlab-mcp-server binary")


def pypi_wheels(version, budget=None):
    """Yield (filename, url) for every wheel of this release."""
    meta = json.loads(fetch(f"https://pypi.org/pypi/{PYPI_DIST}/{version}/json", budget))
    for entry in meta.get("urls", []):
        if entry.get("packagetype") == "bdist_wheel":
            yield entry["filename"], entry["url"]


def wheel_binary(url, version, budget=None):
    blob = fetch(url, budget)
    with zipfile.ZipFile(io.BytesIO(blob)) as zf:
        prefix = f"{PYPI_NORM}-{version}.data/scripts/"
        for name in zf.namelist():
            if name.startswith(prefix) and os.path.basename(name).startswith("gitlab-mcp-server"):
                return zf.read(name)
    raise LookupError(f"{url} ships no binary under {PYPI_NORM}-{version}.data/scripts/")


def nuget_package_url(pkg_id, version):
    """The flat-container download of one package, the URL the SDK itself
    installs from. Ids and versions are lower-cased there."""
    pkg_id = pkg_id.lower()
    version = version.lower()
    return f"{NUGET_FLAT}/{pkg_id}/{version}/{pkg_id}.{version}.nupkg"


def nuget_versions(pkg_id, budget=None):
    """The versions the flat container lists for a package id."""
    index = json.loads(fetch(f"{NUGET_FLAT}/{pkg_id.lower()}/index.json", budget))
    return [v.lower() for v in index.get("versions", [])]


def nuget_binary(rid, version, budget=None):
    """Download the published runtime package and return its binary bytes."""
    url = nuget_package_url(f"{NUGET_ID}.{rid}", version)
    blob = fetch(url, budget)
    with zipfile.ZipFile(io.BytesIO(blob)) as zf:
        prefix = f"tools/any/{rid}/"
        for name in zf.namelist():
            if name.startswith(prefix) and os.path.basename(name) in ("gitlab-mcp-server", "gitlab-mcp-server.exe"):
                return zf.read(name), url
    raise LookupError(f"{url} ships no gitlab-mcp-server binary under {prefix}")


def check_npm(version, digests, problems, budget=None):
    for suffix, asset in NPM_ASSETS.items():
        want = digests.get(asset)
        if want is None:
            problems.append(f"npm {suffix}: checksums.txt does not name the release asset {asset}")
            continue
        try:
            binary, tarball = npm_binary(suffix, version, budget)
        except (urllib.error.URLError, LookupError, KeyError) as exc:
            problems.append(f"npm {suffix}: could not read the published package: {exc}")
            continue
        # Past this point the bytes are in hand, so nothing below is retried.
        got = sha256(binary)
        if got != want:
            problems.append(
                f"npm {suffix}: {tarball} carries sha256 {got}, but the signed checksums.txt says {asset} is {want}"
            )
        else:
            print(f"  ok  npm {suffix:<13} matches {asset}")


def check_pypi(version, digests, problems, budget=None):
    seen = set()
    try:
        wheels = list(pypi_wheels(version, budget))
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
            binary = wheel_binary(url, version, budget)
        except (urllib.error.URLError, LookupError) as exc:
            problems.append(f"pypi {filename}: could not read the wheel: {exc}")
            continue
        # Past this point the bytes are in hand, so nothing below is retried.
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


def check_nuget(version, digests, problems, budget=None):
    # The pointer carries no binary, so its check is that the version is
    # listed at all: a runtime package nobody points at is not installable,
    # and a pointer published without its runtime packages installs nothing.
    try:
        versions = nuget_versions(NUGET_ID, budget)
    except (urllib.error.URLError, ValueError) as exc:
        problems.append(f"nuget {NUGET_ID}: could not list the published versions: {exc}")
    else:
        if version.lower() not in versions:
            problems.append(f"nuget {NUGET_ID}: version {version} is not listed on nuget.org")
        else:
            print(f"  ok  nuget {NUGET_ID:<32} lists {version}")
    for rid, asset in NUGET_ASSETS.items():
        want = digests.get(asset)
        if want is None:
            problems.append(f"nuget {rid}: checksums.txt does not name the release asset {asset}")
            continue
        try:
            binary, url = nuget_binary(rid, version, budget)
        except (urllib.error.URLError, LookupError, zipfile.BadZipFile) as exc:
            problems.append(f"nuget {rid}: could not read the published package: {exc}")
            continue
        # Past this point the bytes are in hand, so nothing below is retried.
        got = sha256(binary)
        if got != want:
            problems.append(
                f"nuget {rid}: {url} carries sha256 {got}, but the signed checksums.txt says {asset} is {want}"
            )
        else:
            print(f"  ok  nuget {rid:<32} matches {asset}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("version", help="release version without the leading v, e.g. 2.7.6")
    parser.add_argument("checksums", help="path to the release's checksums.txt (or a directory holding it)")
    parser.add_argument("--skip-npm", action="store_true", help="do not check npm")
    parser.add_argument("--skip-pypi", action="store_true", help="do not check PyPI")
    parser.add_argument("--skip-nuget", action="store_true", help="do not check NuGet")
    parser.add_argument(
        "--retry-budget",
        type=float,
        default=RETRY_BUDGET,
        metavar="SECONDS",
        help="total seconds this run may spend waiting for a registry to catch up (0 disables retrying)",
    )
    parser.add_argument(
        "--retry-delay",
        type=float,
        default=RETRY_DELAY,
        metavar="SECONDS",
        help="pause between download attempts",
    )
    args = parser.parse_args()

    checksums = args.checksums
    if os.path.isdir(checksums):
        checksums = os.path.join(checksums, "checksums.txt")
    digests = released_digests(checksums)
    if not digests:
        sys.exit(f"verify_published_packages: {checksums} names none of the release binaries")

    print(f"Comparing published packages for v{args.version} against {len(digests)} entries in {checksums}")
    budget = RetryBudget(args.retry_budget, args.retry_delay)
    problems = []
    if not args.skip_npm:
        check_npm(args.version, digests, problems, budget)
    if not args.skip_pypi:
        check_pypi(args.version, digests, problems, budget)
    if not args.skip_nuget:
        check_nuget(args.version, digests, problems, budget)

    if problems:
        print(f"\nFAILED ({len(problems)}):")
        for problem in problems:
            print(f"  x {problem}")
        if budget.waits:
            print(
                f"\nRetried {budget.waits} time(s); {int(budget.remaining)}s of the retry budget was left unspent. "
                "A budget spent to zero means a download never came back at all, "
                "which reads differently from a digest that did not match."
            )
        sys.exit(1)
    print("\nEvery published package carries the binary the release signed.")


if __name__ == "__main__":
    main()
