---
name: "go-source-documentation"
description: "Add idiomatic, godoc-compliant documentation to Go source and test files. Use when user asks to document Go files, add doc comments, improve Go documentation, or mentions /document-go. Covers file headers, package comments, types, functions, interfaces, constants, variables, tests (with detailed explanations), benchmarks, fuzz tests, and examples."
---

# Go Source Documentation Skill

## Activation

Activate this skill when the user:

- Asks to document Go source files or test files
- Asks to add doc comments to Go code
- Asks to improve or fix Go documentation
- Mentions `/document-go` or `/doc-go`
- Asks to add file headers to Go files
- Asks to document a Go package

## Prerequisites

Before starting, gather context:

1. **Read the target file(s)** completely to understand structure and purpose
2. **Read related files** in the same package to understand the package API surface
3. **Check existing tests** to understand what behavior the code implements
4. **Identify the package role** within the project architecture
5. **Use Context7** to verify current Go doc comment conventions if uncertain

## Documentation Patterns

### Pattern 1: Package Comment

Every package needs exactly one `Package` comment, typically in the main file or a dedicated `doc.go`:

```go
// Package tools implements MCP tool handlers for GitLab operations.
//
// Each tool follows a consistent pattern: a typed input struct with jsonschema
// tags defining the parameter schema, a handler function that calls the GitLab
// API, and a typed output struct for the response.
//
// Tools are registered via [RegisterTools] which wires each handler into the
// MCP server with appropriate tool annotations (readOnlyHint, destructiveHint).
//
// # Supported Operations
//
//   - Branches: create, list, protect, unprotect
//   - Issues: create, list, get, update, search
//   - Merge Requests: create, list, get, update, merge
//   - Files: get content, create, update
//   - Commits: list, get details
package tools
```

### Pattern 2: File Header Comment

For multi-file packages, each file gets a header describing its scope. The header goes immediately before the `package` declaration:

```go
// branches.go implements MCP tool handlers for GitLab branch operations.
//
// It provides the following tools:
//   - gitlab_branch_create:     Create a new branch from a ref
//   - gitlab_branch_list:       List branches with optional search
//   - gitlab_branch_protect:    Protect a branch with access levels
//   - gitlab_branch_unprotect:  Remove protection from a branch
package tools
```

### Pattern 3: Input Struct Documentation

Input structs define MCP tool parameters. Document the struct purpose and each field's role:

```go
// MRCreateInput defines parameters for creating a new merge request.
// ProjectID identifies the target project, SourceBranch and TargetBranch
// define the merge direction, and Title becomes the MR title.
type MRCreateInput struct {
    ProjectID    string `json:"project_id"    jsonschema:"required,description=Project ID or URL-encoded path"`
    SourceBranch string `json:"source_branch" jsonschema:"required,description=Source branch name"`
    TargetBranch string `json:"target_branch" jsonschema:"required,description=Target branch name"`
    Title        string `json:"title"         jsonschema:"required,description=Title of the merge request"`
    Description  string `json:"description"   jsonschema:"description=Detailed description (Markdown supported)"`
}
```

### Pattern 4: Handler Function Documentation

Handler functions follow the pattern `func toolName(ctx, client, input) (output, error)`:

```go
// mrCreate creates a new merge request in the specified GitLab project.
// It sets the title, description, source branch, and target branch from
// the input parameters. Returns the created merge request details or an
// error if the project is not found, the source branch does not exist,
// or the GitLab API call fails.
func mrCreate(ctx context.Context, client *gitlabclient.Client, in MRCreateInput) (MROutput, error) {
```

### Pattern 5: Output Struct Documentation

Output structs define the MCP tool response:

```go
// MROutput represents the response from a merge request operation.
// It contains the essential fields returned by the GitLab Merge Requests API.
type MROutput struct {
    IID          int    `json:"iid"`
    Title        string `json:"title"`
    State        string `json:"state"`
    WebURL       string `json:"web_url"`
    SourceBranch string `json:"source_branch"`
    TargetBranch string `json:"target_branch"`
    Author       string `json:"author"`
}
```

### Pattern 6: Converter/Helper Functions

Internal helpers that transform data between API and MCP formats:

```go
// mrToOutput converts a GitLab API [gl.MergeRequest] to the MCP tool
// output format. It extracts the fields relevant for MCP consumers and
// builds the author display name from the GitLab user record.
func mrToOutput(mr *gl.MergeRequest) MROutput {
```

