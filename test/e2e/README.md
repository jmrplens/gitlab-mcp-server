# End-to-End Tests

E2E tests validate the full MCP server against a real GitLab instance using in-memory transport (`mcp.NewInMemoryTransports()`). Build tag: `e2e`.

## Quick Start

### Self-Hosted Mode

Requires a running GitLab instance with a Personal Access Token that has create/delete project permissions.

```bash
# Create .env in project root
cat > .env <<EOF
GITLAB_URL=https://gitlab.example.com
GITLAB_TOKEN=glpat-...
EOF

# Run
go test -v -tags e2e -timeout 300s ./test/e2e/suite/
```

### Docker Mode

Uses an ephemeral GitLab CE container. Requires Docker and ~4 GB RAM.

All Docker infrastructure is version-controlled in this directory:

- `docker-compose.yml` — GitLab CE + Runner definition
- `scripts/setup-gitlab.sh` — Creates test user, PAT, generates `test/e2e/.env.docker`
- `scripts/register-runner.sh` — Registers CI runner
- `scripts/wait-for-gitlab.sh` — Polls readiness endpoint

All commands run from the **project root**:

```bash
docker compose -f test/e2e/docker-compose.yml up -d
./test/e2e/scripts/wait-for-gitlab.sh
./test/e2e/scripts/setup-gitlab.sh
./test/e2e/scripts/register-runner.sh

set -a && source test/e2e/.env.docker && set +a
go test -v -tags e2e -timeout 600s ./test/e2e/suite/

# Cleanup
docker compose -f test/e2e/docker-compose.yml down -v
```

Or use the Makefile target:

```bash
make test-e2e-docker
```

Docker mode enables pipeline and job tests that require a CI runner.

## Architecture

### Test Files

All Go test files live in the `suite/` subdirectory (package `suite`):

| File                       | Purpose                                              |
| -------------------------- | ---------------------------------------------------- |
| `suite/setup_test.go`      | TestMain, 5 MCP sessions, helpers, shared state      |
| `suite/fixture_test.go`    | Self-contained GitLab resource builders               |
| `suite/*_test.go`          | 91 domain-specific test files                         |

### MCP Sessions

| Session            | Purpose                                  |
| ------------------ | ---------------------------------------- |
| `individual`       | Individual tools                          |
| `meta`             | Meta-tools                                |
| `sampling`         | Sampling tools with mock LLM handler      |
| `elicitation`      | Elicitation tools with mock user handler  |
| `safeMode`         | Mutating tools wrapped to return previews |

### Safety Guardrails

- **Snapshot-based cleanup**: `TestMain` captures pre-test project/group/label/variable state and restores it on exit
- **Unique names**: All test resources use timestamped names to avoid conflicts
- **Scoped parallelism**: Most top-level tests call `t.Parallel()`; lifecycle subtests usually stay sequential inside each top-level test when they share IDs or mutable state

### Isolation and capabilities

E2E tests are grouped by the resource scope they touch. New tests that mutate resources must use an existing fixture helper or explicitly register cleanup for every resource they create. See `suite/CAPABILITIES.md` for the current inventory and future gating plan.

| Scope | Meaning | Parallelism guidance |
| ----- | ------- | -------------------- |
| `project` | Project-owned resources such as files, branches, issues, merge requests, packages, releases, and project settings | Parallel by default when each test creates its own project and cleanup is registered |
| `group` | Group-owned resources such as group projects, members, labels, wikis, epics, and group settings | Parallel by default when each test creates its own group and cleanup is registered |
| `user` | Admin-created or test-created user resources | Requires explicit cleanup and, for admin user lifecycle tests, admin capability checks |
| `current-user` | State attached to the authenticated test user, including status, todos, SSH keys, personal access tokens, and notification preferences | Must be serialized or restored before more parallelism is added |
| `instance-global` | Instance-wide resources such as settings, topics, broadcast messages, feature flags, system hooks, OAuth applications, Sidekiq, and metadata | Must be admin-gated and serialized when mutating global state |
| `runner` | Pipeline and job tests that depend on the Docker CI runner | Requires Docker mode with a registered runner; avoid concurrent runner-heavy lifecycles |
| `enterprise` | Premium or Ultimate features enabled through `GITLAB_ENTERPRISE=true` | Skip cleanly when the instance does not expose the feature |
| `external-network` | Tests that require GitLab to fetch public URLs or contact public Git remotes, such as custom emoji image imports, webhooks, and push mirrors | Disabled by default; set `E2E_EXTERNAL_NETWORK=true` only when outbound access is deterministic |
| `safe-mode` | Safe-mode session where mutating tools return previews instead of changing GitLab state | Parallel when assertions are read-only and no shared resources are mutated |
| `sampling` | Sampling-enabled session with a mock LLM handler | Parallel when each test owns any GitLab resources it creates |
| `elicitation` | Elicitation-enabled session with a mock user handler | Parallel when each test owns any GitLab resources it creates |

## Running Individual Workflows

```bash
# Individual tools only
go test -v -tags e2e -timeout 300s -run TestFullWorkflow ./test/e2e/suite/

# Meta-tools only
go test -v -tags e2e -timeout 300s -run TestMetaToolWorkflow ./test/e2e/suite/
```

## Compile-Only Check

Verify E2E code compiles without needing a GitLab instance:

```bash
go test -tags e2e -c -o /dev/null ./test/e2e/suite/  # Linux/macOS
go test -tags e2e -c -o NUL ./test/e2e/suite/         # Windows
```

## Domain Coverage

**Core lifecycle**: user → project CRUD → commits → branches → tags → releases → issues → labels → milestones → members → upload → MR lifecycle → notes → discussions → search → groups → pipelines → packages → cleanup

**Extended domains (meta-tool workflow)**: wikis, CI variables, CI lint, environments, issue links, deploy keys, snippets, issue discussions, draft notes, pipeline schedules, badges, access tokens, award emoji

**Docker-only domains**: pipeline create/get/cancel/retry/delete, job get/log/retry/cancel

**MCP capability tests**: sampling (11 mock tests), elicitation (1 mock test)
