package toolutil

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// SuccessResult builds a success [mcp.CallToolResult] carrying the Markdown
// as its one text block, annotated for the assistant. Empty Markdown yields
// nil, which the dispatchers render as JSON through [FinishToolResult]; it is
// how a nil action result and a result with no formatter are answered.
func SuccessResult(markdown string) *mcp.CallToolResult {
	return ToolResultWithMarkdown(markdown)
}

// ErrorResultAnnotated builds the one error envelope every refusal this
// server writes travels in: IsError set, the Markdown as the one text block
// through [NormalizeResultMarkdown], and the annotation given, or
// [ContentMutate] for nil, since a refusal is something the model has to
// act on. The five hand-built literals it replaced carried no annotation
// and skipped the normalization every success got.
func ErrorResultAnnotated(md string, ann *mcp.Annotations) *mcp.CallToolResult {
	if ann == nil {
		ann = ContentMutate
	}
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			&mcp.TextContent{Text: NormalizeResultMarkdown(md), Annotations: ann},
		},
	}
}

// ErrorResult builds a standard error [mcp.CallToolResult] with IsError set,
// through [ErrorResultAnnotated] with the refusal annotation. The message is
// the server's own sentence, written as prose.
func ErrorResult(message string) *mcp.CallToolResult {
	return ErrorResultAnnotated(message, ContentMutate)
}
