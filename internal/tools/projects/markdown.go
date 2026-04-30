// markdown.go provides Markdown formatting functions for project MCP tool output.
package projects

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatMarkdown renders a single project as a Markdown summary.
func FormatMarkdown(p Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Project: %s\n\n", toolutil.EscapeMdHeading(p.Name))
	fmt.Fprintf(&b, toolutil.FmtMdID, p.ID)
	fmt.Fprintf(&b, toolutil.FmtMdPath, p.PathWithNamespace)
	fmt.Fprintf(&b, toolutil.FmtMdVisibility, p.Visibility)
	fmt.Fprintf(&b, "- **Default Branch**: %s\n", p.DefaultBranch)
	if p.Description != "" {
		fmt.Fprintf(&b, toolutil.FmtMdDescription, p.Description)
	}
	if p.Namespace != "" {
		fmt.Fprintf(&b, "- **Namespace**: %s\n", p.Namespace)
	}
	if p.Archived {
		fmt.Fprintf(&b, "- %s **Archived**\n", toolutil.EmojiArchived)
	}
	if p.ForksCount > 0 {
		fmt.Fprintf(&b, "- **Forks**: %d\n", p.ForksCount)
	}
	if p.StarCount > 0 {
		fmt.Fprintf(&b, "- %s **Stars**: %d\n", toolutil.EmojiStar, p.StarCount)
	}
	if p.OpenIssuesCount > 0 {
		fmt.Fprintf(&b, "- **Open Issues**: %d\n", p.OpenIssuesCount)
	}
	if len(p.Topics) > 0 {
		fmt.Fprintf(&b, "- **Topics**: %s\n", strings.Join(p.Topics, ", "))
	}
	if p.CreatedAt != "" {
		fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(p.CreatedAt))
	}
	fmt.Fprintf(&b, toolutil.FmtMdURL, p.WebURL)
	if p.HTTPURLToRepo != "" {
		fmt.Fprintf(&b, "- **HTTP Clone**: %s\n", p.HTTPURLToRepo)
	}
	if p.SSHURLToRepo != "" {
		fmt.Fprintf(&b, "- **SSH Clone**: %s\n", p.SSHURLToRepo)
	}
	if p.MergeRequestTitleRegex != "" {
		fmt.Fprintf(&b, "- **MR Title Regex**: `%s`\n", p.MergeRequestTitleRegex)
		if p.MergeRequestTitleRegexDescription != "" {
			fmt.Fprintf(&b, "- **MR Title Regex Description**: %s\n", p.MergeRequestTitleRegexDescription)
		}
	}
	toolutil.WriteHints(&b,
		"Use gitlab_branch action 'list' to see branches",
		"Use gitlab_merge_request action 'list' to see open merge requests",
		"Use gitlab_issue action 'list' to see open issues",
		"Use gitlab_pipeline action 'list' to see CI/CD pipelines",
		"Use gitlab_project_get to get project details",
	)
	return b.String()
}

// FormatDeleteMarkdown renders a project deletion result as a Markdown summary.
func FormatDeleteMarkdown(out DeleteOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Project Deletion\n\n")
	fmt.Fprintf(&b, toolutil.FmtMdStatus, out.Status)
	fmt.Fprintf(&b, "- **Message**: %s\n", out.Message)
	if out.MarkedForDeletionOn != "" {
		fmt.Fprintf(&b, "- **Marked for deletion on**: %s\n", out.MarkedForDeletionOn)
	}
	if out.PermanentlyRemoved {
		b.WriteString("- **Permanently removed**: yes\n")
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_project_list` to verify deletion",
	)
	return b.String()
}

// FormatListMarkdown renders a list of projects as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Projects (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Projects), out.Pagination)
	if len(out.Projects) == 0 {
		b.WriteString("No projects found.\n")
		return b.String()
	}
	b.WriteString("| ID | Name | Path | Visibility | " + toolutil.EmojiStar + " |\n")
	b.WriteString(toolutil.TblSep5Col)
	for _, p := range out.Projects {
		archived := ""
		if p.Archived {
			archived = " " + toolutil.EmojiArchived
		}
		fmt.Fprintf(&b, "| %d | [%s](%s)%s | %s | %s | %d |\n", p.ID, toolutil.EscapeMdTableCell(p.Name), p.WebURL, archived, toolutil.EscapeMdTableCell(p.PathWithNamespace), p.Visibility, p.StarCount)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use action 'get' with a project_id to see full project details",
		"Use action 'create' to create a new project",
	)
	return b.String()
}

// FormatListForksMarkdown renders a list of project forks as Markdown.
func FormatListForksMarkdown(out ListForksOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Project Forks (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Forks), out.Pagination)
	if len(out.Forks) == 0 {
		b.WriteString("No forks found.\n")
		return b.String()
	}
	b.WriteString("| ID | Name | Path | Visibility | " + toolutil.EmojiStar + " |\n")
	b.WriteString(toolutil.TblSep5Col)
	for _, p := range out.Forks {
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %d |\n", p.ID, toolutil.EscapeMdTableCell(p.Name), toolutil.EscapeMdTableCell(p.PathWithNamespace), p.Visibility, p.StarCount)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		"Use `gitlab_project_get` to view fork details",
		"Use `gitlab_project_fork` to create a new fork",
	)
	return b.String()
}

// FormatLanguagesMarkdown renders project languages as Markdown.
func FormatLanguagesMarkdown(out LanguagesOutput) string {
	var b strings.Builder
	b.WriteString("## Project Languages\n\n")
	if len(out.Languages) == 0 {
		b.WriteString("No languages detected.\n")
		return b.String()
	}
	b.WriteString("| Language | % |\n")
	b.WriteString(toolutil.TblSep2Col)
	for _, l := range out.Languages {
		fmt.Fprintf(&b, "| %s | %.1f%% |\n", toolutil.EscapeMdTableCell(l.Name), l.Percentage)
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_repository_tree` to browse the codebase",
	)
	return b.String()
}

