package telemetry

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
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
	// A tight teardown bound: these tests assert on the snapshot and never on
	// an export, so the final flush has nowhere to deliver and waiting
	// shutdownTimeout for it would buy each test five silent seconds.
	// Shutdown honors the tighter caller deadline.
	//
	// The bound is necessary and not sufficient, which is why every endpoint
	// below is an address and never a name. A gRPC exporter builds its
	// ClientConn eagerly, so the resolver starts looking the host up at
	// Start, and grpc.ClientConn.Close waits for that watcher to finish
	// while ignoring the context it was given: a name nothing resolves cost
	// the caller a full system-resolver timeout (5.00s here) that no
	// deadline of ours could cut short. An IP literal is handed straight to
	// gRPC's ipResolver with no lookup at all.
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		_ = p.Shutdown(ctx)
	})
	return p.Snapshot()
}

// TestSnapshot_MetricsOnlyDeploymentIsNotDescribedByTraces is the regression.
//
// Provider.protocol held the traces value unconditionally, so a metrics-only
// deployment exporting over gRPC published "http/protobuf" on the server card:
// not imprecise but false, in a document a client reads. The traces variable is
// set here to a value nothing will use, which is precisely the shape that used
// to be reported.
//
// The two endpoints are TEST-NET-1 addresses (RFC 5737), unreachable by
// definition and distinct so the assertion below can say which one the
// snapshot picked.
func TestSnapshot_MetricsOnlyDeploymentIsNotDescribedByTraces(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", "http/protobuf")
	t.Setenv("OTEL_EXPORTER_OTLP_METRICS_PROTOCOL", "grpc")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "http://192.0.2.1:4318")
	t.Setenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", "http://192.0.2.2:4317")

	got := startForSnapshot(t, Signals{Metrics: true})

	if got.Protocol == "http/protobuf" {
		t.Error("the snapshot reports the traces protocol for a deployment that exports no traces")
	}
	if got.Protocol != "grpc" {
		t.Errorf("protocol = %q, want %q: one enabled signal agrees with itself", got.Protocol, "grpc")
	}
	if got.Endpoint != "http://192.0.2.2:4317" {
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
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://192.0.2.3:4318")

	got := startForSnapshot(t, Signals{Traces: true, Metrics: true})

	if got.Protocol != "" {
		t.Errorf("protocol = %q; two enabled signals use different transports, so there is no process-wide answer", got.Protocol)
	}
	if got.SignalProtocols["traces"] != "http/protobuf" || got.SignalProtocols["metrics"] != "grpc" {
		t.Errorf("per-signal protocols = %v, want each signal's own", got.SignalProtocols)
	}
	// The endpoint comes from the shared variable, so that one does agree, and
	// disagreement on one field must not blank the other.
	if got.Endpoint != "http://192.0.2.3:4318" {
		t.Errorf("endpoint = %q; both signals resolve the same one, so it has a summary", got.Endpoint)
	}
}

// TestSnapshot_AgreeingSignalsKeepTheSummary is the common case, and the one a
// consumer of the server card actually meets: everything over one transport to
// one collector.
func TestSnapshot_AgreeingSignalsKeepTheSummary(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "grpc")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://192.0.2.4:4317")

	got := startForSnapshot(t, AllSignals())

	if got.Protocol != "grpc" {
		t.Errorf("protocol = %q, want grpc for three signals that agree", got.Protocol)
	}
	if got.Endpoint != "http://192.0.2.4:4317" {
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
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not mention %q", err, want)
			}
		})
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

// TestProviderAbandon_ShutsDownWhatHadAlreadyStarted covers the path a partial
// failure takes.
//
// Start builds the signals in order, so a failure on the third leaves the first
// two running with nobody holding them: goroutines batching for an exporter
// that will never be flushed, in a process that thinks telemetry did not start.
// abandon is what stops that, and it was reached by no test.
func TestProviderAbandon_ShutsDownWhatHadAlreadyStarted(t *testing.T) {
	provider, err := Start(context.Background(), Config{Enabled: true, Signals: AllSignals()})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	cause := errors.New("the third signal failed")
	returned := provider.abandon(boundedShutdown(t), cause)

	if !errors.Is(returned, cause) {
		t.Errorf("abandon returned %v, which does not wrap the cause; the caller would report the wrong reason", returned)
	}
	if provider.Enabled() {
		t.Error("the provider still reports itself enabled after being abandoned")
	}

	// Idempotent, because the caller defers a shutdown and abandon has already
	// run one: a second pass must not report an error the caller would then
	// log as a failure to clean up.
	if again := provider.Shutdown(boundedShutdown(t)); again != nil {
		t.Errorf("shutting down an abandoned provider returned %v", again)
	}
}

