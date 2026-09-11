package pipelines

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name, beside the ones action_specs.go
// already declares. A hint names the ID every surface resolves, so it is never
// a tool name the serving surface does not register.
const (
	actionPipelineVariables  = "pipeline.variables"
	actionPipelineTestReport = "pipeline.test_report"
	actionPipelineWait       = "pipeline.wait"
	actionJobList            = "job.list"
	actionJobTrace           = "job.trace"
)

type pipelineNotFoundOutput struct {
	Identifier string
}

func formatPipelineNotFound(out pipelineNotFoundOutput) *mcp.CallToolResult {
	return toolutil.NotFoundResult(
		"Pipeline", out.Identifier,
		"Use gitlab_pipeline_list with project_id to list pipelines",
		"Verify the pipeline_id is correct for this project",
	)
}

// FormatListMarkdown renders a page of pipelines as a Markdown table: a
// collection of objects that share columns.
//
// The heading counts what the response can vouch for rather than
// Pagination.TotalItems alone, which keyset pagination never sends: "Pipelines
// (0)" above a table of rows was what a reader saw on every keyset page.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Pipelines) == 0 {
		return toolutil.EmptyMessage("pipelines")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Pipelines", len(out.Pipelines), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Status", "Source", "Ref", "SHA"))
	for _, p := range out.Pipelines {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdTitleLink(fmt.Sprintf("#%d", p.ID), p.WebURL),
			pipelineStatusCell(p.Status),
			toolutil.EscapeMdTableCell(p.Source),
			toolutil.EscapeMdTableCell(p.Ref),
			toolutil.EscapeMdTableCell(shortSHA(p.SHA)),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintAction(actionPipelineGet, "see one pipeline in full"),
		toolutil.HintAction(actionJobList, "see the jobs of a pipeline"),
	)
	return b.String()
}

// pipelineStatusCell renders a pipeline status with its glyph, and nothing
// when GitLab sent no status.
func pipelineStatusCell(status string) string {
	if strings.TrimSpace(status) == "" {
		return ""
	}
	return toolutil.PipelineStatusEmoji(status) + " " + toolutil.EscapeMdTableCell(status)
}

// shortSHA is the abbreviation a commit SHA is shown by in a table cell.
func shortSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

// FormatDetailMarkdown renders one pipeline as the card of a single object.
func FormatDetailMarkdown(p DetailOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, pipelineHeading(p))
	writePipelineDetail(c, p)
	c.End(
		toolutil.HintAction(actionJobList, "see the jobs of this pipeline"),
		toolutil.HintAction(actionPipelineVariables, "see the variables it ran with"),
		toolutil.HintAction(actionPipelineTestReport, "see its test results"),
	)
	return b.String()
}

// pipelineHeading composes the card's heading: the status as a glyph, the
// pipeline's ID and status, and the archived marker an archived pipeline
// carries. Without that marker a reader cannot tell an archived pipeline from
// a live one, and an archived pipeline can neither be retried nor cancelled.
func pipelineHeading(p DetailOutput) string {
	heading := fmt.Sprintf("%s Pipeline #%d: %s", toolutil.PipelineStatusEmoji(p.Status), p.ID, p.Status)
	if p.Archived {
		heading += " " + toolutil.EmojiArchived
	}
	return heading
}

// writePipelineDetail writes the pipeline's own rows onto the card it is
// given, so the wait result can show the same fields under its own H3 instead
// of embedding a second H2 and a second guidance section.
func writePipelineDetail(c *toolutil.Card, p DetailOutput) {
	c.Int("IID", p.IID)
	c.Field("Source", p.Source)
	// A ref is not an identifier: git check-ref-format permits '|', '<' and
	// '>', so a pushable branch could end this row or open a tag in it.
	c.Field("Ref", p.Ref)
	c.Bool("Tag", p.Tag)
	c.Code("SHA", p.SHA)
	c.Code("Before SHA", p.BeforeSHA)
	// The pipeline name comes from workflow:name in .gitlab-ci.yml, which is
	// free text and interpolates CI variables besides.
	c.Field("Name", p.Name)
	c.Field("Detailed Status", detailedStatusLabel(p))
	c.Flag(toolutil.EmojiArchived, "Archived", p.Archived)
	if p.Duration > 0 {
		c.Field("Duration", fmt.Sprintf("%ds", p.Duration))
	}
	if p.QueuedDuration > 0 {
		c.Field("Queued", fmt.Sprintf("%ds", p.QueuedDuration))
	}
	if p.Coverage != "" {
		c.Field("Coverage", p.Coverage+"%")
	}
	// GitLab quotes the user's own CI file back in this message, job and stage
	// names included, and a Psych parse error carries "(<unknown>)".
	c.Text("YAML Errors", p.YamlErrors)
	if p.User != nil {
		c.Markdown("User", toolutil.MdUserLink(p.User.Username, p.User.WebURL))
	}
	c.Time("Created", p.CreatedAt)
	c.Time("Started", p.StartedAt)
	c.Time("Finished", p.FinishedAt)
	c.URL(p.WebURL)
}

