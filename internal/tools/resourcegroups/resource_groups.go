package resourcegroups

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// ListAll.

// ListInput defines parameters for the list operation, and the page of the
// project's resource groups to list. client-go's
// GetAllResourceGroupsForAProject takes no options struct, so the page travels
// as a request option.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	toolutil.PaginationInput
}

// ResourceGroupItem is a CI resource group as GitLab renders it
// (Entities::Ci::ResourceGroup): its key, the process mode that serializes
// the jobs sharing it, and when it was created and last changed.
type ResourceGroupItem struct {
	toolutil.HintableOutput
	ID          int64  `json:"id"`
	Key         string `json:"key"`
	ProcessMode string `json:"process_mode"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

// toResourceGroupItem converts a resource group the three resource group
// endpoints answer with.
func toResourceGroupItem(g *gl.ResourceGroup) ResourceGroupItem {
	return ResourceGroupItem{
		ID:          g.ID,
		Key:         g.Key,
		ProcessMode: g.ProcessMode,
		CreatedAt:   toolutil.RFC3339Ptr(g.CreatedAt),
		UpdatedAt:   toolutil.RFC3339Ptr(g.UpdatedAt),
	}
}

// ListOutput represents one page of a project's resource groups and the
// pagination GitLab sent with it.
type ListOutput struct {
	toolutil.HintableOutput
	Groups     []ResourceGroupItem       `json:"groups"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ListAll lists one page of a project's resource groups.
func ListAll(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	groups, resp, err := client.GL().ResourceGroup.GetAllResourceGroupsForAProject(string(input.ProjectID), gl.WithContext(ctx), toolutil.PaginationRequestOption(input.PaginationInput))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("gitlab_list_resource_groups", err, http.StatusNotFound, "verify project_id with project.get")
	}
	items := make([]ResourceGroupItem, 0, len(groups))
	for _, g := range groups {
		items = append(items, toResourceGroupItem(g))
	}
	return ListOutput{Groups: items, Pagination: toolutil.PaginationFromResponse(resp)}, nil
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
		return ResourceGroupItem{}, toolutil.WrapErrWithStatusHint("gitlab_get_resource_group", err, http.StatusNotFound, "verify the resource group key with pipeline.resource_group_list")
	}
	return toResourceGroupItem(g), nil
}

// Edit.

// EditInput defines parameters for the edit operation.
type EditInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Key         string               `json:"key" jsonschema:"Resource group key,required"`
	ProcessMode string               `json:"process_mode" jsonschema:"Process mode: unordered, oldest_first, newest_first, or newest_ready_first,required"`
}

// Edit edits resources for the resourcegroups package.
func Edit(ctx context.Context, client *gitlabclient.Client, input EditInput) (ResourceGroupItem, error) {
	mode := gl.ResourceGroupProcessMode(input.ProcessMode)
	opts := &gl.EditAnExistingResourceGroupOptions{ProcessMode: &mode}
	g, _, err := client.GL().ResourceGroup.EditAnExistingResourceGroup(string(input.ProjectID), input.Key, opts, gl.WithContext(ctx))
	if err != nil {
		return ResourceGroupItem{}, toolutil.WrapErrWithStatusHint("gitlab_edit_resource_group", err, http.StatusNotFound, "verify the resource group key with pipeline.resource_group_list; valid process_mode values: unordered, oldest_first, newest_first, newest_ready_first")
	}
	return toResourceGroupItem(g), nil
}

// ListUpcomingJobs.

// ListUpcomingJobsInput defines parameters for the list upcoming jobs
// operation, and the page of those jobs to list. client-go's
// ListUpcomingJobsForASpecificResourceGroup takes no options struct, so the
// page travels as a request option.
type ListUpcomingJobsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Key       string               `json:"key" jsonschema:"Resource group key,required"`
	toolutil.PaginationInput
}

