// Holds the documentation to the one way it names the current release.
//
// A page writes %%VERSION%% where it means the release you install today, and
// the build replaces it with the published release, the top-level version of
// the repository's server.json (src/lib/version-token.mjs). Three things can
// still go wrong, and each has a check here:
//
//   SOURCES (the default, run by `pnpm run lint`)
//     - The token in frontmatter. The content layer parses frontmatter outside
//       the Markdown pipeline and caches it by the file's digest, and
//       gen-llms.mjs copies title and description verbatim, so the build would
//       publish the token, or a release it read on an earlier run.
//     - A literal release in a shape that means the current one: an install
//       command, an image tag, a package pin, a download URL, a tag ref, sample
//       `--version` or /health output, or a sentence naming the current or
//       latest release. That is the hand-maintained number the token replaced,
//       and it goes stale on the next release. The shapes are judged whatever
//       the number, so a stale 3.0.0 is caught as well as today's release. A
//       literal kept on purpose (a measurement, a condition about one release)
//       is declared in version-literals.mjs with its category and reason, and
//       a declaration that matches nothing, lacks either, or is a condition
//       about a release server.json has moved past fails too.
//     - `{stats.version}`, the copy of the release stats.json carries, which
//       is only as current as the last `make gen-site-stats`.
//
//   DIST (`--dist`, run after every build; also run by the default mode when
//   dist/ exists)
//     - The token anywhere in the built site, the llms indexes included: some
//       place the substitution did not reach, such as an MDX expression that
//       builds a string at runtime.
//     - A page whose source wrote the token but whose <main> carries the
//       release fewer times than the source wrote the token and the number
//       together, which is what a substitution that replaced it with the
//       wrong thing would look like. Only <main> is counted, because every
//       page prints the release in the JSON-LD of its <head> whatever its body
//       holds.
//
// Run: node scripts/check-version-token.mjs [--dist]
import { existsSync, readFileSync, readdirSync, statSync } from "node:fs";
import { dirname, join, relative, sep } from "node:path";
import { fileURLToPath } from "node:url";

import { VERSION_TOKEN, readRelease } from "../src/lib/version-token.mjs";
import { LITERAL_RELEASES } from "./version-literals.mjs";

const here = dirname(fileURLToPath(import.meta.url));
const siteDir = join(here, "..");
const docsDir = join(siteDir, "src", "content", "docs");
const distDir = join(siteDir, "dist");

const SEMVER = String.raw`\d+\.\d+\.\d+`;

/**
 * What may stand between "the current release" and its number: spacing,
 * punctuation, bold markers and the verbs and articles that join them in
 * English and Spanish ("is", "es la", "sea la"). Anything else, such as "the
 * latest version of Node, 24.18.0", names some other thing's version.
 */
const BETWEEN = String.raw`(?:[\s,:*(]|is|es|sea|la){0,8}`;

/**
 * The shapes in which a release number means the current release. Each is
 * matched against one line at a time, so a sentence wrapped across lines is
 * judged by the line holding the number.
 */
const CURRENT_RELEASE_SHAPES = [
	{ name: "download URL", re: new RegExp(`releases/download/v${SEMVER}`, "g") },
	{ name: "tag ref", re: new RegExp(`refs/tags/v${SEMVER}`, "g") },
	{ name: "image tag", re: new RegExp(`gitlab-mcp-server:${SEMVER}`, "g") },
	{
		name: "package pin",
		re: new RegExp(`gitlab-mcp-server[@=]=?${SEMVER}`, "g"),
	},
	{
		name: "--version output",
		re: new RegExp(`gitlab-mcp-server v?${SEMVER} \\(commit`, "g"),
	},
	{
		name: "installer pin",
		re: new RegExp(`\\bVERSION\\s*=\\s*['"]?v${SEMVER}`, "g"),
	},
	{
		// `winget install jmrplens.gitlab-mcp-server --version 3.2.0`,
		// `dotnet tool install -g gitlab-mcp-server --version 3.2.0`: a pin
		// written as an option rather than glued to the package name.
		name: "--version pin",
		re: new RegExp(
			`gitlab-mcp-server\\b[^\`]*?--version[= ]['"]?v?${SEMVER}`,
			"g",
		),
	},
	{
		name: "server output",
		re: new RegExp(`"(?:version|build)":\\s*"v?${SEMVER}`, "g"),
	},
	{
		name: "current release",
		re: new RegExp(
			`\\b(?:current|latest|newest) (?:release|version)${BETWEEN}v?${SEMVER}`,
			"gi",
		),
	},
	{
		name: "current release",
		re: new RegExp(
			`\\b(?:release|versi[oó]n) (?:actual|vigente|m[aá]s nueva)${BETWEEN}v?${SEMVER}`,
			"gi",
		),
	},
	{
		name: "current release",
		re: new RegExp(
			`[uú]ltima (?:release|versi[oó]n)${BETWEEN}v?${SEMVER}`,
			"gi",
		),
	},
];

