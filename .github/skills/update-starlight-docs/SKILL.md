---
name: update-starlight-docs
description: "Update the user documentation, which lives only in the Astro Starlight site (site/src/content/docs/, English, with its Spanish twin under es/), when code changes affect user-facing features. Use when: adding new tools, changing configuration, updating deployment, modifying capabilities."
---

# Update Starlight Documentation

Update the Astro Starlight user documentation site to reflect code changes that affect user-facing features, configuration, or behavior.

## Before Starting

1. Identify what changed in the code that affects users
2. Read the current Starlight docs structure: `site/src/content/docs/`
3. Determine affected pages (EN and ES)
4. Check whether a generator writes the page or the figure (table below); if one does, change its source and run it instead of editing

## Where Documentation Lives

The site is the only home of the user documentation, in English and Spanish. `docs/` keeps contributor material only, under `docs/development/`, and the README is a short landing page that links the site (ADR-0025).

| Audience | Path | Format |
|----------|------|--------|
| Users | `site/src/content/docs/` (English) and `site/src/content/docs/es/` (Spanish) | MDX (Starlight) |
| Contributors and AI agents | `docs/development/`, `CLAUDE.md`, `AGENTS.md`, `.github/` | Markdown |

**Rule**: a change to user-facing behavior updates the site page that describes it, in English and in Spanish, in the same change. A contributor-only change (a gate, a generator, a convention) goes to `docs/development/` or `CLAUDE.md`, and never to the site. There is no second copy of a user page anywhere in the repository to keep in step.

### Pages and data a generator writes

Never edit these by hand: change the source the generator reads, then run it. `make update-all` runs every Go command in this table, the benchmark through its redraw target; the last row is the site build's own.

| What | Source | Command |
|------|--------|---------|
| `reference/tools/<group>.mdx` and its `es/` twin, one page per catalog group, named after the group's tool name without `gitlab_` and with hyphens (`gitlab_merge_request` is `merge-request.mdx`), and the `index.mdx` beside them | the action catalog and `cmd/gen_tool_reference/domains.json` | `make gen-tool-reference` |
| `reference/fine-grained-permissions.mdx` and its `es/` twin | the handlers and `docs/development/gitlab-api-live.json` | `make gen-action-grants` |
| The block between `{/*START BENCHMARK*/}` and `{/*END BENCHMARK*/}` of `performance/resource-benchmark.mdx` in both languages, the charts under `site/public/benchmarks/`, and `site/src/data/resource-benchmark.json` | a measurement of the binary | `make bench-resources` measures; `make bench-resources-render` redraws from the committed record |
| `site/src/data/stats.json`, the counts (tools, meta-tools, actions, groups, resources, prompts) the pages print as `{stats...}` | the catalog and the registered surfaces | `make gen-site-stats` |
| `site/src/data/token-footprint.json` | a token measurement of every tier, surface and meta schema mode | `make gen-footprint` |
| The site's own `/llms.txt` and `/es/llms.txt` | the content collection and the `SECTIONS` table of `site/scripts/gen-llms.mjs` | written by `pnpm run build`, never committed |

A page that states one of the counts `stats.json` carries imports it and prints the field, rather than writing the figure.

### The release a page names

A page never writes the current release as a number. Where it means the release a reader installs today (an install command, an image tag, a package pin, a download URL or tag ref, sample `--version` or `/health` output, "the current release is"), it writes `%%VERSION%%`, and the build replaces it with the published release: the top-level `version` of the repository's `server.json`. That is the release a reader can download. `VERSION` is not read, because it names the release being prepared and moves days before that release exists (3.1.0: `VERSION` moved on 2026-09-17, the tag came on 2026-09-30), so a command built from it would name a release nobody can download yet. `server.json` is stamped by `scripts/update-server-json-sha.sh` in the release workflow's last job, once the release, its images and its npm, PyPI and NuGet packages are published, and that commit reaches `main` with a deploy key, so the Pages workflow redeploys on it. `site/src/lib/version-token.mjs` holds the reader and the remark plugin, which `site/astro.config.mjs` runs before every other one, so the token works in prose, inline code, fenced code blocks (Expressive Code receives them already substituted), tables, link and image URLs, and MDX component attributes, in English and Spanish alike. A release then moves every page with no edit:

```mdx
curl -fsSLO https://github.com/jmrplens/gitlab-mcp-server/releases/download/v%%VERSION%%/gitlab-mcp-server-linux-amd64

The current release is **v%%VERSION%%**.
```

