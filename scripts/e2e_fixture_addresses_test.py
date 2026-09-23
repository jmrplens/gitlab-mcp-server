#!/usr/bin/env python3
"""Tests where the e2e Docker fixture's side services are looked for.

test/e2e/scripts/run-docker-e2e.sh reads the host out of the GitLab URL and
derives from it where Bitbucket is reached, where the registry says it lives,
and which address the Bitbucket port is published on. The derivation lives in
test/e2e/scripts/fixture-addresses.sh so that these tests can drive it with
the URLs it has to handle, without starting a container.

The failure it guards against is quiet: the Bitbucket setup script is best
effort, so a URL it cannot reach makes it wait, warn and exit 0, and the
import test then skips. A GitLab reached on [::1] used to publish Bitbucket on
127.0.0.1 while the script dialled [::1], one reached on 127.0.0.2 did the
same while the script dialled 127.0.0.2, and a GitLab reached as LOCALHOST
published it on every interface. Every one of those runs passed.

Run with:

    python3 -m unittest discover -s scripts -p 'e2e_fixture_addresses_test.py'
"""

import os
import subprocess
import unittest

ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
SCRIPT = os.path.join(ROOT, "test", "e2e", "scripts", "fixture-addresses.sh")

# The variables the derivation reads and writes, cleared before every case so
# the developer's own environment is never part of one.
DERIVED = ("E2E_DOCKER_BITBUCKET_URL", "E2E_REGISTRY_EXTERNAL_URL", "E2E_BITBUCKET_BIND")

# Sources the script under the options run-docker-e2e.sh runs with, derives,
# and prints each result on a line of its own, "<unset>" for one left unset.
# The last line says whether the case-insensitive matching the bind needs was
# left switched on, which would change every later [[ ]] in the caller.
DRIVER = r"""
set -euo pipefail
. "$1"
derive_fixture_addresses
printf '%s\n' "${E2E_DOCKER_BITBUCKET_URL-<unset>}" "${E2E_REGISTRY_EXTERNAL_URL-<unset>}" "${E2E_BITBUCKET_BIND-<unset>}"
if shopt -q nocasematch; then echo nocasematch-on; else echo nocasematch-off; fi
"""


def derive(gitlab_url, **preset):
    """Runs the derivation for one GitLab URL and returns what it produced."""
    env = {key: value for key, value in os.environ.items() if key not in DERIVED}
    env["E2E_DOCKER_GITLAB_URL"] = gitlab_url
    env.update(preset)
    result = subprocess.run(
        ["bash", "-c", DRIVER, "driver", SCRIPT],
        env=env,
        capture_output=True,
        text=True,
        check=True,
    )
    bitbucket, registry, bind, case_matching = result.stdout.splitlines()
    return {
        "bitbucket": bitbucket,
        "registry": registry,
        "bind": bind,
        "case_matching": case_matching,
        "stderr": result.stderr,
    }


