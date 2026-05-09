# AI Model Evaluation Results

This document publishes the current model-evaluation results selected with
`cmd/eval_meta_tools --publish-docs`. Meta-tools and Dynamic 3-tool results are
kept in separate managed sections so publishing one surface does not replace the
other. Raw reports and traces are not committed.

## Meta-Tools Result

<!-- START MODEL EVAL META RESULTS -->
### 2026-05-05 Full Docker Economy Run

| Model | Preset | Backend | Attempts | Expected ops | Model requests | Tool calls emitted | Tool-selection | Action-selection | First-pass validation | Repair success | Destructive safety | Final task success | Cost/tokens | Commit / branch / date |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- | --- |
| `anthropic:claude-haiku-4-5-20251001` | `docker-read` | Docker GitLab via MCP | 40 | 56 | 56 | 56 | 100.0% | 100.0% | 100.0% | - | 100.0% | 100.0% | in 78457 / out 4324 | - / - / 2026-05-05T15:27:17Z |
| `google:gemini-3.1-flash-lite-preview` | `docker-read` | Docker GitLab via MCP | 40 | 56 | 56 | 56 | 100.0% | 100.0% | 100.0% | - | 100.0% | 100.0% | in 2764442 / out 2186 | - / - / 2026-05-05T15:28:40Z |
| `openai:gpt-5.4-nano` | `docker-read` | Docker GitLab via MCP | 40 | 56 | 56 | 56 | 100.0% | 100.0% | 100.0% | - | 100.0% | 100.0% | in 2106803 / out 2006 | - / - / 2026-05-05T15:38:09Z |
| `qwen:qwen3.6-flash` | `docker-read` | Docker GitLab via MCP | 40 | 56 | 56 | 56 | 100.0% | 100.0% | 100.0% | - | 100.0% | 100.0% | in 2402051 / out 2948 | - / - / 2026-05-05T15:41:48Z |
| `anthropic:claude-haiku-4-5-20251001` | `docker-mutating-safe` | Docker GitLab via MCP | 25 | 28 | 28 | 28 | 100.0% | 100.0% | 100.0% | - | 100.0% | 100.0% | in 41395 / out 2889 | - / - / 2026-05-05T15:42:57Z |
| `google:gemini-3.1-flash-lite-preview` | `docker-mutating-safe` | Docker GitLab via MCP | 25 | 28 | 28 | 28 | 100.0% | 100.0% | 100.0% | - | 100.0% | 100.0% | in 1343665 / out 1756 | 8c696a2d / port/main-small-meta-fixes / 2026-05-05T19:13:44Z |
| `openai:gpt-5.4-nano` | `docker-mutating-safe` | Docker GitLab via MCP | 25 | 28 | 28 | 28 | 100.0% | 100.0% | 100.0% | - | 100.0% | 100.0% | in 1046211 / out 1466 | - / - / 2026-05-05T15:49:03Z |
| `qwen:qwen3.6-flash` | `docker-mutating-safe` | Docker GitLab via MCP | 25 | 28 | 28 | 28 | 100.0% | 100.0% | 100.0% | - | 100.0% | 100.0% | in 1192488 / out 2014 | - / - / 2026-05-05T15:52:15Z |
| `anthropic:claude-haiku-4-5-20251001` | `docker-destructive-safe` | Docker GitLab via MCP | 53 | 156 | 156 | 156 | 100.0% | 100.0% | 100.0% | - | 100.0% | 100.0% | in 147403 / out 16059 | - / - / 2026-05-05T15:56:38Z |
| `google:gemini-3.1-flash-lite-preview` | `docker-destructive-safe` | Docker GitLab via MCP | 53 | 156 | 157 | 157 | 100.0% | 100.0% | 100.0% | 100.0% (1/1) | 100.0% | 100.0% | in 7591720 / out 10405 | - / - / 2026-05-05T16:00:18Z |
| `openai:gpt-5.4-nano` | `docker-destructive-safe` | Docker GitLab via MCP | 53 | 156 | 160 | 160 | 100.0% | 100.0% | 98.1% | 50.0% (2/4) | 100.0% | 100.0% | in 5979869 / out 8793 | 8c696a2d / port/main-small-meta-fixes / 2026-05-05T19:44:05Z |
| **Aggregate** | **all selected** | - | **419** | **804** | **809** | **809** | **100.0%** | **100.0%** | **99.8%** | **60.0% (3/5)** | **100.0%** | **100.0%** | - | - |

Published with `cmd/eval_meta_tools --publish-docs` from reviewed Markdown reports. Raw traces and JSON artifacts are not included here.
<!-- END MODEL EVAL META RESULTS -->

## Dynamic 3-Tool Result

<!-- START MODEL EVAL DYNAMIC3 RESULTS -->
### Dynamic 3-tool all-provider Docker run 2026-05-09

