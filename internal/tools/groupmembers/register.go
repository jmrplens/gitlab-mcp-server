package groupmembers

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all group member individual tools.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := ActionSpecs(client)
	groupMemberTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconUser})
	}

	mcp.AddTool(server, groupMemberTool("gitlab_group_member_get", "Get a single member of a GitLab group by user ID. Returns user details including access level, state, and expiration date.\n\nReturns: JSON with member details including access level, state, and expiration. See also: gitlab_group_members_list."), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetMember(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_member_get", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatMemberMarkdown(out))
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, groupMemberTool("gitlab_group_member_get_inherited", "Get a single inherited member of a GitLab group by user ID. Returns member details including access level inherited from parent groups.\n\nReturns: JSON with inherited member details including access level. See also: gitlab_group_member_get."), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetInheritedMember(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_member_get_inherited", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatMemberMarkdown(out))
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, groupMemberTool("gitlab_group_member_add", "Add a member to a GitLab group. Specify user by user_id or username, and set access level (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer, 50=Owner). Optionally set expiration date.\n\nReturns: JSON with the added member details. See also: gitlab_group_members_list, gitlab_get_user."), func(ctx context.Context, req *mcp.CallToolRequest, input AddInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := AddMember(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_member_add", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatMemberMarkdown(out))
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, groupMemberTool("gitlab_group_member_edit", "Edit a member of a GitLab group. Update access level or expiration date for an existing member.\n\nReturns: JSON with the updated member details. See also: gitlab_group_member_get."), func(ctx context.Context, req *mcp.CallToolRequest, input EditInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := EditMember(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_member_edit", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatMemberMarkdown(out))
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, groupMemberTool("gitlab_group_member_remove", "Remove a member from a GitLab group. Optionally skip subresource removal and unassign issuables.\n\nReturns: JSON confirming member removal. See also: gitlab_group_members_list."), func(ctx context.Context, req *mcp.CallToolRequest, input RemoveInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Remove member (user %d) from group %q?", input.UserID, input.GroupID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := RemoveMember(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_member_remove", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("group member")
	})

	mcp.AddTool(server, groupMemberTool("gitlab_group_share", "Share a GitLab group with another group, granting the shared group a specified access level. Optionally set an expiration date.\n\nReturns: JSON with the group share details. See also: gitlab_group_get."), func(ctx context.Context, req *mcp.CallToolRequest, input ShareInput) (*mcp.CallToolResult, ShareOutput, error) {
		start := time.Now()
		out, err := ShareGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_share", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatShareMarkdown(out))
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, groupMemberTool("gitlab_group_unshare", "Stop sharing a GitLab group with another group, removing the group-level access.\n\nReturns: JSON confirming group unshare.\n\nSee also: gitlab_group_share."), func(ctx context.Context, req *mcp.CallToolRequest, input UnshareInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Stop sharing group %q with group %d?", input.GroupID, input.ShareGroupID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := UnshareGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_unshare", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("group share")
	})
}
