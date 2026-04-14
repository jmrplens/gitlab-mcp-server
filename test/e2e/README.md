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
GITLAB_USER=your-username
EOF

# Run
go test -v -tags e2e -timeout 300s ./test/e2e/
```

### Docker Mode

Uses an ephemeral GitLab CE container. Requires Docker and ~4 GB RAM.

All Docker infrastructure is version-controlled in this directory:

- `docker-compose.yml` — GitLab CE + Runner definition
- `scripts/setup-gitlab.sh` — Creates test user, PAT, generates `.env.docker` at project root
- `scripts/register-runner.sh` — Registers CI runner
- `scripts/wait-for-gitlab.sh` — Polls readiness endpoint

All commands run from the **project root**:

```bash
docker compose -f test/e2e/docker-compose.yml up -d
./test/e2e/scripts/wait-for-gitlab.sh
./test/e2e/scripts/setup-gitlab.sh
./test/e2e/scripts/register-runner.sh

set -a && source .env.docker && set +a
go test -v -tags e2e -timeout 600s ./test/e2e/

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

| File                       | Purpose                                              |
| -------------------------- | ---------------------------------------------------- |
| `setup_test.go`            | TestMain, 4 MCP sessions, helpers, shared state      |
| `workflow_test.go`         | TestFullWorkflow — individual tool subtests (~174)    |
| `metatool_workflow_test.go`| TestMetaToolWorkflow — meta-tool subtests (~151)      |

### MCP Sessions

| Session            | Purpose                                  |
| ------------------ | ---------------------------------------- |
| `session`          | Individual tools (TestFullWorkflow)       |
| `metaSession`      | Meta-tools (TestMetaToolWorkflow)         |
| `samplingSession`  | Sampling tools with mock LLM handler     |
| `elicitSession`    | Elicitation tools with mock user handler |

### Safety Guardrails

- **Snapshot-based cleanup**: `TestMain` captures pre-test project/group/label/variable state and restores it on exit
- **Unique names**: All test resources use timestamped names to avoid conflicts
- **Sequential execution**: All subtests run sequentially sharing state via `testState`/`metaState`

## Running Individual Workflows

```bash
# Individual tools only
go test -v -tags e2e -timeout 300s -run TestFullWorkflow ./test/e2e/

# Meta-tools only
go test -v -tags e2e -timeout 300s -run TestMetaToolWorkflow ./test/e2e/
```

## Compile-Only Check

Verify E2E code compiles without needing a GitLab instance:

```bash
go test -tags e2e -c -o /dev/null ./test/e2e/  # Linux/macOS
go test -tags e2e -c -o NUL ./test/e2e/         # Windows
```

## Domain Coverage

**Core lifecycle**: user → project CRUD → commits → branches → tags → releases → issues → labels → milestones → members → upload → MR lifecycle → notes → discussions → search → groups → pipelines → packages → cleanup

**Extended domains (meta-tool workflow)**: wikis, CI variables, CI lint, environments, issue links, deploy keys, snippets, issue discussions, draft notes, pipeline schedules, badges, access tokens, award emoji

**Docker-only domains**: pipeline create/get/cancel/retry/delete, job get/log/retry/cancel

**MCP capability tests**: sampling (11 mock tests), elicitation (1 mock test)
