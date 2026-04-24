// register.go wires external status check MCP tools to the MCP server.

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
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_set_external_status_check_status",
		Title:       toolutil.TitleFromName("gitlab_set_external_status_check_status"),
		Description: "[DEPRECATED — scheduled for removal in v2.0.0] Set the status of an external status check for a merge request. Use gitlab_set_project_mr_external_status_check_status instead. Requires project_id, mr_iid, sha, external_status_check_id, and status.\n\nReturns: confirmation message. See also: gitlab_list_project_mr_external_status_checks.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconSecurity,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SetStatusInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		err := SetExternalStatusCheckStatus(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_set_external_status_check_status", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("external status check %d status set to %q for MR %d in project %s", input.ExternalStatusCheckID, input.Status, input.MRIID, input.ProjectID))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_project_status_checks",
		Title:       toolutil.TitleFromName("gitlab_list_project_status_checks"),
		Description: "[DEPRECATED — scheduled for removal in v2.0.0] List project-level external status checks. Use gitlab_list_project_external_status_checks instead. Returns: paginated list with ID, name, external URL, HMAC, protected branches.\n\nSee also: gitlab_list_project_external_status_checks, gitlab_create_external_status_check.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconSecurity,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListProjectStatusChecksInput) (*mcp.CallToolResult, ListProjectStatusCheckOutput, error) {
		start := time.Now()
		out, err := ListProjectStatusChecks(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_project_status_checks", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatListProjectMarkdown(out), toolutil.ContentList), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_create_external_status_check",
		Title:       toolutil.TitleFromName("gitlab_create_external_status_check"),
		Description: "[DEPRECATED — scheduled for removal in v2.0.0] Create an external status check for a project. Use gitlab_create_project_external_status_check instead. Requires project_id, name, and external_url.\n\nReturns: confirmation message. See also: gitlab_list_project_status_checks, gitlab_update_external_status_check.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconSecurity,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateLegacyInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		err := CreateExternalStatusCheck(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_create_external_status_check", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("external status check %q created in project %s", input.Name, input.ProjectID))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_external_status_check",
		Title:       toolutil.TitleFromName("gitlab_delete_external_status_check"),
		Description: "[DEPRECATED — scheduled for removal in v2.0.0] Delete an external status check from a project. Use gitlab_delete_project_external_status_check instead. Requires project_id and check_id.\n\nReturns: confirmation message. See also: gitlab_list_project_status_checks.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconSecurity,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteLegacyInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		err := DeleteExternalStatusCheck(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_external_status_check", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("external status check %d from project %s", input.CheckID, input.ProjectID))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_update_external_status_check",
		Title:       toolutil.TitleFromName("gitlab_update_external_status_check"),
		Description: "[DEPRECATED — scheduled for removal in v2.0.0] Update an external status check for a project. Use gitlab_update_project_external_status_check instead. Requires project_id and check_id.\n\nReturns: confirmation message. See also: gitlab_list_project_status_checks, gitlab_create_external_status_check.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconSecurity,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateLegacyInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		err := UpdateExternalStatusCheck(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_update_external_status_check", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("external status check %d updated in project %s", input.CheckID, input.ProjectID))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_retry_failed_status_check_for_mr",
		Title:       toolutil.TitleFromName("gitlab_retry_failed_status_check_for_mr"),
		Description: "[DEPRECATED — scheduled for removal in v2.0.0] Retry a failed external status check for a merge request. Use gitlab_retry_failed_external_status_check_for_project_mr instead. Requires project_id, mr_iid, and check_id.\n\nReturns: confirmation message. See also: gitlab_list_project_mr_external_status_checks.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconSecurity,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RetryLegacyInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		err := RetryFailedStatusCheckForMR(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_retry_failed_status_check_for_mr", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("external status check %d retried for MR %d in project %s", input.CheckID, input.MRIID, input.ProjectID))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_project_mr_external_status_checks",
		Title:       toolutil.TitleFromName("gitlab_list_project_mr_external_status_checks"),
		Description: "List external status checks for a project merge request. Returns: paginated list with ID, name, external URL, status.\n\nSee also: gitlab_set_project_mr_external_status_check_status, gitlab_retry_failed_external_status_check_for_project_mr.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconSecurity,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListProjectMRInput) (*mcp.CallToolResult, ListMergeStatusCheckOutput, error) {
		start := time.Now()
		out, err := ListProjectMRExternalStatusChecks(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_project_mr_external_status_checks", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatListMergeMarkdown(out), toolutil.ContentList), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_project_external_status_checks",
		Title:       toolutil.TitleFromName("gitlab_list_project_external_status_checks"),
		Description: "List external status checks configured for a project. Returns: paginated list with ID, name, external URL, HMAC, protected branches count.\n\nSee also: gitlab_create_project_external_status_check, gitlab_update_project_external_status_check.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconSecurity,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListProjectInput) (*mcp.CallToolResult, ListProjectStatusCheckOutput, error) {
		start := time.Now()
		out, err := ListProjectExternalStatusChecks(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_project_external_status_checks", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatListProjectMarkdown(out), toolutil.ContentList), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_create_project_external_status_check",
		Title:       toolutil.TitleFromName("gitlab_create_project_external_status_check"),
		Description: "Create an external status check for a project. Requires project_id, name, and external_url. Optionally set shared_secret for HMAC and protected_branch_ids.\n\nReturns: created status check with ID, name, external URL, HMAC, protected branches. See also: gitlab_list_project_external_status_checks.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconSecurity,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateProjectInput) (*mcp.CallToolResult, ProjectStatusCheckOutput, error) {
		start := time.Now()
		out, err := CreateProjectExternalStatusCheck(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_create_project_external_status_check", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatProjectCheckMarkdown(out), toolutil.ContentMutate), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_project_external_status_check",
		Title:       toolutil.TitleFromName("gitlab_delete_project_external_status_check"),
		Description: "Delete an external status check from a project. Requires project_id and check_id.\n\nReturns: confirmation message. See also: gitlab_list_project_external_status_checks.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconSecurity,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteProjectInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		err := DeleteProjectExternalStatusCheck(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_project_external_status_check", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("external status check %d from project %s", input.CheckID, input.ProjectID))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_update_project_external_status_check",
		Title:       toolutil.TitleFromName("gitlab_update_project_external_status_check"),
		Description: "Update an external status check for a project. Requires project_id and check_id. Optionally update name, external_url, shared_secret, and protected_branch_ids.\n\nReturns: updated status check with ID, name, external URL, HMAC, protected branches. See also: gitlab_list_project_external_status_checks.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconSecurity,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateProjectInput) (*mcp.CallToolResult, ProjectStatusCheckOutput, error) {
		start := time.Now()
		out, err := UpdateProjectExternalStatusCheck(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_update_project_external_status_check", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatProjectCheckMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_retry_failed_external_status_check_for_project_mr",
		Title:       toolutil.TitleFromName("gitlab_retry_failed_external_status_check_for_project_mr"),
		Description: "Retry a failed external status check for a project merge request. Requires project_id, mr_iid, and check_id.\n\nReturns: confirmation message. See also: gitlab_list_project_mr_external_status_checks.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconSecurity,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RetryProjectInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		err := RetryFailedExternalStatusCheckForProjectMR(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_retry_failed_external_status_check_for_project_mr", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("external status check %d retried for MR %d in project %s", input.CheckID, input.MRIID, input.ProjectID))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_set_project_mr_external_status_check_status",
		Title:       toolutil.TitleFromName("gitlab_set_project_mr_external_status_check_status"),
		Description: "Set the status of an external status check for a project merge request. Requires project_id, mr_iid, sha, external_status_check_id, and status.\n\nReturns: confirmation message. See also: gitlab_list_project_mr_external_status_checks.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconSecurity,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SetProjectStatusInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		err := SetProjectMRExternalStatusCheckStatus(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_set_project_mr_external_status_check_status", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("external status check %d status set to %q for MR %d in project %s", input.ExternalStatusCheckID, input.Status, input.MRIID, input.ProjectID))
	})
}
