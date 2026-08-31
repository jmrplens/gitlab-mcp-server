package mcpotel

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/attribute"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/telemetry"
)

// staticResources answers with whatever it was built with, so a test can drive
// both policies without building a redactor.
type staticResources []attribute.KeyValue

func (r staticResources) ResourceAttributes(uri string) []attribute.KeyValue {
	if uri == "" {
		return nil
	}
	return r
}

// TestResourceAttributes_ReachTheSpanAndNeverAMetric is the regression for a
// hole opened by the change that added the attributes.
//
// describe appends them to call.attributes, and that list feeds both signals,
// so the resource became a metric dimension the moment it became a span
// attribute. Either form is one distinct value per project, merge request,
// pipeline or job a client touches: a series count no operator can predict and
// no deployment can bound.
//
// It reached production before anything caught it. The collector module's
// inventory test exists for precisely this and missed it, because its
// never-on-a-metric list named mcp.resource.uri and not the
// gitlab_mcp.resource.ref key invented alongside it.
func TestResourceAttributes_ReachTheSpanAndNeverAMetric(t *testing.T) {
	tests := []struct {
		name string
		attr attribute.KeyValue
	}{
		{name: "the digest under the default policy", attr: AttrResourceRef.String("10a1a87eec6fea96")},
		{name: "the URI under full", attr: AttrResourceURI.String("gitlab://project/82077663")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reader, restore := newMetricRecorder(t)
			defer restore()
			recorder := newRecorder(t)

			handler := Middleware(Options{Resources: staticResources{tc.attr}})(
				func(context.Context, string, mcp.Request) (mcp.Result, error) {
					return &mcp.ReadResourceResult{}, nil
				},
			)
			_, _ = handler(context.Background(), "resources/read",
				&mcp.ReadResourceRequest{Params: &mcp.ReadResourceParams{URI: "gitlab://project/82077663"}})

			var onSpan bool
			for _, span := range recorder.Ended() {
				if _, ok := attrOf(span, tc.attr.Key); ok {
					onSpan = true
				}
			}
			if !onSpan {
				t.Errorf("%s is absent from the span, where it is the only thing naming what was read", tc.attr.Key)
			}

			if instrument, _ := metricCarryingValue(t, reader, tc.attr.Value.AsString()); instrument != "" {
				t.Errorf("%s reached metric %s; it is one series per resource a client touches", tc.attr.Key, instrument)
			}
		})
	}
}

// TestResourceAttributeKeys_MatchTheRedactor guards a drift this package cannot
// see on its own.
//
// internal/telemetry decides which key a resource is named under and this
// package decides which keys a metric may not carry, and the two write the
// strings out separately because mcpotel imports the OpenTelemetry API and
// never the SDK. A rename on one side would silently stop the filter working:
// the attribute would still be produced, the filter would still run, and it
// would match nothing.
//
// A test can import both, so the constraint is checkable even though the
// production code cannot express it.
func TestResourceAttributeKeys_MatchTheRedactor(t *testing.T) {
	if string(AttrResourceURI) != telemetry.AttrResourceURI {
		t.Errorf("mcpotel says %q and telemetry says %q; the metric filter would match nothing",
			AttrResourceURI, telemetry.AttrResourceURI)
	}
	if string(AttrResourceRef) != telemetry.AttrResourceRef {
		t.Errorf("mcpotel says %q and telemetry says %q; the metric filter would match nothing",
			AttrResourceRef, telemetry.AttrResourceRef)
	}
}
