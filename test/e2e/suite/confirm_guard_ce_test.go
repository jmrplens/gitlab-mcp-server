//go:build e2e && !enterprise

// confirm_guard_ce_test.go verifies the destructive-action confirmation guard
// for clients that do not support MCP elicitation: without an interactive
// prompt available, a destructive tool call must fail closed with an error
// instructing the caller to re-send with confirm=true, and the same call with
// confirm=true must execute. The dynamic surface has its own guard covered in
// dynamic_ce_test.go; this file covers the [toolutil.ConfirmDestructiveAction]
// path shared by the individual and meta surfaces via sessions.noElicit, whose
// client declares no elicitation capability.
package suite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// guardCall invokes a tool on the no-elicitation session and returns the raw
// result so callers can assert on IsError and the guard text. Only transport
// errors fail the test.
func guardCall(ctx context.Context, t *testing.T, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	result, err := sess.noElicit.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	requireNoError(t, err, "call "+name)
	return result
}

// guardResultText concatenates the text content blocks of a tool result.
func guardResultText(result *mcp.CallToolResult) string {
	var text strings.Builder
	for _, content := range result.Content {
		if textContent, ok := content.(*mcp.TextContent); ok {
			text.WriteString(textContent.Text)
		}
	}
	return text.String()
}

// TestDestructiveConfirmGuard_NoElicitationClient verifies that a client
// without the elicitation capability cannot run destructive individual tools
// silently: the call is blocked with a confirm=true instruction, nothing is
// deleted, and the explicit confirm=true retry executes the deletion.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: individual (no elicitation).
func TestDestructiveConfirmGuard_NoElicitationClient(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)

	t.Run("BlockedWithoutConfirm", func(t *testing.T) {
		result := guardCall(ctx, t, "gitlab_project_delete", map[string]any{
			"project_id": proj.pidOf(),
		})
		requireTruef(t, result.IsError, "destructive call without confirm must fail closed, got: %s", guardResultText(result))
		text := guardResultText(result)
		requireTruef(t, strings.Contains(text, "confirm=true"),
			"guard error must instruct re-sending with confirm=true: %s", text)
	})

	t.Run("ProjectStillExists", func(t *testing.T) {
		_, err := callToolOn[map[string]any](ctx, sess.individual, "gitlab_project_get", map[string]any{
			"project_id": proj.pidOf(),
		})
		requireNoError(t, err, "project must still exist after the blocked delete")
	})

	t.Run("ConfirmTrueExecutes", func(t *testing.T) {
		result := guardCall(ctx, t, "gitlab_project_delete", map[string]any{
			"project_id": proj.pidOf(),
			"confirm":    true,
		})
		requireTruef(t, !result.IsError, "delete with confirm=true must execute, got: %s", guardResultText(result))
	})
}