// FormatListHooksMarkdown renders a list of project webhooks as Markdown.
func FormatListHooksMarkdown(out ListHooksOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Project Webhooks (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Hooks), out.Pagination)
	if len(out.Hooks) == 0 {
		b.WriteString("No webhooks found.\n")
		return b.String()
	}
	b.WriteString("| ID | Name | URL | Push | MR | Issues | Pipeline | SSL |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, h := range out.Hooks {
		name := h.Name
		if name == "" {
			name = "-"
		}
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %s | %s | %s | %s |\n",
			h.ID, toolutil.EscapeMdTableCell(name), toolutil.EscapeMdTableCell(h.URL),
			boolIcon(h.PushEvents), boolIcon(h.MergeRequestsEvents),
			boolIcon(h.IssuesEvents), boolIcon(h.PipelineEvents),
			boolIcon(h.EnableSSLVerification))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		"Use `gitlab_project_hook_get` to view a webhook's details",
		"Use `gitlab_project_hook_add` to add a new webhook",
	)
	return b.String()
}

// FormatHookMarkdown renders a single project webhook as Markdown.
func FormatHookMarkdown(out HookOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Webhook #%d\n\n", out.ID)
	if out.Name != "" {
		fmt.Fprintf(&b, "**Name:** %s\n", out.Name)
	}
	fmt.Fprintf(&b, "**URL:** %s\n", out.URL)
	fmt.Fprintf(&b, "**SSL Verification:** %s\n\n", boolIcon(out.EnableSSLVerification))
	b.WriteString("### Event Triggers\n\n")
	b.WriteString("| Event | Enabled |\n")
	b.WriteString(toolutil.TblSep2Col)
	events := []struct {
		name string
		on   bool
	}{
		{"Push", out.PushEvents},
		{"Issues", out.IssuesEvents},
		{"Confidential Issues", out.ConfidentialIssuesEvents},
		{"Merge Requests", out.MergeRequestsEvents},
		{"Tag Push", out.TagPushEvents},
		{"Note", out.NoteEvents},
		{"Confidential Note", out.ConfidentialNoteEvents},
		{"Job", out.JobEvents},
		{"Pipeline", out.PipelineEvents},
		{"Wiki Page", out.WikiPageEvents},
		{"Deployment", out.DeploymentEvents},
		{"Releases", out.ReleasesEvents},
		{"Milestone", out.MilestoneEvents},
		{"Feature Flag", out.FeatureFlagEvents},
		{"Emoji", out.EmojiEvents},
		{"Repository Update", out.RepositoryUpdateEvents},
		{"Resource Access Token", out.ResourceAccessTokenEvents},
	}
	for _, ev := range events {
		fmt.Fprintf(&b, "| %s | %s |\n", ev.name, boolIcon(ev.on))
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_project_hook_edit` to modify event triggers",
		"Use `gitlab_project_hook_test` to test the webhook",
	)
	return b.String()
}

// FormatListProjectUsersMarkdown renders a users list as markdown.
func FormatListProjectUsersMarkdown(out ListProjectUsersOutput) string {
	if len(out.Users) == 0 {
		return "No users found.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Project Users (%d)\n\n", len(out.Users))
	toolutil.WriteListSummary(&b, len(out.Users), out.Pagination)
	b.WriteString("| ID | Name | Username | State |\n")
	b.WriteString("|---|---|---|---|\n")
	for _, u := range out.Users {
		fmt.Fprintf(&b, "| %d | %s | @%s | %s |\n", u.ID, u.Name, u.Username, u.State)
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_project_member_add` to add a new member",
		"Use `gitlab_project_share_with_group` to share with a group",
	)
	return b.String()
}

