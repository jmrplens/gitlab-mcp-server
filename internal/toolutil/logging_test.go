// logging_test.go contains unit tests for the LogToolCall, LogToolCallAll,
// and logToolCallWithUser helpers. Tests capture slog output and assert that
// the correct log level, tool name, and structured fields are present.
package toolutil

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// captureSlog redirects slog to a buffer for the duration of the test.
func captureSlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	original := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(original) })
	return &buf
}

// assertContains fails if the output does not contain the expected substring.
func assertContains(t *testing.T, output, want string) {
	t.Helper()
	if !strings.Contains(output, want) {
		t.Errorf("output missing %q, got:\n%s", want, output)
	}
}

// assertNotContains fails if the output unexpectedly contains the substring.
func assertNotContains(t *testing.T, output, unwanted string) {
	t.Helper()
	if strings.Contains(output, unwanted) {
		t.Errorf("output should not contain %q, got:\n%s", unwanted, output)
	}
}

// TestLogToolCall_Success verifies that LogToolCall logs an INFO message
// with the tool name and duration for a successful call (nil error).
func TestLogToolCall_Success(t *testing.T) {
	buf := captureSlog(t)
	LogToolCall("test_tool", time.Now(), nil)
	out := buf.String()
	assertContains(t, out, `"level":"INFO"`)
	assertContains(t, out, `"msg":"tool call completed"`)
	assertContains(t, out, `"tool":"test_tool"`)
	assertContains(t, out, `"duration"`)
	assertNotContains(t, out, `"error"`)
}

// TestLogToolCall_Error verifies that LogToolCall logs an ERROR message
// with the tool name, duration, and error details for a failed call.
func TestLogToolCall_Error(t *testing.T) {
	buf := captureSlog(t)
	LogToolCall("test_tool", time.Now(), errors.New("something failed"))
	out := buf.String()
	assertContains(t, out, `"level":"ERROR"`)
	assertContains(t, out, `"msg":"tool call failed"`)
	assertContains(t, out, `"tool":"test_tool"`)
	assertContains(t, out, `"error":"something failed"`)
}

// TestLogToolCallAll_NilRequest verifies that LogToolCallAll handles a nil
// CallToolRequest for both success and error paths, logging the correct level.
func TestLogToolCallAll_NilRequest(t *testing.T) {
	buf := captureSlog(t)
	ctx := context.Background()

	LogToolCallAll(ctx, nil, "nil_req_tool", time.Now(), nil)
	LogToolCallAll(ctx, nil, "nil_req_tool", time.Now(), errors.New("err"))

	out := buf.String()
	assertContains(t, out, `"level":"INFO"`)
	assertContains(t, out, `"level":"ERROR"`)
	assertContains(t, out, `"tool":"nil_req_tool"`)
}

// TestLogToolCallAll_WithRequest verifies that LogToolCallAll handles
// a non-nil request without a session, logging the correct fields.
func TestLogToolCallAll_WithRequest(t *testing.T) {
	buf := captureSlog(t)
	ctx := context.Background()
	req := &mcp.CallToolRequest{}

	LogToolCallAll(ctx, req, "req_tool", time.Now(), nil)
	LogToolCallAll(ctx, req, "req_tool", time.Now(), errors.New("err"))

	out := buf.String()
	assertContains(t, out, `"tool":"req_tool"`)
	assertContains(t, out, `"level":"INFO"`)
	assertContains(t, out, `"level":"ERROR"`)
}

// TestLogToolCallAll_WithAuthenticatedUser verifies that LogToolCallAll
// routes to logToolCallWithUser when an authenticated identity is present,
// including user and user_id fields in the log output.
func TestLogToolCallAll_WithAuthenticatedUser(t *testing.T) {
	buf := captureSlog(t)
	identity := UserIdentity{UserID: "123", Username: "testuser"}
	ctx := IdentityToContext(context.Background(), identity)

	LogToolCallAll(ctx, nil, "user_tool", time.Now(), nil)
	LogToolCallAll(ctx, nil, "user_tool", time.Now(), errors.New("test error"))

	out := buf.String()
	assertContains(t, out, `"tool":"user_tool"`)
	assertContains(t, out, `"user":"testuser"`)
	assertContains(t, out, `"user_id":"123"`)
	assertContains(t, out, `"level":"INFO"`)
	assertContains(t, out, `"level":"ERROR"`)
}

// TestLogToolCallWithUser_Success verifies that logToolCallWithUser logs
// an INFO message with user and user_id fields for a successful call.
func TestLogToolCallWithUser_Success(t *testing.T) {
	buf := captureSlog(t)
	user := UserIdentity{UserID: "42", Username: "admin"}
	logToolCallWithUser("user_success_tool", time.Now(), nil, user)

	out := buf.String()
	assertContains(t, out, `"level":"INFO"`)
	assertContains(t, out, `"msg":"tool call completed"`)
	assertContains(t, out, `"tool":"user_success_tool"`)
	assertContains(t, out, `"user":"admin"`)
	assertContains(t, out, `"user_id":"42"`)
	assertNotContains(t, out, `"error"`)
}

// TestLogToolCallWithUser_Error verifies that logToolCallWithUser logs
// an ERROR message with user, user_id, and error fields for a failed call.
func TestLogToolCallWithUser_Error(t *testing.T) {
	buf := captureSlog(t)
	user := UserIdentity{UserID: "42", Username: "admin"}
	logToolCallWithUser("user_error_tool", time.Now(), errors.New("api failure"), user)

	out := buf.String()
	assertContains(t, out, `"level":"ERROR"`)
	assertContains(t, out, `"msg":"tool call failed"`)
	assertContains(t, out, `"tool":"user_error_tool"`)
	assertContains(t, out, `"user":"admin"`)
	assertContains(t, out, `"user_id":"42"`)
	assertContains(t, out, `"error":"api failure"`)
}
