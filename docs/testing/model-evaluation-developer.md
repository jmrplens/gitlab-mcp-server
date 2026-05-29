# AI Model Evaluation Developer Guide

> **Diátaxis type**: How-to and reference
> **Audience**: Maintainers and contributors
> **Prerequisites**: Go toolchain, model provider API keys, Docker for live mode

This guide explains how to run and maintain the AI model evaluation system built
around `cmd/eval_mcp_surfaces`.

## Source Map

| Path                                                     | Purpose                                                                               |
| -------------------------------------------------------- | ------------------------------------------------------------------------------------- |
| `cmd/eval_mcp_surfaces/main.go`                          | Thin command entry point that delegates to the internal evaluator package.            |
| `cmd/eval_mcp_surfaces/internal/evaluator/run.go`        | High-level command workflow, environment setup, catalog preparation, and model runs.  |
| `cmd/eval_mcp_surfaces/internal/evaluator/options.go`    | CLI flags, presets, and tool-surface normalization.                                   |
| `cmd/eval_mcp_surfaces/internal/evaluator/runner.go`     | Model loop, tool-call budgets, validation feedback, and simulated tool results.       |
| `cmd/eval_mcp_surfaces/internal/evaluator/sessions.go`   | Mock and live MCP server sessions, resource/prompt registration, and catalog routing. |
| `cmd/eval_mcp_surfaces/internal/evaluator/bridge.go`     | MCP capability bridge tools for resources, prompts, completions, and capabilities.    |
| `cmd/eval_mcp_surfaces/internal/evaluator/case_*.go`     | Typed case definitions, case registry, prompt rendering, and fixture engine.          |
| `cmd/eval_mcp_surfaces/internal/evaluator/report.go`     | Per-run Markdown reports, metrics, diagnostics, usage, and coverage output.           |
| `cmd/eval_mcp_surfaces/internal/evaluator/comparison.go` | Cross-report comparison for model, token, diagnostic, usage, and coverage trends.     |
| `cmd/eval_mcp_surfaces/internal/evaluator/providers.go`  | Provider adapters for Anthropic, Google, OpenAI, and Qwen-compatible APIs.            |
| `cmd/eval_mcp_surfaces/internal/evaluator/fixtures.go`   | Docker GitLab fixture preparation and placeholder replacement.                        |
| `cmd/eval_mcp_surfaces/internal/evalrun/`                | Small run utilities shared by fixture and model execution code.                       |
| `cmd/eval_mcp_surfaces/internal/termio/`                 | Terminal progress and log routing for long local runs and wrapper scripts.            |
| `dist/evaluation/mcp-surfaces/`                          | Generated reports, traces, and fixture state; ignored by Git.                         |
| `docs/testing/model-results.md`                          | Current published benchmark result copied from generated reports.                     |

Case definitions are grouped in `case_registry_*.go` files by partition:
`case_registry_read.go`, `case_registry_mutating.go`,
`case_registry_destructive.go`, `case_registry_capabilities.go`, and the
Enterprise/Premium variants. Add new cases there rather than in testdata files.

The evaluator implementation intentionally lives under `internal/evaluator` so
the command can stay small while the implementation keeps package-private helper
types. Prefer adding new evaluator logic inside that package unless the code has
a clear standalone boundary like terminal I/O or generic run utilities.

## Result Triage Policy

Use live model traces to decide where a fix belongs before changing prompt text
or MCP metadata:

1. If the model cannot discover the right action, the dynamic ranker, aliases,
  action descriptions, or `gitlab://tools` manifest metadata are the first
  suspects.
2. If the model chooses the right action but invents or omits schema-visible
  parameters, inspect the canonical `ActionSpec`, JSON schema tags, parameter
  guidance, and output `next_steps` before changing evaluator prompts.
3. If the MCP response or Markdown output encourages the wrong follow-up action,
  fix the tool output, hints, or formatter in the MCP implementation.
