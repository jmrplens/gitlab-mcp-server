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

- `dist/evaluation/meta-tools/full-economy-fixed-20260505T005559Z/docker-read.md`
- `dist/evaluation/meta-tools/full-economy-fixed-20260505T005559Z/docker-mutating-safe.md`
- `dist/evaluation/meta-tools/full-economy-corrected-20260505T060742Z/docker-destructive-safe.md`

Known harness noise removed before publishing: per-attempt destructive fixture
reseeding, archived project recovery, environment-scoped CI variable cleanup,
per-model lifecycle resource suffixing, project member cleanup, repeated package
ID prompt replacement, repository file branch handling, and repository file
create/update outputs enriched with current commit metadata.

| Preset | Attempts | Model requests | Tool calls | Tool accuracy | Action accuracy | First pass | Repair success | Destructive safety | Final success |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `docker-read` | 160 | 227 | 226 | 98.8% | 98.1% | 97.5% | 14.3% | 100.0% | 96.9% |
| `docker-mutating-safe` | 100 | 118 | 117 | 98.0% | 98.0% | 93.0% | 66.7% | 100.0% | 97.0% |
| `docker-destructive-safe` | 212 | 625 | 625 | 88.7% | 88.2% | 84.9% | 18.9% | 94.3% | 91.0% |
| **Aggregate** | **472** | **970** | **968** | Not aggregated | Not aggregated | Not aggregated | Not aggregated | Not aggregated | **94.3%** |

Per-model final success across the three Docker presets:

| Model | Attempts | Read | Mutating | Destructive | Aggregate final success |
| --- | ---: | ---: | ---: | ---: | ---: |
| `anthropic:claude-haiku-4-5-20251001` | 118 | 100.0% | 96.0% | 96.2% | 97.5% |
| `google:gemini-3.1-flash-lite-preview` | 118 | 95.0% | 92.0% | 81.1% | 88.1% |
| `openai:gpt-5.4-nano` | 118 | 95.0% | 100.0% | 90.6% | 94.1% |
| `qwen:qwen3.6-flash` | 118 | 97.5% | 100.0% | 96.2% | 97.5% |

Final-failure analysis after the harness fixes:

| Area | Final failures | Interpretation |
| --- | ---: | --- |
| Read-only | 5 / 160 | Route or action misses: Google on `MS-001`/`MS-012`, OpenAI on `MT-002`/`MS-012`, Qwen on `MS-012`. |
| Mutating-safe | 3 / 100 | `MT-036` release creation still trips Anthropic/Google on `params.ref`; Google `MT-061` emitted no tool call. |
| Destructive-safe | 19 / 212 | Mostly model route/step-order misses. Repeated hotspots: project snippet update requires a `files` array (`MS-024`), deploy-key workflow skips `deploy_key_get` (`MS-031`), and Google repeatedly chooses discovery/search before the requested mutating workflow. |

Final-failure inventory:

| Preset | Model | Task | Category | Observed cause |
| --- | --- | --- | --- | --- |
| `docker-read` | Google | `MS-001` | Route miss | Started with discovery, then used repository file read where project `get` was expected, then emitted no tool call. |
| `docker-read` | Google | `MS-012` | Route miss | Used project discovery/listing instead of `gitlab_release/list` and repository compare. |
| `docker-read` | OpenAI | `MT-002` | Parameter/repair miss | Chose project `get`, but with a non-existent project ID, then repaired to list instead of a valid get. |
| `docker-read` | OpenAI | `MS-012` | Route miss | Started discovery, then switched to `gitlab_analyze/release_notes` instead of repository compare. |
| `docker-read` | Qwen | `MS-012` | Route miss | Listed releases, then switched to `gitlab_analyze/release_notes` instead of repository compare. |
| `docker-mutating-safe` | Anthropic | `MT-036` | Parameter shape miss | Created a release without `params.ref`; GitLab rejected the missing ref. |
| `docker-mutating-safe` | Google | `MT-036` | Route and parameter miss | Chose tag creation before release creation and omitted `params.ref`. |
| `docker-mutating-safe` | Google | `MT-061` | Provider/model no-call | Returned no tool-use block for discussion resolution. |
| `docker-destructive-safe` | Anthropic | `MS-024` | Parameter shape miss | Project snippet update omitted the required `files` array, then moved to delete too early. |
| `docker-destructive-safe` | Anthropic | `MS-031` | Step-order miss | Added deploy key, then skipped the required `deploy_key_get` step. |
| `docker-destructive-safe` | Google | `MS-009` | Route miss | Used broadcast-message list/create where instance `settings_get` was the first expected step. |
| `docker-destructive-safe` | Google | `MS-013` | Route miss | Chose discovery/search instead of feature-flag get/list/delete workflow. |
| `docker-destructive-safe` | Google | `MS-016` | Route miss | Chose discovery/search instead of issue creation and issue-link workflow. |
| `docker-destructive-safe` | Google | `MS-018` | Route miss | Chose discovery/project listing instead of release creation from a ref. |
| `docker-destructive-safe` | Google | `MS-019` | Route miss | Repeated discovery instead of pipeline trigger creation. |
| `docker-destructive-safe` | Google | `MS-020` | Route miss | Repeated discovery instead of pipeline schedule creation. |
| `docker-destructive-safe` | Google | `MS-024` | Route and shape miss | Started with discovery and later omitted the snippet update `files` array. |
| `docker-destructive-safe` | Google | `MS-028` | Route miss | Repeated discovery instead of branch creation. |
| `docker-destructive-safe` | Google | `MS-031` | Route and step-order miss | Started with discovery, then skipped required `deploy_key_get`. |
| `docker-destructive-safe` | Google | `MS-032` | Route miss | Used discovery/project listing instead of issue create and time-tracking workflow. |
| `docker-destructive-safe` | OpenAI | `MS-021` | Parameter shape miss | Sent unsupported hook fields `member_events` and `subgroup_events`. |
| `docker-destructive-safe` | OpenAI | `MS-024` | Step-order miss | Used snippet content read where project snippet `get` was expected. |
| `docker-destructive-safe` | OpenAI | `MS-028` | Route and safety miss | Started with discovery and later missed destructive confirmation on a delete step. |
| `docker-destructive-safe` | OpenAI | `MS-031` | Route and step-order miss | Used project `get` instead of deploy-key add, then skipped required `deploy_key_get`. |
| `docker-destructive-safe` | OpenAI | `MS-032` | Step-order miss | Used time stats and estimate reset where spent-time reset was expected. |
| `docker-destructive-safe` | Qwen | `MS-024` | Parameter shape miss | Project snippet update put `file_path` outside the required `files` array. |
| `docker-destructive-safe` | Qwen | `MS-031` | Step-order miss | Added deploy key, then skipped the required `deploy_key_get` step. |

Interpretation: the split meta-tool catalog works with all four providers in
opaque mode against a real GitLab backend. Remaining failures are useful product
signals rather than fixture blockers: improve route descriptions, add targeted
aliases only where they preserve the MCP contract, and make complex update
shapes such as snippet `files` easier for models to infer.

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
