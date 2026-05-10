# Dynamic Toolset

The dynamic toolset is the low-token operating mode for gitlab-mcp-server. It exposes a tiny public MCP surface and lets the model discover the canonical GitLab action catalog progressively.

> **Diataxis type**: Guide + Reference
> **Audience**: Users, operators, and developers evaluating low-token MCP deployments
> **Prerequisites**: Basic MCP tool-call concepts and a configured GitLab token
> **Status**: Meta-tools remain the default today. The dynamic toolset is the current default candidate for a future low-token default.
> **ADR**: See [ADR-0011: Low-token dynamic toolset mode](adr/adr-0011-low-token-dynamic-toolset.md).

## When To Use It

Use the dynamic toolset when the initial MCP `tools/list` payload is the limiting factor for your AI client. This is common with clients that have small tool-context budgets, strict tool-count limits, or slow tool palette rendering.

| Mode | Visible Tools | Best For |
| --- | ---: | --- |
| Meta-tools, current default | 32 base / 47 self-managed enterprise / 48 GitLab.com Enterprise | Broad compatibility and predictable domain-level action selection |
| Dynamic toolset | 3 | Low-token clients that can search, describe, then execute actions |
| Individual tools | 863 CE / 1006 self-managed enterprise / 1011 GitLab.com Enterprise | Clients that benefit from one tool per GitLab operation |

Dynamic mode keeps the same underlying GitLab coverage as meta-tools. It changes discovery, not business behavior.

## Public Tools

`TOOL_SURFACE=dynamic` and `TOOL_SURFACE=dynamic-3` expose the same current three-tool surface:

| Tool | Purpose |
| --- | --- |
| `gitlab_search_tools` | Search the canonical action catalog using natural language, canonical action IDs, domains, verbs, aliases, and fuzzy matching |
| `gitlab_describe_tools` | Return exact input schemas, examples, safety metadata, and output summaries for selected action IDs |
| `gitlab_execute_tool` | Execute one selected action by canonical `domain.action` ID with runtime validation and safety checks |

The parked comparison surface `TOOL_SURFACE=dynamic-2` exposes `gitlab_find_action` and `gitlab_execute_tool`. It exists for evaluation and A/B testing. Do not treat dynamic-2 as the main migration target.

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

`CAPABILITY_SURFACE=minimal` keeps `gitlab://workspace/roots` and omits optional resources and prompts. Dynamic execution still works because `gitlab_describe_tools` returns exact action schemas inline.

## User Workflow

The model should use a three-step workflow:

1. Search for candidate actions with `gitlab_search_tools`.
2. Describe the action or actions it intends to call with `gitlab_describe_tools`.
3. Execute one validated action with `gitlab_execute_tool`.

```mermaid
sequenceDiagram
    participant User
    participant LLM as AI Client
    participant Search as gitlab_search_tools
    participant Describe as gitlab_describe_tools
    participant Execute as gitlab_execute_tool
    participant GitLab

    User->>LLM: Show open MRs authored by me
    LLM->>Search: query="merge request list open authored by me"
    Search-->>LLM: merge_request.list and related candidates
    LLM->>Describe: actions=["merge_request.list"]
    Describe-->>LLM: exact params schema and examples
    LLM->>Execute: action="merge_request.list", params={...}
    Execute->>GitLab: GitLab API request
    GitLab-->>Execute: JSON response
    Execute-->>LLM: Markdown + structured content
    LLM-->>User: Human-readable result
```

## Example Calls

### Search

```json
{
  "tool": "gitlab_search_tools",
  "arguments": {
    "query": "merge request list open authored by me project",
    "limit": 5
  }
}
```

A result contains canonical action IDs such as `merge_request.list`, domains, summaries, aliases, read-only/destructive metadata, and short reasoning hints.

### Describe

```json
{
  "tool": "gitlab_describe_tools",
  "arguments": {
    "actions": ["merge_request.list"]
  }
}
```

Descriptions include input schemas and examples. Output schemas are intentionally omitted from the dynamic description payload to keep discovery small; output behavior remains documented in action summaries and normal tool responses.

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

