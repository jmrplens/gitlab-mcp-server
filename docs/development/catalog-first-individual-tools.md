# Catalog-First Individual Tools Policy

> **Diátaxis type**: Explanation
> **Audience**: Developers changing tool registration surfaces
> **Prerequisites**: Familiarity with `RegisterTools`, meta-tools, and the canonical action catalog

The individual MCP tool surface is still registered by domain packages, but its
visible tool metadata is now catalog-first. Each normal GitLab API tool should
obtain its `mcp.Tool` definition from `toolutil.ActionSpec` projection while the
package keeps the handler closure, logging, not-found behavior, embedded
resources, rich results, and compatibility-specific runtime logic local.

This policy keeps the individual surface stable for existing clients while
removing duplicated metadata from hundreds of `RegisterTools` call sites.

## Current Policy

Individual tool registration has two layers:

| Layer | Owner | Policy |
| --- | --- | --- |
| Handler behavior | Domain `RegisterTools` function | Keep explicit closures for logging, request-aware handlers, not-found conversion, embedded resources, rich results, and compatibility behavior |
| Visible tool metadata | `ActionSpec` projection | Derive title, description, icons, input/output schemas, annotations, and compatibility metadata from specs |

Use `toolutil.MustIndividualToolFromSpecs` when registering normal individual
GitLab API tools. The helper fails fast when the named individual tool has no
matching spec or when spec metadata cannot produce a valid `mcp.Tool`.

Documented source-level exceptions are intentionally narrow:

| Package | Reason |
| --- | --- |
| `internal/tools/dynamic/register.go` | Registers the dynamic search/describe/execute surface generated from the canonical catalog, not individual GitLab API tools |
| `internal/tools/serverupdate/register.go` | Registers updater tools from `cmd/server` with `*autoupdate.Updater`, outside the GitLab client action catalog |

## Parity Checklist

When adding or changing an individual tool, verify each item before merging:

- The owning package exposes an `ActionSpecs` function for the action.
- The spec sets the exact historical individual tool name in `IndividualTool.Name`.
- The spec sets the individual title and any description override needed for the visible tool.
- The spec carries the same input and output schemas as the registered handler route.
- The spec annotations match the existing read-only, destructive, idempotent, and open-world semantics.
- The spec sets content, not-found, embedded-resource, rich-result, schema-validation, and runtime-validation policies when the action needs non-default behavior.
- `RegisterTools` uses `toolutil.MustIndividualToolFromSpecs` or an equivalent projection helper for the visible `mcp.Tool` metadata.
- Any direct `&mcp.Tool{...}` construction is either removed or added to the documented standalone exception list with a reason.
- Tool snapshots and ActionSpec guardrails pass without adding unexpected allowlist entries.

## Representative Patterns

### Standard Projected Tool

Most tools follow the same shape: define the action spec near the domain
handler, then use the projected tool metadata in `RegisterTools`.

```go
func projectTool(client *gitlabclient.Client, enterprise bool, name, description string) *mcp.Tool {
  return toolutil.MustIndividualToolFromSpecs(
    ActionSpecs(client, enterprise),
    name,
    toolutil.IndividualToolProjectionOptions{
      Description: description,
      Icons:       toolutil.IconProject,
    },
  )
}
```

The explicit description option preserves long-form compatibility text while
schemas, annotations, title, and shared metadata come from the matching spec.

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

| Guardrail | What it proves |
| --- | --- |
| `TestIndividualToolMetadata_SourceRegistrationUsesActionSpecProjection` | Source registration files do not reintroduce manual individual `mcp.Tool` metadata outside documented exceptions |
| `TestIndividualToolProjection_GoldenSnapshotParity` | Projected metadata matches `internal/tools/testdata/tools_individual.json` except explicit, reviewed gaps |
| `TestIndividualToolMetadata_CatalogBackedCoverage` | Every catalog-backed spec references a registered individual tool, and every individual tool without a spec is an explicit standalone exception |
| `cmd/audit_action_spec_coverage` | Every source domain is classified across individual, meta, dynamic, and standalone surfaces |
| `TestActionSpecCoverage_AllCatalogRoutesClassified` | Every GitLab.com Enterprise dynamic catalog route is spec-backed |

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
  `RegisterTools` once they can be represented by `ActionSpec` projection.
