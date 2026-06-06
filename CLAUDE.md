# gitlab-mcp-server — AI Development Context

> This file provides comprehensive context for AI assistants working on this project.
> All project artifacts must be written in **English**. Conversations may be in any language.

## Project Overview

**gitlab-mcp-server** is a Model Context Protocol (MCP) server written in Go that exposes GitLab REST API v4 and GraphQL operations as MCP tools for AI assistants. It runs as a local binary communicating via stdio or HTTP transport.

| Attribute     | Value                                               |
| ------------- | --------------------------------------------------- |
| Language      | Go 1.26.4                                           |
| MCP SDK       | `github.com/modelcontextprotocol/go-sdk/mcp` v1.6.0 |
| GitLab Client | `gitlab.com/gitlab-org/api/client-go/v2` v2.29.0       |
| Transport     | stdio (primary), HTTP (optional)                    |
| Platforms     | Windows, Linux & macOS, amd64 & arm64               |
| Version       | 2.1.3                                               |

### Scale

| Metric                    | Count                                                                                                        |
| ------------------------- | ------------------------------------------------------------------------------------------------------------ |
| MCP Tools (individual)    | 1027 self-managed Enterprise/Premium; 1033 on GitLab.com Enterprise/Premium with Orbit                     |
| Meta-mode tools           | 33 base / 49 self-managed enterprise / 50 GitLab.com Enterprise (Orbit)                                    |
| Dynamic-mode tools        | 2 dynamic tools (`gitlab_find_action`, `gitlab_execute_action`) — see Dynamic toolset mode below |
| MCP Resources             | 46 across dynamic/full, meta/full, and individual/full modes; `gitlab://tools` adapts to the active surface |
| MCP Prompts               | 37 (12 core + 4 cross-project + 4 team + 5 project-reports + 4 analytics + 4 milestone-label + 2 git-workflow + 2 audit)      |
| Completion argument types | 17                                                                                                           |
| MCP Capabilities          | 6 (logging, progress, roots, sampling, elicitation, completions)                                             |
| MCP Icons                 | 50 domain SVG icons (base64 data URIs, `Sizes: ["any"]`) on all tools, resources, and prompts                |
| Source files (tools)      | 737 non-test Go files under `internal/tools/`                                                                |
| Test files (tools)        | 347 test files under `internal/tools/`                                                                       |
| Go packages               | 215 total; 176 under `internal/tools/...`                                                                    |

## Project Structure

