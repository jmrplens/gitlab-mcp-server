# AI Model Evaluation Results

This document publishes the current model-evaluation results selected with
`cmd/eval_meta_tools --publish-docs`. Meta-tools and Dynamic 3-tool results are
kept in separate managed sections so publishing one surface does not replace the
other. Raw reports and traces are not committed.

## Meta-Tools Results

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

## Dynamic 2-Tool Results

<!-- START MODEL EVAL DYNAMIC2 RESULTS -->
No Dynamic 2-tool evaluation results have been published yet.
<!-- END MODEL EVAL DYNAMIC2 RESULTS -->

## Dynamic 3-Tool Results

<!-- START MODEL EVAL DYNAMIC3 RESULTS -->
### 2026-05-12 Dynamic all-models hardening

| Model | Preset | Backend | Attempts | Expected ops | Model requests | Tool calls emitted | Tool-selection | Action-selection | First-pass validation | Repair success | Destructive safety | Final task success | Cost/tokens | Commit / branch / date |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- | --- |
| `anthropic:claude-haiku-4-5-20251001` | `docker-read` | Docker GitLab via MCP | 40 | 56 | 70 | 70 | 100.0% | 100.0% | 100.0% | - | 100.0% | 100.0% | in 218806 / out 4996 | 812ffc085d52 / feature/dynamic-search-ranker-improvements / 2026-05-12T13:09:41Z |
| `google:gemini-3.1-flash-lite-preview` | `docker-read` | Docker GitLab via MCP | 40 | 56 | 66 | 66 | 100.0% | 97.5% | 97.5% | 0.0% (0/1) | 100.0% | 100.0% | in 340209 / out 2585 | 812ffc085d52 / feature/dynamic-search-ranker-improvements / 2026-05-12T13:09:41Z |
| `openai:gpt-5.4-nano` | `docker-read` | Docker GitLab via MCP | 40 | 56 | 64 | 64 | 100.0% | 100.0% | 100.0% | 0.0% (0/1) | 100.0% | 100.0% | in 232432 / out 2284 | 812ffc085d52 / feature/dynamic-search-ranker-improvements / 2026-05-12T13:09:41Z |
| `qwen:qwen3.6-flash` | `docker-read` | Docker GitLab via MCP | 40 | 56 | 80 | 80 | 100.0% | 100.0% | 100.0% | - | 100.0% | 100.0% | in 479943 / out 3664 | 812ffc085d52 / feature/dynamic-search-ranker-improvements / 2026-05-12T13:09:41Z |
| `anthropic:claude-haiku-4-5-20251001` | `docker-mutating-safe` | Docker GitLab via MCP | 30 | 34 | 37 | 37 | 100.0% | 100.0% | 100.0% | - | 100.0% | 100.0% | in 117028 / out 3491 | 812ffc085d52 / feature/dynamic-search-ranker-improvements / 2026-05-12T13:09:41Z |
| `google:gemini-3.1-flash-lite-preview` | `docker-mutating-safe` | Docker GitLab via MCP | 30 | 34 | 36 | 36 | 100.0% | 100.0% | 100.0% | - | 100.0% | 100.0% | in 96135 / out 2118 | 812ffc085d52 / feature/dynamic-search-ranker-improvements / 2026-05-12T13:09:41Z |
| `openai:gpt-5.4-nano` | `docker-mutating-safe` | Docker GitLab via MCP | 30 | 34 | 36 | 36 | 100.0% | 100.0% | 100.0% | - | 100.0% | 100.0% | in 85272 / out 1703 | 812ffc085d52 / feature/dynamic-search-ranker-improvements / 2026-05-12T13:09:41Z |
| `qwen:qwen3.6-flash` | `docker-mutating-safe` | Docker GitLab via MCP | 30 | 34 | 43 | 43 | 100.0% | 100.0% | 100.0% | 0.0% (0/1) | 100.0% | 100.0% | in 136836 / out 2618 | 812ffc085d52 / feature/dynamic-search-ranker-improvements / 2026-05-12T13:09:41Z |
| `anthropic:claude-haiku-4-5-20251001` | `docker-destructive-safe` | Docker GitLab via MCP | 53 | 156 | 191 | 191 | 100.0% | 100.0% | 100.0% | 0.0% (0/3) | 100.0% | 100.0% | in 500779 / out 18723 | 812ffc085d52 / feature/dynamic-search-ranker-improvements / 2026-05-12T13:09:41Z |
| `google:gemini-3.1-flash-lite-preview` | `docker-destructive-safe` | Docker GitLab via MCP | 53 | 156 | 168 | 168 | 100.0% | 100.0% | 100.0% | 0.0% (0/4) | 100.0% | 100.0% | in 591212 / out 11186 | 812ffc085d52 / feature/dynamic-search-ranker-improvements / 2026-05-12T13:09:41Z |
| `openai:gpt-5.4-nano` | `docker-destructive-safe` | Docker GitLab via MCP | 53 | 156 | 180 | 180 | 100.0% | 100.0% | 100.0% | 40.0% (2/5) | 100.0% | 100.0% | in 507459 / out 9050 | 812ffc085d52 / feature/dynamic-search-ranker-improvements / 2026-05-12T13:09:41Z |
| `qwen:qwen3.6-flash` | `docker-destructive-safe` | Docker GitLab via MCP | 53 | 156 | 250 | 250 | 100.0% | 100.0% | 100.0% | 33.3% (1/3) | 100.0% | 100.0% | in 983903 / out 14923 | 812ffc085d52 / feature/dynamic-search-ranker-improvements / 2026-05-12T13:09:41Z |
| `anthropic:claude-haiku-4-5-20251001` | `error-recovery` | Docker GitLab via MCP | 5 | 10 | 16 | 16 | 100.0% | 100.0% | 100.0% | 25.0% (1/4) | 100.0% | 100.0% | in 49934 / out 1136 | 812ffc085d52 / feature/dynamic-search-ranker-improvements / 2026-05-12T13:09:41Z |
| `google:gemini-3.1-flash-lite-preview` | `error-recovery` | Docker GitLab via MCP | 5 | 10 | 17 | 17 | 100.0% | 100.0% | 100.0% | 25.0% (1/4) | 100.0% | 100.0% | in 61768 / out 653 | 812ffc085d52 / feature/dynamic-search-ranker-improvements / 2026-05-12T13:09:41Z |
| `openai:gpt-5.4-nano` | `error-recovery` | Docker GitLab via MCP | 5 | 10 | 12 | 12 | 100.0% | 100.0% | 100.0% | 20.0% (1/5) | 100.0% | 100.0% | in 24200 / out 470 | 812ffc085d52 / feature/dynamic-search-ranker-improvements / 2026-05-12T13:09:41Z |
| `qwen:qwen3.6-flash` | `error-recovery` | Docker GitLab via MCP | 5 | 10 | 18 | 18 | 100.0% | 100.0% | 100.0% | 25.0% (1/4) | 100.0% | 100.0% | in 56525 / out 823 | 812ffc085d52 / feature/dynamic-search-ranker-improvements / 2026-05-12T13:09:41Z |
| **Aggregate** | **all selected** | - | **512** | **1024** | **1284** | **1284** | **100.0%** | **99.8%** | **99.8%** | **20.0% (7/35)** | **100.0%** | **100.0%** | - | - |

Published with `cmd/eval_meta_tools --publish-docs` from reviewed Markdown reports. Raw traces and JSON artifacts are not included here.
<!-- END MODEL EVAL DYNAMIC3 RESULTS -->
