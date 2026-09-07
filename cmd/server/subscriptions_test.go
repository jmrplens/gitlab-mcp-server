// subscriptions_test.go covers the wiring that turns the subscriptions
// package into a working resources/subscribe implementation: whether the
// capability is advertised, and whether a real client actually receives a
// notification when a watched GitLab resource changes.
//
// These exercise the seam the unit tests cannot: the manager's fakes prove
// the watcher logic, but only a round trip through the real server proves
// the reader reaches the same handler the router uses and that the notifier
// is attached to a server that can deliver.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/resources"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/serverpool"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/subscriptions"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
)

// pipelineBackend is a mock GitLab whose pipeline status can be changed
// mid-test, which is what makes a change detectable end to end.
type pipelineBackend struct {
	status  atomic.Value // string
	missing atomic.Bool  // serve the watched pipeline as deleted
	hits    atomic.Int64 // reads of the watched pipeline
	other   atomic.Int64 // every other API call, including the ones that 404
}

func newPipelineBackend(t *testing.T, initial string) (*pipelineBackend, *httptest.Server) {
	t.Helper()
	b := &pipelineBackend{}
	b.status.Store(initial)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v4/version":
			_ = json.NewEncoder(w).Encode(map[string]string{"version": "16.0.0", "revision": "test"})
		case strings.HasSuffix(r.URL.Path, "/pipelines/99"):
			b.hits.Add(1)
			if b.missing.Load() {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 99, "iid": 7, "status": b.status.Load().(string),
				"ref": "main", "sha": "abc123",
				"web_url": "https://gitlab.example.com/p/-/pipelines/99",
				"source":  "push",
			})
		default:
			b.other.Add(1)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return b, srv
}

// calls reports every GitLab API request the backend has served, which is
// what a test watches to tell a live watcher from a refused subscription.
func (b *pipelineBackend) calls() int64 { return b.hits.Load() + b.other.Load() }

func (b *pipelineBackend) setStatus(s string) { b.status.Store(s) }

// setMissing makes the watched pipeline read as deleted, which GitLab and a
// revoked token are indistinguishable from: both answer 404.
func (b *pipelineBackend) setMissing() { b.missing.Store(true) }

// subscriptionTestServer builds a real server against a mock GitLab, with
// the given capability surface.
func subscriptionTestServer(t *testing.T, gitlabURL, capabilitySurface string, opts ...serverOption) *mcp.Server {
	t.Helper()
	cfg := &config.ServerConfig{
		ToolSurface:       config.ToolSurfaceDynamic,
		CapabilitySurface: capabilitySurface,
		GitLabURL:         gitlabURL,
	}
	server, err := createServer(t.Context(), subscriptionGitLabClient(t, gitlabURL), cfg, opts...)
	if err != nil {
		t.Fatalf("createServer() error: %v", err)
	}
	return server
}

// fastPolling drives watchers at a pace a test can wait for.
//
// These intervals are three orders of magnitude below production's, and
// deliberately so: how the cadence is *chosen* is covered by the
// subscriptions package's own tests, which run on a fake clock. What is
// under test here is only that a change reaches a connected client, so the
// real clock is unavoidable — the mock GitLab is a real HTTP server, and
// synctest cannot advance time in a bubble that contains a socket.
func fastPolling() serverOption {
	return withSubscriptionOptions(fastOptions())
}

func fastOptions() subscriptions.Options {
	return subscriptions.Options{
		BaseInterval: 20 * time.Millisecond,
		MinInterval:  10 * time.Millisecond,
	}
}

// newTestSubscriptions builds the subscription machinery a single-credential
// server has: the per-shape half, and one credential's runtime over it.
//
// The two halves used to be one object, and every test here drove that object.
// They are separate because a server is now shared by every credential of a
// configuration shape: the streams, the handler index and the stateless flag
// belong to the shape, and the watchers belong to the credential, since a
// watcher polls with a token and its first read is the subscription's
// authorization check. A test that needs only the watchers takes the runtime
// through [newTestRuntime]; a test that needs the streams or the handler pair
// takes both from here.
//
// The owner is empty, which is what stdio passes: one credential, nothing to
// tell apart.
func newTestSubscriptions(
	client *gitlabclient.Client,
	cfg *config.ServerConfig,
	opts subscriptions.Options,
) (*subscriptionShape, *subscriptionRuntime) {
	shape := newSubscriptionShape(client, cfg, opts)
	return shape, shape.newRuntime("", client)
}

// newTestRuntime is [newTestSubscriptions] for the tests that only drive one
// credential's watchers.
func newTestRuntime(
	client *gitlabclient.Client,
	cfg *config.ServerConfig,
	opts subscriptions.Options,
) *subscriptionRuntime {
	_, runtime := newTestSubscriptions(client, cfg, opts)
	return runtime
}

// staticRuntime resolves every request to one runtime, which is what a
// single-credential server does: [serverShell.subscriptionRuntimeFor] falls
// back to the shell's own state whenever nothing bound a credential.
func staticRuntime(runtime *subscriptionRuntime) func(context.Context) *subscriptionRuntime {
	return func(context.Context) *subscriptionRuntime { return runtime }
}

// staticCounter is [staticRuntime] for the listen ceiling: one credential, one
// counter, whatever the request carried.
func staticCounter(counter *listenCounter) func(context.Context) *listenCounter {
	return func(context.Context) *listenCounter { return counter }
}

// leasedPolling drives watchers fast and gives them a lease short enough
// that a test can watch one run out.
func leasedPolling(lease, slow time.Duration) serverOption {
	opts := fastOptions()
	opts.Lease = lease
	opts.SlowInterval = slow
	return withSubscriptionOptions(opts)
}

// waitForPolls blocks until the mock GitLab has served n reads, so a test
// can assert on what happened over several poll cycles without guessing how
// long they take.
func waitForPolls(t *testing.T, b *pipelineBackend, n int64) {
	t.Helper()
	waitForPollsWithin(t, b, n, 5*time.Second)
}

