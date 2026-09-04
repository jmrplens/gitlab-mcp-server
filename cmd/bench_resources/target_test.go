// target_test.go covers what a measured process is handed before it starts:
// the environment it inherits, the buffer that collects its two output streams
// while it is still running, and the port reservation.
//
// The environment is the part that matters most. Every figure this command
// publishes is a figure for one configuration, and the configuration is
// supposed to come from the scenario plan. A developer machine exporting
// GITLAB_MCP_TOOL_SURFACE would otherwise have every scenario measure that
// surface while the page said otherwise, and nothing in the output would look
// wrong.
package main

import (
	"context"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
)

// envValue returns the value of name in a KEY=VALUE list, and whether it is
// there at all.
func envValue(env []string, name string) (string, bool) {
	for _, entry := range env {
		if key, value, ok := strings.Cut(entry, "="); ok && key == name {
			return value, true
		}
	}
	return "", false
}

// TestConfigFreeEnviron_DropsEverythingThisServerReads verifies the ambient
// environment is stripped of every variable that would configure the process
// under measurement.
//
// Three families have to go: anything GITLAB_ prefixed, anything OTEL_
// prefixed, and the bare spellings the config package accepts alongside the
// prefixed ones. That last family is the one worth a test, because it is read
// from config.PrefixedEnvNames rather than restated here, so a setting added
// there has to be stripped without anybody editing this command.
func TestConfigFreeEnviron_DropsEverythingThisServerReads(t *testing.T) {
	t.Setenv("GITLAB_URL", "https://gitlab.example.com")
	t.Setenv("GITLAB_TOKEN", "glpat-not-a-real-token")
	t.Setenv("GITLAB_MCP_TOOL_SURFACE", "individual")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://collector.invalid")
	t.Setenv("OTEL_SDK_DISABLED", "true")
	t.Setenv("TOOL_SURFACE", "meta")
	t.Setenv("BENCH_RESOURCES_KEEP_ME", "kept")

	env := configFreeEnviron()

	for _, name := range []string{
		"GITLAB_URL", "GITLAB_TOKEN", "GITLAB_MCP_TOOL_SURFACE",
		"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_SDK_DISABLED", "TOOL_SURFACE",
	} {
		t.Run("drops "+name, func(t *testing.T) {
			if value, ok := envValue(env, name); ok {
				t.Errorf("%s survived as %q; the measured process would be configured by the developer's shell", name, value)
			}
		})
	}

	t.Run("keeps everything else", func(t *testing.T) {
		if value, ok := envValue(env, "BENCH_RESOURCES_KEEP_ME"); !ok || value != "kept" {
			t.Errorf("BENCH_RESOURCES_KEEP_ME = %q, %v; an unrelated variable must survive", value, ok)
		}
	})

	// The bare names are the ones nobody would think to strip by hand, so
	// check the whole list rather than the one spelled out above.
	for _, bare := range config.PrefixedEnvNames() {
		t.Run("drops the bare "+bare, func(t *testing.T) {
			t.Setenv(bare, "set-by-the-developer")
			if _, ok := envValue(configFreeEnviron(), bare); ok {
				t.Errorf("the bare spelling %s survived", bare)
			}
		})
	}
}

// TestChildEnv_StdioScenario_ConfiguresTheProcessItself verifies a stdio
// scenario passes the instance, credential and surface through the
// environment, since a stdio server has no other way to be told.
func TestChildEnv_StdioScenario_ConfiguresTheProcessItself(t *testing.T) {
	t.Setenv("GITLAB_MCP_TOOL_SURFACE", "individual")

	plan := scenarioPlan{ID: "stdio-meta", Transport: transportStdio, Surface: surfaceMeta}
	env := childEnv(plan, "http://stub.invalid", "http://otlp.invalid", true)

	cases := []struct{ name, want string }{
		{"GITLAB_URL", "http://stub.invalid"},
		{"GITLAB_MCP_TOOL_SURFACE", surfaceMeta},
		{"GOTRACEBACK", "all"},
		{"GITLAB_MCP_LOG_LEVEL", "error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got, ok := envValue(env, tc.name); !ok || got != tc.want {
				t.Errorf("%s = %q, %v; want %q", tc.name, got, ok, tc.want)
			}
		})
	}

	t.Run("the surface is the plan's, not the shell's", func(t *testing.T) {
		// childEnv appends after configFreeEnviron has stripped the ambient
		// value, so the last occurrence has to be the plan's. Reading the
		// first would find nothing; reading the last is what exec does.
		var seen []string
		for _, entry := range env {
			if key, value, ok := strings.Cut(entry, "="); ok && key == "GITLAB_MCP_TOOL_SURFACE" {
				seen = append(seen, value)
			}
		}
		if len(seen) != 1 || seen[0] != surfaceMeta {
			t.Errorf("GITLAB_MCP_TOOL_SURFACE occurrences = %v, want exactly [%s]", seen, surfaceMeta)
		}
	})
}

