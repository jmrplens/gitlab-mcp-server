package prompts

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// registerMyOpenMRsPrompt registers the my_open_mrs prompt.
func registerMyOpenMRsPrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "my_open_mrs",
		Title:       toolutil.TitleFromName("my_open_mrs"),
		Description: "Show all open merge requests across all projects where you are author or assignee. Results are grouped by project for easy scanning. Use this to get a personal MR dashboard without specifying a project.",
		Icons:       toolutil.IconMR,
		Arguments: []*mcp.PromptArgument{
			usernameArg(),
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleMyOpenMRs(ctx, client, req)
	})
}

// handleMyOpenMRs aggregates open MRs authored by and assigned to the user
// across all accessible projects.
func handleMyOpenMRs(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	args := req.Params.Arguments
	username, userID, _, err := resolveUser(ctx, client, args[argUsername])
	if err != nil {
		return nil, err
	}

	// MRs authored by the user
	authoredMRs, _, errAuthored := client.GL().MergeRequests.ListMergeRequests(
		&gl.ListMergeRequestsOptions{
			AuthorID:    new(userID),
			State:       new("opened"),
			ListOptions: gl.ListOptions{PerPage: maxListItems},
		}, gl.WithContext(ctx))
	if errAuthored != nil {
		slog.Warn("failed to list authored MRs", "error", errAuthored)
	}

	// MRs assigned to the user
	assignedMRs, _, errAssigned := client.GL().MergeRequests.ListMergeRequests(
		&gl.ListMergeRequestsOptions{
			AssigneeID:  gl.AssigneeID(userID),
			State:       new("opened"),
			ListOptions: gl.ListOptions{PerPage: maxListItems},
		}, gl.WithContext(ctx))
	if errAssigned != nil {
		slog.Warn("failed to list assigned MRs", "error", errAssigned)
	}

	allMRs := deduplicateMRs(authoredMRs, assignedMRs)
	grouped := groupMRsByProject(allMRs)

	// Count categories
	var draftCount, conflictCount, authoredCount, assignedOnlyCount int
	authoredSet := make(map[int64]bool)
	for _, mr := range authoredMRs {
		authoredSet[mr.IID] = true
	}
	for _, mr := range allMRs {
		if mr.Draft {
			draftCount++
		}
		if mr.HasConflicts {
			conflictCount++
		}
		if authoredSet[mr.IID] {
			authoredCount++
		}
	}
	assignedOnlyCount = len(allMRs) - authoredCount

	var b strings.Builder
	fmt.Fprintf(&b, "# Open Merge Requests for @%s\n\n", username)

	b.WriteString("## Summary\n")
	b.WriteString(tableCategoryHeader)
	b.WriteString(tableCategorySeparator)
	fmt.Fprintf(&b, fmtTableCountRow, "Total open MRs", len(allMRs))
	fmt.Fprintf(&b, fmtTableCountRow, "As author", authoredCount)
	fmt.Fprintf(&b, fmtTableCountRow, "As assignee (not author)", assignedOnlyCount)
	fmt.Fprintf(&b, fmtTableCountRow, "With conflicts", conflictCount)
	fmt.Fprintf(&b, fmtTableCountRow, "Draft", draftCount)

	for _, project := range sortedKeys(grouped) {
		mrs := grouped[project]
		fmt.Fprintf(&b, "\n## %s (%d MRs)\n\n", project, len(mrs))
		writeMRTable(&b, mrs)
	}

	b.WriteString("\n---\nPlease summarize the status of these MRs, highlight any that need attention (conflicts, stale >7d, failing pipeline), and suggest priorities.\n")

	return promptResult(b.String()), nil
}

// registerMyPendingReviewsPrompt registers the my_pending_reviews prompt.
func registerMyPendingReviewsPrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "my_pending_reviews",
		Title:       toolutil.TitleFromName("my_pending_reviews"),
		Description: "Show all open merge requests where you are assigned as reviewer across all projects. Helps track which MRs are waiting for your review. Results grouped by project.",
		Icons:       toolutil.IconMR,
		Arguments: []*mcp.PromptArgument{
			usernameArg(),
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleMyPendingReviews(ctx, client, req)
	})
}

