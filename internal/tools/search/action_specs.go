package search

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Canonical search action IDs referenced by RelatedActions metadata.
const (
	actionSearchCode          = "search.code"
	actionSearchProjects      = "search.projects"
	actionSearchMergeRequests = "search.merge_requests"
	actionSearchIssues        = "search.issues"
	actionSearchCommits       = "search.commits"
	actionSearchMilestones    = "search.milestones"
	actionSearchNotes         = "search.notes"
	actionSearchSnippets      = "search.snippets"
	actionSearchUsers         = "search.users"
	actionSearchWiki          = "search.wiki"
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
	options.RelatedActions = []string{actionSearchProjects, "repository.file_get", "repository.tree"}
	options.IndividualTool.Description = "Search code blobs across global, group, or project scope. Returns: matching blobs with file path, basename, ref, starting line, the surrounding snippet, project ID, and pagination metadata. See also: gitlab_search_projects, gitlab_file_get, gitlab_repository_tree."
	return toolutil.NewReadActionSpec("code", searchRoute(client, Code), options)
}

func searchProjectsSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	options := searchReadOptions("gitlab_search_projects")
	options.Usage = "Search project records by fuzzy project name, path fragment, namespace, or description. Use for broad discovery across many projects; if the prompt gives one exact namespace path like group/project and asks for metadata, use project.get instead. Do not use for code contents."
	options.Aliases = []string{"project search", "repository search", "find projects", "find repositories"}
	options.Tags = append(options.Tags, "project", "repository", "namespace")
	options.RelatedActions = []string{"project.get", "project.list", actionSearchCode}
	options.IndividualTool.Description = "Search projects globally or within a group by name, path, or description. Returns: matching projects with namespace, visibility, default branch, and web URL plus pagination metadata. See also: gitlab_project_get, gitlab_project_list, gitlab_search_code."
	return toolutil.NewReadActionSpec("projects", searchRoute(client, Projects), options)
}

func searchReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := searchReadOptions(individualTool)
	decorateSearchMeta(&options, individualTool)
	return toolutil.NewReadActionSpec(name, route, options)
}

// decorateSearchMeta fills non-generic Usage, natural-language Aliases,
// canonical RelatedActions, and the "Returns: … See also: …" individual-tool
// description for the per-scope search actions that would otherwise inherit
// the generic placeholder metadata from searchReadOptions. It is a no-op for
// tools whose dedicated spec builders (code, projects) already set rich
// metadata (1:1 audit R-META).
func decorateSearchMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	meta, ok := searchActionMeta[individualTool]
	if !ok {
		return
	}
	if meta.usage != "" {
		options.Usage = meta.usage
	}
	if len(meta.aliases) > 0 {
		options.Aliases = append([]string(nil), meta.aliases...)
	}
	if len(meta.tags) > 0 {
		options.Tags = append(options.Tags, meta.tags...)
	}
	if len(meta.related) > 0 {
		options.RelatedActions = append([]string(nil), meta.related...)
	}
	if meta.description != "" {
		options.IndividualTool.Description = meta.description
	}
}

// searchActionMetaEntry is the discovery metadata for one search action.
type searchActionMetaEntry struct {
	usage       string
	aliases     []string
	tags        []string
	related     []string
	description string
}

