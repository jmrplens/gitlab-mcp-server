// markdown.go provides Markdown formatting functions for member role
// MCP tool output.

package memberroles

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown renders a single member role as Markdown.
func FormatOutputMarkdown(o Output) string {
	if o.ID == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Member Role #%d — %s\n\n", o.ID, o.Name)
	if o.Description != "" {
		fmt.Fprintf(&b, "- **Description**: %s\n", o.Description)
	}
	if o.GroupID != 0 {
		fmt.Fprintf(&b, "- **Group ID**: %d\n", o.GroupID)
	}
	fmt.Fprintf(&b, "- **Base Access Level**: %d\n", o.BaseAccessLevel)
	b.WriteString("\n### Permissions\n\n")
	b.WriteString("| Permission | Granted |\n")
	b.WriteString("| ---------- | :-----: |\n")
	writePermRow(&b, "Admin CI/CD Variables", o.AdminCICDVariables)
	writePermRow(&b, "Admin Compliance Framework", o.AdminComplianceFramework)
	writePermRow(&b, "Admin Group Members", o.AdminGroupMembers)
	writePermRow(&b, "Admin Merge Requests", o.AdminMergeRequests)
	writePermRow(&b, "Admin Push Rules", o.AdminPushRules)
	writePermRow(&b, "Admin Terraform State", o.AdminTerraformState)
	writePermRow(&b, "Admin Vulnerability", o.AdminVulnerability)
	writePermRow(&b, "Admin Webhooks", o.AdminWebHook)
	writePermRow(&b, "Archive Project", o.ArchiveProject)
	writePermRow(&b, "Manage Deploy Tokens", o.ManageDeployTokens)
	writePermRow(&b, "Manage Group Access Tokens", o.ManageGroupAccessTokens)
	writePermRow(&b, "Manage MR Settings", o.ManageMergeRequestSettings)
	writePermRow(&b, "Manage Project Access Tokens", o.ManageProjectAccessTokens)
	writePermRow(&b, "Manage Security Policy Link", o.ManageSecurityPolicyLink)
	writePermRow(&b, "Read Code", o.ReadCode)
	writePermRow(&b, "Read Runners", o.ReadRunners)
	writePermRow(&b, "Read Dependency", o.ReadDependency)
	writePermRow(&b, "Read Vulnerability", o.ReadVulnerability)
	writePermRow(&b, "Remove Group", o.RemoveGroup)
	writePermRow(&b, "Remove Project", o.RemoveProject)
	toolutil.WriteHints(&b,
		"Use `gitlab_list_instance_member_roles` or `gitlab_list_group_member_roles` to view all roles",
	)
	return b.String()
}

func writePermRow(b *strings.Builder, name string, val *bool) {
	if val != nil && *val {
		fmt.Fprintf(b, "| %s | ✓ |\n", name)
	}
}

// FormatListMarkdown renders a list of member roles as Markdown.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Roles) == 0 {
		return "No member roles found."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Member Roles (%d)\n\n", len(out.Roles))
	b.WriteString("| ID | Name | Base Level | Group ID |\n")
	b.WriteString("| --: | ---- | ---------: | -------: |\n")
	for _, r := range out.Roles {
		gid := "—"
		if r.GroupID != 0 {
			gid = strconv.FormatInt(r.GroupID, 10)
		}
		fmt.Fprintf(&b, "| %d | %s | %d | %s |\n",
			r.ID,
			toolutil.EscapeMdTableCell(r.Name),
			r.BaseAccessLevel,
			gid,
		)
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_create_instance_member_role` or `gitlab_create_group_member_role` to define a new custom role",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
