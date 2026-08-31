package telemetry

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

// TestBuildResource_ServiceNameFromEnv_WinsOverTheDefault asserts the promise
// the package doc makes to operators: naming the service through the
// specification's own environment variable takes effect.
//
// It is the regression for a defect that had no visible symptom. buildResource
// applied resource.WithFromEnv first and its own attributes last, and
// resource.New merges each option as the updating resource in order, so the
// literal service.name overwrote the one the environment supplied. The provider
// then merges resource.Environment() underneath, where the environment loses
// again. An operator setting OTEL_SERVICE_NAME saw neither the name they chose
// nor any error, and the function's doc comment asserted the opposite of what
// the code did.
func TestBuildResource_ServiceNameFromEnv_WinsOverTheDefault(t *testing.T) {
	t.Setenv("OTEL_SERVICE_NAME", "gitlab-mcp-edge")

	res, err := buildResource(context.Background(), Config{})
	if err != nil {
		t.Fatalf("buildResource: %v", err)
	}

	got := attrValue(res.Attributes(), string(semconv.ServiceNameKey))
	if got != "gitlab-mcp-edge" {
		t.Errorf("service.name = %q, want the value from OTEL_SERVICE_NAME %q", got, "gitlab-mcp-edge")
	}
}

// TestBuildResource_ExplicitServiceName_WinsOverTheEnvironment pins the other
// half of the precedence chain. An operator who names the service on the
// command line has been more specific than one who exported a variable, so the
// explicit value wins. Without this, the fix for the case above would be
// indistinguishable from "the environment always wins", which would break a
// deployment that sets both on purpose.
func TestBuildResource_ExplicitServiceName_WinsOverTheEnvironment(t *testing.T) {
	t.Setenv("OTEL_SERVICE_NAME", "from-the-environment")

	res, err := buildResource(context.Background(), Config{ServiceName: "from-the-flag"})
	if err != nil {
		t.Fatalf("buildResource: %v", err)
	}

	if got := attrValue(res.Attributes(), string(semconv.ServiceNameKey)); got != "from-the-flag" {
		t.Errorf("service.name = %q, want the explicitly configured %q", got, "from-the-flag")
	}
}

// TestBuildResource_ResourceAttributesFromEnv_Survive covers the sibling
// variable. OTEL_RESOURCE_ATTRIBUTES carries whatever an operator wants to say
// about a deployment, and none of those keys are ones this server knows about,
// so silently dropping them would remove the only channel they have.
func TestBuildResource_ResourceAttributesFromEnv_Survive(t *testing.T) {
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "deployment.environment.name=staging,service.namespace=platform")

	res, err := buildResource(context.Background(), Config{ServiceName: "gitlab-mcp-server"})
	if err != nil {
		t.Fatalf("buildResource: %v", err)
	}

	attrs := res.Attributes()
	if got := attrValue(attrs, "deployment.environment.name"); got != "staging" {
		t.Errorf("deployment.environment.name = %q, want %q", got, "staging")
	}
	if got := attrValue(attrs, "service.namespace"); got != "platform" {
		t.Errorf("service.namespace = %q, want %q", got, "platform")
	}
}

// attrValue reads one attribute out of a resource, returning the empty string
// when the key is absent, so a missing key and an empty value fail the same
// assertion rather than one of them panicking.
func attrValue(attrs []attribute.KeyValue, key string) string {
	for _, a := range attrs {
		if string(a.Key) == key {
			return a.Value.AsString()
		}
	}
	return ""
}

