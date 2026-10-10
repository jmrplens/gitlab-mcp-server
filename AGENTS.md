# gitlab-mcp-server — Agent Quick Reference

> MCP server in Go that exposes GitLab REST + GraphQL as MCP tools, resources,
> and prompts. Communicates via stdio (default) or HTTP. Catalog-first tool
> registration: 180 sub-packages under `internal/tools/`, with three runtime
> surfaces (`dynamic` default, `meta`, `individual`).

## Read first

| Where                                               | Why                                                                                 |
| --------------------------------------------------- | ----------------------------------------------------------------------------------- |
| `CLAUDE.md`                                         | Full project context, env vars, ADR list, agents/skills catalog                     |
| `.github/copilot-instructions.md`                   | Auto-loaded by VS Code Copilot; has the language policy, env var table, E2E recipes |
| `.github/instructions/*.md`                         | Auto-applied coding standards (go, MCP, OWASP, comments, code review)               |
| `docs/development/tool-surfaces-and-action-core.md` | Surface ownership and catalog projection rules                                      |
| `docs/development/adr`                              | Architectural Decision Records (catalog-first is ADR-0004)                          |
| `site/src/content/docs/`                            | User documentation: one page per slug, each with its Spanish twin under `es/`       |

That site, published at <https://jmrp.io/docs/gitlab-mcp-server/>, is the only
home of the user documentation; `docs/` keeps `docs/development`, for
contributors (ADR-0025).

The canonical agents live under `.github/agents/` and the skills under
`.github/skills/`; `.claude/agents/` and `.claude/skills/` hold a symbolic
link to each for Claude Code. OpenCode wiring is per developer: `opencode.json` and
`.opencode/` are git-ignored and never committed.

## Hard invariants

- **Catalog-first registration.** For ordinary GitLab API actions, add or
  update domain-local `ActionSpec`s and handlers. Do **not** add
  package-local `RegisterTools` functions or ad hoc `mcp.AddTool` calls.
  The catalog in `internal/tools/action_catalog.go` projects everything
  into meta, dynamic, `gitlab://tools`, audits, LLM files, and individual
  tool surfaces. (ADR-0004.)
- **English-only artifacts.** Every file committed to this repo must be
  in English, including doc comments, ADRs, branches, commits, and MCP
  tool descriptions. The one exception is the Spanish half of the user
  documentation: the twin pages under `site/src/content/docs/es/`, and the
  Spanish text the site and the generators that write it keep beside the
  English (an `es` field, a `labelEs`, `site/src/content/i18n/es.json`).
  Chat with the developer in any language you want.
- **Conventional commits.** `feat:`, `fix:`, `docs:`, `test:`, `refactor:`,
  `chore:`. Use the `git-commit` skill (`.claude/skills/git-commit/`) for
  auto-detected type/scope.
- **Coverage target: maximum possible, 100% when feasible.** CI fails
  below 90% total coverage (`COVERAGE_MIN` in `.github/workflows/ci.yml`);
  in practice the team pushes per-package coverage to 100%
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
make analyze                              # 29 steps: golangci-lint, govulncheck, markdownlint and the cmd/ audits and record checks
make analyze-fix                          # apply gofumpt/goimports/gci/markdownlint --fix
make golangci-lint                        # Go-only gate
golangci-lint run --build-tags e2e ./internal/tools/branches/  # one package

# Audit / regenerate
go run ./cmd/audit_surface_quality/
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

