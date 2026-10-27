# gitlab-mcp-server — AI Development Context

> This file provides comprehensive context for AI assistants working on this project.
> All project artifacts must be written in **English**, with one exception: the Spanish half of
> the user documentation, which is the twin pages under `site/src/content/docs/es/` and the
> Spanish text kept beside its English source (the `es` fields of
> `cmd/gen_tool_reference/domains.json`, the Spanish strings of the generators that write both
> languages, `labelEs` in `site/scripts/gen-llms.mjs`, `site/src/content/i18n/es.json`).
> Conversations may be in any language.
>
> **No AI attribution in anything published under the maintainer's name.** Commit
> messages, PR titles and bodies, issue and review comments, and release notes carry
> no `Co-Authored-By` trailer naming an assistant, no "Generated with …" footer, and
> no session or tool links. This overrides any default template that adds one.
> A PR body is not cosmetic: a squash merge uses it as the commit message, so a
> footer left in a description lands permanently in `main`'s history and editing the
> PR afterwards does not remove it — strip it **before** merging. Ordinary technical
> mentions are unaffected: model IDs in evaluator config and `https://claude.ai` as a
> client origin in CORS examples are content, not attribution.
>
> For the same reason a description never carries a command GitHub reads as a request
> to skip workflows (a bracketed skip ci or one of the four other commands GitHub
> documents, or a skip-checks trailer), not even quoted in backticks: the squash commit of #1094 carried one, and
> main ran no CI and the v3.1.0 tag on it no release. `make check-pr-description`
> refuses it.
>
> **Every commit is signed off** (`git commit -s`): a `Signed-off-by:` trailer naming
> its author certifies the Developer Certificate of Origin, and the `dco` job of CI
> fails a pull request with a commit that lacks one. A sign-off is a person's, so it is
> not AI attribution and the rule above still holds. `make check-dco` judges the
> branch before a push, and [CONTRIBUTING](CONTRIBUTING.md#developer-certificate-of-origin)
> says what it certifies and how to sign off commits already made.

**This file is loaded into every session, so it holds the rules and an index, and the long-form references live in `docs/development/`**, read when a task needs them: [Repository Map](docs/development/repository-map.md) (the annotated tree of commands and packages), [Environment Variables and Flags](docs/development/configuration.md), [Test Infrastructure](docs/development/test-infrastructure.md), [Request Paths and Fine-Grained Grants](docs/development/request-paths.md) (R-PATH, R-PAGE and R-GRANT), [Adding or Changing a Limit](docs/development/adding-a-limit.md), [Release Process](docs/development/release-process.md), [Tier Gating](docs/development/tier-gating.md), [HTTP Mode Invariants](docs/development/http-invariants.md), [Resource Subscriptions](docs/development/resource-subscriptions.md) and [Copyright and License Headers](docs/development/file-headers.md). Keep it that way: what a new section adds here is the rule and a link, and its explanation goes to the page that owns the topic. Claude Code warns at every start once this file passes a size limit that scales with the model's context window and never falls below 40,000 characters (150,000 for current models in a one-million-token window).

## Project Overview

**gitlab-mcp-server** is a Model Context Protocol (MCP) server written in Go that exposes GitLab REST API v4 and GraphQL operations as MCP tools for AI assistants. It runs as a local binary communicating via stdio or HTTP transport.

| Attribute     | Value                                               |
| ------------- | --------------------------------------------------- |
| Language      | Go 1.27.3                                           |
| MCP SDK       | `github.com/modelcontextprotocol/go-sdk/mcp` v1.8.0 |
| GitLab Client | `gitlab.com/gitlab-org/api/client-go/v3` v3.30.0       |
| Transport     | stdio (primary), HTTP (optional)                    |
| Platforms     | Windows, Linux & macOS, amd64 & arm64               |
| Version       | 3.3.0                                               |

### Scale

| Metric                    | Count                                                                                                        |
| ------------------------- | ------------------------------------------------------------------------------------------------------------ |
| MCP Tools (individual)    | By instance tier: ~872 Free/CE; ~1027 Premium; ~1093 Ultimate (self-managed) / ~1099 on GitLab.com Ultimate with Orbit |
| Catalog groups            | By instance tier: 29 Free/CE; 35 Premium; 46 Ultimate                                                       |
| Meta-mode tools           | 34 base (Free/CE) / 40 Premium / 51 self-managed Ultimate / 52 GitLab.com Ultimate (Orbit); `gitlab_server` is served on every one of them and is counted here |
| Dynamic-mode tools        | 2 dynamic tools (`gitlab_find_action`, `gitlab_execute_action`) — see Dynamic toolset mode below |
| MCP Resources             | 45 across dynamic/full, meta/full, and individual/full modes; `gitlab://tools` adapts to the active surface |
| MCP Prompts               | 37 (12 core + 4 cross-project + 4 team + 5 project-reports + 4 analytics + 4 milestone-label + 2 git-workflow + 2 audit)      |
| Completion argument names | 18                                                                                                           |
| MCP Capabilities          | 4 (progress, elicitation, completions, resource subscriptions)                     |
| MCP Icons                 | 51 icons (50 domain + brand mark), each a 3-entry `[]mcp.Icon`: one SVG (base64 data URI, `Sizes: ["any"]`, `currentColor`) plus light/dark 16×16 lossless WebP fallbacks (`Theme`-tagged, `cmd/gen_icon_webp`) for clients that reject SVG. The brand mark is the generated "fan-out" (`cmd/gen_brand` → `brandmark_gen.go`), original artwork replacing the former tanuki |
| Source files (tools)      | 788 non-test Go files under `internal/tools/`                                                                |
| Test files (tools)        | 488 test files under `internal/tools/`                                                                       |
| Go packages               | 312 in the module (`go list ./...`); 339 directories hold tracked `.go` files, the twenty-seven more being what `go list` leaves out: twenty-six `testdata` fixture packages (the two stand-in programs, the coverage audit's planted trees, the closed-server audit's planted module, the stand-in module the SDK route reader's tests read and the platform module the condition measurement's discovery lists) and `test/e2e/modeleval/internal/provider`, whose every file sits behind a build tag. 182 under `internal/tools/...` (the root package plus 181 sub-packages) |

### Orbit live tests

The six read-only `gitlab_orbit_*` tools (`status`, `schema`, `tools`, `dsl`, `query`, `graph_status`) target GitLab.com's experimental Knowledge Graph API, which no self-managed GitLab serves. Their `orbitlive`-gated live suite (`test/e2e/orbit/live_test.go`, every query in GQL since issue 1414 except the one that checks GitLab.com's refusal of the JSON query DSL) runs against the fixtures under `test/fixtures/orbit/` that `scripts/setup-orbit-fixtures.sh` provisions. `make test-e2e-gitlab-com` sets them up, waits for the indexer, runs the suite and re-records what the six routes answer with `make gen-orbit-record` into `docs/development/orbit-responses.json`, the oracle R-PATH holds the Orbit output types to, since GitLab publishes no response schema for them. `make check-orbit-record` is the offline gate (step 24 of `make analyze`), and only a holder of an Orbit-entitled GitLab.com token and the fixture namespace can refresh the record before the provenance window closes. See [Orbit live test fixtures](docs/development/orbit-fixtures.md).

## Project Structure

One line per entry; [Repository Map](docs/development/repository-map.md) carries the annotated version of this tree, and [Command-Line Utilities](docs/development/cmd-utilities.md) the flags and gates of each command.

