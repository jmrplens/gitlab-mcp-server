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
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/resources"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/subscriptions"
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
	server, err := createServer(subscriptionGitLabClient(t, gitlabURL), cfg, nil, opts...)
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
	deadline := time.Now().Add(5 * time.Second)
	for b.hits.Load() < n {
		if time.Now().After(deadline) {
			t.Fatalf("GitLab was polled %d times in 5s, want at least %d", b.hits.Load(), n)
		}
		time.Sleep(2 * time.Millisecond)
	}
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

	// The refusal cost one read; what must not happen is a second one.
	settled := backend.calls()
	time.Sleep(200 * time.Millisecond)
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
	// land, so let things settle before sampling the count.
	time.Sleep(50 * time.Millisecond)
	before := backend.hits.Load()
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
	// A read already in flight may still land after the session ends.
	time.Sleep(50 * time.Millisecond)
	before := backend.calls()
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

// TestSubscribe_IdleSession_WatcherSlowsDown verifies an unrenewed lease
// takes the watcher off full speed.
func TestSubscribe_IdleSession_WatcherSlowsDown(t *testing.T) {
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

	time.Sleep(lease + slow/4)
	demoted := backend.calls()
	time.Sleep(300 * time.Millisecond) // fifteen full-speed periods, a fraction of a slow one

	if grew := backend.calls() - demoted; grew > 2 {
		t.Errorf("GitLab was called %d more times after the lease ran out, want the watcher slowed down", grew)
	}
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
	stillActive := backend.calls()
	busy(8)

	if grew := backend.calls() - stillActive; grew < 3 {
		t.Errorf("GitLab was called %d more times %v after the lease would have run out; "+
			"activity on the session did not renew the watch", grew, 4*lease)
	}
}

// TestSubscribe_KeepaliveOnly_WatcherStillSlowsDown is the negative half of
// the renewal contract.
//
// A ping proves a socket is open, not that anyone is waiting on the other
// end of it. If keep-alive traffic renewed the lease, the lease would be
// unreachable for any connected client — which is the same as having no
// lease at all, and this is the test that would catch that.
func TestSubscribe_KeepaliveOnly_WatcherStillSlowsDown(t *testing.T) {
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

	for range 6 {
		if err := session.Ping(ctx, nil); err != nil {
			t.Fatalf("Ping: %v", err)
		}
		time.Sleep(lease / 2)
	}
	demoted := backend.calls()
	time.Sleep(300 * time.Millisecond)

	if grew := backend.calls() - demoted; grew > 2 {
		t.Errorf("GitLab was called %d more times after a lease of nothing but pings; "+
			"keep-alive traffic must not renew a watch", grew)
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

	reader := resourceReader{index: subscriptionHandlerIndex(t, gitlab.URL)}
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
	reader := resourceReader{index: subscriptionHandlerIndex(t, gitlab.URL)}

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

	release := streams.arm([]string{"uri-a", "uri-b"}, cancel)
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
	release := streams.arm([]string{"mine"}, cancel)
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
	release := streams.arm([]string{"uri"}, func() { cancelled = true })
	release()

	streams.stopped("uri", subscriptions.ErrInaccessible)
	if cancelled {
		t.Error("a released stream was cancelled; its request had already returned")
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
	release := runtime.streams.arm([]string{uri}, cancel)
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
