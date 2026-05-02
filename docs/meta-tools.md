# Meta-Tools Reference

Meta-tools group related GitLab operations under a single MCP tool with an `action` parameter. Instead of 1006 individual tools, **33 base meta-tools** (48 with the Enterprise/Premium catalog) provide the same functionality while reducing token overhead for LLMs.

> **Diátaxis type**: Reference
> **Audience**: 👤🔧 All users
> **Prerequisites**: Understanding of MCP protocol and tool concepts

In meta-tool mode (`META_TOOLS=true`, default), the server registers **33 base tools**: 21 inline + 3 always-registered + 2 delegated + 1 sampling + 2 standalone + 4 interactive elicitation. The Enterprise/Premium catalog registers 15 additional enterprise inline meta-tools for a total of **48 tools**. Stdio mode enables that catalog with `GITLAB_ENTERPRISE=true`; HTTP mode can force it with `--enterprise`, and otherwise auto-detects CE/EE per token+URL pool entry when GitLab reports edition.

> **See also**: [Tools Reference](tools/README.md) | [ADR-0005](adr/adr-0005-meta-tool-consolidation.md)
> 📖 **User documentation**: See the [Meta-tools](https://jmrplens.github.io/gitlab-mcp-server/tools/meta-tools/) on the documentation site for a user-friendly version.

## How Meta-Tools Work

Each meta-tool accepts a common input format:

```json
{
  "action": "list",
  "params": {
    "project_id": "42",
    "owned": true
  }
}
```

The dispatcher routes the request to the underlying handler based on the `action` value. The `params` object contains the same parameters as the equivalent individual tool.

## Configuration

Meta-tools are **enabled by default**. To switch to individual tools:

```env
META_TOOLS=false
```

| Mode                       | Tool Count | Best For                                                         |
| -------------------------- | ---------- | ---------------------------------------------------------------- |
| Meta-tools (`true`)        | 33 base / 48 enterprise | LLMs with limited tool context windows                           |
| Individual tools (`false`) | 1006       | Clients that benefit from granular tool discovery                |

---

## Meta-Tool Inventory

### Core Inline Meta-Tools (17)

| # | Tool Name               | Actions | Domain                                    |
|---|-------------------------|---------|-------------------------------------------|
| 1 | `gitlab_project`        | ~92     | Projects, uploads, hooks, badges, boards, import/export, statistics, pages |
| 2 | `gitlab_branch`         | 11      | Branches, protected branches, branch rules |
| 3 | `gitlab_tag`            | 9       | Tags, protected tags                      |
| 4 | `gitlab_release`        | 11      | Releases, release links                   |
| 5 | `gitlab_merge_request`  | ~46     | MR CRUD, approvals, context-commits, MR emoji, MR resource events |
| 6 | `gitlab_mr_review`      | ~22     | MR notes, discussions, drafts, changes    |
| 7 | `gitlab_repository`     | ~40     | Repository tree/compare, commit discussions, files, submodules, markdown |
| 8 | `gitlab_group`          | ~64     | Groups, members, labels, milestones, boards, uploads, import/export, epic discussions |
| 9 | `gitlab_issue`          | ~55     | Issues, notes, discussions, links, statistics, issue emoji, issue resource events |
| 10 | `gitlab_pipeline`      | ~34     | Pipelines, pipeline triggers, pipeline schedules, wait |
| 11 | `gitlab_job`           | ~25     | Jobs, job token scope, wait               |
| 12 | `gitlab_user`          | ~29     | Users, events, notifications, keys, namespaces, avatar |
| 13 | `gitlab_wiki`          | 6       | Project/group wikis                       |
| 14 | `gitlab_environment`   | ~23     | Environments, protected envs, freeze periods, deployments |
| 15 | `gitlab_ci_variable`   | ~15     | CI/CD variables (project, group, instance) |
| 16 | `gitlab_template`      | 12      | CI/CD, Dockerfile, gitignore templates    |
| 17 | `gitlab_admin`         | ~82     | Server settings, broadcast messages, features, license, system hooks, error tracking, alert management, secure files, terraform states, cluster agents, dependency proxy, import service |

### Consolidated Inline Meta-Tools (4)

| # | Tool Name               | Actions | Sources                                   |
|---|-------------------------|---------|-------------------------------------------|
| 18 | `gitlab_access`        | ~48     | Access tokens, deploy tokens, deploy keys, access requests, invites |
| 19 | `gitlab_package`       | ~20     | Packages, container registry              |
| 20 | `gitlab_snippet`       | ~30     | Snippets, snippet discussions, snippet emoji |
| 21 | `gitlab_feature_flags` | ~10     | Feature flags, feature flag user lists    |

### Always-Registered Meta-Tools (3)

| # | Tool Name               | Actions | Source                                    |
|---|-------------------------|---------|-------------------------------------------|
| 22 | `gitlab_model_registry` | 1      | ML model registry package download        |
| 23 | `gitlab_ci_catalog`    | 2       | CI/CD Catalog resource discovery (GraphQL) |
| 24 | `gitlab_custom_emoji`  | 3       | Group-level custom emoji management (GraphQL) |

### Delegated Meta-Tools (2)

| # | Tool Name               | Actions | Source                                    |
|---|-------------------------|---------|-------------------------------------------|
| 25 | `gitlab_search`        | 10      | Global, project, group search             |
| 26 | `gitlab_runner`        | 34      | Runners, runner management, runner controllers |

### Sampling Tools (1)

| # | Tool Name               | Actions | Source                                    |
|---|-------------------------|---------|-------------------------------------------|
| 27 | `gitlab_analyze`       | 11      | LLM-powered analysis via MCP sampling (MR changes, issues, pipelines, security, deployments, CI config, milestones, release notes, technical debt) |

### Interactive Elicitation Tools (4)

| # | Tool Name               | Actions | Source                                    |
|---|-------------------------|---------|-------------------------------------------|
| 28 | `gitlab_interactive_issue_create` | 1 | Guided issue creation via MCP elicitation |
| 29 | `gitlab_interactive_mr_create` | 1 | Guided merge request creation via MCP elicitation |
| 30 | `gitlab_interactive_project_create` | 1 | Guided project creation via MCP elicitation |
| 31 | `gitlab_interactive_release_create` | 1 | Guided release creation via MCP elicitation |

### Standalone Tools (2)

| # | Tool Name               | Actions | Source                                    |
|---|-------------------------|---------|-------------------------------------------|
| 32 | `gitlab_discover_project` | 1 | Git remote URL to GitLab project resolution |
| 33 | `gitlab_server` | 4-6 | Server diagnostics, schema discovery, and optional self-update |

---

## Architecture

### Consolidation History

The meta-tool architecture evolved through ADR-0005:

- **v1.0**: 70 meta-tools (19 inline + 51 standalone sub-package registrations)
- **v2.0**: 36 meta-tools (19 core inline + 4 consolidated inline + 2 delegated + 11 sampling)
- **v2.1**: 40 meta-tools (+3 runner-controller delegated + 1 standalone project-discovery)
- **v3.0**: 60 meta-tools (43 domain inline + 1 search + 1 runner + 3 runner-controller + 11 sampling + 1 standalone)
- **v4.0**: 40 base / 59 enterprise (23 inline + 5 delegated + 11 sampling + 1 standalone + 19 enterprise inline); 6 former standalone meta-tools consolidated into existing meta-tools as enterprise-only routes
- **v5.0**: 42 base / 57 enterprise (23 inline + 4 always-registered + 3 delegated + 11 sampling + 1 standalone + 15 enterprise inline); 3 runner controller delegated meta-tools consolidated into 1; 4 free-tier always-registered meta-tools added (model registry, CI catalog, branch rules, custom emoji); enterprise count reduced from 19 to 15
- **v6.0**: 32 base / 47 enterprise (23 inline + 4 always-registered + 3 delegated + 1 sampling + 1 standalone + 15 enterprise inline); 11 individual sampling tools consolidated into 1 `gitlab_analyze` meta-tool with 11 actions
- **v7.1**: 32 base / 47 enterprise (21 inline + 3 always-registered + 2 delegated + 1 sampling + 1 standalone + 4 interactive elicitation + 15 enterprise inline); 4 `gitlab_interactive_*` elicitation tools exposed in meta-tools mode
- **v7.2**: 33 base / 48 enterprise (adds `gitlab_server` to production-aligned meta-tool counts); `gitlab_server` now exposes `schema_index` and `schema_get` for model-controlled action schema discovery
- **v7.0**: 28 base / 43 enterprise (21 inline + 3 always-registered + 2 delegated + 1 sampling + 1 standalone + 15 enterprise inline); 4 child meta-tools absorbed into parents: `gitlab_branch_rule` → `gitlab_branch`, `gitlab_deployment` → `gitlab_environment`, `gitlab_pipeline_schedule` → `gitlab_pipeline`, `gitlab_runner_controller` → `gitlab_runner`

The base mode provides a ~53% reduction from v3.0, with enterprise features gated behind the Enterprise/Premium catalog.

- Token usage in `tools/list` MCP responses
- LLM selection confusion when choosing among similar tools
- Client rendering overhead for tool palettes

### Implementation Pattern

All meta-tools use the shared infrastructure in `internal/toolutil/metatool.go`:

- `ActionRoute` — pairs a handler with metadata-driven classification. Typed routes carry both `InputSchema` and `OutputSchema` so each action can expose exact params and result contracts
- `ActionMap` — `map[string]ActionRoute` mapping action names to route definitions
- `Route(fn)` / `DestructiveRoute(fn)` — legacy constructors for already-adapted handlers
- `DeriveAnnotations(routes)` — auto-derives tool-level annotations from route metadata: if any route is destructive → `MetaAnnotations`, otherwise → `NonDestructiveMetaAnnotations`
- `MakeMetaHandler()` — creates action-dispatch handlers from route maps; successful results automatically enrich `structuredContent` with `next_steps` hints extracted from Markdown, while `isError` results omit structured content
- `MetaToolInput` — common input struct with `action` and `params` fields
- `MetaAnnotations` — shared annotations (destructiveHint: true) for meta-tools with destructive actions
- `ReadOnlyMetaAnnotations` — for meta-tools with only read operations (e.g., `gitlab_template`, `gitlab_search`)
- `NonDestructiveMetaAnnotations` — for meta-tools without destructive actions (e.g., `gitlab_user`)
- `RouteAction()` / `RouteVoidAction()` / `DestructiveAction()` / `DestructiveVoidAction()` — composite wrappers that combine handler adaptation, route classification, and input/output schema capture
- `RouteActionWithRequest()` / `DestructiveActionWithRequest()` / `DestructiveVoidActionWithRequest()` — request-aware variants for handlers that need the incoming MCP request; they preserve the same input/output schema capture and route classification as their non-request counterparts

### How Actions Are Routed

```text
User: gitlab_project { action: "board_create", params: { project_id: "42", name: "Sprint Board" } }
  │
  ├─ MakeMetaHandler looks up "board_create" in ActionMap routes
  │
  ├─ Routes to: ActionRoute{Handler: wrapAction(client, boards.Create), Destructive: false}
  │
  ├─ boards.Create unmarshals params, calls GitLab API
  │
  ├─ Result formatted via markdownForResult type-switch
  │
  └─ enrichWithHints extracts next_steps from Markdown into structuredContent JSON
```

### Response Enrichment

Successful meta-tool responses include a `next_steps` array in the JSON `structuredContent`. This is critical for IDEs like VS Code that only read JSON:

```json
{
  "branches": [...],
  "pagination": { "page": 1, "total_pages": 2, "has_more": true },
  "next_steps": [
    "When presenting these results, always include the clickable [text](url) links",
    "Get details of a specific branch",
    "Create a new branch from any ref"
  ]
}
```

The enrichment is automatic — `MakeMetaHandler` calls `enrichWithHints()` which parses the Markdown "💡 Next steps" section and merges the hints into the JSON output. If a route returns `isError: true`, `MakeMetaHandler` returns the actionable Markdown error without `structuredContent`, matching the MCP rule that successful structured results must conform to the declared `OutputSchema`.

See [Output Format](output-format.md) for the complete response format specification.

---

## Usage Examples

### List projects

```json
{
  "tool": "gitlab_project",
  "arguments": {
    "action": "list",
    "params": { "owned": true, "per_page": 10 }
  }
}
```

### Create an issue

```json
{
  "tool": "gitlab_issue",
  "arguments": {
    "action": "create",
    "params": {
      "project_id": "my-group/my-project",
      "title": "Fix login bug",
      "labels": "bug,critical"
    }
  }
}
```

### Search code

```json
{
  "tool": "gitlab_search",
  "arguments": {
    "action": "code",
    "params": {
      "scope": "blobs",
      "search": "func RegisterTools"
    }
  }
}
```

### Delete a branch (with confirmation)

```json
{
  "tool": "gitlab_branch",
  "arguments": {
    "action": "delete",
    "params": {
      "project_id": "42",
      "branch_name": "feature/old-branch"
    }
  }
}
```

If the MCP client supports elicitation, the server will ask for user confirmation before executing destructive actions. Set `YOLO_MODE=true` or `AUTOPILOT=true` to skip confirmation.

---

## Discovering the params shape

Meta-tools advertise a deliberately compact input schema by default (`META_PARAM_SCHEMA=opaque`): the LLM sees the `action` enum and an opaque `params` object. To discover the exact `params` shape for a chosen action, three mechanisms are available:

1. **Model-controlled schema actions** (recommended for LLMs) — call `gitlab_server` with `schema_index` or `schema_get`:

   ```json
   {
     "action": "schema_index",
     "params": {
       "tool": "gitlab_merge_request"
     }
   }
   ```

   `schema_index` returns the visible meta-tools/actions, schema URIs, action counts, and destructive flags. Omit `tool` to list every visible meta-tool. After choosing a tool/action pair, call `schema_get`:

   ```json
   {
     "action": "schema_get",
     "params": {
       "tool": "gitlab_merge_request",
       "action": "create"
     }
   }
   ```

   The response is the JSON Schema for the chosen action's `params` object only. `schema_index` and `schema_get` expose the same post-filter route set as the schema resources, so excluded or read-only-filtered meta-tools are not discoverable through either path.

2. **MCP Resource** (works in every mode) — read the per-action JSON Schema:

   ```text
   gitlab://schema/meta/{tool}/{action}
   ```

   For example, `gitlab://schema/meta/gitlab_merge_request/create` returns the JSON Schema for the `create` action's `params`. The `gitlab://schema/meta/` index resource enumerates every registered meta-tool and its actions.

   The index resource returns a JSON object with the URI template and the action catalog visible for the current server configuration:

   ```json
   {
     "uri_template": "gitlab://schema/meta/{tool}/{action}",
     "tools": [
       {
         "tool": "gitlab_merge_request",
         "actions": ["create", "get", "list", "merge"]
       }
     ]
   }
   ```

   After choosing a tool/action pair, read the concrete resource for that action. For example:

   ```json
   {
     "method": "resources/read",
     "params": {
       "uri": "gitlab://schema/meta/gitlab_merge_request/create"
     }
   }
   ```

   The response content is the JSON Schema for the `params` object only. The final tool call still uses the common meta-tool envelope:

   ```json
   {
     "action": "create",
     "params": {
       "project_id": "42",
       "source_branch": "feature/docs",
       "target_branch": "main",
       "title": "Update documentation"
     }
   }
   ```

3. **Embed schemas in the tool description** — set `META_PARAM_SCHEMA=full` (or the lighter `compact` mode) at startup. The meta-tool's `inputSchema` then exposes a `oneOf` discriminating on `action`, with the per-action params shape inlined. See [env-reference.md](env-reference.md) for size/cost trade-offs.

The dispatch behaviour is identical across modes — only the schema sent to the LLM changes.
