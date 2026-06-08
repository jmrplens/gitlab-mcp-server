package pipelinetriggers

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// FormatTriggerMarkdown formats a single pipeline trigger as markdown.
func FormatTriggerMarkdown(out Output) string {
	var b strings.Builder
	b.WriteString("## Pipeline Trigger\n\n")
	b.WriteString("| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&b, "| ID | %d |\n", out.ID)
	fmt.Fprintf(&b, "| Description | %s |\n", toolutil.EscapeMdTableCell(out.Description))
	fmt.Fprintf(&b, "| Token | %s |\n", toolutil.EscapeMdTableCell(out.Token))
	if out.OwnerName != "" {
		fmt.Fprintf(&b, "| Owner | %s |\n", toolutil.EscapeMdTableCell(out.OwnerName))
	}
	if out.CreatedAt != "" {
		fmt.Fprintf(&b, "| Created | %s |\n", toolutil.FormatTime(out.CreatedAt))
	}
	if out.LastUsed != "" {
		fmt.Fprintf(&b, "| Last Used | %s |\n", toolutil.FormatTime(out.LastUsed))
	}
	toolutil.WriteHints(
		&b,
		"Use the selected tool surface's pipeline-trigger update action with the same project_id and trigger_id to modify this trigger",
		"Use the selected tool surface's pipeline-trigger run action with the same project_id, ref, and this token to execute a pipeline",
		"Use the selected tool surface's pipeline-trigger delete action with the same project_id, trigger_id, and explicit confirm=true to remove this trigger",
	)
	return b.String()
}

// FormatListTriggersMarkdown formats a list of pipeline triggers as markdown.
func FormatListTriggersMarkdown(out ListOutput) string {
	var b strings.Builder
	b.WriteString("## Pipeline Triggers\n\n")
	toolutil.WriteListSummary(&b, len(out.Triggers), out.Pagination)
	if len(out.Triggers) == 0 {
		b.WriteString("No pipeline triggers found.\n")
		return b.String()
	}
	b.WriteString("| ID | Description | Token | Owner | Last Used |\n|---|---|---|---|---|\n")
	for _, t := range out.Triggers {
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %s |\n",
			t.ID,
			toolutil.EscapeMdTableCell(t.Description),
			toolutil.EscapeMdTableCell(t.Token),
			toolutil.EscapeMdTableCell(t.OwnerName),
			toolutil.FormatTime(t.LastUsed))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(
		&b,
		toolutil.HintPreserveLinks,
		"Use the selected tool surface's pipeline-trigger get action with the same project_id and trigger_id for full details",
		"Use the selected tool surface's pipeline-trigger create action with project_id to add a new pipeline trigger",
	)
	return b.String()
}

// FormatRunOutputMarkdown formats the result of triggering a pipeline as markdown.
func FormatRunOutputMarkdown(out RunOutput) string {
	var b strings.Builder
	b.WriteString("## Pipeline Triggered\n\n")
	b.WriteString("| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&b, "| Pipeline ID | %d |\n", out.PipelineID)
	fmt.Fprintf(&b, "| SHA | %s |\n", out.SHA)
	fmt.Fprintf(&b, "| Ref | %s |\n", toolutil.EscapeMdTableCell(out.Ref))
	fmt.Fprintf(&b, "| Status | %s |\n", out.Status)
	if out.WebURL != "" {
		fmt.Fprintf(&b, "| URL | %s |\n", toolutil.MdTitleLink(fmt.Sprintf("Pipeline #%d", out.PipelineID), out.WebURL))
	}
	toolutil.WriteHints(
		&b,
		toolutil.HintPreserveLinks,
		"Use the selected tool surface's pipeline get action with pipeline_id to monitor progress",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatTriggerMarkdown)
	toolutil.RegisterMarkdown(FormatListTriggersMarkdown)
	toolutil.RegisterMarkdown(FormatRunOutputMarkdown)
}
