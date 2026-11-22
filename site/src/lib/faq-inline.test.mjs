// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Unit tests for the FAQ inline markup (src/lib/faq-inline.mjs).
// Run with `pnpm run twins:test` (node --test), which the site lint runs too.
import assert from "node:assert/strict";
import { describe, test } from "node:test";

import { inlineSpans, plainInline } from "./faq-inline.mjs";

describe("inlineSpans", () => {
	test("splits code, bold and links out of the text around them", () => {
		assert.deepEqual(
			inlineSpans("Set `--max-http-clients`, **not** [this](/a/) one"),
			[
				{ type: "text", value: "Set " },
				{ type: "code", value: "--max-http-clients" },
				{ type: "text", value: ", " },
				{ type: "bold", value: "not" },
				{ type: "text", value: " " },
				{ type: "link", value: "this", href: "/a/" },
				{ type: "text", value: " one" },
			],
		);
	});

	// Backticks bind tighter than emphasis in Markdown.
	test("bold markers inside a code span stay code", () => {
		assert.deepEqual(inlineSpans("`**literal**`"), [
			{ type: "code", value: "**literal**" },
		]);
	});

	test("text without markup is one text span, and none is no span", () => {
		assert.deepEqual(inlineSpans("How much memory?"), [
			{ type: "text", value: "How much memory?" },
		]);
		assert.deepEqual(inlineSpans(""), []);
	});
});

describe("plainInline", () => {
	// schema.org Question.name and Answer.text are plain text: a question that
	// writes a flag as code is advertised without the backticks.
	test("drops the markers and keeps what they mark", () => {
		assert.equal(
			plainInline("Is `--max-http-clients` how I size the memory?"),
			"Is --max-http-clients how I size the memory?",
		);
		assert.equal(
			plainInline("**Yes**, see [the page](/sizing/)."),
			"Yes, see the page.",
		);
	});

	test("text without markup comes back unchanged", () => {
		assert.equal(
			plainInline("How many calls a second?"),
			"How many calls a second?",
		);
	});
});
