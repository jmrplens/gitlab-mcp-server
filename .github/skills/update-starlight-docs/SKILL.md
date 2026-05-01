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
| New MCP tool | `tools-reference`, relevant domain page |
| New config option | `configuration`, `env-reference` |
| New capability | `capabilities`, getting-started |
| Transport change | `getting-started`, `http-server-mode` |
| Error handling change | `troubleshooting`, `error-handling` |
| Security change | `security` |
| Auto-update change | `auto-update` |

### 2. Edit EN pages first

All pages live under `site/src/content/docs/en/`:

```text
site/src/content/docs/en/
├── index.mdx          # Landing page
├── getting-started.mdx
├── configuration.mdx
├── tools-reference.mdx
├── ...
```

### 3. Edit corresponding ES pages

Mirror structure under `site/src/content/docs/es/` with translated content.

### 4. Frontmatter requirements

Every `.mdx` file must have:

```yaml
---
title: "Page Title"
description: "Brief description for SEO and sidebar"
sidebar:
  order: 5  # Controls sidebar ordering
---
```

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
- Maintain consistent sidebar ordering across locales
- Use Starlight components (Aside, Tabs, etc.) instead of raw HTML
- Link between Starlight pages with relative paths (e.g., `./configuration`)
- Do NOT modify `astro.config.mjs` or `src/content.config.ts` unless adding a new content collection
- Images go in `site/src/assets/` and are referenced with relative imports

## Validation Checklist

- [ ] All affected EN pages updated
- [ ] All affected ES pages updated with translated content
- [ ] Frontmatter (title, description, sidebar order) is correct
- [ ] Starlight components used correctly (imports present)
- [ ] `cd site && pnpm run build` succeeds with zero errors
- [ ] No broken internal links between pages
- [ ] Developer docs (`docs/`) also updated if applicable
