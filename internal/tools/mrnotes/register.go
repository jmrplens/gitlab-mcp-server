// Package mrnotes provides MCP tool handlers for GitLab merge request note
// operations: create, list, update, and delete.
package mrnotes

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all merge request note tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_note_create",
		Title:       toolutil.TitleFromName("gitlab_mr_note_create"),
		Description: "Add a comment (note) to a GitLab merge request. The comment appears in the merge request's activity timeline as a top-level note.\n\nReturns: JSON with the created note (ID, author, body, timestamps). See also: gitlab_mr_get.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_note_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_notes_list",
		Title:       toolutil.TitleFromName("gitlab_mr_notes_list"),
		Description: "List all comments (notes) on a GitLab merge request ordered by creation date. Includes both user comments and system-generated notes (status changes, label updates).\n\nReturns: JSON with array of notes and pagination info. See also: gitlab_mr_get, gitlab_mr_discussion_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_notes_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_note_update",
		Title:       toolutil.TitleFromName("gitlab_mr_note_update"),
		Description: "Edit the body text of an existing comment on a GitLab merge request. Only the note author or a project maintainer can update a note.\n\nReturns: JSON with the updated note details. See also: gitlab_mr_note_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_note_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_note_get",
		Title:       toolutil.TitleFromName("gitlab_mr_note_get"),
		Description: "Get a single comment (note) from a GitLab merge request by its note ID, including author, timestamps, resolution status, and body.\n\nReturns: JSON with note details (ID, author, body, timestamps, resolution status). See also: gitlab_mr_notes_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetNote(ctx, client, input)
		if err != nil && toolutil.IsHTTPStatus(err, 404) {
			toolutil.LogToolCallAll(ctx, req, "gitlab_mr_note_get", start, nil)
			return toolutil.NotFoundResult("MR Note", fmt.Sprintf("note %d on MR !%d in project %s", input.NoteID, input.MRIID, input.ProjectID),
				"Use gitlab_mr_notes_list to list notes on this merge request",
				"Verify the note_id and merge_request_iid are correct",
			), Output{}, nil
		}
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_note_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_note_delete",
		Title:       toolutil.TitleFromName("gitlab_mr_note_delete"),
		Description: "Permanently delete a comment from a GitLab merge request. Only the note author or a project maintainer can delete a note.\n\nReturns: JSON confirmation of deletion. See also: gitlab_mr_note_get.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete note %d from MR !%d in project %q?", input.NoteID, input.MRIID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_note_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("note %d from MR !%d in project %s", input.NoteID, input.MRIID, input.ProjectID))
	})
}
