// register_management.go wires user management MCP tools to the MCP server:
// admin state actions, CRUD, SSH keys, misc, and service accounts.

package users

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const fmtDeletedRow = "- **Deleted**: %s %v\n"

func registerAdminTools(server *mcp.Server, client *gitlabclient.Client) {
	type adminTool struct {
		name   string
		desc   string
		action func(context.Context, *gitlabclient.Client, AdminActionInput) (AdminActionOutput, error)
	}

	tools := []adminTool{
		{
			name:   "gitlab_block_user",
			desc:   "Block a GitLab user, preventing login and API access (admin only). The user's contributions remain intact.\n\nSee also: gitlab_unblock_user, gitlab_get_user\n\nReturns: JSON with action confirmation.",
			action: BlockUser,
		},
		{
			name:   "gitlab_unblock_user",
			desc:   "Unblock a previously blocked GitLab user, restoring login and API access (admin only).\n\nSee also: gitlab_block_user, gitlab_get_user\n\nReturns: JSON with action confirmation.",
			action: UnblockUser,
		},
		{
			name:   "gitlab_ban_user",
			desc:   "Ban a GitLab user, hiding their activity and preventing login (admin only). More restrictive than blocking.\n\nSee also: gitlab_unban_user, gitlab_block_user\n\nReturns: JSON with action confirmation.",
			action: BanUser,
		},
		{
			name:   "gitlab_unban_user",
			desc:   "Unban a previously banned GitLab user (admin only).\n\nSee also: gitlab_ban_user, gitlab_unblock_user\n\nReturns: JSON with action confirmation.",
			action: UnbanUser,
		},
		{
			name:   "gitlab_activate_user",
			desc:   "Activate a deactivated GitLab user (admin only). Deactivated users have been inactive and need reactivation.\n\nSee also: gitlab_deactivate_user, gitlab_get_user\n\nReturns: JSON with action confirmation.",
			action: ActivateUser,
		},
		{
			name:   "gitlab_deactivate_user",
			desc:   "Deactivate an active GitLab user (admin only). Deactivated users cannot login but accounts are preserved.\n\nSee also: gitlab_activate_user, gitlab_block_user\n\nReturns: JSON with action confirmation.",
			action: DeactivateUser,
		},
		{
			name:   "gitlab_approve_user",
			desc:   "Approve a pending GitLab user registration (admin only). Required when user sign-up requires admin approval.\n\nSee also: gitlab_reject_user, gitlab_list_users\n\nReturns: JSON with action confirmation.",
			action: ApproveUser,
		},
		{
			name:   "gitlab_reject_user",
			desc:   "Reject a pending GitLab user registration (admin only). The user account will be deleted.\n\nSee also: gitlab_approve_user, gitlab_list_users\n\nReturns: JSON with action confirmation.",
			action: RejectUser,
		},
		{
			name:   "gitlab_disable_two_factor",
			desc:   "Disable two-factor authentication for a GitLab user (admin only). Use with caution as this reduces account security.\n\nSee also: gitlab_get_user, gitlab_modify_user\n\nReturns: JSON with action confirmation.",
			action: DisableTwoFactor,
		},
	}

	for _, t := range tools {
		tool := t
		annot := toolutil.UpdateAnnotations
		if tool.name == "gitlab_reject_user" {
			annot = toolutil.DeleteAnnotations
		}
		mcp.AddTool(server, &mcp.Tool{
			Name:        tool.name,
			Title:       toolutil.TitleFromName(tool.name),
			Description: tool.desc,
			Annotations: annot,
			Icons:       toolutil.IconUser,
		}, func(ctx context.Context, req *mcp.CallToolRequest, input AdminActionInput) (*mcp.CallToolResult, AdminActionOutput, error) {
			start := time.Now()
			out, err := tool.action(ctx, client, input)
			toolutil.LogToolCallAll(ctx, req, tool.name, start, err)
			return toolutil.ToolResultWithMarkdown(FormatAdminActionMarkdownString(out)), out, err
		})
	}
}

func registerCRUDTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_create_user",
		Title:       toolutil.TitleFromName("gitlab_create_user"),
		Description: "Create a new GitLab user account (admin only). Requires email, name, and username. Supports setting password, admin status, and profile details.\n\nSee also: gitlab_modify_user, gitlab_delete_user\n\nReturns: JSON with created user profile.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_create_user", start, err)
		result := FormatMarkdown(out)
		return result, out, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_modify_user",
		Title:       toolutil.TitleFromName("gitlab_modify_user"),
		Description: "Modify an existing GitLab user account (admin only). Supports updating email, name, username, password, admin status, profile details, and permissions.\n\nSee also: gitlab_create_user, gitlab_get_user\n\nReturns: JSON with updated user profile.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ModifyInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Modify(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_modify_user", start, err)
		result := FormatMarkdown(out)
		return result, out, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_user",
		Title:       toolutil.TitleFromName("gitlab_delete_user"),
		Description: "Delete a GitLab user account (admin only). This permanently removes the user and all their data. Use gitlab_block_user if you want to preserve data.\n\nSee also: gitlab_block_user, gitlab_get_user_associations_count\n\nReturns: JSON with deletion confirmation.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, DeleteOutput, error) {
		start := time.Now()
		out, err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_user", start, err)
		return toolutil.ToolResultWithMarkdown(
			fmt.Sprintf("## User Deleted\n\n"+toolutil.FmtMdID+fmtDeletedRow,
				out.UserID, toolutil.EmojiSuccess, out.Deleted),
		), out, err
	})
}

func registerSSHKeyTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_ssh_keys_for_user",
		Title:       toolutil.TitleFromName("gitlab_list_ssh_keys_for_user"),
		Description: "List SSH keys for a specific GitLab user by user ID. Returns key ID, title, content, usage type, and dates.\n\nSee also: gitlab_list_ssh_keys, gitlab_get_ssh_key_for_user\n\nReturns: JSON array of SSH keys with pagination.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListSSHKeysForUserInput) (*mcp.CallToolResult, SSHKeyListOutput, error) {
		start := time.Now()
		out, err := ListSSHKeysForUser(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_ssh_keys_for_user", start, err)
		return FormatSSHKeyListMarkdown(out), out, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_ssh_key",
		Title:       toolutil.TitleFromName("gitlab_get_ssh_key"),
		Description: "Retrieve a specific SSH key by its ID for the current user.\n\nSee also: gitlab_list_ssh_keys, gitlab_add_ssh_key\n\nReturns: JSON with SSH key details.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetSSHKeyInput) (*mcp.CallToolResult, SSHKeyOutput, error) {
		start := time.Now()
		out, err := GetSSHKey(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_ssh_key", start, err)
		return FormatSSHKeyMarkdown(out), out, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_ssh_key_for_user",
		Title:       toolutil.TitleFromName("gitlab_get_ssh_key_for_user"),
		Description: "Retrieve a specific SSH key for a specific user by user ID and key ID.\n\nSee also: gitlab_list_ssh_keys_for_user, gitlab_add_ssh_key_for_user\n\nReturns: JSON with SSH key details.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetSSHKeyForUserInput) (*mcp.CallToolResult, SSHKeyOutput, error) {
		start := time.Now()
		out, err := GetSSHKeyForUser(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_ssh_key_for_user", start, err)
		return FormatSSHKeyMarkdown(out), out, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_add_ssh_key",
		Title:       toolutil.TitleFromName("gitlab_add_ssh_key"),
		Description: "Add an SSH key to the currently authenticated GitLab user. Requires a title and the public key content.\n\nSee also: gitlab_list_ssh_keys, gitlab_delete_ssh_key\n\nReturns: JSON with the created SSH key details.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AddSSHKeyInput) (*mcp.CallToolResult, SSHKeyOutput, error) {
		start := time.Now()
		out, err := AddSSHKey(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_add_ssh_key", start, err)
		return FormatSSHKeyMarkdown(out), out, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_add_ssh_key_for_user",
		Title:       toolutil.TitleFromName("gitlab_add_ssh_key_for_user"),
		Description: "Add an SSH key to a specific GitLab user (admin only). Requires user ID, title, and public key content.\n\nSee also: gitlab_list_ssh_keys_for_user, gitlab_delete_ssh_key_for_user\n\nReturns: JSON with the created SSH key details.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AddSSHKeyForUserInput) (*mcp.CallToolResult, SSHKeyOutput, error) {
		start := time.Now()
		out, err := AddSSHKeyForUser(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_add_ssh_key_for_user", start, err)
		return FormatSSHKeyMarkdown(out), out, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_ssh_key",
		Title:       toolutil.TitleFromName("gitlab_delete_ssh_key"),
		Description: "Delete an SSH key from the currently authenticated GitLab user.\n\nSee also: gitlab_list_ssh_keys, gitlab_add_ssh_key\n\nReturns: JSON with deletion confirmation.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteSSHKeyInput) (*mcp.CallToolResult, DeleteSSHKeyOutput, error) {
		start := time.Now()
		out, err := DeleteSSHKey(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_ssh_key", start, err)
		return toolutil.ToolResultWithMarkdown(
			fmt.Sprintf("## SSH Key Deleted\n\n"+toolutil.FmtMdID+fmtDeletedRow,
				out.KeyID, toolutil.EmojiSuccess, out.Deleted),
		), out, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_ssh_key_for_user",
		Title:       toolutil.TitleFromName("gitlab_delete_ssh_key_for_user"),
		Description: "Delete an SSH key from a specific GitLab user (admin only).\n\nSee also: gitlab_list_ssh_keys_for_user, gitlab_add_ssh_key_for_user\n\nReturns: JSON with deletion confirmation.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteSSHKeyForUserInput) (*mcp.CallToolResult, DeleteSSHKeyOutput, error) {
		start := time.Now()
		out, err := DeleteSSHKeyForUser(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_ssh_key_for_user", start, err)
		return toolutil.ToolResultWithMarkdown(
			fmt.Sprintf("## SSH Key Deleted\n\n"+toolutil.FmtMdID+fmtDeletedRow,
				out.KeyID, toolutil.EmojiSuccess, out.Deleted),
		), out, err
	})
}

func registerMiscTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_current_user_status",
		Title:       toolutil.TitleFromName("gitlab_current_user_status"),
		Description: "Retrieve the status of the currently authenticated GitLab user. Returns emoji, message, availability, and clear-at time.\n\nSee also: gitlab_set_user_status, gitlab_user_current\n\nReturns: JSON with current user status.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CurrentInput) (*mcp.CallToolResult, StatusOutput, error) {
		start := time.Now()
		out, err := CurrentUserStatus(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_current_user_status", start, err)
		return FormatStatusMarkdown(out), out, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_user_activities",
		Title:       toolutil.TitleFromName("gitlab_get_user_activities"),
		Description: "List last activity dates for GitLab users (admin only). Useful for auditing inactive accounts.\n\nSee also: gitlab_list_users, gitlab_list_user_contribution_events\n\nReturns: JSON array of user activities with pagination.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetUserActivitiesInput) (*mcp.CallToolResult, UserActivitiesOutput, error) {
		start := time.Now()
		out, err := GetUserActivities(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_user_activities", start, err)
		return toolutil.ToolResultWithMarkdown(FormatUserActivitiesMarkdownString(out)), out, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_user_memberships",
		Title:       toolutil.TitleFromName("gitlab_get_user_memberships"),
		Description: "List a user's project and group memberships with access levels (admin only). Useful for auditing user permissions.\n\nSee also: gitlab_get_user, gitlab_get_user_associations_count\n\nReturns: JSON array of memberships with pagination.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetUserMembershipsInput) (*mcp.CallToolResult, UserMembershipsOutput, error) {
		start := time.Now()
		out, err := GetUserMemberships(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_user_memberships", start, err)
		return toolutil.ToolResultWithMarkdown(FormatUserMembershipsMarkdownString(out)), out, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_create_user_runner",
		Title:       toolutil.TitleFromName("gitlab_create_user_runner"),
		Description: "Create a GitLab CI runner linked to the current user. Runners execute CI/CD jobs. Specify runner_type (instance_type, group_type, or project_type).\n\nSee also: gitlab_user_current, gitlab_list_runners\n\nReturns: JSON with runner ID and authentication token.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateUserRunnerInput) (*mcp.CallToolResult, UserRunnerOutput, error) {
		start := time.Now()
		out, err := CreateUserRunner(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_create_user_runner", start, err)
		return toolutil.ToolResultWithMarkdown(FormatUserRunnerMarkdownString(out)), out, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_user_identity",
		Title:       toolutil.TitleFromName("gitlab_delete_user_identity"),
		Description: "Delete a user's identity provider link (e.g., LDAP, SAML) from GitLab (admin only). The user account itself is preserved.\n\nSee also: gitlab_get_user, gitlab_modify_user\n\nReturns: JSON with deletion confirmation.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteUserIdentityInput) (*mcp.CallToolResult, DeleteUserIdentityOutput, error) {
		start := time.Now()
		out, err := DeleteUserIdentity(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_user_identity", start, err)
		return toolutil.ToolResultWithMarkdown(FormatDeleteUserIdentityMarkdownString(out)), out, err
	})
}

func registerServiceAccountTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_create_service_account",
		Title:       toolutil.TitleFromName("gitlab_create_service_account"),
		Description: "Create a new GitLab service account user. Service accounts are machine users for automation.\n\nSee also: gitlab_list_service_accounts\n\nReturns: JSON with the created user details.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateServiceAccountInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := CreateServiceAccount(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_create_service_account", start, err)
		return FormatMarkdown(out), out, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_service_accounts",
		Title:       toolutil.TitleFromName("gitlab_list_service_accounts"),
		Description: "List all GitLab service accounts with optional ordering and pagination.\n\nSee also: gitlab_create_service_account\n\nReturns: JSON array of service accounts.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListServiceAccountsInput) (*mcp.CallToolResult, ServiceAccountListOutput, error) {
		start := time.Now()
		out, err := ListServiceAccounts(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_service_accounts", start, err)
		return toolutil.ToolResultWithMarkdown(FormatServiceAccountListMarkdownString(out)), out, err
	})
}

func registerCurrentUserPATTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_create_current_user_pat",
		Title:       toolutil.TitleFromName("gitlab_create_current_user_pat"),
		Description: "Create a personal access token for the currently authenticated GitLab user. Requires token name and scopes.\n\nSee also: gitlab_user_current\n\nReturns: JSON with the created token (includes the token value).",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateCurrentUserPATInput) (*mcp.CallToolResult, CurrentUserPATOutput, error) {
		start := time.Now()
		out, err := CreateCurrentUserPAT(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_create_current_user_pat", start, err)
		return toolutil.ToolResultWithMarkdown(FormatCurrentUserPATMarkdownString(out)), out, err
	})
}
