#!/usr/bin/env node

import { execFileSync } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";
import path from "node:path";

const repoRoot = execFileSync("git", ["rev-parse", "--show-toplevel"], {
	encoding: "utf8",
}).trim();
const trackedDocs = [
	...new Set(
		execFileSync(
			"git",
			["ls-files", "*.md", "*.mdx", ":(glob)**/*.md", ":(glob)**/*.mdx"],
			{
				cwd: repoRoot,
				encoding: "utf8",
			},
		)
			.trim()
			.split("\n")
			.filter(Boolean),
	),
]
	.filter((file) => !file.startsWith("plan/"))
	.filter((file) => !file.startsWith(".github/skills/"));

// ---------------------------------------------------------------------------
// Anchors
//
// One file is read in one place, and the place decides how its headings are
// named. A file under site/src/content is a page of the documentation site,
// rendered by Starlight; every other tracked file is read in the repository,
// rendered by GitHub. The two agree on almost every heading in this corpus and
// disagree in ways that are easy to hit by accident, so each is taught here
// rather than one being made to stand for both.
// ---------------------------------------------------------------------------

// GitHub's anchors come from html-pipeline's TableOfContentsFilter: the text is
// downcased, every character outside Ruby's \p{Word} plus hyphen and space is
// dropped, the spaces become hyphens, and a repeat takes "-N" from a counter
// kept per original slug. Ruby's \p{Word} is Alphabetic, Mark, Digit,
// Connector_Punctuation and Join_Control, which is why the two zero-width
// joiners of an emoji sequence survive in a GitHub anchor and nowhere else.
const gitHubPunctuation =
	/[^\p{L}\p{M}\p{Nd}\p{Nl}\p{Pc}\p{Join_Control}\- ]/gu;

// Starlight's come from Astro's rehypeHeadingIds, which is github-slugger: the
// same shape without Join_Control, and a repeat resolved by walking up from
// "-1" until a free name is found rather than by a counter. The two orders part
// company on a page whose headings are "Foo", "Foo" and "Foo 1": GitHub hands
// the third the "foo-1" it already gave the second, github-slugger gives it
// "foo-1-1".
const sluggerPunctuation = /[^\p{L}\p{M}\p{Nd}\p{Nl}\p{Pc}\- ]/gu;

const gitHubDialect = {
	name: "GitHub",
	// A page read in the repository has only the anchors its own headings and
	// HTML make.
	pageAnchors: [],
	newSlugger() {
		const counts = new Map();
		return (text) => {
			const id = text
				.toLowerCase()
				.replace(gitHubPunctuation, "")
				.replaceAll(" ", "-");
			const seen = counts.get(id) ?? 0;
			counts.set(id, seen + 1);
			return seen > 0 ? `${id}-${seen}` : id;
		};
	},
};

const starlightDialect = {
	name: "Starlight",
	// Starlight gives every page a "_top" anchor, on the <h1> it renders from
	// the frontmatter title. That title is not a Markdown heading and never
	// becomes a slug, which is why a site page is linked by "#_top" and a
	// repository file by the slug of the "# Title" it carries in its body.
	pageAnchors: ["_top"],
	newSlugger() {
		const occurrences = new Map();
		return (text) => {
			const original = text
				.toLowerCase()
				.replace(sluggerPunctuation, "")
				.replaceAll(" ", "-");
			let result = original;
			while (occurrences.has(result)) {
				const next = (occurrences.get(original) ?? 0) + 1;
				occurrences.set(original, next);
				result = `${original}-${next}`;
			}
			occurrences.set(result, 0);
			return result;
		};
	},
};

const siteContentRoot = path.join(repoRoot, "site", "src", "content") + path.sep;

const anchorCache = new Map();

const issues = [];
let anchorsChecked = 0;

for (const file of trackedDocs) {
	const absoluteFile = path.join(repoRoot, file);
	const content = readFileSync(absoluteFile, "utf8");

	eachProseLine(content, (line, lineNumber) => {
		for (const target of inlineLinks(line)) {
			checkTarget(file, lineNumber, target);
		}

		const reference = line.match(/^\s*\[(?!\^)[^\]]+\]:\s*(<[^>]+>|\S+)/);
		if (reference) {
			checkTarget(file, lineNumber, reference[1]);
		}
	});
}

if (issues.length > 0) {
	console.error("Broken local documentation links:");
	for (const issue of issues) {
		console.error(
			`- ${issue.file}:${issue.line} -> ${issue.target} (${issue.reason})`,
		);
	}
	process.exit(1);
}

console.log(
	`Checked ${trackedDocs.length} Markdown/MDX files; local links are valid, ` +
		`including ${anchorsChecked} anchors.`,
);

