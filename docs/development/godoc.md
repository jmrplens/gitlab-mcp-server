# Godoc Compliance

> **Diátaxis type**: Reference
> **Audience**: 🔧 Developers, contributors
> **Prerequisites**: Go toolchain installed, Make optional

This project treats Go documentation as part of the public development surface. The module is primarily an
executable, but pkg.go.dev still indexes `cmd/`, `internal/`, and `test/` packages, so package comments,
exported symbols, and high-value test helpers should be readable in generated documentation.

## Quick Start

```bash
# Generate a Markdown report, including _test.go functions
make audit-godocs
# Output: dist/analysis/godoc.md

# Fail when any Godoc findings remain
make audit-godocs-check

# Serve local pkg.go.dev-style documentation
make docs-local-go
# URL: http://127.0.0.1:6060
```

If `pkgsite` is not installed, install it first:

```bash
go install golang.org/x/pkgsite/cmd/pkgsite@latest
```

## Audit Scope

The `cmd/audit_godocs` command scans every Go package returned by `go list ./...`.

By default, it checks:

- Package comments on non-test Go files
- Exported functions and methods
- Exported types
- Exported constants and variables

With `--include-tests`, it also checks:

- `Test*` and `TestMain` functions
- `Benchmark*` functions
- `Fuzz*` functions
- `Example*` functions and their output comments

## Documentation Rules

Package comments must be attached directly to the `package` clause and start with the correct Go convention:

```go
// Package config loads and validates runtime configuration.
package config
```

Command packages use `Command` instead of `Package`:

```go
// Command audit_godocs audits Go package and symbol documentation.
package main
```

Exported symbol comments must start with the exported identifier:

```go
// Config contains runtime settings loaded from flags and environment variables.
type Config struct {
    // ...
}
```

Test function comments should also start with the function name when the test needs to appear clearly in Godoc or
the audit report:

```go
// TestLoadConfig_MissingToken_ReturnsError verifies that configuration loading fails without credentials.
func TestLoadConfig_MissingToken_ReturnsError(t *testing.T) {
    // ...
}
```

## Common Package Comment Pitfall

Go treats any leading comment immediately before `package` as package documentation. This means a file-level
comment can accidentally become package documentation:

```go
// parser.go contains helpers for parsing GitLab URLs.
package projectdiscovery
```

That comment is not a harmless file header. It becomes a malformed package comment in Godoc and can also create
multiple package comments when the package already has a correct `doc.go`.

Prefer one canonical `doc.go` package comment, then remove package-adjacent file comments from other files.

## CLI Reference

```bash
go run ./cmd/audit_godocs/ [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--format` | `markdown` | Report format: `markdown` or `json` |
| `--output` | stdout | Write the report to a file |
| `--include-tests` | `false` | Include `Test`, `Benchmark`, `Fuzz`, and `Example` functions |
| `--fail-on-findings` | `false` | Exit non-zero when findings are present |
| `--ignore-internal` | `false` | Skip packages whose import path contains `/internal/` |

The non-failing target is intended for remediation work while a baseline exists. The check target is intended for
future CI gating once the baseline is cleared.
