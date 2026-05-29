# Tool Surfaces And Canonical Action Core

> **Diátaxis type**: Explanation
> **Audience**: Developers and AI agents changing GitLab tool registration
> **Prerequisites**: Familiarity with MCP tools, Go packages, and the project architecture

gitlab-mcp-server exposes the GitLab API through multiple MCP tool surfaces. The
surfaces differ in packaging and discovery behavior, not in the underlying
GitLab business logic.

## Tool Surfaces

| Surface          | Selector                                              |                             Visible MCP tools | Source of action metadata                                              |
| ---------------- | ----------------------------------------------------- | --------------------------------------------: | ---------------------------------------------------------------------- |
| Individual tools | `TOOL_SURFACE=individual` (`META_TOOLS=false` legacy) |                 One tool per GitLab operation | Canonical action catalog projected by `RegisterIndividualCatalogTools` |
| Dynamic          | default, `TOOL_SURFACE=dynamic`                       | `gitlab_find_action`, `gitlab_execute_action` | Canonical action catalog                                               |
| Meta-tools       | `TOOL_SURFACE=meta`                                   | Domain dispatchers with `action` and `params` | Canonical action catalog                                               |

Individual tools, meta-tools, and dynamic tools are now catalog-backed surfaces
over the same action core. Domain packages still own typed handlers,
ActionSpecs, markdown formatting, and compatibility behavior, but the root
individual runtime no longer loops through per-domain `RegisterTools` calls.
`RegisterAll` builds the canonical catalog and projects visible individual MCP
tools from it.

Catalog-first generation for individual tools is the runtime contract. New and
existing individual tools must derive their metadata from specs unless they are
documented non-GitLab standalone surfaces. See
[Catalog-First Individual Tools Policy](catalog-first-individual-tools.md) for
the parity checklist and current generation policy.

## Canonical Action Core

The canonical action core is the intermediate layer between typed GitLab domain
handlers and catalog-backed MCP surfaces.

```mermaid
flowchart LR
        handlers[Typed domain handlers\nand route constructors]
    specs[Domain ActionSpec builders]
  individual[Individual tool metadata\nprojection]
        catalog[Canonical action catalog]
        meta[Visible meta-tools\ndomain dispatchers]
        dynamic[Dynamic tools\nfind / execute]
    manifest[Tool manifest resources]
  docs[Generated docs\nLLM indexes and snapshots]
    audits[Catalog audits\nand snapshots]

    handlers --> specs
  specs --> individual
    specs --> catalog
        catalog --> meta
        catalog --> dynamic
    catalog --> manifest
  specs --> docs
    catalog --> audits
  specs --> audits
```

The core pieces are:

| File                                          | Responsibility                                                                                                                |
| --------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------- |
| `internal/toolutil/action_spec.go`            | Canonical per-action metadata model, validation, defensive cloning, compatibility policy, and route/catalog projection        |
| `internal/toolutil/action_spec_individual.go` | `ActionSpec` projection into individual `mcp.Tool` metadata and schema/annotation policy                                      |
| `internal/toolutil/metatool.go`               | Route primitives such as `ActionRoute`, `ActionMap`, schema helpers, destructive confirmation dispatch, and `MakeMetaHandler` |
| `internal/tools/actioncatalog/catalog.go`     | Canonical action catalog data model, deterministic ordering, filters, and `domain.action` IDs                                 |
| `internal/tools/actioncatalog/group_spec.go`  | `CatalogGroupSpec`, `SurfaceKind`, group validation, group option projection, and compatibility alias conflict checks         |
| `internal/tools/action_specs.go`              | Deterministic collector for domain `ActionSpec` builders, including Enterprise and GitLab.com gating                          |
| `internal/tools/action_catalog.go`            | Builds the canonical catalog from collected `ActionSpec` groups and the generated manifest                                    |
| `internal/tools/meta_catalog.go`              | Registers visible meta-tools from catalog groups                                                                              |
| `internal/tools/dynamic/register.go`          | Builds the dynamic registry, find output, internal search/describe helpers, and execute dispatch from the catalog             |
| `internal/tools/dynamic/standalone.go`        | Adds dynamic-only catalog actions that do not fit the normal meta route model                                                 |
| `cmd/audit_action_spec_coverage`              | Generates source-discovered ActionSpec coverage inventory across individual, meta, dynamic, and standalone surfaces           |

