#!/usr/bin/env python3
# Copyright (c) 2026 José M. Requena Plens
# SPDX-License-Identifier: MIT

"""Tests the site's post-build pass over dist/, site/scripts/normalize-html.mjs.

The pass rewrites what html-validate refuses in the built pages, and since
issue 1109 it also holds every page Astro writes for an entry of `redirects`
to sending its reader somewhere: a redirect whose target this site does not
serve fails the build. Astro puts the site's base in front of a redirect's
source and not of its destination, so a destination written without the base
builds into a stub that refreshes to a 404 on the published site, and the
first build of the icons move did exactly that. Nothing else reads a stub's
target, so this is the one place that can.

Each case writes a stand-in dist/ tree and runs the real script in it.

Run with:

    python3 -m unittest discover -s scripts -p 'site_normalize_html_test.py'
"""

import os
import shutil
import subprocess
import tempfile
import unittest

ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
SCRIPT = os.path.join(ROOT, "site", "scripts", "normalize-html.mjs")


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


def write(path, content):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w", encoding="utf-8") as fh:
        fh.write(content)


def read(path):
    with open(path, encoding="utf-8") as fh:
        return fh.read()


def stub(source, target):
    """The page Astro writes for a redirect from source to target, as Astro writes it."""
    return (
        "<!doctype html><title>Redirecting to: " + target + "</title>"
        '<meta http-equiv="refresh" content="0;url=' + target + '">'
        '<meta name="robots" content="noindex">'
        '<link rel="canonical" href="https://docs.jmrp.io' + target + '">'
        '<body>\t<a href="' + target + '">Redirecting from <code>' + source + "</code> to <code>"
        + target + "</code></a></body>"
    )


class NormalizeHtmlTest(unittest.TestCase):
    """site/scripts/normalize-html.mjs, run in a stand-in site."""

    def setUp(self):
        self.node = require_tool(self, "node")
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.site = os.path.join(self.tmp.name, "site")
        os.makedirs(os.path.join(self.site, "scripts"))
        shutil.copy(SCRIPT, os.path.join(self.site, "scripts", "normalize-html.mjs"))
        self.dist = os.path.join(self.site, "dist")
        write(os.path.join(self.dist, "index.html"), "<!DOCTYPE html><title>Home</title>\n")
        write(os.path.join(self.dist, "reference", "icons", "index.html"), "<!DOCTYPE html><title>Icons</title>\n")
        write(os.path.join(self.dist, "es", "reference", "icons", "index.html"), "<!DOCTYPE html><title>Iconos</title>\n")
        write(os.path.join(self.dist, "llms.txt"), "# llms\n")

    def run_script(self):
        run = subprocess.run(
            [self.node, os.path.join(self.site, "scripts", "normalize-html.mjs")],
            cwd=self.site,
            capture_output=True,
            text=True,
            check=False,
        )
        return run.returncode, run.stdout + run.stderr

    def redirect(self, rel, source, target):
        path = os.path.join(self.dist, rel, "index.html")
        write(path, stub(source, target))
        return path

    def test_a_redirect_to_a_built_page_is_normalised_and_passes(self):
        path = self.redirect("capabilities/icons", "/gitlab-mcp-server/capabilities/icons/", "/gitlab-mcp-server/reference/icons/")
        status, output = self.run_script()
        self.assertEqual(status, 0, output)
        self.assertIn("normalised 1 redirect page(s)", output)
        html = read(path)
        self.assertTrue(html.startswith("<!DOCTYPE html>"), html)
        self.assertIn("<title>Page moved</title>", html)
        self.assertIn("Redirecting from <code>/gitlab-mcp-server/capabilities/icons/</code> to <code>", html)

    def test_a_spanish_redirect_page_reads_in_spanish(self):
        path = self.redirect(
            "es/capabilities/icons", "/gitlab-mcp-server/es/capabilities/icons/", "/gitlab-mcp-server/es/reference/icons/"
        )
        status, output = self.run_script()
        self.assertEqual(status, 0, output)
        html = read(path)
        self.assertIn("<title>Página trasladada</title>", html)
        self.assertIn(
            "Redirigiendo de <code>/gitlab-mcp-server/es/capabilities/icons/</code> a "
            "<code>/gitlab-mcp-server/es/reference/icons/</code>",
            html,
        )
        self.assertNotIn("Redirecting", html)

    def test_every_target_the_site_serves_passes(self):
        for name, target in [
            ("site root", "/gitlab-mcp-server/"),
            ("page without its trailing slash", "/gitlab-mcp-server/reference/icons"),
            ("page with a fragment", "/gitlab-mcp-server/reference/icons/#what-icon-does-the-server-itself-carry"),
            ("page with a query", "/gitlab-mcp-server/reference/icons/?x=1"),
            ("file", "/gitlab-mcp-server/llms.txt"),
            ("another site", "https://github.com/jmrplens/gitlab-mcp-server/blob/main/test/e2e/README.md"),
        ]:
            with self.subTest(name=name):
                self.redirect("old", "/gitlab-mcp-server/old/", target)
                status, output = self.run_script()
                self.assertEqual(status, 0, output)

    def test_a_target_without_the_base_fails_naming_the_page(self):
        self.redirect("capabilities/icons", "/gitlab-mcp-server/capabilities/icons/", "/reference/icons/")
        status, output = self.run_script()
        self.assertEqual(status, 1, output)
        self.assertIn(
            "dist/capabilities/icons/index.html redirects to /reference/icons/, which is outside /gitlab-mcp-server/",
            output,
        )

    def test_a_target_the_site_does_not_serve_fails(self):
        # The third is a directory the build wrote with no page in it.
        for target in [
            "/gitlab-mcp-server/reference/gone/",
            "/gitlab-mcp-server/reference/icons/missing.txt",
            "/gitlab-mcp-server/reference/",
        ]:
            with self.subTest(target=target):
                self.redirect("old", "/gitlab-mcp-server/old/", target)
                status, output = self.run_script()
                self.assertEqual(status, 1, output)
                self.assertIn("dist/old/index.html redirects to " + target + ", a page this build does not hold", output)

    def test_every_broken_redirect_is_named(self):
        self.redirect("a", "/gitlab-mcp-server/a/", "/a/")
        self.redirect("b", "/gitlab-mcp-server/b/", "/gitlab-mcp-server/b-gone/")
        status, output = self.run_script()
        self.assertEqual(status, 1, output)
        self.assertIn("dist/a/index.html redirects to /a/", output)
        self.assertIn("dist/b/index.html redirects to /gitlab-mcp-server/b-gone/", output)

    def test_a_page_that_is_not_a_redirect_is_only_cleaned(self):
        path = os.path.join(self.dist, "guide", "index.html")
        write(path, "<!DOCTYPE html><p>a<br/></br>b</p>   \n<a href=\"/elsewhere/\">x</a>\t\n")
        status, output = self.run_script()
        self.assertEqual(status, 0, output)
        self.assertEqual(read(path), "<!DOCTYPE html><p>a<br>b</p>\n<a href=\"/elsewhere/\">x</a>\n")
        self.assertIn("normalised 0 redirect page(s)", output)


if __name__ == "__main__":
    unittest.main()
