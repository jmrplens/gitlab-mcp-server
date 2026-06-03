# gitlab-mcp-server — GitLab MCP Server in Go

## Project Overview

This project implements a **Model Context Protocol (MCP) server** that exposes GitLab operations as MCP tools. It is written in **Go** using the official `github.com/modelcontextprotocol/go-sdk` package and communicates with the **GitLab REST API v4** (primary) and **GraphQL API** (for domains without REST coverage — see ADR-0006).

## Architecture

- **Language**: Go 1.26.4
- **MCP SDK**: `github.com/modelcontextprotocol/go-sdk/mcp` v1.6.0
- **GitLab Client**: `gitlab.com/gitlab-org/api/client-go/v2` v2.29.0 (official client, migrated from deprecated `xanzy/go-gitlab`)
- **Transport**: stdio (primary), HTTP (optional)
- **Cross-platform**: Windows, Linux & macOS, amd64 & arm64

## Project Structure

```text
gitlab-mcp-server/
├── cmd/                    # Entry points and dev utilities
│   ├── server/
│   │   └── main.go         # MCP server entry point
│   │   └── shutdown.go     # --shutdown flag: terminate all running instances
│   ├── add_docs/
│   │   └── main.go         # AST tool: adds godoc comments to undocumented symbols
│   ├── audit_tools/
│   │   └── main.go         # Audits MCP tool metadata violations
│   ├── audit_output/
│   │   └── main.go         # Audits MCP tool output quality (OutputSchema, annotations)
│   ├── audit_metrics/
│   │   └── main.go         # Audits MCP tool metrics (tool count, resource count, etc.)
│   ├── audit_test_names/
│   │   └── main.go         # Audits test function naming convention compliance
│   ├── format_md_tables/
│   │   └── main.go         # Formats Markdown pipe tables in README.md and docs/
│   ├── gen_llms/
│   │   └── main.go         # Generates llms.txt and llms-full.txt for LLM discovery
│   └── find_dupes/
│       └── main.go         # Finds duplicated string literals missing constants
├── internal/
│   ├── config/             # Configuration loading (.env, flags)
│   ├── gitlab/             # GitLab API client wrapper
│   ├── serverpool/         # HTTP mode: per-token+URL server pool & LRU cache
│   ├── toolutil/           # Shared tool utilities (errors, pagination, markdown, logging)
│   ├── testutil/           # Shared test helpers (NewTestClient, RespondJSON)
│   ├── tools/              # Tool orchestration layer + 176 internal/tools packages
│   │   ├── register.go     # RegisterAll() — projects individual tools from the canonical action catalog
│   │   ├── register_meta.go # RegisterAllMeta() — registers catalog-backed meta groups and standalone surfaces
│   │   ├── dynamic/        # Low-token dynamic find/execute surface
│   │   ├── branches/       # Branch & protected branch tools
│   │   ├── commits/        # Commit tools
│   │   ├── issues/         # Issue CRUD tools
│   │   ├── mergerequests/  # Merge request CRUD tools
│   │   ├── projects/       # Project CRUD tools
│   │   └── ...             # 176 internal/tools packages total
│   ├── resources/          # MCP resource implementations
│   └── prompts/            # MCP prompt implementations
├── docs/                   # Documentation, ADRs, specs
│   ├── adr/
│   ├── spec/
│   ├── oauth-app-setup.md  # Creating GitLab OAuth applications for MCP clients
│   └── ide-configuration.md # Per-IDE MCP JSON configuration (stdio, HTTP legacy, OAuth)
├── plan/                   # Implementation plans
├── .github/                # Copilot agents, skills, instructions
├── .env                    # Local dev secrets (gitignored)
├── .gitignore
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

## Development Conventions

### Go Standards

- Follow idiomatic Go and the repository's consolidated `golangci-lint` configuration (`goimports`, `gofumpt`, `gci`, `govet`, `staticcheck`, `gosec`, and related checks)
- Prefer standard library over third-party when equivalent
- All exported types and functions must have doc comments
- Error wrapping with `fmt.Errorf("context: %w", err)`
- Use `context.Context` consistently for cancellation/timeouts
- Table-driven tests with `t.Run()` subtests

### MCP Patterns

- Each GitLab operation is defined once as a typed `ActionSpec` and projected into meta, dynamic, `gitlab://tools`, and individual surfaces
- Use `jsonschema` struct tags for tool input documentation
- Register runtime surfaces from the canonical action catalog only; ordinary GitLab actions must not add package-local `RegisterTools` functions or package-level meta registration paths
- Resources for read-only data (project info, user info, etc.)
- Graceful shutdown via signal handling
- Dynamic mode (`TOOL_SURFACE=dynamic`) exposes `gitlab_find_action` and `gitlab_execute_action` over the canonical action catalog shared with meta-tools. It is the default tool surface; set `TOOL_SURFACE=meta` for consolidated domain meta-tools.
- When adding GitLab actions, add or update domain-local `ActionSpecs` and the generated/audited catalog manifest. Meta-tools, dynamic find/execute, `gitlab://tools` resources, LLM files, and individual tool projection consume that catalog. Do not add package-local `RegisterTools` functions for ordinary GitLab API actions.
- For the detailed developer architecture of individual tools, meta-tools, dynamic mode, and the canonical action core, see `docs/development/tool-surfaces-and-action-core.md`.

