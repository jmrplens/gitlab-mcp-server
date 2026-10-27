// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package memberroles

import (
	"fmt"
	"strconv"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// accessLevel renders a role's numeric base access level as the name GitLab
// gives it with the number beside it, "Developer (30)": the base level is the
// role a custom role starts from, and the bare integer said nothing about it.
func accessLevel(level int) string {
	return fmt.Sprintf("%s (%d)", toolutil.AccessLevelDescription(gl.AccessLevelValue(level)), level)
}

// FormatOutputMarkdown renders a single member role as Markdown.
func FormatOutputMarkdown(o Output) string {
	if o.ID == 0 {
		return ""
	}
	var b strings.Builder
	// A custom member role's name and description are typed by the group owner
	// who created it.
	c := toolutil.NewCard(&b, fmt.Sprintf("Member Role #%d: %s", o.ID, o.Name))
	c.Text("Description", o.Description)
	c.Count("Group ID", o.GroupID)
	c.Field("Base Access Level", accessLevel(o.BaseAccessLevel))
	writeGrantedPermissions(c, o)
	c.End(
		toolutil.HintAction("member_role.list_group", "view every custom role of a group"),
		toolutil.HintAction("member_role.list_instance", "view every custom role of the instance"),
	)
	return b.String()
}

// writeGrantedPermissions writes the permissions the role carries as a nested
// collection, and nothing at all when it carries none: a header over no rows
// is a table that says a role has permissions and names none.
func writeGrantedPermissions(c *toolutil.Card, o Output) {
	granted := make([]string, 0, len(permissionRows(o)))
	for _, row := range permissionRows(o) {
		if row.granted != nil && *row.granted {
			granted = append(granted, row.label)
		}
	}
	if len(granted) == 0 {
		return
	}
	t := c.Table("Permissions", "Permission", "Granted")
	for _, label := range granted {
		t.Row(toolutil.EscapeMdTableCell(label), toolutil.BoolEmoji(true))
	}
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

// FormatListMarkdown renders a group's member roles as Markdown. GitLab
// serves the group routes, this list among them, only on GitLab.com: a
// self-managed instance answers each of them 400 as deprecated, and GitLab.com
// refuses the instance routes, so a group list reaches this formatter only
// where the group create works, and that is its one next step.
func FormatListMarkdown(out ListOutput) string {
	return formatRoleList(out.Roles,
		toolutil.HintAction("member_role.create_group", "define a new custom role in a group"))
}

// FormatInstanceListMarkdown renders the instance's member roles as Markdown.
// Instance-level roles exist only on self-managed, where a group-level create
// is refused as deprecated, so its one next step is the instance create.
func FormatInstanceListMarkdown(out InstanceListOutput) string {
	return formatRoleList(out.Roles,
		toolutil.HintAction("member_role.create_instance", "define a new custom role on the instance"))
}

// formatRoleList renders a list of member roles with the next step its scope
// serves.
func formatRoleList(roles []Output, next string) string {
	if len(roles) == 0 {
		return toolutil.EmptyMessage("member roles")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Member Roles", len(roles), toolutil.PaginationOutput{})
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Base Level", "Group ID"))
	for _, r := range roles {
		gid := "-"
		if r.GroupID != 0 {
			gid = strconv.FormatInt(r.GroupID, 10)
		}
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(r.ID, 10),
			toolutil.EscapeMdTableCell(r.Name),
			accessLevel(r.BaseAccessLevel),
			gid,
		))
	}
	toolutil.WriteListFooter(&b, toolutil.PaginationOutput{}, false, next)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatInstanceListMarkdown)
}