```text
gitlab-mcp-server/
├── cmd/
│   ├── server/                  # MCP server entry point and --shutdown support
│   ├── add_docs/                # AST-based tool: adds godoc comments to undocumented symbols
│   ├── audit_action_spec_coverage/ # Audits ActionSpec catalog coverage
│   ├── audit_dynamic_aliases/   # Audits dynamic discovery aliases
│   ├── audit_eval_coverage/     # Audits evaluation case coverage
│   ├── audit_godocs/            # Audits Go documentation coverage
│   ├── audit_meta_schema/       # Audits meta-tool schema generation
│   ├── audit_metrics/           # Audits MCP tool/resource/prompt metrics
│   ├── audit_output/            # Audits MCP tool output quality
│   ├── audit_test_names/        # Audits test function naming convention compliance
│   ├── audit_tokens/            # Audits token usage for model-facing surfaces
│   ├── audit_tools/             # Audits MCP tool metadata violations
│   ├── eval_mcp_surfaces/       # Evaluates model-facing MCP surface behavior
│   ├── find_dupes/              # Finds duplicated string literals missing constants
│   ├── format_md_tables/        # Formats Markdown pipe tables in README.md and docs/
│   ├── gen_action_catalog_manifest/ # Generates audited action catalog manifest
│   ├── gen_docker_tools/        # Generates Docker-related tool metadata
│   ├── gen_llms/                # Generates llms.txt and llms-full.txt for LLM discovery
│   ├── gen_readme/              # Generates README sections from source metadata
│   └── gen_testing_docs/        # Generates docs/testing/testing.md
├── internal/
│   ├── autoupdate/              # Self-update: background startup checks, rename trick, restart activation
│   ├── config/                  # Configuration loading (.env, flags, env vars)
│   ├── gitlab/                  # GitLab API client wrapper (client.GL() accessor)
│   ├── oauth/                   # OAuth HTTP mode: token cache, GitLab verifier, header middleware, RFC 9728 metadata
│   ├── serverpool/              # HTTP mode: bounded LRU pool of per-token+URL MCP servers (with observability metrics)
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
│   │   ├── samplingtools/       # LLM sampling tools (summarize/analyze)
│   │   ├── search/              # Search tools (code, MRs, issues, etc.)
│   │   ├── serverupdate/       # Server self-update MCP tools (check/apply)
│   │   ├── projectdiscovery/   # Git remote URL to GitLab project resolution
│   │   ├── tags/                # Tag tools
│   │   ├── todos/               # Todo tools
│   │   ├── uploads/             # Project upload tools
│   │   ├── users/               # User tools
│   │   └── wikis/               # Wiki tools
│   ├── resources/               # 46 MCP resource implementations
│   ├── prompts/                 # 37 MCP prompt implementations
│   ├── completions/             # 17 argument completion types
│   ├── logging/                 # MCP logging capability
│   ├── progress/                # MCP progress notifications
│   ├── roots/                   # MCP roots capability
│   ├── sampling/                # MCP sampling capability
│   ├── elicitation/             # MCP elicitation capability
│   └── wizard/                  # Setup wizard (Web UI, TUI, CLI modes)
├── docs/                        # Project documentation (Diátaxis framework)
│   ├── adr/                     # Architectural Decision Records
│   ├── tools/                   # Per-domain tool documentation
│   ├── capabilities/            # MCP capability docs
│   ├── examples/                # Usage examples
│   ├── oauth-app-setup.md       # Creating GitLab OAuth applications for MCP clients
│   └── ide-configuration.md     # Per-IDE MCP JSON configuration (stdio, HTTP legacy, OAuth)
├── test/e2e/                    # End-to-end integration tests
│   ├── docker-compose.yml       # Ephemeral GitLab CE + Runner + fixture service for Docker mode
│   ├── .env.docker              # Docker mode environment variables
│   ├── README.md                # E2E documentation
│   ├── scripts/                 # E2E provisioning scripts (setup, runner, wait)
│   └── suite/                   # Go test package (91 test files)
│       ├── setup_test.go        # MCP server/client setup, test helpers, shared state
│       └── fixture_test.go      # Self-contained GitLab resource builders
├── plan/                        # Implementation plans for features
├── .github/                     # AI assistance infrastructure
│   ├── copilot-instructions.md  # GitHub Copilot context (auto-loaded by VS Code)
│   ├── agents/                  # 7 specialized AI agents
│   ├── skills/                  # 18 reusable skill templates
│   └── instructions/            # 7 coding standard instruction files
├── Makefile                     # Build, test, lint targets
└── VERSION                      # Semantic version (2.1.3)
```

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
10. Update `docs/tools/{domain}.md` and `docs/tools/README.md`
11. After completing a test-focused tool implementation phase, run `go run ./cmd/gen_testing_docs/` or `make gen-testing-docs` to refresh `docs/testing/testing.md`, then verify with `go run ./cmd/gen_testing_docs/ --check`

See `docs/output-format.md` for the complete response format specification.

### Tool naming convention

`gitlab_{action}_{resource}` in snake_case (e.g., `gitlab_create_issue`, `gitlab_list_projects`)

### Error handling in tool handlers

Four error wrapping functions in `internal/toolutil/errors.go`, used across the 176 packages under `internal/tools/`:

