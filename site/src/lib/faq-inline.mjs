// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// The inline markup an FAQ question or answer may write in the `faq:`
// frontmatter: `code`, **bold** and [links](/target/). The visible section
// (src/components/FAQ.astro, through src/utils/faq.ts) renders it as
// elements, the FAQPage JSON-LD (src/components/Head.astro) and the twin
// check (scripts/check-twins.mjs) read it as the plain text schema.org asks
// for. Plain JavaScript with no data imports, so node --test can load it.

/**
 * One inline span: literal text, inline code, bold, or a link.
 *
 * @typedef {object} InlineSpan
 * @property {"text" | "code" | "bold" | "link"} type
 * @property {string} value the text a reader sees, markers removed
 * @property {string} [href]
 * @property {InlineSpan[]} [children] the spans of a bold or a link whose
 *   text holds markup itself, such as **`api`**
 */

/**
 * Splits text into inline spans for rendering.
 *
 * @param {string} text with any stat tokens already resolved
 * @returns {InlineSpan[]}
 */
export function inlineSpans(text) {
	/** @type {InlineSpan[]} */
	const spans = [];
	// Inline code first: backticks bind tighter than emphasis in Markdown, so a
	// `**literal**` inside code must not be treated as bold.
	const pattern = /`([^`]+)`|\*\*([^*]+)\*\*|\[([^\]]+)\]\(([^)]+)\)/g;
	let last = 0;
	for (let m = pattern.exec(text); m !== null; m = pattern.exec(text)) {
		if (m.index > last) {
			spans.push({ type: "text", value: text.slice(last, m.index) });
		}
		if (m[1] !== undefined) {
			spans.push({ type: "code", value: m[1] });
		} else if (m[2] !== undefined) {
			spans.push({ type: "bold", ...nested(m[2]) });
		} else {
			spans.push({ type: "link", ...nested(m[3]), href: m[4] });
		}
		last = m.index + m[0].length;
	}
	if (last < text.length) {
		spans.push({ type: "text", value: text.slice(last) });
	}
	return spans;
}

/**
 * The text of a bold or a link, with the spans it holds when it holds markup
 * of its own: a scope written as **`api`** is bold code, not bold text
 * between backticks.
 *
 * @param {string} text
 * @returns {{ value: string, children?: InlineSpan[] }}
 */
function nested(text) {
	const children = inlineSpans(text);
	if (children.every((span) => span.type === "text")) return { value: text };
	return { value: children.map((span) => span.value).join(""), children };
}

/**
 * The text with its inline markers removed and what they mark kept: a link
 * keeps its text and drops its URL, since schema.org's Question.name and
 * Answer.text are prose, and a bare Markdown target there reads as noise to
 * an extractor. Read from the spans the page renders, so a code span keeps
 * the markers written inside it, as the visible question shows them.
 *
 * @param {string} text with any stat tokens already resolved
 * @returns {string}
 */
export function plainInline(text) {
	return inlineSpans(text)
		.map((span) => span.value)
		.join("");
}
