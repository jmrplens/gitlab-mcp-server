package prompts

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// Access level names from GitLab's permission model.
var accessLevelNames = map[gl.AccessLevelValue]string{
	10: "Guest",
	20: "Reporter",
	30: "Developer",
	40: "Maintainer",
	50: "Owner",
}

// accessLevelName returns the human-readable name for a GitLab access level.
func accessLevelName(level gl.AccessLevelValue) string {
	if name, ok := accessLevelNames[level]; ok {
		return name
	}
	return fmt.Sprintf("Unknown(%d)", level)
}

// settingValueTableHeader is the common Markdown table header for setting/value tables.
const settingValueTableHeader = "| Setting | Value |\n|---------|-------|\n"

// isDefaultBranchProtected checks if the default branch is in the protected branches list.
func isDefaultBranchProtected(branches []*gl.ProtectedBranch, defaultBranch string) bool {
	for _, pb := range branches {
		if pb.Name == defaultBranch {
			return true
		}
	}
	return false
}

// registerAuditPrompts registers all project audit prompts.
func registerAuditPrompts(server *mcp.Server, client *gitlabclient.Client) {
	registerAuditProjectSettingsPrompt(server, client)
	registerAuditBranchProtectionPrompt(server, client)
	registerAuditProjectAccessPrompt(server, client)
	registerAuditProjectWorkflowPrompt(server, client)
	registerAuditProjectFullPrompt(server, client)
}

// audit_project_settings.

// registerAuditProjectSettingsPrompt registers the audit_project_settings prompt.
func registerAuditProjectSettingsPrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:  "audit_project_settings",
		Title: toolutil.TitleFromName("audit_project_settings"),
		Description: "Audit a GitLab project's core settings. Reviews visibility, merge strategy, " +
			"CI/CD configuration, default branch, wiki/issues/snippets toggles, and push rules. " +
			"Use this to identify misconfigurations or deviations from best practices.",
		Icons: toolutil.IconSecurity,
		Arguments: []*mcp.PromptArgument{
			projectIDArg(),
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleAuditProjectSettings(ctx, client, req)
	})
}