/** Every file under dir, recursively, as absolute paths. */
function walk(dir, keep) {
	const out = [];
	for (const entry of readdirSync(dir)) {
		const path = join(dir, entry);
		if (statSync(path).isDirectory()) out.push(...walk(path, keep));
		else if (keep(entry)) out.push(path);
	}
	return out;
}

/** A path under base, with "/" separators. */
const rel = (base, path) => relative(base, path).split(sep).join("/");

/** The frontmatter block of a page, delimiters included, or null. */
function frontmatter(text) {
	const match = /^---\r?\n([\s\S]*?)\r?\n---/.exec(text);
	return match ? match[0] : null;
}

/** The problems with one page's source, and the declarations it used. */
function checkPage(page, text) {
	const problems = [];
	const used = new Set();
	const lines = text.split("\n");
	const front = frontmatter(text);
	const frontLines = front ? front.split("\n").length : 0;

	lines.forEach((line, index) => {
		const where = `${page}:${index + 1}`;
		if (index < frontLines && line.includes(VERSION_TOKEN)) {
			problems.push(
				`${where}: frontmatter carries ${VERSION_TOKEN}, which the build replaces only in the page body: frontmatter is parsed and cached outside the Markdown pipeline. Write the release in the body.`,
			);
		}
		if (line.includes("{stats.version}")) {
			problems.push(
				`${where}: {stats.version} prints the copy of VERSION make gen-site-stats writes into stats.json, which names the release being prepared. Write ${VERSION_TOKEN}, which the build replaces with the published release in server.json.`,
			);
		}
		for (const { name, re } of CURRENT_RELEASE_SHAPES) {
			for (const match of line.matchAll(re)) {
				const release = /\d+\.\d+\.\d+/.exec(match[0])[0];
				const declared = LITERAL_RELEASES.find(
					(d) =>
						d.page === page &&
						line.includes(d.text) &&
						d.text.includes(release),
				);
				if (declared) {
					used.add(declared);
					continue;
				}
				problems.push(
					`${where}: ${name} with a literal release (${match[0]}). Write ${VERSION_TOKEN} where the page means the release you install today; if this number records a measurement or a condition about that one release, declare it in scripts/version-literals.mjs with its category and reason.`,
				);
			}
		}
	});
	return { problems, used };
}

/** Every source page under docsDir, keyed by its path under it. */
function sourcePages() {
	return walk(docsDir, (entry) => /\.mdx?$/.test(entry)).map((path) => ({
		page: rel(docsDir, path),
		text: readFileSync(path, "utf8"),
	}));
}

/**
 * Why a literal may stay. A measurement is a fact about the release it was
 * taken on and stays true; a condition names the release that is the latest
 * one and is only true until the next, so it expires when server.json's
 * published release moves past it.
 */
const CATEGORIES = new Set(["measurement", "condition"]);

/** The three numbers of a release, for ordering two of them. */
const releaseParts = (release) =>
	/^(\d+)\.(\d+)\.(\d+)/.exec(release).slice(1, 4).map(Number);

/** Whether release a comes before release b, pre-release suffixes ignored. */
function releaseBefore(a, b) {
	const [x, y] = [releaseParts(a), releaseParts(b)];
	for (let i = 0; i < 3; i++) {
		if (x[i] !== y[i]) return x[i] < y[i];
	}
	return false;
}

/**
 * The problems with the declarations themselves: each one says which page and
 * line it excuses, why, and under which category, and a condition about a
 * release the published one has moved past no longer holds.
 */
