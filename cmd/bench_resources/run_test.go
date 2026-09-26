// run_test.go covers the parts of a scenario run that can be decided without
// starting a server: what the record is allowed to claim about processor time
// when a sample is missing or goes backwards, which processes the sampler is
// pointed at, and what a health response is read as.
package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestCPUFigures_PublishesOnlyWhatWasMeasured verifies the load figures are
// omitted, with a note, whenever the difference behind them is not meaningful.
//
// sampleCPU used to report an unanswered sample as zero, which is
// indistinguishable from an idle process. The caller subtracted one sample
// from the other, so an unanswered second sample against an answered first one
// produced negative seconds and a negative percentage, and both were written
// into the committed record and drawn on the published charts.
func TestCPUFigures_PublishesOnlyWhatWasMeasured(t *testing.T) {
	const wall = 2 * time.Second
	cases := []struct {
		name             string
		startup, total   cpuSample
		loadWall         time.Duration
		wantLoadSeconds  float64
		wantLoadPercent  float64
		wantStartup      float64
		wantTotalSeconds float64
		wantNote         string
	}{
		{
			name:             "both answered",
			startup:          cpuSample{seconds: 1, ok: true},
			total:            cpuSample{seconds: 3, ok: true},
			loadWall:         wall,
			wantStartup:      1,
			wantTotalSeconds: 3,
			wantLoadSeconds:  2,
			wantLoadPercent:  100,
		},
		{
			name:        "second sample unanswered",
			startup:     cpuSample{seconds: 1.2, ok: true},
			total:       cpuSample{ok: false},
			loadWall:    wall,
			wantStartup: 1.2,
			wantNote:    "did not answer",
		},
		{
			name:             "first sample unanswered",
			startup:          cpuSample{ok: false},
			total:            cpuSample{seconds: 3, ok: true},
			loadWall:         wall,
			wantTotalSeconds: 3,
			wantNote:         "did not answer",
		},
		{
			name:             "consumed time fell between samples",
			startup:          cpuSample{seconds: 5, ok: true},
			total:            cpuSample{seconds: 4, ok: true},
			loadWall:         wall,
			wantStartup:      5,
			wantTotalSeconds: 4,
			wantNote:         "fell between samples",
		},
		{
			name:             "no load window to divide by",
			startup:          cpuSample{seconds: 1, ok: true},
			total:            cpuSample{seconds: 3, ok: true},
			loadWall:         0,
			wantStartup:      1,
			wantTotalSeconds: 3,
			wantLoadSeconds:  2,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cpu, notes := cpuFigures(tc.startup, tc.total, tc.loadWall)

			if cpu.StartupSeconds != tc.wantStartup {
				t.Errorf("StartupSeconds = %v, want %v", cpu.StartupSeconds, tc.wantStartup)
			}
			if cpu.TotalSeconds != tc.wantTotalSeconds {
				t.Errorf("TotalSeconds = %v, want %v", cpu.TotalSeconds, tc.wantTotalSeconds)
			}
			if cpu.LoadSeconds != tc.wantLoadSeconds {
				t.Errorf("LoadSeconds = %v, want %v", cpu.LoadSeconds, tc.wantLoadSeconds)
			}
			if cpu.LoadPercent != tc.wantLoadPercent {
				t.Errorf("LoadPercent = %v, want %v", cpu.LoadPercent, tc.wantLoadPercent)
			}
			// Nothing this function publishes may be negative: that is the
			// defect it exists to prevent, whatever the inputs were.
			if cpu.StartupSeconds < 0 || cpu.TotalSeconds < 0 || cpu.LoadSeconds < 0 || cpu.LoadPercent < 0 {
				t.Errorf("published a negative figure: %+v", cpu)
			}

			switch {
			case tc.wantNote == "" && len(notes) != 0:
				t.Errorf("notes = %v, want none", notes)
			case tc.wantNote != "" && len(notes) == 0:
				t.Errorf("notes are empty, want one saying %q", tc.wantNote)
			case tc.wantNote != "" && !strings.Contains(notes[0], tc.wantNote):
				t.Errorf("note = %q, want it to say %q", notes[0], tc.wantNote)
			}
		})
	}
}

