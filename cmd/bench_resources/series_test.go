// series_test.go covers the concurrency series: the guard that keeps it from
// taking the host down, the reasons it records for stopping, the batched
// admission, the steady phase, and a walk through the steps against fakes,
// with the whole thing driven against the stand-in binary once.
package main

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestEstimateRSS_FitsALineThroughThePeaks verifies the budget estimate is a
// straight line through the completed steps' peaks, and a proportional
// extrapolation when only one step exists to draw a line from.
//
// The estimate is what stands between the series and a host with no memory
// left, so the arithmetic is pinned exactly: a fit that under-estimated would
// start a step the host cannot hold.
func TestEstimateRSS_FitsALineThroughThePeaks(t *testing.T) {
	step := func(clients int, peak float64) SeriesStep {
		return SeriesStep{Clients: clients, RSSPeakMiB: peak}
	}
	cases := []struct {
		name   string
		steps  []SeriesStep
		next   int
		want   float64
		wantOK bool
	}{
		{name: "nothing to estimate from", next: 2},
		{
			name: "one step extrapolates in proportion", steps: []SeriesStep{step(1, 200)},
			next: 5, want: 1000, wantOK: true,
		},
		{
			name: "two steps fix slope and intercept", steps: []SeriesStep{step(1, 220), step(2, 290)},
			next: 5, want: 500, wantOK: true,
		},
		{
			name:  "least squares over noisy steps",
			steps: []SeriesStep{step(1, 210), step(2, 300), step(5, 480), step(10, 860)},
			// The fit through those four is 71.12 per credential over a
			// 142.45 base, by hand: slope (4*11810 - 18*1850) / (4*130 - 18*18).
			next: 20, want: 1564.898, wantOK: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := estimateRSS(tc.steps, tc.next)
			if ok != tc.wantOK {
				t.Fatalf("estimateRSS ok = %v, want %v", ok, tc.wantOK)
			}
			if round(got) != tc.want {
				t.Errorf("estimateRSS = %v, want %v", round(got), tc.want)
			}
		})
	}
}

