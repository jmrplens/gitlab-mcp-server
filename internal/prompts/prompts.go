package prompts

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const fmtFilesChanged = "- **Files changed**: %d\n"

// Reusable prompt argument names and descriptions.
const (
	argProjectID  = "project_id"
	argMRIID      = "merge_request_iid"
	descProjectID = "Project ID (numeric) or URL-encoded path (e.g. 'group/project')"
	descMRIID     = "Merge request IID (project-scoped numeric ID, visible as !N in GitLab)"

	fmtTwoArgsRequired  = "%s and %s are required"
	fmtOneArgRequired   = "%s is required"
	fmtGetMRFailed      = "failed to get merge request: %w"
	fmtGetMRDiffsFailed = "failed to get MR diffs: %w"
	fmtListItem         = "- %s (%s)\n"

	tableCategoryHeader    = "| Category | Count |\n"
	tableCategorySeparator = "|----------|-------|\n"
	fmtTableCountRow       = "| %s | %d |\n"
)

// projectIDArg returns a required prompt argument for the GitLab project ID.
func projectIDArg() *mcp.PromptArgument {
	return &mcp.PromptArgument{Name: argProjectID, Title: toolutil.TitleFromName(argProjectID), Description: descProjectID, Required: true}
}

// mrIIDArg returns a required prompt argument for the merge request IID.
func mrIIDArg() *mcp.PromptArgument {
	return &mcp.PromptArgument{Name: argMRIID, Title: toolutil.TitleFromName(argMRIID), Description: descMRIID, Required: true}
}

// Register registers all MCP prompts (AI-optimized summaries, etc).
func Register(server *mcp.Server, client *gitlabclient.Client) {
	registerSummarizeMRChangesPrompt(server, client)
	registerReviewMRPrompt(server, client)
	registerSummarizePipelineStatusPrompt(server, client)
	registerSuggestMRReviewersPrompt(server, client)
	registerGenerateReleaseNotesPrompt(server, client)
	registerSummarizeOpenMRsPrompt(server, client)
	registerProjectHealthCheckPrompt(server, client)
	registerCompareBranchesPrompt(server, client)
	registerDailyStandupPrompt(server, client)
	registerMRRiskAssessmentPrompt(server, client)
	registerTeamMemberWorkloadPrompt(server, client)
	registerUserStatsPrompt(server, client)

	// Cross-project prompts (personal dashboards)
	registerCrossProjectPrompts(server, client)

	// Team management prompts (group-level)
	registerTeamPrompts(server, client)

	// Project report prompts (project-level analysis)
	registerProjectReportPrompts(server, client)

	// Analytics prompts (velocity, releases, recaps)
	registerAnalyticsPrompts(server, client)

	// Milestone, label, and contributor prompts
	registerMilestoneLabelPrompts(server, client)

	// Project audit prompts (settings, branch protection, access, workflow)
	registerAuditPrompts(server, client)
}

// registerSummarizeMRChangesPrompt registers the summarize_mr_changes prompt.
func registerSummarizeMRChangesPrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "summarize_mr_changes",
		Title:       toolutil.TitleFromName("summarize_mr_changes"),
		Description: "Summarize the changed files and key modifications in a merge request. Lists each file with its change type (new/modified/deleted/renamed). Use this to quickly understand the scope of a merge request.",
		Icons:       toolutil.IconMR,
		Arguments: []*mcp.PromptArgument{
			projectIDArg(),
			mrIIDArg(),
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleSummarizeMRChanges(ctx, client, req)
	})
}

// handleSummarizeMRChanges lists changed files with their change types for a MR.
func handleSummarizeMRChanges(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	args := req.Params.Arguments
	projectID := args[argProjectID]
	mrIID := args[argMRIID]
	if projectID == "" || mrIID == "" {
		return nil, fmt.Errorf(fmtTwoArgsRequired, argProjectID, argMRIID)
	}

	changes, _, err := client.GL().MergeRequests.ListMergeRequestDiffs(projectID, parseIID(mrIID), nil, gl.WithContext(ctx))
	if err != nil {
		return nil, err
	}

	var summary []string
	for _, c := range changes {
		line := fmt.Sprintf("- %s → %s (%s)", c.OldPath, c.NewPath, changeType(c))
		summary = append(summary, line)
	}
	msg := "Changed files:\n" + strings.Join(summary, "\n")

	return promptResult(msg), nil
}

// registerReviewMRPrompt registers the review_mr prompt.
func registerReviewMRPrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "review_mr",
		Title:       toolutil.TitleFromName("review_mr"),
		Description: "Generate a structured code review for a merge request. Files are categorized by risk (high-risk, business logic, tests, documentation) with per-file metrics and a review plan. Full diffs are included without truncation.",
		Icons:       toolutil.IconMR,
		Arguments: []*mcp.PromptArgument{
			projectIDArg(),
			mrIIDArg(),
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleReviewMR(ctx, client, req)
	})
}

// handleReviewMR generates a structured code review with files categorized by
// risk level and full diffs included.
func handleReviewMR(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	projectID := req.Params.Arguments[argProjectID]
	mrIID := req.Params.Arguments[argMRIID]
	if projectID == "" || mrIID == "" {
		return nil, fmt.Errorf(fmtTwoArgsRequired, argProjectID, argMRIID)
	}

	iid := parseIID(mrIID)
	mr, _, err := client.GL().MergeRequests.GetMergeRequest(projectID, iid, &gl.GetMergeRequestsOptions{}, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf(fmtGetMRFailed, err)
	}

	diffs, _, err := client.GL().MergeRequests.ListMergeRequestDiffs(projectID, iid, nil, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf(fmtGetMRDiffsFailed, err)
	}

	// Categorize files by risk/type
	highRisk, logic, tests, docs := categorizeDiffs(diffs)
	m := computeDiffMetrics(diffs)

	var b strings.Builder
	fmt.Fprintf(&b, "# Code Review: %s (MR !%d)\n\n", mr.Title, mr.IID)
	fmt.Fprintf(&b, "**Branch**: %s → %s\n", mr.SourceBranch, mr.TargetBranch)
	if mr.Description != "" {
		fmt.Fprintf(&b, "**Description**: %s\n", mr.Description)
	}

	// Overall metrics
	fmt.Fprintf(&b, "\n## Metrics\n")
	fmt.Fprintf(&b, fmtFilesChanged, len(diffs))
	fmt.Fprintf(&b, "- **Lines added**: %d\n", m.additions)
	fmt.Fprintf(&b, "- **Lines removed**: %d\n", m.deletions)
	fmt.Fprintf(&b, "- **Has conflicts**: %v\n", mr.HasConflicts)

	// Review plan
	fmt.Fprintf(&b, "\n## Review Plan\n")
	fmt.Fprintf(&b, "1. **High-risk files** (%d) — security, config, migrations, CI\n", len(highRisk))
	fmt.Fprintf(&b, "2. **Business logic** (%d) — core application code\n", len(logic))
	fmt.Fprintf(&b, "3. **Tests** (%d) — test files and specs\n", len(tests))
	fmt.Fprintf(&b, "4. **Documentation** (%d) — docs, README, comments\n", len(docs))
	b.WriteString("\nReview each group in order, then provide a global assessment.\n")

	// Write each group with full diffs
	writeDiffGroup(&b, "High-Risk Files", highRisk)
	writeDiffGroup(&b, "Business Logic", logic)
	writeDiffGroup(&b, "Tests", tests)
	writeDiffGroup(&b, "Documentation", docs)

	b.WriteString("\n## Review Checklist\n")
	b.WriteString("1. Correctness and logic errors\n")
	b.WriteString("2. Security vulnerabilities\n")
	b.WriteString("3. Performance concerns\n")
	b.WriteString("4. Code style and best practices\n")
	b.WriteString("5. Missing tests or edge cases\n")

	b.WriteString("\n## How to Submit Review Comments\n")
	b.WriteString("Use the batch review workflow to avoid sending one notification per comment:\n")
	b.WriteString("1. For each finding, call `gitlab_mr_review` with action=`draft_note_create` and include the `position` object for inline comments on specific diff lines.\n")
	b.WriteString("   Position fields: base_sha, start_sha, head_sha, new_path, old_path, and EITHER new_line OR old_line (not both).\n")
	b.WriteString("   - Modified/added line → set `new_line` only (line number in the new file).\n")
	b.WriteString("   - Removed line → set `old_line` only (line number in the old file).\n")
	b.WriteString("   - Unchanged context line → set both `old_line` and `new_line`.\n")
	fmt.Fprintf(&b, "   Use base_sha=`%s`, start_sha=`%s`, head_sha=`%s` from this MR.\n", mr.DiffRefs.BaseSha, mr.DiffRefs.StartSha, mr.DiffRefs.HeadSha)
	b.WriteString("2. For general comments not tied to a specific line, call `draft_note_create` without the position field.\n")
	b.WriteString("3. To reply to existing open discussions, call `draft_note_create` with `in_reply_to_discussion_id` set to the discussion ID. You can also set `resolve_discussion: true` to resolve it when published.\n")
	b.WriteString("4. After ALL comments and replies are created as draft notes, call `draft_note_publish_all` ONCE to publish them all at once (single notification to the MR author).\n")
	b.WriteString("5. Do NOT use `discussion_create`, `discussion_reply`, or `note_create` for review comments — those publish immediately and generate one notification each.\n")

	return promptResult(b.String()), nil
}

