package pipelineschedules

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// FormatOutputMarkdown renders a single pipeline schedule as Markdown.
func FormatOutputMarkdown(s Output) string {
	if s.ID == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Pipeline Schedule #%d\n\n", s.ID)
	b.WriteString("| Field | Value |\n")
	b.WriteString(toolutil.TblSep2Col)
	fmt.Fprintf(&b, "| Description | %s |\n", toolutil.EscapeMdTableCell(s.Description))
	fmt.Fprintf(&b, "| Ref | %s |\n", toolutil.EscapeMdTableCell(s.Ref))
	fmt.Fprintf(&b, "| Cron | `%s` |\n", toolutil.EscapeMdTableCell(s.Cron))
	if s.CronTimezone != "" {
		fmt.Fprintf(&b, "| Timezone | %s |\n", toolutil.EscapeMdTableCell(s.CronTimezone))
	}
	fmt.Fprintf(&b, "| Active | %s |\n", toolutil.BoolEmoji(s.Active))
	if s.NextRunAt != "" {
		fmt.Fprintf(&b, "| Next Run | %s |\n", toolutil.FormatTime(s.NextRunAt))
	}
	if s.OwnerName != "" {
		fmt.Fprintf(&b, "| Owner | %s |\n", toolutil.EscapeMdTableCell(s.OwnerName))
	}
	if s.CreatedAt != "" {
		fmt.Fprintf(&b, "| Created | %s |\n", toolutil.FormatTime(s.CreatedAt))
	}
	if s.UpdatedAt != "" {
		fmt.Fprintf(&b, "| Updated | %s |\n", toolutil.FormatTime(s.UpdatedAt))
	}
	toolutil.WriteHints(
		&b,
		"Use the selected tool surface's pipeline-schedule update action with the same project_id and schedule_id to modify schedule settings",
		"Use the selected tool surface's pipeline-schedule run action with the same project_id and schedule_id to trigger this schedule immediately",
		"Use the selected tool surface's pipeline-schedule delete action with the same project_id, schedule_id, and explicit confirm=true to remove this schedule",
	)
	return b.String()
}

// FormatListMarkdown renders a paginated list of pipeline schedules as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Schedules) == 0 {
		return "No pipeline schedules found.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Pipeline Schedules (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Schedules), out.Pagination)
	b.WriteString("| ID | Description | Ref | Cron | Active | Owner |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- |\n")
	for _, s := range out.Schedules {
		fmt.Fprintf(&b, "| %d | %s | %s | `%s` | %t | %s |\n",
			s.ID, toolutil.EscapeMdTableCell(s.Description), toolutil.EscapeMdTableCell(s.Ref), toolutil.EscapeMdTableCell(s.Cron), s.Active, toolutil.EscapeMdTableCell(s.OwnerName))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(
		&b,
		toolutil.HintPreserveLinks,
		"Use the selected tool surface's pipeline-schedule get action with the same project_id and schedule_id for full details",
		"Use the selected tool surface's pipeline-schedule create action with project_id to add a new schedule",
	)
	return b.String()
}

// FormatVariableMarkdown renders a pipeline schedule variable as Markdown.
func FormatVariableMarkdown(v VariableOutput) string {
	var b strings.Builder
	b.WriteString("## Pipeline Schedule Variable\n\n")
	fmt.Fprintf(&b, "- **Key**: %s\n", v.Key)
	fmt.Fprintf(&b, "- **Value**: %s\n", v.Value)
	if v.VariableType != "" {
		fmt.Fprintf(&b, "- **Type**: %s\n", v.VariableType)
	}
	toolutil.WriteHints(
		&b,
		"Use the selected tool surface's pipeline-schedule variable edit action with the same project_id, schedule_id, and key to change this variable",
		"Use the selected tool surface's pipeline-schedule variable delete action with the same project_id, schedule_id, key, and explicit confirm=true to remove it",
	)
	return b.String()
}

// FormatTriggeredPipelinesMarkdown renders a list of triggered pipelines as Markdown.
func FormatTriggeredPipelinesMarkdown(out TriggeredPipelinesListOutput) string {
	if len(out.Pipelines) == 0 {
		return "No triggered pipelines found.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Triggered Pipelines (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Pipelines), out.Pagination)
	b.WriteString("| ID | IID | Ref | Status | Source |\n")
	b.WriteString("| --- | --- | --- | --- | --- |\n")
	for _, p := range out.Pipelines {
		fmt.Fprintf(&b, "| %s | %d | %s | %s | %s |\n",
			toolutil.MdTitleLink(fmt.Sprintf("#%d", p.ID), p.WebURL), p.IID, toolutil.EscapeMdTableCell(p.Ref), p.Status, p.Source)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(
		&b,
		toolutil.HintPreserveLinks,
		"Use the selected tool surface's pipeline get action with pipeline_id for full details",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatVariableMarkdown)
	toolutil.RegisterMarkdown(FormatTriggeredPipelinesMarkdown)
}
