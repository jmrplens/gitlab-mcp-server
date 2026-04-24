// markdown_test.go contains unit tests for group credential Markdown
// formatting functions.

package groupcredentials

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestFormatPATMarkdown verifies single PAT markdown rendering covers
// all output fields including optional scopes, expires_at, and last_used_at.
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
				State:      "active",
			},
			contains: []string{
				"deploy-token", "ID: 1",
				"10", "active",
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
				State:     "inactive",
			},
			contains: []string{"basic-token", "ID: 2", "inactive"},
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

// TestFormatPATListMarkdown verifies list rendering for empty token lists
// and populated lists with pagination metadata.
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
					{ID: 1, Name: "tok1", UserID: 10, State: "active", Scopes: []string{"api"}, ExpiresAt: "2026-01-01"},
					{ID: 2, Name: "tok2", UserID: 20, State: "revoked", Revoked: true},
				},
				Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 2},
			},
			contains: []string{
				"Personal Access Tokens (2)",
				"tok1", "tok2",
				"active", "revoked",
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

// TestFormatSSHKeyMarkdown verifies single SSH key markdown rendering
// with both short keys (displayed in full) and long keys (truncated to 57 chars).
func TestFormatSSHKeyMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    SSHKeyOutput
		contains []string
		excludes []string
	}{
		{
			name: "short key with expires_at",
			input: SSHKeyOutput{
				ID:        5,
				Title:     "my-key",
				Key:       "ssh-rsa AAAA",
				CreatedAt: "2026-01-01T00:00:00Z",
				ExpiresAt: "2026-06-01T00:00:00Z",
				UserID:    10,
			},
			contains: []string{"my-key", "ID: 5", "ssh-rsa AAAA", "Expires At"},
		},
		{
			name: "long key truncated",
			input: SSHKeyOutput{
				ID:        6,
				Title:     "long-key",
				Key:       "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC7n+ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789abcdef",
				CreatedAt: "2026-01-01T00:00:00Z",
				UserID:    11,
			},
			contains: []string{"long-key", "..."},
			excludes: []string{"Expires At"},
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

// TestFormatSSHKeyListMarkdown verifies list rendering for empty SSH key lists
// and populated lists with pagination metadata.
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
