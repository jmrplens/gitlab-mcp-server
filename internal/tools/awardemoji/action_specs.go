package awardemoji

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	awardEmojiResourceName       = "Award Emoji"
	awardEmojiDeleteResult       = "award emoji"
	awardEmojiHintVerifyBasic    = "Verify the award_id, iid, and project_id are correct"
	awardEmojiHintVerifyWithNote = "Verify the award_id, note_id, iid, and project_id are correct"
)

// SnippetActionSpecs returns canonical specs for snippet award emoji actions.
func SnippetActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		snippetEmojiReadSpec("emoji_snippet_list", toolutil.RouteAction(client, ListSnippetAwardEmoji), "gitlab_snippet_emoji_list"),
		snippetEmojiReadSpec("emoji_snippet_get", awardEmojiGetRoute(client, GetSnippetAwardEmoji, snippetEmojiNotFound), "gitlab_snippet_emoji_get"),
		snippetEmojiCreateSpec("emoji_snippet_create", toolutil.RouteAction(client, CreateSnippetAwardEmoji), "gitlab_snippet_emoji_create"),
		snippetEmojiDeleteSpec("emoji_snippet_delete", awardEmojiDeleteRoute[SnippetDeleteInput](client, DeleteSnippetAwardEmoji), "gitlab_snippet_emoji_delete"),
		snippetEmojiReadSpec("emoji_snippet_note_list", toolutil.RouteAction(client, ListSnippetNoteAwardEmoji), "gitlab_snippet_note_emoji_list"),
		snippetEmojiReadSpec("emoji_snippet_note_get", awardEmojiGetRoute(client, GetSnippetNoteAwardEmoji, snippetNoteEmojiNotFound), "gitlab_snippet_note_emoji_get"),
		snippetEmojiCreateSpec("emoji_snippet_note_create", toolutil.RouteAction(client, CreateSnippetNoteAwardEmoji), "gitlab_snippet_note_emoji_create"),
		snippetEmojiDeleteSpec("emoji_snippet_note_delete", awardEmojiDeleteRoute[SnippetDeleteOnNoteInput](client, DeleteSnippetNoteAwardEmoji), "gitlab_snippet_note_emoji_delete"),
	}
}

// IssueActionSpecs returns canonical specs for issue award emoji actions.
func IssueActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		issueEmojiReadSpec("emoji_issue_list", toolutil.RouteAction(client, ListIssueAwardEmoji), "gitlab_issue_emoji_list"),
		issueEmojiReadSpec("emoji_issue_get", awardEmojiGetRoute(client, GetIssueAwardEmoji, issueEmojiNotFound), "gitlab_issue_emoji_get"),
		issueEmojiCreateSpec("emoji_issue_create", toolutil.RouteAction(client, CreateIssueAwardEmoji), "gitlab_issue_emoji_create"),
		issueEmojiDeleteSpec("emoji_issue_delete", awardEmojiDeleteRoute[IssueDeleteInput](client, DeleteIssueAwardEmoji), "gitlab_issue_emoji_delete"),
		issueEmojiReadSpec("emoji_issue_note_list", toolutil.RouteAction(client, ListIssueNoteAwardEmoji), "gitlab_issue_note_emoji_list"),
		issueEmojiReadSpec("emoji_issue_note_get", awardEmojiGetRoute(client, GetIssueNoteAwardEmoji, issueNoteEmojiNotFound), "gitlab_issue_note_emoji_get"),
		issueEmojiCreateSpec("emoji_issue_note_create", toolutil.RouteAction(client, CreateIssueNoteAwardEmoji), "gitlab_issue_note_emoji_create"),
		issueEmojiDeleteSpec("emoji_issue_note_delete", awardEmojiDeleteRoute[IssueDeleteOnNoteInput](client, DeleteIssueNoteAwardEmoji), "gitlab_issue_note_emoji_delete"),
	}
}

func issueEmojiReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, issueEmojiOptions(individualTool))
}

func issueEmojiCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, issueEmojiOptions(individualTool))
}

func issueEmojiDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, issueEmojiOptions(individualTool))
}