// waitForPollsWithin is waitForPolls with the deadline chosen by the test,
// for the cases where the deadline is the assertion: a watch still at its
// base period reaches n within milliseconds, and one demoted to the slow
// interval cannot reach it before a deadline of a few seconds, so the
// deadline separates the two outcomes without measuring how fast the machine
// is. A count that a fixed sleep was supposed to reach did measure it, and
// on a slow runner two reads landed where three were expected.
func waitForPollsWithin(t *testing.T, b *pipelineBackend, n int64, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for b.hits.Load() < n {
		if time.Now().After(deadline) {
			t.Fatalf("GitLab was polled %d times in %v, want at least %d", b.hits.Load(), within, n)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// settledCount returns a backend counter once it has not moved for a full
// quiet span, so a read already in flight when a watcher was stopped has
// landed before the test starts counting, rather than a moment after the
// sample was taken. The counter is passed in because the backend keeps two,
// every request and the reads it answered, and a test must settle the one it
// then compares. A count that keeps moving until the deadline is a watcher
// that never stopped, and is reported as that.
func settledCount(t *testing.T, count func() int64, quiet time.Duration) int64 {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	last, since := count(), time.Now()
	for time.Since(since) < quiet {
		if time.Now().After(deadline) {
			t.Fatalf("GitLab requests kept arriving for 5s (%d so far); the watcher never stopped", count())
		}
		time.Sleep(2 * time.Millisecond)
		if now := count(); now != last {
			last, since = now, time.Now()
		}
	}
	return last
}

// TestCreateServer_SubscribeCapability_FollowsCapabilitySurface verifies
// the capability is advertised exactly when there is something to
// subscribe to.
//
// Advertising it on a surface that registers no GitLab resources would
// invite clients to subscribe to URIs this server never serves — and at
// least one shipping client subscribes to everything a server advertises.
func TestCreateServer_SubscribeCapability_FollowsCapabilitySurface(t *testing.T) {
	_, gitlab := newPipelineBackend(t, "running")

	tests := []struct {
		surface string
		want    bool
	}{
		{config.CapabilitySurfaceFull, true},
		{config.CapabilitySurfaceMinimal, false},
	}
	for _, tt := range tests {
		t.Run(tt.surface, func(t *testing.T) {
			server := subscriptionTestServer(t, gitlab.URL, tt.surface)
			session := connectInMemory(t, server)

			caps := session.InitializeResult().Capabilities
			got := caps.Resources != nil && caps.Resources.Subscribe
			if got != tt.want {
				t.Errorf("resources.subscribe advertised = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSubscribe_UnsubscribableURI_StartsNoWatcher verifies the whitelist is
// enforced at the protocol boundary, not merely inside the manager.
//
// The assertion is about traffic rather than about the error the client
// receives, because on the protocol version the SDK negotiates by default
// (2026-07-28) there is no error for it to receive: Subscribe opens a
// subscriptions/listen stream, and the SDK's callSubscriptionsListen fires
// that call without ever awaiting its response, so a server-side refusal is
// discarded client-side. What a subscriber can still observe is that
// nothing is being watched — which is exactly what the refusal is for.
func TestSubscribe_UnsubscribableURI_StartsNoWatcher(t *testing.T) {
	backend, gitlab := newPipelineBackend(t, "running")
	client := subscriptionGitLabClient(t, gitlab.URL)
	manager := newTestRuntime(client, subscriptionCfg(config.CapabilitySurfaceFull), fastOptions()).manager
	t.Cleanup(manager.Close)

	const uri = "gitlab://project/42/issues" // a collection: deliberately excluded
	err := manager.Subscribe(context.Background(), testSession, uri)
	if !errors.Is(err, subscriptions.ErrNotSubscribable) {
		t.Fatalf("Subscribe(collection) error = %v, want ErrNotSubscribable", err)
	}
	if manager.Len() != 0 {
		t.Errorf("watchers = %d after a refused subscribe, want 0", manager.Len())
	}

	// A refusal that still polled would burn API budget forever on a URI
	// this server never notifies about.
	time.Sleep(100 * time.Millisecond)
	if got := backend.calls(); got != 0 {
		t.Errorf("GitLab was called %d times for a URI the whitelist rejects, want 0", got)
	}
}

// TestSubscribe_UnreadableURI_StartsNoWatcher verifies the synchronous first
// read rejects a URI this token cannot read, rather than accepting a
// subscription that could never fire.
func TestSubscribe_UnreadableURI_StartsNoWatcher(t *testing.T) {
	backend, gitlab := newPipelineBackend(t, "running")
	client := subscriptionGitLabClient(t, gitlab.URL)
	manager := newTestRuntime(client, subscriptionCfg(config.CapabilitySurfaceFull), fastOptions()).manager
	t.Cleanup(manager.Close)

	// Pipeline 12345 is not served by the backend, so the read 404s.
	const uri = "gitlab://project/42/pipeline/12345"
	if err := manager.Subscribe(context.Background(), testSession, uri); err == nil {
		t.Fatal("Subscribe(unreadable) error = nil, want a refusal")
	}
	if manager.Len() != 0 {
		t.Errorf("watchers = %d after a refused subscribe, want 0", manager.Len())
	}

	// The refusal cost one read; what must not happen is a second one. The
	// count is sampled once it has stopped moving, so the read the refusal
	// itself made cannot land between the sample and the check.
	settled := settledCount(t, backend.calls, 100*time.Millisecond)
	time.Sleep(200 * time.Millisecond) // ten or more poll periods
	if got := backend.calls(); got != settled {
		t.Errorf("GitLab calls went %d -> %d after a refused subscribe; a watcher outlived the refusal", settled, got)
	}
}

// TestSubscribe_RefusalIsInvisibleToTheClient documents, rather than
// endorses, what a client sees when the server refuses a subscription.
//
// This is a property of go-sdk v1.7.0, not of this server: on protocol
// 2026-07-28 the client's Subscribe returns nil no matter what the server
// answers. The test exists so that if a later SDK starts surfacing the
// refusal, this fails and the two tests above can assert the error directly.
func TestSubscribe_RefusalIsInvisibleToTheClient(t *testing.T) {
	_, gitlab := newPipelineBackend(t, "running")
	server := subscriptionTestServer(t, gitlab.URL, config.CapabilitySurfaceFull, fastPolling())
	session := connectInMemory(t, server)

	if got := session.InitializeResult().ProtocolVersion; got < "2026-07-28" {
		t.Skipf("negotiated protocol %s still returns subscribe errors synchronously", got)
	}
	err := session.Subscribe(context.Background(), &mcp.SubscribeParams{URI: "gitlab://project/42/issues"})
	if err != nil {
		t.Errorf("Subscribe(collection) error = %v; the SDK now reports refusals — "+
			"assert the error directly in the tests above", err)
	}
}

// TestSubscribe_ResourceChanges_ClientIsNotified is the end-to-end proof:
// a real client subscribes, GitLab's answer changes, and the notification
// arrives over the wire.
func TestSubscribe_ResourceChanges_ClientIsNotified(t *testing.T) {
	backend, gitlab := newPipelineBackend(t, "running")
	server := subscriptionTestServer(t, gitlab.URL, config.CapabilitySurfaceFull, fastPolling())

	updated := make(chan string, 8)
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1"}, &mcp.ClientOptions{
		ResourceUpdatedHandler: func(_ context.Context, req *mcp.ResourceUpdatedNotificationRequest) {
			select {
			case updated <- req.Params.URI:
			default: // a full buffer already proves the point
			}
		},
	})

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	session, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer session.Close()

	const uri = "gitlab://project/42/pipeline/99"
	if subErr := session.Subscribe(ctx, &mcp.SubscribeParams{URI: uri}); subErr != nil {
		t.Fatalf("Subscribe: %v", subErr)
	}

	// Several polls with nothing changed must produce silence, or a
	// notification would only mean "we polled", not "it changed".
	waitForPolls(t, backend, 3)
	select {
	case got := <-updated:
		t.Fatalf("notified about %s before anything changed", got)
	default:
	}

	backend.setStatus("success")
	select {
	case got := <-updated:
		if got != uri {
			t.Errorf("notified URI = %q, want %q", got, uri)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the pipeline finished but no resources/updated reached the client")
	}
}

// TestSubscribe_Unsubscribe_StopsPolling verifies unsubscribing actually
// stops the GitLab traffic, which is what the API budget depends on.
func TestSubscribe_Unsubscribe_StopsPolling(t *testing.T) {
	backend, gitlab := newPipelineBackend(t, "running")
	server := subscriptionTestServer(t, gitlab.URL, config.CapabilitySurfaceFull, fastPolling())
	session := connectInMemory(t, server)
	ctx := context.Background()

	const uri = "gitlab://project/42/pipeline/99"
	if err := session.Subscribe(ctx, &mcp.SubscribeParams{URI: uri}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	waitForPolls(t, backend, 3)

	if err := session.Unsubscribe(ctx, &mcp.UnsubscribeParams{URI: uri}); err != nil {
		t.Fatalf("Unsubscribe: %v", err)
	}
	// A read already in flight when the watcher was cancelled may still
	// land, so the count is sampled once it has stopped moving.
	before := settledCount(t, backend.hits.Load, 100*time.Millisecond)
	time.Sleep(200 * time.Millisecond) // ten or more poll periods

	if after := backend.hits.Load(); after != before {
		t.Errorf("GitLab hits went %d -> %d after unsubscribing; polling did not stop", before, after)
	}
}

// TestSubscribe_SessionEnds_WatcherStops verifies a watcher is bounded by
// the lifetime of the session that asked for it, on the protocol a modern
// client negotiates.
func TestSubscribe_SessionEnds_WatcherStops(t *testing.T) {
	backend, gitlab := newPipelineBackend(t, "running")
	server := subscriptionTestServer(t, gitlab.URL, config.CapabilitySurfaceFull, fastPolling())

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}

	const uri = "gitlab://project/42/pipeline/99"
	if subErr := session.Subscribe(ctx, &mcp.SubscribeParams{URI: uri}); subErr != nil {
		t.Fatalf("Subscribe: %v", subErr)
	}
	waitForPolls(t, backend, 3)

	if closeErr := session.Close(); closeErr != nil {
		t.Fatalf("session close: %v", closeErr)
	}
	// A read already in flight may still land after the session ends, so
	// the count is sampled once it has stopped moving.
	before := settledCount(t, backend.calls, 100*time.Millisecond)
	time.Sleep(200 * time.Millisecond) // ten or more poll periods

	if after := backend.calls(); after != before {
		t.Errorf("GitLab calls went %d -> %d after the session closed; the watcher outlived its subscriber",
			before, after)
	}
}

// TestSessionBridge_SessionEnds_ReleasesItsWatchers is the test that
// actually pins sessionBridge.awaitEnd.
//
// The end-to-end version above cannot: on protocol 2026-07-28 a
// subscription is an open subscriptions/listen request, and the SDK's own
// handler unsubscribes when that request unwinds — so the watcher would
// stop even with awaitEnd deleted. What the SDK never does is call the
// unsubscribe handler for a session that simply disconnects, which is the
// case a legacy client produces and the one this covers, by driving the
// bridge with a session whose transport it can close underneath it.
func TestSessionBridge_SessionEnds_ReleasesItsWatchers(t *testing.T) {
	_, gitlab := newPipelineBackend(t, "running")
	runtime := newTestRuntime(subscriptionGitLabClient(t, gitlab.URL),
		subscriptionCfg(config.CapabilitySurfaceFull), fastOptions())
	t.Cleanup(runtime.manager.Close)
	bridge := newSessionBridge(runtime.manager)

	serverSession, clientSession := connectedSessions(t)
	ctx := context.Background()

	const uri = "gitlab://project/42/pipeline/99"
	subErr := bridge.Subscribe(ctx, &mcp.SubscribeRequest{
		Session: serverSession,
		Params:  &mcp.SubscribeParams{URI: uri},
	})
	if subErr != nil {
		t.Fatalf("Subscribe: %v", subErr)
	}
	if runtime.manager.Len() != 1 {
		t.Fatalf("watchers = %d, want 1", runtime.manager.Len())
	}

	// The client goes away without ever unsubscribing — the common case.
	if closeErr := clientSession.Close(); closeErr != nil {
		t.Fatalf("client close: %v", closeErr)
	}

	deadline := time.Now().Add(5 * time.Second)
	for runtime.manager.Len() > 0 {
		if time.Now().After(deadline) {
			t.Fatal("the session disconnected and its watcher kept running; " +
				"nothing releases a watch the SDK never reports as unsubscribed")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestSessionBridge_DuplicateSubscribe_ReleasesOnFirstUnsubscribe verifies a
// session that subscribes twice still lets go on one unsubscribe.
//
// The bridge is driven directly rather than through a client session on
// purpose: the SDK keeps its own per-session subscription table and would
// swallow the second subscribe before it ever reached this server, hiding
// exactly the case under test. What a real client does reach is one
// Subscribe per subscriptions/listen stream, and nothing stops it opening
// two for one URI.
func TestSessionBridge_DuplicateSubscribe_ReleasesOnFirstUnsubscribe(t *testing.T) {
	_, gitlab := newPipelineBackend(t, "running")
	runtime := newTestRuntime(subscriptionGitLabClient(t, gitlab.URL),
		subscriptionCfg(config.CapabilitySurfaceFull), fastOptions())
	t.Cleanup(runtime.manager.Close)
	bridge := newSessionBridge(runtime.manager)
	session, _ := connectedSessions(t)

	const uri = "gitlab://project/42/pipeline/99"
	ctx := context.Background()
	for range 2 {
		err := bridge.Subscribe(ctx, &mcp.SubscribeRequest{
			Session: session,
			Params:  &mcp.SubscribeParams{URI: uri},
		})
		if err != nil {
			t.Fatalf("Subscribe: %v", err)
		}
	}
	if runtime.manager.Len() != 1 {
		t.Fatalf("watchers = %d after two subscribes to one URI, want 1", runtime.manager.Len())
	}

	err := bridge.Unsubscribe(ctx, &mcp.UnsubscribeRequest{
		Session: session,
		Params:  &mcp.UnsubscribeParams{URI: uri},
	})
	if err != nil {
		t.Fatalf("Unsubscribe: %v", err)
	}

	if runtime.manager.Len() != 0 {
		t.Errorf("watchers = %d after unsubscribing, want 0; "+
			"a duplicate subscribe left a hold only a second unsubscribe could clear", runtime.manager.Len())
	}
}

// TestSessionBridge_ForeignUnsubscribe_LeavesTheWatchAlone verifies one
// session cannot release a watch another session holds.
func TestSessionBridge_ForeignUnsubscribe_LeavesTheWatchAlone(t *testing.T) {
	_, gitlab := newPipelineBackend(t, "running")
	runtime := newTestRuntime(subscriptionGitLabClient(t, gitlab.URL),
		subscriptionCfg(config.CapabilitySurfaceFull), fastOptions())
	t.Cleanup(runtime.manager.Close)
	bridge := newSessionBridge(runtime.manager)
	session, _ := connectedSessions(t)
	other, _ := connectedSessions(t)

	const uri = "gitlab://project/42/pipeline/99"
	ctx := context.Background()
	err := bridge.Subscribe(ctx, &mcp.SubscribeRequest{
		Session: session,
		Params:  &mcp.SubscribeParams{URI: uri},
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	err = bridge.Unsubscribe(ctx, &mcp.UnsubscribeRequest{
		Session: other,
		Params:  &mcp.UnsubscribeParams{URI: uri},
	})
	if err != nil {
		t.Fatalf("Unsubscribe(other session): %v", err)
	}

	if runtime.manager.Len() != 1 {
		t.Error("a session that never subscribed stopped another session's watch")
	}
}

// TestSubscribe_IdleSession_KeepsWatchingWhileItsStreamIsOpen records that an
// open subscription stream holds its own lease, whatever the session does.
//
// This test used to assert the opposite, and the change is deliberate. The
// lease is a proxy for "is anyone still there", answered from request traffic
// on the session. A subscription opened through subscriptions/listen — which is
// what the SDK client sends, and the only path stateless HTTP has — can answer
// that question directly: the request is still open, so the subscriber is still
// connected and still reading. Traffic is the weaker evidence of the two, and on
// the default transport it does not exist at all, since every stateless POST is
// its own session and a listen stream's session sees only its own listen.
//
// Without this the shipped default would have degraded by a factor of forty
// half an hour in, while the client sat on a stream it was still reading, and
// nothing would have said so.
//
// The lease is not gone. It still governs the legacy resources/subscribe, which
// has no stream to speak for it — pinned by
// TestSessionBridge_ALegacySubscribeStillFollowsTheLease — and MaxLifetime
// still caps every watch at 24 hours regardless.
func TestSubscribe_IdleSession_KeepsWatchingWhileItsStreamIsOpen(t *testing.T) {
	const (
		lease = 100 * time.Millisecond
		slow  = 2 * time.Second
	)
	backend, gitlab := newPipelineBackend(t, "running")
	server := subscriptionTestServer(t, gitlab.URL, config.CapabilitySurfaceFull, leasedPolling(lease, slow))
	session := connectInMemory(t, server)

	const uri = "gitlab://project/42/pipeline/99"
	if err := session.Subscribe(context.Background(), &mcp.SubscribeParams{URI: uri}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	waitForPolls(t, backend, 3)

	// Several leases of silence on a session that sends nothing else. The
	// baseline counts reads of the watched pipeline only, the same counter
	// the wait below watches, so nothing but polling can move it.
	time.Sleep(lease + slow/4)
	afterLease := backend.hits.Load()

	// Three more reads at the base period arrive within milliseconds; a
	// watch demoted to the slow interval would need three of those, seconds
	// apart, so the deadline separates the two outcomes without measuring
	// how fast the machine is.
	waitForPollsWithin(t, backend, afterLease+3, slow/2)
}

// TestSubscribe_ActiveSession_WatcherStaysFast verifies any traffic on the
// session holds the lease open.
//
// Renewal is what makes a short lease safe. Without it the choice would be
// between a lease long enough to be useless as a bound and one short enough
// to slow down a client that is sitting there watching a pipeline.
//
// The threshold separates two very different outcomes rather than
// measuring one: across this window a renewed watch keeps reading on the
// 20ms base, while an unrenewed one demotes after the first lease and then
// waits out a 2-second interval, so it cannot reach a fraction of the
// count no matter how the machine is loaded.
func TestSubscribe_ActiveSession_WatcherStaysFast(t *testing.T) {
	const (
		lease = 100 * time.Millisecond
		slow  = 2 * time.Second
	)
	backend, gitlab := newPipelineBackend(t, "running")
	server := subscriptionTestServer(t, gitlab.URL, config.CapabilitySurfaceFull, leasedPolling(lease, slow))
	session := connectInMemory(t, server)
	ctx := context.Background()

	const uri = "gitlab://project/42/pipeline/99"
	if err := session.Subscribe(ctx, &mcp.SubscribeParams{URI: uri}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	waitForPolls(t, backend, 3)

	// Stay busy well past the first lease, then start counting. Measuring
	// only after the point where an unrenewed watch would already have
	// demoted turns this into a yes-or-no question — is it still polling at
	// all? — instead of a comparison of rates, which the machine's speed
	// would otherwise decide.
	busy := func(rounds int) {
		for range rounds {
			// A tool call, not tools/list: from protocol 2026-07-28 the
			// SDK client caches catalog responses, so a listing can be
			// answered locally and never reach this server at all. Only
			// traffic that arrives is evidence anyone is still there,
			// which is exactly what the lease is asking about.
			_, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      "gitlab_find_action",
				Arguments: map[string]any{"query": "pipeline"},
			})
			if err != nil {
				t.Errorf("CallTool: %v", err)
				return
			}
			time.Sleep(lease / 2)
		}
	}
	busy(8) // four lease periods
	stillActive := backend.hits.Load()

	// Keep the session busy while waiting for three more reads of the
	// watched pipeline. At the base period they arrive within milliseconds;
	// a watch demoted to the slow interval would need seconds, so the
	// deadline is the assertion and the machine's speed is not. Only reads
	// of the pipeline count: the busy traffic itself reaches GitLab, and
	// counting it would let the renewal calls stand in for the polling they
	// are supposed to keep alive.
	deadline := time.Now().Add(slow / 2)
	for backend.hits.Load()-stillActive < 3 {
		if time.Now().After(deadline) {
			t.Fatalf("the pipeline was read %d more times in %v after the lease would have run out; "+
				"activity on the session did not renew the watch", backend.hits.Load()-stillActive, slow/2)
		}
		busy(1)
	}
}

// TestKeepalive_IsNotRenewalActivity is the negative half of the renewal
// contract.
//
// A ping proves a socket is open, not that anyone is waiting on the other end
// of it. If keep-alive traffic renewed the lease, the lease would be
// unreachable for any connected client, which is the same as having no lease.
//
// This used to be observed end to end, by pinging a subscribed session and
// watching the poll rate fall anyway. That observation no longer distinguishes
// anything: the SDK client subscribes through subscriptions/listen, and an open
// listen stream now holds its own watch at full speed regardless of what else
// the session does — see
// TestSubscribe_IdleSession_KeepsWatchingWhileItsStreamIsOpen. The rule still
// exists and is still worth pinning; it is pinned where it lives, at the
// predicate that decides what counts as activity, and the lease itself is
// exercised through the legacy path in
// TestSessionBridge_ALegacySubscribeStillFollowsTheLease.
func TestKeepalive_IsNotRenewalActivity(t *testing.T) {
	for _, method := range []string{"ping", "notifications/initialized", "notifications/cancelled"} {
		t.Run(method, func(t *testing.T) {
			req := &mcp.ServerRequest[*mcp.CallToolParams]{Session: &mcp.ServerSession{}}
			if _, ok := activeSession(method, req); ok {
				t.Errorf("%q was treated as evidence that a subscriber is still there", method)
			}
		})
	}

	// The control: something a client only sends because it wants an answer.
	req := &mcp.ServerRequest[*mcp.CallToolParams]{Session: &mcp.ServerSession{}}
	if _, ok := activeSession("tools/call", req); !ok {
		t.Error("a tool call was not treated as session activity; nothing would renew a legacy watch")
	}
}

// TestActiveSession_NonServerSession_IsNotActivity verifies renewal only
// fires for a request that actually names a server session.
func TestActiveSession_NonServerSession_IsNotActivity(t *testing.T) {
	if _, ok := activeSession("tools/call", nil); ok {
		t.Error("a nil request was treated as session activity")
	}
	if _, ok := activeSession("ping", &mcp.ServerRequest[*mcp.CallToolParams]{Session: &mcp.ServerSession{}}); ok {
		t.Error("a ping was treated as session activity")
	}
	session := &mcp.ServerSession{}
	got, ok := activeSession("tools/call", &mcp.ServerRequest[*mcp.CallToolParams]{Session: session})
	if !ok || got != session {
		t.Errorf("activeSession(tools/call) = %v, %v; want the request's own session", got, ok)
	}
}

// TestIsClientActivity_KeepaliveAndHandshake_DoNotRenew verifies what does
// and does not count as a subscriber being present.
//
// A ping proves a socket is open, not that anyone is waiting on the other
// end of it. Counting keep-alive traffic as activity would make the lease
// unreachable for any connected client, which is the same as having no
// lease at all.
func TestIsClientActivity_KeepaliveAndHandshake_DoNotRenew(t *testing.T) {
	tests := []struct {
		method string
		want   bool
	}{
		{"tools/call", true},
		{"resources/read", true},
		{"resources/subscribe", true},
		{"prompts/get", true},
		{"ping", false},
		{"initialize", false},
		{"notifications/initialized", false},
		{"notifications/cancelled", false},
		{"notifications/progress", false},
	}
	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			if got := isClientActivity(tt.method); got != tt.want {
				t.Errorf("isClientActivity(%q) = %v, want %v", tt.method, got, tt.want)
			}
		})
	}
}

// TestResourceReader_ReadsThroughTheRegisteredHandler verifies the watcher
// and a client's own resources/read see byte-identical content.
//
// If these ever diverged, "the content changed" would stop meaning "what
// you would read changed", and notifications would be about something the
// subscriber cannot observe.
func TestResourceReader_ReadsThroughTheRegisteredHandler(t *testing.T) {
	_, gitlab := newPipelineBackend(t, "running")
	server := subscriptionTestServer(t, gitlab.URL, config.CapabilitySurfaceFull)
	session := connectInMemory(t, server)
	ctx := context.Background()

	const uri = "gitlab://project/42/pipeline/99"
	result, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if len(result.Contents) == 0 {
		t.Fatal("ReadResource returned no contents")
	}

	reader := newTestReader(t, gitlab.URL)
	watched, err := reader.Read(ctx, uri)
	if err != nil {
		t.Fatalf("resourceReader.Read: %v", err)
	}

	if string(watched) != result.Contents[0].Text {
		t.Errorf("the watcher and the client see different content:\n watcher: %s\n client:  %s",
			watched, result.Contents[0].Text)
	}
}

// TestResourceReader_RejectsUnsubscribableURI verifies the reader refuses a
// URI the whitelist does not cover, so a stale watcher cannot read outside
// the subscribable set.
func TestResourceReader_RejectsUnsubscribableURI(t *testing.T) {
	_, gitlab := newPipelineBackend(t, "running")
	reader := newTestReader(t, gitlab.URL)

	if _, err := reader.Read(context.Background(), "gitlab://project/42/issues"); err == nil {
		t.Error("resourceReader.Read(collection) error = nil, want a refusal")
	}
}

// TestSubscribe_StatelessHTTP_LegacyPathIsRefused verifies a subscription
// this transport could never deliver on is refused instead of accepted.
//
// In stateless HTTP — the default — the SDK gives each POST its own session
// and closes it when the POST returns, so a legacy resources/subscribe
// would be cancelled microseconds after being acknowledged, having already
// spent a GitLab round-trip on the authorization read. The capability stays
// advertised because the 2026-07-28 subscriptions/listen path does work
// there: a subscription is an open request, which is exactly what stateless
// mode still has.
func TestSubscribe_StatelessHTTP_LegacyPathIsRefused(t *testing.T) {
	_, gitlab := newPipelineBackend(t, "running")
	client := subscriptionGitLabClient(t, gitlab.URL)
	cfg := subscriptionCfg(config.CapabilitySurfaceFull)
	cfg.Stateless = true

	shape, runtime := newTestSubscriptions(client, cfg, fastOptions())
	t.Cleanup(runtime.manager.Close)
	subscribe, unsubscribe := shape.handlers(staticRuntime(runtime))
	if subscribe == nil || unsubscribe == nil {
		t.Fatal("handlers() returned nil in stateless mode; the capability must stay advertised for subscriptions/listen")
	}

	err := subscribe(context.Background(), &mcp.SubscribeRequest{
		Session: testSession,
		Params:  &mcp.SubscribeParams{URI: "gitlab://project/42/pipeline/99"},
	})
	if err == nil {
		t.Fatal("legacy subscribe was accepted in stateless mode, where no notification could ever reach the client")
	}
	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) || rpcErr.Code != jsonrpc.CodeInvalidRequest {
		t.Errorf("refusal error = %v, want *jsonrpc.Error with code %d — a plain error reaches the wire as code 0", err, jsonrpc.CodeInvalidRequest)
	}
	if runtime.manager.Len() != 0 {
		t.Errorf("watchers = %d after a refused subscribe, want 0", runtime.manager.Len())
	}
}

// TestSubscribe_StatefulHTTP_LegacyPathIsAccepted verifies the refusal is
// scoped to the transport that cannot deliver, not to HTTP in general.
func TestSubscribe_StatefulHTTP_LegacyPathIsAccepted(t *testing.T) {
	_, gitlab := newPipelineBackend(t, "running")
	cfg := subscriptionCfg(config.CapabilitySurfaceFull)
	cfg.Stateless = false

	shape, runtime := newTestSubscriptions(subscriptionGitLabClient(t, gitlab.URL), cfg, fastOptions())
	t.Cleanup(runtime.manager.Close)
	subscribe, _ := shape.handlers(staticRuntime(runtime))

	session, _ := connectedSessions(t)
	err := subscribe(context.Background(), &mcp.SubscribeRequest{
		Session: session,
		Params:  &mcp.SubscribeParams{URI: "gitlab://project/42/pipeline/99"},
	})
	if err != nil {
		t.Fatalf("legacy subscribe on a session-bearing transport: %v", err)
	}
	if runtime.manager.Len() != 1 {
		t.Errorf("watchers = %d, want 1", runtime.manager.Len())
	}
}

// TestNewSubscriptionShape_MinimalSurface_IsNil verifies no shape, no runtime
// and therefore no capability on a surface without GitLab resources.
//
// Every method that is reached on the nil shape is exercised here, because nil
// is not an error case on this surface: it is what --capability-surface=minimal
// produces, and the wiring calls all of them on it unconditionally.
func TestNewSubscriptionShape_MinimalSurface_IsNil(t *testing.T) {
	_, gitlab := newPipelineBackend(t, "running")
	client := subscriptionGitLabClient(t, gitlab.URL)

	shape, runtime := newTestSubscriptions(client, subscriptionCfg(config.CapabilitySurfaceMinimal), subscriptions.Options{})
	if shape != nil {
		t.Errorf("newSubscriptionShape(minimal) = %v, want nil", shape)
	}
	if runtime != nil {
		t.Errorf("newRuntime on no shape = %v, want nil", runtime)
	}

	sub, unsub := shape.handlers(staticRuntime(runtime))
	if sub != nil || unsub != nil {
		t.Error("handlers() on no shape returned non-nil handlers; the SDK would advertise a capability with no backing")
	}

	if shape.streamRegistry() == nil {
		t.Error("streamRegistry() on no shape returned nil; nothing could then end a stream the SDK is holding")
	}

	// setIndex is called by registration on every surface, and registration
	// does not know whether subscriptions were offered.
	shape.setIndex(subscriptionHandlerIndex(t, gitlab.URL))

	// close on the runtime is what eviction calls, on whatever the surface
	// produced.
	runtime.close()
}

// TestSubscriptionShape_Handlers_ARequestBoundToNoCredential_IsRefused covers
// the fail-closed branch of the two handlers a shared server registers.
//
// Each call is routed to the runtime of the pool entry the request belongs to.
// A request that resolves to none is refused rather than served from a default,
// because on a shared server the default watches with the unbound client: a
// subscription accepted there would be answered by a watcher that can never
// read, and its first read is the authorization check ADR-0015 relies on.
func TestSubscriptionShape_Handlers_ARequestBoundToNoCredential_IsRefused(t *testing.T) {
	_, gitlab := newPipelineBackend(t, "running")
	shape, runtime := newTestSubscriptions(subscriptionGitLabClient(t, gitlab.URL),
		subscriptionCfg(config.CapabilitySurfaceFull), fastOptions())
	t.Cleanup(runtime.manager.Close)

	subscribe, unsubscribe := shape.handlers(staticRuntime(nil))
	if subscribe == nil || unsubscribe == nil {
		t.Fatal("handlers() returned nil on a full capability surface")
	}

	const uri = "gitlab://project/42/pipeline/99"
	subErr := subscribe(t.Context(), &mcp.SubscribeRequest{
		Session: testSession,
		Params:  &mcp.SubscribeParams{URI: uri},
	})
	if !errors.Is(subErr, errUnboundSubscribe) {
		t.Errorf("subscribe error = %v, want %v; an unattributed subscription must not be watched", subErr, errUnboundSubscribe)
	}
	unsubErr := unsubscribe(t.Context(), &mcp.UnsubscribeRequest{
		Session: testSession,
		Params:  &mcp.UnsubscribeParams{URI: uri},
	})
	if !errors.Is(unsubErr, errUnboundSubscribe) {
		t.Errorf("unsubscribe error = %v, want %v", unsubErr, errUnboundSubscribe)
	}
	if runtime.manager.Len() != 0 {
		t.Errorf("watchers = %d after two refusals, want 0", runtime.manager.Len())
	}

	t.Run("an abandoned request is answered with why it ended", func(t *testing.T) {
		// The one legitimate cause of the same state: a POST the client
		// abandoned takes its carrier with it, and the carrier is where the
		// credential is read from. Calling that a wiring defect and asking for
		// a report is wrong about a client that pressed stop.
		gone := errors.New("the caller went away")
		abandoned, cancel := context.WithCancelCause(t.Context())
		cancel(gone)

		err := subscribe(abandoned, &mcp.SubscribeRequest{
			Session: testSession,
			Params:  &mcp.SubscribeParams{URI: uri},
		})
		if !errors.Is(err, gone) {
			t.Errorf("subscribe error = %v, want %v; a client that went away is not a wiring defect", err, gone)
		}
	})
}

// TestServerNotifier_ResourceUpdated_TagsWhatTheFilterReads drives the notifier
// the way a watcher does, through a real server, and asserts the tag is on the
// params that reach the sending middleware.
//
// The unit test below calls watchMeta directly, which is the one thing that
// cannot go wrong here: the notifier has a method of that name and the package
// has a function of that name, and the difference between them is the owner tag.
// Calling the package function from ResourceUpdated compiles, passes every
// cmd/server test, and produces exactly the failure ADR-0020 records from the
// first implementation of this: every notification from a pooled entry sent and
// then dropped by the filter for want of a tag, with the server logging that it
// had sent them.
//
// So this drives ResourceUpdated instead, with both middlewares installed in the
// order the real wiring installs them, and asserts on both ends: the params the
// filter is handed carry the owner, and the client on the other side of the
// filter actually receives the notification.
func TestServerNotifier_ResourceUpdated_TagsWhatTheFilterReads(t *testing.T) {
	const owner = "owner-mine"
	const uri = "gitlab://project/42/pipeline/99"

	// The pipeline never changes, so the server's own watcher stays silent and
	// the only notification on the wire is the one this test sends.
	_, gitlab := newPipelineBackend(t, "running")
	server := subscriptionTestServer(t, gitlab.URL, config.CapabilitySurfaceFull, fastPolling())

	sessions := newSessionOwners(false)
	server.AddSendingMiddleware(sessions.sendingMiddleware)
	// Added after the filter, so it wraps it and sees the params before the
	// owner key is stripped for the wire.
	var sent []mcp.Params
	var mu sync.Mutex
	acked := make(chan struct{}, 1)
	server.AddSendingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			switch method {
			case notificationResourceUpdated:
				mu.Lock()
				sent = append(sent, req.GetParams())
				mu.Unlock()
			case "notifications/subscriptions/acknowledged":
				select {
				case acked <- struct{}{}:
				default:
				}
			}
			return next(ctx, method, req)
		}
	})

	delivered := make(chan string, 4)
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1"}, &mcp.ClientOptions{
		ResourceUpdatedHandler: func(_ context.Context, req *mcp.ResourceUpdatedNotificationRequest) {
			select {
			case delivered <- req.Params.URI:
			default:
			}
		},
	})
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	session, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	if subErr := session.Subscribe(ctx, &mcp.SubscribeParams{URI: uri}); subErr != nil {
		t.Fatalf("Subscribe: %v", subErr)
	}
	// Subscribe returns before the server has filed the subscription, so the
	// acknowledgement is what says the SDK's table now holds this session: the
	// listen handler sends it only after every URI it carries has been
	// subscribed. Without this wait the notification below is sent to nobody.
	select {
	case <-acked:
	case <-time.After(5 * time.Second):
		t.Fatal("the subscription was never acknowledged, so nothing is subscribed to notify")
	}

	// The session is recorded under the same owner the notifier stamps, which
	// is what the request path does on a shared server.
	for serverSession := range server.Sessions() {
		sessions.record(serverSession, owner)
	}

	notifier := &serverNotifier{owner: owner}
	notifier.attach(server)
	if updateErr := notifier.ResourceUpdated(ctx, subscriptions.Update{URI: uri, Interval: time.Second}); updateErr != nil {
		t.Fatalf("ResourceUpdated: %v", updateErr)
	}

	mu.Lock()
	seen := slices.Clone(sent)
	mu.Unlock()
	if len(seen) != 1 {
		t.Fatalf("the sending chain saw %d resource-updated notifications, want 1", len(seen))
	}
	params, ok := seen[0].(*mcp.ResourceUpdatedNotificationParams)
	if !ok {
		t.Fatalf("params = %T, want *mcp.ResourceUpdatedNotificationParams", seen[0])
	}
	if got := params.Meta[ownerMetaKey]; got != owner {
		t.Errorf("owner tag on the params = %v, want %q: the filter drops what it cannot attribute", got, owner)
	}

	select {
	case got := <-delivered:
		if got != uri {
			t.Errorf("delivered URI = %q, want %q", got, uri)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the notification was sent and never delivered, which is what an untagged notification looks like")
	}
}

// TestServerNotifier_WatchMeta_StampsTheOwnerOnASharedServer covers the tag the
// delivery filter reads.
//
// A notification carries it so that a server answering for many credentials can
// tell whose watcher produced it; the filter drops anything untagged, so an
// unstamped notification from a pooled entry would be silently undeliverable.
// On stdio the owner is empty, and the key is left out rather than written
// empty, because "" is also what an unrecorded session reads as.
func TestServerNotifier_WatchMeta_StampsTheOwnerOnASharedServer(t *testing.T) {
	update := subscriptions.Update{URI: "gitlab://project/42/pipeline/99", Interval: time.Second}

	tests := map[string]string{
		"a pooled credential": "owner-mine",
		"stdio, which serves one credential and tells none apart": "",
	}

	for name, owner := range tests {
		t.Run(name, func(t *testing.T) {
			notifier := &serverNotifier{owner: owner}

			meta := notifier.watchMeta(update)

			got, tagged := meta[ownerMetaKey]
			if owner == "" {
				if tagged {
					t.Errorf("the owner key was written as %v with no owner to name", got)
				}
				return
			}
			if got != owner {
				t.Errorf("owner tag = %v, want %q; the delivery filter would drop this notification", got, owner)
			}
			if _, described := meta[watchMetaKey]; !described {
				t.Error("stamping the owner dropped the watch metadata the notification carries for the client")
			}
		})
	}
}

// TestListenCounter_ANilCounter_IsANoOp covers the nil receiver every method
// tolerates.
//
// Nil is what a request nothing could attribute to a credential resolves to.
// The ceiling lets it through — the process-wide one is what actually bounds
// the process — so acquire, release and count all have to answer rather than
// panic on the request path.
func TestListenCounter_ANilCounter_IsANoOp(t *testing.T) {
	var counter *listenCounter

	if !counter.acquire(1) {
		t.Error("a nil counter refused a slot; an unattributed listen would be refused rather than bounded per process")
	}
	counter.release()
	if got := counter.count(); got != 0 {
		t.Errorf("count = %d on a nil counter, want 0", got)
	}
}

// TestSubscriptionRuntime_NoRuntime_ShutdownStillEndsListenStreams verifies a
// server that offers no resource subscriptions still ends the listen streams
// the SDK holds on its behalf.
//
// The capability surface decides whether subscriptions are advertised. It does
// not decide whether the SDK acknowledges a subscriptions/listen carrying only
// list-changed notifications: it always does, and the go-sdk client opens one
// by itself at connect time whenever it registers a list-changed handler. That
// request is then held open until its handler's context ends, and nothing but
// the stream registry ends it.
//
// So on --capability-surface=minimal, where newSubscriptionShape returns nil,
// the registry used to be absent and those streams became unreachable. The
// visible symptom was in shutdown: the process ignored its signal for the full
// HTTP drain budget and then died with "http server shutdown: context deadline
// exceeded" and exit 1. Since [shutdownHTTPServer] that budget ends in a forced
// close instead, so the delay and the streams left without a completion result
// are what the bug would show today. The wire half of this is pinned in
// test/e2e/http/shutdown_test.go, which drives the real binary with that flag
// and a real signal; this half pins the wiring that makes it possible.
//
// The server here is a bare one carrying the capabilities the minimal surface
// declares, rather than one from createServer. What changed is attach's
// contract on a nil runtime, and registering the whole catalog to reach it
// would add several seconds to a package that is already close to the test
// timeout.
func TestSubscriptionRuntime_NoRuntime_ShutdownStillEndsListenStreams(t *testing.T) {
	// The server's own context, which is what the signal handler cancels.
	ctx, shutdown := context.WithCancel(context.Background())
	t.Cleanup(shutdown)

	server := mcp.NewServer(&mcp.Implementation{Name: "no-subscriptions", Version: "1"}, &mcp.ServerOptions{
		// What createServer declares on the minimal capability surface: list
		// changes, and no resources.subscribe.
		Capabilities: &mcp.ServerCapabilities{
			Tools:     &mcp.ToolCapabilities{ListChanged: true},
			Resources: &mcp.ResourceCapabilities{ListChanged: true},
		},
	})

	// Nil is exactly what the minimal capability surface produces, and attach
	// is called on it unconditionally.
	var shape *subscriptionShape
	shape.attach(ctx, server, staticRuntime(nil))

	// Applied last, so it wraps the stream registry's middleware and sees what
	// the SDK's handler finally returned.
	ended := make(chan error, 1)
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method != methodSubscriptionsListen {
				return next(ctx, method, req)
			}
			result, listenErr := next(ctx, method, req)
			select {
			case ended <- listenErr:
			default:
			}
			return result, listenErr
		}
	})

	// The acknowledgment is the SDK's own signal that the stream is registered
	// and it is about to block, so waiting for it is what makes the shutdown
	// below land on an established stream every time. Canceling earlier would
	// exercise the arrived-during-the-drain path instead, which is
	// TestListenStreams_ArmedAfterCloseAll_EndsImmediately's job.
	acknowledged := make(chan struct{}, 1)
	server.AddSendingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			result, sendErr := next(ctx, method, req)
			if method == "notifications/subscriptions/acknowledged" {
				select {
				case acknowledged <- struct{}{}:
				default:
				}
			}
			return result, sendErr
		}
	})

	session := connectListeningForToolChanges(t, server)
	if session.InitializeResult().ProtocolVersion < "2026-07-28" {
		t.Skip("subscriptions/listen only exists from protocol 2026-07-28")
	}

	select {
	case <-acknowledged:
	case <-time.After(5 * time.Second):
		t.Fatal("the client's listen stream was never acknowledged, so there was nothing for shutdown to close")
	}

	shutdown()

	select {
	case listenErr := <-ended:
		if listenErr != nil {
			t.Errorf("the listen stream ended with %v, want the graceful result", listenErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown left a listen stream open; the process would wait out its whole drain budget and exit with an error")
	}
}

// connectListeningForToolChanges connects a client that asks for tools/list
// change notifications, which is what makes the SDK open a subscriptions/listen
// of its own during the handshake.
//
// It is the shape a real 2026-07-28 client has, since a list-changed handler is
// ordinary, and therefore the shape that produced open streams on a server
// advertising no subscriptions at all.
func connectListeningForToolChanges(t *testing.T, server *mcp.Server) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "listening-client", Version: "1"}, &mcp.ClientOptions{
		ToolListChangedHandler: func(context.Context, *mcp.ToolListChangedRequest) {},
	})
	session, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// TestServerNotifier_BeforeAttach_IsQuiet verifies the notifier tolerates
