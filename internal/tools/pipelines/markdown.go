// markdown.go provides Markdown formatting functions for pipeline MCP tool output.

package pipelines

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatListMarkdown renders a paginated list of pipelines as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Pipelines (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Pipelines), out.Pagination)
	if len(out.Pipelines) == 0 {
		b.WriteString("No pipelines found.\n")
		return b.String()
	}
	b.WriteString("| ID | Status | Source | Ref | SHA |\n")
	b.WriteString(toolutil.TblSep5Col)
	for _, p := range out.Pipelines {
		sha := p.SHA
		if len(sha) > 8 {
			sha = sha[:8]
		}
		fmt.Fprintf(&b, "| [#%d](%s) | %s %s | %s | %s | %s |\n",
			p.ID, p.WebURL, toolutil.PipelineStatusEmoji(p.Status), p.Status, toolutil.EscapeMdTableCell(p.Source), toolutil.EscapeMdTableCell(p.Ref), sha)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use action 'get' with a pipeline_id for full details",
		"Use gitlab_job action 'list' with pipeline_id to see jobs",
	)
	return b.String()
}

// FormatDetailMarkdown renders a single pipeline detail as a Markdown summary.
func FormatDetailMarkdown(p DetailOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s Pipeline #%d: %s\n\n", toolutil.PipelineStatusEmoji(p.Status), p.ID, p.Status)
	fmt.Fprintf(&b, "- **IID**: %d\n", p.IID)
	fmt.Fprintf(&b, "- **Source**: %s\n", p.Source)
	fmt.Fprintf(&b, "- **Ref**: %s (tag: %v)\n", p.Ref, p.Tag)
	fmt.Fprintf(&b, "- **SHA**: %s\n", p.SHA)
	if p.BeforeSHA != "" {
		fmt.Fprintf(&b, "- **Before SHA**: %s\n", p.BeforeSHA)
	}
	if p.Name != "" {
		fmt.Fprintf(&b, toolutil.FmtMdName, p.Name)
	}
	if p.Duration > 0 {
		fmt.Fprintf(&b, "- **Duration**: %ds\n", p.Duration)
	}
	if p.QueuedDuration > 0 {
		fmt.Fprintf(&b, "- **Queued**: %ds\n", p.QueuedDuration)
	}
	if p.Coverage != "" {
		fmt.Fprintf(&b, "- **Coverage**: %s%%\n", p.Coverage)
	}
	if p.YamlErrors != "" {
		fmt.Fprintf(&b, "- **YAML Errors**: %s\n", p.YamlErrors)
	}
	if p.UserUsername != "" {
		fmt.Fprintf(&b, "- **User**: %s\n", p.UserUsername)
	}
	if p.CreatedAt != "" {
		fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(p.CreatedAt))
	}
	if p.StartedAt != "" {
		fmt.Fprintf(&b, "- **Started**: %s\n", toolutil.FormatTime(p.StartedAt))
	}
	if p.FinishedAt != "" {
		fmt.Fprintf(&b, "- **Finished**: %s\n", toolutil.FormatTime(p.FinishedAt))
	}
	fmt.Fprintf(&b, toolutil.FmtMdURL, p.WebURL)
	toolutil.WriteHints(&b,
		"Use gitlab_job action 'list' with this pipeline_id to see all jobs",
		"Use action 'variables' to see pipeline variables",
		"Use action 'test_report' to see test results",
	)
	return b.String()
}

