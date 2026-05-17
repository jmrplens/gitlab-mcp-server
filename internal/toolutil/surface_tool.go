package toolutil

import (
	"context"
	"fmt"
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

type surfaceToolTextOnlyMarker interface {
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
		if route.Destructive {
			message := fmt.Sprintf("Confirm destructive action %q?", toolName)
			if result := ConfirmDestructiveAction(ctx, req, input, message); result != nil {
				LogToolCallAll(ctx, req, toolName, start, nil)
				return result, nil, nil
			}
		}
		result, err := route.Handler(ContextWithRequest(ctx, req), input)
		LogToolCallAll(ctx, req, toolName, start, err)
		if err != nil {
			return nil, nil, err
		}
		callResult := formatResult(result)
		if callResult != nil && callResult.IsError {
			return callResult, nil, nil
		}
		if _, ok := result.(surfaceToolTextOnlyMarker); ok {
			return callResult, nil, nil
		}
		return WithHints(callResult, result, nil)
	}
}
