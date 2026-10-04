package main

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
)

// TestStatefulSessionsFor_IsHalfTheHeldCeiling pins the derivation at the held
// ceilings a deployment meets, and the floor: a process that may hold one call
// still keeps one session.
func TestStatefulSessionsFor_IsHalfTheHeldCeiling(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		held, want int64
	}{
		{held: 192, want: 96},
		{held: 193, want: 96},
		{held: 1536, want: 768},
		{held: 229120, want: 114560},
		{held: 458496, want: 229248},
		{held: 3, want: 1},
		{held: 2, want: 1},
		{held: 1, want: 1},
	} {
		t.Run(strconv.FormatInt(tc.held, 10), func(t *testing.T) {
			t.Parallel()
			if got := statefulSessionsFor(tc.held); got != tc.want {
				t.Errorf("statefulSessionsFor(%d) = %d, want %d", tc.held, got, tc.want)
			}
		})
	}
}

// TestProcessStatefulSessions_IsHalfThisProcessHeldCeiling pins the count every
// gate shares to the held-call ceiling this process was sized with.
func TestProcessStatefulSessions_IsHalfThisProcessHeldCeiling(t *testing.T) {
	t.Parallel()
	if want := statefulSessionsFor(processHeldRequests.limit); processStatefulSessions.limit != want {
		t.Errorf("process session ceiling = %d, want %d, half of %d", processStatefulSessions.limit, want,
			processHeldRequests.limit)
	}
}

