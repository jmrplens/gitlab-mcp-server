package awardemoji

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// SnippetActionSpecs returns canonical specs for snippet award emoji actions.
func SnippetActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		snippetEmojiReadSpec("emoji_snippet_list", toolutil.RouteAction(client, ListSnippetAwardEmoji), "gitlab_snippet_emoji_list"),
		snippetEmojiReadSpec("emoji_snippet_get", toolutil.RouteAction(client, GetSnippetAwardEmoji), "gitlab_snippet_emoji_get"),
		snippetEmojiCreateSpec("emoji_snippet_create", toolutil.RouteAction(client, CreateSnippetAwardEmoji), "gitlab_snippet_emoji_create"),
		snippetEmojiDeleteSpec("emoji_snippet_delete", toolutil.DestructiveVoidAction(client, DeleteSnippetAwardEmoji), "gitlab_snippet_emoji_delete"),
		snippetEmojiReadSpec("emoji_snippet_note_list", toolutil.RouteAction(client, ListSnippetNoteAwardEmoji), "gitlab_snippet_note_emoji_list"),
		snippetEmojiReadSpec("emoji_snippet_note_get", toolutil.RouteAction(client, GetSnippetNoteAwardEmoji), "gitlab_snippet_note_emoji_get"),
		snippetEmojiCreateSpec("emoji_snippet_note_create", toolutil.RouteAction(client, CreateSnippetNoteAwardEmoji), "gitlab_snippet_note_emoji_create"),
		snippetEmojiDeleteSpec("emoji_snippet_note_delete", toolutil.DestructiveVoidAction(client, DeleteSnippetNoteAwardEmoji), "gitlab_snippet_note_emoji_delete"),
	}
}

// IssueActionSpecs returns canonical specs for issue award emoji actions.
func IssueActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		issueEmojiReadSpec("emoji_issue_list", toolutil.RouteAction(client, ListIssueAwardEmoji), "gitlab_issue_emoji_list"),
		issueEmojiReadSpec("emoji_issue_get", toolutil.RouteAction(client, GetIssueAwardEmoji), "gitlab_issue_emoji_get"),
		issueEmojiCreateSpec("emoji_issue_create", toolutil.RouteAction(client, CreateIssueAwardEmoji), "gitlab_issue_emoji_create"),
		issueEmojiDeleteSpec("emoji_issue_delete", toolutil.DestructiveVoidAction(client, DeleteIssueAwardEmoji), "gitlab_issue_emoji_delete"),
		issueEmojiReadSpec("emoji_issue_note_list", toolutil.RouteAction(client, ListIssueNoteAwardEmoji), "gitlab_issue_note_emoji_list"),
		issueEmojiReadSpec("emoji_issue_note_get", toolutil.RouteAction(client, GetIssueNoteAwardEmoji), "gitlab_issue_note_emoji_get"),
		issueEmojiCreateSpec("emoji_issue_note_create", toolutil.RouteAction(client, CreateIssueNoteAwardEmoji), "gitlab_issue_note_emoji_create"),
		issueEmojiDeleteSpec("emoji_issue_note_delete", toolutil.DestructiveVoidAction(client, DeleteIssueNoteAwardEmoji), "gitlab_issue_note_emoji_delete"),
	}
}

func issueEmojiReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := issueEmojiOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func issueEmojiCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, issueEmojiOptions(individualTool))
}

func issueEmojiDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := issueEmojiOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func issueEmojiOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"issue", "emoji"},
		OpenWorld:      true,
		OwnerPackage:   "awardemoji",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

// MergeRequestActionSpecs returns canonical specs for merge request award emoji actions.
func MergeRequestActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		mergeRequestEmojiReadSpec("emoji_mr_list", toolutil.RouteAction(client, ListMRAwardEmoji), "gitlab_mr_emoji_list"),
		mergeRequestEmojiReadSpec("emoji_mr_get", toolutil.RouteAction(client, GetMRAwardEmoji), "gitlab_mr_emoji_get"),
		mergeRequestEmojiCreateSpec("emoji_mr_create", toolutil.RouteAction(client, CreateMRAwardEmoji), "gitlab_mr_emoji_create"),
		mergeRequestEmojiDeleteSpec("emoji_mr_delete", toolutil.DestructiveVoidAction(client, DeleteMRAwardEmoji), "gitlab_mr_emoji_delete"),
		mergeRequestEmojiReadSpec("emoji_mr_note_list", toolutil.RouteAction(client, ListMRNoteAwardEmoji), "gitlab_mr_note_emoji_list"),
		mergeRequestEmojiReadSpec("emoji_mr_note_get", toolutil.RouteAction(client, GetMRNoteAwardEmoji), "gitlab_mr_note_emoji_get"),
		mergeRequestEmojiCreateSpec("emoji_mr_note_create", toolutil.RouteAction(client, CreateMRNoteAwardEmoji), "gitlab_mr_note_emoji_create"),
		mergeRequestEmojiDeleteSpec("emoji_mr_note_delete", toolutil.DestructiveVoidAction(client, DeleteMRNoteAwardEmoji), "gitlab_mr_note_emoji_delete"),
	}
}

func mergeRequestEmojiReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := mergeRequestEmojiOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func mergeRequestEmojiCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, mergeRequestEmojiOptions(individualTool))
}

func mergeRequestEmojiDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := mergeRequestEmojiOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func mergeRequestEmojiOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"merge_request", "emoji"},
		OpenWorld:      true,
		OwnerPackage:   "awardemoji",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

func snippetEmojiReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := snippetEmojiOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func snippetEmojiCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, snippetEmojiOptions(individualTool))
}

func snippetEmojiDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := snippetEmojiOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func snippetEmojiOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"snippet", "emoji"},
		OpenWorld:      true,
		OwnerPackage:   "awardemoji",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
