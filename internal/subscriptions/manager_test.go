// manager_test.go validates the watcher lifecycle: what a subscription
// costs to start, when it notifies, when it backs off, and — the part that
// actually keeps the server honest — every way it must stop.
//
// Time is faked with testing/synctest rather than shortened with tiny real
// durations. That keeps leases, back-off windows and poll cadences at their
// production values in the assertions while still running instantly, and it
// removes the usual source of flakiness in tests like these.
package subscriptions

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

// fakeReader serves scripted content and errors for one or more URIs, and
// records how many reads each URI received.
type fakeReader struct {
	mu      sync.Mutex
	content map[string][]byte
	err     map[string]error
	reads   map[string]int
}

func newFakeReader() *fakeReader {
	return &fakeReader{
		content: map[string][]byte{},
		err:     map[string]error{},
		reads:   map[string]int{},
	}
}

func (f *fakeReader) Read(_ context.Context, uri string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads[uri]++
	if err := f.err[uri]; err != nil {
		return nil, err
	}
	if c, ok := f.content[uri]; ok {
		return c, nil
	}
	return []byte(`{"id":1}`), nil
}

func (f *fakeReader) set(uri, content string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.content[uri] = []byte(content)
}

func (f *fakeReader) fail(uri string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err[uri] = err
}

func (f *fakeReader) heal(uri string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.err, uri)
}

func (f *fakeReader) readCount(uri string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reads[uri]
}

// fakeNotifier records every notified URI.
type fakeNotifier struct {
	mu   sync.Mutex
	uris []string
	last Update
	err  error
}

func (n *fakeNotifier) ResourceUpdated(_ context.Context, update Update) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.uris = append(n.uris, update.URI)
	n.last = update
	return n.err
}

// last returns the most recent update, for tests that assert on the watch
// state a notification carries.
func (n *fakeNotifier) latest() Update {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.last
}

func (n *fakeNotifier) count() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.uris)
}

// quietOptions returns options with logging discarded, so a test's output
// is its assertions rather than the watcher's debug chatter.
func quietOptions(o Options) Options {
	o.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	return o
}

// newTestManager wires a manager onto fresh fakes.
func newTestManager(t *testing.T, o Options) (*Manager[string], *fakeReader, *fakeNotifier) {
	t.Helper()
	r, n := newFakeReader(), &fakeNotifier{}
	m := New[string](r, n, quietOptions(o))
	t.Cleanup(m.Close)
	return m, r, n
}

// subA and subB stand in for two MCP sessions. Identity is what the manager
// counts by, so tests that only need "somebody" use subA throughout.
const (
	subA = "session-a"
	subB = "session-b"
)

const testURI = "gitlab://project/42/pipeline/99"

func TestSubscribe_UnsubscribableURI_IsRejectedWithoutReading(t *testing.T) {
	m, r, _ := newTestManager(t, Options{})

	err := m.Subscribe(context.Background(), subA, "gitlab://project/42/issues")
	if !errors.Is(err, ErrNotSubscribable) {
		t.Fatalf("Subscribe(collection) error = %v, want ErrNotSubscribable", err)
	}
	if got := r.readCount("gitlab://project/42/issues"); got != 0 {
		t.Errorf("reads = %d, want 0 — a rejected URI must not cost an API call", got)
	}
	if m.Len() != 0 {
		t.Errorf("Len() = %d, want 0", m.Len())
	}
}

// TestSubscribe_InitialReadFails_SubscriptionIsRefused verifies the
// synchronous first read doubles as the authorization check.
//
// Accepting a subscription the token cannot read is the worst outcome
// available: the client is told it succeeded, then waits forever for a
// notification that can never come.
func TestSubscribe_InitialReadFails_SubscriptionIsRefused(t *testing.T) {
	m, r, _ := newTestManager(t, Options{})
	r.fail(testURI, ErrInaccessible)

	err := m.Subscribe(context.Background(), subA, testURI)
	if err == nil {
		t.Fatal("Subscribe() error = nil, want the read failure surfaced")
	}
	if !errors.Is(err, ErrInaccessible) {
		t.Errorf("Subscribe() error = %v, want it to wrap ErrInaccessible", err)
	}
	if m.Len() != 0 {
		t.Errorf("Len() = %d, want 0 — a refused subscription must leave no watcher", m.Len())
	}
}

func TestSubscribe_AtCapacity_IsRejected(t *testing.T) {
	m, _, _ := newTestManager(t, Options{MaxWatchers: 2})
	ctx := context.Background()

	for i, uri := range []string{
		"gitlab://project/42/pipeline/1",
		"gitlab://project/42/pipeline/2",
	} {
		if err := m.Subscribe(ctx, subA, uri); err != nil {
			t.Fatalf("Subscribe(#%d) error: %v", i, err)
		}
	}

	err := m.Subscribe(ctx, subA, "gitlab://project/42/pipeline/3")
	if !errors.Is(err, ErrTooManySubscriptions) {
		t.Fatalf("Subscribe() past the cap = %v, want ErrTooManySubscriptions", err)
	}
	if m.Len() != 2 {
		t.Errorf("Len() = %d, want 2", m.Len())
	}
}

// TestSubscribe_SameURITwice_SharesOneWatcher verifies coalescing: two
// sessions watching one URI cost one poll, not two.
func TestSubscribe_SameURITwice_SharesOneWatcher(t *testing.T) {
	m, r, _ := newTestManager(t, Options{})
	ctx := context.Background()

	if err := m.Subscribe(ctx, subA, testURI); err != nil {
		t.Fatalf("first Subscribe: %v", err)
	}
	if err := m.Subscribe(ctx, subB, testURI); err != nil {
		t.Fatalf("second Subscribe: %v", err)
	}

	if m.Len() != 1 {
		t.Errorf("Len() = %d, want 1 — both subscribers share one watcher", m.Len())
	}
	if got := r.readCount(testURI); got != 1 {
		t.Errorf("reads = %d, want 1 — joining an existing watcher must not re-read", got)
	}
}

