// register.go wires securefiles MCP tools to the MCP server.

package securefiles

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all secure file tools with the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_secure_files",
		Title:       toolutil.TitleFromName("gitlab_list_secure_files"),
		Description: "List CI/CD secure files for a GitLab project.\n\nReturns: JSON array of secure files with pagination.\n\nSee also: gitlab_show_secure_file, gitlab_ci_variable_list",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconSecurity,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_secure_files", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_show_secure_file",
		Title:       toolutil.TitleFromName("gitlab_show_secure_file"),
		Description: "Show details of a CI/CD secure file.\n\nReturns: JSON with secure file details.\n\nSee also: gitlab_list_secure_files, gitlab_create_secure_file",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconSecurity,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ShowInput) (*mcp.CallToolResult, SecureFileItem, error) {
		start := time.Now()
		out, err := Show(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_show_secure_file", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatShowMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_create_secure_file",
		Title:       toolutil.TitleFromName("gitlab_create_secure_file"),
		Description: "Create a new CI/CD secure file. Provide either file_path (absolute path to a local file) or content_base64 (base64-encoded content), not both.\n\nReturns: JSON with the created secure file details.\n\nSee also: gitlab_list_secure_files, gitlab_remove_secure_file",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconSecurity,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, SecureFileItem, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_create_secure_file", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatShowMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_remove_secure_file",
		Title:       toolutil.TitleFromName("gitlab_remove_secure_file"),
		Description: "Remove a CI/CD secure file.\n\nReturns: confirmation message.\n\nSee also: gitlab_list_secure_files, gitlab_create_secure_file",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconSecurity,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RemoveInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Remove secure file %d from project %s?", input.FileID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := Remove(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_remove_secure_file", start, err)
		r, o, _ := toolutil.DeleteResult("secure file")
		if err != nil {
			return nil, o, err
		}
		return r, o, nil
	})
}

// RegisterMeta registers the gitlab_secure_file meta-tool.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := toolutil.ActionMap{
		"list":   toolutil.RouteAction(client, List),
		"show":   toolutil.RouteAction(client, Show),
		"create": toolutil.RouteAction(client, Create),
		"remove": toolutil.DestructiveVoidAction(client, Remove),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_secure_file",
		Title: toolutil.TitleFromName("gitlab_secure_file"),
		Description: `Manage CI/CD secure files in GitLab. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- list: List secure files for a project. Params: project_id (required), page, per_page
- show: Show details of a secure file. Params: project_id (required), file_id (required, int)
- create: Create a new secure file. Params: project_id (required), name (required), content (required, base64-encoded)
- remove: Remove a secure file. Params: project_id (required), file_id (required, int)`,
		Annotations:  toolutil.DeriveAnnotations(routes),
		Icons:        toolutil.IconSecurity,
		InputSchema:  toolutil.MetaToolSchema(routes),
		OutputSchema: toolutil.MetaToolOutputSchema(),
	}, toolutil.MakeMetaHandler("gitlab_secure_file", routes, nil))
}
