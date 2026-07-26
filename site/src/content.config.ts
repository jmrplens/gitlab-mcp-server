import { defineCollection } from "astro:content";
// `z` re-exported from `astro:content` is deprecated in Astro 7; `astro/zod` is
// the supported path and keeps `astro check` free of deprecation hints.
import { z } from "astro/zod";
import { docsLoader, i18nLoader } from "@astrojs/starlight/loaders";
import { docsSchema, i18nSchema } from "@astrojs/starlight/schema";

export const collections = {
	docs: defineCollection({
		loader: docsLoader(),
		// `datePublished` feeds the TechArticle node in src/components/Head.astro.
		// Git only knows when a file last changed, so first publication is opt-in
		// per page (ISO 8601 date, e.g. "2026-04-21") rather than inferred.
		schema: docsSchema({
			extend: z.object({
				datePublished: z.string().optional(),
				// Page FAQ, rendered as a linked FAQPage node by
				// src/components/Head.astro. Authoring it here rather than as raw
				// JSON-LD in `head:` keeps every FAQ node addressable and tied to
				// its article: hand-written blocks consistently omitted @id,
				// inLanguage, isPartOf and about, leaving a second unlinked
				// page-level entity competing with the TechArticle. It also removes
				// the de-facto 4-question ceiling those copied blocks settled into.
				faq: z.array(z.object({ q: z.string(), a: z.string() })).optional(),
			}),
		}),
	}),
	i18n: defineCollection({ loader: i18nLoader(), schema: i18nSchema() }),
};
