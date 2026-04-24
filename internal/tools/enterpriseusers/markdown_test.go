// markdown_test.go contains unit tests for enterprise user Markdown
// formatting functions.

package enterpriseusers

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestFormatOutputMarkdown validates the single-user Markdown formatter.
// Covers full output with all fields, zero-ID (empty), missing optional fields,
// and boolean flag rendering (admin, 2FA, external, locked, bot).
func TestFormatOutputMarkdown(t *testing.T) {
	tests := []struct {
		name      string
		input     Output
		wantEmpty bool
		contains  []string
		excludes  []string
	}{
		{
			name:      "zero ID returns empty string",
			input:     Output{},
			wantEmpty: true,
		},
		{
			name: "full output with all fields",
			input: Output{
				ID:               10,
				Username:         "alice",
				Name:             "Alice Wonderland",
				Email:            "alice@example.com",
				State:            "active",
				WebURL:           "https://gitlab.example.com/alice",
				IsAdmin:          true,
				Bot:              false,
				TwoFactorEnabled: true,
				External:         false,
				Locked:           false,
				CreatedAt:        "2026-01-01T00:00:00Z",
			},
			contains: []string{
				"## Enterprise User: Alice Wonderland",
				"10",
				"alice",
				"alice@example.com",
				"active",
				"**Admin**: true",
				"**2FA Enabled**: true",
				"**External**: false",
				"**Locked**: false",
				"**Bot**: false",
				"https://gitlab.example.com/alice",
				"2026-01-01T00:00:00Z",
				"gitlab_disable_2fa_enterprise_user",
				"gitlab_list_enterprise_users",
			},
		},
		{
			name: "no web URL and no created_at omits those lines",
			input: Output{
				ID:       5,
				Username: "bob",
				Name:     "Bob",
				Email:    "bob@example.com",
				State:    "blocked",
			},
			contains: []string{
				"## Enterprise User: Bob",
				"bob@example.com",
				"blocked",
			},
			excludes: []string{
				"URL",
				"Created",
			},
		},
		{
			name: "admin and locked flags render correctly",
			input: Output{
				ID:       99,
				Username: "admin_user",
				Name:     "Admin",
				Email:    "admin@example.com",
				State:    "active",
				IsAdmin:  true,
				Locked:   true,
				External: true,
				Bot:      true,
			},
			contains: []string{
				"**Admin**: true",
				"**Locked**: true",
				"**External**: true",
				"**Bot**: true",
			},
		},
		{
			name: "special characters in name are escaped",
			input: Output{
				ID:       7,
				Username: "pipe_user",
				Name:     "User | With Pipe",
				Email:    "pipe@example.com",
				State:    "active",
			},
			contains: []string{
				"Enterprise User:",
				"pipe_user",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatOutputMarkdown(tt.input)
			if tt.wantEmpty {
				if got != "" {
					t.Fatalf("expected empty string, got %q", got)
				}
				return
			}
			if got == "" {
				t.Fatal("expected non-empty markdown, got empty string")
			}
			for _, s := range tt.contains {
				if !strings.Contains(got, s) {
					t.Errorf("output missing %q:\n%s", s, got)
				}
			}
			for _, s := range tt.excludes {
				if strings.Contains(got, s) {
					t.Errorf("output should not contain %q:\n%s", s, got)
				}
			}
		})
	}
}

// TestFormatListMarkdown validates the list Markdown formatter.
// Covers empty list, single user, multiple users, and 2FA column rendering.
func TestFormatListMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    ListOutput
		contains []string
		excludes []string
	}{
		{
			name:  "empty list returns no-results message",
			input: ListOutput{Users: []Output{}},
			contains: []string{
				"No enterprise users found.",
			},
			excludes: []string{
				"| ID |",
			},
		},
		{
			name: "single user with 2FA enabled",
			input: ListOutput{
				Users: []Output{
					{
						ID:               1,
						Username:         "alice",
						Name:             "Alice",
						Email:            "alice@example.com",
						State:            "active",
						TwoFactorEnabled: true,
					},
				},
				Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 1},
			},
			contains: []string{
				"## Enterprise Users (1)",
				"| ID | Username | Name | Email | State | 2FA |",
				"| 1 | alice | Alice | alice@example.com | active | Yes |",
			},
		},
		{
			name: "multiple users with mixed 2FA status",
			input: ListOutput{
				Users: []Output{
					{
						ID:               1,
						Username:         "alice",
						Name:             "Alice",
						Email:            "alice@example.com",
						State:            "active",
						TwoFactorEnabled: true,
					},
					{
						ID:               2,
						Username:         "bob",
						Name:             "Bob",
						Email:            "bob@example.com",
						State:            "blocked",
						TwoFactorEnabled: false,
					},
				},
				Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 2},
			},
			contains: []string{
				"## Enterprise Users (2)",
				"| 1 | alice | Alice | alice@example.com | active | Yes |",
				"| 2 | bob | Bob | bob@example.com | blocked | No |",
			},
		},
		{
			name: "special characters in table cells are escaped",
			input: ListOutput{
				Users: []Output{
					{
						ID:       3,
						Username: "pipe_user",
						Name:     "User|Pipe",
						Email:    "pipe@example.com",
						State:    "active",
					},
				},
				Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 1},
			},
			contains: []string{
				"## Enterprise Users (1)",
				"pipe_user",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatListMarkdown(tt.input)
			if got == "" {
				t.Fatal("expected non-empty markdown, got empty string")
			}
			for _, s := range tt.contains {
				if !strings.Contains(got, s) {
					t.Errorf("output missing %q:\n%s", s, got)
				}
			}
			for _, s := range tt.excludes {
				if strings.Contains(got, s) {
					t.Errorf("output should not contain %q:\n%s", s, got)
				}
			}
		})
	}
}