### Pattern 7: Registration Functions

Functions that wire tools into the MCP server:

```go
// RegisterBranchTools registers all branch-related MCP tools on the given
// server. Each tool is configured with appropriate annotations indicating
// whether the operation is read-only or destructive.
func RegisterBranchTools(srv *server.MCPServer, client *gitlabclient.Client) {
```

### Pattern 8: Test File Documentation

```go
// branches_test.go contains unit tests for the branch MCP tool handlers
// defined in branches.go. Tests use httptest to mock GitLab API responses
// and verify both success paths and error conditions.
//
// Each test function creates a dedicated httptest server with a handler
// that simulates the relevant GitLab API endpoint, then calls the tool
// handler directly and asserts the output or error.
package tools
```

### Pattern 9: Individual Test Function Documentation

Every test MUST have a detailed doc comment explaining:

1. **What**: The specific function, behavior, or scenario being tested
2. **How**: The test setup (mock configuration, inputs, preconditions)
3. **Expected**: The specific assertions and expected outcomes
4. **Why**: The business rule or edge case this test protects

```go
// TestBranchCreate_Success verifies that branchCreate creates a branch
// when the GitLab API returns HTTP 201 Created.
//
// The test mocks POST /projects/:id/repository/branches to return a
// branch object with name "feature/login" and a known commit SHA. It
// asserts that no error is returned, the output branch name matches
// "feature/login", and the web URL is correctly constructed.
func TestBranchCreate_Success(t *testing.T) {
```

Error scenario:

```go
// TestBranchCreate_ProjectNotFound verifies that branchCreate returns an
// error when the target project does not exist in GitLab.
//
// The mock returns HTTP 404 with a GitLab error body. The test asserts
// that an error is returned and the error message contains "404". This
// protects against silent failures when operating on deleted projects.
func TestBranchCreate_ProjectNotFound(t *testing.T) {
```

### Pattern 10: Table-Driven Test Documentation

Document the overall strategy AND list all covered scenarios:

```go
// TestBranchList_Scenarios uses table-driven subtests to validate branchList
// across multiple conditions:
//
//  - "returns branches with pagination": successful listing with 2 branches,
//    verifies item count, names, and pagination metadata
//  - "returns empty list": HTTP 200 with empty array, verifies non-nil empty
//    slice
//  - "returns error on 404": HTTP 404, verifies error propagation
//  - "returns error on 500": HTTP 500, verifies error propagation
//  - "respects search filter": verifies the search query param is forwarded
//
// Each subtest configures a dedicated httptest handler that returns the
// appropriate response for the scenario being tested.
func TestBranchList_Scenarios(t *testing.T) {
```

### Pattern 11: Test Helper Documentation

```go
// newTestGitLabClient creates a [gitlabclient.Client] connected to an
// httptest server using the provided handler. The test server is
// automatically stopped when the test completes via [testing.T.Cleanup].
func newTestGitLabClient(t *testing.T, handler http.Handler) *gitlabclient.Client {
    t.Helper()
```

### Pattern 12: Interface Documentation

Document the contract, not the implementation. List the method set and explain the
behavioral expectations:

```go
// ToolRegistrar registers MCP tools and meta-tools with the server.
// Implementations must be safe for concurrent use from multiple goroutines.
//
// RegisterTools registers individual, fine-grained tools.
// RegisterMeta registers aggregated meta-tools that dispatch to individual tools.
type ToolRegistrar interface {
    RegisterTools(server *mcp.Server, client *gitlabclient.Client)
    RegisterMeta(server *mcp.Server, client *gitlabclient.Client)
}
```

### Pattern 13: Deprecation Notices

Use the standard `// Deprecated:` directive (Go 1.19+) on its own paragraph:

```go
// FormatResponse formats an API response into a human-readable string.
//
// Deprecated: Use [FormatMarkdown] instead, which produces richer output
// with proper heading levels and code blocks.
func FormatResponse(data any) string {
```

### Pattern 14: Benchmark and Fuzz Test Documentation

Benchmark tests document the operation being measured and any special setup:

```go
// BenchmarkBranchList measures the throughput of branchList with a
// mock returning 100 branches per page. The benchmark uses b.ResetTimer
// after httptest setup to exclude initialization from measurements.
func BenchmarkBranchList(b *testing.B) {
```

Fuzz tests document the invariant being checked and the seed corpus:

