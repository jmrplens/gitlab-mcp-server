package subscriptions

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"
)

// Errors a [Reader] may return to steer a watcher. Everything else is
// treated as a transient failure: logged, and retried on the next tick.
var (
	// ErrInaccessible means the resource can no longer be read — it was
	// deleted, or this token lost access to it. GitLab answers 404 for
	// both cases so as not to leak existence, and the watcher treats them
	// the same way: stop watching. Continuing would burn budget on a
	// resource that can never produce a notification, and would keep
	// polling with a token whose access was deliberately revoked.
	ErrInaccessible = errors.New("subscriptions: resource inaccessible")

	// ErrRateLimited means GitLab refused the read for rate-limiting
	// reasons. It pauses every watcher on this manager, not just the one
	// that hit it: the limit is enforced per user, so the others are
	// about to hit it too.
	ErrRateLimited = errors.New("subscriptions: rate limited")
)

// ErrNotSubscribable is returned by [Manager.Subscribe] for a URI outside
// the whitelist. The SDK does not check that a subscribed URI names a
// registered resource, so this is the only thing standing between a client
// and a subscription that can never fire.
var ErrNotSubscribable = errors.New("subscriptions: resource is not subscribable")

// ErrTooManySubscriptions is returned when a manager is already watching
// its configured maximum.
var ErrTooManySubscriptions = errors.New("subscriptions: too many active subscriptions")

// ErrClosed is returned once the manager has been shut down.
var ErrClosed = errors.New("subscriptions: manager is closed")

// Reasons reported to [Options.OnStop] — the ways a watch can end without
// the client having asked for it.
var (
	// ErrLifetimeExceeded means the watch hit [Options.MaxLifetime].
	ErrLifetimeExceeded = errors.New("subscriptions: maximum watch lifetime reached")

	// ErrEvicted means the watch was stopped to make room for one whose
	// subscriber is still active.
	ErrEvicted = errors.New("subscriptions: evicted to make room for an active subscription")
)

// Reader reads the current content of a resource URI. Implementations
// return [ErrInaccessible] or [ErrRateLimited] where those apply; any other
// error is treated as transient.
type Reader interface {
	// Read returns the current content of uri, or an error describing why
	// it could not be read.
	Read(ctx context.Context, uri string) ([]byte, error)
}

// Update describes a change worth telling a subscriber about, together with
// the state of the watch that noticed it.
//
// The watch state travels with the notification because there is nowhere
// else to put it: MCP defines no message for "your subscription slowed
// down" or "renew it by this time", and a notification is the only thing
// this server sends unprompted. A client that ignores all of it — every
// client known today does — still gets a correct notification.
type Update struct {
	// URI is the resource whose content changed.
	URI string
	// Slow reports that the watch is past its lease and now polling at
	// [Options.SlowInterval].
	Slow bool
	// RenewBy is when the current lease runs out and the watch slows down.
	// Any request on the session renews it.
	RenewBy time.Time
	// Interval is the cadence the watch is running at right now.
	Interval time.Duration
}

// Notifier delivers a resources/updated notification. Delivery is
// best-effort, matching MCP's own posture on notifications: a failure is
// logged and the watcher carries on.
type Notifier interface {
	// ResourceUpdated announces that a resource's content changed. The
	// error is advisory: the watcher logs it and carries on.
	ResourceUpdated(ctx context.Context, update Update) error
}

// Defaults for [Options]. The interval numbers come from GitLab's rate
// limits rather than from taste: a self-managed instance that enables the
// optional authenticated-API throttle gets 120 requests a minute by
// default (the throttle itself ships disabled — see ADR-0017), so ten
// watchers at a five-second interval would consume such a user's entire
// budget while that same user is making tool calls through it.
const (
	DefaultBaseInterval = 15 * time.Second
	DefaultMinInterval  = 5 * time.Second
	DefaultLease        = 30 * time.Minute
	DefaultSlowInterval = 10 * time.Minute
	DefaultMaxLifetime  = 24 * time.Hour
	DefaultMaxWatchers  = 10

	// Rate-limit back-off, applied to every watcher at once and doubled
	// per consecutive refusal.
	rateLimitBackoff    = 30 * time.Second
	maxRateLimitBackoff = 5 * time.Minute
	// jitterFraction spreads retries so watchers that were paused together
	// do not resume in lockstep and re-trigger the limit.
	jitterFraction = 0.2
)

