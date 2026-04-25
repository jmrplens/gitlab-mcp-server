// search.go implements GitLab API search functions used by the completion
// handler. Each function queries a specific GitLab API endpoint and returns
// formatted string entries suitable for MCP completion results.

package completions

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
)

// searchPerPage is intentionally larger than maxCompletionResults (10) so
// that toResult can compute an accurate HasMore flag after truncation.
const searchPerPage = 20

// totalFromResponse extracts the total match count from a GitLab REST
// pagination response. GitLab populates [gitlab.Response.TotalItems] from
// the X-Total header for offset-paginated endpoints; it is 0 for keyset
// pagination or when the server omits the header (e.g. on collections
// large enough to skip counting).
func totalFromResponse(resp *gl.Response) int {
	if resp == nil {
		return 0
	}
	return int(resp.TotalItems)
}

// searchProjects returns project entries matching the query plus the total
// match count from the GitLab pagination header (X-Total) when available.
func searchProjects(ctx context.Context, client *gitlabclient.Client, query string) ([]string, int, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	opts := &gl.ListProjectsOptions{
		Membership: new(true),
	}
	opts.PerPage = searchPerPage
	if query != "" {
		opts.Search = new(query)
	}
	projects, resp, err := client.GL().Projects.ListProjects(opts, gl.WithContext(ctx))
	if err != nil {
		return nil, 0, fmt.Errorf("search projects: %w", err)
	}
	values := make([]string, 0, len(projects))
	for _, p := range projects {
		values = append(values, formatProjectEntry(p.ID, p.PathWithNamespace))
	}
	return values, totalFromResponse(resp), nil
}

// searchGroups returns group entries matching the query plus the total
// match count from the GitLab pagination header.
func searchGroups(ctx context.Context, client *gitlabclient.Client, query string) ([]string, int, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	opts := &gl.ListGroupsOptions{}
	opts.PerPage = searchPerPage
	if query != "" {
		opts.Search = new(query)
	}
	groups, resp, err := client.GL().Groups.ListGroups(opts, gl.WithContext(ctx))
	if err != nil {
		return nil, 0, fmt.Errorf("search groups: %w", err)
	}
	values := make([]string, 0, len(groups))
	for _, g := range groups {
		values = append(values, formatGroupEntry(g.ID, g.FullPath))
	}
	return values, totalFromResponse(resp), nil
}

// searchUsers returns usernames matching the query plus the total match
// count from the GitLab pagination header.
func searchUsers(ctx context.Context, client *gitlabclient.Client, query string) ([]string, int, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	opts := &gl.ListUsersOptions{
		Active: new(true),
	}
	opts.PerPage = searchPerPage
	if query != "" {
		opts.Search = new(query)
	}
	users, resp, err := client.GL().Users.ListUsers(opts, gl.WithContext(ctx))
	if err != nil {
		return nil, 0, fmt.Errorf("search users: %w", err)
	}
	values := make([]string, 0, len(users))
	for _, u := range users {
		values = append(values, u.Username)
	}
	return values, totalFromResponse(resp), nil
}

// searchMRs returns merge request entries for a project, filtered by IID prefix.
func searchMRs(ctx context.Context, client *gitlabclient.Client, projectID, query string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	opts := &gl.ListProjectMergeRequestsOptions{
		State: new("opened"),
	}
	opts.PerPage = searchPerPage
	mrs, _, err := client.GL().MergeRequests.ListProjectMergeRequests(projectID, opts, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("search merge requests: %w", err)
	}
	values := make([]string, 0, len(mrs))
	for _, mr := range mrs {
		entry := formatMREntry(mr.IID, mr.Title)
		if query == "" || strings.HasPrefix(strconv.FormatInt(mr.IID, 10), query) {
			values = append(values, entry)
		}
	}
	return values, nil
}

// searchIssues returns issue entries for a project, filtered by IID prefix.
func searchIssues(ctx context.Context, client *gitlabclient.Client, projectID, query string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	opts := &gl.ListProjectIssuesOptions{
		State: new("opened"),
	}
	opts.PerPage = searchPerPage
	issues, _, err := client.GL().Issues.ListProjectIssues(projectID, opts, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("search issues: %w", err)
	}
	values := make([]string, 0, len(issues))
	for _, issue := range issues {
		entry := formatIssueEntry(issue.IID, issue.Title)
		if query == "" || strings.HasPrefix(strconv.FormatInt(issue.IID, 10), query) {
			values = append(values, entry)
		}
	}
	return values, nil
}

// searchBranches returns branch names matching the query plus the total
// match count from the GitLab pagination header.
func searchBranches(ctx context.Context, client *gitlabclient.Client, projectID, query string) ([]string, int, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	opts := &gl.ListBranchesOptions{}
	opts.PerPage = searchPerPage
	if query != "" {
		opts.Search = new(query)
	}
	branches, resp, err := client.GL().Branches.ListBranches(projectID, opts, gl.WithContext(ctx))
	if err != nil {
		return nil, 0, fmt.Errorf("search branches: %w", err)
	}
	values := make([]string, 0, len(branches))
	for _, b := range branches {
		values = append(values, b.Name)
	}
	return values, totalFromResponse(resp), nil
}

