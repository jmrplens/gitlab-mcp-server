// register.go wires MR draft note tools into the MCP server.
package mrdraftnotes

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all MR draft note MCP tools with the server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_draft_note_list",
		Title:       toolutil.TitleFromName("gitlab_mr_draft_note_list"),
		Description: "List all draft notes (pending review comments) on a GitLab merge request. Draft notes are only visible to the author until published. Supports pagination and sorting.\n\nReturns: JSON array of draft notes. See also: gitlab_mr_get, gitlab_mr_discussion_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_draft_note_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_draft_note_get",
		Title:       toolutil.TitleFromName("gitlab_mr_draft_note_get"),
		Description: "Get a single draft note from a GitLab merge request by its ID. Returns the note body, author, and associated commit/discussion details.\n\nReturns: JSON with draft note details. See also: gitlab_mr_draft_note_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		if err != nil && toolutil.IsHTTPStatus(err, 404) {
			toolutil.LogToolCallAll(ctx, req, "gitlab_mr_draft_note_get", start, nil)
			return toolutil.NotFoundResult("MR Draft Note", fmt.Sprintf("draft note %d on MR !%d in project %s", input.NoteID, input.MRIID, input.ProjectID),
				"Use gitlab_mr_draft_note_list to list draft notes on this merge request",
				"Draft notes may have been published or deleted",
			), Output{}, nil
		}
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_draft_note_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_draft_note_create",
		Title:       toolutil.TitleFromName("gitlab_mr_draft_note_create"),
		Description: "Create a new draft note (pending review comment) on a GitLab merge request. Supports inline diff comments on specific lines via the position parameter. Draft notes stay private until published with draft_note_publish_all. Prefer this over discussion_create for code reviews to batch all comments into a single notification.\n\nReturns: JSON with the created draft note details. See also: gitlab_mr_get.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_draft_note_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_draft_note_update",
		Title:       toolutil.TitleFromName("gitlab_mr_draft_note_update"),
		Description: "Update the body text of an existing draft note on a GitLab merge request. Only the draft author can update it.\n\nReturns: JSON with the updated draft note details. See also: gitlab_mr_draft_note_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_draft_note_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_draft_note_delete",
		Title:       toolutil.TitleFromName("gitlab_mr_draft_note_delete"),
		Description: "Permanently delete a draft note from a GitLab merge request. Only the draft author can delete it.\n\nReturns: confirmation message. See also: gitlab_mr_draft_note_list.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete draft note %d from MR !%d in project %q?", input.NoteID, input.MRIID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_draft_note_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("draft note %d from MR !%d in project %s", input.NoteID, input.MRIID, input.ProjectID))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_draft_note_publish",
		Title:       toolutil.TitleFromName("gitlab_mr_draft_note_publish"),
		Description: "Publish a single draft note on a GitLab merge request, making it visible to all participants. This action cannot be undone.\n\nReturns: confirmation message. See also: gitlab_mr_draft_note_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input PublishInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		err := Publish(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_draft_note_publish", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		out := toolutil.DeleteOutput{Status: "success", Message: fmt.Sprintf("Draft note %d published on MR !%d in project %s", input.NoteID, input.MRIID, input.ProjectID)}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(out.Message), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_draft_note_publish_all",
		Title:       toolutil.TitleFromName("gitlab_mr_draft_note_publish_all"),
		Description: "Publish all pending draft notes on a GitLab merge request at once, making them visible to all participants. This action cannot be undone.\n\nReturns: confirmation message. See also: gitlab_mr_draft_note_list.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconDiscussion,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input PublishAllInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		err := PublishAll(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_draft_note_publish_all", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		out := toolutil.DeleteOutput{Status: "success", Message: fmt.Sprintf("All draft notes published on MR !%d in project %s", input.MRIID, input.ProjectID)}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(out.Message), out, nil)
	})
}
