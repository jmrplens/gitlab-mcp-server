import { defineConfig } from "astro/config";
import starlight from "@astrojs/starlight";
import mermaid from "astro-mermaid";

export default defineConfig({
	site: "https://jmrplens.github.io",
	base: "/gitlab-mcp-server",
	integrations: [
		mermaid({
			theme: "default",
			autoTheme: true,
			enableLog: false,
		}),
		starlight({
			title: "GitLab MCP Server",
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
							slug: "examples/usage",
							label: "Usage Examples",
							translations: { es: "Ejemplos de uso" },
						},
					],
				},
			],
			customCss: ["./src/styles/custom.css"],
		}),
	],
});
