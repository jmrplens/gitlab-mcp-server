// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// The markdown twin of a documentation page: the same words, without the
// chrome.
//
// A rendered page is mostly navigation. The sidebar, the table of contents,
// the search index and the syntax highlighting are the bulk of its bytes, and
// none of them are the page. An agent that wants one page had two options:
// parse 80 KB of HTML around it, or download the whole corpus. The twin is the
// third: the page's own markdown, at the page's own path with `index.md`
// appended, announced from the page with `<link rel="alternate">`. The edge in
// front of the site answers `Accept: text/markdown` with a redirect to it, but
// only when the file exists, so a page without its twin is a page served to an
// agent as HTML.
//
// MDX components are reduced, not dropped. An <Aside> becomes a blockquote
// with its title, a <TabItem> keeps its label, a <LinkCard> becomes a link
// carrying its description, <FAQ /> becomes the questions it renders from the
// frontmatter. A component this file does not know throws, so a page that
// starts using one fails the build here instead of quietly losing a section
// the HTML still shows. The same goes for an expression it cannot evaluate and
// for an import it cannot resolve.
//
// Code is opaque. Fenced blocks and code spans are masked before anything
// else runs, so `<NAME>`, `<GitLab's message>`, `{project_id}` or an
// `import ... from` line in a sample is text, never a component, an
// expression or an import, and the link rewriting never touches a path in a
// command.
import { readFileSync } from "node:fs";
import { dirname, join, posix } from "node:path";

import { resolveStatTokens } from "./stat-tokens.mjs";

/** The origin every canonical URL of the site names. */
export const ORIGIN = "https://docs.jmrp.io";

/** The path the site is served under, as the MDX sources write it. */
export const BASE = "/gitlab-mcp-server";

/**
 * Where a relative asset target resolves for a reader of the twin. A page
 * served at /gitlab-mcp-server/about/index.md cannot carry a path relative to
 * its source file, which names a file in the repository, not one on the site,
 * so the twin names that file's bytes on GitHub.
 */
export const SOURCE_ROOT =
	"https://raw.githubusercontent.com/jmrplens/gitlab-mcp-server/main";

/** Every component this file reduces, named in the failure for one it does not. */
export const COMPONENTS = [
	"Aside",
	"Card",
	"CardGrid",
	"ChartPair",
	"Code",
	"FAQ",
	"Fact",
	"Facts",
	"Home",
	"LinkCard",
	"Steps",
	"TabItem",
	"Tabs",
];

// Starlight's default titles for an untitled aside or directive, in the two
// locales the site publishes, so an untitled note and an untitled warning stay
// distinguishable in the twin as they are on the page.
const ASIDE_TITLES = {
	en: { note: "Note", tip: "Tip", caution: "Caution", danger: "Danger" },
	es: {
		note: "Nota",
		tip: "Consejo",
		caution: "Precaución",
		danger: "Peligro",
	},
};

// The heading FAQ.astro gives the section when the page names none.
export const FAQ_TITLES = {
	en: "Frequently asked questions",
	es: "Preguntas frecuentes",
};

/** The absolute URL of a page route: "" is the English home, "es" the Spanish. */
export function pageUrl(route) {
	return route ? `${ORIGIN}${BASE}/${route}/` : `${ORIGIN}${BASE}/`;
}

/** The locale a route is served in. */
export function localeOf(route) {
	return route === "es" || route.startsWith("es/") ? "es" : "en";
}

/**
 * The route of a content collection entry id: "about" for "about.mdx" or
 * "about", "" for "index", "reference/tools" for "reference/tools/index".
 */
export function routeOf(id) {
	const bare = id.replace(/\.mdx?$/, "");
	return bare === "index" ? "" : bare.replace(/\/index$/, "");
}

// --- Masking ---------------------------------------------------------------
//
// A masked character is moved into Supplementary Private Use Area-A, where no
// page writes anything, so no pattern below can match it, and moved back at
// the end. Leading indentation stays unmasked because the reduction dedents
// and indents blocks, and a fence nested in a tab moves with its tab.

const MASK_BASE = 0xf0000;
// A line inside a fence that holds nothing. Kept apart from an empty line so
// the blank-line collapse does not shorten a code sample.
const EMPTY_LINE = String.fromCodePoint(MASK_BASE + 0xfffd);

/** @param {string} text @returns {string} the text, masked */
function mask(text) {
	let out = "";
	for (const char of text) {
		const code = char.codePointAt(0) ?? 0;
		out +=
			char === "\n" || code > 0xffff
				? char
				: String.fromCodePoint(MASK_BASE + code);
	}
	return out;
}

// An empty code line: the marker alone on its line, or behind the indentation
// and the `>` a blockquote put before it. A `>` of the page's own code is
// masked by then, so every `>` this matches is a quote marker.
const EMPTY_CODE_LINE = /^([ \t>]*?)[ \t]*\u{ffffd}[ \t]*$/u;

/** @param {string} text @returns {string} the text with every mask lifted */
function unmask(text) {
	return text
		.split("\n")
		.map((line) =>
			line.replace(EMPTY_CODE_LINE, (_, prefix) => prefix.trimEnd()),
		)
		.join("\n")
		.replace(/[\u{f0000}-\u{ffffd}]/gu, (char) =>
			String.fromCodePoint((char.codePointAt(0) ?? MASK_BASE) - MASK_BASE),
		);
}

/** Masks a line after its indentation; an empty one becomes EMPTY_LINE. */
function maskLine(line) {
	if (!line.trim()) return EMPTY_LINE;
	const indent = /^[ \t]*/.exec(line)?.[0] ?? "";
	return indent + mask(line.slice(indent.length));
}

