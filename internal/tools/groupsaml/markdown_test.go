// markdown_test.go contains unit tests for group SAML Markdown formatting
// functions. Each case compares the whole document the formatter renders.
package groupsaml

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// samlLinkHints is the guidance section every SAML link card ends with.
const samlLinkHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'group.saml_link_delete' to remove this link\n"

// samlListHints is the guidance section a list of SAML links ends with.
const samlListHints = "\n---\n💡 **Next steps:**\n" +
	"- These map SAML group names to access levels\n" +
	"- Use action 'group.saml_users_list' to list the users provisioned through SAML SSO\n"

// TestFormatOutputMarkdown validates the whole card a single SAML link
// renders: the access level named as well as numbered, no row for a field
// GitLab did not send, and the guidance last.
func TestFormatOutputMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		input Output
		want  string
	}{
		{
			name: "all fields populated",
			input: Output{
				Name:         "saml-admins",
				AccessLevel:  40,
				MemberRoleID: 99,
				Provider:     "okta",
			},
			want: "## SAML Link: saml-admins\n\n" +
				"- **Name**: saml-admins\n" +
				"- **Access Level**: Maintainer (40)\n" +
				"- **Member Role ID**: 99\n" +
				"- **Provider**: okta\n" +
				samlLinkHints,
		},
		{
			name: "minimal fields omits member role and provider",
			input: Output{
				Name:        "saml-devs",
				AccessLevel: 30,
			},
			want: "## SAML Link: saml-devs\n\n" +
				"- **Name**: saml-devs\n" +
				"- **Access Level**: Developer (30)\n" +
				samlLinkHints,
		},
		{
			name: "provider set but member role zero",
			input: Output{
				Name:        "saml-guest",
				AccessLevel: 10,
				Provider:    "azure-ad",
			},
			want: "## SAML Link: saml-guest\n\n" +
				"- **Name**: saml-guest\n" +
				"- **Access Level**: Guest (10)\n" +
				"- **Provider**: azure-ad\n" +
				samlLinkHints,
		},
		{
			name: "member role set but provider empty",
			input: Output{
				Name:         "saml-maint",
				AccessLevel:  40,
				MemberRoleID: 7,
			},
			want: "## SAML Link: saml-maint\n\n" +
				"- **Name**: saml-maint\n" +
				"- **Access Level**: Maintainer (40)\n" +
				"- **Member Role ID**: 7\n" +
				samlLinkHints,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatOutputMarkdown(tt.input); got != tt.want {
				t.Errorf("FormatOutputMarkdown =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

// TestFormatListMarkdown validates the whole document a list of SAML links
// renders: the heading a list opens with instead of the bold paragraph that
// used to sit between the hints and the table, and the table itself.
func TestFormatListMarkdown(t *testing.T) {
	const header = "| Name | Access Level | Provider |\n| --- | --- | --- |\n"

	tests := []struct {
		name  string
		input ListOutput
		want  string
	}{
		{
			name:  "empty list returns no-results message",
			input: ListOutput{Links: nil},
			want:  "No SAML group links found.\n",
		},
		{
			name:  "empty links slice returns no-results message",
			input: ListOutput{Links: []Output{}},
			want:  "No SAML group links found.\n",
		},
		{
			name: "single link renders table",
			input: ListOutput{
				Links: []Output{
					{Name: "saml-devs", AccessLevel: 30, Provider: "okta"},
				},
			},
			want: "## SAML Group Links (1)\n\n" + header +
				"| saml-devs | Developer (30) | okta |\n" +
				samlListHints,
		},
		{
			name: "multiple links render all rows",
			input: ListOutput{
				Links: []Output{
					{Name: "saml-devs", AccessLevel: 30, Provider: ""},
					{Name: "saml-admins", AccessLevel: 50, Provider: "azure-ad"},
				},
			},
			want: "## SAML Group Links (2)\n\n" + header +
				"| saml-devs | Developer (30) |  |\n" +
				"| saml-admins | Owner (50) | azure-ad |\n" +
				samlListHints,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatListMarkdown(tt.input); got != tt.want {
				t.Errorf("FormatListMarkdown =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

// TestFormatSAMLUsersListMarkdown validates the whole document a page of
// SAML-provisioned users renders: the heading counting the total GitLab
// reported rather than the page length, and the guidance last.
func TestFormatSAMLUsersListMarkdown(t *testing.T) {
	t.Run("one page of a larger set", func(t *testing.T) {
		got := FormatSAMLUsersListMarkdown(SAMLUsersListOutput{
			Users: []SAMLUserOutput{
				{ID: 1, Username: "alice", Name: "Alice", State: "active", WebURL: "https://gl/alice"},
			},
			Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 3, TotalItems: 45, PerPage: 20},
		})
		want := "## SAML Users (45)\n\n" +
			"Showing 1 of 45 results (page 1 of 3)\n\n" +
			"| ID | Username | Name | State |\n| --- | --- | --- | --- |\n" +
			"| 1 | [@alice](https://gl/alice) | Alice | active |\n" +
			"\nPage 1 of 3 | 45 items total | 20 per page\n" +
			"\n---\n💡 **Next steps:**\n" +
			"- " + toolutil.HintPreserveLinks + "\n" +
			"- These are users provisioned through SAML SSO\n" +
			"- Use action 'group.saml_link_list' to see the SAML group-to-access-level link mappings\n"
		if got != want {
			t.Errorf("FormatSAMLUsersListMarkdown =\n%q\nwant\n%q", got, want)
		}
	})
}
