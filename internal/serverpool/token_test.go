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
			name:       "valid HTTPS URL",
			header:     "https://gitlab.example.com",
			defaultURL: "https://gitlab.com",
			wantURL:    "https://gitlab.example.com",
		},
		{
			name:       "valid HTTP URL",
			header:     "http://gitlab.local:8080",
			defaultURL: "https://gitlab.com",
			wantURL:    "http://gitlab.local:8080",
		},
		{
			name:       "trailing slash stripped",
			header:     "https://gitlab.example.com/",
			defaultURL: "https://gitlab.com",
			wantURL:    "https://gitlab.example.com",
		},
		{
			name:       "whitespace trimmed",
			header:     "  https://gitlab.example.com  ",
			defaultURL: "https://gitlab.com",
			wantURL:    "https://gitlab.example.com",
		},
		{
			name:       "invalid scheme rejected",
			header:     "ftp://gitlab.example.com",
			defaultURL: "https://gitlab.com",
			wantErr:    true,
		},
		{
			name:       "missing host rejected",
			header:     "https://",
			defaultURL: "https://gitlab.com",
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
			name:       "uppercase scheme accepted and case preserved",
			header:     "HTTPS://gitlab.example.com",
			defaultURL: "https://gitlab.com",
			wantURL:    "HTTPS://gitlab.example.com",
		},
		{
			name:       "malformed URL rejected",
			header:     "://not-a-url",
			defaultURL: "https://gitlab.com",
			wantErr:    true,
		},
		{
			name:       "URL with path preserved (only trailing slash stripped)",
			header:     "https://gitlab.example.com/api",
			defaultURL: "https://gitlab.com",
			wantURL:    "https://gitlab.example.com/api",
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
