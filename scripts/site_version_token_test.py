#!/usr/bin/env python3
"""Tests how the documentation site names the current release.

A page writes %%VERSION%% where it means the release you install today, and
the build replaces it with the published release: the top-level version of
the repository's server.json, which the release workflow stamps once the
release is out. VERSION is not read, because it names the release being
prepared. Three pieces make that hold, and each is driven here through the
real file:

- site/src/lib/version-token.mjs, the one reader of the published release and
  the remark plugin the build runs first. Its tests hand it a Markdown tree carrying the
  token in every place a page can hold one (prose, inline code, a fenced
  block and its meta, a table cell, link and image URLs, an MDX attribute,
  string and template literals of an MDX expression) and read the tree back.
  The build itself only shows that the pages this repository has today come
  out right; it cannot show that a node type no page uses yet would.
- site/scripts/check-version-token.mjs, the gate. Its tests write a stand-in
  site (a server.json, a few pages, a dist/ tree and a declarations table)
  and run the real script in it, because the half that fails silently is the
  refusal: a shape that stops matching, or a dist walk that stops reading,
  leaves the gate green with nothing checked.
- site/scripts/gen-llms.mjs, which reads title and description verbatim and so
  has to refuse the token there before it writes the site's llms index.

Run with:

    python3 -m unittest discover -s scripts -p 'site_version_token_test.py'
"""

import json
import os
import shutil
import subprocess
import tempfile
import textwrap
import unittest

ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
LIB = os.path.join(ROOT, "site", "src", "lib", "version-token.mjs")
CHECK = os.path.join(ROOT, "site", "scripts", "check-version-token.mjs")
GEN_LLMS = os.path.join(ROOT, "site", "scripts", "gen-llms.mjs")

TOKEN = "%%VERSION%%"


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


def page(body, front="title: Page\ndescription: A page."):
    return "---\n" + front + "\n---\n\n" + body + "\n"


def manifest(version):
    """A server.json carrying version at its top level, beside a package entry that names another."""
    return json.dumps(
        {
            "name": "io.github.example/server",
            "version": version,
            "packages": [{"registryType": "npm", "identifier": "pkg", "version": "0.0.1"}],
        },
        indent=2,
    ) + "\n"


