// register.go wires keys MCP tools to the MCP server.

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

// RegisterMeta registers the gitlab_key meta-tool.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := toolutil.ActionMap{
		"get_key_with_user":      toolutil.RouteAction(client, GetKeyWithUser),
		"get_key_by_fingerprint": toolutil.RouteAction(client, GetKeyByFingerprint),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_key",
		Title: toolutil.TitleFromName("gitlab_key"),
		Description: `Look up GitLab SSH keys. Use 'action' to specify the operation.

Actions:
- get_key_with_user: Get an SSH key and its user by key ID. Params: key_id (required)
- get_key_by_fingerprint: Get an SSH key and its user by fingerprint. Params: fingerprint (required, e.g. SHA256:abc123)`,
		Annotations:  toolutil.DeriveAnnotations(routes),
		Icons:        toolutil.IconKey,
		InputSchema:  toolutil.MetaToolSchema(routes),
		OutputSchema: toolutil.MetaToolOutputSchema(routes),
	}, toolutil.MakeMetaHandler("gitlab_key", routes, nil))
}
