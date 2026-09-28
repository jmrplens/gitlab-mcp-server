// held_test.go covers the held-request mode: the flags it reads, how a step
// files each call, when a step is sampled, what a sample takes from the
// process, and the whole ladder driven against the stand-in.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fastHeldSettling makes a step settle in milliseconds rather than seconds.
func fastHeldSettling(t *testing.T) {
	t.Helper()
	poll, quiet, deadline := heldPoll, heldQuiet, heldDeadline
	t.Cleanup(func() { heldPoll, heldQuiet, heldDeadline = poll, quiet, deadline })
	heldPoll, heldQuiet, heldDeadline = 5*time.Millisecond, 300*time.Millisecond, 30*time.Second
}

// readHeld decodes a document a -held run wrote.
func readHeld(t *testing.T, path string) HeldDoc {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the held document: %v", err)
	}
	var doc HeldDoc
	if decodeErr := json.Unmarshal(raw, &doc); decodeErr != nil {
		t.Fatalf("decode the held document: %v", decodeErr)
	}
	return doc
}

// heldOptions are the flags of a held run against the stand-in, writing where
// the caller says.
func heldOptions(binary, out, counts string) options {
	return options{
		binary:          binary,
		record:          "site/src/data/resource-benchmark.json",
		sampleInterval:  20 * time.Millisecond,
		held:            counts,
		heldCredentials: 2,
		heldJSON:        out,
	}
}

// TestParseFlags_HeldFlags_AreRead verifies the held mode's flags reach the
// options, and what a run that types none of them gets.
func TestParseFlags_HeldFlags_AreRead(t *testing.T) {
	opts := withArgs(t, "-held=10,20", "-held-credentials=4", "-held-nofile=512", "-held-json=/tmp/held.json")
	if opts.held != "10,20" || opts.heldCredentials != 4 || opts.heldNoFile != 512 || opts.heldJSON != "/tmp/held.json" {
		t.Errorf("held %q, credentials %d, nofile %d, json %q; want what was typed",
			opts.held, opts.heldCredentials, opts.heldNoFile, opts.heldJSON)
	}
	defaults := withArgs(t)
	if defaults.held != "" || defaults.heldCredentials != 1 || defaults.heldNoFile != 0 || defaults.heldJSON != defaultHeldRecord {
		t.Errorf("defaults held %q, credentials %d, nofile %d, json %q; want off, one credential, the inherited limit and %s",
			defaults.held, defaults.heldCredentials, defaults.heldNoFile, defaults.heldJSON, defaultHeldRecord)
	}
}

// TestOptionsValidate_HeldNoFile_IsRefusedOnWindows verifies a descriptor
// limit is refused where the shell that would set it does not exist, and
// accepted wherever it does.
func TestOptionsValidate_HeldNoFile_IsRefusedOnWindows(t *testing.T) {
	previous := runtimeGOOS
	t.Cleanup(func() { runtimeGOOS = previous })
	base := options{held: "1", heldCredentials: 1, rounds: 1, sampleInterval: time.Millisecond, stepDuration: time.Second}

	runtimeGOOS = "windows"
	limited := base
	limited.heldNoFile = 64
	if err := limited.validate(); err == nil || !strings.Contains(err.Error(), "Windows") {
		t.Errorf("validate = %v, want a Windows refusal of -held-nofile", err)
	}
	if err := base.validate(); err != nil {
		t.Errorf("validate = %v, want a Windows run with no limit accepted", err)
	}
	runtimeGOOS = "darwin"
	if err := limited.validate(); err != nil {
		t.Errorf("validate = %v, want a limit accepted where /bin/sh exists", err)
	}
}

// TestExecute_HeldRun_AnswersBeforeTheRecordIsRead verifies the dispatch: a
// held run returns from its own mode, so a refusal only that mode makes is
// what comes back, rather than anything about the record or the matrix.
func TestExecute_HeldRun_AnswersBeforeTheRecordIsRead(t *testing.T) {
	err := execute(options{
		rounds: 1, sampleInterval: time.Millisecond, stepDuration: time.Millisecond,
		held: "1", heldCredentials: 1, heldJSON: defaultRecord,
	})
	if err == nil || !strings.Contains(err.Error(), "published record") {
		t.Errorf("execute = %v, want the held mode's refusal to write over the record", err)
	}
}

