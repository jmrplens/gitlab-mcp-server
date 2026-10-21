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

# Fail on a package or exported-symbol finding (the CI gate)
make check-godocs

# Fail when any finding remains, test functions included (not a gate yet)
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

The `cmd/godoc_tool audit` subcommand scans every Go package returned by `go list ./...`.

By default, it checks:

- Package comments on non-test Go files
- Exported functions and methods
- Exported types
- Exported constants and variables: a declaration of one exported name needs a comment that opens with it, on the
  group or, in a group that has none, on the spec declaring the name; a declaration of several needs a comment on the
  group, whatever its form, or a doc comment on each exported name that opens with it (`const_group_missing` and
  `var_group_missing` for a name with none, the form finding for one that does not open with its name). A trailing
  line comment never counts

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
// Command godoc_tool audits Go package and symbol documentation.
package main
```

Exported symbol comments must start with the exported identifier:

```go
// Config contains runtime settings loaded from flags and environment variables.
type Config struct {
    // ...
}
```

A const or var group that declares several exported names takes one comment for the set, which need not open with
any one of them, unless every exported name in it has a doc comment of its own that opens with it:

```go
// The states a sample workflow can be in.
const (
    StateOpen   = "open"
    StateClosed = "closed"
)
```

A group that declares one exported name and has no group comment is judged by the doc comment on that name's spec,
which must open with the name, as revive's `exported` rule judges it. A trailing line comment is never read as
documentation, on one name or on several.

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
multiple package comments when the package already has a correct `doc.go`. A blank line between the comment and the
`package` clause keeps it as the file header it was meant to be.

The package comment has one home: `doc.go`. The audit reports a well-formed package comment that lives in any other file (`pkgdoc_location`), because a comment there is one file header away from being mistaken for one and one more file away from being duplicated. `go run ./cmd/godoc_tool/ fix --move-package-doc <path>` moves it: the comment lands verbatim above the package clause of a new `doc.go`, and the file it came from keeps everything else, its build constraint included.

## CLI Reference

```bash
go run ./cmd/godoc_tool/ audit [flags]
```

| Flag                 | Default    | Description                                                  |
| -------------------- | ---------- | ------------------------------------------------------------ |
| `--format`           | `markdown` | Report format: `markdown` or `json`                          |
| `--output`           | stdout     | Write the report to a file                                   |
| `--include-tests`    | `false`    | Include `Test`, `Benchmark`, `Fuzz`, and `Example` functions |
| `--fail-on-findings` | `false`    | Exit non-zero when findings are present                      |
| `--ignore-internal`  | `false`    | Skip packages whose import path contains `/internal/`        |

`make check-godocs` is the gate: it runs the audit without `--include-tests` and with `--fail-on-findings`, as a step
of CI's `generated` job and as step 31 of `make analyze`. golangci-lint cannot stand in for it, because the `comments`
exclusion preset in `.golangci.yml` silences revive's `exported` and `package-comments` rules and staticcheck's ST1000
and ST1020 to ST1022.
`make audit-godocs-check` adds `--include-tests` and stays a report until the test functions it finds have a comment
that opens with their name; `make audit-godocs` writes the same findings to `dist/analysis/godoc.md`.
