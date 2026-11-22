// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// The closure between a page, its markdown twin and the llms.txt indexes.
//
// Each of those is generated, and each of them is a valid document when it is
// wrong: a page whose twin was never emitted still renders, a twin left behind
// by a deleted page still parses, an index still lists a URL that now 404s.
// Nothing else in the pipeline reads them, so this is the class of drift that
// ships silently and is only noticed by whoever fetched the twin instead of
// the page, which is a machine, which does not file bugs. The edge in front of
// the site only negotiates `Accept: text/markdown` for a page whose twin
// exists, so a missing twin is a page served to every agent as HTML.
//
// Run after a build (the site lint runs it):
//   node scripts/check-twins.mjs [dist-directory]
import { existsSync, readFileSync, readdirSync } from "node:fs";
import { join, resolve } from "node:path";
import process from "node:process";

import { TWIN_NOTE } from "./gen-llms.mjs";
import { dateProblems } from "./twin-dates.mjs";
import { twinQuestions } from "./twin-questions.mjs";
import { unanchoredTargets } from "./twin-targets.mjs";

const DIST = resolve(process.argv[2] ?? "dist");

// The site root as the build itself states it, rather than a constant this
// file would have to keep in step: the sitemap index is written from the same
// `site` and `base` every URL on the site is built from.
const sitemapIndex = join(DIST, "sitemap-index.xml");
if (!existsSync(sitemapIndex)) {
	console.error(
		`[twins] no sitemap-index.xml under ${DIST}. Run pnpm build first.`,
	);
	process.exit(1);
}
const SITE_ROOT = /<loc>([^<]+)\/sitemap-\d+\.xml<\/loc>/.exec(
	readFileSync(sitemapIndex, "utf8"),
)?.[1];
if (!SITE_ROOT) {
	console.error(
		"[twins] could not read the site root out of sitemap-index.xml",
	);
	process.exit(1);
}

const problems = [];
const fail = (message) => problems.push(message);

// The <lastmod> the sitemap states for each page, by its dist-relative
// directory ("" for the landing, "tools/overview/" for a page).
/** @type {Map<string, string>} */
const lastmods = new Map();
for (const name of readdirSync(DIST)) {
	if (!/^sitemap-\d+\.xml$/.test(name)) continue;
	for (const entry of readFileSync(join(DIST, name), "utf8").matchAll(
		/<loc>([^<]+)<\/loc><lastmod>([^<]+)<\/lastmod>/g,
	)) {
		if (entry[1].startsWith(`${SITE_ROOT}/`)) {
			lastmods.set(entry[1].slice(SITE_ROOT.length + 1), entry[2]);
		}
	}
}

/** The JSON-LD node of a page with one of the given ids, if it has one. */
function jsonLdNode(html, ids) {
	for (const script of html.matchAll(
		/<script type="application\/ld\+json"[^>]*>([\s\S]*?)<\/script>/g,
	)) {
		let data;
		try {
			data = JSON.parse(script[1]);
		} catch {
			continue;
		}
		for (const node of data["@graph"] ?? [data]) {
			if (ids.includes(node?.["@id"])) return node;
		}
	}
	return undefined;
}

/**
 * The dateModified of a page's own document node, the TechArticle or, on the
 * landings, the WebPage: not the site-wide graph's, which dates the software.
 */
const pageDateModified = (html, page) =>
	jsonLdNode(html, [
		`${SITE_ROOT}/${page}#article`,
		`${SITE_ROOT}/${page}#webpage`,
	])?.dateModified;

/** Every file under dist matching a name test, as dist-relative paths. */
function* files(dir, matches, prefix = "") {
	for (const entry of readdirSync(dir, { withFileTypes: true })) {
		const relative = prefix ? `${prefix}/${entry.name}` : entry.name;
		if (entry.isDirectory())
			yield* files(join(dir, entry.name), matches, relative);
		else if (entry.isFile() && matches(entry.name)) yield relative;
	}
}

/** Fails a file holding any target unanchoredTargets finds, naming a few. */
function checkTargets(name, text) {
	const targets = unanchoredTargets(text);
	if (targets.length > 0) {
		fail(
			`/${name} has ${targets.length} link target(s) with no origin, such as ${targets
				.slice(0, 3)
				.join(", ")}`,
		);
	}
}

// A page is an index.html that is not a redirect stub: the moved URLs in the
// `redirects` table of astro.config.mjs are built as pages that only refresh
// to where the content went, and they have nothing to double. The not-found
// page is dist/404.html, never an index.html.
const isRedirect = (html) => /<meta\s+http-equiv="refresh"/i.test(html);
const pages = [...files(DIST, (name) => name === "index.html")]
	.filter((path) => !isRedirect(readFileSync(join(DIST, path), "utf8")))
	.map((path) => path.replace(/index\.html$/, ""));
const twins = new Set(files(DIST, (name) => name === "index.md"));

if (pages.length === 0)
	fail(`no built pages under ${DIST}. Run pnpm build first.`);

