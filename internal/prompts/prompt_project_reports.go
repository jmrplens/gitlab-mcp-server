package prompts

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const (
	// mdSummaryHeader identifies the md summary header constant used by this package.
	mdSummaryHeader = "## Summary\n\n"
	// mdCategoryTableHeader identifies the md category table header constant used by this package.
	mdCategoryTableHeader = "| Category | Count |\n|----------|-------|\n"
)

// registerProjectReportPrompts registers all project-level report prompts.
func registerProjectReportPrompts(server promptAdder, client *gitlabclient.Client) {
	registerBranchMRSummaryPrompt(server, client)
	registerProjectActivityReportPrompt(server, client)
	registerMRDiscussionHealthPrompt(server, client)
	registerUnassignedItemsPrompt(server, client)
	registerStaleItemsReportPrompt(server, client)
}

// registerBranchMRSummaryPrompt registers the branch_mr_summary prompt.
func registerBranchMRSummaryPrompt(server promptAdder, client *gitlabclient.Client) {
	addPrompt(server, &mcp.Prompt{
		Name:        "branch_mr_summary",
		Title:       toolutil.TitleFromName("branch_mr_summary"),
		Description: "List all MRs targeting a specific branch in a project. Shows readiness summary with conflict/draft/approval counts. Ideal for release branch reviews.",
		Icons:       toolutil.IconBranch,
		Arguments: []*mcp.PromptArgument{
			projectIDArg(),
			targetBranchArg(true),
			mrStateArg("opened"),
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleBranchMRSummary(ctx, client.For(ctx), req)
	})
}

// handleBranchMRSummary handles handle branch MR summary and returns [*mcp.GetPromptResult].
func handleBranchMRSummary(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	projectID := req.Params.Arguments[argProjectID]
	if projectID == "" {
		return nil, toolutil.InvalidParams(errors.New("branch_mr_summary: project_id is required"))
	}
	targetBranch := req.Params.Arguments[argTargetBranch]
	if targetBranch == "" {
		return nil, toolutil.InvalidParams(errors.New("branch_mr_summary: target_branch is required"))
	}
	state := getArgOr(req.Params.Arguments, argState, "opened")

	mrs, _, err := client.GL().MergeRequests.ListProjectMergeRequests(projectID, &gl.ListProjectMergeRequestsOptions{
		TargetBranch: new(targetBranch),
		State:        new(state),
		PerPage:      maxListItems,
	}, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("branch_mr_summary: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# MRs targeting %s in %s (%d %s)\n\n", mdHeading(targetBranch), mdHeading(projectID), len(mrs), state)

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

	writeClosingRule(&b, "Please summarize the readiness of these MRs for merging, highlight blockers (conflicts, drafts), and suggest priorities.")

	return promptResult(b.String()), nil
}

// registerProjectActivityReportPrompt registers the project_activity_report prompt.
func registerProjectActivityReportPrompt(server promptAdder, client *gitlabclient.Client) {
	addPrompt(server, &mcp.Prompt{
		Name:        "project_activity_report",
		Title:       toolutil.TitleFromName("project_activity_report"),
		Description: "Generate a project activity report including recent events, merged MRs, and open issues. Shows daily activity chart and contributor breakdown.",
		Icons:       toolutil.IconAnalytics,
		Arguments: []*mcp.PromptArgument{
			projectIDArg(),
			daysArg(7),
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleProjectActivityReport(ctx, client.For(ctx), req)
	})
}

// handleProjectActivityReport handles handle project activity report and returns [*mcp.GetPromptResult].
func handleProjectActivityReport(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	projectID := req.Params.Arguments[argProjectID]
	if projectID == "" {
		return nil, toolutil.InvalidParams(errors.New("project_activity_report: project_id is required"))
	}
	days := parseDays(getArgOr(req.Params.Arguments, argDays, "7"), 7)
	since := sinceDate(days)
	sinceISO := gl.ISOTime(since)

	// Project events
	events, _, err := client.GL().Events.ListProjectVisibleEvents(projectID, &gl.ListProjectVisibleEventsOptions{
		After: &sinceISO,
	}, gl.WithContext(ctx))
	if err != nil {
		slog.WarnContext(ctx, "failed to fetch project events", "error", err)
	}

	// Merged MRs in the period
	mergedMRs, _, _ := client.GL().MergeRequests.ListProjectMergeRequests(projectID, &gl.ListProjectMergeRequestsOptions{
		State:        new("merged"),
		CreatedAfter: &since,
		PerPage:      maxListItems,
	}, gl.WithContext(ctx))

	// Open MRs
	openMRs, _, _ := client.GL().MergeRequests.ListProjectMergeRequests(projectID, &gl.ListProjectMergeRequestsOptions{
		State:   new("opened"),
		PerPage: maxListItems,
	}, gl.WithContext(ctx))

	// Open issues
	openIssues, _, _ := client.GL().Issues.ListProjectIssues(projectID, &gl.ListProjectIssuesOptions{
		State:   new("opened"),
		PerPage: maxListItems,
	}, gl.WithContext(ctx))

	var b strings.Builder
	fmt.Fprintf(&b, "# Project Activity Report: %s (last %d days)\n\n", mdHeading(projectID), days)

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

	writeActivityContributors(&b, events)

	// Recently merged MRs
	if len(mergedMRs) > 0 {
		b.WriteString("## Recently Merged MRs\n\n")
		writeMRTable(&b, mergedMRs)
		b.WriteString("\n")
	}

	if days := projectEventDays(events); len(days) > 0 {
		writeDailyActivityChart(&b, days)
	}

	writeClosingRule(&b, "Please analyze the project activity, highlight trends, and identify areas needing attention.")

	return promptResult(b.String()), nil
}

// writeActivityContributors writes who produced the events in the period.
//
// This section and the daily activity chart beside it are the two the prompt's
// own description has always promised ("Shows daily activity chart and
// contributor breakdown") and the handler never wrote. Neither costs a request:
// both are read from the project events already fetched for the breakdown
// above. A description is what a model reads to choose a prompt, so the choice
// was between writing the sections and withdrawing the promise, and the data
// was already in hand.
func writeActivityContributors(b *strings.Builder, events []*gl.ProjectEvent) {
	if len(events) == 0 {
		return
	}
	byAuthor := make(map[string]int, len(events))
	for _, e := range events {
		byAuthor[eventAuthor(e)]++
	}
	b.WriteString("## Contributors\n\n")
	b.WriteString("| Contributor | Events |\n|-------------|--------|\n")
	for _, name := range sortedKeys(byAuthor) {
		fmt.Fprintf(b, "| %s | %d |\n", mdInline(name), byAuthor[name])
	}
	b.WriteString("\n")
}

// eventAuthor names whoever produced a project event. GitLab sends the username
// twice, once flat and once inside the author object, and a system event can
// carry neither.
func eventAuthor(e *gl.ProjectEvent) string {
	if e.AuthorUsername != "" {
		return e.AuthorUsername
	}
	if e.Author.Username != "" {
		return e.Author.Username
	}
	return "unknown"
}

// projectEventDays groups project events by the calendar day GitLab stamped
// them with, sorted chronologically.
//
// A project event carries its timestamp as a string where a contribution event
// carries a *time.Time, so this cannot reuse [groupEventsByDay]; an event whose
// timestamp parses as neither a full RFC 3339 instant nor a bare date is left
// out rather than counted under a day with no name.
func projectEventDays(events []*gl.ProjectEvent) []dayActivity {
	counts := make(map[string]int, len(events))
	for _, e := range events {
		if day, ok := eventDay(e.CreatedAt); ok {
			counts[day]++
		}
	}
	days := sortedKeys(counts)
	result := make([]dayActivity, len(days))
	for i, d := range days {
		result[i] = dayActivity{date: d, count: counts[d]}
	}
	return result
}

// eventDay reads the calendar day out of an event timestamp, and reports
// whether it could.
func eventDay(stamp string) (string, bool) {
	for _, layout := range []string{time.RFC3339, toolutil.DateFormatISO} {
		if t, err := time.Parse(layout, stamp); err == nil {
			return t.Format(toolutil.DateFormatISO), true
		}
	}
	return "", false
}

// registerMRDiscussionHealthPrompt registers the mr_discussion_health prompt.
func registerMRDiscussionHealthPrompt(server promptAdder, client *gitlabclient.Client) {
	addPrompt(server, &mcp.Prompt{
		Name:        "mr_discussion_health",
		Title:       toolutil.TitleFromName("mr_discussion_health"),
		Description: "Analyze unresolved discussion threads across open MRs in a project. Use this for review follow-up and merge-readiness cleanup, not approval-rule status.",
		Icons:       toolutil.IconMR,
		Arguments: []*mcp.PromptArgument{
			projectIDArg(),
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleMRDiscussionHealth(ctx, client.For(ctx), req)
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

// handleMRDiscussionHealth handles handle MR discussion health and returns [*mcp.GetPromptResult].
func handleMRDiscussionHealth(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	projectID := req.Params.Arguments[argProjectID]
	if projectID == "" {
		return nil, toolutil.InvalidParams(errors.New("mr_discussion_health: project_id is required"))
	}

	mrs, _, err := client.GL().MergeRequests.ListProjectMergeRequests(projectID, &gl.ListProjectMergeRequestsOptions{
		State:   new("opened"),
		PerPage: 20,
	}, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("mr_discussion_health: %w", err)
	}

	infos := collectMRDiscussionInfos(ctx, client, projectID, mrs)

	var b strings.Builder
	fmt.Fprintf(&b, "# MR Discussion Health: %s (%d open MRs)\n\n", mdHeading(projectID), len(mrs))

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
		fmt.Fprintf(&b, "| !%d | %s | @%s | %d | %d |\n", info.iid, mdInline(info.title), mdInline(info.author), info.threads, info.unresolved)
	}
	b.WriteString("\n")

	writeClosingRule(&b, "Please identify MRs with the most unresolved threads, assess review health, and suggest actions.")

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
			PerPage: maxListItems,
		}, gl.WithContext(ctx))
		if dErr != nil {
			slog.WarnContext(ctx, "failed to fetch discussions", "merge_request_iid", mr.IID, "error", dErr)
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
func registerUnassignedItemsPrompt(server promptAdder, client *gitlabclient.Client) {
	addPrompt(server, &mcp.Prompt{
		Name:        "unassigned_items",
		Title:       toolutil.TitleFromName("unassigned_items"),
		Description: "Find open MRs and issues in a project that have no assignee. Helps identify ownership gaps and items needing attention.",
		Icons:       toolutil.IconIssue,
		Arguments: []*mcp.PromptArgument{
			projectIDArg(),
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleUnassignedItems(ctx, client.For(ctx), req)
	})
}

// handleUnassignedItems handles handle unassigned items and returns [*mcp.GetPromptResult].
func handleUnassignedItems(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	projectID := req.Params.Arguments[argProjectID]
	if projectID == "" {
		return nil, toolutil.InvalidParams(errors.New("unassigned_items: project_id is required"))
	}

	// Unassigned MRs
	unassignedMRs, _, _ := client.GL().MergeRequests.ListProjectMergeRequests(projectID, &gl.ListProjectMergeRequestsOptions{
		State:      new("opened"),
		AssigneeID: gl.AssigneeID(0),
		PerPage:    maxListItems,
	}, gl.WithContext(ctx))

	// Unassigned issues
	unassignedIssues, _, _ := client.GL().Issues.ListProjectIssues(projectID, &gl.ListProjectIssuesOptions{
		State:      new("opened"),
		AssigneeID: gl.AssigneeID(0),
		PerPage:    maxListItems,
	}, gl.WithContext(ctx))

	var b strings.Builder
	fmt.Fprintf(&b, "# Unassigned Items: %s\n\n", mdHeading(projectID))

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

	writeClosingRule(&b, "Please identify the most critical unassigned items and suggest who should own them based on expertise.")

	return promptResult(b.String()), nil
}

// registerStaleItemsReportPrompt registers the stale_items_report prompt.
func registerStaleItemsReportPrompt(server promptAdder, client *gitlabclient.Client) {
	addPrompt(server, &mcp.Prompt{
		Name:        "stale_items_report",
		Title:       toolutil.TitleFromName("stale_items_report"),
		Description: "Find MRs and issues in a project that haven't been updated for a configurable number of days. Helps identify forgotten or blocked items.",
		Icons:       toolutil.IconIssue,
		Arguments: []*mcp.PromptArgument{
			projectIDArg(),
			{Name: "stale_days", Title: toolutil.TitleFromName("stale_days"), Description: "Days without update to consider stale (default: 14)", Required: false},
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleStaleItemsReport(ctx, client.For(ctx), req)
	})
}

// handleStaleItemsReport handles handle stale items report and returns [*mcp.GetPromptResult].
func handleStaleItemsReport(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	projectID := req.Params.Arguments[argProjectID]
	if projectID == "" {
		return nil, toolutil.InvalidParams(errors.New("stale_items_report: project_id is required"))
	}
	staleDays := parseDays(getArgOr(req.Params.Arguments, "stale_days", "14"), 14)
	staleDate := time.Now().UTC().AddDate(0, 0, -staleDays)

	// Stale MRs (not updated since staleDate)
	staleMRs, _, _ := client.GL().MergeRequests.ListProjectMergeRequests(projectID, &gl.ListProjectMergeRequestsOptions{
		State:         new("opened"),
		UpdatedBefore: &staleDate,
		PerPage:       maxListItems,
	}, gl.WithContext(ctx))

	// Stale issues
	staleIssues, _, _ := client.GL().Issues.ListProjectIssues(projectID, &gl.ListProjectIssuesOptions{
		State:         new("opened"),
		UpdatedBefore: &staleDate,
		PerPage:       maxListItems,
	}, gl.WithContext(ctx))

	var b strings.Builder
	fmt.Fprintf(&b, "# Stale Items Report: %s (no updates in %d+ days)\n\n", mdHeading(projectID), staleDays)

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

	writeClosingRule(&b, "Please analyze stale items, identify which should be closed, reassigned, or prioritized.")

	return promptResult(b.String()), nil
}
