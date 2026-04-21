// register.go wires awardemoji MCP tools to the MCP server.

package awardemoji

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const (
	resourceName       = "Award Emoji"
	deleteAction       = "delete award emoji"
	deleteResult       = "award emoji"
	hintVerifyBasic    = "Verify the award_id, iid, and project_id are correct"
	hintVerifyWithNote = "Verify the award_id, note_id, iid, and project_id are correct"
)

// RegisterTools registers individual award emoji tools.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	// Issue award emoji.
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_issue_emoji_list",
		Title:       toolutil.TitleFromName("gitlab_issue_emoji_list"),
		Description: "List all award emoji on a project issue.\n\nSee also: gitlab_issue_emoji_create, gitlab_mr_emoji_list\n\nReturns: JSON array of award emoji with pagination. Fields include id, name, user, and awardable_type.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListIssueAwardEmoji(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_emoji_list", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_issue_emoji_get",
		Title:       toolutil.TitleFromName("gitlab_issue_emoji_get"),
		Description: "Get a single award emoji on a project issue.\n\nSee also: gitlab_issue_emoji_list, gitlab_issue_emoji_create\n\nReturns: JSON with award emoji details including id, name, user, and created_at.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetIssueAwardEmoji(ctx, client, input)
		if err != nil && toolutil.IsHTTPStatus(err, 404) {
			toolutil.LogToolCallAll(ctx, req, "gitlab_issue_emoji_get", start, nil)
			return toolutil.NotFoundResult(resourceName, fmt.Sprintf("award %d on issue IID %d in project %s", input.AwardID, input.IID, input.ProjectID),
				"Use gitlab_issue_emoji_list to list emojis on this issue",
				hintVerifyBasic,
			), Output{}, nil
		}
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_emoji_get", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_issue_emoji_create",
		Title:       toolutil.TitleFromName("gitlab_issue_emoji_create"),
		Description: "Add an award emoji reaction to a project issue.\n\nSee also: gitlab_issue_emoji_list, gitlab_issue_emoji_delete\n\nReturns: JSON with the created award emoji.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := CreateIssueAwardEmoji(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_emoji_create", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_issue_emoji_delete",
		Title:       toolutil.TitleFromName("gitlab_issue_emoji_delete"),
		Description: "Delete an award emoji from a project issue.\n\nSee also: gitlab_issue_emoji_list, gitlab_issue_emoji_create\n\nReturns: confirmation message.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		toolutil.ConfirmAction(ctx, req, deleteAction)
		err := DeleteIssueAwardEmoji(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_emoji_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(deleteResult)
	})

	// Issue note award emoji.
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_issue_note_emoji_list",
		Title:       toolutil.TitleFromName("gitlab_issue_note_emoji_list"),
		Description: "List all award emoji on a project issue note.\n\nSee also: gitlab_issue_note_emoji_create, gitlab_issue_emoji_list\n\nReturns: JSON array of award emoji with pagination. Fields include id, name, user, and awardable_type.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListOnNoteInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListIssueNoteAwardEmoji(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_note_emoji_list", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_issue_note_emoji_get",
		Title:       toolutil.TitleFromName("gitlab_issue_note_emoji_get"),
		Description: "Get a single award emoji on a project issue note.\n\nSee also: gitlab_issue_note_emoji_list, gitlab_issue_note_emoji_create\n\nReturns: JSON with award emoji details including id, name, user, and created_at.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetOnNoteInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetIssueNoteAwardEmoji(ctx, client, input)
		if err != nil && toolutil.IsHTTPStatus(err, 404) {
			toolutil.LogToolCallAll(ctx, req, "gitlab_issue_note_emoji_get", start, nil)
			return toolutil.NotFoundResult(resourceName, fmt.Sprintf("award %d on note %d (issue IID %d) in project %s", input.AwardID, input.NoteID, input.IID, input.ProjectID),
				"Use gitlab_issue_note_emoji_list to list emojis on this note",
				hintVerifyWithNote,
			), Output{}, nil
		}
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_note_emoji_get", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_issue_note_emoji_create",
		Title:       toolutil.TitleFromName("gitlab_issue_note_emoji_create"),
		Description: "Add an award emoji reaction to a project issue note.\n\nSee also: gitlab_issue_note_emoji_list, gitlab_issue_note_emoji_delete\n\nReturns: JSON with the created award emoji.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateOnNoteInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := CreateIssueNoteAwardEmoji(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_note_emoji_create", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_issue_note_emoji_delete",
		Title:       toolutil.TitleFromName("gitlab_issue_note_emoji_delete"),
		Description: "Delete an award emoji from a project issue note.\n\nSee also: gitlab_issue_note_emoji_list, gitlab_issue_note_emoji_create\n\nReturns: confirmation message.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteOnNoteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		toolutil.ConfirmAction(ctx, req, deleteAction)
		err := DeleteIssueNoteAwardEmoji(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_note_emoji_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(deleteResult)
	})

	// MR award emoji.
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_emoji_list",
		Title:       toolutil.TitleFromName("gitlab_mr_emoji_list"),
		Description: "List all award emoji on a merge request.\n\nSee also: gitlab_mr_emoji_create, gitlab_issue_emoji_list\n\nReturns: JSON array of award emoji with pagination. Fields include id, name, user, and awardable_type.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListMRAwardEmoji(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_emoji_list", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_emoji_get",
		Title:       toolutil.TitleFromName("gitlab_mr_emoji_get"),
		Description: "Get a single award emoji on a merge request.\n\nSee also: gitlab_mr_emoji_list, gitlab_mr_emoji_create\n\nReturns: JSON with award emoji details including id, name, user, and created_at.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetMRAwardEmoji(ctx, client, input)
		if err != nil && toolutil.IsHTTPStatus(err, 404) {
			toolutil.LogToolCallAll(ctx, req, "gitlab_mr_emoji_get", start, nil)
			return toolutil.NotFoundResult(resourceName, fmt.Sprintf("award %d on MR IID %d in project %s", input.AwardID, input.IID, input.ProjectID),
				"Use gitlab_mr_emoji_list to list emojis on this merge request",
				hintVerifyBasic,
			), Output{}, nil
		}
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_emoji_get", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_emoji_create",
		Title:       toolutil.TitleFromName("gitlab_mr_emoji_create"),
		Description: "Add an award emoji reaction to a merge request.\n\nSee also: gitlab_mr_emoji_list, gitlab_mr_emoji_delete\n\nReturns: JSON with the created award emoji.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := CreateMRAwardEmoji(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_emoji_create", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_emoji_delete",
		Title:       toolutil.TitleFromName("gitlab_mr_emoji_delete"),
		Description: "Delete an award emoji from a merge request.\n\nSee also: gitlab_mr_emoji_list, gitlab_mr_emoji_create\n\nReturns: confirmation message.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		toolutil.ConfirmAction(ctx, req, deleteAction)
		err := DeleteMRAwardEmoji(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_emoji_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(deleteResult)
	})

	// MR note award emoji.
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_note_emoji_list",
		Title:       toolutil.TitleFromName("gitlab_mr_note_emoji_list"),
		Description: "List all award emoji on a merge request note.\n\nSee also: gitlab_mr_note_emoji_create, gitlab_mr_emoji_list\n\nReturns: JSON array of award emoji with pagination. Fields include id, name, user, and awardable_type.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListOnNoteInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListMRNoteAwardEmoji(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_note_emoji_list", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_note_emoji_get",
		Title:       toolutil.TitleFromName("gitlab_mr_note_emoji_get"),
		Description: "Get a single award emoji on a merge request note.\n\nSee also: gitlab_mr_note_emoji_list, gitlab_mr_note_emoji_create\n\nReturns: JSON with award emoji details including id, name, user, and created_at.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetOnNoteInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetMRNoteAwardEmoji(ctx, client, input)
		if err != nil && toolutil.IsHTTPStatus(err, 404) {
			toolutil.LogToolCallAll(ctx, req, "gitlab_mr_note_emoji_get", start, nil)
			return toolutil.NotFoundResult(resourceName, fmt.Sprintf("award %d on note %d (MR IID %d) in project %s", input.AwardID, input.NoteID, input.IID, input.ProjectID),
				"Use gitlab_mr_note_emoji_list to list emojis on this note",
				hintVerifyWithNote,
			), Output{}, nil
		}
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_note_emoji_get", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_note_emoji_create",
		Title:       toolutil.TitleFromName("gitlab_mr_note_emoji_create"),
		Description: "Add an award emoji reaction to a merge request note.\n\nSee also: gitlab_mr_note_emoji_list, gitlab_mr_note_emoji_delete\n\nReturns: JSON with the created award emoji.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateOnNoteInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := CreateMRNoteAwardEmoji(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_note_emoji_create", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_note_emoji_delete",
		Title:       toolutil.TitleFromName("gitlab_mr_note_emoji_delete"),
		Description: "Delete an award emoji from a merge request note.\n\nSee also: gitlab_mr_note_emoji_list, gitlab_mr_note_emoji_create\n\nReturns: confirmation message.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteOnNoteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		toolutil.ConfirmAction(ctx, req, deleteAction)
		err := DeleteMRNoteAwardEmoji(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_note_emoji_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(deleteResult)
	})

	// Snippet award emoji.
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_snippet_emoji_list",
		Title:       toolutil.TitleFromName("gitlab_snippet_emoji_list"),
		Description: "List all award emoji on a project snippet.\n\nSee also: gitlab_snippet_emoji_create, gitlab_issue_emoji_list\n\nReturns: JSON array of award emoji with pagination. Fields include id, name, user, and awardable_type.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListSnippetAwardEmoji(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_snippet_emoji_list", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_snippet_emoji_get",
		Title:       toolutil.TitleFromName("gitlab_snippet_emoji_get"),
		Description: "Get a single award emoji on a project snippet.\n\nSee also: gitlab_snippet_emoji_list, gitlab_snippet_emoji_create\n\nReturns: JSON with award emoji details including id, name, user, and created_at.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetSnippetAwardEmoji(ctx, client, input)
		if err != nil && toolutil.IsHTTPStatus(err, 404) {
			toolutil.LogToolCallAll(ctx, req, "gitlab_snippet_emoji_get", start, nil)
			return toolutil.NotFoundResult(resourceName, fmt.Sprintf("award %d on snippet IID %d in project %s", input.AwardID, input.IID, input.ProjectID),
				"Use gitlab_snippet_emoji_list to list emojis on this snippet",
				hintVerifyBasic,
			), Output{}, nil
		}
		toolutil.LogToolCallAll(ctx, req, "gitlab_snippet_emoji_get", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_snippet_emoji_create",
		Title:       toolutil.TitleFromName("gitlab_snippet_emoji_create"),
		Description: "Add an award emoji reaction to a project snippet.\n\nSee also: gitlab_snippet_emoji_list, gitlab_snippet_emoji_delete\n\nReturns: JSON with the created award emoji.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := CreateSnippetAwardEmoji(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_snippet_emoji_create", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_snippet_emoji_delete",
		Title:       toolutil.TitleFromName("gitlab_snippet_emoji_delete"),
		Description: "Delete an award emoji from a project snippet.\n\nSee also: gitlab_snippet_emoji_list, gitlab_snippet_emoji_create\n\nReturns: confirmation message.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		toolutil.ConfirmAction(ctx, req, deleteAction)
		err := DeleteSnippetAwardEmoji(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_snippet_emoji_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(deleteResult)
	})

	// Snippet note award emoji.
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_snippet_note_emoji_list",
		Title:       toolutil.TitleFromName("gitlab_snippet_note_emoji_list"),
		Description: "List all award emoji on a project snippet note.\n\nReturns: JSON array of award emoji with pagination. Fields include id, name, user, and awardable_type.\n\nSee also: gitlab_snippet_note_emoji_create.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListOnNoteInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListSnippetNoteAwardEmoji(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_snippet_note_emoji_list", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_snippet_note_emoji_get",
		Title:       toolutil.TitleFromName("gitlab_snippet_note_emoji_get"),
		Description: "Get a single award emoji on a project snippet note.\n\nReturns: JSON with award emoji details including id, name, user, and created_at.\n\nSee also: gitlab_snippet_note_emoji_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetOnNoteInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetSnippetNoteAwardEmoji(ctx, client, input)
		if err != nil && toolutil.IsHTTPStatus(err, 404) {
			toolutil.LogToolCallAll(ctx, req, "gitlab_snippet_note_emoji_get", start, nil)
			return toolutil.NotFoundResult(resourceName, fmt.Sprintf("award %d on note %d (snippet IID %d) in project %s", input.AwardID, input.NoteID, input.IID, input.ProjectID),
				"Use gitlab_snippet_note_emoji_list to list emojis on this note",
				hintVerifyWithNote,
			), Output{}, nil
		}
		toolutil.LogToolCallAll(ctx, req, "gitlab_snippet_note_emoji_get", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_snippet_note_emoji_create",
		Title:       toolutil.TitleFromName("gitlab_snippet_note_emoji_create"),
		Description: "Add an award emoji reaction to a project snippet note.\n\nReturns: JSON with the created award emoji.\n\nSee also: gitlab_snippet_note_emoji_list.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateOnNoteInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := CreateSnippetNoteAwardEmoji(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_snippet_note_emoji_create", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_snippet_note_emoji_delete",
		Title:       toolutil.TitleFromName("gitlab_snippet_note_emoji_delete"),
		Description: "Delete an award emoji from a project snippet note.\n\nReturns: confirmation message.\n\nSee also: gitlab_snippet_note_emoji_list.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteOnNoteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		toolutil.ConfirmAction(ctx, req, deleteAction)
		err := DeleteSnippetNoteAwardEmoji(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_snippet_note_emoji_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(deleteResult)
	})
}