// eachProseLine visits the lines a reader is shown as prose, which is every
// line outside a fenced code block.
//
// A fence closes on the marker that opened it, at the same length or longer,
// and on nothing else. The toggle this replaced flipped on any line starting a
// fence, so the ``` inside a ```` block closed it and every line after that was
// read in the opposite state: in .github/instructions/*.md, where Markdown
// samples are nested that way, whole sections were scanned as prose or skipped
// as code depending on how many inner fences had gone by.
function eachProseLine(content, visit) {
	const lines = content.split("\n");
	let fence = null;

	for (let index = 0; index < lines.length; index++) {
		const line = lines[index].replace(/\r$/, "");
		const marker = line.match(/^ {0,3}(`{3,}|~{3,})(.*)$/);

		if (fence) {
			if (
				marker &&
				marker[1][0] === fence.char &&
				marker[1].length >= fence.length &&
				marker[2].trim() === ""
			) {
				fence = null;
			}
			continue;
		}

		if (marker) {
			fence = { char: marker[1][0], length: marker[1].length };
			continue;
		}

		visit(line, index + 1);
	}
}

function inlineLinks(line) {
	const targets = [];
	const pattern = /!?\[[^\]]*\]\(\s*(<[^>]+>|[^)\s]+)(?:\s+"[^"]*")?\s*\)/g;
	let match;
	const prose = withoutCodeSpans(line);
	while ((match = pattern.exec(prose)) !== null) {
		targets.push(match[1]);
	}
	return targets;
}

// withoutCodeSpans blanks the contents of every backtick span, keeping the line
// the same length so a reported column still points at the right place.
//
// Text inside a code span is quoted, not linked: prose about a formatter that
// must not hand-write `[%s](%s)` was read as a link to a file named `%s` and
// failed this check, which is the checker being wrong about Markdown rather
// than the sentence being wrong about the code.
function withoutCodeSpans(line) {
	return line.replace(/(`+)([^`]|[^`][\s\S]*?[^`])\1(?!`)/g, (span, fence, body) =>
		fence + " ".repeat(body.length) + fence,
	);
}

function checkTarget(file, line, rawTarget) {
	const target = normalizeTarget(rawTarget);
	if (shouldSkipTarget(target)) {
		return;
	}

	const hash = target.indexOf("#");
	const fragment = hash === -1 ? "" : target.slice(hash + 1);
	const withoutFragment = (hash === -1 ? target : target.slice(0, hash)).replace(
		/\?.*$/,
		"",
	);

	// A bare "#anchor" points into the file the link is written in.
	if (withoutFragment === "") {
		checkFragment(file, line, target, path.join(repoRoot, file), fragment);
		return;
	}

	const decoded = safeDecodeURIComponent(withoutFragment);
	const basePath = path.resolve(
		path.dirname(path.join(repoRoot, file)),
		decoded,
	);
	const candidates = [
		basePath,
		`${basePath}.md`,
		`${basePath}.mdx`,
		path.join(basePath, "README.md"),
		path.join(basePath, "index.md"),
		path.join(basePath, "index.mdx"),
	];

	const resolved = candidates.find((candidate) => existsSync(candidate));
	if (!resolved) {
		issues.push({ file, line, target, reason: "no such file" });
		return;
	}

	checkFragment(file, line, target, resolved, fragment);
}

// checkFragment holds the other half of a link, the half that used to be
// dropped: the anchor. A link to a heading that was renamed, or that never
// existed, resolved its file and passed.
//
// Only a Markdown target is judged. A fragment on anything else is a line
// reference into source a renderer numbers for itself, and nothing here can
// read it.
function checkFragment(file, line, target, resolvedPath, fragment) {
	if (fragment === "" || !/\.mdx?$/i.test(resolvedPath)) {
		return;
	}

	const anchor = safeDecodeURIComponent(fragment);
	anchorsChecked++;

	if (anchorsFor(resolvedPath).has(anchor)) {
		return;
	}

	issues.push({
		file,
		line,
		target,
		reason:
			`no ${dialectFor(resolvedPath).name} anchor #${anchor} ` +
			`in ${path.relative(repoRoot, resolvedPath)}`,
	});
}

function normalizeTarget(rawTarget) {
	return rawTarget.trim().replace(/^<|>$/g, "");
}

// shouldSkipTarget names what this checker cannot answer for. A scheme is
// somebody else's server; a root-relative path is a route on the documentation
// site rather than a file here, and the site validates its own routes and their
// anchors at build time through starlight-links-validator.
//
// A "#anchor" is deliberately absent from this list: it is the one shape that
// is always answerable, since the file it points into is the file it is
// written in.
function shouldSkipTarget(target) {
	if (target === "") {
		return true;
	}
	if (/^(?:[a-z][a-z0-9+.-]*:|\/)/i.test(target)) {
		return true;
	}
	if (/^(?:url|link|path|file|filename|directory|fn)$/i.test(target)) {
		return true;
	}
	if (/^[<{].*[>}]$/.test(target)) {
		return true;
	}
	return false;
}

function safeDecodeURIComponent(value) {
	try {
		return decodeURIComponent(value);
	} catch {
		return value;
	}
}

function dialectFor(absolutePath) {
	return absolutePath.startsWith(siteContentRoot)
		? starlightDialect
		: gitHubDialect;
}

function anchorsFor(absolutePath) {
	let anchors = anchorCache.get(absolutePath);
	if (!anchors) {
		anchors = collectAnchors(absolutePath);
		anchorCache.set(absolutePath, anchors);
	}
	return anchors;
}

function collectAnchors(absolutePath) {
	const dialect = dialectFor(absolutePath);
	const anchors = new Set(dialect.pageAnchors);
	const slug = dialect.newSlugger();
	const content = withoutFrontmatter(readFileSync(absolutePath, "utf8"));
	// The line before the current one, when it could be the text of a heading
	// underlined in the Setext style.
	let underlinable = null;

	eachProseLine(content, (line) => {
		for (const id of explicitIds(line)) {
			anchors.add(id);
		}

		const atx = line.match(/^ {0,3}#{1,6}(?:[ \t]+(.*?))?[ \t]*$/);
		if (atx) {
			anchors.add(slug(headingText(atx[1] ?? "")));
			underlinable = null;
			return;
		}

		const underline = line.match(/^ {0,3}(=+|-+)[ \t]*$/);
		if (
			underline &&
			underlinable !== null &&
			// A row of dashes under a line carrying pipes is a table, not a
			// heading; a row of equals signs is never anything else.
			(underline[1][0] === "=" || !underlinable.includes("|"))
		) {
			anchors.add(slug(headingText(underlinable)));
			underlinable = null;
			return;
		}

		underlinable = isParagraphLine(line) ? line.trim() : null;
	});

	return anchors;
}

// withoutFrontmatter drops the YAML block a page opens with. It carries the
// title Starlight renders the <h1> from, and GitHub draws it as a table: a
// heading in neither, and a "title: x" line above the closing "---" that would
// otherwise read as a Setext heading in both.
function withoutFrontmatter(content) {
	const frontmatter = content.match(
		/^---[ \t]*\r?\n[\s\S]*?\r?\n(?:---|\.\.\.)[ \t]*(?:\r?\n|$)/,
	);
	return frontmatter ? content.slice(frontmatter[0].length) : content;
}

// headingText renders a heading's source the way a reader sees it, and only as
// far as the slug can tell the difference. The characters a slugger keeps are
// letters, digits, underscores, hyphens and spaces, so a construct matters here
// exactly when dropping its markup changes one of those: a link's URL, an
// image's URL, a reference label, a tag name and the text of a comment all
// disappear on the page and would otherwise be slugged. Backticks, asterisks
// and tildes need no handling at all, since the slugger drops them wherever
// they stand.
function headingText(raw) {
	const withoutComments = stripRepeatedly(
		raw.replace(/[ \t]+#+[ \t]*$/, ""),
		/<!--[\s\S]*?-->/g,
	);
	const withoutLinks = withoutComments
		.replace(/!\[([^\]]*)\]\([^)]*\)/g, "$1")
		.replace(/\[([^\]]*)\]\((?:[^()]|\([^()]*\))*\)/g, "$1")
		.replace(/\[([^\]]*)\]\[[^\]]*\]/g, "$1");
	return (
		stripRepeatedly(
			withoutLinks,
			/<\/?[A-Za-z][A-Za-z0-9-]*(?:\s[^<>]*)?\/?>/g,
		)
			// Underscores are kept by both sluggers, so emphasis written with
			// them has to go while the ones inside a name like
			// audit_md_escaping stay. Only a pair on a word boundary is
			// emphasis.
			.replace(/(?<![\p{L}\p{N}_])__?([^_]+?)__?(?![\p{L}\p{N}_])/gu, "$1")
			.trim()
	);
}

