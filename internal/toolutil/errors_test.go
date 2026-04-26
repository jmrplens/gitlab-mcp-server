// errors_test.go contains unit tests for ToolError formatting, the WrapErr
// helper, ClassifyError semantic classification, isConnectionRefused, and
// ClassifyHTTPStatus.

package toolutil

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"syscall"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

const (
	fmtErrWant       = "Error() = %q, want %q"
	msgUnexpectedErr = "unexpected error"
)

// TestToolError_WithStatusCode verifies that Error() includes the HTTP status
// code in the formatted string when StatusCode is non-zero.
func TestToolError_WithStatusCode(t *testing.T) {
	err := &ToolError{Tool: "gitlab_project_get", Message: "not found", StatusCode: 404}
	want := "gitlab_project_get: not found (HTTP 404)"
	if got := err.Error(); got != want {
		t.Errorf(fmtErrWant, got, want)
	}
}

// TestToolError_WithoutStatusCode verifies that Error() omits the HTTP status
// code suffix when StatusCode is zero.
func TestToolError_WithoutStatusCode(t *testing.T) {
	err := &ToolError{Tool: "gitlab_project_get", Message: "connection refused"}
	want := "gitlab_project_get: connection refused"
	if got := err.Error(); got != want {
		t.Errorf(fmtErrWant, got, want)
	}
}

// TestToolError_ZeroStatusCode verifies that a zero StatusCode produces the
// same output as no status code.
func TestToolError_ZeroStatusCode(t *testing.T) {
	err := &ToolError{Tool: "test", Message: "fail", StatusCode: 0}
	want := "test: fail"
	if got := err.Error(); got != want {
		t.Errorf(fmtErrWant, got, want)
	}
}

// TestWrapErr_AddsContextAndClassification verifies that WrapErr prepends
// the operation name and a semantic classification to the original error.
func TestWrapErr_AddsContextAndClassification(t *testing.T) {
	original := &ToolError{Tool: "inner", Message: "broken"}
	wrapped := WrapErr("outer_op", original)
	if wrapped == nil {
		t.Fatal("WrapErr returned nil")
	}
	if !strings.Contains(wrapped.Error(), "outer_op:") {
		t.Errorf("WrapErr() = %q, want operation prefix 'outer_op:'", wrapped.Error())
	}
	if !strings.Contains(wrapped.Error(), msgUnexpectedErr) {
		t.Errorf("WrapErr() = %q, want '%s' classification for unknown error type", wrapped.Error(), msgUnexpectedErr)
	}
}

// TestClassifyError_HTTPStatuses verifies semantic messages for common HTTP codes.
func TestClassifyError_HTTPStatuses(t *testing.T) {
	tests := []struct {
		code int
		want string
	}{
		{400, "bad request"},
		{401, "authentication failed"},
		{403, "access denied"},
		{404, "not found"},
		{409, "conflict"},
		{422, "validation failed"},
		{429, "rate limited"},
		{500, "internal server error"},
		{502, "bad gateway"},
		{503, "maintenance"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("HTTP_%d", tt.code), func(t *testing.T) {
			glErr := &gl.ErrorResponse{
				Response: &http.Response{StatusCode: tt.code},
				Message:  fmt.Sprintf("%d error", tt.code),
			}
			got := ClassifyError(glErr)
			if !strings.Contains(strings.ToLower(got), tt.want) {
				t.Errorf("ClassifyError(HTTP %d) = %q, want substring %q", tt.code, got, tt.want)
			}
		})
	}
}

// TestClassifyError_ConnectionRefused verifies detection of connection refused errors.
func TestClassifyError_ConnectionRefused(t *testing.T) {
	err := fmt.Errorf("dial tcp 10.0.0.1:443: connection refused")
	got := ClassifyError(err)
	if !strings.Contains(got, "unreachable") {
		t.Errorf("ClassifyError(conn refused) = %q, want 'unreachable'", got)
	}
}

