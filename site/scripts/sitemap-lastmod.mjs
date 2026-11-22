// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// The <lastmod> step of scripts/add-sitemap-lastmod.mjs: what each sitemap
// URL is stamped with, and the stamping itself. Kept in a module of its own,
// with no side effects, so its unit tests can import it without a build.

/**
 * The <lastmod> of one sitemap URL: the day of the page it is built from, as
 * the page's dateModified and its twin's Updated line state it. A page git
 * does not date (one not committed yet, or a checkout with no history) states
 * no date anywhere, so it has none here either; only a URL that resolves to
 * no page of this site is stamped with the build date.
 *
 * @param {string | null | undefined} source the content file the URL is built from
 * @param {(file: string) => { day: string } | undefined} dates the page-date lookup
 * @param {string} buildDate the day of the build
 * @returns {string | undefined} the day to stamp, or none
 */
export function lastmodOf(source, dates, buildDate) {
	return source ? dates(source)?.day : buildDate;
}

/**
 * Adds <lastmod> to each <url> that lacks one and has a day, leaving a URL
 * with none as it was. Matching the whole <url>…</url> block (not just <loc>)
 * keeps this idempotent: if an entry already carries a <lastmod>, it is left
 * untouched rather than getting a duplicate.
 *
 * @param {string} xml a child sitemap
 * @param {(loc: string) => string | undefined} lastmodFor the day for one URL
 * @returns {{ xml: string, changed: number }} the sitemap and how many URLs it stamped
 */
export function stampLastmods(xml, lastmodFor) {
	let changed = 0;
	const out = xml.replace(/<url>[\s\S]*?<\/url>/g, (block) => {
		if (block.includes("<lastmod>")) return block;
		return block.replace(/<loc>([^<]+)<\/loc>/, (locMatch, loc) => {
			const lastmod = lastmodFor(loc);
			if (!lastmod) return locMatch;
			changed++;
			return `<loc>${loc}</loc><lastmod>${lastmod}</lastmod>`;
		});
	});
	return { xml: out, changed };
}