// TestBudgetStop_HoldsBackOnlyWhatWouldNotFit verifies the guard is silent
// with no budget or nothing to estimate from, and otherwise stops exactly
// when the estimate exceeds the budget, recording what it estimated.
func TestBudgetStop_HoldsBackOnlyWhatWouldNotFit(t *testing.T) {
	two := []SeriesStep{{Clients: 1, RSSPeakMiB: 220}, {Clients: 2, RSSPeakMiB: 290}}
	cases := []struct {
		name   string
		budget float64
		steps  []SeriesStep
		next   int
		want   *SeriesStop
	}{
		{name: "no budget", budget: 0, steps: two, next: 1000},
		{name: "nothing measured yet", budget: 100, steps: nil, next: 1},
		{name: "fits", budget: 1000, steps: two, next: 5},
		{name: "exactly the budget fits", budget: 500, steps: two, next: 5},
		{
			name: "does not fit", budget: 1000, steps: two, next: 20,
			want: &SeriesStop{Kind: stopBudget, NextClients: 20, EstimateMiB: 1550},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &runner{budgetMiB: tc.budget}
			if got := r.budgetStop(tc.steps, tc.next); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("budgetStop = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestSeriesStop_RecordsSkippedStepsAndTheSentence verifies the two ways a
// series marks its end: before a step, which skips it and everything after,
// and after one, which skips only what follows; and that each kind of stop
// reads as the sentence the record and the terminal carry.
func TestSeriesStop_RecordsSkippedStepsAndTheSentence(t *testing.T) {
	fresh := func() SeriesScenario {
		return SeriesScenario{Clients: []int{1, 2, 5, 10}, BudgetMiB: 800, StoppedAt: 2}
	}

	t.Run("before a step", func(t *testing.T) {
		s := fresh()
		s.stopBefore(2, &SeriesStop{Kind: stopBudget, NextClients: 5, EstimateMiB: 900})
		if !reflect.DeepEqual(s.Skipped, []int{5, 10}) {
			t.Errorf("Skipped = %v, want the step and everything after", s.Skipped)
		}
		want := "stopped at 2 credentials: the next step (5) was estimated at 900 MiB against a budget of 800 MiB"
		if s.StopReason != want {
			t.Errorf("StopReason = %q, want %q", s.StopReason, want)
		}
	})

	t.Run("after a step", func(t *testing.T) {
		s := fresh()
		s.stopAfter(1, &SeriesStop{Kind: stopLatency, P99Ms: 31000})
		if !reflect.DeepEqual(s.Skipped, []int{5, 10}) {
			t.Errorf("Skipped = %v, want everything after the step", s.Skipped)
		}
		if !strings.Contains(s.StopReason, "p99 reached 31000 ms") || !strings.Contains(s.StopReason, "30000 ms ceiling") {
			t.Errorf("StopReason = %q, want the tail and the ceiling named", s.StopReason)
		}
	})

	t.Run("after the last step", func(t *testing.T) {
		s := fresh()
		s.stopAfter(3, &SeriesStop{Kind: stopLatency, P99Ms: 31000})
		if len(s.Skipped) != 0 {
			t.Errorf("Skipped = %v, want nothing after the last step", s.Skipped)
		}
	})

	t.Run("a failure", func(t *testing.T) {
		s := fresh()
		s.stopBefore(3, &SeriesStop{Kind: stopFailure, NextClients: 10, Error: "cold tools/list for client 7: refused"})
		if !strings.Contains(s.StopReason, "next step (10) failed: cold tools/list for client 7: refused") {
			t.Errorf("StopReason = %q, want the failure named", s.StopReason)
		}
	})
}

// TestCPUPerCall_PublishesOnlyWhatWasMeasured verifies the per-call figure
// follows the same honesty rule as the scenario CPU figures: an unanswered
// or backwards sample, or a phase with no completed call, publishes nothing
// and says why.
func TestCPUPerCall_PublishesOnlyWhatWasMeasured(t *testing.T) {
	cases := []struct {
		name       string
		start, end cpuSample
		calls      int
		want       float64
		wantNote   string
	}{
		{name: "measured", start: cpuSample{seconds: 1, ok: true}, end: cpuSample{seconds: 3, ok: true}, calls: 400, want: 5},
		{name: "start unanswered", start: cpuSample{}, end: cpuSample{seconds: 3, ok: true}, calls: 400, wantNote: "did not answer"},
		{name: "end unanswered", start: cpuSample{seconds: 1, ok: true}, end: cpuSample{}, calls: 400, wantNote: "did not answer"},
		{name: "fell between samples", start: cpuSample{seconds: 3, ok: true}, end: cpuSample{seconds: 1, ok: true}, calls: 400, wantNote: "fell between"},
		{name: "no calls", start: cpuSample{seconds: 1, ok: true}, end: cpuSample{seconds: 3, ok: true}, calls: 0, wantNote: "no call completed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, notes := cpuPerCall(tc.start, tc.end, tc.calls)
			if got != tc.want {
				t.Errorf("cpuPerCall = %v, want %v", got, tc.want)
			}
			switch {
			case tc.wantNote == "" && len(notes) != 0:
				t.Errorf("notes = %v, want none", notes)
			case tc.wantNote != "" && (len(notes) != 1 || !strings.Contains(notes[0], tc.wantNote)):
				t.Errorf("notes = %v, want one saying %q", notes, tc.wantNote)
			}
		})
	}
}

// methodConn is a client connection that counts calls per method and fails
// the one method it is told to.
type methodConn struct {
	mu     sync.Mutex
	calls  map[string]int
	fail   string
	closed atomic.Bool
}

func (c *methodConn) call(_ context.Context, method string, _ map[string]any) ([]byte, error) {
	c.mu.Lock()
	if c.calls == nil {
		c.calls = map[string]int{}
	}
	c.calls[method]++
	c.mu.Unlock()
	if method == c.fail {
		return nil, errors.New("refused " + method)
	}
	return []byte(`{"jsonrpc":"2.0","id":1,"result":{}}`), nil
}

func (c *methodConn) close() { c.closed.Store(true) }

// count reports how often a method was called.
func (c *methodConn) count(method string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls[method]
}

// TestSteadyLoad_AlternatesMethodsAndTimesEveryCall verifies the steady
// phase keeps every worker calling until the deadline, alternating the two
// methods, records a duration per completed call and a note per method whose
// calls failed.
func TestSteadyLoad_AlternatesMethodsAndTimesEveryCall(t *testing.T) {
	call, err := callFor(surfaceDynamic)
	if err != nil {
		t.Fatalf("callFor: %v", err)
	}
	healthy := &methodConn{}
	broken := &methodConn{fail: methodToolsList}
	conns := []*clientConn{{rpc: healthy, label: "a"}, {rpc: broken, label: "b"}}

	out := steadyLoad(t.Context(), conns, 2, 60*time.Millisecond, call)

	for _, method := range []string{methodToolsCall, methodToolsList} {
		t.Run(method, func(t *testing.T) {
			if healthy.count(method) == 0 {
				t.Errorf("the healthy connection was never asked for %s", method)
			}
			if len(out.samples[method]) == 0 {
				t.Errorf("no durations recorded for %s", method)
			}
		})
	}
	// Alternation: each worker makes as many of one as of the other, give or
	// take the call it was in the middle of at the deadline.
	if calls, lists := healthy.count(methodToolsCall), healthy.count(methodToolsList); calls < lists || calls > lists+2 {
		t.Errorf("calls %d and lists %d do not alternate", calls, lists)
	}
	if len(out.notes) != 1 || !strings.Contains(out.notes[0], "tools/list calls failed") {
		t.Errorf("notes = %v, want one about the failing tools/list", out.notes)
	}
	if got := broken.count(methodToolsList); got == 0 || strings.Contains(strings.Join(out.notes, " "), strconv.Itoa(got)+" tools/list calls failed") == false {
		t.Errorf("the note %v does not count the %d failures", out.notes, got)
	}
}

// TestSteadyLoad_CancelledContext_EndsEarly verifies a cancelled run stops
// the workers before the deadline instead of letting them spin on errors.
func TestSteadyLoad_CancelledContext_EndsEarly(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	conn := &methodConn{}
	started := time.Now()
	steadyLoad(ctx, []*clientConn{{rpc: conn}}, 1, 5*time.Second, toolCall{Name: "x"})
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Errorf("the phase ran %s on a cancelled context", elapsed)
	}
}

// fakeTarget is a target whose clients answer in memory: admission can be
// made to fail by index, a client's calls can be made to fail, and every
// call can be made to take a moment so a latency has something to measure.
type fakeTarget struct {
	addErr  map[int]error
	callErr map[int]error
	delay   time.Duration
	added   atomic.Int64
}

func (f *fakeTarget) start(context.Context) (time.Duration, error) { return 0, nil }

func (f *fakeTarget) addClient(_ context.Context, index int) (*clientConn, time.Duration, error) {
	if err := f.addErr[index]; err != nil {
		return nil, 0, err
	}
	f.added.Add(1)
	return &clientConn{rpc: &countingConn{err: f.callErr[index], delay: f.delay}, label: "client " + strconv.Itoa(index)}, 0, nil
}

func (f *fakeTarget) processes() []*os.Process { return []*os.Process{{Pid: os.Getpid()}} }
func (f *fakeTarget) goroutines() (int, error) { return 0, errors.New("not a process") }
func (f *fakeTarget) serverInfo() ServerInfo   { return ServerInfo{} }
func (f *fakeTarget) close()                   {}

// TestAdmit_WarmsEveryCredentialAndReportsTheFirstFailure verifies batched
// admission connects every index asked for, warms each with a tools/list,
// returns every connection it made so the caller can close them, and names
// the first failure.
func TestAdmit_WarmsEveryCredentialAndReportsTheFirstFailure(t *testing.T) {
	r := &runner{progress: progressFunc(false)}

	t.Run("all warm", func(t *testing.T) {
		tgt := &fakeTarget{}
		conns, err := r.admit(t.Context(), tgt, 2, 12)
		if err != nil {
			t.Fatalf("admit: %v", err)
		}
		if len(conns) != 10 || tgt.added.Load() != 10 {
			t.Errorf("admitted %d connections (%d added), want 10", len(conns), tgt.added.Load())
		}
		for i, conn := range conns {
			if conn.label != "client "+strconv.Itoa(i+2) {
				t.Errorf("conns[%d] is %q, want the indexes in order", i, conn.label)
			}
		}
	})

	t.Run("a cold list fails", func(t *testing.T) {
		tgt := &fakeTarget{callErr: map[int]error{1: errors.New("refused")}}
		conns, err := r.admit(t.Context(), tgt, 0, 3)
		if err == nil || !strings.Contains(err.Error(), "cold tools/list for client 1") {
			t.Errorf("admit = %v, want the cold list failure naming client 1", err)
		}
		if len(conns) != 3 {
			t.Errorf("admit returned %d connections, want all three so they can be closed", len(conns))
		}
	})

	t.Run("a client cannot be made", func(t *testing.T) {
		tgt := &fakeTarget{addErr: map[int]error{0: errors.New("no socket")}}
		conns, err := r.admit(t.Context(), tgt, 0, 2)
		if err == nil || !strings.Contains(err.Error(), "no socket") {
			t.Errorf("admit = %v, want the admission failure", err)
		}
		if len(conns) != 1 {
			t.Errorf("admit returned %d connections, want the one that was made", len(conns))
		}
	})
}

// seriesFixture is what walkSteps needs besides a target: a sampler over
// this test's own process, a profile client against an in-process listener,
// and a result shaped the way runSeries shapes it.
type seriesFixture struct {
	plan     scenarioPlan
	sampler  *sampler
	profiler *pprofClient
	call     toolCall
	result   SeriesScenario
	conns    []*clientConn
}

// newSeriesFixture builds the fixture for a plan over the given counts, with
// a steady phase short enough for a test.
func newSeriesFixture(t *testing.T, steps []int, budget float64) *seriesFixture {
	t.Helper()
	plan := scenarioPlan{
		ID: "http-dynamic-series", Transport: transportHTTP, Surface: surfaceDynamic,
		Parallel: 1, Steps: steps, StepDuration: 80 * time.Millisecond,
	}
	call, err := callFor(plan.Surface)
	if err != nil {
		t.Fatalf("callFor: %v", err)
	}
	self := os.Getpid()
	s := newSampler(t.Context(), 5*time.Millisecond, func() []int { return []int{self} })
	if _, sampleErr := s.current(); sampleErr != nil {
		t.Skipf("this platform does not report process statistics: %v", sampleErr)
	}
	s.start()
	t.Cleanup(s.stop)
	return &seriesFixture{
		plan: plan, sampler: s, profiler: newPprofTestClient(t), call: call,
		result: SeriesScenario{
			ID: plan.ID, Transport: plan.Transport, Surface: plan.Surface, Parallel: plan.Parallel,
			StepSeconds: round(plan.StepDuration.Seconds()), Clients: plan.Steps, BudgetMiB: budget,
		},
	}
}

// walk runs the steps against a target and returns the result.
func (f *seriesFixture) walk(t *testing.T, r *runner, tgt target) SeriesScenario {
	t.Helper()
	r.walkSteps(t.Context(), f.plan, tgt, f.profiler, f.sampler, f.call, &f.result, &f.conns)
	t.Cleanup(func() { closeConns(f.conns) })
	return f.result
}

// TestWalkSteps_EveryStepRuns_FillsEachOne walks three counts against a
// target that answers everything, with profiles written, and checks each
// step carries every figure the table and the charts read, the pool
// counters the driver knows, and the two profile paths.
func TestWalkSteps_EveryStepRuns_FillsEachOne(t *testing.T) {
	profiles := t.TempDir()
	r := &runner{progress: progressFunc(false), profilesDir: profiles}
	f := newSeriesFixture(t, []int{1, 2, 4}, 0)

	got := f.walk(t, r, &fakeTarget{})

	if len(got.Steps) != 3 || got.StopReason != "" || got.Stop != nil || len(got.Skipped) != 0 {
		t.Fatalf("result %+v, want three steps and no stop", got)
	}
	if got.StoppedAt != 4 {
		t.Errorf("StoppedAt = %d, want the last count", got.StoppedAt)
	}
	for i, step := range got.Steps {
		t.Run(strconv.Itoa(step.Clients), func(t *testing.T) {
			assertSeriesStep(t, f.plan, step, f.plan.Steps[i], profiles)
		})
	}
	if len(f.conns) != 4 {
		t.Errorf("%d connections held at the end, want one per credential", len(f.conns))
	}
}

// assertSeriesStep checks one step carries every figure the table and the
// charts read, the pool counters the driver knows, and its two profiles on
// disk, with no note saying something was unavailable.
func assertSeriesStep(t *testing.T, plan scenarioPlan, step SeriesStep, wantClients int, profiles string) {
	t.Helper()
	if step.Clients != wantClients || step.Pool != (PoolCounters{Entries: step.Clients, Capacity: 4}) {
		t.Errorf("step %+v does not describe count %d in a pool of 4", step, wantClients)
	}
	if step.RSSMeanMiB <= 0 || step.RSSPeakMiB < step.RSSMeanMiB {
		t.Errorf("resident set mean %v peak %v, want both measured with the peak at least the mean", step.RSSMeanMiB, step.RSSPeakMiB)
	}
	if step.Calls == 0 || step.CallP50Ms < 0 || step.ListP50Ms < 0 {
		t.Errorf("latency %+v over %d calls, want a distribution per method", step, step.Calls)
	}
	if step.Goroutines < 1 {
		t.Errorf("goroutines = %d, want the listing's total", step.Goroutines)
	}
	want := StepProfiles{
		CPU:  plan.ID + "/" + strconv.Itoa(step.Clients) + ".cpu.pb.gz",
		Heap: plan.ID + "/" + strconv.Itoa(step.Clients) + ".heap.pb.gz",
	}
	if step.Profiles != want {
		t.Errorf("profiles = %+v, want %+v", step.Profiles, want)
	}
	for _, rel := range []string{step.Profiles.CPU, step.Profiles.Heap} {
		t.Run(rel, func(t *testing.T) {
			if _, statErr := os.Stat(filepath.Join(profiles, filepath.FromSlash(rel))); statErr != nil {
				t.Errorf("profile %s was not written: %v", rel, statErr)
			}
		})
	}
	for _, note := range step.Notes {
		if strings.Contains(note, "unavailable") {
			t.Errorf("note %q on a step where everything answered", note)
		}
	}
}

// TestWalkSteps_NoProfilesDirectory_TakesNone verifies a run that keeps no
// profiles asks the listener for none, and still reads the goroutine count.
func TestWalkSteps_NoProfilesDirectory_TakesNone(t *testing.T) {
	r := &runner{progress: progressFunc(false)}
	f := newSeriesFixture(t, []int{1}, 0)

	got := f.walk(t, r, &fakeTarget{})

	if len(got.Steps) != 1 {
		t.Fatalf("%d steps, want one", len(got.Steps))
	}
	if step := got.Steps[0]; step.Profiles != (StepProfiles{}) || step.Goroutines < 1 {
		t.Errorf("step %+v, want no profile paths and a goroutine count", step)
	}
}

// TestWalkSteps_BudgetStopsBeforeTheStepThatWouldNotFit verifies the budget
// guard holds back a step and everything after it, with the estimate and the
// budget in the reason, and that what ran is kept.
func TestWalkSteps_BudgetStopsBeforeTheStepThatWouldNotFit(t *testing.T) {
	r := &runner{progress: progressFunc(false), budgetMiB: 1}
	f := newSeriesFixture(t, []int{1, 2, 5}, 1)

	got := f.walk(t, r, &fakeTarget{})

	if len(got.Steps) != 1 || got.StoppedAt != 1 {
		t.Fatalf("result %+v, want the first step kept", got)
	}
	if got.Stop == nil || got.Stop.Kind != stopBudget || got.Stop.NextClients != 2 || got.Stop.EstimateMiB <= 1 {
		t.Errorf("Stop = %+v, want a budget stop before 2 credentials", got.Stop)
	}
	if !reflect.DeepEqual(got.Skipped, []int{2, 5}) {
		t.Errorf("Skipped = %v, want [2 5]", got.Skipped)
	}
	if !strings.Contains(got.StopReason, "stopped at 1 credentials") || !strings.Contains(got.StopReason, "against a budget of 1 MiB") {
		t.Errorf("StopReason = %q", got.StopReason)
	}
}

// TestWalkSteps_AdmissionFailure_KeepsWhatRan verifies a credential that
// cannot be admitted ends the series with the steps before it kept and the
// failure named, rather than measuring a pool that is only partly built.
func TestWalkSteps_AdmissionFailure_KeepsWhatRan(t *testing.T) {
	r := &runner{progress: progressFunc(false)}
	f := newSeriesFixture(t, []int{1, 3}, 0)

	got := f.walk(t, r, &fakeTarget{callErr: map[int]error{2: errors.New("refused")}})

	if len(got.Steps) != 1 || got.StoppedAt != 1 {
		t.Fatalf("result %+v, want the first step kept", got)
	}
	if got.Stop == nil || got.Stop.Kind != stopFailure || got.Stop.NextClients != 3 || !strings.Contains(got.Stop.Error, "client 2") {
		t.Errorf("Stop = %+v, want the admission failure before 3 credentials", got.Stop)
	}
	if !reflect.DeepEqual(got.Skipped, []int{3}) {
		t.Errorf("Skipped = %v, want [3]", got.Skipped)
	}
	if len(f.conns) != 3 {
		t.Errorf("%d connections held, want every one made so they are all closed", len(f.conns))
	}
}

// TestWalkSteps_LatencyCeiling_MakesTheStepTheLast lowers the ceiling to
// nothing, so the first step's tail is over it, and checks the series ends
// after that step with the tail in the reason.
func TestWalkSteps_LatencyCeiling_MakesTheStepTheLast(t *testing.T) {
	previous := latencyCeiling
	latencyCeiling = 0
	t.Cleanup(func() { latencyCeiling = previous })

	r := &runner{progress: progressFunc(false)}
	f := newSeriesFixture(t, []int{1, 2}, 0)

	// A call that takes a millisecond, so the tail is above a ceiling of
	// nothing; an instantaneous fake rounds to a p99 of zero.
	got := f.walk(t, r, &fakeTarget{delay: time.Millisecond})

	if len(got.Steps) != 1 || got.StoppedAt != 1 {
		t.Fatalf("result %+v, want the step that crossed the ceiling kept as the last", got)
	}
	if got.Stop == nil || got.Stop.Kind != stopLatency || got.Stop.P99Ms != got.Steps[0].CallP99Ms {
		t.Errorf("Stop = %+v, want a latency stop carrying the step's p99", got.Stop)
	}
	if !reflect.DeepEqual(got.Skipped, []int{2}) {
		t.Errorf("Skipped = %v, want [2]", got.Skipped)
	}
}

// TestRunStep_ListenerGone_NotesWhatItCouldNotRead verifies a step whose
// profile listener does not answer still publishes its load figures, with a
// note per thing it could not read, since the memory and latency of a step
// are worth keeping even when the profiles are not.
func TestRunStep_ListenerGone_NotesWhatItCouldNotRead(t *testing.T) {
	r := &runner{progress: progressFunc(false), profilesDir: t.TempDir()}
	f := newSeriesFixture(t, []int{1}, 0)
	f.profiler = newPprofClient("http://127.0.0.1:1")

	step := r.runStep(t.Context(), stepInput{
		plan: f.plan, call: f.call, conns: []*clientConn{{rpc: &countingConn{}}},
		sampler: f.sampler, profiler: f.profiler, capacity: 1,
	})

	if step.Calls == 0 || step.RSSPeakMiB <= 0 {
		t.Errorf("step %+v lost its load figures", step)
	}
	if step.Goroutines != 0 || step.Profiles != (StepProfiles{}) {
		t.Errorf("step %+v claims what the listener never answered", step)
	}
	joined := strings.Join(step.Notes, "\n")
	for _, want := range []string{"goroutine count unavailable", "cpu profile unavailable", "heap profile unavailable"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(joined, want) {
				t.Errorf("notes %v do not say %q", step.Notes, want)
			}
		})
	}
}

