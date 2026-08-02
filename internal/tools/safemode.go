package tools

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// SafeModePreview is the structured response returned when a mutating
// operation is intercepted by Safe Mode. It aliases
// [toolutil.SafeModePreview] so individual tool wrapping and the per-action
// interception used by dispatcher surfaces return the identical payload.
type SafeModePreview = toolutil.SafeModePreview

// WrapMutatingToolsForSafeMode lists all registered tools via an
// ephemeral in-memory session and replaces mutating tool handlers
// (ReadOnlyHint == false) with a handler that returns a
// [SafeModePreview] instead of executing. Returns the number of tools
// wrapped. If listing tools fails, logs the error and returns 0.
func WrapMutatingToolsForSafeMode(server *mcp.Server) int {
	ctx := context.Background()
	tools, err := toolutil.ListRegisteredTools(ctx, server, "safemode-filter")
	if err != nil {
		slog.Error("WrapMutatingToolsForSafeMode: list registered tools failed", "error", err)
		return 0
	}

	var wrapped int
	for _, t := range tools {
		if t.Annotations != nil && t.Annotations.ReadOnlyHint {
			continue
		}
		toolCopy := *t
		server.AddTool(&toolCopy, safeModeHandler(toolCopy.Name))
		wrapped++
	}
	return wrapped
}

// safeModeHandler returns an [mcp.ToolHandler] that builds a
// [SafeModePreview] from the request and returns it as JSON text content
// without executing the real operation. Returns an IsError result when
// the preview cannot be marshaled to JSON.
func safeModeHandler(toolName string) mcp.ToolHandler {
	return func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		preview := SafeModePreview{
			Status: "blocked",
			Mode:   "safe",
			Tool:   toolName,
			Params: req.Params.Arguments,
			Hint:   toolutil.SafeModeHint,
		}

		data, err := json.Marshal(preview)
		if err != nil {
			return &mcp.CallToolResult{ //nolint:nilerr // MCP convention: surface errors in result content, not as Go errors
				Content: []mcp.Content{&mcp.TextContent{Text: "safe mode: failed to marshal preview"}},
				IsError: true,
			}, nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil
	}
}