// TestMcpServerGate_OpensSession covers which requests open a session that can
// outlive them: a POST with no session id, only on a deployment that keeps
// sessions, and only on a revision that has them. A POST on 2026-07-28 or
// later gets a session the SDK closes with it, whether it is the discover a
// client probes with or a call the transport refuses.
func TestMcpServerGate_OpensSession(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		stateless bool
		method    string
		session   string
		revision  string
		want      bool
	}{
		{name: "a POST with no session id", method: http.MethodPost, want: true},
		{name: "a POST of 2025-11-25", method: http.MethodPost, revision: "2025-11-25", want: true},
		{name: "a POST of 2026-07-28", method: http.MethodPost, revision: "2026-07-28"},
		{name: "a POST of a later revision", method: http.MethodPost, revision: "2027-01-01"},
		{name: "a POST on a session", method: http.MethodPost, session: "abc"},
		{name: "a POST on the sessionless transport", stateless: true, method: http.MethodPost},
		{name: "a GET", method: http.MethodGet},
		{name: "a DELETE", method: http.MethodDelete},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequestWithContext(t.Context(), tc.method, "/mcp", http.NoBody)
			if tc.session != "" {
				req.Header.Set(mcpSessionIDHeader, tc.session)
			}
			if tc.revision != "" {
				req.Header.Set("MCP-Protocol-Version", tc.revision)
			}
			g := &mcpServerGate{stateless: tc.stateless}
			if got := g.opensSession(req); got != tc.want {
				t.Errorf("opensSession = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestSessionSlot_IsGivenBackOnce covers the two ends of a slot: the session
// that keeps it is the only one to, a slot no session kept is given back when
// its POST ends, and one a session kept is not, however often either is asked.
func TestSessionSlot_IsGivenBackOnce(t *testing.T) {
	t.Parallel()
	t.Run("kept by a session", func(t *testing.T) {
		t.Parallel()
		sessions := &processSlots{limit: 1}
		sessions.open.Store(1)
		slot := &sessionSlot{sessions: sessions}
		if !slot.keep() {
			t.Fatal("the first request on the session did not keep the slot")
		}
		if slot.keep() {
			t.Error("a second request kept the slot the first one holds")
		}
		slot.releaseUnkept()
		if got := sessions.open.Load(); got != 1 {
			t.Errorf("open = %d after the POST ended, want the session's slot still held", got)
		}
	})
	t.Run("kept by none", func(t *testing.T) {
		t.Parallel()
		sessions := &processSlots{limit: 1}
		sessions.open.Store(1)
		slot := &sessionSlot{sessions: sessions}
		slot.releaseUnkept()
		slot.releaseUnkept()
		if got := sessions.open.Load(); got != 0 {
			t.Errorf("open = %d after the POST ended twice, want its slot given back once", got)
		}
		if slot.keep() {
			t.Error("a slot given back was kept afterwards")
		}
	})
	t.Run("with its stream's slot", func(t *testing.T) {
		t.Parallel()
		sessions, stream := &processSlots{limit: 1}, &processSlots{limit: 2}
		sessions.open.Store(1)
		stream.open.Store(2)
		slot := &sessionSlot{sessions: sessions, stream: stream}
		slot.releaseUnkept()
		slot.releaseUnkept()
		if sessions.open.Load() != 0 || stream.open.Load() != 1 {
			t.Errorf("open = %d sessions and %d held after the POST ended twice, want one of each given back once",
				sessions.open.Load(), stream.open.Load())
		}
	})
}

// TestClaimSessionSlot_HandsTheSlotToOneRequest covers the handover: a POST the
// gate took a session slot for gives it to its first request and to no other,
// and a POST with no such slot, or none at all, gives nothing.
func TestClaimSessionSlot_HandsTheSlotToOneRequest(t *testing.T) {
	t.Parallel()
	if claimSessionSlot("a token no POST registered") != nil {
		t.Error("a request whose POST is gone claimed a slot")
	}
	if claimSessionSlot(registeredCarrier(t, t.Context())) != nil {
		t.Error("a request on a POST the gate took no slot for claimed one")
	}
	slot := &sessionSlot{sessions: &processSlots{limit: 1}}
	slotted := registeredCarrier(t, context.WithValue(t.Context(), sessionSlotKey{}, slot))
	if got := claimSessionSlot(slotted); got != slot {
		t.Errorf("the first request on the POST claimed %v, want the gate's slot", got)
	}
	if claimSessionSlot(slotted) != nil {
		t.Error("a second request on the same POST claimed the slot the first one holds")
	}
}

// connectedServerSession is a server session the test can end, over the SDK's
// in-memory transport.
func connectedServerSession(t *testing.T) (*mcp.ServerSession, *mcp.ClientSession) {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	session, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatalf("connect the server: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "0"}, nil)
	clientSession, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatalf("connect the client: %v", err)
	}
	t.Cleanup(func() {
		_ = clientSession.Close()
		_ = session.Close()
	})
	return session, clientSession
}

// runSessionsMiddleware passes req through the middleware and reports whether
// the handler behind it ran.
func runSessionsMiddleware(t *testing.T, req mcp.Request) bool {
	t.Helper()
	reached := false
	next := func(context.Context, string, mcp.Request) (mcp.Result, error) {
		reached = true
		return &mcp.CallToolResult{}, nil
	}
	if _, err := statefulSessionsMiddleware(next)(t.Context(), "initialize", req); err != nil {
		t.Errorf("the middleware returned %v, want the handler's answer", err)
	}
	return reached
}

// waitOpen waits until the count holds want, and fails the test if it never
// does: a session gives its slot back from a goroutine of its own once it ends.
func waitOpen(t *testing.T, count *processSlots, want int64) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for count.open.Load() != want && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := count.open.Load(); got != want {
		t.Fatalf("open = %d, want %d", got, want)
	}
}

// TestStatefulSessionsMiddleware_TheSessionKeepsTheSlotUntilItEnds covers the
// handover on a real session: the first request on the POST that opened it
// keeps the gate's slot, its stream's held slot with it, the request is
// served, and both are given back only once the session ends.
func TestStatefulSessionsMiddleware_TheSessionKeepsTheSlotUntilItEnds(t *testing.T) {
	t.Parallel()
	sessions, stream := &processSlots{limit: 1}, &processSlots{limit: 1}
	sessions.open.Store(1)
	stream.open.Store(1)
	slot := &sessionSlot{sessions: sessions, stream: stream}
	session, _ := connectedServerSession(t)
	req := &mcp.CallToolRequest{
		Session: session,
		Extra:   carriedExtra(t, context.WithValue(t.Context(), sessionSlotKey{}, slot)),
	}

	if !runSessionsMiddleware(t, req) {
		t.Fatal("the request that opened the session was not served")
	}
	if !slot.kept.Load() {
		t.Fatal("the session did not keep the gate's slot")
	}
	slot.releaseUnkept()
	if sessions.open.Load() != 1 || stream.open.Load() != 1 {
		t.Fatalf("open = %d sessions and %d held once the POST ended, want the session's slots held while it lives",
			sessions.open.Load(), stream.open.Load())
	}
	if err := session.Close(); err != nil {
		t.Fatalf("close the session: %v", err)
	}
	waitOpen(t, sessions, 0)
	waitOpen(t, stream, 0)
}

// TestStatefulSessionsMiddleware_LeavesWhatOpenedNoSession covers the requests
// that pass untouched: one on a POST the gate took no slot for, one with no
// session to hand a slot to, and one whose session is not a server's, which no
// server receives but whose slot, if it took one, no session would give back.
func TestStatefulSessionsMiddleware_LeavesWhatOpenedNoSession(t *testing.T) {
	t.Parallel()
	session, clientSession := connectedServerSession(t)
	for _, tc := range []struct {
		name    string
		slotted bool
		req     func(extra *mcp.RequestExtra) mcp.Request
	}{
		{name: "a POST the gate took no slot for", req: func(extra *mcp.RequestExtra) mcp.Request {
			return &mcp.CallToolRequest{Session: session, Extra: extra}
		}},
		{name: "no session", slotted: true, req: func(extra *mcp.RequestExtra) mcp.Request {
			return &mcp.CallToolRequest{Extra: extra}
		}},
		{name: "a client's session", slotted: true, req: func(*mcp.RequestExtra) mcp.Request {
			return &mcp.ElicitRequest{Session: clientSession}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			slot := &sessionSlot{sessions: &processSlots{limit: 1}}
			carrier := t.Context()
			if tc.slotted {
				carrier = context.WithValue(carrier, sessionSlotKey{}, slot)
			}
			if !runSessionsMiddleware(t, tc.req(carriedExtra(t, carrier))) {
				t.Error("the request was not served")
			}
			if slot.kept.Load() {
				t.Error("a slot was kept by a request with no server session to give it back")
			}
		})
	}
}

// sessionsGate is a gate on the stateful transport with a session count of its
// own and room for one session, and a handler behind it that records whether
// the gate handed it a slot and keeps the slot when told to, as a session
// whose initialize completed does.
type sessionsGate struct {
	gate     *mcpServerGate
	handler  http.Handler
	sessions *processSlots
	keep     bool
	slotted  bool
}

// newSessionsGate builds the gate and the recording handler. The gate counts
// no held call until a test gives it a count to.
func newSessionsGate(t *testing.T, stateless bool) *sessionsGate {
	t.Helper()
	g := &sessionsGate{sessions: &processSlots{limit: 1}}
	gate := newGate(t, okFactory)
	gate.statefulSessions = g.sessions
	gate.stateless = stateless
	g.gate = gate
	g.handler = gate.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slot, slotted := r.Context().Value(sessionSlotKey{}).(*sessionSlot)
		g.slotted = slotted
		if slotted && g.keep {
			slot.keep()
		}
		w.WriteHeader(http.StatusOK)
	}))
	return g
}

