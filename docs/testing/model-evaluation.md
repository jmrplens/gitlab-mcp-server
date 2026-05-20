# AI Model Evaluation

> **Diátaxis type**: Explanation
> **Audience**: Users, evaluators, maintainers
> **Prerequisites**: Basic understanding of MCP tools and GitLab operations

AI model evaluation measures whether a model can use `gitlab-mcp-server` as an
MCP tool provider. It is not a benchmark of prose quality. It is a benchmark of
tool use: choosing the right MCP tool, choosing the right action, placing
parameters in the correct schema, recovering from actionable errors, and
finishing the requested GitLab operation with the fewest necessary calls.

This matters because an MCP server is an interface for AI agents. A tool can be
correct for humans and still be hard for models to use if descriptions are
ambiguous, schemas are too large, aliases are missing, or errors do not explain
how to recover.

## What Is Evaluated

The evaluator uses natural-language tasks from
`cmd/eval_mcp_surfaces/testdata/automated-mcp-surface-cases.md`. Each row declares
the expected tool, action, required parameters, whether the task is destructive,
and the success condition.

| Case type           | Prefix | Purpose                                                                          |
| ------------------- | ------ | -------------------------------------------------------------------------------- |
| Single operation    | `MT-`  | One clear user task should usually require one model call and one MCP tool call. |
| Multi-step workflow | `MS-`  | The model must sequence multiple MCP calls in the requested order.               |
| Failure simulation  | `MF-`  | The model must recover from injected failures or unsafe output.                  |

The current automated corpus contains 159 cases and 300 expected tool
operations:

| Area                          | Count |
| ----------------------------- | ----: |
| Single-operation cases        |   117 |
| Multi-step workflow scenarios |    37 |
| Failure simulation scenarios  |     5 |
| Total cases                   |   159 |
| Expected tool operations      |   300 |

## Evaluation Modes

The evaluator runs against the same model-facing tool surfaces as the server.
`dynamic` is the default surface and exposes `gitlab_find_action` plus
`gitlab_execute_tool` over the canonical action catalog. `meta` exposes the
domain grouped meta-tools. The evaluator does not reduce the server to only the
manifest resources; when capability bridge tools are enabled they let the model
inspect the resources, prompts, completions, and capability metadata that a full
MCP session exposes.

The surface-aware `gitlab://tools` manifest is available in both surfaces. In
dynamic mode it lists canonical `domain.action` IDs accepted by
`gitlab_execute_tool`; in meta mode it lists `gitlab_<domain>.<action>` entries
and their `{action, params}` call shapes. Reading this manifest is useful for
capability-discovery tasks, but ordinary task success is measured by the final
GitLab operation, not by whether the model read the manifest first.

### Schema Evaluation

Schema evaluation calls real model providers with the MCP tool catalog, but it
does not execute GitLab operations. It validates whether the model can infer the
correct tool, action, and argument shape from the schema and descriptions.

Use schema evaluation when changing:

- Tool descriptions
- Meta-tool action names
- Parameter aliases
- Provider adapters
- Token-reduction strategies
- `META_PARAM_SCHEMA` behavior

The project currently keeps meta-tool params in opaque mode. Provider-specific
compatibility, such as Google Gemini validated function calling, is handled by
the evaluator/provider adapter rather than by changing the global MCP schema.

### Docker Evaluation

Docker evaluation runs the model against the real MCP server and an ephemeral,
populated GitLab CE instance. The model's validated tool calls are executed
through MCP, so failures can come from model choice, argument shape, GitLab API
state, permissions, or fixture gaps.

Docker evaluation is split into safe presets:

| Preset                    | Scope                     | Mutation policy                                                              |
| ------------------------- | ------------------------- | ---------------------------------------------------------------------------- |
| `docker-read`             | Read-only tasks           | No mutating or destructive operations.                                       |
| `docker-mutating-safe`    | Safe create/update tasks  | Mutates disposable Docker fixtures.                                          |
| `docker-destructive-safe` | Safe delete/archive tasks | Uses disposable or just-in-time fixtures and requires confirmation metadata. |

The Docker fixture base must contain all resources needed by successful tasks.
If a task is not intentionally testing an error, missing GitLab state is treated
as harness noise and should be fixed in fixtures before judging the model.

## Core Metrics

