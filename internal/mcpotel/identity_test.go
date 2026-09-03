package mcpotel

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/attribute"
)

// staticUsers is an attributer that always answers the same thing, which is
// what a test needs: the policy's own behavior is covered in internal/telemetry.
func staticUsers(attrs ...attribute.KeyValue) UserAttributer {
	return UserAttributerFunc(func(context.Context, mcp.Request) []attribute.KeyValue {
		return attrs
	})
}

// TestMiddleware_IdentityReachesTheSpan asserts the seam works at all.
//
// The identity policy, its three modes and its per-process salt, was written
// and tested before anything called it. That is the failure mode worth pinning:
// a feature that exists, passes its own tests, and is wired to nothing looks
// finished from every angle except the one that matters.
func TestMiddleware_IdentityReachesTheSpan(t *testing.T) {
	span := runOnceWithOptions(t,
		Options{Users: staticUsers(attribute.String("user.id", "42"), attribute.String("user.name", "jane"))},
		"tools/call",
		callToolRequest("gitlab_issue_list", map[string]any{}, nil),
		&mcp.CallToolResult{}, nil)

	for key, want := range map[attribute.Key]string{"user.id": "42", "user.name": "jane"} {
		t.Run(string(key), func(t *testing.T) {
			value, ok := attrOf(span, key)
			if !ok {
				t.Errorf("%s is absent from the span; the identity policy is wired to nothing", key)
				return
			}
			if value.AsString() != want {
				t.Errorf("%s = %q, want %q", key, value.AsString(), want)
			}
		})
	}
}

// TestMiddleware_IdentityIsAbsentByDefault pins the default at the layer that
// emits it, not only at the layer that decides it.
//
// Two places have to agree for nothing to be recorded: the policy has to say
// none, and the middleware has to cope with an attributer that was never
// configured. A nil interface reaching a per-request path is the ordinary case
// here, not an error, and it must produce no attribute rather than a panic.
func TestMiddleware_IdentityIsAbsentByDefault(t *testing.T) {
	span := runOnce(t, Options{}, "tools/call",
		callToolRequest("gitlab_issue_list", map[string]any{}, nil),
		&mcp.CallToolResult{}, nil)

	for _, key := range []attribute.Key{"user.id", "user.name", "user.hash"} {
		t.Run(string(key), func(t *testing.T) {
			if _, ok := attrOf(span, key); ok {
				t.Errorf("%s was recorded with no identity policy configured", key)
			}
		})
	}
}

// TestMiddleware_IdentityNeverReachesTheMetric is the assertion that stops a
// privacy feature from becoming a billing incident.
//
// A span carrying a user id costs one span. A metric dimension carrying a user
// id is one time series per person, forever, and it grows with the number of
// people using the deployment, which is precisely the number an operator cannot
// predict. The Go SDK does not refuse it either: past its cardinality limit it
// folds the overflow into a bucket marked otel.metric.overflow, so the failure
// is silent data destruction rather than an error anybody sees.
//
// The two attribute lists are built separately for this reason, and this test
// is what fails if somebody notices the duplication and helpfully shares them.
func TestMiddleware_IdentityNeverReachesTheMetric(t *testing.T) {
	reader, restore := newMetricRecorder(t)
	defer restore()

	handler := Middleware(Options{
		Users: staticUsers(attribute.String("user.id", "42"), attribute.String("user.name", "jane")),
	})(func(context.Context, string, mcp.Request) (mcp.Result, error) {
		return &mcp.CallToolResult{}, nil
	})
	_, _ = handler(context.Background(), "tools/call",
		callToolRequest("gitlab_issue_list", map[string]any{}, nil))

	for _, kv := range collectedAttributes(t, reader) {
		switch kv.Key {
		case "user.id", "user.name", "user.hash":
			t.Errorf("%s reached a metric dimension as %q; that is one time series per person", kv.Key, kv.Value.AsString())
		}
	}
}
