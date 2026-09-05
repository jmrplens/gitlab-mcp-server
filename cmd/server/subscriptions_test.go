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

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/resources"
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
	manager := newSubscriptionRuntime(client, subscriptionCfg(config.CapabilitySurfaceFull), fastOptions()).manager
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
	manager := newSubscriptionRuntime(client, subscriptionCfg(config.CapabilitySurfaceFull), fastOptions()).manager
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
	runtime := newSubscriptionRuntime(subscriptionGitLabClient(t, gitlab.URL),
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
	runtime := newSubscriptionRuntime(subscriptionGitLabClient(t, gitlab.URL),
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
	runtime := newSubscriptionRuntime(subscriptionGitLabClient(t, gitlab.URL),
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

	reader := &resourceReader{}
	reader.setIndex(subscriptionHandlerIndex(t, gitlab.URL))
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
	reader := &resourceReader{}
	reader.setIndex(subscriptionHandlerIndex(t, gitlab.URL))

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

	runtime := newSubscriptionRuntime(client, cfg, fastOptions())
	t.Cleanup(runtime.manager.Close)
	subscribe, unsubscribe := runtime.handlers()
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

	runtime := newSubscriptionRuntime(subscriptionGitLabClient(t, gitlab.URL), cfg, fastOptions())
	t.Cleanup(runtime.manager.Close)
	subscribe, _ := runtime.handlers()

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

// TestNewSubscriptionRuntime_MinimalSurface_IsNil verifies no manager, and
// therefore no capability, on a surface without GitLab resources.
func TestNewSubscriptionRuntime_MinimalSurface_IsNil(t *testing.T) {
	_, gitlab := newPipelineBackend(t, "running")
	client := subscriptionGitLabClient(t, gitlab.URL)

	runtime := newSubscriptionRuntime(client, subscriptionCfg(config.CapabilitySurfaceMinimal), subscriptions.Options{})
	if runtime != nil {
		t.Errorf("newSubscriptionRuntime(minimal) = %v, want nil", runtime)
	}

	sub, unsub := runtime.handlers()
	if sub != nil || unsub != nil {
		t.Error("handlers() on no runtime returned non-nil handlers; the SDK would advertise a capability with no backing")
	}

	if runtime.streamRegistry() == nil {
		t.Error("streamRegistry() on no runtime returned nil; nothing could then end a stream the SDK is holding")
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
// So on --capability-surface=minimal, where newSubscriptionRuntime returns nil,
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
	var runtime *subscriptionRuntime
	runtime.attach(ctx, server)

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

	_, release := streams.arm([]string{"uri-a", "uri-b"}, cancel)
	defer release()

	streams.stopped("uri-a", subscriptions.ErrInaccessible)
	if ctx.Err() != nil {
		t.Fatal("the stream ended while it was still watching uri-b")
	}

	streams.stopped("uri-b", subscriptions.ErrInaccessible)
	if ctx.Err() == nil {
		t.Error("every watched URI stopped and the stream was left open")
	}
}

// TestListenStreams_UnrelatedURIStops_LeavesTheStream verifies one stream's
// ending does not disturb another's.
func TestListenStreams_UnrelatedURIStops_LeavesTheStream(t *testing.T) {
	streams := newListenStreams()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, release := streams.arm([]string{"mine"}, cancel)
	defer release()

	streams.stopped("somebody-elses", subscriptions.ErrEvicted)
	if ctx.Err() != nil {
		t.Error("a stream ended over a URI it never subscribed to")
	}
}

// TestListenStreams_ReleasedStream_IsNotCancelled verifies a stream that
// already returned is forgotten.
func TestListenStreams_ReleasedStream_IsNotCancelled(t *testing.T) {
	streams := newListenStreams()
	cancelled := false
	_, release := streams.arm([]string{"uri"}, func() { cancelled = true })
	release()

	streams.stopped("uri", subscriptions.ErrInaccessible)
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
		_, release := streams.arm(tc.uris, func() { ended[i] = true })
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
	_, release := streams.arm(nil, cancel)
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
	runtime := newSubscriptionRuntime(subscriptionGitLabClient(t, gitlab.URL),
		subscriptionCfg(config.CapabilitySurfaceFull), fastOptions())
	t.Cleanup(runtime.manager.Close)

	const uri = "gitlab://project/42/pipeline/99"
	if err := runtime.manager.Subscribe(context.Background(), testSession, uri); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, release := runtime.streams.arm([]string{uri}, cancel)
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
	runtime := newSubscriptionRuntime(subscriptionGitLabClient(t, gitlab.URL),
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
	runtime := newSubscriptionRuntime(subscriptionGitLabClient(t, gitlab.URL),
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
	runtime := newSubscriptionRuntime(subscriptionGitLabClient(t, gitlab.URL),
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
// A short lease is injected so the renewal has to actually happen rather than
// the test passing because nothing had time to expire.
func TestSessionBridge_AnOpenListenKeepsItsWatchAtFullSpeed(t *testing.T) {
	_, gitlab := newPipelineBackend(t, "running")
	opts := fastOptions()
	opts.Lease = 150 * time.Millisecond
	runtime := newSubscriptionRuntime(subscriptionGitLabClient(t, gitlab.URL),
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

	// Several leases' worth of silence. With no renewal the watch is demoted
	// well before this elapses.
	time.Sleep(700 * time.Millisecond)

	if runtime.manager.Len() != 1 {
		t.Fatalf("the watch is gone entirely; this test can no longer see what it checks")
	}
	if demoted := runtime.manager.DemotedCount(); demoted != 0 {
		t.Errorf("%d watch(es) demoted while the listen stream was open and being read", demoted)
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
	runtime := newSubscriptionRuntime(subscriptionGitLabClient(t, gitlab.URL),
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
	runtime := newSubscriptionRuntime(subscriptionGitLabClient(t, gitlab.URL),
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
		name        string
		perServer   int
		perProcess  int
		streams     int
		wantServed  int
		wantRefused int
	}{
		{"under_the_per_server_cap", 4, 100, 3, 3, 0},
		{"at_the_per_server_cap", 4, 100, 4, 4, 0},
		{"over_the_per_server_cap", 4, 100, 7, 4, 3},
		{"the_process_cap_binds_first", 100, 2, 5, 2, 3},
		{"disabled_by_a_non_positive_cap", 0, 0, 6, 6, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			limits := listenLimits{
				perServer:   tc.perServer,
				perProcess:  tc.perProcess,
				processOpen: &listenCounter{},
				serverOpen:  &listenCounter{},
			}

			held := make(chan struct{})
			var inFlight atomic.Int64
			handler := limits.middleware()(func(ctx context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
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
		perServer:   1,
		perProcess:  1,
		processOpen: &listenCounter{},
		serverOpen:  &listenCounter{},
	}
	held := make(chan struct{})
	defer close(held)
	handler := limits.middleware()(func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		<-held
		return &mcp.CallToolResult{}, nil
	})

	opened := make(chan struct{})
	go func() {
		close(opened)
		_, _ = handler(t.Context(), methodSubscriptionsListen, nil)
	}()
	<-opened
	waitFor(t, func() bool { return limits.serverOpen.count() == 1 })

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
		perServer:   1,
		perProcess:  1,
		processOpen: &listenCounter{},
		serverOpen:  &listenCounter{},
	}
	handler := limits.middleware()(func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		return &mcp.CallToolResult{}, nil
	})
	for _, method := range []string{"tools/call", "tools/list", "resources/read", "ping"} {
		t.Run(method, func(t *testing.T) {
			if _, err := handler(t.Context(), method, nil); err != nil {
				t.Fatalf("%s: %v", method, err)
			}
			if open := limits.serverOpen.count(); open != 0 {
				t.Errorf("%s left %d listen slots taken, want 0", method, open)
			}
		})
	}
}

// TestListenLimitsFromEnv verifies that the ceilings are configurable and that
// anything that is not a positive number leaves the defaults in place.
func TestListenLimitsFromEnv(t *testing.T) {
	for _, tc := range []struct {
		name           string
		perServer      string
		wantPerServer  int
		wantPerProcess int
	}{
		{"unset", "", maxListenStreamsPerServer, maxListenStreamsPerProcess},
		{"explicit", "8", 8, maxListenStreamsPerProcess},
		{"zero_disables", "0", 0, maxListenStreamsPerProcess},
		{"garbage_keeps_the_default", "many", maxListenStreamsPerServer, maxListenStreamsPerProcess},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.perServer != "" {
				t.Setenv(maxListenStreamsEnv, tc.perServer)
			}
			got := listenLimitsFromEnv()
			if got.perServer != tc.wantPerServer {
				t.Errorf("perServer = %d, want %d", got.perServer, tc.wantPerServer)
			}
			if got.perProcess != tc.wantPerProcess {
				t.Errorf("perProcess = %d, want %d", got.perProcess, tc.wantPerProcess)
			}
			if got.processOpen == nil || got.serverOpen == nil {
				t.Error("the counters must be non-nil, or the middleware cannot count")
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
	// resources/list and would otherwise still be served.
	reader := &resourceReader{}
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
// the reader before registration has published an index. The runtime seeds
// one, so nothing built through it gets here; the point is that a reader that
// somehow does answers with the retryable "not ready" rather than
// dereferencing nil inside a watcher.
func TestResourceReader_WithoutAnIndex_AnswersNotReadyInsteadOfPanicking(t *testing.T) {
	t.Parallel()
	reader := &resourceReader{}
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
	runtime := newSubscriptionRuntime(subscriptionGitLabClient(t, gitlab.URL),
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
	runtime := newSubscriptionRuntime(subscriptionGitLabClient(t, gitlab.URL),
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
