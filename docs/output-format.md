# Output Format

How gitlab-mcp-server formats tool responses for both human and machine consumption.

> **Diátaxis type**: Explanation
> **Audience**: 👤 End users, AI assistant users

---

## Overview

Successful tool responses contain **two representations** of the same data:

1. **Markdown content** — human-readable text with tables, clickable links, and next-step hints. Targeted at the LLM (`audience: assistant`) so it can reason over the data and present it to you.
2. **Structured JSON** (`structuredContent`) — machine-readable data for programmatic clients. IDEs like VS Code read this to extract fields, and it also includes a `next_steps` array with actionable hints.

```text
┌──────────────────── Tool Response ────────────────────┐
│                                                        │
│  Content (Markdown)          structuredContent (JSON)   │
│  ┌──────────────────┐       ┌──────────────────────┐   │
│  │ ## Branches (5)   │       │ {                    │   │
│  │                   │       │   "branches": [...], │   │
│  │ | Branch | ...    │       │   "pagination": {},  │   │
│  │ | [main](url)     │       │   "next_steps": [    │   │
│  │                   │       │     "Get details...",│   │
│  │ 💡 Next steps:    │       │     "Create branch"  │   │
│  │ - Get details...  │       │   ]                  │   │
│  │ - Create branch   │       │ }                    │   │
│  └──────────────────┘       └──────────────────────┘   │
│                                                        │
│  audience: ["assistant"]     Read by IDEs like VS Code  │
│  Read by the LLM             and JetBrains              │
└────────────────────────────────────────────────────────┘
```

## What You See as a User

When you ask your AI assistant a question like *"Show me the open merge requests"*, the response typically includes:

### Clickable Links

List results include clickable links that open directly in GitLab:

```markdown
| MR | Title | Author | Status |
|----|-------|--------|--------|
| [!243](https://gitlab.example.com/project/-/merge_requests/243) | Fix login | alice | open |
| [!241](https://gitlab.example.com/project/-/merge_requests/241) | Add tests | bob | open |
```

Click on `!243` to open the merge request in your browser. This works for merge requests, issues, pipelines, projects, branches, commits, releases, todos, milestones, and members — **14 domains** with clickable links.

### Next Steps

After each response, you will see suggested next actions:

```text
💡 Next steps:
- Get details of a specific MR by its number
- Create a new merge request
- Approve or merge an open MR
```

These hints are available in both the Markdown and JSON output, so your IDE can display them regardless of which format it reads. Tool execution errors use `isError: true` and may omit `structuredContent` so clients do not confuse an error payload with a successful typed result.

### Formatted Data

- **Dates** appear in readable format (`2025-01-15 10:30`) instead of raw ISO timestamps
- **Status** values use emoji indicators (✅ success, ❌ failed, ⏳ running)
- **Pagination** shows "Page 1 of 3 (20 per page)" with hints to request more

## How Clients Consume Responses

Different MCP clients read different parts of the response:

| Client | Reads Markdown `content` | Reads `structuredContent` JSON | How hints arrive |
|--------|--------------------------|-------------------------------|-----------------|
| **VS Code / Copilot** | ❌ Ignores | ✅ Primary | `next_steps` array in JSON |
| **Cursor** | ✅ Primary | ✅ Also available | Both `💡 Next steps` in Markdown and `next_steps` in JSON |
| **Claude Desktop** | ✅ Primary | ✅ Also available | Both formats |
| **CLI tools** | ✅ Primary | ❌ Often ignored | `💡 Next steps` in Markdown |
| **Custom HTTP clients** | Depends | Depends | Both available in JSON-RPC response |

The server ensures hints appear in **both** formats so no client misses them.

## Content Annotations

