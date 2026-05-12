package snippets

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

func snippetCreateInputSchema[T any]() *jsonschema.Schema {
	schema, err := jsonschema.For[T](nil)
	if err != nil {
		panic(fmt.Sprintf("snippet create input schema: %v", err))
	}
	addSnippetCreateFileRequirement(schema)
	return schema
}

func snippetCreateInputSchemaMap[T any]() map[string]any {
	data, err := json.Marshal(snippetCreateInputSchema[T]())
	if err != nil {
		panic(fmt.Sprintf("marshal snippet create input schema: %v", err))
	}
	var schema map[string]any
	if unmarshalErr := json.Unmarshal(data, &schema); unmarshalErr != nil {
		panic(fmt.Sprintf("unmarshal snippet create input schema: %v", unmarshalErr))
	}
	return schema
}

// CreateInputSchemaMap returns the input schema for personal snippet creation.
func CreateInputSchemaMap() map[string]any {
	return snippetCreateInputSchemaMap[CreateInput]()
}

// ProjectCreateInputSchemaMap returns the input schema for project snippet creation.
func ProjectCreateInputSchemaMap() map[string]any {
	return snippetCreateInputSchemaMap[ProjectCreateInput]()
}

func addSnippetCreateFileRequirement(schema *jsonschema.Schema) {
	if schema == nil {
		return
	}
	if schema.Properties == nil {
		schema.Properties = make(map[string]*jsonschema.Schema)
	}
	if files := schema.Properties["files"]; files != nil {
		minItems := 1
		files.MinItems = &minItems
	}
	schema.AnyOf = []*jsonschema.Schema{
		{Required: []string{"file_name", "content"}},
		{Required: []string{"files"}},
	}
}

