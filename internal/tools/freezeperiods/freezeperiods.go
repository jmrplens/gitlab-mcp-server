// Package freezeperiods implements MCP tools for GitLab deploy freeze period operations.
package freezeperiods

import (
	"context"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// Input types.

// ListInput is the input for listing freeze periods.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Page      int64                `json:"page,omitempty" jsonschema:"Page number for pagination"`
	PerPage   int64                `json:"per_page,omitempty" jsonschema:"Number of items per page"`
}

// GetInput is the input for getting a single freeze period.
type GetInput struct {
	ProjectID      toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	FreezePeriodID int64                `json:"freeze_period_id" jsonschema:"Freeze period ID,required"`
}

// CreateInput is the input for creating a freeze period.
type CreateInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	FreezeStart  string               `json:"freeze_start" jsonschema:"Cron expression for freeze start (e.g. 0 23 * * 5),required"`
	FreezeEnd    string               `json:"freeze_end" jsonschema:"Cron expression for freeze end (e.g. 0 7 * * 1),required"`
	CronTimezone string               `json:"cron_timezone,omitempty" jsonschema:"Timezone for cron expressions (e.g. America/New_York)"`
}

// UpdateInput is the input for updating a freeze period.
type UpdateInput struct {
	ProjectID      toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	FreezePeriodID int64                `json:"freeze_period_id" jsonschema:"Freeze period ID,required"`
	FreezeStart    string               `json:"freeze_start,omitempty" jsonschema:"Cron expression for freeze start"`
	FreezeEnd      string               `json:"freeze_end,omitempty" jsonschema:"Cron expression for freeze end"`
	CronTimezone   string               `json:"cron_timezone,omitempty" jsonschema:"Timezone for cron expressions"`
}

// DeleteInput is the input for deleting a freeze period.
type DeleteInput struct {
	ProjectID      toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	FreezePeriodID int64                `json:"freeze_period_id" jsonschema:"Freeze period ID,required"`
}

// Output types.

// Output represents a freeze period.
type Output struct {
	toolutil.HintableOutput
	ID           int64  `json:"id"`
	FreezeStart  string `json:"freeze_start"`
	FreezeEnd    string `json:"freeze_end"`
	CronTimezone string `json:"cron_timezone"`
	CreatedAt    string `json:"created_at,omitempty"`
	UpdatedAt    string `json:"updated_at,omitempty"`
}

// ListOutput represents a list of freeze periods.
type ListOutput struct {
	toolutil.HintableOutput
	FreezePeriods []Output                  `json:"freeze_periods"`
	Pagination    toolutil.PaginationOutput `json:"pagination"`
}

// Handlers.

// List lists freeze periods for a project.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.WrapErrWithMessage("freeze_period_list", toolutil.ErrFieldRequired("project_id"))
	}
	opts := &gl.ListFreezePeriodsOptions{ListOptions: gl.ListOptions{Page: input.Page, PerPage: input.PerPage}}
	periods, resp, err := client.GL().FreezePeriods.ListFreezePeriods(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("freeze_period_list", err, http.StatusNotFound,
			"verify project_id; freeze periods are project-scoped, not group-scoped")
	}
	return toListOutput(periods, resp), nil
}

// Get gets a single freeze period.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("freeze_period_get", toolutil.ErrFieldRequired("project_id"))
	}
	if input.FreezePeriodID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("freeze_period_get", "freeze_period_id")
	}
	fp, _, err := client.GL().FreezePeriods.GetFreezePeriod(string(input.ProjectID), input.FreezePeriodID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("freeze_period_get", err, http.StatusNotFound,
			"verify freeze_period_id with gitlab_list_freeze_periods")
	}
	return toOutput(fp), nil
}

// Create creates a freeze period.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("freeze_period_create", toolutil.ErrFieldRequired("project_id"))
	}
	opts := &gl.CreateFreezePeriodOptions{
		FreezeStart: new(input.FreezeStart),
		FreezeEnd:   new(input.FreezeEnd),
	}
	if input.CronTimezone != "" {
		opts.CronTimezone = new(input.CronTimezone)
	}
	fp, _, err := client.GL().FreezePeriods.CreateFreezePeriodOptions(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("freeze_period_create", err,
				"creating freeze periods requires Maintainer or Owner role")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("freeze_period_create", err, http.StatusBadRequest,
			"freeze_start and freeze_end must be valid POSIX cron strings (e.g. '0 23 * * 5'); cron_timezone defaults to UTC \u2014 use IANA names like 'Europe/Madrid' or POSIX offsets")
	}
	return toOutput(fp), nil
}

// Update updates a freeze period.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("freeze_period_update", toolutil.ErrFieldRequired("project_id"))
	}
	if input.FreezePeriodID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("freeze_period_update", "freeze_period_id")
	}
	opts := &gl.UpdateFreezePeriodOptions{}
	if input.FreezeStart != "" {
		opts.FreezeStart = new(input.FreezeStart)
	}
	if input.FreezeEnd != "" {
		opts.FreezeEnd = new(input.FreezeEnd)
	}
	if input.CronTimezone != "" {
		opts.CronTimezone = new(input.CronTimezone)
	}
	fp, _, err := client.GL().FreezePeriods.UpdateFreezePeriodOptions(string(input.ProjectID), input.FreezePeriodID, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("freeze_period_update", err,
				"updating freeze periods requires Maintainer or Owner role")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("freeze_period_update", err, http.StatusNotFound,
			"verify freeze_period_id with gitlab_list_freeze_periods; cron strings must be valid POSIX cron")
	}
	return toOutput(fp), nil
}

// Delete deletes a freeze period.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if input.ProjectID == "" {
		return toolutil.WrapErrWithMessage("freeze_period_delete", toolutil.ErrFieldRequired("project_id"))
	}
	if input.FreezePeriodID <= 0 {
		return toolutil.ErrRequiredInt64("freeze_period_delete", "freeze_period_id")
	}
	_, err := client.GL().FreezePeriods.DeleteFreezePeriod(string(input.ProjectID), input.FreezePeriodID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return toolutil.WrapErrWithHint("freeze_period_delete", err,
				"deleting freeze periods requires Maintainer or Owner role")
		}
		return toolutil.WrapErrWithStatusHint("freeze_period_delete", err, http.StatusNotFound,
			"the freeze period may already be deleted \u2014 verify with gitlab_list_freeze_periods")
	}
	return nil
}

// Converters.

// toOutput converts the GitLab API response to the tool output format.
func toOutput(fp *gl.FreezePeriod) Output {
	out := Output{
		ID:           fp.ID,
		FreezeStart:  fp.FreezeStart,
		FreezeEnd:    fp.FreezeEnd,
		CronTimezone: fp.CronTimezone,
	}
	if fp.CreatedAt != nil {
		out.CreatedAt = fp.CreatedAt.Format(time.RFC3339)
	}
	if fp.UpdatedAt != nil {
		out.UpdatedAt = fp.UpdatedAt.Format(time.RFC3339)
	}
	return out
}

// toListOutput converts the GitLab API response to the tool output format.
func toListOutput(periods []*gl.FreezePeriod, resp *gl.Response) ListOutput {
	out := ListOutput{
		FreezePeriods: make([]Output, 0, len(periods)),
		Pagination:    toolutil.PaginationFromResponse(resp),
	}
	for _, fp := range periods {
		out.FreezePeriods = append(out.FreezePeriods, toOutput(fp))
	}
	return out
}

// Formatters.