// TestEnvSwitch_ReadsTheServerOwnVariable covers the switch an operator sets
// when they do not use the flag.
//
// It is this server's own variable rather than a standard one, so nothing else
// validates its spelling, and it follows the specification's boolean grammar
// rather than Go's: envBool has its own test for that, and this asserts the
// wiring reaches it.
func TestEnvSwitch_ReadsTheServerOwnVariable(t *testing.T) {
	tests := []struct {
		name  string
		value string
		set   bool
		want  bool
	}{
		{name: "unset is off, which is the default this server promises", want: false},
		{name: "true", set: true, value: "true", want: true},
		{name: "TRUE", set: true, value: "TRUE", want: true},
		{name: "false", set: true, value: "false", want: false},
		{name: "one is not true here", set: true, value: "1", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set {
				t.Setenv(EnvSwitchName, tc.value)
			} else {
				os.Unsetenv(EnvSwitchName)
			}
			if got := EnvSwitch(); got != tc.want {
				t.Errorf("EnvSwitch() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestProvider_ZeroValueAndNil_AnswerAsAnUntelemeteredProcess covers the
// accessors on a provider that never started.
//
// Both shapes are ordinary rather than defensive: Start returns &Provider{} when
// telemetry is off or vetoed, and the server card asks the same questions
// whether or not anything is running. Each answer has to be the untelemetered
// one — no signals, a snapshot that says disabled — rather than a panic on the
// path that renders a public document.
func TestProvider_ZeroValueAndNil_AnswerAsAnUntelemeteredProcess(t *testing.T) {
	t.Parallel()

	var absent *Provider
	if got := absent.Signals(); got != (Signals{}) {
		t.Errorf("(*Provider)(nil).Signals() = %+v, want no signals", got)
	}
	if absent.Enabled() {
		t.Error("(*Provider)(nil).Enabled() = true")
	}

	off := &Provider{}
	if got := off.Signals(); got != (Signals{}) {
		t.Errorf("Signals() = %+v on a provider that never started, want no signals", got)
	}
	snapshot := off.Snapshot()
	if snapshot.Enabled {
		t.Errorf("Snapshot() = %+v, want Enabled false", snapshot)
	}
	if len(snapshot.Signals) != 0 {
		t.Errorf("Snapshot().Signals = %v, want nothing announced", snapshot.Signals)
	}
}

// TestProviderAbandon_WithNothingStarted_ReturnsTheCauseAlone covers the
// abandon path when the shutdown it runs has nothing to flush.
//
// The cause has to come back unwrapped and unjoined, because it is the error
// the operator will see: joining a nil shutdown error would be invisible, but
// returning anything other than the cause would report the wrong reason for a
// failed start. The globals are put back to no-ops on the way out so a signal
// that had already installed its provider does not leave every instrument in
// the process pointing at a dead pipeline.
func TestProviderAbandon_WithNothingStarted_ReturnsTheCauseAlone(t *testing.T) {
	provider := &Provider{enabled: true}
	cause := errors.New("the first signal failed")

	returned := provider.abandon(boundedShutdown(t), cause)

	if !errors.Is(returned, cause) {
		t.Errorf("abandon returned %v, want the cause it was given", returned)
	}
	if returned.Error() != cause.Error() {
		t.Errorf("abandon returned %q, want exactly the cause with nothing joined onto it", returned)
	}
	// The globals are what a failed start must leave behind: no-ops, so every
	// instrument already handed out by a signal that did come up points at
	// nothing rather than at a pipeline nobody will flush.
	if _, isNoop := otel.GetTracerProvider().(nooptrace.TracerProvider); !isNoop {
		t.Errorf("tracer provider = %T after abandon, want the no-op", otel.GetTracerProvider())
	}
}

// TestStart_WithoutAnySignalSelected_ExportsEverything covers the default an
// operator gets by turning telemetry on and saying nothing else.
//
// The zero Signals value means "not chosen", not "none": a config that selected
// nothing would otherwise start a provider that reports itself enabled while
// exporting nothing at all, which is the failure mode that looks exactly like a
// broken collector from the outside.
func TestStart_WithoutAnySignalSelected_ExportsEverything(t *testing.T) {
	// A port nothing listens on: the OTLP exporters connect lazily, so this
	// asserts what starting does without depending on a collector.
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:1")
	t.Setenv("OTEL_EXPORTER_OTLP_TIMEOUT", "200")
	t.Setenv("OTEL_BSP_EXPORT_TIMEOUT", "200")

	provider, err := Start(context.Background(), Config{Enabled: true})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = provider.Shutdown(boundedShutdown(t)) })

	if got := provider.Signals(); got != AllSignals() {
		t.Errorf("Signals() = %+v, want every signal when the config selected none", got)
	}
	snapshot := provider.Snapshot()
	if !snapshot.Enabled {
		t.Error("Snapshot() reports telemetry disabled after a successful Start")
	}
	if len(snapshot.Signals) != 3 {
		t.Errorf("Snapshot().Signals = %v, want all three announced", snapshot.Signals)
	}
}

// TestBuildResource_ServiceNameInResourceAttributes_BeatsTheDefault covers the
// second spelling of the same operator decision.
//
// OTEL_SERVICE_NAME is the obvious one and has its own test; service.name
// inside OTEL_RESOURCE_ATTRIBUTES is what a deployment written against the
// specification's resource variable uses, and it has to win over this server's
// built-in default in exactly the same way. Reading only the dedicated variable
// discarded it silently, with no error and no log line, while the doc comment
// claimed the environment won.
func TestBuildResource_ServiceNameInResourceAttributes_BeatsTheDefault(t *testing.T) {
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "deployment.environment=staging,service.name=named-by-the-operator")

	res, err := buildResource(context.Background(), Config{ServiceVersion: "2.7.6"})
	if err != nil {
		t.Fatalf("buildResource: %v", err)
	}

	var name, version, environment string
	for _, attr := range res.Attributes() {
		switch attr.Key {
		case semconv.ServiceNameKey:
			name = attr.Value.AsString()
		case semconv.ServiceVersionKey:
			version = attr.Value.AsString()
		case "deployment.environment":
			environment = attr.Value.AsString()
		}
	}

	if name != "named-by-the-operator" {
		t.Errorf("service.name = %q, want the value from OTEL_RESOURCE_ATTRIBUTES", name)
	}
	if version != "2.7.6" {
		t.Errorf("service.version = %q, want the configured version", version)
	}
	if environment != "staging" {
		t.Errorf("deployment.environment = %q, want the rest of OTEL_RESOURCE_ATTRIBUTES kept", environment)
	}
}

// TestStart_EachSignalValidatesItsOwnProtocol pins that the per-signal protocol
// variables are resolved for the signals that are on, and only those.
//
// A metrics-only deployment cannot act on a complaint about the traces
// protocol, and refusing to start over a variable that would never be read is a
// failure with no fix. The mirror case is the one that must fail: a misspelled
// protocol for a signal this deployment does export has to stop startup with
// the name in it, rather than surface later as a rejected batch nobody sees.
func TestStart_EachSignalValidatesItsOwnProtocol(t *testing.T) {
	tests := []struct {
		name     string
		variable string
		signals  Signals
		wantErr  string
	}{
		{
			name:     "the metrics protocol is refused for a metrics deployment",
			variable: "OTEL_EXPORTER_OTLP_METRICS_PROTOCOL",
			signals:  Signals{Metrics: true},
			wantErr:  "metrics OTLP protocol",
		},
		{
			name:     "the logs protocol is refused for a logs deployment",
			variable: "OTEL_EXPORTER_OTLP_LOGS_PROTOCOL",
			signals:  Signals{Logs: true},
			wantErr:  "logs OTLP protocol",
		},
		{
			name:     "a disabled signal's protocol is not read",
			variable: "OTEL_EXPORTER_OTLP_METRICS_PROTOCOL",
			signals:  Signals{Traces: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.variable, "carrier-pigeon")
			t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:1")
			t.Setenv("OTEL_EXPORTER_OTLP_TIMEOUT", "200")

			provider, err := Start(context.Background(), Config{Enabled: true, Signals: tt.signals})
			t.Cleanup(func() { _ = provider.Shutdown(boundedShutdown(t)) })

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Start refused a signal that is off: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("Start accepted a protocol it cannot honor for a signal it exports")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to name %q", err, tt.wantErr)
			}
		})
	}
}

