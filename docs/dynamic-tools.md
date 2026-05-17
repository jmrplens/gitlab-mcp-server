# Dynamic Toolset

The dynamic toolset is the low-token operating mode for gitlab-mcp-server. It exposes a tiny public MCP surface and lets the model discover the canonical GitLab action catalog progressively.

> **Diataxis type**: Guide + Reference
> **Audience**: Users, operators, and developers evaluating low-token MCP deployments
> **Prerequisites**: Basic MCP tool-call concepts and a configured GitLab token
> **Status**: Meta-tools remain the default today. `TOOL_SURFACE=dynamic` is the current two-tool low-token candidate for a future default.
> **ADR**: See [ADR-0011: Low-token dynamic toolset mode](adr/adr-0011-low-token-dynamic-toolset.md).

## When To Use It

Use the dynamic toolset when the initial MCP `tools/list` payload is the limiting factor for your AI client. This is common with clients that have small tool-context budgets, strict tool-count limits, or slow tool palette rendering.

| Mode | Visible Tools | Best For |
| --- | ---: | --- |
| Meta-tools, current default | 33 base / 49 self-managed enterprise / 50 GitLab.com Enterprise | Broad compatibility and predictable domain-level action selection |
| Dynamic toolset | 2 | Low-token clients that can find an action with schema, then execute it |
| Dynamic-3 toolset | 3 | Clients or evaluations that need explicit search, describe, then execute separation |
| Individual tools | 863 CE / 1014 self-managed enterprise / 1019 GitLab.com Enterprise | Clients that benefit from one tool per GitLab operation |

Dynamic mode keeps the same underlying GitLab coverage as meta-tools. It changes discovery, not business behavior.

## Public Tools

`TOOL_SURFACE=dynamic` and `TOOL_SURFACE=dynamic-2` expose the current two-tool surface:

| Tool | Purpose |
| --- | --- |
| `gitlab_find_action` | Search the canonical action catalog and return exact input schemas, examples, safety metadata, and output summaries for matching action IDs |
| `gitlab_execute_tool` | Execute one selected action by canonical `domain.action` ID with runtime validation and safety checks |

`TOOL_SURFACE=dynamic-3` explicitly selects the earlier three-tool surface:

| Tool | Purpose |
| --- | --- |
| `gitlab_search_tools` | Search the canonical action catalog using natural language, canonical action IDs, domains, verbs, aliases, and fuzzy matching |
| `gitlab_describe_tools` | Return exact input schemas, examples, safety metadata, and output summaries for selected action IDs |
| `gitlab_execute_tool` | Execute one selected action by canonical `domain.action` ID with runtime validation and safety checks |

## Configuration

### Stdio Mode

```env
GITLAB_TOKEN=glpat-xxxxxxxxxxxxxxxxxxxx
TOOL_SURFACE=dynamic
```

For self-managed GitLab, add:

```env
GITLAB_URL=https://gitlab.example.com
```

`META_TOOLS=dynamic` is also accepted as a legacy/convenience selector, but `TOOL_SURFACE=dynamic` is clearer and overrides `META_TOOLS` when both are set.

### HTTP Mode

```bash
gitlab-mcp-server --http \
  --gitlab-url=https://gitlab.com \
  --tool-surface=dynamic
```

For the smallest overall startup context, pair it with the minimal capability surface:

```bash
gitlab-mcp-server --http \
  --gitlab-url=https://gitlab.com \
  --tool-surface=dynamic \
  --capability-surface=minimal
```

`CAPABILITY_SURFACE=minimal` keeps `gitlab://workspace/roots` and omits optional resources, prompts, workflow guides, and meta-schema resources. Dynamic execution still works because `gitlab_find_action` returns exact action schemas inline. `META_PARAM_SCHEMA` does not affect the visible dynamic tool schemas; leave it at the default `opaque` for dynamic deployments.

## User Workflow

The model should use a two-step workflow:

1. Find candidate actions and exact schemas with `gitlab_find_action`.
2. Execute one validated action with `gitlab_execute_tool`.

```mermaid
sequenceDiagram
    participant User
    participant LLM as AI Client
    participant Find as gitlab_find_action
    participant Execute as gitlab_execute_tool
    participant GitLab

    User->>LLM: Show open MRs authored by me
    LLM->>Find: query="merge request list open authored by me"
    Find-->>LLM: merge_request.list with exact params schema and examples
    LLM->>Execute: action="merge_request.list", params={...}
    Execute->>GitLab: GitLab API request
    GitLab-->>Execute: JSON response
    Execute-->>LLM: Markdown + structured content
    LLM-->>User: Human-readable result
```

