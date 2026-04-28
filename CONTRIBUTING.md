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
2. Create a `.env` file with your GitLab credentials (see [Configuration](docs/configuration.md))
3. Run `make build` to verify the setup
4. Run `make test` to ensure all tests pass

## Development Setup

### Prerequisites

- **Go 1.26+** — [Download](https://go.dev/dl/)
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

# Run end-to-end tests in Docker mode (ephemeral GitLab CE, ~4 GB RAM)
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
test(sampling): increase coverage to 90%
docs(readme): update tool count after wiki tools
```

## Pull Requests

### Before Submitting

- [ ] Code compiles: `go build ./...`
- [ ] All tests pass: `go test ./... -count=1`
- [ ] Static analysis is clean: `make analyze` (run `make analyze-fix` first to auto-fix `goimports`/`gofmt`/`modernize` issues)
- [ ] New tools have tests with >80% coverage
- [ ] Documentation is updated if public API changed
- [ ] Commit messages follow conventional commits

### PR Process

1. Create a feature branch from `main`
2. Make your changes in small, focused commits
3. Push the branch and open a pull request — reviewers are auto-requested via [CODEOWNERS](CODEOWNERS)
4. Fill in the PR template (auto-populated from `.github/pull_request_template.md`)
5. Address review feedback
6. Squash-merge once approved (the only allowed merge strategy)

### PR Size Guidelines

- **Small** (<200 lines): Preferred — faster review, fewer conflicts
- **Medium** (200–500 lines): Acceptable for feature additions
- **Large** (>500 lines): Split into smaller PRs when possible

## Code Standards

### Go Conventions

- Follow idiomatic Go: `gofmt`, `goimports`, `go vet`, `staticcheck`
- All exported types and functions must have doc comments
- Error wrapping with `fmt.Errorf("context: %w", err)`
- Use `context.Context` consistently for cancellation/timeouts
- Table-driven tests with `t.Run()` subtests

### MCP Tool Patterns

- Each GitLab operation = one tool with typed input/output structs
- Use `jsonschema` struct tags for tool input documentation
- Register tools via `mcp.AddTool()` with descriptive names
- Set appropriate annotations (readOnlyHint, destructiveHint, etc.)
- Return both structured JSON and human-readable Markdown

### File Organization

```text
internal/tools/
├── register.go              # RegisterAll() — delegates to sub-package RegisterTools()
├── register_meta.go         # RegisterAllMeta() — meta-tool registration
├── metatool.go              # Meta-tool registration infrastructure
├── pagination.go            # Pagination type aliases
├── errors.go                # Error helpers (bridge to toolutil)
├── markdown.go              # Markdown formatting (bridge to toolutil)
├── logging.go               # Tool call logging (bridge to toolutil)
└── <domain>/                # 162 domain sub-packages
    ├── register.go          # RegisterTools() for this domain
    ├── <domain>.go          # Typed input/output structs + handlers
    ├── <domain>_test.go     # Table-driven unit tests
    └── markdown.go          # Markdown formatters (self-registered via init())
```

## Testing

### Requirements

- **Unit tests** for every tool handler — use `httptest` to mock GitLab API responses
- **Table-driven tests** with `t.Run()` subtests
- **Test naming**: `TestToolName_Scenario_ExpectedResult`
- **Coverage target**: >80% on tool handlers
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
go test -tags e2e -timeout 300s ./test/e2e/suite/
```

## Documentation

### AI-Assisted Development

This project ships with **7 AI agents** and **18 skills** for GitHub Copilot and compatible assistants. Key workflows for contributors:

- **Adding new tools**: Use the `create-mcp-tool` skill — it scaffolds the full tool lifecycle (struct, handler, registration, tests, docs).
- **Improving test coverage**: Use the `increase-test-coverage` skill to identify gaps and reach the 80% coverage target.
- **Code quality reviews**: Use the `review-and-refactor` skill for code quality + OWASP security + MCP pattern checks.

See [AGENTS.md](AGENTS.md) for the complete catalog of agents, skills, and instruction files.

### Snapshot Testing (Golden Files)

Tool definitions are snapshot-tested to detect unintentional changes. Golden files live in `internal/tools/testdata/`:

- `tools_individual.json` — all individual tool definitions
- `tools_meta.json` — all meta-tool definitions

When you intentionally change a tool definition (name, description, schema, annotations), update the golden files:

```bash
UPDATE_TOOLSNAPS=true go test ./internal/tools/ -run TestToolSnapshots -count=1
```

Then commit the updated golden files alongside your code changes. The CI will fail if snapshots are out of date.

### When to Update

- Adding a new tool → update the relevant `docs/tools/<domain>.md` and `docs/tools/README.md`
- Adding a new meta-tool action → update `docs/meta-tools.md`
- Adding a new resource → update `docs/resources-reference.md`
- Adding a new prompt → update `docs/prompts-reference.md`
- Adding a new capability → update `docs/capabilities.md`
- Changing configuration → update `docs/configuration.md`
- Adding or modifying tests → update `docs/development/testing.md` with new test counts and coverage values

### Language Policy

All project artifacts must be written in **English**:

- Source code, comments, doc comments
- Commit messages, branch names
- Documentation, ADRs, specs
- MCP tool names, descriptions, error messages
- Test names and assertions

## Release Process

When creating a new release and uploading binaries to GitHub Releases:

1. Build cross-platform binaries with `make release` (uses GoReleaser locally, flattens `dist/` to match GitHub Release asset names)
2. Create a GitHub release with the new tag and upload the binaries + checksum

## Issue Reporting

Open an issue at <https://github.com/jmrplens/gitlab-mcp-server/issues/new/choose> and pick a template:

- **Bug Report** — reproducible defects
- **Feature Request** — new functionality / new MCP tool
- **Enhancement** — improvement to existing behavior
- **Documentation** — missing, outdated or incorrect docs

For **security issues**, do not open a public issue — report privately via [GitHub Security Advisories](https://github.com/jmrplens/gitlab-mcp-server/security/advisories/new) (see [SECURITY.md](SECURITY.md)).

Templates auto-apply the relevant labels listed in [Labels](#labels).

## Labels

Issue templates auto-assign labels on submission. The repo uses a flat label set (no `type::`/`priority::` namespaces — those are GitLab conventions):

| Label              | Color     | Used by                                          |
| ------------------ | --------- | ------------------------------------------------ |
| `bug`              | `#d73a4a` | Bug Report template                              |
| `feature`          | `#a2eeef` | Feature Request template                         |
| `enhancement`      | `#a2eeef` | Enhancement template (GitHub default)            |
| `documentation`    | `#0075ca` | Documentation template (GitHub default)          |
| `security`         | `#d73a4a` | Manual — applied to GitHub Security Advisories   |
| `high-priority`    | `#b60205` | Manual — critical bugs and security advisories   |
| `needs-triage`     | `#c2e0c6` | All issue templates (auto-applied on submission) |
| `good first issue` | `#7057ff` | Manual — newcomer-friendly issues                |
| `help wanted`      | `#008672` | Manual — community contributions welcome         |
| `question`         | `#d876e3` | Manual — questions / discussions                 |
| `duplicate`        | `#cfd3d7` | Manual — duplicates of existing issues           |
| `invalid`          | `#e4e669` | Manual — out of scope                            |
| `wontfix`          | `#ffffff` | Manual — accepted but won't implement            |

Manage labels with `gh label list` / `gh label create`.