class VersionTokenModuleTest(unittest.TestCase):
    """The reader of the published release and the remark plugin, through Node."""

    def setUp(self):
        self.node = require_tool(self, "node")

    def run_module(self, body):
        """Runs an ES module that imports the library as `lib`, returning (status, stdout, stderr)."""
        source = 'import * as lib from "' + "file://" + LIB + '";\n' + textwrap.dedent(body)
        run = subprocess.run(
            [self.node, "--input-type=module", "-e", source],
            capture_output=True,
            text=True,
            check=False,
        )
        return run.returncode, run.stdout, run.stderr

    def test_plugin_replaces_the_token_wherever_a_page_can_hold_it(self):
        status, out, err = self.run_module(
            """
            const t = "%%VERSION%%";
            const tree = { type: "root", children: [
              { type: "paragraph", children: [
                { type: "text", value: "the current release is " + t + "." },
                { type: "inlineCode", value: "pkg@" + t },
                { type: "link", url: "https://h/releases/download/v" + t + "/a", title: "v" + t,
                  children: [{ type: "text", value: "release " + t }] },
                { type: "image", url: "/shot-" + t + ".png", alt: "shot " + t },
              ] },
              { type: "code", lang: "bash", meta: 'title="install ' + t + '"', value: "VERSION=v" + t + " sh" },
              { type: "table", children: [{ type: "tableRow", children: [
                { type: "tableCell", children: [{ type: "text", value: "Stable, v" + t }] } ] }] },
              { type: "definition", identifier: "r", url: "https://h/tag/v" + t },
              { type: "mdxJsxFlowElement", name: "Card", children: [], attributes: [
                { type: "mdxJsxAttribute", name: "title", value: "Release " + t },
                { type: "mdxJsxAttribute", name: "note", value: {
                  type: "mdxJsxAttributeValueExpression", value: '"' + t + '"',
                  data: { estree: { type: "Program", body: [{ type: "ExpressionStatement",
                    expression: { type: "Literal", value: t, raw: '"' + t + '"' } }] } } } },
              ] },
              { type: "mdxFlowExpression", value: "`v" + t + "`", data: { estree: { type: "Program", body: [
                { type: "ExpressionStatement", expression: { type: "TemplateLiteral", expressions: [],
                  quasis: [{ type: "TemplateElement", tail: true, value: { raw: "v" + t, cooked: "v" + t } }] } } ] } } },
              { type: "text", value: "removed in 3.0.0", position: { start: { line: 9, column: 1, offset: 80 } } },
            ] };
            lib.remarkVersionToken({ version: "9.8.7" })(tree);
            console.log(JSON.stringify(tree));
            """
        )
        self.assertEqual(status, 0, err)
        self.assertNotIn(TOKEN, out)
        tree = json.loads(out)
        paragraph, code, table, definition, card, expression, historical = tree["children"]
        text, inline, link, image = paragraph["children"]
        self.assertEqual(text["value"], "the current release is 9.8.7.")
        self.assertEqual(inline["value"], "pkg@9.8.7")
        self.assertEqual(link["url"], "https://h/releases/download/v9.8.7/a")
        self.assertEqual(link["title"], "v9.8.7")
        self.assertEqual(link["children"][0]["value"], "release 9.8.7")
        self.assertEqual(image["url"], "/shot-9.8.7.png")
        self.assertEqual(image["alt"], "shot 9.8.7")
        self.assertEqual(code["meta"], 'title="install 9.8.7"')
        self.assertEqual(code["value"], "VERSION=v9.8.7 sh")
        self.assertEqual(table["children"][0]["children"][0]["children"][0]["value"], "Stable, v9.8.7")
        self.assertEqual(definition["url"], "https://h/tag/v9.8.7")
        title, note = card["attributes"]
        self.assertEqual(title["value"], "Release 9.8.7")
        literal = note["value"]["data"]["estree"]["body"][0]["expression"]
        self.assertEqual((literal["value"], literal["raw"]), ("9.8.7", '"9.8.7"'))
        quasi = expression["data"]["estree"]["body"][0]["expression"]["quasis"][0]["value"]
        self.assertEqual((quasi["raw"], quasi["cooked"]), ("v9.8.7", "v9.8.7"))
        self.assertEqual(historical["value"], "removed in 3.0.0")
        self.assertEqual(historical["position"]["start"]["line"], 9)

    def test_substitution_counts_what_it_changed_and_survives_a_cycle(self):
        status, out, err = self.run_module(
            """
            const node = { type: "text", value: "a %%VERSION%% b %%VERSION%%", other: "no token" };
            const tree = { type: "root", children: [node, node] };
            node.parent = tree;
            const changed = lib.substituteVersionToken(tree, "1.2.3");
            console.log(JSON.stringify({ changed, value: node.value, other: node.other }));
            """
        )
        self.assertEqual(status, 0, err)
        self.assertEqual(json.loads(out), {"changed": 1, "value": "a 1.2.3 b 1.2.3", "other": "no token"})

    def test_read_release_takes_the_top_level_version_of_server_json(self):
        for version in ["3.2.0", "3.2.0-rc.1", "10.0.0+build.7"]:
            with self.subTest(version=version), tempfile.TemporaryDirectory() as tmp:
                path = os.path.join(tmp, "server.json")
                write(path, manifest(version))
                status, out, err = self.run_module("console.log(lib.readRelease(" + json.dumps(path) + "));")
                self.assertEqual(status, 0, err)
                self.assertEqual(out.strip(), version)

    def test_read_release_refuses_a_version_that_is_not_a_release(self):
        for version in [None, "", " 3.2.0", "v3.2.0", "3.2", "3.2.0\n", "%%VERSION%%", 3, ["3.2.0"]]:
            with self.subTest(version=version), tempfile.TemporaryDirectory() as tmp:
                path = os.path.join(tmp, "server.json")
                content = json.loads(manifest("0.0.0"))
                if version is None:
                    del content["version"]
                else:
                    content["version"] = version
                write(path, json.dumps(content))
                status, _, err = self.run_module("lib.readRelease(" + json.dumps(path) + ");")
                self.assertNotEqual(status, 0, "accepted " + repr(version))
                self.assertIn("which is not a release", err)
                self.assertIn(path, err)

    def test_read_release_refuses_a_file_that_is_not_json(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "server.json")
            write(path, "3.2.0\n")
            status, _, err = self.run_module("lib.readRelease(" + json.dumps(path) + ");")
            self.assertNotEqual(status, 0)
            self.assertIn(path + " is not JSON", err)

    def test_read_release_names_a_missing_file(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "server.json")
            status, _, err = self.run_module("lib.readRelease(" + json.dumps(path) + ");")
            self.assertNotEqual(status, 0)
            self.assertIn("cannot read " + path, err)

    def test_plugin_without_options_reads_the_published_release(self):
        with open(os.path.join(ROOT, "server.json"), encoding="utf-8") as fh:
            want = json.load(fh)["version"]
        status, out, err = self.run_module(
            """
            const tree = { type: "root", children: [{ type: "text", value: "%%VERSION%%" }] };
            lib.remarkVersionToken()(tree);
            console.log(tree.children[0].value);
            """
        )
        self.assertEqual(status, 0, err)
        self.assertEqual(out.strip(), want)


