// register.go wires runners MCP tools to the MCP server.

package runners

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/runnercontrollers"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/runnercontrollerscopes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/runnercontrollertokens"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all runner management MCP tools.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_runner_list",
		Title:       toolutil.TitleFromName("gitlab_runner_list"),
		Description: "List owned CI/CD runners. Filter by type (instance_type, group_type, project_type), status (online, offline, stale, never_contacted), paused state, and tags.\n\nSee also: gitlab_runner_get, gitlab_runner_list_project\n\nReturns: JSON array of runners with pagination. Fields include id, description, status, and runner_type.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconRunner,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_runner_get",
		Title:       toolutil.TitleFromName("gitlab_runner_get"),
		Description: "Get detailed information about a specific CI/CD runner by its ID. Returns description, status, tags, access level, projects, and groups.\n\nSee also: gitlab_runner_list, gitlab_runner_jobs\n\nReturns: JSON with runner details including id, description, status, architecture, and platform.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconRunner,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, DetailsOutput, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatDetailsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_runner_update",
		Title:       toolutil.TitleFromName("gitlab_runner_update"),
		Description: "Update a CI/CD runner's configuration. Modify description, paused state, tags, access level, maximum timeout, and maintenance note.\n\nSee also: gitlab_runner_get, gitlab_runner_list\n\nReturns: JSON with the updated runner details.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconRunner,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, DetailsOutput, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatDetailsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_runner_remove",
		Title:       toolutil.TitleFromName("gitlab_runner_remove"),
		Description: "Remove a CI/CD runner by its ID. This action cannot be undone.\n\nSee also: gitlab_runner_list, gitlab_runner_delete_registered\n\nReturns: confirmation message.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconRunner,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RemoveInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_runner_jobs",
		Title:       toolutil.TitleFromName("gitlab_runner_jobs"),
		Description: "List jobs processed by a specific CI/CD runner. Filter by status (running, success, failed, canceled). Supports sorting and pagination.\n\nSee also: gitlab_runner_get, gitlab_runner_list\n\nReturns: JSON array of jobs run by the runner with pagination.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconRunner,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListJobsInput) (*mcp.CallToolResult, JobListOutput, error) {
		start := time.Now()
		out, err := ListJobs(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_jobs", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatJobListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_runner_list_project",
		Title:       toolutil.TitleFromName("gitlab_runner_list_project"),
		Description: "List CI/CD runners available in a specific project. Filter by type, status, and tags.\n\nSee also: gitlab_runner_enable_project, gitlab_runner_list_group\n\nReturns: JSON array of runners with pagination. Fields include id, description, status, and runner_type.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconRunner,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListProjectInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_list_project", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_runner_enable_project",
		Title:       toolutil.TitleFromName("gitlab_runner_enable_project"),
		Description: "Assign an existing CI/CD runner to a project. Requires project_id and runner_id.\n\nSee also: gitlab_runner_disable_project, gitlab_runner_list_project\n\nReturns: JSON with the runner assignment details.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconRunner,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input EnableProjectInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := EnableProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_enable_project", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_runner_disable_project",
		Title:       toolutil.TitleFromName("gitlab_runner_disable_project"),
		Description: "Remove a CI/CD runner from a project. The runner itself is not deleted.\n\nSee also: gitlab_runner_enable_project, gitlab_runner_list_project\n\nReturns: confirmation message.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconRunner,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DisableProjectInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_runner_list_group",
		Title:       toolutil.TitleFromName("gitlab_runner_list_group"),
		Description: "List CI/CD runners available in a specific group. Filter by type, status, and tags.\n\nSee also: gitlab_runner_list_project, gitlab_runner_list\n\nReturns: JSON array of runners with pagination. Fields include id, description, status, and runner_type.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconRunner,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListGroupInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_list_group", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_runner_register",
		Title:       toolutil.TitleFromName("gitlab_runner_register"),
		Description: "Register a new CI/CD runner with a registration token. Optionally set description, tags, access level, and timeout.\n\nSee also: gitlab_runner_list, gitlab_runner_delete_registered\n\nReturns: JSON with the registered runner details including token.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconRunner,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RegisterInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Register(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_register", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_runner_delete_registered",
		Title:       toolutil.TitleFromName("gitlab_runner_delete_registered"),
		Description: "Delete a registered CI/CD runner by its ID. This action cannot be undone.\n\nSee also: gitlab_runner_register, gitlab_runner_list\n\nReturns: confirmation message.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconRunner,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteByIDInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_runner_verify",
		Title:       toolutil.TitleFromName("gitlab_runner_verify"),
		Description: "Verify a CI/CD runner authentication token. Returns success if the token is valid.\n\nSee also: gitlab_runner_reset_token, gitlab_runner_register\n\nReturns: confirmation that the runner token is valid.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconRunner,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input VerifyInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		err := Verify(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_verify", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.ToolResultWithMarkdown("Runner token is valid."), toolutil.DeleteOutput{Status: "success", Message: "Runner token is valid."}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_runner_reset_token",
		Title:       toolutil.TitleFromName("gitlab_runner_reset_token"),
		Description: "Reset the authentication token for a CI/CD runner. Returns the new token and expiry.\n\nSee also: gitlab_runner_verify, gitlab_runner_get\n\nReturns: JSON with the new authentication token and expiry.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconRunner,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ResetAuthTokenInput) (*mcp.CallToolResult, AuthTokenOutput, error) {
		start := time.Now()
		out, err := ResetAuthToken(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_reset_token", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatAuthTokenMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_runner_list_all",
		Title:       toolutil.TitleFromName("gitlab_runner_list_all"),
		Description: "List all CI/CD runners in the GitLab instance (admin). Filter by type, status, paused state, and tags.\n\nSee also: gitlab_runner_list, gitlab_runner_list_project\n\nReturns: JSON array of runners with pagination. Fields include id, description, status, and runner_type.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconRunner,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListAllInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListAll(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_list_all", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_runner_delete_by_token",
		Title:       toolutil.TitleFromName("gitlab_runner_delete_by_token"),
		Description: "Delete a registered CI/CD runner using its authentication token. This action cannot be undone.\n\nSee also: gitlab_runner_delete_registered, gitlab_runner_verify\n\nReturns: confirmation message.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconRunner,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteByTokenInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_runner_reset_instance_reg_token",
		Title:       toolutil.TitleFromName("gitlab_runner_reset_instance_reg_token"),
		Description: "Reset the instance-level runner registration token. Deprecated: scheduled for removal in GitLab 20.0.\n\nSee also: gitlab_runner_reset_group_reg_token, gitlab_runner_reset_project_reg_token\n\nReturns: JSON with the new registration token.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconRunner,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ResetInstanceRegTokenInput) (*mcp.CallToolResult, AuthTokenOutput, error) {
		start := time.Now()
		out, err := ResetInstanceRegToken(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_reset_instance_reg_token", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatRegTokenMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_runner_reset_group_reg_token",
		Title:       toolutil.TitleFromName("gitlab_runner_reset_group_reg_token"),
		Description: "Reset a group's runner registration token. Deprecated: scheduled for removal in GitLab 20.0.\n\nSee also: gitlab_runner_reset_instance_reg_token, gitlab_runner_reset_project_reg_token\n\nReturns: JSON with the new registration token.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconRunner,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ResetGroupRegTokenInput) (*mcp.CallToolResult, AuthTokenOutput, error) {
		start := time.Now()
		out, err := ResetGroupRegToken(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_reset_group_reg_token", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatRegTokenMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_runner_reset_project_reg_token",
		Title:       toolutil.TitleFromName("gitlab_runner_reset_project_reg_token"),
		Description: "Reset a project's runner registration token. Deprecated: scheduled for removal in GitLab 20.0.\n\nSee also: gitlab_runner_reset_instance_reg_token, gitlab_runner_reset_group_reg_token\n\nReturns: JSON with the new registration token.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconRunner,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ResetProjectRegTokenInput) (*mcp.CallToolResult, AuthTokenOutput, error) {
		start := time.Now()
		out, err := ResetProjectRegToken(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_reset_project_reg_token", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatRegTokenMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_runner_list_managers",
		Title:       toolutil.TitleFromName("gitlab_runner_list_managers"),
		Description: "List all managers (executors) for a specific CI/CD runner. Returns system ID, version, platform, architecture, IP address, and status.\n\nSee also: gitlab_runner_get, gitlab_runner_list\n\nReturns: JSON array of runner managers.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconRunner,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListManagersInput) (*mcp.CallToolResult, ManagerListOutput, error) {
		start := time.Now()
		out, err := ListManagers(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_list_managers", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatManagerListMarkdown(out)), out, err)
	})
}

// RegisterMeta registers the gitlab_runner meta-tool with all runner actions.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := toolutil.ActionMap{
		"list":                     toolutil.RouteAction(client, List),
		"list_all":                 toolutil.RouteAction(client, ListAll),
		"get":                      toolutil.RouteAction(client, Get),
		"update":                   toolutil.RouteAction(client, Update),
		"remove":                   toolutil.DestructiveVoidAction(client, Remove),
		"jobs":                     toolutil.RouteAction(client, ListJobs),
		"list_project":             toolutil.RouteAction(client, ListProject),
		"enable_project":           toolutil.RouteAction(client, EnableProject),
		"disable_project":          toolutil.DestructiveVoidAction(client, DisableProject),
		"list_group":               toolutil.RouteAction(client, ListGroup),
		"register":                 toolutil.RouteAction(client, Register),
		"delete_registered":        toolutil.DestructiveVoidAction(client, DeleteByID),
		"delete_by_token":          toolutil.DestructiveVoidAction(client, DeleteByToken),
		"verify":                   toolutil.RouteVoidAction(client, Verify),
		"reset_token":              toolutil.RouteAction(client, ResetAuthToken),
		"reset_instance_reg_token": toolutil.RouteAction(client, ResetInstanceRegToken),
		"reset_group_reg_token":    toolutil.RouteAction(client, ResetGroupRegToken),
		"reset_project_reg_token":  toolutil.RouteAction(client, ResetProjectRegToken),
		"list_managers":            toolutil.RouteAction(client, ListManagers),
		// Runner controller CRUD (admin, experimental)
		"controller_list":   toolutil.RouteAction(client, runnercontrollers.List),
		"controller_get":    toolutil.RouteAction(client, runnercontrollers.Get),
		"controller_create": toolutil.RouteAction(client, runnercontrollers.Create),
		"controller_update": toolutil.RouteAction(client, runnercontrollers.Update),
		"controller_delete": toolutil.DestructiveVoidAction(client, runnercontrollers.Delete),
		// Runner controller scope management
		"controller_scope_list":            toolutil.RouteAction(client, runnercontrollerscopes.List),
		"controller_scope_add_instance":    toolutil.RouteAction(client, runnercontrollerscopes.AddInstanceScope),
		"controller_scope_remove_instance": toolutil.DestructiveVoidAction(client, runnercontrollerscopes.RemoveInstanceScope),
		"controller_scope_add_runner":      toolutil.RouteAction(client, runnercontrollerscopes.AddRunnerScope),
		"controller_scope_remove_runner":   toolutil.DestructiveVoidAction(client, runnercontrollerscopes.RemoveRunnerScope),
		// Runner controller token management
		"controller_token_list":   toolutil.RouteAction(client, runnercontrollertokens.List),
		"controller_token_get":    toolutil.RouteAction(client, runnercontrollertokens.Get),
		"controller_token_create": toolutil.RouteAction(client, runnercontrollertokens.Create),
		"controller_token_rotate": toolutil.RouteAction(client, runnercontrollertokens.Rotate),
		"controller_token_revoke": toolutil.DestructiveVoidAction(client, runnercontrollertokens.Revoke),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_runner",
		Title: toolutil.TitleFromName("gitlab_runner"),
		Description: `Manage CI/CD runners in GitLab, including runner controllers (admin, experimental). Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- list: List owned runners. Params: type, status, paused (bool), tag_list (comma-separated), page, per_page
- list_all: List all runners (admin). Params: type, status, paused (bool), tag_list (comma-separated), page, per_page
- get: Get runner details. Params: runner_id (required, int)
- update: Update runner. Params: runner_id (required, int), description, paused (bool), tag_list (array), run_untagged (bool), locked (bool), access_level, maximum_timeout (int), maintenance_note
- remove: Remove runner. Params: runner_id (required, int)
- jobs: List runner jobs. Params: runner_id (required, int), status (running/success/failed/canceled), order_by, sort, page, per_page
- list_project: List project runners. Params: project_id (required), type, status, tag_list, page, per_page
- enable_project: Assign runner to project. Params: project_id (required), runner_id (required, int)
- disable_project: Remove runner from project. Params: project_id (required), runner_id (required, int)
- list_group: List group runners. Params: group_id (required), type, status, tag_list, page, per_page
- register: Register new runner. Params: token (required), description, paused (bool), locked (bool), run_untagged (bool), tag_list (array), access_level, maximum_timeout (int), maintenance_note
- delete_registered: Delete registered runner. Params: runner_id (required, int)
- delete_by_token: Delete runner by auth token. Params: token (required)
- verify: Verify runner token. Params: token (required)
- reset_token: Reset runner auth token. Params: runner_id (required, int)
- reset_instance_reg_token: Reset instance runner registration token (deprecated). No params
- reset_group_reg_token: Reset group runner registration token (deprecated). Params: group_id (required)
- reset_project_reg_token: Reset project runner registration token (deprecated). Params: project_id (required)
- list_managers: List all managers for a runner. Params: runner_id (required, int)
- controller_list: List all runner controllers (admin). Params: page, per_page
- controller_get: Get runner controller details (admin). Params: controller_id (required, int)
- controller_create: Register a new runner controller (admin). Params: description, state (enabled/disabled/dry_run)
- controller_update: Update a runner controller (admin). Params: controller_id (required, int), description, state (enabled/disabled/dry_run)
- controller_delete: Delete a runner controller (admin). Params: controller_id (required, int)
- controller_scope_list: List all scopes for a controller. Params: controller_id (required, int)
- controller_scope_add_instance: Add instance-level scope. Params: controller_id (required, int)
- controller_scope_remove_instance: Remove instance-level scope. Params: controller_id (required, int)
- controller_scope_add_runner: Add runner scope. Params: controller_id (required, int), runner_id (required, int)
- controller_scope_remove_runner: Remove runner scope. Params: controller_id (required, int), runner_id (required, int)
- controller_token_list: List all tokens for a controller. Params: controller_id (required, int), page, per_page
- controller_token_get: Get a specific controller token. Params: controller_id (required, int), token_id (required, int)
- controller_token_create: Create a controller token. Params: controller_id (required, int), description
- controller_token_rotate: Rotate a controller token. Params: controller_id (required, int), token_id (required, int)
- controller_token_revoke: Revoke a controller token. Params: controller_id (required, int), token_id (required, int)

Use this tool for managing runner instances, tokens, and runner controllers (admin).
See also: gitlab_pipeline`,
		Annotations: toolutil.DeriveAnnotations(routes),
		Icons:       toolutil.IconRunner,
		InputSchema: toolutil.MetaToolSchema(routes),
	}, toolutil.MakeMetaHandler("gitlab_runner", routes, nil))
}
