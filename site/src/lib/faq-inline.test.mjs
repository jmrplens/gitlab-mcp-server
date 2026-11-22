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

	// A scope written as **`api`** and a link whose text is code are markup
	// inside markup: the bold or the link holds the code as a child span, so
	// the page shows code rather than backticks, and its value is the text.
	test("code inside bold or a link is a child span", () => {
		assert.deepEqual(inlineSpans("scope **`api`** or [`icons.go`](/i/)"), [
			{ type: "text", value: "scope " },
			{
				type: "bold",
				value: "api",
				children: [{ type: "code", value: "api" }],
			},
			{ type: "text", value: " or " },
			{
				type: "link",
				value: "icons.go",
				href: "/i/",
				children: [{ type: "code", value: "icons.go" }],
			},
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

	// Backticks bind tighter than emphasis, so the plain text keeps what the
	// visible question shows inside a code span, markers included.
	test("markup inside a code span stays literal", () => {
		assert.equal(plainInline("Is `**literal**` bold?"), "Is **literal** bold?");
		assert.equal(plainInline("Run `[a](b)` or **c**"), "Run [a](b) or c");
	});

	test("code inside bold or a link loses its backticks", () => {
		assert.equal(
			plainInline("The scope **`api`**, see [`icons.go`](/i/)."),
			"The scope api, see icons.go.",
		);
	});

	test("text without markup comes back unchanged", () => {
		assert.equal(
			plainInline("How many calls a second?"),
			"How many calls a second?",
		);
	});
});
