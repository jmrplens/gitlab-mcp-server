// markdown_test.go contains unit tests for group SAML Markdown formatting functions.
package groupsaml

import (
	"strings"
	"testing"
)

// TestFormatOutputMarkdown validates the Markdown formatter for a single SAML link.
// Covers: heading, name, access level, conditional member role ID, conditional provider,
// and the hints footer.
func TestFormatOutputMarkdown(t *testing.T) {
	tests := []struct {
		name      string
		input     Output
		wantParts []string
		dontWant  []string
	}{
		{
			name: "all fields populated",
			input: Output{
				Name:         "saml-admins",
				AccessLevel:  40,
				MemberRoleID: 99,
				Provider:     "okta",
			},
			wantParts: []string{
				"## SAML Link: saml-admins",
				"saml-admins",
				"**Access Level**: 40",
				"**Member Role ID**: 99",
				"**Provider**: okta",
				"gitlab_group_saml_link_delete",
			},
		},
		{
			name: "minimal fields omits member role and provider",
			input: Output{
				Name:        "saml-devs",
				AccessLevel: 30,
			},
			wantParts: []string{
				"## SAML Link: saml-devs",
				"**Access Level**: 30",
			},
			dontWant: []string{
				"Member Role ID",
				"Provider",
			},
		},
		{
			name: "provider set but member role zero",
			input: Output{
				Name:        "saml-guest",
				AccessLevel: 10,
				Provider:    "azure-ad",
			},
			wantParts: []string{
				"**Provider**: azure-ad",
				"**Access Level**: 10",
			},
			dontWant: []string{
				"Member Role ID",
			},
		},
		{
			name: "member role set but provider empty",
			input: Output{
				Name:         "saml-maint",
				AccessLevel:  40,
				MemberRoleID: 7,
			},
			wantParts: []string{
				"**Member Role ID**: 7",
			},
			dontWant: []string{
				"**Provider**",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatOutputMarkdown(tt.input)
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

// TestFormatListMarkdown validates the Markdown formatter for a list of SAML links.
// Covers: empty list message, single item table, multi-item table with provider column.
func TestFormatListMarkdown(t *testing.T) {
	tests := []struct {
		name      string
		input     ListOutput
		wantParts []string
		dontWant  []string
	}{
		{
			name:  "empty list returns no-results message",
			input: ListOutput{Links: nil},
			wantParts: []string{
				"No SAML group links found.",
			},
			dontWant: []string{
				"| Name |",
			},
		},
		{
			name: "single link renders table",
			input: ListOutput{
				Links: []Output{
					{Name: "saml-devs", AccessLevel: 30, Provider: "okta"},
				},
			},
			wantParts: []string{
				"**1 SAML link(s)**",
				"| Name | Access Level | Provider |",
				"| saml-devs | 30 | okta |",
			},
		},
		{
			name: "multiple links render all rows",
			input: ListOutput{
				Links: []Output{
					{Name: "saml-devs", AccessLevel: 30, Provider: ""},
					{Name: "saml-admins", AccessLevel: 50, Provider: "azure-ad"},
				},
			},
			wantParts: []string{
				"**2 SAML link(s)**",
				"| saml-devs | 30 |",
				"| saml-admins | 50 | azure-ad |",
			},
		},
		{
			name:  "empty links slice returns no-results message",
			input: ListOutput{Links: []Output{}},
			wantParts: []string{
				"No SAML group links found.",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatListMarkdown(tt.input)
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
