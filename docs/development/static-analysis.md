# Static Analysis Tools

This document describes the static analysis gates used in **gitlab-mcp-server**, their configuration, and how to run them.

> **Diátaxis type**: Reference
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

Go analysis targets pass every end-to-end build tag (`e2e,collectore2e,httpe2e,orbitlive,stdioe2e`, the Makefile's `GO_ANALYSIS_TAGS`) so every tagged suite under `test/e2e/` is analysed without being run; a suite added behind a new tag is invisible to `go vet` and `golangci-lint` until its tag is added to that list. Markdown linting remains repository-wide for Markdown files, excluding `plan/` drafts in Make targets.

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

The command scans `README.md`, `docs/` and `site/src/content/docs/` by default, skips fenced code blocks, preserves left/right/center alignment markers, and pads table columns for readable source Markdown. Use `--check` in review or CI contexts when you want a non-writing verification pass.

## Tool Installation

All Go tools install into `$GOBIN`, usually `$GOPATH/bin`:

```bash
make install-tools
```

This installs:

| Tool          | Install command                                                             | Version                                                                                                                                  |
| ------------- | --------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------- |
| golangci-lint | `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.1` | v2.13.1 is what CI runs (`GOLANGCI_LINT_VERSION` in the Makefile); a newer release may report findings CI does not, or miss ones it does |
| govulncheck   | `go install golang.org/x/vuln/cmd/govulncheck@<version>`                    | the version the `tool` directive in `go.mod` names is what CI runs; install that one, or run `go tool govulncheck` and let Go resolve it |
| gotestsum     | `go install gotest.tools/gotestsum@latest`                                  | latest                                                                                                                                   |

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

| Target                | Description                                                                                                                                                                                                                                            |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `make analyze`        | Run `golangci-lint config verify`, `golangci-lint fmt --diff`, `golangci-lint run`, `govulncheck`, `markdownlint`, the test-goroutine abort gate, the subtest gate, the supply-chain policy gate, and the two GraphQL gates (pinned schema, documents) |
| `make analyze-fix`    | Apply auto-fixes with `golangci-lint fmt`, `golangci-lint run --fix`, and `markdownlint --fix`                                                                                                                                                         |
| `make analyze-report` | Generate a combined Markdown report at `dist/analysis/report.txt`                                                                                                                                                                                      |
| `make lint`           | Backward-compatible alias for `make golangci-lint`                                                                                                                                                                                                     |

### Project Audit Targets

> Full CLI reference (flags, usage, output formats): [cmd-utilities.md](cmd-utilities.md)

| Target                              | Description                                                                                                                                                                                                                                           |
| ----------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `make audit-surface-quality`        | Consolidated MCP surface audit (metadata + output quality)                                                                                                                                                                                            |
| `make audit-tokens`                 | Measure exposed tool token overhead (`--compare-schemas` for meta-tool sizing spike)                                                                                                                                                                  |
| `make audit-metrics`                | Report MCP tool/resource/prompt counts                                                                                                                                                                                                                |
| `make audit-1to1`                   | Consolidated 1:1 SDK↔API parity audit (struct/action/metadata/enum streams + merged backlog), then the SDK parity gate                                                                                                                                |
| `make audit-1to1-sdk`               | Gate every client-go service, raw-GraphQL operation and enum value on a decision; fails on a finding                                                                                                                                                  |
| `make audit-1to1-enums`             | The enum value rule alone (R-ENUM): every SDK enum constant an action's field can carry is offered, and nothing is offered the SDK does not declare; fails on a finding                                                                               |
| `make audit-catalog-first`          | Generate ActionSpec surface coverage inventory in `dist/action-spec-coverage.json`                                                                                                                                                                    |
| `make audit-dynamic-aliases`        | Audit Dynamic search aliases and canonical action reachability                                                                                                                                                                                        |
| `make audit-test-names`             | Audit test function naming convention compliance                                                                                                                                                                                                      |
| `make audit-test-goroutines`        | Report `testing.T` aborts (`t.Fatal`/`FailNow`) made off the test goroutine, with a JSON work list                                                                                                                                                    |
| `make check-test-goroutines`        | Fail when any off-goroutine abort site remains (errorf-without-return stays advisory)                                                                                                                                                                 |
| `make audit-test-subtests`          | Report case loops in Test functions that assert without a `t.Run` subtest, with a JSON work list                                                                                                                                                      |
| `make check-test-subtests`          | Fail when any case loop still asserts without a subtest                                                                                                                                                                                               |
| `make check-graphql-documents`      | Fail when a raw GraphQL document in the source is one the pinned GitLab schema refuses                                                                                                                                                                |
| `make audit-graphql-documents`      | Same check, listing every document it accepted rather than only the refused ones                                                                                                                                                                      |
| `make check-graphql-schema`         | Fail when the committed GitLab schema does not parse, its provenance record does not decode, or the pin is of something else: another instance, a truncated or narrower answer, no recorded version, or older than 180 days                           |
| `make gen-graphql-schema`           | Re-pin the GitLab schema from a live instance; needs the network, so it is not a gate. Set `GITLAB_TOKEN` to a gitlab.com credential, since an anonymous introspection records no version and the check refuses that                                  |
| `make check-graphql-documents-live` | Judge every document against a schema fetched from gitlab.com right now rather than the pin, which is the only check that reports a field GitLab has narrowed since. Needs the network, so it runs under `make test-e2e-gitlab-com` rather than in CI |
| `make check-supply-chain`           | Fail when a release-configuration invariant breaks (pinned actions, locked release jobs, stated Dependabot cooldowns, current security policy, signature-verifying installers)                                                                        |
| `make audit-godocs`                 | Generate `dist/analysis/godoc.md` with package, exported symbol, and test documentation findings                                                                                                                                                      |
| `make audit-godocs-check`           | Run the same Godoc audit and fail if findings remain                                                                                                                                                                                                  |

## Tool Details

### golangci-lint

`golangci-lint` is the canonical Go analysis gate. It runs configured linters and formatters from [`.golangci.yml`](../../.golangci.yml), including tools that were previously run as standalone Make/CI jobs.

```bash
make golangci-lint
golangci-lint config verify
golangci-lint fmt --diff
golangci-lint run --build-tags e2e,collectore2e,httpe2e,orbitlive,stdioe2e ./...
```

The Make target performs these steps:

1. Validate `.golangci.yml`.
2. Check configured Go formatters with `golangci-lint fmt --diff`.
3. Run configured linters with every end-to-end build tag.

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
govulncheck -tags e2e,collectore2e,httpe2e,orbitlive,stdioe2e ./...
```

It remains separate because it is a vulnerability database scanner, not a normal lint rule inside `golangci-lint`.

#### Accepted-advisory allowlist

`govulncheck` has no native ignore mechanism, so `make govulncheck` runs through
[`scripts/govulncheck.sh`](../../scripts/govulncheck.sh). The wrapper runs
`govulncheck` unchanged and, when advisories are reported, passes **only if every
reported advisory ID is on its allowlist**. Any advisory not on the list — that
is, any newly introduced or fixable one — still fails the build. Running
`govulncheck` directly (as in the example above) shows the raw findings without
the allowlist.

Accepted advisories (keep the list in the script in sync with this table):

**None.** The allowlist is empty and should stay that way. It held one entry,
[`GO-2026-5932`](https://pkg.go.dev/vuln/GO-2026-5932), "the
`golang.org/x/crypto/openpgp` package is unmaintained, unsafe by design, and has
known security issues". It reached the binary through
`github.com/creativeprojects/go-selfupdate`: that module's `validate.go` imports
openpgp unconditionally for its `PGPValidator` type, linking it into anything
that imports `selfupdate`. The advisory covers every version of the module
(`introduced: 0`, `Fixed in: N/A`), so no dependency bump could ever have cleared
it. Removing the self-update subsystem removed the call path, and govulncheck now
reports "Your code is affected by 0 vulnerabilities" where it previously named
ours.

Be precise about what did **not** happen, because the shorter version of this
story is wrong. The advisory is keyed to the module `golang.org/x/crypto`, not to
the `openpgp` package, and that module is still a direct requirement:
`cmd/eval_mcp_surfaces` imports `golang.org/x/crypto/ssh` to build its fixtures.
So `govulncheck -show verbose ./...` still lists `GO-2026-5932` under module
results, and always will. What changed is the only thing that was ever
actionable: nothing in this repository calls into openpgp any more, so the
package is not linked into any shipped binary.

That distinction is also what the wrapper gates on. It defers to govulncheck's
own exit status, which reports whether **our code calls** a vulnerable symbol,
rather than scraping advisory IDs out of the printed text. Scraping was the
earlier design and it made the gate depend on a flag the caller passed:
`scripts/govulncheck.sh -show verbose ./...` failed while the same scan without
the flag passed, because module-level results are printed only at the higher
verbosity.

An entry here is a vulnerability shipped on purpose in code we actually call.

To accept a new advisory, add its OSV ID to `ALLOWLIST` in
`scripts/govulncheck.sh` and add a row here with the justification. To retire one
(e.g. once a fix ships), remove it from both.

### markdownlint-cli2

`markdownlint-cli2` checks Markdown style and consistency.

```bash
make mdlint
make mdlint-fix
```

Make excludes `plan/` because it contains working drafts that are not versioned as polished documentation.

## SonarCloud (local)

`make sonar` reproduces the CI SonarCloud pipeline locally: it runs the unit
tests with coverage (`go test -coverpkg=./cmd/...,./internal/...`), uploads the
analysis with `sonar-scanner` (configuration from `sonar-project.properties`),
polls the SonarCloud Compute Engine task until the analysis is processed, and
prints the quality gate result with its per-condition status and key measures.
It reads `SONARQUBE_TOKEN` from `.env` and analyzes the current git branch
(override with `SONAR_BRANCH=<name>`); it exits non-zero when the gate fails.

`make sonar-status` fetches and prints the latest gate for the current branch
without re-running tests or re-uploading — useful for re-checking a result.

```bash
make sonar                       # full pipeline: tests + upload + poll + report
make sonar SONAR_BRANCH=main     # analyze a specific branch
make sonar-status                # just print the latest gate (no re-scan)
```

## CI Integration

GitHub Actions uses the same separation as Make:

- The `golangci-lint` job installs the pinned `golangci-lint` release through the official action and runs `make golangci-lint`, so CI and a developer's machine run the same three commands with the same version. Its per-package analysis cache is kept between runs.
- The `govulncheck` job installs the `govulncheck` named by the `tool` directive in `go.mod` (a bare `go install`, so the pin lives in one place) and runs `make govulncheck`.
- The `Markdown` job runs `markdownlint-cli2` for Markdown and MDX content through the linter's own action, which bundles the tool and touches no registry.

Separate jobs for `goimports`, `gofmt`, `go vet`, `modernize`, `gosec`, and `staticcheck` are intentionally omitted because `golangci-lint` already covers them with the repository configuration.

### The race detector

The race detector is a gate of its own, in `.github/workflows/race.yml`, and it deliberately does not run on a pull request. The unit suite takes minutes without it and the better part of an hour with it, which no push should pay for a class of defect that most changes cannot introduce. It runs in two places instead:

| When            | How                                                                                                   | What it covers                                    |
| --------------- | ----------------------------------------------------------------------------------------------------- | ------------------------------------------------- |
| Every release   | `release.yml` calls `race.yml` beside the E2E gate, in front of the first job that publishes anything | so a race cannot reach a tag                      |
| Mondays, weekly | the workflow's own `schedule`, beside the scheduled E2E run                                           | so a race introduced this week surfaces this week |

Three jobs run there: the whole unit suite (`go test -race ./cmd/... ./internal/...`), and the two transport end-to-end modules, which are the only tests that drive the shipped binary as a process. `go test -race` instruments the test binary alone, so each transport harness passes the flag on to the server it builds through a build-tag seam (`harness_race_test.go` beside `harness_norace_test.go` in `test/e2e/http` and `test/e2e/stdio`) and starts it with `GORACE=halt_on_error=1`, or a detected race would be printed to a log and the run would stay green.

Every one of those runs carries an explicit `-timeout`, and so does `make test-race`. Go's default is ten minutes **per package binary**, and `internal/tools` alone takes about 976 s under the detector against 113 s without it, so before the timeouts existed the target could not finish and reported a timeout rather than a race. `RACE_TIMEOUT` in the Makefile is that bound; it is a bound and not an expectation, there to end a deadlock with every goroutine stack printed.

Nothing here reaches the `CI` workflow's graph, so the single required check is unchanged.

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

If a cold run hits the timeout, suspect concurrency before suspecting the
timeout. `golangci-lint` defaults to one worker per logical CPU, and across 226
packages and 55 linters that working set does not fit comfortably in memory on a
many-threaded machine — it thrashes rather than going faster. Measured on a cold
cache on the same 16-thread machine:

| Concurrency           | Peak RSS | Cold run         |
| --------------------- | -------- | ---------------- |
| 16 (former default)   | 4.4 GB   | >18m, unfinished |
| 4 (`run.concurrency`) | 3.6 GB   | **3m01s**        |

`run.concurrency: 4` is therefore set in `.golangci.yml`. CI runners have 4 vCPUs
and were already getting this implicitly, which is why CI completed in ~4 minutes
while a much larger local machine timed out. Raising it is unlikely to help; if
you need to experiment, override per-run with `-j`:

```bash
golangci-lint run -j 8 ./...
```

The analysis cache matters too — a warm run takes seconds against minutes cold —
but it is rebuilt automatically and needs no manual warming.

### Need a standalone tool during investigation

Use standalone commands temporarily for debugging if they provide more focused output, but do not add them back to `make analyze` or CI when the same check is already enforced through `golangci-lint`.