// A code span: a run of backticks closed by a run of the same length, on one
// line or wrapped onto the next, but not across a blank line, which ends the
// paragraph. A run nothing closes is literal text.
const CODE_SPAN = /(?<!`)(`+)(?!`)(?:[^\n]|\n(?![ \t]*\n))*?(?<!`)\1(?!`)/g;

// The blockquote markers a line opens with, and a fence line behind them: its
// marker and what follows it.
const QUOTES = /^(?:[ \t]*>)*/;
const FENCE = /^(?:[ \t]*>)*[ \t]*(`{3,}|~{3,})(.*)$/;

/**
 * Masks every fenced block, fence lines included, and every code span.
 *
 * @param {string} text MDX body
 * @returns {string} the body with its code opaque
 */
export function maskCode(text) {
	const out = [];
	let prose = [];
	/** @type {{ char: string, length: number, depth: number, quotes: RegExp } | null} */
	let fence = null;
	const flush = () => {
		if (prose.length > 0) {
			out.push(prose.join("\n").replace(CODE_SPAN, (span) => mask(span)));
		}
		prose = [];
	};
	for (const line of text.split("\n")) {
		const quotes = QUOTES.exec(line)?.[0] ?? "";
		const depth = quotes.split(">").length - 1;
		const marker = FENCE.exec(line);
		// A fence in a blockquote ends with the quote, closed or not.
		if (fence && depth < fence.depth) fence = null;
		if (fence) {
			// The fence's own quote markers stay unmasked, so the quote reads
			// as one; a deeper marker is code.
			const own = fence.quotes.exec(line)?.[0] ?? "";
			out.push(own + maskLine(line.slice(own.length)));
			if (
				marker &&
				depth === fence.depth &&
				marker[1][0] === fence.char &&
				marker[1].length >= fence.length &&
				!marker[2].trim()
			) {
				fence = null;
			}
			continue;
		}
		if (marker) {
			flush();
			out.push(quotes + maskLine(line.slice(quotes.length)));
			fence = {
				char: marker[1][0],
				length: marker[1].length,
				depth,
				quotes: new RegExp(`^(?:[ \\t]*>){${depth}}`),
			};
			continue;
		}
		prose.push(line);
	}
	flush();
	return out.join("\n");
}

/**
 * A fenced block for code the reduction itself places (a <Code> component, a
 * command on the landing), already masked like the page's own fences. The
 * fence is longer than any run of backticks in the code, so the code cannot
 * close it.
 *
 * @param {string} code
 * @param {string} [lang]
 * @param {string} [meta] the rest of the info string, such as a title
 * @returns {string} masked markdown
 */