The action ID is canonical. Aliases can help search, but execution should use the action ID returned by search or describe.

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
    "params": {
      "project_id": "my-group/my-project",
      "confirm": true
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
        E[gitlab_search_tools]
        F[gitlab_describe_tools]
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

The canonical action catalog is filtered after policy decisions such as enterprise catalog selection, GitLab.com-only routing, read-only mode, safe mode, excluded tools, and token-scope filtering. That means dynamic search only advertises actions that the current server instance can route.

### Search Ranking

`gitlab_search_tools` combines several signals:

- Canonical IDs such as `merge_request.list`.
- Domain and action names.
- Human descriptions and aliases.
- Verb synonyms such as show/list/get and remove/delete.
- Stopword filtering for natural language queries.
- Fuzzy matching for typo recovery.
- Segmented matching for multi-intent prompts, such as `discover project from remote url merge request list current user open authored`.

The goal is not to make the model guess blindly. The model should use search to shortlist actions, describe to fetch exact schemas, then execute.

## Dynamic vs Meta-Tools

| Concern | Meta-tools | Dynamic toolset |
| --- | --- | --- |
| Initial tool count | 32/47/48 | 3 |
| Model selection | Choose a domain tool and action | Search an action registry, describe, execute |
| Schema discovery | `action` enum plus optional schema resources | `gitlab_describe_tools` returns action schemas inline |
| Compatibility | Best current default | Best low-token candidate |
| Failure mode | Wrong domain/action choice | Skipped search/describe or wrong action ID |
| Rollback | Default path | Switch back to `TOOL_SURFACE=meta` or unset `TOOL_SURFACE` |

For most users today, meta-tools remain the conservative default. Dynamic mode is recommended for low-token clients, evaluations, and deployments preparing for the expected future default.

## Developer Notes

Implementation entry points:

| File | Responsibility |
| --- | --- |
| `internal/tools/actionregistry/registry.go` | Canonical action catalog data model, deterministic action ordering, lookup, and filters |
| `internal/tools/action_catalog.go` | Builds the canonical catalog from meta-tool definitions without constructing an MCP server |
| `internal/tools/meta_catalog.go` | Registers visible meta-tools from the canonical catalog |
| `internal/tools/dynamic/register.go` | Public dynamic tools, catalog-backed registry, search, describe, find, and execute logic |
| `internal/tools/dynamic/standalone.go` | Adds standalone actions such as project discovery and interactive creation flows to the canonical action catalog |
| `internal/tools/register_meta.go` | Source of the domain meta-tool definitions used to build the canonical action catalog |
| `internal/toolutil/metatool.go` | Shared `ActionMap`, `ActionRoute`, route classification, schema capture, and execution wrappers |
| `cmd/server/main.go` | Selects `TOOL_SURFACE` and registers meta, individual, dynamic, or comparison surfaces |
| `cmd/eval_meta_tools` | Evaluates meta and dynamic surfaces against schema-only and Docker-backed tasks |
| `test/e2e/suite/dynamic_test.go` | E2E coverage for the default dynamic three-tool surface |

### Registering New Actions

Add or change GitLab actions in the domain route definitions that feed `internal/tools/register_meta.go`, using typed route constructors such as `RouteAction`, `RouteActionWithRequest`, `DestructiveAction`, and their void variants. Do not add dynamic-only action definitions for normal GitLab operations. Once the action is in the canonical catalog, meta-tools expose it as a domain `action`, dynamic search can discover it by canonical `domain.action` ID, `gitlab_describe_tools` can return its exact schema, and schema resources can expose `gitlab://schema/meta/{tool}/{action}`.

Standalone dynamic helpers such as project discovery are the exception: add them in `internal/tools/dynamic/standalone.go` only when they are not normal meta-tool actions. Keep `dynamic-2` as an evaluation-only comparison surface unless the project explicitly decides otherwise.

When adding or changing GitLab actions, keep these rules in sync:

1. Register the action in the normal meta-tool route map first.
2. Preserve typed input/output structs so schema capture remains accurate.
3. Add or update dynamic search tags only when natural language discovery would otherwise be weak.
4. Keep destructive classification on the route, not in dynamic-specific code.
5. Add regression tests for realistic search phrases that caused misses.
6. Refresh generated testing documentation after adding tests.

## Evaluation

Dynamic mode has dedicated unit coverage for search ranking, schema cloning, registry behavior, and query-shape edge cases. It also has Docker-backed E2E coverage for the default three-tool workflow:

```bash
E2E_MODE=docker go test -v -tags e2e -timeout 600s \
  -run '^TestDynamicToolSurface_' ./test/e2e/suite/
```

Model-facing evaluations can compare surfaces with `cmd/eval_meta_tools`:

```bash
go run ./cmd/eval_meta_tools --tool-surface=dynamic --dry-run --partition base-read
```

Use `dynamic-3` when you need an explicit name for the current three-tool candidate in A/B reports. Use `dynamic` for production-like configuration.

## Troubleshooting

| Symptom | Cause | Fix |
| --- | --- | --- |
| Only three tools appear | Dynamic mode is enabled | This is expected. Use `gitlab_search_tools` and `gitlab_describe_tools` to discover actions |
| Search returns many broad list actions | Query is too generic | Include the domain, resource, verb, and filter terms, such as `merge request list open authored by me` |
| Execute says the action is unknown | The model invented an action ID or the action was excluded | Search again and execute the canonical action ID from the result |
| Execute rejects parameters | Params do not match the described schema | Call `gitlab_describe_tools` for that action and retry with the exact field names and types |
| Destructive action returns an error | Confirmation is missing or policy blocks mutation | Add `confirm:true` only when intentional, or check `GITLAB_READ_ONLY` and `GITLAB_SAFE_MODE` |
| Resources and prompts still use context | Capability surface is still full | Set `CAPABILITY_SURFACE=minimal` or `--capability-surface=minimal` |

## See Also

- [Meta-Tools Reference](meta-tools.md)
- [Configuration](configuration.md)
- [Environment Variable Reference](env-reference.md)
- [CLI Reference](cli-reference.md)
- [Architecture Overview](architecture.md)
- [AI Model Evaluation](testing/model-evaluation.md)