// TestMcpServerGate_Middleware_BoundsTheSessionsThePOSTsOpen drives the gate
// the way a stateful client meets the ceiling: a POST that opens a session
// reaches the handler with the gate's slot, which stays taken once the session
// keeps it; the next POST that would open one is refused in the gate with a 503
// echoing its id, with Retry-After and the connection closed, and the operator
// is told which ceiling refused; and a POST whose session did not survive it
// gives its slot back as it returns.
//
// Not parallel: it replaces the process-wide default logger.
func TestMcpServerGate_Middleware_BoundsTheSessionsThePOSTsOpen(t *testing.T) {
	var logged bytes.Buffer
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logged, nil)))
	forgetRefusalLines()

	g := newSessionsGate(t, false)
	unkept := httptest.NewRecorder()
	g.handler.ServeHTTP(unkept, heldPost(t, "1", "ping", "2025-11-25"))
	if unkept.Code != http.StatusOK || !g.slotted {
		t.Fatalf("status %d, slotted %v; want the POST served with the gate's slot", unkept.Code, g.slotted)
	}
	if got := g.sessions.open.Load(); got != 0 {
		t.Fatalf("open = %d after a POST whose session did not survive it, want its slot given back", got)
	}

	g.keep = true
	kept := httptest.NewRecorder()
	g.handler.ServeHTTP(kept, heldPost(t, "2", "initialize", "2025-11-25"))
	if kept.Code != http.StatusOK || g.sessions.open.Load() != 1 {
		t.Fatalf("status %d, open %d; want the session served and its slot kept", kept.Code, g.sessions.open.Load())
	}

	g.slotted = false
	refused := httptest.NewRecorder()
	g.handler.ServeHTTP(refused, heldPost(t, "3", "initialize", "2025-11-25"))
	if refused.Code != http.StatusServiceUnavailable || g.slotted {
		t.Fatalf("status %d, reached the handler %v; want the gate's 503 at the ceiling: %s",
			refused.Code, g.slotted, refused.Body.String())
	}
	if got := refused.Header().Get(headerRetryAfter); got != strconv.Itoa(int(upstreamRetryAfter.Seconds())) {
		t.Errorf("Retry-After = %q, want the register's fixed pause", got)
	}
	if got := refused.Header().Get("Connection"); got != "close" {
		t.Errorf("Connection = %q, want the refused caller's connection closed", got)
	}
	decoded := decodeJSONRPCError(t, refused.Body.String(), "3")
	if decoded.Error.Code != errCodeUpstreamUnavailable || decoded.Error.Message != heldRefusalText {
		t.Errorf("code %d, message %q; want %d and %q", decoded.Error.Code, decoded.Error.Message,
			errCodeUpstreamUnavailable, heldRefusalText)
	}
	line := logged.String()
	if !strings.Contains(line, "too many stateful sessions across the process") || !strings.Contains(line, `"scope":"process"`) ||
		!strings.Contains(line, `"limit_stateful_sessions":1`) {
		t.Errorf("the refusal left no line naming its scope and figure: %s", line)
	}
	if got := g.sessions.open.Load(); got != 1 {
		t.Errorf("open = %d after the refusal, want the one session's slot and no other", got)
	}
}

