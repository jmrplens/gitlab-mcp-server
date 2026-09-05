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

// TestOnceMap_ConcurrentCallersShareOneBuild verifies the property the caches
// exist for: a burst of callers for one cold key runs one build and every
// caller receives its result, rather than each building a copy and all but one
// being dropped. Written for the race detector; run it with make test-race.
func TestOnceMap_ConcurrentCallersShareOneBuild(t *testing.T) {
	t.Parallel()

	var cache OnceMap[string, *int]
	var builds atomic.Int32
	const callers = 16
	results := make([]*int, callers)
	release := make(chan struct{})
	var wg sync.WaitGroup
	for i := range callers {
		wg.Go(func() {
			<-release
			results[i] = cache.Load("one", func() *int {
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
