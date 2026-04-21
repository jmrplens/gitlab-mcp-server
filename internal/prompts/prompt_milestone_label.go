// prompt_milestone_label.go registers MCP prompts for milestone and label management.

package prompts

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// registerMilestoneLabelPrompts registers all milestone and label prompts.
func registerMilestoneLabelPrompts(server *mcp.Server, client *gitlabclient.Client) {
	registerMilestoneProgressPrompt(server, client)
	registerLabelDistributionPrompt(server, client)
	registerGroupMilestoneProgressPrompt(server, client)
	registerProjectContributorsPrompt(server, client)
}

// countIssueStates counts closed and open issues from a slice.
func countIssueStates(issues []*gl.Issue) (closed, open int) {
	for _, i := range issues {
		if i.State == "closed" {
			closed++
		} else {
			open++
		}
	}
	return closed, open
}

// countMRStates counts merged and open merge requests from a slice.
func countMRStates(mrs []*gl.BasicMergeRequest) (merged, open int) {
	for _, m := range mrs {
		switch m.State {
		case "merged":
			merged++
		case "opened":
			open++
		}
	}
	return merged, open
}

// writeDueDateSection appends the due date with remaining/overdue days to the builder.
func writeDueDateSection(b *strings.Builder, dueDate *gl.ISOTime) {
	if dueDate == nil {
		return
	}
	t := time.Time(*dueDate)
	daysLeft := int(time.Until(t).Hours() / 24)
	if daysLeft >= 0 {
		fmt.Fprintf(b, "\n**Due Date**: %s (%d days remaining)\n", t.Format(toolutil.DateFormatISO), daysLeft)
	} else {
		fmt.Fprintf(b, "\n**Due Date**: %s (**%d days overdue**)\n", t.Format(toolutil.DateFormatISO), -daysLeft)
	}
}

// registerMilestoneProgressPrompt registers the milestone_progress prompt.
func registerMilestoneProgressPrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "milestone_progress",
		Title:       toolutil.TitleFromName("milestone_progress"),
		Description: "Track milestone progress for a project. Shows issue/MR completion, progress bar, and due date risk. Omit milestone argument to see all active milestones.",
		Icons:       toolutil.IconMilestone,
		Arguments: []*mcp.PromptArgument{
			projectIDArg(),
			{Name: "milestone", Title: toolutil.TitleFromName("milestone"), Description: "Specific milestone title (omit for all active)", Required: false},
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleMilestoneProgress(ctx, client, req)
	})
}

// handleMilestoneProgress performs the handle milestone progress operation using the GitLab API and returns [*mcp.GetPromptResult].
func handleMilestoneProgress(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	projectID := req.Params.Arguments[argProjectID]
	if projectID == "" {
		return nil, errors.New("milestone_progress: project_id is required")
	}
	milestoneTitle := req.Params.Arguments["milestone"]

	opts := &gl.ListMilestonesOptions{
		State:       new("active"),
		ListOptions: gl.ListOptions{PerPage: maxListItems},
	}
	if milestoneTitle != "" {
		opts.Title = new(milestoneTitle)
	}

	milestones, _, err := client.GL().Milestones.ListMilestones(projectID, opts, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("milestone_progress: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Milestone Progress — %s\n\n", projectID)

	if len(milestones) == 0 {
		b.WriteString("No active milestones found.\n")
		return promptResult(b.String()), nil
	}

	for _, ms := range milestones {
		fmt.Fprintf(&b, "## %s\n\n", ms.Title)

		issues, _, _ := client.GL().Milestones.GetMilestoneIssues(projectID, ms.ID, &gl.GetMilestoneIssuesOptions{
			ListOptions: gl.ListOptions{PerPage: maxListItems},
		}, gl.WithContext(ctx))
		mrs, _, _ := client.GL().Milestones.GetMilestoneMergeRequests(projectID, ms.ID, &gl.GetMilestoneMergeRequestsOptions{
			ListOptions: gl.ListOptions{PerPage: maxListItems},
		}, gl.WithContext(ctx))

		closedIssues, openIssues := countIssueStates(issues)
		mergedMRs, openMRs := countMRStates(mrs)

		totalItems := len(issues) + len(mrs)
		completedItems := closedIssues + mergedMRs

		fmt.Fprintf(&b, "%s\n\n", progressBar(completedItems, totalItems))

		b.WriteString("| Category | Count |\n|----------|-------|\n")
		fmt.Fprintf(&b, "| Total issues | %d |\n", len(issues))
		fmt.Fprintf(&b, "| Closed issues | %d |\n", closedIssues)
		fmt.Fprintf(&b, "| Open issues | %d |\n", openIssues)
		fmt.Fprintf(&b, "| Total MRs | %d |\n", len(mrs))
		fmt.Fprintf(&b, "| Merged MRs | %d |\n", mergedMRs)
		fmt.Fprintf(&b, "| Open MRs | %d |\n", openMRs)

		writeDueDateSection(&b, ms.DueDate)
		b.WriteString("\n")
	}

	b.WriteString("---\nPlease analyze milestone progress, identify risks of missing due dates, and suggest priorities.\n")

	return promptResult(b.String()), nil
}

// registerLabelDistributionPrompt registers the label_distribution prompt.
func registerLabelDistributionPrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "label_distribution",
		Title:       toolutil.TitleFromName("label_distribution"),
		Description: "Analyze label usage distribution in a project. Shows open/closed issue counts and open MR counts per label. Zero additional API calls beyond label list.",
		Icons:       toolutil.IconLabel,
		Arguments: []*mcp.PromptArgument{
			projectIDArg(),
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleLabelDistribution(ctx, client, req)
	})
}

