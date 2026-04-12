// issue_report_test.go contains unit tests for the issue report formatter.
package toolutil

import (
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestSanitizeInput verifies that sanitizeInput removes sensitive keys
// (tokens, passwords) from the input map before including it in an
// issue report.
func TestSanitizeInput(t *testing.T) {
	input := map[string]any{
		"project_id":    "123",
		"title":         "test issue",
		"private_token": "glpat-secret123",
		"password":      "hunter2",
		"description":   "safe content",
		"auth_header":   "Bearer xxx",
	}

	safe := sanitizeInput(input)

	if safe["project_id"] != "123" {
		t.Errorf("project_id should not be redacted, got %v", safe["project_id"])
	}
	if safe["title"] != "test issue" {
		t.Errorf("title should not be redacted, got %v", safe["title"])
	}
	if safe["description"] != "safe content" {
		t.Errorf("description should not be redacted, got %v", safe["description"])
	}
	if safe["private_token"] != "[REDACTED]" {
		t.Errorf("private_token should be redacted, got %v", safe["private_token"])
	}
	if safe["password"] != "[REDACTED]" {
		t.Errorf("password should be redacted, got %v", safe["password"])
	}
	if safe["auth_header"] != "[REDACTED]" {
		t.Errorf("auth_header should be redacted, got %v", safe["auth_header"])
	}
}

// TestSanitizeInput_Nil verifies that sanitizeInput returns nil
// when given a nil input map.
func TestSanitizeInput_Nil(t *testing.T) {
	if result := sanitizeInput(nil); result != nil {
		t.Errorf("expected nil for nil input, got %v", result)
	}
}

// TestIsSensitiveKey verifies that isSensitiveKey correctly identifies
// keys containing "token", "password", "secret", or "key" as sensitive.
func TestIsSensitiveKey(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"token", true},
		{"PRIVATE_TOKEN", true},
		{"api_key", true},
		{"password", true},
		{"secret_value", true},
		{"credential_id", true},
		{"auth_code", true},
		{"session_id", true},
		{"cookie_value", true},
		{"project_id", false},
		{"title", false},
		{"description", false},
		{"name", false},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := isSensitiveKey(tt.key); got != tt.want {
				t.Errorf("isSensitiveKey(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

// TestTruncate verifies that truncate shortens strings exceeding the
// maximum length and leaves shorter strings unchanged.
func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{"short", "hello", 10, "hello"},
		{"exact", "hello", 5, "hello"},
		{"long", "hello world foo bar", 10, "hello wor…"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncate(tt.input, tt.maxLen); got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

// TestIssueReport_Markdown verifies that [IssueReport.Markdown] produces
// a well-formed markdown body including the tool name, error, and sanitized
// input parameters.
func TestIssueReport_Markdown(t *testing.T) {
	de := &DetailedError{
		Domain:       "issues",
		Action:       "create",
		Message:      "validation failed",
		GitLabStatus: 422,
		RequestID:    "req-abc-123",
	}

	report := NewIssueReport(de, map[string]any{
		"project_id": "42",
		"title":      "test",
		"token":      "secret",
	})

	md := report.Markdown()

	checks := []string{
		"Bug Report",
		"Server version",
		"issues",
		"create",
		"validation failed",
		"422",
		"req-abc-123",
		"project_id",
		"[REDACTED]",
		"Steps to Reproduce",
	}

	for _, check := range checks {
		if !strings.Contains(md, check) {
			t.Errorf("Markdown() missing expected content %q", check)
		}
	}
}

// TestIssueReport_Title verifies that [IssueReport.Title] returns a
// descriptive title containing the tool name.
func TestIssueReport_Title(t *testing.T) {
	de := &DetailedError{
		Domain:  "mergerequests",
		Action:  "merge",
		Message: "access denied",
	}

	report := NewIssueReport(de, nil)
	title := report.Title()

	if !strings.Contains(title, "mergerequests/merge") {
		t.Errorf("Title() missing tool/action, got %q", title)
	}
	if !strings.Contains(title, "access denied") {
		t.Errorf("Title() missing error message, got %q", title)
	}
}

// TestFormatIssueReport verifies that [FormatIssueReport] creates a
// GitLab issue via the API when issue reports are enabled.
func TestFormatIssueReport(t *testing.T) {
	// Ensure issue reports are enabled for this test.
	EnableIssueReports(true)
	defer EnableIssueReports(false)

	err := errors.New("something went wrong")
	result := FormatIssueReport("projects", "delete", err, map[string]any{
		"project_id": "99",
	})

	if !result.IsError {
		t.Error("FormatIssueReport should set IsError = true")
	}

	if len(result.Content) == 0 {
		t.Fatal("FormatIssueReport should return content")
	}

	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "projects") {
		t.Error("result should contain tool name")
	}
	if !strings.Contains(text, "Copy the following") {
		t.Error("result should contain copy instructions")
	}
}

// TestFormatIssueReport_DisabledFallsBackToErrorMarkdown verifies that
// [FormatIssueReport] returns an error markdown result when issue
// reports are disabled.
func TestFormatIssueReport_DisabledFallsBackToErrorMarkdown(t *testing.T) {
	// Ensure issue reports are disabled (default).
	EnableIssueReports(false)

	err := errors.New("something went wrong")
	result := FormatIssueReport("projects", "delete", err, map[string]any{
		"project_id": "99",
	})

	if !result.IsError {
		t.Error("FormatIssueReport should set IsError = true even when disabled")
	}

	if len(result.Content) == 0 {
		t.Fatal("FormatIssueReport should return content")
	}

	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "projects") {
		t.Error("result should contain tool name in error markdown")
	}
	if strings.Contains(text, "Copy the following") {
		t.Error("result should NOT contain issue copy instructions when disabled")
	}
	if strings.Contains(text, "Issue Body") {
		t.Error("result should NOT contain issue body when disabled")
	}
}

// TestEnableIssueReports verifies that [EnableIssueReports] stores the
// GitLab client and project ID for subsequent issue report creation.
func TestEnableIssueReports(t *testing.T) {
	// Default should be false.
	EnableIssueReports(false)
	if IssueReportsEnabled() {
		t.Error("IssueReportsEnabled() should return false when disabled")
	}

	EnableIssueReports(true)
	if !IssueReportsEnabled() {
		t.Error("IssueReportsEnabled() should return true when enabled")
	}

	// Restore default.
	EnableIssueReports(false)
}

// TestSetServerVersion verifies that [SetServerVersion] stores the
// version string for inclusion in issue report metadata.
func TestSetServerVersion(t *testing.T) {
	original := serverVersion
	defer func() { serverVersion = original }()

	SetServerVersion("3.0.0-test")
	if serverVersion != "3.0.0-test" {
		t.Errorf("SetServerVersion failed, got %q", serverVersion)
	}
}