func awardEmojiBaseOptions(individualTool, ownerPackage string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases:        []string{individualTool},
		OpenWorld:      true,
		OwnerPackage:   ownerPackage,
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

func issueEmojiOptions(individualTool string) toolutil.ActionSpecOptions {
	opts := awardEmojiBaseOptions(individualTool, "awardemoji")
	opts.Tags = []string{"issue", "emoji"}
	opts.Usage = "Manage issue and issue-note award emojis (list/get/create/delete). Use this for reactions and lightweight signals on issues."
	opts.RelatedActions = []string{"issue.get", "issue_note.list"}
	opts.ParameterGuidance = map[string]toolutil.ParameterGuidance{
		"project_id": {
			SemanticRole:   "scope_project",
			ValueSource:    "Project ID or path that owns the issue.",
			ExampleBinding: `params.project_id:"group/project"`,
		},
		"issue_iid": {
			SemanticRole:   "issue_iid",
			ValueSource:    "Issue IID from issue list/get outputs.",
			ExampleBinding: "params.issue_iid:123",
		},
	}
	return opts
}

// MergeRequestActionSpecs returns canonical specs for merge request award emoji actions.
func MergeRequestActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		mergeRequestEmojiReadSpec("emoji_mr_list", toolutil.RouteAction(client, ListMRAwardEmoji), "gitlab_mr_emoji_list"),
		mergeRequestEmojiReadSpec("emoji_mr_get", awardEmojiGetRoute(client, GetMRAwardEmoji, mrEmojiNotFound), "gitlab_mr_emoji_get"),
		mergeRequestEmojiCreateSpec("emoji_mr_create", toolutil.RouteAction(client, CreateMRAwardEmoji), "gitlab_mr_emoji_create"),
		mergeRequestEmojiDeleteSpec("emoji_mr_delete", awardEmojiDeleteRoute[MRDeleteInput](client, DeleteMRAwardEmoji), "gitlab_mr_emoji_delete"),
		mergeRequestEmojiReadSpec("emoji_mr_note_list", toolutil.RouteAction(client, ListMRNoteAwardEmoji), "gitlab_mr_note_emoji_list"),
		mergeRequestEmojiReadSpec("emoji_mr_note_get", awardEmojiGetRoute(client, GetMRNoteAwardEmoji, mrNoteEmojiNotFound), "gitlab_mr_note_emoji_get"),
		mergeRequestEmojiCreateSpec("emoji_mr_note_create", toolutil.RouteAction(client, CreateMRNoteAwardEmoji), "gitlab_mr_note_emoji_create"),
		mergeRequestEmojiDeleteSpec("emoji_mr_note_delete", awardEmojiDeleteRoute[MRDeleteOnNoteInput](client, DeleteMRNoteAwardEmoji), "gitlab_mr_note_emoji_delete"),
	}
}

func mergeRequestEmojiReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, mergeRequestEmojiOptions(individualTool))
}

func mergeRequestEmojiCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, mergeRequestEmojiOptions(individualTool))
}

func mergeRequestEmojiDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, mergeRequestEmojiOptions(individualTool))
}

func mergeRequestEmojiOptions(individualTool string) toolutil.ActionSpecOptions {
	opts := awardEmojiBaseOptions(individualTool, "awardemoji")
	opts.Tags = []string{"merge_request", "emoji"}
	opts.Usage = "Manage merge request and MR-note award emojis (list/get/create/delete). Use for feedback and quick approval/review signals."
	opts.RelatedActions = []string{"merge_request.get", "mr_note.list"}
	opts.ParameterGuidance = map[string]toolutil.ParameterGuidance{
		"project_id": {
			SemanticRole:   "scope_project",
			ValueSource:    "Project ID or path that owns the merge request.",
			ExampleBinding: `params.project_id:"group/project"`,
		},
		"merge_request_iid": {
			SemanticRole:   "merge_request_iid",
			ValueSource:    "Merge request IID from MR list/get outputs.",
			ExampleBinding: "params.merge_request_iid:77",
		},
	}
	return opts
}

func snippetEmojiReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, snippetEmojiOptions(individualTool))
}

func snippetEmojiCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, snippetEmojiOptions(individualTool))
}

func snippetEmojiDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, snippetEmojiOptions(individualTool))
}

