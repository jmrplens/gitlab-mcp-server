// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Unit tests for the FAQ stat tokens (src/lib/stat-tokens.mjs).
// Run with `pnpm run twins:test` (node --test), which the site lint runs too.
import assert from "node:assert/strict";
import { describe, test } from "node:test";

import { numberLocale, resolveStatTokens } from "./stat-tokens.mjs";

const stats = { tools: { gitlab_com: 1102, free: 875 }, name: "catalog" };

describe("resolveStatTokens", () => {
	test("formats a figure in the page's locale", () => {
		assert.equal(
			resolveStatTokens("{{tools.gitlab_com}}", stats, "en"),
			"1,102",
		);
		assert.equal(
			resolveStatTokens("{{tools.gitlab_com}}", stats, "es"),
			"1102",
		);
	});

	test("prints text as it is and leaves an unknown path as written", () => {
		assert.equal(
			resolveStatTokens(
				"{{name}} of {{tools.free}}, {{tools.typo}}",
				stats,
				"es",
			),
			"catalog of 875, {{tools.typo}}",
		);
	});

	test("a language the site does not publish formats en-US", () => {
		assert.equal(numberLocale("fr"), "en-US");
	});
});