// handleMyPendingReviews lists all open MRs where the user is a reviewer.
func handleMyPendingReviews(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	args := req.Params.Arguments
	username, userID, _, err := resolveUser(ctx, client, args[argUsername])
	if err != nil {
		return nil, err
	}

	mrs, _, err := client.GL().MergeRequests.ListMergeRequests(
		&gl.ListMergeRequestsOptions{
			ReviewerID:  gl.ReviewerID(userID),
			State:       new("opened"),
			ListOptions: gl.ListOptions{PerPage: maxListItems},
		}, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("failed to list pending reviews: %w", err)
	}

	grouped := groupMRsByProject(mrs)

	var b strings.Builder
	fmt.Fprintf(&b, "# Pending Reviews for @%s (%d MRs)\n\n", username, len(mrs))

	if len(mrs) == 0 {
		b.WriteString("No pending reviews found. You're all caught up! " + toolutil.EmojiParty + "\n")
	} else {
		for _, project := range sortedKeys(grouped) {
			projectMRs := grouped[project]
			fmt.Fprintf(&b, "## %s (%d MRs)\n\n", project, len(projectMRs))
			writeMRTable(&b, projectMRs)
			b.WriteString("\n")
		}
	}

	b.WriteString("\n---\nPlease prioritize these reviews, highlighting urgent ones (old >5d, large changes, or blocking release).\n")

	return promptResult(b.String()), nil
}

// registerMyIssuesPrompt registers the my_issues prompt.
func registerMyIssuesPrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "my_issues",
		Title:       toolutil.TitleFromName("my_issues"),
		Description: "Show all issues assigned to you across all projects. Includes overdue detection and project grouping. Use this to see your full issue backlog without specifying a project.",
		Icons:       toolutil.IconIssue,
		Arguments: []*mcp.PromptArgument{
			usernameArg(),
			stateArg("opened"),
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleMyIssues(ctx, client, req)
	})
}

// handleMyIssues lists all issues assigned to the user across all projects.
func handleMyIssues(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	args := req.Params.Arguments
	username, userID, _, err := resolveUser(ctx, client, args[argUsername])
	if err != nil {
		return nil, err
	}

	state := getArgOr(args, argState, "opened")

	issues, _, err := client.GL().Issues.ListIssues(
		&gl.ListIssuesOptions{
			AssigneeID:  gl.AssigneeID(userID),
			State:       new(state),
			ListOptions: gl.ListOptions{PerPage: maxListItems},
		}, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("failed to list issues: %w", err)
	}

	grouped := groupIssuesByProject(issues)

	// Count overdue and no-milestone
	now := time.Now()
	var overdueCount, noMilestoneCount int
	for _, issue := range issues {
		if issue.DueDate != nil && time.Time(*issue.DueDate).Before(now) {
			overdueCount++
		}
		if issue.Milestone == nil {
			noMilestoneCount++
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Issues Assigned to @%s (%d issues, state: %s)\n\n", username, len(issues), state)

	b.WriteString("## Summary\n")
	b.WriteString(tableCategoryHeader)
	b.WriteString(tableCategorySeparator)
	fmt.Fprintf(&b, fmtTableCountRow, "Total", len(issues))
	fmt.Fprintf(&b, fmtTableCountRow, "Overdue", overdueCount)
	fmt.Fprintf(&b, fmtTableCountRow, "No milestone", noMilestoneCount)

	for _, project := range sortedKeys(grouped) {
		projectIssues := grouped[project]
		fmt.Fprintf(&b, "\n## %s (%d issues)\n\n", project, len(projectIssues))
		writeIssueTable(&b, projectIssues)
	}

	b.WriteString("\n---\nPlease summarize the issue backlog, highlight overdue items, and suggest priorities based on due dates and labels.\n")

	return promptResult(b.String()), nil
}

// registerMyActivitySummaryPrompt registers the my_activity_summary prompt.
func registerMyActivitySummaryPrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "my_activity_summary",
		Title:       toolutil.TitleFromName("my_activity_summary"),
		Description: "Generate a personal activity summary for a configurable time period. Includes contribution events breakdown, MRs created/merged/reviewed, issues created/closed, and a daily activity chart. Aggregates across all projects.",
		Icons:       toolutil.IconUser,
		Arguments: []*mcp.PromptArgument{
			usernameArg(),
			daysArg(7),
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleMyActivitySummary(ctx, client, req)
	})
}

