package telemetry

import (
	"context"
	"os"
	"strings"
	"testing"

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