class DeriveFixtureAddressesTest(unittest.TestCase):
    def assert_derived(self, gitlab_url, bitbucket, registry, bind, **preset):
        got = derive(gitlab_url, **preset)
        self.assertEqual(
            (got["bitbucket"], got["registry"], got["bind"]),
            (bitbucket, registry, bind),
            f"derived from {gitlab_url!r} with {preset!r}",
        )
        self.assertEqual(got["case_matching"], "nocasematch-off", "the derivation left nocasematch switched on")
        return got

    def test_localhost_is_published_on_ipv4_loopback(self):
        self.assert_derived("http://localhost:8929", "http://localhost:7990", "http://localhost:5050", "127.0.0.1")

    def test_localhost_in_capitals_is_loopback_too(self):
        self.assert_derived("http://LOCALHOST:8929", "http://LOCALHOST:7990", "http://LOCALHOST:5050", "127.0.0.1")

    def test_an_ipv4_loopback_address_is_published_on_ipv4_loopback(self):
        self.assert_derived("http://127.0.0.1:8929", "http://127.0.0.1:7990", "http://127.0.0.1:5050", "127.0.0.1")

    def test_another_ipv4_loopback_address_is_published_on_itself(self):
        # A port published on 127.0.0.1 refuses a connection to 127.0.0.2,
        # which is the address the setup script dials.
        self.assert_derived("http://127.0.0.2:8929", "http://127.0.0.2:7990", "http://127.0.0.2:5050", "127.0.0.2")

    def test_a_name_that_only_begins_like_a_loopback_address_is_not_loopback(self):
        # A name is published on every interface whatever it resolves to, even
        # one spelled as a whole loopback address followed by a domain, and so
        # is a dotted string whose last part is too long to be an octet.
        for host in ("127.example.org", "127.0.0.1.nip.io", "127.0.0.1000"):
            with self.subTest(host=host):
                self.assert_derived(
                    f"http://{host}:8929",
                    f"http://{host}:7990",
                    f"http://{host}:5050",
                    "0.0.0.0",
                )

    def test_ipv6_loopback_is_published_on_the_loopback_it_names(self):
        self.assert_derived("http://[::1]:8929", "http://[::1]:7990", "http://[::1]:5050", "[::1]")

    def test_a_lan_address_is_published_on_every_interface(self):
        self.assert_derived("http://192.168.0.40:8929/", "http://192.168.0.40:7990", "http://192.168.0.40:5050", "0.0.0.0")

    def test_a_name_that_only_begins_like_localhost_is_not_loopback(self):
        self.assert_derived(
            "https://localhost.example:8443",
            "http://localhost.example:7990",
            "http://localhost.example:5050",
            "0.0.0.0",
        )

    def test_a_url_with_no_host_keeps_the_loopback_defaults_and_says_so(self):
        got = self.assert_derived("gitlab", "http://localhost:7990", "<unset>", "127.0.0.1")
        self.assertIn("WARN no host can be read out of E2E_DOCKER_GITLAB_URL=gitlab", got["stderr"])

    def test_a_port_that_is_not_a_number_leaves_no_host_to_read(self):
        got = self.assert_derived("http://gitlab.example:89x29", "http://localhost:7990", "<unset>", "127.0.0.1")
        self.assertIn("WARN no host can be read out of", got["stderr"])

    def test_a_derived_url_says_nothing(self):
        got = self.assert_derived("http://localhost:8929", "http://localhost:7990", "http://localhost:5050", "127.0.0.1")
        self.assertEqual(got["stderr"], "")

    def test_a_bitbucket_url_set_by_hand_moves_the_bind_with_it(self):
        self.assert_derived(
            "http://192.168.0.40:8929",
            "http://[::1]:7990",
            "http://192.168.0.40:5050",
            "[::1]",
            E2E_DOCKER_BITBUCKET_URL="http://[::1]:7990",
        )

    def test_values_set_by_hand_are_kept(self):
        self.assert_derived(
            "http://localhost:8929",
            "http://bitbucket.example:7990",
            "http://registry.example:5050",
            "10.0.0.5",
            E2E_DOCKER_BITBUCKET_URL="http://bitbucket.example:7990",
            E2E_REGISTRY_EXTERNAL_URL="http://registry.example:5050",
            E2E_BITBUCKET_BIND="10.0.0.5",
        )

    def test_a_caller_that_switched_nocasematch_on_keeps_it_on(self):
        env = {key: value for key, value in os.environ.items() if key not in DERIVED}
        env["E2E_DOCKER_GITLAB_URL"] = "http://localhost:8929"
        result = subprocess.run(
            ["bash", "-c", 'shopt -s nocasematch; . "$1"; derive_fixture_addresses; shopt -q nocasematch && echo on', "driver", SCRIPT],
            env=env,
            capture_output=True,
            text=True,
            check=True,
        )
        self.assertEqual(result.stdout.strip(), "on")

    def test_bitbucket_bind_called_in_the_callers_shell_restores_nocasematch(self):
        # derive_fixture_addresses calls it in a command substitution, whose
        # subshell would hide a leak; called directly, nothing does.
        for before, want in (("-u", "off"), ("-s", "on")):
            with self.subTest(nocasematch=want):
                script = (
                    f'shopt {before} nocasematch; . "$1"; bitbucket_bind http://LOCALHOST:7990; '
                    "if shopt -q nocasematch; then echo on; else echo off; fi"
                )
                result = subprocess.run(
                    ["bash", "-c", script, "driver", SCRIPT],
                    capture_output=True,
                    text=True,
                    check=True,
                )
                self.assertEqual(result.stdout.split(), ["127.0.0.1", want])


if __name__ == "__main__":
    unittest.main()
