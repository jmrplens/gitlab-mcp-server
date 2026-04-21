// Package runnercontrollertokens implements MCP tool handlers for GitLab Runner Controller Tokens.
// This is an admin-only API. Experimental: may change or be removed in future versions.
package runnercontrollertokens

import (
	"context"
	"errors"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const (
	errControllerIDRequired = "controller_id is required and must be > 0"
	errTokenIDRequired      = "token_id is required and must be > 0" // #nosec G101 -- false positive: error message, not a credential //nolint:gosec
)

// ---------------------------------------------------------------------------
// Output types
// ---------------------------------------------------------------------------.

// Output represents a runner controller token in responses.
type Output struct {
	toolutil.HintableOutput
	ID                 int64  `json:"id"`
	RunnerControllerID int64  `json:"runner_controller_id"`
	Description        string `json:"description"`
	Token              string `json:"token,omitempty"`
	LastUsedAt         string `json:"last_used_at,omitempty"`
	CreatedAt          string `json:"created_at,omitempty"`
	UpdatedAt          string `json:"updated_at,omitempty"`
}

// ListOutput holds a paginated list of runner controller tokens.
type ListOutput struct {
	toolutil.HintableOutput
	Tokens     []Output                  `json:"tokens"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ---------------------------------------------------------------------------
// Converter
// ---------------------------------------------------------------------------.

// toOutput converts a GitLab API [gl.RunnerControllerToken] to the MCP tool output
// format with formatted timestamps for last used, created, and updated times.
func toOutput(t *gl.RunnerControllerToken) Output {
	out := Output{
		ID:                 t.ID,
		RunnerControllerID: t.RunnerControllerID,
		Description:        t.Description,
		Token:              t.Token,
	}
	if t.LastUsedAt != nil {
		out.LastUsedAt = t.LastUsedAt.Format(time.RFC3339)
	}
	if t.CreatedAt != nil {
		out.CreatedAt = t.CreatedAt.Format(time.RFC3339)
	}
	if t.UpdatedAt != nil {
		out.UpdatedAt = t.UpdatedAt.Format(time.RFC3339)
	}
	return out
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------.

// ListInput defines parameters for listing runner controller tokens.
type ListInput struct {
	ControllerID int64 `json:"controller_id" jsonschema:"Runner controller ID,required"`
	toolutil.PaginationInput
}

// List retrieves all tokens for a runner controller (admin only).
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if input.ControllerID <= 0 {
		return ListOutput{}, errors.New(errControllerIDRequired)
	}
	if err := ctx.Err(); err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	opts := &gl.ListRunnerControllerTokensOptions{}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}

	tokens, resp, err := client.GL().RunnerControllerTokens.ListRunnerControllerTokens(input.ControllerID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("list runner controller tokens", err)
	}

	items := make([]Output, len(tokens))
	for i, t := range tokens {
		items[i] = toOutput(t)
	}
	return ListOutput{Tokens: items, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------.

// GetInput defines parameters for getting a runner controller token.
type GetInput struct {
	ControllerID int64 `json:"controller_id" jsonschema:"Runner controller ID,required"`
	TokenID      int64 `json:"token_id" jsonschema:"Token ID,required"`
}

// Get retrieves a single runner controller token (admin only).
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if input.ControllerID <= 0 {
		return Output{}, errors.New(errControllerIDRequired)
	}
	if input.TokenID <= 0 {
		return Output{}, errors.New(errTokenIDRequired)
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	t, _, err := client.GL().RunnerControllerTokens.GetRunnerControllerToken(input.ControllerID, input.TokenID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("get runner controller token", err)
	}
	return toOutput(t), nil
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------.

// CreateInput defines parameters for creating a runner controller token.
type CreateInput struct {
	ControllerID int64  `json:"controller_id" jsonschema:"Runner controller ID,required"`
	Description  string `json:"description,omitempty" jsonschema:"Description of the token"`
}

// Create creates a new runner controller token (admin only).
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if input.ControllerID <= 0 {
		return Output{}, errors.New(errControllerIDRequired)
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	opts := &gl.CreateRunnerControllerTokenOptions{}
	if input.Description != "" {
		opts.Description = &input.Description
	}

	t, _, err := client.GL().RunnerControllerTokens.CreateRunnerControllerToken(input.ControllerID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("create runner controller token", err)
	}
	return toOutput(t), nil
}

// ---------------------------------------------------------------------------
// Rotate
// ---------------------------------------------------------------------------.

// RotateInput defines parameters for rotating a runner controller token.
type RotateInput struct {
	ControllerID int64 `json:"controller_id" jsonschema:"Runner controller ID,required"`
	TokenID      int64 `json:"token_id" jsonschema:"Token ID,required"`
}

// Rotate rotates a runner controller token (admin only).
func Rotate(ctx context.Context, client *gitlabclient.Client, input RotateInput) (Output, error) {
	if input.ControllerID <= 0 {
		return Output{}, errors.New(errControllerIDRequired)
	}
	if input.TokenID <= 0 {
		return Output{}, errors.New(errTokenIDRequired)
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	t, _, err := client.GL().RunnerControllerTokens.RotateRunnerControllerToken(input.ControllerID, input.TokenID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("rotate runner controller token", err)
	}
	return toOutput(t), nil
}

// ---------------------------------------------------------------------------
// Revoke
// ---------------------------------------------------------------------------.

// RevokeInput defines parameters for revoking a runner controller token.
type RevokeInput struct {
	ControllerID int64 `json:"controller_id" jsonschema:"Runner controller ID,required"`
	TokenID      int64 `json:"token_id" jsonschema:"Token ID,required"`
}

// Revoke revokes a runner controller token (admin only).
func Revoke(ctx context.Context, client *gitlabclient.Client, input RevokeInput) error {
	if input.ControllerID <= 0 {
		return errors.New(errControllerIDRequired)
	}
	if input.TokenID <= 0 {
		return errors.New(errTokenIDRequired)
	}
	if err := ctx.Err(); err != nil {
		return toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	_, err := client.GL().RunnerControllerTokens.RevokeRunnerControllerToken(input.ControllerID, input.TokenID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("revoke runner controller token", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.
