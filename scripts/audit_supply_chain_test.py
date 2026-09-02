#!/usr/bin/env python3
"""Tests for scripts/audit_supply_chain.py.

Each test feeds the checker the shape the repository had when the corresponding
security finding was written, asserts it is reported, then feeds it the shape
after the fix and asserts silence. Run with:

    python3 -m unittest discover -s scripts -p 'audit_supply_chain_test.py'
"""

import os
import sys
import tempfile
import textwrap
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import audit_supply_chain as audit  # noqa: E402  (path juggling must precede the import)

SHA = "3d3c42e5aac5ba805825da76410c181273ba90b1"


class PinnedUsesTest(unittest.TestCase):
    """Verifies that only 40-character commit SHAs count as a pinned action.

    A mutable major tag is resolved by the runner at job start, so it is the
    reference an upstream tag hijack (CVE-2025-30066) travels through.
    """

    def test_reports_every_unpinned_reference(self):
        cases = [
            ("mutable major tag", "actions/checkout@v7", False),
            ("exact version tag", "sigstore/cosign-installer@v4.1.2", False),
            ("branch", "some/action@main", False),
            ("short sha", "some/action@3d3c42e", False),
            ("commit sha", f"actions/checkout@{SHA}", True),
            ("commit sha with version comment", f"actions/checkout@{SHA} # v7", True),
            ("subpath action pinned", f"github/codeql-action/init@{SHA} # v4", True),
            ("local action", "./.github/actions/thing", True),
        ]
        for name, ref, want_ok in cases:
            with self.subTest(name):
                text = f"jobs:\n  a:\n    steps:\n      - uses: {ref}\n"
                problems = audit.check_pinned_uses("wf.yml", text)
                self.assertEqual(problems == [], want_ok, problems)


class CredentialedJobTest(unittest.TestCase):
    """Verifies which jobs the hardening rules apply to.

    A job is credentialed when its effective permissions (its own, else the
    workflow's) grant contents: write or id-token: write — the two that make it
    worth attacking, since one rewrites the repository and the other mints the
    OIDC identity npm, PyPI, cosign and the MCP Registry all trust.
    """

    def test_effective_permissions_decide(self):
        cases = [
            ("job-level id-token write", {"permissions": {"id-token": "write"}}, {}, True),
            ("job-level contents write", {"permissions": {"contents": "write"}}, {}, True),
            ("job-level read only", {"permissions": {"contents": "read"}}, {}, False),
            ("job-level none", {"permissions": {}}, {"permissions": {"id-token": "write"}}, False),
            ("inherits workflow write", {}, {"permissions": {"id-token": "write"}}, True),
            ("inherits workflow read", {}, {"permissions": {"contents": "read"}}, False),
            ("no permissions anywhere", {}, {}, False),
            ("packages write only", {"permissions": {"packages": "write"}}, {}, False),
        ]
        for name, job, doc, want in cases:
            with self.subTest(name):
                self.assertEqual(audit.is_credentialed(doc, job), want)


class CheckoutCredentialTest(unittest.TestCase):
    """Verifies that a credentialed job's checkout must not persist the token.

    actions/checkout defaults persist-credentials to true, leaving the
    write-capable GITHUB_TOKEN in .git/config for every later step — including
    the ones that run third-party code.
    """

    def test_persist_credentials_must_be_false(self):
        cases = [
            ("default (true)", None, True),
            ("explicit true", {"persist-credentials": True}, True),
            ("explicit false", {"persist-credentials": False}, False),
            ("false beside other inputs", {"fetch-depth": 0, "persist-credentials": False}, False),
        ]
        for name, with_, want_problem in cases:
            with self.subTest(name):
                step = {"uses": f"actions/checkout@{SHA}"}
                if with_ is not None:
                    step["with"] = with_
                doc = {"jobs": {"release": {"permissions": {"contents": "write"}, "steps": [step]}}}
                problems = audit.check_workflow_jobs("wf.yml", doc, ".")
                self.assertEqual(bool(problems), want_problem, problems)

    def test_read_only_job_is_not_subject_to_the_rule(self):
        doc = {"jobs": {"lint": {"permissions": {"contents": "read"}, "steps": [{"uses": f"actions/checkout@{SHA}"}]}}}
        self.assertEqual(audit.check_workflow_jobs("wf.yml", doc, "."), [])


