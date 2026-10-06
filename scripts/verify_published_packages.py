#!/usr/bin/env python3
"""Compare what npm, PyPI and NuGet actually serve with the release's signed checksums.

Usage:
    python3 scripts/verify_published_packages.py <version> <checksums.txt> [--nuget-digests <file>]
        [--retry-budget <seconds>] [--retry-delay <seconds>] [--nuget-deadline <seconds>]

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

NuGet packages are also compared whole. The nuget job attests the seven
.nupkg files before it pushes them and records their SHA-256 values
(--nuget-digests, sha256sum's format). nuget.org adds its own repository
signature, a .signature.p7s entry, to every package it serves, so the served
bytes never match those values; a verifier removes that entry the way NuGet
defines an unsigned package and looks up the digest of what is left. This
does the same and compares the result with the recorded values, so a packer
change that writes a layout the signing rearranges, or a signing change at
nuget.org, fails here rather than leaving an attestation nobody can find.
The attestation itself is not looked up: the comparison proves nuget.org
serves exactly the bytes the job attested, and the job fails if the
attestation step does.

The third-party notices are compared too. From the first release after
3.1.0, THIRD_PARTY_NOTICES is a release asset listed in checksums.txt, and
every npm platform package and every wheel carries a copy beside LICENSE
(package/THIRD_PARTY_NOTICES, and .dist-info/licenses/THIRD_PARTY_NOTICES);
each copy read back must hash to the signed entry, and a package without one
is a finding. A checksums.txt that names no notices is a release from before
they were generated, so the comparison is skipped and says so. The NuGet
packages are covered by the whole-package comparison above when the attested
digests are given. LICENSE is not a release asset, so there is no signed
digest to hold it to.

Standard library only, and anonymous: it talks to registry.npmjs.org, pypi.org
and api.nuget.org and needs no credential of any kind, so it is also the
out-of-band check to run days later.

Retries. This job gates the ones that advertise the release, so it is the
strictest gate in the pipeline, and it runs minutes after the uploads it reads
back. A version a registry has not finished publishing answers 404, the same
not-yet-visible state the mcp-registry job already waits out at 60s
intervals, so it is waited for rather than reported, from one of two
allowances (--retry-delay is the pause between attempts under both).

npm and PyPI share a retry budget (--retry-budget), counted in the seconds
slept between attempts. Both make a version visible within minutes, and
either a registry has the version or it does not, so a slow one is absorbed by
the first download that waits for it and an absent one spends the budget once
and fails the rest promptly.

NuGet has a deadline of its own (--nuget-deadline), counted in wall-clock
seconds from the start of the run rather than in sleep granted when its check
begins, because nuget.org validates every push before it serves it and does
so while npm and PyPI are being waited for. On 3.1.0 that took 23 minutes from
this step's start: the seven packages were pushed at 19:29 UTC, committed to
nuget.org's catalog between 19:48 and 19:52 and all served by 19:53, while
npm's lag had already spent 13 of the 16 waits of the one budget the run then
had, and the step failed. Until the version is served, the pointer's
flat-container index answers with the versions published before it, so an
index that does not list this version yet is waited for from the deadline
like a 404, and reported only once the deadline is spent.

--retry-budget 0 disables every wait, NuGet's included: that is the
out-of-band run, days later, where there is no lag left to wait for.

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
import re
import struct
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

# The release asset holding the third-party notices, the name every package
# carries its copy under.
NOTICES = "THIRD_PARTY_NOTICES"

# The entry nuget.org's repository signature adds, and the zip records the
# unsigning below reads.
NUGET_SIGNATURE_ENTRY = ".signature.p7s"
ZIP_EOCD = b"PK\x05\x06"
ZIP_EOCD_SIZE = 22
ZIP_CENTRAL_HEADER = b"PK\x01\x02"
ZIP_CENTRAL_HEADER_SIZE = 46

TIMEOUT = 120

# Seconds of retry sleep the npm and PyPI checks may spend between them, and
# the pause between attempts, which the NuGet deadline below uses too. The
# budget is shared rather than granted per URL because the failure modes here
# are correlated: either a registry has this version or it does not, and
# letting each of six npm packages wait out a budget of its own would turn one
# unpublished release into an hour of CI. A slow registry is absorbed by the
# first download that waits for it; a genuinely absent version exhausts the
# budget once and then fails the remaining checks promptly. NuGet does not
# draw on it: npm's lag alone spent 390 of the 480 seconds the release job
# gives this budget on 3.1.0.
RETRY_BUDGET = 600.0
RETRY_DELAY = 20.0

# How long, in seconds from the start of the run, the NuGet checks may keep
# waiting for nuget.org to serve the version. Sized on 3.1.0, whose last
# package was served 23 minutes after this step started (20 to 25 after the
# push), with margin; 3.0.0's was served about nine minutes after its push.
# One allowance serves all seven packages for the correlation the budget above
# rests on, and it ends at an instant rather than after an amount of sleep
# because nuget.org validates while npm and PyPI are being waited for: their
# time is time nuget.org spent validating, and an allowance granted only when
# NuGet's check began would make how long the step can run depend on how long
# npm lagged, so no job timeout could be sized against it. The release job's
# timeout has to exceed this by the downloads that follow its end.
NUGET_DEADLINE = 2400.0


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


class RetryDeadline:
    """An allowance of waiting between attempts that ends at an instant.

    The instant is fixed when the deadline is made, so time spent before its
    first wait counts against it. A wait is allowed only when it would end by
    the deadline, which is RetryBudget's rule read on a clock. clock and sleep
    are injected so a test can drive a deadline of forty minutes in no time.
    """

    def __init__(self, seconds=NUGET_DEADLINE, delay=RETRY_DELAY, clock=time.monotonic, sleep=time.sleep):
        self.seconds = float(seconds)
        self.delay = float(delay)
        self.waits = 0
        self._clock = clock
        self._sleep = sleep
        self.end = clock() + self.seconds

    @property
    def remaining(self):
        """Seconds left until the deadline, never negative."""
        return max(0.0, self.end - self._clock())

    def wait(self):
        """Pause before another attempt, and report whether one was allowed."""
        if self.delay <= 0 or self.remaining < self.delay:
            return False
        self.waits += 1
        self._sleep(self.delay)
        return True


def fetch(url, budget=None):
    """GET url, spending budget, a RetryBudget or a RetryDeadline, on anything
    that fails in transit.

    A 404 is retried like any other failure: minutes after an upload it is
    indistinguishable from propagation lag. It is not forgiven (once the
    allowance is spent the FetchError becomes a reported problem and the job
    fails); it is only waited for.
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
    wanted = set(NPM_ASSETS.values()) | set(WHEEL_ASSETS.values()) | set(NUGET_ASSETS.values()) | {NOTICES}
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


