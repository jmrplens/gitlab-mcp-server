// markdown_test.go contains unit tests for the Markdown formatting functions
// in the memberroles package. Each case compares the whole document the
// formatter renders: a card for one role with its permissions as a nested
// collection, and a table for a list of them.
package memberroles

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// roleCardHints is the guidance section every member-role card ends with.
const roleCardHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'member_role.list_group' to view every custom role of a group\n" +
	"- Use action 'member_role.list_instance' to view every custom role of the instance\n"

// roleListHints is the guidance section a list of member roles ends with.
const roleListHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'member_role.create_group' to define a new custom role in a group\n" +
	"- Use action 'member_role.create_instance' to define a new custom role on the instance\n"

// TestFormatOutputMarkdown validates the whole card FormatOutputMarkdown
// renders for a member role: the base access level named as well as numbered,
// the permissions the role carries as a table of their own, and nothing at all
// for a role GitLab never sent.
func TestFormatOutputMarkdown(t *testing.T) {
	trueVal := true
	falseVal := false

	tests := []struct {
		name  string
		input Output
		want  string
	}{
		{
			name:  "returns empty string for zero-ID role",
			input: Output{},
			want:  "",
		},
		{
			name: "renders role with description and group ID",
			input: Output{
				ID:              42,
				Name:            "custom-dev",
				Description:     "Custom developer role",
				GroupID:         100,
				BaseAccessLevel: 30,
			},
			want: "## Member Role #42: custom-dev\n\n" +
				"- **Description**: Custom developer role\n" +
				"- **Group ID**: 100\n" +
				"- **Base Access Level**: Developer (30)\n" +
				roleCardHints,
		},
		{
			name: "renders role without description and without group ID",
			input: Output{
				ID:              7,
				Name:            "reader",
				BaseAccessLevel: 10,
			},
			want: "## Member Role #7: reader\n\n" +
				"- **Base Access Level**: Guest (10)\n" +
				roleCardHints,
		},
		{
			name: "renders the permissions the role carries",
			input: Output{
				ID:              1,
				Name:            "two-perms",
				BaseAccessLevel: 30,
				Permissions: Permissions{
					AdminCICDVariables: &trueVal,
					ReadCode:           &trueVal,
					ReadRunners:        &falseVal,
				},
			},
			want: "## Member Role #1: two-perms\n\n" +
				"- **Base Access Level**: Developer (30)\n" +
				"\n### Permissions\n\n" +
				"| Permission | Granted |\n| --- | --- |\n" +
				"| Admin CI/CD Variables | " + toolutil.BoolEmoji(true) + " |\n" +
				"| Read Code | " + toolutil.BoolEmoji(true) + " |\n" +
				roleCardHints,
		},
		{
			name: "writes no permissions table when the role carries none",
			input: Output{
				ID:              2,
				Name:            "no-perms",
				BaseAccessLevel: 10,
				Permissions: Permissions{
					ReadCode:    &falseVal,
					ReadRunners: &falseVal,
				},
			},
			want: "## Member Role #2: no-perms\n\n" +
				"- **Base Access Level**: Guest (10)\n" +
				roleCardHints,
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

// TestFormatListMarkdown validates the whole table FormatListMarkdown renders:
// the heading counting the roles, one row each with the base level named, a
// dash where an instance role has no group, and the guidance last.
func TestFormatListMarkdown(t *testing.T) {
	const header = "| ID | Name | Base Level | Group ID |\n| --- | --- | --- | --- |\n"

	tests := []struct {
		name  string
		input ListOutput
		want  string
	}{
		{
			name:  "returns message for empty list",
			input: ListOutput{},
			want:  "No member roles found.\n",
		},
		{
			name: "renders single role with group ID",
			input: ListOutput{
				Roles: []Output{
					{ID: 1, Name: "dev-role", BaseAccessLevel: 30, GroupID: 100},
				},
			},
			want: "## Member Roles (1)\n\n" + header +
				"| 1 | dev-role | Developer (30) | 100 |\n" +
				roleListHints,
		},
		{
			name: "renders multiple roles",
			input: ListOutput{
				Roles: []Output{
					{ID: 1, Name: "reader", BaseAccessLevel: 10, GroupID: 50},
					{ID: 2, Name: "developer", BaseAccessLevel: 30, GroupID: 50},
					{ID: 3, Name: "admin", BaseAccessLevel: 40, GroupID: 0},
				},
			},
			want: "## Member Roles (3)\n\n" + header +
				"| 1 | reader | Guest (10) | 50 |\n" +
				"| 2 | developer | Developer (30) | 50 |\n" +
				"| 3 | admin | Maintainer (40) | - |\n" +
				roleListHints,
		},
		{
			name: "uses dash for zero group ID",
			input: ListOutput{
				Roles: []Output{
					{ID: 5, Name: "instance-role", BaseAccessLevel: 20, GroupID: 0},
				},
			},
			want: "## Member Roles (1)\n\n" + header +
				"| 5 | instance-role | Reporter (20) | - |\n" +
				roleListHints,
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

// TestMarkdownInit_Registry verifies the init markdown formatters are registered.
func TestMarkdownInit_Registry(t *testing.T) {
	out := toolutil.MarkdownForResult(ListOutput{})
	if out == nil {
		t.Fatal("expected non-nil result for ListOutput")
	}
}