The catalog stores executable routes with input schemas, output schemas,
destructive flags, read-only status, icons, descriptions, aliases, tags, usage
hints, related actions, schema resource links, individual-tool compatibility
metadata, content policy placeholders, and formatters. Dynamic action IDs use
stable `domain.action` names such as `project.create`, `merge_request.list`,
and `repository.file_get`.

Every normal GitLab API action in the catalog is now backed by an `ActionSpec`.
Every normal individual GitLab API tool derives its MCP metadata from an
`ActionSpec` projection. The only documented source-level exceptions are the
dynamic catalog find/execute surface and the server auto-update
surface, which is wired from `cmd/server` with an updater instead of a GitLab
client.
Standalone dynamic actions such as `discover_project.resolve` and interactive
elicitation flows are added through their own spec builders before dynamic mode
registers visible tools.

## Final Model Vocabulary

The catalog-first runtime uses these model terms consistently:

- `ActionSpec`: one executable GitLab action, including the typed route,
  action name, behavior annotations, ownership, individual-tool projection
  metadata, discovery hints, result policies, validation notes, and
  compatibility policy.
- `CatalogGroupSpec`: one visible catalog group or utility surface. It declares
  the MCP tool name, title, description, icons, base dynamic domain, ownership,
  edition gates, GitLab.com-only gates, capability requirements, surface kind,
  and owned actions.
- `SurfaceKind`: an explicit classification for catalog and non-catalog
  surfaces. Valid values are `gitlab-action`, `meta-group`,
  `dynamic-controller`, `runtime-utility`, `interactive-utility`,
  `sampling-utility`, and `server-maintenance`.
- `SurfaceToolSpec`: the planned non-GitLab utility model for visible tools that
  are not ordinary GitLab API actions, such as dynamic controllers,
  interactive elicitation flows, sampling helpers, and server maintenance.
- Compatibility alias: an action-level alias with `Alias`, `Target`, `Source`,
  `Searchable`, `Deprecated`, `RemovalVersion`, and `Reason`. These replace
  hardcoded action-specific Dynamic aliases as the owned metadata source.
- Parameter alias: an action-scoped parameter alias with the same policy fields
  as compatibility aliases. Parameter aliases must target an input schema
  property owned by the same action.
- Surface controller: a visible controller tool that orchestrates catalog
  actions rather than representing one GitLab API endpoint, for example Dynamic
  find/execute.
- Runtime utility: a non-GitLab helper tool that supports the server runtime or
  user workflow. Runtime utilities need explicit surface classification,
  capability requirements when applicable, and guardrail tests.

## Registration Flow

`cmd/server/main.go` selects one tool surface when the server is created.

```mermaid
flowchart TD
    server[cmd/server.createServer]
    selector{Selected tool surface}

    server --> selector

    selector -->|individual| registerAll[internal/tools.RegisterAll]
    registerAll --> buildIndividual[internal/tools.BuildActionCatalog]
    buildIndividual --> collectIndividualSpecs[internal/tools.CollectActionSpecs]
    collectIndividualSpecs --> generatedManifest
    buildIndividual --> individualProjection[internal/tools.RegisterIndividualCatalogTools]
    individualProjection --> visibleIndividual[Visible individual tools]

    selector -->|meta| buildMeta[internal/tools.BuildActionCatalog]
    buildMeta --> collectSpecs[internal/tools.CollectActionSpecs]
    collectSpecs --> generatedManifest[action_specs_manifest_gen.go]
    buildMeta --> registerMeta[internal/tools.RegisterMetaCatalog]
    registerMeta --> metaTools[Visible domain meta-tools]

    selector -->|dynamic| buildDynamic[cmd/server.buildDynamicActionCatalog]
    buildDynamic --> collectDynamicSpecs[internal/tools.CollectActionSpecs]
    collectDynamicSpecs --> generatedManifest
    buildDynamic --> standalone[dynamic.AddStandaloneCatalog]
    standalone --> dynamicTools[dynamic.RegisterCatalogFindExecuteTools]
    dynamicTools --> twoTools[gitlab_find_action\ngitlab_execute_action]
```

