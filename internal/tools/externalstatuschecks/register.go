package externalstatuschecks

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all external status check tools on the given MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := ActionSpecs(client)
	statusCheckTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconSecurity})
	}

	mcp.AddTool(server, statusCheckTool("gitlab_list_project_status_checks", "List project-level external status checks. Returns: paginated list with ID, name, external URL, HMAC, protected branches.\n\nSee also: gitlab_list_project_external_status_checks, gitlab_create_project_external_status_check."), func(ctx context.Context, req *mcp.CallToolRequest, input ListProjectStatusChecksInput) (*mcp.CallToolResult, ListProjectStatusCheckOutput, error) {
		start := time.Now()
		out, err := ListProjectStatusChecks(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_project_status_checks", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatListProjectMarkdown(out), toolutil.ContentList), out, err)
	})

	mcp.AddTool(server, statusCheckTool("gitlab_list_project_mr_external_status_checks", "List external status checks for a project merge request. Returns: paginated list with ID, name, external URL, status.\n\nSee also: gitlab_set_project_mr_external_status_check_status, gitlab_retry_failed_external_status_check_for_project_mr."), func(ctx context.Context, req *mcp.CallToolRequest, input ListProjectMRInput) (*mcp.CallToolResult, ListMergeStatusCheckOutput, error) {
		start := time.Now()
		out, err := ListProjectMRExternalStatusChecks(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_project_mr_external_status_checks", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatListMergeMarkdown(out), toolutil.ContentList), out, err)
	})

	mcp.AddTool(server, statusCheckTool("gitlab_list_project_external_status_checks", "List external status checks configured for a project. Returns: paginated list with ID, name, external URL, HMAC, protected branches count.\n\nSee also: gitlab_create_project_external_status_check, gitlab_update_project_external_status_check."), func(ctx context.Context, req *mcp.CallToolRequest, input ListProjectInput) (*mcp.CallToolResult, ListProjectStatusCheckOutput, error) {
		start := time.Now()
		out, err := ListProjectExternalStatusChecks(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_project_external_status_checks", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatListProjectMarkdown(out), toolutil.ContentList), out, err)
	})

	mcp.AddTool(server, statusCheckTool("gitlab_create_project_external_status_check", "Create an external status check for a project. Requires project_id, name, and external_url. Optionally set shared_secret for HMAC and protected_branch_ids.\n\nReturns: created status check with ID, name, external URL, HMAC, protected branches. See also: gitlab_list_project_external_status_checks."), func(ctx context.Context, req *mcp.CallToolRequest, input CreateProjectInput) (*mcp.CallToolResult, ProjectStatusCheckOutput, error) {
		start := time.Now()
		out, err := CreateProjectExternalStatusCheck(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_create_project_external_status_check", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatProjectCheckMarkdown(out), toolutil.ContentMutate), out, err)
	})

	mcp.AddTool(server, statusCheckTool("gitlab_delete_project_external_status_check", "Delete an external status check from a project. Requires project_id and check_id.\n\nReturns: confirmation message. See also: gitlab_list_project_external_status_checks."), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteProjectInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		err := DeleteProjectExternalStatusCheck(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_project_external_status_check", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("external status check %d from project %s", input.CheckID, input.ProjectID))
	})

	mcp.AddTool(server, statusCheckTool("gitlab_update_project_external_status_check", "Update an external status check for a project. Requires project_id and check_id. Optionally update name, external_url, shared_secret, and protected_branch_ids.\n\nReturns: updated status check with ID, name, external URL, HMAC, protected branches. See also: gitlab_list_project_external_status_checks."), func(ctx context.Context, req *mcp.CallToolRequest, input UpdateProjectInput) (*mcp.CallToolResult, ProjectStatusCheckOutput, error) {
		start := time.Now()
		out, err := UpdateProjectExternalStatusCheck(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_update_project_external_status_check", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatProjectCheckMarkdown(out)), out, err)
	})

	mcp.AddTool(server, statusCheckTool("gitlab_retry_failed_external_status_check_for_project_mr", "Retry a failed external status check for a project merge request. Requires project_id, merge_request_iid, and check_id.\n\nReturns: confirmation message. See also: gitlab_list_project_mr_external_status_checks."), func(ctx context.Context, req *mcp.CallToolRequest, input RetryProjectInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		err := RetryFailedExternalStatusCheckForProjectMR(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_retry_failed_external_status_check_for_project_mr", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("external status check %d retried for MR %d in project %s", input.CheckID, input.MRIID, input.ProjectID))
	})

	mcp.AddTool(server, statusCheckTool("gitlab_set_project_mr_external_status_check_status", "Set the status of an external status check for a project merge request. Requires project_id, merge_request_iid, sha, external_status_check_id, and status.\n\nReturns: confirmation message. See also: gitlab_list_project_mr_external_status_checks."), func(ctx context.Context, req *mcp.CallToolRequest, input SetProjectStatusInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		err := SetProjectMRExternalStatusCheckStatus(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_set_project_mr_external_status_check_status", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("external status check %d status set to %q for MR %d in project %s", input.ExternalStatusCheckID, input.Status, input.MRIID, input.ProjectID))
	})
}
