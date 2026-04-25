// register.go wires search MCP tools to the MCP server.

package search

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// markdownForResult dispatches search output types to their Markdown formatter.
func markdownForResult(result any) *mcp.CallToolResult {
	switch v := result.(type) {
	case CodeOutput:
		return toolutil.ToolResultWithMarkdown(FormatCodeMarkdown(v))
	case MergeRequestsOutput:
		return toolutil.ToolResultWithMarkdown(FormatMRsMarkdown(v))
	case IssuesOutput:
		return toolutil.ToolResultWithMarkdown(FormatIssuesMarkdown(v))
	case CommitsOutput:
		return toolutil.ToolResultWithMarkdown(FormatCommitsMarkdown(v))
	case MilestonesOutput:
		return toolutil.ToolResultWithMarkdown(FormatMilestonesMarkdown(v))
	case NotesOutput:
		return toolutil.ToolResultWithMarkdown(FormatNotesMarkdown(v))
	case ProjectsOutput:
		return toolutil.ToolResultWithMarkdown(FormatProjectsMarkdown(v))
	case SnippetsOutput:
		return toolutil.ToolResultWithMarkdown(FormatSnippetsMarkdown(v))
	case UsersOutput:
		return toolutil.ToolResultWithMarkdown(FormatUsersMarkdown(v))
	case WikiOutput:
		return toolutil.ToolResultWithMarkdown(FormatWikiMarkdown(v))
	default:
		return nil
	}
}

