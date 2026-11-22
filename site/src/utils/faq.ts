// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Shared FAQ helpers.
//
// A page's FAQ lives in one place — the `faq:` frontmatter array — and feeds two
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
//   - inline `code` and **bold**, kept for the rendered section and stripped for
//     the JSON-LD, which requires plain text.
import stats from "../data/stats.json";
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
	const resolved = resolveStatTokens(answer, lang);
	const spans: FaqSpan[] = [];
	// Inline code first: backticks bind tighter than emphasis in Markdown, so a
	// `**literal**` inside code must not be treated as bold.
	const pattern = /`([^`]+)`|\*\*([^*]+)\*\*|\[([^\]]+)\]\(([^)]+)\)/g;
	let last = 0;
	for (let m = pattern.exec(resolved); m !== null; m = pattern.exec(resolved)) {
		if (m.index > last) {
			spans.push({ type: "text", value: resolved.slice(last, m.index) });
		}
		if (m[1] !== undefined) {
			spans.push({ type: "code", value: m[1] });
		} else if (m[2] !== undefined) {
			spans.push({ type: "bold", value: m[2] });
		} else {
			spans.push({ type: "link", value: m[3] as string, href: m[4] as string });
		}
		last = m.index + m[0].length;
	}
	if (last < resolved.length) {
		spans.push({ type: "text", value: resolved.slice(last) });
	}
	return spans;
}

/**
 * Renders an answer as the plain text required by schema.org Answer.text:
 * stat tokens resolved for a page in `lang`, inline markers removed, content
 * preserved.
 */
export function faqPlainText(answer: string, lang: string): string {
	return (
		resolveStatTokens(answer, lang)
			.replace(/`([^`]+)`/g, "$1")
			.replace(/\*\*([^*]+)\*\*/g, "$1")
			// Keep the link text, drop the URL: schema.org Answer.text is prose, and a
			// bare Markdown target inside it reads as noise to an extractor.
			.replace(/\[([^\]]+)\]\(([^)]+)\)/g, "$1")
	);
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
