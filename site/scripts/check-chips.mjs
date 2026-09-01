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
//     every absolute href the landing data module (home.ts) declares; an
//     href with a fragment must also name a heading of that page. The
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
	// The schema accepts any YAML form, but this parser reads only the
	// block style with `- text:` opening each entry. Every other accepted
	// form (flow style, href-first entries) would silently parse as
	// chip-free and escape the audit — refuse it loudly instead.
	const flow = lines.find((l) => /^chips:\s*\S/.test(l));
	if (flow) {
		throw new Error(
			`${path}: chips uses a form this audit cannot read (${flow.trim()}); use block style with "- text:" first`,
		);
	}
	const start = lines.findIndex((l) => /^chips:\s*$/.test(l));
	if (start < 0) return [];
	const chips = [];
	for (let i = start + 1; i < lines.length; i++) {
		const line = lines[i];
		if (/^\S/.test(line)) break; // frontmatter key at top level ends the block
		const text = /^\s+-\s+text:\s*(.+)$/.exec(line);
		const href = /^\s+href:\s*(\S+)$/.exec(line);
		const otherEntry = /^\s+-\s+(?!text:)\S/.exec(line);
		if (otherEntry) {
			throw new Error(
				`${path}: a chip entry does not open with "- text:" (${line.trim()}); this audit cannot read it`,
			);
		}
		if (text) chips.push({ text: text[1].trim(), href: null });
		else if (href && chips.length) chips[chips.length - 1].href = href[1];
	}
	return chips;
}

/** Resolve a site href to the content file that renders it, at its own
 * declared path — a Spanish href is checked against the Spanish tree, so a
 * missing translation cannot hide behind its English twin. */
function hrefExists(href) {
	if (!href.startsWith(BASE + "/")) return false;
	const [path, fragment] = href.split("#", 2);
	const rel = path.slice(BASE.length + 1).replace(/\/$/, "");
	const file = ["mdx", "md"]
		.flatMap((ext) => [
			join(ROOT, `${rel}.${ext}`),
			join(ROOT, rel, `index.${ext}`),
		])
		.find((candidate) => existsSync(candidate));
	if (!file) return false;
	// A fragment must name a heading of that page. The slug follows the rules
	// Astro applies (github-slugger): lowercase, punctuation dropped, accents
	// kept, spaces to hyphens. So a deep link into a section is checked
	// against the section, not waved through on the page alone.
	return !fragment || headingSlugs(file).has(decodeURIComponent(fragment));
}

/** The github-slugger form of every Markdown heading in a page, code fences
 * excluded. */
function headingSlugs(file) {
	const slugs = new Set();
	let inFence = false;
	for (const line of readFileSync(file, "utf8").split("\n")) {
		if (/^\s*(```|~~~)/.test(line)) {
			inFence = !inFence;
			continue;
		}
		if (inFence) continue;
		const heading = /^#{1,6}\s+(.+?)\s*#*\s*$/.exec(line);
		if (!heading) continue;
		const custom = /\{#([^}]+)\}\s*$/.exec(heading[1]);
		if (custom) {
			slugs.add(custom[1]);
			continue;
		}
		const text = heading[1]
			.replace(/`([^`]*)`/g, "$1")
			.replace(/\[([^\]]*)\]\([^)]*\)/g, "$1")
			.replace(/[*_]/g, "");
		slugs.add(
			text
				.toLowerCase()
				.replace(/[^\p{L}\p{N}\s-]/gu, "")
				.trim()
				.replace(/\s+/g, "-"),
		);
	}
	return slugs;
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
	if (!hrefExists(href)) {
		fail(`home.ts: href ${href} resolves to no page or heading`);
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
			if (href && !hrefExists(href)) {
				fail(
					`${rel}: ${locale} chip ${i + 1} href ${href} resolves to no page or heading`,
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