// TestSeriesSummaries_NameWhatHappened verifies the lines a run prints for
// a step and for a series carry the figures and the stop.
func TestSeriesSummaries_NameWhatHappened(t *testing.T) {
	step := SeriesStep{
		Clients: 50, RSSMeanMiB: 3000, RSSPeakMiB: 3200, CPUMsPerCall: 1.25, Calls: 4000,
		CallP50Ms: 12, CallP99Ms: 40, ListP50Ms: 5, ListP99Ms: 9, Goroutines: 120,
	}
	for _, want := range []string{"50 credentials", "3000 MiB mean", "3200 MiB peak", "1.250 ms/call", "4000 calls", "p50 12 ms p99 40 ms", "120 goroutines"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(step.summary(), want) {
				t.Errorf("step summary %q lacks %q", step.summary(), want)
			}
		})
	}
	complete := SeriesScenario{Steps: []SeriesStep{{}, {}}, StoppedAt: 2}
	if got := complete.summary(3 * time.Second); !strings.Contains(got, "2 steps to 2 credentials") {
		t.Errorf("series summary = %q", got)
	}
	stopped := SeriesScenario{Steps: []SeriesStep{{}}, StoppedAt: 1, StopReason: "stopped at 1 credentials: because"}
	if got := stopped.summary(3 * time.Second); !strings.Contains(got, "1 steps, stopped at 1 credentials: because") {
		t.Errorf("series summary = %q", got)
	}
}

