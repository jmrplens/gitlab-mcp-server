---
name: go-safe-move-refactor
description: 'Safely move Go source files between packages with zero compilation downtime. Handles package declarations, import updates, symbol exports, type renames, test migration, and forwarding stubs. Validates compilation after every atomic step.'
---

# Go Safe Move Refactor

## Primary Directive

Move one or more Go source files from one package to another while maintaining compilation at every intermediate step. This skill implements the **"bridge pattern"** — keep old code working via forwarding stubs while new code is established, then remove bridges after verification.

## When to Use

- Moving a `.go` file from one package to another
- Extracting a set of functions/types into a new package
- Consolidating multiple files into a single sub-package
- Splitting a large package into smaller ones

## Core Principle: Never Break Compilation

Every change must be an atomic step that leaves `go build ./...` passing. The sequence is:

```text
1. Create destination → 2. Copy & transform → 3. Add forwarding stubs → 4. Verify → 5. Update consumers → 6. Remove stubs → 7. Verify
```

## Process

### Step 1: Pre-Flight Check

```bash
go build ./...    # Must pass
go vet ./...      # Must pass
go test ./...     # Record baseline (should pass)
```

Record:

- Source file path: `${srcFile}` (e.g., `internal/tools/branches.go`)
- Source package: `${srcPkg}` (e.g., `tools`)
- Destination directory: `${dstDir}` (e.g., `internal/tools/branches/`)
- Destination package: `${dstPkg}` (e.g., `branches`)

**Discovery check** (before any move): The **client-go API** defines the canonical domain structure. Verify you have the complete picture:

1. **Inspect client-go types**: Run `go doc gitlab.com/gitlab-org/api/client-go/v2.{Type}` for the domain's key types to understand the canonical fields and API contract
2. List all non-test handler files in the source package to find everything that exists
3. Check `register.go` and `register_meta.go` for the domain's registration functions
4. Look for related files (e.g., a domain might span `{domain}.go` + `{domain}_extra.go`)
5. If `docs/api-mapping/{domain}.md` exists, read it for supplementary field mapping context — but do NOT skip the move if no doc exists

### Step 2: Analyze Dependencies

Before moving, catalog ALL symbols in the source file:

| Symbol | Type | Visibility | Used By |
|--------|------|-----------|---------|
| `BranchCreateInput` | struct | exported | register.go, branches_test.go |
| `BranchOutput` | struct | exported | register.go, branches_test.go, markdown.go |
| `branchCreate` | func | unexported | register.go |
| `branchList` | func | unexported | register.go |

Use `grep` and `go doc` to find all references:

```bash
grep -rn "BranchCreateInput\|BranchOutput\|branchCreate\|branchList" internal/
```

### Step 3: Create Destination Package

```bash
mkdir -p ${dstDir}
```

Create minimal `doc.go` if this is a new package:

```go
// Package branches implements MCP tool handlers for GitLab branch operations.
package branches
```

Verify: `go build ./...` (empty package is fine)

### Step 4: Copy and Transform Source File

