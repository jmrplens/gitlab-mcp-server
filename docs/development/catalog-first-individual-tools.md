# Catalog-First Individual Tools Policy

> **Diátaxis type**: Explanation
> **Audience**: Developers changing tool registration surfaces
> **Prerequisites**: Familiarity with `ActionSpec`, meta-tools, and the canonical action catalog

The individual MCP tool surface is registered from the canonical action catalog.
Each normal GitLab API tool obtains its visible `mcp.Tool` definition from
`toolutil.ActionSpec` projection while the owning package keeps typed handlers,
logging, not-found behavior, embedded resources, rich results, markdown
formatting, and compatibility-specific runtime logic local.

This policy keeps the individual surface stable for existing clients while
removing duplicated metadata from hundreds of former registration call sites.

## Current Policy

Individual tool registration has two layers:

| Layer                 | Owner                                        | Policy                                                                                                                                         |
| --------------------- | -------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| Handler behavior      | Typed domain handlers and `ActionSpec.Route` | Keep explicit closures for logging, request-aware handlers, not-found conversion, embedded resources, rich results, and compatibility behavior |
| Visible tool metadata | `ActionSpec` projection                      | Derive title, description, icons, input/output schemas, annotations, and compatibility metadata from specs                                     |

`RegisterAll` calls `RegisterIndividualCatalogTools`, which projects eligible
catalog actions into individual MCP tools. Projection fails fast when an action
has no individual tool policy or when spec metadata cannot produce a valid
`mcp.Tool`.

Documented source-level exceptions are intentionally narrow:

| Package                                       | Reason                                                                                                                                            |
| --------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------- |
| `internal/tools/dynamic/register.go`          | Registers the dynamic find/execute controller surface generated from the canonical catalog, not individual GitLab API tools                       |
| `internal/tools/serverupdate/action_specs.go` | Defines updater tool handlers that are registered from catalog surface specs with `*autoupdate.Updater`, outside the GitLab client action catalog |

## Parity Checklist

When adding or changing an individual tool, verify each item before merging:

- The owning package exposes an `ActionSpecs` function for the action.
- The spec sets the exact historical individual tool name in `IndividualTool.Name`.
- The spec sets the individual title and any description override needed for the visible tool.
- The spec carries the same input and output schemas as the registered handler route.
- The spec annotations match the existing read-only, destructive, idempotent, and open-world semantics.
- The spec sets content, not-found, embedded-resource, rich-result, schema-validation, and runtime-validation policies when the action needs non-default behavior.
- `RegisterIndividualCatalogTools` projects the action into the visible `mcp.Tool` metadata.
- Any direct `&mcp.Tool{...}` construction is either removed or added to the documented standalone exception list with a reason.
- Tool snapshots and ActionSpec guardrails pass without adding unexpected allowlist entries.

## Representative Patterns

### Standard Projected Tool

Most tools follow the same shape: define the action spec near the domain
handler, then let the catalog-backed individual registrar project the visible
tool metadata.

```go
func ActionSpecs(client *gitlabclient.Client, enterprise bool) []toolutil.ActionSpec {
  route := toolutil.RouteAction(client, Get).
    WithUsage("Use when you already know the project ID or full path.")

  return []toolutil.ActionSpec{
    toolutil.NewActionSpec("get", route, toolutil.ActionSpecOptions{
      ReadOnly:     true,
      Idempotent:   true,
      OwnerPackage: "projects",
      IndividualTool: toolutil.IndividualToolSpec{
        Name:        "gitlab_get_project",
        Title:       "Get project",
        Description: "Get a single GitLab project by ID or path.",
      },
    }),
  }
}
```

The individual registrar receives the built catalog and projects this metadata
into the final `mcp.Tool`. Schemas, annotations, title, icons, compatibility
aliases, and shared metadata all come from the matching spec.

### Handler-Specific Compatibility

Keep compatibility behavior in the domain closure when it is not simple MCP tool
metadata:

- `gitlab_project_get` can convert 404 responses to `toolutil.NotFoundResult`,
  log them at info level, and embed `gitlab://project/{id}` after a successful
  response.
- `gitlab_file_get` can return image content through `ToolResultWithImage` when
  the repository file is an image.
- Package download and publish tools preserve request-aware progress, local file
  validation, and disk output behavior in their handlers.

Those behaviors are documented in specs through policy fields and validation
notes, but the runtime mechanics remain in domain code.

## Guardrails

The migration is enforced by tests and audits:

| Guardrail                                                               | What it proves                                                                                                                                  |
| ----------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| `TestRegisterAllDoesNotUseDomainRegisterTools`                          | Root individual registration cannot regress to per-domain `RegisterTools` loops                                                                 |
| `TestIndividualToolMetadata_SourceRegistrationUsesActionSpecProjection` | Source registration files do not reintroduce manual individual `mcp.Tool` metadata outside documented exceptions                                |
| `TestIndividualToolProjection_GoldenSnapshotParity`                     | Projected metadata matches `internal/tools/testdata/tools_individual.json` except explicit, reviewed gaps                                       |
| `TestIndividualToolMetadata_CatalogBackedCoverage`                      | Every catalog-backed spec references a registered individual tool, and every individual tool without a spec is an explicit standalone exception |
| `cmd/audit_action_spec_coverage`                                        | Every source domain is classified across individual, meta, dynamic, and standalone surfaces                                                     |
| `TestActionSpecCoverage_AllCatalogRoutesClassified`                     | Every GitLab.com Enterprise dynamic catalog route is spec-backed                                                                                |

Run the focused checks before changing individual registration policy:

```bash
go test ./internal/tools -run 'TestIndividualToolMetadata_SourceRegistrationUsesActionSpecProjection|TestIndividualToolMetadata_CatalogBackedCoverage|TestIndividualToolProjection_GoldenSnapshotParity|TestActionSpecCoverage_AllCatalogRoutesClassified' -count=1
make audit-action-spec-coverage
```

Regenerate snapshots only when intentional metadata changes occur:

```bash
UPDATE_TOOLSNAPS=true go test ./internal/tools -run 'TestToolSnapshots_(Individual|Meta)$' -count=1
```

## What Not To Do

- Do not generate production handler closures from the catalog. Domain packages
  still own request handling, logging, special result shaping, and GitLab API
  behavior.
- Do not add dynamic-only action definitions for normal GitLab operations.
  Ordinary actions belong in the owning `ActionSpec` builder and the canonical
  catalog path.
- Do not add broad allowlist entries for snapshot drift. If projection differs
  from the historical individual surface, first decide whether the spec or the
  historical snapshot is correct.
- Do not keep parallel descriptions, annotations, schemas, or icon choices in
  domain registration helpers once they can be represented by `ActionSpec`
  projection.