// handleAuditProjectSettings performs the handle audit project settings operation using the GitLab API and returns [*mcp.GetPromptResult].
func handleAuditProjectSettings(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	projectID := req.Params.Arguments[argProjectID]
	if projectID == "" {
		return nil, fmt.Errorf("audit_project_settings: %s is required", argProjectID)
	}

	project, _, err := client.GL().Projects.GetProject(projectID, &gl.GetProjectOptions{
		Statistics: new(true),
	}, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("audit_project_settings: failed to get project: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Project Settings Audit — %s\n\n", project.PathWithNamespace)

	// General info
	b.WriteString("## General\n\n")
	b.WriteString(settingValueTableHeader)
	fmt.Fprintf(&b, "| Name | %s |\n", project.Name)
	fmt.Fprintf(&b, "| Path | %s |\n", project.PathWithNamespace)
	fmt.Fprintf(&b, "| Description | %s |\n", emptyDash(project.Description))
	fmt.Fprintf(&b, "| Visibility | %s |\n", string(project.Visibility))
	fmt.Fprintf(&b, "| Default branch | %s |\n", emptyDash(project.DefaultBranch))
	fmt.Fprintf(&b, "| Created | %s |\n", formatTimePtr(project.CreatedAt))
	fmt.Fprintf(&b, "| Last activity | %s |\n", formatTimePtr(project.LastActivityAt))
	b.WriteString("\n")

	// Feature toggles
	b.WriteString("## Feature Toggles\n\n")
	b.WriteString("| Feature | Enabled |\n|---------|---------|\n")
	fmt.Fprintf(&b, "| Issues | %s |\n", accessLevelIcon(project.IssuesAccessLevel))
	fmt.Fprintf(&b, "| Merge Requests | %s |\n", accessLevelIcon(project.MergeRequestsAccessLevel))
	fmt.Fprintf(&b, "| Wiki | %s |\n", accessLevelIcon(project.WikiAccessLevel))
	fmt.Fprintf(&b, "| Snippets | %s |\n", accessLevelIcon(project.SnippetsAccessLevel))
	fmt.Fprintf(&b, "| Container Registry | %s |\n", accessLevelIcon(project.ContainerRegistryAccessLevel))
	//lint:ignore SA1019 backward compat with PackagesEnabled field
	fmt.Fprintf(&b, "| Packages | %s |\n", toolutil.BoolEmoji(project.PackagesEnabled)) //nolint:staticcheck // SA1019
	b.WriteString("\n")

	// Merge settings
	b.WriteString("## Merge Settings\n\n")
	b.WriteString(settingValueTableHeader)
	fmt.Fprintf(&b, "| Merge method | %s |\n", emptyDash(string(project.MergeMethod)))
	fmt.Fprintf(&b, "| Squash option | %s |\n", emptyDash(string(project.SquashOption)))
	fmt.Fprintf(&b, "| Only merge if pipeline succeeds | %s |\n", toolutil.BoolEmoji(project.OnlyAllowMergeIfPipelineSucceeds))
	fmt.Fprintf(&b, "| Only merge if all discussions resolved | %s |\n", toolutil.BoolEmoji(project.OnlyAllowMergeIfAllDiscussionsAreResolved))
	fmt.Fprintf(&b, "| Remove source branch on merge | %s |\n", toolutil.BoolEmoji(project.RemoveSourceBranchAfterMerge))
	fmt.Fprintf(&b, "| Allow merge on skipped pipeline | %s |\n", toolutil.BoolEmoji(project.AllowMergeOnSkippedPipeline))
	b.WriteString("\n")

	// CI/CD settings
	b.WriteString("## CI/CD Settings\n\n")
	b.WriteString(settingValueTableHeader)
	fmt.Fprintf(&b, "| CI config path | %s |\n", emptyDash(project.CIConfigPath))
	fmt.Fprintf(&b, "| Auto DevOps enabled | %s |\n", toolutil.BoolEmoji(project.AutoDevopsEnabled))
	fmt.Fprintf(&b, "| Public pipelines | %s |\n", toolutil.BoolEmoji(project.PublicJobs))
	fmt.Fprintf(&b, "| Shared runners | %s |\n", toolutil.BoolEmoji(project.SharedRunnersEnabled))
	b.WriteString("\n")

	// Push rules (best-effort — may fail for non-premium)
	pushRule, _, pushErr := client.GL().Projects.GetProjectPushRules(projectID, gl.WithContext(ctx))
	if pushErr != nil {
		slog.Debug("push rules not available", "error", pushErr)
		b.WriteString("## Push Rules\n\nPush rules not available (may require GitLab Premium).\n\n")
	} else if pushRule != nil {
		b.WriteString("## Push Rules\n\n")
		b.WriteString("| Rule | Value |\n|------|-------|\n")
		fmt.Fprintf(&b, "| Deny delete tag | %s |\n", toolutil.BoolEmoji(pushRule.DenyDeleteTag))
		fmt.Fprintf(&b, "| Member check | %s |\n", toolutil.BoolEmoji(pushRule.MemberCheck))
		fmt.Fprintf(&b, "| Prevent secrets | %s |\n", toolutil.BoolEmoji(pushRule.PreventSecrets))
		fmt.Fprintf(&b, "| Commit message regex | %s |\n", emptyDash(pushRule.CommitMessageRegex))
		fmt.Fprintf(&b, "| Branch name regex | %s |\n", emptyDash(pushRule.BranchNameRegex))
		fmt.Fprintf(&b, "| Author email regex | %s |\n", emptyDash(pushRule.AuthorEmailRegex))
		fmt.Fprintf(&b, "| File name regex | %s |\n", emptyDash(pushRule.FileNameRegex))
		fmt.Fprintf(&b, "| Max file size (MB) | %d |\n", pushRule.MaxFileSize)
		b.WriteString("\n")
	}

	// Statistics
	if project.Statistics != nil {
		b.WriteString("## Storage Statistics\n\n")
		b.WriteString("| Metric | Size |\n|--------|------|\n")
		fmt.Fprintf(&b, "| Repository | %s |\n", formatBytes(project.Statistics.RepositorySize))
		fmt.Fprintf(&b, "| LFS | %s |\n", formatBytes(project.Statistics.LFSObjectsSize))
		fmt.Fprintf(&b, "| Job artifacts | %s |\n", formatBytes(project.Statistics.JobArtifactsSize))
		fmt.Fprintf(&b, "| Packages | %s |\n", formatBytes(project.Statistics.PackagesSize))
		fmt.Fprintf(&b, "| Uploads | %s |\n", formatBytes(project.Statistics.UploadsSize))
		fmt.Fprintf(&b, "| Total | %s |\n", formatBytes(project.Statistics.StorageSize))
		b.WriteString("\n")
	}

	b.WriteString("---\nPlease analyze this project's configuration, identify potential security risks, " +
		"deviations from best practices, and provide specific recommendations for improvement. " +
		"Focus on merge settings, CI/CD configuration, and push rule enforcement.\n")

	return promptResult(b.String()), nil
}

// audit_branch_protection.

// registerAuditBranchProtectionPrompt registers the audit_branch_protection prompt.
func registerAuditBranchProtectionPrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:  "audit_branch_protection",
		Title: toolutil.TitleFromName("audit_branch_protection"),
		Description: "Audit branch protection rules for a GitLab project. Lists all protected branches " +
			"with push/merge access levels and code owner approval settings. Identifies whether the " +
			"default branch is protected and highlights potential security gaps.",
		Icons: toolutil.IconSecurity,
		Arguments: []*mcp.PromptArgument{
			projectIDArg(),
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleAuditBranchProtection(ctx, client, req)
	})
}

