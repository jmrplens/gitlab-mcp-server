//go:build collectore2e

package collectore2e

import (
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// driveDynamic starts a stack with the given extra environment and makes a few
// dynamic-surface calls through it.
//
// One collector and one server per configuration, because these differ in what
// the process was started with and there is no way to change that afterwards:
// the identity policy, the tool-name policy and the surface are all fixed
// before the first request, which is exactly why they are worth covering here
// rather than in a unit test that can set them per case.
func driveDynamic(t *testing.T, extra map[string]string) (*collector, *server) {
	t.Helper()

	c := startCollector(t)
	env := telemetryEnv(c)
	maps.Copy(env, extra)
	srv := startServer(t, env, "--gitlab-url="+startFakeGitLab(t))

	for i := range 3 {
		srv.callAction(t, i+1, "issue.list", "some-group/some-project")
	}
	return c, srv
}

// TestRealCollector_ToolNamePolicyOff covers the View that exists to stop the
// individual surface exhausting the SDK's series budget.
//
// Both keys, because filtering one and keeping the other is what the View did
// until recently: the individual surface projects one visible tool per catalog
// action, so gen_ai.tool.name and gitlab_mcp.action carry the same eleven
// hundred values and dropping either alone sheds nothing. The span keeps both,
// which is the point of doing this with a View rather than by not recording
// them.
func TestRealCollector_ToolNamePolicyOff(t *testing.T) {
	c, srv := driveDynamic(t, map[string]string{"GITLAB_MCP_TELEMETRY_TOOL_NAME": "off"})

	_, span, ok := c.awaitSpan(t, exportDeadline, func(_ otlpResourceSpans, s otlpSpan) bool {
		return strings.HasPrefix(s.Name, "tools/call")
	})
	if !ok {
		t.Fatalf("no tools/call span.\nCollector:\n%s\nServer:\n%s", c.containerLogs(t), srv.logs())
	}

	t.Run("the span keeps both", func(t *testing.T) {
		for _, key := range []string{"gen_ai.tool.name", "gitlab_mcp.action"} {
			if _, present := attr(span.Attributes, key); !present {
				t.Errorf("%s is absent from the span; the policy is about metric series, not about hiding the value", key)
			}
		}
	})

	t.Run("the metric keeps neither", func(t *testing.T) {
		if _, _, found := c.awaitMetric(t, exportDeadline, durationMetric); !found {
			t.Fatalf("no %s metric, so this would pass vacuously", durationMetric)
		}
		for _, key := range []string{"gen_ai.tool.name", "gitlab_mcp.action"} {
			if instrument := metricDimensionExists(t, c, key); instrument != "" {
				t.Errorf("%s is still a dimension of %s under the off policy", key, instrument)
			}
		}
	})
}

// TestRealCollector_IdentityFullExportsTheReadableUser is the third identity
// policy, and the one an operator reaches for when they have decided to record
// who calls.
//
// The assertion that matters is the same under every policy: whatever reaches a
// span, nothing about the caller reaches a metric. A per-user dimension is
// unbounded by the number of people using the deployment, which is the number
// an operator cannot predict.
func TestRealCollector_IdentityFullExportsTheReadableUser(t *testing.T) {
	c, srv := driveDynamic(t, map[string]string{"GITLAB_MCP_TELEMETRY_IDENTITY": "full"})

	_, span, ok := c.awaitSpan(t, exportDeadline, func(_ otlpResourceSpans, s otlpSpan) bool {
		return strings.HasPrefix(s.Name, "tools/call")
	})
	if !ok {
		t.Fatalf("no tools/call span.\nCollector:\n%s\nServer:\n%s", c.containerLogs(t), srv.logs())
	}

	t.Run("the span names the user", func(t *testing.T) {
		if name, present := attr(span.Attributes, "user.name"); !present || name == "" {
			t.Errorf("user.name is absent under the full policy; recorded %v", keys(span.Attributes))
		}
		if _, present := attr(span.Attributes, "user.hash"); present {
			t.Error("the pseudonym is recorded alongside the readable name, which says the same thing twice")
		}
	})

	t.Run("no identity key is a metric dimension", func(t *testing.T) {
		if _, _, found := c.awaitMetric(t, exportDeadline, durationMetric); !found {
			t.Fatalf("no %s metric, so this would pass vacuously", durationMetric)
		}
		for _, key := range []string{"user.id", "user.name", "user.hash"} {
			if instrument := metricDimensionExists(t, c, key); instrument != "" {
				t.Errorf("%q is a dimension of %s; identity must never reach a metric under any policy", key, instrument)
			}
		}
	})
}

// TestRealCollector_TelemetryOffExportsNothing covers the default, which is the
// configuration almost every deployment runs.
//
// Nothing else asserts it. Every other case here turns telemetry on, so a
// change that started an exporter regardless of the flag would be invisible to
// this module while being the most serious thing it could miss: a server that
// exports without being asked to.
func TestRealCollector_TelemetryOffExportsNothing(t *testing.T) {
	c := startCollector(t)

	// The endpoint is configured and the switch is not, which is the shape that
	// catches a provider started from the environment alone.
	env := telemetryEnv(c)
	env["GITLAB_MCP_TELEMETRY"] = "false"
	srv := startServer(t, env, "--gitlab-url="+startFakeGitLab(t))

	for i := range 3 {
		srv.callAction(t, i+1, "issue.list", "some-group/some-project")
	}

	assertNothingExported(t, c, srv)
}

// TestRealCollector_SDKDisabledVetoesAnEnabledDeployment covers the standard
// variable that overrides this server's own switch.
//
// It is a veto rather than a second on switch, and the direction is easy to get
// backwards: OTEL_SDK_DISABLED defaults to "enabled" while telemetry here
// defaults to off, so adopting it as the switch would invert its meaning. This
// asserts the composition, with the operator explicitly asking for telemetry.
func TestRealCollector_SDKDisabledVetoesAnEnabledDeployment(t *testing.T) {
	c := startCollector(t)

	env := telemetryEnv(c)
	env["OTEL_SDK_DISABLED"] = "true"
	srv := startServer(t, env, "--gitlab-url="+startFakeGitLab(t))

	for i := range 3 {
		srv.callAction(t, i+1, "issue.list", "some-group/some-project")
	}

	assertNothingExported(t, c, srv)
}

// assertNothingExported waits out the export schedules and then fails if any
// file has content.
//
// The wait is what makes this an assertion rather than a race: the exporters
// batch, so a check that ran immediately would pass against a server that was
// about to export.
func assertNothingExported(t *testing.T, c *collector, srv *server) {
	t.Helper()

	// Comfortably past the batch schedule and the metric interval this module
	// configures, both of which are 100ms.
	time.Sleep(5 * time.Second)

	for _, name := range []string{tracesFile, metricsFile, logsFile} {
		path := filepath.Join(c.outDir, name)
		info, err := os.Stat(path)
		if err != nil {
			continue // never created, which is the expected outcome
		}
		if info.Size() > 0 {
			t.Errorf("%s holds %d bytes; the deployment exported telemetry it was not asked for.\nServer:\n%s",
				name, info.Size(), srv.logs())
		}
	}
}
