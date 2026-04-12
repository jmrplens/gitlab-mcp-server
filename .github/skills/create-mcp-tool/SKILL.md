---
name: create-mcp-tool
description: "Create a new MCP tool end-to-end: sub-package, input/output structs, handler, markdown formatter, tests, registration, and documentation. Use when adding a new GitLab API endpoint as an MCP tool."
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
├── register.go         # RegisterTools() + RegisterMeta()
├── {domain}.go         # Input/Output structs + handler logic
├── markdown.go         # Markdown formatters + init() registry
└── {domain}_test.go    # Table-driven tests with httptest
```

## Step 1: Define Input/Output Structs

In `{domain}.go`:

```go
package {domain}

import "github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

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

## Step 3: Register Tools

In `register.go`:

```go
package {domain}

import (
    "context"
    "time"

    "github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
    gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
    "github.com/modelcontextprotocol/go-sdk/mcp"
)

func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
    mcp.AddTool(server, &mcp.Tool{
        Name:        "gitlab_{domain}_list",
        Title:       toolutil.TitleFromName("gitlab_{domain}_list"),
        Description: "List {resources} in a project. Returns: ID, name, ...\n\nSee also: gitlab_{domain}_get, gitlab_{domain}_create",
        Annotations: toolutil.ReadAnnotations,
        Icons:       toolutil.Icon{Domain},
    }, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
        start := time.Now()
        out, err := List(ctx, client, input)
        toolutil.LogToolCallAll(ctx, req, "gitlab_{domain}_list", start, err)
        return toolutil.WithHints(FormatListMarkdown(out), out, err)
    })

    mcp.AddTool(server, &mcp.Tool{
        Name:        "gitlab_{domain}_create",
        Title:       toolutil.TitleFromName("gitlab_{domain}_create"),
        Description: "Create a {resource}. Returns: created resource details.\n\nSee also: gitlab_{domain}_list, gitlab_{domain}_get",
        Annotations: toolutil.CreateAnnotations,
        Icons:       toolutil.Icon{Domain},
    }, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
        start := time.Now()
        out, err := Create(ctx, client, input)
        toolutil.LogToolCallAll(ctx, req, "gitlab_{domain}_create", start, err)
        return toolutil.WithHints(FormatOutputMarkdown(out), out, err)
    })
}

func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
    // Register meta-tool if domain uses inline meta-tool pattern
}
```

Annotation presets:

- `ReadAnnotations` — GET/list/search (read-only, idempotent)
- `CreateAnnotations` — POST/create
- `UpdateAnnotations` — PUT/update (idempotent)
- `DeleteAnnotations` — DELETE (destructive, idempotent)

## Step 4: Markdown Formatters

In `markdown.go`:

```go
package {domain}

import (
    "fmt"
    "strings"

    "github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
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

## Step 5: Wire Registration

In `internal/tools/register.go`, add the import and call:

```go
import "{domain}" "{module}/internal/tools/{domain}"
// ...
{domain}.RegisterTools(server, client)
```

In `internal/tools/register_meta.go` (if meta-tool):

```go
{domain}.RegisterMeta(server, client)
```

## Step 6: Write Tests

In `{domain}_test.go`:

```go
package {domain}

import (
    "context"
    "net/http"
    "testing"

    "github.com/jmrplens/gitlab-mcp-server/internal/testutil"
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

1. Add entry to `docs/tools/{domain}.md`
2. Update `docs/tools/README.md` tool count
3. Update `docs/development/testing.md` with new test counts

## Step 8: Verify

```bash
go vet ./internal/tools/{domain}/
go test ./internal/tools/{domain}/ -count=1 -v
golangci-lint run ./internal/tools/{domain}/
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
- [ ] Wired in `register.go` and `register_meta.go`
- [ ] Tests cover success, validation, API error, and markdown
- [ ] `go vet` + `go test` + `golangci-lint` pass
- [ ] Documentation updated