// stripRepeatedly removes every match and then looks again, because one pass
// over nested markup leaves a piece of the outer construct behind: "<<b>i>"
// loses the "<b>" and leaves "<i>", and "<!--<!-- -->-->" leaves "-->". What
// is left would go on to be slugged, and a heading whose anchor nobody can
// guess is exactly the failure this file is meant to report rather than cause.
// It is also the single-pass shape CodeQL names as incomplete sanitization.
function stripRepeatedly(text, pattern) {
	let previous;
	let stripped = text;
	do {
		previous = stripped;
		stripped = stripped.replace(pattern, "");
	} while (stripped !== previous);
	return stripped;
}

// explicitIds collects the anchors a page states rather than derives. Both
// renderers honour one: Astro keeps an id a heading or component already
// carries instead of slugging it, and GitHub resolves a fragment to the
// "<a id>" or "<a name>" it prefixed.
function explicitIds(line) {
	const ids = [];
	const pattern =
		/<[A-Za-z][^<>]*?\s(?:id|name)\s*=\s*["']([^"']+)["'][^<>]*>/g;
	let match;
	while ((match = pattern.exec(line)) !== null) {
		ids.push(match[1]);
	}
	return ids;
}

// isParagraphLine answers whether a line could be the text of a Setext
// heading, which is to say an ordinary paragraph: not a blank, not indented
// code, and not the opening of a block that owns its line. A bullet with
// nothing after it is one of those blocks, which is how the two empty list
// items of .github/pull_request_template.md read as a heading underlined by
// the second.
function isParagraphLine(line) {
	if (!/\S/.test(line) || /^ {4,}/.test(line)) {
		return false;
	}
	return !/^ {0,3}(?:[>#|<]|[-*+](?:[ \t]|$)|\d+[.)](?:[ \t]|$))/.test(line);
}
