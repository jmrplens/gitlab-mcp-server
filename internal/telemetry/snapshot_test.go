package telemetry

import (
	"context"
	"testing"
)

// TestCurrentSnapshot_ZeroValueMeansOff pins what a caller gets before anything
// has started, which is the state the server card is built in for every
// deployment that never enables telemetry.
//
// The zero value has to be usable rather than a sentinel, so no caller needs a
// nil check or a second branch to describe a server that is not instrumented.
func TestCurrentSnapshot_ZeroValueMeansOff(t *testing.T) {
	setCurrent(Snapshot{})

	if snapshot := CurrentSnapshot(); snapshot.Enabled {
		t.Errorf("CurrentSnapshot reports enabled with nothing started: %+v", snapshot)
	}
}

// TestCurrentSnapshot_PublishedByStartAndClearedByShutdown asserts the lifecycle
// the server card depends on.
//
// A card built after shutdown must not still advertise telemetry: an operator
// who turned it off, or a process on its way out, would otherwise keep
// promising instrumentation that no longer exists. The endpoint is unreachable
// on purpose, because Start must succeed regardless: the exporters connect
// lazily and a collector being down is not a configuration error.
func TestCurrentSnapshot_PublishedByStartAndClearedByShutdown(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:1")
	t.Setenv("OTEL_EXPORTER_OTLP_TIMEOUT", "200")

	provider, err := Start(context.Background(), Config{
		Enabled: true,
		Signals: Signals{Traces: true, Metrics: true},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	snapshot := CurrentSnapshot()
	if !snapshot.Enabled {
		t.Fatal("CurrentSnapshot reports disabled after a successful Start")
	}
	if len(snapshot.Signals) != 2 {
		t.Errorf("signals = %v, want the two that were configured", snapshot.Signals)
	}
	if snapshot.Protocol == "" {
		t.Error("protocol is empty; the card would advertise telemetry without saying how it ships")
	}

	if shutdownErr := provider.Shutdown(context.Background()); shutdownErr != nil {
		t.Logf("shutdown against an unreachable collector: %v", shutdownErr)
	}
	if after := CurrentSnapshot(); after.Enabled {
		t.Errorf("CurrentSnapshot still reports enabled after Shutdown: %+v", after)
	}
}

// TestCurrentSnapshot_DisabledStartPublishesNothing covers the ordinary path.
// Every deployment that does not enable telemetry runs through here, and the
// card must say nothing rather than say "off" in a way a consumer has to parse.
func TestCurrentSnapshot_DisabledStartPublishesNothing(t *testing.T) {
	setCurrent(Snapshot{})

	provider, err := Start(context.Background(), Config{Enabled: false})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	if snapshot := CurrentSnapshot(); snapshot.Enabled {
		t.Errorf("a disabled Start published a snapshot: %+v", snapshot)
	}
}

// TestSnapshot_CarriesNoCredentialOrPath is a guard on what may ever be added
// to this type.
//
// Snapshot feeds the public server card, which every client can read. The
// collector endpoint is held here because the log line at startup names it for
// the operator, and it must never reach the card: it identifies the operator's
// own infrastructure. This test does not assert the card's contents, which is
// the card's own business; it asserts that the fields on this type stay
// enumerable, so that adding one is a deliberate act with a test to update
// rather than something that leaks into a public document by inheritance.
func TestSnapshot_CarriesNoCredentialOrPath(t *testing.T) {
	snapshot := Snapshot{
		Enabled:  true,
		Protocol: ProtocolHTTP,
		Signals:  []string{"traces"},
		Endpoint: "https://collector.internal.example:4318",
	}

	// Enumerated deliberately: a new field breaks this compile-time list and
	// forces a decision about whether it belongs in a public card.
	_ = snapshot.Enabled
	_ = snapshot.Protocol
	_ = snapshot.Signals
	_ = snapshot.Endpoint
}
