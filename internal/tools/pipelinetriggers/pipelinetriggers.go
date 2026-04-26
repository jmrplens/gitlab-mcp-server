// Package pipelinetriggers provides MCP tool handlers for GitLab pipeline trigger operations.
package pipelinetriggers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ──────────────────────────────────────────────
// Output types
// ──────────────────────────────────────────────.

// Output represents a single pipeline trigger token.
type Output struct {
	toolutil.HintableOutput
	ID          int64  `json:"id"`
	Description string `json:"description"`
	Token       string `json:"token"`
	OwnerName   string `json:"owner_name,omitempty"`
	OwnerID     int64  `json:"owner_id,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
	LastUsed    string `json:"last_used,omitempty"`
}

// ListOutput represents a paginated list of pipeline triggers.
type ListOutput struct {
	toolutil.HintableOutput
	Triggers   []Output                  `json:"triggers"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// RunOutput represents the result of triggering a pipeline.
type RunOutput struct {
	toolutil.HintableOutput
	PipelineID int64  `json:"pipeline_id"`
	SHA        string `json:"sha"`
	Ref        string `json:"ref"`
	Status     string `json:"status"`
	WebURL     string `json:"web_url"`
	CreatedAt  string `json:"created_at,omitempty"`
}

// ──────────────────────────────────────────────
// Input types
// ──────────────────────────────────────────────.

// ListInput contains parameters for listing pipeline triggers.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	toolutil.PaginationInput
}

// GetInput contains parameters for getting a pipeline trigger.
type GetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	TriggerID int64                `json:"trigger_id" jsonschema:"Pipeline trigger ID,required"`
}

// CreateInput contains parameters for creating a pipeline trigger.
type CreateInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	Description string               `json:"description" jsonschema:"Trigger token description"`
}

// UpdateInput contains parameters for updating a pipeline trigger.
type UpdateInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	TriggerID   int64                `json:"trigger_id" jsonschema:"Pipeline trigger ID,required"`
	Description string               `json:"description,omitempty" jsonschema:"New trigger token description"`
}

// DeleteInput contains parameters for deleting a pipeline trigger.
type DeleteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	TriggerID int64                `json:"trigger_id" jsonschema:"Pipeline trigger ID,required"`
}

// RunInput contains parameters for triggering a pipeline.
type RunInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	Ref       string               `json:"ref" jsonschema:"Branch or tag name to run pipeline on"`
	Token     string               `json:"token" jsonschema:"Pipeline trigger token"`
	Variables string               `json:"variables,omitempty" jsonschema:"JSON object of key-value variable pairs"`
}

// ──────────────────────────────────────────────
// Handlers
// ──────────────────────────────────────────────.

// ListTriggers lists pipeline triggers for a project.
func ListTriggers(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.WrapErrWithMessage("pipeline_trigger_list", toolutil.ErrFieldRequired("project_id"))
	}
	opts := &gl.ListPipelineTriggersOptions{
		ListOptions: gl.ListOptions{
			Page:    int64(input.Page),
			PerPage: int64(input.PerPage),
		},
	}
	triggers, resp, err := client.GL().PipelineTriggers.ListPipelineTriggers(
		string(input.ProjectID), opts, gl.WithContext(ctx),
	)
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("pipeline_trigger_list", err, http.StatusNotFound,
			"verify the project exists with gitlab_project_get and that you have Maintainer+ role (trigger tokens are sensitive)")
	}
	out := ListOutput{
		Triggers:   make([]Output, 0, len(triggers)),
		Pagination: toolutil.PaginationFromResponse(resp),
	}
	for _, t := range triggers {
		out.Triggers = append(out.Triggers, convertTrigger(t))
	}
	return out, nil
}

// GetTrigger gets a single pipeline trigger.
func GetTrigger(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("pipeline_trigger_get", toolutil.ErrFieldRequired("project_id"))
	}
	if input.TriggerID == 0 {
		return Output{}, toolutil.WrapErrWithMessage("pipeline_trigger_get", toolutil.ErrFieldRequired("trigger_id"))
	}
	t, _, err := client.GL().PipelineTriggers.GetPipelineTrigger(
		string(input.ProjectID), input.TriggerID, gl.WithContext(ctx),
	)
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("pipeline_trigger_get", err, http.StatusNotFound,
			"verify trigger_id with gitlab_pipeline_trigger_list \u2014 trigger tokens are scoped to a single project")
	}
	return convertTrigger(t), nil
}

// CreateTrigger creates a new pipeline trigger.
func CreateTrigger(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("pipeline_trigger_create", toolutil.ErrFieldRequired("project_id"))
	}
	if input.Description == "" {
		return Output{}, toolutil.WrapErrWithMessage("pipeline_trigger_create", toolutil.ErrFieldRequired("description"))
	}
	opts := &gl.AddPipelineTriggerOptions{
		Description: new(input.Description),
	}
	t, _, err := client.GL().PipelineTriggers.AddPipelineTrigger(
		string(input.ProjectID), opts, gl.WithContext(ctx),
	)
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("pipeline_trigger_create", err,
				"creating trigger tokens requires Maintainer+ role on the project")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("pipeline_trigger_create", err, http.StatusNotFound,
			"verify the project exists with gitlab_project_get")
	}
	return convertTrigger(t), nil
}

