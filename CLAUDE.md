# gitlab-mcp-server — AI Development Context

> This file provides comprehensive context for AI assistants working on this project.
> All project artifacts must be written in **English**. Conversations may be in any language.
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

## Project Overview

**gitlab-mcp-server** is a Model Context Protocol (MCP) server written in Go that exposes GitLab REST API v4 and GraphQL operations as MCP tools for AI assistants. It runs as a local binary communicating via stdio or HTTP transport.

| Attribute     | Value                                               |
| ------------- | --------------------------------------------------- |
| Language      | Go 1.27.1                                           |
| MCP SDK       | `github.com/modelcontextprotocol/go-sdk/mcp` v1.7.0 |
| GitLab Client | `gitlab.com/gitlab-org/api/client-go/v2` v2.59.0       |
| Transport     | stdio (primary), HTTP (optional)                    |
| Platforms     | Windows, Linux & macOS, amd64 & arm64               |
| Version       | 2.7.5                                               |

### Scale

| Metric                    | Count                                                                                                        |
| ------------------------- | ------------------------------------------------------------------------------------------------------------ |
| MCP Tools (individual)    | By instance tier: ~854 Free/CE; ~1006 Premium; ~1072 Ultimate (self-managed) / ~1078 on GitLab.com Ultimate with Orbit |
| Catalog groups            | By instance tier: 28 Free/CE; 34 Premium; 45 Ultimate                                                       |
| Meta-mode tools           | 32 base (Free/CE) / 49 self-managed Ultimate / 50 GitLab.com Ultimate (Orbit)                              |
| Dynamic-mode tools        | 2 dynamic tools (`gitlab_find_action`, `gitlab_execute_action`) — see Dynamic toolset mode below |
| MCP Resources             | 45 across dynamic/full, meta/full, and individual/full modes; `gitlab://tools` adapts to the active surface |
| MCP Prompts               | 37 (12 core + 4 cross-project + 4 team + 5 project-reports + 4 analytics + 4 milestone-label + 2 git-workflow + 2 audit)      |
| Completion argument types | 17                                                                                                           |
| MCP Capabilities          | 4 (progress, elicitation, completions, resource subscriptions)                     |
| MCP Icons                 | 51 icons (50 domain + brand mark), each a 3-entry `[]mcp.Icon`: one SVG (base64 data URI, `Sizes: ["any"]`, `currentColor`) plus light/dark 16×16 lossless WebP fallbacks (`Theme`-tagged, `cmd/gen_icon_webp`) for clients that reject SVG. The brand mark is the generated "fan-out" (`cmd/gen_brand` → `brandmark_gen.go`), original artwork replacing the former tanuki |
| Source files (tools)      | 760 non-test Go files under `internal/tools/`                                                                |
| Test files (tools)        | 364 test files under `internal/tools/`                                                                       |
| Go packages               | 241 total; 176 under `internal/tools/...`                                                                    |

### Orbit live tests

The six read-only `gitlab_orbit_*` tools (`status`, `schema`, `tools`, `dsl`, `query`, `graph_status`) target GitLab.com's experimental Knowledge Graph API. They have an `orbitlive`-gated live test suite at `test/e2e/orbit/live_test.go` (4 suites, 41 subtests) that exercises the real `https://gitlab.com/api/v4/orbit/*` endpoints. The live test fixtures (`kg-fixtures`, `security-fixtures`) live under `test/fixtures/orbit/` and are provisioned by `scripts/setup-orbit-fixtures.sh`. End-to-end orchestration is exposed as `make test-e2e-gitlab-com` (runs setup, waits for the indexer, then runs the live tests). See [Orbit live test fixtures](docs/development/orbit-fixtures.md) for fixture contents, namespace configuration, and the indexer caveat.

## Project Structure

```text
gitlab-mcp-server/
├── cmd/
│   ├── server/                  # MCP server entry point and --shutdown support
│   ├── audit_1to1/              # Consolidated 1:1 SDK↔API parity audit (R-INPUT/R-OUTPUT/R-ACTION/R-META + merge)
│   ├── audit_catalog_first/     # Enforces catalog-first registration invariants (ADR-0004)
│   ├── audit_discovery_completeness/ # Audits discovery metadata (aliases/usage/related/param-guidance/sibling-cluster; input-enum candidates) — META-001
│   ├── audit_doc_coverage/      # Audits docs/tools/*.md vs canonical action catalog (DOC-002); reads doc-ownership.json
│   ├── audit_dynamic_aliases/   # Audits dynamic discovery aliases
│   ├── audit_e2e_gaps/          # Reports catalog actions not exercised by the e2e suite (make audit-e2e-gaps)
│   ├── audit_edition_tier/      # Audits doc-grounded edition tier gating (Free/Premium/Ultimate)
│   ├── audit_gateway_chars/     # Audits served descriptions/titles for characters MCP gateway validators reject (make check-gateway-chars)
│   ├── audit_install_buttons/  # Decodes every one-click install payload (base64 or percent-encoded JSON) and holds the buttons to one configuration per command (make check-install-buttons)
│   ├── godoc_tool/              # Consolidated Go doc auditor + fixer (was audit_godocs + add_docs)
│   ├── audit_metrics/           # Audits MCP tool/resource/prompt metrics
│   ├── audit_readonly_graphql/  # Fails when an action classified ReadOnly can reach a GraphQL mutation, which `--read-only` and a read_api token's narrowed surface would both keep (make check-readonly-graphql)
│   ├── audit_supply_chain/      # Audits five release-configuration invariants: SHA-pinned uses:, credentialed jobs that run no run-time-resolved code, stated Dependabot cooldowns, a current SECURITY.md, signature-verifying installers (make check-supply-chain)
│   ├── audit_surface_quality/   # Consolidated surface audit: metadata violations + output quality (was audit_tools + audit_output)
│   ├── audit_test_goroutines/   # Audits testing.T aborts made off the test goroutine (A/B categories, --check gate)
│   ├── audit_test_names/        # Audits test function naming compliance; -check-files gates test-file naming (make check-test-file-names)
│   ├── audit_test_subtests/     # Audits case loops that assert without a t.Run subtest; -fix rewrites the unambiguous ones (make check-test-subtests)
│   ├── audit_tokens/            # Audits token usage for model-facing surfaces (+ --compare-schemas sizing spike)
│   ├── eval_mcp_surfaces/       # Evaluates model-facing MCP surface behavior
│   ├── audit_string_dupes/      # Finds duplicated string literals missing constants
│   ├── format_md_tables/        # Formats Markdown pipe tables in README.md and docs/
│   ├── gen_action_catalog_manifest/ # Generates audited action catalog manifest
│   ├── gen_brand/               # Emits every vector brand asset from one parametric geometry (mark, favicon, banner/OG/social cards, in-binary svgBrand)
│   ├── gen_docker_tools/        # Generates Docker-related tool metadata
│   ├── gen_icon_webp/           # Regenerates light/dark WebP icon fallbacks from icons.go (maintainer-only, requires rsvg-convert + cwebp)
│   ├── gen_lhm_manifest/        # Generates the capability arrays in lhm.plugin.json (LobeHub)
│   ├── gen_llms/                # Generates llms.txt and llms-full.txt for LLM discovery
│   ├── gen_stats/               # Generates README stats section from codebase metrics
│   └── gen_testing_docs/        # Generates docs/development/testing/testing.md
├── internal/
│   ├── config/                  # Configuration loading (.env, flags, env vars)
│   ├── gitlab/                  # GitLab API client wrapper (client.GL() accessor)
│   ├── oauth/                   # OAuth HTTP mode: token cache, GitLab verifier, header middleware, RFC 9728 metadata
│   ├── serverpool/              # HTTP mode: bounded LRU pool of per-token+URL MCP servers (with observability metrics)
│   ├── subscriptions/          # resources/subscribe: polled watchers, cadence, leases (ADR-0015)
│   ├── toolutil/                # Shared tool utilities (errors, pagination, markdown, logging)
│   ├── testutil/                # Shared test helpers (NewTestClient, RespondJSON)
│   ├── tools/                   # Tool orchestration layer + 176 internal/tools packages
│   │   ├── register.go          # RegisterAll() — projects individual tools from the canonical action catalog
│   │   ├── register_meta.go     # RegisterAllMeta() — registers catalog-backed meta groups and standalone surfaces
│   │   ├── dynamic/             # Low-token dynamic find/execute surface over catalog routes
│   │   ├── markdown.go          # Thin delegator to the type-based Markdown registry (toolutil.MarkdownForResult)
│   │   ├── metatool.go          # Meta-tool registration: addMetaTool (DeriveAnnotations), addReadOnlyMetaTool, route wrappers
│   │   ├── errors.go            # Error helpers (WrapErr, WrapErrWithMessage, WrapErrWithHint, ExtractGitLabMessage)
│   │   ├── logging.go           # logToolCall helper
│   │   ├── pagination.go        # Pagination type aliases
│   │   ├── branches/            # Branch & protected branch tools
│   │   ├── cilint/              # CI lint tools
│   │   ├── civariables/         # CI variable tools
│   │   ├── commits/             # Commit tools
│   │   ├── deployments/         # Deployment tools
│   │   ├── elicitationtools/    # Interactive creation flows (MCP elicitation)
│   │   ├── environments/        # Environment tools
│   │   ├── files/               # Repository file tools
│   │   ├── groups/              # Group tools
│   │   ├── health/              # Health/version check tools
│   │   ├── integrations/        # Project and group integration tools (incl. group Datadog)
│   │   ├── issuelinks/          # Issue link tools
│   │   ├── issuenotes/          # Issue note tools
│   │   ├── issues/              # Issue CRUD tools
│   │   ├── jobs/                # CI job tools
│   │   ├── labels/              # Label tools
│   │   ├── members/             # Project member tools
│   │   ├── mergerequests/       # Merge request CRUD tools
│   │   ├── milestones/          # Milestone tools
│   │   ├── mrapprovals/         # MR approval tools
│   │   ├── mrchanges/           # MR changes/diff tools
│   │   ├── mrdiscussions/       # MR discussion tools
│   │   ├── mrdraftnotes/        # MR draft note tools
│   │   ├── mrnotes/             # MR note tools
│   │   ├── packages/            # Package registry tools
│   │   ├── pipelines/           # Pipeline tools
│   │   ├── pipelineschedules/   # Pipeline schedule tools
│   │   ├── projects/            # Project CRUD tools
│   │   ├── releaselinks/        # Release link tools
│   │   ├── releases/            # Release tools
│   │   ├── repository/          # Repository tree/compare tools
│   │   ├── search/              # Search tools (code, MRs, issues, etc.)
│   │   ├── projectdiscovery/   # Git remote URL to GitLab project resolution
│   │   ├── tags/                # Tag tools
│   │   ├── todos/               # Todo tools
│   │   ├── uploads/             # Project upload tools
│   │   ├── users/               # User tools
│   │   └── wikis/               # Wiki tools
│   ├── resources/               # 45 MCP resource implementations
│   ├── prompts/                 # 37 MCP prompt implementations
│   ├── completions/             # 17 argument completion types
│   ├── progress/                # MCP progress notifications
│   ├── elicitation/             # MCP elicitation capability
├── docs/                        # Project documentation (Diátaxis framework)
│   ├── adr/                     # Architectural Decision Records
│   ├── tools/                   # Per-domain tool documentation
│   ├── capabilities/            # MCP capability docs
│   ├── examples/                # Usage examples
│   ├── oauth-app-setup.md       # Creating GitLab OAuth applications for MCP clients
│   └── ide-configuration.md     # Per-IDE MCP JSON configuration (stdio, HTTP legacy, OAuth)
├── test/e2e/                    # End-to-end integration tests
│   ├── http/                    # HTTP transport module (`httpe2e`): cross-origin, preflight, auth modes, rate limiting, nginx layer. Needs no GitLab
│   ├── stdio/                   # stdio transport module (`stdioe2e`): drives the real binary over pipes. Needs no GitLab
│   ├── docker-compose.yml       # Ephemeral GitLab CE + Runner + fixture service for Docker mode
│   ├── .env.docker              # Docker mode environment variables
│   ├── README.md                # E2E documentation
│   ├── scripts/                 # E2E provisioning scripts (setup, runner, wait)
│   └── suite/                   # Go test package (166 test files)
│       ├── setup_test.go        # MCP server/client setup, test helpers, shared state
│       └── fixture_ce_test.go   # Self-contained GitLab resource builders (CE runtime)
│       └── fixture_ee_test.go   # Self-contained GitLab resource builders (EE runtime)
├── plan/                        # Implementation plans for features
├── mcpb/                        # Claude Desktop extension (.mcpb) manifest + icon (packed by scripts/build-mcpb.sh)
├── .github/                     # AI assistance infrastructure
│   ├── copilot-instructions.md  # GitHub Copilot context (auto-loaded by VS Code)
│   ├── agents/                  # 7 specialized AI agents
│   ├── skills/                  # 18 reusable skill templates
│   └── instructions/            # 7 coding standard instruction files
├── Makefile                     # Build, test, lint targets
└── VERSION                      # Semantic version (2.7.5)
```

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

