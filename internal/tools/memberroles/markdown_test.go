// markdown_test.go contains unit tests for the Markdown formatting functions
// in the memberroles package. It covers FormatOutputMarkdown (single role
// rendering with and without permissions, empty role), FormatListMarkdown
// (empty list, single role, multiple roles, zero GroupID), and the
// writePermRow helper via integration through FormatOutputMarkdown.
package memberroles

import (
	"strings"
	"testing"
)

// TestFormatOutputMarkdown validates that FormatOutputMarkdown renders a single
// member role with description, group ID, base access level, and a permissions
// table. Each subtest covers a specific rendering scenario.
func TestFormatOutputMarkdown(t *testing.T) {
	trueVal := true
	falseVal := false

	tests := []struct {
		name       string
		input      Output
		wantEmpty  bool
		contains   []string
		notContain []string
	}{
		{
			name:      "returns empty string for zero-ID role",
			input:     Output{},
			wantEmpty: true,
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
			contains: []string{
				"## Member Role #42 — custom-dev",
				"**Description**: Custom developer role",
				"**Group ID**: 100",
				"**Base Access Level**: 30",
				"### Permissions",
				"| Permission | Granted |",
			},
		},
		{
			name: "renders role without description and without group ID",
			input: Output{
				ID:              7,
				Name:            "reader",
				BaseAccessLevel: 10,
			},
			contains: []string{
				"## Member Role #7 — reader",
				"**Base Access Level**: 10",
			},
			notContain: []string{
				"**Description**",
				"**Group ID**",
			},
		},
		{
			name: "renders granted permissions with checkmarks",
			input: Output{
				ID:              1,
				Name:            "all-perms",
				BaseAccessLevel: 30,
				Permissions: Permissions{
					AdminCICDVariables:         &trueVal,
					AdminComplianceFramework:   &trueVal,
					AdminGroupMembers:          &trueVal,
					AdminMergeRequests:         &trueVal,
					AdminPushRules:             &trueVal,
					AdminTerraformState:        &trueVal,
					AdminVulnerability:         &trueVal,
					AdminWebHook:               &trueVal,
					ArchiveProject:             &trueVal,
					ManageDeployTokens:         &trueVal,
					ManageGroupAccessTokens:    &trueVal,
					ManageMergeRequestSettings: &trueVal,
					ManageProjectAccessTokens:  &trueVal,
					ManageSecurityPolicyLink:   &trueVal,
					ReadCode:                   &trueVal,
					ReadRunners:                &trueVal,
					ReadDependency:             &trueVal,
					ReadVulnerability:          &trueVal,
					RemoveGroup:                &trueVal,
					RemoveProject:              &trueVal,
				},
			},
			contains: []string{
				"| Admin CI/CD Variables | ✓ |",
				"| Admin Compliance Framework | ✓ |",
				"| Admin Group Members | ✓ |",
				"| Admin Merge Requests | ✓ |",
				"| Admin Push Rules | ✓ |",
				"| Admin Terraform State | ✓ |",
				"| Admin Vulnerability | ✓ |",
				"| Admin Webhooks | ✓ |",
				"| Archive Project | ✓ |",
				"| Manage Deploy Tokens | ✓ |",
				"| Manage Group Access Tokens | ✓ |",
				"| Manage MR Settings | ✓ |",
				"| Manage Project Access Tokens | ✓ |",
				"| Manage Security Policy Link | ✓ |",
				"| Read Code | ✓ |",
				"| Read Runners | ✓ |",
				"| Read Dependency | ✓ |",
				"| Read Vulnerability | ✓ |",
				"| Remove Group | ✓ |",
				"| Remove Project | ✓ |",
			},
		},
		{
			name: "omits false permissions from table rows",
			input: Output{
				ID:              2,
				Name:            "no-perms",
				BaseAccessLevel: 10,
				Permissions: Permissions{
					ReadCode:    &falseVal,
					ReadRunners: &falseVal,
				},
			},
			contains: []string{
				"### Permissions",
				"| Permission | Granted |",
			},
			notContain: []string{
				"| Read Code | ✓ |",
				"| Read Runners | ✓ |",
			},
		},
		{
			name: "includes hints section",
			input: Output{
				ID:              3,
				Name:            "hinted",
				BaseAccessLevel: 20,
			},
			contains: []string{
				"gitlab_list_instance_member_roles",
				"gitlab_list_group_member_roles",
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
					t.Errorf("output missing expected substring %q", s)
				}
			}
			for _, s := range tt.notContain {
				if strings.Contains(got, s) {
					t.Errorf("output should not contain %q", s)
				}
			}
		})
	}
}

// TestFormatListMarkdown validates that FormatListMarkdown renders a table of
// member roles with ID, name, base level, and group ID columns. Covers empty
// list, single role, multiple roles, and roles without a group ID.
func TestFormatListMarkdown(t *testing.T) {
	tests := []struct {
		name       string
		input      ListOutput
		contains   []string
		notContain []string
	}{
		{
			name:  "returns message for empty list",
			input: ListOutput{},
			contains: []string{
				"No member roles found.",
			},
			notContain: []string{
				"| ID |",
			},
		},
		{
			name: "renders single role with group ID",
			input: ListOutput{
				Roles: []Output{
					{ID: 1, Name: "dev-role", BaseAccessLevel: 30, GroupID: 100},
				},
			},
			contains: []string{
				"## Member Roles (1)",
				"| ID | Name | Base Level | Group ID |",
				"| 1 | dev-role | 30 | 100 |",
			},
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
			contains: []string{
				"## Member Roles (3)",
				"| 1 | reader | 10 | 50 |",
				"| 2 | developer | 30 | 50 |",
				"| 3 | admin | 40 | — |",
			},
		},
		{
			name: "uses dash for zero group ID",
			input: ListOutput{
				Roles: []Output{
					{ID: 5, Name: "instance-role", BaseAccessLevel: 20, GroupID: 0},
				},
			},
			contains: []string{
				"| 5 | instance-role | 20 | — |",
			},
			notContain: []string{
				"| 0 |",
			},
		},
		{
			name: "includes hints section",
			input: ListOutput{
				Roles: []Output{
					{ID: 1, Name: "r", BaseAccessLevel: 10},
				},
			},
			contains: []string{
				"gitlab_create_instance_member_role",
				"gitlab_create_group_member_role",
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
					t.Errorf("output missing expected substring %q", s)
				}
			}
			for _, s := range tt.notContain {
				if strings.Contains(got, s) {
					t.Errorf("output should not contain %q", s)
				}
			}
		})
	}
}
