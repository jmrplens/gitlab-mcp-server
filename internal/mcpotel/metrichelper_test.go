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
	for _, scope := range collected.ScopeMetrics {
		for _, m := range scope.Metrics {
			histogram, ok := m.Data.(metricdata.Histogram[float64])
			if !ok {
				continue
			}
			for _, point := range histogram.DataPoints {
				attrs = append(attrs, point.Attributes.ToSlice()...)
			}
		}
	}
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
