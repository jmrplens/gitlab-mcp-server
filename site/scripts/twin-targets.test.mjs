// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Unit tests for the target scan of the twin check (scripts/twin-targets.mjs).
// Run with `pnpm run twins:test` (node --test), which the site lint runs too.
import assert from "node:assert/strict";
import { describe, test } from "node:test";

import { unanchoredTargets } from "./twin-targets.mjs";

describe("unanchoredTargets", () => {
	test("finds inline targets and attributes with no scheme, outside code", () => {
		const text = [
			"[A](/gitlab-mcp-server/about/) [B](https://example.com)",
			'<a href="#x">C</a> <img src="https://example.com/i.png">',
			"`[D](/in/a/span/)`",
			"```",
			"[E](/in/a/fence/)",
			"```",
		].join("\n");
		assert.deepEqual(unanchoredTargets(text), [
			"/gitlab-mcp-server/about/",
			"#x",
		]);
	});

	test("finds a reference definition's destination with no scheme", () => {
		const text = [
			"[A][a] [B][b] [C][c] [D][d]",
			"",
			'[a]: /gitlab-mcp-server/about/ "About"',
			"[b]: <./img.png>",
			"> [c]: ../relative/",
			"[d]: https://example.com/",
			"[^1]: A footnote, not a link.",
			"```",
			"[e]: /in/a/fence/",
			"```",
		].join("\n");
		assert.deepEqual(unanchoredTargets(text), [
			"/gitlab-mcp-server/about/",
			"./img.png",
			"../relative/",
		]);
	});

	test("a fence line with text after its marker does not close a fence", () => {
		const text = ["~~~md", "~~~text", "~~~", "[A](/relative/)"].join("\n");
		assert.deepEqual(unanchoredTargets(text), ["/relative/"]);
	});

	test("a fence in a blockquote is code, and ends with its quote", () => {
		const text = [
			"> ~~~md",
			"> [A](/in/a/quoted/fence/)",
			"> > ~~~",
			"> [B](/still/in/the/fence/)",
			"> ~~~",
			"> [C](/quoted/prose/)",
			"",
			"> ```",
			"> [D](/in/an/unclosed/quoted/fence/)",
			"[E](/after/the/quote/)",
			"```",
			"> ```",
			"> [F](/in/a/fence/)",
			"```",
		].join("\n");
		assert.deepEqual(unanchoredTargets(text), [
			"/quoted/prose/",
			"/after/the/quote/",
		]);
	});
});
