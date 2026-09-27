//go:build stdioe2e

// listen_end_test.go holds what this server sends when it tears down a
// subscriptions/listen on stdio at 2026-07-28. It is the fourth item of issue
// 961 and row 10 of docs/development/upstream-bugs.md.
//
// 2026-07-28 says a server MUST send notifications/cancelled referencing a
// listen's request id when it tears that subscription down, and go-sdk offers
// no method that sends one for a request the client made (go-sdk#1263). What
// the client gets instead is the listen's own completion result, carrying the
// watch-end reason this server stamps on it. This test pins both halves, so the
// change that starts sending the notification fails here with a message saying
// what to update, and the completion result is shown to have survived it.
package stdioe2e

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// listenEndFix is what a failure about the missing notification asks the pull
// request to do.
const listenEndFix = "Update row 10 of docs/development/upstream-bugs.md and its section, the section of " +
	"docs/development/adr/adr-0015-polled-resource-subscriptions.md on how a listen ends, F-24 in " +
	"internal/tenancy/decision.go, and this test, keeping the watch-end reason on the completion result (PAT-005), " +
	"in this same pull request."

// watchEndMetaKey is the vendor key the server stamps a listen's ending under.
// It is restated rather than imported: this module drives the binary from
// outside, and the key is part of what a client reads.
const watchEndMetaKey = "io.github.jmrplens/watch-end"

// disappearingIssueGitLab serves the startup probes and one issue that answers
// until gone is set and 404 afterwards, which is how a watched resource is
// deleted or withdrawn from the watcher's credential.
func disappearingIssueGitLab(t *testing.T, gone *atomic.Bool) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"version":"17.0.0","revision":"abcdef"}`)
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"id":7,"username":"someone","name":"Some One"}`)
	})
	// An opened issue is polled at the floor, so the read that finds it gone
	// comes one floor interval after the acknowledgment rather than a minute.
	mux.HandleFunc("/api/v4/projects/42/issues/5", func(w http.ResponseWriter, _ *http.Request) {
		if gone.Load() {
			w.WriteHeader(http.StatusNotFound)
			writeJSON(w, `{"message":"404 Not found"}`)
			return
		}
		writeJSON(w, `{"id":500,"iid":5,"project_id":42,"title":"watched","state":"opened","labels":[],"web_url":"http://example.invalid/g/proj/-/issues/5"}`)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

// TestListenEnd_WatchedResourceGone_CompletesTheListenAndSendsNoCancellation
// pins how a listen ends on stdio when its watcher retires on a 404: the
// listen's own result carries the resource_gone reason with the status, no
// notifications/cancelled is sent before or after it, and the connection keeps
// serving.
//
// The resource starts answering 404 only after the acknowledgment, so the
// subscription is accepted first and the ending is the server's decision about
// a live watch rather than a refusal. The call made after the ending is what
// bounds the absence: stdio is one ordered stream, so a notification the
// teardown sent would arrive before that call's response.
func TestListenEnd_WatchedResourceGone_CompletesTheListenAndSendsNoCancellation(t *testing.T) {
	var gone atomic.Bool
	env := baseEnv(disappearingIssueGitLab(t, &gone))
	env["GITLAB_MCP_CAPABILITY_SURFACE"] = "full"
	s := startSession(t, env)

	const listenID = 7
	s.send(t, request(listenID, "subscriptions/listen",
		`{"notifications":{"resourceSubscriptions":["gitlab://project/42/issue/5"]}}`))

	var cancellations []map[string]any
	var ending map[string]any
	acknowledged := false
	deadline := time.Now().Add(60 * time.Second)
	for ending == nil && time.Now().Before(deadline) {
		got := s.readMessage(t, 60*time.Second)
		switch method, _ := got["method"].(string); {
		case method == "notifications/subscriptions/acknowledged":
			acknowledged = true
			gone.Store(true)
		case method == "notifications/cancelled":
			cancellations = append(cancellations, got)
		case sameID(got["id"], listenID):
			ending = got
		}
	}
	if ending == nil {
		t.Fatalf("the listen was never ended after its resource answered 404:\n%s", s.stderrText())
	}
	if !acknowledged {
		t.Fatalf("the listen ended without being acknowledged, so this is a refusal and not a teardown: %v", ending)
	}
	if ending["error"] != nil {
		t.Fatalf("the teardown answered the listen with an error, want its completion result: %v", ending["error"])
	}

	result, _ := ending["result"].(map[string]any)
	meta, _ := result["_meta"].(map[string]any)
	end, _ := meta[watchEndMetaKey].(map[string]any)
	if end["reason"] != "resource_gone" || end["status"] != float64(http.StatusNotFound) {
		t.Errorf("the listen's completion result carries %s = %v, want reason resource_gone with status 404 (END-001): %v",
			watchEndMetaKey, end, result)
	}

	// The connection still serves, and nothing the teardown sent was left in
	// the pipe ahead of this answer.
	if got := s.call(t, request(listenID+1, "tools/list", "")); got["error"] != nil {
		t.Fatalf("tools/list after the teardown failed: %v", got["error"])
	}
	s.mu.Lock()
	for _, n := range s.notifications {
		if n["method"] == "notifications/cancelled" {
			cancellations = append(cancellations, n)
		}
	}
	s.mu.Unlock()
	if len(cancellations) != 0 {
		t.Errorf("the teardown sent %d notifications/cancelled (%v): the server now tells the client it ended the listen, which go-sdk#1263 asked go-sdk to make possible. %s",
			len(cancellations), cancellations, listenEndFix)
	}
}
