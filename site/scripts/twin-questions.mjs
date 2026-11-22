// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// The FAQ questions a markdown twin carries, for scripts/check-twins.mjs.
// Kept in a module of its own, with no side effects, so its unit tests can
// import it without running the check over a build.
import { FAQ_TITLES } from "../src/lib/page-markdown.mjs";

/**
 * The questions of the FAQ section a twin carries: the `###` headings under
 * the `##` heading FAQ.astro gives it, up to the next `##`. Fenced code is
 * skipped, so a heading inside a sample is not a question.
 *
 * @param {string} text the twin
 * @returns {string[]}
 */
export function twinQuestions(text) {
	const titles = new Set(Object.values(FAQ_TITLES).map((t) => `## ${t}`));
	const questions = [];
	let inFaq = false;
	let fence = "";
	for (const line of text.split("\n")) {
		const marker = /^\s*(`{3,}|~{3,})/.exec(line)?.[1];
		if (fence) {
			if (marker && marker[0] === fence[0] && marker.length >= fence.length)
				fence = "";
			continue;
		}
		if (marker) {
			fence = marker;
			continue;
		}
		if (/^## /.test(line)) inFaq = titles.has(line.trim());
		else if (inFaq && /^### /.test(line)) questions.push(line.slice(4).trim());
	}
	return questions;
}
