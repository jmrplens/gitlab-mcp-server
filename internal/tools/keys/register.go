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
	specs := ActionSpecs(client)
	keyTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconKey})
	}

	mcp.AddTool(server, keyTool("gitlab_get_key_with_user", "Get an SSH key and its associated user by key ID.\n\nReturns: JSON with SSH key and user details.\n\nSee also: gitlab_get_key_by_fingerprint."), func(ctx context.Context, req *mcp.CallToolRequest, input GetByIDInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetKeyWithUser(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_key_with_user", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, keyTool("gitlab_get_key_by_fingerprint", "Get an SSH key and its user by SSH key fingerprint (SHA256: or MD5:).\n\nReturns: JSON with SSH key and user details.\n\nSee also: gitlab_get_key_with_user."), func(ctx context.Context, req *mcp.CallToolRequest, input GetByFingerprintInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetKeyByFingerprint(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_key_by_fingerprint", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})
}
