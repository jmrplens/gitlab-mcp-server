// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Unit tests for the sitemap's <lastmod> step (scripts/sitemap-lastmod.mjs).
// Run with `pnpm run twins:test` (node --test), which the site lint runs too.
import assert from "node:assert/strict";
import { describe, test } from "node:test";

import { lastmodOf, stampLastmods } from "./sitemap-lastmod.mjs";

const buildDate = "2026-11-22";
const dates = (/** @type {string} */ file) =>
	file === "src/content/docs/about.mdx" ? { day: "2026-11-20" } : undefined;

describe("lastmodOf", () => {
	test("stamps a dated page with its day", () => {
		assert.equal(
			lastmodOf("src/content/docs/about.mdx", dates, buildDate),
			"2026-11-20",
		);
	});

	// The page, its dateModified and its twin carry no date for a page git
	// does not date, so the sitemap states none either.
	test("stamps nothing for a page git does not date", () => {
		assert.equal(
			lastmodOf("src/content/docs/new.mdx", dates, buildDate),
			undefined,
		);
	});

	test("stamps a URL that resolves to no page with the build date", () => {
		assert.equal(lastmodOf(null, dates, buildDate), buildDate);
	});
});

describe("stampLastmods", () => {
	const xml = [
		"<urlset>",
		"<url><loc>https://x/a/</loc></url>",
		"<url><loc>https://x/b/</loc></url>",
		"<url><loc>https://x/c/</loc><lastmod>2026-01-01</lastmod></url>",
		"</urlset>",
	].join("");

	test("stamps each URL that lacks a date and leaves an undated one bare", () => {
		const out = stampLastmods(xml, (loc) =>
			loc.endsWith("/a/") ? "2026-11-20" : undefined,
		);
		assert.equal(out.changed, 1);
		assert.equal(
			out.xml,
			[
				"<urlset>",
				"<url><loc>https://x/a/</loc><lastmod>2026-11-20</lastmod></url>",
				"<url><loc>https://x/b/</loc></url>",
				"<url><loc>https://x/c/</loc><lastmod>2026-01-01</lastmod></url>",
				"</urlset>",
			].join(""),
		);
	});
});
