// register.go wires uploads MCP tools to the MCP server.

package uploads

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/elicitation"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers upload-related tools for GitLab projects.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_upload",
		Title:       toolutil.TitleFromName("gitlab_project_upload"),
		Description: "Upload a file to a GitLab project's markdown uploads area. Provide either file_path (absolute path to a local file) or content_base64 (base64-encoded content), not both. Returns a Markdown embed string (e.g. '![alt](/uploads/hash/filename)') that can be inserted into MR descriptions, note bodies, or discussion bodies.\n\nReturns: JSON with the upload URL and Markdown embed string.\n\nSee also: gitlab_project_upload_list, gitlab_project_upload_delete",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconUpload,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UploadInput) (*mcp.CallToolResult, UploadOutput, error) {
		start := time.Now()
		out, err := Upload(ctx, req, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_upload", start, err)
		return toolutil.WithHints(UploadToolResult(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_upload_list",
		Title:       toolutil.TitleFromName("gitlab_project_upload_list"),
		Description: "List all file uploads (markdown attachments) for a GitLab project. Returns upload ID, filename, size, and creation date for each upload.\n\nReturns: JSON array of uploads with pagination.\n\nSee also: gitlab_project_upload",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconUpload,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_upload_list", start, err)
		return nil, out, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_upload_delete",
		Title:       toolutil.TitleFromName("gitlab_project_upload_delete"),
		Description: "Delete a file upload (markdown attachment) from a GitLab project by upload ID. This action cannot be undone.\n\nReturns: confirmation message.\n\nSee also: gitlab_project_upload_list",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconUpload,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := elicitation.ConfirmAction(ctx, req, fmt.Sprintf("Delete upload %d from project %s?", input.UploadID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_upload_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("upload %d from project %s", input.UploadID, input.ProjectID))
	})
}
