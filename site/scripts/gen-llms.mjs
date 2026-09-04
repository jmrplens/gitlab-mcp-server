// Builds what the documentation domain serves under /llms.txt, and republishes
// the repository's machine-readable references alongside it.
//
// The llms.txt convention is that a site's /llms.txt maps *that site*. This
// domain serves documentation, so its index lists documentation pages. It used
// to serve the repository's own llms.txt verbatim, which describes the MCP
// server and its tool catalog instead, so a model landing here was handed a
// server summary where it asked for a table of contents, and the entire Spanish
// half of the site was invisible through this channel.
//
// So two things are published from here:
//
//   - /llms.txt and /es/llms.txt are generated from the Astro content
//     collection: one entry per documentation page, in sidebar order, with the
//     page's own frontmatter description.
//   - /llms-server.txt plus the five reference companions are copied verbatim
//     from the repository root, where `cmd/gen_llms` generates and validates
//     them. Keeping them under their own names is what leaves `make check-llms`
//     meaningful: it still checks the files it generates, byte for byte.
//
// Run as a `prebuild` step, so the published copies are always regenerated from
// their single source of truth and never committed. `--check` validates the
// section table against the content collection without writing anything.
import {
	copyFileSync,
	existsSync,
	mkdirSync,
	readFileSync,
	readdirSync,
	statSync,
	writeFileSync,
} from "node:fs";
import { dirname, join, relative, sep } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = join(here, "..", "..");
const publicDir = join(here, "..", "public");
const contentDir = join(here, "..", "src", "content", "docs");

// The authorship domain, not the Pages origin: every documentation link in the
// project resolves through jmrp.io/docs/gitlab-mcp-server, which is a
// path-preserving redirect. Only the site's own canonicals stay on Pages.
const DOCS_BASE = "https://jmrp.io/docs/gitlab-mcp-server/";
const REPO_BASE = "https://github.com/jmrplens/gitlab-mcp-server/";

// The repository's own machine-readable files, republished here under their own
// names. `label` is what the index calls them; `covers` says what a retriever
// gets by fetching that one file instead of the whole catalog. Their contents
// are English in both indexes: they are generated from the server's catalog,
// which has no translation.
const REFERENCE_FILES = [
	{
		name: "llms.txt",
		publishAs: "llms-server.txt",
		label: "Server index",
		labelEs: "Índice del servidor",
		covers:
			"the MCP server itself: install channels, client configuration, environment variables, and a one-line summary of every meta-tool, resource and prompt",
		coversEs:
			"el servidor MCP en sí: canales de instalación, configuración del cliente, variables de entorno y un resumen de una línea de cada meta-herramienta, recurso y prompt",
	},
	{
		name: "llms-medium.txt",
		publishAs: "llms-medium.txt",
		label: "Medium reference",
		labelEs: "Referencia media",
		covers:
			"every tool and action with its description, without the per-action JSON schemas. The largest of these that still loads into a context window",
		coversEs:
			"cada herramienta y acción con su descripción, sin los esquemas JSON por acción. El mayor de estos archivos que aún cabe en una ventana de contexto",
	},
	{
		name: "llms-full-meta-tools.txt",
		publishAs: "llms-full-meta-tools.txt",
		label: "Meta-tool and dynamic surfaces, full schemas",
		labelEs: "Superficies dinámica y de meta-herramientas, esquemas completos",
		covers:
			"complete schemas for the two-tool dynamic surface and the domain meta-tools only",
		coversEs:
			"esquemas completos solo de la superficie dinámica de dos herramientas y de las meta-herramientas por dominio",
	},
	{
		name: "llms-full-individual-tools.txt",
		publishAs: "llms-full-individual-tools.txt",
		label: "Individual tools, full schemas",
		labelEs: "Herramientas individuales, esquemas completos",
		covers: "complete schemas for the one-tool-per-operation surface only",
		coversEs:
			"esquemas completos solo de la superficie de una herramienta por operación",
	},
	{
		name: "llms-full-resources-prompts.txt",
		publishAs: "llms-full-resources-prompts.txt",
		label: "Resources and prompts, full definitions",
		labelEs: "Recursos y prompts, definiciones completas",
		covers: "MCP resource and prompt definitions only",
		coversEs: "solo las definiciones de recursos y prompts MCP",
	},
	{
		name: "llms-full.txt",
		publishAs: "llms-full.txt",
		label: "Full reference",
		labelEs: "Referencia completa",
		covers:
			"the three per-surface splits above concatenated: every surface with every per-action JSON schema. Far beyond any context window, so fetch it only through search or retrieval",
		coversEs:
			"los tres archivos por superficie anteriores concatenados: cada superficie con cada esquema JSON por acción. Muy por encima de cualquier ventana de contexto, así que consúltalo solo mediante búsqueda o recuperación",
	},
];

