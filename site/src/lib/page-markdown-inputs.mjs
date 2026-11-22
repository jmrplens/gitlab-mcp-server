// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// What the markdown twins are rendered from besides the pages themselves,
// loaded once per build: the component labels in both locales, the counted
// figures, the published release and the date each page last changed.
//
// Paths are taken from the site directory the caller names rather than from
// this module's own URL, because inside the build this module is bundled into
// a chunk under dist/, where a URL relative to it names nothing.
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { join } from "node:path";

import { readRelease } from "./version-token.mjs";

/** @param {string} path @returns {any} */
const json = (path) => JSON.parse(readFileSync(path, "utf8"));

/**
 * The date each content file last changed, as git records it: the same
 * source Starlight's `lastUpdated` reads, so the twin's `Updated:` line and
 * the page's dateModified name one day. A checkout without history, or no git
 * at all, gives an empty map and twins without the line, which is optional.
 *
 * @param {string} root the site directory
 * @returns {Map<string, string>} site-relative path to YYYY-MM-DD (UTC)
 */
export function lastChanged(root) {
	/** @type {Map<string, string>} */
	const dates = new Map();
	let log = "";
	try {
		log = execFileSync(
			"git",
			[
				"log",
				"--format=t:%ct",
				"--name-only",
				"--no-renames",
				"--relative",
				"--",
				"src/content/docs",
			],
			{ cwd: root, encoding: "utf8", maxBuffer: 64 * 1024 * 1024 },
		);
	} catch {
		return dates;
	}
	let day = "";
	for (const line of log.split("\n")) {
		const stamp = /^t:(\d+)$/.exec(line);
		if (stamp) {
			day = new Date(Number(stamp[1]) * 1000).toISOString().slice(0, 10);
		} else if (line && day && !dates.has(line)) {
			dates.set(line, day);
		}
	}
	return dates;
}

/**
 * @param {string} root the site directory
 * @returns {{ labels: Record<"en" | "es", Record<string, string>>, stats: any, footprint: any, release: string, updated: Map<string, string> }}
 */
export function loadInputs(root) {
	return {
		labels: {
			en: json(join(root, "src/content/i18n/en.json")),
			es: json(join(root, "src/content/i18n/es.json")),
		},
		stats: json(join(root, "src/data/stats.json")),
		footprint: json(join(root, "src/data/token-footprint.json")),
		release: readRelease(join(root, "..", "server.json")),
		updated: lastChanged(root),
	};
}
