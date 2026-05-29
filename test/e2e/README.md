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
# Required when running webhook/custom-emoji tests outside Docker mode.
# Must be reachable from the GitLab instance, not just from the test process.
E2E_FIXTURE_URL=https://fixture.example.com
# Optional when GitLab must reach itself through a different URL for push mirror tests.
E2E_GITLAB_INTERNAL_URL=https://gitlab.example.com
EOF

# Run
go test -v -tags e2e -timeout 300s ./test/e2e/suite/
```

### Docker Mode

Uses an ephemeral GitLab CE container. Requires Docker and ~4 GB RAM.

All Docker infrastructure is version-controlled in this directory:

- `docker-compose.yml` — GitLab CE + Runner + fixture service definition
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

### Docker Enterprise Mode

Enterprise mode uses the same Docker topology with the EE image and a local
Ultimate subscription. Store a 24-character activation code in `.env` as
`ENTERPRISE_LICENSE` or `GITLAB_ACTIVATION_CODE`, or export it in the shell; the
Docker target passes activation codes to the GitLab EE container during startup.
`make test-e2e-docker-enterprise` runs with the `e2e enterprise` build tags, so
common harness files plus `test/e2e/suite/*_ee_test.go` Enterprise/Premium tests
are compiled and executed. CE-only tests live in `test/e2e/suite/*_ce_test.go`
and remain in `make test-e2e-docker`.
After a successful activation-code run, the setup script exports the generated
license key to `test/e2e/.enterprise-license` with owner-only permissions. Future
runs prefer that ignored local cache and install it through the License API, so
they do not need to spend the activation code again. Delete the cache file to
force a fresh activation-code flow.
Legacy `.gitlab-license` keys can still be stored in `ENTERPRISE_LICENSE`; the
setup script installs those through the License API without writing the secret
into `test/e2e/.env.docker`.

```bash
make test-e2e-docker-enterprise
```

Equivalent manual setup:

```bash
GITLAB_ACTIVATION_CODE="$ENTERPRISE_LICENSE" env GITLAB_IMAGE=gitlab/gitlab-ee:latest docker compose -f test/e2e/docker-compose.yml up -d
./test/e2e/scripts/wait-for-gitlab.sh
GITLAB_ENTERPRISE=true ./test/e2e/scripts/setup-gitlab.sh
./test/e2e/scripts/register-runner.sh

set -a && source test/e2e/.env.docker && set +a
go test -v -tags e2e -timeout 600s ./test/e2e/suite/

env GITLAB_IMAGE=gitlab/gitlab-ee:latest docker compose -f test/e2e/docker-compose.yml down -v
```

Docker mode enables pipeline and job tests that require a CI runner, and starts an internal fixture service used by webhook and custom emoji tests. The setup script also writes `E2E_FIXTURE_URL` and `E2E_GITLAB_INTERNAL_URL` into `.env.docker` so CI runs all non-EE tests without public Internet dependencies.

## Architecture

### Test Files

All Go test files live in the `suite/` subdirectory (package `suite`):

| File                       | Purpose                                              |
| -------------------------- | ---------------------------------------------------- |
| `suite/setup_test.go`      | TestMain, 6 MCP sessions, helpers, shared state      |
| `suite/fixture_test.go`    | Self-contained GitLab resource builders               |
| `suite/*_test.go`          | 91 domain-specific test files                         |

### MCP Sessions

| Session            | Purpose                                  |
| ------------------ | ---------------------------------------- |
| `individual`       | Individual tools                          |
| `meta`             | Meta-tools                                |
| `dynamic`          | Default dynamic find/execute surface                 |
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
| `external-network` | Reserved for tests that truly require public Internet access | Prefer Docker fixture endpoints or test-owned GitLab projects so CI can execute non-EE tests without skips |
| `safe-mode` | Safe-mode session where mutating tools return previews instead of changing GitLab state | Parallel when assertions are read-only and no shared resources are mutated |
| `dynamic` | Default two-tool dynamic surface over the canonical action catalog | Parallel when each test owns created resources and uses find/execute rather than direct meta-tool calls |
| `sampling` | Sampling-enabled session with a mock LLM handler | Parallel when each test owns any GitLab resources it creates |
| `elicitation` | Elicitation-enabled session with a mock user handler | Parallel when each test owns any GitLab resources it creates |

## Running Individual Workflows

```bash
# Individual tools only
go test -v -tags e2e -timeout 300s -run TestFullWorkflow ./test/e2e/suite/

# Meta-tools only
go test -v -tags e2e -timeout 300s -run TestMetaToolWorkflow ./test/e2e/suite/

# Dynamic find/execute surface only
go test -v -tags e2e -timeout 300s -run '^TestDynamicToolSurface_' ./test/e2e/suite/

# Dynamic surface only in Docker mode after setup-gitlab.sh and register-runner.sh
E2E_MODE=docker go test -v -tags e2e -timeout 600s -run '^TestDynamicToolSurface_' ./test/e2e/suite/
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

**Dynamic surface workflow**: public tool inventory, find, execute, standalone project discovery, multi-intent discovery, and destructive-action confirmation guard

**Docker-only domains**: pipeline create/get/cancel/retry/delete, job get/log/retry/cancel

**MCP capability tests**: sampling (11 mock tests), elicitation (1 mock test)
