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
	"reflect"
	"strings"
	"syscall"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

const (
	// fmtErrWant identifies the fmt err want constant used by this package.
	fmtErrWant = "Error() = %q, want %q"
	// msgUnexpectedErr identifies the msg unexpected err constant used by this package.
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
	err := errors.New("dial tcp 10.0.0.1:443: connection refused")
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
	err := errors.New("Get https://gitlab.example.com: x509: certificate signed by unknown authority")
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

// Error returns the error message for timeoutError.
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

// TestDetailedError_MarkdownMinimal verifies Markdown with only required fields.
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
			err:  errors.New("404 Not Found"),
			want: true,
		},
		{
			name: "wrapped plain text 404 Not Found",
			err:  errors.New("GET http://example.com/api: 404 Not Found"),
			want: true,
		},
		{
			name: "403 error should not match",
			err:  errors.New("GET http://example.com/api: 403 Forbidden"),
			want: false,
		},
		{
			name: "port containing 404 should not match",
			err:  errors.New("GET http://127.0.0.1:40456/api/v4/projects: 403 Forbidden"),
			want: false,
		},
		{
			name: "port 40400 should not match",
			err:  errors.New("GET http://127.0.0.1:40400/api/v4/projects: 500 Internal Server Error"),
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

// acceptsTrue is a convenience acceptor predicate for tests that drive
// alias helpers directly.
func acceptsTrue(_ string) bool { return true }

func acceptsFalse(_ string) bool { return false }

func fieldSet(names ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(names))
	for _, name := range names {
		out[name] = struct{}{}
	}
	return out
}

// aliasClone returns a "clone" callback matching the production
// normalizeParamAliasesWithFields pattern. For test purposes we return
// the same map on every call so that mutations made through the clone
// callback are visible to the caller; this mirrors what the production
// helper does after the first clone (it reuses the same backing map).
// Tests using this helper should treat the supplied params as
// in-out: the same map will be mutated in place.
func aliasClone(initial map[string]any) (clone func() map[string]any, _ *map[string]any) {
	clone = func() map[string]any { return initial }
	return clone, nil
}

// TestExplainIIDParamAliasDirect verifies the explanation helper
// detects the iid→merge_request_iid alias only when the canonical
// field is present in the target schema, the alias is provided, and
// there is exactly one candidate canonical field.
func TestExplainIIDParamAliasDirect(t *testing.T) {
	// accepts("iid") must be false for the helper to consider it an alias.
	acceptsNoIID := func(name string) bool { return name != "iid" }
	fields := fieldSet("merge_request_iid", "title")
	params := map[string]any{"iid": "1"}

	explanation, ok := explainIIDParamAlias(params, fields, acceptsNoIID)
	if !ok {
		t.Fatal("explainIIDParamAlias = false, want true")
	}
	if explanation.Canonical != "merge_request_iid" {
		t.Errorf("Canonical = %q, want merge_request_iid", explanation.Canonical)
	}
	if explanation.Alias != "iid" {
		t.Errorf("Alias = %q, want iid", explanation.Alias)
	}
	if explanation.Source != "schema_common" {
		t.Errorf("Source = %q, want schema_common", explanation.Source)
	}

	// canonical field is absent from fields → no explanation
	if _, hasMatch := explainIIDParamAlias(params, fieldSet("title"), acceptsNoIID); hasMatch {
		t.Error("explainIIDParamAlias(canonical missing) = true, want false")
	}

	// iid param is absent → no explanation
	if _, hasMatch := explainIIDParamAlias(map[string]any{}, fields, acceptsNoIID); hasMatch {
		t.Error("explainIIDParamAlias(no iid) = true, want false")
	}

	// multiple _iid fields → no explanation (ambiguous)
	if _, hasMatch := explainIIDParamAlias(params, fieldSet("merge_request_iid", "issue_iid"), acceptsNoIID); hasMatch {
		t.Error("explainIIDParamAlias(ambiguous) = true, want false")
	}

	// accepts("iid") is true → no explanation
	if _, hasMatch := explainIIDParamAlias(params, fields, acceptsTrue); hasMatch {
		t.Error("explainIIDParamAlias(iid accepted) = true, want false")
	}
}

