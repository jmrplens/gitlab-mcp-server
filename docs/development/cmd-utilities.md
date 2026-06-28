# Command-Line Utilities Reference

The `cmd/` directory contains the developer tooling binaries that power audits, code generation, formatting, and the documentation pipeline for this project. They are **not** part of the runtime MCP server, with one exception: `cmd/server` is the server entry point itself.

Every utility can be run directly with `go run ./cmd/<name>/ [flags]`, or through the convenience Make targets documented per section. Several binaries also expose a `--check` (or `-check`) mode that validates generated output without writing it; these are wired into CI gates (see [CI gate targets](#ci-gate-targets)).

## Quick reference

| Utility                        | Category                  | Purpose                                                                | Make target                        |
| ------------------------------ | ------------------------- | ---------------------------------------------------------------------- | ---------------------------------- |
| `audit_1to1`                   | SDK/API parity audits     | Consolidated SDK↔API parity audit (struct/action/metadata gap streams) | `make audit-1to1`                  |
| `audit_action_spec_coverage`   | Catalog & metadata audits | Source-discovered ActionSpec catalog-first coverage inventory          | `make audit-action-spec-coverage`  |
| `audit_discovery_completeness` | Catalog & metadata audits | Extended META-001 model-discovery metadata quality auditor             | `make audit-discovery`             |
| `audit_doc_coverage`           | Catalog & metadata audits | Per-doc-file gaps vs the action catalog (DOC-002)                      | `make audit-doc-coverage`          |
| `audit_dynamic_aliases`        | Catalog & metadata audits | Dynamic-toolset alias governance (collisions, ambiguity)               | `make audit-dynamic-aliases`       |
| `audit_edition_tier`           | Catalog & metadata audits | Doc-grounded licensing tier (Free/Premium/Ultimate) vs binary gating   | `make audit-edition-tier`          |
| `audit_surface_quality`        | Surface quality audits    | Consolidated MCP tool surface quality audit (metadata + output)        | `make audit-surface-quality`       |
| `audit_tokens`                 | Surface quality audits    | LLM context-window overhead of every tool/resource/prompt definition   | `make audit-tokens`                |
| `audit_metrics`                | Surface quality audits    | Comprehensive metrics summary (tools, resources, prompts, codebase)    | `make audit-metrics`               |
| `godoc_tool`                   | Source quality audits     | Godoc compliance auditor and fixer (audit + fix subcommands)           | `make audit-godocs`                |
| `audit_test_names`             | Source quality audits     | Classifies `Test*` functions by naming pattern; emits rename hints     | `make audit-test-names`            |
| `find_dupes`                   | Source quality audits     | Finds duplicated string literals missing `const`/`var` declarations    | —                                  |
| `gen_action_catalog_manifest`  | Generators                | Generates the ActionSpec group-builder manifest                        | `make gen-action-catalog-manifest` |
| `gen_llms`                     | Generators                | Generates `llms.txt` and `llms-full.txt`                               | `make gen-llms`                    |
| `gen_readme`                   | Generators                | Regenerates the managed token-footprint sections in `README.md`        | `make gen-readme`                  |
| `gen_testing_docs`             | Generators                | Regenerates the test-metrics block in `docs/testing/testing.md`        | `make gen-testing-docs`            |
| `gen_docker_tools`             | Generators                | Generates a Docker MCP Registry-compatible `tools.json`                | —                                  |
| `format_md_tables`             | Formatters                | Normalizes Markdown pipe tables in `README.md` and `docs/`             | part of `make audit-docs`          |
| `eval_mcp_surfaces`            | Evaluation                | Evaluates model behavior across MCP tool surfaces                      | `make eval-surfaces-docker*`       |
| `server`                       | Server                    | The main `gitlab-mcp-server` MCP binary (runtime entry point)          | `make build`, `make run`           |

## SDK/API parity audits

### audit_1to1

Consolidated 1:1 SDK↔API parity audit. It combines three gap streams behind a single `-scope` flag: struct field mapping (R-INPUT/R-OUTPUT), action coverage (R-ACTION), and discovery metadata (R-META). When all three scopes run (the default), it produces a merged per-package backlog. It replaces the former `audit_struct_completeness`, `audit_action_coverage`, `audit_metadata_completeness`, and `gen_1to1_backlog` binaries.

#### Usage

```bash
# Run all three scopes and write a merged backlog
go run ./cmd/audit_1to1/

# Single-scope run, gaps only, to stdout
go run ./cmd/audit_1to1/ -scope=structs -gaps-only -output=-
```

#### Flags

| Flag         | Type     | Default                    | Description                                                                          |
| ------------ | -------- | -------------------------- | ------------------------------------------------------------------------------------ |
| `-gaps-only` | `bool`   | `false`                    | Only include entries with at least one finding                                       |
| `-output`    | `string` | `-`                        | Path to write the JSON report, or `-` for stdout                                     |
| `-scope`     | `string` | `structs,actions,metadata` | Comma-separated subset of `{structs,actions,metadata}`; default all (merged backlog) |

#### Output

JSON. A single-scope run produces that auditor's native shape. An all-scopes run produces a merged backlog containing `schema_version`, a `summary` block (9 counters), and a `packages[]` array.

#### Make targets

- `make audit-1to1` — writes `plan/1to1-backlog.json`.
- `make audit-struct-completeness` — legacy wrapper running `-scope=structs`.
- `make audit-action-coverage` — legacy wrapper running `-scope=actions`.
- `make audit-metadata-completeness` — legacy wrapper running `-scope=metadata`.

#### Notes

This single binary replaces four former binaries. The legacy Make targets remain as thin `-scope` wrappers for backward compatibility.

## Catalog & metadata audits

### audit_action_spec_coverage

Generates the source-discovered inventory of ActionSpec catalog-first coverage. It reports `RegisterTools`/`RegisterMeta`/`ActionSpecs` presence, surface classification, and dynamic-catalog counts, plus catalog-first invariant checks.

#### Usage

```bash
# Write the inventory to the default path
go run ./cmd/audit_action_spec_coverage/

# Print to stdout
go run ./cmd/audit_action_spec_coverage/ -output=-
```

#### Flags

| Flag      | Type     | Default                          | Description                                                    |
| --------- | -------- | -------------------------------- | -------------------------------------------------------------- |
| `-output` | `string` | `dist/action-spec-coverage.json` | Path to write the action spec coverage JSON, or `-` for stdout |

#### Output

A JSON report with invariant checks. The binary exits non-zero on catalog-first invariant violations.

#### Make targets

- `make audit-action-spec-coverage`

#### Notes

There is no `--check` mode; instead, the catalog-first invariants return a non-nil error on regression, which is what fails the build.

### audit_discovery_completeness

Extended META-001 auditor for model-discovery metadata quality. It checks action-level gaps (`weak_aliases`, `generic_usage`, `empty_related`, `weak_individual_description`, `missing_next_steps`), field-level gaps (`empty_output_description`, `param_enum_candidate`, `empty_param_description`), and sibling-cluster gaps (`missing_disambiguation`, `missing_parameter_guidance`). It applies cluster-aware severity escalation for non-CRUD action families.

#### Usage

```bash
# Full report to stdout
go run ./cmd/audit_discovery_completeness/

# CI gate: fail on any error-severity finding
go run ./cmd/audit_discovery_completeness/ -check -severity=error

# Tighten the alias minimum and write a backlog
go run ./cmd/audit_discovery_completeness/ -min-aliases=5 -output=plan/discovery-backlog.json
```

#### Flags

| Flag           | Type     | Default | Description                                                                  |
| -------------- | -------- | ------- | ---------------------------------------------------------------------------- |
| `-check`       | `bool`   | `false` | Exit non-zero if any finding meets or exceeds the `-severity` threshold      |
| `-gaps-only`   | `bool`   | `false` | Only include actions that raise at least one flag                            |
| `-min-aliases` | `int`    | `3`     | Minimum non-canonical, non-toolname aliases required to clear `weak_aliases` |
| `-output`      | `string` | `-`     | Path to write the JSON report, or `-` for stdout                             |
| `-severity`    | `string` | `error` | Threshold severity for `-check`: `error\|warning\|info`                      |

#### Output

A JSON report with `packages[]`, `clusters[]`, per-finding `severity`, `cluster`, and `fields[]`.

#### Make targets

- `make audit-discovery` — writes `plan/discovery-backlog.json`.
- `make audit-discovery-check` — CI gate.

#### Notes

This is a CI gate binary. The cluster-aware severity model is intentional layered design; the flat R-META baseline lives in `audit_1to1 -scope=metadata`.

### audit_doc_coverage

Reports per-doc-file gaps between `docs/tools/*.md` and the canonical action catalog (DOC-002): missing or orphan tools, tier-badge mismatches, and count drift.

#### Usage

```bash
# Write the backlog to the default path
go run ./cmd/audit_doc_coverage/

# CI gate
go run ./cmd/audit_doc_coverage/ -check
```

#### Flags

| Flag           | Type     | Default                        | Description                                                         |
| -------------- | -------- | ------------------------------ | ------------------------------------------------------------------- |
| `-check`       | `bool`   | `false`                        | Exit non-zero if any file has missing/orphan/tier_mismatch findings |
| `-docs-root`   | `string` | `docs/tools`                   | Directory of per-domain docs (relative to repo root)                |
| `-gaps-only`   | `bool`   | `false`                        | Only include files that have at least one finding                   |
| `-output`      | `string` | `plan/docs-tools-backlog.json` | Path to write the JSON report (relative to repo root)               |
| `-readme-path` | `string` | `docs/tools/README.md`         | Path to the Domains-table README (relative to repo root)            |

#### Output

A JSON backlog with per-file findings.

#### Make targets

- `make audit-doc-coverage`
- `make audit-doc-coverage-check` — CI gate.

### audit_dynamic_aliases

Audits the dynamic-toolset compatibility alias catalog for governance issues: canonical-route collisions and ambiguous aliases.

#### Usage

```bash
go run ./cmd/audit_dynamic_aliases/
```

#### Flags

This utility takes no flags.

#### Output

Tab-separated values to stdout (`Severity\tProblem\tAlias\tCanonical\tMessage`). The binary exits `1` if any error-severity finding is present.

#### Make targets

- `make audit-dynamic-aliases` — also runs as part of `make audit-docs`.

#### Notes

No flags and no arguments. The TSV schema is the machine-readable contract consumed by CI.

### audit_edition_tier

Reports the doc-grounded licensing tier (Free/Premium/Ultimate) of every action, parsed from GitLab API doc Tier badges, and compares it against the action's current binary CE/EE gating.

#### Usage

```bash
# Online: fetch latest docs from gitlab.com
go run ./cmd/audit_edition_tier/

# Offline: use only the cached docs
go run ./cmd/audit_edition_tier/ -offline
```

#### Flags

| Flag         | Type     | Default | Description                                      |
| ------------ | -------- | ------- | ------------------------------------------------ |
| `-gaps-only` | `bool`   | `false` | Only include domains that need tier work         |
| `-offline`   | `bool`   | `false` | Use only cached docs; do not fetch               |
| `-output`    | `string` | `-`     | Path to write the JSON report, or `-` for stdout |

#### Output

A JSON report with per-action tier classification and doc-vs-binary discrepancies.

#### Make targets

- `make audit-edition-tier`

#### Notes

Fetches remote docs from `gitlab.com` when not run with `-offline`. The working docs cache lives in `.tmp-tier-docs/`.

## Surface quality audits

### audit_surface_quality

Consolidated MCP tool surface quality audit. It combines metadata-quality checks (naming, annotations, schema shape, duplicates — formerly `audit_tools`) and output-quality checks (`OutputSchema`, Returns/See-also, Title — formerly `audit_output`) behind a single `-view` flag.

#### Usage

```bash
# Both views (default)
go run ./cmd/audit_surface_quality/

# Metadata view only
go run ./cmd/audit_surface_quality/ -view=metadata

# Output view only
go run ./cmd/audit_surface_quality/ -view=output
```

#### Flags

| Flag    | Type     | Default | Description                                             |
| ------- | -------- | ------- | ------------------------------------------------------- |
| `-view` | `string` | `all`   | Which audit view to run: `metadata`, `output`, or `all` |

#### Output

A Markdown report to stdout with summary tables, violations/findings grouped by category, and a full tool listing.

#### Make targets

- `make audit-surface-quality` — both views.
- `make audit-tools` — `-view=metadata`.
- `make audit-output` — `-view=output`.

#### Notes

The shared `listTools` applies `LockdownInputSchemas`, so the audit reflects exactly what clients see. Legacy wrappers (`audit-tools`, `audit-output`) exist for backward compatibility.

### audit_tokens

Measures the LLM context-window overhead of every registered tool/resource/prompt definition across the individual, meta, and dynamic surfaces using a bytes/4 heuristic. With `--compare-schemas`, it runs a sizing spike comparing `META_PARAM_SCHEMA` modes.

#### Usage

```bash
# Default token audit
go run ./cmd/audit_tokens/

# Compare META_PARAM_SCHEMA modes
go run ./cmd/audit_tokens/ -compare-schemas
```

#### Flags

| Flag               | Type   | Default | Description                                                                                                                |
| ------------------ | ------ | ------- | -------------------------------------------------------------------------------------------------------------------------- |
| `-compare-schemas` | `bool` | `false` | Compare `META_PARAM_SCHEMA` modes (opaque/full/compact) for meta-tool InputSchema sizing instead of the normal token audit |

#### Output

Default mode: a Markdown report to stdout with mode comparison, per-tool costs, and domain totals. `--compare-schemas` mode: a sizing table with opaque/full/compact byte costs per meta-tool.

#### Make targets

- `make audit-tokens`

#### Notes

The `--compare-schemas` mode replaces the former `audit_meta_schema` spike binary.

### audit_metrics

Comprehensive metrics summary: individual/meta/dynamic tool counts, catalog actions, resources/prompts, codebase file counts, and a per-domain breakdown.

#### Usage

```bash
go run ./cmd/audit_metrics/
```

#### Flags

This utility takes no flags.

#### Output

A Markdown report to stdout.

#### Make targets

- `make audit-metrics`

## Source quality audits

### godoc_tool

Consolidated Go documentation auditor and fixer. The `audit` subcommand reports missing or malformed doc comments; the `fix` subcommand generates and inserts godoc-compliant comments using naming-convention heuristics. It replaces the former `audit_godocs` and `add_docs` binaries. See [godoc.md](godoc.md) for the full rules taxonomy.

#### Usage

```bash
# Audit (default subcommand), Markdown report
go run ./cmd/godoc_tool/ audit

# CI gate
go run ./cmd/godoc_tool/ audit --fail-on-findings

# Fix a path (dry-run first)
go run ./cmd/godoc_tool/ fix --dry-run ./internal/tools/branches/
go run ./cmd/godoc_tool/ fix ./internal/tools/branches/
```

#### Subcommands

| Subcommand | Default | Description                                                       |
| ---------- | ------- | ----------------------------------------------------------------- |
| `audit`    | yes     | Report missing or malformed doc comments                          |
| `fix`      | no      | Generate and insert godoc-compliant comments into the given paths |

#### Flags (audit)

| Flag                 | Type     | Default    | Description                                           |
| -------------------- | -------- | ---------- | ----------------------------------------------------- |
| `--fail-on-findings` | `bool`   | `false`    | Exit non-zero when findings are present               |
| `--format`           | `string` | `markdown` | Report format: `markdown` or `json`                   |
| `--ignore-internal`  | `bool`   | `false`    | Skip packages whose import path contains `/internal/` |
| `--include-tests`    | `bool`   | `false`    | Also audit test files                                 |
| `--output`           | `string` | `""`       | Write the report to this path instead of stdout       |

#### Flags (fix)

| Flag         | Type       | Default | Description                                   |
| ------------ | ---------- | ------- | --------------------------------------------- |
| `--dry-run`  | `bool`     | `false` | Print what would change without writing files |
| `<paths...>` | positional | —       | File or directory paths to fix                |

#### Output

`audit` emits a Markdown or JSON report. `fix` modifies `.go` files in place (or prints a dry-run summary when `--dry-run` is set).

#### Make targets

- `make audit-godocs` — writes `dist/analysis/godoc.md`.
- `make audit-godocs-check` — CI gate.
- `make fix-godocs ARGS="<paths>"` — run the fixer.

#### Notes

Replaces the former `audit_godocs` and `add_docs` binaries. The full rules taxonomy (package comments, exported-symbol conventions, test-function comments, and the common package-comment pitfall) is documented in [godoc.md](godoc.md).

### audit_test_names

Scans Go `_test.go` files and classifies `Test*` functions by naming pattern (3-part / 2-part / no-underscore / TestCov / skip), then emits rename suggestions.

#### Usage

```bash
# Scan the standard source directories
go run ./cmd/audit_test_names/ cmd internal test
```

#### Flags

This utility takes no flags; pass positional directory arguments.

#### Positional arguments

| Argument   | Type       | Description         |
| ---------- | ---------- | ------------------- |
| `<dir>...` | positional | Directories to scan |

#### Output

CSV to stdout (`file,current_name,pattern,suggested_name`), with a summary printed to stderr.

#### Make targets

- `make audit-test-names` — runs with `cmd internal test`.

### find_dupes

Scans non-test Go source for string literals appearing three or more times that are not already `const`/`var` values.

#### Usage

```bash
go run ./cmd/find_dupes/ ./internal/tools/branches/
```

#### Flags

This utility takes no flags; pass positional path arguments.

#### Positional arguments

| Argument        | Type       | Description   |
| --------------- | ---------- | ------------- |
| `<dir|file>...` | positional | Paths to scan |

#### Output

Per-file sections to stdout using a `[Ndx] "value"` format that shows the occurrence count and index of each duplicate literal.

#### Make targets

None. Run directly with `go run`.

## Generators

### gen_action_catalog_manifest

Generates `internal/tools/action_specs_manifest_gen.go`, listing every `buildXxxActionSpecs` group-builder function discovered via an AST walk.

#### Usage

```bash
# Regenerate the manifest
go run ./cmd/gen_action_catalog_manifest/

# CI gate: verify it is current
go run ./cmd/gen_action_catalog_manifest/ -check
```

#### Flags

| Flag      | Type     | Default                                       | Description                                                 |
| --------- | -------- | --------------------------------------------- | ----------------------------------------------------------- |
| `-check`  | `bool`   | `false`                                       | Verify the generated manifest is up to date without writing |
| `-output` | `string` | `internal/tools/action_specs_manifest_gen.go` | Generated manifest path                                     |
| `-source` | `string` | `internal/tools`                              | Directory containing action spec group builder source files |

#### Output

Rewrites the manifest Go file in place, or (with `-check`) verifies it is current without writing.

#### Make targets

- `make gen-action-catalog-manifest`
- `make check-action-catalog-manifest` — CI gate.

### gen_llms

Generates `llms.txt` (the concise llmstxt.org index) and `llms-full.txt` (detailed per-tool schemas) by introspecting all surfaces, resources, and prompts via in-memory MCP.

#### Usage

```bash
# Regenerate both files
go run ./cmd/gen_llms/

# CI gate
go run ./cmd/gen_llms/ -check
```

#### Flags

| Flag     | Type   | Default | Description                                            |
| -------- | ------ | ------- | ------------------------------------------------------ |
| `-check` | `bool` | `false` | Validate the generated llms files without writing them |

#### Output

Rewrites `llms.txt` and `llms-full.txt` at the project root.

#### Make targets

- `make gen-llms`
- `make check-llms` — CI gate, also part of `make audit-docs`.

### gen_readme

Regenerates the managed `README.md` sections between the `<!-- START TOKEN FOOTPRINT -->` / `<!-- END TOKEN FOOTPRINT -->` markers with statistics: visible tools, reachable actions, and tool-schema + shared tokens across dynamic/meta/individual × full/minimal.

#### Usage

```bash
go run ./cmd/gen_readme/
```

#### Flags

This utility takes no flags.

#### Output

Rewrites the managed `README.md` sections in place.

#### Make targets

- `make gen-readme`

### gen_testing_docs

Regenerates the managed test-metrics block in `docs/testing/testing.md`: package discovery, AST test counts, naming-pattern stats, coverage tables, and low-coverage exceptions.

#### Usage

```bash
# Regenerate, including coverage runs
go run ./cmd/gen_testing_docs/

# CI gate
go run ./cmd/gen_testing_docs/ -check

# Fast path: skip the coverage runs
go run ./cmd/gen_testing_docs/ -skip-coverage
```

#### Flags

| Flag               | Type     | Default                   | Description                                                              |
| ------------------ | -------- | ------------------------- | ------------------------------------------------------------------------ |
| `-check`           | `bool`   | `false`                   | Fail if the generated section is not current                             |
| `-coverage-dir`    | `string` | `""`                      | Directory for temporary coverage profiles; defaults to a temp directory  |
| `-file`            | `string` | `docs/testing/testing.md` | Testing documentation file to update                                     |
| `-include-e2e-run` | `bool`   | `false`                   | Also run the build-tagged E2E suite; requires a GitLab test environment  |
| `-skip-coverage`   | `bool`   | `false`                   | Skip `go test` coverage execution and update count-only sections         |
| `-timeout`         | `string` | (from environment)        | `go test` timeout for coverage runs                                      |
| `-top-tool-rows`   | `int`    | `25`                      | Number of high-test-count tool sub-packages to show in the summary table |

#### Output

Rewrites the managed sections of `docs/testing/testing.md`.

#### Make targets

- `make gen-testing-docs`, also part of `make audit-docs` (with `-check`).

### gen_docker_tools

Generates a Docker MCP Registry-compatible `tools.json` (flattened name/description/arguments) by introspecting the chosen surface.

#### Usage

```bash
# Meta-tools (default)
go run ./cmd/gen_docker_tools/

# Include enterprise meta-tools
go run ./cmd/gen_docker_tools/ --enterprise

# Emit individual tools instead
go run ./cmd/gen_docker_tools/ --individual
```

#### Flags

| Flag           | Type   | Default | Description                                 |
| -------------- | ------ | ------- | ------------------------------------------- |
| `--enterprise` | `bool` | `false` | Include enterprise meta-tools               |
| `--individual` | `bool` | `false` | Emit individual tools instead of meta-tools |

#### Output

A JSON array to stdout.

#### Make targets

None. Run directly with `go run`.

## Formatters

### format_md_tables

Normalizes Markdown pipe tables in `README.md` and `docs/` (or explicit positional paths).

#### Usage

```bash
# Format the default set (README.md and docs/)
go run ./cmd/format_md_tables/

# CI gate
go run ./cmd/format_md_tables/ -check

# Format explicit paths
go run ./cmd/format_md_tables/ README.md docs/tools/issues.md
```

#### Flags

| Flag     | Type     | Default | Description                                        |
| -------- | -------- | ------- | -------------------------------------------------- |
| `-check` | `bool`   | `false` | Fail if any Markdown table needs formatting        |
| `-root`  | `string` | `.`     | Repository root containing `README.md` and `docs/` |

#### Positional arguments

| Argument                | Type       | Description                                     |
| ----------------------- | ---------- | ----------------------------------------------- |
| (optional) `<paths...>` | positional | Explicit paths; defaults to `{README.md, docs}` |

#### Output

Rewrites files in place unless `-check` is set, in which case it only verifies formatting.

#### Make targets

- Part of `make audit-docs` (with `-check`).
- `make analyze-fix` — applies fixes.

## Evaluation

### eval_mcp_surfaces

Evaluates model behavior across MCP tool surfaces by running typed evaluation cases against the server in mock or live (Docker/self-hosted) mode. See [`cmd/eval_mcp_surfaces/README.md`](../../cmd/eval_mcp_surfaces/README.md) for the full guide, case formats, and run modes.

**Make targets:** the `make eval-surfaces-docker*` family (`eval-surfaces-docker`, `eval-surfaces-docker-enterprise`, `eval-surfaces-docker-enterprise-ce`, `eval-surfaces-docker-enterprise-all`, `eval-surfaces-docker-enterprise-all-fixtures`).

## Server

### server

The main `gitlab-mcp-server` MCP binary — the runtime entry point and the only `cmd/` binary that ships to users. See [cli-reference.md](../cli-reference.md) for the full CLI reference and [configuration.md](../configuration.md) for environment and configuration details.

**Make targets:** `make build` (builds `./dist/gitlab-mcp-server`), `make run` (builds and runs locally).

## CI gate targets

The following utilities expose a verification mode (`--check` or `-check`, or an invariant/error exit) that CI runs to guard against drift. The combined documentation gate is `make audit-docs`, which chains the markdown/llms/testing/formatter/godoc/surface/alias checks plus link and site checks.

| Make target                              | Utility                        | What it gates                                                      | Exit behavior                                             |
| ---------------------------------------- | ------------------------------ | ------------------------------------------------------------------ | --------------------------------------------------------- |
| `check-action-catalog-manifest`          | `gen_action_catalog_manifest`  | Generated ActionSpec manifest is current                           | Non-zero if the manifest is stale                         |
| `check-llms`                             | `gen_llms`                     | `llms.txt` and `llms-full.txt` are current and structurally valid  | Non-zero if either file is stale or malformed             |
| `audit-discovery-check`                  | `audit_discovery_completeness` | No META-001 finding meets the configured severity threshold        | Non-zero if any finding meets `-severity` (default error) |
| `audit-doc-coverage-check`               | `audit_doc_coverage`           | No `docs/tools/*.md` has missing/orphan/tier_mismatch findings     | Non-zero if any file has a finding                        |
| `audit-godocs-check`                     | `godoc_tool audit`             | No package, symbol, or test Godoc findings remain                  | Non-zero when findings are present                        |
| `audit-dynamic-aliases`                  | `audit_dynamic_aliases`        | No error-severity alias governance finding (collisions, ambiguity) | Non-zero (`1`) if any error-severity finding exists       |
| `audit-docs` → `format_md_tables -check` | `format_md_tables`             | All Markdown pipe tables are normalized                            | Non-zero if any table needs formatting                    |
| `audit-docs` → `gen_testing_docs -check` | `gen_testing_docs`             | The `docs/testing/testing.md` test-metrics block is current        | Non-zero if the generated section is stale                |
