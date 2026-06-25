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
	options.Usage = "Diff two refs to see what changed between them; set params.from to the base ref and params.to to the target ref. Use for release diffs or pre-merge review."
	options.Aliases = []string{"diff two branches", "compare refs", "git diff between commits", "what changed between tags"}
	options.RelatedActions = []string{"repository.merge_base", "repository.tree", "branch.list", "release.list"}
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
		options.IndividualTool.Description = "Get a git blob by SHA. Returns: blob SHA, byte size, decoded text content (or image data), and a content category (text/image/binary). See also: gitlab_repository_raw_blob, gitlab_repository_tree."
	case "gitlab_repository_raw_blob":
		options.Usage = "Fetch the raw, unparsed bytes of a blob identified by its SHA. Use when you already have the blob SHA (e.g. from the tree) and want exact file content rather than metadata."
		options.Aliases = []string{"get raw blob by sha", "fetch blob bytes", "download blob content", "read raw blob"}
		options.RelatedActions = []string{"repository.blob", "repository.tree"}
		options.IndividualTool.Description = "Get the raw content of a git blob by SHA. Returns: blob SHA, byte size, decoded text content (or image data), and a content category (text/image/binary). See also: gitlab_repository_blob, gitlab_repository_tree."
	case "gitlab_repository_contributors":
		options.Usage = "List who has committed to the repository and how much, with per-author commit counts and line stats. Use to surface top authors or attribute ownership of a codebase."
		options.Aliases = []string{"list repository authors", "show commit counts per author", "who contributed to this repo", "top committers"}
		options.RelatedActions = []string{"repository.tree", "commit.list"}
		options.IndividualTool.Description = "List repository contributors. Returns: each contributor's name, email, commit count, additions, deletions, and pagination metadata. See also: gitlab_repository_tree, gitlab_commit_list."
	case "gitlab_repository_merge_base":
		options.Usage = "Find the most recent common ancestor commit shared by two or more refs. Use to compute a fork point before diffing or to understand branch divergence."
		options.Aliases = []string{"find common ancestor commit", "git merge-base", "fork point of two branches", "nearest shared commit"}
		options.RelatedActions = []string{"repository.compare", "branch.list"}
		options.IndividualTool.Description = "Find the common ancestor (merge base) of two or more refs. Returns: the merge-base commit with metadata and web URL. See also: gitlab_repository_compare, gitlab_branch_list."
	case "gitlab_repository_archive":
		options.Usage = "Build a downloadable archive URL (zip/tar.gz/tar.bz2/tar) for a repository ref or SHA. Use to export a snapshot of the source tree at a point in time."
		options.Aliases = []string{"download repository as zip", "export source snapshot", "get tarball url", "archive repo at ref"}
		options.RelatedActions = []string{"repository.tree", "tag.list"}
		options.IndividualTool.Description = "Build the download URL for a repository archive. Returns: the project, ref/SHA, format, and archive download URL (binary content is not returned). See also: gitlab_repository_tree, gitlab_tag_list."
	case "gitlab_repository_changelog_generate":
		options.Usage = "Render changelog notes for a version range from conventional commits without committing them. Use to preview release notes before writing them to a changelog file."
		options.Aliases = []string{"preview changelog notes", "generate release notes from commits", "build changelog for version", "render changelog without committing"}
		options.RelatedActions = []string{"repository.changelog_add", "release.create", "tag.list"}
		options.IndividualTool.Description = "Generate changelog notes for a version range without committing. Returns: the rendered changelog notes (read-only preview). See also: gitlab_repository_changelog_add, gitlab_release_create."
	case "gitlab_repository_changelog_add":
		options.Usage = "Create or append changelog entries in the repository for a version. Use in release workflows when structured changelog updates are requested."
		options.Aliases = []string{"add changelog", "update changelog", "write release notes file"}
		options.RelatedActions = []string{"repository.changelog_generate", "release.create", "tag.create"}
		options.IndividualTool.Description = "Add changelog data to a changelog file by committing the generated entries. Returns: a success confirmation with the version. See also: gitlab_repository_changelog_generate, gitlab_release_create."
	}

	return options
}