// The sidebar, restated as data. Astro's config cannot be imported here: it
// fetches a remote identity document at module scope, so importing it would put
// a network call in the middle of a file generator. The completeness check
// below is what keeps this table honest — a page added to the collection and
// not listed here fails the build rather than silently disappearing from the
// index.
const SECTIONS = [
	{
		label: "Getting started",
		labelEs: "Primeros pasos",
		slugs: [
			"getting-started",
			"configuration",
			"architecture",
			"compatibility",
		],
	},
	{
		label: "Installation",
		labelEs: "Instalación",
		slugs: [
			"install/overview",
			"install/binary",
			"install/homebrew",
			"install/winget",
			"install/docker",
			"install/npm",
			"install/pypi",
			"claude-desktop",
			"install/agent-plugin",
			"install/hosted",
		],
	},
	{
		label: "Tools",
		labelEs: "Herramientas",
		slugs: [
			"tools/overview",
			"tools/meta-tools",
			"tools/dynamic-tools",
			"tools/orbit",
			"tools/resources-prompts",
		],
	},
	{
		label: "MCP capabilities",
		labelEs: "Capacidades MCP",
		slugs: [
			"capabilities/overview",
			"capabilities/elicitation",
			"capabilities/completions",
			"capabilities/subscriptions",
			"capabilities/progress",
			"capabilities/icons",
		],
	},
	{
		label: "Operations",
		labelEs: "Operaciones",
		slugs: [
			"operations/security",
			"operations/privacy",
			"operations/telemetry",
			"operations/http-server",
			"operations/remote-deployment",
			"operations/error-handling",
			"operations/ci-cd",
			"operations/docker-testing",
			"operations/troubleshooting",
		],
	},
	{
		label: "Examples",
		labelEs: "Ejemplos",
		slugs: [
			"use-cases",
			"examples/usage",
			"examples/ci-cd-workflows",
			"examples/code-review-workflows",
			"examples/issue-management",
		],
	},
	{
		label: "About",
		labelEs: "Acerca de",
		slugs: ["about", "comparison", "glossary", "changelog"],
	},
];

/** Every .mdx under a directory, as paths relative to it, with "/" separators. */
function collectPages(dir) {
	const out = [];
	for (const entry of readdirSync(dir)) {
		const full = join(dir, entry);
		if (statSync(full).isDirectory()) {
			out.push(...collectPages(full).map((p) => `${entry}/${p}`));
		} else if (entry.endsWith(".mdx")) {
			out.push(entry);
		}
	}
	return out;
}

/**
 * Reads `title` and `description` out of a page's YAML frontmatter.
 *
 * Deliberately not a YAML parser: only these two top-level scalars are needed,
 * and every page in the collection writes them on one line. A page that grows a
 * block scalar there would be caught by the empty-value guard rather than
 * silently producing a truncated entry.
 */
