# Evaluation Documentation

This directory documents the evaluation system used to validate `gitlab-mcp-server` as an MCP server for model-driven GitLab workflows.

The evaluation suite has three complementary layers:

| Document | Purpose |
| --- | --- |
| [Automated meta-tool cases](automated-meta-tool-cases.md) | Human-readable reference for the 134 automated evaluation cases used by `cmd/eval_meta_tools`. |
| [Current results](current-results.md) | Latest static, model, token, and quality results for the current branch. |
| [User prompt playbook](user-prompt-playbook.md) | Copy-ready prompts and scenario patterns for users who want to exercise the server manually. |

## Evaluation Goals

The current suite validates whether a model can use the compressed opaque meta-tool catalog without receiving every action schema inline. It focuses on four questions:

| Question | Evaluation signal |
| --- | --- |
| Can the model choose the right meta-tool? | Tool-selection accuracy. |
| Can the model choose the right action and parameter envelope? | Action-selection accuracy and first-call validation pass rate. |
| Can the model recover when validation returns an error? | Repair success rate. |
| Can the model avoid unsafe destructive behavior? | Destructive safety. |

## Automated Evaluation Flow

The automated evaluation is a validation harness for model tool-calling behavior. It does not execute GitLab operations against a live project.

| Step | What happens |
| --- | --- |
| 1. Parse fixture | `cmd/eval_meta_tools` reads [Automated meta-tool cases](automated-meta-tool-cases.md) and extracts every row beginning with `MT-*`, `MS-*`, or `MF-*`. |
| 2. Build catalog | By default, the command creates an in-memory MCP server with the real meta-tool registration code and reads its `tools/list` catalog through MCP in-memory transports. |
| 3. Mock GitLab bootstrap | The catalog build uses a tiny `httptest` GitLab server that only returns version metadata. It is not used to execute the evaluated GitLab actions. |
| 4. Send model request | In model-backed mode, the harness sends the catalog as Anthropic tool definitions plus a fixed system prompt and one wrapped task prompt. |
| 5. Simulate schema discovery | If the model calls `gitlab_server` with `schema_index` or `schema_get`, the harness returns the real locally derived schema index or action schema. |
| 6. Validate final calls | For normal tool calls, the harness checks tool name, action, required params, schema-exposed params, meta-tool envelope shape, standalone-tool shape, step order, and `confirm:true` for destructive routes. |
| 7. Simulate tool results | Valid non-final multi-step calls receive an `ok; continue` tool result. Invalid first attempts receive an error tool result so the model can repair once. `MF-*` rows can also inject one-time transient errors, 404 fallback paths, and untrusted tool output. |
| 8. Write report and traces | The report records route accuracy, first-pass rate, repair rate, schema lookup usage, destructive safety, token usage, cost, and catalog coverage. Model-backed runs also write per-case JSON traces and a `traces.jsonl` file for later LLM analysis. |

`--tools-file=/path/to/tools_meta.json` replaces step 2 with a saved `tools/list` snapshot, which is how the branch is compared against `main` without checking out or starting a second server.

`--dry-run` skips the model entirely and checks whether every expected route in the fixture exists in the selected catalog. With the live catalog, it also verifies fixture destructive flags and listed params against route metadata and action schemas.

## Harness

Run the automated harness from the repository root:

```bash
go run ./cmd/eval_meta_tools/ --dry-run --repeat=1 --out /tmp/eval-dry-run.md
```

For a model-backed run, set `ANTHROPIC_API_KEY` in the environment or `.env` and run:

```bash
go run ./cmd/eval_meta_tools/ \
  --model claude-sonnet-4-6 \
  --repeat=1 \
  --pause=250ms \
  --retries=8 \
  --retry-wait=65s \
  --out dist/evaluation/meta-tools/eval-anthropic.md
```

By default, model-backed reports and traces are written under `dist/evaluation/meta-tools/`, which is ignored by git. Keep full prompt/action trace samples there for local analysis instead of versioning them.

Useful flags:

| Flag | Use |
| --- | --- |
| `--dry-run` | Validate fixture routes against the local catalog without calling a model. |
| `--task=MT-091,MS-002` | Run a targeted subset. |
| `--repeat=N` | Repeat selected cases to check stability. |
| `--tools-file=/path/to/tools_meta.json` | Compare against a saved `tools/list` snapshot, such as the `main` branch catalog. |
| `--trace-dir=/path/to/traces` | Store model-backed per-case JSON traces, `traces.jsonl`, and `index.md`; defaults to `<report>.traces`, normally under ignored `dist/`. |
| `--pause=1s` | Add spacing between API calls when running a model-backed evaluation. |
| `--retries=8 --retry-wait=65s` | Survive transient model API throttling. |

Trace artifacts intentionally omit API keys and request headers. Each trace stores the exact system prompt, wrapped user prompt, expected route sequence, assistant `tool_use` blocks, simulated `tool_result` blocks, validation messages, simulation events, token usage for model responses, and the final summary.

MCP completions are not directly model-callable in this Anthropic tool-calling harness. Completion quality should be tested through MCP `completion/complete` client/server tests and prompt/resource argument completion tests; this harness can only evaluate the tools that appear in the model-visible `tools/list` catalog.

## Design Notes

The suite follows a hybrid of deterministic and model-backed evaluation practices:

| Practice | How this repository applies it |
| --- | --- |
| Versioned fixtures | The case matrix is checked into this directory and parsed by `cmd/eval_meta_tools`. |
| Trajectory checks | The harness validates expected tool, action, required params, step order, and destructive confirmation. |
| Failure injection | Dedicated `MF-*` cases simulate transient GitLab failure, missing resources, and prompt-injection text embedded in tool output. |
| Capability fallback | Dedicated `MF-*` cases simulate unsupported sampling and elicitation capabilities, then validate raw-tool or non-interactive fallback paths. |
| Outcome checks | Each case has a success verifier describing the expected user-visible outcome. |
| Safety gates | Destructive steps must list `confirm` and model calls must include `confirm:true` on the destructive route. |
| Reproducible reports | Reports include model name, request counts, token usage, cache usage, estimated cost, per-task status, and fixture coverage. |
| Trace artifacts | Model-backed runs produce JSON and JSONL transcripts that preserve the prompts, tool calls, validation messages, and simulated results for every case. |
| Full catalog coverage | The fixture covers all 48 advertised meta-tools in the enterprise opaque catalog. |

External references consulted while reorganizing this directory include MCP Evals, OpenAI Frontier Evals and nanoeval, promptfoo trajectory assertions, LangSmith dataset and tracing concepts, Inspect AI evaluation patterns, and Anthropic agent-evaluation guidance.
