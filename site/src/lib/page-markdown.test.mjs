// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Unit tests for the markdown twin reducer (src/lib/page-markdown.mjs).
// Run with `pnpm run test` (node --test), which the site lint runs too.
import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { after, before, describe, test } from "node:test";

import { renderHome } from "./home-markdown.mjs";
import {
	evaluate,
	inlineHtml,
	reduceBody,
	renderTwin,
	routeOf,
} from "./page-markdown.mjs";

let root = "";

// A site directory of its own, holding the data files and the raw import the
// pages under test read, so the tests do not depend on the real figures.
before(() => {
	root = mkdtempSync(join(tmpdir(), "page-markdown-"));
	mkdirSync(join(root, "src/content/docs/es"), { recursive: true });
	mkdirSync(join(root, "src/data"), { recursive: true });
	mkdirSync(join(root, "examples"), { recursive: true });
	writeFileSync(
		join(root, "src/data/stats.json"),
		JSON.stringify({
			tools: { free: 875, gitlab_com: 1102 },
			meta: { base: 34, premium: 40 },
		}),
	);
	writeFileSync(join(root, "examples/sample.yaml"), "key: value\n\nother: 1\n");
});

after(() => rmSync(root, { recursive: true, force: true }));

const LABELS = { "gm.fact.exposes": "What it exposes" };

/** Reduces a body as the page `route` would be. */
const reduce = (body, overrides = {}) => {
	const route = overrides.route ?? "guide";
	return reduceBody({
		body,
		file: `src/content/docs/${route || "index"}.mdx`,
		route,
		root,
		release: "9.8.7",
		labels: LABELS,
		stats: { tools: { free: 875 } },
		...overrides,
	});
};

describe("routeOf", () => {
	test("maps collection ids to routes", () => {
		assert.equal(routeOf("index"), "");
		assert.equal(routeOf("es/index"), "es");
		assert.equal(routeOf("about.mdx"), "about");
		assert.equal(routeOf("reference/tools/index"), "reference/tools");
	});
});

describe("components", () => {
	test("an unknown wrapping component fails the reduction", () => {
		assert.throws(
			() => reduce("<Mystery>text</Mystery>"),
			/no markdown reduction for <Mystery>/,
		);
	});

	test("an unknown self-closing component fails the reduction", () => {
		assert.throws(
			() => reduce("<Widget />"),
			/no markdown reduction for <Widget \/>/,
		);
	});

	test("tabs become a labelled list, nested sets included", () => {
		const out = reduce(
			[
				"<Tabs>",
				'<TabItem label="Outer">',
				"Before.",
				"<Tabs>",
				'<TabItem label="Inner">Deep.</TabItem>',
				"</Tabs>",
				"</TabItem>",
				'<TabItem label="Second">Two.</TabItem>',
				"</Tabs>",
			].join("\n"),
		);
		assert.match(out, /^- \*\*Outer\*\*\n\n {2}Before\./m);
		assert.match(out, /^ {2}- \*\*Inner\*\*\n\n {4}Deep\./m);
		assert.match(out, /^- \*\*Second\*\*\n\n {2}Two\./m);
		assert.doesNotMatch(out, /<\/?Tab/);
	});

	test("an aside and a directive become titled blockquotes", () => {
		const out = reduce(
			'<Aside type="caution" title="Careful">Mind the gap.</Aside>\n\n:::tip\nA hint.\n:::',
		);
		assert.match(out, /^> \*\*Careful\*\*\n>\n> Mind the gap\./m);
		assert.match(out, /^> \*\*Tip\*\*\n>\n> A hint\./m);
	});

	test("a directive on a Spanish page takes the Spanish default title", () => {
		assert.match(
			reduce(":::note\nUna nota.\n:::", { route: "es/guide" }),
			/> \*\*Nota\*\*/,
		);
	});

	test("facts, link cards, charts and cards reduce to lists and images", () => {
		const out = reduce(
			[
				"<Facts>",
				'\t<Fact kind="exposes">',
				"\t\tone tool per",
				"\t\toperation.",
				"\t</Fact>",
				'\t<Fact label="Note">Plain.</Fact>',
				"</Facts>",
				"",
				'<LinkCard title="About" description="Who and why." href="/gitlab-mcp-server/about/" />',
				"",
				'<ChartPair name="memory" lang="en" alt="Bars." />',
				"",
				'<CardGrid><Card title="Review" icon="pencil">',
				"  First.",
				"",
				"     Second.",
				"</Card></CardGrid>",
			].join("\n"),
		);
		assert.match(out, /^- \*\*What it exposes\*\*: one tool per operation\.$/m);
		assert.match(out, /^- \*\*Note\*\*: Plain\.$/m);
		assert.match(
			out,
			/^- \[About\]\(https:\/\/docs\.jmrp\.io\/gitlab-mcp-server\/about\/\): Who and why\.$/m,
		);
		assert.match(
			out,
			/!\[Bars\.\]\(https:\/\/docs\.jmrp\.io\/gitlab-mcp-server\/benchmarks\/en\/memory\.light\.svg\)/,
		);
		assert.match(out, /^- \*\*Review\*\*\n\n {2}First\.\n\n {2}Second\.$/m);
	});

	test("cards of a grid stay siblings however unevenly they are indented", () => {
		const out = reduce(
			[
				"<CardGrid>",
				' <Card title="Review">',
				"  First.",
				" </Card>",
				"",
				'    <Card title="Deploy">',
				"     Second.",
				"    </Card>",
				"</CardGrid>",
			].join("\n"),
		);
		assert.match(out, /^- \*\*Review\*\*$/m);
		assert.match(out, /^- \*\*Deploy\*\*\n\n {2}Second\.$/m);
	});

	test("FAQ renders the frontmatter questions with their tokens resolved", () => {
		const out = reduce(
			'import FAQ from "../../components/FAQ.astro";\n\n<FAQ />',
			{
				faq: [{ q: "How many?", a: "{{tools.free}} tools, see `{id}`." }],
			},
		);
		assert.match(
			out,
			/^## Frequently asked questions\n\n### How many\?\n\n875 tools, see `\{id\}`\.$/m,
		);
	});

	test("FAQ on a page without questions renders nothing", () => {
		assert.throws(() => reduce("<FAQ />"), /nothing left after reduction/);
	});

	test("Code prints the raw import it is given, blank lines kept", () => {
		const out = reduce(
			'import sample from "../../../examples/sample.yaml?raw";\n\n<Code code={sample} lang="yaml" title="sample.yaml" />',
		);
		assert.match(
			out,
			/^```yaml title="sample\.yaml"\nkey: value\n\nother: 1\n```$/m,
		);
	});

	test("Home renders through the landing renderer it is given", () => {
		const out = reduce(
			'import { en } from "../data/home";\n\n<Home content={en} />',
			{
				route: "",
				home: { en: { greeting: "Hello" } },
				renderHome: (content) => `## ${content.greeting}\n\n\`<b>\``,
			},
		);
		assert.equal(out, "## Hello\n\n`<b>`");
	});
});

