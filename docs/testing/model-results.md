# AI Model Evaluation Results

This document publishes the current model-evaluation results selected with
`cmd/eval_mcp_surfaces --publish-docs`. Meta-tools and Dynamic 3-tool results are
kept in separate managed sections so publishing one surface does not replace the
other. Raw reports and traces are not committed.

## Meta-Tools Results

<!-- START MODEL EVAL META RESULTS -->
### 2026-05-13 Docker CE meta opaque full plus reactivated

| Model | Preset | Backend | Attempts | Expected ops | Model requests | Tool calls emitted | Tool-selection | Action-selection | First-pass validation | Repair success | Destructive safety | Final task success | Cost/tokens | Commit / branch / date |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- | --- |
| `anthropic:claude-haiku-4-5-20251001` | `docker-read` | Docker GitLab via MCP | 40 | 56 | 56 | 56 | 100.0% | 100.0% | 100.0% | - | 100.0% | 100.0% | in 66022 / out 4322 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T18:08:00Z |
| `google:gemini-3.1-flash-lite-preview` | `docker-read` | Docker GitLab via MCP | 40 | 56 | 56 | 55 | 97.5% | 97.5% | 97.5% | - | 100.0% | 97.5% | in 2679525 / out 2157 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T18:08:00Z |
| `openai:gpt-5.4-nano` | `docker-read` | Docker GitLab via MCP | 40 | 56 | 58 | 58 | 100.0% | 97.5% | 97.5% | 50.0% (1/2) | 100.0% | 100.0% | in 2290935 / out 2076 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T18:08:00Z |
| `qwen:qwen3.6-flash` | `docker-read` | Docker GitLab via MCP | 40 | 56 | 56 | 56 | 100.0% | 100.0% | 100.0% | - | 100.0% | 100.0% | in 2357159 / out 2965 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T18:08:00Z |
| `anthropic:claude-haiku-4-5-20251001` | `docker-mutating-safe` | Docker GitLab via MCP | 32 | 36 | 36 | 36 | 100.0% | 100.0% | 100.0% | - | 100.0% | 100.0% | in 48505 / out 3370 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T18:08:00Z |
| `google:gemini-3.1-flash-lite-preview` | `docker-mutating-safe` | Docker GitLab via MCP | 32 | 36 | 36 | 36 | 100.0% | 100.0% | 100.0% | - | 100.0% | 100.0% | in 1692057 / out 2025 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T18:08:00Z |
| `openai:gpt-5.4-nano` | `docker-mutating-safe` | Docker GitLab via MCP | 32 | 36 | 36 | 36 | 100.0% | 96.9% | 96.9% | 0.0% (0/2) | 100.0% | 96.9% | in 1412893 / out 1675 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T18:08:00Z |
| `qwen:qwen3.6-flash` | `docker-mutating-safe` | Docker GitLab via MCP | 32 | 36 | 36 | 36 | 100.0% | 100.0% | 100.0% | - | 100.0% | 100.0% | in 1503519 / out 2298 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T18:08:00Z |
| `anthropic:claude-haiku-4-5-20251001` | `docker-destructive-safe` | Docker GitLab via MCP | 62 | 168 | 168 | 168 | 100.0% | 100.0% | 100.0% | - | 100.0% | 100.0% | in 162775 / out 17123 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T18:08:00Z |
| `google:gemini-3.1-flash-lite-preview` | `docker-destructive-safe` | Docker GitLab via MCP | 62 | 168 | 169 | 168 | 98.4% | 98.4% | 98.4% | 100.0% (1/1) | 100.0% | 98.4% | in 8037725 / out 10594 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T18:08:00Z |
| `openai:gpt-5.4-nano` | `docker-destructive-safe` | Docker GitLab via MCP | 62 | 168 | 168 | 168 | 98.4% | 98.4% | 96.8% | 0.0% (0/3) | 98.4% | 98.4% | in 6621370 / out 8827 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T18:08:00Z |
| `qwen:qwen3.6-flash` | `docker-destructive-safe` | Docker GitLab via MCP | 62 | 168 | 172 | 172 | 100.0% | 100.0% | 100.0% | 0.0% (0/2) | 100.0% | 98.4% | in 7222265 / out 12521 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T18:08:00Z |
| `anthropic:claude-haiku-4-5-20251001` | `error-recovery` | Docker GitLab via MCP | 5 | 10 | 11 | 11 | 100.0% | 100.0% | 100.0% | 25.0% (1/4) | 100.0% | 100.0% | in 10524 / out 897 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T18:08:00Z |
| `google:gemini-3.1-flash-lite-preview` | `error-recovery` | Docker GitLab via MCP | 5 | 10 | 11 | 11 | 100.0% | 100.0% | 100.0% | 25.0% (1/4) | 100.0% | 100.0% | in 512846 / out 482 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T18:08:00Z |
| `openai:gpt-5.4-nano` | `error-recovery` | Docker GitLab via MCP | 5 | 10 | 11 | 11 | 100.0% | 100.0% | 100.0% | 25.0% (1/4) | 100.0% | 100.0% | in 429141 / out 414 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T18:08:00Z |
| `qwen:qwen3.6-flash` | `error-recovery` | Docker GitLab via MCP | 5 | 10 | 12 | 12 | 100.0% | 80.0% | 80.0% | 25.0% (1/4) | 100.0% | 100.0% | in 498945 / out 656 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T18:08:00Z |
| `anthropic:claude-haiku-4-5-20251001` | `schema-enterprise` | Docker GitLab via MCP | 1 | 1 | 1 | 1 | 100.0% | 100.0% | 100.0% | - | 100.0% | 100.0% | in 1408 / out 104 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T18:08:00Z |
| `google:gemini-3.1-flash-lite-preview` | `schema-enterprise` | Docker GitLab via MCP | 1 | 1 | 1 | 1 | 100.0% | 100.0% | 100.0% | - | 100.0% | 100.0% | in 47000 / out 70 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T18:08:00Z |
| `openai:gpt-5.4-nano` | `schema-enterprise` | Docker GitLab via MCP | 1 | 1 | 1 | 1 | 100.0% | 100.0% | 100.0% | - | 100.0% | 100.0% | in 39352 / out 51 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T18:08:00Z |
| `qwen:qwen3.6-flash` | `schema-enterprise` | Docker GitLab via MCP | 1 | 1 | 1 | 1 | 100.0% | 100.0% | 100.0% | - | 100.0% | 100.0% | in 41848 / out 82 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T18:08:00Z |
| **Aggregate** | **all selected** | - | **560** | **1084** | **1096** | **1094** | **99.5%** | **98.9%** | **98.8%** | **23.1% (6/26)** | **99.8%** | **99.1%** | - | - |

