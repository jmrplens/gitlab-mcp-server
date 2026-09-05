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
	"path/filepath"
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

	// The old spellings are the ones nobody would think to strip by hand, so
	// check the whole list rather than the one spelled out above; for the
	// settings that already carried GITLAB_, the old spelling is that name
	// and not the bare suffix.
	for _, name := range config.PrefixedEnvNames() {
		old := config.LegacyEnvName(name)
		t.Run("drops the old spelling "+old, func(t *testing.T) {
			t.Setenv(old, "set-by-the-developer")
			if _, ok := envValue(configFreeEnviron(), old); ok {
				t.Errorf("the old spelling %s survived", old)
			}
		})
	}
	t.Run("drops AUTOPILOT, the alias other tooling sets", func(t *testing.T) {
		t.Setenv("AUTOPILOT", "true")
		if _, ok := envValue(configFreeEnviron(), "AUTOPILOT"); ok {
			t.Error("AUTOPILOT survived; it would switch the measured process into yolo mode")
		}
	})
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

// standinPlan is the plan the process-level tests hand a target: two clients,
// two in flight, one round, on the surface the stand-in claims to serve.
func standinPlan(transport string) scenarioPlan {
	return scenarioPlan{
		ID: transport + "-dynamic", Transport: transport, Surface: surfaceDynamic,
		Clients: 2, Parallel: 2, Rounds: 1,
	}
}

// assertClientTalks admits one client and checks it gets the surface back.
func assertClientTalks(t *testing.T, tgt target, index int) *clientConn {
	t.Helper()
	conn, _, err := tgt.addClient(t.Context(), index)
	if err != nil {
		t.Fatalf("addClient(%d): %v", index, err)
	}
	payload, callErr := conn.rpc.call(t.Context(), methodToolsList, nil)
	if callErr != nil {
		t.Fatalf("tools/list on client %d: %v", index, callErr)
	}
	if !strings.Contains(string(payload), "gitlab_find_action") {
		t.Errorf("client %d got %q, want the stand-in's surface", index, payload)
	}
	return conn
}

// TestHTTPTarget_OneProcessServesEveryCredential starts the stand-in the way
// the HTTP scenarios do and walks the target's whole life: readiness through
// /health, one process however many clients, a client per credential, the
// traceback that ends it, and a close that finds nothing left to stop.
func TestHTTPTarget_OneProcessServesEveryCredential(t *testing.T) {
	stub := startStubGitLab()
	t.Cleanup(stub.close)
	tgt := &httpTarget{binary: standinBinary(t), plan: standinPlan(transportHTTP), stubURL: stub.url}
	t.Cleanup(tgt.close)

	if procs := tgt.processes(); procs != nil {
		t.Errorf("processes() before start = %v, want none", procs)
	}
	if _, err := tgt.goroutines(); err == nil {
		t.Error("goroutines() before start returned a count")
	}

	ready, err := tgt.start(t.Context())
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if ready <= 0 {
		t.Errorf("start reported %s to readiness", ready)
	}
	if info := tgt.serverInfo(); info.Version != "standin" || info.Commit == "" {
		t.Errorf("serverInfo = %+v, want what /health reported", info)
	}
	if procs := tgt.processes(); len(procs) != 1 {
		t.Errorf("%d processes, want the one HTTP server", len(procs))
	}

	first := assertClientTalks(t, tgt, 0)
	second := assertClientTalks(t, tgt, 1)
	if procs := tgt.processes(); len(procs) != 1 {
		t.Errorf("%d processes after two clients, want still one", len(procs))
	}
	first.rpc.close()
	second.rpc.close()

	count, dumpErr := tgt.goroutines()
	if dumpErr != nil {
		t.Fatalf("goroutines: %v", dumpErr)
	}
	if count < 1 {
		t.Errorf("counted %d goroutines in the traceback", count)
	}
	tgt.close() // the process is already gone, and closing twice must be harmless
}

// TestHTTPTarget_ExecFailure_IsReportedAsSuch gives the target a binary that
// is not there, which must fail at the exec rather than after sixty seconds of
// polling a port nothing listens on.
func TestHTTPTarget_ExecFailure_IsReportedAsSuch(t *testing.T) {
	tgt := &httpTarget{
		binary: filepath.Join(t.TempDir(), "absent"), plan: standinPlan(transportHTTP), stubURL: "http://127.0.0.1:1",
	}
	_, err := tgt.start(t.Context())
	if err == nil || !strings.Contains(err.Error(), "start server") {
		t.Errorf("start = %v, want the exec failure", err)
	}
}

// TestStdioTarget_EveryClientIsItsOwnProcess spawns two stand-ins the way the
// stdio scenarios do and checks what the transport's shape implies: nothing
// exists before a client, each client is a process, the traceback ends only
// the first, and the second keeps answering until close.
func TestStdioTarget_EveryClientIsItsOwnProcess(t *testing.T) {
	stub := startStubGitLab()
	t.Cleanup(stub.close)
	tgt := &stdioTarget{binary: standinBinary(t), plan: standinPlan(transportStdio), stubURL: stub.url}
	t.Cleanup(tgt.close)

	if ready, err := tgt.start(t.Context()); err != nil || ready != 0 {
		t.Errorf("start = (%s, %v), want the honest zero for a transport with no process yet", ready, err)
	}
	if info := tgt.serverInfo(); info != (ServerInfo{}) {
		t.Errorf("serverInfo = %+v, want empty on stdio", info)
	}
	if _, err := tgt.goroutines(); err == nil {
		t.Error("goroutines() with no process returned a count")
	}

	conn, spawn, err := tgt.addClient(t.Context(), 0)
	if err != nil {
		t.Fatalf("addClient(0): %v", err)
	}
	if spawn <= 0 {
		t.Errorf("the exec took %s", spawn)
	}
	if _, callErr := conn.rpc.call(t.Context(), methodToolsList, nil); callErr != nil {
		t.Fatalf("tools/list on process 0: %v", callErr)
	}
	second := assertClientTalks(t, tgt, 1)
	if procs := tgt.processes(); len(procs) != 2 {
		t.Fatalf("%d processes, want one per client", len(procs))
	}

	count, dumpErr := tgt.goroutines()
	if dumpErr != nil {
		t.Fatalf("goroutines: %v", dumpErr)
	}
	if count < 1 {
		t.Errorf("counted %d goroutines in the traceback", count)
	}
	if _, callErr := second.rpc.call(t.Context(), methodResourcesList, nil); callErr != nil {
		t.Errorf("the second process stopped answering after the first was dumped: %v", callErr)
	}
	if _, callErr := conn.rpc.call(t.Context(), methodResourcesList, nil); callErr == nil {
		t.Error("the dumped process still answered")
	}
	tgt.close()
}

// TestStdioTarget_ExecFailure_NamesTheClient checks a client whose process
// cannot start is reported by index, and leaves no process recorded for the
// sampler to ask about.
func TestStdioTarget_ExecFailure_NamesTheClient(t *testing.T) {
	tgt := &stdioTarget{
		binary: filepath.Join(t.TempDir(), "absent"), plan: standinPlan(transportStdio), stubURL: "http://127.0.0.1:1",
	}
	_, _, err := tgt.addClient(t.Context(), 3)
	if err == nil || !strings.Contains(err.Error(), "start stdio server 3") {
		t.Errorf("addClient = %v, want the exec failure naming client 3", err)
	}
	if procs := tgt.processes(); len(procs) != 0 {
		t.Errorf("%d processes recorded for a client that never started", len(procs))
	}
}