// detailedStatusLabel is the human label GitLab computes for a pipeline's
// status, shown only when it says more than the status word already on the
// card: GitLab sends "passed" for a successful pipeline and "failed (allowed
// to fail)" for one whose only failures were allowed, which the bare status
// cannot express.
func detailedStatusLabel(p DetailOutput) string {
	if p.DetailedStatus == nil {
		return ""
	}
	label := strings.TrimSpace(p.DetailedStatus.Label)
	if label == "" || strings.EqualFold(label, p.Status) {
		return ""
	}
	return label
}

// FormatVariablesMarkdown renders a pipeline's variables as a Markdown table.
func FormatVariablesMarkdown(out VariablesOutput) string {
	if len(out.Variables) == 0 {
		return toolutil.EmptyMessage("pipeline variables")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Pipeline Variables", len(out.Variables), toolutil.PaginationOutput{})
	b.WriteString(toolutil.MarkdownTableHeader("Key", "Value", "Type"))
	for _, v := range out.Variables {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.EscapeMdTableCell(v.Key),
			toolutil.EscapeMdTableCell(v.Value),
			toolutil.EscapeMdTableCell(v.VariableType),
		))
	}
	toolutil.WriteListFooter(&b, toolutil.PaginationOutput{}, false,
		toolutil.HintAction(actionPipelineGet, "see the pipeline these variables ran"),
	)
	return b.String()
}

// FormatTestReportMarkdown renders a pipeline's test report as the card of one
// object: the totals as rows, the suites as the nested collection they are.
//
// The report carries no individual test case, so the hints name the jobs and
// their logs rather than promising per-test detail this rendering cannot show.
func FormatTestReportMarkdown(out TestReportOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Pipeline Test Report")
	writeTestReportTotals(c, out.TotalCount, out.TotalTime, out.SuccessCount, out.FailedCount, out.SkippedCount, out.ErrorCount)
	writeTestSuites(c, testSuiteOutputs(out.TestSuites))
	c.End(
		toolutil.HintAction(actionJobList, "see the jobs these suites ran in"),
		toolutil.HintAction(actionJobTrace, "read the log of a failing job"),
	)
	return b.String()
}

// FormatTestReportSummaryMarkdown renders a pipeline's test report summary as
// the card of one object.
//
// The full report shows the same per-suite totals and no test cases, so the
// hint says that rather than offering "full test details".
func FormatTestReportSummaryMarkdown(out TestReportSummaryOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Pipeline Test Report Summary")
	writeTestReportTotals(c, out.TotalCount, out.TotalTime, out.SuccessCount, out.FailedCount, out.SkippedCount, out.ErrorCount)
	writeTestSuites(c, testSuiteSummaryOutputs(out.TestSuites))
	c.End(
		toolutil.HintAction(actionPipelineTestReport, "see the same per-suite totals in the full report"),
		toolutil.HintAction(actionJobList, "investigate failures job by job"),
	)
	return b.String()
}

type testSuiteMarkdown struct {
	Name         string
	TotalTime    float64
	TotalCount   int64
	SuccessCount int64
	FailedCount  int64
	SkippedCount int64
	ErrorCount   int64
}

// writeTestReportTotals writes one row per count. The four counts used to
// share a line separated by pipes, which is a table row anywhere a renderer
// looks for one and four values a reader has to parse out of one item.
func writeTestReportTotals(c *toolutil.Card, totalCount int64, totalTime float64, successCount, failedCount, skippedCount, errorCount int64) {
	c.Int("Total", totalCount)
	c.Field("Time", fmt.Sprintf("%.2fs", totalTime))
	c.Int("Passed", successCount)
	c.Int("Failed", failedCount)
	c.Int("Skipped", skippedCount)
	c.Int("Errors", errorCount)
}

