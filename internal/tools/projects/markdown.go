package projects

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

type projectNotFoundOutput struct {
	Identifier string
}

func formatProjectNotFound(out projectNotFoundOutput) *mcp.CallToolResult {
	return toolutil.NotFoundResult(
		"Project", out.Identifier,
		"Use gitlab_project_list to search for projects by name or path",
		"Verify the project ID or URL-encoded path is correct (e.g. 'group%2Fproject')",
		"The project may have been deleted or you may lack access",
	)
}

// FormatMarkdown renders a single project as a card: one row per field GitLab
// sent, the description as prose under its label, and the hints last.
func FormatMarkdown(p Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Project: "+p.Name)
	c.Int("ID", p.ID)
	c.Field("Path", p.PathWithNamespace)
	c.Field("Visibility", p.Visibility)
	c.Field("Default Branch", p.DefaultBranch)
	c.Text("Description", p.Description)
	if p.Namespace != nil {
		c.Field("Namespace", p.Namespace.FullPath)
	}
	if p.ForkedFromProject != nil {
		c.Field("Forked From", p.ForkedFromProject.PathWithNamespace)
	}
	c.Flag(toolutil.EmojiArchived, "Archived", p.Archived)
	// An empty repository is why a branch, file or pipeline action against this
	// project answers with nothing, so the card says so rather than leaving the
	// reader to infer it.
	c.Flag(toolutil.EmojiInfo, "Empty Repository", p.EmptyRepo)
	// GitLab schedules a deletion rather than performing it, and answers with
	// the date on both the current key and the one it used to send.
	c.Time("Marked for Deletion", markedForDeletion(p))
	c.Count("Forks", p.ForksCount)
	c.Count("Stars", p.StarCount)
	c.Count("Open Issues", p.OpenIssuesCount)
	if len(p.Topics) > 0 {
		c.Field("Topics", strings.Join(p.Topics, ", "))
	}
	c.Time("Created", p.CreatedAt)
	c.URL(p.WebURL)
	c.Code("HTTP Clone", p.HTTPURLToRepo)
	c.Code("SSH Clone", p.SSHURLToRepo)
	// A title regex is a pattern a maintainer typed, where '|' is ordinary
	// alternation: inside a code span it reaches the reader as GitLab holds it.
	c.Code("MR Title Regex", p.MergeRequestTitleRegex)
	if p.MergeRequestTitleRegex != "" {
		c.Field("MR Title Regex Description", p.MergeRequestTitleRegexDescription)
	}
	c.BoolPtr("Protected MR Pipelines", p.ProtectMergeRequestPipelines)
	c.End(
		toolutil.HintAction("branch.list", "see this project's branches"),
		toolutil.HintAction("merge_request.list", "see its open merge requests"),
		toolutil.HintAction("issue.list", "see its open issues"),
		toolutil.HintAction("pipeline.list", "see its CI/CD pipelines"),
		toolutil.HintAction(actionProjectUpdate, "change this project's settings"),
	)
	return b.String()
}

// markedForDeletion is the deletion date GitLab sent, whichever of the two keys
// it used: marked_for_deletion_on is the current spelling and
// marked_for_deletion_at the one the older entity sends.
func markedForDeletion(p Output) string {
	if p.MarkedForDeletionOn != "" {
		return p.MarkedForDeletionOn
	}
	return p.MarkedForDeletionAt
}

// FormatDeleteMarkdown renders a project deletion result as a card: GitLab
// schedules the deletion and answers with when it will happen, so the result is
// an object rather than a one-line confirmation.
func FormatDeleteMarkdown(out DeleteOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Project Deletion")
	c.Field("Status", out.Status)
	// The message is server-authored but interpolates the caller's own
	// project_id, which nothing validates before it lands here.
	c.Field("Message", out.Message)
	c.Time("Marked for Deletion On", out.MarkedForDeletionOn)
	c.Warn("Permanently Removed", out.PermanentlyRemoved)
	c.End(toolutil.HintAction(actionProjectList, "verify the deletion"))
	return b.String()
}