// handleLabelDistribution performs the handle label distribution operation using the GitLab API and returns [*mcp.GetPromptResult].
func handleLabelDistribution(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	projectID := req.Params.Arguments[argProjectID]
	if projectID == "" {
		return nil, errors.New("label_distribution: project_id is required")
	}

	labels, _, err := client.GL().Labels.ListLabels(projectID, &gl.ListLabelsOptions{
		ListOptions: gl.ListOptions{PerPage: maxListItems},
	}, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("label_distribution: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Label Distribution — %s (%d labels)\n\n", projectID, len(labels))

	if len(labels) == 0 {
		b.WriteString("No labels found in this project.\n")
		return promptResult(b.String()), nil
	}

	// Sort labels by total usage (open+closed issues + open MRs) descending
	sort.Slice(labels, func(i, j int) bool {
		totalI := labels[i].OpenIssuesCount + labels[i].ClosedIssuesCount + labels[i].OpenMergeRequestsCount
		totalJ := labels[j].OpenIssuesCount + labels[j].ClosedIssuesCount + labels[j].OpenMergeRequestsCount
		return totalI > totalJ
	})

	b.WriteString("| Label | Open Issues | Closed Issues | Open MRs | Total |\n")
	b.WriteString("|-------|-------------|---------------|----------|-------|\n")
	var totalOpen, totalClosed, totalMRs int64
	for _, l := range labels {
		total := l.OpenIssuesCount + l.ClosedIssuesCount + l.OpenMergeRequestsCount
		if total == 0 {
			continue // skip unused labels
		}
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %d |\n", l.Name, l.OpenIssuesCount, l.ClosedIssuesCount, l.OpenMergeRequestsCount, total)
		totalOpen += l.OpenIssuesCount
		totalClosed += l.ClosedIssuesCount
		totalMRs += l.OpenMergeRequestsCount
	}
	fmt.Fprintf(&b, "| **Total** | **%d** | **%d** | **%d** | **%d** |\n", totalOpen, totalClosed, totalMRs, totalOpen+totalClosed+totalMRs)
	b.WriteString("\n")

	// Mermaid pie chart for top labels by open issues
	var pieLabels []string
	for _, l := range labels {
		if l.OpenIssuesCount > 0 && len(pieLabels) < 8 {
			pieLabels = append(pieLabels, fmt.Sprintf("    %q : %d", l.Name, l.OpenIssuesCount))
		}
	}
	if len(pieLabels) > 0 {
		b.WriteString("```mermaid\npie title Open Issues by Label\n")
		for _, line := range pieLabels {
			b.WriteString(line + "\n")
		}
		b.WriteString("```\n\n")
	}

	b.WriteString("---\nPlease analyze label usage patterns, identify underused labels, and suggest improvements to the labeling strategy.\n")

	return promptResult(b.String()), nil
}

// registerGroupMilestoneProgressPrompt registers the group_milestone_progress prompt.
func registerGroupMilestoneProgressPrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "group_milestone_progress",
		Title:       toolutil.TitleFromName("group_milestone_progress"),
		Description: "Track milestone progress across all projects in a group. Shows issue/MR completion per milestone with progress bars.",
		Icons:       toolutil.IconMilestone,
		Arguments: []*mcp.PromptArgument{
			groupIDArg(),
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleGroupMilestoneProgress(ctx, client, req)
	})
}

