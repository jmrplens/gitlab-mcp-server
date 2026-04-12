---
title: "ADR-0006: Raw GraphQL.Do() for domains without client-go service wrappers"
status: "Accepted"
date: "2026-03-23"
authors: "jmrplens"
tags: ["architecture", "decision", "graphql", "api-coverage"]
superseded_by: ""
---

# ADR-0006: Raw GraphQL.Do() for domains without client-go service wrappers

## Status

**Accepted** — extends ADR-0004 (modular tools sub-packages) with a new API access pattern.

## Context

The project uses the official GitLab Go client (`gitlab.com/gitlab-org/api/client-go/v2`) as its primary interface to the GitLab API. This client wraps most GitLab REST API v4 endpoints with typed Go methods and response structs, covering approximately 95% of the API surface (162 domain sub-packages use REST exclusively).

However, several GitLab API domains are **only available via GraphQL** and have no corresponding service wrapper in `client-go`:

| Domain             | GitLab API Coverage                          | client-go Status     |
| ------------------ | -------------------------------------------- | -------------------- |
| Vulnerabilities    | REST (partial) + GraphQL (full: mutations, severity counts, pipeline summary) | REST only, no mutations |
| Security Findings  | GraphQL only (pipeline security tab)         | Not covered          |
| CI/CD Catalog      | GraphQL only (catalog resources, components) | Not covered          |
| Branch Rules       | GraphQL only (consolidated branch protections) | Not covered          |
| Custom Emoji       | GraphQL only (group-level custom emoji CRUD) | Not covered          |

### Options considered

#### Option 1: Wait for client-go coverage (rejected)

- **Pros**: Maintains single API access pattern, typed responses
- **Cons**: No timeline for coverage; some domains (CI/CD Catalog, Branch Rules) were introduced in GitLab 16+ and may take years to be wrapped. Blocks feature delivery indefinitely.

#### Option 2: Fork or patch client-go (rejected)

- **Pros**: Typed interface, consistent with existing patterns
- **Cons**: Maintenance burden of keeping fork in sync, review overhead, upstream rejection risk. Disproportionate effort for 5 domains.

#### Option 3: Use a separate GraphQL Go client (rejected)

- **Pros**: Type-safe GraphQL with code generation (e.g., `shurcooL/graphql`, `hasura/go-graphql-client`)
- **Cons**: Adds a new dependency, requires schema introspection or manual type definitions. The existing `client-go` already provides `GraphQL.Do()` as a low-level escape hatch — adding another client is redundant.

#### Option 4: Raw GraphQL.Do() via existing client-go (accepted)

- **Pros**: Zero new dependencies, uses the `client-go` `GraphQL` service already available on the client instance. Consistent client lifecycle (authentication, TLS, retries). Allows targeting any GraphQL query/mutation immediately.
- **Cons**: No compile-time type safety on GraphQL responses; requires manual JSON unmarshalling into `Data` envelope structs. Queries are raw strings without schema validation.

## Decision

**Use the existing `client-go` `GraphQL.Do()` method** to call GitLab's GraphQL API for domains not covered by typed service wrappers. Each domain sub-package embeds raw GraphQL query strings, defines its own typed response structs, and unmarshals via a generic `Data` envelope pattern.

### Two usage patterns

#### Pattern 1: Tool handlers (5 domains)

Used by `vulnerabilities`, `securityfindings`, `cicatalog`, `branchrules`, and `customemoji`. Each sub-package:

1. Defines GraphQL query/mutation strings as Go constants
2. Defines typed response structs matching the expected JSON shape
3. Calls `client.GL().GraphQL.Do(gl.GraphQLQuery{Query: ..., Variables: ...})` with `json.Unmarshal` into a `Data` envelope struct
4. Converts raw responses to typed output structs

