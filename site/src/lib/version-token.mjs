/**
 * The release a page names: the one a reader can install today, which is the
 * top-level `version` of the repository's server.json.
 *
 * A page that means "the release you install today" (an install command, an
 * image tag, a download URL, a package pin, sample `--version` output, "the
 * current release is") writes VERSION_TOKEN instead of the number, and the
 * build replaces it with the published release. Nobody edits a page when the
 * release changes, and the number is read where it lives rather than copied
 * into the site, so no copy can fall behind it.
 *
 * Why server.json and not the VERSION file: VERSION names the release being
 * prepared, and moves when the preparation starts (3.1.0: moved 2026-09-17,
 * tagged 2026-09-30), so a command built from it names a release nobody can
 * download yet. server.json's version is stamped by
 * scripts/update-server-json-sha.sh in the release workflow's last job, once
 * the release, its images and its npm, PyPI and NuGet packages are published,
 * and that commit reaches main with a deploy key, so it runs the workflows a
 * push runs and the Pages workflow redeploys the site on it.
 *
 * The replacement runs as the first remark plugin, on the Markdown tree of
 * every page before anything renders it: prose, inline code, fenced code
 * blocks and their meta string (Expressive Code is a rehype plugin, so it
 * receives the block already substituted), table cells, link and image URLs
 * and titles, the string attributes of MDX components, and the string and
 * template literals of an MDX expression or attribute expression. It walks
 * every string the tree holds rather than a list of node types, so a node
 * type nobody thought of is covered too, and the token is chosen so that it
 * cannot occur by accident: `%` means nothing to Markdown, MDX or YAML inside
 * a line, and no command, URL or text in the documentation writes two of them
 * around a word.
 *
 * Frontmatter is the one place the token is refused rather than replaced. The
 * content layer parses it outside the Markdown pipeline and caches the result
 * by the file's own digest, so a new release would leave a cached title or
 * description on the old one, and site/scripts/gen-llms.mjs copies title and
 * description verbatim. site/scripts/check-version-token.mjs fails on the
 * token there, and on the token anywhere in the built site.
 */
import { readFileSync } from "node:fs";

/** What a page writes where it means the current release. */
export const VERSION_TOKEN = "%%VERSION%%";

/** The repository's server.json, two levels above site/src/lib. */
export const RELEASE_FILE = new URL("../../../server.json", import.meta.url);

/**
 * A release as server.json spells it: three numbers, optionally a pre-release
 * and build suffix. Anything else there (no version, a leading `v`, a stray
 * space) would be published on every page, so it stops the build.
 */
const RELEASE = /^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$/;

/** A file's path for a message, whether it was given as a URL or a string. */
const shown = (file) => (file instanceof URL ? file.pathname : file);

/**
 * Reads the published release, the top-level `version` of server.json.
 * Throws, naming the file, when it cannot be read, is not JSON or does not
 * hold a release: a build that cannot say which release it documents has
 * nothing correct to publish.
 */
export function readRelease(file = RELEASE_FILE) {
	let text;
	try {
		text = readFileSync(file, "utf8");
	} catch (error) {
		throw new Error(
			`[version-token] cannot read ${shown(file)}: ${error.message}`,
			{ cause: error },
		);
	}
	let manifest;
	try {
		manifest = JSON.parse(text);
	} catch (error) {
		throw new Error(
			`[version-token] ${shown(file)} is not JSON: ${error.message}`,
			{ cause: error },
		);
	}
	const version = manifest?.version;
	if (typeof version !== "string" || !RELEASE.test(version)) {
		throw new Error(
			`[version-token] ${shown(file)} carries version ${JSON.stringify(version ?? null)}, which is not a release such as 3.2.0`,
		);
	}
	return version;
}

/** The text with every token replaced by version. */
export function replaceVersionToken(text, version) {
	return text.split(VERSION_TOKEN).join(version);
}

/**
 * Replaces the token in every string reachable from value, in place, and
 * returns how many strings changed. Objects and arrays are walked; anything
 * else that is not a string is left alone. A node reached twice is visited
 * once, so a shared object or a cycle cannot loop.
 */
export function substituteVersionToken(value, version) {
	const seen = new WeakSet();
	let changed = 0;
	const visit = (node) => {
		if (node === null || typeof node !== "object" || seen.has(node)) return;
		seen.add(node);
		for (const key of Object.keys(node)) {
			const child = node[key];
			if (typeof child === "string") {
				if (child.includes(VERSION_TOKEN)) {
					node[key] = replaceVersionToken(child, version);
					changed++;
				}
			} else {
				visit(child);
			}
		}
	};
	visit(value);
	return changed;
}

/**
 * The remark plugin. `version` defaults to the published release, read once
 * when the plugin is attached rather than once per page.
 */
export function remarkVersionToken(options = {}) {
	const version = options.version ?? readRelease();
	return (tree) => {
		substituteVersionToken(tree, version);
	};
}