function fenced(code, lang = "", meta = "") {
	const longest = Math.max(
		0,
		...[...code.matchAll(/`+/g)].map((run) => run[0].length),
	);
	const ticks = "`".repeat(Math.max(3, longest + 1));
	const info = [lang, meta].filter(Boolean).join(" ");
	return [`${ticks}${info}`, ...code.replace(/\n+$/, "").split("\n"), ticks]
		.map(maskLine)
		.join("\n");
}

/** Inline code the reduction places, masked like the page's own spans. */
const span = (code) => {
	const ticks = code.includes("`") ? "``" : "`";
	const pad = code.startsWith("`") || code.endsWith("`") ? " " : "";
	return mask(`${ticks}${pad}${code}${pad}${ticks}`);
};

// --- Layout helpers --------------------------------------------------------

/** @param {string} line */
const isBlank = (line) => !line.trim() || line.trim() === EMPTY_LINE;

/**
 * Removes the indentation a component's children carry because of the tag
 * around them, keeping their indentation relative to each other. Left alone,
 * the children of a <TabItem> indented one tab arrive as an indented code
 * block rather than the prose they are.
 */
function dedent(text) {
	const lines = text.split("\n");
	const widths = lines
		.filter((line) => !isBlank(line))
		.map((line) => (/^[ \t]*/.exec(line) ?? [""])[0].length);
	const shift = widths.length > 0 ? Math.min(...widths) : 0;
	return lines
		.map((line) => (isBlank(line) ? line.trim() : line.slice(shift)))
		.join("\n");
}

/** Puts a block back at the indentation its opening tag sat at. */
const indentBy = (text, indent) =>
	indent
		? text
				.split("\n")
				.map((line) => (line.trim() ? indent + line : line))
				.join("\n")
		: text;

/** @param {string} text @returns {string} the text as a blockquote */
const blockquote = (text) =>
	text
		.trim()
		.split("\n")
		.map((line) => (line.trim() ? `> ${line}` : ">"))
		.join("\n");

/** A list item whose body may span several blocks. */
const listItem = (lead, body) => {
	const trimmed = body.trim();
	if (!trimmed) return `- ${lead}`;
	return `- ${lead}\n\n${indentBy(trimmed, "  ")}`;
};

/**
 * Collapses the blank lines the reduction leaves behind, and keeps two
 * blockquotes that follow each other apart: separated by one blank line they
 * are one blockquote to a markdown parser, which is what two asides in a row
 * reduce to, and an empty HTML comment separates them without adding a word.
 */
const tidy = (text) =>
	text
		.replace(/[ \t]+$/gm, "")
		.replace(/\n{3,}/g, "\n\n")
		.replace(/(^>.*)\n\n(?=>)/gm, "$1\n\n<!-- -->\n\n");

// --- Tables ------------------------------------------------------------------

// A pipe-table row behind the indentation and quote markers of the blocks it
// sits in: its prefix, then the row from its opening pipe to its closing one.
const TABLE_ROW = /^([ \t>]*)(\|.*\|)[ \t]*$/;

/**
 * Splits a table row into its cells at every pipe no backslash escapes. A
 * pipe inside a code span is masked by now and splits nothing. GFM lets a row
 * leave out its closing pipe, so a row whose last pipe is escaped ends in a
 * cell holding that pipe, which is kept rather than dropped.
 *
 * @param {string} row from the opening pipe to the last one
 * @returns {string[]}
 */
function tableCells(row) {
	const cells = [];
	let cell = "";
	let closed = false;
	for (let i = 1; i < row.length; i += 1) {
		const char = row[i];
		closed = false;
		if (char === "\\" && i + 1 < row.length) {
			cell += char + row[i + 1];
			i += 1;
		} else if (char === "|") {
			cells.push(cell);
			cell = "";
			closed = true;
		} else {
			cell += char;
		}
	}
	if (!closed) cells.push(cell);
	return cells;
}

/**
 * Removes the padding the sources align their table columns with. The page
 * keeps it because a person reads the source in an editor; a reader of the
 * twin parses the table, and the padding was a third of the twins' bytes. A
 * cell keeps one space on each side and a separator cell becomes `---`, its
 * alignment colons kept. Code is masked by now, so a table in a sample is not
 * a table here and keeps its spacing.
 *
 * @param {string} text masked markdown
 * @returns {string}
 */
function collapseTables(text) {
	return text
		.split("\n")
		.map((line) => {
			const row = TABLE_ROW.exec(line);
			if (!row) return line;
			const cells = tableCells(row[2]).map((cell) => cell.trim());
			const separator = cells.every((cell) => /^:?-+:?$/.test(cell));
			const out = cells.map((cell) => {
				if (separator) {
					return ` ${cell.startsWith(":") ? ":" : ""}---${cell.endsWith(":") ? ":" : ""} `;
				}
				return cell ? ` ${cell} ` : " ";
			});
			return `${row[1]}|${out.join("|")}|`;
		})
		.join("\n");
}

// --- Inline HTML -------------------------------------------------------------

/**
 * The few inline elements the landing's copy writes as HTML, as markdown.
 * Anything else throws: a tag nobody taught this file would reach the twin as
 * markup, or vanish.
 *
 * @param {string} html
 * @param {string} where named in a failure
 * @returns {string} masked-aware markdown
 */
export function inlineHtml(html, where) {
	const out = html
		.replace(/<code>([\s\S]*?)<\/code>/g, (_, code) => span(decode(code)))
		.replace(/<strong>([\s\S]*?)<\/strong>/g, "**$1**")
		.replace(/<em>([\s\S]*?)<\/em>/g, "_$1_")
		.replace(/<a href="([^"]*)">([\s\S]*?)<\/a>/g, "[$2]($1)");
	const left = /<\/?[a-zA-Z][^>]*>/.exec(out);
	if (left) {
		throw new Error(
			`${where}: no markdown for the markup ${left[0]}. Teach inlineHtml in src/lib/page-markdown.mjs.`,
		);
	}
	return decode(out);
}

/**
 * The value of one attribute of an HTML tag, quoted either way.
 *
 * @param {string} tag
 * @param {string} name
 * @returns {string | undefined}
 */
function attribute(tag, name) {
	const match = new RegExp(`\\s${name}\\s*=\\s*(?:"([^"]*)"|'([^']*)')`).exec(
		tag,
	);
	return match ? (match[1] ?? match[2]) : undefined;
}

/**
 * An <img> as markdown. One drawn from a data URI becomes its alternative
 * text: the twin is read as text, and kilobytes of encoded SVG say nothing a
 * reader of it can use. Any other becomes a markdown image.
 */
function htmlImage(tag, file) {
	const alt = attribute(tag, "alt");
	const src = attribute(tag, "src") ?? "";
	if (!alt) {
		throw new Error(
			`${file}: an <img> with no alt text, which the markdown twin would have nothing to say for`,
		);
	}
	return src.startsWith("data:") ? alt : `![${alt}](${src})`;
}

/**
 * The inline HTML a page writes where markdown cannot: images, code, links.
 * A tag behind a backslash is MDX's escape for its `<`, which the page shows
 * as the tag's text, so it stays text here as it does for TAG and the check
 * for elements left after the reduction. In a table cell the pipes of the
 * code are escaped before the span masks them, since GFM splits a row at a
 * bare pipe even inside a code span; outside a table code keeps them as
 * written.
 *
 * @param {string} html
 * @param {string} file named in a failure
 * @param {boolean} [cell] whether the html is a table cell's
 */
const inlineElements = (html, file, cell = false) =>
	html
		.replace(/(?<!\\)<img\b[^>]*>/g, (tag) => htmlImage(tag, file))
		.replace(/(?<!\\)<code>([\s\S]*?)<\/code>/g, (_, code) =>
			span(cell ? escapeCellPipes(decode(code)) : decode(code)),
		)
		.replace(/(?<!\\)<a\s+href="([^"]*)"\s*>([\s\S]*?)<\/a>/g, "[$2]($1)");

/**
 * The raw HTML a page writes, as markdown. A page writes HTML where markdown
 * cannot say it: `<code>{expression}</code>` (a code span would print the
 * expression), a styled table, an image drawn from a data URI. A table
 * becomes a pipe table, its first row the header, and one holding anything
 * beside its rows and their cells fails; an image, a link and inline code
 * become their markdown. Anything else is left for the check after the
 * reduction, which fails the build on it.
 *
 * @param {string} text masked markdown, expressions resolved
 * @param {string} file named in a failure
 * @returns {string}
 */
function reduceHtml(text, file) {
	const tables = text.replace(
		/^([ \t]*)<table\b[^>]*>([\s\S]*?)<\/table>/gm,
		(_, indent, inner) => {
			refuseLeftover(
				inner
					.replace(/<tr\b[^>]*>[\s\S]*?<\/tr>/g, "")
					.replace(/<\/?(?:thead|tbody|tfoot)\b[^>]*>/g, ""),
				"table",
				file,
			);
			const rows = [...inner.matchAll(/<tr\b[^>]*>([\s\S]*?)<\/tr>/g)].map(
				([, row]) => {
					refuseLeftover(
						row.replace(/<(t[hd])\b[^>]*>[\s\S]*?<\/\1>/g, ""),
						"table row",
						file,
					);
					return [...row.matchAll(/<(t[hd])\b[^>]*>([\s\S]*?)<\/\1>/g)].map(
						([, , cell]) =>
							escapeCellPipes(
								decodeText(inlineElements(cell, file, true))
									.replace(/\s+/g, " ")
									.trim(),
							),
					);
				},
			);
			if (rows.length === 0) {
				throw new Error(`${file}: an HTML table with no rows`);
			}
			const line = (/** @type {string[]} */ cells) =>
				`| ${cells.join(" | ")} |`;
			const [head, ...body] = rows;
			return indentBy(
				[line(head), line(head.map(() => "---")), ...body.map(line)].join("\n"),
				indent,
			);
		},
	);
	return inlineElements(tables, file);
}

/**
 * Fails on what an HTML table, or one of its rows, holds beside the parts
 * the reduction keeps: the rows of a table (its <thead>, <tbody> and <tfoot>
 * only group them) and the cells of a row. The pipe table it becomes has no
 * place for a <caption> or for text between cells, and the whole table is
 * replaced, so the check for markup left after the reduction would never see
 * what vanished.
 *
 * @param {string} rest the table or row with the kept parts removed
 * @param {string} what "table" or "table row", named in the failure
 * @param {string} file named in the failure
 */
function refuseLeftover(rest, what, file) {
	const left = rest.trim();
	if (!left) return;
	const shown = /^<[^>]*>|^[^<]{1,40}/.exec(left)?.[0]?.trim() ?? left;
	throw new Error(
		`${file}: an HTML ${what} holding ${shown}, which the markdown twin has no place for`,
	);
}

/**
 * Escapes every pipe of a table cell that no backslash escapes already. The
 * cell is markdown, where a backslash escapes the character after it, and MDX
 * read the page's `\|` as a pipe: that escape is kept as written, so a
 * backslash is copied together with the character after it and only a bare
 * pipe gains one. Escaping every pipe again would turn `\|` into `\\|`, an
 * escaped backslash followed by a pipe that ends the cell.
 *
 * @param {string} cell
 * @returns {string}
 */
function escapeCellPipes(cell) {
	let out = "";
	for (let i = 0; i < cell.length; i += 1) {
		const char = cell[i];
		if (char === "\\" && i + 1 < cell.length) {
			out += char + cell[i + 1];
			i += 1;
		} else {
			out += char === "|" ? "\\|" : char;
		}
	}
	return out;
}

/**
 * Prose with its entities read, for text outside code. A `<` read from
 * `&lt;` is written escaped, as a page writes `\<id>`: the page showed it as
 * text, and bare it would open an element for a markdown reader and for the
 * check after the reduction. The pass that lifts MDX's escapes drops the
 * backslash again wherever no tag could start.
 *
 * @param {string} text
 * @returns {string}
 */
const decodeText = (text) => decode(text.replace(/&lt;/g, "\\<"));

/** @param {string} text @returns {string} the text with its entities read */
const decode = (text) =>
	text
		.replace(/&lt;/g, "<")
		.replace(/&gt;/g, ">")
		.replace(/&quot;/g, '"')
		.replace(/&#39;/g, "'")
		.replace(/&amp;/g, "&");

// --- Imports and expressions -------------------------------------------------

/**
 * Reads a page's import block: which names are components, and what the rest
 * hold. A JSON module holds its parsed value, a `?raw` import its text, the
 * landing's copy module the object the caller passes in. Any other import
 * throws, because a name nothing resolves is a name an expression or an
 * attribute would print as source.
 *
 * @param {string} text MDX body, code masked
 * @param {{ file: string, root: string, home?: Record<string, unknown> }} context
 * @returns {{ body: string, bindings: Map<string, unknown> }}
 */
function readImports(text, context) {
	/** @type {Map<string, unknown>} */
	const bindings = new Map();
	const source = join(context.root, context.file);
	const body = text.replace(
		/^import\s+(\{[^}]*\}|[A-Za-z_$][\w$]*)\s+from\s+["']([^"']+)["'];?[ \t]*$/gm,
		(_, clause, specifier) => {
			const names = clause.trim();
			if (
				specifier === "@astrojs/starlight/components" ||
				specifier.endsWith(".astro")
			) {
				return "";
			}
			if (/(^|\/)data\/home$/.test(specifier)) {
				for (const name of names.replace(/[{}]/g, "").split(",")) {
					const key = name.trim();
					if (!key) continue;
					if (!context.home || !(key in context.home)) {
						throw new Error(
							`${context.file}: imports ${key} from ${specifier}, which the twin was not given.`,
						);
					}
					bindings.set(key, context.home[key]);
				}
				return "";
			}
			if (names.startsWith("{")) {
				throw new Error(
					`${context.file}: named import from ${specifier} has no reduction. ` +
						"Teach readImports in src/lib/page-markdown.mjs.",
				);
			}
			const path = join(dirname(source), specifier.replace(/\?raw$/, ""));
			if (specifier.endsWith("?raw")) {
				bindings.set(names, readFileSync(path, "utf8"));
			} else if (specifier.endsWith(".json")) {
				bindings.set(names, JSON.parse(readFileSync(path, "utf8")));
			} else {
				throw new Error(
					`${context.file}: import from ${specifier} has no reduction. ` +
						"Teach readImports in src/lib/page-markdown.mjs.",
				);
			}
			return "";
		},
	);
	const left = /^import\s.*$/m.exec(body);
	if (left) {
		throw new Error(
			`${context.file}: cannot read the import ${JSON.stringify(left[0])}. ` +
				"Teach readImports in src/lib/page-markdown.mjs.",
		);
	}
	return { body, bindings };
}

/**
 * Evaluates one MDX expression of the shapes the pages write: a data path
 * (`stats.meta.base`), the same with `.toLocaleString("es-ES")`, a number or
 * string literal, and the four arithmetic operators between them. Anything
 * else throws rather than being printed as source.
 *
 * @param {string} expression the text between the braces
 * @param {Map<string, unknown>} bindings
 * @param {string} file named in a failure
 * @returns {unknown}
 */
export function evaluate(expression, bindings, file) {
	const tokens = expression.match(
		/\s+|"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'|\d+(?:\.\d+)?|[A-Za-z_$][\w$]*(?:\.[A-Za-z_$][\w$]*)*(?:\("[^"]*"\))?|[-+*/]|./g,
	);
	const fail = (why) => {
		throw new Error(
			`${file}: the expression {${expression}} ${why}, so the markdown twin would publish it as written. ` +
				"Teach evaluate in src/lib/page-markdown.mjs.",
		);
	};
	const parts = (tokens ?? []).filter((token) => token.trim());
	/** @param {string} token */
	const term = (token) => {
		if (/^\d/.test(token)) return Number(token);
		if (/^["']/.test(token)) return JSON.parse(`"${token.slice(1, -1)}"`);
		const call = /^(.*)\.toLocaleString\("([^"]*)"\)$/.exec(token);
		const path = (call ? call[1] : token).split(".");
		if (!bindings.has(path[0]))
			fail(`names ${path[0]}, which the page does not import`);
		/** @type {unknown} */
		let value = bindings.get(path[0]);
		for (const key of path.slice(1)) {
			value =
				value && typeof value === "object"
					? /** @type {Record<string, unknown>} */ (value)[key]
					: undefined;
		}
		if (value === undefined || value === null) fail("has no value");
		if (call) {
			if (typeof value !== "number")
				fail("formats something that is not a number");
			return value.toLocaleString(call[2]);
		}
		return value;
	};
	if (parts.length === 0 || parts.length % 2 === 0) fail("is not one");
	/** @param {unknown} left @param {string} op @param {unknown} right */
	const apply = (left, op, right) => {
		if (typeof left !== "number" || typeof right !== "number") {
			if (op === "+") return `${left}${right}`;
			return fail("does arithmetic on something that is not a number");
		}
		if (op === "+") return left + right;
		if (op === "-") return left - right;
		if (op === "*") return left * right;
		return left / right;
	};
	// JavaScript's precedence, which MDX evaluates with: `*` and `/` fold into
	// the term before them as they are read, then `+` and `-` fold the terms,
	// each level left to right.
	const terms = [term(parts[0])];
	const additive = [];
	for (let i = 1; i < parts.length; i += 2) {
		const op = parts[i];
		const right = term(parts[i + 1]);
		if (!/^[-+*/]$/.test(op)) fail(`uses ${op}`);
		if (op === "*" || op === "/") {
			terms.push(apply(terms.pop(), op, right));
		} else {
			additive.push(op);
			terms.push(right);
		}
	}
	return additive.reduce(
		(value, op, i) => apply(value, op, terms[i + 1]),
		terms[0],
	);
}

/**
 * Replaces every expression left in the prose with its value. Braces in code
 * are masked by now, so a brace here is one MDX evaluates, unless escaped.
 */
function resolveExpressions(text, bindings, file) {
	let out = "";
	let rest = text;
	for (;;) {
		const at = rest.search(/(?<!\\)\{/);
		if (at === -1) return out + rest;
		let depth = 0;
		let end = -1;
		for (let i = at; i < rest.length; i += 1) {
			if (rest[i] === "{") depth += 1;
			else if (rest[i] === "}") {
				depth -= 1;
				if (depth === 0) {
					end = i;
					break;
				}
			}
		}
		if (end === -1) throw new Error(`${file}: an expression is never closed`);
		const expression = rest.slice(at + 1, end).trim();
		const value = evaluate(expression, bindings, file);
		// An attribute may take an object (<Home content={en} />); prose
		// cannot, and MDX would print it as "[object Object]".
		if (typeof value === "object") {
			throw new Error(
				`${file}: the expression {${expression}} is an object, not text`,
			);
		}
		out += rest.slice(0, at) + String(value);
		rest = rest.slice(end + 1);
	}
}

// --- Components -----------------------------------------------------------

// A component tag, opening, closing or self-closing, across newlines: several
// <LinkCard> tags spread their attributes over four lines. A `<` behind a
// backslash is MDX's escape for the character, as in `\<NAME\>`, never a tag.
const TAG =
	/(?<!\\)<(\/?)([A-Z][A-Za-z0-9]*)((?:"[^"]*"|'[^']*'|\{[^}]*\}|[^>"'{])*?)(\/?)>/;

/**
 * The attributes of a tag: a quoted value as its text, an expression as the
 * value it evaluates to. A bare attribute is `true`.
 */
function parseAttributes(raw, bindings, file) {
	/** @type {Record<string, unknown>} */
	const attributes = {};
	for (const match of raw.matchAll(
		/([A-Za-z][\w-]*)(?:\s*=\s*(?:"([^"]*)"|'([^']*)'|\{([^}]*)\}))?/g,
	)) {
		const [, name, double, single, expression] = match;
		attributes[name] =
			expression !== undefined
				? evaluate(expression.trim(), bindings, file)
				: (double ?? single ?? true);
	}
	return attributes;
}

/** @param {unknown} value @returns {string} */
const text = (value) => (typeof value === "string" ? value : "");

/**
 * Renders one component that wraps content.
 *
 * @param {string} name
 * @param {Record<string, unknown>} attributes
 * @param {string} children the already reduced children
 * @param {Context} context
 * @returns {string}
 */
function renderWrapper(name, attributes, children, context) {
	switch (name) {
		case "Aside": {
			const type = text(attributes.type) || "note";
			const title =
				text(attributes.title) ||
				ASIDE_TITLES[context.locale][
					/** @type {keyof typeof ASIDE_TITLES.en} */ (type)
				] ||
				type;
			return `\n\n${blockquote(`**${title}**\n\n${children.trim()}`)}\n\n`;
		}
		// A tab strip is a set of alternatives, and that is what it stays: one
		// list item per tab, its label leading it, so each body stays attached
		// to its own label rather than merging with the next.
		case "TabItem":
			return `\n\n${listItem(`**${text(attributes.label)}**`, children)}\n\n`;
		// A card's paragraphs are dedented one by one: the pages indent them
		// unevenly, which MDX ignores because it has no indented code blocks,
		// while a markdown reader of the twin would take a paragraph five
		// spaces in for code.
		case "Card": {
			const body = children
				.split(/\n(?:[ \t]*\n)+/)
				.map((block) => dedent(block).trim())
				.join("\n\n");
			return `\n\n${listItem(`**${text(attributes.title)}**`, body)}\n\n`;
		}
		// One declaration of a spec sheet: its label, then the prose that argues
		// it, which is already markdown.
		case "Fact": {
			const label = attributes.kind
				? context.labels[`gm.fact.${text(attributes.kind)}`]
				: text(attributes.label);
			if (!label) {
				throw new Error(
					`${context.file}: <Fact> with no label for kind=${JSON.stringify(attributes.kind)}`,
				);
			}
			const body = children.trim();
			const single =
				!body.includes("\n\n") && !/^\s*(?:[-*+] |\d+\. |>|\|)/m.test(body);
			return single
				? `\n\n- **${label}**: ${body.replace(/\s*\n\s*/g, " ")}\n\n`
				: `\n\n${listItem(`**${label}**`, body)}\n\n`;
		}
		// Pure layout around content that is already markdown: an ordered list
		// for <Steps>, a grid of cards, the sheet that holds the facts.
		case "Tabs":
		case "Steps":
		case "CardGrid":
		case "Facts":
			return `\n\n${children.trim()}\n\n`;
		default:
			throw new Error(
				`${context.file}: no markdown reduction for <${name}>. ` +
					`Teach src/lib/page-markdown.mjs, which knows ${COMPONENTS.join(", ")}.`,
			);
	}
}

/**
 * Renders one self-closing component.
 *
 * @param {string} name
 * @param {Record<string, unknown>} attributes
 * @param {Context} context
 * @returns {string}
 */
function renderSelfClosing(name, attributes, context) {
	switch (name) {
		case "LinkCard": {
			const title = text(attributes.title) || text(attributes.href);
			const description = text(attributes.description);
			return `\n- [${title}](${text(attributes.href)})${description ? `: ${description}` : ""}\n`;
		}
		// The section FAQ.astro renders from the page's `faq:` frontmatter, under
		// the same headings.
		case "FAQ": {
			const items = context.faq ?? [];
			if (items.length === 0) return "";
			const title = text(attributes.title) || FAQ_TITLES[context.locale];
			// The figures are formatted as the page's own body formats them, and
			// as the rendered section and the JSON-LD do (src/lib/stat-tokens.mjs).
			const entries = items.map(
				({ q, a }) =>
					`### ${q}\n\n${resolveStatTokens(a, context.stats, context.locale)}`,
			);
			// The answers come from the frontmatter, outside the masked body, so
			// their code spans are masked here before the passes that follow.
			return maskCode(`\n\n## ${title}\n\n${entries.join("\n\n")}\n\n`);
		}
		// A benchmark figure is generated as a light and a dark SVG; the light one
		// carries the alternative text, and the dark one repeats it.
		case "ChartPair": {
			const figure = `${BASE}/benchmarks/${text(attributes.lang)}/${text(attributes.name)}.light.svg`;
			return `\n\n![${text(attributes.alt)}](${figure})\n\n`;
		}
		case "Code": {
			if (typeof attributes.code !== "string") {
				throw new Error(`${context.file}: <Code> without a code string`);
			}
			const title = text(attributes.title);
			return `\n\n${fenced(attributes.code, text(attributes.lang), title ? `title="${title}"` : "")}\n\n`;
		}
		case "Home": {
			if (!context.renderHome) {
				throw new Error(`${context.file}: <Home> without a landing renderer`);
			}
			return maskCode(
				`\n\n${context.renderHome(attributes.content, context)}\n\n`,
			);
		}
		default:
			throw new Error(
				`${context.file}: no markdown reduction for <${name} />. ` +
					`Teach src/lib/page-markdown.mjs, which knows ${COMPONENTS.join(", ")}.`,
			);
	}
}

/**
 * @typedef {object} Context
 * @property {string} file source path, named in any failure
 * @property {"en" | "es"} locale
 * @property {Record<string, string>} labels the locale's `gm.*` strings
 * @property {Map<string, unknown>} bindings what the page's imports hold
 * @property {{ q: string, a: string }[]} [faq] the page's FAQ frontmatter
 * @property {unknown} stats src/data/stats.json, for FAQ answers
 * @property {(content: unknown, context: Context) => string} [renderHome]
 */

// The containers whose children are sibling components and nothing else. MDX
// ignores how far each child is indented, and the pages indent them unevenly
// (the first <Card> of a grid one space in, the rest four), so a child of one
// of these is placed at the container's own indentation rather than its own:
// kept, the deeper cards would nest under the first as a sub-list.
const LAYOUT = new Set(["CardGrid", "Tabs", "Facts"]);

/**
 * Reduces MDX to markdown, resolving components as it goes.
 *
 * @param {string} source code already masked
 * @param {Context} context
 * @param {boolean} [flat] children of a LAYOUT container, placed at no indentation of their own
 * @returns {string}
 */
function reduce(source, context, flat = false) {
	let out = "";
	let rest = source;
	for (;;) {
		const match = TAG.exec(rest);
		if (!match) return out + rest;
		const [whole, closing, name, raw, selfClosing] = match;
		out += rest.slice(0, match.index);
		rest = rest.slice(match.index + whole.length);
		if (closing) {
			throw new Error(`${context.file}: </${name}> without an opening tag`);
		}
		const attributes = parseAttributes(raw, context.bindings, context.file);
		const indent = flat ? "" : (/(?:^|\n)([ \t]*)$/.exec(out) ?? ["", ""])[1];
		if (selfClosing) {
			out += indentBy(renderSelfClosing(name, attributes, context), indent);
			continue;
		}
		// Scan to the matching close, counting nested tags of the same name so a
		// tab set inside a tab closes the inner one first.
		const scanner = new RegExp(
			`(?<!\\\\)(?:<${name}(?=[\\s/>])|</${name}>)`,
			"g",
		);
		let depth = 1;
		let end = -1;
		let after = -1;
		for (let inner = scanner.exec(rest); inner; inner = scanner.exec(rest)) {
			depth += inner[0].startsWith("</") ? -1 : 1;
			if (depth === 0) {
				end = inner.index;
				after = scanner.lastIndex;
				break;
			}
		}
		if (end === -1) {
			throw new Error(`${context.file}: <${name}> is never closed`);
		}
		const children = rest.slice(0, end);
		rest = rest.slice(after);
		out += indentBy(
			renderWrapper(
				name,
				attributes,
				dedent(reduce(children, context, LAYOUT.has(name))),
				context,
			),
			indent,
		);
	}
}

/**
 * Starlight's container directives (`:::note[Title]` ... `:::`), as the same
 * blockquote an <Aside> reduces to.
 */
function reduceDirectives(text, locale, file) {
	const lines = text.split("\n");
	const out = [];
	for (let i = 0; i < lines.length; i += 1) {
		const open =
			/^([ \t]*):::(note|tip|caution|danger)(?:\[(.*)\])?[ \t]*$/.exec(
				lines[i],
			);
		if (!open) {
			out.push(lines[i]);
			continue;
		}
		const [, indent, type, title] = open;
		const close = lines.findIndex(
			(line, j) => j > i && new RegExp(`^${indent}:::[ \\t]*$`).test(line),
		);
		if (close === -1) throw new Error(`${file}: :::${type} is never closed`);
		const body = dedent(lines.slice(i + 1, close).join("\n"));
		const heading = title || ASIDE_TITLES[locale][type];
		out.push(
			"",
			indentBy(blockquote(`**${heading}**\n\n${body.trim()}`), indent),
			"",
		);
		i = close;
	}
	return out.join("\n");
}

// --- Link targets ---------------------------------------------------------

/**
 * On a Spanish page, a site link that does not name a locale is rewritten to
 * the Spanish page, which is what remarkLocaleLinks in astro.config.mjs does
 * to the HTML: markdown links (not images) and every `href` attribute.
 */
function localizeLinks(text) {
	const prefix = `${BASE}/`;
	const esPrefix = `${BASE}/es/`;
	const localize = (target) =>
		target.startsWith(prefix) && !target.startsWith(esPrefix)
			? `${esPrefix}${target.slice(prefix.length)}`
			: target;
	return text
		.replace(/\]\(([^)\s]+)/g, (whole, target, offset) =>
			isImage(text, offset) ? whole : `](${localize(target)}`,
		)
		.replace(/\bhref="([^"]*)"/g, (_, target) => `href="${localize(target)}"`);
}

/** Whether the `](` at an offset closes an image's alternative text. */
function isImage(text, offset) {
	let depth = 0;
	for (let i = offset; i >= 0; i -= 1) {
		if (text[i] === "]") depth += 1;
		else if (text[i] === "[") {
			depth -= 1;
			if (depth === 0) return text[i - 1] === "!";
		}
	}
	return false;
}

const SCHEME = /^[a-z][a-z0-9+.-]*:/i;

// A reference definition, `[label]: destination "title"`, at the start of a
// line or behind the quote and list markers of the blocks it sits in. The
// destination is bare or in angle brackets. A label opening with `^` is a
// footnote's, which carries text rather than a destination.
const DEFINITION =
	/^((?:[ \t]*(?:>|[-*+][ \t]|\d{1,9}[.)][ \t]))*[ \t]*\[(?!\^)(?:[^\]\\\n]|\\.)+\]:[ \t]*)(?:<([^<>\n]*)>|([^\s<]\S*))/gm;

