// register.go wires issuediscussions MCP tools to the MCP server.

package issuediscussions

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all issue discussion tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_issue_discussions",
		Title:       toolutil.TitleFromName("gitlab_list_issue_discussions"),
		Description: "List discussion threads on a project issue.\n\nReturns: JSON array of discussions with pagination. See also: gitlab_issue_get, gitlab_issue_note_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_issue_discussions", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_issue_discussion",
		Title:       toolutil.TitleFromName("gitlab_get_issue_discussion"),
		Description: "Get a single discussion thread on a project issue.\n\nReturns: JSON with discussion details and notes. See also: gitlab_list_issue_discussions.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_issue_discussion", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_create_issue_discussion",
		Title:       toolutil.TitleFromName("gitlab_create_issue_discussion"),
		Description: "Create a new discussion thread on a project issue.\n\nReturns: JSON with the created discussion. See also: gitlab_issue_get.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_create_issue_discussion", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_add_issue_discussion_note",
		Title:       toolutil.TitleFromName("gitlab_add_issue_discussion_note"),
		Description: "Add a reply note to an existing issue discussion thread.\n\nReturns: JSON with the created reply note. See also: gitlab_get_issue_discussion.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AddNoteInput) (*mcp.CallToolResult, NoteOutput, error) {
		start := time.Now()
		out, err := AddNote(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_add_issue_discussion_note", start, err)
		return toolutil.WithHints(FormatNoteMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_update_issue_discussion_note",
		Title:       toolutil.TitleFromName("gitlab_update_issue_discussion_note"),
		Description: "Update an existing note in an issue discussion thread.\n\nReturns: JSON with the updated note details. See also: gitlab_get_issue_discussion.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateNoteInput) (*mcp.CallToolResult, NoteOutput, error) {
		start := time.Now()
		out, err := UpdateNote(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_update_issue_discussion_note", start, err)
		return toolutil.WithHints(FormatNoteMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_issue_discussion_note",
		Title:       toolutil.TitleFromName("gitlab_delete_issue_discussion_note"),
		Description: "Delete a note from an issue discussion thread.\n\nReturns: JSON confirming note deletion. See also: gitlab_get_issue_discussion.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteNoteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, "delete issue discussion note"); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := DeleteNote(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_issue_discussion_note", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("issue discussion note")
	})
}

// RegisterMeta registers the gitlab_issue_discussion meta-tool.
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
		Name:  "gitlab_issue_discussion",
		Title: toolutil.TitleFromName("gitlab_issue_discussion"),
		Description: `Manage GitLab issue discussion threads. Use 'action' to specify the operation.

Actions:
- list: List discussion threads on an issue. Params: project_id, issue_iid (required), page, per_page
- get: Get a single discussion. Params: project_id, issue_iid, discussion_id (required)
- create: Create a new discussion thread. Params: project_id, issue_iid, body (required)
- add_note: Reply to an existing discussion. Params: project_id, issue_iid, discussion_id, body (required)
- update_note: Update a discussion note. Params: project_id, issue_iid, discussion_id, note_id, body (required)
- delete_note: Delete a discussion note. Params: project_id, issue_iid, discussion_id, note_id (required)`,
		Annotations: toolutil.DeriveAnnotations(routes),
		Icons:       toolutil.IconDiscussion,
		InputSchema: toolutil.MetaToolSchema(routes),
	}, toolutil.MakeMetaHandler("gitlab_issue_discussion", routes, nil))
}