- `WrapErr(op, err)` — read-only operations (list, get, search). Generic classification only.
- `WrapErrWithMessage(op, err)` — mutating operations (create, update, delete). Includes GitLab-specific error detail via `ExtractGitLabMessage`.
- `WrapErrWithHint(op, err, hint)` — when a specific corrective action is known (e.g., "use gitlab_branch_unprotect first"). Includes detail + actionable suggestion.
- `WrapErrWithStatusHint(op, err, code, hint)` — combines `IsHTTPStatus` check + `WrapErrWithHint` in a single call. Use when the hint applies only to a specific HTTP status code; returns `WrapErrWithMessage` for all other codes.
- `NotFoundResult(resource, identifier, hints...)` — for get handlers when `IsHTTPStatus(err, 404)`. Returns an informational `CallToolResult` with `IsError: true` and domain-specific hints instead of a Go error. Logged at INFO level. Applied to 27 get handlers across 21 domains. Defined in `internal/toolutil/not_found.go`.

Use `IsHTTPStatus(err, code)` and `ContainsAny(err, substrs...)` for status-specific branching before calling `WrapErrWithHint`. For get handlers, check `IsHTTPStatus(err, 404)` **before** `LogToolCallAll` and return `NotFoundResult` with `nil` error to log at INFO instead of ERROR. See [ADR-0007](docs/adr/adr-0007-rich-error-semantics.md) and [Error Handling](docs/error-handling.md).

### Test infrastructure

All tests use `httptest` to mock GitLab API responses. Shared helpers in `internal/testutil/`:

- `testutil.NewTestClient()` — creates a mock GitLab client pointing to httptest server
- `testutil.RespondJSON()` — responds with JSON body
- `testutil.RespondJSONWithPagination()` — responds with pagination headers
- Test naming: `TestToolName_Scenario_ExpectedResult`

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
make test-e2e-docker                                   # Ephemeral GitLab CE + runner + fixture service (Docker, ~4 GB RAM)
go test -tags e2e -c -o NUL ./test/e2e/suite/           # Compile-only check (Windows)
go test -tags e2e -c -o /dev/null ./test/e2e/suite/     # Compile-only check (Linux)

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

When creating a new release and uploading binaries to GitHub Releases:

1. Build cross-platform binaries with `make release` (uses GoReleaser locally, flattens `dist/` to match GitHub Release asset names)
2. **Release link names MUST be exact filenames** (e.g. `checksums.txt.asc`, `gitlab-mcp-server-linux-amd64`). Never add descriptive suffixes like `(GPG signature)` — `go-selfupdate` matches asset names exactly and will fail to find files with decorated names

### Post-implementation verification

After making changes, run targeted verification on the **changed files/packages only** (not the entire project):