The dynamic tools return normal MCP tool results. The examples below show the `structuredContent` payloads, not the full MCP envelope. In the full result, human-readable Markdown appears in `content`, JSON data appears in `structuredContent`, and `isError` is set on the MCP result envelope when the server returns repair guidance instead of a successful action payload. The dynamic layer does not invent a second execution path: `gitlab_execute_tool` dispatches to the same handler, markdown formatter, schema validation, policy checks, and GitLab client used by the corresponding meta-tool action.

## MCP Response Shapes

### `gitlab_find_action`

Find returns a ranked shortlist of catalog actions with exact schemas inline. The Markdown response is a compact table with action IDs, destructive flags, required params, and examples. The `structuredContent` payload uses this shape:

```json
{
  "query": "merge request list open authored by me project",
  "count": 1,
  "results": [
    {
      "id": "merge_request.list",
      "tool": "gitlab_merge_request",
      "domain": "merge_request",
      "action": "list",
      "schema_uri": "gitlab://schema/meta/gitlab_merge_request/list",
      "destructive": false,
      "required_params": ["project_id"],
      "input_schema": {
        "type": "object",
        "required": ["project_id"],
        "properties": {
          "project_id": { "type": "string" },
          "state": { "type": "string" },
          "scope": { "type": "string" }
        }
      },
      "example": {
        "tool": "gitlab_execute_tool",
        "arguments": {
          "action": "merge_request.list",
          "params": { "project_id": "group/project" }
        }
      },
      "score": 275
    }
  ]
}
```

Pass `explain: true` to include deterministic scoring reasons in each result. The default omits explanations to keep responses compact. Enabling `explain` does not alter ranking; it only adds reasoning metadata.

### Dynamic-3: `gitlab_search_tools`

Search returns a ranked shortlist of catalog actions. The Markdown response is a compact table with action IDs, destructive flags, and required params. The `structuredContent` payload uses this shape:

```json
{
  "query": "merge request list open authored by me project",
  "count": 1,
  "results": [
    {
      "id": "merge_request.list",
      "tool": "gitlab_merge_request",
      "domain": "merge_request",
      "action": "list",
      "schema_uri": "gitlab://schema/meta/gitlab_merge_request/list",
      "destructive": false,
      "required_params": ["project_id"],
      "usage": "",
      "related_actions": [],
      "score": 275
    }
  ]
}
```

Pass `explain: true` to include deterministic scoring reasons in each result. The default omits explanations to keep responses compact. Enabling `explain` does not alter ranking; it only adds reasoning metadata. Search may also set `low_confidence: true` when the top result score or margin is weak, and `ambiguous_with` when a query matches a known ambiguous alias.

An empty query returns `isError: true` with example search terms. A query with no matches returns a non-error result with `count: 0` and a small `suggestions` array of nearby tokens and common domains, so the model can broaden the query and try again without receiving a catalog dump.

### Dynamic-3: `gitlab_describe_tools`

Describe hydrates one or more canonical action IDs. It accepts either `action` or `actions`, trims duplicates, resolves unambiguous aliases, and rejects unknown or ambiguous IDs with repair guidance. The `structuredContent` payload includes the exact action-specific input schema and an executable example:

```json
{
  "count": 1,
  "actions": [
    {
      "id": "merge_request.list",
      "tool": "gitlab_merge_request",
      "domain": "merge_request",
      "action": "list",
      "schema_uri": "gitlab://schema/meta/gitlab_merge_request/list",
      "destructive": false,
      "required_params": ["project_id"],
      "usage": "",
      "related_actions": [],
      "input_schema": {
        "type": "object",
        "required": ["project_id"],
        "properties": {
          "project_id": { "type": "string" },
          "state": { "type": "string" },
          "scope": { "type": "string" }
        }
      },
      "example": {
        "tool": "gitlab_execute_tool",
        "arguments": {
          "action": "merge_request.list",
          "params": { "project_id": "group/project" }
        }
      }
    }
  ]
}
```

`usage` distinguishes commonly confused actions. `related_actions` appears only for curated workflows where order matters, such as comparing refs before generating release notes or checking tag/release state before deletion. `output_schema` is included when the underlying action route provides one. The Markdown response includes the core metadata and embeds the compact JSON input schema for clients that display only text content.

