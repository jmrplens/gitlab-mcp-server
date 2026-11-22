// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Starlight route middleware.
//
// Normalises `siteTitleHref` so the site-title link always carries a trailing
// slash. Starlight derives it from the locale as `${base}/${locale}`, which for
// Spanish produced `/gitlab-mcp-server/es` — a URL that 301s to
// `/gitlab-mcp-server/es/`. Every one of the 30 Spanish pages linked there, so
// the canonical Spanish homepage was reachable only through a redirect and was
// never linked directly: it was an orphan for crawlers, and 30 pages spent
// crawl budget on a hop that carries no link equity.
//
// The English side is unaffected (`${base}/` already ends in a slash), but the
// normalisation is written locale-agnostically so a third locale cannot
// reintroduce the bug.
//
// It also dates the page by what it shows. Starlight's `lastUpdated` is the
// last commit to the MDX file alone, while a page that prints a count, a
// release or an imported example changes when that data does. The date comes
// from src/lib/page-dates.mjs, the one source the markdown twin and the
// sitemap read too, and it feeds both the "Last updated" footer and the
// dateModified Head.astro writes.
import { defineRouteMiddleware } from "@astrojs/starlight/route-data";

import { pageDates } from "./lib/page-dates.mjs";

// The build and the dev server run from the site directory, which is what
// the content paths Starlight reports are relative to.
const dates = pageDates(process.cwd());

export const onRequest = defineRouteMiddleware((context) => {
	// Starlight augments App.Locals with `starlightRoute` at runtime; the ambient
	// type is not visible from a standalone module, so narrow it here.
	const route = (
		context.locals as {
			starlightRoute?: {
				siteTitleHref?: string;
				hasSidebar?: boolean;
				lastUpdated?: Date;
				entry?: { filePath?: string; data?: { template?: string } };
			};
		}
	).starlightRoute;
	if (!route) return;

	// Only a page Starlight dates at all is redated, so a page that opts out
	// with `lastUpdated: false` stays without one.
	if (route.lastUpdated instanceof Date && route.entry?.filePath) {
		const date = dates(route.entry.filePath);
		if (date) route.lastUpdated = date.time;
	}

	// Starlight builds a `splash` page with `hasSidebar: false` and renders the
	// mobile menu button only when the route has a sidebar — so on a phone the
	// landing would be the only page with no way to reach the navigation. Only
	// the flag is touched: the tree is already computed, and the desktop column
	// is collapsed in splash-menu.css instead, so the drawer stays populated.
	if (route.entry?.data?.template === "splash") {
		route.hasSidebar = true;
	}

	if (!route.siteTitleHref) return;
	if (!route.siteTitleHref.endsWith("/")) {
		route.siteTitleHref = `${route.siteTitleHref}/`;
	}
});
