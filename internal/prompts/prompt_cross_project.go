// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package prompts

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// registerMyOpenMRsPrompt registers the my_open_mrs prompt.
func registerMyOpenMRsPrompt(server promptAdder, client *gitlabclient.Client) {
	addPrompt(server, &mcp.Prompt{
		Name:        "my_open_mrs",
		Title:       toolutil.TitleFromName("my_open_mrs"),
		Description: "Show the open merge requests across all projects where you are author or assignee, up to 100 of each. Results are grouped by project for easy scanning. Use this to get a personal MR dashboard without specifying a project.",
		Icons:       toolutil.IconMR,
		Arguments: []*mcp.PromptArgument{
			usernameArg(),
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleMyOpenMRs(ctx, client.For(ctx), req)
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
			AuthorID: new(userID),
			State:    new("opened"),
			Scope:    new(scopeAll),
			PerPage:  maxListItems,
		}, gl.WithContext(ctx),
	)
	if errAuthored != nil {
		slog.WarnContext(ctx, "failed to list authored MRs", "error", errAuthored)
	}

	// MRs assigned to the user
	assignedMRs, _, errAssigned := client.GL().MergeRequests.ListMergeRequests(
		&gl.ListMergeRequestsOptions{
			AssigneeID: gl.AssigneeID(userID),
			State:      new("opened"),
			Scope:      new(scopeAll),
			PerPage:    maxListItems,
		}, gl.WithContext(ctx),
	)
	if errAssigned != nil {
		slog.WarnContext(ctx, "failed to list assigned MRs", "error", errAssigned)
	}

	allMRs := deduplicateMRs(authoredMRs, assignedMRs)
	grouped := groupMRsByProject(allMRs)

	// Count categories. The set is keyed on the MR's identity rather than on
	// its IID: this prompt spans every project the caller can see, and !1 of
	// one project is not !1 of another. Keyed on the IID alone, an MR assigned
	// to the caller counted as authored by them whenever any project they
	// author in happened to have an MR with the same number, which on a busy
	// account is most of them.
	var draftCount, conflictCount, authoredCount, assignedOnlyCount int
	authoredSet := make(map[mrIdentity]bool, len(authoredMRs))
	for _, mr := range authoredMRs {
		authoredSet[identifyMR(mr)] = true
	}
	for _, mr := range allMRs {
		if mr.Draft {
			draftCount++
		}
		if mr.HasConflicts {
			conflictCount++
		}
		if authoredSet[identifyMR(mr)] {
			authoredCount++
		}
	}
	assignedOnlyCount = len(allMRs) - authoredCount

	var b strings.Builder
	fmt.Fprintf(&b, "# Open Merge Requests for @%s\n\n", mdHeading(username))

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
		fmt.Fprintf(&b, "\n## %s (%d MRs)\n\n", mdHeading(project), len(mrs))
		writeMRTable(&b, mrs)
	}

	writeClosingRule(&b, "Please summarize the status of these MRs, highlight any that need attention (conflicts, stale >7d, failing pipeline), and suggest priorities.")

	return promptResult(b.String()), nil
}

