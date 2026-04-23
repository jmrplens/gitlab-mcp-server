import { defineConfig } from "astro/config";
import starlight from "@astrojs/starlight";
import starlightLinksValidator from "starlight-links-validator";
import mermaid from "astro-mermaid";

// Auto-injects locale prefix into internal links for translated content files.
// ES files write links as /gitlab-mcp-server/path/ (same as EN); this plugin
// transforms them to /gitlab-mcp-server/es/path/ at build time, preventing
// locale-stripping bugs and keeping MDX source DRY across languages.
function remarkLocaleLinks() {
	return (tree, file) => {
		const filePath = file.history?.[0] || "";
		if (!filePath.includes("/docs/es/")) return;

		const prefix = `${basePath}/`;
		const esPrefix = `${basePath}/es/`;

		(function visitNode(node) {
			// Markdown links: [text](/gitlab-mcp-server/path/)
			if (
				node.type === "link" &&
				node.url?.startsWith(prefix) &&
				!node.url?.startsWith(esPrefix)
			) {
				node.url = node.url.replace(prefix, esPrefix);
			}
			// MDX JSX elements: <LinkCard href="/gitlab-mcp-server/path/" />
			if (
				(node.type === "mdxJsxFlowElement" ||
					node.type === "mdxJsxTextElement") &&
				node.attributes
			) {
				for (const attr of node.attributes) {
					if (
						attr.name === "href" &&
						typeof attr.value === "string" &&
						attr.value.startsWith(prefix) &&
						!attr.value.startsWith(esPrefix)
					) {
						attr.value = attr.value.replace(prefix, esPrefix);
					}
				}
			}
			if (node.children) node.children.forEach(visitNode);
		})(tree);
	};
}

// Converts deprecated HTML align attributes to CSS text-align (WCAG2AA compliance)
function rehypeTableAlign() {
	return (tree) => {
		(function visit(node) {
			if (
				node.type === "element" &&
				(node.tagName === "td" || node.tagName === "th") &&
				node.properties?.align
			) {
				const val = node.properties.align;
				node.properties.style = `text-align:${val}`;
				delete node.properties.align;
			}
			if (node.children) node.children.forEach(visit);
		})(tree);
	};
}

const siteUrl = "https://jmrplens.github.io";
const basePath = "/gitlab-mcp-server";
const fullUrl = `${siteUrl}${basePath}`;

const jsonLd = JSON.stringify({
	"@context": "https://schema.org",
	"@graph": [
		{
			"@type": "WebSite",
			name: "GitLab MCP Server",
			url: `${fullUrl}/`,
			description:
				"A Model Context Protocol (MCP) server exposing 1000+ GitLab operations as AI-accessible tools. Written in Go.",
			inLanguage: ["en", "es"],
			publisher: {
				"@type": "Person",
				name: "José Manuel Requena Plens",
				alternateName: "jmrplens",
				url: "https://jmrp.io",
				sameAs: [
					"https://github.com/jmrplens",
					"https://linkedin.com/in/jmrplens",
					"https://mstdn.jmrp.io/@jmrplens",
					"https://scholar.google.com/citations?user=9b0kPaUAAAAJ",
					"https://matrix.to/#/@jmrplens:matrix.jmrp.io",
					"https://keyoxide.org/0A993B268654DBBA52B7E8D3FCF653391E2C91FC",
				],
			},
		},
		{
			"@type": "SoftwareApplication",
			name: "GitLab MCP Server",
			applicationCategory: "DeveloperApplication",
			operatingSystem: "Windows, Linux, macOS",
			programmingLanguage: "Go",
			url: "https://github.com/jmrplens/gitlab-mcp-server",
			downloadUrl: "https://github.com/jmrplens/gitlab-mcp-server/releases",
			license: "https://opensource.org/licenses/MIT",
			description:
				"Model Context Protocol server that exposes 1000+ GitLab operations as AI-accessible tools.",
			offers: {
				"@type": "Offer",
				price: "0",
				priceCurrency: "USD",
			},
			author: {
				"@type": "Person",
				name: "José Manuel Requena Plens",
				url: "https://jmrp.io",
			},
		},
	],
});

