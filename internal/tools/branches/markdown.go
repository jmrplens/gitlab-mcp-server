package branches

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name that the action specs do not already
// spell, the one form every surface resolves: the dynamic surface executes
// them, and the meta and individual surfaces resolve them to their own tool
// names.
const (
	actionBranchGet          = "branch.get"
	actionBranchCreate       = "branch.create"
	actionBranchDelete       = "branch.delete"
	actionCommitList         = "repository.commit_list"
	actionMergeRequestCreate = "merge_request.create"
)

type branchNotFoundOutput struct {
	Identifier string
}

func formatBranchNotFound(out branchNotFoundOutput) *mcp.CallToolResult {
	return toolutil.NotFoundResult(
		"Branch", out.Identifier,
		"Use gitlab_branch_list with project_id to list available branches",
		"Verify the branch name is spelled correctly (case-sensitive)",
	)
}

// FormatOutputMarkdown renders one branch as a card: its flags, what the
// caller may do with it, and the head commit as a nested object.
//
// The three permission flags GitLab sends on every branch — whether the caller
// can push, and whether developers may push or merge — used to be dropped, so
// a reader could not tell a branch they may write from one they may not.
func FormatOutputMarkdown(br Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Branch: "+br.Name)
	c.Bool("Protected", br.Protected)
	c.Bool("Default", br.Default)
	c.Bool("Merged", br.Merged)
	c.Bool("You Can Push", br.CanPush)
	c.Bool("Developers Can Push", br.DevelopersCanPush)
	c.Bool("Developers Can Merge", br.DevelopersCanMerge)
	if br.Commit != nil {
		commit := c.Sub("Commit")
		commit.Code("SHA", br.Commit.ID)
		commit.Field("Title", br.Commit.Title)
		commit.Field("Author", br.Commit.AuthorName)
		commit.Time("Committed", br.Commit.CommittedDate)
	}
	c.URL(br.WebURL)
	c.End(
		toolutil.HintAction(actionMergeRequestCreate, "open a merge request from this branch"),
		toolutil.HintAction(actionCommitList, "see recent commits on this branch"),
		toolutil.HintAction(actionBranchDelete, "remove the branch after merging"),
	)
	return b.String()
}

// FormatListMarkdown renders a page of branches as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Branches) == 0 {
		return toolutil.EmptyMessage("branches")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Branches", len(out.Branches), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("Name", "Protected", "Default", "Merged"))
	for _, br := range out.Branches {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdTitleLink(br.Name, br.WebURL),
			toolutil.BoolEmoji(br.Protected),
			toolutil.BoolEmoji(br.Default),
			toolutil.BoolEmoji(br.Merged),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintAction(actionBranchGet, "see one branch in full"),
		toolutil.HintAction(actionBranchCreate, "create a new branch"),
		toolutil.HintAction(actionBranchProtect, "protect a branch"),
	)
	return b.String()
}

// accessLevelsSummary renders the access levels of a protected branch rule as
// the role names GitLab gives them ("Maintainers, Developers"), or "-" when
// the array is empty. The numbers alone, which this used to print, are a
// lookup table a reader does not have.
func accessLevelsSummary(levels []BranchAccessDescriptionOutput) string {
	if len(levels) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(levels))
	for _, l := range levels {
		parts = append(parts, accessLevelLabel(l))
	}
	return strings.Join(parts, ", ")
}

// accessLevelLabel names one access-level entry: the role the level stands
// for, and the principal the entry names when it grants one user, group or
// deploy key rather than a role.
func accessLevelLabel(l BranchAccessDescriptionOutput) string {
	label := toolutil.AccessLevelDescription(gl.AccessLevelValue(l.AccessLevel))
	switch {
	case l.UserID != 0:
		return fmt.Sprintf("%s (User #%d)", label, l.UserID)
	case l.GroupID != 0:
		return fmt.Sprintf("%s (Group #%d)", label, l.GroupID)
	case l.DeployKeyID != 0:
		return fmt.Sprintf("%s (Deploy Key #%d)", label, l.DeployKeyID)
	default:
		return label
	}
}

// FormatProtectedMarkdown renders one protected branch rule as a card.
func FormatProtectedMarkdown(pb ProtectedOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Protected Branch: "+pb.Name)
	c.Int("ID", pb.ID)
	c.Field("Push Access Levels", accessLevelsSummary(pb.PushAccessLevels))
	c.Field("Merge Access Levels", accessLevelsSummary(pb.MergeAccessLevels))
	c.Field("Unprotect Access Levels", accessLevelsSummary(pb.UnprotectAccessLevels))
	c.Bool("Allow Force Push", pb.AllowForcePush)
	c.Bool("Code Owner Approval Required", pb.CodeOwnerApprovalRequired)
	c.Flag("", "Inherited from the group, not set on the project", pb.Inherited)
	c.End(
		toolutil.HintAction(actionBranchGetProtected, "fetch this protection again before updating it"),
		toolutil.HintAction(actionBranchUpdateProtected, "change protection settings"),
		toolutil.HintAction(actionBranchUnprotect, "remove branch protection"),
	)
	return b.String()
}

// FormatProtectedListMarkdown renders a page of protected branch rules as a
// Markdown table.
func FormatProtectedListMarkdown(out ProtectedListOutput) string {
	if len(out.Branches) == 0 {
		return toolutil.EmptyMessage("protected branches")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Protected Branches", len(out.Branches), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("Name", "Push Levels", "Merge Levels", "Force Push", "Code Owner Approval"))
	for _, pb := range out.Branches {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.EscapeMdTableCell(pb.Name),
			toolutil.EscapeMdTableCell(accessLevelsSummary(pb.PushAccessLevels)),
			toolutil.EscapeMdTableCell(accessLevelsSummary(pb.MergeAccessLevels)),
			toolutil.BoolEmoji(pb.AllowForcePush),
			toolutil.BoolEmoji(pb.CodeOwnerApprovalRequired),
		))
	}
	// The table carries no link, so the instruction to preserve one is dropped.
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction(actionBranchGetProtected, "see one rule in full before updating or unprotecting"),
		toolutil.HintAction(actionBranchProtect, "add branch protection"),
		toolutil.HintAction(actionBranchList, "list the branches these rules match"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdownResult(formatBranchNotFound)
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatProtectedMarkdown)
	toolutil.RegisterMarkdown(FormatProtectedListMarkdown)
}