| Metric                          | Meaning                                                                                                                              |
| ------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------ |
| Tool-selection accuracy         | The first or final model call selected the expected MCP tool name.                                                                   |
| Action-selection accuracy       | The selected action matched the expected action inside an action-based meta-tool.                                                    |
| First-call validation pass rate | The first emitted tool call matched schema, required params, and destructive-safety requirements.                                    |
| Schema lookup use rate          | Percentage of attempts where the model used schema lookup before or during the task. Low is better for clear single-operation tasks. |
| Repair success rate             | Percentage of invalid first calls that were corrected after the tool returned an error.                                              |
| Destructive safety              | Destructive calls included the required confirmation and used the expected destructive route.                                        |
| Final task success proxy        | The evaluator's final success signal after validation and optional MCP execution.                                                    |
| Model requests                  | Number of provider calls made by the evaluator.                                                                                      |
| Tool calls emitted              | Number of tool calls emitted by the model.                                                                                           |
| MCP bridge calls                | Calls to evaluator bridge tools that represent MCP client capability access, such as reading resources or prompts.                   |

For clear single-operation tasks, the target is `model_calls=1` and
`tool_calls=1`. Extra calls are acceptable only when the prompt is genuinely
ambiguous, the task is multi-step, or a real GitLab error requires recovery.
For exact dynamic tasks, an extra `gitlab_find_action` or schema lookup usually
means the tool descriptions are costing context or calls; for ambiguous dynamic
tasks, using `gitlab_find_action` is expected.

## Failure Categories

Failures are useful only after separating model behavior from harness noise.
Use these categories when triaging traces:

| Category                   | Meaning                                                                  | Typical fix                                                        |
| -------------------------- | ------------------------------------------------------------------------ | ------------------------------------------------------------------ |
| Model route miss           | The model chose the wrong tool or action.                                | Improve descriptions, action names, examples, or aliases.          |
| Model parameter shape miss | The model chose the right route but emitted invalid params.              | Strengthen schema descriptions or add safe alias normalization.    |
| Provider adapter issue     | The provider API transformed or rejected a valid MCP schema.             | Fix the provider adapter without changing the global MCP contract. |
| Sampling unsupported       | The evaluator client did not advertise MCP sampling.                     | Add a deterministic `CreateMessageHandler` for evaluator clients.  |
| Fixture gap                | Docker GitLab lacks a resource the task expects.                         | Add initial or just-in-time fixture setup.                         |
| GitLab limitation          | The Docker GitLab edition does not support the API.                      | Filter or mark the route unavailable for that edition.             |
| MCP implementation bug     | The MCP handler fails despite valid model input and valid fixture state. | Fix the handler and add unit/E2E coverage.                         |

## Compatibility Expectations

The evaluator supports several provider families through adapters. A model is
compatible when it can receive the tool catalog, emit tool calls, preserve tool
call IDs across repair turns, and accept MCP-shaped JSON Schema.

| Provider  | Example model                          | Compatibility expectation                                             |
| --------- | -------------------------------------- | --------------------------------------------------------------------- |
| Anthropic | `anthropic:claude-sonnet-4-6`          | Supported.                                                            |
| Anthropic | `anthropic:claude-haiku-4-5-20251001`  | Supported.                                                            |
| Google    | `google:gemini-3.1-flash-lite-preview` | Supported with validated function-calling mode.                       |
| OpenAI    | `openai:gpt-5.4-mini`                  | Supported.                                                            |
| OpenAI    | `openai:gpt-5.4-nano`                  | Supported.                                                            |
| Qwen      | `qwen:qwen3.6-flash`                   | Supported through the OpenAI-compatible adapter using `QWEN_API_KEY`. |

Published percentages belong in [AI Model Evaluation Results](model-results.md),
not in this conceptual guide.

## Reading Results

Start with final success and first-call validation. If final success is high but
first-call validation is low, the model can recover but the schema or
description is still costing extra calls. If tool and action accuracy are high
but final success is low, inspect Docker fixture state and MCP execution errors.
If destructive safety is below 100%, treat it as a blocking issue before
running broader destructive evaluations.

For every failed model run, read the trace JSON in the report's `.traces/`
directory. The trace records the system prompt, user prompt, emitted tool call,
validation error, MCP result, and any repair attempt.

## Why Docker Mode Is Valuable

Schema-only evaluations can show that a model understands the catalog, but they
cannot prove the server works against GitLab. Docker mode closes that gap by
executing the actual MCP call against a populated GitLab instance. This catches
real problems such as missing sampling capability, GitLab API edge cases,
stale fixture IDs, destructive ordering, and provider-specific argument repair.
