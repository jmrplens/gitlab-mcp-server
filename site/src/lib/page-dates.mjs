// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// The date a page last changed, one answer for the three places that state
// it: the twin's `Updated:` line, the page's "Last updated" footer with its
// JSON-LD dateModified (both read Starlight's `lastUpdated`, which
// src/routeData.ts sets from here), and the sitemap's <lastmod>
// (scripts/add-sitemap-lastmod.mjs). They used to be three readings of git
// that disagreed: the twin took the commit's UTC day, the sitemap the
// committer's local day, and none of them looked past the MDX file.
//
// A page changes when what it shows changes, and much of what a page shows
// is not in its MDX: the counts come from src/data/stats.json, the token
// figures from src/data/token-footprint.json, the collector example from the
// YAML files it imports raw, the release every install command names from
// ../server.json, and the landing's copy from src/data/home.ts. So a page's
// date is the newest commit among its own file and the files it reads, and
// the day is always the UTC one.
//
// Paths are relative to the site directory the caller names, written with
// forward slashes: "src/content/docs/about.mdx", "../server.json".
import { execFileSync } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";
import { join, posix, relative, sep } from "node:path";

/** What a page can read besides itself, as paths relative to the site. */
export const DATA = {
	stats: "src/data/stats.json",
	footprint: "src/data/token-footprint.json",
	home: "src/data/home.ts",
	release: "../server.json",
};

// Every path a page's date can depend on, so one `git log` answers them all.
const WATCHED = ["src/content/docs", "src/data", "../examples", DATA.release];

/** @param {string} path @returns {string} the path with forward slashes */
const slashed = (path) => path.split(sep).join("/");

/**
 * The newest commit time of every file under the watched paths, from one
 * `git log` pass. A checkout without history, or no git at all, gives an
 * empty map, and every page then has no date.
 *
 * @param {string} root the site directory
 * @returns {Map<string, number>} site-relative path to seconds since the epoch
 */
export function commitTimes(root) {
	/** @type {Map<string, number>} */
	const times = new Map();
	let top = "";
	let log = "";
	try {
		top = execFileSync("git", ["rev-parse", "--show-toplevel"], {
			cwd: root,
			encoding: "utf8",
		}).trim();
		log = execFileSync(
			"git",
			[
				"log",
				"--format=t:%ct",
				"--name-only",
				"--no-renames",
				"--",
				...WATCHED,
			],
			{ cwd: root, encoding: "utf8", maxBuffer: 64 * 1024 * 1024 },
		);
	} catch {
		return times;
	}
	let time = 0;
	for (const line of log.split("\n")) {
		const stamp = /^t:(\d+)$/.exec(line);
		if (stamp) {
			time = Number(stamp[1]);
			continue;
		}
		if (!line || !time) continue;
		// git names files from the top of the repository; the log is newest
		// first, so the first time seen for a file is its newest.
		const path = slashed(relative(root, join(top, line)));
		if (!times.has(path)) times.set(path, time);
	}
	return times;
}

/**
 * The data files a page reads, from its source (frontmatter included): the
 * relative imports that are not components, the release when it writes the
 * version token, stats.json when an FAQ answer names a figure, and the
 * landing's copy and figures when it imports the landing's data. A path that
 * names no file is left out, so a sample in a code block that looks like an
 * import costs nothing.
 *
 * @param {string} file the page, relative to the site
 * @param {string} source its full text
 * @param {string} root the site directory
 * @returns {string[]} site-relative paths, without the page itself
 */
export function pageInputs(file, source, root) {
	/** @type {Set<string>} */
	const inputs = new Set();
	const dir = posix.dirname(slashed(file));
	for (const match of source.matchAll(
		/^import\s[^\n]*?\sfrom\s+["'](\.{1,2}\/[^"']+)["']/gm,
	)) {
		const specifier = match[1].replace(/\?raw$/, "");
		if (specifier.endsWith(".astro")) continue;
		const path = posix.normalize(posix.join(dir, specifier));
		if (path === posix.normalize(DATA.home).replace(/\.ts$/, "")) {
			inputs.add(DATA.home).add(DATA.stats).add(DATA.footprint);
		} else if (existsSync(join(root, path))) {
			inputs.add(path);
		}
	}
	if (source.includes("%%VERSION%%")) inputs.add(DATA.release);
	if (/\{\{[\w.]+\}\}/.test(source)) inputs.add(DATA.stats);
	return [...inputs].sort();
}

/**
 * The newest commit among a page and its inputs.
 *
 * @param {string} file the page, relative to the site
 * @param {string} source its full text
 * @param {{ root: string, times: Map<string, number> }} context
 * @returns {{ time: Date, day: string } | undefined} the moment and its UTC day
 */
export function pageDate(file, source, { root, times }) {
	// Starlight dates only a page git tracks, and src/routeData.ts redates
	// only what Starlight dated, so a page git does not know yet has no date
	// here either, whatever its data's commits.
	if (!times.has(slashed(file))) return undefined;
	let newest = 0;
	for (const path of [slashed(file), ...pageInputs(file, source, root)]) {
		newest = Math.max(newest, times.get(path) ?? 0);
	}
	if (!newest) return undefined;
	const time = new Date(newest * 1000);
	return { time, day: time.toISOString().slice(0, 10) };
}

/** @type {Map<string, (file: string) => ReturnType<typeof pageDate>>} */
const dated = new Map();

/**
 * A page-date lookup for one site directory, reading git once and each page
 * once however many times it is asked: the build asks for every page from the
 * route middleware and again from the twin endpoint.
 *
 * @param {string} root the site directory
 * @returns {(file: string) => { time: Date, day: string } | undefined}
 */
export function pageDates(root) {
	const known = dated.get(root);
	if (known) return known;
	const times = commitTimes(root);
	/** @type {Map<string, ReturnType<typeof pageDate>>} */
	const cache = new Map();
	const lookup = (/** @type {string} */ file) => {
		if (!cache.has(file)) {
			const path = join(root, file);
			const source = existsSync(path) ? readFileSync(path, "utf8") : "";
			cache.set(file, pageDate(file, source, { root, times }));
		}
		return cache.get(file);
	};
	dated.set(root, lookup);
	return lookup;
}

/**
 * The content file a page route is built from: "tools/overview" is
 * tools/overview.mdx, "reference/tools" is reference/tools/index.mdx, "" the
 * English landing and "es" the Spanish one.
 *
 * @param {string} route the page's path below the site base, without slashes
 * @param {string} root the site directory
 * @returns {string | undefined} the file, relative to the site
 */
export function sourceOf(route, root) {
	const base = route ? `src/content/docs/${route}` : "src/content/docs";
	const candidates = route
		? [`${base}.mdx`, `${base}.md`, `${base}/index.mdx`, `${base}/index.md`]
		: [`${base}/index.mdx`, `${base}/index.md`];
	return candidates.find((candidate) => existsSync(join(root, candidate)));
}
