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
