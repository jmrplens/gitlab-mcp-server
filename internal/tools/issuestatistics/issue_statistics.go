package issuestatistics

import (
	"context"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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
	Labels           []string             `json:"labels,omitempty"`
	Milestone        string               `json:"milestone,omitempty"`
	Scope            string               `json:"scope,omitempty"`
	Search           string               `json:"search,omitempty"`
	AssigneeID       toolutil.StringOrInt `json:"assignee_id,omitempty"`
	AssigneeUsername []string             `json:"assignee_username,omitempty"`
	AuthorID         *int64               `json:"author_id,omitempty"`
	AuthorUsername   string               `json:"author_username,omitempty"`
	Confidential     *bool                `json:"confidential,omitempty"`
	CreatedAfter     string               `json:"created_after,omitempty"`
	CreatedBefore    string               `json:"created_before,omitempty"`
	UpdatedAfter     string               `json:"updated_after,omitempty"`
	UpdatedBefore    string               `json:"updated_before,omitempty"`
	IIDs             []int64              `json:"iids[],omitempty"`
	In               string               `json:"in,omitempty"`
	MyReactionEmoji  string               `json:"my_reaction_emoji,omitempty"`
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

// GetInput contains parameters for global issue statistics, which GitLab
// counts over the issues the caller created unless Scope names another scope.
type GetInput struct {
	Labels           []string             `json:"labels,omitempty"            jsonschema:"Label names to filter by"`
	Milestone        string               `json:"milestone,omitempty"         jsonschema:"Milestone title to filter by"`
	Scope            string               `json:"scope,omitempty"             jsonschema:"Which issues to count: created_by_me (the caller's own, and GitLab's default when scope is omitted), assigned_to_me, or all (every issue the caller can see)"`
	Search           string               `json:"search,omitempty"            jsonschema:"Search string for title and description"`
	In               string               `json:"in,omitempty"                jsonschema:"Fields the search query applies to (title, description, or title,description)"`
	AssigneeID       toolutil.StringOrInt `json:"assignee_id,omitempty"       jsonschema:"Filter by assignee: a user ID, None for issues nobody is assigned, or Any for issues somebody is"`
	AssigneeUsername []string             `json:"assignee_username,omitempty" jsonschema:"Filter by assignee usernames"`
	AuthorID         *int64               `json:"author_id,omitempty"         jsonschema:"Filter by author user ID"`
	AuthorUsername   string               `json:"author_username,omitempty"   jsonschema:"Filter by author username"`
	Confidential     *bool                `json:"confidential,omitempty"      jsonschema:"Filter by confidential status"`
	CreatedAfter     string               `json:"created_after,omitempty"     jsonschema:"Return issues created after date (ISO 8601, e.g. 2025-01-01T00:00:00Z)"`
	CreatedBefore    string               `json:"created_before,omitempty"    jsonschema:"Return issues created before date (ISO 8601, e.g. 2025-12-31T23:59:59Z)"`
	UpdatedAfter     string               `json:"updated_after,omitempty"     jsonschema:"Return issues updated after date (ISO 8601, e.g. 2025-01-01T00:00:00Z)"`
	UpdatedBefore    string               `json:"updated_before,omitempty"    jsonschema:"Return issues updated before date (ISO 8601, e.g. 2025-12-31T23:59:59Z)"`
	IIDs             []int64              `json:"iids,omitempty"              jsonschema:"Filter by issue internal IDs"`
	MyReactionEmoji  string               `json:"my_reaction_emoji,omitempty" jsonschema:"Filter by issues you reacted to with this emoji (or None/Any)"`
}

// Get retrieves global issue statistics via the GitLab Issue statistics API
// (GET /issues_statistics). The scope is passed through as the caller wrote
// it, so an omitted one gets GitLab's default, created_by_me, and the counts
// cover only the issues the caller created; scope=all counts every issue it
// can see. Optional filters narrow the result.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (StatisticsOutput, error) {
	opts := &gl.GetIssuesStatisticsOptions{
		Labels:           labelOptions(input.Labels),
		Milestone:        optStr(input.Milestone),
		Scope:            optStr(input.Scope),
		Search:           optStr(input.Search),
		In:               optStr(input.In),
		AuthorID:         input.AuthorID,
		AuthorUsername:   optStr(input.AuthorUsername),
		AssigneeUsername: optStrings(input.AssigneeUsername),
		MyReactionEmoji:  optStr(input.MyReactionEmoji),
		IIDs:             optIIDs(input.IIDs),
		Confidential:     input.Confidential,
	}
	assigneeQuery, err := parseStatisticsFilters(statisticsFilters{
		AssigneeID:   input.AssigneeID,
		CreatedAfter: input.CreatedAfter, CreatedBefore: input.CreatedBefore,
		UpdatedAfter: input.UpdatedAfter, UpdatedBefore: input.UpdatedBefore,
	}, statisticsDests{
		assignee:     &opts.AssigneeID,
		createdAfter: &opts.CreatedAfter, createdBefore: &opts.CreatedBefore,
		updatedAfter: &opts.UpdatedAfter, updatedBefore: &opts.UpdatedBefore,
	})
	if err != nil {
		return StatisticsOutput{}, err
	}
	stats, _, err := client.GL().IssuesStatistics.GetIssuesStatistics(opts, gl.WithContext(ctx), assigneeQuery)
	if err != nil {
		return StatisticsOutput{}, toolutil.WrapErrWithStatusHint("gitlab_get_issue_statistics", err, http.StatusForbidden, "verify your token has read_api scope")
	}
	return fromGL(stats), nil
}