4. If the MCP metadata is already precise and the failure comes from an
  evaluator-only compact workflow plan, fixture placeholder, or assertion rule,
  change the evaluator harness.

When auditing full runs, treat a `report_clean` result as necessary but not
sufficient. Also inspect repaired first-pass diagnostics, Docker live triage,
and trace-level validation failures so hidden MCP guidance problems are not
papered over by retries.

## Environment

The evaluator reads model provider keys from environment variables:

| Provider  | Environment variable                 |
| --------- | ------------------------------------ |
| Anthropic | `ANTHROPIC_API_KEY`                  |
| Google    | `GOOGLE_API_KEY` or `GEMINI_API_KEY` |
| OpenAI    | `OPENAI_API_KEY`                     |
| Qwen      | `QWEN_API_KEY`                       |

Docker mode also needs `test/e2e/.env.docker`, created by the E2E provisioning
scripts. Enterprise Docker mode additionally needs `GITLAB_ENTERPRISE=true`, the
EE image, and `ENTERPRISE_LICENSE` supplied through the shell or the repository
`.env` file. Never print or commit `.env`, `.env.docker`, provider keys,
licenses, raw traces, or generated fixture state.

The documented Qwen configuration uses `QWEN_API_KEY` directly. Keep provider
fallbacks out of `.env.example` unless the evaluator command examples also need
those fallback variables.

The commands below resolve `go` through an explicit `PATH` so they also work in
non-interactive shells where `timeout` cannot find the Go binary.

## Model Set

Use this economy-oriented model set for the standard compatibility matrix unless
a focused run requires different models:

```bash
EVAL_MODELS="anthropic:claude-haiku-4-5-20251001,google:gemini-flash-latest,openai:gpt-5.4-nano,qwen:qwen3.6-flash"
```

`google:gemini-flash-latest` resolves to the latest Gemini Flash model available
to the API key. If you pin a concrete Google model ID instead, verify it with
Google ListModels first; Gemini preview IDs can retire without code changes in
this repository.

## Surfaces And Capability Access

`cmd/eval_mcp_surfaces` evaluates the model-facing MCP surface, not a reduced
test-only catalog. The default `--tool-surface` is `dynamic`, matching the
server default. Add `--tool-surface meta` when you need a meta-tool baseline.
The evaluator intentionally does not support `individual` today because the
individual catalog is too large for the model-compatibility matrix and is
already covered by unit and E2E tool registration tests.

Every live evaluator session registers the same public capability shape as a
normal full server: GitLab resources, workflow guides, prompts, completions, and
the surface-aware `gitlab://tools` / `gitlab://tools/{id}` manifest. When a task
needs to inspect those MCP primitives, the evaluator exposes bridge tools such
as `gitlab_list_resources`, `gitlab_read_resource`, `gitlab_list_prompts`,
`gitlab_get_prompt`, and `gitlab_complete`. These bridge tools represent client
MCP calls; they do not replace or hide the normal GitLab operation tools.

Use the `docker-capability-discovery` preset only for targeted capability
fallback work. For ordinary dynamic or meta full runs, keep the classic Docker
presets (`docker-read`, `docker-mutating-safe`, and `docker-destructive-safe`) so
the model sees the same broad MCP server surface while executing GitLab tasks.

## Run Schema Evaluation

Schema evaluation does not need Docker. It exercises provider tool-calling
against the MCP catalog and evaluator validation rules.

