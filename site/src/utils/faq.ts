// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Shared FAQ helpers.
//
// A page's FAQ lives in one place, the `faq:` frontmatter array, and feeds two
// consumers that must never disagree: the visible FAQ section (components/FAQ.astro)
// and the FAQPage JSON-LD (components/Head.astro). Before this, each page carried
// the questions twice, hand-written in both places, and they had already drifted:
// /tools/meta-tools/ rendered five questions in the body and advertised a
// different five in its structured data.
//
// Answers may contain:
//   - stat tokens, `{{tools.free}}`, resolved from src/data/stats.json so the
//     published numbers cannot fall behind the real catalog, and formatted in
//     the page's locale as its body formats its own figures
//     (src/lib/stat-tokens.mjs, which the markdown twin reads too);
//   - inline `code`, **bold** and links, kept for the rendered section and
//     stripped for the JSON-LD, which requires plain text
//     (src/lib/faq-inline.mjs, which the twin check reads too).
//
// Questions take the same inline markup and no stat tokens.
import stats from "../data/stats.json";
import { inlineSpans, plainInline } from "../lib/faq-inline.mjs";
import { resolveStatTokens as resolveTokens } from "../lib/stat-tokens.mjs";

export interface FaqItem {
	q: string;
	a: string;
}

/** One inline span of an answer: literal text, inline code, bold, or a link. */
export interface FaqSpan {
	type: "text" | "code" | "bold" | "link";
	value: string;
	href?: string;
}

/**
 * Replaces `{{dotted.path}}` tokens with values from stats.json, a number in
 * the number format of `lang`, the page's language. An unknown path is left
 * verbatim so a typo shows up in the rendered page instead of silently
 * publishing an empty string.
 */
export function resolveStatTokens(text: string, lang: string): string {
	return resolveTokens(text, stats, lang);
}

/** Splits an answer into inline spans for rendering on a page in `lang`. */
export function parseFaqSpans(answer: string, lang: string): FaqSpan[] {
	return inlineSpans(resolveStatTokens(answer, lang));
}

/**
 * Splits a question into inline spans for its heading. A question takes the
 * same inline markup as an answer, so a flag written as code renders as code
 * rather than between literal backticks; it takes no stat tokens, which the
 * markdown twin does not resolve in a heading either.
 */
export function parseFaqQuestionSpans(question: string): FaqSpan[] {
	return inlineSpans(question);
}

/**
 * Renders an answer as the plain text required by schema.org Answer.text:
 * stat tokens resolved for a page in `lang`, inline markers removed, content
 * preserved.
 */
export function faqPlainText(answer: string, lang: string): string {
	return plainInline(resolveStatTokens(answer, lang));
}

/**
 * Renders a question as the plain text required by schema.org Question.name:
 * inline markers removed, content preserved.
 */
export function faqPlainQuestion(question: string): string {
	return plainInline(question);
}

/** Slugifies a question into a stable heading anchor. */
export function faqSlug(question: string): string {
	return question
		.toLowerCase()
		.normalize("NFD")
		.replace(/[\u0300-\u036f]/g, "")
		.replace(/[^a-z0-9]+/g, "-")
		.replace(/^-+|-+$/g, "");
}
