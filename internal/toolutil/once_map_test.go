package toolutil

import (
	"sync"
	"sync/atomic"
	"testing"
)

// TestOnceMap_BuildsOncePerKeyAndServesTheResult verifies the memo itself: a
// key is built the first time it is asked for and served from then on,
// different keys build independently, and a zero value is memoized like any
// other rather than rebuilt on every call.
func TestOnceMap_BuildsOncePerKeyAndServesTheResult(t *testing.T) {
	t.Parallel()

	var cache OnceMap[string, string]
	var builds atomic.Int32
	build := func(value string) func() string {
		return func() string {
			builds.Add(1)
			return value
		}
	}

	if got := cache.Load("a", build("first")); got != "first" {
		t.Errorf("Load(a) = %q, want the built value", got)
	}
	if got := cache.Load("a", build("second")); got != "first" {
		t.Errorf("Load(a) after the build = %q, want the memoized value", got)
	}
	if got := cache.Load("b", build("other")); got != "other" {
		t.Errorf("Load(b) = %q, want its own build", got)
	}
	if got := builds.Load(); got != 2 {
		t.Errorf("builds = %d, want one per key", got)
	}

	// A build that returns the zero value is still a build: a deterministic
	// failure must not be retried on every call.
	if got := cache.Load("empty", build("")); got != "" {
		t.Errorf("Load(empty) = %q, want the zero value", got)
	}
	if got := cache.Load("empty", build("late")); got != "" {
		t.Errorf("Load(empty) again = %q, want the memoized zero value", got)
	}
	if got := builds.Load(); got != 3 {
		t.Errorf("builds = %d, want the zero value memoized rather than rebuilt", got)
	}
}

// TestOnceMap_Peek_ReportsOnlyFinishedBuilds verifies the read a test uses to
// ask whether a call populated the cache: it never builds, and it answers
// false for a key nobody has built.
func TestOnceMap_Peek_ReportsOnlyFinishedBuilds(t *testing.T) {
	t.Parallel()

	var cache OnceMap[string, int]
	if value, ok := cache.Peek("absent"); ok || value != 0 {
		t.Errorf("Peek(absent) = %d, %t; want the zero value and false", value, ok)
	}
	cache.Load("present", func() int { return 7 })
	if value, ok := cache.Peek("present"); !ok || value != 7 {
		t.Errorf("Peek(present) = %d, %t; want 7 and true", value, ok)
	}
	if _, ok := cache.Peek("still absent"); ok {
		t.Error("Peek() reported a key it was never asked to build")
	}
}

// arrivalGate returns a channel closed once callers goroutines have each
// reported arriving at the call under test, together with the report they
// make. A build closure that waits on the channel therefore holds the memo
// open until every caller is at its own call, which is what makes a burst of
// callers contend rather than queue behind a build that is already over.
//
// The gate is the arrival at the call, not the entry into the build closure,
// because a correct memo lets exactly one caller into the closure: a barrier
// waiting for the others there would wait for callers that will never come.
// The remaining gap is a caller descheduled between its report and the call
// itself, which no barrier reachable from outside the memo can close; with
// this many callers a regression would have to lose every one of them in that
// window to escape.
func arrivalGate(callers int) (<-chan struct{}, func()) {
	var arrived sync.WaitGroup
	arrived.Add(callers)
	gate := make(chan struct{})
	go func() {
		arrived.Wait()
		close(gate)
	}()
	return gate, arrived.Done
}

// TestOnceMap_ConcurrentCallersShareOneBuild verifies the property the caches
// exist for: a burst of callers for one cold key runs one build and every
// caller receives its result, rather than each building a copy and all but one
// being dropped. Written for the race detector; run it with make test-race.
//
// The build waits on an [arrivalGate] so the callers really are concurrent.
// Without it the winner's build is over before most of them reach Load, and
// rewriting Load as load, build, store-if-absent — the regression this test
// exists for — was caught 6 runs in 100.
func TestOnceMap_ConcurrentCallersShareOneBuild(t *testing.T) {
	t.Parallel()

	var cache OnceMap[string, *int]
	var builds atomic.Int32
	const callers = 16
	results := make([]*int, callers)
	release := make(chan struct{})
	allArrived, arrive := arrivalGate(callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Go(func() {
			<-release
			arrive()
			results[i] = cache.Load("one", func() *int {
				<-allArrived
				n := int(builds.Add(1))
				return &n
			})
		})
	}
	close(release)
	wg.Wait()

	if got := builds.Load(); got != 1 {
		t.Fatalf("builds = %d, want one build shared by every caller", got)
	}
	for i := 1; i < callers; i++ {
		if results[i] != results[0] {
			t.Fatalf("caller %d received a different value from caller 0", i)
		}
	}
}
