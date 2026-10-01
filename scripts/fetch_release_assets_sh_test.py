#!/usr/bin/env python3
"""Tests how scripts/fetch-release-assets.sh fetches and verifies the Claude Desktop bundles.

A release publishes one bundle per operating system (gitlab-mcp-server-darwin
.mcpb, -windows.mcpb and -linux.mcpb) beside the universal
gitlab-mcp-server.mcpb. The bundles are built outside GoReleaser, so they are
not in the signed checksums.txt and each is held to its own build-provenance
attestation instead. Two things follow, and both are tested here. The default
patterns, which the npm, PyPI and NuGet jobs use to fetch the binaries, must
not match the per-OS bundles, which a plain gitlab-mcp-server-* glob would.
And every bundle that lands in the destination is verified, one attestation
each, whichever pattern brought it; one that fails verification stops the job.
A rehearsal unpacks its own build and verifies no attestation, since it mints
none.

Each case runs the real script with stand-ins for gh and cosign first on PATH:
gh's release download copies the files of a scratch release that match the
patterns it is given and logs every call, its attestation verify refuses the
files named in FAIL_ATTEST, and cosign accepts every signature.

Run with:

    python3 -m unittest discover -s scripts -p 'fetch_release_assets_sh_test.py'
"""

import hashlib
import os
import shutil
import subprocess
import tarfile
import tempfile
import unittest

ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
SCRIPT = os.path.join(ROOT, "scripts", "fetch-release-assets.sh")
VERSION = "9.8.7"

BINARIES = [
    "gitlab-mcp-server-darwin-all",
    "gitlab-mcp-server-darwin-arm64",
    "gitlab-mcp-server-linux-amd64",
    "gitlab-mcp-server-linux-arm64",
    "gitlab-mcp-server-windows-amd64.exe",
]
SBOMS = [name + ".sbom.json" for name in BINARIES]
NOTICES = "THIRD_PARTY_NOTICES"
PER_OS_BUNDLES = [
    "gitlab-mcp-server-darwin.mcpb",
    "gitlab-mcp-server-windows.mcpb",
    "gitlab-mcp-server-linux.mcpb",
]
UNIVERSAL_BUNDLE = "gitlab-mcp-server.mcpb"
# The signer every attestation is held to: the release workflow at the tag,
# which is also what the cosign check over checksums.txt demands. The stand-in
# gh refuses an attestation verify that does not name it.
SIGNER = "https://github.com/example/gitlab-mcp-server/.github/workflows/release.yml@refs/tags/v" + VERSION

GH = """#!/usr/bin/env bash
echo "$*" >> "$GH_LOG"
if [ "$1 $2" = "release download" ]; then
  shift 3
  dest="" patterns=()
  while [ $# -gt 0 ]; do
    case "$1" in
      --dir) dest="$2"; shift 2 ;;
      --pattern) patterns+=("$2"); shift 2 ;;
      --repo) shift 2 ;;
      *) shift ;;
    esac
  done
  for file in "$RELEASE_DIR"/*; do
    name=${file##*/}
    for pattern in "${patterns[@]}"; do
      # shellcheck disable=SC2254
      case "$name" in
        $pattern) cp "$file" "$dest/$name"; break ;;
      esac
    done
  done
  exit 0
fi
if [ "$1 $2" = "attestation verify" ]; then
  case " $* " in
    *" --cert-identity $EXPECT_IDENTITY "*) ;;
    *) echo "attestation verify names no release workflow signer: $*" >&2; exit 3 ;;
  esac
  name=${3##*/}
  for refused in $FAIL_ATTEST; do
    if [ "$name" = "$refused" ]; then
      echo "no attestation matches $name" >&2
      exit 1
    fi
  done
  exit 0
fi
echo "unexpected gh call: $*" >&2
exit 2
"""

COSIGN = """#!/bin/sh
exit 0
"""


