package epicdiscussions

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all epic discussion tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := ActionSpecs(client)
	epicDiscussionTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconDiscussion})
	}

	mcp.AddTool(server, epicDiscussionTool("gitlab_list_epic_discussions", "List discussion threads on a group epic via the Work Items GraphQL API.\n\nReturns: JSON with discussion threads including notes and authors.\n\nSee also: gitlab_create_epic_discussion, gitlab_group_list"), func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_epic_discussions", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, epicDiscussionTool("gitlab_get_epic_discussion", "Get a single discussion thread on a group epic via the Work Items GraphQL API.\n\nReturns: JSON with discussion thread details including all notes.\n\nSee also: gitlab_list_epic_discussions, gitlab_add_epic_discussion_note"), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_epic_discussion", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, epicDiscussionTool("gitlab_create_epic_discussion", "Create a new discussion thread on a group epic via the Work Items GraphQL API.\n\nReturns: JSON with created discussion thread including ID and initial note.\n\nSee also: gitlab_list_epic_discussions, gitlab_add_epic_discussion_note"), func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_create_epic_discussion", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, epicDiscussionTool("gitlab_add_epic_discussion_note", "Add a reply note to an existing epic discussion thread via the Work Items GraphQL API.\n\nReturns: JSON with created note including ID, body, and author.\n\nSee also: gitlab_get_epic_discussion, gitlab_update_epic_discussion_note"), func(ctx context.Context, req *mcp.CallToolRequest, input AddNoteInput) (*mcp.CallToolResult, NoteOutput, error) {
		start := time.Now()
		out, err := AddNote(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_add_epic_discussion_note", start, err)
		return toolutil.WithHints(FormatNoteMarkdown(out), out, err)
	})

	mcp.AddTool(server, epicDiscussionTool("gitlab_update_epic_discussion_note", "Update an existing note in an epic discussion thread via the Work Items GraphQL API.\n\nReturns: JSON with updated note including ID, body, and author.\n\nSee also: gitlab_get_epic_discussion, gitlab_delete_epic_discussion_note"), func(ctx context.Context, req *mcp.CallToolRequest, input UpdateNoteInput) (*mcp.CallToolResult, NoteOutput, error) {
		start := time.Now()
		out, err := UpdateNote(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_update_epic_discussion_note", start, err)
		return toolutil.WithHints(FormatNoteMarkdown(out), out, err)
	})

	mcp.AddTool(server, epicDiscussionTool("gitlab_delete_epic_discussion_note", "Delete a note from an epic discussion thread via the Work Items GraphQL API.\n\nReturns: JSON with deletion confirmation.\n\nSee also: gitlab_list_epic_discussions, gitlab_add_epic_discussion_note"), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteNoteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, "delete epic discussion note"); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := DeleteNote(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_epic_discussion_note", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("epic discussion note")
	})
}
