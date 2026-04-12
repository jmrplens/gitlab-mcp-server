---
name: modularize-go-package
description: 'Modularize a monolithic Go package into domain-specific sub-packages. Extracts shared utilities, moves domain files, updates imports, renames types, creates registration functions, and validates compilation+tests at every step. Designed for large-scale refactoring of 50-100+ file packages.'
---

# Modularize Go Package

## Primary Directive

Transform a monolithic Go package into a modular structure with domain-specific sub-packages and a shared utilities package. Every step must maintain compilation and test integrity.

## Execution Model

This skill operates in **atomic migration batches**. Each batch moves one domain to its own sub-package. Between batches, the project must compile and all tests must pass. Never move to the next batch until the current one is verified.

## Prerequisites

Before invoking this skill, ensure:

1. Clean git working directory (`git status` shows no uncommitted changes)
2. All tests pass: `go test ./internal/... -count=1`
3. All code compiles: `go build ./...`
4. You have identified the source package and its domain files

## Input Parameters

- `${sourcePackage}` — The monolithic package to modularize (e.g., `internal/tools`)
- `${utilPackage}` — Name for the shared utilities package (e.g., `internal/toolutil`)
- `${domains}` — Comma-separated list of domains to extract (e.g., `branches,commits,issues`)

## Process

### Step 1: Inventory Analysis

Scan the source package and classify every file:

| Category | Files | Action |
|----------|-------|--------|
| **Shared utilities** | errors.go, pagination.go, logging.go, markdown.go, text.go, metatool.go, string_or_int.go, fileutils.go, time_helpers.go | Extract to `${utilPackage}` |
| **Shared constants** | Annotation variables, format constants | Extract to `${utilPackage}` |
| **Domain handlers** | branches.go, commits.go, etc. | Move to `${sourcePackage}/{domain}/` |
| **Domain tests** | branches_test.go, commits_test.go, etc. | Move with their domain |
| **Test helpers** | helpers_test.go | Extract to `${utilPackage}` as testutil |
| **Registration** | register.go, register_meta.go | Keep in `${sourcePackage}`, update to delegate |
| **Package doc** | (doc comment in any file) | Create `${sourcePackage}/doc.go` |

### CRITICAL: Dynamic Discovery

The **client-go API library** (`gitlab.com/gitlab-org/api/client-go/v2`) is the source of truth for domain organization, structures, and field definitions. Do NOT rely only on the tables in this skill or on `docs/api-mapping/`.

Before starting migration, run this discovery sequence:

```bash
# 1. Discover ALL client-go services (defines the universe of possible domains)
go doc gitlab.com/gitlab-org/api/client-go/v2.Client | Select-String -Pattern '\s+\w+\s+\*\w+Service'

# 2. List all non-test handler files in the source package (what we actually implement)
ls ${sourcePackage}/*.go | grep -v _test.go | grep -v -E '(errors|pagination|logging|markdown|text|metatool|string_or_int|fileutils|time_helpers|register|helpers)\.go$'
```

Compare the result against the domain mapping table in this skill. For any file NOT in the table:

1. **Check client-go types first**: Run `go doc gitlab.com/gitlab-org/api/client-go/v2.{Type}` to understand the canonical struct fields and API contracts for that domain
2. **Check `client.GL().{Service}.*` calls** in the source file → determines the sub-package name
3. **Check `register.go` / `register_meta.go`** → determines registration status
4. **Check `docs/api-mapping/{domain}.md`** IF it exists → supplementary field mapping context

The sub-package name must align with the client-go service name, not with our file naming.

### Step 2: Create Shared Utilities Package

Create `${utilPackage}/` and extract shared code in dependency order:

```text
Order (no circular deps):
1. Pure types with zero internal deps (StringOrInt, time helpers)
2. Types depending only on external libs (PaginationInput, ToolError)
3. Functions depending on types above (wrapErr, paginationFromResponse)
4. Complex utilities (markdown formatters, metatool dispatcher)
```

For each extracted file:

1. Create new file in `${utilPackage}/`
2. Change `package tools` → `package toolutil`
3. **Export all symbols** that are used by domain handlers:
   - Functions: `wrapErr` → `WrapErr`
   - Variables: `readAnnotations` → `ReadAnnotations`
   - Types remain exported if already exported
4. Keep the old file in `${sourcePackage}` temporarily as a **forwarding stub**:

   ```go
   // DEPRECATED: forwarding stub — will be removed when all domains are migrated.
   package tools

   import "github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

   var wrapErr = toolutil.WrapErr
   ```

5. Verify: `go build ./...`

### Step 3: Migrate Domain (Repeat per Domain)

For each domain in priority order:

#### 3a. Create Sub-Package

```bash
mkdir -p ${sourcePackage}/{domain}
```

#### 3b. Move and Transform Handler File