// being called before the server exists rather than panicking, since it is
// constructed first by necessity.
func TestServerNotifier_BeforeAttach_IsQuiet(t *testing.T) {
	var n serverNotifier
	update := subscriptions.Update{URI: "gitlab://project/42/pipeline/99"}
	if err := n.ResourceUpdated(context.Background(), update); err != nil {
		t.Errorf("ResourceUpdated before attach = %v, want nil", err)
	}
}

// TestWatchMeta_DescribesTheWatch verifies the state that travels alongside
// a notification says what the watch is actually doing.
func TestWatchMeta_DescribesTheWatch(t *testing.T) {
	renewBy := time.Date(2026, 8, 24, 19, 30, 0, 0, time.UTC)
	tests := []struct {
		name     string
		update   subscriptions.Update
		wantKeys map[string]any
	}{
		{
			name:   "active watch",
			update: subscriptions.Update{URI: "u", RenewBy: renewBy, Interval: 15 * time.Second},
			wantKeys: map[string]any{
				"state":          "active",
				"pollIntervalMs": int64(15000),
				"renewBy":        "2026-08-24T19:30:00Z",
			},
		},
		{
			// Settled is not demoted: a finished pipeline polls at the 60s
			// settled cadence while its lease is intact, and the _meta must
			// let a client tell that apart from the 10-minute lease
			// slowdown.
			name:   "settled watch",
			update: subscriptions.Update{URI: "u", RenewBy: renewBy, Interval: 60 * time.Second},
			wantKeys: map[string]any{
				"state":          "active",
				"pollIntervalMs": int64(60000),
			},
		},
		{
			name:   "demoted watch",
			update: subscriptions.Update{URI: "u", Slow: true, RenewBy: renewBy, Interval: 10 * time.Minute},
			wantKeys: map[string]any{
				"state":          "slow",
				"pollIntervalMs": int64(600000),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta := watchMeta(tt.update)
			watch, ok := meta[watchMetaKey].(map[string]any)
			if !ok {
				t.Fatalf("_meta[%q] = %#v, want a map", watchMetaKey, meta[watchMetaKey])
			}
			for key, want := range tt.wantKeys {
				if got := watch[key]; got != want {
					t.Errorf("_meta watch %q = %#v, want %#v", key, got, want)
				}
			}
		})
	}
}

