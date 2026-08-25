// Chip parity and link audit for the page-title chips.
//
// Two invariants, both of the kind a reviewer cannot hold in their head
// across 70 pages:
//
//   - PARITY: an English page and its Spanish twin carry the same number of
//     chips, in the same order of kinds (link or plain), with each link
//     pointing at the same target modulo the /es/ locale segment. A chip
//     added to one locale alone is exactly the structural drift the
//     data-driven landing was built to remove.
//   - EXISTENCE: every chip href resolves to a content page, and so does
//     every absolute href the landing data module (home.ts) declares. The
//     links validator only sees the content AST; frontmatter and data-module
//     hrefs are invisible to it. ES-only pages are swept too, so a chip
//     cannot escape by living outside the EN-driven pair walk.
//
// Run: node scripts/check-chips.mjs
import { readFileSync, readdirSync, existsSync, statSync } from "node:fs";
import { join } from "node:path";

const ROOT = new URL("../src/content/docs", import.meta.url).pathname;
const BASE = "/gitlab-mcp-server";

function pages(dir, out = []) {
	for (const name of readdirSync(dir)) {
		const path = join(dir, name);
		if (statSync(path).isDirectory()) pages(path, out);
		else if (/\.mdx?$/.test(name)) out.push(path);
	}
	return out;
}

/** The chips array of one page's frontmatter, parsed structurally. */
function chipsOf(path) {
	const src = readFileSync(path, "utf8");
	const fm = /^---\n([\s\S]*?)\n---/.exec(src);
	if (!fm) return [];
	const lines = fm[1].split("\n");
	const start = lines.findIndex((l) => /^chips:\s*$/.test(l));
	if (start < 0) return [];
	const chips = [];
	for (let i = start + 1; i < lines.length; i++) {
		const line = lines[i];
		if (/^\S/.test(line)) break; // frontmatter key at top level ends the block
		const text = /^\s+-\s+text:\s*(.+)$/.exec(line);
		const href = /^\s+href:\s*(\S+)$/.exec(line);
		if (text) chips.push({ text: text[1].trim(), href: null });
		else if (href && chips.length) chips[chips.length - 1].href = href[1];
	}
	return chips;
}

/** Resolve a site href to the content file that renders it. */
function hrefExists(href) {
	if (!href.startsWith(BASE + "/")) return false;
	const rel = href.slice(BASE.length + 1).replace(/\/$/, "");
	return ["mdx", "md"].some(
		(ext) =>
			existsSync(join(ROOT, `${rel}.${ext}`)) ||
			existsSync(join(ROOT, rel, `index.${ext}`)),
	);
}

let failures = 0;
const fail = (msg) => {
	failures++;
	console.error(`  FAIL ${msg}`);
};

// The landing's data module carries hrefs of its own (readout labels, start
// steps) that the links validator never sees either — audit every absolute
// site href it declares, both locales, against the content tree.
const homeTs = readFileSync(
	new URL("../src/data/home.ts", import.meta.url).pathname,
	"utf8",
);
let homeHrefs = 0;
for (const [, href] of homeTs.matchAll(/"(\/gitlab-mcp-server\/[^"\s]*)"/g)) {
	homeHrefs++;
	if (!hrefExists(href.replace(`${BASE}/es/`, `${BASE}/`))) {
		fail(`home.ts: href ${href} resolves to no page`);
	}
}

// ES-only pages never appear in the EN-driven pair walk below, so their
// chips would escape both checks; sweep them for href existence here (and
// flag them — a Spanish page with no English twin is itself parity drift).
for (const esPage of pages(join(ROOT, "es"))) {
	const rel = esPage.slice(join(ROOT, "es").length + 1);
	if (
		existsSync(join(ROOT, rel)) ||
		existsSync(join(ROOT, rel.replace(/\.md$/, ".mdx"))) ||
		existsSync(join(ROOT, rel.replace(/\.mdx$/, ".md")))
	)
		continue;
	const orphanChips = chipsOf(esPage);
	if (orphanChips.length) {
		fail(`es/${rel}: carries chips but has no English twin`);
	}
}

const en = pages(ROOT).filter((p) => !p.includes("/es/"));
let withChips = 0;
for (const page of en) {
	const rel = page.slice(ROOT.length + 1);
	const esPage = join(ROOT, "es", rel);
	const a = chipsOf(page);
	const b = existsSync(esPage) ? chipsOf(esPage) : [];
	if (a.length === 0 && b.length === 0) continue;
	withChips++;
	if (!existsSync(esPage)) {
		fail(`${rel}: carries chips but has no Spanish twin`);
		continue;
	}
	if (a.length !== b.length) {
		fail(`${rel}: ${a.length} chips in EN, ${b.length} in ES`);
		continue;
	}
	for (let i = 0; i < a.length; i++) {
		const ha = a[i].href;
		const hb = b[i].href;
		if ((ha === null) !== (hb === null)) {
			fail(
				`${rel}: chip ${i + 1} is a ${ha ? "link" : "label"} in EN but not in ES`,
			);
			continue;
		}
		if (ha && hb && hb !== ha.replace(`${BASE}/`, `${BASE}/es/`)) {
			fail(`${rel}: chip ${i + 1} links ${ha} in EN but ${hb} in ES`);
		}
		for (const [locale, href] of [
			["EN", ha],
			["ES", hb],
		]) {
			if (href && !hrefExists(href.replace(`${BASE}/es/`, `${BASE}/`))) {
				fail(
					`${rel}: ${locale} chip ${i + 1} href ${href} resolves to no page`,
				);
			}
		}
	}
}

if (failures) {
	console.error(
		`chip audit: ${failures} failure(s) across ${withChips} chip-carrying page pair(s).`,
	);
	process.exit(1);
}
console.log(
	`chip audit OK: ${withChips} page pair(s) with chips, parity and links verified.`,
);
