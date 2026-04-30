// prompt_project_reports.go registers MCP prompts for project-level reporting and summaries.
package prompts

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const (
	mdSummaryHeader       = "## Summary\n\n"
	mdCategoryTableHeader = "| Category | Count |\n|----------|-------|\n"
)

// registerProjectReportPrompts registers all project-level report prompts.
func registerProjectReportPrompts(server *mcp.Server, client *gitlabclient.Client) {
	registerBranchMRSummaryPrompt(server, client)
	registerProjectActivityReportPrompt(server, client)
	registerMRReviewStatusPrompt(server, client)
	registerUnassignedItemsPrompt(server, client)
	registerStaleItemsReportPrompt(server, client)
}

// registerBranchMRSummaryPrompt registers the branch_mr_summary prompt.
func registerBranchMRSummaryPrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "branch_mr_summary",
		Title:       toolutil.TitleFromName("branch_mr_summary"),
		Description: "List all MRs targeting a specific branch in a project. Shows readiness summary with conflict/draft/approval counts. Ideal for release branch reviews.",
		Icons:       toolutil.IconBranch,
		Arguments: []*mcp.PromptArgument{
			projectIDArg(),
			targetBranchArg(true),
			stateArg("opened"),
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleBranchMRSummary(ctx, client, req)
	})
}

// handleBranchMRSummary performs the handle branch m r summary operation using the GitLab API and returns [*mcp.GetPromptResult].
func handleBranchMRSummary(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	projectID := req.Params.Arguments[argProjectID]
	if projectID == "" {
		return nil, errors.New("branch_mr_summary: project_id is required")
	}
	targetBranch := req.Params.Arguments[argTargetBranch]
	if targetBranch == "" {
		return nil, errors.New("branch_mr_summary: target_branch is required")
	}
	state := getArgOr(req.Params.Arguments, argState, "opened")

	mrs, _, err := client.GL().MergeRequests.ListProjectMergeRequests(projectID, &gl.ListProjectMergeRequestsOptions{
		TargetBranch: new(targetBranch),
		State:        new(state),
		ListOptions:  gl.ListOptions{PerPage: maxListItems},
	}, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("branch_mr_summary: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# MRs targeting %s in %s (%d %s)\n\n", targetBranch, projectID, len(mrs), state)

	if len(mrs) == 0 {
		b.WriteString("No merge requests found matching the criteria.\n")
		return promptResult(b.String()), nil
	}

	var drafts, conflicts int
	for _, mr := range mrs {
		if mr.Draft {
			drafts++
		}
		if mr.HasConflicts {
			conflicts++
		}
	}

	b.WriteString(mdSummaryHeader)
	b.WriteString(mdCategoryTableHeader)
	fmt.Fprintf(&b, "| Total | %d |\n", len(mrs))
	fmt.Fprintf(&b, "| Draft | %d |\n", drafts)
	fmt.Fprintf(&b, "| With conflicts | %d |\n", conflicts)
	b.WriteString("\n## Merge Requests\n\n")

	writeMRTable(&b, mrs)

	b.WriteString("\n---\nPlease summarize the readiness of these MRs for merging, highlight blockers (conflicts, drafts), and suggest priorities.\n")

	return promptResult(b.String()), nil
}

// registerProjectActivityReportPrompt registers the project_activity_report prompt.
func registerProjectActivityReportPrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "project_activity_report",
		Title:       toolutil.TitleFromName("project_activity_report"),
		Description: "Generate a project activity report including recent events, merged MRs, and open issues. Shows daily activity chart and contributor breakdown.",
		Icons:       toolutil.IconAnalytics,
		Arguments: []*mcp.PromptArgument{
			projectIDArg(),
			daysArg(7),
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleProjectActivityReport(ctx, client, req)
	})
}