// TestNewTarget_PicksTheTransportThePlanNames verifies a plan is measured on
// the transport it asks for, and that both kinds are handed the same stand-in
// servers.
//
// The two targets differ in almost everything a scenario does: one starts a
// process per client and the other one process with a client per credential.
// A plan measured on the wrong one would publish figures under the other
// transport's name and look entirely plausible.
func TestNewTarget_PicksTheTransportThePlanNames(t *testing.T) {
	stub := startStubGitLab()
	defer stub.close()
	sink := startOTLPSink()
	defer sink.close()

	r := &runner{binary: "/somewhere/gitlab-mcp-server", stub: stub, otlp: sink}

	cases := []struct {
		name      string
		transport string
		want      any
	}{
		{"http", transportHTTP, &httpTarget{}},
		{"stdio", transportStdio, &stdioTarget{}},
		// Anything the matrix did not name falls back to stdio, which is the
		// transport a client uses without asking for one.
		{"unknown", "carrier-pigeon", &stdioTarget{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := r.newTarget(scenarioPlan{ID: "t", Transport: tc.transport, Surface: surfaceDynamic})
			if reflect.TypeOf(got) != reflect.TypeOf(tc.want) {
				t.Fatalf("newTarget = %T, want %T", got, tc.want)
			}
			switch target := got.(type) {
			case *httpTarget:
				if target.stubURL != stub.url || target.otlpURL != sink.url || target.binary != r.binary {
					t.Errorf("the target was not given the runner's binary and stand-ins: %+v", target)
				}
			case *stdioTarget:
				if target.stubURL != stub.url || target.otlpURL != sink.url || target.binary != r.binary {
					t.Errorf("the target was not given the runner's binary and stand-ins: %+v", target)
				}
			}
		})
	}
}

// TestSampleCPU_NoProcesses_SaysSo verifies an unanswered reading is reported
// as unanswered rather than as zero, which is the distinction the published
// CPU figures rest on.
func TestSampleCPU_NoProcesses_SaysSo(t *testing.T) {
	s := newSampler(t.Context(), 10*time.Millisecond, func() []int { return nil })
	seconds, ok := sampleCPU(s)
	if ok {
		t.Errorf("sampleCPU reported %v seconds with nothing to sample", seconds)
	}
	if seconds != 0 {
		t.Errorf("an unanswered sample carried %v seconds", seconds)
	}
}

// countingConn is a client connection that records how often it was called
// and answers with whatever the test wants, after a delay when one is set.
type countingConn struct {
	calls atomic.Int64
	err   error
	delay time.Duration
}

func (c *countingConn) call(context.Context, string, map[string]any) ([]byte, error) {
	c.calls.Add(1)
	if c.delay > 0 {
		time.Sleep(c.delay)
	}
	return nil, c.err
}

func (c *countingConn) close() {}

// totalCalls adds up what a set of connections was asked.
func totalCalls(conns []*countingConn) int64 {
	var total int64
	for _, conn := range conns {
		total += conn.calls.Load()
	}
	return total
}

// TestFanOut_IssuesEveryRequestAndTimesEachOne verifies the load generator
// makes clients times parallel calls and returns one duration per success.
//
// The counts are the measurement: a fan-out that dropped requests would
// publish percentiles over fewer samples than the scenario claims, and one
// that dropped failures would publish percentiles that quietly exclude the
// slow path.
func TestFanOut_IssuesEveryRequestAndTimesEachOne(t *testing.T) {
	cases := []struct {
		name          string
		clients       int
		parallel      int
		err           error
		wantSamples   int
		wantFailures  int
		wantFirstErr  bool
		wantTotalCall int
	}{
		{name: "one client, one call", clients: 1, parallel: 1, wantSamples: 1, wantTotalCall: 1},
		{name: "four clients, three in flight", clients: 4, parallel: 3, wantSamples: 12, wantTotalCall: 12},
		{
			name: "every call fails", clients: 2, parallel: 2, err: errors.New("refused"),
			wantFailures: 4, wantFirstErr: true, wantTotalCall: 4,
		},
		{name: "nothing to do", clients: 0, parallel: 4},
		{name: "no requests in flight", clients: 4, parallel: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			conns := make([]*clientConn, 0, tc.clients)
			counters := make([]*countingConn, 0, tc.clients)
			for i := range tc.clients {
				counter := &countingConn{err: tc.err}
				counters = append(counters, counter)
				conns = append(conns, &clientConn{rpc: counter, label: strconv.Itoa(i)})
			}

			samples, failures, firstErr := fanOut(context.Background(), conns, tc.parallel,
				func(ctx context.Context, c *clientConn) error {
					_, err := c.rpc.call(ctx, methodToolsList, nil)
					return err
				})

			if len(samples) != tc.wantSamples {
				t.Errorf("collected %d samples, want %d", len(samples), tc.wantSamples)
			}
			if failures != tc.wantFailures {
				t.Errorf("counted %d failures, want %d", failures, tc.wantFailures)
			}
			if (firstErr != nil) != tc.wantFirstErr {
				t.Errorf("first failure = %v, want an error: %v", firstErr, tc.wantFirstErr)
			}
			if total := totalCalls(counters); total != int64(tc.wantTotalCall) {
				t.Errorf("the connections were called %d times, want %d", total, tc.wantTotalCall)
			}
		})
	}
}

