package jobs

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatOutputMarkdown renders one job as the card of a single object.
func FormatOutputMarkdown(j Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, jobHeading(j))
	writeJobDetail(c, j)
	c.End(jobHints(j)...)
	return b.String()
}

// jobHeading composes the card's heading: the status as a glyph, the job's ID
// and name, and the archived marker an archived job carries.
func jobHeading(j Output) string {
	heading := fmt.Sprintf("%s Job #%d: %s", toolutil.PipelineStatusEmoji(j.Status), j.ID, j.Name)
	if j.Archived {
		heading += " " + toolutil.EmojiArchived
	}
	return heading
}

// writeJobDetail writes the job's own rows onto the card it is given, so the
// wait result shows the same fields under its own H3 rather than embedding a
// second H2 and a second guidance section.
func writeJobDetail(c *toolutil.Card, j Output) {
	if j.Pipeline != nil && j.Pipeline.ID > 0 {
		c.Field("Pipeline", fmt.Sprintf("#%d", j.Pipeline.ID))
	}
	// A stage name is written in .gitlab-ci.yml, and GitLab constrains its
	// length rather than its characters.
	c.Field("Stage", j.Stage)
	c.Markdown("Status", jobStatusCell(j.Status))
	c.Bool("Allow Failure", j.AllowFailure)
	c.Field("Ref", j.Ref)
	c.Bool("Tag", j.Tag)
	if j.Commit != nil && j.Commit.ID != "" {
		c.Code("Commit", j.Commit.ID[:min(len(j.Commit.ID), 12)])
	}
	if j.Duration > 0 {
		c.Field("Duration", fmt.Sprintf("%.1fs", j.Duration))
	}
	if j.QueuedDuration > 0 {
		c.Field("Queued", fmt.Sprintf("%.1fs", j.QueuedDuration))
	}
	// GitLab stores the failure reason as an integer column behind a fixed
	// enum map and serializes the key, such as script_failure.
	c.Field("Failure Reason", j.FailureReason)
	if j.Coverage > 0 {
		c.Field("Coverage", fmt.Sprintf("%.1f%%", j.Coverage))
	}
	if j.User != nil {
		c.Markdown("User", toolutil.MdUserLink(j.User.Username, j.User.WebURL))
	}
	c.Time("Created", j.CreatedAt)
	// The three rows below decide what a reader may still do with the job:
	// an erased job has no log left, an archived one can be neither retried
	// nor cancelled, and an expiry says how long the artifacts remain.
	c.Time("Erased", j.ErasedAt)
	c.Time("Artifacts Expire", j.ArtifactsExpireAt)
	c.Flag(toolutil.EmojiArchived, "Archived", j.Archived)
	c.URL(j.WebURL)
}

// jobStatusCell renders a job status with its glyph, and nothing when GitLab
// sent no status.
func jobStatusCell(status string) string {
	if strings.TrimSpace(status) == "" {
		return ""
	}
	return toolutil.PipelineStatusEmoji(status) + " " + toolutil.EscapeMdTableCell(status)
}

// jobHints names only what the job's own state still allows: GitLab keeps no
// log for an erased job, and refuses both a retry and a cancel on an archived
// one, so offering those was advice that could only fail.
//
// Each condition replaces a hint rather than dropping one, so the card always
// closes with the same three next steps and a reader is never left with a
// state and nothing to do about it.
func jobHints(j Output) []string {
	log := toolutil.HintAction(actionJobTrace, "read this job's log")
	if j.ErasedAt != "" {
		log = toolutil.HintAction(actionJobListProject, "look at the project's other jobs, since this one's log was erased")
	}
	if j.Archived {
		return []string{
			log,
			toolutil.HintAction(actionPipelineGet, "open the pipeline this job ran in"),
			toolutil.HintAction(actionJobList, "see the other jobs of that pipeline"),
		}
	}
	return []string{
		log,
		toolutil.HintAction(actionJobRetry, "re-run this job"),
		toolutil.HintAction(actionJobCancel, "cancel it while it is still running"),
	}
}

// FormatListMarkdown renders a page of jobs as a Markdown table.
//
// The heading counts what the response can vouch for rather than
// Pagination.TotalItems alone, which keyset pagination never sends.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Jobs) == 0 {
		return toolutil.EmptyMessage("jobs")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Jobs", len(out.Jobs), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Stage", "Status", "Duration"))
	for _, j := range out.Jobs {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdTitleLink(fmt.Sprintf("#%d", j.ID), j.WebURL),
			jobNameCell(j),
			toolutil.EscapeMdTableCell(j.Stage),
			jobStatusCell(j.Status),
			fmt.Sprintf("%.1fs", j.Duration),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintAction(actionJobGet, "see one job in full"),
		toolutil.HintAction(actionJobTrace, "read a job's log"),
	)
	return b.String()
}

// jobNameCell carries the archived marker, without which a reader cannot tell
// an archived job — one GitLab will neither retry nor cancel — from a live one.
func jobNameCell(j Output) string {
	cell := toolutil.EscapeMdTableCell(j.Name)
	if j.Archived {
		cell += " " + toolutil.EmojiArchived
	}
	return cell
}

// FormatTraceMarkdown renders a job's log as the card of that job: the note
// says what was cut, then the log in a fence sized to its own content.
func FormatTraceMarkdown(t TraceOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Job #%d Trace", t.JobID))
	if t.Truncated {
		// Naming the end that is missing matters: the log is cut at the front
		// 100 KB, and a failure is almost always at the end of it, so a reader
		// told only "truncated" would look for the error in what is shown.
		c.Note(toolutil.EmojiWarning + " Showing the first 100 KB of the log. The end, where a failure usually appears, is not included.")
	}
	c.Fence("", "", t.Trace)
	c.End(
		toolutil.HintAction(actionJobGet, "see this job's details"),
		toolutil.HintAction(actionJobRetry, "re-run it"),
	)
	return b.String()
}

