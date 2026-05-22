# Static Analysis Tools

This document describes the static analysis gates used in **gitlab-mcp-server**, their configuration, and how to run them.

> **Diataxis type**: Reference
> **Audience**: Developers, contributors
> **Prerequisites**: Go toolchain installed, Make optional. Make targets export the project Go toolchain from `go.mod` by default.

---

## Overview

The project uses three complementary analysis surfaces:

| Tool                | Purpose                                  | Auto-fix      | Config                     | Docs                                                               |
| ------------------- | ---------------------------------------- | ------------- | -------------------------- | ------------------------------------------------------------------ |
| `golangci-lint`     | Go linting plus configured Go formatters | Partial       | `.golangci.yml`            | [golangci-lint.run](https://golangci-lint.run/)                    |
| `govulncheck`       | Go dependency and reachable CVE scanner  | No            | N/A                        | [pkg.go.dev](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck) |
| `markdownlint-cli2` | Markdown lint and auto-fix               | Yes (`--fix`) | `.markdownlint-cli2.jsonc` | [github.com](https://github.com/DavidAnson/markdownlint-cli2)      |

Standalone Go tools that are already executed through `golangci-lint` are not run separately in Make or CI. This avoids duplicate work, divergent flags, and inconsistent findings. The consolidated Go gate covers `govet`, `modernize`, `gosec`, `staticcheck`, `goimports`, `gofumpt`, and `gci` through `.golangci.yml`.

Go analysis targets pass the `e2e` build tag so files under `test/e2e/` are included without running E2E tests. Markdown linting remains repository-wide for Markdown files, excluding `plan/` drafts in Make targets.

## Quick Start

```bash
# Install required command-line tools once
make install-tools

# Run the complete analysis suite
make analyze

# Override the toolchain only when debugging local Go installations
make GOTOOLCHAIN=auto analyze

# Generate an LLM-consumable report file
make analyze-report
# Output: dist/analysis/report.txt

# Apply automatic fixes from configured Go formatters/linters and markdownlint
make analyze-fix
```

## Markdown Table Formatting

Source documentation tables can be normalized with the dedicated formatter:

```bash
go run ./cmd/format_md_tables/
go run ./cmd/format_md_tables/ --check
```

The command scans `README.md` and `docs/` by default, skips fenced code blocks, preserves left/right/center alignment markers, and pads table columns for readable source Markdown. Use `--check` in review or CI contexts when you want a non-writing verification pass.

## Tool Installation

All Go tools install into `$GOBIN`, usually `$GOPATH/bin`:

```bash
make install-tools
```

This installs:

| Tool          | Install command                                                            | Version |
| ------------- | -------------------------------------------------------------------------- | ------- |
| golangci-lint | `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest` | v2.11+  |
| govulncheck   | `go install golang.org/x/vuln/cmd/govulncheck@latest`                      | v1.1+   |
| gotestsum     | `go install gotest.tools/gotestsum@latest`                                 | latest  |

Verify installation:

```bash
golangci-lint version
govulncheck -version
gotestsum --version
```

`markdownlint-cli2` is run through `npx`, so no global Node installation step is required beyond a working Node/npm environment.

## Makefile Targets

### Individual Targets

| Target               | Description                                                                           |
| -------------------- | ------------------------------------------------------------------------------------- |
| `make golangci-lint` | Verify `.golangci.yml`, check configured Go formatting, and run configured Go linters |
| `make fmt`           | Apply configured Go formatters through `golangci-lint fmt`                            |
| `make govulncheck`   | Scan Go dependencies and reachable calls for known CVEs                               |
| `make mdlint`        | Lint all Markdown files, excluding `plan/`                                            |
| `make mdlint-fix`    | Auto-fix Markdown lint issues                                                         |

### Combined Targets

| Target                | Description                                                                                                           |
| --------------------- | --------------------------------------------------------------------------------------------------------------------- |
| `make analyze`        | Run `golangci-lint config verify`, `golangci-lint fmt --diff`, `golangci-lint run`, `govulncheck`, and `markdownlint` |
| `make analyze-fix`    | Apply auto-fixes with `golangci-lint fmt`, `golangci-lint run --fix`, and `markdownlint --fix`                        |
| `make analyze-report` | Generate a combined Markdown report at `dist/analysis/report.txt`                                                     |
| `make lint`           | Backward-compatible alias for `make golangci-lint`                                                                    |

### Project Audit Targets

| Target                            | Description                                                                                      |
| --------------------------------- | ------------------------------------------------------------------------------------------------ |
| `make audit-output`               | Run the MCP output quality audit on all tools                                                    |
| `make audit-tokens`               | Measure exposed tool token overhead                                                              |
| `make audit-tools`                | Audit MCP tool metadata violations                                                               |
| `make audit-metrics`              | Report MCP tool/resource/prompt counts                                                           |
| `make audit-action-spec-coverage` | Generate ActionSpec surface coverage inventory in `dist/action-spec-coverage.json`               |
| `make audit-dynamic-aliases`      | Audit Dynamic search aliases and canonical action reachability                                   |
| `make audit-test-names`           | Audit test function naming convention compliance                                                 |
| `make audit-godocs`               | Generate `dist/analysis/godoc.md` with package, exported symbol, and test documentation findings |
| `make audit-godocs-check`         | Run the same Godoc audit and fail if findings remain                                             |

## Tool Details

### golangci-lint

`golangci-lint` is the canonical Go analysis gate. It runs configured linters and formatters from [`.golangci.yml`](../../.golangci.yml), including tools that were previously run as standalone Make/CI jobs.

```bash
make golangci-lint
golangci-lint config verify
golangci-lint fmt --diff
golangci-lint run --build-tags e2e ./...
```

The Make target performs these steps:

1. Validate `.golangci.yml`.
2. Check configured Go formatters with `golangci-lint fmt --diff`.
3. Run configured linters with the `e2e` build tag.

Configured formatters:

- `goimports` for import grouping and ordering.
- `gofumpt` for stricter gofmt-compatible formatting.
- `gci` for deterministic import section grouping.

Key configured linters include:

- `govet` with all checks enabled except `fieldalignment`.
- `staticcheck` with all checks enabled.
- `gosec` with audit mode enabled and `G104` excluded because unchecked errors are covered by `errcheck`.
- `modernize` for modern Go idiom suggestions.
- `errcheck`, `bodyclose`, `noctx`, `nilerr`, `nilnil`, `errorlint`, `gocyclo`, `gocognit`, `nestif`, `maintidx`, `dupl`, `revive`, `gocritic`, `nolintlint`, `usetesting`, `perfsprint`, and related checks.

Standalone `go vet`, `modernize`, `gosec`, `staticcheck`, `goimports`, and `gofmt` checks are intentionally not duplicated in Make or CI. Their checks are represented by the configured `golangci-lint` run and formatter pass.

### govulncheck

`govulncheck` scans Go dependencies for known vulnerabilities and uses call graph analysis to report vulnerabilities reachable from the codebase.

```bash
make govulncheck
govulncheck -tags e2e ./...
```

It remains separate because it is a vulnerability database scanner, not a normal lint rule inside `golangci-lint`.

### markdownlint-cli2

`markdownlint-cli2` checks Markdown style and consistency.

```bash
make mdlint
make mdlint-fix
```

Make excludes `plan/` because it contains working drafts that are not versioned as polished documentation.

## CI Integration

GitHub Actions uses the same separation as Make:

- `golangci-lint` job installs `golangci-lint` and runs `make golangci-lint`.
- `govulncheck` job installs `govulncheck` and runs `make govulncheck`.
- `Analyze Markdown` runs `markdownlint-cli2` for Markdown and MDX content.

Separate jobs for `goimports`, `gofmt`, `go vet`, `modernize`, `gosec`, and `staticcheck` are intentionally omitted because `golangci-lint` already covers them with the repository configuration.

## GitLab CI Example

```yaml
golangci-lint:
  stage: lint
  script: make golangci-lint

govulncheck:
  stage: lint
  script: make govulncheck

markdownlint:
  stage: lint
  script: make mdlint
```

## Troubleshooting

### Tool not found

Ensure `$GOPATH/bin` or `$GOBIN` is in your `PATH`:

```bash
go env GOPATH GOBIN
export PATH="$(go env GOPATH)/bin:$PATH"
```

Then install tools:

```bash
make install-tools
```

### golangci-lint timeout

The configured timeout is in [`.golangci.yml`](../../.golangci.yml). For one-off local debugging, run with an explicit timeout:

```bash
golangci-lint run --timeout 20m ./...
```

### Need a standalone tool during investigation

Use standalone commands temporarily for debugging if they provide more focused output, but do not add them back to `make analyze` or CI when the same check is already enforced through `golangci-lint`.
