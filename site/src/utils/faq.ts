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
//     published numbers cannot fall behind the real catalog;
//   - inline `code` and **bold**, kept for the rendered section and stripped for
//     the JSON-LD, which requires plain text.
import stats from "../data/stats.json";

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
 * Replaces `{{dotted.path}}` tokens with values from stats.json.
 * An unknown path is left verbatim so a typo shows up in the rendered page
 * instead of silently publishing an empty string.
 */
export function resolveStatTokens(text: string): string {
	return text.replace(/\{\{([\w.]+)\}\}/g, (whole, path: string) => {
		const value = path
			.split(".")
			.reduce<unknown>(
				(node, key) =>
					node && typeof node === "object"
						? (node as Record<string, unknown>)[key]
						: undefined,
				stats,
			);
		if (value === undefined || value === null) return whole;
		return typeof value === "number"
			? value.toLocaleString("en-US")
			: String(value);
	});
}

/** Splits an answer into inline spans for rendering. */
export function parseFaqSpans(answer: string): FaqSpan[] {
	const resolved = resolveStatTokens(answer);
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
 * stat tokens resolved, inline markers removed, content preserved.
 */
export function faqPlainText(answer: string): string {
	return (
		resolveStatTokens(answer)
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
