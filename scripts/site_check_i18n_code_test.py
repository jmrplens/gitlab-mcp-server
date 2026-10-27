#!/usr/bin/env python3
# Copyright (c) 2026 José M. Requena Plens
# SPDX-License-Identifier: MIT

"""Tests how site/scripts/check-i18n-code.mjs holds a Spanish page's code blocks to English.

Issue 1110 decided that a code block on a Spanish page is the English block
byte for byte, placeholders included, and that only its comments and the
prose around it are translated, so the documented commands can be run from
either language. The gate runs inside `pnpm run i18n:check`. The site's own
pages only show that the blocks it carries today agree; they cannot show that
a translated placeholder would be reported, which is the half that fails
silently: a comment rule that swallows code, or a fence the reader stops
recognising, leaves the gate green with nothing compared.

Each case writes a stand-in site with an English page and its Spanish twin
and runs the real script in it.

Run with:

    python3 -m unittest discover -s scripts -p 'site_check_i18n_code_test.py'
"""

import os
import shutil
import subprocess
import tempfile
import textwrap
import unittest

ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
CHECK = os.path.join(ROOT, "site", "scripts", "check-i18n-code.mjs")

FENCE = "```"


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


def page(body, title="Page"):
    """A page whose body starts on line 6: frontmatter is lines 1-4, line 5 is blank."""
    return "---\ntitle: " + title + "\ndescription: A page.\n---\n\n" + textwrap.dedent(body).lstrip("\n")


def block(info, body):
    """A fenced block, its body dedented, as page text."""
    return FENCE + info + "\n" + textwrap.dedent(body).strip("\n") + "\n" + FENCE + "\n"