// handleAuditBranchProtection performs the handle audit branch protection operation using the GitLab API and returns [*mcp.GetPromptResult].
func handleAuditBranchProtection(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	projectID := req.Params.Arguments[argProjectID]
	if projectID == "" {
		return nil, fmt.Errorf("audit_branch_protection: %s is required", argProjectID)
	}

	project, _, err := client.GL().Projects.GetProject(projectID, &gl.GetProjectOptions{}, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("audit_branch_protection: failed to get project: %w", err)
	}

	protectedBranches, _, err := client.GL().ProtectedBranches.ListProtectedBranches(projectID, &gl.ListProtectedBranchesOptions{
		ListOptions: gl.ListOptions{PerPage: maxListItems},
	}, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("audit_branch_protection: failed to list protected branches: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Branch Protection Audit — %s\n\n", project.PathWithNamespace)
	fmt.Fprintf(&b, "**Default branch**: %s\n\n", project.DefaultBranch)

	defaultProtected := isDefaultBranchProtected(protectedBranches, project.DefaultBranch)

	b.WriteString("## Summary\n\n")
	b.WriteString("| Metric | Value |\n|--------|-------|\n")
	fmt.Fprintf(&b, "| Protected branches | %d |\n", len(protectedBranches))
	fmt.Fprintf(&b, "| Default branch protected | %s |\n", toolutil.BoolEmoji(defaultProtected))
	b.WriteString("\n")

	if len(protectedBranches) == 0 {
		b.WriteString(toolutil.EmojiWarning + " **No protected branches found.** This is a significant security risk.\n\n")
	} else {
		b.WriteString("## Protected Branch Details\n\n")
		for _, pb := range protectedBranches {
			writeBranchDetail(&b, pb, project.DefaultBranch)
		}
	}

	b.WriteString("---\nPlease analyze the branch protection configuration, identify security gaps " +
		"(unprotected default branch, overly permissive push/merge access, missing code owner approvals), " +
		"and recommend improvements aligned with GitLab security best practices.\n")

	return promptResult(b.String()), nil
}

// writeBranchDetail writes the detail section for a single protected branch.
func writeBranchDetail(b *strings.Builder, pb *gl.ProtectedBranch, defaultBranch string) {
	suffix := ""
	if pb.Name == defaultBranch {
		suffix = " (default)"
	}
	fmt.Fprintf(b, "### %s%s\n\n", pb.Name, suffix)

	writeAccessLevelLine(b, "Push access", pb.PushAccessLevels)
	writeAccessLevelLine(b, "Merge access", pb.MergeAccessLevels)
	if len(pb.UnprotectAccessLevels) > 0 {
		writeAccessLevelLine(b, "Unprotect access", pb.UnprotectAccessLevels)
	}
	fmt.Fprintf(b, "**Allow force push:** %s\n", toolutil.BoolEmoji(pb.AllowForcePush))
	fmt.Fprintf(b, "**Code owner approval required:** %s\n\n", toolutil.BoolEmoji(pb.CodeOwnerApprovalRequired))
}

// writeAccessLevelLine writes a labeled line of branch access levels.
func writeAccessLevelLine(b *strings.Builder, label string, levels []*gl.BranchAccessDescription) {
	fmt.Fprintf(b, "**%s:** ", label)
	if len(levels) == 0 {
		b.WriteString("No restrictions\n")
		return
	}
	var parts []string
	for _, al := range levels {
		parts = append(parts, formatBranchAccessLevel(al))
	}
	b.WriteString(strings.Join(parts, ", ") + "\n")
}

// formatBranchAccessLevel formats a BranchAccessDescription for display.
func formatBranchAccessLevel(al *gl.BranchAccessDescription) string {
	name := accessLevelName(al.AccessLevel)
	if al.UserID != 0 {
		return fmt.Sprintf("User #%d (%s)", al.UserID, name)
	}
	if al.GroupID != 0 {
		return fmt.Sprintf("Group #%d (%s)", al.GroupID, name)
	}
	return name
}

// audit_project_access.

// registerAuditProjectAccessPrompt registers the audit_project_access prompt.
func registerAuditProjectAccessPrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:  "audit_project_access",
		Title: toolutil.TitleFromName("audit_project_access"),
		Description: "Audit user access and permissions for a GitLab project. Lists all members " +
			"(direct and inherited) with their access levels, identifies users with elevated " +
			"privileges (Maintainer/Owner), and flags inactive or blocked accounts.",
		Icons: toolutil.IconSecurity,
		Arguments: []*mcp.PromptArgument{
			projectIDArg(),
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleAuditProjectAccess(ctx, client, req)
	})
}

// memberGroups holds members classified by access level and state.
type memberGroups struct {
	owners, maintainers, developers, reporters, guests []*gl.ProjectMember
	blocked, inactive                                  []*gl.ProjectMember
}

// classifyMembers categorizes project members by access level and account state.
func classifyMembers(members []*gl.ProjectMember) memberGroups {
	var g memberGroups
	for _, m := range members {
		switch {
		case m.AccessLevel >= 50:
			g.owners = append(g.owners, m)
		case m.AccessLevel >= 40:
			g.maintainers = append(g.maintainers, m)
		case m.AccessLevel >= 30:
			g.developers = append(g.developers, m)
		case m.AccessLevel >= 20:
			g.reporters = append(g.reporters, m)
		default:
			g.guests = append(g.guests, m)
		}
		if m.State == "blocked" {
			g.blocked = append(g.blocked, m)
		} else if m.State != "active" {
			g.inactive = append(g.inactive, m)
		}
	}
	return g
}