## Key Development Patterns

### Adding a New MCP Tool

1. Create `internal/tools/{domain}/` sub-package directory
2. Create `{domain}.go` with typed input/output structs (no domain prefix — package provides namespace)
3. Create `{domain}_test.go` with table-driven tests using `testutil.NewTestClient` and `httptest`
4. Add or update domain-local `ActionSpecs` so the action has one canonical route, owner package, metadata, compatibility policy, individual projection metadata, and tests.
5. If the domain is new, update the generated/audited catalog aggregation path rather than adding ad hoc root registration calls.
6. Add markdown formatters in the sub-package `markdown.go` `init()` function using `toolutil.RegisterMarkdown[T]` with appropriate content annotations (`ContentList`, `ContentDetail`, `ContentMutate`)
7. For list formatters: add `toolutil.HintPreserveLinks` as the first hint in `WriteHints()` to instruct the LLM to preserve clickable links
8. Add clickable `[text](url)` links in Markdown table columns where applicable (MRs, issues, pipelines, etc.)
9. Meta-tools automatically get `next_steps` in JSON via `enrichWithHints()` — no extra work needed
10. Update `docs/tools/{domain}.md` and `docs/reference/tools/README.md`
11. After completing a test-focused tool implementation phase, run `go run ./cmd/gen_testing_docs/` or `make gen-testing-docs` to refresh `docs/development/testing/testing.md`, then verify with `go run ./cmd/gen_testing_docs/ --check`

See `docs/reference/output-format.md` for the complete response format specification.

### Tool naming convention

A tool's runtime name depends on the surface, and on the individual surface it is **declared**, not derived — `IndividualTool.Name` in the domain's `ActionSpec` — so never infer one from a formula.

- **Individual surface** (`GITLAB_MCP_TOOL_SURFACE=individual`): the prevailing form is domain-first `gitlab_{domain}_{action}` — `issue.list` is `gitlab_issue_list`, `project.get` is `gitlab_project_get`, `branch.create` is `gitlab_branch_create`. A large legacy set is verb-first instead (`gitlab_list_issue_discussions`, `gitlab_get_issue_statistics`, `gitlab_add_ssh_key`), and both forms are real. New actions take the domain-first form.
- **Meta surface** (`GITLAB_MCP_TOOL_SURFACE=meta`): mostly the bare domain — `gitlab_issue`, `gitlab_project`, `gitlab_group` — with the operation in the `action` argument, alongside a few standalone tools (`gitlab_discover_project`, `gitlab_server`, the `gitlab_interactive_*` elicitation flows). A domain **without** a meta-tool of its own is a set of routes on a base one, not a tool: epics, labels, milestones, boards, members, wikis and releases at group scope are actions on `gitlab_group`; issue discussions and statistics are actions on `gitlab_issue`; todos are actions on `gitlab_user`.
- **Dynamic surface** (the default): only `gitlab_find_action` and `gitlab_execute_action`, which take the canonical catalog ID directly — `{"action": "issue.list", "params": {…}}`. This is the portable form for an example, because it does not depend on `GITLAB_MCP_TOOL_SURFACE`.

Every documentation example must name a tool the surface it shows actually registers, and say which surface that is — an individual-tool example without `GITLAB_MCP_TOOL_SURFACE=individual` is wrong twice over, since the default surface registers neither. `go run ./cmd/audit_doc_tool_names/` checks every `gitlab_*` mention across `docs/`, `site/src/content/docs/`, `README.md` and this file against the names the server really registers; `--check` makes it a gate.

### Error handling in tool handlers

Four error wrapping functions in `internal/toolutil/errors.go`, used across the 176 packages under `internal/tools/`:

- `WrapErr(op, err)` — read-only operations (list, get, search). Generic classification only.
- `WrapErrWithMessage(op, err)` — mutating operations (create, update, delete). Includes GitLab-specific error detail via `ExtractGitLabMessage`.
- `WrapErrWithHint(op, err, hint)` — when a specific corrective action is known (e.g., "use gitlab_branch_unprotect first"). Includes detail + actionable suggestion.
- `WrapErrWithStatusHint(op, err, code, hint)` — combines `IsHTTPStatus` check + `WrapErrWithHint` in a single call. Use when the hint applies only to a specific HTTP status code; returns `WrapErrWithMessage` for all other codes.
- `NotFoundResult(resource, identifier, hints...)` — for get handlers when `IsHTTPStatus(err, 404)`. Returns an informational `CallToolResult` with `IsError: true` and domain-specific hints instead of a Go error. Logged at INFO level. Applied to 27 get handlers across 21 domains. Defined in `internal/toolutil/not_found.go`.

Use `IsHTTPStatus(err, code)` and `ContainsAny(err, substrs...)` for status-specific branching before calling `WrapErrWithHint`. For get handlers, check `IsHTTPStatus(err, 404)` **before** `LogToolCallAll` and return `NotFoundResult` with `nil` error to log at INFO instead of ERROR. See [ADR-0007](docs/development/adr/adr-0007-rich-error-semantics.md) and [Error Handling](docs/concepts/error-handling.md).

### Transport end-to-end modules

Two modules start the **real binary** and drive it the way a client does, both
without GitLab or credentials, and both run on every CI push:

- `test/e2e/http` (`httpe2e`) — the HTTP handler chain: cross-origin decisions,
  preflight, authentication modes, rate limiting, the JSON-RPC shape of a
  rejection, and the flags that restrict them.
- `test/e2e/stdio` (`stdioe2e`) — the stdio transport: pipes, process lifetime,
  stdout carrying nothing but JSON-RPC, logs on stderr, and the environment
  variables stdio configuration actually uses.

**They exist because the e2e suite cannot see any of this.** `test/e2e/suite`
drives an in-memory transport in the same process, which is the right shape for
questions about tool behavior and answers none about the transport: no streams,
no process, no separation of stdout from stderr, and no flags or environment
variables, since it builds the server directly.

stdio is the primary transport and nothing drove it until `test/e2e/stdio`
existed. Two defects shipped through that gap: a nil dereference that killed the
process on an ordinary eliciting tool call, and a keepalive ping that closed the
session of any client speaking 2026-07-28 after 45 idle seconds — the second
held in place by a unit test asserting the ping ought to be there. Both were
found by hand against a binary, which is what these modules automate.

When adding a transport-level behavior, put its test here rather than in a unit
test that reassembles the handler chain: a test that builds its own copy of the
thing under test is testing the copy.

### Test infrastructure

All tests use `httptest` to mock GitLab API responses. Shared helpers in `internal/testutil/`:

- `testutil.NewTestClient()` — creates a mock GitLab client pointing to httptest server
- `testutil.RespondJSON()` — responds with JSON body
- `testutil.RespondJSONWithPagination()` — responds with pagination headers
- **Never `t.Fatal`/`FailNow` off the test goroutine** (httptest handlers, `go` statements, MCP handlers): follow the six-rule contract in `.github/instructions/test-goroutines.instructions.md` — `t.Errorf` + deterministic response + `return`, or record with atomics and assert afterwards. `make check-test-goroutines` detects violations; `make audit-test-goroutines` writes the work list
- **Every case table runs under `t.Run`**: a range over a slice or map literal that asserts must open one subtest per case, named by a `name` field, the string element, or the map key. `go run ./cmd/audit_test_subtests/ -fix` rewrites the unambiguous shapes; `// sequential: <reason>` on the line above a loop declares dependent steps rather than cases; `make check-test-subtests` gates it in CI
- Test naming: `TestToolName_Scenario_ExpectedResult`
- Test-file naming: a `_test.go` exists only under the name of a module it tests (`register.go` → `register_test.go`); `export_test.go`, build-constrained qualifiers (`fileutils_unix_test.go`), external-package qualifiers (`kind_integration_test.go`), and `test/e2e/` are the codified exemptions. `make check-test-file-names` gates it in CI

