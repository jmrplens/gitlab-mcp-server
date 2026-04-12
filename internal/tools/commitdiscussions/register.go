// register.go wires commitdiscussions MCP tools to the MCP server.

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
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_commit_discussions",
		Title:       toolutil.TitleFromName("gitlab_list_commit_discussions"),
		Description: "List discussion threads on a project commit.\n\nReturns: JSON with discussion threads including notes, authors, and positions.\n\nSee also: gitlab_create_commit_discussion, gitlab_get_commit",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_commit_discussions", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_commit_discussion",
		Title:       toolutil.TitleFromName("gitlab_get_commit_discussion"),
		Description: "Get a single discussion thread on a project commit.\n\nReturns: JSON with discussion thread details including all notes and positions.\n\nSee also: gitlab_list_commit_discussions, gitlab_add_commit_discussion_note",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_commit_discussion", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_create_commit_discussion",
		Title:       toolutil.TitleFromName("gitlab_create_commit_discussion"),
		Description: "Create a new discussion thread on a project commit. Supports inline diff comments via position.\n\nReturns: JSON with created discussion thread including ID and initial note.\n\nSee also: gitlab_list_commit_discussions, gitlab_get_commit",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_create_commit_discussion", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_add_commit_discussion_note",
		Title:       toolutil.TitleFromName("gitlab_add_commit_discussion_note"),
		Description: "Add a reply note to an existing commit discussion thread.\n\nReturns: JSON with created note including ID, body, and author.\n\nSee also: gitlab_get_commit_discussion, gitlab_update_commit_discussion_note",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AddNoteInput) (*mcp.CallToolResult, NoteOutput, error) {
		start := time.Now()
		out, err := AddNote(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_add_commit_discussion_note", start, err)
		return toolutil.WithHints(FormatNoteMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_update_commit_discussion_note",
		Title:       toolutil.TitleFromName("gitlab_update_commit_discussion_note"),
		Description: "Update an existing note in a commit discussion thread.\n\nReturns: JSON with updated note including ID, body, and author.\n\nSee also: gitlab_get_commit_discussion, gitlab_delete_commit_discussion_note",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateNoteInput) (*mcp.CallToolResult, NoteOutput, error) {
		start := time.Now()
		out, err := UpdateNote(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_update_commit_discussion_note", start, err)
		return toolutil.WithHints(FormatNoteMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_commit_discussion_note",
		Title:       toolutil.TitleFromName("gitlab_delete_commit_discussion_note"),
		Description: "Delete a note from a commit discussion thread.\n\nReturns: JSON with deletion confirmation.\n\nSee also: gitlab_list_commit_discussions, gitlab_add_commit_discussion_note",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteNoteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		toolutil.ConfirmAction(ctx, req, "delete commit discussion note")
		err := DeleteNote(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_commit_discussion_note", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("commit discussion note")
	})
}

// RegisterMeta registers the gitlab_commit_discussion meta-tool.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]toolutil.ActionFunc{
		"list":        toolutil.WrapAction(client, List),
		"get":         toolutil.WrapAction(client, Get),
		"create":      toolutil.WrapAction(client, Create),
		"add_note":    toolutil.WrapAction(client, AddNote),
		"update_note": toolutil.WrapAction(client, UpdateNote),
		"delete_note": toolutil.WrapVoidAction(client, DeleteNote),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_commit_discussion",
		Title: toolutil.TitleFromName("gitlab_commit_discussion"),
		Description: `Manage GitLab commit discussion threads. Use 'action' to specify the operation.

Actions:
- list: List discussion threads on a commit. Params: project_id, commit_sha (required), page, per_page
- get: Get a single discussion. Params: project_id, commit_sha, discussion_id (required)
- create: Create a new discussion thread. Params: project_id, commit_sha, body (required), position (optional for inline diff)
- add_note: Reply to an existing discussion. Params: project_id, commit_sha, discussion_id, body (required)
- update_note: Update a discussion note. Params: project_id, commit_sha, discussion_id, note_id, body (required)
- delete_note: Delete a discussion note. Params: project_id, commit_sha, discussion_id, note_id (required)`,
		Annotations: toolutil.MetaAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, toolutil.MakeMetaHandler("gitlab_commit_discussion", routes, nil))
}
