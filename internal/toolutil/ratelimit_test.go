// ratelimit_test.go verifies the token-bucket rate limiter and the MCP
// receiving middleware that converts over-budget tools/call requests into
// structured tool error results.
package toolutil

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestNewRateLimiter_Disabled verifies that a non-positive rps disables the
// limiter (returns nil, treated as no-op by AttachRateLimit).
func TestNewRateLimiter_Disabled(t *testing.T) {
	t.Parallel()
	for _, rps := range []float64{0, -1, -100} {
		if l := NewRateLimiter(rps, 10); l != nil {
			t.Errorf("NewRateLimiter(%g, 10) = %v, want nil", rps, l)
		}
	}
}

// TestNewRateLimiter_ClampsBurst verifies that a burst < 1 is clamped to 1
// when rps is positive, since rate.Limiter would otherwise be unusable.
func TestNewRateLimiter_ClampsBurst(t *testing.T) {
	t.Parallel()
	l := NewRateLimiter(10, 0)
	if l == nil {
		t.Fatal("NewRateLimiter(10, 0) = nil, want non-nil")
	}
	if !l.allow() {
		t.Error("first allow() = false, want true (burst clamped to 1)")
	}
}

// TestRateLimiter_AllowsBurstThenBlocks verifies that within a single
// second the limiter grants burst tokens then blocks subsequent requests.
func TestRateLimiter_AllowsBurstThenBlocks(t *testing.T) {
	t.Parallel()
	l := NewRateLimiter(1, 3)
	for i := range 3 {
		if !l.allow() {
			t.Fatalf("allow() #%d = false, want true within burst", i+1)
		}
	}
	if l.allow() {
		t.Error("allow() after burst = true, want false")
	}
}

// TestRateLimiter_NilSafe verifies that the nil receiver always allows.
func TestRateLimiter_NilSafe(t *testing.T) {
	t.Parallel()
	var l *RateLimiter
	if !l.allow() {
		t.Error("(*RateLimiter)(nil).allow() = false, want true")
	}
}

// TestAttachRateLimit_NoLimiter verifies that a nil limiter does not register
// any middleware (calling tools/call still succeeds).
func TestAttachRateLimit_NoLimiter(t *testing.T) {
	t.Parallel()
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	registerEchoTool(server)
	AttachRateLimit(server, nil)

	session, ctx := connectClient(t, server)
	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "echo"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Errorf("CallTool with nil limiter returned IsError=true: %+v", res)
	}
}

// TestAttachRateLimit_BlocksAfterBurst verifies that once the bucket is
// drained subsequent tools/call requests return IsError with a "rate limit"
// message and do not invoke the underlying handler.
func TestAttachRateLimit_BlocksAfterBurst(t *testing.T) {
	t.Parallel()

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	calls := 0
	mcp.AddTool(server, &mcp.Tool{
		Name:        "echo",
		Description: "Counts how many times the underlying handler runs.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		calls++
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil, nil
	})

	AttachRateLimit(server, NewRateLimiter(1, 2))

	session, ctx := connectClient(t, server)

	for i := range 2 {
		res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "echo"})
		if err != nil {
			t.Fatalf("CallTool #%d: %v", i+1, err)
		}
		if res.IsError {
			t.Fatalf("CallTool #%d returned IsError=true within burst", i+1)
		}
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "echo"})
	if err != nil {
		t.Fatalf("CallTool over-budget: %v", err)
	}
	if !res.IsError {
		t.Fatalf("CallTool over-budget: IsError=false, want true")
	}
	if calls != 2 {
		t.Errorf("handler invocations = %d, want 2 (rate limit must short-circuit)", calls)
	}
	if len(res.Content) == 0 {
		t.Fatal("rate-limited result has empty Content")
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("rate-limited content type = %T, want *TextContent", res.Content[0])
	}
	if !strings.Contains(text.Text, "rate limit") {
		t.Errorf("rate-limited message = %q, want to contain 'rate limit'", text.Text)
	}
	if !strings.Contains(text.Text, "echo") {
		t.Errorf("rate-limited message = %q, want to contain tool name 'echo'", text.Text)
	}
}

// TestAttachRateLimit_ListNotGated verifies that tools/list bypasses the
// limiter so clients can always discover available tools regardless of
// burst exhaustion.
func TestAttachRateLimit_ListNotGated(t *testing.T) {
	t.Parallel()
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	registerEchoTool(server)
	AttachRateLimit(server, NewRateLimiter(1, 1))

	session, ctx := connectClient(t, server)
	for i := range 5 {
		if _, err := session.ListTools(ctx, nil); err != nil {
			t.Fatalf("ListTools #%d: %v", i+1, err)
		}
	}
}

// TestValidateRateLimit verifies the validation rules for limiter
// configuration: rps must be >= 0, and burst must be >= 1 when rps > 0.
func TestValidateRateLimit(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		rps     float64
		burst   int
		wantErr bool
	}{
		{"disabled", 0, 0, false},
		{"valid", 10, 5, false},
		{"valid_default_burst", 1, 40, false},
		{"negative_rps", -1, 1, true},
		{"zero_burst_with_rps", 1, 0, true},
		{"negative_burst_with_rps", 1, -5, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateRateLimit(tc.rps, tc.burst)
			gotErr := err != nil
			if gotErr != tc.wantErr {
				t.Fatalf("ValidateRateLimit(%g, %d) err = %v, wantErr %v", tc.rps, tc.burst, err, tc.wantErr)
			}
			if tc.wantErr && !errors.Is(err, ErrInvalidRateLimit) {
				t.Errorf("error %v does not wrap ErrInvalidRateLimit", err)
			}
		})
	}
}

// registerEchoTool adds a no-op echo tool used by rate-limit tests.
func registerEchoTool(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "echo",
		Description: "Echo tool used for rate-limit middleware verification tests.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil, nil
	})
}

// connectClient wires an in-memory transport to server and returns a client
// session ready to issue requests. Cleanup closes the session on test exit.
func connectClient(t *testing.T, server *mcp.Server) (*mcp.ClientSession, context.Context) {
	t.Helper()
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	session, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session, ctx
}

// TestExtractToolName_NilRequest verifies that extractToolName is safe for
// nil input (returns empty string).
func TestExtractToolName_NilRequest(t *testing.T) {
	t.Parallel()
	if got := extractToolName(nil); got != "" {
		t.Errorf("extractToolName(nil) = %q, want empty", got)
	}
}