// TestExplainEnvironmentIDParamAliasDirect verifies the helper produces
// an explanation only when environment_id is supplied, environment is
// accepted, and environment_id is not accepted.
func TestExplainEnvironmentIDParamAliasDirect(t *testing.T) {
	// accepts("environment_id") must be false; accepts("environment") must be true.
	acceptsEnv := func(name string) bool { return name == "environment" }
	params := map[string]any{"environment_id": "prod"}

	explanation, ok := explainEnvironmentIDParamAlias(params, acceptsEnv)
	if !ok {
		t.Fatal("explainEnvironmentIDParamAlias = false, want true")
	}
	if explanation.Alias != "environment_id" || explanation.Canonical != "environment" {
		t.Errorf("explanation = %+v, want environment_id→environment", explanation)
	}

	if _, hasMatch := explainEnvironmentIDParamAlias(map[string]any{}, acceptsEnv); hasMatch {
		t.Error("explainEnvironmentIDParamAlias(no param) = true, want false")
	}
	if _, hasMatch := explainEnvironmentIDParamAlias(params, acceptsFalse); hasMatch {
		t.Error("explainEnvironmentIDParamAlias(no env accepted) = true, want false")
	}
	// environment_id is accepted → no explanation
	if _, hasMatch := explainEnvironmentIDParamAlias(params, acceptsTrue); hasMatch {
		t.Error("explainEnvironmentIDParamAlias(environment_id accepted) = true, want false")
	}
}

// TestRemoveContextOnlyDiscussionIDDirect verifies the helper drops a
// stray discussion_id when the target schema accepts note_id and the
// caller has already supplied note_id.
func TestRemoveContextOnlyDiscussionIDDirect(t *testing.T) {
	acceptsNoteID := func(name string) bool { return name == "note_id" }
	acceptsAll := func(_ string) bool { return true }

	t.Run("removes when note_id is canonical and present", func(t *testing.T) {
		params := map[string]any{"discussion_id": "abc", "note_id": 7}
		clone, _ := aliasClone(params)
		removeContextOnlyDiscussionID(params, acceptsNoteID, clone)
		if _, ok := params["discussion_id"]; ok {
			t.Errorf("discussion_id not removed: %+v", params)
		}
		if params["note_id"] != 7 {
			t.Errorf("note_id = %v, want 7", params["note_id"])
		}
	})

	t.Run("keeps when schema accepts discussion_id", func(t *testing.T) {
		params := map[string]any{"discussion_id": "abc", "note_id": 7}
		clone, _ := aliasClone(params)
		removeContextOnlyDiscussionID(params, acceptsAll, clone)
		if _, ok := params["discussion_id"]; !ok {
			t.Error("discussion_id removed even though schema accepts it")
		}
	})

	t.Run("keeps when note_id is absent", func(t *testing.T) {
		params := map[string]any{"discussion_id": "abc"}
		clone, _ := aliasClone(params)
		removeContextOnlyDiscussionID(params, acceptsNoteID, clone)
		if _, ok := params["discussion_id"]; !ok {
			t.Error("discussion_id removed even though note_id is absent")
		}
	})
}

