# gitlab-mcp-server — AI Development Context

> This file provides comprehensive context for AI assistants working on this project.
> All project artifacts must be written in **English**. Conversations may be in any language.

## Project Overview

**gitlab-mcp-server** is a Model Context Protocol (MCP) server written in Go that exposes GitLab REST API v4 operations as MCP tools for AI assistants. It runs as a local binary communicating via stdio or HTTP transport.

| Attribute     | Value                                               |
| ------------- | --------------------------------------------------- |
| Language      | Go 1.26+                                            |
| MCP SDK       | `github.com/modelcontextprotocol/go-sdk/mcp` v1.5.0 |
| GitLab Client | `gitlab.com/gitlab-org/api/client-go/v2` v2.20.1       |
| Transport     | stdio (primary), HTTP (optional)                    |
| Platforms     | Windows, Linux & macOS, amd64 & arm64               |
| Version       | 1.2.4                                               |

### Scale

| Metric                    | Count                                                                                                        |
| ------------------------- | ------------------------------------------------------------------------------------------------------------ |
| MCP Tools (individual)    | 1000                                                                                                         |
| Meta-mode tools           | 32 base / 47 enterprise (24 inline + 3 delegated + 1 standalone + 4 interactive + 15 enterprise inline) |
| MCP Resources             | 24                                                                                                           |
| MCP Prompts               | 38 (12 core + 4 cross-project + 4 team + 5 project-reports + 4 analytics + 4 milestone-label + 5 audit)      |
| Completion argument types | 17                                                                                                           |
| MCP Capabilities          | 6 (logging, progress, roots, sampling, elicitation, completions)                                             |
| MCP Icons                 | 50 domain SVG icons (base64 data URIs, `Sizes: ["any"]`) on all tools, resources, and prompts                |
| Source files (tools)      | 522+ (infrastructure + 162 sub-packages)                                                                     |
| Test files (tools)        | 320                                                                                                          |
| Go packages               | 187 (16 core + 163 tool packages + 8 cmd)                                                                    |

## Project Structure

```text
gitlab-mcp-server/
├── cmd/
│   ├── server/main.go          # Entry point, transport setup, graceful shutdown
│   │   └── shutdown.go         # --shutdown flag: terminate all running instances
│   ├── add_docs/main.go        # AST-based tool: adds godoc comments to undocumented symbols
│   ├── audit_output/main.go    # Audits MCP tool output quality (OutputSchema, annotations)
│   ├── audit_metrics/main.go   # Audits MCP tool metrics (tool count, resource count, etc.)
│   ├── audit_tools/main.go     # Audits MCP tool metadata violations (naming, annotations)
│   ├── audit_test_names/main.go # Audits test function naming convention compliance
│   ├── gen_llms/main.go        # Generates llms.txt and llms-full.txt for LLM discovery
│   └── find_dupes/main.go      # Finds duplicated string literals missing constants
├── internal/
│   ├── autoupdate/              # Self-update: pre-start check, rename trick, syscall.Exec (Unix)
│   ├── config/                  # Configuration loading (.env, flags, env vars)
│   ├── gitlab/                  # GitLab API client wrapper (client.GL() accessor)
│   ├── oauth/                   # OAuth HTTP mode: token cache, GitLab verifier, header middleware, RFC 9728 metadata
│   ├── serverpool/              # HTTP mode: bounded LRU pool of per-token+URL MCP servers (with observability metrics)
│   ├── toolutil/                # Shared tool utilities (errors, pagination, markdown, logging)
│   ├── testutil/                # Shared test helpers (NewTestClient, RespondJSON)
│   ├── tools/                   # Tool orchestration layer + 162 domain sub-packages
│   │   ├── register.go          # RegisterAll() — delegates to sub-package RegisterTools()
│   │   ├── register_meta.go     # RegisterAllMeta() — delegates to sub-package RegisterMeta()
│   │   ├── markdown.go          # Thin delegator to type-based markdown registry (toolutil.MarkdownForResult)
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
│   ├── resources/               # 24 MCP resource implementations
│   ├── prompts/                 # 38 MCP prompt implementations
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
│   ├── docker-compose.yml       # Ephemeral GitLab CE + Runner for Docker mode
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
└── VERSION                      # Semantic version (1.2.4)
```

## Key Development Patterns

### Adding a New MCP Tool

