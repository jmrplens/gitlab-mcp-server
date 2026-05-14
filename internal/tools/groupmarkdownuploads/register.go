package groupmarkdownuploads

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all group markdown upload tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := ActionSpecs(client)
	groupMarkdownUploadTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconUpload})
	}

	mcp.AddTool(server, groupMarkdownUploadTool("gitlab_list_group_markdown_uploads", "List markdown uploads for a group.\n\nReturns: JSON array of uploads with pagination.\n\nSee also: gitlab_delete_group_markdown_upload_by_id, gitlab_upload_project_file"), func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, *ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_group_markdown_uploads", start, err)
		if err != nil {
			return nil, nil, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatList(out)), out, nil)
	})

	mcp.AddTool(server, groupMarkdownUploadTool("gitlab_delete_group_markdown_upload_by_id", "Delete a group markdown upload by ID.\n\nReturns: confirmation message.\n\nSee also: gitlab_list_group_markdown_uploads, gitlab_delete_group_markdown_upload_by_secret"), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteByIDInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete group markdown upload %d from group %s?", input.UploadID, input.GroupID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := DeleteByID(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_group_markdown_upload_by_id", start, err)
		r, o, _ := toolutil.DeleteResult("group markdown upload")
		if err != nil {
			return nil, o, err
		}
		return r, o, nil
	})

	mcp.AddTool(server, groupMarkdownUploadTool("gitlab_delete_group_markdown_upload_by_secret", "Delete a group markdown upload by secret and filename.\n\nReturns: confirmation message.\n\nSee also: gitlab_list_group_markdown_uploads, gitlab_delete_group_markdown_upload_by_id"), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteBySecretAndFilenameInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete group markdown upload %s from group %s?", input.Filename, input.GroupID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := DeleteBySecretAndFilename(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_group_markdown_upload_by_secret", start, err)
		r, o, _ := toolutil.DeleteResult("group markdown upload")
		if err != nil {
			return nil, o, err
		}
		return r, o, nil
	})
}