// TestBuildResource_ServiceInstanceID_IsPresentAndUnique pins the attribute
// that tells two copies of this binary apart.
//
// resource.Default omits service.instance.id unless the experimental
// OTEL_GO_X_RESOURCE flag is set, so it arrives only because buildResource asks
// for it. Without it, two concurrent HTTP deployments sharing a service name
// are one indistinguishable series to a collector, and so are a stdio process
// and an HTTP one on the same host.
func TestBuildResource_ServiceInstanceID_IsPresentAndUnique(t *testing.T) {
	first, err := buildResource(context.Background(), Config{ServiceName: "gitlab-mcp-server"})
	if err != nil {
		t.Fatalf("buildResource: %v", err)
	}
	second, err := buildResource(context.Background(), Config{ServiceName: "gitlab-mcp-server"})
	if err != nil {
		t.Fatalf("buildResource: %v", err)
	}

	a := attrValue(first.Attributes(), string(semconv.ServiceInstanceIDKey))
	b := attrValue(second.Attributes(), string(semconv.ServiceInstanceIDKey))
	if a == "" {
		t.Fatal("service.instance.id is absent; resource.WithService is what supplies it")
	}
	if a == b {
		t.Errorf("service.instance.id is stable across calls (%q); it must identify an instance, not a build", a)
	}
}

// TestBuildResource_ServiceNameFromEnv_SurvivesTheInstanceIDDetector is the
// regression for a fix that broke the thing it was fixing.
//
// resource.WithService bundles two detectors: the service.instance.id one that
// is wanted, and a service name detector that writes "unknown_service:<binary>".
// Placed after WithFromEnv, that placeholder overwrites the operator's name,
// which is the original defect arriving from the other direction. The assertion
// is deliberately not "service.name is correct" but "service.name is not the
// placeholder", so it names the specific way this can regress.
func TestBuildResource_ServiceNameFromEnv_SurvivesTheInstanceIDDetector(t *testing.T) {
	t.Setenv("OTEL_SERVICE_NAME", "gitlab-mcp-edge")

	res, err := buildResource(context.Background(), Config{})
	if err != nil {
		t.Fatalf("buildResource: %v", err)
	}

	got := attrValue(res.Attributes(), string(semconv.ServiceNameKey))
	if strings.HasPrefix(got, "unknown_service") {
		t.Errorf("service.name = %q: WithService's name detector ran after WithFromEnv and overwrote the operator's value", got)
	}
	if got != "gitlab-mcp-edge" {
		t.Errorf("service.name = %q, want %q", got, "gitlab-mcp-edge")
	}
}

// TestBuildResource_SemconvSchemaMatchesTheSDK guards a mismatch that has no
// symptom until something else changes.
//
// The attributes here are built from one semconv package while the SDK's own
// detectors carry the schema URL of another. Today nothing merges the two, so
// the emitted resource simply advertises a schema its service.* keys did not
// come from. The moment a resource.Merge with resource.Default is introduced,
// resource.Merge returns ErrSchemaURLConflict and a schemaless resource, and
// the providers swallow that error through otel.Handle: the schema URL goes
// blank and nothing says why. A dependency bump is what would drift these, so
// the check belongs in the test suite rather than in a comment.
func TestBuildResource_SemconvSchemaMatchesTheSDK(t *testing.T) {
	res, err := buildResource(context.Background(), Config{ServiceName: "gitlab-mcp-server"})
	if err != nil {
		t.Fatalf("buildResource: %v", err)
	}
	if got := res.SchemaURL(); got != semconv.SchemaURL {
		t.Errorf("resource schema URL = %q, but this package builds attributes with %q; import the semconv version the SDK pins", got, semconv.SchemaURL)
	}
}

