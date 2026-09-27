package resourcegroups

import (
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatListMarkdown renders a project's resource groups as a Markdown table.
//
// The two next steps are named separately: reading one group and changing its
// process mode are different actions, and one hint offering both named a tool
// that only reads.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Groups) == 0 {
		return toolutil.EmptyMessage("resource groups")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Resource Groups", len(out.Groups), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Key", "Process Mode"))
	for _, g := range out.Groups {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(g.ID, 10),
			toolutil.EscapeMdTableCell(g.Key),
			toolutil.EscapeMdTableCell(g.ProcessMode),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction(actionResourceGroupGet, "see one resource group in full"),
		toolutil.HintAction(actionResourceGroupEdit, "change a group's process mode"),
	)
	return b.String()
}

// FormatGroupMarkdown renders one resource group as the card of a single
// object.
func FormatGroupMarkdown(g ResourceGroupItem) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Resource Group: "+g.Key)
	c.Int("ID", g.ID)
	// The key is the resource_group name written in .gitlab-ci.yml.
	c.Field("Key", g.Key)
	c.Field("Process Mode", g.ProcessMode)
	c.Time("Created", g.CreatedAt)
	c.Time("Updated", g.UpdatedAt)
	c.End(
		toolutil.HintAction(actionResourceGroupUpcomingJobs, "see the jobs waiting on this group"),
		toolutil.HintAction(actionResourceGroupEdit, "change its process mode"),
	)
	return b.String()
}

// FormatJobsMarkdown renders the jobs waiting on a resource group as a
// Markdown table.
func FormatJobsMarkdown(out ListUpcomingJobsOutput) string {
	if len(out.Jobs) == 0 {
		return toolutil.EmptyMessage("upcoming jobs")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Upcoming Jobs", len(out.Jobs), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Status", "Stage", "Ref", "Pipeline", "Created"))
	for _, j := range out.Jobs {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdTitleLink(strconv.FormatInt(j.ID, 10), j.WebURL),
			toolutil.EscapeMdTableCell(j.Name),
			jobStatusCell(j.Status),
			toolutil.EscapeMdTableCell(j.Stage),
			toolutil.EscapeMdTableCell(j.Ref),
			pipelineCell(j.Pipeline),
			toolutil.FormatTime(j.CreatedAt),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintAction(actionJobGet, "see one of these jobs in full"),
		toolutil.HintAction(actionJobTrace, "read a job's log"),
		toolutil.HintAction(actionResourceGroupList, "see the other resource groups of this project"),
	)
	return b.String()
}

// jobStatusCell renders a job status with the glyph every job row in the tree
// shows, and nothing when GitLab sent no status.
func jobStatusCell(status string) string {
	if strings.TrimSpace(status) == "" {
		return ""
	}
	return toolutil.PipelineStatusEmoji(status) + " " + toolutil.EscapeMdTableCell(status)
}

// pipelineCell names the pipeline a waiting job belongs to, linked to its page
// when GitLab sent one, or nothing when GitLab rendered no pipeline.
func pipelineCell(p *JobPipelineItem) string {
	if p == nil {
		return ""
	}
	return toolutil.MdTitleLink("#"+strconv.FormatInt(p.ID, 10), p.WebURL)
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatGroupMarkdown)
	toolutil.RegisterMarkdown(FormatJobsMarkdown)
}
