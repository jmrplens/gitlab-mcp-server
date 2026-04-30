// token_test.go contains unit tests for token extraction and validation.
package serverpool

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestExtractToken validates extract token across multiple scenarios using table-driven subtests.
func TestExtractToken(t *testing.T) {
	tests := []struct {
		name     string
		headers  map[string]string
		expected string
	}{
		{
			name:     "PRIVATE-TOKEN header",
			headers:  map[string]string{"PRIVATE-TOKEN": "glpat-abc123"},
			expected: "glpat-abc123",
		},
		{
			name:     "Bearer token",
			headers:  map[string]string{"Authorization": "Bearer glpat-xyz789"},
			expected: "glpat-xyz789",
		},
		{
			name: "PRIVATE-TOKEN takes precedence over Bearer",
			headers: map[string]string{
				"PRIVATE-TOKEN": "glpat-private",
				"Authorization": "Bearer glpat-bearer",
			},
			expected: "glpat-private",
		},
		{
			name:     "no headers returns empty",
			headers:  map[string]string{},
			expected: "",
		},
		{
			name:     "empty Bearer returns empty",
			headers:  map[string]string{"Authorization": "Bearer "},
			expected: "",
		},
		{
			name:     "Basic auth ignored",
			headers:  map[string]string{"Authorization": "Basic dXNlcjpwYXNz"},
			expected: "",
		},
		{
			name:     "Bearer without space ignored",
			headers:  map[string]string{"Authorization": "Bearertoken"},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			got := ExtractToken(req)
			if got != tt.expected {
				t.Errorf("ExtractToken() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// TestExtractGitLabURL validates GitLab URL extraction from GITLAB-URL header.
func TestExtractGitLabURL(t *testing.T) {
	tests := []struct {
		name       string
		header     string
		defaultURL string
		wantURL    string
		wantErr    bool
	}{
		{
			name:       "no header returns default",
			header:     "",
			defaultURL: "https://gitlab.com",
			wantURL:    "https://gitlab.com",
		},
		{
			name:       "valid HTTPS URL without default",
			header:     "https://gitlab.example.com",
			defaultURL: "",
			wantURL:    "https://gitlab.example.com",
		},
		{
			name:       "valid HTTP URL without default",
			header:     "http://gitlab.local:8080",
			defaultURL: "",
			wantURL:    "http://gitlab.local:8080",
		},
		{
			name:       "trailing slash stripped without default",
			header:     "https://gitlab.example.com/",
			defaultURL: "",
			wantURL:    "https://gitlab.example.com",
		},
		{
			name:       "whitespace trimmed without default",
			header:     "  https://gitlab.example.com  ",
			defaultURL: "",
			wantURL:    "https://gitlab.example.com",
		},
		{
			name:       "matching header accepted when default configured",
			header:     "https://gitlab.example.com/",
			defaultURL: "https://gitlab.example.com",
			wantURL:    "https://gitlab.example.com",
		},
		{
			name:       "different header ignored when default configured",
			header:     "https://other.gitlab.example.com",
			defaultURL: "https://gitlab.example.com",
			wantURL:    "https://gitlab.example.com",
		},
		{
			name:       "invalid scheme ignored when default configured",
			header:     "ftp://gitlab.example.com",
			defaultURL: "https://gitlab.com",
			wantURL:    "https://gitlab.com",
		},
		{
			name:       "invalid scheme rejected without default",
			header:     "ftp://gitlab.example.com",
			defaultURL: "",
			wantErr:    true,
		},
		{
			name:       "missing host rejected without default",
			header:     "https://",
			defaultURL: "",
			wantErr:    true,
		},
		{
			name:       "no header and no default returns empty",
			header:     "",
			defaultURL: "",
			wantURL:    "",
		},
		{
			name:       "whitespace-only header falls back to default",
			header:     "   ",
			defaultURL: "https://gitlab.com",
			wantURL:    "https://gitlab.com",
		},
		{
			name:       "default URL with trailing slash is normalized",
			header:     "",
			defaultURL: "https://gitlab.example.com/",
			wantURL:    "https://gitlab.example.com",
		},
		{
			name:       "uppercase scheme accepted and canonicalized without default",
			header:     "HTTPS://gitlab.example.com",
			defaultURL: "",
			wantURL:    "https://gitlab.example.com",
		},
		{
			name:       "malformed URL rejected without default",
			header:     "://not-a-url",
			defaultURL: "",
			wantErr:    true,
		},
		{
			name:       "URL with path preserved without default",
			header:     "https://gitlab.example.com/api",
			defaultURL: "",
			wantURL:    "https://gitlab.example.com/api",
		},
		{
			name:       "credentials rejected without default",
			header:     "https://user:secret@gitlab.example.com",
			defaultURL: "",
			wantErr:    true,
		},
		{
			name:       "query rejected without default",
			header:     "https://gitlab.example.com?token=secret",
			defaultURL: "",
			wantErr:    true,
		},
		{
			name:       "fragment rejected without default",
			header:     "https://gitlab.example.com#internal.example.com",
			defaultURL: "",
			wantErr:    true,
		},
		{
			name:       "invalid default URL is also rejected",
			header:     "",
			defaultURL: "ftp://bad-default.example.com",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", nil)
			if tt.header != "" {
				req.Header.Set("GITLAB-URL", tt.header)
			}
			got, err := ExtractGitLabURL(req, tt.defaultURL)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantURL {
				t.Errorf("ExtractGitLabURL() = %q, want %q", got, tt.wantURL)
			}
		})
	}
}

// TestResolveRequestOptions_IgnoredOptions verifies that server-wide MCP
// configuration records request options that were ignored.
func TestResolveRequestOptions_IgnoredOptions(t *testing.T) {
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", nil)
	req.Header.Set("GITLAB-URL", "https://other.gitlab.example.com")
	req.Header.Set("RATE-LIMIT-RPS", "999")
	req.Header.Set("META-PARAM-SCHEMA", "full")

	options, err := ResolveRequestOptions(req, "https://gitlab.example.com/")
	if err != nil {
		t.Fatalf("ResolveRequestOptions() error: %v", err)
	}
	if options.GitLabURL != "https://gitlab.example.com" {
		t.Fatalf("GitLabURL = %q, want %q", options.GitLabURL, "https://gitlab.example.com")
	}
	if !options.HasIgnoredOptions() {
		t.Fatal("HasIgnoredOptions() = false, want true")
	}
	ignored := options.IgnoredOptionsCopy()
	want := []string{"META_PARAM_SCHEMA", "RATE_LIMIT_RPS", RequestOptionGitLabURL}
	if !slicesEqual(ignored, want) {
		t.Fatalf("IgnoredOptions = %v, want %v", ignored, want)
	}
}

// TestResolveRequestOptions_ServerManagedHeadersIgnoredWithoutDefault verifies
// that config-like request headers never override MCP server configuration,
// even in multi-instance mode where GITLAB-URL is accepted.
func TestResolveRequestOptions_ServerManagedHeadersIgnoredWithoutDefault(t *testing.T) {
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", nil)
	req.Header.Set("GITLAB-URL", "https://gitlab.example.com")
	req.Header.Set("RATE_LIMIT_BURST", "999")
	req.Header.Set("META_PARAM_SCHEMA", "full")
	req.Header.Set("GITLAB-SAFE-MODE", "false")

	options, err := ResolveRequestOptions(req, "")
	if err != nil {
		t.Fatalf("ResolveRequestOptions() error: %v", err)
	}
	if options.GitLabURL != "https://gitlab.example.com" {
		t.Fatalf("GitLabURL = %q, want %q", options.GitLabURL, "https://gitlab.example.com")
	}
	want := []string{"META_PARAM_SCHEMA", "GITLAB_SAFE_MODE", "RATE_LIMIT_BURST"}
	if !slicesEqual(options.IgnoredOptionsCopy(), want) {
		t.Fatalf("IgnoredOptions = %v, want %v", options.IgnoredOptionsCopy(), want)
	}
}

// slicesEqual compares two string slices in order for ignored-option tests.
func slicesEqual(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

// TestAppendOptionName_DeduplicatesExisting verifies the internal option-name
// accumulator keeps the first occurrence when multiple aliases map to one
// server-managed option.
func TestAppendOptionName_DeduplicatesExisting(t *testing.T) {
	options := []string{"META_PARAM_SCHEMA"}
	got := appendOptionName(options, "META_PARAM_SCHEMA")
	if !slicesEqual(got, options) {
		t.Fatalf("appendOptionName() = %v, want %v", got, options)
	}
}

// TestInvalidGitLabURLError_DoesNotLeakURL verifies that [Error] never
// embeds the raw offending URL in its message — the URL may contain
// credentials in userinfo or sensitive query parameters that must not
// be copied verbatim into server logs (OWASP A09 logging hygiene).
func TestInvalidGitLabURLError_DoesNotLeakURL(t *testing.T) {
	t.Parallel()
	sensitive := "https://user:super-secret-password@gitlab.example.com/?token=abc123"
	err := &InvalidGitLabURLError{URL: sensitive, Reason: "scheme must be http or https"}
	msg := err.Error()
	if strings.Contains(msg, "super-secret-password") || strings.Contains(msg, "abc123") ||
		strings.Contains(msg, "user:") || strings.Contains(msg, "gitlab.example.com") {
		t.Errorf("Error() leaked URL contents: %q", msg)
	}
	if !strings.Contains(msg, "scheme must be http or https") {
		t.Errorf("Error() missing reason: %q", msg)
	}
}
