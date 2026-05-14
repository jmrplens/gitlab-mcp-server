package groupcredentials

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers MCP tools for GitLab group credential operations.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := ActionSpecs(client)
	groupCredentialTool := func(name, description string, icons []mcp.Icon) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: icons})
	}

	mcp.AddTool(server, groupCredentialTool("gitlab_list_group_personal_access_tokens", "List personal access tokens managed by a GitLab group. Supports filtering by name, state, and revoked status.\n\nReturns: JSON with tokens array. See also: gitlab_revoke_group_personal_access_token.", toolutil.IconToken), func(ctx context.Context, req *mcp.CallToolRequest, input ListPATsInput) (*mcp.CallToolResult, PATListOutput, error) {
		start := time.Now()
		out, err := ListPATs(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_group_personal_access_tokens", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatPATListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, groupCredentialTool("gitlab_list_group_ssh_keys", "List SSH keys managed by a GitLab group.\n\nReturns: JSON with keys array. See also: gitlab_delete_group_ssh_key.", toolutil.IconKey), func(ctx context.Context, req *mcp.CallToolRequest, input ListSSHKeysInput) (*mcp.CallToolResult, SSHKeyListOutput, error) {
		start := time.Now()
		out, err := ListSSHKeys(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_group_ssh_keys", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatSSHKeyListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, groupCredentialTool("gitlab_revoke_group_personal_access_token", "Revoke a personal access token managed by a GitLab group.\n\nReturns: JSON with revocation confirmation. See also: gitlab_list_group_personal_access_tokens.", toolutil.IconToken), func(ctx context.Context, req *mcp.CallToolRequest, input RevokePATInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Revoke personal access token %d from group %q?", input.TokenID, input.GroupID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := RevokePAT(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_revoke_group_personal_access_token", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("personal access token %d from group %s", input.TokenID, input.GroupID))
	})

	mcp.AddTool(server, groupCredentialTool("gitlab_delete_group_ssh_key", "Delete an SSH key managed by a GitLab group.\n\nReturns: JSON with deletion confirmation. See also: gitlab_list_group_ssh_keys.", toolutil.IconKey), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteSSHKeyInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete SSH key %d from group %q?", input.KeyID, input.GroupID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := DeleteSSHKey(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_group_ssh_key", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("SSH key %d from group %s", input.KeyID, input.GroupID))
	})
}
