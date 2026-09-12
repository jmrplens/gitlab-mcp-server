// markdown_test.go contains unit tests for group SSH certificate Markdown
// formatting functions.
package groupsshcerts

import (
	"testing"
)

// The guidance section each SSH certificate formatter closes with.
const (
	certCardHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'group.ssh_cert_delete' to revoke this certificate\n" +
		"- Use action 'group.ssh_cert_list' to see the group's other certificates\n"
	certListHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'group.ssh_cert_create' to add another certificate\n"
	certTableHead = "| ID | Title | Created |\n| --- | --- | --- |\n"
)

// assertCertMarkdown compares a whole rendered response with what the
// formatter is meant to write, byte for byte.
func assertCertMarkdown(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("markdown mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestFormatOutputMarkdown validates the whole card one SSH CA certificate
// renders: a certificate GitLab sent nothing for renders nothing, a key longer
// than the card shows is truncated, and the creation time is in the display
// form rather than the wire form it arrives in.
func TestFormatOutputMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		input Output
		want  string
	}{
		{
			name:  "zero ID returns empty string",
			input: Output{},
			want:  "",
		},
		{
			name: "all fields with short key",
			input: Output{
				ID:        1,
				Title:     "deploy-key",
				Key:       "ssh-rsa AAAA1234",
				CreatedAt: "2026-01-15T10:30:00Z",
			},
			want: "## SSH Certificate #1\n\n" +
				"- **ID**: 1\n" +
				"- **Title**: deploy-key\n" +
				"- **Key**: `ssh-rsa AAAA1234`\n" +
				"- **Created**: 15 Jan 2026 10:30 UTC\n" +
				certCardHints,
		},
		{
			name: "long key gets truncated at 60 chars",
			input: Output{
				ID:    2,
				Title: "long-key-cert",
				Key:   "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC7QbpPnVFGkYLlWxyz1234567890abcdefghij",
			},
			want: "## SSH Certificate #2\n\n" +
				"- **ID**: 2\n" +
				"- **Title**: long-key-cert\n" +
				"- **Key**: `ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC7QbpPnVFGkYLlWxyz1...`\n" +
				certCardHints,
		},
		{
			name: "a key at the boundary is not truncated",
			input: Output{
				ID:    3,
				Title: "exact-key",
				Key:   "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC7QbpPnVFGkYLlWx",
			},
			want: "## SSH Certificate #3\n\n" +
				"- **ID**: 3\n" +
				"- **Title**: exact-key\n" +
				"- **Key**: `ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC7QbpPnVFGkYLlWx`\n" +
				certCardHints,
		},
		{
			name: "missing created_at omits the row",
			input: Output{
				ID:    4,
				Title: "no-date-cert",
				Key:   "ssh-ed25519 AAAA",
			},
			want: "## SSH Certificate #4\n\n" +
				"- **ID**: 4\n" +
				"- **Title**: no-date-cert\n" +
				"- **Key**: `ssh-ed25519 AAAA`\n" +
				certCardHints,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertCertMarkdown(t, FormatOutputMarkdown(tt.input), tt.want)
		})
	}
}

// TestFormatListMarkdown validates the whole table a group's SSH certificates
// render: an empty list is the one sentence alone, and a title carrying a pipe
// is neutralized rather than splitting its row.
func TestFormatListMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		input ListOutput
		want  string
	}{
		{
			name:  "empty list returns no certificates message",
			input: ListOutput{Certificates: []Output{}},
			want:  "No SSH certificates found.\n",
		},
		{
			name:  "nil certificates returns no certificates message",
			input: ListOutput{},
			want:  "No SSH certificates found.\n",
		},
		{
			name: "single certificate renders table",
			input: ListOutput{
				Certificates: []Output{
					{ID: 1, Title: "cert-one", CreatedAt: "2026-03-01T00:00:00Z"},
				},
			},
			want: "## SSH Certificates (1)\n\n" +
				certTableHead +
				"| 1 | cert-one | 1 Mar 2026 00:00 UTC |\n" +
				certListHints,
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
			want: "## SSH Certificates (3)\n\n" +
				certTableHead +
				"| 10 | deploy-key | 1 Jan 2026 00:00 UTC |\n" +
				"| 20 | ci-bot | 15 Jun 2026 12:00 UTC |\n" +
				"| 30 | backup-key |  |\n" +
				certListHints,
		},
		{
			name: "title with pipe character is escaped",
			input: ListOutput{
				Certificates: []Output{
					{ID: 5, Title: "key|with|pipes", CreatedAt: "2026-01-01T00:00:00Z"},
				},
			},
			want: "## SSH Certificates (1)\n\n" +
				certTableHead +
				"| 5 | key&#124;with&#124;pipes | 1 Jan 2026 00:00 UTC |\n" +
				certListHints,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertCertMarkdown(t, FormatListMarkdown(tt.input), tt.want)
		})
	}
}
