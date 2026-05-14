package snippetnotes

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers MCP tools for GitLab snippet note operations.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := ActionSpecs(client)
	snippetNoteTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconSnippet})
	}

	mcp.AddTool(server, snippetNoteTool("gitlab_snippet_note_list", "List all comments (notes) on a GitLab project snippet. Supports ordering by created_at or updated_at, sort direction, and pagination.\n\nReturns: JSON with notes array including body, author, timestamps, and system flags. See also: gitlab_project_snippet_get."), func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_snippet_note_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, snippetNoteTool("gitlab_snippet_note_get", "Get a single comment (note) from a GitLab project snippet by its note ID, including author, timestamps, body, and system flag.\n\nReturns: JSON with note details including ID, body, author, and timestamps. See also: gitlab_snippet_note_list."), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_snippet_note_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, snippetNoteTool("gitlab_snippet_note_create", "Add a comment (note) to a GitLab project snippet. Supports Markdown formatting.\n\nReturns: JSON with created note including ID, body, author, and timestamps. See also: gitlab_project_snippet_get."), func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_snippet_note_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, snippetNoteTool("gitlab_snippet_note_update", "Edit the body text of an existing comment on a GitLab project snippet. Only the note author or a project maintainer can update a note.\n\nReturns: JSON with updated note including ID, body, author, and timestamps. See also: gitlab_snippet_note_get."), func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_snippet_note_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, snippetNoteTool("gitlab_snippet_note_delete", "Permanently delete a comment from a GitLab project snippet. Only the note author or a project maintainer can delete a note.\n\nReturns: JSON with deletion confirmation. See also: gitlab_snippet_note_get."), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete note %d from snippet %d in project %q?", input.NoteID, input.SnippetID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_snippet_note_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("note %d from snippet %d in project %s", input.NoteID, input.SnippetID, input.ProjectID))
	})
}
