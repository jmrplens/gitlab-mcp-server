# Starlight UI Translations

This directory holds the UI strings of the site, one file per locale (`en.json`
and `es.json`).

Starlight loads only `.json`, `.yml`, and `.yaml` files from this collection.
The locale files override none of Starlight's own UI strings. They carry the
strings the documentation components supply for themselves: the `gm.fact.*`
labels a `<Fact kind>` prints (`site/src/components/docs/Fact.astro`). A key
is declared in three places, which change together: both locale files, the
`i18n` collection schema in `site/src/content.config.ts`, and the
`StarlightApp.I18n` interface in `site/types/starlight-i18n.d.ts`, without
which the component's call to `t` does not type-check.

Page-level SEO metadata belongs in each documentation page frontmatter, such as
`title` and `description` in `site/src/content/docs/` and localized pages under
`site/src/content/docs/es/`.
