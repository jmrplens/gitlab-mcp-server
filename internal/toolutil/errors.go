package toolutil

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// ToolError represents a structured error from a tool handler.
type ToolError struct {
	Tool       string `json:"tool"`
	Message    string `json:"message"`
	StatusCode int    `json:"status_code,omitempty"`
}

// Error returns a human-readable representation of the tool error.
// When StatusCode is set, it is appended as "(HTTP <code>)".
func (e *ToolError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("%s: %s (HTTP %d)", e.Tool, e.Message, e.StatusCode)
	}
	return fmt.Sprintf("%s: %s", e.Tool, e.Message)
}

// WrapErr classifies the error, enriches it with a semantic message, and
// wraps it with the operation name. All tool handlers funnel through here
// so connectivity and auth problems are reported consistently.
func WrapErr(operation string, err error) error {
	semantic := ClassifyError(err)
	return fmt.Errorf("%s: %s: %w", operation, semantic, err)
}

// ClassifyError inspects the error chain and returns a short, human-friendly
// diagnostic message explaining what went wrong at a high level.
func ClassifyError(err error) string {
	if err == nil {
		return "unknown error"
	}

	// GitLab API returned an HTTP error response
	var glErr *gl.ErrorResponse
	if errors.As(err, &glErr) && glErr.Response != nil {
		return ClassifyHTTPStatus(glErr.Response.StatusCode)
	}

	// Network-level errors (connection refused, DNS, timeout, TLS)
	if isConnectionRefused(err) {
		return "GitLab server is unreachable (connection refused). Check GITLAB_URL and whether the server is running"
	}
	if isDNSError(err) {
		return "GitLab server hostname could not be resolved (DNS error). Check GITLAB_URL"
	}
	if isTimeout(err) {
		return "Request to GitLab timed out. The server may be overloaded or unreachable"
	}
	if isTLSError(err) {
		return "TLS/SSL handshake failed. If using self-signed certificates, set GITLAB_SKIP_TLS_VERIFY=true"
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return fmt.Sprintf("network error reaching GitLab (%s)", urlErr.Op)
	}

	return "unexpected error"
}

// httpStatusDescriptions maps HTTP status codes to semantic descriptions.
var httpStatusDescriptions = map[int]string{
	400: "bad request — check your input parameters",
	401: "authentication failed — GITLAB_TOKEN may be invalid or expired",
	403: "access denied — your token lacks the required permissions. This can mean: (1) missing API scope on the token, (2) insufficient project role (some operations require Maintainer or Owner), or (3) the feature is restricted by instance admin settings",
	404: "not found — the requested resource does not exist, you lack access, or the feature requires a higher GitLab tier. Verify the ID/path is correct",
	405: "method not allowed — the action cannot be performed on this resource in its current state",
	409: "conflict — the resource already exists or there is a state conflict",
	422: "validation failed — GitLab rejected the request due to invalid data",
	429: "rate limited — too many requests, please wait before retrying",
	500: "GitLab internal server error — the server encountered an unexpected condition",
	502: "GitLab is temporarily unavailable (bad gateway) — try again shortly",
	503: "GitLab is under maintenance or overloaded (service unavailable) — try again shortly",
}

// ClassifyHTTPStatus returns a semantic description for common HTTP status codes.
func ClassifyHTTPStatus(code int) string {
	if desc, ok := httpStatusDescriptions[code]; ok {
		return desc
	}
	return fmt.Sprintf("GitLab returned HTTP %d", code)
}

// isConnectionRefused checks for ECONNREFUSED at any depth in the error chain.
func isConnectionRefused(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if errors.Is(opErr.Err, syscall.ECONNREFUSED) {
			return true
		}
	}
	return ContainsAny(err, "connection refused", "connectex:")
}

// isDNSError checks for DNS resolution failures.
func isDNSError(err error) bool {
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr)
}

// isTimeout checks for context deadline or network timeout errors.
func isTimeout(err error) bool {
	type timeouter interface{ Timeout() bool }
	var t timeouter
	return errors.As(err, &t) && t.Timeout()
}

// isTLSError detects TLS handshake failures from error message patterns.
func isTLSError(err error) bool {
	return ContainsAny(err, "tls:", "certificate", "x509:")
}

// ContainsAny returns true if err.Error() contains any of the substrings.
func ContainsAny(err error, substrs ...string) bool {
	msg := strings.ToLower(err.Error())
	for _, s := range substrs {
		if strings.Contains(msg, strings.ToLower(s)) {
			return true
		}
	}
	return false
}

