package toolutil

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// SuccessResult builds a standard success [mcp.CallToolResult] with Markdown
// and the structured output for both human-readable and programmatic consumption.
// If markdown is empty, the result contains only a structured JSON annotation.
func SuccessResult(markdown string) *mcp.CallToolResult {
	return ToolResultWithMarkdown(markdown)
}

// ErrorResult builds a standard error [mcp.CallToolResult] with IsError set.
// It returns the error message as Markdown content for display.
func ErrorResult(message string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			&mcp.TextContent{Text: message},
		},
	}
}
