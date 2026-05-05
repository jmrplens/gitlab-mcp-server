# AI Model Evaluation Results

This document publishes only the current model-evaluation result selected with
`cmd/eval_meta_tools --publish-docs`. Raw reports and traces are not committed.

## Current Result

<!-- START MODEL EVAL RESULTS -->
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
<!-- END MODEL EVAL RESULTS -->
