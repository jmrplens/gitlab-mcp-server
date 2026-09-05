# Testing

> **Diátaxis type**: Reference
> **Audience**: 🔧 Developers, contributors
> **Prerequisites**: Go testing basics, understanding of httptest
>
> Comprehensive test documentation for gitlab-mcp-server.
>
> **Maintenance Rule**: Whenever tests are added, modified, or removed, run `go run ./cmd/gen_testing_docs/` to refresh the generated counts and coverage values.

---

<!-- START TESTING STATS -->
@@GENERATED@@
<!-- END TESTING STATS -->

## Test Types

### Unit Tests

All unit tests use `httptest` to mock GitLab API responses. No real GitLab API calls are made during unit testing.

**Patterns used:**

- **Table-driven tests** with `t.Run()` subtests — standard across all packages
- **Mock server**: `testutil.NewTestClient()` creates a GitLab client pointing to a local `httptest.Server`
- **JSON responses**: `testutil.RespondJSON()` and `testutil.RespondJSONWithPagination()` helpers
- **Naming convention**: `TestToolName_Scenario_ExpectedResult`

**Example structure:**

```go
func TestGetBranch_Success(t *testing.T) {
    client, mux, cleanup := testutil.NewTestClient()
    defer cleanup()

    mux.HandleFunc("/api/v4/projects/1/repository/branches/main", func(w http.ResponseWriter, r *http.Request) {
        testutil.RespondJSON(w, gitlab.Branch{Name: "main"})
    })

    // ... invoke tool handler, assert result
}
```

### End-to-End Tests

E2E tests run against a **real GitLab instance** using in-memory MCP transport (build tag `e2e`). Two modes are supported:

#### Self-Hosted Mode

Requires a running GitLab instance with credentials in `.env`:

```bash
# .env
GITLAB_URL=https://gitlab.example.com
GITLAB_TOKEN=glpat-...
```

```bash
go test -v -tags e2e -timeout 300s ./test/e2e/suite/
make test-e2e
```

#### Docker Mode

Uses an ephemeral GitLab CE container provisioned by Docker Compose. Requires Docker and ~4 GB RAM. Enterprise mode uses the same topology with a GitLab EE image plus a locally supplied Ultimate license.

All E2E Docker infrastructure is version-controlled under `test/e2e/`:

- `test/e2e/docker-compose.yml` — GitLab CE/EE + Runner + fixture service compose definition
- `test/e2e/scripts/setup-gitlab.sh` — Creates test user, PAT, installs `ENTERPRISE_LICENSE` when requested, writes `.env.docker`
- `test/e2e/scripts/register-runner.sh` — Registers CI runner in GitLab
- `test/e2e/scripts/wait-for-gitlab.sh` — Polls GitLab readiness endpoint

```bash
# Start GitLab + Bitbucket fixture and provision test environment
export E2E_BITBUCKET_ADMIN_PASSWORD=$(openssl rand -hex 16)
docker compose -f test/e2e/docker-compose.yml --profile bitbucket up -d
./test/e2e/scripts/wait-for-gitlab.sh
./test/e2e/scripts/setup-gitlab.sh     # Creates .env.docker
./test/e2e/scripts/register-runner.sh  # Registers CI runner
./test/e2e/scripts/setup-bitbucket.sh  # Provisions the import fixture

# Run tests
set -a && source test/e2e/.env.docker && set +a
go test -v -tags e2e -timeout 600s ./test/e2e/suite/

# Cleanup
docker compose -f test/e2e/docker-compose.yml down -v
```

Or use the Makefile target that automates the full lifecycle:

```bash
make test-e2e-docker
```

For Enterprise/Premium E2E coverage, set `ENTERPRISE_LICENSE` in `.env` or the shell and use:

```bash
make test-e2e-docker-enterprise
```

The Enterprise target runs with the `e2e enterprise` build tags, so common harness files plus `test/e2e/suite/*_ee_test.go` Enterprise/Premium tests are compiled and executed. CE-only tests live in `test/e2e/suite/*_ce_test.go` and remain in `make test-e2e-docker`, while Enterprise-specific fixture behavior can be tuned independently.

