# Contributing to gitlab-mcp-server

Thank you for your interest in contributing to gitlab-mcp-server! This guide covers the process for submitting changes, reporting issues, and following project conventions.

By participating, you agree to abide by the [Code of Conduct](CODE_OF_CONDUCT.md).
For security issues, please follow the [Security Policy](SECURITY.md) instead of opening a public issue.

## Table of Contents

- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Branch Naming](#branch-naming)
- [Commit Messages](#commit-messages)
- [Pull Requests](#pull-requests)
- [Code Standards](#code-standards)
- [Testing](#testing)
- [Documentation](#documentation)
- [Issue Reporting](#issue-reporting)
- [Labels](#labels)

## Getting Started

1. Clone the repository
2. To run the end-to-end suite against your own GitLab, create a `.env` at the repository root with `GITLAB_URL` and `GITLAB_TOKEN`: `make test-e2e` reads it. The server itself never loads a `.env` from its working directory (see [Configuration](https://jmrp.io/docs/gitlab-mcp-server/configuration/) for how it is configured)
3. Run `make build` to verify the setup
4. Run `make test` to ensure all tests pass

## Development Setup

### Prerequisites

- **Go 1.27+** — [Download](https://go.dev/dl/)
- **Node.js 24.18+ with Corepack**, required for the documentation site and MCP Inspector. The `packageManager` field in `site/package.json` names the pnpm version (`pnpm@12.4.2` today) and is authoritative; pnpm settings live in `site/pnpm-workspace.yaml`.
- **Git** — configured with push access
- **GitLab instance** — with a Personal Access Token (`api` scope)

### Build and Test

```bash
# Build
make build

# Run all tests
make test

# Run tests with race detector
make test-race

# Run end-to-end tests (requires .env with real GitLab credentials)
make test-e2e

# Run end-to-end tests in Docker mode (ephemeral GitLab CE; GitLab peaks ~7.5 GiB, capped at 12 GiB)
make test-e2e-docker

# Check test coverage
make coverage

# Lint
make lint

# Launch MCP Inspector (interactive tool testing UI)
make inspector

# Stop MCP Inspector
make inspector-stop
```

## Branch Naming

Use the following naming convention for branches:

| Prefix      | Purpose                 | Example                       |
| ----------- | ----------------------- | ----------------------------- |
| `feature/`  | New functionality       | `feature/gitlab-wiki-tools`   |
| `fix/`      | Bug fixes               | `fix/pagination-off-by-one`   |
| `docs/`     | Documentation only      | `docs/add-wiki-reference`     |
| `test/`     | Test additions          | `test/increase-mr-coverage`   |
| `refactor/` | Code restructuring      | `refactor/extract-pagination` |
| `chore/`    | Build, CI, dependencies | `chore/upgrade-go-sdk`        |

## Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```text
<type>(<scope>): <description>

[optional body]

[optional footer(s)]
```

### Types

| Type       | Description                                             |
| ---------- | ------------------------------------------------------- |
| `feat`     | New feature or tool                                     |
| `fix`      | Bug fix                                                 |
| `docs`     | Documentation changes                                   |
| `test`     | Adding or updating tests                                |
| `refactor` | Code change that neither fixes a bug nor adds a feature |
| `chore`    | Build process, CI, dependency updates                   |
| `perf`     | Performance improvement                                 |
| `style`    | Code formatting (no logic change)                       |

### Scopes

Use the package name as scope when applicable:

```text
feat(tools): add gitlab_wiki_page_create tool
fix(config): handle empty GITLAB_URL gracefully
test(branches): increase coverage to 90%
docs(site): describe the wiki tools in English and Spanish
```

## Pull Requests

### Before Submitting

- [ ] Code compiles: `go build ./...`
- [ ] All tests pass: `go test ./... -count=1`
- [ ] Static analysis is clean: `make analyze` (run `make analyze-fix` first to apply supported Go and Markdown fixes)
- [ ] New tools have tests, and total coverage stays at or above the 90% CI enforces
- [ ] Documentation is updated where the change shows: the site page for user-facing behavior (English and Spanish), `docs/development/` for contributor-facing changes (see [Documentation](#documentation))
- [ ] Commit messages follow conventional commits

### PR Process

1. Create a feature branch from `main`
2. Make your changes in small, focused commits
3. Push the branch and open a pull request — reviewers are auto-requested via [CODEOWNERS](CODEOWNERS)
4. Fill in the PR template (auto-populated from `.github/pull_request_template.md`)
5. Address review feedback
6. Squash-merge once approved (the only allowed merge strategy). The squash commit's message is the pull request description, so the description carries no skip command GitHub reads (a bracketed skip ci or one of the four other commands GitHub documents, or a skip-checks trailer), which would turn off every workflow on main for that commit; `make check-pr-description` refuses one

### PR Size Guidelines

- **Small** (<200 lines): Preferred — faster review, fewer conflicts
- **Medium** (200–500 lines): Acceptable for feature additions
- **Large** (>500 lines): Split into smaller PRs when possible

## Code Standards

### Go Conventions

- Follow idiomatic Go and the repository's consolidated `golangci-lint` configuration (`goimports`, `gofumpt`, `gci`, `govet`, `staticcheck`, `gosec`, and related checks)
- All exported types and functions must have doc comments
- Error wrapping with `fmt.Errorf("context: %w", err)`
- Use `context.Context` consistently for cancellation/timeouts
- Table-driven tests with `t.Run()` subtests

### MCP Tool Patterns

- Each GitLab operation = one canonical action with typed input/output structs
- Use `jsonschema` struct tags for tool input documentation
- Register runtime surfaces from the canonical action catalog
- Set appropriate annotations (readOnlyHint, destructiveHint, etc.)
- Return both structured JSON and human-readable Markdown

### File Organization

```text
internal/tools/
├── action_specs.go          # CollectActionSpecs(): aggregates every domain's ActionSpecs
├── action_catalog.go        # BuildActionCatalog(): the canonical action catalog built from them
├── register.go              # RegisterAll(): projects individual tools from the canonical catalog
├── meta_catalog.go          # RegisterMetaCatalog(): one meta-tool per catalog group
├── register_meta.go         # RegisterMetaStandaloneTools(): the standalone utilities (gitlab_discover_project and the gitlab_interactive_* flows) on the meta and individual surfaces
├── meta_tool.go             # Route wrappers and the meta parameter-schema mode
├── markdown.go              # Delegates to the type-based Markdown registry in toolutil
└── <domain>/                # 180 sub-packages
    ├── doc.go               # Package comment
    ├── action_specs.go      # Canonical ActionSpecs for catalog-backed tool surfaces
    ├── <domain>.go          # Typed input/output structs + handlers
    ├── <domain>_test.go     # Table-driven unit tests
    └── markdown.go          # Markdown formatters (self-registered via init())
```

## Testing

### Requirements

- **Unit tests** for every tool handler — use `httptest` to mock GitLab API responses
- **Table-driven tests** with `t.Run()` subtests
- **Test naming**: `TestToolName_Scenario_ExpectedResult`
- **Coverage target**: CI fails when total coverage of `./cmd/...` and `./internal/...` drops below 90% (`COVERAGE_MIN` in `.github/workflows/ci.yml`); the `increase-test-coverage` skill aims at 100% for every package a change touches
- **No external dependencies**: Unit tests must not call real GitLab APIs

### Running Tests

```bash
# All unit tests
go test ./... -count=1

# Specific package
go test ./internal/tools/... -count=1 -v

# With coverage
go test ./internal/tools/... -coverprofile=cover.out
go tool cover -func=cover.out

# E2E tests (requires real GitLab)
go test -tags e2e -p 1 -timeout 2700s ./test/e2e/gitlab/...
```

## Documentation

### AI-Assisted Development

This project ships with **7 AI agents** and **18 skills** for GitHub Copilot and compatible assistants. Key workflows for contributors:

- **Adding new tools**: Use the `create-mcp-tool` skill — it scaffolds the full tool lifecycle (struct, handler, registration, tests, docs).
- **Improving test coverage**: Use the `increase-test-coverage` skill to identify gaps and cover every package you touch.
- **Documenting a change**: Use the `update-project-documentation` skill, and `update-starlight-docs` for the site pages it leads to.
- **Code quality reviews**: Use the `review-and-refactor` skill for code quality + OWASP security + MCP pattern checks.

See [AGENTS.md](AGENTS.md) for the complete catalog of agents, skills, and instruction files.

### Snapshot Testing (Golden Files)

Tool definitions are snapshot-tested to detect unintentional changes. Golden files live in `internal/tools/testdata/`:

- `tools_individual.json`: all individual tool definitions
- `tools_meta.json`: all meta-tool definitions
- `tools_meta_compact.json` and `tools_meta_full.json`: the meta-tool definitions under the `compact` and `full` parameter schemas

When you intentionally change a tool definition (name, description, schema, annotations), update the golden files:

```bash
UPDATE_TOOLSNAPS=true go test ./internal/tools/ -run TestToolSnapshots -count=1
```

Then commit the updated golden files alongside your code changes. The CI will fail if snapshots are out of date.

### Where Documentation Lives

The user documentation lives only on the [documentation site](https://jmrp.io/docs/gitlab-mcp-server/). Its pages are under `site/src/content/docs/` in English, each with a Spanish twin at the same path under `site/src/content/docs/es/`, and an English page and its twin change in the same pull request: `pnpm run i18n:check` (in `site/`) fails when a page has no twin, and the translation itself is the author's job. `docs/` keeps contributor documentation only, under [`docs/development/`](docs/development/README.md), and the README is a short landing page that links the site. ADR-0025, in the [ADR index](docs/development/adr/README.md), records the decision.

A site page is named by its slug: `operations/http-server` is `site/src/content/docs/operations/http-server.mdx`, published at `https://jmrp.io/docs/gitlab-mcp-server/operations/http-server/`. Link it from a repository file by that URL; `make check-doc-links` resolves the URL and its anchor to the page.

### What Is Generated

Some documentation is written by a generator and never edited by hand. Change what the generator reads, run it, and commit the result; `make update-all` runs every command below (the benchmark through its redraw target, which measures nothing), and CI compares each committed result with what the tree generates.

| Content                                                                                                                       | Command                                                                                                                                |
| ----------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| The per-domain tool reference, `site/src/content/docs/reference/tools/` (its index included) and its `es/` twin               | `make gen-tool-reference` (a new catalog group first needs its overview and sample questions in `cmd/gen_tool_reference/domains.json`) |
| The fine-grained permissions reference, `reference/fine-grained-permissions.mdx` and its twin                                 | `make gen-action-grants`                                                                                                               |
| The counts the site prints (tools, meta-tools, actions, resources, prompts), `site/src/data/stats.json`                       | `make gen-site-stats`                                                                                                                  |
| The token footprint: the README's token claim, `site/src/data/token-footprint.json` and `docs/development/token-footprint.md` | `make gen-footprint`                                                                                                                   |
| The figures and charts of the `performance/resource-benchmark` page                                                           | `make bench-resources` measures; `make bench-resources-render` redraws from the committed record                                       |
| `llms.txt` and the other `llms*.txt` files at the repository root                                                             | `make gen-llms`                                                                                                                        |
| The managed block of `docs/development/testing/testing.md`                                                                    | `make gen-testing-docs`                                                                                                                |
| The capability arrays of `lhm.plugin.json`                                                                                    | `make gen-lhm-manifest`                                                                                                                |

### When to Update

- Adding a new tool → run `make gen-tool-reference`, `make gen-site-stats` and `make gen-action-grants` (or `make update-all`, which runs all three); a new catalog group needs its overview and sample questions in `cmd/gen_tool_reference/domains.json` first
- Adding a new meta-tool action → update the action count table of the site page `tools/meta-tools`
- Adding a new resource or prompt → update the site page `tools/resources-prompts`
- Adding a new capability → update the site page `capabilities/overview` and the capability's own page under `capabilities/`, and [`docs/development/capabilities.md`](docs/development/capabilities.md) for the API a contributor calls
- Changing configuration → update the site pages `configuration` and `reference/environment`, `reference/cli` for a flag, the variable and flag tables in `CLAUDE.md`, and the environment variable and flag tables in `.github/copilot-instructions.md`
- Changing how the code is put together (a new package, the path a call takes) → update [`docs/development/architecture.md`](docs/development/architecture.md); a change to error classification or wrapping → [`docs/development/error-handling.md`](docs/development/error-handling.md); a change to how a bundle or channel is built or published → [`docs/development/distribution.md`](docs/development/distribution.md)
- Changing how the repository works (a gate, a generator, a convention) → update `docs/development/` or `CLAUDE.md`, never the site
- Adding or modifying tests → run `make gen-testing-docs` to refresh `docs/development/testing/testing.md`

Every site page in that list changes in English and Spanish together. The `update-starlight-docs` skill maps the rest of the changes to their pages and lists the site's own checks (`cd site && pnpm run build && pnpm run lint`).

### Language Policy

All project artifacts must be written in **English**:

- Source code, comments, doc comments
- Commit messages, branch names
- Documentation, ADRs, specs
- MCP tool names, descriptions, error messages
- Test names and assertions

The one exception is the Spanish half of the user documentation: the Spanish twin of each site page under `site/src/content/docs/es/`, which translates its English page and changes with it, and the Spanish text kept beside its English source (the `es` fields of `cmd/gen_tool_reference/domains.json`, the Spanish strings of the generators that write both languages, `labelEs` in `site/scripts/gen-llms.mjs`, `site/src/content/i18n/es.json`).

## Release Process

A release is cut by a tag. Pushing a `v*` tag runs `.github/workflows/release.yml`, which builds every binary with GoReleaser, signs and attests what it publishes, creates the GitHub release and publishes the other channels (container images, npm, PyPI, NuGet, Homebrew, winget, the MCP Registry). `gh workflow run release.yml --ref <branch>` rehearses the whole run without publishing anything.

`make release` builds the same binaries locally as a GoReleaser snapshot and flattens `dist/` to the release asset names, which is a way to inspect them, not to publish them. The Release process section of [CLAUDE.md](CLAUDE.md#release-process) carries the details.

## Issue Reporting

Open an issue at <https://github.com/jmrplens/gitlab-mcp-server/issues/new/choose> and pick a template:

- **Bug Report** — reproducible defects
- **Feature Request** — new functionality / new MCP tool
- **Enhancement** — improvement to existing behavior
- **Documentation** — missing, outdated or incorrect docs

For **security issues**, do not open a public issue — report privately via [GitHub Security Advisories](https://github.com/jmrplens/gitlab-mcp-server/security/advisories/new) (see [SECURITY.md](SECURITY.md)).

Templates auto-apply the relevant labels listed in [Labels](#labels).

## Labels

Labels arrive four ways, none of which removes one placed by hand. Issue templates apply their kind label on submission, and their "Area" dropdown is read by `.github/workflows/issue-area.yml`, which applies the area label the answer names. Pull requests are labeled from the paths they touch by `.github/workflows/labeler.yml` against the map in `.github/labeler.yml`, and from the title's conventional prefix by the same workflow: `fix:` is a `bug`, `feat:` a `feature`, and nothing else is mapped. Milestones pair with a version label both ways (`v2.8.0` for milestone `2.8.0`), kept in step by `.github/workflows/version-labels.yml`. The repo uses a flat label set (no `type::`/`priority::` namespaces — those are GitLab conventions), and the area labels use the same words on issues and on pull requests so the two can be filtered together:

| Label              | Color     | Used by                                                                          |
| ------------------ | --------- | -------------------------------------------------------------------------------- |
| `bug`              | `#d73a4a` | Bug Report template; `fix:` titles                                               |
| `feature`          | `#a2eeef` | Feature Request template; `feat:` titles                                         |
| `enhancement`      | `#a2eeef` | Enhancement template (GitHub default)                                            |
| `documentation`    | `#0075ca` | Documentation template; path labeler on docs-only PRs                            |
| `security`         | `#d73a4a` | Security Advisories; path labeler on security paths; Area "Authentication"       |
| `ci`               | `#bfdadc` | Path labeler — pipeline, workflows, lint configuration                           |
| `distribution`     | `#fbca04` | Path labeler — release artifacts and install channels; Area "Installation"       |
| `dependencies`     | `#0052cc` | Path labeler and Dependabot — dependency files                                   |
| `transport`        | `#5c8dd6` | Path labeler — `cmd/server`, the transport e2e modules; Area "Transport"         |
| `mcp`              | `#a371f7` | Path labeler — resources, prompts, completions, subscriptions; Area "MCP"        |
| `tools`            | `#1f883d` | Area "GitLab tools" only: two thirds of PRs touch `internal/tools/`, so no path  |
| `telemetry`        | `#6fbf9a` | Path labeler — `internal/telemetry`, `internal/mcpotel`; Area "Telemetry"        |
| `site`             | `#f0a35c` | Path labeler — the site's build and components, not its content; Area "Site"     |
| `tooling`          | `#9aa5b1` | Path labeler — the auditors and generators under `cmd/`; Area "CI and tooling"   |
| `platform`         | `#e9c46a` | Path labeler — `*_windows.go`, `*_darwin.go`, `*_unix.go`; Area "Platform"       |
| `e2e`              | `#8bd3c7` | Path labeler — the e2e suite and its Docker fixtures                             |
| `release`          | `#0e8a16` | Manual — release tracking issues                                                 |
| `high-priority`    | `#b60205` | Manual — critical bugs and security advisories                                   |
| `needs-triage`     | `#c2e0c6` | All issue templates; retired by `triage.yml` on milestone or close               |
| `v<version>`       | `#1d76db` | `version-labels.yml` — paired with the milestone of the same number              |
| `good first issue` | `#7057ff` | Manual — newcomer-friendly issues                                                |
| `help wanted`      | `#008672` | Manual — community contributions welcome                                         |
| `question`         | `#d876e3` | Manual — questions / discussions                                                 |
| `duplicate`        | `#cfd3d7` | Manual — duplicates of existing issues                                           |
| `invalid`          | `#e4e669` | Manual — out of scope                                                            |
| `wontfix`          | `#ffffff` | Manual — accepted but won't implement                                            |

The Area dropdown's options are the strings `issue-area.yml` matches, so a change to one has to land in the three templates and that workflow together; an answer the workflow does not know fails its run rather than passing quietly, which is how the drift is noticed.

The path labeler never removes a label (`sync-labels: false`), so anything applied by hand survives a later push. It also cannot create one, because the workflow grants only `pull-requests: write`. A label named in `.github/labeler.yml` that does not exist in the repository is therefore not skipped: the action sends the whole set in a single request, so that one name costs the pull request every label it should have had, and the job says so rather than passing quietly. Create the label first, with `gh label list` / `gh label create`.