// TestPids_SkipsProcessesThatNeverStarted verifies the sampler is pointed only
// at processes that exist.
//
// A stdio scenario starts one process per client and collects them in a slice,
// so a client that failed to start leaves a nil in it. Passing that on as a
// process id would make every sample fail and the scenario report no memory at
// all, rather than the memory of the clients that did start.
func TestPids_SkipsProcessesThatNeverStarted(t *testing.T) {
	self := &os.Process{Pid: os.Getpid()}
	other := &os.Process{Pid: os.Getpid() + 1}

	cases := []struct {
		name  string
		procs []*os.Process
		want  []int
	}{
		{"none at all", nil, []int{}},
		{"all started", []*os.Process{self, other}, []int{self.Pid, other.Pid}},
		{"one never started", []*os.Process{self, nil, other}, []int{self.Pid, other.Pid}},
		{"none started", []*os.Process{nil, nil}, []int{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := pids(tc.procs)
			if len(got) != len(tc.want) {
				t.Fatalf("pids = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("pids = %v, want %v", got, tc.want)
					return
				}
			}
		})
	}
}

// TestDecodeHealth_ReadsTheBuildTheServerReports verifies the build stamped on
// every published record comes from the process that was actually measured.
//
// The version and commit are printed under the figures, so a run that silently
// read them as empty would publish measurements nobody could attribute to a
// build.
func TestDecodeHealth_ReadsTheBuildTheServerReports(t *testing.T) {
	cases := []struct {
		name        string
		status      int
		body        string
		wantErr     bool
		wantVersion string
		wantCommit  string
	}{
		{
			name:        "a healthy server",
			status:      http.StatusOK,
			body:        `{"version":"2.7.6","commit":"0123456789abcdef"}`,
			wantVersion: "2.7.6",
			wantCommit:  "0123456789abcdef",
		},
		{
			name:   "a server that answers but says nothing",
			status: http.StatusOK,
			body:   `{}`,
		},
		{
			name:    "a server that is not healthy",
			status:  http.StatusServiceUnavailable,
			body:    `{"version":"2.7.6"}`,
			wantErr: true,
		},
		{
			name:    "a body that is not JSON",
			status:  http.StatusOK,
			body:    "not json at all",
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			// Before the body: writing first sends an implicit 200 and the
			// status set afterwards would never reach Result.
			recorder.WriteHeader(tc.status)
			if _, err := recorder.WriteString(tc.body); err != nil {
				t.Fatalf("writing the stand-in body: %v", err)
			}
			resp := recorder.Result()
			defer func() { _ = resp.Body.Close() }()

			info, err := decodeHealth(resp)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("decodeHealth = %+v, want an error", info)
				}
				return
			}
			if err != nil {
				t.Fatalf("decodeHealth: %v", err)
			}
			if info.Version != tc.wantVersion || info.Commit != tc.wantCommit {
				t.Errorf("decodeHealth = %+v, want version %q commit %q", info, tc.wantVersion, tc.wantCommit)
			}
		})
	}
}

// standinRunner is a runner over the stand-in: the two stand-in services, a
// fast sampler and a quiet reporter.
func standinRunner(t *testing.T) *runner {
	t.Helper()
	binary := standinBinary(t)
	stub := startStubGitLab()
	t.Cleanup(stub.close)
	sink := startOTLPSink()
	// Closing waits for every export the sink accepted, so a sink that never
	// finished reading one would hold this test, and the package after it,
	// until the binary's own timeout; bounded, it fails here instead.
	t.Cleanup(func() {
		closed := make(chan struct{})
		go func() {
			sink.close()
			close(closed)
		}()
		select {
		case <-closed:
		case <-time.After(30 * time.Second):
			t.Error("the OTLP sink did not close within 30 s: an export it accepted was never read to its end")
		}
	})
	return &runner{
		binary:         binary,
		stub:           stub,
		otlp:           sink,
		sampleInterval: 20 * time.Millisecond,
		progress:       progressFunc(false),
	}
}

