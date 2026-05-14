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
	specs := ActionSpecs(client)
	registryTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconContainer})
	}

	mcp.AddTool(server, registryTool("gitlab_registry_list_project", "List container registry repositories for a GitLab project.\n\nSee also: gitlab_registry_list_tags, gitlab_registry_list_group\n\nReturns: JSON array of container repositories with pagination."), func(ctx context.Context, req *mcp.CallToolRequest, input ListProjectInput) (*mcp.CallToolResult, RepositoryListOutput, error) {
		start := time.Now()
		out, err := ListProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_registry_list_project", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatRepositoryListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, registryTool("gitlab_registry_list_group", "List container registry repositories for a GitLab group.\n\nSee also: gitlab_registry_list_project, gitlab_registry_get_repository\n\nReturns: JSON array of container repositories with pagination."), func(ctx context.Context, req *mcp.CallToolRequest, input ListGroupInput) (*mcp.CallToolResult, RepositoryListOutput, error) {
		start := time.Now()
		out, err := ListGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_registry_list_group", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatRepositoryListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, registryTool("gitlab_registry_get_repository", "Get details of a single container registry repository by its ID.\n\nSee also: gitlab_registry_list_project, gitlab_registry_list_tags\n\nReturns: JSON with repository details."), func(ctx context.Context, req *mcp.CallToolRequest, input GetRepositoryInput) (*mcp.CallToolResult, RepositoryOutput, error) {
		start := time.Now()
		out, err := GetRepository(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_registry_get_repository", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatRepositoryMarkdown(out)), out, err)
	})

	mcp.AddTool(server, registryTool("gitlab_registry_delete_repository", "Delete a container registry repository. This action cannot be undone.\n\nSee also: gitlab_registry_list_project, gitlab_registry_get_repository\n\nReturns: confirmation message."), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteRepositoryInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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

	mcp.AddTool(server, registryTool("gitlab_registry_list_tags", "List tags for a container registry repository.\n\nSee also: gitlab_registry_get_tag, gitlab_registry_get_repository\n\nReturns: JSON array of tags with pagination."), func(ctx context.Context, req *mcp.CallToolRequest, input ListTagsInput) (*mcp.CallToolResult, TagListOutput, error) {
		start := time.Now()
		out, err := ListTags(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_registry_list_tags", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatTagListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, registryTool("gitlab_registry_get_tag", "Get details of a specific container registry repository tag.\n\nSee also: gitlab_registry_list_tags, gitlab_registry_delete_tag\n\nReturns: JSON with tag details."), func(ctx context.Context, req *mcp.CallToolRequest, input GetTagInput) (*mcp.CallToolResult, TagOutput, error) {
		start := time.Now()
		out, err := GetTag(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_registry_get_tag", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatTagMarkdown(out)), out, err)
	})

	mcp.AddTool(server, registryTool("gitlab_registry_delete_tag", "Delete a single container registry repository tag. This action cannot be undone.\n\nSee also: gitlab_registry_list_tags, gitlab_registry_delete_tags_bulk\n\nReturns: confirmation message."), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteTagInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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

	mcp.AddTool(server, registryTool("gitlab_registry_delete_tags_bulk", "Delete container registry repository tags in bulk using regex patterns. Use name_regex_delete to match tags to delete and name_regex_keep to exclude tags from deletion.\n\nSee also: gitlab_registry_list_tags, gitlab_registry_delete_tag\n\nReturns: confirmation message."), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteTagsBulkInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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

	mcp.AddTool(server, registryTool("gitlab_registry_protection_list", "List container registry protection rules for a GitLab project.\n\nSee also: gitlab_registry_protection_create, gitlab_registry_list_project\n\nReturns: JSON array of protection rules."), func(ctx context.Context, req *mcp.CallToolRequest, input ListProtectionRulesInput) (*mcp.CallToolResult, ProtectionRuleListOutput, error) {
		start := time.Now()
		out, err := ListProtectionRules(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_registry_protection_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatProtectionRuleListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, registryTool("gitlab_registry_protection_create", "Create a container registry protection rule to restrict push/delete access by minimum access level.\n\nSee also: gitlab_registry_protection_list, gitlab_registry_list_project\n\nReturns: JSON with the created protection rule details."), func(ctx context.Context, req *mcp.CallToolRequest, input CreateProtectionRuleInput) (*mcp.CallToolResult, ProtectionRuleOutput, error) {
		start := time.Now()
		out, err := CreateProtectionRule(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_registry_protection_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatProtectionRuleMarkdown(out)), out, err)
	})

	mcp.AddTool(server, registryTool("gitlab_registry_protection_update", "Update a container registry protection rule.\n\nSee also: gitlab_registry_protection_list, gitlab_registry_protection_create\n\nReturns: JSON with the updated protection rule details."), func(ctx context.Context, req *mcp.CallToolRequest, input UpdateProtectionRuleInput) (*mcp.CallToolResult, ProtectionRuleOutput, error) {
		start := time.Now()
		out, err := UpdateProtectionRule(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_registry_protection_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatProtectionRuleMarkdown(out)), out, err)
	})

	mcp.AddTool(server, registryTool("gitlab_registry_protection_delete", "Delete a container registry protection rule. This action cannot be undone.\n\nSee also: gitlab_registry_protection_list, gitlab_registry_protection_create\n\nReturns: confirmation message."), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteProtectionRuleInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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
