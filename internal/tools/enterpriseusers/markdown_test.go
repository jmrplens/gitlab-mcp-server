// markdown_test.go contains unit tests for enterprise user Markdown
// formatting functions.
package enterpriseusers

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// assertMarkdown compares a rendered result with the whole document it is
// meant to be. A substring assertion is what let a card open a table and then
// write list rows into it in two packages of this tree: every row the test
// named was present in the string and none of them rendered as a row, so the
// rule here is the whole document or nothing.
func assertMarkdown(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("markdown mismatch\n--- got ---\n%s\n--- want ---\n%s\n--- got (quoted) ---\n%q", got, want, got)
	}
}

// TestFormatOutputMarkdown validates the single-user card: the identity rows,
// the flags as the tick or cross BoolEmoji gives them, the lock as the warning
// it is, the address as a link, the creation instant in the display form, and
// the two next steps naming canonical action IDs. A response with no ID is not
// a user and renders nothing.
func TestFormatOutputMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		input Output
		want  string
	}{
		{
			name:  "zero ID renders nothing",
			input: Output{},
			want:  "",
		},
		{
			name: "every field renders",
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
			want: "## Enterprise User: Alice Wonderland\n\n" +
				"- **ID**: 10\n" +
				"- **Username**: alice\n" +
				"- **Email**: alice@example.com\n" +
				"- **State**: active\n" +
				"- **Admin**: ✅\n" +
				"- **2FA Enabled**: ✅\n" +
				"- **External**: ❌\n" +
				"- **Bot**: ❌\n" +
				"- **URL**: [https://gitlab.example.com/alice](https://gitlab.example.com/alice)\n" +
				"- **Created**: 1 Jan 2026 00:00 UTC\n" +
				"\n---\n💡 **Next steps:**\n" +
				"- Use action 'enterprise_user.disable_2fa' to reset two-factor authentication\n" +
				"- Use action 'enterprise_user.list' to browse all enterprise users\n",
		},
		{
			name: "an absent address and an absent instant write no row",
			input: Output{
				ID:       5,
				Username: "bob",
				Name:     "Bob",
				Email:    "bob@example.com",
				State:    "blocked",
			},
			want: "## Enterprise User: Bob\n\n" +
				"- **ID**: 5\n" +
				"- **Username**: bob\n" +
				"- **Email**: bob@example.com\n" +
				"- **State**: blocked\n" +
				"- **Admin**: ❌\n" +
				"- **2FA Enabled**: ❌\n" +
				"- **External**: ❌\n" +
				"- **Bot**: ❌\n" +
				"\n---\n💡 **Next steps:**\n" +
				"- Use action 'enterprise_user.disable_2fa' to reset two-factor authentication\n" +
				"- Use action 'enterprise_user.list' to browse all enterprise users\n",
		},
		{
			name: "a locked account carries the warning and never a tick",
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
			want: "## Enterprise User: Admin\n\n" +
				"- **ID**: 99\n" +
				"- **Username**: admin_user\n" +
				"- **Email**: admin@example.com\n" +
				"- **State**: active\n" +
				"- **Admin**: ✅\n" +
				"- **2FA Enabled**: ❌\n" +
				"- **External**: ✅\n" +
				"- **Bot**: ✅\n" +
				"- ⚠️ **Locked**\n" +
				"\n---\n💡 **Next steps:**\n" +
				"- Use action 'enterprise_user.disable_2fa' to reset two-factor authentication\n" +
				"- Use action 'enterprise_user.list' to browse all enterprise users\n",
		},
		{
			name: "a pipe in a value is neutralized in the row it lands in",
			input: Output{
				ID:       7,
				Username: "pipe_user",
				Name:     "User | With Pipe",
				Email:    "pipe@example.com",
				State:    "active",
			},
			want: "## Enterprise User: User | With Pipe\n\n" +
				"- **ID**: 7\n" +
				"- **Username**: pipe_user\n" +
				"- **Email**: pipe@example.com\n" +
				"- **State**: active\n" +
				"- **Admin**: ❌\n" +
				"- **2FA Enabled**: ❌\n" +
				"- **External**: ❌\n" +
				"- **Bot**: ❌\n" +
				"\n---\n💡 **Next steps:**\n" +
				"- Use action 'enterprise_user.disable_2fa' to reset two-factor authentication\n" +
				"- Use action 'enterprise_user.list' to browse all enterprise users\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertMarkdown(t, FormatOutputMarkdown(tt.input), tt.want)
		})
	}
}