/**
 * Every link and image target, `href` and `src` made absolute. The twins are
 * read as text, often after being copied off the site, and text keeps no host
 * to resolve `/gitlab-mcp-server/about/` against, nor a page to resolve
 * `#heading` against. A site-rooted target gets the origin, a fragment the
 * page it was written on, a relative target its file in the repository. Any
 * other target throws, because it resolves nowhere a reader can follow. The
 * destination of a reference definition is a target like any other.
 */
function absoluteTargets(text, { file, route }) {
	const dir = posix.dirname(`site/${file}`);
	const absolute = (target) => {
		if (SCHEME.test(target)) return target;
		if (target.startsWith("#")) return `${pageUrl(route)}${target}`;
		if (target.startsWith(`${BASE}/`) || target === BASE) {
			return `${ORIGIN}${target}`;
		}
		// A relative target is an asset beside the source file, so it names a
		// file with an extension; a relative page link would resolve to a
		// directory of the repository rather than to the page, and throws.
		if (
			(target.startsWith("./") || target.startsWith("../")) &&
			/\.[A-Za-z0-9]+(?:[?#].*)?$/.test(target)
		) {
			const source = posix.join(dir, target);
			if (!source.startsWith("../")) return `${SOURCE_ROOT}/${source}`;
		}
		throw new Error(
			`${file}: the link target "${target}" points nowhere a reader of the markdown twin can follow. ` +
				`Write it site-rooted (${BASE}/...), as a fragment, or relative to a file in the repository.`,
		);
	};
	return text
		.replace(
			/\]\(([^)\s]+)((?:\s+"[^"]*")?)\)/g,
			(_, target, title) => `](${absolute(target)}${title})`,
		)
		.replace(
			/\b(href|src)="([^"]*)"/g,
			(_, name, target) => `${name}="${absolute(target)}"`,
		)
		.replace(DEFINITION, (_, lead, bracketed, bare) =>
			bracketed === undefined
				? `${lead}${absolute(bare)}`
				: `${lead}<${absolute(bracketed)}>`,
		);
}

