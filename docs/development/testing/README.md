# Testing Documentation

> **Diátaxis type**: Overview
> **Audience**: Users, evaluators, maintainers, contributors
> **Prerequisites**: Basic MCP concepts; Docker for live GitLab validation

This directory is the documentation hub for all validation work in
`gitlab-mcp-server`: conventional Go tests, real GitLab E2E tests, and AI
model evaluations that measure whether models can use the MCP catalog
correctly.

## Documents

| Document                                                             | Audience              | Purpose                                                                                                                |
| -------------------------------------------------------------------- | --------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| [Testing Reference](testing.md)                                      | Contributors          | Generated unit, integration, E2E, coverage, and package test reference.                                                |
| [AI Model Evaluation](model-evaluation.md)                           | Users and evaluators  | Explains what AI model evaluations prove, how schema and Docker modes differ, and how to interpret the metrics.        |
| [AI Model Evaluation Developer Guide](model-evaluation-developer.md) | Maintainers           | Operational guide for running schema and Docker model evaluations, adding cases, reading traces, and updating results. |
| [AI Model Evaluation Results](model-results.md)                      | Users and maintainers | Current published benchmark result selected from generated reports.                                                    |

## Validation Layers

| Layer                   | Runner                                                    | GitLab backend                     | What it proves                                                                                 |
| ----------------------- | --------------------------------------------------------- | ---------------------------------- | ---------------------------------------------------------------------------------------------- |
| Unit tests              | `go test ./internal/... ./cmd/...`                        | Mock `httptest` servers            | Handler logic, schema validation, formatting, routing, and error handling.                     |
| E2E tests               | `go test -tags e2e ./test/e2e/suite/`                     | Real GitLab, self-hosted or Docker | The MCP server can execute registered tools against GitLab APIs.                               |
| Schema model evaluation | `cmd/eval_mcp_surfaces --preset schema-enterprise`        | Mock catalog                       | Models can select tools/actions and shape arguments from the MCP schema and descriptions.      |
| Docker model evaluation | `cmd/eval_mcp_surfaces --preset docker-* --execute-tools` | Docker GitLab CE                   | Models can drive real MCP calls against a populated GitLab instance, including safe mutations. |

## Beyond Statement Coverage

Statement coverage is at or near 100% across `cmd/` and `internal/`, so it no
longer answers the question it is usually asked. Two tools answer what it
cannot, and both are run by hand on the package being worked on rather than
over the tree:

| Tool                                    | Command                                         | What it finds                                                                                           |
| --------------------------------------- | ----------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| Condition coverage ([gobco][gobco])     | `make coverage-conditions PKG=./internal/oauth` | An operand of a compound condition that no test ever evaluated both ways: a covered line, half decided. |
| Mutation testing ([gremlins][gremlins]) | `make coverage-mutants PKG=./internal/oauth`    | A line every test runs and none asserts: the tool flips an operator or a boundary and asks who notices. |

[gobco]: https://github.com/rillig/gobco
[gremlins]: https://github.com/go-gremlins/gremlins

### What they cost, and why the figures are not generated

Measured on an 8-core machine, September 2026, one package at a time:

| Package                        | Own test time | gobco | gremlins |
| ------------------------------ | ------------: | ----: | -------: |
| `internal/config`              |         1.5 s |   4 s |    128 s |
| `internal/oauth`               |         0.2 s |   6 s |     88 s |
| `internal/gitlab`              |         8.0 s |  68 s |    229 s |
| `internal/serverpool`          |         1.0 s |  69 s |    207 s |
| `internal/tools/actioncatalog` |         0.5 s |  68 s |        — |
| `internal/subscriptions`       |         1.5 s |  72 s |        — |
| `internal/tools/dynamic`       |        35.0 s | 231 s |        — |
| `internal/tools`               |        89.2 s | 283 s |        — |

Four packages therefore cost about eleven minutes of mutation testing and
another two and a half of condition coverage, against roughly six minutes for
the whole-tree coverage pass `cmd/gen_testing_docs` runs. Extending either to
the packages that carry logic would be four to five times the generator's
current cost, which is why neither figure is a generated column in
[Testing Reference](testing.md): a number nobody can afford to regenerate goes
stale in the file while looking current.

**gobco cannot analyse a package with build-constrained files at all.** It
copies every `.go` file into its work directory ignoring `//go:build`, so
`internal/toolutil` and `cmd/server` both fail with a redeclaration panic
(`openLeafNoFollow`, `isConnRefused`). Those two are covered by mutation
testing only.

### Reading a survivor

Not every survivor is a gap. Three kinds cannot be killed by any test and
should be recorded rather than chased:

- **A boundary whose two sides agree at the boundary.** Flipping `>` to `>=`
  where both branches assign the same value at the boundary changes nothing.
  Six of the retry-clamp mutants in `internal/gitlab` are this shape.
- **A guard that a second guard makes unobservable.** The negative token
  cache checks "disabled" in both `RecordKind` and `Lookup`, so removing the
  second changes no answer.
- **A tool artifact.** Mutations inside package-level constant initializers
  and `switch { case … }` expressions are reported as not covered because
  neither carries a statement counter, not because no test reaches them.

## When To Use Each Layer

Use unit tests for implementation changes and regression coverage. Use E2E
tests when a handler or capability needs real GitLab behavior. Use schema model
evaluation when changing tool descriptions, meta-tool schemas, provider
adapters, or token budget. Use Docker model evaluation when the question is
whether a model can complete a realistic task through the actual MCP server and
a real GitLab API.

## Result Policy

Generated model reports and traces are written under
`dist/evaluation/mcp-surfaces/` and are intentionally ignored by Git. Publish only
curated summaries in [AI Model Evaluation Results](model-results.md). Use
`cmd/eval_mcp_surfaces --publish-docs --publish-from <report>` after the selected
reports have been reviewed; use `--check-docs` to verify the managed blocks
without writing. A curated summary should include the model ID, evaluation mode,
preset or task set, number of expected operations, emitted model/tool calls,
success percentages, and any known caveats.

## Maintenance Rules

- When unit or E2E tests are added, modified, or removed, run
  `go run ./cmd/gen_testing_docs/` and keep [Testing Reference](testing.md)
  current.
- When model-evaluation cases, fixtures, or metrics change, update
  [AI Model Evaluation](model-evaluation.md),
  [AI Model Evaluation Developer Guide](model-evaluation-developer.md), and
  [AI Model Evaluation Results](model-results.md) together.
- Do not commit raw model traces, provider payloads, `.env` files, Docker
  fixture state, or generated reports under `dist/`.
