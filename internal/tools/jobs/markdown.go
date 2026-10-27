// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

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
	// A job that never started has no duration and no queue time: GitLab
	// sends null for both, and the rows are left out rather than read as 0s.
	if j.Duration != nil {
		c.Field("Duration", FormatDuration(j.Duration))
	}
	if j.QueuedDuration != nil {
		c.Field("Queued", FormatDuration(j.QueuedDuration))
	}
	// GitLab stores the failure reason as an integer column behind a fixed
	// enum map and serializes the key, such as script_failure.
	c.Field("Failure Reason", j.FailureReason)
	if j.Coverage != nil {
		c.Field("Coverage", fmt.Sprintf("%.1f%%", *j.Coverage))
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
			FormatDuration(j.Duration),
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
	hints := []string{
		toolutil.HintAction(actionJobGet, "see this job's details"),
		toolutil.HintAction(actionJobRetry, "re-run it"),
	}
	if t.Truncated {
		// Saying which part is shown matters: a failure is almost always at
		// the end of a log, and the end is what is kept, so a reader told
		// only "truncated" might look for the error in what is missing.
		c.Note(fmt.Sprintf("%s Showing the end of the log, at most its last 100 KB. The first %d bytes are left out.", toolutil.EmojiWarning, t.OmittedBytes))
		hints = append([]string{toolutil.HintAction(actionJobTrace, "read only the last lines of the log with tail_lines")}, hints...)
	}
	c.Fence("", "", t.Trace)
	c.End(hints...)
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
			FormatDuration(br.Duration),
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
	c.Warn(withheldWarning, out.Truncated)
	c.Note(artifactsNote(out))
	c.End(artifactsNextStep(out))
	return b.String()
}

// artifactsNextStep is the one-file read that takes what the caller of each
// download holds: the read by job ID after the download by job ID, and the
// read by ref, for the same ref and job, after the download by ref, whose
// caller has no job ID to give the other.
func artifactsNextStep(out ArtifactsOutput) string {
	if out.JobID > 0 {
		return toolutil.HintAction(actionJobDownloadSingle, "fetch one file out of the archive instead")
	}
	return toolutil.HintAction(actionJobDownloadSingleByRef, "fetch one file out of the archive instead, for the same ref and job")
}

// withheldWarning marks an artifact returned with no content for its size.
const withheldWarning = "Over the 1 MB one response carries, so no content is returned"

// artifactsHeading names the job when the caller asked for one, since the
// by-ref download answers for a ref rather than for a job ID.
func artifactsHeading(out ArtifactsOutput) string {
	if out.JobID > 0 {
		return fmt.Sprintf("Job #%d Artifacts", out.JobID)
	}
	return "Artifacts"
}

// artifactsNote says what the content is for the action that produced it.
// Only the download by job ID takes file_type (GitLab 19.4 declares it on
// that route alone), so the by-ref download, which carries no job ID, is
// always the zip archive and must not point a model at a parameter its
// action does not have. An artifact withheld for its size has no content to
// describe, and the note says where to read instead, which is a path no
// action lists: the archive's entries are the job's artifacts:paths. The
// by-ref download is sent to the one-file read by ref, since its caller has
// a ref and a job name and no job ID.
func artifactsNote(out ArtifactsOutput) string {
	if out.Truncated && out.JobID > 0 {
		return "A prefix of an archive or a compressed report cannot be opened, so none is returned. Read one file of it with job.download_single_artifact: its path is one of the job's artifacts:paths in .gitlab-ci.yml."
	}
	if out.Truncated {
		return "A prefix of an archive cannot be opened, so none is returned. Read one file of it with job.download_single_artifact_by_ref, for the same ref and job: its path is one of the job's artifacts:paths in .gitlab-ci.yml."
	}
	if out.JobID > 0 {
		return "The content is base64-encoded as GitLab stores it: the zip archive, or the report file_type named, which may be gzip-compressed. Decode it before reading."
	}
	return "The zip archive is base64-encoded. Decode it to extract the files."
}

// FormatSingleArtifactMarkdown renders one artifact file as the card of that
// file, its content in a fence sized to the content.
func FormatSingleArtifactMarkdown(out SingleArtifactOutput) string {
	var b strings.Builder
	// The artifact path is echoed from the caller's own argument: an entry
	// name in a zip, authored by whoever wrote the CI job.
	c := toolutil.NewCard(&b, singleArtifactHeading(out))
	c.Int("Size (bytes)", int64(out.Size))
	c.Field("Encoding", out.Encoding)
	switch out.Encoding {
	case "":
		// Only a binary file over the limit comes back with no encoding: it
		// has no content, since a prefix of one is of no use.
		c.Warn(withheldWarning, out.Truncated)
		c.Note("A prefix of a binary file cannot be used, so none is returned. Download the file from GitLab directly.")
	case encodingBase64:
		c.Note("The file is not text, so its bytes are base64-encoded. Decode them before use.")
	default:
		c.Warn("Truncated at 1 MB", out.Truncated)
	}
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
//
// The outcomes are if statements rather than the cases of a tagless switch,
// because a mutation run cannot credit a test with the expression of such a
// case and reports it as never reached.
func jobWaitHints(out WaitOutput) []string {
	if out.TimedOut {
		return []string{
			toolutil.HintAction(actionJobWait, "keep waiting for this job"),
			toolutil.HintAction(actionJobCancel, "abort it instead"),
		}
	}
	if out.FinalStatus == "failed" {
		return []string{
			toolutil.HintAction(actionJobTrace, "read the log for the failure"),
			toolutil.HintAction(actionJobRetry, "retry the job"),
		}
	}
	return nil
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
