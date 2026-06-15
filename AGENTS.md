# gitlab-mcp-server — Agent Quick Reference

> MCP server in Go that exposes GitLab REST + GraphQL as MCP tools, resources,
> and prompts. Communicates via stdio (default) or HTTP. Catalog-first tool
> registration: 176 sub-packages under `internal/tools/`, with three runtime
> surfaces (`dynamic` default, `meta`, `individual`).

## Read first

| Where                                               | Why                                                                                 |
| --------------------------------------------------- | ----------------------------------------------------------------------------------- |
| `CLAUDE.md`                                         | Full project context, env vars, ADR list, agents/skills catalog                     |
| `.github/copilot-instructions.md`                   | Auto-loaded by VS Code Copilot; has the language policy, env var table, E2E recipes |
| `.github/instructions/*.md`                         | Auto-applied coding standards (go, MCP, OWASP, comments, code review)               |
| `docs/development/tool-surfaces-and-action-core.md` | Surface ownership and catalog projection rules                                      |
| `docs/adr/`                                         | Architectural Decision Records (catalog-first is ADR-0004)                          |

OpenCode-specific wiring (this file's agents, skills, paths) lives in
`opencode.json` + `.opencode/agent/`. The canonical agents and skills also
exist under `.github/agents/` and `.claude/skills/` for Copilot/Claude; the
opencode mirror is a thin frontmatter shim so they show up in the OpenCode
CLI.

## Hard invariants

- **Catalog-first registration.** For ordinary GitLab API actions, add or
  update domain-local `ActionSpec`s and handlers. Do **not** add
  package-local `RegisterTools` functions or ad hoc `mcp.AddTool` calls.
  The catalog in `internal/tools/action_catalog.go` projects everything
  into meta, dynamic, `gitlab://tools`, audits, LLM files, and individual
  tool surfaces. (ADR-0004.)
- **English-only artifacts.** Every file committed to this repo must be
  in English, including doc comments, ADRs, branches, commits, and MCP
  tool descriptions. Chat with the developer in any language you want.
- **Conventional commits.** `feat:`, `fix:`, `docs:`, `test:`, `refactor:`,
  `chore:`. Use the `git-commit` skill (`.claude/skills/git-commit/`) for
  auto-detected type/scope.
- **Coverage target: maximum possible, 100% when feasible.** CI enforces
  an 80% floor; in practice the team pushes per-package coverage to 100%
  unless there is a hard blocker (generated code, third-party
  interfaces). When you cannot reach 100% on a function, add a
  documented reason at the call site and prefer table-driven tests that
  exercise both happy and error paths.

## Common commands

```bash
# Build
make build                                # ./dist/gitlab-mcp-server
make build-all                            # all 6 GOOS/GOARCH targets

# Test (fast)
make test-pkg PKG=branches                # one domain — the workhorse
go test ./internal/tools/branches/ -count=1 -v
go test ./internal/tools/ -run TestBranch -count=1

# Test (full)
make test-short                           # all unit tests, no coverage
make test                                 # all unit tests, with coverage.out
make coverage                             # writes coverage.html

# Lint / analyze
make analyze                              # golangci-lint + govulncheck + markdownlint
make analyze-fix                          # apply gofumpt/goimports/gci/markdownlint --fix
make golangci-lint                        # Go-only gate
golangci-lint run --build-tags e2e ./internal/tools/branches/  # one package

# Audit / regenerate
go run ./cmd/audit_tools/
go run ./cmd/audit_output/
go run ./cmd/audit_tokens/
go run ./cmd/audit_dynamic_aliases/
go run ./cmd/audit_test_names cmd internal test
make audit-godocs                         # writes dist/analysis/godoc.md

# MCP Inspector
make inspector                            # builds + launches at http://127.0.0.1:6274
make inspector-stop
```

> `golangci-lint` runs with `--build-tags e2e` (set in `.golangci.yml`).
> Running it directly without that tag will fail on e2e-tagged files.
> Use `make golangci-lint` or pass `--build-tags e2e` yourself.

## Post-edit regeneration matrix

| You edited                                          | Run                                                               |
| --------------------------------------------------- | ----------------------------------------------------------------- |
| Domain tool (added/renamed/changed input or output) | `go run ./cmd/gen_readme/`                                        |
| ActionSpec metadata (catalog routes)                | `go run ./cmd/gen_action_catalog_manifest/` (and `--check` in CI) |
| Pipe tables in `README.md` or `docs/`               | `go run ./cmd/format_md_tables/` (and `--check`)                  |
| Tests, after a test phase                           | `go run ./cmd/gen_testing_docs/` (and `--check`)                  |
| Tool surface (registered tools, resources, prompts) | `go run ./cmd/gen_llms/` (and `--check` via `make check-llms`)    |
| `server.json`                                       | `make check-server-json` (uses MCP publisher)                     |

The combined gate is `make audit-docs`. CI runs it on every PR.

## Adding a new GitLab API tool

For a full walkthrough use the `create-mcp-tool` skill
(`.claude/skills/create-mcp-tool/`). Short version:

1. **Plan/spec**: `@plan-expert` agent or `create-implementation-plan` skill
   for non-trivial work.
2. **Sub-package**: `internal/tools/{domain}/{domain}.go` with typed
   `Input`/`Output` structs (`jsonschema` tags). No domain prefix on types
   — the package is the namespace.
3. **ActionSpec**: add a canonical route in `internal/tools/{domain}/action_specs.go`
   with metadata, owner package, compatibility policy, and tests.
4. **Handler + tests**: `httptest`-based table-driven tests using
   `testutil.NewTestClient` and `testutil.RespondJSON`.
5. **Markdown formatter**: register via `toolutil.RegisterMarkdown[T](fn)`
   in the sub-package `markdown.go` `init()`. List formatters must add
   `toolutil.HintPreserveLinks` as the first hint in `WriteHints()`.
6. **Refresh**: `gen_readme`, `gen_action_catalog_manifest`, `format_md_tables`,
   `gen_testing_docs`, `gen_llms` (run `--check` on each before pushing).
7. **Verify**: `make test-pkg PKG={domain}` and
   `golangci-lint run --build-tags e2e ./internal/tools/{domain}/`.
8. **Document**: `docs/tools/{domain}.md` and `docs/tools/README.md`.

## Error handling in tool handlers

All five helpers live in `internal/toolutil/errors.go` (ADR-0007):

- `WrapErr(op, err)` — read-only ops, generic classification
- `WrapErrWithMessage(op, err)` — mutating ops, includes GitLab message
- `WrapErrWithHint(op, err, hint)` — when a corrective action is known
- `WrapErrWithStatusHint(op, err, code, hint)` — hint only applies on a
  specific HTTP status; falls through to `WrapErrWithMessage` otherwise
- `NotFoundResult(resource, id, hints...)` — for `IsHTTPStatus(err, 404)`
  on get handlers; returns a structured result with hints at INFO level

For get handlers: check `IsHTTPStatus(err, 404)` **before** `LogToolCallAll`
and return `NotFoundResult` with `nil` error. `IsHTTPStatus` and
`ContainsAny` come first; status-specific hints come last.

## Markdown formatter pattern

Sub-packages self-register formatters via `init()` against a type-keyed
registry in `internal/toolutil/mdregistry.go`. There is no central
dispatch. `internal/tools/markdown.go` is a thin delegator (~19 lines) to
`toolutil.MarkdownForResult`. List output should include
`toolutil.HintPreserveLinks` so LLMs keep `[text](url)` clickable.

## E2E test gotchas

- **Build tag**: all E2E tests are gated by `-tags e2e`. The unit test
  suite must still compile when that tag is set.
- **Compile-only check** (no GitLab required):
  `go test -tags e2e -c -o /dev/null ./test/e2e/suite/`
- **Self-hosted mode** reads `GITLAB_URL` + `GITLAB_TOKEN` from `.env`.
  Tests create and delete real resources; the user must have permission.
  `make test-e2e` adds a confirmation prompt.
- **Docker mode** needs ~4 GB RAM and runs GitLab CE + runner + fixture
  service:

  ```bash
  set -a && source test/e2e/.env.docker && set +a
  E2E_MODE=docker go test -v -tags e2e -timeout 600s ./test/e2e/suite/
  ```

  Pipeline/Job tools **only** work in Docker mode (CI runner required).
- **Orbit live tests** are a separate package at `test/e2e/orbit/`
  with the `orbitlive` tag. They hit real `https://gitlab.com/api/v4/orbit/*`
  with `GITLAB_COM_TOKEN` from `.env` (default namespace `plens1`):

  ```bash
  make test-e2e-gitlab-com                       # full orchestration
  make test-e2e-gitlab-com ORBIT_FIXTURES_NAMESPACE=acme  # other namespace
  ```

  See `docs/development/orbit-fixtures.md` for the fixture layout and
  indexer caveat.
- **Dynamic-surface only** in Docker mode:
  `E2E_MODE=docker go test -v -tags e2e -timeout 600s -run '^TestDynamicToolSurface' ./test/e2e/suite/`

## Release process

1. `make release` — GoReleaser snapshot, flattens `dist/` to GitHub asset names.
2. **Release link names must be exact filenames** (e.g.
   `checksums.txt.asc`, `gitlab-mcp-server-linux-amd64`). Never add
   descriptive suffixes like `(GPG signature)` — `go-selfupdate` matches
   asset names exactly and will fail with decorated names.
3. Push the tag; CI publishes the GitHub Release and the Docker image.
4. `make fly-deploy-release` ships the matching tag to Fly.io (HTTP mode).

## Common traps

- **Tool surface default is `dynamic` (2 tools).** Most users expect
  meta-tools; remind them to set `TOOL_SURFACE=meta` (stdio) or
  `--tool-surface=meta` (HTTP) when they want the 33/49/50-tool catalog.
- **`.tmp-kg-fixtures/` and `.tmp-token-audit/`** are working dirs for
  the fixture and token-audit scripts. Safe to ignore.
- **`go-selfupdate` asset names** — see the release section above.
- **Coverage minimum 80%** is enforced in CI; locally
  `go test -coverprofile=coverage.out ./cmd/... ./internal/...` then
  `go tool cover -func=coverage.out` reports totals.
- **HTTP mode without `--gitlab-url`** requires every client request to
  send `GITLAB-URL`; missing it is a common first-time error.
- **Auto-update defaults to `true`** in stdio and HTTP. The server may
  restart on its own; disable with `AUTO_UPDATE=false` for local
  debugging.