```bash
# Go files — run on affected packages
go test ./internal/tools/branches/ -count=1    # tests on changed package
golangci-lint run --build-tags e2e ./internal/tools/branches/ # lint changed package

# Markdown files — run on specific changed files
npx markdownlint-cli2 docs/auto-update.md README.md  # lint specific .md files
npx markdownlint-cli2 --fix docs/auto-update.md      # auto-fix specific .md files

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

| Variable                 | Required | Description                                              |
| ------------------------ | -------- | -------------------------------------------------------- |
| `GITLAB_URL`             | Stdio    | GitLab instance URL (e.g., `https://gitlab.example.com`). In HTTP mode, optional via `--gitlab-url`; when set it fixes the GitLab instance, and when omitted clients must send `GITLAB-URL` per request |
| `GITLAB_TOKEN`           | Stdio    | Personal Access Token (`glpat-...`)                      |
| `GITLAB_SKIP_TLS_VERIFY` | No       | Skip TLS verification for self-signed certs (`true`)     |
| `META_TOOLS`             | No       | Deprecated compatibility selector; prefer `TOOL_SURFACE` for new configs |
| `TOOL_SURFACE`           | No       | Explicit tool catalog selector: `dynamic`, `meta`, or `individual`; default is `dynamic` when unset, unless legacy `META_TOOLS` is explicitly set |
| `CAPABILITY_SURFACE`     | No       | Resource and prompt catalog selector: `full` or `minimal`; `minimal` keeps `gitlab://workspace/roots` plus the surface-aware `gitlab://tools` manifest |
| `META_PARAM_SCHEMA`      | No       | Meta-tool input-schema strategy: `opaque` (default), `compact` (~5x), or `full` (~10x). Independent of `META_TOOLS`. Per-action call shapes and input schemas are discoverable through `gitlab://tools` and `gitlab://tools/{id}` for every surface |
| `GITLAB_READ_ONLY`       | No       | Read-only mode: disables all mutating tools (`false` default) |
| `GITLAB_SAFE_MODE`       | No       | Safe mode: intercepts mutating tools and returns a JSON preview (`false` default) |
| `AUTO_UPDATE`            | No       | Enable auto-update: `true` (default), `check`, `false`  |
| `AUTO_UPDATE_REPO`       | No       | GitHub repository slug for release assets (`jmrplens/gitlab-mcp-server`) |
| `AUTO_UPDATE_INTERVAL`   | No       | Periodic check interval (`1h` default, HTTP mode)        |
| `AUTO_UPDATE_TIMEOUT`    | No       | Startup/background update timeout (`60s` default, range 5s–10m) |
| `GITLAB_ENTERPRISE`      | No       | Enable Enterprise/Premium tools in stdio mode. In HTTP mode, `--enterprise` explicitly forces the Enterprise/Premium catalog; when omitted, CE/EE is auto-detected per token+URL pool entry when GitLab reports edition (`false` default) |
| `AUTH_MODE`              | No       | HTTP mode auth: `legacy` (default) or `oauth` (RFC 9728 Bearer verification) |
| `OAUTH_CACHE_TTL`        | No       | OAuth token identity cache TTL (`15m` default, range 1m–2h) |
| `RATE_LIMIT_RPS`         | No       | Per-server tools/call rate limit in req/s (`0` = disabled) |
| `RATE_LIMIT_BURST`       | No       | Token-bucket burst size when RPS > 0 (`40` default)       |
| `LOG_LEVEL`              | No       | Logging verbosity (`debug`, `info`, `warn`, `error`)     |

In **HTTP mode**, configuration comes from CLI flags instead of environment variables:

| Flag                  | Default | Description                                              |
| --------------------- | ------- | -------------------------------------------------------- |
| `--gitlab-url`        | —       | Fixed GitLab instance URL (optional; omit to require `GITLAB-URL` per request) |
| `--skip-tls-verify`   | `false` | Skip TLS verification for self-signed certs              |
| `--meta-tools`        | `true`  | Enable meta-tools for tool discovery                     |
| `--tool-surface`      | _(empty)_ | Explicit tool catalog selector: `meta`, `individual`, or `dynamic`; overrides `--meta-tools` when set |
| `--capability-surface` | `full` | Resource and prompt catalog selector: `full` or `minimal` |
| `--enterprise`        | `false` | Force Enterprise/Premium tools when explicitly set; omit to auto-detect CE/EE per token+URL pool entry when GitLab reports edition |
| `--read-only`         | `false` | Read-only mode: disables all mutating tools              |
| `--safe-mode`         | `false` | Safe mode: intercepts mutating tools, returns preview    |
| `--max-http-clients`  | `100`   | Maximum concurrent client sessions                       |
| `--session-timeout`   | `30m`   | Idle session timeout                                     |
| `--http-addr`         | `:8080` | HTTP listen address                                      |
| `--auth-mode`         | `legacy` | Authentication mode: `legacy` or `oauth` (RFC 9728 Bearer verification) |
| `--oauth-cache-ttl`   | `15m`   | OAuth token identity cache TTL (range 1m–2h)             |
| `--revalidate-interval` | `15m` | Token re-validation interval; `0` to disable (upper bound: 24h) |
| `--trusted-proxy-header` | _(empty)_ | HTTP header with real client IP for rate limiting behind proxies (e.g. `Fly-Client-IP`, `X-Forwarded-For`) |
| `--auto-update`       | `true`  | Enable auto-update (`true`, `check`, `false`)            |
| `--auto-update-repo`  | `jmrplens/gitlab-mcp-server` | GitHub repository for release assets |
| `--auto-update-interval` | `1h` | Periodic update check interval                           |
| `--rate-limit-rps` | `0` | Per-server tools/call rate limit in req/s (0 = disabled) |
| `--rate-limit-burst` | `40` | Token-bucket burst size when --rate-limit-rps > 0        |
| `--auto-update-timeout` | `60s` | Startup/background update timeout (range 5s–10m)         |

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
| **Test Expert** | `test-expert.agent.md`  | Writing, analyzing, and improving Go tests. Covers new test development, existing test analysis, coverage analysis to 90%+, false-pass detection, edge case identification, mandatory test documentation, and refreshing `docs/testing/testing.md` with `cmd/gen_testing_docs` at phase completion. Uses Context7 for up-to-date Go testing docs. |

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
| **Create ADR**                 | `create-architectural-decision-record/` | ADR with standardized format (POS-001, NEG-001, etc.). Saves to `docs/adr/`.                           |
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