// searchActionMeta maps each per-scope search tool to its discovery metadata.
// The query runs across global, group, or project scope depending on whether
// group_id or project_id is supplied (project_id takes priority over group_id).
var searchActionMeta = map[string]searchActionMetaEntry{
	"gitlab_search_merge_requests": {
		usage:       "Search merge requests by title and description across global, group (group_id), or project (project_id) scope. Use when the prompt names keywords from an MR rather than a specific MR IID in a known project.",
		aliases:     []string{"search merge requests", "search MRs", "full-text MR search", "find MRs by content"},
		tags:        []string{"merge_request"},
		related:     []string{actionSearchIssues, "merge_request.list", "merge_request.get"},
		description: "Search merge requests across global, group, or project scope. Returns: matching merge requests with title, state, author, source and target branches, and web URL plus pagination metadata. See also: gitlab_search_issues, gitlab_mr_list, gitlab_mr_get.",
	},
	"gitlab_search_issues": {
		usage:       "Search issues by title and description across global, group (group_id), or project (project_id) scope. Use when the prompt gives keywords; if it already names a project plus an issue number, use issue.get instead.",
		aliases:     []string{"search issues", "search tickets", "full-text issue search", "find issues by content"},
		tags:        []string{"issue"},
		related:     []string{actionSearchMergeRequests, "issue.list", "issue.get"},
		description: "Search issues across global, group, or project scope. Returns: matching issues with title, state, labels, assignees, author, and web URL plus pagination metadata. See also: gitlab_search_merge_requests, gitlab_issue_list, gitlab_issue_get.",
	},
	"gitlab_search_commits": {
		usage:       "Search commit messages across global, group (group_id), or project (project_id) scope. Use to find commits by message keywords; use repository.commit_get when the SHA is already known.",
		aliases:     []string{"search commits", "search commit messages", "find commit by message", "full-text commit search"},
		tags:        []string{"commit"},
		related:     []string{actionSearchCode, "repository.commit_get", "repository.commit_list"},
		description: "Search commit messages across global, group, or project scope. Returns: matching commits with short and full SHA, title, message, author, and committed date plus pagination metadata. See also: gitlab_search_code, gitlab_commit_get, gitlab_commit_list.",
	},
	"gitlab_search_milestones": {
		usage:       "Search milestones by title and description across global, group (group_id), or project (project_id) scope. Use to locate a milestone by name when its ID is unknown.",
		aliases:     []string{"search milestones", "find milestone by name", "full-text milestone search"},
		tags:        []string{"milestone"},
		related:     []string{"milestone.list", "milestone.get", actionSearchIssues},
		description: "Search milestones across global, group, or project scope. Returns: matching milestones with title, description, state, start and due dates, and web URL plus pagination metadata. See also: gitlab_milestone_list, gitlab_milestone_get, gitlab_search_issues.",
	},
	"gitlab_search_notes": {
		usage:       "Search note bodies within a single project (project_id is required). Use to find comments on issues, merge requests, or commits that mention given keywords.",
		aliases:     []string{"search notes", "search comments", "find notes by content", "full-text note search"},
		tags:        []string{"note", "comment"},
		related:     []string{actionSearchIssues, actionSearchMergeRequests, "issue.notes_list"},
		description: "Search note bodies within one project. Returns: matching notes with body, author, noteable type and ID, system flag, and timestamps plus pagination metadata. See also: gitlab_search_issues, gitlab_search_merge_requests, gitlab_issue_note_list.",
	},
	"gitlab_search_snippets": {
		usage:       "Search snippet titles globally (no group or project scope). Use to locate a snippet by its title keywords across the instance.",
		aliases:     []string{"search snippets", "search snippet titles", "full-text snippet search"},
		tags:        []string{"snippet"},
		related:     []string{actionSearchCode, actionSearchProjects},
		description: "Search snippet titles globally. Returns: matching snippets with title, file name, description, visibility, author, project ID, web and raw URLs, and timestamps plus pagination metadata. See also: gitlab_search_code, gitlab_search_projects.",
	},
	"gitlab_search_users": {
		usage:       "Search users by name or username across global, group (group_id), or project (project_id) scope. Use to resolve a person to their user record before assigning or mentioning them.",
		aliases:     []string{"global user search", "find people by name", "look up user account", "full-text user search"},
		tags:        []string{"user"},
		related:     []string{"user.get", "user.list", "project.members_list"},
		description: "Search users across global, group, or project scope. Returns: matching users with ID, username, name, state, avatar URL, and web URL plus pagination metadata. See also: gitlab_get_user, gitlab_list_users, gitlab_project_members_list.",
	},
	"gitlab_search_wiki": {
		usage:       "Search wiki page contents across global, group (group_id), or project (project_id) scope. Use to find wiki pages by their body or title keywords.",
		aliases:     []string{"search wiki", "search wiki content", "find wiki by content", "full-text wiki search"},
		tags:        []string{"wiki", "blob"},
		related:     []string{actionSearchCode, "wiki.list", "wiki.get"},
		description: "Search wiki blobs across global, group, or project scope. Returns: matching wiki pages with slug, title, content snippet, and format plus pagination metadata. See also: gitlab_search_code, gitlab_wiki_list, gitlab_wiki_get.",
	},
}

func searchReadOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute search domain action.", Tags: []string{"search"},
		OpenWorld:      true,
		OwnerPackage:   "search",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
