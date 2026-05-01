package tools

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// markdownForResult dispatches to the registered Markdown formatter based
// on the concrete type of result. Used by meta-tool handlers where the
// output type is any. Returns a success confirmation for nil (void actions).
func markdownForResult(result any) *mcp.CallToolResult {
	return toolutil.MarkdownForResult(result)
}