```bash
timeout 10800s bash -lc '
set -euo pipefail

export PATH="/usr/local/go/bin:$HOME/go/bin:/snap/bin:$PATH"
GO_BIN="${GO_BIN:-$(command -v go)}"
EVAL_MODELS="anthropic:claude-haiku-4-5-20251001,google:gemini-flash-latest,openai:gpt-5.4-nano,qwen:qwen3.6-flash"

timeout 10800s "$GO_BIN" run ./cmd/eval_mcp_surfaces \
  --preset schema-enterprise \
  --models "$EVAL_MODELS" \
  --skip-unavailable \
  --out dist/evaluation/mcp-surfaces/schema-enterprise-all-models.md
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

For Enterprise Ultimate validation, use the EE image. With a 24-character
activation code, pass it to the container as `GITLAB_ACTIVATION_CODE` during
startup. Legacy `.gitlab-license` keys can remain in `ENTERPRISE_LICENSE`; the
setup script installs those without echoing the license:

```bash
timeout 3600s env GITLAB_IMAGE=gitlab/gitlab-ee:latest GITLAB_ACTIVATION_CODE="$ENTERPRISE_LICENSE" docker compose -f test/e2e/docker-compose.yml up -d
timeout 1800s ./test/e2e/scripts/wait-for-gitlab.sh
timeout 1800s GITLAB_ENTERPRISE=true ./test/e2e/scripts/setup-gitlab.sh
timeout 1800s ./test/e2e/scripts/register-runner.sh
```

After an activation-code run succeeds, `setup-gitlab.sh` exports the generated
license key from GitLab's license usage CSV into `test/e2e/.enterprise-license`
with owner-only permissions. The Docker Enterprise wrappers prefer that ignored
cache on later runs and install it through the License API instead of passing the
activation code again. Remove the cache file when you intentionally want to test
a fresh activation-code flow.

The evaluator can refresh its own model-evaluation fixtures with
`--prepare-fixtures`. Some destructive tasks also create just-in-time resources
per attempt so repeated runs do not fail because a previous run deleted the
initial fixture.

## Run Docker Evaluation For One Model

This is the cheapest full Docker pass when using the current OpenAI nano model.
It evaluates the default dynamic surface. Add `--tool-surface meta` to each
command when comparing against the meta-tool surface.

```bash
timeout 10800s bash -lc '
set -euo pipefail

export PATH="/usr/local/go/bin:$HOME/go/bin:/snap/bin:$PATH"
GO_BIN="${GO_BIN:-$(command -v go)}"

for preset in docker-read docker-mutating-safe docker-destructive-safe; do
  timeout 3600s "$GO_BIN" run ./cmd/eval_mcp_surfaces \
    --preset "$preset" \
    --model openai:gpt-5.4-nano \
    --backend=gitlab \
    --gitlab-env-file test/e2e/.env.docker \
    --prepare-fixtures \
    --use-fixtures \
    --execute-tools \
    --skip-unavailable \
    --out "dist/evaluation/mcp-surfaces/${preset}-openai-gpt-5.4-nano.md"
done
'
```

## Run Docker Evaluation For All Models

This runs the classic Docker presets against the default dynamic surface. To
publish a meta-tool comparison, repeat the same loop with `--tool-surface meta`
and separate output file names.

```bash
timeout 21600s bash -lc '
set -euo pipefail

export PATH="/usr/local/go/bin:$HOME/go/bin:/snap/bin:$PATH"
GO_BIN="${GO_BIN:-$(command -v go)}"
EVAL_MODELS="anthropic:claude-haiku-4-5-20251001,google:gemini-flash-latest,openai:gpt-5.4-nano,qwen:qwen3.6-flash"

for preset in docker-read docker-mutating-safe docker-destructive-safe; do
  timeout 7200s "$GO_BIN" run ./cmd/eval_mcp_surfaces \
    --preset "$preset" \
    --models "$EVAL_MODELS" \
    --backend=gitlab \
    --gitlab-env-file test/e2e/.env.docker \
    --prepare-fixtures \
    --use-fixtures \
    --execute-tools \
    --skip-unavailable \
    --out "dist/evaluation/mcp-surfaces/${preset}-all-models.md"