1. Use `@Test Expert` agent — it runs `go test -coverprofile`, identifies gaps, detects false passes, generates documented tests, and refreshes `docs/testing/testing.md` with `go run ./cmd/gen_testing_docs/` at the end of the test phase
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

ADRs document key decisions in `docs/adr/`:

| ADR      | Decision                                                       | Status                                       |
| -------- | -------------------------------------------------------------- | -------------------------------------------- |
| ADR-0004 | Modular sub-packages under `internal/tools/{domain}/`          | Accepted (176 `internal/tools` packages, 1027 self-managed tools / 1033 GitLab.com Enterprise tools) |
| ADR-0006 | Raw GraphQL.Do() for domains without client-go service wrappers | Accepted (7 GraphQL-only domains)             |
| ADR-0007 | Rich error semantics for LLM-actionable diagnostics            | Accepted (WrapErrWithMessage, WrapErrWithHint) |
| ADR-0009 | Progressive GraphQL migration strategy                         | Accepted (trigger-based REST→GraphQL migration) |

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

`TOOL_SURFACE=dynamic` registers only `gitlab_find_action` and `gitlab_execute_action`. It is the default when `TOOL_SURFACE` and legacy `META_TOOLS` are unset. The dynamic registry is built from the canonical action catalog shared with meta-tools and augmented with standalone routes such as project discovery, so execution reuses existing handlers, typed schemas, destructive-action classification, read-only filtering, safe-mode previews, markdown formatters, and scope filtering.

Developers add normal GitLab actions through domain-local `ActionSpecs` and the audited catalog aggregation path. `internal/tools/action_catalog.go` builds the canonical catalog from those specs; meta-tools register visible domain dispatchers from it, dynamic mode builds find/execute over it, and individual mode projects one visible tool per action from the same catalog. Do not add package-local `RegisterTools` functions, duplicate dynamic-only action definitions, or package-level meta registration for ordinary GitLab API operations. See `docs/development/tool-surfaces-and-action-core.md` for the detailed developer architecture.

Find combines canonical `domain.action` IDs, domain/action names, aliases, natural-language stopword filtering (removing frequent non-informative words), synonyms, fuzzy matching, and segmented matching for multi-intent prompts. Models should use `gitlab_find_action` to retrieve exact schemas, then execute the canonical action ID returned by find. See `docs/dynamic-tools.md` and ADR-0011.

### Enterprise tool gating

