---
name: create-mcp-tool
description: "Create a new MCP tool end-to-end: sub-package, input/output structs, handler, ActionSpec metadata, markdown formatter, tests, catalog projection, and documentation. Use when adding a new GitLab API endpoint as an MCP tool."
---

# Create MCP Tool — GitLab

Step-by-step workflow for creating a new MCP tool that wraps a GitLab REST/GraphQL API endpoint.

## Prerequisites

- Identify the GitLab API endpoint(s) (REST v4 or GraphQL)
- Confirm the `client-go` library supports the endpoint — if not, consider the `upstream-contribution` skill
- Decide the domain name (e.g., `tags`, `branches`, `pipelines`)

## File Structure

Create a new sub-package under `internal/tools/{domain}/`:

```text
{domain}/
├── doc.go              # Package comment (the one `// Package {domain} ...` comment)
├── {domain}.go         # Input/Output structs + handler logic
├── action_specs.go     # Canonical ActionSpec route metadata
├── markdown.go         # Markdown formatters + init() registry
├── shapes.go           # Optional: nested output shapes mirrored from client-go sub-objects
└── {domain}_test.go    # Tests with httptest (plus action_specs_test.go, markdown_test.go as needed)
```

A multi-word domain separates the words with underscores in its **file** names
only: `merge_requests.go` and `merge_requests_test.go` in `package mergerequests`.
Go's convention and the `stylecheck`/`revive` naming rules refuse underscores in
a package identifier, and the directory follows the package.

## Step 1: Define Input/Output Structs

In `{domain}.go`:

```go
package {domain}

import "github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"

type ListInput struct {
    toolutil.PaginationInput
    toolutil.KeysetPaginationInput // only when the endpoint supports keyset pagination
    ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

type Output struct {
    toolutil.HintableOutput
    ID     int    `json:"id"`
    Name   string `json:"name"`
    WebURL string `json:"web_url"`
}

type ListOutput struct {
    toolutil.HintableOutput
    Items      []Output                 `json:"items"`
    Pagination toolutil.PaginationOutput `json:"pagination"`
}
```

Rules:

- Embed `toolutil.HintableOutput` as first field (enables `next_steps` in JSON)
- Embed `toolutil.PaginationInput` for list operations, and `toolutil.KeysetPaginationInput` beside it when the GitLab endpoint supports keyset pagination
- Use `toolutil.StringOrInt` for project/group IDs
- Tag fields that only exist at a higher GitLab tier with `tier:"premium"` or `tier:"ultimate"`; the catalog prunes them from the schema below that tier
- Use `jsonschema:"description,required"` for required fields
- Use `json:",omitempty"` for optional fields
- No domain prefix on type names — the package provides namespace

## Step 2: Implement Handler Functions

In `{domain}.go`:

```go
import (
    "context"
    "errors"

    gl "gitlab.com/gitlab-org/api/client-go/v2"

    gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
    "github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
    if err := ctx.Err(); err != nil {
        return ListOutput{}, err
    }
    if input.ProjectID == "" {
        return ListOutput{}, errors.New("xxxList: project_id is required. Use gitlab_project_list to find the ID first")
    }
    opts := &gl.ListXxxOptions{}
    toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)

    items, resp, err := client.GL().Xxx.ListXxx(input.ProjectID.String(), opts, gl.WithContext(ctx))
    if err != nil {
        return ListOutput{}, toolutil.WrapErr("xxxList", err)
    }

    out := ListOutput{
        Items:      convertItems(items),
        Pagination: toolutil.PaginationFromResponse(resp),
    }
    return out, nil
}