Every Markdown response includes [MCP annotations](https://modelcontextprotocol.io/specification/2025-11-25/server/utilities/annotations) that tell the client who the content is for and how important it is:

| Annotation | Audience | Priority | Used For |
|-----------|----------|----------|----------|
| `ContentList` | `assistant` | 0.4 | List and search results |
| `ContentDetail` | `assistant` | 0.6 | Single-entity details (get, show) |
| `ContentMutate` | `assistant` | 0.8 | Create, update, delete confirmations |
| `ContentAssistant` | `assistant` | 0.7 | General assistant-targeted content |
| `ContentUser` | `user` | 0.8 | Content for direct user display |
| `ContentBoth` | `user`, `assistant` | 0.5 | Content for both audiences |

### What Does `audience: ["assistant"]` Mean?

When content is marked `audience: ["assistant"]`, it tells the MCP client: *"This content is for the AI to reason over, not for direct display to the user."* This prevents the raw Markdown from being shown alongside the formatted JSON in clients like VS Code that render both. The LLM still sees and uses the Markdown — it just is not duplicated in the UI.

### What Is Priority?

The `priority` value (0.0 to 1.0) hints to the client how important the content is relative to other content in the same response. Higher priority content should be processed first. For example, a mutation result (0.8) is more immediately relevant than a list result (0.4).

## Tool Annotations

Separate from content annotations, every **tool** has behavioral annotations that describe what it does:

| Annotation | Type | Meaning |
|-----------|------|---------|
| `readOnlyHint` | `bool` | The tool only reads data, never modifies anything |
| `destructiveHint` | `*bool` | The tool may perform irreversible operations (delete, drop) |
| `idempotentHint` | `bool` | Calling the tool multiple times with the same input produces the same result |
| `openWorldHint` | `*bool` | The tool interacts with external systems (GitLab API) |

These annotations help your AI assistant and IDE make safety decisions:

- Tools with `destructiveHint: true` may trigger confirmation prompts
- Tools with `readOnlyHint: true` can be called freely without risk
- Tools with `idempotentHint: true` are safe to retry on failure

## Response Format Examples

### List Response

When you ask *"Show me the branches in my project"*:

**Markdown content** (what the LLM sees):

```markdown
## Branches (5)

| Branch | Protected | Default | Merged | Web URL |
|--------|-----------|---------|--------|---------|
| [main](https://gitlab.example.com/.../main) | ✅ | ✅ | — | [↗](url) |
| [develop](https://gitlab.example.com/.../develop) | ✅ | — | — | [↗](url) |
| feature/login | — | — | — | [↗](url) |

Page 1 of 1 (20 per page) · 5 items

---
💡 **Next steps:**
- When presenting these results, always include the clickable [text](url) links
- Get details of a specific branch
- Create a new branch from any ref
```

**Structured JSON** (what VS Code reads):

```json
{
  "branches": [
    { "name": "main", "protected": true, "default": true, "web_url": "https://..." },
    { "name": "develop", "protected": true, "default": false, "web_url": "https://..." }
  ],
  "pagination": { "page": 1, "per_page": 20, "total_items": 5, "total_pages": 1, "has_more": false },
  "next_steps": [
    "When presenting these results, always include the clickable [text](url) links",
    "Get details of a specific branch",
    "Create a new branch from any ref"
  ]
}
```

### Detail Response

When you ask *"Show me merge request !243"*:

**Markdown content**:

```markdown
## Merge Request !243: Fix Login Bug

| Field | Value |
|-------|-------|
| Status | open |
| Author | [alice](https://gitlab.example.com/alice) |
| Created | 2025-03-15 10:30 |
| Updated | 2025-03-20 14:15 |
| Source | feature/fix-login → main |
| Web URL | [!243](https://gitlab.example.com/project/-/merge_requests/243) |

---
💡 **Next steps:**
- View the changes (diff) for this MR
- List discussions and review comments
- Approve or merge this MR
```

### Mutation Response

When you ask *"Create a new issue titled 'Fix the login page'"*:

```markdown
## Issue Created: #42 — Fix the login page

| Field | Value |
|-------|-------|
| IID | [#42](https://gitlab.example.com/project/-/issues/42) |
| State | opened |
| Author | you |
| Created | 2025-03-21 09:00 |

---
💡 **Next steps:**
- Add labels or assignees to this issue
- Create a merge request linked to this issue
- Add a comment with more details
```

### Not Found Response

When a "get" operation targets a resource that does not exist the server returns an informational result instead of an opaque error:

```text
## ❓ Branch Not Found

The branch **"nonexistent" in project 42** does not exist or is not accessible with your current permissions.

💡 **Next steps:**
- Use gitlab_branch_list with project_id to list available branches
- Verify the branch name is spelled correctly (case-sensitive)
```

Not-found responses have `IsError: true` but include actionable hints so the AI assistant can self-correct or suggest alternatives. This pattern covers 27 "get" handlers across 21 domains.

## Embedded Resources

Selected `gitlab_*_get` tools attach an additional content block of type `resource` (`mcp.EmbeddedResource`) carrying the canonical MCP resource URI for the entity returned. This lets clients that only render `Content` blocks (and ignore `StructuredContent`) still surface a stable, dereferenceable identifier the user or LLM can pass to `resources/read`, follow-up tool calls, or UI deep-links.

Currently embedded by 22 `gitlab_*_get` handlers:

| Tool                          | Canonical URI                                            |
| ----------------------------- | -------------------------------------------------------- |
| `gitlab_board_get`            | `gitlab://project/{project_id}/board/{board_id}`         |
| `gitlab_branch_get`           | `gitlab://project/{project_id}/branch/{branch_name}`     |
| `gitlab_commit_get`           | `gitlab://project/{project_id}/commit/{sha}`             |
| `gitlab_deploy_key_get`       | `gitlab://project/{project_id}/deploy_key/{key_id}`      |
| `gitlab_deployment_get`       | `gitlab://project/{project_id}/deployment/{deployment_id}` |
| `gitlab_environment_get`      | `gitlab://project/{project_id}/environment/{environment_id}` |
| `gitlab_feature_flag_get`     | `gitlab://project/{project_id}/feature_flag/{name}`      |
| `gitlab_group_get`            | `gitlab://group/{group_id}`                              |
| `gitlab_group_label_get`      | `gitlab://group/{group_id}/label/{label_id}`             |
| `gitlab_group_milestone_get`  | `gitlab://group/{group_id}/milestone/{milestone_iid}`    |
| `gitlab_issue_get`            | `gitlab://project/{project_id}/issue/{issue_iid}`        |
| `gitlab_job_get`              | `gitlab://project/{project_id}/job/{job_id}`             |
| `gitlab_label_get`            | `gitlab://project/{project_id}/label/{label_id}`         |
| `gitlab_milestone_get`        | `gitlab://project/{project_id}/milestone/{milestone_iid}`|
| `gitlab_mr_get`               | `gitlab://project/{project_id}/mr/{merge_request_iid}`              |
| `gitlab_pipeline_get`         | `gitlab://project/{project_id}/pipeline/{pipeline_id}`   |
| `gitlab_project_get`          | `gitlab://project/{project_id}`                          |
| `gitlab_project_snippet_get`  | `gitlab://project/{project_id}/snippet/{snippet_id}`     |
| `gitlab_release_get`          | `gitlab://project/{project_id}/release/{tag_name}`       |
| `gitlab_snippet_get`          | `gitlab://snippet/{snippet_id}`                          |
| `gitlab_tag_get`              | `gitlab://project/{project_id}/tag/{tag_name}`           |
| `gitlab_wiki_get`             | `gitlab://project/{project_id}/wiki/{slug}`              |

The embedded resource carries `MIMEType: "application/json"` and a `Text` payload equal to the JSON-marshaled output struct — duplicating `StructuredContent` so simpler clients lose nothing. Not-found responses do **not** embed (the entity does not exist).

This behaviour is enabled by default and can be disabled globally with `EMBEDDED_RESOURCES=false` (env var) or `--embedded-resources=false` (HTTP-mode flag) as a kill-switch for clients that don't tolerate duplicate content blocks.

## Per-Route OutputSchema (Meta-Tools)

Meta-tools declare a single tool-level `OutputSchema` (the envelope with `next_steps` and `pagination` fields). In addition, each action route can carry its own output schema describing the exact shape returned by that specific action.

Per-route schemas are populated automatically when using typed route constructors (`RouteAction[T,R]`, `DestructiveAction[T,R]`, `RouteActionWithRequest[T,R]`, `DestructiveActionWithRequest[T,R]`). Void actions and plain `Route()` calls do not have per-route schemas.

These schemas are:

- **Exposed in `llms-full.txt`** under "Action Output Schemas" for each meta-tool, using collapsible `<details>` blocks per action
- **Audited by `cmd/audit_output`** which reports routes with missing schemas (category `route-output-schema`)
- **Accessible programmatically** via `toolutil.MetaRoutes()` which returns all registered route maps
- **Cached** by `reflect.Type` to avoid redundant schema generation

This enables LLMs to predict the exact response shape of each meta-tool action without trial-and-error.

## See Also

- [Architecture — Response Format](architecture.md#design-patterns) — implementation patterns for dual-output
- [Meta-Tools](meta-tools.md) — how domain meta-tools use this format
- [Tools Reference](tools/README.md) — per-domain tool documentation
- [Troubleshooting](troubleshooting.md) — common output format issues
- [MCP Specification — Annotations](https://modelcontextprotocol.io/specification/2025-11-25/server/utilities/annotations)