// FormatVariablesMarkdown renders pipeline variables as a Markdown table.
func FormatVariablesMarkdown(out VariablesOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Pipeline Variables (%d)\n\n", len(out.Variables))
	if len(out.Variables) == 0 {
		b.WriteString("No pipeline variables found.\n")
		return b.String()
	}
	b.WriteString("| Key | Value | Type |\n")
	b.WriteString(toolutil.TblSep3Col)
	for _, v := range out.Variables {
		fmt.Fprintf(&b, "| %s | %s | %s |\n",
			toolutil.EscapeMdTableCell(v.Key), toolutil.EscapeMdTableCell(v.Value), v.VariableType)
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_pipeline_get` to view pipeline details",
	)
	return b.String()
}

// FormatTestReportMarkdown renders a pipeline test report as Markdown.
func FormatTestReportMarkdown(out TestReportOutput) string {
	var b strings.Builder
	b.WriteString("## Pipeline Test Report\n\n")
	fmt.Fprintf(&b, "- **Total**: %d tests (%.2fs)\n", out.TotalCount, out.TotalTime)
	fmt.Fprintf(&b, "- **Passed**: %d | **Failed**: %d | **Skipped**: %d | **Errors**: %d\n\n",
		out.SuccessCount, out.FailedCount, out.SkippedCount, out.ErrorCount)
	if len(out.TestSuites) > 0 {
		b.WriteString("### Test Suites\n\n")
		b.WriteString("| Suite | Total | Passed | Failed | Skipped | Errors | Time |\n")
		b.WriteString("| --- | --- | --- | --- | --- | --- | --- |\n")
		for _, s := range out.TestSuites {
			fmt.Fprintf(&b, "| %s | %d | %d | %d | %d | %d | %.2fs |\n",
				toolutil.EscapeMdTableCell(s.Name), s.TotalCount, s.SuccessCount, s.FailedCount, s.SkippedCount, s.ErrorCount, s.TotalTime)
		}
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_job_list` to see individual job results",
		"Use `gitlab_job_trace` to view job logs for failures",
	)
	return b.String()
}

// FormatTestReportSummaryMarkdown renders a pipeline test report summary as Markdown.
func FormatTestReportSummaryMarkdown(out TestReportSummaryOutput) string {
	var b strings.Builder
	b.WriteString("## Pipeline Test Report Summary\n\n")
	fmt.Fprintf(&b, "- **Total**: %d tests (%.2fs)\n", out.TotalCount, out.TotalTime)
	fmt.Fprintf(&b, "- **Passed**: %d | **Failed**: %d | **Skipped**: %d | **Errors**: %d\n\n",
		out.SuccessCount, out.FailedCount, out.SkippedCount, out.ErrorCount)
	if len(out.TestSuites) > 0 {
		b.WriteString("### Test Suites\n\n")
		b.WriteString("| Suite | Total | Passed | Failed | Skipped | Errors | Time |\n")
		b.WriteString("| --- | --- | --- | --- | --- | --- | --- |\n")
		for _, s := range out.TestSuites {
			fmt.Fprintf(&b, "| %s | %d | %d | %d | %d | %d | %.2fs |\n",
				toolutil.EscapeMdTableCell(s.Name), s.TotalCount, s.SuccessCount, s.FailedCount, s.SkippedCount, s.ErrorCount, s.TotalTime)
		}
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_pipeline_test_report` for full test details",
		"Use `gitlab_job_list` to investigate failures",
	)
	return b.String()
}

// FormatWaitMarkdown renders the wait result as a Markdown summary.
func FormatWaitMarkdown(out WaitOutput) string {
	var b strings.Builder
	if out.TimedOut {
		fmt.Fprintf(&b, "## ⏰ Pipeline #%d: Timed Out (current: %s)\n\n", out.Pipeline.ID, out.Pipeline.Status)
	} else {
		fmt.Fprintf(&b, "## %s Pipeline #%d: %s\n\n", toolutil.PipelineStatusEmoji(out.FinalStatus), out.Pipeline.ID, out.FinalStatus)
	}
	fmt.Fprintf(&b, "- **Waited**: %s (%d polls)\n", out.WaitedFor, out.PollCount)
	fmt.Fprintf(&b, "- **Final Status**: %s\n", out.FinalStatus)
	if out.TimedOut {
		b.WriteString("- **Timed Out**: yes\n")
	}
	b.WriteString("\n### Pipeline Details\n\n")
	b.WriteString(FormatDetailMarkdown(out.Pipeline))
	if out.TimedOut {
		toolutil.WriteHints(&b,
			"Pipeline is still running — call gitlab_pipeline_wait again to continue waiting",
			"Use gitlab_pipeline_cancel to abort the pipeline",
		)
	} else if out.FinalStatus == "failed" {
		toolutil.WriteHints(&b,
			"Use gitlab_job action 'list' with scope 'failed' to find failed jobs",
			"Use gitlab_pipeline_retry to retry failed jobs",
		)
	}
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatDetailMarkdown)
	toolutil.RegisterMarkdown(FormatVariablesMarkdown)
	toolutil.RegisterMarkdown(FormatTestReportMarkdown)
	toolutil.RegisterMarkdown(FormatTestReportSummaryMarkdown)
	toolutil.RegisterMarkdown(FormatWaitMarkdown)
}