function readFrontmatter(file) {
	const text = readFileSync(file, "utf8");
	const match = text.match(/^---\r?\n([\s\S]*?)\r?\n---/);
	if (!match) throw new Error(`${file}: no YAML frontmatter`);
	const block = match[1];
	const scalar = (key) => {
		const found = block.match(new RegExp(`^${key}:[ \\t]*(.*)$`, "m"));
		if (!found) throw new Error(`${file}: frontmatter has no ${key}`);
		let value = found[1].trim();
		if (
			(value.startsWith('"') && value.endsWith('"')) ||
			(value.startsWith("'") && value.endsWith("'"))
		) {
			value = value.slice(1, -1);
		}
		value = value.replace(/\\"/g, '"');
		if (!value) throw new Error(`${file}: frontmatter ${key} is empty`);
		return value;
	};
	return { title: scalar("title"), description: scalar("description") };
}

/** "install/npm.mdx" -> "install/npm"; "index.mdx" -> "". */
function slugOf(relPath) {
	const withoutExt = relPath.replace(/\.mdx$/, "");
	return withoutExt === "index" ? "" : withoutExt;
}

/** Absolute page URL on the documentation domain. */
function pageURL(slug, locale) {
	const prefix = locale === "es" ? "es/" : "";
	return slug === ""
		? `${DOCS_BASE}${prefix}`
		: `${DOCS_BASE}${prefix}${slug}/`;
}

/** "1.4 MB" / "153 KB", the way a reader decides whether to fetch something. */
function humanSize(bytes) {
	if (bytes >= 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
	return `${Math.round(bytes / 1024)} KB`;
}

/**
 * Rough token count for a plain-text reference file. Four bytes per token is
 * the usual English approximation and is what a consumer needs here: the
 * decision it informs is "does this fit", and no answer near the boundary
 * changes it.
 */
function approxTokens(bytes) {
	const tokens = bytes / 4;
	if (tokens >= 1_000_000) return `~${(tokens / 1_000_000).toFixed(1)}M tokens`;
	return `~${Math.round(tokens / 1000)}k tokens`;
}

/** Reads the released version, so the index dates itself. */
function readVersion() {
	try {
		return readFileSync(join(repoRoot, "VERSION"), "utf8").trim();
	} catch {
		return "";
	}
}

/**
 * Fails when the section table and the content collection disagree. A page in
 * the collection that no section lists would be missing from the index; a slug
 * listed twice would appear twice; a slug listed for a page that no longer
 * exists would publish a 404 to every crawler that reads this file.
 */
function assertSectionsCoverCollection(pagesBySlug) {
	const listed = SECTIONS.flatMap((section) => section.slugs);
	const problems = [];

	const seen = new Set();
	for (const slug of listed) {
		if (seen.has(slug))
			problems.push(`listed twice in the section table: ${slug}`);
		seen.add(slug);
		if (!pagesBySlug.has(slug)) {
			problems.push(
				`listed in the section table but not in the collection: ${slug}`,
			);
		}
	}
	for (const slug of pagesBySlug.keys()) {
		// The home page is the index itself, and is linked from the header rather
		// than listed as one entry among the others.
		if (slug === "") continue;
		if (!seen.has(slug)) {
			problems.push(`in the collection but no section lists it: ${slug}`);
		}
	}
	if (problems.length > 0) {
		throw new Error(
			`site/scripts/gen-llms.mjs: section table is out of date.\n  ${problems.join("\n  ")}\n` +
				"Add the page to SECTIONS in the order it appears in the sidebar.",
		);
	}
}

/** Renders one localized index. */
function renderIndex({ locale, pages, version, referenceSizes }) {
	const es = locale === "es";
	const home = pages.get("");
	const lines = [];
	const push = (line = "") => lines.push(line);

	push(
		es
			? "# Documentación de GitLab MCP Server"
			: "# GitLab MCP Server documentation",
	);
	push();
	push(
		es
			? "> Documentación de gitlab-mcp-server, un servidor Model Context Protocol que expone la API REST v4 y GraphQL de GitLab como herramientas para asistentes de IA."
			: "> Documentation for gitlab-mcp-server, a Model Context Protocol server that exposes the GitLab REST API v4 and GraphQL operations as tools for AI assistants.",
	);
	push();
	push(
		es
			? `Este es el índice del sitio de documentación${version ? ` (versión ${version})` : ""}. Cada entrada enlaza una página de documentación con su propia descripción. El código, los issues y las releases están en ${REPO_BASE.replace(/\/$/, "")}.`
			: `This is the index of the documentation site${version ? ` (version ${version})` : ""}. Every entry links one documentation page and carries that page's own description. Source, issues and releases live at ${REPO_BASE.replace(/\/$/, "")}.`,
	);
	push();
	push(
		es
			? `Portada: [${home.title}](${pageURL("", locale)}) - ${home.description}`
			: `Home: [${home.title}](${pageURL("", locale)}) - ${home.description}`,
	);
	push();
	push(
		es
			? `La documentación está completa en inglés y en español. Esta es la mitad en español; el índice en inglés está en ${DOCS_BASE}llms.txt.`
			: `The documentation is complete in English and Spanish. This is the English half; the Spanish index is at ${DOCS_BASE}es/llms.txt.`,
	);
	push();

	for (const section of SECTIONS) {
		push(`## ${es ? section.labelEs : section.label}`);
		push();
		for (const slug of section.slugs) {
			const page = pages.get(slug);
			push(`- [${page.title}](${pageURL(slug, locale)}): ${page.description}`);
		}
		push();
	}

	push(
		es
			? "## Referencias legibles por máquina"
			: "## Machine-readable references",
	);
	push();
	push(
		es
			? "Estos archivos los genera el catálogo del servidor, no el sitio, y se republican aquí sin cambios; su contenido está en inglés, porque ese catálogo no tiene traducción. Cada entrada lleva su tamaño, porque de eso depende cuáles puedes cargar y cuáles solo consultar por recuperación. Empieza por el índice del servidor."
			: "These files are generated from the server's own catalog rather than from the site, and are republished here unchanged. Every entry carries its size, because that is what decides which ones you can load and which ones you can only retrieve from. Start with the server index.",
	);
	push();
	for (const file of REFERENCE_FILES) {
		const size = referenceSizes.get(file.publishAs);
		const sizeNote = size ? ` (${humanSize(size)}, ${approxTokens(size)})` : "";
		push(
			`- [${es ? file.labelEs : file.label}](${DOCS_BASE}${file.publishAs})${sizeNote}: ${es ? file.coversEs : file.covers}`,
		);
	}
	push();

	push(es ? "## Otros idiomas" : "## Other languages");
	push();
	if (es) {
		push(
			`- [Índice de la documentación en inglés](${DOCS_BASE}llms.txt): la misma documentación en inglés, página por página`,
		);
	} else {
		push(
			`- [Spanish documentation index](${DOCS_BASE}es/llms.txt): the same documentation in Spanish, page for page`,
		);
	}
	push();

	return `${lines
		.join("\n")
		.replace(/\n{3,}/g, "\n\n")
		.trimEnd()}\n`;
}

function main() {
	const checkOnly = process.argv.includes("--check");

	const enPages = new Map();
	const esPages = new Map();
	for (const rel of collectPages(contentDir)) {
		const front = readFrontmatter(join(contentDir, ...rel.split("/")));
		if (rel.startsWith("es/")) {
			esPages.set(slugOf(rel.slice("es/".length)), front);
		} else {
			enPages.set(slugOf(rel), front);
		}
	}

	assertSectionsCoverCollection(enPages);
	const missingEs = [...enPages.keys()].filter((slug) => !esPages.has(slug));
	if (missingEs.length > 0) {
		throw new Error(
			`site/scripts/gen-llms.mjs: Spanish pages missing for: ${missingEs.join(", ")}`,
		);
	}

	const missingReferences = REFERENCE_FILES.filter(
		(file) => !existsSync(join(repoRoot, file.name)),
	).map((file) => file.name);

	if (checkOnly) {
		if (missingReferences.length > 0) {
			throw new Error(
				`site/scripts/gen-llms.mjs: missing repository files: ${missingReferences.join(", ")}. Run \`make gen-llms\`.`,
			);
		}
		console.log(
			`[gen-llms] section table covers all ${enPages.size} English pages and ${esPages.size} Spanish pages`,
		);
		return;
	}

	mkdirSync(publicDir, { recursive: true });
	mkdirSync(join(publicDir, "es"), { recursive: true });

	const referenceSizes = new Map();
	for (const file of REFERENCE_FILES) {
		const src = join(repoRoot, file.name);
		if (!existsSync(src)) {
			console.warn(
				`[gen-llms] ${file.name} not found at repo root, skipping. Run \`make gen-llms\`.`,
			);
			continue;
		}
		const dest = join(publicDir, file.publishAs);
		copyFileSync(src, dest);
		referenceSizes.set(file.publishAs, statSync(dest).size);
		console.log(
			`[gen-llms] published ${file.name} -> ${relative(repoRoot, dest).split(sep).join("/")}`,
		);
	}

	const version = readVersion();
	for (const [locale, pages, target] of [
		["en", enPages, join(publicDir, "llms.txt")],
		["es", esPages, join(publicDir, "es", "llms.txt")],
	]) {
		writeFileSync(
			target,
			renderIndex({ locale, pages, version, referenceSizes }),
		);
		console.log(
			`[gen-llms] generated ${relative(repoRoot, target).split(sep).join("/")}`,
		);
	}
}

try {
	main();
} catch (error) {
	console.error(error.message);
	process.exit(1);
}