// TestMcpServerGate_Middleware_CountsOnlyWhatOpensASession holds the other half:
// with the only slot taken, a POST on a session that exists, and any POST on
// the sessionless transport, reach the handler with no slot of the gate's and
// leave the count alone, and a request refused before admission never reaches
// the count.
func TestMcpServerGate_Middleware_CountsOnlyWhatOpensASession(t *testing.T) {
	t.Parallel()
	t.Run("a POST on a session", func(t *testing.T) {
		t.Parallel()
		g := newSessionsGate(t, false)
		g.sessions.open.Store(1)
		req := heldPost(t, "1", "tools/list", "2025-11-25")
		req.Header.Set(mcpSessionIDHeader, "an-open-session")
		rec := httptest.NewRecorder()
		g.handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || g.slotted || g.sessions.open.Load() != 1 {
			t.Errorf("status %d, slotted %v, open %d; want it served, uncounted", rec.Code, g.slotted, g.sessions.open.Load())
		}
	})
	t.Run("the sessionless transport", func(t *testing.T) {
		t.Parallel()
		g := newSessionsGate(t, true)
		g.sessions.open.Store(1)
		rec := httptest.NewRecorder()
		g.handler.ServeHTTP(rec, heldPost(t, "1", "initialize", "2025-11-25"))
		if rec.Code != http.StatusOK || g.slotted || g.sessions.open.Load() != 1 {
			t.Errorf("status %d, slotted %v, open %d; want it served, uncounted", rec.Code, g.slotted, g.sessions.open.Load())
		}
	})
	t.Run("a refusal before admission", func(t *testing.T) {
		t.Parallel()
		g := newSessionsGate(t, false)
		req := heldPost(t, "1", "initialize", "2025-11-25")
		req.Header.Del("PRIVATE-TOKEN")
		rec := httptest.NewRecorder()
		g.handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized || g.sessions.open.Load() != 0 {
			t.Errorf("status %d, open %d; want the 401 and no slot taken", rec.Code, g.sessions.open.Load())
		}
	})
}