describe("code stays opaque", () => {
	test("pseudo-tags and braces in code are neither components nor expressions", () => {
		const body = [
			"Named `GITLAB_MCP_<NAME>` and `{project_id}`.",
			"",
			"```text",
			"<operation>: <classification> (<GitLab's message>)",
			"<VirtualHost *:443>",
			"import x from 'y'",
			"{message: 403 Forbidden}",
			"",
			"",
			"[link](/relative)",
			"```",
		].join("\n");
		assert.equal(reduce(body), body);
	});

	test("an indented fence moves with its tab and keeps its blank lines", () => {
		const out = reduce(
			'<Tabs>\n\t<TabItem label="A">\n\t\t```sh\n\t\techo <URL>\n\n\n\t\techo two\n\t\t```\n\t</TabItem>\n</Tabs>',
		);
		assert.match(
			out,
			/^ {2}```sh\n {2}echo <URL>\n\n\n {2}echo two\n {2}```$/m,
		);
	});
});

describe("expressions", () => {
	test("data paths, formatting and arithmetic resolve to their values", () => {
		const out = reduce(
			[
				'import stats from "../../data/stats.json";',
				"",
				'{stats.tools.free} on Free, {stats.tools.gitlab_com.toLocaleString("en-US")} at most,',
				"{stats.meta.premium - stats.meta.base} more, \\{literal\\}.",
			].join("\n"),
		);
		assert.equal(out, "875 on Free, 1,102 at most,\n6 more, {literal}.");
	});

	test("an expression naming nothing the page imports fails", () => {
		assert.throws(
			() => reduce("{footprint.total}"),
			/names footprint, which the page does not import/,
		);
	});

	test("a path with no value fails", () => {
		assert.throws(
			() => evaluate("stats.nope", new Map([["stats", {}]]), "x.mdx"),
			/has no value/,
		);
	});

	test("an object in prose fails", () => {
		assert.throws(
			() =>
				reduce('import stats from "../../data/stats.json";\n\n{stats.tools}'),
			/is an object, not text/,
		);
	});

	test("an import with no reduction fails", () => {
		assert.throws(
			() => reduce('import thing from "./thing.ts";\n\nText.'),
			/has no reduction/,
		);
	});
});