// TestFormatListMarkdown verifies the enterprise-user list: the empty answer
// is the one sentence and nothing else, the heading counts what GitLab
// reported rather than what the page holds, the handle links to the profile
// the preserve-links hint is about, and the 2FA column is a flag rather than
// the words Yes and No.
func TestFormatListMarkdown(t *testing.T) {
	const listHints = "\n---\n💡 **Next steps:**\n" +
		"- When presenting these results, always include the clickable [text](url) links from the table so the user can navigate to GitLab\n" +
		"- Use action 'enterprise_user.get' to read one enterprise user in full\n"

	tests := []struct {
		name  string
		input ListOutput
		want  string
	}{
		{
			name:  "an empty list is the sentence and nothing else",
			input: ListOutput{Users: []Output{}},
			want:  "No enterprise users found.\n",
		},
		{
			name: "one user, the handle linked to the profile",
			input: ListOutput{
				Users: []Output{
					{
						ID:               1,
						Username:         "alice",
						Name:             "Alice",
						Email:            "alice@example.com",
						State:            "active",
						WebURL:           "https://gitlab.example.com/alice",
						TwoFactorEnabled: true,
					},
				},
				Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 1},
			},
			want: "## Enterprise Users (1)\n\n" +
				"| ID | Username | Name | Email | State | 2FA |\n" +
				"| --- | --- | --- | --- | --- | --- |\n" +
				"| 1 | [@alice](https://gitlab.example.com/alice) | Alice | alice@example.com | active | ✅ |\n" +
				"\nPage 1 of 1 | 1 items total\n" +
				listHints,
		},
		{
			name: "the heading counts the total GitLab reported, not the page",
			input: ListOutput{
				Users: []Output{
					{
						ID:               1,
						Username:         "alice",
						Name:             "Alice",
						Email:            "alice@example.com",
						State:            "active",
						WebURL:           "https://gitlab.example.com/alice",
						TwoFactorEnabled: true,
					},
					{
						ID:               2,
						Username:         "bob",
						Name:             "Bob",
						Email:            "bob@example.com",
						State:            "blocked",
						WebURL:           "https://gitlab.example.com/bob",
						TwoFactorEnabled: false,
					},
				},
				Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 3, TotalItems: 45, PerPage: 2},
			},
			want: "## Enterprise Users (45)\n\n" +
				"Showing 2 of 45 results (page 1 of 3)\n\n" +
				"| ID | Username | Name | Email | State | 2FA |\n" +
				"| --- | --- | --- | --- | --- | --- |\n" +
				"| 1 | [@alice](https://gitlab.example.com/alice) | Alice | alice@example.com | active | ✅ |\n" +
				"| 2 | [@bob](https://gitlab.example.com/bob) | Bob | bob@example.com | blocked | ❌ |\n" +
				"\nPage 1 of 3 | 45 items total | 2 per page\n" +
				listHints,
		},
		{
			name: "a pipe in a cell is an entity and never ends the row",
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
			want: "## Enterprise Users (1)\n\n" +
				"| ID | Username | Name | Email | State | 2FA |\n" +
				"| --- | --- | --- | --- | --- | --- |\n" +
				"| 3 | @pipe_user | User&#124;Pipe | pipe@example.com | active | ❌ |\n" +
				"\nPage 1 of 1 | 1 items total\n" +
				listHints,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertMarkdown(t, FormatListMarkdown(tt.input), tt.want)
		})
	}
}
