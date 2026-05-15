package runners

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all runner management MCP tools.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := ActionSpecs(client)
	runnerTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconRunner})
	}

	mcp.AddTool(server, runnerTool("gitlab_runner_list", "List owned CI/CD runners. Filter by type (instance_type, group_type, project_type), status (online, offline, stale, never_contacted), paused state, and tags.\n\nSee also: gitlab_runner_get, gitlab_runner_list_project\n\nReturns: JSON array of runners with pagination. Fields include id, description, status, and runner_type."), func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, runnerTool("gitlab_runner_get", "Get detailed information about a specific CI/CD runner by its ID. Returns description, status, tags, access level, projects, and groups.\n\nSee also: gitlab_runner_list, gitlab_runner_jobs\n\nReturns: JSON with runner details including id, description, status, architecture, and platform."), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, DetailsOutput, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatDetailsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, runnerTool("gitlab_runner_update", "Update a CI/CD runner's configuration. Modify description, paused state, tags, access level, maximum timeout, and maintenance note.\n\nSee also: gitlab_runner_get, gitlab_runner_list\n\nReturns: JSON with the updated runner details."), func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, DetailsOutput, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatDetailsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, runnerTool("gitlab_runner_remove", "Remove a CI/CD runner by its ID. This action cannot be undone.\n\nSee also: gitlab_runner_list, gitlab_runner_delete_registered\n\nReturns: confirmation message."), func(ctx context.Context, req *mcp.CallToolRequest, input RemoveInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Remove runner %d? This cannot be undone.", input.RunnerID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := Remove(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_remove", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("runner")
	})

	mcp.AddTool(server, runnerTool("gitlab_runner_jobs", "List jobs processed by a specific CI/CD runner. Filter by status (running, success, failed, canceled). Supports sorting and pagination.\n\nSee also: gitlab_runner_get, gitlab_runner_list\n\nReturns: JSON array of jobs run by the runner with pagination."), func(ctx context.Context, req *mcp.CallToolRequest, input ListJobsInput) (*mcp.CallToolResult, JobListOutput, error) {
		start := time.Now()
		out, err := ListJobs(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_jobs", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatJobListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, runnerTool("gitlab_runner_list_project", "List CI/CD runners available in a specific project. Filter by type, status, and tags.\n\nSee also: gitlab_runner_enable_project, gitlab_runner_list_group\n\nReturns: JSON array of runners with pagination. Fields include id, description, status, and runner_type."), func(ctx context.Context, req *mcp.CallToolRequest, input ListProjectInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_list_project", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, runnerTool("gitlab_runner_enable_project", "Assign an existing CI/CD runner to a project. Requires project_id and runner_id.\n\nSee also: gitlab_runner_disable_project, gitlab_runner_list_project\n\nReturns: JSON with the runner assignment details."), func(ctx context.Context, req *mcp.CallToolRequest, input EnableProjectInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := EnableProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_enable_project", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, runnerTool("gitlab_runner_disable_project", "Remove a CI/CD runner from a project. The runner itself is not deleted.\n\nSee also: gitlab_runner_enable_project, gitlab_runner_list_project\n\nReturns: confirmation message."), func(ctx context.Context, req *mcp.CallToolRequest, input DisableProjectInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Disable runner %d in project %s?", input.RunnerID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := DisableProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_disable_project", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("project runner assignment")
	})

	mcp.AddTool(server, runnerTool("gitlab_runner_list_group", "List CI/CD runners available in a specific group. Filter by type, status, and tags.\n\nSee also: gitlab_runner_list_project, gitlab_runner_list\n\nReturns: JSON array of runners with pagination. Fields include id, description, status, and runner_type."), func(ctx context.Context, req *mcp.CallToolRequest, input ListGroupInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_list_group", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, runnerTool("gitlab_runner_register", "Register a new CI/CD runner with a registration token. Optionally set description, tags, access level, and timeout.\n\nSee also: gitlab_runner_list, gitlab_runner_delete_registered\n\nReturns: JSON with the registered runner details including token."), func(ctx context.Context, req *mcp.CallToolRequest, input RegisterInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Register(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_register", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, runnerTool("gitlab_runner_delete_registered", "Delete a registered CI/CD runner by its ID. This action cannot be undone.\n\nSee also: gitlab_runner_register, gitlab_runner_list\n\nReturns: confirmation message."), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteByIDInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete registered runner %d? This cannot be undone.", input.RunnerID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := DeleteByID(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_delete_registered", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("registered runner")
	})

	mcp.AddTool(server, runnerTool("gitlab_runner_verify", "Verify a CI/CD runner authentication token. Returns success if the token is valid.\n\nSee also: gitlab_runner_reset_token, gitlab_runner_register\n\nReturns: confirmation that the runner token is valid."), func(ctx context.Context, req *mcp.CallToolRequest, input VerifyInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		err := Verify(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_verify", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.ToolResultWithMarkdown("Runner token is valid."), toolutil.DeleteOutput{Status: "success", Message: "Runner token is valid."}, nil
	})

	mcp.AddTool(server, runnerTool("gitlab_runner_reset_token", "Reset the authentication token for a CI/CD runner. Returns the new token and expiry.\n\nSee also: gitlab_runner_verify, gitlab_runner_get\n\nReturns: JSON with the new authentication token and expiry."), func(ctx context.Context, req *mcp.CallToolRequest, input ResetAuthTokenInput) (*mcp.CallToolResult, AuthTokenOutput, error) {
		start := time.Now()
		out, err := ResetAuthToken(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_reset_token", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatAuthTokenMarkdown(out)), out, err)
	})

	mcp.AddTool(server, runnerTool("gitlab_runner_list_all", "List all CI/CD runners in the GitLab instance (admin). Filter by type, status, paused state, and tags.\n\nSee also: gitlab_runner_list, gitlab_runner_list_project\n\nReturns: JSON array of runners with pagination. Fields include id, description, status, and runner_type."), func(ctx context.Context, req *mcp.CallToolRequest, input ListAllInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListAll(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_list_all", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, runnerTool("gitlab_runner_delete_by_token", "Delete a registered CI/CD runner using its authentication token. This action cannot be undone.\n\nSee also: gitlab_runner_delete_registered, gitlab_runner_verify\n\nReturns: confirmation message."), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteByTokenInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, "Delete runner by authentication token? This cannot be undone."); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := DeleteByToken(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_delete_by_token", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("registered runner")
	})

	mcp.AddTool(server, runnerTool("gitlab_runner_reset_instance_reg_token", "Reset the instance-level runner registration token. Deprecated: scheduled for removal in GitLab 20.0.\n\nSee also: gitlab_runner_reset_group_reg_token, gitlab_runner_reset_project_reg_token\n\nReturns: JSON with the new registration token."), func(ctx context.Context, req *mcp.CallToolRequest, input ResetInstanceRegTokenInput) (*mcp.CallToolResult, AuthTokenOutput, error) {
		start := time.Now()
		out, err := ResetInstanceRegToken(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_reset_instance_reg_token", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatRegTokenMarkdown(out)), out, err)
	})

	mcp.AddTool(server, runnerTool("gitlab_runner_reset_group_reg_token", "Reset a group's runner registration token. Deprecated: scheduled for removal in GitLab 20.0.\n\nSee also: gitlab_runner_reset_instance_reg_token, gitlab_runner_reset_project_reg_token\n\nReturns: JSON with the new registration token."), func(ctx context.Context, req *mcp.CallToolRequest, input ResetGroupRegTokenInput) (*mcp.CallToolResult, AuthTokenOutput, error) {
		start := time.Now()
		out, err := ResetGroupRegToken(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_reset_group_reg_token", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatRegTokenMarkdown(out)), out, err)
	})

	mcp.AddTool(server, runnerTool("gitlab_runner_reset_project_reg_token", "Reset a project's runner registration token. Deprecated: scheduled for removal in GitLab 20.0.\n\nSee also: gitlab_runner_reset_instance_reg_token, gitlab_runner_reset_group_reg_token\n\nReturns: JSON with the new registration token."), func(ctx context.Context, req *mcp.CallToolRequest, input ResetProjectRegTokenInput) (*mcp.CallToolResult, AuthTokenOutput, error) {
		start := time.Now()
		out, err := ResetProjectRegToken(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_reset_project_reg_token", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatRegTokenMarkdown(out)), out, err)
	})

	mcp.AddTool(server, runnerTool("gitlab_runner_list_managers", "List all managers (executors) for a specific CI/CD runner. Returns system ID, version, platform, architecture, IP address, and status.\n\nSee also: gitlab_runner_get, gitlab_runner_list\n\nReturns: JSON array of runner managers."), func(ctx context.Context, req *mcp.CallToolRequest, input ListManagersInput) (*mcp.CallToolResult, ManagerListOutput, error) {
		start := time.Now()
		out, err := ListManagers(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_list_managers", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatManagerListMarkdown(out)), out, err)
	})
}
