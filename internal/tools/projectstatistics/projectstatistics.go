// Package projectstatistics implements MCP tools for GitLab project statistics.
package projectstatistics

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// GetInput contains parameters for getting project statistics.
type GetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// DayStat represents a single day's fetch count.
type DayStat struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

// GetOutput contains project statistics (last 30 days fetch data).
type GetOutput struct {
	toolutil.HintableOutput
	TotalFetches int64     `json:"total_fetches"`
	Days         []DayStat `json:"days"`
}

// Get retrieves the last 30 days of project statistics.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (GetOutput, error) {
	stats, _, err := client.GL().ProjectStatistics.Last30DaysStatistics(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return GetOutput{}, toolutil.WrapErrWithStatusHint("gitlab_get_project_statistics", err, http.StatusNotFound, "verify project_id with gitlab_project_get")
	}
	days := make([]DayStat, 0, len(stats.Fetches.Days))
	for _, d := range stats.Fetches.Days {
		days = append(days, DayStat{Date: d.Date, Count: d.Count})
	}
	return GetOutput{TotalFetches: stats.Fetches.Total, Days: days}, nil
}