// TestSpanLimits_BoundAnAttributeValueUnlessTheOperatorChose covers the floor
// under every span attribute, and the guard that keeps it a floor.
//
// The specification leaves the attribute-value length unlimited by default, so
// an attribute is as large as whatever produced it — which on this server's
// pre-authentication HTTP span meant as large as an anonymous caller chose. The
// individual carrier is bounded where it is written; this is the backstop for
// the attributes nobody has written yet.
//
// The environment wins because a value passed as an option beats the
// environment in this SDK, so an unconditional limit would silently override an
// operator who set one, which is the thing this project refuses to do anywhere
// in the OTEL_ namespace.
func TestSpanLimits_BoundAnAttributeValueUnlessTheOperatorChose(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want int
	}{
		{
			name: "nothing configured takes this server's floor",
			want: defaultAttributeValueLength,
		},
		{
			name: "an operator's own limit wins",
			env:  "128",
			want: 128,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != "" {
				t.Setenv("OTEL_SPAN_ATTRIBUTE_VALUE_LENGTH_LIMIT", tt.env)
			}

			limits := spanLimits()
			if limits.AttributeValueLengthLimit != tt.want {
				t.Errorf("AttributeValueLengthLimit = %d, want %d",
					limits.AttributeValueLengthLimit, tt.want)
			}
			// The other limits must still come from the SDK's own reading of
			// the environment: filling one field by hand is not a license to
			// replace the rest with zero values, which would silently drop
			// every attribute past the first.
			if limits.AttributeCountLimit != sdktrace.NewSpanLimits().AttributeCountLimit {
				t.Errorf("AttributeCountLimit = %d, want the SDK's %d: the other limits were replaced rather than kept",
					limits.AttributeCountLimit, sdktrace.NewSpanLimits().AttributeCountLimit)
			}
		})
	}
}