function checkDeclarations(version) {
	const problems = [];
	for (const declaration of LITERAL_RELEASES) {
		const where = `scripts/version-literals.mjs: the declaration for ${declaration.page} (${JSON.stringify(declaration.text)})`;
		const blank = ["page", "text", "reason"].filter(
			(key) => typeof declaration[key] !== "string" || !declaration[key].trim(),
		);
		if (blank.length) {
			problems.push(`${where} has no ${blank.join(", ")}.`);
		}
		if (!CATEGORIES.has(declaration.category)) {
			problems.push(
				`${where} has category ${JSON.stringify(declaration.category)}; it must be one of ${[...CATEGORIES].join(", ")}.`,
			);
		}
		const release = /\d+\.\d+\.\d+/.exec(declaration.text ?? "")?.[0];
		if (!release) {
			problems.push(`${where} names no release in its text.`);
		} else if (
			declaration.category === "condition" &&
			releaseBefore(release, version)
		) {
			problems.push(
				`${where} is a condition about ${release} being the latest release, and the published release in server.json is ${version}: the condition no longer holds. Rewrite the sentence for the current release and remove the declaration.`,
			);
		}
	}
	return problems;
}

function checkSources() {
	const problems = checkDeclarations(readRelease());
	const used = new Set();
	for (const { page, text } of sourcePages()) {
		const result = checkPage(page, text);
		problems.push(...result.problems);
		for (const declaration of result.used) used.add(declaration);
	}
	for (const declaration of LITERAL_RELEASES) {
		if (!used.has(declaration)) {
			problems.push(
				`scripts/version-literals.mjs: the declaration for ${declaration.page} (${JSON.stringify(declaration.text)}) matches no literal release on that page; remove it.`,
			);
		}
	}
	return problems;
}

/** The built page a source page becomes: "install/npm.mdx" is install/npm/index.html. */
function builtPage(page) {
	const slug = page.replace(/\.mdx?$/, "").replace(/(^|\/)index$/, "");
	return join(distDir, ...(slug ? slug.split("/") : []), "index.html");
}

function checkDist() {
	if (!existsSync(distDir)) {
		return [`${rel(siteDir, distDir)}/ does not exist; build the site first.`];
	}
	const problems = [];
	const token = Buffer.from(VERSION_TOKEN);
	for (const path of walk(distDir, () => true)) {
		const bytes = readFileSync(path);
		if (bytes.includes(token)) {
			problems.push(
				`dist/${rel(distDir, path)}: carries ${VERSION_TOKEN}, which the build should have replaced. Something renders it outside the Markdown tree (frontmatter, or a string an MDX expression builds); write the release where the remark plugin reaches it.`,
			);
		}
	}
	const version = readRelease();
	for (const { page, text } of sourcePages()) {
		const body = text.slice((frontmatter(text) ?? "").length);
		const tokens = count(body, VERSION_TOKEN);
		if (!tokens) continue;
		const built = builtPage(page);
		if (!existsSync(built)) {
			problems.push(
				`${page}: writes ${VERSION_TOKEN}, and dist/${rel(distDir, built)} was not built.`,
			);
			continue;
		}
		const content = mainContent(readFileSync(built, "utf8"));
		if (content === null) {
			problems.push(
				`dist/${rel(distDir, built)}: has no <main> element, so nothing shows where ${page} wrote ${VERSION_TOKEN}.`,
			);
			continue;
		}
		// Every page also prints the release in the SoftwareApplication JSON-LD
		// of its <head>, and a page may write the same number literally as
		// history, so only a count inside <main> shows that each token became
		// the release. The count may come out higher, never lower: a code
		// block's copy button repeats its code in data-code.
		const expected = tokens + count(body, version);
		const found = count(content, version);
		if (found < expected) {
			problems.push(
				`dist/${rel(distDir, built)}: carries ${version} ${found} time(s) in <main>, fewer than the ${expected} ${page} writes as ${VERSION_TOKEN} and as ${version} together.`,
			);
		}
	}
	return problems;
}

/** How many times needle occurs in text. */
function count(text, needle) {
	return text.split(needle).length - 1;
}

/** The part of a built page inside its <main> element, or null without one. */
function mainContent(html) {
	const start = html.indexOf("<main");
	const end = html.lastIndexOf("</main>");
	return start === -1 || end < start ? null : html.slice(start, end);
}

function main() {
	const distOnly = process.argv.includes("--dist");
	const problems = [];
	if (!distOnly) problems.push(...checkSources());
	if (distOnly || existsSync(distDir)) problems.push(...checkDist());
	if (problems.length) {
		console.error("check-version-token FAILED:");
		for (const problem of problems) console.error(`  ${problem}`);
		process.exit(1);
	}
	console.log(
		distOnly
			? `[version-token] the built site carries no ${VERSION_TOKEN}`
			: `[version-token] every page names the current release as ${VERSION_TOKEN}, ${LITERAL_RELEASES.length} literal release(s) declared`,
	);
}

try {
	main();
} catch (error) {
	console.error(error.message);
	process.exit(1);
}
