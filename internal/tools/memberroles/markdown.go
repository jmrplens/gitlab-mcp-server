package memberroles

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatOutputMarkdown renders a single member role as Markdown.
func FormatOutputMarkdown(o Output) string {
	if o.ID == 0 {
		return ""
	}
	var b strings.Builder
	// A custom member role's name and description are typed by the group owner
	// who created it.
	fmt.Fprintf(&b, "## Member Role #%d: %s\n\n", o.ID, toolutil.EscapeMdHeading(o.Name))
	if o.Description != "" {
		fmt.Fprintf(&b, "- **Description**: %s\n", toolutil.EscapeMdTableCell(o.Description))
	}
	if o.GroupID != 0 {
		fmt.Fprintf(&b, "- **Group ID**: %d\n", o.GroupID)
	}
	fmt.Fprintf(&b, "- **Base Access Level**: %d\n", o.BaseAccessLevel)
	b.WriteString("\n### Permissions\n\n")
	b.WriteString("| Permission | Granted |\n")
	b.WriteString("| ---------- | :-----: |\n")
	for _, row := range permissionRows(o) {
		writePermRow(&b, row.label, row.granted)
	}
	toolutil.WriteHints(
		&b,
		"Use `gitlab_list_instance_member_roles` or `gitlab_list_group_member_roles` to view all roles",
	)
	return b.String()
}

// permissionRow is one line of the permissions table: the label a reader sees
// and the flag deciding whether the role carries it.
type permissionRow struct {
	label   string
	granted *bool
}

// permissionRows is every customizable permission a member role can carry, in
// the order the table prints them: the twenty client-go models, then the
// twenty-five read from the captured response.
//
// A table rather than a run of calls because the second half grows whenever
// GitLab adds a customizable permission, which is most releases, and a list is
// where that is one line rather than a line plus a call site.
func permissionRows(o Output) []permissionRow {
	return []permissionRow{
		{"Admin CI/CD Variables", o.AdminCICDVariables},
		{"Admin Compliance Framework", o.AdminComplianceFramework},
		{"Admin Group Members", o.AdminGroupMembers},
		{"Admin Merge Requests", o.AdminMergeRequests},
		{"Admin Push Rules", o.AdminPushRules},
		{"Admin Terraform State", o.AdminTerraformState},
		{"Admin Vulnerability", o.AdminVulnerability},
		{"Admin Webhooks", o.AdminWebHook},
		{"Archive Project", o.ArchiveProject},
		{"Manage Deploy Tokens", o.ManageDeployTokens},
		{"Manage Group Access Tokens", o.ManageGroupAccessTokens},
		{"Manage MR Settings", o.ManageMergeRequestSettings},
		{"Manage Project Access Tokens", o.ManageProjectAccessTokens},
		{"Manage Security Policy Link", o.ManageSecurityPolicyLink},
		{"Read Code", o.ReadCode},
		{"Read Runners", o.ReadRunners},
		{"Read Dependency", o.ReadDependency},
		{"Read Vulnerability", o.ReadVulnerability},
		{"Remove Group", o.RemoveGroup},
		{"Remove Project", o.RemoveProject},
		{"Admin AI Catalog Item", o.AdminAICatalogItem},
		{"Admin AI Catalog Item Consumer", o.AdminAICatalogItemConsumer},
		{"Admin Integrations", o.AdminIntegrations},
		{"Admin Protected Branch", o.AdminProtectedBranch},
		{"Admin Protected Environments", o.AdminProtectedEnvironments},
		{"Admin Runners", o.AdminRunners},
		{"Admin Security Attributes", o.AdminSecurityAttributes},
		{"Apply Security Scan Profiles", o.ApplySecurityScanProfiles},
		{"Create Security Scan Profiles", o.CreateSecurityScanProfiles},
		{"Delete Security Scan Profiles", o.DeleteSecurityScanProfiles},
		{"Destroy Package", o.DestroyPackage},
		{"Read Admin CI/CD", o.ReadAdminCICD},
		{"Read Admin Groups", o.ReadAdminGroups},
		{"Read Admin Monitoring", o.ReadAdminMonitoring},
		{"Read Admin Projects", o.ReadAdminProjects},
		{"Read Admin Subscription", o.ReadAdminSubscription},
		{"Read Admin Users", o.ReadAdminUsers},
		{"Read Agent Artifacts", o.ReadAgentArtifacts},
		{"Read Compliance Dashboard", o.ReadComplianceDashboard},
		{"Read CRM Contact", o.ReadCRMContact},
		{"Read Security Attribute", o.ReadSecurityAttribute},
		{"Read Security Scan Profiles", o.ReadSecurityScanProfiles},
		{"Read Virtual Registry", o.ReadVirtualRegistry},
		{"Update Security AI Workflow Settings", o.UpdateSecAIWorkflowSettings},
		{"Update Security Scan Profiles", o.UpdateSecurityScanProfiles},
	}
}

// writePermRow appends a table row for an enabled permission. The value cell
// holds a check mark (✓), written as an escape so the source stays ASCII.
//
//gitlab:allow-unescaped name: a permission label from permissionRows, written by this file and never a value GitLab sent.
func writePermRow(b *strings.Builder, name string, val *bool) {
	if val != nil && *val {
		fmt.Fprintf(b, "| %s | \u2713 |\n", name)
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
		gid := "-"
		if r.GroupID != 0 {
			gid = strconv.FormatInt(r.GroupID, 10)
		}
		fmt.Fprintf(
			&b, "| %d | %s | %d | %s |\n",
			r.ID,
			toolutil.EscapeMdTableCell(r.Name),
			r.BaseAccessLevel,
			gid,
		)
	}
	toolutil.WriteHints(
		&b,
		"Use `gitlab_create_instance_member_role` or `gitlab_create_group_member_role` to define a new custom role",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