// TestMcpServerGate_Middleware_TakesTheStreamsSlotWithTheSession covers the one
// request a stateful session holds open that is not a call, its standalone
// stream. The POST that opens the session takes a held slot for it beside the
// session's, and the session keeps both; the stream's GET then takes no slot
// of its own, so it is served with every held slot taken, where a refused one
// would leave the session without it for its whole life. With no held slot
// left, the POST that would open a session is refused in the gate instead, with
// the held ceiling's 503 and its log line, and gives back the session slot it
// had taken.
//
// Not parallel: it replaces the process-wide default logger.
func TestMcpServerGate_Middleware_TakesTheStreamsSlotWithTheSession(t *testing.T) {
	var logged bytes.Buffer
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logged, nil)))
	forgetRefusalLines()

	g := newSessionsGate(t, false)
	g.sessions.limit = 2
	held := &processSlots{limit: 1}
	g.gate.held = held
	g.keep = true

	opened := httptest.NewRecorder()
	g.handler.ServeHTTP(opened, heldPost(t, "1", "initialize", "2025-11-25"))
	if opened.Code != http.StatusOK || g.sessions.open.Load() != 1 || held.open.Load() != 1 {
		t.Fatalf("status %d, open %d sessions and %d held; want the session kept with its stream's slot",
			opened.Code, g.sessions.open.Load(), held.open.Load())
	}

	stream := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/mcp", http.NoBody)
	stream.Header.Set("PRIVATE-TOKEN", gateTestToken)
	stream.Header.Set(mcpSessionIDHeader, "an-open-session")
	served := httptest.NewRecorder()
	g.handler.ServeHTTP(served, stream)
	if served.Code != http.StatusOK || held.open.Load() != 1 {
		t.Errorf("the stream with every held slot taken = %d, open %d held; want it served on the session's slot",
			served.Code, held.open.Load())
	}

	g.slotted = false
	refused := httptest.NewRecorder()
	g.handler.ServeHTTP(refused, heldPost(t, "2", "initialize", "2025-11-25"))
	if refused.Code != http.StatusServiceUnavailable || g.slotted || !strings.Contains(refused.Body.String(), heldRefusalText) {
		t.Fatalf("status %d, reached the handler %v: %s; want the held ceiling's 503 before the SDK",
			refused.Code, g.slotted, refused.Body.String())
	}
	if g.sessions.open.Load() != 1 || held.open.Load() != 1 {
		t.Errorf("open %d sessions and %d held after the refusal, want the kept session's slots and no other",
			g.sessions.open.Load(), held.open.Load())
	}
	if line := logged.String(); !strings.Contains(line, "too many requests held across the process") ||
		!strings.Contains(line, `"limit_held_requests":1`) {
		t.Errorf("the refusal left no line naming the held ceiling: %s", line)
	}
}

// startSessionsServer mounts the handler chain a legacy deployment on the
// stateful transport mounts over the shaped pool, and serves it: each POST runs
// through the gate, the carrier, the SDK and the real server shape, counted on
// the process's own counts.
func startSessionsServer(t *testing.T) *httptest.Server {
	t.Helper()
	return startSessionsServerWith(t, func(h http.Handler) http.Handler { return h })
}

// startSessionsServerWith is startSessionsServer with wrap around the whole
// chain, for a test that watches what the chain answers.
func startSessionsServerWith(t *testing.T, wrap func(http.Handler) http.Handler) *httptest.Server {
	t.Helper()
	gitlab := newHoldingGitLab(t)
	cfg := &config.Config{
		GitLabURL: gitlab.url, Tier: edition.Free, TierExplicit: true, IgnoreScopes: true, Stateless: false,
		ToolSurface: config.ToolSurfaceDynamic, CapabilitySurface: config.CapabilitySurfaceFull,
		MaxHTTPClients: config.DefaultMaxHTTPClients, SessionTimeout: config.DefaultSessionTimeout,
	}
	binding, pool := newShapedServerPool(t.Context(), cfg)
	t.Cleanup(pool.Close)
	mux := http.NewServeMux()
	registerLegacyMCPHandlers(t.Context(), cfg, pool, binding, mux)
	srv := httptest.NewServer(wrap(mux))
	t.Cleanup(srv.Close)
	return srv
}