// registerMyPendingReviewsPrompt registers the my_pending_reviews prompt.
func registerMyPendingReviewsPrompt(server promptAdder, client *gitlabclient.Client) {
	addPrompt(server, &mcp.Prompt{
		Name:        "my_pending_reviews",
		Title:       toolutil.TitleFromName("my_pending_reviews"),
		Description: "Show up to 100 open merge requests across all projects where you are assigned as reviewer. Helps track which MRs are waiting for your review. Results grouped by project.",
		Icons:       toolutil.IconMR,
		Arguments: []*mcp.PromptArgument{
			usernameArg(),
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleMyPendingReviews(ctx, client.For(ctx), req)
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
			ReviewerID: gl.ReviewerID(userID),
			State:      new("opened"),
			Scope:      new(scopeAll),
			PerPage:    maxListItems,
		}, gl.WithContext(ctx),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list pending reviews: %w", err)
	}

	grouped := groupMRsByProject(mrs)

	var b strings.Builder
	fmt.Fprintf(&b, "# Pending Reviews for @%s (%d MRs)\n\n", mdHeading(username), len(mrs))

	if len(mrs) == 0 {
		b.WriteString("No pending reviews found. You're all caught up! " + toolutil.EmojiParty + "\n")
	} else {
		for _, project := range sortedKeys(grouped) {
			projectMRs := grouped[project]
			fmt.Fprintf(&b, "## %s (%d MRs)\n\n", mdHeading(project), len(projectMRs))
			writeMRTable(&b, projectMRs)
			b.WriteString("\n")
		}
	}

	writeClosingRule(&b, "Please prioritize these reviews, highlighting urgent ones (old >5d, large changes, or blocking release).")

	return promptResult(b.String()), nil
}

// registerMyIssuesPrompt registers the my_issues prompt.
func registerMyIssuesPrompt(server promptAdder, client *gitlabclient.Client) {
	addPrompt(server, &mcp.Prompt{
		Name:        "my_issues",
		Title:       toolutil.TitleFromName("my_issues"),
		Description: "Show up to 100 issues assigned to you across all projects. Includes overdue detection and project grouping. Use this to see your issue backlog without specifying a project.",
		Icons:       toolutil.IconIssue,
		Arguments: []*mcp.PromptArgument{
			usernameArg(),
			issueStateArg("opened"),
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleMyIssues(ctx, client.For(ctx), req)
	})
}

// handleMyIssues lists all issues assigned to the user across all projects.
func handleMyIssues(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	args := req.Params.Arguments
	username, userID, isSelf, err := resolveUser(ctx, client, args[argUsername])
	if err != nil {
		return nil, err
	}

	state := getArgOr(args, argState, "opened")

	issues, partial, err := listIssuesAssignedTo(ctx, client, username, userID, isSelf, state)
	if err != nil {
		return nil, err
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
	fmt.Fprintf(&b, "# Issues Assigned to @%s (%d issues, state: %s)\n\n", mdHeading(username), len(issues), state)
	if partial != "" {
		fmt.Fprintf(&b, "> GitLab answered this list in part (%s), so an issue it could not resolve may be missing from it.\n\n", mdInline(partial))
	}

	b.WriteString("## Summary\n")
	b.WriteString(tableCategoryHeader)
	b.WriteString(tableCategorySeparator)
	fmt.Fprintf(&b, fmtTableCountRow, "Total", len(issues))
	fmt.Fprintf(&b, fmtTableCountRow, "Overdue", overdueCount)
	fmt.Fprintf(&b, fmtTableCountRow, "No milestone", noMilestoneCount)

	for _, project := range sortedKeys(grouped) {
		projectIssues := grouped[project]
		fmt.Fprintf(&b, "\n## %s (%d issues)\n\n", mdHeading(project), len(projectIssues))
		writeIssueTable(&b, projectIssues)
	}

	writeClosingRule(&b, "Please summarize the issue backlog, highlight overdue items, and suggest priorities based on due dates and labels.")

	return promptResult(b.String()), nil
}

// scopeAllRefusal is the sentence GitLab.com refuses scope=all on the global
// issue list with: reject_global_scope_all! in lib/api/issues.rb, behind the
// gitlab_com_derisk feature flag restrict_global_issues_scope_all (19.6),
// answers a 400 whose message is "400 Bad request - ", this sentence and the
// scopes and lists to use instead. The status alone does not say it, since
// GitLab answers 400 for any parameter it cannot read.
const scopeAllRefusal = "`scope=all` is not supported on this endpoint"

// refusesGlobalScopeAll reports whether GitLab answered err with GitLab.com's
// refusal of scope=all on the global issue list (issue 1779).
func refusesGlobalScopeAll(err error) bool {
	return toolutil.IsHTTPStatus(err, http.StatusBadRequest) && toolutil.ContainsAny(err, scopeAllRefusal)
}

// listIssuesAssignedTo reads up to [maxListItems] issues in state assigned to
// the user, across every project the caller can see, and returns with them
// the account [toolutil.GraphQLPartialPage] gives of a list GitLab answered in
// part, empty for one it answered whole.
//
// The caller's own issues are what scope=assigned_to_me names, which every
// GitLab accepts, GitLab.com included. Another user's are named by the
// assignee filter alone, so the scope has to name everybody's issues (issue
// 1164), and GitLab.com refuses scope=all on this list (issue 1779). There the
// same issues are read from GraphQL's root issue list instead
// ([listIssuesAssignedToByGraphQL]), which that refusal does not reach.
func listIssuesAssignedTo(ctx context.Context, client *gitlabclient.Client, username string, userID int64, isSelf bool, state string) (issues []*gl.Issue, partial string, err error) {
	opts := &gl.ListIssuesOptions{
		State:   new(state),
		Scope:   new(scopeAssignedToMe),
		PerPage: maxListItems,
	}
	if !isSelf {
		opts.AssigneeID = gl.AssigneeID(userID)
		opts.Scope = new(scopeAll)
	}
	issues, _, err = client.GL().Issues.ListIssues(opts, gl.WithContext(ctx))
	if err == nil {
		return issues, "", nil
	}
	if !refusesGlobalScopeAll(err) {
		return nil, "", fmt.Errorf("failed to list issues: %w", err)
	}
	issues, partial, err = listIssuesAssignedToByGraphQL(ctx, client, username, state)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list issues: GitLab.com refuses scope=all on the REST issue list, and the GraphQL issue list read in its place failed: %w", err)
	}
	return issues, partial, nil
}

// myIssuesOperation names the GraphQL read of my_issues in the errors and the
// partial-page account it gives.
const myIssuesOperation = "my_issues"

// assignedIssuesQuery reads the issues assigned to one user across every
// project the caller can see, from GraphQL's root issue list (Query.issues),
// which is what the issue dashboard of GitLab.com reads and which GitLab.com
// does not refuse the way it refuses scope=all on GET /issues (issue 1779).
//
// It is the REST read of my_issues about another user in another form: the
// assignee by username, the state, the same page of [maxListItems], the same
// order (created_desc is the default sort of both), archived projects left out
// on both (includeArchived and non_archived default to it), and the same issue
// types. GET /issues without an issue_type reads every base type but epic,
// key_result and objective (unfiltered_base_types_for_issues, which
// lib/api/helpers/issues_helpers.rb passes to the finder), while the root
// resolver's types argument has no default and would list objectives and key
// results as well, so the document names the six: ISSUE, INCIDENT, TEST_CASE,
// REQUIREMENT, TASK and TICKET. GitLab marks the field an experiment and
// refuses it without a filter, which the assignee is. It selects what the
// summary and the issue table read: the reference names the project an issue
// is grouped under, as references.full does on REST.
const assignedIssuesQuery = `query AssignedIssues($assignees: [String!], $state: IssuableState, $first: Int) {
  issues(assigneeUsernames: $assignees, state: $state, types: [ISSUE, INCIDENT, TEST_CASE, REQUIREMENT, TASK, TICKET], first: $first) {
    nodes {
      iid
      title
      reference(full: true)
      createdAt
      dueDate
      milestone {
        title
      }
      labels {
        nodes {
          title
        }
      }
    }
  }
}`

// assignedIssuesAnswer is GitLab's answer to [assignedIssuesQuery]. The list is
// a pointer so a list GitLab answered null reads apart from an empty one, and
// its nodes are pointers so a node answered null is counted rather than read
// as an issue of zero values (issue 1298).
type assignedIssuesAnswer struct {
	Data struct {
		Issues *struct {
			Nodes []*assignedIssueNode `json:"nodes"`
		} `json:"issues"`
	} `json:"data"`
	Errors []toolutil.GraphQLError `json:"errors"`
}

// assignedIssueNode is one issue of [assignedIssuesAnswer]. GitLab sends the
// iid as a String, which the ",string" option reads as the number REST sends,
// and the due date as a date, which [gl.ISOTime] reads.
type assignedIssueNode struct {
	IID       int64       `json:"iid,string"`
	Title     string      `json:"title"`
	Reference string      `json:"reference"`
	CreatedAt time.Time   `json:"createdAt"`
	DueDate   *gl.ISOTime `json:"dueDate"`
	Milestone *struct {
		Title string `json:"title"`
	} `json:"milestone"`
	Labels *struct {
		Nodes []*struct {
			Title string `json:"title"`
		} `json:"nodes"`
	} `json:"labels"`
}

// issue converts n to the REST issue the summary and the issue table read,
// leaving out a label GitLab answered null.
func (n assignedIssueNode) issue() *gl.Issue {
	issue := &gl.Issue{
		IID:        n.IID,
		Title:      n.Title,
		CreatedAt:  &n.CreatedAt,
		DueDate:    n.DueDate,
		References: &gl.IssueReferences{Full: n.Reference},
	}
	if n.Milestone != nil {
		issue.Milestone = &gl.Milestone{Title: n.Milestone.Title}
	}
	if n.Labels != nil {
		for _, label := range n.Labels.Nodes {
			if label != nil {
				issue.Labels = append(issue.Labels, label.Title)
			}
		}
	}
	return issue
}

// listIssuesAssignedToByGraphQL reads the issues [assignedIssuesQuery] lists
// for username in state, and the account [toolutil.GraphQLPartialPage] gives
// of a page GitLab answered in part. A list GitLab answered null is an error:
// with errors[] beside it, theirs, and without them one saying so, since
// read as an empty list it would say the user has no issues, which nobody
// knows.
func listIssuesAssignedToByGraphQL(ctx context.Context, client *gitlabclient.Client, username, state string) (issues []*gl.Issue, partial string, err error) {
	var answer assignedIssuesAnswer
	_, err = client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query: assignedIssuesQuery,
		Variables: map[string]any{
			"assignees": []string{username},
			"state":     state,
			"first":     maxListItems,
		},
	}, &answer, gl.WithContext(ctx))
	if err != nil {
		return nil, "", err
	}
	if answer.Data.Issues == nil {
		if graphQLErr := toolutil.GraphQLTopLevelError(myIssuesOperation, answer.Errors); graphQLErr != nil {
			return nil, "", graphQLErr
		}
		return nil, "", fmt.Errorf("%s: GitLab answered the issue list null with no error", myIssuesOperation)
	}
	issues, nulled := toolutil.NodesAnswered(answer.Data.Issues.Nodes, assignedIssueNode.issue)
	partial, err = toolutil.GraphQLPartialPage(myIssuesOperation, answer.Errors, len(issues), nulled)
	if err != nil {
		return nil, "", err
	}
	return issues, partial, nil
}