// TestSubscribe_SameSubscriberTwice_IsIdempotent verifies a session that
// asks twice for the same URI holds it once.
//
// Counting it twice would mean the client had to unsubscribe twice to stop
// something it only asked for once, and a client that unsubscribed the
// normal single time would leave a watch nothing could ever release. The
// SDK makes this easy to hit: on protocol 2026-07-28 every Subscribe opens
// its own subscriptions/listen stream.
func TestSubscribe_SameSubscriberTwice_IsIdempotent(t *testing.T) {
	m, _, _ := newTestManager(t, Options{})
	ctx := context.Background()

	for range 2 {
		if err := m.Subscribe(ctx, subA, testURI); err != nil {
			t.Fatalf("Subscribe: %v", err)
		}
	}
	if err := m.Unsubscribe(subA, testURI); err != nil {
		t.Fatalf("Unsubscribe: %v", err)
	}

	if m.Len() != 0 {
		t.Errorf("Len() = %d after one unsubscribe, want 0 — the duplicate subscribe left a hold behind", m.Len())
	}
}

// TestUnsubscribe_ForeignSubscriber_LeavesTheWatchAlone verifies one
// session cannot release another's watch.
//
// Without identity this is a real cross-session defect rather than a
// hygiene concern: an unsubscribe from a session that never subscribed
// would decrement somebody else's hold and, at a count of one, stop a watch
// its actual subscriber is still waiting on.
func TestUnsubscribe_ForeignSubscriber_LeavesTheWatchAlone(t *testing.T) {
	m, _, _ := newTestManager(t, Options{})
	ctx := context.Background()

	if err := m.Subscribe(ctx, subA, testURI); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if err := m.Unsubscribe(subB, testURI); err != nil {
		t.Fatalf("Unsubscribe(other session): %v", err)
	}

	if m.Len() != 1 {
		t.Error("a session that never subscribed stopped somebody else's watch")
	}
}

// TestUnsubscribeAll_SubscriberLeaves_StopsOnlyItsWatchers verifies a
// closing session takes its own watches with it and nothing else.
func TestUnsubscribeAll_SubscriberLeaves_StopsOnlyItsWatchers(t *testing.T) {
	m, _, _ := newTestManager(t, Options{})
	ctx := context.Background()

	mine := "gitlab://project/42/pipeline/1"
	shared := "gitlab://project/42/pipeline/2"
	theirs := "gitlab://project/42/pipeline/3"
	for _, uri := range []string{mine, shared} {
		if err := m.Subscribe(ctx, subA, uri); err != nil {
			t.Fatalf("Subscribe(%s): %v", uri, err)
		}
	}
	for _, uri := range []string{shared, theirs} {
		if err := m.Subscribe(ctx, subB, uri); err != nil {
			t.Fatalf("Subscribe(%s): %v", uri, err)
		}
	}

	if stopped := m.UnsubscribeAll(subA); stopped != 1 {
		t.Errorf("UnsubscribeAll() = %d, want 1 — only the watch nobody else holds", stopped)
	}
	if m.Len() != 2 {
		t.Errorf("Len() = %d, want 2 — the shared and the other session's watches must survive", m.Len())
	}
}

// TestUnsubscribe_LastSubscriberLeaves_StopsWatcher verifies the shared
// watcher survives until its last subscriber leaves: one session leaving
// must not blind the other.
func TestUnsubscribe_LastSubscriberLeaves_StopsWatcher(t *testing.T) {
	m, _, _ := newTestManager(t, Options{})
	ctx := context.Background()

	for _, subscriber := range []string{subA, subB} {
		if err := m.Subscribe(ctx, subscriber, testURI); err != nil {
			t.Fatalf("Subscribe: %v", err)
		}
	}

	if err := m.Unsubscribe(subA, testURI); err != nil {
		t.Fatalf("first Unsubscribe: %v", err)
	}
	if m.Len() != 1 {
		t.Fatalf("Len() = %d after one of two left, want the watcher still running", m.Len())
	}

	if err := m.Unsubscribe(subB, testURI); err != nil {
		t.Fatalf("second Unsubscribe: %v", err)
	}
	if m.Len() != 0 {
		t.Errorf("Len() = %d after the last subscriber left, want 0", m.Len())
	}
}

// TestUnsubscribe_UnknownURI_IsNotAnError verifies a late unsubscribe is
// tolerated, since a lease may already have retired the watcher.
func TestUnsubscribe_UnknownURI_IsNotAnError(t *testing.T) {
	m, _, _ := newTestManager(t, Options{})
	if err := m.Unsubscribe(subA, testURI); err != nil {
		t.Errorf("Unsubscribe(unknown) = %v, want nil", err)
	}
}

func TestWatcher_ContentChanges_NotifiesOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m, r, n := newTestManager(t, Options{})
		r.set(testURI, `{"id":99,"status":"running"}`)
		if err := m.Subscribe(context.Background(), subA, testURI); err != nil {
			t.Fatalf("Subscribe: %v", err)
		}

		// Unchanged content must not notify, however many polls elapse.
		time.Sleep(DefaultMinInterval * 5)
		synctest.Wait()
		if got := n.count(); got != 0 {
			t.Fatalf("notifications = %d before any change, want 0", got)
		}

		r.set(testURI, `{"id":99,"status":"success"}`)
		time.Sleep(DefaultMinInterval * 2)
		synctest.Wait()
		if got := n.count(); got != 1 {
			t.Fatalf("notifications = %d after one change, want exactly 1", got)
		}

		// The new content is now the baseline, so it must not re-notify.
		time.Sleep(DefaultBaseInterval * settledFactor * 3)
		synctest.Wait()
		if got := n.count(); got != 1 {
			t.Errorf("notifications = %d, want the baseline to have advanced to 1", got)
		}
	})
}

// TestWatcher_BaselineIsTakenAtSubscribe verifies a subscriber is never
// told about a change that predates its own subscription.
func TestWatcher_BaselineIsTakenAtSubscribe(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m, r, n := newTestManager(t, Options{})
		r.set(testURI, `{"id":99,"status":"success"}`)

		if err := m.Subscribe(context.Background(), subA, testURI); err != nil {
			t.Fatalf("Subscribe: %v", err)
		}
		time.Sleep(DefaultBaseInterval * settledFactor * 2)
		synctest.Wait()

		if got := n.count(); got != 0 {
			t.Errorf("notifications = %d, want 0 — the content never changed after subscribing", got)
		}
	})
}

