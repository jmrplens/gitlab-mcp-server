// markdown.go provides Markdown formatting functions for pipeline trigger MCP tool output.
package pipelinetriggers

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatTriggerMarkdown formats a single pipeline trigger as markdown.
func FormatTriggerMarkdown(out Output) string {
	var b strings.Builder
	b.WriteString("## Pipeline Trigger\n\n")
	b.WriteString("| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&b, "| ID | %d |\n", out.ID)
	b.WriteString("| Description | " + toolutil.EscapeMdTableCell(out.Description) + " |\n")
	b.WriteString("| Token | " + toolutil.EscapeMdTableCell(out.Token) + " |\n")
	if out.OwnerName != "" {
		b.WriteString("| Owner | " + toolutil.EscapeMdTableCell(out.OwnerName) + " |\n")
	}
	if out.CreatedAt != "" {
		b.WriteString("| Created | " + toolutil.FormatTime(out.CreatedAt) + " |\n")
	}
	if out.LastUsed != "" {
		b.WriteString("| Last Used | " + out.LastUsed + " |\n")
	}
	toolutil.WriteHints(&b,
		"Use action 'update' to modify this trigger",
		"Use action 'run' to execute a pipeline with this trigger",
		"Use action 'delete' to remove this trigger",
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
			t.LastUsed)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		"Use action 'get' with trigger_id for full details",
		"Use action 'create' to add a new pipeline trigger",
	)
	return b.String()
}

// FormatRunOutputMarkdown formats the result of triggering a pipeline as markdown.
func FormatRunOutputMarkdown(out RunOutput) string {
	var b strings.Builder
	b.WriteString("## Pipeline Triggered\n\n")
	b.WriteString("| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&b, "| Pipeline ID | %d |\n", out.PipelineID)
	b.WriteString("| SHA | " + out.SHA + " |\n")
	b.WriteString("| Ref | " + toolutil.EscapeMdTableCell(out.Ref) + " |\n")
	b.WriteString("| Status | " + out.Status + " |\n")
	if out.WebURL != "" {
		b.WriteString("| URL | " + toolutil.MdTitleLink(fmt.Sprintf("Pipeline #%d", out.PipelineID), out.WebURL) + " |\n")
	}
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use gitlab_pipeline with the pipeline_id to monitor progress",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatTriggerMarkdown)
	toolutil.RegisterMarkdown(FormatListTriggersMarkdown)
	toolutil.RegisterMarkdown(FormatRunOutputMarkdown)
}
