package externalstatuschecks

import (
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The two canonical action IDs the merge-request views name, beside the ones
// action_specs.go already declares. The ID is the one form every surface
// resolves, so a hint written this way is never a name the serving surface
// does not register.
const (
	actionSetProjectMRStatus  = "external_status_check.set_project_mr_status"
	actionRetryProjectMRCheck = "external_status_check.retry_project"
)

// allBranches is what an empty protected-branch list means: GitLab scopes a
// status check to the branches it names, and a check that names none applies
// everywhere. Rendering the empty list as nothing, or as a count of zero, read
// as a check that applies to no branch at all.
const allBranches = "All branches"

// FormatMergeCheckMarkdown renders one merge request status check as the card
// of one object.
func FormatMergeCheckMarkdown(out MergeStatusCheckOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, checkHeading(out.Name))
	c.Int("ID", out.ID)
	c.Field("Name", out.Name)
	c.Field("Status", out.Status)
	// The external URL is whatever the maintainer who added the check typed.
	c.Link("External URL", out.ExternalURL, out.ExternalURL)
	c.End(
		toolutil.HintAction(actionSetProjectMRStatus, "record this check's result on the merge request"),
		toolutil.HintAction(actionRetryProjectMRCheck, "retry a failed check"),
		toolutil.HintAction(actionListProjectMR, "see the merge request's other checks"),
	)
	return b.String()
}

// FormatProjectCheckMarkdown renders one project status check as the card of
// one object, with the branches it is scoped to as a nested collection.
func FormatProjectCheckMarkdown(out ProjectStatusCheckOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, checkHeading(out.Name))
	c.Int("ID", out.ID)
	c.Field("Name", out.Name)
	c.Int("Project ID", out.ProjectID)
	// The external URL is whatever the maintainer who added the check typed.
	c.Link("External URL", out.ExternalURL, out.ExternalURL)
	c.Bool("HMAC", out.HMAC)
	if len(out.ProtectedBranches) == 0 {
		c.Field("Protected Branches", allBranches)
	} else {
		branches := c.Table("Protected Branches", "ID", "Name", "Code Owner Approval")
		for _, pb := range out.ProtectedBranches {
			branches.Row(
				strconv.FormatInt(pb.ID, 10),
				// git check-ref-format permits '|', '<' and '>' in a branch
				// name, so a name is not an identifier.
				toolutil.EscapeMdTableCell(pb.Name),
				toolutil.BoolEmoji(pb.CodeOwnerApprovalRequired),
			)
		}
	}
	c.End(
		toolutil.HintAction(actionUpdateProject, "change this check's name, URL or branch scope"),
		toolutil.HintAction(actionDeleteProject, "remove this check"),
		toolutil.HintAction(actionListProject, "see the project's other checks"),
	)
	return b.String()
}

// FormatListMergeMarkdown renders a page of merge request status checks as a
// Markdown table: a collection of objects that share columns.
func FormatListMergeMarkdown(out ListMergeStatusCheckOutput) string {
	if len(out.Items) == 0 {
		return toolutil.EmptyMessage("merge status checks")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Merge Status Checks", len(out.Items), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "External URL", "Status"))
	for _, c := range out.Items {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(c.ID, 10),
			toolutil.EscapeMdTableCell(c.Name),
			toolutil.MdTitleLink(c.ExternalURL, c.ExternalURL),
			toolutil.EscapeMdTableCell(c.Status),
		))
	}
	// The URL column is a link, so the footer keeps the instruction to
	// preserve it, which WriteListFooter puts first.
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintAction(actionSetProjectMRStatus, "record a check's result on the merge request"),
		toolutil.HintAction(actionRetryProjectMRCheck, "retry a failed check"),
	)
	return b.String()
}

// FormatListProjectMarkdown renders a page of project status checks as a
// Markdown table. The branch column says what the check is scoped to rather
// than how many branches are named, since a check that names none applies to
// every branch.
func FormatListProjectMarkdown(out ListProjectStatusCheckOutput) string {
	if len(out.Items) == 0 {
		return toolutil.EmptyMessage("project external status checks")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Project External Status Checks", len(out.Items), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "External URL", "HMAC", "Protected Branches"))
	for _, c := range out.Items {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(c.ID, 10),
			toolutil.EscapeMdTableCell(c.Name),
			toolutil.MdTitleLink(c.ExternalURL, c.ExternalURL),
			toolutil.BoolEmoji(c.HMAC),
			branchScope(len(c.ProtectedBranches)),
		))
	}
	// The URL column is a link, so the footer keeps the instruction to
	// preserve it, which WriteListFooter puts first.
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintAction(actionCreateProject, "add a check to this project"),
		toolutil.HintAction(actionUpdateProject, "change one check's name, URL or branch scope"),
		toolutil.HintAction(actionDeleteProject, "remove a check"),
	)
	return b.String()
}

// checkHeading names the check in the card's heading, or opens the generic one
// when GitLab sent no name.
func checkHeading(name string) string {
	if strings.TrimSpace(name) == "" {
		return "External Status Check"
	}
	return "External Status Check: " + name
}

// branchScope renders how many protected branches a check names, or what it
// means when it names none.
func branchScope(n int) string {
	if n == 0 {
		return allBranches
	}
	return strconv.Itoa(n)
}

func init() {
	toolutil.RegisterMarkdown(FormatMergeCheckMarkdown)
	toolutil.RegisterMarkdown(FormatProjectCheckMarkdown)
	toolutil.RegisterMarkdown(FormatListMergeMarkdown)
	toolutil.RegisterMarkdown(FormatListProjectMarkdown)
}
