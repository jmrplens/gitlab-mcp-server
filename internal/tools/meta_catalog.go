package tools

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/actionregistry"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterMetaCatalog registers visible meta-tools from the canonical action
// catalog.
func RegisterMetaCatalog(server *mcp.Server, catalog *actionregistry.Catalog) {
	if server == nil || catalog == nil {
		return
	}
	for _, group := range catalog.Groups() {
		formatResult := group.FormatResult
		if formatResult == nil {
			formatResult = markdownForResult
		}
		if group.ReadOnly {
			toolutil.AddReadOnlyMetaTool(server, group.ToolName, group.Description, group.ActionMap(), group.Icons, formatResult)
			continue
		}
		toolutil.AddMetaTool(server, group.ToolName, group.Description, group.ActionMap(), group.Icons, formatResult)
	}
}
