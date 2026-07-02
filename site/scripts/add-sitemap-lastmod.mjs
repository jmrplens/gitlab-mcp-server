// Injects <lastmod> into the generated sitemap.
//
// Starlight's built-in sitemap (via @astrojs/sitemap) emits <loc> entries with
// no <lastmod>, so crawlers — including AI crawlers — can't tell when a page last
// changed. This post-build step maps each sitemap URL back to its source content
// file and stamps the file's last Git commit date as <lastmod>. Runs after
// `astro build` (see package.json `postbuild`).
//
// Date resolution per URL: Git commit date of the source .mdx/.md → file mtime →
// build time. In a shallow CI checkout Git dates may collapse to the latest
// commit; the deploy workflow uses fetch-depth: 0 so per-file dates are accurate.
import { execFileSync } from "node:child_process";
import {
	existsSync,
	readdirSync,
	readFileSync,
	statSync,
	writeFileSync,
} from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const siteRoot = join(here, "..");
const repoRoot = join(siteRoot, "..");
const distDir = join(siteRoot, "dist");
const docsDir = join(siteRoot, "src", "content", "docs");

const SITE = "https://jmrplens.github.io";
const BASE = "/gitlab-mcp-server";
const buildDate = new Date().toISOString().slice(0, 10);

// Map a sitemap URL to its source content file, or null if none is found.
function sourceFileFor(url) {
	let rel;
	try {
		rel = new URL(url).pathname;
	} catch {
		return null;
	}
	if (rel.startsWith(BASE)) rel = rel.slice(BASE.length);
	rel = rel.replace(/^\/+|\/+$/g, ""); // trim slashes → "tools/overview" | "es" | ""
	const base = rel === "" ? "index" : rel === "es" ? "es/index" : rel;
	for (const ext of [".mdx", ".md"]) {
		const candidate = join(docsDir, base + ext);
		if (existsSync(candidate)) return candidate;
	}
	return null;
}

// Git commit date (YYYY-MM-DD) for a file, or null if unavailable.
function gitDate(absPath) {
	try {
		const out = execFileSync(
			"git",
			["log", "-1", "--format=%cI", "--", absPath],
			{ cwd: repoRoot, encoding: "utf8" },
		).trim();
		return out ? out.slice(0, 10) : null;
	} catch {
		return null;
	}
}

function lastmodFor(url) {
	const src = sourceFileFor(url);
	if (!src) return buildDate;
	return (
		gitDate(src) ?? statSync(src).mtime.toISOString().slice(0, 10) ?? buildDate
	);
}

// Add <lastmod> after each <loc> that lacks one.
function stampSitemap(file) {
	const xml = readFileSync(file, "utf8");
	let changed = 0;
	const out = xml.replace(/<url>\s*<loc>([^<]+)<\/loc>/g, (match, loc) => {
		if (match.includes("<lastmod>")) return match;
		changed++;
		return `<url><loc>${loc}</loc><lastmod>${lastmodFor(loc)}</lastmod>`;
	});
	if (changed > 0) {
		writeFileSync(file, out);
		console.log(
			`[sitemap-lastmod] stamped ${changed} URLs in ${file.replace(distDir, "dist")}`,
		);
	}
}

if (!existsSync(distDir)) {
	console.warn("[sitemap-lastmod] dist/ not found — skipping");
	process.exit(0);
}

const sitemaps = readdirSync(distDir).filter(
	(f) => /^sitemap-\d+\.xml$/.test(f), // child sitemaps only, not sitemap-index.xml
);
if (sitemaps.length === 0) {
	console.warn("[sitemap-lastmod] no child sitemap found — skipping");
	process.exit(0);
}
for (const f of sitemaps) stampSitemap(join(distDir, f));
