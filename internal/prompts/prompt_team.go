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
	mdSummaryHeading = "## Summary\n\n"
	tblCategoryCount = "| Category | Count |\n|----------|-------|\n"
)

// registerTeamPrompts registers all team management prompts.
func registerTeamPrompts(server *mcp.Server, client *gitlabclient.Client) {
	registerUserActivityReportPrompt(server, client)
	registerTeamOverviewPrompt(server, client)
	registerTeamMRDashboardPrompt(server, client)
	registerReviewerWorkloadPrompt(server, client)
}

// registerUserActivityReportPrompt registers the user_activity_report prompt.
func registerUserActivityReportPrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "user_activity_report",
		Title:       toolutil.TitleFromName("user_activity_report"),
		Description: "Generate a detailed activity report for a specific user: contribution events, merged MRs, reviewed MRs, daily activity chart. Designed for managers to review team member productivity.",
		Icons:       toolutil.IconUser,
		Arguments: []*mcp.PromptArgument{
			{Name: argUsername, Title: toolutil.TitleFromName(argUsername), Description: "GitLab username to report on", Required: true},
			daysArg(7),
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleUserActivityReport(ctx, client, req)
	})
}

// handleUserActivityReport performs the handle user activity report operation using the GitLab API and returns [*mcp.GetPromptResult].
func handleUserActivityReport(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	username := req.Params.Arguments[argUsername]
	if username == "" {
		return nil, errors.New("user_activity_report: username is required")
	}

	resolvedUser, userID, isSelf, err := resolveUser(ctx, client, username)
	if err != nil {
		return nil, fmt.Errorf("user_activity_report: %w", err)
	}

	days := parseDays(getArgOr(req.Params.Arguments, argDays, "7"), 7)
	since := sinceDate(days)

	// Contribution events
	eventOpts := &gl.ListContributionEventsOptions{After: new(gl.ISOTime(since))}
	var events []*gl.ContributionEvent
	if isSelf {
		events, _, err = client.GL().Events.ListCurrentUserContributionEvents(eventOpts, gl.WithContext(ctx))
	} else {
		events, _, err = client.GL().Users.ListUserContributionEvents(userID, eventOpts, gl.WithContext(ctx))
	}
	if err != nil {
		slog.Warn("failed to fetch events", "error", err)
	}

	// Merged MRs in period
	mergedMRs, _, _ := client.GL().MergeRequests.ListMergeRequests(&gl.ListMergeRequestsOptions{
		AuthorID:     new(userID),
		State:        new("merged"),
		CreatedAfter: new(since),
		ListOptions:  gl.ListOptions{PerPage: maxListItems},
	}, gl.WithContext(ctx))

	// MRs under review
	reviewMRs, _, _ := client.GL().MergeRequests.ListMergeRequests(&gl.ListMergeRequestsOptions{
		ReviewerID:   gl.ReviewerID(userID),
		State:        new("opened"),
		UpdatedAfter: new(since),
		ListOptions:  gl.ListOptions{PerPage: maxListItems},
	}, gl.WithContext(ctx))

	var b strings.Builder
	fmt.Fprintf(&b, "# Activity Report for @%s (last %d days)\n\n", resolvedUser, days)

	// Event breakdown
	b.WriteString("## Contribution Events\n\n")
	if len(events) == 0 {
		b.WriteString("No contribution events found in this period.\n\n")
	} else {
		eventTypes := countEventTypes(events)
		fmt.Fprintf(&b, "Total events: %d\n\n", len(events))
		b.WriteString("| Action | Count |\n|--------|-------|\n")
		for _, k := range sortedKeys(eventTypes) {
			fmt.Fprintf(&b, "| %s | %d |\n", k, eventTypes[k])
		}
		b.WriteString("\n")
	}

	// Merged MRs
	b.WriteString("## Merged MRs\n\n")
	if len(mergedMRs) == 0 {
		b.WriteString("No merged MRs in this period.\n\n")
	} else {
		grouped := groupMRsByProject(mergedMRs)
		for _, proj := range sortedKeys(grouped) {
			fmt.Fprintf(&b, "### %s\n\n", proj)
			writeMRTable(&b, grouped[proj])
			b.WriteString("\n")
		}
	}

	// Reviewed MRs
	b.WriteString("## MRs Under Review\n\n")
	if len(reviewMRs) == 0 {
		b.WriteString("No MRs under review.\n\n")
	} else {
		grouped := groupMRsByProject(reviewMRs)
		for _, proj := range sortedKeys(grouped) {
			fmt.Fprintf(&b, "### %s\n\n", proj)
			writeMRTable(&b, grouped[proj])
			b.WriteString("\n")
		}
	}

	// Daily activity chart
	if len(events) > 0 {
		byDay := groupEventsByDay(events)
		b.WriteString("## Daily Activity\n\n")
		b.WriteString("```mermaid\nxychart-beta\n  title \"Daily Events\"\n  x-axis [")
		for i, d := range byDay {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(d.date)
		}
		b.WriteString("]\n  y-axis \"Events\"\n  bar [")
		for i, d := range byDay {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%d", d.count)
		}
		b.WriteString("]\n```\n\n")
	}

	b.WriteString("---\nPlease analyze this team member's activity, highlight strengths and areas for improvement, and compare workload balance.\n")

	return promptResult(b.String()), nil
}