// RegisterMeta registers the gitlab_award_emoji meta-tool.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]toolutil.ActionFunc{
		"issue_list":          toolutil.WrapAction(client, ListIssueAwardEmoji),
		"issue_get":           toolutil.WrapAction(client, GetIssueAwardEmoji),
		"issue_create":        toolutil.WrapAction(client, CreateIssueAwardEmoji),
		"issue_delete":        toolutil.WrapVoidAction(client, DeleteIssueAwardEmoji),
		"issue_note_list":     toolutil.WrapAction(client, ListIssueNoteAwardEmoji),
		"issue_note_get":      toolutil.WrapAction(client, GetIssueNoteAwardEmoji),
		"issue_note_create":   toolutil.WrapAction(client, CreateIssueNoteAwardEmoji),
		"issue_note_delete":   toolutil.WrapVoidAction(client, DeleteIssueNoteAwardEmoji),
		"mr_list":             toolutil.WrapAction(client, ListMRAwardEmoji),
		"mr_get":              toolutil.WrapAction(client, GetMRAwardEmoji),
		"mr_create":           toolutil.WrapAction(client, CreateMRAwardEmoji),
		"mr_delete":           toolutil.WrapVoidAction(client, DeleteMRAwardEmoji),
		"mr_note_list":        toolutil.WrapAction(client, ListMRNoteAwardEmoji),
		"mr_note_get":         toolutil.WrapAction(client, GetMRNoteAwardEmoji),
		"mr_note_create":      toolutil.WrapAction(client, CreateMRNoteAwardEmoji),
		"mr_note_delete":      toolutil.WrapVoidAction(client, DeleteMRNoteAwardEmoji),
		"snippet_list":        toolutil.WrapAction(client, ListSnippetAwardEmoji),
		"snippet_get":         toolutil.WrapAction(client, GetSnippetAwardEmoji),
		"snippet_create":      toolutil.WrapAction(client, CreateSnippetAwardEmoji),
		"snippet_delete":      toolutil.WrapVoidAction(client, DeleteSnippetAwardEmoji),
		"snippet_note_list":   toolutil.WrapAction(client, ListSnippetNoteAwardEmoji),
		"snippet_note_get":    toolutil.WrapAction(client, GetSnippetNoteAwardEmoji),
		"snippet_note_create": toolutil.WrapAction(client, CreateSnippetNoteAwardEmoji),
		"snippet_note_delete": toolutil.WrapVoidAction(client, DeleteSnippetNoteAwardEmoji),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_award_emoji",
		Title: toolutil.TitleFromName("gitlab_award_emoji"),
		Description: `Manage GitLab award emoji (reactions) on issues, merge requests, snippets, and their notes. Use 'action' to specify the operation.

Actions — Issue emoji:
- issue_list: List emoji on issue. Params: project_id (required), iid (required), page, per_page
- issue_get: Get single emoji on issue. Params: project_id (required), iid (required), award_id (required)
- issue_create: Add emoji to issue. Params: project_id (required), iid (required), name (required)
- issue_delete: Delete emoji from issue. Params: project_id (required), iid (required), award_id (required)

Actions — Issue note emoji:
- issue_note_list: List emoji on issue note. Params: project_id (required), iid (required), note_id (required), page, per_page
- issue_note_get: Get single emoji on issue note. Params: project_id (required), iid (required), note_id (required), award_id (required)
- issue_note_create: Add emoji to issue note. Params: project_id (required), iid (required), note_id (required), name (required)
- issue_note_delete: Delete emoji from issue note. Params: project_id (required), iid (required), note_id (required), award_id (required)

Actions — MR emoji:
- mr_list: List emoji on MR. Params: project_id (required), iid (required), page, per_page
- mr_get: Get single emoji on MR. Params: project_id (required), iid (required), award_id (required)
- mr_create: Add emoji to MR. Params: project_id (required), iid (required), name (required)
- mr_delete: Delete emoji from MR. Params: project_id (required), iid (required), award_id (required)

Actions — MR note emoji:
- mr_note_list: List emoji on MR note. Params: project_id (required), iid (required), note_id (required), page, per_page
- mr_note_get: Get single emoji on MR note. Params: project_id (required), iid (required), note_id (required), award_id (required)
- mr_note_create: Add emoji to MR note. Params: project_id (required), iid (required), note_id (required), name (required)
- mr_note_delete: Delete emoji from MR note. Params: project_id (required), iid (required), note_id (required), award_id (required)

Actions — Snippet emoji:
- snippet_list: List emoji on snippet. Params: project_id (required), iid (required), page, per_page
- snippet_get: Get single emoji on snippet. Params: project_id (required), iid (required), award_id (required)
- snippet_create: Add emoji to snippet. Params: project_id (required), iid (required), name (required)
- snippet_delete: Delete emoji from snippet. Params: project_id (required), iid (required), award_id (required)

Actions — Snippet note emoji:
- snippet_note_list: List emoji on snippet note. Params: project_id (required), iid (required), note_id (required), page, per_page
- snippet_note_get: Get single emoji on snippet note. Params: project_id (required), iid (required), note_id (required), award_id (required)
- snippet_note_create: Add emoji to snippet note. Params: project_id (required), iid (required), note_id (required), name (required)
- snippet_note_delete: Delete emoji from snippet note. Params: project_id (required), iid (required), note_id (required), award_id (required)`,
		Annotations: toolutil.MetaAnnotations,
		Icons:       toolutil.IconLabel,
	}, toolutil.MakeMetaHandler("gitlab_award_emoji", routes, nil))
}
