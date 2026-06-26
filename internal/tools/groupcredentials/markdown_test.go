// markdown_test.go contains unit tests for group credential Markdown
// formatting functions.
package groupcredentials

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestFormatPATMarkdown verifies the PATMarkdown Markdown formatter for a representative pat input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatPATMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    PATOutput
		contains []string
		excludes []string
	}{
		{
			name: "all fields present",
			input: PATOutput{
				ID:         1,
				Name:       "deploy-token",
				Revoked:    false,
				CreatedAt:  "2026-01-01T00:00:00Z",
				Scopes:     []string{"api", "read_user"},
				UserID:     10,
				LastUsedAt: "2026-06-15T10:30:00Z",
				Active:     true,
				ExpiresAt:  "2026-01-01",
			},
			contains: []string{
				"deploy-token", "ID: 1",
				"10", "true",
				"api, read_user",
				"2026-01-01",
				"2026-01-01T00:00:00Z",
				"2026-06-15T10:30:00Z",
			},
		},
		{
			name: "no optional fields",
			input: PATOutput{
				ID:        2,
				Name:      "basic-token",
				CreatedAt: "2026-01-01T00:00:00Z",
				UserID:    20,
			},
			contains: []string{"basic-token", "ID: 2", "Active", "Revoked"},
			excludes: []string{"Expires At", "Last Used At"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatPATMarkdown(tt.input)
			for _, s := range tt.contains {
				if !strings.Contains(got, s) {
					t.Errorf("expected output to contain %q, got:\n%s", s, got)
				}
			}
			for _, s := range tt.excludes {
				if strings.Contains(got, s) {
					t.Errorf("expected output NOT to contain %q, got:\n%s", s, got)
				}
			}
		})
	}
}

// TestFormatPATListMarkdown verifies the PATListMarkdown Markdown formatter for a representative patlist input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatPATListMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    PATListOutput
		contains []string
	}{
		{
			name:     "empty list",
			input:    PATListOutput{},
			contains: []string{"No personal access tokens found"},
		},
		{
			name: "with tokens",
			input: PATListOutput{
				Tokens: []PATOutput{
					{ID: 1, Name: "tok1", UserID: 10, Active: true, Scopes: []string{"api"}, ExpiresAt: "2026-01-01"},
					{ID: 2, Name: "tok2", UserID: 20, Revoked: true},
				},
				Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 2},
			},
			contains: []string{
				"Personal Access Tokens (2)",
				"tok1", "tok2",
				"Active", "Revoked",
				"api",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatPATListMarkdown(tt.input)
			for _, s := range tt.contains {
				if !strings.Contains(got, s) {
					t.Errorf("expected output to contain %q, got:\n%s", s, got)
				}
			}
		})
	}
}

// TestFormatSSHKeyMarkdown verifies single SSH key markdown rendering, including
// the optional usage-type, expiry, and last-used fields.
func TestFormatSSHKeyMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    SSHKeyOutput
		contains []string
		excludes []string
	}{
		{
			name: "all optional fields present",
			input: SSHKeyOutput{
				ID:         5,
				Title:      "my-key",
				UsageType:  "auth",
				CreatedAt:  "2026-01-01T00:00:00Z",
				ExpiresAt:  "2026-06-01T00:00:00Z",
				LastUsedAt: "2026-05-01T00:00:00Z",
				UserID:     10,
			},
			contains: []string{"my-key", "ID: 5", "auth", "Expires At", "Last Used At"},
		},
		{
			name: "no optional fields",
			input: SSHKeyOutput{
				ID:        6,
				Title:     "basic-key",
				CreatedAt: "2026-01-01T00:00:00Z",
				UserID:    11,
			},
			contains: []string{"basic-key", "ID: 6", "Created At"},
			excludes: []string{"Expires At", "Last Used At", "Usage Type"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatSSHKeyMarkdown(tt.input)
			for _, s := range tt.contains {
				if !strings.Contains(got, s) {
					t.Errorf("expected output to contain %q, got:\n%s", s, got)
				}
			}
			for _, s := range tt.excludes {
				if strings.Contains(got, s) {
					t.Errorf("expected output NOT to contain %q, got:\n%s", s, got)
				}
			}
		})
	}
}

// TestFormatSSHKeyListMarkdown verifies the SSHKeyListMarkdown Markdown formatter for a representative sshkeylist input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatSSHKeyListMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    SSHKeyListOutput
		contains []string
	}{
		{
			name:     "empty list",
			input:    SSHKeyListOutput{},
			contains: []string{"No SSH keys found"},
		},
		{
			name: "with keys",
			input: SSHKeyListOutput{
				Keys: []SSHKeyOutput{
					{ID: 5, Title: "key-1", UserID: 10, CreatedAt: "2026-01-01T00:00:00Z", ExpiresAt: "2026-06-01T00:00:00Z"},
					{ID: 6, Title: "key-2", UserID: 20, CreatedAt: "2026-02-01T00:00:00Z"},
				},
				Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 2},
			},
			contains: []string{
				"SSH Keys (2)",
				"key-1", "key-2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatSSHKeyListMarkdown(tt.input)
			for _, s := range tt.contains {
				if !strings.Contains(got, s) {
					t.Errorf("expected output to contain %q, got:\n%s", s, got)
				}
			}
		})
	}
}