// TestWatchMeta_IsNotSharedBetweenNotifications verifies each notification
// gets its own map.
//
// The SDK stamps its own subscription ID into this map, once per subscribed
// session, so a shared map would be rewritten per recipient and those
// mutations would leak back into the next notification.
func TestWatchMeta_IsNotSharedBetweenNotifications(t *testing.T) {
	update := subscriptions.Update{URI: "u", Interval: time.Second}
	first, second := watchMeta(update), watchMeta(update)

	first["io.modelcontextprotocol/subscriptionId"] = "stamped-by-the-sdk"
	if _, leaked := second["io.modelcontextprotocol/subscriptionId"]; leaked {
		t.Error("two notifications share one _meta map; per-session stamping would cross over")
	}
}

// TestListenStreams_EveryURIStops_ClosesTheStream verifies a subscription
// stream ends once there is nothing left to watch for it.
func TestListenStreams_EveryURIStops_ClosesTheStream(t *testing.T) {
	streams := newListenStreams()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, release := streams.arm([]string{"uri-a", "uri-b"}, "owner", nil, cancel)
	defer release()

	streams.stoppedFor("owner", "uri-a", subscriptions.ErrInaccessible)
	if ctx.Err() != nil {
		t.Fatal("the stream ended while it was still watching uri-b")
	}

	streams.stoppedFor("owner", "uri-b", subscriptions.ErrInaccessible)
	if ctx.Err() == nil {
		t.Error("every watched URI stopped and the stream was left open")
	}
}

// TestListenStreams_AnotherCredentialsWatchStops_LeavesTheStream verifies the
// scope that keeps a shared server honest.
//
// Watchers are per credential, so "this URI stopped" is a statement about one
// credential's watch. Without the owner, one tenant's watcher retiring on a 404
// would close every other tenant's open listen over the same URI, and the
// specification's graceful completion result would be delivered to clients
// whose subscription is perfectly alive.
func TestListenStreams_AnotherCredentialsWatchStops_LeavesTheStream(t *testing.T) {
	streams := newListenStreams()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const uri = "gitlab://project/42/pipeline/99"
	_, release := streams.arm([]string{uri}, "mine", nil, cancel)
	defer release()

	streams.stoppedFor("somebody-elses", uri, subscriptions.ErrInaccessible)
	if ctx.Err() != nil {
		t.Fatal("another credential's watch stopping closed this credential's stream over the same URI")
	}

	streams.stoppedFor("mine", uri, subscriptions.ErrInaccessible)
	if ctx.Err() == nil {
		t.Error("the stream's own watch stopped and the stream was left open")
	}
}

// TestListenStreams_UnrelatedURIStops_LeavesTheStream verifies one stream's
// ending does not disturb another's.
func TestListenStreams_UnrelatedURIStops_LeavesTheStream(t *testing.T) {
	streams := newListenStreams()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, release := streams.arm([]string{"mine"}, "owner", nil, cancel)
	defer release()

	streams.stoppedFor("owner", "somebody-elses", subscriptions.ErrEvicted)
	if ctx.Err() != nil {
		t.Error("a stream ended over a URI it never subscribed to")
	}
}

// TestListenStreams_ReleasedStream_IsNotCancelled verifies a stream that
// already returned is forgotten.
func TestListenStreams_ReleasedStream_IsNotCancelled(t *testing.T) {
	streams := newListenStreams()
	cancelled := false
	_, release := streams.arm([]string{"uri"}, "owner", nil, func() { cancelled = true })
	release()

	streams.stoppedFor("owner", "uri", subscriptions.ErrInaccessible)
	if cancelled {
		t.Error("a released stream was cancelled; its request had already returned")
	}
}

// TestListenStreams_CloseAll_EndsEveryStreamItHolds verifies shutdown reaches
// streams no URI will ever close on its own.
//
// stopped() only ever ends a stream once every URI it named has stopped, so a
// listen carrying only list-changed subscriptions names none and can never be
// ended that way. closeAll is the only thing that ends those, and the SDK's
// handler blocks until it does, so a stream missed here is a process that
// outlives its own shutdown.
func TestListenStreams_CloseAll_EndsEveryStreamItHolds(t *testing.T) {
	streams := newListenStreams()

	cases := []struct {
		name string
		uris []string
	}{
		{name: "a stream watching resources", uris: []string{"gitlab://project/42/pipeline/99"}},
		{name: "a list-changed stream, which names no URI", uris: nil},
	}

	// One armed stream per case, each recording its own cancellation, so the
	// assertions below can tell which of them closeAll reached. The real cancel
	// is a context's; a bare function is enough to observe the call, and
	// closeAll makes it on this goroutine.
	ended := make([]bool, len(cases))
	for i, tc := range cases {
		_, release := streams.arm(tc.uris, "owner", nil, func() { ended[i] = true })
		t.Cleanup(release)
	}

	streams.closeAll()

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !ended[i] {
				t.Error("closeAll left the stream open; its handler would block shutdown until the drain deadline")
			}
		})
	}
}

// TestListenStreams_CloseOwner_EndsOnlyThatCredentialsStreams covers what an
// evicted credential's clients are told.
//
// Eviction stops the credential's watchers through Manager.Close, which fires no
// OnStop by contract, so nothing reaches stoppedFor and the open
// subscriptions/listen was left neither closed nor completed: the client went on
// holding a stream that would never speak again. This is what ends them, and it
// ends the list-changed ones too, since those name no URI and nothing else could
// ever close them. What it must not touch is another credential's stream on the
// same shared server.
func TestListenStreams_CloseOwner_EndsOnlyThatCredentialsStreams(t *testing.T) {
	streams := newListenStreams()

	cases := []struct {
		name      string
		owner     string
		uris      []string
		wantEnded bool
	}{
		{
			name:      "the evicted credential's resource stream",
			owner:     "owner-evicted",
			uris:      []string{"gitlab://project/42/pipeline/99"},
			wantEnded: true,
		},
		{
			name:      "the evicted credential's list-changed stream, which names no URI",
			owner:     "owner-evicted",
			wantEnded: true,
		},
		{
			name:  "another credential's stream over the same URI",
			owner: "owner-still-pooled",
			uris:  []string{"gitlab://project/42/pipeline/99"},
		},
	}

	ended := make([]bool, len(cases))
	for i, tc := range cases {
		_, release := streams.arm(tc.uris, tc.owner, nil, func() { ended[i] = true })
		t.Cleanup(release)
	}

	streams.closeOwner("owner-evicted", endOfCredentialEviction)

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if ended[i] != tc.wantEnded {
				t.Errorf("stream ended = %v, want %v", ended[i], tc.wantEnded)
			}
		})
	}
}

// TestListenStreams_CloseOwner_ReportsTheSessionsItGaveAnEndingTo covers the
// answer eviction reads to decide which sessions still need telling.
//
// A session whose stream this cancelled is already being told: the SDK writes
// the stream's completion result as the handler unwinds, which is the graceful
// ending the specification asks for. Terminating such a session as well would
// race that write. A session-era resources/subscribe holds no stream, appears
// in no answer here, and is exactly what [sessionOwners.endSessionsWithoutStreams]
// then has to end.
func TestListenStreams_CloseOwner_ReportsTheSessionsItGaveAnEndingTo(t *testing.T) {
	streams := newListenStreams()
	session, _ := connectedSessions(t)

	// One stream with a session, one without: the process-wide sentinel and
	// every stdio stream carry none, and a nil must not be reported as a
	// session that was told.
	_, releaseWith := streams.arm([]string{"uri"}, "owner-evicted", session, func() {})
	t.Cleanup(releaseWith)
	_, releaseWithout := streams.arm([]string{"uri"}, "owner-evicted", nil, func() {})
	t.Cleanup(releaseWithout)

	told := streams.closeOwner("owner-evicted", endOfCredentialEviction)

	if len(told) != 1 {
		t.Fatalf("closeOwner reported %d sessions, want 1: a session it never ended a stream on "+
			"would be left out of the only ending it can still be given", len(told))
	}
	if _, ok := told[session]; !ok {
		t.Error("closeOwner reported a session other than the one whose stream it ended")
	}
}

// TestListenStreams_CloseOwner_WithoutAnOwner_EndsNothing covers the guard in
// front of it.
//
// Every stream on a single-credential server carries the empty owner, which is
// also what an unattributed request would produce, so treating "" as a match
// would let one mistake end every open subscription in the process.
func TestListenStreams_CloseOwner_WithoutAnOwner_EndsNothing(t *testing.T) {
	streams := newListenStreams()
	ended := false
	_, release := streams.arm([]string{"uri"}, "", nil, func() { ended = true })
	t.Cleanup(release)

	streams.closeOwner("", endOfCredentialEviction)
	var absent *listenStreams
	absent.closeOwner("owner", endOfCredentialEviction)

	if ended {
		t.Error("closeOwner(\"\") ended a stream; on stdio that is every stream there is")
	}
}

// TestListenStreams_ArmedAfterCloseAll_EndsImmediately verifies a listen that
// arrives during the drain is ended rather than filed away.
//
// Shutdown stops the listener and lets in-flight requests finish, so a request
// already on the wire can reach this middleware after closeAll has emptied the
// registry. Registering it there would put it beyond anyone's reach a second
// time, and one such stream is enough to hold the process open for the whole
// shutdown budget, which is the exact outcome closeAll exists to prevent.
func TestListenStreams_ArmedAfterCloseAll_EndsImmediately(t *testing.T) {
	streams := newListenStreams()
	streams.closeAll()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, release := streams.arm(nil, "owner", nil, cancel)
	defer release()

	if ctx.Err() == nil {
		t.Error("a listen armed after shutdown was left open, so its handler would never return")
	}
}

