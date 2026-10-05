---
name: update-project-documentation
description: 'Update existing project documentation to maintain parity with source code changes: the site pages (English and Spanish together) for users, docs/development for contributors. Analyzes code diffs, identifies documentation gaps, and surgically updates affected documents while preserving structure and style.'
---

# Update Project Documentation

## Primary Directive

Update existing documentation to reflect current source code changes. Perform a delta analysis between the implementation and documentation, then surgically update only the affected sections while preserving the overall document structure, style, and formatting.

The documentation lives in two places, one per audience (ADR-0025):

- **User documentation** is the site only: `site/src/content/docs/` in English and its Spanish twin under `site/src/content/docs/es/`, published at `https://jmrp.io/docs/gitlab-mcp-server/`. An English page and its twin change together. Follow the `update-starlight-docs` skill for the page map, the generated pages and the site's checks.
- **Contributor documentation** is `docs/development/`, plus `CLAUDE.md`, `AGENTS.md` and `.github/` for the guidance AI assistants read. Beside `docs/development/`, `docs/` holds only a README that points at the site.

## Execution Context

This skill is triggered after code changes to ensure documentation stays in sync. It focuses on efficiency — only updating what has changed rather than regenerating entire documents.

## Analysis Phase

### Step 1: Identify Changed Code

1. Check recent changes using git diff or by examining the files provided
2. If no specific files are indicated, scan all Go source files for modifications
3. Build a list of changed exported types, functions, constants, and configurations
4. Identify new, modified, or removed public APIs

### Step 2: Map Changes to Documentation

For each code change, identify which documentation files are affected:

Site pages are named by slug: `operations/http-server` is `site/src/content/docs/operations/http-server.mdx` and `site/src/content/docs/es/operations/http-server.mdx`.

| Change Type | Affected Documentation |
|-------------|----------------------|
| New exported type/function | Its godoc comment (`make audit-godocs-check`), possibly the tools or resources pages of the site |
| Modified function signature | Godoc comment, tools reference, examples |
| New MCP tool | The site's per-domain tool reference, which is generated: run `make gen-tool-reference` (a new catalog group first needs its entry in `cmd/gen_tool_reference/domains.json`), `make gen-site-stats` for the counts the site prints and `make gen-action-grants` for the fine-grained table, which holds one row per action |
| New MCP resource or prompt | Site page `tools/resources-prompts`; `docs/development/architecture.md` (Resources and prompts: where each lives) |
| Configuration change | Site pages `configuration`, `reference/environment` and `reference/cli`, the `CLAUDE.md` variable and flag tables, and the environment variable and flag tables in `.github/copilot-instructions.md` |
| New package | The Project Structure of `docs/development/development.md`, the packages tables of `docs/development/architecture.md` and the project tree in `CLAUDE.md`; `docs/development/cmd-utilities.md` for a new `cmd/` utility |
| Architecture change | Site page `architecture` for what a user sees (transports, surfaces, security model), diagrams included; `docs/development/architecture.md` (packages, the path a call takes, handler patterns), `docs/development/development.md` and `docs/development/tool-surfaces-and-action-core.md` for how the code is put together |
| Error handling change | Site pages `operations/error-handling` and `operations/troubleshooting`; `docs/development/error-handling.md` for the classification and wrapping functions |
| Capability change | Site pages under `capabilities/`; `docs/development/capabilities.md` for the progress, elicitation and completion APIs and the icons |
| Build/deploy change | `docs/development/development.md`, and `docs/development/distribution.md` for the bundles and the channel publishers; site pages under `install/` and `operations/remote-deployment` |
| Removed API | All referencing documents |

### Step 3: Assess Impact

For each affected document:

1. Read the current documentation
2. Compare with the current source code
3. Classify the update as: **Minor** (parameter change), **Moderate** (new section), or **Major** (restructure)
4. Prioritize Critical and High priority documents first

## Update Strategy

### Principles

