// Package groupanalytics implements MCP tools for GitLab group activity analytics,
// providing counts of recently created issues, merge requests, and new members.
package groupanalytics

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// hintVerifyGroupPathPremium is the 404 hint shared by group analytics tools.
const hintVerifyGroupPathPremium = "verify group_path \u2014 requires Premium license"

// IssuesCountInput holds parameters for retrieving recently created issues count.
type IssuesCountInput struct {
	GroupPath string `json:"group_path" jsonschema:"Full path of the group (e.g. my-group or parent/child),required"`
}

// MRCountInput holds parameters for retrieving recently created merge requests count.
type MRCountInput struct {
	GroupPath string `json:"group_path" jsonschema:"Full path of the group (e.g. my-group or parent/child),required"`
}

// MembersCountInput holds parameters for retrieving recently added members count.
type MembersCountInput struct {
	GroupPath string `json:"group_path" jsonschema:"Full path of the group (e.g. my-group or parent/child),required"`
}

// IssuesCountOutput represents the count of recently created issues in a group.
type IssuesCountOutput struct {
	toolutil.HintableOutput
	GroupPath   string `json:"group_path"`
	IssuesCount int64  `json:"issues_count"`
}

// MRCountOutput represents the count of recently created merge requests in a group.
type MRCountOutput struct {
	toolutil.HintableOutput
	GroupPath          string `json:"group_path"`
	MergeRequestsCount int64  `json:"merge_requests_count"`
}

// MembersCountOutput represents the count of recently added members in a group.
type MembersCountOutput struct {
	toolutil.HintableOutput
	GroupPath       string `json:"group_path"`
	NewMembersCount int64  `json:"new_members_count"`
}

// GetIssuesCount retrieves the count of recently created issues for a group.
func GetIssuesCount(ctx context.Context, client *gitlabclient.Client, in IssuesCountInput) (IssuesCountOutput, error) {
	if err := ctx.Err(); err != nil {
		return IssuesCountOutput{}, err
	}
	if in.GroupPath == "" {
		return IssuesCountOutput{}, toolutil.ErrFieldRequired("group_path")
	}

	opts := &gl.GetRecentlyCreatedIssuesCountOptions{
		GroupPath: in.GroupPath,
	}
	result, _, err := client.GL().GroupActivityAnalytics.GetRecentlyCreatedIssuesCount(opts, gl.WithContext(ctx))
	if err != nil {
		return IssuesCountOutput{}, toolutil.WrapErrWithStatusHint("get recently created issues count", err, http.StatusNotFound, hintVerifyGroupPathPremium)
	}

	return IssuesCountOutput{
		GroupPath:   in.GroupPath,
		IssuesCount: result.IssuesCount,
	}, nil
}

// GetMRCount retrieves the count of recently created merge requests for a group.
func GetMRCount(ctx context.Context, client *gitlabclient.Client, in MRCountInput) (MRCountOutput, error) {
	if err := ctx.Err(); err != nil {
		return MRCountOutput{}, err
	}
	if in.GroupPath == "" {
		return MRCountOutput{}, toolutil.ErrFieldRequired("group_path")
	}

	opts := &gl.GetRecentlyCreatedMergeRequestsCountOptions{
		GroupPath: in.GroupPath,
	}
	result, _, err := client.GL().GroupActivityAnalytics.GetRecentlyCreatedMergeRequestsCount(opts, gl.WithContext(ctx))
	if err != nil {
		return MRCountOutput{}, toolutil.WrapErrWithStatusHint("get recently created merge requests count", err, http.StatusNotFound, hintVerifyGroupPathPremium)
	}

	return MRCountOutput{
		GroupPath:          in.GroupPath,
		MergeRequestsCount: result.MergeRequestsCount,
	}, nil
}

// GetMembersCount retrieves the count of recently added members for a group.
func GetMembersCount(ctx context.Context, client *gitlabclient.Client, in MembersCountInput) (MembersCountOutput, error) {
	if err := ctx.Err(); err != nil {
		return MembersCountOutput{}, err
	}
	if in.GroupPath == "" {
		return MembersCountOutput{}, toolutil.ErrFieldRequired("group_path")
	}

	opts := &gl.GetRecentlyAddedMembersCountOptions{
		GroupPath: in.GroupPath,
	}
	result, _, err := client.GL().GroupActivityAnalytics.GetRecentlyAddedMembersCount(opts, gl.WithContext(ctx))
	if err != nil {
		return MembersCountOutput{}, toolutil.WrapErrWithStatusHint("get recently added members count", err, http.StatusNotFound, hintVerifyGroupPathPremium)
	}

	return MembersCountOutput{
		GroupPath:       in.GroupPath,
		NewMembersCount: result.NewMembersCount,
	}, nil
}