### GitLab Integration

- Stdio mode uses `GITLAB_URL`; HTTP mode uses `--gitlab-url` when fixed, or per-request `GITLAB-URL` headers when omitted
- Authentication via `GITLAB_TOKEN` (Personal Access Token)
- Self-signed TLS certificates: skip verification when `GITLAB_SKIP_TLS_VERIFY=true`
- All API calls must respect `context.Context` for cancellation
- Rate limiting awareness and retry logic

### Testing

- Unit tests for every tool handler
- Use `httptest` for mocking GitLab API responses in unit tests
- Test naming: `TestToolName_Scenario_ExpectedResult`
- Aim for >80% coverage on tool handlers
- **After completing a test-focused phase or milestone, run `go run ./cmd/gen_testing_docs/` or `make gen-testing-docs`** to refresh `docs/testing/testing.md`, then verify with `go run ./cmd/gen_testing_docs/ --check`

### Verification After Changes

After implementing changes, run targeted analysis on the **changed files/packages only**:

```bash
# Go files — run on affected packages (replace path with changed package)
go test ./internal/tools/{domain}/ -count=1
golangci-lint run --build-tags e2e ./internal/tools/{domain}/

# Markdown files — run on specific changed .md files
npx markdownlint-cli2 path/to/changed.md

# README.md/docs tables — normalize pipe tables, or verify with --check
go run ./cmd/format_md_tables/
go run ./cmd/format_md_tables/ --check
```

- 3 analysis gates available: `golangci-lint` (v2; includes Go linters and formatters such as `goimports`, `gofumpt`, `gci`, `govet`, `modernize`, `gosec`, and `staticcheck`), `govulncheck`, and `markdownlint-cli2`
- Configuration: `.golangci.yml` (Go linters/formatters), `.markdownlint-cli2.jsonc` (Markdown rules)
- Markdown table formatting: when creating or editing pipe tables in `README.md` or `docs/`, use `go run ./cmd/format_md_tables/` to normalize column padding and alignment markers, then verify with `go run ./cmd/format_md_tables/ --check`
- Formatting: always run `make analyze-fix` before committing to apply configured Go formatters (`goimports`, `gofumpt`, `gci`) and Markdown fixes
- Full project: `make analyze` (all analysis gates), `make analyze-fix` (auto-fix), `make analyze-report` (LLM report)
- See `docs/development/static-analysis.md` for full documentation

### End-to-End Tests

E2E tests run against a real GitLab instance via in-memory MCP transport (build tag `e2e`):

```bash
# Run full E2E suite
go test -v -tags e2e -timeout 300s ./test/e2e/suite/
make test-e2e

# Docker mode (ephemeral GitLab CE with CI runner and fixture service)
docker compose -f test/e2e/docker-compose.yml up -d
./test/e2e/scripts/wait-for-gitlab.sh && ./test/e2e/scripts/setup-gitlab.sh && ./test/e2e/scripts/register-runner.sh
set -a && source test/e2e/.env.docker && set +a
go test -v -tags e2e -timeout 600s ./test/e2e/suite/
docker compose -f test/e2e/docker-compose.yml down -v

# Or via Makefile
make test-e2e-docker

# Compile-only check (no GitLab needed)
go test -tags e2e -c -o NUL ./test/e2e/suite/       # Windows
go test -tags e2e -c -o /dev/null ./test/e2e/suite/  # Linux
```

