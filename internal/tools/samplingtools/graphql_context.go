// graphql_context.go provides GraphQL-powered context builders that replace
// multiple sequential REST API calls with a single aggregation query. Each
// builder returns a formatted Markdown string ready for LLM analysis, plus
// key metadata fields needed by the sampling tool output structs.
//
// If the GraphQL query fails (e.g. GitLab version too old, GraphQL disabled,
// or a numeric project ID was provided instead of a path), callers should
// fall back to the existing REST-based data fetching.

package samplingtools

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// MRContextResult holds the GraphQL-fetched merge request context.
type MRContextResult struct {
	IID     int64
	Title   string
	Content string // Formatted Markdown ready for LLM
}

// BuildMRContext fetches MR details, discussions, approval state, and head
// pipeline status in a single GraphQL request and formats it as Markdown.
// The projectPath must be the full path (e.g. "group/project"), not a numeric ID.
func BuildMRContext(ctx context.Context, client *gitlabclient.Client, projectPath string, mrIID int64) (MRContextResult, error) {
	var resp struct {
		Data gqlMRContextResp `json:"data"`
	}
	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query: queryMRContext,
		Variables: map[string]any{
			"projectPath": projectPath,
			"mrIID":       strconv.FormatInt(mrIID, 10),
		},
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return MRContextResult{}, fmt.Errorf("GraphQL MR context query: %w", err)
	}

	mr := resp.Data.Project.MergeRequest
	if mr == nil {
		return MRContextResult{}, fmt.Errorf("merge request !%d not found in project %s", mrIID, projectPath)
	}

	iid, _ := strconv.ParseInt(mr.IID, 10, 64)
	content := formatMRContextMarkdown(mr)

	return MRContextResult{
		IID:     iid,
		Title:   mr.Title,
		Content: content,
	}, nil
}

// formatMRContextMarkdown renders the GraphQL merge request context into a
// Markdown document suitable for LLM analysis, including diff stats, pipeline
// status, approvals, reviewers, and discussion threads.
func formatMRContextMarkdown(mr *gqlMRContext) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# MR Review: !%s — %s\n\n", mr.IID, mr.Title)
	fmt.Fprintf(&b, toolutil.FmtMdState, mr.State)
	fmt.Fprintf(&b, "- **Source**: %s → %s\n", mr.SourceBranch, mr.TargetBranch)
	fmt.Fprintf(&b, "- **Merge Status**: %s\n", mr.MergeStatusEnum)

	if mr.DiffStatsSummary != nil {
		fmt.Fprintf(&b, "- **Changes**: +%d -%d across %d files\n",
			mr.DiffStatsSummary.Additions, mr.DiffStatsSummary.Deletions, mr.DiffStatsSummary.FileCount)
	}

	if mr.HeadPipeline != nil {
		status := mr.HeadPipeline.Status
		if mr.HeadPipeline.DetailedStatus != nil && mr.HeadPipeline.DetailedStatus.Label != "" {
			status = mr.HeadPipeline.DetailedStatus.Label
		}
		fmt.Fprintf(&b, "- **Pipeline**: %s\n", status)
	}

	if mr.Description != "" {
		fmt.Fprintf(&b, "\n## Description\n\n%s\n", mr.Description)
	}

	// Approval state.
	approvedBy := extractUsernames(mr.ApprovedBy)
	if mr.Approved || len(approvedBy) > 0 {
		b.WriteString("\n## Approval State\n\n")
		if mr.Approved {
			fmt.Fprintf(&b, "- "+toolutil.EmojiGreen+" **Approved** (required: %d, approved by: %s)\n",
				mr.ApprovalsReq, strings.Join(approvedBy, ", "))
		} else {
			fmt.Fprintf(&b, "- "+toolutil.EmojiYellow+" **Pending** (required: %d, approved by: %s)\n",
				mr.ApprovalsReq, strings.Join(approvedBy, ", "))
		}
	}

	// Discussions.
	writeDiscussions(&b, mr.Discussions.Nodes)

	return b.String()
}

