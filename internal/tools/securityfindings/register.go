// register.go wires security findings MCP tools to the MCP server.
package securityfindings

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers security findings tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_security_findings",
		Title:       toolutil.TitleFromName("gitlab_list_security_findings"),
		Description: "List security report findings for a pipeline (requires Ultimate/Premium). Replaces deprecated REST vulnerability_findings endpoint. Supports filtering by severity, confidence, scanner, and report type. Returns: paginated list with finding details, scanner info, and linked vulnerabilities. See also: gitlab_list_vulnerabilities, gitlab_pipeline_security_summary.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconVulnerability,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_security_findings", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatListMarkdown(out), toolutil.ContentList), out, err)
	})
}