// registerTeamOverviewPrompt registers the team_overview prompt.
func registerTeamOverviewPrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "team_overview",
		Title:       toolutil.TitleFromName("team_overview"),
		Description: "Generate a team dashboard showing all group members with their open MR counts and recently merged MRs. Includes a workload distribution pie chart. Requires a GitLab group ID.",
		Icons:       toolutil.IconGroup,
		Arguments: []*mcp.PromptArgument{
			groupIDArg(),
			daysArg(7),
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleTeamOverview(ctx, client, req)
	})
}

// handleTeamOverview performs the handle team overview operation using the GitLab API and returns [*mcp.GetPromptResult].
func handleTeamOverview(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	groupID := req.Params.Arguments[argGroupID]
	if groupID == "" {
		return nil, errors.New("team_overview: group_id is required")
	}
	days := parseDays(getArgOr(req.Params.Arguments, argDays, "7"), 7)
	since := sinceDate(days)

	// Group members
	members, _, err := client.GL().Groups.ListGroupMembers(groupID, &gl.ListGroupMembersOptions{
		ListOptions: gl.ListOptions{PerPage: 50},
	}, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("team_overview: failed to list group members: %w", err)
	}

	// Open MRs for the group
	openMRs, _, _ := client.GL().MergeRequests.ListGroupMergeRequests(groupID, &gl.ListGroupMergeRequestsOptions{
		State:       new("opened"),
		ListOptions: gl.ListOptions{PerPage: maxListItems},
	}, gl.WithContext(ctx))

	// Merged MRs in the period
	mergedMRs, _, _ := client.GL().MergeRequests.ListGroupMergeRequests(groupID, &gl.ListGroupMergeRequestsOptions{
		State:        new("merged"),
		CreatedAfter: new(since),
		ListOptions:  gl.ListOptions{PerPage: maxListItems},
	}, gl.WithContext(ctx))

	// Build per-member stats
	type memberStats struct {
		name      string
		openMRs   int
		mergedMRs int
		reviewMRs int
	}
	stats := make(map[string]*memberStats)
	for _, m := range members {
		if m.State != "active" {
			continue
		}
		stats[m.Username] = &memberStats{name: m.Name}
	}

	for _, mr := range openMRs {
		if mr.Author != nil {
			if s, ok := stats[mr.Author.Username]; ok {
				s.openMRs++
			}
		}
		for _, r := range mr.Reviewers {
			if s, ok := stats[r.Username]; ok {
				s.reviewMRs++
			}
		}
	}
	for _, mr := range mergedMRs {
		if mr.Author != nil {
			if s, ok := stats[mr.Author.Username]; ok {
				s.mergedMRs++
			}
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Team Overview — Group %s (last %d days)\n\n", groupID, days)

	// Summary
	b.WriteString(mdSummaryHeading)
	b.WriteString(tblCategoryCount)
	activeCount := len(stats)
	fmt.Fprintf(&b, "| Active members | %d |\n", activeCount)
	fmt.Fprintf(&b, "| Open MRs | %d |\n", len(openMRs))
	fmt.Fprintf(&b, "| Merged MRs (period) | %d |\n", len(mergedMRs))
	b.WriteString("\n")

	// Member workload table
	b.WriteString("## Member Workload\n\n")
	b.WriteString("| Member | Name | Open MRs | Merged | Reviewing |\n")
	b.WriteString("|--------|------|----------|--------|-----------|\n")
	for _, uname := range sortedKeys(stats) {
		s := stats[uname]
		fmt.Fprintf(&b, "| @%s | %s | %d | %d | %d |\n", uname, s.name, s.openMRs, s.mergedMRs, s.reviewMRs)
	}
	b.WriteString("\n")

	// Mermaid pie chart
	if activeCount > 0 {
		b.WriteString("## Workload Distribution (Open MRs)\n\n```mermaid\npie title Open MRs by Author\n")
		for _, uname := range sortedKeys(stats) {
			s := stats[uname]
			if s.openMRs > 0 {
				fmt.Fprintf(&b, "  \"%s\" : %d\n", uname, s.openMRs)
			}
		}
		b.WriteString("```\n\n")
	}

	b.WriteString("---\nPlease analyze the team's workload distribution, identify bottlenecks, and suggest rebalancing actions.\n")

	return promptResult(b.String()), nil
}

// registerTeamMRDashboardPrompt registers the team_mr_dashboard prompt.
func registerTeamMRDashboardPrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "team_mr_dashboard",
		Title:       toolutil.TitleFromName("team_mr_dashboard"),
		Description: "List all merge requests for a GitLab group with optional state and target branch filters. Shows MRs grouped by project with summary statistics.",
		Icons:       toolutil.IconMR,
		Arguments: []*mcp.PromptArgument{
			groupIDArg(),
			stateArg("opened"),
			targetBranchArg(false),
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleTeamMRDashboard(ctx, client, req)
	})
}

