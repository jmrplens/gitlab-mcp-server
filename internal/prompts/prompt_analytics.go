package prompts

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// registerAnalyticsPrompts registers all analytics prompts.
func registerAnalyticsPrompts(server *mcp.Server, client *gitlabclient.Client) {
	registerMergeVelocityPrompt(server, client)
	registerReleaseReadinessPrompt(server, client)
	registerReleaseCadencePrompt(server, client)
	registerWeeklyTeamRecapPrompt(server, client)
}

// registerMergeVelocityPrompt registers the merge_velocity prompt.
func registerMergeVelocityPrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "merge_velocity",
		Title:       toolutil.TitleFromName("merge_velocity"),
		Description: "Analyze MR throughput metrics for a project. Shows merge rate, average time-to-merge, and daily merged count chart. Ideal for tracking team delivery pace.",
		Icons:       toolutil.IconAnalytics,
		Arguments: []*mcp.PromptArgument{
			projectIDArg(),
			daysArg(30),
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleMergeVelocity(ctx, client, req)
	})
}

// handleMergeVelocity performs the handle merge velocity operation using the GitLab API and returns [*mcp.GetPromptResult].
func handleMergeVelocity(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	projectID := req.Params.Arguments[argProjectID]
	if projectID == "" {
		return nil, errors.New("merge_velocity: project_id is required")
	}
	days := parseDays(getArgOr(req.Params.Arguments, argDays, "30"), 30)
	since := sinceDate(days)

	mrs, _, err := client.GL().MergeRequests.ListProjectMergeRequests(projectID, &gl.ListProjectMergeRequestsOptions{
		State:        new("merged"),
		CreatedAfter: &since,
		ListOptions:  gl.ListOptions{PerPage: maxListItems},
	}, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("merge_velocity: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Merge Velocity — %s (last %d days)\n\n", projectID, days)

	if len(mrs) == 0 {
		b.WriteString("No merged MRs found in the period.\n")
		return promptResult(b.String()), nil
	}

	// Calculate time-to-merge stats
	var durations []time.Duration
	for _, mr := range mrs {
		d := mergeDuration(mr)
		if d > 0 {
			durations = append(durations, d)
		}
	}

	b.WriteString(mdSummaryHeader)
	b.WriteString("| Metric | Value |\n|--------|-------|\n")
	fmt.Fprintf(&b, "| MRs merged | %d |\n", len(mrs))
	if days > 0 {
		fmt.Fprintf(&b, "| Merge rate | %.1f MRs/week |\n", float64(len(mrs))/float64(days)*7)
	}
	if len(durations) > 0 {
		fmt.Fprintf(&b, "| Average time-to-merge | %s |\n", formatDuration(avgDuration(durations)))
		fmt.Fprintf(&b, "| Median time-to-merge | %s |\n", formatDuration(medianDuration(durations)))
	}
	b.WriteString("\n")

	writeDailyMergeChart(&b, mrs)

	// Recently merged table
	b.WriteString("## Recently Merged\n\n")
	writeMRTable(&b, mrs)

	b.WriteString("\n---\nPlease analyze merge velocity trends, identify bottlenecks, and suggest improvements.\n")

	return promptResult(b.String()), nil
}

// writeDailyMergeChart writes a Mermaid bar chart of daily merged MR counts.
func writeDailyMergeChart(b *strings.Builder, mrs []*gl.BasicMergeRequest) {
	dailyCounts := make(map[string]int)
	for _, mr := range mrs {
		if mr.MergedAt != nil {
			day := mr.MergedAt.Format("2006-01-02")
			dailyCounts[day]++
		}
	}

	if len(dailyCounts) == 0 {
		return
	}

	b.WriteString("## Daily Merged MRs\n\n")
	sortedDays := sortedKeys(dailyCounts)
	b.WriteString("```mermaid\nxychart-beta\n  title \"Merged MRs per day\"\n  x-axis [")
	for i, day := range sortedDays {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(day[5:]) // MM-DD
	}
	b.WriteString("]\n  y-axis \"MRs merged\"\n  bar [")
	for i, day := range sortedDays {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(b, "%d", dailyCounts[day])
	}
	b.WriteString("]\n```\n\n")
}

// registerReleaseReadinessPrompt registers the release_readiness prompt.
func registerReleaseReadinessPrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "release_readiness",
		Title:       toolutil.TitleFromName("release_readiness"),
		Description: "Check readiness of a release branch by analyzing open MRs targeting it, draft/conflict counts, and unresolved discussion threads.",
		Icons:       toolutil.IconRelease,
		Arguments: []*mcp.PromptArgument{
			projectIDArg(),
			{Name: "branch", Title: toolutil.TitleFromName("branch"), Description: "Target release branch (default: main)", Required: false},
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleReleaseReadiness(ctx, client, req)
	})
}

