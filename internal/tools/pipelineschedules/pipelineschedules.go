// Package pipelineschedules implements MCP tool handlers for GitLab pipeline
// schedule operations including list, get, create, update, delete, run, and
// schedule variable management via the PipelineSchedules API.
package pipelineschedules

import (
	"context"
	"errors"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ---------------------------------------------------------------------------
// Input types
// ---------------------------------------------------------------------------.

// ListInput contains parameters for listing pipeline schedules.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id"       jsonschema:"Project ID or URL-encoded path,required"`
	Scope     string               `json:"scope,omitempty"  jsonschema:"Filter by scope: active or inactive"`
	toolutil.PaginationInput
}

// GetInput contains parameters for retrieving a single pipeline schedule.
type GetInput struct {
	ProjectID  toolutil.StringOrInt `json:"project_id"   jsonschema:"Project ID or URL-encoded path,required"`
	ScheduleID int                  `json:"schedule_id"  jsonschema:"Pipeline schedule ID,required"`
}

// CreateInput contains parameters for creating a pipeline schedule.
type CreateInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id"              jsonschema:"Project ID or URL-encoded path,required"`
	Description  string               `json:"description"             jsonschema:"Schedule description,required"`
	Ref          string               `json:"ref"                     jsonschema:"Branch or tag to run the pipeline on,required"`
	Cron         string               `json:"cron"                    jsonschema:"Cron expression (e.g. 0 1 * * *),required"`
	CronTimezone string               `json:"cron_timezone,omitempty" jsonschema:"Cron timezone (e.g. UTC or America/New_York)"`
	Active       *bool                `json:"active,omitempty"        jsonschema:"Whether the schedule is active (default: true)"`
}

// UpdateInput contains parameters for editing a pipeline schedule.
type UpdateInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id"              jsonschema:"Project ID or URL-encoded path,required"`
	ScheduleID   int                  `json:"schedule_id"             jsonschema:"Pipeline schedule ID,required"`
	Description  string               `json:"description,omitempty"   jsonschema:"Updated description"`
	Ref          string               `json:"ref,omitempty"           jsonschema:"Updated branch or tag"`
	Cron         string               `json:"cron,omitempty"          jsonschema:"Updated cron expression"`
	CronTimezone string               `json:"cron_timezone,omitempty" jsonschema:"Updated cron timezone"`
	Active       *bool                `json:"active,omitempty"        jsonschema:"Enable or disable the schedule"`
}

// DeleteInput contains parameters for deleting a pipeline schedule.
type DeleteInput struct {
	ProjectID  toolutil.StringOrInt `json:"project_id"   jsonschema:"Project ID or URL-encoded path,required"`
	ScheduleID int                  `json:"schedule_id"  jsonschema:"Pipeline schedule ID,required"`
}

// RunInput contains parameters for triggering a pipeline schedule.
type RunInput struct {
	ProjectID  toolutil.StringOrInt `json:"project_id"   jsonschema:"Project ID or URL-encoded path,required"`
	ScheduleID int                  `json:"schedule_id"  jsonschema:"Pipeline schedule ID,required"`
}

// ---------------------------------------------------------------------------
// Output types
// ---------------------------------------------------------------------------.

// Output represents a single pipeline schedule in MCP responses.
type Output struct {
	toolutil.HintableOutput
	ID           int    `json:"id"`
	Description  string `json:"description"`
	Ref          string `json:"ref"`
	Cron         string `json:"cron"`
	CronTimezone string `json:"cron_timezone"`
	NextRunAt    string `json:"next_run_at,omitempty"`
	Active       bool   `json:"active"`
	OwnerName    string `json:"owner_name,omitempty"`
	CreatedAt    string `json:"created_at,omitempty"`
	UpdatedAt    string `json:"updated_at,omitempty"`
}