// TestWatcher_BusyResourcePollsFasterThanSettled verifies the cadence is
// actually driven by the resource's own state.
func TestWatcher_BusyResourcePollsFasterThanSettled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const busyURI = "gitlab://project/42/pipeline/1"
		const settledURI = "gitlab://project/42/pipeline/2"

		m, r, _ := newTestManager(t, Options{})
		r.set(busyURI, `{"id":1,"status":"running"}`)
		r.set(settledURI, `{"id":2,"status":"success"}`)

		ctx := context.Background()
		if err := m.Subscribe(ctx, subA, busyURI); err != nil {
			t.Fatalf("Subscribe(busy): %v", err)
		}
		if err := m.Subscribe(ctx, subA, settledURI); err != nil {
			t.Fatalf("Subscribe(settled): %v", err)
		}

		time.Sleep(DefaultBaseInterval * settledFactor)
		synctest.Wait()

		busy, settled := r.readCount(busyURI), r.readCount(settledURI)
		if busy <= settled {
			t.Errorf("busy polled %d times, settled %d — a running resource must poll more often", busy, settled)
		}
	})
}

// TestWatcher_SettledResourceKeepsPolling is the executable form of the
// correction that GitLab has no terminal state.
//
// A retried pipeline reuses its ID and goes back to running, so a watcher
// that stopped on "success" would go permanently blind to exactly the event
// its subscriber was waiting for.
func TestWatcher_SettledResourceKeepsPolling(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m, r, n := newTestManager(t, Options{})
		r.set(testURI, `{"id":99,"status":"failed"}`)
		if err := m.Subscribe(context.Background(), subA, testURI); err != nil {
			t.Fatalf("Subscribe: %v", err)
		}

		// Someone retries the pipeline: same ID, back to running.
		time.Sleep(DefaultBaseInterval * settledFactor * 2)
		synctest.Wait()
		r.set(testURI, `{"id":99,"status":"running"}`)
		time.Sleep(DefaultBaseInterval * settledFactor * 2)
		synctest.Wait()

		if n.count() == 0 {
			t.Fatal("a retried pipeline produced no notification — the watcher treated a finished state as terminal")
		}
	})
}

// TestWatcher_Inaccessible_StopsWatching verifies a revoked or deleted
// resource retires its watcher instead of polling a token that lost access.
func TestWatcher_Inaccessible_StopsWatching(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m, r, _ := newTestManager(t, Options{})
		if err := m.Subscribe(context.Background(), subA, testURI); err != nil {
			t.Fatalf("Subscribe: %v", err)
		}

		r.fail(testURI, ErrInaccessible)
		time.Sleep(DefaultBaseInterval * 3)
		synctest.Wait()

		if m.Len() != 0 {
			t.Fatalf("Len() = %d, want 0 — the watcher must retire", m.Len())
		}
		before := r.readCount(testURI)
		time.Sleep(DefaultBaseInterval * 5)
		synctest.Wait()
		if after := r.readCount(testURI); after != before {
			t.Errorf("reads went %d -> %d after retiring; the watcher is still polling", before, after)
		}
	})
}

// TestWatcher_TransientError_KeepsWatching verifies an ordinary failure
// costs latency, not the subscription — the whole justification for
// polling being the authoritative path.
func TestWatcher_TransientError_KeepsWatching(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m, r, n := newTestManager(t, Options{})
		r.set(testURI, `{"id":99,"status":"running"}`)
		if err := m.Subscribe(context.Background(), subA, testURI); err != nil {
			t.Fatalf("Subscribe: %v", err)
		}

		r.fail(testURI, errors.New("connection reset"))
		time.Sleep(DefaultBaseInterval * 3)
		synctest.Wait()
		if m.Len() != 1 {
			t.Fatalf("Len() = %d during a transient failure, want the watcher kept alive", m.Len())
		}

		r.heal(testURI)
		r.set(testURI, `{"id":99,"status":"success"}`)
		time.Sleep(DefaultBaseInterval * 3)
		synctest.Wait()
		if got := n.count(); got != 1 {
			t.Errorf("notifications = %d after recovery, want 1 — the change must still be delivered", got)
		}
	})
}

// TestWatcher_RateLimited_PausesEveryWatcher verifies one refusal pauses
// the whole manager, because GitLab enforces the limit per user: the other
// watchers are about to hit it too.
func TestWatcher_RateLimited_PausesEveryWatcher(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const limitedURI = "gitlab://project/42/pipeline/1"
		const otherURI = "gitlab://project/42/pipeline/2"

		m, r, _ := newTestManager(t, Options{})
		r.set(limitedURI, `{"id":1,"status":"running"}`)
		r.set(otherURI, `{"id":2,"status":"running"}`)

		ctx := context.Background()
		if err := m.Subscribe(ctx, subA, limitedURI); err != nil {
			t.Fatalf("Subscribe(limited): %v", err)
		}
		if err := m.Subscribe(ctx, subA, otherURI); err != nil {
			t.Fatalf("Subscribe(other): %v", err)
		}

		r.fail(limitedURI, ErrRateLimited)
		// Let the limited watcher hit the limit and set the pause.
		time.Sleep(DefaultMinInterval * 2)
		synctest.Wait()

		otherBefore := r.readCount(otherURI)
		// Well inside the minimum back-off, so an unpaused watcher would
		// have polled several more times by now.
		time.Sleep(rateLimitBackoff / 2)
		synctest.Wait()

		if otherAfter := r.readCount(otherURI); otherAfter != otherBefore {
			t.Errorf("the unaffected watcher polled %d -> %d during a rate-limit pause; "+
				"the pause must apply to every watcher on the manager", otherBefore, otherAfter)
		}
	})
}

// TestWatcher_RateLimitClears_ResumesPolling verifies the pause is
// temporary and a recovered manager goes back to its normal cadence.
func TestWatcher_RateLimitClears_ResumesPolling(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m, r, _ := newTestManager(t, Options{})
		r.set(testURI, `{"id":99,"status":"running"}`)
		if err := m.Subscribe(context.Background(), subA, testURI); err != nil {
			t.Fatalf("Subscribe: %v", err)
		}

		r.fail(testURI, ErrRateLimited)
		time.Sleep(DefaultMinInterval * 2)
		synctest.Wait()
		r.heal(testURI)

		// Past the longest jittered first back-off.
		time.Sleep(rateLimitBackoff * 2)
		synctest.Wait()
		before := r.readCount(testURI)
		time.Sleep(DefaultMinInterval * 4)
		synctest.Wait()

		if r.readCount(testURI) <= before {
			t.Errorf("reads stayed at %d after the pause cleared; the watcher never resumed", before)
		}
	})
}

// leaseOptions returns options whose lease and slow cadence are short
// enough to reason about, with the rest left at their defaults.
func leaseOptions(lease, slow time.Duration) Options {
	return Options{
		BaseInterval: time.Minute,
		MinInterval:  time.Minute,
		Lease:        lease,
		SlowInterval: slow,
	}
}