// writeSharedGroups writes the shared groups section to the builder.
func writeSharedGroups(b *strings.Builder, groups []gl.ProjectSharedWithGroup) {
	if len(groups) == 0 {
		return
	}
	b.WriteString("## Shared With Groups\n\n")
	b.WriteString("| Group | Access Level | Expires |\n|-------|-------------|--------|\n")
	for _, sg := range groups {
		expires := "—"
		if sg.ExpiresAt != nil {
			expires = sg.ExpiresAt.String()
		}
		fmt.Fprintf(b, "| %s (#%d) | %s | %s |\n",
			sg.GroupName, sg.GroupID,
			accessLevelName(gl.AccessLevelValue(sg.GroupAccessLevel)),
			expires)
	}
	b.WriteString("\n")
}

// handleAuditProjectAccess performs the handle audit project access operation using the GitLab API and returns [*mcp.GetPromptResult].
func handleAuditProjectAccess(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	projectID := req.Params.Arguments[argProjectID]
	if projectID == "" {
		return nil, fmt.Errorf("audit_project_access: %s is required", argProjectID)
	}

	members, _, err := client.GL().ProjectMembers.ListAllProjectMembers(projectID, &gl.ListProjectMembersOptions{
		ListOptions: gl.ListOptions{PerPage: maxListItems},
	}, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("audit_project_access: failed to list members: %w", err)
	}

	project, _, projErr := client.GL().Projects.GetProject(projectID, &gl.GetProjectOptions{}, gl.WithContext(ctx))
	if projErr != nil {
		return nil, fmt.Errorf("audit_project_access: failed to get project: %w", projErr)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Project Access Audit — %s\n\n", project.PathWithNamespace)

	g := classifyMembers(members)

	b.WriteString("## Summary\n\n")
	b.WriteString("| Access Level | Count |\n|-------------|-------|\n")
	fmt.Fprintf(&b, "| Owner | %d |\n", len(g.owners))
	fmt.Fprintf(&b, "| Maintainer | %d |\n", len(g.maintainers))
	fmt.Fprintf(&b, "| Developer | %d |\n", len(g.developers))
	fmt.Fprintf(&b, "| Reporter | %d |\n", len(g.reporters))
	fmt.Fprintf(&b, "| Guest | %d |\n", len(g.guests))
	fmt.Fprintf(&b, "| **Total** | **%d** |\n", len(members))
	b.WriteString("\n")

	if len(g.blocked) > 0 || len(g.inactive) > 0 {
		b.WriteString("## " + toolutil.EmojiWarning + " Accounts Needing Attention\n\n")
		if len(g.blocked) > 0 {
			b.WriteString("### Blocked Accounts\n\n")
			writeMemberTable(&b, g.blocked)
		}
		if len(g.inactive) > 0 {
			b.WriteString("### Inactive Accounts\n\n")
			writeMemberTable(&b, g.inactive)
		}
	}

	if len(g.owners) > 0 || len(g.maintainers) > 0 {
		b.WriteString("## Elevated Access (Owner + Maintainer)\n\n")
		elevated := make([]*gl.ProjectMember, 0, len(g.owners)+len(g.maintainers))
		elevated = append(elevated, g.owners...)
		elevated = append(elevated, g.maintainers...)
		writeMemberTable(&b, elevated)
	}

	b.WriteString("## All Members\n\n")
	writeMemberTable(&b, members)

	writeSharedGroups(&b, project.SharedWithGroups)

	b.WriteString("---\nPlease analyze the access configuration, identify security concerns " +
		"(too many maintainers/owners, blocked accounts still listed, overly broad group sharing), " +
		"and recommend access policy improvements following the principle of least privilege.\n")

	return promptResult(b.String()), nil
}

// writeMemberTable writes a Markdown table of project members.
func writeMemberTable(b *strings.Builder, members []*gl.ProjectMember) {
	if len(members) == 0 {
		b.WriteString("No members found.\n\n")
		return
	}
	b.WriteString("| User | Name | Access | State |\n")
	b.WriteString("|------|------|--------|-------|\n")
	for _, m := range members {
		fmt.Fprintf(b, "| @%s | %s | %s | %s |\n",
			m.Username, m.Name, accessLevelName(m.AccessLevel), m.State)
	}
	b.WriteString("\n")
}

// audit_project_workflow.

// registerAuditProjectWorkflowPrompt registers the audit_project_workflow prompt.
func registerAuditProjectWorkflowPrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:  "audit_project_workflow",
		Title: toolutil.TitleFromName("audit_project_workflow"),
		Description: "Audit workflow configuration for a GitLab project: labels (names, colors, descriptions), " +
			"milestones (open/closed, due dates), and issue/MR templates. Identifies gaps like " +
			"labels without descriptions, milestones without due dates, or missing templates.",
		Icons: toolutil.IconSecurity,
		Arguments: []*mcp.PromptArgument{
			projectIDArg(),
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleAuditProjectWorkflow(ctx, client, req)
	})
}

