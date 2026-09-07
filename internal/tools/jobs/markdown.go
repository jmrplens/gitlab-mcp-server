package jobs

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// FormatOutputMarkdown renders a single job as a Markdown summary.
func FormatOutputMarkdown(j Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s Job #%d: %s\n\n", toolutil.PipelineStatusEmoji(j.Status), j.ID, toolutil.EscapeMdHeading(j.Name))
	if j.Pipeline != nil && j.Pipeline.ID > 0 {
		fmt.Fprintf(&b, "- **Pipeline**: #%d\n", j.Pipeline.ID)
	}
	// A stage name is written in .gitlab-ci.yml, and GitLab constrains its
	// length rather than its characters; the jobs tables already escape it.
	fmt.Fprintf(&b, "- **Stage**: %s\n", toolutil.EscapeMdTableCell(j.Stage))
	//gitlab:allow-unescaped j.Status: a job status, GitLab's own build state (created, running, success, failed and the rest).
	fmt.Fprintf(&b, toolutil.FmtMdStatus, j.Status)
	if j.AllowFailure {
		b.WriteString("- **Allow Failure**: yes\n")
	}
	fmt.Fprintf(&b, "- **Ref**: %s\n", toolutil.EscapeMdTableCell(j.Ref))
	if j.Commit != nil && j.Commit.ID != "" {
		// Named rather than sliced inline so the exemption can refer to it: a
		// directive's expression is cut at its first colon.
		shortID := j.Commit.ID[:min(len(j.Commit.ID), 12)]
		//gitlab:allow-unescaped shortID: the first twelve characters of a git object id, which is hexadecimal.
		fmt.Fprintf(&b, "- **Commit**: `%s`\n", shortID)
	}
	if j.Duration > 0 {
		fmt.Fprintf(&b, "- **Duration**: %.1fs\n", j.Duration)
	}
	if j.QueuedDuration > 0 {
		fmt.Fprintf(&b, "- **Queued**: %.1fs\n", j.QueuedDuration)
	}
	if j.FailureReason != "" {
		//gitlab:allow-unescaped j.FailureReason: GitLab stores the failure reason as an integer column behind a fixed enum map and serializes the key, such as script_failure.
		fmt.Fprintf(&b, "- **Failure Reason**: %s\n", j.FailureReason)
	}
	if j.Coverage > 0 {
		fmt.Fprintf(&b, "- **Coverage**: %.1f%%\n", j.Coverage)
	}
	if j.User != nil && j.User.Username != "" {
		fmt.Fprintf(&b, "- **User**: %s\n", toolutil.EscapeMdTableCell(j.User.Username))
	}
	if j.CreatedAt != "" {
		fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(j.CreatedAt))
	}
	toolutil.WriteMdURL(&b, j.WebURL)
	toolutil.WriteHints(
		&b,
		"Use action 'trace' to view the full job log output",
		"Use action 'retry' to re-run this job",
		"Use action 'cancel' to cancel a running job",
	)
	return b.String()
}

// FormatListMarkdown renders a paginated list of jobs as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Jobs (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Jobs), out.Pagination)
	if len(out.Jobs) == 0 {
		b.WriteString("No jobs found.\n")
		return b.String()
	}
	b.WriteString("| ID | Name | Stage | Status | Duration |\n")
	b.WriteString(toolutil.TblSep5Col)
	for _, j := range out.Jobs {
		fmt.Fprintf(&b, "| %s | %s | %s | %s %s | %.1fs |\n",
			toolutil.MdTitleLink(fmt.Sprintf("#%d", j.ID), j.WebURL), toolutil.EscapeMdTableCell(j.Name), toolutil.EscapeMdTableCell(j.Stage), toolutil.PipelineStatusEmoji(j.Status), j.Status, j.Duration)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(
		&b,
		toolutil.HintPreserveLinks,
		"Use action 'get' with a job_id to see job details",
		"Use action 'trace' to view job log output",
	)
	return b.String()
}

// FormatTraceMarkdown renders a job trace log in a code fence.
func FormatTraceMarkdown(t TraceOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Job #%d Trace\n\n", t.JobID)
	if t.Truncated {
		b.WriteString(toolutil.EmojiWarning + " *Trace truncated at 100KB.*\n\n")
	}
	b.WriteString(toolutil.MarkdownFencedBlock("", t.Trace))
	toolutil.WriteHints(
		&b,
		"Use `gitlab_job_get` to see job details",
		"Use `gitlab_job_retry` to retry this job",
	)
	return b.String()
}

// FormatBridgeListMarkdown renders a paginated list of bridge jobs as a Markdown table.
func FormatBridgeListMarkdown(out BridgeListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Bridge Jobs (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Bridges), out.Pagination)
	if len(out.Bridges) == 0 {
		b.WriteString("No bridge jobs found.\n")
		return b.String()
	}
	b.WriteString("| ID | Name | Stage | Status | Duration | Downstream |\n")
	b.WriteString(toolutil.TblSep6Col)
	for _, br := range out.Bridges {
		ds := ""
		if br.DownstreamPipeline != nil && br.DownstreamPipeline.ID > 0 {
			ds = fmt.Sprintf("#%d", br.DownstreamPipeline.ID)
		}
		fmt.Fprintf(&b, "| %d | %s | %s | %s %s | %.1fs | %s |\n",
			br.ID, toolutil.EscapeMdTableCell(br.Name), toolutil.EscapeMdTableCell(br.Stage),
			//gitlab:allow-unescaped br.Status: a bridge carries the same job status enum a job does.
			toolutil.PipelineStatusEmoji(br.Status), br.Status, br.Duration, ds)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(
		&b,
		"Use `gitlab_pipeline_get` to view the downstream pipeline",
	)
	return b.String()
}