// TestProviderAbandon_JoinsAShutdownFailureOntoTheCause covers the half of
// abandon that reports rather than cleans up.
//
// A start that failed on the third signal and then could not stop the first two
// has two things wrong with it, and the caller logs one line. Returning the
// cause alone would say the third signal failed and leave the operator with no
// hint that exporters are still holding connections; the join is what makes the
// second failure reachable through errors.Is.
func TestProviderAbandon_JoinsAShutdownFailureOntoTheCause(t *testing.T) {
	stopErr := errors.New("the trace exporter refused to flush")
	provider := &Provider{
		enabled:   true,
		shutdowns: []func(context.Context) error{func(context.Context) error { return stopErr }},
	}
	cause := errors.New("the third signal failed")

	returned := provider.abandon(boundedShutdown(t), cause)

	if !errors.Is(returned, cause) {
		t.Errorf("abandon returned %v, which does not wrap the cause; the caller would report the wrong reason", returned)
	}
	if !errors.Is(returned, stopErr) {
		t.Errorf("abandon returned %v, which drops the shutdown failure; exporters left running would go unreported", returned)
	}
}

// TestProviderShutdown_ReportsEveryExporterThatCouldNotStop pins the promise the
// doc comment makes: the errors are joined rather than returned on the first
// failure.
//
// Each exporter owns a connection of its own, so the first one that cannot be
// flushed must not hide the third. A middle exporter that stops cleanly sits
// between them on purpose: a collector that only recorded failures would pass
// this with two entries, and one that only recorded successes would return nil
// and report a clean shutdown of a provider that did not have one.
func TestProviderShutdown_ReportsEveryExporterThatCouldNotStop(t *testing.T) {
	first := errors.New("the trace exporter refused to flush")
	last := errors.New("the log exporter refused to flush")
	provider := &Provider{
		enabled: true,
		shutdowns: []func(context.Context) error{
			func(context.Context) error { return first },
			func(context.Context) error { return nil },
			func(context.Context) error { return last },
		},
	}

	err := provider.Shutdown(boundedShutdown(t))

	if err == nil {
		t.Fatal("Shutdown reported success over two exporters that could not stop")
	}
	if !errors.Is(err, first) {
		t.Errorf("Shutdown returned %v, which does not wrap the first failure", err)
	}
	if !errors.Is(err, last) {
		t.Errorf("Shutdown returned %v, which does not wrap the last failure", err)
	}
	if provider.Enabled() {
		t.Error("the provider still reports itself enabled after a shutdown that reported errors")
	}
}

