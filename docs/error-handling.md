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

### Error Function Decision Tree

| Scenario | Function | Example |
| --- | --- | --- |
| Read-only operation (list, get, search) | `WrapErr` | `WrapErr("listBranches", err)` |
| Mutating operation (create, update, delete) | `WrapErrWithMessage` | `WrapErrWithMessage("fileCreate", err)` |
| Specific error with known corrective action | `WrapErrWithHint` | `WrapErrWithHint("branchDelete", err, "use gitlab_branch_unprotect first")` |

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
| `internal/toolutil/errors.go` | ToolError, DetailedError, WrapErr, WrapErrWithMessage, WrapErrWithHint, ExtractGitLabMessage, ClassifyError, ClassifyHTTPStatus, IsHTTPStatus, ContainsAny |
| `internal/toolutil/issue_report.go` | IssueReport, FormatIssueReport, secret redaction |
| `internal/toolutil/confirm.go` | Destructive action confirmation flow |
| `internal/toolutil/output.go` | SuccessResult, ErrorResult helpers |
