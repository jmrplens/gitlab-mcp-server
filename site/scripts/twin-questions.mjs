// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// The FAQ questions a markdown twin carries, for scripts/check-twins.mjs.
// Kept in a module of its own, with no side effects, so its unit tests can
// import it without running the check over a build.
import { FAQ_TITLES } from "../src/lib/page-markdown.mjs";

// A list item's marker after its indentation: a bullet, or up to nine digits
// and a period or a parenthesis, then spaces or tabs or the end of the line.
const LIST_ITEM = /^([ \t]*)([-*+]|\d{1,9}[.)])([ \t]+|$)/;

/**
 * The column a run of spaces and tabs ends at when it starts at `from`, a tab
 * advancing to the next multiple of four as CommonMark counts it.
 *
 * @param {string} space
 * @param {number} [from]
 * @returns {number}
 */
function columnAfter(space, from = 0) {
	let column = from;
	for (const char of space) {
		column = char === "\t" ? column + 4 - (column % 4) : column + 1;
	}
	return column;
}

/**
 * The questions of the FAQ section a twin carries: the `###` headings under
 * the `##` heading FAQ.astro gives it, up to the next `##`. Fenced code is
 * skipped, so a heading inside a sample is not a question. A fence opens and
 * closes as CommonMark has it: on a marker indented at most three columns
 * past the content of the list item it sits in (column 0 outside a list), so
 * a marker indented further is code; and it closes on a run of its marker at
 * least as long as the one that opened it, followed by nothing but spaces or
 * tabs. Unlike page-markdown.mjs's maskCode, which reads MDX, where the limit
 * does not apply.
 *
 * @param {string} text the twin
 * @returns {string[]}
 */
export function twinQuestions(text) {
	const titles = new Set(Object.values(FAQ_TITLES).map((t) => `## ${t}`));
	const questions = [];
	let inFaq = false;
	/** @type {{ marker: string, base: number } | null} */
	let fence = null;
	// The content columns of the list items open at this line, innermost last.
	const items = [];
	for (const line of text.split("\n")) {
		const [, indent = "", marker, rest = ""] =
			/^([ \t]*)(`{3,}|~{3,})(.*)/.exec(line) ?? [];
		if (fence) {
			if (
				marker &&
				marker[0] === fence.marker[0] &&
				marker.length >= fence.marker.length &&
				/^[ \t]*$/.test(rest) &&
				columnAfter(indent) - fence.base < 4
			)
				fence = null;
			continue;
		}
		if (line.trim()) {
			const column = columnAfter(/^[ \t]*/.exec(line)?.[0] ?? "");
			while (items.length > 0 && column < (items.at(-1) ?? 0)) items.pop();
			const base = items.at(-1) ?? 0;
			if (marker && column - base < 4) {
				fence = { marker, base };
				continue;
			}
			const item = LIST_ITEM.exec(line);
			if (item && column - base < 4) {
				const end = column + item[2].length;
				const gap = columnAfter(item[3], end) - end;
				items.push(end + (gap >= 1 && gap <= 4 ? gap : 1));
			}
		}
		if (/^## /.test(line)) inFaq = titles.has(line.trim());
		else if (inFaq && /^### /.test(line)) questions.push(line.slice(4).trim());
	}
	return questions;
}