// registerSummarizePipelineStatusPrompt registers the summarize_pipeline_status prompt.
func registerSummarizePipelineStatusPrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "summarize_pipeline_status",
		Title:       toolutil.TitleFromName("summarize_pipeline_status"),
		Description: "Summarize the latest CI/CD pipeline status for a project. Groups jobs by outcome (failed/passed/other) and includes failure reasons for debugging.",
		Icons:       toolutil.IconPipeline,
		Arguments: []*mcp.PromptArgument{
			projectIDArg(),
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleSummarizePipelineStatus(ctx, client, req)
	})
}

// handleSummarizePipelineStatus fetches the latest pipeline and its jobs to
// produce a pipeline status summary.
func handleSummarizePipelineStatus(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	projectID := req.Params.Arguments[argProjectID]
	if projectID == "" {
		return nil, fmt.Errorf(fmtOneArgRequired, argProjectID)
	}

	pipeline, _, err := client.GL().Pipelines.GetLatestPipeline(projectID, &gl.GetLatestPipelineOptions{}, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("failed to get latest pipeline: %w", err)
	}

	jobs, _, err := client.GL().Jobs.ListPipelineJobs(projectID, pipeline.ID, &gl.ListJobsOptions{}, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("failed to list pipeline jobs: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Pipeline #%d Status: %s\n\n", pipeline.ID, strings.ToUpper(pipeline.Status))
	fmt.Fprintf(&b, "**Ref**: %s | **SHA**: %s\n", pipeline.Ref, shortSHA(pipeline.SHA))
	fmt.Fprintf(&b, "**URL**: %s\n\n", pipeline.WebURL)

	var failed, passed, other []string
	for _, j := range jobs {
		line := fmt.Sprintf("- **%s** (%s): %s", j.Name, j.Stage, j.Status)
		if j.FailureReason != "" {
			line += fmt.Sprintf(" — reason: %s", j.FailureReason)
		}
		switch j.Status {
		case "failed":
			failed = append(failed, line)
		case "success":
			passed = append(passed, line)
		default:
			other = append(other, line)
		}
	}

	if len(failed) > 0 {
		fmt.Fprintf(&b, "## Failed Jobs (%d)\n%s\n\n", len(failed), strings.Join(failed, "\n"))
	}
	if len(passed) > 0 {
		fmt.Fprintf(&b, "## Passed Jobs (%d)\n%s\n\n", len(passed), strings.Join(passed, "\n"))
	}
	if len(other) > 0 {
		fmt.Fprintf(&b, "## Other Jobs (%d)\n%s\n\n", len(other), strings.Join(other, "\n"))
	}

	return promptResult(b.String()), nil
}

// registerSuggestMRReviewersPrompt registers the suggest_mr_reviewers prompt.
func registerSuggestMRReviewersPrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "suggest_mr_reviewers",
		Title:       toolutil.TitleFromName("suggest_mr_reviewers"),
		Description: "Suggest suitable merge request reviewers based on the files changed and the list of active project members. Excludes the MR author from suggestions.",
		Icons:       toolutil.IconMR,
		Arguments: []*mcp.PromptArgument{
			projectIDArg(),
			mrIIDArg(),
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleSuggestMRReviewers(ctx, client, req)
	})
}

// handleSuggestMRReviewers identifies potential reviewers based on blame data
// for files changed in a merge request.
func handleSuggestMRReviewers(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	projectID := req.Params.Arguments[argProjectID]
	mrIID := req.Params.Arguments[argMRIID]
	if projectID == "" || mrIID == "" {
		return nil, fmt.Errorf(fmtTwoArgsRequired, argProjectID, argMRIID)
	}

	iid := parseIID(mrIID)
	mr, _, err := client.GL().MergeRequests.GetMergeRequest(projectID, iid, &gl.GetMergeRequestsOptions{}, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf(fmtGetMRFailed, err)
	}

	diffs, _, err := client.GL().MergeRequests.ListMergeRequestDiffs(projectID, iid, nil, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf(fmtGetMRDiffsFailed, err)
	}

	members, _, err := client.GL().ProjectMembers.ListAllProjectMembers(projectID, &gl.ListProjectMembersOptions{}, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("failed to list project members: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Reviewer Suggestions for MR !%d: %s\n\n", mr.IID, mr.Title)

	fmt.Fprintf(&b, "## Changed Files (%d)\n", len(diffs))
	for _, d := range diffs {
		fmt.Fprintf(&b, fmtListItem, d.NewPath, changeType(d))
	}

	authorName := ""
	if mr.Author != nil {
		authorName = mr.Author.Username
	}

	fmt.Fprintf(&b, "\n## Project Members (excluding author: %s)\n", authorName)
	for _, m := range members {
		if m.Username == authorName || m.State != "active" {
			continue
		}
		fmt.Fprintf(&b, "- **%s** (%s) — access level: %d\n", m.Name, m.Username, m.AccessLevel)
	}

	b.WriteString("\nBased on the changed files and project members, suggest the most suitable reviewers and explain why.")

	return promptResult(b.String()), nil
}

// registerGenerateReleaseNotesPrompt registers the generate_release_notes prompt.
func registerGenerateReleaseNotesPrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "generate_release_notes",
		Title:       toolutil.TitleFromName("generate_release_notes"),
		Description: "Generate comprehensive release notes from commits, merge requests, and file changes between two Git refs (tags, branches, or SHAs). Produces a structured document with commits, merged MRs with labels, contributors, and statistics for organizing into user-friendly release notes.",
		Icons:       toolutil.IconRelease,
		Arguments: []*mcp.PromptArgument{
			projectIDArg(),
			{Name: "from", Title: toolutil.TitleFromName("from"), Description: "Starting ref: tag name (e.g. 'v1.0.0'), branch name, or commit SHA", Required: true},
			{Name: "to", Title: toolutil.TitleFromName("to"), Description: "Ending ref: tag name, branch name, or commit SHA (defaults to HEAD if omitted)", Required: false},
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleGenerateReleaseNotes(ctx, client, req)
	})
}