// FormatListProjectGroupsMarkdown renders a project groups list as markdown.
func FormatListProjectGroupsMarkdown(out ListProjectGroupsOutput) string {
	if len(out.Groups) == 0 {
		return "No groups found.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Project Groups (%d)\n\n", len(out.Groups))
	toolutil.WriteListSummary(&b, len(out.Groups), out.Pagination)
	b.WriteString("| ID | Name | Full Path |\n")
	b.WriteString("|---|---|---|\n")
	for _, g := range out.Groups {
		fmt.Fprintf(&b, "| %d | %s | %s |\n", g.ID, g.Name, g.FullPath)
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_group_get` to view group details",
	)
	return b.String()
}

// FormatListStarrersMarkdown renders a starrers list as markdown.
func FormatListStarrersMarkdown(out ListProjectStarrersOutput) string {
	if len(out.Starrers) == 0 {
		return "No starrers found.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Project Starrers (%d)\n\n", len(out.Starrers))
	toolutil.WriteListSummary(&b, len(out.Starrers), out.Pagination)
	b.WriteString("| User | Username | Starred Since |\n")
	b.WriteString("|---|---|---|\n")
	for _, s := range out.Starrers {
		fmt.Fprintf(&b, "| %s | @%s | %s |\n", s.User.Name, s.User.Username, toolutil.FormatTime(s.StarredSince))
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_project_get` to view full project details",
	)
	return b.String()
}

// FormatShareProjectMarkdown renders a share-project result as markdown.
func FormatShareProjectMarkdown(out ShareProjectOutput) string {
	var b strings.Builder
	b.WriteString("## Project Shared\n\n")
	fmt.Fprintf(&b, "%s\n", out.Message)
	if out.GroupID != 0 {
		fmt.Fprintf(&b, "\n| Field | Value |\n")
		b.WriteString("|---|---|\n")
		fmt.Fprintf(&b, "| Group ID | %d |\n", out.GroupID)
		fmt.Fprintf(&b, "| Access Role | %s |\n", out.AccessRole)
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_project_list_groups` to verify the share",
		"Use `gitlab_project_delete_shared_group` to revoke access",
	)
	return b.String()
}

// FormatPushRuleMarkdown renders a push rule as markdown.
func FormatPushRuleMarkdown(out PushRuleOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Push Rule (ID: %d)\n\n", out.ID)
	fmt.Fprintf(&b, "**Project ID:** %d\n\n", out.ProjectID)

	b.WriteString("| Rule | Value |\n")
	b.WriteString("|---|---|\n")

	type rule struct {
		name string
		val  string
	}
	rules := []rule{
		{"Commit message regex", out.CommitMessageRegex},
		{"Commit message negative regex", out.CommitMessageNegativeRegex},
		{"Branch name regex", out.BranchNameRegex},
		{"Author email regex", out.AuthorEmailRegex},
		{"File name regex", out.FileNameRegex},
		{"Max file size (MB)", strconv.FormatInt(out.MaxFileSize, 10)},
		{"Deny delete tag", boolIcon(out.DenyDeleteTag)},
		{"Member check", boolIcon(out.MemberCheck)},
		{"Prevent secrets", boolIcon(out.PreventSecrets)},
		{"Commit committer check", boolIcon(out.CommitCommitterCheck)},
		{"Commit committer name check", boolIcon(out.CommitCommitterNameCheck)},
		{"Reject unsigned commits", boolIcon(out.RejectUnsignedCommits)},
		{"Reject non-DCO commits", boolIcon(out.RejectNonDCOCommits)},
	}
	for _, r := range rules {
		val := r.val
		if val == "" {
			val = "—"
		}
		fmt.Fprintf(&b, "| %s | %s |\n", r.name, val)
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_project_edit_push_rule` to modify push rules",
		"Use `gitlab_project_delete_push_rule` to remove push rules",
	)
	return b.String()
}