A number stays literal when it is history, whatever the published release is today: "removed in 3.1.0", "since 3.0.0", "up to 3.1.0", "from the first release after 3.1.0", or a measurement taken on one release ("checked on 3.1.0"). Decide by what the sentence means, not by whether the number matches. Avoid a condition naming the latest release ("the one to download while the latest release is 3.1.0"): it turns false the day the next one publishes, so write what stays true whichever release is the latest ("the one to download when the latest release predates the per-system bundles"). A value that belongs to one release and is not the release number (an image digest, a release date) is not written either: show where to read it, as `install/docker` and `operations/remote-deployment` read the image reference with its digest out of `server.json` on `main`, which records what the release published rather than what a tag points at now.

The token is replaced in the page body only. Frontmatter (`title`, `description`, `faq`, `chips`) is parsed and cached by the content layer outside the Markdown pipeline and copied verbatim into the site's `/llms.txt`, so it never carries the token; and `{stats.version}` is the copy of `VERSION` that `make gen-site-stats` writes into `stats.json`, the release being prepared, so the token replaces it. `pnpm run version:check` (`site/scripts/check-version-token.mjs`, part of `pnpm run lint` and of CI's site lint) refuses both, and a literal release in a shape that means the current one (an install command, image tag, package pin, download URL, tag ref, `--version` or `/health` output, or "current release", "latest release", "release actual", "última release" before a number), whatever the number. A literal in one of those shapes kept on purpose is declared in `site/scripts/version-literals.mjs` with its category (`measurement`, or `condition` for one true only while that release is the published one) and its reason, and a declaration that matches nothing, lacks either, or is a condition about a release the published one has moved past fails the check. That last failure arrives with the commit the release workflow stamps `server.json` with, on `main`, which is why a sentence that stays true is better than a condition. The build's postbuild step fails when `%%VERSION%%` survives anywhere in `dist/`, the llms indexes included, or when a page's `<main>` carries the release fewer times than its source wrote the token and the number together.

## Steps

### 1. Map code changes to affected docs

| Code Change | User Doc Pages |
|-------------|---------------|
| New action or catalog group | the generated `reference/tools/` pages and `stats.json` (above); hand-written: `tools/overview`, `tools/meta-tools` (its per-meta-tool action count table) or `tools/dynamic-tools`, and `tools/orbit` for GitLab.com Orbit tools |
| New resource or prompt | `tools/resources-prompts`; a resource kind that can be watched, `capabilities/subscriptions` too |
| New config option | `configuration`, `reference/environment`, and `reference/cli` when it has a flag |
| New capability | `capabilities/overview` and the capability's own page under `capabilities/`, `getting-started` |
| Transport change | `getting-started`, `operations/http-server` |
| Output format change | `reference/output-format` |
| Error handling change | `operations/troubleshooting`, `operations/error-handling` |
| Security or authentication change | `operations/security`; `operations/oauth-app` and `operations/fine-grained-tokens` when the change is about those |
| Telemetry change | `operations/telemetry`, `operations/privacy` |
| Deployment change | `operations/remote-deployment` and the pages under `enterprise/` |
| Installation channel change | the channel page under `install/` and `install/overview`; the Claude Desktop bundle, `claude-desktop` |
| Client behavior change | `install/clients`, `compatibility` |

The same changes often have a contributor half, which goes under `docs/development` and never on the site: `architecture.md` for a new package or a change to the path a call takes, `error-handling.md` for an error handling change, `capabilities.md` for a capability or icon change, and `distribution.md` for a change to how a bundle or channel is built or published. The `update-project-documentation` skill maps those.

### 2. Edit EN pages first

English is the Starlight `root` locale, so the English pages live directly under `site/src/content/docs/` (there is no `en/` folder):

```text
site/src/content/docs/
├── index.mdx          # Landing page
├── getting-started.mdx
├── configuration.mdx
├── architecture.mdx
├── about.mdx, changelog.mdx, claude-desktop.mdx, comparison.mdx, compatibility.mdx, glossary.mdx, use-cases.mdx
├── capabilities/      # overview + one page per capability
├── enterprise/        # overview, load-balancing, tls, mcp-gateways, operations
├── examples/          # usage + workflow examples
├── install/           # overview, clients, and one page per distribution channel
├── operations/        # http-server, oauth-app, fine-grained-tokens, remote-deployment, security, privacy, telemetry, error-handling, ci-cd, troubleshooting
├── performance/       # resource-benchmark (generated block), sizing
├── reference/         # cli, environment, output-format, fine-grained-permissions (generated), tools/ (generated)
├── tools/             # overview, meta-tools, dynamic-tools, orbit, resources-prompts
└── es/                # the Spanish twin of every page above, at the same path
```

