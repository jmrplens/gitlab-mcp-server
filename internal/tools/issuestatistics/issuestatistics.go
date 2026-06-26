package issuestatistics

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Shared output.

// CountsOutput mirrors [gl.IssuesStatisticsCounts]: the all/closed/opened
// issue counts returned by the GitLab Issue statistics API.
type CountsOutput struct {
	All    int64 `json:"all"`
	Closed int64 `json:"closed"`
	Opened int64 `json:"opened"`
}

// StatisticsCountsOutput mirrors [gl.IssuesStatisticsStatistics], nesting the
// issue counts under a "counts" object exactly as the GitLab API does.
type StatisticsCountsOutput struct {
	Counts CountsOutput `json:"counts"`
}

// StatisticsOutput mirrors [gl.IssuesStatistics]: a single "statistics"
// object containing the nested issue counts. The nested shape matches the
// GitLab Issue statistics API response 1:1.
type StatisticsOutput struct {
	toolutil.HintableOutput
	Statistics StatisticsCountsOutput `json:"statistics"`
}

// fromGL converts a [gl.IssuesStatistics] response into the package's
// [StatisticsOutput], preserving the nested statistics/counts structure.
func fromGL(s *gl.IssuesStatistics) StatisticsOutput {
	return StatisticsOutput{
		Statistics: StatisticsCountsOutput{
			Counts: CountsOutput{
				All:    s.Statistics.Counts.All,
				Closed: s.Statistics.Counts.Closed,
				Opened: s.Statistics.Counts.Opened,
			},
		},
	}
}

// statisticsFilters is the shared filter set for global, group, and project
// issue-statistics lookups. It mirrors the common option fields of
// [gl.GetIssuesStatisticsOptions], [gl.GetGroupIssuesStatisticsOptions], and
// [gl.GetProjectIssuesStatisticsOptions] so the option-builder helpers stay in
// lockstep across the three scopes.
type statisticsFilters struct {
	Labels           []string `json:"labels,omitempty"`
	Milestone        string   `json:"milestone,omitempty"`
	Scope            string   `json:"scope,omitempty"`
	Search           string   `json:"search,omitempty"`
	AssigneeID       *int64   `json:"assignee_id,omitempty"`
	AssigneeUsername []string `json:"assignee_username,omitempty"`
	AuthorID         *int64   `json:"author_id,omitempty"`
	AuthorUsername   string   `json:"author_username,omitempty"`
	Confidential     *bool    `json:"confidential,omitempty"`
	CreatedAfter     string   `json:"created_after,omitempty"`
	CreatedBefore    string   `json:"created_before,omitempty"`
	UpdatedAfter     string   `json:"updated_after,omitempty"`
	UpdatedBefore    string   `json:"updated_before,omitempty"`
	IIDs             []int64  `json:"iids[],omitempty"`
	In               string   `json:"in,omitempty"`
	MyReactionEmoji  string   `json:"my_reaction_emoji,omitempty"`
}

// labelOptions converts a label-name slice into a [*gl.LabelOptions], or nil
// when empty.
func labelOptions(values []string) *gl.LabelOptions {
	if len(values) == 0 {
		return nil
	}
	labels := gl.LabelOptions(values)
	return &labels
}

// optStr returns a pointer to s, or nil when s is empty.
func optStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// optStrings returns a pointer to a string slice, or nil when empty.
func optStrings(values []string) *[]string {
	if len(values) == 0 {
		return nil
	}
	return &values
}

// optIIDs returns a pointer to the int64 IID slice, or nil when empty.
func optIIDs(iids []int64) *[]int64 {
	if len(iids) == 0 {
		return nil
	}
	return &iids
}

// Get (global).