| Model | Preset | Backend | Attempts | Expected ops | Model requests | Tool calls emitted | Tool-selection | Action-selection | First-pass validation | Repair success | Destructive safety | Final task success | Cost/tokens | Commit / branch / date |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- | --- |
| `anthropic:claude-haiku-4-5-20251001` | `docker-read` | Docker GitLab via MCP | 40 | 56 | 63 | 63 | 100.0% | 100.0% | 100.0% | - | 100.0% | 100.0% | in 221506 / out 4712 | 65fcba4c047d / feature/dynamic-toolset-low-token / 2026-05-09T16:15:48Z |
| `google:gemini-3.1-flash-lite-preview` | `docker-read` | Docker GitLab via MCP | 40 | 56 | 67 | 67 | 100.0% | 100.0% | 100.0% | 0.0% (0/1) | 100.0% | 100.0% | in 372509 / out 2616 | 65fcba4c047d / feature/dynamic-toolset-low-token / 2026-05-09T16:15:48Z |
| `openai:gpt-5.4-nano` | `docker-read` | Docker GitLab via MCP | 40 | 56 | 75 | 75 | 100.0% | 100.0% | 100.0% | 0.0% (0/3) | 100.0% | 100.0% | in 287112 / out 2613 | 65fcba4c047d / feature/dynamic-toolset-low-token / 2026-05-09T16:15:48Z |
| `qwen:qwen3.6-flash` | `docker-read` | Docker GitLab via MCP | 40 | 56 | 81 | 81 | 100.0% | 100.0% | 100.0% | - | 100.0% | 100.0% | in 386269 / out 3669 | 65fcba4c047d / feature/dynamic-toolset-low-token / 2026-05-09T16:15:48Z |
| `anthropic:claude-haiku-4-5-20251001` | `docker-mutating-safe` | Docker GitLab via MCP | 30 | 34 | 41 | 41 | 100.0% | 100.0% | 100.0% | 100.0% (2/2) | 100.0% | 100.0% | in 128253 / out 3837 | 65fcba4c047d / feature/dynamic-toolset-low-token / 2026-05-09T16:15:48Z |
| `google:gemini-3.1-flash-lite-preview` | `docker-mutating-safe` | Docker GitLab via MCP | 30 | 34 | 37 | 37 | 100.0% | 100.0% | 100.0% | - | 100.0% | 100.0% | in 109001 / out 2116 | 65fcba4c047d / feature/dynamic-toolset-low-token / 2026-05-09T16:15:48Z |
| `openai:gpt-5.4-nano` | `docker-mutating-safe` | Docker GitLab via MCP | 30 | 34 | 46 | 46 | 100.0% | 100.0% | 100.0% | 100.0% (4/4) | 100.0% | 100.0% | in 113096 / out 2032 | 65fcba4c047d / feature/dynamic-toolset-low-token / 2026-05-09T16:15:48Z |
| `qwen:qwen3.6-flash` | `docker-mutating-safe` | Docker GitLab via MCP | 30 | 34 | 59 | 59 | 100.0% | 100.0% | 100.0% | - | 100.0% | 100.0% | in 189451 / out 3000 | 65fcba4c047d / feature/dynamic-toolset-low-token / 2026-05-09T16:15:48Z |
| `anthropic:claude-haiku-4-5-20251001` | `docker-destructive-safe` | Docker GitLab via MCP | 53 | 156 | 184 | 184 | 100.0% | 100.0% | 100.0% | 33.3% (1/3) | 100.0% | 100.0% | in 500396 / out 18360 | 65fcba4c047d / feature/dynamic-toolset-low-token / 2026-05-09T16:15:48Z |
| `google:gemini-3.1-flash-lite-preview` | `docker-destructive-safe` | Docker GitLab via MCP | 53 | 156 | 172 | 172 | 100.0% | 100.0% | 100.0% | 0.0% (0/7) | 100.0% | 100.0% | in 589122 / out 11177 | 65fcba4c047d / feature/dynamic-toolset-low-token / 2026-05-09T16:15:48Z |
| `openai:gpt-5.4-nano` | `docker-destructive-safe` | Docker GitLab via MCP | 53 | 156 | 201 | 201 | 100.0% | 100.0% | 100.0% | 43.8% (7/16) | 100.0% | 100.0% | in 548833 / out 10106 | 65fcba4c047d / feature/dynamic-toolset-low-token / 2026-05-09T16:15:48Z |
| `qwen:qwen3.6-flash` | `docker-destructive-safe` | Docker GitLab via MCP | 53 | 156 | 303 | 303 | 100.0% | 100.0% | 100.0% | - | 100.0% | 100.0% | in 1183026 / out 16258 | 65fcba4c047d / feature/dynamic-toolset-low-token / 2026-05-09T16:15:48Z |
| `anthropic:claude-haiku-4-5-20251001` | `error-recovery` | Docker GitLab via MCP | 5 | 10 | 13 | 13 | 100.0% | 100.0% | 100.0% | 25.0% (1/4) | 100.0% | 100.0% | in 36257 / out 1043 | 65fcba4c047d / feature/dynamic-toolset-low-token / 2026-05-09T16:15:48Z |
| `google:gemini-3.1-flash-lite-preview` | `error-recovery` | Docker GitLab via MCP | 5 | 10 | 16 | 16 | 100.0% | 100.0% | 100.0% | 25.0% (1/4) | 100.0% | 100.0% | in 51907 / out 634 | 65fcba4c047d / feature/dynamic-toolset-low-token / 2026-05-09T16:15:48Z |
| `openai:gpt-5.4-nano` | `error-recovery` | Docker GitLab via MCP | 5 | 10 | 15 | 15 | 100.0% | 100.0% | 100.0% | 25.0% (1/4) | 100.0% | 100.0% | in 34705 / out 549 | 65fcba4c047d / feature/dynamic-toolset-low-token / 2026-05-09T16:15:48Z |
| `qwen:qwen3.6-flash` | `error-recovery` | Docker GitLab via MCP | 5 | 10 | 24 | 24 | 100.0% | 100.0% | 100.0% | 25.0% (1/4) | 100.0% | 100.0% | in 71448 / out 992 | 65fcba4c047d / feature/dynamic-toolset-low-token / 2026-05-09T16:15:48Z |
| **Aggregate** | **all selected** | - | **512** | **1024** | **1397** | **1397** | **100.0%** | **100.0%** | **100.0%** | **34.6% (18/52)** | **100.0%** | **100.0%** | - | - |

Published with `cmd/eval_meta_tools --publish-docs` from reviewed Markdown reports. Raw traces and JSON artifacts are not included here.
<!-- END MODEL EVAL DYNAMIC3 RESULTS -->