// handleReleaseReadiness performs the handle release readiness operation using the GitLab API and returns [*mcp.GetPromptResult].
func handleReleaseReadiness(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	projectID := req.Params.Arguments[argProjectID]
	if projectID == "" {
		return nil, errors.New("release_readiness: project_id is required")
	}
	branch := getArgOr(req.Params.Arguments, "branch", "main")

	mrs, _, err := client.GL().MergeRequests.ListProjectMergeRequests(projectID, &gl.ListProjectMergeRequestsOptions{
		TargetBranch: new(branch),
		State:        new("opened"),
		ListOptions:  gl.ListOptions{PerPage: maxListItems},
	}, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("release_readiness: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Release Readiness — %s → %s\n\n", projectID, branch)

	if len(mrs) == 0 {
		b.WriteString("No open MRs targeting this branch. The branch appears ready for release.\n")
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

	totalUnresolved := countUnresolvedThreads(ctx, client, projectID, mrs)

	blockers := drafts + conflicts + totalUnresolved
	readiness := readinessLabel(blockers)

	b.WriteString("## Status\n\n")
	fmt.Fprintf(&b, "**Overall**: %s\n\n", readiness)
	b.WriteString(mdCategoryTableHeader)
	fmt.Fprintf(&b, "| Open MRs | %d |\n", len(mrs))
	fmt.Fprintf(&b, "| Drafts | %d |\n", drafts)
	fmt.Fprintf(&b, "| With conflicts | %d |\n", conflicts)
	fmt.Fprintf(&b, "| Unresolved threads | %d |\n", totalUnresolved)
	b.WriteString("\n")

	b.WriteString("## Open MRs\n\n")
	writeMRTable(&b, mrs)

	b.WriteString("\n---\nPlease assess release readiness, flag critical blockers, and recommend a go/no-go decision.\n")

	return promptResult(b.String()), nil
}

// countUnresolvedThreads counts unresolved discussion threads across all MRs.
func countUnresolvedThreads(ctx context.Context, client *gitlabclient.Client, projectID string, mrs []*gl.BasicMergeRequest) int {
	total := 0
	for _, mr := range mrs {
		discussions, _, err := client.GL().Discussions.ListMergeRequestDiscussions(projectID, mr.IID, &gl.ListMergeRequestDiscussionsOptions{
			ListOptions: gl.ListOptions{PerPage: maxListItems},
		}, gl.WithContext(ctx))
		if err != nil {
			continue
		}
		for _, d := range discussions {
			for _, n := range d.Notes {
				if n.Resolvable && !n.Resolved {
					total++
				}
			}
		}
	}
	return total
}

// readinessLabel returns a readiness emoji+label based on blocker count.
func readinessLabel(blockers int) string {
	if blockers > 5 {
		return toolutil.EmojiRed + " Not Ready"
	}
	if blockers > 0 {
		return toolutil.EmojiYellow + " Needs Attention"
	}
	return toolutil.EmojiGreen + " Ready"
}

// registerReleaseCadencePrompt registers the release_cadence prompt.
func registerReleaseCadencePrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "release_cadence",
		Title:       toolutil.TitleFromName("release_cadence"),
		Description: "Analyze release frequency for a project. Shows time between releases, average cadence, and release history chart.",
		Icons:       toolutil.IconRelease,
		Arguments: []*mcp.PromptArgument{
			projectIDArg(),
			daysArg(90),
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleReleaseCadence(ctx, client, req)
	})
}