// TestHeldTally_Record_FilesEachOutcomeAndKeepsTheFirstMessage verifies the
// three outcomes are told apart the way the gate answers: a 503, wrapped or
// not, is the ceiling's refusal; any other status and any other error is a
// failure; and the first message of each kind is the one kept.
func TestHeldTally_Record_FilesEachOutcomeAndKeepsTheFirstMessage(t *testing.T) {
	var tally heldTally
	tally.record(nil)
	tally.record(&httpStatusError{Method: methodToolsCall, Status: http.StatusServiceUnavailable, Snippet: "first refusal"})
	tally.record(fmt.Errorf("call: %w", &httpStatusError{Status: http.StatusServiceUnavailable, Snippet: "second refusal"}))
	tally.record(&httpStatusError{Method: methodToolsCall, Status: http.StatusTooManyRequests, Snippet: "not this ceiling"})
	tally.record(errors.New("second failure"))

	if got := [4]int64{tally.returned.Load(), tally.served.Load(), tally.refused.Load(), tally.failed.Load()}; got != [4]int64{5, 1, 2, 2} {
		t.Errorf("returned, served, refused, failed = %v, want [5 1 2 2]", got)
	}
	if tally.firstRefusal != "first refusal" {
		t.Errorf("first refusal = %q, want the first 503's text", tally.firstRefusal)
	}
	if want := "tools/call: HTTP 429: not this ceiling"; tally.firstFailure != want {
		t.Errorf("first failure = %q, want %q", tally.firstFailure, want)
	}
}

// TestHeldCallParams_AreAFreshProjectReadEachTime verifies the call a held
// step sends, and that no two calls share a map the encoder writes into.
func TestHeldCallParams_AreAFreshProjectReadEachTime(t *testing.T) {
	first, second := heldCallParams(), heldCallParams()
	want := `{"arguments":{"action":"project.get","params":{"project_id":"1"}},"name":"gitlab_execute_action"}`
	encoded, err := json.Marshal(first)
	if err != nil || string(encoded) != want {
		t.Errorf("params = %s (%v), want %s", encoded, err, want)
	}
	first["_meta"] = "written by the encoder"
	if _, shared := second["_meta"]; shared {
		t.Error("two calls share one parameter map")
	}
}

// TestNofileLabel_NamesTheLimitOrItsAbsence verifies the progress line.
func TestNofileLabel_NamesTheLimitOrItsAbsence(t *testing.T) {
	for nofile, want := range map[int]string{0: "inherited", -1: "inherited", 1: "1", 256: "256"} {
		t.Run(want+" for "+strconv.Itoa(nofile), func(t *testing.T) {
			if got := nofileLabel(nofile); got != want {
				t.Errorf("nofileLabel(%d) = %q, want %q", nofile, got, want)
			}
		})
	}
}

// heldClock is a clock only a pause moves, so a step's wait can be driven to
// the instant it ends and no further.
type heldClock struct{ now time.Time }

// useHeldClock installs a clock of the test's own, with a ten-millisecond poll,
// and returns it.
func useHeldClock(t *testing.T, quiet, deadline time.Duration) *heldClock {
	t.Helper()
	clock := &heldClock{now: time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)}
	now, sleep := heldNow, heldSleep
	poll, previousQuiet, previousDeadline := heldPoll, heldQuiet, heldDeadline
	t.Cleanup(func() {
		heldNow, heldSleep = now, sleep
		heldPoll, heldQuiet, heldDeadline = poll, previousQuiet, previousDeadline
	})
	heldNow = func() time.Time { return clock.now }
	heldSleep = func(d time.Duration) { clock.now = clock.now.Add(d) }
	heldPoll, heldQuiet, heldDeadline = 10*time.Millisecond, quiet, deadline
	return clock
}

