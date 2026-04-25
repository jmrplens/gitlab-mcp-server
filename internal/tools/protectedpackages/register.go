// register.go wires protectedpackages MCP tools to the MCP server.

package protectedpackages

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers MCP tools for GitLab package protection rule operations.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_package_protection_rules",
		Title:       toolutil.TitleFromName("gitlab_list_package_protection_rules"),
		Description: "List all package protection rules for a GitLab project. Protection rules restrict who can push or delete matching packages.\n\nReturns: JSON with rules array including name pattern, package type, and minimum access levels. See also: gitlab_create_package_protection_rule.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconShield,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_package_protection_rules", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_create_package_protection_rule",
		Title:       toolutil.TitleFromName("gitlab_create_package_protection_rule"),
		Description: "Create a package protection rule for a GitLab project. Restricts push/delete operations on packages matching a name pattern.\n\nReturns: JSON with created rule details. See also: gitlab_list_package_protection_rules.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconShield,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_create_package_protection_rule", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_update_package_protection_rule",
		Title:       toolutil.TitleFromName("gitlab_update_package_protection_rule"),
		Description: "Update an existing package protection rule for a GitLab project.\n\nReturns: JSON with updated rule details. See also: gitlab_list_package_protection_rules.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconShield,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_update_package_protection_rule", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_package_protection_rule",
		Title:       toolutil.TitleFromName("gitlab_delete_package_protection_rule"),
		Description: "Delete a package protection rule from a GitLab project.\n\nReturns: JSON with deletion confirmation. See also: gitlab_list_package_protection_rules.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconShield,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete package protection rule %d from project %q?", input.RuleID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_package_protection_rule", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("package protection rule %d from project %s", input.RuleID, input.ProjectID))
	})
}