// TestClassifyError_DNS verifies detection of DNS resolution failures.
func TestClassifyError_DNS(t *testing.T) {
	dnsErr := &net.DNSError{Err: "no such host", Name: "unknown.example.com"}
	err := fmt.Errorf("lookup failed: %w", dnsErr)
	got := ClassifyError(err)
	if !strings.Contains(got, "DNS") {
		t.Errorf("ClassifyError(DNS) = %q, want 'DNS'", got)
	}
}

// TestClassifyError_TLS verifies detection of TLS/certificate errors.
func TestClassifyError_TLS(t *testing.T) {
	err := fmt.Errorf("Get https://gitlab.example.com: x509: certificate signed by unknown authority")
	got := ClassifyError(err)
	if !strings.Contains(got, "TLS") {
		t.Errorf("ClassifyError(TLS) = %q, want 'TLS'", got)
	}
}

// TestClassifyError_Timeout verifies detection of timeout errors.
func TestClassifyError_Timeout(t *testing.T) {
	err := &timeoutError{msg: "deadline exceeded"}
	got := ClassifyError(err)
	if !strings.Contains(got, "timed out") {
		t.Errorf("ClassifyError(timeout) = %q, want 'timed out'", got)
	}
}

// TestClassifyError_NilError verifies handling of nil errors.
func TestClassifyError_NilError(t *testing.T) {
	got := ClassifyError(nil)
	if got != "unknown error" {
		t.Errorf("ClassifyError(nil) = %q, want %q", got, "unknown error")
	}
}

// TestClassifyError_GenericError verifies the fallback message for unknown errors.
func TestClassifyError_GenericError(t *testing.T) {
	got := ClassifyError(errors.New("something weird happened"))
	if got != msgUnexpectedErr {
		t.Errorf("ClassifyError(generic) = %q, want %q", got, msgUnexpectedErr)
	}
}

// TestWrapErr_PropagatesSemanticClassification verifies that the full WrapErr
// output includes the semantic classification for a GitLab 401 error.
func TestWrapErr_PropagatesSemanticClassification(t *testing.T) {
	glErr := &gl.ErrorResponse{
		Response: &http.Response{StatusCode: http.StatusUnauthorized},
		Message:  "401 Unauthorized",
	}
	wrapped := WrapErr("userCurrent", glErr)
	msg := wrapped.Error()

	if !strings.Contains(msg, "userCurrent:") {
		t.Errorf("missing operation name in: %q", msg)
	}
	if !strings.Contains(msg, "authentication failed") {
		t.Errorf("missing semantic classification in: %q", msg)
	}
	if !strings.Contains(msg, "GITLAB_TOKEN") {
		t.Errorf("missing remediation hint in: %q", msg)
	}
}

// timeoutError is a test helper implementing net.Error with Timeout() = true.
type timeoutError struct{ msg string }

// Error performs the error operation on *timeoutError.
func (e *timeoutError) Error() string { return e.msg }

// Timeout reports whether the *timeoutError satisfies the timeout condition.
func (e *timeoutError) Timeout() bool { return true }

// Temporary reports whether the *timeoutError satisfies the temporary condition.
func (e *timeoutError) Temporary() bool { return false }