// Options configures a [Manager]. The zero value is usable: every field
// falls back to its documented default.
type Options struct {
	// BaseInterval is the polling interval for a resource with no
	// lifecycle signal. Defaults to [DefaultBaseInterval].
	BaseInterval time.Duration
	// MinInterval is the floor for a resource with work in flight.
	// Defaults to [DefaultMinInterval].
	MinInterval time.Duration
	// Lease is how long a subscription is polled at full speed before it
	// slows down. Defaults to [DefaultLease].
	//
	// Reaching it demotes the watcher to [Options.SlowInterval]; it never
	// stops one. That distinction is the whole design: MCP has no message
	// that means "your subscription expired" — the specification defines no
	// lease, no TTL and no renewal, and the one notification available says
	// only "this resource changed, read it again" — so a watcher that
	// retired at the deadline would go silent in a way no client could tell
	// apart from "nothing has happened yet". Slowing down is a claim the
	// server can make honestly without saying anything at all.
	//
	// Any activity on the same session renews it, and so does
	// [Manager.Renew].
	Lease time.Duration
	// SlowInterval is the cadence of a demoted watcher. Defaults to
	// [DefaultSlowInterval].
	//
	// At ten minutes, ten abandoned subscriptions cost one request a
	// minute against the 120-a-minute budget of a throttled self-managed
	// instance — cheap enough to leave running, slow enough that nobody
	// would rely on it, which is what makes a renewal worth asking for.
	SlowInterval time.Duration
	// MaxLifetime is the absolute cap on a single subscription, renewals
	// included. Defaults to [DefaultMaxLifetime]. This is the only deadline
	// that truly stops a watcher on time alone.
	MaxLifetime time.Duration
	// MaxWatchers caps concurrent subscriptions. Defaults to
	// [DefaultMaxWatchers]. This is the real safety valve on API budget,
	// and it also bounds concurrent outbound requests, which nothing else
	// in this server does.
	MaxWatchers int
	// OnStop, if set, is called once when a watch ends for a reason the
	// subscriber did not ask for: [ErrInaccessible], [ErrLifetimeExceeded]
	// or [ErrEvicted]. It is not called when a client unsubscribes, when
	// its session ends, or when the manager is closed — in all three the
	// client either asked for it or is already gone.
	//
	// It exists so the transport layer can tell the client something, and
	// runs on the watcher's goroutine with no locks held.
	OnStop func(uri string, reason error)
	// Logger defaults to slog.Default().
	Logger *slog.Logger
}