// searchTags returns tag names matching the query plus the total match
// count from the GitLab pagination header.
func searchTags(ctx context.Context, client *gitlabclient.Client, projectID, query string) ([]string, int, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	opts := &gl.ListTagsOptions{}
	opts.PerPage = searchPerPage
	if query != "" {
		opts.Search = new(query)
	}
	tags, resp, err := client.GL().Tags.ListTags(projectID, opts, gl.WithContext(ctx))
	if err != nil {
		return nil, 0, fmt.Errorf("search tags: %w", err)
	}
	values := make([]string, 0, len(tags))
	for _, t := range tags {
		values = append(values, t.Name)
	}
	return values, totalFromResponse(resp), nil
}

// searchPipelines returns recent pipelines for a project, filtered by ID prefix.
func searchPipelines(ctx context.Context, client *gitlabclient.Client, projectID, query string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	opts := &gl.ListProjectPipelinesOptions{}
	opts.PerPage = searchPerPage
	opts.OrderBy = new("id")
	opts.Sort = new("desc")
	pipelines, _, err := client.GL().Pipelines.ListProjectPipelines(projectID, opts, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("search pipelines: %w", err)
	}
	values := make([]string, 0, len(pipelines))
	for _, p := range pipelines {
		entry := formatPipelineEntry(p.ID, p.Ref, p.Status)
		if query == "" || strings.HasPrefix(strconv.FormatInt(p.ID, 10), query) {
			values = append(values, entry)
		}
	}
	return values, nil
}

// searchCommits returns recent commits for a project, filtered by SHA prefix.
func searchCommits(ctx context.Context, client *gitlabclient.Client, projectID, query string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	opts := &gl.ListCommitsOptions{}
	opts.PerPage = searchPerPage
	commits, _, err := client.GL().Commits.ListCommits(projectID, opts, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("search commits: %w", err)
	}
	values := make([]string, 0, len(commits))
	for _, c := range commits {
		entry := formatCommitEntry(c.ShortID, c.Title)
		if query == "" || strings.HasPrefix(strings.ToLower(c.ShortID), strings.ToLower(query)) || strings.HasPrefix(strings.ToLower(c.ID), strings.ToLower(query)) {
			values = append(values, entry)
		}
	}
	return values, nil
}

// searchLabels returns label names for a project matching the query plus
// the total match count from the GitLab pagination header.
func searchLabels(ctx context.Context, client *gitlabclient.Client, projectID, query string) ([]string, int, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	opts := &gl.ListLabelsOptions{}
	opts.PerPage = searchPerPage
	if query != "" {
		opts.Search = new(query)
	}
	labels, resp, err := client.GL().Labels.ListLabels(projectID, opts, gl.WithContext(ctx))
	if err != nil {
		return nil, 0, fmt.Errorf("search labels: %w", err)
	}
	values := make([]string, 0, len(labels))
	for _, l := range labels {
		values = append(values, l.Name)
	}
	return values, totalFromResponse(resp), nil
}

// searchMilestones returns milestone entries for a project plus the total
// match count from the GitLab pagination header.
func searchMilestones(ctx context.Context, client *gitlabclient.Client, projectID, query string) ([]string, int, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	opts := &gl.ListMilestonesOptions{
		State: new("active"),
	}
	opts.PerPage = searchPerPage
	if query != "" {
		opts.Search = new(query)
	}
	milestones, resp, err := client.GL().Milestones.ListMilestones(projectID, opts, gl.WithContext(ctx))
	if err != nil {
		return nil, 0, fmt.Errorf("search milestones: %w", err)
	}
	values := make([]string, 0, len(milestones))
	for _, m := range milestones {
		values = append(values, formatMilestoneEntry(m.ID, m.Title))
	}
	return values, totalFromResponse(resp), nil
}

// searchMilestoneTitles returns active milestone titles for a project, filtered by query.
// Unlike [searchMilestones], it returns plain titles (not "id: title") for use as
// completion values for prompt arguments that accept a milestone title.
func searchMilestoneTitles(ctx context.Context, client *gitlabclient.Client, projectID, query string) ([]string, int, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	opts := &gl.ListMilestonesOptions{
		State: new("active"),
	}
	opts.PerPage = searchPerPage
	if query != "" {
		opts.Search = new(query)
	}
	milestones, resp, err := client.GL().Milestones.ListMilestones(projectID, opts, gl.WithContext(ctx))
	if err != nil {
		return nil, 0, fmt.Errorf("search milestone titles: %w", err)
	}
	values := make([]string, 0, len(milestones))
	for _, m := range milestones {
		values = append(values, m.Title)
	}
	return values, totalFromResponse(resp), nil
}

// searchJobs returns job entries for a pipeline, filtered by ID prefix.
func searchJobs(ctx context.Context, client *gitlabclient.Client, projectID string, pipelineID int64, query string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	opts := &gl.ListJobsOptions{}
	opts.PerPage = searchPerPage
	jobs, _, err := client.GL().Jobs.ListPipelineJobs(projectID, pipelineID, opts, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("search jobs: %w", err)
	}
	values := make([]string, 0, len(jobs))
	for _, j := range jobs {
		entry := formatJobEntry(j.ID, j.Name, j.Status)
		if query == "" || strings.HasPrefix(strconv.FormatInt(j.ID, 10), query) {
			values = append(values, entry)
		}
	}
	return values, nil
}
