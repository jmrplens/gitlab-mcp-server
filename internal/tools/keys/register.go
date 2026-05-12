package keys

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all key tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_key_with_user",
		Title:       toolutil.TitleFromName("gitlab_get_key_with_user"),
		Description: "Get an SSH key and its associated user by key ID.\n\nReturns: JSON with SSH key and user details.\n\nSee also: gitlab_get_key_by_fingerprint.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconKey,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetByIDInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetKeyWithUser(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_key_with_user", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_key_by_fingerprint",
		Title:       toolutil.TitleFromName("gitlab_get_key_by_fingerprint"),
		Description: "Get an SSH key and its user by SSH key fingerprint (SHA256: or MD5:).\n\nReturns: JSON with SSH key and user details.\n\nSee also: gitlab_get_key_with_user.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconKey,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetByFingerprintInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetKeyByFingerprint(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_key_by_fingerprint", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})
}
