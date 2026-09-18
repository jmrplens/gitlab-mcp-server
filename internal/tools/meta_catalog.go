package tools

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// RegisterMetaCatalog registers visible meta-tools from the canonical
// action catalog. Each catalog group becomes one visible MCP meta-tool
// whose name matches the group ToolName and whose routes come from the
// group's [actioncatalog.Group.ActionMap]. Read-only groups use
// [toolutil.AddReadOnlyMetaTool] so the registered annotations reflect
// the read-only semantics; mutating groups use [toolutil.AddMetaTool].
// Nil server or catalog inputs are accepted as no-ops.
func RegisterMetaCatalog(server *mcp.Server, catalog *actioncatalog.Catalog) {
	// There is deliberately no nil guard in front of this loop. Either nil is
	// already a no-op one call down — Groups() answers nil for a nil catalog,
	// and AddMetaTool and AddReadOnlyMetaTool both refuse a nil server — so an
	// early return here decided nothing any caller could observe, which is a
	// branch no test can hold to its meaning. The contract is stated by
	// [TestRegisterMetaCatalog_NilInputs] instead, over all three nil
	// combinations, so the day one of those refusals goes away it fails there
	// rather than passing here. Its sibling in RegisterIndividualCatalogTools
	// keeps its guard, since mcp.AddTool dereferences the server it is given.
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