The E2E harness also re-validates the GitLab tier at runtime by calling the License API (`GET /api/v4/license`). When an enterprise tier is requested (via `GITLAB_TIER=premium`/`ultimate`, or the legacy `GITLAB_ENTERPRISE=true` harness toggle) but the fixture reports a Free license, the session downgrades to CE and `*_ee_test.go` tests skip cleanly with a logged reason instead of failing outright. This keeps the suite safe against accidental CE/EE mismatches.

Docker mode enables pipeline and job tests that require a CI runner. It also starts an internal `e2e-fixture` HTTP service and configures GitLab to allow local outbound requests, so project webhook, push mirror, and custom emoji tests use deterministic in-network endpoints instead of public Internet access.

#### Test Reports

`make test-e2e`, `make test-e2e-docker`, and `make test-e2e-docker-enterprise` use [gotestsum](https://github.com/gotestyourself/gotestsum) to produce structured test reports in `dist/e2e-reports/`:

| File             | Format    | Purpose                                       |
| ---------------- | --------- | --------------------------------------------- |
| `e2e-junit.xml`  | JUnit XML | CI/CD integration (GitHub Actions, SonarQube) |
| `e2e-log.json`   | JSON      | Programmatic analysis, filtering              |
| `e2e-output.txt` | Plain     | Human-readable console output (`testdox`)     |

Docker mode files use the `e2e-docker-` prefix, and Enterprise Docker files use the `e2e-docker-enterprise-` prefix. Reports are written to `dist/e2e-reports/` (gitignored via `dist/`).

The Makefile targets run `gotestsum` through `tee` with `pipefail` so test failures propagate to the target exit code. Docker targets still tear down containers and volumes before returning a non-zero status on failure.

Install gotestsum via `make install-tools` or `go install gotest.tools/gotestsum@latest`.

#### Test Architecture

The suite uses five MCP server/client pairs via `mcp.NewInMemoryTransports()`:

| Session       | Purpose                                      |
| ------------- | -------------------------------------------- |
| `individual`  | Individual GitLab tools                      |
| `meta`        | Domain meta-tools and action dispatch        |
| `elicitation` | Elicitation tools with mock user handler     |
| `safeMode`    | Mutating tools wrapped as safe-mode previews |

**Workflows:**

| Area               | Description                                                     |
| ------------------ | --------------------------------------------------------------- |
| Individual tools   | Exercises domain tools directly against real GitLab             |
| Meta-tools         | Exercises domain action dispatch through meta-tools             |
| MCP capabilities   | Verifies progress, completions, elicitation, and safe mode      |
| Docker-only runner | Exercises CI pipeline and job behavior with a registered runner |

Docker validation snapshots are written under `dist/e2e-reports/` after `make test-e2e-docker` or `make test-e2e-docker-enterprise`. The generated metrics above count E2E `Test*` entry points statically; they do not replace the runtime report produced by gotestsum.

**Lifecycle covered:** user → project CRUD → commits → branches → tags → releases → issues → labels → milestones → members → upload → MR lifecycle → notes → discussions → search → groups → pipelines → packages → wikis → CI variables → environments → issue links → deploy keys → snippets → pipeline schedules → badges → access tokens → award emoji → elicitation → cleanup

**Domains added in Docker mode** (require CI runner):

- Pipeline create/get/cancel/retry/delete
- Job get/log/retry/cancel

**MCP capability tests** (mock handlers):

- Elicitation tools (1 test): confirm destructive action
- Resource subscriptions: covered by unit tests (`internal/subscriptions/`,
  `cmd/server/subscriptions_test.go`), not e2e — the e2e client drives
  tools, and a subscription needs a client that holds one open

#### Fixture Cleanup

Test fixtures (`fixture_ce_test.go` / `fixture_ee_test.go`) register `t.Cleanup` handlers that **permanently delete** projects created during tests. GitLab's Delayed Deletion feature requires a two-step process:

1. Mark the project for deletion (`DELETE /projects/:id`)
2. Permanently remove it (`DELETE /projects/:id?permanently_remove=true&full_path=...`)

The `cleanupOrphanedProjects` function in `setup_test.go` runs at suite start to remove leftover projects from interrupted runs, including those already in pending-delete state (`IncludePendingDelete` option).

### Meta-Tool Tests

Meta-tool tests verify the action-dispatch layer that consolidates individual tools into base and Enterprise/Premium domain meta-tools. These tests live in `internal/tools/` (the orchestration package).

**What meta-tool tests cover:**

- **Action routing**: Each meta-tool correctly dispatches to the underlying sub-package handler based on the `action` parameter
- **Invalid action**: Requests with unknown actions return an error listing valid actions
- **Metadata audit**: `TestMetadataAudit_*` tests enforce naming conventions, annotations, and tool count invariants across the registered tool catalog
- **Destructive metadata consistency**: `TestDestructiveMetadataConsistency` cross-checks `ActionRoute.Destructive` metadata against `toolutil.DeleteAnnotations` on individual tools — ensures meta-tool routes and individual tools agree on which actions are destructive
- **Markdown formatting**: `markdownForResult` delegates to the type-based registry (`toolutil.MarkdownForResult`) which invokes the formatter registered by the sub-package `init()` function
- **next_steps enrichment**: `enrichWithHints()` correctly extracts hints from Markdown and injects them into JSON `structuredContent`

**Running meta-tool tests:**

```bash
# All orchestration tests (register, metatool, markdown, errors)
go test ./internal/tools/ -count=1 -v

# Metadata audit only
go test ./internal/tools/ -run TestMetadataAudit -count=1 -v

# Specific domain meta-tool tests
go test ./internal/tools/ -run TestProject -count=1 -v
go test ./internal/tools/ -run TestBranch -count=1 -v
```

**E2E meta-tool tests:**

The E2E suite now uses domain-focused `TestMeta_*` entry points rather than one large workflow test. These tests exercise project lifecycle operations and extended domains through meta-tool action dispatch, validating routing, parameter passthrough, and response formatting in a real GitLab environment.

```bash
# Run all meta-tool E2E tests
go test -v -tags e2e -timeout 300s -run '^TestMeta_' ./test/e2e/suite/
```

### Validation Tests

Validation tests in `internal/tools/register_validation_test.go` ensure structural integrity across all sub-packages:

| Test                                                | Purpose                                                                                                                                                  |
| --------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `TestRegisterAllDoesNotUseDomainRegisterTools`      | Verifies root individual registration stays catalog-backed and cannot regress to per-domain `RegisterTools` loops                                        |
| `TestActionSpecCoverage_AllCatalogRoutesClassified` | Builds the GitLab.com Enterprise dynamic catalog and verifies every catalog action is spec-backed                                                        |
| `TestAllMarkdownFormattersRegistered`               | Verifies all ~266 output types across 76 sub-packages have registered markdown formatters via `toolutil.RegisterMarkdown[T]`                             |
| `TestAllHintReferencesValid`                        | Validates all `action 'xxx'` and backtick-quoted `` `gitlab_xxx` `` references in WriteHints across all markdown.go files match registered tools/actions |

```bash
# Run validation tests
go test ./internal/tools/ -run "TestRegisterAllDoesNotUseDomainRegisterTools|TestActionSpecCoverage|TestAllMarkdown|TestAllHint" -count=1 -v
```

## Running Tests

### Unit Tests

```bash
# All unit tests
go test ./internal/... -count=1

# Specific package (verbose)
go test ./internal/tools/branches/ -count=1 -v

# Specific test by name
go test ./internal/tools/ -run TestBranch -count=1

# With coverage
go test ./internal/tools/branches/ -coverprofile=cover.out -count=1
go tool cover -func=cover.out

# With race detector
go test ./internal/... -race -count=1
```

### E2E Tests

```bash
# Full suite (self-hosted GitLab)
go test -v -tags e2e -timeout 300s ./test/e2e/suite/
make test-e2e

# Docker mode (ephemeral GitLab CE container + Bitbucket import fixture)
export E2E_BITBUCKET_ADMIN_PASSWORD=$(openssl rand -hex 16)
docker compose -f test/e2e/docker-compose.yml --profile bitbucket up -d
./test/e2e/scripts/wait-for-gitlab.sh && ./test/e2e/scripts/setup-gitlab.sh && ./test/e2e/scripts/register-runner.sh && ./test/e2e/scripts/setup-bitbucket.sh
set -a && source test/e2e/.env.docker && set +a
go test -v -tags e2e -timeout 600s ./test/e2e/suite/
docker compose -f test/e2e/docker-compose.yml --profile bitbucket down -v

# Individual and meta-tool domains
go test -v -tags e2e -timeout 300s -run '^TestIndividual_' ./test/e2e/suite/
go test -v -tags e2e -timeout 300s -run '^TestMeta_' ./test/e2e/suite/

# Compile-only (verify builds without GitLab)
go test -tags e2e -c -o NUL ./test/e2e/suite/       # Windows
go test -tags e2e -c -o /dev/null ./test/e2e/suite/  # Linux
```

### Coverage Report

```bash
# Full coverage for all internal packages
go test ./internal/... -coverprofile=coverage.out -count=1
go tool cover -func=coverage.out

# HTML coverage report
go tool cover -html=coverage.out -o coverage.html

# Per-package summary
go test ./internal/... -cover -count=1
```

### Makefile Targets

```bash
make test          # Run all unit tests
make test-race     # Run with race detector
make test-e2e      # Run E2E tests (self-hosted GitLab) — generates JUnit + JSON reports
make test-e2e-docker # Run E2E tests with ephemeral GitLab CE — generates JUnit + JSON reports
make test-e2e-docker-enterprise # Run E2E tests with ephemeral GitLab EE + license
make test-e2e-gitlab-com # Run Orbit live tests against GitLab.com (provisions fixtures, waits for indexer, then runs the orbitlive-tagged tests)
make coverage      # Generate coverage report
make lint          # Run consolidated golangci-lint checks
make inspector     # Compile + launch MCP Inspector UI via stdio
make inspector-stop # Stop Inspector and clean up temp binary
```

### Orbit Live Tests

The six `gitlab_orbit_*` tools have a separate `orbitlive`-gated live test suite at `test/e2e/orbit/live_test.go` that exercises the real `https://gitlab.com/api/v4/orbit/*` endpoints against a fixture-provisioned namespace. Unlike the `e2e`-tagged suite, these tests are **not** run by `make test` or any CI gate — they require a GitLab.com Personal Access Token and explicit opt-in.

The suite is organized as four entry points:

| Entry point                              | Subtests | What it exercises                                                                                                                                                                                                                                                                                  |
| ---------------------------------------- | -------: | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `TestOrbitLiveGitLabCom`                 |       14 | All six handlers against the live API: status, schema, tools, DSL (default/llm/raw), query (traversal/aggregation/neighbors/path_finding/llm-format), and graph_status (full_path/namespace_id)                                                                                                    |
| `TestOrbitLiveGitLabCom_ShapeDiscovery`  |        6 | Regression coverage of the canonical Query DSL shapes for each `query_type` variant — `aggregation_with_filter`, `aggregation_with_node_ids`, `neighbors_id_reference`, `path_finding_shortest`, and the default schema format                                                                     |
| `TestOrbitLiveGitLabCom_Fixtures`        |        7 | Filter-based queries against the live `kg-fixtures` and `security-fixtures` projects, scoped by `ORBIT_FIXTURES_NAMESPACE` so the test is portable across developer namespaces                                                                                                                     |
| `TestOrbitLiveGitLabCom_FeatureCoverage` |       14 | Comprehensive DSL surface: filter operators (`in`, `contains`, `gt`), multi-node traversal with `IN_PROJECT`, aggregations with `group_by` (node/property), `sum`/`max`/`avg`, `order_by`, virtual columns (`diff`, `content`), cursor pagination, `id_range` scope, and `options.dynamic_columns` |

Total: **4 suites, 41 subtests** behind the `orbitlive` build tag.

The Orbit indexer is eventually consistent. Subtests that match content the indexer has not yet picked up will report `row_count=0` and pass — they are informational, not strict equality. Re-run the live test a few minutes after `make test-e2e-gitlab-com` to allow the indexer to catch up.

To run the full flow:

```bash
# Add a Personal Access Token (api scope) to .env first
echo 'GITLAB_COM_TOKEN=glpat-...' >> .env

# Default namespace is plens1; override with ORBIT_FIXTURES_NAMESPACE
make test-e2e-gitlab-com ORBIT_FIXTURES_NAMESPACE=acme-research
```

To run only the live tests (when fixtures are already provisioned):

```bash
GITLAB_COM_TOKEN=glpat-... \
  go test -tags orbitlive -count=1 -v -timeout 300s ./test/e2e/orbit/

# Just one suite
GITLAB_COM_TOKEN=glpat-... \
  go test -tags orbitlive -count=1 -v -run '^TestOrbitLiveGitLabCom_Fixtures$' ./test/e2e/orbit/
```

See [Orbit Live Test Fixtures](../orbit-fixtures.md) for fixture contents, the `scripts/setup-orbit-fixtures.sh` script, and the indexer caveat.

## Test Infrastructure

### Shared Helpers (`internal/testutil/`)

| Helper                        | Purpose                                      |
| ----------------------------- | -------------------------------------------- |
| `NewTestClient()`             | Creates mock GitLab client + httptest server |
| `RespondJSON()`               | Writes JSON response body                    |
| `RespondJSONWithPagination()` | Writes JSON + pagination headers             |

### Test File Organization

Each tool sub-package follows this structure:

```text
internal/tools/{domain}/
├── {domain}.go          # Tool handlers
├── {domain}_test.go     # Unit tests
├── action_specs.go      # Canonical ActionSpec route metadata
├── markdown.go          # Markdown formatters (if any)
└── markdown_test.go     # Formatter tests (if any)
```

### E2E Test Structure

```text
test/e2e/
├── docker-compose.yml        # Ephemeral GitLab CE + Runner + fixture service
├── .env.docker               # Docker mode environment variables
├── README.md                 # E2E documentation
├── scripts/                  # Provisioning scripts
│   ├── register-runner.sh
│   ├── setup-gitlab.sh
│   └── wait-for-gitlab.sh
└── suite/                    # Go test package (137 test files)
    ├── setup_test.go         # MCP server setup, helpers, shared state
    ├── fixture_ce_test.go    # Self-contained GitLab CE resource builders
    ├── fixture_ee_test.go    # Self-contained GitLab EE resource builders
    └── *_test.go             # Domain-specific test files
```

### Wizard Test Helpers

The `internal/wizard/` package tests interactive UI code (Web UI, Bubble Tea
TUI, CLI) that would normally open browsers, OS dialogs, and write to real
user config files. Test isolation is achieved via **package-level function
variables** overridden in tests with `t.Cleanup` to restore originals.

**Function variables** (defined in source files, overridden in tests):

| Variable          | Source file    | Real function     | Purpose                    |
| ----------------- | -------------- | ----------------- | -------------------------- |
| `allClientsFn`    | `clients.go`   | `AllClients()`    | Returns MCP client configs |
| `openBrowserFn`   | `browser.go`   | `openBrowser()`   | Launches default browser   |
| `pickDirectoryFn` | `dirpicker.go` | `pickDirectory()` | Opens OS directory picker  |

**Test helpers** (`testhelpers_test.go`):

| Helper                            | Purpose                                                                                                |
| --------------------------------- | ------------------------------------------------------------------------------------------------------ |
| `useFakeClients(t)`               | Overrides `allClientsFn` with clients using temp dir paths — prevents writing to real `mcp.json` files |
| `stubPickDirectory(t, path, err)` | Overrides `pickDirectoryFn` — prevents OS directory dialog                                             |
| `stubOpenBrowser(t)`              | Overrides `openBrowserFn` — prevents browser launch                                                    |

**File organization** (12 test files, 159 test functions):

```text
internal/wizard/
├── clients_test.go        # 25 tests — MCP client detection and config paths
├── cli_test.go            # 18 tests — CLI-mode wizard flow
├── env_file_test.go        #  3 tests — .env file operations
├── install_test.go        #  6 tests — Binary installation logic
├── jsonmerge_test.go      #  9 tests — JSON config merge operations
├── paths_test.go          #  4 tests — Platform-specific path resolution
├── prompt_test.go         # 20 tests — User prompt/input handling
├── run_test.go            #  1 test  — Top-level Run() entry point
├── testhelpers_test.go    #  0 tests — Shared test helpers only
├── tui_test.go            # 43 tests — Bubble Tea TUI model and view
├── webui_test.go          # 15 tests — Web UI HTTP handlers
└── wizard_test.go         # 15 tests — Core wizard orchestration
```