// httpPlan is an HTTP scenario small enough for a test and large enough to
// have a ramp: two credentials, two requests in flight, the rounds given.
func httpPlan(rounds int) scenarioPlan {
	return scenarioPlan{
		ID: "http-dynamic-telemetry", Transport: transportHTTP, Surface: surfaceDynamic,
		Telemetry: true, Clients: 2, Parallel: 2, Rounds: rounds,
	}
}

// TestRunScenario_HTTP_FillsEveryPublishedFigure measures the stand-in over
// HTTP with telemetry on and checks every figure the tables and charts read
// is there: the startup milestones, the idle and per-client resident sets, a
// ramp point per credential, one distribution per method with every call
// counted, and the goroutine count that ends the process.
//
// The plan's three counts differ, two clients, three in flight and four
// rounds, so a header that took one count from another's field cannot come
// out right, and the header is compared whole, transport and surface included.
func TestRunScenario_HTTP_FillsEveryPublishedFigure(t *testing.T) {
	r := standinRunner(t)
	plan := httpPlan(4)
	plan.Parallel = 3

	scenario, err := r.runScenario(t.Context(), plan)
	if err != nil {
		t.Fatalf("runScenario: %v", err)
	}

	header := Scenario{
		ID: scenario.ID, Transport: scenario.Transport, Surface: scenario.Surface, Telemetry: scenario.Telemetry,
		Clients: scenario.Clients, Parallel: scenario.Parallel, Rounds: scenario.Rounds,
	}
	want := Scenario{
		ID: "http-dynamic-telemetry", Transport: transportHTTP, Surface: surfaceDynamic, Telemetry: true,
		Clients: 2, Parallel: 3, Rounds: 4,
	}
	if !reflect.DeepEqual(header, want) {
		t.Errorf("scenario header %+v, want %+v from the plan", header, want)
	}
	assertStartupMeasured(t, scenario)
	assertMemoryMeasured(t, scenario)
	assertLatencyPerMethod(t, scenario, plan.Clients*plan.Parallel*plan.Rounds)
	if scenario.Goroutines < 1 {
		t.Errorf("goroutines = %d, want the traceback counted", scenario.Goroutines)
	}
	if len(scenario.Notes) != 0 {
		t.Errorf("notes %v on a scenario where everything answered", scenario.Notes)
	}
	if r.serverInfo.Version != "standin" {
		t.Errorf("the runner kept server info %+v, want what /health reported", r.serverInfo)
	}
	// Telemetry was on, so the endpoint the harness configured must have
	// been reached: a plan that says telemetry while nothing is exported
	// would publish figures for a configuration that was never measured.
	if r.otlp.requests.Load() == 0 {
		t.Error("the OTLP sink received no export from a scenario with telemetry on")
	}
}

// assertStartupMeasured checks the three startup milestones and the size of
// the cold listing were all taken.
func assertStartupMeasured(t *testing.T, scenario Scenario) {
	t.Helper()
	if s := scenario.Startup; s.ProcessReadyMs <= 0 || s.FirstListMs <= 0 || s.WarmListMs <= 0 {
		t.Errorf("startup milestones %+v, want all three measured", s)
	}
	if scenario.ListBytes == 0 {
		t.Error("the cold tools/list response was not sized")
	}
}

// assertMemoryMeasured checks every resident-set figure came from the kernel
// and the ramp has a point per credential.
func assertMemoryMeasured(t *testing.T, scenario Scenario) {
	t.Helper()
	if m := scenario.Memory; m.IdleMiB <= 0 || m.OneClientMiB <= 0 || m.AllClientsMiB <= 0 || m.PeakMiB <= 0 {
		t.Errorf("memory figures %+v, want every one read from the kernel", m)
	}
	if len(scenario.Ramp) != 2 || scenario.Ramp[1].Client != 2 || scenario.Ramp[1].ColdListMs <= 0 {
		t.Errorf("ramp %+v, want one point per credential", scenario.Ramp)
	}
}

// assertLatencyPerMethod checks each measured method has a distribution over
// every call the plan issued.
func assertLatencyPerMethod(t *testing.T, scenario Scenario, wantCount int) {
	t.Helper()
	for _, method := range []string{methodResourcesList, methodToolsCall, methodToolsList} {
		t.Run(method, func(t *testing.T) {
			latency, ok := scenario.latency(method)
			if !ok {
				t.Fatalf("no distribution for %s", method)
			}
			if latency.Count != wantCount {
				t.Errorf("%d samples, want %d (clients x parallel x rounds)", latency.Count, wantCount)
			}
			if latency.P50 <= 0 || latency.Max < latency.P50 {
				t.Errorf("percentiles %+v are not a distribution", latency)
			}
		})
	}
}

