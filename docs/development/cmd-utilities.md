# Command-Line Utilities Reference

> **Diátaxis type**: Reference · **Audience**: 🛠️ Contributors & maintainers

The `cmd/` directory contains the developer tooling binaries that power audits, code generation, formatting, and the documentation pipeline for this project. They are **not** part of the runtime MCP server, with one exception: `cmd/server` is the server entry point itself.

Every utility can be run directly with `go run ./cmd/<name>/ [flags]`, or through the convenience Make targets documented per section. Several binaries also expose a `--check` (or `-check`) mode that validates generated output without writing it; these are wired into CI gates (see [CI gate targets](#ci-gate-targets)).

## Quick reference

| Utility                        | Category                      | Purpose                                                                                                                                                                                             | Make target                                           |
| ------------------------------ | ----------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------- |
| `audit_1to1`                   | SDK/API parity audits         | Consolidated SDK↔API parity audit (struct/action/metadata gap streams, plus the `sdk` service and raw-GraphQL gate)                                                                                 | `make audit-1to1`                                     |
| `audit_catalog_first`          | Catalog & metadata audits     | Source-discovered ActionSpec catalog-first coverage inventory                                                                                                                                       | `make audit-catalog-first`                            |
| `audit_discovery_completeness` | Catalog & metadata audits     | Extended META-001 model-discovery metadata quality auditor                                                                                                                                          | `make audit-discovery`                                |
| `audit_doc_coverage`           | Catalog & metadata audits     | Per-doc-file gaps vs the action catalog (DOC-002)                                                                                                                                                   | `make audit-doc-coverage`                             |
| `audit_doc_tool_names`         | Catalog & metadata audits     | Every `gitlab_*` tool name the documentation mentions is one some surface registers                                                                                                                 | `make check-doc-tool-names`                           |
| `audit_dynamic_aliases`        | Catalog & metadata audits     | Dynamic-toolset alias governance (collisions, ambiguity)                                                                                                                                            | `make audit-dynamic-aliases`                          |
| `audit_e2e_gaps`               | Catalog & metadata audits     | Catalog actions the e2e suite never exercises                                                                                                                                                       | `make audit-e2e-gaps`                                 |
| `audit_edition_tier`           | Catalog & metadata audits     | Doc-grounded licensing tier (Free/Premium/Ultimate) vs binary gating                                                                                                                                | `make audit-edition-tier`                             |
| `audit_graphql_documents`      | Catalog & metadata audits     | Every raw GraphQL document in the source is one the pinned GitLab schema accepts                                                                                                                    | `make check-graphql-documents`                         |
| `audit_readonly_graphql`       | Catalog & metadata audits     | No action classified ReadOnly can reach a GraphQL mutation                                                                                                                                          | `make check-readonly-graphql`                         |
| `audit_surface_quality`        | Surface quality audits        | Consolidated MCP tool surface quality audit (metadata + output)                                                                                                                                     | `make audit-surface-quality`                          |
| `audit_gateway_chars`          | Surface quality audits        | Served descriptions and titles carry no character an MCP gateway validator rejects                                                                                                                  | `make check-gateway-chars`                            |
| `audit_tokens`                 | Surface quality audits        | LLM context-window overhead of every tool/resource/prompt definition; `-footprint` regenerates the README token-footprint section                                                                   | `make audit-tokens`, `make gen-footprint`             |
| `audit_metrics`                | Surface quality audits        | Comprehensive metrics summary (tools, resources, prompts, codebase); `-site-stats` writes the site's stats JSON                                                                                     | `make audit-metrics`, `make gen-site-stats`           |
| `gen_graphql_schema`           | Generators                    | Pins a GitLab GraphQL schema by introspecting a live instance; `--check` gates the committed one                                                                                                    | `make gen-graphql-schema`, `make check-graphql-schema` |
| `godoc_tool`                   | Source quality audits         | Godoc compliance auditor and fixer (audit + fix subcommands)                                                                                                                                        | `make audit-godocs`                                   |
| `audit_test_names`             | Source quality audits         | Classifies `Test*` functions by naming pattern; emits rename hints; `-check-files` gates test-file naming                                                                                           | `make audit-test-names`, `make check-test-file-names` |
| `audit_test_goroutines`        | Source quality audits         | `testing.T` aborts made off the test goroutine                                                                                                                                                      | `make check-test-goroutines`                          |
| `audit_test_subtests`          | Source quality audits         | Case loops that assert without a `t.Run` subtest; `-fix` rewrites the unambiguous ones                                                                                                              | `make check-test-subtests`                            |
| `audit_md_escaping`            | Source quality audits         | Values a Markdown formatter interpolates into a table cell, heading, list item or link without an escaping helper                                                                                   | `make check-md-escaping`                              |
| `audit_string_dupes`           | Source quality audits         | Finds duplicated string literals missing `const`/`var` declarations                                                                                                                                 | —                                                     |
| `audit_supply_chain`           | Release & supply-chain audits | Five release-configuration invariants: pinned actions, credentialed jobs that run no run-time-resolved code, stated Dependabot cooldowns, a current security policy, signature-verifying installers | `make check-supply-chain`                             |
| `audit_install_buttons`        | Release & supply-chain audits | Decodes every one-click install button and holds the buttons to one configuration per command                                                                                                       | `make check-install-buttons`                          |
| `gen_action_catalog_manifest`  | Generators                    | Generates the ActionSpec group-builder manifest                                                                                                                                                     | `make gen-action-catalog-manifest`                    |
| `gen_lhm_manifest`             | Generators                    | Generates the tools/prompts/resources arrays in `lhm.plugin.json` (LobeHub Marketplace)                                                                                                             | `make gen-lhm-manifest`                               |
| `gen_llms`                     | Generators                    | Generates `llms.txt` and `llms-full.txt`                                                                                                                                                            | `make gen-llms`                                       |
| `gen_stats`                    | Generators                    | Regenerates the managed repository statistics section in `README.md`                                                                                                                                | `make gen-stats`                                      |
| `gen_testing_docs`             | Generators                    | Regenerates the test-metrics block in `docs/development/testing/testing.md`                                                                                                                         | `make gen-testing-docs`                               |
| `gen_docker_tools`             | Generators                    | Generates a Docker MCP Registry-compatible `tools.json`                                                                                                                                             | —                                                     |
| `gen_brand`                    | Generators                    | Emits every vector brand asset from one parametric geometry                                                                                                                                         | `make brand`, `make brand-check`                      |
| `gen_icon_webp`                | Generators                    | Rasterizes the SVG icons into light/dark WebP fallbacks (maintainer-only)                                                                                                                           | `make gen-icon-webp`                                  |
| `format_md_tables`             | Formatters                    | Normalizes Markdown pipe tables in `README.md`, `docs/` and `site/src/content/docs/`                                                                                                                | part of `make audit-docs`                             |
| `bench_resources`              | Benchmarks                    | Measures what the server costs to run (memory, startup, a second credential) and draws the published charts                                                                                         | `make bench-resources`                                |
| `eval_mcp_surfaces`            | Evaluation                    | Evaluates model behavior across MCP tool surfaces                                                                                                                                                   | `make eval-surfaces-docker*`                          |
| `server`                       | Server                        | The main `gitlab-mcp-server` MCP binary (runtime entry point)                                                                                                                                       | `make build`, `make run`                              |

## SDK/API parity audits

### audit_1to1

Consolidated 1:1 SDK↔API parity audit. It combines four gap streams behind a single `-scope` flag: struct field mapping (R-INPUT/R-OUTPUT), action coverage (R-ACTION), discovery metadata (R-META), and enum values (R-ENUM). When all four scopes run (the default), it produces a merged per-package backlog. It replaces the former `audit_struct_completeness`, `audit_action_coverage`, `audit_metadata_completeness`, and `gen_1to1_backlog` binaries.

A fifth scope, `sdk`, differs from the first three in both its universe and its severity, and is described under [SDK parity gate](#sdk-parity-gate) below. The enum stream shares that severity: it is merged into the backlog like the candidate streams, and folded into the `sdk` gate so a finding fails the build. It is described under [Enum values](#enum-values-r-enum).

#### Usage

```bash
# Run all four streams and write a merged backlog
go run ./cmd/audit_1to1/

# Single-scope run, gaps only, to stdout
go run ./cmd/audit_1to1/ -scope=structs -gaps-only -output=-

# Enum value rule on its own: exits non-zero on a finding
go run ./cmd/audit_1to1/ -scope=enums -gaps-only

# SDK parity gate (services, raw GraphQL, enum values): exits non-zero on a finding
go run ./cmd/audit_1to1/ -scope=sdk -gaps-only

# Validate that every doc/api citation behind the adjudication tables is still
# fetchable (the official API doc is the 1:1 ground truth)
go run ./cmd/audit_1to1/ -validate-docs
```

#### Flags

| Flag             | Type       | Default                          | Description                                                                                                                                        |
| ---------------- | ---------- | -------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- |
| `-gaps-only`     | `bool`     | `false`                          | Only include entries with at least one finding                                                                                                     |
| `-output`        | `string`   | `-`                              | Path to write the JSON report, or `-` for stdout                                                                                                   |
| `-scope`         | `string`   | `structs,actions,metadata,enums` | A single `{structs,actions,metadata,enums,sdk}` scope, or exactly the first four (default, merged backlog); any other combination is rejected      |
| `-validate-docs` | `bool`     | `false`                          | Instead of the audit, verify every `doc/api/<area>.md` citation in the adjudication tables is still fetchable (exits non-zero on a stale citation) |
| `-refresh`       | `bool`     | `false`                          | With `-validate-docs`, force re-fetch of cited docs even when cached and fresh                                                                     |
| `-offline`       | `bool`     | `false`                          | With `-validate-docs`, use only cached docs; do not fetch                                                                                          |
| `-max-age`       | `duration` | `168h`                           | With `-validate-docs`, re-download cached docs older than this (default 7 days)                                                                    |

#### Output

JSON. A single-scope run produces that auditor's native shape. An all-scopes run produces a merged backlog containing `schema_version`, a `summary` block (11 counters), and a `packages[]` array in which each package carries its `struct`, `actions`, `metadata` and `enums` sections. `-validate-docs` emits `{schema_version, checked, ok, stale[]}`.

#### SDK parity gate

`-scope=sdk`. The three candidate streams derive their universe from this repository: they walk our call expressions and report what those calls miss. That cannot answer "is there a service we call nothing on", because a service nothing references never enters the map. `WorkItemSavedViewsService` arrived upstream with seven methods, sat entirely unexposed, and the audit went on reporting zero gaps.

The `sdk` scope enumerates the services from client-go's `Client` struct instead, and holds each one to a decision:

- **covered** — a handler calls it;
- **declared** — `declaredServices` in `cmd/audit_1to1/internal/sdk/decisions.go` names it, with a category (`COVERED_RAW`, `COVERED_GENERIC`, `COVERED_GRAPHQL`, `SUPERSEDED_UPSTREAM`, `UNWRAPPED_TRACKED`) and the evidence behind it;
- **undeclared** — a finding.

It carries a second rule for the same reason. [ADR-0006](adr/adr-0006-raw-graphql-for-uncovered-domains.md) admits raw `GraphQL.Do()` for domains **without** a client-go service wrapper; the wrapper appearing later is what retires that exemption, and nothing was checking. Every raw-GraphQL operation whose package maps to a client-go service is therefore held to a decision too, `KEEP` or `MIGRATE`, in `graphqlDecisions`. The unit is the **operation**, not the package: several packages use GraphQL for one operation and the SDK for the rest, so a package-level verdict would be mostly noise.

The gate also carries the enum value rule described next, so one `-scope=sdk` run answers every question whose universe is the SDK rather than our call sites.

All three tables are checked for staleness in the same run: a declaration for a service the tree now calls, a declaration for a service upstream removed, a decision for an operation that no longer exists, or an enum exemption that excuses nothing is itself a finding.

Unlike the candidate scopes this one **exits non-zero** on a finding, and is deliberately kept out of the merged backlog so `plan/1to1-backlog.json` keeps its shape for the tooling that reads it.

Report keys: `schema_version`, `client_go_path`, a `summary` block (12 counters), `services[]`, `graphql_operations[]`, `enum_fields[]`, and `stale_declarations[]`. With `-gaps-only` the first three arrays hold only findings; `stale_declarations[]` never holds anything else, so the flag does not change it.

#### Enum values (R-ENUM)

`-scope=enums`, and folded into `-scope=sdk`. The struct rule projects a client-go enum type (`type XxxValue string`, or an integer kind, with a `const` block of values) to a scalar in its field comparison, so a field of that type counts as covered the moment a same-named scalar exists on our side, and the **values** the SDK declares for it are never read. A constant added upstream was therefore invisible while the field stayed covered; the Dependency Firewall ecosystems were guarded against exactly that by a hand-written test that named each of the eleven constants, a list a twelfth constant would pass unchanged.

The enum rule reads the values instead. For every client-go enum type it finds the fields of that type an action exposes, through the same (MCP struct, SDK struct) pairs the struct rule diffs, and compares the SDK's constant set with the values our surface offers:

- **`enum`** — the schema property (or an array property's `items`) carries an `enum` list. Compared both ways: a value the SDK declares that the list lacks is **missing**, a value the list carries that the SDK does not declare is **extra**.
- **`description`** — the property has no enum and its description names at least one of the values, as whole tokens (`0=No access, 30=Developer`, `cargo, composer, or npm`). Prose can only confirm a value, so it is compared one way: unmentioned SDK values are missing, and nothing is extra. A sentence that names values in order to exclude them (`60=Admin is not valid`) is skipped.
- **`none`** — nothing on the surface says what the values are. For an **input** every SDK value is missing, since a model has to choose one and nothing says what the choices are. For an **output** the field is counted as `unsurfaced_output_fields` and not reported: the output relays whatever GitLab answers, and its schema is reflected from the response struct, whose fields carry no descriptions. The rule holds an output to its values only where a description or an enum surfaces them, which is where a stale list would mislead.

A documented per-endpoint subset (the branch protection levels are `0`, `30`, `40` and `60` out of the ten `AccessLevelValue` constants) and a documented value the SDK has no constant for (`epic` as an events `target_type`) are both recorded in `acceptedEnumGaps` in `cmd/audit_1to1/internal/enums/exemptions.go`, keyed `<pkg>.<MCPType>.<tag>` for a whole field or `<pkg>.<MCPType>.<tag>=<value>` for one value, each with the `doc/api` page or the upstream-bugs entry that justifies it. An exemption that excuses nothing is stale and fails the gate, so the table cannot outlive the gaps it describes. The SDK gaps behind the extra values are recorded in [upstream-bugs.md](upstream-bugs.md).

Report keys: `schema_version`, `client_go_path`, a `summary` block (`sdk_enums`, `fields`, `fields_with_gaps`, `unsurfaced_output_fields`, `missing_values`, `extra_values`, `stale_exemptions` and `packages`), `packages[]` with one finding per (action, field), and `stale_exemptions[]`. With `-gaps-only` only the fields with a finding are listed.

#### Make targets

- `make audit-1to1` — writes `plan/1to1-backlog.json`, then runs `audit-1to1-sdk`.
- `make audit-1to1-sdk` — the SDK parity gate, enum values included; fails the build on a finding.
- `make audit-1to1-enums` — the enum value rule alone; fails the build on a finding.
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

### audit_doc_tool_names

Checks every `gitlab_*` tool name the documentation mentions against the names the server actually registers. `audit_doc_coverage` compares `domain.action` IDs, so a page can name a tool no surface has ever registered and still audit clean; that is how a verb-first spelling of the issue list survived in guides while the individual surface projects `gitlab_issue_list`, and every copy-pasted example answered `unknown tool`. The name set is built in memory from the same registration paths the server uses, across the individual, meta and dynamic surfaces at the Ultimate tier, so it needs no network and cannot drift from the catalog.

The roots scanned are `docs/`, `site/src/content/docs/`, `README.md`, `llms-install.md`, `CLAUDE.md` and `npm/gitlab-mcp-server/README.md`; the npm launcher's README is in the list because it is published to a registry, where a wrong name is not fixable without republishing a version. Tokens that look like tool names but are not (the evaluator's bridge tools, for example) are listed in the source with the reason for each exemption.

#### Usage

```bash
# Report
go run ./cmd/audit_doc_tool_names/

# CI gate
go run ./cmd/audit_doc_tool_names/ --check
```

#### Flags

| Flag     | Type   | Default | Description                                                 |
| -------- | ------ | ------- | ----------------------------------------------------------- |
| `-check` | `bool` | `false` | Exit non-zero when the docs name a tool that does not exist |

#### Output

The number of registered names and of documentation files scanned, then each unregistered name with the files that mention it. Exits `1` under `-check` when any is found, and `1` whenever the documentation tree cannot be scanned.

#### Make targets

- `make audit-doc-tool-names` — the report.
- `make check-doc-tool-names` — CI gate.

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

### audit_e2e_gaps

Reports which canonical catalog actions the e2e suite under `test/e2e/suite` never exercises. It builds the Ultimate-tier action catalog offline and scans the suite sources for the three invocation shapes: individual tool names (`gitlab_branch_create`), meta calls (a `gitlab_branch` literal followed by an `"action": "create"` pair within a short window), and dynamic execute calls naming canonical `domain.action` IDs. An action counts as exercised when any surface references it.

#### Usage

```bash
go run ./cmd/audit_e2e_gaps/
go run ./cmd/audit_e2e_gaps/ -output json
```

#### Flags

| Flag      | Type     | Default          | Description                    |
| --------- | -------- | ---------------- | ------------------------------ |
| `-suite`  | `string` | `test/e2e/suite` | e2e suite source directory     |
| `-output` | `string` | `tsv`            | Output format: `tsv` or `json` |

#### Output

With `tsv`, one tab-separated row per uncovered action (`id`, group, edition, `readonly=`, `destructive=`) and a summary line, `e2e gap audit: N/M actions exercised (P%), K uncovered`. With `json`, a report carrying the catalog count, the exercised count and the uncovered rows. Any other format is rejected with exit `2`; a catalog that cannot be built or a suite that cannot be scanned exits `1`. Gaps alone do not fail the command: it is a work list, not a gate.

#### Make targets

- `make audit-e2e-gaps`

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

### audit_readonly_graphql

Fails when an action the canonical catalog classifies `ReadOnly` can reach a GraphQL mutation.

`--read-only` removes actions through `FilterReadOnlyActions`, and the surface served to a `read_api` OAuth token is narrowed the same way. Both key on the action's catalog classification, not on what its handler does, so an action classified `ReadOnly` whose handler issues a mutation survives both filters and writes precisely where a write is supposed to be impossible.

The HTTP method cannot be the test: `client-go` sends every GraphQL request as a POST, so around twenty read-only actions legitimately POST. The operation type in the document is the whole of the distinction, and it is in the source.

The audit loads `./internal/...`, resolves every read-only catalog action to the function its route runs, walks what that function can call, and classifies every GraphQL document those bodies name. An action that sends no GraphQL is not a finding, and neither is a mutation reached from an action already classified as mutating.

#### Usage

```bash
# CI gate
go run ./cmd/audit_readonly_graphql/

# Also list the read-only actions that touch GraphQL at all
go run ./cmd/audit_readonly_graphql/ -v
```

#### Flags

| Flag   | Type     | Default | Description                                   |
| ------ | -------- | ------- | --------------------------------------------- |
| `-dir` | `string` | `.`     | Repository root to audit                      |
| `-v`   | `bool`   | `false` | Report what was checked, not only what failed |

#### Output

One block per finding on stderr, naming the action, the file the action is declared in, the function that sends the mutation, and the document. Exits `1` when any finding is reported and when the catalog or the source tree cannot be loaded, so a gate that cannot read its inputs never looks like a gate that passed.

Three things count as findings, not only the obvious one:

- a read-only action whose handler can reach a mutation document;
- a read-only action no `ActionSpec` construction resolves to, or whose route resolves to no handler, because an action the audit cannot classify is one it cannot vouch for;
- an exception directive that no longer excuses anything, so an exception cannot outlive its reason.

#### Declaring an exception

A deliberate exception is declared in the source next to the action, never in the auditor:

```go
//gitlab:allow-readonly-graphql-mutation <action_name>: <reason>
```

The directive has to sit in the package that owns the action and name that action, so the exception is visible to a reader of the handler.

#### Make targets

- `make check-readonly-graphql`: the CI gate.
- `make audit-readonly-graphql`: the same gate, with the GraphQL-sending read-only actions listed.

### audit_graphql_documents

Fails when a raw GraphQL document in the source is one the pinned GitLab schema refuses.

Until this existed, no test in the repository could fail for the reason that matters. Every GraphQL test answers the request from an `httptest` handler that returns whatever the test wrote, so a passing test proved that our handler agreed with our own fixture and said nothing about whether GitLab would accept the document. Four registered tools shipped documents no current instance accepts, with every test green.

`internal/testutil.NewTestClient` now validates every document a test sends, which covers most of them and cannot cover all of them: a document no test drives still ships. This audit reads them out of the source instead, so the coverage of the gate stops depending on the coverage of the tests.

It loads the whole program with `go/packages` rather than matching the source with a regular expression, because four of this repository's documents are assembled by concatenating a shared fragment constant and only the type checker knows what the assembled value is. Constants are folded during type checking, so a document written as three pieces is judged as the one string GitLab would receive.

What it cannot check is variables: a document read out of the source has no request behind it, so nothing says which variables a handler will send or what they will hold. That half belongs to the test transport, which sees a real request.

#### Usage

```bash
# CI gate
go run ./cmd/audit_graphql_documents/

# Also list every document it accepted
go run ./cmd/audit_graphql_documents/ -v
```

#### Flags

| Flag   | Type     | Default | Description                                            |
| ------ | -------- | ------- | ------------------------------------------------------ |
| `-dir` | `string` | `.`     | Repository root to audit                               |
| `-v`   | `bool`   | `false` | List every document checked, not only the refused ones |

#### Output

One block per refused document on stderr, naming the package, the constant it is declared as (or "an inline document"), the file and line, and every validation error underneath. The summary line carries the pin's provenance, so a reader is told which instance and which day the judgement came from. Exits `1` on any refusal, on a source tree that cannot be loaded, and when no documents are found at all, since an audit that finds nothing is looking in the wrong place.

#### Make targets

- `make check-graphql-documents`: the CI gate.
- `make audit-graphql-documents`: the same gate, listing what it accepted.

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

### audit_gateway_chars

Scans everything a client receives from `tools/list`, `prompts/list` and `resources/list`, on every tool surface and at the widest tier, for characters that MCP gateway validators reject. It exists because of a real rejection: a gateway introspecting this server refused onboarding with `Description contains unsafe characters: ';'`. The semicolons were ordinary English punctuation, but the gateway is the door, and the door's rules win. The audit measures the served surface rather than grepping the source, because a description is assembled from several source strings and a semicolon that survives assembly is a rejection wherever it came from. The policy is pure ASCII prose plus a short list of rejected ASCII characters (the semicolon).

#### Usage

```bash
# Report every offender with enough context to find the source string
go run ./cmd/audit_gateway_chars/

# CI gate
go run ./cmd/audit_gateway_chars/ -check

# Verify that a GITLAB_MCP_DESCRIPTION_SUBSTITUTIONS value clears the audit
GITLAB_MCP_DESCRIPTION_SUBSTITUTIONS='old=new' go run ./cmd/audit_gateway_chars/ -apply -check
```

#### Flags

| Flag     | Type   | Default | Description                                                                                                    |
| -------- | ------ | ------- | -------------------------------------------------------------------------------------------------------------- |
| `-check` | `bool` | `false` | Exit non-zero if any offending character is served                                                             |
| `-apply` | `bool` | `false` | Apply `GITLAB_MCP_DESCRIPTION_SUBSTITUTIONS` before scanning, to verify a substitution config clears the audit |
| `-full`  | `bool` | `false` | Print each offending string whole (tab-separated) instead of a one-line excerpt                                |

#### Output

One line per offender (surface, location, excerpt), sorted by surface, then a summary line. Exits `1` under `-check` when anything is served with an offending character, and `1` when `-apply` is given a malformed substitution value.

#### Make targets

- `make audit-gateway-chars` — the report.
- `make check-gateway-chars` — CI gate.

### audit_tokens

Measures the LLM context-window overhead of every registered tool/resource/prompt definition across the individual, meta, and dynamic surfaces using the cl100k_base tokenizer (via `github.com/tiktoken-go/tokenizer`, with a bytes/4 fallback). With `--compare-schemas`, it runs a sizing spike comparing `GITLAB_MCP_META_PARAM_SCHEMA` modes. With `-footprint`, it measures every tier × surface × schema-mode combination and regenerates the README token-claim block and token-footprint section, the standalone reference doc and the site's `token-footprint.json`; add `-check` to verify those are current without writing (CI gate).

#### Usage

```bash
# Default token audit
go run ./cmd/audit_tokens/

# Compare GITLAB_MCP_META_PARAM_SCHEMA modes
go run ./cmd/audit_tokens/ -compare-schemas

# Regenerate the README token-footprint section + docs/development/token-footprint.md
go run ./cmd/audit_tokens/ -footprint

# Verify the token-footprint section + doc are current without writing (CI gate)
go run ./cmd/audit_tokens/ -footprint -check
```

#### Flags

| Flag               | Type   | Default | Description                                                                                                                                                                                                              |
| ------------------ | ------ | ------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `-footprint`       | `bool` | `false` | Measure all tiers × surfaces × `GITLAB_MCP_META_PARAM_SCHEMA` modes and write the README token-claim block and token-footprint section, `docs/development/token-footprint.md` and `site/src/data/token-footprint.json`   |
| `-check`           | `bool` | `false` | With `-footprint`, verify the README token-claim block and token-footprint section, `docs/development/token-footprint.md` and `site/src/data/token-footprint.json` are current without writing (exits non-zero on drift) |
| `-compare-schemas` | `bool` | `false` | Compare `GITLAB_MCP_META_PARAM_SCHEMA` modes (opaque/full/compact) for meta-tool InputSchema sizing instead of the normal token audit                                                                                    |
| `-json`            | `bool` | `false` | Emit a JSON summary instead of the Markdown report                                                                                                                                                                       |
| `-top-tools`       | `int`  | `30`    | Number of individual tools to list by token cost                                                                                                                                                                         |
| `-top-domains`     | `int`  | `20`    | Number of domains to list by token cost                                                                                                                                                                                  |

#### Output

Default mode: a Markdown report to stdout with mode comparison, per-tool costs, and domain totals. `-compare-schemas` mode: a sizing table with opaque/full/compact byte costs per meta-tool. `-footprint` mode: rewrites the token-claim block and the `<!-- START TOKEN FOOTPRINT -->` section of `README.md`, and writes `docs/development/token-footprint.md` and `site/src/data/token-footprint.json`.

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

| Flag           | Type     | Default | Description                                                                                                       |
| -------------- | -------- | ------- | ----------------------------------------------------------------------------------------------------------------- |
| `-json`        | `bool`   | `false` | Emit a JSON summary instead of the Markdown report                                                                |
| `-top-domains` | `int`    | `20`    | Number of domains to list by tool count (must be >= 0)                                                            |
| `-site-stats`  | `string` | `""`    | Write the single-sourced site stats JSON (`site/src/data/stats.json`) to this path instead of printing the report |
| `-check`       | `bool`   | `false` | With `-site-stats`, verify the committed file is current instead of writing it (exits non-zero on drift)          |

#### Output

A Markdown report to stdout by default, or a JSON summary with `-json`. With `-site-stats`, the stats JSON the documentation site reads.

#### Make targets

- `make audit-metrics`
- `make gen-site-stats` — writes `site/src/data/stats.json`.
- `make check-site-stats` — CI gate; also part of `make audit-docs`.

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

# Move every package comment below a path into a doc.go of its own
go run ./cmd/godoc_tool/ fix --move-package-doc ./internal/
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
- `make check-test-goroutines` — CI gate; also step [6/8] of `make analyze`.

### audit_test_names

Scans Go `_test.go` files and classifies `Test*` functions by naming pattern (3-part / 2-part / no-underscore / TestCov / skip), then emits rename suggestions.

#### Usage

```bash
# Scan the standard source directories
go run ./cmd/audit_test_names/ cmd internal test
```

#### Flags

| Flag           | Type   | Default | Description                                                                                              |
| -------------- | ------ | ------- | -------------------------------------------------------------------------------------------------------- |
| `-apply`       | `bool` | `false` | Rename test functions in place to match the suggested names                                              |
| `-dry-run`     | `bool` | `false` | Print what would be renamed without writing files (use with `-apply`)                                    |
| `-check-files` | `bool` | `false` | Audit test **file** names against the module-naming convention instead, and exit non-zero on a violation |

Pass one or more positional directory arguments after the flags; with none the command prints its usage and exits `1`.

#### Positional arguments

| Argument   | Type       | Description         |
| ---------- | ---------- | ------------------- |
| `<dir>...` | positional | Directories to scan |

#### Output

CSV to stdout (`file,current_name,pattern,suggested_name`), with a summary printed to stderr. `-check-files` prints the offending file names instead.

#### Make targets

- `make audit-test-names` — runs with `cmd internal test`.
- `make check-test-file-names` — `-check-files cmd internal test`; CI gate for test-file naming (`export_test.go`, build-constrained and external-package qualifiers, and `test/e2e` are the codified exemptions).

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
- `make check-test-subtests` — CI gate; also step [7/9] of `make analyze`.

### audit_md_escaping

Type-checks the packages under `internal/`, finds every call that writes Markdown with a runtime value in it, and fails when a value this server did not write can reach a construct it can change the shape of. `toolutil.EscapeMdTableCell` belongs on every GitLab-authored string between two pipes of a table row and on every single-line list value, `toolutil.EscapeMdHeading` on the one value a formatter puts in a heading, and `toolutil.MdTitleLink` on both halves of a link. The containment is not only table geometry: `EscapeMdTableCell` entity-encodes `<` so GitLab-authored text cannot open raw HTML in a client that renders Markdown, and a title of `` Fix login](http://attacker.invalid/x) `` closes the label of a hand-built link and opens a destination of its own.

A sink is an `fmt` formatting call whose format argument is a constant (a literal or a named constant, both resolved by the type checker) or a call of `toolutil.MarkdownTableRow` or `MarkdownTableHeader`, which have no template because every argument they take is a cell. The template is parsed with `fmt`'s own grammar, explicit argument indices included, so `[%[1]s](%[1]s)` pairs correctly; only `%s`, `%v` and `%q` are judged. The construct comes from the line the hole sits on: a leading pipe is a table cell, one to six `#` and a space a heading, a bullet or ordered marker a list item, an unclosed `[` a link label, and the text after `](` a link destination. Prose is skipped.

Each value is then followed back to where it came from: a constant, a non-textual type, an escaper, a nested `Sprintf` of safe halves, a `strings` transform of safe values, a helper whose every return is safe, a local whose every assignment is safe, or a parameter every caller passes a safe value to. A call binds its arguments to the callee's parameters, so a helper is judged at the call site that reaches it, which matters for `toolutil.FormatTime`: it returns its argument verbatim when neither layout parses, and it is called from about a hundred and fifty places. A value that bottoms out at a field of a struct filled from a GitLab response is a finding; anything the walk cannot follow is reported in an unresolved bucket of its own and never counted as safe.

```bash
go run ./cmd/audit_md_escaping/
go run ./cmd/audit_md_escaping/ -v -json plan/md-escaping-backlog.json
go run ./cmd/audit_md_escaping/ -check
go run ./cmd/audit_md_escaping/ -contexts table-cell,heading -check
go run ./cmd/audit_md_escaping/ -check ./internal/tools/issues
```

#### Flags

| Flag               | Type   | Default | Description                                                                                                                     |
| ------------------ | ------ | ------- | ------------------------------------------------------------------------------------------------------------------------------- |
| `-dir`             | string | `.`     | Repository root to audit                                                                                                        |
| `-json`            | string | _(off)_ | Write the JSON work list to this path                                                                                           |
| `-contexts`        | string | `all`   | Constructs to judge: `all`, or a comma-separated list of `heading`, `link-destination`, `link-label`, `list-item`, `table-cell` |
| `-check`           | bool   | `false` | Exit non-zero when a value still reaches a Markdown construct unescaped                                                         |
| `-v`               | bool   | `false` | List the excused and unresolved values as well as the failing ones                                                              |
| `-fail-unresolved` | bool   | `false` | Count a value the audit cannot follow as a failure                                                                              |

Package patterns after the flags narrow the sweep, which makes checking one domain while working on it cheap. The default is `./internal/...`.

#### Declaring that a value is already safe

Escaping a value that needs none is noise that teaches the next reader the wrong rule, so an exemption is declared in the source, in the package that owns the formatter:

```go
//gitlab:allow-unescaped result.ID: a canonical catalog ID, compiled in from an ActionSpec rather than read from GitLab.
```

The expression is the one the report prints, and the directive excuses that expression wherever the package interpolates it. A directive that excuses nothing fails the gate, so an exemption cannot outlive the reason it was written for.

#### Output

Findings grouped by package, each naming the file, line, formatter, construct, verb, expression, the helper it wants and how the walk got there; then a summary with the counts and a breakdown by construct. `-v` adds the excused and unresolved buckets. The JSON work list carries `findings`, `unresolved`, `excused`, `stale_directives` and a `summary` with per-construct and per-package counts.

#### Make targets

- `make audit-md-escaping` — report plus `plan/md-escaping-backlog.json`.
- `make check-md-escaping` — CI gate; also step [9/9] of `make analyze`.

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

## Release & supply-chain audits

### audit_supply_chain

Checks five properties of the release configuration, each of which was false at some point and none of which any other gate in this repository can see:

1. Every `uses:` in `.github/workflows` is pinned to a 40-character commit SHA. A mutable tag is resolved by the runner at job start, so a hijacked `v7` is consumed with no pull request, no cooldown and no review.
2. A job holding `contents: write` or `id-token: write` runs no code resolved at run time: no `npx`, no `@latest`, no `curl` piped into a shell, no `pip install` without `--require-hashes`, in its own `run:` blocks or in any `scripts/` file those blocks invoke. `actions/checkout` must set `persist-credentials: false`, a downloaded tool (GoReleaser) must be pinned to an exact `vX.Y.Z`, and `anchore/sbom-action` is refused outright, because on Linux it fetches `install.sh` from syft's `main` branch and runs it.
3. Dependabot states a cooldown of its own rather than inheriting a platform default GitHub can change.
4. `SECURITY.md` names the major version the repository actually ships, and marks no older major as supported.
5. Both installers verify the release's Sigstore bundle, not only a `checksums.txt` fetched from the same mutable release.

Pinning is decided on the raw file text and job structure on the parsed YAML, so a `uses:` inside a comment or an unparsed region still counts. A version pin held in the workflow's top-level `env:` block and referenced from a step is resolved through that one indirection.

```bash
go run ./cmd/audit_supply_chain/
go run ./cmd/audit_supply_chain/ --root /path/to/a/checkout
```

#### Flags

| Flag     | Type   | Default             | Description                                  |
| -------- | ------ | ------------------- | -------------------------------------------- |
| `--root` | string | _(the module root)_ | Repository root to audit instead of this one |

#### Output

One line per violation under a `supply-chain audit FAILED (N problems):` header, or a single success line naming the five properties. Exits `1` when any invariant is broken and when the audit cannot be performed at all, so a gate that cannot read its inputs never looks like a gate that passed.

#### Make targets

- `make check-supply-chain` — CI gate; also step [8/8] of `make analyze`.

### audit_install_buttons

Checks the one-click install buttons against what the pages around them claim. A button's configuration travels inside its URL, base64 in every client this project links and percent-encoded on top of that in some, so nothing about it is visible in review and no text search finds a flag inside it: removing `--http=false` from every example in the tree left eight buttons still registering it. The audit therefore decodes rather than searches, and holds the buttons to the promise the prose makes, that every button registers the same configuration. Buttons are grouped by the command they launch, since a Docker button and an npx button are different configurations on purpose, and within a group the arguments have to agree.

#### Usage

```bash
go run ./cmd/audit_install_buttons/
go run ./cmd/audit_install_buttons/ -v
```

#### Flags

| Flag   | Type     | Default | Description                        |
| ------ | -------- | ------- | ---------------------------------- |
| `-dir` | `string` | `.`     | Repository root to audit           |
| `-v`   | `bool`   | `false` | List every button that was checked |

#### Output

`audit_install_buttons: N buttons decode cleanly and agree within each command`, or one line per problem on stderr followed by a count, with exit `1`. Finding no button at all is also exit `1`, because it means the audit is looking in the wrong place.

#### Make targets

- `make check-install-buttons` — CI gate.

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

### gen_graphql_schema

Pins a GitLab GraphQL schema into `internal/graphqlschema`, where the test transport and `audit_graphql_documents` both read it.

It introspects a live instance, converts the answer to SDL, and writes `gitlab-schema.graphql.gz` beside `source.json`, which records the instance, the version it reported and the day it answered so a reader can tell how old the pin is without asking git. GitLab answers introspection to anyone, so no token is needed for the schema itself; `GITLAB_TOKEN` is read only so the record can name the version, which GitLab refuses to tell an anonymous caller.

The conversion emits objects with their `implements` clauses, interfaces, unions, enums, input objects, custom scalars, argument and input defaults, and the `schema { query mutation subscription }` block. Built-in scalars and `__`-prefixed types are omitted, because gqlparser's own prelude already defines them and emitting them again fails the load with a duplicate definition. Everything nameable is sorted, so a re-pin produces a diff of what changed rather than a reshuffle. Descriptions and deprecation reasons are dropped: validation never consults them and they would triple the file.

Nothing is written until the converted schema has been loaded back through the same parser that will judge every document, so a renderer that dropped an `implements` clause fails here rather than by refusing a valid document months later.

Generating needs the network, so it is not a gate. `--check` is: it loads the committed files from disk and fails when the schema does not parse or the record does not decode.

#### Usage

```bash
# Re-pin from gitlab.com
go run ./cmd/gen_graphql_schema/

# Re-pin from a self-managed instance
go run ./cmd/gen_graphql_schema/ -url https://gitlab.example.com/api/graphql

# CI gate, no network
go run ./cmd/gen_graphql_schema/ --check
```

#### Flags

| Flag     | Type     | Default                          | Description                                                                      |
| -------- | -------- | -------------------------------- | -------------------------------------------------------------------------------- |
| `-url`   | `string` | `https://gitlab.com/api/graphql` | GraphQL endpoint to introspect                                                   |
| `-dir`   | `string` | `internal/graphqlschema`         | Directory holding the pinned schema and its provenance record                    |
| `-check` | `bool`   | `false`                          | Load the committed schema instead of fetching one, and fail if it does not parse |

#### Output

Writes `gitlab-schema.graphql.gz` and `source.json` into `-dir`, and reports the type count and provenance on stdout. `--check` writes only the summary. Exits `1` on any failure, so a half-written artifact never reaches a branch.

#### Make targets

- `make gen-graphql-schema`
- `make check-graphql-schema` — CI gate.

### gen_lhm_manifest

Regenerates the `tools`, `prompts`, and `resources` arrays in `lhm.plugin.json`, the manifest published to the LobeHub Marketplace. LobeHub derives the listing's capability badges from those arrays — its scanner cannot introspect a server distributed as a Go binary or a Docker image — so a manifest without them advertises zero tools no matter what the server registers.

The declared tool surface is the default one, `dynamic`, pinned explicitly rather than read from `GITLAB_MCP_TOOL_SURFACE`; the round-trip runs against an in-process stub client rather than `GITLAB_URL`/`GITLAB_TOKEN`. Both keep the committed file independent of the machine that generated it. Output schemas and tool icons are dropped: neither is part of the shape LobeHub documents, and the base64 icon data URIs alone would triple the file. Every other field is preserved, `version` included — the release stamp owns that one.

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

Rewrites six files at the project root: `llms.txt`, `llms-full.txt`, `llms-medium.txt`, and the
three per-surface splits (`llms-full-meta-tools.txt`, `llms-full-individual-tools.txt`,
`llms-full-resources-prompts.txt`). The companions are rendered before `llms.txt` is, because
`llms.txt` quotes each one's size and token estimate, and `llms-full.txt` is far past every
context window, so an index that does not say so sends models at it anyway.

These describe the **server**. The documentation site publishes its own index of documentation
pages instead, generated by `site/scripts/gen-llms.mjs` from the Astro content collection, and
republishes these six alongside it (`llms.txt` becomes `llms-server.txt` there, so the two never
collide and `make check-llms` keeps checking the files it generates).

#### Make targets

- `make gen-llms`
- `make check-llms` — CI gate, also part of `make audit-docs`.
- `pnpm run llms:check` in `site/` — checks the site index's section table still covers the
  content collection.

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

# CI gate: everything a checkout determines, in seconds
go run ./cmd/gen_testing_docs/ --check -skip-coverage

# Refresh the counts without recomputing coverage, keeping the recorded values
go run ./cmd/gen_testing_docs/ -skip-coverage

# Verify the coverage values as well, which takes minutes
go run ./cmd/gen_testing_docs/ -check

# Give a slow package more room than the 30 minutes each go test run gets
go run ./cmd/gen_testing_docs/ -timeout 45m
```

`-skip-coverage` carries the coverage already recorded in the document forward
instead of blanking it, so it is a real refresh of everything else and, with
`--check`, a freshness gate that holds everything the source tree determines:
the counts, the naming breakdown, the per-layer tables, and the set of packages
in the coverage tables, which is what caught `cmd/audit_install_buttons` missing
from them. It takes seconds, because it runs no coverage at all.

The coverage values are not gated, because they are a property of the machine as
much as of the tree. Several tests assert refusals that permission bits never
produce for uid 0 and skip when the process is privileged, so
`cmd/format_md_tables` measures 95.8% as root and 96.7% otherwise, and
`internal/tools/projectimportexport` 99.5% and 100.0%. `rsvg-convert` and
`cwebp`, installed on the maintainer machine and deliberately absent from CI,
move `cmd/gen_icon_webp` from 90.2% to 92.3%. `cmd/server` measures 95.7% run on
its own and 95.6% inside a loaded full pass, on a branch that depends on timing.
A byte-exact check of those columns cannot pass in two places at once, so
`make gen-testing-docs` refreshes them and no gate holds them.

`-timeout` is the bound handed to every `go test` this generator runs, not just
its own deadline. Without it the coverage pass obeyed the 10 minutes `go test`
applies by default, which `cmd/audit_metrics` exceeds under coverage
instrumentation on a loaded machine, and nothing outside the generator could
raise it. The generation as a whole gets that budget once per `go test` run it
issues, plus five minutes for package listing, the coverage summary and the
document write.

#### Flags

| Flag               | Type       | Default                               | Description                                                              |
| ------------------ | ---------- | ------------------------------------- | ------------------------------------------------------------------------ |
| `-check`           | `bool`     | `false`                               | Fail if the generated section is not current                             |
| `-coverage-dir`    | `string`   | `""`                                  | Directory for temporary coverage profiles; defaults to a temp directory  |
| `-file`            | `string`   | `docs/development/testing/testing.md` | Testing documentation file to update                                     |
| `-include-e2e-run` | `bool`     | `false`                               | Also run the build-tagged E2E suite; requires a GitLab test environment  |
| `-skip-coverage`   | `bool`     | `false`                               | Skip the `go test` coverage run and keep the values already recorded     |
| `-timeout`         | `duration` | `30m`                                 | Per-package timeout handed to each `go test` run                         |
| `-top-tool-rows`   | `int`      | `25`                                  | Number of high-test-count tool sub-packages to show in the summary table |

#### Output

Rewrites the managed sections of `docs/development/testing/testing.md`.

#### Make targets

- `make gen-testing-docs` to regenerate, `make check-testing-docs` to verify.
  The check runs in `make audit-docs` and in the CI `Test` job, beside the
  other generated-artifact gates.

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

### gen_brand

Emits every vector brand asset from one parametric geometry, so the mark cannot drift between its surfaces. The mark is the "fan-out": a source node projecting three branch arcs, each ending in a node, which reads as a git graph and as the project's architecture (one canonical action catalog projected to three tool surfaces). The geometry lives in the command as constants; every emitter renders the same arcs at its own scale, so editing a curve edits every asset in the same run.

Outputs, relative to the repository root: `site/src/assets/logo.svg` (canonical classed mark, painted by the site's CSS tokens), `.github/brand/logo-mono.svg` (single-color `currentColor` variant), `site/public/favicon.svg` (self-contained colors on its own dark ground), `internal/toolutil/brandmark_gen.go` (the 24x24 `currentColor` MCP brand mark, as a Go constant), and the `.github/brand/banner.svg`, `og.svg` and `social.svg` cards.

#### Usage

```bash
# Write all assets
go run ./cmd/gen_brand/

# CI gate: byte-compare the committed assets against the geometry
go run ./cmd/gen_brand/ --check
```

#### Flags

| Flag      | Type   | Default | Description                                                       |
| --------- | ------ | ------- | ----------------------------------------------------------------- |
| `--check` | `bool` | `false` | Verify the committed assets match the geometry instead of writing |

#### Output

Rewrites the seven assets in place, or (with `--check`) names each stale one and exits `1`.

#### Make targets

- `make brand` — also the first generator `make update-all` runs, so a geometry change cannot leave the committed assets stale.
- `make brand-check` — CI gate.
- `make brand-rasters` — renders the raster derivatives (README banner WebP, OG and social PNGs, marketplace icons) from those vectors; maintainer-only, needs `rsvg-convert` and `cwebp`, and therefore stays out of `update-all`.

### gen_icon_webp

Rasterizes every `svg<Name>` constant in `internal/toolutil/icons.go` into two 16x16 lossless WebP files under `internal/toolutil/icons/webp/`, `<name>-light.webp` (near-black glyph) and `<name>-dark.webp` (near-white glyph), the `Theme`-tagged fallbacks served to MCP clients whose icon MIME allowlist admits `image/webp` but not SVG. It requires `rsvg-convert` (librsvg) and `cwebp` (libwebp) on `PATH` and refuses to start without them. Maintainer-only: the generated files are committed, so ordinary builds and CI never invoke it. Run it after adding or editing an icon.

#### Usage

```bash
go run ./cmd/gen_icon_webp/
go run ./cmd/gen_icon_webp/ --check
```

#### Flags

| Flag      | Type   | Default | Description                                                            |
| --------- | ------ | ------- | ---------------------------------------------------------------------- |
| `--check` | `bool` | `false` | Verify the committed WebP assets match `icons.go` without writing them |

#### Make targets

- `make gen-icon-webp`
- `make check-icon-webp` — same external-tool requirement, so it is not part of CI.

## Formatters

### format_md_tables

Normalizes Markdown pipe tables in `README.md`, `docs/` and `site/src/content/docs/` (or explicit positional paths).

#### Usage

```bash
# Format the default set (README.md, docs/ and site/src/content/docs/)
go run ./cmd/format_md_tables/

# CI gate
go run ./cmd/format_md_tables/ -check

# Format explicit paths
go run ./cmd/format_md_tables/ README.md docs/reference/tools/issues.md
```

#### Flags

| Flag     | Type     | Default | Description                                                                  |
| -------- | -------- | ------- | ---------------------------------------------------------------------------- |
| `-check` | `bool`   | `false` | Fail if any Markdown table needs formatting                                  |
| `-root`  | `string` | `.`     | Repository root containing `README.md`, `docs/` and `site/src/content/docs/` |

#### Positional arguments

| Argument                | Type       | Description                                                            |
| ----------------------- | ---------- | ---------------------------------------------------------------------- |
| (optional) `<paths...>` | positional | Explicit paths; defaults to `{README.md, docs, site/src/content/docs}` |

#### Output

Rewrites files in place unless `-check` is set, in which case it only verifies formatting.

#### Make targets

- Part of `make audit-docs` (with `-check`).
- `make analyze-fix` — applies fixes.

## Benchmarks

### bench_resources

Measures what the server costs to run, and draws the charts the documentation publishes. Everything else measured in this repository is about tokens and tool counts; none of it tells an operator how much memory to give a container, how long a client waits before the first tool call answers, or what a second credential adds to a shared deployment. This command answers those from the real binary, on both transports, and writes one record every downstream artifact is rendered from.

It needs nothing but a Go toolchain: GitLab is stood in for by an in-process HTTP server on loopback, and the OTLP collector by another, so a run is offline and a second machine measures the same thing rather than its own network. The tool surface is passed to the server explicitly and never read from the environment, for the reason given about generators: a developer machine exporting `GITLAB_MCP_TOOL_SURFACE` would otherwise publish different numbers than CI.

#### Usage

```bash
# Measure, then render
go run ./cmd/bench_resources/

# Redraw the charts and tables from the committed record
go run ./cmd/bench_resources/ -render

# CI gate: are the committed charts and tables current?
go run ./cmd/bench_resources/ -check

# Short smoke matrix, for verifying a change to this command
go run ./cmd/bench_resources/ -quick -json /tmp/x.json
```

#### Flags

| Flag               | Type       | Default                                                       | Description                                                                                                                                   |
| ------------------ | ---------- | ------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| `-binary`          | `string`   | `""`                                                          | Server binary to measure; empty builds `./cmd/server` into a temporary directory                                                              |
| `-json`            | `string`   | `site/src/data/resource-benchmark.json`                       | Measurement record to write, and to render from                                                                                               |
| `-doc-charts`      | `string`   | `docs/reference/benchmarks`                                   | Directory for the Markdown documentation's SVG charts                                                                                         |
| `-site-charts`     | `string`   | `site/public/benchmarks`                                      | Directory for the site's SVG charts                                                                                                           |
| `-doc-page`        | `string`   | `docs/reference/resource-benchmark.md`                        | Markdown page whose generated block is rewritten                                                                                              |
| `-site-page`       | `string`   | `site/src/content/docs/performance/resource-benchmark.mdx`    | English site page whose generated block is rewritten                                                                                          |
| `-site-page-es`    | `string`   | `site/src/content/docs/es/performance/resource-benchmark.mdx` | Spanish site page whose generated block is rewritten                                                                                          |
| `-scenarios`       | `string`   | `""`                                                          | Comma-separated scenario ids to measure; empty runs the whole matrix                                                                          |
| `-rounds`          | `int`      | `3`                                                           | Measured rounds per method                                                                                                                    |
| `-sample-interval` | `duration` | `100ms`                                                       | How often the resident set is sampled                                                                                                         |
| `-render`          | `bool`     | `false`                                                       | Skip measurement: redraw charts and tables from the committed record                                                                          |
| `-check`           | `bool`     | `false`                                                       | Verify the committed charts and tables match the committed record; implies `-render`                                                          |
| `-quick`           | `bool`     | `false`                                                       | Short smoke matrix, for verifying a change to this command                                                                                    |
| `-v`               | `bool`     | `false`                                                       | Print progress for every client and round                                                                                                     |
| `-clients`         | `string`   | `""`                                                          | Comma-separated credential counts for the concurrency series, ascending; empty uses `1,2,5,10,20,50,100,200,500,1000` (`1,2,5` with `-quick`) |
| `-step-duration`   | `duration` | `10s`                                                         | Steady phase per series step (`2s` with `-quick` unless given)                                                                                |
| `-memory-budget`   | `int`      | `0`                                                           | Resident set, in MiB, beyond which a series step is not started; `0` takes 80% of the host's available memory                                 |
| `-profiles`        | `string`   | `bench/profiles`                                              | Directory the series writes its CPU and heap profiles under; empty writes none                                                                |
| `-no-render`       | `bool`     | `false`                                                       | Measure and write the record and profiles, then stop: for a host with no repository to render into                                            |

A partial matrix (`-scenarios`, `-quick`, `-clients`) is refused unless `-json` names a record of its own, so it cannot overwrite the published one.

With `-no-render` and `-binary` the driver reads nothing from a checkout, so a prebuilt driver and server can measure the series on a host with no Go toolchain; the record is then copied back and rendered with `-render`. The server is started with `--pprof-addr` on a loopback port the driver picks, which is where the per-step profiles and goroutine counts come from.

#### Output

The measurement record, the SVG chart pairs under the two chart directories, and the generated blocks of the three documentation pages; and, for the series, one CPU and one heap profile per step under the profiles directory, which git ignores. A full run takes several minutes for the point scenarios, since every one builds a tool catalog per client, and then as long as the host's memory lets the series run.

#### Make targets

- `make bench-resources` — measure and render.
- `make bench-resources-render` — redraw from the committed record; what to run after changing a figure.
- `make check-bench-resources` — CI gate; seconds, since no benchmark is run.

## Evaluation

### eval_mcp_surfaces

Evaluates model behavior across MCP tool surfaces by running typed evaluation cases against the server in mock or live (Docker/self-hosted) mode. See [`cmd/eval_mcp_surfaces/README.md`](../../cmd/eval_mcp_surfaces/README.md) for the full guide, case formats, and run modes.

**Make targets:** the `make eval-surfaces-docker*` family (`eval-surfaces-docker`, `eval-surfaces-docker-enterprise`, `eval-surfaces-docker-enterprise-ce`, `eval-surfaces-docker-enterprise-all`, `eval-surfaces-docker-enterprise-all-fixtures`).

## Server

### server

The main `gitlab-mcp-server` MCP binary — the runtime entry point and the only `cmd/` binary that ships to users. See [CLI Reference](../reference/cli.md) for the full CLI reference and [configuration.md](../reference/configuration.md) for environment and configuration details.

**Make targets:** `make build` (builds `./dist/gitlab-mcp-server`), `make run` (builds and runs locally).

## CI gate targets

The following utilities expose a verification mode (`--check` or `-check`, or an invariant/error exit) that CI runs to guard against drift. The combined documentation gate is `make audit-docs`, which chains markdownlint, the table formatter, the llms, LobeHub-manifest, testing-docs and site-stats checks, the local-link check, the godoc, surface-quality and alias audits, and the site's own `check`, `build` and `lint`.

| Make target                              | Utility                            | What it gates                                                                                                              | Exit behavior                                             |
| ---------------------------------------- | ---------------------------------- | -------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------- |
| `check-action-catalog-manifest`          | `gen_action_catalog_manifest`      | Generated ActionSpec manifest is current                                                                                   | Non-zero if the manifest is stale                         |
| `check-llms`                             | `gen_llms`                         | `llms.txt` and `llms-full.txt` are current and structurally valid                                                          | Non-zero if either file is stale or malformed             |
| `check-lhm-manifest`                     | `gen_lhm_manifest`                 | `lhm.plugin.json` declares the registered tools, prompts, and resources                                                    | Non-zero if the manifest is stale                         |
| `check-footprint`                        | `audit_tokens -footprint`          | README token-footprint section, `docs/development/token-footprint.md` and `site/src/data/token-footprint.json` are current | Non-zero if any is stale                                  |
| `check-stats`                            | `gen_stats`                        | README repository-statistics section is current                                                                            | Non-zero if the section is stale                          |
| `audit-discovery-check`                  | `audit_discovery_completeness`     | No META-001 finding meets the configured severity threshold                                                                | Non-zero if any finding meets `-severity` (default error) |
| `audit-doc-coverage-check`               | `audit_doc_coverage`               | No `docs/reference/tools/*.md` has missing/orphan/tier_mismatch findings                                                   | Non-zero if any file has a finding                        |
| `audit-godocs-check`                     | `godoc_tool audit`                 | No package, symbol, or test Godoc findings remain                                                                          | Non-zero when findings are present                        |
| `audit-dynamic-aliases`                  | `audit_dynamic_aliases`            | No error-severity alias governance finding (collisions, ambiguity)                                                         | Non-zero (`1`) if any error-severity finding exists       |
| `audit-docs` → `format_md_tables -check` | `format_md_tables`                 | All Markdown pipe tables are normalized                                                                                    | Non-zero if any table needs formatting                    |
| `check-testing-docs`                     | `gen_testing_docs`                 | The `docs/development/testing/testing.md` test-metrics block is current                                                    | Non-zero if the generated section is stale                |
| `check-supply-chain`                     | `audit_supply_chain`               | The five release-configuration invariants still hold                                                                       | Non-zero if any is broken, or if the audit cannot be run  |
| `check-doc-tool-names`                   | `audit_doc_tool_names`             | Every `gitlab_*` name the documentation mentions is registered on some surface                                             | Non-zero if any name is unregistered                      |
| `check-gateway-chars`                    | `audit_gateway_chars`              | Nothing served carries a character a gateway validator rejects                                                             | Non-zero if any offender is served                        |
| `check-install-buttons`                  | `audit_install_buttons`            | Every install button decodes and agrees with the others for its command                                                    | Non-zero on a problem, or when no button is found         |
| `check-test-goroutines`                  | `audit_test_goroutines`            | No `testing.T` abort is made off the test goroutine                                                                        | Non-zero if any abort site exists                         |
| `check-test-subtests`                    | `audit_test_subtests`              | No case loop asserts without a `t.Run` subtest                                                                             | Non-zero if any site remains                              |
| `check-test-file-names`                  | `audit_test_names -check-files`    | Every `_test.go` is named after a module it tests                                                                          | Non-zero on a violation                                   |
| `check-md-escaping`                      | `audit_md_escaping -check`         | No value reaches a Markdown table cell, heading, list item or link unescaped                                               | Non-zero on a finding or a directive that excuses nothing |
| `check-site-stats`                       | `audit_metrics -site-stats -check` | `site/src/data/stats.json` is current                                                                                      | Non-zero if the file is stale                             |
| `check-bench-resources`                  | `bench_resources -check`           | The committed benchmark charts and tables match the committed record                                                       | Non-zero if they are stale                                |
| `brand-check`                            | `gen_brand --check`                | The committed brand assets match the geometry                                                                              | Non-zero on drift                                         |
| `check-icon-webp`                        | `gen_icon_webp --check`            | The committed WebP icons match `icons.go` (needs `rsvg-convert` and `cwebp`, so not run in CI)                             | Non-zero on drift                                         |
| `check-readonly-graphql`                 | `audit_readonly_graphql`           | No action classified ReadOnly can reach a GraphQL mutation                                                                 | Non-zero on any finding, or if the audit cannot be run    |
| `check-graphql-schema`                   | `gen_graphql_schema --check`       | The committed GitLab schema parses and its provenance record decodes                                                       | Non-zero if either file is missing or unusable            |
| `check-graphql-documents`                | `audit_graphql_documents`          | Every raw GraphQL document in the source is one the pinned GitLab schema accepts                                           | Non-zero on any refusal, or if no documents are found     |