// TestNormalizeActiveAliasDirect verifies the active→paused negation
// helper behaves correctly for accepted/present/missing combinations
// and non-bool inputs.
func TestNormalizeActiveAliasDirect(t *testing.T) {
	acceptsPaused := func(name string) bool { return name == "paused" }

	t.Run("active false becomes paused true", func(t *testing.T) {
		params := map[string]any{"active": false}
		clone, _ := aliasClone(params)
		normalizeActiveAlias(params, acceptsPaused, clone)
		if v, ok := params["paused"]; !ok || v != true {
			t.Errorf("paused = %v/%v, want true", v, ok)
		}
		if _, ok := params["active"]; ok {
			t.Error("active not removed")
		}
	})

	t.Run("active true becomes paused false", func(t *testing.T) {
		params := map[string]any{"active": true}
		clone, _ := aliasClone(params)
		normalizeActiveAlias(params, acceptsPaused, clone)
		if v, ok := params["paused"]; !ok || v != false {
			t.Errorf("paused = %v/%v, want false", v, ok)
		}
	})

	t.Run("no-op when paused not accepted", func(t *testing.T) {
		params := map[string]any{"active": false}
		clone, _ := aliasClone(params)
		normalizeActiveAlias(params, acceptsFalse, clone)
		if _, ok := params["paused"]; ok {
			t.Error("paused added when schema does not accept it")
		}
	})

	t.Run("preserves existing paused value", func(t *testing.T) {
		params := map[string]any{"active": false, "paused": true}
		clone, _ := aliasClone(params)
		normalizeActiveAlias(params, acceptsPaused, clone)
		if params["paused"] != true {
			t.Errorf("paused = %v, want true (preserved)", params["paused"])
		}
	})

	t.Run("non-bool active is ignored", func(t *testing.T) {
		params := map[string]any{"active": "yes"}
		clone, _ := aliasClone(params)
		normalizeActiveAlias(params, acceptsPaused, clone)
		if _, ok := params["paused"]; ok {
			t.Error("paused added for non-bool active")
		}
	})
}

// TestNormalizeFilePathAliasDirect verifies the file_path → path+filename
// split helper covers each branch.
func TestNormalizeFilePathAliasDirect(t *testing.T) {
	acceptsBoth := func(name string) bool { return name == "path" || name == "filename" }

	t.Run("splits file_path into path+filename", func(t *testing.T) {
		params := map[string]any{"file_path": "packages/npm/pkg.tgz"}
		clone, _ := aliasClone(params)
		normalizeFilePathAlias(params, acceptsBoth, clone)
		if params["path"] != "packages/npm" {
			t.Errorf("path = %q, want packages/npm", params["path"])
		}
		if params["filename"] != "pkg.tgz" {
			t.Errorf("filename = %q, want pkg.tgz", params["filename"])
		}
		if _, ok := params["file_path"]; ok {
			t.Error("file_path not removed")
		}
	})

	t.Run("preserves existing path", func(t *testing.T) {
		params := map[string]any{"file_path": "packages/npm/pkg.tgz", "path": "custom"}
		clone, _ := aliasClone(params)
		normalizeFilePathAlias(params, acceptsBoth, clone)
		if params["path"] != "custom" {
			t.Errorf("path = %q, want custom (preserved)", params["path"])
		}
	})

	t.Run("non-string file_path is ignored", func(t *testing.T) {
		params := map[string]any{"file_path": 42}
		clone, _ := aliasClone(params)
		normalizeFilePathAlias(params, acceptsBoth, clone)
		if _, ok := params["path"]; ok {
			t.Error("path added for non-string file_path")
		}
	})

	t.Run("empty file_path is ignored", func(t *testing.T) {
		params := map[string]any{"file_path": ""}
		clone, _ := aliasClone(params)
		normalizeFilePathAlias(params, acceptsBoth, clone)
		if _, ok := params["path"]; ok {
			t.Error("path added for empty file_path")
		}
	})
}

