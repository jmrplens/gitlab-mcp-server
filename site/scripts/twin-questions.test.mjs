// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Unit tests for the FAQ reader of the twin check (scripts/twin-questions.mjs).
// Run with `pnpm run twins:test` (node --test), which the site lint runs too.
import assert from "node:assert/strict";
import { describe, test } from "node:test";

import { twinQuestions } from "./twin-questions.mjs";

describe("twinQuestions", () => {
	test("reads the questions under the FAQ heading, up to the next section", () => {
		const twin = [
			"## Setup",
			"### Not a question",
			"## Frequently asked questions",
			"### How do I start?",
			"### Does it work offline?",
			"## Next steps",
			"### Not one either",
		].join("\n");
		assert.deepEqual(twinQuestions(twin), [
			"How do I start?",
			"Does it work offline?",
		]);
	});

	test("skips a heading inside a fenced sample", () => {
		const twin = [
			"## Frequently asked questions",
			"### How do I start?",
			"````markdown",
			"```",
			"### Inside the sample",
			"```",
			"````",
			"### Does it work offline?",
		].join("\n");
		assert.deepEqual(twinQuestions(twin), [
			"How do I start?",
			"Does it work offline?",
		]);
	});

	// CommonMark closes a fence only on a marker run followed by nothing but
	// spaces or tabs, so a line that carries more is a line of the sample.
	test("keeps a fence open on a marker line that carries text after it", () => {
		const twin = [
			"## Frequently asked questions",
			"### How do I start?",
			"```markdown",
			"```not-a-close",
			"### Inside the sample",
			"```  ",
			"### Does it work offline?",
		].join("\n");
		assert.deepEqual(twinQuestions(twin), [
			"How do I start?",
			"Does it work offline?",
		]);
	});
});
