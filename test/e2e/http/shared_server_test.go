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
		id, uri, meta)
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

	resp, err := srv.httpClient().Do(req)
	if err != nil {
		cancel()
		t.Fatalf("sending the listen for %s: %v", token, err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		cancel()
		t.Fatalf("listen for %s answered %d, want 200", token, resp.StatusCode)
	}

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