// TestChildEnv_HTTPScenario_LeavesTheInstanceToTheFlags verifies an HTTP
// scenario is not given GITLAB_URL, because that transport takes its instance
// from a flag and its credential from each request.
func TestChildEnv_HTTPScenario_LeavesTheInstanceToTheFlags(t *testing.T) {
	plan := scenarioPlan{ID: "http-dynamic", Transport: transportHTTP, Surface: surfaceDynamic}
	env := childEnv(plan, "http://stub.invalid", "http://otlp.invalid", false)

	for _, name := range []string{"GITLAB_URL", "GITLAB_TOKEN", "GITLAB_MCP_TOOL_SURFACE"} {
		t.Run("no "+name, func(t *testing.T) {
			if value, ok := envValue(env, name); ok {
				t.Errorf("%s = %q; an HTTP scenario is configured by flags", name, value)
			}
		})
	}
}

// TestChildEnv_Telemetry_UsesTheSpecifiedDurationUnit verifies the OTEL
// durations are plain integers.
//
// Every OTEL_ duration is an integer number of milliseconds by specification.
// A Go duration string such as "2s" parses as nothing there and leaves the
// ten-second default in place, so a telemetry scenario would quietly measure
// the default batching rather than the fast batching it asked for.
func TestChildEnv_Telemetry_UsesTheSpecifiedDurationUnit(t *testing.T) {
	plan := scenarioPlan{ID: "http-dynamic-telemetry", Transport: transportHTTP, Surface: surfaceDynamic, Telemetry: true}
	env := childEnv(plan, "http://stub.invalid", "http://otlp.invalid", false)

	if value, ok := envValue(env, "GITLAB_MCP_TELEMETRY"); !ok || value != "true" {
		t.Errorf("GITLAB_MCP_TELEMETRY = %q, %v; want true", value, ok)
	}
	if value, ok := envValue(env, "OTEL_EXPORTER_OTLP_ENDPOINT"); !ok || value != "http://otlp.invalid" {
		t.Errorf("OTEL_EXPORTER_OTLP_ENDPOINT = %q, %v; want the sink", value, ok)
	}
	for _, name := range []string{
		"OTEL_EXPORTER_OTLP_TIMEOUT",
		"OTEL_BSP_SCHEDULE_DELAY",
		"OTEL_METRIC_EXPORT_INTERVAL",
		"OTEL_BLRP_SCHEDULE_DELAY",
	} {
		t.Run(name, func(t *testing.T) {
			value, ok := envValue(env, name)
			if !ok {
				t.Fatalf("%s is absent", name)
			}
			if strings.ContainsAny(value, "smh") {
				t.Errorf("%s = %q, want an integer number of milliseconds", name, value)
			}
		})
	}
}

// TestChildEnv_NoTelemetry_SetsNoOTELVariable verifies a plain scenario is not
// handed exporter configuration, so telemetry-off really measures telemetry
// off.
func TestChildEnv_NoTelemetry_SetsNoOTELVariable(t *testing.T) {
	plan := scenarioPlan{ID: "http-dynamic", Transport: transportHTTP, Surface: surfaceDynamic}
	for _, entry := range childEnv(plan, "http://stub.invalid", "http://otlp.invalid", false) {
		if strings.HasPrefix(entry, "OTEL_") || strings.HasPrefix(entry, "GITLAB_MCP_TELEMETRY=") {
			t.Errorf("a telemetry-off scenario was given %q", entry)
		}
	}
}

// TestLockedBuffer_ConcurrentWrites_LoseNothing verifies the buffer collecting
// a child's two streams is safe to hand to both at once, which is exactly how
// it is used: stdout and stderr of a running process write to it concurrently
// while the harness reads it.
func TestLockedBuffer_ConcurrentWrites_LoseNothing(t *testing.T) {
	var buf lockedBuffer
	const writers, each = 8, 64

	var wg sync.WaitGroup
	for range writers {
		wg.Go(func() {
			for range each {
				if _, err := buf.Write([]byte("x")); err != nil {
					// Not t.Fatalf: this is not the test goroutine.
					t.Errorf("Write: %v", err)
					return
				}
			}
		})
	}
	wg.Wait()

	if got := len(buf.String()); got != writers*each {
		t.Errorf("collected %d bytes, want %d", got, writers*each)
	}
}

// TestFreePort_ReturnsAPortThatCanBeBound verifies the reservation hands back
// a usable port and releases it, since the caller's next move is to give it to
// a child process.
func TestFreePort_ReturnsAPortThatCanBeBound(t *testing.T) {
	port, err := freePort(context.Background())
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	if port <= 0 || port > 65535 {
		t.Fatalf("freePort returned %d, want a usable TCP port", port)
	}

	// The port has to be free again, or the process this reserves it for
	// could not start.
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(context.Background(), "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("the reserved port %d was not released: %v", port, err)
	}
	if closeErr := listener.Close(); closeErr != nil {
		t.Errorf("closing the test listener: %v", closeErr)
	}
}

// TestFreePort_SuccessiveCalls_DoNotCollide verifies two reservations do not
// hand back the same port while the first is still unused.
//
// The race with another process is documented and accepted, which is why start
// waits for health rather than trusting the reservation. This only pins that
// the command does not race with itself, which it would if the port came from
// anywhere but the kernel.
func TestFreePort_SuccessiveCalls_DoNotCollide(t *testing.T) {
	first, err := freePort(context.Background())
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	var listenConfig net.ListenConfig
	held, err := listenConfig.Listen(context.Background(), "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(first)))
	if err != nil {
		t.Fatalf("holding the first port: %v", err)
	}
	defer func() { _ = held.Close() }()

	second, err := freePort(context.Background())
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	if second == first {
		t.Errorf("both reservations returned %d while the first was held", first)
	}
}