done
'
```

For Enterprise Ultimate model runs, prefer the wrapper so the EE image, license
installation, fixture refreshes, and Enterprise presets stay together:

```bash
make eval-surfaces-docker-enterprise SURFACE=dynamic
```

The underlying presets are `docker-enterprise-read`,
`docker-enterprise-mutating-safe`, and `docker-enterprise-destructive-safe`.
The wrapper passes `--edition enterprise`, so it excludes CE/base and capability
discovery cases from the full Enterprise run. A focused run can pass one preset:

```bash
make eval-surfaces-docker-enterprise SURFACE=dynamic PRESET=docker-enterprise-read
```

## Run Targeted Tasks

Use targeted runs after fixing a schema description, provider adapter, fixture,
or MCP handler. Keep the task list small and inspect every failure trace.

```bash
timeout 1800s bash -lc '
set -euo pipefail

export PATH="/usr/local/go/bin:$HOME/go/bin:/snap/bin:$PATH"
GO_BIN="${GO_BIN:-$(command -v go)}"

timeout 1800s "$GO_BIN" run ./cmd/eval_mcp_surfaces \
  --model openai:gpt-5.4-nano \
  --backend=gitlab \
  --gitlab-env-file test/e2e/.env.docker \
  --prepare-fixtures \
  --use-fixtures \
  --execute-tools \
  --task MT-032,MT-039,MT-093,MT-095 \
  --out dist/evaluation/mcp-surfaces/targeted-openai-gpt-5.4-nano.md