// JobItem is one job waiting on a resource group, as a compact row: what the
// job is, where it comes from and whether its failure would block the
// pipeline. GitLab answers the queue with Ci::JobBasic, whose other keys are
// job.get's: the commit, the project and the user, and the fields of a run
// (its start and finish, duration, coverage and failure reason), which a job
// that has not run yet leaves empty.
type JobItem struct {
	ID           int64            `json:"id"`
	Name         string           `json:"name"`
	Status       string           `json:"status"`
	Stage        string           `json:"stage"`
	Ref          string           `json:"ref,omitempty"`
	Tag          bool             `json:"tag,omitempty"`
	AllowFailure bool             `json:"allow_failure,omitempty"`
	Pipeline     *JobPipelineItem `json:"pipeline,omitempty"`
	WebURL       string           `json:"web_url,omitempty"`
	CreatedAt    string           `json:"created_at,omitempty"`
}

// JobPipelineItem is the pipeline a waiting job belongs to, as the job
// renders it (Entities::Ci::PipelineBasic): the pipeline's ID, number and
// project, the ref and commit it runs for, its status and what started it,
// when it was created and last changed, and its page. The first five are
// what client-go's JobPipeline models; the rest are read from the captured
// response (see [upcomingJobExtra]).
type JobPipelineItem struct {
	ID        int64  `json:"id"`
	IID       int64  `json:"iid,omitempty"`
	ProjectID int64  `json:"project_id"`
	Ref       string `json:"ref"`
	SHA       string `json:"sha"`
	Status    string `json:"status"`
	Source    string `json:"source,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
	WebURL    string `json:"web_url,omitempty"`
}

// ListUpcomingJobsOutput represents one page of the jobs waiting on a
// resource group and the pagination GitLab sent with it.
type ListUpcomingJobsOutput struct {
	toolutil.HintableOutput
	Jobs       []JobItem                 `json:"jobs"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ListUpcomingJobs lists one page of the jobs waiting on a resource group.
func ListUpcomingJobs(ctx context.Context, client *gitlabclient.Client, input ListUpcomingJobsInput) (ListUpcomingJobsOutput, error) {
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	jobs, resp, err := client.GL().ResourceGroup.ListUpcomingJobsForASpecificResourceGroup(string(input.ProjectID), input.Key, gl.WithContext(ctx), toolutil.PaginationRequestOption(input.PaginationInput))
	if err != nil {
		return ListUpcomingJobsOutput{}, toolutil.WrapErrWithStatusHint("gitlab_list_resource_group_upcoming_jobs", err, http.StatusNotFound, "verify the resource group key with pipeline.resource_group_list")
	}
	extras, err := capturedUpcomingJobs(captured, len(jobs))
	if err != nil {
		return ListUpcomingJobsOutput{}, toolutil.WrapErr("gitlab_list_resource_group_upcoming_jobs", err)
	}
	items := make([]JobItem, 0, len(jobs))
	for i, j := range jobs {
		item := JobItem{
			ID:           j.ID,
			Name:         j.Name,
			Status:       j.Status,
			Stage:        j.Stage,
			Ref:          j.Ref,
			Tag:          j.Tag,
			AllowFailure: j.AllowFailure,
			WebURL:       j.WebURL,
			CreatedAt:    toolutil.RFC3339Ptr(j.CreatedAt),
		}
		// A job always belongs to a pipeline, so an empty one is a pipeline
		// GitLab did not render rather than one with ID zero.
		if j.Pipeline.ID != 0 {
			extra := extras[i].Pipeline
			item.Pipeline = &JobPipelineItem{
				ID:        j.Pipeline.ID,
				IID:       extra.IID,
				ProjectID: j.Pipeline.ProjectID,
				Ref:       j.Pipeline.Ref,
				SHA:       j.Pipeline.Sha,
				Status:    j.Pipeline.Status,
				Source:    extra.Source,
				CreatedAt: toolutil.RFC3339Ptr(extra.CreatedAt),
				UpdatedAt: toolutil.RFC3339Ptr(extra.UpdatedAt),
				WebURL:    extra.WebURL,
			}
		}
		items = append(items, item)
	}
	return ListUpcomingJobsOutput{Jobs: items, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// formatters.
