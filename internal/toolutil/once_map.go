package toolutil

import "sync"

// OnceMap memoizes one value per key and builds each value exactly once,
// however many callers ask for it at the same moment.
//
// The caches this replaces were all written as load, build, store-if-absent,
// which is correct and wasteful in the one situation they exist for: the HTTP
// pool starting a server per credential, where every one of them misses the
// same cold key, every one builds a full copy of a registry shape, a manifest
// snapshot or a reflected schema, and all but one are dropped having cost what
// the survivor cost. The entry carries the [sync.Once], so the callers that
// lose the race wait for the winner's build instead of racing it.
//
// The zero value is ready to use. The mutex is held only while the entry is
// looked up, never while a value is built, so builds for different keys still
// run in parallel.
type OnceMap[K comparable, V any] struct {
	mu      sync.Mutex
	entries map[K]*onceEntry[V]
}

// onceEntry is one key's slot: the value, the guard that builds it once, and
// whether that build has finished, which only [OnceMap.Peek] needs.
type onceEntry[V any] struct {
	once  sync.Once
	value V
	built bool
}

// Load returns the value memoized for key, calling build to produce it the
// first time the key is asked for. Concurrent callers for one key wait for
// that single build; callers for different keys build in parallel.
//
// A build that returns a zero value is memoized like any other, so a
// deterministic failure is not retried on every call. A build that panics
// memoizes nothing: the panic reaches its own caller unchanged and the key is
// left to be built again, which is what the load, build, store-if-absent
// caches this replaced did. [sync.Once] does not do that by itself, since it
// marks itself done however its function ends, so the spent slot is dropped
// by the next caller to find it holding nothing. Without that the key would
// be poisoned for the life of the process: the zero value served to everyone,
// no rebuild, and [OnceMap.Peek] reporting that nobody ever built it.
func (m *OnceMap[K, V]) Load(key K, build func() V) V {
	for {
		entry := m.entryFor(key)
		entry.once.Do(func() {
			value := build()
			// Written under the mutex so that Peek, which cannot rely on the
			// once for ordering, can read it without racing an in-flight build.
			m.mu.Lock()
			entry.value, entry.built = value, true
			m.mu.Unlock()
		})
		// Read without the mutex, as the memo always has: the once orders
		// the build's writes before the return of every Do that waited on
		// it. The mutex around the write is there for Peek, which has no
		// once to take that ordering from.
		if entry.built {
			return entry.value
		}
		// The once is spent and nothing was produced, which only a panicking
		// build leaves behind. Drop the slot and go round: this caller builds
		// the key itself rather than being served a value nobody ever made.
		// The loop terminates because the slot it drops is one whose build
		// has already ended, and the fresh one it takes next has a once that
		// has not run.
		m.forget(key, entry)
	}
}

// forget drops key's slot, so the next [OnceMap.Load] builds a fresh one. It
// removes the slot only while it is still the one passed in, because two
// callers released by one panicking build both come here: the second must
// not take away the slot the first has already put in its place.
func (m *OnceMap[K, V]) forget(key K, entry *onceEntry[V]) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.entries[key] == entry {
		delete(m.entries, key)
	}
}

// Peek returns the value memoized for key, and false when no build for that
// key has finished. It never builds one, which is what makes it the right
// question for a test asking whether a call populated the cache.
func (m *OnceMap[K, V]) Peek(key K) (V, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if entry, ok := m.entries[key]; ok && entry.built {
		return entry.value, true
	}
	var zero V
	return zero, false
}

// entryFor returns key's slot, creating it on first use.
func (m *OnceMap[K, V]) entryFor(key K) *onceEntry[V] {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.entries == nil {
		m.entries = make(map[K]*onceEntry[V])
	}
	entry, ok := m.entries[key]
	if !ok {
		entry = &onceEntry[V]{}
		m.entries[key] = entry
	}
	return entry
}