// fillProcessStatefulSessions takes every slot of the process's session count
// but free, and gives them back when the test ends.
func fillProcessStatefulSessions(t *testing.T, free int64) {
	t.Helper()
	settleProcessSlots(t)
	previous := processStatefulSessions.open.Swap(processStatefulSessions.limit - free)
	t.Cleanup(func() { processStatefulSessions.open.Store(previous) })
}

// settleProcessSlots waits until no session or call an earlier test opened
// still holds a slot of the process's two counts. A session gives its slots
// back from a goroutine of its own once it has ended, after the DELETE that
// ended it was answered, so a test that runs next and fills a count, or reads
// one to compare against, would otherwise take that session's slots for its
// own and see them come back in the middle of its assertions: one session slot
// more free than it made, and one held slot fewer than it counted.
func settleProcessSlots(t *testing.T) {
	t.Helper()
	waitOpen(t, processStatefulSessions, 0)
	waitOpen(t, processHeldRequests, 0)
}

// initializeBody opens a session on an older revision.
func initializeBody(id int) string {
	return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"initialize","params":{"protocolVersion":"2025-11-25",`+
		`"capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`, id)
}

// deleteSession ends a session the way a client does.
func deleteSession(t *testing.T, srv *httptest.Server, id string) int {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodDelete, srv.URL+"/mcp", http.NoBody)
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}
	req.Header.Set("PRIVATE-TOKEN", gateTestToken)
	req.Header.Set(mcpSessionIDHeader, id)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

// TestStatefulSessions_EachSessionHoldsASlotUntilItEnds drives the whole chain
// with one session slot of the process free: an initialize opens a session
// that keeps the slot, and a held slot for its stream, the next initialize is
// refused in the gate with the 503, the session's DELETE gives both slots
// back, and a new session opens.
//
// Not parallel: it fills the process-wide count.
func TestStatefulSessions_EachSessionHoldsASlotUntilItEnds(t *testing.T) {
	srv := startSessionsServer(t)
	fillProcessStatefulSessions(t, 1)
	full := processStatefulSessions.limit
	held := processHeldRequests.open.Load()

	status, header, got := postHeld(t, srv, gateTestToken, initializeBody(1), legacyHeader("2025-11-25"))
	session := header.Get(mcpSessionIDHeader)
	if status != http.StatusOK || session == "" {
		t.Fatalf("initialize = %d with session %q: %s", status, session, got)
	}
	if processStatefulSessions.open.Load() != full || processHeldRequests.open.Load() != held+1 {
		t.Fatalf("open = %d sessions and %d held with the session open, want %d and %d",
			processStatefulSessions.open.Load(), processHeldRequests.open.Load(), full, held+1)
	}

	status, header, got = postHeld(t, srv, gateTestToken, initializeBody(2), legacyHeader("2025-11-25"))
	if status != http.StatusServiceUnavailable || !strings.Contains(got, heldRefusalText) || !strings.Contains(got, `"id":2`) {
		t.Errorf("the initialize past the ceiling = %d: %s; want the gate's 503 echoing id 2", status, got)
	}
	if header.Get(mcpSessionIDHeader) != "" {
		t.Error("the refused initialize was answered with a session id")
	}

	if code := deleteSession(t, srv, session); code != http.StatusNoContent {
		t.Fatalf("DELETE = %d, want 204", code)
	}
	waitOpen(t, processStatefulSessions, full-1)
	waitOpen(t, processHeldRequests, held)
	status, header, got = postHeld(t, srv, gateTestToken, initializeBody(3), legacyHeader("2025-11-25"))
	if status != http.StatusOK || header.Get(mcpSessionIDHeader) == "" {
		t.Errorf("initialize once the slot was given back = %d: %s; want a new session", status, got)
	}
	if code := deleteSession(t, srv, header.Get(mcpSessionIDHeader)); code != http.StatusNoContent {
		t.Errorf("DELETE = %d, want 204", code)
	}
	waitOpen(t, processStatefulSessions, full-1)
}