func (o Options) withDefaults() Options {
	if o.BaseInterval <= 0 {
		o.BaseInterval = DefaultBaseInterval
	}
	if o.MinInterval <= 0 {
		o.MinInterval = DefaultMinInterval
	}
	if o.MinInterval > o.BaseInterval {
		o.MinInterval = o.BaseInterval
	}
	if o.Lease <= 0 {
		o.Lease = DefaultLease
	}
	if o.SlowInterval <= 0 {
		o.SlowInterval = DefaultSlowInterval
	}
	if o.SlowInterval < o.BaseInterval {
		// A "slow" cadence faster than the normal one would make demotion
		// a speed-up, which nothing in the design expects.
		o.SlowInterval = o.BaseInterval
	}
	if o.MaxLifetime <= 0 {
		o.MaxLifetime = DefaultMaxLifetime
	}
	if o.MaxLifetime < o.Lease {
		o.MaxLifetime = o.Lease
	}
	if o.MaxWatchers <= 0 {
		o.MaxWatchers = DefaultMaxWatchers
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	return o
}

// Manager owns the watchers for one MCP server, which in HTTP mode means
// one GitLab token: the server pool keys an *mcp.Server by token and URL,
// so a manager never spans two identities and a rate-limit pause or an
// access revocation applies cleanly to everything it owns.
type Manager[S comparable] struct {
	reader   Reader
	notifier Notifier
	opts     Options
	wg       sync.WaitGroup

	mu          sync.Mutex
	watchers    map[string]*watcher[S]
	closed      bool
	pausedUntil time.Time
	rateLimits  int // consecutive rate-limit refusals, for back-off growth
}

// watcher tracks one polled URI. Several MCP sessions may subscribe to the
// same URI; they share a single watcher and a single poll, since the SDK
// fans one ResourceUpdated call out to every subscribed session.
type watcher[S comparable] struct {
	uri  string
	kind Kind
	// subscribers holds who asked for this watch, by identity rather than
	// by count. A set is what makes the bookkeeping idempotent: one session
	// subscribing twice must not need two unsubscribes, and a session that
	// never subscribed must not be able to release somebody else's watch by
	// unsubscribing.
	subscribers map[S]struct{}
	// ready is closed once the first subscriber's read has been decided,
	// and readyErr says how it went. Later subscribers wait on it rather
	// than joining blind: until that read returns, nobody knows whether the
	// URI is readable at all, and a watcher that fails to start is one no
	// joiner should have been told succeeded.
	ready    chan struct{}
	readyErr error

	digest [sha256.Size]byte
	cancel context.CancelFunc

	// leaseAt is when full-speed polling ends and demotion begins. It moves
	// forward on renewal; the absolute cap lives on the watcher's context.
	leaseAt time.Time
	demoted bool
	// interval is the cadence the last read asked for, kept so a renewal
	// can restore it without waiting for another poll to recompute it.
	interval time.Duration
	// renew wakes the poll loop when a demoted watcher is revived. Buffered
	// so a renewal never blocks on a watcher that is mid-read.
	renew chan struct{}
	// evicted records that this watcher was stopped to free a slot, which
	// is a cancellation the subscriber did not ask for.
	evicted bool
}

// New creates a manager. Call [Manager.Close] to stop every watcher; the
// manager is unusable afterwards.
func New[S comparable](reader Reader, notifier Notifier, opts Options) *Manager[S] {
	return &Manager[S]{
		reader:   reader,
		notifier: notifier,
		opts:     opts.withDefaults(),
		watchers: make(map[string]*watcher[S]),
	}
}

// Subscribe starts watching uri on behalf of subscriber, or joins the
// watcher already watching it.
//
// The first subscriber's read happens synchronously, on the caller's
// context, and doubles as the authorization check: it runs with the
// subscriber's own token, so a URI this token cannot read is refused here
// rather than being accepted and then failing silently forever. It also
// establishes the baseline that later polls are compared against, so a
// subscriber is never notified about a change that predates its
// subscription.
//
// A later subscriber waits for that read rather than joining blind, and
// receives the same answer. It does not read again: every session on one
// manager shares one token by construction — see [Manager] — so the first
// check answered the question for all of them. What it must not do is
// return success while the only read anyone attempted was still in flight,
// or had already failed.
//
// Subscribing twice to the same URI as the same subscriber is a no-op: the
// caller is asking for a state it already holds.
func (m *Manager[S]) Subscribe(ctx context.Context, subscriber S, uri string) error {
	kind, ok := Classify(uri)
	if !ok {
		return fmt.Errorf("%w: %s", ErrNotSubscribable, uri)
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return ErrClosed
	}
	if w, exists := m.watchers[uri]; exists {
		w.subscribers[subscriber] = struct{}{}
		count := len(w.subscribers)
		m.mu.Unlock()
		m.opts.Logger.Debug("subscription joined an existing watcher",
			"uri", uri, "kind", kind.String(), "subscribers", count)
		return m.awaitStart(ctx, w, subscriber)
	}
	if len(m.watchers) >= m.opts.MaxWatchers && !m.evictDemotedLocked() {
		m.mu.Unlock()
		return fmt.Errorf("%w: %d watching, limit %d", ErrTooManySubscriptions, len(m.watchers), m.opts.MaxWatchers)
	}
	// Reserve the slot before releasing the lock so two concurrent
	// subscribes cannot both pass the cap check.
	w := &watcher[S]{
		uri:         uri,
		kind:        kind,
		subscribers: map[S]struct{}{subscriber: {}},
		ready:       make(chan struct{}),
		renew:       make(chan struct{}, 1),
	}
	m.watchers[uri] = w
	m.mu.Unlock()

	return m.start(ctx, w)
}

// start performs the first read and either launches the poll loop or
// abandons the watcher, releasing anyone waiting on it either way.
func (m *Manager[S]) start(ctx context.Context, w *watcher[S]) error {
	content, err := m.reader.Read(ctx, w.uri)

	// The deadline is the absolute cap, not the lease: reaching the lease
	// slows a watcher down, and only [Options.MaxLifetime] ends it. The
	// context roots at Background rather than anything request-scoped —
	// the whole point of a subscription is to outlive the request that
	// created it — and Close stops the watcher through its cancel func,
	// which the registry holds for exactly that purpose.
	watchCtx, cancel := context.WithTimeout(context.Background(), m.opts.MaxLifetime)

	m.mu.Lock()
	switch {
	case err != nil:
		startErr := fmt.Errorf("subscriptions: initial read of %s: %w", w.uri, err)
		m.abandonLocked(w, startErr)
		m.mu.Unlock()
		cancel()
		return startErr

	case m.closed:
		m.abandonLocked(w, ErrClosed)
		m.mu.Unlock()
		cancel()
		return ErrClosed

	case m.watchers[w.uri] != w:
		// Every subscriber withdrew while the read was in flight, or the
		// manager was closed and reopened around it. Launching now would
		// produce a watcher no Unsubscribe could ever reach, since nothing
		// holds a reference to it any more.
		m.abandonLocked(w, nil)
		m.mu.Unlock()
		cancel()
		m.opts.Logger.Debug("subscription withdrawn before its first read finished", "uri", w.uri)
		return nil
	}

	first := m.kindInterval(w.kind, content)
	w.digest = sha256.Sum256(content)
	w.cancel = cancel
	w.leaseAt = time.Now().Add(m.opts.Lease)
	w.interval = first
	close(w.ready)
	m.mu.Unlock()

	m.wg.Add(1)
	// The watcher deliberately does not inherit the caller's context. That
	// context ends when the subscribe request returns, whereas the whole
	// point of a subscription is to outlive it; the watcher's lifetime is
	// bounded by the absolute cap instead.
	go m.watch(watchCtx, w, first) //nolint:contextcheck // see above

	m.opts.Logger.Debug("subscription started",
		"uri", w.uri, "kind", w.kind.String(), "lease", m.opts.Lease)
	return nil
}

// abandonLocked drops a watcher that never started and releases whoever is
// waiting on it. m.mu must be held.
func (m *Manager[S]) abandonLocked(w *watcher[S], reason error) {
	if m.watchers[w.uri] == w {
		delete(m.watchers, w.uri)
	}
	w.readyErr = reason
	close(w.ready)
}

// awaitStart blocks until the watcher's first read has been decided, and
// reports its outcome to a subscriber that joined while it was in flight.
func (m *Manager[S]) awaitStart(ctx context.Context, w *watcher[S], subscriber S) error {
	select {
	case <-w.ready:
	case <-ctx.Done():
		// The joiner gave up. Its own interest has to go with it, or the
		// watcher would outlive every subscriber that can still act on it.
		_ = m.Unsubscribe(subscriber, w.uri)
		return ctx.Err()
	}

	m.mu.Lock()
	startErr := w.readyErr
	m.mu.Unlock()
	return startErr
}

// Unsubscribe drops one subscriber's interest in uri. The watcher stops
// once the last subscriber leaves.
//
// Unsubscribing something this subscriber does not hold is not an error and
// has no effect: a client may unsubscribe twice, or after a watch already
// ended on its own, and neither may disturb a watch somebody else is
// holding.
func (m *Manager[S]) Unsubscribe(subscriber S, uri string) error {
	m.mu.Lock()
	w, exists := m.watchers[uri]
	if !exists {
		m.mu.Unlock()
		return nil
	}
	if _, held := w.subscribers[subscriber]; !held {
		m.mu.Unlock()
		return nil
	}
	delete(w.subscribers, subscriber)
	if len(w.subscribers) > 0 {
		count := len(w.subscribers)
		m.mu.Unlock()
		m.opts.Logger.Debug("subscriber left, watcher still in use", "uri", uri, "subscribers", count)
		return nil
	}
	delete(m.watchers, uri)
	cancel := w.cancel
	m.mu.Unlock()

	// A watcher still doing its first read has no cancel func yet; start
	// finds its map entry gone and abandons it.
	if cancel != nil {
		cancel()
	}
	m.opts.Logger.Debug("subscription stopped, no subscribers left", "uri", uri)
	return nil
}

// UnsubscribeAll drops every subscription one subscriber holds and reports
// how many watchers that stopped.
//
// This is what a closing MCP session calls. The SDK drops a disconnected
// session from its own subscriber table without ever invoking the
// unsubscribe handler, so without this a watch would outlive the only
// client that could receive its notifications.
func (m *Manager[S]) UnsubscribeAll(subscriber S) int {
	m.mu.Lock()
	stopped := make([]*watcher[S], 0, len(m.watchers))
	for uri, w := range m.watchers {
		if _, held := w.subscribers[subscriber]; !held {
			continue
		}
		delete(w.subscribers, subscriber)
		if len(w.subscribers) == 0 {
			delete(m.watchers, uri)
			stopped = append(stopped, w)
		}
	}
	m.mu.Unlock()

	for _, w := range stopped {
		if w.cancel != nil {
			w.cancel()
		}
	}
	if len(stopped) > 0 {
		m.opts.Logger.Debug("subscriber left, stopping its watchers", "watchers", len(stopped))
	}
	return len(stopped)
}

// Close stops every watcher and waits for them to finish. It is safe to
// call more than once.
//
// This is the explicit stop, for an owner shutting the manager down. It is
// not what bounds a watcher in normal operation: a subscription ends when
// the session that asked for it disconnects, slows down when its lease runs
// out unrenewed, and stops for good at [Options.MaxLifetime] or the first
// read that says the resource is gone. In HTTP mode the server pool never
// calls this — it lets an evicted entry expire with its sessions rather
// than terminating one that may still be serving a live connection, and by
// the time the last session on that server ends there is nothing left to
// watch.
func (m *Manager[S]) Close() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	// Collect every running watcher's cancel under the lock. A watcher
	// whose first read is still in flight has no cancel yet, and needs
	// none: start re-checks closed under this same lock and abandons the
	// launch.
	cancels := make([]context.CancelFunc, 0, len(m.watchers))
	for _, w := range m.watchers {
		if w.cancel != nil {
			cancels = append(cancels, w.cancel)
		}
	}
	m.watchers = make(map[string]*watcher[S])
	m.mu.Unlock()

	for _, cancel := range cancels {
		cancel()
	}
	m.wg.Wait()
}