func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
    if err := ctx.Err(); err != nil {
        return Output{}, err
    }
    if input.ProjectID == "" {
        return Output{}, errors.New("xxxCreate: project_id is required")
    }
    opts := &gl.CreateXxxOptions{
        Name: gl.Ptr(input.Name),
    }

    item, _, err := client.GL().Xxx.CreateXxx(input.ProjectID.String(), opts, gl.WithContext(ctx))
    if err != nil {
        switch {
        case toolutil.ContainsAny(err, "already exists"):
            return Output{}, toolutil.WrapErrWithHint("xxxCreate", err,
                "a resource with this name already exists")
        default:
            return Output{}, toolutil.WrapErrWithMessage("xxxCreate", err)
        }
    }

    return convertItem(item), nil
}
```

Error handling rules:

- `WrapErr(op, err)` — read-only operations only
- `WrapErrWithMessage(op, err)` — mutating operations (extracts GitLab error detail)
- `WrapErrWithHint(op, err, hint)` — when a recovery action is known
- `WrapErrWithStatusHint(op, err, code, hint)` — when the hint applies to one HTTP status only (`IsHTTPStatus` + `WrapErrWithHint` in one call)
- `NotFoundResult(resource, identifier, hints...)` — in get handlers on `IsHTTPStatus(err, 404)`: an informational `IsError` result logged at INFO, with `nil` error
- Validate required inputs before calling GitLab and check `ctx.Err()` first, as the real handlers do (`internal/tools/branches/branches.go`); the tests below expect the empty-`project_id` error

## Step 3: Add ActionSpecs

In `action_specs.go`, define the canonical route metadata once. Meta-tools, dynamic find/execute, `gitlab://tools` resources, audits, and individual tool projection consume this spec.

```go
package {domain}

import (
    gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
    "github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for {domain} actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
    return []toolutil.ActionSpec{
        // gitlab_{domain}_list: read-only, idempotent.
        toolutil.NewReadActionSpec("list", toolutil.RouteAction(client, List), listOptions()),
        // gitlab_{domain}_create: mutating, not idempotent.
        toolutil.NewCreateActionSpec("create", toolutil.RouteAction(client, Create), createOptions()),
        // gitlab_{domain}_delete: destructive, the route asks for confirmation before running.
        toolutil.NewDeleteActionSpec("delete", toolutil.DestructiveVoidAction(client, Delete), deleteOptions()),
    }
}

func listOptions() toolutil.ActionSpecOptions {
    return toolutil.ActionSpecOptions{
        Aliases: []string{"gitlab_{domain}_list", "list {resources}", "show {resources}"},
        Usage:   "List {resources} of a project. Use after locating the project with project.get, then {domain}.get for one item's details.",
        Tags:    []string{"{domain}"},
        ParameterGuidance: map[string]toolutil.ParameterGuidance{
            "project_id": {SemanticRole: "project", ValueSource: "Project that owns the {resources}."},
        },
        RelatedActions: []string{"{domain}.get", "{domain}.create", "project.get"},
        OpenWorld:      true,
        OwnerPackage:   "{domain}",
        IndividualTool: toolutil.IndividualToolSpec{
            Name:        "gitlab_{domain}_list",
            Title:       toolutil.TitleFromName("gitlab_{domain}_list"),
            Description: "List {resources} of a project. Returns: ... See also: gitlab_{domain}_get, gitlab_{domain}_create.",
        },
    }
}
```

`createOptions` and `deleteOptions` follow the same shape. A complete, current example of all four buckets is `internal/tools/issuelinks/action_specs.go`.

Spec rules:

- Pick the constructor that matches the operation: `NewReadActionSpec`, `NewCreateActionSpec`, `NewUpdateActionSpec`, `NewAdditiveActionSpec`, or `NewDeleteActionSpec`; each presets `ReadOnly`, `Destructive`, and `Idempotent`. A destructive action's route must be built with `toolutil.DestructiveAction` (returns an output) or `toolutil.DestructiveVoidAction` (returns only an error), which gate execution behind confirmation.
- Set `OwnerPackage` to the sub-package name.
- Set `IndividualTool` so `GITLAB_MCP_TOOL_SURFACE=individual` can project the visible per-action tool; the name is declared here, never derived, and new actions take the domain-first form (`gitlab_{domain}_{action}`).
- Add compatibility aliases and parameter aliases through the approved `actioncompat` policy when historical names must keep working.
- New domains must be added through the catalog aggregation/generation path, not by hand-adding root runtime registration calls.
- Set `Edition` (`free` / `premium` / `ultimate`; empty means `free`) on each spec. In `internal/tools/action_catalog.go`, `filterActionSpecGroupsByTier` withholds an action whose edition exceeds the tier resolved from `GITLAB_MCP_TIER` / `--tier`, and `pruneSchemaFieldsByTier` removes the `tier:"..."`-tagged fields from the schemas.
- Fill discovery metadata (`Aliases`, `Usage`, `ParameterGuidance`, `RelatedActions`) on every spec; `go run ./cmd/audit_discovery_completeness/` is the gate (`release.link_create_batch` is the gold standard).

