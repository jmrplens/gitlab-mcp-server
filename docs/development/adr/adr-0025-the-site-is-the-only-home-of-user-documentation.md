---
title: "ADR-0025: The site is the only home of user documentation"
status: "Accepted"
date: "2026-10-05"
authors: "jmrplens"
tags: ["documentation", "decision", "site", "i18n", "generators"]
supersedes: "ADR-0013"
superseded_by: ""
---

# ADR-0025: The site is the only home of user documentation

## Status

Accepted, 2026-10-05. It records the decision
[issue 1163](https://github.com/jmrplens/gitlab-mcp-server/issues/1163) took on 2026-10-04,
which settles step 9 of [issue 529](https://github.com/jmrplens/gitlab-mcp-server/issues/529)
(the relationship between `docs/` and the site), and it supersedes
[ADR-0013](adr-0013-documentation-artifact-boundaries.md). The changes it names landed as one
stack under issue 1163: the README statistics removed, the README cut to a front door, the
site brought level with the guides in both languages, the generated references written into
the site, every link pointed at it, this record and the contributor guidance, and the retired
copies deleted.

## Context

Until issue 1163 the user documentation lived twice. `docs/getting-started.md`, `docs/guides`,
`docs/reference` and `docs/concepts` were one English manual, read on GitHub. The Astro
Starlight site under `site/src/content/docs` was another, in English and in Spanish, published
at <https://jmrp.io/docs/gitlab-mcp-server/>. ADR-0013 drew boundaries between documentation
surfaces (stable documentation, the site, AI guidance, generated blocks, transient artifacts),
but it never said which of the two manuals a fact belonged to. Every user-facing fact had two
homes, and nothing held them to each other.

The [September 2026 documentation audit](../documentation-audit-2026-09.md#duplication)
measured the result: 118 files and some 248,000 words under `docs/` beside 46 pages per
language on the site, nineteen topics written twice, the two largest operator guides 58 and
60 per cent verbatim identical to their site twins with neither linking the other, the
environment variable table five times in English and the flag table five times. Its plan asked
for the relationship to be decided and written down (step 9), and declined to choose.

The two copies did not agree, and readers met the difference. Issue 1163 lists what it found:
the README listed Cline among the compatible clients and left out Codex while the site did the
reverse; the README's privacy section described an update check that `PRIVACY.md` says does
not exist; its Claude Code example passed the token on the command line, which the site tells
a reader never to do; and the site had no page for client configuration, the OAuth
application, the command line and environment references, the output format or the HTTP
rejection table, while its fine-grained token page lagged the guide by a dozen facts.

Bringing the site level showed that the guides were wrong too, each in places of its own.
`docs/concepts/meta-tools.md` said next steps reach the structured result on the meta surface
alone, through an `enrichWithHints` function the code does not have, while
`toolutil.FinishToolResult`, the tail every dispatcher applies, sets them on every surface for
any output type that declares them. `docs/concepts/error-handling.md` presented a
`DetailedError` card as the error format, and no production path returns one. `docs/guides/remote-deployment.md` said a renewed TLS certificate
takes effect only after a restart, while `docs/guides/http-server-mode.md` said, correctly,
that the next handshake picks it up (`cmd/server/tls_reload.go`). A second copy had not made
either copy right. It had given each copy mistakes of its own.

Two further pieces of the tree existed to make the duplication tolerable:

- **The per-domain tool reference** was 44 hand-written pages, some 14,300 lines, under
  `docs/reference/tools`. It drifted from the action catalog, and was held to it only by
  `cmd/audit_doc_coverage`, through `doc-ownership.json`, a map from tool-name prefixes to
  pages.
- **The README** had grown to 546 lines. Some 75 of them were a statistics section that
  `cmd/gen_stats` rewrote and that a CI step and a pre-push hook held current. Because it
  counted source lines, changing a comment made the README stale and refused the push.

## Decision

**The documentation site is the only home of the user documentation, in English and
Spanish.**

1. **The site.** Everything a user of the server reads is a page under
   `site/src/content/docs/<slug>.mdx`, with its Spanish twin at
   `site/src/content/docs/es/<slug>.mdx`. That covers installing the server, configuring a
   client, running it over stdio or HTTP, its configuration, flags and environment, the tool
   surfaces and capabilities, the per-domain tool reference, fine-grained permissions,
   security, privacy, performance and troubleshooting. `.github/workflows/pages.yml` builds the
   site from `main` and deploys it to GitHub Pages. Readers reach it at
   `https://jmrp.io/docs/gitlab-mcp-server/<slug>/`, a path-preserving redirect to the Pages
   origin.
2. **`docs/` is for contributors.** It keeps `docs/development` alone (the development guide,
   the command utilities, static analysis, the testing references, the ADRs, the upstream
   register, and the committed records the audits read), and `docs/README.md` points a reader
   at the site. `docs/concepts/graphql.md`, which explains how this repository writes and
   checks GraphQL rather than how a user runs the server, moves to
   `docs/development/graphql.md`. The contributor-only content of the retired pages is kept
   under `docs/development` as well. Other contributor documentation stays where it is read
   beside the code: `CONTRIBUTING.md`, `test/e2e/README.md`, `AGENTS.md`, `CLAUDE.md` and the
   guidance under `.github/`. The site links to it on GitHub and does not copy it.
3. **The README is a front door.** It says what the server is, states the default surface's
   startup cost (its one generated block), shows the install buttons, the one-line installs and
   the hosted endpoint, links the site, and gives a line each to contributing, security and the
   licence. It is some 120 lines.
4. **Files that another tool reads where it expects them stay there.** The package READMEs
   npm, PyPI and NuGet publish (`npm/gitlab-mcp-server/README.md`, `pypi/README.md`,
   `nuget/README.md`), the Docker Hub description (`.github/dockerhub-description.md`) and the
   agent installation guide `llms-install.md` stay where those tools read them, stay short,
   and link the site. The policies `SECURITY.md`, `CODE_OF_CONDUCT.md` and `PRIVACY.md` stay at
   the root, where GitHub looks for the first two and where the Claude Desktop extension
   manifest names the third as its privacy policy. They are policies, not pages of the manual.

### What is generated, and by which command

Every artifact below is written by its command and never by hand. A generated page carries an
MDX comment naming the command that writes it. A generated block inside a hand-written file
sits between `START` and `END` markers.

| Command                     | Writes                                                                                                                                                                                                                                                    | Regenerate                                                                   | Gate                                                                                          | CI step                                           |
| --------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------- | ------------------------------------------------- |
| `cmd/gen_action_grants`     | `site/src/content/docs/reference/fine-grained-permissions.mdx` and its `es/` twin, beside `docs/development/action-requests.json` and `internal/tools/actiongrants/table_gen.go`                                                                          | `make gen-action-grants`                                                     | `make check-action-grants`; `make check-action-grants-derivation` holds the derivation itself | "Check the fine-grained permissions per action"   |
| `cmd/gen_tool_reference`    | `site/src/content/docs/reference/tools/<group>.mdx`, one page per catalog group, and the `index.mdx` beside them, with their `es/` twins, from the action catalog and `cmd/gen_tool_reference/domains.json`                                               | `make gen-tool-reference`                                                    | `make check-tool-reference`                                                                   | "Check the per-domain tool reference"             |
| `cmd/bench_resources`       | `site/src/data/resource-benchmark.json` (the measurement record, written only by a measuring run), the SVG charts under `site/public/benchmarks`, and the block between the `BENCHMARK` markers of `performance/resource-benchmark.mdx` in both languages | `make bench-resources` measures; `make bench-resources-render` redraws       | `make check-bench-resources`                                                                  | "Check the committed benchmark charts and tables" |
| `cmd/audit_metrics`         | `site/src/data/stats.json`, the counts the site's pages import                                                                                                                                                                                            | `make gen-site-stats`                                                        | `make check-site-stats`                                                                       | "Check site stats"                                |
| `cmd/audit_tokens`          | `site/src/data/token-footprint.json`, the README's token claim (between the `TOKEN CLAIM` markers) and `docs/development/token-footprint.md`                                                                                                              | `make gen-footprint`                                                         | `make check-footprint`                                                                        | "Check token footprint"                           |
| `cmd/gen_llms`              | `llms.txt`, `llms-medium.txt`, `llms-full.txt` and the three `llms-full-*.txt` splits at the repository root, which the site's `prebuild` republishes                                                                                                     | `make gen-llms`                                                              | `make check-llms`                                                                             | "Check generated llms files"                      |
| `cmd/gen_testing_docs`      | the managed block of `docs/development/testing/testing.md`                                                                                                                                                                                                | `make gen-testing-docs`                                                      | `make check-testing-docs`                                                                     | "Check the generated testing reference"           |
| `cmd/gen_model_results`     | the five managed blocks of `docs/development/testing/model-results.md`, drawn from the record `docs/development/testing/model-results.json` once a run is folded (none is committed yet, and `make check-model-results` passes with a note until then)    | `make gen-model-results` redraws; `make model-results-record` folds a run in | `make check-model-results`                                                                    | "Check the published model evaluation results"    |
| `cmd/gen_lhm_manifest`      | the `tools`, `prompts` and `resources` arrays of `lhm.plugin.json`                                                                                                                                                                                        | `make gen-lhm-manifest`                                                      | `make check-lhm-manifest`                                                                     | "Check LobeHub manifest"                          |
| `site/scripts/gen-llms.mjs` | the site's own `/llms.txt` and `/es/llms.txt`, built from the content collection at `prebuild` and never committed, and the copies of the root llms files                                                                                                 | `pnpm run build` in `site/`                                                  | `pnpm run llms:check`                                                                         | "Static site gates" (`🎨 Site lint`)               |

The first nine CI steps are in the `🔎 Generated artifacts` job (`generated` in
`.github/workflows/ci.yml`). Each runs only when `FRESHNESS` is `checked`, so a stacked layer
below the top of its stack leaves the artifacts stale on purpose and the top refreshes them
once. `make check-action-grants-derivation` derives from the tree instead of comparing a
committed file, so it is never deferred. `make update-all` runs every Go command in the table,
using the redraw target where the measurement is slow or paid (`bench-resources-render`,
`gen-model-results`), and then formats the tables; the site's build runs the last row itself.

Other commands keep contributor records under `docs/development` in the same way:
`cmd/audit_e2e_coverage` (`docs/development/e2e-coverage.json` and
`docs/development/testing/e2e-coverage.md`), `cmd/gen_model_corpus`
(`docs/development/testing/model-corpus.md`), `cmd/gen_request_inventory`, `cmd/gen_api_live`
and `cmd/gen_orbit_record`. [`cmd-utilities.md`](../cmd-utilities.md#generators) describes
each.

### What stays hand-written, and who owns it

Hand-written:

- every site page not in the table, in both languages, with its frontmatter (title,
  description, FAQ, chips), and the prose around the generated block of the resource benchmark
  page;
- the sidebar in `site/astro.config.mjs`, where only `reference/tools` is listed with
  `autogenerate`;
- the section table (`SECTIONS`) of the site's llms index in `site/scripts/gen-llms.mjs`, and
  the landing data in `site/src/data/home.ts`;
- `cmd/gen_tool_reference/domains.json`: each catalog group's category, title, description,
  overview and sample questions, in both languages. It must name exactly the groups the
  catalog builds, or generation stops;
- `docs/development` outside the records its generators write;
- the README outside its token claim. Its opening count of actions is held to
  `site/src/data/stats.json` by `TestReadme_ActionCount_MatchesTheCommittedSiteStats` in
  `cmd/audit_metrics`.

**A page belongs to the change that makes it untrue.** A change to what the server does,
accepts, refuses or answers edits the site page that states it, in English and Spanish, in the
same change. A change to how the repository is built, tested or released edits
`docs/development`. A generated page belongs to its command: it changes through the command or
its input, and then the target in the table.

**An English page and its Spanish twin change together.** The twin sits at the same relative
path under `es/`. `pnpm run i18n:check` (`site/scripts/check-i18n-parity.mjs`) fails when an
English page has no Spanish twin or a Spanish page has no English original. That is all it
compares: it catches a page added, moved or deleted in one language, and not a sentence
changed in one language only. Two checks hold more of the structure. `pnpm run facts:check`
holds each spec sheet to the same sequence of fact kinds in both languages, and each table to
the same narrow-screen layout in both. `pnpm run chips:check` holds the page-title chips to the
same count, kinds and targets. What a page says in Spanish against what it says in English is
held by its author and its review, and by nothing else.

The gates that hold this layout are listed under [Compliance](#compliance).

### What was retired, and why

- **`docs/getting-started.md`, `docs/guides`, `docs/reference` and `docs/concepts`.** They were
  a second English copy of the user documentation, and it drifted from the site in both
  directions. Their content was carried onto the site, in both languages, with each fact
  checked against the code rather than copied, before they were deleted.
- **`cmd/gen_stats` and the README statistics**, with the `gen-stats`, `check-stats` and
  `gen-readme` targets and the CI step that ran `check-stats`. Nobody read the section, and
  because it counted source lines it made every comment change a refused push.
- **The README's token footprint table and its four model evaluation summaries.** The table's
  figures are on the site's dynamic tools page, read from `site/src/data/token-footprint.json`.
  The results page under `docs/development/testing` is the one place the model evaluation is
  published.
- **The hand-written per-domain tool pages under `docs/reference/tools`.** They are replaced by
  the pages `cmd/gen_tool_reference` writes from the catalog, so what a page says about an
  action is what the server serves. What an operator has to know before an action works and
  the quoted description does not say (a feature flag that ships disabled, a minimum GitLab
  release, a role, a transfer GitLab applies in the background, the directory a download needs
  to write in) goes in its group's overview in `domains.json`, which is where the caveats of
  the retired pages were carried.
- **`cmd/audit_doc_coverage` and `doc-ownership.json`.** They held the hand-written tool pages
  to the catalog. A page generated from the catalog cannot miss an action or name one the
  catalog lacks, so there was nothing left for them to hold.

### What ADR-0013 decided that still holds

This record supersedes ADR-0013 whole, so the parts of it that still hold are restated here:

- Documentation, on the site and under `docs/development` alike, describes durable behaviour,
  architecture, reference material and reproducible procedures. Implementation progress, local
  test output, workstation benchmark snapshots, one-off model run traces and pull request
  status go elsewhere: to `plan/`, which git ignores and `make check-plan-untracked` keeps
  untracked; to an ADR when they justify a decision; to ignored output under `dist/`; or to the
  pull request.
- A generated block is allowed where a command in `cmd/` maintains it, and that command is its
  source of truth. Its markers are not removed unless the command changes with them.
- Mermaid is preferred over ASCII diagrams, unless the exact text layout is itself the
  artifact. The site renders Mermaid at build time, and GitHub renders it in
  `docs/development`.

## Consequences

### Positive

- **POS-001**: A user-facing fact has one home per language. A correction lands once in English
  and once in Spanish, and no second English copy can contradict it.
- **POS-002**: The reference that can be derived is derived. The per-domain tool reference, the
  fine-grained permissions, the benchmark tables and charts, the published counts and the token
  figures are written from the catalog, the live GitLab record and the measurement records, so
  they change when what they describe changes.
- **POS-003**: The site is held like the code. It is built, link-checked and linted on every
  change rather than after a merge to `main`, and the files outside it link into it through
  addresses `check-doc-links` resolves to a page and a heading.
- **POS-004**: The README stays right because it says little. Its one figure is generated and
  checked (`make check-footprint`), its one hand-written count is held to the site stats by a
  test, and a comment change no longer refuses a push.
- **POS-005**: The Spanish pages are the twin of the one manual rather than a translation kept
  beside a fuller English manual elsewhere.

### Negative

- **NEG-001**: A contributor reading on GitHub follows a link out to the site for anything
  user-facing. The page sources under `site/src/content/docs` are in the repository, but they
  are MDX whose components and imported figures only the site build renders.
- **NEG-002**: Every user-facing change is written twice, in English and in Spanish, and only
  the existence and the structure of the twin is checked. A sentence changed in one language
  and not in the other passes every gate.
- **NEG-003**: The site describes `main`, because `pages.yml` builds it from there. A user of an
  older release reads pages about the newest code, and finds that release's pages only in the
  repository at its tag.
- **NEG-004**: The Pages workflow redeploys only on what its `push` trigger names, or by hand.
  A change to the repository-root llms files alone, which the site's build republishes, used to
  reach the documentation domain with the next site change rather than with its own. That is
  closed: the trigger names `site/**`, the six root `llms*.txt` files and `VERSION`, which the
  build reads into its `/llms.txt` index. What stays is the obligation behind it: a file outside
  `site/` that the build starts to read joins that trigger in the same change.
- **NEG-005**: Editing the site with its checks needs the site's toolchain: Node at the version
  `site/.node-version` pins and pnpm at the version `site/package.json` pins, and a Playwright
  Chromium for the Mermaid render of a full build. A page under `docs/` needed markdownlint
  alone.
- **NEG-006**: A link anyone took to a page under `docs/guides`, `docs/reference` or
  `docs/concepts`, or to `docs/getting-started.md`, on `main` now answers 404, and GitHub does
  not redirect a deleted file. Such links were published: the `llms.txt` of 3.1.0, which the
  documentation domain republished as `/llms-server.txt`, carried 17 absolute `blob/main` links
  into those trees, and the README of 3.1.0 carried 28 relative ones, which a reader browsing
  `main` copies as `main` addresses. The regenerated `llms.txt` points at the site, but a copy
  taken earlier does not. The page is found on the site by its slug, or in the repository at a
  release tag.

## Alternatives Considered

### Keep the mirror

- **ALT-001**: **Description**: Keep `docs/guides`, `docs/reference` and `docs/concepts` as an
  English manual beside the site, each page carrying a banner naming its twin, which is the
  "peers that cross-link" answer the audit's step 9 offered.
- **ALT-002**: **Rejection Reason**: The audit and the work of bringing the site level measured
  what the mirror costs: two manuals that agreed where one repeated the other word for word and
  drifted apart everywhere else, each carrying mistakes of its own, and no gate that can compare
  two prose copies. A banner tells a reader that another copy exists. It does not tell them which
  copy is right.

### Generate the site from `docs/`

- **ALT-003**: **Description**: Keep `docs/` as the one source, and render the site's pages
  from it.
- **ALT-004**: **Rejection Reason**: The site's pages are not plain Markdown. They are MDX that
  import the generated figures (`site/src/data/stats.json`, `token-footprint.json`), carry FAQ
  and chip frontmatter, and use Starlight's components and the spec-sheet component the site's
  own checks read. A `docs/` source would have to become MDX to carry them, at which point it
  is the site under another directory, and GitHub would render it no better. The Spanish half
  has no source under `docs/` to be generated from. And the references that can be generated
  are already written by Go commands straight into the site, so a `docs/` source would put a
  step between them and their page.

### Publish the contributor documentation on the site as well

- **ALT-005**: **Description**: Move `docs/development` onto the site, so that all
  documentation is in one place.
- **ALT-006**: **Rejection Reason**: Contributor documentation is read beside the code it
  describes, names repository paths by relative links `check-doc-links` holds, and changes in
  the same change as that code. The site's reader is a user of the server. When the site was
  brought level, its Docker end-to-end page moved the other way, to `test/e2e/README.md`
  beside the suite it describes.

## Compliance

These gates hold the layout:

| Gate                                                                 | What it holds                                                                                                                                                                                                                                                                                                                                                                                                                                           | Where it runs in CI                                                          |
| -------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------- |
| `scripts/check-doc-links.mjs` (`make check-doc-links`)               | Every tracked Markdown and MDX file outside `plan/`, the skills under `.github/skills/` included: its relative links and anchors, and every link to `https://jmrp.io/docs/gitlab-mcp-server/...` or `https://jmrplens.github.io/gitlab-mcp-server/...`, resolved to the page under `site/src/content/docs` that serves the route and to that page's Starlight heading anchors. It reads tracked files only, so a new file is checked once git tracks it | `📝 Markdown` (`analyze-md`), "Check documentation local links"               |
| `pnpm run i18n:check`, `facts:check`, `chips:check` and `llms:check` | The language pairs, the spec sheets, the page-title chips (each chip target must resolve too), and the site's llms index, which must list every page of the content collection                                                                                                                                                                                                                                                                          | `🎨 Site lint` (`site-lint`), "Static site gates"                             |
| `cmd/audit_doc_tool_names` (`make check-doc-tool-names`)             | Every `gitlab_*` tool name and every `domain.action` ID in `docs/`, `site/src/content/docs`, `README.md`, `llms-install.md`, `CLAUDE.md` and the npm launcher's README, against the tools the server registers and the catalog it builds. ADRs are exempt as historical records                                                                                                                                                                         | `🔎 Generated artifacts`, "Check documented tool names exist", never deferred |
| `make check-md-tables` (`go run ./cmd/format_md_tables/ --check`)    | Every pipe table in `README.md`, `docs/` and `site/src/content/docs` in the shape the formatter writes. The site's `.prettierignore` leaves the site's tables to it                                                                                                                                                                                                                                                                                     | `🔎 Generated artifacts`, "Check Markdown tables are formatted"               |
| The site build (`pnpm run build` in `site/`)                         | starlight-links-validator holds root-relative links and anchors between pages, and rehype-mermaid renders every Mermaid block; html-validate, htmlhint and the headings-and-labels check then read the built pages                                                                                                                                                                                                                                      | `📚 Site build` (`site-build`), on every change                               |
| markdownlint                                                         | Every `.md` and `.mdx` file outside `plan/` and `node_modules`                                                                                                                                                                                                                                                                                                                                                                                          | `📝 Markdown` (`analyze-md`)                                                  |

The `🧰 Checks` verdict waits on every job in this table. `make audit-docs` runs most of these
gates in one local target: markdownlint, the table check, the llms, manifest, tool reference,
testing and site stats checks, the link check, and the site's check, build and lint.
`make check-doc-tool-names` is not among them and runs on its own.

Beside them, `TestReadme_ActionCount_MatchesTheCommittedSiteStats` in
`cmd/audit_metrics/site_stats_test.go` holds the README's one hand-written count to
`site/src/data/stats.json`, and each generator in the table under
[What is generated](#what-is-generated-and-by-which-command) has the `check-*` gate and CI
step named beside it.

## Implementation Notes

- **IMP-001**: A new site page is two files at the same relative path, under
  `site/src/content/docs` and under its `es/` directory. It needs an entry in the sidebar in
  `site/astro.config.mjs` unless its directory is listed with `autogenerate`, and an entry in
  `SECTIONS` in `site/scripts/gen-llms.mjs` unless it sits in a section's generated directory;
  `pnpm run llms:check` fails otherwise.
- **IMP-002**: A file outside the site links a page by its absolute address,
  `https://jmrp.io/docs/gitlab-mcp-server/<slug>/` (the Spanish page at `.../es/<slug>/`). A
  page inside the site links another as `/gitlab-mcp-server/<slug>/`, and a repository file by
  its GitHub URL. An anchor is the target page's Starlight heading slug.
- **IMP-003**: A generated page or block changes through its command or its input, followed by
  the target in the table. `make update-all` runs every Go command in that table.
- **IMP-004**: Locally, `make audit-docs` runs most of the documentation gate in one target.
  In `site/`, `pnpm run analyze` builds the site and runs its lint, part of which reads the
  built pages. The four parity and index checks run without a build: `pnpm run i18n:check`,
  `facts:check`, `chips:check` and `llms:check`.

## References

- **REF-001**: [ADR-0013: Documentation artifact boundaries](adr-0013-documentation-artifact-boundaries.md),
  superseded by this record.
- **REF-002**: [ADR-0014: Catalog-first runtime architecture](adr-0014-catalog-first-runtime-architecture.md),
  the catalog the generated tool reference is written from.
- **REF-003**: [Issue 1163](https://github.com/jmrplens/gitlab-mcp-server/issues/1163), the
  decision, and [issue 529](https://github.com/jmrplens/gitlab-mcp-server/issues/529), whose
  step 9 it settles.
- **REF-004**: [Documentation audit, September 2026](../documentation-audit-2026-09.md): its
  [Duplication](../documentation-audit-2026-09.md#duplication) measurements and
  [plan](../documentation-audit-2026-09.md#the-plan-in-execution-order).
- **REF-005**: [Command utilities](../cmd-utilities.md): the [generators](../cmd-utilities.md#generators)
  and [benchmarks](../cmd-utilities.md#benchmarks).
- **REF-006**: The site: [home](https://jmrp.io/docs/gitlab-mcp-server/),
  [tools by domain](https://jmrp.io/docs/gitlab-mcp-server/reference/tools/) and
  [fine-grained permissions](https://jmrp.io/docs/gitlab-mcp-server/reference/fine-grained-permissions/).