// FormatListMarkdown renders a list of projects as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	return formatProjectTable("Projects", "projects", out.Projects, out.SimpleProjects, out.Pagination,
		toolutil.HintAction(actionProjectGet, "see one project in full"),
		toolutil.HintAction("project.create", "create a new project"),
	)
}

// formatProjectTable renders a page of projects in whichever of the two
// entities GitLab sent: full rows, or the BasicProjectDetails rows a caller
// gets by passing simple. The project list and the fork list are the same
// table and differ only in their title, empty line and hints.
func formatProjectTable(title, resource string, full []Output, basic []BasicOutput, pagination toolutil.PaginationOutput, hints ...string) string {
	rows := len(full) + len(basic)
	if rows == 0 {
		return toolutil.EmptyMessage(resource)
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, title, rows, pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Path", "Visibility", toolutil.EmojiStar))
	for _, p := range full {
		writeProjectListRow(&b, p.BasicOutput, p.Archived)
	}
	// A simple row is BasicProjectDetails, which does not say whether the
	// project is archived, so no row of it claims either answer.
	for _, p := range basic {
		writeProjectListRow(&b, p, false)
	}
	toolutil.WriteListFooter(&b, pagination, projectRowsLink(full, basic), hints...)
	return b.String()
}

// projectRowsLink reports whether any row of this page carries a link, which is
// what decides whether the footer tells the model to preserve them: an
// instruction about links a table has none of is noise it has to read past.
func projectRowsLink(full []Output, basic []BasicOutput) bool {
	for _, p := range full {
		if p.WebURL != "" {
			return true
		}
	}
	for _, p := range basic {
		if p.WebURL != "" {
			return true
		}
	}
	return false
}

// writeProjectListRow writes one row of a project list table. It takes the
// basic entity because that is what every row of both list shapes carries;
// archived is passed apart since only a full row knows it.
func writeProjectListRow(b *strings.Builder, p BasicOutput, archived bool) {
	mark := ""
	if archived {
		mark = " " + toolutil.EmojiArchived
	}
	b.WriteString(toolutil.MarkdownTableRow(
		strconv.FormatInt(p.ID, 10),
		toolutil.MdTitleLink(p.Name, p.WebURL)+mark,
		toolutil.EscapeMdTableCell(p.PathWithNamespace),
		toolutil.EscapeMdTableCell(p.Visibility),
		strconv.FormatInt(p.StarCount, 10),
	))
}

// FormatListForksMarkdown renders a list of project forks as Markdown.
func FormatListForksMarkdown(out ListForksOutput) string {
	return formatProjectTable("Project Forks", "forks", out.Forks, out.SimpleForks, out.Pagination,
		toolutil.HintAction(actionProjectGet, "view one fork's details"),
		toolutil.HintAction(actionProjectFork, "create a new fork"),
	)
}

// FormatLanguagesMarkdown renders project languages as Markdown.
func FormatLanguagesMarkdown(out LanguagesOutput) string {
	if len(out.Languages) == 0 {
		return toolutil.EmptyMessage("languages")
	}
	var b strings.Builder
	var pagination toolutil.PaginationOutput
	toolutil.WriteListHeading(&b, "Project Languages", len(out.Languages), pagination)
	b.WriteString(toolutil.MarkdownTableHeader("Language", "%"))
	for _, l := range out.Languages {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.EscapeMdTableCell(l.Name),
			fmt.Sprintf("%.1f%%", l.Percentage),
		))
	}
	toolutil.WriteListFooter(&b, pagination, false,
		"Use action 'repository.tree' to browse the codebase")
	return b.String()
}