### `gitlab_execute_tool`

Execute accepts a canonical `domain.action` ID and a required `params` object. Use `params: {}` for actions with no parameters.

Before dispatching, it resolves unambiguous aliases, normalizes known parameter aliases against the selected schema, and applies a small set of action-scoped compatibility conversions.

It also moves top-level `confirm: true` into action params for destructive calls. Compatibility conversions cover observed low-token model patterns such as issue lifecycle aliases (`issue.close` and `issue.reopen`), pipeline schedule action spellings, project snippet single-file params, deploy-key ID aliases, and feature-flag user-list list filters that should stay project-scoped.

Unknown parameters, including unsupported security-sensitive fields such as `masked` or `protected` on pipeline schedule variables, are rejected before dispatch rather than silently removed.

```json
{
  "action": "merge_request.list",
  "params": {
    "project_id": "my-group/my-project",
    "state": "opened",
    "scope": "created_by_me",
    "per_page": 20
  }
}
```

The response is the existing action response: the same Markdown and structured result returned by the backing meta-tool handler. If the action ID is unknown, execute returns `isError: true` and may suggest nearby canonical IDs. If params fail validation, the backing handler returns the same repairable validation error it would return in meta-tool mode.

## Example Calls

### Find

```json
{
  "tool": "gitlab_find_action",
  "arguments": {
    "query": "merge request list open authored by me project",
    "limit": 5
  }
}
```

A result contains canonical action IDs such as `merge_request.list`, backing meta-tool names, domains, action names, schema URIs, destructive metadata, required params, usage hints, scores, exact input schemas, and executable examples.

### Execute

```json
{
  "tool": "gitlab_execute_tool",
  "arguments": {
    "action": "merge_request.list",
    "params": {
      "project_id": "my-group/my-project",
      "state": "opened",
      "scope": "created_by_me",
      "per_page": 20
    }
  }
}
```

The action ID is canonical. Aliases can help discovery, but execution should use the action ID returned by find, search, or describe.

## Repair Loop

Dynamic mode is designed for repairable failures. Models should treat `isError: true` as feedback, not as a dead end:

| Failure | Server response | Model recovery |
| --- | --- | --- |
| Missing find query | Error result with example query terms | Retry `gitlab_find_action` with domain, resource, verb, and filters |
| Unknown action ID | Error result, often with `Did you mean ...?` canonical IDs | Find the suggested canonical action ID |
| Ambiguous alias | Error result listing the valid canonical targets | Pick one listed `domain.action` ID and find or describe it |
| Invalid params | Backing handler validation error | Call `gitlab_find_action` and rebuild `params` from `input_schema` |
| Destructive action without confirmation | Error result explaining that `confirm=true` is required | Ask the user for explicit approval, then retry with top-level `confirm: true` only if approved |

## Destructive Actions

Dynamic mode reuses the same destructive-action protection as meta-tools. Destructive actions require explicit confirmation unless safe mode or read-only mode blocks them earlier.

```json
{
  "tool": "gitlab_execute_tool",
  "arguments": {
    "action": "project.delete",
    "params": {
      "project_id": "my-group/my-project"
    }
  }
}
```

Without confirmation, destructive execution returns `isError: true` with guidance instead of performing the operation. To execute intentionally:

```json
{
  "tool": "gitlab_execute_tool",
  "arguments": {
    "action": "project.delete",
    "confirm": true,
    "params": {
      "project_id": "my-group/my-project"
    }
  }
}
```

For safer deployments:

- Set `GITLAB_READ_ONLY=true` to remove mutating actions at startup.
- Set `GITLAB_SAFE_MODE=true` to return previews for mutating actions instead of executing them.
- Keep `YOLO_MODE=false` and `AUTOPILOT=false` unless the deployment is fully trusted.

## Architecture

Dynamic mode is a progressive-disclosure layer over the canonical action catalog. It does not duplicate GitLab handlers.

```mermaid
flowchart TD
  subgraph Startup
    A[Build canonical action catalog]
    B[Apply policy filters]
    C[Add standalone dynamic actions]
    D[Build dynamic action registry]
  end

    subgraph Public MCP Surface
        E[gitlab_find_action]
        F[gitlab_search_tools / gitlab_describe_tools for dynamic-3]
        G[gitlab_execute_tool]
    end

    subgraph Shared Execution Path
        H[ActionRoute metadata]
        I[Runtime schema validation]
        J[Existing domain handler]
        K[GitLab REST v4 or GraphQL]
    end

    A --> B --> C --> D
    D --> E
    D --> F
    D --> G
    G --> H --> I --> J --> K
```

