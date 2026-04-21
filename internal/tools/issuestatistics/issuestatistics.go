// Package issuestatistics implements MCP tools for GitLab issue statistics operations.
package issuestatistics

import (
	"context"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// Shared output.

// StatisticsOutput contains issue statistics counts.
type StatisticsOutput struct {
	toolutil.HintableOutput
	All    int64 `json:"all"`
	Closed int64 `json:"closed"`
	Opened int64 `json:"opened"`
}

// fromGL is an internal helper for the issuestatistics package.
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

// Get retrieves global issue statistics.
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
		return StatisticsOutput{}, toolutil.WrapErrWithMessage("gitlab_get_issue_statistics", err)
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

// GetGroup retrieves issue statistics for a group.
func GetGroup(ctx context.Context, client *gitlabclient.Client, input GetGroupInput) (StatisticsOutput, error) {
	opts := &gl.GetGroupIssuesStatisticsOptions{}
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
	stats, _, err := client.GL().IssuesStatistics.GetGroupIssuesStatistics(input.GroupID, opts, gl.WithContext(ctx))
	if err != nil {
		return StatisticsOutput{}, toolutil.WrapErrWithMessage("gitlab_get_group_issue_statistics", err)
	}
	return fromGL(stats), nil
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

// GetProject retrieves issue statistics for a project.
func GetProject(ctx context.Context, client *gitlabclient.Client, input GetProjectInput) (StatisticsOutput, error) {
	opts := &gl.GetProjectIssuesStatisticsOptions{}
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
	stats, _, err := client.GL().IssuesStatistics.GetProjectIssuesStatistics(input.ProjectID, opts, gl.WithContext(ctx))
	if err != nil {
		return StatisticsOutput{}, toolutil.WrapErrWithMessage("gitlab_get_project_issue_statistics", err)
	}
	return fromGL(stats), nil
}

// formatters.
