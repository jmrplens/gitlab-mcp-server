package toolutil

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type surfaceToolTestInput struct {
	ID int `json:"id" jsonschema:"ID to delete,required"`
}

// TestRegisterSurfaceToolFromSpec_DestructiveDeclineStopsRoute verifies catalog-backed individual tools centralize destructive confirmation.
func TestRegisterSurfaceToolFromSpec_DestructiveDeclineStopsRoute(t *testing.T) {
	var called atomic.Bool
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	route := RouteFunc(func(_ context.Context, _ surfaceToolTestInput) (DeleteOutput, error) {
		called.Store(true)
		return DeleteOutput{Status: "success", Message: "deleted"}, nil
	})
	route.Destructive = true
	spec := NewActionSpec("delete", route, ActionSpecOptions{
		IndividualTool: IndividualToolSpec{Name: "gitlab_test_delete", Title: "Test Delete"},
	})
	RegisterSurfaceToolFromSpec(server, spec, SurfaceToolRegisterOptions{Description: "Test destructive tool."})

	session := newSurfaceToolSession(t, server, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "decline"}, nil
	})
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "gitlab_test_delete", Arguments: map[string]any{"id": 1}})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if called.Load() {
		t.Fatal("destructive route was called after declined confirmation")
	}
	if result == nil || strings.TrimSpace(surfaceToolResultText(result)) == "" {
		t.Fatal("expected non-empty cancellation result")
	}
}

// TestRegisterSurfaceToolFromSpec_ExplicitConfirmBypassesPrompt verifies confirm:true proceeds without elicitation.
func TestRegisterSurfaceToolFromSpec_ExplicitConfirmBypassesPrompt(t *testing.T) {
	var called atomic.Bool
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	route := RouteFunc(func(_ context.Context, _ surfaceToolTestInput) (DeleteOutput, error) {
		called.Store(true)
		return DeleteOutput{Status: "success", Message: "deleted"}, nil
	})
	route.Destructive = true
	spec := NewActionSpec("delete", route, ActionSpecOptions{
		IndividualTool: IndividualToolSpec{Name: "gitlab_test_delete", Title: "Test Delete"},
	})
	RegisterSurfaceToolFromSpec(server, spec, SurfaceToolRegisterOptions{Description: "Test destructive tool."})

	session := newSurfaceToolSession(t, server, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		t.Fatal("elicitation should not run when confirm is true")
		return nil, nil
	})
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "gitlab_test_delete", Arguments: map[string]any{"id": 1, "confirm": true}})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if !called.Load() {
		t.Fatal("destructive route was not called after explicit confirmation")
	}
	if result == nil {
		t.Fatal("expected non-nil success result")
	}
}

func newSurfaceToolSession(t *testing.T, server *mcp.Server, elicitation func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error)) *mcp.ClientSession {
	t.Helper()
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, &mcp.ClientOptions{ElicitationHandler: elicitation})
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		session.Close()
		_ = serverSession.Wait()
	})
	return session
}

func surfaceToolResultText(result *mcp.CallToolResult) string {
	var b strings.Builder
	for _, content := range result.Content {
		if textContent, ok := content.(*mcp.TextContent); ok {
			b.WriteString(textContent.Text)
		}
	}
	return b.String()
}