// TestProviderShutdown_TakesTheTighterOfTheTwoBounds covers the arithmetic the
// flush bound is chosen by, in both directions.
//
// The caller's cancellation is detached on purpose, but a deadline the caller
// chose is a bound to honor: before this, the internal five seconds silently
// overrode any tighter one, which made cmd/server's own shutdown bound dead
// code. The other direction matters just as much and is the easier one to break
// while fixing the first: a caller who allows an hour must not be able to widen
// the internal bound to an hour, or a collector that has gone away holds the
// process open for exactly as long as the caller was willing to wait.
func TestProviderShutdown_TakesTheTighterOfTheTwoBounds(t *testing.T) {
	tests := []struct {
		name   string
		caller time.Duration
		// atMost is what the flush must be given, expressed as an upper bound
		// so the assertion does not race the clock between the two reads.
		atMost time.Duration
	}{
		{
			name:   "a caller deadline tighter than the internal one wins",
			caller: 50 * time.Millisecond,
			atMost: time.Second,
		},
		{
			name:   "a caller deadline looser than the internal one does not widen it",
			caller: time.Hour,
			atMost: shutdownTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var given time.Duration
			var sawDeadline bool
			provider := &Provider{
				enabled: true,
				shutdowns: []func(context.Context) error{
					func(ctx context.Context) error {
						if deadline, ok := ctx.Deadline(); ok {
							given, sawDeadline = time.Until(deadline), true
						}
						return nil
					},
				},
			}

			ctx, cancel := context.WithTimeout(context.Background(), tt.caller)
			defer cancel()
			if err := provider.Shutdown(ctx); err != nil {
				t.Fatalf("Shutdown: %v", err)
			}

			if !sawDeadline {
				t.Fatal("the flush was given a context with no deadline, so neither bound applies to it")
			}
			if given > tt.atMost {
				t.Errorf("the flush was given %s, want at most %s (caller %s, internal %s)",
					given, tt.atMost, tt.caller, shutdownTimeout)
			}
		})
	}
}

// TestProviderShutdown_WithoutACallerDeadline_UsesTheInternalBound is the third
// case of the same choice, and the one where there is nothing to compare
// against.
//
// A caller that passes context.Background is the ordinary path, and the flush
// must still be bounded: an exporter blocking on a collector that has gone away
// is exactly when shutdown is least welcome to hang.
func TestProviderShutdown_WithoutACallerDeadline_UsesTheInternalBound(t *testing.T) {
	var given time.Duration
	var sawDeadline bool
	provider := &Provider{
		enabled: true,
		shutdowns: []func(context.Context) error{
			func(ctx context.Context) error {
				if deadline, ok := ctx.Deadline(); ok {
					given, sawDeadline = time.Until(deadline), true
				}
				return nil
			},
		},
	}

	if err := provider.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	if !sawDeadline {
		t.Fatal("the flush was given no deadline at all, so a stuck exporter would hold the process open")
	}
	if given > shutdownTimeout {
		t.Errorf("the flush was given %s, want at most the internal %s", given, shutdownTimeout)
	}
}

