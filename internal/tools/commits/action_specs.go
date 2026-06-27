package commits

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionRepositoryTree = "repository.tree"
	actionCommitDiff     = "commit.diff"
	actionCommitGet      = "commit.get"
	actionCommitList     = "commit.list"
	actionCommitStatuses = "commit.statuses"
	actionBranchList     = "branch.list"
)

// ActionSpecs returns canonical specs for commit actions exposed as MCP tools.
// Every read, create, update, and history route is projected into the dynamic,
// meta, individual, and audit surfaces by the action catalog (ADR-0004).
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

// projectGuidance is the shared parameter guidance for the project_id input,
// which every commit action requires.
func projectGuidance() toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		SemanticRole:   "scope_project",
		ValueSource:    "Project numeric ID or full path that owns the repository.",
		ExampleBinding: `params.project_id:"group/project"`,
	}
}

// shaGuidance is the shared parameter guidance for the sha input used by
// per-commit read/write actions.
func shaGuidance() toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		SemanticRole:   "commit_sha",
		ValueSource:    "Commit SHA from list output, branch HEAD, or task context.",
		ExampleBinding: `params.sha:"abc123def"`,
	}
}

// commitOptionsForAction returns action-specific discovery metadata (usage,
// natural-language aliases, canonical related actions, parameter guidance, and
// the individual-tool description) for each commit action (R-META).
func commitOptionsForAction(actionName, individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases:        []string{individualTool},
		Usage:          "Use to execute a commits domain action.",
		Tags:           []string{"repository", "commit"},
		RelatedActions: []string{actionRepositoryTree, actionBranchList},
		OpenWorld:      true,
		OwnerPackage:   "commits",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
		ParameterGuidance: map[string]toolutil.ParameterGuidance{
			"project_id": projectGuidance(),
		},
	}

	switch actionName {
	case "commit_list":
		options.Usage = "List commits for a project/ref with date, path, and author filters plus offset or keyset pagination. Use this to inspect history before selecting a commit for diff, comments, statuses, or revert/cherry-pick workflows."
		options.Aliases = []string{"list commits", "show commit history", "find commits", "git log"}
		options.RelatedActions = []string{actionCommitGet, actionCommitDiff, actionCommitStatuses, actionBranchList}
		options.IndividualTool.Description = "List repository commits for a project, optionally filtered by ref_name, since/until, path, or author. Returns: commit summaries (SHA, title, author, dates, parent IDs, stats, trailers, last pipeline) with pagination metadata. See also: gitlab_commit_get, gitlab_commit_diff, gitlab_commit_statuses, gitlab_branch_list."
	case "file_history":
		options.Usage = "List commit history for repository content context with path/ref filters and pagination. This action projects to the same individual tool as commit_list and preserves the same discovery guidance."
		options.Aliases = []string{"file history", "history of changes", "list file commits", "log for path"}
		options.RelatedActions = []string{actionCommitGet, actionCommitDiff, actionRepositoryTree}
		options.IndividualTool.Description = "List repository commits for a project, optionally filtered by ref_name, since/until, path, or author. Returns: commit summaries (SHA, title, author, dates, parent IDs, stats, trailers, last pipeline) with pagination metadata. See also: gitlab_commit_get, gitlab_commit_diff, gitlab_commit_statuses, gitlab_branch_list."
	case "commit_get":
		options.Usage = "Get detailed commit information by sha. Use this when a specific commit is referenced and full metadata, message, stats, trailers, or last pipeline are needed; set stats=true to force inclusion of line stats."
		options.Aliases = []string{"get commit", "show commit details", "lookup commit", "describe commit"}
		options.RelatedActions = []string{actionCommitList, actionCommitDiff, "commit.refs", actionCommitStatuses}
		options.ParameterGuidance["sha"] = shaGuidance()
		options.IndividualTool.Description = "Get a single commit from a project by SHA (full, short, branch, or tag). Returns: full commit metadata, message, parent IDs, author/committer, dates, stats, trailers, project ID, and last pipeline. See also: gitlab_commit_list, gitlab_commit_diff, gitlab_commit_refs, gitlab_commit_statuses."
	case "commit_diff":
		options.Usage = "Get the file diffs for a single commit with offset or keyset pagination. Use unidiff=true for a git-compatible unified-diff format when applying or rendering changes."
		options.Aliases = []string{"commit diff", "show commit changes", "get commit diff", "diff commit"}
		options.RelatedActions = []string{actionCommitGet, actionCommitList, "commit.comments"}
		options.ParameterGuidance["sha"] = shaGuidance()
		options.IndividualTool.Description = "Get the file diffs of a single commit, optionally in unified-diff format. Returns: per-file diffs (old/new path, diff body, new/renamed/deleted flags, file modes) with pagination metadata. See also: gitlab_commit_get, gitlab_commit_list, gitlab_commit_comments."
	case "commit_refs":
		options.Usage = "List the branches and tags that contain a commit, filtered by type (branch, tag, or all) with pagination. Use this to discover where a commit has been merged or released."
		options.Aliases = []string{"commit refs", "branches containing commit", "tags containing commit", "where is commit"}
		options.RelatedActions = []string{actionCommitGet, actionBranchList, "tag.list"}
		options.ParameterGuidance["sha"] = shaGuidance()
		options.IndividualTool.Description = "List branches and tags that reference (contain) a commit, optionally filtered by ref type. Returns: ref entries with type and name plus pagination metadata. See also: gitlab_commit_get, gitlab_branch_list, gitlab_tag_list."
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaPropertyOverride("type", map[string]any{"enum": []any{"branch", "tag", "all"}}),
		}
	case "commit_comments":
		options.Usage = "List the comments posted on a commit with pagination. Use this before adding a comment or to review existing review notes attached to a commit."
		options.Aliases = []string{"commit comments", "list commit comments", "show commit notes", "read commit discussion"}
		options.RelatedActions = []string{"commit.comment_create", actionCommitGet, actionCommitDiff}
		options.ParameterGuidance["sha"] = shaGuidance()
		options.IndividualTool.Description = "List the comments on a commit. Returns: comment entries with note text, author user object, optional file path, line, and line type, plus pagination metadata. See also: gitlab_commit_comment_create, gitlab_commit_get, gitlab_commit_diff."
	case "commit_comment_create":
		options.Usage = "Post a comment on a commit, optionally inline by supplying path, line, and line_type. Use this to leave review feedback on a specific commit or changed line."
		options.Aliases = []string{"comment on commit", "add commit comment", "post commit note", "review commit line"}
		options.RelatedActions = []string{"commit.comments", actionCommitDiff, actionCommitGet}
		options.ParameterGuidance["sha"] = shaGuidance()
		options.ParameterGuidance["note"] = toolutil.ParameterGuidance{
			SemanticRole:   "comment_body",
			ValueSource:    "Markdown comment text to attach to the commit.",
			ExampleBinding: `params.note:"Looks good to me"`,
		}
		options.IndividualTool.Description = "Post a comment on a commit, optionally inline on a specific file path and line. Returns: the created comment with note text, author user object, and optional path/line/line_type. See also: gitlab_commit_comments, gitlab_commit_diff, gitlab_commit_get."
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaPropertyOverride("line_type", map[string]any{"enum": []any{"new", "old"}}),
		}
	case "commit_statuses":
		options.Usage = "List the pipeline/external statuses of a commit, filtered by ref, stage, name, or pipeline_id, with pagination. Use this to check CI/CD or integration check results for a commit."
		options.Aliases = []string{"commit statuses", "list commit statuses", "show commit checks", "ci status for commit"}
		options.RelatedActions = []string{"commit.status_set", actionCommitGet, "pipeline.list"}
		options.ParameterGuidance["sha"] = shaGuidance()
		options.IndividualTool.Description = "List the statuses of a commit (CI jobs and external integrations), filtered by ref, stage, name, or pipeline. Returns: status entries with state, name, ref, coverage, pipeline ID, timestamps, and author user object, plus pagination metadata. See also: gitlab_commit_status_set, gitlab_commit_get, gitlab_pipeline_list."
	case "commit_status_set":
		options.Usage = "Set the pipeline/check status of a commit SHA (pending, running, success, failed, canceled, skipped). Use this for external CI or reporting integrations that publish commit check results."
		options.Aliases = []string{"set commit status", "update commit status", "report commit check", "publish commit status"}
		options.RelatedActions = []string{actionCommitStatuses, actionCommitGet, "pipeline.list"}
		options.ParameterGuidance["sha"] = shaGuidance()
		options.ParameterGuidance["state"] = toolutil.ParameterGuidance{
			SemanticRole:   "status_state",
			ValueSource:    "Target state: pending, running, success, failed, canceled, or skipped.",
			ExampleBinding: `params.state:"success"`,
		}
		options.IndividualTool.Description = "Set or update the pipeline status of a commit (used by external CI/reporting integrations). Returns: the resulting status with state, name, ref, coverage, pipeline ID, timestamps, and author user object. See also: gitlab_commit_statuses, gitlab_commit_get, gitlab_pipeline_list."
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaPropertyOverride("state", map[string]any{"enum": []any{"pending", "running", "success", "failed", "canceled", "skipped"}}),
		}
	case "commit_merge_requests":
		options.Usage = "List the merge requests that include a commit. Use this to trace which MR introduced or carries a commit."
		options.Aliases = []string{"merge requests for commit", "mrs containing commit", "list commit merge requests", "which mr has commit"}
		options.RelatedActions = []string{actionCommitGet, "mr.get", "mr.changes_get"}
		options.ParameterGuidance["sha"] = shaGuidance()
		options.IndividualTool.Description = "List merge requests associated with a commit. Returns: basic MR entries with IID, title, state, source/target branches, web URL, and author. See also: gitlab_commit_get, gitlab_mr_get, gitlab_mr_changes_get."
	case "commit_cherry_pick":
		options.Usage = "Cherry-pick a commit onto a target branch, optionally as a dry run to detect conflicts before creating the commit. Use this to port a single change to another branch."
		options.Aliases = []string{"cherry-pick commit", "cherry pick", "apply commit to branch", "port commit"}
		options.RelatedActions = []string{actionCommitGet, actionBranchList, "commit.revert"}
		options.ParameterGuidance["sha"] = shaGuidance()
		options.ParameterGuidance["branch"] = toolutil.ParameterGuidance{
			SemanticRole:   "git_branch",
			ValueSource:    "Target branch to receive the cherry-picked commit.",
			ExampleBinding: `params.branch:"main"`,
		}
		options.IndividualTool.Description = "Cherry-pick a commit onto a target branch, optionally as a dry run. Returns: the newly created commit (SHA, title, author, dates, web URL) or a conflict error. See also: gitlab_commit_get, gitlab_branch_list, gitlab_commit_revert."
	case "commit_revert":
		options.Usage = "Revert a commit on a target branch, creating a new commit that undoes its changes. Use this to back out a change that has already been merged."
		options.Aliases = []string{"revert commit", "undo commit", "back out commit", "create revert"}
		options.RelatedActions = []string{actionCommitGet, actionBranchList, "commit.cherry_pick"}
		options.ParameterGuidance["sha"] = shaGuidance()
		options.ParameterGuidance["branch"] = toolutil.ParameterGuidance{
			SemanticRole:   "git_branch",
			ValueSource:    "Target branch to receive the revert commit.",
			ExampleBinding: `params.branch:"main"`,
		}
		options.IndividualTool.Description = "Revert a commit on a target branch, creating a new commit that undoes it. Returns: the newly created revert commit (SHA, title, author, dates, web URL) or a conflict error. See also: gitlab_commit_get, gitlab_branch_list, gitlab_commit_cherry_pick."
	case "commit_signature":
		options.Usage = "Get the GPG, X.509, or SSH signature of a commit. Use this to verify commit authenticity; a 404 is also returned for unsigned commits."
		options.Aliases = []string{"commit signature", "gpg signature", "verify commit signature", "commit signing key"}
		options.RelatedActions = []string{actionCommitGet, actionCommitList}
		options.ParameterGuidance["sha"] = shaGuidance()
		options.IndividualTool.Description = "Get the signature (GPG/X.509/SSH) of a commit for authenticity verification. Returns: the signing key ID, primary key ID, signer name/email, verification status, and optional subkey ID. See also: gitlab_commit_get, gitlab_commit_list."
	case "commit_create":
		options.Usage = "Create a commit by applying one or more file actions (create/update/delete/move/chmod) to a branch in a single API call. Use for multi-file changes where single-file repository APIs are not enough."
		options.Aliases = []string{"create commit", "commit file changes", "batch commit changes", "commit multiple files"}
		options.RelatedActions = []string{actionBranchList, actionRepositoryTree, actionCommitGet}
		options.ParameterGuidance["branch"] = toolutil.ParameterGuidance{
			SemanticRole:   "git_branch",
			ValueSource:    "Target branch receiving the commit actions.",
			ExampleBinding: `params.branch:"main"`,
		}
		options.ParameterGuidance["actions"] = toolutil.ParameterGuidance{
			SemanticRole:   "commit_actions",
			ValueSource:    "Array of commit actions (create/update/delete/move/chmod) with file paths and content.",
			ExampleBinding: `params.actions:[{"action":"create","file_path":"README.md","content":"text"}]`,
		}
		options.IndividualTool.Description = "Create a commit with multiple file actions (create/update/delete/move/chmod) on a branch in one call. Returns: the created commit with SHA, title, author/committer, dates, parent IDs, stats, trailers, and web URL. See also: gitlab_branch_list, gitlab_repository_tree, gitlab_commit_get."
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaPropertyOverride("actions.action", map[string]any{"enum": []any{"create", "delete", "move", "update", "chmod"}}),
		}
	}

	return options
}
