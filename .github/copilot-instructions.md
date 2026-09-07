# gitlab-mcp-server — GitLab MCP Server in Go

## Project Overview

This project implements a **Model Context Protocol (MCP) server** that exposes GitLab operations as MCP tools. It is written in **Go** using the official `github.com/modelcontextprotocol/go-sdk` package and communicates with the **GitLab REST API v4** (primary) and **GraphQL API** (for domains without REST coverage — see ADR-0006).

## Architecture

- **Language**: Go 1.27.1
- **MCP SDK**: `github.com/modelcontextprotocol/go-sdk/mcp` v1.7.0
- **GitLab Client**: `gitlab.com/gitlab-org/api/client-go/v2` v2.62.0 (official client, migrated from deprecated `xanzy/go-gitlab`)
- **Transport**: stdio (primary), HTTP (optional)
- **Cross-platform**: Windows, Linux & macOS, amd64 & arm64

## Project Structure

```text
gitlab-mcp-server/
├── cmd/                    # server + 31 dev utility binaries — see docs/development/cmd-utilities.md for the full reference
│   ├── server/             # MCP server entry point (+ --shutdown, --probe flags)
│   ├── audit_1to1/         # 1:1 SDK↔API parity audit (-scope structs|actions|metadata|enums|sdk; -validate-docs)
│   ├── audit_catalog_first/        # Catalog-first registration invariants (ADR-0004)
│   ├── audit_discovery_completeness/ # Discovery-metadata quality audit (META-001)
│   ├── audit_doc_coverage/ # docs/reference/tools/*.md vs catalog coverage gaps (DOC-002)
│   ├── audit_doc_tool_names/ # Every `gitlab_*` name the docs mention exists on the surface it claims (make check-doc-tool-names)
│   ├── audit_dynamic_aliases/ # Dynamic-toolset alias governance
│   ├── audit_e2e_gaps/     # Catalog actions the e2e suite never exercises
│   ├── audit_edition_tier/ # Doc-grounded licensing tier vs binary gating
│   ├── audit_gateway_chars/ # Served descriptions/titles vs strict gateway validators (make check-gateway-chars)
│   ├── audit_install_buttons/ # One-click install payloads decode to one configuration per command
│   ├── audit_surface_quality/ # MCP surface metadata + output quality (-view; was audit_tools + audit_output)
│   ├── audit_supply_chain/ # Release-configuration invariants (SHA-pinned uses:, Dependabot cooldowns, SECURITY.md, installers)
│   ├── audit_tokens/       # Token overhead; -footprint, --compare-schemas (was audit_meta_schema); cl100k_base tokenizer (tokens.go)
│   ├── audit_metrics/      # MCP tool/resource/prompt metrics
│   ├── audit_readonly_graphql/ # No ReadOnly action may reach a GraphQL mutation (make check-readonly-graphql)
│   ├── audit_test_goroutines/ # Off-goroutine testing.T abort audit (--check gate)
│   ├── audit_test_names/   # Test naming convention (+ -apply/-dry-run; -check-files gates test-file naming)
│   ├── audit_test_subtests/ # Case loops that assert without a t.Run subtest (-fix rewrites the unambiguous ones)
│   ├── audit_string_dupes/ # Duplicated string literals missing constants
│   ├── bench_resources/    # What the server costs to run, and the charts the docs publish
│   ├── godoc_tool/         # Godoc auditor + fixer (audit/fix; was audit_godocs + add_docs)
│   ├── format_md_tables/   # Markdown pipe-table normalizer
│   ├── gen_action_catalog_manifest/ # ActionSpec group-builder manifest
│   ├── gen_brand/          # Every vector brand asset from one parametric geometry
│   ├── gen_docker_tools/   # Docker MCP Registry tools.json
│   ├── gen_icon_webp/      # Light/dark WebP icon fallbacks from icons.go
│   ├── gen_lhm_manifest/   # Capability arrays in lhm.plugin.json (LobeHub)
│   ├── gen_llms/           # llms.txt / llms-full.txt
│   ├── gen_stats/          # README repository-statistics section (was inside gen_readme)
│   ├── gen_testing_docs/   # docs/development/testing/testing.md test-metrics block
│   ├── eval_mcp_surfaces/  # Model-behavior evaluation across MCP surfaces
│   └── internal/           # Helpers shared by the commands (apidocs, auditshared, docgen, mcpsurface)
├── internal/
│   ├── config/             # Configuration loading (dotenv files, flags, env vars, HTTP env overlay)
│   ├── edition/            # Licensing tier model (Free/Premium/Ultimate)
│   ├── gitlab/             # GitLab API client wrapper
│   ├── oauth/              # OAuth HTTP mode: token cache, GitLab verifier, RFC 9728 metadata
│   ├── serverpool/         # HTTP mode: per-token+URL server pool & LRU cache
│   ├── subscriptions/      # resources/subscribe: polled watchers, leases (ADR-0015)
│   ├── telemetry/, mcpotel/ # OpenTelemetry export and MCP instrumentation
│   ├── clientcompat/, gatewaycompat/, cachehints/, capguard/ # Per-client and gateway compatibility, cache hints, capability guard
│   ├── toolutil/           # Shared tool utilities (errors, pagination, markdown, logging)
│   ├── testutil/           # Shared test helpers (NewTestClient, RespondJSON)
│   ├── tools/              # Tool orchestration layer + 177 internal/tools packages
│   │   ├── action_catalog.go # Canonical action catalog built from domain ActionSpecs
│   │   ├── register.go     # RegisterAll() — projects individual tools from the canonical action catalog
│   │   ├── register_meta.go # RegisterAllMeta() — registers catalog-backed meta groups and standalone surfaces
│   │   ├── dynamic/        # Low-token dynamic find/execute surface
│   │   ├── dynamiccatalog/ # Build(): the dynamic catalog assembled the way the server assembles it
│   │   ├── branches/       # Branch & protected branch tools
│   │   ├── commits/        # Commit tools
│   │   ├── issues/         # Issue CRUD tools
│   │   ├── mergerequests/  # Merge request CRUD tools
│   │   ├── projects/       # Project CRUD tools
│   │   └── ...             # 177 internal/tools packages total
│   ├── resources/          # MCP resource implementations
│   ├── prompts/            # MCP prompt implementations
│   ├── completions/        # Argument completion handler
│   ├── progress/           # MCP progress notifications
│   └── elicitation/        # MCP elicitation capability
├── docs/                   # Documentation (Diátaxis: guides/, reference/, concepts/, development/)
│   ├── guides/             # installation, ide-configuration.md, oauth-app-setup.md, http-server-mode, remote-deployment, telemetry
│   ├── reference/          # cli, configuration, env, output format, tools/ (per domain), resources, prompts, capabilities/
│   ├── concepts/           # architecture, dynamic tools, meta-tools, error handling, GraphQL, security
│   └── development/        # adr/ (Architectural Decision Records), testing/ (generated), static analysis, cmd utilities
├── plan/                   # Implementation plans
├── .github/                # Copilot agents, skills, instructions
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
- Dynamic mode (`GITLAB_MCP_TOOL_SURFACE=dynamic`) exposes `gitlab_find_action` and `gitlab_execute_action` over the canonical action catalog shared with meta-tools. It is the default tool surface; set `GITLAB_MCP_TOOL_SURFACE=meta` for consolidated domain meta-tools.
- When adding GitLab actions, add or update domain-local `ActionSpecs` and the generated/audited catalog manifest. Meta-tools, dynamic find/execute, `gitlab://tools` resources, LLM files, and individual tool projection consume that catalog. Do not add package-local `RegisterTools` functions for ordinary GitLab API actions.
- For the detailed developer architecture of individual tools, meta-tools, dynamic mode, and the canonical action core, see `docs/development/tool-surfaces-and-action-core.md`.

