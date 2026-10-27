// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Holds every code block of a Spanish page to its English twin.
//
// A code block is something a reader copies and runs, pastes into a client
// configuration, or compares with what the server printed, so the Spanish
// page carries the English block byte for byte, placeholders included: a
// path written as /ruta/a/gitlab-mcp-server or a token written as
// glpat-tu-token is a command that differs from the one the English page was
// checked with, and the end-to-end runs of the documented commands read the
// English page. Only two things in a block are translated:
//
//   - COMMENTS. What a language marks as a comment is removed from both
//     blocks before they are compared, together with a line it leaves empty,
//     so a comment may be translated, reflowed, added or dropped. Which text
//     is a comment is decided per language (COMMENT_SYNTAX below); a language
//     the table does not name has no comments, and its block must match byte
//     for byte, comments included, unless its fence names the syntax with
//     `comments=hash`, `ini`, `slash` or `markup` (an HAProxy configuration,
//     which the highlighter has no grammar for, is a `text comments=hash`
//     block). The option is a fence option like any other, so both fences
//     carry it, and Expressive Code passes over a key it does not know.
//   - PROSE. A Mermaid diagram is a picture whose labels are words, so its
//     body is never compared. A `text` or `markdown` block that holds prose
//     rather than code (a prompt a reader types to the assistant, a sketch of
//     an exchange) says so with the word `translated` in its fence, in both
//     languages. Expressive Code reads an unknown bare word as a boolean
//     option no plugin asks for, so the word changes nothing on the page. A
//     block that declares itself translated and is identical to its twin
//     excuses nothing, and fails as well. A sketch of an exchange quotes code,
//     though, and the translation keeps it: the code such a block quotes
//     (QUOTED_CODE below) is held to its twin's, in any order, since a
//     translated sentence may put it elsewhere.
//
// The fence must agree too: the same language, and the same options once a
// `title="…"` (a caption, which is translated) is set aside. Pages pair by
// path, as check-i18n-parity.mjs requires, and blocks pair by their order on
// the page; a page whose two languages hold a different number of blocks is
// reported as such, because pairing past the first missing block would
// compare unrelated blocks.
//
// Run: node scripts/check-i18n-code.mjs
import { existsSync, readFileSync, readdirSync, statSync } from "node:fs";
import { join, relative } from "node:path";
import { fileURLToPath } from "node:url";

const docsDir = fileURLToPath(new URL("../src/content/docs", import.meta.url));
const esDir = join(docsDir, "es");

// The marker a prose block carries in its fence, and the languages that may
// carry it. Every other language is code.
const TRANSLATED = "translated";
const PROSE_LANGUAGES = new Set(["text", "markdown"]);

// How each language writes a comment.
//   hash:   `#` at the start of a word, outside quotes, to the end of the line
//   ini:    a line whose first character is `;` or `#`
//   slash:  `//` to the end of the line and `/* … */`, outside quotes
//   markup: `<!-- … -->`
const COMMENT_SYNTAX = new Map([
	...[
		"bash",
		"sh",
		"shell",
		"zsh",
		"yaml",
		"yml",
		"toml",
		"nginx",
		"apache",
		"powershell",
		"dockerfile",
		"dotenv",
		"python",
	].map((lang) => [lang, "hash"]),
	["ini", "ini"],
	...["typescript", "ts", "javascript", "js", "go", "jsonc"].map((lang) => [
		lang,
		"slash",
	]),
	...["xml", "html", "markdown", "md", "mdx"].map((lang) => [lang, "markup"]),
]);

// The comment syntaxes a fence may name with `comments=`.
const COMMENT_NAMES = ["hash", "ini", "slash", "markup"];

