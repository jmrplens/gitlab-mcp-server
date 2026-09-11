package tools

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// markdownForResult dispatches to the registered Markdown formatter based
// on the concrete type of result, a pointer to a registered type included.
// Used by the meta and individual dispatchers, where the output type is any.
// Returns a success confirmation for nil (void actions) and nil for a type
// with no formatter, which [toolutil.FinishToolResult] then renders as JSON.
// Delegates to [toolutil.MarkdownForResult], which walks the type registry.
func markdownForResult(result any) *mcp.CallToolResult {
	return toolutil.MarkdownForResult(result)
}
