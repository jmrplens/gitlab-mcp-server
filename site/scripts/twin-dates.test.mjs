// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Unit tests for the date check of the twin check (scripts/twin-dates.mjs).
// Run with `pnpm run twins:test` (node --test), which the site lint runs too.
import assert from "node:assert/strict";
import { describe, test } from "node:test";

import { dateProblems } from "./twin-dates.mjs";

const twin = "about/index.md";

describe("dateProblems", () => {
	test("finds nothing when the three dates agree", () => {
		assert.deepEqual(
			dateProblems({
				twin,
				updated: "2026-11-20",
				lastmod: "2026-11-20",
				modified: "2026-11-20T21:04:00.000Z",
			}),
			[],
		);
	});

	test("finds nothing to compare when the twin states no date", () => {
		assert.deepEqual(dateProblems({ twin, lastmod: "2026-11-20" }), []);
	});

	// A page that opts out of its date with `lastUpdated: false` has a twin
	// with no Updated line, so a dated twin names a page that must date itself.
	test("names a dated page that lost its own date", () => {
		assert.deepEqual(
			dateProblems({ twin, updated: "2026-11-20", lastmod: "2026-11-20" }),
			[
				"/about/index.md says Updated: 2026-11-20, and the page's JSON-LD carries no dateModified",
			],
		);
	});

	test("names a sitemap date and a page date that disagree with the twin", () => {
		assert.deepEqual(
			dateProblems({
				twin,
				updated: "2026-11-20",
				lastmod: "2026-11-19",
				modified: "2026-11-21T00:30:00.000Z",
			}),
			[
				"/about/index.md says Updated: 2026-11-20, the sitemap says <lastmod>2026-11-19</lastmod>",
				"/about/index.md says Updated: 2026-11-20, the page's JSON-LD says dateModified 2026-11-21T00:30:00.000Z",
			],
		);
	});
});
