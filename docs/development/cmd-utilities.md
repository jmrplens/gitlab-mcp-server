# Command-Line Utilities Reference

> **Diátaxis type**: Reference · **Audience**: 🛠️ Contributors & maintainers

The `cmd/` directory contains the developer tooling binaries that power audits, code generation, formatting, and the documentation pipeline for this project. They are **not** part of the runtime MCP server, with one exception: `cmd/server` is the server entry point itself.

Every utility can be run directly with `go run ./cmd/<name>/ [flags]`, or through the convenience Make targets documented per section. Several binaries also expose a `--check` (or `-check`) mode that validates generated output without writing it; these are wired into CI gates (see [CI gate targets](#ci-gate-targets)).

## Quick reference

| Utility                        | Category                      | Purpose                                                                                                                                                                                                                                                                                                                                                   | Make target                                                                                                                                                             |
| ------------------------------ | ----------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `audit_1to1`                   | SDK/API parity audits         | Consolidated SDK↔API parity audit (struct/action/metadata gap streams, plus the `sdk` service and raw-GraphQL gate, and `paths` for the request a handler builds: the endpoint, the shapes GitLab sends and whether a list says where it ends)                                                                                                            | `make audit-1to1`                                                                                                                                                       |
| `audit_catalog_first`          | Catalog & metadata audits     | Source-discovered ActionSpec catalog-first coverage inventory                                                                                                                                                                                                                                                                                             | `make audit-catalog-first`                                                                                                                                              |
| `audit_discovery_completeness` | Catalog & metadata audits     | Extended META-001 model-discovery metadata quality auditor                                                                                                                                                                                                                                                                                                | `make audit-discovery`                                                                                                                                                  |
| `audit_doc_coverage`           | Catalog & metadata audits     | Per-doc-file gaps vs the action catalog (DOC-002)                                                                                                                                                                                                                                                                                                         | `make audit-doc-coverage`                                                                                                                                               |
| `audit_action_ids`             | Catalog & metadata audits     | Every canonical action ID the server publishes to a model, in a cross-link, a hint, a usage line or a description, or quoted back in a substring the e2e suite asserts a served text carries, is one the catalog has, and the served prose names no tool                                                                                                  | `make audit-action-ids`, `make check-action-ids`                                                                                                                        |
| `audit_doc_tool_names`         | Catalog & metadata audits     | Every `gitlab_*` tool name and every `domain.action` ID the documentation mentions is one the server serves                                                                                                                                                                                                                                               | `make check-doc-tool-names`                                                                                                                                             |
| `audit_dynamic_aliases`        | Catalog & metadata audits     | Dynamic-toolset alias governance (collisions, ambiguity)                                                                                                                                                                                                                                                                                                  | `make audit-dynamic-aliases`                                                                                                                                            |
| `audit_e2e_coverage`           | Catalog & metadata audits     | What the e2e suite covered, from the calls it recorded and the actions the server dispatched: every runtime x surface x mode x action classified, the levels L1 to L3, the non-tool capabilities; `-static` is the push gate over typed action ids, and `-record`/`-check-record`/`-check-record-page` commit the per-runtime summary and gate it offline | `make audit-e2e-coverage`, `make audit-e2e-gaps`, `make check-e2e-static`, `make e2e-coverage-record`, `make check-e2e-coverage-record`, `make check-e2e-coverage-page` |
| `audit_edition_tier`           | Catalog & metadata audits     | Doc-grounded licensing tier (Free/Premium/Ultimate) vs binary gating                                                                                                                                                                                                                                                                                      | `make audit-edition-tier`                                                                                                                                               |
| `audit_graphql_documents`      | Catalog & metadata audits     | Every raw GraphQL document in the source is one the pinned GitLab schema accepts; `-live` judges by what an instance serves now and reports the drift under our own documents                                                                                                                                                                             | `make check-graphql-documents`, `make check-graphql-documents-live`                                                                                                     |
| `audit_graphql_shapes`         | Catalog & metadata audits     | Every struct a GraphQL response is decoded into can hold what its document selects and declares nothing the document never selects; `-report` writes the reverse, what the schema offers there and no document of the decoding package selects                                                                                                            | `make check-graphql-shapes`, `make audit-graphql-shapes`, `make audit-graphql-sent`                                                                                     |
| `audit_dead_consts`            | Catalog & metadata audits     | Every unexported constant in `internal/` and `cmd/` is one something reads, which `staticcheck`'s `unused` cannot answer for a member of a const group                                                                                                                                                                                                    | `make check-dead-consts`                                                                                                                                                |
| `audit_readonly_graphql`       | Catalog & metadata audits     | No action classified ReadOnly can reach a GraphQL mutation                                                                                                                                                                                                                                                                                                | `make check-readonly-graphql`                                                                                                                                           |
| `audit_surface_quality`        | Surface quality audits        | Consolidated MCP tool surface quality audit (metadata + output); `-check` gates on every rule that reads the served surface, the edition tier a description states and the constant index a list formatter reads included                                                                                                                                 | `make audit-surface-quality`, `make check-surface-quality`                                                                                                              |
| `audit_gateway_chars`          | Surface quality audits        | Served descriptions and titles carry no character an MCP gateway validator rejects                                                                                                                                                                                                                                                                        | `make check-gateway-chars`                                                                                                                                              |
| `audit_meta_descriptions`      | Surface quality audits        | Every parameter and value a served meta-tool description offers is one its actions accept, whether the value set is a schema enum or one a schema description spells                                                                                                                                                                                      | `make check-meta-descriptions`                                                                                                                                          |
| `audit_tokens`                 | Surface quality audits        | LLM context-window overhead of every tool/resource/prompt definition; `-footprint` regenerates the README token-footprint section                                                                                                                                                                                                                         | `make audit-tokens`, `make gen-footprint`                                                                                                                               |
| `audit_metrics`                | Surface quality audits        | Comprehensive metrics summary (tools, resources, prompts, codebase); `-site-stats` writes the site's stats JSON                                                                                                                                                                                                                                           | `make audit-metrics`, `make gen-site-stats`                                                                                                                             |
| `gen_graphql_schema`           | Generators                    | Pins a GitLab GraphQL schema by introspecting a live instance; `--check` gates the committed one                                                                                                                                                                                                                                                          | `make gen-graphql-schema`, `make check-graphql-schema`                                                                                                                  |
| `gen_api_live`                 | Generators                    | Boots a released GitLab image and asks the loaded application what its own REST API is, entity by entity and route by route; `-check` gates the committed record                                                                                                                                                                                          | `make gen-api-live`, `make check-api-live`                                                                                                                              |
| `godoc_tool`                   | Source quality audits         | Godoc compliance auditor and fixer (audit + fix subcommands)                                                                                                                                                                                                                                                                                              | `make audit-godocs`                                                                                                                                                     |
| `audit_test_names`             | Source quality audits         | Classifies `Test*` functions by naming pattern; emits rename hints; `-check-files` gates test-file naming                                                                                                                                                                                                                                                 | `make audit-test-names`, `make check-test-file-names`                                                                                                                   |
| `audit_test_goroutines`        | Source quality audits         | `testing.T` aborts made off the test goroutine                                                                                                                                                                                                                                                                                                            | `make check-test-goroutines`                                                                                                                                            |
| `audit_test_subtests`          | Source quality audits         | Case loops that assert without a `t.Run` subtest; `-fix` rewrites the unambiguous ones                                                                                                                                                                                                                                                                    | `make check-test-subtests`                                                                                                                                              |
| `audit_md_escaping`            | Source quality audits         | Values a Markdown formatter interpolates into a table cell, heading, list item or link without an escaping helper, or into a code fence it wrote by hand; staged, card rows written by hand and flags or timestamps printed without their helpers                                                                                                         | `make check-md-escaping`                                                                                                                                                |
| `audit_sdk_context`            | Source quality audits         | Every call into client-go in `internal/` and `cmd/` hands the SDK the caller's context, which it takes only as the `gl.WithContext` request option and which neither `noctx` nor `contextcheck` can see                                                                                                                                                   | `make check-sdk-context`                                                                                                                                                |
| `audit_supply_chain`           | Release & supply-chain audits | Five release-configuration invariants: pinned actions, credentialed jobs that run no run-time-resolved code, stated Dependabot cooldowns, a current security policy, signature-verifying installers                                                                                                                                                       | `make check-supply-chain`                                                                                                                                               |
| `audit_install_buttons`        | Release & supply-chain audits | Decodes every one-click install button and holds the buttons to one configuration per command                                                                                                                                                                                                                                                             | `make check-install-buttons`                                                                                                                                            |
| `gen_action_catalog_manifest`  | Generators                    | Generates the ActionSpec group-builder manifest                                                                                                                                                                                                                                                                                                           | `make gen-action-catalog-manifest`                                                                                                                                      |
| `gen_lhm_manifest`             | Generators                    | Generates the tools/prompts/resources arrays in `lhm.plugin.json` (LobeHub Marketplace)                                                                                                                                                                                                                                                                   | `make gen-lhm-manifest`                                                                                                                                                 |
| `gen_llms`                     | Generators                    | Generates `llms.txt` and `llms-full.txt`                                                                                                                                                                                                                                                                                                                  | `make gen-llms`                                                                                                                                                         |
| `gen_request_inventory`        | Generators                    | Merges the requests the unit suite records into `docs/development/request-inventory.json`                                                                                                                                                                                                                                                                 | `make gen-request-inventory`                                                                                                                                            |
| `gen_stats`                    | Generators                    | Regenerates the managed repository statistics section in `README.md`                                                                                                                                                                                                                                                                                      | `make gen-stats`                                                                                                                                                        |
| `gen_testing_docs`             | Generators                    | Regenerates the test-metrics block in `docs/development/testing/testing.md`                                                                                                                                                                                                                                                                               | `make gen-testing-docs`                                                                                                                                                 |
| `gen_brand`                    | Generators                    | Emits every vector brand asset from one parametric geometry                                                                                                                                                                                                                                                                                               | `make brand`, `make brand-check`                                                                                                                                        |
| `gen_icon_webp`                | Generators                    | Rasterizes the SVG icons into light/dark WebP fallbacks (maintainer-only)                                                                                                                                                                                                                                                                                 | `make gen-icon-webp`                                                                                                                                                    |
| `format_md_tables`             | Formatters                    | Normalizes Markdown pipe tables in `README.md`, `docs/` and `site/src/content/docs/`                                                                                                                                                                                                                                                                      | part of `make audit-docs`                                                                                                                                               |
| `bench_resources`              | Benchmarks                    | Measures what the server costs to run (memory, startup, a second credential), draws the published charts, and measures whether a bound leaves a quiet tenant better off                                                                                                                                                                                   | `make bench-resources`, `make bench-fairness`                                                                                                                           |
| `gen_model_corpus`             | Evaluation                    | Renders the model evaluation corpus breadth ledger: what the corpus asks about, counted against the action catalog                                                                                                                                                                                                                                        | `make gen-model-corpus`, `make check-model-corpus`                                                                                                                      |
| `gen_model_results`            | Evaluation                    | Folds a run's observation shards into the committed record, scores them and redraws the published pages and README tables                                                                                                                                                                                                                                 | `make gen-model-results`, `make model-results-record`, `make model-results-refold`, `make check-model-results`                                                          |
| `server`                       | Server                        | The main `gitlab-mcp-server` MCP binary (runtime entry point)                                                                                                                                                                                                                                                                                             | `make build`, `make run`                                                                                                                                                |

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

#### Request paths (R-PATH)

`-scope=paths`. Every other rule describes the surface: the fields we accept, the fields we return, the actions we register, the discovery metadata we attach, the enum values we advertise. All of them compare what we publish against what the SDK and the API documentation offer, and none of them looks at the request a handler builds. That is how nine registered tools shipped while being unable to work: a perfect input struct, a perfect output struct, a registered action, complete enums, and a request GitLab refuses.

This rule reads the request instead, out of `docs/development/request-inventory.json`, the inventory `internal/testutil` records and [gen_request_inventory](#gen_request_inventory) commits. Seven checks. Five of them read the inventory as a list of endpoints and parameter names, which is what the recording keeps of a request; the last two read **values**, because names alone leave two whole classes invisible.

- **Has the path ever been observed.** An action whose owning package issued no request at all has never had its request seen by anything, which is exactly the state the broken documents were in. Held at package grain, because nothing on the wire names an action, and the report says so beside the number (`actions_observed_grain`): 990 of 1082 observed means 990 actions whose owning package issued some request, not 990 actions whose own request anybody has seen. As a regression guard it is real; as per-action assurance it is nothing. A package may nevertheless be silent for a reason, and `internal/tools/adminspecs` is the whole of it today: it declares specs whose handlers live in other packages, so its requests are recorded under the package that makes them. Such a package is held to a declaration with a category and a reason in `declaredSilentOwners` (`cmd/audit_1to1/internal/paths/declarations.go`), the way `-scope=sdk` holds a client-go service to one, and a declaration that no longer describes the tree is itself a finding. An action whose owner names no package fails too: nothing validates that field, and such an action used to be classified unmapped, which the gate ignored and `-gaps-only` dropped, so it could be neither counted nor seen. The owner `tools` is the catalog's own exception, naming the orchestration package rather than a domain under it, and resolves to `internal/tools`; an owner that names a real package and the wrong one is a lie no version of this check can catch.

  Where an end-to-end run has recorded its calls, the same question is asked one grain finer (`e2e_observation`, `-e2e-calls <dir>`, `make audit-1to1-paths-e2e`). The unit recording cannot name an action, because the `httptest` server answers on its own goroutine with the calling test out of reach; a Docker run can, because the harness stamps a trace id into each MCP call, the server's span carries it back with the route the dispatcher actually chose, and every GitLab request the handler made is a child of that span. The receiver counts those children per trace and the harness writes the count on the `dispatch` line it already emits (`e2ecalls.Dispatch.Requests`), so the report can say, per action, which were seen issuing a request (`actions_issuing_requests`) and which ran and issued none (`actions_that_ran_and_issued_nothing`). It reports and never gates. The shards are a byproduct of a run CI does not schedule and never commits, so failing on their absence would fail every push; and the counts are floors, because the batching span processor drops silently when its queue overflows, which makes a positive claim solid and a negative one a lead. A dispatch carrying a refusal reason is kept off the lead list: the server declining to run something is not a handler that could not build a request.
- **Does the document validate.** Every raw GraphQL document in the source, against the pinned schema. The reading and the judging are `cmd/internal/graphqldocs`', shared with the standalone gate [audit_graphql_documents](#audit_graphql_documents), so there is one answer to whether a document is one GitLab would refuse. This judges the document and never the values sent with it, which is the other half of the same defect family: of the nine tools that could not work, four sent a document the schema refuses and five sent an accepted document carrying a value GitLab does not have. The second half is caught by the validating transport in `internal/testutil`, which checks the variables with the document on every request a test drives, and neither check substitutes for the other.

  The other half of the server's GraphQL surface is judged beside it (`sdk_graphql`). client-go builds 42 documents inside its own module, for the achievement, work item, saved view, security attribute, security category, scan profile, target branch rule and Terraform state services, and every one of them reaches GitLab through this server while being judged by nothing except whichever of them a unit test happens to drive. `graphqldocs.SDKDocuments` points the same type-checking walk at the module directory `structs.CollectOutputPairings` already resolves, which is what folds a document assembled from a shared field constant into the string GitLab receives; it needs no GitLab and no network, only `GOWORK=off` and `-mod=readonly`, since client-go ships a `go.work` naming a directory its module zip does not carry and a module cache is read-only. Six of the 42 are counted apart rather than judged, and named with their positions: the work item documents are `text/template` shells and the Terraform state queries are printf format strings, so what the type checker folds carries a placeholder where a value belongs, no schema can judge it, and a refusal list with permanent entries in it is a list a reader learns to skip. The remaining 36 the pinned schema accepts. It reports and never gates, for a reason unlike the usual one about an incomplete oracle: the pin is what GitLab serves, so a refusal there is real, but the answer is an upstream merge request and a version bump rather than an edit here, and that is not a state to fail a build on.
- **Does the endpoint exist.** `-check-endpoints` compares every recorded REST endpoint with GitLab's own API documentation, and **fails on an endpoint no declaration accounts for**. The declarations are what make that safe, because the oracle is prose: 57 documentation lines omit the leading slash, `emoji_reactions.md` gives the note reactions one plaintext block for issues and leaves the merge request and snippet variants to a sentence, `usage_data.md` documents `/usage_data/track_events` only in prose and a curl example, `attestations.md` writes its endpoint lines without the `projects` scope its own curl example shows, the generic package registry and Terraform state are documented outside `doc/api` entirely, and the `/services/` alias for the integrations endpoints, which client-go still uses, is not documented at all. Each of those shapes is written down in `declaredUndocumentedEndpoints` (`cmd/audit_1to1/internal/paths/endpoint_declarations.go`) with a category and a reason, and a declaration that stops matching anything is a finding of its own, so the excuse cannot outlive the thing it excuses. The listing of pages comes from the repository tree rather than `api_resources.md`, because 101 of the 253 pages under `doc/api` are not linked from that index or from `rest/_index.md`, and following every link out of the pages they do list still leaves 77 unreached. Against the full 247 readable pages the comparison finds 82 undocumented endpoints out of 1378 recorded, all of them declared; against the index alone it would report roughly five times as many, nearly all about where the index stops. A hole in the corpus produces candidates that are only about the hole, so the report counts the pages it could not read.

- **Does GitLab say it sends what we publish.** The fourth check reads `docs/development/gitlab-api-live.json`, the record [gen_api_live](#gen_api_live) takes from a booted GitLab. It is the only rule here whose oracle is GitLab rather than client-go or a documentation page: the other five compare us against what the SDK models, which is a second model of the API and not the API. It reports and never gates, and it asks its question at two grains, kept side by side so a reader can see which findings the sharper one keeps. It replaced two records that were readings of text, GitLab's generated OpenAPI document and a scan of the Grape source beside it, and the reason is what the epics domain cost: a generated document lists a conditional expose as a plain property, so `subscribed` and `reference` were published on the strength of a record that could not see the `if:` that gates them. An instance has already evaluated the entity, so the field and its condition come from one reading and cannot disagree.

  At **package grain** (`shapes.unpublished`) it joins the inventory and reports every `*Output` field under `internal/tools` that no endpoint its package was recorded calling declares in a response. The inventory names a package, so the fields of every endpoint a package calls are unioned before the comparison, which makes the check exact for a package with one endpoint and weaker as the package grows; an operation the document gives no response schema for (553 of the 1847) contributes nothing, silences its package rather than condemning it, and is left out of the `endpoints_searched` a finding carries, which counts searched responses and means the same thing at both grains; and a nested output type is left out entirely here, since a nested type is only comparable against the object it sits under, which the type grain does and this grain cannot. Every one of those choices loses findings and none of them invents one. It reports 635 fields across 130 packages, and most of them are not phantoms: `ListOutput.users` and `SSHKeyListOutput.keys` are our own wrappers around a JSON array the document describes by its element; `DeleteOutput.deleted` and `userNotFoundOutput.identifier` are our own answers to a 204 and to a not-found, which no endpoint sends because they are not an endpoint's response; and `files.Output` is reported for its 12 fields only because the endpoint behind it declares no response schema while some of the other 29 endpoints `files` calls do, so the union is non-empty and the package is not silenced. The join itself is reported (`shapes.join`) because a package whose rows all miss would otherwise have every field of its output condemned by a lookup failure, and the literal path segments our own fixtures left untemplated are reported beside it (`shapes.untemplated`), which measures the recorder rather than the server: 894 of 1479 REST rows match exactly, 504 more only once a fixture value such as `mygroup` or `myproject` is accepted where GitLab has a placeholder, and 81 match nothing.

  At **type grain** (`shapes.typed`) it compares a type against the response of the operations that type actually models, and only those. The chain touches the inventory nowhere, which is the point, since the inventory records a package by construction and cannot be sharpened: `structs.CollectOutputPairings` gives the client-go struct a converter fills each output type from, out of the same pairing pass the R-OUTPUT field diff runs over; `readSDKRoutes` parses the SDK source the handlers compile against and returns, per struct, the endpoints the service methods answering with it reach, reading the `route()` templates and the `withMethod` options and reproducing client-go's own template normalisation so the two spellings meet; and the document supplies the response. Of 441 top-level output types it compares 26 and reports 11 fields. The other 415 are counted rather than judged: 401 that no converter pairs with a client-go struct, which is where the wrappers and the synthetic results go; those whose struct no service method answers with, so there is no endpoint to ask about, a count kept though it reads zero today; and 14 whose every route the document either does not carry or carries with no response schema, an empty union meaning the document does not say rather than that GitLab sends nothing. A type's published fields are the names encoding/json would write, an untagged embed's promoted into it, and a type is nested when any struct of its package names it as a tagged field's type, output type or not; an embed names nothing nested, since its fields are promoted rather than placed under a key, and the embedded type stays a response of its own (`shapes.typed.nested_unpublished` therefore never holds a type reached through an embed). The one package a domain package takes a shape from, `internal/toolutil`, is read once and resolved wherever a package names one of its shapes as a field's type, embeds one, or aliases one under its own name (`type NoteOutput = toolutil.NoteOutput`), the hints type aside since its next steps are the server's and not GitLab's; an alias is nested when the shape is named under either name or by another shared shape, unless an exported function of the package returns it, which is what a handler does with the note it adds to a discussion. Those rules were learned from the first list this produced: a details type embedding the row type was reported missing every field it promoted, and the user under the row of a list of uploads, reached through a plain struct, was held to the endpoints answering with a whole user. A match here is exact only: the loose lookup the package grain leans on accepts a literal segment of ours where GitLab has a placeholder, which is evidence about a fixture value in the inventory and would be a guess against a route template, whose placeholders are already placeholders. The phantom of [issue 580](https://github.com/jmrplens/gitlab-mcp-server/issues/580) is the case the join was built against and the fixture that proves it: `mrapprovals.ConfigOutput` as it stood before the fix produces exactly the twenty findings the fix removed, and as it stands now produces none.

  Since schema version 2 of the record, the same grain also asks about **nested output types** (`shapes.typed.nested_unpublished`): a type reached through a field of a compared type is held against the properties the document gives that field, one level down and no further. A nested type whose property the record describes no object for is not compared at all, which is the same reticence that skips a type with no response schema and is what keeps this level usable: 21 nested types compared, 25 fields reported. None of the 25 carries a declaration yet, and `summary.typed_undeclared_fields` spans both levels, so it reads 25 today; the three top-level findings are answered. They sit in `internal/tools/issuelinks` and `internal/tools/pipelinetriggers`, and adjudicating one means reading its API page first. The two discussion packages were here too, beside the 8 top-level fields the shared note shape published that GitLab's note does not carry: the notes review removed those and read the four the shape lacked from the captured response (ADR-0021), which is what a finding at this grain is for. One level is deliberate. The record grew from 1.1 MB to 1.5 MB carrying it, which is still a diff a reviewer reads, and each further level multiplies that by the branching of GitLab's schemas rather than adding to it.

  A finding at type grain can be **answered rather than fixed**, because the oracle is generated and is not always complete: an endpoint that renders a bare hash gets no schema worth comparing against, and a nested property can be given a narrower entity than the endpoint renders. `cmd/audit_1to1/internal/paths/shape_declarations.go` is where such a finding is written down with a category and the evidence, on the terms every other declaration table in this audit works on: a declaration that matches nothing is itself reported as stale. Today it holds one entry, for `invites.InviteResultOutput`, whose POST GitLab answers with `{"status": "success"}` while the generated document carries the pending-invitation object of the GET at the same path.

  The same two grains ask the **reverse question** as well, since a record that speaks for GitLab can say what GitLab sends that we do not publish. `shapes.sent.unsurfaced` (package grain) and `shapes.typed.unsurfaced` (type grain) list every response field the searched endpoints send that the package, or the type, does not publish, and each finding carries what gates it: `sent` is `always` for a field the entity exposes with no condition and `when` for one behind an `if:` or `unless:`, recorded beside it with the license tier the condition's feature symbols resolve to and `ee` when the condition was written under `ee/`. The entity a field is read on is the one named by the first endpoint carrying that field, per field rather than per type or per package, because the responses of one type's endpoints resolve to different entities: the fingerprint lookup of a key is annotated as answering with a user, and reading the key's own fields on that entity answered for the wrong object. The summary carries the counts at each grain (`unsurfaced_fields`, `unsurfaced_sent_always`, `unsurfaced_sent_when` and their `typed_` twins). Neither grain gates: a field GitLab sends that this server does not surface is a candidate for the 1:1 surface, and the type-grain list, which names the output type and the endpoints it models, is what the field-by-field review reads. What a package publishes, for this direction, is the json name of every exported struct of the package that is not an input, the inner ones included, since the row of a list is nested under the list and is exactly what the list endpoint sends; the other direction judges the top-level types alone, for the reason given above. The package grain over-reports in one known way, since a package that calls an endpoint for something other than surfacing its answer (`health` reads `/user` to learn who the token is; `projectdiscovery` reads `/projects/{id}` to resolve a path) is reported as missing that answer's every field.

  A finding here can be **answered rather than surfaced** too, on the terms of the other direction: GitLab's generated document lists, for some operations, the response the operation's description names rather than the one its handler presents, and a field of that response is a defect of the document rather than a gap in the surface. `cmd/audit_1to1/internal/paths/sent_declarations.go` records such an answer with a category and the source that says so, keyed by the component the finding was read on, since a finding carries its operations per type and its component per field: the fingerprint lookup of a key (`GET /keys`) is described as answering with `UserWithAdmin` and `lib/api/keys.rb` presents a key, so the user fields the document lists at the top level are declared, while the key fields the same type lacks are not; and the add-a-member `POST /invitations` is described as answering with an `Invitation` and returns the `status` and `message` pair, the same thing the shape declaration for the other direction records. A declared finding keeps its `sent` answer and gains a `category` and a `reason`, the summary counts them apart (`unsurfaced_declared`, `typed_unsurfaced_declared`), and a declaration that accounts for no finding is reported stale and fails the gate like every other declaration table here.

- **Does a list say where it ends (R-PAGE).** The fifth check, and the one that looks at a part of GitLab's answer no other rule in this repository can see. Pagination is a field of no entity: an offset page arrives in the `X-Page`, `X-Next-Page`, `X-Per-Page`, `X-Total` and `X-Total-Pages` response headers and a keyset page in a `Link` header, so every rule that compares a published field against client-go's struct, a documentation page or the entity record is looking in the one place the answer is not. All six of them were green on `internal/tools/impersonationtokens`, which answers `user.list_impersonation_tokens` with a bare array of tokens while GitLab serves twenty of them at a time: the caller cannot tell it has one page, cannot ask for the second, and is not told either.

  The oracle is the live record's own params. 308 of its 2110 mounted routes declare `per_page`; 304 of those declare `page` beside it, which is GitLab's offset pagination, and the other four take a `cursor` or a `page_token` instead, which is keyset and whose answer is a cursor rather than a page number. The two are counted apart (`paginated_routes`, `keyset_routes`) because a page-and-total block is the wrong shape to publish for a keyset endpoint, and a keyset endpoint therefore raises no finding here; none of the four is reached by a recorded request today.

  An action is judged when its output type is a **collection envelope**: exactly one content field, and that field a list of objects, with this server's own framing (the hints block, a pagination block) taken out before the count. That strictness is where "the route declares the params but the action reads a single object" is answered, structurally rather than one declaration at a time — a project carrying `shared_with_groups` or a job carrying `artifacts` is a single-object read whose caller was never promised the list, and admitting those turned the finding list from 24 into 79, of which 55 were about a list nobody asked to page. Of 268 collection-reading actions, 224 already publish a pagination block and 20 sit in a package no paginated endpoint was recorded for, which this check says nothing about.

  The join from an action to an endpoint is the **package**, because the inventory records the package that built the client and nothing on the wire names an action; `pagination_endpoint_grain` says so beside the number, the way `actions_observed_grain` does for the observation check. The cost is measurable and is paid in declarations: of the 24 findings, 14 name an action whose own GitLab route declares `per_page` and 10 matched an endpoint belonging to a sibling action of the same package. Each of those 10 is written down in `declaredUnpaginatedCollections` (`cmd/audit_1to1/internal/paths/pagination_declarations.go`) with the route the record holds for it and what its params are — `GET /projects/:id/issues/:issue_iid/participants` and `GET /projects/:id/merge_requests/:merge_request_iid/reviewers` declare neither param, `GET /projects/:id/languages` answers with an object mapping a language to its share rather than an array at all, and `project.target_branch_rule_list` is read through GraphQL so the record holds no REST route for it. The table is deliberately not a place to record a paginated endpoint we have decided not to publish pagination for: that is the finding.

  It **reports and does not gate**, because a finding is a surface change and the findings are not uniform: `request_paginates` on each says whether the action already offers `page` and `per_page`, so an output-only edit is told from one that has to widen the input first. Its declaration table gates, on the terms every declaration table here is held to: a claim that has stopped matching anything is reported stale and fails the run.

- **Is a param sent that the caller never asked to send.** The sixth check, and the first of the two that read values. The recorder writes down that a body carried `package_name_pattern` and nothing about what was in it, so a field this server sends as `null` on every call reads exactly like one a caller filled in, and every dimension of this audit was green on `package.protection_rule_update` while it did precisely that.

  Two records already here answer it between them. client-go's own source says which option struct a service method is given and which json keys `encoding/json` writes whatever the handler set (no `omitempty` and no `omitzero`), read through embedded option structs (whose fields are promoted) and through nested ones (named the way Grape declares a nested param, so `PositionOptions.PositionType` is `position[position_type]` and the members of a list are tried under `actions[][action]` as well as `actions[action]`). The live record says which params GitLab declares on the route receiving them and which of those it requires. A key written unconditionally whose param GitLab **requires** is correct; one whose param GitLab lets a caller **leave out** is a value nobody can decline to send.

  That line is sharp rather than heuristic, and it was measured before it was written: over the 231 option structs the 243 recorded body-carrying endpoints reach, it separates **10 findings from 34 benign always-sent fields** and 2 params the route declares nothing about, with no declaration table doing any of the work. The 10 include the case it was built against (`UpdatePackageProtectionRulesOptions` writes `package_name_pattern` and `package_type` on a PATCH where GitLab marks both optional, while the POST beside it requires them and is right to send them always) and `CreateIssueLinkOptions.LinkType`, `CreateDependencyListExportOptions.ExportType`, `CreateGroupIssueBoardListOptions.LabelID` and the `expires_at` of the three member and share option structs.

  Two blind spots are stated rather than absorbed. A GET is never judged, since client-go encodes its options as a query string where a nil pointer is left out whatever the json tag says. And a field whose type is a plain struct carrying `omitempty` is still written on every call, because under v1 semantics a struct is never empty; client-go has no such field today, since its nullable values are `Nullable[T]`, which is a map and which `omitempty` does omit, but an SDK change could open that silently. The join is the endpoint rather than the action, so a route two option structs reach is asked about under both and a finding can name a struct the recorded package never passes; the endpoint is on every finding, so a reader can see which. It **reports and does not gate**: the tag belongs to client-go, so almost every fix is an upstream merge request rather than an edit here, and whether GitLab minds a null in a given position is per endpoint.

- **Was the path ever built out of anything but the fixture.** The seventh, and the one that had to be paid for in the recorder. Templating `/projects/1/issues` into `/projects/:project_id/issues` is what makes a row an endpoint, and it is also what hides a hard-coded identifier: with one fixture value, a handler that reads the caller's project and a handler with that id written into it produce the same row. That is why the class had to be found by hand, in `runnercontrollers`, `snippetdiscussions`, `repository`, `runnercontrollerscopes` and `securityscanprofiles`.

  The recorder now counts, per row and per placeholder, how many distinct raw values it templated away. The count is running rather than final, because the recorder is process-global with no shutdown hook to write a total from: a new value costs one more line carrying the higher count, and the merge takes the highest of a row's lines (the highest rather than the sum, since the lines are a running total of one set; and a row belongs to one package, which the compiler builds into one test binary, so two shards carrying one row means one package ran twice over the same fixtures). The values themselves never leave the test process, on the same terms the query and body names do: a value is a fixture and a count is a property of the suite's reach.

  A placeholder seen with exactly one distinct value is a **lead**, never a verdict: a fixture that makes one project and drives forty endpoints against it produces forty innocent ones. The leads are printed sharpest first, a row where another placeholder did vary being the shape a hard-coded identifier makes, and capped at 50 with the true count beside them, since a list nobody can read is a list nobody reads. It **reports, and is meant to keep reporting**: what would make it gateable is fixtures that use two identifiers wherever they can, which is a change to hundreds of tests rather than to this rule. There is deliberately no cheap static form of the question either, since a lint over numeric literals in handler source would fire on every legitimate constant and one over test files would fire on every fixture in the tree.

The first two checks need no network and no suite run: the inventory is committed, the schema is pinned, and the catalog is compiled in. That is what makes them a CI gate. The fourth, fifth, sixth and seventh need no network either, since the API record is committed like the inventory, and they run with them; none of them contributes a gate outcome beyond its declaration table, and the seventh has none to contribute. Its type grain does cost the typed load of `./internal/tools/...` that the other scopes already pay for, memoized per root and so free to a run that has done it, and it reads the client-go source out of the module cache the build resolved rather than fetching one. The third needs 250 pages over the network, so it runs only when asked for, and nothing schedules it today: a declaration going stale is noticed the next time somebody runs `make audit-1to1-paths-endpoints`.

Report keys: `schema_version`, `inventory`, a `summary` block (50 counters, `actions_observed_grain` and `pagination_endpoint_grain` among them), `graphql_refusals[]`, `silent_owners[]`, `stale_declarations[]`, an `endpoints` block that says whether the documentation comparison ran, how many pages it read, which it could not, the endpoints no page spells out with the declaration that accounts for each, and the declarations that accounted for none, a `shapes` block carrying the join quality, the untemplated segments most frequent first, the package-grain unpublished fields, and a `typed` block with the type-grain counts and findings, a `pagination` block carrying what the record says GitLab pages, how much of that the inventory reached, how the collection-reading actions divide, and `unpaginated[]`, an `sdk_graphql` block with the documents client-go builds, the template shells it could not judge and the refusals, an `always_sent` block carrying how many option structs and endpoints were walked, how the always-sent fields divide between required, undeclared and optional, and `optional_but_always_sent[]`, an `identifier_values` block that says whether the inventory carries identifier counts at all and holds the capped lead list with the true count beside it, and an `e2e_observation` block that is empty unless `-e2e-calls` named a shard directory. Every shape finding of either grain carries a `grain` key naming the join that produced it, and a type-grain one also names the client-go struct and the operations searched; every pagination finding carries the paginated endpoints of its package as its evidence. With `-gaps-only` `silent_owners[]` holds the undeclared and the unmapped ones, `endpoints.undocumented[]` holds the undeclared ones, `pagination.unpaginated[]` drops the declared ones, and the two report-only blocks drop their context (`sdk_graphql.template_documents` and `e2e_observation.actions_issuing_requests`) and keep their work; `shapes` is unaffected, because every entry in it is already a finding.

#### Make targets

- `make audit-1to1` — writes `plan/1to1-backlog.json`, then runs `audit-1to1-sdk`.
- `make audit-1to1-sdk` — the SDK parity gate, enum values included; fails the build on a finding.
- `make audit-1to1-enums` — the enum value rule alone; fails the build on a finding.
- `make audit-1to1-paths` — the request-path gate (R-PATH); fails the build on a finding. No network.
- `make audit-1to1-paths-endpoints` — the same plus the documentation comparison, written to `plan/1to1-paths.json`; fails on an endpoint no declaration accounts for. Needs the network.
- `make audit-1to1-paths-e2e` — the same plus the observation question asked per action, read from the shards a Docker end-to-end run left under `dist/e2e-calls`, written to `plan/1to1-paths-e2e.json`. Gates on nothing the plain target does not. No network.
- `make audit-1to1-validate-docs` — validates the doc/api citations (CI gate).
- `make audit-struct-completeness` — legacy wrapper running `-scope=structs`.
- `make audit-action-coverage` — legacy wrapper running `-scope=actions`.
- `make audit-metadata-completeness` — legacy wrapper running `-scope=metadata`.

#### Notes

This single binary replaces four former binaries. The legacy Make targets remain as thin `-scope` wrappers for backward compatibility. `-validate-docs` uses the shared `cmd/internal/apidocs` fetcher (cache in `.cache/gitlab-api-docs/`, 7-day TTL) — the same source-of-truth docs as `audit_edition_tier`.

## Catalog & metadata audits

### audit_catalog_first

Generates the source-discovered inventory of ActionSpec catalog-first coverage. It reports `RegisterTools`/`RegisterMeta`/`ActionSpecs` presence, surface classification, and dynamic-catalog counts, plus catalog-first invariant checks.

It also holds every exported `ActionSpecs` under `internal/tools` to something that aggregates it. That rule is separate from the inventory and reads the tree differently: the inventory sees a function of that name and treats its presence as health, while this asks whether anything calls it, resolved through the type checker (`cmd/internal/goprogram`) rather than by a text scan. A package whose specs nothing aggregates publishes usage lines, aliases, tags and parameter guidance that no surface serves, so a maintainer editing the obvious file changes nothing a model reads and gets a green build for it. The join cannot be `OwnerPackage`: an admin action's owner is the domain package its handler lives in, so a package can own catalog actions while its own `ActionSpecs` reaches nothing. A package deliberately in that state is declared in `cmd/audit_catalog_first/declarations.go`, where a declaration matching nothing is itself a finding.

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
- `make analyze` (step 18)

#### Notes

There is no `--check` mode; instead, the catalog-first invariants return a non-nil error on regression, which is what fails the build.

### audit_discovery_completeness

Extended META-001 auditor for model-discovery metadata quality. It checks action-level gaps (`weak_aliases`, `generic_usage`, `empty_related`, `weak_individual_description`, `missing_next_steps`), field-level gaps (`empty_output_description`, `param_enum_candidate`, `empty_param_description`, `missing_parameter_guidance`), and the sibling-cluster gap `missing_disambiguation`. It applies cluster-aware severity escalation for non-CRUD action families.

`missing_parameter_guidance` asks a question that can fire. Until 3.1.0 it asked whether a spec carrying a scope-suggestive parameter had any `ParameterGuidance` at all, and it reported zero across the whole catalog and always would: `tools.CollectActionSpecs` runs `toolutil.FillScopeParameterGuidanceSingle` over every spec before the auditor reads one, and that fill adds a default entry for exactly the names the check looked for. It now flags an action whose guidance is no richer than that central fill while it requires an identifier the fill does not cover (`topic_id`, `hook_id`, `agent_id`, `merge_request_iid`), and the per-field breakdown names each such parameter. `name`, `key` and `slug` are left out because nothing in a schema separates the ones that identify an object from the ones that carry content the caller invents. Asked this way it names 270 actions across 52 packages, so it is classified `info` like `param_enum_candidate`: visible in `-gaps-only`, counted in neither `errors` nor `warnings`, and due a promotion once the backlog is drained.

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

Checks every name the documentation teaches a reader to call against what the server really serves: the `gitlab_*` tool names against the names it registers, and the `domain.action` IDs against the catalog it builds. `audit_doc_coverage` asks which actions are documented rather than whether the documented ones exist, so a page can name a tool no surface has ever registered and still audit clean; that is how a verb-first spelling of the issue list survived in guides while the individual surface projects `gitlab_issue_list`, and every copy-pasted example answered `unknown tool`. The name set is built in memory from the same registration paths the server uses, across the individual, meta and dynamic surfaces at the Ultimate tier, so it needs no network and cannot drift from the catalog.

**The ID rule exists because the tool rule alone left the other half of every such sentence unjudged.** A page teaching a call names the tool on the individual surface and the canonical ID on the dynamic one, and the tool regex cannot see an ID at all: it matches `gitlab_[a-z0-9_]+` and an ID carries no such prefix. Eight IDs across five pages were wrong that way, among them a whole `pipeline_schedule.` family the CI/CD page asserted in both languages while the catalog spells it `pipeline.schedule_*`; each of those pages paired the wrong ID with the right tool name, so this command was green over all of them. What counts as an ID is `cmd/internal/actionids`, shared with `audit_action_ids`, so a spelling cannot be a cross-link in the code and prose in the docs.

Three shapes pass the dotted-token test and are not IDs, and only the last is a table of instances: a file name whose stem is a catalog domain (`issue.rb`, `pipeline.svg`), judged by its last segment, since a page writes file names and a file name's tail is not an action; a meta-surface manifest entry, whose left half the tool rule already checks (`gitlab_merge_request.create` is what `gitlab://tools/{id}` publishes on that surface); and the declarations in `cmd/audit_doc_tool_names/ids.go`, which are telemetry and protocol attributes (`user.id`, `resources.subscribe`), paths into a document or a data file (`stats.tools`, `result.content`) and names belonging to somebody else's software (`gotest.tools`). Without the file-name class rule the report was 75 tokens of which about fifty were `issue.rb` and `project.svg`; enumerating those one by one would have filled the table with entries a reader learns to skip.

The roots scanned are `docs/`, `site/src/content/docs/`, `README.md`, `llms-install.md`, `CLAUDE.md` and `npm/gitlab-mcp-server/README.md`; the npm launcher's README is in the list because it is published to a registry, where a wrong name is not fixable without republishing a version. Tokens that look like tool names but are not (the evaluator's bridge tools, for example) are listed in the source with the reason for each exemption.

#### Usage

```bash
# Report
go run ./cmd/audit_doc_tool_names/

# CI gate
go run ./cmd/audit_doc_tool_names/ --check
```

#### Flags

| Flag     | Type   | Default | Description                                                                 |
| -------- | ------ | ------- | --------------------------------------------------------------------------- |
| `-check` | `bool` | `false` | Exit non-zero when the docs name a tool or an action ID that does not exist |

#### Output

The number of registered names, of catalog action IDs and of documentation files scanned, then each unregistered tool name with the files that mention it, then each action ID the catalog does not publish, with the nearest one it does and, when the token is a registered alias, what it stands for. Exits `1` under `-check` when any is found, and `1` whenever the catalog cannot be built or the documentation tree cannot be scanned.

#### Make targets

- `make audit-doc-tool-names` — the report.
- `make check-doc-tool-names` — CI gate.

### audit_action_ids

Holds every canonical action ID this repository publishes to a model against the IDs the catalog really builds. `audit_doc_tool_names` asks the same question of the documentation; this asks it of the server's own output, where a wrong ID is worse: an ID that resolves to nothing answers `unknown action` the moment a model follows it, and the model concludes the capability is missing rather than that the cross-link is wrong.

Three kinds of string reach a model as an ID it is invited to call next, and all three are read and gated: the `RelatedActions` list of an `ActionSpec`, which the dynamic find and execute results carry; the first argument of `toolutil.HintAction`, which a Markdown formatter writes into the result; and a dotted ID spelled inside a `Usage` line or an individual tool's `Description`. A fourth kind is the prose the server serves around them, and a fifth the e2e suite's quotation of it: see the two rules below. The source is loaded through `cmd/internal/goprogram` and constants are folded by the type checker rather than matched as text, which is the whole reason for the loader: the IDs are written as package-local constants, about fifty packages keep them in a metadata table with a lowercase `related` field copied onto the options, one keeps its table as a map to an anonymous struct, several hand the list in as a parameter, and two build an ID by concatenating a domain constant onto a name. A scan over literals reports the bare prefix as a finding and passes the folded value in silence, which is how a regex over `runnercontrollertokens` produced five phantoms that were never there. A value the walk cannot fold is named in an unresolved bucket rather than passed over, since a site the audit could not see must not be reported clean. A call that only hands back lists recorded where they are written is passed over rather than followed into: a copy of a `RelatedActions` list, the narrowing the dynamic registry applies before publishing one, and the merge `ActionRoute.WithRelatedActions` makes of the route's list and a list parameter named as related, whose callers are judged where they write it. The three share one hole, stated rather than absorbed: an ID such a body adds of its own is not read. A call handed related-named list parameters alone is followed into, which is where a helper appending an ID to what it was handed writes it.

The IDs are judged against `cmd/internal/actionids`, the reader `audit_doc_tool_names` shares so that one spelling cannot be a cross-link here and prose there: the catalog this tree builds at the Ultimate tier, twice, once against a self-managed stub instance and once against GitLab.com, with the union as the oracle. Orbit registers only for GitLab.com, so a single self-managed build reports its six IDs as phantoms and a fixer deletes six working cross-links. The standalone dynamic actions are added the way `cmd/server` adds them, because `gitlab_execute_action` takes those IDs too.

`-check` refuses four things about a published ID, and the first two are the rule itself. The two prose rules below add their findings to them, and the suite's rule adds its helper table.

A published ID that resolves to nothing is a cross-link a model cannot follow. A published ID that resolves **only as a registered alias** is refused as well, which is the demand this gate exists to make: not that the ID resolve, but that it be the canonical one. `gitlab_execute_action` resolves an alias, so such a link does work when it is followed, and that is exactly what kept the class invisible; `gitlab_find_action` publishes canonical IDs, so a model that looks the name up in a listing does not find it. The sixty spellings that were sitting in that bucket were individual tool names in a `RelatedActions` list (`gitlab_runner_get` for `runner.get`), which is a third naming scheme in a field documented as carrying catalog IDs, and demanding mere resolvability would have left every one of them in place.

A declaration that excuses nothing is a finding, on the terms every declaration table here is held to. And a site the type checker could not fold is a finding too: it is the audit's own blind spot rather than a defect of the tree, and it fails anyway, because a gate whose blind spot is silent is one any future site can step into: an ID assembled at run time would be reported unreadable and pass, which is the shape every wrong ID would then take. The remedy is to spell the ID as a constant, which every site in the tree does today. That rule caught its first case immediately: the layer that turned the gate on rewrote the two generated runner reset-token entries to take their siblings as a struct, and the two IDs inside it stopped being foldable.

The limit worth knowing before a clean run is read for more than it is: it answers whether an ID **resolves**, never whether it is the **right** ID. The catalog has both `snippet.get` and `snippet.project_get`, so a project-snippet action cross-linked to the first is silent here.

A dotted token in prose is only taken as an ID when one of its halves is one the catalog uses, which turns away `github.com`, `gitlab.com` and every `params.note_id` an example binding writes. The tokens that pass that test and are still not IDs are declared in `cmd/audit_action_ids/declarations.go` with a reason each, in two tables: one for a token that is not an action ID at all, and one for the prose that names a **registered alias on purpose**, which is the single shape the canonical-ID demand would be wrong for (the `gitlab_issue_update` usage line exists to tell a model that dynamic execute accepts `issue.close`). Only a prose site consults the second, so a cross-link naming one of those is still a finding. Both are judged only by a run over the whole tree: over one package every entry excuses nothing, and reporting them all stale would be an answer about the patterns rather than about the declarations.

#### The rule over served prose

A model reads the prose a handler hands it exactly as it reads the rest, and nothing judged it until issue 869. A hint saying "verify project_id with `gitlab_project_get`" names a tool the default dynamic surface does not register at all, and on meta only the bare domain tools exist, so the name is right for one surface of three. The canonical ID is the portable form here for the reason it is the portable form in a documentation example: it does not depend on `GITLAB_MCP_TOOL_SURFACE`.

The rule began with the hint argument of `toolutil.WrapErrWithHint`, `WrapErrWithStatusHint` and `NotFoundResult` and the struct fields such a hint is written into on its way to one, since nineteen domains reach those helpers through a field of their own output. Issue 910 widened it to the rest of what a model reads, because the class had simply moved: 162 of its first findings were in error messages, most of them telling a model to "Use `gitlab_project_list` to find the ID first", and 60 in a result's next steps. The sinks are a table keyed by the function's full name (`proseSinks` in `source.go`), each with the argument its prose starts at and the kind it is counted under:

| Kind                 | What it reads                                                                                                                                                                                                                                                 |
| -------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `error_hint`         | The hint of `WrapErrWithHint`, `WrapErrWithStatusHint` and `NotFoundResult`                                                                                                                                                                                   |
| `hint_field`         | A field named for a hint, `NextSteps` and `NextStep` included, wherever it is written                                                                                                                                                                         |
| `message`            | The argument of `errors.New`, `toolutil.ErrorResult` and `toolutil.CancelledResult`, a whole `fmt.Errorf`, and a field named for a message (`Message`, `missingProjectMsg`, `EmptyMessage`) where what is written folds                                       |
| `next_step`          | The hints of `toolutil.WriteHints`, `WriteListFooter` and `Card.End`                                                                                                                                                                                          |
| `param_guidance`     | `ParameterGuidance.ValueSource` and `CommonConfusions`, which every surface serves beside an action's schema                                                                                                                                                  |
| `schema_description` | The `jsonschema` tag of every struct field in the served load, with the `,required` marker the schema builder strips stripped here too, and the `description` entry of a schema written as a map (an input schema override, a hand-built schema)              |
| `usage`              | A spec's `Usage` line, judged here for tool names only: its dotted IDs are the published-ID section's, where `declaredAliasMentions` excuses the two aliases the `issue.update` line names on purpose, and judging them here too would count a wrong ID twice |
| `description`        | An individual tool's constant `Description`, judged here for tool names everywhere but its `See also:` clause, the one part the resource manifests rewrite per surface; its dotted IDs are the published-ID section's, as a `Usage` line's are                |

`WriteListFooter` and `Card.End` are sinks of their own although both forward to `WriteHints`, because the forwarding is a body and a run narrowed to one domain loads `toolutil` from export data, where there is no body to follow. `ErrorResultAnnotated` is deliberately not one, because nothing reaches it that a sink here does not already read or that is a sentence of the server's: most callers hand it Markdown another sink wrote, `CancelledResult` hands it a declined confirmation's refusal, which is read as that function's own argument, and the rate-limit refusal hands it a sentence built around the name of the tool the caller called, a value it reports. A sink's own prose parameters are never followed back out, since the sink's visit already reads every call of it; `hintedError` hands `WrapErrWithHint`'s hint to a format and `NotFoundResult` hands its hints to `Card.End`, and following those reached every caller's argument a second time under the other kind, which kind won depending on the package the walk met first. A domain action's individual tool `Description`, where it is a constant, is read for tool names everywhere but its `See also:` clause (`description` in the table above): `gitlab://tools` serves it verbatim as the description of that action's entry on the dynamic and meta surfaces as well, and rewrites only the clause into each surface's names, so the clause is the one part where an individual tool name is right. Until issue 910's third review it was not read at all, on the premise that only its tool serves it, and eleven descriptions named a tool outside the clause ("Reversible via `gitlab_unban_user`"); three of the eleven are assembled at run time and were rewritten by hand. The standalone surface tools are read whole: the guided flows (`interactive.issue_create`, `interactive.mr_create`, `interactive.project_create`, `interactive.release_create`) and `discover_project.resolve` are registered as tools on meta and on individual alike, and the dynamic surface runs them by ID and serves their description as their `Usage`, so the one text reaches every surface. Each writes it as its `Usage` and as its tool's `Description` both, which is where this rule reads it (`TestStandaloneToolSpecs_EverySurface_ServesOneTextNamingNoTool` in `internal/tools/surfaces` holds the two equal), and names every action by canonical ID, its `See also:` clause included. Until issue 910's second review they wrote the text as the `Description` alone, which `actioncatalog` projected as the `Usage`; the rule read neither, and all five named tools two surfaces do not register (`gitlab_issue (action='create')`, `gitlab_issue_create`, `gitlab_search_projects`).

A format is folded rather than reported. `fmt.Errorf` and `fmt.Sprintf`, at a site and inside a one-line helper, fold to the format as written, verbs and all, followed by each constant argument; the verbs are masked with a space only where the value is judged, so a verb followed by a dot and an action name does not read as a dotted ID and a verb run into a tool name (`%sgitlab_project_list`) does not hide it, while the fixer still finds the literal inside the value. An argument named for a hint is followed to what it is handed, and so is a parameter named for a message where the site's kind follows one, a helper handing on the sentence each caller wrote, while a local or a field so named is GitLab's message spelled into the server's sentence (`glMsg`). No helper in the tree spells such a parameter into a format today (`files`' `missingProjectMsg` and `projects`' `missingUserMsg` go to `errors.New`), so the shape is the one the test fixture pins, `requireProject(op, missingProjectMsg)`. A read of a field this walk records is a copy judged where it was written, and a sink call is read by its own visit, which is what keeps `fmt.Errorf("...: %w", fmt.Errorf("..."))` from passing the inner sentence over. A `Usage` line's format alone follows its other calls too: a call of a function declared in the load is read at every branch it returns from, which is how `badges` assembles its twelve lines, `fmt.Sprintf("%s Use %s ... %s", badgeActionDescription(verb, scope), ..., badgeScopeBoundary(scope))`, and both of `badgeScopeBoundary`'s sentences named a meta tool while the call was passed over as a value the line reports. A hint's or a message's calls stay values, since there they are what the sentence reports (`err.Error()`, a joined list, an escaped field). Every other argument is a value the sentence reports, an ID, a path, GitLab's own message or the error it wraps, and is **passed over and counted** (`values_passed_over`) rather than listed: with some four hundred formats in the served tree the list would be GitLab data from end to end. Inside a one-line helper such a value is passed over **without** being counted, since the fold reads the helper's body once per call and the value has no site of its own to count at; `dynamic`'s `queryTooLongMessage` is that shape. A message field written from another struct's field (`Message: c.Message`) is passed over on the same terms. That is the rule's one deliberate exception to naming its blind spots, and the figure is what says the walk reads less than it did. The format fold is for prose only: a helper that builds an ID with `fmt.Sprintf("%s.%s", ...)` stays a site nothing folds, since judged as its format it would be the ID `%s.%s`.

The carriers are the ones the tree writes. A parameter is followed out to its callers when its name says it carries this kind of prose: a hint's name for every kind, a message's (`missingMsg`, `emptyMessage`) for a message, a guidance field's own name for guidance, which is how `toolutil.DiscussionIDParamGuidance(valueSource)` is read in the domain that wrote the sentence, a name ending in `description` for a schema description (`branches`' `branchProtectionAccessLevelSchema`), and one ending in `usage` for a `Usage` line. The value variable of a range over a list of strings is followed to the list, read as one. A helper handed a recorded field read, alone or beside such parameters, is a copy or a merge of prose recorded where it was written, with the hole those shapes carry: a sentence its body adds is not read. A helper handed such parameters alone merges nothing recorded, so it is followed into as well, which is where one appending a sentence of its own to what it was handed writes it; `WriteListFooter`'s `withoutPreserveLinks(hints)` is that shape, read through the filter's range over what it was handed. A call of a function value handed them alone is listed with the sites nothing folds, and one into another module (`strings.TrimSpace(hint)`) adds no sentence of this repository's. A helper that returns a list of hints is followed into, whatever branch it returns from, which is how a formatter's `jobHints(j)` is read.

A `Usage` line assembled at run time is folded the way a hint is: its literal halves are kept, a local is followed to what it is given, a format to its format and constant arguments, and a helper that picks the line by the action's name to every branch it returns from (`ffuserlists`' `userListUsage`), which is the one kind whose calls are followed into that way. A hint's helpers build their sentence from values or escape it, and following those read `toolutil`'s escaping as ten sites nothing folds where the call had been one. A read of another `Usage` field is a copy of a line recorded where it was written (`runners`' `options.Usage = meta.usage`). Such a line was passed over in silence until issue 910's review, on the grounds that it carried no literal ID a reader could get wrong, which stopped being true when the tool-name rule began reading the same line. What still folds nowhere in it is counted with the served prose nothing folds and never with the published IDs, since it is a sentence a reader can read rather than an ID nobody can.

Three spellings are reported: a `gitlab_*` tool name, a registered alias, and a dotted ID that resolves nowhere. Two tables excuse a tool-shaped token: `hintToolExemptions` for one that names no tool (the `gitlab_ci_ymls` template family), anywhere, and `declaredSurfaceToolMentions` for a tool the package's own surface registers, which today is `gitlab_find_action` and `gitlab_execute_action` in `internal/tools/dynamic` alone, whose every sentence one of those two tools returns. A schema description of `internal/tools/dynamic` may also name one of the declared aliases, because the one that does is dynamic execute's action parameter, whose subject is that execute accepts one; that use keeps the declaration alive, since the declaration names it as a reason, and a schema description anywhere else naming an alias is refused. A hint a domain hands to a `toolutil` helper under a hint-named parameter (the `listHint` and `detailHint` of `NewTemplateRenderer`, the hints of `NewDiscussionRenderer`, the hint of `ExecGraphQLDestroyNote`) is followed out to the domain from the helper's own signature, which is why `./internal/toolutil` is part of the default load although it declares no action: the walk follows a value only into a package it loaded.

It **gates**, and both widenings reached zero before they did. The first whole-tree run of the hint rule reported 785 findings across 137 packages, every one a tool name, and `-fix-hints` closed 712 of them mechanically. The served-prose run reported 328 in 69 packages over 9671 sentences, 323 of them tool names and five dotted IDs that resolve nowhere. `-fix-hints` rewrote the mechanical majority and the rest were rewritten by hand, all in the change below the one that widened the rule, so the gate turned on green: the 17 meta spellings (`gitlab_project action 'list'`) the catalog cannot map to an action, the 18 schema descriptions a tag holds as one literal with the field's json name, and the five IDs. A stale entry of either table fails the run, as every declaration table here does; it used to be reported only, which made the hint table the one exception.

The fixer is worth knowing about, because the naive version of it is wrong in two ways that both look right. It rewrites a string literal only when the literal's text is part of a sentence the walk folded, which keeps an individual tool's `Description` out: the same token is correct there, in the same file and often the same declaration block. And it refuses a literal with no space in it, because that is a name rather than a sentence: without that rule the first run renamed the tools themselves, rewriting 48 lines of `internal/tools/projects/action_specs.go`. A token either table declares is neither rewritten nor listed as left behind. `-fix-hints-tests` moves an assertion with the sentence it pins, and admits a test literal on a rule of its own: a piece of a folded sentence, as in production, or a literal that, read with its tool names rewritten, holds a whole folded sentence. The second is the rendered bullet (a test's `"- Use gitlab_project_list to ...\n"`) or the whole card a formatter's test compares, which contains the hint rather than being contained in it, and because it is judged against the text the literal will have, it works on a tree whose production prose was rewritten in an earlier run.

Its own blind spots are counted beside its findings and do not fail, which is a deliberate departure from how the gate treats the unfoldable sites of the other three rules. A published ID that cannot be read is an ID nobody can check; an unfoldable sentence is still one a reader reads, and the twenty-three in the tree build one from a helper that branches (`levelHint`, `searchNextStep`), a map read (`groupserviceaccounts`' per-action `leads`), a call into another module (`accesstokens`' operation phrase, whose last branch spells the action's name with its underscores replaced) or a parameter no rule follows (`notifications`' `updateUsage(head, levels, flags)`), or read one back out of rendered text (`toolutil`'s safe-mode preview parser). Five of them came with the review of issue 910, which read the run-time `Usage` lines and the map schemas the rule used to pass in silence, and one with its second review, which follows a `Usage` format's helper calls to every branch. A sentence concatenated from a literal and a value is folded to its literal halves, and the half it leaves unfolded is read on its own: a name is followed to the values it is handed, where a tool name is judged, and anything else is counted with the sites nothing folds. Keeping only the literal half used to count the sentence as read whole, and `awardemoji` handed its three note deletes a list tool's name through exactly that shape (`"list awards with " + listToolHint`), which no run could see until the half was read.

Its limits are stated rather than absorbed. A package-level map of prose read through a local and joined (`mergeStatusHints` in `mergerequests`) is not read, since neither the map nor the local carries a hint's name, and it was rewritten by hand. `internal/prompts` and `internal/resources` serve prose too and are outside the load, which reads `./internal/tools/...` and `./internal/toolutil`: the review prompt's actions are held to the catalog by a test of its own, `TestReviewMR_NamesActionsByCanonicalID`, and the resource manifests label each entry for the surface serving it and rewrite a domain action's `See also:` clause into that surface's names. A standalone surface tool's clause is rewritten on the dynamic surface alone, where its canonical IDs map to themselves: the meta and individual surfaces register the guided flows and project discovery beside a catalog that does not carry them, so their manifests serve those descriptions verbatim, canonical IDs included, which `gitlab://tools/{id}` resolves. No gate reads the names the manifests' own prose spells: until issue 910's second review the `tool_detail` template's description gave, as the individual entry for `project.get`, a verb-first spelling that no surface registers and no alias resolves, where the entry is `gitlab_project_get`. The operation label a wrapper prefixes an error with is not read either, though some 104 calls under `internal/` spell it as a tool name (`WrapErrWithStatusHint("gitlab_list_group_iterations", err, http.StatusNotFound, ...)`, and `ErrRequiredString("gitlab_get_project_statistics", "project_id")`, whose message starts with the label); it names the operation that failed rather than inviting a call. A value a format reports is passed over and counted, and inside a one-line helper passed over uncounted, as the format paragraph above says. A bare meta action name (`Use action 'list'`, `Use action 'file_list'` in `packages`) is not read at all: it carries neither the `gitlab_` prefix the tool-name rule matches nor the dot the ID rule matches, and it resolves only on the meta surface, where `action` is an argument of the domain tool. The class is 65 lines in 12 packages under `internal/tools`, counted as the non-test lines matching `action '<name>'` with no dot in the name, less the two that are comments (`clusteragents`' `action_ids.go`, `mrapprovals`' `markdown.go`) and the two in `mrchanges` whose name is a `%s` verb filled with a canonical ID: `containerregistry` 13, `packages` 13, `snippets` 12, `issuediscussions` 6, `epicnotes`, `labeldata` and `snippetnotes` 4 each, `accesstokens`, `civariables`, `groupvariables` and `instancevariables` 2 each, and `geo` 1. `internal/toolutil`, which the rule loads too, adds two more that every project, group and instance variable detail result serves: the update and delete lines of the shared card `FormatCICDVariableDetailMarkdown`. A rule for it needs the domain a sentence belongs to, which the sentence does not spell, so it is fixed by rewriting to `toolutil.HintAction` with the canonical ID, a package at a time. Two holes are the price of shapes the walk accepts: a helper handed a recorded field read beside a carrier is a merge, and a sentence its body adds is not read; and a domain action's individual tool `Description` assembled at run time is read by no rule, since both rules read a `Description` only where it is a constant (a standalone surface tool's is read, as its `Usage`). Those are held by a test instead, `TestToolManifest_ServedDescriptions_NameNoToolOutsideTheirSeeAlsoClause` in `internal/resources`, which reads every `Description` the built catalog carries, Ultimate for a self-managed instance and for GitLab.com with the standalone tools added, and refuses a tool name outside its `See also:` clause whatever assembled it: the switches over the action name in `deploykeys` and `deploytokens` and the map in `pages` among them. The clause is one pattern, `actioncatalog.SeeAlsoClause`, which the manifests rewrite and this rule and that test pass over, so the part skipped cannot drift from the part rewritten. The `description` of a schema map in `internal/resources` (`tool_manifest.go`) is outside the load with the rest of that package.

The dotted-ID half was never large: the five unresolvable IDs the hint rule's first run found were one constant in `internal/tools/workitemsavedviews` that spelled its list action under a `work_item_saved_view` domain, where the catalog registers those actions as routes on the issue domain and the ID is `issue.work_item_saved_view_list`. The remedy for the class is to spell the ID through the constant the catalog registers rather than writing it out.

#### The suite that quotes it

The e2e suite is the one corpus that reads the server's hints back to it, and until issue 902 nothing held it to the catalog. Issue 901 is what that cost: twenty assertion literals still named a tool after issue 883 had moved the hints they quoted to canonical IDs, and the 53 subtests they failed were found by the licensed run, which happens on tags, a month later. A grep for tool names in the suite is the wrong rule, because a tool name there is usually right: `harness.Raw` names one on purpose, and the manifest, mode, exclusion and annotation scenarios look tools up by name. What made the twenty wrong was the **position** they sat in.

So a fifth kind of site is read: the substring a helper asserts a served text carries. The helpers are declared in `servedTextAssertions` (`cmd/audit_action_ids/suite.go`), each with the parameter its substring is passed in, and that parameter is found by name in the callee's own signature: the `substrings` of `assertMentions` and `mentionsAny`, the `needles` of `containsAny`, and the `contains` of `harness.ExpectToolError`, whose options after it are left alone. A helper matches by name, and only when the function a call resolves to is declared under `test/e2e/`, so the two copies of `assertMentions` in `common` and `ee` are one entry and a function of the same name elsewhere is none. The suite is loaded from `./test/e2e/gitlab/...` in a second load with the test variants and the `e2e` tag, which the command states itself as `audit_e2e_coverage -static` does, so neither the Makefile nor CI passes a flag. A suite wrapper that takes the needles under one of the declared names, or under a hint's, and hands them to a helper in a position that asserts (an assertion, or a predicate under `!`) is followed out to its callers.

The two predicates are judged only where the call is negated. `if !mentionsAny(...)` fails the test when none of the needles is there, so each needle is a claim about what the server wrote; `if containsAny(...)` is an absence check or a classification, and the exclusion scenario, which asserts that a refusal does **not** echo the excluded tool's name, has to spell exactly that name there.

A needle is held to the three spellings a hint is, a tool name, a registered alias and a dotted ID nothing resolves, and the findings gate in a section of their own (`e2e_assertions` in the work list), so a reader can tell a defect of the server from a defect of its test. It consults the hint rule's exemption tables without keeping any entry alive, since those describe the served source, and one table more: `declaredAliasMentions`, because a `Usage` line may name one of those two aliases by design and a test quoting that line quotes it faithfully. A needle the type checker cannot fold, built from a fixture's name at run time or read off a test table's field, is counted and listed under `-v` and fails nothing, on the hint rule's terms, and so is the half of a concatenated needle that no name answers for: a needle written as a literal plus a value keeps its literal halves, and the value is followed to what it is given where it is a name, so a tool name concatenated into a needle is judged too. The whole-tree run over today's suite reads 109 helper calls and 152 needles, folds every one of them and finds nothing to refuse.

The helper table is held to the suite on the terms every declaration table here is. Any run names a copy of a helper that takes no parameter of the declared name, with the package that declares it, and a run over the whole suite names an entry nothing calls, which is what a renamed helper looks like from here; both fail the gate, because either would stop every call of that helper being read without a word. The two are fixed in different places, and each row says which. An entry nothing calls is the table's to fix. A parameter that differs is the copy's: one entry names the parameter of every copy of its helper, `assertMentions` in `common` and in `ee` alike, so no edit of the table can agree with two copies that disagree. The judgement about an entry nothing calls needs the whole suite, so it is made only by a run over the whole suite: the run with no arguments, one naming `./test/e2e/gitlab/...` itself, and one naming a wildcard that encloses it, `./...` or `./test/...`, which brings the whole suite into the suite load. The suite patterns are compared with `./test/e2e/gitlab/...` once those a wildcard of the same list encloses are set aside, read slash-separated, so `.\test\e2e\gitlab\...` on Windows is the same run: a suite package named beside the suite or beside such a wildcard is one the suite already encloses and leaves the run whole, while one outside `./test/e2e/gitlab/...`, such as the harness, makes it a run over more than the suite. The patterns are compared and not the packages they load, so naming the suite's three packages one by one, which loads the same packages, is not judged whole. Any other run says it did not judge the table whole, in its report and as `helpers_judged: false` in the work list, since an empty stale list from it says nothing about the entries its packages do not call. A pattern given as an absolute path below the repository root, or as an import path below this module (`github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/gitlab/...`), is read as the relative pattern it names before it is sorted, and that is the pattern the load is handed; both spellings used to go to the served load whatever they named, which read the suite's three packages as a `doc.go` each and reported them clean. A wildcard pattern that encloses the suite, `./...` or `./test/...`, goes to the served load as given and brings the whole suite (`./test/e2e/gitlab/...`) into the suite load besides, since sorted by its prefix alone it produced that same clean run. A relative pattern keeps its leading `./`: without it `go list` reads `test/e2e/gitlab/...` as an import path, which matches no package of this module, and the run is refused with `no packages matched` before anything is judged. A run naming `./test/e2e/gitlab/ee` reads that package alone and no served source, so it says the published-ID and served-prose rules were not run (`served_judged` is false in the work list) rather than printing their counts over nothing, and a run naming `./internal/tools/...` reads no suite at all and prints no section for it.

The limits are the table's. A helper under a name it does not declare is not read until it is declared, and neither is one called through a function value, nor a wrapper that takes the needles under another name, which is reported as a needle nothing folds. A copy of a declared helper may take its needles as a variadic tail or as a `[]string` of its own, and both are read element by element; the second used to be read as one needle that folded to nothing, which fails nothing. A bare `strings.Contains` on a served text is not read at all, by design: the suite calls it on its own values as often as on the server's, and on the server's for needles that name no tool and no action, a status code (`403`) or one of the server's fixed phrases (`unknown action`, `confirm=true`). So the rule `test/e2e/README.md` states is narrower than "never": a quotation that names a tool, an alias or an action ID goes through the helpers, and one written as a bare call is not read. The polarity is syntactic, so `ok := mentionsAny(...); if !ok` is not judged, and neither is a wrapper that returns a predicate's answer, negated or not (`func has(...) bool { return mentionsAny(...) }`, `func lacks(...) bool { return !mentionsAny(...) }`): the first's inner call is not negated, the second's negation is part of what the return hands back and is passed over for that reason, and neither's callers are followed, since whether the needles are claims is decided at each caller, negated or not, and following them all would judge an absence check as a claim. A negation inside a function literal a return hands back is still read, since that body runs where it is called, unless that body returns it in turn, which makes the literal such a wrapper itself. Nothing names such a wrapper, so it is a limit the suite keeps by writing none. A dotted needle is judged as the whole ID it spells, so one that is only the front of a longer ID in a domain the catalog uses is refused although it matches at run time; the remedy, quoting the whole ID, asserts strictly more.

The first thing it made visible was on the server's side. The refusal a guided flow answers with on a client without elicitation offered as alternatives the meta tools of the issue and merge request domains, which is right for one surface of three, and an e2e scenario quoted that name in an either-or check whose other needle always matched. `toolutil.ErrorResult` was not one of the helpers the rule read until issue 910, so no gate held the sentence at all. It offered those two whichever flow was refused, too, while the release and project flows' descriptions each promised their own. Each flow now hands the refusal the one action its description names, `issue.create`, `merge_request.create`, `project.create` or `release.create`, both drawn from one constant; a test of the catalog in `internal/tools` holds all four to an action the Free catalog serves, and the scenario asserts the issue flow's on its own.

#### Usage

```bash
# Report, with the work list
go run ./cmd/audit_action_ids/

# The gate
go run ./cmd/audit_action_ids/ -check -json ''

# Also what a clean run judged, by kind
go run ./cmd/audit_action_ids/ -v

# One package, without touching the tree's work list
go run ./cmd/audit_action_ids/ -json "" ./internal/tools/issues

# One suite package
go run ./cmd/audit_action_ids/ -json "" ./test/e2e/gitlab/ee
```

#### Flags

| Flag               | Type     | Default                | Description                                                                                                                                                                                                                                                                       |
| ------------------ | -------- | ---------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `-check`           | `bool`   | `false`                | Exit non-zero on anything `Report.Clean` refuses: a published ID that is not canonical, an alias named in prose, an unfolded site, a stale declaration of any table, served prose or an e2e assertion spelling a capability no listing publishes, a helper entry matching no call |
| `-dir`             | `string` | `.`                    | Repository root the patterns are resolved against                                                                                                                                                                                                                                 |
| `-fix-hints`       | `bool`   | `false`                | Rewrite each tool name a folded sentence of served prose spells to the canonical ID of the action the tool projects, and report what moved and what the catalog could not answer for                                                                                              |
| `-fix-hints-tests` | `bool`   | `false`                | With `-fix-hints`, rewrite the test literals that are a piece of, or that pin, a folded sentence                                                                                                                                                                                  |
| `-json`            | `string` | `plan/action-ids.json` | Write the work list here; empty writes none                                                                                                                                                                                                                                       |
| `-v`               | `bool`   | `false`                | Also print what fails nothing: the alias heading of a clean run, the breakdowns by kind, the prose sites nothing folded and the calls per helper                                                                                                                                  |

Positional arguments are package patterns; with none, `./internal/tools/...` with `./internal/toolutil`, and the suite `./test/e2e/gitlab/...`. A pattern under `test/e2e/` is read as suite and any other as served source, an absolute path or an import path being read first as the relative pattern it names, a wildcard enclosing the suite (`./...`, `./test/...`) bringing the whole suite in besides, and each load is made only when a pattern asks for it.

#### Output

Every wrong ID with its file, its line, the string and the closest real ID, grouped by the package that has to act on it, then the alias references, the sites nothing could fold, the stale declarations and a summary naming how many IDs were judged against how many catalog IDs; then the rows, the stale declarations and the count of the served-prose rule, with the values it passed over beside it, and, when the run loaded the suite, the rows and the count of the suite's rule and the helper table entries that describe no call. The rows of both prose rules are printed by every run, because each fails the gate and `make check-action-ids` passes no `-v`, so a red job's log names the file, the line and the spelling to fix. `-v` adds what fails nothing: the prose sites nothing could fold, the breakdowns by kind and how many calls of each helper were read. Without `-check` it exits `1` only for a run that could not be made: a catalog that would not build, source or a suite that did not type-check, or a work list that could not be written.

#### Make targets

- `make audit-action-ids`: the report plus `plan/action-ids.json`.
- `make check-action-ids`: CI gate, and step 11 of `make analyze`.

### audit_dead_consts

Reports every unexported constant in this repository that nothing reads.

`staticcheck`'s `unused`, which `golangci-lint` runs as a gate on every push, treats a const group as one unit: a declaration whose first member is read is read, and the rest of it is never judged. Measured against the pinned toolchain, a package holding `const deadConst = "never used"` on its own is reported and the same constant written as the second member of a group whose first member is used produces no issue at all. The configuration cannot change that either, since `golangci-lint` v2's schema does not accept `unused`'s `constants-are-used` setting.

That would be a narrow gap in another codebase and is a wide one here, because the group is the prevailing shape: every domain under `internal/tools/` writes its canonical action IDs as one const block, and about fifty of them write their assertion messages and fixture strings as another. A cross-link written down and never published leaves its ID in that block, reading like a live cross-reference to anyone who opens the file. The first run of this rule found twenty-five such constants, every one of them inside a group the linter had already looked at and passed.

It loads `./internal/...` and `./cmd/...` through `cmd/internal/goprogram` with the **test variants included**, and judges a constant by the type checker's `Uses` map rather than by a text search, so a name that also appears in a comment, a string or another package does not make it look read. Test files are loaded because a constant a test reads is read: forty-odd packages hand their action-ID block to the catalog test through an `export_test.go`, and a rule that skipped those files would report the live half of every one of them. That is also this rule's one blind spot, and it is worth knowing where it falls: a block exported wholesale is read wholesale, so an ID such a list carries and the package publishes nowhere is invisible here. Reading the block is what finds that.

Exported constants are out of scope. One may be read from anywhere, including the end-to-end packages behind their own build tags, so a run over these patterns could not tell a dead one from one it never looked at. The platforms go the other way: a package carrying a `GOOS`- or `GOARCH`-constrained file is read again under each operating system and architecture pair this project builds for (`linux`, `darwin` and `windows`, each on `amd64` and `arm64`), because a constant only the Windows half of a package reads is read, and so is one only its arm64 half reads; failing a Linux amd64 run over either would be failing over code doing its job. Both halves of the pair are set on every reload, since setting the operating system alone keeps the host's architecture and leaves an `_arm64.go` file out exactly as the first load did. Only the packages whose files the first load actually left out are re-read.

A constant that is kept although nothing reads it is declared in `cmd/audit_dead_consts/declarations.go` with the reason keeping it is right, and a declaration that excuses nothing is reported like every stale declaration in this repository. The key is the constant's identity rather than its name alone: `package:name` for one at package scope, and `package:Func.name` or `package:Type.Method.name` for one declared inside a function, so an entry for the package-level constant never excuses a local one that shares its name, and the reverse. The stale judgement is scoped to the packages the run loaded, so pointing the command at one package does not condemn the whole table.

#### Usage

```bash
# Report
go run ./cmd/audit_dead_consts/

# Report, naming the platform loads the run made
go run ./cmd/audit_dead_consts/ -v

# Gate
go run ./cmd/audit_dead_consts/ -check

# One package
go run ./cmd/audit_dead_consts/ ./internal/tools/issues
```

#### Flags

| Flag     | Type     | Default | Description                                       |
| -------- | -------- | ------- | ------------------------------------------------- |
| `-dir`   | `string` | `.`     | Repository root the patterns are resolved against |
| `-check` | `bool`   | `false` | Exit non-zero when a constant is never read       |
| `-v`     | `bool`   | `false` | Name the platform loads the run made              |

Positional arguments are package patterns; with none, `./internal/...` and `./cmd/...`.

#### Output

Every unread constant with its file, its line and whether it was declared on its own or inside a group the linter cannot see, then the stale declarations, then a summary naming how many constants were judged in how many packages. Exits `1` under `-check` on any finding, and `1` whenever the source could not be loaded.

#### Make targets

- `make audit-dead-consts`: the report.
- `make check-dead-consts`: CI gate, also step 4 of `make analyze`.

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

### audit_e2e_coverage

Says what the end-to-end suite covers, from what happened rather than from what the source mentions. The suite records every call a test makes and every dispatch the server's own span reports (through `internal/testutil/e2ecalls`, when `GITLAB_MCP_TEST_E2E_CALLS_DIR` is set); this command reads those shards, joins each dispatch to its call on the trace id, compares them with the catalog the runtime served, and gives every runtime x surface x mode x action exactly one state: `asserted` (a passing test asked for it, expected success, got it, and the server dispatched exactly that action), `unobserved` (the same with no span to confirm the dispatch), the shallower credits `sweep-only`, `error-path-only`, `refused-only`, `preview-only` and `cleanup-only`, and the reasons there is no credit: `unasserted` (every result-bearing call site discards the answer, which only the source can say), `unservable` (no individual tool, a shadowed name, withheld by scope or by read-only mode), `skipped`, `failed`, `absent`. L1 is asserted on any surface in the default mode, L2 on the dynamic surface, L3 on all three.

The capabilities beside the tools are classified on the same states, each counted at the grain its content varies along (`capabilityGrains` in `classify.go`, which the committed page prints). Resources (by template), prompts, completions (by reference and argument) and subscriptions (by kind) get one cell per item per capability surface: the server registers them from `GITLAB_MCP_CAPABILITY_SURFACE` and the operator's exclusions alone, so neither the tool surface nor the protective mode changes what a session is served, and a cell per shape would be one nothing could fill differently from its twin. Subscriptions exist on the full capability surface only. `gitlab://tools` and `gitlab://tools/{id}` are the exception, a kind of their own called `tool_manifest` with one cell per surface x mode x capability surface, because they list what the session's tool surface registered after the read-only and safe passes; `internal/resources.ToolSurfaceResourceURIs` names the pair, so the command never spells it. The elicitation flows and the protective modes stay at surface x mode, being reached through the actions a surface serves in a mode. The report's `capability_surfaces` rows are the denominator of the capability-grain cells, one per capability surface, since the session rows fold both capability surfaces into one row per shape. The completion half of that denominator is the session line's `completions` list, which the harness writes from every argument of every prompt and every variable of every template a session listed, spelled through the one function a completion call's record names its target with, so a call and the reference it is counted against cannot disagree. A subscription is recorded as answered only when the server acknowledged it (`notifications/subscriptions/acknowledged`, the one word a client gets on protocol 2026-07-28, since the SDK discards the server's answer to a subscribe), and as a protocol error when it did not, so a subscription the server declined is never credited as asserted; it reads `error-path-only`, the same credit a resource read that answered a handled error earns. The resource and completion sweeps assert nothing about what came back. The subscription sweep does assert on each outcome: a decline its `knownDeclines` table does not name fails it, and so does a declared template the server acknowledges or no longer advertises. It is still credited as a sweep, because every template it subscribes to is one the server advertised and none is a subscription of its own choosing. All three say so by passing `harness.For(harness.PurposeSweep)` to every read, completion and subscribe they make, so what they alone reached reads `sweep-only` rather than `asserted`; the records committed before the reads and completions carried that purpose count the `resources` and `completions` rows the sweeps filled as `asserted`, and the next record moves those cells to `sweep-only`. `-baseline` compares the action cells alone, so the grain the capability cells are counted at does not reach it.

Each session row carries `dispatch_observed`: whether at least one session of that surface x mode made a traced call and every one that did had the server's own span of at least one of them arrive, of whatever method, a resource read, a prompt or a completion counting as a tool call does. A session that made no traced call at all is idle (its session line says `idle`), is named in `diagnostics.idle_sessions`, is counted in the row's `idle_sessions`, and holds no row false on its own, since it made no call that carried a trace (a subscribe on protocol 2026-07-28 carries none); the sessions started only to compare what they list at a pinned tier are the usual case. The shards carry schema version 2 since `dispatch_observed` came to count a span of any method and `idle` was added, and a shard of version 1 is refused rather than folded under that reading: re-record it. Two readers are the exception, `-baseline` here and R-PATH's `-e2e-calls` (`audit_1to1 -scope=paths`), because both read the action cells alone, which come from the call and dispatch lines, and version 2 changed none that they read: it did stop writing a dispatch line whose span named neither a tool nor an action, so a version 1 shard can carry such lines, and every reader skips a dispatch line that names no action. Both read a version 1 shard too and drop that shard's session lines, which is what keeps the old suite's baseline, recorded once by a suite that no longer exists, comparable, and what keeps it from turning R-PATH's observation into an error under the default `dist/e2e-calls`. A row whose sessions were all idle reads false, because nothing about its telemetry was seen, and its `idle_sessions` equal to its `sessions` says so. A session that made traced calls and saw no span of any of them is named in `diagnostics.unobserved_sessions` and does hold its row false. The flag decides no credit: a resource read, a prompt and a completion are credited on their answer, and a tool call on the action its own span named. Only a span that names the tool or the action it ran is written as a dispatch line, so the `dispatch_lines` diagnostic counts those and not every span that arrived; a line naming only the find tool is among them, though the join passes over it for naming no action.

A runtime is an edition and a tier, read off the run lines; every shard under one directory is one runtime, and a parent whose children each hold shards is read as one runtime per child, which is the layout `dist/e2e-calls/<target>` produces.

#### Usage

```bash
# The JSON report for every runtime under dist/e2e-calls
go run ./cmd/audit_e2e_coverage/ -calls dist/e2e-calls

# The work list the old gap audit used to print, plus a Markdown summary
go run ./cmd/audit_e2e_coverage/ -calls dist/e2e-calls/ce -report -summary -

# The floors, joined with the go test -json stream gotestsum wrote
go run ./cmd/audit_e2e_coverage/ -calls dist/e2e-calls/ce -results dist/e2e-reports/e2e-docker-log.json -runtime ce -check

# The superset check against the old suite's recorded baseline
go run ./cmd/audit_e2e_coverage/ -calls dist/e2e-calls/ce -baseline dist/e2e-calls/baseline-ce -runtime ce

# The push-time gates, with no GitLab
go run ./cmd/audit_e2e_coverage/ -static
go run ./cmd/audit_e2e_coverage/ -port-map

# Commit one half of the per-runtime coverage record and redraw its page
go run ./cmd/audit_e2e_coverage/ -calls dist/e2e-calls/ce -results dist/e2e-reports/e2e-ce-log.json \
  -runtime ce -static -record

# Judge the committed record against this tree, with no GitLab and no network
go run ./cmd/audit_e2e_coverage/ -check-record

# Compare the committed page with a fresh rendering of that record
go run ./cmd/audit_e2e_coverage/ -check-record-page

# Redraw the page from the committed record alone
go run ./cmd/audit_e2e_coverage/ -render-record
```

#### Flags

| Flag                 | Type     | Default           | Description                                                                                                                                                                                                                                           |
| -------------------- | -------- | ----------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `-calls`             | `string` |                   | Shard directory written by the e2e suite, or a directory holding one per runtime                                                                                                                                                                      |
| `-results`           | `string` |                   | `go test -json` stream (`gotestsum --jsonfile`) to join: it settles the test status of every call, in the package the call's shard names, and lists the tests that passed while calling nothing                                                       |
| `-runtime`           | `string` |                   | Comma-separated runtimes to report and, with `-check`, to require: `ce` (any community runtime), `ee` (a licensed enterprise one), or an `edition/tier` key                                                                                           |
| `-baseline`          | `string` |                   | Shard directory to compare against, such as the old suite's, whose schema 1 shards it reads without their session lines; fails on any runtime x surface x mode x action x credit it reached in a passing test that `-calls` does not                  |
| `-check`             | `bool`   | `false`           | Apply the floors: every expected runtime left a run line, a test call was recorded, no package refused or ran under a `-run` filter (a partial run is not a coverage claim), and the asserted count is at or above the floor `exemptions.go` records  |
| `-report`            | `bool`   | `false`           | Print the gap work list as TSV instead of the JSON report: one row per action not asserted on any surface, with its state on each, then a summary line                                                                                                |
| `-o`                 | `string` |                   | Write the JSON report to this path instead of stdout                                                                                                                                                                                                  |
| `-summary`           | `string` |                   | Write a Markdown summary to this path, or `-` for stdout, where it takes the JSON's place (give `-o` to keep both); what a CI step puts in `GITHUB_STEP_SUMMARY`                                                                                      |
| `-static`            | `bool`   | `false`           | The push-time gate over `test/e2e/gitlab`, loaded with its tests under the `e2e` tag                                                                                                                                                                  |
| `-port-map`          | `bool`   | `false`           | Check that every `Test` function of the old suite has a `// Replaces:` successor in the new one or a declared drop in `portmap.go`; an old test whose file is already deleted stays on the map through the retired list there, held to the same rule  |
| `-old-suite`         | `string` |                   | Where the port map reads the retired suite's `Test` functions from. Required, and deliberately without a default: that tree is no longer in this repository, so it has to name a checkout that still carries it                                       |
| `-new-suite`         | `string` | `test/e2e/gitlab` | Where the port map reads the `Replaces:` lines from                                                                                                                                                                                                   |
| `-record`            | `bool`   | `false`           | Fold this run's runtimes into the committed coverage record and redraw its page; needs `-calls`, `-results` and a `-static` scan that actually ran, since without them the record would freeze a more generous classification than the gates judge by |
| `-check-record`      | `bool`   | `false`           | Judge the committed record against this tree, with no GitLab and no network                                                                                                                                                                           |
| `-check-record-page` | `bool`   | `false`           | Compare the committed page with a fresh rendering of the record; the freshness half of the gate, separate because it is the only half the source tree can move                                                                                        |
| `-render-record`     | `bool`   | `false`           | Redraw the record's Markdown page from the record itself                                                                                                                                                                                              |
| `-record-path`       | `string` |                   | The record to write, check or render; the repository's `docs/development/e2e-coverage.json` when empty                                                                                                                                                |
| `-record-page`       | `string` |                   | The page rendered from it; the repository's `docs/development/testing/e2e-coverage.md` when empty                                                                                                                                                     |
| `-dir`               | `string` |                   | Repository root; found from the working directory when empty                                                                                                                                                                                          |

#### The static gate

`-static` needs no GitLab and runs on every push. It loads `test/e2e/gitlab` and `test/e2e/internal` with `Tests: true` and `-tags=e2e` through `cmd/internal/goprogram`, and reads every constant of the harness's `ActionID` type off the type checker's own record, so a literal passed to a helper's `ActionID` parameter, a typed constant at its use sites and the entries of a table are all found. Each must name a catalog action; none in `common` or `ce` may be above Free; an `ee` test naming an Ultimate action, itself or through any chain of same-package helpers and methods, must declare `Needs(Tier(edition.Ultimate))` somewhere on that chain, and a `Tier(edition.Ultimate)` that is not an argument of `Needs` declares nothing. A verb called with a non-constant id is listed, not failed, and so is one handed every argument by a single multi-value call, `DoVoid(args())`, which is listed by that call rather than passed over. A `Do`, `Try` or `Eventually` whose answer is assigned to blanks or dropped as a statement is a finding, whichever way its arguments arrive. The harness exports nothing outside the harness uses are listed, and a type is counted used when anything hands it out. The ratchet is on (`ratchetEnabled` in `exemptions.go`, switched on with the gate), so every catalog action needs an id in a package that can run it or an entry in `exemptions.go` with a category and a reason, a stale entry is a finding, and the unused exports fail too. On a tree where `test/e2e/gitlab` does not exist the gate says so and exits 0.

#### The committed coverage record

The report above describes one run and is written into the gitignored `dist/`, so nothing on `main` can say what the suite covers. `-record` commits an allowlist of it to `docs/development/e2e-coverage.json`, keyed by the two Docker targets (`ce`, `ee`): the runtime, the run rows with their commit, GitLab version and fixture profile, the session rows, the capability surface rows, the summary, and the three level lists, which are what make the record an answer to _which_ actions rather than only to how many. The per-action cells stay out, because a thousand rows per runtime would make every refresh a diff nobody reads; `report.Directory` stays out because it is a path on the machine that ran the suite. Each entry's `retrieved_at` is read off the run IDs' own timestamps rather than the writing clock, so re-running the command over old shards does not reset the window.

`docs/development/testing/e2e-coverage.md` is rendered from the committed record by `-render-record`, which is why that half is a member of `make update-all` and the measurement is not: redrawing needs a checkout and measuring needs a booted GitLab.

`-check-record` is the offline gate over the document. It refuses a schema it does not write, a missing or unexpected runtime key, an entry filed under the wrong key, a level count that disagrees with the list beside it, an id at `l2` or `l3` that `l1` does not carry, an `L1` larger than the catalog, the same floors `-check` applies to a live run, a date nothing can read or that has not happened, and a record older than 90 days. The window is deliberately not `cmd/internal/provenance`'s 180 days: that one is about GitLab's release cadence for records copied out of `gitlab-org/gitlab`, and this record pins nothing external — it measures this repository's own suite against a catalog that moves whenever an action is added here. Its last fortnight is a note rather than a finding, because clearing the window needs a Docker run of both halves and the licensed half needs an activation code CI does not hold: the day it closes, every open pull request would go red over something no contributor can fix, so the deadline is announced for two weeks first.

`-check-record-page` is the other half: the committed page against a fresh rendering of the committed record, byte for byte. It is a separate flag because it is the only half the source tree can move — the figures need a booted GitLab, while the renderer and the table formatter are code here and `make update-all` redraws the page — which makes it a freshness gate CI defers below a stack's tip, exactly as it does `check-bench-resources`, while the judgments above hold on every layer.

Two things are reported and do not fail. A catalog that has moved under the record (a different size, or an action the record credits that the catalog no longer has) is a note, because `make check-e2e-static` already fails on the same rename from the scenario's side and a rename must be committable without booting a GitLab first. And an entry measured on a revision that is not an ancestor of `HEAD` is a note — but only when git can resolve it: CI checks the repository out at depth one, where every revision but `HEAD` is unknown, so an unresolvable revision says nothing at all rather than warning on every run.

A third note names an entry recorded before the capability grain, which carries no `capability_surfaces` rows and whose histograms count every capability item once per surface x mode; the page states the older grain in that entry's section. The schema version did not move for the rows: they are optional and omitted when empty, both versions of the command read a document holding them or not, and a refresh folds one runtime at a time, so which grain an entry was measured at is said by the entry rather than by the document. Re-recording the half with `make e2e-coverage-record-ce` or `make e2e-coverage-record-ee` clears the note.

#### Output

The JSON report is a list with one entry per runtime: the run and session rows, the summary and levels, one row per catalog action with its default-mode state per surface, every non-absent cell with the tests behind it, the capability surface rows, the capability cells (each carrying only the coordinates its kind is counted at), the dispatch mismatches, the tools no session served, the served tools no test called, and the diagnostics about the record itself. Exit `0` when nothing failed, `1` on a finding (a check floor, a baseline loss, an incomplete port map, a static finding, a committed record that does not hold), `2` when the audit could not run.

#### Make targets

- `make audit-e2e-coverage`
- `make check-e2e-static`
- `make e2e-coverage-record` (both halves), `make e2e-coverage-record-ce`, `make e2e-coverage-record-ee`
- `make e2e-coverage-record-render`
- `make check-e2e-coverage-record`, `make check-e2e-coverage-page`

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

The documents themselves come from `cmd/internal/graphqldocs`, the same reading [audit_graphql_documents](#audit_graphql_documents) judges against the schema. It used to find them with a walk of its own over string constants and package-level variables, which is every document this repository writes today and not every document it may write tomorrow: a document moved into a `.graphql` file and pulled in with an embed directive folds to nothing for the type checker, so that walk saw none of it while the schema gate read it straight off disk. Two detectors of one thing disagree by construction, and the narrower one was the gate.

The rule that says whether a document reads or writes is the inventory's too, for the same reason one step down. This command kept a text rule of its own after it started reading the shared inventory, and the two did not describe the same set: the inventory wanted the operation keyword at the very start of the comment-stripped text and refused a brace-wrapped single word with no space in it, where this command's rule accepted a keyword opening any line. A mutation written either way was therefore in neither the inventory nor the report, while the gate went on printing that no read-only action reaches a mutation: narrower than the rule it describes, and silent about it. `graphqldocs.LooksLikeDocument` now says what a document is and `graphqldocs.DefinesMutation` says whether it writes, so this command sees exactly the documents the schema gate judges and classifies every one of them. What stays here is what the answer means: an action classified `ReadOnly` must not be able to reach a mutation.

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

Four things count as findings, not only the obvious one:

- a read-only action whose handler can reach a mutation document;
- a read-only action no `ActionSpec` construction resolves to, or whose route resolves to no handler, because an action the audit cannot classify is one it cannot vouch for;
- an exception directive that no longer excuses anything, so an exception cannot outlive its reason;
- a document in the shared inventory that this audit can tie to no handler: a `.graphql` file belongs to no object and to no function body, and neither does a document assembled in a package-level initializer, so the reachability walk can never reach either. The repository writes every document as a named constant today, so this is silent, and the day one moves it says so instead of going quiet.

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

It loads the whole program with `go/packages` rather than matching the source with a regular expression, because four of this repository's documents are assembled by concatenating a shared fragment constant and only the type checker knows what the assembled value is. Constants are folded during type checking, so a document written as three pieces is judged as the one string GitLab would receive. Documents held in `.graphql` files are read straight off disk as well, since a `go:embed` variable is not a constant and folds to nothing: without that, moving a long document into its own file would drop it out of the inventory and the audit would report one fewer document and still exit `0`.

What it cannot check is variables: a document read out of the source has no request behind it, so nothing says which variables a handler will send or what they will hold. That half belongs to the test transport, which sees a real request.

What it does not read is client-go, which builds another 42 documents of its own for the achievements, work item, security attribute and terraform state services among others. Those reach GitLab through this server too, and only the test transport judges them, on whichever ones a test happens to drive. The summary counts this repository's documents, not the server's whole GraphQL surface.

#### The live re-probe

`-live` introspects a GraphQL endpoint right now and judges the documents against the schema that instance serves, instead of the pin. It is what `make check-graphql-documents-live` runs, and it is the one check the pin cannot perform: the pin says our documents were valid on gitlab.com on the day it was taken, this says they are valid on the GitLab that shipped today, which is the version a self-managed instance actually runs. The introspection and the SDL conversion come from `cmd/internal/graphqlintrospect`, shared with `gen_graphql_schema` rather than written twice, so both commands understand an instance's answer the same way.

An introspection carrying fewer types than a GitLab schema has is refused rather than judged by, at the same floor `gen_graphql_schema --check` refuses a truncated pin with. Without that, an instance that boots and answers with a fragment would let every document validate against the little that arrived, and the run would report success for a question nobody asked.

The same run reports where the pin and the probed schema disagree **about a type, field or argument one of our documents touches**. Whole-schema drift between two GitLab releases is thousands of lines and says nothing; the drift under our own selection sets is a handful of coordinates, and it is what turns the pin's age into a number a reader sees. Each document is walked through whichever of the two schemas accepts it, preferring the probed one, so a document the pin accepts and the instance refuses is still walked and the field that stopped existing is named.

`-schema` is the same comparison against an SDL file already on disk: a schema captured earlier, or the one belonging to the particular self-managed release a document is meant for. Naming both `-live` and `-schema` is refused, since which schema judged the documents is the whole meaning of the result.

The scheduled `.github/workflows/ee-schema.yml` job points `-live` at an unlicensed `gitlab/gitlab-ee:latest`, which serves the whole Enterprise schema because GitLab builds the schema at boot, before any license exists. See [Enterprise Schema Checks](enterprise-schema-checks.md).

#### Usage

```bash
# CI gate
go run ./cmd/audit_graphql_documents/

# Also list every document it accepted
go run ./cmd/audit_graphql_documents/ -v

# Judge against the schema an instance serves right now, with the drift report
go run ./cmd/audit_graphql_documents/ -live https://gitlab.com/api/graphql

# Judge against an SDL file already on disk
go run ./cmd/audit_graphql_documents/ -schema /tmp/live/gitlab-schema.graphql
```

#### Flags

| Flag      | Type     | Default   | Description                                                                        |
| --------- | -------- | --------- | ---------------------------------------------------------------------------------- |
| `-dir`    | `string` | `.`       | Repository root to audit                                                           |
| `-v`      | `bool`   | `false`   | List every document checked, not only the refused ones                             |
| `-live`   | `string` | _(empty)_ | GraphQL endpoint to introspect now and judge against, instead of the pinned schema |
| `-schema` | `string` | _(empty)_ | SDL file to judge the documents against, instead of the pinned schema              |

`GITLAB_TOKEN` is read for `-live` and sent to that endpoint only. GitLab answers introspection to anyone, so it decides nothing but whether the report can name the version that answered, which GitLab refuses to tell an anonymous caller.

#### Output

One block per refused document on stderr, naming the package, the constant it is declared as (or the `.graphql` file, or "an inline document"), the file and line, and every validation error underneath. The summary line says which schema judged them: the pin's provenance, the instance `-live` asked, or the path handed to `-schema`. A `-live` or `-schema` run also prints the drift block on stdout, which is information rather than a failure: it never changes the exit status. Exits `1` on any refusal, on a source tree that cannot be loaded, on an instance that could not be asked or answered with less than a whole schema, on a `-schema` file that cannot be read or parsed, and when no documents are found at all, since an audit that finds nothing is looking in the wrong place.

#### Make targets

- `make check-graphql-documents`: the CI gate.
- `make audit-graphql-documents`: the same gate, listing what it accepted.
- `make check-graphql-documents-live`: the same documents against a schema fetched from gitlab.com now. Needs the network, so it runs under `make test-e2e-gitlab-com` rather than in CI.

### audit_graphql_shapes

Fails when a struct this repository decodes a GraphQL response into cannot hold what the document it is sent with asks for.

Every other GraphQL gate reads the request. The pinned schema judges the document and the test transport judges the variables, and neither looks at the struct the answer is decoded into, because the fixture a test answers with is written to match that struct. A decoder that disagrees with GitLab is therefore invisible to all of them: the licensed e2e run found `startLine` and `endLine` declared as `String` by the schema and decoded into `int` by two response structs, every located finding failed to decode, and every gate was green.

This command pairs the two halves. It loads `./internal/...` with `go/packages`, finds every call whose first argument is a `GraphQLQuery`, folds the document that call sends and resolves what the pointer it decodes into points at, then walks the validated selection set against that type under `encoding/json`'s own rules: a field is matched by its json tag or, failing that, by its name case-insensitively; embedded structs are flattened; a type with an `UnmarshalJSON` method is trusted to know what it reads; a `map[string]T` takes every selected field as a `T`; and a type parameter is read as whatever the caller bound it to. A document reaches its call in several ways and all are followed: a constant named at the call, a local variable given one constant or one per branch, a parameter of a wrapper function (or a field of a struct parameter, the `toolutil.GraphQLNoteMutation` shape), and a generic wrapper called by another generic wrapper, whose type parameters are bound through the whole chain.

The scalar table is how GitLab serializes each scalar the pin declares: `Int` is a JSON integer, `Float` and `Duration` may carry a fraction, `Boolean` is a boolean, `JSON` and `CiInputsValue` are arbitrary, and everything else, `BigInt` included, is a string. A scalar the table does not name and that is not a global id is reported rather than guessed, so a scalar a future pin adds is classified on purpose.

Three disagreements fail the gate:

- a Go kind that cannot hold what the schema says GitLab sends: a `String` into an `int`, a list into a struct, an object into a string, a `Float` into an integer;
- a Go field the document never selects, which is always empty and lies to the model about what GitLab answered (a shared node struct decoding a document that selects less is the common cause, and the fix is one selection written once, as `vulnFields` now is);
- a scalar with no serialization in the table.

A field the document selects and no Go field reads is reported under `-v` and does not fail: it is transfer, not truth. A document built at run time, a request that is not a literal, a decode target that is not a pointer, a wrapper nothing calls, and a document `cmd/internal/graphqldocs` finds that no call this audit can see sends, are all failures rather than silences, because a document nobody judges is the shape this gate exists to refuse.

What it does not read is client-go's own documents and decoders, for the reason `audit_graphql_documents` gives: they reach GitLab through this server, and only the test transport judges them.

#### What the schema offers and nobody asks for

The same walk answers the reverse question, and until it did nothing asked it. R-PATH holds GitLab's own OpenAPI record against our REST output types and reports the fields we do not publish; the eleven GraphQL-only domains contribute **zero** findings to it, because their output types pair with no client-go struct and the REST record says nothing about them. `-report` writes what this walk finds instead: at every object a Go struct decodes, the fields the pinned schema offers that **no document of the package decoding it** selects. Today that is 970 fields across 12 packages and 32 schema types, 236 of them undeclared, out of the 40 document-and-decoder pairings.

**The claim is package-wide, and is computed as one.** A package sends several documents at one object and they differ on purpose: `branchrules` expresses a licensing tier as a CE document that omits `codeOwnerApprovalRequired` and an EE document that selects it, and `epicissues` sends a query that reads a work item beside a mutation whose payload can echo little more than an id. So the walk records what each document _did_ select alongside what it did not, keyed the same way a finding is, and a field any document of the package selects is not reported. Judging each document alone made 3 of the 19 `branchrules` findings and 8 of the 44 in `epicissues` false, and the falsehoods landed on exactly the values those tools publish today. The document a finding names is the witness the field was offered at, not the extent of the claim.

**A finding names the package whose struct decodes the object**, which is not always the package the call is in. Every epic note and discussion mutation is sent by `toolutil.ExecGraphQLNoteMutation` and decoded into the domain's own struct; filing the finding against the send names a package that publishes nothing and cannot act on it, and it made every one of the 29 same-name annotations attributed to `toolutil` wrong, since the lookup ran against a package with no output types of its own. That annotation is `same_name_in_package`, named as the lead it is: the match is over a package's output types flattened together rather than over the type modelling the object, so a hit may be the same value under another spelling or an unrelated field sharing a name. `vulnerabilities` is the case that settled the name: it declared `web_url` and filled it nowhere, because no document selected `webUrl`. The defect is fixed, and it is the evidence for the name rather than an open finding.

Three conditions bound it, and each is what separates a backlog from a dump of the schema:

- **a Go struct decodes the position**, so the walk is bounded by our own decoders rather than by the schema graph;
- **it is not the operation root**, whose fields are other requests rather than this response. The root alone offers 17 of the 22 thousand fields an unbounded walk reports, and what it offers is the catalog question the action catalog answers;
- **the object is read rather than traversed**, which a struct decoding at least one scalar or enum shows. A struct that decodes only the next hop is an envelope, and the object is a route.

Six exclusions sit on top, each costing what `sent.go` records: connection plumbing (`edges`, `nodes`, `node`, `cursor`, `pageInfo`, `count`) and the `PageInfo` object, which are transport; `__typename`, which no schema declares; a field with a **required** argument, which is a second request rather than something GitLab sends with this response; and `clientMutationId`, an echo of a value we never supply. Connection-**typed** fields are deliberately not excluded — they are reported as class `collection`, because `BranchRule.approvalRules` and `Note.awardEmoji` are sub-collections a 1:1 surface plausibly owes — and neither are fields whose arguments are all optional. At a union or interface the question is asked once per member the document names in a fragment and never about one it does not: that is what keeps a security finding's whole `location` family (the DAST `hostname`, `param` and `requestMethod` our decoder does not read) without reporting every `VulnerabilityLocation*` variant on every finding.

**It reports and does not gate**, for a different reason than the REST joins. Their oracle is incomplete; this one is not, since the pin is exactly what GitLab serves. What makes it a report is that a field GitLab offers and this server does not surface is a candidate for the surface rather than a defect in it, GitLab adds fields weekly, and this dimension has **no tier oracle and no deprecation oracle**: the schema declares no tier (GitLab gates a GraphQL field at resolve time, so an unlicensed instance answers null rather than omitting the field), and the pin carries no `@deprecated`, because `cmd/internal/graphqlintrospect` drops the directives and descriptions on decode. Both are stated on the report itself rather than left for a reader to notice, and both are closed by one sidecar record written from the introspection fetch that already carries the data. A `tier` is omitted from a finding rather than emitted empty: an always-empty field would read as a condition that was checked and found absent, which is the opposite of true.

One sub-class **does** gate, on every run rather than only under `-report`: a mutation payload whose `errors` **no field of the decoder reads**, which drops GitLab's account of a refused mutation and lets the tool report success. The condition is the decoder and not the document, because a payload that asks GitLab for its errors and decodes none loses them exactly as completely as one that asks for nothing; the reverse, a decoder field the document never selects, is already a hard failure of the always-empty leg above and is left to it. Every payload this repository sends passes today, so the gate costs nothing to adopt and keeps it that way.

A finding is answered rather than fixed by an entry in `sent_declarations.go`, keyed by package, schema type and field, with `*` covering a whole schema type at a package — which is what makes the table tractable, since one entry answers all fifty-odd `UserCore` fields under a note's author. Nine entries answer 734 of the 970 rows today: the user under an author, the reference stubs a vulnerability names, the lookup document that resolves a work item id, and the work item a notes query names only to reach its notes widget, which is 68 rows in `epicdiscussions` and `epicnotes` and is the same judgement the third condition makes on its own wherever the anchor selects no leaf. A declaration that answers nothing **fails the run**, on the terms every declaration table in this repository is held to.

#### How much of the surface the question was put to

The report publishes a coverage breakdown rather than one number, because the positions asked about are fewer than the positions reached and one count would say otherwise. Today: 199 object positions reached, **133 asked about**, 52 skipped as traversed rather than read, 14 skipped because that pairing had already been asked about that schema type — the `asked` set closes a type after its first leaf-reading position, so a later position selecting strictly less is skipped too.

Three more positions are left in silence, and are counted so the silence is visible rather than assumed away:

- **an object selection no Go field decodes** stops the walk with a non-gating note before anything under it is reached (3 today: `mutationDeleteCustomEmoji` selects `customEmoji { id name }` while its payload struct decodes only `Errors`, so the other four fields `CustomEmoji` offers are never asked about, and `mutationDestroyNote` does the same with the note it echoes, at each of its two callers);
- **an object decoded into a map** has neither fields to judge nor a package to file a finding against, and is the one place the mutation-errors gate is not applied either (0 today: the only maps in this repository decode an operation root, which is not a response);
- **a type that unmarshals itself** is trusted with its own decoding and walked no further (0 today).

Two limits sit outside the walk altogether. Like `audit_graphql_documents`, it loads `./internal/...`, so it sees only the documents this repository's source writes: an operation client-go builds has its document text and its decoder inside the SDK's module, where there is no send here to pair and no decoder here to compare a schema type with, and a GraphQL-only endpoint has no REST operation for R-PATH to see either. The report names that set rather than leaving a reader to infer it, reading it from `docs/development/request-inventory.json`: today **38 operations in 7 packages** (`achievements` 12, `workitems` 7, `epics` 6, `workitemsavedviews` 6, `projects` 3, `securityscanprofiles` 3, `terraformstates` 1), each with the operations named. Closing that gap means teaching `cmd/internal/graphqldocs` to fold a document client-go assembles at run time, which is a separate piece of work and is not started. A run whose `-dir` holds no inventory says so on the report rather than reporting an empty set.

#### Usage

```bash
# CI gate
go run ./cmd/audit_graphql_shapes/

# Also list every pairing judged and every selection nothing reads
go run ./cmd/audit_graphql_shapes/ -v

# Judge against an SDL file already on disk
go run ./cmd/audit_graphql_shapes/ -schema /tmp/live/gitlab-schema.graphql

# Write what the schema offers and no document of the decoding package selects
go run ./cmd/audit_graphql_shapes/ -report plan/graphql-sent.json
```

#### Flags

| Flag      | Type     | Default   | Description                                                                                         |
| --------- | -------- | --------- | --------------------------------------------------------------------------------------------------- |
| `-dir`    | `string` | `.`       | Repository root to audit                                                                            |
| `-v`      | `bool`   | `false`   | List every pairing judged and every selection nothing reads, not only the disagreements             |
| `-schema` | `string` | _(empty)_ | SDL file to judge the documents against, instead of the pinned schema                               |
| `-report` | `string` | _(empty)_ | Write the fields the schema offers that no document of their package selects, as JSON, to this path |

#### Output

One block per pairing with a disagreement on stderr, naming the package, the constant the document is declared as, the call, and, when the call received the document through a wrapper, where it was handed over; every finding sits under it with the response path it is about, from `data` down. Every call that could not be paired, and every sent declaration that answers nothing, is one stderr line of its own. Under `-v`, stdout carries an `ok` line per clean pairing and a block per pairing whose only findings are selections nothing reads. With `-report`, stdout also names the file and how many fields it holds, on a second line how much GraphQL was never asked about at all, since a count of findings alone reads as the whole surface. The summary line says which schema judged them. Exits `1` on any disagreement, on anything unpaired, on a stale declaration, on a source tree that cannot be loaded, and when no call at all was found, since an audit that found nothing to judge is broken rather than satisfied.

The report holds the findings deduplicated by package, schema type and field, with the positions counted rather than listed; a finding names the document it was witnessed at, the operation, the response path, the Go type at that position, the field's SDL type, its class (`leaf`, `object` or `collection`), whether the schema promises it with every answer (`always` or `nullable`), the spelling that package publishes the value under when it does, and the declaration answering it when one does. The check block above them says what could not be consulted, what became of every position walked, and which packages' GraphQL is outside the walk.

#### Make targets

- `make check-graphql-shapes`: the CI gate; also step [15/15] of `make analyze`.
- `make audit-graphql-shapes`: the same gate, listing everything it judged.
- `make audit-graphql-sent`: the same run, writing `plan/graphql-sent.json`. The record is deliberately uncommitted and not freshness-gated: a schema re-pin would churn it every time.

## Surface quality audits

### audit_surface_quality

Consolidated MCP tool surface quality audit. It combines metadata-quality checks (naming, annotations, schema shape, duplicates — formerly `audit_tools`) and output-quality checks (`OutputSchema`, Returns/See-also, Title — formerly `audit_output`) behind a single `-view` flag.

Both views judge the surface as a client receives it, because both list it through `cmd/internal/mcpsurface`: the meta view therefore includes `gitlab_server`, and every schema it inspects has been through the lockdown and the pagination bounds. `make audit-docs` runs the command once with the default `-view=all` rather than once per view, since one listing now serves both.

The metadata view ends with a **Result Envelopes** section (the `envelopes` key of its JSON report): every registered Markdown formatter driven through `MarkdownForResult` with a zero and a populated fixture from `internal/testutil`, counting the nil renders of a zero value apart from those of a populated value, listing every content block that carries no `Annotations`, every formatter that panicked, and the registry's own record of refused or half-honored registrations. It reports and does not gate: the dispatchers annotate every text block on the way out, so a bare block here is a formatter building its own envelope, and the list is the migration's work list.

**`-check` makes the command a gate.** It prints the violations that gate and nothing else, and exits 1 on any. What gates is every rule that reads the served surface, in both views. The result-envelope section is excluded and stays a report, because a nil render of a zero value is a guard rather than a defect and the ten duplicate registrations it lists are a backlog nobody has worked through, so failing on them would fail every run from the day the flag was added.

Two rules the same run answers were added with it:

- **`edition-tier`.** The tier an individual tool's own description states, against the lowest tier the surface serves that tool at, read by listing the individual surface at Free, Premium and Ultimate. It reads the served surface rather than an `ActionSpec`'s `Edition` field because the field is not what gates a client: the catalog aggregation tags whole domains and overwrites whatever a spec declared, so a spec can disagree with its own description, with the catalog tag and with GitLab's licence table while nothing observes it. That is exactly the state `securitysettings.groupSecuritySettingsOptions` was in. Only a parenthetical of the first sentence counts, and only one whose leading clause is a tier phrase, which is the convention the descriptions follow: `(Premium/Ultimate, destructive)` states Premium, `(Premium/Ultimate iteration list type)` states nothing. The prose form (`Requires an Ultimate license`) is deliberately not read yet, because reading it today reports the three group Datadog tools, which are served at Free and say they need Premium: that is a gating question about group-level integrations rather than a description defect, and a gate must not be introduced red.
- **`constant-index`.** A list formatter that renders the head of a slice where it means to render every element. It is read off the populated render the envelope section already produces: the audit fills its fixtures through its own text function, which spells the whole field path rather than the field's name, so the two elements of a slice of structs differ. Until they did, this walk could not tell a correct render from one that prints the first row twice. A formatter is reported when the render carries the first element's sentinel twice and the second element's not at all; twice rather than once, because a summary that names only the head of a list is not a defect. `constantIndexDeclarations` answers a finding the rule cannot tell from a defect, and a declaration that answers nothing fails.

#### Usage

```bash
# Both views (default)
go run ./cmd/audit_surface_quality/

# Metadata view only
go run ./cmd/audit_surface_quality/ -view=metadata

# Output view only
go run ./cmd/audit_surface_quality/ -view=output

# As a gate
go run ./cmd/audit_surface_quality/ -check
```

#### Flags

| Flag     | Type     | Default | Description                                                                                                                                      |
| -------- | -------- | ------- | ------------------------------------------------------------------------------------------------------------------------------------------------ |
| `-view`  | `string` | `all`   | Which audit view to run: `metadata`, `output`, or `all`                                                                                          |
| `-json`  | `bool`   | `false` | Emit JSON instead of Markdown; requires `-view=metadata` or `-view=output` (rejected with `-view=all`, which would emit two top-level documents) |
| `-check` | `bool`   | `false` | Print only the violations that gate and exit 1 on any; rejected with `-json`, which prints the whole report                                      |

#### Output

A Markdown report to stdout with summary tables, violations/findings grouped by category, and a full tool listing. With `-json` (single view), a JSON document for that view. With `-check`, one line per view plus one line per violation.

#### Make targets

- `make audit-surface-quality` — both views.
- `make check-surface-quality`: the gate, also step 21 of `make analyze` and a step of CI's generated-artifacts job.
- `make audit-tools` — `-view=metadata`.
- `make audit-output` — `-view=output`.

#### Notes

Both views read their listing from [`cmd/internal/mcpsurface`](#cmdinternalmcpsurface), which applies the served schema chain before handing it back, so the audit reflects exactly what clients see. Legacy wrappers (`audit-tools`, `audit-output`) exist for backward compatibility.

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

### audit_meta_descriptions

Checks the prose a meta-tool serves against the parameters its actions accept. A meta group's description enumerates its parameters by hand, and nothing connected the prose to the schemas: the text is read out of `internal/tools/testdata/tools_meta.json`, which is the golden snapshot the regenerator writes from the running server, so the description's only source is the file that records the description and `TestToolSnapshots` compares two copies of one string. Repairing an action can therefore leave the served prose offering a parameter GitLab refuses, with every gate green. This audit is the third party: it reads the served description on one side, over a real `tools/list` round-trip on the meta surface at the widest tier, and the routes' input schemas on the other.

It fails on a parameter name the description enumerates that no route of that group accepts, on an enum value it spells for a parameter whose routes publish an enum without it, on a value it spells for a parameter that publishes no enum but whose own schema description spells a value set of its own, and on a `Parameter guidance:` line written for a parameter the action it names does not accept.

That third rule exists because a JSON Schema enum is a closed set of strings, so a numeric value set cannot be one and nothing compared the two sides. `gitlab_member_role`'s `base_access_level` offered `5` in the prose while its schema description said 10 to 50, and `doc/api/member_roles.md` agrees with the schema. Both sides are extracted by one function, so the property's own description is the oracle and a parameter whose description spells no set is judged against nothing rather than guessed at.

The extraction rule and the shapes it does and does not parse are written out in the command's package comment (`cmd/audit_meta_descriptions/doc.go`). In short: a line is read as a parameter enumeration only when every comma-separated item on it parses as a parameter token, so prose and the `Returns:` block are skipped whole rather than mined for words. The report prints how many description lines it read, because a rule that suddenly reads nothing would otherwise be a silent pass.

A line whose head names exactly one action is judged against that action's own schema; a line shared by several actions, or one whose head is a wildcard, is judged against the group's pooled union, which is the only set that can hold a line whose items belong to different actions. Judging every line against the union is what let `gitlab_project` offer `pages_update` a `pages_access_level` that belongs to `project.update`, and what hid `bulk_import_start`'s flat `url` behind another action's `url` of the same name.

Skipping costs coverage, so the report names that too: every line the rule refused is counted, and `-uncovered` prints them one per line. A refusal only counts when the line's head names an action of the group, since `- Destructive: …` and `- HTTPS: …` open exactly like an enumeration and name no action, and the bullets under a `Returns:` heading describe what an action answers with rather than what it accepts. What is left is a work list a maintainer can drive to zero, and the served descriptions carry none.

#### Usage

```bash
# Report every disagreement with the line it came from
go run ./cmd/audit_meta_descriptions/

# Report, plus every action line the extraction rule refused
go run ./cmd/audit_meta_descriptions/ -uncovered

# CI gate
go run ./cmd/audit_meta_descriptions/ -check
```

#### Flags

| Flag         | Type   | Default | Description                                                                                      |
| ------------ | ------ | ------- | ------------------------------------------------------------------------------------------------ |
| `-check`     | `bool` | `false` | Exit non-zero when a served description disagrees with the schemas                               |
| `-uncovered` | `bool` | `false` | Name every action line the extraction rule refused, instead of only counting them in the summary |

#### Output

One line per finding (tool, kind, the offending name or `parameter=value`, and the description line it came from), sorted by tool then kind, then a summary line naming how many lines were read and how many were refused. Under `-uncovered`, each refused line is printed as well, marked `uncovered`. Exits `1` under `-check` when anything disagrees; a refused line is coverage the report admits to, never a failure.

#### Make targets

- `make audit-meta-descriptions` prints the report and passes `-uncovered`, since a line outside the check is where a stale parameter hides.
- `make check-meta-descriptions` is the CI gate.

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
- `make check-test-goroutines` — CI gate; also step [6/15] of `make analyze`.

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
- `make check-test-subtests` — CI gate; also step [7/15] of `make analyze`.

### audit_md_escaping

Type-checks the packages under `internal/`, finds every call that writes Markdown with a runtime value in it, and fails when a value this server did not write can reach a construct it can change the shape of. `toolutil.EscapeMdTableCell` belongs on every GitLab-authored string between two pipes of a table row and on every single-line list value, `toolutil.EscapeMdHeading` on the one value a formatter puts in a heading, and `toolutil.MdTitleLink` on both halves of a link. The containment is not only table geometry: `EscapeMdTableCell` entity-encodes `<` so GitLab-authored text cannot open raw HTML in a client that renders Markdown, and a title of `` Fix login](http://attacker.invalid/x) `` closes the label of a hand-built link and opens a destination of its own.

A sink is an `fmt` formatting call whose format argument is a constant (a literal or a named constant, both resolved by the type checker), a call of `toolutil.MarkdownTableRow` or `MarkdownTableHeader`, which have no template because every argument they take is a cell, or a call of the two `toolutil.Card` writes whose value the caller renders, `Card.Markdown` and `CardTable.Row`: every other Card method escapes what it is given, while those two write the value as given and the hole is the call site. The template is parsed with `fmt`'s own grammar, explicit argument indices included, so `[%[1]s](%[1]s)` pairs correctly; only `%s`, `%v` and `%q` are judged. The construct comes from the line the hole sits on: a leading pipe is a table cell, one to six `#` and a space a heading, a bullet or ordered marker a list item, an unclosed `[` a link label, and the text after `](` a link destination. Prose is skipped.

Each value is then followed back to where it came from: a constant, a non-textual type, an escaper, a nested `Sprintf` of safe halves, a `strings` transform of safe values, a slice allocated by `make` and built by `append` out of safe values, a helper whose every return is safe, a local whose every assignment is safe, or a parameter every caller passes a safe value to. A call binds its arguments to the callee's parameters, so a helper is judged at the call site that reaches it, which matters for `toolutil.FormatTime`: it returns its argument verbatim when neither layout parses, and it is called from about a hundred and fifty places. A parameter no call site binds is answered by every caller, and the reason names the caller that made it fail by package, file and line, since the helper's own line is not where the fix goes. A value that bottoms out at a field of a struct filled from a GitLab response is a finding; anything the walk cannot follow is reported in an unresolved bucket of its own and never counted as safe. `-fail-unresolved-in` holds the named packages to no unresolved value at all, and the gate holds `internal/toolutil` to it, since a blind spot there sits behind every formatter that calls it.

Two further rules ride the same sinks, and both gate: `check-md-escaping` names them beside `all`, which is what the flag was built for. They were staged and report-only while the card migration ran, because a gate over a backlog fails on the first push and teaches everyone to ignore it; the migration closed both to zero, so what a run reports now is a regression. The `card` rule lists every one-object card row written by hand instead of through `toolutil.Card`: a constant line opening `- **Label**:` or `- **Label**` (with or without an emoji before the label), a bullet-less `**Label**:` line, a two-cell table row whose first cell is a constant label, and the `| Field | Value |` header of a field table in its four spellings, whether written as text or built with `MarkdownTableHeader`. A row whose label is a value, an author item `- **@%s**`, a row of a table of objects and a metrics table keyed by a map key are not read as a card; the prompts and `card.go` itself are left alone. A card finding has no verdict to reach, and its directive names a function rather than a value: a hand-written row has no value of its own to name, and its text carries a colon of its own (`**Title**: %s`) that the directive grammar would cut a reason at, so the subject is the function that writes the rows, which is also the grain the claim is made at (`//gitlab:allow-card <function>: <reason>`). The case it exists for is the interactive consent prompt, whose values go through `EscapeConsentValue` and are defanged of a URL scheme on top of the code span a card writes, which moving them onto `Card` would take away. The `bool-time` rule asks a second question of every hole, what the value is rather than where it lands: a flag printed by `%t`, a boolean under a textual verb, `strconv.FormatBool` or a yes-or-no helper of the package's own, and a `time.Time` under a textual verb, a `Format` call with a layout of the formatter's own, or a string field named like an instant (`CreatedAt`, `DueDate`) printed as GitLab sent it, through the escapers the first verdict sees through. The two verdicts are independent, so a timestamp excused for escaping is still reported for display, and the raw one has its own directive, `//gitlab:allow-raw <expression>: <reason>`, stale only once the rule has run.

The sixth construct, `fence`, is the exception to "the line decides", because a fenced code block opens on one line and closes on another. One pass over each function body follows the text written to each `strings.Builder` or `bytes.Buffer` in source order, opening a block at a line starting with three or more backticks and closing it at a line starting with at least as many, and a value written while a block is open is judged as being in it whatever its own line says. There is no way to escape a value into a fence, so the fix is always `toolutil.MarkdownFencedBlock` or `toolutil.MarkdownCodeFence`, both of which measure the body; a block built that way leaves no literal backtick run in the source, so the pass sees no fence and judges nothing, which is what makes the rule self-enforcing. Two limits keep it from inventing findings: a nested block inherits the state it is entered with and hands nothing back, so a fence opened in one branch of an `if` and closed in another is not followed, and a call the pass cannot read that is handed the builder gives the state up rather than keeping it, since that call may be what writes the closing fence.

```bash
go run ./cmd/audit_md_escaping/
go run ./cmd/audit_md_escaping/ -v -json plan/md-escaping-backlog.json
go run ./cmd/audit_md_escaping/ -check
go run ./cmd/audit_md_escaping/ -check -fail-unresolved-in internal/toolutil
go run ./cmd/audit_md_escaping/ -contexts table-cell,heading -check
go run ./cmd/audit_md_escaping/ -contexts all,card,bool-time -v
go run ./cmd/audit_md_escaping/ -check ./internal/tools/issues
```

#### Flags

| Flag                  | Type   | Default | Description                                                                                                                                                                                                                                  |
| --------------------- | ------ | ------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `-dir`                | string | `.`     | Repository root to audit                                                                                                                                                                                                                     |
| `-json`               | string | _(off)_ | Write the JSON work list to this path                                                                                                                                                                                                        |
| `-contexts`           | string | `all`   | Constructs to judge: `all` (the six gating contexts), or a comma-separated list of `fence`, `heading`, `link-destination`, `link-label`, `list-item`, `table-cell` and the staged rules `card` and `bool-time`; `all` may appear in the list |
| `-check`              | bool   | `false` | Exit non-zero when a value still reaches a Markdown construct unescaped                                                                                                                                                                      |
| `-v`                  | bool   | `false` | List the excused and unresolved values as well as the failing ones                                                                                                                                                                           |
| `-fail-unresolved`    | bool   | `false` | Count a value the audit cannot follow as a failure                                                                                                                                                                                           |
| `-fail-unresolved-in` | string | _(off)_ | Count a value the audit cannot follow as a failure in these packages only: a comma-separated list of repository-relative prefixes, such as `internal/toolutil`                                                                               |

Package patterns after the flags narrow the sweep, which makes checking one domain while working on it cheap. The default is `./internal/...`.

#### Declaring that a value is already safe

Escaping a value that needs none is noise that teaches the next reader the wrong rule, so an exemption is declared in the source, in the package that owns the formatter:

```go
//gitlab:allow-unescaped result.ID: a canonical catalog ID, compiled in from an ActionSpec rather than read from GitLab.
//gitlab:allow-raw item.DueDate: a date GitLab sends without a time, shown as the day it names.
```

The expression is the one the report prints, and the directive excuses that expression wherever the package interpolates it, for the one verdict it names: an escaping exemption does not excuse a raw timestamp, and a raw one does not excuse a pipe. A directive that excuses nothing fails the gate, so an exemption cannot outlive the reason it was written for; a raw directive is judged stale only in a run that judged the `bool-time` rule.

#### Output

Findings grouped by package, each naming the file, line, formatter, construct, verb, expression, the helper it wants and how the walk got there; then a summary with the counts and a breakdown by construct. `-v` adds the excused and unresolved buckets. The JSON work list carries `findings`, `unresolved`, `excused`, `stale_directives` and a `summary` with per-construct and per-package counts; a card finding's expression is the row as the source wrote it.

#### Make targets

- `make audit-md-escaping` — report plus `plan/md-escaping-backlog.json`, with the two staged rules judged beside the gating contexts.
- `make check-md-escaping` — CI gate over the six gating contexts, holding `internal/toolutil` to no unresolved value; also step [9/15] of `make analyze`.

### audit_sdk_context

Reports every call into client-go that does not hand the SDK the caller's context.

client-go takes the context of a request only as a request option, `gl.WithContext(ctx)`, and builds the request from `context.Background()` without one. A handler that forgets the option compiles, answers correctly and passes every test that does not cancel mid-flight, while three things the context carries never reach the request it sends: the action deadline (`GITLAB_MCP_ACTION_TIMEOUT`, applied through the context by the `WrapAction` functions), the cancellation of an abandoned HTTP POST, and the parent of the outbound trace span, so the request starts a trace of its own and the e2e observation reads the action as having issued none. Fourteen calls in four packages were in that state when this rule was written, after the same defect had already been fixed once in `uploads`. `golangci-lint` runs `noctx` and `contextcheck`, and neither can see it: `noctx` knows the standard library's request constructors, and `contextcheck` follows parameters of type `context.Context`, which a variadic `RequestOptionFunc` is not.

It loads `./internal/...` and `./cmd/...` through `cmd/internal/goprogram`, **without test files**, and judges every call whose callee takes client-go request options, as a variadic `...RequestOptionFunc` or as a `[]RequestOptionFunc` the way the request builders do. The callee is read from the type checker's signature, so a service method, a method value kept in a variable or a struct field, a function-typed parameter and a helper of this repository's own are judged alike, and the name client-go is imported under does not matter. A call passes when one of its options is:

- `gl.WithContext` of a context that can end;
- a variable that was ever assigned one, which covers a slice initialized with `gl.WithContext` and appended to on a branch, and a single option held in a variable;
- a request-option parameter of the function the call sits in, or of any function around it, handed on. That is forwarding, and whoever calls that function is judged by the same rule instead, which is how the list helpers here pass a closure the service method and add the context where they call it.

A request built by hand through `NewRequest`, `NewRequestToURL` or `UploadRequest` also passes when the variable holding it is rebound through the request's own `WithContext`: `req = req.WithContext(ctx)`, or `client.Do(req.WithContext(ctx), &out)`. go-retryablehttp's own constructors are held to the same rule, because `(*gl.Client).Do` sends whatever request it is handed: `retryablehttp.NewRequest` builds from `context.Background()` and passes only when rebound, and `NewRequestWithContext` passes on a context that can end, or when rebound. The constructor is judged rather than the send, so a request a helper built, as the group board handlers build theirs, is judged where it was built and not again where it is sent; the standard library's constructors, which is where the request `FromRequest` wraps comes from, are `noctx`'s. `gl.WithContext(context.Background())` and `context.TODO()` are reported wherever they sit among a call's options, written there directly, appended or held in a variable, whatever else the call passes: client-go applies the options in order and a later `WithContext` replaces an earlier one, so one placed after the caller's context detaches the request and one placed before it does nothing while reading like the fix. The same context on the request's own method, or handed to `NewRequestWithContext`, counts as none; only the direct call is recognized, since a command's own context is usually derived from `context.Background()` by a timeout before it is used. What the walk cannot trace, options kept in a struct field, built by a function or a method, or handed over as another call's results, is treated as carrying nothing, so it fails the call only when no other option carries the context; the answer then is to pass `gl.WithContext(ctx)` beside them. The same reading is a stated limit in the other direction: a detaching option a helper builds, placed beside the caller's `gl.WithContext(ctx)`, is not seen, although client-go applies it after the caller's and the request ends unbounded.

None of these rules reads control flow. A variable counts as carrying the context if any assignment of it does, a parameter counts as forwarded even where the function reassigns it before the call, and a rebinding counts wherever it sits in the variable's scope, after the send included. That is what lets a slice appended to on a branch pass without the walk following the branch, and it is a limit rather than a guarantee: none of the shapes it would pass wrongly exists in the tree, and a fixture (`TestScan_TheRulesDoNotReadControlFlow`) holds each of them.

**Blind spots, stated rather than discovered.** Test files are not read, and that includes every end-to-end scenario package under `test/e2e/gitlab`, which are test files in their entirety. A test may build a request with no context on purpose, as the test of `testutil.CancelOnArrival` does to show what the fixture does with one, and holding tests to the rule would be a second rule with a declaration table of its own. The rest of `test/e2e`, the harness and the fixture library, is outside the patterns by decision: it runs in a test process against a real instance rather than inside a handler, nothing holds it to this rule, and not every request it makes passes a context (the harness's rate-limit setup and its state snapshot pass none). A file a build constraint leaves out of the load is not read either, and within a package the load reached that one is not silent: such a file that imports client-go is reported as unjudged and fails the gate, unless it is a test file, which would not be read under any constraint. A directory whose every file a constraint leaves out is not a package the load returns at all, so nothing is said about it. Neither kind exists under `./internal` or `./cmd` today. A call through a value whose type is a type parameter has no signature the checker can name and is not judged; none exists in the tree.

A function allowed to call client-go without the caller's context is declared in `cmd/audit_sdk_context/declarations.go`, keyed `package:Func` or `package:Type.Method`, with a category and a reason. A declaration that excuses nothing is reported like every stale declaration in this repository, scoped to the packages the run loaded, and so is one naming a category nobody defined. The table is empty.

#### Usage

```bash
# Report
go run ./cmd/audit_sdk_context/

# Report, listing the calls a declaration excuses
go run ./cmd/audit_sdk_context/ -v

# Gate
go run ./cmd/audit_sdk_context/ -check

# One package
go run ./cmd/audit_sdk_context/ ./internal/tools/packages
```

#### Flags

| Flag     | Type     | Default | Description                                                              |
| -------- | -------- | ------- | ------------------------------------------------------------------------ |
| `-dir`   | `string` | `.`     | Repository root the patterns are resolved against                        |
| `-check` | `bool`   | `false` | Exit non-zero when a call reaches client-go without the caller's context |
| `-v`     | `bool`   | `false` | Also list the calls a declaration excuses                                |

Positional arguments are package patterns; with none, `./internal/...` and `./cmd/...`.

#### Output

Every call without the caller's context with its file, its line, the call as written, the reason and the declaration it sits in; then the stale declarations, the declarations with an undefined category and the unjudged files; then a summary counting the calls judged (those that build or send a request: every call handing client-go request options, and go-retryablehttp's two constructors), the packages and how many of the clean calls rest on forwarding and on rebinding, and after them the findings and the calls a declaration excused, which are counted apart from the findings. Exits `1` under `-check` on any finding, stale or undefined declaration or unjudged file, and `1` whenever the source could not be loaded.

#### Make targets

- `make audit-sdk-context`: the report, verbose.
- `make check-sdk-context`: CI gate, also step 22 of `make analyze`.

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

- `make check-supply-chain` — CI gate; also step [8/15] of `make analyze`.

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

It introspects a live instance, converts the answer to SDL, and writes `gitlab-schema.graphql` beside `source.json`, which records the instance, the version it reported and the day it answered so a reader can tell how old the pin is without asking git. GitLab answers introspection to anyone, so no token is needed for the schema itself; `GITLAB_TOKEN` is read so the record can name the version, which GitLab refuses to tell an anonymous caller, and `--check` refuses a pin that records none.

The SDL is committed as text, not gzipped. A re-pin is the one moment somebody needs to read what GitLab changed, and git stores two revisions of the text in about half the pack it needs for two gzip streams, which it can neither delta nor diff. The schema never reaches a released binary, so nothing weighs the other way.

The introspection and the conversion live in `cmd/internal/graphqlintrospect`, shared with `audit_graphql_documents -live`, which fetches a schema the same way rather than through a second copy of the same quirks: the canned payload every instance answers whatever it was asked, the deprecation arguments an older release refuses, and the wrapper depth a type reference nests.

The conversion emits objects with their `implements` clauses, interfaces, unions, enums, input objects, custom scalars, argument and input defaults, and the `schema { query mutation subscription }` block. Built-in scalars and `__`-prefixed types are omitted, because gqlparser's own prelude already defines them and emitting them again fails the load with a duplicate definition. Everything nameable is sorted, so a re-pin produces a diff of what changed rather than a reshuffle. Descriptions and deprecation reasons are dropped: validation never consults them and they would triple the file.

Nothing is written until the converted schema has been loaded back through the same parser that will judge every document, so a renderer that dropped an `implements` clause fails here rather than by refusing a valid document months later.

Generating needs the network, so it is not a gate. `--check` is, and it asks more than whether the bytes parse: it refuses a record naming an instance other than `https://gitlab.com/api/graphql`, one carrying fewer than 4000 types, one whose GitLab version is blank or `unknown`, one whose retrieval date is unreadable or in the future, and one retrieved longer ago than the window in [`cmd/internal/provenance`](#cmdinternalprovenance). It also refuses a record whose type count is not the count the schema beside it loads with, which is the check that makes the two files one pin: a truncated schema that still parses, committed next to a record from a whole one, passed everything else. The record therefore carries the loaded count rather than the introspected one, since that is the only number `--check` can recompute from disk; the two are equal for a real GitLab schema anyway, because the renderer omits the built-in scalars and the `__` types and the loader's prelude puts the same thirteen back. A `-url` pin is therefore a diagnostic, not a commit: it narrows the gate's promise to that instance, and a Community Edition instance narrows it below the surface this server registers, so `--check` names it rather than accepting it in silence. A generation that would produce such a pin warns on stderr as it writes, so the person who ran it learns from the command rather than from CI an hour later.

The window that last refusal measures against, and the reason for its length, are [`cmd/internal/provenance`](#cmdinternalprovenance)'s and are stated once there. What is this command's own is the way out of it: to catch a narrowing on the day it happens rather than at the next re-pin, use `make check-graphql-documents-live`.

#### Usage

```bash
# Re-pin from gitlab.com (GITLAB_TOKEN set, so the version is recorded)
go run ./cmd/gen_graphql_schema/

# Probe a self-managed instance; a pin written this way will not pass --check
go run ./cmd/gen_graphql_schema/ -url https://gitlab.example.com/api/graphql -dir /tmp/probe

# CI gate, no network
go run ./cmd/gen_graphql_schema/ --check
```

#### Flags

| Flag     | Type     | Default                          | Description                                                                                                              |
| -------- | -------- | -------------------------------- | ------------------------------------------------------------------------------------------------------------------------ |
| `-url`   | `string` | `https://gitlab.com/api/graphql` | GraphQL endpoint to introspect                                                                                           |
| `-dir`   | `string` | `internal/graphqlschema`         | Directory holding the pinned schema and its provenance record                                                            |
| `-check` | `bool`   | `false`                          | Load the committed schema instead of fetching one, and fail when it does not parse or is not a current pin of gitlab.com |

#### Output

Writes `gitlab-schema.graphql` and `source.json` into `-dir`, and reports the type count and provenance on stdout. `--check` writes only the summary. Exits `1` on any failure, so a half-written artifact never reaches a branch.

#### Make targets

- `make gen-graphql-schema`
- `make check-graphql-schema` — CI gate.

### gen_api_live

Boots a released GitLab image, asks the loaded application what its own REST API is, and writes the answer to `docs/development/gitlab-api-live.json`.

It replaced two records that were both readings of text: the OpenAPI document GitLab generates from its Grape definitions, and a scan of that Grape source for the condition each field is sent under. Both were downstream of the object that actually decides what a request returns, and both lost the same thing, a name that is not written down. `GeoSiteStatus` exposes its fields by iterating a constant assembled from two method calls; the source says "expose the loop variable", a scanner read 26 fields, and the instance sends 606. That is not a hole a better parser closes.

What comes back is every `API::Entities` class the instance had loaded, keyed by its Ruby name and holding the exposures in declaration order with everything it inherits and everything an Enterprise module prepended already flattened in; every route Grape had mounted, with the entity its `desc … success/entity` annotation names and the parameters it declares; and the licensed feature table, mapping each feature symbol to the tier that unlocks it. A Grape condition is a Proc, which knows where it was written and not what it says, so the script reads those lines back from inside the same image: a condition arrives both located and quoted, and the file it was written in is what says whether it is Enterprise.

An exposure declared with `merge: true` is marked as such, and `apilive` resolves it into the keys it contributes rather than into a key of its own. It sends its child's keys on the parent object and no key named after itself, so an exposure list read literally says the opposite of what GitLab sends, in both directions at once: with `API::Entities::Member` merging `UserBasic`, the record claimed a member carries a `user` object and said nothing about the `id`, `username` and `name` it really carries, and R-PATH reported the one key GitLab never sends as missing from our output and the nine it does send as invented by us. Marking them removed 34 such phantom findings and uncovered 16 real gaps that were hidden underneath. The instance has 14 merged exposures, and six of them merge a value that renders with no entity, whose keys nothing static can name: those contribute nothing, because contributing their own name would be the one answer certain to be wrong.

The boot is a generator and never an audit. It needs Docker and takes a few minutes on a cold image and about forty seconds afterwards; every gate downstream reads the committed record with no Docker and no network, which is the only way a gate can be one. It needs no licence and no fixtures either, because a licence gates `feature_available?` when a request is served and not when a class is defined.

The wait before the introspection asks the database a question, not just Rails. A GitLab loads its Rails environment before its migrations have created the tables, so a probe that only proves the environment loaded reports ready on a container whose first query raises `relation "application_settings" does not exist`; two five-minute boots were spent that way. The probe asks for a table the application always has, which is the same ground the run afterwards covers. A runner that fails anyway now reports the tail of its stderr, since only stdout is the answer and discarding the rest left an exit status and nothing to act on.

Floors refuse a boot that half ran, before anything is written: fewer than 400 entities, 5000 exposed fields, 1500 routes or 150 licensed features, or any entity that refused to describe itself. An introspection that half ran does not fail, it returns less, and written down that record says GitLab stopped sending things while every audit downstream reports the difference as a gap in this server.

Thirty-four of the 1425 annotated routes name an entity the record does not hold, and none of them is a defect in GitLab. Eight annotate `File`, which is how GitLab marks a route that answers with bytes rather than an object: the raw file read, the four upload downloads, the repository snapshot, a Terraform module and a Terraform state version. The other twenty-six name classes outside the `API::Entities::` namespace this walk collects, which are the `GitlabSubscriptions::API::Entities::Internal` family, the VS Code settings entities, and the Rails serializers that render a few responses (`ProjectEntity`, `TestReportEntity`, `Vulnerabilities::FindingEntity`). The first group is a fact about the route and belongs in the record as one; the second is this walk's own limit and closes by collecting every `Grape::Entity` subclass rather than filtering on the namespace.

#### Usage

```bash
# Re-take the record from the pinned image
go run ./cmd/gen_api_live/

# Another image, kept up afterwards for a look inside
go run ./cmd/gen_api_live/ -image gitlab/gitlab-ee:19.4.0-ee.0 -keep

# Write the raw introspection without wrapping it, or read one back
go run ./cmd/gen_api_live/ -dump /tmp/introspect.json

# Build the record from a dump taken on another machine, keeping its provenance
go run ./cmd/gen_api_live/ -dump /tmp/introspect.json -digest sha256:b516…

# CI gate, no Docker
go run ./cmd/gen_api_live/ -check
```

#### Flags

| Flag      | Type     | Default            | Description                                                                   |
| --------- | -------- | ------------------ | ----------------------------------------------------------------------------- |
| `-image`  | `string` | the pinned release | GitLab image to boot                                                          |
| `-dump`   | `string` | _(empty)_          | Read an introspection already on disk instead of booting, or write one to it  |
| `-digest` | `string` | _(empty)_          | With `-dump`, the repository digest of the image that introspection came from |
| `-keep`   | `bool`   | `false`            | Leave the container running after the run, for a look inside a failed boot    |
| `-dir`    | `string` | `docs/development` | Directory holding the committed record                                        |
| `-check`  | `bool`   | `false`            | Read the committed record instead of booting, and fail when it is not usable  |

`-digest` exists for the split this record sometimes has to be taken across: booting a GitLab wants several gigabytes, so the boot may happen on one machine and the record be built in the checkout on another, and only the first of the two can ask Docker what the image really was. A tag moves and a digest does not, so a record built from a dump without one names the weaker half of its own provenance.

#### Output

Writes `gitlab-api-live.json` into `-dir` and reports the entity, field, route and feature counts with the image, its digest and the day. `-check` reports the provenance line only, and exits `1` on a schema version this build cannot read, a count below any floor, missing provenance, or a record past the window in [`cmd/internal/provenance`](#cmdinternalprovenance).

#### Make targets

- `make gen-api-live`
- `make check-api-live` — CI gate; also step [14/15] of `make analyze`.

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

### gen_request_inventory

Merges the request shards the unit suite records into `docs/development/request-inventory.json`, the committed answer to what this server actually sends GitLab.

The 1:1 audit has five dimensions and every one of them describes the surface: the fields we accept, the fields we return, the actions we register, the discovery metadata we attach, the enum values we advertise. All five compare what we publish with what the SDK and the API documentation offer, and not one looks at the request a handler builds. That is how nine registered tools shipped unable to work, with a perfect input struct, a perfect output struct, a registered action, complete enums, and a request GitLab refuses. This is the first half of the missing dimension: recording it.

`internal/testutil.NewTestClient` does the recording, so the 6444 clients the suite already builds became the instrument at no cost to the tests themselves. Each records the method, the path with its identifiers replaced by placeholders, how many distinct values each placeholder has stood for, the query parameter names, the top-level field names of a JSON request body, and for GraphQL the operation and the variables the document declares. Recording is off unless `GITLAB_MCP_TEST_INVENTORY_DIR` names an absolute directory, and one test process writes one shard, so package binaries running in parallel never contend. A client built by `internal/testutil`'s own tests records nothing: those fixtures exercise this harness's mock and are not requests this server sends GitLab.

A row is one package, method and templated endpoint, carrying the union of the parameter, body-field and variable names that package was seen sending it. Two calls that differ only in an optional filter are the same endpoint, so the union answers which names were sent and deliberately not which of them were sent together: a combination is a property of the fixtures, and a file that recorded them would be read as a contract. That is why `gitlab_get_catalog_resource` could send GitLab `id` and `full_path` together for a release while this file listed both names on one row.

The body-field names matter more than they look. Without them 97% of the mutating rows carried a method and a path and nothing else, which is where a wrong field name lives, and the artifact still read as an answer to what this server sends.

A row also carries `identifiers`: per placeholder of its path, how many **distinct** raw values the recorder templated away. It is the one thing the fold discards that a reader needs back, because the fold is also what hides a hard-coded identifier: with a single fixture value, a handler that reads the caller's project and a handler with that id written into it produce the same row, which is why that class had to be found by hand in five packages. The count is running rather than final, since the recorder is process-global with no shutdown hook to write a total from: a new value costs one more line carrying the higher count, and the merge takes the highest of a row's lines rather than their sum, because the lines are a running total of one set. The values themselves never leave the test process, on the same terms the query and body names do. A count of one is a lead and never a verdict, and reading it is R-PATH's business, not this command's.

The path rule reads the shape of a segment and never a list of names, because a list would have to be kept in step with a thousand actions. A segment is an identifier when it is all digits, when it carries a percent-encoded slash (at either escaping depth), when it is hexadecimal and long enough to be a commit, or when it is shaped like a UUID. The placeholder is named after the collection segment in front of it, so `/projects/1/issues/2/notes` is `/projects/:project_id/issues/:issue_id/notes`: the placeholders used to be positional, `:id` for the first identifier and `:iid` for the rest, and that made `:id` name the project on one row and the board on the next whenever the project was spelled as a fixture word the rule cannot recognize.

What the rule cannot catch is an identifier that looks like a word: a branch named `main`, a wiki slug, a CI variable key, a project addressed as `my-project`, all stay in the path verbatim, so one endpoint reached with three of them is three rows. That limit is left visible rather than papered over. Templating by the parent segment instead, so that anything after `projects` or `groups` is the project or group, was measured against the recorded requests rather than assumed: `projects` is also followed by the literals `import`, `shared` and `user`, `groups` by `import` and `shared`, `packages` by `generic`, `npm` and `ml_models`, `snippets` by `all` and `public`, `runners` by `all`, `verify` and `reset_registration_token`, and `personal_access_tokens` by `self`. Each of those is a different endpoint from the one with an identifier in that position.

The attribution is the package that built the client and the test that built it, never the action, and the reason is where the recording sits rather than a law about what can be known. Nothing on the wire names an action, and at the moment a request is observed nothing on the stack does either, because the `httptest` server answers on its own goroutine while the test goroutine that called the handler is blocked out of sight. A `RoundTripper` on the client would see that goroutine, since an outgoing request is dispatched on the caller's own; what it would name is a Go function, and the catalog's route for an action is a closure over its handler, so turning a frame into an action ID means recording the handler's function identity on the spec, in production code, for a test artifact. The honest coarse attribution was worth more than a mapping that is a guess. The committed artifact keeps the package and drops the test name, so adding a test that reaches an endpoint already listed does not change it; the shard keeps the test name, so a row can still be traced back.

The committed recording is a Linux one, made as root on a filesystem with symlinks. Nothing in the suite currently issues a request only under one of those conditions, and one test that did was renamed to publish a file name another test already sends, so today the artifact is machine-independent. A regeneration on another platform may legitimately differ, and a test that skips conditionally must not be the only producer of a row.

The summary this prints counts actions whose owning package recorded nothing, which is weaker than "this action was never exercised" in two ways worth knowing: it is package-grained, and a package that declares specs whose handlers live elsewhere is counted silent even though the handler's own package recorded the request. `internal/tools/adminspecs` is the whole of that case today, and it is why the silent list is read package by package rather than action by action.

What this does not do is judge. It says what we send, not whether GitLab would accept it: comparing these paths with GitLab's own documentation is the check that follows, and a response our output struct misreads can only be caught by a real instance.

#### Usage

```bash
# Record a run, and nothing else
make record-request-inventory

# Record the suite and rewrite the artifact
make gen-request-inventory

# Gate: verify the artifact against shards already recorded
go run ./cmd/gen_request_inventory/ -check

# Name every package the catalog owns actions in that issued no request
go run ./cmd/gen_request_inventory/ -v -check
```

#### Flags

| Flag      | Type     | Default                                   | Description                                                                        |
| --------- | -------- | ----------------------------------------- | ---------------------------------------------------------------------------------- |
| `-shards` | `string` | `dist/request-inventory`                  | Directory holding the recorded shards, absolute or relative to the repository root |
| `-out`    | `string` | `docs/development/request-inventory.json` | Committed inventory path                                                           |
| `-check`  | `bool`   | `false`                                   | Verify the committed inventory is current without writing it                       |
| `-v`      | `bool`   | `false`                                   | Name every package the catalog owns actions in that recorded nothing               |

#### Output

The artifact on disk, and a three-line summary on stderr: how many rows, distinct paths and packages the inventory holds, when the shards it merged were written, and how much of the catalog the recording could see. Exits non-zero when the shard directory holds no shard, when a shard cannot be read, and in `-check` mode when the committed artifact is not what the shards say it should be. An empty shard directory is an error rather than an empty inventory, because writing that would erase the artifact and report the whole file as a change; so is a directory that is not there at all, which is what a checkout that has recorded nothing yet has, and the refusal names the target that would record one.

The recording time is on the summary because this command never records. It merges whatever run last left shards, so a comparison is a statement about that run, and about the working tree only when the recording was made from it.

#### Make targets

- `make record-request-inventory`: runs the suite with recording on and leaves the shards. The minutes live here, and a suite that fails deletes the shards and says so, so the next step refuses rather than merging a partial run.
- `make gen-request-inventory`: records, then rewrites the artifact.
- `make check-request-inventory`: merges the shards of the last recorded run and gates instead of writing. It runs no suite of its own, so it costs one `go run`; recording is the opt-in half. CI gets both for nothing: it sets `GITLAB_MCP_TEST_INVENTORY_DIR` on the coverage job's suite run and merges those shards.
- `make audit-request-inventory`: the same gate, naming the silent packages.

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
go run ./cmd/gen_testing_docs/ --check

# Refresh the counts without recomputing coverage, keeping the recorded values
go run ./cmd/gen_testing_docs/ -skip-coverage

# Verify the coverage values as well, which takes minutes
go run ./cmd/gen_testing_docs/ --check -skip-coverage=false

# Give a slow package more room than the 30 minutes each go test run gets
go run ./cmd/gen_testing_docs/ -timeout 45m
```

`-skip-coverage` carries the coverage already recorded in the document forward
instead of blanking it, so it is a real refresh of everything else and, with
`--check`, a freshness gate that holds everything the source tree determines:
the counts, the naming breakdown, the per-layer tables, and the set of packages
in the coverage tables, which is what caught `cmd/audit_install_buttons` missing
from them. It takes seconds, because it runs no coverage at all.

`--check` implies it. The cheap answer is the one a check means, and it used to
depend on the caller remembering the flag: `go run ./cmd/gen_testing_docs/
--check` on its own ran the whole coverage pass and compared numbers this
document deliberately does not gate. Measuring under a check is still
available, and now says so: `--check -skip-coverage=false`.

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
| `-skip-coverage`   | `bool`     | `false`, `true` under `-check`        | Skip the `go test` coverage run and keep the values already recorded     |
| `-timeout`         | `duration` | `30m`                                 | Per-package timeout handed to each `go test` run                         |
| `-top-tool-rows`   | `int`      | `25`                                  | Number of high-test-count tool sub-packages to show in the summary table |

#### Output

Rewrites the managed sections of `docs/development/testing/testing.md`.

#### Make targets

- `make gen-testing-docs` to regenerate, `make check-testing-docs` to verify.
  The check runs in `make audit-docs` and in the CI `Test` job, beside the
  other generated-artifact gates.

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

# Fairness: does a bound leave the quiet tenant better off?
go run ./cmd/bench_resources/ -fairness tools-call-rps
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

#### The fairness mode

`-fairness <bound>` replaces the matrix with a different question. Every scenario above starts the server with the limiter off and drives every credential the same way, so none of them can see fairness: a harness where every caller behaves alike has no quiet neighbor to protect, and with no bound in force there is nothing to protect it. This mode runs two populations of credentials against one server, twice, and reports the quiet one's experience with the bound in force and without it.

It writes its own document to `bench/fairness.json`, which is not committed, and draws no chart, so it touches neither the published record nor the artifacts `-check` compares. Rendering waits on the chart rework rather than adding figures that are about to be redrawn.

Six things about it are load-bearing, and each is a way the report would otherwise mislead:

- **Both populations are open loop.** A refusal returns in about two milliseconds against tens for a served call, so a closed-loop driver would send several times as many requests in the arm with the bound on and the arms would be different experiments. The schedule is computed before the phase and is identical in both.
- **Served and refused are never one number.** Four terminal outcomes per population and per method (served, refused, failed, timed out), held to `served + refused + failed + timed out == dispatched` in code, with separate latency distributions that no field merges. A refused request also never enters a served percentile.
- **Processor time is published per served request**, never per request, beside the counts and the saturation reading. A per-request figure falls by an order of magnitude the moment a bound refuses anything, which would present not doing the work as doing it cheaply.
- **Every percentile is quoted with its survivorship.** The distributions are over served requests alone, so what they mean depends on how much of the population reached them: a repetition in which less than three quarters of the quiet population's dispatched requests completed is refused rather than compared, and the share that did completes every sentence the verdict can be quoted from.
- **The harness is measured too, and can disqualify its own result.** The driver shares the host with the server, and with the bound in force it parses a two-kilobyte refusal where it parsed a hundred-and-seventy-kilobyte result, so it hands the host back in exactly the arm that is supposed to look better. Both processes are sampled, and a claim that the quiet tenant improved is refused when the driver handed back at least as much of the machine as the server did.
- **The verdict can say no.** `better`, `worse`, `indistinguishable` and `not comparable` are all first-class. It refuses to call a change an improvement when the quiet population was itself refused, when the arms did not offer the server the same work, when the host was never contended for (a per-credential bound can only reach the quiet tenant through contention), or when the driver's own dispatch lateness moved between the arms by as much as the difference being claimed. Two answers need no comparison at all and are given before any: the bound refusing a quiet request, and the quiet population abandoning more requests with the bound in force in every repetition.

| Flag                   | Type       | Default               | Description                                                                                                 |
| ---------------------- | ---------- | --------------------- | ----------------------------------------------------------------------------------------------------------- |
| `-fairness`            | `string`   | `""`                  | Bound to measure: `tools-call-rps`, `tools-list-rps` or `listen-streams`                                    |
| `-fairness-json`       | `string`   | `bench/fairness.json` | Document to write; refused if it names the published record                                                 |
| `-fairness-surface`    | `string`   | `dynamic`             | Tool surface the run drives                                                                                 |
| `-fairness-quiet`      | `int`      | `8`                   | Credentials in the quiet population                                                                         |
| `-fairness-noisy`      | `int`      | `4`                   | Credentials in the noisy population                                                                         |
| `-fairness-quiet-rate` | `float`    | `0`                   | Requests per second each quiet credential offers; `0` takes a quarter of what the bound meters, capped at 2 |
| `-fairness-noisy-rate` | `float`    | `20`                  | Requests per second each noisy credential offers                                                            |
| `-fairness-phase`      | `duration` | `20s`                 | Measured window per arm                                                                                     |
| `-fairness-lead-in`    | `duration` | `5s`                  | Unmeasured window before it, which drains the bound's burst                                                 |
| `-fairness-deadline`   | `duration` | `2s`                  | How long a request may take from its intended dispatch before a client would have given up                  |
| `-fairness-repeats`    | `int`      | `2`                   | How many times the pair of arms runs, alternating their order                                               |

The defaults are sized for a modest host and take a few minutes. A larger one needs a noisier population rather than a longer phase, since the answer depends on the host actually being contended: `-fairness-noisy=16 -fairness-quiet=32 -fairness-phase=60s -fairness-repeats=4`. Raising the noisy population past a few dozen credentials needs the file-descriptor limit raised with it, since a credential may hold `rate x deadline` requests outstanding. With `-binary` the mode needs no checkout at all, so a prebuilt driver and server measure it on a host that has neither the repository nor a Go toolchain.

The quiet rate is left to the bound for a reason. What counts as quiet is a fact about the bound and not about this command: the shipped bucket meters ten requests a second and the listing bucket derived from it meters one, so a single default that leaves the first a tenfold margin would sit exactly on the second, and the run would report on a tenant the bound was itself turning away. A rate given explicitly is refused when it reaches half of what the bound meters, counting only the verbs the bound looks at.

The mode is refused together with `-render` and `-check`, which draw the committed artifacts and measure nothing: it is a measurement whatever else was asked for, and combining them ran two servers for minutes while claiming to verify a drawing.

`tools-list-rps` measures the bound [PR 566](https://github.com/jmrplens/gitlab-mcp-server/pull/566) adds and stops the run on a build that does not meter listings, rather than reporting a bound that was never there as one that helped nobody. Its bucket is derived rather than configured: `--rate-limit-rps=10` puts it in force but listings refill a tenth as fast, so the rate a noisy population has to exceed is one a second per credential, and the lead-in that drains the burst is computed from that rather than from the flag. `listen-streams` is declared but refused by name: it bounds a held resource rather than a rate, and this driver has no verb that opens a stream and keeps it open.

With `-no-render` and `-binary` the driver reads nothing from a checkout, so a prebuilt driver and server can measure the series on a host with no Go toolchain; the record is then copied back and rendered with `-render`. The server is started with `--pprof-addr` on a loopback port the driver picks, which is where the per-step profiles and goroutine counts come from.

#### Output

The measurement record, the SVG chart pairs under the two chart directories, and the generated blocks of the three documentation pages; and, for the series, one CPU and one heap profile per step under the profiles directory, which git ignores. A full run takes several minutes for the point scenarios, since every one builds a tool catalog per client, and then as long as the host's memory lets the series run.

#### Make targets

- `make bench-resources` — measure and render.
- `make bench-resources-render` — redraw from the committed record; what to run after changing a figure.
- `make check-bench-resources` — CI gate; seconds, since no benchmark is run.

## Evaluation

The evaluation itself is not a command. It is `test/e2e/modeleval`, which boots
a GitLab and drives the real binary over stdio, so a run measures the surface a
client is actually served. The two commands here are its generators: one says
what the corpus asks about before any run, the other turns what a run observed
into what is published.

### gen_model_corpus

Renders the corpus breadth ledger: which catalog actions, domains and tiers the
corpus asks about, counted against the action catalog this tree builds, so a
reader can see what a published figure does and does not cover.

**Make targets:** `make gen-model-corpus`, `make check-model-corpus`.

### gen_model_results

Turns what a run observed into what this repository publishes. A run writes
observation and no verdict, so every number is computed here, from the shards
and from the corpus at HEAD, at the moment a run is folded in. That is what
lets a scoring rule corrected today re-score a past run without spending a
token: `-refold` drops the rows the given shards publish and folds them again,
naming each drop. A second fold of a run already published is refused by name
rather than replacing it, and a run whose shards were not kept cannot be
re-scored at all, which is the reason to keep them.

```bash
go run ./cmd/gen_model_results/ -shards dist/modeleval/ce -render         # fold a run in and redraw
go run ./cmd/gen_model_results/ -shards dist/modeleval/ce -refold -render # re-score it under today's rules
go run ./cmd/gen_model_results/ -render                                   # redraw from the record alone
go run ./cmd/gen_model_results/ -check                                    # the offline gate
```

**Make targets:** `make gen-model-results`, `make model-results-record`, `make model-results-refold`, `make check-model-results`.

## Server

### server

The main `gitlab-mcp-server` MCP binary — the runtime entry point and the only `cmd/` binary that ships to users. See [CLI Reference](../reference/cli.md) for the full CLI reference and [configuration.md](../reference/configuration.md) for environment and configuration details.

**Make targets:** `make build` (builds `./dist/gitlab-mcp-server`), `make run` (builds and runs locally).

## Shared packages

None of these is a command. They are the libraries under `cmd/internal/` that the commands above share, documented here because a generator's output is decided as much by them as by its own flags.

### cmd/internal/mcpsurface

The one reader of the served MCP surface. It answers what the server registers — the individual, meta and dynamic tool listings, the resources, the resource templates and the prompts — over a real MCP round-trip against an in-process stub client, so a generator describes what a client receives rather than what a maintainer believed.

Three properties are the reason it exists as one package rather than a helper per command:

- **The surface is pinned, never read from the environment.** Each constructor takes its surface and tier as arguments and talks to `NewStubClientWithToken`, so a developer machine with `GITLAB_MCP_TOOL_SURFACE=individual` or a `GITLAB_URL` exported generates the same artifact CI checks. A new environment-sensitive input belongs here, pinned once for every caller, not at a call site.
- **`Session` applies the served schema chain.** `LockdownInputSchemas` then `EnrichPaginationConstraints`, in the order `cmd/server` installs them, because a listing that applies neither measures a schema no client ever receives: without `additionalProperties: false`, with the jsonschema `,required` tag suffixes still in the descriptions, and without the page/per_page bounds.
- **A truncated listing stops the run.** Every listing is checked for a next cursor and panics on one instead of describing a partial surface as the whole of it. That was the failure mode the readers this package replaced had: they paginated no further than the first page and said nothing.

Listings are memoized on (client, surface, tier, meta parameter-schema mode), since registering a full surface costs seconds and every caller only reads the result. Callers must not sort the returned slice in place.

### cmd/internal/auditshared

The analysis helpers shared by the auditors: the projected individual-tool descriptions (a projection over `mcpsurface.IndividualTools`), owner-package resolution, the usage and description quality checks that `cmd/audit_1to1` R-META and `cmd/audit_discovery_completeness` both apply, and `NewStubGitLabClient`, the offline client the eight audit commands construct — a thin delegation to `mcpsurface.NewStubClientWithToken` so that the audits and the generators share one definition of what "no instance, no credentials" means.

### cmd/internal/testsource

The three questions every command that reads `_test.go` files used to answer for itself: whether a function name is a Go test entry point (`IsTestFunction`), which naming bucket it falls in (`ClassifyTestName`, with the four `Pattern*` constants), and which files a scan of the tree may look at (`WalkFiles`, with one `SkipDir` list).

All three had drifted, and the drift was not theoretical. `gen_stats` required an upper-case rune after `Test` while `gen_testing_docs` required a non-lower-case one, under a comment claiming the two agreed; `audit_test_names` skipped every name starting with the `TestMain` prefix, so every test named `TestMain_Something` was counted by both generators and invisible to the auditor. Go's own rule decides for all of them, which makes the reconciliation a fix to the auditor rather than a change to either published count. The walks disagreed the same way: two skip lists and two descents that skipped nothing, so whether `testdata` is part of the corpus had two answers and no recorded reason. `SkipDir` is that answer, written down once, and `audit_test_names` applies it in its `-check-files` gate as well as its report, so the gate certifies the corpus the report describes.

A root is entered whatever it is called, so a scan pointed straight at a fixtures or dot directory scans it, and whatever it is: `filepath.WalkDir` lstats its root, so `WalkFiles` resolves a root that is a symlink to a directory before walking it and reports every path back under the name the caller gave. Without that, a tree named through a link is handed to the callback as a plain file, a report comes back empty and `-check-files` certifies it clean. Below the root nothing is resolved; a link that resolves to nothing is a read error rather than an empty corpus.

`WalkFiles` stops at the first error and returns it, whether the walk raised it (an absent root, a directory the process may not read) or the visitor did, and there is deliberately no best-effort mode. Every caller is a gate or the input to one, and a gate that skipped an unreadable directory would certify a tree it never read; the files gathered before such an error are a prefix of the tree and look exactly like the whole of it. A caller that wants to continue past a failure swallows it inside its own visitor, where it can say which file it gave up on. What no caller may do is discard the returned error, because by then the walk has already stopped and a truncated report reads exactly like a complete one.

What the package deliberately does **not** own is discovery. `gen_stats` keeps asking git (`git ls-files`) so `check-stats` stays a function of what is committed, and `gen_testing_docs` keeps enumerating packages through `go list` because it describes packages; sharing the input universe would break both.

One predicate also stays where it is. `cmd/godoc_tool` asks which functions need a test-form doc comment rather than which functions the testing package runs, so it keeps `TestMain` and the lower-case `Test`-prefixed helpers that `IsTestFunction` excludes; routing it through the shared predicate would silently drop those findings from `make audit-docs`.

### cmd/internal/goprogram

The go/packages front end for the gates that type-check this repository's own source. Nine packages load through it: [`audit_md_escaping`](#audit_md_escaping) walks the calls that interpolate a GitLab-authored value into Markdown, [`audit_readonly_graphql`](#audit_readonly_graphql) walks the calls a read-only action can reach, `cmd/internal/graphqldocs` folds every raw GraphQL document to the one string GitLab would receive, [`audit_graphql_shapes`](#audit_graphql_shapes) pairs each of those documents with the struct that decodes it, [`audit_action_ids`](#audit_action_ids) folds the action IDs the server publishes to a model, [`audit_catalog_first`](#audit_catalog_first) resolves the calls that aggregate each package's ActionSpecs, [`audit_dead_consts`](#audit_dead_consts) holds every unexported constant to something that reads it, [`audit_sdk_context`](#audit_sdk_context) holds every call into client-go to the caller's context, and [`audit_e2e_coverage`](#audit_e2e_coverage)'s static check reads the end-to-end test packages. It owns `LoadMode`, `Load(dir, patterns, overlay)`, `LoadWith` for a load that needs test variants or build tags, and the refusal of a package that did not type-check; the indexers, the detectors, the questions and the binaries stay with each gate.

`LoadMode` deliberately omits `NeedDeps`, for one reason that holds for every caller: each gate only ever reads bodies written inside the patterns it loads, so type-checking the dependency tree from source would cost minutes and change no answer, while export data still gives every dependency object the identity the packages using it see.

The refusal is why this is a package rather than a copy per gate. Each gate answers "cannot tell" for what it cannot resolve, and a partially typed package resolves nothing: the escaping audit would classify every value as unfollowable, the read-only audit would find no handlers, and the document collector and the shape audit would fold no constants, so each would report a clean run over source it never understood. The first four gates wrote the rule four times with four wordings, and a change to it was a four-file edit with one file easy to forget.

The `overlay` parameter is not a convenience. It is how the tests of six of the nine (`audit_action_ids`, `audit_dead_consts`, `audit_md_escaping`, `audit_readonly_graphql`, `audit_sdk_context` and `cmd/internal/graphqldocs`) supply a fixture package written in the test file itself, type-checked against the real packages it imports, so the classifiers are exercised on the shapes they have to handle rather than on a mock of them. Production passes `nil`, and so do the tests of the other three, which write their fixtures to disk: `audit_graphql_shapes` and `audit_catalog_first` a module in a temporary directory, `audit_e2e_coverage` the planted trees under its `testdata`.

`cmd/audit_1to1/internal/shared.LoadToolPackages` is deliberately not folded in. Its mode is now this one less `NeedCompiledGoFiles`, which it has no reader for, but its contract is not this one: it refuses more widely than `Load` does, collecting every error of every package it loaded and aborting on all of them at once where `Load` stops at the first error of a package the caller asked for; it returns a subset rather than what it loaded, keeping the packages under `internal/tools` and dropping the rest; and it memoizes that result per root. That is a different contract, not a different wording of this one.

It did load with `NeedDeps`, and dropping it applied the reasoning above to the one loader that had not taken it. Nothing in the audit reads a dependency's syntax — it asks client-go for struct fields, json tags and service method signatures, all of which export data carries — and `go/packages` marks the _immediate_ dependencies of every source package as needing types regardless of `NeedDeps`, which is what keeps `ClientGoTypes` and `structs.clientGoDir` finding client-go through the import graph. Type-checking the tree from source cost about half of every load and changed no answer: all six reports come out byte for byte identical.

### cmd/internal/docgen

The Markdown renderers the generators share (`RenderMarkdownTable`, and `ReplaceSection` / `ComputeReplacedSection` for a managed region of a hand-written document), plus the two ways a command puts bytes on disk. Those two are kept apart on purpose, because they answer different questions:

- **`WriteOrCheck(path, content, check, regenerate)`** is the whole-file freshness convention for a committed artifact, and the one place its decisions are taken: a write creates the parent directory (a check never does, so a gate reports a missing tree rather than making one), the file is written through an `os.Root` opened on that directory so the write can only land on the named file, content is given the trailing newline that keeps a generated file from being the one text file in the repository without one, the comparison ignores carriage returns so a Windows checkout does not report drift a Linux one cannot see, and a stale artifact is reported with one sentence naming the file and the command that refreshes it. The mode is `0o600`: seven of the eight callers already used it, it is what gosec's G306 accepts without a suppression, and it only ever applies to a file the generator creates from nothing, since neither `os.WriteFile` nor `Root.WriteFile` changes the mode of a file that is already there.
- **`WriteReport(path, content)`** is the `-` means stdout convention for an auditor's `-output` flag. Nothing there is committed and nothing is compared, and the path is the operator's own, so it is contained to no directory.

Merging the two behind one signature with a mode flag is the one way to make this worse than the copies it replaced, which is why there are two functions. The managed-section helpers stay separate for the same reason: what is generated there is a region, and the rest of the file is somebody's prose.

`NormalizeNewlines` is exported for one reason: the commands whose artifacts `WriteOrCheck` writes hold the same bytes to the same rule in their own tests (`gen_llms` against the six committed files, `audit_metrics` against the committed `stats.json`), and private copies of that one line in each test file are the drift this package exists to stop.

Half of one `-` writer stays where it is. [`audit_edition_tier`](#audit_edition_tier) writes its stdout branch to a writer the caller injects, which is how its tests read that branch back without swapping `os.Stdout`; folding it in would mean giving up that seam or giving `WriteReport` a writer parameter no other caller has a use for. Its file branch is the shared helper's, which also gave it the missing parent directory it did not create before.

### cmd/internal/provenance

The age verdict passed on a committed record of something that lives in `gitlab-org/gitlab`: the retrieval date's arithmetic (`Age`, `Days`), the default clock the `--check` halves share (`Clock`), the three ways a date stops a record being one a gate can rest on (`Problems`), and the one staleness window with its one recorded reason (`MaxAge`).

Two commands pin such a record — [`gen_api_live`](#gen_api_live) what a booted GitLab says its REST API is, [`gen_graphql_schema`](#gen_graphql_schema) the GraphQL schema — and each has the same shape for the same reason: generating needs a network or a container so it cannot gate, `--check` gates precisely because it needs neither, and a check that runs offline can prove the record is readable, whole and provenanced while proving nothing about whether it still matches the GitLab it was taken from. The window is the answer to that gap, and it is an answer about GitLab's release cadence rather than about either record: a reason to widen or narrow it moves both. It had been written down three times with three copies of its justification and a comment in one pointing at its twin.

Why 180 days: GitLab ships monthly and narrows fields in place, so half a year is roughly six releases of drift — long enough not to ambush an unrelated change often, short enough that a narrowing is noticed within a release cycle or two. A record older than that is a gate that has quietly stopped asking, and the only honest way to say so is to fail. This paragraph and `MaxAge`'s comment are the whole of that decision, and moving the window means editing those two: the number appears nowhere else in the repository, and everything that mentions the gate — the three command sections here, the gate table in [Static analysis](static-analysis.md), the staleness paragraphs in [GraphQL Integration](../concepts/graphql.md) and the `cmd/` tree in `CLAUDE.md` — says only that a record past the window is refused, and points here for how long that window is and why.

What stays with each command is what makes its record its own: its `Source` type, its floor (`MinimumOperations`, `MinimumEntities`, `MinimumTypes`), the identity checks that ask what the record is a record of, its artifact dialect, its make targets and its binary. `Subject` carries the two words that differ between the three messages — the noun (`record`, `pin`) and what this particular record can no longer report — so each sentence in CI output is still about one artifact.

Two nearby commands are deliberately not members. [`gen_request_inventory`](#gen_request_inventory) commits no provenance at all: its `-check` is byte equality against a fresh recording of the unit suite, so there is no date to judge and no window to share. [`audit_graphql_documents`](#audit_graphql_documents) owns no record either; it reads the pinned schema's, and calls `Age` only to say how long ago the pin was taken in its drift report, where an unreadable date stays silent rather than becoming a second complaint about a field `check-graphql-schema` already refuses.

### cmd/internal/golist

The row shape two commands ask `go list` for and read back (`Format`, `PackageInfo`, `ParseRows`), and the one answer to which `go` binary they run (`Executable`).

Two commands enumerate this module's packages from the toolchain: [`godoc_tool`](#godoc_tool) lists `./...` because it audits every package's doc comments, and [`gen_testing_docs`](#gen_testing_docs) lists `./cmd/...`, `./internal/...` and `./test/e2e/...` with the e2e build tags because it describes those packages. Both need the same three fields — the directory to read the files out of, the import path to name the package by, and the package clause's own name, which neither of the other two implies — and both had written the template, the tab-separated parse, the `unexpected go list row` refusal and the struct for themselves, in a different field order each.

`Executable` is the reason this is a package rather than two tidy copies. Neither command may resolve `go` through `PATH`, because a lookup in a directory list the environment controls is what Sonar's `go:S4036` refuses; the function joins it out of `GOROOT` instead, appends the Windows suffix, and carries the `//nolint` comment and the `runtimeGOOS` seam that makes the Windows branch reachable from a Linux test. Written twice, that is a rule that holds until one copy is edited by somebody who did not read the other.

Running the command is deliberately not shared. `godoc_tool` wants one listing's stdout under a 30-second bound; `gen_testing_docs` runs `go test` and `go tool cover` through the same runner, which pins `GOTOOLCHAIN` to the `go` directive of `go.mod` and merges stderr into the output so a failure is reported with its tail — which is also why its warning rows reach `ParseRows`, and why refusing a row that is not exactly three fields matters rather than being pedantry. [`gen_stats`](#gen_stats) is not a member and cannot become one: it discovers packages through `git ls-files` so that `check-stats` stays a function of what is committed, which is a different universe rather than a different parse.

## CI gate targets

The following utilities expose a verification mode (`--check` or `-check`, or an invariant/error exit) that CI runs to guard against drift. The combined documentation gate is `make audit-docs`, which chains markdownlint, the table formatter, the llms, LobeHub-manifest, testing-docs and site-stats checks, the local-link check, the godoc, surface-quality and alias audits, and the site's own `check`, `build` and `lint`.

| Make target                              | Utility                                                          | What it gates                                                                                                                                                                                      | Exit behavior                                                                                                                                                       |
| ---------------------------------------- | ---------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `check-action-catalog-manifest`          | `gen_action_catalog_manifest`                                    | Generated ActionSpec manifest is current                                                                                                                                                           | Non-zero if the manifest is stale                                                                                                                                   |
| `check-llms`                             | `gen_llms`                                                       | `llms.txt` and `llms-full.txt` are current and structurally valid                                                                                                                                  | Non-zero if either file is stale or malformed                                                                                                                       |
| `check-lhm-manifest`                     | `gen_lhm_manifest`                                               | `lhm.plugin.json` declares the registered tools, prompts, and resources                                                                                                                            | Non-zero if the manifest is stale                                                                                                                                   |
| `check-footprint`                        | `audit_tokens -footprint`                                        | README token-footprint section, `docs/development/token-footprint.md` and `site/src/data/token-footprint.json` are current                                                                         | Non-zero if any is stale                                                                                                                                            |
| `check-stats`                            | `gen_stats`                                                      | README repository-statistics section is current                                                                                                                                                    | Non-zero if the section is stale                                                                                                                                    |
| `audit-discovery-check`                  | `audit_discovery_completeness`                                   | No META-001 finding meets the configured severity threshold                                                                                                                                        | Non-zero if any finding meets `-severity` (default error)                                                                                                           |
| `audit-doc-coverage-check`               | `audit_doc_coverage`                                             | No `docs/reference/tools/*.md` has missing/orphan/tier_mismatch findings                                                                                                                           | Non-zero if any file has a finding                                                                                                                                  |
| `audit-godocs-check`                     | `godoc_tool audit`                                               | No package, symbol, or test Godoc findings remain                                                                                                                                                  | Non-zero when findings are present                                                                                                                                  |
| `audit-dynamic-aliases`                  | `audit_dynamic_aliases`                                          | No error-severity alias governance finding (collisions, ambiguity)                                                                                                                                 | Non-zero (`1`) if any error-severity finding exists                                                                                                                 |
| `audit-docs` → `format_md_tables -check` | `format_md_tables`                                               | All Markdown pipe tables are normalized                                                                                                                                                            | Non-zero if any table needs formatting                                                                                                                              |
| `check-testing-docs`                     | `gen_testing_docs`                                               | The `docs/development/testing/testing.md` test-metrics block is current                                                                                                                            | Non-zero if the generated section is stale                                                                                                                          |
| `check-supply-chain`                     | `audit_supply_chain`                                             | The five release-configuration invariants still hold                                                                                                                                               | Non-zero if any is broken, or if the audit cannot be run                                                                                                            |
| `check-doc-tool-names`                   | `audit_doc_tool_names`                                           | Every `gitlab_*` name the documentation mentions is registered on some surface                                                                                                                     | Non-zero if any name is unregistered                                                                                                                                |
| `check-gateway-chars`                    | `audit_gateway_chars`                                            | Nothing served carries a character a gateway validator rejects                                                                                                                                     | Non-zero if any offender is served                                                                                                                                  |
| `check-meta-descriptions`                | `audit_meta_descriptions -check`                                 | Every parameter and value a served meta-tool description offers is one its actions accept                                                                                                          | Non-zero if a description and the schemas disagree                                                                                                                  |
| `check-install-buttons`                  | `audit_install_buttons`                                          | Every install button decodes and agrees with the others for its command                                                                                                                            | Non-zero on a problem, or when no button is found                                                                                                                   |
| `check-test-goroutines`                  | `audit_test_goroutines`                                          | No `testing.T` abort is made off the test goroutine                                                                                                                                                | Non-zero if any abort site exists                                                                                                                                   |
| `check-test-subtests`                    | `audit_test_subtests`                                            | No case loop asserts without a `t.Run` subtest                                                                                                                                                     | Non-zero if any site remains                                                                                                                                        |
| `check-test-file-names`                  | `audit_test_names -check-files`                                  | Every `_test.go` is named after a module it tests                                                                                                                                                  | Non-zero on a violation                                                                                                                                             |
| `check-md-escaping`                      | `audit_md_escaping -check -fail-unresolved-in internal/toolutil` | No value reaches a Markdown table cell, heading, list item or link unescaped, none lands inside a hand-written code fence, and `internal/toolutil` holds no value the audit cannot follow          | Non-zero on a finding, a directive that excuses nothing, or an unresolved value in toolutil                                                                         |
| `check-site-stats`                       | `audit_metrics -site-stats -check`                               | `site/src/data/stats.json` is current                                                                                                                                                              | Non-zero if the file is stale                                                                                                                                       |
| `check-bench-resources`                  | `bench_resources -check`                                         | The committed benchmark charts and tables match the committed record                                                                                                                               | Non-zero if they are stale                                                                                                                                          |
| `brand-check`                            | `gen_brand --check`                                              | The committed brand assets match the geometry                                                                                                                                                      | Non-zero on drift                                                                                                                                                   |
| `check-icon-webp`                        | `gen_icon_webp --check`                                          | The committed WebP icons match `icons.go` (needs `rsvg-convert` and `cwebp`, so not run in CI)                                                                                                     | Non-zero on drift                                                                                                                                                   |
| `check-readonly-graphql`                 | `audit_readonly_graphql`                                         | No action classified ReadOnly can reach a GraphQL mutation                                                                                                                                         | Non-zero on any finding, or if the audit cannot be run                                                                                                              |
| `check-dead-consts`                      | `audit_dead_consts`                                              | Every unexported constant in `internal/` and `cmd/` is read by something, and every entry of its declaration table excuses one                                                                     | Non-zero on any unread constant or stale declaration, or if the source cannot be loaded                                                                             |
| `check-sdk-context`                      | `audit_sdk_context -check`                                       | Every call into client-go in `internal/` and `cmd/` library code hands the SDK the caller's context, and every entry of its declaration table excuses one                                          | Non-zero on any call without the context, stale or undefined declaration, or build-constrained non-test file importing client-go, or if the source cannot be loaded |
| `check-graphql-schema`                   | `gen_graphql_schema --check`                                     | The committed GitLab schema parses and its provenance record decodes                                                                                                                               | Non-zero if either file is missing or unusable                                                                                                                      |
| `check-graphql-documents`                | `audit_graphql_documents`                                        | Every raw GraphQL document in the source is one the pinned GitLab schema accepts                                                                                                                   | Non-zero on any refusal, or if no documents are found                                                                                                               |
| `check-graphql-shapes`                   | `audit_graphql_shapes`                                           | Every struct a GraphQL response is decoded into can hold what its document selects, and declares nothing it never selects                                                                          | Non-zero on any disagreement, anything unpaired, a mutation payload whose errors no field of the decoder reads, a stale sent declaration, or if no call is found    |
| `check-request-inventory`                | `gen_request_inventory -check`                                   | The committed request inventory is what the last recorded run issued                                                                                                                               | Non-zero if the artifact is stale or no run has recorded any shard                                                                                                  |
| `audit-1to1-paths`                       | `audit_1to1 -scope=paths`                                        | Every action's owning package was seen issuing a request, every GraphQL document is one the pinned schema accepts, and the shape, pagination and client-go document comparisons report beside them | Non-zero on a refused document, an undeclared silent package, or a stale declaration                                                                                |
| `audit-1to1-paths-e2e`                   | `audit_1to1 -scope=paths -e2e-calls dist/e2e-calls`              | The same, with the observation question asked per action from the shards a Docker end-to-end run recorded                                                                                          | Non-zero on the same findings; the per-action report gates on nothing                                                                                               |