// --- Entry points -----------------------------------------------------------

/**
 * The body of a page as markdown: imports read and dropped, components and
 * expressions resolved, links made absolute.
 *
 * @param {object} page
 * @param {string} page.body MDX body, without frontmatter
 * @param {string} page.file source path relative to the site directory
 * @param {string} page.route the page's route
 * @param {string} page.root the site directory
 * @param {string} page.release the published release, for the version token
 * @param {Record<string, string>} page.labels the locale's `gm.*` strings
 * @param {unknown} page.stats src/data/stats.json
 * @param {{ q: string, a: string }[]} [page.faq]
 * @param {Record<string, unknown>} [page.home] the landing's copy, by name
 * @param {(content: unknown, context: Context) => string} [page.renderHome]
 * @returns {string}
 */
export function reduceBody(page) {
	return unmaskedFailures(() => reduceMasked(page));
}

/**
 * Runs a reduction step, lifting the mask from the message of any failure it
 * throws: a message quotes the page, and a code span of the page is masked
 * while the reduction runs, so it would otherwise read as private-use
 * characters rather than the backticks the author wrote.
 *
 * @template T
 * @param {() => T} step
 * @returns {T}
 */
function unmaskedFailures(step) {
	try {
		return step();
	} catch (error) {
		if (error instanceof Error) error.message = unmask(error.message);
		throw error;
	}
}