class CheckI18nCodeTest(unittest.TestCase):
    """site/scripts/check-i18n-code.mjs, run in a stand-in site."""

    def setUp(self):
        self.node = require_tool(self, "node")
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.site = os.path.join(self.tmp.name, "site")
        os.makedirs(os.path.join(self.site, "scripts"))
        shutil.copy(CHECK, os.path.join(self.site, "scripts", "check-i18n-code.mjs"))
        os.makedirs(os.path.join(self.site, "src", "content", "docs", "es"))

    def pair(self, en, es, rel="page.mdx"):
        write(os.path.join(self.site, "src", "content", "docs", rel), en)
        write(os.path.join(self.site, "src", "content", "docs", "es", rel), es)

    def blocks(self, info, en, es, es_info=None):
        """An English page holding one block and its twin holding the Spanish body."""
        self.pair(
            page("Intro.\n\n" + block(info, en)),
            page("Introducción.\n\n" + block(info if es_info is None else es_info, es)),
        )

    def check(self):
        run = subprocess.run(
            [self.node, os.path.join(self.site, "scripts", "check-i18n-code.mjs")],
            cwd=self.site,
            capture_output=True,
            text=True,
            check=False,
        )
        return run.returncode, run.stdout + run.stderr

    def assert_passes(self):
        status, output = self.check()
        self.assertEqual(status, 0, output)
        self.assertIn("i18n code blocks OK", output)
        return output

    def assert_refused(self, *reasons):
        status, output = self.check()
        self.assertEqual(status, 1, output)
        self.assertIn("i18n code block check FAILED", output)
        for reason in reasons:
            self.assertIn(reason, output)
        return output

    # What passes.

    def test_identical_blocks_pass_and_are_counted(self):
        self.blocks("bash", "gitlab-mcp-server --version\n", "gitlab-mcp-server --version\n")
        self.pair(page("No code.\n"), page("Sin código.\n"), rel="other/plain.md")
        output = self.assert_passes()
        self.assertIn("every code block of 2 Spanish pages", output)

    def test_the_spanish_tree_is_never_the_english_side_of_a_pair(self):
        # A page under es/ is a Spanish twin, so nothing below it is paired as
        # if it were English, not even a path that would find a twin of its own.
        self.blocks("bash", "make build\n", "make build\n")
        write(
            os.path.join(self.site, "src", "content", "docs", "es", "es", "page.mdx"),
            page(block("bash", "make test\n")),
        )
        output = self.assert_passes()
        self.assertIn("every code block of 1 Spanish pages", output)

    def test_translated_comments_pass_in_every_language_that_has_them(self):
        cases = [
            (
                "bash",
                """
                # Start the server
                gitlab-mcp-server --http  # listen on 8080
                """,
                """
                # Arranca el servidor,
                # en dos líneas
                gitlab-mcp-server --http    # escucha en 8080
                """,
            ),
            ("yaml", "image: gitlab-mcp-server  # the image\n", "image: gitlab-mcp-server # la imagen\n"),
            ("toml", '# Codex\ncommand = "gitlab-mcp-server"\n', '# Codex, en español\ncommand = "gitlab-mcp-server"\n'),
            ("nginx", "# Proxy\nlocation / { proxy_pass http://h; }\n", "# Proxy inverso\nlocation / { proxy_pass http://h; }\n"),
            ("powershell", "# Install\nwinget install x\n", "# Instalar\nwinget install x\n"),
            ("ini", "; unit\n[Service]\n# restart\nRestart=always\n", "; unidad\n[Service]\n# reinicio\nRestart=always\n"),
            (
                "typescript",
                "// the icon\nconst a = 1; // one\n/* a\n   block */ const b = 2;\n",
                "// el icono\nconst a = 1; // uno\n/* un\n   bloque\n   largo */ const b = 2;\n",
            ),
            ("xml", "<!-- the plist -->\n<plist>\n</plist>\n", "<!-- la\n     plist -->\n<plist>\n</plist>\n"),
            ("markdown", "## Projects\n<!-- a note -->\n", "## Projects\n<!-- una nota -->\n"),
        ]
        for lang, en, es in cases:
            with self.subTest(lang=lang):
                self.blocks(lang, en, es)
                self.assert_passes()

    def test_hash_inside_a_value_or_after_an_apostrophe_is_read_right(self):
        # `#` opens a comment only at the start of a word, an apostrophe in a
        # word opens no string, and a backslash escapes nothing inside single
        # quotes, so each of these trailing comments is a comment.
        for lang, en, es in [
            ("bash", 'curl https://h/#frag "${v#x}" $#   # don\'t do this\n', 'curl https://h/#frag "${v#x}" $#   # no hagas esto\n'),
            ("yaml", "title: It's ready  # the title\n", "title: It's ready  # el título\n"),
            ("bash", "echo 'C:\\' # a Windows path\n", "echo 'C:\\' # una ruta de Windows\n"),
        ]:
            with self.subTest(lang=lang, en=en):
                self.blocks(lang, en, es)
                self.assert_passes()

    def test_mermaid_labels_are_never_compared(self):
        self.blocks("mermaid", "flowchart LR\n  A[Client] --> B[Server]\n", "flowchart LR\n  A[Cliente] --> B[Servidor]\n")
        self.assert_passes()

    def test_prose_block_marked_translated_in_both_languages_passes(self):
        for lang in ["text", "markdown"]:
            with self.subTest(lang=lang):
                self.blocks(lang + " translated", "List my GitLab projects\n", "Lista mis proyectos de GitLab\n")
                self.assert_passes()

    def test_title_is_a_caption_and_may_be_translated(self):
        for en_info, es_info in [
            ('bash title="~/.bashrc or ~/.zshrc"', 'bash title="~/.bashrc o ~/.zshrc"'),
            ("bash title='~/.bashrc or ~/.zshrc'", "bash title='~/.bashrc o ~/.zshrc'"),
        ]:
            with self.subTest(en_info=en_info):
                self.blocks(en_info, "export A=1\n", "export A=1\n", es_info=es_info)
                self.assert_passes()

    def test_prose_block_may_translate_and_reorder_its_prose_around_the_code_it_quotes(self):
        # A sentence in quotes is prose, "Server suggests:" names no key, and a
        # translation may put the quoted code in another order.
        self.blocks(
            "text translated",
            """
            gitlab_find_action → query: "list open merge requests"
            → read gitlab://tools, then merge_request.list (2 calls)
            User types "mcp" → Server suggests: "group/gitlab-mcp-server"
            daily_standup(project_id="42") → your last 24 hours
            """,
            """
            gitlab_find_action → query: "listar merge requests abiertas"
            → leer merge_request.list después de gitlab://tools (2 llamadas)
            El usuario escribe "mcp" → El servidor sugiere: "group/gitlab-mcp-server"
            daily_standup(project_id="42") → tus últimas 24 horas
            """,
        )
        self.assert_passes()

    def test_comments_option_gives_a_language_without_comments_a_comment_syntax(self):
        for info, en, es in [
            ("text comments=hash", "frontend mcp\n  # The credential\n  bind *:443\n", "frontend mcp\n  # La credencial\n  bind *:443\n"),
            ("json comments=slash", '{\n  // the server\n  "a": 1\n}\n', '{\n  // el servidor\n  "a": 1\n}\n'),
        ]:
            with self.subTest(info=info):
                self.blocks(info, en, es)
                self.assert_passes()

    def test_indentation_of_the_fence_is_not_part_of_the_block(self):
        en = page("1. Run:\n\n   " + FENCE + "bash\n   make build\n     make test\n   " + FENCE + "\n")
        es = page('<TabItem label="Linux">\n\n    ' + FENCE + "bash\n    make build\n      make test\n    " + FENCE + "\n\n</TabItem>\n")
        self.pair(en, es)
        self.assert_passes()

    def test_a_longer_fence_holds_a_shorter_one_and_tildes_are_fences(self):
        inner = "````markdown\n```bash\nls\n```\n````\n\n~~~yaml\na: 1\n~~~\n"
        self.pair(page(inner), page(inner))
        self.assert_passes()
        es = "````markdown\n```bash\nls -la\n```\n````\n\n~~~yaml\na: 1\n~~~\n"
        self.pair(page(inner), page(es))
        self.assert_refused("es/page.mdx:6 (page.mdx:6): the markdown block differs from English outside comments")

    def test_frontmatter_is_not_read_for_fences(self):
        front_en = "---\ntitle: Page\ndescription: |\n  ```bash\n  a\n  ```\n---\n\n"
        front_es = "---\ntitle: Página\ndescription: |\n  ```bash\n  b\n  ```\n---\n\n"
        self.pair(front_en + "Text.\n", front_es + "Texto.\n")
        self.assert_passes()

    def test_a_page_without_frontmatter_is_read_from_its_first_line(self):
        # The thematic break further down is not the end of a frontmatter.
        self.pair("```bash\na\n```\n\n---\n\nText.\n", "```bash\nb\n```\n\n---\n\nTexto.\n")
        self.assert_refused("es/page.mdx:1 (page.mdx:1): the bash block differs from English outside comments")

    def test_a_backtick_in_the_info_string_opens_no_fence(self):
        # CommonMark: a backtick fence's info string carries no backtick, so
        # these lines are prose, translated like any other.
        self.pair(page("```x`y` is not a fence\n"), page("```x`y` no es una valla\n"))
        self.assert_passes()

    def test_a_fence_closes_only_on_its_own_character(self):
        self.blocks("text", "~~~\nA\n", "~~~\nB\n")
        self.assert_refused("the text block differs from English outside comments", "en 10: A", "es 10: B")

    def test_a_spanish_page_with_no_english_twin_is_left_to_the_parity_check(self):
        self.blocks("bash", "make\n", "make\n")
        write(os.path.join(self.site, "src", "content", "docs", "es", "orphan.mdx"), page(block("bash", "otra cosa\n")))
        write(os.path.join(self.site, "src", "content", "docs", "lonely.mdx"), page(block("bash", "make\n")))
        output = self.assert_passes()
        self.assertIn("every code block of 1 Spanish pages", output)

    # What is refused.

    def test_translated_placeholder_is_refused_with_both_lines(self):
        self.pair(
            page(
                """
                Intro.

                ```json title="claude_desktop_config.json"
                {
                  "command": "/path/to/gitlab-mcp-server",
                  "env": { "GITLAB_TOKEN": "glpat-your-token" }
                }
                ```
                """
            ),
            page(
                """
                Introducción, que ocupa
                dos líneas.

                ```json title="claude_desktop_config.json"
                {
                  "command": "/ruta/a/gitlab-mcp-server",
                  "env": { "GITLAB_TOKEN": "glpat-tu-token" }
                }
                ```
                """
            ),
        )
        output = self.assert_refused(
            "es/page.mdx:9 (page.mdx:8): the json block differs from English outside comments (and 1 more differing line)",
            'en 10:   "command": "/path/to/gitlab-mcp-server",',
            'es 11:   "command": "/ruta/a/gitlab-mcp-server",',
        )
        self.assertEqual(output.count("differs from English"), 1, output)

    def test_text_inside_quotes_or_after_an_escaped_hash_is_code(self):
        for en, es in [
            ('echo "the server # exited"\n', 'echo "el servidor # terminó"\n'),
            ("echo 'a # b'\n", "echo 'a # c'\n"),
            ('echo "say \\"hi # there\\""\n', 'echo "say \\"hi # allí\\""\n'),
            ("echo a\\ #b\n", "echo a\\ #c\n"),
            ('--prompt="Summarize # the week"\n', '--prompt="Summarize # la semana"\n'),
            ("url=http://h/#a\n", "url=http://h/#b\n"),
            ("echo $'a # b'\n", "echo $'a # c'\n"),
            ('echo $"a # b"\n', 'echo $"a # c"\n'),
        ]:
            with self.subTest(en=en):
                self.blocks("bash", en, es)
                self.assert_refused("the bash block differs from English outside comments")

    def test_a_language_without_comments_is_compared_byte_for_byte(self):
        for lang in ["json", "http", "text", "unknownlang"]:
            with self.subTest(lang=lang):
                self.blocks(lang, "# a\nGET / HTTP/1.1\n", "# b\nGET / HTTP/1.1\n")
                self.assert_refused("the " + lang + " block differs from English outside comments")

    def test_a_comment_rule_does_not_reach_past_its_own_syntax(self):
        cases = [
            ("ini", "key=a ; b\n", "key=a ; c\n"),
            ("typescript", 'const u = "http://h//x";\n', 'const u = "http://h//y";\n'),
            ("typescript", "const s = `a /* b */`;\n", "const s = `a /* c */`;\n"),
            ("typescript", "const s = 'it\\'s // a';\n", "const s = 'it\\'s // b';\n"),
            ("xml", "<a>x</a>\n", "<a>y</a>\n"),
            ("xml", "<!-- c --> <a>x</a>\n", "<!-- c --> <a>y</a>\n"),
            ("xml", "<a>x</a> <!-- c -->\n", "<a>y</a> <!-- c -->\n"),
        ]
        for lang, en, es in cases:
            with self.subTest(lang=lang, en=en):
                self.blocks(lang, en, es)
                self.assert_refused("the " + lang + " block differs from English outside comments")

    def test_a_blank_line_is_code(self):
        self.blocks("bash", "a\n\nb\n", "a\nb\n")
        self.assert_refused("the bash block differs from English outside comments", "en 10: ", "es 10: b")

    def test_every_other_differing_line_is_counted_when_the_lines_align(self):
        self.blocks("bash", "a\nb\nc\nd\n", "x\nb\ny\nz\n")
        self.assert_refused("the bash block differs from English outside comments (and 2 more differing lines)")

    def test_a_line_only_one_side_carries_is_refused(self):
        self.blocks("bash", "make\nmake test\n", "make\n")
        self.assert_refused("es/page.mdx:8 (page.mdx:8): the bash block differs", "en 10: make test", "es (the block ends)")
        self.blocks("bash", "make\n", "make\nmake test\n")
        self.assert_refused("en (the block ends)", "es 10: make test")

    def test_prose_block_without_the_marker_is_refused(self):
        self.blocks("text", "List my GitLab projects\n", "Lista mis proyectos de GitLab\n")
        self.assert_refused(
            "es/page.mdx:8 (page.mdx:8): the text block differs from English outside comments",
            "en 9: List my GitLab projects",
            "es 9: Lista mis proyectos de GitLab",
        )

    def test_marker_on_one_side_only_is_refused(self):
        self.blocks("text translated", "List\n", "Lista\n", es_info="text")
        self.assert_refused("es/page.mdx:8 (page.mdx:8): the fence options differ", "en: translated", "es: (none)")

    def test_prose_block_that_changes_the_code_it_quotes_is_refused(self):
        en = (
            'job.list → project_id: "42", state: "opened", job_id: 500\n'
            '  ref: "main"\n'
            "Read gitlab://tools/job.list, which names merge_request_iid\n"
        )
        for name, es, only_en, only_es in [
            ("action ID", en.replace("job.list →", "job.listar →"), "job.list", "job.listar"),
            ("key at the start of a line", en.replace("  ref:", "  rama:"), "ref", "rama"),
            ("name in prose", en.replace("names merge_request_iid", "nombra iid_de_merge_request"), "merge_request_iid", "iid_de_merge_request"),
            ("one of two", en.replace("job.list →", "job.list, job.list →"), "(none)", "job.list"),
            ("key", en.replace("state:", "estado:"), "state", "estado"),
            ("value dropped", en.replace(', job_id: 500', ''), "job_id 500", "(none)"),
            ("quoted value", en.replace('"opened"', '"abierto"'), '"opened"', '"abierto"'),
            ("number", en.replace("500", "501"), "500", "501"),
            ("URI", en.replace("gitlab://tools/job.list", "gitlab://tools/job.lista"), "gitlab://tools/job.list", "gitlab://tools/job.lista"),
        ]:
            with self.subTest(name=name):
                self.blocks("text translated", en, es)
                self.assert_refused(
                    "es/page.mdx:8 (page.mdx:8): the text block marked translated quotes code its twin does not",
                    "    en: " + only_en,
                    "    es: " + only_es,
                )

    def test_comments_option_must_name_a_comment_syntax(self):
        self.blocks("text comments=semicolons", "a\n", "a\n")
        self.assert_refused(
            "es/page.mdx:8 (page.mdx:8): comments=semicolons names no comment syntax; use hash, ini, slash or markup"
        )

    def test_comments_option_still_compares_the_code(self):
        self.blocks("text comments=hash", "bind *:443  # tls\n", "bind *:8443  # tls\n")
        self.assert_refused("the text block differs from English outside comments", "en 9: bind *:443", "es 9: bind *:8443")

    def test_marker_that_excuses_nothing_is_refused(self):
        self.blocks("text translated", "gitlab://tools\n", "gitlab://tools\n")
        self.assert_refused("marked translated and identical to its twin; drop the marker from both fences")

    def test_marker_outside_a_prose_language_is_refused(self):
        for lang in ["bash", "json", "mermaid"]:
            for es in ["b\n", "a\n"]:
                with self.subTest(lang=lang, es=es):
                    self.blocks(lang + " translated", "a\n", es)
                    output = self.assert_refused(
                        "a " + lang + " block cannot be marked translated; only text and markdown blocks hold prose"
                    )
                    # One finding is the whole verdict on such a block.
                    self.assertNotIn("identical to its twin", output)

    def test_a_title_naming_comments_is_a_caption(self):
        self.blocks('text title="comments=hash"', "a\n", "a\n", es_info='text title="comments=ninguno"')
        self.assert_passes()

    def test_other_fence_options_must_agree(self):
        self.blocks('bash {2} title="x"', "a\nb\n", "a\nb\n", es_info='bash {1} title="y"')
        self.assert_refused("the fence options differ", "en: {2}", "es: {1}")

    def test_language_must_agree(self):
        self.blocks("bash", "make\n", "make\n", es_info="sh")
        self.assert_refused("es/page.mdx:8 (page.mdx:8): a sh block where English has bash")
        self.blocks("bash", "make\n", "make\n", es_info="")
        self.assert_refused("a plain block where English has bash")

    def test_different_number_of_blocks_is_refused_without_pairing(self):
        self.pair(
            page(block("bash", "a\n") + "\n" + block("bash", "b\n")),
            page(block("bash", "z\n")),
            rel="guide/x.mdx",
        )
        output = self.assert_refused(
            "es/guide/x.mdx: 1 code blocks, guide/x.mdx has 2; blocks pair by their order"
        )
        self.assertNotIn("differs from English", output)
        # And the other way round: a block only the Spanish page carries.
        self.pair(
            page(block("bash", "a\n")),
            page(block("bash", "a\n") + "\n" + block("bash", "rm -rf /\n")),
            rel="guide/x.mdx",
        )
        self.assert_refused("es/guide/x.mdx: 2 code blocks, guide/x.mdx has 1; blocks pair by their order")

    def test_an_unclosed_fence_runs_to_the_end_of_the_page(self):
        self.pair(page("```bash\nmake\n\nMore text.\n"), page("```bash\nmake\n\nMás texto.\n"))
        self.assert_refused("the bash block differs from English outside comments", "en 9: More text.")

    def test_every_differing_page_is_reported_in_path_order(self):
        self.pair(page(block("yaml", "a: 1\n")), page(block("yaml", "a: 2\n")), rel="two/three.md")
        self.pair(page(block("bash", "b\n")), page(block("bash", "c\n")), rel="zeta.mdx")
        self.pair(page(block("bash", "a\n")), page(block("bash", "b\n")), rel="one.mdx")
        output = self.assert_refused("es/one.mdx:6 (one.mdx:6)", "es/two/three.md:6 (two/three.md:6)", "es/zeta.mdx:6")
        self.assertLess(output.index("es/one.mdx"), output.index("es/two/three.md"))
        self.assertLess(output.index("es/two/three.md"), output.index("es/zeta.mdx"))


if __name__ == "__main__":
    unittest.main()