// GetInput contains parameters for global issue statistics.
type GetInput struct {
	Labels           []string `json:"labels,omitempty"            jsonschema:"Label names to filter by"`
	Milestone        string   `json:"milestone,omitempty"         jsonschema:"Milestone title to filter by"`
	Scope            string   `json:"scope,omitempty"             jsonschema:"Scope: created_by_me, assigned_to_me, all"`
	Search           string   `json:"search,omitempty"            jsonschema:"Search string for title and description"`
	In               string   `json:"in,omitempty"                jsonschema:"Fields the search query applies to (title, description, or title,description)"`
	AssigneeID       *int64   `json:"assignee_id,omitempty"       jsonschema:"Filter by assignee user ID"`
	AssigneeUsername []string `json:"assignee_username,omitempty" jsonschema:"Filter by assignee usernames"`
	AuthorID         *int64   `json:"author_id,omitempty"         jsonschema:"Filter by author user ID"`
	AuthorUsername   string   `json:"author_username,omitempty"   jsonschema:"Filter by author username"`
	Confidential     *bool    `json:"confidential,omitempty"      jsonschema:"Filter by confidential status"`
	CreatedAfter     string   `json:"created_after,omitempty"     jsonschema:"Return issues created after date (ISO 8601, e.g. 2025-01-01T00:00:00Z)"`
	CreatedBefore    string   `json:"created_before,omitempty"    jsonschema:"Return issues created before date (ISO 8601, e.g. 2025-12-31T23:59:59Z)"`
	UpdatedAfter     string   `json:"updated_after,omitempty"     jsonschema:"Return issues updated after date (ISO 8601, e.g. 2025-01-01T00:00:00Z)"`
	UpdatedBefore    string   `json:"updated_before,omitempty"    jsonschema:"Return issues updated before date (ISO 8601, e.g. 2025-12-31T23:59:59Z)"`
	IIDs             []int64  `json:"iids,omitempty"              jsonschema:"Filter by issue internal IDs"`
	MyReactionEmoji  string   `json:"my_reaction_emoji,omitempty" jsonschema:"Filter by issues you reacted to with this emoji (or None/Any)"`
}

// Get retrieves global issue statistics across all projects visible to
// the authenticated user via the GitLab Issue statistics API
// (GET /issues_statistics). Optional filters narrow the result.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (StatisticsOutput, error) {
	opts := &gl.GetIssuesStatisticsOptions{
		Labels:           labelOptions(input.Labels),
		Milestone:        optStr(input.Milestone),
		Scope:            optStr(input.Scope),
		Search:           optStr(input.Search),
		In:               optStr(input.In),
		AuthorID:         input.AuthorID,
		AuthorUsername:   optStr(input.AuthorUsername),
		AssigneeID:       input.AssigneeID,
		AssigneeUsername: optStrings(input.AssigneeUsername),
		MyReactionEmoji:  optStr(input.MyReactionEmoji),
		IIDs:             optIIDs(input.IIDs),
		Confidential:     input.Confidential,
		CreatedAfter:     toolutil.ParseOptionalTime(input.CreatedAfter),
		CreatedBefore:    toolutil.ParseOptionalTime(input.CreatedBefore),
		UpdatedAfter:     toolutil.ParseOptionalTime(input.UpdatedAfter),
		UpdatedBefore:    toolutil.ParseOptionalTime(input.UpdatedBefore),
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
	GroupID          string   `json:"group_id"                    jsonschema:"Group ID or URL-encoded path,required"`
	Labels           []string `json:"labels,omitempty"            jsonschema:"Label names to filter by"`
	Milestone        string   `json:"milestone,omitempty"         jsonschema:"Milestone title to filter by"`
	Scope            string   `json:"scope,omitempty"             jsonschema:"Scope: created_by_me, assigned_to_me, all"`
	Search           string   `json:"search,omitempty"            jsonschema:"Search string for title and description"`
	AssigneeID       *int64   `json:"assignee_id,omitempty"       jsonschema:"Filter by assignee user ID"`
	AssigneeUsername []string `json:"assignee_username,omitempty" jsonschema:"Filter by assignee usernames"`
	AuthorID         *int64   `json:"author_id,omitempty"         jsonschema:"Filter by author user ID"`
	AuthorUsername   string   `json:"author_username,omitempty"   jsonschema:"Filter by author username"`
	Confidential     *bool    `json:"confidential,omitempty"      jsonschema:"Filter by confidential status"`
	CreatedAfter     string   `json:"created_after,omitempty"     jsonschema:"Return issues created after date (ISO 8601)"`
	CreatedBefore    string   `json:"created_before,omitempty"    jsonschema:"Return issues created before date (ISO 8601)"`
	UpdatedAfter     string   `json:"updated_after,omitempty"     jsonschema:"Return issues updated after date (ISO 8601)"`
	UpdatedBefore    string   `json:"updated_before,omitempty"    jsonschema:"Return issues updated before date (ISO 8601)"`
	IIDs             []int64  `json:"iids,omitempty"              jsonschema:"Filter by issue internal IDs"`
	MyReactionEmoji  string   `json:"my_reaction_emoji,omitempty" jsonschema:"Filter by issues you reacted to with this emoji (or None/Any)"`
}

