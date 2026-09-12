// markdown_test.go contains unit tests for group credential Markdown
// formatting functions.
package groupcredentials

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The guidance section each group credential formatter closes with.
const (
	patCardHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'group.credential_list_pats' to see the group's other tokens\n" +
		"- Use action 'group.credential_revoke_pat' to revoke this token\n"
	patListHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'group.credential_revoke_pat' to revoke one of these tokens\n"
	sshKeyCardHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'group.credential_list_ssh_keys' to see the group's other keys\n" +
		"- Use action 'group.credential_delete_ssh_key' to delete this key\n"
	sshKeyListHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'group.credential_delete_ssh_key' to delete one of these keys\n"
)

// assertCredentialMarkdown compares a whole rendered response with what the
// formatter is meant to write, byte for byte. A substring assertion is what
// let these two listings write their hints between the heading and the table
// header — which left the header lazily continuing the guidance list, so no
// table rendered at all — and still pass.
func assertCredentialMarkdown(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("markdown mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestFormatPATMarkdown verifies the whole card one enterprise personal access
// token renders: every timestamp in the display form, the active flag as a
// glyph, and no row for a field GitLab did not send.
func TestFormatPATMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		input PATOutput
		want  string
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
			want: "## Personal Access Token: deploy-token (ID: 1)\n\n" +
				"- **ID**: 1\n" +
				"- **User ID**: 10\n" +
				"- **Active**: ✅\n" +
				"- **Scopes**: api, read_user\n" +
				"- **Expires At**: 1 Jan 2026\n" +
				"- **Created**: 1 Jan 2026 00:00 UTC\n" +
				"- **Last Used**: 15 Jun 2026 10:30 UTC\n" +
				patCardHints,
		},
		{
			name: "a revoked token is marked with a warning, not a tick",
			input: PATOutput{
				ID:        2,
				Name:      "basic-token",
				CreatedAt: "2026-01-01T00:00:00Z",
				UserID:    20,
				Revoked:   true,
			},
			want: "## Personal Access Token: basic-token (ID: 2)\n\n" +
				"- **ID**: 2\n" +
				"- **User ID**: 20\n" +
				"- **Active**: ❌\n" +
				"- ⚠️ **Revoked**\n" +
				"- **Created**: 1 Jan 2026 00:00 UTC\n" +
				patCardHints,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertCredentialMarkdown(t, FormatPATMarkdown(tt.input), tt.want)
		})
	}
}

// TestFormatPATListMarkdown verifies the whole table a page of tokens renders:
// the heading counting the total GitLab sent rather than the page length, the
// table header opening a block of its own, and the hints last.
func TestFormatPATListMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		input PATListOutput
		want  string
	}{
		{
			name:  "empty list renders the one sentence",
			input: PATListOutput{},
			want:  "No personal access tokens found.\n",
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
			want: "## Personal Access Tokens (2)\n\n" +
				"| ID | Name | User ID | Active | Revoked | Scopes | Expires At |\n" +
				"| --- | --- | --- | --- | --- | --- | --- |\n" +
				"| 1 | tok1 | 10 | ✅ | ❌ | api | 1 Jan 2026 |\n" +
				"| 2 | tok2 | 20 | ❌ | ✅ |  |  |\n" +
				"\nPage 1 of 1 | 2 items total\n" +
				patListHints,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertCredentialMarkdown(t, FormatPATListMarkdown(tt.input), tt.want)
		})
	}
}

// TestFormatSSHKeyMarkdown verifies the whole card one enterprise SSH key
// renders, and that a key GitLab sent no optional field for writes no row for
// one: a labeled row with an empty value is what the card replaced.
func TestFormatSSHKeyMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		input SSHKeyOutput
		want  string
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
			want: "## SSH Key: my-key (ID: 5)\n\n" +
				"- **ID**: 5\n" +
				"- **User ID**: 10\n" +
				"- **Usage Type**: auth\n" +
				"- **Created**: 1 Jan 2026 00:00 UTC\n" +
				"- **Expires At**: 1 Jun 2026 00:00 UTC\n" +
				"- **Last Used**: 1 May 2026 00:00 UTC\n" +
				sshKeyCardHints,
		},
		{
			name: "no optional fields",
			input: SSHKeyOutput{
				ID:        6,
				Title:     "basic-key",
				CreatedAt: "2026-01-01T00:00:00Z",
				UserID:    11,
			},
			want: "## SSH Key: basic-key (ID: 6)\n\n" +
				"- **ID**: 6\n" +
				"- **User ID**: 11\n" +
				"- **Created**: 1 Jan 2026 00:00 UTC\n" +
				sshKeyCardHints,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertCredentialMarkdown(t, FormatSSHKeyMarkdown(tt.input), tt.want)
		})
	}
}

// TestFormatSSHKeyListMarkdown verifies the whole table a page of SSH keys
// renders.
func TestFormatSSHKeyListMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		input SSHKeyListOutput
		want  string
	}{
		{
			name:  "empty list renders the one sentence",
			input: SSHKeyListOutput{},
			want:  "No SSH keys found.\n",
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
			want: "## SSH Keys (2)\n\n" +
				"| ID | Title | User ID | Created | Expires At |\n" +
				"| --- | --- | --- | --- | --- |\n" +
				"| 5 | key-1 | 10 | 1 Jan 2026 00:00 UTC | 1 Jun 2026 00:00 UTC |\n" +
				"| 6 | key-2 | 20 | 1 Feb 2026 00:00 UTC |  |\n" +
				"\nPage 1 of 1 | 2 items total\n" +
				sshKeyListHints,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertCredentialMarkdown(t, FormatSSHKeyListMarkdown(tt.input), tt.want)
		})
	}
}
