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
	specs := append(IssueActionSpecs(client), MergeRequestActionSpecs(client)...)
	specs = append(specs, SnippetActionSpecs(client)...)
	awardEmojiTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconLabel})
	}

	mcp.AddTool(server, awardEmojiTool("gitlab_issue_emoji_list", "List all award emoji on a project issue.\n\nSee also: gitlab_issue_emoji_create, gitlab_mr_emoji_list\n\nReturns: JSON array of award emoji with pagination. Fields include id, name, user, and awardable_type."), func(ctx context.Context, req *mcp.CallToolRequest, input IssueListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListIssueAwardEmoji(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_emoji_list", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, awardEmojiTool("gitlab_issue_emoji_get", "Get a single award emoji on a project issue.\n\nSee also: gitlab_issue_emoji_list, gitlab_issue_emoji_create\n\nReturns: JSON with award emoji details including id, name, user, and created_at."), func(ctx context.Context, req *mcp.CallToolRequest, input IssueGetInput) (*mcp.CallToolResult, Output, error) {
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

	mcp.AddTool(server, awardEmojiTool("gitlab_issue_emoji_create", "Add an award emoji reaction to a project issue.\n\nSee also: gitlab_issue_emoji_list, gitlab_issue_emoji_delete\n\nReturns: JSON with the created award emoji."), func(ctx context.Context, req *mcp.CallToolRequest, input IssueCreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := CreateIssueAwardEmoji(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_emoji_create", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, awardEmojiTool("gitlab_issue_emoji_delete", "Delete an award emoji from a project issue.\n\nSee also: gitlab_issue_emoji_list, gitlab_issue_emoji_create\n\nReturns: confirmation message."), func(ctx context.Context, req *mcp.CallToolRequest, input IssueDeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, deleteAction); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := DeleteIssueAwardEmoji(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_emoji_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(deleteResult)
	})

	mcp.AddTool(server, awardEmojiTool("gitlab_issue_note_emoji_list", "List all award emoji on a project issue note.\n\nSee also: gitlab_issue_note_emoji_create, gitlab_issue_emoji_list\n\nReturns: JSON array of award emoji with pagination. Fields include id, name, user, and awardable_type."), func(ctx context.Context, req *mcp.CallToolRequest, input IssueListOnNoteInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListIssueNoteAwardEmoji(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_note_emoji_list", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, awardEmojiTool("gitlab_issue_note_emoji_get", "Get a single award emoji on a project issue note.\n\nSee also: gitlab_issue_note_emoji_list, gitlab_issue_note_emoji_create\n\nReturns: JSON with award emoji details including id, name, user, and created_at."), func(ctx context.Context, req *mcp.CallToolRequest, input IssueGetOnNoteInput) (*mcp.CallToolResult, Output, error) {
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

	mcp.AddTool(server, awardEmojiTool("gitlab_issue_note_emoji_create", "Add an award emoji reaction to a project issue note.\n\nSee also: gitlab_issue_note_emoji_list, gitlab_issue_note_emoji_delete\n\nReturns: JSON with the created award emoji."), func(ctx context.Context, req *mcp.CallToolRequest, input IssueCreateOnNoteInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := CreateIssueNoteAwardEmoji(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_note_emoji_create", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, awardEmojiTool("gitlab_issue_note_emoji_delete", "Delete an award emoji from a project issue note.\n\nSee also: gitlab_issue_note_emoji_list, gitlab_issue_note_emoji_create\n\nReturns: confirmation message."), func(ctx context.Context, req *mcp.CallToolRequest, input IssueDeleteOnNoteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, deleteAction); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := DeleteIssueNoteAwardEmoji(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_note_emoji_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(deleteResult)
	})

	mcp.AddTool(server, awardEmojiTool("gitlab_mr_emoji_list", "List all award emoji on a merge request.\n\nSee also: gitlab_mr_emoji_create, gitlab_issue_emoji_list\n\nReturns: JSON array of award emoji with pagination. Fields include id, name, user, and awardable_type."), func(ctx context.Context, req *mcp.CallToolRequest, input MRListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListMRAwardEmoji(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_emoji_list", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, awardEmojiTool("gitlab_mr_emoji_get", "Get a single award emoji on a merge request.\n\nSee also: gitlab_mr_emoji_list, gitlab_mr_emoji_create\n\nReturns: JSON with award emoji details including id, name, user, and created_at."), func(ctx context.Context, req *mcp.CallToolRequest, input MRGetInput) (*mcp.CallToolResult, Output, error) {
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

	mcp.AddTool(server, awardEmojiTool("gitlab_mr_emoji_create", "Add an award emoji reaction to a merge request.\n\nSee also: gitlab_mr_emoji_list, gitlab_mr_emoji_delete\n\nReturns: JSON with the created award emoji."), func(ctx context.Context, req *mcp.CallToolRequest, input MRCreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := CreateMRAwardEmoji(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_emoji_create", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, awardEmojiTool("gitlab_mr_emoji_delete", "Delete an award emoji from a merge request.\n\nSee also: gitlab_mr_emoji_list, gitlab_mr_emoji_create\n\nReturns: confirmation message."), func(ctx context.Context, req *mcp.CallToolRequest, input MRDeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, deleteAction); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := DeleteMRAwardEmoji(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_emoji_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(deleteResult)
	})

	mcp.AddTool(server, awardEmojiTool("gitlab_mr_note_emoji_list", "List all award emoji on a merge request note.\n\nSee also: gitlab_mr_note_emoji_create, gitlab_mr_emoji_list\n\nReturns: JSON array of award emoji with pagination. Fields include id, name, user, and awardable_type."), func(ctx context.Context, req *mcp.CallToolRequest, input MRListOnNoteInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListMRNoteAwardEmoji(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_note_emoji_list", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, awardEmojiTool("gitlab_mr_note_emoji_get", "Get a single award emoji on a merge request note.\n\nSee also: gitlab_mr_note_emoji_list, gitlab_mr_note_emoji_create\n\nReturns: JSON with award emoji details including id, name, user, and created_at."), func(ctx context.Context, req *mcp.CallToolRequest, input MRGetOnNoteInput) (*mcp.CallToolResult, Output, error) {
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

	mcp.AddTool(server, awardEmojiTool("gitlab_mr_note_emoji_create", "Add an award emoji reaction to a merge request note.\n\nSee also: gitlab_mr_note_emoji_list, gitlab_mr_note_emoji_delete\n\nReturns: JSON with the created award emoji."), func(ctx context.Context, req *mcp.CallToolRequest, input MRCreateOnNoteInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := CreateMRNoteAwardEmoji(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_note_emoji_create", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, awardEmojiTool("gitlab_mr_note_emoji_delete", "Delete an award emoji from a merge request note.\n\nSee also: gitlab_mr_note_emoji_list, gitlab_mr_note_emoji_create\n\nReturns: confirmation message."), func(ctx context.Context, req *mcp.CallToolRequest, input MRDeleteOnNoteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, deleteAction); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := DeleteMRNoteAwardEmoji(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_note_emoji_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(deleteResult)
	})

	mcp.AddTool(server, awardEmojiTool("gitlab_snippet_emoji_list", "List all award emoji on a project snippet.\n\nSee also: gitlab_snippet_emoji_create, gitlab_issue_emoji_list\n\nReturns: JSON array of award emoji with pagination. Fields include id, name, user, and awardable_type."), func(ctx context.Context, req *mcp.CallToolRequest, input SnippetListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListSnippetAwardEmoji(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_snippet_emoji_list", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, awardEmojiTool("gitlab_snippet_emoji_get", "Get a single award emoji on a project snippet.\n\nSee also: gitlab_snippet_emoji_list, gitlab_snippet_emoji_create\n\nReturns: JSON with award emoji details including id, name, user, and created_at."), func(ctx context.Context, req *mcp.CallToolRequest, input SnippetGetInput) (*mcp.CallToolResult, Output, error) {
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

	mcp.AddTool(server, awardEmojiTool("gitlab_snippet_emoji_create", "Add an award emoji reaction to a project snippet.\n\nSee also: gitlab_snippet_emoji_list, gitlab_snippet_emoji_delete\n\nReturns: JSON with the created award emoji."), func(ctx context.Context, req *mcp.CallToolRequest, input SnippetCreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := CreateSnippetAwardEmoji(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_snippet_emoji_create", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, awardEmojiTool("gitlab_snippet_emoji_delete", "Delete an award emoji from a project snippet.\n\nSee also: gitlab_snippet_emoji_list, gitlab_snippet_emoji_create\n\nReturns: confirmation message."), func(ctx context.Context, req *mcp.CallToolRequest, input SnippetDeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, deleteAction); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := DeleteSnippetAwardEmoji(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_snippet_emoji_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(deleteResult)
	})

	mcp.AddTool(server, awardEmojiTool("gitlab_snippet_note_emoji_list", "List all award emoji on a project snippet note.\n\nReturns: JSON array of award emoji with pagination. Fields include id, name, user, and awardable_type.\n\nSee also: gitlab_snippet_note_emoji_create."), func(ctx context.Context, req *mcp.CallToolRequest, input SnippetListOnNoteInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListSnippetNoteAwardEmoji(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_snippet_note_emoji_list", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, awardEmojiTool("gitlab_snippet_note_emoji_get", "Get a single award emoji on a project snippet note.\n\nReturns: JSON with award emoji details including id, name, user, and created_at.\n\nSee also: gitlab_snippet_note_emoji_list."), func(ctx context.Context, req *mcp.CallToolRequest, input SnippetGetOnNoteInput) (*mcp.CallToolResult, Output, error) {
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

	mcp.AddTool(server, awardEmojiTool("gitlab_snippet_note_emoji_create", "Add an award emoji reaction to a project snippet note.\n\nReturns: JSON with the created award emoji.\n\nSee also: gitlab_snippet_note_emoji_list."), func(ctx context.Context, req *mcp.CallToolRequest, input SnippetCreateOnNoteInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := CreateSnippetNoteAwardEmoji(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_snippet_note_emoji_create", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, awardEmojiTool("gitlab_snippet_note_emoji_delete", "Delete an award emoji from a project snippet note.\n\nReturns: confirmation message.\n\nSee also: gitlab_snippet_note_emoji_list."), func(ctx context.Context, req *mcp.CallToolRequest, input SnippetDeleteOnNoteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, deleteAction); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := DeleteSnippetNoteAwardEmoji(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_snippet_note_emoji_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(deleteResult)
	})
}