`GITLAB_ENTERPRISE` controls access to GitLab Premium/Ultimate features in stdio mode. In HTTP mode, the `--enterprise` flag explicitly forces the Premium/Ultimate catalog; when omitted, CE/EE is auto-detected per token+URL pool entry when GitLab reports edition. The catalog effect is the same in individual and meta-tool modes:

**Individual mode** (`TOOL_SURFACE=individual`; legacy `META_TOOLS=false`) — gates Enterprise/Premium actions through catalog metadata:

- projects (push rules), projectmirrors, mergetrains, auditevents, dorametrics, dependencies, externalstatuschecks, groupscim, memberroles, enterpriseusers, attestations, compliancepolicy, projectaliases, geo, groupstoragemoves, vulnerabilities, securityattributes, securitycategories, securityfindings, securitysettings, groupanalytics, groupcredentials, groupsshcerts, projectiterations, groupiterations, epics, epicissues, epicnotes, epicdiscussions, groupepicboards, groupwikis, groupprotectedbranches, groupprotectedenvs, groupreleases, groupldap, groupsaml, groupserviceaccounts

**Meta-tool mode** (`TOOL_SURFACE=meta`) — gates 16 dedicated Enterprise/Premium catalog groups:

- gitlab_merge_train, gitlab_audit_event, gitlab_dora_metrics, gitlab_dependency, gitlab_external_status_check, gitlab_group_scim, gitlab_member_role, gitlab_enterprise_user, gitlab_attestation, gitlab_compliance_policy, gitlab_project_alias, gitlab_geo, gitlab_vulnerability, gitlab_security_attribute, gitlab_security_category, gitlab_security_finding

Plus enterprise-only routes injected into 3 base meta-tools:

- `gitlab_project` → push_rule_*, mirror_*, security_settings_*
- `gitlab_group` → iterations, epics, wikis, protected branches/envs, releases, LDAP, SAML, SSH certs, credentials, analytics, service accounts
- `gitlab_issue` → iterations

---

## Debugging Tips (Development)

### MCP transport debugging

The server communicates via stdio (JSON-RPC over stdin/stdout). To debug:

```bash
# Run with debug logging
LOG_LEVEL=debug ./gitlab-mcp-server 2>debug.log

# HTTP mode for easier debugging with curl
./gitlab-mcp-server --http --http-addr=localhost:8080
curl -X POST http://localhost:8080/mcp -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"tools/list","id":1}'
```

### Common issues

- **TLS errors**: Set `GITLAB_SKIP_TLS_VERIFY=true` for self-signed certs
- **Tool not found**: Check the action's `ActionSpec`, catalog aggregation, `action_catalog.go`, and `docs/development/tool-surfaces-and-action-core.md` for surface ownership rules
- **Meta-tools disabled**: legacy `META_TOOLS=false` maps to `TOOL_SURFACE=individual`; prefer setting `TOOL_SURFACE=meta` explicitly
- **Dynamic mode shows only two tools**: this is expected by default. Use `gitlab_find_action` and `gitlab_execute_action`; set `TOOL_SURFACE=meta` to use meta-tools.
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
docker compose -f test/e2e/docker-compose.yml up -d
./test/e2e/scripts/wait-for-gitlab.sh && ./test/e2e/scripts/setup-gitlab.sh && ./test/e2e/scripts/register-runner.sh
set -a && source test/e2e/.env.docker && set +a
go test -v -tags e2e -timeout 600s ./test/e2e/suite/
docker compose -f test/e2e/docker-compose.yml down -v
```

The suite runs three sequential workflows:

- **TestFullWorkflow** (~174 subtests): exercises all individual tools through a complete project lifecycle (user → project CRUD → commits → branches → tags → releases → issues → labels → milestones → members → upload → MR lifecycle → notes → discussions → search → groups → pipelines → packages → sampling → elicitation → cleanup)
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

- Sampling tools (11 tests): summarize issue, analyze MR, generate release notes, etc.
- Elicitation tools (1 test): confirm destructive action
