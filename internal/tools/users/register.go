package users

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers tools for user-related operations.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := individualActionSpecs(client, false)
	registerCoreTools(server, client, specs)
	registerAdminTools(server, client, specs)
	registerCRUDTools(server, client, specs)
	registerSSHKeyTools(server, client, specs)
	registerMiscTools(server, client, specs)
	registerCurrentUserPATTools(server, client, specs)
}

// RegisterEnterpriseTools registers enterprise-only user tools (service accounts).
func RegisterEnterpriseTools(server *mcp.Server, client *gitlabclient.Client) {
	registerServiceAccountTools(server, client, individualActionSpecs(client, true))
}

func individualActionSpecs(client *gitlabclient.Client, enterprise bool) []toolutil.ActionSpec {
	specs := ActionSpecs(client, enterprise)
	out := make([]toolutil.ActionSpec, 0, len(specs))
	for _, spec := range specs {
		if spec.Name == "me" && spec.IndividualTool.Name == "gitlab_user_current" {
			continue
		}
		out = append(out, spec)
	}
	return out
}

func userTool(specs []toolutil.ActionSpec, name, description string) *mcp.Tool {
	return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconUser})
}

func registerCoreTools(server *mcp.Server, client *gitlabclient.Client, specs []toolutil.ActionSpec) {
	mcp.AddTool(server, userTool(specs, "gitlab_user_current", "Retrieve information about the currently authenticated GitLab user. Returns user ID, username, name, email, state, avatar URL, web URL, and admin status. Useful for confirming identity and permissions.\n\nSee also: gitlab_get_user, gitlab_list_users\n\nReturns: JSON with current user profile details."), func(ctx context.Context, req *mcp.CallToolRequest, input CurrentInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Current(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_user_current", start, err)
		result := FormatMarkdown(out)
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, userTool(specs, "gitlab_list_users", "List GitLab users with optional filters. Supports search by name/username/email, filtering by active/blocked/external status, ordering, and pagination. Useful for finding users or auditing accounts.\n\nSee also: gitlab_get_user, gitlab_user_current\n\nReturns: JSON array of users with pagination."), func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_users", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, userTool(specs, "gitlab_get_user", "Retrieve detailed information about a specific GitLab user by their ID. Returns profile details including username, email, state, bio, and admin status.\n\nSee also: gitlab_list_users, gitlab_get_user_status\n\nReturns: JSON with user profile details."), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		if err != nil && toolutil.IsHTTPStatus(err, 404) {
			toolutil.LogToolCallAll(ctx, req, "gitlab_get_user", start, nil)
			return toolutil.NotFoundResult("User", fmt.Sprintf("ID %d", input.UserID),
				"Use gitlab_list_users to search users by username or email",
				"The user may have been blocked or deleted",
			), Output{}, nil
		}
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_user", start, err)
		result := FormatMarkdown(out)
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, userTool(specs, "gitlab_get_user_status", "Retrieve the status of a specific GitLab user. Returns emoji, message, availability, and clear-at time.\n\nSee also: gitlab_set_user_status, gitlab_get_user\n\nReturns: JSON with user status including emoji, message, and availability."), func(ctx context.Context, req *mcp.CallToolRequest, input GetStatusInput) (*mcp.CallToolResult, StatusOutput, error) {
		start := time.Now()
		out, err := GetStatus(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_user_status", start, err)
		return toolutil.WithHints(FormatStatusMarkdown(out), out, err)
	})

	mcp.AddTool(server, userTool(specs, "gitlab_set_user_status", "Set the status of the currently authenticated GitLab user. Supports setting emoji, message, availability (not_set/busy), and auto-clear duration.\n\nSee also: gitlab_get_user_status, gitlab_user_current\n\nReturns: JSON with the updated user status."), func(ctx context.Context, req *mcp.CallToolRequest, input SetStatusInput) (*mcp.CallToolResult, StatusOutput, error) {
		start := time.Now()
		out, err := SetStatus(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_set_user_status", start, err)
		return toolutil.WithHints(FormatStatusMarkdown(out), out, err)
	})

	mcp.AddTool(server, userTool(specs, "gitlab_list_ssh_keys", "List SSH keys for the currently authenticated GitLab user. Returns key ID, title, key content, usage type, and creation/expiration dates.\n\nSee also: gitlab_user_current, gitlab_deploy_key_list_project\n\nReturns: JSON array of SSH keys with pagination."), func(ctx context.Context, req *mcp.CallToolRequest, input ListSSHKeysInput) (*mcp.CallToolResult, SSHKeyListOutput, error) {
		start := time.Now()
		out, err := ListSSHKeys(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_ssh_keys", start, err)
		return toolutil.WithHints(FormatSSHKeyListMarkdown(out), out, err)
	})

	mcp.AddTool(server, userTool(specs, "gitlab_list_emails", "List email addresses for the currently authenticated GitLab user. Returns email ID, address, and confirmation status.\n\nSee also: gitlab_user_current, gitlab_list_users\n\nReturns: JSON array of user email addresses."), func(ctx context.Context, req *mcp.CallToolRequest, input ListEmailsInput) (*mcp.CallToolResult, EmailListOutput, error) {
		start := time.Now()
		out, err := ListEmails(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_emails", start, err)
		return toolutil.WithHints(FormatEmailListMarkdown(out), out, err)
	})

	mcp.AddTool(server, userTool(specs, "gitlab_list_user_contribution_events", "List contribution events for a specific GitLab user. Returns events with action type, target information, and timestamps. Supports filtering by action, target type, date range, and pagination.\n\nSee also: gitlab_get_user, gitlab_list_user_contribution_events\n\nReturns: JSON array of contribution events with pagination."), func(ctx context.Context, req *mcp.CallToolRequest, input ListContributionEventsInput) (*mcp.CallToolResult, ContributionEventsOutput, error) {
		start := time.Now()
		out, err := ListContributionEvents(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_user_contribution_events", start, err)
		return toolutil.WithHints(FormatContributionEventsMarkdown(out), out, err)
	})

	mcp.AddTool(server, userTool(specs, "gitlab_get_user_associations_count", "Get the count of a user's associations including groups, projects, issues, and merge requests. Useful for understanding user activity scope before account management operations.\n\nSee also: gitlab_get_user, gitlab_list_user_contribution_events\n\nReturns: JSON with counts of groups, projects, issues, and merge requests."), func(ctx context.Context, req *mcp.CallToolRequest, input GetAssociationsCountInput) (*mcp.CallToolResult, AssociationsCountOutput, error) {
		start := time.Now()
		out, err := GetAssociationsCount(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_user_associations_count", start, err)
		return toolutil.WithHints(FormatAssociationsCountMarkdown(out), out, err)
	})
}