// GetGroup.

// GetGroupInput contains parameters for group issue statistics.
type GetGroupInput struct {
	GroupID          string               `json:"group_id"                    jsonschema:"Group ID or URL-encoded path,required"`
	Labels           []string             `json:"labels,omitempty"            jsonschema:"Label names to filter by"`
	Milestone        string               `json:"milestone,omitempty"         jsonschema:"Milestone title to filter by"`
	Scope            string               `json:"scope,omitempty"             jsonschema:"Scope: created_by_me, assigned_to_me, all"`
	Search           string               `json:"search,omitempty"            jsonschema:"Search string for title and description"`
	AssigneeID       toolutil.StringOrInt `json:"assignee_id,omitempty"       jsonschema:"Filter by assignee: a user ID, None for issues nobody is assigned, or Any for issues somebody is"`
	AssigneeUsername []string             `json:"assignee_username,omitempty" jsonschema:"Filter by assignee usernames"`
	AuthorID         *int64               `json:"author_id,omitempty"         jsonschema:"Filter by author user ID"`
	AuthorUsername   string               `json:"author_username,omitempty"   jsonschema:"Filter by author username"`
	Confidential     *bool                `json:"confidential,omitempty"      jsonschema:"Filter by confidential status"`
	CreatedAfter     string               `json:"created_after,omitempty"     jsonschema:"Return issues created after date (ISO 8601)"`
	CreatedBefore    string               `json:"created_before,omitempty"    jsonschema:"Return issues created before date (ISO 8601)"`
	UpdatedAfter     string               `json:"updated_after,omitempty"     jsonschema:"Return issues updated after date (ISO 8601)"`
	UpdatedBefore    string               `json:"updated_before,omitempty"    jsonschema:"Return issues updated before date (ISO 8601)"`
	IIDs             []int64              `json:"iids,omitempty"              jsonschema:"Filter by issue internal IDs"`
	MyReactionEmoji  string               `json:"my_reaction_emoji,omitempty" jsonschema:"Filter by issues you reacted to with this emoji (or None/Any)"`
}

// GetGroup retrieves issue statistics scoped to a group and its
// descendant projects via the GitLab Group Issue statistics API
// (GET /groups/:id/issues_statistics).
func GetGroup(ctx context.Context, client *gitlabclient.Client, input GetGroupInput) (StatisticsOutput, error) {
	//gitlab:request mandatory: scopedStatistics sends this read on every call whose filters parse
	return scopedStatistics(ctx, scopedStatisticsRead[*gl.GetGroupIssuesStatisticsOptions]{
		id: input.GroupID,
		filters: statisticsFilters{
			Labels: input.Labels, Milestone: input.Milestone, Scope: input.Scope, Search: input.Search,
			AssigneeID: input.AssigneeID, AssigneeUsername: input.AssigneeUsername,
			AuthorID: input.AuthorID, AuthorUsername: input.AuthorUsername,
			Confidential: input.Confidential, CreatedAfter: input.CreatedAfter, CreatedBefore: input.CreatedBefore,
			UpdatedAfter: input.UpdatedAfter, UpdatedBefore: input.UpdatedBefore,
			IIDs: input.IIDs, MyReactionEmoji: input.MyReactionEmoji,
		},
		build:     groupIssueStatsOptions,
		get:       client.GL().IssuesStatistics.GetGroupIssuesStatistics,
		operation: "gitlab_get_group_issue_statistics",
		hint:      "verify group_id with group.get",
	})
}

// scopedStatisticsRead is one statistics read scoped to a group or a project:
// the scope's ID, the filters the caller set, how they become the route's
// options, and the client-go method that reads it.
type scopedStatisticsRead[O any] struct {
	id        string
	filters   statisticsFilters
	build     func(statisticsFilters) (O, gl.RequestOptionFunc, error)
	get       func(any, O, ...gl.RequestOptionFunc) (*gl.IssuesStatistics, *gl.Response, error)
	operation string
	hint      string
}