// handleProjectActivityReport performs the handle project activity report operation using the GitLab API and returns [*mcp.GetPromptResult].
func handleProjectActivityReport(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	projectID := req.Params.Arguments[argProjectID]
	if projectID == "" {
		return nil, errors.New("project_activity_report: project_id is required")
	}
	days := parseDays(getArgOr(req.Params.Arguments, argDays, "7"), 7)
	since := sinceDate(days)
	sinceISO := gl.ISOTime(since)

	// Project events
	events, _, err := client.GL().Events.ListProjectVisibleEvents(projectID, &gl.ListProjectVisibleEventsOptions{
		After: &sinceISO,
	}, gl.WithContext(ctx))
	if err != nil {
		slog.Warn("failed to fetch project events", "error", err)
	}

	// Merged MRs in the period
	mergedMRs, _, _ := client.GL().MergeRequests.ListProjectMergeRequests(projectID, &gl.ListProjectMergeRequestsOptions{
		State:        new("merged"),
		CreatedAfter: &since,
		ListOptions:  gl.ListOptions{PerPage: maxListItems},
	}, gl.WithContext(ctx))

	// Open MRs
	openMRs, _, _ := client.GL().MergeRequests.ListProjectMergeRequests(projectID, &gl.ListProjectMergeRequestsOptions{
		State:       new("opened"),
		ListOptions: gl.ListOptions{PerPage: maxListItems},
	}, gl.WithContext(ctx))

	// Open issues
	openIssues, _, _ := client.GL().Issues.ListProjectIssues(projectID, &gl.ListProjectIssuesOptions{
		State:       new("opened"),
		ListOptions: gl.ListOptions{PerPage: maxListItems},
	}, gl.WithContext(ctx))

	var b strings.Builder
	fmt.Fprintf(&b, "# Project Activity Report — %s (last %d days)\n\n", projectID, days)

	// Summary
	b.WriteString(mdSummaryHeader)
	b.WriteString(mdCategoryTableHeader)
	fmt.Fprintf(&b, "| Events | %d |\n", len(events))
	fmt.Fprintf(&b, "| Merged MRs | %d |\n", len(mergedMRs))
	fmt.Fprintf(&b, "| Open MRs | %d |\n", len(openMRs))
	fmt.Fprintf(&b, "| Open issues | %d |\n", len(openIssues))
	b.WriteString("\n")

	// Event breakdown by type
	if len(events) > 0 {
		b.WriteString("## Event Breakdown\n\n")
		eventTypes := make(map[string]int)
		for _, e := range events {
			eventTypes[e.ActionName]++
		}
		b.WriteString("| Action | Count |\n|--------|-------|\n")
		for _, k := range sortedKeys(eventTypes) {
			fmt.Fprintf(&b, "| %s | %d |\n", k, eventTypes[k])
		}
		b.WriteString("\n")
	}

	// Recently merged MRs
	if len(mergedMRs) > 0 {
		b.WriteString("## Recently Merged MRs\n\n")
		writeMRTable(&b, mergedMRs)
		b.WriteString("\n")
	}

	b.WriteString("---\nPlease analyze the project activity, highlight trends, and identify areas needing attention.\n")

	return promptResult(b.String()), nil
}

// registerMRReviewStatusPrompt registers the mr_review_status prompt.
func registerMRReviewStatusPrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "mr_review_status",
		Title:       toolutil.TitleFromName("mr_review_status"),
		Description: "Analyze the discussion health of open MRs in a project. Shows unresolved thread counts per MR to identify items needing attention.",
		Icons:       toolutil.IconMR,
		Arguments: []*mcp.PromptArgument{
			projectIDArg(),
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleMRReviewStatus(ctx, client, req)
	})
}

// mrDiscussionInfo holds per-MR discussion thread statistics.
type mrDiscussionInfo struct {
	iid        int64
	title      string
	author     string
	threads    int
	unresolved int
}

