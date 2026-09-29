//go:build httpe2e

// sessions_test.go holds the ceiling on the stateful sessions the process
// keeps (register row HLD-010, issue 951) on the wire.
//
// On --stateless=false the SDK keeps every session a client opens until the
// client deletes it or it sits idle for --session-timeout, and initialize is
// metered to no bucket, so before this ceiling a caller could open sessions as
// fast as it could post. The test opens as many as the ceiling allows, each
// with the standalone stream a stateful client holds open, and then asks for
// more.
package httpe2e

import (
	"context"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// sessionRefusalLine is the operator's line for a refusal at the ceiling.
const sessionRefusalLine = "request refused: too many stateful sessions across the process"

// sessionRevision is the revision a session is opened on: the stateful
// transport refuses 2026-07-28, which has no sessions.
const sessionRevision = "2025-11-25"

// sessionInitialize opens a session.
func sessionInitialize(id int) string {
	return `{"jsonrpc":"2.0","id":` + strconv.Itoa(id) + `,"method":"initialize","params":{"protocolVersion":"` +
		sessionRevision + `","capabilities":{},"clientInfo":{"name":"httpe2e","version":"0"}}}`
}

// sessionHeader is what a request on session carries, or on none.
func sessionHeader(session string) http.Header {
	header := http.Header{"Mcp-Protocol-Version": {sessionRevision}}
	if session != "" {
		header.Set("Mcp-Session-Id", session)
	}
	return header
}

// openSession initializes a session and completes its handshake, for a
// goroutine or the test's own: it touches no *testing.T, and a session the
// server refused comes back as the answer with no id.
func openSession(ctx context.Context, client *http.Client, baseURL string, id int) (string, response, error) {
	opened, err := heldPost(ctx, client, baseURL, sessionInitialize(id), sessionHeader(""))
	if err != nil || opened.status != http.StatusOK {
		return "", opened, err
	}
	session := opened.header.Get("Mcp-Session-Id")
	_, err = heldPost(ctx, client, baseURL, `{"jsonrpc":"2.0","method":"notifications/initialized"}`, sessionHeader(session))
	return session, opened, err
}

// sessionRequest builds a request of method on session with the test's
// credential.
func sessionRequest(ctx context.Context, method, baseURL, session string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, baseURL+"/mcp", http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("PRIVATE-TOKEN", "glpat-held")
	req.Header.Set("Mcp-Protocol-Version", sessionRevision)
	req.Header.Set("Mcp-Session-Id", session)
	return req, nil
}

// openSessionStream opens a session's standalone stream and returns its body,
// which the caller closes to end it.
func openSessionStream(ctx context.Context, client *http.Client, baseURL, session string) (io.ReadCloser, int, error) {
	req, err := sessionRequest(ctx, http.MethodGet, baseURL, session)
	if err != nil {
		return nil, 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, resp.StatusCode, nil
	}
	return resp.Body, resp.StatusCode, nil
}

// endSession deletes a session and reports the status the server answered.
func endSession(ctx context.Context, client *http.Client, baseURL, session string) (int, error) {
	req, err := sessionRequest(ctx, http.MethodDelete, baseURL, session)
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	_ = resp.Body.Close()
	return resp.StatusCode, nil
}

// announcedFigure reads a figure the startup line announced.
func announcedFigure(t *testing.T, srv *server, field string) int {
	t.Helper()
	line := awaitLogLine(t, srv, `"`+field+`":`)
	match := regexp.MustCompile(`"` + field + `":(\d+)`).FindStringSubmatch(line)
	if match == nil {
		t.Fatalf("the startup line announces no %s: %q", field, line)
	}
	figure, err := strconv.Atoi(match[1])
	if err != nil {
		t.Fatalf("the announced %s %q is not a number: %v", field, match[1], err)
	}
	return figure
}

// offerSessionsPastTheCeiling sends count initializes at once, each on a
// connection of its own, and reports how many were refused with the gate's
// 503. It touches no *testing.T, since the requests run on goroutines of their
// own.
func offerSessionsPastTheCeiling(ctx context.Context, baseURL string, count int) int {
	flood := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	var refused atomic.Int64
	var wait sync.WaitGroup
	for i := range count {
		wait.Go(func() {
			resp, err := heldPost(ctx, flood, baseURL, sessionInitialize(5000+i), sessionHeader(""))
			if err == nil && resp.status == http.StatusServiceUnavailable {
				refused.Add(1)
			}
		})
	}
	wait.Wait()
	return int(refused.Load())
}

// openSessions is the sessions a test opened and the streams it holds open on
// them, closed and deleted when the test ends.
type openSessions struct {
	ids     []string
	streams []io.ReadCloser
}

// close ends every stream and deletes every session.
func (o *openSessions) close(client *http.Client, baseURL string) {
	for _, stream := range o.streams {
		_ = stream.Close()
	}
	for _, id := range o.ids {
		_, _ = endSession(context.Background(), client, baseURL, id)
	}
}

// fill opens count sessions, each with its standalone stream open, and fails
// the test at the first the server does not open.
func (o *openSessions) fill(ctx context.Context, t *testing.T, client *http.Client, baseURL string, count int) {
	t.Helper()
	for i := range count {
		session, resp, openErr := openSession(ctx, client, baseURL, i+1)
		if openErr != nil || session == "" {
			t.Fatalf("session %d of %d = %d (%v): %s", i+1, count, resp.status, openErr, truncate(resp.body))
		}
		o.ids = append(o.ids, session)
		stream, status, streamErr := openSessionStream(ctx, client, baseURL, session)
		if streamErr != nil || stream == nil {
			t.Fatalf("the standalone stream of session %d = %d (%v)", i+1, status, streamErr)
		}
		o.streams = append(o.streams, stream)
	}
}

// endFirst closes the first session's stream and deletes the session, and
// fails the test unless the server answers the DELETE with 204.
func (o *openSessions) endFirst(ctx context.Context, t *testing.T, client *http.Client, baseURL string) {
	t.Helper()
	_ = o.streams[0].Close()
	o.streams = o.streams[1:]
	if status, endErr := endSession(ctx, client, baseURL, o.ids[0]); endErr != nil || status != http.StatusNoContent {
		t.Fatalf("DELETE of a session = %d (%v), want 204", status, endErr)
	}
	o.ids = o.ids[1:]
}

// awaitOpen opens one more session, retrying for as long as the server takes
// to give back the slot of a session that just ended: the slot is released
// once the session's connection has finished closing, which the DELETE's 204
// does not wait for.
func (o *openSessions) awaitOpen(ctx context.Context, t *testing.T, client *http.Client, baseURL string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		session, resp, openErr := openSession(ctx, client, baseURL, 3000)
		if openErr == nil && session != "" {
			o.ids = append(o.ids, session)
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("no session opened once one was deleted: %d (%v): %s", resp.status, openErr, truncate(resp.body))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestLimit_ProcessBoundsStatefulSessions starts a stateful server under a
// descriptor limit of 1024 and opens as many sessions as the ceiling that
// limit gives it, each with its standalone stream open, which is what a
// stateful client holds.
//
// The next initialize is refused in the gate, before the SDK creates a
// session: a 503 whose JSON-RPC body echoes its id and carries -50300, with
// the register's fixed Retry-After, the connection closed, the words the held
// ceiling uses, and a line in the log naming the process as the scope. A call
// on a session already open is still served, since the streams of every
// session hold at most half of the held-call slots. Six hundred more
// initializes at once are refused too, and /health answers. Once a session is
// deleted its slot comes back, and a new session opens.
//
// The rate limit is off so one credential can open every session: initialize
// is metered to no bucket anyway, and the calls are not what is measured.
func TestLimit_ProcessBoundsStatefulSessions(t *testing.T) {
	gitlab := startHoldingProjectsGitLab(t)
	srv, err := tryStartServerLaunched(t, ephemeralPort, underDescriptorLimit(heldDescriptorLimit), nil,
		"--gitlab-url="+gitlab.url, "--rate-limit-rps=0", "--stateless=false")
	if err != nil {
		t.Fatalf("starting the server under a descriptor limit of %d: %v", heldDescriptorLimit, err)
	}
	ceiling := announcedFigure(t, srv, "stateful_sessions_per_process")
	if held := announcedFigure(t, srv, "held_requests_per_process"); ceiling != 96 || held != 192 {
		t.Fatalf("the server announced %d sessions and %d held calls under a descriptor limit of %d, want 96 and 192",
			ceiling, held, heldDescriptorLimit)
	}

	client := &http.Client{Transport: &http.Transport{MaxIdleConnsPerHost: 8}}
	defer client.CloseIdleConnections()
	ctx := t.Context()
	opened := &openSessions{}
	defer opened.close(client, srv.baseURL)
	opened.fill(ctx, t, client, srv.baseURL, ceiling)

	refused, err := heldPost(ctx, client, srv.baseURL, sessionInitialize(1000), sessionHeader(""))
	if err != nil {
		t.Fatalf("the initialize past the ceiling could not be made: %v", err)
	}
	assertBusyGateRefusal(t, srv, refused, "1000", sessionRefusalLine, "limit_stateful_sessions", ceiling)
	if refused.header.Get("Mcp-Session-Id") != "" {
		t.Error("the refused initialize was answered with a session id")
	}

	served, err := heldPost(ctx, client, srv.baseURL, legacyCall(2000), sessionHeader(opened.ids[0]))
	if err != nil || served.status != http.StatusOK || !strings.Contains(served.body, "g/p") ||
		strings.Contains(served.body, `"isError":true`) {
		t.Errorf("a call on an open session with every session slot taken = %d (%v): %s; want the project",
			served.status, err, truncate(served.body))
	}

	const extra = 600
	if got := offerSessionsPastTheCeiling(ctx, srv.baseURL, extra); got != extra {
		t.Errorf("%d of the %d initializes offered past the ceiling were refused with 503, want every one", got, extra)
	}
	if health := srv.do(t, request{method: http.MethodGet, path: "/health"}); health.status != http.StatusOK {
		t.Errorf("/health = %d after %d initializes were offered past the ceiling, want 200", health.status, extra)
	}

	opened.endFirst(ctx, t, client, srv.baseURL)
	opened.awaitOpen(ctx, t, client, srv.baseURL)
}