## Action Spec Builder Ownership

Action spec builders are the boundary between domain handler packages and the
canonical action catalog. A builder owns route wiring, aliases, tags, usage
hints, related actions, parameter guidance, read-only status, destructive route
classification, individual-tool compatibility metadata, and Enterprise/Premium
route injection for one visible catalog group.

Target builder rules:

- A builder must return `[]toolutil.ActionSpec` or catalog metadata derived from
  specs; it must not require a live MCP server to produce routes.
- Building the catalog must not register MCP tools as a side effect. Visible
  tool registration belongs to `RegisterMetaCatalog` for meta mode and
  `dynamic.RegisterCatalogFindExecuteTools` for dynamic mode.
- Builders may stay in `internal/tools` while they depend on many domain
  packages. Domain-local `ActionSpecs` functions are preferred when one package
  owns the action semantics; central aggregation builders remain appropriate for
  visible groups that span many packages, such as `gitlab_admin` or
  `gitlab_group`.
- `action_specs_manifest_gen.go` is generated from the source-defined
  `build*ActionSpecs` aggregation builders. Add or remove a builder in source,
  then run `make gen-action-catalog-manifest`; CI and audits should use
  `make check-action-catalog-manifest` or `cmd/audit_action_spec_coverage` to
  fail when the manifest is stale.
- Domain packages should expose typed handlers, markdown formatters, and
  `ActionSpecs` without importing `internal/tools/actioncatalog`. Catalog
  projection and visible tool registration belong to the central
  `internal/tools` layer.
- Package-level `RegisterMeta` functions are no longer an approved runtime
  registration pattern. Ordinary GitLab API operations should flow through
  ActionSpecs, the generated builder manifest, `BuildActionCatalog`, and the
  catalog-backed surface registrars.

Current direction: keep the catalog composition in `internal/tools`, keep
domain-owned spec builders close to their handlers, and use central aggregation
only when a visible meta group intentionally spans multiple packages.

## Standalone Dynamic Actions

Most dynamic actions come from the canonical catalog built directly from specs
and the generated builder manifest. A small set of actions are added only for
dynamic mode because they are standalone tools or interactive flows rather than
normal meta-tool actions.

Current standalone dynamic additions live in `internal/tools/dynamic/standalone.go`:

- `discover_project.resolve` from `gitlab_discover_project`.
- `interactive.issue_create`, `interactive.mr_create`, `interactive.project_create`, and `interactive.release_create` when read-only mode and exclusions allow them.

Do not add dynamic-only copies of ordinary GitLab API operations. Add ordinary
operations through the owning domain's `ActionSpecs` and the route definitions
that feed the canonical catalog.

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
meta-tools from the filtered catalog. After tools are registered, the server
exposes `gitlab://tools` and `gitlab://tools/{id}` as the public, surface-aware
manifest for accepted call shapes and input schemas.

## Tool Manifest Resources

Every runtime tool surface registers the same public manifest resources:

```text
gitlab://tools
gitlab://tools/{id}
```

The manifest payload adapts to the active surface selected at startup. Dynamic
entries use canonical `domain.action` IDs such as `project.get`; meta entries
use `{tool}.{action}` IDs such as `gitlab_project.get`; individual entries use
the direct tool name such as `gitlab_get_project`. Compatibility helpers for
older `gitlab://schema/*` resources remain isolated in package tests, but they
are not registered by the production server.

## Historical Duplication

ADR-0005 consolidated many standalone meta-tools into broader domain meta-tools.
The visible taxonomy changed, and the intermediate package-level `RegisterMeta`
bridge has now been removed from active runtime registration.