// The code a block of prose quotes, which its translation keeps, matched in
// this order: a gitlab:// URI; a quoted string, which is a value when it
// holds no space and a sentence (prose, passed over) when it does; a key
// given a value (`state: "opened"`, `username="x"`) at the start of a line
// or after `{`, `(`, `,` or an arrow; a name written in code, holding a dot
// between two word characters (an action ID, a file) or an underscore (a
// parameter, a tool, a prompt); and a number. A word of prose is none of
// these, and neither is a label such as "Server suggests:".
const QUOTED_CODE =
	/gitlab:\/\/[^\s"'`<>)\]},]+|"[^"\n]*"|(?<=^[ \t]*|[{(,→][ \t]*)[a-z][a-z0-9_]*(?=[ \t]*[:=](?!\/\/))|\b[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z0-9_*]+)+|\b[A-Za-z0-9]*_[A-Za-z0-9_]*\b|\b\d+(?:[.,]\d+)*\b/g;

/** Every .md/.mdx page under `dir`, recursively, leaving out the Spanish tree. */
function walk(dir) {
	const out = [];
	for (const entry of readdirSync(dir).sort()) {
		const p = join(dir, entry);
		if (statSync(p).isDirectory()) {
			if (p === esDir) continue;
			out.push(...walk(p));
		} else if (/\.mdx?$/.test(entry)) {
			out.push(p);
		}
	}
	return out;
}

/**
 * The fenced code blocks of a page, in order, after its frontmatter. A fence
 * opens with three or more backticks or tildes at any indentation (a block
 * inside a list item or a component is indented) and closes with a run of
 * the same character at least as long, as CommonMark has it; a fence left
 * open runs to the end of the page. Each body line loses the indentation of
 * the opening fence.
 */
export function fencedBlocks(source) {
	const lines = source.split("\n");
	const blocks = [];
	let i = 0;
	if (lines[0] === "---") {
		const end = lines.indexOf("---", 1);
		if (end > 0) i = end + 1;
	}
	for (; i < lines.length; i++) {
		const open = /^([ \t]*)(`{3,}|~{3,})(.*)$/.exec(lines[i]);
		if (!open) continue;
		const [, indent, fence, rest] = open;
		if (fence[0] === "`" && rest.includes("`")) continue;
		const info = rest.trim();
		const body = [];
		let j = i + 1;
		for (; j < lines.length; j++) {
			const close = /^[ \t]*(`{3,}|~{3,})[ \t]*$/.exec(lines[j]);
			if (close && close[1][0] === fence[0] && close[1].length >= fence.length)
				break;
			const line = lines[j];
			body.push(
				line.startsWith(indent) ? line.slice(indent.length) : line.trimStart(),
			);
		}
		const lang = info.split(/\s+/)[0];
		blocks.push({
			line: i + 1,
			lang,
			meta: info.slice(lang.length).trim(),
			body,
		});
		i = j;
	}
	return blocks;
}

/** The options of a fence that both languages must share: all but the title. */
export function sharedOptions(meta) {
	return meta
		.replace(/\btitle\s*=\s*("[^"]*"|'[^']*'|\S+)/g, " ")
		.split(/\s+/)
		.filter(Boolean)
		.join(" ");
}

function isTranslated(meta) {
	return sharedOptions(meta).split(" ").includes(TRANSLATED);
}

/** The comment syntax a fence names with `comments=`, or undefined when it names none. */
function commentOption(meta) {
	return /(?:^| )comments=(\S+)/.exec(sharedOptions(meta))?.[1];
}

/**
 * The code a block of prose quotes (QUOTED_CODE), as a count per value. A
 * quoted string holding a space is a sentence and is left out.
 */
export function quotedCode(body) {
	const counts = new Map();
	for (const line of body) {
		for (const [token] of line.matchAll(QUOTED_CODE)) {
			if (token.startsWith('"') && /\s/.test(token)) continue;
			counts.set(token, (counts.get(token) ?? 0) + 1);
		}
	}
	return counts;
}

/** The values a holds more often than b, each as many times as it has more. */
function surplus(a, b) {
	const out = [];
	for (const [token, count] of a) {
		for (let k = b.get(token) ?? 0; k < count; k++) out.push(token);
	}
	return out;
}

/**
 * Removes the comments of one line of a `hash` language. `#` opens a comment
 * at the start of a word (the start of the line, or after whitespace that no
 * backslash escapes), never inside quotes; a quote opens a string only where
 * a value can begin (the start of a word, or after one of `=:[{(,$`), so the
 * apostrophe in "don't" opens nothing.
 */
function stripHash(line) {
	let out = "";
	let quote = null;
	// What precedes the current character: "start" for the start of a word,
	// otherwise the character itself.
	let prev = "start";
	for (let i = 0; i < line.length; i++) {
		const c = line[i];
		if (quote) {
			out += c;
			if (c === "\\" && quote === '"' && i + 1 < line.length) out += line[++i];
			else if (c === quote) quote = null;
			prev = c;
			continue;
		}
		if (c === "\\" && i + 1 < line.length) {
			out += c + line[++i];
			prev = line[i];
			continue;
		}
		if (
			(c === '"' || c === "'") &&
			(prev === "start" || "=:[{(,$".includes(prev))
		) {
			quote = c;
			out += c;
			prev = c;
			continue;
		}
		if (c === "#" && prev === "start") return { code: out, comment: true };
		out += c;
		prev = /\s/.test(c) ? "start" : c;
	}
	return { code: out, comment: false };
}

/**
 * Removes the comments of one line of a `slash` language, carrying an open
 * block comment from line to line in `state`.
 */
function stripSlash(line, state) {
	let out = "";
	let quote = null;
	let comment = false;
	for (let i = 0; i < line.length; i++) {
		const c = line[i];
		if (state.inBlock) {
			comment = true;
			if (c === "*" && line[i + 1] === "/") {
				state.inBlock = false;
				i++;
			}
			continue;
		}
		if (quote) {
			out += c;
			if (c === "\\" && i + 1 < line.length) out += line[++i];
			else if (c === quote) quote = null;
			continue;
		}
		if (c === '"' || c === "'" || c === "`") {
			quote = c;
			out += c;
			continue;
		}
		if (c === "/" && line[i + 1] === "/") return { code: out, comment: true };
		if (c === "/" && line[i + 1] === "*") {
			state.inBlock = true;
			comment = true;
			i++;
			continue;
		}
		out += c;
	}
	return { code: out, comment };
}

/** Removes the `<!-- … -->` comments of one line, carrying an open one in `state`. */
function stripMarkup(line, state) {
	let out = "";
	let comment = false;
	let i = 0;
	while (i < line.length) {
		if (state.inBlock) {
			comment = true;
			const end = line.indexOf("-->", i);
			if (end < 0) return { code: out, comment };
			state.inBlock = false;
			i = end + 3;
			continue;
		}
		const start = line.indexOf("<!--", i);
		if (start < 0) {
			out += line.slice(i);
			break;
		}
		out += line.slice(i, start);
		state.inBlock = true;
		comment = true;
		i = start + 4;
	}
	return { code: out, comment };
}

/**
 * The lines of a block that are code, under the comment syntax its fence
 * names or its language has (undefined for none): each line with its
 * comments removed and its trailing whitespace trimmed, and a line that held
 * nothing but a comment left out. Each entry keeps its index in the body.
 */
export function codeLines(body, syntax) {
	const state = { inBlock: false };
	const out = [];
	body.forEach((line, index) => {
		let result = { code: line, comment: false };
		if (syntax === "hash") result = stripHash(line);
		else if (syntax === "ini")
			result = /^\s*[;#]/.test(line)
				? { code: "", comment: true }
				: { code: line, comment: false };
		else if (syntax === "slash") result = stripSlash(line, state);
		else if (syntax === "markup") result = stripMarkup(line, state);
		const code = result.code.trimEnd();
		if (result.comment && code.trim() === "") return;
		out.push({ index, text: code });
	});
	return out;
}

/**
 * The findings for one pair of pages. `enRel` and `esRel` name them in
 * messages; each finding is one line, followed by indented detail lines.
 */
export function comparePages(enRel, enSource, esRel, esSource) {
	const en = fencedBlocks(enSource);
	const es = fencedBlocks(esSource);
	if (en.length !== es.length) {
		return [
			`${esRel}: ${es.length} code blocks, ${enRel} has ${en.length}; ` +
				`blocks pair by their order, so the two pages must hold the same blocks`,
		];
	}
	const findings = [];
	en.forEach((a, k) => {
		const b = es[k];
		const where = `${esRel}:${b.line} (${enRel}:${a.line})`;
		if (a.lang !== b.lang) {
			findings.push(
				`${where}: a ${b.lang || "plain"} block where English has ${a.lang || "plain"}`,
			);
			return;
		}
		const aOptions = sharedOptions(a.meta);
		const bOptions = sharedOptions(b.meta);
		if (aOptions !== bOptions) {
			findings.push(
				`${where}: the fence options differ`,
				`    en: ${aOptions || "(none)"}`,
				`    es: ${bOptions || "(none)"}`,
			);
			return;
		}
		const option = commentOption(a.meta);
		if (option !== undefined && !COMMENT_NAMES.includes(option)) {
			findings.push(
				`${where}: comments=${option} names no comment syntax; ` +
					`use ${COMMENT_NAMES.slice(0, -1).join(", ")} or ${COMMENT_NAMES.at(-1)}`,
			);
			return;
		}
		if (isTranslated(a.meta)) {
			if (!PROSE_LANGUAGES.has(a.lang)) {
				findings.push(
					`${where}: a ${a.lang} block cannot be marked ${TRANSLATED}; ` +
						`only ${[...PROSE_LANGUAGES].join(" and ")} blocks hold prose`,
				);
				return;
			}
			if (a.body.join("\n") === b.body.join("\n")) {
				findings.push(
					`${where}: marked ${TRANSLATED} and identical to its twin; ` +
						`drop the marker from both fences`,
				);
			}
			// Two identical bodies quote the same code, so this adds nothing to
			// the finding above.
			const enCode = quotedCode(a.body);
			const esCode = quotedCode(b.body);
			const onlyEn = surplus(enCode, esCode);
			const onlyEs = surplus(esCode, enCode);
			if (onlyEn.length || onlyEs.length) {
				findings.push(
					`${where}: the ${a.lang} block marked ${TRANSLATED} quotes code its twin does not`,
					`    en: ${onlyEn.join(" ") || "(none)"}`,
					`    es: ${onlyEs.join(" ") || "(none)"}`,
				);
			}
			return;
		}
		if (a.lang === "mermaid") return;
		const syntax = option ?? COMMENT_SYNTAX.get(a.lang);
		const difference = firstDifference(
			codeLines(a.body, syntax),
			codeLines(b.body, syntax),
		);
		if (!difference) return;
		const { x, y, more } = difference;
		const line = (block, entry) =>
			entry
				? `${block.line + 1 + entry.index}: ${entry.text}`
				: "(the block ends)";
		const tail = more
			? ` (and ${more} more differing line${more > 1 ? "s" : ""})`
			: "";
		findings.push(
			`${where}: the ${a.lang} block differs from English outside comments${tail}`,
			`    en ${line(a, x)}`,
			`    es ${line(b, y)}`,
		);
	});
	return findings;
}

/**
 * The first pair of code lines that differ, or null when none does. When both
 * blocks hold as many code lines, the lines stay aligned past the first
 * difference, so `more` counts the other lines that differ too; otherwise
 * nothing after the first difference can be paired and `more` is 0.
 */
function firstDifference(en, es) {
	const n = Math.max(en.length, es.length);
	for (let m = 0; m < n; m++) {
		if (en[m] && es[m] && en[m].text === es[m].text) continue;
		let more = 0;
		if (en.length === es.length) {
			for (let k = m + 1; k < n; k++) if (en[k].text !== es[k].text) more++;
		}
		return { x: en[m], y: es[m], more };
	}
	return null;
}

function main() {
	const findings = [];
	let pairs = 0;
	for (const enPath of walk(docsDir)) {
		const rel = relative(docsDir, enPath);
		const esPath = join(esDir, rel);
		// A page with no twin is check-i18n-parity.mjs's finding.
		if (!existsSync(esPath)) continue;
		pairs++;
		findings.push(
			...comparePages(
				rel,
				readFileSync(enPath, "utf8"),
				`es/${rel}`,
				readFileSync(esPath, "utf8"),
			),
		);
	}
	if (findings.length) {
		console.error("i18n code block check FAILED:");
		for (const line of findings) console.error(`  ${line}`);
		process.exit(1);
	}
	console.log(
		`i18n code blocks OK: every code block of ${pairs} Spanish pages matches its English twin outside comments.`,
	);
}

if (process.argv[1] === fileURLToPath(import.meta.url)) main();