1. Create `internal/tools/{domain}/` sub-package directory
2. Create `{domain}.go` with typed input/output structs (no domain prefix — package provides namespace)
3. Create `{domain}_test.go` with table-driven tests using `testutil.NewTestClient` and `httptest`
4. Create `register.go` with `RegisterTools(server, client)` — use `mcp.AddTool` with typed `Out` struct to auto-generate `OutputSchema` and `StructuredContent`
5. Wire the sub-package in `internal/tools/register.go` and `register_meta.go`
6. Add markdown formatters in the sub-package `markdown.go` `init()` function using `toolutil.RegisterMarkdown[T]` with appropriate content annotations (`ContentList`, `ContentDetail`, `ContentMutate`)
7. For list formatters: add `toolutil.HintPreserveLinks` as the first hint in `WriteHints()` to instruct the LLM to preserve clickable links
8. Add clickable `[text](url)` links in Markdown table columns where applicable (MRs, issues, pipelines, etc.)
9. Meta-tools automatically get `next_steps` in JSON via `enrichWithHints()` — no extra work needed
10. Update `docs/tools/{domain}.md` and `docs/tools/README.md`
11. Update `docs/development/testing.md` with new test counts and coverage values

See `docs/output-format.md` for the complete response format specification.

### Tool naming convention

`gitlab_{action}_{resource}` in snake_case (e.g., `gitlab_create_issue`, `gitlab_list_projects`)

### Error handling in tool handlers

Three error wrapping functions in `internal/toolutil/errors.go`, used by all 162 domain sub-packages:

- `WrapErr(op, err)` — read-only operations (list, get, search). Generic classification only.
- `WrapErrWithMessage(op, err)` — mutating operations (create, update, delete). Includes GitLab-specific error detail via `ExtractGitLabMessage`.
- `WrapErrWithHint(op, err, hint)` — when a specific corrective action is known (e.g., "use gitlab_branch_unprotect first"). Includes detail + actionable suggestion.
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
go vet ./...                             # Static analysis

# End-to-end tests (requires .env with GITLAB_URL, GITLAB_TOKEN)
go test -v -tags e2e -timeout 300s ./test/e2e/suite/   # Run all e2e tests
make test-e2e                                          # Same via Makefile
make test-e2e-docker                                   # Ephemeral GitLab CE container (Docker, ~4 GB RAM)
go test -tags e2e -c -o NUL ./test/e2e/suite/           # Compile-only check (Windows)
go test -tags e2e -c -o /dev/null ./test/e2e/suite/     # Compile-only check (Linux)
```

### Release process

When creating a new release and uploading binaries to GitHub Releases:

1. Build cross-platform binaries with `make release` (uses GoReleaser locally, flattens `dist/` to match GitHub Release asset names)
2. **Release link names MUST be exact filenames** (e.g. `checksums.txt.asc`, `gitlab-mcp-server-linux-amd64`). Never add descriptive suffixes like `(GPG signature)` — `go-selfupdate` matches asset names exactly and will fail to find files with decorated names

### Post-implementation verification

After making changes, run targeted verification on the **changed files/packages only** (not the entire project):

```bash
# Go files — run on affected packages
go vet ./internal/tools/branches/              # vet on changed package
go test ./internal/tools/branches/ -count=1    # tests on changed package
golangci-lint run ./internal/tools/branches/   # lint on changed package

# Markdown files — run on specific changed files
npx markdownlint-cli2 docs/auto-update.md README.md  # lint specific .md files
npx markdownlint-cli2 --fix docs/auto-update.md      # auto-fix specific .md files

# MCP Inspector (interactive tool testing UI at http://127.0.0.1:6274)
make inspector                             # compile + launch Inspector via stdio
make inspector-stop                        # stop Inspector and clean up