class CheckVersionTokenTest(unittest.TestCase):
    """site/scripts/check-version-token.mjs, run in a stand-in site."""

    def setUp(self):
        self.node = require_tool(self, "node")
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = self.tmp.name
        self.site = os.path.join(self.root, "site")
        shutil.copy(LIB, self.lib_path())
        os.makedirs(os.path.join(self.site, "scripts"), exist_ok=True)
        shutil.copy(CHECK, os.path.join(self.site, "scripts", "check-version-token.mjs"))
        write(os.path.join(self.root, "server.json"), manifest("1.2.3"))
        # The release being prepared, which the gate must not read.
        write(os.path.join(self.root, "VERSION"), "1.3.0\n")
        self.declare([])

    def lib_path(self):
        path = os.path.join(self.site, "src", "lib", "version-token.mjs")
        os.makedirs(os.path.dirname(path), exist_ok=True)
        return path

    def declare(self, declarations):
        write(
            os.path.join(self.site, "scripts", "version-literals.mjs"),
            "export const LITERAL_RELEASES = " + json.dumps(declarations) + ";\n",
        )

    def page(self, rel, content):
        write(os.path.join(self.site, "src", "content", "docs", rel), content)

    def dist(self, rel, content):
        write(os.path.join(self.site, "dist", rel), content)

    def check(self, *args):
        run = subprocess.run(
            [self.node, os.path.join(self.site, "scripts", "check-version-token.mjs"), *args],
            cwd=self.site,
            capture_output=True,
            text=True,
            check=False,
        )
        return run.returncode, run.stdout + run.stderr

    def assert_passes(self, *args):
        status, output = self.check(*args)
        self.assertEqual(status, 0, output)
        return output

    def assert_refused(self, *reasons, args=()):
        status, output = self.check(*args)
        self.assertEqual(status, 1, output)
        for reason in reasons:
            self.assertIn(reason, output)
        return output

    def test_token_in_the_body_passes(self):
        self.page("install.mdx", page("Run `pkg@%%VERSION%%`; the current release is %%VERSION%%."))
        self.page("es/install.mdx", page("Ejecuta `pkg@%%VERSION%%`; la release actual es la %%VERSION%%."))
        self.assert_passes()

    def test_token_in_frontmatter_is_refused(self):
        for front in [
            "title: Install %%VERSION%%\ndescription: A page.",
            "title: Page\ndescription: Install %%VERSION%% today.",
            'title: Page\ndescription: A page.\nfaq:\n  - q: "Which release?"\n    a: "%%VERSION%%"',
        ]:
            with self.subTest(front=front):
                self.page("install.mdx", page("Body.", front))
                self.assert_refused("install.mdx:", "frontmatter carries %%VERSION%%")

    def test_literal_current_release_shapes_are_refused(self):
        for line, shape in [
            ("curl -O https://h/jmrplens/gitlab-mcp-server/releases/download/v1.2.3/bin", "download URL"),
            ('--certificate-identity "https://h/release.yml@refs/tags/v1.2.3"', "tag ref"),
            ("docker pull ghcr.io/jmrplens/gitlab-mcp-server:1.2.3", "image tag"),
            ('`"@jmrp.io/gitlab-mcp-server@1.0.0"`', "package pin"),
            ("pip install jmrplens-gitlab-mcp-server==1.2.3", "package pin"),
            ("# gitlab-mcp-server 1.2.3 (commit: ...)", "--version output"),
            ("curl -fsSL https://h/install.sh | VERSION=v1.2.3 sh", "installer pin"),
            ("$env:VERSION = 'v1.2.3'", "installer pin"),
            ("winget install jmrplens.gitlab-mcp-server --version 1.2.3", "--version pin"),
            ("dotnet tool install -g gitlab-mcp-server --version=1.2.3", "--version pin"),
            ('  "version": "1.2.3",', "server output"),
            ('  "build": "1.2.3+abcdef0",', "server output"),
            ("The current release is **v1.2.3**.", "current release"),
            ("Current version: 1.2.3.", "current release"),
            ("while the latest release is 1.2.3.", "current release"),
            ("El release actual es la **v1.2.3**.", "current release"),
            ("Versión actual: 1.2.3.", "current release"),
            ("mientras la última release sea la 1.2.3.", "current release"),
        ]:
            with self.subTest(line=line):
                self.page("install.mdx", page("Intro.\n\n" + line))
                self.assert_refused("install.mdx:8: " + shape + " with a literal release")

    def test_history_is_not_refused(self):
        self.page(
            "install.mdx",
            page(
                "The old spellings were removed in 1.0.0. From the first release after 1.2.3 a bundle\n"
                "per system ships; up to 1.2.3 there is one. It was checked on 1.2.3 against the\n"
                "release, and 1.1.0 stopped the process with a panic. Go 1.27.2 and GitLab 19.4.1 too.\n"
                "Use the latest version of Node, 24.18.0 or newer; la versión actual de GitLab, 19.4.1.\n"
                "Run `gitlab-mcp-server --version`; `node --version` prints 24.18.0."
            ),
        )
        self.assert_passes()

    def test_declared_literal_passes_and_excuses_only_its_own_line(self):
        self.declare([{"page": "install.mdx", "text": "`dnx gitlab-mcp-server@1.0.0`", "category": "measurement", "reason": "measured"}])
        self.page("install.mdx", page("Measured with `dnx gitlab-mcp-server@1.0.0` cached."))
        self.assert_passes()
        self.page("install.mdx", page('Measured with `dnx gitlab-mcp-server@1.0.0` cached.\n\nPin `"@jmrp.io/gitlab-mcp-server@1.0.0"`.'))
        self.assert_refused("install.mdx:8: package pin with a literal release")
        self.page("es/install.mdx", page("Medido con `dnx gitlab-mcp-server@1.0.0` en caché."))
        self.page("install.mdx", page("Measured with `dnx gitlab-mcp-server@1.0.0` cached."))
        self.assert_refused("es/install.mdx:6: package pin with a literal release")

    def test_declaration_that_matches_nothing_is_refused(self):
        self.declare([{"page": "install.mdx", "text": "`dnx gitlab-mcp-server@1.0.0`", "category": "measurement", "reason": "measured"}])
        self.page("install.mdx", page("The release is %%VERSION%%."))
        self.assert_refused("the declaration for install.mdx", "matches no literal release on that page")

    def test_declaration_without_a_category_or_reason_is_refused(self):
        self.page("install.mdx", page("Measured with `dnx gitlab-mcp-server@1.0.0` cached."))
        good = {"page": "install.mdx", "text": "`dnx gitlab-mcp-server@1.0.0`", "category": "measurement", "reason": "measured"}
        for change, reason in [
            ({"reason": ""}, "has no reason"),
            ({"reason": "   "}, "has no reason"),
            ({"category": "history"}, 'has category "history"'),
            ({"category": None}, "has category null"),
        ]:
            with self.subTest(change=change):
                self.declare([{**good, **change}])
                self.assert_refused("the declaration for install.mdx", reason)
        declaration = dict(good)
        del declaration["category"]
        self.declare([declaration])
        self.assert_refused("has category undefined")

    def test_condition_holds_until_the_published_release_moves_past_it(self):
        self.page("start.mdx", page("Download the universal bundle while the latest release is 1.2.3."))
        self.declare([{"page": "start.mdx", "text": "latest release is 1.2.3", "category": "condition", "reason": "per-OS bundles start after it"}])
        self.assert_passes()
        # VERSION has moved on to the release being prepared and changes nothing;
        # the published release moving on is what ends the condition.
        write(os.path.join(self.root, "VERSION"), "9.0.0\n")
        self.assert_passes()
        write(os.path.join(self.root, "server.json"), manifest("1.2.4"))
        self.assert_refused(
            "is a condition about 1.2.3 being the latest release, and the published release in server.json is 1.2.4"
        )

    def test_measurement_stays_whatever_the_published_release(self):
        self.page("install.mdx", page("Measured with `dnx gitlab-mcp-server@1.0.0` cached."))
        self.declare([{"page": "install.mdx", "text": "`dnx gitlab-mcp-server@1.0.0`", "category": "measurement", "reason": "measured"}])
        write(os.path.join(self.root, "server.json"), manifest("4.0.0"))
        self.assert_passes()

    def test_stats_version_is_refused(self):
        self.page("about.mdx", page("Current version: **{stats.version}**."))
        self.assert_refused("about.mdx:6: {stats.version}")

    def test_dist_without_the_token_and_with_the_release_passes(self):
        self.page("install/npm.mdx", page("Run `pkg@%%VERSION%%`; up to 1.2.3 it was `pkg@1.2.3`."))
        self.page("index.mdx", page("Home."))
        # The copy button repeats a code block in data-code, so a page may carry
        # the release more often than it wrote it, never less.
        self.dist(
            "install/npm/index.html",
            '<head><script type="application/ld+json">{"softwareVersion":"1.2.3"}</script></head>'
            '<main><p>Run <code>pkg@1.2.3</code>; up to 1.2.3 it was <code>pkg@1.2.3</code>.</p>'
            '<pre data-code="pkg@1.2.3"><code>pkg@1.2.3</code></pre></main>',
        )
        self.dist("index.html", "<main><p>Home.</p></main>")
        self.dist("llms.txt", "index (version 1.2.3)\n")
        self.assertIn("carries no %%VERSION%%", self.assert_passes("--dist"))

    def test_token_left_in_dist_is_refused_wherever_it_is(self):
        self.page("index.mdx", page("Home."))
        for rel in ["index.html", "llms.txt", "es/llms.txt", "_astro/page.js", "sitemap-0.xml"]:
            with self.subTest(rel=rel):
                shutil.rmtree(os.path.join(self.site, "dist"), ignore_errors=True)
                self.dist("index.html", "<p>Home.</p>")
                self.dist(rel, "before %%VERSION%% after")
                self.assert_refused("dist/" + rel + ": carries %%VERSION%%", args=("--dist",))

    def test_page_whose_release_did_not_reach_its_html_is_refused(self):
        self.page("install/npm.mdx", page("Run `pkg@%%VERSION%%`."))
        self.page("es/index.mdx", page("Release %%VERSION%%."))
        self.dist("install/npm/index.html", "<main><p>Run <code>pkg@</code>.</p></main>")
        self.dist("es/index.html", "<main><p>Release 1.2.3.</p></main>")
        output = self.assert_refused(
            "dist/install/npm/index.html: carries 1.2.3 0 time(s) in <main>, fewer than the 1 install/npm.mdx writes",
            args=("--dist",),
        )
        self.assertNotIn("es/index.html", output)
        os.remove(os.path.join(self.site, "dist", "install", "npm", "index.html"))
        self.assert_refused("install/npm.mdx: writes %%VERSION%%, and dist/install/npm/index.html was not built", args=("--dist",))

    def test_release_outside_main_does_not_stand_for_the_token(self):
        # Every built page prints the release in the JSON-LD of its <head>, so a
        # page whose token became nothing still holds the number somewhere.
        self.page("install/npm.mdx", page("Run `pkg@%%VERSION%%`."))
        self.dist(
            "install/npm/index.html",
            '<head><script type="application/ld+json">{"softwareVersion":"1.2.3"}</script></head>'
            "<main><p>Run <code>pkg@</code>.</p></main>",
        )
        self.assert_refused("dist/install/npm/index.html: carries 1.2.3 0 time(s) in <main>", args=("--dist",))

    def test_history_written_as_the_release_does_not_stand_for_the_token(self):
        # A page may write the published release literally as history; the token
        # beside it still has to become the release too.
        self.page("install/npm.mdx", page("Run `pkg@%%VERSION%%`; removed in 1.2.3."))
        self.dist("install/npm/index.html", "<main><p>Run <code>pkg@</code>; removed in 1.2.3.</p></main>")
        self.assert_refused("carries 1.2.3 1 time(s) in <main>, fewer than the 2 install/npm.mdx writes", args=("--dist",))

    def test_built_page_without_main_is_refused(self):
        self.page("install/npm.mdx", page("Run `pkg@%%VERSION%%`."))
        self.dist("install/npm/index.html", "<p>Run <code>pkg@1.2.3</code>.</p>")
        self.assert_refused("dist/install/npm/index.html: has no <main> element", args=("--dist",))

    def test_dist_mode_without_a_build_is_refused(self):
        self.page("index.mdx", page("Home."))
        self.assert_refused("dist/ does not exist", args=("--dist",))

    def test_default_mode_reads_dist_when_there_is_one(self):
        self.page("index.mdx", page("Home."))
        self.assert_passes()
        self.dist("index.html", "<p>%%VERSION%%</p>")
        self.assert_refused("dist/index.html: carries %%VERSION%%")

    def test_unreadable_release_stops_the_dist_check(self):
        self.page("index.mdx", page("Release %%VERSION%%."))
        self.dist("index.html", "<main><p>Release v1.2.3.</p></main>")
        write(os.path.join(self.root, "server.json"), manifest("v1.2.3"))
        self.assert_refused("which is not a release", args=("--dist",))

    def test_dist_is_held_to_the_published_release_not_to_version(self):
        # VERSION names the release being prepared (1.3.0 here); a build that
        # printed it would advertise a release nobody can download yet.
        self.page("install/npm.mdx", page("Run `pkg@%%VERSION%%`."))
        self.dist("install/npm/index.html", "<main><p>Run <code>pkg@1.3.0</code>.</p></main>")
        self.assert_refused("carries 1.2.3 0 time(s) in <main>", args=("--dist",))