// TestClosableListenURIs_MixedStream_IsLeftAlone verifies which listen
// requests may be ended from the server side.
//
// A stream that also carries list-changed subscriptions must survive: those
// are still valid, and canceling the request would take them down with the
// resources that went away.
func TestClosableListenURIs_MixedStream_IsLeftAlone(t *testing.T) {
	listenReq := func(n *mcp.NotificationSubscriptions) mcp.Request {
		return &mcp.SubscriptionsListenRequest{Params: &mcp.SubscriptionsListenParams{Notifications: n}}
	}
	tests := []struct {
		name   string
		method string
		req    mcp.Request
		want   bool
	}{
		{
			name:   "resources only",
			method: methodSubscriptionsListen,
			req:    listenReq(&mcp.NotificationSubscriptions{ResourceSubscriptions: []string{"a"}}),
			want:   true,
		},
		{
			name:   "resources plus tools list-changed",
			method: methodSubscriptionsListen,
			req: listenReq(&mcp.NotificationSubscriptions{
				ResourceSubscriptions: []string{"a"},
				ToolsListChanged:      true,
			}),
			want: false,
		},
		{
			name:   "no resources",
			method: methodSubscriptionsListen,
			req:    listenReq(&mcp.NotificationSubscriptions{ResourcesListChanged: true}),
			want:   false,
		},
		{
			name:   "no notifications block",
			method: methodSubscriptionsListen,
			req:    listenReq(nil),
			want:   false,
		},
		{
			name:   "another method",
			method: "tools/call",
			req:    listenReq(&mcp.NotificationSubscriptions{ResourceSubscriptions: []string{"a"}}),
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, got := closableListenURIs(tt.method, tt.req)
			if got != tt.want {
				t.Errorf("closableListenURIs() closable = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSubscriptionRuntime_WatchStops_EndsTheStream verifies the manager's
// stop reaches the stream that was waiting on it.
//
// This is the wire between the two halves: the watcher discovers the
// resource is gone, and the client's open subscription request is answered
// instead of being left hanging against a server that stopped watching.
func TestSubscriptionRuntime_WatchStops_EndsTheStream(t *testing.T) {
	backend, gitlab := newPipelineBackend(t, "running")
	shape, runtime := newTestSubscriptions(subscriptionGitLabClient(t, gitlab.URL),
		subscriptionCfg(config.CapabilitySurfaceFull), fastOptions())
	t.Cleanup(runtime.manager.Close)

	const uri = "gitlab://project/42/pipeline/99"
	if err := runtime.manager.Subscribe(context.Background(), testSession, uri); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Armed under the same owner the runtime watches as, which is what scopes
	// the stop to this credential's streams.
	_, release := shape.streams.arm([]string{uri}, "", nil, cancel)
	defer release()

	// The pipeline becomes unreadable: deleted, or this token lost access.
	backend.setMissing()

	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the watch stopped but its subscription stream was left open")
	}
}

// TestSubscribe_WatchStops_StreamEndsWithAResult is the proof that the
// graceful close actually happens on the wire.
//
// The specification says a server that tears a subscription down should
// answer the client's open subscriptions/listen request rather than go
// quiet, and the SDK gives application code no way to send that answer
// directly: SubscriptionsListenResult embeds an unexported type. The only
// route is to end the SDK's own handler, and this checks that doing so
// produces a result rather than a cancellation error — if it ever stopped
// working, the server would look like it had crashed mid-subscription
// instead of having finished cleanly.
func TestSubscribe_WatchStops_StreamEndsWithAResult(t *testing.T) {
	type outcome struct {
		result mcp.Result
		err    error
	}
	backend, gitlab := newPipelineBackend(t, "running")
	server := subscriptionTestServer(t, gitlab.URL, config.CapabilitySurfaceFull, fastPolling())

	// Applied last, so it wraps the subscription middleware and sees what
	// the SDK's handler finally returned.
	ended := make(chan outcome, 1)
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method != methodSubscriptionsListen {
				return next(ctx, method, req)
			}
			result, err := next(ctx, method, req)
			select {
			case ended <- outcome{result, err}:
			default:
			}
			return result, err
		}
	})

	session := connectInMemory(t, server)
	if session.InitializeResult().ProtocolVersion < "2026-07-28" {
		t.Skip("the graceful close only exists from protocol 2026-07-28")
	}

	const uri = "gitlab://project/42/pipeline/99"
	if err := session.Subscribe(context.Background(), &mcp.SubscribeParams{URI: uri}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	waitForPolls(t, backend, 2)

	// The pipeline becomes unreadable, so the watch stops for good.
	backend.setMissing()

	select {
	case got := <-ended:
		if got.err != nil {
			t.Fatalf("the subscription stream ended with %v, want the graceful result", got.err)
		}
		if _, ok := got.result.(*mcp.SubscriptionsListenResult); !ok {
			t.Errorf("stream result = %T, want *mcp.SubscriptionsListenResult", got.result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the watch stopped but the client's subscription request never returned")
	}
}

// testSession stands in for the MCP session a subscription belongs to, in
// the tests that drive the *manager* directly. The manager only ever
// compares it, so a bare value is enough there — but never pass one to the
// session bridge, which waits on the session's connection.
var testSession = &mcp.ServerSession{}

// connectedSessions returns a live server/client session pair, for the
// tests that drive the session bridge and therefore need a session that can
// actually be waited on and closed.
func connectedSessions(t *testing.T) (*mcp.ServerSession, *mcp.ClientSession) {
	t.Helper()
	ctx := context.Background()
	host := mcp.NewServer(&mcp.Implementation{Name: "host", Version: "1"}, nil)
	st, ct := mcp.NewInMemoryTransports()
	serverSession, err := host.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	clientSession, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })
	return serverSession, clientSession
}

// subscriptionCfg is the server configuration a runtime is built from,
// reduced to the fields that decide whether subscriptions are offered.
func subscriptionCfg(capabilitySurface string) *config.ServerConfig {
	return &config.ServerConfig{CapabilitySurface: capabilitySurface}
}

// subscriptionGitLabClient builds a GitLab client aimed at a mock backend.
func subscriptionGitLabClient(t *testing.T, gitlabURL string) *gitlabclient.Client {
	t.Helper()
	client, err := gitlabclient.NewClient(&config.Config{GitLabURL: gitlabURL, GitLabToken: testToken})
	if err != nil {
		t.Fatalf("gitlabclient.NewClient: %v", err)
	}
	return client
}

// subscriptionHandlerIndex builds the handler index the watcher reads
// through, against a mock GitLab.
func subscriptionHandlerIndex(t *testing.T, gitlabURL string) resources.HandlerIndex {
	t.Helper()
	return resources.NewHandlerIndex(subscriptionGitLabClient(t, gitlabURL))
}

// newTestReader builds a resource reader with an index of its own, which is
// what [subscriptionShape.newRuntime] hands a watcher.
//
// The index pointer is the shape's, shared by every credential's reader, so a
// reader built as a bare literal has none and setIndex would dereference nil.
func newTestReader(t *testing.T, gitlabURL string) *resourceReader {
	t.Helper()
	reader := &resourceReader{
		index:  &atomic.Pointer[resources.HandlerIndex]{},
		client: subscriptionGitLabClient(t, gitlabURL),
	}
	reader.setIndex(subscriptionHandlerIndex(t, gitlabURL))
	return reader
}

// connectInMemory returns a client session connected to server.
func connectInMemory(t *testing.T, server *mcp.Server) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// TestCreateServer_UndeclaredMethodsRefused verifies the capability guard:
// methods whose capability the handshake withholds answer -32601 instead of
// a hollow success. logging/setLevel is gated on every surface (the server
// never declares logging), and the prompts methods are gated exactly when
// the minimal capability surface withholds the prompts capability.
func TestCreateServer_UndeclaredMethodsRefused(t *testing.T) {
	_, gitlab := newPipelineBackend(t, "running")

	tests := []struct {
		name        string
		surface     string
		call        func(ctx context.Context, s *mcp.ClientSession) error
		wantRefused bool
	}{
		{"logging/setLevel refused on full", config.CapabilitySurfaceFull, setLoggingLevel, true},
		{"logging/setLevel refused on minimal", config.CapabilitySurfaceMinimal, setLoggingLevel, true},
		{"prompts/list served on full", config.CapabilitySurfaceFull, listPrompts, false},
		{"prompts/list refused on minimal", config.CapabilitySurfaceMinimal, listPrompts, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := subscriptionTestServer(t, gitlab.URL, tt.surface)
			session := connectInMemory(t, server)

			err := tt.call(context.Background(), session)
			if !tt.wantRefused {
				if err != nil {
					t.Fatalf("call error = %v, want success: the capability is declared on this surface", err)
				}
				return
			}
			var rpcErr *jsonrpc.Error
			if !errors.As(err, &rpcErr) || rpcErr.Code != jsonrpc.CodeMethodNotFound {
				t.Errorf("call error = %v, want *jsonrpc.Error with code %d for a method whose capability is undeclared", err, jsonrpc.CodeMethodNotFound)
			}
		})
	}
}

func setLoggingLevel(ctx context.Context, s *mcp.ClientSession) error {
	// The deprecated method is called on purpose: the test proves this
	// server refuses it instead of answering a hollow success.
	return s.SetLoggingLevel(ctx, &mcp.SetLoggingLevelParams{Level: "info"}) //nolint:staticcheck // SEP-2577 deprecation is the point under test
}

func listPrompts(ctx context.Context, s *mcp.ClientSession) error {
	_, err := s.ListPrompts(ctx, nil)
	return err
}

// TestWireSubscribeError_MapsSentinelsToCodes verifies each manager
// sentinel leaves the subscribe boundary as a *jsonrpc.Error with a
// deliberate code instead of the accidental code 0 the SDK marshals for a
// plain error, with the message preserved verbatim.
func TestWireSubscribeError_MapsSentinelsToCodes(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int64
	}{
		{"not subscribable is invalid params", fmt.Errorf("%w: gitlab://project/1/issues", subscriptions.ErrNotSubscribable), jsonrpc.CodeInvalidParams},
		{"inaccessible is invalid params", fmt.Errorf("%w: 404", subscriptions.ErrInaccessible), jsonrpc.CodeInvalidParams},
		{"rate limited is server busy", subscriptions.ErrRateLimited, codeServerBusy},
		{"watcher cap is server busy", subscriptions.ErrTooManySubscriptions, codeServerBusy},
		{"manager closed is server busy", subscriptions.ErrClosed, codeServerBusy},
		{"transient failure is internal", errors.New("gitlab: 500"), jsonrpc.CodeInternalError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wireSubscribeError(tt.err)
			var rpcErr *jsonrpc.Error
			if !errors.As(got, &rpcErr) {
				t.Fatalf("wireSubscribeError(%v) = %v, want *jsonrpc.Error", tt.err, got)
			}
			if rpcErr.Code != tt.want {
				t.Errorf("code = %d, want %d", rpcErr.Code, tt.want)
			}
			if rpcErr.Message != tt.err.Error() {
				t.Errorf("message = %q, want the original %q preserved", rpcErr.Message, tt.err.Error())
			}
		})
	}

	t.Run("nil stays nil", func(t *testing.T) {
		if got := wireSubscribeError(nil); got != nil {
			t.Errorf("wireSubscribeError(nil) = %v, want nil", got)
		}
	})
	t.Run("an already-coded error passes through unchanged", func(t *testing.T) {
		if got := wireSubscribeError(errStatelessSubscribe); got != errStatelessSubscribe { //nolint:errorlint // pointer identity is the assertion: the coded error must pass through untouched
			t.Errorf("wireSubscribeError(coded) = %v, want the same *jsonrpc.Error untouched", got)
		}
	})
}

// TestSubscribe_MissingResource_RefusedWithInvalidParamsCode verifies
// the boundary end to end: a stateful subscribe whose authorization read
// finds the resource gone reaches the SDK as invalid params — the code the
// SDK itself answers an unknown resources/read with — not as a plain error
// the wire would carry as code 0.
func TestSubscribe_MissingResource_RefusedWithInvalidParamsCode(t *testing.T) {
	backend, gitlab := newPipelineBackend(t, "running")
	backend.setMissing()
	runtime := newTestRuntime(subscriptionGitLabClient(t, gitlab.URL),
		subscriptionCfg(config.CapabilitySurfaceFull), fastOptions())
	t.Cleanup(runtime.manager.Close)
	bridge := newSessionBridge(runtime.manager)

	session, _ := connectedSessions(t)
	err := bridge.Subscribe(context.Background(), &mcp.SubscribeRequest{
		Session: session,
		Params:  &mcp.SubscribeParams{URI: "gitlab://project/42/pipeline/99"},
	})
	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) || rpcErr.Code != jsonrpc.CodeInvalidParams {
		t.Fatalf("Subscribe(missing) error = %v, want *jsonrpc.Error with code %d", err, jsonrpc.CodeInvalidParams)
	}
	if runtime.manager.Len() != 0 {
		t.Errorf("watchers = %d after a refused subscribe, want 0", runtime.manager.Len())
	}
}

// TestSessionBridge_OneListenTeardownKeepsAnothersWatch pins the identity a
// subscription has at protocol 2026-07-28.
//
// The manager counts subscribers by session, which is right for the legacy
// resources/subscribe: there the session really is the subscription, and
// ADR-0015 says so deliberately. It is wrong for a listen. That revision makes
// the listen request the subscription's identity, a session may open several,
// and the SDK unsubscribes every URI a listen carried when that listen ends —
// so with the session as the only identity, the first stream to close released
// a watch its sibling was still holding. The sibling's stream stayed open and
// acknowledged and could never fire again, which is worse than an error.
//
// The bridge is driven directly because the SDK's own per-session table would
// swallow the second subscribe before it reached this server, hiding the case.
// What a real client reaches is one Subscribe per listen stream, and nothing
// stops it opening two for one URI.
func TestSessionBridge_OneListenTeardownKeepsAnothersWatch(t *testing.T) {
	_, gitlab := newPipelineBackend(t, "running")
	runtime := newTestRuntime(subscriptionGitLabClient(t, gitlab.URL),
		subscriptionCfg(config.CapabilitySurfaceFull), fastOptions())
	t.Cleanup(runtime.manager.Close)
	bridge := newSessionBridge(runtime.manager)
	session, _ := connectedSessions(t)

	const uri = "gitlab://project/42/pipeline/99"

	// Two listen streams, as two subscriptions/listen requests on one session.
	first := &listenStream{cancel: func() {}}
	second := &listenStream{cancel: func() {}}
	// sequential: the second listen joins the watcher the first one created, and the teardown below needs both in place
	for _, stream := range []*listenStream{first, second} {
		ctx := withListenStream(context.Background(), stream)
		if err := bridge.Subscribe(ctx, &mcp.SubscribeRequest{
			Session: session,
			Params:  &mcp.SubscribeParams{URI: uri},
		}); err != nil {
			t.Fatalf("Subscribe: %v", err)
		}
	}
	if got := runtime.manager.Len(); got != 1 {
		t.Fatalf("watchers = %d, want 1: two listens on one URI share a watcher", got)
	}

	// The first stream ends. The second is still listening.
	if err := bridge.Unsubscribe(withListenStream(context.Background(), first),
		&mcp.UnsubscribeRequest{Session: session, Params: &mcp.UnsubscribeParams{URI: uri}}); err != nil {
		t.Fatalf("Unsubscribe(first): %v", err)
	}
	if got := runtime.manager.Len(); got != 1 {
		t.Fatalf("watchers = %d after one listen ended, want 1: the surviving stream can never fire", got)
	}

	// The second ends too, and now nothing is holding it.
	if err := bridge.Unsubscribe(withListenStream(context.Background(), second),
		&mcp.UnsubscribeRequest{Session: session, Params: &mcp.UnsubscribeParams{URI: uri}}); err != nil {
		t.Fatalf("Unsubscribe(second): %v", err)
	}
	if got := runtime.manager.Len(); got != 0 {
		t.Errorf("watchers = %d after every listen ended, want 0: the watch outlived its subscribers", got)
	}
}

