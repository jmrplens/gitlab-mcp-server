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
//     doctype is uppercased and the title shortened, and on a Spanish page
//     the visible sentence is put in Spanish; the refresh, the canonical link
//     and the link itself are left as Astro wrote them.
//
// It also holds each of those pages to sending its reader somewhere. Astro
// puts the base in front of a redirect's source and not of its destination,
// so a destination written without the base builds into a page that
// refreshes to a 404 on the published site (issue 1109). A target on this
// site has to be under BASE and name a page or a file the build wrote; a
// target on another site is not checked. Every redirect that fails is named
// and the build fails.
import {
	existsSync,
	readdirSync,
	readFileSync,
	statSync,
	writeFileSync,
} from "node:fs";
import { join, sep } from "node:path";

const DIST = "dist";
const BASE = "/gitlab-mcp-server/";

/** True for the page Astro writes for an entry of `redirects`. */
function isRedirectStub(html) {
	return /<meta http-equiv="refresh"/.test(html);
}

/** The redirect page's doctype, title and sentence, in the language its path names. */
function normalizeRedirectStub(html, file) {
	const spanish = file.startsWith(`${DIST}${sep}es${sep}`);
	const title = spanish ? "Página trasladada" : "Page moved";
	let next = html
		.replace(/^<!doctype html>/i, "<!DOCTYPE html>")
		.replace(/<title>[^<]*<\/title>/, `<title>${title}</title>`);
	if (spanish) {
		next = next.replace(
			/>Redirecting from <code>([^<]*)<\/code> to <code>/,
			">Redirigiendo de <code>$1</code> a <code>",
		);
	}
	return next;
}

/** True when path names a file under DIST. */
function isFile(path) {
	return existsSync(path) && statSync(path).isFile();
}

/**
 * Why the redirect page file sends its reader nowhere, or null when its
 * target is a page or a file this build wrote, or is on another site.
 */
function brokenRedirect(html, file) {
	const target =
		/<meta http-equiv="refresh" content="\d+;url=([^"]*)"/.exec(html)?.[1] ??
		"";
	if (/^[a-z][a-z0-9+.-]*:/i.test(target)) return null;
	const where = `${file} redirects to ${target}`;
	if (!target.startsWith(BASE)) return `${where}, which is outside ${BASE}`;
	const rel = target.slice(BASE.length).replace(/[?#].*$/, "");
	if (isFile(join(DIST, rel, "index.html")) || isFile(join(DIST, rel)))
		return null;
	return `${where}, a page this build does not hold`;
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
const broken = [];

for (const file of htmlFiles(DIST)) {
	const original = readFileSync(file, "utf8");

	const brMatches = original.match(/<br\s*\/?><\/br>/g);
	let next = original.replace(/<br\s*\/?><\/br>/g, "<br>");
	if (brMatches) brFixed += brMatches.length;

	if (isRedirectStub(next)) {
		next = normalizeRedirectStub(next, file);
		redirects++;
		const problem = brokenRedirect(next, file);
		if (problem) broken.push(problem);
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

if (broken.length) {
	console.error(
		"[normalize-html] redirect pages that send their reader nowhere:",
	);
	for (const problem of broken) console.error(`  ${problem}`);
	process.exit(1);
}
