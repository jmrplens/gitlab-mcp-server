//go:build httpe2e

// shared_server_test.go drives the one thing a shared MCP server has to get
// right that a server per credential got right by construction: keeping each
// credential's data, notifications and sessions to that credential.
//
// Since ADR-0020 the pool builds one mcp.Server per configuration shape and
// hands it to every credential that hashes to it. The isolation that used to be
// structural is now made of three moving parts: the per-request credential
// binding, the owner tag on every resource-updated notification, and the
// sending middleware that filters delivery by it. None of the three can be
// tested where it lives. A unit test over the middleware sees a correct
// middleware; what it cannot see is whether the notification that reaches it
// was ever tagged, whether the session was ever recorded, and whether the two
// agree. Only the wire can answer that.
package httpe2e

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// listenBody is a subscriptions/listen asking for one resource, as a
// 2026-07-28 client sends it.
func listenBody(id int, uri string) string {
	const meta = `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28",` +
		`"io.modelcontextprotocol/clientCapabilities":{},` +
		`"io.modelcontextprotocol/clientInfo":{"name":"probe","version":"1"}}`
	return fmt.Sprintf(
		`{"jsonrpc":"2.0","id":%d,"method":"subscriptions/listen","params":{"notifications":{"resourceSubscriptions":[%q]},%s}}`,
		id, uri, meta,
	)
}

// listenStream is an open subscriptions/listen request and the frames it has
// delivered so far.
type listenStream struct {
	frames chan string
	// failure carries a transport-level problem back to the test goroutine.
	// The reader runs on a goroutine of its own, where a testing.T abort is
	// forbidden, so the abort is made on the goroutine that started it.
	failure chan error
	cancel  context.CancelFunc
}