class DownloadedToolPinTest(unittest.TestCase):
    """Verifies that tools an action downloads are pinned, not just the action.

    SHA-pinning goreleaser-action or sbom-action fixes the JavaScript that runs;
    the binary it then fetches is chosen by a version input, and 'latest' or
    '~> v2' leaves that binary unpinned in the same job that holds the signing
    identity.
    """

    def test_goreleaser_and_syft_versions(self):
        cases = [
            ("goreleaser range", "goreleaser/goreleaser-action", {"version": "~> v2"}, True),
            ("goreleaser latest", "goreleaser/goreleaser-action", {"version": "latest"}, True),
            ("goreleaser exact", "goreleaser/goreleaser-action", {"version": "v2.13.0"}, False),
            ("syft unpinned", "anchore/sbom-action/download-syft", {}, True),
            ("syft floating major", "anchore/sbom-action/download-syft", {"syft-version": "v1"}, True),
            ("syft pinned", "anchore/sbom-action/download-syft", {"syft-version": "v1.36.0"}, False),
        ]
        for name, action, with_, want_problem in cases:
            with self.subTest(name):
                job = {
                    "permissions": {"id-token": "write"},
                    "steps": [{"uses": f"{action}@{SHA}", "with": with_}],
                }
                problems = audit.check_credentialed_job("wf.yml", "release", job, ".")
                self.assertEqual(bool(problems), want_problem, problems)

    def test_versions_declared_in_the_workflow_env_are_resolved(self):
        """A pin held in `env:` and referenced by a step is still a pin."""
        cases = [
            ("resolves to an exact version", {"GORELEASER_VERSION": "v2.18.0"}, False),
            ("resolves to a range", {"GORELEASER_VERSION": "~> v2"}, True),
            ("names an undefined variable", {}, True),
        ]
        for name, env, want_problem in cases:
            with self.subTest(name):
                doc = {"env": env}
                job = {
                    "permissions": {"id-token": "write"},
                    "steps": [{
                        "uses": f"goreleaser/goreleaser-action@{SHA}",
                        "with": {"version": "${{ env.GORELEASER_VERSION }}"},
                    }],
                }
                problems = audit.check_credentialed_job("wf.yml", "release", job, ".", doc)
                self.assertEqual(bool(problems), want_problem, problems)


class UnlockedCodeTest(unittest.TestCase):
    """Verifies that a credentialed job runs nothing resolved at run time.

    The release job holds the npm and PyPI trusted-publisher identities, the
    cosign signer and a write-scoped token, and used to invoke `npx --yes`,
    whose nine caret ranges were resolved fresh from the registry on every
    release. A rationale comment mentioning the pattern is not the pattern.
    """

    def test_run_block_patterns(self):
        cases = [
            ("npx", "npx --yes @anthropic-ai/mcpb@2.1.2 pack a b", True),
            ("go install latest", "go install gotest.tools/gotestsum@latest", True),
            ("curl piped to sh", "curl -fsSL https://example.test/i.sh | sh", True),
            ("pip install unhashed", "pip install --quiet jsonschema", True),
            ("pip install hashed", "pip install --require-hashes -r req.txt", False),
            ("comment about npx", "# Pinned, not @latest: npx would resolve at run time\nnpm ci", False),
            ("pinned npm install", "npm install -g npm@11.5.1", False),
            ("plain build", "make build", False),
        ]
        for name, run, want_problem in cases:
            with self.subTest(name):
                job = {"permissions": {"contents": "write"}, "steps": [{"run": run}]}
                problems = audit.check_credentialed_job("wf.yml", "release", job, ".")
                self.assertEqual(bool(problems), want_problem, problems)

    def test_scripts_a_credentialed_job_invokes_are_scanned(self):
        with tempfile.TemporaryDirectory() as root:
            os.makedirs(os.path.join(root, "scripts"))
            path = os.path.join(root, "scripts", "build-thing.sh")
            job = {
                "permissions": {"id-token": "write"},
                "steps": [{"run": "bash scripts/build-thing.sh 2.7.5"}],
            }
            cases = [
                ("script runs npx", 'npx --yes "@anthropic-ai/mcpb@2.1.2" pack a b\n', True),
                ("script uses zip", 'cd bundle && zip -r -X ../out.mcpb .\n', False),
            ]
            for name, body, want_problem in cases:
                with self.subTest(name):
                    with open(path, "w", encoding="utf-8") as fh:
                        fh.write(body)
                    problems = audit.check_credentialed_job("wf.yml", "release", job, root)
                    self.assertEqual(bool(problems), want_problem, problems)


