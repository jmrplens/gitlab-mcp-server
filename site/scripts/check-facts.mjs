// Structural audit of the spec sheets (src/components/docs/Facts.astro).
//
// Three jobs, and the first two gate.
//
// PARITY. `<Fact kind="…">` carries a language-independent vocabulary, which
// is what lets an English declaration and its Spanish twin be recognised as
// the same declaration. So every sheet must present the same sequence of
// kinds in both locales. check-i18n-parity.mjs counts headings and backticked
// identifiers and notices neither a sheet that loses its "what it does not
// do" row nor one that gains a kind in one locale — the labels live in the
// component now, not in the prose it compares.
//
// TABLE LAYOUT. Whether a markdown table stacks on a narrow screen is decided
// from its own contents per file (src/lib/wide-tables.mjs), so a page and its
// Spanish twin can land on opposite sides of a threshold and silently render
// with different layouts — a translated heading is easily a few characters
// wider. Both locales must reach the same verdict for the same table. This
// runs the site's own classifier, so the thresholds live in exactly one
// place.
//
// CENSUS. Every `data-fact="not-covered"` row is a promise about a boundary,
// and the set of them is the honest answer to "what will this surface or
// mode not do?". A spec sheet with no such row has not been audited rather
// than having nothing to declare, so the census names the gaps. It reports
// rather than fails: the remedy is a sentence somebody has to verify, not a
// sentence this script can write. Troubleshooting triples
// (symptom/meaning/fixes) are excluded — a symptom owes no boundary.
//
// Run: node scripts/check-facts.mjs
import { readFileSync, readdirSync, statSync } from "node:fs";
import { join, relative } from "node:path";
import { fileURLToPath } from "node:url";

import { classify } from "../src/lib/wide-tables.mjs";

const docsDir = fileURLToPath(new URL("../src/content/docs", import.meta.url));
const esDir = join(docsDir, "es");

/** Every .mdx/.md under `dir`, recursively, excluding the Spanish subtree. */
function walk(dir) {
	const out = [];
	for (const entry of readdirSync(dir)) {
		const p = join(dir, entry);
		if (statSync(p).isDirectory()) {
			if (p === esDir) continue;
			out.push(...walk(p));
		} else if (/\.mdx?$/.test(entry)) {
			out.push(p);
		}
	}
	return out;
}

/**
 * The spec sheets of one page: for each, the heading that introduces it and
 * the ordered kinds it declares. A free-text `label` row becomes "note",
 * matching what the component emits, because its wording is allowed to
 * differ between locales while its position is not.
 */
