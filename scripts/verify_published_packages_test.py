#!/usr/bin/env python3
"""Tests the retry behaviour of scripts/verify_published_packages.py.

That script became a hard gate: mcp-registry and commit-manifests both wait on
it, so a single unlucky HTTP response now fails a release that cannot be
re-run. It also runs minutes after the uploads it reads back, and a registry
that has not propagated a new version yet answers 404 — the one condition the
rest of the workflow already expects, since the mcp-registry publish step
retries eight times at 60s intervals for exactly it.

So the two properties worth pinning are opposites of each other, and both are
asserted here:

  * a download that fails in transit is retried, and a version that shows up
    during the wait is accepted;
  * a digest that does not match the signed checksums.txt is reported the
    first time it is seen, with no second attempt to launder it.

The waiting comes from two allowances, and what tells them apart is pinned
too. npm and PyPI share a budget of seconds slept. NuGet has a deadline of its
own, counted in wall-clock time from the start of the run, because nuget.org
validates a push for many minutes while the other two are being waited for:
3.1.0 failed here because the one shared budget was spent, 13 of its 16 waits
by npm's lag and the other 3 by NuGet's first runtime package, and the
pointer's index, which lists the older versions until the new one is served,
was never asked twice. The tests drive a clock of their own, so a deadline of
forty minutes costs no time at all.

The NuGet packages are also held whole to the digests the nuget job attested,
once nuget.org's repository signature is removed, so the unsigning is pinned
here against packages signed the way nuget.org signs them, against every
archive it must refuse to guess about, and through the command line that hands
it the digests.

Run with:

    python3 -m unittest discover -s scripts -p 'verify_published_packages_test.py'
"""

import contextlib
import hashlib
import importlib.util
import io
import os
import struct
import sys
import tarfile
import tempfile
import unittest
import urllib.error
import warnings
import zipfile

ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
MODULE_PATH = os.path.join(ROOT, "scripts", "verify_published_packages.py")


def load_module():
    """Import the script by path; scripts/ is not a package."""
    spec = importlib.util.spec_from_file_location("verify_published_packages", MODULE_PATH)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


vpp = load_module()


def http_error(url, code=404):
    return urllib.error.HTTPError(url, code, "Not Found", {}, None)


class Response(io.BytesIO):
    """The context-manager shape urlopen returns, over fixed bytes."""

    def __enter__(self):
        return self

    def __exit__(self, *exc):
        return False


class FakeOpener:
    """Stands in for urllib.request.urlopen with a scripted answer per URL.

    Each URL maps to a list of outcomes consumed in order: an exception is
    raised, bytes are returned. The final outcome repeats, so "always 404" is
    a one-element list.
    """

    def __init__(self, script):
        self.script = {url: list(outcomes) for url, outcomes in script.items()}
        self.calls = []

    def __call__(self, request, timeout=None):
        url = request.full_url if hasattr(request, "full_url") else request
        self.calls.append(url)
        outcomes = self.script.get(url)
        if not outcomes:
            raise http_error(url)
        outcome = outcomes.pop(0) if len(outcomes) > 1 else outcomes[0]
        if isinstance(outcome, Exception):
            raise outcome
        return Response(outcome)

    def count(self, url):
        return self.calls.count(url)


class FakeClock:
    """A monotonic clock that moves only when something sleeps on it, or when
    a test moves it to stand for time spent elsewhere."""

    def __init__(self):
        self.now = 0.0
        self.slept = []

    def __call__(self):
        return self.now

    def sleep(self, seconds):
        self.slept.append(seconds)
        self.now += seconds


class ClockedOpener:
    """Stands in for urlopen with answers that change with the time.

    Each URL maps to a list of (from, outcome) pairs in ascending order: the
    answer at a moment is the outcome of the last pair whose from has passed,
    and a URL with no pair that has passed yet answers 404, which is how a
    registry answers a version it does not serve yet.
    """

    def __init__(self, clock, script):
        self.clock = clock
        self.script = script
        self.calls = []

    def __call__(self, request, timeout=None):
        url = request.full_url if hasattr(request, "full_url") else request
        self.calls.append(url)
        outcome = None
        for since, answer in self.script.get(url, []):
            if since <= self.clock():
                outcome = answer
        if outcome is None:
            raise http_error(url)
        if isinstance(outcome, Exception):
            raise outcome
        return Response(outcome)

    def count(self, url):
        return self.calls.count(url)


class RetryBudgetTest(unittest.TestCase):
    """The budget is an allowance of sleep, shared by every download."""

    def test_it_stops_when_the_allowance_runs_out(self):
        slept = []
        budget = vpp.RetryBudget(seconds=10, delay=4, sleep=slept.append)
        self.assertTrue(budget.wait())
        self.assertTrue(budget.wait())
        self.assertFalse(budget.wait(), "8s of a 10s budget spent leaves no room for a third 4s wait")
        self.assertEqual(slept, [4, 4])
        self.assertEqual(budget.waits, 2)

    def test_a_zero_budget_never_waits(self):
        """--retry-budget 0 is the out-of-band run, where lag is not a story."""
        budget = vpp.RetryBudget(seconds=0, delay=20, sleep=lambda _: self.fail("slept"))
        self.assertFalse(budget.wait())

    def test_a_zero_delay_never_waits(self):
        budget = vpp.RetryBudget(seconds=600, delay=0, sleep=lambda _: self.fail("slept"))
        self.assertFalse(budget.wait())