- **UPD-001**: Preserve existing document structure, heading hierarchy, and formatting style
- **UPD-002**: Use surgical edits — replace only the changed sections, not entire files
- **UPD-003**: Maintain cross-reference integrity — check all links still work
- **UPD-004**: Update Mermaid diagrams if component relationships changed
- **UPD-005**: Update tables (parameters, types, functions) to match source
- **UPD-006**: Add deprecation notices for removed APIs rather than deleting immediately
- **UPD-007**: Never introduce TBD/TODO placeholders in updates
- **UPD-008**: Maintain consistent terminology with the rest of the documentation
- **UPD-009**: When creating or editing Markdown pipe tables in `README.md`, `docs/` or the site's pages, run `go run ./cmd/format_md_tables/` and verify with `go run ./cmd/format_md_tables/ --check` so source tables keep consistent padding and alignment markers
- **UPD-010**: Never hand-edit generator-owned content: the README's token claim block, the managed block of `docs/development/testing/testing.md`, `model-corpus.md` and `e2e-coverage.md` beside it, the managed blocks of `docs/development/testing/model-results.md`, `docs/development/token-footprint.md`, the site's per-domain tool reference under `site/src/content/docs/reference/tools/` (its `index.mdx` included) and its `es/` twin, the site's `reference/fine-grained-permissions.mdx` and its twin, the generated block of the site's `performance/resource-benchmark.mdx` with the charts under `site/public/benchmarks/`, the data files under `site/src/data/` (`stats.json`, `token-footprint.json`, `resource-benchmark.json`), the `llms*.txt` files at the repository root, and `lhm.plugin.json`'s capability arrays. Run the generator instead (`make update-all` runs every one of them except the benchmark measurement: it redraws the benchmark with `make bench-resources-render`, and only `make bench-resources` re-measures and rewrites `resource-benchmark.json`; ADR-0025 lists which command writes which path). The versions in `server.json` and the other manifests are stamped by the release, never by hand
- **UPD-011**: A user-facing change updates the English page and its Spanish twin in the same change. `pnpm run i18n:check` (in `site/`) only proves that every page has a twin, not that the two agree

### For New APIs

1. Add new entries to the appropriate reference document tables
2. Add new sections following the existing document pattern and style
3. Update the package documentation if the API belongs to an existing package
4. Add cross-references from related documents
5. Update the documentation index if new documents are created

### For Modified APIs

1. Update parameter tables with new/changed/removed parameters
2. Update return type documentation if changed
3. Update code examples to reflect the new signature
4. Add migration notes if the change is breaking
5. Update Mermaid diagrams if type relationships changed

### For Removed APIs

1. Mark the API as deprecated with a notice and removal version/date
2. Suggest the replacement API if one exists
3. Update cross-references that pointed to the removed API
4. After one release cycle, fully remove the deprecated section

### For Configuration Changes

1. Update the environment variables table
2. Update default values and descriptions
3. Add migration instructions if existing users need to change their config
4. Update deployment documentation if the change affects deployment

## Parity Verification

After all updates are applied, perform a parity check:

### For Each Updated Document

1. Read the updated documentation
2. Compare every documented API, parameter, and type against source code
3. Verify all code examples are syntactically valid
4. Confirm Mermaid diagrams reflect current architecture
5. Check all cross-reference links resolve correctly

### Verification Checklist

- [ ] All changed exported types/functions are documented correctly
- [ ] Parameter tables match current function signatures
- [ ] Return types and error handling match implementation
- [ ] Code examples compile and reflect current API
- [ ] Mermaid diagrams reflect current component relationships
- [ ] Cross-reference links are valid
- [ ] No TBD/TODO placeholders in updated sections
- [ ] Consistent terminology and style with surrounding content
- [ ] Deprecation notices added for removed APIs
- [ ] Every site page changed has its Spanish twin changed with it
- [ ] Markdown pipe tables in `README.md`, `docs/` and the site were verified with `go run ./cmd/format_md_tables/ --check`
- [ ] `make check-doc-links` and `make check-doc-tool-names` pass, and, when a site page changed, `cd site && pnpm run build && pnpm run lint`

## Output Format

After completing updates, provide a summary:

```markdown
## Documentation Update Summary

### Documents Updated
| Document | Sections Changed | Change Type |
|----------|-----------------|-------------|
| `site/src/content/docs/reference/tools/branch.mdx` | Regenerated with `make gen-tool-reference` for `branch.new_action` | New API |
| `site/src/content/docs/configuration.mdx` and `es/configuration.mdx` | New variable in the settings table | Modified API |

### Parity Status
- [x] All changes documented
- [x] Examples validated
- [x] Diagrams updated
- [x] Cross-references verified

### Notes
[Any observations, recommendations for follow-up, or areas needing manual review]
```

## Error Handling

- **ERR-001**: Source file not found — report the expected path and skip
- **ERR-002**: Documentation file not found — recommend running `generate-project-documentation` skill first
- **ERR-003**: Ambiguous change — document both interpretations and flag for manual review
- **ERR-004**: Breaking change detected — add prominent migration notice with before/after examples
- **ERR-005**: Diagram rendering failure — provide the Mermaid source and flag for manual validation
