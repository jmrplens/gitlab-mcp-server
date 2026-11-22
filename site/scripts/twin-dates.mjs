// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// The date check of scripts/check-twins.mjs: one date in three places, all
// read from src/lib/page-dates.mjs, namely the twin's Updated line, the
// sitemap's <lastmod> and the page's dateModified. Each used to read git its
// own way, and they disagreed by a day wherever a commit's local date was not
// its UTC one. Kept in a module of its own, with no side effects, so its unit
// tests can import it without running the check over a build.

/**
 * What disagrees among the dates of one page. A twin with no Updated line
 * states no date, so there is nothing to compare: that is the twin of a page
 * that opts out with `lastUpdated: false`, or of a checkout with no history.
 * A twin with one names a page Starlight dates, so the page's own date is
 * required, not merely compared when present.
 *
 * @param {object} page
 * @param {string} page.twin the twin's dist-relative path, named in a problem
 * @param {string} [page.updated] the twin's Updated line
 * @param {string} [page.lastmod] the sitemap's <lastmod> for the page
 * @param {unknown} [page.modified] the dateModified of the page's own JSON-LD node
 * @returns {string[]} one message per disagreement
 */
export function dateProblems({ twin, updated, lastmod, modified }) {
	if (!updated) return [];
	const problems = [];
	if (lastmod !== updated) {
		problems.push(
			`/${twin} says Updated: ${updated}, the sitemap says <lastmod>${lastmod ?? "nothing"}</lastmod>`,
		);
	}
	if (modified === undefined) {
		problems.push(
			`/${twin} says Updated: ${updated}, and the page's JSON-LD carries no dateModified`,
		);
	} else if (String(modified).slice(0, 10) !== updated) {
		problems.push(
			`/${twin} says Updated: ${updated}, the page's JSON-LD says dateModified ${modified}`,
		);
	}
	return problems;
}
