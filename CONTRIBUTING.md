# Contributing to gitlab-mcp-server

Thank you for your interest in contributing to gitlab-mcp-server! This guide covers the process for submitting changes, reporting issues, and following project conventions.

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
- [ ] No lint issues: `go vet ./...` and `staticcheck ./...`
- [ ] Code is formatted: `gofmt -w .` and `goimports -w .`
- [ ] New tools have tests with >80% coverage
- [ ] Documentation is updated if public API changed
- [ ] Commit messages follow conventional commits

### PR Process

1. Create a feature branch from `main`
2. Make your changes in small, focused commits
3. Push the branch and create a pull request
4. Fill in the PR template (auto-populated from `.github/pull_request_template.md`)
5. Assign to `@jmrplens` for review
6. Address review feedback
7. Squash and merge once approved

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

1. Build cross-platform binaries with `scripts/build-release.ps1` (Windows) or `scripts/build-release.sh` (Linux/macOS)
2. Create a GitHub release with the new tag and upload the binaries + checksum

## Issue Reporting

### Bug Reports

Use the **Bug Report** issue template. Include:

1. Steps to reproduce
2. Expected behavior
3. Actual behavior
4. Environment (Go version, OS, GitLab version)
5. Relevant logs or error messages

### Feature Requests

Use the **Feature Request** issue template. Include:

1. Problem description
2. Proposed solution
3. Use cases
4. Alternatives considered

## Labels

Issues and pull requests use the following labels for categorization:

### Type Labels

| Label                 | Color     | Description                           |
| --------------------- | --------- | ------------------------------------- |
| `type::bug`           | `#d73a4a` | Something is broken                   |
| `type::feature`       | `#a2eeef` | New functionality                     |
| `type::enhancement`   | `#7057ff` | Improvement to existing functionality |
| `type::documentation` | `#0075ca` | Documentation changes                 |
| `type::refactor`      | `#e4e669` | Code restructuring                    |
| `type::test`          | `#bfd4f2` | Test additions or improvements        |
| `type::chore`         | `#d4c5f9` | Maintenance, dependencies, CI         |

### Priority Labels

| Label                | Color     | Description          |
| -------------------- | --------- | -------------------- |
| `priority::critical` | `#b60205` | Must fix immediately |
| `priority::high`     | `#d93f0b` | Important, fix soon  |
| `priority::medium`   | `#fbca04` | Normal priority      |
| `priority::low`      | `#0e8a16` | Nice to have         |

### Status Labels

| Label                  | Color     | Description               |
| ---------------------- | --------- | ------------------------- |
| `status::needs-triage` | `#c2e0c6` | Needs initial assessment  |
| `status::ready`        | `#0075ca` | Ready for implementation  |
| `status::in-progress`  | `#fbca04` | Currently being worked on |
| `status::blocked`      | `#d73a4a` | Blocked by dependency     |
| `status::needs-review` | `#7057ff` | Waiting for code review   |

### Component Labels

| Label                      | Color     | Description                                |
| -------------------------- | --------- | ------------------------------------------ |
| `component::tools`         | `#1d76db` | MCP tool handlers                          |
| `component::resources`     | `#1d76db` | MCP resource handlers                      |
| `component::prompts`       | `#1d76db` | MCP prompt handlers                        |
| `component::capabilities`  | `#1d76db` | MCP capabilities (logging, sampling, etc.) |
| `component::config`        | `#1d76db` | Configuration and environment              |
| `component::gitlab-client` | `#1d76db` | GitLab API client wrapper                  |
| `component::ci-cd`         | `#1d76db` | CI/CD pipeline and build                   |
| `component::docs`          | `#1d76db` | Documentation                              |

### Scope Labels

| Label                    | Color     | Description                 |
| ------------------------ | --------- | --------------------------- |
| `scope::breaking-change` | `#b60205` | Contains breaking changes   |
| `scope::security`        | `#d73a4a` | Security-related            |
| `scope::performance`     | `#e4e669` | Performance impact          |
| `scope::ux`              | `#a2eeef` | User experience improvement |