// handleAuditProjectWorkflow performs the handle audit project workflow operation using the GitLab API and returns [*mcp.GetPromptResult].
func handleAuditProjectWorkflow(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	projectID := req.Params.Arguments[argProjectID]
	if projectID == "" {
		return nil, fmt.Errorf("audit_project_workflow: %s is required", argProjectID)
	}

	project, _, err := client.GL().Projects.GetProject(projectID, &gl.GetProjectOptions{}, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("audit_project_workflow: failed to get project: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Workflow Audit — %s\n\n", project.PathWithNamespace)

	labels, _, labelsErr := client.GL().Labels.ListLabels(projectID, &gl.ListLabelsOptions{
		ListOptions: gl.ListOptions{PerPage: maxListItems},
	}, gl.WithContext(ctx))
	if labelsErr != nil {
		slog.Warn("failed to fetch labels", "error", labelsErr)
	}
	writeLabelsAudit(&b, labels)

	activeMilestones, _, _ := client.GL().Milestones.ListMilestones(projectID, &gl.ListMilestonesOptions{
		State:       new("active"),
		ListOptions: gl.ListOptions{PerPage: maxListItems},
	}, gl.WithContext(ctx))
	closedMilestones, _, _ := client.GL().Milestones.ListMilestones(projectID, &gl.ListMilestonesOptions{
		State:       new("closed"),
		ListOptions: gl.ListOptions{PerPage: maxListItems},
	}, gl.WithContext(ctx))
	writeMilestonesAudit(&b, activeMilestones, closedMilestones)

	issueTemplates, _, issueTPLErr := client.GL().ProjectTemplates.ListTemplates(projectID, "issues", &gl.ListProjectTemplatesOptions{
		ListOptions: gl.ListOptions{PerPage: maxListItems},
	}, gl.WithContext(ctx))
	if issueTPLErr != nil {
		slog.Debug("issue templates not available", "error", issueTPLErr)
	}
	mrTemplates, _, mrTPLErr := client.GL().ProjectTemplates.ListTemplates(projectID, "merge_requests", &gl.ListProjectTemplatesOptions{
		ListOptions: gl.ListOptions{PerPage: maxListItems},
	}, gl.WithContext(ctx))
	if mrTPLErr != nil {
		slog.Debug("MR templates not available", "error", mrTPLErr)
	}
	writeTemplatesAudit(&b, issueTemplates, mrTemplates)

	b.WriteString("---\nPlease analyze the workflow configuration, identify gaps (labels without descriptions, " +
		"milestones without due dates, missing templates, missing priority/severity labels), " +
		"and suggest improvements for better project organization and contributor experience.\n")

	return promptResult(b.String()), nil
}

// writeLabelsAudit writes the labels audit section.
func writeLabelsAudit(b *strings.Builder, labels []*gl.Label) {
	b.WriteString("## Labels\n\n")
	if len(labels) == 0 {
		b.WriteString(toolutil.EmojiWarning + " **No labels configured.** Labels help categorize issues and MRs.\n\n")
		return
	}
	labelsNoDesc := 0
	for _, l := range labels {
		if l.Description == "" {
			labelsNoDesc++
		}
	}
	fmt.Fprintf(b, "**Total:** %d | **Without description:** %d\n\n", len(labels), labelsNoDesc)
	b.WriteString("| Label | Color | Description | Open Issues | Open MRs |\n")
	b.WriteString("|-------|-------|-------------|-------------|----------|\n")
	for _, l := range labels {
		desc := l.Description
		if desc == "" {
			desc = toolutil.EmojiWarning + " _missing_"
		}
		fmt.Fprintf(b, "| %s | %s | %s | %d | %d |\n",
			l.Name, l.Color, desc, l.OpenIssuesCount, l.OpenMergeRequestsCount)
	}
	b.WriteString("\n")
}

// writeMilestonesAudit writes the milestones audit section.
func writeMilestonesAudit(b *strings.Builder, active, closed []*gl.Milestone) {
	b.WriteString("## Milestones\n\n")
	total := len(active) + len(closed)
	if total == 0 {
		b.WriteString(toolutil.EmojiWarning + " **No milestones configured.** Milestones help track release progress.\n\n")
		return
	}
	fmt.Fprintf(b, "**Active:** %d | **Closed:** %d | **Total:** %d\n\n", len(active), len(closed), total)
	if len(active) > 0 {
		b.WriteString("### Active Milestones\n\n")
		b.WriteString("| Milestone | Due Date | Expired |\n|-----------|----------|--------|\n")
		for _, m := range active {
			due := toolutil.EmojiWarning + " _not set_"
			expired := "—"
			if m.DueDate != nil {
				due = m.DueDate.String()
				if m.Expired != nil && *m.Expired {
					expired = toolutil.EmojiWarning + " Yes"
				} else {
					expired = "No"
				}
			}
			fmt.Fprintf(b, "| %s | %s | %s |\n", m.Title, due, expired)
		}
		b.WriteString("\n")
	}
}

// writeTemplatesAudit writes the templates audit section.
func writeTemplatesAudit(b *strings.Builder, issueTPL, mrTPL []*gl.ProjectTemplate) {
	b.WriteString("## Templates\n\n")
	b.WriteString("| Type | Count |\n|------|-------|\n")
	fmt.Fprintf(b, "| Issue templates | %d |\n", len(issueTPL))
	fmt.Fprintf(b, "| MR templates | %d |\n", len(mrTPL))
	b.WriteString("\n")

	if len(issueTPL) == 0 && len(mrTPL) == 0 {
		b.WriteString(toolutil.EmojiWarning + " **No templates found.** Templates ensure consistent issue/MR creation.\n\n")
		return
	}
	if len(issueTPL) > 0 {
		b.WriteString("### Issue Templates\n\n")
		for _, t := range issueTPL {
			fmt.Fprintf(b, "- %s\n", t.Name)
		}
		b.WriteString("\n")
	}
	if len(mrTPL) > 0 {
		b.WriteString("### MR Templates\n\n")
		for _, t := range mrTPL {
			fmt.Fprintf(b, "- %s\n", t.Name)
		}
		b.WriteString("\n")
	}
}

// audit_project_full.

// registerAuditProjectFullPrompt registers the audit_project_full prompt.
func registerAuditProjectFullPrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:  "audit_project_full",
		Title: toolutil.TitleFromName("audit_project_full"),
		Description: "Run a comprehensive audit of a GitLab project covering settings, branch protection, " +
			"access management, labels, milestones, and templates in a single report. " +
			"Use this for a complete project health assessment with actionable recommendations.",
		Icons: toolutil.IconSecurity,
		Arguments: []*mcp.PromptArgument{
			projectIDArg(),
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleAuditProjectFull(ctx, client, req)
	})
}

