package tenancy

import (
	"fmt"
	"math"
	"testing"
)

// fakeHoldings is a [Holdings] with fixed counts that records how often each
// is read. With strict set, reading the watchers while a stream is open fails
// the test, since on the server that read takes the subscription manager's
// lock (internal/subscriptions Manager.Len) under the pool's write lock.
type fakeHoldings struct {
	streams      int64
	watchers     int
	streamReads  int
	watcherReads int
	strict       testing.TB
}

// OpenListenStreams returns the fixed stream count.
func (h *fakeHoldings) OpenListenStreams() int64 {
	h.streamReads++
	return h.streams
}

// Watchers returns the fixed watcher count.
func (h *fakeHoldings) Watchers() int {
	h.watcherReads++
	if h.strict != nil && h.streams > 0 {
		h.strict.Errorf("Watchers read with %d listen streams open; the answer was already known", h.streams)
	}
	return h.watchers
}

// legacyBusy is the oracle [Busy] is held to: the body of credentialState.busy
// in cmd/server as it stood at cb6379f53 (cmd/server/credential.go:112-120),
// before the register decided it, copied verbatim over a [Holdings] fake.
//
// Two reads are spelled as the interface's methods: s.listen.count() is
// OpenListenStreams, and s.subs != nil && s.subs.manager.Len() is Watchers,
// which the server answers with 0 when it has no subscription runtime, as the
// nil check did. The guard for a nil state stays in the server, as
// s != nil && tenancy.Busy(s). It is kept apart from Busy on purpose: a later
// edit of the register that changes what busy means fails the tests below until
// this copy is edited too, which is what makes such an edit a visible change of
// policy (issue 565).
func legacyBusy(h Holdings) bool {
	if h.OpenListenStreams() > 0 {
		return true
	}
	return h.Watchers() > 0
}

// agreesWithLegacy fails the test unless Busy gives the replaced predicate's
// answer on the counts and reads each of them exactly as often, which is what
// keeps the manager's lock taken exactly when it was.
func agreesWithLegacy(t *testing.T, streams int64, watchers int) {
	t.Helper()
	legacy := &fakeHoldings{streams: streams, watchers: watchers}
	promoted := &fakeHoldings{streams: streams, watchers: watchers}
	if got, want := Busy(promoted), legacyBusy(legacy); got != want {
		t.Errorf("Busy(streams=%d, watchers=%d) = %v, the replaced predicate said %v", streams, watchers, got, want)
	}
	if promoted.streamReads != legacy.streamReads || promoted.watcherReads != legacy.watcherReads {
		t.Errorf("Busy(streams=%d, watchers=%d) read the streams %d and the watchers %d times, the replaced predicate %d and %d",
			streams, watchers, promoted.streamReads, promoted.watcherReads, legacy.streamReads, legacy.watcherReads)
	}
}

// TestBusy_AgreesWithTheReplacedPredicate holds Busy to the predicate it
// replaced over no, one and several open listen streams crossed with no, one
// and several watchers: the same answer, from the same reads in the same
// number. The answer and the reads being identical is the whole of why
// promoting it changed nothing (issue 565, plan L9).
func TestBusy_AgreesWithTheReplacedPredicate(t *testing.T) {
	for _, streams := range []int64{0, 1, 5} {
		for _, watchers := range []int{0, 1, 10} {
			t.Run(fmt.Sprintf("streams=%d/watchers=%d", streams, watchers), func(t *testing.T) {
				agreesWithLegacy(t, streams, watchers)
			})
		}
	}
}

// TestBusy_ReadsWatchersOnlyWhenNoStreamIsOpen covers the order of the two
// reads. On the server the watcher count is read behind the subscription
// manager's mutex, under the pool's write lock, so an open stream must answer
// the question on its own: the fake fails the test if the watchers are read
// while a stream is open, and the watchers must be read once when none is,
// since only then do they decide.
func TestBusy_ReadsWatchersOnlyWhenNoStreamIsOpen(t *testing.T) {
	for _, tc := range []struct {
		name      string
		streams   int64
		watchers  int
		want      bool
		readsWant int
	}{
		{"a stream open and no watcher", 1, 0, true, 0},
		{"several streams open and watchers", 5, 10, true, 0},
		{"no stream and a watcher", 0, 1, true, 1},
		{"no stream and no watcher", 0, 0, false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &fakeHoldings{streams: tc.streams, watchers: tc.watchers, strict: t}
			if got := Busy(h); got != tc.want {
				t.Errorf("Busy = %v, want %v", got, tc.want)
			}
			if h.streamReads != 1 || h.watcherReads != tc.readsWant {
				t.Errorf("read the streams %d and the watchers %d times, want 1 and %d", h.streamReads, h.watcherReads, tc.readsWant)
			}
		})
	}
}

// TestBusy_AllocatesNothing pins the register's half of the claim the server's
// own allocation test makes about the pool's question: reading the holdings
// through the interface allocates nothing.
func TestBusy_AllocatesNothing(t *testing.T) {
	h := &fakeHoldings{watchers: 1}
	if allocs := testing.AllocsPerRun(1000, func() { _ = Busy(h) }); allocs != 0 {
		t.Errorf("Busy allocates %v times per call, want 0", allocs)
	}
}

// TestBusy_IsTheRulePOL003Promotes ties the function to the row that names it:
// POL-003 is promoted, names Busy, and is decided per pool entry, which is
// what the holdings are counted for.
func TestBusy_IsTheRulePOL003Promotes(t *testing.T) {
	d, ok := Lookup("POL-003")
	if !ok {
		t.Fatal("no row POL-003")
	}
	if d.Disposition != Promoted || !has(d.Functions, "Busy") || d.Key != KeyEntry {
		t.Errorf("POL-003 = %+v, want a promoted row naming Busy, keyed on the entry", d)
	}
}

// FuzzBusy holds Busy to the replaced predicate on counts nobody listed,
// negative and extreme ones included, which no real counter reaches but which
// the comparison with zero has to answer the same way.
func FuzzBusy(f *testing.F) {
	for _, streams := range []int64{0, 1, 5, -1, math.MinInt64, math.MaxInt64} {
		for _, watchers := range []int{0, 1, 10, -1, math.MinInt, math.MaxInt} {
			f.Add(streams, watchers)
		}
	}
	f.Fuzz(agreesWithLegacy)
}
