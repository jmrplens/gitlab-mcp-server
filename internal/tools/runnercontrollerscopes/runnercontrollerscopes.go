// Package runnercontrollerscopes implements MCP tool handlers for GitLab Runner Controller Scopes.
// This is an admin-only API. Experimental: may change or be removed in future versions.
package runnercontrollerscopes

import (
	"context"
	"errors"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ---------------------------------------------------------------------------
// Output types
// ---------------------------------------------------------------------------.

// ScopesOutput represents all scopes for a runner controller.
type ScopesOutput struct {
	toolutil.HintableOutput
	InstanceLevelScopings []InstanceScopeItem `json:"instance_level_scopings"`
	RunnerLevelScopings   []RunnerScopeItem   `json:"runner_level_scopings"`
}

// InstanceScopeItem represents an instance-level scope.
type InstanceScopeItem struct {
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// RunnerScopeItem represents a runner-level scope.
type RunnerScopeItem struct {
	RunnerID  int64  `json:"runner_id"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// InstanceScopeOutput represents a single instance scope result.
type InstanceScopeOutput struct {
	toolutil.HintableOutput
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// RunnerScopeOutput represents a single runner scope result.
type RunnerScopeOutput struct {
	toolutil.HintableOutput
	RunnerID  int64  `json:"runner_id"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// ---------------------------------------------------------------------------
// Converters
// ---------------------------------------------------------------------------.

// toScopesOutput converts a GitLab API [gl.RunnerControllerScopes] to the MCP tool
// output format, collecting instance-level and runner-level scoping entries.
func toScopesOutput(s *gl.RunnerControllerScopes) ScopesOutput {
	out := ScopesOutput{}
	for _, is := range s.InstanceLevelScopings {
		item := InstanceScopeItem{}
		if is.CreatedAt != nil {
			item.CreatedAt = is.CreatedAt.Format(time.RFC3339)
		}
		if is.UpdatedAt != nil {
			item.UpdatedAt = is.UpdatedAt.Format(time.RFC3339)
		}
		out.InstanceLevelScopings = append(out.InstanceLevelScopings, item)
	}
	for _, rs := range s.RunnerLevelScopings {
		item := RunnerScopeItem{RunnerID: rs.RunnerID}
		if rs.CreatedAt != nil {
			item.CreatedAt = rs.CreatedAt.Format(time.RFC3339)
		}
		if rs.UpdatedAt != nil {
			item.UpdatedAt = rs.UpdatedAt.Format(time.RFC3339)
		}
		out.RunnerLevelScopings = append(out.RunnerLevelScopings, item)
	}
	return out
}

// toInstanceScopeOutput converts a GitLab API [gl.RunnerControllerInstanceLevelScoping]
// to the MCP tool output format with formatted timestamps.
func toInstanceScopeOutput(is *gl.RunnerControllerInstanceLevelScoping) InstanceScopeOutput {
	out := InstanceScopeOutput{}
	if is.CreatedAt != nil {
		out.CreatedAt = is.CreatedAt.Format(time.RFC3339)
	}
	if is.UpdatedAt != nil {
		out.UpdatedAt = is.UpdatedAt.Format(time.RFC3339)
	}
	return out
}

// toRunnerScopeOutput converts a GitLab API [gl.RunnerControllerRunnerLevelScoping]
// to the MCP tool output format with runner ID and formatted timestamps.
func toRunnerScopeOutput(rs *gl.RunnerControllerRunnerLevelScoping) RunnerScopeOutput {
	out := RunnerScopeOutput{RunnerID: rs.RunnerID}
	if rs.CreatedAt != nil {
		out.CreatedAt = rs.CreatedAt.Format(time.RFC3339)
	}
	if rs.UpdatedAt != nil {
		out.UpdatedAt = rs.UpdatedAt.Format(time.RFC3339)
	}
	return out
}

// ---------------------------------------------------------------------------
// ListScopes
// ---------------------------------------------------------------------------.

// ListInput defines parameters for listing runner controller scopes.
type ListInput struct {
	ControllerID int64 `json:"controller_id" jsonschema:"Runner controller ID,required"`
}

// List retrieves all scopes for a runner controller (admin only).
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ScopesOutput, error) {
	if input.ControllerID <= 0 {
		return ScopesOutput{}, errors.New("controller_id is required and must be > 0")
	}
	if err := ctx.Err(); err != nil {
		return ScopesOutput{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	scopes, _, err := client.GL().RunnerControllerScopes.ListRunnerControllerScopes(input.ControllerID, gl.WithContext(ctx))
	if err != nil {
		return ScopesOutput{}, toolutil.WrapErrWithMessage("list runner controller scopes", err)
	}
	return toScopesOutput(scopes), nil
}

// ---------------------------------------------------------------------------
// AddInstanceScope
// ---------------------------------------------------------------------------.

// AddInstanceScopeInput defines parameters for adding an instance-level scope.
type AddInstanceScopeInput struct {
	ControllerID int64 `json:"controller_id" jsonschema:"Runner controller ID,required"`
}

// AddInstanceScope adds an instance-level scope to a runner controller (admin only).
func AddInstanceScope(ctx context.Context, client *gitlabclient.Client, input AddInstanceScopeInput) (InstanceScopeOutput, error) {
	if input.ControllerID <= 0 {
		return InstanceScopeOutput{}, errors.New("controller_id is required and must be > 0")
	}
	if err := ctx.Err(); err != nil {
		return InstanceScopeOutput{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	is, _, err := client.GL().RunnerControllerScopes.AddRunnerControllerInstanceScope(input.ControllerID, gl.WithContext(ctx))
	if err != nil {
		return InstanceScopeOutput{}, toolutil.WrapErrWithMessage("add instance scope", err)
	}
	return toInstanceScopeOutput(is), nil
}

// ---------------------------------------------------------------------------
// RemoveInstanceScope
// ---------------------------------------------------------------------------.

// RemoveInstanceScopeInput defines parameters for removing an instance-level scope.
type RemoveInstanceScopeInput struct {
	ControllerID int64 `json:"controller_id" jsonschema:"Runner controller ID,required"`
}

// RemoveInstanceScope removes the instance-level scope from a runner controller (admin only).
func RemoveInstanceScope(ctx context.Context, client *gitlabclient.Client, input RemoveInstanceScopeInput) error {
	if input.ControllerID <= 0 {
		return errors.New("controller_id is required and must be > 0")
	}
	if err := ctx.Err(); err != nil {
		return toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	_, err := client.GL().RunnerControllerScopes.RemoveRunnerControllerInstanceScope(input.ControllerID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("remove instance scope", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// AddRunnerScope
// ---------------------------------------------------------------------------.

// AddRunnerScopeInput defines parameters for adding a runner scope.
type AddRunnerScopeInput struct {
	ControllerID int64 `json:"controller_id" jsonschema:"Runner controller ID,required"`
	RunnerID     int64 `json:"runner_id" jsonschema:"Instance-level runner ID to scope,required"`
}

// AddRunnerScope adds a runner scope to a runner controller (admin only).
func AddRunnerScope(ctx context.Context, client *gitlabclient.Client, input AddRunnerScopeInput) (RunnerScopeOutput, error) {
	if input.ControllerID <= 0 {
		return RunnerScopeOutput{}, errors.New("controller_id is required and must be > 0")
	}
	if input.RunnerID <= 0 {
		return RunnerScopeOutput{}, errors.New("runner_id is required and must be > 0")
	}
	if err := ctx.Err(); err != nil {
		return RunnerScopeOutput{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	rs, _, err := client.GL().RunnerControllerScopes.AddRunnerControllerRunnerScope(input.ControllerID, input.RunnerID, gl.WithContext(ctx))
	if err != nil {
		return RunnerScopeOutput{}, toolutil.WrapErrWithMessage("add runner scope", err)
	}
	return toRunnerScopeOutput(rs), nil
}

// ---------------------------------------------------------------------------
// RemoveRunnerScope
// ---------------------------------------------------------------------------.

// RemoveRunnerScopeInput defines parameters for removing a runner scope.
type RemoveRunnerScopeInput struct {
	ControllerID int64 `json:"controller_id" jsonschema:"Runner controller ID,required"`
	RunnerID     int64 `json:"runner_id" jsonschema:"Runner ID to remove from scope,required"`
}

// RemoveRunnerScope removes a runner scope from a runner controller (admin only).
func RemoveRunnerScope(ctx context.Context, client *gitlabclient.Client, input RemoveRunnerScopeInput) error {
	if input.ControllerID <= 0 {
		return errors.New("controller_id is required and must be > 0")
	}
	if input.RunnerID <= 0 {
		return errors.New("runner_id is required and must be > 0")
	}
	if err := ctx.Err(); err != nil {
		return toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	_, err := client.GL().RunnerControllerScopes.RemoveRunnerControllerRunnerScope(input.ControllerID, input.RunnerID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("remove runner scope", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.

// FormatScopesResult formats scopes as an MCP tool result.
func FormatScopesResult(out ScopesOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatScopesMarkdown(out))
}