- Requires `.env` with `GITLAB_URL`, `GITLAB_TOKEN` (user needs create/delete project permissions)
- Two sequential workflows: `TestFullWorkflow` (~174 subtests, individual tools) and `TestMetaToolWorkflow` (~151 subtests, meta-tools)
- Dynamic surface coverage lives in `TestDynamicToolSurface_*` and validates the default two-tool find/execute workflow against the same E2E GitLab fixture. To run only that workflow in Docker mode, run `set -a && source test/e2e/.env.docker && set +a` after the Docker GitLab setup scripts complete, then use `E2E_MODE=docker go test -v -tags e2e -timeout 600s -run '^TestDynamicToolSurface_' ./test/e2e/suite/`.
- Covers: user, project CRUD, commits, branches, tags, releases, issues, labels, milestones, members, upload, MR lifecycle, notes, discussions, search, groups, pipelines, packages, wikis, CI variables, environments, issue links, deploy keys, snippets, pipeline schedules, badges, access tokens, award emoji, sampling, elicitation
- Docker mode also writes `E2E_FIXTURE_URL` and `E2E_GITLAB_INTERNAL_URL` for deterministic webhook, custom emoji, and push mirror tests without public Internet dependencies
- Not covered (needs Docker mode): pipeline CRUD (CI runner), job tools

### Surface Evaluator (Docker)

Use these Makefile targets for model-backed surface evaluation with the Docker GitLab fixture:

```bash
# CE case set
make eval-surfaces-docker SURFACE=dynamic
make eval-surfaces-docker SURFACE=meta

# Enterprise case set on GitLab EE runtime
make eval-surfaces-docker-enterprise SURFACE=dynamic
make eval-surfaces-docker-enterprise SURFACE=meta

# CE + Enterprise case set together on GitLab EE runtime
make eval-surfaces-docker-enterprise-all SURFACE=dynamic
make eval-surfaces-docker-enterprise-all SURFACE=meta
```

- `SURFACE` must be `dynamic` or `meta`.
- Add `PRESET=...` to run a single Docker preset.
- `eval-surfaces-docker-enterprise-all` sets `EVAL_SURFACE_CASE_SET=all` and is the standard full validation command for CE+Enterprise regression checks.

### Build & Cross-Compilation

```bash
# Build for current platform
go build -o dist/gitlab-mcp-server ./cmd/server

# Cross-compile all targets
GOOS=linux GOARCH=amd64 go build -o dist/gitlab-mcp-server-linux-amd64 ./cmd/server
GOOS=linux GOARCH=arm64 go build -o dist/gitlab-mcp-server-linux-arm64 ./cmd/server
GOOS=windows GOARCH=amd64 go build -o dist/gitlab-mcp-server-windows-amd64.exe ./cmd/server
GOOS=windows GOARCH=arm64 go build -o dist/gitlab-mcp-server-windows-arm64.exe ./cmd/server
GOOS=darwin GOARCH=amd64 go build -o dist/gitlab-mcp-server-darwin-amd64 ./cmd/server
GOOS=darwin GOARCH=arm64 go build -o dist/gitlab-mcp-server-darwin-arm64 ./cmd/server
```

### Release Process

When creating a new release and uploading binaries to GitHub Releases:

1. Build cross-platform binaries with `make release` (uses GoReleaser locally, flattens `dist/` to match GitHub Release asset names)
2. **Release link names MUST be exact filenames** (e.g. `checksums.txt.asc`, `gitlab-mcp-server-linux-amd64`). Never add descriptive suffixes like `(GPG signature)` — `go-selfupdate` matches asset names exactly and will fail to find files with decorated names

### Git Workflow

- Use conventional commits: `feat:`, `fix:`, `docs:`, `test:`, `refactor:`, `chore:`
- Develop on feature branches: `feature/tool-name`, `fix/description`
- Main branch protected, merge via pull requests

## Key Environment Variables

