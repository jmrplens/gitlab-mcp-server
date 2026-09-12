package pipelineschedules

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name. Every surface resolves the ID: the
// dynamic surface executes it and the meta and individual surfaces resolve it
// to their own tool names, so a hint written this way never names something
// the serving surface does not register — which "the selected tool surface's
// pipeline-schedule update action" was a sentence-long way of avoiding.
//
// The ID is the catalog's: these actions are registered under the
// gitlab_pipeline group, so their domain is "pipeline" and not
// "pipeline_schedule", which is what the related-action constants beside
// ActionSpecs still spell.
const (
	hintActionScheduleGet            = "pipeline.schedule_get"
	hintActionScheduleList           = "pipeline.schedule_list"
	hintActionScheduleCreate         = "pipeline.schedule_create"
	hintActionScheduleUpdate         = "pipeline.schedule_update"
	hintActionScheduleDelete         = "pipeline.schedule_delete"
	hintActionScheduleRun            = "pipeline.schedule_run"
	hintActionScheduleEditVariable   = "pipeline.schedule_edit_variable"
	hintActionScheduleDeleteVariable = "pipeline.schedule_delete_variable"
	hintActionPipelineGet            = "pipeline.get"
)

// FormatOutputMarkdown renders one pipeline schedule as the card of a single
// object. A schedule with no ID is no schedule and renders nothing.
func FormatOutputMarkdown(s Output) string {
	if s.ID == 0 {
		return ""
	}
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Pipeline Schedule #%d", s.ID))
	// The description, the ref and the cron are what a maintainer typed.
	c.Field("Description", s.Description)
	c.Field("Ref", s.Ref)
	c.Code("Cron", s.Cron)
	c.Field("Timezone", s.CronTimezone)
	c.Bool("Active", s.Active)
	c.Time("Next Run", s.NextRunAt)
	if s.Owner != nil {
		c.Markdown("Owner", toolutil.MdUserLink(s.Owner.Username, s.Owner.WebURL))
	}
	if s.LastPipeline != nil {
		// The documented last_pipeline reference carries no web_url, so this
		// row names the pipeline rather than linking it.
		c.Field("Last Pipeline", fmt.Sprintf("#%d (%s)", s.LastPipeline.ID, s.LastPipeline.Status))
	}
	// A schedule runs with its variables and inputs, and a reader deciding
	// whether to run one needs to know which are set. The values never appear:
	// a schedule variable may be a secret, and GitLab shows one nowhere.
	c.Field("Variables", variableKeySummary(s.Variables))
	c.Field("Inputs", inputNameSummary(s.Inputs))
	c.Time("Created", s.CreatedAt)
	c.Time("Updated", s.UpdatedAt)
	c.End(
		toolutil.HintAction(hintActionScheduleUpdate, "change this schedule"),
		toolutil.HintAction(hintActionScheduleRun, "trigger it now"),
		toolutil.HintAction(hintActionScheduleDelete, "remove it"),
	)
	return b.String()
}

// variableKeySummary names the keys a schedule carries and the type of each,
// never a value.
func variableKeySummary(variables []VariableObject) string {
	if len(variables) == 0 {
		return ""
	}
	parts := make([]string, 0, len(variables))
	for _, v := range variables {
		if v.VariableType == "" {
			parts = append(parts, v.Key)
			continue
		}
		parts = append(parts, fmt.Sprintf("%s (%s)", v.Key, v.VariableType))
	}
	return strings.Join(parts, ", ")
}

// inputNameSummary names the pipeline inputs a schedule carries, without their
// values, which are typed like the variables and shown on the same terms.
func inputNameSummary(inputs []InputObject) string {
	if len(inputs) == 0 {
		return ""
	}
	names := make([]string, 0, len(inputs))
	for _, in := range inputs {
		names = append(names, in.Name)
	}
	return strings.Join(names, ", ")
}

// FormatListMarkdown renders a page of pipeline schedules as a Markdown table.
//
// The table carries no link, so the footer carries no instruction to keep
// them, and the heading counts what the response can vouch for rather than a
// total keyset pagination never sends.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Schedules) == 0 {
		return toolutil.EmptyMessage("pipeline schedules")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Pipeline Schedules", len(out.Schedules), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Description", "Ref", "Cron", "Active", "Owner"))
	for _, s := range out.Schedules {
		owner := ""
		if s.Owner != nil {
			owner = toolutil.MdUserHandle(s.Owner.Username)
		}
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.Itoa(s.ID),
			toolutil.EscapeMdTableCell(s.Description),
			toolutil.EscapeMdTableCell(s.Ref),
			toolutil.MdCodeSpanCell(s.Cron),
			toolutil.BoolEmoji(s.Active),
			owner,
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction(hintActionScheduleGet, "see one schedule in full"),
		toolutil.HintAction(hintActionScheduleCreate, "add a schedule"),
	)
	return b.String()
}

// FormatVariableMarkdown renders one pipeline schedule variable as the card of
// a single object.
func FormatVariableMarkdown(v VariableOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Pipeline Schedule Variable")
	c.Field("Key", v.Key)
	c.Field("Value", v.Value)
	// A CI variable type is one of GitLab's fixed set (env_var, file).
	c.Field("Type", v.VariableType)
	c.End(
		toolutil.HintAction(hintActionScheduleEditVariable, "change this variable"),
		toolutil.HintAction(hintActionScheduleDeleteVariable, "remove it"),
	)
	return b.String()
}

// FormatTriggeredPipelinesMarkdown renders the pipelines a schedule triggered
// as a Markdown table.
func FormatTriggeredPipelinesMarkdown(out TriggeredPipelinesListOutput) string {
	if len(out.Pipelines) == 0 {
		return toolutil.EmptyMessage("triggered pipelines")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Triggered Pipelines", len(out.Pipelines), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "IID", "Ref", "Status", "Source"))
	for _, p := range out.Pipelines {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdTitleLink(fmt.Sprintf("#%d", p.ID), p.WebURL),
			strconv.Itoa(p.IID),
			toolutil.EscapeMdTableCell(p.Ref),
			pipelineStatusCell(p.Status),
			toolutil.EscapeMdTableCell(p.Source),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintAction(hintActionPipelineGet, "see one pipeline in full"),
		toolutil.HintAction(hintActionScheduleList, "see the schedules of this project"),
	)
	return b.String()
}

// pipelineStatusCell renders a pipeline status with the glyph every pipeline
// row in the tree shows, and nothing when GitLab sent no status.
func pipelineStatusCell(status string) string {
	if strings.TrimSpace(status) == "" {
		return ""
	}
	return toolutil.PipelineStatusEmoji(status) + " " + toolutil.EscapeMdTableCell(status)
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatVariableMarkdown)
	toolutil.RegisterMarkdown(FormatTriggeredPipelinesMarkdown)
}