// handleAuditProjectFull performs the handle audit project full operation using the GitLab API and returns [*mcp.GetPromptResult].
func handleAuditProjectFull(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	projectID := req.Params.Arguments[argProjectID]
	if projectID == "" {
		return nil, fmt.Errorf("audit_project_full: %s is required", argProjectID)
	}

	// Fetch all data in sequence (could be parallelized but keeping simple)
	project, _, err := client.GL().Projects.GetProject(projectID, &gl.GetProjectOptions{
		Statistics: new(true),
	}, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("audit_project_full: failed to get project: %w", err)
	}

	protectedBranches, _, _ := client.GL().ProtectedBranches.ListProtectedBranches(projectID, &gl.ListProtectedBranchesOptions{
		ListOptions: gl.ListOptions{PerPage: maxListItems},
	}, gl.WithContext(ctx))

	members, _, _ := client.GL().ProjectMembers.ListAllProjectMembers(projectID, &gl.ListProjectMembersOptions{
		ListOptions: gl.ListOptions{PerPage: maxListItems},
	}, gl.WithContext(ctx))

	labels, _, _ := client.GL().Labels.ListLabels(projectID, &gl.ListLabelsOptions{
		ListOptions: gl.ListOptions{PerPage: maxListItems},
	}, gl.WithContext(ctx))

	activeMilestones, _, _ := client.GL().Milestones.ListMilestones(projectID, &gl.ListMilestonesOptions{
		State:       new("active"),
		ListOptions: gl.ListOptions{PerPage: maxListItems},
	}, gl.WithContext(ctx))

	issueTemplates, _, _ := client.GL().ProjectTemplates.ListTemplates(projectID, "issues", &gl.ListProjectTemplatesOptions{
		ListOptions: gl.ListOptions{PerPage: maxListItems},
	}, gl.WithContext(ctx))

	mrTemplates, _, _ := client.GL().ProjectTemplates.ListTemplates(projectID, "merge_requests", &gl.ListProjectTemplatesOptions{
		ListOptions: gl.ListOptions{PerPage: maxListItems},
	}, gl.WithContext(ctx))

	pushRule, _, _ := client.GL().Projects.GetProjectPushRules(projectID, gl.WithContext(ctx))

	webhooks, _, _ := client.GL().Projects.ListProjectHooks(projectID, &gl.ListProjectHooksOptions{
		ListOptions: gl.ListOptions{PerPage: maxListItems},
	}, gl.WithContext(ctx))

	var b strings.Builder
	fmt.Fprintf(&b, "# Full Project Audit — %s\n\n", project.PathWithNamespace)

	writeFullScorecard(&b, scorecardData{
		project:    project,
		branches:   protectedBranches,
		pushRule:   pushRule,
		labels:     labels,
		milestones: activeMilestones,
		issueTPL:   issueTemplates,
		mrTPL:      mrTemplates,
		webhooks:   webhooks,
	})
	writeFullSettingsSection(&b, project)
	writeFullBranchSection(&b, protectedBranches)
	writeFullAccessSection(&b, members, project.SharedWithGroups)
	writeFullLabelsSection(&b, labels)
	writeFullMilestonesSection(&b, activeMilestones)
	writeFullTemplatesSection(&b, issueTemplates, mrTemplates)
	writeFullWebhooksSection(&b, webhooks)
	writeFullPushRulesSection(&b, pushRule)

	b.WriteString("---\nPlease provide a comprehensive assessment of this project's configuration health. " +
		"For each section, identify issues ranked by severity (critical/important/suggestion) and provide " +
		"specific, actionable recommendations. Focus on security, compliance, and developer experience.\n")

	return promptResult(b.String()), nil
}