### Build & test commands

```bash
go build ./...                           # Build all
go build -o dist/gitlab-mcp-server ./cmd/server  # Build binary
go test ./internal/... -count=1          # Run all unit tests
go test ./internal/tools/branches/ -count=1 -v  # Run domain tests verbose
go test ./internal/tools/ -run TestBranch -count=1  # Run specific tests
make golangci-lint                       # Consolidated Go formatting and linting

# End-to-end tests (requires .env with GITLAB_URL, GITLAB_TOKEN)
go test -v -tags e2e -timeout 300s ./test/e2e/suite/   # Run all e2e tests
make test-e2e                                          # Same via Makefile
make test-e2e-http                                     # HTTP transport module: no GitLab, no credentials
make test-e2e-stdio                                    # stdio transport module: no GitLab, no credentials
make test-e2e-docker                                   # Ephemeral GitLab CE + runner + fixture service (Docker, ~4 GB RAM)
go test -tags e2e -c -o NUL ./test/e2e/suite/           # Compile-only check (Windows)
go test -tags e2e -c -o /dev/null ./test/e2e/suite/     # Compile-only check (Linux)

# Orbit live tests against GitLab.com (requires GITLAB_COM_TOKEN; auto-provisions fixtures)
GITLAB_COM_TOKEN=glpat-... go test -tags orbitlive -count=1 -v ./test/e2e/orbit/
make test-e2e-gitlab-com                                # Orchestrated: ensure token, setup fixtures, wait indexer, run live tests

# Surface evaluator (Docker GitLab fixture)
# CE case set
make eval-surfaces-docker SURFACE=dynamic
make eval-surfaces-docker SURFACE=meta

# Enterprise-only case set on GitLab EE runtime
make eval-surfaces-docker-enterprise SURFACE=dynamic
make eval-surfaces-docker-enterprise SURFACE=meta

# CE + Enterprise case set together on GitLab EE runtime
make eval-surfaces-docker-enterprise-all SURFACE=dynamic
make eval-surfaces-docker-enterprise-all SURFACE=meta
```

For targeted debugging, append `PRESET=...` to any evaluator target to run a single preset.

### Release process

**Major-version policy**: the server's major tracks client-go's major — when client-go moves to v3, this project moves to v3 in that same dependency bump (removing the deprecated compatibility fields kept during v2, e.g. the flat copies in the dual-shape group Datadog output). All other dependency updates ship as minor/patch.

When creating a new release and uploading binaries to GitHub Releases:

1. Build cross-platform binaries with `make release` (uses GoReleaser locally, flattens `dist/` to match GitHub Release asset names). In CI, GoReleaser creates the GitHub release **as a draft** and a dedicated staging step copies each binary from its per-target subdirectory to `dist/<release-asset-name>` (driven by `dist/artifacts.json`) — the npm steps read those paths. The draft is published (`gh release edit --draft=false`) only after the `.mcpb` is attached and every artifact is attested with `actions/attest-build-provenance`, so `go-selfupdate` never observes a partial release. Each release also ships one SPDX SBOM per binary (`<asset>.sbom.json`, listed in `checksums.txt` before cosign signs it as `checksums.txt.sigstore.json`).
2. **Release link names MUST be exact filenames** (e.g. `checksums.txt.sigstore.json`, `gitlab-mcp-server-linux-amd64`). Never add descriptive suffixes like `(GPG signature)` — `go-selfupdate` matches asset names exactly and will fail to find files with decorated names
3. The tag-triggered release workflow also publishes derived artifacts automatically:
   - `gitlab-mcp-server-darwin-all` — macOS universal (fat) binary from GoReleaser `universal_binaries`
   - `gitlab-mcp-server.mcpb` — Claude Desktop extension built by `scripts/build-mcpb.sh` from `mcpb/manifest.json` (validate with `make check-mcpb`, build locally with `make mcpb`)
   - Homebrew formula update pushed to `jmrplens/homebrew-tap` by `scripts/update-homebrew-tap.sh` (secret `TAP_DEPLOY_KEY_B64`)
   - winget version PR to `microsoft/winget-pkgs` via `winget-releaser` (secret `WINGET_TOKEN`)
   - Version stamping of `server.json`, `plugin.json` (Agent Plugins), `.plugin/plugin.json` (legacy Open Plugins), `mcpb/manifest.json`, and `lhm.plugin.json` (LobeHub Marketplace) committed back to main by `scripts/update-server-json-sha.sh` — which also pins the OCI image tag and re-derives the `.mcpb` hash from the bundle passed as its third argument
4. **LobeHub Marketplace is a manual publish step.** The stamping script keeps `lhm.plugin.json`'s version current on every release, but the LobeHub CLI (`@lobehub/market-cli`) has no non-interactive publish path (browser OIDC login + GitHub connect are required once), so it cannot run in CI. After a release, run `make publish-lobehub` from a machine with `lhm login` + `lhm github connect` already completed (the target runs `lhm plugin update`; the CLI's `plugin publish <gitUrl>` verb is for first-time listings only). The listing is `jmrplens-gitlab-mcp-server`.
5. **The manifest's capability arrays are generated, its version is not.** LobeHub derives the listing's capability badges from the `tools`, `prompts`, and `resources` arrays in `lhm.plugin.json` — its scanner cannot introspect a server shipped as a binary or an image, so a manifest without them advertises zero tools. `make gen-lhm-manifest` rewrites those three arrays from a live `tools/list` + `prompts/list` + `resources/list` round-trip and leaves every other field, `version` included, to the release stamp. `make check-lhm-manifest` gates freshness in CI and runs before `make publish-lobehub`.

   The generator declares the **default** surface: it pins `dynamic` and talks to an in-process stub client instead of reading `GITLAB_MCP_TOOL_SURFACE`, `GITLAB_URL` or `GITLAB_TOKEN`. Anything a generator reads from the environment ships in the committed file, so a developer machine with `GITLAB_MCP_TOOL_SURFACE=individual` exported would otherwise publish a different manifest than CI checks. New environment-sensitive inputs belong in `cmd/internal/mcpsurface`, pinned there once for every generator, not read at the call site.

6. **npm publishes automatically via an OIDC trusted publisher — no stored token.** The distribution is a launcher package (`@jmrp.io/gitlab-mcp-server`) plus six per-platform packages that each carry a release binary — the esbuild/biome model, so `npx` works, `--ignore-scripts` works, and nothing is downloaded at install time. The release job's `Publish to npm` step runs right after GoReleaser, reading the same `dist/` binaries the mcpb step uses. It authenticates through [npm trusted publishing](https://docs.npmjs.com/trusted-publishers): the job's `id-token: write` lets npm mint an OIDC token, and npm ≥ 11.5.1 (the step upgrades the setup-node 22 default) performs the exchange, so no secret is stored anywhere and provenance is attached automatically (no `--provenance` flag). `scripts/build-npm.mjs` assembles the packages from `dist/` (Node's `win32`/`x64` names mapped to Go's `windows`/`amd64` in one table there); `scripts/publish-npm.sh` publishes the platform packages first, then the launcher — that order matters, or an install racing the publish resolves optional dependencies not on the registry yet — and it **skips any version already published** so re-running a release job does not die on npm's 409. **`scripts/validate-npm.mjs` runs before the publish step** (a published npm version is permanent): it packs all seven tarballs and checks their file set, the binary's executable bit and platform magic number, a size floor, and the os/cpu/name/version, then installs the launcher plus the host-native platform package and drives a real MCP handshake asserting stdout is pure JSON-RPC. `make validate-npm NPM_BINARIES=dist` runs it in a clean `node:22` container so a developer's machine is never touched; the release job runs `make validate-npm-local` on the ephemeral runner. **Bootstrap constraint:** a package cannot have a trusted publisher configured until it exists, so the first version (v2.7.5) was created by a one-time tokened `make publish-npm`; the trusted publisher on npmjs.com is then configured **per package** (all seven) naming this repo, the workflow file `release.yml`, and the `release` environment (the job sets `environment: release`), after which the token is removed and every later tag publishes with no credential. The launcher's version and its optionalDependency pins move together via `build-npm.mjs --sync-only`, wired into `scripts/update-server-json-sha.sh` so the committed `npm/gitlab-mcp-server/package.json` stays current with the release stamp. For an out-of-band publish, `make publish-npm NPM_BINARIES=dist` from a machine with an npm token (rehearse with `make publish-npm-dry`). The unscoped name `gitlab-mcp-server` was already taken on npm, hence the scope.