// DetailedError represents a rich, structured error with domain context for
// diagnostic output. It extends ToolError with additional fields useful for
// automated issue creation and Markdown error reporting.
type DetailedError struct {
	Domain       string `json:"domain"`
	Action       string `json:"action"`
	Message      string `json:"message"`
	Details      string `json:"details,omitempty"`
	GitLabStatus int    `json:"gitlab_status,omitempty"`
	RequestID    string `json:"request_id,omitempty"`
}

// Error returns a concise representation: "domain/action: message".
func (e *DetailedError) Error() string {
	base := fmt.Sprintf("%s/%s: %s", e.Domain, e.Action, e.Message)
	if e.GitLabStatus > 0 {
		return fmt.Sprintf("%s (HTTP %d)", base, e.GitLabStatus)
	}
	return base
}

// Markdown renders the error as a Markdown block suitable for display in MCP
// tool results. Includes all available context for diagnostics.
func (e *DetailedError) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "## "+EmojiCross+" Error: %s/%s\n\n", e.Domain, e.Action)
	fmt.Fprintf(&b, "**Message**: %s\n", e.Message)
	if e.GitLabStatus > 0 {
		fmt.Fprintf(&b, "**HTTP Status**: %d — %s\n", e.GitLabStatus, ClassifyHTTPStatus(e.GitLabStatus))
	}
	if e.Details != "" {
		fmt.Fprintf(&b, "**Details**: %s\n", e.Details)
	}
	if e.RequestID != "" {
		fmt.Fprintf(&b, "**Request ID**: `%s`\n", e.RequestID)
	}
	return b.String()
}

// NewDetailedError creates a DetailedError from a GitLab API error, extracting
// HTTP status and request ID when available.
func NewDetailedError(domain, action string, err error) *DetailedError {
	de := &DetailedError{
		Domain:  domain,
		Action:  action,
		Message: ClassifyError(err),
	}

	// Safely extract details — gl.ErrorResponse.Error() can panic with nil Body
	func() {
		defer func() { recover() }() //nolint:errcheck // intentional panic recovery
		de.Details = err.Error()
	}()

	var glErr *gl.ErrorResponse
	if errors.As(err, &glErr) && glErr.Response != nil {
		de.GitLabStatus = glErr.Response.StatusCode
		de.RequestID = glErr.Response.Header.Get("X-Request-Id")
		if de.Details == "" {
			de.Details = glErr.Message
		}
	}

	return de
}

// ErrFieldRequired returns a validation error indicating that a required field
// is missing or empty. It produces the message "<field> is required", which is
// the standard validation pattern used across all tool handlers.
func ErrFieldRequired(field string) error {
	return fmt.Errorf("%s is required", field)
}

// ErrRequiredInt64 returns a formatted error when a required int64 field is
// missing or has its zero value. This catches silent deserialization failures
// in meta-tool dispatch, where a misnamed JSON parameter (e.g. "mr_iid"
// instead of "merge_request_iid") is silently ignored and the field defaults to 0.
func ErrRequiredInt64(operation, field string) error {
	return fmt.Errorf("%s: %s is required (must be > 0). Ensure you use the exact parameter name '%s' as documented in the tool description", operation, field, field)
}

// ErrRequiredString returns a formatted error when a required string field is
// missing or empty. Like ErrRequiredInt64, this guides LLMs to use the exact
// parameter name when silent deserialization failures occur.
func ErrRequiredString(operation, field string) error {
	return fmt.Errorf("%s: %s is required (must be non-empty). Ensure you use the exact parameter name '%s' as documented in the tool description", operation, field, field)
}

// ErrInvalidEnum returns a validation error indicating that a field value
// is not one of the allowed options. The error message lists the valid values
// to guide LLMs toward correct parameter usage.
func ErrInvalidEnum(field, value string, validValues []string) error {
	return fmt.Errorf("invalid %s %q, must be one of: %s", field, value, strings.Join(validValues, ", "))
}

// IsHTTPStatus reports whether err wraps a GitLab ErrorResponse with the
// given HTTP status code. Useful for handling specific API responses like
// 404 (feature not available on CE) or 403 (insufficient permissions).
func IsHTTPStatus(err error, code int) bool {
	if code == http.StatusNotFound && errors.Is(err, gl.ErrNotFound) {
		return true
	}
	var glErr *gl.ErrorResponse
	return errors.As(err, &glErr) && glErr.Response != nil && glErr.Response.StatusCode == code
}