// Len reports how many URIs are currently watched.
func (m *Manager[S]) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.watchers)
}

// watch polls one URI until its lease expires, its last subscriber leaves,
// or the manager closes.
func (m *Manager[S]) watch(ctx context.Context, w *watcher[S], first time.Duration) {
	defer m.wg.Done()
	// Releasing the context is not housekeeping: it carries the absolute
	// lifetime timer, which would otherwise stay armed for up to 24 hours
	// after a watcher stopped for some other reason.
	defer w.cancel()

	timer := time.NewTimer(first)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			m.opts.Logger.Debug("watcher stopped", "uri", w.uri, "reason", ctx.Err())
			m.retire(w, ctx.Err())
			return
		case <-w.renew:
			// A demoted watcher was revived: resume at once rather than
			// sitting out the rest of a ten-minute sleep, which would
			// make renewal look like it did nothing.
			timer.Stop()
			timer.Reset(m.resumeInterval(w))
			continue
		case <-timer.C:
		}

		// A rate-limit pause is manager-wide, so a watcher that was
		// asleep through one still has to honor the remainder.
		if wait := m.remainingPause(); wait > 0 {
			timer.Reset(wait)
			continue
		}

		next, stopReason := m.poll(ctx, w)
		if stopReason != nil {
			m.retire(w, stopReason)
			return
		}
		timer.Reset(m.applyLease(w, next))
	}
}

