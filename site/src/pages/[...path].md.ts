// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// The markdown twin of every documentation page, at the page's own path with
// `index.md` appended: /gitlab-mcp-server/about/ is doubled by
// /gitlab-mcp-server/about/index.md, the landing by /gitlab-mcp-server/index.md.
//
// Generated from the content collection rather than written by hand or copied
// out of the build: a twin that is not derived from the page it doubles goes
// stale the first time the page is edited, and nothing notices, because a
// stale twin is still a valid document. As an endpoint it is also what
// `astro dev` serves at the same URL, so the twin of a page being written can
// be read while it is written. scripts/check-twins.mjs asserts the closure
// this route is meant to guarantee.
import type { APIRoute, GetStaticPaths } from "astro";
import { getCollection } from "astro:content";

import { en, es } from "../data/home";
import { renderHome } from "../lib/home-markdown.mjs";
import { loadInputs } from "../lib/page-markdown-inputs.mjs";
import { localeOf, renderTwin, routeOf } from "../lib/page-markdown.mjs";

// The build runs from the site directory, which is where the content paths
// the collection reports are relative to.
const root = process.cwd();
const inputs = loadInputs(root);

// The part of a collection entry the twin reads, typed here as Head.astro
// types its own view, rather than through the generated collection types.
interface Hero {
	tagline?: unknown;
	actions?: { text: string; link: string }[];
}
interface DocsEntry {
	id: string;
	filePath?: string;
	body?: string;
	data: {
		title: string;
		description?: string;
		faq?: { q: string; a: string }[];
		hero?: Hero;
	};
}

export const getStaticPaths = (async () => {
	const entries = (await getCollection("docs")) as DocsEntry[];
	return entries.map((entry: DocsEntry) => {
		const route = routeOf(entry.id);
		return {
			// "tools/overview" -> /tools/overview/index.md, "" -> /index.md.
			params: { path: route ? `${route}/index` : "index" },
			props: { entry, route },
		};
	});
}) satisfies GetStaticPaths;

interface Props {
	entry: DocsEntry;
	route: string;
}

/**
 * The splash pages' hero, which Starlight renders above the body from the
 * frontmatter: the tagline, then its actions as links.
 */
function heroLead(hero: Hero | undefined): string {
	if (!hero) return "";
	const lines: string[] = [];
	if (typeof hero.tagline === "string" && hero.tagline)
		lines.push(hero.tagline);
	const actions = (hero.actions ?? []).map(
		(action) => `- [${action.text}](${action.link})`,
	);
	if (actions.length > 0) lines.push(actions.join("\n"));
	return lines.join("\n\n");
}

export const GET: APIRoute = ({ props }) => {
	const { entry, route } = props as Props;
	const { title, description, faq, hero } = entry.data;
	const file = entry.filePath ?? `src/content/docs/${entry.id}.mdx`;
	const locale = localeOf(route) as "en" | "es";
	return new Response(
		renderTwin({
			route,
			title,
			description: description ?? "",
			body: entry.body ?? "",
			file,
			root,
			release: inputs.release,
			labels: inputs.labels[locale],
			stats: inputs.stats,
			faq,
			home: { en, es },
			renderHome: (content: unknown) =>
				renderHome(content, {
					stats: inputs.stats,
					footprint: inputs.footprint,
					file,
				}),
			updated: inputs.updated(file),
			lead: heroLead(hero),
		}),
		{ headers: { "content-type": "text/markdown; charset=utf-8" } },
	);
};
