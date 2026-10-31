// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build httpe2e

// request_state_test.go drives the state a multi-round-trip elicitation hands a
// client between rounds through a shared server (issue 953): the rounds of one
// guided flow, each a stateless POST of its own, complete under the credential
// that answered them, and the same state replayed under another credential of
// the same configuration shape is asked again from the first question rather
// than continued with the first caller's answers.
//
// Since ADR-0020 the credentials of one shape share one server, and so one
// signing key, which until issue 953 bound the state to the call alone: state
// one caller was issued verified for any other caller of that server for its
// ten minutes. A unit test can show the binding refusing a state it is handed;
// only the real binary shows that the principal the binding reads is the
// credential each POST was authenticated as.
package httpe2e

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// createdIssue is one issue the fake instance was asked to create: the
// credential that asked and the title it gave.
type createdIssue struct {
	token string
	title string
}

// issueFlowGitLab is an instance for the guided issue creation. It admits two
// credentials as two users and records each issue it is asked to create, so a
// test can tell whose answers reached GitLab and under which credential.
type issueFlowGitLab struct {
	*httptest.Server
	mu      sync.Mutex
	created []createdIssue
}

// issues returns what the instance has been asked to create so far.
func (g *issueFlowGitLab) issues() []createdIssue {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]createdIssue(nil), g.created...)
}

// startIssueFlowGitLab starts the instance, admitting first as user 7 and
// second as user 8 and refusing every other token.
func startIssueFlowGitLab(t *testing.T, first, second string) *issueFlowGitLab {
	t.Helper()

	fake := &issueFlowGitLab{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"17.0.0","revision":"abcdef"}`))
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Header.Get("PRIVATE-TOKEN") {
		case first:
			_, _ = w.Write([]byte(`{"id":7,"username":"first"}`))
		case second:
			_, _ = w.Write([]byte(`{"id":8,"username":"second"}`))
		default:
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(plainUnauthorizedBody))
		}
	})
	mux.HandleFunc("POST /api/v4/projects/42/issues", func(w http.ResponseWriter, r *http.Request) {
		var asked struct {
			Title string `json:"title"`
		}
		if err := json.NewDecoder(r.Body).Decode(&asked); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		fake.mu.Lock()
		fake.created = append(fake.created, createdIssue{token: r.Header.Get("PRIVATE-TOKEN"), title: asked.Title})
		fake.mu.Unlock()
		issue, err := json.Marshal(map[string]any{
			"id": 1, "iid": 1, "project_id": 42, "title": asked.Title, "state": "opened",
			"web_url": "http://example.invalid/g/p/-/issues/1",
		})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(issue)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	fake.Server = httptest.NewServer(mux)
	t.Cleanup(fake.Close)
	return fake
}

// TestSharedServer_RequestState_IsBoundToTheCredentialItWasIssuedUnder walks
// the guided issue creation through its rounds as one credential, a stateless
// POST each, echoing the state every round hands the next, to the final
// confirmation. It then presents that state with the approval under a second
// credential of the same shape, which must be asked the first question again,
// with nothing created, and finally under the first credential, which must
// complete the flow with its own answers.
//
// Before issue 953 the replay continued: the state verified for any caller of
// the server, so the second credential's approval created an issue under its
// own credential with the title the first had typed.
func TestSharedServer_RequestState_IsBoundToTheCredentialItWasIssuedUnder(t *testing.T) {
	const (
		action  = "interactive.issue_create"
		params  = `{"project_id":"42"}`
		first   = "glpat-answers-the-questions"
		second  = "glpat-replays-the-state"
		title   = "Asked by the first credential"
		approve = `{"confirm":{"action":"accept","content":{"confirmed":true}}}`
	)
	gitlab := startIssueFlowGitLab(t, first, second)
	srv := startServer(t, nil, "--gitlab-url="+gitlab.URL)

	round := func(t *testing.T, token, responses, state string) map[string]any {
		t.Helper()
		return answerOf(t, postMessagesAs(t, srv, token, action,
			guidedFlowBody(t, action, params, formElicitation, responses, state, false)))
	}

	answer := round(t, first, "", "")
	// sequential: each round answers the question the previous one asked and echoes the state it handed over
	for _, step := range []struct{ asks, responses string }{
		{asks: "title", responses: `{"title":{"action":"accept","content":{"title":"` + title + `"}}}`},
		{asks: "description", responses: `{"description":{"action":"decline"}}`},
		{asks: "labels", responses: `{"labels":{"action":"decline"}}`},
		{asks: "confidential", responses: `{"confidential":{"action":"decline"}}`},
	} {
		formOf(t, answer, step.asks)
		answer = round(t, first, step.responses, requestStateOf(answer))
	}
	formOf(t, answer, "confirm")
	state := requestStateOf(answer)
	if state == "" {
		t.Fatalf("the round asking for the confirmation handed over no state: %v", answer)
	}

	t.Run("another credential replaying the state is asked again", func(t *testing.T) {
		replayed := round(t, second, approve, state)
		if message, property, _ := formOf(t, replayed, "title"); property != "title" {
			t.Errorf("the replay asks %q for %q, want the title asked again", message, property)
		}
		if created := gitlab.issues(); len(created) != 0 {
			t.Errorf("the replay reached GitLab: %+v", created)
		}
	})

	t.Run("the credential it was issued under completes the flow", func(t *testing.T) {
		done := round(t, first, approve, state)
		result, _ := done["result"].(map[string]any)
		if result["resultType"] == "input_required" || result["isError"] == true {
			t.Fatalf("the last round = %v, want the created issue", done)
		}
		created := gitlab.issues()
		if want := (createdIssue{token: first, title: title}); len(created) != 1 || created[0] != want {
			t.Errorf("GitLab was asked to create %+v, want exactly %+v", created, want)
		}
	})
}