// TestNormalizeIIDAliasDirect verifies the iid→canonical-_iid mapping
// for single, missing, ambiguous, and accepted-iid scenarios.
func TestNormalizeIIDAliasDirect(t *testing.T) {
	acceptsMRIID := func(name string) bool { return name == "merge_request_iid" }
	acceptsNoIID := func(name string) bool { return name != "iid" }

	t.Run("iid is renamed to merge_request_iid", func(t *testing.T) {
		params := map[string]any{"iid": "5"}
		clone, _ := aliasClone(params)
		normalizeIIDAlias(params, fieldSet("merge_request_iid"), acceptsMRIID, clone)
		if params["merge_request_iid"] != "5" {
			t.Errorf("merge_request_iid = %v, want 5", params["merge_request_iid"])
		}
		if _, ok := params["iid"]; ok {
			t.Error("iid not removed")
		}
	})

	t.Run("no rename when iid is accepted", func(t *testing.T) {
		params := map[string]any{"iid": "5"}
		clone, _ := aliasClone(params)
		normalizeIIDAlias(params, fieldSet("iid"), acceptsNoIID, clone)
		if _, ok := params["merge_request_iid"]; ok {
			t.Error("iid was renamed even though it is accepted")
		}
	})

	t.Run("ambiguous canonical field → no rename", func(t *testing.T) {
		params := map[string]any{"iid": "5"}
		clone, _ := aliasClone(params)
		normalizeIIDAlias(params, fieldSet("merge_request_iid", "issue_iid"), acceptsNoIID, clone)
		if _, ok := params["merge_request_iid"]; ok {
			t.Error("iid was renamed when canonical was ambiguous")
		}
	})

	t.Run("missing canonical field → no rename", func(t *testing.T) {
		params := map[string]any{"iid": "5"}
		clone, _ := aliasClone(params)
		normalizeIIDAlias(params, fieldSet("title"), acceptsNoIID, clone)
		if _, ok := params["merge_request_iid"]; ok {
			t.Error("iid was renamed when no canonical field exists")
		}
	})
}

// TestNormalizeEnvironmentNameAliasDirect verifies the environment→name
// alias rewrite covers acceptance, name-already-present, and
// environment_scope-accepted branches.
func TestNormalizeEnvironmentNameAliasDirect(t *testing.T) {
	acceptsName := func(name string) bool { return name == "name" }
	acceptsScope := func(name string) bool { return name == "name" || name == "environment_scope" }

	t.Run("environment is renamed to name", func(t *testing.T) {
		params := map[string]any{"environment": "production"}
		clone, _ := aliasClone(params)
		normalizeEnvironmentNameAlias(params, acceptsName, clone)
		if params["name"] != "production" {
			t.Errorf("name = %q, want production", params["name"])
		}
		if _, ok := params["environment"]; ok {
			t.Error("environment not removed")
		}
	})

	t.Run("preserves existing name and drops environment", func(t *testing.T) {
		params := map[string]any{"environment": "production", "name": "stage"}
		clone, _ := aliasClone(params)
		normalizeEnvironmentNameAlias(params, acceptsName, clone)
		if params["name"] != "stage" {
			t.Errorf("name = %q, want stage (preserved)", params["name"])
		}
		if _, ok := params["environment"]; ok {
			t.Error("environment not removed when name was present")
		}
	})

	t.Run("no-op when environment_scope is accepted", func(t *testing.T) {
		params := map[string]any{"environment": "production"}
		clone, _ := aliasClone(params)
		normalizeEnvironmentNameAlias(params, acceptsScope, clone)
		if _, ok := params["name"]; ok {
			t.Error("name was added despite environment_scope being accepted")
		}
	})
}

// TestDecodeEncodedPathIdentifierDirect verifies the %2f decoder
// returns the decoded path with the changed flag and rejects paths
// that do not contain an encoded slash.
func TestDecodeEncodedPathIdentifierDirect(t *testing.T) {
	got, changed := decodeEncodedPathIdentifier("group%2Fsubgroup%2Fproject")
	if !changed {
		t.Fatal("decodeEncodedPathIdentifier(%2F) changed = false, want true")
	}
	if got != "group/subgroup/project" {
		t.Errorf("decoded = %q, want group/subgroup/project", got)
	}

	// URL percent-encoding is case-insensitive: lowercase %2f must
	// decode the same as uppercase %2F.
	gotLower, changedLower := decodeEncodedPathIdentifier("group%2fsubgroup%2fproject")
	if !changedLower {
		t.Fatal("decodeEncodedPathIdentifier(%2f) changed = false, want true")
	}
	if gotLower != "group/subgroup/project" {
		t.Errorf("decoded = %q, want group/subgroup/project", gotLower)
	}

	if _, isChanged := decodeEncodedPathIdentifier("plain/path"); isChanged {
		t.Error("decodeEncodedPathIdentifier(no %2F) changed = true, want false")
	}
	if _, isChanged := decodeEncodedPathIdentifier(""); isChanged {
		t.Error("decodeEncodedPathIdentifier(empty) changed = true, want false")
	}
	if _, isChanged := decodeEncodedPathIdentifier("no-slash-here"); isChanged {
		t.Error("decodeEncodedPathIdentifier(no slash) changed = true, want false")
	}
}