// awaitHeldWithin runs awaitHeld and fails the test if it never returns, as a
// wait whose ends were broken would not.
func awaitHeldWithin(t *testing.T, ctx context.Context, offered int, accounted func() int64) string {
	t.Helper()
	var got string
	finishWithin(t, 5*time.Second, "the wait before a sample", func() { got = awaitHeld(ctx, offered, accounted) })
	return got
}

// TestAwaitHeld_SaysHowTheWaitEnded verifies the three ends of a step's wait
// and the instant each comes: every call accounted for, as soon as it is; a
// count that stopped moving, once it has been still for exactly the quiet
// period; and a deadline, which a cancelled run reaches at once.
func TestAwaitHeld_SaysHowTheWaitEnded(t *testing.T) {
	t.Run("every call accounted for", func(t *testing.T) {
		useHeldClock(t, time.Hour, time.Hour)
		var seen atomic.Int64
		got := awaitHeldWithin(t, t.Context(), 3, func() int64 { return seen.Add(1) })
		if got != settledAll || seen.Load() != 3 {
			t.Errorf("awaitHeld = %q after %d reads, want %q on the third", got, seen.Load(), settledAll)
		}
	})
	t.Run("a count exactly at the offer", func(t *testing.T) {
		useHeldClock(t, time.Hour, time.Hour)
		if got := awaitHeldWithin(t, t.Context(), 2, func() int64 { return 2 }); got != settledAll {
			t.Errorf("awaitHeld = %q, want %q", got, settledAll)
		}
	})
	t.Run("a count that stopped moving", func(t *testing.T) {
		clock := useHeldClock(t, 10*time.Millisecond, time.Hour)
		started := clock.now
		var reads atomic.Int64
		got := awaitHeldWithin(t, t.Context(), 5, func() int64 { reads.Add(1); return 1 })
		if got != settledQuiet || reads.Load() != 2 || clock.now.Sub(started) != heldQuiet {
			t.Errorf("awaitHeld = %q after %d reads and %v, want %q the moment the count had been still for %v",
				got, reads.Load(), clock.now.Sub(started), settledQuiet, heldQuiet)
		}
	})
	t.Run("a count that never settles", func(t *testing.T) {
		clock := useHeldClock(t, time.Hour, 30*time.Millisecond)
		started := clock.now
		var moving atomic.Int64
		got := awaitHeldWithin(t, t.Context(), 1<<30, func() int64 { return moving.Add(1) })
		if got != settledLimit || clock.now.Sub(started) != 40*time.Millisecond {
			t.Errorf("awaitHeld = %q after %v, want %q at the first read past the %v deadline",
				got, clock.now.Sub(started), settledLimit, heldDeadline)
		}
	})
	t.Run("a cancelled run", func(t *testing.T) {
		useHeldClock(t, time.Hour, time.Hour)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		var reads atomic.Int64
		got := awaitHeldWithin(t, ctx, 5, func() int64 { reads.Add(1); return 0 })
		if got != settledLimit || reads.Load() != 1 {
			t.Errorf("awaitHeld = %q after %d reads, want %q after one", got, reads.Load(), settledLimit)
		}
	})
}