// TestDetailedError_Error verifies the string representation of DetailedError.
func TestDetailedError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  DetailedError
		want string
	}{
		{
			name: "without status",
			err:  DetailedError{Domain: "projects", Action: "delete", Message: "not found"},
			want: "projects/delete: not found",
		},
		{
			name: "with status",
			err:  DetailedError{Domain: "issues", Action: "create", Message: "validation failed", GitLabStatus: 422},
			want: "issues/create: validation failed (HTTP 422)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestDetailedError_Markdown verifies the Markdown rendering includes all fields.
func TestDetailedError_Markdown(t *testing.T) {
	de := &DetailedError{
		Domain:       "projects",
		Action:       "delete",
		Message:      "access denied",
		Details:      "403 Forbidden: insufficient permissions",
		GitLabStatus: 403,
		RequestID:    "req-abc-123",
	}
	md := de.Markdown()

	checks := []string{
		"## ❌ Error: projects/delete",
		"**Message**: access denied",
		"**HTTP Status**: 403",
		"**Details**: 403 Forbidden",
		"**Request ID**: `req-abc-123`",
	}
	for _, want := range checks {
		if !strings.Contains(md, want) {
			t.Errorf("Markdown() missing %q in:\n%s", want, md)
		}
	}
}

// TestDetailedError_Markdown_MinimalFields verifies Markdown with only required fields.
func TestDetailedError_MarkdownMinimal(t *testing.T) {
	de := &DetailedError{
		Domain:  "repos",
		Action:  "get",
		Message: "unexpected error",
	}
	md := de.Markdown()
	if strings.Contains(md, "**HTTP Status**") {
		t.Error("minimal Markdown should not contain HTTP Status")
	}
	if strings.Contains(md, "**Details**") {
		t.Error("minimal Markdown should not contain Details")
	}
	if strings.Contains(md, "**Request ID**") {
		t.Error("minimal Markdown should not contain Request ID")
	}
}

// TestNewDetailedError_GitLabError verifies extraction of HTTP status from GitLab errors.
func TestNewDetailedError_GitLabError(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusNotFound,
		Header:     http.Header{"X-Request-Id": []string{"req-xyz"}},
		Body:       http.NoBody,
	}
	glErr := &gl.ErrorResponse{
		Response: resp,
		Message:  "404 Not Found",
	}
	de := NewDetailedError("branches", "get", glErr)
	if de.GitLabStatus != 404 {
		t.Errorf("GitLabStatus = %d, want 404", de.GitLabStatus)
	}
	if de.RequestID != "req-xyz" {
		t.Errorf("RequestID = %q, want %q", de.RequestID, "req-xyz")
	}
	if !strings.Contains(de.Message, "not found") {
		t.Errorf("Message = %q, want to contain 'not found'", de.Message)
	}
}

// TestNewDetailedError_GenericError verifies handling of non-GitLab errors.
func TestNewDetailedError_GenericError(t *testing.T) {
	de := NewDetailedError("tags", "create", errors.New("something broke"))
	if de.GitLabStatus != 0 {
		t.Errorf("GitLabStatus = %d, want 0", de.GitLabStatus)
	}
	if de.RequestID != "" {
		t.Errorf("RequestID = %q, want empty", de.RequestID)
	}
}

// TestErrorResultMarkdown verifies the MCP error result construction.
func TestErrorResultMarkdown(t *testing.T) {
	result := ErrorResultMarkdown("projects", "delete", errors.New("boom"))
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.IsError {
		t.Error("expected IsError = true")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content, got %d", len(result.Content))
	}
}

// TestIsConnectionRefused_OpError verifies detection of ECONNREFUSED wrapped
// inside a net.OpError (the typed errors.As branch in isConnectionRefused).
func TestIsConnectionRefused_OpError(t *testing.T) {
	inner := &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Addr: &net.TCPAddr{
			IP:   net.IPv4(127, 0, 0, 1),
			Port: 443,
		},
		Err: &os.SyscallError{Syscall: "connect", Err: syscall.ECONNREFUSED},
	}
	if !isConnectionRefused(inner) {
		t.Error("expected true for OpError wrapping ECONNREFUSED")
	}
}

// TestIsConnectionRefused_StringFallback verifies the string-match fallback
// path when the error is not a typed net.OpError.
func TestIsConnectionRefused_StringFallback(t *testing.T) {
	err := errors.New("dial tcp 10.0.0.1:443: connection refused")
	if !isConnectionRefused(err) {
		t.Error("expected true for string containing 'connection refused'")
	}
}

// TestIsConnectionRefused_Unrelated verifies false for an unrelated error.
func TestIsConnectionRefused_Unrelated(t *testing.T) {
	err := errors.New("something else happened")
	if isConnectionRefused(err) {
		t.Error("expected false for unrelated error")
	}
}

// TestClassifyHTTPStatus_DefaultCode verifies the default branch for HTTP
// status codes not explicitly handled (e.g. 418 I'm a Teapot).
func TestClassifyHTTPStatus_DefaultCode(t *testing.T) {
	got := ClassifyHTTPStatus(418)
	want := "GitLab returned HTTP 418"
	if got != want {
		t.Errorf("ClassifyHTTPStatus(418) = %q, want %q", got, want)
	}
}