// retire removes a watcher from the registry and tells the transport layer
// why it ended, when the ending is one its subscribers did not ask for.
//
// Removal happens under the lock together with the decision to stop, so
// there is no window in which a subscriber could join a watcher that is
// already unwinding and be told it succeeded.
//
// An ordinary cancellation is silent on purpose: it means Unsubscribe, a
// closed session, or the manager shutting down, and in each of those the
// client either requested the stop or is no longer there to hear about it.
func (m *Manager[S]) retire(w *watcher[S], cause error) {
	m.mu.Lock()
	if m.watchers[w.uri] == w {
		delete(m.watchers, w.uri)
	}
	evicted := w.evicted
	m.mu.Unlock()

	switch {
	case errors.Is(cause, ErrInaccessible):
		m.announceStop(w.uri, ErrInaccessible)
	case errors.Is(cause, context.DeadlineExceeded):
		m.announceStop(w.uri, ErrLifetimeExceeded)
	case evicted:
		m.announceStop(w.uri, ErrEvicted)
	}
}

// announceStop hands a stop reason to the transport layer, if anyone is
// listening. It runs with no locks held: the callback reaches back into the
// MCP server, which must never be called under the manager's mutex.
func (m *Manager[S]) announceStop(uri string, reason error) {
	if m.opts.OnStop != nil {
		m.opts.OnStop(uri, reason)
	}
}