// handleTeamMRDashboard performs the handle team m r dashboard operation using the GitLab API and returns [*mcp.GetPromptResult].
func handleTeamMRDashboard(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	groupID := req.Params.Arguments[argGroupID]
	if groupID == "" {
		return nil, errors.New("team_mr_dashboard: group_id is required")
	}

	state := getArgOr(req.Params.Arguments, argState, "opened")
	opts := &gl.ListGroupMergeRequestsOptions{
		State:       new(state),
		ListOptions: gl.ListOptions{PerPage: maxListItems},
	}
	if tb := req.Params.Arguments[argTargetBranch]; tb != "" {
		opts.TargetBranch = new(tb)
	}

	mrs, _, err := client.GL().MergeRequests.ListGroupMergeRequests(groupID, opts, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("team_mr_dashboard: %w", err)
	}

	var b strings.Builder
	branchInfo := ""
	if opts.TargetBranch != nil {
		branchInfo = fmt.Sprintf(" targeting %s", *opts.TargetBranch)
	}
	fmt.Fprintf(&b, "# Group MR Dashboard — %s (%d %s MRs%s)\n\n", groupID, len(mrs), state, branchInfo)

	if len(mrs) == 0 {
		b.WriteString("No merge requests found matching the criteria.\n")
		return promptResult(b.String()), nil
	}

	// Summary stats
	var drafts, conflicts int
	for _, mr := range mrs {
		if mr.Draft {
			drafts++
		}
		if mr.HasConflicts {
			conflicts++
		}
	}
	b.WriteString(mdSummaryHeading)
	b.WriteString(tblCategoryCount)
	fmt.Fprintf(&b, "| Total | %d |\n", len(mrs))
	fmt.Fprintf(&b, "| Draft | %d |\n", drafts)
	fmt.Fprintf(&b, "| With conflicts | %d |\n", conflicts)
	b.WriteString("\n")

	// Group by project
	grouped := groupMRsByProject(mrs)
	for _, proj := range sortedKeys(grouped) {
		projMRs := grouped[proj]
		fmt.Fprintf(&b, "## %s (%d MRs)\n\n", proj, len(projMRs))
		writeMRTable(&b, projMRs)
		b.WriteString("\n")
	}

	b.WriteString("---\nPlease summarize the MR status across the group, highlight blockers, and suggest priorities.\n")

	return promptResult(b.String()), nil
}