// handleReleaseCadence performs the handle release cadence operation using the GitLab API and returns [*mcp.GetPromptResult].
func handleReleaseCadence(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	projectID := req.Params.Arguments[argProjectID]
	if projectID == "" {
		return nil, errors.New("release_cadence: project_id is required")
	}
	days := parseDays(getArgOr(req.Params.Arguments, argDays, "90"), 90)
	since := sinceDate(days)

	releases, _, err := client.GL().Releases.ListReleases(projectID, &gl.ListReleasesOptions{
		ListOptions: gl.ListOptions{PerPage: maxListItems},
	}, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("release_cadence: %w", err)
	}

	filtered := filterRecentReleases(releases, since)

	var b strings.Builder
	fmt.Fprintf(&b, "# Release Cadence — %s (last %d days)\n\n", projectID, days)

	if len(filtered) == 0 {
		b.WriteString("No releases found in the analysis period.\n")
		return promptResult(b.String()), nil
	}

	// Calculate intervals
	var intervals []time.Duration
	for i := 1; i < len(filtered); i++ {
		prev := releaseDate(filtered[i-1])
		curr := releaseDate(filtered[i])
		intervals = append(intervals, curr.Sub(prev))
	}

	b.WriteString(mdSummaryHeader)
	b.WriteString("| Metric | Value |\n|--------|-------|\n")
	fmt.Fprintf(&b, "| Total releases | %d |\n", len(filtered))
	if len(intervals) > 0 {
		fmt.Fprintf(&b, "| Average interval | %s |\n", formatDuration(avgDuration(intervals)))
		fmt.Fprintf(&b, "| Median interval | %s |\n", formatDuration(medianDuration(intervals)))
	}
	b.WriteString("\n")

	writeReleaseHistoryTable(&b, filtered)

	b.WriteString("---\nPlease analyze release cadence, compare to team goals, and suggest if frequency should change.\n")

	return promptResult(b.String()), nil
}

// filterRecentReleases returns releases after since, sorted ascending by date.
func filterRecentReleases(releases []*gl.Release, since time.Time) []*gl.Release {
	var filtered []*gl.Release
	for _, r := range releases {
		releaseTime := r.ReleasedAt
		if releaseTime == nil {
			releaseTime = r.CreatedAt
		}
		if releaseTime != nil && releaseTime.After(since) {
			filtered = append(filtered, r)
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		return releaseDate(filtered[i]).Before(releaseDate(filtered[j]))
	})
	return filtered
}

// writeReleaseHistoryTable writes a Markdown table of release history with intervals.
func writeReleaseHistoryTable(b *strings.Builder, releases []*gl.Release) {
	b.WriteString("## Release History\n\n")
	b.WriteString("| Release | Tag | Date | Days Since Previous |\n")
	b.WriteString("|---------|-----|------|---------------------|\n")
	for i, r := range releases {
		dateFmt := releaseDate(r).Format("2006-01-02")
		daysSince := "—"
		if i > 0 {
			d := releaseDate(r).Sub(releaseDate(releases[i-1]))
			daysSince = strconv.Itoa(int(d.Hours() / 24))
		}
		name := r.Name
		if name == "" {
			name = r.TagName
		}
		fmt.Fprintf(b, "| %s | %s | %s | %s |\n", name, r.TagName, dateFmt, daysSince)
	}
	b.WriteString("\n")
}

// releaseDate returns the best-available date for a release.
func releaseDate(r *gl.Release) time.Time {
	if r.ReleasedAt != nil {
		return *r.ReleasedAt
	}
	if r.CreatedAt != nil {
		return *r.CreatedAt
	}
	return time.Time{}
}