class RetryDeadlineTest(unittest.TestCase):
    """NuGet's allowance is an instant on the clock, not an amount of sleep:
    whatever time passes before its check begins is time nuget.org spent
    validating, and it counts."""

    def test_it_stops_when_a_wait_would_end_past_the_deadline(self):
        clock = FakeClock()
        deadline = vpp.RetryDeadline(seconds=10, delay=4, clock=clock, sleep=clock.sleep)
        self.assertTrue(deadline.wait())
        self.assertTrue(deadline.wait())
        self.assertFalse(deadline.wait(), "a third 4s wait would end at 12s, past the 10s deadline")
        self.assertEqual(clock.slept, [4, 4])
        self.assertEqual(deadline.waits, 2)
        self.assertEqual(deadline.remaining, 2)

    def test_time_spent_before_the_first_wait_is_counted(self):
        """Counted from the run's start, not from the first wait: eighty
        seconds spent on npm leave twenty of a hundred."""
        clock = FakeClock()
        deadline = vpp.RetryDeadline(seconds=100, delay=30, clock=clock, sleep=clock.sleep)
        clock.now = 80
        self.assertEqual(deadline.remaining, 20)
        self.assertFalse(deadline.wait())
        self.assertEqual(clock.slept, [])

    def test_a_deadline_long_past_has_nothing_left(self):
        clock = FakeClock()
        deadline = vpp.RetryDeadline(seconds=10, delay=1, clock=clock, sleep=clock.sleep)
        clock.now = 50
        self.assertEqual(deadline.remaining, 0)
        self.assertFalse(deadline.wait())

    def test_a_zero_deadline_or_delay_never_waits(self):
        cases = [("no deadline", 0, 20), ("no delay", 600, 0)]
        for name, seconds, delay in cases:
            with self.subTest(name):
                clock = FakeClock()
                deadline = vpp.RetryDeadline(seconds=seconds, delay=delay, clock=clock,
                                             sleep=lambda _: self.fail("slept"))
                self.assertFalse(deadline.wait())
                self.assertEqual(deadline.waits, 0)

    def test_fetch_spends_it_like_a_budget(self):
        """fetch() takes either allowance: a 404 that clears inside the
        deadline is waited out, one that does not is a FetchError."""
        url = vpp.nuget_package_url(f"{vpp.NUGET_ID}.linux-x64", "1.0.0")
        real_urlopen = vpp.urllib.request.urlopen
        self.addCleanup(setattr, vpp.urllib.request, "urlopen", real_urlopen)
        cases = [("served at 90s", 90, b"package"), ("served at 900s", 900, None)]
        for name, served_at, want in cases:
            with self.subTest(name):
                clock = FakeClock()
                vpp.urllib.request.urlopen = ClockedOpener(clock, {url: [(served_at, b"package")]})
                deadline = vpp.RetryDeadline(seconds=600, delay=30, clock=clock, sleep=clock.sleep)
                with contextlib.redirect_stdout(io.StringIO()):
                    if want is None:
                        with self.assertRaises(vpp.FetchError) as caught:
                            vpp.fetch(url, deadline)
                        self.assertIn("after 21 attempt(s)", str(caught.exception))
                        self.assertEqual(clock.now, 600)
                    else:
                        self.assertEqual(vpp.fetch(url, deadline), want)
                        self.assertEqual(clock.now, 90)


class FetchRetryTest(unittest.TestCase):
    """fetch() is the only place a retry happens, and it retries a 404."""

    def setUp(self):
        self.real_urlopen = vpp.urllib.request.urlopen
        self.addCleanup(setattr, vpp.urllib.request, "urlopen", self.real_urlopen)

    def budget(self, seconds=100, delay=1):
        return vpp.RetryBudget(seconds=seconds, delay=delay, sleep=lambda _: None)

    def test_a_404_that_clears_is_waited_out(self):
        """Propagation lag: the version appears while the budget is spent.

        Without a retry budget this raised on the first response, which is the
        regression — the strictest gate in the pipeline failing on the one
        condition the mcp-registry job already waits out.
        """
        url = "https://registry.npmjs.org/pkg/1.0.0"
        opener = FakeOpener({url: [http_error(url), http_error(url), b"payload"]})
        vpp.urllib.request.urlopen = opener

        budget = self.budget()
        self.assertEqual(vpp.fetch(url, budget), b"payload")
        self.assertEqual(opener.count(url), 3)
        self.assertEqual(budget.waits, 2)

    def test_a_transport_failure_is_waited_out_too(self):
        """A reset connection is not an answer about the package either."""
        url = "https://pypi.org/pypi/dist/1.0.0/json"
        opener = FakeOpener({url: [urllib.error.URLError("connection reset"), b"{}"]})
        vpp.urllib.request.urlopen = opener

        self.assertEqual(vpp.fetch(url, self.budget()), b"{}")
        self.assertEqual(opener.count(url), 2)

    def test_a_persistent_404_still_fails(self):
        """Waiting is not forgiving: the budget runs out and the job fails."""
        url = "https://registry.npmjs.org/pkg/1.0.0"
        opener = FakeOpener({url: [http_error(url)]})
        vpp.urllib.request.urlopen = opener

        budget = self.budget(seconds=3, delay=1)
        with self.assertRaises(vpp.FetchError) as caught:
            vpp.fetch(url, budget)
        self.assertEqual(opener.count(url), 4, "three waits, four attempts")
        self.assertIn("HTTP 404", str(caught.exception))
        self.assertIn("after 4 attempt(s)", str(caught.exception))
        self.assertIsInstance(caught.exception, urllib.error.URLError, "callers catch URLError")

    def test_no_budget_means_one_attempt(self):
        url = "https://registry.npmjs.org/pkg/1.0.0"
        opener = FakeOpener({url: [http_error(url)]})
        vpp.urllib.request.urlopen = opener

        with self.assertRaises(vpp.FetchError):
            vpp.fetch(url)
        self.assertEqual(opener.count(url), 1)


def npm_tarball(payload, notices=None):
    """A .tgz shaped like a published npm platform package: a directory
    entry, its package.json and LICENSE, the binary when payload is given,
    and package/THIRD_PARTY_NOTICES when notices is given."""
    entries = [("package/package.json", b"{}"), ("package/LICENSE", b"MIT License\n")]
    if payload is not None:
        entries.append(("package/bin/gitlab-mcp-server", payload))
    if notices is not None:
        entries.append(("package/" + vpp.NOTICES, notices))
    blob = io.BytesIO()
    with tarfile.open(fileobj=blob, mode="w:gz") as tar:
        directory = tarfile.TarInfo("package/bin")
        directory.type = tarfile.DIRTYPE
        tar.addfile(directory)
        for name, data in entries:
            info = tarfile.TarInfo(name)
            info.size = len(data)
            tar.addfile(info, io.BytesIO(data))
    return blob.getvalue()


def wheel(version, payload, notices=None):
    """A .whl shaped like a published platform wheel: the notices under
    .dist-info/licenses/ when given, written first the way a wheel lists its
    metadata, then the binary when payload is given."""
    blob = io.BytesIO()
    with zipfile.ZipFile(blob, "w") as zf:
        if notices is not None:
            zf.writestr(f"{vpp.PYPI_NORM}-{version}.dist-info/licenses/{vpp.NOTICES}", notices)
        if payload is not None:
            zf.writestr(f"{vpp.PYPI_NORM}-{version}.data/scripts/gitlab-mcp-server", payload)
    return blob.getvalue()


def nupkg(rid, payload):
    """A .nupkg shaped like a published runtime-identifier package."""
    blob = io.BytesIO()
    with zipfile.ZipFile(blob, "w") as zf:
        zf.writestr(f"tools/any/{rid}/DotnetToolSettings.xml", "<DotNetCliTool Version=\"2\" />")
        zf.writestr(f"tools/any/{rid}/gitlab-mcp-server", payload)
    return blob.getvalue()


