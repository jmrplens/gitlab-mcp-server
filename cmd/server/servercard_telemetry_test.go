package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/telemetry"
)

// TestServerCard_AnnouncesTelemetryWithoutNamingTheCollector is two assertions
// that pull in opposite directions, which is why they belong in one test.
//
// The card must SAY telemetry is running. The switch is off by default for
// privacy, and a privacy default nobody can observe is worth less than one they
// can: somebody connecting to a published endpoint should be able to see that
// their calls are instrumented without having to ask the operator.
//
// The card must NOT say where the telemetry goes. The collector endpoint names
// the operator's own infrastructure, and a server card is fetched by every
// client that asks. What a caller needs is that their calls are recorded and in
// what form; where the records land is not theirs to know.
func TestServerCard_AnnouncesTelemetryWithoutNamingTheCollector(t *testing.T) {
	const collector = "https://collector.internal.example:4318"

	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", collector)
	t.Setenv("OTEL_EXPORTER_OTLP_TIMEOUT", "200")

	provider, err := telemetry.Start(t.Context(), telemetry.Config{
		Enabled: true,
		Signals: telemetry.Signals{Traces: true},
	})
	if err != nil {
		t.Fatalf("starting telemetry: %v", err)
	}
	t.Cleanup(func() { _ = provider.Shutdown(boundedShutdown(t)) })

	cfg := &config.Config{SkipTLSVerify: true, MetaTools: true}
	data, err := buildServerCard(t.Context(), cfg)
	if err != nil {
		t.Fatalf("buildServerCard: %v", err)
	}

	if strings.Contains(string(data), collector) {
		t.Errorf("the server card names the collector endpoint; it identifies the operator's infrastructure and every client can read this document\n%s", data)
	}
	if strings.Contains(string(data), "collector.internal.example") {
		t.Error("the collector host reached the card in some other form")
	}

	var card map[string]any
	if unmarshalErr := json.Unmarshal(data, &card); unmarshalErr != nil {
		t.Fatalf("the card is not valid JSON: %v", unmarshalErr)
	}
	block, ok := card["telemetry"].(map[string]any)
	if !ok {
		t.Fatalf("the card carries no telemetry block while telemetry is running:\n%s", data)
	}
	if enabled, _ := block["enabled"].(bool); !enabled {
		t.Error("the telemetry block does not report itself enabled")
	}
	if signals, _ := block["signals"].([]any); len(signals) == 0 {
		t.Error("the telemetry block names no signals, so a reader cannot tell what is recorded")
	}
	for _, key := range []string{"recorded", "not_recorded"} {
		if text, _ := block[key].(string); text == "" {
			t.Errorf("the telemetry block has no %q line; the point of announcing is to say what is captured", key)
		}
	}
}

// TestServerCard_OmitsTelemetryWhenItIsOff covers every deployment that never
// enables it, which is the default and therefore the common case.
//
// Absent rather than "enabled": false. A consumer should not have to parse a
// negation to learn that nothing is being recorded, and a block that is always
// present invites one written by hand that says the wrong thing.
func TestServerCard_OmitsTelemetryWhenItIsOff(t *testing.T) {
	provider, err := telemetry.Start(t.Context(), telemetry.Config{Enabled: false})
	if err != nil {
		t.Fatalf("starting telemetry: %v", err)
	}
	t.Cleanup(func() { _ = provider.Shutdown(boundedShutdown(t)) })

	cfg := &config.Config{SkipTLSVerify: true, MetaTools: true}
	data, err := buildServerCard(t.Context(), cfg)
	if err != nil {
		t.Fatalf("buildServerCard: %v", err)
	}

	var card map[string]any
	if unmarshalErr := json.Unmarshal(data, &card); unmarshalErr != nil {
		t.Fatalf("the card is not valid JSON: %v", unmarshalErr)
	}
	if _, present := card["telemetry"]; present {
		t.Errorf("the card carries a telemetry block with telemetry off:\n%s", data)
	}
}