| You edited                                            | Run                                                                                                                                                      |
| ----------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Domain tool (added/renamed/changed input or output)   | `go run ./cmd/audit_tokens/ -footprint`                                                                                                                  |
| ActionSpec metadata (catalog routes)                  | `go run ./cmd/gen_action_catalog_manifest/` (and `--check` in CI)                                                                                        |
| Pipe tables in `README.md`, `docs/` or the site       | `go run ./cmd/format_md_tables/` (and `--check`)                                                                                                         |
| A hand-written page under `site/src/content/docs/`    | Its Spanish twin under `es/` in the same change, then `pnpm run i18n:check`, `facts:check`, `chips:check`, `llms:check` and `version:check` from `site/` |
| A new site page                                       | Its entry in the sidebar (`site/astro.config.mjs`) and in `SECTIONS` (`site/scripts/gen-llms.mjs`); the generated `reference/tools/` need neither        |
| Tests, after a test phase                             | `go run ./cmd/gen_testing_docs/` (and `--check`)                                                                                                         |
| Tool surface (registered tools, resources, prompts)   | `go run ./cmd/gen_llms/` (and `--check` via `make check-llms`), plus `go run ./cmd/gen_lhm_manifest/` (and `--check` via `make check-lhm-manifest`)      |
| Catalog actions or groups (the site's tool reference) | `make gen-tool-reference` (and `make check-tool-reference`); a new group needs its entry in `cmd/gen_tool_reference/domains.json`                        |
| `server.json`                                         | `make check-server-json` (uses MCP publisher)                                                                                                            |

`make audit-docs` runs the combined documentation gate locally. **CI does not
run that target**, but it runs its parts in separate jobs: the `generated` job
runs the Go freshness gates (`check-llms`, `check-tool-reference`,
`check-lhm-manifest`, `check-site-stats`, `check-testing-docs` and
`check-md-tables` among them, beside `check-server-json`, `check-openplugin`,
`check-footprint` and `check-action-catalog-manifest`), the `analyze-md` job
runs markdownlint over every `.md` and `.mdx` file and the two gates that need
Node (`check-doc-links`, `check-mcpb`), `site-lint` runs the site's static
checks and `site-build` builds the site and checks what it wrote. Adding a new
`check-*` target means adding it to one of those jobs. The README statistics, since removed, and
`check-footprint` sat stale on `main` for several releases precisely because
nothing gated them, and
`docs/development/testing/testing.md` drifted for the same reason.
`check-testing-docs` gates everything in that file a checkout determines and
not the coverage columns, which no two machines agree on; refresh those with
`make gen-testing-docs`.

Three parts of `audit-docs` run in no CI job and are yours to run before
pushing: the godoc audit (`go run ./cmd/godoc_tool/ audit`), the dynamic alias
audit (`go run ./cmd/audit_dynamic_aliases/`) and the surface-quality report
(`go run ./cmd/audit_surface_quality/ -view=all`; CI runs its gate,
`make check-surface-quality`).

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
   `testutil.NewTestClient` and `testutil.RespondJSON`. A GraphQL document sent
   through that client is validated against the pinned GitLab schema before the
   mock answers, so a mock can no longer accept what GitLab refuses; a document
   no test drives is covered by `make check-graphql-documents`. That client also
   records the request each call issues, so a new endpoint shows up in
   `docs/development/request-inventory.json` after `make gen-request-inventory`.
   A package no test drives records nothing, and `make audit-1to1-paths` fails
   on that: it is the one check that reads the request rather than the surface.
5. **Markdown formatter**: register via `toolutil.RegisterMarkdown[T](fn)`
   in the sub-package `markdown.go` `init()`. List formatters must add
   `toolutil.HintPreserveLinks` as the first hint in `WriteHints()`.
6. **Refresh**: `audit_tokens -footprint`, `audit_metrics -site-stats` (`make gen-site-stats`),
   `gen_action_grants` (`make gen-action-grants`), `gen_action_catalog_manifest`, `format_md_tables`,
   `gen_testing_docs`, `gen_llms`, `gen_lhm_manifest` (run `--check` on each before pushing).
7. **Verify**: `make test-pkg PKG={domain}` and
   `golangci-lint run --build-tags e2e ./internal/tools/{domain}/`.
8. **Document**: `make gen-tool-reference` regenerates the site's per-domain tool reference from the catalog (`make update-all` runs it with `gen-site-stats` and `gen-action-grants`); a new catalog group needs its overview and sample questions, in English and Spanish, in `cmd/gen_tool_reference/domains.json`. A change a user can see also updates the site page that describes it and that page's Spanish twin; `docs/` holds contributor documentation only.

## Error handling in tool handlers

The four wrappers live in `internal/toolutil/errors.go` and `NotFoundResult`
in `internal/toolutil/not_found.go` (ADR-0007):

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

A hint that names a role, a license or an owner is keyed on
`IsPermissionRefusal(err)`, not on a status: GitLab refuses a missing
permission with 401 at many routes (entry 55 of
`docs/development/upstream-bugs.md`), and a hint keyed on 401 alone would
follow the verdict that the token itself was refused. It is true for a REST
401 or 403 whose body carries no RFC 6750 error code and does not refuse the
account itself (a blocked or deactivated account, for one); a route whose 403
means something else pairs it with `IsHTTPStatus`.

## Markdown formatter pattern

Sub-packages self-register formatters via `init()` against a type-keyed
registry in `internal/toolutil/md_registry.go`. There is no central
dispatch. `internal/tools/markdown.go` is a thin delegator (~17 lines) to
`toolutil.MarkdownForResult`. List output should include
`toolutil.HintPreserveLinks` so LLMs keep `[text](url)` clickable.

## E2E test gotchas

- **Build tag**: all E2E tests are gated by `-tags e2e`. The unit test
  suite must still compile when that tag is set.
- **Compile-only check** (no GitLab required):
  `go test -tags e2e -c -o /dev/null ./test/e2e/gitlab/...`
- **Self-hosted mode** reads `GITLAB_URL` + `GITLAB_TOKEN` from `.env`.
  Tests create and delete real resources; the user must have permission.
  `make test-e2e` (an alias of `make test-e2e-gitlab`) runs it, and a
  package the instance cannot serve skips.
- **Docker mode** needs the memory for GitLab (it peaks at about 7.5 GiB in a
  complete CE run and is capped at 12 GiB) and runs GitLab CE + runner + fixture
  service:

  ```bash
  make test-e2e-ce   # provisions the stack, runs it, tears it down
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
- **The surface is a subtest, not a family**: every scenario runs on
  dynamic, meta and individual, so `-run 'TestIssue_Lifecycle/dynamic'`
  is how one surface is driven on its own.

## Release process

1. `make release` — GoReleaser snapshot, flattens `dist/` to GitHub asset names.
2. **Release link names must be exact filenames** (e.g.
   `checksums.txt.sigstore.json`, `gitlab-mcp-server-linux-amd64`). Never add
   descriptive suffixes like `(GPG signature)` — the Homebrew formula,
   winget, the installers and `scripts/fetch-release-assets.sh` look
   assets up by exact name and will not find a decorated one.
3. Push the tag; CI publishes the GitHub Release and the Docker image.
4. The public hosted endpoint `https://mcp.jmrp.io/gitlab` is deployed out of
   band from the [mcp.jmrp.io](https://github.com/jmrplens/mcp.jmrp.io) host —
   this repository publishes artifacts only.

## Common traps

- **Tool surface default is `dynamic` (2 tools).** Most users expect
  meta-tools; remind them to set `GITLAB_MCP_TOOL_SURFACE=meta` (stdio) or
  `--tool-surface=meta` (HTTP) when they want the 34/40/51/52-tool catalog
  (Free/CE, Premium, self-managed Ultimate, GitLab.com Ultimate).
- **`.tmp-*/` directories** at the repository root are git-ignored working
  dirs (`scripts/setup-orbit-fixtures.sh` clones into `.tmp-orbit-*` and
  removes each when it is done). Safe to ignore.
- **Release asset names are looked up verbatim** — see the release section above.
- **Coverage minimum 90%** (total) is enforced in CI; locally
  `go test -count=1 -coverpkg=./cmd/...,./internal/... -coverprofile=coverage.out ./cmd/... ./internal/...`
  then `go tool cover -func=coverage.out` reports the total the same way.
- **HTTP mode without `--gitlab-url`** (or `GITLAB_URL` in the environment)
  refuses to start unless `--allow-any-gitlab-url` is passed, which only a
  loopback address or a unix socket accepts. With several instances
  published, or none under that flag, every client request must send
  `GITLAB-URL`.
