import { defineCollection } from "astro:content";
// `z` re-exported from `astro:content` is deprecated in Astro 7; `astro/zod` is
// the supported path and keeps `astro check` free of deprecation hints.
import { z } from "astro/zod";
import { docsLoader, i18nLoader } from "@astrojs/starlight/loaders";
import { docsSchema, i18nSchema } from "@astrojs/starlight/schema";

// The page-title chip contract, exported so the renderer
// (src/components/docs/PageChips.astro) derives its type from this schema
// instead of redeclaring the shape — a schema change is then a type error
// in the renderer, not a silent drift.
export const chipSchema = z.object({
	text: z.string(),
	href: z.string().optional(),
});
export type Chip = z.infer<typeof chipSchema>;

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
				// Page-title chips: at most four short QUALITIES of the page —
				// "Catalog-first", never "1065 tools". A number typed here would
				// be a second copy of something stats.json already knows, and a
				// fifth pill under the largest type on the page turns a
				// qualifier into a paragraph. A chip with an href is a link and
				// takes the accent ring; one without is a plain label.
				chips: z.array(chipSchema).max(4).optional(),
			}),
		}),
	}),
	// The UI strings the documentation components supply for themselves: the
	// fact labels a <Fact kind> prints. One decision covers both locales
	// instead of a label typed per sheet per language.
	i18n: defineCollection({
		loader: i18nLoader(),
		schema: i18nSchema({
			extend: z.object({
				"gm.fact.exposes": z.string().optional(),
				"gm.fact.requires": z.string().optional(),
				"gm.fact.tier": z.string().optional(),
				"gm.fact.notCovered": z.string().optional(),
				"gm.fact.symptom": z.string().optional(),
				"gm.fact.meaning": z.string().optional(),
				"gm.fact.fixes": z.string().optional(),
			}),
		}),
	}),
};