The canonical action catalog is filtered after policy decisions such as enterprise catalog selection, GitLab.com-only routing, read-only mode, safe mode, excluded tools, and token-scope filtering. That means dynamic discovery only advertises actions that the current server instance can route.

### Search Ranking

`gitlab_find_action` and the dynamic-3 `gitlab_search_tools` use the same ranking signals:

- **Normalization**: query text is lower-cased and split on spaces, dots, underscores, and hyphens. Frequent words such as `the`, `to`, `with`, and `please` are dropped.
- **Search corpus**: each action is indexed by canonical ID, split ID words, backing meta-tool name, domain, action name, aliases, tags, required params, and schema property names.
- **Synonyms**: common task words expand to domain-specific alternatives. For example, `mr` expands toward merge-request terms, `secret` toward CI variables and tokens, `show` toward `get`, and `remove` toward `delete`. Backend vocabulary such as `github pr` and `jira ticket` normalizes to GitLab merge-request or issue concepts without exposing non-GitLab action IDs.
- **Exact ranking**: exact canonical IDs score highest, followed by aliases, tags, domain/action names, required params, schema enum values, schema property names, and broader search-text matches.
- **Fuzzy fallback**: typo-tolerant matching runs only when exact lexical search returns no matches or only low-confidence matches. It uses bounded Levenshtein distance up to two edits, ignores query tokens shorter than three characters, and suppresses weak fuzzy matches for destructive actions.
- **Segmented matching**: long multi-intent prompts are also searched in overlapping three- to six-term windows. This helps prompts such as `discover project from remote url merge request list current user open authored` surface both project discovery and merge-request listing candidates.
- **Ambiguity handling**: ambiguous aliases are reported with explicit canonical alternatives; execute rejects ambiguous aliases until the caller chooses one canonical `domain.action` ID.

The goal is not to make the model guess blindly. The model should use find to shortlist actions and fetch exact schemas, then execute. Use `dynamic-3` when you specifically want separate search and describe calls.

Find and search return only actions visible to the current server instance. Enterprise gating, GitLab.com-only routing, `GITLAB_READ_ONLY`, `GITLAB_SAFE_MODE`, excluded tools, and token-scope filtering are applied before the dynamic registry is exposed.

### Metadata Guidance

Dynamic discovery intentionally keeps most action metadata derived from canonical IDs, route schemas, required params, enum values, and route safety flags. Add hand-authored aliases, tags, usage hints, or related-action guidance only when deterministic corpus tests or model-backed Docker evaluations show that models confuse a specific family of actions.

Prefer compact metadata that teaches the distinction rather than broad synonyms on every action. Good candidates are dense domains where similar verbs compete, such as deploy keys versus deploy tokens, protected environments versus deployment approvals, release assets versus release links, issue notes versus issue discussions, and feature-flag user lists versus feature flags. After adding metadata, run the dynamic search corpus and a targeted Docker-backed model evaluation for the affected tasks.

## Dynamic vs Meta-Tools

| Concern | Meta-tools | Dynamic toolset |
| --- | --- | --- |
| Initial tool count | 33/49/50 | 2 |
| Model selection | Choose a domain tool and action | Find an action with schema, execute |
| Schema discovery | `action` enum plus optional schema resources or `META_PARAM_SCHEMA=compact/full` | `gitlab_find_action` returns action schemas inline |
| Minimal capabilities | Loses meta-schema resources and prompts | Keeps action schema discovery through find |
| Compatibility | Best current default | Best low-token alternative |
| Failure mode | Wrong domain/action choice | Skipped find or wrong action ID |
| Rollback | Default path | Switch back to `TOOL_SURFACE=meta` or unset `TOOL_SURFACE` |

For most users today, meta-tools remain the conservative default. Dynamic mode is recommended for low-token clients, evaluations, and deployments that benefit from compact progressive discovery.

## Developer Notes

Implementation entry points:

For the broader developer architecture of individual tools, meta-tools, dynamic surfaces, and the canonical action core, see [Tool Surfaces And Canonical Action Core](development/tool-surfaces-and-action-core.md).

