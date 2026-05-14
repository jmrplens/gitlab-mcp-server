package repository

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
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
	options := repositoryOptions("gitlab_repository_compare")
	options.ReadOnly = true
	options.Idempotent = true
	options.Usage = "Compares two refs using params.from and params.to; use before analyze.release_notes when the task asks to inspect the diff."
	options.RelatedActions = append(options.RelatedActions, "analyze.release_notes", "release.list")
	return toolutil.NewActionSpec("compare", route, options)
}

func repositoryReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := repositoryOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func repositoryCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, repositoryOptions(individualTool))
}

func repositoryOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"repository", "git"},
		RelatedActions: []string{"branch.list", "tag.list"},
		OpenWorld:      true,
		OwnerPackage:   "repository",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