// TestCloneAccessLevelAliasesDirect verifies the alias-to-access_level
// promotion covers the access_level-already-present, known-alias
// matches, no-alias-found, and unhandled-alias scenarios.
func TestCloneAccessLevelAliasesDirect(t *testing.T) {
	// access_level present → no clone
	original := map[string]any{"access_level": 30, "deploy_access_level": 40}
	cloneFn := func() map[string]any {
		t.Fatal("clone() should not be called when access_level is present")
		return nil
	}
	if got := cloneAccessLevelAliases(original, cloneFn); !reflect.DeepEqual(got, original) {
		t.Errorf("got = %+v, want original (access_level present)", got)
	}

	// deploy_access_level present → promoted
	original = map[string]any{"deploy_access_level": 30}
	called := false
	cloned := cloneAccessLevelAliases(original, func() map[string]any {
		called = true
		return map[string]any{}
	})
	if !called {
		t.Error("clone() not called for deploy_access_level")
	}
	if cloned["access_level"] != 30 {
		t.Errorf("access_level = %v, want 30", cloned["access_level"])
	}
	if _, ok := cloned["deploy_access_level"]; ok {
		t.Error("deploy_access_level not removed")
	}

	// no access-level field at all → unchanged
	original = map[string]any{"name": "main"}
	called = false
	got := cloneAccessLevelAliases(original, func() map[string]any {
		called = true
		return map[string]any{}
	})
	if called {
		t.Error("clone() called when no access-level alias is present")
	}
	if !reflect.DeepEqual(got, original) {
		t.Errorf("got = %+v, want original (no alias)", got)
	}
}

// TestHasStructuredApprovalCountDirect verifies the approval-count
// detector accepts both the canonical field and the alias family.
func TestHasStructuredApprovalCountDirect(t *testing.T) {
	for _, key := range []string{"required_approvals", "required_approval_count", "approval_count", "approvals_required"} {
		if !hasStructuredApprovalCount(map[string]any{key: 2}) {
			t.Errorf("hasStructuredApprovalCount(%s) = false, want true", key)
		}
	}
	if hasStructuredApprovalCount(map[string]any{"other": 1}) {
		t.Error("hasStructuredApprovalCount(other) = true, want false")
	}
	if hasStructuredApprovalCount(map[string]any{}) {
		t.Error("hasStructuredApprovalCount(empty) = true, want false")
	}
}

// TestHasStructuredApprovalPrincipalDirect verifies the principal
// detector accepts both canonical fields and the access-level alias
// family used for protected-branch entries.
func TestHasStructuredApprovalPrincipalDirect(t *testing.T) {
	canonical := []string{"access_level", "user_id", "group_id"}
	aliases := []string{"deploy_access_level", "group_access_level", "project_access_level", "machine_user_access_level"}

	for _, key := range append(canonical, aliases...) {
		if !hasStructuredApprovalPrincipal(map[string]any{key: 1}) {
			t.Errorf("hasStructuredApprovalPrincipal(%s) = false, want true", key)
		}
	}
	if hasStructuredApprovalPrincipal(map[string]any{"name": "alice"}) {
		t.Error("hasStructuredApprovalPrincipal(name) = true, want false")
	}
	if hasStructuredApprovalPrincipal(map[string]any{}) {
		t.Error("hasStructuredApprovalPrincipal(empty) = true, want false")
	}
}