// TestRunScenario_Stdio_TimesTheSpawnAndPublishesNoIdleFigure measures the
// stand-in over stdio, where the record's shape differs on purpose: readiness
// is the exec of the first client's process, and there is no idle figure
// because a stdio process has no idle state to size for.
func TestRunScenario_Stdio_TimesTheSpawnAndPublishesNoIdleFigure(t *testing.T) {
	r := standinRunner(t)
	plan := scenarioPlan{
		ID: "stdio-dynamic", Transport: transportStdio, Surface: surfaceDynamic, Clients: 2, Parallel: 1, Rounds: 1,
	}

	scenario, err := r.runScenario(t.Context(), plan)
	if err != nil {
		t.Fatalf("runScenario: %v", err)
	}
	if scenario.Startup.ProcessReadyMs <= 0 {
		t.Errorf("ProcessReadyMs = %v, want the first client's exec timed", scenario.Startup.ProcessReadyMs)
	}
	if scenario.Memory.IdleMiB != 0 {
		t.Errorf("IdleMiB = %v on stdio, want none", scenario.Memory.IdleMiB)
	}
	if scenario.Memory.AllClientsMiB <= 0 || len(scenario.Ramp) != 2 {
		t.Errorf("memory %+v ramp %+v, want two processes measured", scenario.Memory, scenario.Ramp)
	}
	if scenario.Goroutines < 1 {
		t.Errorf("goroutines = %d, want the first process's traceback counted", scenario.Goroutines)
	}
	if r.serverInfo.Version != "" {
		t.Errorf("stdio reported server info %+v, want none", r.serverInfo)
	}
}

// TestRunScenario_Refusals covers the ways a scenario fails before it has
// anything to publish: a surface with no tool call, an HTTP server that
// cannot start, and a stdio client whose process cannot start.
func TestRunScenario_Refusals(t *testing.T) {
	cases := []struct {
		name string
		plan scenarioPlan
		want string
	}{
		{
			name: "unknown surface",
			plan: scenarioPlan{ID: "x", Transport: transportHTTP, Surface: "nonsense", Clients: 1, Parallel: 1, Rounds: 1},
			want: "unknown surface",
		},
		{
			name: "http server cannot start",
			plan: scenarioPlan{ID: "x", Transport: transportHTTP, Surface: surfaceDynamic, Clients: 1, Parallel: 1, Rounds: 1},
			want: "start server",
		},
		{
			name: "stdio process cannot start",
			plan: scenarioPlan{ID: "x", Transport: transportStdio, Surface: surfaceDynamic, Clients: 1, Parallel: 1, Rounds: 1},
			want: "start stdio server 0",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := standinRunner(t)
			r.binary = filepath.Join(t.TempDir(), "absent")
			_, err := r.runScenario(t.Context(), tc.plan)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("runScenario = %v, want an error saying %q", err, tc.want)
			}
		})
	}
}

// TestRunScenario_GoroutineDumpTimesOut_IsNoted lowers the wait for the
// traceback to nothing, so the process cannot have exited by the time it is
// checked, and verifies the scenario still completes with a note in place
// of the count.
func TestRunScenario_GoroutineDumpTimesOut_IsNoted(t *testing.T) {
	previous := dumpWait
	dumpWait = time.Nanosecond
	t.Cleanup(func() { dumpWait = previous })

	r := standinRunner(t)
	scenario, err := r.runScenario(t.Context(), httpPlan(1))
	if err != nil {
		t.Fatalf("runScenario: %v", err)
	}
	if scenario.Goroutines != 0 {
		t.Errorf("goroutines = %d, want none counted", scenario.Goroutines)
	}
	if !strings.Contains(strings.Join(scenario.Notes, "\n"), "goroutine count unavailable") {
		t.Errorf("notes %v do not say the count was unavailable", scenario.Notes)
	}
}