// writeTestSuites writes the suites as the nested collection of a card.
func writeTestSuites(c *toolutil.Card, suites []testSuiteMarkdown) {
	if len(suites) == 0 {
		return
	}
	table := c.Table("Test Suites", "Suite", "Total", "Passed", "Failed", "Skipped", "Errors", "Time")
	for _, suite := range suites {
		table.Row(
			toolutil.EscapeMdTableCell(suite.Name),
			strconv.FormatInt(suite.TotalCount, 10),
			strconv.FormatInt(suite.SuccessCount, 10),
			strconv.FormatInt(suite.FailedCount, 10),
			strconv.FormatInt(suite.SkippedCount, 10),
			strconv.FormatInt(suite.ErrorCount, 10),
			fmt.Sprintf("%.2fs", suite.TotalTime),
		)
	}
}

func testSuiteOutputs(suites []TestSuiteOutput) []testSuiteMarkdown {
	out := make([]testSuiteMarkdown, 0, len(suites))
	for _, suite := range suites {
		out = append(out, testSuiteMarkdown(suite))
	}
	return out
}

func testSuiteSummaryOutputs(suites []TestSuiteSummaryOutput) []testSuiteMarkdown {
	out := make([]testSuiteMarkdown, 0, len(suites))
	for _, suite := range suites {
		out = append(out, testSuiteMarkdown{
			Name:         suite.Name,
			TotalTime:    suite.TotalTime,
			TotalCount:   suite.TotalCount,
			SuccessCount: suite.SuccessCount,
			FailedCount:  suite.FailedCount,
			SkippedCount: suite.SkippedCount,
			ErrorCount:   suite.ErrorCount,
		})
	}
	return out
}

// FormatWaitMarkdown renders the wait result as the card of the pipeline that
// was waited for: the wait's own rows, then the pipeline's fields under one
// H3, and one guidance section at the end.
//
// The pipeline used to be embedded by calling [FormatDetailMarkdown], which
// wrote a second H2 under the H3 and a guidance section in the middle of the
// response, where ExtractHints never looked.
func FormatWaitMarkdown(out WaitOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, waitHeading(out))
	// The poller writes this duration itself, with time.Duration.String over
	// time.Since, so it is the server's own text rather than GitLab's.
	c.Field("Waited", out.WaitedFor)
	c.Int("Polls", int64(out.PollCount))
	c.Field("Final Status", out.FinalStatus)
	c.Warn("Timed Out", out.TimedOut)
	writePipelineDetail(c.Section("Pipeline Details"), out.Pipeline)
	c.End(waitHints(out)...)
	return b.String()
}

// waitHeading names the outcome: the timer glyph and the status still running
// when the wait gave up, and the pipeline's own status glyph otherwise.
func waitHeading(out WaitOutput) string {
	if out.TimedOut {
		return fmt.Sprintf("⏰ Pipeline #%d: Timed Out (current: %s)", out.Pipeline.ID, out.Pipeline.Status)
	}
	return fmt.Sprintf("%s Pipeline #%d: %s", toolutil.PipelineStatusEmoji(out.FinalStatus), out.Pipeline.ID, out.FinalStatus)
}

// waitHints names what a caller can do next with the outcome the wait
// reached, and nothing when the pipeline simply succeeded.
func waitHints(out WaitOutput) []string {
	switch {
	case out.TimedOut:
		return []string{
			toolutil.HintAction(actionPipelineWait, "keep waiting for this pipeline"),
			toolutil.HintAction(actionPipelineCancel, "abort it instead"),
		}
	case out.FinalStatus == "failed":
		return []string{
			toolutil.HintAction(actionJobList, "find the jobs that failed"),
			toolutil.HintAction(actionPipelineRetry, "retry the failed jobs"),
		}
	default:
		return nil
	}
}

func formatWaitResult(out WaitOutput) *mcp.CallToolResult {
	result := toolutil.ToolResultAnnotated(FormatWaitMarkdown(out), toolutil.ContentDetail)
	if out.TimedOut {
		result.IsError = true
	}
	return result
}

func init() {
	toolutil.RegisterMarkdownResult(formatPipelineNotFound)
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatDetailMarkdown)
	toolutil.RegisterMarkdown(FormatVariablesMarkdown)
	toolutil.RegisterMarkdown(FormatTestReportMarkdown)
	toolutil.RegisterMarkdown(FormatTestReportSummaryMarkdown)
	toolutil.RegisterMarkdownResult(formatWaitResult)
}