// handleGenerateReleaseNotes compares two refs and generates release note
// content from commits, merged MRs, and changed files between them.
func handleGenerateReleaseNotes(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	projectID := req.Params.Arguments[argProjectID]
	from := req.Params.Arguments["from"]
	if projectID == "" || from == "" {
		return nil, fmt.Errorf("%s and from are required", argProjectID)
	}
	to := req.Params.Arguments["to"]
	if to == "" {
		to = "HEAD"
	}

	comparison, _, err := client.GL().Repositories.Compare(projectID, &gl.CompareOptions{
		From: new(from),
		To:   new(to),
	}, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("failed to compare refs: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Release Notes: %s → %s\n\n", from, to)

	// Fetch merged MRs in the commit date range.
	mergedMRs := fetchMergedMRsForRange(ctx, client, projectID, comparison.Commits)
	writeReleaseNotesMRs(&b, mergedMRs)

	fmt.Fprintf(&b, "## Commits (%d)\n\n", len(comparison.Commits))
	for _, c := range comparison.Commits {
		title := strings.SplitN(c.Title, "\n", 2)[0]
		fmt.Fprintf(&b, "- %s — %s (%s)\n", shortSHA(c.ID), title, c.AuthorName)
	}

	fmt.Fprintf(&b, "\n## Files Changed (%d)\n\n", len(comparison.Diffs))
	for _, d := range comparison.Diffs {
		fmt.Fprintf(&b, "- %s\n", d.NewPath)
	}

	writeReleaseNotesStats(&b, comparison)

	b.WriteString("\n---\n\n")
	b.WriteString("Please organize the information above into a polished release notes document. ")
	b.WriteString("Use the merge request titles, labels, and descriptions as the primary source for categorization. ")
	b.WriteString("Group items into sections such as: **Features**, **Bug Fixes**, **Improvements**, **Breaking Changes**, **Documentation**, and **Other**. ")
	b.WriteString("Use labels (e.g. 'bug', 'feature', 'enhancement', 'breaking') to assign categories when available. ")
	b.WriteString("For each entry, include the MR reference (!IID) or commit SHA, a concise description, and the author. ")
	b.WriteString("Omit merge commits and internal-only changes that are not relevant to end users.")

	return promptResult(b.String()), nil
}

// fetchMergedMRsForRange fetches MRs merged within the time range of the given commits.
func fetchMergedMRsForRange(ctx context.Context, client *gitlabclient.Client, projectID string, commits []*gl.Commit) []*gl.BasicMergeRequest {
	if len(commits) == 0 {
		return nil
	}

	// Determine the date range from commits.
	var earliest, latest time.Time
	for _, c := range commits {
		if c.CommittedDate == nil {
			continue
		}
		if earliest.IsZero() || c.CommittedDate.Before(earliest) {
			earliest = *c.CommittedDate
		}
		if latest.IsZero() || c.CommittedDate.After(latest) {
			latest = *c.CommittedDate
		}
	}
	if earliest.IsZero() {
		return nil
	}

	// Add buffer to ensure we capture MRs merged near the boundaries.
	earliest = earliest.Add(-24 * time.Hour)

	mrs, _, err := client.GL().MergeRequests.ListProjectMergeRequests(projectID, &gl.ListProjectMergeRequestsOptions{
		State:        new("merged"),
		UpdatedAfter: new(earliest),
		OrderBy:      new("updated_at"),
		Sort:         new("desc"),
	}, gl.WithContext(ctx))
	if err != nil {
		slog.Warn("failed to fetch merged MRs for release notes", "error", err)
		return nil
	}

	// Filter to only MRs actually merged within the range.
	var filtered []*gl.BasicMergeRequest
	for _, mr := range mrs {
		if mr.MergedAt != nil && !mr.MergedAt.After(latest.Add(24*time.Hour)) {
			filtered = append(filtered, mr)
		}
	}

	return filtered
}

// writeReleaseNotesMRs writes the merged MRs section for release notes.
func writeReleaseNotesMRs(b *strings.Builder, mrs []*gl.BasicMergeRequest) {
	if len(mrs) == 0 {
		return
	}

	fmt.Fprintf(b, "## Merge Requests (%d)\n\n", len(mrs))
	for _, mr := range mrs {
		author := "unknown"
		if mr.Author != nil {
			author = mr.Author.Username
		}
		labels := ""
		if len(mr.Labels) > 0 {
			labels = " [" + strings.Join(mr.Labels, ", ") + "]"
		}
		fmt.Fprintf(b, "- !%d — %s (@%s)%s\n", mr.IID, mr.Title, author, labels)
		if mr.Description != "" {
			desc := strings.SplitN(mr.Description, "\n", 2)[0]
			if len(desc) > 200 {
				desc = desc[:200] + "..."
			}
			fmt.Fprintf(b, "  > %s\n", desc)
		}
	}
	b.WriteString("\n")
}

// writeReleaseNotesStats writes summary statistics for release notes.
func writeReleaseNotesStats(b *strings.Builder, comparison *gl.Compare) {
	contributors := make(map[string]struct{})
	for _, c := range comparison.Commits {
		if c.AuthorEmail != "" {
			contributors[c.AuthorEmail] = struct{}{}
		}
	}

	b.WriteString("\n## Statistics\n\n")
	fmt.Fprintf(b, "- **Commits**: %d\n", len(comparison.Commits))
	fmt.Fprintf(b, fmtFilesChanged, len(comparison.Diffs))
	fmt.Fprintf(b, "- **Contributors**: %d\n", len(contributors))
}

// registerSummarizeOpenMRsPrompt registers the summarize_open_mrs prompt.
func registerSummarizeOpenMRsPrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "summarize_open_mrs",
		Title:       toolutil.TitleFromName("summarize_open_mrs"),
		Description: "Summarize all open merge requests in a project including title, author, branches, age in days, and merge status. Highlights stale MRs (>7 days) that need attention.",
		Icons:       toolutil.IconMR,
		Arguments: []*mcp.PromptArgument{
			projectIDArg(),
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleSummarizeOpenMRs(ctx, client, req)
	})
}

// handleSummarizeOpenMRs lists all open merge requests in a project with
// author, age, and merge status details.
func handleSummarizeOpenMRs(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	projectID := req.Params.Arguments[argProjectID]
	if projectID == "" {
		return nil, fmt.Errorf(fmtOneArgRequired, argProjectID)
	}

	mrs, _, err := client.GL().MergeRequests.ListProjectMergeRequests(projectID, &gl.ListProjectMergeRequestsOptions{
		State: new("opened"),
	}, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("failed to list open MRs: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Open Merge Requests (%d)\n\n", len(mrs))

	for _, mr := range mrs {
		author := "unknown"
		if mr.Author != nil {
			author = mr.Author.Username
		}
		age := time.Since(*mr.CreatedAt).Hours() / 24
		fmt.Fprintf(&b, "## !%d: %s\n", mr.IID, mr.Title)
		fmt.Fprintf(&b, "- **Author**: %s | **Branch**: %s → %s\n", author, mr.SourceBranch, mr.TargetBranch)
		fmt.Fprintf(&b, "- **Age**: %.0f days | **Status**: %s\n", age, mr.DetailedMergeStatus)
		if mr.Description != "" {
			desc := mr.Description
			if len(desc) > 200 {
				desc = desc[:200] + "..."
			}
			fmt.Fprintf(&b, "- **Description**: %s\n", desc)
		}
		b.WriteString("\n")
	}

	b.WriteString("Please provide an overview of these MRs, highlighting any that are stale (>7 days) or need attention.")

	return promptResult(b.String()), nil
}

// registerProjectHealthCheckPrompt registers the project_health_check prompt.
func registerProjectHealthCheckPrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "project_health_check",
		Title:       toolutil.TitleFromName("project_health_check"),
		Description: "Comprehensive project health assessment combining latest pipeline status, open merge requests, and branch hygiene (merged/stale branch counts). Provides actionable recommendations for project maintenance.",
		Icons:       toolutil.IconHealth,
		Arguments: []*mcp.PromptArgument{
			projectIDArg(),
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleProjectHealthCheck(ctx, client, req)
	})
}