| Variable                 | Description                       | Example            |
| ------------------------ | --------------------------------- | ------------------ |
| `GITLAB_URL`             | GitLab instance URL. In HTTP mode, optional via `--gitlab-url`; when set it fixes the GitLab instance, and when omitted clients must send `GITLAB-URL` per request | `https://gitlab.example.com` |
| `GITLAB_TOKEN`           | Personal Access Token (stdio mode) | `glpat-...`        |
| `GITLAB_SKIP_TLS_VERIFY` | Skip TLS certificate verification | `true`             |
| `META_TOOLS`             | Deprecated compatibility selector; prefer `TOOL_SURFACE` for new configs | _(unset)_          |
| `TOOL_SURFACE`           | Explicit tool catalog selector: `dynamic`, `meta`, or `individual`; overrides legacy `META_TOOLS` | `dynamic` (default when unset) |
| `CAPABILITY_SURFACE`     | Resource and prompt catalog selector: `full` or `minimal`; pair `minimal` with dynamic experiments when startup context must be tiny | `full` (default)   |
| `META_PARAM_SCHEMA`      | Meta-tool input-schema strategy: `opaque` (default), `compact` (~5x), or `full` (~10x). Independent of `META_TOOLS`. Per-action call shapes and input schemas are discoverable through `gitlab://tools` and `gitlab://tools/{id}` for every surface | `opaque` (default) |
| `GITLAB_READ_ONLY`       | Read-only mode: disables all mutating tools | `false` (default)  |
| `GITLAB_SAFE_MODE`       | Safe mode: intercepts mutating tools and returns a JSON preview | `false` (default)  |
| `AUTO_UPDATE`            | Enable auto-update: `true` (default), `check`, `false` | `true` (default)   |
| `AUTO_UPDATE_REPO`       | GitHub repository slug for release assets (owner/repo) | `jmrplens/gitlab-mcp-server` |
| `AUTO_UPDATE_INTERVAL`   | Periodic check interval, HTTP mode | `1h` (default)     |
| `AUTO_UPDATE_TIMEOUT`    | Startup/background update timeout (range 5s–10m) | `60s` (default)    |
| `GITLAB_ENTERPRISE`      | Enable Enterprise/Premium tools in stdio mode. In HTTP mode, `--enterprise` explicitly forces the Enterprise/Premium catalog; when omitted, CE/EE is auto-detected per token+URL pool entry when GitLab reports edition | `false` (default) |
| `MAX_HTTP_CLIENTS`       | Max client sessions, HTTP mode (also `--max-http-clients` flag) | `100` (default)    |
| `SESSION_TIMEOUT`        | Idle session timeout, HTTP mode (also `--session-timeout` flag) | `30m` (default)  |
| `RATE_LIMIT_RPS`         | Per-server tools/call rate limit in req/s (also `--rate-limit-rps` flag; `0` = disabled) | `0` (default)    |
| `RATE_LIMIT_BURST`       | Token-bucket burst size when RPS > 0 (also `--rate-limit-burst` flag) | `40` (default)   |
| `AUTH_MODE`              | HTTP mode auth: `legacy` (default) or `oauth` (RFC 9728 Bearer verification) | `legacy` (default) |
| `OAUTH_CACHE_TTL`        | OAuth token identity cache TTL (also `--oauth-cache-ttl` flag) | `15m` (default)  |

**HTTP-only flags** (no environment variable equivalent):

| Flag                       | Description                                                    | Default            |
| -------------------------- | -------------------------------------------------------------- | ------------------ |
| `--trusted-proxy-header`   | HTTP header with real client IP for rate limiting behind proxies (e.g. `Fly-Client-IP`, `X-Forwarded-For`) | _(empty)_          |

**General flags** (both stdio and HTTP modes):

| Flag           | Default | Description                                                    |
| -------------- | ------- | -------------------------------------------------------------- |
| `--shutdown`   | `false` | Terminate all running instances of this binary and exit. Used by external updaters (pe-agnostic-store) before replacing the binary on disk. |

## AI Assistance Infrastructure

This project includes 7 agents, 18 skills, and 7 instruction files in `.github/` for AI-assisted development. See `CLAUDE.md` at the project root for a comprehensive catalog of all agents, skills, workflows, and when to use each one.

Key agents: `go-mcp-expert` (primary coding), `test-expert` (testing, coverage, false-pass detection), `plan-expert` (strategic planning), `debug` (debugging), `se-reviewer` (OWASP + architecture), `documentation-writer` (project docs with Context7 + web research).

## Language Policy

> **All project artifacts must be written in English without exception.**

| Artifact                                     | Language |
| -------------------------------------------- | -------- |
| Source code (all `.go` files)                | English  |
| Comments and doc comments                    | English  |
| Commit messages                              | English  |
| Documentation (`README`, `docs/`, `plan/`)   | English  |
| MCP tool names, descriptions, error messages | English  |
| Test names and assertions                    | English  |
| ADRs, specs, instructions                    | English  |
| Git branch names                             | English  |

Conversations with the developer may be in any language, but **every file committed to this repository must be in English**.
