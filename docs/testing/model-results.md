# AI Model Evaluation Results

> **Diataxis type**: Reference
> **Audience**: Users, maintainers, release reviewers
> **Prerequisites**: Familiarity with [AI Model Evaluation](model-evaluation.md)

This document records curated model-evaluation snapshots. Raw reports and traces
remain under `dist/evaluation/meta-tools/` and are not committed. Copy only the
high-signal metrics here after a run has been inspected and known harness noise
has been removed or explicitly called out.

## Corpus Summary

Source fixture: `cmd/eval_meta_tools/testdata/automated-meta-tool-cases.md`.

| Area | Count |
| --- | ---: |
| Single-operation meta-tool cases | 117 |
| Multi-step workflow scenarios | 37 |
| Failure simulation scenarios | 5 |
| Total automated cases | 159 |
| Expected tool operations across all cases | 300 |

For clear single-operation rows, the target is one model request and one MCP
tool call. Multi-step and failure-recovery rows may legitimately use more calls,
but the expected operation count remains fixed by the fixture.

## Compatibility Matrix

Compatibility means the evaluator can send the catalog to the provider, receive
tool calls, preserve repair-turn tool IDs, and validate MCP-shaped arguments.
It does not mean every benchmark run has 100% task success.

| Provider | Model | Adapter status | Notes |
| --- | --- | --- | --- |
| Anthropic | `anthropic:claude-sonnet-4-6` | Compatible | Native tool calling. |
| Anthropic | `anthropic:claude-haiku-4-5-20251001` | Compatible | Native tool calling. |
| Google | `google:gemini-3.1-flash-lite-preview` | Compatible | Economy Gemini 3 option; uses validated function-calling mode for opaque meta-tool params. |
| OpenAI | `openai:gpt-5.4-mini` | Compatible | Chat Completions tool calling. |
| OpenAI | `openai:gpt-5.4-nano` | Compatible | Cheapest standard model used for broad Docker smoke passes. |
| Qwen | `qwen:qwen3.6-flash` | Compatible | OpenAI-compatible endpoint with thinking disabled by adapter; uses `QWEN_API_KEY`. |

## Published Snapshots

### 2026-05-05 Full Docker Economy Matrix

Purpose: validate the 33-tool Community Edition meta-tool catalog against a
real, populated Docker GitLab CE backend using low-cost provider models. This
snapshot uses opaque meta-tool parameter schemas; provider-specific schema
compatibility remains adapter-side.

Source reports:

- `dist/evaluation/meta-tools/full-economy-clean-d658228e-20260505T163400Z/*.md`
- `dist/evaluation/meta-tools/full-economy-incremental-f21c9de4-20260505T173232Z/*.md`
- `dist/evaluation/meta-tools/openai-mutating-mt068-instance-create-20260505T173101Z.md`
- `dist/evaluation/meta-tools/google-destructive-ms030-deploy-token-date-20260505T173132Z.md`
- `dist/evaluation/meta-tools/openai-destructive-focused-cleanup-20260505T181032Z.md`
- `dist/evaluation/meta-tools/qwen-destructive-focused-cleanup-20260505T181032Z.md`
- `dist/evaluation/meta-tools/qwen-destructive-confirm-envelope-20260505T181828Z.md`

The focal reports supersede only the task rows changed after the full matrix
started. Unaffected preset/model pairs keep their full-run results to avoid
unnecessary provider calls. Known harness and prompt-shaping noise removed
before publishing: per-attempt destructive fixture reseeding, archived project
recovery, environment-scoped CI variable cleanup, per-model lifecycle resource
suffixing, project member cleanup, repeated package ID prompt replacement,
repository file branch/ref handling, repository file create/update outputs,
instance CI variable action disambiguation, deploy-token date handling,
destructive `confirm` envelope placement, project badge URL defaults, project
snippet `files` update shape, issue-link cleanup IDs, branch unprotect cleanup,
and group milestone lifecycle ordering.

| Preset | Attempts | Expected ops | Model requests | Tool calls | Tool accuracy | Action accuracy | First pass | Repair success | Destructive safety | Final success |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `docker-read` | 160 | 224 | 224 | 224 | 100.0% | 100.0% | 100.0% | 100.0% | 100.0% | 100.0% |
| `docker-mutating-safe` | 100 | 112 | 112 | 112 | 100.0% | 100.0% | 100.0% | 100.0% | 100.0% | 100.0% |
| `docker-destructive-safe` | 212 | 624 | 624 | 624 | 100.0% | 100.0% | 100.0% | 100.0% | 100.0% | 100.0% |
| **Aggregate** | **472** | **960** | **960** | **960** | **100.0%** | **100.0%** | **100.0%** | **100.0%** | **100.0%** | **100.0%** |

Per-model final success across the three Docker presets:

| Model | Attempts | Expected ops | Read | Mutating | Destructive | Aggregate final success |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| `anthropic:claude-haiku-4-5-20251001` | 118 | 240 | 100.0% | 100.0% | 100.0% | 100.0% |
| `google:gemini-3.1-flash-lite-preview` | 118 | 240 | 100.0% | 100.0% | 100.0% | 100.0% |
| `openai:gpt-5.4-nano` | 118 | 240 | 100.0% | 100.0% | 100.0% | 100.0% |
| `qwen:qwen3.6-flash` | 118 | 240 | 100.0% | 100.0% | 100.0% | 100.0% |

Result analysis after the harness and prompt fixes:

