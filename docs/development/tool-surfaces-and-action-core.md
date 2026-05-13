# Tool Surfaces And Canonical Action Core

> **Diátaxis type**: Explanation
> **Audience**: Developers and AI agents changing GitLab tool registration
> **Prerequisites**: Familiarity with MCP tools, Go packages, and the project architecture

gitlab-mcp-server exposes the GitLab API through multiple MCP tool surfaces. The
surfaces differ in packaging and discovery behavior, not in the underlying
GitLab business logic.

## Tool Surfaces

| Surface | Selector | Visible MCP tools | Source of action metadata |
| --- | --- | ---: | --- |
| Individual tools | `TOOL_SURFACE=individual` or `META_TOOLS=false` | One tool per GitLab operation | Direct `RegisterTools` calls in domain packages |
| Meta-tools | default, `TOOL_SURFACE=meta` | Domain dispatchers with `action` and `params` | Canonical action catalog |
| Dynamic / dynamic-3 | `TOOL_SURFACE=dynamic` or `TOOL_SURFACE=dynamic-3` | `gitlab_search_tools`, `gitlab_describe_tools`, `gitlab_execute_tool` | Canonical action catalog |
| Dynamic-2 | `TOOL_SURFACE=dynamic-2` | `gitlab_find_action`, `gitlab_execute_tool` | Canonical action catalog |

Individual tools are a direct compatibility surface. Meta-tools and dynamic
tools are catalog-backed surfaces over the same action core.

Catalog-first generation for individual tools has been evaluated and deferred.
See [Catalog-First Individual Tools Evaluation](catalog-first-individual-tools.md)
for the current decision and the metadata gaps that must be closed before any
future generator is considered.

## Canonical Action Core

The canonical action core is the intermediate layer between typed GitLab domain
handlers and catalog-backed MCP surfaces.

```text
Typed domain handlers and route constructors
                 |
                 v
Canonical action catalog
       |                         |
       v                         v
Visible meta-tools        Dynamic search/describe/execute
```

The core pieces are:

| File | Responsibility |
| --- | --- |
| `internal/toolutil/metatool.go` | Route primitives such as `ActionRoute`, `ActionMap`, schema helpers, destructive confirmation dispatch, and `MakeMetaHandler` |
| `internal/tools/actioncatalog/catalog.go` | Canonical action catalog data model, deterministic ordering, filters, and `domain.action` IDs |
| `internal/tools/action_catalog.go` | Builds the catalog from current meta route definitions without constructing an MCP server |
| `internal/tools/meta_catalog.go` | Registers visible meta-tools from catalog groups |
| `internal/tools/dynamic/register.go` | Builds the dynamic registry, search index, describe output, and execute dispatch from the catalog |
| `internal/tools/dynamic/standalone.go` | Adds dynamic-only catalog actions that do not fit the normal meta route model |

The catalog stores executable routes with input schemas, output schemas,
destructive flags, read-only status, icons, descriptions, aliases, tags, and
formatters. Dynamic action IDs use stable `domain.action` names such as
`project.create`, `merge_request.list`, and `repository.file_get`.

## Registration Flow

`cmd/server/main.go` selects one tool surface when the server is created.

```text
TOOL_SURFACE=individual
        |
        v
internal/tools.RegisterAll
        |
        v
domain RegisterTools functions
```

```text
TOOL_SURFACE=meta
        |
        v
internal/tools.BuildActionCatalog
        |
        v
internal/tools.RegisterMetaCatalog
```

```text
TOOL_SURFACE=dynamic or dynamic-3
        |
        v
cmd/server.buildDynamicActionCatalog
        |
        v
dynamic.AddStandaloneCatalog
        |
        v
dynamic.RegisterCatalogTools
```

```text
TOOL_SURFACE=dynamic-2
        |
        v
cmd/server.buildDynamicActionCatalog
        |
        v
dynamic.RegisterCatalogFindExecuteTools
```

## Action Group Builder Ownership

Action group builders are the boundary between domain handler packages and the
canonical action catalog. A builder owns route maps, descriptions, icons,
read-only status, destructive route classification, custom markdown formatters,
and Enterprise/Premium route injection for one visible catalog group.

Target builder rules:

- A builder must return catalog metadata or captureable meta-tool metadata; it
  must not require a live MCP server to produce routes.
- Building the catalog must not register MCP tools as a side effect. Visible
  tool registration belongs to `RegisterMetaCatalog` for meta mode and
  `dynamic.RegisterCatalogTools` / `dynamic.RegisterCatalogFindExecuteTools` for
  dynamic modes.
- Builders may stay in `internal/tools` while they depend on many domain
  packages. Prefer splitting central files by domain area before moving builders
  into domain sub-packages.