// writeDiscussions appends MR discussion threads to the Markdown builder,
// filtering out system notes and formatting each user note with author and timestamp.
func writeDiscussions(b *strings.Builder, discussions []gqlDiscussion) {
	var userNotes int
	for _, d := range discussions {
		for _, n := range d.Notes.Nodes {
			if !n.System {
				userNotes++
			}
		}
	}
	if userNotes == 0 {
		return
	}

	fmt.Fprintf(b, "\n## Discussions (%d notes)\n\n", userNotes)
	for _, d := range discussions {
		for _, n := range d.Notes.Nodes {
			if n.System {
				continue
			}
			resolved := ""
			if n.Resolvable {
				if n.Resolved {
					resolved = " [RESOLVED]"
				} else {
					resolved = " [UNRESOLVED]"
				}
			}
			ts := toolutil.FormatTime(n.CreatedAt)
			if ts == "" {
				ts = "unknown"
			}
			fmt.Fprintf(b, "**%s** (%s)%s:\n%s\n\n---\n\n", n.Author.Username, ts, resolved, n.Body)
		}
	}
}

// IssueContextResult holds the GraphQL-fetched issue context.
type IssueContextResult struct {
	IID     int64
	Title   string
	Content string // Formatted Markdown ready for LLM
}

// BuildIssueContext fetches issue details, notes, participants, time tracking,
// labels, assignees, milestone, and related MRs in a single GraphQL request.
// This replaces up to 6 sequential REST calls used by analyze_issue_scope.
func BuildIssueContext(ctx context.Context, client *gitlabclient.Client, projectPath string, issueIID int64) (IssueContextResult, error) {
	var resp struct {
		Data gqlIssueContextResp `json:"data"`
	}
	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query: queryIssueContext,
		Variables: map[string]any{
			"projectPath": projectPath,
			"issueIID":    strconv.FormatInt(issueIID, 10),
		},
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return IssueContextResult{}, fmt.Errorf("GraphQL issue context query: %w", err)
	}

	issue := resp.Data.Project.Issue
	if issue == nil {
		return IssueContextResult{}, fmt.Errorf("issue #%d not found in project %s", issueIID, projectPath)
	}

	iid, _ := strconv.ParseInt(issue.IID, 10, 64)
	content := formatIssueContextMarkdown(issue)

	return IssueContextResult{
		IID:     iid,
		Title:   issue.Title,
		Content: content,
	}, nil
}

