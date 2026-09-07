package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// scriptedConn answers whatever the test tells it to, after whatever delay,
// counting what it was asked for.
type scriptedConn struct {
	delay  time.Duration
	answer func(method string, seen int) error
	calls  atomic.Int64
}

func (c *scriptedConn) call(ctx context.Context, method string, _ map[string]any) ([]byte, error) {
	seen := int(c.calls.Add(1))
	if c.delay > 0 {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("%s: %w", method, ctx.Err())
		case <-time.After(c.delay):
		}
	}
	if c.answer != nil {
		if err := c.answer(method, seen); err != nil {
			return nil, err
		}
	}
	return []byte(`{"jsonrpc":"2.0","id":1,"result":{}}`), nil
}

func (c *scriptedConn) close() {}

// refusal is the wire shape a refused tools/call comes back in, built the way
// the server builds it.
func refusalOf(tool string) error {
	return fmt.Errorf("tools/call: %w", &toolResultError{
		Text: toolutil.RateLimitRefusalPrefix + tool + "; retry after a short backoff",
	})
}

// twoPopulationPlan is a plan small enough for a test: one credential each
// side, a handful of ticks, and the shipped bucket's refusal shapes.
func twoPopulationPlan(t *testing.T) fairnessPlan {
	t.Helper()
	bound, err := boundByID("tools-call-rps")
	if err != nil {
		t.Fatalf("boundByID: %v", err)
	}
	return fairnessPlan{
		ID: "test", Surface: surfaceDynamic, Bound: bound,
		Quiet: populationSpec{Name: populationQuiet, Credentials: 1, Rate: 100, Verbs: []string{verbCall, verbList}},
		Noisy: populationSpec{Name: populationNoisy, Credentials: 1, Rate: 100, Verbs: []string{verbCall}},
		Phase: 40 * time.Millisecond, LeadIn: 0, Deadline: time.Second, Repeats: 1,
	}
}

// TestDrive_KeepsServedAndRefusedApartPerPopulation verifies the driver files
// each population's requests under its own name and each request under its own
// outcome, and that a refusal never lands in the served distribution.
//
// This is the trap the whole scenario exists to avoid: a refusal is cheap, so
// pooling it with the served calls would make any bound look like an
// improvement the more it refused.
func TestDrive_KeepsServedAndRefusedApartPerPopulation(t *testing.T) {
	plan := twoPopulationPlan(t)
	call, err := callFor(plan.Surface)
	if err != nil {
		t.Fatalf("callFor: %v", err)
	}
	quiet := &scriptedConn{}
	noisy := &scriptedConn{answer: func(method string, seen int) error {
		if method == methodToolsCall && seen > 1 {
			return refusalOf(call.Name)
		}
		return nil
	}}
	tally := newFairTally(call, plan.Bound.Refusals)
	drive(t.Context(), plan, []*clientConn{{rpc: quiet, label: "q"}, {rpc: noisy, label: "n"}}, call, plan.ticks, tally)

	populations := tally.populations(plan)
	if len(populations) != 2 || populations[0].Name != populationQuiet {
		t.Fatalf("populations = %+v, want the quiet one first", populations)
	}
	quietCall, ok := populations[0].method(methodToolsCall)
	if !ok {
		t.Fatal("the quiet population issued no tools/call")
	}
	if quietCall.Refused != 0 || quietCall.Served == 0 {
		t.Errorf("quiet tools/call = %+v, want it served and never refused", quietCall)
	}
	if _, listed := populations[0].method(methodToolsList); !listed {
		t.Error("the quiet population's second verb was never issued")
	}
	noisyCall, ok := populations[1].method(methodToolsCall)
	if !ok {
		t.Fatal("the noisy population issued no tools/call")
	}
	if noisyCall.Refused == 0 {
		t.Fatalf("noisy tools/call = %+v, want the refusals recorded", noisyCall)
	}
	if noisyCall.ServedLatency.Count != noisyCall.Served || noisyCall.RefusedLatency.Count != noisyCall.Refused {
		t.Errorf("noisy tools/call = %+v, want each distribution to hold its own outcome", noisyCall)
	}
	if got, want := noisyCall.Dispatched, noisyCall.Served+noisyCall.Refused+noisyCall.Failed+noisyCall.TimedOut; got != want {
		t.Errorf("dispatched %d does not equal the %d outcomes recorded", got, want)
	}
	if noisyCall.Intended != noisyCall.Dispatched+noisyCall.Dropped {
		t.Errorf("intended %d is not dispatched %d plus dropped %d",
			noisyCall.Intended, noisyCall.Dispatched, noisyCall.Dropped)
	}
}