def npm_package(suffix, version, budget=None):
    """Download the published npm platform package and return its binary
    bytes, its THIRD_PARTY_NOTICES bytes (None when it carries none) and the
    tarball's URL."""
    name = f"{NPM_SCOPE}/gitlab-mcp-server-{suffix}"
    meta = json.loads(fetch(f"https://registry.npmjs.org/{urllib.parse.quote(name, safe='')}/{version}", budget))
    tarball = meta["dist"]["tarball"]
    blob = fetch(tarball, budget)
    binary = notices = None
    with tarfile.open(fileobj=io.BytesIO(blob), mode="r:gz") as tar:
        for member in tar.getmembers():
            if not member.isfile():
                continue
            if os.path.basename(member.name) in ("gitlab-mcp-server", "gitlab-mcp-server.exe"):
                binary = tar.extractfile(member).read()
            elif member.name == "package/" + NOTICES:
                notices = tar.extractfile(member).read()
    if binary is None:
        raise LookupError(f"{name}@{version} ships no gitlab-mcp-server binary")
    return binary, notices, tarball


def pypi_wheels(version, budget=None):
    """Yield (filename, url) for every wheel of this release."""
    meta = json.loads(fetch(f"https://pypi.org/pypi/{PYPI_DIST}/{version}/json", budget))
    for entry in meta.get("urls", []):
        if entry.get("packagetype") == "bdist_wheel":
            yield entry["filename"], entry["url"]


def wheel_contents(url, version, budget=None):
    """Download one wheel and return its binary bytes and its
    THIRD_PARTY_NOTICES bytes (None when it carries none)."""
    blob = fetch(url, budget)
    with zipfile.ZipFile(io.BytesIO(blob)) as zf:
        names = zf.namelist()
        notices_name = f"{PYPI_NORM}-{version}.dist-info/licenses/{NOTICES}"
        notices = zf.read(notices_name) if notices_name in names else None
        prefix = f"{PYPI_NORM}-{version}.data/scripts/"
        for name in names:
            if name.startswith(prefix) and os.path.basename(name).startswith("gitlab-mcp-server"):
                return zf.read(name), notices
    raise LookupError(f"{url} ships no binary under {PYPI_NORM}-{version}.data/scripts/")


