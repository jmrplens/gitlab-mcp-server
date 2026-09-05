# GraphQL Integration

> **Diátaxis type**: Explanation
> **Audience**: 🔧 Developers, contributors
> **Prerequisites**: Familiarity with Go, GraphQL basics, and the project architecture

---

## Overview

gitlab-mcp-server uses two API strategies to communicate with GitLab:

1. **REST API v4** — the primary approach, used by the majority of tools via the [client-go](https://pkg.go.dev/gitlab.com/gitlab-org/api/client-go/v2) service wrappers
2. **GraphQL API** — used for domains where REST endpoints are deprecated, unavailable, or significantly less efficient

This document explains when and how the GraphQL integration is used, the patterns involved, and the architectural rationale behind the design.

## When REST vs GraphQL

| Use REST when                                | Use GraphQL when                                              |
| -------------------------------------------- | ------------------------------------------------------------- |
| client-go has a typed service wrapper        | No REST endpoint exists (e.g. CI/CD Catalog, Branch Rules)    |
| The domain is well-served by REST            | The REST endpoint is deprecated (e.g. vulnerability findings) |
| Single-resource CRUD operations              | Multiple related resources need to be fetched in one request  |
| The feature is available on all GitLab tiers | The feature is GraphQL-only (e.g. CI/CD Catalog)              |

### Domains using GraphQL

| Domain                     | Package                              | Reason                                                                                                                                       |
| -------------------------- | ------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------- |
| Epics (6 tools)            | `internal/tools/epics/`              | REST API deprecated since GitLab 17.0 (removal 19.0); migrated to Work Items GraphQL API via client-go `WorkItems` service                   |
| Epic Notes (5 tools)       | `internal/tools/epicnotes/`          | REST API deprecated since GitLab 17.0 (removal 19.0); raw GraphQL against the Work Items notes widget, GID resolved by `epicworkitems`       |
| Epic Discussions (6 tools) | `internal/tools/epicdiscussions/`    | REST API deprecated since GitLab 17.0 (removal 19.0); raw GraphQL against the Work Items discussions widget, GID resolved by `epicworkitems` |
| Epic Issues (4 tools)      | `internal/tools/epicissues/`         | REST API deprecated since GitLab 17.0 (removal 19.0); raw GraphQL against the Work Items hierarchy widget, GID resolved by `epicworkitems`   |
| Work Items (13 tools)      | `internal/tools/workitems/`          | Work items and saved views are GraphQL-only; typed client-go `WorkItems` service                                                             |
| Vulnerabilities            | `internal/tools/vulnerabilities/`    | GraphQL provides richer query/mutation capabilities than REST                                                                                |
| Security Attributes        | `internal/tools/securityattributes/` | GraphQL-only namespace classification feature; raw `GraphQL.Do()` queries and mutations                                                      |
| Security Categories        | `internal/tools/securitycategories/` | GraphQL-only namespace classification feature; raw `GraphQL.Do()` queries and mutations                                                      |
| Security Findings          | `internal/tools/securityfindings/`   | REST endpoint deprecated; GraphQL `Pipeline.securityReportFindings` is the replacement                                                       |
| CI/CD Catalog              | `internal/tools/cicatalog/`          | GraphQL-only feature — no REST API exists                                                                                                    |
| Branch Rules               | `internal/tools/branchrules/`        | GraphQL-only aggregated view of branch protections, approval rules, and status checks                                                        |
| Custom Emoji               | `internal/tools/customemoji/`        | GraphQL-only — no REST API for custom emoji management                                                                                       |

## Architecture

```mermaid
graph TD
    subgraph "Catalog-backed ActionSpecs"
        A[REST-backed GitLab actions]
        B[GraphQL actions — raw GraphQL.Do]
        G[GraphQL actions — WorkItems service]
    end

    subgraph "client-go v2"
        C[Service Wrappers<br/>Projects, MRs, Issues...]
        D[GraphQL.Do<br/>Raw query execution]
        H[WorkItems Service<br/>Typed GraphQL wrappers]
    end

    subgraph "GitLab Instance"
        E[REST API v4]
        F[GraphQL API]
    end

    A --> C
    B --> D
    G --> H
    C --> E
    D --> F
    H --> F
```

## The Two GraphQL Patterns

### Pattern 1: Raw `GraphQL.Do()` for tool handlers

Used by domain sub-packages (`vulnerabilities`, `securityfindings`, `securityattributes`, `securitycategories`, `cicatalog`, `branchrules`, `customemoji`, and the epic widgets in `epicnotes`, `epicdiscussions` and `epicissues`) that implement complete tool handlers with GraphQL queries. The `epicworkitems` helper package (`ResolveEpicGID`, `ResolveWorkItemGID`) turns a group path and IID into the global ID those widget mutations need.

```go
// Define the query as a Go constant
const queryListVulnerabilities = `
query($projectPath: ID!, $first: Int!, $after: String) {
  project(fullPath: $projectPath) {
    vulnerabilities(first: $first, after: $after) {
      nodes { id title severity state }
      pageInfo { hasNextPage endCursor }
    }
  }
}
`

// Response struct must include Data envelope wrapper
var resp struct {
    Data struct {
        Project struct {
            Vulnerabilities struct {
                Nodes    []gqlVulnerabilityNode     `json:"nodes"`
                PageInfo toolutil.GraphQLRawPageInfo `json:"pageInfo"`
            } `json:"vulnerabilities"`
        } `json:"project"`
    } `json:"data"`
}

// Execute — note the two return values (response pointer, error)
_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
    Query:     queryListVulnerabilities,
    Variables: vars,
}, &resp, gl.WithContext(ctx))
```

### Pattern 2: client-go `WorkItems` service wrappers

Used by the `epics` package after migrating from the deprecated Epics REST API, and by `workitems`. The client-go `WorkItems` service provides typed Go methods that execute GraphQL queries internally, so tool handlers don't write raw GraphQL. Every method is addressed by namespace path and IID; client-go resolves the global ID itself.

```go
// List epics — client-go builds and executes the GraphQL query internally
items, _, err := client.GL().WorkItems.ListWorkItems(input.FullPath, &gl.ListWorkItemsOptions{
    First: &defaultFirst,
    Types: []string{"EPIC"},
    State: gl.Ptr(input.State),
}, gl.WithContext(ctx))

// Get a single epic by IID
item, _, err := client.GL().WorkItems.GetWorkItem(input.FullPath, input.IID, gl.WithContext(ctx))

// Create an epic (WorkItemTypeEpic = "gid://gitlab/WorkItems::Type/8")
item, _, err := client.GL().WorkItems.CreateWorkItem(input.FullPath, gl.WorkItemTypeEpic, &gl.CreateWorkItemOptions{
    Title: input.Title,
}, gl.WithContext(ctx))

// Update and delete take the same path + IID pair
item, _, err := client.GL().WorkItems.UpdateWorkItem(input.FullPath, input.IID, &gl.UpdateWorkItemOptions{
    Title: gl.Ptr(newTitle),
}, gl.WithContext(ctx))
_, err = client.GL().WorkItems.DeleteWorkItem(input.FullPath, input.IID, gl.WithContext(ctx))
```

**When to use this pattern**: When client-go provides typed service wrappers for the GraphQL domain (currently: WorkItems). This is preferred over raw `GraphQL.Do()` because it avoids hand-written query strings and response structs.

**Where a GID is still needed**: the epic notes, discussions and issue-link mutations are raw GraphQL against work item widgets, and those mutations take a GitLab Global ID (a string like `"gid://gitlab/WorkItem/101"`) rather than a path and IID. `epicworkitems.ResolveEpicGID` and `ResolveWorkItemGID` do that lookup once so the three sub-packages share it.

## Key Design Decisions

### Data envelope wrapper

The client-go `GraphQL.Do()` method decodes the full JSON response (including the `{"data": ...}` wrapper) into the provided struct. Response structs **must** include a `Data` field:

```go
// CORRECT — includes Data wrapper
var resp struct {
    Data struct {
        Project struct { ... } `json:"project"`
    } `json:"data"`
}

// WRONG — client-go does NOT strip the data envelope
var resp struct {
    Project struct { ... } `json:"project"`
}
```

### Two return values from `GraphQL.Do()`

The method returns `(*Response, error)`. When you only need the error:

```go
_, err := client.GL().GraphQL.Do(query, &resp, gl.WithContext(ctx))
```

### GitLab Global IDs (GIDs)

GraphQL uses GIDs in the format `gid://gitlab/Type/NumericID`. The `toolutil` package provides helpers:

```go
gid := toolutil.FormatGID("Vulnerability", 42)
// → "gid://gitlab/Vulnerability/42"

typeName, id, err := toolutil.ParseGID("gid://gitlab/Vulnerability/42")
// → "Vulnerability", 42, nil
```

### Cursor-based pagination

GraphQL uses cursor-based pagination instead of REST's page/per_page model. The `toolutil.GraphQLPaginationInput` struct provides a consistent interface:

| Parameter | Description                                        |
| --------- | -------------------------------------------------- |
| `first`   | Number of items to return (default 20, max 100)    |
| `after`   | Forward pagination cursor                          |
| `last`    | Number of items from the end (backward pagination) |
| `before`  | Backward pagination cursor                         |

The `Variables()` method converts these to a GraphQL variables map, and `PageInfoToOutput()` normalizes the camelCase API response to snake\_case output.

### GraphQL mutation error handling

GraphQL mutations return both transport-level errors and application-level errors in the response body:

```go
var resp struct {
    Data struct {
        VulnerabilityDismiss struct {
            Vulnerability gqlVulnerabilityNode `json:"vulnerability"`
            Errors        []string             `json:"errors"`
        } `json:"vulnerabilityDismiss"`
    } `json:"data"`
}

_, err := client.GL().GraphQL.Do(query, &resp, gl.WithContext(ctx))
if err != nil {
    return ..., toolutil.WrapErr("dismiss_vulnerability", err)
}
if len(resp.Data.VulnerabilityDismiss.Errors) > 0 {
    return ..., toolutil.GraphQLMutationError("dismiss_vulnerability",
        resp.Data.VulnerabilityDismiss.Errors)
}
```

## Shared Utilities

The `internal/toolutil/graphql.go` module provides shared GraphQL infrastructure:

| Type/Function               | Purpose                                                        |
| --------------------------- | -------------------------------------------------------------- |
| `GraphQLPaginationInput`    | Cursor-based pagination input struct with `Variables()` method |
| `GraphQLPaginationOutput`   | Normalized pagination output for tool responses                |
| `GraphQLRawPageInfo`        | Raw camelCase page info from API responses                     |
| `PageInfoToOutput()`        | Converts raw page info to output format                        |
| `FormatGraphQLPagination()` | Renders pagination as Markdown summary                         |
| `FormatGID()`               | Builds a GitLab GID string                                     |
| `ParseGID()`                | Extracts type and ID from a GID string                         |
| `MergeVariables()`          | Merges multiple variable maps                                  |
| `GraphQLTopLevelError()`    | Wraps the top-level `errors` array of a GraphQL response       |
| `GraphQLMutationError()`    | Wraps a mutation payload's `errors` array as one error         |

## Testing GraphQL Tools

GraphQL tools are tested using `httptest` mocking at the `/api/graphql` endpoint. The `testutil` package provides:

- `testutil.GraphQLHandler(map[string]http.HandlerFunc)` — routes GraphQL requests by matching query strings
- `testutil.RespondGraphQL(w, status, dataJSON)` — wraps response in `{"data": ...}` envelope

```go
client := testutil.NewTestClient(t, testutil.GraphQLHandler(
    map[string]http.HandlerFunc{
        "vulnerabilities": func(w http.ResponseWriter, r *http.Request) {
            testutil.RespondGraphQL(w, http.StatusOK, `{
                "project": {
                    "vulnerabilities": {
                        "nodes": [...],
                        "pageInfo": {"hasNextPage": false}
                    }
                }
            }`)
        },
    },
))
```

## References

- [GitLab GraphQL API Reference](https://docs.gitlab.com/ee/api/graphql/reference/)
- [GitLab GraphQL Explorer](https://docs.gitlab.com/ee/api/graphql/#interactive-graphql-explorer)
- [client-go GraphQL.Do()](https://pkg.go.dev/gitlab.com/gitlab-org/api/client-go/v2#GraphQL.Do)
- [ADR-0006: Raw GraphQL.Do() for Uncovered Domains](../development/adr/adr-0006-raw-graphql-for-uncovered-domains.md)
