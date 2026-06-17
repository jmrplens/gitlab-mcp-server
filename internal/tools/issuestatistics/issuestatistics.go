package issuestatistics

import (
	"context"
	"net/http"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Shared output.

// StatisticsOutput contains issue statistics counts.
type StatisticsOutput struct {
	toolutil.HintableOutput
	All    int64 `json:"all"`
	Closed int64 `json:"closed"`
	Opened int64 `json:"opened"`
}

// fromGL converts a [gl.IssuesStatistics] response into the package's
// [StatisticsOutput], flattening the nested counts structure.
func fromGL(s *gl.IssuesStatistics) StatisticsOutput {
	return StatisticsOutput{
		All:    s.Statistics.Counts.All,
		Closed: s.Statistics.Counts.Closed,
		Opened: s.Statistics.Counts.Opened,
	}
}

// Get (global).

// GetInput contains parameters for global issue statistics.
type GetInput struct {
	Labels    string `json:"labels" jsonschema:"Comma-separated label names"`
	Milestone string `json:"milestone" jsonschema:"Milestone title"`
	Scope     string `json:"scope" jsonschema:"Scope: created_by_me, assigned_to_me, all"`
	Search    string `json:"search" jsonschema:"Search string"`
}

// Get retrieves global issue statistics across all projects visible to
// the authenticated user via the GitLab Issue statistics API
// (GET /issues_statistics). Optional filters narrow the result by label,
// milestone, scope, or free-text search.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (StatisticsOutput, error) {
	opts := &gl.GetIssuesStatisticsOptions{}
	if input.Labels != "" {
		lbl := gl.LabelOptions(strings.Split(input.Labels, ","))
		opts.Labels = &lbl
	}
	if input.Milestone != "" {
		opts.Milestone = new(input.Milestone)
	}
	if input.Scope != "" {
		opts.Scope = new(input.Scope)
	}
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	stats, _, err := client.GL().IssuesStatistics.GetIssuesStatistics(opts, gl.WithContext(ctx))
	if err != nil {
		return StatisticsOutput{}, toolutil.WrapErrWithStatusHint("gitlab_get_issue_statistics", err, http.StatusForbidden, "verify your token has read_api scope")
	}
	return fromGL(stats), nil
}

// GetGroup.

// GetGroupInput contains parameters for group issue statistics.
type GetGroupInput struct {
	GroupID   string `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	Labels    string `json:"labels" jsonschema:"Comma-separated label names"`
	Milestone string `json:"milestone" jsonschema:"Milestone title"`
	Scope     string `json:"scope" jsonschema:"Scope: created_by_me, assigned_to_me, all"`
	Search    string `json:"search" jsonschema:"Search string"`
}

// GetGroup retrieves issue statistics scoped to a group and its
// descendant projects via the GitLab Group Issue statistics API
// (GET /groups/:id/issues_statistics).
func GetGroup(ctx context.Context, client *gitlabclient.Client, input GetGroupInput) (StatisticsOutput, error) {
	stats, _, err := client.GL().IssuesStatistics.GetGroupIssuesStatistics(input.GroupID, groupIssueStatsOptions(statisticsFilters{
		Labels: input.Labels, Milestone: input.Milestone, Scope: input.Scope, Search: input.Search,
	}), gl.WithContext(ctx))
	if err != nil {
		return StatisticsOutput{}, toolutil.WrapErrWithStatusHint("gitlab_get_group_issue_statistics", err, http.StatusNotFound, "verify group_id with gitlab_group_get")
	}
	return fromGL(stats), nil
}

// statisticsFilters is the shared filter set for group and project
// issue-statistics lookups, used to keep the option-builder helpers in
// lockstep across the two scopes.
type statisticsFilters struct {
	Labels    string
	Milestone string
	Scope     string
	Search    string
}

// applyStatisticsFilters copies non-empty filter values into the supplied
// option setters, avoiding the need to repeat the same nil checks in
// every helper.
func applyStatisticsFilters(filters statisticsFilters, setLabels func(*gl.LabelOptions), setMilestone, setScope, setSearch func(*string)) {
	if filters.Labels != "" {
		labels := gl.LabelOptions(strings.Split(filters.Labels, ","))
		setLabels(&labels)
	}
	if filters.Milestone != "" {
		setMilestone(&filters.Milestone)
	}
	if filters.Scope != "" {
		setScope(&filters.Scope)
	}
	if filters.Search != "" {
		setSearch(&filters.Search)
	}
}

// groupIssueStatsOptions builds a [gl.GetGroupIssuesStatisticsOptions]
// from the shared [statisticsFilters] set.
func groupIssueStatsOptions(filters statisticsFilters) *gl.GetGroupIssuesStatisticsOptions {
	opts := &gl.GetGroupIssuesStatisticsOptions{}
	applyStatisticsFilters(filters,
		func(value *gl.LabelOptions) { opts.Labels = value },
		func(value *string) { opts.Milestone = value },
		func(value *string) { opts.Scope = value },
		func(value *string) { opts.Search = value })
	return opts
}

// GetProject.

// GetProjectInput contains parameters for project issue statistics.
type GetProjectInput struct {
	ProjectID string `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Labels    string `json:"labels" jsonschema:"Comma-separated label names"`
	Milestone string `json:"milestone" jsonschema:"Milestone title"`
	Scope     string `json:"scope" jsonschema:"Scope: created_by_me, assigned_to_me, all"`
	Search    string `json:"search" jsonschema:"Search string"`
}

// GetProject retrieves issue statistics scoped to a single project via
// the GitLab Project Issue statistics API
// (GET /projects/:id/issues_statistics).
func GetProject(ctx context.Context, client *gitlabclient.Client, input GetProjectInput) (StatisticsOutput, error) {
	stats, _, err := client.GL().IssuesStatistics.GetProjectIssuesStatistics(input.ProjectID, projectIssueStatsOptions(statisticsFilters{
		Labels: input.Labels, Milestone: input.Milestone, Scope: input.Scope, Search: input.Search,
	}), gl.WithContext(ctx))
	if err != nil {
		return StatisticsOutput{}, toolutil.WrapErrWithStatusHint("gitlab_get_project_issue_statistics", err, http.StatusNotFound, "verify project_id with gitlab_project_get")
	}
	return fromGL(stats), nil
}

// projectIssueStatsOptions builds a [gl.GetProjectIssuesStatisticsOptions]
// from the shared [statisticsFilters] set.
func projectIssueStatsOptions(filters statisticsFilters) *gl.GetProjectIssuesStatisticsOptions {
	opts := &gl.GetProjectIssuesStatisticsOptions{}
	applyStatisticsFilters(filters,
		func(value *gl.LabelOptions) { opts.Labels = value },
		func(value *string) { opts.Milestone = value },
		func(value *string) { opts.Scope = value },
		func(value *string) { opts.Search = value })
	return opts
}

// formatters.