7. **PyPI publishes right after npm, from the same staged `dist/` binaries.** The distribution is six platform wheels (`scripts/build_pypi.py`, standard library only) with the native binary in the wheel's `.data/scripts` directory — the uv/ruff model, so the installer places it on the scripts path as the `gitlab-mcp-server` command itself and no Python runs on the hot path (`uvx jmrplens-gitlab-mcp-server` works via a console-script wrapper named after the distribution, and the installed command stays `gitlab-mcp-server`; a `gitlab_mcp_server` module remains for `python -m` and programmatic lookup). Two traps the validator (`scripts/validate_pypi.py`, run before every upload because a published PyPI file name is burned forever) pins: the binary's zip entry needs `S_IFREG` in `external_attr` (pip's `zip_item_is_executable` requires `S_ISREG` before honoring the 0o111 bits — permission bits alone install it non-executable), and the manylinux_2_17 tag is verified against the binary's actual glibc symbol demands (none: Go internal linking). The wheel METADATA embeds `pypi/README.md`, which must keep the `mcp-name: io.github.jmrplens/gitlab-mcp-server` token the MCP Registry validates for pypi ownership. The distribution name is `jmrplens-gitlab-mcp-server`: the unprefixed PyPI name is an empty registration held by an unrelated account, under a PEP 541 reclamation request; when reclaimed, the rename is `DIST_NAME` in `scripts/build_pypi.py` and `scripts/validate_pypi.py`, the `server.json` identifier, and the docs. CI publishes via `pypa/gh-action-pypi-publish` under the OIDC trusted publisher (configured on pypi.org naming this repo, `release.yml`, environment `release`) with `skip-existing`, so re-runs are safe and PEP 740 attestations attach automatically; `make publish-pypi PYPI_BINARIES=dist` is the tokened out-of-band path (`PYPI_TOKEN`), and `make validate-pypi PYPI_BINARIES=dist` runs the whole check in a clean `python:3.14-slim` container.

8. **A `server.json` `remotes` URL must be globally unique across the whole MCP Registry, and the comparison is on the literal string.** The registry refuses a publish whose remote URL any other server already claims — templates included. This repository used to declare `https://{host}:{port}/` as a self-hosted form; the sibling `libgen-mcp` copied that pattern and its v1.5.2 publish was rejected with `remote URL https://{host}:{port}/ is already used by server io.github.jmrplens/gitlab-mcp-server`. Checking that nothing claims your _hostname_ is not the check — the string is. Generic templates are therefore not worth claiming: the self-hosted HTTP mode they advertise is documented in the guides anyway, and holding one blocks every other server you publish. `make check-server-json` validates the schema only; uniqueness is a registry-side property no local gate can see.

9. **`server.json`'s `packages` array says what a client should install, and almost nothing about it is schema-checkable.** Four package entries are declared: the `.mcpb` bundle, the `ghcr.io` image, the npm launcher, and the pypi distribution. Three traps, each of which passed `make check-server-json` while being wrong:
   - **`registryType: "mcpb"` means the bundle, not a binary.** Up to v2.7.5 the manifest declared six `mcpb` packages pointing at the raw per-platform executables (`7f 45 4c 46`) while the real bundle (`50 4b 03 04`) shipped in the same release, undeclared. Every directory that scores a server by installing what it declares scored the raw ELF.
   - **The OCI entry must declare no `packageArguments` at all.** Any argument after the image name replaces `CMD` wholesale, so an entry that names one is opting out of whatever the image's command does. While `CMD` was `--http --http-addr 0.0.0.0:8080`, opting out was exactly what the entry wanted: a client running `docker run -i` with no argument override got an HTTP listener and hung at `initialize` forever, and the entry carried `packageArguments: [{"type": "positional", "value": "--http=false"}]` to undo it. `CMD` is now `--transport auto --http-addr 0.0.0.0:8080`, which reads the transport off file descriptor 0, so the `-i` implied by the entry's stdio transport hint is enough on its own and an argument would only take the `--http-addr` away with it. `scripts/validate-server-json-packages.sh` therefore accepts an argument-free entry only when `CMD` reaches stdio by itself, and still demands an explicit stdio argument from any entry that declares arguments at all. It reads that `CMD` from the image the entry pins, which is a _published_ image and so lags this repository by a release; when the published one predates the inference it falls back to the `Dockerfile` here and says so, the same transitional shape the npm branch uses for `mcpName`. A `Dockerfile` regressed to a bare `--http` fails either way.
   - **An OCI entry must NOT carry `version`, `registryBaseUrl` or `fileSha256`.** The registry rejects them at `mcp-publisher publish`, server-side, long after the schema check passed. Its version lives in the image reference, which `scripts/update-server-json-sha.sh` pins separately from the URL rewrite. Ownership is validated through the image's `io.modelcontextprotocol.server.name` label, which the `Dockerfile` sets.
   - **The image is pinned as `<repo>:<tag>@sha256:<digest>`**, the third form the registry's OCI validator accepts. The tag stays readable and matches every doc; the digest is what a client actually resolves, so re-pushing a published version tag cannot change what gets installed — it was the last mutable pointer in the manifest (the bundle has `fileSha256`, an npm version is immutable). The digest comes from the docker job's `docker-push` output, which is why `release` depends on `docker`; the stamper takes it as a fourth argument and **refuses to stamp** a digest-pinned identifier without one, since pinning the new tag beside the previous release's digest would be worse than either alone.

   `make check-server-json-packages` (`scripts/validate-server-json-packages.sh`) gates all of this by downloading each declared artifact: it hashes the bundle, opens it as a zip, checks the image tag resolves and its ownership label matches, and rejects the fields the registry rejects. CI runs it on pushes to main only, since it downloads 40 MB. The bundle's SHA256 cannot come from GoReleaser's `checksums.txt` — the bundle is not a GoReleaser artifact, and appending it there would invalidate `checksums.txt.asc` — so the release workflow builds the `.mcpb` **before** the stamp and passes its path as the stamper's third argument.

   A third package entry declares the npm launcher (`@jmrp.io/gitlab-mcp-server`, `runtimeHint: "npx"`). The registry validates npm ownership through an `mcpName` field in the **published** `package.json`; the field is committed in `npm/gitlab-mcp-server/package.json`, and release.yml publishes npm **before** `mcp-publisher` runs, so the version the registry fetches always carries it. The gate's npm branch checks the identifier/mcpName/version lockstep against the committed launcher and warns (rather than fails) while the published version still predates `mcpName`. Like the OCI entry, the npm entry must not carry `fileSha256`; unlike it, a `version` is required and the stamp's step 2 bumps it with every release.

### Post-implementation verification

After making changes, run targeted verification on the **changed files/packages only** (not the entire project):

```bash
# Go files — run on affected packages
go test ./internal/tools/branches/ -count=1    # tests on changed package
golangci-lint run --build-tags e2e ./internal/tools/branches/ # lint changed package

# Markdown files — run on specific changed files
npx markdownlint-cli2 docs/guides/ci-cd.md README.md  # lint specific .md files
npx markdownlint-cli2 --fix docs/guides/ci-cd.md      # auto-fix specific .md files

# README.md/docs tables — normalize pipe tables, or verify with --check
go run ./cmd/format_md_tables/
go run ./cmd/format_md_tables/ --check

# MCP Inspector (interactive tool testing UI at http://127.0.0.1:6274)
make inspector                             # compile + launch Inspector via stdio
make inspector-stop                        # stop Inspector and clean up

# Full project analysis (use sparingly — for pre-commit or CI)
make analyze                               # all analysis gates, full project
make analyze-fix                           # auto-fix what can be fixed
make analyze-report                        # generate LLM-consumable report
```

**Static analysis tools** (3 consolidated gates): `golangci-lint` (v2, 25+ linters plus `goimports`, `gofumpt`, and `gci` formatters), `govulncheck`, and `markdownlint-cli2`. Configuration: `.golangci.yml`, `.markdownlint-cli2.jsonc`. Full docs: `docs/development/static-analysis.md`.

**Markdown table formatter**: When creating or editing pipe tables in `README.md` or `docs/`, run `go run ./cmd/format_md_tables/` to normalize source-readable padding and left/right/center alignment markers, then verify with `go run ./cmd/format_md_tables/ --check` before markdownlint.

**Formatting tools**: Before committing, always run `make analyze-fix` to apply configured Go formatters: `goimports` (import cleanup), `gofumpt` (stricter gofmt-compatible formatting), and `gci` (deterministic import section grouping).

### Environment variables

Settings this project defines are read as `GITLAB_MCP_<NAME>`; `config.Getenv`
and `config.TrimmedGetenv` in `internal/config/envname.go` resolve both
spellings, prefer the prefixed one, and record the fallback so `cmd/server`
can warn once at startup. A new setting is added to `prefixedNames` there and
read through those helpers, never through `os.Getenv`.

Names deliberately left bare, because reading them through the helper is safe
but prefixing them is not: `GITLAB_URL` and `GITLAB_TOKEN` (GitLab's own
convention, and what an assistant writes from memory into a client config),
the `GITLAB_`-prefixed switches such as `GITLAB_TIER` and `GITLAB_READ_ONLY`
(already namespaced, so a second prefix churns every configuration for
nothing), every `OTEL_*` variable (owned by the OpenTelemetry specification and
read by the exporters themselves), `YOLO_MODE` and `AUTOPILOT` (conventions
other agent tooling sets), and the evaluator's `EVAL_SURFACE_*` (developer-only,
driven by `make` targets, never beside another tool's variables in a shell).

| Variable                 | Required | Description                                              |
| ------------------------ | -------- | -------------------------------------------------------- |
| `GITLAB_URL`             | Stdio    | GitLab instance URL (e.g., `https://gitlab.example.com`). In HTTP mode it is `--gitlab-url`, which is **required** unless `--allow-any-gitlab-url` is passed, and the count decides the header's meaning: none (with the escape hatch) lets `GITLAB-URL` select freely per request, and a request that omits it is refused rather than resolved to `https://gitlab.com`; exactly one fixes the instance and the header is ignored; several publish an allow-list among which the header is required, since choosing for the caller would send their token to an instance they never named. A comma-separated value spells the list |
| `GITLAB_TOKEN`           | Stdio    | Personal Access Token (`glpat-...`)                      |
| `GITLAB_SKIP_TLS_VERIFY` | No       | Skip TLS verification for self-signed certs (`true`)     |
| `GITLAB_MCP_ENV_FILE`    | No       | One dotenv file to load besides `~/.gitlab-mcp-server.env`, resolved **once** from the process environment before any loaded file could rewrite it, so a file this server loads cannot nominate another. Precedence, highest first: the process environment, this file, the home file. A working-directory `.env` is not loaded at all, only found and named at WARN with the keys it wanted to set. Give an absolute path: a relative one follows the client into every workspace it opens, which is the load this replaced, and startup says so. `internal/config.EnvFileVar` |
| `GITLAB_MCP_STDIO_MAX_LINE_BYTES` | No | Longest stdio message assembled, in bytes (default 4 MiB, matching the SDK's own HTTP body default so both transports refuse the same messages). A longer line is refused and answered rather than accumulated. A value that is missing, unparseable or non-positive warns and keeps the default, because a mistyped number should not take the client down with the server. `cmd/server.stdioMaxLineBytesEnv` |
| `GITLAB_MCP_MAX_LISTEN_STREAMS` | No | Concurrent `subscriptions/listen` streams one credential may hold open (default 64; `0` removes the per-credential ceiling). A second ceiling of 512 per process is deliberately not configurable: the per-credential one multiplies by however many tokens a caller holds, so only the process-wide one bounds the process. `MaxWatchers` does not cover this, since a listen asking only for list-changed notifications creates no watcher. Both transports. `cmd/server.maxListenStreamsEnv` |
| `GITLAB_MCP_META_TOOLS`             | No       | Deprecated compatibility selector; prefer `GITLAB_MCP_TOOL_SURFACE` for new configs |
| `GITLAB_MCP_TOOL_SURFACE`           | No       | Explicit tool catalog selector: `dynamic`, `meta`, or `individual`; default is `dynamic` when unset, unless legacy `GITLAB_MCP_META_TOOLS` is explicitly set |
| `GITLAB_MCP_CAPABILITY_SURFACE`     | No       | Resource and prompt catalog selector: `full` or `minimal`; `minimal` keeps the surface-aware `gitlab://tools` manifest |
| `GITLAB_MCP_META_PARAM_SCHEMA`      | No       | Meta-tool input-schema strategy: `opaque` (default), `compact` (~6.5x), or `full` (~11.9x). Independent of `GITLAB_MCP_META_TOOLS`. Per-action call shapes and input schemas are discoverable through `gitlab://tools` and `gitlab://tools/{id}` for every surface |
| `GITLAB_READ_ONLY`       | No       | Read-only mode: removes mutating operations per action; reads keep working on every surface (`false` default) |
| `GITLAB_SAFE_MODE`       | No       | Safe mode: intercepts mutating operations per action and returns a JSON preview naming the action; reads keep working (`false` default) |
| `GITLAB_TIER`            | No       | Licensing tier selector: `free`/`ce` (Free), `premium`, or `ultimate`. When set, the tier is used verbatim with no license check. When unset, the tier is detected from the instance license (`GET /license` → plan), falling back to `free`. In HTTP mode use `--tier`; when omitted the tier is detected per token+URL pool entry. Enterprise/Premium tools are gated when the resolved tier is Premium or Ultimate |
| `GITLAB_MCP_ALLOWED_UPLOAD_DIRS` | No | Extra directories a tool may READ a local file from (every `file_path` and `directory_path` input), separated by the OS path-list separator. The working directory and the OS temp directory are always allowed; a path is canonicalized through symlinks before the check. `internal/toolutil.UploadDirAllowlistEnv` |
| `GITLAB_MCP_ALLOWED_DOWNLOAD_DIRS` | No | Extra directories a tool may WRITE a downloaded file into (`output_path`), same syntax and same always-allowed roots. Checked twice, once before creating the parent directories and once after, so the second call resolves a parent that now exists. `internal/toolutil.DownloadDirAllowlistEnv` |
| `GITLAB_MCP_ALLOWED_IMPORT_DIRS` | No | Extra directories a project or group import archive may be read from, same syntax and same always-allowed roots. `internal/toolutil.ImportArchiveAllowlistEnv` |
| `GITLAB_ENTERPRISE`      | No       | **Deprecated** — use `GITLAB_TIER`. Honored for back-compat only when `GITLAB_TIER` is unset: `true` → `ultimate`, `false` → `free`. Logs a deprecation warning |
| `GITLAB_MCP_AUTH_MODE`              | No       | HTTP mode auth: `legacy` (default) or `oauth` (RFC 9728 Bearer verification) |
| `GITLAB_MCP_PUBLIC_URL`             | No       | Externally reachable https origin; required with `GITLAB_MCP_AUTH_MODE=oauth` (RFC 9728 resource identifier; flag `--public-url`) |
| `GITLAB_MCP_TRUSTED_ORIGINS`        | No       | Comma-separated absolute origins allowed to make cross-origin browser requests; `*` accepts any origin and disables the protection; empty adds none, though a configured `GITLAB_MCP_PUBLIC_URL` origin is trusted regardless (flag `--trusted-origins`) |
| `GITLAB_MCP_OAUTH_CACHE_TTL`        | No       | OAuth token identity cache TTL (`15m` default, range 1m–2h) |
| `GITLAB_MCP_OAUTH_CLIENT_UID`       | No       | Comma-separated GitLab OAuth application uids whose tokens are admitted (flag `--oauth-client-uid`). Empty (default) admits any credential the instance accepts; setting it refuses personal access tokens, which belong to no application. RFC 8707 audience binding is unavailable — GitLab publishes no `resource_indicators_supported` — so this is the specification's "otherwise verify" alternative (ADR-0019) |
| `GITLAB_MCP_POOL_IDLE_TIMEOUT`      | No       | HTTP mode: reclaim a pooled per-token-and-URL server entry after this long unused (`1h` default, `0` disables, max 24h) |
| `GITLAB_MCP_RATE_LIMIT_RPS`         | No       | Per-server tools/call rate limit in req/s (`0` = disabled) |
| `GITLAB_MCP_RATE_LIMIT_BURST`       | No       | Token-bucket burst size when RPS > 0 (`40` default)       |
| `GITLAB_MCP_CLIENT_COMPAT`          | No       | Per-client response compatibility (`auto` default): Codex sessions get float `priority` in annotations rounded to 0/1; `off` disables. Read from the process environment in both stdio and HTTP modes (no flag equivalent). See `internal/clientcompat` and `docs/guides/client-compatibility.md` |
| `GITLAB_MCP_DESCRIPTION_SUBSTITUTIONS` | No | Rewrite listed catalog text for strict MCP gateway validators: comma-separated `old=new` pairs applied in order to every listed description and title (backslash escapes `\,` `\=` `\\`). Covers tools (all surfaces, schema-embedded descriptions included), prompts, resources, and resource templates; never names, URIs, `pattern`, `const`, enum values, or tool-call payloads. Malformed values refuse startup. Flag `--description-substitutions`; both modes. The served surface itself stays pure ASCII with no semicolons (`make check-gateway-chars`); verify a config with `go run ./cmd/audit_gateway_chars/ -apply -check`. See `internal/gatewaycompat` and `docs/guides/client-compatibility.md` |
| `GITLAB_MCP_TELEMETRY`   | No       | Export OpenTelemetry traces, metrics and logs over OTLP (`false` default). Off for privacy: telemetry goes to a collector the operator configures, never to the maintainer; the exporters' only default is their own `localhost`. Endpoint, credentials, sampling and batching all come from the standard `OTEL_EXPORTER_OTLP_*` variables the exporters read themselves. `OTEL_SDK_DISABLED=true` vetoes it regardless. Flag `--telemetry`. See `docs/guides/telemetry.md` |
| `GITLAB_MCP_TELEMETRY_IDENTITY` | No | How much telemetry records about who made a call: `none` (default, records nobody), `pseudonymous` (a per-process HMAC digest that correlates one caller's calls without naming them), or `full` (`user.id` and `user.name`). Identity never reaches a metric under any policy. Flag `--telemetry-identity` |
| `GITLAB_MCP_TELEMETRY_IDENTITY_KEY` | No | Secret the `pseudonymous` policy derives its pseudonymisation keys from (HKDF-SHA256, one key for callers and one for resource URIs). Empty (default) generates one per process, so a digest identifies a caller within one process and nowhere else. Set it when several replicas must agree, or when a distinct-user count has to survive a restart; it then never rotates here, because a key the operator supplied is theirs to rotate. Setting it makes the pseudonym a person pseudonym in EDPB terms, and the key is Article 4(5) "additional information": keep it away from wherever the telemetry lands, since GitLab user ids are small enough to enumerate against a known key. No flag on purpose: process arguments are readable through `/proc` by any local principal |
| `GITLAB_MCP_TELEMETRY_IDENTITY_ROTATION` | No | How long a **generated** pseudonymisation key lives, e.g. `24h`. Empty or `0` (default) keeps it for the life of the process; maximum 30 days. Ignored when `GITLAB_MCP_TELEMETRY_IDENTITY_KEY` is set, with a warning at startup. Off by default because replicas start at different moments and would rotate out of phase, churning a distinct-user count nobody asked to churn. Flag `--telemetry-identity-rotation` |
| `GITLAB_MCP_TELEMETRY_TOOL_NAME` | No | Whether `gen_ai.tool.name` is a metric dimension: `auto` (default), `on` or `off`. Auto keeps it on the dynamic and meta surfaces and drops it on individual, where ~1078 tools would exhaust the SDK's 2000-series cardinality limit and collapse the long tail into one `otel.metric.overflow` bucket, first-come-wins under cumulative temporality. Implemented as a metric View, because filtering runs before the limit is counted. Flag `--telemetry-tool-name` |
| `GITLAB_MCP_LOG_LEVEL`              | No       | Logging verbosity (`debug`, `info`, `warn`, `error`)     |
| `EVAL_SURFACE_ENTERPRISE` | No      | `cmd/eval_mcp_surfaces`: run the enterprise case set on top of the base corpus. Used by `make eval-surfaces-docker-enterprise*` targets |
| `EVAL_SURFACE_CASE_SET`   | No      | `cmd/eval_mcp_surfaces`: case-set selector — `ce` (Community Edition only), `all` (CE+Enterprise). Used by `make eval-surfaces-docker-enterprise-all` |
| `EVAL_SURFACE_SERVER_MODE` | No     | `cmd/eval_mcp_surfaces`: protective server mode under evaluation — `default`, `read-only`, or `safe-mode`. Alias `SERVER_MODE=` on the Makefile target |
| `EVAL_SURFACE_FIXTURE_SMOKE` | No   | `cmd/eval_mcp_surfaces`: limit the run to fixture-smoke cases (fast smoke check) |
| `--max-output-retries`  | No       | `cmd/eval_mcp_surfaces`: re-runs a task when it fails solely due to malformed model tool-call output (`2` default, `0` disables) |

None of the three `GITLAB_MCP_ALLOWED_*_DIRS` allow-lists applies in HTTP mode: a server reached over HTTP refuses every caller-supplied local path, since the caller has no files on the machine the server runs on and `content_base64` is the remote form. The transport is inferred from the process arguments in `internal/toolutil/fileutils.go`, so a deployment that never heard of this policy still gets the right answer.

In **HTTP mode**, configuration comes from CLI flags instead of environment variables:

| Flag                  | Default | Description                                              |
| --------------------- | ------- | -------------------------------------------------------- |
| `--gitlab-url`        | —       | GitLab instance URL. **Required in HTTP mode** unless `--allow-any-gitlab-url` is passed: a deployment that names no instance makes requests to whatever host a caller puts in `GITLAB-URL`, with whatever token that caller supplied. Repeatable/comma-separated: all are published in RFC 9728 `authorization_servers`, and `GITLAB-URL` then selects among them. With several published the header is **required** in both auth modes (400 without it), and a value outside the list is refused with 403 rather than ignored. The refusal names the published instances only in oauth mode, where RFC 9728 metadata already serves that same set unauthenticated; legacy mode publishes no metadata and rejects before the credential is judged, so it says to ask the operator rather than let any non-empty token enumerate the operator's hostnames |
| `--allow-any-gitlab-url` | `false` | Start with no instance published, letting `GITLAB-URL` name any host. For a single-user local deployment where the operator is the caller; it warns at startup and must not be used on a listener anyone else can reach |
| `--skip-tls-verify`   | `false` | Skip TLS verification for self-signed certs              |
| `--meta-tools`        | `false` _(deprecated)_ | Legacy boolean tool selector, kept for compatibility and ignored when `--tool-surface` is set. Leave it unset for the default dynamic surface; use `--tool-surface=individual` when migrating an old `--meta-tools=false` config |
| `--tool-surface`      | _(empty)_ | Explicit tool catalog selector: `meta`, `individual`, or `dynamic`; overrides `--meta-tools` when set |
| `--capability-surface` | `full` | Resource and prompt catalog selector: `full` or `minimal` |
| `--tier`              | _(empty)_ | Force licensing tier: `free`, `ce`, `premium`, or `ultimate`. When set, used verbatim with no license check; when omitted, the tier is detected from the instance license per token+URL pool entry (fallback `free`) |
| `--read-only`         | `false` | Read-only mode: removes mutating operations per action; reads keep working |
| `--safe-mode`         | `false` | Safe mode: intercepts mutating operations per action, returns preview |
| `--max-http-clients`  | `100`   | Maximum unique (token, GitLab URL) server entries kept in the pool; bounds pooled entries, not sessions or concurrent requests |
| `--session-timeout`   | `30m`   | Idle MCP session timeout; applies to `--stateless=false` only — under the default stateless transport each POST's session ends with its response |
| `--http-idle-timeout` | `0` (disabled) | HTTP server idle connection timeout; `0` (default) disables idle closure so `--session-timeout` is the effective lifetime; set a positive duration to recycle idle connections sooner |
| `--stateless`         | `true`  | Sessionless streamable HTTP (SEP-2567 / protocol 2026-07-28; default): no `Mcp-Session-Id` tracking, every POST is self-contained, GET/DELETE return `405`; synchronous server-initiated requests are unavailable, but protocol 2026-07-28 clients keep elicitation through MRTR. Use `--stateless=false` for legacy stateful sessions |
| `--json-response`     | `false` | Return `application/json` response bodies instead of `text/event-stream` (SSE) |
| `--max-request-body-bytes` | `0` | Maximum streamable HTTP request body size in bytes; `0` uses the SDK default (4 MiB) |
| `--http-addr`         | `:8080` | Listen address. `host:port` binds TCP; a value containing a path separator binds a unix socket instead. Host-local: a same-host proxy connects through it and the TCP hop between them disappears rather than being encrypted; remote clients still arrive via that proxy |
| `--http-socket-mode`  | `0660`  | Octal permission mode for a unix socket named by `--http-addr` |
| `--tls-cert` / `--tls-key` | — | PEM certificate and key; serves HTTPS on the listener itself, for a proxy that does not share the machine. Both or neither, loaded at startup, TLS 1.2 floor |
| `--auth-mode`         | `legacy` | Authentication mode: `legacy` or `oauth` (RFC 9728 Bearer verification) |
| `--public-url`        | _(empty)_ | Externally reachable https origin; required with `--auth-mode=oauth` (RFC 9728 resource identifier and metadata-URL derivation) |
| `--resource-documentation` | _(empty)_ | https URL published as RFC 9728 `resource_documentation`; point it at a page describing your own OAuth application. Empty publishes this project's OAuth setup guide |
| `--resource-policy-uri` | _(empty)_ | https URL published as RFC 9728 `resource_policy_uri`; your own page on what this deployment does with the data reached through it. Empty omits the field, which is the right default: a link to a page that does not exist would land on a consent screen |
| `--resource-tos-uri` | _(empty)_ | https URL published as RFC 9728 `resource_tos_uri`; your own terms of service. Empty omits the field |
| `--oauth-cache-ttl`   | `15m`   | OAuth token identity cache TTL (range 1m–2h)             |
| `--oauth-client-uid`  | _(empty)_ | Comma-separated GitLab OAuth application uids whose tokens are admitted; empty admits any credential the instance accepts |
| `--pool-idle-timeout` | `1h` | Reclaim a pooled per-token-and-URL server entry after this long unused; `0` keeps entries until the pool size bound evicts them (upper bound: 24h) |
| `--revalidate-interval` | `15m` | Token re-validation interval; `0` stops the periodic check, but an entry whose credential is older than 1h is still rebuilt, which re-runs the probe. Upper bound 24h |
| `--trusted-proxy-header` | _(empty)_ | HTTP header with real client IP for rate limiting behind proxies (e.g. `CF-Connecting-IP`, `X-Forwarded-For`) |
| `--rate-limit-rps` | `10` | Per-server tools/call rate limit in req/s (`0` disables it). On by default in HTTP mode; the `GITLAB_MCP_RATE_LIMIT_RPS` env var used by stdio still defaults to `0` |
| `--rate-limit-burst` | `40` | Token-bucket burst size when --rate-limit-rps > 0        |
| `--telemetry`       | `false` | Export OpenTelemetry traces, metrics and logs over OTLP. Applies to both transports. Endpoint and credentials come from the standard `OTEL_EXPORTER_OTLP_*` environment |
| `--telemetry-identity` | `none` | What telemetry records about the caller: `none`, `pseudonymous` or `full` |
| `--telemetry-tool-name` | `auto` | Whether `gen_ai.tool.name` is a metric dimension: `auto`, `on` or `off` |
| `--telemetry-identity-rotation` | _(empty)_ | How long a generated pseudonymisation key lives, e.g. `24h`; ignored when a key is configured |

**General flags** (both stdio and HTTP modes):

| Flag           | Default | Description                                                    |
| -------------- | ------- | -------------------------------------------------------------- |
| `--shutdown`   | `false` | Terminate all running instances of this binary and exit. Used by external updaters (pe-agnostic-store) before replacing the binary on disk. |

---

## AI Assistance Infrastructure

This project includes a comprehensive set of AI agents, skills, and instruction files in `.github/` to support development workflows. All are oriented toward **development tasks**, not end-user usage.

### Instructions (Auto-loaded by File Pattern)

Instruction files in `.github/instructions/` are automatically applied when editing matching files:

| Instruction                                        | Applies to | Purpose                                                                   |
| -------------------------------------------------- | ---------- | ------------------------------------------------------------------------- |
| `go.instructions.md`                               | `**/*.go`  | Idiomatic Go practices, naming, error handling, package rules             |
| `go-mcp-server.instructions.md`                    | `**/*.go`  | MCP server patterns: tool registration, typed I/O, annotations, transport |
| `mcp-best-practices.instructions.md`               | `**/*.go`  | Protocol-level tool design, response formats, pagination, security        |
| `security-and-owasp.instructions.md`               | `*`        | OWASP Top 10, input validation, secrets management, injection prevention  |
| `code-review-generic.instructions.md`              | `**`       | Code review priorities (Critical/Important/Suggestion), checklist         |
| `context-engineering.instructions.md`              | `**`       | Project structure principles for AI-readable code                         |
| `self-explanatory-code-commenting.instructions.md` | `**`       | Comment only WHY, not WHAT; avoid redundant comments                      |

### Agents (7 Specialized AI Agents)

Agents are invoked explicitly for specific development tasks. Each agent has a focused role:

#### Core Development

| Agent                    | File                     | When to Use                                                                                                              |
| ------------------------ | ------------------------ | ------------------------------------------------------------------------------------------------------------------------ |
| **Go MCP Server Expert** | `go-mcp-expert.agent.md` | Implementing new MCP tools, fixing tool handlers, MCP SDK questions. The primary coding agent for this project. Has Context7 integration for up-to-date library docs. |
| **Debug Mode**           | `debug.agent.md`         | Systematic bug investigation: reproduce → hypothesize → fix → verify. 4-phase workflow.                                  |

#### Testing

| Agent           | File                    | When to Use                                                                                                                                                                                              |
| --------------- | ----------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Test Expert** | `test-expert.agent.md`  | Writing, analyzing, and improving Go tests. Covers new test development, existing test analysis, coverage analysis to 90%+, false-pass detection, edge case identification, mandatory test documentation, and refreshing `docs/development/testing/testing.md` with `cmd/gen_testing_docs` at phase completion. Uses Context7 for up-to-date Go testing docs. |

#### Planning & Architecture

| Agent                   | File                                       | When to Use                                                                                                       |
| ----------------------- | ------------------------------------------ | ----------------------------------------------------------------------------------------------------------------- |
| **Plan Expert**         | `plan-expert.agent.md`                     | Strategic planning for features, refactoring, architecture, tests, bugs, docs, and upgrades. 7 planning modes with structured output to `plan/`. Uses Context7 for dependency research. Does NOT generate code. |

#### Documentation

| Agent                    | File                            | When to Use                                                                                                    |
| ------------------------ | ------------------------------- | -------------------------------------------------------------------------------------------------------------- |
| **Documentation Writer** | `documentation-writer.agent.md` | Generate project documentation (architecture, references, guides). Uses Diátaxis framework + Mermaid diagrams. Uses Context7 and web fetch for up-to-date external references, specs, and protocol docs. Validates output with markdownlint-cli2. |
| **Go Source Documenter** | `go-source-documenter.agent.md` | Add godoc-compliant doc comments to Go source and test files. Covers file headers, package comments, functions, types, interfaces, tests (detailed what/how/expected/why), benchmarks, fuzz tests, examples, deprecation notices, and BUG/TODO annotations. Uses Context7 for up-to-date Go doc conventions. |

#### Security & Architecture

| Agent            | File                    | When to Use                                                                                               |
| ---------------- | ----------------------- | --------------------------------------------------------------------------------------------------------- |
| **SE: Reviewer** | `se-reviewer.agent.md`  | Security review (OWASP Top 10, LLM security, Zero Trust) and architecture review (Well-Architected frameworks, ADRs). Two modes in one agent. |

### Skills (18 Reusable Task Templates)

Skills are task templates that can be invoked by any agent or directly. They define structured workflows:

#### Documentation Skills

| Skill                              | Directory                         | Purpose                                                                                                 |
| ---------------------------------- | --------------------------------- | ------------------------------------------------------------------------------------------------------- |
| **Generate Project Documentation** | `generate-project-documentation/` | Full documentation suite (architecture, package docs, tool references, onboarding). Diátaxis framework. |
| **Update Project Documentation**   | `update-project-documentation/`   | Delta-update docs after code changes. Maps changes to affected documents.                               |
| **Update Starlight Docs**          | `update-starlight-docs/`          | Update Astro Starlight user docs (EN/ES) when developer docs change.                                    |
| **Go Source Documentation**        | `go-source-documentation/`        | Add godoc-compliant comments to Go files. 11 documented patterns specific to this project.              |

#### Planning & Design Skills

| Skill                          | Directory                               | Purpose                                                                                                |
| ------------------------------ | --------------------------------------- | ------------------------------------------------------------------------------------------------------ |
| **Create Implementation Plan** | `create-implementation-plan/`           | Structured plan with phased tasks (TASK-001, etc.). Saves to `plan/`.                                  |
| **Create ADR**                 | `create-architectural-decision-record/` | ADR with standardized format (POS-001, NEG-001, etc.). Saves to `docs/development/adr`.                           |
| **Create Specification**       | `create-specification/`                 | Formal spec with requirements (REQ-001), acceptance criteria (Given-When-Then). Saves to `docs/spec/`. |

#### Quality & Testing Skills

| Skill                      | Directory                 | Purpose                                                                                                  |
| -------------------------- | ------------------------- | -------------------------------------------------------------------------------------------------------- |
| **Increase Test Coverage** | `increase-test-coverage/` | Research → Plan → Implement pipeline to reach 90%+ coverage. Uses httptest mocks specific to GitLab API. |
| **Review and Refactor**    | `review-and-refactor/`    | Review code quality + MCP patterns + OWASP, then refactor. Reads all instruction files for context.      |
| **Go Testing Patterns**    | `golang-testing/`         | Reference: table-driven tests, subtests, benchmarks, fuzzing, httptest, TDD methodology.                 |
| **Go Patterns**            | `golang-patterns/`        | Reference: error handling, concurrency, interfaces, structs, memory, anti-patterns.                      |

#### Evaluation & Operations Skills

| Skill                     | Directory                | Purpose                                                                                                           |
| ------------------------- | ------------------------ | ----------------------------------------------------------------------------------------------------------------- |
| **Create MCP Evaluation** | `create-mcp-evaluation/` | Generate 10 Q&A pairs to benchmark MCP server quality. Multi-hop, read-only, verifiable answers.                  |
| **Git Commit**            | `git-commit/`            | Conventional commit with auto-detected type/scope from diff. Follows project's `feat:`/`fix:`/`docs:` convention. |

#### Refactoring Skills

| Skill                       | Directory                  | Purpose                                                                                                           |
| --------------------------- | -------------------------- | ----------------------------------------------------------------------------------------------------------------- |
| **Go Safe Move Refactor**   | `go-safe-move-refactor/`   | Safely move Go source files between packages with zero compilation downtime. Handles imports, stubs, tests.       |
| **Modularize Go Package**   | `modularize-go-package/`   | Modularize a monolithic Go package into domain sub-packages. Designed for large-scale 50–100+ file refactoring.   |

#### MCP Development Skills

| Skill                       | Directory                  | Purpose                                                                                                           |
| --------------------------- | -------------------------- | ----------------------------------------------------------------------------------------------------------------- |
| **Create MCP Tool**         | `create-mcp-tool/`         | End-to-end workflow for creating a new MCP tool: sub-package, structs, handler, ActionSpec metadata, markdown, tests, catalog projection, and documentation. |
| **Upstream Contribution**   | `upstream-contribution/`   | Contribute fixes to upstream gitlab.com/gitlab-org/api/client-go. Fork → branch → fix → test → MR workflow.       |

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

1. Use `update-project-documentation` skill — analyzes code delta, maps to affected docs, applies surgical updates
2. For full regeneration: Use `generate-project-documentation` skill

---

## Architecture Decisions

ADRs document key decisions in `docs/development/adr`:

| ADR      | Decision                                                       | Status                                       |
| -------- | -------------------------------------------------------------- | -------------------------------------------- |
| ADR-0004 | Modular sub-packages under `internal/tools/{domain}/`          | Accepted (176 `internal/tools` packages; tools by tier: ~854 Free/CE, ~1006 Premium, ~1072 Ultimate self-managed, ~1078 GitLab.com Ultimate) |
| ADR-0006 | Raw GraphQL.Do() for domains without client-go service wrappers | Accepted (7 GraphQL-only domains)             |
| ADR-0007 | Rich error semantics for LLM-actionable diagnostics            | Accepted (WrapErrWithMessage, WrapErrWithHint) |
| ADR-0009 | Progressive GraphQL migration strategy                         | Accepted (trigger-based REST→GraphQL migration) |
| ADR-0015 | Polled resource subscriptions (supersedes ADR-0010)            | Accepted (26 subscribable kinds, 10 watchers/token, lease demotes rather than stops) |
| ADR-0016 | No webhook ingestion                                           | Accepted (`Reader` seam unchanged; no inbound HTTP surface added) |
| ADR-0017 | Pull-safe event sources surveyed and declined                  | Accepted (Events API, ActionCable, ETag probed live; polling stays the only freshness source; revisit triggers recorded) |
| ADR-0018 | Authorization admits at the minimum scope; writes gated per action | Accepted (a read_api token is admitted and served a read-only surface; `tools/list` stays authenticated per the MCP authorization spec) |

### Modular tools sub-packages (ADR-0004)

The `internal/tools/` package family is split into 176 packages. Runtime tool surfaces are projected from canonical `ActionSpec` and surface specs. Package-local `RegisterTools` functions have been removed for ordinary GitLab API actions; the catalog-first runtime is the exclusive registration model. This provides:

- Package-level namespace eliminates need for domain prefixes on types (`branches.Output` vs old `BranchOutput`)
- Each sub-package is independently testable with isolated `httptest` mocks
- Zero import cycles — sub-packages import from `toolutil/` only, never from each other
- `internal/tools/register.go` registers individual tools from the canonical action catalog projection
- Validated by catalog and source guardrails such as `TestRegisterAllDoesNotUseDomainRegisterTools` and ActionSpec coverage audits

### Markdown registry pattern

Markdown formatters use a type-based registry in `internal/toolutil/mdregistry.go` instead of a central dispatch switch. Each sub-package self-registers its formatters via `init()` functions:

- `toolutil.RegisterMarkdown[T](fn)` — registers a formatter for output type `T`
- `toolutil.RegisterMarkdownResult[T](fn)` — registers a formatter for `*mcp.CallToolResult` types
- `toolutil.MarkdownForResult(result any)` — looks up and invokes the registered formatter by `reflect.Type`
- `internal/tools/markdown.go` is a thin delegator (~19 lines) that calls `toolutil.MarkdownForResult`
- ~266 formatters across 76 sub-packages, validated by `TestAllMarkdownFormattersRegistered`

### Dynamic toolset mode

`GITLAB_MCP_TOOL_SURFACE=dynamic` registers only `gitlab_find_action` and `gitlab_execute_action`. It is the default when `GITLAB_MCP_TOOL_SURFACE` and legacy `GITLAB_MCP_META_TOOLS` are unset. The dynamic registry is built from the canonical action catalog shared with meta-tools and augmented with standalone routes such as project discovery, so execution reuses existing handlers, typed schemas, destructive-action classification, read-only filtering, safe-mode previews, markdown formatters, and scope filtering.

Developers add normal GitLab actions through domain-local `ActionSpecs` and the audited catalog aggregation path. `internal/tools/action_catalog.go` builds the canonical catalog from those specs; meta-tools register visible domain dispatchers from it, dynamic mode builds find/execute over it, and individual mode projects one visible tool per action from the same catalog. Do not add package-local `RegisterTools` functions, duplicate dynamic-only action definitions, or package-level meta registration for ordinary GitLab API operations. See `docs/development/tool-surfaces-and-action-core.md` for the detailed developer architecture.

Find combines canonical `domain.action` IDs, domain/action names, aliases, natural-language stopword filtering (removing frequent non-informative words), synonyms, fuzzy matching, and segmented matching for multi-intent prompts. Models should use `gitlab_find_action` to retrieve exact schemas, then execute the canonical action ID returned by find. See `docs/concepts/dynamic-tools.md` and ADR-0011.

### Enterprise tool gating

`GITLAB_TIER` controls access to GitLab Premium/Ultimate features in stdio mode (Enterprise tools are gated when the resolved tier is Premium or Ultimate). In HTTP mode, the `--tier` flag forces the tier; when omitted, the tier is detected from the instance license per token+URL pool entry (fallback `free`). The catalog effect is the same in individual and meta-tool modes.

The tier affects tool registration (input/output schemas and tool lists) through `pruneSchemaFieldsByTier` in `internal/tools/action_catalog.go:138` — every registered action has its input schema pruned strictly (lower-tier clients never see higher-tier input fields, even though the SDK type still carries them) and its output schema pruned leniently (`lenientExtra=true`: higher-tier output fields are kept but omitted from the model-facing schema, so a Premium client reading an Ultimate response sees the data; an Ultimate client reading a Premium response does not). The 3-tier field-level gating is described per-field with `tier:"premium"` / `tier:"ultimate"` struct tags throughout `internal/tools/*/action_specs.go` and `internal/tools/*/shapes.go`.

**Individual mode** (`GITLAB_MCP_TOOL_SURFACE=individual`; legacy `GITLAB_MCP_META_TOOLS=false`) — gates Enterprise/Premium actions through catalog metadata:

- projects (push rules), projectmirrors, mergetrains, auditevents, dorametrics, dependencies, externalstatuschecks, groupscim, memberroles, enterpriseusers, attestations, compliancepolicy, projectaliases, geo, groupstoragemoves, vulnerabilities, securityattributes, securitycategories, securityfindings, securitysettings, groupanalytics, groupcredentials, groupsshcerts, projectiterations, groupiterations, epics, epicissues, epicnotes, epicdiscussions, groupepicboards, groupwikis, groupprotectedbranches, groupprotectedenvs, groupreleases, groupldap, groupsaml, groupserviceaccounts

**Meta-tool mode** (`GITLAB_MCP_TOOL_SURFACE=meta`) — gates 17 dedicated Enterprise/Premium catalog groups:

- gitlab_merge_train, gitlab_audit_event, gitlab_dora_metrics, gitlab_dependency, gitlab_external_status_check, gitlab_group_scim, gitlab_member_role, gitlab_enterprise_user, gitlab_attestation, gitlab_compliance_policy, gitlab_project_alias, gitlab_geo, gitlab_vulnerability, gitlab_security_attribute, gitlab_security_category, gitlab_security_finding, gitlab_security_scan_profile

Plus enterprise-only routes injected into 3 base meta-tools:

- `gitlab_project` → push_rule_*, mirror_*, security_settings_*
- `gitlab_group` → iterations, epics, wikis, protected branches/envs, releases, LDAP, SAML, SSH certs, credentials, analytics, service accounts
- `gitlab_issue` → iterations

**Dynamic mode** (default) — the same catalog gating applies: `gitlab_find_action` never lists and `gitlab_execute_action` never runs an action above the resolved tier, so the tier changes which `domain.action` IDs exist rather than which tools are registered.

### OAuth admission, per-action write gating, and HTTP routing

Three invariants of HTTP mode that are easy to break by touching the wrong layer:

- **Admission asks for the minimum, not the deployment's scope.** `oauth.MinimumScope` (`read_api`) is what the door checks; `oauth.RequiredScope` is only what the challenge _recommends_ and the first entry of `oauth.SupportedScopes`. A `read_api` token is admitted by a deployment that writes and is served a read-only surface: `serverpool.applyScopeReadOnly` sets `ServerConfig.ReadOnly` when `gitlabclient.WriteCapable` says the token cannot write, so the write check is the per-action one the catalog already carries. Do not reintroduce a deployment-wide scope demand at the door — that is what refused a read-only OAuth application at `initialize`. Unknown scopes (`nil`) mean write-capable: a wrong "no" silently removes tools, a wrong "yes" surfaces as GitLab's own 403. **A narrowed surface must say it is narrowed.** `filterActionCatalog` returns what each filter removed, split into `byTokenScope` and `byOperator`, and the dynamic registry answers a request for one of those with the cause (`dynamic.WithWithheldActions`) instead of `unknown action … Did you mean …`. The suggestions in that older message were all real read-only actions, so a model reading it concluded the server lacked the capability rather than that the credential was narrow. Tools removed by name through `--exclude-tools` stay out of both lists: the operator asked for them not to exist.
- **Routing happens before authentication, but only stateless GET/DELETE skip the gate.** `mountMCPEndpoint` mounts the MCP handler on `/{$}`, `/mcp`, and the `--public-url` path prefix (for a proxy that forwards its prefix instead of stripping it); everything else is an unauthenticated 404. Non-POST methods bypass the credential check **only when `--stateless`**, where the SDK answers 405 whatever they carry: on `--stateless=false` a GET opens a session's standalone SSE stream and a DELETE terminates the session, so both are resolved and ownership-checked like a POST. The SDK's own server-initiated keepalive ping is off for every HTTP pool entry (`withKeepAlive(0)`), stateful included — it closes a session on the first unanswered ping, which idle clients fail. Liveness is the SSE keep-alive comment in `sseAwareWriter` instead, which puts bytes on the wire without asking the client for anything. A catch-all auth gate told every scanner that `/.well-known/oauth-authorization-server` was a protected document that is not there. `securityHeadersMiddleware` is the outermost wrapper so even a middleware that answers instead of forwarding (host validation's 403) carries `nosniff`, CSP, `X-Frame-Options` and `Referrer-Policy`.
- **The instance is chosen from an allow-list, and verification follows it.** `--gitlab-url` is repeatable; `serverpool.ResolveRequestOptionsFor` refuses a `GITLAB-URL` header naming an unpublished instance instead of ignoring it, `oauth.NewGitLabVerifierFor` verifies against the instance the request selected, and `oauth.TokenCache` keys on instance **and** token. Keying that cache on the token alone would let a credential verified against one published instance pass as identity on another.

Nothing here should need a reverse proxy to be correct: the binary answers its own CORS preflights, its own 404s, its own security headers, and can terminate TLS or listen on a unix socket itself.

### Resource subscriptions (ADR-0015)

`resources/subscribe` is honored by polling, on `GITLAB_MCP_CAPABILITY_SURFACE=full` only. `internal/subscriptions/` owns the watchers and knows nothing about MCP; `cmd/server/subscriptions.go` is the bridge.

- **What is subscribable** — a whitelist of 26 kinds in `kind.go` — single objects plus three single-parent lists (a pipeline's jobs, an MR's discussions and notes) — parsed segment by segment. Open-ended top-level collections are excluded on purpose. Two drift guards round-trip the real resource registry, so a new template must be classified and a renamed one cannot leave a stale decision behind.
- **Reads go through the registered handler** — `internal/resources.HandlerIndex` captures the same handlers `resources/read` dispatches to, so "the content changed" means "what a client would read changed". `NewHandlerIndex` exists because `mcp.ServerOptions` must be built before `mcp.NewServer` returns.
- **The session is the subscriber identity** — `Manager[S comparable]` holds a set of subscribers per watcher, not a count, so a duplicate subscribe is idempotent and one session cannot release another's watch. `sessionBridge` passes `*mcp.ServerSession` as that identity, and waits on `ServerSession.Wait()` to call `UnsubscribeAll`: the SDK drops a disconnected session from its subscriber table without ever calling `UnsubscribeHandler`.
- **A joiner waits for the first read** — the watcher's `ready` channel carries the outcome of the initial read (which is the authorization check), so a second subscriber is never told "subscribed" while the only read anyone attempted is still in flight or has already failed.
- **Renewal happens after the handler, not before it** — a subscribe request is itself activity, so renewing first would un-demote every watcher a moment before that same request looked for a demoted one to evict, and eviction at the cap could never fire.
- **The lease demotes, it does not stop** — 30 minutes without traffic drops a watch to a 10-minute poll; any request that reaches the server restores it (`renewOnActivity` middleware). From protocol 2026-07-28 a client may answer `tools/list` or a repeated `resources/read` from its own cache, so those never renew. Only `MaxLifetime` (24h), a 401/403/404, or eviction at the cap stop one without the client acting; `resources/unsubscribe` and session disconnect stop one from the client side.
- **Stateless HTTP refuses the legacy path, and only that path** — each stateless POST gets its own session that closes with the response, so `resources/subscribe` is answered with an error rather than accepted and left undeliverable. The capability bit stays on because `subscriptions/listen` does work there. That last sentence is load-bearing and was false for a while: the SDK routes both methods through the one `SubscribeHandler` (`subscriptionsListen` calls it per resource URI and returns the first error before acknowledging anything), so a refusal written for the legacy method killed every listen carrying a resource, and took a mixed listen's list-changed half with it. The listen path is marked in `listenStreams.middleware` and `subscribeUnlessStateless` consults that mark. A handler-level test cannot see this class of defect — the handler is correct in isolation and wrong only because of who else calls it — so the regression lives in `test/e2e/http/subscriptions_test.go`, on the wire.
- **Ending a watch closes the client's stream** — on protocol 2026-07-28 a subscription is an open `subscriptions/listen` request. `SubscriptionsListenResult` cannot be constructed by application code, so `listenStreams` cancels the SDK handler's context, which makes the SDK emit it. A stream is only closed when every URI it carries has stopped, and never if it also carries list-changed subscriptions.

Do not use `synctest` for tests that need a real HTTP backend: an `httptest` server's goroutines block on I/O, which is not a durable block, so the fake clock never advances and the test hangs until its timeout. The manager's own tests use fakes and synctest; the wiring tests use real time with short intervals injected through `withSubscriptionOptions`.

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

- **TLS errors**: Set `GITLAB_SKIP_TLS_VERIFY=true` for self-signed certs
- **Tool not found**: Check the action's `ActionSpec`, catalog aggregation, `action_catalog.go`, and `docs/development/tool-surfaces-and-action-core.md` for surface ownership rules
- **Meta-tools disabled**: legacy `GITLAB_MCP_META_TOOLS=false` maps to `GITLAB_MCP_TOOL_SURFACE=individual`; unset both to get the default dynamic surface, or set `GITLAB_MCP_TOOL_SURFACE=meta` explicitly when meta-tools are what you want
- **Dynamic mode shows only two tools**: this is expected by default. Use `gitlab_find_action` and `gitlab_execute_action`; set `GITLAB_MCP_TOOL_SURFACE=meta` to use meta-tools.
- **Pagination missing**: Ensure tool uses `buildPaginationResponse()` helper for list operations
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

E2E tests run against a real GitLab instance using in-memory MCP transport (no network). Two modes are supported:

**Self-hosted mode** — requires a `.env` file with `GITLAB_URL` and `GITLAB_TOKEN` (user must have permissions to create/delete projects):

```bash
# Run full E2E suite (three workflows: individual tools + meta-tools + dynamic tools)
go test -v -tags e2e -timeout 300s ./test/e2e/suite/
make test-e2e

# Compile-only check (no GitLab needed)
go test -tags e2e -c -o NUL ./test/e2e/suite/       # Windows
go test -tags e2e -c -o /dev/null ./test/e2e/suite/  # Linux
```

**Docker mode** — ephemeral GitLab CE container with CI runner and fixture service (enables pipeline/job tests and deterministic webhook/custom-emoji/mirror endpoints):

```bash
export E2E_BITBUCKET_ADMIN_PASSWORD=$(openssl rand -hex 16)
docker compose -f test/e2e/docker-compose.yml --profile bitbucket up -d
./test/e2e/scripts/wait-for-gitlab.sh && ./test/e2e/scripts/setup-gitlab.sh && ./test/e2e/scripts/register-runner.sh && ./test/e2e/scripts/setup-bitbucket.sh
set -a && source test/e2e/.env.docker && set +a
go test -v -tags e2e -timeout 600s ./test/e2e/suite/
docker compose -f test/e2e/docker-compose.yml --profile bitbucket down -v
```

The suite runs three sequential workflows:

- **TestFullWorkflow** (~174 subtests): exercises all individual tools through a complete project lifecycle (user → project CRUD → commits → branches → tags → releases → issues → labels → milestones → members → upload → MR lifecycle → notes → discussions → search → groups → pipelines → packages → elicitation → cleanup)
- **TestMetaToolWorkflow** (~151 subtests): exercises the same operations through meta-tools plus 15 additional domains (wikis, CI variables, CI lint, environments, issue links, deploy keys, snippets, issue discussions, draft notes, pipeline schedules, badges, access tokens, award emoji, labels, milestones)
- **TestDynamicToolSurface**: exercises the default dynamic two-tool find/execute surface, including standalone project discovery, multi-intent discovery, and destructive-action confirmation guards. Run only this workflow in Docker mode after the Docker GitLab setup scripts complete:

	```bash
	E2E_MODE=docker \
		go test -v -tags e2e -timeout 600s \
		-run '^TestDynamicToolSurface' \
		./test/e2e/suite/
	```

Domains **added in Docker mode** (require CI runner):

- Pipeline create/get/cancel/retry/delete
- Job get/log/retry/cancel

**MCP capability tests** (mock handlers, always available):

- Elicitation tools (1 test): confirm destructive action