// TestRunSeries_Standin_StepsThroughTheCountsAndProfiles drives the whole
// series against the stand-in binary the way measure does: a real process
// started with --pprof-addr and a pool sized to the largest step, three
// counts, profiles on disk under the scenario's directory, and the build
// read off /health.
func TestRunSeries_Standin_StepsThroughTheCountsAndProfiles(t *testing.T) {
	r := standinRunner(t)
	profiles := t.TempDir()
	r.profilesDir = profiles
	var lines []string
	r.report = func(format string, args ...any) { lines = append(lines, format) }
	plan := seriesPlan(surfaceDynamic, 2, matrixSettings{steps: []int{1, 2, 3}, stepDuration: time.Second})

	series, err := r.runSeries(t.Context(), plan)
	if err != nil {
		t.Fatalf("runSeries: %v", err)
	}
	if series.ID != plan.ID || !reflect.DeepEqual(series.Clients, plan.Steps) || series.StepSeconds != 1 || series.Parallel != 2 {
		t.Errorf("series header %+v does not describe the plan", series)
	}
	if len(series.Steps) != 3 || series.StoppedAt != 3 || series.Stop != nil {
		t.Fatalf("series %+v, want every step run", series)
	}
	for _, step := range series.Steps {
		t.Run(strconv.Itoa(step.Clients), func(t *testing.T) {
			if step.Calls == 0 || step.RSSPeakMiB <= 0 || step.Goroutines < 1 {
				t.Errorf("step %+v is missing a figure", step)
			}
			if step.Pool.Capacity != 3 {
				t.Errorf("pool capacity %d, want the largest step", step.Pool.Capacity)
			}
			if _, statErr := os.Stat(filepath.Join(profiles, plan.ID, strconv.Itoa(step.Clients)+".cpu.pb.gz")); statErr != nil {
				t.Errorf("no CPU profile for %d credentials: %v", step.Clients, statErr)
			}
		})
	}
	if r.serverInfo.Version != "standin" {
		t.Errorf("the runner kept server info %+v, want what /health reported", r.serverInfo)
	}
	if len(lines) != 3 {
		t.Errorf("%d lines reported, want one per step", len(lines))
	}
	if series.BudgetMiB != 0 || len(series.Notes) != 1 || !strings.Contains(series.Notes[0], "no memory budget") {
		t.Errorf("a series with no budget must say so: budget %v notes %v", series.BudgetMiB, series.Notes)
	}
}