// handleMRReviewStatus performs the handle m r review status operation using the GitLab API and returns [*mcp.GetPromptResult].
func handleMRReviewStatus(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	projectID := req.Params.Arguments[argProjectID]
	if projectID == "" {
		return nil, errors.New("mr_review_status: project_id is required")
	}

	mrs, _, err := client.GL().MergeRequests.ListProjectMergeRequests(projectID, &gl.ListProjectMergeRequestsOptions{
		State:       new("opened"),
		ListOptions: gl.ListOptions{PerPage: 20},
	}, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("mr_review_status: %w", err)
	}

	infos := collectMRDiscussionInfos(ctx, client, projectID, mrs)

	var b strings.Builder
	fmt.Fprintf(&b, "# MR Review Status — %s (%d open MRs)\n\n", projectID, len(mrs))

	if len(infos) == 0 {
		b.WriteString("No open merge requests found.\n")
		return promptResult(b.String()), nil
	}

	// Summary
	totalUnresolved := 0
	mrsWithUnresolved := 0
	for _, info := range infos {
		totalUnresolved += info.unresolved
		if info.unresolved > 0 {
			mrsWithUnresolved++
		}
	}
	b.WriteString(mdSummaryHeader)
	b.WriteString(mdCategoryTableHeader)
	fmt.Fprintf(&b, "| Open MRs | %d |\n", len(infos))
	fmt.Fprintf(&b, "| MRs with unresolved threads | %d |\n", mrsWithUnresolved)
	fmt.Fprintf(&b, "| Total unresolved threads | %d |\n", totalUnresolved)
	b.WriteString("\n")

	// Table
	b.WriteString("## Discussion Details\n\n")
	b.WriteString("| MR | Title | Author | Threads | Unresolved |\n")
	b.WriteString("|----|-------|--------|---------|------------|\n")
	for _, info := range infos {
		fmt.Fprintf(&b, "| !%d | %s | @%s | %d | %d |\n", info.iid, info.title, info.author, info.threads, info.unresolved)
	}
	b.WriteString("\n")

	b.WriteString("---\nPlease identify MRs with the most unresolved threads, assess review health, and suggest actions.\n")

	return promptResult(b.String()), nil
}

// collectMRDiscussionInfos fetches discussion thread counts for each MR.
func collectMRDiscussionInfos(ctx context.Context, client *gitlabclient.Client, projectID string, mrs []*gl.BasicMergeRequest) []mrDiscussionInfo {
	infos := make([]mrDiscussionInfo, 0, len(mrs))
	for _, mr := range mrs {
		info := mrDiscussionInfo{
			iid:   mr.IID,
			title: mr.Title,
		}
		if mr.Author != nil {
			info.author = mr.Author.Username
		}
		discussions, _, dErr := client.GL().Discussions.ListMergeRequestDiscussions(projectID, mr.IID, &gl.ListMergeRequestDiscussionsOptions{
			ListOptions: gl.ListOptions{PerPage: maxListItems},
		}, gl.WithContext(ctx))
		if dErr != nil {
			slog.Warn("failed to fetch discussions", "merge_request_iid", mr.IID, "error", dErr)
			infos = append(infos, info)
			continue
		}
		countDiscussionThreads(&info, discussions)
		infos = append(infos, info)
	}
	return infos
}

// countDiscussionThreads tallies resolvable and unresolved notes in discussions.
func countDiscussionThreads(info *mrDiscussionInfo, discussions []*gl.Discussion) {
	for _, d := range discussions {
		for _, n := range d.Notes {
			if n.Resolvable {
				info.threads++
				if !n.Resolved {
					info.unresolved++
				}
			}
		}
	}
}

// registerUnassignedItemsPrompt registers the unassigned_items prompt.
func registerUnassignedItemsPrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "unassigned_items",
		Title:       toolutil.TitleFromName("unassigned_items"),
		Description: "Find open MRs and issues in a project that have no assignee. Helps identify ownership gaps and items needing attention.",
		Icons:       toolutil.IconIssue,
		Arguments: []*mcp.PromptArgument{
			projectIDArg(),
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleUnassignedItems(ctx, client, req)
	})
}

