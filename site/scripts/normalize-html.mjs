// Post-build HTML normalisation.
//
// Three fixes, all applied in one pass over dist/**/*.html:
//
//  1. Trailing whitespace on every line. Previously an inline `node -e` in the
//     postbuild script; moved here so it is readable and testable.
//
//  2. `<br></br>` -> `<br>`. Mermaid emits `<br/>` inside the <foreignObject>
//     labels of build-time-rendered diagrams. That is well-formed XML, but the
//     page is served as HTML, where `</br>` is not a valid end tag — browsers
//     may treat it as a second line break, and it fails htmlhint's tag-pair
//     rule. Rewriting to a plain `<br>` is correct in both parsers.
//
//  3. The page Astro writes for an entry of `redirects` in astro.config.mjs.
//     It opens with a lowercase doctype and a title naming the whole target
//     URL, which html-validate refuses (doctype-style, long-title). The
//     doctype is uppercased and the title shortened; the refresh, the
//     canonical link and the visible link are left as Astro wrote them.
import { readdirSync, readFileSync, writeFileSync } from "node:fs";
import { join, sep } from "node:path";

const DIST = "dist";

/** True for the page Astro writes for an entry of `redirects`. */
function isRedirectStub(html) {
	return /<meta http-equiv="refresh"/.test(html);
}

/** The redirect page's doctype and title, in the language its path names. */
function normalizeRedirectStub(html, file) {
	const spanish = file.startsWith(`${DIST}${sep}es${sep}`);
	const title = spanish ? "Página trasladada" : "Page moved";
	return html
		.replace(/^<!doctype html>/i, "<!DOCTYPE html>")
		.replace(/<title>[^<]*<\/title>/, `<title>${title}</title>`);
}

/** Yields every .html file under dir, recursively. */
function* htmlFiles(dir) {
	for (const entry of readdirSync(dir, { withFileTypes: true })) {
		const full = join(dir, entry.name);
		if (entry.isDirectory()) yield* htmlFiles(full);
		else if (entry.isFile() && entry.name.endsWith(".html")) yield full;
	}
}

let changed = 0;
let brFixed = 0;
let redirects = 0;

for (const file of htmlFiles(DIST)) {
	const original = readFileSync(file, "utf8");

	const brMatches = original.match(/<br\s*\/?><\/br>/g);
	let next = original.replace(/<br\s*\/?><\/br>/g, "<br>");
	if (brMatches) brFixed += brMatches.length;

	if (isRedirectStub(next)) {
		next = normalizeRedirectStub(next, file);
		redirects++;
	}

	next = next
		.split("\n")
		.map((line) => line.replace(/[ \t\r]+$/, ""))
		.join("\n");

	if (next !== original) {
		writeFileSync(file, next);
		changed++;
	}
}

console.log(
	`[normalize-html] rewrote ${changed} file(s); fixed ${brFixed} invalid </br> tag(s); normalised ${redirects} redirect page(s)`,
);
