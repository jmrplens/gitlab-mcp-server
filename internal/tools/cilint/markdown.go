package cilint

import (
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name.
const (
	actionTemplateLint        = "template.lint"
	actionTemplateLintProject = "template.lint_project"
)

// FormatOutputMarkdown renders a CI lint result as the card of one object: the
// verdict, then the errors, warnings, includes and jobs as the nested
// collections they are, then the merged YAML in a fence.
//
// The messages are a collection and are rendered as table rows rather than as
// bare list items: a lint message quotes the user's own CI file back, and a
// message beginning with '#' or '-' opened a heading or a nested list where it
// was written as the content of a list item.
func FormatOutputMarkdown(v Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, lintHeading(v))
	if isEmptyResult(v) {
		c.Note("The configuration is valid, with no errors, warnings, includes or jobs.")
		c.End()
		return b.String()
	}
	writeMessages(c, "Errors", v.Errors)
	writeMessages(c, "Warnings", v.Warnings)
	writeIncludes(c, v.Includes)
	writeLintJobs(c, v.Jobs)
	c.Fence("Merged YAML", "yaml", v.MergedYaml)
	c.End(lintHints(v)...)
	return b.String()
}

// lintHeading states the verdict the whole result turns on.
func lintHeading(v Output) string {
	if v.Valid {
		return "CI Lint: " + toolutil.BoolEmoji(true) + " Valid"
	}
	return "CI Lint: " + toolutil.BoolEmoji(false) + " Invalid"
}

// isEmptyResult reports whether GitLab accepted the configuration and said
// nothing else about it.
func isEmptyResult(v Output) bool {
	return v.Valid && len(v.Errors) == 0 && len(v.Warnings) == 0 &&
		len(v.Includes) == 0 && len(v.Jobs) == 0 && v.MergedYaml == ""
}

// writeMessages renders one list of lint messages as a nested collection.
func writeMessages(c *toolutil.Card, title string, messages []string) {
	if len(messages) == 0 {
		return
	}
	table := c.Table(title, "Message")
	for _, message := range messages {
		table.Row(toolutil.EscapeMdTableCell(message))
	}
}

// writeIncludes renders the files the configuration pulls in.
func writeIncludes(c *toolutil.Card, includes []Include) {
	if len(includes) == 0 {
		return
	}
	table := c.Table("Includes", "Type", "Location", "Context Project")
	for _, inc := range includes {
		table.Row(
			toolutil.EscapeMdTableCell(inc.Type),
			toolutil.EscapeMdTableCell(inc.Location),
			toolutil.EscapeMdTableCell(inc.ContextProject),
		)
	}
}

// writeLintJobs renders the jobs the configuration expands into, which GitLab
// sends when include_jobs is set and which this result carried without ever
// showing: the expansion is the answer to "what will this pipeline run".
func writeLintJobs(c *toolutil.Card, jobs []toolutil.LintJobOutput) {
	if len(jobs) == 0 {
		return
	}
	table := c.Table("Jobs", "Name", "Stage", "When", "Allow Failure", "Tags")
	for _, job := range jobs {
		table.Row(
			toolutil.EscapeMdTableCell(job.Name),
			toolutil.EscapeMdTableCell(job.Stage),
			toolutil.EscapeMdTableCell(job.When),
			toolutil.BoolEmoji(job.AllowFailure),
			toolutil.EscapeMdTableCell(strings.Join(job.TagList, ", ")),
		)
	}
}

// lintHints tells a reader what to do next, and says nothing about fixing
// errors when GitLab reported none.
func lintHints(v Output) []string {
	if len(v.Errors) > 0 || len(v.Warnings) > 0 {
		return []string{
			"Fix the reported errors and warnings before committing this CI configuration",
			toolutil.HintAction(actionTemplateLint, "check the corrected configuration again"),
		}
	}
	return []string{
		toolutil.HintAction(actionTemplateLintProject, "lint the configuration committed in a project"),
		toolutil.HintAction(actionPipelineCreate, "run a pipeline with it"),
	}
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
}