// FormatForkRelationMarkdown renders a fork relation as Markdown.
func FormatForkRelationMarkdown(out ForkRelationOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Fork Relation (ID: %d)\n\n", out.ID)
	fmt.Fprintf(&b, "- **Forked To Project ID**: %d\n", out.ForkedToProjectID)
	fmt.Fprintf(&b, "- **Forked From Project ID**: %d\n", out.ForkedFromProjectID)
	if out.CreatedAt != "" {
		fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(out.CreatedAt))
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_project_delete_fork_relation` to remove the fork relation",
	)
	return b.String()
}

// FormatDownloadAvatarMarkdown renders an avatar download result as Markdown.
func FormatDownloadAvatarMarkdown(out DownloadAvatarOutput) string {
	var b strings.Builder
	b.WriteString("## Project Avatar\n\n")
	fmt.Fprintf(&b, "- **Size**: %d bytes\n", out.SizeBytes)
	fmt.Fprintf(&b, "- **Content**: base64-encoded (%d chars)\n", len(out.ContentBase64))
	toolutil.WriteHints(&b,
		"Use `gitlab_project_upload_avatar` to replace the avatar",
	)
	return b.String()
}

// FormatApprovalConfigMarkdown renders approval configuration as Markdown.
func FormatApprovalConfigMarkdown(out ApprovalConfigOutput) string {
	var b strings.Builder
	b.WriteString("## Approval Configuration\n\n")
	b.WriteString("| Setting | Value |\n")
	b.WriteString(toolutil.TblSep2Col)
	fmt.Fprintf(&b, "| Approvals before merge | %d |\n", out.ApprovalsBeforeMerge)
	fmt.Fprintf(&b, "| Reset approvals on push | %s |\n", boolIcon(out.ResetApprovalsOnPush))
	fmt.Fprintf(&b, "| Disable overriding approvers per MR | %s |\n", boolIcon(out.DisableOverridingApproversPerMergeRequest))
	fmt.Fprintf(&b, "| Author self-approval | %s |\n", boolIcon(out.MergeRequestsAuthorApproval))
	fmt.Fprintf(&b, "| Disable committers approval | %s |\n", boolIcon(out.MergeRequestsDisableCommittersApproval))
	fmt.Fprintf(&b, "| Require reauthentication to approve | %s |\n", boolIcon(out.RequireReauthenticationToApprove))
	fmt.Fprintf(&b, "| Selective code owner removals | %s |\n", boolIcon(out.SelectiveCodeOwnerRemovals))
	toolutil.WriteHints(&b,
		"Use `gitlab_project_approval_config_change` to modify settings",
		"Use `gitlab_project_approval_rule_list` to see approval rules",
	)
	return b.String()
}

// FormatApprovalRuleMarkdown renders a single approval rule as Markdown.
func FormatApprovalRuleMarkdown(out ApprovalRuleOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Approval Rule: %s\n\n", toolutil.EscapeMdHeading(out.Name))
	fmt.Fprintf(&b, toolutil.FmtMdID, out.ID)
	fmt.Fprintf(&b, "- **Approvals Required**: %d\n", out.ApprovalsRequired)
	if out.RuleType != "" {
		fmt.Fprintf(&b, "- **Rule Type**: %s\n", out.RuleType)
	}
	if out.ReportType != "" {
		fmt.Fprintf(&b, "- **Report Type**: %s\n", out.ReportType)
	}
	fmt.Fprintf(&b, "- **Applies to all protected branches**: %s\n", boolIcon(out.AppliesToAllProtectedBranches))
	fmt.Fprintf(&b, "- **Contains hidden groups**: %s\n", boolIcon(out.ContainsHiddenGroups))
	if len(out.Users) > 0 {
		fmt.Fprintf(&b, "- **Users**: %s\n", strings.Join(out.Users, ", "))
	}
	if len(out.Groups) > 0 {
		fmt.Fprintf(&b, "- **Groups**: %s\n", strings.Join(out.Groups, ", "))
	}
	if len(out.EligibleApprovers) > 0 {
		fmt.Fprintf(&b, "- **Eligible Approvers**: %s\n", strings.Join(out.EligibleApprovers, ", "))
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_project_approval_rule_update` to modify this rule",
		"Use `gitlab_project_approval_rule_delete` to remove this rule",
	)
	return b.String()
}