// TestSessionBridge_LegacySubscribeIsStillSessionScoped checks that the holder
// tracking did not change the older method's contract.
//
// ADR-0015's "a subscriber is an identity, not a count" still governs
// resources/subscribe, where a session subscribing twice is idempotent and one
// unsubscribe releases it. Only the listen path needed a finer identity.
func TestSessionBridge_LegacySubscribeIsStillSessionScoped(t *testing.T) {
	_, gitlab := newPipelineBackend(t, "running")
	runtime := newTestRuntime(subscriptionGitLabClient(t, gitlab.URL),
		subscriptionCfg(config.CapabilitySurfaceFull), fastOptions())
	t.Cleanup(runtime.manager.Close)
	bridge := newSessionBridge(runtime.manager)
	session, _ := connectedSessions(t)

	const uri = "gitlab://project/42/pipeline/99"
	ctx := context.Background()
	for range 2 {
		if err := bridge.Subscribe(ctx, &mcp.SubscribeRequest{
			Session: session,
			Params:  &mcp.SubscribeParams{URI: uri},
		}); err != nil {
			t.Fatalf("Subscribe: %v", err)
		}
	}

	if err := bridge.Unsubscribe(ctx, &mcp.UnsubscribeRequest{
		Session: session,
		Params:  &mcp.UnsubscribeParams{URI: uri},
	}); err != nil {
		t.Fatalf("Unsubscribe: %v", err)
	}
	if got := runtime.manager.Len(); got != 0 {
		t.Errorf("watchers = %d, want 0: one unsubscribe releases a session's own subscription", got)
	}
}

// TestSessionBridge_AnOpenListenKeepsItsWatchAtFullSpeed pins the renewal
// signal the default transport actually has.
//
// The lease asks "is anyone still there", and answers it from request traffic
// on the session. Under stateless HTTP that evidence does not exist: every POST
// is its own session, so a listen stream's session sees one request — the
// listen — and nothing renews it afterwards. The watch would drop to the slow
// poll half an hour in while the client sat on an open stream it was still
// reading, and nothing would say so.
//
// This became reachable only once subscriptions/listen worked at all, which is
// why it is fixed alongside: the feature would otherwise have shipped and
// quietly degraded by a factor of forty after thirty minutes.
//
// The renewal is observed rather than the clock. This test used to sleep for
// several short leases and then assert that no watch had been demoted, which
// failed on a loaded macOS runner: with a 150 ms lease, the renewal goroutine
// only had to be starved for 100 ms for the watch to slow down, and the test
// reported a defect that was not there. Now the lease is long enough that only
// a real failure to renew could demote the watch, and the assertion waits for
// the bridge's own renewal count to move, so what is checked is that the open
// stream renews and that the renewals keep the watch at full speed.
func TestSessionBridge_AnOpenListenKeepsItsWatchAtFullSpeed(t *testing.T) {
	_, gitlab := newPipelineBackend(t, "running")
	opts := fastOptions()
	// The stream renews every Lease/3; a lease of three seconds gives a
	// renewal every second and two seconds of slack before demotion, which no
	// scheduler starvation a runner produces gets near.
	opts.Lease = 3 * time.Second
	runtime := newTestRuntime(subscriptionGitLabClient(t, gitlab.URL),
		subscriptionCfg(config.CapabilitySurfaceFull), opts)
	t.Cleanup(runtime.manager.Close)
	bridge := newSessionBridge(runtime.manager)
	session, _ := connectedSessions(t)

	streamCtx, endStream := context.WithCancel(t.Context())
	defer endStream()

	ctx := withListenStream(streamCtx, &listenStream{cancel: func() {}})
	if err := bridge.Subscribe(ctx, &mcp.SubscribeRequest{
		Session: session,
		Params:  &mcp.SubscribeParams{URI: "gitlab://project/42/pipeline/99"},
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// Two renewals prove the ticker is running and renewing; with the lease
	// above they take about two seconds, and the deadline is generous because
	// a slow renewal is exactly what must not fail this test.
	const wantRenewals = 2
	deadline := time.Now().Add(20 * time.Second)
	for bridge.streamRenewals() < wantRenewals {
		if time.Now().After(deadline) {
			t.Fatalf("the listen stream renewed %d time(s) in 20 s, want at least %d; the renewal ticker is not running",
				bridge.streamRenewals(), wantRenewals)
		}
		time.Sleep(20 * time.Millisecond)
	}

	if runtime.manager.Len() != 1 {
		t.Fatalf("the watch is gone entirely; this test can no longer see what it checks")
	}
	if demoted := runtime.manager.DemotedCount(); demoted != 0 {
		t.Errorf("%d watch(es) demoted while the listen stream was open and renewing", demoted)
	}
}

// TestSessionBridge_ALegacySubscribeStillFollowsTheLease checks that the stream
// renewal did not quietly exempt everything from the lease.
//
// A resources/subscribe has no stream to stand as evidence, and its session
// does see ordinary request traffic, so it keeps the original rule: go quiet
// long enough and the watch slows down.
func TestSessionBridge_ALegacySubscribeStillFollowsTheLease(t *testing.T) {
	_, gitlab := newPipelineBackend(t, "running")
	opts := fastOptions()
	opts.Lease = 100 * time.Millisecond
	runtime := newTestRuntime(subscriptionGitLabClient(t, gitlab.URL),
		subscriptionCfg(config.CapabilitySurfaceFull), opts)
	t.Cleanup(runtime.manager.Close)
	bridge := newSessionBridge(runtime.manager)
	session, _ := connectedSessions(t)

	if err := bridge.Subscribe(context.Background(), &mcp.SubscribeRequest{
		Session: session,
		Params:  &mcp.SubscribeParams{URI: "gitlab://project/42/pipeline/99"},
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// A silent lease of 100ms runs out well inside the budget; a watch that
	// is never demoted is what the deadline reports.
	deadline := time.Now().Add(5 * time.Second)
	for runtime.manager.DemotedCount() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("a silent legacy subscription was never demoted; the lease no longer applies to anything")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestSessionBridge_OneRenewalTickerPerStream pins that a listen carrying
// several resources starts one renewal ticker rather than one per resource.
//
// The SDK calls SubscribeHandler once for every URI a listen names, and
// RenewAll renews every watch the session holds — so a second ticker wakes up
// on the same schedule to redo what the first just did. Harmless in ones and
// twos, and the kind of thing that only shows up as load once a client
// subscribes to a dozen resources at a time.
func TestSessionBridge_OneRenewalTickerPerStream(t *testing.T) {
	_, gitlab := newPipelineBackend(t, "running")
	runtime := newTestRuntime(subscriptionGitLabClient(t, gitlab.URL),
		subscriptionCfg(config.CapabilitySurfaceFull), fastOptions())
	t.Cleanup(runtime.manager.Close)
	bridge := newSessionBridge(runtime.manager)
	session, _ := connectedSessions(t)

	streamCtx, endStream := context.WithCancel(t.Context())
	defer endStream()
	stream := &listenStream{cancel: func() {}}
	ctx := withListenStream(streamCtx, stream)

	// The SDK calls SubscribeHandler once per URI a listen names, so what
	// decides how many tickers start is how many times Subscribe runs on one
	// stream — not which URIs they were for. Three calls stand in for a listen
	// carrying three resources.
	const uri = "gitlab://project/42/pipeline/99"
	subscribe := func() {
		t.Helper()
		if err := bridge.Subscribe(ctx, &mcp.SubscribeRequest{
			Session: session,
			Params:  &mcp.SubscribeParams{URI: uri},
		}); err != nil {
			t.Fatalf("Subscribe: %v", err)
		}
	}

	subscribe()

	// Two more, standing in for the rest of a listen's URIs. Neither may add a
	// ticker: RenewAll already renews every watch the session holds.
	subscribe()
	subscribe()

	// The bridge counts its own tickers, so this is exact rather than sampled:
	// a process-wide goroutine count can move for reasons that have nothing to
	// do with this test, and would both miss a duplicate and invent one.
	if got := bridge.activeRenewals(); got != 1 {
		t.Errorf("%d renewal tickers running for one stream, want 1", got)
	}

	// The claim itself is the mechanism, and it must refuse a repeat.
	if bridge.startRenewing(stream) {
		t.Error("startRenewing granted a second ticker for a stream that already has one")
	}

	// And the ticker must end with its stream, or a long-lived server would
	// accumulate one per subscription it ever served.
	endStream()
	deadline := time.Now().Add(5 * time.Second)
	for bridge.activeRenewals() != 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := bridge.activeRenewals(); got != 0 {
		t.Errorf("%d renewal ticker(s) still running after the stream ended", got)
	}
}

// TestListenLimit_CapsConcurrentStreams verifies that concurrent
// subscriptions/listen requests are bounded per server and process-wide, that
// the refusal is the busy code rather than a closed stream, and that streams
// under the cap stay open.
//
// Each listen blocks server-side until its context ends, holding a goroutine,
// an ephemeral session and transport, and a file descriptor. The documented
// valve, MaxWatchers, counts only resource watchers created through
// manager.Subscribe: a listen that asks only for list-changed notifications
// creates none, and one that carries a resource joins an existing watcher's
// subscriber set with no cap check either — so stream count has never been
// bounded on any path. Measured at 2000 concurrent streams on one token:
// 2007 descriptors and 55 to 100 KB retained each, about tenfold what an idle
// connection costs. The last assertion is the one that matters: a "fix" that
// closed every stream would satisfy the cap and destroy the feature.
func TestListenLimit_CapsConcurrentStreams(t *testing.T) {
	for _, tc := range []struct {
		name          string
		perCredential int
		perProcess    int
		streams       int
		wantServed    int
		wantRefused   int
	}{
		{"under_the_per_credential_cap", 4, 100, 3, 3, 0},
		{"at_the_per_credential_cap", 4, 100, 4, 4, 0},
		{"over_the_per_credential_cap", 4, 100, 7, 4, 3},
		{"the_process_cap_binds_first", 100, 2, 5, 2, 3},
		{"disabled_by_a_non_positive_cap", 0, 0, 6, 6, 0},
		{"an_unattributed_request_is_bounded_by_the_process_cap_alone", 1, 2, 5, 2, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			limits := listenLimits{
				perCredential: tc.perCredential,
				perProcess:    tc.perProcess,
				processOpen:   &listenCounter{},
			}

			// A nil counter is what a request nothing could attribute to a
			// credential resolves to, and the last case drives exactly that:
			// the per-credential ceiling cannot be charged to anybody, so only
			// the process-wide one binds.
			counter := &listenCounter{}
			if tc.name == "an_unattributed_request_is_bounded_by_the_process_cap_alone" {
				counter = nil
			}

			held := make(chan struct{})
			var inFlight atomic.Int64
			handler := limits.middleware(staticCounter(counter))(func(ctx context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
				inFlight.Add(1)
				defer inFlight.Add(-1)
				<-held
				return &mcp.CallToolResult{}, nil
			})

			var served, refused atomic.Int64
			var wg sync.WaitGroup
			for range tc.streams {
				wg.Go(func() {
					_, err := handler(t.Context(), methodSubscriptionsListen, nil)
					if err != nil {
						refused.Add(1)
						return
					}
					served.Add(1)
				})
			}

			// Let the refusals land while the served streams are still held.
			waitFor(t, func() bool {
				return refused.Load() == int64(tc.wantRefused) && inFlight.Load() == int64(tc.wantServed)
			})
			if got := inFlight.Load(); got != int64(tc.wantServed) {
				t.Errorf("streams held open = %d, want %d", got, tc.wantServed)
			}
			close(held)
			wg.Wait()

			if got := served.Load(); got != int64(tc.wantServed) {
				t.Errorf("streams served = %d, want %d", got, tc.wantServed)
			}
			if got := refused.Load(); got != int64(tc.wantRefused) {
				t.Errorf("streams refused = %d, want %d", got, tc.wantRefused)
			}
		})
	}
}

// TestListenLimit_RefusalCarriesTheBusyCode verifies the wire shape of the
// refusal: -32000, the code this server already uses for state-dependent
// refusals a retry can clear, rather than an invalid-params verdict on a
// request that was well formed.
func TestListenLimit_RefusalCarriesTheBusyCode(t *testing.T) {
	limits := listenLimits{
		perCredential: 1,
		perProcess:    1,
		processOpen:   &listenCounter{},
	}
	credential := &listenCounter{}
	held := make(chan struct{})
	defer close(held)
	handler := limits.middleware(staticCounter(credential))(func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		<-held
		return &mcp.CallToolResult{}, nil
	})

	opened := make(chan struct{})
	go func() {
		close(opened)
		_, _ = handler(t.Context(), methodSubscriptionsListen, nil)
	}()
	<-opened
	waitFor(t, func() bool { return credential.count() == 1 })

	_, err := handler(t.Context(), methodSubscriptionsListen, nil)
	if err == nil {
		t.Fatal("a stream over the cap was accepted")
	}
	wireErr, ok := errors.AsType[*jsonrpc.Error](err)
	if !ok {
		t.Fatalf("refusal is %T, want *jsonrpc.Error so the code reaches the client", err)
	}
	if wireErr.Code != codeServerBusy {
		t.Errorf("code = %d, want %d", wireErr.Code, codeServerBusy)
	}
	if !strings.Contains(wireErr.Message, "subscriptions/listen") {
		t.Errorf("message = %q, want it to name the method that was refused", wireErr.Message)
	}
}

// TestListenLimit_OtherMethodsAreNotCounted verifies that the cap applies to
// subscriptions/listen alone: an ordinary request must not consume a slot, or
// a busy server would start refusing subscriptions for reasons unrelated to
// them.
func TestListenLimit_OtherMethodsAreNotCounted(t *testing.T) {
	limits := listenLimits{
		perCredential: 1,
		perProcess:    1,
		processOpen:   &listenCounter{},
	}
	credential := &listenCounter{}
	handler := limits.middleware(staticCounter(credential))(func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		return &mcp.CallToolResult{}, nil
	})
	for _, method := range []string{"tools/call", "tools/list", "resources/read", "ping"} {
		t.Run(method, func(t *testing.T) {
			if _, err := handler(t.Context(), method, nil); err != nil {
				t.Fatalf("%s: %v", method, err)
			}
			if open := credential.count(); open != 0 {
				t.Errorf("%s left %d listen slots taken, want 0", method, open)
			}
		})
	}
}

// TestNewSubscriptionShape_TakesTheProcessWatcherCeiling verifies production
// gets the shared ceiling and a test can still bring its own.
//
// There is one place a manager is built, and it takes its gate from the shape,
// so this is the whole of the wiring: a shape without the gate would leave
// every watcher in the process unbounded except per credential, which is the
// hole on --stateless=false that issue 561 asked to close.
func TestNewSubscriptionShape_TakesTheProcessWatcherCeiling(t *testing.T) {
	_, gitlab := newPipelineBackend(t, "running")
	client := subscriptionGitLabClient(t, gitlab.URL)
	cfg := subscriptionCfg(config.CapabilitySurfaceFull)

	t.Run("nothing injected takes the process ceiling", func(t *testing.T) {
		shape := newSubscriptionShape(client, cfg, subscriptions.Options{})
		if shape.opts.SharedWatchers != processWatchers {
			t.Error("the shape did not take the process watcher ceiling, so nothing bounds watchers across credentials")
		}
	})

	t.Run("an injected gate is kept", func(t *testing.T) {
		opts := fastOptions()
		opts.SharedWatchers = subscriptions.NewWatcherGate(1)
		shape := newSubscriptionShape(client, cfg, opts)
		if shape.opts.SharedWatchers != opts.SharedWatchers {
			t.Error("the caller's gate was replaced, so a test cannot reach the ceiling without opening 512 subscriptions")
		}
	})
}

// TestWatcherCeiling_IsTheSameNumberAsTheStreamCeiling pins the arithmetic the
// operator's lever is documented with.
//
// A pooled entry counts as busy when it holds a listen stream or a watcher, so
// the number of busy entries at rest cannot exceed the sum of the two
// process-wide ceilings, and --max-http-clients above that sum makes the pool's
// busy eviction fallback unreachable. The guides state that sum as one number.
// Raising one ceiling alone would leave them stating the wrong one.
func TestWatcherCeiling_IsTheSameNumberAsTheStreamCeiling(t *testing.T) {
	if maxWatchersPerProcess != maxListenStreamsPerProcess {
		t.Errorf("watcher ceiling %d, stream ceiling %d: the documented lever is their sum, so both docs move with either",
			maxWatchersPerProcess, maxListenStreamsPerProcess)
	}
}

// TestSubscriptionShape_TheWatcherCeilingBindsAcrossCredentials verifies the
// gate reaches every credential's manager, and that reaching it refuses the
// newcomer instead of taking somebody else's watch.
//
// One credential per manager is the shape ADR-0020 builds, and a credential is
// one API call to mint, so this is the ceiling that actually bounds the
// process. The refusal is checked as the client would see it, since a sentinel
// the wire renders as code 0 would reach a client as "unknown error".
func TestSubscriptionShape_TheWatcherCeilingBindsAcrossCredentials(t *testing.T) {
	_, gitlab := newPipelineBackend(t, "running")
	client := subscriptionGitLabClient(t, gitlab.URL)
	opts := fastOptions()
	opts.SharedWatchers = subscriptions.NewWatcherGate(1)
	shape := newSubscriptionShape(client, subscriptionCfg(config.CapabilitySurfaceFull), opts)

	holder := shape.newRuntime("owner-holding", client)
	t.Cleanup(holder.manager.Close)
	newcomer := shape.newRuntime("owner-arriving", client)
	t.Cleanup(newcomer.manager.Close)

	if err := holder.manager.Subscribe(t.Context(), testSession, "gitlab://project/42/pipeline/99"); err != nil {
		t.Fatalf("the holding credential's subscribe: %v", err)
	}

	err := newcomer.manager.Subscribe(t.Context(), testSession, "gitlab://project/43/pipeline/99")
	if !errors.Is(err, subscriptions.ErrTooManySubscriptions) {
		t.Fatalf("a second credential's subscribe past the shared ceiling = %v, want ErrTooManySubscriptions", err)
	}
	var wireErr *jsonrpc.Error
	if !errors.As(wireSubscribeError(err), &wireErr) {
		t.Fatalf("the refusal reaches the wire as %T, want a *jsonrpc.Error carrying a deliberate code", err)
	}
	if wireErr.Code != codeServerBusy {
		t.Errorf("code = %d, want %d: the request is well formed and a later retry can succeed",
			wireErr.Code, codeServerBusy)
	}
	if !strings.Contains(wireErr.Message, "server-wide") {
		t.Errorf("message = %q, want it to name which ceiling was reached", wireErr.Message)
	}
	if got := holder.manager.Len(); got != 1 {
		t.Errorf("the holding credential keeps %d watchers, want 1: no watch of another credential may be taken", got)
	}
	if got := newcomer.manager.Len(); got != 0 {
		t.Errorf("the refused credential holds %d watchers, want 0", got)
	}
}

// TestListenLimitsFromEnv verifies that the ceilings are configurable and that
// anything that is not a positive number leaves the defaults in place.
func TestListenLimitsFromEnv(t *testing.T) {
	for _, tc := range []struct {
		name              string
		configured        string
		wantPerCredential int
		wantPerProcess    int
	}{
		{"unset", "", maxListenStreamsPerServer, maxListenStreamsPerProcess},
		{"explicit", "8", 8, maxListenStreamsPerProcess},
		{"zero_disables", "0", 0, maxListenStreamsPerProcess},
		{"garbage_keeps_the_default", "many", maxListenStreamsPerServer, maxListenStreamsPerProcess},
		{"negative_keeps_the_default", "-1", maxListenStreamsPerServer, maxListenStreamsPerProcess},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.configured != "" {
				t.Setenv(maxListenStreamsEnv, tc.configured)
			}
			got := listenLimitsFromEnv()
			if got.perCredential != tc.wantPerCredential {
				t.Errorf("perCredential = %d, want %d", got.perCredential, tc.wantPerCredential)
			}
			if got.perProcess != tc.wantPerProcess {
				t.Errorf("perProcess = %d, want %d", got.perProcess, tc.wantPerProcess)
			}
			// The per-credential counter is no longer a field: one server
			// answers for every credential of a configuration shape, so a
			// counter held here would be one budget shared by every tenant.
			if got.processOpen == nil {
				t.Error("the process counter must be non-nil, or the middleware cannot count")
			}
		})
	}
}

