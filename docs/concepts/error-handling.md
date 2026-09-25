# Error Handling

This document describes the error handling architecture in gitlab-mcp-server. All error types, classification logic, and formatting utilities live in `internal/toolutil/`.

> **Diátaxis type**: Explanation
> **Audience**: 🔧 Developers, contributors
> **Prerequisites**: Go programming, understanding of MCP tool handlers
> 📖 **User documentation**: See the [Error Handling](https://jmrp.io/docs/gitlab-mcp-server/operations/error-handling/) on the documentation site for a user-friendly version.

---

## Error Types

### ToolError

The basic structured error for tool handlers:

```go
type ToolError struct {
    Tool       string `json:"tool"`
    Message    string `json:"message"`
    StatusCode int    `json:"status_code,omitempty"`
}
```

Use when a tool handler needs to report a typed error with optional HTTP status context.

### DetailedError

A richer error type with domain context for diagnostic output:

```go
type DetailedError struct {
    Domain       string `json:"domain"`
    Action       string `json:"action"`
    Message      string `json:"message"`
    Details      string `json:"details,omitempty"`
    GitLabStatus int    `json:"gitlab_status,omitempty"`
    RequestID    string `json:"request_id,omitempty"`
}
```

Created via `NewDetailedError(domain, action, err)` which automatically:

- Classifies the error into a human-friendly message
- Extracts the HTTP status from GitLab API error responses, REST and GraphQL alike, and the X-Request-Id from each one that carries its response. A 404 carries none: client-go answers every 404 with its `ErrNotFound` sentinel, which records the status alone, so the card for a 404 has the status and no request ID
- Safely handles nil response bodies (the GitLab client can panic on `.Error()`)

## Error Classification

### ClassifyError

Inspects the error chain and returns a diagnostic message:

| Error Type           | Message                                                                                                            |
| -------------------- | ------------------------------------------------------------------------------------------------------------------ |
| GitLab HTTP response | Delegates to `ClassifyHTTPStatus`, over REST and GraphQL alike, except a 401 that names the credential (see below) |
| Connection refused   | "GitLab server is unreachable (connection refused). Check GITLAB_URL and whether the server is running"            |
| DNS failure          | "GitLab server hostname could not be resolved (DNS error). Check GITLAB_URL"                                       |
| Timeout              | "Request to GitLab timed out. The server may be overloaded or unreachable"                                         |
| TLS/SSL              | "TLS/SSL handshake failed. If using self-signed certificates, set GITLAB_MCP_SKIP_TLS_VERIFY=true"                 |
| URL error            | "network error reaching GitLab (\<op\>)"                                                                           |
| Other                | "unexpected error"                                                                                                 |

A 404 is one of those GitLab responses although client-go hands it back without the response: it answers every 404 with its `ErrNotFound` sentinel, over REST as it is and over GraphQL wrapped in the query error, and the sentinel records the status alone. `ClassifyError` reads the status off the error when it carries no response, so a 404 on either surface is described as not found rather than as an unexpected error.

### ClassifyHTTPStatus

Maps HTTP status codes to actionable guidance:

| Code | Message                                                                                                                                                                                                                                                                                  |
| ---- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 400  | "bad request: check your input parameters"                                                                                                                                                                                                                                               |
| 401  | "unauthorized: either the token (GITLAB_TOKEN) is invalid or expired, or it is valid and lacks a permission this action needs, since some GitLab endpoints answer a missing permission with 401 rather than 403. If the token works for other calls, treat this as a permission refusal" |
| 403  | "access denied: your token lacks the required permissions. This can mean: (1) missing API scope on the token, (2) insufficient project role (some operations require Maintainer or Owner), or (3) the feature is restricted by instance admin settings"                                  |
| 404  | "not found: the requested resource does not exist, you lack access, or the feature requires a higher GitLab tier. Verify the ID/path is correct"                                                                                                                                         |
| 405  | "method not allowed: the action cannot be performed on this resource in its current state"                                                                                                                                                                                               |
| 409  | "conflict: the resource already exists or there is a state conflict"                                                                                                                                                                                                                     |
| 422  | "validation failed: GitLab rejected the request due to invalid data"                                                                                                                                                                                                                     |
| 429  | "rate limited: too many requests, please wait before retrying"                                                                                                                                                                                                                           |
| 500  | "GitLab internal server error: the server encountered an unexpected condition"                                                                                                                                                                                                           |
| 502  | "GitLab is temporarily unavailable (bad gateway): try again shortly"                                                                                                                                                                                                                     |
| 503  | "GitLab is under maintenance or overloaded (service unavailable): try again shortly"                                                                                                                                                                                                     |

### Why a 401 names two causes

GitLab answers 401 for two different things. Its API guard answers it for a credential it cannot use, and at a family of REST routes GitLab's own API helper `unauthorized!` answers it for a **valid** credential that lacks a permission: merging, cancelling auto-merge, approving and resetting approvals, adding to a merge train, remote mirrors, access token reads, lists and rotation, external status checks, security settings, group SAML links, award emoji removal, group updates and fork links. Approving a merge request you opened, on an instance that prevents approval by the author, is the common one. The upstream half is [entry 55 of the upstream bugs register](../development/upstream-bugs.md#a-permission-refusal-is-answered-401-rather-than-403).

The status cannot tell the two apart, so `ClassifyHTTPStatus(401)` names both and ends with the test that separates them: if the same token works for other calls, the 401 is a permission refusal. It opens with "unauthorized" rather than "authentication failed", because for a permission refusal authentication succeeded.

`ClassifyError` has the whole response, and narrows the answer in one direction only. It describes a 401 as a rejected credential ("authentication failed: GitLab rejected the token (GITLAB_TOKEN) itself as invalid, expired, revoked or without the api or read_api scope, so renew or replace it") when:

- the body carries the RFC 6750 code `invalid_token`, which GitLab's REST API guard writes for an expired, revoked or impersonation-disabled token and nothing else in the REST API writes. The code is a REST signal only.
- the GraphQL endpoint answered it. That endpoint answers 401 only from its authentication checks, with `{"errors":[{"message":"Invalid token"}]}` and no code, and refuses a field the caller may not see with a 200, so a GraphQL 401 has no permission refusal to be confused with. One of those checks is the scope: the endpoint authenticates a token only when it carries `api` or `read_api`, and answers one carrying neither with that same body, where REST answers it 403 `insufficient_scope`, which is why the sentence names the scope. The endpoint is recognised by the request path as sent, escaped, so a REST path parameter that decodes to `api/graphql` does not pass for it. client-go returns such an answer as `*gl.GraphQLResponseError`, which does not unwrap to the response, so `ClassifyError` looks through it to find the status.

The opposite verdict cannot be read off a response: a token GitLab has no record of at all is answered through `unauthorized!` too, byte for byte like a permission refusal, so a REST 401 without the code keeps the sentence that names both causes.

What the handler adds after that sentence is the permission itself, and it is keyed on the same reading rather than on the status: see [A permission hint on a 401](#a-permission-hint-on-a-401).

The rule is not `ClassifyError`'s own. It is `UnauthorizedNamesCredential` in `internal/gitlab/credential_refusal.go`, and it has a second reader: in HTTP mode the client's innermost transport applies it to every 401 GitLab answers a request the SDK makes (the credential probe's own answer is read by status alone and never reported, so confirming a 401 cannot raise another), and the credential pool ends a caller's pooled entry at once only when it says the credential was refused. A 401 it cannot place is confirmed with `GET /api/v4/user` first, and the entry is kept when GitLab still accepts the token (see [Refused calls](../guides/http-server-mode.md#refused-calls)). One rule is what keeps the two from disagreeing, so a permission refusal is never described to a model as a missing permission while the pool treats it as a revoked token. The `DetailedError` card carries the reading once, in its message: the HTTP Status row is the code and its reason phrase (`401 Unauthorized`) and nothing more, for a REST and a GraphQL 401 alike, because a second classification of the status alone would put the sentence naming both causes beside a message that has already ruled one of them out.

## Error Flow in Tool Handlers

### Standard Pattern

Every tool handler follows the triple-return convention:

```go
func handler(ctx context.Context, req *mcp.CallToolRequest, input T) (*mcp.CallToolResult, OutputType, error)
```

- **Success**: `return ToolResultWithMarkdown(md), output, nil`
- **Error**: `return nil, zero, WrapErr("operation_name", err)` (read-only) or `WrapErrWithMessage("operation_name", err)` (mutating)

### WrapErr

The basic error enrichment function for **read-only** operations (list, get, search). Classifies the error and wraps it with the operation name:

```go
err := WrapErr("list_issues", originalErr)
// Result for an expired token: "list_issues: authentication failed: GitLab rejected the token (GITLAB_TOKEN) itself as invalid, expired, revoked or without the api or read_api scope, so renew or replace it: <original>"
```

### ExtractGitLabMessage

Extracts the specific error detail from a `*gl.ErrorResponse.Message` field in the error chain. Handles nested formats like `{message: {base: [text]}}`, filters out messages that merely restate the HTTP status code, and truncates at 300 characters:

```go
msg := ExtractGitLabMessage(err)
// Example: "A file with this name already exists"
// Example: "[title is too long (maximum is 255 characters)]"
// Returns "" if no useful detail is available
```

### WrapErrWithMessage

Like `WrapErr` but also includes the specific GitLab error message when available. **Recommended for mutating operations** (create, update, delete) where the specific error detail helps the LLM understand what went wrong:

```go
err := WrapErrWithMessage("fileCreate", originalErr)
// Result: "fileCreate: bad request: check your input parameters (A file with this name already exists): POST .../files: 400"
// Falls back to WrapErr format when glErr.Message adds no useful detail
```

### WrapErrWithHint

Like `WrapErrWithMessage` but appends an actionable suggestion that tells the LLM what to do next. Use when you know the corrective action for a specific error scenario:

```go
if toolutil.IsHTTPStatus(err, 409) {
    return toolutil.WrapErrWithHint("branchProtect", err,
        "protected branch rule already exists, use branch.get_protected to view current rules")
}
// Result: "branchProtect: conflict: the resource already exists or there is a state conflict (Protected branch rule already exists).
//          Suggestion: protected branch rule already exists, use branch.get_protected to view current rules: <original>"
```

### WrapErrWithStatusHint

Convenience wrapper that compresses the dominant single-status pattern into one call. Returns `WrapErrWithHint` when the error matches the requested HTTP status, otherwise falls back to `WrapErrWithMessage`:

```go
// Equivalent to:
//   if toolutil.IsHTTPStatus(err, 404) {
//       return toolutil.WrapErrWithHint("issueGet", err, "verify issue_iid with issue.list")
//   }
//   return toolutil.WrapErrWithMessage("issueGet", err)
return toolutil.WrapErrWithStatusHint("issueGet", err, 404,
    "verify issue_iid with issue.list")
```

For handlers that need different hints per status code, use a `switch` over `IsHTTPStatus` checks instead — each branch carries genuinely different context.

### A permission hint on a 401

A hint that names a role, a license or an owner is keyed on `IsPermissionRefusal(err)`, never on a status alone. At the routes [entry 55](../development/upstream-bugs.md#a-permission-refusal-is-answered-401-rather-than-403) lists, GitLab refuses a missing permission with 401, so a hint scoped to 403 is never shown there; and a hint keyed on 401 alone follows the verdict that GitLab rejected the token itself, which it then contradicts. `IsPermissionRefusal` is true for a REST 401 or 403 whose body carries no RFC 6750 error code, which is what GitLab's API helpers `unauthorized!`, `forbidden!` and `render_api_error!` render, and false for every GraphQL answer and for both ways the API guard refuses the credential rather than the call. Its token and scope refusals carry a code (`invalid_token`, `dpop_error` and `restricted_language_server_client_error` on a 401, `insufficient_scope` and `insufficient_granular_scope` on a 403). An account the API will not serve (blocked, deactivated, pending approval, an internal user, the Terms of Service not accepted, the primary email unconfirmed, the password expired) it refuses with a plain 403 `forbidden!` that carries no code, so that answer is told apart by its message, one of the fixed reasons of `lib/gitlab/auth/user_access_denied_reason.rb`. The guard also maps a missing token to rack-oauth2's default code `unauthorized`, but nothing in GitLab raises that error: a request with no token gets `authenticate!`'s plain 401, which the predicate reads as a possible permission refusal, and that is harmless here because this server always sends a token. The rule is `RefusalMayBePermission` in `internal/gitlab/credential_refusal.go`, beside the rule that decides the description, so the two cannot drift apart. Two kinds of action are not keyed on it: the four group SAML link actions add their hint to every error, a rejected token included, because the licensed end-to-end suite quotes that wording, and the two self-rotations keep a 401 hint about the calling token, which agrees with the verdict that the token was refused rather than contradicting it (see entry 55).

```go
if toolutil.IsPermissionRefusal(err) {
    return toolutil.WrapErrWithHint("mrMerge", err,
        "merging needs the right to push to the merge request's target branch (read it with branch.get_protected)")
}
return toolutil.WrapErrWithMessage("mrMerge", err)
```

It is a predicate and not another wrapper on purpose: `make check-action-ids` reads a hint where it is passed to `WrapErrWithHint`, `WrapErrWithStatusHint` or `NotFoundResult`, so a hint passed through a new wrapper would escape the gate unless the wrapper takes it under a parameter named for a hint.

### Every sentence a model reads names actions by their canonical ID

A hint is not the only prose a model reads. The message of an error a handler returns (`errors.New`, `fmt.Errorf`), the refusal `toolutil.ErrorResult` or `toolutil.CancelledResult` answers with, a field named for a message, the next steps a formatter writes through `toolutil.WriteHints`, `WriteListFooter` or a card's `End`, the `NextSteps` a meta tool's JSON carries, a spec's `Usage` line and parameter guidance (`ValueSource`, `CommonConfusions`), and the description of every input and output field, whether a `jsonschema` tag or the `description` entry of a schema written as a map (`toolutil.SchemaPropertyOverride`), all reach it, and all of them are served on every surface. A `gitlab_*` tool name in any of them is right for one surface of three: the default dynamic surface registers only `gitlab_find_action` and `gitlab_execute_action`, and meta registers the bare domain tools. So every one of them names an action by its canonical ID, `project.list` rather than `gitlab_project_list`, which each surface resolves:

```go
return Output{}, errors.New("commitCreate: project_id is required. Use project.list to find the ID first, then pass it as project_id")
```

`make check-action-ids` reads these and fails on a tool name, a registered alias or an ID that resolves nowhere. Two places may name a tool: `internal/tools/dynamic`, whose text is returned only by the two tools of the surface that registers them, so it may name those two, and the "See also" clause of a domain action's individual tool `Description`. The rest of that description is read like a `Usage` line, because `gitlab://tools` serves it verbatim on the dynamic and meta surfaces too and rewrites only the clause into each surface's names. A standalone surface tool's description is no such place: the guided flows and `discover_project.resolve` are registered on meta and individual alike and served as their `Usage` on dynamic, so each writes its one text as its `Usage` too, where the rule reads it, and names actions by canonical ID throughout. What the rule does not read is stated with it rather than absorbed ([its limits](../development/cmd-utilities.md#the-rule-over-served-prose)): the operation label a `WrapErr*` or `ErrRequired*` call puts in front of an error, which is sometimes spelled as a tool name (`ErrRequiredString("gitlab_get_project_statistics", "project_id")`) and names what failed rather than inviting a call; a value a format reports, which it passes over and counts; a bare meta action name (`Use action 'list'`), which carries neither the `gitlab_` prefix nor a dot; and `internal/prompts` and `internal/resources`, which are outside its load.

A route whose 403 means something other than the permission its 401 refuses pairs the predicate with `IsHTTPStatus` to tell them apart. The external status check routes of a merge request answer the license with 401 and the caller's role on the merge request with 403; the fork link answers the target namespace with 401 and the caller's abilities with 403, which are the Owner role on the target together with the right to create forks in its top-level namespace, and the right to fork the source; the security settings answer the role with 401 and the license, an archived project or an instance-enforced setting with 403, so that handler reads the 403 first.

### Error Function Decision Tree

| Scenario                                    | Function                                  | Example                                                                          |
| ------------------------------------------- | ----------------------------------------- | -------------------------------------------------------------------------------- |
| Read-only operation (list, get, search)     | `WrapErr`                                 | `WrapErr("listBranches", err)`                                                   |
| Get operation returning 404                 | `NotFoundResult`                          | `NotFoundResult("Branch", "main in project 42", "Use branch.list...")`           |
| Mutating operation (create, update, delete) | `WrapErrWithMessage`                      | `WrapErrWithMessage("fileCreate", err)`                                          |
| Specific error with known corrective action | `WrapErrWithHint`                         | `WrapErrWithHint("branchDelete", err, "use branch.unprotect first")`             |
| Single-status hint (the common case)        | `WrapErrWithStatusHint`                   | `WrapErrWithStatusHint("issueGet", err, 404, "verify issue_iid")`                |
| A permission refused with 401 or 403        | `IsPermissionRefusal` + `WrapErrWithHint` | `if toolutil.IsPermissionRefusal(err) { WrapErrWithHint("mrMerge", err, hint) }` |

### NotFoundResult — Informational 404 Responses

For "get" handlers, HTTP 404 errors are intercepted **before** the standard error flow and returned as structured, informational results instead of opaque Go errors. This improves the LLM experience: instead of a raw error, the assistant receives an `IsError: true` result with a human-readable explanation and domain-specific next-step hints.

```go
// In the domain's action_specs.go, wrapping the get route: a 404 becomes a
// typed not-found output returned with a nil error.
route := toolutil.RouteAction(client, Get)
baseHandler := route.Handler
route.Handler = func(ctx context.Context, input map[string]any) (any, error) {
    result, err := baseHandler(ctx, input)
    if err != nil && toolutil.IsHTTPStatus(err, http.StatusNotFound) {
        branchName, _ := input["branch_name"].(string)
        projectID, _ := input["project_id"].(string)
        return branchNotFoundOutput{Identifier: fmt.Sprintf("%q in project %s", branchName, projectID)}, nil
    }
    return result, err
}

// In the domain's markdown.go, registered from init(): the registry turns
// that output into the informational error result.
func formatBranchNotFound(out branchNotFoundOutput) *mcp.CallToolResult {
    return toolutil.NotFoundResult("Branch", out.Identifier,
        toolutil.HintAction("branch.list", "list the project's branches"),
        "Verify the branch name is spelled correctly (case-sensitive)",
    )
}
toolutil.RegisterMarkdownResult(formatBranchNotFound)
```

The `NotFoundResult(resource, identifier string, hints ...string)` function in `internal/toolutil/not_found.go`:

1. Creates a Markdown-formatted `CallToolResult` with `IsError: true`
2. Includes a `## ❓ {Resource} Not Found` heading with the identifier
3. Appends `💡 Next steps` hints specific to the domain
4. The route returns a `nil` Go error, so the call is logged at INFO level rather than ERROR; `LogToolCallAll` receives the formatted result and reads `IsError` off it to stamp `is_error: true` on that record. Without that, every 404 the server turns into a helpful message would be counted as a success

This pattern is applied through one shared not-found formatter in each of **19 domains**: award emoji, badges, branches, dependency firewall, deployments, environments, files, groups, labels, merge requests, milestones, Orbit, pipelines, projects, releases, snippets, tags, users, and wikis. A domain's formatter covers every get variant it has (project and group badges, each award-emoji target), which is why the typed output carries the identifier and hints rather than the formatter hardcoding them.

### ErrorResultMarkdown

For errors that should be returned as tool results (with `IsError: true`) rather than Go errors:

```go
result := ErrorResultMarkdown("issues", "list", err)
```

Renders the `DetailedError` as a Markdown block with all diagnostic fields.

## Error Result Hygiene

Error results are designed for LLM self-correction without exposing request bodies or secrets. Handlers return structured diagnostics such as operation name, error class, HTTP status, GitLab request ID, and actionable hints. Input parameters are not copied into error Markdown.

## Network Error Helpers

Lower-level helpers detect specific network conditions:

| Helper                | Detects                                   |
| --------------------- | ----------------------------------------- |
| `isConnectionRefused` | ECONNREFUSED, "connectex:"                |
| `isDNSError`          | `*net.DNSError` in error chain            |
| `isTimeout`           | Any error implementing `Timeout() bool`   |
| `isTLSError`          | "tls:", "certificate", "x509:" in message |
| `ContainsAny`         | Generic substring match on `err.Error()`  |

## Parameter-Name Guidance Helpers

Meta-tool parameter parsing combines two complementary mechanisms to surface
actionable errors when LLMs mistype argument names:

1. **Strict unknown-key rejection** — `strictUnmarshal` in
   `internal/toolutil/meta_tool.go` decodes the `params` envelope with
   `json.Decoder.DisallowUnknownFields()`. Reserved meta keys (e.g. `confirm`)
   are stripped from the params map before unmarshalling; any other key that
   does not map to a field on the action's input struct produces an immediate
   error of the form `json: unknown field "foo"`. This prevents the silent
   drop-and-default behaviour of `encoding/json` and lets the LLM self-correct
   on misspellings.
2. **Required-field helpers** — once a key is accepted, two helpers detect
   missing required values and emit messages that name the exact documented
   parameter:

| Helper                         | Use Case                       | Example Output                                                                                                          |
| ------------------------------ | ------------------------------ | ----------------------------------------------------------------------------------------------------------------------- |
| `ErrRequiredInt64(op, field)`  | Required int64 field is 0      | `"milestoneGet: milestone_iid is required (must be > 0). Ensure you use the exact parameter name 'milestone_iid'..."`   |
| `ErrRequiredString(op, field)` | Required string field is empty | `"branchCreate: branch_name is required (must be non-empty). Ensure you use the exact parameter name 'branch_name'..."` |

Used in `milestones`, `branches`, `mergerequests`, and other domains where LLMs frequently confuse parameter names (e.g., `milestone_id` vs `milestone_iid`, `branch` vs `branch_name`, `iid` vs `merge_request_iid`).

## Destructive Action Confirmation

Before executing destructive operations (delete, force-push), handlers use the confirmation flow in `confirm.go`:

1. **GITLAB_MCP_YOLO_MODE / AUTOPILOT** env var set → skip confirmation
2. **Explicit `confirm: true`** in params → proceed
3. **MCP elicitation supported** → ask user interactively via `ConfirmAction()`; a decline or cancel returns `CancelledResult`
4. **No confirmation mechanism** → fail closed: return an `IsError` result asking the caller to re-send with `confirm: true` once the user has approved

## Testing Error Handling

When writing tests for error scenarios, use `http.StatusBadRequest` (400) instead of 500 for mock API errors. Status 500 triggers the `retryablehttp` client's retry loop, causing test hangs.

```go
// Correct: use 400 for error mocks in tests
testutil.RespondJSON(w, http.StatusBadRequest, map[string]string{
    "message": "Bad Request",
})
```

## File Reference

| File                                    | Purpose                                                                                                                                                                                                |
| --------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `internal/toolutil/errors.go`           | ToolError, DetailedError, WrapErr, WrapErrWithMessage, WrapErrWithHint, WrapErrWithStatusHint, ExtractGitLabMessage, ClassifyError, ClassifyHTTPStatus, IsHTTPStatus, IsPermissionRefusal, ContainsAny |
| `internal/gitlab/credential_refusal.go` | UnauthorizedNamesCredential and RefusalMayBePermission, the two readings of a refusal the description, the hints and the HTTP pool share                                                               |
| `internal/toolutil/not_found.go`        | NotFoundResult, the informational 404 pattern for get handlers                                                                                                                                         |
| `internal/toolutil/confirm.go`          | Destructive action confirmation flow                                                                                                                                                                   |
| `internal/toolutil/output.go`           | SuccessResult, ErrorResult helpers                                                                                                                                                                     |

## LLM Ergonomics Hint Rollout

Actionable hints were added across the entire codebase to help LLMs self-correct when API calls fail. The rollout converted `WrapErrWithMessage` calls to `WrapErrWithHint` (GraphQL) or `WrapErrWithStatusHint` (REST) with domain-specific suggestions.

### Coverage

| Metric                                         | Count                                |
| ---------------------------------------------- | ------------------------------------ |
| `WrapErrWithHint` call sites (GraphQL)         | 320                                  |
| `WrapErrWithStatusHint` call sites (REST)      | 879                                  |
| **Total hinted error sites**                   | **1,199**                            |
| `WrapErrWithMessage` (skip-category, retained) | 363                                  |
| `NotFoundResult` (informational 404s)          | 19 shared formatters, one per domain |
| `internal/tools` packages with hints           | 162 of 177                           |
| Source files with hints                        | 189                                  |

The call-site counts above are source-level counts from `grep` over `internal/` (non-test files, `internal/toolutil` itself excluded); package totals can be verified with `go list ./internal/tools/...`. `NotFoundResult` is counted by formatter because each shared formatter covers every get-handler variant of its domain.

### Skip Categories

The following `WrapErrWithMessage` calls were intentionally retained because the error originates from local operations, not from the GitLab API:

- **Input validation**: `ErrFieldRequired`, `ErrRequiredInt64`, `ErrRequiredString`
- **Body parsing**: `json.Unmarshal`, `io.ReadAll`, `io.ReadFull`, `os.ReadFile`, `base64.Decode`
- **Time parsing**: `time.Parse`
- **Local construction**: `NewRequest` (constructs HTTP request object locally)
- **Context cancellation**: `ctx.Err()`

### Hint Patterns

REST error sites use `WrapErrWithStatusHint` which checks a single HTTP status code and appends the hint only when matched, falling back to `WrapErrWithMessage` for other statuses:

```go
return toolutil.WrapErrWithStatusHint("issueGet", err, http.StatusNotFound,
    "verify issue_iid with issue.list")
```

GraphQL error sites mostly use `WrapErrWithHint`, which always appends the hint, because an error GitLab reports inside a `200` response carries no status to match:

```go
return toolutil.WrapErrWithHint("list_vulnerabilities", err,
    "verify the project fullPath is correct and your token has access to security features")
```

A GraphQL refusal GitLab answers with an error status does carry one: `IsHTTPStatus`, `ExtractGitLabMessage` and the sanitizer read it through client-go's `*gl.GraphQLResponseError` the way `ClassifyError` does, so `WrapErrWithStatusHint` attaches its hint there as it does over REST:

```go
return toolutil.WrapErrWithStatusHint("create_custom_emoji", err, http.StatusBadRequest,
    "verify group_path, name is unique, and url points to a valid image")
```

A 404 never arrives in that type: client-go answers it with the `ErrNotFound` sentinel before it reads a body, so a status hint on 404 over GraphQL matches through the sentinel described under [ClassifyError](#classifyerror) rather than through the wrapper.

That type's rendering carries more than the response's: after it, it appends `(GraphQL errors: ...)` listing every `errors[].message` of the body. The sanitizer swaps the whole of it, and holds the list to what a REST message is held to: flattened onto one line and capped at 300 characters as one list, and dropped altogether when the body carries a top-level key other than `data`, `errors` and `extensions`, since GitLab did not compose that body.