// handleProjectHealthCheck aggregates pipeline status, open MRs, and branch
// statistics into a project health report.
func handleProjectHealthCheck(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	projectID := req.Params.Arguments[argProjectID]
	if projectID == "" {
		return nil, fmt.Errorf(fmtOneArgRequired, argProjectID)
	}

	project, _, err := client.GL().Projects.GetProject(projectID, &gl.GetProjectOptions{}, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Project Health Check: %s\n\n", project.PathWithNamespace)

	writePipelineSection(ctx, &b, client, projectID)
	writeOpenMRsSection(ctx, &b, client, projectID)
	writeBranchesSection(ctx, &b, client, projectID)

	b.WriteString("Based on this data, provide a health assessment with recommendations for improving project hygiene.")

	return promptResult(b.String()), nil
}

// writePipelineSection appends the latest pipeline status to the builder.
func writePipelineSection(ctx context.Context, b *strings.Builder, client *gitlabclient.Client, projectID string) {
	pipeline, _, err := client.GL().Pipelines.GetLatestPipeline(projectID, &gl.GetLatestPipelineOptions{}, gl.WithContext(ctx))
	if err != nil {
		b.WriteString("## Latest Pipeline: N/A\n\n")
		return
	}
	fmt.Fprintf(b, "## Latest Pipeline: %s\n", strings.ToUpper(pipeline.Status))
	fmt.Fprintf(b, "- **Ref**: %s | **SHA**: %s\n", pipeline.Ref, shortSHA(pipeline.SHA))
	fmt.Fprintf(b, "- **URL**: %s\n\n", pipeline.WebURL)
}

// writeOpenMRsSection appends a list of open merge requests to the builder.
func writeOpenMRsSection(ctx context.Context, b *strings.Builder, client *gitlabclient.Client, projectID string) {
	mrs, _, err := client.GL().MergeRequests.ListProjectMergeRequests(projectID, &gl.ListProjectMergeRequestsOptions{
		State: new("opened"),
	}, gl.WithContext(ctx))
	if err != nil {
		return
	}
	fmt.Fprintf(b, "## Open Merge Requests: %d\n", len(mrs))
	for _, mr := range mrs {
		author := "unknown"
		if mr.Author != nil {
			author = mr.Author.Username
		}
		age := time.Since(*mr.CreatedAt).Hours() / 24
		fmt.Fprintf(b, "- !%d: %s (by %s, %.0fd old)\n", mr.IID, mr.Title, author, age)
	}
	b.WriteString("\n")
}

// writeBranchesSection appends branch statistics (total, merged, stale) to the builder.
func writeBranchesSection(ctx context.Context, b *strings.Builder, client *gitlabclient.Client, projectID string) {
	branches, _, err := client.GL().Branches.ListBranches(projectID, &gl.ListBranchesOptions{}, gl.WithContext(ctx))
	if err != nil {
		return
	}
	merged, stale := countBranchStats(branches)
	fmt.Fprintf(b, "## Branches: %d total, %d merged, %d stale (>30 days)\n\n", len(branches), merged, stale)
}

// countBranchStats returns the number of merged and stale (>30 days) branches.
func countBranchStats(branches []*gl.Branch) (merged, stale int) {
	for _, br := range branches {
		if br.Merged {
			merged++
		}
		if br.Commit != nil && br.Commit.CommittedDate != nil {
			age := time.Since(*br.Commit.CommittedDate).Hours() / 24
			if age > 30 {
				stale++
			}
		}
	}
	return
}

// registerCompareBranchesPrompt registers the compare_branches prompt.
func registerCompareBranchesPrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "compare_branches",
		Title:       toolutil.TitleFromName("compare_branches"),
		Description: "Compare two Git branches or refs showing commit differences and file changes between them. Useful for understanding divergence before merging or releasing.",
		Icons:       toolutil.IconBranch,
		Arguments: []*mcp.PromptArgument{
			projectIDArg(),
			{Name: "from", Title: toolutil.TitleFromName("from"), Description: "Source branch name, tag, or commit SHA to compare from", Required: true},
			{Name: "to", Title: toolutil.TitleFromName("to"), Description: "Target branch name, tag, or commit SHA to compare to", Required: true},
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleCompareBranches(ctx, client, req)
	})
}

// handleCompareBranches compares two refs and produces a summary of commits
// and changed files between them.
func handleCompareBranches(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	projectID := req.Params.Arguments[argProjectID]
	from := req.Params.Arguments["from"]
	to := req.Params.Arguments["to"]
	if projectID == "" || from == "" || to == "" {
		return nil, fmt.Errorf("%s, from, and to are required", argProjectID)
	}

	comparison, _, err := client.GL().Repositories.Compare(projectID, &gl.CompareOptions{
		From: new(from),
		To:   new(to),
	}, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("failed to compare branches: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Branch Comparison: %s → %s\n\n", from, to)

	if comparison.CompareSameRef {
		b.WriteString("Both refs point to the same commit. No differences.\n")
		return promptResult(b.String()), nil
	}

	fmt.Fprintf(&b, "## Commits (%d)\n\n", len(comparison.Commits))
	for _, c := range comparison.Commits {
		title := strings.SplitN(c.Title, "\n", 2)[0]
		fmt.Fprintf(&b, "- %s — %s (%s)\n", shortSHA(c.ID), title, c.AuthorName)
	}

	fmt.Fprintf(&b, "\n## File Changes (%d)\n\n", len(comparison.Diffs))
	for _, d := range comparison.Diffs {
		ct := "modified"
		if d.NewFile {
			ct = "new"
		} else if d.DeletedFile {
			ct = "deleted"
		} else if d.RenamedFile {
			ct = "renamed"
		}
		fmt.Fprintf(&b, fmtListItem, d.NewPath, ct)
	}

	b.WriteString("\nPlease summarize the key differences between these branches.")

	return promptResult(b.String()), nil
}

// registerDailyStandupPrompt registers the daily_standup prompt.
func registerDailyStandupPrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "daily_standup",
		Title:       toolutil.TitleFromName("daily_standup"),
		Description: "Generate a daily standup summary based on the user's GitLab activity in the last 24 hours: contribution events, authored MRs, assigned MRs, MRs under review, assigned issues, and created issues. Produces a comprehensive report with done/planned/blockers sections.",
		Icons:       toolutil.IconUser,
		Arguments: []*mcp.PromptArgument{
			projectIDArg(),
			{Name: "username", Title: toolutil.TitleFromName("username"), Description: "GitLab username to generate the standup for (defaults to the authenticated user if omitted)", Required: false},
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleDailyStandup(ctx, client, req)
	})
}

