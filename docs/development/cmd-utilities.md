# Command-Line Utilities Reference

> **Diátaxis type**: Reference · **Audience**: 🛠️ Contributors & maintainers

The `cmd/` directory contains the developer tooling binaries that power audits, code generation, formatting, and the documentation pipeline for this project. They are **not** part of the runtime MCP server, with one exception: `cmd/server` is the server entry point itself.

Every utility can be run directly with `go run ./cmd/<name>/ [flags]`, or through the convenience Make targets documented per section. Several binaries also expose a `--check` (or `-check`) mode that validates generated output without writing it; these are wired into CI gates (see [CI gate targets](#ci-gate-targets)).

## Quick reference

| Utility                        | Category                  | Purpose                                                                                                                           | Make target                               |
| ------------------------------ | ------------------------- | --------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------- |
| `audit_1to1`                   | SDK/API parity audits     | Consolidated SDK↔API parity audit (struct/action/metadata gap streams)                                                            | `make audit-1to1`                         |
| `audit_catalog_first`          | Catalog & metadata audits | Source-discovered ActionSpec catalog-first coverage inventory                                                                     | `make audit-catalog-first`                |
| `audit_discovery_completeness` | Catalog & metadata audits | Extended META-001 model-discovery metadata quality auditor                                                                        | `make audit-discovery`                    |
| `audit_doc_coverage`           | Catalog & metadata audits | Per-doc-file gaps vs the action catalog (DOC-002)                                                                                 | `make audit-doc-coverage`                 |
| `audit_dynamic_aliases`        | Catalog & metadata audits | Dynamic-toolset alias governance (collisions, ambiguity)                                                                          | `make audit-dynamic-aliases`              |
| `audit_edition_tier`           | Catalog & metadata audits | Doc-grounded licensing tier (Free/Premium/Ultimate) vs binary gating                                                              | `make audit-edition-tier`                 |
| `audit_surface_quality`        | Surface quality audits    | Consolidated MCP tool surface quality audit (metadata + output)                                                                   | `make audit-surface-quality`              |
| `audit_tokens`                 | Surface quality audits    | LLM context-window overhead of every tool/resource/prompt definition; `-footprint` regenerates the README token-footprint section | `make audit-tokens`, `make gen-footprint` |
| `audit_metrics`                | Surface quality audits    | Comprehensive metrics summary (tools, resources, prompts, codebase)                                                               | `make audit-metrics`                      |
| `godoc_tool`                   | Source quality audits     | Godoc compliance auditor and fixer (audit + fix subcommands)                                                                      | `make audit-godocs`                       |
| `audit_test_names`             | Source quality audits     | Classifies `Test*` functions by naming pattern; emits rename hints                                                                | `make audit-test-names`                   |
| `audit_string_dupes`           | Source quality audits     | Finds duplicated string literals missing `const`/`var` declarations                                                               | —                                         |
| `gen_action_catalog_manifest`  | Generators                | Generates the ActionSpec group-builder manifest                                                                                   | `make gen-action-catalog-manifest`        |
| `gen_lhm_manifest`             | Generators                | Generates the tools/prompts/resources arrays in `lhm.plugin.json` (LobeHub Marketplace)                                           | `make gen-lhm-manifest`                   |
| `gen_llms`                     | Generators                | Generates `llms.txt` and `llms-full.txt`                                                                                          | `make gen-llms`                           |
| `gen_stats`                    | Generators                | Regenerates the managed repository statistics section in `README.md`                                                              | `make gen-stats`                          |
| `gen_testing_docs`             | Generators                | Regenerates the test-metrics block in `docs/development/testing/testing.md`                                                       | `make gen-testing-docs`                   |
| `gen_docker_tools`             | Generators                | Generates a Docker MCP Registry-compatible `tools.json`                                                                           | —                                         |
| `format_md_tables`             | Formatters                | Normalizes Markdown pipe tables in `README.md` and `docs/`                                                                        | part of `make audit-docs`                 |
| `eval_mcp_surfaces`            | Evaluation                | Evaluates model behavior across MCP tool surfaces                                                                                 | `make eval-surfaces-docker*`              |
| `server`                       | Server                    | The main `gitlab-mcp-server` MCP binary (runtime entry point)                                                                     | `make build`, `make run`                  |

## SDK/API parity audits

### audit_1to1

Consolidated 1:1 SDK↔API parity audit. It combines three gap streams behind a single `-scope` flag: struct field mapping (R-INPUT/R-OUTPUT), action coverage (R-ACTION), and discovery metadata (R-META). When all three scopes run (the default), it produces a merged per-package backlog. It replaces the former `audit_struct_completeness`, `audit_action_coverage`, `audit_metadata_completeness`, and `gen_1to1_backlog` binaries.

#### Usage

```bash
# Run all three scopes and write a merged backlog
go run ./cmd/audit_1to1/

# Single-scope run, gaps only, to stdout
go run ./cmd/audit_1to1/ -scope=structs -gaps-only -output=-

# Validate that every doc/api citation behind the adjudication tables is still
# fetchable (the official API doc is the 1:1 ground truth)
go run ./cmd/audit_1to1/ -validate-docs
```

#### Flags

| Flag             | Type       | Default                    | Description                                                                                                                                        |
| ---------------- | ---------- | -------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- |
| `-gaps-only`     | `bool`     | `false`                    | Only include entries with at least one finding                                                                                                     |
| `-output`        | `string`   | `-`                        | Path to write the JSON report, or `-` for stdout                                                                                                   |
| `-scope`         | `string`   | `structs,actions,metadata` | A single `{structs,actions,metadata}` scope, or all three (default, merged backlog); two-scope combinations are rejected                           |
| `-validate-docs` | `bool`     | `false`                    | Instead of the audit, verify every `doc/api/<area>.md` citation in the adjudication tables is still fetchable (exits non-zero on a stale citation) |
| `-refresh`       | `bool`     | `false`                    | With `-validate-docs`, force re-fetch of cited docs even when cached and fresh                                                                     |
| `-offline`       | `bool`     | `false`                    | With `-validate-docs`, use only cached docs; do not fetch                                                                                          |
| `-max-age`       | `duration` | `168h`                     | With `-validate-docs`, re-download cached docs older than this (default 7 days)                                                                    |

#### Output

JSON. A single-scope run produces that auditor's native shape. An all-scopes run produces a merged backlog containing `schema_version`, a `summary` block (9 counters), and a `packages[]` array. `-validate-docs` emits `{schema_version, checked, ok, stale[]}`.

#### Make targets

- `make audit-1to1` — writes `plan/1to1-backlog.json`.
- `make audit-1to1-validate-docs` — validates the doc/api citations (CI gate).
- `make audit-struct-completeness` — legacy wrapper running `-scope=structs`.
- `make audit-action-coverage` — legacy wrapper running `-scope=actions`.
- `make audit-metadata-completeness` — legacy wrapper running `-scope=metadata`.

#### Notes

This single binary replaces four former binaries. The legacy Make targets remain as thin `-scope` wrappers for backward compatibility. `-validate-docs` uses the shared `cmd/internal/apidocs` fetcher (cache in `.cache/gitlab-api-docs/`, 7-day TTL) — the same source-of-truth docs as `audit_edition_tier`.

## Catalog & metadata audits

### audit_catalog_first

Generates the source-discovered inventory of ActionSpec catalog-first coverage. It reports `RegisterTools`/`RegisterMeta`/`ActionSpecs` presence, surface classification, and dynamic-catalog counts, plus catalog-first invariant checks.

#### Usage

```bash
# Write the inventory to the default path
go run ./cmd/audit_catalog_first/

# Print to stdout
go run ./cmd/audit_catalog_first/ -output=-
```

#### Flags

| Flag      | Type     | Default                          | Description                                                    |
| --------- | -------- | -------------------------------- | -------------------------------------------------------------- |
| `-output` | `string` | `dist/action-spec-coverage.json` | Path to write the action spec coverage JSON, or `-` for stdout |

#### Output

A JSON report with invariant checks. The binary exits non-zero on catalog-first invariant violations.

#### Make targets

- `make audit-catalog-first`

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

Reports per-doc-file gaps between `docs/reference/tools/*.md` and the canonical action catalog (DOC-002): missing or orphan tools, tier-badge mismatches, and count drift.

#### Usage

```bash
# Write the backlog to the default path
go run ./cmd/audit_doc_coverage/

# CI gate
go run ./cmd/audit_doc_coverage/ -check
```

#### Flags

| Flag           | Type     | Default                          | Description                                                         |
| -------------- | -------- | -------------------------------- | ------------------------------------------------------------------- |
| `-check`       | `bool`   | `false`                          | Exit non-zero if any file has missing/orphan/tier_mismatch findings |
| `-docs-root`   | `string` | `docs/tools`                     | Directory of per-domain docs (relative to repo root)                |
| `-gaps-only`   | `bool`   | `false`                          | Only include files that have at least one finding                   |
| `-output`      | `string` | `plan/docs-tools-backlog.json`   | Path to write the JSON report (relative to repo root)               |
| `-readme-path` | `string` | `docs/reference/tools/README.md` | Path to the Domains-table README (relative to repo root)            |

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

| Flag      | Type     | Default | Description                    |
| --------- | -------- | ------- | ------------------------------ |
| `-output` | `string` | `tsv`   | Output format: `tsv` or `json` |

#### Output

With `-output tsv` (default): tab-separated values to stdout (`Severity\tProblem\tAlias\tCanonical\tSource\tMessage`) plus a pass/fail summary line. With `-output json`: the findings as a JSON array. Any other value is rejected with exit `2`. The binary exits `1` if any error-severity finding is present.

#### Make targets

- `make audit-dynamic-aliases` — also runs as part of `make audit-docs`.

#### Notes

The TSV schema is the machine-readable contract consumed by CI; `-output json` is available for programmatic consumers.

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

| Flag         | Type       | Default | Description                                              |
| ------------ | ---------- | ------- | -------------------------------------------------------- |
| `-gaps-only` | `bool`     | `false` | Only include domains that need tier work                 |
| `-offline`   | `bool`     | `false` | Use only cached docs; do not fetch                       |
| `-refresh`   | `bool`     | `false` | Force re-fetch docs even when cached and fresh           |
| `-max-age`   | `duration` | `168h`  | Re-download cached docs older than this (default 7 days) |
| `-output`    | `string`   | `-`     | Path to write the JSON report, or `-` for stdout         |

#### Output

A JSON report with per-action tier classification and doc-vs-binary discrepancies.

#### Make targets

- `make audit-edition-tier`

#### Notes

Fetches the GitLab API reference docs from `gitlab.com` via the shared `cmd/internal/apidocs` fetcher. Docs are cached in `.cache/gitlab-api-docs/` (gitignored, shared with `audit_1to1 -validate-docs`) and reused while younger than the 7-day TTL; `-refresh` forces a re-download and `-offline` uses only the cache. The fetcher honors `Retry-After`, backs off with jitter, and spaces requests so a full sweep does not trip the raw rate limiter.

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

| Flag    | Type     | Default | Description                                                                                                                                      |
| ------- | -------- | ------- | ------------------------------------------------------------------------------------------------------------------------------------------------ |
| `-view` | `string` | `all`   | Which audit view to run: `metadata`, `output`, or `all`                                                                                          |
| `-json` | `bool`   | `false` | Emit JSON instead of Markdown; requires `-view=metadata` or `-view=output` (rejected with `-view=all`, which would emit two top-level documents) |

#### Output

A Markdown report to stdout with summary tables, violations/findings grouped by category, and a full tool listing. With `-json` (single view), a JSON document for that view.

#### Make targets

- `make audit-surface-quality` — both views.
- `make audit-tools` — `-view=metadata`.
- `make audit-output` — `-view=output`.

#### Notes

The shared `listTools` applies `LockdownInputSchemas`, so the audit reflects exactly what clients see. Legacy wrappers (`audit-tools`, `audit-output`) exist for backward compatibility.

### audit_tokens

Measures the LLM context-window overhead of every registered tool/resource/prompt definition across the individual, meta, and dynamic surfaces using the cl100k_base tokenizer (via `github.com/tiktoken-go/tokenizer`, with a bytes/4 fallback). With `--compare-schemas`, it runs a sizing spike comparing `META_PARAM_SCHEMA` modes. With `-footprint`, it measures every tier × surface × schema-mode combination and regenerates the README token-footprint section plus the standalone reference doc; add `-check` to verify those are current without writing (CI gate).

#### Usage

```bash
# Default token audit
go run ./cmd/audit_tokens/

# Compare META_PARAM_SCHEMA modes
go run ./cmd/audit_tokens/ -compare-schemas

# Regenerate the README token-footprint section + docs/development/token-footprint.md
go run ./cmd/audit_tokens/ -footprint

# Verify the token-footprint section + doc are current without writing (CI gate)
go run ./cmd/audit_tokens/ -footprint -check
```

#### Flags

| Flag               | Type   | Default | Description                                                                                                                                                  |
| ------------------ | ------ | ------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `-footprint`       | `bool` | `false` | Measure all tiers × surfaces × `META_PARAM_SCHEMA` modes and write the README token-footprint section + `docs/development/token-footprint.md`                |
| `-check`           | `bool` | `false` | With `-footprint`, verify the README token-footprint section and `docs/development/token-footprint.md` are current without writing (exits non-zero on drift) |
| `-compare-schemas` | `bool` | `false` | Compare `META_PARAM_SCHEMA` modes (opaque/full/compact) for meta-tool InputSchema sizing instead of the normal token audit                                   |
| `-json`            | `bool` | `false` | Emit a JSON summary instead of the Markdown report                                                                                                           |
| `-top-tools`       | `int`  | `30`    | Number of individual tools to list by token cost                                                                                                             |
| `-top-domains`     | `int`  | `20`    | Number of domains to list by token cost                                                                                                                      |

#### Output

Default mode: a Markdown report to stdout with mode comparison, per-tool costs, and domain totals. `-compare-schemas` mode: a sizing table with opaque/full/compact byte costs per meta-tool. `-footprint` mode: rewrites the `<!-- START TOKEN FOOTPRINT -->` section of `README.md` and writes `docs/development/token-footprint.md`.

#### Make targets

- `make audit-tokens`
- `make gen-footprint` — runs `-footprint` mode.
- `make check-footprint` — runs `-footprint -check` (CI gate; non-zero on drift).
- `make gen-readme` — umbrella that also regenerates the stats section.

#### Notes

The `--compare-schemas` mode replaces the former `audit_meta_schema` spike binary. The `-footprint` mode replaces the token-footprint half of the former `gen_readme` binary; the statistics half moved to `gen_stats`.

### audit_metrics

Comprehensive metrics summary: individual/meta/dynamic tool counts, catalog actions, resources/prompts, codebase file counts, and a per-domain breakdown.

#### Usage

```bash
go run ./cmd/audit_metrics/
```

#### Flags

| Flag           | Type   | Default | Description                                            |
| -------------- | ------ | ------- | ------------------------------------------------------ |
| `-json`        | `bool` | `false` | Emit a JSON summary instead of the Markdown report     |
| `-top-domains` | `int`  | `20`    | Number of domains to list by tool count (must be >= 0) |

#### Output

A Markdown report to stdout by default, or a JSON summary with `-json`.

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

### audit_test_goroutines

Scans `_test.go` files for `testing.T` aborts (`t.Fatal`, `t.Fatalf`, `t.FailNow`) made inside function literals that cross a goroutine boundary (`http.HandlerFunc`, `HandleFunc`, `go` statements, `errgroup.Go`, MCP `AddTool` handlers, middleware, handler struct fields). Classifies each site as category A (tail position) or B (handler still had work to do), and also reports advisory `t.Errorf` sites not followed by `return`. Enforces the contract in `.github/instructions/test-goroutines.instructions.md`.

#### Usage

```bash
# Human-readable report over the default roots (cmd, internal, test)
go run ./cmd/audit_test_goroutines/

# JSON work list + CI gate
go run ./cmd/audit_test_goroutines/ -json plan/test-goroutines-backlog.json
go run ./cmd/audit_test_goroutines/ --check
```

#### Flags

| Flag      | Type   | Default | Description                                                                    |
| --------- | ------ | ------- | ------------------------------------------------------------------------------ |
| `-json`   | string | _(off)_ | Write the JSON work list to this path                                          |
| `--check` | bool   | `false` | Exit non-zero when any abort site exists; errorf-without-return stays advisory |

#### Output

Human report to stdout (per-site `file:line [category] boundary`), summary line, and optionally the JSON work list (`fatal`, `errorf_no_return`, `summary`).

#### Make targets

- `make audit-test-goroutines` — writes `plan/test-goroutines-backlog.json`.
- `make check-test-goroutines` — CI gate; also step [6/7] of `make analyze`.

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

### audit_test_subtests

Finds range loops over case tables (slice or map literals, inline or bound to a local) inside `Test*` functions whose body asserts (`t.Error*`, `t.Fatal*`, or a helper that receives `t`) without opening a `t.Run` subtest, which is the shape the table-driven rule in `go.instructions.md` forbids. A `// sequential: <reason>` comment on the line above a loop declares a sequence of dependent steps and is reported separately instead of failing, and so is a loop inside a `synctest.Test` bubble, where the testing package panics on `t.Run`.

```bash
go run ./cmd/audit_test_subtests/
go run ./cmd/audit_test_subtests/ -json plan/test-subtests-backlog.json
go run ./cmd/audit_test_subtests/ -fix
go run ./cmd/audit_test_subtests/ -check ./internal/toolutil
```

#### Flags

| Flag     | Type   | Default | Description                                                      |
| -------- | ------ | ------- | ---------------------------------------------------------------- |
| `-json`  | string | _(off)_ | Write the JSON work list to this path                            |
| `-check` | bool   | `false` | Exit non-zero when any case loop still asserts without a subtest |
| `-fix`   | bool   | `false` | Rewrite the unambiguous sites in place, then report what remains |

`-fix` wraps the loop body in `t.Run(name, func(t *testing.T) { ... })` when the name is unambiguous: a `[]string` table names each case after its element, a struct table after a string field called `name`, `desc`, `description`, `label`, `title` or `id`, and a `map[string]...` table after its key. A bare `continue` becomes `return`. Loops that `break` or `goto`, and tables with no such field, stay in the report for a hand rewrite (add a `name` field). The mechanical pass of 2026-09-01 rewrote 827 of 911 sites this way.

#### Output

Per-file tallies (`sites`, `fixable`), a summary line, and optionally the JSON work list (`findings` with the rewrite each site qualifies for, `sequential`, `summary`).

#### Make targets

- `make audit-test-subtests` — writes `plan/test-subtests-backlog.json`.
- `make check-test-subtests` — CI gate; also step [7/7] of `make analyze`.

### audit_string_dupes

Scans non-test Go source for string literals appearing often enough (default: three or more times) and long enough (default: three or more characters) that are not already `const`/`var` values.

#### Usage

```bash
go run ./cmd/audit_string_dupes/ ./internal/tools/branches/

# Custom thresholds
go run ./cmd/audit_string_dupes/ -threshold 4 -min-length 5 ./internal/
```

#### Flags

| Flag          | Type  | Default | Description                                                   |
| ------------- | ----- | ------- | ------------------------------------------------------------- |
| `-threshold`  | `int` | `3`     | Minimum occurrence count to report a duplicate (must be >= 1) |
| `-min-length` | `int` | `3`     | Minimum string length to consider (must be >= 1)              |

Pass one or more positional path arguments after the flags.

#### Positional arguments

| Argument             | Type       | Description   |
| -------------------- | ---------- | ------------- |
| `<dir&#124;file>...` | positional | Paths to scan |

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

### gen_lhm_manifest

Regenerates the `tools`, `prompts`, and `resources` arrays in `lhm.plugin.json`, the manifest published to the LobeHub Marketplace. LobeHub derives the listing's capability badges from those arrays — its scanner cannot introspect a server distributed as a Go binary or a Docker image — so a manifest without them advertises zero tools no matter what the server registers.

The declared tool surface is the default one, `dynamic`, pinned explicitly rather than read from `TOOL_SURFACE`; the round-trip runs against an in-process stub client rather than `GITLAB_URL`/`GITLAB_TOKEN`. Both keep the committed file independent of the machine that generated it. Output schemas and tool icons are dropped: neither is part of the shape LobeHub documents, and the base64 icon data URIs alone would triple the file. Every other field is preserved, `version` included — the release stamp owns that one.

#### Usage

```bash
# Rewrite the capability arrays
go run ./cmd/gen_lhm_manifest/

# CI gate
go run ./cmd/gen_lhm_manifest/ --check
```

#### Flags

| Flag      | Type   | Default | Description                                                                   |
| --------- | ------ | ------- | ----------------------------------------------------------------------------- |
| `--check` | `bool` | `false` | Verify the committed manifest matches the registered surface, without writing |

#### Output

Rewrites `lhm.plugin.json` at the project root.

#### Make targets

- `make gen-lhm-manifest`
- `make check-lhm-manifest` — CI gate; also runs before `make publish-lobehub`.

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

### gen_stats

Regenerates the managed `README.md` repository statistics section between the `<!-- START STATS -->` / `<!-- END STATS -->` markers: file/function/line counts, code-pattern tallies, dependency and git history metrics, and "hall of fame" records (longest names, largest files).

#### Usage

```bash
go run ./cmd/gen_stats/
```

#### Flags

- `--check` — verify the stats section is current without writing; exits non-zero if stale.

#### Output

Rewrites the managed stats section of `README.md` in place.

#### Make targets

- `make gen-stats`
- `make gen-readme` — convenience umbrella that also runs the token-footprint generator.

> **Token footprint moved.** The README `<!-- START TOKEN FOOTPRINT -->` section and `docs/development/token-footprint.md` are now regenerated by the `-footprint` flag of `audit_tokens` (formerly the token-footprint half of `gen_readme`):
>
> ```bash
> go run ./cmd/audit_tokens/ -footprint
> ```

### gen_testing_docs

Regenerates the managed test-metrics block in `docs/development/testing/testing.md`: package discovery, AST test counts, naming-pattern stats, coverage tables, and low-coverage exceptions.

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

| Flag               | Type     | Default                               | Description                                                              |
| ------------------ | -------- | ------------------------------------- | ------------------------------------------------------------------------ |
| `-check`           | `bool`   | `false`                               | Fail if the generated section is not current                             |
| `-coverage-dir`    | `string` | `""`                                  | Directory for temporary coverage profiles; defaults to a temp directory  |
| `-file`            | `string` | `docs/development/testing/testing.md` | Testing documentation file to update                                     |
| `-include-e2e-run` | `bool`   | `false`                               | Also run the build-tagged E2E suite; requires a GitLab test environment  |
| `-skip-coverage`   | `bool`   | `false`                               | Skip `go test` coverage execution and update count-only sections         |
| `-timeout`         | `string` | (from environment)                    | `go test` timeout for coverage runs                                      |
| `-top-tool-rows`   | `int`    | `25`                                  | Number of high-test-count tool sub-packages to show in the summary table |

#### Output

Rewrites the managed sections of `docs/development/testing/testing.md`.

#### Make targets

- `make gen-testing-docs`, also part of `make audit-docs` (with `-check`).

### gen_docker_tools

Generates a Docker MCP Registry-compatible `tools.json` (flattened name/description/arguments) by introspecting the chosen surface.

#### Usage

```bash
# Meta-tools (the generator's default output; the server's default surface is dynamic, which this generator does not emit)
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
go run ./cmd/format_md_tables/ README.md docs/reference/tools/issues.md
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

The main `gitlab-mcp-server` MCP binary — the runtime entry point and the only `cmd/` binary that ships to users. See [CLI Reference](../reference/cli.md) for the full CLI reference and [configuration.md](../reference/configuration.md) for environment and configuration details.

**Make targets:** `make build` (builds `./dist/gitlab-mcp-server`), `make run` (builds and runs locally).

## CI gate targets

The following utilities expose a verification mode (`--check` or `-check`, or an invariant/error exit) that CI runs to guard against drift. The combined documentation gate is `make audit-docs`, which chains the markdown/llms/testing/formatter/godoc/surface/alias checks plus link and site checks.

| Make target                              | Utility                        | What it gates                                                                        | Exit behavior                                             |
| ---------------------------------------- | ------------------------------ | ------------------------------------------------------------------------------------ | --------------------------------------------------------- |
| `check-action-catalog-manifest`          | `gen_action_catalog_manifest`  | Generated ActionSpec manifest is current                                             | Non-zero if the manifest is stale                         |
| `check-llms`                             | `gen_llms`                     | `llms.txt` and `llms-full.txt` are current and structurally valid                    | Non-zero if either file is stale or malformed             |
| `check-lhm-manifest`                     | `gen_lhm_manifest`             | `lhm.plugin.json` declares the registered tools, prompts, and resources              | Non-zero if the manifest is stale                         |
| `check-footprint`                        | `audit_tokens -footprint`      | README token-footprint section and `docs/development/token-footprint.md` are current | Non-zero if either is stale                               |
| `check-stats`                            | `gen_stats`                    | README repository-statistics section is current                                      | Non-zero if the section is stale                          |
| `audit-discovery-check`                  | `audit_discovery_completeness` | No META-001 finding meets the configured severity threshold                          | Non-zero if any finding meets `-severity` (default error) |
| `audit-doc-coverage-check`               | `audit_doc_coverage`           | No `docs/reference/tools/*.md` has missing/orphan/tier_mismatch findings             | Non-zero if any file has a finding                        |
| `audit-godocs-check`                     | `godoc_tool audit`             | No package, symbol, or test Godoc findings remain                                    | Non-zero when findings are present                        |
| `audit-dynamic-aliases`                  | `audit_dynamic_aliases`        | No error-severity alias governance finding (collisions, ambiguity)                   | Non-zero (`1`) if any error-severity finding exists       |
| `audit-docs` → `format_md_tables -check` | `format_md_tables`             | All Markdown pipe tables are normalized                                              | Non-zero if any table needs formatting                    |
| `audit-docs` → `gen_testing_docs -check` | `gen_testing_docs`             | The `docs/development/testing/testing.md` test-metrics block is current              | Non-zero if the generated section is stale                |