// TestClassifyError_URLError verifies the url.Error fallback branch in
// ClassifyError for network errors that aren't DNS, timeout, TLS, or
// connection refused.
func TestClassifyError_URLError(t *testing.T) {
	inner := &url.Error{
		Op:  "Get",
		URL: "https://gitlab.example.com/api/v4/projects",
		Err: errors.New("some unknown network issue"),
	}
	got := ClassifyError(inner)
	if !strings.Contains(got, "network error") {
		t.Errorf("ClassifyError(url.Error) = %q, want 'network error'", got)
	}
	if !strings.Contains(got, "Get") {
		t.Errorf("ClassifyError(url.Error) = %q, want operation 'Get'", got)
	}
}

// TestErrInvalidEnum verifies the message lists the valid values and
// includes the rejected value.
func TestErrInvalidEnum(t *testing.T) {
	err := ErrInvalidEnum("status", "pending", []string{"approved", "rejected"})
	got := err.Error()
	if !strings.Contains(got, "status") {
		t.Errorf("ErrInvalidEnum() = %q, want field name", got)
	}
	if !strings.Contains(got, `"pending"`) {
		t.Errorf("ErrInvalidEnum() = %q, want rejected value", got)
	}
	if !strings.Contains(got, "approved, rejected") {
		t.Errorf("ErrInvalidEnum() = %q, want valid values", got)
	}
}

// TestErrInvalidEnum_SingleValue verifies ErrInvalidEnum with a single valid option.
func TestErrInvalidEnum_SingleValue(t *testing.T) {
	err := ErrInvalidEnum("visibility", "hidden", []string{"public"})
	got := err.Error()
	if !strings.Contains(got, "public") {
		t.Errorf("ErrInvalidEnum() = %q, want valid value listed", got)
	}
}