```text
gitlab-mcp-server/
├── cmd/
│   ├── server/                       # MCP server entry point and --shutdown support
│   ├── audit_1to1/                   # 1:1 SDK/API parity audit: R-INPUT, R-OUTPUT, R-ACTION, R-META, R-ENUM, R-PATH, R-PAGE, R-GRANT and requiredness
│   ├── audit_action_ids/             # Action IDs and tool names in served prose held to the catalog (make check-action-ids)
│   ├── audit_catalog_first/          # Catalog-first registration (ADR-0004); every ActionSpecs aggregated (make audit-catalog-first)
│   ├── audit_dead_consts/            # Unread constants in const groups staticcheck cannot judge (make check-dead-consts)
│   ├── audit_discovery_completeness/ # Discovery metadata: aliases, usage, related, param guidance (META-001)
│   ├── audit_doc_tool_names/         # Tool names and action IDs the documentation teaches (make check-doc-tool-names)
│   ├── audit_dynamic_aliases/        # Dynamic discovery aliases
│   ├── audit_e2e_coverage/           # What the e2e suite covered; static, skip and release gates (make check-e2e-static)
│   ├── audit_edition_tier/           # Doc-grounded edition tier gating
│   ├── audit_gateway_chars/          # Characters strict MCP gateways reject (make check-gateway-chars)
│   ├── audit_graphql_documents/      # Raw GraphQL documents against the pinned schema (make check-graphql-documents)
│   ├── audit_graphql_shapes/         # GraphQL documents against the structs they decode into (make check-graphql-shapes)
│   ├── audit_install_buttons/        # One-click install payloads (make check-install-buttons)
│   ├── audit_binary_vulns/           # Release binaries against the Go vulnerability database (make check-binary-vulns)
│   ├── audit_image_vulns/            # The image's Alpine packages against Trivy's database, before the push (make check-image-vulns)
│   ├── reproduce_release/            # Release binaries rebuilt from the tag and compared byte for byte (make check-reproducible, check-repeatable)
│   ├── audit_site_headers/           # The documentation site's security headers, after each deploy (make check-site-headers)
│   ├── audit_dco/                    # Every commit of a pull request signed off by its author, the DCO (make check-dco)
│   ├── audit_file_headers/           # Every source file opens with the copyright and license header (make check-file-headers, fix-file-headers)
│   ├── godoc_tool/                   # Go doc auditor and fixer (make check-godocs)
│   ├── audit_md_escaping/            # GitLab-authored values reaching Markdown unescaped (make check-md-escaping)
│   ├── audit_metrics/                # Tool, resource and prompt metrics
│   ├── audit_meta_descriptions/      # Meta-tool prose against the parameters its actions accept (make check-meta-descriptions)
│   ├── audit_readonly_graphql/       # ReadOnly actions that can reach a GraphQL mutation (make check-readonly-graphql)
│   ├── audit_sdk_context/            # client-go calls without the caller's context (make check-sdk-context)
│   ├── audit_tenancy/                # Tenant policy register against the code that enforces it (make check-tenancy)
│   ├── audit_supply_chain/           # Release-configuration invariants (make check-supply-chain)
│   ├── audit_surface_quality/        # Served surface metadata and output quality (make check-surface-quality)
│   ├── audit_test_goroutines/        # testing.T aborts off the test goroutine (make check-test-goroutines)
│   ├── audit_test_names/             # Test function and file naming (make check-test-file-names)
│   ├── audit_test_subtests/          # Case loops that assert without t.Run (make check-test-subtests)
│   ├── audit_test_closed_servers/    # httptest servers closed while their address is still used (make check-closed-servers)
│   ├── audit_tokens/                 # Token usage of the model-facing surfaces
│   ├── bench_resources/              # CPU and memory measurements and the charts the docs publish (make bench-resources)
│   ├── gen_model_corpus/             # Model evaluation corpus breadth ledger
│   ├── gen_model_results/            # Folds model evaluation shards into the committed record and results page
│   ├── format_md_tables/             # Markdown pipe table formatter (make check-md-tables)
│   ├── measure_conditions/           # The cross-OS condition and branch coverage workflow and the whole-tree figures (make coverage-conditions-tree)
│   ├── fuzz_targets/                 # The fuzz targets the weekly fuzz.yml runs with go test -fuzz
│   ├── gen_action_catalog_manifest/  # Audited action catalog manifest
│   ├── gen_api_live/                 # REST oracle evaluated from a booted GitLab: gitlab-api-live.json (make gen-api-live)
│   ├── gen_action_grants/            # What each action sends and what a token needs for it (make gen-action-grants)
│   ├── gen_brand/                    # Every vector brand asset from one geometry
│   ├── gen_graphql_schema/           # Pins GitLab's GraphQL schema (make gen-graphql-schema)
│   ├── gen_icon_webp/                # WebP icon fallbacks (maintainer-only)
│   ├── gen_orbit_record/             # What GitLab.com's Orbit routes answer (make gen-orbit-record)
│   ├── gen_lhm_manifest/             # Capability arrays of lhm.plugin.json (LobeHub)
│   ├── gen_llms/                     # llms.txt and llms-full.txt
│   ├── gen_request_inventory/        # request-inventory.json from the shards the unit suite records
│   ├── gen_testing_docs/             # docs/development/testing/testing.md
│   ├── gen_tool_reference/           # The site's per-domain tool reference (make gen-tool-reference)
│   ├── gen_third_party_notices/      # THIRD_PARTY_NOTICES at release time
│   └── internal/                     # Helpers the commands share (actionrequests, sdkroutes, actionids, apilive, fileheader, graphqldocs, goprogram, sourcegate, mcpsurface, provenance, ...)
├── internal/
│   ├── cachehints/                   # SEP-2549 cache hints on MCP results
│   ├── capguard/                     # Methods answered kept in step with capabilities declared
│   ├── clientcompat/                 # Per-client response compatibility (GITLAB_MCP_CLIENT_COMPAT)
│   ├── cmdutil/                      # Shared helpers for the repository commands (Must at the leaf)
│   ├── config/                       # Configuration loading (dotenv files, flags, env vars, HTTP env overlay)
│   ├── edition/                      # Licensing tier model (Free/Premium/Ultimate)
│   ├── finegrained/                  # What a fine-grained token needs per action, and the classic scope (ADR-0024, ADR-0026)
│   ├── freshness/                    # Test-only reader of GITLAB_MCP_TEST_SNAPSHOT_PARITY
│   ├── gatewaycompat/                # Description and title rewriting for strict MCP gateways
│   ├── gitlab/                       # GitLab API client wrapper (client.GL() accessor)
│   ├── graphqlschema/                # The pinned GitLab GraphQL schema and Validate()
│   ├── i18n/                         # Person-facing message catalog (English source) and the request language (GITLAB_MCP_LANGUAGE)
│   ├── mcpotel/                      # OpenTelemetry instrumentation of MCP request handling (API only)
│   ├── oauth/                        # OAuth HTTP mode: token cache, verifier, RFC 9728 metadata
│   ├── serverpool/                   # HTTP mode: bounded LRU pool of per-token+URL entries
│   ├── sourcewalk/                   # Which directories below a walk root are this repository's own source
│   ├── subscriptions/                # resources/subscribe: polled watchers and leases (ADR-0015)
│   ├── tenancy/                      # The tenant policy register (ADR-0023): decides, never enforces
│   ├── telemetry/                    # The one place that knows the OpenTelemetry SDK
│   ├── toolutil/                     # Shared tool utilities (errors, pagination, markdown, logging)
│   ├── testutil/                     # Shared test helpers (NewTestClient, RespondJSON); netfixture/ for an address nothing answers
│   ├── tools/                        # Tool orchestration layer + 181 sub-packages
│   │   ├── action_catalog.go         # Builds the canonical action catalog from domain ActionSpecs
│   │   ├── actiongrants/             # The generated fine-grained table (table_gen.go, never edited)
│   │   ├── catalog_filter.go         # Read-only, token-scope and --exclude-tools filters
│   │   ├── scope_filter.go           # PAT scopes per catalog group, applied before registration
│   │   ├── register.go               # Projects individual tools from the catalog
│   │   ├── register_meta.go          # Standalone utility tools beside the meta groups
│   │   ├── dynamic/                  # Low-token find/execute surface over catalog routes
│   │   ├── dynamiccatalog/           # The dynamic catalog as the server assembles it
│   │   ├── toolvisibility/           # The pass over tools registered outside the catalog
│   │   ├── markdown.go               # Delegator to the type-based Markdown registry
│   │   ├── meta_tool.go              # Route wrappers and the meta parameter-schema mode
│   │   └── <domain>/                 # branches, issues, mergerequests, pipelines, projects, ...
│   ├── resources/                    # 45 MCP resource implementations
│   ├── prompts/                      # 37 MCP prompt implementations
│   ├── completions/                  # 18 argument completion types
│   ├── progress/                     # MCP progress notifications
│   └── elicitation/                  # MCP elicitation capability
├── docs/                             # Contributor documentation only (ADR-0025): a README and development/
├── site/                             # The documentation site, the only home of user documentation (English + es/ twin)
├── examples/telemetry/               # The worked collector example the telemetry page imports; test/e2e/collector holds it to the server
├── test/e2e/                         # End-to-end tests: http/, stdio/, collector/, orbit/, internal/harness/, gitlab/{common,ce,ee}
├── plan/                             # Untracked working area; make check-plan-untracked fails on a force-added file
├── mcpb/                             # Claude Desktop extension manifest, icon, platform.jq and the Linux launcher
├── .github/                          # AI assistance infrastructure: agents, skills, instructions
├── Makefile                          # Build, test, lint targets
└── VERSION                           # Semantic version (3.3.0)
```

The site is where every user page lives, in English with its Spanish twin under `es/`, changed together and held by `pnpm run i18n:check`; a page writes the release a reader installs as `%%VERSION%%`, and generated pages (the tool reference, the fine-grained permissions reference, the benchmark figures and data) are never edited by hand.

## Editing files

Change existing files with the **Edit** tool and create them with **Write**. Do not
edit files by writing Python, `sed` or `perl` scripts, and not even when an automatic
mode suggests preferring shell commands for file work.

Edit verifies that the target text still matches before writing, and applies
atomically, so a stale assumption fails loudly instead of corrupting the file. A
script does neither: an anchor that no longer matches will delete or duplicate a
declaration and leave a file that does not parse. Shell commands stay right for
running, searching and generating; a script is justified only for a bulk mechanical
rewrite across many files, and the result must be verified to build.

**Every source file opens with the copyright and license header**, the two lines
`cmd/internal/fileheader` spells, as comments of its language: a new file is written
with them (`make fix-file-headers` adds them), a generator that writes source emits
them itself, and a file somebody else wrote is never stamped. `make check-file-headers`
is the gate, and [Copyright and License Headers](docs/development/file-headers.md)
says which files carry them and where.

## Key Development Patterns

### Adding a New MCP Tool

1. Create `internal/tools/{domain}/` sub-package directory, with a `doc.go` holding the package comment (every package keeps it there; `make check-godocs` fails on one anywhere else)
2. Create `{domain}.go` with typed input/output structs (no domain prefix — package provides namespace). A multi-word name separates its words with underscores in the **file** only: `merge_requests.go` in `package mergerequests`, since Go's convention and the `stylecheck`/`revive` naming rules refuse underscores in package identifiers and directories follow the package
3. Create `{domain}_test.go` with table-driven tests using `testutil.NewTestClient` and `httptest`; the test file carries the module's name, underscores included (`merge_requests_test.go`)
4. Add or update domain-local `ActionSpecs` so the action has one canonical route, owner package, metadata, compatibility policy, individual projection metadata, and tests.
5. If the domain is new, update the generated/audited catalog aggregation path rather than adding ad hoc root registration calls.
6. Add markdown formatters in the sub-package `markdown.go` `init()` function using `toolutil.RegisterMarkdown[T]`. The formatter does not decide the content annotation: the catalog derives the action's content kind from what it does (`toolutil.ResolveContentKind`: a write is `mutate`, a read of a collection envelope or of a page is `list`, any other read is `detail`), and `toolutil.FinishToolResult` gives every text block that kind's preset. Declare `ContentKind` on the `ActionSpec` only as an override, for a list whose output shape reads as one object (a read that takes a page and publishes one list fails `TestBuildActionCatalog_EveryAction_ResolvesToListDetailOrMutate` until it does); a declaration the derivation agrees with fails `TestCollectedActionSpecs_ContentKindDeclarations_OverrideTheDerivation`. See [Content kind](docs/development/tool-surfaces-and-action-core.md#content-kind)
7. For list formatters: add `toolutil.HintPreserveLinks` as the first hint in `WriteHints()` to instruct the LLM to preserve clickable links
8. Add clickable `[text](url)` links in Markdown table columns where applicable (MRs, issues, pipelines, etc.)
9. Next steps need no extra work on any surface: `toolutil.FinishToolResult`, applied by the meta, dynamic, individual and standalone dispatchers alike, copies the hints the Markdown carries (the ones `WriteHints` wrote) onto the typed output's `next_steps` when the output type embeds `toolutil.HintableOutput`. A type that does not embed it carries its hints in the Markdown alone, since the individual surface validates structured output against the schema the type declares
10. Run `make gen-tool-reference` to regenerate the site's per-domain tool reference (`site/src/content/docs/reference/tools/` and its `es/` twin), which is generated from the catalog and never edited by hand; a new catalog group also needs its overview and sample questions, in English and Spanish, in `cmd/gen_tool_reference/domains.json`, or generation stops. Then run `make gen-site-stats` (the action count in `site/src/data/stats.json`) and `make gen-action-grants` (the fine-grained table holds one row per action), or `make update-all`, which runs all three
11. After completing a test-focused tool implementation phase, run `go run ./cmd/gen_testing_docs/` or `make gen-testing-docs` to refresh `docs/development/testing/testing.md`, then verify with `go run ./cmd/gen_testing_docs/ --check`
12. Write an end-to-end scenario for the action under `test/e2e/gitlab/`, and declare its ID as a typed `harness.ActionID` constant in that package's `actions_test.go` — `common` for an action any instance serves, `ce` for one only an unlicensed instance does, `ee` for Premium or Ultimate. **This is a gate, not a suggestion**: `make check-e2e-static` (`cmd/audit_e2e_coverage -static`, run by `make analyze` and by CI's compile job) fails on a catalog action no scenario names. An action nothing on a Docker instance can run is declared in `cmd/audit_e2e_coverage/exemptions.go` with a category and a reason, and a declaration that stops describing the tree is itself a finding. A string literal at the call site is invisible to the gate, which reads typed constants out of the type checker's record