// RegisterTools registers all snippet MCP tools (personal + project).
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	// Personal snippets.

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_snippet_list",
		Title:       toolutil.TitleFromName("gitlab_snippet_list"),
		Description: "List all snippets for the current authenticated user.\n\nReturns: JSON array of snippets with pagination. See also: gitlab_snippet_get, gitlab_snippet_create.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconSnippet,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_snippet_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_snippet_list_all",
		Title:       toolutil.TitleFromName("gitlab_snippet_list_all"),
		Description: "List all snippets across the GitLab instance (admin endpoint).\n\nReturns: JSON array of snippets with pagination. See also: gitlab_snippet_get, gitlab_snippet_create.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconSnippet,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListAllInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListAll(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_snippet_list_all", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_snippet_get",
		Title:       toolutil.TitleFromName("gitlab_snippet_get"),
		Description: "Get a single personal snippet by ID.\n\nReturns: JSON with snippet details. See also: gitlab_snippet_update.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconSnippet,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		if err != nil && toolutil.IsHTTPStatus(err, 404) {
			toolutil.LogToolCallAll(ctx, req, "gitlab_snippet_get", start, nil)
			return toolutil.NotFoundResult("Snippet", fmt.Sprintf("ID %d", input.SnippetID),
				"Use gitlab_snippet_list to list your snippets",
				"Verify the snippet_id is correct",
				"The snippet may be private or have been deleted",
			), Output{}, nil
		}
		toolutil.LogToolCallAll(ctx, req, "gitlab_snippet_get", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatMarkdown(out))
		if err == nil && out.ID != 0 {
			toolutil.EmbedResourceJSON(result,
				fmt.Sprintf("gitlab://snippet/%d", out.ID),
				out)
		}
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_snippet_content",
		Title:       toolutil.TitleFromName("gitlab_snippet_content"),
		Description: "Get the raw content of a personal snippet.\n\nReturns: raw snippet content. See also: gitlab_snippet_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconSnippet,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ContentInput) (*mcp.CallToolResult, ContentOutput, error) {
		start := time.Now()
		out, err := Content(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_snippet_content", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatContentMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_snippet_file_content",
		Title:       toolutil.TitleFromName("gitlab_snippet_file_content"),
		Description: "Get the raw content of a specific file in a snippet by ref and filename.\n\nReturns: raw snippet file content. See also: gitlab_snippet_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconSnippet,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input FileContentInput) (*mcp.CallToolResult, FileContentOutput, error) {
		start := time.Now()
		out, err := FileContent(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_snippet_file_content", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatFileContentMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_snippet_create",
		Title:       toolutil.TitleFromName("gitlab_snippet_create"),
		Description: "Create a new personal snippet. Use 'files' for multi-file snippets or 'file_name'+'content' for single-file.\n\nReturns: JSON with the created snippet details. See also: gitlab_snippet_get.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconSnippet,
		InputSchema: snippetCreateInputSchema[CreateInput](),
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_snippet_create", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatMarkdown(out))
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_snippet_update",
		Title:       toolutil.TitleFromName("gitlab_snippet_update"),
		Description: "Update an existing personal snippet.\n\nReturns: JSON with the updated snippet details. See also: gitlab_snippet_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconSnippet,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_snippet_update", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatMarkdown(out))
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_snippet_delete",
		Title:       toolutil.TitleFromName("gitlab_snippet_delete"),
		Description: "Delete a personal snippet. This action cannot be undone.\n\nReturns: confirmation message. See also: gitlab_snippet_list.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconSnippet,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete snippet %d? This cannot be undone.", input.SnippetID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_snippet_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("snippet")
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_snippet_explore",
		Title:       toolutil.TitleFromName("gitlab_snippet_explore"),
		Description: "List all public snippets on the GitLab instance.\n\nReturns: JSON array of public snippets with pagination. See also: gitlab_snippet_get, gitlab_snippet_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconSnippet,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ExploreInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := Explore(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_snippet_explore", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	// Project snippets.

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_snippet_list",
		Title:       toolutil.TitleFromName("gitlab_project_snippet_list"),
		Description: "List snippets for a GitLab project.\n\nReturns: JSON array of snippets with pagination. See also: gitlab_project_snippet_get, gitlab_project_snippet_create.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconSnippet,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ProjectListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ProjectList(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_snippet_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_snippet_get",
		Title:       toolutil.TitleFromName("gitlab_project_snippet_get"),
		Description: "Get a single project snippet by ID.\n\nReturns: JSON with snippet details. See also: gitlab_project_snippet_update.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconSnippet,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ProjectGetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := ProjectGet(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_snippet_get", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatMarkdown(out))
		if err == nil && out.ID != 0 && string(input.ProjectID) != "" {
			toolutil.EmbedResourceJSON(result,
				fmt.Sprintf("gitlab://project/%s/snippet/%d", url.PathEscape(string(input.ProjectID)), out.ID),
				out)
		}
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_snippet_content",
		Title:       toolutil.TitleFromName("gitlab_project_snippet_content"),
		Description: "Get the raw content of a project snippet.\n\nReturns: raw snippet content. See also: gitlab_project_snippet_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconSnippet,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ProjectContentInput) (*mcp.CallToolResult, ContentOutput, error) {
		start := time.Now()
		out, err := ProjectContent(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_snippet_content", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatContentMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_snippet_create",
		Title:       toolutil.TitleFromName("gitlab_project_snippet_create"),
		Description: "Create a new snippet in a GitLab project. Use 'files' for multi-file snippets or 'file_name'+'content' for single-file.\n\nReturns: JSON with the created snippet details. See also: gitlab_project_snippet_get.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconSnippet,
		InputSchema: snippetCreateInputSchema[ProjectCreateInput](),
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ProjectCreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := ProjectCreate(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_snippet_create", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatMarkdown(out))
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_snippet_update",
		Title:       toolutil.TitleFromName("gitlab_project_snippet_update"),
		Description: "Update an existing project snippet.\n\nReturns: JSON with the updated snippet details. See also: gitlab_project_snippet_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconSnippet,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ProjectUpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := ProjectUpdate(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_snippet_update", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatMarkdown(out))
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_snippet_delete",
		Title:       toolutil.TitleFromName("gitlab_project_snippet_delete"),
		Description: "Delete a project snippet. This action cannot be undone.\n\nReturns: confirmation message. See also: gitlab_project_snippet_list.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconSnippet,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ProjectDeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete snippet %d from project %s? This cannot be undone.", input.SnippetID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := ProjectDelete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_snippet_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("project snippet")
	})
}
