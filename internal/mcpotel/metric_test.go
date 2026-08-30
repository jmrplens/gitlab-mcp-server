package mcpotel

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// TestMiddleware_DurationHistogramMatchesTheConvention pins the three
// properties a dashboard silently depends on, none of which produces an error
// when wrong.
//
// The name is what every query is written against. The unit is seconds, which
// the convention fixes and which is the opposite of the milliseconds this
// server logs, so it is exactly the kind of thing a reader assumes rather than
// checks. And the bucket boundaries are a SHOULD in the convention with a very
// different default in the Go SDK: passing them is what makes a p99 comparable
// between this server and any other MCP server an operator runs.
//
// Getting any of the three wrong produces a working metric that means something
// else, which is worse than a missing one.
func TestMiddleware_DurationHistogramMatchesTheConvention(t *testing.T) {
	reader, restore := newMetricRecorder(t)
	defer restore()

	handler := Middleware(Options{Surface: "dynamic"})(
		func(context.Context, string, mcp.Request) (mcp.Result, error) {
			return &mcp.CallToolResult{}, nil
		},
	)
	_, _ = handler(context.Background(), "tools/call",
		callToolRequest("gitlab_issue_list", map[string]any{}, nil))

	recorded := collectedHistogram(t, reader, "mcp.server.operation.duration")

	if recorded.Unit != "s" {
		t.Errorf("unit = %q, want %q; the convention fixes seconds and this server logs milliseconds, so the two disagree by a factor of a thousand",
			recorded.Unit, "s")
	}

	histogram, ok := recorded.Data.(metricdata.Histogram[float64])
	if !ok {
		t.Fatalf("data is %T, want a float64 histogram", recorded.Data)
	}
	if len(histogram.DataPoints) == 0 {
		t.Fatal("no data point was recorded for a call that completed")
	}

	want := []float64{0.01, 0.02, 0.05, 0.1, 0.2, 0.5, 1, 2, 5, 10, 30, 60, 120, 300}
	got := histogram.DataPoints[0].Bounds
	if len(got) != len(want) {
		t.Fatalf("%d bucket boundaries, want %d; the SDK default was used instead of the convention's",
			len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("boundary %d = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestMiddleware_DurationCarriesTheMethodAndTheAction asserts the metric can
// answer the question it exists for.
//
// A duration with no dimensions is a single number for the whole server, which
// tells an operator that something is slow and nothing about what. The method
// is Required by the convention; the action is ours and is the only thing that
// distinguishes two calls on the default surface, where every tools/call names
// the same tool.
func TestMiddleware_DurationCarriesTheMethodAndTheAction(t *testing.T) {
	reader, restore := newMetricRecorder(t)
	defer restore()

	identifier := IdentifierFunc(func(string, any) (Identity, bool) {
		return Identity{ActionID: "issue.list", Domain: "issue"}, true
	})
	handler := Middleware(Options{Identifier: identifier, Surface: "dynamic"})(
		func(context.Context, string, mcp.Request) (mcp.Result, error) {
			return &mcp.CallToolResult{}, nil
		},
	)
	_, _ = handler(context.Background(), "tools/call",
		callToolRequest("gitlab_execute_action", map[string]any{"action": "issue.list"}, nil))

	found := map[string]string{}
	for _, kv := range collectedAttributes(t, reader) {
		found[string(kv.Key)] = kv.Value.AsString()
	}
	for key, want := range map[string]string{
		string(AttrMCPMethodName): "tools/call",
		string(AttrActionID):      "issue.list",
		string(AttrToolSurface):   "dynamic",
	} {
		if found[key] != want {
			t.Errorf("%s = %q on the metric, want %q", key, found[key], want)
		}
	}
}