// snapshot captures the watch state that travels with a notification.
func (m *Manager[S]) snapshot(w *watcher[S]) Update {
	m.mu.Lock()
	defer m.mu.Unlock()
	update := Update{URI: w.uri, Slow: w.demoted, RenewBy: w.leaseAt, Interval: w.interval}
	if w.demoted {
		update.Interval = m.opts.SlowInterval
	}
	if update.Interval <= 0 {
		update.Interval = m.opts.BaseInterval
	}
	return update
}

// applyLease demotes a watcher whose lease has run out, and reports how long
// it should actually wait before its next read.
func (m *Manager[S]) applyLease(w *watcher[S], next time.Duration) time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !w.demoted && !w.leaseAt.IsZero() && !time.Now().Before(w.leaseAt) {
		w.demoted = true
		m.opts.Logger.Info("subscription lease expired, slowing to a background poll",
			"uri", w.uri, "kind", w.kind.String(), "interval", m.opts.SlowInterval)
	}
	if w.demoted {
		return m.opts.SlowInterval
	}
	return next
}

// resumeInterval reports the cadence a revived watcher should return to.
func (m *Manager[S]) resumeInterval(w *watcher[S]) time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	if w.interval > 0 {
		return w.interval
	}
	return m.opts.BaseInterval
}

// Renew extends the lease on one URI, restoring full-speed polling if it had
// slowed. It reports whether anything was watching that URI.
func (m *Manager[S]) Renew(uri string) bool {
	m.mu.Lock()
	w, ok := m.watchers[uri]
	revived := ok && m.renewLocked(w)
	m.mu.Unlock()
	if revived {
		wake(w)
	}
	return ok
}

