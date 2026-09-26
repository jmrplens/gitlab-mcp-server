package tenancy

// Holdings is what a pool entry holds that the pool cannot see: the work that
// keeps a credential busy while it sends no request (register row POL-003).
//
// The server's per-credential state implements it; the counters, the manager
// and the idle sweep stay where they are. What lives here is which holdings
// make an entry busy: no longer counting one is an edit to the register and to
// its oracle test, and counting a third is that plus a method here, which the
// server's state then implements (an accessor in the layer, never a predicate).
type Holdings interface {
	// OpenListenStreams is how many subscriptions/listen streams the entry
	// holds open, the count the per-credential listen ceiling draws on
	// (HLD-001). It is counted whether or not a ceiling is configured.
	OpenListenStreams() int64
	// Watchers is how many URIs the entry's subscription manager watches, or
	// zero on a surface that offers no subscriptions. It may take the
	// manager's lock, which is why [Busy] asks it only when no stream is open.
	Watchers() int
}

// Busy reports whether an entry is doing work the pool cannot see: holding an
// open listen stream, or at least one watcher (POL-003).
//
// A watcher polls GitLab on its own, and an open subscriptions/listen is a
// request the client holds rather than repeats, so neither refreshes the pool
// entry that owns them. The pool asks this before it evicts an idle entry, and
// prefers an entry that is not busy under size pressure.
//
// The stream count is read first, and the watcher count only when no stream is
// open. The pool asks under its write lock, where the one lock this may take
// is the manager's, so reading the watchers only when the answer still depends
// on them is what keeps that lock taken exactly as often as it was when the
// server decided this itself. It allocates nothing. It is not inlined, since
// two calls through the interface cost more than the compiler's inlining
// budget; the predicate it replaced was not inlined either, so what the pool
// pays beyond it is one or two dynamic dispatches per entry it asks about,
// where that predicate read its counters directly.
func Busy(h Holdings) bool {
	return h.OpenListenStreams() > 0 || h.Watchers() > 0
}
