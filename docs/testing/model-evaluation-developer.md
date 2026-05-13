# AI Model Evaluation Developer Guide

> **Diátaxis type**: How-to and reference
> **Audience**: Maintainers and contributors
> **Prerequisites**: Go toolchain, model provider API keys, Docker for live mode

This guide explains how to run and maintain the AI model evaluation system built
around `cmd/eval_meta_tools`.

## Source Map

| Path | Purpose |
| --- | --- |
| `cmd/eval_meta_tools/main.go` | Evaluation runner, task filtering, MCP execution, report writing, trace writing. |
| `cmd/eval_meta_tools/providers.go` | Provider adapters for Anthropic, Google, OpenAI, and Qwen-compatible APIs. |
| `cmd/eval_meta_tools/fixtures.go` | Docker GitLab fixture preparation and placeholder replacement. |
| `cmd/eval_meta_tools/testdata/automated-meta-tool-cases.md` | Canonical task corpus. |
| `dist/evaluation/meta-tools/` | Generated reports, traces, and fixture state; ignored by Git. |
| `docs/testing/model-results.md` | Current published benchmark result copied from generated reports. |

## Environment

The evaluator reads model provider keys from environment variables:

| Provider | Environment variable |
| --- | --- |
| Anthropic | `ANTHROPIC_API_KEY` |
| Google | `GOOGLE_API_KEY` or `GEMINI_API_KEY` |
| OpenAI | `OPENAI_API_KEY` |
| Qwen | `QWEN_API_KEY` |

Docker mode also needs `test/e2e/.env.docker`, created by the E2E provisioning
scripts. Never print or commit `.env`, `.env.docker`, provider keys, raw traces,
or generated fixture state.

The documented Qwen configuration uses `QWEN_API_KEY` directly. Keep provider
fallbacks out of `.env.example` unless the evaluator command examples also need
those fallback variables.

The commands below resolve `go` through an explicit `PATH` so they also work in
non-interactive shells where `timeout` cannot find the Go binary.

## Model Set

Use this economy-oriented model set for the standard compatibility matrix unless
a focused run requires different models:

```bash
EVAL_MODELS="anthropic:claude-haiku-4-5-20251001,google:gemini-3.1-flash-lite-preview,openai:gpt-5.4-nano,qwen:qwen3.6-flash"
```

## Run Schema Evaluation

Schema evaluation does not need Docker. It exercises provider tool-calling
against the MCP catalog and evaluator validation rules.

```bash
timeout 10800s bash -lc '
set -euo pipefail

export PATH="/usr/local/go/bin:$HOME/go/bin:/snap/bin:$PATH"
GO_BIN="${GO_BIN:-$(command -v go)}"
EVAL_MODELS="anthropic:claude-haiku-4-5-20251001,google:gemini-3.1-flash-lite-preview,openai:gpt-5.4-nano,qwen:qwen3.6-flash"

timeout 10800s "$GO_BIN" run ./cmd/eval_meta_tools \
  --preset schema-enterprise \
  --models "$EVAL_MODELS" \
  --skip-unavailable \
  --out dist/evaluation/meta-tools/schema-enterprise-all-models.md
'
```

## Prepare Docker GitLab

Use Docker mode when model calls should execute against a real GitLab CE
instance.

```bash
timeout 3600s docker compose -f test/e2e/docker-compose.yml up -d
timeout 1800s ./test/e2e/scripts/wait-for-gitlab.sh
timeout 1800s ./test/e2e/scripts/setup-gitlab.sh
timeout 1800s ./test/e2e/scripts/register-runner.sh
```

The evaluator can refresh its own model-evaluation fixtures with
`--prepare-fixtures`. Some destructive tasks also create just-in-time resources
per attempt so repeated runs do not fail because a previous run deleted the
initial fixture.

## Run Docker Evaluation For One Model

This is the cheapest full Docker pass when using the current OpenAI nano model.

```bash
timeout 10800s bash -lc '
set -euo pipefail

export PATH="/usr/local/go/bin:$HOME/go/bin:/snap/bin:$PATH"
GO_BIN="${GO_BIN:-$(command -v go)}"

for preset in docker-read docker-mutating-safe docker-destructive-safe; do
  timeout 3600s "$GO_BIN" run ./cmd/eval_meta_tools \
    --preset "$preset" \
    --model openai:gpt-5.4-nano \
    --backend=gitlab \
    --gitlab-env-file test/e2e/.env.docker \
    --prepare-fixtures \
    --use-fixtures \
    --execute-tools \
    --skip-unavailable \
    --out "dist/evaluation/meta-tools/${preset}-openai-gpt-5.4-nano.md"
done
'
```

## Run Docker Evaluation For All Models

```bash
timeout 21600s bash -lc '
set -euo pipefail

export PATH="/usr/local/go/bin:$HOME/go/bin:/snap/bin:$PATH"
GO_BIN="${GO_BIN:-$(command -v go)}"
EVAL_MODELS="anthropic:claude-haiku-4-5-20251001,google:gemini-3.1-flash-lite-preview,openai:gpt-5.4-nano,qwen:qwen3.6-flash"

for preset in docker-read docker-mutating-safe docker-destructive-safe; do
  timeout 7200s "$GO_BIN" run ./cmd/eval_meta_tools \
    --preset "$preset" \
    --models "$EVAL_MODELS" \
    --backend=gitlab \
    --gitlab-env-file test/e2e/.env.docker \
    --prepare-fixtures \
    --use-fixtures \
    --execute-tools \
    --skip-unavailable \
    --out "dist/evaluation/meta-tools/${preset}-all-models.md"
done
'
```