// RenewAll extends the lease on every watch one subscriber holds, and
// reports how many of them that revived from a demoted state.
//
// This is what ties a subscription's lifetime to its subscriber being
// present: any request on a session renews everything that session could be
// waiting on, and nothing else. Renewing per-URI on reads of that URI would
// be worse than useless — a watcher only notifies on a real change, so a
// quiet resource produces no notification, no re-read, and would expire
// during exactly the wait its subscriber cared about. Renewing every watch
// on the manager would be wrong in the other direction: one busy session
// would keep another session's abandoned watches at full speed forever.
func (m *Manager[S]) RenewAll(subscriber S) int {
	m.mu.Lock()
	revived := make([]*watcher[S], 0, len(m.watchers))
	for _, w := range m.watchers {
		if _, held := w.subscribers[subscriber]; !held {
			continue
		}
		if m.renewLocked(w) {
			revived = append(revived, w)
		}
	}
	m.mu.Unlock()

	for _, w := range revived {
		wake(w)
	}
	return len(revived)
}

// renewLocked pushes a watcher's lease out and reports whether that revived
// a demoted one. m.mu must be held.
func (m *Manager[S]) renewLocked(w *watcher[S]) bool {
	w.leaseAt = time.Now().Add(m.opts.Lease)
	if !w.demoted {
		return false
	}
	w.demoted = false
	m.opts.Logger.Info("subscription renewed, resuming normal polling",
		"uri", w.uri, "kind", w.kind.String())
	return true
}

// wake nudges a watcher's poll loop without blocking: the channel is
// buffered, and a pending nudge is as good as a second one.
func wake[S comparable](w *watcher[S]) {
	select {
	case w.renew <- struct{}{}:
	default:
	}
}

// evictDemotedLocked makes room for a new subscription by stopping the
// longest-demoted watcher, reporting whether it found one. m.mu must be
// held.
//
// Without this, watchers that nobody renewed would hold every slot for as
// long as their absolute lifetime allows, and a client asking to watch
// something it is actively waiting on would be refused in favor of watches
// its own inactivity already devalued.
func (m *Manager[S]) evictDemotedLocked() bool {
	var oldest *watcher[S]
	for _, w := range m.watchers {
		if !w.demoted {
			continue
		}
		if oldest == nil || w.leaseAt.Before(oldest.leaseAt) {
			oldest = w
		}
	}
	if oldest == nil {
		return false
	}
	delete(m.watchers, oldest.uri)
	oldest.evicted = true
	if oldest.cancel != nil {
		oldest.cancel()
	}
	m.opts.Logger.Info("evicted a demoted subscription to make room",
		"uri", oldest.uri, "kind", oldest.kind.String())
	return true
}

// poll performs one read and reports how long to wait before the next one,
// and whether the watcher should stop entirely.
func (m *Manager[S]) poll(ctx context.Context, w *watcher[S]) (next time.Duration, stopReason error) {
	content, err := m.reader.Read(ctx, w.uri)
	switch {
	case err == nil:
		m.clearRateLimit()

	case errors.Is(err, ErrInaccessible):
		m.opts.Logger.Info("watcher stopping: resource is gone or access was revoked",
			"uri", w.uri, "kind", w.kind.String(), "error", err)
		return 0, err

	case errors.Is(err, ErrRateLimited):
		wait := m.recordRateLimit()
		m.opts.Logger.Warn("rate limited by GitLab, pausing every watcher",
			"uri", w.uri, "pause", wait, "error", err)
		// Deliberately not recorded as this watcher's cadence: a back-off
		// is how long to wait before trying again, not how often this
		// resource is being watched, and a subscriber told the latter
		// would be told something untrue.
		return wait, nil

	case ctx.Err() != nil:
		// The lease expired or the manager closed mid-read. Reporting no
		// stop reason here is deliberate: the select at the top of the
		// loop sees the same cancellation and retires the watcher with the
		// cause, where this branch could only guess at it.
		//nolint:nilerr // the cancellation is reported by the caller's select, not here
		return m.opts.BaseInterval, nil

	default:
		// Transient: a network blip, a 500, a decode failure. Keep the
		// subscription alive and try again on the normal cadence — the
		// whole point of polling as the floor is that one lost read costs
		// latency, not correctness.
		m.opts.Logger.Debug("watcher read failed, will retry",
			"uri", w.uri, "error", err)
		return m.opts.BaseInterval, nil
	}

	digest := sha256.Sum256(content)
	if m.updateDigest(w, digest) {
		if notifyErr := m.notifier.ResourceUpdated(ctx, m.snapshot(w)); notifyErr != nil {
			// Best-effort by design: the baseline still advances, so a
			// client that missed one notification is not told about the
			// same change forever.
			m.opts.Logger.Debug("resource updated notification failed",
				"uri", w.uri, "error", notifyErr)
		}
	}
	cadence := m.kindInterval(w.kind, content)
	m.setInterval(w, cadence)
	return cadence, nil
}

