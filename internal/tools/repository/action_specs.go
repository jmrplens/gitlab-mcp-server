package repository

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for repository browsing actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		repositoryReadSpec("tree", toolutil.RouteAction(client, Tree), "gitlab_repository_tree"),
		repositoryCompareSpec(toolutil.RouteAction(client, Compare)),
		repositoryReadSpec("contributors", toolutil.RouteAction(client, Contributors), "gitlab_repository_contributors"),
		repositoryReadSpec("merge_base", toolutil.RouteAction(client, MergeBase), "gitlab_repository_merge_base"),
		repositoryReadSpec("blob", toolutil.RouteAction(client, Blob), "gitlab_repository_blob"),
		repositoryReadSpec("raw_blob", toolutil.RouteAction(client, RawBlobContent), "gitlab_repository_raw_blob"),
		repositoryReadSpec("archive", toolutil.RouteAction(client, Archive), "gitlab_repository_archive"),
		repositoryCreateSpec("changelog_add", toolutil.RouteAction(client, AddChangelog), "gitlab_repository_changelog_add"),
		repositoryReadSpec("changelog_generate", toolutil.RouteAction(client, GenerateChangelogData), "gitlab_repository_changelog_generate"),
	}
}

func repositoryCompareSpec(route toolutil.ActionRoute) toolutil.ActionSpec {
	options := repositoryOptionsForAction("compare", "gitlab_repository_compare")
	options.Usage = "Compares two refs using params.from and params.to; use before analyze.release_notes when the task asks to inspect the diff."
	options.RelatedActions = append(options.RelatedActions, "analyze.release_notes", "release.list")
	options.IndividualTool.Description = "Compare two refs (branches, tags, or commits) in a project. Use from_project_id for cross-project comparison. Returns: commits, diffs, and comparison metadata. See also: gitlab_repository_tree, gitlab_branch_list."
	return toolutil.NewReadActionSpec("compare", route, options)
}

func repositoryReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, repositoryOptionsForAction(name, individualTool))
}

func repositoryCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, repositoryOptionsForAction(name, individualTool))
}

func repositoryOptionsForAction(actionName, individualTool string) toolutil.ActionSpecOptions {
	_ = actionName

	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute repository domain action.", Tags: []string{"repository", "git"},
		RelatedActions: []string{"branch.list", "tag.list"},
		OpenWorld:      true,
		OwnerPackage:   "repository",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}

	switch individualTool {
	case "gitlab_repository_tree":
		options.Usage = "List repository tree entries for a project/ref/path. Use this to browse directories and locate files before file/blob operations."
		options.Aliases = []string{"list repository files", "show repo tree", "browse repository"}
		options.RelatedActions = []string{"repository.blob", "repository.raw_blob", "branch.list"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": {
				SemanticRole:   "scope_project",
				ValueSource:    "Project ID or path containing the repository.",
				ExampleBinding: `params.project_id:"group/project"`,
			},
			"ref": {
				SemanticRole:   "git_ref",
				ValueSource:    "Branch/tag/commit to inspect; default branch when omitted.",
				ExampleBinding: `params.ref:"main"`,
			},
		}
		options.IndividualTool.Description = "List repository tree items. Returns: paths, object IDs, entry types (blob/tree), and pagination metadata. See also: gitlab_repository_blob, gitlab_repository_raw_blob, gitlab_branch_list."
	case "gitlab_repository_blob":
		options.Usage = "Get blob metadata/content for a specific file path and ref. Use when you need one file's content or metadata after locating its path."
		options.Aliases = []string{"get file blob", "show file content", "read repository blob"}
		options.RelatedActions = []string{"repository.tree", "repository.raw_blob"}
	case "gitlab_repository_changelog_add":
		options.Usage = "Create or append changelog entries in the repository for a version. Use in release workflows when structured changelog updates are requested."
		options.Aliases = []string{"add changelog", "update changelog", "write release notes file"}
		options.RelatedActions = []string{"repository.changelog_generate", "release.create", "tag.create"}
	}

	return options
}
