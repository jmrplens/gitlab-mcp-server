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
import { defineRouteMiddleware } from "@astrojs/starlight/route-data";

export const onRequest = defineRouteMiddleware((context) => {
	// Starlight augments App.Locals with `starlightRoute` at runtime; the ambient
	// type is not visible from a standalone module, so narrow it here.
	const route = (
		context.locals as { starlightRoute?: { siteTitleHref?: string } }
	).starlightRoute;
	if (!route?.siteTitleHref) return;
	if (!route.siteTitleHref.endsWith("/")) {
		route.siteTitleHref = `${route.siteTitleHref}/`;
	}
});