A page's slug is its path without the extension: `operations/http-server.mdx` is `operations/http-server`, published at `https://jmrp.io/docs/gitlab-mcp-server/operations/http-server/`.

### 3. Edit corresponding ES pages

Edit the twin at the same path under `site/src/content/docs/es/`, with translated content. `pnpm run i18n:check` fails when an English page has no Spanish twin or the reverse; it cannot tell whether the two say the same thing, so translating the change is your job.

### 4. Frontmatter requirements

Every `.mdx` file must have `title` and `description`. Most pages also carry `datePublished` (the TechArticle JSON-LD in `src/components/Head.astro`) and many a `faq` list (the FAQPage JSON-LD and the `<FAQ />` component); a few carry `chips`, which `pnpm run chips:check` holds to the Spanish twin. `src/content.config.ts` declares all three. Copy the shape of a neighbouring page:

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

Sidebar position is not set in frontmatter: the sidebar is the explicit `sidebar` array in `site/astro.config.mjs`, where every entry names a `slug`, a `label` and its `translations.es` label. The one exception is the generated per-domain tool reference, which the sidebar lists with `autogenerate` over `reference/tools`, so a catalog group the generator adds needs no edit there.

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
cd site
pnpm install --frozen-lockfile
pnpm exec playwright install chromium   # once: rehype-mermaid renders every Mermaid block with it
pnpm run build                          # also runs starlight-links-validator over every internal link and anchor
pnpm run lint                           # type check, contrast, chips, facts, i18n, llms, version, eslint, prettier, then the dist gates
```

Must produce zero errors. The build regenerates the site's `/llms.txt` first (`prebuild`), and fails when a page is missing from its `SECTIONS` table. From the repository root, also run the gates CI runs on content:

```bash
npx markdownlint-cli2 site/src/content/docs/<page>.mdx site/src/content/docs/es/<page>.mdx
go run ./cmd/format_md_tables/ --check   # the site's tables belong to this formatter, not to prettier
make check-doc-tool-names                # every gitlab_* name and domain.action ID a page teaches exists
make check-doc-links                     # every relative link and jmrp.io/docs/gitlab-mcp-server URL in a tracked .md or .mdx file resolves, anchor included
```

## Rules

- Always update the EN and ES pages together, in the same change
- Keep ES translations accurate: do not leave English text in ES pages
- Never edit a generated page or data file (the table under "Where Documentation Lives"); change its source and run its generator
- Write the release a reader installs today as `%%VERSION%%`, never as a number; keep a literal release only for history (see "The release a page names")
- Touch `site/astro.config.mjs` for two reasons only: a page added, renamed or removed changes the `sidebar` array (slug, label and `translations.es`), and a page that moves or leaves the site gets an entry in its `redirects` map for its old URL, English and Spanish, so a bookmark or an inbound link lands where the content went
- A page added, renamed or removed also changes the `SECTIONS` table of `site/scripts/gen-llms.mjs`, in the order the sidebar shows it
- Use Starlight components (Aside, Tabs, etc.) instead of raw HTML
- Link between site pages by their published path: `/gitlab-mcp-server/<slug>/` in an English page and `/gitlab-mcp-server/es/<slug>/` in its Spanish twin, with an anchor read from the target's heading. A file outside the site links a page as `https://jmrp.io/docs/gitlab-mcp-server/<slug>/`
- Do NOT modify `src/content.config.ts` unless adding a new content collection
- Images a page imports go in `site/src/assets/`; the benchmark charts under `site/public/benchmarks/` are generated

## Validation Checklist

- [ ] All affected EN pages updated
- [ ] All affected ES pages updated with translated content
- [ ] No generated page or data file edited by hand; the generator was run instead
- [ ] The current release written as `%%VERSION%%`, in the body and never in frontmatter; a literal release only where it is history
- [ ] Frontmatter (title, description, and the chips/datePublished/faq fields the neighbouring pages carry) is correct
- [ ] New, renamed or removed pages reflected in the `sidebar` array of `site/astro.config.mjs` (with their Spanish label), in `SECTIONS` of `site/scripts/gen-llms.mjs`, and, for a page that moved or left, in `redirects`
- [ ] Starlight components used correctly (imports present)
- [ ] `cd site && pnpm run build` succeeds with zero errors, and `pnpm run lint` passes
- [ ] markdownlint, `go run ./cmd/format_md_tables/ --check`, `make check-doc-tool-names` and `make check-doc-links` pass
- [ ] A contributor-facing part of the change (a gate, a generator, a convention) documented in `docs/development/` or `CLAUDE.md`, not on the site
