// token_test.go contains unit tests for token extraction and validation.
package serverpool

import (
	"net/http"
	"net/http/httptest"
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