// FormatListHooksMarkdown renders a list of project webhooks as Markdown.
func FormatListHooksMarkdown(out ListHooksOutput) string {
	if len(out.Hooks) == 0 {
		return toolutil.EmptyMessage("webhooks")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Project Webhooks", len(out.Hooks), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "URL", "Push", "MR", "Issues", "Pipeline", "SSL"))
	for _, h := range out.Hooks {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(h.ID, 10),
			hookNameCell(h.Name),
			toolutil.MdCodeSpanCell(h.URL),
			toolutil.BoolEmoji(h.PushEvents),
			toolutil.BoolEmoji(h.MergeRequestsEvents),
			toolutil.BoolEmoji(h.IssuesEvents),
			toolutil.BoolEmoji(h.PipelineEvents),
			toolutil.BoolEmoji(h.EnableSSLVerification),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction(actionProjectHookGet, "view one webhook's details"),
		toolutil.HintAction("project.hook_add", "add a new webhook"),
	)
	return b.String()
}

// hookNameCell renders a webhook's name, or the dash that says GitLab sent
// none, since a hook may be created without one.
func hookNameCell(name string) string {
	if name == "" {
		return "-"
	}
	return toolutil.EscapeMdTableCell(name)
}

// hookEvents pairs each event a webhook can subscribe to with whether this hook
// does, in the order the card lists them.
func hookEvents(out HookOutput) []struct {
	name string
	on   bool
} {
	return []struct {
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
		{"Resource Deploy Token", out.ResourceDeployTokenEvents},
		{"Vulnerability", out.VulnerabilityEvents},
	}
}

// FormatHookMarkdown renders a single project webhook as a card, with the
// events it subscribes to as the nested collection they are and the URL
// variable and custom header keys through the one writer of a redacted key
// table. The name and the URL used to be written as bullet-less label lines,
// which render as one run-on paragraph, and neither was escaped.
func FormatHookMarkdown(out HookOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Webhook #"+strconv.FormatInt(out.ID, 10))
	c.Field("Name", out.Name)
	c.Code("URL", out.URL)
	c.Bool("SSL Verification", out.EnableSSLVerification)
	c.Bool("Token Present", out.TokenPresent)
	c.Bool("Signing Token Present", out.SigningTokenPresent)
	events := c.Table("Event Triggers", "Event", "Enabled")
	for _, ev := range hookEvents(out) {
		events.Row(toolutil.EscapeMdTableCell(ev.name), toolutil.BoolEmoji(ev.on))
	}
	toolutil.WriteHookSecretKeys(&b, hookKeys(out.URLVariables), headerKeys(out.CustomHeaders))
	c.End(
		toolutil.HintAction(actionProjectHookEdit, "modify the event triggers"),
		toolutil.HintAction("project.hook_test", "test the webhook"),
	)
	return b.String()
}

// hookKeys lists the URL-variable keys a webhook has set; GitLab never sends
// the values back.
func hookKeys(variables []HookURLVariable) []string {
	keys := make([]string, 0, len(variables))
	for _, variable := range variables {
		keys = append(keys, variable.Key)
	}
	return keys
}

// headerKeys lists the custom-header keys a webhook has set, for the same
// reason as [hookKeys].
func headerKeys(headers []HookCustomHeader) []string {
	keys := make([]string, 0, len(headers))
	for _, header := range headers {
		keys = append(keys, header.Key)
	}
	return keys
}

// FormatListProjectUsersMarkdown renders a users list as markdown.
func FormatListProjectUsersMarkdown(out ListProjectUsersOutput) string {
	if len(out.Users) == 0 {
		return toolutil.EmptyMessage("users")
	}
	var b strings.Builder
	linked := false
	toolutil.WriteListHeading(&b, "Project Users", len(out.Users), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Username", "State"))
	for _, u := range out.Users {
		linked = linked || u.WebURL != ""
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(u.ID, 10),
			toolutil.EscapeMdTableCell(u.Name),
			toolutil.MdUserLink(u.Username, u.WebURL),
			toolutil.EscapeMdTableCell(u.State),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, linked,
		"Use action 'project.member_add' to add a new member",
		"Use action 'project.share_with_group' to share this project with a group",
	)
	return b.String()
}

// FormatListProjectGroupsMarkdown renders a project groups list as markdown.
func FormatListProjectGroupsMarkdown(out ListProjectGroupsOutput) string {
	if len(out.Groups) == 0 {
		return toolutil.EmptyMessage("groups")
	}
	var b strings.Builder
	linked := false
	toolutil.WriteListHeading(&b, "Project Groups", len(out.Groups), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Full Path"))
	for _, g := range out.Groups {
		linked = linked || g.WebURL != ""
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(g.ID, 10),
			toolutil.MdTitleLink(g.Name, g.WebURL),
			toolutil.EscapeMdTableCell(g.FullPath),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, linked,
		toolutil.HintAction(actionGroupGet, "view one group's details"))
	return b.String()
}

// FormatListStarrersMarkdown renders a starrers list as markdown.
func FormatListStarrersMarkdown(out ListProjectStarrersOutput) string {
	if len(out.Starrers) == 0 {
		return toolutil.EmptyMessage("starrers")
	}
	var b strings.Builder
	linked := false
	toolutil.WriteListHeading(&b, "Project Starrers", len(out.Starrers), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("User", "Username", "Starred Since"))
	for _, s := range out.Starrers {
		linked = linked || s.User.WebURL != ""
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.EscapeMdTableCell(s.User.Name),
			toolutil.MdUserLink(s.User.Username, s.User.WebURL),
			toolutil.FormatTime(s.StarredSince),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, linked,
		toolutil.HintAction(actionProjectGet, "view the project itself"))
	return b.String()
}

// FormatShareProjectMarkdown renders a share-project result as a card: the
// share GitLab created is the object the action returns.
func FormatShareProjectMarkdown(out ShareProjectOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Project Shared")
	// The message is server-authored but interpolates the caller's own
	// project_id, which nothing validates before it lands here.
	c.Field("Message", out.Message)
	c.Count("Group ID", out.GroupID)
	c.Field("Access Role", out.AccessRole)
	c.End(
		toolutil.HintAction(actionProjectListInvGroups, "verify the share"),
		toolutil.HintAction("project.delete_shared_group", "revoke the group's access"),
	)
	return b.String()
}

// FormatTriggerTestHookMarkdown renders a webhook test trigger result as
// markdown. The message names the event the caller asked for, so it is escaped
// rather than written as it arrived.
func FormatTriggerTestHookMarkdown(out TriggerTestHookOutput) string {
	return toolutil.EmojiSuccess + " " + toolutil.EscapeMdTableCell(out.Message) + "\n"
}

// FormatPushRuleMarkdown renders a push rule as a card. Every regex is a code
// span: a rule holding an alternation used to reach the model as
// `^(feat&#124;fix):`, which is not the pattern GitLab enforces.
func FormatPushRuleMarkdown(out PushRuleOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Push Rule (ID: "+strconv.FormatInt(out.ID, 10)+")")
	c.Int("Project ID", out.ProjectID)
	c.Code("Commit message regex", out.CommitMessageRegex)
	c.Code("Commit message negative regex", out.CommitMessageNegativeRegex)
	c.Code("Branch name regex", out.BranchNameRegex)
	c.Code("Author email regex", out.AuthorEmailRegex)
	c.Code("File name regex", out.FileNameRegex)
	// GitLab's own zero for this setting means no limit, which "0" said the
	// opposite of.
	c.Field("Max file size (MB)", maxFileSize(out.MaxFileSize))
	c.Bool("Deny delete tag", out.DenyDeleteTag)
	c.Bool("Member check", out.MemberCheck)
	c.Bool("Prevent secrets", out.PreventSecrets)
	c.Bool("Commit committer check", out.CommitCommitterCheck)
	c.Bool("Commit committer name check", out.CommitCommitterNameCheck)
	c.Bool("Reject unsigned commits", out.RejectUnsignedCommits)
	c.Bool("Reject non-DCO commits", out.RejectNonDCOCommits)
	c.Time("Created", out.CreatedAt)
	c.End(
		toolutil.HintAction(actionProjectEditPushRule, "modify these push rules"),
		toolutil.HintAction(actionProjectDeletePushRule, "remove them"),
	)
	return b.String()
}

// maxFileSize renders the push rule's file-size ceiling, where GitLab's zero
// means there is none.
func maxFileSize(mb int64) string {
	if mb == 0 {
		return "unlimited"
	}
	return strconv.FormatInt(mb, 10)
}

// FormatDownloadAvatarMarkdown renders an avatar download result as a card.
func FormatDownloadAvatarMarkdown(out DownloadAvatarOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Project Avatar")
	c.Int("Size", int64(out.SizeBytes))
	c.Field("Content", fmt.Sprintf("base64-encoded (%d chars)", len(out.ContentBase64)))
	c.End(toolutil.HintAction("project.upload_avatar", "replace the avatar"))
	return b.String()
}

// FormatApprovalConfigMarkdown renders approval configuration as a card.
func FormatApprovalConfigMarkdown(out ApprovalConfigOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Approval Configuration")
	c.Int("Approvals before merge", out.ApprovalsBeforeMerge)
	c.Bool("Reset approvals on push", out.ResetApprovalsOnPush)
	c.Bool("Disable overriding approvers per MR", out.DisableOverridingApproversPerMergeRequest)
	c.Bool("Author self-approval", out.MergeRequestsAuthorApproval)
	c.Bool("Disable committers approval", out.MergeRequestsDisableCommittersApproval)
	c.Bool("Require reauthentication to approve", out.RequireReauthenticationToApprove)
	c.Bool("Selective code owner removals", out.SelectiveCodeOwnerRemovals)
	c.End(
		toolutil.HintAction("project.approval_config_change", "modify these settings"),
		toolutil.HintAction(actionProjectApprovalRuleList, "see the approval rules"),
	)
	return b.String()
}

// FormatApprovalRuleMarkdown renders a single approval rule as a card.
func FormatApprovalRuleMarkdown(out ApprovalRuleOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Approval Rule: "+out.Name)
	c.Int("ID", out.ID)
	c.Int("Approvals Required", out.ApprovalsRequired)
	c.Field("Rule Type", out.RuleType)
	c.Field("Report Type", out.ReportType)
	c.Bool("Applies to all protected branches", out.AppliesToAllProtectedBranches)
	c.Bool("Contains hidden groups", out.ContainsHiddenGroups)
	c.Field("Users", strings.Join(userNames(out.Users), ", "))
	c.Field("Groups", strings.Join(groupNames(out.Groups), ", "))
	c.Field("Eligible Approvers", strings.Join(userNames(out.EligibleApprovers), ", "))
	c.End(
		toolutil.HintAction("project.approval_rule_update", "modify this rule"),
		toolutil.HintAction("project.approval_rule_delete", "remove it"),
	)
	return b.String()
}

// FormatListApprovalRulesMarkdown renders a list of approval rules as Markdown.
func FormatListApprovalRulesMarkdown(out ListApprovalRulesOutput) string {
	if len(out.Rules) == 0 {
		return toolutil.EmptyMessage("approval rules")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Approval Rules", len(out.Rules), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Type", "Approvals", "All Protected", "Users", "Groups"))
	for _, r := range out.Rules {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(r.ID, 10),
			toolutil.EscapeMdTableCell(r.Name),
			dashIfEmpty(toolutil.EscapeMdTableCell(r.RuleType)),
			strconv.FormatInt(r.ApprovalsRequired, 10),
			toolutil.BoolEmoji(r.AppliesToAllProtectedBranches),
			dashIfEmpty(toolutil.EscapeMdTableCell(strings.Join(userNames(r.Users), ", "))),
			dashIfEmpty(toolutil.EscapeMdTableCell(strings.Join(groupNames(r.Groups), ", "))),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction(actionProjectApprovalRuleGet, "see one rule in full"),
		toolutil.HintAction("project.approval_rule_create", "add a new rule"),
	)
	return b.String()
}

// dashIfEmpty renders the dash a table cell shows where a card row would write
// nothing: a row has to have a cell in every column.
func dashIfEmpty(cell string) string {
	if cell == "" {
		return "-"
	}
	return cell
}

// FormatPullMirrorMarkdown renders pull mirror details as a card.
func FormatPullMirrorMarkdown(out PullMirrorOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Pull Mirror (ID: "+strconv.FormatInt(out.ID, 10)+")")
	c.Bool("Enabled", out.Enabled)
	// The mirror source URL is whatever the maintainer configuring it typed.
	c.Code("URL", out.URL)
	c.Field("Update Status", out.UpdateStatus)
	// GitLab quotes the remote's own output back in this field.
	c.Text("Last Error", out.LastError)
	c.Time("Last Successful Update", out.LastSuccessfulUpdateAt)
	c.Time("Last Update", out.LastUpdateAt)
	c.Time("Last Update Started", out.LastUpdateStartedAt)
	c.Bool("Trigger Builds", out.MirrorTriggerBuilds)
	c.Bool("Only Protected Branches", out.OnlyMirrorProtectedBranches)
	c.Bool("Overwrite Diverged Branches", out.MirrorOverwritesDivergedBranches)
	c.Code("Branch Regex", out.MirrorBranchRegex)
	c.End(
		toolutil.HintAction("project.pull_mirror_configure", "modify the mirror settings"),
		toolutil.HintAction("project.start_mirroring", "trigger an immediate update"),
	)
	return b.String()
}

// FormatRepositoryStorageMarkdown renders repository storage info as a card.
func FormatRepositoryStorageMarkdown(out RepositoryStorageOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Repository Storage")
	c.Int("Project ID", out.ProjectID)
	c.Code("Disk Path", out.DiskPath)
	// A storage shard name is an identifier the instance operator chose.
	c.Code("Repository Storage", out.RepositoryStorage)
	c.Time("Created", out.CreatedAt)
	c.End(toolutil.HintAction("project.start_housekeeping", "optimize the repository"))
	return b.String()
}

// FormatListTargetBranchRulesMarkdown renders a project's target branch rules
// as a Markdown table.
func FormatListTargetBranchRulesMarkdown(out ListTargetBranchRulesOutput) string {
	if len(out.TargetBranchRules) == 0 {
		return toolutil.EmptyMessage("target branch rules")
	}
	var b strings.Builder
	var pagination toolutil.PaginationOutput
	toolutil.WriteListHeading(&b, "Target Branch Rules", len(out.TargetBranchRules), pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Source Pattern", "Target Branch", "Created"))
	for _, r := range out.TargetBranchRules {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(r.ID, 10),
			toolutil.MdCodeSpanCell(r.Name),
			toolutil.MdCodeSpanCell(r.TargetBranch),
			dashIfEmpty(toolutil.FormatTime(r.CreatedAt)),
		))
	}
	toolutil.WriteListFooter(&b, pagination, false,
		"Use action 'project.target_branch_rule_create' to add a rule",
		"Use action 'project.target_branch_rule_delete' with a rule_id to remove a rule",
	)
	return b.String()
}

// FormatTargetBranchRuleMarkdown renders a single target branch rule as a card.
func FormatTargetBranchRuleMarkdown(out TargetBranchRuleOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Target Branch Rule: "+out.Name)
	c.Int("ID", out.ID)
	// Both come straight off the caller's own create input.
	c.Code("Source Pattern", out.Name)
	c.Code("Target Branch", out.TargetBranch)
	c.Time("Created", out.CreatedAt)
	c.End("Use action 'project.target_branch_rule_list' to see all rules for the project")
	return b.String()
}

func init() {
	toolutil.RegisterMarkdownResult(formatProjectNotFound)
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
	toolutil.RegisterMarkdown(FormatShareProjectMarkdown)
	toolutil.RegisterMarkdown(FormatTriggerTestHookMarkdown)
	toolutil.RegisterMarkdown(FormatPushRuleMarkdown)
	toolutil.RegisterMarkdown(FormatDownloadAvatarMarkdown)
	toolutil.RegisterMarkdown(FormatApprovalConfigMarkdown)
	toolutil.RegisterMarkdown(FormatApprovalRuleMarkdown)
	toolutil.RegisterMarkdown(FormatListApprovalRulesMarkdown)
	toolutil.RegisterMarkdown(FormatPullMirrorMarkdown)
	toolutil.RegisterMarkdown(FormatRepositoryStorageMarkdown)
	toolutil.RegisterMarkdown(FormatListTargetBranchRulesMarkdown)
	toolutil.RegisterMarkdown(FormatTargetBranchRuleMarkdown)
}