// TestWatcher_LeaseExpires_SlowsInsteadOfStopping verifies the lease demotes
// a watcher rather than retiring it.
//
// The distinction is the design's central bet. MCP has no message meaning
// "your subscription expired", so a watcher that stopped at the deadline
// would go silent in a way no client can tell apart from "nothing has
// happened yet" — and it would go blind precisely on the case that
// motivated the whole feature, since GitLab has no terminal state: a
// retried pipeline reuses its ID and starts running again.
func TestWatcher_LeaseExpires_SlowsInsteadOfStopping(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const (
			lease = 10 * time.Minute
			slow  = time.Hour
		)
		m, r, _ := newTestManager(t, leaseOptions(lease, slow))
		r.set(testURI, `{"id":99,"status":"running"}`)
		if err := m.Subscribe(context.Background(), subA, testURI); err != nil {
			t.Fatalf("Subscribe: %v", err)
		}

		time.Sleep(lease / 2)
		synctest.Wait()
		fullSpeed := r.readCount(testURI)
		if fullSpeed < 2 {
			t.Fatalf("reads = %d mid-lease, want the watcher polling normally", fullSpeed)
		}

		time.Sleep(lease)
		synctest.Wait()
		if m.Len() != 1 {
			t.Fatalf("Len() = %d after the lease expired, want the watcher kept", m.Len())
		}

		// Demoted means slower, not silent: over an hour it reads about
		// once, where full speed would have read sixty times.
		demoted := r.readCount(testURI)
		time.Sleep(slow * 2)
		synctest.Wait()
		after := r.readCount(testURI)
		switch {
		case after == demoted:
			t.Error("a demoted watcher stopped reading entirely; it must keep polling slowly")
		case after-demoted > 4:
			t.Errorf("reads grew by %d over two slow intervals; the watcher did not slow down", after-demoted)
		}
	})
}

// TestWatcher_DemotedResource_StillNotifies verifies a slowed watcher has
// not given up its actual job.
func TestWatcher_DemotedResource_StillNotifies(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const (
			lease = 5 * time.Minute
			slow  = 10 * time.Minute
		)
		m, r, n := newTestManager(t, leaseOptions(lease, slow))
		r.set(testURI, `{"id":99,"status":"running"}`)
		if err := m.Subscribe(context.Background(), subA, testURI); err != nil {
			t.Fatalf("Subscribe: %v", err)
		}

		time.Sleep(lease * 2)
		synctest.Wait()

		r.set(testURI, `{"id":99,"status":"success"}`)
		time.Sleep(slow * 2)
		synctest.Wait()

		if n.count() == 0 {
			t.Error("the resource changed while demoted and nobody was told")
		}
	})
}

// stopRecord collects the stop reasons a manager reports.
type stopRecord struct {
	mu      sync.Mutex
	reasons map[string]error
}

func newStopRecord() *stopRecord {
	return &stopRecord{reasons: map[string]error{}}
}

func (s *stopRecord) record(uri string, reason error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reasons[uri] = reason
}

func (s *stopRecord) reason(uri string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reasons[uri]
}

func (s *stopRecord) len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.reasons)
}

// TestOnStop_UnaskedEndings_AreReported verifies the endings a client could
// not have predicted are the ones it gets told about.
//
// This is what the transport layer needs to close a subscription stream
// gracefully instead of leaving it open against a server that stopped
// watching.
func TestOnStop_UnaskedEndings_AreReported(t *testing.T) {
	t.Run("inaccessible", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			stops := newStopRecord()
			r, n := newFakeReader(), &fakeNotifier{}
			m := New[string](r, n, quietOptions(Options{OnStop: stops.record}))
			t.Cleanup(m.Close)

			if err := m.Subscribe(context.Background(), subA, testURI); err != nil {
				t.Fatalf("Subscribe: %v", err)
			}
			r.fail(testURI, ErrInaccessible)
			time.Sleep(DefaultBaseInterval * 3)
			synctest.Wait()

			if got := stops.reason(testURI); !errors.Is(got, ErrInaccessible) {
				t.Errorf("stop reason = %v, want ErrInaccessible", got)
			}
		})
	})

	t.Run("maximum lifetime", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			stops := newStopRecord()
			opts := leaseOptions(time.Minute, 2*time.Minute)
			opts.MaxLifetime = 3 * time.Minute
			opts.OnStop = stops.record
			r, n := newFakeReader(), &fakeNotifier{}
			m := New[string](r, n, quietOptions(opts))
			t.Cleanup(m.Close)

			if err := m.Subscribe(context.Background(), subA, testURI); err != nil {
				t.Fatalf("Subscribe: %v", err)
			}
			time.Sleep(4 * time.Minute)
			synctest.Wait()

			if got := stops.reason(testURI); !errors.Is(got, ErrLifetimeExceeded) {
				t.Errorf("stop reason = %v, want ErrLifetimeExceeded", got)
			}
		})
	})

	t.Run("eviction", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			stops := newStopRecord()
			opts := leaseOptions(time.Minute, time.Hour)
			opts.MaxWatchers = 1
			opts.OnStop = stops.record
			r, n := newFakeReader(), &fakeNotifier{}
			m := New[string](r, n, quietOptions(opts))
			t.Cleanup(m.Close)
			ctx := context.Background()

			evicted := "gitlab://project/42/pipeline/1"
			if err := m.Subscribe(ctx, subA, evicted); err != nil {
				t.Fatalf("Subscribe: %v", err)
			}
			time.Sleep(3 * time.Minute)
			synctest.Wait()
			if err := m.Subscribe(ctx, subA, testURI); err != nil {
				t.Fatalf("Subscribe past the cap: %v", err)
			}
			synctest.Wait()

			if got := stops.reason(evicted); !errors.Is(got, ErrEvicted) {
				t.Errorf("stop reason = %v, want ErrEvicted", got)
			}
		})
	})
}

