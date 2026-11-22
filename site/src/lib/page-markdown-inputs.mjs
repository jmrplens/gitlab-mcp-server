// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// What the markdown twins are rendered from besides the pages themselves,
// loaded once per build: the component labels in both locales, the counted
// figures, the published release and the date each page last changed.
//
// Paths are taken from the site directory the caller names rather than from
// this module's own URL, because inside the build this module is bundled into
// a chunk under dist/, where a URL relative to it names nothing.
import { readFileSync } from "node:fs";
import { join } from "node:path";

import { pageDates } from "./page-dates.mjs";
import { readRelease } from "./version-token.mjs";

/** @param {string} path @returns {any} */
const json = (path) => JSON.parse(readFileSync(path, "utf8"));

/**
 * @param {string} root the site directory
 * @returns {{ labels: Record<"en" | "es", Record<string, string>>, stats: any, footprint: any, release: string, updated: (file: string) => string | undefined }}
 */
export function loadInputs(root) {
	const dates = pageDates(root);
	return {
		labels: {
			en: json(join(root, "src/content/i18n/en.json")),
			es: json(join(root, "src/content/i18n/es.json")),
		},
		stats: json(join(root, "src/data/stats.json")),
		footprint: json(join(root, "src/data/token-footprint.json")),
		release: readRelease(join(root, "..", "server.json")),
		// The UTC day the page or the data it shows last changed: the day its
		// footer, its dateModified and the sitemap state too, all three read
		// from src/lib/page-dates.mjs. A checkout without history gives none,
		// and the twin's `Updated:` line is optional.
		updated: (file) => dates(file)?.day,
	};
}