// scorecardData holds all data needed to render the quick scorecard section.
type scorecardData struct {
	project    *gl.Project
	branches   []*gl.ProtectedBranch
	pushRule   *gl.ProjectPushRules
	labels     []*gl.Label
	milestones []*gl.Milestone
	issueTPL   []*gl.ProjectTemplate
	mrTPL      []*gl.ProjectTemplate
	webhooks   []*gl.ProjectHook
}

// writeFullScorecard writes the quick scorecard section of the full audit.
func writeFullScorecard(b *strings.Builder, s scorecardData) {
	b.WriteString("## Quick Scorecard\n\n")
	b.WriteString("| Area | Status |\n|------|--------|\n")
	fmt.Fprintf(b, "| Default branch protected | %s |\n", toolutil.BoolEmoji(isDefaultBranchProtected(s.branches, s.project.DefaultBranch)))
	fmt.Fprintf(b, "| Pipeline required for merge | %s |\n", toolutil.BoolEmoji(s.project.OnlyAllowMergeIfPipelineSucceeds))
	fmt.Fprintf(b, "| Discussions must be resolved | %s |\n", toolutil.BoolEmoji(s.project.OnlyAllowMergeIfAllDiscussionsAreResolved))
	hasPushRules := s.pushRule != nil && (s.pushRule.CommitMessageRegex != "" || s.pushRule.PreventSecrets || s.pushRule.MemberCheck)
	fmt.Fprintf(b, "| Push rules configured | %s |\n", toolutil.BoolEmoji(hasPushRules))
	fmt.Fprintf(b, "| Labels configured | %s |\n", toolutil.BoolEmoji(len(s.labels) > 0))
	fmt.Fprintf(b, "| Active milestones | %s |\n", toolutil.BoolEmoji(len(s.milestones) > 0))
	fmt.Fprintf(b, "| Issue templates | %s |\n", toolutil.BoolEmoji(len(s.issueTPL) > 0))
	fmt.Fprintf(b, "| MR templates | %s |\n", toolutil.BoolEmoji(len(s.mrTPL) > 0))
	fmt.Fprintf(b, "| Webhooks configured | %s |\n", toolutil.BoolEmoji(len(s.webhooks) > 0))
	b.WriteString("\n")
}

// writeFullSettingsSection writes the project settings section of the full audit.
func writeFullSettingsSection(b *strings.Builder, project *gl.Project) {
	b.WriteString("## 1. Project Settings\n\n")
	b.WriteString(settingValueTableHeader)
	fmt.Fprintf(b, "| Visibility | %s |\n", string(project.Visibility))
	fmt.Fprintf(b, "| Default branch | %s |\n", emptyDash(project.DefaultBranch))
	fmt.Fprintf(b, "| Merge method | %s |\n", emptyDash(string(project.MergeMethod)))
	fmt.Fprintf(b, "| Squash option | %s |\n", emptyDash(string(project.SquashOption)))
	fmt.Fprintf(b, "| Pipeline required | %s |\n", toolutil.BoolEmoji(project.OnlyAllowMergeIfPipelineSucceeds))
	fmt.Fprintf(b, "| All discussions resolved | %s |\n", toolutil.BoolEmoji(project.OnlyAllowMergeIfAllDiscussionsAreResolved))
	fmt.Fprintf(b, "| Remove source branch | %s |\n", toolutil.BoolEmoji(project.RemoveSourceBranchAfterMerge))
	fmt.Fprintf(b, "| CI config path | %s |\n", emptyDash(project.CIConfigPath))
	b.WriteString("\n")
}

// writeFullBranchSection writes the branch protection section of the full audit.
func writeFullBranchSection(b *strings.Builder, branches []*gl.ProtectedBranch) {
	b.WriteString("## 2. Branch Protection\n\n")
	fmt.Fprintf(b, "**Protected branches:** %d\n\n", len(branches))
	if len(branches) == 0 {
		return
	}
	b.WriteString("| Branch | Push Access | Merge Access | Force Push | Code Owners |\n")
	b.WriteString("|--------|-----------|-------------|-----------|------------|\n")
	for _, pb := range branches {
		push := formatAccessLevels(pb.PushAccessLevels)
		merge := formatAccessLevels(pb.MergeAccessLevels)
		fmt.Fprintf(b, "| %s | %s | %s | %s | %s |\n",
			pb.Name, push, merge, toolutil.BoolEmoji(pb.AllowForcePush), toolutil.BoolEmoji(pb.CodeOwnerApprovalRequired))
	}
	b.WriteString("\n")
}

// writeFullAccessSection writes the access & members section of the full audit.
func writeFullAccessSection(b *strings.Builder, members []*gl.ProjectMember, groups []gl.ProjectSharedWithGroup) {
	b.WriteString("## 3. Access & Members\n\n")
	fmt.Fprintf(b, "**Total members:** %d\n\n", len(members))
	if len(members) > 0 {
		accessCounts := make(map[string]int)
		for _, m := range members {
			accessCounts[accessLevelName(m.AccessLevel)]++
		}
		b.WriteString("| Access Level | Count |\n|-------------|-------|\n")
		for _, level := range []string{"Owner", "Maintainer", "Developer", "Reporter", "Guest"} {
			if c, ok := accessCounts[level]; ok {
				fmt.Fprintf(b, "| %s | %d |\n", level, c)
			}
		}
		b.WriteString("\n")
	}
	if len(groups) > 0 {
		fmt.Fprintf(b, "**Shared with %d group(s):** ", len(groups))
		groupNames := make([]string, 0, len(groups))
		for _, sg := range groups {
			groupNames = append(groupNames, fmt.Sprintf("%s (%s)", sg.GroupName, accessLevelName(gl.AccessLevelValue(sg.GroupAccessLevel))))
		}
		b.WriteString(strings.Join(groupNames, ", ") + "\n\n")
	}
}