// TestSettledRSS_NeverSettles_ReturnsAtTheCeiling lowers the settle ceiling
// and feeds the loop a resident set that alternates between two values, so
// no two consecutive samples agree, and checks it returns the last reading
// rather than looping.
func TestSettledRSS_NeverSettles_ReturnsAtTheCeiling(t *testing.T) {
	previous := settleCeiling
	settleCeiling = 150 * time.Millisecond
	t.Cleanup(func() { settleCeiling = previous })

	self := os.Getpid()
	var toggle atomic.Bool
	var reads atomic.Int64
	s := newSampler(t.Context(), 5*time.Millisecond, func() []int {
		reads.Add(1)
		// One process, then the same process counted twice: the sum doubles
		// on every other sample.
		if toggle.Load() {
			toggle.Store(false)
			return []int{self, self}
		}
		toggle.Store(true)
		return []int{self}
	})
	if _, err := s.current(); err != nil {
		t.Skipf("this platform does not report process statistics: %v", err)
	}

	started := time.Now()
	got := settledRSS(s)
	if got == 0 {
		t.Error("settledRSS returned zero at the ceiling instead of the last reading")
	}
	if elapsed := time.Since(started); elapsed < settleCeiling {
		t.Errorf("settled in %s on a process that never settles", elapsed)
	}
	// Sampled at a pace of one read per 50 ms step: a loop that read as fast
	// as it could would spend the ceiling on reads of a process that has not
	// had time to change.
	if got := reads.Load(); got > 10 {
		t.Errorf("the process was read %d times within a %s ceiling, want one read per 50 ms step", got, settleCeiling)
	}
}

// TestRunScenario_ColdListRefused_FailsTheScenario makes the stand-in refuse
// tools/list, which is the first thing the ramp asks: a scenario whose
// surface cannot be listed has nothing to publish and must say so rather than
// record zeros.
func TestRunScenario_ColdListRefused_FailsTheScenario(t *testing.T) {
	t.Setenv("STANDIN_FAIL", methodToolsList)
	r := standinRunner(t)

	_, err := r.runScenario(t.Context(), httpPlan(1))
	if err == nil || !strings.Contains(err.Error(), "cold tools/list for client 0") {
		t.Errorf("runScenario = %v, want the cold tools/list failure", err)
	}
}

// TestRunScenario_FailedCalls_AreNotedRatherThanFatal makes the stand-in
// refuse tools/call: the load phase records how many calls failed and why,
// keeps the other methods' distributions, and the scenario still completes.
func TestRunScenario_FailedCalls_AreNotedRatherThanFatal(t *testing.T) {
	t.Setenv("STANDIN_FAIL", methodToolsCall)
	r := standinRunner(t)
	plan := httpPlan(1)

	scenario, err := r.runScenario(t.Context(), plan)
	if err != nil {
		t.Fatalf("runScenario: %v", err)
	}
	wantNote := strconv.Itoa(plan.Clients*plan.Parallel) + " tools/call calls failed"
	if len(scenario.Notes) != 1 || !strings.Contains(scenario.Notes[0], wantNote) {
		t.Errorf("notes %v, want one saying %q", scenario.Notes, wantNote)
	}
	if calls, ok := scenario.latency(methodToolsCall); !ok || calls.Count != 0 {
		t.Errorf("tools/call distribution %+v, want an empty one kept in place", calls)
	}
	if list, ok := scenario.latency(methodToolsList); !ok || list.Count != plan.Clients*plan.Parallel {
		t.Errorf("tools/list distribution %+v, want unaffected", list)
	}
	if resources, ok := scenario.latency(methodResourcesList); !ok || resources.Count != plan.Clients*plan.Parallel {
		t.Errorf("resources/list distribution %+v, want unaffected", resources)
	}
}

// TestRamp_OneClient_PublishesNoPerClientCost verifies a scenario with a
// single client reports no per-extra-client figure, rather than one divided by
// the zero extra clients it had.
//
// "What does the next credential cost" is the difference between the first and
// the last divided by the clients in between, and with one client there are
// none: the division is zero by zero, which is a NaN, and a NaN written to the
// record marshals as a value the site's chart code reads as a number. The
// figure a one-client scenario can honestly publish is nothing at all.
//
// The sampler is pointed at no process on purpose. What this asserts is the
// arithmetic the guard performs, and a real process would put an unrepeatable
// resident set on both sides of a subtraction that has to be exactly zero.
func TestRamp_OneClient_PublishesNoPerClientCost(t *testing.T) {
	r := &runner{progress: progressFunc(false)}
	s := newSampler(t.Context(), time.Hour, func() []int { return nil })
	plan := scenarioPlan{
		ID: "http-dynamic", Transport: transportHTTP, Surface: surfaceDynamic,
		Clients: 1, Parallel: 1, Rounds: 1,
	}

	var result Scenario
	if _, err := r.ramp(t.Context(), &fakeTarget{}, s, plan, &result); err != nil {
		t.Fatalf("ramp: %v", err)
	}

	if len(result.Ramp) != 1 || result.Ramp[0].Client != 1 {
		t.Fatalf("ramp = %+v, want the one credential admitted", result.Ramp)
	}
	per := result.Memory.PerExtraClientMiB
	if math.IsNaN(per) || math.IsInf(per, 0) {
		t.Fatalf("PerExtraClientMiB = %v, want a number the record can carry", per)
	}
	if per != 0 {
		t.Errorf("PerExtraClientMiB = %v with one client, want 0", per)
	}
	if result.Memory.AllClientsMiB != result.Memory.OneClientMiB {
		t.Errorf("all clients %v against one client %v, want the same reading for the same single client",
			result.Memory.AllClientsMiB, result.Memory.OneClientMiB)
	}
}

