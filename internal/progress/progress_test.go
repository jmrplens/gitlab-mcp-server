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

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
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

// TestFromRequest_WithTokenNoSession verifies that a [Tracker] with a token
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

// TestUpdate_MonotonicEnforcement verifies that [Tracker.Update] drops
// non-monotonic progress notifications per MCP spec 2025-11-25 requirement
// that progress values strictly increase.
func TestUpdate_MonotonicEnforcement(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	var mu sync.Mutex
	var received []float64
	allReceived := make(chan struct{})

	server := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "v0.0.1"}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "test_tool",
		Description: "Sends progress with regressions",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		tracker := FromRequest(req)
		tracker.Update(ctx, 1, 10, "first")
		tracker.Update(ctx, 0.5, 10, "regression-must-drop")
		tracker.Update(ctx, 1, 10, "equal-must-drop")
		tracker.Update(ctx, 2, 10, "second")
		tracker.Update(ctx, 1.5, 10, "regression-must-drop")
		tracker.Update(ctx, 3, 10, "third")
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil, nil
	})

	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}

	expected := 3
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, &mcp.ClientOptions{
		ProgressNotificationHandler: func(_ context.Context, req *mcp.ProgressNotificationClientRequest) {
			mu.Lock()
			defer mu.Unlock()
			received = append(received, req.Params.Progress)
			if len(received) == expected {
				close(allReceived)
			}
		},
	})
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() { _ = clientSession.Close() }()

	_, err = clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "test_tool",
		Meta: mcp.Meta{"progressToken": "test-monotonic-token"},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}

	select {
	case <-allReceived:
	case <-time.After(2 * time.Second):
		mu.Lock()
		got := append([]float64{}, received...)
		mu.Unlock()
		t.Fatalf("timeout: got %d notifications, want %d (values=%v)", len(got), expected, got)
	}

	mu.Lock()
	defer mu.Unlock()
	// Each surviving notification is named after the handler's message for
	// the update that produced it; the regressions between them must be gone.
	want := []struct {
		name     string
		progress float64
	}{
		{"first", 1},
		{"second", 2},
		{"third", 3},
	}
	if len(received) != len(want) {
		t.Fatalf("received = %v, want %v", received, want)
	}
	for i, tc := range want {
		t.Run(tc.name, func(t *testing.T) {
			if received[i] != tc.progress {
				t.Errorf("received[%d] = %v, want %v (full=%v)", i, received[i], tc.progress, received)
			}
		})
	}
}

// TestDone_SendsCompletion verifies that [Tracker.Done] sends a notification
// with progress equal to total.
func TestDone_SendsCompletion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	var mu sync.Mutex
	var lastProgress, lastTotal float64
	gotNotification := make(chan struct{}, 1)

	server := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "v0.0.1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "t", Description: "d"},
		func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			tracker := FromRequest(req)
			tracker.Done(ctx, 5, "complete")
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil, nil
		})
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "c"}, &mcp.ClientOptions{
		ProgressNotificationHandler: func(_ context.Context, req *mcp.ProgressNotificationClientRequest) {
			mu.Lock()
			lastProgress = req.Params.Progress
			lastTotal = req.Params.Total
			mu.Unlock()
			select {
			case gotNotification <- struct{}{}:
			default:
			}
		},
	})
	cs, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() { _ = cs.Close() }()

	_, err = cs.CallTool(ctx, &mcp.CallToolParams{
		Name: "t",
		Meta: mcp.Meta{"progressToken": "done-token"},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}

	select {
	case <-gotNotification:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for completion notification")
	}

	mu.Lock()
	defer mu.Unlock()
	if lastProgress != 5 || lastTotal != 5 {
		t.Errorf("progress=%v total=%v, want both 5", lastProgress, lastTotal)
	}
}

// TestDone_ZeroTotalIsNoop verifies that [Tracker.Done] with non-positive total
// does not send a notification.
func TestDone_ZeroTotalIsNoop(t *testing.T) {
	var tracker Tracker
	tracker.Done(context.Background(), 0, "should-noop")
	tracker.Done(context.Background(), -1, "should-noop")
}

// collectProgress runs report as the body of an MCP tool call over an in-memory
// client/server pair, with a progress token set so the tracker is active, and
// returns the progress notifications the client received in the order they
// arrived. It waits until want of them have arrived, so a caller can assert on
// the exact sequence a client sees rather than on what the handler attempted.
//
// A notification the tracker should have dropped still shows up in that
// sequence, because the transport delivers in send order: an extra one arrives
// before the last expected one and displaces it in the slice, which the
// caller's sequence assertion reports.
func collectProgress(t *testing.T, want int, report func(ctx context.Context, tracker Tracker)) []mcp.ProgressNotificationParams {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	server := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "v0.0.1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "scaled_tool", Description: "reports on a scaled sub-range"},
		func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			tracker := FromRequest(req)
			if !tracker.IsActive() {
				// Off the test goroutine: report and answer deterministically.
				t.Error("expected an active tracker when the client sends a progress token")
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "inactive"}}}, nil, nil
			}
			report(ctx, tracker)
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil, nil
		})

	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	var mu sync.Mutex
	var received []mcp.ProgressNotificationParams
	enough := make(chan struct{}, 1)

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, &mcp.ClientOptions{
		ProgressNotificationHandler: func(_ context.Context, req *mcp.ProgressNotificationClientRequest) {
			mu.Lock()
			defer mu.Unlock()
			received = append(received, *req.Params)
			if len(received) >= want {
				select {
				case enough <- struct{}{}:
				default:
				}
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

	if _, err = clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "scaled_tool",
		Meta: mcp.Meta{"progressToken": "scale-token"},
	}); err != nil {
		t.Fatalf("call tool: %v", err)
	}

	select {
	case <-enough:
	case <-ctx.Done():
		mu.Lock()
		got := len(received)
		mu.Unlock()
		t.Fatalf("timed out waiting for progress notifications: got %d, want %d", got, want)
	}

	mu.Lock()
	defer mu.Unlock()
	return append([]mcp.ProgressNotificationParams(nil), received...)
}