// FormatListApprovalRulesMarkdown renders a list of approval rules as Markdown.
func FormatListApprovalRulesMarkdown(out ListApprovalRulesOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Approval Rules (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Rules), out.Pagination)
	if len(out.Rules) == 0 {
		b.WriteString("No approval rules found.\n")
		return b.String()
	}
	b.WriteString("| ID | Name | Type | Approvals | All Protected | Users | Groups |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, r := range out.Rules {
		ruleType := r.RuleType
		if ruleType == "" {
			ruleType = "—"
		}
		users := "—"
		if len(r.Users) > 0 {
			users = strings.Join(r.Users, ", ")
		}
		groups := "—"
		if len(r.Groups) > 0 {
			groups = strings.Join(r.Groups, ", ")
		}
		fmt.Fprintf(&b, "| %d | %s | %s | %d | %s | %s | %s |\n",
			r.ID, toolutil.EscapeMdTableCell(r.Name), ruleType,
			r.ApprovalsRequired, boolIcon(r.AppliesToAllProtectedBranches),
			toolutil.EscapeMdTableCell(users), toolutil.EscapeMdTableCell(groups))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use `gitlab_project_approval_rule_get` to see rule details",
		"Use `gitlab_project_approval_rule_create` to add a new rule",
	)
	return b.String()
}

// FormatPullMirrorMarkdown renders pull mirror details as Markdown.
func FormatPullMirrorMarkdown(out PullMirrorOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Pull Mirror (ID: %d)\n\n", out.ID)
	fmt.Fprintf(&b, "- **Enabled**: %s\n", boolIcon(out.Enabled))
	if out.URL != "" {
		fmt.Fprintf(&b, "- **URL**: %s\n", out.URL)
	}
	if out.UpdateStatus != "" {
		fmt.Fprintf(&b, "- **Update Status**: %s\n", out.UpdateStatus)
	}
	if out.LastError != "" {
		fmt.Fprintf(&b, "- **Last Error**: %s\n", out.LastError)
	}
	if out.LastSuccessfulUpdateAt != "" {
		fmt.Fprintf(&b, "- **Last Successful Update**: %s\n", toolutil.FormatTime(out.LastSuccessfulUpdateAt))
	}
	if out.LastUpdateAt != "" {
		fmt.Fprintf(&b, "- **Last Update**: %s\n", toolutil.FormatTime(out.LastUpdateAt))
	}
	fmt.Fprintf(&b, "- **Trigger Builds**: %s\n", boolIcon(out.MirrorTriggerBuilds))
	fmt.Fprintf(&b, "- **Only Protected Branches**: %s\n", boolIcon(out.OnlyMirrorProtectedBranches))
	fmt.Fprintf(&b, "- **Overwrite Diverged Branches**: %s\n", boolIcon(out.MirrorOverwritesDivergedBranches))
	if out.MirrorBranchRegex != "" {
		fmt.Fprintf(&b, "- **Branch Regex**: `%s`\n", out.MirrorBranchRegex)
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_project_pull_mirror_configure` to modify mirror settings",
		"Use `gitlab_project_start_mirroring` to trigger an immediate update",
	)
	return b.String()
}

// FormatRepositoryStorageMarkdown renders repository storage info as Markdown.
func FormatRepositoryStorageMarkdown(out RepositoryStorageOutput) string {
	var b strings.Builder
	b.WriteString("## Repository Storage\n\n")
	fmt.Fprintf(&b, "- **Project ID**: %d\n", out.ProjectID)
	fmt.Fprintf(&b, "- **Disk Path**: %s\n", out.DiskPath)
	fmt.Fprintf(&b, "- **Repository Storage**: %s\n", out.RepositoryStorage)
	if out.CreatedAt != "" {
		fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(out.CreatedAt))
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_project_start_housekeeping` to optimize the repository",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatDeleteMarkdown)
	toolutil.RegisterMarkdown(FormatListForksMarkdown)
	toolutil.RegisterMarkdown(FormatLanguagesMarkdown)
	toolutil.RegisterMarkdown(FormatListHooksMarkdown)
	toolutil.RegisterMarkdown(FormatHookMarkdown)
	toolutil.RegisterMarkdown(FormatListProjectUsersMarkdown)
	toolutil.RegisterMarkdown(FormatListProjectGroupsMarkdown)
	toolutil.RegisterMarkdown(FormatListStarrersMarkdown)
	toolutil.RegisterMarkdown(FormatPushRuleMarkdown)
	toolutil.RegisterMarkdown(FormatForkRelationMarkdown)
	toolutil.RegisterMarkdown(FormatDownloadAvatarMarkdown)
	toolutil.RegisterMarkdown(FormatApprovalConfigMarkdown)
	toolutil.RegisterMarkdown(FormatApprovalRuleMarkdown)
	toolutil.RegisterMarkdown(FormatListApprovalRulesMarkdown)
	toolutil.RegisterMarkdown(FormatPullMirrorMarkdown)
	toolutil.RegisterMarkdown(FormatRepositoryStorageMarkdown)
}