for (const page of pages) {
	const twin = `${page}index.md`;
	if (!twins.has(twin)) {
		fail(`/${page} has no markdown twin at /${twin}`);
		continue;
	}
	const text = readFileSync(join(DIST, twin), "utf8");
	// The header every sibling site writes: title, description, the page's own
	// canonical URL, the optional date, and a line that is exactly `---`, which
	// is where the edge stops reading the header: without it the date is never
	// read. An empty twin is worse than a missing one: it answers the request,
	// so whatever fetched it reads the page as having no content.
	const header =
		/^# \S.*\n\n\S.*\n\nSource: (\S+)\n(?:Updated: (\d{4}-\d{2}-\d{2})\n)?\n---\n\n\S/.exec(
			text,
		);
	if (!header) {
		fail(
			`/${twin} does not open with "# title", the description, "Source:", the optional "Updated:" and a "---" line`,
		);
	} else if (header[1] !== `${SITE_ROOT}/${page}`) {
		fail(`/${twin} names ${header[1]} as its source, not ${SITE_ROOT}/${page}`);
	}
	// A character the reducer masks code with, left behind.
	if (/[\u{f0000}-\u{ffffd}]/u.test(text)) {
		fail(`/${twin} carries a masked character the reduction did not restore`);
	}
	checkTargets(twin, text);

	// The announcement, which is how anything finds the twin without guessing
	// the convention, and the same address in the page's structured data.
	const html = readFileSync(join(DIST, `${page}index.html`), "utf8");
	const link = /<link[^>]*rel="alternate"[^>]*type="text\/markdown"[^>]*>/.exec(
		html,
	);
	const href = link && /href="([^"]+)"/.exec(link[0])?.[1];
	const expected = new URL(SITE_ROOT).pathname.replace(/\/$/, "") + `/${twin}`;
	if (href !== expected) {
		fail(
			`/${page} announces its twin as ${href ?? "nothing"}; expected <link rel="alternate" type="text/markdown" href="${expected}">`,
		);
	}
	if (!html.includes(`"contentUrl":"${SITE_ROOT}/${twin}"`)) {
		fail(`/${page} does not describe its twin in its JSON-LD encoding`);
	}

	// The questions a page advertises in its FAQPage JSON-LD are the ones it
	// shows, and the twin carries the same. The JSON-LD is built from the
	// frontmatter whether or not the body places <FAQ />, so a page that
	// forgot the component advertised questions no reader could see.
	const faqPage = jsonLdNode(html, [`${SITE_ROOT}/${page}#faq`]);
	const advertised = (faqPage?.mainEntity ?? []).map((q) => q.name);
	const shown = twinQuestions(text);
	if (
		advertised.length !== shown.length ||
		advertised.some((q, i) => q !== shown[i])
	) {
		fail(
			`/${page} advertises ${advertised.length} FAQ question(s) in its FAQPage JSON-LD and its twin shows ${shown.length}; ` +
				"a page with faq: in its frontmatter places <FAQ /> in its body",
		);
	}

	// One date in three places (scripts/twin-dates.mjs says which).
	for (const problem of dateProblems({
		twin,
		updated: header?.[2],
		lastmod: lastmods.get(page),
		modified: pageDateModified(html, page),
	})) {
		fail(problem);
	}
}

for (const twin of twins) {
	if (!pages.includes(twin.replace(/index\.md$/, ""))) {
		fail(`/${twin} has no page: the twin outlived whatever it doubled`);
	}
}

// The two llms.txt indexes: they say the twins exist, every page they list
// exists, and every page of their locale is listed.
for (const [index, locale] of [
	["llms.txt", "en"],
	["es/llms.txt", "es"],
]) {
	const path = join(DIST, index);
	if (!existsSync(path)) {
		fail(`/${index} was not generated`);
		continue;
	}
	const text = readFileSync(path, "utf8");
	checkTargets(index, text);
	if (!text.includes(TWIN_NOTE[locale])) {
		fail(`/${index} does not carry the markdown twins note`);
	}
	const listed = [...text.matchAll(/\]\((https?:\/\/[^)]+)\)/g)].map(
		(m) => m[1],
	);
	if (listed.length === 0) fail(`/${index} lists no pages`);
	for (const url of listed) {
		if (!url.startsWith(SITE_ROOT)) continue; // an outbound link, not a page
		const relative = url.slice(SITE_ROOT.length).replace(/^\//, "");
		const target =
			relative === "" || relative.endsWith("/")
				? `${relative}index.html`
				: relative;
		if (!existsSync(join(DIST, target))) {
			fail(`/${index} lists ${url}, which the build does not contain`);
		}
	}
	const ours = pages.filter(
		(page) => page.startsWith("es/") === (locale === "es"),
	);
	for (const page of ours) {
		if (!listed.includes(`${SITE_ROOT}/${page}`)) {
			fail(`/${index} does not list /${page}`);
		}
	}
}

if (problems.length > 0) {
	console.error(`[twins] ${problems.length} problem(s):`);
	for (const problem of problems) console.error(`  ${problem}`);
	process.exit(1);
}

console.log(
	`[twins] ${pages.length} pages, ${twins.size} twins, each announced and each with a page; both llms.txt indexes closed.`,
);