def check_notices(label, notices, where, digests, problems):
    """Hold one package's copy of THIRD_PARTY_NOTICES to the signed entry. A
    release whose checksums.txt names no notices predates them, and main says
    once that nothing is compared."""
    want = digests.get(NOTICES)
    if want is None:
        return
    if notices is None:
        problems.append(f"{label}: {where} carries no {NOTICES}, but the release signed one")
        return
    got = sha256(notices)
    if got != want:
        problems.append(
            f"{label}: {NOTICES} in {where} is sha256 {got}, but the signed checksums.txt says {want}"
        )
    else:
        print(f"  ok  {label:<17} {NOTICES} matches the signed one")


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


def nuget_package_file(pkg_id, version):
    """The file name scripts/build_nuget.py writes for a package, which is the
    name the nuget job records the package's attested digest under."""
    return f"{pkg_id}.{version}.nupkg"


def nuget_package(pkg_id, version, budget=None):
    """Download one published package from the flat container."""
    url = nuget_package_url(pkg_id, version)
    return fetch(url, budget), url


def nuget_binary(blob, rid, url):
    """Return the binary bytes a downloaded runtime package carries."""
    with zipfile.ZipFile(io.BytesIO(blob)) as zf:
        prefix = f"tools/any/{rid}/"
        for name in zf.namelist():
            if name.startswith(prefix) and os.path.basename(name) in ("gitlab-mcp-server", "gitlab-mcp-server.exe"):
                return zf.read(name)
    raise LookupError(f"{url} ships no gitlab-mcp-server binary under {prefix}")


def attested_nupkg_digests(path):
    """Parse the `<sha256>  <file>.nupkg` lines the nuget job recorded for the
    packages it attested. A line that is not one is ignored, as
    released_digests ignores the assets it does not want."""
    digests = {}
    with open(path, encoding="utf-8") as fh:
        for line in fh:
            parts = line.replace("\r", "").strip().split(None, 1)
            if len(parts) != 2:
                continue
            digest, name = parts[0].lower(), parts[1].lstrip("*")
            if re.fullmatch(r"[0-9a-f]{64}", digest) and name.endswith(".nupkg"):
                digests[name] = digest
    return digests


def nuget_unsigned(blob):
    """Return a served package as it was before nuget.org signed it.

    NuGet signs a package by writing a stored .signature.p7s local entry after
    the last local entry and its central directory record after the last
    record, and defines the unsigned package as what removing both gives back:
    every byte before the signature's local header, the central directory
    without its record, and the end record counted down by one with the
    directory's new offset and size. That is the package as it was attested.
    A package carrying no signature entry is returned unchanged. ValueError
    names an archive that cannot be read that way.
    """
    eocd = blob.rfind(ZIP_EOCD)
    if eocd < 0 or eocd + ZIP_EOCD_SIZE > len(blob):
        raise ValueError("no end-of-central-directory record")
    disk, cd_disk, n_disk, n_total, cd_size, cd_offset, comment_len = struct.unpack_from(
        "<HHHHIIH", blob, eocd + 4)
    if eocd + ZIP_EOCD_SIZE + comment_len != len(blob):
        raise ValueError("bytes follow the end-of-central-directory record")
    records = []
    offset = cd_offset
    for _ in range(n_total):
        if blob[offset:offset + 4] != ZIP_CENTRAL_HEADER:
            raise ValueError(f"no central directory record at offset {offset}")
        name_len, extra_len, record_comment_len = struct.unpack_from("<HHH", blob, offset + 28)
        local_offset = struct.unpack_from("<I", blob, offset + 42)[0]
        name = blob[offset + ZIP_CENTRAL_HEADER_SIZE:offset + ZIP_CENTRAL_HEADER_SIZE + name_len]
        size = ZIP_CENTRAL_HEADER_SIZE + name_len + extra_len + record_comment_len
        records.append((name.decode("utf-8", errors="replace"), offset, size, local_offset))
        offset += size
    if offset != cd_offset + cd_size:
        raise ValueError("the central directory is not the size its end record says")
    signatures = [record for record in records if record[0] == NUGET_SIGNATURE_ENTRY]
    if not signatures:
        return blob
    if len(signatures) > 1:
        raise ValueError(f"more than one {NUGET_SIGNATURE_ENTRY} entry")
    _, record_at, record_size, local_at = signatures[0]
    if any(record[3] > local_at for record in records):
        raise ValueError(f"{NUGET_SIGNATURE_ENTRY} is not the last local entry")
    directory = blob[cd_offset:record_at] + blob[record_at + record_size:cd_offset + cd_size]
    end = struct.pack("<4sHHHHIIH", ZIP_EOCD, disk, cd_disk, n_disk - 1, n_total - 1,
                      cd_size - record_size, local_at, comment_len)
    return blob[:local_at] + directory + end + blob[eocd + ZIP_EOCD_SIZE:]


