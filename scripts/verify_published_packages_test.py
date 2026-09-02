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

Run with:

    python3 -m unittest discover -s scripts -p 'verify_published_packages_test.py'
"""

import hashlib
import importlib.util
import io
import os
import tarfile
import unittest
import urllib.error
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


def npm_tarball(payload):
    """A .tgz shaped like a published npm platform package."""
    blob = io.BytesIO()
    with tarfile.open(fileobj=blob, mode="w:gz") as tar:
        info = tarfile.TarInfo("package/bin/gitlab-mcp-server")
        info.size = len(payload)
        tar.addfile(info, io.BytesIO(payload))
    return blob.getvalue()


def wheel(version, payload):
    """A .whl shaped like a published platform wheel."""
    blob = io.BytesIO()
    with zipfile.ZipFile(blob, "w") as zf:
        zf.writestr(f"{vpp.PYPI_NORM}-{version}.data/scripts/gitlab-mcp-server", payload)
    return blob.getvalue()


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


if __name__ == "__main__":
    unittest.main()
