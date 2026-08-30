package telemetry

import (
	"context"
	"strings"
	"testing"
)

// TestResolveProtocol_SignalSpecificBeatsTheGeneralVariable pins the rule the
// protocol specification states as a MUST: "Each configuration option MUST be
// overridable by a signal specific option."
//
// It is not a refinement. otlptracehttp reads the signal-specific variable
// itself, to pick its payload encoding, so a check that consulted only the
// general variable would let a signal-specific value through to the exporter
// unexamined. For metrics and logs the situation is the opposite and just as
// consequential: otlpmetrichttp, otlpmetricgrpc, otlploghttp and otlploggrpc
// contain no protocol handling at all, so this function is the only thing that
// gives those variables any effect whatsoever.
func TestResolveProtocol_SignalSpecificBeatsTheGeneralVariable(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf")
	t.Setenv(metricsProtocolKey, "grpc")

	got, err := resolveProtocol("", metricsProtocolKey)
	if err != nil {
		t.Fatalf("resolveProtocol: %v", err)
	}
	if got != ProtocolGRPC {
		t.Errorf("metrics protocol = %q, want %q from the signal-specific variable", got, ProtocolGRPC)
	}
}

// TestResolveProtocol_OneSignalDoesNotDecideForAnother is the corollary. A
// deployment sending traces over gRPC and everything else over HTTP is a
// supported configuration, and it only works if each signal reads its own
// variable rather than the first one that happens to be set.
func TestResolveProtocol_OneSignalDoesNotDecideForAnother(t *testing.T) {
	t.Setenv(tracesProtocolKey, "grpc")

	traces, err := resolveProtocol("", tracesProtocolKey)
	if err != nil {
		t.Fatalf("resolveProtocol(traces): %v", err)
	}
	logs, err := resolveProtocol("", logsProtocolKey)
	if err != nil {
		t.Fatalf("resolveProtocol(logs): %v", err)
	}

	if traces != ProtocolGRPC {
		t.Errorf("traces = %q, want %q", traces, ProtocolGRPC)
	}
	if logs != ProtocolHTTP {
		t.Errorf("logs = %q, want the default %q; the traces variable leaked", logs, ProtocolHTTP)
	}
}

// TestResolveProtocol_ConfiguredBeatsTheEnvironment pins this server's house
// precedence, which puts the more specific source first: a protocol chosen on
// the command line beats one exported into the environment.
func TestResolveProtocol_ConfiguredBeatsTheEnvironment(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "grpc")
	t.Setenv(tracesProtocolKey, "grpc")

	got, err := resolveProtocol(ProtocolHTTP, tracesProtocolKey)
	if err != nil {
		t.Fatalf("resolveProtocol: %v", err)
	}
	if got != ProtocolHTTP {
		t.Errorf("protocol = %q, want the configured %q", got, ProtocolHTTP)
	}
}

// TestResolveProtocol_EmptyCountsAsUnsetAtEveryLevel covers the case container
// orchestrators produce constantly: a variable exported with no value because
// the secret or setting behind it was never provided.
//
// "The SDK MUST interpret an empty value of an environment variable the same
// way as when the variable is unset." Reading an empty signal-specific variable
// as a decision would mask the general one that was actually set.
func TestResolveProtocol_EmptyCountsAsUnsetAtEveryLevel(t *testing.T) {
	t.Setenv(tracesProtocolKey, "")
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "grpc")

	got, err := resolveProtocol("", tracesProtocolKey)
	if err != nil {
		t.Fatalf("resolveProtocol: %v", err)
	}
	if got != ProtocolGRPC {
		t.Errorf("protocol = %q, want %q; an empty signal variable masked the general one", got, ProtocolGRPC)
	}
}

// TestResolveProtocol_RefusesJSONFromASignalSpecificVariable is the hole this
// whole function was written to close.
//
// Since v1.46.0 otlptracehttp implements http/json, selecting its payload
// encoding from exactly this variable. otlpmetrichttp and otlploghttp do not.
// So a deployment that set the traces variable alone, with the general one
// unset, would previously have slipped past every check here and emitted JSON
// spans beside protobuf metrics and logs, from one setting that reads like it
// selects one thing. The failure must arrive at startup with a name.
func TestResolveProtocol_RefusesJSONFromASignalSpecificVariable(t *testing.T) {
	t.Setenv(tracesProtocolKey, "http/json")

	_, err := resolveProtocol("", tracesProtocolKey)
	if err == nil {
		t.Fatal("http/json in the signal-specific variable was accepted")
	}
	if !strings.Contains(err.Error(), "http/json") {
		t.Errorf("error does not name the refused value: %v", err)
	}
}

// TestStart_RefusesAnUnhonorableProtocolBeforeBuildingAnything asserts that the
// refusal reaches the caller rather than being buried in one signal's setup.
//
// The value here is a plausible typo rather than a nonsense string, because
// that is what an operator will actually produce, and because the error has to
// be readable enough to tell them what to write instead.
func TestStart_RefusesAnUnhonorableProtocolBeforeBuildingAnything(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuff")

	p, err := Start(context.Background(), Config{Enabled: true, Signals: Signals{Traces: true}})
	if err == nil {
		_ = p.Shutdown(context.Background())
		t.Fatal("Start accepted an unknown protocol")
	}
	for _, want := range []string{"http/protobuff", ProtocolHTTP, ProtocolGRPC} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}
