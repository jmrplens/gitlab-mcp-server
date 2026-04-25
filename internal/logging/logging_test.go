// logging_test.go contains unit and integration tests for the logging package.
// Unit tests verify nil-safety and [buildLogData] behavior.
// Integration tests use an in-memory MCP client/server pair to verify that
// [SessionLogger] methods emit log entries at the correct level.

package logging

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	fmtSetLogging    = "SetLoggingLevel failed: %v"
	errExpLogEntries = "expected log entries, got none"
)

// ---------------------------------------------------------------------------
// Unit tests — nil-safety and buildLogData
// ---------------------------------------------------------------------------.

// TestNewSessionLogger_NilSession verifies that [NewSessionLogger] returns nil
// when given a nil server session, preventing nil pointer dereferences.
func TestNewSessionLogger_NilSession(t *testing.T) {
	got := NewSessionLogger(nil)
	if got != nil {
		t.Fatal("NewSessionLogger(nil) should return nil")
	}
}

// TestFromToolRequest_NilRequest verifies that [FromToolRequest] returns nil
// when given a nil request.
func TestFromToolRequest_NilRequest(t *testing.T) {
	got := FromToolRequest(nil)
	if got != nil {
		t.Fatal("FromToolRequest(nil) should return nil")
	}
}

// TestFromToolRequest_NilSession verifies that [FromToolRequest] returns nil
// when the request has no associated server session.
func TestFromToolRequest_NilSession(t *testing.T) {
	req := &mcp.CallToolRequest{}
	got := FromToolRequest(req)
	if got != nil {
		t.Fatal("FromToolRequest with nil Session should return nil")
	}
}

// TestFromToolRequest_UninitializedSession verifies that [FromToolRequest]
// returns nil when the session exists but InitializeParams returns nil,
// indicating the MCP handshake has not completed yet.
func TestFromToolRequest_UninitializedSession(t *testing.T) {
	// ServerSession zero-value has nil InitializeParams
	req := &mcp.CallToolRequest{Session: &mcp.ServerSession{}}
	got := FromToolRequest(req)
	if got != nil {
		t.Fatal("FromToolRequest with uninitialized session should return nil")
	}
}

// TestNilLogger_MethodsDoNotPanic verifies that calling any method on a nil
// [SessionLogger] does not panic, ensuring safe usage without nil checks.
func TestNilLogger_MethodsDoNotPanic(t *testing.T) {
	var l *SessionLogger
	ctx := context.Background()

	l.Debug(ctx, "msg", nil)
	l.Info(ctx, "msg", nil)
	l.Warning(ctx, "msg", nil)
	l.Error(ctx, "msg", nil)
	l.LogToolCall(ctx, "tool", time.Now(), nil)
	l.LogToolCall(ctx, "tool", time.Now(), errors.New("err"))
}

// TestBuildLogData_NilData verifies that [buildLogData] returns the message
// as a plain string when no structured data is provided.
func TestBuildLogData_NilData(t *testing.T) {
	got := buildLogData("hello", nil)
	s, ok := got.(string)
	if !ok || s != "hello" {
		t.Fatalf("expected string 'hello', got %v (%T)", got, got)
	}
}

// TestBuildLogData_MapData verifies that [buildLogData] merges the message
// into existing map data under the "message" key.
func TestBuildLogData_MapData(t *testing.T) {
	m := map[string]any{"key": "value"}
	got := buildLogData("msg", m)
	result, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", got)
	}
	if result["message"] != "msg" {
		t.Errorf("expected message='msg', got %v", result["message"])
	}
	if result["key"] != "value" {
		t.Errorf("expected key='value', got %v", result["key"])
	}
}

// TestBuildLogData_OtherData verifies that [buildLogData] wraps non-map data
// in a map with "message" and "data" keys.
func TestBuildLogData_OtherData(t *testing.T) {
	got := buildLogData("msg", 42)
	result, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", got)
	}
	if result["message"] != "msg" {
		t.Errorf("expected message='msg', got %v", result["message"])
	}
	if result["data"] != 42 {
		t.Errorf("expected data=42, got %v", result["data"])
	}
}