```go
// FuzzParseProjectID verifies that parseProjectID never panics for
// arbitrary string inputs. The seed corpus includes empty strings,
// numeric IDs, namespaced paths, and URL-encoded paths.
func FuzzParseProjectID(f *testing.F) {
```

### Pattern 15: Example Function Documentation

Example functions appear in godoc under the symbol they demonstrate:

```go
// ExampleNewClient demonstrates creating a GitLab client with a
// personal access token and custom base URL.
func ExampleNewClient() {
```

### Pattern 16: BUG and TODO Annotations

Use `// BUG(who):` at package level for known bugs that appear in godoc.
Use `// TODO:` with a ticket reference for planned work (does NOT appear in godoc):

```go
// BUG(jmrplens): ListBranches does not handle pagination for projects
// with more than 10,000 branches due to a GitLab API limitation.

// TODO(TICKET-123): Add retry logic for transient 502 errors.
```

## Decision Framework

For each symbol, decide the documentation level:

| Symbol Type     | Exported? | Doc Required?         | Level of Detail              |
| --------------- | --------- | --------------------- | ---------------------------- |
| Package         | —         | YES (one per package) | Purpose, scope, key types    |
| Type/Struct     | Yes       | YES                   | What instances represent     |
| Type/Struct     | No        | If non-obvious        | Brief purpose                |
| Interface       | Yes       | YES                   | Contract, behavioral expectations, concurrency |
| Function        | Yes       | YES                   | What it does, params, errors |
| Function        | No        | If non-obvious        | Brief purpose                |
| Method          | Yes       | YES                   | Start with receiver context  |
| Const/Var group | Yes       | YES                   | Group purpose                |
| Const/Var group | No        | If non-obvious        | Brief purpose                |
| Test function   | —         | YES                   | What/How/Expected/Why        |
| Test helper     | —         | YES                   | What it creates/configures   |
| Benchmark       | —         | YES                   | Operation measured, setup    |
| Fuzz test       | —         | YES                   | Invariant, seed corpus       |
| Example func    | —         | YES                   | What it demonstrates         |
| Deprecation     | —         | YES                   | `Deprecated:` + replacement  |

## Validation Steps

After documenting each file:

1. **Syntax check**: `go vet ./path/to/package/...`
2. **Build check**: `go build ./path/to/package/...`
3. **Test check**: `go test ./path/to/package/...`
4. **Doc check**: `go doc ./path/to/package` — verify all exported symbols appear
5. **No logic changes**: Confirm only comments were added/modified

## Common Mistakes to Avoid

1. **Changing code logic** — NEVER modify function bodies, signatures, or variable names
2. **Blank line between comment and declaration** — renders as regular comment, not doc comment:

   ```go
   // WRONG — blank line breaks the association
   // Package branches handles branch operations.

   package branches
   ```

3. **Not starting with symbol name** — `go doc` synopsis will be wrong
4. **Using block comments for doc comments** — use `//` line comments (block `/* */` style is non-idiomatic for doc comments)
5. **Redundant comments** — don't restate what the code already says clearly
6. **Missing error documentation** — always document when/why errors are returned
7. **Forgetting test docs** — test functions MUST have detailed documentation
8. **Inconsistent tense** — use present tense ("creates", "returns", not "will create")
9. **Missing package comment** — one file per package must have `// Package name ...`
10. **Over-documenting** — a clear name like `UserID string` doesn't need a comment saying "the user's ID"
11. **Headings without blank line before** — Go 1.19+ headings (`# Heading`) must have a blank `//` line before them
12. **Doc links to unexported symbols** — `[unexportedFunc]` won't resolve; only link to exported symbols

## Project-Specific Notes

This project (gitlab-mcp-server) has specific patterns to recognize:

- **MCP tool input structs** have `jsonschema` tags — mention the tool parameters they define
- **Handler functions** follow `func name(ctx, client, input) (output, error)` — document the API operation
- **Registration functions** use `mcp.AddTool()` — document which tools are registered
- **Tests use `httptest`** — mention the API endpoint being mocked and HTTP method
- **Constants like endpoint paths** — document they are test fixtures for specific API routes
- **`gitlabclient.Client`** wraps the GitLab API — reference it as `[gitlabclient.Client]`
- **`toolutil` helpers** — reference using doc links: `[toolutil.WrapErr]`, `[toolutil.BuildPaginationResponse]`
- **Meta-tool registration** — `RegisterMeta()` functions register domain-level dispatch tools