// TestDrive_MeasuresLatencyFromTheIntendedInstant verifies a request held up
// before it was sent is charged the waiting.
//
// This is the coordinated-omission fix. A driver that timed from the send
// would report a queued request as fast, which is exactly the queueing a quiet
// tenant suffers behind a noisy one and exactly what this scenario is looking
// for.
func TestDrive_MeasuresLatencyFromTheIntendedInstant(t *testing.T) {
	call, err := callFor(surfaceDynamic)
	if err != nil {
		t.Fatalf("callFor: %v", err)
	}
	tally := newFairTally(call, nil)
	verb := verbs[verbCall]
	conn := &scriptedConn{}
	// An instant already thirty milliseconds in the past, which is what a
	// driver that fell behind hands to its own request.
	intended := time.Now().Add(-30 * time.Millisecond)
	issue(t.Context(), paceInput{conn: &clientConn{rpc: conn}, pop: populationQuiet, call: call, deadline: time.Second, tally: tally},
		verb, intended)

	entry := tally.methods[populationQuiet][methodToolsCall]
	if len(entry.served) != 1 {
		t.Fatalf("served %d requests, want one", len(entry.served))
	}
	if entry.served[0] < 30*time.Millisecond {
		t.Errorf("latency %s, want it to include the %s the request waited to be sent", entry.served[0], 30*time.Millisecond)
	}
	if len(entry.lateness) != 1 || entry.lateness[0] < 30*time.Millisecond {
		t.Errorf("lateness %v, want the driver's own delay recorded beside it", entry.lateness)
	}
}

// TestIssue_GivesUpWhenAClientWould verifies the per-request deadline is
// anchored at the intended instant, so a request the driver dispatched late
// does not get a fresh deadline the moment it leaves.
//
// A request published as served at tens of seconds would report starvation as
// slowness, which is the reading the quiet population's figures exist to
// prevent.
func TestIssue_GivesUpWhenAClientWould(t *testing.T) {
	call, err := callFor(surfaceDynamic)
	if err != nil {
		t.Fatalf("callFor: %v", err)
	}
	tally := newFairTally(call, nil)
	conn := &scriptedConn{delay: time.Minute}
	issue(t.Context(), paceInput{conn: &clientConn{rpc: conn}, pop: populationQuiet, call: call, deadline: 20 * time.Millisecond, tally: tally},
		verbs[verbCall], time.Now())

	entry := tally.methods[populationQuiet][methodToolsCall]
	if entry.counts[outcomeTimedOut] != 1 {
		t.Errorf("outcomes = %v, want the request counted as timed out", entry.counts)
	}
	if len(entry.served) != 0 {
		t.Errorf("served %v, want a request the client gave up on to be in no latency distribution", entry.served)
	}
}

// TestPaceCredential_RecordsTheTicksItCouldNotSend verifies a tick that finds
// no free slot is counted rather than deferred.
//
// Deferring it would turn the open loop back into a closed one, and skipping it
// silently would let an arm offer less than it claimed while the record said
// both arms offered the same.
func TestPaceCredential_RecordsTheTicksItCouldNotSend(t *testing.T) {
	call, err := callFor(surfaceDynamic)
	if err != nil {
		t.Fatalf("callFor: %v", err)
	}
	tally := newFairTally(call, nil)
	conn := &scriptedConn{delay: 200 * time.Millisecond}
	paceCredential(t.Context(), paceInput{
		conn: &clientConn{rpc: conn}, pop: populationNoisy, start: time.Now(),
		period: time.Millisecond, ticks: 5, verbs: []string{verbCall}, call: call,
		deadline: time.Second, inFlight: 1, tally: tally,
	})
	entry := tally.methods[populationNoisy][methodToolsCall]
	if entry.dropped == 0 {
		t.Errorf("dropped = %d, want the ticks that found no slot counted", entry.dropped)
	}
	if entry.intended != 5 {
		t.Errorf("intended = %d, want every tick the schedule reached", entry.intended)
	}
	if entry.intended != entry.dropped+len(entry.lateness) {
		t.Errorf("intended %d is not dropped %d plus dispatched %d", entry.intended, entry.dropped, len(entry.lateness))
	}
}

// TestPaceCredential_StopsWhenTheRunIsCancelled verifies a cancelled run ends
// the schedule instead of spinning through it on errors.
func TestPaceCredential_StopsWhenTheRunIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	call, err := callFor(surfaceDynamic)
	if err != nil {
		t.Fatalf("callFor: %v", err)
	}
	tally := newFairTally(call, nil)
	started := time.Now()
	paceCredential(ctx, paceInput{
		conn: &clientConn{rpc: &scriptedConn{}}, pop: populationQuiet, start: time.Now(),
		period: time.Second, ticks: 100, verbs: []string{verbCall}, call: call,
		deadline: time.Second, inFlight: 4, tally: tally,
	})
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Errorf("the schedule ran %s on a cancelled context", elapsed)
	}
	if len(tally.methods) != 0 {
		t.Errorf("tally = %v, want nothing recorded for a schedule that never ran", tally.methods)
	}
}