Current baseline for delegated meta ownership:

| Metric                                                                           | Count |
| -------------------------------------------------------------------------------- | ----: |
| Package-level `RegisterMeta` definitions under `internal/tools/*`                |     0 |
| Delegated `RegisterMeta` calls referenced from `internal/tools/register_meta.go` |     0 |
| Apparent legacy `RegisterMeta` definitions requiring verification                |     0 |

Historical names still handled as compatibility aliases:

| Historical name              | Current visible surface |
| ---------------------------- | ----------------------- |
| `gitlab_feature_flag`        | `gitlab_feature_flags`  |
| `gitlab_ff_user_list`        | `gitlab_feature_flags`  |
| `gitlab_registry`            | `gitlab_package`        |
| `gitlab_registry_protection` | `gitlab_package`        |
| `gitlab_access_request`      | `gitlab_access`         |
| `gitlab_project_snippet`     | `gitlab_snippet`        |

Package-level `RegisterMeta` functions are now treated as audit violations.
Former delegated groups such as `gitlab_search`, `gitlab_runner`,
`gitlab_analyze`, and `gitlab_orbit` are catalog-backed groups.

## Import Layering Rules

- Domain packages under `internal/tools/{domain}` may import `internal/toolutil`.
- `internal/toolutil` must not import domain packages.
- `internal/tools` may import domain packages to wire registration.
- `internal/tools/actioncatalog` may import `internal/toolutil` for route metadata.
- `internal/tools/dynamic` may import `internal/tools/actioncatalog` and `internal/toolutil`, but must not depend on visible individual MCP tools.
- `cmd/server` is the composition root for selecting and filtering the active surface.

## Catalog Metadata Guidance

The canonical catalog should stay compact and executable first. Specs must carry
the stable action ID, typed input/output schemas, read-only and destructive
flags, schema resource links, icons, markdown formatter metadata, individual
tool compatibility metadata, aliases, tags, usage hints, related actions, and
parameter guidance. Dynamic search derives most discovery signals from that
source: canonical ID words, domain and action names, required params, schema
properties, enum values, aliases, tags, usage hints, and related actions.

Add hand-authored dynamic aliases, tags, usage hints, or related actions only when there is evidence of model confusion. Evidence can come from the deterministic dynamic search corpus, provider traces, targeted model-backed evaluations, or a real user prompt that mapped to the wrong action. Keep additions narrow to the confused family and prefer terms that disambiguate competing routes instead of generic keywords that would match many actions.

Use the alias and discovery audits in `internal/tools/dynamic` after metadata changes. If a registry is already built, call `AuditRegistryDiscoveryTerms`; use `AuditCatalogDiscoveryTerms` when only a catalog is available. A dense action family should either be discoverable from schemas and route names or have compact targeted guidance.

## ActionSpec Authoring Pattern

Domain-local specs should wrap the same typed route constructors used by the
catalog and individual metadata projections. The constructor call site stays
familiar, and optional catalog metadata is added with `ActionRoute` copy helpers
before the route is passed to `toolutil.NewActionSpec`.

```go
func ProjectSpecs(client *gitlabclient.Client, _ bool) []toolutil.ActionSpec {
  listRoute := toolutil.RouteAction(client, projects.List).
    WithAliases("project search").
    WithTags("project").
    WithUsage("Use to list or search projects; use project.get when one project is known.").
    WithRelatedActions("project.get").
    WithParameterGuidance(map[string]toolutil.ParameterGuidance{
      "search": {SemanticRole: "project_search_query"},
    })

  return []toolutil.ActionSpec{
    toolutil.NewActionSpec("list", listRoute, toolutil.ActionSpecOptions{
      ReadOnly:       true,
      Idempotent:     true,
      OpenWorld:      true,
      OwnerPackage:   "projects",
      IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_list_projects"},
      ContentKind:    "list",
    }),
  }
}
```