// TestAgreedValue_APartialAgreementIsNotAnAgreement covers the rule the summary
// field rests on.
//
// Two signals naming one collector while a third names another has no single
// answer, and reporting one anyway is how the field came to be wrong in the
// first place. The count check is therefore not an optimization: a map holding
// fewer entries than there are enabled signals means at least one signal
// reported nothing, and an answer drawn from the rest would describe a
// deployment by a value some of it never used.
func TestAgreedValue_APartialAgreementIsNotAnAgreement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		values  map[string]string
		enabled int
		want    string
	}{
		{
			name:    "every enabled signal agrees",
			values:  map[string]string{"traces": "http://c:4318", "metrics": "http://c:4318"},
			enabled: 2,
			want:    "http://c:4318",
		},
		{
			name:    "one enabled signal reported nothing",
			values:  map[string]string{"traces": "http://c:4318"},
			enabled: 2,
			want:    "",
		},
		{
			name:    "the enabled signals disagree",
			values:  map[string]string{"traces": "http://a:4318", "metrics": "http://b:4318"},
			enabled: 2,
			want:    "",
		},
		{
			name:    "nothing is enabled at all",
			values:  map[string]string{},
			enabled: 0,
			want:    "",
		},
		{
			name:    "one signal, which agrees with itself",
			values:  map[string]string{"metrics": "http://c:4318"},
			enabled: 1,
			want:    "http://c:4318",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := agreedValue(tt.values, tt.enabled); got != tt.want {
				t.Errorf("agreedValue(%v, %d) = %q, want %q", tt.values, tt.enabled, got, tt.want)
			}
		})
	}
}

// TestStart_RefusesUnreadableTLSMaterialBeforeBuildingAnything covers the
// second of the two checks Start runs before an exporter exists.
//
// The exporters answer a CA file they cannot read by falling back to the system
// roots and saying nothing, so a typo in the path produces a process that
// announces telemetry enabled and exports to a collector it is not
// authenticating the way the operator asked. Refusing at startup, naming the
// variable, is the only outcome they can act on; and it has to happen before
// anything is built, or the refusal leaves exporters running with no owner.
func TestStart_RefusesUnreadableTLSMaterialBeforeBuildingAnything(t *testing.T) {
	absent := filepath.Join(t.TempDir(), "absent.pem")
	t.Setenv("OTEL_EXPORTER_OTLP_CERTIFICATE", absent)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:1")

	provider, err := Start(context.Background(), Config{Enabled: true, Signals: AllSignals()})

	if err == nil {
		t.Fatalf("Start accepted a CA file it cannot read; the exporters would fall back to the system roots and say nothing")
		return
	}
	if !strings.Contains(err.Error(), "OTEL_EXPORTER_OTLP_CERTIFICATE") {
		t.Errorf("Start refused with %q, want it to name the variable the operator has to fix", err)
	}
	if provider.Enabled() {
		t.Error("the returned provider reports itself enabled after a refused start")
	}
	if got := CurrentSnapshot(); got.Enabled {
		t.Error("a refused start published a snapshot saying telemetry is on")
	}
}