### GitLab Integration

- Stdio mode uses `GITLAB_URL`; HTTP mode requires `--gitlab-url` (one instance fixes it, several publish an allow-list the `GITLAB-URL` header selects from) unless `--allow-any-gitlab-url` is passed, which lets the header name any host and is meant for single-user local deployments only
- Authentication via `GITLAB_TOKEN` (Personal Access Token); the token is read from the environment or a dotenv file (`~/.gitlab-mcp-server.env`, or the file `GITLAB_MCP_ENV_FILE` names), never from a working-directory `.env` and never from a flag
- Self-signed TLS certificates: skip verification when `GITLAB_MCP_SKIP_TLS_VERIFY=true`
- All API calls must respect `context.Context` for cancellation
- Rate limiting awareness and retry logic

### Testing

- Unit tests for every tool handler
- Use `httptest` for mocking GitLab API responses in unit tests
- Test naming: `TestToolName_Scenario_ExpectedResult`
- Aim for >80% coverage on tool handlers
- **After completing a test-focused phase or milestone, run `go run ./cmd/gen_testing_docs/` or `make gen-testing-docs`** to refresh `docs/development/testing/testing.md`, then verify with `go run ./cmd/gen_testing_docs/ --check`

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
export E2E_BITBUCKET_ADMIN_PASSWORD=$(openssl rand -hex 16)
docker compose -f test/e2e/docker-compose.yml --profile bitbucket up -d
./test/e2e/scripts/wait-for-gitlab.sh && ./test/e2e/scripts/setup-gitlab.sh && ./test/e2e/scripts/register-runner.sh && ./test/e2e/scripts/setup-bitbucket.sh
set -a && source test/e2e/.env.docker && set +a
go test -v -tags e2e -timeout 600s ./test/e2e/suite/
docker compose -f test/e2e/docker-compose.yml --profile bitbucket down -v