# Full project analysis (use sparingly — for pre-commit or CI)
make analyze                               # all 9 tools, full project
make analyze-fix                           # auto-fix what can be fixed
make analyze-report                        # generate LLM-consumable report
```

**Static analysis tools** (9 total): `goimports`, `gofmt`, `go vet`, `modernize`, `golangci-lint` (v2, 25+ linters), `gosec`, `staticcheck`, `govulncheck`, `markdownlint-cli2`. Configuration: `.golangci.yml`, `.markdownlint-cli2.jsonc`. Full docs: `docs/development/static-analysis.md`.

**Formatting tools**: Before committing, always run `make analyze-fix` to apply `goimports` (import grouping) and `gofmt` (standard formatting). These are the Go equivalents of `clang-format` — all Go code must pass both.

### Environment variables

| Variable                 | Required | Description                                              |
| ------------------------ | -------- | -------------------------------------------------------- |
| `GITLAB_URL`             | Stdio    | GitLab instance URL (e.g., `https://gitlab.example.com`). In HTTP mode, optional via `--gitlab-url` (per-request override via `GITLAB-URL` header) |
| `GITLAB_TOKEN`           | Stdio    | Personal Access Token (`glpat-...`)                      |
| `GITLAB_SKIP_TLS_VERIFY` | No       | Skip TLS verification for self-signed certs (`true`)     |
| `META_TOOLS`             | No       | Enable meta-tools for tool discovery (`true` by default) |
| `GITLAB_READ_ONLY`       | No       | Read-only mode: disables all mutating tools (`false` default) |
| `GITLAB_SAFE_MODE`       | No       | Safe mode: intercepts mutating tools and returns a JSON preview (`false` default) |
| `AUTO_UPDATE`            | No       | Enable auto-update: `true` (default), `check`, `false`  |
| `AUTO_UPDATE_REPO`       | No       | GitHub repository slug for release assets (`jmrplens/gitlab-mcp-server`) |
| `AUTO_UPDATE_INTERVAL`   | No       | Periodic check interval (`1h` default, HTTP mode)        |
| `AUTO_UPDATE_TIMEOUT`    | No       | Pre-start download timeout (`60s` default, range 5s–10m) |
| `GITLAB_ENTERPRISE`      | No       | Enable Enterprise/Premium tools: gates 35 individual tool sub-packages and 15 dedicated meta-tools for GitLab Premium/Ultimate (`false` default) |
| `AUTH_MODE`              | No       | HTTP mode auth: `legacy` (default) or `oauth` (RFC 9728 Bearer verification) |
| `OAUTH_CACHE_TTL`        | No       | OAuth token identity cache TTL (`15m` default, range 1m–2h) |
| `LOG_LEVEL`              | No       | Logging verbosity (`debug`, `info`, `warn`, `error`)     |

In **HTTP mode**, configuration comes from CLI flags instead of environment variables:

| Flag                  | Default | Description                                              |
| --------------------- | ------- | -------------------------------------------------------- |
| `--gitlab-url`        | —       | Default GitLab instance URL (optional; per-request override via `GITLAB-URL` header) |
| `--skip-tls-verify`   | `false` | Skip TLS verification for self-signed certs              |
| `--meta-tools`        | `true`  | Enable meta-tools for tool discovery                     |
| `--enterprise`        | `false` | Enable Enterprise/Premium tools (35 individual + 15 meta-tools) |
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
| `--auto-update-timeout` | `60s` | Pre-start download timeout (range 5s–10m)                |

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
| **Test Expert** | `test-expert.agent.md`  | Writing, analyzing, and improving Go tests. Covers new test development, existing test analysis, coverage analysis to 90%+, false-pass detection, edge case identification, and mandatory test documentation. Uses Context7 for up-to-date Go testing docs. |

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
| **Create MCP Tool**         | `create-mcp-tool/`         | End-to-end workflow for creating a new MCP tool: sub-package, structs, handler, markdown, tests, registration.    |
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

1. Use `@Test Expert` agent — it runs `go test -coverprofile`, identifies gaps, detects false passes, and generates documented tests
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
| ADR-0004 | Modular sub-packages under `internal/tools/{domain}/`          | Accepted (162 sub-packages, 1000 tools)      |
| ADR-0006 | Raw GraphQL.Do() for domains without client-go service wrappers | Accepted (5 GraphQL-only domains)             |
| ADR-0007 | Rich error semantics for LLM-actionable diagnostics            | Accepted (WrapErrWithMessage, WrapErrWithHint) |
| ADR-0009 | Progressive GraphQL migration strategy                         | Accepted (trigger-based REST→GraphQL migration) |

### Modular tools sub-packages (ADR-0004)

The `internal/tools/` package is split into 162 domain sub-packages (161 registered in `internal/tools/register.go` + 1 `serverupdate` registered in `cmd/server/main.go` due to its different constructor signature). Each sub-package has its own `register.go`. This provides:

- Package-level namespace eliminates need for domain prefixes on types (`branches.Output` vs old `BranchOutput`)
- Each sub-package is independently testable with isolated `httptest` mocks
- Zero import cycles — sub-packages import from `toolutil/` only, never from each other
- `internal/tools/register.go` delegates to all sub-package `RegisterTools()` functions
- Validated by `TestAllSubPackagesRegistered` which scans all sub-directories and verifies registration

### Markdown registry pattern

Markdown formatters use a type-based registry in `internal/toolutil/mdregistry.go` instead of a central dispatch switch. Each sub-package self-registers its formatters via `init()` functions:

- `toolutil.RegisterMarkdown[T](fn)` — registers a formatter for output type `T`
- `toolutil.RegisterMarkdownResult[T](fn)` — registers a formatter for `*mcp.CallToolResult` types
- `toolutil.MarkdownForResult(result any)` — looks up and invokes the registered formatter by `reflect.Type`
- `internal/tools/markdown.go` is a thin delegator (~19 lines) that calls `toolutil.MarkdownForResult`
- ~266 formatters across 76 sub-packages, validated by `TestAllMarkdownFormattersRegistered`

### Enterprise tool gating

`GITLAB_ENTERPRISE` controls access to GitLab Premium/Ultimate features in both individual and meta-tool modes:

**Individual mode** (`META_TOOLS=false`) — gates 35 tool sub-package registrations in `register.go`:

- projects (push rules), projectmirrors, mergetrains, auditevents, dorametrics, dependencies, externalstatuschecks, groupscim, memberroles, enterpriseusers, attestations, compliancepolicy, projectaliases, geo, groupstoragemoves, vulnerabilities, securityfindings, securitysettings, groupanalytics, groupcredentials, groupsshcerts, projectiterations, groupiterations, epics, epicissues, epicnotes, epicdiscussions, groupepicboards, groupwikis, groupprotectedbranches, groupprotectedenvs, groupreleases, groupldap, groupsaml, groupserviceaccounts

**Meta-tool mode** (`META_TOOLS=true`, default) — gates 15 dedicated meta-tools in `register_meta.go`:

- gitlab_merge_train, gitlab_audit_event, gitlab_dora_metrics, gitlab_dependency, gitlab_external_status_check, gitlab_group_scim, gitlab_member_role, gitlab_enterprise_user, gitlab_attestation, gitlab_compliance_policy, gitlab_project_alias, gitlab_geo, gitlab_storage_move, gitlab_vulnerability, gitlab_security_finding

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
- **Tool not found**: Check `register.go` and `register_meta.go` for registration
- **Meta-tools disabled**: `META_TOOLS=false` disables discovery tools — set to `true` (default)
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
# Run full E2E suite (two workflows: individual tools + meta-tools)
go test -v -tags e2e -timeout 300s ./test/e2e/suite/
make test-e2e

# Compile-only check (no GitLab needed)
go test -tags e2e -c -o NUL ./test/e2e/suite/       # Windows
go test -tags e2e -c -o /dev/null ./test/e2e/suite/  # Linux
```

**Docker mode** — ephemeral GitLab CE container with CI runner (enables pipeline/job tests):

```bash
docker compose -f test/e2e/docker-compose.yml up -d
./test/e2e/scripts/wait-for-gitlab.sh && ./test/e2e/scripts/setup-gitlab.sh && ./test/e2e/scripts/register-runner.sh
set -a && source test/e2e/.env.docker && set +a
go test -v -tags e2e -timeout 600s ./test/e2e/suite/
docker compose -f test/e2e/docker-compose.yml down -v
```

The suite runs two sequential workflows:

- **TestFullWorkflow** (~174 subtests): exercises all individual tools through a complete project lifecycle (user → project CRUD → commits → branches → tags → releases → issues → labels → milestones → members → upload → MR lifecycle → notes → discussions → search → groups → pipelines → packages → sampling → elicitation → cleanup)
- **TestMetaToolWorkflow** (~151 subtests): exercises the same operations through meta-tools plus 15 additional domains (wikis, CI variables, CI lint, environments, issue links, deploy keys, snippets, issue discussions, draft notes, pipeline schedules, badges, access tokens, award emoji, labels, milestones)

Domains **added in Docker mode** (require CI runner):

- Pipeline create/get/cancel/retry/delete
- Job get/log/retry/cancel

**MCP capability tests** (mock handlers, always available):

- Sampling tools (11 tests): summarize issue, analyze MR, generate release notes, etc.
- Elicitation tools (1 test): confirm destructive action