class DependabotCooldownTest(unittest.TestCase):
    """Verifies that every cooldown-capable ecosystem states its own window.

    An absent cooldown key means 'whatever GitHub defaults to today', which is
    not a property this repository controls. SemVer sub-keys on the docker
    ecosystems are rejected by Dependabot, and a rejected configuration stops
    that ecosystem's updates entirely.
    """

    def test_entries(self):
        cases = [
            ("gomod without cooldown", {"package-ecosystem": "gomod", "directory": "/"}, True),
            ("gomod with 7 days", {"package-ecosystem": "gomod", "directory": "/", "cooldown": {"default-days": 7}}, False),
            ("gomod with 1 day", {"package-ecosystem": "gomod", "directory": "/", "cooldown": {"default-days": 1}}, True),
            ("docker with semver keys", {"package-ecosystem": "docker", "directory": "/", "cooldown": {"default-days": 7, "semver-major-days": 30}}, True),
            ("docker plain", {"package-ecosystem": "docker", "directory": "/", "cooldown": {"default-days": 7}}, False),
            ("docker-compose exempt", {"package-ecosystem": "docker-compose", "directory": "/"}, False),
        ]
        for name, entry, want_problem in cases:
            with self.subTest(name):
                problems = audit.check_dependabot({"updates": [entry]})
                self.assertEqual(bool(problems), want_problem, problems)


class SecurityPolicyTest(unittest.TestCase):
    """Verifies that SECURITY.md's supported-versions table tracks VERSION.

    The self-updater removal edited the paragraph beside the table and stepped
    over the table itself, leaving 1.x advertised as the supported line while
    the repository shipped 2.7.5 — a reporter could not tell whether 2.x was
    receiving fixes at all.
    """

    def test_table_must_name_the_shipping_major(self):
        stale = textwrap.dedent("""\
            | Version              | Supported          |
            | -------------------- | ------------------ |
            | Latest `1.x` release | :white_check_mark: |
            | Older `1.x` releases | :x:                |
            """)
        current = textwrap.dedent("""\
            | Version              | Supported          |
            | -------------------- | ------------------ |
            | Latest `2.x` release | :white_check_mark: |
            | Older `2.x` releases | :x:                |
            | `1.x` and `0.x`      | :x:                |
            """)
        cases = [
            ("stale 1.x table on a 2.x release", "2.7.5\n", stale, True),
            ("current 2.x table on a 2.x release", "2.7.5\n", current, False),
            ("same table once 1.x ships", "1.9.0\n", stale, False),
            ("no table at all", "2.7.5\n", "Report privately.\n", True),
        ]
        for name, version, table, want_problem in cases:
            with self.subTest(name):
                problems = audit.check_security_policy(version, table)
                self.assertEqual(bool(problems), want_problem, problems)


class InstallerSignatureTest(unittest.TestCase):
    """Verifies that both installers reach for the release's Sigstore bundle.

    checksums.txt is fetched from the same mutable release as the binary, so a
    principal who can clobber release assets replaces both consistently and the
    hash comparison passes. Every release already publishes
    checksums.txt.sigstore.json; the installers used to ignore it.
    """

    def test_both_installers(self):
        without = "dl \"$base/checksums.txt\" \"$tmp/checksums.txt\"\n"
        with_ = without + "cosign verify-blob --bundle \"$tmp/checksums.txt.sigstore.json\" \"$tmp/checksums.txt\"\n"
        cases = [
            ("neither verifies", without, without, 4),
            ("only sh verifies", with_, without, 2),
            ("both verify", with_, with_, 0),
        ]
        for name, sh, ps1, want_count in cases:
            with self.subTest(name):
                self.assertEqual(len(audit.check_installers(sh, ps1)), want_count)


class RepositoryTest(unittest.TestCase):
    """Verifies the audit passes on the repository as committed.

    This is the gate itself: it fails the build the moment an action is
    unpinned, a release job gains an unlocked download, a cooldown is dropped,
    the security policy goes stale, or an installer stops checking signatures.
    """

    def test_repository_is_clean(self):
        root = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
        problems = audit.audit(root)
        self.assertEqual(problems, [], "\n".join(problems))


if __name__ == "__main__":
    unittest.main()
