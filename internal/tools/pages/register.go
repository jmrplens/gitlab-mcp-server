// register.go wires pages MCP tools to the MCP server.

package pages

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers individual Pages and Pages Domains tools.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	// PagesService tools
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_pages_get",
		Title:       toolutil.TitleFromName("gitlab_pages_get"),
		Description: "Get Pages settings for a project. Returns URL, unique domain status, HTTPS enforcement, deployments, and primary domain.\n\nReturns: JSON with Pages configuration and deployment details.\n\nSee also: gitlab_pages_update, gitlab_pages_unpublish",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconFile,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetPagesInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetPages(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pages_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatPagesMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_pages_update",
		Title:       toolutil.TitleFromName("gitlab_pages_update"),
		Description: "Update Pages settings for a project. Can configure unique domain, HTTPS enforcement, and primary domain.\n\nReturns: JSON with the updated Pages settings.\n\nSee also: gitlab_pages_get",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconFile,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdatePagesInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := UpdatePages(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pages_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatPagesMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_pages_unpublish",
		Title:       toolutil.TitleFromName("gitlab_pages_unpublish"),
		Description: "Unpublish Pages for a project. Removes all published Pages content.\n\nReturns: confirmation message.\n\nSee also: gitlab_pages_get",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconFile,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UnpublishPagesInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Unpublish Pages for project %s? All published content will be removed.", input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := UnpublishPages(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pages_unpublish", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("pages")
	})

	// PagesDomainsService tools
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_pages_domain_list_all",
		Title:       toolutil.TitleFromName("gitlab_pages_domain_list_all"),
		Description: "List all Pages domains across all projects accessible to the authenticated user.\n\nReturns: JSON array of Pages domains.\n\nSee also: gitlab_pages_domain_list",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconFile,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListAllDomainsInput) (*mcp.CallToolResult, ListAllDomainsOutput, error) {
		start := time.Now()
		out, err := ListAllDomains(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pages_domain_list_all", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatAllDomainsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_pages_domain_list",
		Title:       toolutil.TitleFromName("gitlab_pages_domain_list"),
		Description: "List Pages domains for a specific project. Supports pagination.\n\nReturns: JSON array of Pages domains with pagination.\n\nSee also: gitlab_pages_domain_get, gitlab_pages_domain_create",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconFile,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListDomainsInput) (*mcp.CallToolResult, ListDomainsOutput, error) {
		start := time.Now()
		out, err := ListDomains(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pages_domain_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatDomainListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_pages_domain_get",
		Title:       toolutil.TitleFromName("gitlab_pages_domain_get"),
		Description: "Get a single Pages domain for a project, including certificate details.\n\nReturns: JSON with Pages domain details including SSL certificate information.\n\nSee also: gitlab_pages_domain_update, gitlab_pages_domain_delete",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconFile,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetDomainInput) (*mcp.CallToolResult, DomainOutput, error) {
		start := time.Now()
		out, err := GetDomain(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pages_domain_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatDomainMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_pages_domain_create",
		Title:       toolutil.TitleFromName("gitlab_pages_domain_create"),
		Description: "Create a new Pages domain for a project. Optionally configure SSL certificate.\n\nReturns: JSON with the created Pages domain details.\n\nSee also: gitlab_pages_domain_get, gitlab_pages_domain_delete",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconFile,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateDomainInput) (*mcp.CallToolResult, DomainOutput, error) {
		start := time.Now()
		out, err := CreateDomain(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pages_domain_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatDomainMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_pages_domain_update",
		Title:       toolutil.TitleFromName("gitlab_pages_domain_update"),
		Description: "Update an existing Pages domain for a project. Can update SSL settings.\n\nReturns: JSON with the updated Pages domain details.\n\nSee also: gitlab_pages_domain_get",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconFile,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateDomainInput) (*mcp.CallToolResult, DomainOutput, error) {
		start := time.Now()
		out, err := UpdateDomain(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pages_domain_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatDomainMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_pages_domain_delete",
		Title:       toolutil.TitleFromName("gitlab_pages_domain_delete"),
		Description: "Delete a Pages domain from a project.\n\nReturns: confirmation message.\n\nSee also: gitlab_pages_domain_create",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconFile,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteDomainInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete Pages domain %q from project %s?", input.Domain, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := DeleteDomain(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pages_domain_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("pages domain %s", input.Domain))
	})
}

// RegisterMeta registers the gitlab_page meta-tool.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := toolutil.ActionMap{
		"get_pages":        toolutil.RouteAction(client, GetPages),
		"update_pages":     toolutil.RouteAction(client, UpdatePages),
		"unpublish_pages":  toolutil.DestructiveVoidAction(client, UnpublishPages),
		"list_all_domains": toolutil.RouteAction(client, ListAllDomains),
		"list_domains":     toolutil.RouteAction(client, ListDomains),
		"get_domain":       toolutil.RouteAction(client, GetDomain),
		"create_domain":    toolutil.RouteAction(client, CreateDomain),
		"update_domain":    toolutil.RouteAction(client, UpdateDomain),
		"delete_domain":    toolutil.DestructiveVoidAction(client, DeleteDomain),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_page",
		Title: toolutil.TitleFromName("gitlab_page"),
		Description: `Manage GitLab Pages and Pages domains. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- get_pages: Get Pages settings for a project. Params: project_id (required)
- update_pages: Update Pages settings. Params: project_id (required), pages_unique_domain_enabled, pages_https_only, pages_primary_domain
- unpublish_pages: Unpublish Pages for a project. Params: project_id (required)
- list_all_domains: List all Pages domains across all projects. No params required.
- list_domains: List Pages domains for a project. Params: project_id (required), page, per_page
- get_domain: Get a single Pages domain. Params: project_id (required), domain (required)
- create_domain: Create a Pages domain. Params: project_id (required), domain (required), auto_ssl_enabled, certificate, key
- update_domain: Update a Pages domain. Params: project_id (required), domain (required), auto_ssl_enabled, certificate, key
- delete_domain: Delete a Pages domain. Params: project_id (required), domain (required)`,
		Annotations: toolutil.DeriveAnnotations(routes),
		Icons:       toolutil.IconFile,
		InputSchema: toolutil.MetaToolSchema(routes),
	}, toolutil.MakeMetaHandler("gitlab_page", routes, nil))
}
