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

// A reference definition's destination, bare or in angle brackets, on a line
// of its own behind any quote or list markers. A footnote's label opens with
// `^` and carries text, not a destination.
const DEFINITION =
	/^(?:[ \t]*(?:>|[-*+][ \t]|\d{1,9}[.)][ \t]))*[ \t]*\[(?!\^)(?:[^\]\\\n]|\\.)+\]:[ \t]*(?:<([^<>\n]*)>|([^\s<]\S*))/gm;

/**
 * The link and image targets of a markdown text that carry no scheme, the
 * destinations of reference definitions included.
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
		for (const match of bare.matchAll(DEFINITION)) {
			const target = match[1] ?? match[2];
			if (!SCHEME.test(target)) found.push(target);
		}
		prose = [];
	};
	for (const line of text.split("\n")) {
		const marker = /^[ \t]*(`{3,}|~{3,})(.*)$/.exec(line);
		if (fence !== null) {
			// A closing fence carries nothing after its marker but spaces or
			// tabs; a line such as ~~~text is code.
			if (
				marker &&
				marker[1][0] === fence[0] &&
				marker[1].length >= fence.length &&
				/^[ \t]*$/.test(marker[2])
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