## Run Targeted Tasks

Use targeted runs after fixing a schema description, provider adapter, fixture,
or MCP handler. Keep the task list small and inspect every failure trace.

```bash
timeout 1800s bash -lc '
set -euo pipefail

export PATH="/usr/local/go/bin:$HOME/go/bin:/snap/bin:$PATH"
GO_BIN="${GO_BIN:-$(command -v go)}"

timeout 1800s "$GO_BIN" run ./cmd/eval_meta_tools \
  --model openai:gpt-5.4-nano \
  --backend=gitlab \
  --gitlab-env-file test/e2e/.env.docker \
  --prepare-fixtures \
  --use-fixtures \
  --execute-tools \
  --task MT-032,MT-039,MT-093,MT-095 \
  --out dist/evaluation/meta-tools/targeted-openai-gpt-5.4-nano.md
'
```

## Important Flags

| Flag | Meaning |
| --- | --- |
| `--preset schema-enterprise` | Schema-only Enterprise/Premium route coverage; dry-run by default. |
| `--preset docker-read` | Docker read-only partition. |
| `--preset docker-mutating-safe` | Docker safe mutation partition. |
| `--preset docker-destructive-safe` | Docker safe destructive partition. |
| `--model` | One provider/model pair. Overrides `--models`. |
| `--models` | Comma-separated provider/model list. |
| `--backend=gitlab` | Build the catalog against the real GitLab backend. |
| `--gitlab-env-file` | Load Docker GitLab credentials from `test/e2e/.env.docker`. |
| `--prepare-fixtures` | Create or refresh Docker GitLab resources used by evaluation tasks. |
| `--use-fixtures` | Replace placeholder IDs in prompts with fixture state. |
| `--execute-tools` | Execute validated model tool calls through MCP. |
| `--skip-unavailable` | Skip routes not available in the current catalog or GitLab edition. |
| `--task` | Comma-separated task IDs for targeted runs. |
| `--out` | Markdown report path. Trace directory defaults to `<report>.traces/`. |
| `--publish-docs` | Publish reviewed evaluation reports into the managed docs blocks. |
| `--publish-from` | Reviewed Markdown report path to publish; repeat once per report. |
| `--publish-label` | Human-readable label for the published snapshot. |
| `--check-docs` | Verify committed docs match the selected `--publish-from` reports without writing files. |

## Outputs

Each model-backed run writes:

| Output | Purpose |
| --- | --- |
| `*.md` report | Startup placeholder, then final summary metrics, task results, API usage, and failure triage. If the run stops before final metrics, the file is replaced with a failure report. |
| `*.traces/index.md` | Trace index. |
| `*.traces/*.json` | Per-task trace with prompts, tool calls, validation, MCP results, and repairs. |
| `*.traces/traces.jsonl` | JSONL stream for programmatic analysis. |
| `e2e-fixtures.json` | Docker model-evaluation fixture IDs; generated and ignored. |

For long runs, always pass an explicit `--out` path and redirect stdout/stderr
to a sibling `.log` file. The Markdown report is the review artifact; terminal
output is only progress logging.

## Triage Workflow

1. Read the report metrics and identify failing tasks.
2. Open each failing task trace in the `.traces/` directory.
3. Classify the failure as model route miss, parameter shape miss, provider
   adapter issue, fixture gap, GitLab edition limitation, sampling support gap,
   or MCP implementation bug.
4. Fix harness noise before judging model quality.
5. Re-run the targeted task set.
6. Re-run the affected preset.
7. Publish the reviewed reports with `cmd/eval_meta_tools --publish-docs`.

Use `--publish-from` once per reviewed Markdown report and set a clear
`--publish-label`. The publication phase updates only the managed marker blocks
in [AI Model Evaluation Results](model-results.md) and the repository README;
normal evaluator runs never update documentation automatically. Use
`--check-docs` in CI-style validation when the selected reports should already
match the committed docs.

## Adding Or Updating Cases

Edit `cmd/eval_meta_tools/testdata/automated-meta-tool-cases.md`. Preserve the
existing table format and update the summary counts at the bottom. Use the
following guidance:

- Include `MT-` cases for one clear operation.
- Define `MS-` cases for real workflows where sequencing matters.
- Cover `MF-` cases for failure recovery and prompt-injection resilience.
- Include only required params in the required column.
- Mark destructive steps precisely so the evaluator can enforce confirmation.
- Prefer Docker fixtures over assumptions about a manually prepared instance.

## Keeping Documentation Current

After changing tests or evaluation behavior, run focused verification and lint
the affected Markdown files:

```bash
timeout 300s go test ./cmd/eval_meta_tools ./cmd/gen_testing_docs -count=1
timeout 120s go run ./cmd/gen_testing_docs/ --check
timeout 120s npx markdownlint-cli2 docs/testing/*.md
```