Published with `cmd/eval_mcp_surfaces --publish-docs` from reviewed Markdown reports. Raw traces and JSON artifacts are not included here.
<!-- END MODEL EVAL META RESULTS -->

## Dynamic 2-Tool Results

<!-- START MODEL EVAL DYNAMIC2 RESULTS -->
No Dynamic 2-tool evaluation results have been published yet.
<!-- END MODEL EVAL DYNAMIC2 RESULTS -->

## Dynamic 3-Tool Results

<!-- START MODEL EVAL DYNAMIC3 RESULTS -->
### 2026-05-13 Docker CE dynamic-3 full reactivated

| Model | Preset | Backend | Attempts | Expected ops | Model requests | Tool calls emitted | Tool-selection | Action-selection | First-pass validation | Repair success | Destructive safety | Final task success | Cost/tokens | Commit / branch / date |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- | --- |
| `anthropic:claude-haiku-4-5-20251001` | `docker-read` | Docker GitLab via MCP | 40 | 56 | 69 | 69 | 100.0% | 100.0% | 100.0% | 0.0% (0/1) | 100.0% | 100.0% | in 218427 / out 4990 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T19:57:05Z |
| `google:gemini-3.1-flash-lite-preview` | `docker-read` | Docker GitLab via MCP | 40 | 56 | 67 | 67 | 100.0% | 97.5% | 97.5% | 0.0% (0/2) | 100.0% | 100.0% | in 350940 / out 2615 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T19:57:05Z |
| `openai:gpt-5.4-nano` | `docker-read` | Docker GitLab via MCP | 40 | 56 | 66 | 66 | 100.0% | 100.0% | 100.0% | 0.0% (0/1) | 100.0% | 100.0% | in 236661 / out 2365 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T19:57:05Z |
| `qwen:qwen3.6-flash` | `docker-read` | Docker GitLab via MCP | 40 | 56 | 80 | 80 | 100.0% | 100.0% | 100.0% | - | 100.0% | 100.0% | in 476187 / out 3658 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T19:57:05Z |
| `anthropic:claude-haiku-4-5-20251001` | `docker-mutating-safe` | Docker GitLab via MCP | 32 | 36 | 39 | 39 | 100.0% | 100.0% | 100.0% | - | 100.0% | 100.0% | in 123378 / out 3679 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T19:57:05Z |
| `google:gemini-3.1-flash-lite-preview` | `docker-mutating-safe` | Docker GitLab via MCP | 32 | 36 | 39 | 39 | 100.0% | 96.9% | 96.9% | 0.0% (0/1) | 100.0% | 100.0% | in 103721 / out 2295 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T19:57:05Z |
| `openai:gpt-5.4-nano` | `docker-mutating-safe` | Docker GitLab via MCP | 32 | 36 | 38 | 38 | 100.0% | 100.0% | 100.0% | - | 100.0% | 100.0% | in 89864 / out 1796 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T19:57:05Z |
| `qwen:qwen3.6-flash` | `docker-mutating-safe` | Docker GitLab via MCP | 32 | 36 | 46 | 46 | 100.0% | 100.0% | 100.0% | 0.0% (0/1) | 100.0% | 100.0% | in 145878 / out 2776 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T19:57:05Z |
| `anthropic:claude-haiku-4-5-20251001` | `docker-destructive-safe` | Docker GitLab via MCP | 63 | 169 | 209 | 209 | 100.0% | 100.0% | 100.0% | 33.3% (1/3) | 100.0% | 100.0% | in 561566 / out 20119 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T19:57:05Z |
| `google:gemini-3.1-flash-lite-preview` | `docker-destructive-safe` | Docker GitLab via MCP | 63 | 169 | 182 | 182 | 100.0% | 100.0% | 100.0% | 0.0% (0/2) | 100.0% | 100.0% | in 672949 / out 11671 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T19:57:05Z |
| `openai:gpt-5.4-nano` | `docker-destructive-safe` | Docker GitLab via MCP | 63 | 169 | 194 | 194 | 100.0% | 100.0% | 100.0% | 22.2% (2/9) | 100.0% | 98.4% | in 489389 / out 9973 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T19:57:05Z |
| `qwen:qwen3.6-flash` | `docker-destructive-safe` | Docker GitLab via MCP | 63 | 169 | 260 | 260 | 100.0% | 100.0% | 100.0% | 0.0% (0/2) | 100.0% | 100.0% | in 1010575 / out 15700 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T19:57:05Z |
| `anthropic:claude-haiku-4-5-20251001` | `error-recovery` | Docker GitLab via MCP | 5 | 10 | 16 | 16 | 100.0% | 100.0% | 100.0% | 25.0% (1/4) | 100.0% | 100.0% | in 47009 / out 1134 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T19:57:05Z |
| `google:gemini-3.1-flash-lite-preview` | `error-recovery` | Docker GitLab via MCP | 5 | 10 | 17 | 17 | 100.0% | 100.0% | 100.0% | 25.0% (1/4) | 100.0% | 100.0% | in 62614 / out 653 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T19:57:05Z |
| `openai:gpt-5.4-nano` | `error-recovery` | Docker GitLab via MCP | 5 | 10 | 12 | 12 | 100.0% | 100.0% | 100.0% | 25.0% (1/4) | 100.0% | 100.0% | in 25211 / out 468 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T19:57:05Z |
| `qwen:qwen3.6-flash` | `error-recovery` | Docker GitLab via MCP | 5 | 10 | 18 | 18 | 100.0% | 100.0% | 100.0% | 25.0% (1/4) | 100.0% | 100.0% | in 56137 / out 823 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T19:57:05Z |
| `anthropic:claude-haiku-4-5-20251001` | `schema-enterprise` | Docker GitLab via MCP | 1 | 1 | 1 | 1 | 100.0% | 100.0% | 100.0% | - | 100.0% | 100.0% | in 3134 / out 110 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T19:57:05Z |
| `google:gemini-3.1-flash-lite-preview` | `schema-enterprise` | Docker GitLab via MCP | 1 | 1 | 1 | 1 | 100.0% | 100.0% | 100.0% | - | 100.0% | 100.0% | in 2441 / out 76 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T19:57:05Z |
| `openai:gpt-5.4-nano` | `schema-enterprise` | Docker GitLab via MCP | 1 | 1 | 1 | 1 | 100.0% | 100.0% | 100.0% | - | 100.0% | 100.0% | in 2359 / out 54 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T19:57:05Z |
| `qwen:qwen3.6-flash` | `schema-enterprise` | Docker GitLab via MCP | 1 | 1 | 1 | 1 | 100.0% | 100.0% | 100.0% | - | 100.0% | 100.0% | in 2832 / out 92 | 1efd9f82eb59 / docs/eval-doc-refresh-2026-05-13 / 2026-05-13T19:57:05Z |
| **Aggregate** | **all selected** | - | **564** | **1088** | **1356** | **1356** | **100.0%** | **99.6%** | **99.6%** | **18.4% (7/38)** | **100.0%** | **99.8%** | - | - |

Published with `cmd/eval_mcp_surfaces --publish-docs` from reviewed Markdown reports. Raw traces and JSON artifacts are not included here.
<!-- END MODEL EVAL DYNAMIC3 RESULTS -->