// scopedStatistics builds the options of read, refusing a filter that does not
// parse before any request, and reads the statistics, answering a 404 with the
// read's hint. The request option build returns beside the options carries
// what they cannot, the None and Any of assignee_id.
func scopedStatistics[O any](ctx context.Context, read scopedStatisticsRead[O]) (StatisticsOutput, error) {
	opts, query, err := read.build(read.filters)
	if err != nil {
		return StatisticsOutput{}, err
	}
	stats, _, err := read.get(read.id, opts, gl.WithContext(ctx), query)
	if err != nil {
		return StatisticsOutput{}, toolutil.WrapErrWithStatusHint(read.operation, err, http.StatusNotFound, read.hint)
	}
	return fromGL(stats), nil
}

// statisticsDests names the option fields of one statistics read that
// [parseStatisticsFilters] fills.
type statisticsDests struct {
	assignee      **int64
	createdAfter  **time.Time
	createdBefore **time.Time
	updatedAfter  **time.Time
	updatedBefore **time.Time
}

// parseStatisticsFilters reads the filters of a statistics read that have to
// be parsed into dest, refusing the call on the first that does not parse:
// the assignee, a user ID or None or Any, then the four dates. client-go types
// the assignee *int64 on all three routes (upstream register row 125), so an
// ID goes into the option and None or Any come back as the request option
// that writes them into the query (issue 1354).
func parseStatisticsFilters(filters statisticsFilters, dest statisticsDests) (gl.RequestOptionFunc, error) {
	assignee, assigneeQuery, err := toolutil.Int64UserIDFilter("assignee_id", filters.AssigneeID)
	if err != nil {
		return nil, err
	}
	*dest.assignee = assignee
	dateErr := parseStatisticsDates(filters, dest.createdAfter, dest.createdBefore, dest.updatedAfter, dest.updatedBefore)
	if dateErr != nil {
		return nil, dateErr
	}
	return assigneeQuery, nil
}

// parseStatisticsDates parses the created and updated bounds of filters, which
// every statistics route shares, into their option fields, refusing the call
// on the first value that does not parse. The parser before it answered nil
// for such a value and the counts covered every issue the range was meant to
// leave out (issue 1308).
func parseStatisticsDates(filters statisticsFilters, createdAfter, createdBefore, updatedAfter, updatedBefore **time.Time) error {
	return toolutil.ParseTimeInputs(
		toolutil.TimeInput{Field: "created_after", Value: filters.CreatedAfter, Dest: createdAfter},
		toolutil.TimeInput{Field: "created_before", Value: filters.CreatedBefore, Dest: createdBefore},
		toolutil.TimeInput{Field: "updated_after", Value: filters.UpdatedAfter, Dest: updatedAfter},
		toolutil.TimeInput{Field: "updated_before", Value: filters.UpdatedBefore, Dest: updatedBefore},
	)
}

// groupIssueStatsOptions builds a [gl.GetGroupIssuesStatisticsOptions]
// from the shared [statisticsFilters] set, with the request option that
// carries an assignee_id of None or Any.
func groupIssueStatsOptions(filters statisticsFilters) (*gl.GetGroupIssuesStatisticsOptions, gl.RequestOptionFunc, error) {
	opts := &gl.GetGroupIssuesStatisticsOptions{
		Labels:           labelOptions(filters.Labels),
		IIDs:             optIIDs(filters.IIDs),
		Milestone:        optStr(filters.Milestone),
		Scope:            optStr(filters.Scope),
		AuthorID:         filters.AuthorID,
		AuthorUsername:   optStr(filters.AuthorUsername),
		AssigneeUsername: optStrings(filters.AssigneeUsername),
		MyReactionEmoji:  optStr(filters.MyReactionEmoji),
		Search:           optStr(filters.Search),
		Confidential:     filters.Confidential,
	}
	assigneeQuery, err := parseStatisticsFilters(filters, statisticsDests{
		assignee:     &opts.AssigneeID,
		createdAfter: &opts.CreatedAfter, createdBefore: &opts.CreatedBefore,
		updatedAfter: &opts.UpdatedAfter, updatedBefore: &opts.UpdatedBefore,
	})
	if err != nil {
		return nil, nil, err
	}
	return opts, assigneeQuery, nil
}

// GetProject.

