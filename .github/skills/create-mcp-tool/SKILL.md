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
├── {domain}.go         # Input/Output structs + handler logic
├── action_specs.go     # Canonical ActionSpec route metadata
├── markdown.go         # Markdown formatters + init() registry
└── {domain}_test.go    # Table-driven tests with httptest
```

## Step 1: Define Input/Output Structs

In `{domain}.go`:

```go
package {domain}

import "github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"

type ListInput struct {
    toolutil.PaginationInput
    ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

type Output struct {
    toolutil.HintableOutput
    ID   int    `json:"id"`
    Name string `json:"name"`
}

type ListOutput struct {
    toolutil.HintableOutput
    Items      []Output                 `json:"items"`
    Pagination toolutil.PaginationOutput `json:"pagination"`
}
```

Rules:

- Embed `toolutil.HintableOutput` as first field (enables `next_steps` in JSON)
- Embed `toolutil.PaginationInput` for list operations
- Use `toolutil.StringOrInt` for project/group IDs
- Use `jsonschema:"description,required"` for required fields
- Use `json:",omitempty"` for optional fields
- No domain prefix on type names — the package provides namespace

## Step 2: Implement Handler Functions

In `{domain}.go`:

```go
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
    opts := &gl.ListXxxOptions{
        ListOptions: gl.ListOptions{
            Page:    input.Page(),
            PerPage: input.PerPage(),
        },
    }

    items, resp, err := client.GL().Xxx.ListXxx(input.ProjectID.String(), opts, gl.WithContext(ctx))
    if err != nil {
        return ListOutput{}, toolutil.WrapErrWithMessage("xxxList", err)
    }

    out := ListOutput{
        Items:      convertItems(items),
        Pagination: toolutil.BuildPagination(resp),
    }
    return out, nil
}

func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
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
        toolutil.NewActionSpec("list", toolutil.RouteAction(client, List), actionOptions("gitlab_{domain}_list", true)),
        toolutil.NewActionSpec("create", toolutil.RouteAction(client, Create), actionOptions("gitlab_{domain}_create", false)),
    }
}

func actionOptions(individualTool string, readOnly bool) toolutil.ActionSpecOptions {
    return toolutil.ActionSpecOptions{
        ReadOnly:       readOnly,
        Idempotent:     readOnly,
        Tags:           []string{"{domain}"},
        OwnerPackage:   "{domain}",
        IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
    }
}
```

Spec rules:

- Set `ReadOnly`, `Destructive`, and `Idempotent` accurately; destructive actions must use destructive route helpers or options.
- Set `OwnerPackage` to the sub-package name.
- Set `IndividualTool` so `TOOL_SURFACE=individual` can project the visible per-action tool.
- Add compatibility aliases and parameter aliases through the approved `actioncompat` policy when historical names must keep working.
- New domains must be added through the catalog aggregation/generation path, not by hand-adding root runtime registration calls.

## Step 4: Markdown Formatters

In `markdown.go`:

```go
package {domain}

import (
    "fmt"
    "strings"

    "github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

func init() {
    toolutil.RegisterMarkdown(FormatOutputMarkdownString)
    toolutil.RegisterMarkdown(FormatListMarkdownString)
}

func FormatOutputMarkdownString(out Output) string {
    return FormatOutputMarkdown(out)
}

func FormatOutputMarkdown(out Output) string {
    var sb strings.Builder
    fmt.Fprintf(&sb, "# %s\n\n", out.Name)
    fmt.Fprintf(&sb, "| Field | Value |\n")
    sb.WriteString(toolutil.TableSep2 + "\n")
    fmt.Fprintf(&sb, "| ID | %d |\n", out.ID)
    fmt.Fprintf(&sb, "| Name | %s |\n", out.Name)
    toolutil.WriteHints(&sb,
        "Use gitlab_{domain}_update to modify this resource",
        "Use gitlab_{domain}_delete to remove it",
    )
    return sb.String()
}

func FormatListMarkdownString(out ListOutput) string {
    return FormatListMarkdown(out)
}

func FormatListMarkdown(out ListOutput) string {
    var sb strings.Builder
    sb.WriteString("# {Resources}\n\n")
    if len(out.Items) == 0 {
        sb.WriteString("No items found.\n")
        return sb.String()
    }
    sb.WriteString("| ID | Name |\n")
    sb.WriteString(toolutil.TableSep2 + "\n")
    for _, item := range out.Items {
        fmt.Fprintf(&sb, "| %d | %s |\n", item.ID, item.Name)
    }
    toolutil.WriteHints(&sb,
        toolutil.HintPreserveLinks,
        "Use gitlab_{domain}_get with ID for details",
    )
    return sb.String()
}
```

Rules:

- Register all formatters in `init()` via `toolutil.RegisterMarkdown`
- `HintPreserveLinks` as first hint in list formatters with clickable links
- Markdown tables use `toolutil.TableSep2`, `TableSep3`, etc.
- Empty state: always handle `len(items) == 0`

## Step 5: Wire Catalog Aggregation

For a new domain, add its `ActionSpecs(client)` builder to the audited catalog aggregation path used by `BuildActionCatalog`.

Do not add package-local `RegisterTools` functions or package-level `RegisterMeta` calls for ordinary GitLab API actions. Root individual registration is catalog-backed through `RegisterIndividualCatalogTools`.

Expected checks:

- `make audit-action-spec-coverage`
- `go test ./internal/tools -run 'TestActionSpecCoverage|TestRegisterAllDoesNotUseDomainRegisterTools' -count=1`
- `go test ./internal/tools/{domain}/ -count=1`

## Step 6: Write Tests

In `{domain}_test.go`:

```go
package {domain}

import (
    "context"
    "net/http"
    "testing"

    "github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

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
        w.WriteHeader(http.StatusForbidden)
        w.Write([]byte(`{"message":"403 Forbidden"}`))
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

## Step 7: Update Documentation

1. Add or update `docs/tools/{domain}.md`
2. Update `docs/tools/README.md` only when the domain index changes
3. At the end of the tool implementation phase, run `go run ./cmd/gen_testing_docs/` to refresh `docs/testing/testing.md` with new test counts and coverage values

## Step 8: Verify

```bash
go test ./internal/tools/{domain}/ -count=1 -v
go run ./cmd/gen_testing_docs/ --check
npx markdownlint-cli2 docs/testing/testing.md
golangci-lint run --build-tags e2e ./internal/tools/{domain}/
```

## Validation Checklist

- [ ] Sub-package created with all 4 files
- [ ] Input structs use `jsonschema` tags with descriptions
- [ ] Output structs embed `toolutil.HintableOutput`
- [ ] Correct annotation preset per operation type
- [ ] Markdown formatters registered in `init()`
- [ ] Empty state handled in list formatters
- [ ] `HintPreserveLinks` in list formatters with links
- [ ] Error handling uses correct WrapErr variant
- [ ] Added to ActionSpec/catalog aggregation and covered by `make audit-action-spec-coverage`
- [ ] Tests cover success, validation, API error, and markdown
- [ ] `go test` + `golangci-lint` pass
- [ ] Documentation updated