// registerWeeklyTeamRecapPrompt registers the weekly_team_recap prompt.
func registerWeeklyTeamRecapPrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "weekly_team_recap",
		Title:       toolutil.TitleFromName("weekly_team_recap"),
		Description: "Generate a comprehensive weekly recap for a team. Combines merged MRs, open MRs, issues activity, and events into a single summary with Mermaid charts.",
		Icons:       toolutil.IconAnalytics,
		Arguments: []*mcp.PromptArgument{
			groupIDArg(),
			daysArg(7),
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleWeeklyTeamRecap(ctx, client, req)
	})
}

// handleWeeklyTeamRecap performs the handle weekly team recap operation using the GitLab API and returns [*mcp.GetPromptResult].
func handleWeeklyTeamRecap(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	groupID := req.Params.Arguments[argGroupID]
	if groupID == "" {
		return nil, errors.New("weekly_team_recap: group_id is required")
	}
	days := parseDays(getArgOr(req.Params.Arguments, argDays, "7"), 7)
	since := sinceDate(days)

	// Merged MRs in the recap period
	mergedMRs, _, err := client.GL().MergeRequests.ListGroupMergeRequests(groupID, &gl.ListGroupMergeRequestsOptions{
		State:        new("merged"),
		CreatedAfter: &since,
		ListOptions:  gl.ListOptions{PerPage: maxListItems},
	}, gl.WithContext(ctx))
	if err != nil {
		slog.Warn("failed to fetch merged MRs", "error", err)
	}

	// Open MRs
	openMRs, _, _ := client.GL().MergeRequests.ListGroupMergeRequests(groupID, &gl.ListGroupMergeRequestsOptions{
		State:       new("opened"),
		ListOptions: gl.ListOptions{PerPage: maxListItems},
	}, gl.WithContext(ctx))

	// Open issues
	openIssues, _, _ := client.GL().Issues.ListGroupIssues(groupID, &gl.ListGroupIssuesOptions{
		State:       new("opened"),
		ListOptions: gl.ListOptions{PerPage: maxListItems},
	}, gl.WithContext(ctx))

	var b strings.Builder
	fmt.Fprintf(&b, "# Weekly Team Recap — %s (last %d days)\n\n", groupID, days)

	// Summary
	b.WriteString(mdSummaryHeader)
	b.WriteString(mdCategoryTableHeader)
	fmt.Fprintf(&b, "| MRs merged | %d |\n", len(mergedMRs))
	fmt.Fprintf(&b, "| MRs open | %d |\n", len(openMRs))
	fmt.Fprintf(&b, "| Issues open | %d |\n", len(openIssues))
	b.WriteString("\n")

	// Merged MRs by project
	if len(mergedMRs) > 0 {
		b.WriteString("## Merged MRs\n\n")
		byProject := groupMRsByProject(mergedMRs)
		for _, k := range sortedKeys(byProject) {
			fmt.Fprintf(&b, "### %s\n\n", k)
			writeMRTable(&b, byProject[k])
			b.WriteString("\n")
		}
	}

	// Open MR stats
	if len(openMRs) > 0 {
		var drafts, conflicts int
		for _, mr := range openMRs {
			if mr.Draft {
				drafts++
			}
			if mr.HasConflicts {
				conflicts++
			}
		}
		b.WriteString("## Open MR Health\n\n")
		b.WriteString(mdCategoryTableHeader)
		fmt.Fprintf(&b, "| Total open | %d |\n", len(openMRs))
		fmt.Fprintf(&b, "| Drafts | %d |\n", drafts)
		fmt.Fprintf(&b, "| With conflicts | %d |\n", conflicts)
		b.WriteString("\n")
	}

	b.WriteString("---\nPlease write a concise weekly recap email highlighting achievements, blockers, and next-week priorities.\n")

	return promptResult(b.String()), nil
}
