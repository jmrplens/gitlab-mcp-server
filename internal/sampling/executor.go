// executor.go implements the sampling request executor that delegates LLM.

package sampling

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ServerToolExecutor dispatches tool calls to explicitly registered handlers,
// restricting execution to an allow-list of tool names.
//
// SECURITY: Only tools whose handlers are registered can be executed.
// This prevents the LLM from invoking destructive or unrelated tools during
// sampling.
type ServerToolExecutor struct {
	handlers map[string]mcp.ToolHandler
	session  *mcp.ServerSession
}

// NewServerToolExecutor creates a ServerToolExecutor with the given tool handlers.
// The session is attached to each CallToolRequest so handlers can access it.
// Only the tools registered here can be called during sampling.
func NewServerToolExecutor(session *mcp.ServerSession, handlers map[string]mcp.ToolHandler) *ServerToolExecutor {
	return &ServerToolExecutor{
		handlers: handlers,
		session:  session,
	}
}

// ExecuteTool dispatches a tool call to the registered handler.
// Returns an error if the tool name is not registered.
func (e *ServerToolExecutor) ExecuteTool(ctx context.Context, name string, args map[string]any) (*mcp.CallToolResult, error) {
	handler, ok := e.handlers[name]
	if !ok {
		return nil, fmt.Errorf("sampling: tool %q is not in the allowed list", name)
	}

	argsJSON, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("sampling: marshal args for tool %q: %w", name, err)
	}

	req := &mcp.CallToolRequest{
		Session: e.session,
		Params: &mcp.CallToolParamsRaw{
			Name:      name,
			Arguments: argsJSON,
		},
	}

	return handler(ctx, req)
}
