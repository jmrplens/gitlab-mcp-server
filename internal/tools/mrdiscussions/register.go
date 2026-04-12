// Package mrdiscussions provides MCP tool handlers for GitLab merge request
// discussion operations: create (general and inline), resolve, reply, and list.
package mrdiscussions

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/progress"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all merge request discussion tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_discussion_create",
		Title:       toolutil.TitleFromName("gitlab_mr_discussion_create"),
		Description: "Start a new threaded discussion on a GitLab merge request. For a general discussion, just provide 'body'. For an inline diff comment, also provide 'position' with base_sha, start_sha, head_sha (get these from gitlab_mr_get diff_refs field), new_path, and old_line/new_line.\n\nReturns: JSON with the created discussion. See also: gitlab_mr_get, gitlab_mr_discussion_list.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_discussion_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_discussion_resolve",
		Title:       toolutil.TitleFromName("gitlab_mr_discussion_resolve"),
		Description: "Resolve or unresolve a discussion thread on a GitLab merge request. Resolved discussions are collapsed in the UI to reduce review noise.\n\nReturns: JSON with the updated discussion resolution status. See also: gitlab_mr_discussion_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ResolveInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Resolve(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_discussion_resolve", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_discussion_reply",
		Title:       toolutil.TitleFromName("gitlab_mr_discussion_reply"),
		Description: "Add a reply to an existing discussion thread on a GitLab merge request. The reply appears nested under the original discussion note.\n\nReturns: JSON with the created reply note. See also: gitlab_mr_discussion_get.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ReplyInput) (*mcp.CallToolResult, NoteOutput, error) {
		start := time.Now()
		out, err := Reply(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_discussion_reply", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatNoteMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_discussion_list",
		Title:       toolutil.TitleFromName("gitlab_mr_discussion_list"),
		Description: "List all discussion threads on a GitLab merge request including inline diff comments and general discussions. Each thread contains its notes and resolution status. Returns paginated results.\n\nReturns: JSON array of discussions with pagination. See also: gitlab_mr_get, gitlab_mr_note_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		tracker := progress.FromRequest(req)
		tracker.Step(ctx, 1, 2, "Fetching discussions...")
		out, err := List(ctx, client, input)
		tracker.Step(ctx, 2, 2, toolutil.StepFormattingResponse)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_discussion_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_discussion_get",
		Title:       toolutil.TitleFromName("gitlab_mr_discussion_get"),
		Description: "Get a single discussion thread from a GitLab merge request by its discussion ID, including all notes in the thread.\n\nReturns: JSON with discussion details and notes. See also: gitlab_mr_discussion_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_discussion_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_discussion_note_update",
		Title:       toolutil.TitleFromName("gitlab_mr_discussion_note_update"),
		Description: "Update the body or resolved status of a note within a merge request discussion thread. You can modify the text, change resolution status, or both.\n\nReturns: JSON with the updated note details. See also: gitlab_mr_discussion_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateNoteInput) (*mcp.CallToolResult, NoteOutput, error) {
		start := time.Now()
		out, err := UpdateNote(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_discussion_note_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatNoteMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_discussion_note_delete",
		Title:       toolutil.TitleFromName("gitlab_mr_discussion_note_delete"),
		Description: "Delete a note from a merge request discussion thread. Only the note author or project maintainers can delete notes.\n\nReturns: JSON confirming note deletion. See also: gitlab_mr_discussion_get.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteNoteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete discussion note %d from MR !%d in project %q?", input.NoteID, input.MRIID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		if err := DeleteNote(ctx, client, input); err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_discussion_note_delete", start, nil)
		return toolutil.DeleteResult(fmt.Sprintf("discussion note %d from MR !%d in project %s", input.NoteID, input.MRIID, input.ProjectID))
	})
}
