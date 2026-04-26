# Error Handling

This document describes the error handling architecture in gitlab-mcp-server. All error types, classification logic, and formatting utilities live in `internal/toolutil/`.

> **Diátaxis type**: Explanation
> **Audience**: 🔧 Developers, contributors
> **Prerequisites**: Go programming, understanding of MCP tool handlers
> 📖 **User documentation**: See the [Error Handling](https://jmrplens.github.io/gitlab-mcp-server/operations/error-handling/) on the documentation site for a user-friendly version.

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

A richer error type with domain context for diagnostic output and automated issue reporting:

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

| Error Type | Example Message |
| --- | --- |
| GitLab HTTP response | Delegates to `ClassifyHTTPStatus` |
| Connection refused | "GitLab server is unreachable (connection refused)" |
| DNS failure | "GitLab server hostname could not be resolved" |
| Timeout | "Request to GitLab timed out" |
| TLS/SSL | "TLS/SSL handshake failed" |
| URL error | "network error reaching GitLab" |
| Other | "unexpected error" |

### ClassifyHTTPStatus

Maps HTTP status codes to actionable guidance:

| Code | Message |
| --- | --- |
| 400 | "bad request — check your input parameters" |
| 401 | "authentication failed — GITLAB_TOKEN may be invalid or expired" |
| 403 | "access denied — your token lacks the required permissions" |
| 404 | "not found — the requested resource does not exist or you lack access" |
| 409 | "conflict — the resource already exists or there is a state conflict" |
| 422 | "validation failed — GitLab rejected the request due to invalid data" |
| 429 | "rate limited — too many requests, please wait before retrying" |
| 500 | "GitLab internal server error" |
| 502 | "GitLab is temporarily unavailable (bad gateway)" |
| 503 | "GitLab is under maintenance or overloaded" |

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
// Result: "list_issues: authentication failed — GITLAB_TOKEN may be invalid or expired: <original>"
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
// Result: "fileCreate: bad request — A file with this name already exists: POST .../files: 400"
// Falls back to WrapErr format when glErr.Message adds no useful detail
```

### WrapErrWithHint

Like `WrapErrWithMessage` but appends an actionable suggestion that tells the LLM what to do next. Use when you know the corrective action for a specific error scenario:

```go
if toolutil.IsHTTPStatus(err, 409) {
    return toolutil.WrapErrWithHint("branchProtect", err,
        "protected branch rule already exists — use gitlab_protected_branch_get to view current rules")
}
// Result: "branchProtect: conflict — Protected branch rule already exists.
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

| Scenario | Function | Example |
| --- | --- | --- |
| Read-only operation (list, get, search) | `WrapErr` | `WrapErr("listBranches", err)` |
| Get operation returning 404 | `NotFoundResult` | `NotFoundResult("Branch", "main in project 42", "Use gitlab_branch_list...")` |
| Mutating operation (create, update, delete) | `WrapErrWithMessage` | `WrapErrWithMessage("fileCreate", err)` |
| Specific error with known corrective action | `WrapErrWithHint` | `WrapErrWithHint("branchDelete", err, "use gitlab_branch_unprotect first")` |
| Single-status hint (the common case) | `WrapErrWithStatusHint` | `WrapErrWithStatusHint("issueGet", err, 404, "verify issue_iid")` |

### NotFoundResult — Informational 404 Responses

For "get" handlers, HTTP 404 errors are intercepted **before** the standard error flow and returned as structured, informational results instead of opaque Go errors. This improves the LLM experience: instead of a raw error, the assistant receives an `IsError: true` result with a human-readable explanation and domain-specific next-step hints.

```go
// In register.go handler closures:
out, err := Get(ctx, client, input)
if err != nil && toolutil.IsHTTPStatus(err, 404) {
    toolutil.LogToolCallAll(ctx, req, "gitlab_branch_get", start, nil) // nil → INFO log
    return toolutil.NotFoundResult("Branch", fmt.Sprintf("%q in project %s", input.BranchName, input.ProjectID),
        "Use gitlab_branch_list with project_id to list available branches",
        "Verify the branch name is spelled correctly (case-sensitive)",
    ), Output{}, nil // nil error → SDK logs at INFO, not ERROR
}
```

The `NotFoundResult(resource, identifier string, hints ...string)` function in `internal/toolutil/not_found.go`:

1. Creates a Markdown-formatted `CallToolResult` with `IsError: true`
2. Includes a `## ❓ {Resource} Not Found` heading with the identifier
3. Appends `💡 Next steps` hints specific to the domain
4. The handler returns `nil` as the Go error so `LogToolCallAll` logs at INFO level

This pattern is applied to **27 get handlers** across 21 domains: projects, groups, branches, tags, commits, files, issues (get + get_by_id), merge requests, milestones, labels, pipelines, releases, release links, environments, deployments, snippets, wikis, users, issue links, issue notes, MR notes, MR discussions, MR draft notes, badges (project + group), and award emoji (6 variants).

### ErrorResultMarkdown

For errors that should be returned as tool results (with `IsError: true`) rather than Go errors:

```go
result := ErrorResultMarkdown("issues", "list", err)
```

Renders the `DetailedError` as a Markdown block with all diagnostic fields.

## Automated Issue Reporting

> **Opt-in feature**: Issue report generation is disabled by default. Set `ISSUE_REPORTS=true` to enable it. When disabled, `FormatIssueReport` falls back to `ErrorResultMarkdown` — the standard Markdown error output.

For unrecoverable errors, `FormatIssueReport` generates a complete bug report that can be copied into a GitLab or GitHub issue:

```go
result := FormatIssueReport("issues", "create", err, inputParams)
```

### IssueReport Contents

The generated Markdown includes:

| Section | Content |
| --- | --- |
| Environment | Server version, Go version, OS/Arch, timestamp |
| Error Details | Tool, action, error message, HTTP status, request ID |
| Input (sanitized) | All input parameters with secrets redacted |
| Steps to Reproduce | Pre-filled reproduction steps |
| Suggested Labels | `bug`, `mcp-tool`, `automated-report` |

### Secret Redaction

Input parameters are automatically sanitized before inclusion. Any key containing these substrings (case-insensitive) is replaced with `[REDACTED]`:

`token`, `password`, `secret`, `key`, `credential`, `auth`, `cookie`, `session`, `private`

### Server Version

The report includes the server version, read from the `VERSION` file at package initialization. Override with `SetServerVersion(v)` when the binary runs from a different working directory.

## Network Error Helpers

Lower-level helpers detect specific network conditions:

| Helper | Detects |
| --- | --- |
| `isConnectionRefused` | ECONNREFUSED, "connectex:" |
| `isDNSError` | `*net.DNSError` in error chain |
| `isTimeout` | Any error implementing `Timeout() bool` |
| `isTLSError` | "tls:", "certificate", "x509:" in message |
| `ContainsAny` | Generic substring match on `err.Error()` |

## Parameter-Name Guidance Helpers

When LLMs call meta-tools, misnamed JSON parameters are silently ignored during deserialization and the field defaults to its zero value. Two helpers produce error messages that guide the LLM to use the exact documented parameter name:

| Helper | Use Case | Example Output |
| --- | --- | --- |
| `ErrRequiredInt64(op, field)` | Required int64 field is 0 | `"milestoneGet: milestone_iid is required (must be > 0). Ensure you use the exact parameter name 'milestone_iid'..."` |
| `ErrRequiredString(op, field)` | Required string field is empty | `"branchCreate: branch_name is required (must be non-empty). Ensure you use the exact parameter name 'branch_name'..."` |

Used in `milestones`, `branches`, `mergerequests`, and other domains where LLMs frequently confuse parameter names (e.g., `milestone_id` vs `milestone_iid`, `branch` vs `branch_name`, `merge_request_iid` vs `mr_iid`).

## Destructive Action Confirmation

Before executing destructive operations (delete, force-push), handlers use the confirmation flow in `confirm.go`:

1. **YOLO_MODE / AUTOPILOT** env var set → skip confirmation
2. **Explicit `confirm: true`** in params → proceed
3. **MCP elicitation supported** → ask user interactively via `elicitation.Confirm()`
4. **No confirmation mechanism** → return `CancelledResult`

## Testing Error Handling

When writing tests for error scenarios, use `http.StatusBadRequest` (400) instead of 500 for mock API errors. Status 500 triggers the `retryablehttp` client's retry loop, causing test hangs.

```go
// Correct: use 400 for error mocks in tests
testutil.RespondJSON(w, http.StatusBadRequest, map[string]string{
    "message": "Bad Request",
})
```

## File Reference

| File | Purpose |
| --- | --- |
| `internal/toolutil/errors.go` | ToolError, DetailedError, WrapErr, WrapErrWithMessage, WrapErrWithHint, WrapErrWithStatusHint, ExtractGitLabMessage, ClassifyError, ClassifyHTTPStatus, IsHTTPStatus, ContainsAny |
| `internal/toolutil/not_found.go` | NotFoundResult — informational 404 pattern for get handlers |
| `internal/toolutil/issue_report.go` | IssueReport, FormatIssueReport, secret redaction |
| `internal/toolutil/confirm.go` | Destructive action confirmation flow |
| `internal/toolutil/output.go` | SuccessResult, ErrorResult helpers |

## LLM Ergonomics Hint Rollout

Actionable hints were added across the entire codebase to help LLMs self-correct when API calls fail. The rollout converted `WrapErrWithMessage` calls to `WrapErrWithHint` (GraphQL) or `WrapErrWithStatusHint` (REST) with domain-specific suggestions.

### Coverage

| Metric | Count |
| --- | --- |
| `WrapErrWithHint` call sites (GraphQL) | 257 |
| `WrapErrWithStatusHint` call sites (REST) | 858 |
| **Total hinted error sites** | **1,115** |
| `WrapErrWithMessage` (skip-category, retained) | 344 |
| `NotFoundResult` (informational 404s) | 32 |
| Domain sub-packages with hints | 153 of 162 |
| Source files with hints | 171 |

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