export default defineConfig({
	site: siteUrl,
	base: basePath,
	integrations: [
		mermaid({
			theme: "default",
			autoTheme: true,
			enableLog: false,
		}),
		starlight({
			title: "GitLab MCP Server",
			plugins: [
				starlightLinksValidator({
					errorOnRelativeLinks: false,
					errorOnFallbackPages: false,
				}),
			],
			description:
				"A Model Context Protocol (MCP) server exposing 1000+ GitLab operations as AI-accessible tools. Written in Go.",
			logo: {
				dark: "./src/assets/logo-dark.svg",
				light: "./src/assets/logo-light.svg",
				replacesTitle: false,
			},
			social: [
				{
					icon: "github",
					label: "GitHub",
					href: "https://github.com/jmrplens/gitlab-mcp-server",
				},
				{
					icon: "mastodon",
					label: "Mastodon",
					href: "https://mstdn.jmrp.io/@jmrplens",
				},
				{
					icon: "linkedin",
					label: "LinkedIn",
					href: "https://linkedin.com/in/jmrplens",
				},
			],
			head: [
				// Open Graph image
				{
					tag: "meta",
					attrs: {
						property: "og:image",
						content: `${fullUrl}/og-image.png`,
					},
				},
				{
					tag: "meta",
					attrs: {
						property: "og:image:alt",
						content: "GitLab MCP Server — 1000+ GitLab tools for AI assistants",
					},
				},
				{
					tag: "meta",
					attrs: { property: "og:image:width", content: "1200" },
				},
				{
					tag: "meta",
					attrs: { property: "og:image:height", content: "630" },
				},
				// Twitter card image
				{
					tag: "meta",
					attrs: {
						name: "twitter:image",
						content: `${fullUrl}/og-image.png`,
					},
				},
				// Author
				{
					tag: "meta",
					attrs: {
						name: "author",
						content: "José Manuel Requena Plens",
					},
				},
				// Theme color
				{
					tag: "meta",
					attrs: { name: "theme-color", content: "#A78BFA" },
				},
				// rel="me" identity links
				{
					tag: "link",
					attrs: {
						rel: "me",
						href: "https://github.com/jmrplens",
					},
				},
				{
					tag: "link",
					attrs: {
						rel: "me",
						href: "https://linkedin.com/in/jmrplens",
					},
				},
				{
					tag: "link",
					attrs: {
						rel: "me",
						href: "https://mstdn.jmrp.io/@jmrplens",
					},
				},
				{
					tag: "link",
					attrs: {
						rel: "me",
						href: "https://scholar.google.com/citations?user=9b0kPaUAAAAJ",
					},
				},
				{
					tag: "link",
					attrs: {
						rel: "me",
						href: "https://matrix.to/#/@jmrplens:matrix.jmrp.io",
					},
				},
				{
					tag: "link",
					attrs: {
						rel: "me",
						href: "https://keyoxide.org/0A993B268654DBBA52B7E8D3FCF653391E2C91FC",
					},
				},
				{
					tag: "link",
					attrs: { rel: "me", href: "https://jmrp.io" },
				},
				// PGP public key
				{
					tag: "link",
					attrs: {
						rel: "pgpkey",
						type: "application/pgp-keys",
						href: "https://keys.openpgp.org/vks/v1/by-fingerprint/0A993B268654DBBA52B7E8D3FCF653391E2C91FC",
					},
				},
				// Web app manifest
				{
					tag: "link",
					attrs: {
						rel: "manifest",
						href: `${basePath}/manifest.json`,
					},
				},
				// JSON-LD structured data
				{
					tag: "script",
					attrs: { type: "application/ld+json" },
					content: jsonLd,
				},
			],
			editLink: {
				baseUrl:
					"https://github.com/jmrplens/gitlab-mcp-server/edit/main/site/",
			},
			defaultLocale: "root",
			locales: {
				root: { label: "English", lang: "en" },
				es: { label: "Español", lang: "es" },
			},
			lastUpdated: true,
			favicon: "/favicon.svg",
			sidebar: [
				{
					label: "Getting Started",
					translations: { es: "Primeros pasos" },
					items: [
						{
							slug: "getting-started",
							label: "Quick Start",
							translations: { es: "Inicio rápido" },
						},
						{
							slug: "configuration",
							label: "Configuration",
							translations: { es: "Configuración" },
						},
						{
							slug: "architecture",
							label: "Architecture",
							translations: { es: "Arquitectura" },
						},
						{
							slug: "compatibility",
							label: "Compatibility",
							translations: { es: "Compatibilidad" },
						},
					],
				},
				{
					label: "Tools",
					translations: { es: "Herramientas" },
					items: [
						{
							slug: "tools/overview",
							label: "Overview",
							translations: { es: "Descripción general" },
						},
						{
							slug: "tools/meta-tools",
							label: "Meta-tools",
							translations: { es: "Meta-herramientas" },
						},
						{
							slug: "tools/analysis",
							label: "Analysis Tools",
							translations: { es: "Herramientas de análisis" },
						},
						{
							slug: "tools/resources-prompts",
							label: "Resources & Prompts",
							translations: { es: "Recursos y Prompts" },
						},
					],
				},
				{
					label: "MCP Capabilities",
					translations: { es: "Capacidades MCP" },
					items: [
						{
							slug: "capabilities/overview",
							label: "Overview",
							translations: { es: "Descripción general" },
						},
						{
							slug: "capabilities/sampling",
							label: "Sampling",
						},
						{
							slug: "capabilities/roots",
							label: "Roots",
						},
						{
							slug: "capabilities/elicitation",
							label: "Elicitation",
							translations: { es: "Elicitación" },
						},
						{
							slug: "capabilities/completions",
							label: "Completions",
						},
						{
							slug: "capabilities/logging",
							label: "Logging",
						},
						{
							slug: "capabilities/progress",
							label: "Progress",
							translations: { es: "Progreso" },
						},
						{
							slug: "capabilities/icons",
							label: "Icons",
							translations: { es: "Iconos" },
						},
					],
				},
				{
					label: "Operations",
					translations: { es: "Operaciones" },
					items: [
						{
							slug: "operations/security",
							label: "Security",
							translations: { es: "Seguridad" },
						},
						{
							slug: "operations/auto-update",
							label: "Auto-update",
							translations: { es: "Auto-actualización" },
						},
						{
							slug: "operations/http-server",
							label: "HTTP Server",
							translations: { es: "Servidor HTTP" },
						},
						{
							slug: "operations/error-handling",
							label: "Error Handling",
							translations: { es: "Errores y formato" },
						},
						{
							slug: "operations/ci-cd",
							label: "CI/CD Usage",
							translations: { es: "Uso en CI/CD" },
						},
						{
							slug: "operations/troubleshooting",
							label: "Troubleshooting",
							translations: { es: "Solución de problemas" },
						},
					],
				},
				{
					label: "Examples",
					translations: { es: "Ejemplos" },
					items: [
						{
							slug: "use-cases",
							label: "Use Cases",
							translations: { es: "Casos de uso" },
						},
						{
							slug: "examples/usage",
							label: "Usage Examples",
							translations: { es: "Ejemplos de uso" },
						},
						{
							slug: "examples/ci-cd-workflows",
							label: "CI/CD Workflows",
							translations: { es: "Flujos CI/CD" },
						},
						{
							slug: "examples/code-review-workflows",
							label: "Code Review",
							translations: { es: "Revisión de código" },
						},
						{
							slug: "examples/issue-management",
							label: "Issue Management",
							translations: { es: "Gestión de issues" },
						},
					],
				},
			],
			customCss: ["./src/styles/custom.css"],
		}),
	],
	markdown: {
		remarkPlugins: [remarkLocaleLinks],
		rehypePlugins: [rehypeTableAlign],
	},
});
