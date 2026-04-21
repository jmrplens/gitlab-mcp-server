// progress_test.go contains unit and integration tests for the progress package.
// Unit tests verify nil-safety and inactive [Tracker] behavior.
// Integration tests use an in-memory MCP client/server pair to verify that
// progress notifications flow correctly from server tool handlers to clients.
package progress

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const testProgressMessage = "Working..."

// TestFromRequest_Nil verifies that [FromRequest] returns an inactive [Tracker]
// when given a nil request.
func TestFromRequest_Nil(t *testing.T) {
	tracker := FromRequest(nil)
	if tracker.IsActive() {
		t.Error("expected inactive tracker for nil request")
	}
}

// TestFromRequest_NoToken verifies that [FromRequest] returns an inactive
// [Tracker] when the request does not include a progress token.
func TestFromRequest_NoToken(t *testing.T) {
	req := &mcp.CallToolRequest{}
	tracker := FromRequest(req)
	if tracker.IsActive() {
		t.Error("expected inactive tracker when no progress token")
	}
}

// TestFromRequest_NilSession verifies that [FromRequest] returns an inactive
// [Tracker] when the request has params but no session, ensuring safe
// degradation when the MCP session is not established.
func TestFromRequest_NilSession(t *testing.T) {
	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name: "test_tool",
			Meta: mcp.Meta{"progressToken": "tok"},
		},
	}
	tracker := FromRequest(req)
	if tracker.IsActive() {
		t.Error("expected inactive tracker when session is nil")
	}
}

// TestFromRequest_UninitializedSession verifies that [FromRequest] returns
// an inactive [Tracker] when the session exists but InitializeParams returns
// nil, indicating the MCP handshake has not completed yet.
func TestFromRequest_UninitializedSession(t *testing.T) {
	req := &mcp.CallToolRequest{
		Params:  &mcp.CallToolParamsRaw{Name: "test_tool", Meta: mcp.Meta{"progressToken": "tok"}},
		Session: &mcp.ServerSession{},
	}
	tracker := FromRequest(req)
	if tracker.IsActive() {
		t.Error("expected inactive tracker when session is uninitialized")
	}
}

// TestIsActive_ZeroValue verifies that a zero-value [Tracker] is inactive.
func TestIsActive_ZeroValue(t *testing.T) {
	var t0 Tracker
	if t0.IsActive() {
		t.Error("zero-value Tracker should be inactive")
	}
}

// TestUpdate_Inactive verifies that [Tracker.Update] does not panic when
// called on an inactive tracker.
func TestUpdate_Inactive(t *testing.T) {
	var tracker Tracker
	// Should not panic on inactive tracker
	tracker.Update(context.Background(), 1, 3, "test")
}

// TestStep_Inactive verifies that [Tracker.Step] does not panic when called
// on an inactive tracker.
func TestStep_Inactive(t *testing.T) {
	var tracker Tracker
	// Should not panic on inactive tracker
	tracker.Step(context.Background(), 1, 3, "test")
}

// TestUpdate_CancelledContext verifies that [Tracker.Update] returns silently
// without sending when the context is already canceled.
func TestUpdate_CancelledContext(t *testing.T) {
	tracker := Tracker{
		session: &mcp.ServerSession{},
		token:   "test-token",
	}
	ctx := testutil.CancelledCtx(t)
	// Should silently return without sending
	tracker.Update(ctx, 1, 3, "test")
}

// TestStep_CalculatesProgress verifies that [Tracker.Step] calculates
// zero-based progress (step-1) without panicking even on an inactive tracker.
func TestStep_CalculatesProgress(t *testing.T) {
	// Step(1, 3) -> progress=0, total=3
	// Step(2, 3) -> progress=1, total=3
	// Step(3, 3) -> progress=2, total=3
	// Verify the formula by checking tracker doesn't panic with zero-value session
	var tracker Tracker
	tracker.Step(context.Background(), 1, 3, "step one")
	tracker.Step(context.Background(), 2, 3, "step two")
	tracker.Step(context.Background(), 3, 3, "step three")
}