def check_attested_nupkg(label, blob, url, filename, attested, problems):
    """Hold one served package, its repository signature removed, to the
    digest the nuget job attested for it."""
    want = attested.get(filename)
    if want is None:
        problems.append(f"nuget {label}: the nuget job recorded no attested digest for {filename}")
        return
    try:
        unsigned = nuget_unsigned(blob)
    except ValueError as exc:
        problems.append(f"nuget {label}: {url} cannot be unsigned the way NuGet defines it: {exc}")
        return
    got = sha256(unsigned)
    if got != want:
        problems.append(
            f"nuget {label}: {url} without its repository signature is sha256 {got}, "
            f"but the nuget job attested {filename} as {want}"
        )
    else:
        print(f"  ok  nuget {label:<32} unsigned, equals the attested {filename}")


def check_npm(version, digests, problems, budget=None):
    for suffix, asset in NPM_ASSETS.items():
        want = digests.get(asset)
        if want is None:
            problems.append(f"npm {suffix}: checksums.txt does not name the release asset {asset}")
            continue
        try:
            binary, notices, tarball = npm_package(suffix, version, budget)
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
        check_notices(f"npm {suffix}", notices, tarball, digests, problems)


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
            binary, notices = wheel_contents(url, version, budget)
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
        check_notices(f"pypi {filename}", notices, url, digests, problems)
    missing = sorted(set(WHEEL_ASSETS.values()) - seen)
    if missing:
        problems.append(f"pypi: no wheel published for {', '.join(missing)}")


def check_nuget_listing(version, problems, budget=None):
    """Check the pointer's flat-container index lists version, waiting from
    budget for an index that does not list it yet.

    While nuget.org validates a push, the index answers 200 with the versions
    it already serves, so an index without this one is the lag a 404 is and is
    waited for the same way; it becomes a finding only once budget is spent. An
    index that cannot be read at all is reported at once, beyond what fetch()
    already waited for: a body that is not JSON is an answer, not lag.
    """
    attempts = 0
    while True:
        attempts += 1
        try:
            versions = nuget_versions(NUGET_ID, budget)
        except (urllib.error.URLError, ValueError) as exc:
            problems.append(f"nuget {NUGET_ID}: could not list the published versions: {exc}")
            return
        if version.lower() in versions:
            print(f"  ok  nuget {NUGET_ID:<32} lists {version}")
            return
        if budget is None or not budget.wait():
            problems.append(f"nuget {NUGET_ID}: version {version} is not listed on nuget.org "
                            f"(still unlisted after {attempts} attempt(s))")
            return
        print(f"  .. nuget {NUGET_ID}: version {version} is not listed yet; retrying (attempt {attempts + 1})",
              flush=True)


def check_nuget(version, digests, problems, budget=None, attested=None):
    """Check the pointer is listed and each runtime package carries the signed
    binary; with attested digests, hold all seven packages to them as well.
    budget is the NuGet deadline in a run of main(), which every download here
    and the listing draw on."""
    # The pointer carries no binary, so its check is that the version is
    # listed at all: a runtime package nobody points at is not installable,
    # and a pointer published without its runtime packages installs nothing.
    check_nuget_listing(version, problems, budget)
    if attested is not None and not attested:
        problems.append("nuget: the attested digests name no package, so nothing the nuget job attested "
                        "can be compared; its nupkg_sha256 output did not reach this run")
        attested = None
    if attested is not None:
        published = {nuget_package_file(NUGET_ID, version)}
        published.update(nuget_package_file(f"{NUGET_ID}.{rid}", version) for rid in NUGET_ASSETS)
        for name in sorted(set(attested) - published):
            problems.append(f"nuget: the nuget job attested {name}, which is not one of the packages of {version}")
        # The pointer is what dnx and dotnet tool install read first, and its
        # DotnetToolSettings.xml decides which package runs, so it is held to
        # its attestation like the packages that carry the binary.
        try:
            blob, url = nuget_package(NUGET_ID, version, budget)
        except urllib.error.URLError as exc:
            problems.append(f"nuget {NUGET_ID}: could not read the published package: {exc}")
        else:
            check_attested_nupkg(NUGET_ID, blob, url, nuget_package_file(NUGET_ID, version), attested, problems)
    for rid, asset in NUGET_ASSETS.items():
        want = digests.get(asset)
        if want is None:
            problems.append(f"nuget {rid}: checksums.txt does not name the release asset {asset}")
            continue
        try:
            blob, url = nuget_package(f"{NUGET_ID}.{rid}", version, budget)
            binary = nuget_binary(blob, rid, url)
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
        if attested is not None:
            check_attested_nupkg(rid, blob, url, nuget_package_file(f"{NUGET_ID}.{rid}", version), attested,
                                 problems)


