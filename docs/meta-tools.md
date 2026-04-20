# Meta-Tools Reference

Meta-tools group related GitLab operations under a single MCP tool with an `action` parameter. Instead of 1004 individual tools, **42 domain meta-tools** (57 with `GITLAB_ENTERPRISE=true`) provide the same functionality while reducing token overhead for LLMs.

> **Diátaxis type**: Reference
> **Audience**: 👤🔧 All users
> **Prerequisites**: Understanding of MCP protocol and tool concepts

In meta-tool mode (`META_TOOLS=true`, default), the server registers **42 base tools**: 23 inline + 4 always-registered + 3 delegated + 11 sampling + 1 standalone. With `GITLAB_ENTERPRISE=true`, 15 additional enterprise inline meta-tools are registered for a total of **57 tools**.

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
| Meta-tools (`true`)        | 42 base / 57 enterprise | LLMs with limited tool context windows                           |
| Individual tools (`false`) | 1004       | Clients that benefit from granular tool discovery                |

---

## Meta-Tool Inventory

### Core Inline Meta-Tools (19)

| # | Tool Name               | Actions | Domain                                    |
|---|-------------------------|---------|-------------------------------------------|
| 1 | `gitlab_project`        | ~92     | Projects, uploads, hooks, badges, boards, import/export, statistics, pages |
| 2 | `gitlab_branch`         | 10      | Branches, protected branches              |
| 3 | `gitlab_tag`            | 9       | Tags, protected tags                      |
| 4 | `gitlab_release`        | 11      | Releases, release links                   |
| 5 | `gitlab_merge_request`  | ~46     | MR CRUD, approvals, context-commits, MR emoji, MR resource events |
| 6 | `gitlab_mr_review`      | ~22     | MR notes, discussions, drafts, changes    |
| 7 | `gitlab_repository`     | ~40     | Repository tree/compare, commit discussions, files, submodules, markdown |
| 8 | `gitlab_group`          | ~64     | Groups, members, labels, milestones, boards, uploads, import/export, epic discussions |
| 9 | `gitlab_issue`          | ~55     | Issues, notes, discussions, links, statistics, issue emoji, issue resource events |
| 10 | `gitlab_pipeline`      | ~22     | Pipelines, pipeline triggers, wait        |
| 11 | `gitlab_job`           | ~25     | Jobs, job token scope, wait               |
| 12 | `gitlab_user`          | ~29     | Users, events, notifications, keys, namespaces, avatar |
| 13 | `gitlab_wiki`          | 6       | Project/group wikis                       |
| 14 | `gitlab_environment`   | ~16     | Environments, protected envs, freeze periods |
| 15 | `gitlab_deployment`    | 7       | Deployments                               |
| 16 | `gitlab_pipeline_schedule` | 11  | Pipeline schedules, schedule variables    |
| 17 | `gitlab_ci_variable`   | ~15     | CI/CD variables (project, group, instance) |
| 18 | `gitlab_template`      | 12      | CI/CD, Dockerfile, gitignore templates    |
| 19 | `gitlab_admin`         | ~82     | Server settings, broadcast messages, features, license, system hooks, error tracking, alert management, secure files, terraform states, cluster agents, dependency proxy, import service |

### Consolidated Inline Meta-Tools (4)

| # | Tool Name               | Actions | Sources                                   |
|---|-------------------------|---------|-------------------------------------------|
| 20 | `gitlab_access`        | ~48     | Access tokens, deploy tokens, deploy keys, access requests, invites |
| 21 | `gitlab_package`       | ~20     | Packages, container registry              |
| 22 | `gitlab_snippet`       | ~30     | Snippets, snippet discussions, snippet emoji |
| 23 | `gitlab_feature_flags` | ~10     | Feature flags, feature flag user lists    |

### Always-Registered Meta-Tools (4)

| # | Tool Name               | Actions | Source                                    |
|---|-------------------------|---------|-------------------------------------------|
| 24 | `gitlab_model_registry` | 1      | ML model registry package download        |
| 25 | `gitlab_ci_catalog`    | 2       | CI/CD Catalog resource discovery (GraphQL) |
| 26 | `gitlab_branch_rule`   | 1       | Branch rules aggregated view (GraphQL)    |
| 27 | `gitlab_custom_emoji`  | 3       | Group-level custom emoji management (GraphQL) |