// handleDailyStandup gathers recent MRs and issues for a user to generate
// daily standup report content.
func handleDailyStandup(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	projectID := req.Params.Arguments[argProjectID]
	if projectID == "" {
		return nil, fmt.Errorf(fmtOneArgRequired, argProjectID)
	}

	username, userID, isCurrentUser, err := resolveUser(ctx, client, req.Params.Arguments["username"])
	if err != nil {
		return nil, err
	}

	// Recent contribution events (last 24h)
	eventOpts := &gl.ListContributionEventsOptions{
		After: new(gl.ISOTime(time.Now().AddDate(0, 0, -1))),
	}
	var events []*gl.ContributionEvent
	if isCurrentUser {
		events, _, err = client.GL().Events.ListCurrentUserContributionEvents(eventOpts, gl.WithContext(ctx))
	} else {
		events, _, err = client.GL().Users.ListUserContributionEvents(userID, eventOpts, gl.WithContext(ctx))
	}
	if err != nil {
		return nil, fmt.Errorf("failed to list events: %w", err)
	}

	// Open MRs authored by user
	authoredMRs, _, mrAuthorErr := client.GL().MergeRequests.ListProjectMergeRequests(projectID, &gl.ListProjectMergeRequestsOptions{
		State:          new("opened"),
		AuthorUsername: new(username),
	}, gl.WithContext(ctx))

	// MRs assigned to user
	assignedMRs, _, mrAssignedErr := client.GL().MergeRequests.ListProjectMergeRequests(projectID, &gl.ListProjectMergeRequestsOptions{
		State:      new("opened"),
		AssigneeID: gl.AssigneeID(userID),
	}, gl.WithContext(ctx))

	// MRs where user is reviewer
	reviewMRs, _, mrReviewErr := client.GL().MergeRequests.ListProjectMergeRequests(projectID, &gl.ListProjectMergeRequestsOptions{
		State:            new("opened"),
		ReviewerUsername: new(username),
	}, gl.WithContext(ctx))

	// Issues assigned to user
	assignedIssues, _, issueAssignedErr := client.GL().Issues.ListProjectIssues(projectID, &gl.ListProjectIssuesOptions{
		State:            new("opened"),
		AssigneeUsername: new(username),
	}, gl.WithContext(ctx))

	// Issues created by user
	createdIssues, _, issueCreatedErr := client.GL().Issues.ListProjectIssues(projectID, &gl.ListProjectIssuesOptions{
		State:          new("opened"),
		AuthorUsername: new(username),
	}, gl.WithContext(ctx))

	var b strings.Builder
	fmt.Fprintf(&b, "# Daily Standup for @%s\n\n", username)

	// Events section
	fmt.Fprintf(&b, "## Recent Activity (last 24h)\n")
	if len(events) == 0 {
		b.WriteString("No events found in the last 24 hours.\n")
	}
	for _, e := range events {
		fmt.Fprintf(&b, "- %s %s: %s\n", e.ActionName, e.TargetType, e.TargetTitle)
	}

	// Authored MRs section
	writeMRSection(&b, "Open MRs by", username, authoredMRs, mrAuthorErr)

	// Assigned MRs section
	writeMRSection(&b, "MRs Assigned to", username, assignedMRs, mrAssignedErr)

	// Reviewer MRs section
	writeMRSection(&b, "MRs Under Review by", username, reviewMRs, mrReviewErr)

	// Assigned issues section
	writeIssueSection(&b, "Issues Assigned to", username, assignedIssues, issueAssignedErr)

	// Created issues section
	writeIssueSection(&b, "Issues Created by", username, createdIssues, issueCreatedErr)

	b.WriteString("\nPlease generate a concise standup report with: What was done yesterday, What is planned for today, and Any blockers.")

	return promptResult(b.String()), nil
}

// resolveUser resolves a username to its ID. If username is empty, the current
// authenticated user is used. Returns the username, user ID, whether the user
// is the current authenticated user, and any error.
func resolveUser(ctx context.Context, client *gitlabclient.Client, username string) (resolvedName string, userID int64, isSelf bool, err error) {
	if username == "" {
		u, _, curErr := client.GL().Users.CurrentUser(gl.WithContext(ctx))
		if curErr != nil {
			return "", 0, false, fmt.Errorf("failed to get current user: %w", curErr)
		}
		return u.Username, u.ID, true, nil
	}

	users, _, err := client.GL().Users.ListUsers(&gl.ListUsersOptions{
		Username: new(username),
	}, gl.WithContext(ctx))
	if err != nil {
		return "", 0, false, fmt.Errorf("failed to look up user %q: %w", username, err)
	}
	if len(users) == 0 {
		return "", 0, false, fmt.Errorf("user %q not found", username)
	}
	return users[0].Username, users[0].ID, false, nil
}

// writeMRSection appends a merge request section to the builder. If fetchErr
// is non-nil the section is silently skipped (non-fatal).
func writeMRSection(b *strings.Builder, heading, username string, mrs []*gl.BasicMergeRequest, fetchErr error) {
	if fetchErr != nil {
		slog.Warn("skipping MR section due to API error", "heading", heading, "error", fetchErr)
		return
	}
	if len(mrs) == 0 {
		return
	}
	fmt.Fprintf(b, "\n## %s @%s (%d)\n", heading, username, len(mrs))
	for _, mr := range mrs {
		fmt.Fprintf(b, "- !%d: %s (%s → %s) — %s\n", mr.IID, mr.Title, mr.SourceBranch, mr.TargetBranch, mr.DetailedMergeStatus)
	}
}

// writeIssueSection appends an issue section to the builder. If fetchErr
// is non-nil the section is silently skipped (non-fatal).
func writeIssueSection(b *strings.Builder, heading, username string, issues []*gl.Issue, fetchErr error) {
	if fetchErr != nil {
		slog.Warn("skipping issue section due to API error", "heading", heading, "error", fetchErr)
		return
	}
	if len(issues) == 0 {
		return
	}
	fmt.Fprintf(b, "\n## %s @%s (%d)\n", heading, username, len(issues))
	for _, issue := range issues {
		age := time.Since(*issue.CreatedAt) / (24 * time.Hour)
		fmt.Fprintf(b, "- #%d: %s (opened %d days ago)\n", issue.IID, issue.Title, age)
	}
}

// registerTeamMemberWorkloadPrompt registers the team_member_workload prompt.
func registerTeamMemberWorkloadPrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "team_member_workload",
		Title:       toolutil.TitleFromName("team_member_workload"),
		Description: "Generate a comprehensive workload summary for a specific team member over a configurable time period. Includes contribution events, authored and assigned merge requests, MRs under review, authored and assigned issues. Use this for team management and capacity planning.",
		Icons:       toolutil.IconUser,
		Arguments: []*mcp.PromptArgument{
			projectIDArg(),
			{Name: "username", Title: toolutil.TitleFromName("username"), Description: "GitLab username of the team member to analyze", Required: true},
			{Name: "days", Title: toolutil.TitleFromName("days"), Description: "Number of days to look back for activity (default: 7)", Required: false},
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleTeamMemberWorkload(ctx, client, req)
	})
}

