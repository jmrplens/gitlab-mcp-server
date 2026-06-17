package tools

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// RegisterMetaCatalog registers visible meta-tools from the canonical
// action catalog. Each catalog group becomes one visible MCP meta-tool
// whose name matches the group ToolName and whose routes come from the
// group's [actioncatalog.Group.ActionMap]. Read-only groups use
// [toolutil.AddReadOnlyMetaTool] so the registered annotations reflect
// the read-only semantics; mutating groups use [toolutil.AddMetaTool].
// Nil server or catalog inputs are accepted as no-ops.
func RegisterMetaCatalog(server *mcp.Server, catalog *actioncatalog.Catalog) {
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
