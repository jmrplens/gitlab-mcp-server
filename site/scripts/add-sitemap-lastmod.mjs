// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Injects <lastmod> into the generated sitemap.
//
// Starlight's built-in sitemap (via @astrojs/sitemap) emits <loc> entries with
// no <lastmod>, so crawlers — including AI crawlers — can't tell when a page last
// changed. This post-build step maps each sitemap URL back to its source content
// file and stamps the day that page last changed as <lastmod>. Runs after
// `astro build` (see package.json `postbuild`).
//
// The day comes from src/lib/page-dates.mjs, the same source the page's footer,
// its dateModified and its markdown twin read: the newest commit among the page
// and the data it shows, as a UTC day. A page git does not date (one not
// committed yet, or a checkout with no history) states no date on the page or
// in its twin, so its URL gets no <lastmod> either; only a URL with no source
// file falls back to the build date. In a shallow CI checkout Git dates may
// collapse to the latest commit; the deploy workflow uses fetch-depth: 0 so
// per-file dates are accurate.
import { existsSync, readdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

import { pageDates, sourceOf } from "../src/lib/page-dates.mjs";
import { lastmodOf, stampLastmods } from "./sitemap-lastmod.mjs";

const here = dirname(fileURLToPath(import.meta.url));
const siteRoot = join(here, "..");
const distDir = join(siteRoot, "dist");

const SITE = "https://docs.jmrp.io";
const BASE = "/gitlab-mcp-server";
const buildDate = new Date().toISOString().slice(0, 10);
const dates = pageDates(siteRoot);

// Map a sitemap URL to its source content file, relative to the site, or null
// if none is found or the URL is not on this site's origin (so unrelated URLs
// are never stamped). A section index such as /reference/tools/ is built from
// reference/tools/index.mdx, which sourceOf resolves like any other page.
function sourceFileFor(url) {
	let parsed;
	try {
		parsed = new URL(url);
	} catch {
		return null;
	}
	if (parsed.origin !== SITE) return null;
	let rel = parsed.pathname;
	if (rel.startsWith(BASE)) rel = rel.slice(BASE.length);
	rel = rel.replace(/^\/+|\/+$/g, ""); // trim slashes → "tools/overview" | "es" | ""
	return sourceOf(rel, siteRoot) ?? null;
}

const lastmodFor = (url) => lastmodOf(sourceFileFor(url), dates, buildDate);

function stampSitemap(file) {
	const { xml: out, changed } = stampLastmods(
		readFileSync(file, "utf8"),
		lastmodFor,
	);
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

// Match sitemap.xml and sitemap-<n>.xml, but never the sitemap index.
const sitemaps = readdirSync(distDir).filter(
	(f) => /^sitemap(-\d+)?\.xml$/.test(f) && f !== "sitemap-index.xml",
);
if (sitemaps.length === 0) {
	console.warn("[sitemap-lastmod] no child sitemap found — skipping");
	process.exit(0);
}
for (const f of sitemaps) stampSitemap(join(distDir, f));
