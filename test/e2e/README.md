# End-to-End Tests

E2E tests validate the full MCP server against a real GitLab instance using in-memory transport (`mcp.NewInMemoryTransports()`). Build tag: `e2e`.

There are five modules, answering different questions:

| Module               | Build tag      | Needs GitLab | What it covers                                                                       |
| -------------------- | -------------- | ------------ | ------------------------------------------------------------------------------------ |
| `test/e2e/suite`     | `e2e`          | yes          | Tool behaviour against a real instance, over in-memory MCP transport                  |
| `test/e2e/http`      | `httpe2e`      | no           | The HTTP transport itself: cross-origin, preflight, auth modes, rate limiting, proxy  |
| `test/e2e/stdio`     | `stdioe2e`     | no           | The stdio transport: pipes, process lifetime, exit status, environment configuration  |
| `test/e2e/collector` | `collectore2e` | no           | Telemetry against a real OpenTelemetry Collector in Docker: acceptance and span shape |
| `test/e2e/orbit`     | `orbitlive`    | gitlab.com   | The experimental Knowledge Graph API                                                  |

Each tag has to be listed in `GO_ANALYSIS_TAGS` in the Makefile and in `e2eTags` in `cmd/gen_testing_docs`, or the module is invisible to `go vet`, to `golangci-lint` and to the generated test metrics. A file behind a tag nothing names is analysed by nothing.

## HTTP transport module

```bash
make test-e2e-http
```

No GitLab and no credentials: the module builds the binary, starts it with the flags each case needs, and stands up a fake instance for the few behaviours that only appear once GitLab answers — the failure budget is charged when a credential is *rejected*, and deliberately not when the instance is merely unreachable.

It exists because the in-memory suite cannot reach any of this. The handler chain is assembled in `package main`, so a unit test could not import it and a test that reassembled it would be testing its own copy. Every case in the module corresponds to something that shipped broken: a preflight answered `401`, which made `--trusted-origins` useless in a browser; a throttled GitLab reported as an invalid token; an invalid token relayed upstream on every retry; and an `x-mcp-header` annotation carrying a prefix the transport also adds.

The `TestProxy_*` cases run a real nginx in Docker in front of the server, and **skip** when Docker is unavailable rather than modelling one. That layer is not optional detail: a proxy answering `OPTIONS` itself hides a server that cannot, and a proxy adding its own CORS headers collides with the server's to produce a response `curl` reports as `200` and a browser refuses outright.

## stdio transport module

```bash
make test-e2e-stdio
```

Also no GitLab and no credentials. It builds the binary, starts it with the environment each case needs, and speaks JSON-RPC to it over real pipes.

stdio is this project's primary transport and nothing anywhere drove it until this module existed. The in-memory suite runs in one process, so it has no pipes, no separation of stdout from stderr, no process to exit, and no environment-variable configuration, since HTTP mode takes flags instead. Every case here corresponds to something that shipped broken and was found by hand against a binary: a nil dereference that killed the process on an ordinary eliciting tool call; a keepalive ping that closed the session of any client speaking `2026-07-28` after 45 idle seconds, held in place by a unit test asserting the ping ought to be there; an exit status of 1 on every normal shutdown; and a log stream whose steady state was entirely the SDK's own session chatter.

## Collector module

```bash
make test-e2e-collector
```

No GitLab and no credentials either, but Docker is required: it starts a pinned `otel/opentelemetry-collector-contrib` with an OTLP receiver and file exporters, points the real binary at it, drives traffic, and asserts on the OTLP JSON the collector wrote after parsing what it was sent. Without Docker it **skips**, with the reason, rather than modeling a receiver.

It exists because every other telemetry test in this repository is graded by code we wrote. The in-process receiver in `test/e2e/http` answers `200` to whatever it is handed and stores the bytes, which is right for the two questions it asks (was the collector credential sent, did anything private leak) and is no evidence that the export is well formed. A malformed protobuf, a resource missing an attribute a backend requires, or a metric whose unit contradicts its name all pass a stub and all ship telemetry an operator cannot use. So this module asserts acceptance and shape, and deliberately repeats none of the credential or privacy assertions.

Unlike the two transport modules it is **not** in the push-triggered CI jobs, because it pulls a container image. It belongs with the Docker-mode targets.

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

Uses an ephemeral GitLab CE container plus a Bitbucket Data Center import fixture. Requires Docker and ~5.5 GB RAM.

All Docker infrastructure is version-controlled in this directory:

- `docker-compose.yml` — GitLab CE + Runner + Bitbucket + fixture service definition
- `scripts/setup-gitlab.sh` — Creates test user, PAT, generates `test/e2e/.env.docker`
- `scripts/setup-bitbucket.sh` — Provisions the Bitbucket import fixture, appends `BITBUCKET_SERVER_*`
- `scripts/register-runner.sh` — Registers CI runner
- `scripts/wait-for-gitlab.sh` — Polls readiness endpoint

All commands run from the **project root**. The Bitbucket fixture needs an
ephemeral admin password shared between compose and the provisioning script
(`make test-e2e-docker` generates one per run automatically):

