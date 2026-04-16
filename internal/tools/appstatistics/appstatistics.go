// Package appstatistics implements MCP tools for GitLab Application Statistics API.
package appstatistics

import (
	"context"
	"encoding/json"

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

// flexibleStats mirrors ApplicationStatistics but accepts both string and number
// JSON values, working around certain GitLab versions returning string-encoded integers.
type flexibleStats struct {
	Forks         json.Number `json:"forks"`
	Issues        json.Number `json:"issues"`
	MergeRequests json.Number `json:"merge_requests"`
	Notes         json.Number `json:"notes"`
	Snippets      json.Number `json:"snippets"`
	SSHKeys       json.Number `json:"ssh_keys"`
	Milestones    json.Number `json:"milestones"`
	Users         json.Number `json:"users"`
	Groups        json.Number `json:"groups"`
	Projects      json.Number `json:"projects"`
	ActiveUsers   json.Number `json:"active_users"`
}

// Get retrieves current application statistics (admin).
// Uses a raw HTTP request to work around upstream client-go issue where
// ApplicationStatistics uses int64 fields but some GitLab versions return
// string-encoded numbers.
func Get(ctx context.Context, client *gitlabclient.Client, _ GetInput) (GetOutput, error) {
	req, err := client.GL().NewRequest("GET", "application/statistics", nil, nil)
	if err != nil {
		return GetOutput{}, toolutil.WrapErrWithMessage("get_application_statistics", err)
	}
	req = req.WithContext(ctx)

	var raw flexibleStats
	if _, err := client.GL().Do(req, &raw); err != nil {
		return GetOutput{}, toolutil.WrapErrWithMessage("get_application_statistics", err)
	}

	toInt := func(n json.Number) int64 {
		v, _ := n.Int64()
		return v
	}

	return GetOutput{
		Forks:         toInt(raw.Forks),
		Issues:        toInt(raw.Issues),
		MergeRequests: toInt(raw.MergeRequests),
		Notes:         toInt(raw.Notes),
		Snippets:      toInt(raw.Snippets),
		SSHKeys:       toInt(raw.SSHKeys),
		Milestones:    toInt(raw.Milestones),
		Users:         toInt(raw.Users),
		Groups:        toInt(raw.Groups),
		Projects:      toInt(raw.Projects),
		ActiveUsers:   toInt(raw.ActiveUsers),
	}, nil
}
