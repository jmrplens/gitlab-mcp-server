// common.go provides shared helpers for sampling tool implementations.

package samplingtools

import (
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// SamplingUnsupportedResult returns a structured error tool result when the
// MCP client does not support sampling. Suggests alternative non-sampling tools.
func SamplingUnsupportedResult(toolName string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf(
				"Tool %q requires the MCP sampling capability (createMessage). "+
					"Your MCP client does not support sampling. "+
					"Check your client's MCP documentation for sampling support.\n\n"+
					"**Alternatives without sampling**:\n"+
					"- For MR diffs: use gitlab_merge_request action 'changes_get'\n"+
					"- For issue details: use gitlab_issue action 'get'\n"+
					"- For pipeline status: use gitlab_pipeline action 'get'\n"+
					"- For release info: use gitlab_release action 'list'",
				toolName,
			)},
		},
		IsError: true,
	}
}