```bash
export E2E_BITBUCKET_ADMIN_PASSWORD=$(openssl rand -hex 16)
docker compose -f test/e2e/docker-compose.yml --profile bitbucket up -d
./test/e2e/scripts/wait-for-gitlab.sh
./test/e2e/scripts/setup-gitlab.sh
./test/e2e/scripts/register-runner.sh
./test/e2e/scripts/setup-bitbucket.sh

set -a && source test/e2e/.env.docker && set +a
go test -v -tags e2e -timeout 600s ./test/e2e/suite/

# Cleanup
docker compose -f test/e2e/docker-compose.yml --profile bitbucket down -v
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

### What setup-gitlab.sh provisions

Beyond the test user and PAT, the setup script prepares the instance so
coverage that used to skip can run:

- **Instance settings**: enables the `github`, `bitbucket`, and
  `bitbucket_server` importer sources (GitLab ships with them disabled) and
  sets `default_branch_protection=0` so unprotect-then-push flows cannot race
  GitLab's asynchronous default-branch protection job.
- **Container registry seed**: creates the `e2e-registry-seed` project and
  pushes `busybox:stable` with two tags (`seed-a`, `seed-b`) through the
  plain-HTTP registry at `localhost:5050`, recording `E2E_REGISTRY_PROJECT`
  in `.env.docker`. The push writes its auth into a scratch `DOCKER_CONFIG`
  instead of calling `docker login`, which needs an unlocked keychain on
  macOS. Without a docker CLI the registry tag tests skip.
- **GitLab Pages** is enabled in the omnibus config; the Pages write test
  publishes a real deployment through a `pages` CI job before asserting.
- **Pending schema migration seed**: deletes the newest `schema_migrations`
  row via `gitlab-psql` and records its version as
  `E2E_DB_MIGRATION_VERSION`, so the mutating `db_migration_mark` path runs
  for real — the test's mark call re-inserts the row, leaving the instance
  as it started. The newest version is used because GitLab squashes old
  migrations and only recent versions still have a resolvable migration
  file.

### What setup-bitbucket.sh provisions

The CE docker target also starts a real Bitbucket Data Center container
(`e2e-bitbucket`, compose profile `bitbucket`) so `import_bitbucket_server`
runs instead of skipping:

- **Unattended setup** is driven by a `bitbucket.properties` file written
  before the stock entrypoint runs, using Atlassian's public 3-hour
  [timebomb Data Center test license](https://developer.atlassian.com/platform/marketplace/timebomb-licenses-for-testing-server-apps/)
  — long enough for an ephemeral run, torn down with the stack. Search is
  disabled and the embedded eval database is used, keeping the container at
  one ~1 GB JVM.
- **Fixture content**: project `E2E`, repository `fixture` with an initial
  commit seeded through the browse edit REST API, and an admin HTTP access
  token for the importer.
- The `BITBUCKET_SERVER_*` variables are appended to `.env.docker` with the
  compose-network URL (`http://e2e-bitbucket:7990`) because GitLab — not the
  test process — dereferences it. Provisioning is best-effort: on any
  failure it warns and the suite falls back to the documented skip.

### Optional operator credentials

Tests that talk to external services read the repository root `.env` (loaded
as a fallback in Docker mode; `.env.docker` values keep precedence) and skip
when unset:

| Variable | Used by |
| --- | --- |
| `GH_TOKEN` | `import_github` / `import_cancel_github` / `import_gists` (a repository owned by the token's user is resolved via the GitHub API) |
| `BITBUCKET_USERNAME`, `BITBUCKET_API_TOKEN`, `BITBUCKET_EMAIL`, `BITBUCKET_REPO_PATH` | Bitbucket Cloud imports (Atlassian API token created **with Bitbucket scopes**, paired with the account email) |
| `BITBUCKET_SERVER_URL`, `BITBUCKET_SERVER_USERNAME`, `BITBUCKET_SERVER_TOKEN`, `BITBUCKET_SERVER_PROJECT_KEY`, `BITBUCKET_SERVER_REPO_SLUG` | `import_bitbucket_server` against a self-hosted Bitbucket Server (auto-provisioned by the Docker fixture; set manually only for self-hosted mode) |
| `E2E_DB_MIGRATION_VERSION` | The mutating `db_migration_mark` path (auto-seeded by setup-gitlab.sh in Docker mode; set manually only when debugging a genuinely stuck migration on a self-hosted instance — the error path always runs) |

Imported projects are registered for permanent deletion in the per-test
resource ledger.

## Architecture

### Test Files

All Go test files live in the `suite/` subdirectory (package `suite`):

| File                       | Purpose                                              |
| -------------------------- | ---------------------------------------------------- |
| `suite/setup_test.go`      | TestMain, 6 MCP sessions, helpers, shared state      |
| `suite/fixture_ce_test.go` | Self-contained GitLab resource builders (CE runtime) |
| `suite/fixture_ee_test.go` | Self-contained GitLab resource builders (EE runtime) |
| `suite/*_test.go`          | 137 domain-specific test files                        |

### MCP Sessions

| Session            | Purpose                                  |
| ------------------ | ---------------------------------------- |
| `individual`       | Individual tools                          |
| `meta`             | Meta-tools                                |
| `dynamic`          | Default dynamic find/execute surface                 |
| `elicitation`      | Elicitation tools with mock user handler  |
| `safeMode`         | Mutating tools wrapped to return previews |

Resource subscriptions (`resources/subscribe`) are deliberately not part of
this suite: the e2e client drives tools through one-shot calls, while a
subscription needs a client that holds one open and waits for
notifications. They are covered by unit tests instead
(`internal/subscriptions/`, `cmd/server/subscriptions_test.go`).

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

**MCP capability tests**: elicitation (1 mock test)
