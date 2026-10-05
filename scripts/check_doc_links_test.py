#!/usr/bin/env python3
"""Tests how scripts/check-doc-links.mjs (`make check-doc-links`) reads a link to the site.

Issue 1163 made the documentation site the one home of the user
documentation, so the README and every file that used to link into docs/ now
link to https://jmrp.io/docs/gitlab-mcp-server/... instead. The checker holds
such a link to the page under site/src/content/docs that serves it, and to
the heading its anchor names, rather than skipping it as somebody else's
server. The repository's own corpus only shows that every link it carries
resolves; it cannot show that a broken one would be reported, which is the
half that fails silently: a pattern that stops recognising the site's
address, or a return before the anchor is judged, leaves the job green with
nothing checked.

Each case writes a small repository with a site tree and one Markdown file
carrying the links under test, commits nothing (the checker reads what git
tracks, so the files are added to the index), and runs the real script in it.

Run with:

    python3 -m unittest discover -s scripts -p 'check_doc_links_test.py'
"""

import os
import shutil
import subprocess
import tempfile
import unittest

ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
SCRIPT = os.path.join(ROOT, "scripts", "check-doc-links.mjs")

SITE = "https://jmrp.io/docs/gitlab-mcp-server"
PAGES = "https://jmrplens.github.io/gitlab-mcp-server"

# The pages and files the stand-in site serves.
TREE = {
    "site/src/content/docs/index.mdx": "---\ntitle: Home\n---\n\nWelcome.\n",
    "site/src/content/docs/configuration.mdx": "---\ntitle: Configuration\n---\n\n## Environment variables\n\nText.\n",
    "site/src/content/docs/install/index.mdx": "---\ntitle: Install\n---\n\n## Pick a channel\n\nText.\n",
    "site/src/content/docs/install/docker.mdx": "---\ntitle: Docker\n---\n\n## Configure your client\n\nText.\n",
    "site/src/content/docs/es/configuration.mdx": "---\ntitle: Configuración\n---\n\n## Variables de entorno\n\nTexto.\n",
    "site/public/robots.txt": "User-agent: *\n",
    "site/public/benchmarks/en/latency.light.svg": "<svg/>\n",
    "site/scripts/gen-llms.mjs": 'const REFERENCE_FILES = [\n\t{ name: "llms.txt", publishAs: "llms-server.txt" },\n];\n',
}


def require_tool(test, name):
    """Returns the path of a tool, failing under CI and skipping elsewhere when it is missing."""
    path = shutil.which(name)
    if path is None:
        # A skip keeps the job green, so on a runner without the tool this
        # check would stop running and nobody would be told. GitHub sets CI on
        # every step; there a missing tool is a failure.
        if os.environ.get("CI"):
            test.fail(name + " is not installed, and CI must run this check rather than skip it")
        test.skipTest(name + " is not installed")
    return path


class CheckDocLinksSiteTest(unittest.TestCase):
    def setUp(self):
        self.node = require_tool(self, "node")
        self.git = require_tool(self, "git")

    def check(self, links):
        """Runs the checker over a README carrying one link per line, returning (status, output)."""
        with tempfile.TemporaryDirectory() as repo:
            files = dict(TREE)
            files["README.md"] = "# Readme\n\n" + "".join("- [link](" + link + ")\n" for link in links)
            for relative, content in files.items():
                target = os.path.join(repo, relative)
                os.makedirs(os.path.dirname(target), exist_ok=True)
                with open(target, "w", encoding="utf-8") as fh:
                    fh.write(content)
            os.makedirs(os.path.join(repo, "scripts"))
            shutil.copy(SCRIPT, os.path.join(repo, "scripts", "check-doc-links.mjs"))
            subprocess.run([self.git, "init", "-q", repo], check=True)
            subprocess.run([self.git, "-C", repo, "add", "-A"], check=True)
            run = subprocess.run(
                [self.node, os.path.join(repo, "scripts", "check-doc-links.mjs")],
                cwd=repo,
                capture_output=True,
                text=True,
                check=False,
            )
        return run.returncode, run.stdout + run.stderr

    def assert_passes(self, link):
        status, output = self.check([link])
        self.assertEqual(status, 0, link + " was refused:\n" + output)
        self.assertIn("local links are valid", output)

    def assert_refused(self, link, reason):
        status, output = self.check([link])
        self.assertEqual(status, 1, link + " passed:\n" + output)
        self.assertIn(link, output)
        self.assertIn(reason, output)

    def test_page_and_its_heading_pass_under_both_addresses(self):
        for link in [
            SITE + "/configuration/#environment-variables",
            PAGES + "/configuration/#environment-variables",
            SITE + "/configuration",
            SITE + "/configuration/?ref=readme",
        ]:
            with self.subTest(link=link):
                self.assert_passes(link)

    def test_folder_index_spanish_page_and_root_pass(self):
        for link in [
            SITE + "/install/#pick-a-channel",
            SITE + "/install/docker/#configure-your-client",
            SITE + "/es/configuration/#variables-de-entorno",
            SITE,
            SITE + "/",
            SITE + "#_top",
        ]:
            with self.subTest(link=link):
                self.assert_passes(link)

    def test_missing_page_is_refused(self):
        for link, route in [
            (SITE + "/nope/", "/nope"),
            (SITE + "/es/install/docker/", "/es/install/docker"),
            (PAGES + "/operations/telemetry/", "/operations/telemetry"),
        ]:
            with self.subTest(link=link):
                self.assert_refused(link, "no site page serves " + route)

    def test_missing_heading_is_refused(self):
        for link in [
            SITE + "/configuration/#nope",
            SITE + "/install/#configure-your-client",
            SITE + "/es/configuration/#environment-variables",
        ]:
            with self.subTest(link=link):
                self.assert_refused(link, "no Starlight anchor")

    def test_served_files_pass(self):
        for link in [
            SITE + "/robots.txt",
            SITE + "/benchmarks/en/latency.light.svg",
            SITE + "/llms.txt",
            SITE + "/es/llms.txt",
            SITE + "/llms-server.txt",
            PAGES + "/sitemap-index.xml",
            SITE + "/sitemap-0.xml",
        ]:
            with self.subTest(link=link):
                self.assert_passes(link)

    def test_file_the_site_does_not_serve_is_refused(self):
        for link, route in [
            (SITE + "/llms-nope.txt", "/llms-nope.txt"),
            (SITE + "/benchmarks/en/missing.svg", "/benchmarks/en/missing.svg"),
            (PAGES + "/es/llms-server.txt", "/es/llms-server.txt"),
            (SITE + "/../README.md", "/../README.md"),
        ]:
            with self.subTest(link=link):
                self.assert_refused(link, "no file the site serves " + route)

    def test_another_host_is_skipped(self):
        for link in [
            "https://jmrp.io/projects/",
            "https://jmrp.io/docs/gitlab-mcp-server-other/nope/",
            "https://example.com/docs/gitlab-mcp-server/nope/",
        ]:
            with self.subTest(link=link):
                self.assert_passes(link)

    def test_every_broken_link_is_reported_at_once(self):
        status, output = self.check([SITE + "/nope/", SITE + "/configuration/#nope", SITE + "/llms-nope.txt"])
        self.assertEqual(status, 1, output)
        self.assertIn("no site page serves /nope", output)
        self.assertIn("no Starlight anchor #nope", output)
        self.assertIn("no file the site serves /llms-nope.txt", output)


if __name__ == "__main__":
    unittest.main()