See the [output format reference](https://docs.jmrp.io/gitlab-mcp-server/reference/output-format/) for the complete response format specification.

### Served input schemas

**No tool any surface registers carries a combinator or a conditional at the root of its input schema** (`allOf`, `anyOf`, `oneOf`, `not`, `if`, `then`, `else`): the Anthropic Messages API refuses a request whose tool list holds one such tool, so a client that passes the schemas it lists to that API as they are has every request carrying it refused. A rule such as `toolutil.SchemaAnyOfRequired` or `SchemaOneOfRequired` stays catalog data, which find, `gitlab://tools` and the tool reference read, and the handler enforces it before it sends anything, in words that name the parameters, which `TestEveryRootAlternative_IsRefusedByTheHandlerBeforeAnyRequest` holds every such action to. The individual projection drops the root keywords and the meta `compact` and `full` modes serve each action's params under `$defs`; `TestCreateServer_ServedInputSchemas_CarryNoRootCombinator` and `make check-surface-quality` (`input-schema-root`) gate it. See [Served Input Schemas](docs/development/tool-surfaces-and-action-core.md#served-input-schemas).

### Tool naming convention

A tool's runtime name depends on the surface, and on the individual surface it is **declared**, not derived — `IndividualTool.Name` in the domain's `ActionSpec` — so never infer one from a formula.

- **Individual surface** (`GITLAB_MCP_TOOL_SURFACE=individual`): the prevailing form is domain-first `gitlab_{domain}_{action}` — `issue.list` is `gitlab_issue_list`, `project.get` is `gitlab_project_get`, `branch.create` is `gitlab_branch_create`. A large legacy set is verb-first instead (`gitlab_list_issue_discussions`, `gitlab_get_issue_statistics`, `gitlab_add_ssh_key`), and both forms are real. New actions take the domain-first form.
- **Meta surface** (`GITLAB_MCP_TOOL_SURFACE=meta`): mostly the bare domain — `gitlab_issue`, `gitlab_project`, `gitlab_group` — with the operation in the `action` argument, alongside a few standalone tools (`gitlab_discover_project`, `gitlab_server`, the `gitlab_interactive_*` elicitation flows). A domain **without** a meta-tool of its own is a set of routes on a base one, not a tool: epics, labels, milestones, boards, members, wikis and releases at group scope are actions on `gitlab_group`; issue discussions and statistics are actions on `gitlab_issue`; todos are actions on `gitlab_user`.
- **Dynamic surface** (the default): only `gitlab_find_action` and `gitlab_execute_action`, which take the canonical catalog ID directly — `{"action": "issue.list", "params": {…}}`. This is the portable form for an example, because it does not depend on `GITLAB_MCP_TOOL_SURFACE`.

Every documentation example must name a tool the surface it shows actually registers, and say which surface that is: an individual-tool example without `GITLAB_MCP_TOOL_SURFACE=individual` is wrong twice over, since the default surface registers neither. `go run ./cmd/audit_doc_tool_names/` checks every `gitlab_*` mention and every `domain.action` ID across `docs/`, `site/src/content/docs/`, `README.md`, `llms-install.md`, `npm/gitlab-mcp-server/README.md` and this file against the names the server registers and the catalog it builds; `--check` makes it a gate. The same demand holds inside the server's own output: `make check-action-ids` fails when a `RelatedActions` entry, a `toolutil.HintAction` argument or a dotted ID in a `Usage` line is not a canonical catalog ID, an alias included, and when any sentence the server serves (an error hint or message, a refusal, a next step, parameter guidance, a schema description in a `jsonschema` tag or a schema map, a `Usage` line, or an individual tool `Description`) names a `gitlab_*` tool. Two places may name one: `internal/tools/dynamic`, its own two tools, and the `See also:` clause of a domain action's individual tool `Description`, the one part `gitlab://tools` rewrites into each surface's names; the rest of a constant `Description` is read like a `Usage` line, and one assembled at run time is held to the same by `TestToolManifest_ServedDescriptions_NameNoToolOutsideTheirSeeAlsoClause` in `internal/resources`, since `gitlab://tools` serves a domain action's `Description` verbatim on the dynamic and meta surfaces as well. A standalone surface tool's description is not one of them: the guided flows and `discover_project.resolve` are registered on meta and individual and served as their `Usage` on dynamic, so each writes its one text as its `Usage` as well, where the rule reads it. The rule's limits are stated in `docs/development/cmd-utilities.md` (The rule over served prose): it does not read the operation label a `WrapErr*` or `ErrRequired*` call prefixes an error with, which is sometimes spelled as a tool name, a value a format reports, a bare meta action name (`Use action 'list'`), or `internal/prompts` and `internal/resources`, which are outside its load.

### Error handling in tool handlers

Four error wrapping functions in `internal/toolutil/errors.go`, used across the 181 sub-packages under `internal/tools/`:

- `WrapErr(op, err)` — read-only operations (list, get, search). Generic classification only.
- `WrapErrWithMessage(op, err)` — mutating operations (create, update, delete). Includes GitLab-specific error detail via `ExtractGitLabMessage`.
- `WrapErrWithHint(op, err, hint)`: when a specific corrective action is known (e.g., "use branch.unprotect first", naming the action by its canonical ID). Includes detail + actionable suggestion.
- `WrapErrWithStatusHint(op, err, code, hint)` — combines `IsHTTPStatus` check + `WrapErrWithHint` in a single call. Use when the hint applies only to a specific HTTP status code; returns `WrapErrWithMessage` for all other codes.
- `NotFoundResult(resource, identifier, hints...)`: for get handlers when `IsHTTPStatus(err, 404)`. Returns an informational `CallToolResult` with `IsError: true` and domain-specific hints instead of a Go error. Logged at INFO level. Used by the get handlers of 22 domains, each through a helper in its `markdown.go`, whose route is wrapped with `ActionRoute.WrapNotFound`, which hands the output's builder the arguments with the documented aliases resolved as the handler read them, and names the identifier through `toolutil.ParamText` so a numeric id reads as the caller wrote it. Defined in `internal/toolutil/not_found.go`.

Use `IsHTTPStatus(err, code)` and `ContainsAny(err, substrs...)` for status-specific branching before calling `WrapErrWithHint`. A hint that names a role, a license or an owner is keyed on `IsPermissionRefusal(err)` instead of a status: GitLab refuses a missing permission with 401 at the routes of [entry 55](docs/development/upstream-bugs.md#a-permission-refusal-is-answered-401-rather-than-403), so a hint scoped to 403 is never shown there, and a hint keyed on 401 alone would follow the verdict that the token itself was refused. The predicate is true for a REST 401 or 403 whose body carries no RFC 6750 error code and does not refuse the account itself, as the API guard's plain 403 for a blocked or deactivated account does (`gitlabclient.RefusalMayBePermission`), and a route whose 403 means something else pairs it with `IsHTTPStatus`. A refusal of a fine-grained token's grant is neither: `ClassifyError` describes it with the permission GitLab names, `IsPermissionRefusal` is false for it, and `WrapErrWithHint` drops the handler's hint from it, so a 403-keyed hint never follows it either. A hint keyed on the 405 of a request other than a GET (HEAD included) that names a project by `:id` checks `IsMovedProjectRefusal(err)` first and wraps with `WrapErrWithMessage`, since GitLab refuses most non-GET requests naming a moved project by its old path with 405 before the route looks at the merge request, token or file the hint is about ([the contributor page](docs/development/error-handling.md#why-a-405-can-name-the-projects-path)). A GraphQL mutation handler decodes its payload as a pointer beside the top-level `errors`, because a refused mutation answers with the payload `null` and its reason there, and returns `GraphQLTopLevelError` when the payload is absent. The opposite answer, a write GitLab ran and answered without the object it returns (it checks a fine-grained token against the payload's object only after the write, issue 1103), goes through `toolutil.UnconfirmedWrite` or, in a handler that decodes the payload itself, `client.Authority().UnconfirmedWrite`: a fine-grained session is told the write was probably committed, and a classic session, whose client carries no authority, keeps the handler's own error. Which writes the fine-grained table says can answer so is held by `TestTable_WritesAnsweredWithoutTheirObject_AreEachAnswered` in `internal/tools/actiongrants`, which fails on one a regeneration adds until its handler is looked at and declared. For get handlers, check `IsHTTPStatus(err, 404)` **before** `LogToolCallAll` and return `NotFoundResult` with `nil` error to log at INFO instead of ERROR. See [ADR-0007](docs/development/adr/adr-0007-rich-error-semantics.md), [the contributor page](docs/development/error-handling.md) and [Error Handling](https://docs.jmrp.io/gitlab-mcp-server/operations/error-handling/).

### Every client-go call carries the handler's context

client-go takes the caller's context only as a request option and builds the request from `context.Background()` without one, so **every SDK call passes `gl.WithContext(ctx)`**, and a request built with `NewRequest`, `NewRequestToURL` or `UploadRequest` carries it among its options or is rebound with `req = req.WithContext(ctx)` before it is sent. A call without it compiles, answers correctly and passes its tests, while the action deadline, an abandoned HTTP POST and the outbound trace all lose it; that shipped in five packages before anything checked. `make check-sdk-context` (`cmd/audit_sdk_context`, step 22 of `make analyze` and a CI step) is the gate, and `testutil.CancelOnArrival` is the fixture that proves a handler passes it: its context ends when the first request reaches the mock, so a handler returns `context.Canceled` promptly only if that first request carried the context, and any request that sits out the mock's hold, having been sent without it, fails the test at cleanup (a later request carrying the context would be refused by the transport and read as the cancellation, so the return value alone proves only the first).

### Adding or changing a limit

Every decision about who a caller is and what it may hold, spend or be told (a ceiling, a rate, a budget, a lifetime, what zero means for a setting, a refusal with its channel and code) is **decided in the tenant policy register, `internal/tenancy`** ([ADR-0023](docs/development/adr/adr-0023-tenant-policy-is-declared-once.md)), and enforced where the resource is counted: the register decides and never enforces. A limit written as a literal in the layer that enforces it is the shape issue 565 retired. In short: write the row in the family file for the question it answers and its values in `values.go`; pin the value and add the row's ID to `specRequirementIDs`; run `go test ./internal/tenancy`, whose `Validate` holds the row to the specification's invariants; make the layer read the constant and declare its sites on the row, which **`make check-tenancy`** gates; and answer the rest of the [validation checklist](docs/development/tenant-policy-spec.md#validation-checklist) in the pull request. A change of value is its own pull request, and a change that claims to move policy without changing it is proved by `make tenancy-code-identity`. The full procedure, what the gate cannot see and how to choose the base that proof compares against are in [Adding or Changing a Limit](docs/development/adding-a-limit.md).

### encoding/json/v2: selectively, never as a migration

Go 1.27 has the v2 engine in its baseline, so **every `json.Marshal` here already runs on it** without a line changing and there is no performance case for adopting the API. What the explicit API buys is per-call options, and that is the only reason to reach for it.

**Never migrate a package wholesale to v2 semantics.** Under v2, `omitempty` means "empty in JSON" rather than "the Go zero", so an `int` at 0 and a `bool` at false stop being omitted: measured on this tree, **535 fields** in `internal/tools/` would start appearing in responses (the 834 pointers to a scalar and the 3044 strings would not). That moves the published surface, breaks the golden snapshots, and enlarges what a model pays tokens to read.

**Adopt it one call at a time, with v1's own semantics restored**, so what a valid document decodes to is exactly what it decoded to before and the only thing that changes is the option you named:

```go
jsonv2.UnmarshalRead(r, out, json.DefaultOptionsV1(), jsontext.AllowDuplicateNames(false))
```

`internal/oauth.decodeInstanceJSON` is the worked example. It refuses a body that names an object member twice, because encoding/json v1 keeps the last occurrence and reports nothing: an instance answering `{"id":1,"id":999}` used to admit the token under whichever id the reader happened to keep, and the identity is what every later decision is attributed to.

**The condition for using it is the caller's failure branch, not the document's origin.** It belongs where refusing is the safe answer. `fetchIntrospection` in the same file deliberately keeps encoding/json: a nil return there means "the endpoint did not answer", and `introspectToken` treats that as an instance it could not ask and assumes `api`, so refusing an ambiguous body would resolve it to **more** authority than either copy carries. Strictness on a fail-open path is a weakening, and it reads like a hardening, which is what makes it worth writing down.

The package exists because `jsonv2` is in Go 1.27's baseline experiment set, which is the toolchain's default and which nothing here configures. Turning it off is possible and would stop the import compiling, but no workflow, image or script in this repository sets `GOEXPERIMENT` at all, so it is a property of the dependency rather than something to guard against.

### Fields client-go does not model

A field GitLab sends that client-go's struct does not carry is read from the **captured response** rather than from a request of the handler's own ([ADR-0021](docs/development/adr/adr-0021-captured-response-for-fields-the-sdk-does-not-model.md)): wrap the context with `gitlabclient.WithResponseCapture`, pass it to the SDK call as always, and decode the capture into a small type naming the field as GitLab spells it. The transport hands the SDK the same bytes, so the route, the options, the retries, the pagination and the request inventory stay exactly what client-go makes them; a body that does not decode into the handler's type is an error the handler returns. The `invites` pattern (a `client.GL().NewRequest` of the handler's own) stays for an endpoint client-go has no method for at all. Each such gap is recorded in `docs/development/upstream-bugs.md` with the handler that carries it, since an upstream contribution retires it.

### Transport end-to-end modules

Two modules start the **real binary** and drive it the way a client does, both
without GitLab or credentials, and both run on every CI push:

- `test/e2e/http` (`httpe2e`) — the HTTP handler chain: cross-origin decisions,
  preflight, authentication modes, rate limiting, the JSON-RPC shape of a
  rejection, and the flags that restrict them.
- `test/e2e/stdio` (`stdioe2e`) — the stdio transport: pipes, process lifetime,
  stdout carrying nothing but JSON-RPC, logs on stderr, and the environment
  variables stdio configuration actually uses.

**They exist because the GitLab suite asks a different question.**
`test/e2e/gitlab` drives the same real binary, and drives it against a real
instance, so what it can answer is what a tool call does to GitLab. It reaches
the transport only as the pipe its own session happens to use: it never starts
the process twice, never reads stderr apart from stdout, never passes a flag,
and could not tell a listener that answers a preflight from one that does not.
These three need no GitLab at all and ask only about the process: how it is
started, what it writes where, what it accepts on a socket, and how it ends.
Both drive the binary; only these three are about the binary.

stdio is the primary transport and nothing drove it until `test/e2e/stdio`
existed. Two defects shipped through that gap: a nil dereference that killed the
process on an ordinary eliciting tool call, and a keepalive ping that closed the
session of any client speaking 2026-07-28 after 45 idle seconds — the second
held in place by a unit test asserting the ping ought to be there. Both were
found by hand against a binary, which is what these modules automate.

The two modules also hold a second kind of case: pins of what the go-sdk the
module is built against does with this server's answers today, where that is a
defect recorded in `docs/development/upstream-bugs.md` (rows 10, 11 and 68 are
held by `test/e2e/stdio/listen_end_test.go`,
`test/e2e/stdio/elicitation_declared_version_test.go` and
`test/e2e/http/session_not_found_test.go`). They fail on a dependency bump
rather than on a change of ours, with a message naming the entry and every page
that has to change with it, and a new pin of that kind belongs in one of these
modules rather than in a unit test, because what it holds is the SDK meeting the
real binary. A defect this server works around is the exception, since the real
binary no longer shows it: row 66, a tool result a middleware makes going out
with no `resultType`, is labeled by `toolutil.LabelForRevision`, its pin is a
unit test on a bare SDK server (`TestSDK_MiddlewareToolResult_GoesOutUnlabeled`
in `internal/toolutil`), and `test/e2e/http/result_type_test.go` holds the
answer the binary gives with the workaround in place.

When adding a transport-level behavior, put its test here rather than in a unit
test that reassembles the handler chain: a test that builds its own copy of the
thing under test is testing the copy.

### Test infrastructure

All tests mock GitLab with `httptest` through `internal/testutil` (`NewTestClient`, `RespondJSON`, `RespondJSONWithPagination`, and `CancelOnArrival`, which proves a handler passes its context). The mock **refuses what GitLab refuses**: every GraphQL request has its document and variables validated against the pinned schema, a refusal is reported with `t.Errorf` and never `t.Fatal`, and `testutil.AllowInvalidGraphQL(t)` exempts only a test that sends a malformed document on purpose; `testutil.GitLabQueryComplexity` holds a document's cost to the figure GitLab measured. The mock also **records** every request when `GITLAB_MCP_TEST_INVENTORY_DIR` names an absolute directory, which `make gen-request-inventory` merges into `docs/development/request-inventory.json`. The rules every test is held to:

- **Never `t.Fatal`/`FailNow` off the test goroutine** (httptest handlers, `go` statements, MCP handlers): `t.Errorf`, a deterministic response and `return`, or record with atomics and assert afterwards, per `.github/instructions/test-goroutines.instructions.md`; `make check-test-goroutines` gates it
- **Every case table runs under `t.Run`**, one subtest per case; `// sequential: <reason>` on the line above a loop declares dependent steps rather than cases, and `make check-test-subtests` gates it
- **An address nothing answers comes from `internal/testutil/netfixture`** (`DeadEndpoint`, or `NewStoppableServer` for an instance that answers and then goes away), never from a closed `httptest` server, whose freed port another listener can answer on; `make check-closed-servers` gates it
- **A test that compares a committed generated artifact calls `freshness.SkipIfDeferred(t)` first**, guarding the comparison and nothing else
- Test naming: `TestToolName_Scenario_ExpectedResult`; a `_test.go` exists only under the name of a module it tests, with the exemptions `make check-test-file-names` codifies

What each of these does, and why, is in [Test Infrastructure](docs/development/test-infrastructure.md).

### Request paths (R-PATH) and fine-grained grants (R-GRANT)

Two dimensions of the 1:1 audit look at the requests a handler sends rather than the surface it publishes: R-PATH at the requests the unit suite recorded, R-GRANT at the ones it derives from the handlers' code. **R-PATH** (`go run ./cmd/audit_1to1/ -scope=paths`, `make audit-1to1-paths`) was added after nine registered tools shipped unable to work while the other five dimensions were green. It reads `docs/development/request-inventory.json` and asks whether each owning package was observed sending a request, whether each GraphQL document validates, whether each REST endpoint exists in GitLab's documentation (`make audit-1to1-paths-endpoints`), whether GitLab's live record sends the fields we publish and which fields it sends that we do not (each marked by whether client-go models it), whether every list says where it ends and offers `page_token` only on a route the live record's schema 5 says can be paged by keyset (R-PAGE), whether a param is sent that the caller never asked to send, and whether each path was ever built from more than one fixture value. **R-GRANT** derives from each action's handlers what it sends, joins it to what a booted GitLab declares a fine-grained token needs, and generates the table the server enforces (`internal/tools/actiongrants/table_gen.go`, never edited by hand) together with the classic scope each action needs (ADR-0026); `make check-action-grants-derivation`, `make check-action-grants` and `make audit-1to1-grants` are steps 26 to 28 of `make analyze`. Every declaration table either reads fails on an entry that answers nothing. Both, with each check, oracle, gate and declaration table, are in [Request Paths and Fine-Grained Grants](docs/development/request-paths.md).

**None of that machinery may reach the server binary**: `internal/graphqlschema` and `internal/testutil` stay out of `cmd/server`'s import graph, held by `TestDependencies_TestSupport_NeverReachesTheServerBinary` in `cmd/server/main_test.go`.

### Build & test commands

```bash
go build ./...                           # Build all
go build -o dist/gitlab-mcp-server ./cmd/server  # Build binary
go test ./internal/... -count=1          # Run all unit tests
go test ./internal/tools/branches/ -count=1 -v  # Run domain tests verbose
go test ./internal/tools/ -run TestBranch -count=1  # Run specific tests
make golangci-lint                       # Consolidated Go formatting and linting

# End-to-end tests. The suite drives the real binary, so it is staged first;
# every target below does that for you through e2e-server-binary.
make test-e2e                                          # self-hosted GitLab from .env (alias of test-e2e-gitlab)
make test-e2e-ce                                       # ephemeral GitLab CE + runner + fixtures (Docker; GitLab peaks ~7.5 GiB, capped at 12 GiB)
make test-e2e-ee                                       # ephemeral GitLab EE, licensed; runs the common and ee packages
make test-e2e-harness                                  # the harness library's own tests: no GitLab
make test-e2e-http                                     # HTTP transport module: no GitLab, no credentials
make test-e2e-stdio                                    # stdio transport module: no GitLab, no credentials
make test-e2e-collector                                # telemetry into a real OTLP collector (Docker; skips without it)

# Compile-only check. The runtime is decided by the package, not a build tag,
# so all three are compiled together under the one `e2e` tag.
go test -tags e2e -c -o NUL ./test/e2e/gitlab/...          # Windows
go test -tags e2e -c -o /dev/null ./test/e2e/gitlab/...    # Linux

# Orbit live tests against GitLab.com (requires GITLAB_COM_TOKEN; auto-provisions fixtures)
GITLAB_COM_TOKEN=glpat-... go test -tags orbitlive -count=1 -v ./test/e2e/orbit/
make test-e2e-gitlab-com                                # Orchestrated: ensure token, setup fixtures, wait indexer, run live tests

# Model evaluation: the corpus put to a model against the real binary and a
# real GitLab. It asks a paid provider, so nothing schedules it.
MODELEVAL_MODELS=fake:perfect make modeleval-ce          # rehearse the whole pipe, no provider call, no cost
MODELEVAL_SPEND=yes MODELEVAL_BUDGET_USD=25 \
  MODELEVAL_MODELS='anthropic:claude-haiku-4-5-20251001' make modeleval-ce
MODELEVAL_SPEND=yes MODELEVAL_MODELS='...' make modeleval-ee   # the licensed half of the corpus
MODELEVAL_PROBE=yes make modeleval-probe                 # does each provider accept the request we build

# A run writes observation and scores nothing; the numbers are computed on the fold.
make model-results-record MODELEVAL_SHARDS=dist/modeleval/ce
make model-results-refold MODELEVAL_SHARDS=dist/modeleval/ce   # re-score under today's rules
make check-model-results                                        # the offline gate
```

`MODELEVAL_CASES` narrows a run to named case IDs, which is how one corrected case is re-run and re-folded without the other 257. **Keep the shards** under `dist/modeleval/`: the committed record holds scored columns, a redraw scores nothing, and a run whose shards were discarded can never be re-scored.

### Release process

**Major-version policy**: the server's major tracks client-go's major, moving in the same dependency bump, and every other dependency update ships as a minor or a patch. The rules that bite most often:

- **Rehearse before tagging.** `gh workflow run release.yml --ref <branch>` builds, smoke-tests and dry-runs every publisher from the dispatched tree, as the version its `VERSION` file carries, and publishes nothing; its publisher jobs run in the `release-rehearsal` environment, which no trusted publisher names.
- **Only the newest stable release moves "latest"** (`scripts/release-order.sh`): a backport publishes under its own version on every channel and moves no pointer.
- **Every release opens with a written summary**: the pull request that moves `VERSION` adds `.github/release-notes/vX.Y.Z.md` (Summary, Breaking changes, Upgrading, Security), which `make check-supply-chain` requires and GoReleaser publishes with `--release-header`; see [Release Process](docs/development/release-process.md).
- **Release link names are exact filenames**, never decorated: the Homebrew formula, winget, the installers and `scripts/fetch-release-assets.sh` look assets up by exact name.
- **What a generator reads from the environment ships in the committed file**, so a generator that describes the served surface pins its inputs in `cmd/internal/mcpsurface` (the per-surface listings and the offline stub client) rather than reading `GITLAB_MCP_TOOL_SURFACE`, `GITLAB_URL` or `GITLAB_TOKEN` at the call site. A generator whose job is to record a GitLab, such as `cmd/gen_graphql_schema` or `cmd/gen_orbit_record`, reads its credential on purpose.
- **`server.json` is stamped by the release, not edited by hand**, for what the release decides (the mcpb entries, the image digest, the versions), and its traps pass `make check-server-json` while being wrong: a `remotes` URL must be unique across the whole MCP Registry as a literal string, the OCI entries carry no `packageArguments`, `version`, `registryBaseUrl` or `fileSha256`, and `registryType: "mcpb"` means the bundle. `make check-server-json-packages` downloads what it declares.
- npm, PyPI and NuGet publish through OIDC trusted publishers with no stored token, each validated before an upload that cannot be replaced; LobeHub is the one manual step, `make publish-lobehub` after a release.

The whole workflow, channel by channel, is in [Release Process](docs/development/release-process.md), and how each artifact is built in [Distribution](docs/development/distribution.md).

### Post-implementation verification

After making changes, run targeted verification on the **changed files/packages only** (not the entire project):

```bash
# Go files — run on affected packages
go test ./internal/tools/branches/ -count=1    # tests on changed package
golangci-lint run --build-tags e2e ./internal/tools/branches/ # lint changed package

# Markdown files — run on specific changed files
npx markdownlint-cli2 docs/development/cmd-utilities.md README.md  # lint specific .md files
npx markdownlint-cli2 --fix docs/development/cmd-utilities.md      # auto-fix specific .md files

# README.md, docs/ and site page tables: normalize pipe tables, or verify with --check
go run ./cmd/format_md_tables/
go run ./cmd/format_md_tables/ --check

# Site pages (site/src/content/docs/<slug>.mdx and its es/ twin): the site's scripts run in site/
(cd site && pnpm -s run i18n:check)   # every English page has its Spanish twin, and the reverse; code blocks match outside comments
(cd site && pnpm -s run facts:check && pnpm -s run chips:check && pnpm -s run llms:check)
(cd site && pnpm -s run build && pnpm -s run lint)   # the whole site gate; lint reads the build
node scripts/check-doc-links.mjs   # from the root: tracked local links and site URLs, anchors included

# MCP Inspector (interactive tool testing UI at http://127.0.0.1:6274)
make inspector                             # compile + launch Inspector via stdio
make inspector-stop                        # stop Inspector and clean up

# Full project analysis (use sparingly — for pre-commit or CI)
make analyze                               # all analysis gates, full project
make analyze-fix                           # auto-fix what can be fixed
make analyze-report                        # generate LLM-consumable report
```

**Static analysis tools** (3 consolidated gates): `golangci-lint` (v2, 25+ linters plus `goimports`, `gofumpt`, and `gci` formatters), `govulncheck`, and `markdownlint-cli2`. Configuration: `.golangci.yml`, `.markdownlint-cli2.jsonc`. Full docs: `docs/development/static-analysis.md`.

**Markdown table formatter**: When creating or editing pipe tables in `README.md`, `docs/` or a site page (`site/src/content/docs`, `.mdx` included), run `go run ./cmd/format_md_tables/` to normalize source-readable padding and left/right/center alignment markers, then verify with `go run ./cmd/format_md_tables/ --check` before markdownlint.

**Formatting tools**: Before committing, always run `make analyze-fix` to apply configured Go formatters: `goimports` (import cleanup), `gofumpt` (stricter gofmt-compatible formatting), and `gci` (deterministic import section grouping).

### Environment variables

Settings this project defines are read as `GITLAB_MCP_<NAME>` and under no other spelling: a new one is added to `prefixedNames` in `internal/config/env_name.go` and read through `config.Getenv` or `config.TrimmedGetenv` there, never through `os.Getenv` (the one exception is `GITLAB_MCP_ENV_FILE`, read once before any dotenv file loads), and `make check-tenancy` (G14) fails on a `GITLAB_MCP_` variable read any other way that no register row or declaration in `cmd/audit_tenancy/declarations.go` answers, and on a name `config.Getenv` reads bare because the list does not hold it that no declaration there says is somebody else's. Only `GITLAB_URL`, `GITLAB_TOKEN`, the `OTEL_*` variables and the `AUTOPILOT` alias stay bare; the model evaluation's `MODELEVAL_*` and the test-only `GITLAB_MCP_TEST_*` variables configure the harness rather than the server and are outside the rule. A retired spelling still set is found by `config.RetiredEnvUses`: `GITLAB_READ_ONLY`, `GITLAB_SAFE_MODE` and `EXCLUDE_TOOLS` refuse startup and every other one warns. In HTTP mode an explicitly passed flag wins over its variable, which wins over the default, and an invalid value fails startup. A flag only HTTP mode reads, given to a stdio run, is named at startup, and refused when ignoring it would serve what it withholds or send the token to an instance it never named; a variable of a setting only HTTP mode has (`config.httpOnlyNames`) is never read on stdio and is named the same way.

| Variable | What it sets |
| --- | --- |
| `GITLAB_URL`, `GITLAB_TOKEN` | Instance and personal access token, classic or fine-grained (stdio); HTTP mode takes `--gitlab-url`, required unless `--allow-any-gitlab-url` |
| `GITLAB_MCP_SKIP_TLS_VERIFY` | Skip TLS verification for self-signed certificates |
| `GITLAB_MCP_ENV_FILE` | One dotenv file besides `~/.gitlab-mcp-server.env`; a working-directory `.env` is never loaded |
| `GITLAB_MCP_STDIO_MAX_LINE_BYTES` | Longest stdio message (4 MiB) |
| `GITLAB_MCP_MAX_LISTEN_STREAMS` | `subscriptions/listen` streams per credential (64; 512 per process, fixed) |
| `GITLAB_MCP_TOOL_SURFACE` | `dynamic` (default), `meta` or `individual` |
| `GITLAB_MCP_CAPABILITY_SURFACE` | Resources and prompts: `full` or `minimal` |
| `GITLAB_MCP_META_PARAM_SCHEMA` | Meta-tool input schema: `opaque` (default), `compact` or `full` |
| `GITLAB_MCP_READ_ONLY`, `GITLAB_MCP_SAFE_MODE` | Remove mutating actions, or answer them with a preview |
| `GITLAB_MCP_TIER` | Pin the licensing tier (`free`/`ce`, `premium`, `ultimate`); detected when unset |
| `GITLAB_MCP_IGNORE_SCOPES` | Skip the scope filter and the `read_api` narrowing |
| `GITLAB_MCP_EMBEDDED_RESOURCES` | Embed `gitlab://` URIs in get results (`true`) |
| `GITLAB_MCP_EXCLUDE_TOOLS` | Tools, groups or action IDs removed on every surface |
| `GITLAB_MCP_UPLOAD_MAX_FILE_SIZE` | Largest local file an upload or read accepts (`2GB`) |
| `GITLAB_MCP_MAX_HTTP_CLIENTS` | HTTP: pooled (token, URL) entries (100) |
| `GITLAB_MCP_SESSION_TIMEOUT`, `GITLAB_MCP_SESSION_REVALIDATE_INTERVAL` | HTTP: idle session timeout on `--stateless=false` (`30m`); token revalidation (`15m`), which on stdio re-reads a fine-grained token's grant |
| `GITLAB_MCP_YOLO_MODE` / `AUTOPILOT` | Skip the confirmation of destructive actions |
| `GITLAB_MCP_ALLOWED_UPLOAD_DIRS`, `GITLAB_MCP_ALLOWED_DOWNLOAD_DIRS`, `GITLAB_MCP_ALLOWED_IMPORT_DIRS` | Extra directories a tool may read from, write to, or import from (never in HTTP mode) |
| `GITLAB_MCP_ALLOW_PRIVATE_INSTANCES` | Permit private addresses for destinations the operator did not choose (ADR-0022) |
| `GITLAB_MCP_AUTH_MODE`, `GITLAB_MCP_PUBLIC_URL` | HTTP authentication, `legacy` or `oauth`, and the public https origin `oauth` requires |
| `GITLAB_MCP_TRUSTED_ORIGINS` | Origins allowed to make cross-origin requests |
| `GITLAB_MCP_ALLOW_PLAINTEXT_HTTP` | HTTP: plain HTTP on a listener off loopback without TLS (`true`, warned about while unset); `false` is the hardened mode, which refuses to start one (`ADM-015`) |
| `GITLAB_MCP_OAUTH_CACHE_TTL`, `GITLAB_MCP_OAUTH_CLIENT_UID` | OAuth identity cache TTL (`15m`) and the application uids admitted |
| `GITLAB_MCP_POOL_IDLE_TIMEOUT` | HTTP: reclaim an unused credential entry (`1h`) |
| `GITLAB_MCP_ACTION_TIMEOUT` | Cancel an action still running (`65m`) |
| `GITLAB_MCP_DRAIN_DELAY` | HTTP: answer `/health` with `503 draining` before closing (`0`) |
| `GITLAB_MCP_RATE_LIMIT_RPS`, `GITLAB_MCP_RATE_LIMIT_BURST` | Per-credential rate limit (off by default on stdio, 10 req/s in HTTP mode) and burst (40) |
| `GITLAB_MCP_AUTH_FAILURE_LIMIT`, `GITLAB_MCP_AUTH_FAILURE_WINDOW` | Failed authentications per address (an IPv6 one by its `/64`) before a block (10 per `1m`) |
| `GITLAB_MCP_AUTH_DISTINCT_TOKEN_LIMIT`, `GITLAB_MCP_AUTH_DISTINCT_TOKEN_WINDOW` | Distinct refused credentials per address (an IPv6 one by its `/64`) before an escalating block (50 per `10m`) |
| `GITLAB_MCP_DISCOVERY_RATE_LIMIT_RPS`, `GITLAB_MCP_DISCOVERY_RATE_LIMIT_BURST` | HTTP: one bucket per address for the two server cards and the RFC 9728 metadata (20 req/s, burst 200; `0` turns it off) |
| `GITLAB_MCP_CLIENT_COMPAT` | Per-client response compatibility (`auto`) |
| `GITLAB_MCP_DESCRIPTION_SUBSTITUTIONS` | Rewrite listed text for strict gateway validators |
| `GITLAB_MCP_LANGUAGE` | Language of the texts a client shows a person directly (`en`, the only one so far) |
| `GITLAB_MCP_TELEMETRY`, `GITLAB_MCP_TELEMETRY_*` | OTLP export (off), identity policy, its key and rotation, tool-name metric dimensions |
| `GITLAB_MCP_LOG_LEVEL` | Logging verbosity |
| `GITLAB_MCP_PPROF_ADDR` | Loopback-only `net/http/pprof` listener |
| `MODELEVAL_*` | Model evaluation harness (developer-only) |
| `GITLAB_MCP_TEST_INVENTORY_DIR` | Test transport request recording (developer-only) |
| `GITLAB_MCP_TEST_SNAPSHOT_PARITY` | `deferred` skips committed-artifact comparisons (`internal/freshness`) |

Every variable with its bounds, defaults, the code that reads it and the register row behind it, and every HTTP and general flag, is in [Environment Variables and Flags](docs/development/configuration.md).

---

## AI Assistance Infrastructure

This project includes a comprehensive set of AI agents, skills, and instruction files in `.github/` to support development workflows. All are oriented toward **development tasks**, not end-user usage.

### Instructions (Auto-loaded by File Pattern)

Instruction files in `.github/instructions/` are automatically applied when editing matching files:

| Instruction                                        | Applies to | Purpose                                                                   |
| -------------------------------------------------- | ---------- | ------------------------------------------------------------------------- |
| `go.instructions.md`                               | `**/*.go`, `**/go.mod`, `**/go.sum` | Idiomatic Go practices, naming, error handling, package rules             |
| `go-mcp-server.instructions.md`                    | `**/*.go`, `**/go.mod`, `**/go.sum` | MCP server patterns: tool registration, typed I/O, annotations, transport |
| `mcp-best-practices.instructions.md`               | `**/*.go`  | Protocol-level tool design, response formats, pagination, security        |
| `test-goroutines.instructions.md`                  | `**/*_test.go` | The six-rule contract for assertions off the test goroutine (`make check-test-goroutines`) |
| `security-and-owasp.instructions.md`               | `*`        | OWASP Top 10, input validation, secrets management, injection prevention  |
| `code-review-generic.instructions.md`              | `**`       | Code review priorities (Critical/Important/Suggestion), checklist         |
| `context-engineering.instructions.md`              | `**`       | Project structure principles for AI-readable code                         |
| `self-explanatory-code-commenting.instructions.md` | `**`       | Comment only WHY, not WHAT; avoid redundant comments                      |

### Agents and skills

The 7 agents in `.github/agents/` are invoked explicitly for a task; each file's description says when:

| Agent | File | When to use |
| --- | --- | --- |
| **Go MCP Server Expert** | `go-mcp-expert.agent.md` | Implementing or fixing MCP tools and handlers, MCP SDK questions; the primary coding agent |
| **Debug Mode** | `debug.agent.md` | Systematic bug investigation: reproduce, hypothesize, fix, verify |
| **Test Expert** | `test-expert.agent.md` | Writing and analyzing tests, coverage gaps, false passes, and refreshing the testing reference |
| **Plan Expert** | `plan-expert.agent.md` | Plans for features, refactors, tests, bugs, docs and upgrades, written to `plan/`; writes no code |
| **Documentation Writer** | `documentation-writer.agent.md` | Site pages in English and Spanish, and contributor pages in `docs/development/` |
| **Go Source Documenter** | `go-source-documenter.agent.md` | Godoc comments for source and test files |
| **SE: Reviewer** | `se-reviewer.agent.md` | Security review (OWASP, LLM security, Zero Trust) and architecture review |

The 18 skills in `.github/skills/` are task templates any agent can invoke:

| Skill | Purpose |
| --- | --- |
| `generate-project-documentation`, `update-project-documentation`, `update-starlight-docs` | A full documentation pass, a delta update after a code change, and the site's user pages with their Spanish twins |
| `go-source-documentation` | Godoc-compliant comments for Go files |
| `create-implementation-plan`, `create-architectural-decision-record`, `create-specification` | A phased plan in `plan/`, an ADR in `docs/development/adr`, a formal specification |
| `increase-test-coverage`, `review-and-refactor`, `golang-testing`, `golang-patterns` | Coverage to the target, a review and refactor against the instruction files, and the Go testing and idiom references |
| `create-mcp-evaluation`, `generate-release-notes`, `git-commit` | Evaluation Q&A pairs, categorized release notes between two refs, a conventional commit |
| `go-safe-move-refactor`, `modularize-go-package` | Moving Go files between packages, and splitting a package into domain sub-packages, building at every step |
| `create-mcp-tool`, `upstream-contribution` | A new MCP tool end to end, and a fix contributed to GitLab's own projects on gitlab.com |

---

## Common Development Workflows

### Adding a new GitLab API tool

1. **Plan**: Use `@Plan Expert` agent to define scope and generate implementation plan
2. **Specify**: Use `create-specification` skill if complex
3. **Test**: Use `@Test Expert` to write comprehensive tests (new tests or coverage analysis)
4. **Implement**: Use `@Go MCP Server Expert` to implement the tool
5. **Verify**: Run targeted analysis on changed packages (see "Post-implementation verification" above)
6. **Document**: Use `@Go Source Documenter` for code, then `update-project-documentation` skill for docs
7. **Commit**: Use `git-commit` skill with conventional commit format

### Increasing test coverage

1. Use `@Test Expert` agent — it runs `go test -coverprofile`, identifies gaps, detects false passes, generates documented tests, and refreshes `docs/development/testing/testing.md` with `go run ./cmd/gen_testing_docs/` at the end of the test phase
2. Or use `increase-test-coverage` skill for the same workflow invoked from any agent

### Reviewing code quality

1. Use `review-and-refactor` skill — reads all `.github/instructions/` files, reviews against them, then refactors
2. For security or architecture review: Use `@SE: Reviewer` agent (specify "review security" or "review architecture")

### Debugging a failing test or unexpected behavior

1. Use `@Debug Mode` agent — systematic 4-phase investigation
2. Provide the error message, test name, or failing behavior

### Checking library documentation

1. Use `@Go MCP Server Expert` agent — has Context7 integration, resolves library ID, fetches current docs
2. Useful for MCP SDK, GitLab client, or any Go dependency questions

### Updating documentation after changes

1. Use `update-project-documentation` skill: it analyzes the code delta, maps it to the affected pages and applies surgical updates. What a user reads is a site page (`site/src/content/docs/<slug>.mdx`), and its Spanish twin under `es/` changes in the same commit (`pnpm run i18n:check` catches a page with no twin and a code block that differs outside its comments, not prose left behind); what only a contributor needs goes to `docs/development/`. Either is written by hand unless step 2 names it, and nothing a user reads is written under `docs/` (ADR-0025)
2. A generated page or figure is never edited: change its source and rerun its command, `make gen-tool-reference` for the per-domain tool reference (and `cmd/gen_tool_reference/domains.json` for its prose), `make gen-action-grants` for the fine-grained permissions reference, `make bench-resources` for the performance measurements and charts (`make bench-resources-render` redraws them from the committed measurements), `make gen-site-stats` for `site/src/data/stats.json`, `make gen-footprint` for the token figures, `make gen-llms` for the `llms*.txt` files and `make gen-testing-docs` for `docs/development/testing/testing.md`; each has a `check-*` twin that CI runs
3. For full regeneration: Use `generate-project-documentation` skill

---

## Architecture Decisions

ADRs document key decisions in `docs/development/adr`:

| ADR      | Decision                                                       | Status                                       |
| -------- | -------------------------------------------------------------- | -------------------------------------------- |
| ADR-0004 | Modular sub-packages under `internal/tools/{domain}/`          | Accepted (181 `internal/tools` sub-packages; tools by tier: ~872 Free/CE, ~1027 Premium, ~1093 Ultimate self-managed, ~1099 GitLab.com Ultimate) |
| ADR-0005 | Meta-tool consolidation into a compact domain catalog          | Accepted (refines ADR-0004; its runtime mechanics are superseded by the catalog-first architecture of ADR-0014) |
| ADR-0006 | Raw GraphQL.Do() for domains without client-go service wrappers | Accepted (7 GraphQL-only domains)             |
| ADR-0007 | Rich error semantics for LLM-actionable diagnostics            | Accepted (WrapErrWithMessage, WrapErrWithHint) |
| ADR-0008 | Universal user identity across transport modes                 | Accepted, partially unimplemented (stdio and HTTP OAuth populate it; HTTP legacy does not) |
| ADR-0009 | Progressive GraphQL migration strategy                         | Accepted (trigger-based REST→GraphQL migration) |
| ADR-0010 | No resource subscribe capability                               | Superseded by ADR-0015                        |
| ADR-0011 | Low-token dynamic toolset mode                                 | Accepted (`gitlab_find_action` + `gitlab_execute_action`, the default surface) |
| ADR-0012 | Action catalog package name                                    | Accepted                                      |
| ADR-0013 | Documentation artifact boundaries                              | Superseded by ADR-0025                        |
| ADR-0014 | Catalog-first runtime architecture                             | Accepted (every surface is projected from `ActionSpec`; `cmd/audit_catalog_first` enforces it) |
| ADR-0015 | Polled resource subscriptions (supersedes ADR-0010)            | Accepted (26 subscribable kinds, 10 watchers/token and 512 per process, lease demotes rather than stops) |
| ADR-0016 | No webhook ingestion                                           | Accepted (`Reader` seam unchanged; no inbound HTTP surface added) |
| ADR-0017 | Pull-safe event sources surveyed and declined                  | Accepted (Events API, ActionCable, ETag probed live; polling stays the only freshness source; revisit triggers recorded) |
| ADR-0018 | Authorization admits at the minimum scope; writes gated per action | Accepted, narrowing refined by ADR-0026 (a read_api token is admitted; `tools/list` stays authenticated per the MCP authorization spec) |
| ADR-0019 | Audience binding is unavailable at the authorization server    | Accepted (GitLab publishes no `resource_indicators_supported`; `--oauth-client-uid` is the "otherwise verify" alternative) |
| ADR-0020 | One MCP server per configuration shape, owner-filtered delivery | Accepted (HTTP mode shares one server per shape; the credential is bound per request and resource-updated notifications are filtered by pool entry) |
| ADR-0021 | A field client-go does not model is read from the captured response | Accepted (`gitlabclient.WithResponseCapture` in the SDK transport chain; the `invites` pattern stays for an endpoint client-go has no method for) |
| ADR-0022 | An operator-named instance is exempt from the outbound destination guard | Accepted (tier A refuses cloud metadata addresses on every hop; tier B refuses private addresses only for a caller-named instance or an off-origin redirect; an address `--gitlab-url` named is never checked) |
| ADR-0023 | Tenant policy is declared once, in a register the layers read | Accepted (`internal/tenancy` holds every decision about who a caller is and what it may hold, `Validate` holds the rows to the specification's invariants, and the layers keep enforcing) |
| ADR-0024 | Fine-grained token authority is evaluated per action from GitLab | Accepted (a token whose scope list is `granular` is unknown authority and never narrowed to read-only by it; phase A withholds what no fine-grained token can reach at the GitLab release the permission table was recorded from, phase B lists what a readable grant reaches; the grant is never a key, and R-GRANT holds the generated table to the live record) |
| ADR-0025 | The site is the only home of user documentation (supersedes ADR-0013) | Accepted (user pages live in `site/src/content/docs` in English and Spanish, changed together; `docs/` keeps `docs/development` for contributors; the tool reference, fine-grained permissions reference, charts and figures are generated and gated) |
| ADR-0026 | A read_api token is served what GitLab accepts from it, derived per action (refines ADR-0018) | Accepted (each action's classic scope is derived from the requests it sends, declared where GitLab departs from its method rule, and held by gates 4 and 5; a `read_api` token is served what that scope reaches, not the read-only surface, and the tool reference prints the classic and fine-grained token lines) |

### Modular tools sub-packages (ADR-0004)

The `internal/tools/` package family is split into 182 packages, the root package plus 181 sub-packages. Runtime tool surfaces are projected from canonical `ActionSpec` and surface specs. Package-local `RegisterTools` functions have been removed for ordinary GitLab API actions; the catalog-first runtime is the exclusive registration model. This provides:

- Package-level namespace eliminates need for domain prefixes on types (`branches.Output` vs old `BranchOutput`)
- Each sub-package is independently testable with isolated `httptest` mocks
- Zero import cycles, which the compiler enforces; it forbids cycles, not imports. A sub-package imports `toolutil/` and, where it publishes a row another package owns and judges, that package: a shared data package (`labeldata`, `iterationdata`, `protectedenvdata`) or a domain (`commits.Output` in a repository comparison, `projects.BasicOutput` in the job token allowlist, `events.ContributionEventOutput` in a user's events). Otherwise a shape two packages both render is mirrored rather than imported, the convention the comments call C-IMPORTS
- `internal/tools/register.go` registers individual tools from the canonical action catalog projection
- Validated by catalog and source guardrails such as `TestRegisterAllDoesNotUseDomainRegisterTools` and ActionSpec coverage audits

### Markdown registry pattern

Markdown formatters use a type-based registry in `internal/toolutil/md_registry.go` instead of a central dispatch switch. Each sub-package self-registers its formatters via `init()` functions:

- `toolutil.RegisterMarkdown[T](fn)` — registers a formatter for output type `T`
- `toolutil.RegisterMarkdownResult[T](fn)` — registers a formatter for `*mcp.CallToolResult` types
- `toolutil.MarkdownForResult(result any)` — looks up and invokes the registered formatter by `reflect.Type`
- `internal/tools/markdown.go` is a thin delegator (~17 lines) that calls `toolutil.MarkdownForResult`
- ~579 registrations across 166 sub-packages, validated by `TestAllMarkdownFormattersRegistered`

A formatter escapes what it interpolates: `toolutil.EscapeMdTableCell` on every GitLab-authored string between two pipes of a table row and on every single-line list value, `toolutil.EscapeMdHeading` on the one value that lands in a heading, and `toolutil.MdTitleLink` on both halves of a link rather than hand-writing `[%s](%s)`. A code block around GitLab-authored content is built with `toolutil.MarkdownFencedBlock`, or its fence sized with `toolutil.MarkdownCodeFence`, never written as three backticks: a fixed fence is closed by the first run of three the body contains, and everything after it renders as Markdown of the response. `make check-md-escaping` (`cmd/audit_md_escaping`) is the gate for both; a value that needs none is declared in its own package with `//gitlab:allow-unescaped <expression>: <reason>`, and a declaration that excuses nothing fails too.

### Dynamic toolset mode

`GITLAB_MCP_TOOL_SURFACE=dynamic` registers only `gitlab_find_action` and `gitlab_execute_action`. It is the default when `GITLAB_MCP_TOOL_SURFACE` is unset. The dynamic registry is built from the canonical action catalog shared with meta-tools and augmented with standalone routes such as project discovery, so execution reuses existing handlers, typed schemas, destructive-action classification, read-only filtering, safe-mode previews, markdown formatters, and scope filtering.

Developers add normal GitLab actions through domain-local `ActionSpecs` and the audited catalog aggregation path. `internal/tools/action_catalog.go` builds the canonical catalog from those specs; meta-tools register visible domain dispatchers from it, dynamic mode builds find/execute over it, and individual mode projects one visible tool per action from the same catalog. Do not add package-local `RegisterTools` functions, duplicate dynamic-only action definitions, or package-level meta registration for ordinary GitLab API operations. See `docs/development/tool-surfaces-and-action-core.md` for the detailed developer architecture.

Find combines canonical `domain.action` IDs, domain/action names, aliases, natural-language stopword filtering (removing frequent non-informative words), synonyms, fuzzy matching, and segmented matching for multi-intent prompts. Models should use `gitlab_find_action` to retrieve exact schemas, then execute the canonical action ID returned by find. See [dynamic tools](https://docs.jmrp.io/gitlab-mcp-server/tools/dynamic-tools/) and ADR-0011.

**The `related_actions` a result carries are narrowed to the session that reads them.** The curated lists in the `ActionSpec`s are written against the whole catalog and every session is served a narrower one, so a Free instance used to be handed cross-links to Premium actions and an `--exclude-tools` deployment links to actions it had removed. `Registry.publishedRelatedActions` decides by what following the link would answer: one that resolves here is published under its canonical ID whatever spelling it was written in, one a filter withheld is kept because asking for it produces the narrowing rather than a typo correction (`withheldActionMessage`), and one neither can explain is dropped, since `unknown action` plus near-miss suggestions reads as a spelling correction and teaches a model the capability is absent. A filter there includes the tier since issue 956: `tools.TierWithheld` names every action the tier removed (what a catalog built without the tier filter holds and the session's does not, never one the operator excluded by name), `dynamiccatalog.Build` records it as `WithheldActions.ByTier`, and `dynamic.WithTierWithheld` makes execute answer one with the tier it needs and whether the session's tier was pinned or detected, asking the operator, and with read-only mode or the scopes the credential lacks when either would still withhold the action once the tier served it, so the operator is not sent to pin a tier that only uncovers the next refusal. Measured at the Free tier the dropped set was 5 links in 3 packages, all of them Free actions naming Premium siblings, and is 0 now.

**What one find call may cost is bounded, in three places that belong together.** The query is capped at `dynamic.MaxSearchQueryLength` (256 characters, published as the schema's `maxLength` and refused, never truncated), because a search costs the word count times the catalog three times over (lexical, fuzzy, and one pass per segment window) and a request body may be 4 MiB. The scoring passes check the context, so an abandoned POST stops costing and a deadline can end one; a cancelled search returns the error rather than an empty result. And the handler takes `toolutil.WithActionDeadline` itself, because it is registered directly rather than through one of the `WrapAction` functions and so passed through no deadline at all. `BenchmarkFind_PathologicalQuery` is what keeps a scorer change from quietly raising the ceiling.

### Enterprise tool gating

The licensing tier (`GITLAB_MCP_TIER`, `--tier`, or detected per credential) decides which actions exist on every surface, and `pruneSchemaFieldsByTier` in `internal/tools/action_catalog.go` prunes each action's input schema strictly and its output schema leniently, by the `tier:"premium"` / `tier:"ultimate"` struct tags on its fields. An action's minimum tier is its `ActionSpec.Edition`, set by the domain package or for a whole package by `editionTaggedSpecs` in `internal/tools/action_specs.go`, and an empty one is Free. On meta, 17 dedicated groups are Premium or Ultimate (an eighteenth, `gitlab_orbit`, exists on GitLab.com only) and paid actions join seven base groups; on dynamic, find never lists and execute never runs an action above the resolved tier, and execute answers one as withheld with the tier it needs. The groups and actions each tier adds, surface by surface, are in [Tier Gating](docs/development/tier-gating.md).

### OAuth admission, per-action write gating, and HTTP routing

Eight invariants of HTTP mode that are easy to break by touching the wrong layer, each explained in full in [HTTP Mode Invariants](docs/development/http-invariants.md):

- **One `mcp.Server` per configuration shape, not per credential** (ADR-0020). `serverShapeKey` names what a server is built for, the credential lives on its `serverpool.Entry` and is bound to each request, and the binding middleware is added after the telemetry, rate-limit, listen-ceiling and subscription middlewares so that it runs before them. Notifications are filtered by owner on the way out, by a middleware that reads the params, never the request, and never writes to them.
- **Admission asks for the minimum, not the deployment's scope**: `read_api` (`oauth.MinimumScope`) at the door, and a `read_api` token is served what GitLab accepts from it, derived per action (ADR-0026). The scope filter is applied to the catalog, never to registered tool names, and a fine-grained token is unknown authority whose grant decides per action (ADR-0024) and never reaches a shape, catalog or manifest key.
- **Routing happens before authentication, and the gate resolves only what can act on a session**: every POST, and GET and DELETE on `--stateless=false`; every other request, a CORS preflight included, passes it unchecked.
- **The card routes serve two different documents**: the SEP-2127 card at `/server-card` and `/mcp/server-card`, and the enumerating SEP-1649 document at `/.well-known/mcp/server-card.json`, each with an `ETag` that hashes the document's bytes and nothing about the process.
- **There is one Host guard, and it is ours**, so `StreamableHTTPOptions.DisableLocalhostProtection` stays set.
- **The instance is chosen from an allow-list, and verification follows it**: the OAuth token cache keys on instance and token.
- **Held calls are counted where the SDK dispatches them, and the gate counts only what its headers prove**, against ceilings derived from the descriptor limit, with no per-credential ceiling beside them.
- **The POST is the lifetime of the calls it carries, and the handler learns that through a header**, because the handler context does not descend from the HTTP request.

Nothing here should need a reverse proxy to be correct: the binary answers its own CORS preflights, its own 404s, its own security headers, and can terminate TLS or listen on a unix socket itself.

### Resource subscriptions (ADR-0015)

`resources/subscribe` is honored by polling, on `GITLAB_MCP_CAPABILITY_SURFACE=full` only. `internal/subscriptions/` owns the watchers and knows nothing about MCP; `cmd/server/subscriptions.go` is the bridge. A whitelist of 26 kinds is subscribable, reads go through the registered resource handler, the session is the subscriber identity, the manager is per credential (10 watchers each, 512 per process), a joiner waits for the first read, the lease demotes a quiet watch rather than stopping it, stateless HTTP refuses only the legacy path, and ending a watch closes the client's `subscriptions/listen` stream. Do not use `synctest` for a test that needs a real HTTP backend. Why each holds is in [Resource Subscriptions](docs/development/resource-subscriptions.md).

---

## Debugging Tips (Development)

### MCP transport debugging

The server communicates via stdio (JSON-RPC over stdin/stdout). To debug:

```bash
# Run with debug logging
GITLAB_MCP_LOG_LEVEL=debug ./gitlab-mcp-server 2>debug.log

# HTTP mode for easier debugging with curl
./gitlab-mcp-server --http --http-addr=localhost:8080
curl -X POST http://localhost:8080/mcp -H "Content-Type: application/json" -H "Accept: application/json, text/event-stream" -H "GITLAB-URL: $GITLAB_URL" -H "PRIVATE-TOKEN: $GITLAB_TOKEN" -d '{"jsonrpc":"2.0","method":"tools/list","id":1}'
```

### Common issues

- **TLS errors**: Set `GITLAB_MCP_SKIP_TLS_VERIFY=true` for self-signed certs
- **Tool not found**: Check the action's `ActionSpec`, catalog aggregation, `action_catalog.go`, and `docs/development/tool-surfaces-and-action-core.md` for surface ownership rules
- **Meta-tools disabled**: `GITLAB_MCP_TOOL_SURFACE` decides; unset it for the default dynamic surface, set `individual` for one tool per action, or `meta` when meta-tools are what you want. The `META_TOOLS` boolean it replaced was removed in 3.0.0
- **Dynamic mode shows only two tools**: this is expected by default. Use `gitlab_find_action` and `gitlab_execute_action`; set `GITLAB_MCP_TOOL_SURFACE=meta` to use meta-tools.
- **Pagination missing**: Ensure the list handler applies `toolutil.ApplyListOptions` to the request and fills its output with `toolutil.PaginationFromResponse` (plus `AdjustPagination` on a route that sends no totals, which is GitLab's search API: it numbers the page and derives `has_more`, and fills the totals only on a lone page, since nothing on any other page counts the result), and that a list GitLab pages by keyset passes `toolutil.KeysetContinuation` on its SDK call, or the `next_page_token` its pages publish cannot be followed
- **Test mocking**: All tests use `httptest.NewServer` — check URL routing in mock handler

### Running specific test domains

```bash
go test ./internal/tools/ -run TestBranch -count=1 -v    # Branch tools
go test ./internal/tools/ -run TestMR -count=1 -v         # Merge request tools
go test ./internal/tools/ -run TestPipeline -count=1 -v   # Pipeline tools
go test ./internal/resources/ -count=1 -v                  # Resources
go test ./internal/prompts/ -count=1 -v                    # Prompts
```

### Running E2E tests

The suite drives the **real `cmd/server` binary** over stdio against a real GitLab, so a tool call crosses a process boundary and then the network; read `test/e2e/README.md` before changing any of it. **The package decides the runtime, not a build tag**: `test/e2e/gitlab/common` for what any instance serves (176 test files), `ce` for what only an unlicensed one does (7), `ee` for Premium and Ultimate (49), all three compiled together under the one `e2e` tag. Every target stages the binary first, so none of them is a bare `go test`:

```bash
make test-e2e        # self-hosted, from .env (GITLAB_URL, GITLAB_TOKEN); a package the instance cannot serve skips
make test-e2e-ce     # ephemeral GitLab CE + runner + fixtures: common and ce
make test-e2e-ee     # ephemeral GitLab EE, licensed: common and ee
```

A test is named for what it exercises rather than for a surface, because **one scenario runs on all three surfaces** (`harness.SurfacesWith`). What a run covered is recorded rather than declared, and `make check-e2e-static` fails on a catalog action no scenario names (an action no Docker instance can run is declared in `cmd/audit_e2e_coverage/exemptions.go`). `make test-e2e-ce` and `make test-e2e-ee` are the complete runs: a skip `cmd/audit_e2e_coverage/skip_declarations.go` does not declare fails them, and every Docker run stops before the tests on a GitLab newer than the release the fine-grained action table records. How the images are kept current and what each gate refuses is under A complete run in `test/e2e/README.md`.