// openListen sends a subscriptions/listen for one URI as one credential and
// streams its frames until the test ends.
//
// The stream is deliberately not read to EOF: a listen stays open for as long
// as the subscription lives, so the reader is a goroutine and the test asserts
// on what has arrived by a deadline of its own.
func openListen(t *testing.T, srv *server, token, uri string, id int) *listenStream {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.baseURL+"/mcp", strings.NewReader(listenBody(id, uri)))
	if err != nil {
		cancel()
		t.Fatalf("building the listen request for %s: %v", token, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", acceptHeader)
	req.Header.Set("MCP-Protocol-Version", protocolVersion)
	req.Header.Set("Mcp-Method", "subscriptions/listen")
	req.Header.Set("PRIVATE-TOKEN", token)

	// The body outlives this function on purpose: a listen stays open, so the
	// reader goroutine below owns it and closes it when the stream ends. Every
	// path that does not reach that goroutine closes it here.
	resp, err := srv.httpClient().Do(req) //nolint:bodyclose // closed by the reader goroutine, and on every early return
	if err != nil {
		cancel()
		t.Fatalf("sending the listen for %s: %v", token, err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		cancel()
		t.Fatalf("listen for %s answered %d, want 200", token, resp.StatusCode)
	}

	return readStream(ctx, resp, cancel)
}

// readStream turns an open SSE response into a stream of frames a test can wait
// on, taking ownership of the body.
//
// It takes no *testing.T because it runs the reader on a goroutine of its own,
// where an abort is forbidden; a transport failure travels back on the failure
// channel and is raised by whichever assertion reads it next.
func readStream(ctx context.Context, resp *http.Response, cancel context.CancelFunc) *listenStream {
	stream := &listenStream{
		frames:  make(chan string, 64),
		failure: make(chan error, 1),
		cancel:  cancel,
	}
	go func() {
		defer resp.Body.Close()
		defer close(stream.frames)
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			after, ok := strings.CutPrefix(scanner.Text(), "data: ")
			if !ok {
				continue
			}
			select {
			case stream.frames <- after:
			case <-ctx.Done():
				return
			}
		}
		if scanErr := scanner.Err(); scanErr != nil && ctx.Err() == nil {
			select {
			case stream.failure <- scanErr:
			default:
			}
		}
	}()
	return stream
}

// awaitFrame waits for the first frame containing want, returning every frame
// it saw on the way.
func (s *listenStream) awaitFrame(t *testing.T, want string, within time.Duration) (string, []string, bool) {
	t.Helper()

	var seen []string
	deadline := time.After(within)
	for {
		select {
		case frame, open := <-s.frames:
			if !open {
				return "", seen, false
			}
			seen = append(seen, frame)
			if strings.Contains(frame, want) {
				return frame, seen, true
			}
		case err := <-s.failure:
			t.Fatalf("reading the listen stream: %v", err)
		case <-deadline:
			return "", seen, false
		}
	}
}

// drain returns every frame delivered so far without waiting for more.
func (s *listenStream) drain() []string {
	var seen []string
	for {
		select {
		case frame, open := <-s.frames:
			if !open {
				return seen
			}
			seen = append(seen, frame)
		default:
			return seen
		}
	}
}

// openStandaloneStream opens the standalone SSE stream of a stateful session.
//
// It is the GET half of the legacy transport: the stream a session-era client
// leaves open to receive server-initiated messages, which is where a
// resources/subscribe would deliver its notifications. Nothing in the shared
// server file drove --stateless=false before, and this is the only path on
// which a subscription holds no request of its own.
func openStandaloneStream(t *testing.T, srv *server, token, sessionID, protocol string) *listenStream {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.baseURL+"/mcp", http.NoBody)
	if err != nil {
		cancel()
		t.Fatalf("building the standalone stream request for %s: %v", token, err)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("MCP-Protocol-Version", protocol)
	req.Header.Set("Mcp-Session-Id", sessionID)
	req.Header.Set("PRIVATE-TOKEN", token)

	// As in openListen, the body outlives this function: the reader goroutine
	// owns it, and every path that does not reach that goroutine closes it.
	resp, err := srv.httpClient().Do(req) //nolint:bodyclose // closed by the reader goroutine, and on every early return
	if err != nil {
		cancel()
		t.Fatalf("opening the standalone stream for %s: %v", token, err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		cancel()
		t.Fatalf("the standalone stream for %s answered %d, want 200", token, resp.StatusCode)
	}
	return readStream(ctx, resp, cancel)
}

// TestSharedServer_AnEvictedCredentialsLegacySubscriptionIsEnded pins the other
// half of what eviction owes a subscriber, on the transport that has it.
//
// Ending a credential's listen streams reaches everything a 2026-07-28
// subscriber holds, because there the subscription IS an open request. A
// session-era resources/subscribe holds nothing: it is answered and over, and
// the notifications would arrive later on the session's standalone SSE stream.
// So eviction stopped its watchers, which Manager.Close does without firing
// OnStop, forgot which credential the session belonged to, and left that stream
// open and mute for the rest of its life. The client was not told, could not
// tell, and ADR-0020 claimed the opposite.
//
// The only ending the protocol offers a subscriber with no open request is
// terminating its session, which is what this asserts: the standalone stream
// ends, and the client's next request re-initializes. It is also what the gate
// would enforce on that next request anyway; the point is that a client whose
// only activity is a subscription never makes one.
//
// Reachable on --stateless=false alone. The default transport gives each POST a
// session of its own and refuses the legacy subscribe outright, so there is
// nothing there to leave hanging.
func TestSharedServer_AnEvictedCredentialsLegacySubscriptionIsEnded(t *testing.T) {
	const (
		subscribedToken = "glpat-legacy-subscriber"
		arrivingToken   = "glpat-arrives-and-takes-the-slot"
		uri             = "gitlab://project/123"
		statefulVersion = "2025-06-18"
	)

	gitlab := startTwoTenantGitLab(t, subscribedToken, arrivingToken)
	srv := startServer(t, nil,
		"--gitlab-url="+gitlab.URL,
		"--capability-surface=full",
		"--stateless=false",
		"--max-http-clients=1",
	)

	headers := openStatefulSessionAs(t, srv, subscribedToken)
	stream := openStandaloneStream(t, srv, subscribedToken, headers["Mcp-Session-Id"], statefulVersion)

	subscribed := srv.do(t, request{
		body:    fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"resources/subscribe","params":{"uri":%q}}`, uri),
		headers: headers,
	})
	if subscribed.status != http.StatusOK || strings.Contains(subscribed.body, `"error"`) {
		t.Fatalf("the session-era subscribe was refused (%d), so there is no hanging client to test: %s",
			subscribed.status, subscribed.body)
	}

	// One more credential than the pool may hold. Every entry is busy, so the
	// bound is what evicts: see TestSharedServer_AnEvictedCredentialsListenIsEnded.
	//
	// Sent as a session-era client, because --stateless=false refuses
	// 2026-07-28 outright and the harness sends that revision by default.
	arriving := srv.do(t, request{
		body: legacyToolsListBody,
		headers: map[string]string{
			"PRIVATE-TOKEN":        arrivingToken,
			"MCP-Protocol-Version": "",
			"Mcp-Method":           "",
		},
	})
	if arriving.status != http.StatusOK {
		t.Fatalf("the arriving credential was answered %d: %s", arriving.status, arriving.body)
	}

	if seen, ended := stream.awaitEnd(t, 30*time.Second); !ended {
		t.Errorf("the evicted credential's standalone stream was left open: its watchers are gone, so it is "+
			"served nothing and told nothing.\nframes: %v\nserver output:\n%s", seen, srv.logs())
	}
}

// openStatefulSessionAs completes the session-era handshake as one credential
// and returns the headers its later requests carry.
//
// The protocol version is the session-era one on purpose: 2026-07-28 removed
// resources/subscribe, and the SDK answers it itself before any middleware runs.
func openStatefulSessionAs(t *testing.T, srv *server, token string) map[string]string {
	t.Helper()

	base := map[string]string{
		"PRIVATE-TOKEN":        token,
		"MCP-Protocol-Version": "",
		"Mcp-Method":           "",
	}

	const initialize = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18",` +
		`"capabilities":{},"clientInfo":{"name":"httpe2e","version":"0"}}}`
	opened := srv.do(t, request{body: initialize, headers: base})
	if opened.status != http.StatusOK {
		t.Fatalf("opening a stateful session for %s: initialize = %d: %s", token, opened.status, opened.body)
	}
	sessionID := opened.header.Get("Mcp-Session-Id")
	if sessionID == "" {
		t.Fatalf("the stateful server answered initialize without a session id: %v", opened.header)
	}

	headers := map[string]string{
		"PRIVATE-TOKEN":        token,
		"MCP-Protocol-Version": "",
		"Mcp-Method":           "",
		"Mcp-Session-Id":       sessionID,
	}
	ready := srv.do(t, request{body: `{"jsonrpc":"2.0","method":"notifications/initialized"}`, headers: headers})
	if ready.status != http.StatusOK && ready.status != http.StatusAccepted {
		t.Fatalf("completing the handshake for %s: notifications/initialized = %d: %s", token, ready.status, ready.body)
	}

	// The catalog is built here rather than under the subscribe, so the
	// readiness gate is not what the rest of the test is waiting on.
	warm := srv.do(t, request{body: legacyToolsListBody, headers: headers})
	if warm.status != http.StatusOK {
		t.Fatalf("warming the pool entry for %s: tools/list = %d: %s", token, warm.status, warm.body)
	}
	return headers
}

// twoTenantGitLab is a GitLab whose one project changes for the first token
// and never changes for the second.
//
// That asymmetry is what makes the test decisive. Only the first credential's
// watcher can ever have something to report, so any resource-updated
// notification arriving on the second credential's stream came from the first
// credential's watcher and nowhere else.
type twoTenantGitLab struct {
	*httptest.Server
	changing string
	frozen   string
	// revoked, once set, makes the project unreadable for the frozen token, as
	// a membership removal would.
	revoked atomic.Bool
	reads   atomic.Int64
}

func startTwoTenantGitLab(t *testing.T, changingToken, frozenToken string) *twoTenantGitLab {
	t.Helper()

	fake := &twoTenantGitLab{changing: changingToken, frozen: frozenToken}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"17.0.0","revision":"abcdef"}`))
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := 7
		if r.Header.Get("PRIVATE-TOKEN") == fake.frozen {
			id = 8
		}
		_, _ = fmt.Fprintf(w, `{"id":%d,"username":"user%d"}`, id, id)
	})
	mux.HandleFunc("/api/v4/projects/123", func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("PRIVATE-TOKEN")
		if token == fake.frozen && fake.revoked.Load() {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		description := "unchanging"
		if token == fake.changing {
			// A different body on every read, so this credential's watcher
			// always has a change to report and the other's never does.
			description = "change-" + strconv.FormatInt(fake.reads.Add(1), 10)
		}
		_, _ = fmt.Fprintf(w,
			`{"id":123,"name":"proj","description":%q,"path_with_namespace":"g/p","web_url":"http://example.invalid/g/p"}`,
			description)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	fake.Server = httptest.NewServer(mux)
	t.Cleanup(fake.Close)
	return fake
}

// countShapeBuilds reports how many configuration shapes the server has built.
func countShapeBuilds(logs string) int {
	return strings.Count(logs, "built the MCP server for a configuration shape")
}

// TestSharedServer_TwoCredentialsOnOneURIEachSeeOnlyTheirOwnWatcher is the
// invariant of ADR-0020 on the wire.
//
// Two credentials of one configuration shape are served by one mcp.Server, so
// they appear together in the SDK's subscription table under the same resource
// URI, and Server.ResourceUpdated notifies every session in that table. Only
// the owner tag and the sending middleware keep them apart. The fake GitLab
// changes the project for the first credential alone, so a notification on the
// second credential's stream can only have come from the first credential's
// watcher.
//
// The private tag is checked too: it exists to be read by the middleware and
// stripped before the frame is written, so its appearance in any frame would be
// this server leaking an internal identifier to a client.
func TestSharedServer_TwoCredentialsOnOneURIEachSeeOnlyTheirOwnWatcher(t *testing.T) {
	const (
		busyToken = "glpat-busy-tenant"
		idleToken = "glpat-idle-tenant"
		uri       = "gitlab://project/123"
	)

	gitlab := startTwoTenantGitLab(t, busyToken, idleToken)
	srv := startServer(t, nil, "--gitlab-url="+gitlab.URL, "--capability-surface=full")

	busy := openListen(t, srv, busyToken, uri, 1)
	idle := openListen(t, srv, idleToken, uri, 2)

	// Every frame either tenant is handed is kept, because the last check here
	// is about what was written on the wire and a frame read off the channel by
	// one assertion is gone for the next.
	_, busyFrames, busyAcked := busy.awaitFrame(t, "notifications/subscriptions/acknowledged", 20*time.Second)
	if !busyAcked {
		t.Fatalf("the busy tenant's listen was never acknowledged; frames: %v", busyFrames)
	}
	_, idleFrames, idleAcked := idle.awaitFrame(t, "notifications/subscriptions/acknowledged", 20*time.Second)
	if !idleAcked {
		t.Fatalf("the idle tenant's listen was never acknowledged; frames: %v", idleFrames)
	}

	// One build for both credentials is the whole point of the change, and it
	// is also what makes the rest of this test meaningful: two servers would
	// have no delivery to filter.
	if builds := countShapeBuilds(srv.logs()); builds != 1 {
		t.Errorf("the server built %d configuration shapes for two credentials of one configuration, want 1", builds)
	}

	// The polling cadence is the shipped one (15s between reads), so this
	// waits out a poll rather than pretending it can be hurried.
	update, seen, ok := busy.awaitFrame(t, "notifications/resources/updated", 90*time.Second)
	busyFrames = append(busyFrames, seen...)
	if !ok {
		t.Fatalf("the busy tenant never received an update for a resource that changes on every read; frames: %v\nserver output:\n%s",
			busyFrames, srv.logs())
	}
	if !strings.Contains(update, uri) {
		t.Errorf("the update names a different resource:\n%s", update)
	}
	if !strings.Contains(update, "io.github.jmrplens/watch") {
		t.Errorf("the update carries no watch state, which every notification this server sends does:\n%s", update)
	}

	// Drained once and added to what the acknowledgement wait already took. A
	// second drain returns an empty slice, since the frames are read off a
	// channel, so a check that drains again iterates nothing and passes
	// whatever the idle tenant had actually received.
	idleFrames = append(idleFrames, idle.drain()...)

	// Whatever the idle tenant received, none of it may be a resource update:
	// its own resource never changed, so an update on this stream is somebody
	// else's.
	for _, frame := range idleFrames {
		if strings.Contains(frame, "notifications/resources/updated") {
			t.Errorf("the idle tenant received a resource update it could not have generated:\n%s", frame)
		}
	}

	// The owner tag is internal. It is read by the sending middleware and
	// removed from the clone that is written, so no frame on any stream may
	// carry it.
	for name, frames := range map[string][]string{"busy": busyFrames, "idle": idleFrames} {
		t.Run(name, func(t *testing.T) {
			if len(frames) == 0 {
				t.Fatalf("the %s tenant delivered no frames at all, so this proves nothing about the private tag", name)
			}
			for _, frame := range frames {
				if strings.Contains(frame, "io.github.jmrplens/watch-owner") {
					t.Errorf("the %s tenant's frame carries the private owner tag:\n%s", name, frame)
				}
			}
		})
	}
}

// TestSharedServer_ARevokedCredentialReceivesNothing pins the second half of
// the invariant.
//
// The dangerous shape is not a credential that loses access and is told so. It
// is a credential that loses access, keeps an open listen, and goes on being
// told the resource changed because somebody else on the same server can still
// read it. That is exactly what a shared subscription table produces without a
// filter, and it is worse than a stale answer: it is a covert channel reporting
// activity on a resource the caller may no longer see at all.
func TestSharedServer_ARevokedCredentialReceivesNothing(t *testing.T) {
	const (
		busyToken    = "glpat-still-a-member"
		revokedToken = "glpat-removed-from-project"
		uri          = "gitlab://project/123"
	)

	gitlab := startTwoTenantGitLab(t, busyToken, revokedToken)
	srv := startServer(t, nil, "--gitlab-url="+gitlab.URL, "--capability-surface=full")

	busy := openListen(t, srv, busyToken, uri, 1)
	revoked := openListen(t, srv, revokedToken, uri, 2)

	if _, seen, ok := revoked.awaitFrame(t, "notifications/subscriptions/acknowledged", 20*time.Second); !ok {
		t.Fatalf("the second tenant's listen was never acknowledged; frames: %v", seen)
	}
	if _, seen, ok := busy.awaitFrame(t, "notifications/subscriptions/acknowledged", 20*time.Second); !ok {
		t.Fatalf("the first tenant's listen was never acknowledged; frames: %v", seen)
	}

	// The same check its sibling makes, and for the same reason: two servers
	// would keep these credentials apart by construction, so without this the
	// test would pass vacuously if the two ever hashed to different shapes.
	if builds := countShapeBuilds(srv.logs()); builds != 1 {
		t.Errorf("the server built %d configuration shapes for two credentials of one configuration, want 1", builds)
	}

	// Access goes away after the subscription is established, which is the
	// case the SDK's table cannot express: the session stays in it for as long
	// as the listen is open.
	gitlab.revoked.Store(true)

	if _, seen, ok := busy.awaitFrame(t, "notifications/resources/updated", 90*time.Second); !ok {
		t.Fatalf("the tenant that kept access received no update, so nothing was delivered to filter; frames: %v", seen)
	}

	for _, frame := range revoked.drain() {
		if strings.Contains(frame, "notifications/resources/updated") {
			t.Errorf("the revoked tenant was told the resource changed:\n%s", frame)
		}
	}
}

// awaitEnd waits for the server to end this stream, returning every frame that
// arrived before it did.
//
// The stream's reader goroutine closes the frame channel when the response body
// ends, which is what the SDK's listen handler returning produces on the wire.
func (s *listenStream) awaitEnd(t *testing.T, within time.Duration) ([]string, bool) {
	t.Helper()

	var seen []string
	deadline := time.After(within)
	for {
		select {
		case frame, open := <-s.frames:
			if !open {
				return seen, true
			}
			seen = append(seen, frame)
		case err := <-s.failure:
			t.Fatalf("reading the listen stream: %v", err)
		case <-deadline:
			return seen, false
		}
	}
}

// TestSharedServer_AnEvictedCredentialsListenIsEnded pins what a client is told
// when the pool drops the entry its subscription belongs to.
//
// Eviction stops that credential's watchers, and stopping them is silent by
// design: Manager.Close is the one path that fires no OnStop, because the three
// endings it does announce are ones the subscriber did not ask for and a closed
// manager used to mean a server going away. On a shared server it means one
// tenant leaving, and without the stream teardown the client's open
// subscriptions/listen was left open and silent for the rest of its life. It
// still held its slot in the process-wide stream ceiling too, while the entry
// rebuilt for the same credential got a fresh counter of its own.
//
// The pressure is --max-http-clients=1, and that number is the whole reason a
// subscriber is evicted at all. Size pressure prefers an entry that is not busy
// and takes a busy one only when every entry is busy, which a pool of one
// always is: what this drives is that fallback, chosen because the pool is
// bounded before it is polite. Raise the maximum and the arriving credential
// evicts nothing, since there is room; fill it with quiet callers and they go
// first. Before that preference existed the tail went unconditionally, so a
// caller could evict every quiet subscriber in a pool by presenting
// --max-http-clients credentials of its own.
//
// The idle sweep is the other eviction path and never fires here: it takes
// --pool-idle-timeout, and an entry with a live subscription is not idle by
// that measure either.
func TestSharedServer_AnEvictedCredentialsListenIsEnded(t *testing.T) {
	const (
		subscribedToken = "glpat-subscribed-then-evicted"
		arrivingToken   = "glpat-arrives-and-takes-the-slot"
		uri             = "gitlab://project/123"
	)

	gitlab := startTwoTenantGitLab(t, subscribedToken, arrivingToken)
	srv := startServer(t, nil,
		"--gitlab-url="+gitlab.URL,
		"--capability-surface=full",
		"--max-http-clients=1",
	)

	subscribed := openListen(t, srv, subscribedToken, uri, 1)
	if _, seen, ok := subscribed.awaitFrame(t, "notifications/subscriptions/acknowledged", 20*time.Second); !ok {
		t.Fatalf("the listen was never acknowledged; frames: %v", seen)
	}

	// One more credential than the pool may hold, which evicts the one that is
	// not making requests: exactly the client whose only activity is the
	// subscription above.
	arriving := srv.do(t, request{
		body:    toolsListBody,
		headers: map[string]string{"PRIVATE-TOKEN": arrivingToken},
	})
	if arriving.status != http.StatusOK {
		t.Fatalf("the arriving credential was answered %d: %s", arriving.status, arriving.body)
	}

	seen, ended := subscribed.awaitEnd(t, 30*time.Second)
	if !ended {
		t.Fatalf("the evicted credential's listen was left open: its watchers are gone, so it is served nothing "+
			"and told nothing.\nframes: %v\nserver output:\n%s", seen, srv.logs())
	}
	// The SDK writes the listen's result when its handler's context ends, which
	// is the graceful completion the specification asks for. Its absence would
	// mean the stream was torn down rather than ended.
	var completed bool
	for _, frame := range seen {
		if strings.Contains(frame, `"result"`) && strings.Contains(frame, `"id":1`) {
			completed = true
		}
	}
	if !completed {
		t.Errorf("the stream closed without the listen's completion result; frames: %v", seen)
	}
}

// namedGitLab is an instance that says which one it is, so a response can be
// traced back to the GitLab that produced it.
func namedGitLab(t *testing.T, name string) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"version":"17.0.0","revision":%q}`, name)
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":1,"username":%q}`, name)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// TestSharedServer_TwoPublishedInstancesShareOneShape is the riskiest part of
// the design driven on the wire.
//
// The instance URL is deliberately not in the shape key: two instances of one
// tier are served the same catalog, and what differs between them is the client,
// which is per credential regardless. That is what makes a deployment publishing
// several instances the one that gains most from the sharing, and it is also
// what makes a mistake here send a caller's request to an instance they never
// named. Nothing else drives two credentials at two published instances through
// one server.
//
// The server the shape is registered with carries the credential-less client for
// its instance class, whose base URL is the synthetic gitlab.invalid. A response
// naming that host would mean a handler answered from the registration client
// rather than from the request's own.
func TestSharedServer_TwoPublishedInstancesShareOneShape(t *testing.T) {
	first := namedGitLab(t, "first-instance")
	second := namedGitLab(t, "second-instance")

	srv := startServer(t, nil,
		"--gitlab-url="+first.URL,
		"--gitlab-url="+second.URL,
	)

	instances := []struct {
		name     string
		token    string
		gitlab   *httptest.Server
		revision string
	}{
		{name: "the first published instance", token: "glpat-at-the-first", gitlab: first, revision: "first-instance"},
		{name: "the second published instance", token: "glpat-at-the-second", gitlab: second, revision: "second-instance"},
	}

	for _, tt := range instances {
		t.Run(tt.name, func(t *testing.T) {
			body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"gitlab_execute_action",` +
				`"arguments":{"action":"server.status","params":{}},` +
				`"_meta":{"io.modelcontextprotocol/protocolVersion":"` + protocolVersion + `","io.modelcontextprotocol/clientCapabilities":{}}}}`
			got := srv.do(t, request{
				body: body,
				headers: map[string]string{
					"PRIVATE-TOKEN":    tt.token,
					"GITLAB-URL":       tt.gitlab.URL,
					"Mcp-Param-Action": "server.status",
				},
			})
			if got.status != http.StatusOK {
				t.Fatalf("server.status = %d: %s", got.status, got.body)
			}
			payload := jsonRPCPayload(t, got.body)
			if !strings.Contains(payload, tt.gitlab.URL) {
				t.Errorf("the answer does not name the instance this credential asked for (%s):\n%s", tt.gitlab.URL, payload)
			}
			if !strings.Contains(payload, tt.revision) {
				t.Errorf("the answer did not come from %s, whose revision is %q:\n%s", tt.gitlab.URL, tt.revision, payload)
			}
			for _, other := range instances {
				if other.revision != tt.revision && strings.Contains(payload, other.revision) {
					t.Errorf("the answer names the other published instance %q:\n%s", other.revision, payload)
				}
			}
			if strings.Contains(payload, "gitlab.invalid") {
				t.Errorf("the answer names the synthetic host the shape is registered against:\n%s", payload)
			}
		})
	}

	// One server for both instances is the property under test: with a server
	// per instance there would be no per-request client resolution to get wrong.
	if builds := countShapeBuilds(srv.logs()); builds != 1 {
		t.Errorf("the server built %d configuration shapes for two published instances of one tier, want 1", builds)
	}
}

// TestSharedServer_PerCredentialLimitsProveTheBindingRunsOutermost asserts the
// middleware order the real wiring produces, through what that order decides.
//
// The credential binding is added last of the receiving middlewares, which makes
// it run first, because the telemetry, rate-limit, listen-ceiling and
// subscription middlewares each ask which credential a request belongs to.
// Moving it inward is silent: every tenant would then draw on the shell's own
// default bucket and counter, and nothing would fail to compile or answer. The
// only test of that ordering was one that assembled a chain of its own, which
// proves the middleware and not the wiring.
//
// Each half here is a consequence only the real order can produce. The
// subscription half is covered by the two owner tests above, where one
// credential's watcher must not notify another's session.
//
// Telemetry is the fourth middleware the binding is documented as running
// outside, and it is deliberately not asserted here: it resolves the caller
// through toolutil.ResolveIdentity, which reads the identity the authentication
// gate put on the request context, not the credential state the binding
// installs. Moving the binding inward therefore changes nothing a span records
// today. Keeping it outside stays right as a rule, since an attributer that did
// read the credential would break silently, but there is no consequence to
// assert, and a test asserting one would be asserting its own setup.
func TestSharedServer_PerCredentialLimitsProveTheBindingRunsOutermost(t *testing.T) {
	const (
		firstToken  = "glpat-first-tenant"
		secondToken = "glpat-second-tenant"
		uri         = "gitlab://project/123"
	)

	t.Run("the rate-limit bucket is the credential's own", func(t *testing.T) {
		gitlab := startTwoTenantGitLab(t, firstToken, secondToken)
		// One token in the bucket and a refill of one per thousand seconds, so
		// the bucket the first credential drains stays empty for the rest of
		// the test. At one per second a slow runner could refill it between the
		// two calls below, and the second credential would then pass on a
		// refill rather than on a bucket of its own: the assertion would hold
		// with the binding in the wrong place.
		srv := startServer(t, nil,
			"--gitlab-url="+gitlab.URL,
			"--rate-limit-rps=0.001",
			"--rate-limit-burst=1",
		)

		var drained bool
		for range 15 {
			if isRateLimited(findActionAs(t, srv, firstToken).body) {
				drained = true
				break
			}
		}
		if !drained {
			t.Fatal("15 calls against a bucket of one that does not refill were never limited")
		}

		if got := findActionAs(t, srv, secondToken); isRateLimited(got.body) {
			t.Errorf("a second credential was refused by the first one's exhausted bucket: %s\n"+
				"the binding is running inside the rate limit, so every tenant shares the shell's default bucket", got.body)
		}
	})

	t.Run("the listen ceiling is the credential's own", func(t *testing.T) {
		gitlab := startTwoTenantGitLab(t, firstToken, secondToken)
		srv := startServer(t, map[string]string{"GITLAB_MCP_MAX_LISTEN_STREAMS": "1"},
			"--gitlab-url="+gitlab.URL,
			"--capability-surface=full",
		)

		held := openListen(t, srv, firstToken, uri, 1)
		if _, seen, ok := held.awaitFrame(t, "notifications/subscriptions/acknowledged", 20*time.Second); !ok {
			t.Fatalf("the first stream was never acknowledged; frames: %v", seen)
		}

		refused := openListen(t, srv, firstToken, uri, 2)
		if _, seen, ok := refused.awaitFrame(t, "too many open subscriptions/listen streams", 20*time.Second); !ok {
			t.Errorf("a second stream for the same credential was accepted past a ceiling of one; frames: %v", seen)
		}

		other := openListen(t, srv, secondToken, uri, 3)
		frame, seen, ok := other.awaitFrame(t, "notifications/subscriptions/acknowledged", 20*time.Second)
		if !ok {
			t.Fatalf("a second credential's first stream was refused by the first credential's ceiling; frames: %v\n"+
				"the binding is running inside the listen ceiling, so every tenant shares one counter", seen)
		}
		if strings.Contains(frame, "too many open") {
			t.Errorf("the second credential was refused: %s", frame)
		}
	})
}

// findActionAs drives one gitlab_find_action call as the given credential.
func findActionAs(t *testing.T, srv *server, token string) response {
	t.Helper()
	return srv.do(t, request{
		body: `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"gitlab_find_action",` +
			`"arguments":{"query":"list projects"},` +
			`"_meta":{"io.modelcontextprotocol/protocolVersion":"` + protocolVersion + `","io.modelcontextprotocol/clientCapabilities":{}}}}`,
		headers: map[string]string{"PRIVATE-TOKEN": token},
	})
}

// isRateLimited reports whether a response is the limiter's refusal, by its
// JSON-RPC code as well as its text.
func isRateLimited(body string) bool {
	return strings.Contains(strings.ToLower(body), "rate limit") || strings.Contains(body, "-42900")
}