// handleTeamMemberWorkload aggregates MR and issue counts for a user to assess
// their current workload.
func handleTeamMemberWorkload(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	projectID := req.Params.Arguments[argProjectID]
	if projectID == "" {
		return nil, fmt.Errorf(fmtOneArgRequired, argProjectID)
	}

	usernameArg := req.Params.Arguments["username"]
	if usernameArg == "" {
		return nil, errors.New("argument 'username' is required")
	}

	days := 7
	if d := req.Params.Arguments["days"]; d != "" {
		parsed, err := strconv.Atoi(d)
		if err != nil || parsed <= 0 {
			return nil, fmt.Errorf("argument 'days' must be a positive integer, got %q", d)
		}
		days = parsed
	}

	username, userID, isCurrentUser, err := resolveUser(ctx, client, usernameArg)
	if err != nil {
		return nil, err
	}

	since := time.Now().AddDate(0, 0, -days)

	// Contribution events over the period
	eventOpts := &gl.ListContributionEventsOptions{
		After: new(gl.ISOTime(since)),
	}
	var events []*gl.ContributionEvent
	if isCurrentUser {
		events, _, err = client.GL().Events.ListCurrentUserContributionEvents(eventOpts, gl.WithContext(ctx))
	} else {
		events, _, err = client.GL().Users.ListUserContributionEvents(userID, eventOpts, gl.WithContext(ctx))
	}
	if err != nil {
		slog.Warn("failed to fetch contribution events", "error", err)
	}

	// Authored MRs (open)
	openAuthoredMRs, _, errOpenAuthored := client.GL().MergeRequests.ListProjectMergeRequests(projectID, &gl.ListProjectMergeRequestsOptions{
		State:          new("opened"),
		AuthorUsername: new(username),
	}, gl.WithContext(ctx))

	// Authored MRs (merged recently)
	mergedAuthoredMRs, _, errMergedAuthored := client.GL().MergeRequests.ListProjectMergeRequests(projectID, &gl.ListProjectMergeRequestsOptions{
		State:          new("merged"),
		AuthorUsername: new(username),
		CreatedAfter:   new(since),
	}, gl.WithContext(ctx))

	// Assigned MRs (open)
	assignedMRs, _, errAssigned := client.GL().MergeRequests.ListProjectMergeRequests(projectID, &gl.ListProjectMergeRequestsOptions{
		State:      new("opened"),
		AssigneeID: gl.AssigneeID(userID),
	}, gl.WithContext(ctx))

	// MRs under review
	reviewMRs, _, errReview := client.GL().MergeRequests.ListProjectMergeRequests(projectID, &gl.ListProjectMergeRequestsOptions{
		State:            new("opened"),
		ReviewerUsername: new(username),
	}, gl.WithContext(ctx))

	// Authored issues (open)
	authoredIssues, _, errAuthoredIssues := client.GL().Issues.ListProjectIssues(projectID, &gl.ListProjectIssuesOptions{
		State:          new("opened"),
		AuthorUsername: new(username),
	}, gl.WithContext(ctx))

	// Assigned issues (open)
	assignedIssues, _, errAssignedIssues := client.GL().Issues.ListProjectIssues(projectID, &gl.ListProjectIssuesOptions{
		State:            new("opened"),
		AssigneeUsername: new(username),
	}, gl.WithContext(ctx))

	var b strings.Builder
	fmt.Fprintf(&b, "# Workload Summary for @%s (last %d days)\n\n", username, days)

	// Activity summary
	b.WriteString("## Contribution Events\n")
	if len(events) == 0 {
		b.WriteString("No contribution events found in this period.\n")
	} else {
		eventTypes := countEventTypes(events)
		fmt.Fprintf(&b, "Total events: %d\n", len(events))
		for evType, count := range eventTypes {
			fmt.Fprintf(&b, "- %s: %d\n", evType, count)
		}
	}

	// MR sections
	writeMRSection(&b, "Open MRs Authored by", username, openAuthoredMRs, errOpenAuthored)
	writeMRSection(&b, "Recently Merged MRs by", username, mergedAuthoredMRs, errMergedAuthored)
	writeMRSection(&b, "Open MRs Assigned to", username, assignedMRs, errAssigned)
	writeMRSection(&b, "MRs Under Review by", username, reviewMRs, errReview)

	// Issue sections
	writeIssueSection(&b, "Open Issues Created by", username, authoredIssues, errAuthoredIssues)
	writeIssueSection(&b, "Open Issues Assigned to", username, assignedIssues, errAssignedIssues)

	// Summary counts for quick scanning
	b.WriteString("\n## Quick Summary\n")
	b.WriteString(tableCategoryHeader)
	b.WriteString(tableCategorySeparator)
	writeCountRow(&b, "Contribution events", len(events), nil)
	writeCountRow(&b, "Open MRs authored", len(openAuthoredMRs), errOpenAuthored)
	writeCountRow(&b, "Recently merged MRs", len(mergedAuthoredMRs), errMergedAuthored)
	writeCountRow(&b, "MRs assigned", len(assignedMRs), errAssigned)
	writeCountRow(&b, "MRs under review", len(reviewMRs), errReview)
	writeCountRow(&b, "Issues authored", len(authoredIssues), errAuthoredIssues)
	writeCountRow(&b, "Issues assigned", len(assignedIssues), errAssignedIssues)

	b.WriteString("\nBased on the above workload data, provide an assessment of the team member's current capacity, highlight any overload risks, and suggest priorities.")

	return promptResult(b.String()), nil
}

// countEventTypes groups contribution events by action name.
func countEventTypes(events []*gl.ContributionEvent) map[string]int {
	counts := make(map[string]int)
	for _, e := range events {
		counts[e.ActionName]++
	}
	return counts
}

// writeCountRow appends a table row; if fetchErr is non-nil, shows "N/A".
func writeCountRow(b *strings.Builder, label string, count int, fetchErr error) {
	if fetchErr != nil {
		fmt.Fprintf(b, "| %s | N/A |\n", label)
		return
	}
	fmt.Fprintf(b, fmtTableCountRow, label, count)
}

// registerUserStatsPrompt registers the user_stats prompt.
func registerUserStatsPrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "user_stats",
		Title:       toolutil.TitleFromName("user_stats"),
		Description: "Generate comprehensive user statistics from GitLab: contribution events breakdown, merge request stats (authored/assigned/reviewed by state), issue stats (authored/assigned by state), daily activity trends, and a Mermaid activity chart. Use this for performance reviews, productivity tracking, or personal dashboards.",
		Icons:       toolutil.IconUser,
		Arguments: []*mcp.PromptArgument{
			projectIDArg(),
			{Name: "username", Title: toolutil.TitleFromName("username"), Description: "GitLab username to generate stats for (defaults to the authenticated user if omitted)", Required: false},
			{Name: "days", Title: toolutil.TitleFromName("days"), Description: "Number of days to look back for activity (default: 30)", Required: false},
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleUserStats(ctx, client, req)
	})
}