The helpers return copies, so callers can safely reuse base routes without
sharing mutable schema, guidance, alias, tag, or related-action slices. When a
spec wraps a route, route-local aliases, tags, usage, related actions, and
parameter guidance become defaults; explicit `ActionSpecOptions` values may add
or override the metadata for that spec.

## When Adding A GitLab Action

1. Add or update the typed handler in the appropriate domain package.
2. Add an `ActionSpec` in the owning domain package or central aggregation builder with the exact individual tool name and title when the individual surface should expose it.
3. Ensure the source-defined builder is present in `action_specs_manifest_gen.go` by running `make gen-action-catalog-manifest` or `go run ./cmd/gen_action_catalog_manifest/`.
4. Keep destructive classification on route/spec metadata, not in dynamic-only code.
5. Add dynamic search tags or aliases only when natural language discovery is weak and there is evidence from tests, traces, evaluations, or user prompts.
6. Update tests, generated docs, and tool manifest expectations as required.

## ActionSpec Guardrails

The migration is enforced by source-level tests and audits:

- `TestActionSpecCoverage_AllCatalogRoutesClassified` builds the GitLab.com
  Enterprise dynamic catalog and fails if any catalog action is not spec-backed.
- `TestCollectedActionSpecs_ProjectIntoActionCatalog` builds the catalog for
  CE, self-managed Enterprise, and GitLab.com Enterprise, then checks spec
  action routes, destructive flags, schemas, read-only projection, and
  individual-tool metadata.
- `TestCollectedActionSpecs_KnownGuidancePreserved` locks the parameter guidance
  for historically ambiguous actions such as merge request creation, issue
  links, epic issue assignment, CI job token scope removal, and deploy token
  deletion.
- `TestCollectedActionSpecs_DeclareCatalogOwnership` requires every collected
  catalog group to declare an owner and every action owner to resolve to the
  central `tools` package or a real `internal/tools/*` package.
- `TestIndividualToolProjection_RepresentativeDomainParity` pilots the
  `ActionSpec`-to-`mcp.Tool` projection adapter against registered individual
  tools for project, issue, merge request, job, and group domains.
- `TestIndividualToolProjection_GoldenSnapshotParity` compares projected
  individual metadata against `internal/tools/testdata/tools_individual.json`
  and fails on unexpected schema or annotation drift.
- `TestIndividualToolMetadata_SourceRegistrationUsesActionSpecProjection` scans
  `internal/tools/*/register.go` and fails if manual `&mcp.Tool{...}` metadata
  returns outside documented standalone surfaces.
- `TestIndividualToolMetadata_CatalogBackedCoverage` verifies every
  catalog-backed spec points to a registered individual tool and every
  registered individual tool without ActionSpec metadata is an explicit
  standalone exception.
- `cmd/audit_action_spec_coverage` writes `dist/action-spec-coverage.json` with
  source-discovered surface classifications for domain coverage sweeps and
  fails when `action_specs_manifest_gen.go` is stale.
- `make check-action-catalog-manifest` verifies the generated ActionSpec group
  builder manifest without writing files.
- `TestActionCatalog_BaselineCountsDoNotRegress` keeps CE, Enterprise, and
  GitLab.com Enterprise catalog action counts stable.
- `make audit-dynamic-aliases`, `go run ./cmd/audit_output/`,
  `go run ./cmd/audit_tools/`, `go run ./cmd/audit_meta_schema/`, and
  `go run ./cmd/audit_tokens/` are the post-metadata-change surface audits.

## Useful Checks

```bash
go run ./cmd/gen_action_catalog_manifest/ --check
rg -n "func RegisterMeta\(" internal/tools --glob '*.go'
rg -n "\*gitlab\.Client" internal/tools -g 'register.go'
rg -n "BuildActionCatalog\(|RegisterMetaCatalog\(|actioncatalog\.|internal/tools/actioncatalog|internal/tools/dynamic" --glob '*.go' cmd internal
```

Use these checks after adding or moving catalog builders, changing registration
paths, or tightening legacy registration audits.
