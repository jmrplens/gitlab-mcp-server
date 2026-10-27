// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package testutil

import (
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// WriteRecorder is a stand-in GitLab that answers every read with one body and
// every write with another, and keeps the JSON body of the last write it
// received. It is the fixture for the question issue 1306 kept asking of an
// update handler: whether the value a caller set to empty a field, "" or [],
// reached GitLab at all.
type WriteRecorder struct {
	mu      sync.Mutex
	members map[string]json.RawMessage
}

// RecordWrites returns a client on a [WriteRecorder] that answers a GET or HEAD
// with read and any other method with write, both 200. The client is built by
// [NewTestClient], so a GraphQL document is still validated and the request is
// still recorded into the inventory.
func RecordWrites(tb testing.TB, read, write string) (*gitlabclient.Client, *WriteRecorder) {
	tb.Helper()
	recorder := &WriteRecorder{}
	client := NewTestClient(tb, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			RespondJSON(w, http.StatusOK, read)
			return
		}
		recorder.keep(r)
		RespondJSON(w, http.StatusOK, write)
	}))
	return client, recorder
}

// keep reads a write's body and keeps its members, or none when the body is
// not a JSON object.
func (r *WriteRecorder) keep(req *http.Request) {
	raw, _ := io.ReadAll(req.Body)
	var members map[string]json.RawMessage
	_ = json.Unmarshal(raw, &members)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.members = members
}

// Member returns the named member of the last write's body as it was encoded,
// and whether the body carried it at all. A member sent as null reads "null",
// which is how a field only null clears is told from one left out.
func (r *WriteRecorder) Member(name string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value, ok := r.members[name]
	return string(value), ok
}

// memberReporter is the part of a test [WriteRecorder.AssertMembers] reports
// through.
type memberReporter interface {
	Helper()
	Errorf(format string, args ...any)
}

// AssertMembers reports every member of want whose encoding in the last
// write's body differs from the one given, a member the body did not carry
// included.
func (r *WriteRecorder) AssertMembers(t memberReporter, want map[string]string) {
	t.Helper()
	for name, encoded := range want {
		if got, ok := r.Member(name); !ok || got != encoded {
			t.Errorf("the write's %s = %s (carried: %v), want %s", name, got, ok, encoded)
		}
	}
}