// IsNotFound reports whether err represents a 404 Not Found, either via a
// structured GitLab ErrorResponse status code or via a plain-text error
// message from client-go (which may contain "404 Not Found" as text).
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	if IsHTTPStatus(err, http.StatusNotFound) {
		return true
	}
	return ContainsAny(err, "404 Not Found")
}

// ExtractGitLabMessage extracts the specific error message from a GitLab
// ErrorResponse in the error chain. Returns empty string if not found or if
// the message only repeats the HTTP status text (e.g., "405 Method Not Allowed").
// The extracted message is truncated to 300 characters to prevent overly verbose
// error output from user-generated content.
func ExtractGitLabMessage(err error) string {
	var glErr *gl.ErrorResponse
	if !errors.As(err, &glErr) {
		return ""
	}
	msg := glErr.Message
	if msg == "" {
		return ""
	}
	// Filter out messages that are just the HTTP status text — they add no information
	// beyond what ClassifyHTTPStatus already provides.
	if glErr.Response != nil {
		statusText := strconv.Itoa(glErr.Response.StatusCode)
		normalized := strings.TrimSpace(msg)
		if normalized == statusText || strings.HasPrefix(normalized, statusText+" ") {
			return ""
		}
		// Also filter wrapped status messages like "{message: 405 Method Not Allowed}"
		if strings.Contains(normalized, statusText+" ") && !strings.ContainsAny(normalized, "[]") {
			inner := normalized
			inner = strings.TrimPrefix(inner, "{message: ")
			inner = strings.TrimSuffix(inner, "}")
			inner = strings.TrimSpace(inner)
			if strings.HasPrefix(inner, statusText+" ") {
				return ""
			}
		}
	}
	const maxLen = 300
	if len(msg) > maxLen {
		msg = msg[:maxLen] + "…"
	}
	return msg
}

// WrapErrWithMessage works like WrapErr but also includes the specific GitLab
// error message (from ErrorResponse.Message) when available. This produces
// richer errors like:
//
//	"fileCreate: bad request — {error: A file with this name already exists}: POST .../files: 400"
//
// Use WrapErrWithMessage for mutating operations where the specific GitLab
// error detail helps the LLM understand what went wrong. Use WrapErr for
// read-only operations where the generic classification suffices.
func WrapErrWithMessage(operation string, err error) error {
	semantic := ClassifyError(err)
	glMsg := ExtractGitLabMessage(err)
	if glMsg != "" {
		return fmt.Errorf("%s: %s — %s: %w", operation, semantic, glMsg, err)
	}
	return fmt.Errorf("%s: %s: %w", operation, semantic, err)
}

// WrapErrWithHint works like WrapErrWithMessage but appends an actionable hint
// that tells the LLM what to do next. Example:
//
//	"branchDelete: bad request — Cannot delete: protected branch.
//	 Suggestion: use gitlab_branch_unprotect first, then retry deletion: <original>"
//
// The hint should be a concise suggestion starting with a verb (e.g., "use
// gitlab_branch_list to verify the branch name").
func WrapErrWithHint(operation string, err error, hint string) error {
	semantic := ClassifyError(err)
	glMsg := ExtractGitLabMessage(err)
	if glMsg != "" {
		return fmt.Errorf("%s: %s — %s. Suggestion: %s: %w", operation, semantic, glMsg, hint, err)
	}
	return fmt.Errorf("%s: %s. Suggestion: %s: %w", operation, semantic, hint, err)
}

// WrapErrWithStatusHint returns WrapErrWithHint(operation, err, hint) when err
// matches the given HTTP status code, otherwise falls back to
// WrapErrWithMessage(operation, err). It compresses the common pattern:
//
//	if toolutil.IsHTTPStatus(err, 404) {
//	    return ..., toolutil.WrapErrWithHint(op, err, hint)
//	}
//	return ..., toolutil.WrapErrWithMessage(op, err)
//
// into a single call. For handlers that need different hints per status, use
// a switch over IsHTTPStatus checks; this helper covers the dominant single-
// status case.
func WrapErrWithStatusHint(operation string, err error, code int, hint string) error {
	if IsHTTPStatus(err, code) {
		return WrapErrWithHint(operation, err, hint)
	}
	return WrapErrWithMessage(operation, err)
}

// ErrorResultMarkdown creates an MCP tool error result with Markdown formatting.
// The result has IsError = true for MCP clients that distinguish error results.
func ErrorResultMarkdown(domain, action string, err error) *mcp.CallToolResult {
	de := NewDetailedError(domain, action, err)
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: de.Markdown()},
		},
		IsError: true,
	}
}
