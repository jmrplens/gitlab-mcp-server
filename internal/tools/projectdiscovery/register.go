package projectdiscovery

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers project discovery tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	for _, spec := range ActionSpecs(client) {
		toolutil.RegisterSurfaceToolFromSpec(server, spec, toolutil.SurfaceToolRegisterOptions{Icons: toolutil.IconProject})
	}
}