// handleUnassignedItems performs the handle unassigned items operation using the GitLab API and returns [*mcp.GetPromptResult].
func handleUnassignedItems(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	projectID := req.Params.Arguments[argProjectID]
	if projectID == "" {
		return nil, errors.New("unassigned_items: project_id is required")
	}

	// Unassigned MRs
	unassignedMRs, _, _ := client.GL().MergeRequests.ListProjectMergeRequests(projectID, &gl.ListProjectMergeRequestsOptions{
		State:       new("opened"),
		AssigneeID:  gl.AssigneeID(0),
		ListOptions: gl.ListOptions{PerPage: maxListItems},
	}, gl.WithContext(ctx))

	// Unassigned issues
	unassignedIssues, _, _ := client.GL().Issues.ListProjectIssues(projectID, &gl.ListProjectIssuesOptions{
		State:       new("opened"),
		AssigneeID:  gl.AssigneeID(0),
		ListOptions: gl.ListOptions{PerPage: maxListItems},
	}, gl.WithContext(ctx))

	var b strings.Builder
	fmt.Fprintf(&b, "# Unassigned Items — %s\n\n", projectID)

	b.WriteString(mdSummaryHeader)
	b.WriteString(mdCategoryTableHeader)
	fmt.Fprintf(&b, "| Unassigned MRs | %d |\n", len(unassignedMRs))
	fmt.Fprintf(&b, "| Unassigned issues | %d |\n", len(unassignedIssues))
	b.WriteString("\n")

	if len(unassignedMRs) > 0 {
		b.WriteString("## Unassigned Merge Requests\n\n")
		writeMRTable(&b, unassignedMRs)
		b.WriteString("\n")
	}

	if len(unassignedIssues) > 0 {
		b.WriteString("## Unassigned Issues\n\n")
		writeIssueTable(&b, unassignedIssues)
		b.WriteString("\n")
	}

	if len(unassignedMRs) == 0 && len(unassignedIssues) == 0 {
		b.WriteString("All open items have assignees. Great job!\n")
	}

	b.WriteString("---\nPlease identify the most critical unassigned items and suggest who should own them based on expertise.\n")

	return promptResult(b.String()), nil
}

// registerStaleItemsReportPrompt registers the stale_items_report prompt.
func registerStaleItemsReportPrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "stale_items_report",
		Title:       toolutil.TitleFromName("stale_items_report"),
		Description: "Find MRs and issues in a project that haven't been updated for a configurable number of days. Helps identify forgotten or blocked items.",
		Icons:       toolutil.IconIssue,
		Arguments: []*mcp.PromptArgument{
			projectIDArg(),
			{Name: "stale_days", Title: toolutil.TitleFromName("stale_days"), Description: "Days without update to consider stale (default: 14)", Required: false},
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleStaleItemsReport(ctx, client, req)
	})
}

// handleStaleItemsReport performs the handle stale items report operation using the GitLab API and returns [*mcp.GetPromptResult].
func handleStaleItemsReport(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	projectID := req.Params.Arguments[argProjectID]
	if projectID == "" {
		return nil, errors.New("stale_items_report: project_id is required")
	}
	staleDays := parseDays(getArgOr(req.Params.Arguments, "stale_days", "14"), 14)
	staleDate := time.Now().UTC().AddDate(0, 0, -staleDays)

	// Stale MRs (not updated since staleDate)
	staleMRs, _, _ := client.GL().MergeRequests.ListProjectMergeRequests(projectID, &gl.ListProjectMergeRequestsOptions{
		State:         new("opened"),
		UpdatedBefore: &staleDate,
		ListOptions:   gl.ListOptions{PerPage: maxListItems},
	}, gl.WithContext(ctx))

	// Stale issues
	staleIssues, _, _ := client.GL().Issues.ListProjectIssues(projectID, &gl.ListProjectIssuesOptions{
		State:         new("opened"),
		UpdatedBefore: &staleDate,
		ListOptions:   gl.ListOptions{PerPage: maxListItems},
	}, gl.WithContext(ctx))

	var b strings.Builder
	fmt.Fprintf(&b, "# Stale Items Report — %s (no updates in %d+ days)\n\n", projectID, staleDays)

	b.WriteString(mdSummaryHeader)
	b.WriteString(mdCategoryTableHeader)
	fmt.Fprintf(&b, "| Stale MRs | %d |\n", len(staleMRs))
	fmt.Fprintf(&b, "| Stale issues | %d |\n", len(staleIssues))
	b.WriteString("\n")

	if len(staleMRs) > 0 {
		b.WriteString("## Stale Merge Requests\n\n")
		writeMRTable(&b, staleMRs)
		b.WriteString("\n")
	}

	if len(staleIssues) > 0 {
		b.WriteString("## Stale Issues\n\n")
		writeIssueTable(&b, staleIssues)
		b.WriteString("\n")
	}

	if len(staleMRs) == 0 && len(staleIssues) == 0 {
		b.WriteString("No stale items found. The project is well-maintained!\n")
	}

	b.WriteString("---\nPlease analyze stale items, identify which should be closed, reassigned, or prioritized.\n")

	return promptResult(b.String()), nil
}