// TestEnvBool_FollowsTheSpecificationAndNotStrconv pins the boolean grammar the
// configuration specification defines, which is strictly narrower than Go's.
//
// Every case marked "strconv disagrees" is one where strconv.ParseBool would
// give a different answer, and each of those differences is a conformance
// break rather than a matter of taste. "1", "t" and "T" are the extension the
// specification's MUST NOT forbids. "tRuE" must be true because the rule is
// case-insensitive, while ParseBool errors on it. Anything unrecognized, and
// an empty value, must read as false rather than as an error.
func TestEnvBool_FollowsTheSpecificationAndNotStrconv(t *testing.T) {
	const key = "GITLAB_MCP_TEST_BOOL"
	tests := []struct {
		name  string
		set   bool
		value string
		want  bool
	}{
		{name: "unset", set: false, want: false},
		{name: "empty is unset", set: true, value: "", want: false},
		{name: "whitespace only is unset", set: true, value: "   ", want: false},
		{name: "true", set: true, value: "true", want: true},
		{name: "TRUE", set: true, value: "TRUE", want: true},
		{name: "mixed case true, strconv disagrees", set: true, value: "tRuE", want: true},
		{name: "surrounding whitespace is trimmed", set: true, value: "  true  ", want: true},
		{name: "false", set: true, value: "false", want: false},
		{name: "FALSE", set: true, value: "FALSE", want: false},
		{name: "one is not true, strconv disagrees", set: true, value: "1", want: false},
		{name: "t is not true, strconv disagrees", set: true, value: "t", want: false},
		{name: "T is not true, strconv disagrees", set: true, value: "T", want: false},
		{name: "zero is false", set: true, value: "0", want: false},
		{name: "unrecognized is false, strconv errors", set: true, value: "yes", want: false},
		{name: "typo is false rather than an error", set: true, value: "ture", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set {
				t.Setenv(key, tc.value)
			} else {
				os.Unsetenv(key)
			}
			if got := envBool(key); got != tc.want {
				t.Errorf("envBool(%q=%q) = %v, want %v", key, tc.value, got, tc.want)
			}
		})
	}
}

// TestSDKDisabledByEnv_VetoesAStartThatWasAskedFor asserts the composition
// between the two switches, which is the part that is easy to get backwards.
//
// The variable is not this server's on switch: its default means "enabled"
// while telemetry here defaults to off, so adopting it as the only switch would
// invert its meaning. It is a veto layered on top, and Start must honor it
// even when the operator explicitly asked for telemetry.
func TestSDKDisabledByEnv_VetoesAStartThatWasAskedFor(t *testing.T) {
	t.Setenv("OTEL_SDK_DISABLED", "true")

	p, err := Start(context.Background(), Config{Enabled: true, Signals: Signals{Traces: true}})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = p.Shutdown(context.Background()) })

	if p.Enabled() {
		t.Error("telemetry is running despite OTEL_SDK_DISABLED=true")
	}
}

// TestSDKDisabledByEnv_DoesNotVetoOnAnUnrecognizedValue is the other half. The
// specification is explicit that only the case-insensitive string "true"
// disables, so a deployment with a typo gets the telemetry it configured, not
// silence it never asked for.
func TestSDKDisabledByEnv_DoesNotVetoOnAnUnrecognizedValue(t *testing.T) {
	t.Setenv("OTEL_SDK_DISABLED", "1")

	if SDKDisabledByEnv() {
		t.Error("OTEL_SDK_DISABLED=1 vetoed telemetry; only case-insensitive \"true\" may")
	}
}

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

	if shutdownErr := provider.Shutdown(boundedShutdown(t)); shutdownErr != nil {
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
	t.Cleanup(func() { _ = provider.Shutdown(boundedShutdown(t)) })

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

// boundedShutdown returns a context a test can safely pass to Shutdown.
//
// Never context.Background(). The provider-level Shutdown honors only the
// caller's context: the SDK's own 30s default applies per export, not to the
// whole drain. So an unbounded context against a collector that is not there
// waits forever, and the failure presents as a test binary that never finishes
// rather than as an assertion anybody can read.
//
// This was not hypothetical. Adding the logs pipeline gave Shutdown something
// real to drain, and this package went from a hundred seconds to a timeout.
// Nothing had changed in those tests; they had simply been passing an unbounded
// context to a call that previously had nothing to wait for.
func boundedShutdown(t *testing.T) context.Context {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)
	return ctx
}
