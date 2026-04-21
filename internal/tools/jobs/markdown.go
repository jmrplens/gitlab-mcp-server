// markdown.go provides Markdown formatting functions for CI job MCP tool output.

package jobs

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown renders a single job as a Markdown summary.
func FormatOutputMarkdown(j Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s Job #%d: %s\n\n", toolutil.PipelineStatusEmoji(j.Status), j.ID, toolutil.EscapeMdHeading(j.Name))
	if j.PipelineID > 0 {
		fmt.Fprintf(&b, "- **Pipeline**: #%d\n", j.PipelineID)
	}
	fmt.Fprintf(&b, "- **Stage**: %s\n", j.Stage)
	fmt.Fprintf(&b, toolutil.FmtMdStatus, j.Status)
	if j.AllowFailure {
		b.WriteString("- **Allow Failure**: yes\n")
	}
	fmt.Fprintf(&b, "- **Ref**: %s\n", j.Ref)
	if j.CommitSHA != "" {
		fmt.Fprintf(&b, "- **Commit**: `%s`\n", j.CommitSHA[:min(len(j.CommitSHA), 12)])
	}
	if j.Duration > 0 {
		fmt.Fprintf(&b, "- **Duration**: %.1fs\n", j.Duration)
	}
	if j.QueuedDuration > 0 {
		fmt.Fprintf(&b, "- **Queued**: %.1fs\n", j.QueuedDuration)
	}
	if j.FailureReason != "" {
		fmt.Fprintf(&b, "- **Failure Reason**: %s\n", j.FailureReason)
	}
	if j.Coverage > 0 {
		fmt.Fprintf(&b, "- **Coverage**: %.1f%%\n", j.Coverage)
	}
	if j.UserUsername != "" {
		fmt.Fprintf(&b, "- **User**: %s\n", j.UserUsername)
	}
	if j.CreatedAt != "" {
		fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(j.CreatedAt))
	}
	fmt.Fprintf(&b, toolutil.FmtMdURL, j.WebURL)
	toolutil.WriteHints(&b,
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
		fmt.Fprintf(&b, "| [#%d](%s) | %s | %s | %s %s | %.1fs |\n",
			j.ID, j.WebURL, toolutil.EscapeMdTableCell(j.Name), toolutil.EscapeMdTableCell(j.Stage), toolutil.PipelineStatusEmoji(j.Status), j.Status, j.Duration)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
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
	b.WriteString("```\n")
	b.WriteString(t.Trace)
	b.WriteString(fmtCodeFenceEnd)
	toolutil.WriteHints(&b,
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
		if br.DownstreamPipeline > 0 {
			ds = fmt.Sprintf("#%d", br.DownstreamPipeline)
		}
		fmt.Fprintf(&b, "| %d | %s | %s | %s %s | %.1fs | %s |\n",
			br.ID, toolutil.EscapeMdTableCell(br.Name), toolutil.EscapeMdTableCell(br.Stage),
			toolutil.PipelineStatusEmoji(br.Status), br.Status, br.Duration, ds)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
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
	toolutil.WriteHints(&b,
		"Use `gitlab_job_download_single_artifact` to get a specific file",
	)
	return b.String()
}

// FormatSingleArtifactMarkdown renders a single artifact file content.
func FormatSingleArtifactMarkdown(out SingleArtifactOutput) string {
	var b strings.Builder
	if out.JobID > 0 {
		fmt.Fprintf(&b, "## Job #%d — %s\n\n", out.JobID, out.ArtifactPath)
	} else {
		fmt.Fprintf(&b, "## %s\n\n", out.ArtifactPath)
	}
	fmt.Fprintf(&b, "- **Size**: %d bytes\n", out.Size)
	if out.Truncated {
		b.WriteString("- " + toolutil.EmojiWarning + " **Truncated**: content exceeds 1MB limit\n")
	}
	b.WriteString(fmtCodeFenceEnd)
	b.WriteString(out.Content)
	b.WriteString(fmtCodeFenceEnd)
	toolutil.WriteHints(&b,
		"Use `gitlab_job_artifacts` to download the full artifacts archive",
	)
	return b.String()
}

// FormatWaitMarkdown renders the job wait result as a Markdown summary.
func FormatWaitMarkdown(out WaitOutput) string {
	var b strings.Builder
	if out.TimedOut {
		fmt.Fprintf(&b, "## ⏰ Job #%d: Timed Out (current: %s)\n\n", out.Job.ID, out.Job.Status)
	} else {
		var emoji string
		switch out.FinalStatus {
		case "failed":
			emoji = "❌"
		case "canceled":
			emoji = "🚫"
		default:
			emoji = "✅"
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
		toolutil.WriteHints(&b,
			"Job is still running — call gitlab_job_wait again to continue waiting",
			"Use gitlab_job_cancel to abort the job",
		)
	} else if out.FinalStatus == "failed" {
		toolutil.WriteHints(&b,
			"Use gitlab_job action 'trace' to see the job log for failure details",
			"Use gitlab_job action 'retry' to retry the failed job",
		)
	}
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatTraceMarkdown)
	toolutil.RegisterMarkdown(FormatBridgeListMarkdown)
	toolutil.RegisterMarkdown(FormatArtifactsMarkdown)
	toolutil.RegisterMarkdown(FormatSingleArtifactMarkdown)
	toolutil.RegisterMarkdown(FormatWaitMarkdown)
}