// numberedFailureConn fails every call, with a message naming which call it
// was, so a test can tell the first failure of a method from a later one.
type numberedFailureConn struct{ calls atomic.Int64 }

func (c *numberedFailureConn) call(context.Context, string, map[string]any) ([]byte, error) {
	return nil, fmt.Errorf("failure number %d", c.calls.Add(1))
}

func (c *numberedFailureConn) close() {}

// TestLoad_SeveralRounds_NamesTheFirstFailureNotTheLast verifies the note a
// method's failures produce carries the first one the load phase saw, across
// every round rather than within the last of them.
//
// The note is the only account of why calls failed, and the first failure is
// the one that explains the rest: a server that stopped answering fails the
// same way from then on, and reporting the last one names the aftermath. One
// client at a parallelism of one, so the order of the calls is the order of
// the rounds and the expectation is arithmetic rather than a race.
func TestLoad_SeveralRounds_NamesTheFirstFailureNotTheLast(t *testing.T) {
	r := &runner{progress: progressFunc(false)}
	call, err := callFor(surfaceDynamic)
	if err != nil {
		t.Fatalf("callFor: %v", err)
	}
	conns := []*clientConn{{rpc: &numberedFailureConn{}, label: "client 0"}}
	plan := scenarioPlan{
		ID: "http-dynamic", Transport: transportHTTP, Surface: surfaceDynamic,
		Clients: 1, Parallel: 1, Rounds: 3,
	}

	var result Scenario
	notes := r.load(t.Context(), conns, plan, call, &result)

	if len(notes) != 3 {
		t.Fatalf("notes = %v, want one per method whose calls failed", notes)
	}
	// The methods run in order, three calls each, so the first failure of the
	// first method is call one and of the second is call four.
	wants := []struct {
		note  string
		first string
		last  string
	}{
		{note: notes[0], first: "failure number 1", last: "failure number 3"},
		{note: notes[1], first: "failure number 4", last: "failure number 6"},
	}
	for _, tc := range wants {
		t.Run(tc.first, func(t *testing.T) {
			if !strings.Contains(tc.note, tc.first) {
				t.Errorf("note %q does not name %q", tc.note, tc.first)
			}
			if strings.Contains(tc.note, tc.last) {
				t.Errorf("note %q names %q, which is the last failure rather than the first", tc.note, tc.last)
			}
		})
	}
}

// rampTarget is a target whose every admission changes what there is to
// measure: the Nth client leaves the process holding N hundred MiB, answers
// tools/list with N thousand bytes and took N times ten milliseconds to spawn,
// so every figure the ramp takes from one client says which client it came
// from.
type rampTarget struct {
	fakeTarget
	setRSS func(kib int)
}

func (r *rampTarget) addClient(_ context.Context, index int) (*clientConn, time.Duration, error) {
	n := index + 1
	r.setRSS(n * 100 * 1024)
	return &clientConn{rpc: &sizedConn{size: n * 1000}, label: "client " + strconv.Itoa(index)}, time.Duration(n) * 10 * time.Millisecond, nil
}

// sizedConn answers every call at once with a body of the given length.
type sizedConn struct{ size int }

func (c *sizedConn) call(context.Context, string, map[string]any) ([]byte, error) {
	return make([]byte, c.size), nil
}

func (c *sizedConn) close() {}

// recordedLines keeps what a reporter was asked to print, for a test that
// asserts the wording rather than only that nothing panicked.
type recordedLines struct{ lines []string }

func (r *recordedLines) printf(format string, args ...any) {
	r.lines = append(r.lines, fmt.Sprintf(format, args...))
}