// TestStatefulSessions_APOSTWhoseSessionDiesWithItGivesItsSlotBack covers the
// sessions the SDK creates and closes within one POST: a ping sent before any
// initialize reaches a handler, so its session keeps the slot until the SDK
// closes it as the POST ends; and a call sent before any initialize is refused
// by the SDK before any handler runs, so the gate gives the slot back itself.
// Either way the slot is free once the POST is over.
//
// Not parallel: it fills the process-wide count.
func TestStatefulSessions_APOSTWhoseSessionDiesWithItGivesItsSlotBack(t *testing.T) {
	srv := startSessionsServer(t)
	fillProcessStatefulSessions(t, 1)
	free := processStatefulSessions.limit - 1

	for _, tc := range []struct{ name, body string }{
		{name: "a ping", body: `{"jsonrpc":"2.0","id":1,"method":"ping"}`},
		{name: "a call", body: `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, _, got := postHeld(t, srv, gateTestToken, tc.body, legacyHeader("2025-11-25"))
			if status != http.StatusOK {
				t.Errorf("status %d: %s; want the SDK's answer", status, got)
			}
			waitOpen(t, processStatefulSessions, free)
		})
	}
}

// TestStatefulSessions_AModernPOST_IsAnsweredByTheSDKWithEverySlotTaken covers a
// 2026-07-28 client probing a stateful deployment whose every session slot is
// taken: its server/discover, and any call, is told the revision is not served
// here and which ones are, so the client learns to fall back, as it is with
// slots free. Neither is refused as busy, which would tell the client to retry
// what it should instead downgrade, and neither takes a slot, since the SDK
// closes the session of each with its POST.
//
// Not parallel: it fills the process-wide count.
func TestStatefulSessions_AModernPOST_IsAnsweredByTheSDKWithEverySlotTaken(t *testing.T) {
	srv := startSessionsServer(t)
	fillProcessStatefulSessions(t, 0)
	const meta = `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}`
	for _, method := range []string{"server/discover", "ping"} {
		t.Run(method, func(t *testing.T) {
			body := `{"jsonrpc":"2.0","id":1,"method":"` + method + `","params":{` + meta + `}}`
			header := http.Header{"Mcp-Protocol-Version": {"2026-07-28"}, "Mcp-Method": {method}}
			status, _, got := postHeld(t, srv, gateTestToken, body, header)
			if status != http.StatusBadRequest || !strings.Contains(got, "-32022") || !strings.Contains(got, `"2025-11-25"`) {
				t.Errorf("%s with every session slot taken = %d: %s; want the SDK's 400 with -32022 naming 2025-11-25",
					method, status, got)
			}
			if got := processStatefulSessions.open.Load(); got != processStatefulSessions.limit {
				t.Errorf("open = %d sessions, want the %d the test holds and none taken", got, processStatefulSessions.limit)
			}
		})
	}
}

// streamStatuses reports the status the chain answers each GET with, which is
// a stateful session's standalone stream. It unwraps to the writer it wraps,
// so the stream can still be flushed as it is written.
type streamStatuses struct {
	http.ResponseWriter
	statuses chan<- int
}

// WriteHeader records a GET's status and passes it on.
func (s *streamStatuses) WriteHeader(status int) {
	select {
	case s.statuses <- status:
	default:
	}
	s.ResponseWriter.WriteHeader(status)
}

// Unwrap is the writer the chain's own flushes reach.
func (s *streamStatuses) Unwrap() http.ResponseWriter {
	return s.ResponseWriter
}

// TestStatefulSessions_AGoSDKClientKeepsItsStreamWithEveryHeldSlotTaken drives
// the SDK's own client against the whole chain with one held slot of the
// process left. Its initialize takes that slot for the session's standalone
// stream, so the stream the client opens next is served although every held
// slot is then taken; the SDK's client does not ask again for a stream it was
// refused, and a session that lost it would carry on without any message the
// server sends outside a response. With no held slot left, the next client's
// initialize is refused instead, which the client reports as the connection
// failing.
//
// Not parallel: it fills the process-wide counts.
func TestStatefulSessions_AGoSDKClientKeepsItsStreamWithEveryHeldSlotTaken(t *testing.T) {
	statuses := make(chan int, 16)
	srv := startSessionsServerWith(t, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				w = &streamStatuses{ResponseWriter: w, statuses: statuses}
			}
			next.ServeHTTP(w, r)
		})
	})
	fillProcessHeldRequests(t, 1)
	sessions := processStatefulSessions.open.Load()
	transport := func() *mcp.StreamableClientTransport {
		return &mcp.StreamableClientTransport{Endpoint: srv.URL + "/mcp", HTTPClient: &http.Client{
			Transport: headerRoundTripper{base: http.DefaultTransport, header: http.Header{"PRIVATE-TOKEN": {gateTestToken}}},
		}}
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "sessions-client", Version: "0"}, nil)

	clientSession, err := client.Connect(t.Context(), transport(), nil)
	if err != nil {
		t.Fatalf("the client with one held slot left could not connect: %v", err)
	}
	select {
	case status := <-statuses:
		if status != http.StatusOK {
			t.Errorf("the session's standalone stream = %d, want it served on the slot its initialize took", status)
		}
	case <-time.After(10 * time.Second):
		t.Error("the client opened no standalone stream")
	}
	if got := processHeldRequests.open.Load(); got != processHeldRequests.limit {
		t.Errorf("open = %d held with the session and its stream open, want every one of %d taken",
			got, processHeldRequests.limit)
	}

	if refused, connectErr := client.Connect(t.Context(), transport(), nil); connectErr == nil {
		_ = refused.Close()
		t.Error("a second client connected with no held slot left for its stream")
	}

	if closeErr := clientSession.Close(); closeErr != nil {
		t.Errorf("closing the session: %v", closeErr)
	}
	waitOpen(t, processHeldRequests, processHeldRequests.limit-1)
	waitOpen(t, processStatefulSessions, sessions)
}

// TestRegisterOAuthMCPHandlers_RefusesPastTheProcessSessionCeiling holds the
// oauth mode's wiring of the session count: with every slot of the process
// taken, an initialize with a verified bearer is refused in the gate with the
// 503, as it is in legacy mode.
//
// Not parallel: it fills the process-wide count.
func TestRegisterOAuthMCPHandlers_RefusesPastTheProcessSessionCeiling(t *testing.T) {
	gitlab := newMockGitLabServerWithUser(t)
	cfg := &config.Config{
		GitLabURL:      gitlab.URL,
		MaxHTTPClients: config.DefaultMaxHTTPClients,
		SessionTimeout: config.DefaultSessionTimeout,
		ToolSurface:    config.ToolSurfaceDynamic,
		AuthMode:       config.AuthModeOAuth,
		PublicURL:      "https://mcp.example.com",
		OAuthCacheTTL:  config.DefaultOAuthCacheTTL,
		Stateless:      false,
		Tier:           edition.Free,
		TierExplicit:   true,
		IgnoreScopes:   true,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addr, errCh := oauthAddr(t, ctx, cfg)
	fillProcessStatefulSessions(t, 0)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+addr+"/mcp",
		strings.NewReader(initializeBody(5)))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set(hdrContentType, mimeJSON)
	req.Header.Set("Accept", mimeJSONSSE)
	req.Header.Set("MCP-Protocol-Version", "2025-11-25")
	req.Header.Set("Authorization", "Bearer "+testToken)
	resp, err := testHTTPClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	got := readAndCloseBody(t, resp)
	if resp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(got, heldRefusalText) {
		t.Errorf("status %d, body %s; want the session ceiling's 503", resp.StatusCode, got)
	}

	cancel()
	select {
	case srvErr := <-errCh:
		if srvErr != nil {
			t.Fatalf("serveHTTP error: %v", srvErr)
		}
	case <-time.After(testHTTPLivenessTimeout):
		t.Fatal("shutdown timeout")
	}
}