## Step 4: Markdown Formatters

In `markdown.go`:

```go
package {domain}

import (
    "fmt"
    "strings"

    "github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// FormatOutputMarkdown renders a single {resource} as Markdown.
func FormatOutputMarkdown(out Output) string {
    if out.ID == 0 {
        return ""
    }
    var sb strings.Builder
    fmt.Fprintf(&sb, "## %s\n\n", out.Name)
    fmt.Fprintf(&sb, toolutil.FmtMdID, out.ID)
    fmt.Fprintf(&sb, "- **Name**: %s\n", out.Name)
    toolutil.WriteHints(&sb,
        "Use `gitlab_{domain}_update` to modify this resource",
        "Use `gitlab_{domain}_delete` to remove it",
    )
    return sb.String()
}

// FormatListMarkdown renders a list of {resources} as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
    if len(out.Items) == 0 {
        return "No {resources} found.\n"
    }
    var sb strings.Builder
    fmt.Fprintf(&sb, "## {Resources} (%d)\n\n", len(out.Items))
    sb.WriteString("| ID | Name |\n")
    sb.WriteString(toolutil.MarkdownTableSeparator(2))
    for _, item := range out.Items {
        fmt.Fprintf(&sb, "| %d | %s |\n", item.ID, toolutil.MdTitleLink(item.Name, item.WebURL))
    }
    toolutil.WriteHints(&sb,
        toolutil.HintPreserveLinks,
        "Use `gitlab_{domain}_get` with the ID for details",
    )
    return sb.String()
}

func init() {
    toolutil.RegisterMarkdown(FormatOutputMarkdown)
    toolutil.RegisterMarkdown(FormatListMarkdown)
}
```

Rules:

- Register all formatters in `init()` via `toolutil.RegisterMarkdown` (`RegisterMarkdownPair` / `RegisterMarkdownTriple` bundle several); `TestAllMarkdownFormattersRegistered` in `internal/tools` checks every output type has one
- `HintPreserveLinks` as first hint in list formatters with clickable links; `toolutil.MdTitleLink(title, url)` renders the link cell
- Markdown table separator rows come from `toolutil.MarkdownTableSeparator(columns)`; single-record fields use the `toolutil.FmtMd*` format constants (`FmtMdID`, ...)
- Empty state: always handle `len(items) == 0`

## Step 5: Wire Catalog Aggregation

For a new domain, add its `ActionSpecs(client)` builder to the audited catalog aggregation path used by `BuildActionCatalog`: in `internal/tools/action_specs.go`, append it inside the `build*ActionSpecs` function of the catalog group it belongs to (for example `buildIssueActionSpecs` appends `issuelinks.ActionSpecs(client)...` into the `gitlab_issue` group), or add a new group function and list it in `CollectActionSpecs`. Then regenerate the audited manifest with `make gen-action-catalog-manifest` (it rewrites `internal/tools/action_specs_manifest_gen.go`) and confirm it with `make check-action-catalog-manifest`.

Do not add package-local `RegisterTools` functions or package-level `RegisterMeta` calls for ordinary GitLab API actions. Root individual registration is catalog-backed through `RegisterIndividualCatalogTools`.

Expected checks:

- `make audit-catalog-first`
- `make check-action-catalog-manifest`
- `go test ./internal/tools -run 'TestActionSpecCoverage|TestRegisterAllDoesNotUseDomainRegisterTools|TestAllMarkdownFormattersRegistered' -count=1`
- `go test ./internal/tools/{domain}/ -count=1`

## Step 6: Write Tests

In `{domain}_test.go`:

```go
package {domain}

import (
    "context"
    "net/http"
    "strings"
    "testing"

    "github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

// TestList_Success verifies that List returns the items the mocked
// GET /api/v4/projects/42/{endpoint} endpoint serves.
func TestList_Success(t *testing.T) {
    client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/{endpoint}" {
            testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"name":"item1"}]`)
            return
        }
        http.NotFound(w, r)
    }))

    out, err := List(context.Background(), client, ListInput{
        ProjectID: "42",
    })
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if len(out.Items) != 1 {
        t.Errorf("got %d items, want 1", len(out.Items))
    }
    if out.Items[0].Name != "item1" {
        t.Errorf("Name = %q, want %q", out.Items[0].Name, "item1")
    }
}

func TestList_EmptyProjectID(t *testing.T) {
    client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        http.NotFound(w, r)
    }))

    _, err := List(context.Background(), client, ListInput{})
    if err == nil {
        t.Fatal("expected error for empty project ID")
    }
}

func TestCreate_APIError(t *testing.T) {
    client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
    }))

    _, err := Create(context.Background(), client, CreateInput{
        ProjectID: "42",
        Name:      "test",
    })
    if err == nil {
        t.Fatal("expected error for 403")
    }
}

func TestFormatListMarkdown_Empty(t *testing.T) {
    md := FormatListMarkdown(ListOutput{})
    if !strings.Contains(md, "No items found") {
        t.Error("empty list should show 'No items found'")
    }
}
```

Test categories (all required):

- `Test{Tool}_Success` — happy path
- `Test{Tool}_EmptyProjectID` — input validation
- `Test{Tool}_APIError` — error classification
- `TestFormat{X}Markdown_*` — markdown output
- `TestFormat{X}Markdown_Empty` — empty state

Test rules that CI gates:

- Name tests `TestToolName_Scenario_ExpectedResult`; every test function carries a doc comment
- A case table (`for _, tt := range tests`) must run each case under `t.Run` (`make check-test-subtests`; `go run ./cmd/audit_test_subtests/ -fix` rewrites the unambiguous shapes)
- Never `t.Fatal`/`t.Fatalf`/`t.FailNow` inside an `httptest` handler or any other goroutine: `t.Errorf` + a deterministic response + `return`, or record with atomics and assert afterwards (`.github/instructions/test-goroutines.instructions.md`; `make check-test-goroutines`). `testutil.AssertRequestPath` / `AssertRequestMethod` / `AssertQueryParam` and `testutil.ForbiddenHandler` already follow the contract
- A `_test.go` file is named after the module it tests (`make check-test-file-names`)

## Step 7: Update Documentation

1. Add the tools to the page under `docs/reference/tools/` that owns the domain (`docs/reference/tools/doc-ownership.json` maps tool-name prefixes to pages, and `go run ./cmd/audit_doc_coverage/` is the gate); create a new page and an ownership entry only for a new area
2. The catalog tables in `docs/reference/tools/README.md` are generator-owned; do not hand-edit them
3. At the end of the tool implementation phase, run `go run ./cmd/gen_testing_docs/` to refresh `docs/development/testing/testing.md` with new test counts and coverage values

## Step 8: Verify

```bash
go test ./internal/tools/{domain}/ -count=1 -v
go run ./cmd/gen_testing_docs/ --check
npx markdownlint-cli2 docs/development/testing/testing.md
golangci-lint run --build-tags e2e ./internal/tools/{domain}/
make check-test-subtests check-test-goroutines check-test-file-names
go run ./cmd/audit_doc_coverage/
```

## Validation Checklist

- [ ] Sub-package created with `doc.go`, the handler file, `action_specs.go`, `markdown.go`, and the test file
- [ ] Input structs use `jsonschema` tags with descriptions
- [ ] Output structs embed `toolutil.HintableOutput`
- [ ] Correct `New*ActionSpec` constructor and route helper per operation type
- [ ] Markdown formatters registered in `init()`
- [ ] Empty state handled in list formatters
- [ ] `HintPreserveLinks` in list formatters with links
- [ ] Error handling uses correct WrapErr variant
- [ ] Added to ActionSpec/catalog aggregation and covered by `make audit-catalog-first`
- [ ] Tests cover success, validation, API error, and markdown
- [ ] `go test` + `golangci-lint` pass
- [ ] Documentation updated