# Or via Makefile
make test-e2e-docker

# Compile-only check (no GitLab needed)
go test -tags e2e -c -o NUL ./test/e2e/suite/       # Windows
go test -tags e2e -c -o /dev/null ./test/e2e/suite/  # Linux
```

- Requires `GITLAB_URL` and `GITLAB_TOKEN` in the environment (user needs create/delete project permissions)
- One test file per domain (172 files), in three families named by the surface they drive: `TestIndividual_*` (individual tools), `TestMeta_*` (meta-tools; `TestEE_*` for the Enterprise-only domains on an EE runtime) and `TestDynamicToolSurface_*`
- Dynamic surface coverage lives in `TestDynamicToolSurface_*` and validates the default two-tool find/execute workflow against the same E2E GitLab fixture. To run only that family in Docker mode, run `set -a && source test/e2e/.env.docker && set +a` after the Docker GitLab setup scripts complete, then use `E2E_MODE=docker go test -v -tags e2e -timeout 600s -run '^TestDynamicToolSurface_' ./test/e2e/suite/`.
- Covers: user, project CRUD, commits, branches, tags, releases, issues, labels, milestones, members, upload, MR lifecycle, notes, discussions, search, groups, pipelines, packages, wikis, CI variables, environments, issue links, deploy keys, snippets, pipeline schedules, badges, access tokens, award emoji, elicitation
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
2. **Release link names MUST be exact filenames** (e.g. `checksums.txt.sigstore.json`, `gitlab-mcp-server-linux-amd64`). Never add descriptive suffixes like `(GPG signature)` — the Homebrew formula, winget, the installers and `scripts/fetch-release-assets.sh` look assets up by exact name and will not find a decorated one
3. The full chain (draft-then-publish, `.mcpb` bundle, npm, PyPI and NuGet trusted publishers, Homebrew tap, winget, `server.json` stamping, and how to rehearse it with `gh workflow run release.yml --ref <branch>`) is documented under "Release process" in `CLAUDE.md`

### Git Workflow

- Use conventional commits: `feat:`, `fix:`, `docs:`, `test:`, `refactor:`, `chore:`
- Develop on feature branches: `feature/tool-name`, `fix/description`
- Main branch protected, merge via pull requests

## Key Environment Variables

| Variable                 | Description                       | Example            |
| ------------------------ | --------------------------------- | ------------------ |
| `GITLAB_URL`             | GitLab instance URL. In HTTP mode it is `--gitlab-url`, required unless `--allow-any-gitlab-url` is passed; one value fixes the instance, several publish an allow-list the `GITLAB-URL` header must select from | `https://gitlab.example.com` |
| `GITLAB_TOKEN`           | Personal Access Token (stdio mode) | `glpat-...`        |
| `GITLAB_MCP_SKIP_TLS_VERIFY` | Skip TLS certificate verification | `true`             |
| `GITLAB_MCP_META_TOOLS`             | Deprecated compatibility selector; prefer `GITLAB_MCP_TOOL_SURFACE` for new configs | _(unset)_          |
| `GITLAB_MCP_TOOL_SURFACE`           | Explicit tool catalog selector: `dynamic`, `meta`, or `individual`; overrides legacy `GITLAB_MCP_META_TOOLS` | `dynamic` (default when unset) |
| `GITLAB_MCP_CAPABILITY_SURFACE`     | Resource and prompt catalog selector: `full` or `minimal`; pair `minimal` with dynamic experiments when startup context must be tiny | `full` (default)   |
| `GITLAB_MCP_META_PARAM_SCHEMA`      | Meta-tool input-schema strategy: `opaque` (default), `compact` (~8.1x), or `full` (~18.0x). Independent of `GITLAB_MCP_META_TOOLS`. Per-action call shapes and input schemas are discoverable through `gitlab://tools` and `gitlab://tools/{id}` for every surface | `opaque` (default) |
| `GITLAB_MCP_READ_ONLY`       | Read-only mode: removes mutating operations per action; reads keep working on every surface | `false` (default)  |
| `GITLAB_MCP_SAFE_MODE`       | Safe mode: intercepts mutating operations per action and returns a JSON preview | `false` (default)  |
| `GITLAB_ENTERPRISE`      | **Deprecated** — use `GITLAB_MCP_TIER`. Honored for back-compat only when `GITLAB_MCP_TIER` is unset (`true` → `ultimate`, `false` → `free`); logs a deprecation warning | `false` (default) |
| `GITLAB_MCP_TIER`            | Licensing tier selector: `free`/`ce`, `premium`, or `ultimate`. When set, used verbatim; when unset, detected from `GET /license` (fallback `free`). Tier gates Enterprise/Premium tools AND per-field schema pruning (see `pruneSchemaFieldsByTier` in `internal/tools/action_catalog.go`) | `free` (default)   |
| `EVAL_SURFACE_ENTERPRISE` | `cmd/eval_mcp_surfaces`: run the enterprise case set on top of the base corpus | `false` (default)  |
| `EVAL_SURFACE_CASE_SET`   | `cmd/eval_mcp_surfaces`: case-set selector — `ce` (CE only), `all` (CE+Enterprise) | `ce` (default)     |
| `EVAL_SURFACE_FIXTURE_SMOKE` | `cmd/eval_mcp_surfaces`: limit the run to fixture-smoke cases (fast smoke check) | `false` (default) |
| `--max-output-retries`   | `cmd/eval_mcp_surfaces`: re-runs a task when it fails solely due to malformed model tool-call output | `2` (default)      |
| `GITLAB_MCP_MAX_HTTP_CLIENTS`       | Maximum unique (token, GitLab URL) server entries kept in the HTTP pool; bounds pooled entries, not sessions (also `--max-http-clients` flag) | `100` (default)    |
| `GITLAB_MCP_SESSION_TIMEOUT` | Idle MCP session timeout, HTTP mode with `--stateless=false` only (also `--session-timeout` flag) | `30m` (default)  |
| `GITLAB_MCP_ACTION_TIMEOUT`         | Cancel an action still running after this long, both transports (also `--action-timeout` in HTTP mode; `0` disables, max 24h). Above the longest wait any action offers | `65m`            |
| `GITLAB_MCP_DRAIN_DELAY`            | HTTP mode: after `SIGTERM`, answer `/health` with `503 draining` for this long before closing the listener, so a polling balancer removes the instance first (also `--drain-delay`; max 5m) | `0`              |
| `GITLAB_MCP_RATE_LIMIT_RPS`         | Per-credential rate limit, in req/s, on every call that reaches GitLab (`tools/call`, `resources/read`, `resources/subscribe`, `subscriptions/listen`, `prompts/get`), plus `tools/list` on a bucket of its own refilled a tenth as fast, charged for spending the processor every tenant shares rather than for reaching GitLab; `0` disables it. Both transports: `0` in stdio, `10` in HTTP mode, where `--rate-limit-rps` overrides it | `0` / `10` (HTTP) |
| `GITLAB_MCP_RATE_LIMIT_BURST`       | Token-bucket burst size when RPS > 0 (also `--rate-limit-burst` flag) | `40` (default)   |
| `GITLAB_MCP_AUTH_MODE`              | HTTP mode auth: `legacy` (default) or `oauth` (RFC 9728 Bearer verification) | `legacy` (default) |
| `GITLAB_MCP_OAUTH_CACHE_TTL`        | OAuth token identity cache TTL (also `--oauth-cache-ttl` flag) | `15m` (default)  |