// setInterval records the cadence a successful read asked for, so a
// notification can report how often the resource is really being watched.
func (m *Manager[S]) setInterval(w *watcher[S], cadence time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w.interval = cadence
}

// updateDigest stores the new digest and reports whether the content
// changed. It returns false once the watcher has been removed, so a poll
// racing with Unsubscribe cannot notify after the last subscriber left.
func (m *Manager[S]) updateDigest(w *watcher[S], digest [sha256.Size]byte) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.watchers[w.uri] != w {
		return false
	}
	if bytes.Equal(w.digest[:], digest[:]) {
		return false
	}
	w.digest = digest
	return true
}

// kindInterval returns the cadence for a resource given the content its
// last read produced.
func (m *Manager[S]) kindInterval(kind Kind, content []byte) time.Duration {
	return kind.pollInterval(content, m.opts.BaseInterval, m.opts.MinInterval)
}

// recordRateLimit escalates the manager-wide pause and returns how long
// this watcher should wait.
func (m *Manager[S]) recordRateLimit() time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rateLimits++
	backoff := rateLimitBackoff << min(m.rateLimits-1, 8)
	// Clamp on both sides of the jitter: capping first alone would let
	// upward jitter carry the result past the ceiling the constant
	// promises, and jittering an uncapped value could overflow it wildly.
	backoff = min(jitter(min(backoff, maxRateLimitBackoff)), maxRateLimitBackoff)
	m.pausedUntil = time.Now().Add(backoff)
	return backoff
}

// clearRateLimit resets the back-off after a read succeeds, unless a pause
// is still in force.
//
// The guard is what makes the pause manager-wide rather than per-watcher.
// Watchers poll concurrently, so a read that was already in flight can
// succeed moments after another watcher was refused; without the guard that
// success would wipe the pause the refusal just set, and the remaining
// watchers would carry on straight into the same limit. It also keeps the
// escalation honest: a success during a pause must not reset the streak, or
// a repeatedly-limited manager would back off by the same 30 seconds
// forever instead of doubling.
func (m *Manager[S]) clearRateLimit() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if time.Now().Before(m.pausedUntil) {
		return
	}
	m.rateLimits = 0
	m.pausedUntil = time.Time{}
}

// remainingPause reports how much of a manager-wide rate-limit pause is
// left, or zero when there is none.
func (m *Manager[S]) remainingPause() time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pausedUntil.IsZero() {
		return 0
	}
	return max(time.Until(m.pausedUntil), 0)
}

// jitter spreads a duration by ±jitterFraction so watchers paused together
// do not resume in lockstep.
func jitter(d time.Duration) time.Duration {
	spread := float64(d) * jitterFraction
	delta := (rand.Float64()*2 - 1) * spread //nolint:gosec // scheduling jitter, not a security decision
	out := time.Duration(float64(d) + delta)
	if out <= 0 {
		return d
	}
	return out
}
