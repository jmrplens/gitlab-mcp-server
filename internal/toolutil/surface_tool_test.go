package toolutil

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type surfaceToolTestInput struct {
	ID int `json:"id" jsonschema:"ID to delete,required"`
}

type surfaceToolTextOnlyOutput struct{}

func (surfaceToolTextOnlyOutput) SurfaceToolTextOnly() {}

// TestRegisterSurfaceToolFromSpec_NilServer verifies nil servers are ignored.
func TestRegisterSurfaceToolFromSpec_NilServer(t *testing.T) {
	RegisterSurfaceToolFromSpec(nil, NewActionSpec("noop", ActionRoute{}, ActionSpecOptions{}), SurfaceToolRegisterOptions{})
}

// TestRegisterSurfaceToolFromSpec_InvalidSpecPanics verifies that
// RegisterSurfaceToolFromSpec panics when IndividualToolFromActionSpec cannot
// project the spec (e.g. no individual tool name configured).
func TestRegisterSurfaceToolFromSpec_InvalidSpecPanics(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	// Missing IndividualTool.Name — IndividualToolFromActionSpec returns an
	// error, which RegisterSurfaceToolFromSpec must surface as a panic.
	spec := NewActionSpec("noop", ActionRoute{}, ActionSpecOptions{})

	defer func() {
		if recover() == nil {
			t.Error("expected panic for invalid ActionSpec, got none")
		}
	}()
	RegisterSurfaceToolFromSpec(server, spec, SurfaceToolRegisterOptions{Description: "noop"})
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
		return &mcp.ElicitResult{}, nil
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

// TestSurfaceToolHandler_ErrorAndFormattedResults verifies direct handler
// branches for route errors, formatter-produced tool errors, and text-only
// outputs that should not be mirrored into structured content.
func TestSurfaceToolHandler_ErrorAndFormattedResults(t *testing.T) {
	t.Run("route error", func(t *testing.T) {
		routeErr := errors.New("route failed")
		handler := surfaceToolHandler("gitlab_test_error", ActionRoute{
			Handler: func(context.Context, map[string]any) (any, error) { return nil, routeErr },
		}, MarkdownForResult)

		result, structured, err := handler(context.Background(), nil, map[string]any{})
		if !errors.Is(err, routeErr) || result != nil || structured != nil {
			t.Fatalf("handler() = result:%+v structured:%+v err:%v, want route error", result, structured, err)
		}
	})

	t.Run("formatter error result", func(t *testing.T) {
		handler := surfaceToolHandler("gitlab_test_formatter", ActionRoute{
			Handler: func(context.Context, map[string]any) (any, error) { return testOutput{Result: "ignored"}, nil },
		}, func(any) *mcp.CallToolResult { return ErrorResult("formatted error") })

		result, structured, err := handler(context.Background(), nil, map[string]any{})
		if err != nil || result == nil || !result.IsError || structured != nil {
			t.Fatalf("handler() = result:%+v structured:%+v err:%v, want formatter error result", result, structured, err)
		}
	})

	t.Run("text only result", func(t *testing.T) {
		handler := surfaceToolHandler("gitlab_test_text_only", ActionRoute{
			Handler: func(context.Context, map[string]any) (any, error) { return surfaceToolTextOnlyOutput{}, nil },
		}, func(any) *mcp.CallToolResult { return SuccessResult("text only") })

		result, structured, err := handler(context.Background(), nil, map[string]any{})
		if err != nil || result == nil || structured != nil {
			t.Fatalf("handler() = result:%+v structured:%+v err:%v, want text-only result", result, structured, err)
		}
	})
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
