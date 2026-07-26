// Guards heading structure and accessible names across the built site.
//
// html-validate and htmlhint check markup validity; neither checks that every
// interactive element has an accessible name, that landmarks are labelled, or
// that main content has exactly one <h1> and no skipped levels. Those are the
// things AI extractors and screen readers both rely on, so they are worth a
// gate of their own.
//
// Deliberately scoped to <main>: the site-title link, the sidebar table of
// contents and the mobile nav all live outside it, and their headings do not
// belong to the document's content outline.
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";

const DIST = "dist";

function* htmlFiles(dir) {
	for (const entry of readdirSync(dir, { withFileTypes: true })) {
		const full = join(dir, entry.name);
		if (entry.isDirectory()) yield* htmlFiles(full);
		else if (entry.isFile() && entry.name === "index.html") yield full;
	}
}

const stripTags = (s) =>
	s
		.replace(/<[^>]+>/g, " ")
		.replace(/&[a-z#0-9]+;/gi, " ")
		.trim();

/** Static approximation of the HTML accessible-name computation. */
function hasAccessibleName(attrs, inner) {
	if (/\saria-label(ledby)?="[^"]+"/.test(attrs)) return true;
	if (stripTags(inner)) return true;
	if (/\stitle="[^"]+"/.test(attrs)) return true;
	if (/<img[^>]*\salt="[^"]+"/.test(inner)) return true;
	if (/<title[^>]*>[^<]+<\/title>/.test(inner)) return true;
	return false;
}

const problems = [];
const report = (file, msg) =>
	problems.push(
		`${file.replace(`${DIST}/`, "/").replace("/index.html", "/")} — ${msg}`,
	);

let pages = 0;
for (const file of htmlFiles(DIST)) {
	const html = readFileSync(file, "utf8");
	pages++;

	for (const [tag, expected] of [
		["main", 1],
		["header", 1],
		["footer", 1],
	]) {
		const found = (html.match(new RegExp(`<${tag}[\\s>]`, "g")) ?? []).length;
		if (found !== expected)
			report(file, `expected ${expected} <${tag}>, found ${found}`);
	}

	for (const m of html.matchAll(/<nav\b([^>]*)>/g)) {
		if (!/\saria-label(ledby)?=/.test(m[1]))
			report(file, "<nav> without an accessible name");
	}

	for (const m of html.matchAll(/aria-labelledby="([^"]+)"/g)) {
		for (const ref of m[1].split(/\s+/)) {
			if (!html.includes(`id="${ref}"`))
				report(file, `aria-labelledby points at missing id "${ref}"`);
		}
	}

	for (const tag of ["a", "button"]) {
		const re = new RegExp(`<${tag}\\b([^>]*)>([\\s\\S]*?)</${tag}>`, "g");
		for (const m of html.matchAll(re)) {
			if (!hasAccessibleName(m[1], m[2])) {
				report(
					file,
					`<${tag}> without an accessible name: ${m[0].slice(0, 70)}`,
				);
			}
		}
	}

	if (!/<html[^>]*\slang="[a-z-]+"/i.test(html))
		report(file, "missing <html lang>");

	const start = html.indexOf("<main");
	const end = html.indexOf("</main>");
	if (start === -1 || end === -1) continue;
	const levels = [...html.slice(start, end).matchAll(/<h([1-6])[^>]*>/g)].map(
		(m) => Number(m[1]),
	);
	const h1s = levels.filter((l) => l === 1).length;
	if (h1s !== 1) report(file, `<main> has ${h1s} <h1>, expected exactly 1`);
	if (levels.length && levels[0] !== 1)
		report(file, `<main> starts at h${levels[0]}, expected h1`);
	for (let i = 1; i < levels.length; i++) {
		if (levels[i] > levels[i - 1] + 1) {
			report(file, `heading level skips h${levels[i - 1]} -> h${levels[i]}`);
		}
	}
}

if (problems.length) {
	console.error(
		`[check-headings-labels] ${problems.length} problem(s) across ${pages} pages:`,
	);
	for (const p of problems) console.error(`  ${p}`);
	process.exit(1);
}
console.log(
	`[check-headings-labels] OK: ${pages} pages, headings and accessible names clean.`,
);