/** @param {Parameters<typeof reduceBody>[0]} page */
function reduceMasked(page) {
	const locale = localeOf(page.route);
	const withRelease = page.body.replaceAll("%%VERSION%%", page.release);
	const { body, bindings } = readImports(maskCode(withRelease), {
		file: page.file,
		root: page.root,
		home: page.home,
	});
	/** @type {Context} */
	const context = {
		file: page.file,
		locale,
		labels: page.labels,
		bindings,
		faq: page.faq,
		stats: page.stats,
		renderHome: page.renderHome,
	};
	const withoutComments = body.replace(/\{\/\*[\s\S]*?\*\/\}/g, "");
	let markdown = reduceDirectives(
		reduce(withoutComments, context),
		locale,
		page.file,
	);
	markdown = resolveExpressions(markdown, bindings, page.file);
	markdown = reduceHtml(markdown, page.file);
	if (locale === "es") markdown = localizeLinks(markdown);
	markdown = absoluteTargets(markdown, { file: page.file, route: page.route });
	const residual = /(?<!\\)<\/?([A-Z][A-Za-z0-9]*)/.exec(markdown);
	if (residual) {
		throw new Error(
			`${page.file}: <${residual[1]}> survived the reduction. Teach src/lib/page-markdown.mjs.`,
		);
	}
	// An HTML element reduceHtml does not know would reach the twin as markup.
	// Code is masked by now and an autolink's scheme ends its name with a
	// colon, so a lowercase tag name here is an element the page wrote.
	const element = /(?<!\\)<\/?([a-z][a-z0-9]*)(?=[\s/>])/.exec(markdown);
	if (element) {
		throw new Error(
			`${page.file}: no markdown reduction for the HTML <${element[1]}>. ` +
				"Teach reduceHtml in src/lib/page-markdown.mjs.",
		);
	}
	// A directive reduceDirectives does not know, such as `:::details`, would
	// reach the twin as its raw fence lines. Code is masked by now, so a line
	// opening with `:::` here is one Starlight reads as a directive, behind
	// its indentation and the `>` of a blockquote it sits in: an <Aside> is a
	// blockquote by now, and a page may quote a directive itself.
	const directive = /^[ \t>]*(:::.*)$/m.exec(markdown);
	if (directive) {
		throw new Error(
			`${page.file}: no markdown reduction for the directive ${JSON.stringify(directive[1].trim())}. ` +
				"Teach reduceDirectives in src/lib/page-markdown.mjs.",
		);
	}
	// MDX's escapes for characters it would otherwise read as syntax, now that
	// nothing reads syntax any more: `\{` and `\}` around an expression, and
	// `\>`. The twin keeps the backslash of `\<` where the `<` would open a
	// tag (`\<id>`), because a markdown renderer reads `<id>` as HTML and drops
	// the placeholder; a `<` no tag can start with is unescaped.
	markdown = markdown
		.replace(/\\([{}>])/g, "$1")
		.replace(/\\<(?![A-Za-z/!?])/g, "<");
	const reduced = unmask(tidy(collapseTables(markdown))).trim();
	if (!reduced) throw new Error(`${page.file}: nothing left after reduction`);
	return reduced;
}

