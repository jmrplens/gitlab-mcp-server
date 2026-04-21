// Package resourcegroups implements MCP tools for GitLab resource groups.
package resourcegroups

import (
	"context"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ListAll.

// ListInput defines parameters for the list operation.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// ResourceGroupItem holds data for resourcegroups operations.
type ResourceGroupItem struct {
	toolutil.HintableOutput
	ID          int64  `json:"id"`
	Key         string `json:"key"`
	ProcessMode string `json:"process_mode"`
}

// ListOutput represents the response from the list operation.
type ListOutput struct {
	toolutil.HintableOutput
	Groups []ResourceGroupItem `json:"groups"`
}

// ListAll lists all for the resourcegroups package.
func ListAll(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	groups, _, err := client.GL().ResourceGroup.GetAllResourceGroupsForAProject(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("gitlab_list_resource_groups", err)
	}
	items := make([]ResourceGroupItem, 0, len(groups))
	for _, g := range groups {
		items = append(items, ResourceGroupItem{ID: g.ID, Key: g.Key, ProcessMode: g.ProcessMode})
	}
	return ListOutput{Groups: items}, nil
}

// Get.

// GetInput defines parameters for the get operation.
type GetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Key       string               `json:"key" jsonschema:"Resource group key,required"`
}

// Get retrieves resources for the resourcegroups package.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (ResourceGroupItem, error) {
	g, _, err := client.GL().ResourceGroup.GetASpecificResourceGroup(string(input.ProjectID), input.Key, gl.WithContext(ctx))
	if err != nil {
		return ResourceGroupItem{}, toolutil.WrapErrWithMessage("gitlab_get_resource_group", err)
	}
	return ResourceGroupItem{ID: g.ID, Key: g.Key, ProcessMode: g.ProcessMode}, nil
}

// Edit.

// EditInput defines parameters for the edit operation.
type EditInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Key         string               `json:"key" jsonschema:"Resource group key,required"`
	ProcessMode string               `json:"process_mode" jsonschema:"Process mode (newest_first, oldest_first, unordered),required"`
}

// Edit edits resources for the resourcegroups package.
func Edit(ctx context.Context, client *gitlabclient.Client, input EditInput) (ResourceGroupItem, error) {
	mode := gl.ResourceGroupProcessMode(input.ProcessMode)
	opts := &gl.EditAnExistingResourceGroupOptions{ProcessMode: &mode}
	g, _, err := client.GL().ResourceGroup.EditAnExistingResourceGroup(string(input.ProjectID), input.Key, opts, gl.WithContext(ctx))
	if err != nil {
		return ResourceGroupItem{}, toolutil.WrapErrWithMessage("gitlab_edit_resource_group", err)
	}
	return ResourceGroupItem{ID: g.ID, Key: g.Key, ProcessMode: g.ProcessMode}, nil
}

// ListUpcomingJobs.

// ListUpcomingJobsInput defines parameters for the list upcoming jobs operation.
type ListUpcomingJobsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Key       string               `json:"key" jsonschema:"Resource group key,required"`
}

// JobItem holds data for resourcegroups operations.
type JobItem struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Stage  string `json:"stage"`
}

// ListUpcomingJobsOutput represents the response from the list upcoming jobs operation.
type ListUpcomingJobsOutput struct {
	toolutil.HintableOutput
	Jobs []JobItem `json:"jobs"`
}

// ListUpcomingJobs lists upcoming jobs for the resourcegroups package.
func ListUpcomingJobs(ctx context.Context, client *gitlabclient.Client, input ListUpcomingJobsInput) (ListUpcomingJobsOutput, error) {
	jobs, _, err := client.GL().ResourceGroup.ListUpcomingJobsForASpecificResourceGroup(string(input.ProjectID), input.Key, gl.WithContext(ctx))
	if err != nil {
		return ListUpcomingJobsOutput{}, toolutil.WrapErrWithMessage("gitlab_list_resource_group_upcoming_jobs", err)
	}
	items := make([]JobItem, 0, len(jobs))
	for _, j := range jobs {
		items = append(items, JobItem{ID: j.ID, Name: j.Name, Status: j.Status, Stage: j.Stage})
	}
	return ListUpcomingJobsOutput{Jobs: items}, nil
}

// formatters.