// formatIssueContextMarkdown renders the GraphQL issue context into a Markdown
// document suitable for LLM analysis, including labels, assignees, milestone,
// related issues, and discussion notes.
func formatIssueContextMarkdown(issue *gqlIssueContext) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Issue #%s: %s\n\n", issue.IID, issue.Title)
	fmt.Fprintf(&b, toolutil.FmtMdState, issue.State)
	fmt.Fprintf(&b, toolutil.FmtMdAuthor, issue.Author.Username)
	fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(issue.CreatedAt))

	if issue.DueDate != "" {
		fmt.Fprintf(&b, "- **Due Date**: %s\n", toolutil.FormatTime(issue.DueDate))
	}

	labels := extractLabels(issue.Labels)
	if len(labels) > 0 {
		fmt.Fprintf(&b, "- **Labels**: %s\n", strings.Join(labels, ", "))
	}

	assignees := extractUsernames(issue.Assignees)
	if len(assignees) > 0 {
		fmt.Fprintf(&b, "- **Assignees**: %s\n", strings.Join(assignees, ", "))
	}

	if issue.Weight > 0 {
		fmt.Fprintf(&b, "- **Weight**: %d\n", issue.Weight)
	}

	if issue.Milestone != nil {
		fmt.Fprintf(&b, "- **Milestone**: %s\n", issue.Milestone.Title)
	}

	// Time tracking.
	b.WriteString("\n## Time Tracking\n\n")
	if issue.HumanTimeEstimate != "" {
		fmt.Fprintf(&b, "- **Estimate**: %s\n", issue.HumanTimeEstimate)
	} else {
		b.WriteString("- **Estimate**: not set\n")
	}
	if issue.HumanTotalTimeSpent != "" {
		fmt.Fprintf(&b, "- **Time Spent**: %s\n", issue.HumanTotalTimeSpent)
	} else {
		b.WriteString("- **Time Spent**: none recorded\n")
	}

	// Participants.
	participants := extractUsernames(issue.Participants)
	if len(participants) > 0 {
		fmt.Fprintf(&b, "\n## Participants (%d)\n\n%s\n", len(participants), strings.Join(participants, ", "))
	}

	if issue.Description != "" {
		fmt.Fprintf(&b, "\n## Description\n\n%s\n", issue.Description)
	}

	// Related merge requests (includes closing MRs).
	if len(issue.RelatedMergeRequests.Nodes) > 0 {
		fmt.Fprintf(&b, "\n## Related MRs (%d)\n\n", len(issue.RelatedMergeRequests.Nodes))
		for _, mr := range issue.RelatedMergeRequests.Nodes {
			fmt.Fprintf(&b, "- !%s — %s [%s] (@%s)\n", mr.IID, mr.Title, mr.State, mr.Author.Username)
		}
	}

	// Notes (non-system, non-internal).
	writeIssueNotes(&b, issue.Notes.Nodes)

	return b.String()
}

// writeIssueNotes appends issue discussion notes to the Markdown builder,
// filtering out system and internal notes.
func writeIssueNotes(b *strings.Builder, notes []gqlNote) {
	var userNotes []gqlNote
	for _, n := range notes {
		if !n.System && !n.Internal {
			userNotes = append(userNotes, n)
		}
	}
	if len(userNotes) == 0 {
		return
	}

	fmt.Fprintf(b, "\n## Discussion (%d notes)\n\n", len(userNotes))
	for _, n := range userNotes {
		ts := toolutil.FormatTime(n.CreatedAt)
		if ts == "" {
			ts = "unknown"
		}
		fmt.Fprintf(b, "**%s** (%s):\n%s\n\n---\n\n", n.Author.Username, ts, n.Body)
	}
}

// PipelineContextResult holds the GraphQL-fetched pipeline context.
type PipelineContextResult struct {
	PipelineIID  int64
	Status       string
	Ref          string
	Content      string  // Formatted Markdown ready for LLM
	FailedJobIDs []int64 // Job IDs of failed jobs (for REST trace fetching)
}

// BuildPipelineContext fetches pipeline details with stages and jobs in a single
// GraphQL request. Job traces are not available via GraphQL and must be fetched
// separately via REST. The returned FailedJobIDs can be used for trace retrieval.
func BuildPipelineContext(ctx context.Context, client *gitlabclient.Client, projectPath string, pipelineIID int64) (PipelineContextResult, error) {
	var resp struct {
		Data gqlPipelineContextResp `json:"data"`
	}
	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query: queryPipelineContext,
		Variables: map[string]any{
			"projectPath": projectPath,
			"pipelineIID": strconv.FormatInt(pipelineIID, 10),
		},
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return PipelineContextResult{}, fmt.Errorf("GraphQL pipeline context query: %w", err)
	}

	pipeline := resp.Data.Project.Pipeline
	if pipeline == nil {
		return PipelineContextResult{}, fmt.Errorf("pipeline not found in project %s", projectPath)
	}

	iid, _ := strconv.ParseInt(pipeline.IID, 10, 64)
	content, failedIDs := formatPipelineContextMarkdown(pipeline)

	return PipelineContextResult{
		PipelineIID:  iid,
		Status:       pipeline.Status,
		Ref:          pipeline.Ref,
		Content:      content,
		FailedJobIDs: failedIDs,
	}, nil
}

