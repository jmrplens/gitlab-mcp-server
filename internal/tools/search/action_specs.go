package search

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for GitLab search actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		searchReadSpec("code", searchRoute(client, Code), "gitlab_search_code"),
		searchReadSpec("merge_requests", searchRoute(client, MergeRequests), "gitlab_search_merge_requests"),
		searchReadSpec("issues", searchRoute(client, Issues), "gitlab_search_issues"),
		searchReadSpec("commits", searchRoute(client, Commits), "gitlab_search_commits"),
		searchReadSpec("milestones", searchRoute(client, Milestones), "gitlab_search_milestones"),
		searchReadSpec("notes", searchRoute(client, Notes), "gitlab_search_notes"),
		searchReadSpec("projects", searchRoute(client, Projects), "gitlab_search_projects"),
		searchReadSpec("snippets", searchRoute(client, Snippets), "gitlab_search_snippets"),
		searchReadSpec("users", searchRoute(client, Users), "gitlab_search_users"),
		searchReadSpec("wiki", searchRoute(client, Wiki), "gitlab_search_wiki"),
	}
}

func searchReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, toolutil.ActionSpecOptions{
		Tags:           []string{"search"},
		ReadOnly:       true,
		Idempotent:     true,
		OpenWorld:      true,
		OwnerPackage:   "search",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	})
}