// TestNormalizeProtocol_AcceptsTheSpellingsAnOperatorWrites covers the
// normalization every protocol value passes through, including the two the
// resolver never hands it.
//
// resolveProtocol drops an empty value before it gets here, so "" and the short
// "http" reach this function only from a caller that passes what it was given
// verbatim, and both must resolve to the specification's own spelling rather
// than fall through to the unknown-protocol refusal. The two refusals are
// asserted beside them because their whole value is the message: http/json is
// refused for a reason an operator can act on, and it must not be mistaken for
// a typo.
func TestNormalizeProtocol_AcceptsTheSpellingsAnOperatorWrites(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		want    string
		wantErr string
	}{
		{name: "empty means the default", in: "", want: ProtocolHTTP},
		{name: "whitespace only means the default", in: "   ", want: ProtocolHTTP},
		{name: "the short spelling", in: "http", want: ProtocolHTTP},
		{name: "the specification's own spelling", in: ProtocolHTTP, want: ProtocolHTTP},
		{name: "case is folded", in: "HTTP/Protobuf", want: ProtocolHTTP},
		{name: "grpc", in: ProtocolGRPC, want: ProtocolGRPC},
		{name: "http/json is refused by name", in: "http/json", wantErr: "http/json"},
		{name: "anything else is unknown", in: "thrift", wantErr: "unknown OTLP protocol"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := normalizeProtocol(tt.in)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("normalizeProtocol(%q) = %q with no error, want a refusal naming %q", tt.in, got, tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("normalizeProtocol(%q) refused with %q, want it to name %q", tt.in, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeProtocol(%q): %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("normalizeProtocol(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestSnapshot_ASignalWithNoProtocolContributesNothing covers the guard that
// keeps an unset value out of the per-signal detail.
//
// A signal recorded with an empty protocol would be counted by agreedValue as a
// signal that reported, so the summary field would be computed from one fewer
// answer than there are signals and could claim an agreement that was never
// reached. Omitting the key is what makes "every enabled signal reported one"
// mean what it says.
func TestSnapshot_ASignalWithNoProtocolContributesNothing(t *testing.T) {
	t.Parallel()

	provider := &Provider{
		enabled:        true,
		signals:        Signals{Traces: true, Metrics: true},
		protocol:       ProtocolGRPC,
		endpoint:       "http://192.0.2.4:4317",
		metricProtocol: "",
		metricEndpoint: "",
	}

	got := provider.Snapshot()

	if _, present := got.SignalProtocols["metrics"]; present {
		t.Errorf("a signal with no recorded protocol appears in the detail as %q", got.SignalProtocols["metrics"])
	}
	if _, present := got.SignalEndpoints["metrics"]; present {
		t.Errorf("a signal with no recorded endpoint appears in the detail as %q", got.SignalEndpoints["metrics"])
	}
	if got.Protocol != "" {
		t.Errorf("protocol = %q; one of the two enabled signals reported nothing, so there is no process-wide answer", got.Protocol)
	}
	if got.Endpoint != "" {
		t.Errorf("endpoint = %q; one of the two enabled signals reported nothing, so there is no process-wide answer", got.Endpoint)
	}
	if got.SignalProtocols["traces"] != ProtocolGRPC {
		t.Errorf("the signal that did report is missing from the detail: %v", got.SignalProtocols)
	}
}

// TestEnvHasServiceName_OnlyTheServiceNameAttributeCounts covers the detector
// that decides whether this server supplies its own default name.
//
// Answering yes to an attribute that is not service.name would leave a
// deployment named "unknown_service:gitlab-mcp-server" in every backend, which
// is the SDK's fallback and not a name anybody chose. Answering yes to a
// service.name written without a value would do the same, since the pair
// contributes no name at all.
func TestEnvHasServiceName_OnlyTheServiceNameAttributeCounts(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		attributes  string
		want        bool
	}{
		{name: "nothing names the service", want: false},
		{name: "OTEL_SERVICE_NAME names it", serviceName: "billing", want: true},
		{name: "an empty OTEL_SERVICE_NAME counts as unset", serviceName: "   ", want: false},
		{name: "service.name in the attributes names it", attributes: "service.name=billing", want: true},
		{name: "service.name among other pairs names it", attributes: "deployment.environment=prod,service.name=billing", want: true},
		{name: "surrounding whitespace on the key is trimmed", attributes: " service.name =billing", want: true},
		{name: "another attribute names nothing", attributes: "deployment.environment=prod", want: false},
		{name: "a key with no value names nothing", attributes: "service.name", want: false},
		{name: "an empty attributes variable names nothing", attributes: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Both are set on every row, including to empty: an ambient
			// OTEL_SERVICE_NAME on a developer's machine would otherwise answer
			// yes to every case and the table would assert nothing.
			t.Setenv("OTEL_SERVICE_NAME", tt.serviceName)
			t.Setenv("OTEL_RESOURCE_ATTRIBUTES", tt.attributes)

			if got := envHasServiceName(); got != tt.want {
				t.Errorf("envHasServiceName() = %v with OTEL_SERVICE_NAME=%q OTEL_RESOURCE_ATTRIBUTES=%q, want %v",
					got, tt.serviceName, tt.attributes, tt.want)
			}
		})
	}
}