// registerMyActivitySummaryPrompt registers the my_activity_summary prompt.
func registerMyActivitySummaryPrompt(server promptAdder, client *gitlabclient.Client) {
	addPrompt(server, &mcp.Prompt{
		Name:        "my_activity_summary",
		Title:       toolutil.TitleFromName("my_activity_summary"),
		Description: "Generate a personal activity summary for a configurable period, across all projects: contribution events by action, the counts of MRs authored that were created in the period and merged and of MRs reviewed that were updated in it, and a daily activity chart, up to 100 per list.",
		Icons:       toolutil.IconUser,
		Arguments: []*mcp.PromptArgument{
			usernameArg(),
			daysArg(7),
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleMyActivitySummary(ctx, client.For(ctx), req)
	})
}

// handleMyActivitySummary aggregates user activity across all projects.
func handleMyActivitySummary(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	args := req.Params.Arguments
	days, err := parseDaysArg(args, argDays, 7)
	if err != nil {
		return nil, err
	}
	username, userID, isCurrentUser, err := resolveUser(ctx, client, args[argUsername])
	if err != nil {
		return nil, err
	}

	since := sinceDate(days)

	// Fetch contribution events
	events := fetchContributionEvents(ctx, client, userID, isCurrentUser, since)

	// Merged MRs in period
	mergedMRs, _, errMerged := client.GL().MergeRequests.ListMergeRequests(
		&gl.ListMergeRequestsOptions{
			AuthorID:     new(userID),
			State:        new("merged"),
			CreatedAfter: &since,
			Scope:        new(scopeAll),
			PerPage:      maxListItems,
		}, gl.WithContext(ctx),
	)
	if errMerged != nil {
		slog.WarnContext(ctx, "failed to list merged MRs", "error", errMerged)
	}

	// MRs reviewed in period
	reviewedMRs, _, errReviewed := client.GL().MergeRequests.ListMergeRequests(
		&gl.ListMergeRequestsOptions{
			ReviewerID:   gl.ReviewerID(userID),
			UpdatedAfter: new(since),
			Scope:        new(scopeAll),
			PerPage:      maxListItems,
		}, gl.WithContext(ctx),
	)
	if errReviewed != nil {
		slog.WarnContext(ctx, "failed to list reviewed MRs", "error", errReviewed)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Activity Summary for @%s (last %d days)\n\n", mdHeading(username), days)

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

	writeClosingRule(&b, "Please provide a comprehensive activity summary with productivity insights and trends.")

	return promptResult(b.String()), nil
}

// writeDailyActivityChart renders a Mermaid bar chart of daily event counts.
func writeDailyActivityChart(b *strings.Builder, dailyData []dayActivity) {
	b.WriteString("\n## Daily Activity\n\n")
	writeMermaidChart(b, func(chart *strings.Builder) {
		chart.WriteString("xychart-beta\n  title \"Daily Activity\"\n  x-axis [")
		for i, d := range dailyData {
			if i > 0 {
				chart.WriteString(", ")
			}
			chart.WriteString("\"" + d.date[5:] + "\"")
		}
		chart.WriteString("]\n  y-axis \"Events\"\n  bar [")
		for i, d := range dailyData {
			if i > 0 {
				chart.WriteString(", ")
			}
			fmt.Fprintf(chart, "%d", d.count)
		}
		chart.WriteString("]\n")
	})
}

// registerCrossProjectPrompts registers all cross-project prompts.
func registerCrossProjectPrompts(server promptAdder, client *gitlabclient.Client) {
	registerMyOpenMRsPrompt(server, client)
	registerMyPendingReviewsPrompt(server, client)
	registerMyIssuesPrompt(server, client)
	registerMyActivitySummaryPrompt(server, client)
}