// TestRunSeries_Refusals covers the ways a series fails before it has
// anything to publish: a surface with no tool call, a binary that cannot
// start, and a stand-in that refuses every cold tools/list so that no step
// completes.
func TestRunSeries_Refusals(t *testing.T) {
	t.Run("unknown surface", func(t *testing.T) {
		r := standinRunner(t)
		plan := seriesPlan("nonsense", 1, matrixSettings{steps: []int{1}, stepDuration: time.Second})
		if _, err := r.runSeries(t.Context(), plan); err == nil {
			t.Error("runSeries accepted a surface with no tool call")
		}
	})
	t.Run("no port for the profile listener", func(t *testing.T) {
		previous := reservePort
		reservePort = func(context.Context) (net.Listener, error) { return nil, errors.New("no ports") }
		t.Cleanup(func() { reservePort = previous })
		r := standinRunner(t)
		plan := seriesPlan(surfaceDynamic, 1, matrixSettings{steps: []int{1}, stepDuration: time.Second})
		if _, err := r.runSeries(t.Context(), plan); err == nil || !strings.Contains(err.Error(), "reserve a port") {
			t.Errorf("runSeries = %v, want the reservation failure", err)
		}
	})
	t.Run("binary cannot start", func(t *testing.T) {
		r := standinRunner(t)
		r.binary = filepath.Join(t.TempDir(), "absent")
		plan := seriesPlan(surfaceDynamic, 1, matrixSettings{steps: []int{1}, stepDuration: time.Second})
		if _, err := r.runSeries(t.Context(), plan); err == nil || !strings.Contains(err.Error(), "start server") {
			t.Errorf("runSeries = %v, want the exec failure", err)
		}
	})
	t.Run("no step completes", func(t *testing.T) {
		t.Setenv("STANDIN_FAIL", methodToolsList)
		r := standinRunner(t)
		plan := seriesPlan(surfaceDynamic, 1, matrixSettings{steps: []int{1, 2}, stepDuration: time.Second})
		_, err := r.runSeries(t.Context(), plan)
		if err == nil || !strings.Contains(err.Error(), "no step completed") || !strings.Contains(err.Error(), "cold tools/list for client 0") {
			t.Errorf("runSeries = %v, want the refusal naming the failed warm-up", err)
		}
	})
}