// handleUserStats generates comprehensive user statistics including contribution
// events, MR/issue breakdowns, and daily activity trends.
func handleUserStats(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	projectID := req.Params.Arguments[argProjectID]
	if projectID == "" {
		return nil, fmt.Errorf(fmtOneArgRequired, argProjectID)
	}

	days := 30
	if d := req.Params.Arguments["days"]; d != "" {
		parsed, err := strconv.Atoi(d)
		if err != nil || parsed <= 0 {
			return nil, fmt.Errorf("argument 'days' must be a positive integer, got %q", d)
		}
		days = parsed
	}

	username, userID, isCurrentUser, err := resolveUser(ctx, client, req.Params.Arguments["username"])
	if err != nil {
		return nil, err
	}

	since := time.Now().AddDate(0, 0, -days)

	// Contribution events
	eventOpts := &gl.ListContributionEventsOptions{
		After: new(gl.ISOTime(since)),
	}
	var events []*gl.ContributionEvent
	if isCurrentUser {
		events, _, err = client.GL().Events.ListCurrentUserContributionEvents(eventOpts, gl.WithContext(ctx))
	} else {
		events, _, err = client.GL().Users.ListUserContributionEvents(userID, eventOpts, gl.WithContext(ctx))
	}
	if err != nil {
		slog.Warn("failed to fetch contribution events", "error", err)
	}

	// MR stats: open, merged, closed
	openMRs, _, errOpenMRs := client.GL().MergeRequests.ListProjectMergeRequests(projectID, &gl.ListProjectMergeRequestsOptions{
		State:          new("opened"),
		AuthorUsername: new(username),
	}, gl.WithContext(ctx))

	mergedMRs, _, errMergedMRs := client.GL().MergeRequests.ListProjectMergeRequests(projectID, &gl.ListProjectMergeRequestsOptions{
		State:          new("merged"),
		AuthorUsername: new(username),
		CreatedAfter:   new(since),
	}, gl.WithContext(ctx))

	closedMRs, _, errClosedMRs := client.GL().MergeRequests.ListProjectMergeRequests(projectID, &gl.ListProjectMergeRequestsOptions{
		State:          new("closed"),
		AuthorUsername: new(username),
		CreatedAfter:   new(since),
	}, gl.WithContext(ctx))

	// MRs assigned & under review
	assignedMRs, _, errAssignedMRs := client.GL().MergeRequests.ListProjectMergeRequests(projectID, &gl.ListProjectMergeRequestsOptions{
		State:      new("opened"),
		AssigneeID: gl.AssigneeID(userID),
	}, gl.WithContext(ctx))

	reviewMRs, _, errReviewMRs := client.GL().MergeRequests.ListProjectMergeRequests(projectID, &gl.ListProjectMergeRequestsOptions{
		State:            new("opened"),
		ReviewerUsername: new(username),
	}, gl.WithContext(ctx))

	// Issue stats: open authored, open assigned, closed authored
	openAuthoredIssues, _, errOpenAuthored := client.GL().Issues.ListProjectIssues(projectID, &gl.ListProjectIssuesOptions{
		State:          new("opened"),
		AuthorUsername: new(username),
	}, gl.WithContext(ctx))

	openAssignedIssues, _, errOpenAssigned := client.GL().Issues.ListProjectIssues(projectID, &gl.ListProjectIssuesOptions{
		State:            new("opened"),
		AssigneeUsername: new(username),
	}, gl.WithContext(ctx))

	closedAuthoredIssues, _, errClosedAuthored := client.GL().Issues.ListProjectIssues(projectID, &gl.ListProjectIssuesOptions{
		State:          new("closed"),
		AuthorUsername: new(username),
		CreatedAfter:   new(since),
	}, gl.WithContext(ctx))

	var b strings.Builder
	fmt.Fprintf(&b, "# User Statistics for @%s (last %d days)\n\n", username, days)

	// Activity summary
	b.WriteString("## Activity Summary\n")
	writeEventSummary(&b, events)

	// MR stats table
	b.WriteString("\n## Merge Request Stats\n\n")
	b.WriteString(tableCategoryHeader)
	b.WriteString(tableCategorySeparator)
	writeCountRow(&b, "Open (authored)", len(openMRs), errOpenMRs)
	writeCountRow(&b, "Merged (authored)", len(mergedMRs), errMergedMRs)
	writeCountRow(&b, "Closed (authored)", len(closedMRs), errClosedMRs)
	writeCountRow(&b, "Assigned (open)", len(assignedMRs), errAssignedMRs)
	writeCountRow(&b, "Under review", len(reviewMRs), errReviewMRs)

	// Issue stats table
	b.WriteString("\n## Issue Stats\n\n")
	b.WriteString(tableCategoryHeader)
	b.WriteString(tableCategorySeparator)
	writeCountRow(&b, "Open (authored)", len(openAuthoredIssues), errOpenAuthored)
	writeCountRow(&b, "Open (assigned)", len(openAssignedIssues), errOpenAssigned)
	writeCountRow(&b, "Closed (authored)", len(closedAuthoredIssues), errClosedAuthored)

	// Daily activity chart data
	if len(events) > 0 {
		dailyActivity := groupEventsByDay(events)
		writeDailyActivity(&b, username, dailyActivity)
	}

	// Overall summary table
	b.WriteString("\n## Overall Summary\n\n")
	b.WriteString("| Metric | Value |\n")
	b.WriteString("|--------|-------|\n")
	fmt.Fprintf(&b, "| Period | %d days |\n", days)
	fmt.Fprintf(&b, "| Total events | %d |\n", len(events))
	totalMRs := safeLen(len(openMRs), errOpenMRs) + safeLen(len(mergedMRs), errMergedMRs) + safeLen(len(closedMRs), errClosedMRs)
	fmt.Fprintf(&b, "| Total MRs (authored) | %d |\n", totalMRs)
	totalIssues := safeLen(len(openAuthoredIssues), errOpenAuthored) + safeLen(len(closedAuthoredIssues), errClosedAuthored)
	fmt.Fprintf(&b, "| Total issues (authored) | %d |\n", totalIssues)

	b.WriteString("\nBased on the above statistics, provide a comprehensive analysis of the user's productivity, highlight strengths and areas for improvement, and identify any trends in activity.")

	return promptResult(b.String()), nil
}

// writeEventSummary writes the contribution event breakdown table into the builder.
func writeEventSummary(b *strings.Builder, events []*gl.ContributionEvent) {
	if len(events) == 0 {
		b.WriteString("No contribution events found in this period.\n")
		return
	}
	eventTypes := countEventTypes(events)
	fmt.Fprintf(b, "Total events: %d\n\n", len(events))
	b.WriteString("| Action | Count |\n")
	b.WriteString("|--------|-------|\n")
	for evType, count := range eventTypes {
		fmt.Fprintf(b, fmtTableCountRow, evType, count)
	}
}

// writeDailyActivity writes the daily activity table and Mermaid bar chart
// into the provided builder.
func writeDailyActivity(b *strings.Builder, username string, dailyActivity []dayActivity) {
	b.WriteString("\n## Daily Activity\n\n")
	b.WriteString("| Date | Events |\n")
	b.WriteString("|------|--------|\n")
	for _, da := range dailyActivity {
		fmt.Fprintf(b, fmtTableCountRow, da.date, da.count)
	}

	b.WriteString("\n## Activity Chart\n\n")
	b.WriteString("```mermaid\n")
	b.WriteString("xychart-beta\n")
	fmt.Fprintf(b, "    title \"Daily Activity for @%s\"\n", username)
	b.WriteString("    x-axis [")
	for i, da := range dailyActivity {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(b, "\"%s\"", da.date)
	}
	b.WriteString("]\n")
	b.WriteString("    y-axis \"Events\"\n")
	b.WriteString("    bar [")
	for i, da := range dailyActivity {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(b, "%d", da.count)
	}
	b.WriteString("]\n")
	b.WriteString("```\n")
}

// dayActivity holds the event count for a single calendar day.
type dayActivity struct {
	date  string
	count int
}

// groupEventsByDay aggregates events by date (YYYY-MM-DD), sorted chronologically.
func groupEventsByDay(events []*gl.ContributionEvent) []dayActivity {
	counts := make(map[string]int)
	for _, e := range events {
		if e.CreatedAt == nil {
			continue
		}
		day := e.CreatedAt.Format("2006-01-02")
		counts[day]++
	}

	days := make([]string, 0, len(counts))
	for d := range counts {
		days = append(days, d)
	}
	slices.Sort(days)

	result := make([]dayActivity, len(days))
	for i, d := range days {
		result[i] = dayActivity{date: d, count: counts[d]}
	}
	return result
}