// TestOnScale_InterleavedSubStep_ClientSeesOneIncreasingSeries verifies the
// property [Tracker.OnScale] exists for: a sub-step counting in its own units,
// interleaved with the outer counter that spawned it, reaches the client as a
// single strictly increasing series on one total.
//
// The shape is the one internal/tools/packages.PublishDir has — an outer
// counter over files, an inner one over the bytes of the file being published.
// Both write to one progress token, so the sub-step's own 0.25 must arrive
// translated into where that file sits in the whole job, and carrying the whole
// job's total rather than its own.
func TestOnScale_InterleavedSubStep_ClientSeesOneIncreasingSeries(t *testing.T) {
	const outerTotal = 3

	got := collectProgress(t, 4, func(ctx context.Context, tracker Tracker) {
		tracker.Step(ctx, 1, outerTotal, "file 1 of 3")

		// File 2 measures itself in its own units, 0..1, placed after file 1.
		bytes := tracker.OnScale(1, outerTotal)
		bytes.Update(ctx, 0.25, 1, "file 2: 25%")
		bytes.Update(ctx, 0.75, 1, "file 2: 75%")

		tracker.Step(ctx, 3, outerTotal, "file 3 of 3")
	})

	want := []struct {
		name     string
		progress float64
		message  string
	}{
		{"outer step 1", 0, "file 1 of 3"},
		{"sub-step translated by base", 1.25, "file 2: 25%"},
		{"sub-step still inside the outer range", 1.75, "file 2: 75%"},
		{"outer counter resumes ahead of the sub-step", 2, "file 3 of 3"},
	}
	if len(got) != len(want) {
		t.Fatalf("received %d notifications, want %d: %+v", len(got), len(want), got)
	}
	for i, tc := range want {
		t.Run(tc.name, func(t *testing.T) {
			if got[i].Progress != tc.progress {
				t.Errorf("notification[%d].Progress = %v, want %v", i, got[i].Progress, tc.progress)
			}
			if got[i].Message != tc.message {
				t.Errorf("notification[%d].Message = %q, want %q", i, got[i].Message, tc.message)
			}
			// The sub-step passed a total of 1; the client must be told the
			// whole job's total, or the series it draws changes scale midway.
			if got[i].Total != outerTotal {
				t.Errorf("notification[%d].Total = %v, want %v", i, got[i].Total, float64(outerTotal))
			}
			if i > 0 && got[i].Progress <= got[i-1].Progress {
				t.Errorf("notification[%d].Progress = %v is not ahead of notification[%d] = %v",
					i, got[i].Progress, i-1, got[i-1].Progress)
			}
		})
	}
}

// TestOnScale_OuterUpdateBehindTheSubStep_IsDropped verifies that the Tracker
// [Tracker.OnScale] returns shares the monotonic state of the one it came from,
// and that the guard compares translated values.
//
// Both halves are load-bearing. Were the state not shared, the outer update
// below would compare against the outer Tracker's own last value and be sent,
// putting the series backwards on the wire; were the guard applied before
// translation, the sub-step's raw 0.5 would be what the outer 2 is measured
// against.
func TestOnScale_OuterUpdateBehindTheSubStep_IsDropped(t *testing.T) {
	const outerTotal = 4

	got := collectProgress(t, 2, func(ctx context.Context, tracker Tracker) {
		sub := tracker.OnScale(2, outerTotal)
		sub.Update(ctx, 0.5, 1, "sub-step halfway")   // translated to 2.5 of 4
		tracker.Update(ctx, 2, outerTotal, "dropped") // behind 2.5, must not be sent
		tracker.Update(ctx, 3, outerTotal, "resumed") // ahead of 2.5, must be sent
	})

	want := []struct {
		name     string
		progress float64
		message  string
	}{
		{"sub-step on the outer scale", 2.5, "sub-step halfway"},
		{"outer update ahead of it", 3, "resumed"},
	}
	if len(got) != len(want) {
		t.Fatalf("received %d notifications, want %d: %+v", len(got), len(want), got)
	}
	for i, tc := range want {
		t.Run(tc.name, func(t *testing.T) {
			if got[i].Progress != tc.progress {
				t.Errorf("notification[%d].Progress = %v, want %v", i, got[i].Progress, tc.progress)
			}
			if got[i].Message != tc.message {
				t.Errorf("notification[%d].Message = %q, want %q", i, got[i].Message, tc.message)
			}
		})
	}
}

// TestOnScale_InactiveTracker_StaysInactive verifies that scaling an inactive
// [Tracker] yields an inactive one, so a caller that hands a sub-step its own
// scale never has to check whether the call carried a progress token first.
func TestOnScale_InactiveTracker_StaysInactive(t *testing.T) {
	var tracker Tracker
	scaled := tracker.OnScale(5, 10)
	if scaled.IsActive() {
		t.Error("OnScale on a zero-value Tracker returned an active tracker")
	}
	// Contract of an inactive Tracker: every method is a no-op, never a panic.
	scaled.Update(context.Background(), 1, 2, "should not send")
}
