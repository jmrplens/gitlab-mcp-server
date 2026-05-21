package search

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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

func searchRoute[T, R any](client *gitlabclient.Client, fn func(context.Context, *gitlabclient.Client, T) (R, error)) toolutil.ActionRoute {
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