// handleGroupMilestoneProgress performs the handle group milestone progress operation using the GitLab API and returns [*mcp.GetPromptResult].
func handleGroupMilestoneProgress(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	groupID := req.Params.Arguments[argGroupID]
	if groupID == "" {
		return nil, errors.New("group_milestone_progress: group_id is required")
	}

	milestones, _, err := client.GL().GroupMilestones.ListGroupMilestones(groupID, &gl.ListGroupMilestonesOptions{
		State:       new("active"),
		ListOptions: gl.ListOptions{PerPage: maxListItems},
	}, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("group_milestone_progress: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Group Milestone Progress — %s\n\n", groupID)

	if len(milestones) == 0 {
		b.WriteString("No active group milestones found.\n")
		return promptResult(b.String()), nil
	}

	for _, ms := range milestones {
		fmt.Fprintf(&b, "## %s\n\n", ms.Title)

		issues, _, _ := client.GL().GroupMilestones.GetGroupMilestoneIssues(groupID, ms.ID, &gl.GetGroupMilestoneIssuesOptions{
			ListOptions: gl.ListOptions{PerPage: maxListItems},
		}, gl.WithContext(ctx))
		mrs, _, _ := client.GL().GroupMilestones.GetGroupMilestoneMergeRequests(groupID, ms.ID, &gl.GetGroupMilestoneMergeRequestsOptions{
			ListOptions: gl.ListOptions{PerPage: maxListItems},
		}, gl.WithContext(ctx))

		closedIssues, _ := countIssueStates(issues)
		mergedMRs, _ := countMRStates(mrs)

		totalItems := len(issues) + len(mrs)
		completedItems := closedIssues + mergedMRs

		fmt.Fprintf(&b, "%s\n\n", progressBar(completedItems, totalItems))

		b.WriteString("| Category | Count |\n|----------|-------|\n")
		fmt.Fprintf(&b, "| Issues (closed/total) | %d/%d |\n", closedIssues, len(issues))
		fmt.Fprintf(&b, "| MRs (merged/total) | %d/%d |\n", mergedMRs, len(mrs))

		writeDueDateSection(&b, ms.DueDate)
		b.WriteString("\n")
	}

	b.WriteString("---\nPlease analyze group milestone progress and identify cross-project risks.\n")

	return promptResult(b.String()), nil
}

// registerProjectContributorsPrompt registers the project_contributors prompt.
func registerProjectContributorsPrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "project_contributors",
		Title:       toolutil.TitleFromName("project_contributors"),
		Description: "Rank project contributors by commits, additions, and deletions. Uses the repository contributors API for accurate stats.",
		Icons:       toolutil.IconUser,
		Arguments: []*mcp.PromptArgument{
			projectIDArg(),
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleProjectContributors(ctx, client, req)
	})
}

// handleProjectContributors performs the handle project contributors operation using the GitLab API and returns [*mcp.GetPromptResult].
func handleProjectContributors(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	projectID := req.Params.Arguments[argProjectID]
	if projectID == "" {
		return nil, errors.New("project_contributors: project_id is required")
	}

	contributors, _, err := client.GL().Repositories.Contributors(projectID, &gl.ListContributorsOptions{
		ListOptions: gl.ListOptions{PerPage: maxListItems},
	}, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("project_contributors: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Project Contributors — %s (%d contributors)\n\n", projectID, len(contributors))

	if len(contributors) == 0 {
		b.WriteString("No contributors found.\n")
		return promptResult(b.String()), nil
	}

	// Already sorted by commits desc from GitLab API
	b.WriteString("| Contributor | Commits | Additions | Deletions |\n")
	b.WriteString("|-------------|---------|-----------|----------|\n")
	var totalCommits, totalAdditions, totalDeletions int64
	for _, c := range contributors {
		fmt.Fprintf(&b, "| %s | %d | +%d | -%d |\n", c.Name, c.Commits, c.Additions, c.Deletions)
		totalCommits += c.Commits
		totalAdditions += c.Additions
		totalDeletions += c.Deletions
	}
	fmt.Fprintf(&b, "| **Total** | **%d** | **+%d** | **-%d** |\n", totalCommits, totalAdditions, totalDeletions)
	b.WriteString("\n")

	// Pie chart for commits
	var pieEntries []string
	for _, c := range contributors {
		if len(pieEntries) >= 8 {
			break
		}
		pieEntries = append(pieEntries, fmt.Sprintf("    %q : %d", c.Name, c.Commits))
	}
	if len(pieEntries) > 0 {
		b.WriteString("```mermaid\npie title Commits by Contributor\n")
		for _, line := range pieEntries {
			b.WriteString(line + "\n")
		}
		b.WriteString("```\n\n")
	}

	b.WriteString("---\nPlease analyze contributor distribution, identify key contributors, and note any bus factor risks.\n")

	return promptResult(b.String()), nil
}
