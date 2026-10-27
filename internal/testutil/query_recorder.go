// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package testutil

import (
	"maps"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// QueryRecorder is a stand-in GitLab that answers every request with one body
// and keeps the query of the last request it received, for a test that holds a
// list filter to the query parameter it becomes.
type QueryRecorder struct {
	mu       sync.Mutex
	requests int
	query    url.Values
}

// RecordQueries returns a client on a [QueryRecorder] that answers every
// request with answer and 200. The client is built by [NewTestClient], so the
// request is still recorded into the inventory.
func RecordQueries(tb testing.TB, answer string) (*gitlabclient.Client, *QueryRecorder) {
	tb.Helper()
	recorder := &QueryRecorder{}
	client := NewTestClient(tb, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder.mu.Lock()
		recorder.requests++
		recorder.query = r.URL.Query()
		recorder.mu.Unlock()
		RespondJSON(w, http.StatusOK, answer)
	}))
	return client, recorder
}

// Param returns the named query parameter of the last request and whether it
// carried it at all.
func (r *QueryRecorder) Param(name string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	values, ok := r.query[name]
	if !ok || len(values) == 0 {
		return "", false
	}
	return values[0], true
}

// Requests returns how many requests reached the recorder.
func (r *QueryRecorder) Requests() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.requests
}

// UserIDFilterCase is one value of a filter GitLab reads as a user ID or one
// of None and Any, such as assignee_id and reviewer_id, with what the query
// must carry for it.
type UserIDFilterCase struct {
	// Name names the case as a subtest.
	Name string
	// Value is what the caller sends, nil for a filter left out.
	Value any
	// Want is the query value GitLab must receive.
	Want string
	// Sent reports whether the query carries the parameter at all.
	Sent bool
	// Refused reports whether the handler refuses the value before any
	// request, naming None.
	Refused bool
}

// UserIDFilterCases returns the values every user ID filter is held to: a
// numeric ID, None, any in lower case, 0, which is refused with a pointer to
// None because no user has it, and the filter left out.
func UserIDFilterCases() []UserIDFilterCase {
	return []UserIDFilterCase{
		{Name: "a user ID", Value: 5, Want: "5", Sent: true},
		{Name: "None", Value: "None", Want: "None", Sent: true},
		{Name: "any in lower case", Value: "any", Want: "Any", Sent: true},
		{Name: "zero", Value: 0, Refused: true},
		{Name: "left out"},
	}
}

// Params returns base with the case's value set under name, or base alone for
// a filter left out. base is copied, never changed.
func (c UserIDFilterCase) Params(base map[string]any, name string) map[string]any {
	params := maps.Clone(base)
	if params == nil {
		params = map[string]any{}
	}
	if c.Value != nil {
		params[name] = c.Value
	}
	return params
}

// filterReporter is the part of a test [QueryRecorder.AssertUserIDFilter]
// reports through.
type filterReporter interface {
	Helper()
	Errorf(format string, args ...any)
}

// AssertUserIDFilter reports a call whose outcome for the named filter differs
// from the case: a refusal that did not happen, came after a request, or does
// not name None, or a query that does not carry the value the case wants.
func (r *QueryRecorder) AssertUserIDFilter(t filterReporter, name string, c UserIDFilterCase, err error) {
	t.Helper()
	if c.Refused {
		if err == nil || !strings.Contains(err.Error(), "None") {
			t.Errorf("%s=%v: error = %v, want a refusal naming None", name, c.Value, err)
		}
		if requests := r.Requests(); requests != 0 {
			t.Errorf("%s=%v: %d request(s) reached GitLab, want the refusal before any", name, c.Value, requests)
		}
		return
	}
	if err != nil {
		t.Errorf("%s=%v: unexpected error: %v", name, c.Value, err)
		return
	}
	if got, sent := r.Param(name); sent != c.Sent || got != c.Want {
		t.Errorf("%s=%v: query carries %q (sent %v), want %q (sent %v)", name, c.Value, got, sent, c.Want, c.Sent)
	}
}