// TestBuildLogData_DoesNotMutateCallerMap verifies that buildLogData does not
// mutate the caller's map when merging the message. The original map must not
// gain a "message" key after the call returns.
func TestBuildLogData_DoesNotMutateCallerMap(t *testing.T) {
	original := map[string]any{"key": "value"}
	_ = buildLogData("msg", original)
	if _, exists := original["message"]; exists {
		t.Errorf("buildLogData mutated caller map: original now contains 'message' key")
	}
	if len(original) != 1 {
		t.Errorf("expected original map size 1, got %d", len(original))
	}
}

// ---------------------------------------------------------------------------
// Integration tests — in-memory MCP client/server
// ---------------------------------------------------------------------------.

// logEntry captures a single MCP log message for test assertions.
type logEntry struct {
	Level  mcp.LoggingLevel
	Logger string
	Data   any
}

// newTestSession creates an in-memory MCP server/client pair with a single
// tool that invokes the provided handler. Returns the client session and a
// channel that receives log entries emitted by the server.
func newTestSession(t *testing.T, handler func(ctx context.Context, session *mcp.ServerSession)) (*mcp.ClientSession, <-chan logEntry) {
	t.Helper()

	server := mcp.NewServer(&mcp.Implementation{
		Name: "log-test", Version: "0.0.1",
	}, &mcp.ServerOptions{
		Capabilities: &mcp.ServerCapabilities{
			Logging: &mcp.LoggingCapabilities{},
		},
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "log_test_tool",
		Description: "Test tool for logging",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		handler(ctx, req.Session)
		return &mcp.CallToolResult{}, nil, nil
	})

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}

	var mu sync.Mutex
	logs := make(chan logEntry, 100)

	mcpClient := mcp.NewClient(&mcp.Implementation{
		Name: "log-test-client", Version: "0.0.1",
	}, &mcp.ClientOptions{
		LoggingMessageHandler: func(_ context.Context, req *mcp.LoggingMessageRequest) {
			mu.Lock()
			defer mu.Unlock()
			logs <- logEntry{
				Level:  req.Params.Level,
				Logger: req.Params.Logger,
				Data:   req.Params.Data,
			}
		},
	})

	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	return session, logs
}

// callTestTool invokes the log_test_tool on the given client session with
// a 5-second timeout. Fails the test if the call returns an error.
func callTestTool(t *testing.T, session *mcp.ClientSession) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "log_test_tool"})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
}

// drainLogs collects log entries from the channel until the timeout elapses.
func drainLogs(ch <-chan logEntry, timeout time.Duration) []logEntry {
	var entries []logEntry
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case e := <-ch:
			entries = append(entries, e)
		case <-timer.C:
			return entries
		}
	}
}

// findLogEntryByLevel searches for a log entry at the given level and verifies the logger name.
func findLogEntryByLevel(t *testing.T, entries []logEntry, level mcp.LoggingLevel) bool {
	t.Helper()
	for _, e := range entries {
		if e.Level == level {
			if e.Logger != loggerName {
				t.Errorf("logger = %q, want %q", e.Logger, loggerName)
			}
			return true
		}
	}
	return false
}

// findToolLogEntry searches log entries for a matching tool entry at the expected level.
func findToolLogEntry(entries []logEntry, level mcp.LoggingLevel, toolName string) (map[string]any, bool) {
	for _, e := range entries {
		if e.Level != level {
			continue
		}
		m, ok := e.Data.(map[string]any)
		if !ok {
			continue
		}
		if m["tool"] == toolName {
			return m, true
		}
	}
	return nil, false
}

