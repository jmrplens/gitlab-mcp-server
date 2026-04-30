// register.go wires containerregistry MCP tools to the MCP server.
package containerregistry

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all container registry MCP tools.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_registry_list_project",
		Title:       toolutil.TitleFromName("gitlab_registry_list_project"),
		Description: "List container registry repositories for a GitLab project.\n\nSee also: gitlab_registry_list_tags, gitlab_registry_list_group\n\nReturns: JSON array of container repositories with pagination.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconContainer,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListProjectInput) (*mcp.CallToolResult, RepositoryListOutput, error) {
		start := time.Now()
		out, err := ListProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_registry_list_project", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatRepositoryListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_registry_list_group",
		Title:       toolutil.TitleFromName("gitlab_registry_list_group"),
		Description: "List container registry repositories for a GitLab group.\n\nSee also: gitlab_registry_list_project, gitlab_registry_get_repository\n\nReturns: JSON array of container repositories with pagination.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconContainer,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListGroupInput) (*mcp.CallToolResult, RepositoryListOutput, error) {
		start := time.Now()
		out, err := ListGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_registry_list_group", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatRepositoryListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_registry_get_repository",
		Title:       toolutil.TitleFromName("gitlab_registry_get_repository"),
		Description: "Get details of a single container registry repository by its ID.\n\nSee also: gitlab_registry_list_project, gitlab_registry_list_tags\n\nReturns: JSON with repository details.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconContainer,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetRepositoryInput) (*mcp.CallToolResult, RepositoryOutput, error) {
		start := time.Now()
		out, err := GetRepository(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_registry_get_repository", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatRepositoryMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_registry_delete_repository",
		Title:       toolutil.TitleFromName("gitlab_registry_delete_repository"),
		Description: "Delete a container registry repository. This action cannot be undone.\n\nSee also: gitlab_registry_list_project, gitlab_registry_get_repository\n\nReturns: confirmation message.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconContainer,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteRepositoryInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete container registry repository %d from project %s? This cannot be undone.", input.RepositoryID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := DeleteRepository(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_registry_delete_repository", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("registry repository")
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_registry_list_tags",
		Title:       toolutil.TitleFromName("gitlab_registry_list_tags"),
		Description: "List tags for a container registry repository.\n\nSee also: gitlab_registry_get_tag, gitlab_registry_get_repository\n\nReturns: JSON array of tags with pagination.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconContainer,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListTagsInput) (*mcp.CallToolResult, TagListOutput, error) {
		start := time.Now()
		out, err := ListTags(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_registry_list_tags", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatTagListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_registry_get_tag",
		Title:       toolutil.TitleFromName("gitlab_registry_get_tag"),
		Description: "Get details of a specific container registry repository tag.\n\nSee also: gitlab_registry_list_tags, gitlab_registry_delete_tag\n\nReturns: JSON with tag details.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconContainer,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetTagInput) (*mcp.CallToolResult, TagOutput, error) {
		start := time.Now()
		out, err := GetTag(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_registry_get_tag", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatTagMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_registry_delete_tag",
		Title:       toolutil.TitleFromName("gitlab_registry_delete_tag"),
		Description: "Delete a single container registry repository tag. This action cannot be undone.\n\nSee also: gitlab_registry_list_tags, gitlab_registry_delete_tags_bulk\n\nReturns: confirmation message.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconContainer,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteTagInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete registry tag %q from repository %d in project %s?", input.TagName, input.RepositoryID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := DeleteTag(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_registry_delete_tag", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("registry tag")
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_registry_delete_tags_bulk",
		Title:       toolutil.TitleFromName("gitlab_registry_delete_tags_bulk"),
		Description: "Delete container registry repository tags in bulk using regex patterns. Use name_regex_delete to match tags to delete and name_regex_keep to exclude tags from deletion.\n\nSee also: gitlab_registry_list_tags, gitlab_registry_delete_tag\n\nReturns: confirmation message.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconContainer,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteTagsBulkInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete registry tags in bulk from repository %d in project %s?", input.RepositoryID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := DeleteTagsBulk(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_registry_delete_tags_bulk", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("registry tags (bulk)")
	})

	// Protection Rules.

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_registry_protection_list",
		Title:       toolutil.TitleFromName("gitlab_registry_protection_list"),
		Description: "List container registry protection rules for a GitLab project.\n\nSee also: gitlab_registry_protection_create, gitlab_registry_list_project\n\nReturns: JSON array of protection rules.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconContainer,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListProtectionRulesInput) (*mcp.CallToolResult, ProtectionRuleListOutput, error) {
		start := time.Now()
		out, err := ListProtectionRules(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_registry_protection_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatProtectionRuleListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_registry_protection_create",
		Title:       toolutil.TitleFromName("gitlab_registry_protection_create"),
		Description: "Create a container registry protection rule to restrict push/delete access by minimum access level.\n\nSee also: gitlab_registry_protection_list, gitlab_registry_list_project\n\nReturns: JSON with the created protection rule details.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconContainer,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateProtectionRuleInput) (*mcp.CallToolResult, ProtectionRuleOutput, error) {
		start := time.Now()
		out, err := CreateProtectionRule(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_registry_protection_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatProtectionRuleMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_registry_protection_update",
		Title:       toolutil.TitleFromName("gitlab_registry_protection_update"),
		Description: "Update a container registry protection rule.\n\nSee also: gitlab_registry_protection_list, gitlab_registry_protection_create\n\nReturns: JSON with the updated protection rule details.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconContainer,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateProtectionRuleInput) (*mcp.CallToolResult, ProtectionRuleOutput, error) {
		start := time.Now()
		out, err := UpdateProtectionRule(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_registry_protection_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatProtectionRuleMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_registry_protection_delete",
		Title:       toolutil.TitleFromName("gitlab_registry_protection_delete"),
		Description: "Delete a container registry protection rule. This action cannot be undone.\n\nSee also: gitlab_registry_protection_list, gitlab_registry_protection_create\n\nReturns: confirmation message.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconContainer,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteProtectionRuleInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete registry protection rule %d from project %s?", input.RuleID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := DeleteProtectionRule(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_registry_protection_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("registry protection rule")
	})
}

// RegisterMeta registers the gitlab_registry and gitlab_registry_protection meta-tools.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := toolutil.ActionMap{
		"list_project":      toolutil.RouteAction(client, ListProject),
		"list_group":        toolutil.RouteAction(client, ListGroup),
		"get_repository":    toolutil.RouteAction(client, GetRepository),
		"delete_repository": toolutil.DestructiveVoidAction(client, DeleteRepository),
		"list_tags":         toolutil.RouteAction(client, ListTags),
		"get_tag":           toolutil.RouteAction(client, GetTag),
		"delete_tag":        toolutil.DestructiveVoidAction(client, DeleteTag),
		"delete_tags_bulk":  toolutil.DestructiveVoidAction(client, DeleteTagsBulk),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_registry",
		Title: toolutil.TitleFromName("gitlab_registry"),
		Description: `Manage container registry repositories and tags in GitLab. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- list_project: List project registry repos. Params: project_id (required), tags (bool), tags_count (bool), page, per_page
- list_group: List group registry repos. Params: group_id (required), page, per_page
- get_repository: Get single registry repo. Params: repository_id (required, int), tags (bool), tags_count (bool)
- delete_repository: Delete registry repo. Params: project_id (required), repository_id (required, int)
- list_tags: List tags for a repo. Params: project_id (required), repository_id (required, int), page, per_page
- get_tag: Get tag details. Params: project_id (required), repository_id (required, int), tag_name (required)
- delete_tag: Delete a single tag. Params: project_id (required), repository_id (required, int), tag_name (required)
- delete_tags_bulk: Bulk delete tags by regex. Params: project_id (required), repository_id (required, int), name_regex_delete, name_regex_keep, keep_n (int), older_than`,
		Annotations:  toolutil.DeriveAnnotations(routes),
		Icons:        toolutil.IconContainer,
		InputSchema:  toolutil.MetaToolSchema(routes),
		OutputSchema: toolutil.MetaToolOutputSchema(),
	}, toolutil.MakeMetaHandler("gitlab_registry", routes, nil))

	// Protection rules meta-tool
	protRoutes := toolutil.ActionMap{
		"list":   toolutil.RouteAction(client, ListProtectionRules),
		"create": toolutil.RouteAction(client, CreateProtectionRule),
		"update": toolutil.RouteAction(client, UpdateProtectionRule),
		"delete": toolutil.DestructiveVoidAction(client, DeleteProtectionRule),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_registry_protection",
		Title: toolutil.TitleFromName("gitlab_registry_protection"),
		Description: `Manage container registry protection rules in GitLab. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- list: List protection rules. Params: project_id (required)
- create: Create protection rule. Params: project_id (required), repository_path_pattern (required), minimum_access_level_for_push (maintainer/owner/admin), minimum_access_level_for_delete (maintainer/owner/admin)
- update: Update protection rule. Params: project_id (required), rule_id (required, int), repository_path_pattern, minimum_access_level_for_push, minimum_access_level_for_delete
- delete: Delete protection rule. Params: project_id (required), rule_id (required, int)`,
		Annotations:  toolutil.DeriveAnnotations(protRoutes),
		Icons:        toolutil.IconContainer,
		InputSchema:  toolutil.MetaToolSchema(protRoutes),
		OutputSchema: toolutil.MetaToolOutputSchema(),
	}, toolutil.MakeMetaHandler("gitlab_registry_protection", protRoutes, nil))
}
