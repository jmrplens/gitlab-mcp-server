// logging_test.go contains unit tests for the LogToolCall and LogToolCallAll
// helpers. Tests verify that logging does not panic for success, error, nil
// request, and non-nil request scenarios.
package toolutil

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestLogToolCall_Success verifies that LogToolCall does not panic for a
// successful tool invocation (nil error).
func TestLogToolCall_Success(t *testing.T) {
	LogToolCall("test_tool", time.Now(), nil)
}

// TestLogToolCall_Error verifies that LogToolCall does not panic when
// logging a failed tool invocation.
func TestLogToolCall_Error(t *testing.T) {
	LogToolCall("test_tool", time.Now(), errors.New("something failed"))
}

// TestLogToolCallAll_NilRequest verifies that LogToolCallAll handles a nil
// CallToolRequest without panicking, for both success and error paths.
func TestLogToolCallAll_NilRequest(t *testing.T) {
	ctx := context.Background()
	LogToolCallAll(ctx, nil, "test_tool", time.Now(), nil)
	LogToolCallAll(ctx, nil, "test_tool", time.Now(), errors.New("err"))
}

// TestLogToolCallAll_WithRequest verifies that LogToolCallAll handles
// a non-nil request without a session, silently skipping MCP logging.
func TestLogToolCallAll_WithRequest(t *testing.T) {
	// CallToolRequest without session — MCP logging is silently skipped
	ctx := context.Background()
	req := &mcp.CallToolRequest{}
	LogToolCallAll(ctx, req, "test_tool", time.Now(), nil)
	LogToolCallAll(ctx, req, "test_tool", time.Now(), errors.New("err"))
}
