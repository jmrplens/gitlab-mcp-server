package toolutil

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"
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

// TestRegisterSurfaceToolFromSpec_DestructiveConfirmAcceptedRunsRoute
// verifies that an accepted elicitation confirmation lets the destructive
// route execute. On protocol 2026-07-28 sessions this exercises the full
// multi round-trip loop: the first call returns an input-required result and
// the SDK client middleware retries with the user's answer attached.
func TestRegisterSurfaceToolFromSpec_DestructiveConfirmAcceptedRunsRoute(t *testing.T) {
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

	var elicitations atomic.Int32
	session := newSurfaceToolSession(t, server, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		elicitations.Add(1)
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirmed": true}}, nil
	})
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "gitlab_test_delete", Arguments: map[string]any{"id": 1}})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if !called.Load() {
		t.Fatal("destructive route was not called after accepted confirmation")
	}
	if elicitations.Load() != 1 {
		t.Errorf("elicitations = %d, want 1", elicitations.Load())
	}
	if result == nil {
		t.Fatal("expected non-nil success result")
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

	// The ordinary path: a formatted result that is not an error and is not
	// text-only must travel out with the handler's own output as the structured
	// content, which is what a client reading structuredContent gets and what
	// the hints are attached to. An error result and a text-only one both
	// return nil structured content, so neither says that this one does not.
	t.Run("structured result carries the output", func(t *testing.T) {
		handler := surfaceToolHandler("gitlab_test_structured", ActionRoute{
			Handler: func(context.Context, map[string]any) (any, error) { return testOutput{Result: "kept"}, nil },
		}, func(any) *mcp.CallToolResult { return SuccessResult("formatted") })

		result, structured, err := handler(context.Background(), nil, map[string]any{})
		if err != nil || result == nil {
			t.Fatalf("handler() = result:%+v err:%v, want a formatted success", result, err)
		}
		out, ok := structured.(testOutput)
		if !ok || out.Result != "kept" {
			t.Fatalf("handler() structured = %+v, want the route's own output", structured)
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

// surfaceToolPointerOutput is a test-only output type whose formatter is
// registered by value while the route returns a pointer to it, the shape of
// a handler that returns *ListOutput.
type surfaceToolPointerOutput struct {
	HintableOutput
	Name string `json:"name"`
}

// TestSurfaceToolHandler_PointerOutput_IsFormattedAnnotatedAndHinted verifies
// the standalone dispatcher's tail on the case the registry used to miss: a
// route returning a pointer to a type whose formatter is registered by value
// is rendered through that formatter, the text block carries the route's
// content kind, and the hints the card ends with reach the structured
// output's next_steps.
func TestSurfaceToolHandler_PointerOutput_IsFormattedAnnotatedAndHinted(t *testing.T) {
	snapshotMarkdownRegistries(t)
	snapshotRegistrationProblems(t)
	RegisterMarkdown(func(o surfaceToolPointerOutput) string {
		var b strings.Builder
		c := NewCard(&b, "Thing "+o.Name)
		c.Field("Name", o.Name)
		c.End("Use action 'thing.get' to read it again")
		return b.String()
	})
	handler := surfaceToolHandler("gitlab_test_pointer", ActionRoute{
		Handler:     func(context.Context, map[string]any) (any, error) { return &surfaceToolPointerOutput{Name: "p"}, nil },
		ContentKind: ActionSpecContentDetail,
	}, MarkdownForResult)

	result, structured, err := handler(context.Background(), nil, map[string]any{})
	if err != nil || result == nil || result.IsError {
		t.Fatalf("handler() = result:%+v err:%v, want a formatted success", result, err)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok || text.Annotations != ContentDetail {
		t.Errorf("first block = %+v, want the card annotated with the detail preset", result.Content[0])
	}
	if want := "## Thing p\n\n- **Name**: p\n\n---\n\U0001F4A1 **Next steps:**\n- Use action 'thing.get' to read it again\n"; text != nil && text.Text != want {
		t.Errorf("text:\n got %q\nwant %q", text.Text, want)
	}
	out, ok := structured.(*surfaceToolPointerOutput)
	if !ok || len(out.NextSteps) != 1 || out.NextSteps[0] != "Use action 'thing.get' to read it again" {
		t.Errorf("structured = %+v, want the pointer with its next_steps set", structured)
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

// TestRegisterSurfaceToolFromSpec_AProtocolFaultStopsTheRouteAsAnError covers
// the guard failing rather than the user declining.
//
// A requestState this server did not issue is a protocol fault, not a tool
// outcome: it travels out as a JSON-RPC error so the client fixes what it sent,
// where an error result would invite the model to try the destructive call
// again. Either way the route must not run — a confirmation exchange that
// cannot be completed fails closed.
func TestRegisterSurfaceToolFromSpec_AProtocolFaultStopsTheRouteAsAnError(t *testing.T) {
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
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirmed": true}}, nil
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:         "gitlab_test_delete",
		Arguments:    map[string]any{"id": 1},
		RequestState: "not-a-state-this-server-issued",
	})

	// The SDK surfaces a handler's coded error as a transport error or as an
	// error result depending on the negotiated revision; what must not happen
	// is a successful call.
	if err == nil && (result == nil || !result.IsError) {
		t.Fatalf("a forged requestState was accepted (result = %+v); the gate can be bypassed by sending one", result)
	}
	if called.Load() {
		t.Error("the destructive route ran despite a confirmation that could not be completed")
	}
}

// TestSurfaceToolHandler_UpstreamResponseBody_IsNotReflected verifies that the
// standalone-tool dispatcher contains a GitLab error before it becomes tool
// output and before it is logged.
//
// The route wraps client-go's error the way gitlab_discover_project does, with
// fmt.Errorf and %w, so it never passes through the wrapping helpers that bound
// the body at the point of classification. On a deployment behind a proxy that
// body is an nginx or WAF page naming internal hosts, and it needs no adversary
// to leak: the model reads the tool error, and the operator reads the same text
// in the "tool call failed" line.
func TestSurfaceToolHandler_UpstreamResponseBody_IsNotReflected(t *testing.T) {
	const internalHost = "gitlab-internal-07.corp.invalid"
	target, parseErr := url.Parse("https://gitlab.example.com/api/v4/projects/group%2Fproject")
	if parseErr != nil {
		t.Fatalf("url.Parse() error = %v", parseErr)
	}
	// "failed to parse unknown error format: %s" is verbatim what client-go
	// puts in Message when the body is not JSON, which a proxy's error page
	// never is. Reproducing that prefix is what makes this the real shape
	// rather than an invented one.
	upstream := &gl.ErrorResponse{
		Response: &http.Response{
			StatusCode: http.StatusBadGateway,
			Request:    &http.Request{Method: http.MethodGet, URL: target},
		},
		Message: "failed to parse unknown error format: <html><head><title>502 Bad Gateway</title></head>" +
			"<body>nginx/1.25.3 upstream " + internalHost + ":8080</body></html>",
	}
	handler := surfaceToolHandler("gitlab_discover_project", ActionRoute{
		Handler: func(context.Context, map[string]any) (any, error) {
			return nil, fmt.Errorf("project %q not found on GitLab: %w", "group/project", upstream)
		},
	}, MarkdownForResult)

	_, _, err := handler(context.Background(), nil, map[string]any{})
	if err == nil {
		t.Fatal("handler() error = nil, want the route's failure")
	}
	if strings.Contains(err.Error(), internalHost) {
		t.Errorf("handler() error = %q, want no upstream response body", err.Error())
	}
	// The wrapper's own context is the useful half and must survive: only the
	// dangerous substring is replaced, never the whole message.
	if !strings.Contains(err.Error(), "not found on GitLab") {
		t.Errorf("handler() error = %q, want the handler's own context kept", err.Error())
	}
	if !errors.Is(err, upstream) {
		t.Error("errors.Is(handler() error, upstream) = false, want the cause still reachable")
	}
}
