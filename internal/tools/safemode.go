// safemode.go implements the GITLAB_SAFE_MODE feature that intercepts mutating
// tools and returns a structured JSON preview instead of executing them.

package tools

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// SafeModePreview is the structured response returned when a mutating tool
// is called with Safe Mode enabled.
type SafeModePreview struct {
	Status string          `json:"status"`
	Mode   string          `json:"mode"`
	Tool   string          `json:"tool"`
	Params json.RawMessage `json:"params"`
	Hint   string          `json:"hint"`
}

// WrapMutatingToolsForSafeMode lists all registered tools via an ephemeral
// in-memory session and replaces mutating tool handlers (ReadOnlyHint == false)
// with a handler that returns a [SafeModePreview] instead of executing.
// Returns the number of tools wrapped.
func WrapMutatingToolsForSafeMode(server *mcp.Server) int {
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		slog.Error("WrapMutatingToolsForSafeMode: server connect failed", "error", err)
		return 0
	}
	defer serverSession.Close()

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "safemode-filter", Version: "0"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		slog.Error("WrapMutatingToolsForSafeMode: client connect failed", "error", err)
		return 0
	}
	defer session.Close()

	result, err := session.ListTools(ctx, nil)
	if err != nil {
		slog.Error("WrapMutatingToolsForSafeMode: list tools failed", "error", err)
		return 0
	}

	var wrapped int
	for _, t := range result.Tools {
		if t.Annotations != nil && t.Annotations.ReadOnlyHint {
			continue
		}
		toolCopy := *t
		server.AddTool(&toolCopy, safeModeHandler(toolCopy.Name))
		wrapped++
	}
	return wrapped
}

// safeModeHandler returns a [mcp.ToolHandler] that builds a [SafeModePreview]
// from the request and returns it as JSON text content without executing the
// real operation.
func safeModeHandler(toolName string) mcp.ToolHandler {
	return func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		preview := SafeModePreview{
			Status: "blocked",
			Mode:   "safe",
			Tool:   toolName,
			Params: req.Params.Arguments,
			Hint:   "Set GITLAB_SAFE_MODE=false to execute this operation",
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