class GenLlmsTokenTest(unittest.TestCase):
    """gen-llms.mjs copies title and description verbatim, so it refuses the token there."""

    def setUp(self):
        self.node = require_tool(self, "node")

    def test_token_in_title_or_description_stops_the_index(self):
        for key in ["title", "description"]:
            with self.subTest(key=key), tempfile.TemporaryDirectory() as root:
                site = os.path.join(root, "site")
                os.makedirs(os.path.join(site, "src", "lib"))
                os.makedirs(os.path.join(site, "scripts"))
                shutil.copy(LIB, os.path.join(site, "src", "lib", "version-token.mjs"))
                shutil.copy(GEN_LLMS, os.path.join(site, "scripts", "gen-llms.mjs"))
                write(os.path.join(root, "server.json"), manifest("1.2.3"))
                front = "title: Home\ndescription: The home page."
                front = front.replace("Home" if key == "title" else "The home page", "Release %%VERSION%%")
                write(os.path.join(site, "src", "content", "docs", "index.mdx"), page("Body.", front))
                run = subprocess.run(
                    [self.node, os.path.join(site, "scripts", "gen-llms.mjs"), "--check"],
                    capture_output=True,
                    text=True,
                    check=False,
                )
                self.assertEqual(run.returncode, 1, run.stdout + run.stderr)
                self.assertIn("frontmatter " + key + " carries %%VERSION%%", run.stderr)


if __name__ == "__main__":
    unittest.main()
