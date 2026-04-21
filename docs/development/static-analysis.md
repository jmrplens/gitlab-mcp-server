# Static Analysis Tools

This document describes all static analysis tools used in **gitlab-mcp-server**, their configuration, and how to run them.

> **Diátaxis type**: Reference
> **Audience**: 🔧 Developers, contributors
> **Prerequisites**: Go toolchain installed, Make (optional)

---

## Overview

The project uses nine complementary static analysis tools:

| Tool | Purpose | Auto-fix | Config | Docs |
| --- | --- | --- | --- | --- |
| `goimports` | Import ordering/grouping + gofmt formatting | Yes (`-w`) | N/A | [pkg.go.dev](https://pkg.go.dev/golang.org/x/tools/cmd/goimports) |
| `gofmt` | Official Go code formatter (canonical style) | Yes (`-w`) | N/A | [pkg.go.dev](https://pkg.go.dev/cmd/gofmt) |
| `go vet` | Built-in Go bug detection | No | N/A | [pkg.go.dev](https://pkg.go.dev/cmd/vet) |
| `modernize` | Modern Go idiom suggestions (Go 1.18–1.26) | Yes (`-fix`) | N/A | [pkg.go.dev](https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/modernize) |
| `golangci-lint` | Meta-linter with 100+ checks | Partial | `.golangci.yml` | [golangci-lint.run](https://golangci-lint.run/) |
| `gosec` | OWASP-oriented security scanner | No | `.golangci.yml` | [github.com](https://github.com/securego/gosec) |
| `staticcheck` | Advanced bug/deprecation/simplification analysis | No | `.golangci.yml` | [staticcheck.dev](https://staticcheck.dev/) |
| `govulncheck` | Dependency CVE scanner | No | N/A | [pkg.go.dev](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck) |
| `markdownlint-cli2` | Markdown lint and auto-fix | Yes (`--fix`) | `.markdownlint-cli2.jsonc` | [github.com](https://github.com/DavidAnson/markdownlint-cli2) |

## Quick Start

```bash
# Install all tools (one-time)
make install-tools

# Run ALL analysis tools at once (9 tools)
make analyze

# Generate LLM-consumable report file
make analyze-report
# Output: dist/analysis/report.txt

# Apply automatic fixes (goimports + gofmt + modernize + markdownlint)
make analyze-fix
```

## Tool Installation

All tools install into `$GOBIN` (usually `$GOPATH/bin`):

```bash
make install-tools
```

This installs:

| Tool | Install Command | Version |
| --- | --- | --- |
| modernize | `go install golang.org/x/tools/go/analysis/passes/modernize/cmd/modernize@latest` | latest |
| golangci-lint | `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest` | v2.11+ |
| gosec | `go install github.com/securego/gosec/v2/cmd/gosec@latest` | v2.24+ |
| staticcheck | `go install honnef.co/go/tools/cmd/staticcheck@latest` | 2026.1+ |
| govulncheck | `go install golang.org/x/vuln/cmd/govulncheck@latest` | v1.1+ |
| goimports | `go install golang.org/x/tools/cmd/goimports@latest` | latest |

Verify installation:

```bash
modernize -h
golangci-lint version
gosec -version
staticcheck -version
govulncheck -version
```

## Makefile Targets

### Individual Targets

| Target | Description |
| --- | --- |
| `make goimports` | Apply goimports formatting to all Go files |
| `make goimports-check` | Check if goimports formatting is needed (no changes) |
| `make gofmt-check` | Check if gofmt formatting is needed (no changes) |
| `make fmt` | Apply gofmt formatting to all Go files |
| `make vet` | Run `go vet` on all packages |
| `make modernize` | Report modernization suggestions |
| `make modernize-fix` | Apply modernization fixes automatically |
| `make golangci-lint` | Run golangci-lint (reads `.golangci.yml`) |
| `make gosec` | Run security scanner (medium+ severity/confidence) |
| `make staticcheck` | Run static analysis checks |
| `make govulncheck` | Scan dependencies for known CVEs |
| `make mdlint` | Lint all Markdown files (excludes `plan/`) |
| `make mdlint-fix` | Auto-fix Markdown lint issues |

### Combined Targets

| Target | Description |
| --- | --- |
| `make analyze` | Run ALL 9 tools sequentially, continue on errors |
| `make analyze-fix` | Apply auto-fixes: goimports + gofmt + modernize + markdownlint |
| `make analyze-report` | Generate combined report to `dist/analysis/report.txt` |
| `make lint` | Quick lint (go vet only, backward compatible) |

### LLM Workflow

For AI-assisted code correction:

```bash
# 1. Generate the report
make analyze-report

# 2. Feed the report to an LLM
#    The file dist/analysis/report.txt is formatted in Markdown
#    with sections for each tool, ready for LLM consumption.

# 3. After LLM applies fixes, verify
make analyze
```

## Tool Details

### 1. goimports

**What it does**: Applies `gofmt` formatting **plus** organizes import statements (groups, ordering, removes unused, adds missing).

```bash
make goimports        # Apply formatting (writes files)
make goimports-check  # Check only (no changes, lists files needing fixes)
```

Features:

- All `gofmt` formatting rules
- Groups imports: stdlib, external, local (project module)
- Removes unused imports
- Adds missing imports automatically
- Local prefix configured via golangci-lint: project module path

**Docs**: <https://pkg.go.dev/golang.org/x/tools/cmd/goimports>

### 2. gofmt

**What it does**: The official Go code formatter. All Go code **must** pass `gofmt` — this is a hard requirement in the Go ecosystem.

```bash
make fmt              # Apply formatting (writes files)
make gofmt-check      # Check only (no changes, lists files needing fixes)
```

Features:

- Canonical indentation (tabs)
- Consistent spacing around operators and keywords
- Simplified code with `-s` flag (simplify composite literals, slice expressions)
- Deterministic output — same input always produces same output

**No configuration** — the format is fixed by design (no style debates).

**Docs**: <https://pkg.go.dev/cmd/gofmt>

### 3. go vet

**What it checks**: Built-in Go compiler-adjacent checks for suspicious constructs.

```bash
make vet
```

Detects:

- Printf format string mismatches
- Unreachable code
- Shadowed variables
- Incorrect struct tags
- Copy of sync types
- Unused results of certain function calls

**No configuration needed** — part of the Go toolchain.

**Docs**: <https://pkg.go.dev/cmd/vet>

### 4. modernize

**What it checks**: Code that can use newer Go language features.

```bash
make modernize        # Report only
make modernize-fix    # Apply fixes
```

Suggests replacing:

- `sort.Slice(s, func(i, j int) bool { ... })` → `slices.SortFunc(s, ...)`
- `for i := 0; i < len(s); i++ { ... }` → `for i, v := range s { ... }`
- `strings.HasPrefix(s, p) && s[len(p):]` → `strings.CutPrefix(s, p)`
- `if err != nil { return err }` → `errors.Join()` patterns
- `append([]T(nil), s...)` → `slices.Clone(s)`
- Many more Go 1.18–1.26 modernization patterns

**Docs**: <https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/modernize>

### 5. golangci-lint

**What it checks**: Meta-linter that orchestrates 25+ linters through `.golangci.yml`.

```bash
make golangci-lint
```

Configuration file: [`.golangci.yml`](../../.golangci.yml) (v2 format)

**Docs**: <https://golangci-lint.run/>

#### Enabled Linters

**Bug Detection**:

- `govet` — Go vet checks (all enabled, except `fieldalignment`)
- `staticcheck` — SA/S/ST/QF checks (all except `ST1000`)
- `errcheck` — Unchecked error returns (type assertions checked)
- `ineffassign` — Ineffectual assignments
- `bodyclose` — HTTP response body not closed (critical for GitLab API calls)
- `noctx` — HTTP requests without `context.Context`
- `durationcheck` — Multiplying two `time.Duration` values
- `nilerr` — Returns `nil` when `err` is not nil
- `nilnil` — Returns `nil, nil` without reason
- `errname` — Sentinel error naming (`ErrFoo`)

**Security** (OWASP):

- `gosec` — G1xx–G7xx security rules (excludes G104, G304)

**Code Quality**:

- `revive` — 23 rules (context-as-argument, error-strings, exported, var-naming, etc.)
- `gocritic` — Diagnostic + performance + style tags
- `unconvert` — Unnecessary type conversions
- `unparam` — Unused function parameters
- `unused` — Unused code
- `prealloc` — Slice pre-allocation hints
- `copyloopvar` — Pre-Go 1.22 loop variable patterns

**Style & Formatting**:

- `misspell` — Spelling mistakes (US locale)
- `godot` — Top-level comments end with period
- `usestdlibvars` — Use stdlib constants over magic values

**Formatters** (separate section in v2):

- `goimports` — Import grouping and ordering (local prefix: project module)

**Performance**:

- `perfsprint` — `fmt.Sprintf` → `strconv` conversions

#### Exclusion Rules

| Scope | Relaxed Linters |
| --- | --- |
| `_test.go` | errcheck, gosec, bodyclose, gocritic, revive, unparam, perfsprint |
| `vendor/` | All linters |
| `cmd/server/main.go` | gosec G114 (os.Exit) |
| `internal/testutil/` | errcheck, gosec |

### 6. gosec

**What it checks**: OWASP-oriented security issues in Go code.

```bash
make gosec
```

Flags: `-severity medium -confidence medium -exclude-generated`

Key rules:

- **G101**: Hardcoded credentials
- **G102**: Binding to all interfaces
- **G107**: URL provided as taint input
- **G201–G204**: SQL injection
- **G301–G307**: File permissions and path traversal
- **G401–G406**: Weak cryptographic algorithms
- **G501–G505**: Blacklisted imports (crypto/md5, etc.)
- **G601**: Data race with implicit memory aliasing (pre-Go 1.22)

**Docs**: <https://github.com/securego/gosec>

> gosec is also integrated into golangci-lint, but running it standalone gives more detailed output and allows different flag combinations.

### 7. staticcheck

**What it checks**: Advanced analysis across multiple categories.

```bash
make staticcheck
```

Check categories:

- **SA** (staticcheck): Bugs, questionable constructs
- **S** (simple): Code simplifications
- **ST** (stylecheck): Style issues
- **QF** (quickfix): Suggested refactorings

Examples:

- `SA1012`: Passing nil context
- `SA4006`: Value is never used
- `SA9003`: Empty body in if/else branch
- `S1025`: Redundant `fmt.Sprintf("%s", x)` when x is already a string
- `QF1001`: Apply De Morgan's law

**Docs**: <https://staticcheck.dev/>

> staticcheck is also integrated into golangci-lint. Running standalone provides slightly different behavior for incremental analysis and caching.

### 8. govulncheck

**What it checks**: Known vulnerabilities (CVEs) in Go dependencies.

```bash
make govulncheck
```

Features:

- Scans `go.mod`/`go.sum` for vulnerable module versions
- Performs **call graph analysis** — only reports if vulnerable function is actually called
- Uses the [Go Vulnerability Database](https://vuln.go.dev/)
- Shows affected function, vulnerability ID, and fix version

Example output:

```text
Vulnerability #1: GO-2024-XXXX
  Found in: golang.org/x/crypto@v0.21.0
  Fixed in: golang.org/x/crypto@v0.22.0
  Details: https://pkg.go.dev/vuln/GO-2024-XXXX
```

**Action**: Update the vulnerable dependency with `go get module@version`.

**Docs**: <https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck> and <https://go.dev/doc/security/vuln/>

## Configuration Reference

### .golangci.yml Structure

```yaml
version: "2"                    # golangci-lint v2 config format

run:
  go: "1.26"                    # Target Go version
  timeout: 10m                  # Analysis timeout
  tests: true                   # Include test files
  build-tags: [e2e]             # Include e2e build tag

linters:
  default: none                 # Start from scratch
  enable: [...]                 # Explicit list of linters
  settings:                     # Per-linter configuration
    govet: { enable-all: true } # etc.

  exclusions:
    presets: [comments, std-error-handling]
    rules:                      # Path-based exclusions
      - path: "_test\\.go"
        linters: [errcheck, gosec, ...]
```

See [`.golangci.yml`](../../.golangci.yml) for the complete file.

### Customizing Rules

To enable/disable a linter:

```yaml
# .golangci.yml
linters:
  enable:
    - newlinter    # Add
  # Remove from the enable list to disable
```

To suppress a specific finding:

```go
// In Go code:
//nolint:gosec // G107: URL from user config, validated upstream
resp, err := http.Get(url)
```

To exclude files/paths:

```yaml
# .golangci.yml
linters:
  exclusions:
    rules:
      - path: "generated/"
        linters: [all]
```

## CI Integration

For CI/CD pipelines, add to your `.gitlab-ci.yml`:

```yaml
lint:
  stage: test
  script:
    - make install-tools
    - make analyze
  allow_failure: false
```

Or as separate jobs for parallel execution:

```yaml
vet:
  stage: lint
  script: make vet

golangci-lint:
  stage: lint
  script: make golangci-lint

gosec:
  stage: lint
  script: make gosec

govulncheck:
  stage: lint
  script: make govulncheck
```

## Troubleshooting

### Tool not found

Ensure `$GOPATH/bin` (or `$GOBIN`) is in your `PATH`:

```bash
# Check GOBIN
go env GOBIN

# Add to PATH (PowerShell)
$env:PATH += ";$(go env GOPATH)\bin"

# Reinstall
make install-tools
```

### golangci-lint timeout

Increase timeout in `.golangci.yml`:

```yaml
run:
  timeout: 15m  # Default: 10m
```

### Too many findings

Start by fixing critical issues first:

```bash
# Only security issues
make gosec

# Only vulnerability issues
make govulncheck

# Then broader checks
make golangci-lint
```

### Suppressing false positives

```go
//nolint:lintername // Reason for suppression
```

Always include a reason when suppressing lint findings. Unreasoned `//nolint` comments should be flagged in code review.

### 7. markdownlint-cli2

**What it checks**: Markdown files for style, consistency, and correctness issues.

```bash
make mdlint        # Report only
make mdlint-fix    # Apply fixes
```

Configuration file: [`.markdownlint-cli2.jsonc`](../../.markdownlint-cli2.jsonc)

Runs via `npx` (Node.js required, no global install needed).

#### Key Rules

| Rule | Description |
| --- | --- |
| MD001 | Heading levels should only increment by one |
| MD003 | Heading style should be consistent |
| MD009 | Trailing spaces |
| MD010 | Hard tabs |
| MD012 | Multiple consecutive blank lines |
| MD022 | Headings should be surrounded by blank lines |
| MD031 | Fenced code blocks should be surrounded by blank lines |
| MD032 | Lists should be surrounded by blank lines |
| MD034 | Bare URLs without angle brackets or links |
| MD047 | Files should end with a single newline character |

Full rule list: [markdownlint rules](https://github.com/DavidAnson/markdownlint/blob/main/doc/Rules.md)

**Docs**: <https://github.com/DavidAnson/markdownlint-cli2>

#### Disabled Rules

| Rule | Reason |
| --- | --- |
| MD013 | Line length — many tables and code blocks exceed 80 chars |
| MD024 | Multiple headings with same content — common in changelogs |
| MD025 | Single top-level heading — some docs have multiple H1 by design |
| MD033 | Inline HTML — Mermaid diagrams, badges, details/summary |
| MD041 | First line heading — files with front matter |
| MD060 | Native syntax over HTML — disabled |

#### Ignored Paths

- `plan/**` — implementation plans (working drafts, not published docs)
- `node_modules/**`, `dist/**`, `vendor/**`

#### Suppressing Inline

```markdown
<!-- markdownlint-disable MD033 -->
<details><summary>Click to expand</summary>

Content here.

</details>
<!-- markdownlint-enable MD033 -->
```

Or for a single line:

```markdown
<!-- markdownlint-disable-next-line MD013 -->
| Very long table row that exceeds the line length limit but is necessary for readability |
```

#### VS Code Integration

The project recommends the [markdownlint VS Code extension](https://marketplace.visualstudio.com/items?itemName=DavidAnson.vscode-markdownlint) (`davidanson.vscode-markdownlint`). It reads the same `.markdownlint-cli2.jsonc` config, providing real-time linting in the editor.

Configured in `.vscode/settings.json`:

```jsonc
"[markdown]": {
  "editor.defaultFormatter": "DavidAnson.vscode-markdownlint",
  "editor.codeActionsOnSave": {
    "source.fixAll.markdownlint": "explicit"
  }
}
```