// TestOnStop_ClientAskedForIt_IsSilent verifies the endings a client caused
// produce no report.
//
// Telling a client its subscription ended when the client is the one that
// ended it is noise at best; at worst the transport layer acts on it and
// tears down a stream the client is still using for something else.
func TestOnStop_ClientAskedForIt_IsSilent(t *testing.T) {
	t.Run("unsubscribe", func(t *testing.T) {
		stops := newStopRecord()
		r, n := newFakeReader(), &fakeNotifier{}
		m := New[string](r, n, quietOptions(Options{OnStop: stops.record}))
		t.Cleanup(m.Close)
		ctx := context.Background()

		if err := m.Subscribe(ctx, subA, testURI); err != nil {
			t.Fatalf("Subscribe: %v", err)
		}
		if err := m.Unsubscribe(subA, testURI); err != nil {
			t.Fatalf("Unsubscribe: %v", err)
		}
		waitForNoWatchers(t, m)

		if stops.len() != 0 {
			t.Errorf("stop reasons = %d after an explicit unsubscribe, want 0", stops.len())
		}
	})

	t.Run("manager closed", func(t *testing.T) {
		stops := newStopRecord()
		r, n := newFakeReader(), &fakeNotifier{}
		m := New[string](r, n, quietOptions(Options{OnStop: stops.record}))

		if err := m.Subscribe(context.Background(), subA, testURI); err != nil {
			t.Fatalf("Subscribe: %v", err)
		}
		m.Close()

		if stops.len() != 0 {
			t.Errorf("stop reasons = %d after Close, want 0 — nobody is left to tell", stops.len())
		}
	})
}

// waitForNoWatchers blocks until every watcher goroutine has wound down.
func waitForNoWatchers(t *testing.T, m *Manager[string]) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for m.Len() > 0 {
		if time.Now().After(deadline) {
			t.Fatalf("Len() = %d after 2s, want the watchers stopped", m.Len())
		}
		time.Sleep(time.Millisecond)
	}
	// The goroutine clears the map before it returns; give it that moment.
	time.Sleep(10 * time.Millisecond)
}

// TestWatcher_Update_CarriesTheWatchState verifies a notification reports
// what the watch is doing, not just that something changed.
//
// MCP has no message for "this subscription slowed down", so the only place
// a subscriber could learn it is alongside a notification it was going to
// receive anyway.
func TestWatcher_Update_CarriesTheWatchState(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const (
			lease = 5 * time.Minute
			slow  = 10 * time.Minute
		)
		m, r, n := newTestManager(t, leaseOptions(lease, slow))
		r.set(testURI, `{"id":99,"status":"running"}`)
		if err := m.Subscribe(context.Background(), subA, testURI); err != nil {
			t.Fatalf("Subscribe: %v", err)
		}

		r.set(testURI, `{"id":99,"status":"running","updated":1}`)
		time.Sleep(2 * time.Minute)
		synctest.Wait()

		active := n.latest()
		switch {
		case active.URI != testURI:
			t.Errorf("Update.URI = %q, want %q", active.URI, testURI)
		case active.Slow:
			t.Error("Update.Slow = true while the lease was still running")
		case active.RenewBy.IsZero():
			t.Error("Update.RenewBy is zero; a client cannot tell when the watch slows down")
		case active.Interval <= 0:
			t.Errorf("Update.Interval = %v, want the cadence the watch is running at", active.Interval)
		}

		time.Sleep(lease)
		synctest.Wait()
		r.set(testURI, `{"id":99,"status":"success"}`)
		time.Sleep(slow * 2)
		synctest.Wait()

		demoted := n.latest()
		if !demoted.Slow {
			t.Error("Update.Slow = false after the lease expired")
		}
		if demoted.Interval != slow {
			t.Errorf("Update.Interval = %v while demoted, want %v", demoted.Interval, slow)
		}
	})
}

// TestRenew_DemotedWatcher_ResumesFullSpeed verifies renewal both extends
// the lease and takes effect immediately.
//
// Waking the watcher matters as much as the flag: a renewal that only took
// hold at the next slow tick would leave the subscriber waiting out most of
// a ten-minute sleep before anything changed, which reads as "renewal did
// nothing".
func TestRenew_DemotedWatcher_ResumesFullSpeed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const (
			lease = 5 * time.Minute
			slow  = time.Hour
		)
		m, r, _ := newTestManager(t, leaseOptions(lease, slow))
		r.set(testURI, `{"id":99,"status":"running"}`)
		if err := m.Subscribe(context.Background(), subA, testURI); err != nil {
			t.Fatalf("Subscribe: %v", err)
		}

		time.Sleep(lease * 2)
		synctest.Wait()
		demoted := r.readCount(testURI)

		if !m.Renew(testURI) {
			t.Fatal("Renew() = false, want it to find the watcher")
		}
		time.Sleep(3 * time.Minute)
		synctest.Wait()

		if after := r.readCount(testURI); after-demoted < 2 {
			t.Errorf("reads grew by %d in three minutes after renewal, want full-speed polling back",
				after-demoted)
		}
	})
}

// TestRenewAll_ActiveWatchers_ReportsNothingRevived verifies renewal on an
// undemoted watcher is silent bookkeeping.
//
// Every request on a session renews, so this is the common path: it must
// not wake timers or log for watchers that were never slowed.
func TestRenewAll_ActiveWatchers_ReportsNothingRevived(t *testing.T) {
	m, _, _ := newTestManager(t, Options{})
	if err := m.Subscribe(context.Background(), subA, testURI); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if revived := m.RenewAll(subA); revived != 0 {
		t.Errorf("RenewAll() = %d, want 0 — nothing was demoted", revived)
	}
}

// TestRenewAll_DemotedWatchers_RevivesThem verifies activity anywhere on a
// session brings back every watcher that session had slowed.
func TestRenewAll_DemotedWatchers_RevivesThem(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const (
			lease = 5 * time.Minute
			slow  = time.Hour
		)
		m, _, _ := newTestManager(t, leaseOptions(lease, slow))
		ctx := context.Background()
		uris := []string{testURI, "gitlab://project/42/pipeline/100"}
		for _, uri := range uris {
			if err := m.Subscribe(ctx, subA, uri); err != nil {
				t.Fatalf("Subscribe(%s): %v", uri, err)
			}
		}

		time.Sleep(lease * 2)
		synctest.Wait()

		if revived := m.RenewAll(subA); revived != len(uris) {
			t.Errorf("RenewAll() = %d, want %d", revived, len(uris))
		}
	})
}