This table is the subset an assistant meets most often. Every variable, its bounds and its flag are tabulated under "Environment variables" in `CLAUDE.md`; `docs/reference/env.md` and `docs/reference/cli.md` are the user-facing references. In HTTP mode an explicitly passed flag wins over the environment variable, which wins over the default (`internal/config/http_overlay.go`).

**HTTP-only flags** (no environment variable equivalent; two of many, the rest are in `CLAUDE.md`):

| Flag                       | Description                                                    | Default            |
| -------------------------- | -------------------------------------------------------------- | ------------------ |
| `--trusted-proxies`        | Addresses or CIDR ranges of the proxies whose `--trusted-proxy-header` is believed (e.g. `127.0.0.1,10.0.0.0/8`); required with it | _(empty)_          |
| `--trusted-proxy-header`   | HTTP header with real client IP for rate limiting behind proxies (e.g. `CF-Connecting-IP`, `X-Forwarded-For`); believed only from `--trusted-proxies` | _(empty)_          |

**General flags** (both stdio and HTTP modes):

| Flag           | Default | Description                                                    |
| -------------- | ------- | -------------------------------------------------------------- |
| `--shutdown`   | `false` | Terminate all running instances of this binary and exit. Used by external updaters (pe-agnostic-store) before replacing the binary on disk. |
| `--probe`      | `false` | Ask the running instance's `/health` and exit 0 when it answers; the image's `HEALTHCHECK`. Reads the listener off the instance's own flags, or probes the URL, `unix:<path>` or `host:port` given after the flag. |

## AI Assistance Infrastructure

This project includes 7 agents, 19 skills, and 8 instruction files in `.github/` for AI-assisted development. See `CLAUDE.md` at the project root for a comprehensive catalog of all agents, skills, workflows, and when to use each one.

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

- **Tests with HTTP mocks or goroutines**: never `t.Fatal` inside handler
  literals or `go` statements — follow
  `.github/instructions/test-goroutines.instructions.md`.