1. Copy `{domain}.go` → `${sourcePackage}/{domain}/{domain}.go`
2. Change package declaration: `package tools` → `package {domain}`
3. Update imports to use `${utilPackage}` instead of direct references
4. **Rename types** — remove domain prefix (the package name provides context):
   - `BranchCreateInput` → `CreateInput`
   - `BranchOutput` → `Output`
   - `BranchListOutput` → `ListOutput`
5. **Export handler functions** — remove domain prefix, capitalize:
   - `branchCreate` → `Create`
   - `branchList` → `List`
   - `branchGet` → `Get`
6. Replace internal utility calls:
   - `wrapErr(...)` → `toolutil.WrapErr(...)`
   - `markdownForResult(...)` → `toolutil.MarkdownForResult(...)`
   - `logToolCallAll(...)` → `toolutil.LogToolCallAll(...)`
   - `readAnnotations` → `toolutil.ReadAnnotations`

#### 3c. Create Registration File

Create `${sourcePackage}/{domain}/register.go`:

```go
package {domain}

import (
    "context"
    "time"

    "github.com/modelcontextprotocol/go-sdk/mcp"
    gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
    "github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all {domain} MCP tools on the server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
    // ... tool registrations moved from register.go
}
```

#### 3d. Create Meta Registration File

Create `${sourcePackage}/{domain}/register_meta.go`:

```go
package {domain}

import (
    "github.com/modelcontextprotocol/go-sdk/mcp"
    gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
    "github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterMeta registers the {domain} meta-tool on the server.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
    // ... meta-tool registration moved from register_meta.go
}
```

#### 3e. Move and Transform Test File

1. Copy `{domain}_test.go` → `${sourcePackage}/{domain}/{domain}_test.go`
2. Change package: `package tools` → `package {domain}` (or `package {domain}_test` for black-box)
3. Update type references to match renamed types
4. Import test helpers from `${utilPackage}` or recreate locally
5. Update handler function references

#### 3f. Update Orchestration Layer

In `${sourcePackage}/register.go`, replace:

```go
register{Domain}Tools(server, client)
```

with:

```go
{domain}.RegisterTools(server, client)
```

Remove the old `register{Domain}Tools` function body.

#### 3g. Remove Old Files

Delete the original files from `${sourcePackage}/`:

- `{domain}.go`
- `{domain}_test.go`

#### 3h. Verify

```bash
go build ./...
go vet ./...
go test ./${sourcePackage}/{domain}/ -count=1 -v
go test ./${sourcePackage}/ -count=1
```

### Step 4: Clean Up Forwarding Stubs

After ALL domains are migrated:

1. Remove forwarding stubs from `${sourcePackage}/`
2. Remove old utility files (they now live in `${utilPackage}/`)
3. Final verification:

   ```bash
   go build ./...
   go vet ./...
   go test ./internal/... -count=1
   ```

### Step 5: Update Entry Point

Verify `cmd/server/main.go` still only imports `${sourcePackage}`:

```go
import "github.com/jmrplens/gitlab-mcp-server/internal/tools"

// tools.RegisterAll(server, client) — still works, delegates internally
```

## Validation Checklist

After completing all migrations:

- [ ] `go build ./...` — zero errors
- [ ] `go vet ./...` — zero warnings
- [ ] `go test ./internal/... -count=1` — all pass
- [ ] No import cycles: `go vet -vettool=$(which findcall) ./...` or manual review
- [ ] `cmd/server/main.go` unchanged (still imports `internal/tools`)
- [ ] Each sub-package has: handler file, register.go, register_meta.go, test file
- [ ] `${utilPackage}` has no imports from domain sub-packages
- [ ] Domain sub-packages don't import each other

## Multi-File Domain Handling

For domains that span multiple source files, consolidate during migration:

### Merge Requests (6 files → 1 sub-package)

```text
merge_requests.go      → mergerequests/merge_requests.go
mr_notes.go            → mergerequests/notes.go
mr_discussions.go      → mergerequests/discussions.go
mr_changes.go          → mergerequests/changes.go
mr_approvals.go        → mergerequests/approvals.go
mr_draft_notes.go      → mergerequests/draft_notes.go
```

Each file keeps its handler functions; the `register.go` consolidates all MR tool registrations.

### Packages (4 files → 1 sub-package)

```text
packages.go            → packages/packages.go
packages_chunked.go    → packages/chunked.go
packages_composite.go  → packages/composite.go
packages_stream.go     → packages/stream.go
```

## Error Recovery

### Compilation Error After Move

```text
# Most common: unexported symbol
./internal/tools/branches/branches.go:15: undefined: wrapErr
→ Fix: Change to toolutil.WrapErr

# Missing import
./internal/tools/branches/branches.go:3: imported and not used
→ Fix: Remove unused import, add missing one

# Circular import
package gitlab.example.com/.../tools imports gitlab.example.com/.../tools/branches imports gitlab.example.com/.../tools
→ Fix: Extract shared code to toolutil, break the cycle
```

### Test Failure After Move

