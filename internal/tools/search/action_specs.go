package search

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for GitLab search actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		searchCodeSpec(client),
		searchReadSpec("merge_requests", searchRoute(client, MergeRequests), "gitlab_search_merge_requests"),
		searchReadSpec("issues", searchRoute(client, Issues), "gitlab_search_issues"),
		searchReadSpec("commits", searchRoute(client, Commits), "gitlab_search_commits"),
		searchReadSpec("milestones", searchRoute(client, Milestones), "gitlab_search_milestones"),
		searchReadSpec("notes", searchRoute(client, Notes), "gitlab_search_notes"),
		searchProjectsSpec(client),
		searchReadSpec("snippets", searchRoute(client, Snippets), "gitlab_search_snippets"),
		searchReadSpec("users", searchRoute(client, Users), "gitlab_search_users"),
		searchReadSpec("wiki", searchRoute(client, Wiki), "gitlab_search_wiki"),
	}
}

func searchCodeSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	options := searchReadOptions("gitlab_search_code")
	options.Usage = "Search code blobs and file contents. Use for text, symbols, snippets, or filenames inside repositories; do not use for project or repository name discovery."
	options.Aliases = []string{"code search", "file content search", "find code", "search repository files"}
	options.Tags = append(options.Tags, "code", "blob", "file_content")
	options.RelatedActions = []string{"search.projects", "repository.file_get", "repository.tree"}
	return toolutil.NewReadActionSpec("code", searchRoute(client, Code), options)
}

func searchProjectsSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	options := searchReadOptions("gitlab_search_projects")
	options.Usage = "Search project records by fuzzy project name, path fragment, namespace, or description. Use for broad discovery across many projects; if the prompt gives one exact namespace path like group/project and asks for metadata, use project.get instead. Do not use for code contents."
	options.Aliases = []string{"project search", "repository search", "find projects", "find repositories"}
	options.Tags = append(options.Tags, "project", "repository", "namespace")
	options.RelatedActions = []string{"project.get", "project.list", "search.code"}
	return toolutil.NewReadActionSpec("projects", searchRoute(client, Projects), options)
}

func searchReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, searchReadOptions(individualTool))
}

func searchReadOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute search domain action.", Tags: []string{"search"},
		OpenWorld:      true,
		OwnerPackage:   "search",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