// FormatBridgeListMarkdown renders a page of bridge (trigger) jobs as a
// Markdown table, with the downstream pipeline linked.
func FormatBridgeListMarkdown(out BridgeListOutput) string {
	if len(out.Bridges) == 0 {
		return toolutil.EmptyMessage("bridge jobs")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Bridge Jobs", len(out.Bridges), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Stage", "Status", "Duration", "Downstream"))
	for _, br := range out.Bridges {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdTitleLink(fmt.Sprintf("#%d", br.ID), br.WebURL),
			toolutil.EscapeMdTableCell(br.Name),
			toolutil.EscapeMdTableCell(br.Stage),
			jobStatusCell(br.Status),
			fmt.Sprintf("%.1fs", br.Duration),
			downstreamCell(br),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintAction(actionPipelineGet, "open the downstream pipeline"),
	)
	return b.String()
}

// downstreamCell links the downstream pipeline a bridge triggered, and is
// empty when the bridge triggered none.
func downstreamCell(br BridgeOutput) string {
	if br.DownstreamPipeline == nil || br.DownstreamPipeline.ID == 0 {
		return ""
	}
	return toolutil.MdTitleLink(fmt.Sprintf("#%d", br.DownstreamPipeline.ID), br.DownstreamPipeline.WebURL)
}

// FormatArtifactsMarkdown renders an artifacts download as the card of one
// archive.
func FormatArtifactsMarkdown(out ArtifactsOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, artifactsHeading(out))
	c.Int("Size (bytes)", int64(out.Size))
	c.Warn("Truncated at 1 MB", out.Truncated)
	c.Note("The archive is base64-encoded; decode it to extract the files.")
	c.End(toolutil.HintAction(actionJobDownloadSingle, "fetch one file out of the archive instead"))
	return b.String()
}

// artifactsHeading names the job when the caller asked for one, since the
// by-ref download answers for a ref rather than for a job ID.
func artifactsHeading(out ArtifactsOutput) string {
	if out.JobID > 0 {
		return fmt.Sprintf("Job #%d Artifacts", out.JobID)
	}
	return "Artifacts"
}

// FormatSingleArtifactMarkdown renders one artifact file as the card of that
// file, its content in a fence sized to the content.
func FormatSingleArtifactMarkdown(out SingleArtifactOutput) string {
	var b strings.Builder
	// The artifact path is echoed from the caller's own argument: an entry
	// name in a zip, authored by whoever wrote the CI job.
	c := toolutil.NewCard(&b, singleArtifactHeading(out))
	c.Int("Size (bytes)", int64(out.Size))
	c.Warn("Truncated at 1 MB", out.Truncated)
	// The fence is sized to the content: an artifact carrying a fence marker
	// of its own must not close the block early.
	c.Fence("", "", out.Content)
	c.End(toolutil.HintAction(actionJobArtifacts, "download the whole artifacts archive"))
	return b.String()
}

func singleArtifactHeading(out SingleArtifactOutput) string {
	if out.JobID > 0 {
		return fmt.Sprintf("Job #%d: %s", out.JobID, out.ArtifactPath)
	}
	return out.ArtifactPath
}

// FormatWaitMarkdown renders the job wait result as the card of the job that
// was waited for: the wait's own rows, then the job's fields under one H3,
// and one guidance section at the end.
func FormatWaitMarkdown(out WaitOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, jobWaitHeading(out))
	// The poller writes this duration itself, with time.Duration.String over
	// time.Since, so it is the server's own text rather than GitLab's.
	c.Field("Waited", out.WaitedFor)
	c.Int("Polls", int64(out.PollCount))
	c.Field("Final Status", out.FinalStatus)
	c.Warn("Timed Out", out.TimedOut)
	writeJobDetail(c.Section("Job Details"), out.Job)
	c.End(jobWaitHints(out)...)
	return b.String()
}

// jobWaitHeading names the outcome: the timer glyph and the status still
// running when the wait gave up, and the final status otherwise.
func jobWaitHeading(out WaitOutput) string {
	if out.TimedOut {
		return fmt.Sprintf("⏰ Job #%d: Timed Out (current: %s)", out.Job.ID, out.Job.Status)
	}
	return fmt.Sprintf("%s Job #%d: %s", jobOutcomeEmoji(out.FinalStatus), out.Job.ID, out.FinalStatus)
}

// jobOutcomeEmoji is the glyph a terminal job state is shown by.
func jobOutcomeEmoji(finalStatus string) string {
	switch finalStatus {
	case "failed":
		return toolutil.EmojiCross
	case "canceled":
		return toolutil.EmojiProhibited
	default:
		return toolutil.EmojiSuccess
	}
}

// jobWaitHints names what a caller can do with the outcome the wait reached,
// and nothing when the job simply succeeded.
func jobWaitHints(out WaitOutput) []string {
	switch {
	case out.TimedOut:
		return []string{
			toolutil.HintAction(actionJobWait, "keep waiting for this job"),
			toolutil.HintAction(actionJobCancel, "abort it instead"),
		}
	case out.FinalStatus == "failed":
		return []string{
			toolutil.HintAction(actionJobTrace, "read the log for the failure"),
			toolutil.HintAction(actionJobRetry, "retry the job"),
		}
	default:
		return nil
	}
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