'
```

## Important Flags

| Flag                                          | Meaning                                                                                                                                                   |
| --------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `--preset schema-enterprise`                  | Schema-only Enterprise/Premium route coverage; dry-run by default.                                                                                        |
| `--preset docker-read`                        | Docker read-only partition.                                                                                                                               |
| `--preset docker-mutating-safe`               | Docker safe mutation partition.                                                                                                                           |
| `--preset docker-destructive-safe`            | Docker safe destructive partition.                                                                                                                        |
| `--preset docker-enterprise-read`             | Docker Enterprise/Premium read-only partition.                                                                                                            |
| `--preset docker-enterprise-mutating-safe`    | Docker Enterprise/Premium safe mutation partition.                                                                                                        |
| `--preset docker-enterprise-destructive-safe` | Docker Enterprise/Premium safe destructive partition.                                                                                                     |
| `--edition ce\|enterprise\|all`               | Filter tasks by GitLab edition. Docker presets set this automatically unless explicitly overridden.                                                       |
| `--model`                                     | One provider/model pair. Overrides `--models`.                                                                                                            |
| `--models`                                    | Comma-separated provider/model list.                                                                                                                      |
| `--backend=gitlab`                            | Build the catalog against the real GitLab backend.                                                                                                        |
| `--gitlab-env-file`                           | Load Docker GitLab credentials from `test/e2e/.env.docker`.                                                                                               |
| `--prepare-fixtures`                          | Create or refresh Docker GitLab resources used by evaluation tasks.                                                                                       |
| `--use-fixtures`                              | Replace placeholder IDs in prompts with fixture state.                                                                                                    |
| `--execute-tools`                             | Execute validated model tool calls through MCP.                                                                                                           |
| `--skip-unavailable`                          | Skip routes not available in the current catalog or GitLab edition.                                                                                       |
| `--task`                                      | Comma-separated task IDs for targeted runs.                                                                                                               |
| `--out`                                       | Markdown report path. Trace directory defaults to `<report>.traces/`.                                                                                     |
| `--terminal-log`                              | File receiving progress and terminal output. Defaults beside `--out`, or under `dist/evaluation/mcp-surfaces/terminal/` when no report path is known yet. |
| `--print-output`                              | Also echo progress/output to the terminal. Without this flag, the command writes terminal output only to `--terminal-log`.                                |
| `--publish-docs`                              | Publish reviewed evaluation reports into the managed docs blocks.                                                                                         |
| `--publish-from`                              | Reviewed Markdown report path to publish; repeat once per report.                                                                                         |
| `--publish-label`                             | Human-readable label for the published snapshot.                                                                                                          |
| `--check-docs`                                | Verify committed docs match the selected `--publish-from` reports without writing files.                                                                  |

### Tool Surface Flags

| Flag value               | Model-facing catalog                                                                           | Primary use                                                            |
| ------------------------ | ---------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------- |
| `--tool-surface dynamic` | `gitlab_find_action`, `gitlab_execute_action`, and optional MCP capability bridge tools.       | Default full Docker and schema runs for the current server experience. |
| `--tool-surface meta`    | Consolidated domain meta-tools plus standalone tools and optional MCP capability bridge tools. | Compatibility baseline and comparison with the pre-dynamic default.    |

Capability bridge tools are enabled by task and evaluator options, not by
`CAPABILITY_SURFACE=minimal`. The evaluator should expose the resources,
prompts, and completions a normal full server exposes unless a test explicitly
targets a capability-discovery fallback.

## Outputs

Each model-backed run writes:

| Output                  | Purpose                                                                                                                                                                                  |
| ----------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `*.md` report           | Startup placeholder, then final summary metrics, task results, API usage, and failure triage. If the run stops before final metrics, the file is replaced with a failure report.         |
| `*.log` terminal log    | Progress lines, report-write notifications, provider warnings, and command errors. The evaluator writes this by default and stays silent on the terminal unless `--print-output` is set. |
| `*.traces/index.md`     | Trace index.                                                                                                                                                                             |
| `*.traces/*.json`       | Per-task trace with prompts, tool calls, validation, MCP results, and repairs.                                                                                                           |
| `*.traces/traces.jsonl` | JSONL stream for programmatic analysis.                                                                                                                                                  |
| `e2e-fixtures.json`     | Docker model-evaluation fixture IDs; generated and ignored.                                                                                                                              |

For long runs, always pass an explicit `--out` path so the terminal log defaults
to a sibling `.log` file. The Markdown report is the review artifact; terminal
output is only progress logging and stays in the log file by default.

## Triage Workflow

1. Read the report metrics and identify failing tasks.
2. Open each failing task trace in the `.traces/` directory.
3. Classify the failure as model route miss, parameter shape miss, provider
   adapter issue, fixture gap, GitLab edition limitation, sampling support gap,
   or MCP implementation bug.
4. Check whether the trace used the intended tool surface. Dynamic traces for
  ordinary GitLab operations should call `gitlab_find_action` before each
  `gitlab_execute_action`. Capability bridge traces may call bridge tools such
  as `gitlab_list_resources` or `gitlab_read_resource` directly.
5. Fix harness noise before judging model quality.
6. Re-run the targeted task set.
7. Re-run the affected preset.
8. Publish the reviewed reports with `cmd/eval_mcp_surfaces --publish-docs`.

Use `--publish-from` once per reviewed Markdown report and set a clear
`--publish-label`. The publication phase updates only the managed marker blocks
in [AI Model Evaluation Results](model-results.md) and the repository README.
CE/base and Enterprise/Premium rows are routed into separate dynamic and
meta-tool blocks, so publishing licensed runs does not overwrite CE results.
Normal evaluator runs never update documentation automatically. Use `--check-docs`
in CI-style validation when the selected reports should already match the
committed docs.

## Adding Or Updating Cases

Edit the typed registry files in `cmd/eval_mcp_surfaces/internal/evaluator/`:

- `case_registry_read.go` for CE read-only operations.
- `case_registry_mutating.go` for CE safe mutations.
- `case_registry_destructive.go` for CE destructive operations.
- `case_registry_capabilities.go` for MCP capability bridge scenarios.
- `case_registry_enterprise_*.go` for Enterprise/Premium scenarios.

Use the following guidance:

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
timeout 300s go test ./cmd/eval_mcp_surfaces ./cmd/gen_testing_docs -count=1
timeout 120s go run ./cmd/gen_testing_docs/ --check
timeout 120s npx markdownlint-cli2 docs/testing/*.md
```
