# MCP Output Quality Evaluation Prompts

> **Purpose**: 10 evaluation Q&A pairs that test the output quality of `gitlab-mcp-server` MCP tools.
> Each pair verifies that tool responses are well-structured, actionable, and contain the
> expected structural patterns (markdown headers, next-steps, web URLs, formatted dates).
>
> **GitLab instance**: `https://gitlab.example.com` (self-signed TLS, skip verification)
> **Test project**: `my-org/tools/gitlab-mcp-server` (id: 1835)

---

## How to Use

1. Configure the MCP server pointing to `https://gitlab.example.com`
2. Send each prompt to the LLM as-is
3. Verify the **Expected Quality Indicators** are present in the tool response
4. Document findings in the **Result** column

### Legend

| Symbol | Meaning |
|--------|---------|
| ✅ | Response meets all quality indicators |
| ⚠️ | Response is functional but missing some indicators |
| ❌ | Response fails quality checks |

---

## OQ-001: Markdown header in project detail

| Field | Value |
|-------|-------|
| **Prompt** | "Get details for project 1835 in GitLab." |
| **Expected Tool** | `gitlab_project` → action `get` |
| **Quality Indicators** | Response contains `##` or `#` markdown header; project name appears in header; web URL is present as a clickable link |
| **Verification** | Check that raw output has `##` prefix and contains `https://` URL |
| **Result** | |

## OQ-002: Next-steps hints in issue response

| Field | Value |
|-------|-------|
| **Prompt** | "List the open issues in project 1835." |
| **Expected Tool** | `gitlab_issue` → action `list` with state=opened |
| **Quality Indicators** | Response ends with a `💡 **Next steps:**` section in Markdown containing 2+ actionable suggestions; JSON `structuredContent` includes a `next_steps` array with the same hints; each suggestion references a valid tool or action |
| **Verification** | Search Markdown response for "Next steps" AND verify JSON `structuredContent` contains `next_steps` array with matching entries |
| **Result** | |

## OQ-003: Formatted dates (no raw ISO timestamps)

| Field | Value |
|-------|-------|
| **Prompt** | "Show me merge request !5 in project 1835." |
| **Expected Tool** | `gitlab_merge_request` → action `get` |
| **Quality Indicators** | Dates appear in human-readable format (e.g., "2024-01-15 10:30") NOT raw ISO 8601 (e.g., "2024-01-15T10:30:00Z"); created_at, updated_at are formatted |
| **Verification** | Verify no `T..Z` timestamp pattern in markdown text |
| **Result** | |

## OQ-004: Structured JSON extractable by LLM

| Field | Value |
|-------|-------|
| **Prompt** | "Get the project ID and default branch of project my-org/tools/gitlab-mcp-server." |
| **Expected Tool** | `gitlab_project` → action `get` |
| **Quality Indicators** | LLM can extract `id` (1835) and `default_branch` from the structured output; response includes both values explicitly; OutputSchema ensures typed extraction |
| **Verification** | Ask LLM follow-up "What is the project ID?" — must answer 1835 without re-querying |
| **Result** | |

## OQ-005: Web URLs for navigable entities

| Field | Value |
|-------|-------|
| **Prompt** | "Show me the latest pipeline for project 1835 on branch main." |
| **Expected Tool** | `gitlab_pipeline` → action `list` with ref=main |
| **Quality Indicators** | Pipeline response includes `web_url` linking to the GitLab pipeline page; URL is clickable in the markdown output; status is shown with emoji indicator |
| **Verification** | Verify response contains `https://gitlab.example.com/.../pipelines/` URL |
| **Result** | |

## OQ-006: Empty list produces helpful message

| Field | Value |
|-------|-------|
| **Prompt** | "List all tags in project 1835 matching pattern 'nonexistent-pattern-xyz'." |
| **Expected Tool** | `gitlab_tag` → action `list` with search filter |
| **Quality Indicators** | Empty result produces a clear "no tags found" message (not an error); response includes suggestion to try different filters or create a tag; output is not raw empty JSON |
| **Verification** | Verify response is informative, not just `[]` or blank |
| **Result** | |

## OQ-007: Multi-step chain preserves quality

| Field | Value |
|-------|-------|
| **Prompt** | "Find the latest release in project 1835 and show me what commits are in its tag." |
| **Expected Tool** | `gitlab_release` → `list` (get latest), then `gitlab_commit` → `list` with ref=tag |
| **Quality Indicators** | Both responses maintain quality: headers, formatted dates, web URLs; LLM correctly chains the tag name from release to commit list; final answer references both release and commits |
| **Verification** | Verify both tool responses have `##` headers and no raw timestamps |
| **Result** | |

## OQ-008: Error response is actionable

| Field | Value |
|-------|-------|
| **Prompt** | "Get issue #999999 from project 1835." |
| **Expected Tool** | `gitlab_issue` → action `get` with issue_iid=999999 |
| **Quality Indicators** | Error response is semantic (not raw HTTP 404); error message suggests corrective action (e.g., "verify the issue IID exists"); `IsError` flag is set; LLM can self-correct |
| **Verification** | Verify error includes domain context and suggested fix |
| **Result** | |

## OQ-009: Content annotations indicate audience

| Field | Value |
|-------|-------|
| **Prompt** | "List the branches in project 1835." |
| **Expected Tool** | `gitlab_branch` → action `list` |
| **Quality Indicators** | Response TextContent includes `annotations.audience` metadata (`["assistant"]` for list results); priority value is 0.4 for lists, 0.6 for details, 0.8 for mutations; annotations prevent raw Markdown display in JSON-only clients |
| **Verification** | Inspect raw MCP response for `annotations` field on TextContent; verify `audience` is `["assistant"]` and `priority` matches the operation type |
| **Result** | |

## OQ-010: Pagination metadata in list response

| Field | Value |
|-------|-------|
| **Prompt** | "List the first 2 issues in project 1835." |
| **Expected Tool** | `gitlab_issue` → action `list` with per_page=2 |
| **Quality Indicators** | Response includes "Page X of Y" text; `has_more` field indicates additional pages; `total_count` or `total_pages` provides context; next-steps mention pagination |
| **Verification** | Verify pagination section appears in markdown and structured output |
| **Result** | |