function sheets(path) {
	const text = readFileSync(path, "utf8");
	const found = [];
	const marks = [
		...text.matchAll(/^(#{2,4}) (.+)$/gm),
		...text.matchAll(/<Facts[\s>]/g),
	].sort((a, b) => a.index - b.index);

	let heading = "(no heading)";
	for (const m of marks) {
		if (m[1]) {
			heading = m[2].trim();
			continue;
		}
		const end = text.indexOf("</Facts>", m.index);
		const body = text.slice(m.index, end === -1 ? undefined : end);
		const kinds = [...body.matchAll(/<Fact\s+(kind|label)="([^"]*)"/g)].map(
			(f) => (f[1] === "kind" ? f[2] : "note"),
		);
		found.push({ heading, kinds });
	}
	return found;
}

/**
 * Every markdown table in a page, as rows of cell text rendered the way the
 * classifier sees it at build time: a code span is its contents, a link is
 * its label. Measuring raw markdown instead counts backticks and `](…)` and
 * reaches a different verdict — a gate reporting a divergence that does not
 * exist.
 */
function markdownTables(text) {
	const body = text.slice(text.indexOf("\n---\n", 3) + 5);
	const render = (cell) =>
		cell
			// A JSX stat interpolation renders to a short figure at build time;
			// measuring the raw expression would count its variable path as a
			// 30-character unbreakable token. Both locales carry the same
			// expressions, so a fixed-width placeholder keeps the comparison
			// meaningful.
			.replace(/\{[^}]*\}/g, "9999")
			.replace(/`([^`]*)`/g, "$1")
			.replace(/\[([^\]]*)\]\([^)]*\)/g, "$1")
			.replace(/[*_]/g, "")
			.replace(/\\(.)/g, "$1");
	const tables = [];
	for (const block of body.matchAll(/^\|.*\|\n\|[-: |]+\|\n(?:\|.*\|\n)+/gm)) {
		tables.push(
			block[0]
				.trim()
				.split("\n")
				.filter((line, index) => index !== 1)
				.map((line) =>
					line
						.replace(/^\||\|$/g, "")
						.split("|")
						.map((cell) => render(cell.trim())),
				),
		);
	}
	return tables;
}

const problems = [];
const census = { withBoundary: [], withoutBoundary: [] };
let sheetCount = 0;
let tripleSheets = 0;

/** A troubleshooting triple owes no boundary declaration. */
const isTriple = (kinds) =>
	kinds.some((k) => ["symptom", "meaning", "fixes"].includes(k));

for (const enPath of walk(docsDir)) {
	const rel = relative(docsDir, enPath);
	const esPath = join(esDir, rel);
	let es;
	try {
		es = sheets(esPath);
	} catch {
		if (sheets(enPath).length > 0) {
			problems.push(`${rel}: carries spec sheets but has no Spanish twin`);
		}
		continue;
	}
	const en = sheets(enPath);
	if (en.length === 0 && es.length === 0) continue;

	if (en.length !== es.length) {
		problems.push(
			`${rel}: ${en.length} spec sheet(s) in EN, ${es.length} in ES`,
		);
		continue;
	}

	en.forEach((sheet, i) => {
		sheetCount++;
		const a = sheet.kinds.join(",");
		const b = es[i].kinds.join(",");
		if (a !== b) {
			problems.push(
				`${rel} — sheet ${i + 1} (${sheet.heading}):\n` +
					`      EN declares ${a}\n` +
					`      ES declares ${b}`,
			);
		}
		if (isTriple(sheet.kinds)) {
			tripleSheets++;
			return;
		}
		const name = `${rel}:${sheet.heading}`;
		(sheet.kinds.includes("notCovered")
			? census.withBoundary
			: census.withoutBoundary
		).push(name);
	});
}

// Spanish-only pages never enter the EN-driven walk above, so a sheet added
// there would escape both parity and census; sweep them.
function walkEs(dir) {
	const out = [];
	for (const entry of readdirSync(dir)) {
		const p = join(dir, entry);
		if (statSync(p).isDirectory()) out.push(...walkEs(p));
		else if (/\.mdx?$/.test(entry)) out.push(p);
	}
	return out;
}
for (const esPath of walkEs(esDir)) {
	const rel = relative(esDir, esPath);
	let hasTwin = true;
	try {
		statSync(join(docsDir, rel));
	} catch {
		hasTwin = false;
	}
	if (!hasTwin && sheets(esPath).length > 0) {
		problems.push(
			`es/${rel}: carries spec sheets but has no English counterpart`,
		);
	}
}

// Table-layout locale agreement, via the build's own classifier.
for (const enPath of walk(docsDir)) {
	const rel = relative(docsDir, enPath);
	let esText;
	try {
		esText = readFileSync(join(esDir, rel), "utf8");
	} catch {
		continue;
	}
	const en = markdownTables(readFileSync(enPath, "utf8"));
	const es = markdownTables(esText);
	if (en.length !== es.length) {
		problems.push(
			`${rel}: ${en.length} markdown table(s) in EN, ${es.length} in ES`,
		);
		continue;
	}
	en.forEach((rows, index) => {
		const a = classify(rows);
		const b = classify(es[index]);
		if (a !== b) {
			problems.push(
				`${rel} — table ${index + 1} ("${rows[0]?.[0] ?? ""}"): ` +
					`EN ${a === null ? "stays a table" : `stacks (${a || "below the breakpoint"})`}, ` +
					`ES ${b === null ? "stays a table" : `stacks (${b || "below the breakpoint"})`}`,
			);
		}
	});
}

const total = census.withBoundary.length + census.withoutBoundary.length;
console.log(
	`spec sheets: ${sheetCount} in each locale, with matching declarations` +
		` (${tripleSheets} troubleshooting triple(s))`,
);
console.log(
	`coverage claims: ${census.withBoundary.length} of ${total} spec sheets declare a boundary`,
);
for (const name of census.withoutBoundary) {
	console.log(`  no "what it does not do" row: ${name}`);
}
if (problems.length) {
	console.error("\ncheck-facts FAILED:");
	for (const p of problems) console.error(`  ${p}`);
}
process.exit(problems.length ? 1 : 0);
