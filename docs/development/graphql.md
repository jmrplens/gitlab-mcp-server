# GraphQL Integration

> **Diátaxis type**: Explanation
> **Audience**: 🔧 Developers, contributors
> **Prerequisites**: Familiarity with Go, GraphQL basics, and the project architecture

---

## Overview

gitlab-mcp-server uses two API strategies to communicate with GitLab:

1. **REST API v4** — the primary approach, used by the majority of tools via the [client-go](https://pkg.go.dev/gitlab.com/gitlab-org/api/client-go/v3) service wrappers
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
// Define the query as a Go constant. Every cursor variable the tool's input can
// send is declared here, and $first is nullable because a backward request
// sends $last instead.
const queryListVulnerabilities = `
query($projectPath: ID!, $first: Int, $after: String, $last: Int, $before: String) {
  project(fullPath: $projectPath) {
    vulnerabilities(first: $first, after: $after, last: $last, before: $before) {
      nodes { id title severity state }
      pageInfo { hasNextPage hasPreviousPage endCursor startCursor }
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

// Build the cursor variables against the document about to run. Naming it here
// is what stops a page request carrying a variable the operation never declared.
vars, err := input.Variables(queryListVulnerabilities)
if err != nil {
    return ListOutput{}, fmt.Errorf("list_vulnerabilities: %w", err)
}
vars["projectPath"] = input.ProjectPath

// Execute — note the two return values (response pointer, error)
_, err = client.GL().GraphQL.Do(gl.GraphQLQuery{
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

GraphQL uses cursor-based pagination instead of REST's page/per_page model. Two input structs carry it, and which one a domain embeds says what its connection can do.

`toolutil.GraphQLPaginationInput` is the forward-only shape and the default choice:

| Parameter | Description                                     |
| --------- | ----------------------------------------------- |
| `first`   | Number of items to return (default 20, max 100) |
| `after`   | Forward pagination cursor                       |

`toolutil.GraphQLCursorPaginationInput` embeds it and adds the backward pair, for the connections GitLab lets a caller walk in both directions:

| Parameter | Description                                                                 |
| --------- | --------------------------------------------------------------------------- |
| `last`    | Number of items to return from the end of the range (backward pagination)   |
| `before`  | Backward pagination cursor, taken from a previous response's `start_cursor` |

Forward only is the default because backward pagination is a property of the GitLab field, not of the helper. `Project.branchRules` and the work item notes widget's `discussions` both answer `before` and `last` with `argumentNotAccepted`. What they do with the backward half of `pageInfo` differs, and the difference matters: `branchRules` reports no previous page and no `start_cursor`, while `discussions` is keyset-paginated and reports both from its second page on. Embedding the bidirectional struct over a field like that would break forward pagination too, since a validation error rejects the whole document.

A forward-only tool therefore reports `toolutil.GraphQLForwardPaginationOutput`, which carries `has_next_page` and `end_cursor` and nothing else. Passing the whole of `pageInfo` on would hand a model a `start_cursor` and no parameter that could spend it, which reads as a capability the tool withdrew rather than as the dead end it is.

Both `Variables()` methods take the operation they are about to run, and refuse a document that cannot carry every variable the input can send. That signature is the guard for a defect this repository shipped for months: a variable an operation does not declare is discarded by GitLab rather than rejected, so eight tools advertised `last` and `before`, dropped them on the floor, and answered every backward request with the first page and no error.

The refusal covers three ways a document can fail to carry a variable, because a declaration on its own proves nothing:

| The document                          | Why it is refused                                                                             |
| ------------------------------------- | --------------------------------------------------------------------------------------------- |
| does not declare the variable         | GitLab discards it, and the caller is answered with somebody else's page                      |
| declares it non-null with no default  | the input omits it on some requests, and GitLab rejects the whole operation when it is absent |
| declares it and passes it to no field | as silent as never declaring it, on any document with more than one connection                |

The guard binds a document to a variable map, not to the request that is sent, so it reaches the call sites that ask for the map and no further. A domain whose query lives in client-go calls `Resolve()` instead and takes the direction rule without the document check, since the SDK owns the document: `achievements`, `workitems` and `workitemsavedviews` are the three.

`GraphQLCursorPaginationInput` sends exactly one of `first` and `last`. The cursor picks the direction and the count only sizes the page, so `before` with no `last` still pages backwards at the default size, and `before` with `first` uses that number as the backward page size. Naming both counts is refused rather than reinterpreted, because GitLab's keyset connections answer the pair with `Can only provide either first or last, not both` and the array-backed ones silently intersect them.

`PageInfoToOutput()` normalizes the camelCase API response to snake\_case output.

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

A mutation GitLab refuses outright answers neither way: the payload is `null` and the reason is a top-level `errors[]` entry. A handler that decodes the payload as a value reads that as the zero payload with no errors, which is a success, so a mutation handler decodes its payload as a pointer beside the top-level `errors` and returns `toolutil.GraphQLTopLevelError` when the payload is absent. That is how a fine-grained token's refusal reaches the caller (next section).

## Fine-grained Personal Access Tokens

GitLab judges a fine-grained personal access token on GraphQL in a way REST callers never see, because GraphQL authorizes each object of an answer on its own. At 19.4.1 a type or mutation declares the fine-grained permissions it needs with a directive, at a boundary (project, group, user or instance), and a token is judged against them as follows (read from `lib/gitlab/graphql/authz/` and `app/graphql/types/base_object.rb` at `v19.4.1-ee`, and measured by the end-to-end suite's direct probes on a 19.4.1 instance):

| What the document reaches                                                                                                       | What GitLab answers a fine-grained token                                                                                                                                   |
| ------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| A declared object the grant reaches                                                                                             | The object                                                                                                                                                                 |
| A declared object the grant does not reach, at a nullable position                                                              | `null`, with no error                                                                                                                                                      |
| Items of a connection, or of a list whose type carries abilities, the grant does not reach                                      | The items are removed, with no error                                                                                                                                       |
| A non-null position the grant does not reach                                                                                    | Its null travels to the nearest nullable position above it                                                                                                                 |
| An enforced object type that declares nothing (`Namespace`, `BranchRule`, `CustomEmoji` among 862 on GitLab's own pending list) | `null`, or the items removed, for every fine-grained token whatever its grant                                                                                              |
| A declared mutation the grant does not reach                                                                                    | `200`, the field `null`, and one `errors[]` entry: `Access denied: This operation requires a fine-grained personal access token with the following ... permissions: [...]` |
| A mutation that declares nothing                                                                                                | Refused for every fine-grained token, with GitLab's generic resource-access error                                                                                          |
| A declared mutation whose payload type declares nothing                                                                         | The write **commits**, and the answer is `null`                                                                                                                            |
| An object that never resolves to the boundary its type declares (a group's work item, declared at the project boundary only)    | `null` for a read; a write on it commits and answers `null`                                                                                                                |

The REST asymmetry this server's authorization rests on, that a wrong "yes" surfaces as GitLab's own `403` on the one call that needed the permission ([ADR-0018](adr/adr-0018-authorization-admits-per-action-gating.md)), does not hold here: a wrong "yes" on GraphQL is an empty answer that reads as "nothing there". So `cmd/gen_action_grants` walks every document an action sends against what GitLab 19.4.1 records (`docs/development/gitlab-api-live.json`) and judges the **answer spine**, the objects from the root field down to the first one that selects more than one field, as part of the action's requirement:

- An undeclared type on the spine, or an undeclared mutation, or a payload that commits and answers null, makes the action **withheld** from every fine-grained session, with the reason. 58 actions are withheld this way at 19.4.1, all through GraphQL; withholding the writes whose payload is undeclared also keeps an assistant from repeating a write it read as not done.
- An abstract position (a union or an interface) is judged as its worst member, since GitLab authorizes the type each item resolves to.
- A declared position a non-null chain carries onto the spine is judged as part of it, since denying it nulls the spine.
- Anything else denied is **degraded**: the action is served, and the answer carries a note naming each part GitLab leaves empty, written as the GraphQL selection that reaches it (`vulnerability { issueLinks { nodes } }`), and the permission a declared one needs.

A not-found answer of an action that reads GraphQL, and an empty list from one whose answer is a GraphQL list, carry a note too, since either can be the grant rather than the data. See [Fine-grained Tokens](https://jmrp.io/docs/gitlab-mcp-server/operations/fine-grained-tokens/#what-an-empty-answer-can-mean) and [Fine-grained Permissions](https://jmrp.io/docs/gitlab-mcp-server/reference/fine-grained-permissions/), which prints the spine verdict of every action.

## Shared Utilities

The `internal/toolutil/graphql.go` module provides shared GraphQL infrastructure:

| Type/Function                      | Purpose                                                                                                             |
| ---------------------------------- | ------------------------------------------------------------------------------------------------------------------- |
| `GraphQLPaginationInput`           | Forward-only pagination input; `Variables(document)` refuses a document that cannot carry every variable it sends   |
| `GraphQLCursorPaginationInput`     | Adds `last`/`before` for connections GitLab paginates in both directions, checked against the document the same way |
| `GraphQLPaginationOutput`          | Normalized pagination output for tool responses                                                                     |
| `GraphQLForwardPaginationOutput`   | The same, without the previous-page half, for a forward-only connection                                             |
| `GraphQLRawPageInfo`               | Raw camelCase page info from API responses                                                                          |
| `PageInfoToOutput()`               | Converts raw page info to output format                                                                             |
| `PageInfoToForwardOutput()`        | Converts raw page info, dropping the previous-page half                                                             |
| `FormatGraphQLPagination()`        | Renders pagination as Markdown summary                                                                              |
| `FormatGraphQLForwardPagination()` | Renders it without naming a previous page                                                                           |
| `FormatGID()`                      | Builds a GitLab GID string                                                                                          |
| `ParseGID()`                       | Extracts type and ID from a GID string                                                                              |
| `MergeVariables()`                 | Merges multiple variable maps                                                                                       |
| `GraphQLTopLevelError()`           | Wraps the top-level `errors` array of a GraphQL response                                                            |
| `GraphQLMutationError()`           | Wraps a mutation payload's `errors` array as one error                                                              |

## Testing GraphQL Tools

GraphQL tools are tested using `httptest` mocking at the `/api/graphql` endpoint. The `testutil` package provides:

- `testutil.GraphQLHandler(map[string]http.HandlerFunc)` — routes GraphQL requests by matching query strings
- `testutil.RespondGraphQL(w, status, dataJSON)` — wraps response in `{"data": ...}` envelope

The mock no longer answers what GitLab would refuse: every document sent through a `testutil.NewTestClient` client is validated against the pinned GitLab schema first. See [The pinned schema](#the-pinned-schema) below.

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

## The pinned schema

A mock stands in for GitLab, so it has to refuse what GitLab refuses. Ours did not. Every GraphQL test answers the request from an `httptest` handler that returns whatever the test wrote, so a passing test proved that our handler agreed with our own fixture and said nothing about whether the document was one any instance would accept. Four registered tools shipped documents `https://gitlab.com/api/graphql` rejects outright, with every test green, and the same blindness let eight domains advertise a backward pagination that no operation declared.

GitLab is the only party that refuses a document, and no unit test may reach it, so the schema comes into the repository instead. `internal/graphqlschema` embeds a GitLab schema as SDL, alongside a `source.json` recording the instance it came from, the version that instance reported, and the day it answered. `cmd/gen_graphql_schema` produces both from a live introspection; `make check-graphql-schema` gates the committed pair.

The SDL is committed as text, close to a megabyte of it, rather than compressed. A re-pin is the one moment somebody needs to read what GitLab changed, and a gzip blob renders as `Bin 0 -> 155364 bytes`, cannot be merged when two branches re-pin, and cannot be grepped by anyone verifying a repair. Compression does not even save history: git zlib-compresses and deltas a text blob and can do neither to a gzip stream, so two revisions of the compressed form cost about twice the pack of two revisions of the text. Nothing weighs the other way, because the schema never reaches a released binary; `cmd/server` does not depend on `internal/graphqlschema` at all, only the test helpers and the two commands do.

Two things read it, and they judge different halves of the same question.

### The test transport

`internal/testutil.NewTestClient` wraps the mock every domain test already passes it. For a POST to `/api/graphql` it reads the body and validates both the document and the variables against the pinned schema before the mock answers:

- the **document** half catches a field that does not exist, an argument the field does not accept, a selection set that is missing or forbidden, and a variable used where its declared type does not fit;
- the **variables** half catches a variable the request sends that the operation never declares, which is invisible to the document half and is exactly the shape of the backward-pagination defect, and a value that does not fit the type it was declared as;
- an **enum value in the wrong case** is refused too, and that check is ours rather than the validator's. `gqlparser` compares an enum value with `strings.EqualFold`, so it accepts `critical` for `VulnerabilitySeverity`, while GitLab answers `Expected "critical" to be one of: INFO, UNKNOWN, LOW, MEDIUM, HIGH, CRITICAL` and executes nothing. `Validate` therefore walks the variables against their declared types itself, through list elements and input-object members, and reports every enum position whose value differs from a schema value only in case. It matters because every enum this server sends is uppercased somewhere by a `strings.ToUpper` that a refactor could turn into `strings.ToLower`, and nothing else in the repository would notice.

A query that carries a file is judged the same way. The SDK sends it as the multipart form the [GraphQL multipart request specification](https://github.com/jaydenseric/graphql-multipart-request-spec) defines, with the document in an `operations` part, `null` left where each upload variable was, and the bytes in parts of their own, so a gate that only understood the JSON envelope waved it through. That exempted exactly one thing: the avatar an achievement is created and updated with, the only mutation this server sends with a file attached, and therefore the only one whose variables the SDK rewrites before they reach GitLab. The transport reads the `operations` part when the body is multipart, and a body it finds no document in is still left alone, since a test posting something else to this path is testing the transport rather than a document.

A refusal is reported with `t.Errorf`, naming the operation and its root field, every reason, and the pin that judged it, so a reader can tell a wrong document from a stale pin. The request then proceeds so the test's own assertions still run and still report. It never calls `t.Fatal`: this runs on the httptest server's goroutine, where an abort would terminate the wrong goroutine.

Every GraphQL test the repository already had became a document validator at no cost to the tests themselves. `testutil.AllowInvalidGraphQL(t)` exempts a test that sends a malformed document on purpose, and belongs nowhere else: a document the pinned schema refuses is a document GitLab refuses. Declare it on the test that calls `NewTestClient`, not on a subtest under one: the exemption is keyed by the name of the test the validation reports against, which is the test the client belongs to.

### The static audit

A document no test drives still ships, so the transport alone is not enough. `make check-graphql-documents` (`cmd/audit_graphql_documents`) reads every raw document out of the source and validates it against the same schema. It loads the program with `go/packages` rather than matching text, because several documents are assembled by concatenating a shared fragment constant and only the type checker knows what the assembled value is. Documents that live in `.graphql` files are read straight off disk beside them, because a `go:embed` variable is not a constant and folds to nothing.

It cannot check variables: a document read out of the source has no request behind it.

It reads `./internal/...`, which holds every document this repository writes, and does not read client-go, which builds another 42 of its own for the achievements, work item, security attribute and terraform state services among others. Those reach GitLab through this server too, and the only thing judging them is the test transport, on whichever ones a test happens to drive. The audit's count is this repository's documents, not the server's whole GraphQL surface.

### What a document costs

A document the schema accepts can still be one GitLab refuses. Before it runs anything, GitLab computes a complexity for the query from the document and its variables and refuses the whole query above a limit: 250 for an authenticated user, 300 for an administrator, 200 with no credential (the `*_MAX_COMPLEXITY` constants of GitLab's `GitlabSchema`). Every field costs one plus its children, a field that calls Gitaly costs one more, and a resolver may charge per item it can load: the work item discussions connection costs its contents six times over at a page of a hundred. The schema says none of this, so neither the transport nor the audit can see it: while issue 968 was in review, the epic notes query was widened to a cost of 274, which GitLab refuses on every read, with every test green. The licensed end-to-end run would not have caught it either, because its user is an administrator and gets the 300.

`testutil.GitLabQueryComplexity` runs GitLab's computation (legacy mode of graphql-ruby's `QueryComplexity` with `Types::BaseField`'s field costs) over the pinned schema, with the costs the schema does not carry held in a small table, each entry naming the Ruby that sets it. It is an estimate, because that table holds only the costs a measurement has needed, so a test using it holds the estimate equal to a figure GitLab reported for the same document as well as under the limit: a change to the selection changes the estimate, fails the test, and sends whoever made it to measure again. Measuring needs no credential, since GitLab reports the figure in its refusal (`Query has complexity of 274, which exceeds max complexity of 200`) or, below 200, in a `queryComplexity { score }` selection, whose own cost is 2. The epic notes and epic discussions list documents are held this way, at 220 each.

### What the pin cannot see, and what closes the gap

The schema is a snapshot, and GitLab narrows fields in place. `securityReportFindings` used to accept a `confidence` argument; `Project.vulnerabilities.severity` used to be typed `[String!]`. Both were valid when they were written. A document these gates accept is one the pinned instance accepted on the day `source.json` records, which is a far stronger statement than the mocks used to make and still not the same as one a live instance accepts today.

Four ways of aging are handled, and one is deliberately not.

**The pin is held to what it claims to be.** `make check-graphql-schema` no longer only asks whether the bytes parse. It refuses a record naming an instance other than `https://gitlab.com/api/graphql`, one carrying fewer than 4000 types (a truncated introspection, or a Community Edition instance, which has none of the Ultimate types the vulnerability and security finding documents select), one with no recorded GitLab version (what an introspection produces without a gitlab.com credential, since GitLab tells only an authenticated caller which version it runs), and one older than the staleness window. Until that existed, a re-pin from a self-managed instance passed every gate in the repository in silence, and the guarantee the whole gate rests on could be swapped out by one flag.

**Staleness is a gate, not a note in a file.** How long a pin may stand, and why that length, are recorded once in [`cmd/internal/provenance`](cmd-utilities.md#cmdinternalprovenance), which is where the three commands that pin something out of `gitlab-org/gitlab` share that one decision. Re-pin with `GITLAB_URL=https://gitlab.com GITLAB_TOKEN=<a gitlab.com token> make gen-graphql-schema`. Both are needed: the generator sends the token only to the instance `GITLAB_URL` names, so a token set alone is withheld, the version is recorded as unknown, and the check refuses the pin.

**A narrowing can be caught on the day it happens.** `make check-graphql-documents-live` introspects an instance right now and judges every document against what that instance serves instead of against the pin, which is the one check that reports a field GitLab removed since the pin was taken. It needs the network, so it is not a CI gate; it runs beside the other live suites under `make test-e2e-gitlab-com`, and `GRAPHQL_LIVE_URL` points it anywhere else. The same run names every type, field and argument the two schemas disagree about **under our own selection sets**, an enum's values and a union's or interface's members included, which is what makes the pin's age a number a reader sees rather than an assumption; whole-schema drift between two GitLab releases is thousands of lines and says nothing. It asks the same of the documents client-go builds, in a section of its own, because they reach GitLab through this server as much as ours do and a re-pin that changes what only they read is otherwise read by hand: the 19.5 re-pin added `BUSINESS_LOGIC` to the scan type client-go's scan profile document selects and no report said so. Five of the six client-go documents written as a `text/template` or a printf format are rendered from client-go's own source before they are walked, and the work item list, whose text a caller decides, is named as not walked, as is any client-go document neither schema accepts. Running it with `-schema` against a candidate SDL before a re-pin lands is how the re-pin is read.

**A self-managed release is checked without a license.** `.github/workflows/ee-schema.yml` runs that same re-probe weekly against an unlicensed `gitlab/gitlab-ee:latest`. GitLab builds its GraphQL schema when the process boots and a license is applied afterwards, so an unlicensed instance serves the whole Enterprise schema: 4233 types, `Vulnerability`, `Epic`, `MergeTrain` and `ComplianceFramework` among them. That is what puts the Premium and Ultimate documents, which no CI job compiles a test for, in front of a real GitLab of the version a self-managed instance actually runs. See [Enterprise Schema Checks](enterprise-schema-checks.md).

**The floor is gitlab.com on the pinned day, a pre-release ahead of every self-managed instance.** The check accepts a pin of gitlab.com and of nothing else, and that is a choice rather than a limit: an unlicensed `gitlab-ee` image serves the whole Enterprise schema too, as the paragraph above says, but the gate promises what the instance this server targets accepts, and a pin of another instance would swap that promise for a different one. gitlab.com always runs the next minor's pre-release, so the version `source.json` records is one no self-managed instance runs yet, and one minor ahead of the released `gitlab-ee` image the REST record in `docs/development/gitlab-api-live.json` is taken from: the re-pin of 2026-09-27 took gitlab.com at 19.5.0-pre while the REST record named 19.4.1-ee. The two name the same minor only once the REST record is taken from the release the GraphQL pin anticipates. Being ahead costs something in each direction. A field the pre-release adds is accepted here and refused by every released instance; the weekly re-probe above is what catches a document that selects one, against the release that shipped last, and a domain whose document needs a recent field states the oldest GitLab it works on, as the vulnerability and security finding pages do. A field GitLab removes leaves gitlab.com first, so the pin refuses a document that stopped working there while it still works on every self-managed instance: measured on 2026-09-07 against the 19.4.0-pre pin, 7 types, 44 fields and 17 arguments existed on a self-managed instance of that week and were already gone from gitlab.com, `Analytics.finishedPipelines` among them. Such a document fails this gate with no way to declare why, and no exemption mechanism exists because no document has needed one: the weekly re-probe of 2026-09-24 accepted all 38 documents `main` then carried against gitlab-ee 19.4.1. If one ever does, the fix is a declared exemption in the shape `cmd/audit_1to1` already uses, not a suppression.

Four limits are inherent to validating at this layer rather than at GitLab's, and none is currently reachable by a shipped document. A fractional number passes where `Int` is declared, because JSON has already made every number a float64 by the time the value is seen. A custom scalar accepts anything, so a malformed global id passes as a `VulnerabilityID`. No depth or complexity limit is enforced, so a query GitLab would refuse as too expensive is accepted. And the SDL carries no deprecation marks, so the gate reports a document that is already broken and never one that is about to be; four fields the server selects are flagged experimental on gitlab.com today, and surfacing that is worth doing when the pin is next regenerated with a credential.

**The response is judged too.** The gates above read the request, and a decoder that disagrees with GitLab is invisible to both halves and to the test, since the fixture is written to match the decoder: the licensed e2e run found `startLine` and `endLine` declared as `String` by the schema and decoded into `int` by two response structs, with every gate green. `make check-graphql-shapes` (`cmd/audit_graphql_shapes`) pairs every document with the struct the `Do` call decodes it into, following the document through local variables, wrapper parameters and generic wrappers, and walks the validated selection set against that struct under `encoding/json`'s rules. It fails on a Go kind that cannot hold what the schema says GitLab sends, on a Go field the document never selects (a shared node struct decoding a document that selects less, which left a list claiming an empty description for every vulnerability), and on a scalar its serialization table does not name; a selection no Go field reads is reported and does not fail. A document it cannot pair is a failure rather than a silence.

**And the same walk asks what we never asked for.** Every gate above, and every dimension of the 1:1 audit, describes what this server publishes; the one that asks what GitLab sends and we drop reads GitLab's REST record, where the eleven GraphQL-only domains do not appear at all. `make audit-graphql-sent` writes the missing half to `plan/graphql-sent.json`: at every object one of our decoders reads, the fields the pinned schema offers that **no document of the package decoding it** ever selects. The package is the grain of the claim because a package sends several documents at one object and they differ on purpose — a CE document omits a Premium field and the EE document beside it selects it — so a per-document answer reports a deliberate tier expression as a missing field. It is bounded by our own decoders rather than by the schema graph: the object must be one a Go struct decodes, must not be the operation root (whose fields are other requests), and must be read rather than merely traversed. The report says how far that reached, counting the positions asked about apart from the positions reached and skipped, and it reports without gating, because a field GitLab offers is a candidate for the surface rather than a defect in it. Two annotations a reader of the REST list expects are missing and the report says so on itself: the schema declares no tier, since GitLab gates a GraphQL field at resolve time, and the pin carries no deprecation mark for the reason above, so a field GitLab has deprecated reads here as a gap. One sub-class does gate, on every run: a mutation payload whose `errors` no field of the decoder reads, which drops GitLab's account of a refused mutation and lets the tool report success — the condition is the decoder, because a payload that asks GitLab for its errors and decodes none loses them just as completely.

**What it does not cover, and says so.** The walk loads `./internal/...`, the same tree `check-graphql-documents` reads, so it sees only the documents this repository's source writes. The operations client-go builds have both halves inside the SDK's module — no send here to pair, no decoder here to compare a schema type with — and a GraphQL-only endpoint has no REST operation for R-PATH to see either, which leaves them exactly where this dimension found the rest. The report names that set rather than leaving a reader to infer it, reading it from the committed request inventory: today 38 operations in 7 packages (`achievements`, `workitems`, `epics`, `workitemsavedviews`, `projects`, `securityscanprofiles`, `terraformstates`).

## References

- [GitLab GraphQL API Reference](https://docs.gitlab.com/ee/api/graphql/reference/)
- [GitLab GraphQL Explorer](https://docs.gitlab.com/ee/api/graphql/#interactive-graphql-explorer)
- [client-go GraphQL.Do()](https://pkg.go.dev/gitlab.com/gitlab-org/api/client-go/v3#GraphQL.Do)
- [ADR-0006: Raw GraphQL.Do() for Uncovered Domains](adr/adr-0006-raw-graphql-for-uncovered-domains.md)