// TestRenewAll_OtherSubscriber_DoesNotRenew verifies one session's traffic
// does not hold another session's abandoned watches at full speed.
//
// The lease asks "is anyone still waiting on this?" — and a busy neighbor
// on the same token is not an answer to that question. Renewing everything
// would make an abandoned watch immortal for as long as any session on the
// server stayed active.
func TestRenewAll_OtherSubscriber_DoesNotRenew(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const (
			lease = 5 * time.Minute
			slow  = time.Hour
		)
		m, r, _ := newTestManager(t, leaseOptions(lease, slow))
		ctx := context.Background()

		abandoned := "gitlab://project/42/pipeline/1"
		if err := m.Subscribe(ctx, subA, abandoned); err != nil {
			t.Fatalf("Subscribe: %v", err)
		}
		time.Sleep(lease * 2)
		synctest.Wait()

		// The other session is busy; the abandoned watch is not its
		// business.
		if revived := m.RenewAll(subB); revived != 0 {
			t.Errorf("RenewAll(other session) = %d, want 0", revived)
		}

		demoted := r.readCount(abandoned)
		time.Sleep(30 * time.Minute)
		synctest.Wait()
		if grew := r.readCount(abandoned) - demoted; grew > 1 {
			t.Errorf("the abandoned watch read %d more times in half an hour; "+
				"another session's activity revived it", grew)
		}
	})
}

// TestRenew_UnknownURI_ReportsFalse verifies renewal of something nobody
// watches is answered honestly rather than silently.
func TestRenew_UnknownURI_ReportsFalse(t *testing.T) {
	m, _, _ := newTestManager(t, Options{})
	if m.Renew("gitlab://project/42/pipeline/404") {
		t.Error("Renew(unwatched) = true, want false")
	}
}

// TestWatcher_MaxLifetime_StopsWatching verifies the one deadline that does
// end a subscription on time alone.
//
// Renewal moves the lease but never this: without an absolute cap, a client
// that stays connected for days would hold a watcher for days, and the
// window in which a revoked token keeps being used would have no bound at
// all.
func TestWatcher_MaxLifetime_StopsWatching(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const (
			lease    = time.Minute
			lifetime = 5 * time.Minute
		)
		opts := leaseOptions(lease, 2*time.Minute)
		opts.MaxLifetime = lifetime
		m, r, _ := newTestManager(t, opts)
		if err := m.Subscribe(context.Background(), subA, testURI); err != nil {
			t.Fatalf("Subscribe: %v", err)
		}

		// Keep renewing: the lease never runs out, and the cap still does.
		for range 6 {
			time.Sleep(time.Minute)
			synctest.Wait()
			m.RenewAll(subA)
		}

		if m.Len() != 0 {
			t.Fatalf("Len() = %d past the maximum lifetime, want 0", m.Len())
		}
		before := r.readCount(testURI)
		time.Sleep(10 * time.Minute)
		synctest.Wait()
		if after := r.readCount(testURI); after != before {
			t.Errorf("reads went %d -> %d after the cap; renewal outlived the absolute lifetime", before, after)
		}
	})
}

// TestSubscribe_AtCapacity_EvictsDemotedWatcher verifies a subscription
// somebody is waiting on wins the slot over one nobody renewed, and that
// the one evicted is the one demoted longest.
//
// Without this the cap is a trap: ten watchers whose subscribers went quiet
// would hold every slot until their absolute lifetime ran out, refusing the
// client that is actively waiting for a pipeline right now. Which one goes
// matters too — evicting the most recently demoted would throw away the
// watch most likely to still be wanted.
func TestSubscribe_AtCapacity_EvictsDemotedWatcher(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		opts := leaseOptions(2*time.Minute, time.Hour)
		opts.MaxWatchers = 2
		m, _, _ := newTestManager(t, opts)
		ctx := context.Background()

		older := "gitlab://project/42/pipeline/1"
		newer := "gitlab://project/42/pipeline/2"
		if err := m.Subscribe(ctx, subA, older); err != nil {
			t.Fatalf("Subscribe(older): %v", err)
		}
		// A minute apart, so their leases — and therefore the order they
		// demote in — are distinguishable.
		time.Sleep(time.Minute)
		synctest.Wait()
		if err := m.Subscribe(ctx, subA, newer); err != nil {
			t.Fatalf("Subscribe(newer): %v", err)
		}

		// Let both fall past their lease, then ask for one more.
		time.Sleep(5 * time.Minute)
		synctest.Wait()
		if err := m.Subscribe(ctx, subA, testURI); err != nil {
			t.Fatalf("Subscribe at capacity with demoted watchers: %v", err)
		}
		synctest.Wait()

		if m.Len() != opts.MaxWatchers {
			t.Errorf("Len() = %d, want the cap of %d respected", m.Len(), opts.MaxWatchers)
		}
		// Renew answers only for a URI still being watched, which is how
		// this tells the survivor from the victim.
		if m.Renew(older) {
			t.Error("the longest-demoted watch survived; eviction picked the wrong victim")
		}
		if !m.Renew(newer) {
			t.Error("the more recently demoted watch was evicted; eviction picked the wrong victim")
		}
	})
}

// TestSubscribe_AtCapacity_AllActive_IsStillRejected verifies eviction only
// ever claims a slot nobody is actively waiting on.
func TestSubscribe_AtCapacity_AllActive_IsStillRejected(t *testing.T) {
	m, _, _ := newTestManager(t, Options{MaxWatchers: 2})
	ctx := context.Background()

	for _, uri := range []string{"gitlab://project/42/pipeline/1", "gitlab://project/42/pipeline/2"} {
		if err := m.Subscribe(ctx, subA, uri); err != nil {
			t.Fatalf("Subscribe(%s): %v", uri, err)
		}
	}

	err := m.Subscribe(ctx, subA, testURI)
	if !errors.Is(err, ErrTooManySubscriptions) {
		t.Errorf("Subscribe() = %v with every watcher active, want ErrTooManySubscriptions", err)
	}
	if m.Len() != 2 {
		t.Errorf("Len() = %d, want 2 — an active watch must not be evicted", m.Len())
	}
}

// TestNotifierFailure_AdvancesBaseline verifies a failed notification does
// not wedge the watcher into reporting the same change forever.
func TestNotifierFailure_AdvancesBaseline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r, n := newFakeReader(), &fakeNotifier{err: errors.New("session gone")}
		m := New[string](r, n, quietOptions(Options{}))
		t.Cleanup(m.Close)

		r.set(testURI, `{"id":99,"status":"running"}`)
		if err := m.Subscribe(context.Background(), subA, testURI); err != nil {
			t.Fatalf("Subscribe: %v", err)
		}

		r.set(testURI, `{"id":99,"status":"success"}`)
		time.Sleep(DefaultMinInterval * 3)
		synctest.Wait()
		time.Sleep(DefaultBaseInterval * settledFactor * 3)
		synctest.Wait()

		if got := n.count(); got != 1 {
			t.Errorf("notification attempts = %d, want exactly 1 — a failed delivery must not be retried forever", got)
		}
	})
}

