# Catalog-First Individual Tools Evaluation

> **Diátaxis type**: Explanation
> **Audience**: Developers changing tool registration surfaces
> **Prerequisites**: Familiarity with `RegisterTools`, meta-tools, and the canonical action catalog

This note evaluates whether the individual MCP tool surface should be generated
from the canonical action catalog. The conclusion is conservative: keep
individual tools hand-registered for now, and only revisit generation after the
catalog carries the individual-surface metadata that currently lives in domain
`RegisterTools` functions.

## Decision

Do not generate the production individual surface from the canonical action
catalog yet.

The catalog is already the correct source for meta-tools and dynamic tools, but
`ActionRoute` intentionally stores only executable route metadata: handler,
destructive flag, input schema, and output schema. Individual tools also need a
stable visible tool name, per-tool description, MCP annotations, content
annotations, not-found handling, embedded resources, and result variants such as
image content. Those details are still expressed in `RegisterTools` closures.

Runtime generation is rejected for now because it would make the compatibility
surface depend on incomplete catalog metadata. Build-time generation is also
deferred until a separate design extends catalog metadata and snapshot tests can
prove exact parity with the current individual tools.

## Representative Comparison

| Individual tool | Catalog action | Parity | Gaps |
| --- | --- | --- | --- |
| `gitlab_project_get` | `project.get` | Reuses `projects.Get`, input/output structs, and project markdown formatter. | The individual tool converts 404 responses to `NotFoundResult`, logs them at info level, embeds `gitlab://project/{id}`, and uses a focused per-tool description. |
| `gitlab_issue_create` | `issue.create` | Reuses `issues.Create`, schemas, markdown, and runtime hints. | Mostly compatible. The individual tool still owns the visible tool name, description, and content annotation. |
| `gitlab_package_publish` | `package.publish` | Catalog route uses `routeActionWithRequest`, so request-aware progress and local file handling are available. | The schema cannot express the `file_path` or `content_base64` exclusive choice, so runtime validation remains required. The individual description includes release-link guidance not modeled per action. |
| `gitlab_pipeline_schedule_create` | `pipeline.schedule_create` | Straightforward handler, schemas, annotations, markdown, and hints. | Mostly compatible. This is a good future parity candidate once individual metadata exists in the catalog. |
| `gitlab_file_get` | `repository.file_get` | Reuses `files.Get`, schemas, and repository file markdown logic. | The individual tool name does not mechanically match the catalog action ID. It also converts 404 responses to `NotFoundResult` and may return image content through `ToolResultWithImage`. |

The representative set shows that typed business logic is already shared well.
The remaining duplication is primarily MCP registration policy and user-facing
metadata, not GitLab API logic.

## Blockers

### Missing Individual Tool Metadata

The catalog action model does not store per-action individual tool metadata.
Generating individual tools would require at least:

- Visible tool name, for example `gitlab_file_get` for catalog action `repository.file_get`.
- Per-tool title and description, not only the meta-tool group description.
- MCP tool annotations such as read, create, update, and delete semantics.
- Content annotation to distinguish list, detail, mutate, assistant, and image responses.
- Icons when an individual action should differ from its group icon.
- See-also guidance and action-specific usage warnings.

Without this metadata, a generated individual tool can call the right handler
but cannot preserve the current compatibility contract.

### Not-Found Results And Logging

Many get tools intercept GitLab 404 responses in the `RegisterTools` closure,
log the call as a non-error, and return `toolutil.NotFoundResult` with
actionable hints. This behavior is intentionally different from propagating the
handler error.

Examples include projects, files, groups, branches, tags, labels, milestones,
releases, users, badges, deployments, release links, issue links, award emoji,
and draft notes. A catalog-generated individual surface would need explicit
not-found policy metadata or a wrapper that can identify the resource name,
identifier, and hints for each action.

### Embedded Resources And Rich Results

Several individual tools attach embedded MCP resources after successful reads.
`gitlab_project_get` embeds `gitlab://project/{id}`. Other domains embed branch,
tag, label, milestone, group, deployment, and release resources.

File tools add another special case: `gitlab_file_get` can return image content
when the repository file is an image. A generic action wrapper cannot infer that
from `ActionRoute` alone.

### Naming Is Not Always Mechanical

Most dynamic action IDs follow `domain.action`, but individual tool names are
historical compatibility names. The repository file action is the clearest
example: the catalog exposes `repository.file_get`, while the individual tool is
`gitlab_file_get`. Any generator would need an explicit name map rather than a
pure string transform.

### Runtime Validation Still Matters

Some constraints are intentionally enforced by handlers instead of JSON Schema.
Package publish requires exactly one content source, `file_path` or
`content_base64`. That does not block catalog-backed execution, but generated
tools must preserve handler validation and not imply the schema is complete.

## Recommendation

Keep individual tools as a separate hand-registered compatibility surface.
Continue using the canonical action catalog for meta and dynamic surfaces.

If this topic is revisited, the next design should first add an explicit
individual-tool metadata layer to the catalog or to generated source data. That
metadata must include tool names, descriptions, annotations, content policy,
not-found policy, embedded-resource policy, and rich-result policy. Only after
that should a build-time generator or test-only prototype compare generated
tool definitions against `internal/tools/testdata/tools_individual.json`.

Runtime generation should remain off the table until snapshot parity and MCP
round-trip parity are proven. The individual surface is the compatibility API;
it should not inherit incomplete metadata from a catalog optimized for dispatch.