// TestSessionLogger_LevelsViaIntegration uses table-driven subtests to verify
// that Debug, Info, Warning, and Error each emit a log entry at the correct
// level via a real in-memory MCP client/server connection.
func TestSessionLogger_LevelsViaIntegration(t *testing.T) {
	tests := []struct {
		name  string
		level mcp.LoggingLevel
		fn    func(l *SessionLogger, ctx context.Context)
	}{
		{"debug", "debug", func(l *SessionLogger, ctx context.Context) { l.Debug(ctx, "debug msg", nil) }},
		{"info", "info", func(l *SessionLogger, ctx context.Context) { l.Info(ctx, "info msg", nil) }},
		{"notice", "notice", func(l *SessionLogger, ctx context.Context) { l.Notice(ctx, "notice msg", nil) }},
		{"warning", "warning", func(l *SessionLogger, ctx context.Context) { l.Warning(ctx, "warn msg", nil) }},
		{"error", "error", func(l *SessionLogger, ctx context.Context) { l.Error(ctx, "error msg", nil) }},
		{"critical", "critical", func(l *SessionLogger, ctx context.Context) { l.Critical(ctx, "crit msg", nil) }},
		{"alert", "alert", func(l *SessionLogger, ctx context.Context) { l.Alert(ctx, "alert msg", nil) }},
		{"emergency", "emergency", func(l *SessionLogger, ctx context.Context) { l.Emergency(ctx, "emerg msg", nil) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session, logs := newTestSession(t, func(ctx context.Context, ss *mcp.ServerSession) {
				logger := NewSessionLogger(ss)
				tt.fn(logger, ctx)
			})

			err := session.SetLoggingLevel(context.Background(), &mcp.SetLoggingLevelParams{Level: "debug"})
			if err != nil {
				t.Fatalf(fmtSetLogging, err)
			}

			callTestTool(t, session)
			entries := drainLogs(logs, 2*time.Second)

			if len(entries) == 0 {
				t.Fatal("expected at least one log entry, got none")
			}

			if !findLogEntryByLevel(t, entries, tt.level) {
				t.Errorf("no log entry with level %q found in %d entries", tt.level, len(entries))
			}
		})
	}
}

// TestSessionLogger_LogToolCallSuccess verifies that [SessionLogger.LogToolCall]
// emits an info-level log with status="ok" when no error is provided.
func TestSessionLogger_LogToolCallSuccess(t *testing.T) {
	session, logs := newTestSession(t, func(ctx context.Context, ss *mcp.ServerSession) {
		logger := NewSessionLogger(ss)
		logger.LogToolCall(ctx, "my_tool", time.Now().Add(-100*time.Millisecond), nil)
	})

	err := session.SetLoggingLevel(context.Background(), &mcp.SetLoggingLevelParams{Level: "debug"})
	if err != nil {
		t.Fatalf(fmtSetLogging, err)
	}

	callTestTool(t, session)
	entries := drainLogs(logs, 2*time.Second)

	if len(entries) == 0 {
		t.Fatal(errExpLogEntries)
	}

	m, found := findToolLogEntry(entries, "info", "my_tool")
	if !found {
		t.Fatal("expected info log with tool='my_tool' and status='ok'")
	}
	if m["status"] != "ok" {
		t.Errorf("expected status='ok', got %v", m["status"])
	}
}

// TestSessionLogger_LogToolCallError verifies that [SessionLogger.LogToolCall]
// emits an error-level log with status="error" and the error message when
// a non-nil error is provided.
func TestSessionLogger_LogToolCallError(t *testing.T) {
	session, logs := newTestSession(t, func(ctx context.Context, ss *mcp.ServerSession) {
		logger := NewSessionLogger(ss)
		logger.LogToolCall(ctx, "failing_tool", time.Now(), errors.New("something broke"))
	})

	err := session.SetLoggingLevel(context.Background(), &mcp.SetLoggingLevelParams{Level: "debug"})
	if err != nil {
		t.Fatalf(fmtSetLogging, err)
	}

	callTestTool(t, session)
	entries := drainLogs(logs, 2*time.Second)

	if len(entries) == 0 {
		t.Fatal(errExpLogEntries)
	}

	m, found := findToolLogEntry(entries, "error", "failing_tool")
	if !found {
		t.Fatal("expected error log with tool='failing_tool' and status='error'")
	}
	if m["status"] != "error" {
		t.Errorf("expected status='error', got %v", m["status"])
	}
	if _, hasErr := m["error"]; !hasErr {
		t.Error("expected 'error' key in log data")
	}
}

