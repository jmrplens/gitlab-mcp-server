// Package appstatistics implements MCP tools for GitLab Application Statistics API.
package appstatistics

import (
	"context"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// GetInput is the input (no params).
type GetInput struct{}

// GetOutput is the output for application statistics.
type GetOutput struct {
	toolutil.HintableOutput
	Forks         int64 `json:"forks"`
	Issues        int64 `json:"issues"`
	MergeRequests int64 `json:"merge_requests"`
	Notes         int64 `json:"notes"`
	Snippets      int64 `json:"snippets"`
	SSHKeys       int64 `json:"ssh_keys"`
	Milestones    int64 `json:"milestones"`
	Users         int64 `json:"users"`
	Groups        int64 `json:"groups"`
	Projects      int64 `json:"projects"`
	ActiveUsers   int64 `json:"active_users"`
}

// Get retrieves current application statistics (admin).
func Get(ctx context.Context, client *gitlabclient.Client, _ GetInput) (GetOutput, error) {
	stats, _, err := client.GL().ApplicationStatistics.GetApplicationStatistics(gl.WithContext(ctx))
	if err != nil {
		return GetOutput{}, toolutil.WrapErrWithMessage("get_application_statistics", err)
	}
	return GetOutput{
		Forks:         stats.Forks,
		Issues:        stats.Issues,
		MergeRequests: stats.MergeRequests,
		Notes:         stats.Notes,
		Snippets:      stats.Snippets,
		SSHKeys:       stats.SSHKeys,
		Milestones:    stats.Milestones,
		Users:         stats.Users,
		Groups:        stats.Groups,
		Projects:      stats.Projects,
		ActiveUsers:   stats.ActiveUsers,
	}, nil
}
