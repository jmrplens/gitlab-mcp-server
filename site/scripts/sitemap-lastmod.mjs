// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// The <lastmod> step of scripts/add-sitemap-lastmod.mjs: what each sitemap
// URL is stamped with, and the stamping itself. Kept in a module of its own,
// with no side effects, so its unit tests can import it without a build.

/**
 * The <lastmod> of one sitemap URL.
 *
 * @param {string | null | undefined} source the content file the URL is built from
 * @param {(file: string) => { day: string } | undefined} dates the page-date lookup
 * @param {string} buildDate the day of the build
 * @returns {string} the day to stamp
 */
export function lastmodOf(source, dates, buildDate) {
	return (source && dates(source)?.day) ?? buildDate;
}

/**
 * Adds <lastmod> to each <url> that lacks one. Matching the whole <url>…</url>
 * block (not just <loc>) keeps this idempotent: if an entry already carries a
 * <lastmod>, it is left untouched rather than getting a duplicate.
 *
 * @param {string} xml a child sitemap
 * @param {(loc: string) => string} lastmodFor the day for one URL
 * @returns {{ xml: string, changed: number }} the sitemap and how many URLs it stamped
 */
export function stampLastmods(xml, lastmodFor) {
	let changed = 0;
	const out = xml.replace(/<url>[\s\S]*?<\/url>/g, (block) => {
		if (block.includes("<lastmod>")) return block;
		return block.replace(/<loc>([^<]+)<\/loc>/, (locMatch, loc) => {
			changed++;
			return `<loc>${loc}</loc><lastmod>${lastmodFor(loc)}</lastmod>`;
		});
	});
	return { xml: out, changed };
}