// handleMyActivitySummary aggregates user activity across all projects.
func handleMyActivitySummary(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	args := req.Params.Arguments
	username, userID, isCurrentUser, err := resolveUser(ctx, client, args[argUsername])
	if err != nil {
		return nil, err
	}

	days := parseDays(args[argDays], 7)
	since := sinceDate(days)
	sinceISO := gl.ISOTime(since)

	// Fetch contribution events
	eventOpts := &gl.ListContributionEventsOptions{
		After: &sinceISO,
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

	// Merged MRs in period
	mergedMRs, _, errMerged := client.GL().MergeRequests.ListMergeRequests(
		&gl.ListMergeRequestsOptions{
			AuthorID:     new(userID),
			State:        new("merged"),
			CreatedAfter: &since,
			ListOptions:  gl.ListOptions{PerPage: maxListItems},
		}, gl.WithContext(ctx))
	if errMerged != nil {
		slog.Warn("failed to list merged MRs", "error", errMerged)
	}

	// MRs reviewed in period
	reviewedMRs, _, errReviewed := client.GL().MergeRequests.ListMergeRequests(
		&gl.ListMergeRequestsOptions{
			ReviewerID:   gl.ReviewerID(userID),
			UpdatedAfter: new(since),
			ListOptions:  gl.ListOptions{PerPage: maxListItems},
		}, gl.WithContext(ctx))
	if errReviewed != nil {
		slog.Warn("failed to list reviewed MRs", "error", errReviewed)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Activity Summary for @%s (last %d days)\n\n", username, days)

	// Event breakdown
	b.WriteString("## Contribution Events\n")
	if len(events) == 0 {
		b.WriteString("No contribution events found in this period.\n")
	} else {
		eventTypes := countEventTypes(events)
		fmt.Fprintf(&b, "Total events: %d\n\n", len(events))
		b.WriteString(tableCategoryHeader)
		b.WriteString(tableCategorySeparator)
		for evType, count := range eventTypes {
			fmt.Fprintf(&b, fmtTableCountRow, evType, count)
		}
	}

	// MR summary
	b.WriteString("\n## Merge Request Activity\n")
	b.WriteString(tableCategoryHeader)
	b.WriteString(tableCategorySeparator)
	writeCountRow(&b, "MRs merged", len(mergedMRs), errMerged)
	writeCountRow(&b, "MRs reviewed", len(reviewedMRs), errReviewed)

	// Daily activity chart
	if len(events) > 0 {
		dailyData := groupEventsByDay(events)
		writeDailyActivityChart(&b, dailyData)
	}

	b.WriteString("\n---\nPlease provide a comprehensive activity summary with productivity insights and trends.\n")

	return promptResult(b.String()), nil
}

// writeDailyActivityChart renders a Mermaid bar chart of daily event counts.
func writeDailyActivityChart(b *strings.Builder, dailyData []dayActivity) {
	b.WriteString("\n## Daily Activity\n\n")
	b.WriteString("```mermaid\nxychart-beta\n  title \"Daily Activity\"\n  x-axis [")
	for i, d := range dailyData {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString("\"" + d.date[5:] + "\"")
	}
	b.WriteString("]\n  y-axis \"Events\"\n  bar [")
	for i, d := range dailyData {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(b, "%d", d.count)
	}
	b.WriteString("]\n```\n")
}

// registerCrossProjectPrompts registers all cross-project prompts.
func registerCrossProjectPrompts(server *mcp.Server, client *gitlabclient.Client) {
	registerMyOpenMRsPrompt(server, client)
	registerMyPendingReviewsPrompt(server, client)
	registerMyIssuesPrompt(server, client)
	registerMyActivitySummaryPrompt(server, client)
}