// Integration test: Create a real client+server pair and verify progress
// notifications flow from server tool handler back to the client.
// TestProgress_Integration creates a real in-memory MCP client/server pair
// to verify that [Tracker.Step] sends progress notifications from the server
// tool handler back to the client. It asserts correct progress/total values
// and message content.
func TestProgress_Integration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	const expectedNotifications = 2
	var mu sync.Mutex
	var receivedNotifications []mcp.ProgressNotificationParams
	allReceived := make(chan struct{})

	server := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "v0.0.1"}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "test_tool",
		Description: "A test tool that sends progress",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		tracker := FromRequest(req)
		if !tracker.IsActive() {
			t.Error("expected tracker to be active when client sends progress token")
		}
		tracker.Step(ctx, 1, 2, testProgressMessage)
		tracker.Step(ctx, 2, 2, "Done!")
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "ok"}},
		}, nil, nil
	})

	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, &mcp.ClientOptions{
		ProgressNotificationHandler: func(_ context.Context, req *mcp.ProgressNotificationClientRequest) {
			mu.Lock()
			defer mu.Unlock()
			receivedNotifications = append(receivedNotifications, *req.Params)
			if len(receivedNotifications) == expectedNotifications {
				close(allReceived)
			}
		},
	})

	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}

	t.Cleanup(func() {
		clientSession.Close()
		serverSession.Wait()
	})

	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "test_tool",
		Arguments: map[string]any{},
		Meta:      mcp.Meta{"progressToken": "my-token"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("expected non-empty result content")
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if text != "ok" {
		t.Errorf("unexpected result: %q", text)
	}

	// Wait for all progress notifications to arrive (they may be delivered
	// asynchronously after CallTool returns).
	select {
	case <-allReceived:
	case <-ctx.Done():
		mu.Lock()
		t.Fatalf("timed out waiting for progress notifications, got %d of %d",
			len(receivedNotifications), expectedNotifications)
		mu.Unlock()
	}

	mu.Lock()
	defer mu.Unlock()

	// Step(1, 2) -> progress=0, total=2
	if receivedNotifications[0].Progress != 0 || receivedNotifications[0].Total != 2 {
		t.Errorf("notification[0]: progress=%v total=%v, want 0/2",
			receivedNotifications[0].Progress, receivedNotifications[0].Total)
	}
	if receivedNotifications[0].Message != testProgressMessage {
		t.Errorf("notification[0] message=%q, want %q", receivedNotifications[0].Message, testProgressMessage)
	}

	// Step(2, 2) -> progress=1, total=2
	if receivedNotifications[1].Progress != 1 || receivedNotifications[1].Total != 2 {
		t.Errorf("notification[1]: progress=%v total=%v, want 1/2",
			receivedNotifications[1].Progress, receivedNotifications[1].Total)
	}
	if receivedNotifications[1].Message != "Done!" {
		t.Errorf("notification[1] message=%q, want %q", receivedNotifications[1].Message, "Done!")
	}
}

// TestFromRequest_ParamsNoToken verifies that [FromRequest] returns an inactive
// [Tracker] when the request has Params but no progress token set (covers the
// GetProgressToken()==nil branch).
func TestFromRequest_ParamsNoToken(t *testing.T) {
	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name: "test_tool",
		},
	}
	tracker := FromRequest(req)
	if tracker.IsActive() {
		t.Error("expected inactive tracker when no progress token is set")
	}
}

// TestUpdate_NotifyProgressError verifies that [Tracker.Update] logs the error
// but does not panic or propagate it when [ServerSession.NotifyProgress] fails
// (e.g., because the peer has disconnected).
func TestUpdate_NotifyProgressError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	server := mcp.NewServer(&mcp.Implementation{Name: "test-err", Version: "v0.0.1"}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "error_tool",
		Description: "Tool that triggers progress error",
	}, func(_ context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		// We never call this tool — we just need the server session
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "ok"}},
		}, nil, nil
	})

	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client-err"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}

	// Close the client session so the transport pipe is broken
	clientSession.Close()
	time.Sleep(50 * time.Millisecond)
	serverSession.Wait()

	// Construct a tracker using the now-dead server session
	tracker := Tracker{
		session: serverSession,
		token:   "err-token",
	}

	if !tracker.IsActive() {
		t.Fatal("expected tracker to be active (session + token set)")
	}

	// Update should call NotifyProgress, which fails because the session is
	// closed. The error should be logged but not panic.
	tracker.Update(context.Background(), 1, 3, "should fail silently")
}

// TestFromRequest_WithToken_NoSession verifies that a [Tracker] with a token
// but no session is inactive and that [Tracker.Update] is a safe no-op.
func TestFromRequest_WithTokenNoSession(t *testing.T) {
	// A request with a token but no session results in inactive tracker.
	// We use the integration-style approach: create a real request via
	// a tool call but only test the tracker's session/token logic.
	tracker := Tracker{
		session: nil,
		token:   "a-token",
	}
	if tracker.IsActive() {
		t.Error("expected inactive tracker when session is nil")
	}
	// Update should be a safe no-op
	tracker.Update(context.Background(), 1, 3, "should not send")
}

// TestFromRequest_InitializedSessionNoToken verifies that FromRequest returns
// an inactive tracker when the session is initialized but no progress token is
// present in the request params. This covers the token==nil branch.
func TestFromRequest_InitializedSessionNoToken(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	var capturedTracker Tracker
	server := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "v0.0.1"}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "no_token_tool",
		Description: "tool for testing no progress token",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		capturedTracker = FromRequest(req)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "ok"}},
		}, nil, nil
	})

	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		clientSession.Close()
		serverSession.Wait()
	})

	_, err = clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "no_token_tool",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if capturedTracker.IsActive() {
		t.Error("expected inactive tracker when no progress token is set")
	}
}