func snippetEmojiOptions(individualTool string) toolutil.ActionSpecOptions {
	opts := awardEmojiBaseOptions(individualTool, "awardemoji")
	opts.Tags = []string{"snippet", "emoji"}
	opts.Usage = "Manage snippet and snippet-note award emojis (list/get/create/delete). Use for reaction workflows around snippets."
	opts.RelatedActions = []string{"snippet.get", "snippet_note.list"}
	opts.ParameterGuidance = map[string]toolutil.ParameterGuidance{
		"project_id": {
			SemanticRole:   "scope_project",
			ValueSource:    "Project ID or path that owns the snippet.",
			ExampleBinding: `params.project_id:"group/project"`,
		},
		"snippet_id": {
			SemanticRole:   "snippet_iid",
			ValueSource:    "Snippet IID from snippet list/get outputs.",
			ExampleBinding: "params.snippet_id:12",
		},
	}
	return opts
}

func awardEmojiGetRoute[T any](client *gitlabclient.Client, fn func(context.Context, *gitlabclient.Client, T) (Output, error), notFound func(map[string]any) awardEmojiNotFoundOutput) toolutil.ActionRoute {
	route := toolutil.RouteAction(client, fn)
	baseHandler := route.Handler
	route.Handler = func(ctx context.Context, input map[string]any) (any, error) {
		result, err := baseHandler(ctx, input)
		if err != nil && toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return notFound(input), nil
		}
		return result, err
	}
	return route
}

func awardEmojiDeleteRoute[T any](client *gitlabclient.Client, fn func(context.Context, *gitlabclient.Client, T) error) toolutil.ActionRoute {
	return toolutil.DestructiveAction(client, func(ctx context.Context, client *gitlabclient.Client, input T) (toolutil.DeleteOutput, error) {
		if err := fn(ctx, client, input); err != nil {
			return toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteOutput{Status: "success", Message: fmt.Sprintf("Successfully deleted %s.", awardEmojiDeleteResult)}, nil
	})
}

func issueEmojiNotFound(input map[string]any) awardEmojiNotFoundOutput {
	return awardEmojiNotFoundOutput{
		Identifier: fmt.Sprintf("award %v on issue IID %v in project %v", input["award_id"], input["issue_iid"], input["project_id"]),
		ListHint:   "Use gitlab_issue_emoji_list to list emojis on this issue",
		VerifyHint: awardEmojiHintVerifyBasic,
	}
}

func issueNoteEmojiNotFound(input map[string]any) awardEmojiNotFoundOutput {
	return awardEmojiNotFoundOutput{
		Identifier: fmt.Sprintf("award %v on note %v (issue IID %v) in project %v", input["award_id"], input["note_id"], input["issue_iid"], input["project_id"]),
		ListHint:   "Use gitlab_issue_note_emoji_list to list emojis on this note",
		VerifyHint: awardEmojiHintVerifyWithNote,
	}
}

func mrEmojiNotFound(input map[string]any) awardEmojiNotFoundOutput {
	return awardEmojiNotFoundOutput{
		Identifier: fmt.Sprintf("award %v on MR IID %v in project %v", input["award_id"], input["merge_request_iid"], input["project_id"]),
		ListHint:   "Use gitlab_mr_emoji_list to list emojis on this merge request",
		VerifyHint: awardEmojiHintVerifyBasic,
	}
}

func mrNoteEmojiNotFound(input map[string]any) awardEmojiNotFoundOutput {
	return awardEmojiNotFoundOutput{
		Identifier: fmt.Sprintf("award %v on note %v (MR IID %v) in project %v", input["award_id"], input["note_id"], input["merge_request_iid"], input["project_id"]),
		ListHint:   "Use gitlab_mr_note_emoji_list to list emojis on this note",
		VerifyHint: awardEmojiHintVerifyWithNote,
	}
}

func snippetEmojiNotFound(input map[string]any) awardEmojiNotFoundOutput {
	return awardEmojiNotFoundOutput{
		Identifier: fmt.Sprintf("award %v on snippet IID %v in project %v", input["award_id"], input["snippet_id"], input["project_id"]),
		ListHint:   "Use gitlab_snippet_emoji_list to list emojis on this snippet",
		VerifyHint: awardEmojiHintVerifyBasic,
	}
}

func snippetNoteEmojiNotFound(input map[string]any) awardEmojiNotFoundOutput {
	return awardEmojiNotFoundOutput{
		Identifier: fmt.Sprintf("award %v on note %v (snippet IID %v) in project %v", input["award_id"], input["note_id"], input["snippet_id"], input["project_id"]),
		ListHint:   "Use gitlab_snippet_note_emoji_list to list emojis on this note",
		VerifyHint: awardEmojiHintVerifyWithNote,
	}
}