// TestClose_StopsEveryWatcher verifies eviction cancels rather than
// orphans. An orphaned watcher would keep polling GitLab with a token
// whose session is gone, and an unsubscribe arriving on a rebuilt server
// could never reach it.
func TestClose_StopsEveryWatcher(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r, n := newFakeReader(), &fakeNotifier{}
		m := New[string](r, n, quietOptions(Options{}))

		ctx := context.Background()
		for _, uri := range []string{
			"gitlab://project/42/pipeline/1",
			"gitlab://project/42/pipeline/2",
		} {
			if err := m.Subscribe(ctx, subA, uri); err != nil {
				t.Fatalf("Subscribe(%s): %v", uri, err)
			}
		}

		m.Close()
		if m.Len() != 0 {
			t.Fatalf("Len() = %d after Close, want 0", m.Len())
		}

		before := r.readCount("gitlab://project/42/pipeline/1")
		time.Sleep(DefaultBaseInterval * 5)
		synctest.Wait()
		if after := r.readCount("gitlab://project/42/pipeline/1"); after != before {
			t.Errorf("reads went %d -> %d after Close; a watcher outlived the manager", before, after)
		}
	})
}

func TestClose_IsIdempotentAndRefusesLaterSubscribes(t *testing.T) {
	r, n := newFakeReader(), &fakeNotifier{}
	m := New[string](r, n, quietOptions(Options{}))

	m.Close()
	m.Close() // must not panic or block

	if err := m.Subscribe(context.Background(), subA, testURI); !errors.Is(err, ErrClosed) {
		t.Errorf("Subscribe after Close = %v, want ErrClosed", err)
	}
}

// TestOptions_Defaults verifies the zero value is usable and that a
// nonsensical floor above the base is clamped rather than inverting the
// cadence.
func TestOptions_Defaults(t *testing.T) {
	got := Options{}.withDefaults()
	if got.BaseInterval != DefaultBaseInterval || got.MinInterval != DefaultMinInterval {
		t.Errorf("intervals = %v/%v, want %v/%v", got.BaseInterval, got.MinInterval, DefaultBaseInterval, DefaultMinInterval)
	}
	if got.Lease != DefaultLease || got.MaxWatchers != DefaultMaxWatchers {
		t.Errorf("lease/cap = %v/%d, want %v/%d", got.Lease, got.MaxWatchers, DefaultLease, DefaultMaxWatchers)
	}
	if got.SlowInterval != DefaultSlowInterval || got.MaxLifetime != DefaultMaxLifetime {
		t.Errorf("slow/lifetime = %v/%v, want %v/%v",
			got.SlowInterval, got.MaxLifetime, DefaultSlowInterval, DefaultMaxLifetime)
	}
	if got.Logger == nil {
		t.Error("Logger = nil, want a default")
	}

	clamped := Options{BaseInterval: 10 * time.Second, MinInterval: time.Minute}.withDefaults()
	if clamped.MinInterval > clamped.BaseInterval {
		t.Errorf("MinInterval %v exceeds BaseInterval %v; the cadence is inverted",
			clamped.MinInterval, clamped.BaseInterval)
	}
}

// TestOptions_InvertedLifetimes_AreClamped verifies the two orderings that
// would turn a slowdown into a speed-up, or a cap into a shorter lease.
func TestOptions_InvertedLifetimes_AreClamped(t *testing.T) {
	got := Options{
		BaseInterval: time.Minute,
		SlowInterval: time.Second,
		Lease:        time.Hour,
		MaxLifetime:  time.Minute,
	}.withDefaults()

	if got.SlowInterval < got.BaseInterval {
		t.Errorf("SlowInterval %v is faster than BaseInterval %v; demotion would speed the watcher up",
			got.SlowInterval, got.BaseInterval)
	}
	if got.MaxLifetime < got.Lease {
		t.Errorf("MaxLifetime %v is shorter than Lease %v; the watcher would die before it could slow down",
			got.MaxLifetime, got.Lease)
	}
}

// TestJitter_StaysPositiveAndNear verifies scheduling jitter never yields a
// non-positive delay, which would turn a back-off into a hot loop.
func TestJitter_StaysPositiveAndNear(t *testing.T) {
	for _, d := range []time.Duration{time.Second, rateLimitBackoff, maxRateLimitBackoff} {
		t.Run(d.String(), func(t *testing.T) {
			for range 200 {
				got := jitter(d)
				if got <= 0 {
					t.Fatalf("jitter(%v) = %v, want a positive duration", d, got)
				}
				lo := time.Duration(float64(d) * (1 - jitterFraction*1.01))
				hi := time.Duration(float64(d) * (1 + jitterFraction*1.01))
				if got < lo || got > hi {
					t.Fatalf("jitter(%v) = %v, want within ±%.0f%%", d, got, jitterFraction*100)
				}
			}
		})
	}
}

// TestRecordRateLimit_BackoffGrowsAndIsCapped verifies consecutive
// refusals escalate but never exceed the ceiling.
func TestRecordRateLimit_BackoffGrowsAndIsCapped(t *testing.T) {
	m := New[string](newFakeReader(), &fakeNotifier{}, quietOptions(Options{}))
	t.Cleanup(m.Close)

	first := m.recordRateLimit()
	second := m.recordRateLimit()
	if second <= first {
		t.Errorf("back-off did not grow: %v then %v", first, second)
	}
	for range 20 {
		if got := m.recordRateLimit(); got > maxRateLimitBackoff {
			t.Fatalf("back-off %v exceeded the cap %v", got, maxRateLimitBackoff)
		}
	}
	// Resetting the escalation is covered by
	// TestClearRateLimit_AfterPauseExpires_Resets, which has to advance
	// past the pause first — clearing during one is deliberately a no-op.
}

// TestSubscribe_ConcurrentSameURI_KeepsOneWatcher verifies the cap and the
// registry hold up under concurrent subscribes, which is how a real client
// with several sessions behaves.
func TestSubscribe_ConcurrentSameURI_KeepsOneWatcher(t *testing.T) {
	m, r, _ := newTestManager(t, Options{})
	ctx := context.Background()

	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Go(func() {
			errs[i] = m.Subscribe(ctx, subA, testURI)
		})
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("Subscribe #%d: %v", i, err)
		}
	}
	if m.Len() != 1 {
		t.Errorf("Len() = %d, want 1", m.Len())
	}
	if got := r.readCount(testURI); got != 1 {
		t.Errorf("reads = %d, want 1 — only the first subscriber reads", got)
	}
}

