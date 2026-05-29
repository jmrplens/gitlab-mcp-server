package commits

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionRepositoryTree = "repository.tree"
	actionCommitDiff     = "commit.diff"
	actionCommitGet      = "commit.get"
)

// ActionSpecs returns canonical specs for commit actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		commitCreateSpec("commit_create", toolutil.RouteAction(client, Create), "gitlab_commit_create"),
		commitReadSpec("commit_list", toolutil.RouteAction(client, List), "gitlab_commit_list"),
		commitReadSpec("commit_get", toolutil.RouteAction(client, Get), "gitlab_commit_get"),
		commitReadSpec("commit_diff", toolutil.RouteAction(client, Diff), "gitlab_commit_diff"),
		commitReadSpec("commit_refs", toolutil.RouteAction(client, GetRefs), "gitlab_commit_refs"),
		commitReadSpec("commit_comments", toolutil.RouteAction(client, GetComments), "gitlab_commit_comments"),
		commitCreateSpec("commit_comment_create", toolutil.RouteAction(client, PostComment), "gitlab_commit_comment_create"),
		commitReadSpec("commit_statuses", toolutil.RouteAction(client, GetStatuses), "gitlab_commit_statuses"),
		commitUpdateSpec("commit_status_set", toolutil.RouteAction(client, SetStatus), "gitlab_commit_status_set"),
		commitReadSpec("commit_merge_requests", toolutil.RouteAction(client, ListMRsByCommit), "gitlab_commit_merge_requests"),
		commitCreateSpec("commit_cherry_pick", toolutil.RouteAction(client, CherryPick), "gitlab_commit_cherry_pick"),
		commitCreateSpec("commit_revert", toolutil.RouteAction(client, Revert), "gitlab_commit_revert"),
		commitReadSpec("commit_signature", toolutil.RouteAction(client, GetGPGSignature), "gitlab_commit_signature"),
		commitReadSpec("file_history", toolutil.RouteAction(client, List), "gitlab_commit_list"),
	}
}

func commitReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, commitOptionsForAction(name, individualTool))
}

func commitCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, commitOptionsForAction(name, individualTool))
}

func commitUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, commitOptionsForAction(name, individualTool))
}

func commitOptionsForAction(actionName, individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute commits domain action.", Tags: []string{"repository", "commit"},
		RelatedActions: []string{actionRepositoryTree, "branch.list"},
		OpenWorld:      true,
		OwnerPackage:   "commits",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}

	switch actionName {
	case "commit_list":
		options.Usage = "List commits for a project/ref with pagination. Use this to inspect history before selecting a commit for diff, comments, statuses, or revert/cherry-pick workflows."
		options.Aliases = []string{"list commits", "show commit history", "find commits"}
		options.RelatedActions = []string{actionCommitGet, actionCommitDiff, "commit.statuses"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": {
				SemanticRole:   "scope_project",
				ValueSource:    "Project numeric ID or full path that owns the repository.",
				ExampleBinding: `params.project_id:"group/project"`,
			},
		}
	case "file_history":
		options.Usage = "List commit history for repository content context. This action projects to the same individual tool as commit_list and should preserve the same discovery guidance."
		options.Aliases = []string{"file history", "history of changes", "list file commits"}
		options.RelatedActions = []string{actionCommitGet, actionCommitDiff, actionRepositoryTree}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": {
				SemanticRole:   "scope_project",
				ValueSource:    "Project numeric ID or full path that owns the repository.",
				ExampleBinding: `params.project_id:"group/project"`,
			},
		}
	case "commit_get":
		options.Usage = "Get detailed commit information by sha. Use this when a specific commit is referenced and full metadata/message/stats are needed."
		options.Aliases = []string{"get commit", "show commit details", "lookup commit"}
		options.RelatedActions = []string{"commit.list", actionCommitDiff, "commit.refs"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"sha": {
				SemanticRole:   "commit_sha",
				ValueSource:    "Commit SHA from list output, branch HEAD, or task context.",
				ExampleBinding: `params.sha:"abc123def"`,
			},
		}
	case "commit_create":
		options.Usage = "Create a commit by applying file actions to a branch. Use for multi-file changes where repository file APIs are not enough."
		options.Aliases = []string{"create commit", "commit file changes", "batch commit changes"}
		options.RelatedActions = []string{"branch.list", actionRepositoryTree, actionCommitGet}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"branch": {
				SemanticRole:   "git_branch",
				ValueSource:    "Target branch receiving commit actions.",
				ExampleBinding: `params.branch:"main"`,
			},
			"actions": {
				SemanticRole:   "commit_actions",
				ValueSource:    "Array of commit actions (create/update/delete/move/chmod) with file paths and content.",
				ExampleBinding: `params.actions:[{"action":"create","file_path":"README.md","content":"text"}]`,
			},
		}
	case "commit_status_set":
		options.Usage = "Set commit status/check state for a SHA. Use this for external CI/reporting integrations that need to update pipeline-like status checks."
		options.Aliases = []string{"set commit status", "update commit status", "report commit check"}
		options.RelatedActions = []string{"commit.statuses", "pipeline.list"}
	}

	return options
}