// RegisterTools registers all search-scoped MCP tools on the given server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_search_code",
		Title:       toolutil.TitleFromName("gitlab_search_code"),
		Description: "Search for code (blobs) in GitLab. Scope is determined by which ID you provide: set project_id for project scope, group_id for group scope, or neither for global scope. Only one scope at a time. Returns matching file name, path, ref, and a content snippet with pagination.\n\nReturns: JSON array of matching code blobs with pagination. See also: gitlab_file_get, gitlab_repository_tree.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconSearch,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CodeInput) (*mcp.CallToolResult, CodeOutput, error) {
		start := time.Now()
		out, err := Code(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_search_code", start, err)
		return toolutil.WithHints(markdownForResult(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_search_merge_requests",
		Title:       toolutil.TitleFromName("gitlab_search_merge_requests"),
		Description: "Search for merge requests by keyword. Searches within a project (project_id), a group (group_id), or globally. Returns matching merge requests with title, state, author, labels, and web URL with pagination.\n\nReturns: JSON array of matching merge requests with pagination. See also: gitlab_mr_get, gitlab_mr_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconSearch,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input MergeRequestsInput) (*mcp.CallToolResult, MergeRequestsOutput, error) {
		start := time.Now()
		out, err := MergeRequests(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_search_merge_requests", start, err)
		return toolutil.WithHints(markdownForResult(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_search_issues",
		Title:       toolutil.TitleFromName("gitlab_search_issues"),
		Description: "Search for issues by keyword. Searches within a project (project_id), a group (group_id), or globally. Returns matching issues with title, state, labels, assignees, and web URL with pagination.\n\nReturns: JSON array of matching issues with pagination. See also: gitlab_issue_get, gitlab_issue_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconSearch,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input IssuesInput) (*mcp.CallToolResult, IssuesOutput, error) {
		start := time.Now()
		out, err := Issues(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_search_issues", start, err)
		return toolutil.WithHints(markdownForResult(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_search_commits",
		Title:       toolutil.TitleFromName("gitlab_search_commits"),
		Description: "Search for commits by keyword. Searches within a project (project_id), a group (group_id), or globally. Returns matching commits with ID, title, author, date, and web URL with pagination.\n\nReturns: JSON array of matching commits with pagination. See also: gitlab_commit_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconSearch,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CommitsInput) (*mcp.CallToolResult, CommitsOutput, error) {
		start := time.Now()
		out, err := Commits(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_search_commits", start, err)
		return toolutil.WithHints(markdownForResult(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_search_milestones",
		Title:       toolutil.TitleFromName("gitlab_search_milestones"),
		Description: "Search for milestones by keyword. Searches within a project (project_id), a group (group_id), or globally. Returns matching milestones with title, state, dates, and web URL with pagination.\n\nReturns: JSON array of matching milestones with pagination. See also: gitlab_milestone_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconSearch,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input MilestonesInput) (*mcp.CallToolResult, MilestonesOutput, error) {
		start := time.Now()
		out, err := Milestones(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_search_milestones", start, err)
		return toolutil.WithHints(markdownForResult(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_search_notes",
		Title:       toolutil.TitleFromName("gitlab_search_notes"),
		Description: "Search for notes (comments) within a GitLab project by keyword. Returns matching notes with body, author, notable type/ID, and timestamps with pagination.\n\nReturns: JSON array of matching notes with pagination. See also: gitlab_issue_note_list, gitlab_mr_notes_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconSearch,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input NotesInput) (*mcp.CallToolResult, NotesOutput, error) {
		start := time.Now()
		out, err := Notes(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_search_notes", start, err)
		return toolutil.WithHints(markdownForResult(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_search_projects",
		Title:       toolutil.TitleFromName("gitlab_search_projects"),
		Description: "Search for projects by keyword. Searches within a group (group_id) or globally. Returns matching projects with name, path, visibility, and web URL with pagination.\n\nReturns: JSON array of matching projects with pagination. See also: gitlab_project_get, gitlab_project_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconSearch,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ProjectsInput) (*mcp.CallToolResult, ProjectsOutput, error) {
		start := time.Now()
		out, err := Projects(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_search_projects", start, err)
		return toolutil.WithHints(markdownForResult(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_search_snippets",
		Title:       toolutil.TitleFromName("gitlab_search_snippets"),
		Description: "Search for snippet titles globally in GitLab. Returns matching snippets with title, file name, description, author, and web URL with pagination.\n\nReturns: JSON array of matching snippets with pagination. See also: gitlab_snippet_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconSearch,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SnippetsInput) (*mcp.CallToolResult, SnippetsOutput, error) {
		start := time.Now()
		out, err := Snippets(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_search_snippets", start, err)
		return toolutil.WithHints(markdownForResult(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_search_users",
		Title:       toolutil.TitleFromName("gitlab_search_users"),
		Description: "Search for users by keyword. Searches within a project (project_id), a group (group_id), or globally. Returns matching users with username, name, state, and web URL with pagination.\n\nReturns: JSON array of matching users with pagination. See also: gitlab_get_user.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconSearch,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UsersInput) (*mcp.CallToolResult, UsersOutput, error) {
		start := time.Now()
		out, err := Users(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_search_users", start, err)
		return toolutil.WithHints(markdownForResult(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_search_wiki",
		Title:       toolutil.TitleFromName("gitlab_search_wiki"),
		Description: "Search for wiki blobs by keyword. Searches within a project (project_id), a group (group_id), or globally. Returns matching wiki pages with title, slug, content, and format with pagination.\n\nReturns: JSON array of matching wiki pages with pagination. See also: gitlab_wiki_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconSearch,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input WikiInput) (*mcp.CallToolResult, WikiOutput, error) {
		start := time.Now()
		out, err := Wiki(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_search_wiki", start, err)
		return toolutil.WithHints(markdownForResult(out), out, err)
	})
}

// RegisterMeta registers the gitlab_search meta-tool with all search
// scopes available in the GitLab Search API.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := toolutil.ActionMap{
		"code":           toolutil.RouteAction(client, Code),
		"merge_requests": toolutil.RouteAction(client, MergeRequests),
		"issues":         toolutil.RouteAction(client, Issues),
		"commits":        toolutil.RouteAction(client, Commits),
		"milestones":     toolutil.RouteAction(client, Milestones),
		"notes":          toolutil.RouteAction(client, Notes),
		"projects":       toolutil.RouteAction(client, Projects),
		"snippets":       toolutil.RouteAction(client, Snippets),
		"users":          toolutil.RouteAction(client, Users),
		"wiki":           toolutil.RouteAction(client, Wiki),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_search",
		Title: toolutil.TitleFromName("gitlab_search"),
		Description: `Search GitLab by scope (instance / group / project) for code, MRs, issues, commits, milestones, notes, projects, snippets, users, or wiki pages. Read-only.
Valid actions: ` + toolutil.ValidActionsString(routes) + `

When to use: full-text search across the supplied scope. Most actions accept project_id and / or group_id; if both are omitted the search runs at instance level (an authenticated user always has implicit instance scope on GitLab.com).
NOT for: discovering a project from a git remote (use gitlab_discover_project), listing labels / milestones / issues with structured filters (use gitlab_project, gitlab_issue, gitlab_merge_request — those support filters like state/labels/milestone), reading file contents (use gitlab_repository file_get).

Scope precedence: project_id > group_id > global. Pagination: page, per_page (max 100). All actions need query*.

Returns:
- code: array of {basename, data, path, ref, startline, project_id} blobs.
- merge_requests / issues: arrays of MR / issue objects.
- commits: array of {id, short_id, title, author_name, committed_date, project_id}.
- milestones / projects / snippets / users / wiki: arrays of resource summaries.
- notes: array of {id, body, notable_type, notable_id, notable_iid} entries.
All lists paginate with {page, per_page, total, next_page}.
Errors: 403 (hint: project_id / group_id must be visible to the caller), 404 (hint: project_id / group_id wrong or no permission), 400 (hint: query must not be empty; some scopes only support global — e.g. snippets).

- code: query*, project_id, group_id, ref
- merge_requests / issues / commits / milestones / users / wiki: query*, project_id, group_id
- notes: query*, project_id* (project-scoped only)
- projects: query*, group_id
- snippets: query* (global only)

See also: gitlab_discover_project (resolve git remote URL → project_id), gitlab_project / gitlab_merge_request / gitlab_issue (structured filtering).`,
		Annotations: toolutil.ReadOnlyMetaAnnotationsWithTitle("gitlab_search"),
		Icons:       toolutil.IconSearch,
		InputSchema: toolutil.MetaToolSchema(routes),
	}, toolutil.MakeMetaHandler("gitlab_search", routes, markdownForResult))
}
