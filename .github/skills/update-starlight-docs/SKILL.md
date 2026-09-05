---
name: update-starlight-docs
description: "Update Astro Starlight user documentation (site/src/content/docs/) when code changes affect user-facing features. Use when: adding new tools, changing configuration, updating deployment, modifying capabilities."
---

# Update Starlight Documentation

Update the Astro Starlight user documentation site to reflect code changes that affect user-facing features, configuration, or behavior.

## Before Starting

1. Identify what changed in the code that affects users
2. Read the current Starlight docs structure: `site/src/content/docs/`
3. Determine affected pages (EN and ES)

## Documentation Architecture

Two documentation systems coexist:

| System | Path | Audience | Format |
|--------|------|----------|--------|
| Developer docs | `docs/` | Contributors, AI agents | Markdown |
| User docs | `site/src/content/docs/` | End users | MDX (Starlight) |

**Rule**: Code changes that affect user-facing behavior MUST update BOTH systems.

## Steps

### 1. Map code changes to affected docs

| Code Change | User Doc Pages |
|-------------|---------------|
| New MCP tool | `tools/overview`, `tools/meta-tools` or `tools/dynamic-tools` (`tools/orbit` for GitLab.com Orbit tools) |
| New config option | `configuration` |
| New capability | `capabilities/overview` and the capability's own page under `capabilities/`, `getting-started` |
| Transport change | `getting-started`, `operations/http-server` |
| Error handling change | `operations/troubleshooting`, `operations/error-handling` |
| Security change | `operations/security` |
| Installation channel change | the channel page under `install/` and `install/overview` |

### 2. Edit EN pages first

English is the Starlight `root` locale, so the English pages live directly under `site/src/content/docs/` (there is no `en/` folder):

```text
site/src/content/docs/
├── index.mdx          # Landing page
├── getting-started.mdx
├── configuration.mdx
├── architecture.mdx
├── capabilities/      # overview + one page per capability
├── install/           # one page per distribution channel
├── operations/        # http-server, remote-deployment, security, telemetry, troubleshooting, ...
├── tools/             # overview, meta-tools, dynamic-tools, orbit, resources-prompts
├── examples/
└── es/                # Spanish mirror of everything above
```

### 3. Edit corresponding ES pages

Mirror structure under `site/src/content/docs/es/` with translated content.

### 4. Frontmatter requirements

Every `.mdx` file must have `title` and `description`; the existing pages also carry `chips`, `datePublished` and `faq`, which the site's checks read, so copy the shape of a neighbouring page:

```yaml
---
title: "Page Title"
description: "Brief description for SEO and search"
chips:
  - text: "One short fact"
datePublished: "YYYY-MM-DD"
faq:
  - q: "A question a reader of this page asks?"
    a: "Its answer, in one or two sentences; the site renders the list where the page places <FAQ />."
---
```

Sidebar position is not set in frontmatter: the sidebar is the explicit `sidebar` array in `site/astro.config.mjs`, where every entry names a `slug`, a `label` and its `translations.es` label.

### 5. Use Starlight components

```mdx
import { Aside, Tabs, TabItem, Card, CardGrid, Steps, FileTree, LinkCard } from '@astrojs/starlight/components';

<Aside type="tip">Helpful tip here</Aside>
<Aside type="caution">Warning message</Aside>
<Aside type="danger">Critical warning</Aside>

<Tabs>
  <TabItem label="Linux">Linux instructions</TabItem>
  <TabItem label="macOS">macOS instructions</TabItem>
  <TabItem label="Windows">Windows instructions</TabItem>
</Tabs>

<Steps>
1. First step
2. Second step
3. Third step
</Steps>
```

### 6. Build verification

```bash
cd site && pnpm run build
```

Must produce zero errors. Check `site/dist/` for output.

## Rules

- Always update BOTH EN and ES pages
- Keep ES translations accurate — do not leave English text in ES pages
- A new page must be added to the `sidebar` array in `site/astro.config.mjs` (slug, label and `translations.es`), which keeps both locales in the same order; that array is the only reason to touch the file
- Use Starlight components (Aside, Tabs, etc.) instead of raw HTML
- Link between Starlight pages with relative paths (e.g., `./configuration`)
- Do NOT modify `src/content.config.ts` unless adding a new content collection
- Images go in `site/src/assets/` and are referenced with relative imports

## Validation Checklist

- [ ] All affected EN pages updated
- [ ] All affected ES pages updated with translated content
- [ ] Frontmatter (title, description, and the chips/datePublished/faq fields the neighbouring pages carry) is correct
- [ ] New pages listed in the `sidebar` array of `site/astro.config.mjs` with their Spanish label
- [ ] Starlight components used correctly (imports present)
- [ ] `cd site && pnpm run build` succeeds with zero errors, and `pnpm run lint` (the site's own checks: i18n parity, links, a11y, facts) passes
- [ ] No broken internal links between pages
- [ ] Developer docs (`docs/`) also updated if applicable
