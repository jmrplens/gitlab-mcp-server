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
- Extracts HTTP status and X-Request-Id from GitLab API error responses
- Safely handles nil response bodies (the GitLab client can panic on `.Error()`)

## Error Classification

### ClassifyError

Inspects the error chain and returns a diagnostic message:

| Error Type           | Message                                                                                                 |
| -------------------- | ------------------------------------------------------------------------------------------------------- |
| GitLab HTTP response | Delegates to `ClassifyHTTPStatus`                                                                       |
| Connection refused   | "GitLab server is unreachable (connection refused). Check GITLAB_URL and whether the server is running" |
| DNS failure          | "GitLab server hostname could not be resolved (DNS error). Check GITLAB_URL"                            |
| Timeout              | "Request to GitLab timed out. The server may be overloaded or unreachable"                              |
| TLS/SSL              | "TLS/SSL handshake failed. If using self-signed certificates, set GITLAB_SKIP_TLS_VERIFY=true"          |
| URL error            | "network error reaching GitLab (\<op\>)"                                                                |
| Other                | "unexpected error"                                                                                      |

### ClassifyHTTPStatus

Maps HTTP status codes to actionable guidance:

| Code | Message                                                                                                                                                                                                                                                 |
| ---- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 400  | "bad request: check your input parameters"                                                                                                                                                                                                              |
| 401  | "authentication failed: GITLAB_TOKEN may be invalid or expired"                                                                                                                                                                                         |
| 403  | "access denied: your token lacks the required permissions. This can mean: (1) missing API scope on the token, (2) insufficient project role (some operations require Maintainer or Owner), or (3) the feature is restricted by instance admin settings" |
| 404  | "not found: the requested resource does not exist, you lack access, or the feature requires a higher GitLab tier. Verify the ID/path is correct"                                                                                                        |
| 405  | "method not allowed: the action cannot be performed on this resource in its current state"                                                                                                                                                              |
| 409  | "conflict: the resource already exists or there is a state conflict"                                                                                                                                                                                    |
| 422  | "validation failed: GitLab rejected the request due to invalid data"                                                                                                                                                                                    |
| 429  | "rate limited: too many requests, please wait before retrying"                                                                                                                                                                                          |
| 500  | "GitLab internal server error: the server encountered an unexpected condition"                                                                                                                                                                          |
| 502  | "GitLab is temporarily unavailable (bad gateway): try again shortly"                                                                                                                                                                                    |
| 503  | "GitLab is under maintenance or overloaded (service unavailable): try again shortly"                                                                                                                                                                    |

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
// Result: "list_issues: authentication failed: GITLAB_TOKEN may be invalid or expired: <original>"
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
        "protected branch rule already exists — use gitlab_protected_branch_get to view current rules")
}
// Result: "branchProtect: conflict: the resource already exists or there is a state conflict (Protected branch rule already exists).
//          Suggestion: protected branch rule already exists — use gitlab_protected_branch_get to view current rules: <original>"
```

### WrapErrWithStatusHint

Convenience wrapper that compresses the dominant single-status pattern into one call. Returns `WrapErrWithHint` when the error matches the requested HTTP status, otherwise falls back to `WrapErrWithMessage`:

```go
// Equivalent to:
//   if toolutil.IsHTTPStatus(err, 404) {
//       return toolutil.WrapErrWithHint("issueGet", err, "verify issue_iid with gitlab_issue_list")
//   }
//   return toolutil.WrapErrWithMessage("issueGet", err)
return toolutil.WrapErrWithStatusHint("issueGet", err, 404,
    "verify issue_iid with gitlab_issue_list")
```

For handlers that need different hints per status code, use a `switch` over `IsHTTPStatus` checks instead — each branch carries genuinely different context.

### Error Function Decision Tree

| Scenario                                    | Function                | Example                                                                       |
| ------------------------------------------- | ----------------------- | ----------------------------------------------------------------------------- |
| Read-only operation (list, get, search)     | `WrapErr`               | `WrapErr("listBranches", err)`                                                |
| Get operation returning 404                 | `NotFoundResult`        | `NotFoundResult("Branch", "main in project 42", "Use gitlab_branch_list...")` |
| Mutating operation (create, update, delete) | `WrapErrWithMessage`    | `WrapErrWithMessage("fileCreate", err)`                                       |
| Specific error with known corrective action | `WrapErrWithHint`       | `WrapErrWithHint("branchDelete", err, "use gitlab_branch_unprotect first")`   |
| Single-status hint (the common case)        | `WrapErrWithStatusHint` | `WrapErrWithStatusHint("issueGet", err, 404, "verify issue_iid")`             |

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
        "Use gitlab_branch_list with project_id to list available branches",
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

1. **YOLO_MODE / AUTOPILOT** env var set → skip confirmation
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

| File                             | Purpose                                                                                                                                                                           |
| -------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `internal/toolutil/errors.go`    | ToolError, DetailedError, WrapErr, WrapErrWithMessage, WrapErrWithHint, WrapErrWithStatusHint, ExtractGitLabMessage, ClassifyError, ClassifyHTTPStatus, IsHTTPStatus, ContainsAny |
| `internal/toolutil/not_found.go` | NotFoundResult — informational 404 pattern for get handlers                                                                                                                       |
| `internal/toolutil/confirm.go`   | Destructive action confirmation flow                                                                                                                                              |
| `internal/toolutil/output.go`    | SuccessResult, ErrorResult helpers                                                                                                                                                |

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
    "verify issue_iid with gitlab_issue_list")
```

GraphQL error sites use `WrapErrWithHint` which always appends the hint (GraphQL errors don't carry HTTP status codes):

```go
return toolutil.WrapErrWithHint("list_vulnerabilities", err,
    "verify the project fullPath is correct and your token has access to security features")
```
