// Package runnercontrollers implements MCP tool handlers for GitLab Runner Controllers.
// This is an admin-only API. Experimental: may change or be removed in future versions.
package runnercontrollers

import (
	"context"
	"errors"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const errControllerIDRequired = "controller_id is required and must be > 0"

// ---------------------------------------------------------------------------
// Output types
// ---------------------------------------------------------------------------.

// Output represents a runner controller in responses.
type Output struct {
	toolutil.HintableOutput
	ID          int64  `json:"id"`
	Description string `json:"description"`
	State       string `json:"state"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

// DetailsOutput represents detailed runner controller information.
type DetailsOutput struct {
	toolutil.HintableOutput
	Output
	Connected bool `json:"connected"`
}

// ListOutput holds a paginated list of runner controllers.
type ListOutput struct {
	toolutil.HintableOutput
	Controllers []Output                  `json:"controllers"`
	Pagination  toolutil.PaginationOutput `json:"pagination"`
}

// ---------------------------------------------------------------------------
// Converters
// ---------------------------------------------------------------------------.

// toOutput converts a GitLab API [gl.RunnerController] to the MCP tool output format.
func toOutput(rc *gl.RunnerController) Output {
	out := Output{
		ID:          rc.ID,
		Description: rc.Description,
		State:       string(rc.State),
	}
	if rc.CreatedAt != nil {
		out.CreatedAt = rc.CreatedAt.Format(time.RFC3339)
	}
	if rc.UpdatedAt != nil {
		out.UpdatedAt = rc.UpdatedAt.Format(time.RFC3339)
	}
	return out
}

// toDetailsOutput converts a GitLab API [gl.RunnerControllerDetails] to the MCP
// tool output format, embedding the base [Output] and adding connection status.
func toDetailsOutput(rc *gl.RunnerControllerDetails) DetailsOutput {
	return DetailsOutput{
		Output:    toOutput(&rc.RunnerController),
		Connected: rc.Connected,
	}
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------.

// ListInput defines parameters for listing runner controllers.
type ListInput struct {
	toolutil.PaginationInput
}

// List retrieves all runner controllers (admin only).
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	opts := &gl.ListRunnerControllersOptions{}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}

	controllers, resp, err := client.GL().RunnerControllers.ListRunnerControllers(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list runner controllers", err, http.StatusForbidden,
			"runner controllers are an admin-only API \u2014 verify your token has admin scope")
	}

	items := make([]Output, len(controllers))
	for i, rc := range controllers {
		items[i] = toOutput(rc)
	}
	return ListOutput{Controllers: items, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------.

// GetInput defines parameters for getting a runner controller.
type GetInput struct {
	ControllerID int64 `json:"controller_id" jsonschema:"Runner controller ID,required"`
}

// Get retrieves a single runner controller (admin only).
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (DetailsOutput, error) {
	if input.ControllerID <= 0 {
		return DetailsOutput{}, errors.New(errControllerIDRequired)
	}
	if err := ctx.Err(); err != nil {
		return DetailsOutput{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	rc, _, err := client.GL().RunnerControllers.GetRunnerController(input.ControllerID, gl.WithContext(ctx))
	if err != nil {
		return DetailsOutput{}, toolutil.WrapErrWithStatusHint("get runner controller", err, http.StatusNotFound,
			"verify controller_id with gitlab_runner_controller_list; admin-only API")
	}
	return toDetailsOutput(rc), nil
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------.

// CreateInput defines parameters for creating a runner controller.
type CreateInput struct {
	Description string `json:"description,omitempty" jsonschema:"Description of the runner controller"`
	State       string `json:"state,omitempty" jsonschema:"State: enabled, disabled, or dry_run"`
}

// Create registers a new runner controller (admin only).
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	opts := &gl.CreateRunnerControllerOptions{}
	if input.Description != "" {
		opts.Description = &input.Description
	}
	if input.State != "" {
		s := gl.RunnerControllerStateValue(input.State)
		opts.State = &s
	}

	rc, _, err := client.GL().RunnerControllers.CreateRunnerController(opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("create runner controller", err,
				"creating runner controllers requires admin privileges")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("create runner controller", err, http.StatusBadRequest,
			"check name (unique, required) and description fields; runner controllers are an experimental admin-only API")
	}
	return toOutput(rc), nil
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------.

// UpdateInput defines parameters for updating a runner controller.
type UpdateInput struct {
	ControllerID int64  `json:"controller_id" jsonschema:"Runner controller ID,required"`
	Description  string `json:"description,omitempty" jsonschema:"New description"`
	State        string `json:"state,omitempty" jsonschema:"New state: enabled, disabled, or dry_run"`
}

// Update modifies an existing runner controller (admin only).
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if input.ControllerID <= 0 {
		return Output{}, errors.New(errControllerIDRequired)
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	opts := &gl.UpdateRunnerControllerOptions{}
	if input.Description != "" {
		opts.Description = &input.Description
	}
	if input.State != "" {
		s := gl.RunnerControllerStateValue(input.State)
		opts.State = &s
	}

	rc, _, err := client.GL().RunnerControllers.UpdateRunnerController(input.ControllerID, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("update runner controller", err,
				"updating runner controllers requires admin privileges")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("update runner controller", err, http.StatusNotFound,
			"verify controller_id with gitlab_runner_controller_list")
	}
	return toOutput(rc), nil
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------.

// DeleteInput defines parameters for deleting a runner controller.
type DeleteInput struct {
	ControllerID int64 `json:"controller_id" jsonschema:"Runner controller ID,required"`
}

// Delete removes a runner controller (admin only).
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if input.ControllerID <= 0 {
		return errors.New(errControllerIDRequired)
	}
	if err := ctx.Err(); err != nil {
		return toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	_, err := client.GL().RunnerControllers.DeleteRunnerController(input.ControllerID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return toolutil.WrapErrWithHint("delete runner controller", err,
				"deleting runner controllers requires admin privileges")
		}
		return toolutil.WrapErrWithStatusHint("delete runner controller", err, http.StatusNotFound,
			"the controller may already be deleted \u2014 verify controller_id with gitlab_runner_controller_list")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.
