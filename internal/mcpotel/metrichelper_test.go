package mcpotel

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// newMetricRecorder installs a real meter provider that collects into memory.
//
// A manual reader rather than a fake meter, for the same reason the span tests
// use a real tracer provider: the rules being asserted are enforced inside the
// SDK. Attribute-set deduplication, the cardinality limit, and the exact shape
// of a histogram's data points are all SDK behavior, and a fake would accept
// whatever it was handed and prove nothing about what a collector would receive.
func newMetricRecorder(t *testing.T) (*metric.ManualReader, func()) {
	t.Helper()

	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))

	previous := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)

	return reader, func() {
		otel.SetMeterProvider(previous)
		_ = provider.Shutdown(context.Background())
	}
}

// collectedAttributes returns every attribute on every data point collected.
//
// Flattened deliberately. An assertion about what must never appear has to look
// everywhere rather than at the one instrument the test author had in mind: a
// value leaking onto a metric nobody thought to check is exactly the case worth
// catching.
func collectedAttributes(t *testing.T, reader *metric.ManualReader) []attribute.KeyValue {
	t.Helper()

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &collected); err != nil {
		t.Fatalf("collecting metrics: %v", err)
	}

	var attrs []attribute.KeyValue
	eachDataPointAttributes(collected, func(points []attribute.KeyValue) {
		attrs = append(attrs, points...)
	})
	return attrs
}

// collectedHistogram returns one named histogram, or fails.
func collectedHistogram(t *testing.T, reader *metric.ManualReader, name string) metricdata.Metrics {
	t.Helper()

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &collected); err != nil {
		t.Fatalf("collecting metrics: %v", err)
	}
	for _, scope := range collected.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name == name {
				return m
			}
		}
	}
	t.Fatalf("no metric named %q was recorded", name)
	return metricdata.Metrics{}
}

// eachDataPointAttributes calls fn with the attribute set of every data point,
// whatever the instrument's type.
//
// The type switch rather than one assertion is the whole point. Every
// instrument this server has today is a Histogram[float64], so a helper that
// handled only that type was right by accident, and an assertion about what
// must never appear on a metric would have gone on passing the day somebody
// added a counter carrying it. The failure would be silent, in the direction of
// green.
func eachDataPointAttributes(collected metricdata.ResourceMetrics, fn func([]attribute.KeyValue)) {
	for _, scope := range collected.ScopeMetrics {
		for _, m := range scope.Metrics {
			for _, attrs := range dataPointAttributeSets(m.Data) {
				fn(attrs)
			}
		}
	}
}

// dataPointAttributeSets returns the attribute set of every data point in one
// instrument's aggregation, whatever its type.
//
// Split from the sweep above only because the type switch is six near-identical
// arms and a linter counts that as complexity. It is not: each arm is the same
// sentence about a different generic instantiation, which Go has no way to say
// once.
func dataPointAttributeSets(data metricdata.Aggregation) [][]attribute.KeyValue {
	var out [][]attribute.KeyValue
	switch d := data.(type) {
	case metricdata.Histogram[float64]:
		for _, p := range d.DataPoints {
			out = append(out, p.Attributes.ToSlice())
		}
	case metricdata.Histogram[int64]:
		for _, p := range d.DataPoints {
			out = append(out, p.Attributes.ToSlice())
		}
	case metricdata.Sum[float64]:
		for _, p := range d.DataPoints {
			out = append(out, p.Attributes.ToSlice())
		}
	case metricdata.Sum[int64]:
		for _, p := range d.DataPoints {
			out = append(out, p.Attributes.ToSlice())
		}
	case metricdata.Gauge[float64]:
		for _, p := range d.DataPoints {
			out = append(out, p.Attributes.ToSlice())
		}
	case metricdata.Gauge[int64]:
		for _, p := range d.DataPoints {
			out = append(out, p.Attributes.ToSlice())
		}
	}
	return out
}

// eachNamedDataPoint is the same sweep, with the instrument's name, for the
// assertions that report which metric carried something.
func eachNamedDataPoint(collected metricdata.ResourceMetrics, fn func(name string, attrs []attribute.KeyValue)) {
	for _, scope := range collected.ScopeMetrics {
		for _, m := range scope.Metrics {
			eachDataPointAttributes(
				metricdata.ResourceMetrics{ScopeMetrics: []metricdata.ScopeMetrics{{Metrics: []metricdata.Metrics{m}}}},
				func(attrs []attribute.KeyValue) { fn(m.Name, attrs) },
			)
		}
	}
}