// waitFor blocks until cond holds or the test's budget runs out, so a test
// asserting on concurrent state does not race the goroutines it started.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Error("condition never held within the budget")
}

// TestExcludedActions_NarrowResourcesAndSubscriptions_NotOnlyTools verifies
// that --exclude-tools reaches every request path to the same GitLab object.
//
// The tool surface is not the only one. A resource template returns the same
// data through the same credential, and a subscription polls it on a schedule,
// so an operator who removes an action and finds it still readable through
// resources/read has been given a guard that does not guard. The mechanism for
// this lived in internal/resources, fully tested, and cmd/server never passed
// it anything, which is the state this test exists to prevent returning to.
//
// It names the action and the URI rather than counting handlers. A count proves
// only that something was removed, and would pass just as well if the excluded
// action stayed reachable while an unrelated resource disappeared.
func TestExcludedActions_NarrowResourcesAndSubscriptions_NotOnlyTools(t *testing.T) {
	// The catalog resolves the operator's spelling, so ask it rather than
	// hardcoding an action ID that a rename would silently invalidate.
	catalog, err := gitlabtools.BuildActionCatalog(nil, gitlabtools.ActionCatalogOptions{
		Tier:       edition.Free,
		IncludeMCP: true,
	})
	if err != nil {
		t.Fatalf("building the catalog: %v", err)
	}

	// project.get backs gitlab://project/{project_id}, which is the pairing
	// this whole mechanism exists for: the same object, two request paths.
	const (
		excludedTool = "gitlab_project"
		excludedID   = "project.get"
		excludedURI  = "gitlab://project/{project_id}"
	)

	excluded := catalog.ExcludedActionIDs([]string{excludedTool})
	if !slices.Contains(excluded, excludedID) {
		t.Fatalf("excluding %q did not remove %q, so this test is not measuring what it claims: %v",
			excludedTool, excludedID, excluded)
	}

	full := resources.NewHandlerIndex(nil)
	narrowed := resources.NewHandlerIndex(nil, resources.RegisterOptions{ExcludedActions: excluded})

	if _, ok := full[excludedURI]; !ok {
		t.Fatalf("%s is not registered at all, so its absence below would prove nothing", excludedURI)
	}
	if _, ok := narrowed[excludedURI]; ok {
		t.Errorf("%s is still registered after excluding %q, so resources/read still serves what the operator removed",
			excludedURI, excludedTool)
	}

	// A resource backed by no excluded action must survive, or the narrowing
	// is removing more than it was asked to.
	const keptURI = "gitlab://user/current"
	if _, ok := narrowed[keptURI]; !ok {
		t.Errorf("%s was removed too; excluding %q must not take unrelated resources with it", keptURI, excludedTool)
	}

	// The subscription path reads through this index, so it has to refuse the
	// excluded URI as well: a client that knows the URI never calls
	// resources/list and would otherwise still be served. The index cell comes
	// from the shape, so a reader assembled here has to be given one.
	reader := &resourceReader{index: &atomic.Pointer[resources.HandlerIndex]{}}
	reader.setIndex(narrowed)
	if _, readErr := reader.Read(t.Context(), "gitlab://project/42"); readErr == nil {
		t.Error("the subscription reader served an excluded resource, so a subscription would poll GitLab for it")
	}
}

// TestExcludeTools_RemovesTheResourceFromTheServerItself covers the wiring
// rather than the mechanism.
//
// internal/resources has always been able to narrow itself, with its own tests.
// What was missing was cmd/server passing it anything, so an operator's
// exclusion reached the tool surface and stopped there. A test that calls
// NewHandlerIndex directly cannot see that: it passes whether or not the server
// is wired up, which is exactly how the gap survived being tested.
//
// So this builds the real server through createServer with ExcludeTools set,
// and asks it over a session, the way a client would.
func TestExcludeTools_RemovesTheResourceFromTheServerItself(t *testing.T) {
	const (
		excludedTool = "gitlab_project"
		excludedURI  = "gitlab://project/{project_id}"
		keptURI      = "gitlab://user/current"
	)

	// Two servers built the same way apart from the exclusion, so any
	// difference between their listings is the exclusion and nothing else.
	base := &config.ServerConfig{
		ToolSurface:       config.ToolSurfaceDynamic,
		CapabilitySurface: config.CapabilitySurfaceFull,
		Tier:              edition.Free,
		TierExplicit:      true,
	}
	excludedCfg := *base
	excludedCfg.ExcludeTools = []string{excludedTool}

	client := sharedCreateServerClient(t)
	fullURIs := resourceTemplateURIs(t, mustCreateServer(t, client, base))
	narrowedURIs := resourceTemplateURIs(t, mustCreateServer(t, client, &excludedCfg))

	if !fullURIs[excludedURI] {
		t.Fatalf("%s is not served without the exclusion, so its absence below would prove nothing", excludedURI)
	}
	if narrowedURIs[excludedURI] {
		t.Errorf("%s is still served after excluding %q: the exclusion reached the tools and not the resources",
			excludedURI, excludedTool)
	}
	if !narrowedURIs[keptURI] {
		t.Errorf("%s was removed too; excluding %q must not take unrelated resources with it", keptURI, excludedTool)
	}
}

