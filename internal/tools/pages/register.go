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
	specs := ActionSpecs(client)
	pagesTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconFile})
	}

	mcp.AddTool(server, pagesTool("gitlab_pages_get", "Get Pages settings for a project. Returns URL, unique domain status, HTTPS enforcement, deployments, and primary domain.\n\nReturns: JSON with Pages configuration and deployment details.\n\nSee also: gitlab_pages_update, gitlab_pages_unpublish"), func(ctx context.Context, req *mcp.CallToolRequest, input GetPagesInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetPages(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pages_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatPagesMarkdown(out)), out, err)
	})

	mcp.AddTool(server, pagesTool("gitlab_pages_update", "Update Pages settings for a project. Can configure unique domain, HTTPS enforcement, and primary domain.\n\nReturns: JSON with the updated Pages settings.\n\nSee also: gitlab_pages_get"), func(ctx context.Context, req *mcp.CallToolRequest, input UpdatePagesInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := UpdatePages(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pages_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatPagesMarkdown(out)), out, err)
	})

	mcp.AddTool(server, pagesTool("gitlab_pages_unpublish", "Unpublish Pages for a project. Removes all published Pages content.\n\nReturns: confirmation message.\n\nSee also: gitlab_pages_get"), func(ctx context.Context, req *mcp.CallToolRequest, input UnpublishPagesInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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

	mcp.AddTool(server, pagesTool("gitlab_pages_domain_list_all", "List all Pages domains across all projects accessible to the authenticated user.\n\nReturns: JSON array of Pages domains.\n\nSee also: gitlab_pages_domain_list"), func(ctx context.Context, req *mcp.CallToolRequest, input ListAllDomainsInput) (*mcp.CallToolResult, ListAllDomainsOutput, error) {
		start := time.Now()
		out, err := ListAllDomains(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pages_domain_list_all", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatAllDomainsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, pagesTool("gitlab_pages_domain_list", "List Pages domains for a specific project. Supports pagination.\n\nReturns: JSON array of Pages domains with pagination.\n\nSee also: gitlab_pages_domain_get, gitlab_pages_domain_create"), func(ctx context.Context, req *mcp.CallToolRequest, input ListDomainsInput) (*mcp.CallToolResult, ListDomainsOutput, error) {
		start := time.Now()
		out, err := ListDomains(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pages_domain_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatDomainListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, pagesTool("gitlab_pages_domain_get", "Get a single Pages domain for a project, including certificate details.\n\nReturns: JSON with Pages domain details including SSL certificate information.\n\nSee also: gitlab_pages_domain_update, gitlab_pages_domain_delete"), func(ctx context.Context, req *mcp.CallToolRequest, input GetDomainInput) (*mcp.CallToolResult, DomainOutput, error) {
		start := time.Now()
		out, err := GetDomain(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pages_domain_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatDomainMarkdown(out)), out, err)
	})

	mcp.AddTool(server, pagesTool("gitlab_pages_domain_create", "Create a new Pages domain for a project. Optionally configure SSL certificate.\n\nReturns: JSON with the created Pages domain details.\n\nSee also: gitlab_pages_domain_get, gitlab_pages_domain_delete"), func(ctx context.Context, req *mcp.CallToolRequest, input CreateDomainInput) (*mcp.CallToolResult, DomainOutput, error) {
		start := time.Now()
		out, err := CreateDomain(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pages_domain_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatDomainMarkdown(out)), out, err)
	})

	mcp.AddTool(server, pagesTool("gitlab_pages_domain_update", "Update an existing Pages domain for a project. Can update SSL settings.\n\nReturns: JSON with the updated Pages domain details.\n\nSee also: gitlab_pages_domain_get"), func(ctx context.Context, req *mcp.CallToolRequest, input UpdateDomainInput) (*mcp.CallToolResult, DomainOutput, error) {
		start := time.Now()
		out, err := UpdateDomain(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pages_domain_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatDomainMarkdown(out)), out, err)
	})

	mcp.AddTool(server, pagesTool("gitlab_pages_domain_delete", "Delete a Pages domain from a project.\n\nReturns: confirmation message.\n\nSee also: gitlab_pages_domain_create"), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteDomainInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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
