# Command-Line Utilities Reference

> **Diátaxis type**: Reference · **Audience**: 🛠️ Contributors & maintainers

The `cmd/` directory contains the developer tooling binaries that power audits, code generation, formatting, and the documentation pipeline for this project. They are **not** part of the runtime MCP server, with one exception: `cmd/server` is the server entry point itself.

Every utility can be run directly with `go run ./cmd/<name>/ [flags]`, or through the convenience Make targets documented per section. Several binaries also expose a `--check` (or `-check`) mode that validates generated output without writing it; these are wired into CI gates (see [CI gate targets](#ci-gate-targets)).

## Quick reference

| Utility                        | Category                      | Purpose                                                                                                                                                                                             | Make target                                                         |
| ------------------------------ | ----------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------- |
| `audit_1to1`                   | SDK/API parity audits         | Consolidated SDK↔API parity audit (struct/action/metadata gap streams, plus the `sdk` service and raw-GraphQL gate)                                                                                 | `make audit-1to1`                                                   |
| `audit_catalog_first`          | Catalog & metadata audits     | Source-discovered ActionSpec catalog-first coverage inventory                                                                                                                                       | `make audit-catalog-first`                                          |
| `audit_discovery_completeness` | Catalog & metadata audits     | Extended META-001 model-discovery metadata quality auditor                                                                                                                                          | `make audit-discovery`                                              |
| `audit_doc_coverage`           | Catalog & metadata audits     | Per-doc-file gaps vs the action catalog (DOC-002)                                                                                                                                                   | `make audit-doc-coverage`                                           |
| `audit_doc_tool_names`         | Catalog & metadata audits     | Every `gitlab_*` tool name the documentation mentions is one some surface registers                                                                                                                 | `make check-doc-tool-names`                                         |
| `audit_dynamic_aliases`        | Catalog & metadata audits     | Dynamic-toolset alias governance (collisions, ambiguity)                                                                                                                                            | `make audit-dynamic-aliases`                                        |
| `audit_e2e_gaps`               | Catalog & metadata audits     | Catalog actions the e2e suite never exercises                                                                                                                                                       | `make audit-e2e-gaps`                                               |
| `audit_edition_tier`           | Catalog & metadata audits     | Doc-grounded licensing tier (Free/Premium/Ultimate) vs binary gating                                                                                                                                | `make audit-edition-tier`                                           |
| `audit_graphql_documents`      | Catalog & metadata audits     | Every raw GraphQL document in the source is one the pinned GitLab schema accepts; `-live` judges by what an instance serves now and reports the drift under our own documents                       | `make check-graphql-documents`, `make check-graphql-documents-live` |
| `audit_graphql_shapes`         | Catalog & metadata audits     | Every struct a GraphQL response is decoded into can hold what its document selects and declares nothing the document never selects                                                                  | `make check-graphql-shapes`, `make audit-graphql-shapes`            |
| `audit_readonly_graphql`       | Catalog & metadata audits     | No action classified ReadOnly can reach a GraphQL mutation                                                                                                                                          | `make check-readonly-graphql`                                       |
| `audit_surface_quality`        | Surface quality audits        | Consolidated MCP tool surface quality audit (metadata + output)                                                                                                                                     | `make audit-surface-quality`                                        |
| `audit_gateway_chars`          | Surface quality audits        | Served descriptions and titles carry no character an MCP gateway validator rejects                                                                                                                  | `make check-gateway-chars`                                          |
| `audit_meta_descriptions`      | Surface quality audits        | Every parameter and value a served meta-tool description offers is one its actions accept, whether the value set is a schema enum or one a schema description spells                                | `make check-meta-descriptions`                                      |
| `audit_tokens`                 | Surface quality audits        | LLM context-window overhead of every tool/resource/prompt definition; `-footprint` regenerates the README token-footprint section                                                                   | `make audit-tokens`, `make gen-footprint`                           |
| `audit_metrics`                | Surface quality audits        | Comprehensive metrics summary (tools, resources, prompts, codebase); `-site-stats` writes the site's stats JSON                                                                                     | `make audit-metrics`, `make gen-site-stats`                         |
| `gen_graphql_schema`           | Generators                    | Pins a GitLab GraphQL schema by introspecting a live instance; `--check` gates the committed one                                                                                                    | `make gen-graphql-schema`, `make check-graphql-schema`              |
| `gen_api_shapes`               | Generators                    | Pins what GitLab's own generated OpenAPI document says each REST operation accepts and returns; `--check` gates the committed record                                                                | `make gen-api-shapes`, `make check-api-shapes`                      |
| `gen_api_exposes`              | Generators                    | Pins, from GitLab's Ruby source, the condition under which each field a REST entity exposes is sent and the license tier it belongs to; `-check` gates the record, `-report` prints an entity       | `make gen-api-exposes`, `make check-api-exposes`                    |
| `godoc_tool`                   | Source quality audits         | Godoc compliance auditor and fixer (audit + fix subcommands)                                                                                                                                        | `make audit-godocs`                                                 |
| `audit_test_names`             | Source quality audits         | Classifies `Test*` functions by naming pattern; emits rename hints; `-check-files` gates test-file naming                                                                                           | `make audit-test-names`, `make check-test-file-names`               |
| `audit_test_goroutines`        | Source quality audits         | `testing.T` aborts made off the test goroutine                                                                                                                                                      | `make check-test-goroutines`                                        |
| `audit_test_subtests`          | Source quality audits         | Case loops that assert without a `t.Run` subtest; `-fix` rewrites the unambiguous ones                                                                                                              | `make check-test-subtests`                                          |
| `audit_md_escaping`            | Source quality audits         | Values a Markdown formatter interpolates into a table cell, heading, list item or link without an escaping helper                                                                                   | `make check-md-escaping`                                            |
| `audit_string_dupes`           | Source quality audits         | Finds duplicated string literals missing `const`/`var` declarations                                                                                                                                 | —                                                                   |
| `audit_supply_chain`           | Release & supply-chain audits | Five release-configuration invariants: pinned actions, credentialed jobs that run no run-time-resolved code, stated Dependabot cooldowns, a current security policy, signature-verifying installers | `make check-supply-chain`                                           |
| `audit_install_buttons`        | Release & supply-chain audits | Decodes every one-click install button and holds the buttons to one configuration per command                                                                                                       | `make check-install-buttons`                                        |
| `gen_action_catalog_manifest`  | Generators                    | Generates the ActionSpec group-builder manifest                                                                                                                                                     | `make gen-action-catalog-manifest`                                  |
| `gen_lhm_manifest`             | Generators                    | Generates the tools/prompts/resources arrays in `lhm.plugin.json` (LobeHub Marketplace)                                                                                                             | `make gen-lhm-manifest`                                             |
| `gen_llms`                     | Generators                    | Generates `llms.txt` and `llms-full.txt`                                                                                                                                                            | `make gen-llms`                                                     |
| `gen_request_inventory`        | Generators                    | Merges the requests the unit suite records into `docs/development/request-inventory.json`                                                                                                           | `make gen-request-inventory`                                        |
| `gen_stats`                    | Generators                    | Regenerates the managed repository statistics section in `README.md`                                                                                                                                | `make gen-stats`                                                    |
| `gen_testing_docs`             | Generators                    | Regenerates the test-metrics block in `docs/development/testing/testing.md`                                                                                                                         | `make gen-testing-docs`                                             |
| `gen_docker_tools`             | Generators                    | Generates a Docker MCP Registry-compatible `tools.json`                                                                                                                                             | —                                                                   |
| `gen_brand`                    | Generators                    | Emits every vector brand asset from one parametric geometry                                                                                                                                         | `make brand`, `make brand-check`                                    |
| `gen_icon_webp`                | Generators                    | Rasterizes the SVG icons into light/dark WebP fallbacks (maintainer-only)                                                                                                                           | `make gen-icon-webp`                                                |
| `format_md_tables`             | Formatters                    | Normalizes Markdown pipe tables in `README.md`, `docs/` and `site/src/content/docs/`                                                                                                                | part of `make audit-docs`                                           |
| `bench_resources`              | Benchmarks                    | Measures what the server costs to run (memory, startup, a second credential), draws the published charts, and measures whether a bound leaves a quiet tenant better off                             | `make bench-resources`, `make bench-fairness`                       |
| `eval_mcp_surfaces`            | Evaluation                    | Evaluates model behavior across MCP tool surfaces                                                                                                                                                   | `make eval-surfaces-docker*`                                        |
| `server`                       | Server                        | The main `gitlab-mcp-server` MCP binary (runtime entry point)                                                                                                                                       | `make build`, `make run`                                            |

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

This rule reads the request instead, out of `docs/development/request-inventory.json`, the inventory `internal/testutil` records and [gen_request_inventory](#gen_request_inventory) commits. Four checks:

- **Has the path ever been observed.** An action whose owning package issued no request at all has never had its request seen by anything, which is exactly the state the broken documents were in. Held at package grain, because nothing on the wire names an action, and the report says so beside the number (`actions_observed_grain`): 990 of 1082 observed means 990 actions whose owning package issued some request, not 990 actions whose own request anybody has seen. As a regression guard it is real; as per-action assurance it is nothing. A package may nevertheless be silent for a reason, and `internal/tools/adminspecs` is the whole of it today: it declares specs whose handlers live in other packages, so its requests are recorded under the package that makes them. Such a package is held to a declaration with a category and a reason in `declaredSilentOwners` (`cmd/audit_1to1/internal/paths/declarations.go`), the way `-scope=sdk` holds a client-go service to one, and a declaration that no longer describes the tree is itself a finding. An action whose owner names no package fails too: nothing validates that field, and such an action used to be classified unmapped, which the gate ignored and `-gaps-only` dropped, so it could be neither counted nor seen. The owner `tools` is the catalog's own exception, naming the orchestration package rather than a domain under it, and resolves to `internal/tools`; an owner that names a real package and the wrong one is a lie no version of this check can catch.
- **Does the document validate.** Every raw GraphQL document in the source, against the pinned schema. The reading and the judging are `cmd/internal/graphqldocs`', shared with the standalone gate [audit_graphql_documents](#audit_graphql_documents), so there is one answer to whether a document is one GitLab would refuse. This judges the document and never the values sent with it, which is the other half of the same defect family: of the nine tools that could not work, four sent a document the schema refuses and five sent an accepted document carrying a value GitLab does not have. The second half is caught by the validating transport in `internal/testutil`, which checks the variables with the document on every request a test drives, and neither check substitutes for the other.
- **Does the endpoint exist.** `-check-endpoints` compares every recorded REST endpoint with GitLab's own API documentation, and **fails on an endpoint no declaration accounts for**. The declarations are what make that safe, because the oracle is prose: 57 documentation lines omit the leading slash, `emoji_reactions.md` gives the note reactions one plaintext block for issues and leaves the merge request and snippet variants to a sentence, `usage_data.md` documents `/usage_data/track_events` only in prose and a curl example, `attestations.md` writes its endpoint lines without the `projects` scope its own curl example shows, the generic package registry and Terraform state are documented outside `doc/api` entirely, and the `/services/` alias for the integrations endpoints, which client-go still uses, is not documented at all. Each of those shapes is written down in `declaredUndocumentedEndpoints` (`cmd/audit_1to1/internal/paths/endpoint_declarations.go`) with a category and a reason, and a declaration that stops matching anything is a finding of its own, so the excuse cannot outlive the thing it excuses. The listing of pages comes from the repository tree rather than `api_resources.md`, because 101 of the 253 pages under `doc/api` are not linked from that index or from `rest/_index.md`, and following every link out of the pages they do list still leaves 77 unreached. Against the full 247 readable pages the comparison finds 82 undocumented endpoints out of 1378 recorded, all of them declared; against the index alone it would report roughly five times as many, nearly all about where the index stops. A hole in the corpus produces candidates that are only about the hole, so the report counts the pages it could not read.

- **Does GitLab say it sends what we publish.** The fourth check reads `docs/development/gitlab-api-shapes.json`, the record [gen_api_shapes](#gen_api_shapes) pins from the OpenAPI document GitLab generates out of its own Grape entities. It is the first rule here whose oracle is GitLab rather than client-go or a documentation page: the other five compare us against what the SDK models, which is a second model of the API and not the API. It reports and never gates, and it asks its question at two grains, kept side by side so a reader can see which findings the sharper one keeps. What the record says is an upper bound rather than an answer, because Grape renders a conditional expose only when the route passes the option its `if:` names, and the generator that writes the record cannot see the condition. A field in the record therefore proves that the entity can render it and nothing about a given route, so a decision about one endpoint's response is settled by reading the entity's condition or a live answer, which is how the epics domain came to publish `subscribed` and `reference`: both are in the record, and neither is ever sent on the routes that domain calls.

  At **package grain** (`shapes.unpublished`) it joins the inventory and reports every `*Output` field under `internal/tools` that no endpoint its package was recorded calling declares in a response. The inventory names a package, so the fields of every endpoint a package calls are unioned before the comparison, which makes the check exact for a package with one endpoint and weaker as the package grows; an operation the document gives no response schema for (553 of the 1847) contributes nothing, silences its package rather than condemning it, and is left out of the `endpoints_searched` a finding carries, which counts searched responses and means the same thing at both grains; and a nested output type is left out entirely here, since a nested type is only comparable against the object it sits under, which the type grain does and this grain cannot. Every one of those choices loses findings and none of them invents one. It reports 635 fields across 130 packages, and most of them are not phantoms: `ListOutput.users` and `SSHKeyListOutput.keys` are our own wrappers around a JSON array the document describes by its element; `DeleteOutput.deleted` and `userNotFoundOutput.identifier` are our own answers to a 204 and to a not-found, which no endpoint sends because they are not an endpoint's response; and `files.Output` is reported for its 12 fields only because the endpoint behind it declares no response schema while some of the other 29 endpoints `files` calls do, so the union is non-empty and the package is not silenced. The join itself is reported (`shapes.join`) because a package whose rows all miss would otherwise have every field of its output condemned by a lookup failure, and the literal path segments our own fixtures left untemplated are reported beside it (`shapes.untemplated`), which measures the recorder rather than the server: 894 of 1479 REST rows match exactly, 504 more only once a fixture value such as `mygroup` or `myproject` is accepted where GitLab has a placeholder, and 81 match nothing.

  At **type grain** (`shapes.typed`) it compares a type against the response of the operations that type actually models, and only those. The chain touches the inventory nowhere, which is the point, since the inventory records a package by construction and cannot be sharpened: `structs.CollectOutputPairings` gives the client-go struct a converter fills each output type from, out of the same pairing pass the R-OUTPUT field diff runs over; `readSDKRoutes` parses the SDK source the handlers compile against and returns, per struct, the endpoints the service methods answering with it reach, reading the `route()` templates and the `withMethod` options and reproducing client-go's own template normalisation so the two spellings meet; and the document supplies the response. Of 441 top-level output types it compares 26 and reports 11 fields. The other 415 are counted rather than judged: 401 that no converter pairs with a client-go struct, which is where the wrappers and the synthetic results go; those whose struct no service method answers with, so there is no endpoint to ask about, a count kept though it reads zero today; and 14 whose every route the document either does not carry or carries with no response schema, an empty union meaning the document does not say rather than that GitLab sends nothing. A type's published fields are the names encoding/json would write, an untagged embed's promoted into it, and a type is nested when any struct of its package names it as a tagged field's type, output type or not; an embed names nothing nested, since its fields are promoted rather than placed under a key, and the embedded type stays a response of its own (`shapes.typed.nested_unpublished` therefore never holds a type reached through an embed). The one package a domain package takes a shape from, `internal/toolutil`, is read once and resolved wherever a package names one of its shapes as a field's type, embeds one, or aliases one under its own name (`type NoteOutput = toolutil.NoteOutput`), the hints type aside since its next steps are the server's and not GitLab's; an alias is nested when the shape is named under either name or by another shared shape, unless an exported function of the package returns it, which is what a handler does with the note it adds to a discussion. Those rules were learned from the first list this produced: a details type embedding the row type was reported missing every field it promoted, and the user under the row of a list of uploads, reached through a plain struct, was held to the endpoints answering with a whole user. A match here is exact only: the loose lookup the package grain leans on accepts a literal segment of ours where GitLab has a placeholder, which is evidence about a fixture value in the inventory and would be a guess against a route template, whose placeholders are already placeholders. The phantom of [issue 580](https://github.com/jmrplens/gitlab-mcp-server/issues/580) is the case the join was built against and the fixture that proves it: `mrapprovals.ConfigOutput` as it stood before the fix produces exactly the twenty findings the fix removed, and as it stands now produces none.

  Since schema version 2 of the record, the same grain also asks about **nested output types** (`shapes.typed.nested_unpublished`): a type reached through a field of a compared type is held against the properties the document gives that field, one level down and no further. A nested type whose property the record describes no object for is not compared at all, which is the same reticence that skips a type with no response schema and is what keeps this level usable: 21 nested types compared, 25 fields reported. None of the 25 carries a declaration yet, and `summary.typed_undeclared_fields` spans both levels, so it reads 25 today; the three top-level findings are answered. They sit in `internal/tools/issuelinks` and `internal/tools/pipelinetriggers`, and adjudicating one means reading its API page first. The two discussion packages were here too, beside the 8 top-level fields the shared note shape published that GitLab's note does not carry: the notes review removed those and read the four the shape lacked from the captured response (ADR-0021), which is what a finding at this grain is for. One level is deliberate. The record grew from 1.1 MB to 1.5 MB carrying it, which is still a diff a reviewer reads, and each further level multiplies that by the branching of GitLab's schemas rather than adding to it.

  A finding at type grain can be **answered rather than fixed**, because the oracle is generated and is not always complete: an endpoint that renders a bare hash gets no schema worth comparing against, and a nested property can be given a narrower entity than the endpoint renders. `cmd/audit_1to1/internal/paths/shape_declarations.go` is where such a finding is written down with a category and the evidence, on the terms every other declaration table in this audit works on: a declaration that matches nothing is itself reported as stale. Today it holds one entry, for `invites.InviteResultOutput`, whose POST GitLab answers with `{"status": "success"}` while the generated document carries the pending-invitation object of the GET at the same path.

  The same two grains ask the **reverse question** as well, since a record that speaks for GitLab can say what GitLab sends that we do not publish. `shapes.sent.unsurfaced` (package grain) and `shapes.typed.unsurfaced` (type grain) list every response field the searched operations declare that the package, or the type, does not publish, and each finding carries what `docs/development/gitlab-api-exposes.json`, the record [gen_api_exposes](#gen_api_exposes) pins from GitLab's Ruby source, says about when GitLab sends it: `sent` is `always` for a field the entity exposes with no condition, `when` for one behind an `if:` or `unless:`, recorded beside it with the license tier and edition when the condition names a licensed feature, and `unknown` when no operation carrying the field names a component for its response, the conditions record does not hold the component, or the component names its fields at run time (`expose(*helper.attributes)`, which the record marks as a splat). The component a field is read on is the one named by the first operation carrying that field, per field rather than per type or per package, because the responses of one type's operations resolve to different components: the fingerprint lookup of a key is documented as answering with a user, and reading the key's own fields on that component left the three the key type lacks unknown. The summary carries the three counts at each grain (`unsurfaced_fields`, `unsurfaced_sent_always`, `unsurfaced_sent_when` and their `typed_` twins); the unknown remainder is the difference. Neither grain gates: a field GitLab sends that this server does not surface is a candidate for the 1:1 surface, and the type-grain list, which names the output type and the operations it models, is what the field-by-field review reads. What a package publishes, for this direction, is the json name of every exported struct of the package that is not an input, the inner ones included, since the row of a list is nested under the list and is exactly what the list endpoint sends; the other direction judges the top-level types alone, for the reason given above. The package grain over-reports in one known way, since a package that calls an endpoint for something other than surfacing its answer (`health` reads `/user` to learn who the token is; `projectdiscovery` reads `/projects/{id}` to resolve a path) is reported as missing that answer's every field.

  A finding here can be **answered rather than surfaced** too, on the terms of the other direction: GitLab's generated document lists, for some operations, the response the operation's description names rather than the one its handler presents, and a field of that response is a defect of the document rather than a gap in the surface. `cmd/audit_1to1/internal/paths/sent_declarations.go` records such an answer with a category and the source that says so, keyed by the component the finding was read on, since a finding carries its operations per type and its component per field: the fingerprint lookup of a key (`GET /keys`) is described as answering with `UserWithAdmin` and `lib/api/keys.rb` presents a key, so the user fields the document lists at the top level are declared, while the key fields the same type lacks are not; and the add-a-member `POST /invitations` is described as answering with an `Invitation` and returns the `status` and `message` pair, the same thing the shape declaration for the other direction records. A declared finding keeps its `sent` answer and gains a `category` and a `reason`, the summary counts them apart (`unsurfaced_declared`, `typed_unsurfaced_declared`), and a declaration that accounts for no finding is reported stale and fails the gate like every other declaration table here.

The first two checks need no network and no suite run: the inventory is committed, the schema is pinned, and the catalog is compiled in. That is what makes them a CI gate. The fourth needs no network either, since the API record is committed like the inventory, and it runs with them; it contributes no gate outcome. Its type grain does cost the typed load of `./internal/tools/...` that the other scopes already pay for, memoized per root and so free to a run that has done it, and it reads the client-go source out of the module cache the build resolved rather than fetching one. The third needs 250 pages over the network, so it runs only when asked for, and nothing schedules it today: a declaration going stale is noticed the next time somebody runs `make audit-1to1-paths-endpoints`.

Report keys: `schema_version`, `inventory`, a `summary` block (27 counters, `actions_observed_grain` among them), `graphql_refusals[]`, `silent_owners[]`, `stale_declarations[]`, an `endpoints` block that says whether the documentation comparison ran, how many pages it read, which it could not, the endpoints no page spells out with the declaration that accounts for each, and the declarations that accounted for none, and a `shapes` block carrying the join quality, the untemplated segments most frequent first, the package-grain unpublished fields, and a `typed` block with the type-grain counts and findings. Every finding of either grain carries a `grain` key naming the join that produced it, and a type-grain one also names the client-go struct and the operations searched. With `-gaps-only` `silent_owners[]` holds the undeclared and the unmapped ones and `endpoints.undocumented[]` holds the undeclared ones; `shapes` is unaffected, because every entry in it is already a finding.

#### Make targets

- `make audit-1to1` — writes `plan/1to1-backlog.json`, then runs `audit-1to1-sdk`.
- `make audit-1to1-sdk` — the SDK parity gate, enum values included; fails the build on a finding.
- `make audit-1to1-enums` — the enum value rule alone; fails the build on a finding.
- `make audit-1to1-paths` — the request-path gate (R-PATH); fails the build on a finding. No network.
- `make audit-1to1-paths-endpoints` — the same plus the documentation comparison, written to `plan/1to1-paths.json`; fails on an endpoint no declaration accounts for. Needs the network.
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

#### Usage

```bash
# CI gate
go run ./cmd/audit_graphql_shapes/

# Also list every pairing judged and every selection nothing reads
go run ./cmd/audit_graphql_shapes/ -v

# Judge against an SDL file already on disk
go run ./cmd/audit_graphql_shapes/ -schema /tmp/live/gitlab-schema.graphql
```

#### Flags

| Flag      | Type     | Default   | Description                                                                             |
| --------- | -------- | --------- | --------------------------------------------------------------------------------------- |
| `-dir`    | `string` | `.`       | Repository root to audit                                                                |
| `-v`      | `bool`   | `false`   | List every pairing judged and every selection nothing reads, not only the disagreements |
| `-schema` | `string` | _(empty)_ | SDL file to judge the documents against, instead of the pinned schema                   |

#### Output

One block per pairing with a disagreement on stderr, naming the package, the constant the document is declared as, the call, and, when the call received the document through a wrapper, where it was handed over; every finding sits under it with the response path it is about, from `data` down. Every call that could not be paired is one stderr line of its own. Under `-v`, stdout carries an `ok` line per clean pairing and a block per pairing whose only findings are selections nothing reads. The summary line says which schema judged them. Exits `1` on any disagreement, on anything unpaired, on a source tree that cannot be loaded, and when no call at all was found, since an audit that found nothing to judge is broken rather than satisfied.

#### Make targets

- `make check-graphql-shapes`: the CI gate.
- `make audit-graphql-shapes`: the same gate, listing everything it judged.

## Surface quality audits

### audit_surface_quality

Consolidated MCP tool surface quality audit. It combines metadata-quality checks (naming, annotations, schema shape, duplicates — formerly `audit_tools`) and output-quality checks (`OutputSchema`, Returns/See-also, Title — formerly `audit_output`) behind a single `-view` flag.

Both views judge the surface as a client receives it, because both list it through `cmd/internal/mcpsurface`: the meta view therefore includes `gitlab_server`, and every schema it inspects has been through the lockdown and the pagination bounds. `make audit-docs` runs the command once with the default `-view=all` rather than once per view, since one listing now serves both.

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
- `make check-test-goroutines` — CI gate; also step [7/15] of `make analyze`.

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
- `make check-test-subtests` — CI gate; also step [8/15] of `make analyze`.

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
- `make check-md-escaping` — CI gate; also step [10/15] of `make analyze`.

### audit_string_dupes

Scans non-test Go source for string literals appearing often enough (default: three or more times) and long enough (default: three or more characters) that are not already `const`/`var` values.

A directory argument is walked through `cmd/internal/testsource`, so the corpus is the shared one: the two tracked non-test Go files under `testdata` (`cmd/bench_resources/testdata/standin/main.go` and `cmd/server/testdata/peer/main.go`) are fixtures rather than source this repository holds to its conventions, and their literals are no longer reported. A file named directly is always scanned.

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

#### Exit code

`0` when every path named on the command line was read, whatever the report found; duplicates are a result, not a failure. A path that could not be stat'd, and a directory whose walk stopped part way through because something under it could not be read, are each named on stderr, cost only their own subtree, and make the exit code `1`. That distinction is the point: a truncated duplicate report reads exactly like a clean one, so the exit code is what says the tree was not fully read.

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

- `make check-supply-chain` — CI gate; also step [9/15] of `make analyze`.

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

### gen_api_shapes

Pins what GitLab says its own REST API accepts and returns, into `docs/development/gitlab-api-shapes.json`, beside the request inventory it is compared with.

GitLab generates an OpenAPI 3 document from the Grape entities that render its REST responses, and commits it to its own repository at `doc/api/openapi/openapi_v3.yaml`. This command fetches that document unauthenticated, extracts one entry per operation keyed `METHOD /path` with the property names of the success response, the path and query parameters and the request body, and writes the result with the ref, the retrieval date and the SHA-256 of the bytes it read.

It matters because it is the only oracle in this repository that speaks for **GitLab**. `-scope=structs` and its siblings compare our types against client-go's, which is a second model of the API rather than the API; `-check-endpoints` reads documentation pages, which are prose written for people. This is the code path GitLab renders responses through, exported by GitLab, and it needs no instance, no licence and no image. It covers Premium and Ultimate for free, because `gitlab-org/gitlab` is the Enterprise codebase.

Paths are kept exactly as GitLab spells them, `/api/v4` prefix and `{braces}` included: the record says what GitLab said, and `apishapes.NormalizePath` converts at the moment of comparison rather than baking one reader's dialect into the artifact. An operation that names no response schema, which is 553 of them, carries an empty list, and a reader must treat that as "GitLab does not say" and not as "GitLab sends nothing".

Generating needs the network, so it is not a gate. `--check` is, and it needs none: it refuses a schema version this build cannot read, an extraction of fewer than 1500 operations (a truncated download, or a document that is not this one), a record that does not say which ref it came from or what it hashed, a retrieval date nothing can parse or that has not happened yet, and a record older than the window in [`cmd/internal/provenance`](#cmdinternalprovenance), which is where that window and its reason are recorded. The generator refuses a short extraction too, before writing, so a truncated read cannot replace a whole record and report success.

#### Usage

```bash
# Re-pin from master
go run ./cmd/gen_api_shapes/

# Pin a specific ref into a scratch directory
go run ./cmd/gen_api_shapes/ -ref v18.4.0-ee -dir /tmp/probe

# CI gate, no network
go run ./cmd/gen_api_shapes/ --check
```

#### Flags

| Flag     | Type     | Default            | Description                                                                   |
| -------- | -------- | ------------------ | ----------------------------------------------------------------------------- |
| `-ref`   | `string` | `master`           | `gitlab-org/gitlab` ref to read the OpenAPI document from                     |
| `-dir`   | `string` | `docs/development` | Directory holding the committed record                                        |
| `-check` | `bool`   | `false`            | Read the committed record instead of fetching, and fail when it is not usable |

#### Output

Writes `gitlab-api-shapes.json` into `-dir` and reports the operation count, how many carry a response schema and how many carry a request body. `--check` reports the provenance line only. Exits `1` on any failure.

#### Make targets

- `make gen-api-shapes`
- `make check-api-shapes` — CI gate; also step [15/15] of `make analyze`.

### gen_api_exposes

Pins, from GitLab's own Ruby source, the condition under which each field a REST entity exposes is sent, into `docs/development/gitlab-api-exposes.json`, beside the OpenAPI record it qualifies.

The OpenAPI record is an upper bound. GitLab generates it from the Grape entities that render its responses, and an entity declaring `expose :approvals_before_merge, if: ->(project, _) { project.feature_available?(:merge_request_approvers) }` reaches the document as a plain property: the condition is invisible, so the record lists the field as if every GitLab sent it. The licensed run showed what that hides, nine approval-configuration fields the record lists and a live instance never sends, and every Enterprise field a Community instance never sends. This command reads the source the document was generated from and says, per field, when.

It fetches three subtrees of `gitlab-org/gitlab` as archives at one ref, `lib/api/entities` (the Community entities), `ee/lib/api/entities` (entities only Enterprise declares) and `ee/lib/ee/api/entities` (the Enterprise modules prepended into Community entities, whose `prepended do` blocks add fields to them), plus the licensed-feature table, `features.rb` under `ee/app/models`, which lists every licensed feature symbol under the tier that unlocks it. `cmd/internal/apiexposes` reads the Grape DSL as GitLab writes it: `expose :a, :b, as:, if:, unless:, using:, with:, merge:` over as many lines as it takes, `expose :x do ... end` blocks nesting fields under `x` told apart from `expose :x do |obj| ... end` blocks computing it, `with_options if:` scopes, `if: ->(obj) do ... end` lambdas, class inheritance, `merge: true`, `expose(*helper.attributes)` recorded as a splat, and a class opened again, which GitLab's entity files do two ways: with no superclass it is a namespace reopened to declare an entity inside it (`Ci::Lint::Result` holds `Result::Include`, and reading that line as an entity once replaced the one `result.rb` declares with an empty one, 57 exposes lost across the tree), and with the same superclass it is the same entity opened again (`FeatureFlag` holds `BasicUserList`), its exposes joining the entity's. A second declaration naming another parent is refused, and so is an expose directly under a class with no superclass, since its fields belong to an entity declared elsewhere. Method bodies and value blocks are skipped statement by statement with every `do`, `if`, `def` and `end` tracked; a construct the reader does not understand stops the run with its file and line rather than leaving every frame after it off by one. Each condition is recorded as written, the licensed feature symbols in it (`feature_available?(:x)`, `licensed_feature_available?(:x)`, `License.feature_available?(:x)`) are read further into the tier the table puts them under, and a symbol the table does not list, a project setting such as `:issues`, is named without a tier.

Entities are keyed the way the OpenAPI document names their schemas, `APIEntitiesProject` for `API::Entities::Project`, and the OpenAPI record carries that name per operation (`entity`) and per nested property (`nested_entity`) since its schema version 3, so a reader walks operation to entity to condition without translating. On the day this was written the join reached 327 of the 337 components the OpenAPI record names; the ten it does not are rendered by serializers and API modules declared outside the three entity directories (`ProjectEntity`, `TestReportEntity`, the VS Code settings entities, the subscriptions entities), which the record leaves unqualified rather than guesses at. The archives name the commit the ref resolved to, and a set of archives naming different commits, which a push to master between two downloads produces, is refused rather than recorded as one tree. The record is compared by hand on every regeneration; `-source` reads a local checkout for a run without the network, digested the same way so that `-check` accepts the record, with its commit recorded as a local checkout and a warning that the ref is what the flag claimed.

#### Usage

```bash
# Re-pin from master
go run ./cmd/gen_api_exposes/

# Pin a tag, or read a local checkout
go run ./cmd/gen_api_exposes/ -ref v19.4.0-ee
go run ./cmd/gen_api_exposes/ -source ~/src/gitlab

# CI gate, no network
go run ./cmd/gen_api_exposes/ -check

# What one entity sends: the parent chain first, each field with its condition and tier
go run ./cmd/gen_api_exposes/ -report APIEntitiesProject
```

#### Flags

| Flag      | Type     | Default            | Description                                                                              |
| --------- | -------- | ------------------ | ---------------------------------------------------------------------------------------- |
| `-ref`    | `string` | `master`           | `gitlab-org/gitlab` ref to read the entities from                                        |
| `-dir`    | `string` | `docs/development` | Directory holding the committed record                                                   |
| `-check`  | `bool`   | `false`            | Read the committed record instead of fetching, and fail when it is not usable            |
| `-report` | `string` | _(empty)_          | Print every field the named entity sends, with its conditions, from the committed record |
| `-source` | `string` | _(empty)_          | Read the entities from this local checkout instead of fetching                           |

#### Output

Writes `gitlab-api-exposes.json` into `-dir` and reports the entity, feature, expose and file counts with the commit and the day. `-check` reports the provenance line only, and exits `1` on a schema version this build cannot read, fewer than 400 entities or 200 features, missing provenance, or a record older than the window in [`cmd/internal/provenance`](#cmdinternalprovenance). `-report` prints one line per field, the parent chain's first, with `[tier; if condition]`, `[ee; unless condition]` or `[as APIEntitiesX]` notes and nested fields indented; exits `1` for an entity the record does not hold.

#### Make targets

- `make gen-api-exposes`
- `make check-api-exposes` — CI gate.

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

`internal/testutil.NewTestClient` does the recording, so the 6444 clients the suite already builds became the instrument at no cost to the tests themselves. Each records the method, the path with its identifiers replaced by placeholders, the query parameter names, the top-level field names of a JSON request body, and for GraphQL the operation and the variables the document declares. Recording is off unless `GITLAB_MCP_TEST_INVENTORY_DIR` names an absolute directory, and one test process writes one shard, so package binaries running in parallel never contend. A client built by `internal/testutil`'s own tests records nothing: those fixtures exercise this harness's mock and are not requests this server sends GitLab.

A row is one package, method and templated endpoint, carrying the union of the parameter, body-field and variable names that package was seen sending it. Two calls that differ only in an optional filter are the same endpoint, so the union answers which names were sent and deliberately not which of them were sent together: a combination is a property of the fixtures, and a file that recorded them would be read as a contract. That is why `gitlab_get_catalog_resource` could send GitLab `id` and `full_path` together for a release while this file listed both names on one row.

The body-field names matter more than they look. Without them 97% of the mutating rows carried a method and a path and nothing else, which is where a wrong field name lives, and the artifact still read as an answer to what this server sends.

The path rule reads the shape of a segment and never a list of names, because a list would have to be kept in step with a thousand actions. A segment is an identifier when it is all digits, when it carries a percent-encoded slash (at either escaping depth), when it is hexadecimal and long enough to be a commit, or when it is shaped like a UUID. The placeholder is named after the collection segment in front of it, so `/projects/1/issues/2/notes` is `/projects/:project_id/issues/:issue_id/notes`: the placeholders used to be positional, `:id` for the first identifier and `:iid` for the rest, and that made `:id` name the project on one row and the board on the next whenever the project was spelled as a fixture word the rule cannot recognize.

What the rule cannot catch is an identifier that looks like a word: a branch named `main`, a wiki slug, a CI variable key, a project addressed as `my-project`, all stay in the path verbatim, so one endpoint reached with three of them is three rows. That limit is left visible rather than papered over. Templating by the parent segment instead, so that anything after `projects` or `groups` is the project or group, was measured against the recorded requests rather than assumed: `projects` is also followed by the literals `import`, `shared` and `user`, `groups` by `import` and `shared`, `packages` by `generic`, `npm` and `ml_models`, `snippets` by `all` and `public`, `runners` by `all`, `verify` and `reset_registration_token`, and `personal_access_tokens` by `self`. Each of those is a different endpoint from the one with an identifier in that position.

The attribution is the package that built the client and the test that built it, never the action, and the reason is where the recording sits rather than a law about what can be known. Nothing on the wire names an action, and at the moment a request is observed nothing on the stack does either, because the `httptest` server answers on its own goroutine while the test goroutine that called the handler is blocked out of sight. A `RoundTripper` on the client would see that goroutine, since an outgoing request is dispatched on the caller's own; what it would name is a Go function, and the catalog's route for an action is a closure over its handler, so turning a frame into an action ID means recording the handler's function identity on the spec, in production code, for a test artifact. The honest coarse attribution was worth more than a mapping that is a guess. The committed artifact keeps the package and drops the test name, so adding a test that reaches an endpoint already listed does not change it; the shard keeps the test name, so a row can still be traced back.

The committed recording is a Linux one, made as root on a filesystem with symlinks. Nothing in the suite currently issues a request only under one of those conditions, and one test that did was renamed to publish a file name another test already sends, so today the artifact is machine-independent. A regeneration on another platform may legitimately differ, and a test that skips conditionally must not be the only producer of a row.

The summary this prints counts actions whose owning package recorded nothing, which is weaker than "this action was never exercised" in two ways worth knowing: it is package-grained, and a package that declares specs whose handlers live elsewhere is counted silent even though the handler's own package recorded the request. `internal/tools/adminspecs` is the whole of that case today, and it is why the silent list is read package by package rather than action by action.

What this does not do is judge. It says what we send, not whether GitLab would accept it: comparing these paths with GitLab's own documentation is the check that follows, and a response our output struct misreads can only be caught by a real instance.

#### Usage

```bash
# Record the suite and rewrite the artifact
make gen-request-inventory

# CI gate: verify it against shards already recorded
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

The artifact on disk, and a three-line summary on stderr: how many rows, distinct paths and packages the inventory holds, and how much of the catalog the recording could see. Exits non-zero when the shard directory holds no shard, when a shard cannot be read, and in `-check` mode when the committed artifact is not what the shards say it should be. An empty shard directory is an error rather than an empty inventory, because writing that would erase the artifact and report the whole file as a change.

#### Make targets

- `make gen-request-inventory`: records the suite, then rewrites the artifact.
- `make check-request-inventory`: the same, gating instead of writing. CI does not run this target: it sets `GITLAB_MCP_TEST_INVENTORY_DIR` on the coverage job's suite run and merges those shards, so the gate costs one `go run` rather than a second seven-minute suite.
- `make audit-request-inventory`: the gate, naming the silent packages.

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

### eval_mcp_surfaces

Evaluates model behavior across MCP tool surfaces by running typed evaluation cases against the server in mock or live (Docker/self-hosted) mode. See [`cmd/eval_mcp_surfaces/README.md`](../../cmd/eval_mcp_surfaces/README.md) for the full guide, case formats, and run modes.

**Make targets:** the `make eval-surfaces-docker*` family (`eval-surfaces-docker`, `eval-surfaces-docker-enterprise`, `eval-surfaces-docker-enterprise-ce`, `eval-surfaces-docker-enterprise-all`, `eval-surfaces-docker-enterprise-all-fixtures`).

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

`WalkFiles` stops at the first error and returns it, whether the walk raised it (an absent root, a directory the process may not read) or the visitor did, and there is deliberately no best-effort mode. Every caller but one is a gate, and a gate that skipped an unreadable directory would certify a tree it never read; the files gathered before such an error are a prefix of the tree and look exactly like the whole of it. A caller that wants to continue past a failure swallows it inside its own visitor, where it can say which file it gave up on. What no caller may do is discard the returned error, because by then the walk has already stopped: `audit_string_dupes` was doing that, and now names the tree it could not finish and exits non-zero.

What the package deliberately does **not** own is discovery. `gen_stats` keeps asking git (`git ls-files`) so `check-stats` stays a function of what is committed, and `gen_testing_docs` keeps enumerating packages through `go list` because it describes packages; sharing the input universe would break both.

One predicate also stays where it is. `cmd/godoc_tool` asks which functions need a test-form doc comment rather than which functions the testing package runs, so it keeps `TestMain` and the lower-case `Test`-prefixed helpers that `IsTestFunction` excludes; routing it through the shared predicate would silently drop those findings from `make audit-docs`.

### cmd/internal/goprogram

The go/packages front end for the four gates that type-check this repository's own source: [`audit_md_escaping`](#audit_md_escaping) walks the calls that interpolate a GitLab-authored value into Markdown, [`audit_readonly_graphql`](#audit_readonly_graphql) walks the calls a read-only action can reach, `cmd/internal/graphqldocs` folds every raw GraphQL document to the one string GitLab would receive, and [`audit_graphql_shapes`](#audit_graphql_shapes) pairs each of those documents with the struct that decodes it. It owns `LoadMode`, `Load(dir, patterns, overlay)` and the refusal of a package that did not type-check; the indexers, the detectors, the questions and the binaries stay with each gate.

`LoadMode` deliberately omits `NeedDeps`, for one reason that holds for all four: each gate only ever reads bodies written inside the patterns it loads, so type-checking the dependency tree from source would cost minutes and change no answer, while export data still gives every dependency object the identity the packages using it see.

The refusal is why this is a package rather than four tidy copies. Each gate answers "cannot tell" for what it cannot resolve, and a partially typed package resolves nothing: the escaping audit would classify every value as unfollowable, the read-only audit would find no handlers, and the document collector and the shape audit would fold no constants, so all four would report a clean run over source they never understood. The rule was written four times with four wordings, and a change to it was a four-file edit with one file easy to forget.

The `overlay` parameter is not a convenience. It is how three of the gates' tests supply a fixture package written in the test file itself, type-checked against the real packages it imports, so the classifiers are exercised on the shapes they have to handle rather than on a mock of them. Production passes `nil`, and so does `audit_graphql_shapes`, whose fixtures are written to a module on disk.

`cmd/audit_1to1/internal/shared.LoadToolPackages` is deliberately not folded in. It loads with `NeedDeps`, so it pays for the dependency tree these four refuse to pay for, and it refuses more widely than `Load` does, collecting every error of every loaded package (the dependencies included) and aborting on all of them at once where `Load` stops at the first error of a package the caller asked for. It also returns a subset rather than what it loaded, keeping the packages under `internal/tools` and dropping the rest, and memoizes that result per root. That is a different contract, not a different wording of this one.

### cmd/internal/docgen

The Markdown renderers the generators share (`RenderMarkdownTable`, and `ReplaceSection` / `ComputeReplacedSection` for a managed region of a hand-written document), plus the two ways a command puts bytes on disk. Those two are kept apart on purpose, because they answer different questions:

- **`WriteOrCheck(path, content, check, regenerate)`** is the whole-file freshness convention for a committed artifact, and the one place its decisions are taken: a write creates the parent directory (a check never does, so a gate reports a missing tree rather than making one), the file is written through an `os.Root` opened on that directory so the write can only land on the named file, content is given the trailing newline that keeps a generated file from being the one text file in the repository without one, the comparison ignores carriage returns so a Windows checkout does not report drift a Linux one cannot see, and a stale artifact is reported with one sentence naming the file and the command that refreshes it. The mode is `0o600`: seven of the eight callers already used it, it is what gosec's G306 accepts without a suppression, and it only ever applies to a file the generator creates from nothing, since neither `os.WriteFile` nor `Root.WriteFile` changes the mode of a file that is already there.
- **`WriteReport(path, content)`** is the `-` means stdout convention for an auditor's `-output` flag. Nothing there is committed and nothing is compared, and the path is the operator's own, so it is contained to no directory.

Merging the two behind one signature with a mode flag is the one way to make this worse than the copies it replaced, which is why there are two functions. The managed-section helpers stay separate for the same reason: what is generated there is a region, and the rest of the file is somebody's prose.

`NormalizeNewlines` is exported for one reason: the commands whose artifacts `WriteOrCheck` writes hold the same bytes to the same rule in their own tests (`gen_llms` against the six committed files, `audit_metrics` against the committed `stats.json`), and private copies of that one line in each test file are the drift this package exists to stop.

Half of one `-` writer stays where it is. [`audit_edition_tier`](#audit_edition_tier) writes its stdout branch to a writer the caller injects, which is how its tests read that branch back without swapping `os.Stdout`; folding it in would mean giving up that seam or giving `WriteReport` a writer parameter no other caller has a use for. Its file branch is the shared helper's, which also gave it the missing parent directory it did not create before.

### cmd/internal/provenance

The age verdict passed on a committed record of something that lives in `gitlab-org/gitlab`: the retrieval date's arithmetic (`Age`, `Days`), the default clock the `--check` halves share (`Clock`), the three ways a date stops a record being one a gate can rest on (`Problems`), and the one staleness window with its one recorded reason (`MaxAge`).

Three commands pin such a record — [`gen_api_shapes`](#gen_api_shapes) the OpenAPI document, [`gen_api_exposes`](#gen_api_exposes) the entity conditions, [`gen_graphql_schema`](#gen_graphql_schema) the GraphQL schema — and each has the same shape for the same reason: generating needs the network so it cannot gate, `--check` gates precisely because it needs none, and a check that runs offline can prove the record is readable, whole and provenanced while proving nothing about whether it still matches the GitLab it was taken from. The window is the answer to that gap, and it is an answer about GitLab's release cadence rather than about any one of the three records: a reason to widen or narrow it moves all of them. It had been written down three times with three copies of its justification and a comment in one pointing at its twin.

Why 180 days: GitLab ships monthly and narrows fields in place, so half a year is roughly six releases of drift — long enough not to ambush an unrelated change often, short enough that a narrowing is noticed within a release cycle or two. A record older than that is a gate that has quietly stopped asking, and the only honest way to say so is to fail. This paragraph and `MaxAge`'s comment are the whole of that decision, and moving the window means editing those two: the number appears nowhere else in the repository, and everything that mentions the gate — the three command sections here, the gate table in [Static analysis](static-analysis.md), the staleness paragraphs in [GraphQL Integration](../concepts/graphql.md) and the `cmd/` tree in `CLAUDE.md` — says only that a record past the window is refused, and points here for how long that window is and why.

What stays with each command is what makes its record its own: its `Source` type, its floor (`MinimumOperations`, `MinimumEntities`, `MinimumTypes`), the identity checks that ask what the record is a record of, its artifact dialect, its make targets and its binary. `Subject` carries the two words that differ between the three messages — the noun (`record`, `pin`) and what this particular record can no longer report — so each sentence in CI output is still about one artifact.

Two nearby commands are deliberately not members. [`gen_request_inventory`](#gen_request_inventory) commits no provenance at all: its `-check` is byte equality against a fresh recording of the unit suite, so there is no date to judge and no window to share. [`audit_graphql_documents`](#audit_graphql_documents) owns no record either; it reads the pinned schema's, and calls `Age` only to say how long ago the pin was taken in its drift report, where an unreadable date stays silent rather than becoming a second complaint about a field `check-graphql-schema` already refuses.

## CI gate targets

The following utilities expose a verification mode (`--check` or `-check`, or an invariant/error exit) that CI runs to guard against drift. The combined documentation gate is `make audit-docs`, which chains markdownlint, the table formatter, the llms, LobeHub-manifest, testing-docs and site-stats checks, the local-link check, the godoc, surface-quality and alias audits, and the site's own `check`, `build` and `lint`.

| Make target                              | Utility                            | What it gates                                                                                                              | Exit behavior                                                                        |
| ---------------------------------------- | ---------------------------------- | -------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------ |
| `check-action-catalog-manifest`          | `gen_action_catalog_manifest`      | Generated ActionSpec manifest is current                                                                                   | Non-zero if the manifest is stale                                                    |
| `check-llms`                             | `gen_llms`                         | `llms.txt` and `llms-full.txt` are current and structurally valid                                                          | Non-zero if either file is stale or malformed                                        |
| `check-lhm-manifest`                     | `gen_lhm_manifest`                 | `lhm.plugin.json` declares the registered tools, prompts, and resources                                                    | Non-zero if the manifest is stale                                                    |
| `check-footprint`                        | `audit_tokens -footprint`          | README token-footprint section, `docs/development/token-footprint.md` and `site/src/data/token-footprint.json` are current | Non-zero if any is stale                                                             |
| `check-stats`                            | `gen_stats`                        | README repository-statistics section is current                                                                            | Non-zero if the section is stale                                                     |
| `audit-discovery-check`                  | `audit_discovery_completeness`     | No META-001 finding meets the configured severity threshold                                                                | Non-zero if any finding meets `-severity` (default error)                            |
| `audit-doc-coverage-check`               | `audit_doc_coverage`               | No `docs/reference/tools/*.md` has missing/orphan/tier_mismatch findings                                                   | Non-zero if any file has a finding                                                   |
| `audit-godocs-check`                     | `godoc_tool audit`                 | No package, symbol, or test Godoc findings remain                                                                          | Non-zero when findings are present                                                   |
| `audit-dynamic-aliases`                  | `audit_dynamic_aliases`            | No error-severity alias governance finding (collisions, ambiguity)                                                         | Non-zero (`1`) if any error-severity finding exists                                  |
| `audit-docs` → `format_md_tables -check` | `format_md_tables`                 | All Markdown pipe tables are normalized                                                                                    | Non-zero if any table needs formatting                                               |
| `check-testing-docs`                     | `gen_testing_docs`                 | The `docs/development/testing/testing.md` test-metrics block is current                                                    | Non-zero if the generated section is stale                                           |
| `check-supply-chain`                     | `audit_supply_chain`               | The five release-configuration invariants still hold                                                                       | Non-zero if any is broken, or if the audit cannot be run                             |
| `check-doc-tool-names`                   | `audit_doc_tool_names`             | Every `gitlab_*` name the documentation mentions is registered on some surface                                             | Non-zero if any name is unregistered                                                 |
| `check-gateway-chars`                    | `audit_gateway_chars`              | Nothing served carries a character a gateway validator rejects                                                             | Non-zero if any offender is served                                                   |
| `check-meta-descriptions`                | `audit_meta_descriptions -check`   | Every parameter and value a served meta-tool description offers is one its actions accept                                  | Non-zero if a description and the schemas disagree                                   |
| `check-install-buttons`                  | `audit_install_buttons`            | Every install button decodes and agrees with the others for its command                                                    | Non-zero on a problem, or when no button is found                                    |
| `check-test-goroutines`                  | `audit_test_goroutines`            | No `testing.T` abort is made off the test goroutine                                                                        | Non-zero if any abort site exists                                                    |
| `check-test-subtests`                    | `audit_test_subtests`              | No case loop asserts without a `t.Run` subtest                                                                             | Non-zero if any site remains                                                         |
| `check-test-file-names`                  | `audit_test_names -check-files`    | Every `_test.go` is named after a module it tests                                                                          | Non-zero on a violation                                                              |
| `check-md-escaping`                      | `audit_md_escaping -check`         | No value reaches a Markdown table cell, heading, list item or link unescaped                                               | Non-zero on a finding or a directive that excuses nothing                            |
| `check-site-stats`                       | `audit_metrics -site-stats -check` | `site/src/data/stats.json` is current                                                                                      | Non-zero if the file is stale                                                        |
| `check-bench-resources`                  | `bench_resources -check`           | The committed benchmark charts and tables match the committed record                                                       | Non-zero if they are stale                                                           |
| `brand-check`                            | `gen_brand --check`                | The committed brand assets match the geometry                                                                              | Non-zero on drift                                                                    |
| `check-icon-webp`                        | `gen_icon_webp --check`            | The committed WebP icons match `icons.go` (needs `rsvg-convert` and `cwebp`, so not run in CI)                             | Non-zero on drift                                                                    |
| `check-readonly-graphql`                 | `audit_readonly_graphql`           | No action classified ReadOnly can reach a GraphQL mutation                                                                 | Non-zero on any finding, or if the audit cannot be run                               |
| `check-graphql-schema`                   | `gen_graphql_schema --check`       | The committed GitLab schema parses and its provenance record decodes                                                       | Non-zero if either file is missing or unusable                                       |
| `check-graphql-documents`                | `audit_graphql_documents`          | Every raw GraphQL document in the source is one the pinned GitLab schema accepts                                           | Non-zero on any refusal, or if no documents are found                                |
| `check-graphql-shapes`                   | `audit_graphql_shapes`             | Every struct a GraphQL response is decoded into can hold what its document selects, and declares nothing it never selects  | Non-zero on any disagreement, anything unpaired, or if no call is found              |
| `check-api-shapes`                       | `gen_api_shapes --check`           | The REST twin of the schema pin: the committed OpenAPI record is readable, complete and provenanced                        | Non-zero if it is unreadable, truncated, unprovenanced or past the shared age window |
| `check-api-exposes`                      | `gen_api_exposes -check`           | The committed record of entity field conditions is readable, whole, provenanced and inside the age window                  | Non-zero if any of those fails                                                       |
| `check-request-inventory`                | `gen_request_inventory -check`     | The committed request inventory is what the unit suite records now                                                         | Non-zero if the artifact is stale or no shard was written                            |
| `audit-1to1-paths`                       | `audit_1to1 -scope=paths`          | Every action's owning package was seen issuing a request, and every GraphQL document is one the pinned schema accepts      | Non-zero on a refused document, an undeclared silent package, or a stale declaration |