def pointer_nupkg(readme=b"mcp-name: io.github.jmrplens/gitlab-mcp-server"):
    """A .nupkg shaped like the published pointer package, with no binary."""
    blob = io.BytesIO()
    with zipfile.ZipFile(blob, "w") as zf:
        zf.writestr("tools/net10.0/any/DotnetToolSettings.xml", "<DotNetCliTool Version=\"2\" />")
        zf.writestr("README.md", readme)
    return blob.getvalue()


def sign(unsigned, name=".signature.p7s"):
    """Sign a package the way nuget.org does: a stored signature entry after
    the last local entry and its record after the last central record, every
    byte before them left as it was. zipfile's append mode does exactly that."""
    blob = io.BytesIO(unsigned)
    with zipfile.ZipFile(blob, "a") as zf:
        info = zipfile.ZipInfo(name, date_time=(2026, 1, 1, 0, 0, 0))
        info.compress_type = zipfile.ZIP_STORED
        zf.writestr(info, b"0\x82 nuget.org repository signature")
    return blob.getvalue()


def end_record(blob):
    """The end record's offset and its (count, size, offset) of the directory."""
    eocd = blob.rfind(vpp.ZIP_EOCD)
    count, size, offset = struct.unpack_from("<HII", blob, eocd + 10)
    return eocd, count, size, offset


class MismatchIsNotRetriedTest(unittest.TestCase):
    """The finding this job exists to make is never waited out.

    A retry can turn "not published yet" into a pass, which is the point. It
    must not be able to turn "these are the wrong bytes" into one, so the
    comparison happens after the download returns and is reported once.
    """

    VERSION = "1.0.0"

    def setUp(self):
        self.real_urlopen = vpp.urllib.request.urlopen
        self.addCleanup(setattr, vpp.urllib.request, "urlopen", self.real_urlopen)
        self.signed = b"\x7fELFthe bytes the release signed"
        self.served = b"\x7fELFsomething else entirely"
        self.digests = {asset: hashlib.sha256(self.signed).hexdigest() for asset in vpp.NPM_ASSETS.values()}

    def test_a_wrong_npm_binary_is_reported_on_the_first_look(self):
        """Every platform package serves a binary the release never signed."""
        script = {}
        tarballs = {}
        for suffix in vpp.NPM_ASSETS:
            name = f"{vpp.NPM_SCOPE}/gitlab-mcp-server-{suffix}"
            quoted = vpp.urllib.parse.quote(name, safe="")
            tarball_url = f"https://registry.npmjs.org/{quoted}/-/{suffix}-{self.VERSION}.tgz"
            tarballs[suffix] = tarball_url
            script[f"https://registry.npmjs.org/{quoted}/{self.VERSION}"] = [
                b'{"dist": {"tarball": "%s"}}' % tarball_url.encode()
            ]
            script[tarball_url] = [npm_tarball(self.served)]
        opener = FakeOpener(script)
        vpp.urllib.request.urlopen = opener

        budget = vpp.RetryBudget(seconds=600, delay=20, sleep=lambda _: self.fail("a mismatch was retried"))
        problems = []
        vpp.check_npm(self.VERSION, self.digests, problems, budget)

        mismatches = [p for p in problems if "carries sha256" in p]
        self.assertEqual(len(mismatches), len(vpp.NPM_ASSETS), problems)
        for suffix, url in tarballs.items():
            self.assertEqual(opener.count(url), 1, f"{suffix} was downloaded more than once")
        self.assertEqual(budget.waits, 0)
        self.assertEqual(budget.remaining, 600, "a mismatch spends none of the budget")

    def test_a_wrong_wheel_is_reported_on_the_first_look(self):
        index = f"https://pypi.org/pypi/{vpp.PYPI_DIST}/{self.VERSION}/json"
        wheel_url = "https://files.pythonhosted.org/x/pkg-1.0.0-py3-none-manylinux_2_17_x86_64.whl"
        filename = "pkg-1.0.0-py3-none-manylinux_2_17_x86_64.whl"
        meta = b'{"urls": [{"packagetype": "bdist_wheel", "filename": "%s", "url": "%s"}]}' % (
            filename.encode(),
            wheel_url.encode(),
        )
        opener = FakeOpener({index: [meta], wheel_url: [wheel(self.VERSION, self.served)]})
        vpp.urllib.request.urlopen = opener

        budget = vpp.RetryBudget(seconds=600, delay=20, sleep=lambda _: self.fail("a mismatch was retried"))
        problems = []
        vpp.check_pypi(self.VERSION, self.digests, problems, budget)

        self.assertTrue([p for p in problems if "carries sha256" in p], problems)
        self.assertEqual(opener.count(wheel_url), 1)
        self.assertEqual(budget.waits, 0)

    def test_a_wrong_nuget_binary_is_reported_on_the_first_look(self):
        """Every runtime package serves a binary the release never signed,
        while the pointer lists the version, so the one finding is the digest."""
        index = f"{vpp.NUGET_FLAT}/{vpp.NUGET_ID}/index.json"
        script = {index: [b'{"versions": ["0.9.0", "%s"]}' % self.VERSION.encode()]}
        urls = {}
        for rid in vpp.NUGET_ASSETS:
            url = vpp.nuget_package_url(f"{vpp.NUGET_ID}.{rid}", self.VERSION)
            urls[rid] = url
            script[url] = [nupkg(rid, self.served)]
        opener = FakeOpener(script)
        vpp.urllib.request.urlopen = opener

        budget = vpp.RetryBudget(seconds=600, delay=20, sleep=lambda _: self.fail("a mismatch was retried"))
        problems = []
        vpp.check_nuget(self.VERSION, self.digests, problems, budget)

        mismatches = [p for p in problems if "carries sha256" in p]
        self.assertEqual(len(mismatches), len(vpp.NUGET_ASSETS), problems)
        self.assertEqual(len(problems), len(mismatches), "the listed pointer version is not a finding")
        for rid, url in urls.items():
            self.assertEqual(opener.count(url), 1, f"{rid} was downloaded more than once")
        self.assertEqual(budget.waits, 0)

    def test_an_unlisted_nuget_pointer_is_a_finding(self):
        """The runtime packages match, but nothing points at them."""
        index = f"{vpp.NUGET_FLAT}/{vpp.NUGET_ID}/index.json"
        script = {index: [b'{"versions": ["0.9.0"]}']}
        for rid in vpp.NUGET_ASSETS:
            script[vpp.nuget_package_url(f"{vpp.NUGET_ID}.{rid}", self.VERSION)] = [nupkg(rid, self.signed)]
        vpp.urllib.request.urlopen = FakeOpener(script)

        problems = []
        vpp.check_nuget(self.VERSION, self.digests, problems, vpp.RetryBudget(seconds=0))
        self.assertEqual(len(problems), 1, problems)
        self.assertIn("is not listed on nuget.org", problems[0])