class FetchReleaseAssetsBundleTest(unittest.TestCase):
    """Runs the real script against a scratch release behind a stand-in gh."""

    def setUp(self):
        missing = [tool for tool in ("bash", "sha256sum", "tar") if shutil.which(tool) is None]
        if missing:
            reason = "the script needs " + ", ".join(missing)
            # A skip keeps the job green, so on a runner without these tools
            # every case here would stop running and nobody would be told.
            # GitHub sets CI on every step; there a missing tool is a failure.
            if os.environ.get("CI"):
                self.fail(reason + ", and CI must run these cases rather than skip them")
            self.skipTest(reason)
        self.work = tempfile.mkdtemp(prefix="fetch-release-assets-")
        self.addCleanup(shutil.rmtree, self.work, True)
        self.release = os.path.join(self.work, "release")
        os.makedirs(self.release)
        sums = []
        for name in BINARIES + SBOMS + [NOTICES]:
            body = ("stand-in for " + name).encode()
            self.write(os.path.join(self.release, name), body)
            if name in BINARIES + [NOTICES]:
                sums.append(hashlib.sha256(body).hexdigest() + "  " + name)
        for name in PER_OS_BUNDLES + [UNIVERSAL_BUNDLE]:
            self.write(os.path.join(self.release, name), ("bundle " + name).encode())
        self.write(os.path.join(self.release, "checksums.txt"), ("\n".join(sums) + "\n").encode())
        self.write(os.path.join(self.release, "checksums.txt.sigstore.json"), b"{}")
        self.bin = os.path.join(self.work, "bin")
        os.makedirs(self.bin)
        for name, body in (("gh", GH), ("cosign", COSIGN)):
            path = os.path.join(self.bin, name)
            self.write(path, body.encode())
            os.chmod(path, 0o755)
        self.log = os.path.join(self.work, "gh.log")
        self.dest = os.path.join(self.work, "dist")

    @staticmethod
    def write(path, body):
        with open(path, "wb") as fh:
            fh.write(body)

    def fetch(self, *patterns, fail_attest=(), archive=None):
        env = dict(os.environ)
        env.update(
            PATH=self.bin + os.pathsep + env.get("PATH", ""),
            GH_LOG=self.log,
            RELEASE_DIR=self.release,
            FAIL_ATTEST=" ".join(fail_attest),
            REPO="example/gitlab-mcp-server",
            EXPECT_IDENTITY=SIGNER,
        )
        env.pop("REHEARSAL_ARCHIVE", None)
        if archive is not None:
            env["REHEARSAL_ARCHIVE"] = archive
        return subprocess.run(
            ["bash", SCRIPT, VERSION, self.dest, *patterns],
            cwd=self.work,
            env=env,
            capture_output=True,
            timeout=60,
            check=False,
        )

    def calls(self):
        if not os.path.exists(self.log):
            return []
        with open(self.log, encoding="utf-8") as fh:
            return fh.read().splitlines()

    def fetched(self):
        return sorted(os.listdir(self.dest))

    def test_the_default_patterns_fetch_the_binaries_and_notices_and_no_bundle(self):
        result = self.fetch()
        self.assertEqual(result.returncode, 0, result.stderr.decode())
        self.assertEqual(
            self.fetched(),
            sorted(BINARIES + SBOMS + [NOTICES, "checksums.txt", "checksums.txt.sigstore.json"]),
        )
        self.assertFalse([call for call in self.calls() if call.startswith("attestation")])
        self.assertIn(f"Verified {len(BINARIES) + 1} asset(s) against checksums.txt", result.stdout.decode())

    def test_the_default_patterns_refuse_a_release_without_the_notices(self):
        # gh says nothing about a pattern that matched nothing, so the
        # notices are required by name.
        os.remove(os.path.join(self.release, NOTICES))
        result = self.fetch()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("has no " + NOTICES + " listed in checksums.txt", result.stderr.decode())

    def test_the_default_patterns_refuse_notices_checksums_txt_does_not_list(self):
        sums = os.path.join(self.release, "checksums.txt")
        with open(sums, encoding="utf-8") as fh:
            kept = [line for line in fh.read().splitlines() if not line.endswith("  " + NOTICES)]
        self.write(sums, ("\n".join(kept) + "\n").encode())
        result = self.fetch()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("has no " + NOTICES + " listed in checksums.txt", result.stderr.decode())

    def test_the_default_patterns_refuse_altered_notices(self):
        self.write(os.path.join(self.release, NOTICES), b"something else")
        result = self.fetch()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("an asset does not match checksums.txt", result.stderr.decode())

    def test_named_patterns_do_not_require_the_notices(self):
        # The registry and manifest jobs fetch the bundles alone.
        os.remove(os.path.join(self.release, NOTICES))
        result = self.fetch(*PER_OS_BUNDLES)
        self.assertEqual(result.returncode, 0, result.stderr.decode())

    def test_a_rehearsal_takes_the_notices_from_its_own_build(self):
        archive = os.path.join(self.work, "rehearsal-assets.tar")
        with tarfile.open(archive, "w") as tar:
            for name in ["checksums.txt", NOTICES] + BINARIES:
                tar.add(os.path.join(self.release, name), arcname=name)
        result = self.fetch(archive=archive)
        self.assertEqual(result.returncode, 0, result.stderr.decode())
        self.assertIn(NOTICES, self.fetched())
        self.assertIn(f"Verified {len(BINARIES) + 1} asset(s) against checksums.txt", result.stdout.decode())

    def test_a_rehearsal_without_the_notices_stops_the_default_fetch(self):
        archive = os.path.join(self.work, "rehearsal-assets.tar")
        with tarfile.open(archive, "w") as tar:
            for name in ["checksums.txt"] + BINARIES:
                tar.add(os.path.join(self.release, name), arcname=name)
        result = self.fetch(archive=archive)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("has no " + NOTICES + " listed in checksums.txt", result.stderr.decode())

    def test_every_fetched_bundle_is_held_to_its_own_attestation(self):
        result = self.fetch(*PER_OS_BUNDLES)
        self.assertEqual(result.returncode, 0, result.stderr.decode())
        self.assertEqual(self.fetched(), sorted(PER_OS_BUNDLES + ["checksums.txt", "checksums.txt.sigstore.json"]))
        attested = sorted(call.split()[2].rsplit("/", 1)[1] for call in self.calls() if call.startswith("attestation verify"))
        self.assertEqual(attested, sorted(PER_OS_BUNDLES))
        for name in PER_OS_BUNDLES:
            self.assertIn("Verifying the build-provenance attestation of " + name, result.stdout.decode())
        for call in self.calls():
            if call.startswith("attestation verify"):
                with self.subTest(call.split()[2].rsplit("/", 1)[1]):
                    self.assertIn("--cert-identity " + SIGNER, call)

    def test_a_bundle_whose_attestation_fails_stops_the_job(self):
        result = self.fetch(*PER_OS_BUNDLES, fail_attest=["gitlab-mcp-server-windows.mcpb"])
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("no attestation matches gitlab-mcp-server-windows.mcpb", result.stderr.decode())

    def test_a_bundle_no_pattern_asked_for_is_still_verified(self):
        # Whatever pattern brings a bundle into the destination, it is held to
        # an attestation: here the universal one, through a broad glob.
        result = self.fetch("*.mcpb", fail_attest=[UNIVERSAL_BUNDLE])
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("no attestation matches " + UNIVERSAL_BUNDLE, result.stderr.decode())

    def test_a_rehearsal_counts_each_bundle_of_its_own_build_and_verifies_none(self):
        archive = os.path.join(self.work, "rehearsal-assets.tar")
        with tarfile.open(archive, "w") as tar:
            for name in ["checksums.txt"] + BINARIES + PER_OS_BUNDLES + [UNIVERSAL_BUNDLE]:
                tar.add(os.path.join(self.release, name), arcname=name)
        result = self.fetch(*PER_OS_BUNDLES, archive=archive)
        self.assertEqual(result.returncode, 0, result.stderr.decode())
        self.assertEqual(self.calls(), [], "a rehearsal called gh")
        stdout = result.stdout.decode()
        for name in PER_OS_BUNDLES + [UNIVERSAL_BUNDLE]:
            self.assertIn("Rehearsal: " + name + " came from this run's own build", stdout)


if __name__ == "__main__":
    unittest.main()