// TestErrRequiredString verifies the formatted error when a required string
// field is missing. The message must contain the operation, field name, and
// guidance about using the exact parameter name.
func TestErrRequiredString(t *testing.T) {
	err := ErrRequiredString("issue_create", "title")
	if err == nil {
		t.Fatal("ErrRequiredString should return non-nil error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "issue_create") {
		t.Errorf("error should contain operation, got %q", msg)
	}
	if !strings.Contains(msg, "title") {
		t.Errorf("error should contain field name, got %q", msg)
	}
	if !strings.Contains(msg, "non-empty") {
		t.Errorf("error should mention non-empty constraint, got %q", msg)
	}
}

// TestIsHTTPStatus verifies that IsHTTPStatus correctly identifies GitLab
// ErrorResponse instances matching a given HTTP status code.
func TestIsHTTPStatus(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code int
		want bool
	}{
		{
			name: "matching 404",
			err: &gl.ErrorResponse{
				Response: &http.Response{StatusCode: http.StatusNotFound},
				Message:  "404 Not Found",
			},
			code: http.StatusNotFound,
			want: true,
		},
		{
			name: "non-matching status",
			err: &gl.ErrorResponse{
				Response: &http.Response{StatusCode: http.StatusForbidden},
				Message:  "403 Forbidden",
			},
			code: http.StatusNotFound,
			want: false,
		},
		{
			name: "nil response in ErrorResponse",
			err:  &gl.ErrorResponse{Response: nil, Message: "no response"},
			code: http.StatusNotFound,
			want: false,
		},
		{
			name: "non-GitLab error",
			err:  errors.New("some other error"),
			code: http.StatusNotFound,
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			code: http.StatusNotFound,
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsHTTPStatus(tt.err, tt.code)
			if got != tt.want {
				t.Errorf("IsHTTPStatus(%v, %d) = %v, want %v", tt.err, tt.code, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ExtractGitLabMessage tests
// ---------------------------------------------------------------------------

// TestExtractGitLabMessage verifies extraction of specific error messages from
// GitLab ErrorResponse, filtering out redundant HTTP status text.
func TestExtractGitLabMessage(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "specific validation error",
			err: &gl.ErrorResponse{
				Response: &http.Response{StatusCode: http.StatusConflict},
				Message:  "{message: {base: [Another open merge request already exists for this source branch]}}",
			},
			want: "{message: {base: [Another open merge request already exists for this source branch]}}",
		},
		{
			name: "simple error message",
			err: &gl.ErrorResponse{
				Response: &http.Response{StatusCode: http.StatusBadRequest},
				Message:  "{message: Branch already exists}",
			},
			want: "{message: Branch already exists}",
		},
		{
			name: "status-only message filtered out",
			err: &gl.ErrorResponse{
				Response: &http.Response{StatusCode: http.StatusMethodNotAllowed},
				Message:  "405 Method Not Allowed",
			},
			want: "",
		},
		{
			name: "wrapped status-only message filtered out",
			err: &gl.ErrorResponse{
				Response: &http.Response{StatusCode: http.StatusMethodNotAllowed},
				Message:  "{message: 405 Method Not Allowed}",
			},
			want: "",
		},
		{
			name: "empty message",
			err: &gl.ErrorResponse{
				Response: &http.Response{StatusCode: http.StatusInternalServerError},
				Message:  "",
			},
			want: "",
		},
		{
			name: "non-GitLab error",
			err:  errors.New("some other error"),
			want: "",
		},
		{
			name: "nil error",
			err:  nil,
			want: "",
		},
		{
			name: "nil response still extracts message",
			err: &gl.ErrorResponse{
				Response: nil,
				Message:  "useful error info",
			},
			want: "useful error info",
		},
		{
			name: "truncates long messages",
			err: &gl.ErrorResponse{
				Response: &http.Response{StatusCode: http.StatusBadRequest},
				Message:  strings.Repeat("a", 400),
			},
			want: strings.Repeat("a", 300) + "…",
		},
		{
			name: "array error with brackets preserved",
			err: &gl.ErrorResponse{
				Response: &http.Response{StatusCode: http.StatusUnprocessableEntity},
				Message:  "{error: [title is too long (maximum is 255 characters)]}",
			},
			want: "{error: [title is too long (maximum is 255 characters)]}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractGitLabMessage(tt.err)
			if got != tt.want {
				t.Errorf("ExtractGitLabMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// WrapErrWithMessage tests
// ---------------------------------------------------------------------------

// TestWrapErrWithMessage_IncludesGitLabMessage verifies that WrapErrWithMessage
// includes the specific GitLab error message in addition to the classification.
func TestWrapErrWithMessage_IncludesGitLabMessage(t *testing.T) {
	err := &gl.ErrorResponse{
		Response: &http.Response{
			StatusCode: http.StatusConflict,
			Request:    &http.Request{Method: http.MethodPost, URL: &url.URL{Path: "/api/v4/projects/1/merge_requests"}},
		},
		Message: "{message: {base: [Another open merge request already exists]}}",
	}
	wrapped := WrapErrWithMessage("mrCreate", err)

	msg := wrapped.Error()
	if !strings.Contains(msg, "mrCreate") {
		t.Errorf("expected operation prefix, got: %s", msg)
	}
	if !strings.Contains(msg, "conflict") {
		t.Errorf("expected classification, got: %s", msg)
	}
	if !strings.Contains(msg, "Another open merge request") {
		t.Errorf("expected GitLab message detail, got: %s", msg)
	}
	// Verify error chain preserved
	var glErr *gl.ErrorResponse
	if !errors.As(wrapped, &glErr) {
		t.Error("expected errors.As to find gl.ErrorResponse in chain")
	}
}

// TestWrapErrWithMessage_StatusOnlyFallback verifies that when the GitLab
// message is just a status text, WrapErrWithMessage falls back to the
// classification-only format.
func TestWrapErrWithMessage_StatusOnlyFallback(t *testing.T) {
	err := &gl.ErrorResponse{
		Response: &http.Response{
			StatusCode: http.StatusMethodNotAllowed,
			Request:    &http.Request{Method: http.MethodPut, URL: &url.URL{Path: "/api/v4/projects/1/merge_requests/1/merge"}},
		},
		Message: "405 Method Not Allowed",
	}
	wrapped := WrapErrWithMessage("mrMerge", err)
	msg := wrapped.Error()
	// Status-only message is filtered, so format should be "op: classification: original"
	// not "op: classification — detail: original"
	if !strings.Contains(msg, "mrMerge") {
		t.Errorf("expected operation prefix, got: %s", msg)
	}
	if !strings.Contains(msg, "method not allowed") {
		t.Errorf("expected classification, got: %s", msg)
	}
}

// TestWrapErrWithMessage_NonGitLabError verifies fallback for non-GitLab errors.
func TestWrapErrWithMessage_NonGitLabError(t *testing.T) {
	err := errors.New("connection reset")
	wrapped := WrapErrWithMessage("fileGet", err)
	msg := wrapped.Error()
	if !strings.Contains(msg, "fileGet") {
		t.Errorf("expected operation prefix, got: %s", msg)
	}
	if !strings.Contains(msg, msgUnexpectedErr) {
		t.Errorf("expected unexpected error classification, got: %s", msg)
	}
}

// ---------------------------------------------------------------------------
// WrapErrWithHint tests
// ---------------------------------------------------------------------------

// TestWrapErrWithHint_IncludesHintAndMessage verifies that WrapErrWithHint
// includes both the GitLab message and the actionable hint.
func TestWrapErrWithHint_IncludesHintAndMessage(t *testing.T) {
	err := &gl.ErrorResponse{
		Response: &http.Response{
			StatusCode: http.StatusBadRequest,
			Request:    &http.Request{Method: http.MethodDelete, URL: &url.URL{Path: "/api/v4/projects/1/repository/branches/main"}},
		},
		Message: "{message: Cannot delete: protected branch}",
	}
	wrapped := WrapErrWithHint("branchDelete", err,
		"use gitlab_branch_unprotect first, then retry deletion")

	msg := wrapped.Error()
	if !strings.Contains(msg, "branchDelete") {
		t.Errorf("expected operation prefix, got: %s", msg)
	}
	if !strings.Contains(msg, "Cannot delete: protected branch") {
		t.Errorf("expected GitLab message, got: %s", msg)
	}
	if !strings.Contains(msg, "Suggestion:") {
		t.Errorf("expected hint marker, got: %s", msg)
	}
	if !strings.Contains(msg, "gitlab_branch_unprotect") {
		t.Errorf("expected hint content, got: %s", msg)
	}
	// Verify error chain preserved
	var glErr *gl.ErrorResponse
	if !errors.As(wrapped, &glErr) {
		t.Error("expected errors.As to find gl.ErrorResponse in chain")
	}
}

// TestWrapErrWithHint_NoGitLabMessage verifies that when there's no specific
// GitLab message, the hint is still appended to the classification.
func TestWrapErrWithHint_NoGitLabMessage(t *testing.T) {
	err := &gl.ErrorResponse{
		Response: &http.Response{
			StatusCode: http.StatusMethodNotAllowed,
			Request:    &http.Request{Method: http.MethodPut, URL: &url.URL{Path: "/api/v4/projects/1/merge_requests/1/merge"}},
		},
		Message: "405 Method Not Allowed",
	}
	wrapped := WrapErrWithHint("mrMerge", err, "check merge_status field")
	msg := wrapped.Error()
	if !strings.Contains(msg, "Suggestion: check merge_status field") {
		t.Errorf("expected hint even without GitLab message, got: %s", msg)
	}
}

// TestWrapErrWithStatusHint_MatchAppliesHint verifies that when the error
// matches the requested HTTP status, the hint is appended just like
// WrapErrWithHint would.
func TestWrapErrWithStatusHint_MatchAppliesHint(t *testing.T) {
	err := &gl.ErrorResponse{
		Response: &http.Response{
			StatusCode: http.StatusNotFound,
			Request:    &http.Request{Method: http.MethodGet, URL: &url.URL{Path: "/api/v4/projects/1"}},
		},
		Message: "{message: 404 Project Not Found}",
	}
	wrapped := WrapErrWithStatusHint("projectGet", err, http.StatusNotFound,
		"verify project_id with gitlab_project_list")
	msg := wrapped.Error()
	if !strings.Contains(msg, "Suggestion: verify project_id with gitlab_project_list") {
		t.Errorf("expected hint to be appended on status match, got: %s", msg)
	}
}

// TestWrapErrWithStatusHint_NoMatchFallsBack verifies that when the error
// does not match the requested HTTP status, WrapErrWithStatusHint falls back
// to WrapErrWithMessage (no Suggestion clause).
func TestWrapErrWithStatusHint_NoMatchFallsBack(t *testing.T) {
	err := &gl.ErrorResponse{
		Response: &http.Response{
			StatusCode: http.StatusForbidden,
			Request:    &http.Request{Method: http.MethodGet, URL: &url.URL{Path: "/api/v4/projects/1"}},
		},
		Message: "{message: 403 Forbidden}",
	}
	wrapped := WrapErrWithStatusHint("projectGet", err, http.StatusNotFound,
		"verify project_id with gitlab_project_list")
	msg := wrapped.Error()
	if strings.Contains(msg, "Suggestion:") {
		t.Errorf("expected no Suggestion clause on status mismatch, got: %s", msg)
	}
	if !strings.Contains(msg, "access denied") {
		t.Errorf("expected fallback classification, got: %s", msg)
	}
}

// TestIsHTTPStatus_ErrNotFound verifies that IsHTTPStatus recognizes the
// sentinel gl.ErrNotFound for code 404 without requiring a full ErrorResponse.
func TestIsHTTPStatus_ErrNotFound(t *testing.T) {
	if !IsHTTPStatus(gl.ErrNotFound, http.StatusNotFound) {
		t.Error("expected true for gl.ErrNotFound with 404")
	}
	if IsHTTPStatus(gl.ErrNotFound, http.StatusForbidden) {
		t.Error("expected false for gl.ErrNotFound with 403")
	}
}

// TestIsHTTPStatus_WrappedErrNotFound verifies that a wrapped gl.ErrNotFound
// is still recognized via errors.Is.
func TestIsHTTPStatus_WrappedErrNotFound(t *testing.T) {
	wrapped := fmt.Errorf("some context: %w", gl.ErrNotFound)
	if !IsHTTPStatus(wrapped, http.StatusNotFound) {
		t.Error("expected true for wrapped gl.ErrNotFound with 404")
	}
}

// TestIsNotFound verifies that IsNotFound detects 404 via structured ErrorResponse,
// plain-text error messages, and rejects non-404 errors including port numbers
// that happen to contain "404".
func TestIsNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "structured 404 ErrorResponse",
			err: &gl.ErrorResponse{
				Response: &http.Response{StatusCode: http.StatusNotFound},
			},
			want: true,
		},
		{
			name: "sentinel gl.ErrNotFound",
			err:  gl.ErrNotFound,
			want: true,
		},
		{
			name: "plain text 404 Not Found",
			err:  fmt.Errorf("404 Not Found"),
			want: true,
		},
		{
			name: "wrapped plain text 404 Not Found",
			err:  fmt.Errorf("GET http://example.com/api: 404 Not Found"),
			want: true,
		},
		{
			name: "403 error should not match",
			err:  fmt.Errorf("GET http://example.com/api: 403 Forbidden"),
			want: false,
		},
		{
			name: "port containing 404 should not match",
			err:  fmt.Errorf("GET http://127.0.0.1:40456/api/v4/projects: 403 Forbidden"),
			want: false,
		},
		{
			name: "port 40400 should not match",
			err:  fmt.Errorf("GET http://127.0.0.1:40400/api/v4/projects: 500 Internal Server Error"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsNotFound(tt.err)
			if got != tt.want {
				t.Errorf("IsNotFound() = %v, want %v", got, tt.want)
			}
		})
	}
}