class NugetListingLagTest(unittest.TestCase):
    """While nuget.org validates a push, the pointer's flat-container index
    answers 200 with the versions it already serves. That is the same lag a
    404 is, so it is waited for from NuGet's deadline, and a finding only once
    the deadline is spent."""

    VERSION = "1.0.0"
    INDEX = f"{vpp.NUGET_FLAT}/{vpp.NUGET_ID}/index.json"

    def setUp(self):
        self.real_urlopen = vpp.urllib.request.urlopen
        self.addCleanup(setattr, vpp.urllib.request, "urlopen", self.real_urlopen)
        self.binary = b"\x7fELFthe bytes the release signed"
        self.digests = {asset: hashlib.sha256(self.binary).hexdigest() for asset in vpp.NUGET_ASSETS.values()}
        self.clock = FakeClock()

    def serve(self, listed_at):
        """Serve the runtime packages from the start, and list the version in
        the pointer's index from listed_at on (never, when it is None)."""
        index = [(0, b'{"versions": ["0.9.0"]}')]
        if listed_at is not None:
            index.append((listed_at, b'{"versions": ["0.9.0", "%s"]}' % self.VERSION.encode()))
        script = {self.INDEX: index}
        for rid in vpp.NUGET_ASSETS:
            script[vpp.nuget_package_url(f"{vpp.NUGET_ID}.{rid}", self.VERSION)] = [(0, nupkg(rid, self.binary))]
        opener = ClockedOpener(self.clock, script)
        vpp.urllib.request.urlopen = opener
        return opener

    def check(self, seconds):
        deadline = vpp.RetryDeadline(seconds=seconds, delay=30, clock=self.clock, sleep=self.clock.sleep)
        problems = []
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            vpp.check_nuget(self.VERSION, self.digests, problems, deadline)
        return problems, deadline, out.getvalue()

    def test_a_pointer_listing_that_appears_after_a_few_attempts_passes(self):
        opener = self.serve(listed_at=90)
        problems, deadline, out = self.check(seconds=2400)
        self.assertEqual(problems, [])
        self.assertEqual(opener.count(self.INDEX), 4, "listed on the fourth reading, after three waits")
        self.assertEqual(deadline.waits, 3)
        self.assertIn(f"version {self.VERSION} is not listed yet; retrying (attempt 2)", out)
        self.assertIn(f"lists {self.VERSION}", out)

    def test_a_pointer_listing_that_never_appears_is_a_finding_once_the_deadline_is_spent(self):
        opener = self.serve(listed_at=None)
        problems, deadline, _ = self.check(seconds=100)
        self.assertEqual(len(problems), 1, problems)
        self.assertIn("is not listed on nuget.org", problems[0])
        self.assertIn("after 4 attempt(s)", problems[0])
        self.assertEqual(opener.count(self.INDEX), 4)
        self.assertEqual(deadline.waits, 3)
        self.assertEqual(self.clock.now, 90, "no wait that would end past the deadline")

    def test_an_index_that_cannot_be_read_is_a_finding_and_is_not_read_twice(self):
        """A body that is not JSON is an answer, not lag: nothing about waiting
        changes what nuget.org said."""
        opener = self.serve(listed_at=None)
        opener.script[self.INDEX] = [(0, b"<html>not an index</html>")]
        problems, deadline, _ = self.check(seconds=2400)
        self.assertEqual(len(problems), 1, problems)
        self.assertIn(f"nuget {vpp.NUGET_ID}: could not list the published versions", problems[0])
        self.assertEqual(opener.count(self.INDEX), 1)
        self.assertEqual(deadline.waits, 0)