- Domain packages should expose typed handlers and individual `RegisterTools`
  functions. They should not import `internal/tools/actioncatalog` unless a
  later ADR moves group ownership into those packages.
- Delegated meta groups are allowed only for packages explicitly called from
  `registerAllMetaGroups`; otherwise ordinary GitLab API operations should flow
  through the central catalog builders.

Current direction: keep builders in `internal/tools`, split them into focused
`register_meta_*.go` files by domain area, and keep `register_meta.go` as the
composition entry point for visible meta registration and catalog capture.

## Standalone Dynamic Actions

Most dynamic actions come from the canonical catalog built from meta route
definitions. A small set of actions are added only for dynamic mode because they
are standalone tools or interactive flows rather than normal meta-tool actions.

Current standalone dynamic additions live in `internal/tools/dynamic/standalone.go`:

- `discover_project.resolve` from `gitlab_discover_project`.
- `interactive.issue_create`, `interactive.mr_create`, `interactive.project_create`, and `interactive.release_create` when read-only mode and exclusions allow them.

Do not add dynamic-only copies of ordinary GitLab API operations. Add ordinary
operations through the normal route definitions that feed the canonical catalog.

## Filtering And Policy Order

Filtering must happen before a catalog-backed surface exposes actions to a
client. The relevant policies are:

- Enterprise/Premium and GitLab.com-only catalog selection.
- `ExcludeTools` configuration.
- Token-scope filtering.
- `GITLAB_READ_ONLY` / `--read-only` filtering.
- `GITLAB_SAFE_MODE` / `--safe-mode` previews.
- Capability surface selection for resources and prompts.

Dynamic mode builds a filtered catalog before constructing the dynamic registry,
so search and execute cannot see hidden actions. Meta mode registers visible
meta-tools from the filtered catalog and then exposes schema resources for the
visible route set.

## Schema Resources

Meta and dynamic modes can register meta-schema resources when the capability
surface is full. The URI format remains:

```text
gitlab://schema/meta/{tool}/{action}
```

Individual mode does not need these resources for action-specific schemas
because every individual tool already advertises its own tool input schema.

## Historical Duplication

ADR-0005 consolidated many standalone meta-tools into broader domain meta-tools.
The visible taxonomy changed, but many package-level `RegisterMeta` functions
were intentionally left in place during the migration.

Current baseline for cleanup planning:

| Metric | Count |
| --- | ---: |
| Package-level `RegisterMeta` definitions under `internal/tools/*` | 4 |
| Delegated `RegisterMeta` calls referenced from `internal/tools/register_meta.go` | 4 |
| Apparent legacy `RegisterMeta` definitions requiring verification | 0 |

Known stale examples:

| Historical name | Current visible surface |
| --- | --- |
| `gitlab_feature_flag` | `gitlab_feature_flags` |
| `gitlab_ff_user_list` | `gitlab_feature_flags` |
| `gitlab_registry` | `gitlab_package` |
| `gitlab_registry_protection` | `gitlab_package` |
| `gitlab_access_request` | `gitlab_access` |
| `gitlab_project_snippet` | `gitlab_snippet` |

Before deleting any legacy `RegisterMeta` function, prove that its actions are
covered by `BuildActionCatalog(...).ActionMaps()` through the current visible
meta-tool group and that no tests or generated docs depend on the historical
tool name.

## Import Layering Rules

- Domain packages under `internal/tools/{domain}` may import `internal/toolutil`.
- `internal/toolutil` must not import domain packages.
- `internal/tools` may import domain packages to wire registration.
- `internal/tools/actioncatalog` may import `internal/toolutil` for route metadata.
- `internal/tools/dynamic` may import `internal/tools/actioncatalog` and `internal/toolutil`, but must not depend on visible individual MCP tools.
- `cmd/server` is the composition root for selecting and filtering the active surface.

## When Adding A GitLab Action

1. Add or update the typed handler in the appropriate domain package.
2. Register the individual tool through that package's `RegisterTools` when the individual surface should expose it.
3. Add the catalog-backed route to the central route definitions or an approved delegated meta group using typed `ActionRoute` constructors.
4. Keep destructive classification on the route metadata, not in dynamic-only code.
5. Add dynamic search tags or aliases only when natural language discovery is weak.
6. Update tests, generated docs, and schema resources as required.

## Useful Checks

```bash
rg -n "func RegisterMeta\(" internal/tools --glob '*.go'
rg -n "RegisterMeta\(server, client\)|RegisterMeta\(server, .*client" internal/tools/register_meta.go
rg -n "BuildActionCatalog\(|RegisterMetaCatalog\(|actioncatalog\.|internal/tools/actioncatalog|internal/tools/dynamic" --glob '*.go' cmd internal
```

Use these checks before removing legacy registration functions or moving the
catalog package.