// GetGroup retrieves issue statistics scoped to a group and its
// descendant projects via the GitLab Group Issue statistics API
// (GET /groups/:id/issues_statistics).
func GetGroup(ctx context.Context, client *gitlabclient.Client, input GetGroupInput) (StatisticsOutput, error) {
	stats, _, err := client.GL().IssuesStatistics.GetGroupIssuesStatistics(input.GroupID, groupIssueStatsOptions(statisticsFilters{
		Labels: input.Labels, Milestone: input.Milestone, Scope: input.Scope, Search: input.Search,
		AssigneeID: input.AssigneeID, AssigneeUsername: input.AssigneeUsername,
		AuthorID: input.AuthorID, AuthorUsername: input.AuthorUsername,
		Confidential: input.Confidential, CreatedAfter: input.CreatedAfter, CreatedBefore: input.CreatedBefore,
		UpdatedAfter: input.UpdatedAfter, UpdatedBefore: input.UpdatedBefore,
		IIDs: input.IIDs, MyReactionEmoji: input.MyReactionEmoji,
	}), gl.WithContext(ctx))
	if err != nil {
		return StatisticsOutput{}, toolutil.WrapErrWithStatusHint("gitlab_get_group_issue_statistics", err, http.StatusNotFound, "verify group_id with gitlab_group_get")
	}
	return fromGL(stats), nil
}

// groupIssueStatsOptions builds a [gl.GetGroupIssuesStatisticsOptions]
// from the shared [statisticsFilters] set.
func groupIssueStatsOptions(filters statisticsFilters) *gl.GetGroupIssuesStatisticsOptions {
	return &gl.GetGroupIssuesStatisticsOptions{
		Labels:           labelOptions(filters.Labels),
		IIDs:             optIIDs(filters.IIDs),
		Milestone:        optStr(filters.Milestone),
		Scope:            optStr(filters.Scope),
		AuthorID:         filters.AuthorID,
		AuthorUsername:   optStr(filters.AuthorUsername),
		AssigneeID:       filters.AssigneeID,
		AssigneeUsername: optStrings(filters.AssigneeUsername),
		MyReactionEmoji:  optStr(filters.MyReactionEmoji),
		Search:           optStr(filters.Search),
		CreatedAfter:     toolutil.ParseOptionalTime(filters.CreatedAfter),
		CreatedBefore:    toolutil.ParseOptionalTime(filters.CreatedBefore),
		UpdatedAfter:     toolutil.ParseOptionalTime(filters.UpdatedAfter),
		UpdatedBefore:    toolutil.ParseOptionalTime(filters.UpdatedBefore),
		Confidential:     filters.Confidential,
	}
}

// GetProject.