// FormatArtifactsMarkdown renders artifact download info.
func FormatArtifactsMarkdown(out ArtifactsOutput) string {
	var b strings.Builder
	if out.JobID > 0 {
		fmt.Fprintf(&b, "## Job #%d Artifacts\n\n", out.JobID)
	} else {
		b.WriteString("## Artifacts\n\n")
	}
	fmt.Fprintf(&b, "- **Size**: %d bytes\n", out.Size)
	if out.Truncated {
		b.WriteString("- " + toolutil.EmojiWarning + " **Truncated**: content exceeds 1MB limit\n")
	}
	b.WriteString("- **Content**: base64-encoded archive (use a decoder to extract)\n")
	toolutil.WriteHints(
		&b,
		"Use `gitlab_job_download_single_artifact` to get a specific file",
	)
	return b.String()
}

// FormatSingleArtifactMarkdown renders a single artifact file content.
func FormatSingleArtifactMarkdown(out SingleArtifactOutput) string {
	var b strings.Builder
	if out.JobID > 0 {
		// The artifact path is echoed from the caller's own argument: an entry
		// name in a zip, authored by whoever wrote the CI job.
		fmt.Fprintf(&b, "## Job #%d: %s\n\n", out.JobID, toolutil.EscapeMdHeading(out.ArtifactPath))
	} else {
		fmt.Fprintf(&b, "## %s\n\n", toolutil.EscapeMdHeading(out.ArtifactPath))
	}
	fmt.Fprintf(&b, "- **Size**: %d bytes\n", out.Size)
	if out.Truncated {
		b.WriteString("- " + toolutil.EmojiWarning + " **Truncated**: content exceeds 1MB limit\n")
	}
	// The fence is sized to the content: an artifact that contains a fence
	// marker of its own must not close the block early.
	b.WriteString("\n" + toolutil.MarkdownFencedBlock("", out.Content))
	toolutil.WriteHints(
		&b,
		"Use `gitlab_job_artifacts` to download the full artifacts archive",
	)
	return b.String()
}

// FormatWaitMarkdown renders the job wait result as a Markdown summary.
func FormatWaitMarkdown(out WaitOutput) string {
	var b strings.Builder
	if out.TimedOut {
		//gitlab:allow-unescaped out.Job.Status: the polled job's status is the same GitLab enum a job's own status is.
		//gitlab:allow-unescaped out.FinalStatus: the status the poller last read off the job, so the same GitLab enum.
		//gitlab:allow-unescaped out.WaitedFor: the poller writes this duration itself, with time.Duration.String over time.Since.
		fmt.Fprintf(&b, "## \u23F0 Job #%d: Timed Out (current: %s)\n\n", out.Job.ID, out.Job.Status)
	} else {
		var emoji string
		switch out.FinalStatus {
		case "failed":
			emoji = toolutil.EmojiCross
		case "canceled":
			emoji = toolutil.EmojiProhibited
		default:
			emoji = toolutil.EmojiSuccess
		}
		fmt.Fprintf(&b, "## %s Job #%d: %s\n\n", emoji, out.Job.ID, out.FinalStatus)
	}
	fmt.Fprintf(&b, "- **Waited**: %s (%d polls)\n", out.WaitedFor, out.PollCount)
	fmt.Fprintf(&b, "- **Final Status**: %s\n", out.FinalStatus)
	if out.TimedOut {
		b.WriteString("- **Timed Out**: yes\n")
	}
	b.WriteString("\n### Job Details\n\n")
	b.WriteString(FormatOutputMarkdown(out.Job))
	if out.TimedOut {
		toolutil.WriteHints(
			&b,
			"Job is still running. Call gitlab_job_wait again to continue waiting",
			"Use gitlab_job_cancel to abort the job",
		)
	} else if out.FinalStatus == "failed" {
		toolutil.WriteHints(
			&b,
			"Use gitlab_job action 'trace' to see the job log for failure details",
			"Use gitlab_job action 'retry' to retry the failed job",
		)
	}
	return b.String()
}

// formatWaitResult wraps the Markdown output of [FormatWaitMarkdown] in
// an [mcp.CallToolResult], marking the result as an error when the wait
// timed out so callers can branch on the outcome.
func formatWaitResult(out WaitOutput) *mcp.CallToolResult {
	result := toolutil.ToolResultAnnotated(FormatWaitMarkdown(out), toolutil.ContentDetail)
	if out.TimedOut {
		result.IsError = true
	}
	return result
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatTraceMarkdown)
	toolutil.RegisterMarkdown(FormatBridgeListMarkdown)
	toolutil.RegisterMarkdown(FormatArtifactsMarkdown)
	toolutil.RegisterMarkdown(FormatSingleArtifactMarkdown)
	toolutil.RegisterMarkdownResult(formatWaitResult)
}