// resourceTemplateURIs lists the resource template URIs a server serves.
//
// Templates rather than resources: the ones this exclusion removes are
// parameterised (gitlab://project/{project_id}), so they appear in
// resources/templates/list rather than resources/list.
func resourceTemplateURIs(t *testing.T, server *mcp.Server) map[string]bool {
	t.Helper()

	session := newInMemorySession(t, server)
	uris := map[string]bool{}

	templates, err := session.ListResourceTemplates(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListResourceTemplates: %v", err)
	}
	for _, tmpl := range templates.ResourceTemplates {
		uris[tmpl.URITemplate] = true
	}

	listed, err := session.ListResources(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	for _, r := range listed.Resources {
		uris[r.URI] = true
	}
	return uris
}

// TestResourceReader_WithoutAnIndex_AnswersNotReadyInsteadOfPanicking covers
// the reader before registration has published an index. The shape seeds one,
// so nothing built through it gets here; the point is that a reader that
// somehow does answers with the retryable "not ready" rather than
// dereferencing nil inside a watcher.
//
// The index cell itself is supplied, and has to be: it belongs to the shape and
// is shared by every credential's reader, so a bare &resourceReader{} has no
// cell at all and Read dereferences that instead. Only the unpublished index
// this test is about is reachable from any wiring the server has.
func TestResourceReader_WithoutAnIndex_AnswersNotReadyInsteadOfPanicking(t *testing.T) {
	t.Parallel()
	reader := &resourceReader{index: &atomic.Pointer[resources.HandlerIndex]{}}
	_, err := reader.Read(context.Background(), "gitlab://project/42/pipeline/99")
	if !errors.Is(err, errCatalogNotReady) {
		t.Errorf("Read() error = %v, want %v", err, errCatalogNotReady)
	}
}

// TestSessionBridge_ForgetStream_IgnoresANilStream pins the nil guard on the
// bookkeeping a renewal ticker drops when it ends: a legacy subscribe has no
// stream, and forgetting nothing must be a no-op rather than a map write
// under a nil key.
func TestSessionBridge_ForgetStream_IgnoresANilStream(t *testing.T) {
	t.Parallel()
	bridge := newSessionBridge(nil)
	bridge.renewing[&listenStream{cancel: func() {}}] = struct{}{}

	bridge.forgetStream(nil)

	if len(bridge.renewing) != 1 {
		t.Errorf("forgetStream(nil) changed the renewal set to %d entries, want the one stream left alone", len(bridge.renewing))
	}
}

// TestSessionBridge_ALeaseTooShortToRenew_StartsNoTicker covers the renewal
// interval collapsing to zero: a lease of a couple of nanoseconds leaves
// nothing to divide into thirds, and a ticker cannot be built on a zero
// interval, so the stream holds its watch without one rather than panicking
// in time.NewTicker.
func TestSessionBridge_ALeaseTooShortToRenew_StartsNoTicker(t *testing.T) {
	_, gitlab := newPipelineBackend(t, "running")
	opts := fastOptions()
	opts.Lease = 2 * time.Nanosecond
	runtime := newTestRuntime(subscriptionGitLabClient(t, gitlab.URL),
		subscriptionCfg(config.CapabilitySurfaceFull), opts)
	t.Cleanup(runtime.manager.Close)
	bridge := newSessionBridge(runtime.manager)
	session, _ := connectedSessions(t)

	streamCtx, endStream := context.WithCancel(t.Context())
	defer endStream()
	ctx := withListenStream(streamCtx, &listenStream{cancel: func() {}})
	if err := bridge.Subscribe(ctx, &mcp.SubscribeRequest{
		Session: session,
		Params:  &mcp.SubscribeParams{URI: "gitlab://project/42/pipeline/99"},
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	if runtime.manager.Len() != 1 {
		t.Fatalf("watchers = %d, want 1: the watch itself is unaffected by the missing ticker", runtime.manager.Len())
	}
	if got := bridge.activeRenewals(); got != 0 {
		t.Errorf("activeRenewals() = %d, want 0 when the lease leaves no interval to renew on", got)
	}
}

// TestSessionBridge_SubscribeUnlessStateless_TellsTheListenPathFromTheLegacyOne
// pins the one distinction the stateless handler draws. The SDK routes both
// resources/subscribe and every URI of a subscriptions/listen through it, and
// only the first is undeliverable on a sessionless transport, so the mark on
// the context is what decides between a real watch and the refusal.
func TestSessionBridge_SubscribeUnlessStateless_TellsTheListenPathFromTheLegacyOne(t *testing.T) {
	_, gitlab := newPipelineBackend(t, "running")
	runtime := newTestRuntime(subscriptionGitLabClient(t, gitlab.URL),
		subscriptionCfg(config.CapabilitySurfaceFull), fastOptions())
	t.Cleanup(runtime.manager.Close)
	bridge := newSessionBridge(runtime.manager)
	session, _ := connectedSessions(t)
	req := &mcp.SubscribeRequest{
		Session: session,
		Params:  &mcp.SubscribeParams{URI: "gitlab://project/42/pipeline/99"},
	}

	if err := bridge.subscribeUnlessStateless(t.Context(), req); !errors.Is(err, errStatelessSubscribe) {
		t.Fatalf("a legacy subscribe on a stateless transport returned %v, want %v", err, errStatelessSubscribe)
	}
	if runtime.manager.Len() != 0 {
		t.Fatalf("watchers = %d after the refusal, want none started", runtime.manager.Len())
	}

	streamCtx, endStream := context.WithCancel(t.Context())
	defer endStream()
	listen := withListenStream(streamCtx, &listenStream{cancel: func() {}})
	if err := bridge.subscribeUnlessStateless(listen, req); err != nil {
		t.Fatalf("a listen-path subscribe was refused: %v", err)
	}
	if runtime.manager.Len() != 1 {
		t.Errorf("watchers = %d after the listen-path subscribe, want 1", runtime.manager.Len())
	}
}

// gitLabStatusError is a GitLab failure carrying an HTTP status, wrapped the
// way [subscriptions.TranslateReadError] wraps one, which is what a watcher's
// stop reason really looks like by the time it reaches the transport layer.
func gitLabStatusError(status int) error {
	upstream := &gl.ErrorResponse{
		Response: &http.Response{StatusCode: status},
		Message:  http.StatusText(status),
	}
	return fmt.Errorf("%w: %w", subscriptions.ErrInaccessible, upstream)
}

// TestWatchEndForStop_TurnsAWatchersEndingIntoWhatTheClientIsTold covers the
// half of the vocabulary the watchers supply.
//
// The status is the part worth pinning. 401, 403 and 404 are deliberately
// collapsed into one outcome by [subscriptions.TranslateReadError], because
// GitLab answers 404 for a resource the caller may not see precisely so that it
// cannot be told apart from one that does not exist. Relaying the status lets a
// client log what happened without letting it conclude "deleted", which is what
// the detail says in words.
func TestWatchEndForStop_TurnsAWatchersEndingIntoWhatTheClientIsTold(t *testing.T) {
	tests := []struct {
		name       string
		reason     error
		wantReason string
		wantStatus int
	}{
		{
			name:       "an unauthorized read",
			reason:     gitLabStatusError(http.StatusUnauthorized),
			wantReason: endResourceGone,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "a forbidden read",
			reason:     gitLabStatusError(http.StatusForbidden),
			wantReason: endResourceGone,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "a resource that is gone or invisible",
			reason:     gitLabStatusError(http.StatusNotFound),
			wantReason: endResourceGone,
			wantStatus: http.StatusNotFound,
		},
		{
			// The MCP not-found error carries no HTTP status, so there is
			// none to relay and none is invented.
			name:       "an inaccessible resource with no status behind it",
			reason:     subscriptions.ErrInaccessible,
			wantReason: endResourceGone,
		},
		{
			name:       "the absolute watch lifetime",
			reason:     subscriptions.ErrLifetimeExceeded,
			wantReason: endLifetimeReached,
		},
		{
			name:       "a demoted watch making room",
			reason:     subscriptions.ErrEvicted,
			wantReason: endWatcherEvicted,
		},
		{
			// Nothing else reaches OnStop today, and an ending this server
			// cannot name must leave the bare result rather than borrow the
			// nearest reason to hand.
			name:   "an ending this server has no reason for",
			reason: errors.New("something else entirely"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			end := watchEndForStop(tt.reason)

			if tt.wantReason == "" {
				if end != nil {
					t.Fatalf("watchEndForStop = %+v, want nil so the result stays bare", end)
				}
				return
			}
			if end == nil {
				t.Fatalf("watchEndForStop(%v) = nil, want reason %q", tt.reason, tt.wantReason)
			}
			if end.reason != tt.wantReason {
				t.Errorf("reason = %q, want %q", end.reason, tt.wantReason)
			}
			if end.status != tt.wantStatus {
				t.Errorf("status = %d, want %d", end.status, tt.wantStatus)
			}
			if end.detail == "" {
				t.Error("the ending carries no advice, which is the half a client that does not know the reason reads")
			}
		})
	}
}

// TestWatchEndForCause_TurnsAPoolEvictionIntoWhatTheClientIsTold covers the
// other half, and the one issue 561 is about.
//
// Every pool eviction used to reach the client as the same bare completion
// result, so a credential GitLab had just revoked was told exactly what a
// credential taken for capacity was told: reconnect and carry on. One of those
// clients then retried with a token that will be refused every time. The
// grouping here is by what the client should do next, which is the only thing
// a reason is for.
func TestWatchEndForCause_TurnsAPoolEvictionIntoWhatTheClientIsTold(t *testing.T) {
	tests := []struct {
		name  string
		cause serverpool.EvictionCause
		want  string
	}{
		{name: "size pressure", cause: serverpool.CauseSizePressure, want: endCredentialEvicted},
		{name: "the idle sweep", cause: serverpool.CauseIdle, want: endCredentialReset},
		{name: "a credential too long unchecked", cause: serverpool.CauseStaleCredential, want: endCredentialReset},
		{name: "a shape that has to be rebuilt", cause: serverpool.CauseRebuild, want: endCredentialReset},
		{name: "GitLab refusing a call", cause: serverpool.CauseRejectedCredential, want: endCredentialRevoked},
		{name: "revalidation finding it refused", cause: serverpool.CauseInvalidCredential, want: endCredentialRevoked},
		{name: "the pool closing", cause: serverpool.CausePoolClosed, want: endShutdown},
		{
			// A removal path added later without a decision here leaves the
			// client with the bare ending it always had, which is honest,
			// rather than with the nearest reason to hand, which is not.
			name:  "a cause this server does not know",
			cause: serverpool.EvictionCause("something added later"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			end := watchEndForCause(tt.cause)

			if tt.want == "" {
				if end != nil {
					t.Fatalf("watchEndForCause(%q) = %+v, want nil", tt.cause, end)
				}
				return
			}
			if end == nil {
				t.Fatalf("watchEndForCause(%q) = nil, want reason %q", tt.cause, tt.want)
			}
			if end.reason != tt.want {
				t.Errorf("reason = %q, want %q", end.reason, tt.want)
			}
			if end.status != 0 {
				t.Errorf("status = %d on a pool eviction, want none: no read failed", end.status)
			}
			if !slices.Contains(watchEndReasons, end.reason) {
				t.Errorf("reason %q is outside the published vocabulary %v", end.reason, watchEndReasons)
			}
		})
	}
}

// TestWatchEnd_Meta_OmitsAStatusThereIsNone covers the rendering.
//
// A client reading the status has to be able to tell "GitLab answered 404" from
// "no status came with this ending", and a zero sent as a number says neither.
func TestWatchEnd_Meta_OmitsAStatusThereIsNone(t *testing.T) {
	tests := []struct {
		name       string
		end        *watchEnd
		wantStatus any
	}{
		{name: "with a status", end: &watchEnd{reason: endResourceGone, detail: "gone", status: 404}, wantStatus: 404},
		{name: "without one", end: endOfShutdown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta := tt.end.meta()

			if meta["reason"] != tt.end.reason {
				t.Errorf("reason = %v, want %q", meta["reason"], tt.end.reason)
			}
			if meta["detail"] != tt.end.detail {
				t.Errorf("detail = %v, want %q", meta["detail"], tt.end.detail)
			}
			status, present := meta["status"]
			if tt.wantStatus == nil {
				if present {
					t.Errorf("status = %v, want it absent", status)
				}
				return
			}
			if status != tt.wantStatus {
				t.Errorf("status = %v, want %v", status, tt.wantStatus)
			}
		})
	}
}

// TestStampWatchEnd_AddsTheReasonBesideTheSDKsSubscriptionID is the invariant
// the whole mechanism rests on.
//
// The SDK builds this result itself and stamps
// io.modelcontextprotocol/subscriptionId into its `_meta`, which is how a client
// matches the completion to the listen it sent. A reason written over that map
// rather than added to it would take the correlation away, and the client would
// be told why a subscription it can no longer identify ended.
func TestStampWatchEnd_AddsTheReasonBesideTheSDKsSubscriptionID(t *testing.T) {
	for _, reason := range watchEndReasons {
		t.Run(reason, func(t *testing.T) {
			end := &watchEnd{reason: reason, detail: "advice for " + reason}
			stream := &listenStream{cancel: func() {}}
			stream.endWith(end)

			result := &mcp.SubscriptionsListenResult{Meta: mcp.Meta{mcp.MetaKeySubscriptionID: 7}}
			stampWatchEnd(result, stream)

			if result.Meta[mcp.MetaKeySubscriptionID] != 7 {
				t.Errorf("the SDK's subscription id is %v after the stamp, want 7",
					result.Meta[mcp.MetaKeySubscriptionID])
			}
			stamped, ok := result.Meta[watchEndMetaKey].(map[string]any)
			if !ok {
				t.Fatalf("%s = %v, want the ending as an object", watchEndMetaKey, result.Meta[watchEndMetaKey])
			}
			if stamped["reason"] != reason {
				t.Errorf("reason = %v, want %q", stamped["reason"], reason)
			}
		})
	}
}

// TestStampWatchEnd_LeavesAResultItHasNoReasonFor covers everything the stamp
// deliberately does not touch.
//
// A client that cancelled its own listen gets what it always got: the bare
// result, with no reason, because it caused the ending and the result may never
// reach it anyway. The typed nil is the shape the SDK's own early returns
// produce, and it is reachable here: a listen armed after shutdown records its
// ending before the handler has run, and that handler can still fail on a
// malformed request.
func TestStampWatchEnd_LeavesAResultItHasNoReasonFor(t *testing.T) {
	ended := &listenStream{cancel: func() {}}
	ended.endWith(endOfShutdown)

	tests := []struct {
		name   string
		result mcp.Result
		stream *listenStream
	}{
		{
			name:   "a stream the client itself ended",
			result: &mcp.SubscriptionsListenResult{Meta: mcp.Meta{mcp.MetaKeySubscriptionID: 1}},
			stream: &listenStream{cancel: func() {}},
		},
		{
			name:   "a handler that answered with an error",
			result: nil,
			stream: ended,
		},
		{
			name:   "the typed nil an early return produces",
			result: (*mcp.SubscriptionsListenResult)(nil),
			stream: ended,
		},
		{
			name:   "a result of some other method",
			result: &mcp.CallToolResult{},
			stream: ended,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stampWatchEnd(tt.result, tt.stream)

			listen, isListen := tt.result.(*mcp.SubscriptionsListenResult)
			if !isListen || listen == nil {
				return
			}
			if _, stamped := listen.Meta[watchEndMetaKey]; stamped {
				t.Errorf("a stream with no recorded ending was stamped anyway: %v", listen.Meta)
			}
		})
	}
}

// TestStampWatchEnd_AResultWithNoMetaGetsOne covers the allocation.
//
// Today the SDK always builds this map, because it puts its own subscription id
// in it. Relying on that would make this server's ending disappear the day the
// SDK stops, which is a silent loss rather than a failure.
func TestStampWatchEnd_AResultWithNoMetaGetsOne(t *testing.T) {
	stream := &listenStream{cancel: func() {}}
	stream.endWith(endOfShutdown)

	result := &mcp.SubscriptionsListenResult{}
	stampWatchEnd(result, stream)

	stamped, ok := result.Meta[watchEndMetaKey].(map[string]any)
	if !ok {
		t.Fatalf("_meta = %v, want the ending in a map built for it", result.Meta)
	}
	if stamped["reason"] != endShutdown {
		t.Errorf("reason = %v, want %q", stamped["reason"], endShutdown)
	}
}

// TestListenStream_EndWith_FirstWriterWins covers the race two causes really
// have.
//
// Shutdown closes the pool and cancels every stream from two different
// goroutines, so an eviction arriving at the same moment as a SIGTERM is
// ordinary rather than exceptional. Whichever cause got here first is the one
// the client is given, because by the time the second arrives the completion
// result may already be on the wire.
func TestListenStream_EndWith_FirstWriterWins(t *testing.T) {
	stream := &listenStream{cancel: func() {}}
	stream.endWith(endOfCredentialEviction)

	var wg sync.WaitGroup
	for _, end := range []*watchEnd{endOfShutdown, endOfCredentialRevocation, endOfCredentialReset} {
		wg.Go(func() { stream.endWith(end) })
	}
	wg.Wait()

	if got := stream.end.Load(); got != endOfCredentialEviction {
		t.Errorf("the ending is %+v, want the first one recorded; a later cause overwrote an ending "+
			"the client may already have been given", got)
	}
}

// TestListenStream_EndWith_NoReason_StillEndsTheStream covers the nil end.
//
// A stop reason this server does not name has to end the stream all the same:
// the alternative is a client left holding an open request against a watch that
// no longer exists, which is the very failure the registry was built to close.
func TestListenStream_EndWith_NoReason_StillEndsTheStream(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream := &listenStream{cancel: cancel}

	stream.endWith(nil)

	if ctx.Err() == nil {
		t.Error("a stream ended for an unnamed reason was left open")
	}
	if got := stream.end.Load(); got != nil {
		t.Errorf("the stream recorded %+v for an unnamed ending, want nothing", got)
	}
}

// TestListenStreams_AWatchThatRetires_TellsTheStreamWhy drives the reason
// through the registry rather than through the mapping function alone.
//
// A stream carrying several URIs is ended by whichever one empties the set, so
// that URI's reason is the one its client is given: the ending that actually
// closed the stream, not the first thing that went wrong on it.
func TestListenStreams_AWatchThatRetires_TellsTheStreamWhy(t *testing.T) {
	streams := newListenStreams()
	stream, release := streams.arm([]string{"uri-a", "uri-b"}, "owner", nil, func() {})
	t.Cleanup(release)

	streams.stoppedFor("owner", "uri-a", subscriptions.ErrEvicted)
	if got := stream.end.Load(); got != nil {
		t.Fatalf("the stream recorded %+v while it was still watching uri-b", got)
	}

	streams.stoppedFor("owner", "uri-b", gitLabStatusError(http.StatusNotFound))

	end := stream.end.Load()
	if end == nil {
		t.Fatal("the stream was ended with no reason recorded")
	}
	if end.reason != endResourceGone {
		t.Errorf("reason = %q, want %q: the ending that closed the stream is the one to report", end.reason, endResourceGone)
	}
	if end.status != http.StatusNotFound {
		t.Errorf("status = %d, want 404", end.status)
	}
}

// TestListenStreams_CloseAll_TellsEveryStreamItIsShutdown covers the ending a
// client gets when the process is stopping, which is the one reason reachable
// without a pool at all.
func TestListenStreams_CloseAll_TellsEveryStreamItIsShutdown(t *testing.T) {
	streams := newListenStreams()
	open, release := streams.arm([]string{"uri"}, "owner", nil, func() {})
	t.Cleanup(release)

	streams.closeAll()

	// A listen that arrives during the drain is owed the same answer as one
	// that was already open when it began.
	late, releaseLate := streams.arm([]string{"uri"}, "owner", nil, func() {})
	t.Cleanup(releaseLate)

	for _, tc := range []struct {
		name   string
		stream *listenStream
	}{
		{name: "a stream that was already open", stream: open},
		{name: "a stream armed during the drain", stream: late},
	} {
		t.Run(tc.name, func(t *testing.T) {
			end := tc.stream.end.Load()
			if end == nil {
				t.Fatal("the stream was ended with no reason recorded")
			}
			if end.reason != endShutdown {
				t.Errorf("reason = %q, want %q", end.reason, endShutdown)
			}
		})
	}
}
