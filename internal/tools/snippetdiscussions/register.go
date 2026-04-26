// register.go wires snippetdiscussions MCP tools to the MCP server.

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
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_snippet_discussions",
		Title:       toolutil.TitleFromName("gitlab_list_snippet_discussions"),
		Description: "List discussion threads on a project snippet.\n\nReturns: JSON array of discussions with pagination.\n\nSee also: gitlab_get_snippet_discussion, gitlab_list_project_snippets",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_snippet_discussions", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_snippet_discussion",
		Title:       toolutil.TitleFromName("gitlab_get_snippet_discussion"),
		Description: "Get a single discussion thread on a project snippet.\n\nReturns: JSON with discussion details and notes.\n\nSee also: gitlab_list_snippet_discussions, gitlab_create_snippet_discussion",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_snippet_discussion", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_create_snippet_discussion",
		Title:       toolutil.TitleFromName("gitlab_create_snippet_discussion"),
		Description: "Create a new discussion thread on a project snippet.\n\nReturns: JSON with the created discussion.\n\nSee also: gitlab_list_snippet_discussions, gitlab_add_snippet_discussion_note",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_create_snippet_discussion", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_add_snippet_discussion_note",
		Title:       toolutil.TitleFromName("gitlab_add_snippet_discussion_note"),
		Description: "Add a reply note to an existing snippet discussion thread.\n\nReturns: JSON with the created reply note.\n\nSee also: gitlab_get_snippet_discussion, gitlab_update_snippet_discussion_note",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AddNoteInput) (*mcp.CallToolResult, NoteOutput, error) {
		start := time.Now()
		out, err := AddNote(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_add_snippet_discussion_note", start, err)
		return toolutil.WithHints(FormatNoteMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_update_snippet_discussion_note",
		Title:       toolutil.TitleFromName("gitlab_update_snippet_discussion_note"),
		Description: "Update an existing note in a snippet discussion thread.\n\nReturns: JSON with the updated note details.\n\nSee also: gitlab_get_snippet_discussion, gitlab_delete_snippet_discussion_note",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateNoteInput) (*mcp.CallToolResult, NoteOutput, error) {
		start := time.Now()
		out, err := UpdateNote(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_update_snippet_discussion_note", start, err)
		return toolutil.WithHints(FormatNoteMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_snippet_discussion_note",
		Title:       toolutil.TitleFromName("gitlab_delete_snippet_discussion_note"),
		Description: "Delete a note from a snippet discussion thread.\n\nReturns: JSON confirming note deletion.\n\nSee also: gitlab_list_snippet_discussions, gitlab_add_snippet_discussion_note",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteNoteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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

// RegisterMeta registers the gitlab_snippet_discussion meta-tool.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := toolutil.ActionMap{
		"list":        toolutil.RouteAction(client, List),
		"get":         toolutil.RouteAction(client, Get),
		"create":      toolutil.RouteAction(client, Create),
		"add_note":    toolutil.RouteAction(client, AddNote),
		"update_note": toolutil.RouteAction(client, UpdateNote),
		"delete_note": toolutil.DestructiveVoidAction(client, DeleteNote),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_snippet_discussion",
		Title: toolutil.TitleFromName("gitlab_snippet_discussion"),
		Description: `Manage GitLab snippet discussion threads. Use 'action' to specify the operation.

Actions:
- list: List discussion threads on a snippet. Params: project_id, snippet_id (required), page, per_page
- get: Get a single discussion. Params: project_id, snippet_id, discussion_id (required)
- create: Create a new discussion thread. Params: project_id, snippet_id, body (required)
- add_note: Reply to an existing discussion. Params: project_id, snippet_id, discussion_id, body (required)
- update_note: Update a discussion note. Params: project_id, snippet_id, discussion_id, note_id, body (required)
- delete_note: Delete a discussion note. Params: project_id, snippet_id, discussion_id, note_id (required)`,
		Annotations:  toolutil.DeriveAnnotations(routes),
		Icons:        toolutil.IconDiscussion,
		InputSchema:  toolutil.MetaToolSchema(routes),
		OutputSchema: toolutil.MetaToolOutputSchema(),
	}, toolutil.MakeMetaHandler("gitlab_snippet_discussion", routes, nil))
}