// ListOutput represents a paginated list of pipeline schedules.
type ListOutput struct {
	toolutil.HintableOutput
	Schedules  []Output                  `json:"schedules"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ---------------------------------------------------------------------------
// Converter
// ---------------------------------------------------------------------------.

// toOutput converts the GitLab API response to the tool output format.
func toOutput(s *gitlab.PipelineSchedule) Output {
	out := Output{
		ID:           int(s.ID),
		Description:  s.Description,
		Ref:          s.Ref,
		Cron:         s.Cron,
		CronTimezone: s.CronTimezone,
		Active:       s.Active,
	}
	if s.Owner != nil {
		out.OwnerName = s.Owner.Username
	}
	if s.NextRunAt != nil {
		out.NextRunAt = s.NextRunAt.Format(time.RFC3339)
	}
	if s.CreatedAt != nil {
		out.CreatedAt = s.CreatedAt.Format(time.RFC3339)
	}
	if s.UpdatedAt != nil {
		out.UpdatedAt = s.UpdatedAt.Format(time.RFC3339)
	}
	return out
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------.

// List lists resources for the pipelineschedules package.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if err := ctx.Err(); err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	opts := &gitlab.ListPipelineSchedulesOptions{
		ListOptions: gitlab.ListOptions{
			Page:    int64(input.Page),
			PerPage: int64(input.PerPage),
		},
	}
	if input.Scope != "" {
		scope := gitlab.PipelineScheduleScopeValue(input.Scope)
		opts.Scope = &scope
	}

	schedules, resp, err := client.GL().PipelineSchedules.ListPipelineSchedules(string(input.ProjectID), opts, gitlab.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("list pipeline schedules", err)
	}

	items := make([]Output, 0, len(schedules))
	for _, s := range schedules {
		items = append(items, toOutput(s))
	}

	return ListOutput{
		Schedules:  items,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// Get retrieves resources for the pipelineschedules package.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.ScheduleID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("getPipelineSchedule", "schedule_id")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	s, _, err := client.GL().PipelineSchedules.GetPipelineSchedule(string(input.ProjectID), int64(input.ScheduleID), gitlab.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("get pipeline schedule", err)
	}

	return toOutput(s), nil
}

// Create creates resources for the pipelineschedules package.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.Description == "" {
		return Output{}, toolutil.ErrFieldRequired("description")
	}
	if input.Ref == "" {
		return Output{}, toolutil.ErrFieldRequired("ref")
	}
	if input.Cron == "" {
		return Output{}, toolutil.ErrFieldRequired("cron")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	opts := &gitlab.CreatePipelineScheduleOptions{
		Description: &input.Description,
		Ref:         &input.Ref,
		Cron:        &input.Cron,
	}
	if input.CronTimezone != "" {
		opts.CronTimezone = &input.CronTimezone
	}
	if input.Active != nil {
		opts.Active = input.Active
	}

	s, _, err := client.GL().PipelineSchedules.CreatePipelineSchedule(string(input.ProjectID), opts, gitlab.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("create pipeline schedule", err)
	}

	return toOutput(s), nil
}

// Update updates resources for the pipelineschedules package.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.ScheduleID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("updatePipelineSchedule", "schedule_id")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	opts := &gitlab.EditPipelineScheduleOptions{}
	if input.Description != "" {
		opts.Description = &input.Description
	}
	if input.Ref != "" {
		opts.Ref = &input.Ref
	}
	if input.Cron != "" {
		opts.Cron = &input.Cron
	}
	if input.CronTimezone != "" {
		opts.CronTimezone = &input.CronTimezone
	}
	if input.Active != nil {
		opts.Active = input.Active
	}

	s, _, err := client.GL().PipelineSchedules.EditPipelineSchedule(string(input.ProjectID), int64(input.ScheduleID), opts, gitlab.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("update pipeline schedule", err)
	}

	return toOutput(s), nil
}

// Delete deletes resources for the pipelineschedules package.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if input.ProjectID == "" {
		return toolutil.ErrFieldRequired("project_id")
	}
	if input.ScheduleID <= 0 {
		return toolutil.ErrRequiredInt64("deletePipelineSchedule", "schedule_id")
	}
	if err := ctx.Err(); err != nil {
		return toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	_, err := client.GL().PipelineSchedules.DeletePipelineSchedule(string(input.ProjectID), int64(input.ScheduleID), gitlab.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("delete pipeline schedule", err)
	}
	return nil
}

// Run runs resources for the pipelineschedules package.
func Run(ctx context.Context, client *gitlabclient.Client, input RunInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.ScheduleID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("runPipelineSchedule", "schedule_id")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	_, err := client.GL().PipelineSchedules.RunPipelineSchedule(string(input.ProjectID), int64(input.ScheduleID), gitlab.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("run pipeline schedule", err)
	}

	// Fetch the schedule after triggering to return current state
	s, _, err := client.GL().PipelineSchedules.GetPipelineSchedule(string(input.ProjectID), int64(input.ScheduleID), gitlab.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("get pipeline schedule after run", err)
	}

	return toOutput(s), nil
}

// Take Ownership.

// TakeOwnershipInput defines parameters for taking ownership of a pipeline schedule.
type TakeOwnershipInput struct {
	ProjectID  toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	ScheduleID int                  `json:"schedule_id" jsonschema:"Pipeline schedule ID,required"`
}

// TakeOwnership takes ownership of a pipeline schedule, making the current user the owner.
func TakeOwnership(ctx context.Context, client *gitlabclient.Client, input TakeOwnershipInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, errors.New("take_ownership_pipeline_schedule: project_id is required")
	}
	if input.ScheduleID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("takeOwnershipPipelineSchedule", "schedule_id")
	}

	s, _, err := client.GL().PipelineSchedules.TakeOwnershipOfPipelineSchedule(
		string(input.ProjectID), int64(input.ScheduleID), gitlab.WithContext(ctx),
	)
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("take_ownership_pipeline_schedule", err)
	}
	return toOutput(s), nil
}

// Schedule Variables.

// VariableOutput represents a pipeline schedule variable.
type VariableOutput struct {
	toolutil.HintableOutput
	Key          string `json:"key"`
	Value        string `json:"value"`
	VariableType string `json:"variable_type"`
}

// CreateVariableInput defines parameters for creating a pipeline schedule variable.
type CreateVariableInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	ScheduleID   int                  `json:"schedule_id" jsonschema:"Pipeline schedule ID,required"`
	Key          string               `json:"key" jsonschema:"Variable key,required"`
	Value        string               `json:"value" jsonschema:"Variable value,required"`
	VariableType string               `json:"variable_type,omitempty" jsonschema:"Variable type: env_var (default) or file"`
}

// CreateVariable creates a new variable for a pipeline schedule.
func CreateVariable(ctx context.Context, client *gitlabclient.Client, input CreateVariableInput) (VariableOutput, error) {
	if input.ProjectID == "" {
		return VariableOutput{}, errors.New("create_pipeline_schedule_variable: project_id is required")
	}
	if input.ScheduleID <= 0 {
		return VariableOutput{}, toolutil.ErrRequiredInt64("createPipelineScheduleVariable", "schedule_id")
	}
	if input.Key == "" {
		return VariableOutput{}, errors.New("create_pipeline_schedule_variable: key is required")
	}
	if input.Value == "" {
		return VariableOutput{}, errors.New("create_pipeline_schedule_variable: value is required")
	}

	opts := &gitlab.CreatePipelineScheduleVariableOptions{
		Key:   new(input.Key),
		Value: new(input.Value),
	}
	if input.VariableType != "" {
		opts.VariableType = new(gitlab.VariableTypeValue(input.VariableType))
	}

	v, _, err := client.GL().PipelineSchedules.CreatePipelineScheduleVariable(
		string(input.ProjectID), int64(input.ScheduleID), opts, gitlab.WithContext(ctx),
	)
	if err != nil {
		return VariableOutput{}, toolutil.WrapErrWithMessage("create_pipeline_schedule_variable", err)
	}
	return VariableOutput{Key: v.Key, Value: v.Value, VariableType: string(v.VariableType)}, nil
}

// EditVariableInput defines parameters for editing a pipeline schedule variable.
type EditVariableInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	ScheduleID   int                  `json:"schedule_id" jsonschema:"Pipeline schedule ID,required"`
	Key          string               `json:"key" jsonschema:"Variable key to edit,required"`
	Value        string               `json:"value" jsonschema:"New variable value,required"`
	VariableType string               `json:"variable_type,omitempty" jsonschema:"Variable type: env_var or file"`
}

// EditVariable edits an existing pipeline schedule variable.
func EditVariable(ctx context.Context, client *gitlabclient.Client, input EditVariableInput) (VariableOutput, error) {
	if input.ProjectID == "" {
		return VariableOutput{}, errors.New("edit_pipeline_schedule_variable: project_id is required")
	}
	if input.ScheduleID <= 0 {
		return VariableOutput{}, toolutil.ErrRequiredInt64("editPipelineScheduleVariable", "schedule_id")
	}
	if input.Key == "" {
		return VariableOutput{}, errors.New("edit_pipeline_schedule_variable: key is required")
	}
	if input.Value == "" {
		return VariableOutput{}, errors.New("edit_pipeline_schedule_variable: value is required")
	}

	opts := &gitlab.EditPipelineScheduleVariableOptions{
		Value: new(input.Value),
	}
	if input.VariableType != "" {
		opts.VariableType = new(gitlab.VariableTypeValue(input.VariableType))
	}

	v, _, err := client.GL().PipelineSchedules.EditPipelineScheduleVariable(
		string(input.ProjectID), int64(input.ScheduleID), input.Key, opts, gitlab.WithContext(ctx),
	)
	if err != nil {
		return VariableOutput{}, toolutil.WrapErrWithMessage("edit_pipeline_schedule_variable", err)
	}
	return VariableOutput{Key: v.Key, Value: v.Value, VariableType: string(v.VariableType)}, nil
}

// DeleteVariableInput defines parameters for deleting a pipeline schedule variable.
type DeleteVariableInput struct {
	ProjectID  toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	ScheduleID int                  `json:"schedule_id" jsonschema:"Pipeline schedule ID,required"`
	Key        string               `json:"key" jsonschema:"Variable key to delete,required"`
}

// DeleteVariable deletes a pipeline schedule variable by key.
func DeleteVariable(ctx context.Context, client *gitlabclient.Client, input DeleteVariableInput) error {
	if input.ProjectID == "" {
		return errors.New("delete_pipeline_schedule_variable: project_id is required")
	}
	if input.ScheduleID <= 0 {
		return toolutil.ErrRequiredInt64("deletePipelineScheduleVariable", "schedule_id")
	}
	if input.Key == "" {
		return errors.New("delete_pipeline_schedule_variable: key is required")
	}

	_, _, err := client.GL().PipelineSchedules.DeletePipelineScheduleVariable(
		string(input.ProjectID), int64(input.ScheduleID), input.Key, gitlab.WithContext(ctx),
	)
	if err != nil {
		return toolutil.WrapErrWithMessage("delete_pipeline_schedule_variable", err)
	}
	return nil
}

// List Pipelines Triggered by Schedule.

// ListTriggeredPipelinesInput defines parameters for listing pipelines triggered by a schedule.
type ListTriggeredPipelinesInput struct {
	ProjectID  toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	ScheduleID int                  `json:"schedule_id" jsonschema:"Pipeline schedule ID,required"`
	Page       int64                `json:"page,omitempty" jsonschema:"Page number for pagination"`
	PerPage    int64                `json:"per_page,omitempty" jsonschema:"Items per page (max 100)"`
}

// TriggeredPipelineOutput represents a pipeline triggered by a schedule.
type TriggeredPipelineOutput struct {
	ID     int    `json:"id"`
	IID    int    `json:"iid"`
	Ref    string `json:"ref"`
	SHA    string `json:"sha"`
	Status string `json:"status"`
	Source string `json:"source"`
	WebURL string `json:"web_url"`
}

// TriggeredPipelinesListOutput represents the paginated result of pipelines triggered by a schedule.
type TriggeredPipelinesListOutput struct {
	toolutil.HintableOutput
	Pipelines  []TriggeredPipelineOutput `json:"pipelines"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ListTriggeredPipelines lists all pipelines triggered by a specific schedule.
func ListTriggeredPipelines(ctx context.Context, client *gitlabclient.Client, input ListTriggeredPipelinesInput) (TriggeredPipelinesListOutput, error) {
	if input.ProjectID == "" {
		return TriggeredPipelinesListOutput{}, errors.New("list_triggered_pipelines: project_id is required")
	}
	if input.ScheduleID <= 0 {
		return TriggeredPipelinesListOutput{}, toolutil.ErrRequiredInt64("listTriggeredPipelines", "schedule_id")
	}

	opts := &gitlab.ListPipelinesTriggeredByScheduleOptions{}
	if input.Page > 0 {
		opts.Page = input.Page
	}
	if input.PerPage > 0 {
		opts.PerPage = input.PerPage
	}

	pipelines, resp, err := client.GL().PipelineSchedules.ListPipelinesTriggeredBySchedule(
		string(input.ProjectID), int64(input.ScheduleID), opts, gitlab.WithContext(ctx),
	)
	if err != nil {
		return TriggeredPipelinesListOutput{}, toolutil.WrapErrWithMessage("list_triggered_pipelines", err)
	}

	items := make([]TriggeredPipelineOutput, 0, len(pipelines))
	for _, p := range pipelines {
		items = append(items, TriggeredPipelineOutput{
			ID:     int(p.ID),
			IID:    int(p.IID),
			Ref:    p.Ref,
			SHA:    p.SHA,
			Status: p.Status,
			Source: string(p.Source),
			WebURL: p.WebURL,
		})
	}

	return TriggeredPipelinesListOutput{
		Pipelines:  items,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// Formatters for new types.
