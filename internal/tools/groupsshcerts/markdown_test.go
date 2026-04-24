// markdown_test.go contains unit tests for group SSH certificate Markdown
// formatting functions.
package groupsshcerts

import (
	"strings"
	"testing"
)

// TestFormatOutputMarkdown validates the Markdown formatter for a single SSH certificate.
// Covers: zero ID (empty string), short key, long key truncation, optional CreatedAt,
// and hints footer.
func TestFormatOutputMarkdown(t *testing.T) {
	tests := []struct {
		name      string
		input     Output
		wantParts []string
		dontWant  []string
		wantEmpty bool
	}{
		{
			name:      "zero ID returns empty string",
			input:     Output{},
			wantEmpty: true,
		},
		{
			name: "all fields with short key",
			input: Output{
				ID:        1,
				Title:     "deploy-key",
				Key:       "ssh-rsa AAAA1234",
				CreatedAt: "2026-01-15T10:30:00Z",
			},
			wantParts: []string{
				"## SSH Certificate #1",
				"**Title**: deploy-key",
				"**Key**: `ssh-rsa AAAA1234`",
				"**Created**: 2026-01-15T10:30:00Z",
				"gitlab_delete_group_ssh_certificate",
				"gitlab_list_group_ssh_certificates",
			},
		},
		{
			name: "long key gets truncated at 60 chars",
			input: Output{
				ID:    2,
				Title: "long-key-cert",
				Key:   "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC7QbpPnVFGkYLlWxyz1234567890abcdefghij",
			},
			wantParts: []string{
				"## SSH Certificate #2",
				"**Title**: long-key-cert",
				"...",
			},
			dontWant: []string{
				"**Created**",
			},
		},
		{
			name: "exactly 60 char key is not truncated",
			input: Output{
				ID:    3,
				Title: "exact-key",
				Key:   "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC7QbpPnVFGkYLlWx",
			},
			wantParts: []string{
				"`ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC7QbpPnVFGkYLlWx`",
			},
			dontWant: []string{
				"...",
			},
		},
		{
			name: "missing created_at omits line",
			input: Output{
				ID:    4,
				Title: "no-date-cert",
				Key:   "ssh-ed25519 AAAA",
			},
			wantParts: []string{
				"## SSH Certificate #4",
				"**Title**: no-date-cert",
				"**Key**: `ssh-ed25519 AAAA`",
			},
			dontWant: []string{
				"**Created**",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatOutputMarkdown(tt.input)
			if tt.wantEmpty {
				if got != "" {
					t.Errorf("expected empty string, got:\n%s", got)
				}
				return
			}
			if got == "" {
				t.Fatal("expected non-empty markdown output, got empty string")
			}
			for _, part := range tt.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("output missing %q\ngot:\n%s", part, got)
				}
			}
			for _, part := range tt.dontWant {
				if strings.Contains(got, part) {
					t.Errorf("output should not contain %q\ngot:\n%s", part, got)
				}
			}
		})
	}
}

// TestFormatListMarkdown validates the Markdown formatter for a list of SSH certificates.
// Covers: empty list (no certificates message), single certificate, multiple certificates
// with correct table structure including ID, Title, and Created columns.
func TestFormatListMarkdown(t *testing.T) {
	tests := []struct {
		name      string
		input     ListOutput
		wantParts []string
		dontWant  []string
	}{
		{
			name:  "empty list returns no certificates message",
			input: ListOutput{Certificates: []Output{}},
			wantParts: []string{
				"No SSH certificates found.",
			},
			dontWant: []string{
				"| ID",
			},
		},
		{
			name:  "nil certificates returns no certificates message",
			input: ListOutput{},
			wantParts: []string{
				"No SSH certificates found.",
			},
		},
		{
			name: "single certificate renders table",
			input: ListOutput{
				Certificates: []Output{
					{ID: 1, Title: "cert-one", CreatedAt: "2026-03-01T00:00:00Z"},
				},
			},
			wantParts: []string{
				"## SSH Certificates (1)",
				"| ID | Title | Created |",
				"| 1 | cert-one | 2026-03-01T00:00:00Z |",
				"gitlab_create_group_ssh_certificate",
			},
		},
		{
			name: "multiple certificates renders all rows",
			input: ListOutput{
				Certificates: []Output{
					{ID: 10, Title: "deploy-key", CreatedAt: "2026-01-01T00:00:00Z"},
					{ID: 20, Title: "ci-bot", CreatedAt: "2026-06-15T12:00:00Z"},
					{ID: 30, Title: "backup-key", CreatedAt: ""},
				},
			},
			wantParts: []string{
				"## SSH Certificates (3)",
				"| 10 | deploy-key |",
				"| 20 | ci-bot |",
				"| 30 | backup-key |",
			},
		},
		{
			name: "title with pipe character is escaped",
			input: ListOutput{
				Certificates: []Output{
					{ID: 5, Title: "key|with|pipes", CreatedAt: "2026-01-01T00:00:00Z"},
				},
			},
			wantParts: []string{
				"## SSH Certificates (1)",
			},
			dontWant: []string{
				"| key|with|pipes |",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatListMarkdown(tt.input)
			if got == "" {
				t.Fatal("expected non-empty markdown output, got empty string")
			}
			for _, part := range tt.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("output missing %q\ngot:\n%s", part, got)
				}
			}
			for _, part := range tt.dontWant {
				if strings.Contains(got, part) {
					t.Errorf("output should not contain %q\ngot:\n%s", part, got)
				}
			}
		})
	}
}
