// run_test.go covers the parts of a scenario run that can be decided without
// starting a server: what the record is allowed to claim about processor time
// when a sample is missing or goes backwards, which processes the sampler is
// pointed at, and what a health response is read as.
package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
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
// and answers with whatever the test wants.
type countingConn struct {
	calls atomic.Int64
	err   error
}

func (c *countingConn) call(context.Context, string, map[string]any) ([]byte, error) {
	c.calls.Add(1)
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