class NoticesTest(unittest.TestCase):
    """Every npm platform package and every wheel carries the
    THIRD_PARTY_NOTICES the release signed, when the release signed one."""

    VERSION = "1.0.0"
    WHEEL = "pkg-1.0.0-py3-none-manylinux_2_17_x86_64.whl"
    WHEEL_URL = "https://files.pythonhosted.org/x/" + WHEEL

    def setUp(self):
        self.real_urlopen = vpp.urllib.request.urlopen
        self.addCleanup(setattr, vpp.urllib.request, "urlopen", self.real_urlopen)
        self.binary = b"\x7fELFthe bytes the release signed"
        self.notices = b"Third-party notices for gitlab-mcp-server\n\nthe texts the release signed\n"
        self.digests = {asset: hashlib.sha256(self.binary).hexdigest() for asset in vpp.NPM_ASSETS.values()}
        self.digests[vpp.NOTICES] = hashlib.sha256(self.notices).hexdigest()

    def serve_npm(self, notices_for):
        """Serve every npm platform package, each carrying the notices
        notices_for names for its suffix (self.notices unless named)."""
        script = {}
        for suffix in vpp.NPM_ASSETS:
            quoted = vpp.urllib.parse.quote(f"{vpp.NPM_SCOPE}/gitlab-mcp-server-{suffix}", safe="")
            tarball_url = f"https://registry.npmjs.org/{quoted}/-/{suffix}-{self.VERSION}.tgz"
            script[f"https://registry.npmjs.org/{quoted}/{self.VERSION}"] = [
                b'{"dist": {"tarball": "%s"}}' % tarball_url.encode()
            ]
            script[tarball_url] = [npm_tarball(self.binary, notices_for.get(suffix, self.notices))]
        vpp.urllib.request.urlopen = FakeOpener(script)

    def serve_wheel(self, notices):
        index = f"https://pypi.org/pypi/{vpp.PYPI_DIST}/{self.VERSION}/json"
        meta = b'{"urls": [{"packagetype": "bdist_wheel", "filename": "%s", "url": "%s"}]}' % (
            self.WHEEL.encode(), self.WHEEL_URL.encode())
        vpp.urllib.request.urlopen = FakeOpener({index: [meta], self.WHEEL_URL: [wheel(self.VERSION, self.binary,
                                                                                         notices)]})

    def test_npm_packages_carrying_the_signed_notices_pass(self):
        self.serve_npm({})
        out = io.StringIO()
        problems = []
        with contextlib.redirect_stdout(out):
            vpp.check_npm(self.VERSION, self.digests, problems, vpp.RetryBudget(seconds=0))
        self.assertEqual(problems, [])
        self.assertEqual(out.getvalue().count(f"{vpp.NOTICES} matches the signed one"), len(vpp.NPM_ASSETS))

    def test_npm_packages_with_other_notices_or_none_are_named(self):
        self.serve_npm({"linux-x64": b"Third-party notices for gitlab-mcp-server\n\nanother build\n",
                        "win32-arm64": None})
        problems = []
        with contextlib.redirect_stdout(io.StringIO()):
            vpp.check_npm(self.VERSION, self.digests, problems, vpp.RetryBudget(seconds=0))
        self.assertEqual(len(problems), 2, problems)
        self.assertIn(f"npm linux-x64: {vpp.NOTICES} in https://registry.npmjs.org/", problems[0])
        self.assertIn("but the signed checksums.txt says " + self.digests[vpp.NOTICES], problems[0])
        self.assertIn("npm win32-arm64: https://registry.npmjs.org/", problems[1])
        self.assertIn(f"carries no {vpp.NOTICES}, but the release signed one", problems[1])

    def test_each_wheel_is_held_to_the_signed_notices(self):
        cases = [
            ("the signed notices", self.notices, None),
            ("other notices", b"other notices\n", f"{vpp.NOTICES} in {self.WHEEL_URL} is sha256"),
            ("no notices", None, f"{self.WHEEL_URL} carries no {vpp.NOTICES}, but the release signed one"),
        ]
        for name, notices, want in cases:
            with self.subTest(name):
                self.serve_wheel(notices)
                problems = []
                with contextlib.redirect_stdout(io.StringIO()):
                    vpp.check_pypi(self.VERSION, self.digests, problems, vpp.RetryBudget(seconds=0))
                notice_problems = [p for p in problems if vpp.NOTICES in p]
                if want is None:
                    self.assertEqual(notice_problems, [])
                else:
                    self.assertEqual(len(notice_problems), 1, problems)
                    self.assertIn(want, notice_problems[0])

    def test_a_package_without_its_binary_is_named_and_its_notices_are_not_judged(self):
        """A package that cannot be read is one finding, not two."""
        self.serve_npm({})
        tarball = next(url for url in vpp.urllib.request.urlopen.script if url.endswith("linux-x64-1.0.0.tgz"))
        vpp.urllib.request.urlopen.script[tarball] = [npm_tarball(None, self.notices)]
        problems = []
        with contextlib.redirect_stdout(io.StringIO()):
            vpp.check_npm(self.VERSION, self.digests, problems, vpp.RetryBudget(seconds=0))
        self.assertEqual(len(problems), 1, problems)
        self.assertIn("npm linux-x64: could not read the published package:", problems[0])
        self.assertIn("ships no gitlab-mcp-server binary", problems[0])

        self.serve_wheel(self.notices)
        index = f"https://pypi.org/pypi/{vpp.PYPI_DIST}/{self.VERSION}/json"
        vpp.urllib.request.urlopen.script[self.WHEEL_URL] = [wheel(self.VERSION, None, self.notices)]
        self.assertIn(index, vpp.urllib.request.urlopen.script)
        problems = []
        with contextlib.redirect_stdout(io.StringIO()):
            vpp.check_pypi(self.VERSION, self.digests, problems, vpp.RetryBudget(seconds=0))
        read_failures = [p for p in problems if "could not read the wheel" in p]
        self.assertEqual(len(read_failures), 1, problems)
        self.assertEqual([p for p in problems if vpp.NOTICES in p], [])

    def test_a_release_from_before_the_notices_compares_none(self):
        del self.digests[vpp.NOTICES]
        self.serve_npm({suffix: None for suffix in vpp.NPM_ASSETS})
        problems = []
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            vpp.check_npm(self.VERSION, self.digests, problems, vpp.RetryBudget(seconds=0))
        self.assertEqual(problems, [])
        self.assertNotIn(vpp.NOTICES, out.getvalue())

    def test_the_signed_notices_are_read_from_checksums_txt(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "checksums.txt")
            with open(path, "w", encoding="utf-8") as fh:
                fh.write("a" * 64 + "  gitlab-mcp-server-linux-amd64\n")
                fh.write("b" * 64 + "  " + vpp.NOTICES + "\n")
                fh.write("c" * 64 + "  gitlab-mcp-server-linux-amd64.sbom.json\n")
            self.assertEqual(vpp.released_digests(path), {
                "gitlab-mcp-server-linux-amd64": "a" * 64, vpp.NOTICES: "b" * 64})

    def test_a_checksums_txt_naming_only_the_notices_names_no_binary(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "checksums.txt")
            with open(path, "w", encoding="utf-8") as fh:
                fh.write("b" * 64 + "  " + vpp.NOTICES + "\n")
            real_argv = sys.argv
            sys.argv = ["verify_published_packages.py", self.VERSION, path]
            try:
                with self.assertRaises(SystemExit) as raised:
                    vpp.main()
            finally:
                sys.argv = real_argv
        self.assertIn("names none of the release binaries", str(raised.exception.code))


