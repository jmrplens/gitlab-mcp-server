package search

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const searchTypeSchemaDescription = "Search backend to request. Use 'basic' for GitLab's default search, 'advanced' for Elasticsearch/OpenSearch-backed search, or 'zoekt' for Zoekt-based search. The requested backend must be enabled on the GitLab instance."

func searchTypeEnumValues() []any {
	values := make([]any, 0, len(allowedSearchTypes))
	for _, value := range allowedSearchTypes {
		values = append(values, value)
	}
	return values
}

func searchInputSchema[T any]() *jsonschema.Schema {
	schema, err := jsonschema.For[T](nil)
	if err != nil {
		panic(fmt.Sprintf("search input schema: %v", err))
	}
	if property := schema.Properties["search_type"]; property != nil {
		property.Description = searchTypeSchemaDescription
		property.Enum = searchTypeEnumValues()
	}
	return schema
}

func searchInputSchemaMap[T any]() map[string]any {
	data, err := json.Marshal(searchInputSchema[T]())
	searchSchemaPanic("marshal", err)
	var schema map[string]any
	searchSchemaPanic("unmarshal", json.Unmarshal(data, &schema))
	return schema
}

func searchSchemaPanic(operation string, err error) {
	if err != nil {
		panic(fmt.Sprintf("search input schema %s: %v", operation, err))
	}
}

