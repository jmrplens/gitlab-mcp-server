package telemetry

import (
	"context"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// TestDropToolName_AutoProtectsTheSurfaceThatNeedsIt is the decision this file
// exists to make, asserted per surface because the whole point is that they
// differ.
//
// The individual surface registers up to about 1071 tools, one per catalog
// action. As a metric dimension that is up to 1071 time series per method
// against a default limit of 2000 per instrument. The dynamic surface has two
// tool names and the meta surface about fifty, and on the dynamic surface the
// attribute is nearly all a metric has to go on, so dropping it there would
// cost real information for no benefit.
func TestDropToolName_AutoProtectsTheSurfaceThatNeedsIt(t *testing.T) {
	for surface, wantDropped := range map[string]bool{
		"individual": true,
		"meta":       false,
		"dynamic":    false,
		"":           false,
	} {
		if got := DropToolName(ToolNameAuto, surface); got != wantDropped {
			t.Errorf("auto on the %q surface drops the tool name = %v, want %v", surface, got, wantDropped)
		}
	}
}

// TestDropToolName_ExplicitPoliciesOverrideTheSurface covers the operator who
// has decided for themselves: one who knows their backend can afford a thousand
// series and wants per-tool latency, and one who wants the smallest possible
// footprint whatever they are running.
func TestDropToolName_ExplicitPoliciesOverrideTheSurface(t *testing.T) {
	for _, surface := range []string{"individual", "meta", "dynamic"} {
		if DropToolName(ToolNameOn, surface) {
			t.Errorf("on dropped the tool name on the %q surface", surface)
		}
		if !DropToolName(ToolNameOff, surface) {
			t.Errorf("off kept the tool name on the %q surface", surface)
		}
	}
}

// TestParseToolNamePolicy_RefusesAnUnknownValueByName asserts a typo fails
// loudly rather than selecting a default.
//
// Silently defaulting is worse here than it looks: the value only matters on
// one surface, so an operator who mistyped it would find the dimension present
// in staging and missing in production, or the reverse, with nothing anywhere
// explaining the difference.
func TestParseToolNamePolicy_RefusesAnUnknownValueByName(t *testing.T) {
	if _, err := ParseToolNamePolicy("yes"); err == nil {
		t.Fatal("an unrecognized policy was accepted")
	} else if !strings.Contains(err.Error(), "yes") {
		t.Errorf("the error does not name the offending value: %v", err)
	}

	for input, want := range map[string]ToolNamePolicy{
		"":       ToolNameAuto,
		"AUTO":   ToolNameAuto,
		"  on  ": ToolNameOn,
		"Off":    ToolNameOff,
		"auto":   ToolNameAuto,
	} {
		got, err := ParseToolNamePolicy(input)
		if err != nil {
			t.Errorf("ParseToolNamePolicy(%q): %v", input, err)
			continue
		}
		if got != want {
			t.Errorf("ParseToolNamePolicy(%q) = %q, want %q", input, got, want)
		}
	}
}

// TestToolNameView_RemovesOnlyTheToolName drives the View through the real SDK,
// because that is the only thing that proves the filter does what it claims.
//
// A View is a callback the SDK invokes per instrument, and its AttributeFilter
// runs inside aggregation. A test asserting the predicate directly would prove
// the predicate and nothing about whether the Stream was accepted, whether the
// filter was applied, or whether the other attributes survived it, which is the
// half that would break a dashboard silently.
func TestToolNameView_RemovesOnlyTheToolName(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
		sdkmetric.WithView(toolNameView()),
	)
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	histogram, err := provider.Meter("test").Float64Histogram("mcp.server.operation.duration")
	if err != nil {
		t.Fatalf("creating the instrument: %v", err)
	}
	histogram.Record(context.Background(), 0.1, metric.WithAttributes(
		attribute.String("gen_ai.tool.name", "gitlab_issue_list"),
		attribute.String("mcp.method.name", "tools/call"),
		attribute.String("gitlab_mcp.action", "issue.list"),
	))

	found := collectAttributes(t, reader)
	if _, present := found["gen_ai.tool.name"]; present {
		t.Error("gen_ai.tool.name survived the view; the individual surface would mint a series per tool")
	}
	for _, key := range []string{"mcp.method.name", "gitlab_mcp.action"} {
		if _, present := found[key]; !present {
			t.Errorf("%s was filtered out as well; the view is an allow-list rather than one rejection", key)
		}
	}
}

// TestToolNameView_CollapsesSeriesRatherThanDroppingData pins what the filter
// is actually for, which is not hiding an attribute but bounding the series
// count.
//
// Two calls to different tools must become one time series with two
// measurements, not two series and not one measurement. Losing a measurement
// would be a far worse outcome than the cardinality it was meant to prevent.
func TestToolNameView_CollapsesSeriesRatherThanDroppingData(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
		sdkmetric.WithView(toolNameView()),
	)
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	histogram, err := provider.Meter("test").Float64Histogram("mcp.server.operation.duration")
	if err != nil {
		t.Fatalf("creating the instrument: %v", err)
	}
	for _, tool := range []string{"gitlab_issue_list", "gitlab_branch_delete"} {
		histogram.Record(context.Background(), 0.1, metric.WithAttributes(
			attribute.String("gen_ai.tool.name", tool),
			attribute.String("mcp.method.name", "tools/call"),
		))
	}

	var collected metricdata.ResourceMetrics
	if collectErr := reader.Collect(context.Background(), &collected); collectErr != nil {
		t.Fatalf("collecting: %v", collectErr)
	}

	var points int
	var count uint64
	for _, scope := range collected.ScopeMetrics {
		for _, m := range scope.Metrics {
			data, ok := m.Data.(metricdata.Histogram[float64])
			if !ok {
				continue
			}
			points += len(data.DataPoints)
			for _, point := range data.DataPoints {
				count += point.Count
			}
		}
	}

	if points != 1 {
		t.Errorf("%d data points, want 1: two tools must collapse into one series", points)
	}
	if count != 2 {
		t.Errorf("%d measurements, want 2: collapsing series must not drop data", count)
	}
}

// collectAttributes flattens every attribute on every histogram point.
func collectAttributes(t *testing.T, reader *sdkmetric.ManualReader) map[string]string {
	t.Helper()

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &collected); err != nil {
		t.Fatalf("collecting: %v", err)
	}
	found := map[string]string{}
	for _, scope := range collected.ScopeMetrics {
		for _, m := range scope.Metrics {
			data, ok := m.Data.(metricdata.Histogram[float64])
			if !ok {
				continue
			}
			for _, point := range data.DataPoints {
				for _, kv := range point.Attributes.ToSlice() {
					found[string(kv.Key)] = kv.Value.AsString()
				}
			}
		}
	}
	return found
}