// safeLen returns the count if there was no error, otherwise 0.
func safeLen(count int, err error) int {
	if err != nil {
		return 0
	}
	return count
}

// registerMRRiskAssessmentPrompt registers the mr_risk_assessment prompt.
func registerMRRiskAssessmentPrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "mr_risk_assessment",
		Title:       toolutil.TitleFromName("mr_risk_assessment"),
		Description: "Assess the risk level (LOW/MEDIUM/HIGH/CRITICAL) of a merge request based on size (lines added/removed), number of changed files, new/deleted files, sensitive file patterns (env, auth, migration, CI, security), and conflict status.",
		Icons:       toolutil.IconMR,
		Arguments: []*mcp.PromptArgument{
			projectIDArg(),
			mrIIDArg(),
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleMRRiskAssessment(ctx, client, req)
	})
}

// handleMRRiskAssessment computes diff metrics and identifies sensitive files
// to produce a merge request risk assessment.
func handleMRRiskAssessment(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	projectID := req.Params.Arguments[argProjectID]
	mrIID := req.Params.Arguments[argMRIID]
	if projectID == "" || mrIID == "" {
		return nil, fmt.Errorf(fmtTwoArgsRequired, argProjectID, argMRIID)
	}

	iid := parseIID(mrIID)
	mr, _, err := client.GL().MergeRequests.GetMergeRequest(projectID, iid, &gl.GetMergeRequestsOptions{}, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf(fmtGetMRFailed, err)
	}

	diffs, _, err := client.GL().MergeRequests.ListMergeRequestDiffs(projectID, iid, nil, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf(fmtGetMRDiffsFailed, err)
	}

	m := computeDiffMetrics(diffs)

	var b strings.Builder
	fmt.Fprintf(&b, "# MR Risk Assessment: !%d — %s\n\n", mr.IID, mr.Title)
	fmt.Fprintf(&b, "## Metrics\n")
	fmt.Fprintf(&b, fmtFilesChanged, len(diffs))
	fmt.Fprintf(&b, "- **Lines added**: %d\n", m.additions)
	fmt.Fprintf(&b, "- **Lines removed**: %d\n", m.deletions)
	fmt.Fprintf(&b, "- **New files**: %d\n", m.newFiles)
	fmt.Fprintf(&b, "- **Deleted files**: %d\n", m.deletedFiles)
	fmt.Fprintf(&b, "- **Sensitive files touched**: %d\n", m.sensitiveFiles)
	fmt.Fprintf(&b, "- **Has conflicts**: %v\n", mr.HasConflicts)

	fmt.Fprintf(&b, "\n## Changed Files\n")
	for _, d := range diffs {
		fmt.Fprintf(&b, fmtListItem, d.NewPath, changeType(d))
	}

	b.WriteString("\nBased on the above metrics, provide a risk assessment (LOW / MEDIUM / HIGH / CRITICAL) with justification and recommendations.")

	return promptResult(b.String()), nil
}

// diffMetrics holds aggregated statistics computed from merge request diffs.
type diffMetrics struct {
	additions      int
	deletions      int
	newFiles       int
	deletedFiles   int
	sensitiveFiles int
}

// computeDiffMetrics aggregates line additions, deletions, new/deleted files,
// and sensitive file counts from a set of merge request diffs.
func computeDiffMetrics(diffs []*gl.MergeRequestDiff) diffMetrics {
	var m diffMetrics
	for _, d := range diffs {
		add, del := countDiffLines(d.Diff)
		m.additions += add
		m.deletions += del
		if d.NewFile {
			m.newFiles++
		}
		if d.DeletedFile {
			m.deletedFiles++
		}
		if isSensitivePath(d.NewPath) {
			m.sensitiveFiles++
		}
	}
	return m
}

// countDiffLines counts addition and deletion lines in a unified diff string.
func countDiffLines(diff string) (additions, deletions int) {
	for l := range strings.SplitSeq(diff, "\n") {
		if strings.HasPrefix(l, "+") && !strings.HasPrefix(l, "+++") {
			additions++
		} else if strings.HasPrefix(l, "-") && !strings.HasPrefix(l, "---") {
			deletions++
		}
	}
	return
}

// isSensitivePath reports whether a file path matches known sensitive patterns
// such as env, secrets, migrations, auth, or CI configuration.
func isSensitivePath(path string) bool {
	sensitivePatterns := []string{
		".env", "secret", "password", "token", "key",
		"migration", "schema", "docker", "ci", "deploy",
		"security", "auth", "config",
	}
	lower := strings.ToLower(path)
	for _, pattern := range sensitivePatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

// isTestPath reports whether a file path indicates a test file.
func isTestPath(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, "_test.go") ||
		strings.Contains(lower, "test/") ||
		strings.Contains(lower, "tests/") ||
		strings.Contains(lower, "spec/") ||
		strings.Contains(lower, "__tests__/")
}

// isDocPath reports whether a file path indicates a documentation file.
func isDocPath(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".md") ||
		strings.Contains(lower, "readme") ||
		strings.Contains(lower, "docs/") ||
		strings.Contains(lower, "doc/") ||
		strings.HasSuffix(lower, ".txt") ||
		strings.HasSuffix(lower, ".rst")
}

// categorizeDiffs groups diffs into four review categories by priority.
func categorizeDiffs(diffs []*gl.MergeRequestDiff) (highRisk, logic, tests, docs []*gl.MergeRequestDiff) {
	for _, d := range diffs {
		switch {
		case isSensitivePath(d.NewPath):
			highRisk = append(highRisk, d)
		case isTestPath(d.NewPath):
			tests = append(tests, d)
		case isDocPath(d.NewPath):
			docs = append(docs, d)
		default:
			logic = append(logic, d)
		}
	}
	return
}

// writeDiffGroup writes a section with per-file metrics and full diffs.
func writeDiffGroup(b *strings.Builder, heading string, diffs []*gl.MergeRequestDiff) {
	if len(diffs) == 0 {
		return
	}
	fmt.Fprintf(b, "\n## %s (%d)\n\n", heading, len(diffs))
	for _, d := range diffs {
		add, del := countDiffLines(d.Diff)
		fmt.Fprintf(b, "### %s (%s) — +%d / -%d lines\n", d.NewPath, changeType(d), add, del)
		if d.Diff != "" {
			fmt.Fprintf(b, "```diff\n%s\n```\n\n", d.Diff)
		}
	}
}

// Prompt helpers.

// parseIID converts a string IID to int64.
func parseIID(s string) int64 {
	var iid int64
	_, _ = fmt.Sscanf(s, "%d", &iid)
	return iid
}

// changeType returns a human-readable label for a diff entry's change type.
func changeType(c *gl.MergeRequestDiff) string {
	switch {
	case c.NewFile:
		return "new file"
	case c.RenamedFile:
		return "renamed"
	case c.DeletedFile:
		return "deleted"
	default:
		return "modified"
	}
}

// shortSHA truncates a commit SHA to 8 characters for compact display.
func shortSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

// promptResult builds a standard GetPromptResult with a single assistant message.
func promptResult(text string) *mcp.GetPromptResult {
	return &mcp.GetPromptResult{
		Messages: []*mcp.PromptMessage{{
			Content: &mcp.TextContent{Text: text},
			Role:    "assistant",
		}},
	}
}
