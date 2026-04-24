// output.go documents and enforces the standard MCP tool output pattern.
//
// All tool handlers in this project follow the triple-return pattern:
//
//	func handler(ctx, req, input T) (*mcp.CallToolResult, OutputType, error)
//
// On success:
//
//	return ToolResultWithMarkdown(FormatXMarkdown(out)), out, nil
//
// On error:
//
//	return nil, ZeroValue, err
//
// The *mcp.CallToolResult provides human-readable Markdown via TextContent,
// while the OutputType struct provides structured JSON data for programmatic
// consumption by the SDK serializer.
//
// Meta-tools use MakeMetaHandler with a FormatResultFunc that dispatches
// to the appropriate sub-package Markdown formatter via the type-switch
// in markdownForResult (internal/tools/markdown.go).

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