// TestWaitUntil_AnswersForAnInstantAlreadyPast verifies a driver that fell
// behind catches up rather than stretching the schedule, and that a cancelled
// run says so either way.
func TestWaitUntil_AnswersForAnInstantAlreadyPast(t *testing.T) {
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	cases := []struct {
		name string
		ctx  context.Context
		when time.Time
		want bool
	}{
		{name: "an instant already past", ctx: t.Context(), when: time.Now().Add(-time.Second), want: true},
		{name: "an instant past on a cancelled run", ctx: cancelled, when: time.Now().Add(-time.Second), want: false},
		{name: "an instant to come", ctx: t.Context(), when: time.Now().Add(5 * time.Millisecond), want: true},
		{name: "an instant to come on a cancelled run", ctx: cancelled, when: time.Now().Add(time.Minute), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := waitUntil(tc.ctx, tc.when); got != tc.want {
				t.Errorf("waitUntil = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestFairTally_Record_FilesTheFirstFailureAndNothingAfterIt verifies a
// failure is named once and that the outcomes stay four separate counters.
func TestFairTally_Record_FilesTheFirstFailureAndNothingAfterIt(t *testing.T) {
	call, err := callFor(surfaceDynamic)
	if err != nil {
		t.Fatalf("callFor: %v", err)
	}
	tally := newFairTally(call, nil)
	verb := verbs[verbCall]
	tally.record(populationQuiet, verb, observation{err: errors.New("first")})
	tally.record(populationQuiet, verb, observation{err: errors.New("second")})
	tally.record(populationQuiet, verb, observation{latency: time.Millisecond})

	entry := tally.methods[populationQuiet][methodToolsCall]
	if entry.firstFailure == nil || entry.firstFailure.Error() != "first" {
		t.Errorf("firstFailure = %v, want the first one", entry.firstFailure)
	}
	rendered := entry.render(verb)
	if rendered.Failed != 2 || rendered.Served != 1 || rendered.FirstFailure != "first" {
		t.Errorf("rendered = %+v, want two failures, one served and the first failure named", rendered)
	}
}

// TestFairTally_Populations_LeavesOutAMethodNothingIssued verifies a verb the
// window never reached does not appear as a method with no requests behind it.
func TestFairTally_Populations_LeavesOutAMethodNothingIssued(t *testing.T) {
	plan := twoPopulationPlan(t)
	call, err := callFor(plan.Surface)
	if err != nil {
		t.Fatalf("callFor: %v", err)
	}
	tally := newFairTally(call, nil)
	tally.offered(populationQuiet, verbs[verbCall])
	populations := tally.populations(plan)
	if len(populations[0].Methods) != 1 || populations[0].Methods[0].Method != methodToolsCall {
		t.Errorf("methods = %+v, want only the one that was issued", populations[0].Methods)
	}
	if len(populations[1].Methods) != 0 {
		t.Errorf("noisy methods = %+v, want none", populations[1].Methods)
	}
}

// TestFairnessProcess_PublishesProcessorTimePerServedRequestOnly verifies the
// denominator is the served count and that an unreadable sample publishes
// nothing rather than a difference it cannot support.
//
// A per-request figure would fall by an order of magnitude the moment a bound
// refused anything, since a refusal costs microseconds against tens of
// milliseconds served, and the report would present not doing the work as
// doing it cheaply.
func TestFairnessProcess_PublishesProcessorTimePerServedRequestOnly(t *testing.T) {
	ok := func(seconds float64) cpuSample { return cpuSample{seconds: seconds, ok: true} }
	cases := []struct {
		name       string
		in         processInput
		wantPer    float64
		wantCores  float64
		wantNoteIn string
	}{
		{
			name: "a served phase",
			in: processInput{
				serverStart: ok(1), serverEnd: ok(3), driverStart: ok(0), driverEnd: ok(1),
				wall: 4 * time.Second, served: 100, peakRSS: 2 * 1024 * 1024,
			},
			wantPer: 20, wantCores: 0.5,
		},
		{
			name: "a phase that served nothing",
			in: processInput{
				serverStart: ok(1), serverEnd: ok(3), driverStart: ok(0), driverEnd: ok(1),
				wall: 4 * time.Second,
			},
			wantCores: 0.5, wantNoteIn: "nothing was served",
		},
		{
			name: "a platform that would not answer",
			in: processInput{
				serverStart: cpuSample{}, serverEnd: ok(3), driverStart: ok(0), driverEnd: ok(1),
				wall: 4 * time.Second, served: 10,
			},
			wantNoteIn: "did not answer a sample",
		},
		{
			name: "consumed time that fell between samples",
			in: processInput{
				serverStart: ok(3), serverEnd: ok(1), driverStart: ok(1), driverEnd: ok(0),
				wall: 4 * time.Second, served: 10,
			},
			wantNoteIn: "fell between samples",
		},
		{
			name: "a phase with no wall time",
			in: processInput{
				serverStart: ok(1), serverEnd: ok(3), driverStart: ok(0), driverEnd: ok(1), served: 10,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, notes := fairnessProcess(tc.in)
			if got.CPUMsPerServed != tc.wantPer {
				t.Errorf("cpu ms per served = %v, want %v", got.CPUMsPerServed, tc.wantPer)
			}
			if got.CoresBusy != tc.wantCores {
				t.Errorf("cores busy = %v, want %v", got.CoresBusy, tc.wantCores)
			}
			if tc.wantNoteIn != "" && !strings.Contains(strings.Join(notes, "; "), tc.wantNoteIn) {
				t.Errorf("notes = %v, want one about %q", notes, tc.wantNoteIn)
			}
			if tc.wantNoteIn == "" && len(notes) != 0 {
				t.Errorf("notes = %v, want none", notes)
			}
		})
	}
}

// TestCheckArm_RefusesAnArmThatWasNotWhatItClaimed verifies the two controls.
//
// The positive control is the important one: a bound absent from the build, a
// mistyped switch and two populations sharing a bucket all produce "no
// refusals", and reporting that as a bound that helped nobody would publish a
// verdict on a bound that was never there.
func TestCheckArm_RefusesAnArmThatWasNotWhatItClaimed(t *testing.T) {
	plan := twoPopulationPlan(t)
	withNoisy := func(arm string, method FairnessMethod) FairnessArm {
		return FairnessArm{Arm: arm, Populations: []FairnessPopulation{{Name: populationNoisy, Methods: []FairnessMethod{method}}}}
	}
	served := FairnessMethod{Method: methodToolsCall, Intended: 10, Dispatched: 10, Served: 10}
	refused := FairnessMethod{Method: methodToolsCall, Intended: 10, Dispatched: 10, Served: 4, Refused: 6}

	t.Run("the bound was in force and refused nothing", func(t *testing.T) {
		_, err := checkArm(plan, withNoisy(armOn, served))
		if !errors.Is(err, errBoundDidNotFire) {
			t.Errorf("checkArm = %v, want the positive control to fail", err)
		}
	})
	t.Run("the bound was in force and refused", func(t *testing.T) {
		notes, err := checkArm(plan, withNoisy(armOn, refused))
		if err != nil || len(notes) != 0 {
			t.Errorf("checkArm = %v, %v, want a clean arm", notes, err)
		}
	})
	// The plan above offers ninety metered requests a second above the bound
	// over a forty-millisecond phase, so its own arithmetic expects between
	// three and four refusals and the control demands half of that.
	t.Run("the bound was in force and fired once in a thousand", func(t *testing.T) {
		token := FairnessMethod{Method: methodToolsCall, Intended: 1000, Dispatched: 1000, Served: 999, Refused: 1}
		_, err := checkArm(plan, withNoisy(armOn, token))
		if !errors.Is(err, errBoundDidNotFire) {
			t.Errorf("checkArm = %v, want a single refusal in a thousand refused as a bound that did not fire", err)
		}
	})
	t.Run("the bound was off and something refused anyway", func(t *testing.T) {
		notes, err := checkArm(plan, withNoisy(armOff, refused))
		if err != nil || len(notes) != 1 || !strings.Contains(notes[0], "other than the bound under test") {
			t.Errorf("checkArm = %v, %v, want a note about the unexplained refusals", notes, err)
		}
	})
	t.Run("a schedule that did not run to its end", func(t *testing.T) {
		short := FairnessMethod{Method: methodToolsCall, Intended: 10, Dispatched: 4, Served: 4}
		notes, _ := checkArm(plan, withNoisy(armOff, short))
		if len(notes) != 1 || !strings.Contains(notes[0], "did not run to its end") {
			t.Errorf("notes = %v, want one about the schedule", notes)
		}
	})
	t.Run("requests that failed for a reason that is not the bound", func(t *testing.T) {
		broken := FairnessMethod{Method: methodToolsCall, Intended: 10, Dispatched: 10, Served: 8, Failed: 2, FirstFailure: "connection reset"}
		notes, _ := checkArm(plan, withNoisy(armOff, broken))
		if len(notes) != 1 || !strings.Contains(notes[0], "connection reset") {
			t.Errorf("notes = %v, want one naming the failure", notes)
		}
	})
}

// TestIssue_FilesATickItWasTooLateToSendAsOneItNeverSent verifies a request
// the driver reached after its own deadline is counted as the harness failing
// to run the schedule, not as the tenant giving up on the server.
//
// The deadline is anchored at the intended instant, so a driver that far
// behind enters the call with an expired context: the transport returns a
// deadline without a byte reaching the server. Filed under the timed-out
// outcome, as it was, that is a harness failure wearing the name of
// starvation, and it lands in the very count the check for genuine starvation
// compares between the arms.
func TestIssue_FilesATickItWasTooLateToSendAsOneItNeverSent(t *testing.T) {
	call, err := callFor(surfaceDynamic)
	if err != nil {
		t.Fatalf("callFor: %v", err)
	}
	var asked atomic.Int64
	conn := &scriptedConn{answer: func(string, int) error { asked.Add(1); return nil }}
	tally := newFairTally(call, nil)
	in := paceInput{
		conn: &clientConn{rpc: conn, label: "q"}, pop: populationQuiet,
		deadline: 50 * time.Millisecond, tally: tally, call: call,
	}
	tally.offered(in.pop, verbs[verbCall])
	issue(t.Context(), in, verbs[verbCall], time.Now().Add(-2*time.Second))

	entry := tally.methods[populationQuiet][methodToolsCall]
	if got := entry.render(verbs[verbCall]); got.Dropped != 1 || got.Dispatched != 0 || got.TimedOut != 0 {
		t.Errorf("method = %+v, want the tick recorded as one the driver never sent", got)
	}
	if got := asked.Load(); got != 0 {
		t.Errorf("the server was asked %d times for a request whose deadline had already passed", got)
	}
	t.Run("and one still inside its deadline is sent", func(t *testing.T) {
		inTime := newFairTally(call, nil)
		in.tally = inTime
		inTime.offered(in.pop, verbs[verbCall])
		issue(t.Context(), in, verbs[verbCall], time.Now())
		if got := inTime.methods[populationQuiet][methodToolsCall].render(verbs[verbCall]); got.Served != 1 || got.Dropped != 0 {
			t.Errorf("method = %+v, want the request sent and served", got)
		}
	})
}

// TestNewHarness_ReleasesTheBinaryItBuilt verifies the cleanup removes a build
// the harness made and leaves alone a binary it was given.
func TestNewHarness_ReleasesTheBinaryItBuilt(t *testing.T) {
	t.Run("a build of its own", func(t *testing.T) {
		dir := t.TempDir()
		built := filepath.Join(dir, "inner", "server")
		if err := os.MkdirAll(filepath.Dir(built), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(built, nil, 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		original := buildServerBinary
		t.Cleanup(func() { buildServerBinary = original })
		buildServerBinary = func(string) (string, error) { return built, nil }

		r, cleanup, err := newHarness(options{sampleInterval: time.Second}, dir)
		if err != nil {
			t.Fatalf("newHarness: %v", err)
		}
		if r.binary != built {
			t.Errorf("binary = %q, want the one it built", r.binary)
		}
		cleanup()
		if _, statErr := os.Stat(filepath.Dir(built)); !os.IsNotExist(statErr) {
			t.Errorf("the build directory survived cleanup: %v", statErr)
		}
	})
	t.Run("a build that failed", func(t *testing.T) {
		original := buildServerBinary
		t.Cleanup(func() { buildServerBinary = original })
		buildServerBinary = func(string) (string, error) { return "", errors.New("no toolchain") }
		if _, _, err := newHarness(options{}, t.TempDir()); err == nil {
			t.Error("newHarness accepted a build that failed")
		}
	})
	t.Run("a binary it was given", func(t *testing.T) {
		dir := t.TempDir()
		given := filepath.Join(dir, "server")
		if err := os.WriteFile(given, nil, 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		r, cleanup, err := newHarness(options{binary: given, profiles: "bench/profiles"}, dir)
		if err != nil {
			t.Fatalf("newHarness: %v", err)
		}
		cleanup()
		if r.profilesDir != filepath.Join(dir, "bench", "profiles") {
			t.Errorf("profilesDir = %q, want it resolved against the root", r.profilesDir)
		}
		if _, statErr := os.Stat(given); statErr != nil {
			t.Errorf("cleanup removed a binary it did not build: %v", statErr)
		}
	})
}

// TestRunFairness_MeasuresBothArmsAndWritesItsOwnDocument drives the whole mode
// against the stand-in: two arms started with different switches, a document
// written where it was asked for, and a verdict that does not overclaim from
// one repetition.
func TestRunFairness_MeasuresBothArmsAndWritesItsOwnDocument(t *testing.T) {
	binary := standinBinary(t)
	root := t.TempDir()
	opts := smallFairnessOptions(binary, filepath.Join(root, "out", "fairness.json"))

	if err := runFairness(opts, root); err != nil {
		t.Fatalf("runFairness: %v", err)
	}
	doc, err := readFairness(opts.fairnessJSON)
	if err != nil {
		t.Fatalf("readFairness: %v", err)
	}
	if len(doc.Repeats) != 1 || len(doc.Repeats[0].Arms) != 2 {
		t.Fatalf("repeats = %+v, want one repetition of two arms", doc.Repeats)
	}
	off, ok := doc.Repeats[0].arm(armOff)
	if !ok {
		t.Fatal("the document carries no arm with the bound off")
	}
	on, ok := doc.Repeats[0].arm(armOn)
	if !ok {
		t.Fatal("the document carries no arm with the bound on")
	}
	if off.refusedAnything() {
		t.Errorf("the arm with the bound off recorded refusals: %s", off.summary())
	}
	if !on.refusedAnything() {
		t.Errorf("the arm with the bound on refused nothing: %s", on.summary())
	}
	quiet, ok := on.population(populationQuiet)
	if !ok {
		t.Fatal("the arm with the bound on carries no quiet population")
	}
	for _, method := range quiet.Methods {
		t.Run("the quiet tenant kept "+method.Method, func(t *testing.T) {
			if method.Refused != 0 {
				t.Errorf("%s = %+v, want a quiet tenant well under the bound never refused", method.Method, method)
			}
		})
	}
	if doc.Verdict.Direction != directionIndistinguishable {
		t.Errorf("verdict = %+v, want one repetition to decide nothing", doc.Verdict)
	}
	if doc.Server.Version == "" {
		t.Error("the document does not say which build it measured")
	}
	assertMeasuredThePhaseAlone(t, opts, doc)
}

// assertMeasuredThePhaseAlone verifies each arm's counts are the phase's
// schedule and nothing else, which is how the lead-in stays out of the
// measurement.
//
// This is the accounting the whole scenario rests on and nothing asserted it:
// the lead-in exists to drain the bound's burst, so folding it into the phase's
// tally puts the burst's free requests inside the measured window and
// understates the refusals against the requests served. The plan's arithmetic
// is deterministic, so the intended count is exactly what a window of the
// phase's length offers and a window carrying the lead-in as well cannot match
// it.
func assertMeasuredThePhaseAlone(t *testing.T, opts options, doc *FairnessDoc) {
	t.Helper()
	plan, err := fairnessPlanFor(opts)
	if err != nil {
		t.Fatalf("fairnessPlanFor: %v", err)
	}
	specs := map[string]populationSpec{populationQuiet: plan.Quiet, populationNoisy: plan.Noisy}
	for _, repeat := range doc.Repeats {
		for _, arm := range repeat.Arms {
			for _, pop := range arm.Populations {
				spec := specs[pop.Name]
				for index, method := range pop.Methods {
					// Verbs are cycled one per tick, so the first
					// ticks % len(verbs) of them get one more than the rest,
					// and the methods are recorded in the population's own
					// verb order.
					want := plan.ticks(spec) / len(spec.Verbs)
					if index < plan.ticks(spec)%len(spec.Verbs) {
						want++
					}
					want *= spec.Credentials
					if method.Intended != want {
						t.Errorf("repetition %d, %s arm, %s %s intended %d, want the phase's own %d: "+
							"a window carrying the lead-in as well would offer more",
							repeat.Index+1, arm.Arm, pop.Name, method.Method, method.Intended, want)
					}
				}
			}
		}
	}
}

// smallFairnessOptions are the flags of a fairness run sized for a test: a
// phase of a second and a lead-in long enough to drain the bucket the stand-in
// holds.
//
// The quiet rate is a fifth of what the bound meters once its two verbs are
// counted, which is what makes the population quiet. It used to be exactly the
// metered rate, so the assertion below that the quiet tenant was never refused
// was carried by the burst covering a three-hundred-millisecond window rather
// than by the tenant being under the bound at all, and the same run against a
// longer phase would have refused it.
func smallFairnessOptions(binary, out string) options {
	return options{
		binary:          binary,
		record:          "site/src/data/resource-benchmark.json",
		sampleInterval:  20 * time.Millisecond,
		fairness:        "tools-call-rps",
		fairnessJSON:    out,
		fairnessSurface: surfaceDynamic,
		fairnessQuiet:   1, fairnessNoisy: 1,
		fairnessQuietRate: 4, fairnessNoisyRate: 200,
		fairnessPhase: time.Second, fairnessLeadIn: 300 * time.Millisecond,
		fairnessDeadline: 2 * time.Second, fairnessRepeats: 1,
	}
}

// TestRunFairness_CounterbalancesTheArmsAcrossRepetitions drives two whole
// repetitions against the stand-in and verifies the counterbalancing is wired
// to the run rather than only to the function that computes it.
//
// The order the arms run in is the only defense against a monotone drift of
// the host landing on one of them, and it was asserted nowhere: armOrder had
// its own test, its use had none, and every document handed to the verdict in
// this package was hand-built by a fixture that called armOrder itself, so the
// fixtures could not disagree with the production wiring either. Replacing the
// call with a fixed order left the suite green.
func TestRunFairness_CounterbalancesTheArmsAcrossRepetitions(t *testing.T) {
	binary := standinBinary(t)
	root := t.TempDir()
	opts := smallFairnessOptions(binary, filepath.Join(root, "two.json"))
	opts.fairnessRepeats = 2

	if err := runFairness(opts, root); err != nil {
		t.Fatalf("runFairness: %v", err)
	}
	doc, err := readFairness(opts.fairnessJSON)
	if err != nil {
		t.Fatalf("readFairness: %v", err)
	}
	if len(doc.Repeats) != 2 {
		t.Fatalf("repeats = %d, want two", len(doc.Repeats))
	}
	for index, repeat := range doc.Repeats {
		t.Run(fmt.Sprintf("repetition %d", index+1), func(t *testing.T) {
			want := armOrder(index)
			if repeat.Index != index || len(repeat.Order) != 2 || repeat.Order[0] != want[0] || repeat.Order[1] != want[1] {
				t.Errorf("repetition %d ran %v, want %v", repeat.Index, repeat.Order, want)
			}
			if len(repeat.Arms) != 2 {
				t.Fatalf("repetition %d holds %d arms, want two", index+1, len(repeat.Arms))
			}
			for position, arm := range repeat.Arms {
				if arm.Arm != repeat.Order[position] {
					t.Errorf("arm %d is %q, want the %q the order names", position, arm.Arm, repeat.Order[position])
				}
			}
		})
	}
	if doc.Repeats[0].Order[0] == doc.Repeats[1].Order[0] {
		t.Errorf("both repetitions began with the %s arm, so a drift of the host lands on one of them", doc.Repeats[0].Order[0])
	}
	assertMeasuredThePhaseAlone(t, opts, doc)
	// Whatever this host let the stand-in do, the answer has to be one of the
	// four and it has to carry its reason.
	if doc.Verdict.Direction == "" || doc.Verdict.Reason == "" {
		t.Errorf("verdict = %+v, want a direction and the reason for it", doc.Verdict)
	}
}

// TestRunFairnessArm_MarksAnArmThatFailedAControlIncomparable verifies the one
// line that joins the controls to the verdict.
//
// checkArm's notes were tested and the verdict's honoring of Comparable was
// tested, and the assignment between them was not: replacing it with a
// constant true left the suite green while an arm whose requests failed for a
// reason that is not the bound would be compared anyway and could publish an
// improvement.
func TestRunFairnessArm_MarksAnArmThatFailedAControlIncomparable(t *testing.T) {
	t.Setenv("STANDIN_FAIL", methodToolsCall)
	r := standinRunner(t)
	r.report = progressFunc(false)
	plan := twoPopulationPlan(t)
	plan.Phase = 100 * time.Millisecond
	plan.Quiet.Rate, plan.Noisy.Rate = 20, 200

	arm, err := r.runFairnessArm(t.Context(), plan, armOff)
	if err != nil {
		t.Fatalf("runFairnessArm: %v", err)
	}
	if arm.Comparable {
		t.Errorf("arm = %+v, want an arm that failed a control marked incomparable", arm)
	}
	if len(arm.Notes) == 0 || !strings.Contains(strings.Join(arm.Notes, "; "), "not this bound") {
		t.Errorf("notes = %v, want one naming the failures", arm.Notes)
	}
	t.Run("and the verdict refuses to compare it", func(t *testing.T) {
		doc := fairnessDocOf(8, []armFixture{saturated(armOff, 20), saturated(armOn, 10)},
			[]armFixture{saturated(armOn, 10), saturated(armOff, 20)})
		doc.Repeats[0].Arms[0] = arm
		_, verdict := judgeFairness(doc)
		if verdict.Direction != directionNotComparable {
			t.Errorf("verdict = %+v, want %q", verdict, directionNotComparable)
		}
	})
}

// TestRunFairness_RefusesToWriteWhereItWouldOverwriteTheRecord verifies the
// mode cannot land on the published record, whose charts and tables are gated
// by a byte comparison against it.
func TestRunFairness_RefusesToWriteWhereItWouldOverwriteTheRecord(t *testing.T) {
	root := t.TempDir()
	opts := smallFairnessOptions("/nonexistent", "site/src/data/resource-benchmark.json")
	err := runFairness(opts, root)
	if err == nil || !strings.Contains(err.Error(), "which is the published record") {
		t.Errorf("runFairness = %v, want it refused before anything was measured", err)
	}
}

// TestRunFairness_ReportsWhatItCouldNotDo verifies the mode's failures are
// named rather than measured around.
func TestRunFairness_ReportsWhatItCouldNotDo(t *testing.T) {
	cases := []struct {
		name string
		edit func(*options)
		want string
	}{
		{name: "a bound nobody declared", edit: func(o *options) { o.fairness = "nope" }, want: "no bound named"},
		{name: "a binary that is not there", edit: func(o *options) { o.binary = "/nonexistent/server" }, want: "start server"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			opts := smallFairnessOptions(standinBinary(t), filepath.Join(root, "fairness.json"))
			tc.edit(&opts)
			err := runFairness(opts, root)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("runFairness = %v, want an error about %q", err, tc.want)
			}
		})
	}
	t.Run("a document it could not write", func(t *testing.T) {
		root := t.TempDir()
		opts := smallFairnessOptions(standinBinary(t), t.TempDir())
		if err := runFairness(opts, root); err == nil || !strings.Contains(err.Error(), "write ") {
			t.Errorf("runFairness = %v, want the write failure after the measurement", err)
		}
	})
	t.Run("a harness that could not build", func(t *testing.T) {
		original := buildServerBinary
		t.Cleanup(func() { buildServerBinary = original })
		buildServerBinary = func(string) (string, error) { return "", errors.New("no toolchain") }
		root := t.TempDir()
		opts := smallFairnessOptions("", filepath.Join(root, "fairness.json"))
		if err := runFairness(opts, root); err == nil || !strings.Contains(err.Error(), "no toolchain") {
			t.Errorf("runFairness = %v, want the build failure", err)
		}
	})
}

// TestRunFairnessArm_StopsWhenTheBoundNeverFired verifies the positive control
// end to end, against a stand-in whose bound meters a method nobody drives.
//
// The numbers of a bound that was never there are the numbers of a bound that
// helped nobody, and the conclusions are opposite.
func TestRunFairnessArm_StopsWhenTheBoundNeverFired(t *testing.T) {
	t.Setenv("STANDIN_REFUSE_METHOD", "resources/read")
	r := standinRunner(t)
	r.report = progressFunc(false)
	plan := twoPopulationPlan(t)
	plan.Phase = 100 * time.Millisecond
	plan.Quiet.Rate, plan.Noisy.Rate = 20, 200

	_, err := r.runFairnessArm(t.Context(), plan, armOn)
	if !errors.Is(err, errBoundDidNotFire) {
		t.Errorf("runFairnessArm = %v, want the positive control to stop the run", err)
	}
}

// TestRunFairnessArm_ReportsWhatItCouldNotStart verifies the arm's own failure
// paths: a surface with no call behind it, and credentials that could not be
// warmed.
func TestRunFairnessArm_ReportsWhatItCouldNotStart(t *testing.T) {
	t.Run("a surface this benchmark does not drive", func(t *testing.T) {
		r := &runner{}
		plan := twoPopulationPlan(t)
		plan.Surface = "sideways"
		if _, err := r.runFairnessArm(t.Context(), plan, armOff); err == nil ||
			!strings.Contains(err.Error(), "unknown surface") {
			t.Errorf("runFairnessArm = %v, want the surface refused", err)
		}
	})
	t.Run("credentials that could not be warmed", func(t *testing.T) {
		t.Setenv("STANDIN_FAIL", methodToolsList)
		r := standinRunner(t)
		r.report = progressFunc(false)
		plan := twoPopulationPlan(t)
		if _, err := r.runFairnessArm(t.Context(), plan, armOff); err == nil ||
			!strings.Contains(err.Error(), "cold tools/list") {
			t.Errorf("runFairnessArm = %v, want the warm-up failure", err)
		}
	})
}

// TestRunFairnessScenario_NamesTheRepetitionThatFailed verifies a failure
// carries which repetition and which arm it came from.
func TestRunFairnessScenario_NamesTheRepetitionThatFailed(t *testing.T) {
	t.Setenv("STANDIN_REFUSE_METHOD", "resources/read")
	r := standinRunner(t)
	r.report = progressFunc(false)
	plan := twoPopulationPlan(t)
	plan.Phase = 100 * time.Millisecond
	plan.Quiet.Rate, plan.Noisy.Rate = 20, 200

	err := r.runFairnessScenario(t.Context(), plan, fairnessHeader(plan))
	if err == nil || !strings.Contains(err.Error(), "repeat 1, on arm") {
		t.Errorf("runFairnessScenario = %v, want the repetition and arm named", err)
	}
}

// TestDrive_IsSafeUnderConcurrentPopulations verifies the tally survives every
// credential of both populations writing to it at once, which is the shape the
// race detector is pointed at.
func TestDrive_IsSafeUnderConcurrentPopulations(t *testing.T) {
	call, err := callFor(surfaceDynamic)
	if err != nil {
		t.Fatalf("callFor: %v", err)
	}
	tally := newFairTally(call, nil)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			pop := populationQuiet
			if index%2 == 1 {
				pop = populationNoisy
			}
			for range 20 {
				tally.offered(pop, verbs[verbCall])
				tally.record(pop, verbs[verbCall], observation{latency: time.Millisecond})
				tally.dropped(pop, verbs[verbList])
			}
		}(i)
	}
	wg.Wait()
	for _, pop := range []string{populationQuiet, populationNoisy} {
		t.Run(pop, func(t *testing.T) {
			if got := tally.methods[pop][methodToolsCall].intended; got != 80 {
				t.Errorf("intended = %d, want every tick counted", got)
			}
			if got := tally.methods[pop][methodToolsList].dropped; got != 80 {
				t.Errorf("dropped = %d, want every drop counted", got)
			}
		})
	}
}
