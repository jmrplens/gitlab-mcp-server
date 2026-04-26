// register.go wires groupmarkdownuploads MCP tools to the MCP server.

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
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_group_markdown_uploads",
		Title:       toolutil.TitleFromName("gitlab_list_group_markdown_uploads"),
		Description: "List markdown uploads for a group.\n\nReturns: JSON array of uploads with pagination.\n\nSee also: gitlab_delete_group_markdown_upload_by_id, gitlab_upload_project_file",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconUpload,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, *ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_group_markdown_uploads", start, err)
		if err != nil {
			return nil, nil, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatList(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_group_markdown_upload_by_id",
		Title:       toolutil.TitleFromName("gitlab_delete_group_markdown_upload_by_id"),
		Description: "Delete a group markdown upload by ID.\n\nReturns: confirmation message.\n\nSee also: gitlab_list_group_markdown_uploads, gitlab_delete_group_markdown_upload_by_secret",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconUpload,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteByIDInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_group_markdown_upload_by_secret",
		Title:       toolutil.TitleFromName("gitlab_delete_group_markdown_upload_by_secret"),
		Description: "Delete a group markdown upload by secret and filename.\n\nReturns: confirmation message.\n\nSee also: gitlab_list_group_markdown_uploads, gitlab_delete_group_markdown_upload_by_id",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconUpload,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteBySecretAndFilenameInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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

// RegisterMeta registers the gitlab_group_markdown_upload meta-tool.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := toolutil.ActionMap{
		"list":             toolutil.RouteAction(client, List),
		"delete_by_id":     toolutil.DestructiveVoidAction(client, DeleteByID),
		"delete_by_secret": toolutil.DestructiveVoidAction(client, DeleteBySecretAndFilename),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_group_markdown_upload",
		Title: toolutil.TitleFromName("gitlab_group_markdown_upload"),
		Description: `Manage group markdown uploads in GitLab. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- list: List markdown uploads for a group. Params: group_id (required), page, per_page
- delete_by_id: Delete a group markdown upload by ID. Params: group_id (required), upload_id (required, int)
- delete_by_secret: Delete a group markdown upload by secret and filename. Params: group_id (required), secret (required), filename (required)`,
		Annotations:  toolutil.DeriveAnnotations(routes),
		Icons:        toolutil.IconUpload,
		InputSchema:  toolutil.MetaToolSchema(routes),
		OutputSchema: toolutil.MetaToolOutputSchema(routes),
	}, toolutil.MakeMetaHandler("gitlab_group_markdown_upload", routes, nil))
}
