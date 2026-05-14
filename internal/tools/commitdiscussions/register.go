package commitdiscussions

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all commit discussion tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := ActionSpecs(client)
	commitDiscussionTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconDiscussion})
	}

	mcp.AddTool(server, commitDiscussionTool("gitlab_list_commit_discussions", "List discussion threads on a project commit.\n\nReturns: JSON with discussion threads including notes, authors, and positions.\n\nSee also: gitlab_create_commit_discussion, gitlab_commit_get"), func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_commit_discussions", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, commitDiscussionTool("gitlab_get_commit_discussion", "Get a single discussion thread on a project commit.\n\nReturns: JSON with discussion thread details including all notes and positions.\n\nSee also: gitlab_list_commit_discussions, gitlab_add_commit_discussion_note"), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_commit_discussion", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, commitDiscussionTool("gitlab_create_commit_discussion", "Create a new discussion thread on a project commit. Supports inline diff comments via position.\n\nReturns: JSON with created discussion thread including ID and initial note.\n\nSee also: gitlab_list_commit_discussions, gitlab_commit_get"), func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_create_commit_discussion", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, commitDiscussionTool("gitlab_add_commit_discussion_note", "Add a reply note to an existing commit discussion thread.\n\nReturns: JSON with created note including ID, body, and author.\n\nSee also: gitlab_get_commit_discussion, gitlab_update_commit_discussion_note"), func(ctx context.Context, req *mcp.CallToolRequest, input AddNoteInput) (*mcp.CallToolResult, NoteOutput, error) {
		start := time.Now()
		out, err := AddNote(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_add_commit_discussion_note", start, err)
		return toolutil.WithHints(FormatNoteMarkdown(out), out, err)
	})

	mcp.AddTool(server, commitDiscussionTool("gitlab_update_commit_discussion_note", "Update an existing note in a commit discussion thread.\n\nReturns: JSON with updated note including ID, body, and author.\n\nSee also: gitlab_get_commit_discussion, gitlab_delete_commit_discussion_note"), func(ctx context.Context, req *mcp.CallToolRequest, input UpdateNoteInput) (*mcp.CallToolResult, NoteOutput, error) {
		start := time.Now()
		out, err := UpdateNote(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_update_commit_discussion_note", start, err)
		return toolutil.WithHints(FormatNoteMarkdown(out), out, err)
	})

	mcp.AddTool(server, commitDiscussionTool("gitlab_delete_commit_discussion_note", "Delete a note from a commit discussion thread.\n\nReturns: JSON with deletion confirmation.\n\nSee also: gitlab_list_commit_discussions, gitlab_add_commit_discussion_note"), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteNoteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, "delete commit discussion note"); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := DeleteNote(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_commit_discussion_note", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("commit discussion note")
	})
}
