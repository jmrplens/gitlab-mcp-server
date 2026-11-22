// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Unit tests for the page dates (src/lib/page-dates.mjs).
// Run with `pnpm run twins:test` (node --test), which the site lint runs too.
import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { after, before, describe, test } from "node:test";

import { DATA, pageDate, pageInputs, sourceOf } from "./page-dates.mjs";

let root = "";

// A site directory with the files a page can import, so an import resolves
// to a file the way it does in the real site.
before(() => {
	root = mkdtempSync(join(tmpdir(), "page-dates-"));
	for (const dir of [
		"src/content/docs/es/reference/tools",
		"src/content/docs/operations",
		"src/data",
		"examples/telemetry",
	]) {
		mkdirSync(join(root, dir), { recursive: true });
	}
	for (const file of [
		"src/content/docs/index.mdx",
		"src/content/docs/es/index.mdx",
		"src/content/docs/about.mdx",
		"src/content/docs/es/reference/tools/index.mdx",
		"src/data/stats.json",
		"src/data/token-footprint.json",
		"src/data/home.ts",
		"examples/telemetry/compose.yaml",
	]) {
		writeFileSync(join(root, file), "");
	}
});

after(() => rmSync(root, { recursive: true, force: true }));

const PAGE = "src/content/docs/operations/telemetry.mdx";

describe("pageInputs", () => {
	test("names the data a page imports, the release it prints and the figures its FAQ names", () => {
		const source = [
			"---",
			"faq:",
			"  - q: How many?",
			'    a: "{{tools.free}} tools."',
			"---",
			'import { Code } from "@astrojs/starlight/components";',
			'import FAQ from "../../../components/FAQ.astro";',
			'import stats from "../../../data/stats.json";',
			'import compose from "../../../../examples/telemetry/compose.yaml?raw";',
			"",
			"Install %%VERSION%%.",
		].join("\n");
		assert.deepEqual(pageInputs(PAGE, source, root), [
			DATA.release,
			"examples/telemetry/compose.yaml",
			DATA.stats,
		]);
	});

	test("the landing reads its copy and both figure files", () => {
		const source =
			'import { es } from "../../../data/home";\n\n<Home content={es} />';
		assert.deepEqual(
			pageInputs("src/content/docs/es/index.mdx", source, root),
			[DATA.home, DATA.stats, DATA.footprint],
		);
	});

	test("an import that names no file, such as one in a code sample, is no input", () => {
		const source = '```js\nimport thing from "./not-here.json";\n```';
		assert.deepEqual(pageInputs(PAGE, source, root), []);
	});
});

describe("pageDate", () => {
	// 2026-10-31T00:30:00+01:00, which is the evening before in UTC: the day a
	// committer's local clock shows is not the day every reader of the date
	// agrees on.
	const nearMidnight = Date.UTC(2026, 9, 30, 23, 30) / 1000;

	test("is the newest commit among the page and what it reads, on its UTC day", () => {
		const times = new Map([
			[PAGE, Date.UTC(2026, 9, 5, 12) / 1000],
			[DATA.stats, nearMidnight],
			["src/data/home.ts", Date.UTC(2026, 10, 20) / 1000],
		]);
		const date = pageDate(
			PAGE,
			'import stats from "../../../data/stats.json";',
			{
				root,
				times,
			},
		);
		assert.equal(date?.day, "2026-10-30");
		assert.equal(date?.time.getTime(), nearMidnight * 1000);
	});

	test("a page whose data is older keeps its own date", () => {
		const times = new Map([
			[PAGE, Date.UTC(2026, 10, 2, 8) / 1000],
			[DATA.release, Date.UTC(2026, 9, 1) / 1000],
		]);
		assert.equal(
			pageDate(PAGE, "%%VERSION%%", { root, times })?.day,
			"2026-11-02",
		);
	});

	test("a page git knows nothing about has no date", () => {
		assert.equal(pageDate(PAGE, "", { root, times: new Map() }), undefined);
	});

	test("a page git does not track yet has no date, whatever its data's commits", () => {
		const times = new Map([[DATA.release, Date.UTC(2026, 9, 1) / 1000]]);
		assert.equal(pageDate(PAGE, "%%VERSION%%", { root, times }), undefined);
	});
});

describe("sourceOf", () => {
	test("resolves a page, a directory index and both landings", () => {
		assert.equal(sourceOf("about", root), "src/content/docs/about.mdx");
		assert.equal(
			sourceOf("es/reference/tools", root),
			"src/content/docs/es/reference/tools/index.mdx",
		);
		assert.equal(sourceOf("", root), "src/content/docs/index.mdx");
		assert.equal(sourceOf("es", root), "src/content/docs/es/index.mdx");
		assert.equal(sourceOf("nowhere", root), undefined);
	});
});