// formatPipelineContextMarkdown renders the GraphQL pipeline context into a
// Markdown document and extracts failed job IDs. The Markdown includes ref,
// SHA, duration, YAML errors, stage summaries, and detailed failed job logs.
func formatPipelineContextMarkdown(p *gqlPipelineContext) (markdown string, failedJobIDs []int64) {
	var b strings.Builder
	fmt.Fprintf(&b, "# Pipeline #%s — %s\n\n", p.IID, p.Status)
	fmt.Fprintf(&b, "- **Ref**: %s\n", p.Ref)
	fmt.Fprintf(&b, "- **SHA**: %s\n", p.SHA)
	fmt.Fprintf(&b, "- **Source**: %s\n", p.Source)
	if p.Duration != nil {
		fmt.Fprintf(&b, "- **Duration**: %.0fs\n", *p.Duration)
	}
	if p.YamlErrors != "" {
		fmt.Fprintf(&b, "- **YAML Errors**: %s\n", p.YamlErrors)
	}

	var failedJobs []gqlJob
	for _, stage := range p.Stages.Nodes {
		for _, job := range stage.Jobs.Nodes {
			if strings.EqualFold(job.Status, "FAILED") {
				failedJobs = append(failedJobs, job)
			}
		}
	}

	// Stage overview.
	if len(p.Stages.Nodes) > 0 {
		fmt.Fprintf(&b, "\n## Stages (%d)\n\n", len(p.Stages.Nodes))
		for _, stage := range p.Stages.Nodes {
			fmt.Fprintf(&b, "- **%s**: %s (%d jobs)\n", stage.Name, stage.Status, len(stage.Jobs.Nodes))
		}
	}

	// Failed jobs details.
	fmt.Fprintf(&b, "\n## Failed Jobs (%d)\n\n", len(failedJobs))
	var failedIDs []int64
	for _, j := range failedJobs {
		stageName := ""
		if j.Stage != nil {
			stageName = j.Stage.Name
		}
		fmt.Fprintf(&b, "### %s (stage: %s)\n\n", j.Name, stageName)
		fmt.Fprintf(&b, toolutil.FmtMdStatus, j.Status)
		if j.FailureMessage != "" {
			fmt.Fprintf(&b, "- **Failure Message**: %s\n", j.FailureMessage)
		}
		if j.Duration != nil {
			fmt.Fprintf(&b, "- **Duration**: %.1fs\n", *j.Duration)
		}
		// Extract numeric ID from webPath if available (e.g. "/group/project/-/jobs/123").
		if id := extractJobIDFromWebPath(j.WebPath); id > 0 {
			failedIDs = append(failedIDs, id)
		}
		b.WriteString("\n")
	}

	return b.String(), failedIDs
}

// extractJobIDFromWebPath extracts the numeric job ID from a GitLab web path
// like "/group/project/-/jobs/123".
func extractJobIDFromWebPath(webPath string) int64 {
	if webPath == "" {
		return 0
	}
	idx := strings.LastIndex(webPath, "/")
	if idx < 0 || idx == len(webPath)-1 {
		return 0
	}
	id, err := strconv.ParseInt(webPath[idx+1:], 10, 64)
	if err != nil {
		return 0
	}
	return id
}

// Helper functions.

// extractUsernames collects the Username field from a GraphQL user connection.
func extractUsernames(nodes gqlUsernameNodes) []string {
	if len(nodes.Nodes) == 0 {
		return nil
	}
	names := make([]string, len(nodes.Nodes))
	for i, n := range nodes.Nodes {
		names[i] = n.Username
	}
	return names
}

// extractLabels collects the Title field from a GraphQL label connection.
func extractLabels(nodes gqlLabelNodes) []string {
	if len(nodes.Nodes) == 0 {
		return nil
	}
	labels := make([]string, len(nodes.Nodes))
	for i, n := range nodes.Nodes {
		labels[i] = n.Title
	}
	return labels
}