describe("links", () => {
	test("site, fragment and relative asset targets become absolute", () => {
		const out = reduce(
			'[A](/gitlab-mcp-server/about/) [B](#heading) ![C](./img.png) <a href="/gitlab-mcp-server/x/">D</a> [E](https://example.com)',
		);
		assert.equal(
			out,
			"[A](https://docs.jmrp.io/gitlab-mcp-server/about/) [B](https://docs.jmrp.io/gitlab-mcp-server/guide/#heading) " +
				"![C](https://raw.githubusercontent.com/jmrplens/gitlab-mcp-server/main/site/src/content/docs/img.png) " +
				'<a href="https://docs.jmrp.io/gitlab-mcp-server/x/">D</a> [E](https://example.com)',
		);
	});

	test("a Spanish page localizes site links but not images", () => {
		const out = reduce(
			"[A](/gitlab-mcp-server/about/) [B](/gitlab-mcp-server/es/roadmap/) ![C](/gitlab-mcp-server/og.png)",
			{ route: "es/guide" },
		);
		assert.equal(
			out,
			"[A](https://docs.jmrp.io/gitlab-mcp-server/es/about/) [B](https://docs.jmrp.io/gitlab-mcp-server/es/roadmap/) " +
				"![C](https://docs.jmrp.io/gitlab-mcp-server/og.png)",
		);
	});

	test("a target that resolves nowhere fails", () => {
		assert.throws(() => reduce("[A](/elsewhere/)"), /points nowhere/);
		assert.throws(() => reduce("[A](../about/)"), /points nowhere/);
	});
});

describe("renderTwin", () => {
	test("writes the sibling header, the release and the body", () => {
		const twin = renderTwin({
			title: "Guide",
			description: "What the guide covers.",
			body: "---\n\nInstall %%VERSION%%.",
			file: "src/content/docs/guide.mdx",
			route: "guide",
			root,
			release: "9.8.7",
			labels: LABELS,
			stats: {},
			updated: "2026-11-20",
		});
		assert.equal(
			twin,
			"# Guide\n\nWhat the guide covers.\n\nSource: https://docs.jmrp.io/gitlab-mcp-server/guide/\nUpdated: 2026-11-20\n\n---\n\nInstall 9.8.7.\n",
		);
	});

	test("a page with no description fails", () => {
		assert.throws(
			() =>
				renderTwin({
					title: "T",
					description: "",
					body: "x",
					file: "f.mdx",
					route: "f",
				}),
			/no frontmatter description/,
		);
	});
});

describe("landing", () => {
	test("inline HTML becomes markdown, and unknown markup fails", () => {
		assert.equal(
			inlineHtml('<strong>A</strong> <a href="/x/">b</a> &amp; c', "f"),
			"**A** [b](/x/) & c",
		);
		assert.throws(
			() => inlineHtml("<span>x</span>", "f"),
			/no markdown for the markup <span>/,
		);
	});

	test("renders the figures from the data it is given", () => {
		const content = {
			numberLocale: "en-US",
			statsLabel: "At a glance",
			statLabels: {
				dynamic: "d",
				context: "c",
				tools: "t",
				prompts: "p",
				resources: "r",
			},
			statHrefs: {
				dynamic: "/d/",
				context: "/c/",
				tools: "/t/",
				prompts: "/p/",
				resources: "/r/",
			},
			contextClaim: "Costs {default}; {minimal} minimal.",
			what: { title: "What", body: ["Body."] },
			who: { title: "Who", items: ["You."] },
			honest: { title: "Not", items: ["Magic."] },
			proof: {
				title: "Use",
				lead: "Lead.",
				installTitle: "Install",
				installNote: "Note.",
				install: "run it",
				promptsTitle: "Ask",
				promptsNote: "Ask it.",
				promptHeaders: ["Say", "Does"],
				prompts: [
					["a | b", "c"],
					["x \\| y", "z"],
				],
			},
			surfaces: {
				title: "Surfaces",
				lead: "Three.",
				headers: ["S", "T", "W"],
				rows: [
					{ id: "meta", name: "Meta", summary: "per domain", when: "Often." },
				],
				tierNames: ["Free", "Premium", "Ultimate"],
				legend: "Ranges.",
			},
			start: {
				title: "Start",
				lead: "Go.",
				steps: [
					{
						title: "One",
						body: "Do.",
						code: "x=1",
						href: "/one/",
						linkText: "One",
					},
				],
			},
		};
		const out = renderHome(content, {
			stats: {
				dynamic: 2,
				prompts: 3,
				resources: 4,
				tools: { free: 875, gitlab_com: 1102 },
				meta: { base: 34, gitlab_com: 52 },
			},
			footprint: {
				dynamic: { tool_schema_tokens: 1000 },
				shared: { full: 1500, minimal: 200 },
			},
			file: "index.mdx",
		});
		assert.match(out, /^- \*\*2,500\*\* \[c\]\(\/c\/\)$/m);
		assert.match(out, /^Costs 2,500; 1,200 minimal\.$/m);
		assert.match(out, /^\| a \\\| b \| c \|$/m);
		// A backslash of the copy is a character, escaped before the pipe, so
		// it neither escapes the pipe's escape nor splits the cell.
		assert.match(out, /^\| x \\\\\\\| y \| z \|$/m);
		assert.ok(
			out.includes(
				`| **Meta**: per domain | 34${String.fromCharCode(0x2013)}52 | Often. |`,
			),
		);
		assert.match(
			out,
			/^1\. \*\*One\*\*\n\n {3}Do\.\n\n {3}```bash\n {3}x=1\n {3}```\n\n {3}\[One\]\(\/one\/\)$/m,
		);
	});
});