// UpdateTrigger updates a pipeline trigger.
func UpdateTrigger(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("pipeline_trigger_update", toolutil.ErrFieldRequired("project_id"))
	}
	if input.TriggerID == 0 {
		return Output{}, toolutil.WrapErrWithMessage("pipeline_trigger_update", toolutil.ErrFieldRequired("trigger_id"))
	}
	opts := &gl.EditPipelineTriggerOptions{}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	t, _, err := client.GL().PipelineTriggers.EditPipelineTrigger(
		string(input.ProjectID), input.TriggerID, opts, gl.WithContext(ctx),
	)
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("pipeline_trigger_update", err,
				"only the trigger owner or Maintainer+ can edit trigger tokens")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("pipeline_trigger_update", err, http.StatusNotFound,
			"verify trigger_id with gitlab_pipeline_trigger_list")
	}
	return convertTrigger(t), nil
}

// DeleteTrigger deletes a pipeline trigger.
func DeleteTrigger(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if input.ProjectID == "" {
		return toolutil.WrapErrWithMessage("pipeline_trigger_delete", toolutil.ErrFieldRequired("project_id"))
	}
	if input.TriggerID == 0 {
		return toolutil.WrapErrWithMessage("pipeline_trigger_delete", toolutil.ErrFieldRequired("trigger_id"))
	}
	_, err := client.GL().PipelineTriggers.DeletePipelineTrigger(
		string(input.ProjectID), input.TriggerID, gl.WithContext(ctx),
	)
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return toolutil.WrapErrWithHint("pipeline_trigger_delete", err,
				"only the trigger owner or Maintainer+ can delete trigger tokens \u2014 the token is invalidated immediately on deletion")
		}
		return toolutil.WrapErrWithStatusHint("pipeline_trigger_delete", err, http.StatusNotFound,
			"verify trigger_id with gitlab_pipeline_trigger_list")
	}
	return nil
}

// RunTrigger triggers a pipeline using a trigger token.
func RunTrigger(ctx context.Context, client *gitlabclient.Client, input RunInput) (RunOutput, error) {
	if input.ProjectID == "" {
		return RunOutput{}, toolutil.WrapErrWithMessage("pipeline_trigger_run", toolutil.ErrFieldRequired("project_id"))
	}
	if input.Ref == "" {
		return RunOutput{}, toolutil.WrapErrWithMessage("pipeline_trigger_run", toolutil.ErrFieldRequired("ref"))
	}
	if input.Token == "" {
		return RunOutput{}, toolutil.WrapErrWithMessage("pipeline_trigger_run", toolutil.ErrFieldRequired("token"))
	}
	opts := &gl.RunPipelineTriggerOptions{
		Ref:   new(input.Ref),
		Token: new(input.Token),
	}
	if input.Variables != "" {
		vars := make(map[string]string)
		if err := json.Unmarshal([]byte(input.Variables), &vars); err != nil {
			return RunOutput{}, toolutil.WrapErrWithMessage("pipeline_trigger_run", fmt.Errorf("invalid variables JSON: %w", err))
		}
		opts.Variables = vars
	}
	p, _, err := client.GL().PipelineTriggers.RunPipelineTrigger(
		string(input.ProjectID), opts, gl.WithContext(ctx),
	)
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusUnauthorized) || toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return RunOutput{}, toolutil.WrapErrWithHint("pipeline_trigger_run", err,
				"the token is invalid or has been revoked \u2014 use gitlab_pipeline_trigger_list to find a valid token (Maintainer+ required to read tokens)")
		}
		if toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return RunOutput{}, toolutil.WrapErrWithHint("pipeline_trigger_run", err,
				"the ref does not exist, the project has no .gitlab-ci.yml, or CI/CD is disabled \u2014 verify with gitlab_branch_get/gitlab_tag_get and gitlab_ci_lint")
		}
		return RunOutput{}, toolutil.WrapErrWithStatusHint("pipeline_trigger_run", err, http.StatusNotFound,
			"verify project_id and that the ref (branch/tag) exists with gitlab_branch_get or gitlab_tag_get")
	}
	return convertPipeline(p), nil
}

// ──────────────────────────────────────────────
// Converters
// ──────────────────────────────────────────────.

// convertTrigger is an internal helper for the pipelinetriggers package.
func convertTrigger(t *gl.PipelineTrigger) Output {
	out := Output{
		ID:          t.ID,
		Description: t.Description,
		Token:       t.Token,
	}
	if t.Owner != nil {
		out.OwnerName = t.Owner.Name
		out.OwnerID = t.Owner.ID
	}
	if t.CreatedAt != nil {
		out.CreatedAt = t.CreatedAt.Format(time.RFC3339)
	}
	if t.UpdatedAt != nil {
		out.UpdatedAt = t.UpdatedAt.Format(time.RFC3339)
	}
	if t.LastUsed != nil {
		out.LastUsed = t.LastUsed.Format(time.RFC3339)
	}
	return out
}

// convertPipeline is an internal helper for the pipelinetriggers package.
func convertPipeline(p *gl.Pipeline) RunOutput {
	out := RunOutput{
		PipelineID: p.ID,
		SHA:        p.SHA,
		Ref:        p.Ref,
		Status:     p.Status,
		WebURL:     p.WebURL,
	}
	if p.CreatedAt != nil {
		out.CreatedAt = p.CreatedAt.Format(time.RFC3339)
	}
	return out
}

// ──────────────────────────────────────────────
// Markdown formatters
// ──────────────────────────────────────────────.
