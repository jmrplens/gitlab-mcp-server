package toolutil

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// SurfaceToolRegisterOptions controls how an ActionSpec is exposed as a
// standalone visible MCP tool.
type SurfaceToolRegisterOptions struct {
	Description  string
	Icons        []mcp.Icon
	FormatResult FormatResultFunc
}

// TextOnlySurfaceResult marks a successful control-flow result that should not
// be mirrored into StructuredContent.
type TextOnlySurfaceResult interface {
	// SurfaceToolTextOnly marks this result as text-only for standalone surface tools.
	SurfaceToolTextOnly()
}

// RegisterSurfaceToolFromSpec registers one visible MCP tool by projecting an
// ActionSpec and executing its route handler directly.
func RegisterSurfaceToolFromSpec(server *mcp.Server, spec ActionSpec, opts SurfaceToolRegisterOptions) {
	if server == nil {
		return
	}
	tool, err := IndividualToolFromActionSpec(spec, IndividualToolProjectionOptions{
		Description: opts.Description,
		Icons:       opts.Icons,
	})
	if err != nil {
		panic(err)
	}
	formatResult := opts.FormatResult
	if formatResult == nil {
		formatResult = MarkdownForResult
	}
	mcp.AddTool[map[string]any, any](server, tool, surfaceToolHandler(tool.Name, spec.Route, formatResult))
}

func surfaceToolHandler(toolName string, route ActionRoute, formatResult FormatResultFunc) mcp.ToolHandlerFor[map[string]any, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, input map[string]any) (*mcp.CallToolResult, any, error) {
		start := time.Now()
		result, err := route.Handler(ContextWithRequest(ctx, req), input)
		LogToolCallAll(ctx, req, toolName, start, err)
		if err != nil {
			return nil, nil, err
		}
		callResult := formatResult(result)
		if callResult != nil && callResult.IsError {
			return callResult, nil, nil
		}
		if _, ok := result.(TextOnlySurfaceResult); ok {
			return callResult, nil, nil
		}
		return WithHints(callResult, result, nil)
	}
}