class NugetUnsignedTest(unittest.TestCase):
    """nuget_unsigned gives back the package as it was before nuget.org
    signed it, which is the package the release attested."""

    def test_removing_the_signature_gives_back_the_package_that_was_signed(self):
        cases = [
            ("a runtime package", nupkg("linux-x64", b"\x7fELFbinary")),
            ("the pointer", pointer_nupkg()),
        ]
        for name, unsigned in cases:
            with self.subTest(name):
                signed = sign(unsigned)
                self.assertNotEqual(signed, unsigned)
                self.assertEqual(vpp.nuget_unsigned(signed), unsigned)

    def test_an_archive_comment_is_kept_where_it_was(self):
        blob = io.BytesIO()
        with zipfile.ZipFile(blob, "w") as zf:
            zf.writestr("README.md", b"readme")
            zf.comment = b"kept"
        unsigned = blob.getvalue()
        self.assertEqual(vpp.nuget_unsigned(sign(unsigned)), unsigned)

    def test_an_unsigned_package_is_returned_unchanged(self):
        unsigned = pointer_nupkg()
        self.assertIs(vpp.nuget_unsigned(unsigned), unsigned)

    def test_an_entry_with_another_case_is_not_the_signature(self):
        """NuGet names the entry exactly, so a look-alike stays in the package."""
        signed = sign(pointer_nupkg(), name=".SIGNATURE.P7S")
        self.assertIs(vpp.nuget_unsigned(signed), signed)

    def test_a_package_that_cannot_be_unsigned_is_refused_with_the_reason(self):
        signed = sign(pointer_nupkg())

        def damaged_directory():
            _, _, _, offset = end_record(signed)
            return signed[:offset] + b"XXXX" + signed[offset + 4:]

        def directory_size_off_by_one():
            eocd, _, size, _ = end_record(signed)
            data = bytearray(signed)
            struct.pack_into("<I", data, eocd + 12, size + 1)
            return bytes(data)

        def two_signatures():
            with warnings.catch_warnings():
                warnings.simplefilter("ignore")
                return sign(signed)

        def signature_first():
            blob = io.BytesIO()
            with zipfile.ZipFile(blob, "w") as zf:
                zf.writestr(".signature.p7s", b"signature")
                zf.writestr("README.md", b"readme")
            return blob.getvalue()

        cases = [
            ("not a zip", lambda: b"not a package", "no end-of-central-directory record"),
            ("an end record cut short", lambda: signed[:-4], "no end-of-central-directory record"),
            ("bytes after the end record", lambda: signed + b"junk", "bytes follow"),
            ("a damaged central record", damaged_directory, "no central directory record at offset"),
            ("a directory of another size", directory_size_off_by_one, "not the size its end record says"),
            ("two signatures", two_signatures, "more than one .signature.p7s"),
            ("a signature before another entry", signature_first, "not the last local entry"),
        ]
        for name, build, want in cases:
            with self.subTest(name):
                with self.assertRaises(ValueError) as caught:
                    vpp.nuget_unsigned(build())
                self.assertIn(want, str(caught.exception))


class AttestedNupkgDigestsTest(unittest.TestCase):
    """The nuget job's output, read back in sha256sum's own format."""

    def test_reads_the_lines_it_wants_and_ignores_the_rest(self):
        a, b = "a" * 64, "B" * 64
        text = (
            f"{a}  gitlab-mcp-server.1.0.0.nupkg\r\n"
            f"{b} *gitlab-mcp-server.linux-x64.1.0.0.nupkg\n"
            f"{'c' * 64}  checksums.txt\n"
            f"{'d' * 63}  gitlab-mcp-server.osx-x64.1.0.0.nupkg\n"
            "lonely-token\n"
            "\n"
        )
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "nupkg.sha256")
            with open(path, "w", encoding="utf-8", newline="") as fh:
                fh.write(text)
            self.assertEqual(
                vpp.attested_nupkg_digests(path),
                {
                    "gitlab-mcp-server.1.0.0.nupkg": a,
                    "gitlab-mcp-server.linux-x64.1.0.0.nupkg": b.lower(),
                },
            )


class NugetFixture(unittest.TestCase):
    """Seven packages as nuget.org serves them, signed, and the digests the
    nuget job would have recorded for them before the push."""

    VERSION = "1.0.0"

    def setUp(self):
        self.real_urlopen = vpp.urllib.request.urlopen
        self.addCleanup(setattr, vpp.urllib.request, "urlopen", self.real_urlopen)
        self.binary = b"\x7fELFthe bytes the release signed"
        self.digests = {asset: hashlib.sha256(self.binary).hexdigest() for asset in vpp.NUGET_ASSETS.values()}
        self.unsigned = {vpp.nuget_package_file(vpp.NUGET_ID, self.VERSION): pointer_nupkg()}
        for rid in vpp.NUGET_ASSETS:
            self.unsigned[vpp.nuget_package_file(f"{vpp.NUGET_ID}.{rid}", self.VERSION)] = nupkg(rid, self.binary)
        self.attested = {name: hashlib.sha256(blob).hexdigest() for name, blob in self.unsigned.items()}
        self.served = {name: sign(blob) for name, blob in self.unsigned.items()}

    def url(self, filename):
        pkg_id = filename[: -len(f".{self.VERSION}.nupkg")]
        return vpp.nuget_package_url(pkg_id, self.VERSION)

    def serve(self):
        script = {f"{vpp.NUGET_FLAT}/{vpp.NUGET_ID}/index.json": [b'{"versions": ["%s"]}' % self.VERSION.encode()]}
        for filename, blob in self.served.items():
            script[self.url(filename)] = [blob]
        opener = FakeOpener(script)
        vpp.urllib.request.urlopen = opener
        return opener

    def check(self, attested):
        budget = vpp.RetryBudget(seconds=600, delay=20, sleep=lambda _: self.fail("a finding was retried"))
        problems = []
        with contextlib.redirect_stdout(io.StringIO()):
            vpp.check_nuget(self.VERSION, self.digests, problems, budget, attested)
        return problems