```go
type Data struct {
    Project struct {
        BranchRules struct {
            Nodes    []BranchRuleNode      `json:"nodes"`
            PageInfo toolutil.GraphQLRawPageInfo `json:"pageInfo"`
        } `json:"branchRules"`
    } `json:"project"`
}

_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
    Query:     listBranchRulesQuery,
    Variables: vars,
}, &data)
```

#### Pattern 2: Aggregation for sampling tools

Used by `samplingtools` to fetch rich context (vulnerability summaries, pipeline security reports) for LLM sampling prompts. Same `GraphQL.Do()` mechanism but queries aggregate data from multiple domains in a single request.

### Shared utilities

Shared GraphQL utilities live in `internal/toolutil/graphql.go`:

| Utility                     | Purpose                                                  |
| --------------------------- | -------------------------------------------------------- |
| `GraphQLPaginationInput`    | Cursor-based pagination input (first/after/last/before)  |
| `GraphQLPaginationOutput`   | Pagination metadata for tool responses                   |
| `GraphQLRawPageInfo`        | Raw camelCase PageInfo from GitLab API                   |
| `PageInfoToOutput()`        | Converts camelCase PageInfo to snake_case output          |
| `FormatGraphQLPagination()` | Formats pagination metadata as Markdown summary          |
| `FormatGID()`               | Builds GitLab Global ID (`gid://gitlab/Type/123`)        |
| `ParseGID()`                | Extracts type and numeric ID from a GitLab Global ID     |
| `MergeVariables()`          | Merges multiple variable maps for complex queries        |
| `GraphQLDefaultFirst`       | Default page size (20)                                   |
| `GraphQLMaxFirst`           | Maximum page size (100)                                  |

### Testing approach

All GraphQL tool handlers are tested using the same `httptest` mock infrastructure as REST-based tools. The mock server intercepts POST requests to `/api/graphql` and returns canned JSON responses. This approach:

- Requires no real GitLab instance for unit tests
- Validates query variable composition and response parsing
- Tests error paths (API errors, mutation failures, malformed responses)
- Maintains test consistency with the 162 REST-based sub-packages

## Consequences

### Positive

- **POS-001**: Immediate coverage of 5 previously unreachable API domains (15 new tools)
- **POS-002**: Zero new dependencies — reuses existing `client-go` GraphQL service
- **POS-003**: Consistent authentication, TLS, and retry behavior via shared client
- **POS-004**: Same sub-package structure (ADR-0004) and testing patterns as REST-based tools
- **POS-005**: Shared utilities in `toolutil/graphql.go` reduce boilerplate across domains

### Negative

- **NEG-001**: No compile-time validation of GraphQL queries — schema mismatches are caught at runtime only
- **NEG-002**: Response structs must be manually maintained if GitLab changes GraphQL schema fields
- **NEG-003**: Slightly more verbose than client-go typed methods (query string + Data envelope + type assertion)

### Mitigations

- **NEG-001, NEG-002**: Comprehensive unit tests with realistic mock responses catch schema drift early. E2E tests against a real GitLab instance provide additional validation.
- **NEG-003**: The `Data` envelope pattern and shared utilities keep boilerplate manageable. Each domain has at most 2-3 query strings.

### Migration path

If `client-go` adds typed wrappers for any of these 5 domains in the future, migration is straightforward:

1. Replace `GraphQL.Do()` calls with the new typed service methods
2. Remove the raw query strings and `Data` envelope structs
3. Keep the same input/output types and tool handler signatures
4. No changes to tool registration, meta-tools, or tests (only mock handlers switch from `/api/graphql` to REST endpoints)

## References

- [ADR-0004: Modular tools sub-packages](adr-0004-modular-tools-subpackages.md)
- [GraphQL Integration Architecture](../graphql.md)
- [GitLab GraphQL API](https://docs.gitlab.com/api/graphql/)
- [client-go GraphQL service](https://pkg.go.dev/gitlab.com/gitlab-org/api/client-go/v2#GraphQLService)