func searchRoute[T any, R any](client *gitlabclient.Client, fn func(context.Context, *gitlabclient.Client, T) (R, error)) toolutil.ActionRoute {
	route := toolutil.RouteAction(client, fn)
	route.InputSchema = searchInputSchemaMap[T]()
	return route
}

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
	specs := ActionSpecs(client)
	searchTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconSearch})
	}

	mcp.AddTool(server, searchTool("gitlab_search_code", "Search for code (blobs) in GitLab. Scope is determined by which ID you provide: set project_id for project scope, group_id for group scope, or neither for global scope. Only one scope at a time. Returns matching file name, path, ref, and a content snippet with pagination.\n\nReturns: JSON array of matching code blobs with pagination. See also: gitlab_file_get, gitlab_repository_tree."), func(ctx context.Context, req *mcp.CallToolRequest, input CodeInput) (*mcp.CallToolResult, CodeOutput, error) {
		start := time.Now()
		out, err := Code(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, req.Params.Name, start, err)
		return toolutil.WithHints(markdownForResult(out), out, err)
	})

	mcp.AddTool(server, searchTool("gitlab_search_merge_requests", "Search for merge requests by keyword. Searches within a project (project_id), a group (group_id), or globally. Returns matching merge requests with title, state, author, labels, and web URL with pagination.\n\nReturns: JSON array of matching merge requests with pagination. See also: gitlab_mr_get, gitlab_mr_list."), func(ctx context.Context, req *mcp.CallToolRequest, input MergeRequestsInput) (*mcp.CallToolResult, MergeRequestsOutput, error) {
		start := time.Now()
		out, err := MergeRequests(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, req.Params.Name, start, err)
		return toolutil.WithHints(markdownForResult(out), out, err)
	})

	mcp.AddTool(server, searchTool("gitlab_search_issues", "Search for issues by keyword. Searches within a project (project_id), a group (group_id), or globally. Returns matching issues with title, state, labels, assignees, and web URL with pagination.\n\nReturns: JSON array of matching issues with pagination. See also: gitlab_issue_get, gitlab_issue_list."), func(ctx context.Context, req *mcp.CallToolRequest, input IssuesInput) (*mcp.CallToolResult, IssuesOutput, error) {
		start := time.Now()
		out, err := Issues(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, req.Params.Name, start, err)
		return toolutil.WithHints(markdownForResult(out), out, err)
	})

	mcp.AddTool(server, searchTool("gitlab_search_commits", "Search for commits by keyword. Searches within a project (project_id), a group (group_id), or globally. Returns matching commits with ID, title, author, date, and web URL with pagination.\n\nReturns: JSON array of matching commits with pagination. See also: gitlab_commit_get."), func(ctx context.Context, req *mcp.CallToolRequest, input CommitsInput) (*mcp.CallToolResult, CommitsOutput, error) {
		start := time.Now()
		out, err := Commits(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, req.Params.Name, start, err)
		return toolutil.WithHints(markdownForResult(out), out, err)
	})

	mcp.AddTool(server, searchTool("gitlab_search_milestones", "Search for milestones by keyword. Searches within a project (project_id), a group (group_id), or globally. Returns matching milestones with title, state, dates, and web URL with pagination.\n\nReturns: JSON array of matching milestones with pagination. See also: gitlab_milestone_get."), func(ctx context.Context, req *mcp.CallToolRequest, input MilestonesInput) (*mcp.CallToolResult, MilestonesOutput, error) {
		start := time.Now()
		out, err := Milestones(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, req.Params.Name, start, err)
		return toolutil.WithHints(markdownForResult(out), out, err)
	})

	mcp.AddTool(server, searchTool("gitlab_search_notes", "Search for notes (comments) within a GitLab project by keyword. Returns matching notes with body, author, notable type/ID, and timestamps with pagination.\n\nReturns: JSON array of matching notes with pagination. See also: gitlab_issue_note_list, gitlab_mr_notes_list."), func(ctx context.Context, req *mcp.CallToolRequest, input NotesInput) (*mcp.CallToolResult, NotesOutput, error) {
		start := time.Now()
		out, err := Notes(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, req.Params.Name, start, err)
		return toolutil.WithHints(markdownForResult(out), out, err)
	})

	mcp.AddTool(server, searchTool("gitlab_search_projects", "Search for projects by keyword. Searches within a group (group_id) or globally. Returns matching projects with name, path, visibility, and web URL with pagination.\n\nReturns: JSON array of matching projects with pagination. See also: gitlab_project_get, gitlab_project_list."), func(ctx context.Context, req *mcp.CallToolRequest, input ProjectsInput) (*mcp.CallToolResult, ProjectsOutput, error) {
		start := time.Now()
		out, err := Projects(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, req.Params.Name, start, err)
		return toolutil.WithHints(markdownForResult(out), out, err)
	})

	mcp.AddTool(server, searchTool("gitlab_search_snippets", "Search for snippet titles globally in GitLab. Returns matching snippets with title, file name, description, author, and web URL with pagination.\n\nReturns: JSON array of matching snippets with pagination. See also: gitlab_snippet_get."), func(ctx context.Context, req *mcp.CallToolRequest, input SnippetsInput) (*mcp.CallToolResult, SnippetsOutput, error) {
		start := time.Now()
		out, err := Snippets(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, req.Params.Name, start, err)
		return toolutil.WithHints(markdownForResult(out), out, err)
	})

	mcp.AddTool(server, searchTool("gitlab_search_users", "Search for users by keyword. Searches within a project (project_id), a group (group_id), or globally. Returns matching users with username, name, state, and web URL with pagination.\n\nReturns: JSON array of matching users with pagination. See also: gitlab_get_user."), func(ctx context.Context, req *mcp.CallToolRequest, input UsersInput) (*mcp.CallToolResult, UsersOutput, error) {
		start := time.Now()
		out, err := Users(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, req.Params.Name, start, err)
		return toolutil.WithHints(markdownForResult(out), out, err)
	})

	mcp.AddTool(server, searchTool("gitlab_search_wiki", "Search for wiki blobs by keyword. Searches within a project (project_id), a group (group_id), or globally. Returns matching wiki pages with title, slug, content, and format with pagination.\n\nReturns: JSON array of matching wiki pages with pagination. See also: gitlab_wiki_get."), func(ctx context.Context, req *mcp.CallToolRequest, input WikiInput) (*mcp.CallToolResult, WikiOutput, error) {
		start := time.Now()
		out, err := Wiki(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, req.Params.Name, start, err)
		return toolutil.WithHints(markdownForResult(out), out, err)
	})
}