// registerReviewerWorkloadPrompt registers the reviewer_workload prompt.
func registerReviewerWorkloadPrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "reviewer_workload",
		Title:       toolutil.TitleFromName("reviewer_workload"),
		Description: "Analyze review distribution across group members. Shows how many open MRs each member is reviewing and identifies imbalances. Useful for managers to ensure fair review distribution.",
		Icons:       toolutil.IconUser,
		Arguments: []*mcp.PromptArgument{
			groupIDArg(),
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleReviewerWorkload(ctx, client, req)
	})
}

// handleReviewerWorkload performs the handle reviewer workload operation using the GitLab API and returns [*mcp.GetPromptResult].
func handleReviewerWorkload(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	groupID := req.Params.Arguments[argGroupID]
	if groupID == "" {
		return nil, errors.New("reviewer_workload: group_id is required")
	}

	// Group members
	members, _, err := client.GL().Groups.ListGroupMembers(groupID, &gl.ListGroupMembersOptions{
		ListOptions: gl.ListOptions{PerPage: 50},
	}, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("reviewer_workload: failed to list group members: %w", err)
	}

	// Open MRs
	openMRs, _, _ := client.GL().MergeRequests.ListGroupMergeRequests(groupID, &gl.ListGroupMergeRequestsOptions{
		State:       new("opened"),
		ListOptions: gl.ListOptions{PerPage: maxListItems},
	}, gl.WithContext(ctx))

	// Build reviewer stats
	type reviewerStats struct {
		name     string
		count    int
		oldestMR *time.Time
	}
	rStats := make(map[string]*reviewerStats)
	for _, m := range members {
		if m.State != "active" {
			continue
		}
		rStats[m.Username] = &reviewerStats{name: m.Name}
	}

	for _, mr := range openMRs {
		for _, rev := range mr.Reviewers {
			s, ok := rStats[rev.Username]
			if !ok {
				s = &reviewerStats{name: rev.Username}
				rStats[rev.Username] = s
			}
			s.count++
			if mr.CreatedAt != nil && (s.oldestMR == nil || mr.CreatedAt.Before(*s.oldestMR)) {
				s.oldestMR = mr.CreatedAt
			}
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Reviewer Workload — Group %s\n\n", groupID)

	// Summary
	totalReviews := 0
	for _, s := range rStats {
		totalReviews += s.count
	}
	activeReviewers := 0
	for _, s := range rStats {
		if s.count > 0 {
			activeReviewers++
		}
	}
	b.WriteString(mdSummaryHeading)
	b.WriteString(tblCategoryCount)
	fmt.Fprintf(&b, "| Total open MRs | %d |\n", len(openMRs))
	fmt.Fprintf(&b, "| Total review assignments | %d |\n", totalReviews)
	fmt.Fprintf(&b, "| Active reviewers | %d |\n", activeReviewers)
	if activeReviewers > 0 {
		fmt.Fprintf(&b, "| Avg reviews/reviewer | %.1f |\n", float64(totalReviews)/float64(activeReviewers))
	}
	b.WriteString("\n")

	// Reviewer table
	b.WriteString("## Review Distribution\n\n")
	b.WriteString("| Reviewer | Name | MRs to Review | Oldest Pending |\n")
	b.WriteString("|----------|------|---------------|----------------|\n")
	for _, uname := range sortedKeys(rStats) {
		s := rStats[uname]
		oldest := "-"
		if s.oldestMR != nil {
			oldest = formatAge(time.Since(*s.oldestMR))
		}
		fmt.Fprintf(&b, "| @%s | %s | %d | %s |\n", uname, s.name, s.count, oldest)
	}
	b.WriteString("\n")

	// Mermaid pie chart
	if activeReviewers > 0 {
		b.WriteString("## Distribution Chart\n\n```mermaid\npie title Review Distribution\n")
		for _, uname := range sortedKeys(rStats) {
			s := rStats[uname]
			if s.count > 0 {
				fmt.Fprintf(&b, "  \"%s\" : %d\n", uname, s.count)
			}
		}
		b.WriteString("```\n\n")
	}

	b.WriteString("---\nPlease analyze the review distribution, identify overloaded reviewers, and suggest rebalancing actions.\n")

	return promptResult(b.String()), nil
}
