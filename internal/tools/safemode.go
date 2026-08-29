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
func WrapMutatingToolsForSafeMode(ctx context.Context, server *mcp.Server) int {
	return WrapMutatingToolsForSafeModeExcept(ctx, server, nil)
}

// WrapMutatingToolsForSafeModeExcept wraps mutating tools as
// [WrapMutatingToolsForSafeMode] does, skipping the named tools.
//
// Dispatcher surfaces exempt their catalog-backed tools because those already
// preview per action; wrapping them would block the reads they also serve. Any
// tool registered outside the catalog — the interactive gitlab_interactive_*
// utilities, for instance — must still be wrapped here, or safe mode would let
// it execute for real.
func WrapMutatingToolsForSafeModeExcept(ctx context.Context, server *mcp.Server, exempt map[string]struct{}) int {
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
		if _, skip := exempt[t.Name]; skip {
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
// without executing the real operation.
//
// The result carries IsError. The tool did not run, so it produced none of the
// output its schema describes, and the specification is unconditional about
// what a declared schema obliges: "If an output schema is provided: Servers
// MUST provide structured results that conform to this schema." The tool copy
// keeps its OutputSchema, so a plain success here would be a schema-carrying
// result with nothing structured in it, across every mutating tool at once,
// 561 of the 1065 registered on the individual surface at Ultimate. Populating
// StructuredContent instead is not open to us: the individual schemas are
// additionalProperties:false with required lists, and a preview satisfies none
// of them.
//
// IsError is also the honest reading. Safe mode intercepts a call rather than
// serving it, which is what the flag means, and the dynamic surface already
// answers its destructive guard the same way.
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
			IsError: true,
		}, nil
	}
}

// RemoveNonReadOnlyTools removes every registered tool that does not advertise
// ReadOnlyHint, implementing read-only mode for surfaces whose tools map one to
// one onto actions. Returns the number of tools removed.
//
// Dispatcher surfaces (meta-tools, the dynamic execute tool) must not rely on
// this: one of their tools covers many actions, so read-only filtering happens
// in the catalog instead — see [actioncatalog.Catalog.FilterReadOnlyActions].
func RemoveNonReadOnlyTools(ctx context.Context, server *mcp.Server) int {
	registered, err := toolutil.ListRegisteredTools(ctx, server, "readonly-filter")
	if err != nil {
		slog.Error("RemoveNonReadOnlyTools: list registered tools failed", "error", err)
		return 0
	}
	var toRemove []string
	for _, tool := range registered {
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			toRemove = append(toRemove, tool.Name)
		}
	}
	if len(toRemove) > 0 {
		server.RemoveTools(toRemove...)
	}
	return len(toRemove)
}
