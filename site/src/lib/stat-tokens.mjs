// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// The `{{dotted.path}}` tokens an FAQ answer writes for a figure of
// src/data/stats.json, resolved for the three places that print the answer:
// the visible FAQ section (src/components/FAQ.astro, through
// src/utils/faq.ts), the FAQPage JSON-LD (src/components/Head.astro, the same
// way) and the markdown twin (src/lib/page-markdown.mjs).
//
// A number is formatted in the page's locale, as the body of the same page
// formats its own figures (`stats.tools.gitlab_com.toLocaleString("es-ES")`
// on a Spanish page): formatted en-US everywhere, a Spanish page said "1102"
// in its prose and "1,102" in its answers.

/** The number format of each locale the site publishes. */
export const NUMBER_LOCALES = { en: "en-US", es: "es-ES" };

/**
 * @param {string} lang the page's language, "en" or "es"
 * @returns {string} the locale its numbers are formatted in
 */
export function numberLocale(lang) {
	return (
		NUMBER_LOCALES[/** @type {keyof typeof NUMBER_LOCALES} */ (lang)] ??
		NUMBER_LOCALES.en
	);
}

/**
 * Replaces every `{{dotted.path}}` token with its value in stats. An unknown
 * path is left as written, so a typo shows on the page instead of silently
 * publishing an empty string.
 *
 * @param {string} text
 * @param {unknown} stats src/data/stats.json
 * @param {string} lang the page's language
 * @returns {string}
 */
export function resolveStatTokens(text, stats, lang) {
	return text.replace(/\{\{([\w.]+)\}\}/g, (whole, path) => {
		/** @type {unknown} */
		let value = stats;
		for (const key of path.split(".")) {
			value =
				value && typeof value === "object"
					? /** @type {Record<string, unknown>} */ (value)[key]
					: undefined;
		}
		if (value === undefined || value === null) return whole;
		return typeof value === "number"
			? value.toLocaleString(numberLocale(lang))
			: String(value);
	});
}