// TestSchemaPropertyHasTypeDirect verifies the schema-property type
// detector covers string, []string, []any, and non-object inputs.
func TestSchemaPropertyHasTypeDirect(t *testing.T) {
	if !schemaPropertyHasType(map[string]any{"type": "string"}, "string") {
		t.Error("schemaPropertyHasType(string) = false, want true")
	}
	if !schemaPropertyHasType(map[string]any{"type": []string{"integer", "string"}}, "string") {
		t.Error("schemaPropertyHasType([]string) = false, want true")
	}
	if !schemaPropertyHasType(map[string]any{"type": []any{"integer", "string"}}, "integer") {
		t.Error("schemaPropertyHasType([]any) = false, want true")
	}
	if schemaPropertyHasType(map[string]any{"type": "string"}, "integer") {
		t.Error("schemaPropertyHasType(mismatch) = true, want false")
	}
	if schemaPropertyHasType("not-an-object", "string") {
		t.Error("schemaPropertyHasType(non-object) = true, want false")
	}
	if schemaPropertyHasType(map[string]any{}, "string") {
		t.Error("schemaPropertyHasType(empty) = true, want false")
	}
}

// TestGitLabRoleAccessLevelStringAliases verifies that all canonical
// GitLab role names — including plural forms and underscored/hyphenated
// variants — map to the expected numeric access level.
func TestGitLabRoleAccessLevelStringAliases(t *testing.T) {
	cases := []struct {
		role string
		want int
	}{
		{"guest", 10},
		{"guests", 10},
		{"reporter", 20},
		{"reporters", 20},
		{"developer", 30},
		{"developers", 30},
		{"maintainer", 40},
		{"maintainers", 40},
		{"owner", 50},
		{"owners", 50},
		{"admin", 60},
		{"admins", 60},
		{"administrator", 60},
		{"administrators", 60},
		{"no access", 0},
		{"no one", 0},
		{"nobody", 0},
		{"none", 0},
		// case-insensitive
		{"MAINTAINER", 40},
	}
	for _, tc := range cases {
		got, ok := gitLabRoleAccessLevel(tc.role)
		if !ok || got != tc.want {
			t.Errorf("gitLabRoleAccessLevel(%q) = %d/%v, want %d/true", tc.role, got, ok, tc.want)
		}
	}

	// unknown role → rejected
	if _, ok := gitLabRoleAccessLevel("wizard"); ok {
		t.Error("gitLabRoleAccessLevel(wizard) ok = true, want false")
	}
	// non-string, non-numeric value → rejected
	if _, ok := gitLabRoleAccessLevel(true); ok {
		t.Error("gitLabRoleAccessLevel(bool) ok = true, want false")
	}
}

// TestHasUnknownParamNamesDirect verifies the unknown-parameter
// detector handles empty params, missing properties, and unknown keys.
func TestHasUnknownParamNamesDirect(t *testing.T) {
	// empty params → false
	if hasUnknownParamNames(map[string]any{"x": 1}, map[string]any{}) {
		t.Error("hasUnknownParamNames(empty) = true, want false")
	}
	// schema has no properties → false
	if hasUnknownParamNames(map[string]any{}, map[string]any{"unknown": 1}) {
		t.Error("hasUnknownParamNames(no props) = true, want false")
	}
	// unknown key present → true
	schema := map[string]any{"properties": map[string]any{"known": map[string]any{}}}
	if !hasUnknownParamNames(schema, map[string]any{"unknown": 1}) {
		t.Error("hasUnknownParamNames(unknown) = false, want true")
	}
	// only known keys → false
	if hasUnknownParamNames(schema, map[string]any{"known": 1}) {
		t.Error("hasUnknownParamNames(known only) = true, want false")
	}
}

// TestCollectJSONFieldTypesDirect verifies the recursive JSON field
// type collector handles embedded (anonymous) struct fields.
func TestCollectJSONFieldTypesDirect(t *testing.T) {
	type inner struct {
		Name string `json:"name"`
	}
	type outer struct {
		inner
		ID int `json:"id"`
	}
	fields := map[string]reflect.Type{}
	collectJSONFieldTypes(reflect.TypeFor[outer](), fields)
	if _, ok := fields["name"]; !ok {
		t.Error("collectJSONFieldTypes missing embedded 'name' field")
	}
	if _, ok := fields["id"]; !ok {
		t.Error("collectJSONFieldTypes missing 'id' field")
	}
}