// GetProjectInput contains parameters for project issue statistics.
type GetProjectInput struct {
	ProjectID        string   `json:"project_id"                  jsonschema:"Project ID or URL-encoded path,required"`
	Labels           []string `json:"labels,omitempty"            jsonschema:"Label names to filter by"`
	Milestone        string   `json:"milestone,omitempty"         jsonschema:"Milestone title to filter by"`
	Scope            string   `json:"scope,omitempty"             jsonschema:"Scope: created_by_me, assigned_to_me, all"`
	Search           string   `json:"search,omitempty"            jsonschema:"Search string for title and description"`
	AssigneeID       *int64   `json:"assignee_id,omitempty"       jsonschema:"Filter by assignee user ID"`
	AssigneeUsername []string `json:"assignee_username,omitempty" jsonschema:"Filter by assignee usernames"`
	AuthorID         *int64   `json:"author_id,omitempty"         jsonschema:"Filter by author user ID"`
	AuthorUsername   string   `json:"author_username,omitempty"   jsonschema:"Filter by author username"`
	Confidential     *bool    `json:"confidential,omitempty"      jsonschema:"Filter by confidential status"`
	CreatedAfter     string   `json:"created_after,omitempty"     jsonschema:"Return issues created after date (ISO 8601)"`
	CreatedBefore    string   `json:"created_before,omitempty"    jsonschema:"Return issues created before date (ISO 8601)"`
	UpdatedAfter     string   `json:"updated_after,omitempty"     jsonschema:"Return issues updated after date (ISO 8601)"`
	UpdatedBefore    string   `json:"updated_before,omitempty"    jsonschema:"Return issues updated before date (ISO 8601)"`
	IIDs             []int64  `json:"iids,omitempty"              jsonschema:"Filter by issue internal IDs"`
	MyReactionEmoji  string   `json:"my_reaction_emoji,omitempty" jsonschema:"Filter by issues you reacted to with this emoji (or None/Any)"`
}

// GetProject retrieves issue statistics scoped to a single project via
// the GitLab Project Issue statistics API
// (GET /projects/:id/issues_statistics).
func GetProject(ctx context.Context, client *gitlabclient.Client, input GetProjectInput) (StatisticsOutput, error) {
	stats, _, err := client.GL().IssuesStatistics.GetProjectIssuesStatistics(input.ProjectID, projectIssueStatsOptions(statisticsFilters{
		Labels: input.Labels, Milestone: input.Milestone, Scope: input.Scope, Search: input.Search,
		AssigneeID: input.AssigneeID, AssigneeUsername: input.AssigneeUsername,
		AuthorID: input.AuthorID, AuthorUsername: input.AuthorUsername,
		Confidential: input.Confidential, CreatedAfter: input.CreatedAfter, CreatedBefore: input.CreatedBefore,
		UpdatedAfter: input.UpdatedAfter, UpdatedBefore: input.UpdatedBefore,
		IIDs: input.IIDs, MyReactionEmoji: input.MyReactionEmoji,
	}), gl.WithContext(ctx))
	if err != nil {
		return StatisticsOutput{}, toolutil.WrapErrWithStatusHint("gitlab_get_project_issue_statistics", err, http.StatusNotFound, "verify project_id with gitlab_project_get")
	}
	return fromGL(stats), nil
}

// projectIssueStatsOptions builds a [gl.GetProjectIssuesStatisticsOptions]
// from the shared [statisticsFilters] set.
func projectIssueStatsOptions(filters statisticsFilters) *gl.GetProjectIssuesStatisticsOptions {
	return &gl.GetProjectIssuesStatisticsOptions{
		IIDs:             optIIDs(filters.IIDs),
		Labels:           labelOptions(filters.Labels),
		Milestone:        optStr(filters.Milestone),
		Scope:            optStr(filters.Scope),
		AuthorID:         filters.AuthorID,
		AuthorUsername:   optStr(filters.AuthorUsername),
		AssigneeID:       filters.AssigneeID,
		AssigneeUsername: optStrings(filters.AssigneeUsername),
		MyReactionEmoji:  optStr(filters.MyReactionEmoji),
		Search:           optStr(filters.Search),
		CreatedAfter:     toolutil.ParseOptionalTime(filters.CreatedAfter),
		CreatedBefore:    toolutil.ParseOptionalTime(filters.CreatedBefore),
		UpdatedAfter:     toolutil.ParseOptionalTime(filters.UpdatedAfter),
		UpdatedBefore:    toolutil.ParseOptionalTime(filters.UpdatedBefore),
		Confidential:     filters.Confidential,
	}
}

// formatters.
