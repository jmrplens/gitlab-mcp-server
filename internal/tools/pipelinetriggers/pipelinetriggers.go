package pipelinetriggers

import (
	"context"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ──────────────────────────────────────────────
// Output types
// ──────────────────────────────────────────────.

// Output mirrors gl.PipelineTrigger, a single pipeline trigger token. Per the
// 1:1 audit policy the owner is surfaced as the full nested user object on the
// canonical owner key.
type Output struct {
	toolutil.HintableOutput
	ID          int64       `json:"id"`
	Description string      `json:"description"`
	Token       string      `json:"token"`
	Owner       *UserOutput `json:"owner,omitempty"`
	CreatedAt   string      `json:"created_at,omitempty"`
	UpdatedAt   string      `json:"updated_at,omitempty"`
	DeletedAt   string      `json:"deleted_at,omitempty"`
	LastUsed    string      `json:"last_used,omitempty"`
}

// ListOutput represents a paginated list of pipeline triggers.
type ListOutput struct {
	toolutil.HintableOutput
	Triggers   []Output                  `json:"triggers"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// RunOutput mirrors gl.Pipeline, the result of triggering a pipeline. Per the
// 1:1 audit policy every Pipeline field is surfaced, with the user and
// detailed_status sub-objects preserved as full nested objects.
type RunOutput struct {
	toolutil.HintableOutput
	ID             int64                 `json:"id"`
	IID            int64                 `json:"iid,omitempty"`
	ProjectID      int64                 `json:"project_id,omitempty"`
	Status         string                `json:"status"`
	Source         string                `json:"source,omitempty"`
	Ref            string                `json:"ref"`
	Name           string                `json:"name,omitempty"`
	SHA            string                `json:"sha"`
	BeforeSHA      string                `json:"before_sha,omitempty"`
	Tag            bool                  `json:"tag,omitempty"`
	YamlErrors     string                `json:"yaml_errors,omitempty"`
	User           *BasicUserOutput      `json:"user,omitempty"`
	Duration       int64                 `json:"duration,omitempty"`
	QueuedDuration int64                 `json:"queued_duration,omitempty"`
	Coverage       string                `json:"coverage,omitempty"`
	WebURL         string                `json:"web_url"`
	DetailedStatus *DetailedStatusOutput `json:"detailed_status,omitempty"`
	CreatedAt      string                `json:"created_at,omitempty"`
	UpdatedAt      string                `json:"updated_at,omitempty"`
	StartedAt      string                `json:"started_at,omitempty"`
	FinishedAt     string                `json:"finished_at,omitempty"`
	CommittedAt    string                `json:"committed_at,omitempty"`
}

// ──────────────────────────────────────────────
// Input types
// ──────────────────────────────────────────────.

// ListInput contains parameters for listing pipeline triggers. It embeds both
// offset (PaginationInput) and keyset (KeysetPaginationInput) pagination plus
// the order_by/sort controls accepted by gl.ListOptions.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Field by which to order results when using keyset pagination (e.g. id)."`
	Sort      string               `json:"sort,omitempty" jsonschema:"Sort order: asc or desc. Defaults to the API ordering."`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
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

// RunInput contains parameters for triggering a pipeline. It mirrors
// gl.RunPipelineTriggerOptions: variables maps directly to the SDK's
// map[string]string CI/CD variable shape, and inputs carries typed pipeline
// input parameters.
type RunInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	Ref       string               `json:"ref" jsonschema:"Branch or tag name to run pipeline on"`
	Token     string               `json:"token" jsonschema:"Pipeline trigger token"`
	Variables map[string]string    `json:"variables,omitempty" jsonschema:"Map of CI/CD variable name to value injected into the triggered pipeline."`
	Inputs    map[string]any       `json:"inputs,omitempty" jsonschema:"Map of pipeline input name to value (string, number, boolean, or array of strings) for inputs declared in the pipeline spec."`
}

// ──────────────────────────────────────────────
// Handlers
// ──────────────────────────────────────────────.

// ListTriggers lists pipeline triggers for a project.
func ListTriggers(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.WrapErrWithMessage("pipeline_trigger_list", toolutil.ErrFieldRequired("project_id"))
	}
	opts := &gl.ListPipelineTriggersOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
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
	if len(input.Variables) > 0 {
		opts.Variables = input.Variables
	}
	if len(input.Inputs) > 0 {
		inputs, err := buildPipelineInputs(input.Inputs)
		if err != nil {
			return RunOutput{}, toolutil.WrapErrWithMessage("pipeline_trigger_run", err)
		}
		opts.Inputs = inputs
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

// convertTrigger maps a GitLab pipeline trigger into the MCP output shape,
// surfacing the full owner user object on the canonical owner key.
func convertTrigger(t *gl.PipelineTrigger) Output {
	return Output{
		ID:          t.ID,
		Description: t.Description,
		Token:       t.Token,
		Owner:       userOutput(t.Owner),
		CreatedAt:   formatTimePtr(t.CreatedAt),
		UpdatedAt:   formatTimePtr(t.UpdatedAt),
		DeletedAt:   formatTimePtr(t.DeletedAt),
		LastUsed:    formatTimePtr(t.LastUsed),
	}
}

// convertPipeline maps a triggered GitLab pipeline into the run output shape,
// surfacing every gl.Pipeline field plus the nested user and detailed_status
// sub-objects.
func convertPipeline(p *gl.Pipeline) RunOutput {
	return RunOutput{
		ID:             p.ID,
		IID:            p.IID,
		ProjectID:      p.ProjectID,
		Status:         p.Status,
		Source:         string(p.Source),
		Ref:            p.Ref,
		Name:           p.Name,
		SHA:            p.SHA,
		BeforeSHA:      p.BeforeSHA,
		Tag:            p.Tag,
		YamlErrors:     p.YamlErrors,
		User:           basicUserOutput(p.User),
		Duration:       p.Duration,
		QueuedDuration: p.QueuedDuration,
		Coverage:       p.Coverage,
		WebURL:         p.WebURL,
		DetailedStatus: detailedStatusOutput(p.DetailedStatus),
		CreatedAt:      formatTimePtr(p.CreatedAt),
		UpdatedAt:      formatTimePtr(p.UpdatedAt),
		StartedAt:      formatTimePtr(p.StartedAt),
		FinishedAt:     formatTimePtr(p.FinishedAt),
		CommittedAt:    formatTimePtr(p.CommittedAt),
	}
}

// buildPipelineInputs converts a generic inputs map into the SDK's
// gl.PipelineInputsOption, wrapping each value with the type-safe
// gl.NewPipelineInputValue constructor. Supported value types are string,
// bool, number (float64/int from JSON), and arrays of strings. JSON numbers
// arrive as float64, so they are forwarded as float64.
func buildPipelineInputs(raw map[string]any) (gl.PipelineInputsOption, error) {
	inputs := make(gl.PipelineInputsOption, len(raw))
	for key, val := range raw {
		switch v := val.(type) {
		case string:
			inputs[key] = gl.NewPipelineInputValue(v)
		case bool:
			inputs[key] = gl.NewPipelineInputValue(v)
		case float64:
			inputs[key] = gl.NewPipelineInputValue(v)
		case int:
			inputs[key] = gl.NewPipelineInputValue(v)
		case int64:
			inputs[key] = gl.NewPipelineInputValue(v)
		case []string:
			inputs[key] = gl.NewPipelineInputValue(v)
		case []any:
			strs := make([]string, 0, len(v))
			for _, item := range v {
				s, ok := item.(string)
				if !ok {
					return nil, fmt.Errorf("input %q: array elements must be strings", key)
				}
				strs = append(strs, s)
			}
			inputs[key] = gl.NewPipelineInputValue(strs)
		default:
			return nil, fmt.Errorf("input %q: unsupported value type %T (use string, number, boolean, or array of strings)", key, val)
		}
	}
	return inputs, nil
}

// ──────────────────────────────────────────────
// Markdown formatters
// ──────────────────────────────────────────────.