// writeFullLabelsSection writes the labels section of the full audit.
func writeFullLabelsSection(b *strings.Builder, labels []*gl.Label) {
	b.WriteString("## 4. Labels\n\n")
	fmt.Fprintf(b, "**Total:** %d\n", len(labels))
	if len(labels) > 0 {
		noDesc := 0
		for _, l := range labels {
			if l.Description == "" {
				noDesc++
			}
		}
		if noDesc > 0 {
			fmt.Fprintf(b, toolutil.EmojiWarning+" **%d label(s) without description**\n", noDesc)
		}
	}
	b.WriteString("\n")
}

// writeFullMilestonesSection writes the milestones section of the full audit.
func writeFullMilestonesSection(b *strings.Builder, active []*gl.Milestone) {
	b.WriteString("## 5. Milestones\n\n")
	fmt.Fprintf(b, "**Active:** %d\n", len(active))
	for _, m := range active {
		due := "no due date"
		if m.DueDate != nil {
			due = "due " + m.DueDate.String()
		}
		fmt.Fprintf(b, "- %s (%s)\n", m.Title, due)
	}
	b.WriteString("\n")
}

// writeFullTemplatesSection writes the templates section of the full audit.
func writeFullTemplatesSection(b *strings.Builder, issueTPL, mrTPL []*gl.ProjectTemplate) {
	b.WriteString("## 6. Templates\n\n")
	fmt.Fprintf(b, "**Issue templates:** %d | **MR templates:** %d\n\n", len(issueTPL), len(mrTPL))
}

// writeFullWebhooksSection writes the webhooks section of the full audit.
func writeFullWebhooksSection(b *strings.Builder, webhooks []*gl.ProjectHook) {
	b.WriteString("## 7. Webhooks\n\n")
	fmt.Fprintf(b, "**Configured:** %d\n", len(webhooks))
	if len(webhooks) > 0 {
		b.WriteString("\n| URL | Push | MR | Issues | SSL |\n|-----|------|-----|--------|-----|\n")
		for _, h := range webhooks {
			fmt.Fprintf(b, "| %s | %s | %s | %s | %s |\n",
				maskURL(h.URL), toolutil.BoolEmoji(h.PushEvents), toolutil.BoolEmoji(h.MergeRequestsEvents),
				toolutil.BoolEmoji(h.IssuesEvents), toolutil.BoolEmoji(h.EnableSSLVerification))
		}
	}
	b.WriteString("\n")
}

// writeFullPushRulesSection writes the push rules section of the full audit.
func writeFullPushRulesSection(b *strings.Builder, pushRule *gl.ProjectPushRules) {
	b.WriteString("## 8. Push Rules\n\n")
	if pushRule == nil {
		b.WriteString("Push rules not configured (may require GitLab Premium).\n\n")
		return
	}
	b.WriteString("| Rule | Value |\n|------|-------|\n")
	fmt.Fprintf(b, "| Prevent secrets | %s |\n", toolutil.BoolEmoji(pushRule.PreventSecrets))
	fmt.Fprintf(b, "| Member check | %s |\n", toolutil.BoolEmoji(pushRule.MemberCheck))
	fmt.Fprintf(b, "| Commit message regex | %s |\n", emptyDash(pushRule.CommitMessageRegex))
	fmt.Fprintf(b, "| Branch name regex | %s |\n", emptyDash(pushRule.BranchNameRegex))
	fmt.Fprintf(b, "| Author email regex | %s |\n", emptyDash(pushRule.AuthorEmailRegex))
	b.WriteString("\n")
}

// formatAccessLevels formats a slice of BranchAccessDescription for compact display.
func formatAccessLevels(levels []*gl.BranchAccessDescription) string {
	if len(levels) == 0 {
		return "—"
	}
	var parts []string
	for _, al := range levels {
		parts = append(parts, accessLevelName(al.AccessLevel))
	}
	return strings.Join(parts, ", ")
}

// emptyDash returns "—" for empty strings.
func emptyDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// accessLevelIcon is an internal helper for the prompts package.
func accessLevelIcon(v gl.AccessControlValue) string {
	if v != "" && v != gl.DisabledAccessControl {
		return toolutil.EmojiSuccess
	}
	return toolutil.EmojiCross
}

// formatTimePtr formats a time pointer, returning "—" if nil.
func formatTimePtr(t *time.Time) string {
	if t == nil {
		return "—"
	}
	return t.Format("2006-01-02")
}

// formatBytes converts bytes to a human-readable string.
func formatBytes(bytes int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// maskURL masks the path portion of a webhook URL for security.
func maskURL(u string) string {
	if len(u) <= 30 {
		return u
	}
	return u[:30] + "..."
}
