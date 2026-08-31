package mcpotel

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// unknownName is the sentinel a bounded dimension falls back to.
const unknownName = ErrorTypeOther

// errInvalidParams is what the SDK answers when the tool or prompt named does not
// exist. It is also what it answers for a real name with wrong arguments, which
// is the whole reason the substitution is coarse.
var errInvalidParams = &jsonrpc.Error{Code: -32602, Message: `unknown prompt "not-a-prompt"`}

// TestMetricName_AnUnknownPromptDoesNotBecomeADimensionValue is the regression
// for a hole found by driving the hosted deployment and reading its collector.
//
// A prompts/get naming a prompt that does not exist recorded the invented name
// as a value of gen_ai.prompt.name on mcp.server.operation.duration. The name
// is copied off the request and the convention puts it on the metric, and
// nothing checked that it referred to anything, so a caller could mint one time
// series per string it chose to send. The SDK does not refuse an exhausted
// series budget: it collapses everything past the limit into a single
// otel.metric.overflow bucket, first-come-wins under cumulative temporality, so
// the result is the silent loss of the real series rather than an error.
func TestMetricName_AnUnknownPromptDoesNotBecomeADimensionValue(t *testing.T) {
	reader, restore := newMetricRecorder(t)
	defer restore()

	handler := Middleware(Options{})(
		func(context.Context, string, mcp.Request) (mcp.Result, error) {
			return nil, errInvalidParams
		},
	)
	_, _ = handler(context.Background(), "prompts/get",
		&mcp.GetPromptRequest{Params: &mcp.GetPromptParams{Name: "not-a-prompt"}})

	// The values are collected before they are examined: dimensionValues
	// returns an empty slice when the metric is absent, and a loop over that
	// passes while checking nothing at all.
	values := dimensionValues(t, reader, "gen_ai.prompt.name")
	if len(values) == 0 {
		t.Fatal("no gen_ai.prompt.name was recorded, so this assertion would pass without testing the bound")
	}
	for _, value := range values {
		if value == "not-a-prompt" {
			t.Error("a prompt name that names nothing became a metric dimension value; a caller chooses how many time series this process stores")
		}
		if value != unknownName {
			t.Errorf("gen_ai.prompt.name = %q on a failed lookup, want the %q bucket", value, unknownName)
		}
	}
}

// TestSpanName_AnUnknownPromptIsRecordedVerbatim is the other half, and the
// reason the bound is on the metric alone.
//
// A span has no series budget, and the name a client actually sent is the whole
// content of a report that its calls are failing. Bucketing it there would throw
// away the only evidence of what went wrong.
func TestSpanName_AnUnknownPromptIsRecordedVerbatim(t *testing.T) {
	span := runOnce(t, Options{}, "prompts/get",
		&mcp.GetPromptRequest{Params: &mcp.GetPromptParams{Name: "not-a-prompt"}},
		nil, errInvalidParams)

	value, ok := attrOf(span, AttrGenAIPromptName)
	if !ok {
		t.Fatal("gen_ai.prompt.name is absent from the span")
	}
	if value.AsString() != "not-a-prompt" {
		t.Errorf("span records %q; the span is where the name a client sent must survive exactly", value.AsString())
	}
}

// TestMetricName_AResolvedNameSurvives keeps the bound from swallowing the
// signal it exists to protect.
//
// A call that succeeded named something real, so the metric must carry that
// name rather than the bucket. Without this, a substitution that fired
// unconditionally would pass the test above while making the dimension useless.
func TestMetricName_AResolvedNameSurvives(t *testing.T) {
	reader, restore := newMetricRecorder(t)
	defer restore()

	handler := Middleware(Options{})(
		func(context.Context, string, mcp.Request) (mcp.Result, error) {
			return &mcp.GetPromptResult{}, nil
		},
	)
	_, _ = handler(context.Background(), "prompts/get",
		&mcp.GetPromptRequest{Params: &mcp.GetPromptParams{Name: "review-merge-request"}})

	values := dimensionValues(t, reader, "gen_ai.prompt.name")
	if len(values) == 0 {
		t.Fatal("gen_ai.prompt.name is absent from the metric on a successful prompts/get")
	}
	for _, value := range values {
		if value != "review-merge-request" {
			t.Errorf("gen_ai.prompt.name = %q on a successful call, want the real name", value)
		}
	}
}

// TestMetricName_AnUnknownToolDoesNotBecomeADimensionValue covers the same hole
// on the other attribute that is copied off a request.
//
// It matters most on the individual surface, where the legitimate value space is
// already about eleven hundred names and has the least headroom left before the
// SDK's per-instrument limit.
func TestMetricName_AnUnknownToolDoesNotBecomeADimensionValue(t *testing.T) {
	reader, restore := newMetricRecorder(t)
	defer restore()

	handler := Middleware(Options{})(
		func(context.Context, string, mcp.Request) (mcp.Result, error) {
			return nil, &jsonrpc.Error{Code: -32602, Message: `unknown tool "gitlab_not_a_tool"`}
		},
	)
	_, _ = handler(context.Background(), "tools/call",
		callToolRequest("gitlab_not_a_tool", map[string]any{}, nil))

	values := dimensionValues(t, reader, "gen_ai.tool.name")
	if len(values) == 0 {
		t.Fatal("no gen_ai.tool.name was recorded, so this assertion would pass without testing the bound")
	}
	for _, value := range values {
		if value == "gitlab_not_a_tool" {
			t.Error("a tool name that names nothing became a metric dimension value")
		}
		if value != unknownName {
			t.Errorf("gen_ai.tool.name = %q on a failed lookup, want the %q bucket", value, unknownName)
		}
	}
}

// dimensionValues collects every value recorded for one dimension across every
// instrument, so an assertion about a label space looks at all of it.
func dimensionValues(t *testing.T, reader interface {
	Collect(context.Context, *metricdata.ResourceMetrics) error
}, key string,
) []string {
	t.Helper()

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &collected); err != nil {
		t.Fatalf("collecting metrics: %v", err)
	}

	var out []string
	eachDataPointAttributes(collected, func(attrs []attribute.KeyValue) {
		for _, kv := range attrs {
			if kv.Key == attribute.Key(key) {
				out = append(out, kv.Value.AsString())
			}
		}
	})
	return out
}