1. **Copy** (don't move yet) `${srcFile}` → `${dstDir}/${filename}`
2. **Change package declaration**: `package ${srcPkg}` → `package ${dstPkg}`
3. **Update imports**:
   - Remove self-package imports (they're now local)
   - Add imports for shared utilities: `"module/path/internal/toolutil"`
   - Add imports for GitLab client, MCP SDK as needed
4. **Export functions** that need to be called externally:
   - `branchCreate` → `Create` (exported, package provides namespace)
   - `branchList` → `List`
5. **Rename types** to remove domain prefix:
   - `BranchCreateInput` → `CreateInput`
   - `BranchOutput` → `Output`
6. **Update internal references** to use `toolutil.` prefix for shared utilities

### Step 5: Create Forwarding Stubs (Bridge)

In the **original** package, replace the moved code with forwarding stubs:

```go
// branches.go — forwarding stubs (temporary, remove after all consumers updated)
package tools

import "module/path/internal/tools/branches"

// Type aliases for backward compatibility during migration.
type BranchCreateInput = branches.CreateInput
type BranchOutput = branches.Output
type BranchListOutput = branches.ListOutput

// Function forwarding for backward compatibility during migration.
var branchCreate = branches.Create
var branchList = branches.List
var branchGet = branches.Get
```

**CRITICAL**: This step ensures all existing code that references `tools.BranchCreateInput` or calls `branchCreate()` still compiles.

Verify: `go build ./...` — MUST pass before continuing.

### Step 6: Update Direct Consumers

Find all files that reference the moved symbols:

```bash
grep -rn "branchCreate\|BranchCreateInput\|BranchOutput" internal/ --include="*.go" | grep -v "_test.go" | grep -v "branches/"
```

Update each consumer to import from the new package:

**Before**:

```go
package tools

func registerBranchTools(server *mcp.Server, client *gitlabclient.Client) {
    // ... uses branchCreate, BranchCreateInput directly
}
```

**After**:

```go
package tools

import "module/path/internal/tools/branches"

func registerBranchTools(server *mcp.Server, client *gitlabclient.Client) {
    // ... uses branches.Create, branches.CreateInput
}
```

**Or better** — move registration into the sub-package itself (see Step 7).

Verify after each consumer update: `go build ./...`

### Step 7: Move Registration

Create `${dstDir}/register.go` with `RegisterTools()`:

```go
package branches

import (
    "context"
    "time"

    "github.com/modelcontextprotocol/go-sdk/mcp"
    gitlabclient "module/path/internal/gitlab"
    "module/path/internal/toolutil"
)

func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
    mcp.AddTool(server, &mcp.Tool{
        Name:        "gitlab_branch_create",
        Description: "...",
        Annotations: toolutil.CreateAnnotations,
    }, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
        start := time.Now()
        out, err := Create(ctx, client, input)
        toolutil.LogToolCallAll(ctx, req, "gitlab_branch_create", start, err)
        return toolutil.MarkdownForResult(out), out, err
    })
    // ... other tools
}
```

Update `internal/tools/register.go`:

```go
// Replace:
registerBranchTools(server, client)
// With:
branches.RegisterTools(server, client)
```

Remove the old `registerBranchTools` function.

Verify: `go build ./...`

### Step 8: Move Tests

1. Copy `${srcFile}_test.go` → `${dstDir}/${filename}_test.go`
2. Change package: `package tools` → `package branches`
3. Update type references: `BranchOutput` → `Output`
4. Update function references: `branchCreate(...)` → `Create(...)`
5. Import test helpers (from `toolutil` or recreate locally):

   ```go
   import "module/path/internal/toolutil/testutil"
   ```

6. If test helpers use `newTestClient`, either:
   - Import from a shared test utilities package
   - Or copy the helper into the test file (since it's small)

Verify:

```bash
go test ./${dstDir}/ -count=1 -v
```

### Step 9: Remove Forwarding Stubs

Once ALL consumers are updated and ALL tests pass:

1. Delete the forwarding stub file from `${srcPkg}/`
2. Delete the original test file from `${srcPkg}/`

Verify:

```bash
go build ./...
go test ./internal/... -count=1
```

### Step 10: Final Validation

```bash
go build ./...
go vet ./...
go test ./internal/... -count=1
```

## Handling Complex Cases

### Circular Import Prevention

If moving file A to package B would create a circular import:

```text
tools → branches → tools (CIRCULAR!)
```

Solution: Extract the shared dependency to `toolutil/`:

```text
tools → branches → toolutil (OK)
tools → toolutil (OK)
```

### Shared Types Used Across Domains

If `BranchOutput` is used in another domain (e.g., `merge_requests.go` references branches):

1. Keep the type in the domain that owns it (`branches.Output`)
2. Have the other domain import it: `import "module/path/internal/tools/branches"`
3. OR if the type is truly cross-cutting, extract to `toolutil/`

### Format Functions in markdown.go

`markdown.go` contains `format*` functions for EVERY domain. Options:

**Option A** (Recommended): Keep format functions in `toolutil/markdown.go` as a centralized formatter
**Option B**: Move domain-specific formatters to their domain package
**Option C**: Use an interface — each domain implements `Formatter`

Choose Option A for this project because format functions are simple and don't warrant an interface.

### Test Helpers

The `helpers_test.go` file is shared by ALL domain tests. Options:

**Option A**: Extract to `toolutil/testhelpers_test.go` (only available in `toolutil` tests)
**Option B** (Recommended): Create `internal/testutil/` package with exported helpers
**Option C**: Copy helpers into each domain test file (duplicated but simple)

Choose Option B for this project. Create:

```go
package testutil

// NewTestClient creates a GitLab client pointed at a test HTTP server.
func NewTestClient(t *testing.T, handler http.Handler) *gitlabclient.Client { ... }

// RespondJSON writes a JSON response with the given status code and body.
func RespondJSON(w http.ResponseWriter, status int, body string) { ... }

// RespondJSONWithPagination writes a JSON response with GitLab pagination headers.
func RespondJSONWithPagination(w http.ResponseWriter, status int, body string, p PaginationHeaders) { ... }
```

## Batch Processing Template

For moving multiple domains efficiently:

```text
Batch 1: Extract shared utilities → toolutil/
  ├── Verify: go build && go test
  └── Commit: "refactor: extract shared utilities to internal/toolutil"

Batch 2: Move simple domains (health, users, tags, labels, milestones)
  ├── For each domain: create, move, stub, verify
  ├── Verify: go build && go test
  └── Commit: "refactor: modularize health/users/tags/labels/milestones"

Batch 3: Move medium domains (branches, commits, files, groups, pipelines)
  ├── Same pattern
  └── Commit: "refactor: modularize branches/commits/files/groups/pipelines"

Batch 4: Move complex multi-file domains (mergerequests, packages)
  ├── Consolidate files first, then move
  └── Commit: "refactor: modularize mergerequests and packages"

Batch 5: Clean up stubs, update docs
  └── Commit: "refactor: remove forwarding stubs, update documentation"
```

## Rollback Strategy

If a migration batch goes wrong:

1. `git stash` or `git checkout -- .` to revert current changes
2. Review what broke (usually import paths or missing exports)
3. Fix the specific issue
4. Re-attempt the migration

Since each batch is committed separately, you can always `git revert` a single batch without affecting others.

## GitLab client-go Awareness

When moving tool handlers that call the GitLab API, preserve these patterns exactly:

### client-go Service Access

Every handler accesses the API via `client.GL().{Service}.{Method}()`. The `client` parameter is `*gitlabclient.Client` (alias for `internal/gitlab.Client`). This pattern does NOT change during migration — the client is passed as a parameter, not imported.

### Import Requirements for Moved Files

After moving a tool handler to a sub-package, ensure these imports:

```go
import (
    gl "gitlab.com/gitlab-org/api/client-go/v2"                        // For gl.*Options, gl.Ptr(), gl.WithContext()
    gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"  // For client type
    "github.com/jmrplens/gitlab-mcp-server/internal/toolutil"             // For shared utilities
    "github.com/modelcontextprotocol/go-sdk/mcp"                          // For tool registration
)
```

### Files With Low-Level HTTP Access

Some files (`packages_stream.go`, `packages_chunked.go`, `uploads.go`) use `client.GL().NewRequest()` and `client.GL().Do()` for direct HTTP calls. These also use `client.GL().BaseURL()` for URL construction. Ensure all three patterns work after the move.

### Naming Inconsistency Fix

When moving `repositories.go` → `projects/projects.go`, this is a **rename AND move**. The file uses `client.GL().Projects.*` (not `Repositories`). Update the package doc comment to reflect that this is the Projects domain.

### Domain Reference Hierarchy

The **client-go API library** (`gitlab.com/gitlab-org/api/client-go/v2`) is the source of truth for domain organization, type structures, and field definitions.

Before moving any domain:

1. **Inspect client-go types first**: Run `go doc gitlab.com/gitlab-org/api/client-go/v2.{Type}` for the domain's key types (e.g., `gl.Branch`, `gl.CreateBranchOptions`). This defines the canonical fields and API contract — use it to validate that type renames and field mappings are correct after the move.
2. **Read the source file(s)** (`internal/tools/{domain}.go`) to understand our implementation: handler functions, `client.GL().{Service}.*` calls, and our Input/Output struct subset.
3. **If `docs/api-mapping/{domain}.md` exists**, read it for supplementary field-level context. If no doc exists, `go doc` + source code provide everything needed.
4. **Check registration**: verify the domain appears in both `register.go` and `register_meta.go`. Unregistered files are in-progress features — still move them, but note the gap.