```text
# Test helper not found
./internal/tools/branches/branches_test.go:10: undefined: newTestClient
→ Fix: Import from toolutil or recreate locally

# Type mismatch
cannot use BranchOutput as tools.BranchOutput
→ Fix: Update test to use new type name (Output instead of BranchOutput)
```

## GitLab API Domain Reference

When modularizing `internal/tools/`, use this mapping to understand which files belong together and why. Each sub-package should correspond to a coherent GitLab API domain.

### Service-to-SubPackage Mapping

The project uses `gitlab.com/gitlab-org/api/client-go/v2` v2.17.0. Each `client.GL().{Service}` call tells you which API domain a handler belongs to:

| Sub-Package | client-go Services Used | Source Files |
|---|---|---|
| `branches/` | `Branches`, `ProtectedBranches` | `branches.go` |
| `tags/` | `Tags` | `tags.go` |
| `commits/` | `Commits` | `commits.go` |
| `files/` | `RepositoryFiles` | `files.go` |
| `repository/` | `Repositories` | `repository.go` |
| `projects/` | `Projects` | `repositories.go` (misnamed — rename during move) |
| `mergerequests/` | `MergeRequests`, `MergeRequestApprovals`, `Notes`, `Discussions`, `DraftNotes` | `merge_requests.go`, `mr_notes.go`, `mr_discussions.go`, `mr_changes.go`, `mr_approvals.go`, `mr_draft_notes.go` |
| `issues/` | `Issues`, `Notes` | `issues.go`, `issue_notes.go` |
| `labels/` | `Labels` | `labels.go` |
| `milestones/` | `Milestones` | `milestones.go` |
| `members/` | `ProjectMembers` | `members.go` |
| `groups/` | `Groups` | `groups.go` |
| `pipelines/` | `Pipelines` | `pipelines.go` |
| `jobs/` | `Jobs` | `jobs.go` |
| `releases/` | `Releases`, `ReleaseLinks` | `releases.go`, `release_links.go` |
| `search/` | `Search` | `search.go` |
| `users/` | `Users` | `users.go` |
| `packages/` | `Packages`, `GenericPackages` | `packages.go`, `packages_chunked.go`, `packages_composite.go`, `packages_stream.go` |
| `uploads/` | `ProjectMarkdownUploads` | `uploads.go` |
| `wikis/` | `Wikis` | `wikis.go` |
| `todos/` | `Todos` | `todos.go` |
| `health/` | `Version` | `health.go` |
| `environments/` | `Environments` | `environments.go` |
| `sampling/` | _(MCP-only, no GitLab API)_ | `sampling_tools.go` |
| `elicitation/` | _(MCP-only, no GitLab API)_ | `elicitation_tools.go` |

> **âš ï¸ This table may be incomplete.** Always scan the source package for files not listed here before starting a migration session. Any unlisted handler file is a new domain to add to the plan.

### client-go Import Patterns

After migration, each sub-package will import:

```go
import (
    gl "gitlab.com/gitlab-org/api/client-go/v2"
    gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
    "github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)
```

Preserve these client-go calling patterns exactly:

- **CRUD**: `result, resp, err := client.GL().{Service}.{Method}(args..., gl.WithContext(ctx))`
- **Delete**: `_, err := client.GL().{Service}.Delete{Resource}(id, gl.WithContext(ctx))`
- **Low-level HTTP** (packages only): `client.GL().NewRequest(...)` + `client.GL().Do(...)`
- **Option structs**: `&gl.List{Resource}Options{...}` — these never change, they come from client-go

### Naming Fix During Migration

The file `repositories.go` contains **Projects** CRUD operations (uses `client.GL().Projects.*`), NOT repository operations. When moving to the `projects/` sub-package, rename it to `projects.go`. The actual repository operations (`tree`, `compare`) are in `repository.go` and use `client.GL().Repositories.*`.

### Reference Documentation

The **client-go API library** is the source of truth for domain structure and field definitions. Our source code implements a subset of it. `docs/api-mapping/` is supplementary.

Before migrating each domain:

1. **Inspect client-go types**: Run `go doc gitlab.com/gitlab-org/api/client-go/v2.{Type}` for the domain's key types (e.g., `gl.Environment`, `gl.CreateEnvironmentOptions`). This defines the canonical fields, types, and API contract.
2. **Read the source file(s)** in `internal/tools/{domain}.go` — shows our implementation: which client-go fields we expose, our Input/Output structs, and `client.GL().{Service}` calls.
3. **Check `register.go` and `register_meta.go`** for registration. Unregistered files are in-progress — still migrate them.
4. **Read `docs/api-mapping/{domain}.md` IF it exists** — supplementary field mapping context. If no doc exists, the combination of steps 1+2 provides everything needed.
5. **Discover new domains** by scanning `*.go` files AND running `go doc` on the client to find services we haven't wrapped yet.

Never skip a domain just because it lacks documentation. The client-go types have all the information needed.