/**
 * The markdown twin of one page, in the format the sibling sites publish: the
 * title, the description, the canonical URL, optionally the date the page last
 * changed, a line that is exactly `---`, then the body. The edge turns
 * `Updated:` into Last-Modified, but reads the header only up to the first
 * such line, so the rule is what makes the header a header: without it the
 * edge reads on into the body and ignores the date unless the body happens to
 * open with a rule of its own.
 *
 * @param {Parameters<typeof reduceBody>[0] & { title: string, description: string, updated?: string, lead?: string }} page
 * @returns {string}
 */
export function renderTwin(page) {
	if (!page.description) {
		throw new Error(
			`${page.file}: no frontmatter description to use as the lead`,
		);
	}
	const header = [
		`# ${page.title}`,
		"",
		page.description,
		"",
		`Source: ${pageUrl(page.route)}`,
	];
	if (page.updated) header.push(`Updated: ${page.updated}`);
	const parts = [header.join("\n"), "---"];
	// The splash pages' hero, which Starlight renders from the frontmatter
	// above the body. Its links are written as the page means them, so they
	// are made absolute but not localized.
	if (page.lead) {
		const lead = page.lead.trim();
		parts.push(
			unmaskedFailures(() =>
				unmask(
					absoluteTargets(maskCode(lead), {
						file: page.file,
						route: page.route,
					}),
				),
			),
		);
	}
	parts.push(reduceBody(page));
	return `${parts.join("\n\n")}\n`;
}