class NugetAttestedTest(NugetFixture):
    """check_nuget holds every served package, signature removed, to the
    digest the nuget job attested, and never waits a mismatch out."""

    def test_served_packages_equal_to_the_attested_ones_pass_each_downloaded_once(self):
        opener = self.serve()
        self.assertEqual(self.check(self.attested), [])
        for filename in self.served:
            with self.subTest(filename):
                self.assertEqual(opener.count(self.url(filename)), 1)

    def test_each_departure_from_the_attestation_is_named(self):
        pointer = vpp.nuget_package_file(vpp.NUGET_ID, self.VERSION)
        linux = vpp.nuget_package_file(f"{vpp.NUGET_ID}.linux-x64", self.VERSION)

        def pointer_rebuilt_differently():
            self.served[pointer] = sign(pointer_nupkg(readme=b"another readme"))
            return self.attested

        def runtime_package_not_attested():
            return {k: v for k, v in self.attested.items() if k != linux}

        def another_package_attested():
            return {**self.attested, "gitlab-mcp-server.linux-musl-x64.1.0.0.nupkg": "e" * 64}

        def pointer_cannot_be_unsigned():
            self.served[pointer] = self.served[pointer] + b"junk"
            return self.attested

        def pointer_not_served():
            del self.served[pointer]
            return self.attested

        cases = [
            ("the pointer differs from what was attested", pointer_rebuilt_differently,
             "without its repository signature is sha256"),
            ("a runtime package has no attested digest", runtime_package_not_attested,
             f"recorded no attested digest for {linux}"),
            ("a package nobody published was attested", another_package_attested,
             "which is not one of the packages of 1.0.0"),
            ("the pointer cannot be unsigned", pointer_cannot_be_unsigned,
             "cannot be unsigned the way NuGet defines it"),
            ("the pointer cannot be downloaded", pointer_not_served,
             f"nuget {vpp.NUGET_ID}: could not read the published package"),
        ]
        for name, arrange, want in cases:
            with self.subTest(name):
                self.setUp()
                attested = arrange()
                self.serve()
                budget = vpp.RetryBudget(seconds=0)
                problems = []
                with contextlib.redirect_stdout(io.StringIO()):
                    vpp.check_nuget(self.VERSION, self.digests, problems, budget, attested)
                self.assertEqual(len(problems), 1, problems)
                self.assertIn(want, problems[0])

    def test_a_mismatch_is_not_retried(self):
        linux = vpp.nuget_package_file(f"{vpp.NUGET_ID}.linux-x64", self.VERSION)
        attested = {**self.attested, linux: "0" * 64}
        opener = self.serve()
        problems = self.check(attested)
        self.assertEqual(len(problems), 1, problems)
        self.assertIn(f"attested {linux} as {'0' * 64}", problems[0])
        self.assertEqual(opener.count(self.url(linux)), 1)

    def test_no_attested_digests_at_all_is_one_finding_and_the_binaries_are_still_checked(self):
        opener = self.serve()
        problems = self.check({})
        self.assertEqual(len(problems), 1, problems)
        self.assertIn("name no package", problems[0])
        pointer = vpp.nuget_package_file(vpp.NUGET_ID, self.VERSION)
        self.assertEqual(opener.count(self.url(pointer)), 0, "nothing to compare the pointer with")
        for rid in vpp.NUGET_ASSETS:
            with self.subTest(rid):
                self.assertEqual(opener.count(self.url(vpp.nuget_package_file(f"{vpp.NUGET_ID}.{rid}", self.VERSION))), 1)

    def test_a_runtime_package_without_its_binary_is_named_once(self):
        """Read once for both checks, so a package with nothing to compare is
        one finding, not one per comparison."""
        linux = vpp.nuget_package_file(f"{vpp.NUGET_ID}.linux-x64", self.VERSION)
        self.served[linux] = sign(pointer_nupkg())
        self.serve()
        problems = self.check(self.attested)
        self.assertEqual(len(problems), 1, problems)
        self.assertIn("nuget linux-x64: could not read the published package", problems[0])
        self.assertIn("ships no gitlab-mcp-server binary under tools/any/linux-x64/", problems[0])

    def test_without_attested_digests_the_pointer_is_not_downloaded(self):
        opener = self.serve()
        self.assertEqual(self.check(None), [])
        pointer = vpp.nuget_package_file(vpp.NUGET_ID, self.VERSION)
        self.assertEqual(opener.count(self.url(pointer)), 0)


class MainTest(NugetFixture):
    """The command line hands --nuget-digests to the NuGet check."""

    def run_main(self, *extra):
        with tempfile.TemporaryDirectory() as tmp:
            checksums = os.path.join(tmp, "checksums.txt")
            with open(checksums, "w", encoding="utf-8") as fh:
                for asset, digest in self.digests.items():
                    fh.write(f"{digest}  {asset}\n")
            digests_file = os.path.join(tmp, "nupkg.sha256")
            with open(digests_file, "w", encoding="utf-8") as fh:
                for name, digest in self.attested.items():
                    fh.write(f"{digest}  {name}\n")
            argv = ["verify_published_packages.py", "--skip-npm", "--skip-pypi", "--retry-budget", "0",
                    *[digests_file if arg == "{digests}" else arg for arg in extra], self.VERSION, checksums]
            out = io.StringIO()
            real_argv = sys.argv
            sys.argv = argv
            try:
                with contextlib.redirect_stdout(out):
                    try:
                        vpp.main()
                        code = 0
                    except SystemExit as exc:
                        code = exc.code
            finally:
                sys.argv = real_argv
            return code, out.getvalue()

    def test_the_attested_digests_reach_the_check(self):
        self.serve()
        code, out = self.run_main("--nuget-digests", "{digests}")
        self.assertEqual(code, 0, out)
        self.assertIn("is, without its repository signature, the one the release attested", out)
        self.assertIn("unsigned, equals the attested", out)

    def test_a_package_that_departs_from_its_attestation_fails_the_run(self):
        pointer = vpp.nuget_package_file(vpp.NUGET_ID, self.VERSION)
        self.served[pointer] = sign(pointer_nupkg(readme=b"another readme"))
        self.serve()
        code, out = self.run_main("--nuget-digests", "{digests}")
        self.assertEqual(code, 1, out)
        self.assertIn("without its repository signature is sha256", out)

    def test_skipping_nuget_reads_no_digests_and_downloads_nothing(self):
        opener = self.serve()
        code, out = self.run_main("--skip-nuget", "--nuget-digests", "{digests}")
        self.assertEqual(code, 0, out)
        self.assertEqual(opener.calls, [])
        self.assertNotIn("the one the release attested", out)

    def test_without_the_flag_the_run_says_what_it_did_not_compare(self):
        self.serve()
        code, out = self.run_main()
        self.assertEqual(code, 0, out)
        self.assertIn("no --nuget-digests given", out)
        self.assertNotIn("the one the release attested", out)
        self.assertIn(f"names no {vpp.NOTICES}, a release from before they were generated", out)

    def test_a_release_that_signed_its_notices_does_not_say_it_predates_them(self):
        self.digests[vpp.NOTICES] = "b" * 64
        self.serve()
        code, out = self.run_main()
        self.assertEqual(code, 0, out)
        self.assertNotIn(f"names no {vpp.NOTICES}", out)