def main(argv=None, clock=time.monotonic, sleep=time.sleep):
    """Run the comparison over argv (the process arguments when None). clock
    and sleep are injected so a test can drive both allowances without
    spending the time they describe."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("version", help="release version without the leading v, e.g. 2.7.6")
    parser.add_argument("checksums", help="path to the release's checksums.txt (or a directory holding it)")
    parser.add_argument("--skip-npm", action="store_true", help="do not check npm")
    parser.add_argument("--skip-pypi", action="store_true", help="do not check PyPI")
    parser.add_argument("--skip-nuget", action="store_true", help="do not check NuGet")
    parser.add_argument(
        "--nuget-digests",
        metavar="FILE",
        help="sha256sum lines of the .nupkg files the release attested; each served package, its repository "
        "signature removed, must match its line (without it the NuGet packages are checked by their binaries only)",
    )
    parser.add_argument(
        "--retry-budget",
        type=float,
        default=RETRY_BUDGET,
        metavar="SECONDS",
        help="total seconds the npm and PyPI checks may spend waiting for a registry to catch up "
        "(0 disables every wait, NuGet's included)",
    )
    parser.add_argument(
        "--retry-delay",
        type=float,
        default=RETRY_DELAY,
        metavar="SECONDS",
        help="pause between download attempts, for npm and PyPI and for NuGet",
    )
    parser.add_argument(
        "--nuget-deadline",
        type=float,
        default=NUGET_DEADLINE,
        metavar="SECONDS",
        help="seconds from the start of the run until which the NuGet checks may keep waiting for nuget.org "
        "to serve the version (ignored when --retry-budget is 0)",
    )
    args = parser.parse_args(argv)

    # Made before anything is read, because the deadline is counted from the
    # start: nuget.org validates while npm and PyPI are being waited for.
    # --retry-budget 0 is the out-of-band run, and it waits for nothing.
    nuget_seconds = args.nuget_deadline if args.retry_budget > 0 else 0
    deadline = RetryDeadline(nuget_seconds, args.retry_delay, clock=clock, sleep=sleep)
    budget = RetryBudget(args.retry_budget, args.retry_delay, sleep=sleep)

    checksums = args.checksums
    if os.path.isdir(checksums):
        checksums = os.path.join(checksums, "checksums.txt")
    digests = released_digests(checksums)
    if not set(digests) - {NOTICES}:
        sys.exit(f"verify_published_packages: {checksums} names none of the release binaries")

    print(f"Comparing published packages for v{args.version} against {len(digests)} entries in {checksums}")
    if NOTICES not in digests:
        print(f"  ..  {checksums} names no {NOTICES}, a release from before they were generated, "
              "so the packages' notices are not compared")
    problems = []
    if not args.skip_npm:
        check_npm(args.version, digests, problems, budget)
    if not args.skip_pypi:
        check_pypi(args.version, digests, problems, budget)
    attested = None
    if not args.skip_nuget:
        if args.nuget_digests:
            attested = attested_nupkg_digests(args.nuget_digests)
        else:
            print("  ..  nuget: no --nuget-digests given, so the packages are checked by the binaries they carry only")
        check_nuget(args.version, digests, problems, deadline, attested)

    if problems:
        print(f"\nFAILED ({len(problems)}):")
        for problem in problems:
            print(f"  x {problem}")
        if budget.waits or deadline.waits:
            print()
        if budget.waits:
            print(f"npm and PyPI retried {budget.waits} time(s); {int(budget.remaining)}s of their "
                  f"{int(args.retry_budget)}s retry budget was left unspent.")
        if deadline.waits:
            print(f"NuGet retried {deadline.waits} time(s); {int(deadline.remaining)}s of its "
                  f"{int(deadline.seconds)}s deadline, counted from the start of this run, was left.")
        if budget.waits or deadline.waits:
            print("An allowance spent to zero means a registry never served the version "
                  "(a download never came back, or the pointer's index never listed it), "
                  "which reads differently from a digest that did not match.")
        sys.exit(1)
    print("\nEvery published package carries the binary the release signed.")
    if attested:
        print("Every NuGet package nuget.org serves is, without its repository signature, the one the release attested.")


if __name__ == "__main__":
    main()
