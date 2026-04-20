// register.go wires epicdiscussions MCP tools to the MCP server.

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
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_epic_discussions",
		Title:       toolutil.TitleFromName("gitlab_list_epic_discussions"),
		Description: "List discussion threads on a group epic via the Work Items GraphQL API.\n\nReturns: JSON with discussion threads including notes and authors.\n\nSee also: gitlab_create_epic_discussion, gitlab_list_groups",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_epic_discussions", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_epic_discussion",
		Title:       toolutil.TitleFromName("gitlab_get_epic_discussion"),
		Description: "Get a single discussion thread on a group epic via the Work Items GraphQL API.\n\nReturns: JSON with discussion thread details including all notes.\n\nSee also: gitlab_list_epic_discussions, gitlab_add_epic_discussion_note",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_epic_discussion", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_create_epic_discussion",
		Title:       toolutil.TitleFromName("gitlab_create_epic_discussion"),
		Description: "Create a new discussion thread on a group epic via the Work Items GraphQL API.\n\nReturns: JSON with created discussion thread including ID and initial note.\n\nSee also: gitlab_list_epic_discussions, gitlab_add_epic_discussion_note",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_create_epic_discussion", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_add_epic_discussion_note",
		Title:       toolutil.TitleFromName("gitlab_add_epic_discussion_note"),
		Description: "Add a reply note to an existing epic discussion thread via the Work Items GraphQL API.\n\nReturns: JSON with created note including ID, body, and author.\n\nSee also: gitlab_get_epic_discussion, gitlab_update_epic_discussion_note",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AddNoteInput) (*mcp.CallToolResult, NoteOutput, error) {
		start := time.Now()
		out, err := AddNote(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_add_epic_discussion_note", start, err)
		return toolutil.WithHints(FormatNoteMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_update_epic_discussion_note",
		Title:       toolutil.TitleFromName("gitlab_update_epic_discussion_note"),
		Description: "Update an existing note in an epic discussion thread via the Work Items GraphQL API.\n\nReturns: JSON with updated note including ID, body, and author.\n\nSee also: gitlab_get_epic_discussion, gitlab_delete_epic_discussion_note",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateNoteInput) (*mcp.CallToolResult, NoteOutput, error) {
		start := time.Now()
		out, err := UpdateNote(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_update_epic_discussion_note", start, err)
		return toolutil.WithHints(FormatNoteMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_epic_discussion_note",
		Title:       toolutil.TitleFromName("gitlab_delete_epic_discussion_note"),
		Description: "Delete a note from an epic discussion thread via the Work Items GraphQL API.\n\nReturns: JSON with deletion confirmation.\n\nSee also: gitlab_list_epic_discussions, gitlab_add_epic_discussion_note",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteNoteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		toolutil.ConfirmAction(ctx, req, "delete epic discussion note")
		err := DeleteNote(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_epic_discussion_note", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("epic discussion note")
	})
}

// RegisterMeta registers the gitlab_epic_discussion meta-tool.
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
		Name:  "gitlab_epic_discussion",
		Title: toolutil.TitleFromName("gitlab_epic_discussion"),
		Description: `Manage GitLab epic discussion threads via the Work Items GraphQL API. Use 'action' to specify the operation.

Actions:
- list: List discussion threads on an epic. Params: full_path, iid (required), first, after
- get: Get a single discussion. Params: full_path, iid, discussion_id (required)
- create: Create a new discussion thread. Params: full_path, iid, body (required)
- add_note: Reply to an existing discussion. Params: full_path, iid, discussion_id, body (required)
- update_note: Update a discussion note. Params: full_path, iid, note_id, body (required)
- delete_note: Delete a discussion note. Params: full_path, iid, note_id (required)`,
		Annotations: toolutil.MetaAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, toolutil.MakeMetaHandler("gitlab_epic_discussion", routes, nil))
}