// GetProjectInput contains parameters for project issue statistics.
type GetProjectInput struct {
	ProjectID        string               `json:"project_id"                  jsonschema:"Project ID or URL-encoded path,required"`
	Labels           []string             `json:"labels,omitempty"            jsonschema:"Label names to filter by"`
	Milestone        string               `json:"milestone,omitempty"         jsonschema:"Milestone title to filter by"`
	Scope            string               `json:"scope,omitempty"             jsonschema:"Scope: created_by_me, assigned_to_me, all"`
	Search           string               `json:"search,omitempty"            jsonschema:"Search string for title and description"`
	AssigneeID       toolutil.StringOrInt `json:"assignee_id,omitempty"       jsonschema:"Filter by assignee: a user ID, None for issues nobody is assigned, or Any for issues somebody is"`
	AssigneeUsername []string             `json:"assignee_username,omitempty" jsonschema:"Filter by assignee usernames"`
	AuthorID         *int64               `json:"author_id,omitempty"         jsonschema:"Filter by author user ID"`
	AuthorUsername   string               `json:"author_username,omitempty"   jsonschema:"Filter by author username"`
	Confidential     *bool                `json:"confidential,omitempty"      jsonschema:"Filter by confidential status"`
	CreatedAfter     string               `json:"created_after,omitempty"     jsonschema:"Return issues created after date (ISO 8601)"`
	CreatedBefore    string               `json:"created_before,omitempty"    jsonschema:"Return issues created before date (ISO 8601)"`
	UpdatedAfter     string               `json:"updated_after,omitempty"     jsonschema:"Return issues updated after date (ISO 8601)"`
	UpdatedBefore    string               `json:"updated_before,omitempty"    jsonschema:"Return issues updated before date (ISO 8601)"`
	IIDs             []int64              `json:"iids,omitempty"              jsonschema:"Filter by issue internal IDs"`
	MyReactionEmoji  string               `json:"my_reaction_emoji,omitempty" jsonschema:"Filter by issues you reacted to with this emoji (or None/Any)"`
}

// GetProject retrieves issue statistics scoped to a single project via
// the GitLab Project Issue statistics API
// (GET /projects/:id/issues_statistics).
func GetProject(ctx context.Context, client *gitlabclient.Client, input GetProjectInput) (StatisticsOutput, error) {
	//gitlab:request mandatory: scopedStatistics sends this read on every call whose filters parse
	return scopedStatistics(ctx, scopedStatisticsRead[*gl.GetProjectIssuesStatisticsOptions]{
		id: input.ProjectID,
		filters: statisticsFilters{
			Labels: input.Labels, Milestone: input.Milestone, Scope: input.Scope, Search: input.Search,
			AssigneeID: input.AssigneeID, AssigneeUsername: input.AssigneeUsername,
			AuthorID: input.AuthorID, AuthorUsername: input.AuthorUsername,
			Confidential: input.Confidential, CreatedAfter: input.CreatedAfter, CreatedBefore: input.CreatedBefore,
			UpdatedAfter: input.UpdatedAfter, UpdatedBefore: input.UpdatedBefore,
			IIDs: input.IIDs, MyReactionEmoji: input.MyReactionEmoji,
		},
		build:     projectIssueStatsOptions,
		get:       client.GL().IssuesStatistics.GetProjectIssuesStatistics,
		operation: "gitlab_get_project_issue_statistics",
		hint:      "verify project_id with project.get",
	})
}

// projectIssueStatsOptions builds a [gl.GetProjectIssuesStatisticsOptions]
// from the shared [statisticsFilters] set, with the request option that
// carries an assignee_id of None or Any.
func projectIssueStatsOptions(filters statisticsFilters) (*gl.GetProjectIssuesStatisticsOptions, gl.RequestOptionFunc, error) {
	opts := &gl.GetProjectIssuesStatisticsOptions{
		IIDs:             optIIDs(filters.IIDs),
		Labels:           labelOptions(filters.Labels),
		Milestone:        optStr(filters.Milestone),
		Scope:            optStr(filters.Scope),
		AuthorID:         filters.AuthorID,
		AuthorUsername:   optStr(filters.AuthorUsername),
		AssigneeUsername: optStrings(filters.AssigneeUsername),
		MyReactionEmoji:  optStr(filters.MyReactionEmoji),
		Search:           optStr(filters.Search),
		Confidential:     filters.Confidential,
	}
	assigneeQuery, err := parseStatisticsFilters(filters, statisticsDests{
		assignee:     &opts.AssigneeID,
		createdAfter: &opts.CreatedAfter, createdBefore: &opts.CreatedBefore,
		updatedAfter: &opts.UpdatedAfter, updatedBefore: &opts.UpdatedBefore,
	})
	if err != nil {
		return nil, nil, err
	}
	return opts, assigneeQuery, nil
}

// formatters.