// TestCountDescriptors_ReadsTheProcessFdDirectory verifies the count comes
// from /proc/<pid>/fd, that a directory it cannot list is an error, and that
// a platform without /proc says so rather than guessing.
func TestCountDescriptors_ReadsTheProcessFdDirectory(t *testing.T) {
	root := t.TempDir()
	previousRoot, previousGOOS := procRoot, runtimeGOOS
	t.Cleanup(func() { procRoot, runtimeGOOS = previousRoot, previousGOOS })
	procRoot, runtimeGOOS = root, "linux"
	fd := filepath.Join(root, "42", "fd")
	if err := os.MkdirAll(fd, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for descriptor := range 3 {
		if err := os.WriteFile(filepath.Join(fd, strconv.Itoa(descriptor)), nil, 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	if got, err := countDescriptors(42); err != nil || got != 3 {
		t.Errorf("countDescriptors = %d, %v; want 3", got, err)
	}
	if _, err := countDescriptors(43); err == nil || !strings.Contains(err.Error(), "list descriptors") {
		t.Errorf("countDescriptors of a missing process = %v, want the listing refused", err)
	}
	runtimeGOOS = "darwin"
	if got, err := countDescriptors(42); err == nil || got != 0 || !strings.Contains(err.Error(), "darwin") {
		t.Errorf("countDescriptors on darwin = %d, %v; want an error naming the platform", got, err)
	}
}

// TestProbeHealth_AnswersTheStatusOrUnanswered verifies the probe reports what
// /health answered, on a connection of its own, and "unanswered" for a process
// that did not answer at all or an address no request can be built for.
func TestProbeHealth_AnswersTheStatusOrUnanswered(t *testing.T) {
	var connections atomic.Int64
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
	}
	server.Start()
	defer server.Close()
	addr := strings.TrimPrefix(server.URL, "http://")

	for range 2 {
		if status, elapsed := probeHealth(t.Context(), addr); status != "503" || elapsed <= 0 {
			t.Errorf("probeHealth = %q in %v, want the 503 it answered and a positive time", status, elapsed)
		}
	}
	if connections.Load() != 2 {
		t.Errorf("two probes opened %d connections, want one each", connections.Load())
	}

	closed := httptest.NewServer(http.NotFoundHandler())
	closedAddr := strings.TrimPrefix(closed.URL, "http://")
	closed.Close()
	if status, _ := probeHealth(t.Context(), closedAddr); status != healthUnanswered {
		t.Errorf("probeHealth of a closed listener = %q, want %q", status, healthUnanswered)
	}
	if status, _ := probeHealth(t.Context(), "bad host"); status != healthUnanswered {
		t.Errorf("probeHealth of an address no URL holds = %q, want %q", status, healthUnanswered)
	}
}

// TestHeldStep_CostOver_DividesTheGrowthByTheCallsHeld verifies the per-call
// figures, that a step holding nothing has none, and that an unknown
// descriptor count on either side leaves that figure at zero rather than
// dividing a sentinel.
func TestHeldStep_CostOver_DividesTheGrowthByTheCallsHeld(t *testing.T) {
	idle := HeldSample{Descriptors: 10, Goroutines: 20, HeapBytes: 1 << 20, RSSBytes: 4 << 20}
	step := HeldStep{Held: 4, Sample: HeldSample{
		Descriptors: 18, Goroutines: 44, HeapBytes: (1 << 20) + 4*2048, RSSBytes: (4 << 20) + 4*3072,
	}}
	want := HeldCost{Descriptors: 2, Goroutines: 6, HeapKiB: 2, RSSKiB: 3}
	if got := step.costOver(idle); got == nil || *got != want {
		t.Errorf("costOver = %+v, want %+v", got, want)
	}

	unknownNow := step
	unknownNow.Sample.Descriptors = -1
	if got := unknownNow.costOver(idle); got == nil || got.Descriptors != 0 || got.Goroutines != 6 {
		t.Errorf("costOver with the step's descriptors unknown = %+v, want no descriptor figure and the rest", got)
	}
	unknownIdle := idle
	unknownIdle.Descriptors = -1
	if got := step.costOver(unknownIdle); got == nil || got.Descriptors != 0 || got.RSSKiB != 3 {
		t.Errorf("costOver with the idle descriptors unknown = %+v, want no descriptor figure and the rest", got)
	}
	for _, held := range []int{0, -1} {
		t.Run("held "+strconv.Itoa(held), func(t *testing.T) {
			nothing := step
			nothing.Held = held
			if got := nothing.costOver(idle); got != nil {
				t.Errorf("costOver of a step holding %d = %+v, want nil", held, got)
			}
		})
	}
	one := HeldStep{Held: 1, Sample: HeldSample{Descriptors: 11, Goroutines: 21}}
	if got := one.costOver(idle); got == nil || got.Descriptors != 1 || got.Goroutines != 1 {
		t.Errorf("costOver of one held call = %+v, want one descriptor and one goroutine", got)
	}
	// Zero is a count, not the unknown sentinel, on either side.
	fromNone := HeldStep{Held: 2, Sample: HeldSample{Descriptors: 4}}
	if got := fromNone.costOver(HeldSample{}); got == nil || got.Descriptors != 2 {
		t.Errorf("costOver over an idle process with no descriptors = %+v, want 2 a call", got)
	}
	toNone := HeldStep{Held: 2, Sample: HeldSample{Descriptors: 0}}
	if got := toNone.costOver(HeldSample{Descriptors: 4}); got == nil || got.Descriptors != -2 {
		t.Errorf("costOver down to no descriptors = %+v, want -2 a call", got)
	}
}

// TestHeldSummaries_RenderEveryFigure pins the progress lines, notes and the
// per-call figures included.
func TestHeldSummaries_RenderEveryFigure(t *testing.T) {
	sample := HeldSample{Descriptors: 402, Goroutines: 1173, HeapBytes: 50 << 20, RSSBytes: 190 << 20, Health: "200", HealthMs: 1}
	if got, want := sample.summary(), "402 descriptors, 1173 goroutines, 50.0 MiB heap, 190.0 MiB resident, /health 200 in 1 ms"; got != want {
		t.Errorf("summary = %q, want %q", got, want)
	}
	noted := sample
	noted.Notes = []string{"heap: refused", "resident set: gone"}
	if got := noted.summary(); !strings.HasSuffix(got, " (heap: refused; resident set: gone)") {
		t.Errorf("summary = %q, want the notes after it", got)
	}
	step := HeldStep{Offered: 250, Held: 192, Settled: settledAll, Served: 192, Refused: 58, Sample: sample}
	bare := "250 offered, 192 held (settled): " + sample.summary() + "; then 192 served, 58 refused, 0 failed"
	if got := step.summary(); got != bare {
		t.Errorf("summary = %q, want %q", got, bare)
	}
	step.PerHeld = &HeldCost{Descriptors: 2.04, Goroutines: 6.02, HeapKiB: 53.4, RSSKiB: 284.2}
	if got, want := step.summary(), bare+"; per held call 2.04 descriptors, 6.02 goroutines, 53.4 KiB heap, 284.2 KiB resident"; got != want {
		t.Errorf("summary = %q, want %q", got, want)
	}
}

// TestRunHeldStep_FilesEachCallAndSamplesWhatItCannotReach drives one step
// over scripted connections: each call filed by how it ended, every figure the
// sample could not take noted with its reason, and every connection closed
// for the next step.
func TestRunHeldStep_FilesEachCallAndSamplesWhatItCannotReach(t *testing.T) {
	fastHeldSettling(t)
	stub := startStubGitLab()
	defer stub.close()
	r := &runner{stub: stub}
	served := &scriptedConn{}
	refused := &scriptedConn{answer: func(string, int) error {
		return &httpStatusError{Method: methodToolsCall, Status: http.StatusServiceUnavailable, Snippet: "held full"}
	}}
	failed := &scriptedConn{answer: func(string, int) error { return errors.New("broke") }}
	in := heldInput{
		// A target that never started: no process, no address.
		tgt:      &httpTarget{},
		profiler: newPprofClient("http://127.0.0.1:1"),
		conns:    []*clientConn{{rpc: served}, {rpc: refused}, {rpc: failed}},
	}
	previousGOOS := runtimeGOOS
	t.Cleanup(func() { runtimeGOOS = previousGOOS })
	runtimeGOOS = "plan9"

	step := r.runHeldStep(t.Context(), in, 6)
	if step.Offered != 6 || step.Held != 0 || step.Settled != settledAll {
		t.Errorf("step = %+v, want six offered, none held, every one accounted for", step)
	}
	if step.Served != 2 || step.Refused != 2 || step.Failed != 2 {
		t.Errorf("served, refused, failed = %d, %d, %d; want two of each, round robin", step.Served, step.Refused, step.Failed)
	}
	if step.FirstRefusal != "held full" || step.FirstFailure != "broke" {
		t.Errorf("first refusal %q, first failure %q; want the connections' own", step.FirstRefusal, step.FirstFailure)
	}
	for name, conn := range map[string]*scriptedConn{"served": served, "refused": refused, "failed": failed} {
		t.Run("the "+name+" connection took its share", func(t *testing.T) {
			if conn.calls.Load() != 2 {
				t.Errorf("called %d times, want 2", conn.calls.Load())
			}
		})
	}
	sample := step.Sample
	if sample.Descriptors != -1 || sample.Health != healthUnanswered {
		t.Errorf("sample = %+v, want unknown descriptors and an unanswered /health", sample)
	}
	for _, prefix := range []string{"descriptors: ", "goroutines: ", "heap: ", "resident set: "} {
		t.Run("a note for "+strings.TrimSuffix(prefix, ": "), func(t *testing.T) {
			for _, note := range sample.Notes {
				if strings.HasPrefix(note, prefix) {
					return
				}
			}
			t.Errorf("notes %q, want one starting %q", sample.Notes, prefix)
		})
	}
}

// TestRunHeld_MeasuresTheLadderAndWritesItsOwnDocument drives the whole mode
// against the stand-in: two steps, the second past a held ceiling of three, a
// descriptor limit the server was started under, and a document written where
// it was asked for.
//
// The stand-in holds a project call for as long as the stand-in instance holds
// its read, which is the shape the real server has, so the held count is the
// instance's and the refusals are the stand-in's own 503s.
func TestRunHeld_MeasuresTheLadderAndWritesItsOwnDocument(t *testing.T) {
	binary := standinBinary(t)
	fastHeldSettling(t)
	t.Setenv("STANDIN_HELD_LIMIT", "3")
	root := t.TempDir()
	opts := heldOptions(binary, filepath.Join(root, "out", "held.json"), "2,4")
	if runtime.GOOS == "linux" {
		opts.heldNoFile = 256
	}

	var runErr error
	finishWithin(t, 2*time.Minute, "a two-step held run", func() { runErr = runHeld(opts, root) })
	if runErr != nil {
		t.Fatalf("runHeld: %v", runErr)
	}
	doc := readHeld(t, opts.heldJSON)
	assertHeldHeader(t, doc, opts)
	if len(doc.Steps) != 2 {
		t.Fatalf("steps = %+v, want two", doc.Steps)
	}
	assertHeldSteps(t, doc.Steps[0], doc.Steps[1])
}

// assertHeldHeader checks what a held document says about its run and about
// the idle process every step is priced against.
func assertHeldHeader(t *testing.T, doc HeldDoc, opts options) {
	t.Helper()
	if doc.Schema != heldSchema || doc.Action != heldAction || doc.Credentials != 2 || doc.NoFile != opts.heldNoFile {
		t.Errorf("document header %+v, want the schema, the action, two credentials and the limit asked for", doc)
	}
	if doc.Server.Version != "standin" || doc.GeneratedAt == "" {
		t.Errorf("server %+v generated %q, want the build measured and a timestamp", doc.Server, doc.GeneratedAt)
	}
	if doc.Idle.Health != "200" || doc.Idle.Goroutines <= 0 || doc.Idle.HeapBytes == 0 {
		t.Errorf("idle = %+v, want a healthy process with goroutines and a heap", doc.Idle)
	}
	if runtime.GOOS == "linux" && (doc.Idle.Descriptors <= 0 || doc.Idle.RSSBytes == 0 || len(doc.Idle.Notes) != 0) {
		t.Errorf("idle = %+v, want descriptors and a resident set read from /proc", doc.Idle)
	}
}

// assertHeldSteps checks the two steps of the stand-in ladder: one under the
// stand-in's ceiling of three and one past it.
func assertHeldSteps(t *testing.T, under, over HeldStep) {
	t.Helper()
	if under.Offered != 2 || under.Held != 2 || under.Settled != settledAll ||
		under.Served != 2 || under.Refused != 0 || under.Failed != 0 {
		t.Errorf("first step = %s, want both calls held and then served", under.summary())
	}
	if over.Offered != 4 || over.Held != 3 || over.Settled != settledAll ||
		over.Served != 3 || over.Refused != 1 || over.Failed != 0 {
		t.Errorf("second step = %s, want three held and served and one refused", over.summary())
	}
	if !strings.Contains(over.FirstRefusal, "holding as many requests") {
		t.Errorf("first refusal = %q, want the ceiling's text", over.FirstRefusal)
	}
	if under.PerHeld == nil || over.PerHeld == nil {
		t.Errorf("per held = %+v and %+v, want both steps priced", under.PerHeld, over.PerHeld)
	}
	if under.Sample.Health != "200" || over.Sample.Health != "200" {
		t.Errorf("/health answered %q and %q while calls were held, want 200", under.Sample.Health, over.Sample.Health)
	}
}

// TestRunHeld_ReportsWhatItCouldNotDo covers every way the mode stops short:
// counts it cannot read, a document that would overwrite the published
// record, a server it could not build, start or admit a credential to, and a
// document it could not write.
func TestRunHeld_ReportsWhatItCouldNotDo(t *testing.T) {
	fastHeldSettling(t)
	root := t.TempDir()
	out := filepath.Join(root, "held.json")

	t.Run("counts it cannot read", func(t *testing.T) {
		if err := runHeld(heldOptions("/unused", out, "3,1"), root); err == nil || !strings.Contains(err.Error(), "-held:") {
			t.Errorf("runHeld = %v, want the -held refusal", err)
		}
	})
	for name, target := range map[string]string{"the record it was given": "custom/record.json", "the default record": defaultRecord} {
		t.Run("a document naming "+name, func(t *testing.T) {
			opts := heldOptions("/unused", target, "1")
			opts.record = "custom/record.json"
			if err := runHeld(opts, root); err == nil || !strings.Contains(err.Error(), "published record") {
				t.Errorf("runHeld = %v, want the refusal to write over the record", err)
			}
		})
	}
	t.Run("a build that failed", func(t *testing.T) {
		original := buildServerBinary
		t.Cleanup(func() { buildServerBinary = original })
		buildServerBinary = func(string) (string, error) { return "", errors.New("no toolchain") }
		if err := runHeld(heldOptions("", out, "1"), root); err == nil || !strings.Contains(err.Error(), "no toolchain") {
			t.Errorf("runHeld = %v, want the build failure", err)
		}
	})
	t.Run("a server that does not start", func(t *testing.T) {
		missing := filepath.Join(root, "no-such-server")
		if err := runHeld(heldOptions(missing, out, "1"), root); err == nil {
			t.Error("runHeld measured a server that does not exist")
		}
		if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
			t.Errorf("a run that measured nothing wrote a document: %v", statErr)
		}
	})
	t.Run("a credential that is not admitted", func(t *testing.T) {
		binary := standinBinary(t)
		t.Setenv("STANDIN_FAIL", methodToolsList)
		err := runHeld(heldOptions(binary, out, "1"), root)
		if err == nil || !strings.Contains(err.Error(), "cold tools/list") {
			t.Errorf("runHeld = %v, want the admission failure", err)
		}
	})
	t.Run("a document it cannot write", func(t *testing.T) {
		binary := standinBinary(t)
		blocker := filepath.Join(root, "a-file")
		if err := os.WriteFile(blocker, nil, 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		opts := heldOptions(binary, filepath.Join(blocker, "held.json"), "1")
		var err error
		finishWithin(t, time.Minute, "a one-step held run", func() { err = runHeld(opts, root) })
		if err == nil || !strings.Contains(err.Error(), blocker) {
			t.Errorf("runHeld = %v, want the write refused where a file stands in the way", err)
		}
	})
}
