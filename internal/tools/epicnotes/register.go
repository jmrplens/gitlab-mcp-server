// register.go wires epicnotes MCP tools to the MCP server.

package epicnotes

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers MCP tools for GitLab epic note operations.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_epic_note_list",
		Title:       toolutil.TitleFromName("gitlab_epic_note_list"),
		Description: "List all comments (notes) on a GitLab group epic via the Work Items GraphQL API. Supports cursor-based pagination.\n\nReturns: JSON with notes array including body, author, timestamps, and system flags. See also: gitlab_epic_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconEpic,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_epic_note_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_epic_note_get",
		Title:       toolutil.TitleFromName("gitlab_epic_note_get"),
		Description: "Get a single comment (note) from a GitLab group epic by its note ID via the Work Items GraphQL API, including author, timestamps, body, and system flag.\n\nReturns: JSON with note details including ID, body, author, and timestamps. See also: gitlab_epic_note_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconEpic,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_epic_note_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_epic_note_create",
		Title:       toolutil.TitleFromName("gitlab_epic_note_create"),
		Description: "Add a comment (note) to a GitLab group epic via the Work Items GraphQL API. Supports Markdown formatting.\n\nReturns: JSON with created note including ID, body, author, and timestamps. See also: gitlab_epic_get.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconEpic,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_epic_note_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_epic_note_update",
		Title:       toolutil.TitleFromName("gitlab_epic_note_update"),
		Description: "Edit the body text of an existing comment on a GitLab group epic via the Work Items GraphQL API. Only the note author or a group maintainer can update a note.\n\nReturns: JSON with updated note including ID, body, author, and timestamps. See also: gitlab_epic_note_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconEpic,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_epic_note_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_epic_note_delete",
		Title:       toolutil.TitleFromName("gitlab_epic_note_delete"),
		Description: "Permanently delete a comment from a GitLab group epic via the Work Items GraphQL API. Only the note author or a group maintainer can delete a note.\n\nReturns: JSON with deletion confirmation. See also: gitlab_epic_note_get.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconEpic,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete note %d from epic &%d in group %q?", input.NoteID, input.IID, input.FullPath)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_epic_note_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("note %d from epic &%d in group %s", input.NoteID, input.IID, input.FullPath))
	})
}