| File | Responsibility |
| --- | --- |
| `internal/tools/actioncatalog/catalog.go` | Canonical action catalog data model, deterministic action ordering, lookup, and filters |
| `internal/tools/action_catalog.go` | Builds the canonical catalog from collected `ActionSpec` groups and the generated manifest |
| `internal/tools/meta_catalog.go` | Registers visible meta-tools from the canonical catalog |
| `internal/tools/dynamic/register.go` | Public dynamic tools, catalog-backed registry, search, describe, find, and execute logic |
| `internal/tools/dynamic/standalone.go` | Adds standalone actions such as project discovery and interactive creation flows to the canonical action catalog |
| `internal/tools/actioncompat` | Historical action aliases, parameter aliases, and execute-time compatibility normalizers projected into catalog metadata |
| `internal/toolutil/action_spec.go` | Canonical per-action metadata model, including aliases, tags, usage hints, related actions, and parameter guidance |
| `internal/toolutil/metatool.go` | Shared `ActionRoute`, route classification, schema helpers, and execution wrappers |
| `cmd/server/main.go` | Selects `TOOL_SURFACE` and registers meta, individual, dynamic, or comparison surfaces |
| `cmd/eval_mcp_surfaces` | Evaluates meta and dynamic surfaces against schema-only and Docker-backed tasks |
| `test/e2e/suite/dynamic_test.go` | E2E coverage for the default dynamic two-tool surface |

### Registering New Actions

Add or change GitLab actions by updating the owning typed handler and its `ActionSpec` entry, using typed route constructors such as `RouteAction`, `RouteActionWithRequest`, `DestructiveAction`, and their void variants. Do not add dynamic-only action definitions for normal GitLab operations. Once the action is in the canonical catalog, meta-tools expose it as a domain `action`, dynamic discovery can find it by canonical `domain.action` ID, `gitlab_find_action` can return its exact schema, and full capability surfaces can expose `gitlab://schema/meta/{tool}/{action}` schema resources.

Standalone dynamic helpers such as project discovery are the exception: add them in `internal/tools/dynamic/standalone.go` only when they are not normal meta-tool actions. Keep `dynamic-3` available as an explicit compatibility selector while `dynamic` uses the two-tool surface.

When adding or changing GitLab actions, keep these rules in sync:

1. Add or update the typed input/output structs and handler so schema capture remains accurate.
2. Add or update the owning `ActionSpec`, including the individual tool policy, aliases, tags, usage hints, and related actions when needed.
3. Regenerate the ActionSpec manifest when the source-defined builder set changes.
4. Keep destructive classification on route/spec metadata, not in dynamic-specific code.
5. Add regression tests for realistic search phrases that caused misses.
6. Refresh generated testing documentation after adding tests.

## Evaluation

Dynamic mode has dedicated unit coverage for search ranking, schema cloning, registry behavior, and query-shape edge cases. It also has Docker-backed E2E coverage for the default two-tool workflow:

```bash
E2E_MODE=docker go test -v -tags e2e -timeout 600s \
  -run '^TestDynamicToolSurface_' ./test/e2e/suite/
```

Model-facing evaluations can compare surfaces with `cmd/eval_mcp_surfaces`:

```bash
go run ./cmd/eval_mcp_surfaces --tool-surface=dynamic --dry-run --partition base-read
```

Use `dynamic-3` when you need to pin the explicit three-tool selector. Use `dynamic` for production-like configuration.

## Troubleshooting

| Symptom | Cause | Fix |
| --- | --- | --- |
| Only two tools appear | Dynamic mode is enabled | This is expected. Use `gitlab_find_action` to discover actions and schemas |
| Find returns many broad list actions | Query is too generic | Include the domain, resource, verb, and filter terms, such as `merge request list open authored by me` |
| Execute says the action is unknown | The model invented an action ID or the action was excluded | Find again and execute the canonical action ID from the result |
| Execute rejects parameters | Params do not match the found schema | Call `gitlab_find_action` for that action and retry with the exact field names and types |
| Destructive action returns an error | Confirmation is missing or policy blocks mutation | Add top-level `confirm:true` only when intentional, or check `GITLAB_READ_ONLY` and `GITLAB_SAFE_MODE` |
| Resources and prompts still use context | Capability surface is still full | Set `CAPABILITY_SURFACE=minimal` or `--capability-surface=minimal` |

## See Also

- [Meta-Tools Reference](meta-tools.md)
- [Configuration](configuration.md)
- [Environment Variable Reference](env-reference.md)
- [CLI Reference](cli-reference.md)
- [Architecture Overview](architecture.md)
- [AI Model Evaluation](testing/model-evaluation.md)