// TestSessionLogger_WithMapData verifies that structured map data passed to
// [SessionLogger.Info] is preserved in the emitted log entry alongside the
// message.
func TestSessionLogger_WithMapData(t *testing.T) {
	session, logs := newTestSession(t, func(ctx context.Context, ss *mcp.ServerSession) {
		logger := NewSessionLogger(ss)
		logger.Info(ctx, "structured", map[string]any{"project": "test-project", "count": float64(42)})
	})

	err := session.SetLoggingLevel(context.Background(), &mcp.SetLoggingLevelParams{Level: "debug"})
	if err != nil {
		t.Fatalf(fmtSetLogging, err)
	}

	callTestTool(t, session)
	entries := drainLogs(logs, 2*time.Second)

	if len(entries) == 0 {
		t.Fatal(errExpLogEntries)
	}

	found := false
	for _, e := range entries {
		if m, ok := e.Data.(map[string]any); ok {
			if m["project"] == "test-project" && m["message"] == "structured" {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected log entry with project='test-project' and message='structured'")
	}
}

// TestSessionLogger_WithoutSetLevel verifies that logging calls do not panic
// or error when the client has not explicitly set a logging level.
func TestSessionLogger_WithoutSetLevel(t *testing.T) {
	session, _ := newTestSession(t, func(ctx context.Context, ss *mcp.ServerSession) {
		logger := NewSessionLogger(ss)
		logger.Info(ctx, "this should be silently skipped", nil)
	})

	callTestTool(t, session)
}

// TestFromToolRequest_ValidSession verifies that [FromToolRequest] returns a
// non-nil [SessionLogger] when the request carries a valid server session.
func TestFromToolRequest_ValidSession(t *testing.T) {
	// Exercise the happy path of FromToolRequest via a real MCP tool call.
	var gotLogger *SessionLogger
	session, _ := newTestSession(t, func(ctx context.Context, ss *mcp.ServerSession) {
		req := &mcp.CallToolRequest{}
		req.Session = ss
		gotLogger = FromToolRequest(req)
	})

	callTestTool(t, session)

	if gotLogger == nil {
		t.Fatal("FromToolRequest with valid session should return non-nil logger")
	}
}

// TestSessionLogger_LogErrorPath verifies that the log method gracefully handles
// a session.Log error (e.g. when the client has disconnected) by logging to
// stderr instead of panicking.
func TestSessionLogger_LogErrorPath(t *testing.T) {
	// We capture the server session inside a tool handler, then close the
	// client session to break the transport, then call Debug on the captured
	// logger — session.Log should fail, exercising the error branch.
	var capturedSession *mcp.ServerSession
	done := make(chan struct{})

	server := mcp.NewServer(&mcp.Implementation{
		Name: "log-err-test", Version: "0.0.1",
	}, &mcp.ServerOptions{
		Capabilities: &mcp.ServerCapabilities{
			Logging: &mcp.LoggingCapabilities{},
		},
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "capture_session",
		Description: "Captures server session for later use",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		capturedSession = req.Session
		close(done)
		return &mcp.CallToolResult{}, nil, nil
	})

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{
		Name: "log-err-client", Version: "0.0.1",
	}, nil)

	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}

	// Call tool to capture the server session.
	_, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "capture_session"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	<-done

	// Set log level so session.Log doesn't short-circuit.
	if setErr := session.SetLoggingLevel(ctx, &mcp.SetLoggingLevelParams{Level: "debug"}); setErr != nil {
		t.Fatalf("SetLoggingLevel: %v", setErr)
	}

	// Close the client session to break the transport.
	session.Close()

	// Small delay to ensure transport is fully torn down.
	time.Sleep(100 * time.Millisecond)

	// Now log via the captured session — session.Log should fail since
	// the transport is closed and the log level is set (not empty).
	logger := NewSessionLogger(capturedSession)

	// This should not panic; the error is logged to stderr.
	logger.Debug(ctx, "message after disconnect", nil)
}