### Delegated Meta-Tools (3)

| # | Tool Name               | Actions | Source                                    |
|---|-------------------------|---------|-------------------------------------------|
| 28 | `gitlab_search`        | 10      | Global, project, group search             |
| 29 | `gitlab_runner`        | 19      | Runners, runner management                |
| 30 | `gitlab_runner_controller` | 15  | Runner controller CRUD, tokens, scopes   |

### Sampling Tools (11)

| # | Tool Name               | Actions | Source                                    |
|---|-------------------------|---------|-------------------------------------------|
| 31 | `gitlab_summarize_issue` | 1     | LLM-powered issue summarization (sampling) |
| 32 | `gitlab_analyze_mr_changes` | 1  | LLM-powered MR analysis (sampling)        |
| 33 | `gitlab_generate_release_notes` | 1 | LLM-powered release notes generation (sampling) |
| 34 | `gitlab_analyze_pipeline_failure` | 1 | LLM-powered pipeline failure analysis (sampling) |
| 35 | `gitlab_summarize_mr_review` | 1 | LLM-powered MR review summarization (sampling) |
| 36 | `gitlab_generate_milestone_report` | 1 | LLM-powered milestone report generation (sampling) |
| 37 | `gitlab_analyze_ci_configuration` | 1 | LLM-powered CI configuration analysis (sampling) |
| 38 | `gitlab_analyze_issue_scope` | 1 | LLM-powered issue scope analysis (sampling) |
| 39 | `gitlab_review_mr_security`  | 1 | LLM-powered MR security review (sampling) |
| 40 | `gitlab_find_technical_debt` | 1 | LLM-powered technical debt detection (sampling) |
| 41 | `gitlab_analyze_deployment_history` | 1 | LLM-powered deployment history analysis (sampling) |

### Standalone Tools (1)

| # | Tool Name               | Actions | Source                                    |
|---|-------------------------|---------|-------------------------------------------|
| 42 | `gitlab_resolve_project_from_remote` | 1 | Git remote URL to GitLab project resolution |

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

The base mode provides a 45% reduction from v3.0, with enterprise features gated behind `GITLAB_ENTERPRISE=true`.

- Token usage in `tools/list` MCP responses
- LLM selection confusion when choosing among similar tools
- Client rendering overhead for tool palettes

### Implementation Pattern

All meta-tools use the shared infrastructure in `internal/toolutil/metatool.go`:

- `MakeMetaHandler()` — creates action-dispatch handlers from route maps; automatically enriches `structuredContent` with `next_steps` hints extracted from Markdown
- `MetaToolInput` — common input struct with `action` and `params` fields
- `MetaAnnotations` — shared annotations (destructiveHint: true) for meta-tools with delete actions
- `ReadOnlyMetaAnnotations` — for meta-tools with only read operations (e.g., `gitlab_template`, `gitlab_search`)
- `NonDestructiveMetaAnnotations` — for meta-tools without delete actions (e.g., `gitlab_user`)
- `wrapAction()` / `wrapVoidAction()` — adapters for sub-package handler signatures

### How Actions Are Routed

```text
User: gitlab_project { action: "board_create", params: { project_id: "42", name: "Sprint Board" } }
  │
  ├─ MakeMetaHandler looks up "board_create" in routes map
  │
  ├─ Routes to: wrapAction(client, boards.Create)
  │
  ├─ boards.Create unmarshals params, calls GitLab API
  │
  ├─ Result formatted via markdownForResult type-switch
  │
  └─ enrichWithHints extracts next_steps from Markdown into structuredContent JSON
```

### Response Enrichment

Meta-tool responses include a `next_steps` array in the JSON `structuredContent`. This is critical for IDEs like VS Code that only read JSON:

```json
{
  "branches": [...],
  "pagination": { "page": 1, "total_pages": 2 },
  "next_steps": [
    "When presenting these results, always include the clickable [text](url) links",
    "Get details of a specific branch",
    "Create a new branch from any ref"
  ]
}
```

The enrichment is automatic — `MakeMetaHandler` calls `enrichWithHints()` which parses the Markdown "💡 Next steps" section and merges the hints into the JSON output. Individual (non-meta) tools do not include `structuredContent` by design.

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
