package telemetry

import (
	"context"
	"testing"
)

// startForSnapshot starts a provider with the given signals and returns its
// snapshot, shutting it down afterwards.
//
// A real provider rather than a hand-built struct: the whole defect was that
// Start filled one field from one signal, so a test that assembled the fields
// itself would assert the shape and miss the wiring.
func startForSnapshot(t *testing.T, signals Signals) Snapshot {
	t.Helper()

	p, err := Start(context.Background(), Config{Enabled: true, Signals: signals})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = p.Shutdown(boundedShutdown(t)) })
	return p.Snapshot()
}

// TestSnapshot_MetricsOnlyDeploymentIsNotDescribedByTraces is the regression.
//
// Provider.protocol held the traces value unconditionally, so a metrics-only
// deployment exporting over gRPC published "http/protobuf" on the server card:
// not imprecise but false, in a document a client reads. The traces variable is
// set here to a value nothing will use, which is precisely the shape that used
// to be reported.
func TestSnapshot_MetricsOnlyDeploymentIsNotDescribedByTraces(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", "http/protobuf")
	t.Setenv("OTEL_EXPORTER_OTLP_METRICS_PROTOCOL", "grpc")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "http://traces.invalid:4318")
	t.Setenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", "http://metrics.invalid:4317")

	got := startForSnapshot(t, Signals{Metrics: true})

	if got.Protocol == "http/protobuf" {
		t.Error("the snapshot reports the traces protocol for a deployment that exports no traces")
	}
	if got.Protocol != "grpc" {
		t.Errorf("protocol = %q, want %q: one enabled signal agrees with itself", got.Protocol, "grpc")
	}
	if got.Endpoint != "http://metrics.invalid:4317" {
		t.Errorf("endpoint = %q, want the metrics endpoint", got.Endpoint)
	}
	if got.SignalProtocols["metrics"] != "grpc" {
		t.Errorf("per-signal protocol for metrics = %q, want grpc", got.SignalProtocols["metrics"])
	}
	if _, present := got.SignalProtocols["traces"]; present {
		t.Error("a disabled signal appears in the per-signal detail")
	}
}

// TestSnapshot_DisagreeingSignalsReportNoSummary pins the rule that keeps the
// public field honest.
//
// A field that must hold one value for a process that has two has no correct
// answer, and picking one is how it came to be wrong. Empty is the answer, and
// the per-signal detail carries what an operator needs.
func TestSnapshot_DisagreeingSignalsReportNoSummary(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", "http/protobuf")
	t.Setenv("OTEL_EXPORTER_OTLP_METRICS_PROTOCOL", "grpc")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://shared.invalid:4318")

	got := startForSnapshot(t, Signals{Traces: true, Metrics: true})

	if got.Protocol != "" {
		t.Errorf("protocol = %q; two enabled signals use different transports, so there is no process-wide answer", got.Protocol)
	}
	if got.SignalProtocols["traces"] != "http/protobuf" || got.SignalProtocols["metrics"] != "grpc" {
		t.Errorf("per-signal protocols = %v, want each signal's own", got.SignalProtocols)
	}
	// The endpoint comes from the shared variable, so that one does agree, and
	// disagreement on one field must not blank the other.
	if got.Endpoint != "http://shared.invalid:4318" {
		t.Errorf("endpoint = %q; both signals resolve the same one, so it has a summary", got.Endpoint)
	}
}

// TestSnapshot_AgreeingSignalsKeepTheSummary is the common case, and the one a
// consumer of the server card actually meets: everything over one transport to
// one collector.
func TestSnapshot_AgreeingSignalsKeepTheSummary(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "grpc")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://collector.invalid:4317")

	got := startForSnapshot(t, AllSignals())

	if got.Protocol != "grpc" {
		t.Errorf("protocol = %q, want grpc for three signals that agree", got.Protocol)
	}
	if got.Endpoint != "http://collector.invalid:4317" {
		t.Errorf("endpoint = %q, want the shared one", got.Endpoint)
	}
	if len(got.SignalProtocols) != 3 {
		t.Errorf("per-signal protocols = %v, want one per enabled signal", got.SignalProtocols)
	}
}