| Area | Final failures | Interpretation |
| --- | ---: | --- |
| Read-only | 0 / 160 | All models selected the expected tool/action sequence and completed each read workflow. |
| Mutating-safe | 0 / 100 | All single-operation writes and safe mutating workflows completed with one call per expected operation. |
| Destructive-safe | 0 / 212 | All destructive workflows kept confirmation inside `params`, preserved returned IDs/IIDs, and cleaned up fixtures. |

Interpretation: the split 33-tool CE meta-tool catalog works with all four
providers in opaque mode against a real Docker GitLab backend. The curated
economy matrix now reaches the target behavior: each expected operation maps to
one model request and one MCP tool call, with no schema lookups, no repair turns,
and no final task failures in the published rows.

### 2026-05-04 Targeted Docker Noise-Fix Validation

Purpose: verify fixes for evaluator sampling support, Docker just-in-time
fixtures, project star EOF fallback, and meta-tool envelope clarity.

Source report:
`dist/evaluation/meta-tools/targeted-noise-fixes-openai-gpt-5.4-nano-rerun.md`.

| Field | Value |
| --- | ---: |
| Model | `openai:gpt-5.4-nano` |
| Backend | Docker GitLab CE through MCP execution |
| Catalog tools | 33 |
| Task attempts | 21 |
| Expected tool operations | 21 |
| Model requests | 21 |
| Tool calls emitted | 21 |
| Tool-selection accuracy | 100.0% |
| Action-selection accuracy | 100.0% |
| First-call validation pass rate | 100.0% |
| Schema lookup use rate | 0.0% |
| Repair success rate | 100.0% |
| Destructive safety | 100.0% |
| Final task success proxy | 100.0% |

Task-level result:

| Task set | Tasks | Success | Calls/tools target |
| --- | --- | ---: | --- |
| Core route fixes | `MT-001`, `MT-004`, `MT-032`, `MT-179` | 4 / 4 | All 1 model call and 1 tool call. |
| Docker fixture fixes | `MT-013`, `MT-027`, `MT-031`, `MT-035`, `MT-037`, `MT-044`, `MT-047`, `MT-051`, `MT-057`, `MT-059` | 10 / 10 | All 1 model call and 1 tool call. |
| Sampling fixes | `MT-039`, `MT-093`, `MT-094`, `MT-095`, `MT-096`, `MT-097`, `MT-098` | 7 / 7 | All 1 model call and 1 tool call. |

Interpretation: this run removes harness noise from the targeted failure set.
Failures in later full Docker runs for these tasks should be investigated as
model behavior, provider behavior, or a new fixture regression.

### Historical Full Docker Nano Baseline Before Noise Fixes

Purpose: baseline captured before the targeted fixture, sampling, EOF, and
envelope fixes. Keep this snapshot as evidence of the improvement opportunity;
do not use it as the current compatibility score.

| Preset | Attempts | Final success | Tool accuracy | Action accuracy | Destructive safety |
| --- | ---: | ---: | ---: | ---: | ---: |
| `docker-read` | 41 | 63.4% | 95.1% | 87.8% | Not applicable |
| `docker-mutating-safe` | 27 | 77.8% | 92.6% | 85.2% | Not applicable |
| `docker-destructive-safe` | 54 | 63.0% | 85.2% | 85.2% | 96.3% |
| **Aggregate** | **122** | **66.4%** | Not published | Not published | Not published |

Interpretation: most failures in this baseline mixed model behavior with
evaluator/client limitations and fixture gaps. The targeted Docker validation
above demonstrates that those known noise sources can be removed.

### Google Opaque Params Compatibility Probe

Purpose: verify the Google provider adapter can use the opaque meta-tool schema
without changing the global MCP schema.

| Probe | Model | Task attempts | Tool accuracy | Final success | Result |
| --- | --- | ---: | ---: | ---: | --- |
| Opaque params without validated mode | `google:gemini-3.1-pro-preview` | 1 | 0.0% | 0.0% | Rejected or emitted unusable params. |
| Opaque params with validated mode | `google:gemini-3.1-pro-preview` | 1 | 100.0% | 100.0% | Compatible. |

Interpretation: Google compatibility belongs in the provider adapter. The MCP
catalog remains in opaque meta-tool param mode.

## Full-Run Result Schema

Use these columns when publishing a full schema or Docker benchmark:

| Column | Meaning |
| --- | --- |
| Model | Exact `provider:model` value used by the evaluator. |
| Mode | `schema` or `docker`. |
| Preset | `schema-enterprise`, `docker-read`, `docker-mutating-safe`, or `docker-destructive-safe`. |
| Attempts | Number of task attempts in the report. |
| Expected ops | Sum of expected tool operations for the selected task set. |
| Model requests | Number of provider calls made. |
| Tool calls emitted | Number of tool calls emitted by the model. |
| Tool accuracy | Tool-selection accuracy from the report. |
| Action accuracy | Action-selection accuracy from the report. |
| First-pass validation | First-call validation pass rate from the report. |
| Repair success | Repair success rate from the report. |
| Destructive safety | Destructive safety from the report, when applicable. |
| Final success | Final task success proxy from the report. |

When copying a generated report into this document, include enough context to
answer these questions:

- Which commit or branch produced the run?
- Which model IDs and provider adapters were used?
- Which preset or task subset ran?
- How many tool operations were expected?
- How many model requests and tool calls were emitted?
- Which failures were model failures, fixture failures, provider adapter
  failures, or GitLab edition limitations?
- Did clear single-operation tasks stay at one model request and one MCP tool
  call?