class WaitAllowancesTest(unittest.TestCase):
    """main() gives npm and PyPI one budget of sleep and NuGet a deadline of
    its own counted from the start of the run, and --retry-budget 0 turns both
    off. The timeline is a harsher variant of 3.1.0's, measured from the
    step's start. On 3.1.0 npm's lag took 13 of the 16 waits and NuGet's
    first runtime package the other 3; here two npm platform packages answer
    404 for about eight minutes, so npm spends the whole budget, and nuget.org
    serves the seven packages 23 minutes in, as it did on 3.1.0."""

    VERSION = "1.0.0"
    INDEX = f"{vpp.NUGET_FLAT}/{vpp.NUGET_ID}/index.json"
    NPM_LAGGING = ("linux-arm64", "win32-arm64")

    def setUp(self):
        self.real_urlopen = vpp.urllib.request.urlopen
        self.addCleanup(setattr, vpp.urllib.request, "urlopen", self.real_urlopen)
        self.binary = b"\x7fELFthe bytes the release signed"
        signed = hashlib.sha256(self.binary).hexdigest()
        assets = set(vpp.NPM_ASSETS.values()) | set(vpp.NUGET_ASSETS.values())
        self.digests = {asset: signed for asset in assets}
        self.clock = FakeClock()
        self.runtime_urls = [vpp.nuget_package_url(f"{vpp.NUGET_ID}.{rid}", self.VERSION) for rid in vpp.NUGET_ASSETS]

    def serve(self, npm_lag_until, nuget_at, nuget_binary=None):
        """Serve the npm packages, NPM_LAGGING from npm_lag_until on and the
        rest at once, and the NuGet packages and the pointer's listing from
        nuget_at on (never, when it is None), carrying nuget_binary when one is
        named."""
        script = {self.INDEX: [(0, b'{"versions": ["0.9.0"]}')]}
        for suffix in vpp.NPM_ASSETS:
            quoted = vpp.urllib.parse.quote(f"{vpp.NPM_SCOPE}/gitlab-mcp-server-{suffix}", safe="")
            tarball_url = f"https://registry.npmjs.org/{quoted}/-/{suffix}-{self.VERSION}.tgz"
            since = npm_lag_until if suffix in self.NPM_LAGGING else 0
            script[f"https://registry.npmjs.org/{quoted}/{self.VERSION}"] = [
                (since, b'{"dist": {"tarball": "%s"}}' % tarball_url.encode())
            ]
            script[tarball_url] = [(0, npm_tarball(self.binary))]
        if nuget_at is not None:
            script[self.INDEX].append((nuget_at, b'{"versions": ["0.9.0", "%s"]}' % self.VERSION.encode()))
            for rid in vpp.NUGET_ASSETS:
                url = vpp.nuget_package_url(f"{vpp.NUGET_ID}.{rid}", self.VERSION)
                script[url] = [(nuget_at, nupkg(rid, nuget_binary or self.binary))]
        opener = ClockedOpener(self.clock, script)
        vpp.urllib.request.urlopen = opener
        return opener

    def run_main(self, *extra, sleep=None):
        with tempfile.TemporaryDirectory() as tmp:
            checksums = os.path.join(tmp, "checksums.txt")
            with open(checksums, "w", encoding="utf-8") as fh:
                for asset, digest in sorted(self.digests.items()):
                    fh.write(f"{digest}  {asset}\n")
            argv = ["--skip-pypi", "--retry-delay", "30", *extra, self.VERSION, checksums]
            out = io.StringIO()
            with contextlib.redirect_stdout(out):
                try:
                    vpp.main(argv, clock=self.clock, sleep=sleep or self.clock.sleep)
                    code = 0
                except SystemExit as exc:
                    code = exc.code
            return code, out.getvalue()

    def test_an_npm_lag_spends_npm_allowance_without_touching_nugets(self):
        """npm spends all 480s of its budget; NuGet then waits until 1380s,
        inside its 2400s deadline. With one shared budget, NuGet would have
        had nothing left and the pointer would have been read once."""
        opener = self.serve(npm_lag_until=470, nuget_at=1380)
        code, out = self.run_main("--retry-budget", "480", "--nuget-deadline", "2400")
        self.assertEqual(code, 0, out)
        npm_waits = 16
        self.assertEqual(self.clock.slept[:npm_waits], [30] * npm_waits, "npm spent its whole budget")
        self.assertEqual(sum(self.clock.slept), 1380)
        self.assertEqual(opener.count(self.INDEX), 31, "read at 480s, then every 30s until 1380s")
        for url in self.runtime_urls:
            with self.subTest(url):
                self.assertEqual(opener.count(url), 1, "served by the time the pointer was listed")
        self.assertEqual(out.count("  ok  nuget "), len(vpp.NUGET_ASSETS) + 1, out)

    def test_the_nuget_deadline_is_counted_from_the_start_of_the_run(self):
        """npm's 480s are the first 480s of NuGet's 600, so NuGet waits four
        times, not twenty, and every NuGet check then fails promptly."""
        opener = self.serve(npm_lag_until=470, nuget_at=700)
        code, out = self.run_main("--retry-budget", "480", "--nuget-deadline", "600")
        self.assertEqual(code, 1, out)
        self.assertEqual(self.clock.now, 600)
        self.assertEqual(opener.count(self.INDEX), 5)
        self.assertIn(f"version {self.VERSION} is not listed on nuget.org (still unlisted after 5 attempt(s))", out)
        for url in self.runtime_urls:
            with self.subTest(url):
                self.assertEqual(opener.count(url), 1, "the deadline was spent, so one attempt each")
        self.assertIn("npm and PyPI retried 16 time(s); 0s of their 480s retry budget was left unspent.", out)
        self.assertIn("NuGet retried 4 time(s); 0s of its 600s deadline, counted from the start of this run, "
                      "was left.", out)
        # Every NuGet wait here was spent reading an index that answered 200
        # without the version, so the summary cannot say only that a download
        # never came back.
        self.assertIn("An allowance spent to zero means a registry never served the version "
                      "(a download never came back, or the pointer's index never listed it)", out)

    def test_a_zero_retry_budget_disables_every_wait_nugets_included(self):
        """The out-of-band run: nothing is waited for, NuGet included, so an
        unpublished NuGet version is reported on the first look."""
        opener = self.serve(npm_lag_until=0, nuget_at=None)
        code, out = self.run_main("--retry-budget", "0", "--nuget-deadline", "2400",
                                  sleep=lambda _: self.fail("waited although --retry-budget is 0"))
        self.assertEqual(code, 1, out)
        self.assertEqual(opener.count(self.INDEX), 1)
        for url in self.runtime_urls:
            with self.subTest(url):
                self.assertEqual(opener.count(url), 1)
        self.assertNotIn("retried", out)

    def test_a_nuget_digest_mismatch_fails_at_once_whatever_the_deadline(self):
        opener = self.serve(npm_lag_until=0, nuget_at=0, nuget_binary=b"\x7fELFsomething else entirely")
        code, out = self.run_main("--retry-budget", "480", "--nuget-deadline", "2400",
                                  sleep=lambda _: self.fail("a mismatch was retried"))
        self.assertEqual(code, 1, out)
        self.assertEqual(out.count("carries sha256"), len(vpp.NUGET_ASSETS), out)
        for url in self.runtime_urls:
            with self.subTest(url):
                self.assertEqual(opener.count(url), 1)

    def test_the_defaults_are_the_ones_stated(self):
        """The release job passes its values explicitly; a run by hand gets
        these, and the NuGet deadline is the one sized on 3.1.0's 23 minutes."""
        self.assertEqual(vpp.RETRY_BUDGET, 600)
        self.assertEqual(vpp.RETRY_DELAY, 20)
        self.assertEqual(vpp.NUGET_DEADLINE, 2400)


if __name__ == "__main__":
    unittest.main()
