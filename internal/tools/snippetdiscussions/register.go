package snippetdiscussions

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all snippet discussion tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := ActionSpecs(client)
	snippetDiscussionTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconDiscussion})
	}

	mcp.AddTool(server, snippetDiscussionTool("gitlab_list_snippet_discussions", "List discussion threads on a project snippet.\n\nReturns: JSON array of discussions with pagination.\n\nSee also: gitlab_get_snippet_discussion, gitlab_project_snippet_list"), func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_snippet_discussions", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, snippetDiscussionTool("gitlab_get_snippet_discussion", "Get a single discussion thread on a project snippet.\n\nReturns: JSON with discussion details and notes.\n\nSee also: gitlab_list_snippet_discussions, gitlab_create_snippet_discussion"), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_snippet_discussion", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, snippetDiscussionTool("gitlab_create_snippet_discussion", "Create a new discussion thread on a project snippet.\n\nReturns: JSON with the created discussion.\n\nSee also: gitlab_list_snippet_discussions, gitlab_add_snippet_discussion_note"), func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_create_snippet_discussion", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, snippetDiscussionTool("gitlab_add_snippet_discussion_note", "Add a reply note to an existing snippet discussion thread.\n\nReturns: JSON with the created reply note.\n\nSee also: gitlab_get_snippet_discussion, gitlab_update_snippet_discussion_note"), func(ctx context.Context, req *mcp.CallToolRequest, input AddNoteInput) (*mcp.CallToolResult, NoteOutput, error) {
		start := time.Now()
		out, err := AddNote(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_add_snippet_discussion_note", start, err)
		return toolutil.WithHints(FormatNoteMarkdown(out), out, err)
	})

	mcp.AddTool(server, snippetDiscussionTool("gitlab_update_snippet_discussion_note", "Update an existing note in a snippet discussion thread.\n\nReturns: JSON with the updated note details.\n\nSee also: gitlab_get_snippet_discussion, gitlab_delete_snippet_discussion_note"), func(ctx context.Context, req *mcp.CallToolRequest, input UpdateNoteInput) (*mcp.CallToolResult, NoteOutput, error) {
		start := time.Now()
		out, err := UpdateNote(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_update_snippet_discussion_note", start, err)
		return toolutil.WithHints(FormatNoteMarkdown(out), out, err)
	})

	mcp.AddTool(server, snippetDiscussionTool("gitlab_delete_snippet_discussion_note", "Delete a note from a snippet discussion thread.\n\nReturns: JSON confirming note deletion.\n\nSee also: gitlab_list_snippet_discussions, gitlab_add_snippet_discussion_note"), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteNoteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, "delete snippet discussion note"); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := DeleteNote(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_snippet_discussion_note", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("snippet discussion note")
	})
}