// TestSubscribe_ConcurrentDistinctURIs_RespectsCap verifies the cap cannot
// be exceeded by racing subscribers, since the slot is reserved before the
// read releases the lock.
func TestSubscribe_ConcurrentDistinctURIs_RespectsCap(t *testing.T) {
	const limit = 4
	m, _, _ := newTestManager(t, Options{MaxWatchers: limit})
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			_ = m.Subscribe(ctx, subA, fmt.Sprintf("gitlab://project/42/pipeline/%d", i+1))
		})
	}
	wg.Wait()

	if got := m.Len(); got > limit {
		t.Errorf("Len() = %d, want at most the cap %d", got, limit)
	}
}

// closingReader closes the manager from inside the initial read, which is
// the only way to deterministically exercise the window where Subscribe has
// reserved a slot but Close lands before the watcher starts.
type closingReader struct {
	m  **Manager[string]
	mu sync.Mutex
}

func (c *closingReader) Read(_ context.Context, _ string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if m := *c.m; m != nil {
		m.Close()
	}
	return []byte(`{"id":1}`), nil
}

// TestSubscribe_ClosedDuringInitialRead_LeavesNoWatcher verifies the
// reserved slot is released when Close wins the race with a subscribe whose
// read is still in flight.
//
// Without the post-read re-check, that slot would linger in the map of a
// closed manager and its watcher would never be started or cancelled.
func TestSubscribe_ClosedDuringInitialRead_LeavesNoWatcher(t *testing.T) {
	var m *Manager[string]
	r := &closingReader{m: &m}
	m = New[string](r, &fakeNotifier{}, quietOptions(Options{}))

	err := m.Subscribe(context.Background(), subA, testURI)
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("Subscribe() = %v, want ErrClosed", err)
	}
	if m.Len() != 0 {
		t.Errorf("Len() = %d, want 0 — the reserved slot must be released", m.Len())
	}
}

// TestUpdateDigest_AfterWatcherRemoved_DoesNotNotify verifies a poll that
// finishes after its watcher was unsubscribed cannot deliver a late
// notification to a subscriber that already left.
func TestUpdateDigest_AfterWatcherRemoved_DoesNotNotify(t *testing.T) {
	m := New[string](newFakeReader(), &fakeNotifier{}, quietOptions(Options{}))
	t.Cleanup(m.Close)

	orphan := &watcher[string]{uri: testURI, kind: KindPipeline}
	if m.updateDigest(orphan, sha256.Sum256([]byte(`{"changed":true}`))) {
		t.Error("updateDigest reported a change for a watcher no longer in the registry")
	}
}

// TestPoll_ContextCancelledMidRead_DoesNotRetire verifies a read that fails
// because the lease expired is not mistaken for a resource-level failure.
//
// Retiring here would be harmless but wrong; the watcher's own select
// reports the cancellation, and conflating the two would hide a genuine
// ErrInaccessible behind a shutdown race in the logs.
func TestPoll_ContextCancelledMidRead_DoesNotRetire(t *testing.T) {
	r := newFakeReader()
	r.fail(testURI, errors.New("context canceled"))
	m := New[string](r, &fakeNotifier{}, quietOptions(Options{}))
	t.Cleanup(m.Close)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	next, stopReason := m.poll(ctx, &watcher[string]{uri: testURI, kind: KindPipeline})
	if stopReason != nil {
		t.Errorf("poll() asked to stop with %v on a cancelled context; the select must own that decision", stopReason)
	}
	if next <= 0 {
		t.Errorf("poll() next = %v, want a positive interval", next)
	}
}

// TestJitter_DegenerateDuration_StaysPositive verifies the guard that stops
// a rounded-to-zero jitter turning a back-off into a hot loop.
func TestJitter_DegenerateDuration_StaysPositive(t *testing.T) {
	for range 500 {
		if got := jitter(1); got <= 0 {
			t.Fatalf("jitter(1ns) = %v, want a positive duration", got)
		}
	}
}

// TestClearRateLimit_DuringActivePause_IsIgnored pins the rule that makes
// the rate-limit pause manager-wide instead of per-watcher.
//
// Watchers poll concurrently, so a read already in flight can succeed
// moments after another watcher was refused. If that success cleared the
// pause, every remaining watcher would march straight back into the same
// limit — which is exactly what -race -shuffle caught before this guard
// existed. A success during a pause must also leave the escalation counter
// alone, or a repeatedly-limited manager would back off by the same 30
// seconds forever instead of doubling.
func TestClearRateLimit_DuringActivePause_IsIgnored(t *testing.T) {
	m := New[string](newFakeReader(), &fakeNotifier{}, quietOptions(Options{}))
	t.Cleanup(m.Close)

	first := m.recordRateLimit()
	if first <= 0 {
		t.Fatalf("recordRateLimit() = %v, want a positive pause", first)
	}

	// A concurrent watcher's read succeeds while the pause is in force.
	m.clearRateLimit()

	if m.remainingPause() <= 0 {
		t.Fatal("the pause was cleared by a concurrent success; it must stay in force for every watcher")
	}
	if second := m.recordRateLimit(); second <= first {
		t.Errorf("back-off went %v then %v; the streak was reset by a success during the pause", first, second)
	}
}

// TestClearRateLimit_AfterPauseExpires_Resets verifies the pause is not
// sticky: once it lapses, a successful read restores the normal cadence and
// the escalation starts over.
func TestClearRateLimit_AfterPauseExpires_Resets(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := New[string](newFakeReader(), &fakeNotifier{}, quietOptions(Options{}))
		t.Cleanup(m.Close)

		m.recordRateLimit()
		m.recordRateLimit() // escalate, so a failed reset would be visible

		time.Sleep(maxRateLimitBackoff * 2)
		synctest.Wait()

		m.clearRateLimit()
		if got := m.remainingPause(); got != 0 {
			t.Errorf("remainingPause() = %v after the pause lapsed, want 0", got)
		}
		if got := m.recordRateLimit(); got > jitter(rateLimitBackoff)*2 {
			t.Errorf("back-off = %v, want it reset to roughly the base %v", got, rateLimitBackoff)
		}
	})
}
