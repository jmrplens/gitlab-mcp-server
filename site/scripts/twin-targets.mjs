// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// The link targets of a markdown twin that a reader of the text cannot follow
// without knowing where the text came from, for scripts/check-twins.mjs. Kept
// in a module of its own, with no side effects, so its unit tests can import
// it without running the check over a build.
//
// Written again here rather than imported from the reducer, so a blind spot of
// the code that writes the targets is not also one of the check that reads
// them.
const SCHEME = /^[a-z][a-z0-9+.-]*:/i;

/**
 * The link and image targets of a markdown text that carry no scheme.
 *
 * @param {string} text
 * @returns {string[]}
 */
export function unanchoredTargets(text) {
	const found = [];
	let fence = null;
	let prose = [];
	const scan = () => {
		const bare = prose
			.join("\n")
			.replaceAll(
				/(?<!`)(`+)(?!`)(?:[^\n]|\n(?![ \t]*\n))*?(?<!`)\1(?!`)/g,
				"",
			);
		for (const match of bare.matchAll(/\]\(([^)\s]+)/g)) {
			if (!SCHEME.test(match[1])) found.push(match[1]);
		}
		for (const match of bare.matchAll(/\b(?:href|src)="([^"]*)"/g)) {
			if (!SCHEME.test(match[1])) found.push(match[1]);
		}
		prose = [];
	};
	for (const line of text.split("\n")) {
		const marker = /^[ \t]*(`{3,}|~{3,})/.exec(line);
		if (fence !== null) {
			if (
				marker &&
				marker[1][0] === fence[0] &&
				marker[1].length >= fence.length
			) {
				fence = null;
			}
			continue;
		}
		if (marker) {
			scan();
			fence = marker[1];
			continue;
		}
		prose.push(line);
	}
	scan();
	return found;
}