// TestRamp_EachFigureComesFromTheClientItNames verifies the one-client figures
// are the first client's, the all-client figure is the last client's, the cost
// of each extra credential is the difference between the two over the clients
// in between, and the progress line counts clients from one.
//
// Every client leaves a different resident set, answers with a different size
// and took a different time to spawn, so a figure taken from the wrong client,
// or divided by the wrong count, cannot come out right by coincidence.
func TestRamp_EachFigureComesFromTheClientItNames(t *testing.T) {
	const pid = 11
	tgt := &rampTarget{setRSS: fakeProcess(t, pid)}
	progress := &recordedLines{}
	r := &runner{progress: progress.printf}
	s := newSampler(t.Context(), time.Hour, func() []int { return []int{pid} })
	plan := scenarioPlan{
		ID: "stdio-dynamic", Transport: transportStdio, Surface: surfaceDynamic,
		Clients: 3, Parallel: 1, Rounds: 1,
	}

	var result Scenario
	if _, err := r.ramp(t.Context(), tgt, s, plan, &result); err != nil {
		t.Fatalf("ramp: %v", err)
	}

	if result.Startup.ProcessReadyMs != 10 {
		t.Errorf("ProcessReadyMs = %v, want the first client's 10 ms spawn", result.Startup.ProcessReadyMs)
	}
	if result.ListBytes != 1000 {
		t.Errorf("ListBytes = %d, want the first client's 1000-byte listing", result.ListBytes)
	}
	if result.Memory.OneClientMiB != 100 || result.Memory.AllClientsMiB != 300 {
		t.Errorf("one client %v MiB, all clients %v MiB, want 100 after the first and 300 after the last",
			result.Memory.OneClientMiB, result.Memory.AllClientsMiB)
	}
	if result.Memory.PerExtraClientMiB != 100 {
		t.Errorf("PerExtraClientMiB = %v, want (300 - 100) / 2 extra clients = 100", result.Memory.PerExtraClientMiB)
	}
	for i, point := range result.Ramp {
		if point.Client != i+1 || point.RSSMiB != float64(100*(i+1)) {
			t.Errorf("ramp point %d = %+v, want client %d at %d MiB", i, point, i+1, 100*(i+1))
		}
	}
	if len(progress.lines) != 3 || !strings.Contains(progress.lines[0], "client 1/3 admitted") {
		t.Errorf("progress = %q, want one line per client counted from 1/3", progress.lines)
	}
}

// methodDelayConn answers every method at once except the one it is told to
// take its time over.
type methodDelayConn struct {
	slow  string
	delay time.Duration
}

func (c *methodDelayConn) call(_ context.Context, method string, _ map[string]any) ([]byte, error) {
	if method == c.slow {
		time.Sleep(c.delay)
	}
	return []byte(`{}`), nil
}

func (c *methodDelayConn) close() {}

// TestLoad_TheWarmListingIsTheToolsListMedian verifies the warm tools/list
// figure a scenario publishes beside its cold one is the median of the
// tools/list distribution, and that the per-round progress counts rounds from
// one.
//
// Only tools/list is slow here, so the median of any other method would be a
// figure nobody could mistake for it.
func TestLoad_TheWarmListingIsTheToolsListMedian(t *testing.T) {
	progress := &recordedLines{}
	r := &runner{progress: progress.printf}
	call, err := callFor(surfaceDynamic)
	if err != nil {
		t.Fatalf("callFor: %v", err)
	}
	conns := []*clientConn{{rpc: &methodDelayConn{slow: methodToolsList, delay: 30 * time.Millisecond}, label: "client 0"}}
	plan := scenarioPlan{
		ID: "http-dynamic", Transport: transportHTTP, Surface: surfaceDynamic,
		Clients: 1, Parallel: 1, Rounds: 2,
	}

	var result Scenario
	if notes := r.load(t.Context(), conns, plan, call, &result); len(notes) != 0 {
		t.Fatalf("notes = %v, want none from a connection that answers everything", notes)
	}
	list, ok := result.latency(methodToolsList)
	if !ok || list.P50 < 20 || result.Startup.WarmListMs != list.P50 {
		t.Errorf("WarmListMs = %v against a tools/list median of %v, want the one to be the other", result.Startup.WarmListMs, list.P50)
	}
	joined := strings.Join(progress.lines, "\n")
	if !strings.Contains(joined, methodToolsList+" round 1/2") || !strings.Contains(joined, methodToolsList+" round 2/2") || strings.Contains(joined, "round 0/") {
		t.Errorf("progress = %q, want each method's rounds counted 1/2 and 2/2", progress.lines)
	}
}
