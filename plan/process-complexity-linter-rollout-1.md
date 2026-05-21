---
goal: Add Maintained Complexity Linters Incrementally
version: 1.0
date_created: 2026-05-21
last_updated: 2026-05-21
owner: GitHub Copilot
status: 'In Progress'
tags: [process, linting, refactor, quality]
---

# Introduction

![Status: In Progress](https://img.shields.io/badge/status-In%20Progress-yellow)

This plan adds `gocognit`, `nestif`, `maintidx`, and `dupl` to `.golangci.yml` one at a time. Each phase enables exactly one linter at its GolangCI-Lint default threshold, refactors the reported findings, and validates the repository before moving to the next linter.

## 1. Requirements & Constraints

- **REQ-001**: Keep `gocyclo` enabled with `min-complexity: 20` as the primary cyclomatic complexity gate.
- **REQ-002**: Add exactly one new linter per phase: `gocognit`, `nestif`, `maintidx`, then `dupl`.
- **REQ-003**: Start each added linter at the GolangCI-Lint default threshold: `gocognit.min-complexity: 30`, `nestif.min-complexity: 5`, `maintidx.under: 20`, and `dupl.threshold: 150`.
- **REQ-004**: Do not lower thresholds below defaults during the initial enablement pass unless an explicitly requested ratchet is validated after the default threshold is clean.
- **REQ-005**: Refactor findings instead of adding broad exclusions. Add `//nolint` only for unavoidable cases with a specific linter name and explanation.
- **REQ-006**: Keep `run.tests: true` so production code and tests remain monitored.
- **CON-001**: Project artifacts must be written in English.
- **CON-002**: Keep changes scoped to lint configuration and functions reported by the active phase's linter.
- **CON-003**: Preserve existing public APIs and MCP behavior unless a finding requires an internal helper extraction.
- **PAT-001**: For Go tests, keep new or modified tests in the canonical `_test.go` file matching the source file under test.
- **PAT-002**: Prefer guard clauses, small named helpers, and table-driven test helper extraction over mechanical micro-refactors.
- **VAL-001**: Each phase must pass the linter being introduced, full `golangci-lint run ./...`, and relevant Go tests before completion.

## 2. Implementation Steps

### Implementation Phase 1

- GOAL-001: Enable cognitive complexity checks with `gocognit` at the default threshold, reduce all findings, then validate the requested ratchet to `min-complexity: 25`.

| Task | Description | Completed | Date |
|------|-------------|-----------|------|
| TASK-001 | Update `.golangci.yml` to add `gocognit` under `linters.enable` with comment `Cognitive complexity guard`, add `linters.settings.gocognit.min-complexity: 30`, then apply the validated requested ratchet to `25`. | Yes | 2026-05-21 |
| TASK-002 | Run `golangci-lint run --enable-only=gocognit --max-issues-per-linter=0 --max-same-issues=0 ./...` and save the reported file/function list in the working notes for the phase. | Yes | 2026-05-21 |
| TASK-003 | Refactor each `gocognit` finding by reducing nesting, splitting decision-heavy branches, and extracting named helpers. Known initial hotspots include `cmd/eval_mcp_surfaces/runner.go`, `cmd/format_md_tables/main.go`, `cmd/gen_llms/main.go`, `cmd/server/main_test.go`, and `internal/testutil/embedassert.go`. | Yes | 2026-05-21 |
| TASK-004 | Run `gofmt` or `goimports` on changed Go files, then run targeted `go test` for every changed package. | Yes | 2026-05-21 |
| TASK-005 | Validate with `golangci-lint run --enable-only=gocognit ./...` and `golangci-lint run ./...`. | Yes | 2026-05-21 |
| TASK-006 | If any test files changed, run `go run ./cmd/gen_testing_docs/`, `go run ./cmd/gen_testing_docs/ --check`, and `npx markdownlint-cli2 docs/testing/testing.md`. | Yes | 2026-05-21 |

Phase 1 completed on 2026-05-21. The default `gocognit` rollout was completed first, then the explicitly requested `min-complexity: 25` ratchet was validated with `gocognit`, full `golangci-lint`, targeted Go tests, E2E compile check, and generated testing documentation checks.

### Implementation Phase 2

- GOAL-002: Enable nested conditional checks with `nestif` at the default threshold and reduce all findings.

| Task | Description | Completed | Date |
|------|-------------|-----------|------|
| TASK-007 | Update `.golangci.yml` to add `nestif` under `linters.enable` with comment `Nested if complexity guard`, and add `linters.settings.nestif.min-complexity: 5`. | Yes | 2026-05-21 |
| TASK-008 | Run `golangci-lint run --enable-only=nestif --max-issues-per-linter=0 --max-same-issues=0 ./...` and save the reported file/function list in the working notes for the phase. | Yes | 2026-05-21 |
| TASK-009 | Refactor each `nestif` finding with guard clauses, early returns, extracted validators, or extracted branch handlers. Known initial hotspots include `cmd/add_docs/main.go`, `cmd/eval_mcp_surfaces/fixtures.go`, `cmd/eval_mcp_surfaces/providers.go`, `cmd/eval_mcp_surfaces/task_prompts.go`, `cmd/eval_mcp_surfaces/validation.go`, and `cmd/format_md_tables/main.go`. | Yes | 2026-05-21 |
| TASK-010 | Run `gofmt` or `goimports` on changed Go files, then run targeted `go test` for every changed package. | Yes | 2026-05-21 |
| TASK-011 | Validate with `golangci-lint run --enable-only=nestif ./...` and `golangci-lint run ./...`. | Yes | 2026-05-21 |
| TASK-012 | If any test files changed, refresh and check `docs/testing/testing.md` with `cmd/gen_testing_docs` and `markdownlint-cli2`. | Yes | 2026-05-21 |

Phase 2 completed on 2026-05-21. The `nestif` default threshold found 13 nested conditional hotspots, which were resolved with guard clauses and focused helper extraction across production code, tests, and E2E helpers. Validation included `nestif`, full `golangci-lint`, targeted Go tests, E2E compile check, and generated testing documentation checks.

### Implementation Phase 3

- GOAL-003: Enable maintainability index checks with `maintidx` at the default threshold and reduce all findings.

| Task | Description | Completed | Date |
|------|-------------|-----------|------|
| TASK-013 | Update `.golangci.yml` to add `maintidx` under `linters.enable` with comment `Maintainability index guard`, and add `linters.settings.maintidx.under: 20`. |  |  |
| TASK-014 | Run `golangci-lint run --enable-only=maintidx --max-issues-per-linter=0 --max-same-issues=0 ./...` and save the reported file/function list in the working notes for the phase. |  |  |
| TASK-015 | Refactor each `maintidx` finding by reducing Halstead volume and function size while preserving behavior. Known initial hotspots include `internal/tools/dynamic/register_test.go`, `internal/tools/markdown_test.go`, `internal/tools/modelregistry/model_registry_test.go`, `internal/tools/register_test.go`, and selected `test/e2e/suite/*_test.go` files. |  |  |
| TASK-016 | Keep large test fixtures in canonical package test files; extract local fixture builders or table fragments only when they reduce maintainability findings without obscuring assertions. |  |  |
| TASK-017 | Run `gofmt` or `goimports` on changed Go files, then run targeted `go test` for every changed package. |  |  |
| TASK-018 | Validate with `golangci-lint run --enable-only=maintidx ./...` and `golangci-lint run ./...`. |  |  |
| TASK-019 | Refresh and check `docs/testing/testing.md` because this phase is expected to touch test files. |  |  |

### Implementation Phase 4

- GOAL-004: Enable duplicate-code checks with `dupl` at the default threshold and reduce all findings.

| Task | Description | Completed | Date |
|------|-------------|-----------|------|
| TASK-020 | Update `.golangci.yml` to add `dupl` under `linters.enable` with comment `Duplicate code guard`, and add `linters.settings.dupl.threshold: 150`. |  |  |
| TASK-021 | Run `golangci-lint run --enable-only=dupl --max-issues-per-linter=0 --max-same-issues=0 ./...` and group findings by duplicated behavior rather than by raw line pair. |  |  |
| TASK-022 | Refactor duplicate production code by extracting domain-specific helpers in the same package as the duplicated logic. Known initial hotspots include `cmd/eval_mcp_surfaces/live_targets.go`. |  |  |
| TASK-023 | Refactor duplicate test code by extracting canonical helpers in the existing test file or existing package test helpers. Do not create thematic test files unless the package already uses that helper layout. |  |  |
| TASK-024 | Avoid generic abstraction names. Each extracted helper must describe the domain behavior it performs. |  |  |
| TASK-025 | Run `gofmt` or `goimports` on changed Go files, then run targeted `go test` for every changed package. |  |  |
| TASK-026 | Validate with `golangci-lint run --enable-only=dupl ./...`, `golangci-lint run ./...`, `go test ./... -count=1`, and `go test -tags e2e -c -o /dev/null ./test/e2e/suite/`. |  |  |
| TASK-027 | Refresh and check `docs/testing/testing.md` because this phase is expected to touch test files. |  |  |

## 3. Alternatives

- **ALT-001**: Enable all four linters at once. Rejected because combined findings would mix independent metrics and make regressions harder to isolate.
- **ALT-002**: Add temporary exclusions for tests. Rejected because `run.tests: true` is an explicit quality requirement.
- **ALT-003**: Start with `dupl`. Rejected because `dupl` has the largest initial finding count and should be handled after lower-risk complexity gates are stable.
- **ALT-004**: Start with thresholds stricter than defaults. Rejected for this rollout because default thresholds establish the first baseline; explicitly requested ratchets can follow after the default threshold is clean.

## 4. Dependencies

- **DEP-001**: `golangci-lint` v2.12.2 or newer must be available in the development environment.
- **DEP-002**: `gocognit`, `nestif`, `maintidx`, and `dupl` are provided through GolangCI-Lint and do not require direct repository dependencies.
- **DEP-003**: `npx markdownlint-cli2` must be available for `docs/testing/testing.md` validation after generated documentation changes.

## 5. Files

- **FILE-001**: `.golangci.yml` will be updated in every phase to enable one additional linter and its default threshold.
- **FILE-002**: `cmd/eval_mcp_surfaces/*.go` is expected to be affected by `gocognit`, `nestif`, and `dupl` phases.
- **FILE-003**: `cmd/format_md_tables/main.go` is expected to be affected by `gocognit` and `nestif` phases.
- **FILE-004**: `internal/testutil/embedassert.go` is expected to be affected by the `gocognit` phase.
- **FILE-005**: `internal/tools/dynamic/register_test.go`, `internal/tools/markdown_test.go`, `internal/tools/modelregistry/model_registry_test.go`, and `internal/tools/register_test.go` are expected to be affected by the `maintidx` phase.
- **FILE-006**: `test/e2e/suite/*.go` may be affected by `maintidx` and `dupl` phases.
- **FILE-007**: `docs/testing/testing.md` must be regenerated when test files are changed.

## 6. Testing

- **TEST-001**: After each phase, run the newly enabled linter alone with `golangci-lint run --enable-only=<linter> ./...`.
- **TEST-002**: After each phase, run `golangci-lint run ./...` to ensure the complete lint configuration remains green.
- **TEST-003**: After each phase, run targeted `go test <changed-package> -count=1` for each package with changed Go code.
- **TEST-004**: After phase 4, run `go test ./... -count=1` because duplicate-code refactors can affect shared behavior across many packages.
- **TEST-005**: After changes touching E2E tests, run `go test -tags e2e -c -o /dev/null ./test/e2e/suite/`.
- **TEST-006**: After generated testing documentation changes, run `go run ./cmd/gen_testing_docs/ --check` and `npx markdownlint-cli2 docs/testing/testing.md`.

## 7. Risks & Assumptions

- **RISK-001**: `dupl` may require broad helper extraction and can accidentally over-abstract unrelated code. Mitigation: group findings by behavior and keep helpers package-local.
- **RISK-002**: `maintidx` can flag large data-heavy test fixtures with low cyclomatic complexity. Mitigation: split fixture data only when it improves readability and preserves assertion clarity.
- **RISK-003**: `gocognit` and `nestif` can overlap. Mitigation: complete and validate `gocognit` first, then let the `nestif` phase handle remaining nested branches.
- **RISK-004**: Refactoring E2E test setup can change lifecycle ordering. Mitigation: keep existing setup/cleanup semantics and run E2E compile checks after E2E changes.
- **ASSUMPTION-001**: Initial finding counts from 2026-05-21 were approximately `gocognit` 22 at threshold 30, `nestif` 13 after the phase 1 refactors, `maintidx` 9, and `dupl` 158. Counts may change after each phase.
- **ASSUMPTION-002**: The current `gocyclo` replacement for `cyclop` remains in place before phase 1 starts.

## 8. Related Specifications / Further Reading

- [GolangCI-Lint linter settings](https://golangci-lint.run/docs/linters/configuration/)
- [GolangCI-Lint linters catalog](https://golangci-lint.run/docs/linters/)
- [Project static analysis documentation](../docs/development/static-analysis.md)
