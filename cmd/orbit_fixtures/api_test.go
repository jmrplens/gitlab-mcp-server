// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/netfixture"
)

// TestGitLabAPI_Do_SendsThePathAsWrittenWithTheToken checks a request keeps
// the escapes of its path, which GitLab needs to find a project by its full
// path, carries the token and, with a body, its JSON and content type.
func TestGitLabAPI_Do_SendsThePathAsWrittenWithTheToken(t *testing.T) {
	var path, token, contentType, body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, token, contentType = r.URL.EscapedPath(), r.Header.Get("PRIVATE-TOKEN"), r.Header.Get("Content-Type")
		read, _ := io.ReadAll(r.Body)
		body = string(read)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":7}`)
	}))
	t.Cleanup(server.Close)
	api := gitlabAPI{root: server.URL + "/api/v4", token: "glpat-x", client: server.Client()}

	got := api.do(t.Context(), "POST", "/projects/acme%2Ffixtures", map[string]string{"name": "x"})

	if !got.ok() || got.status != http.StatusCreated || got.String() != `{"id":7}` {
		t.Errorf("answer = %d %q", got.status, got)
	}
	if path != "/api/v4/projects/acme%2Ffixtures" || token != "glpat-x" || contentType != "application/json" || body != `{"name":"x"}` {
		t.Errorf("sent %s with token %q, type %q and body %q", path, token, contentType, body)
	}
	got = api.do(t.Context(), "GET", "/version", nil)
	if contentType != "" || body != "" {
		t.Errorf("a GET carried type %q and body %q", contentType, body)
	}
	var id struct{ ID int }
	if !got.decode(&id) || id.ID != 7 {
		t.Errorf("decode = %+v", id)
	}
}

// TestGitLabAPI_Do_AnUnansweredRequest_IsNoAnswer checks every way a request
// can fail to get an answer ends as status 0 with the reason: a path that
// does not parse, a body that cannot be encoded, a method net/http refuses,
// and an address nothing answers.
func TestGitLabAPI_Do_AnUnansweredRequest_IsNoAnswer(t *testing.T) {
	api := gitlabAPI{root: netfixture.DeadEndpoint(t), token: "x", client: http.DefaultClient}
	cases := []struct {
		name   string
		method string
		path   string
		body   any
		want   string
	}{
		{name: "a path that does not parse", method: "GET", path: "/%zz", want: "does not parse"},
		{name: "a body that cannot be encoded", method: "POST", path: "/x", body: math.Inf(1), want: "unsupported value"},
		{name: "a method net/http refuses", method: "BAD METHOD", path: "/x", want: "invalid method"},
		// The fixture hangs up before any answer: an EOF or a reset.
		{name: "an address nothing answers", method: "GET", path: "/x", want: "/x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := api.do(t.Context(), tc.method, tc.path, tc.body)
			if got.status != 0 || got.ok() || got.err == nil || !strings.Contains(got.String(), tc.want) {
				t.Errorf("answer = %d %q, want no answer naming %q", got.status, got, tc.want)
			}
			var anything any
			if got.decode(&anything) {
				t.Error("no answer decoded")
			}
		})
	}
}

// TestGitLabAPI_Do_AnAnswerCutShort_KeepsItsStatus checks a body that breaks
// off while it is read is an answer with its status and the reason, never
// the half that arrived.
func TestGitLabAPI_Do_AnAnswerCutShort_KeepsItsStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"id":`)
	}))
	t.Cleanup(server.Close)
	got := gitlabAPI{root: server.URL, client: server.Client()}.do(t.Context(), "GET", "/x", nil)
	if got.status != http.StatusOK || got.err == nil || !strings.Contains(got.String(), "reading the answer") {
		t.Errorf("answer = %d %q", got.status, got)
	}
	if !errors.Is(got.err, io.ErrUnexpectedEOF) {
		t.Errorf("err = %v, want an unexpected end of the body", got.err)
	}
}

// TestAnswer_OK_IsThe2xxRangeAlone checks the statuses at either edge of the
// 2xx range: GitLab answers a redirect, 300 among them, for a request it did
// not carry out.
func TestAnswer_OK_IsThe2xxRangeAlone(t *testing.T) {
	cases := []struct {
		status int
		want   bool
	}{
		{status: 199},
		{status: 200, want: true},
		{status: 299, want: true},
		{status: 300},
	}
	for _, tc := range cases {
		t.Run(http.StatusText(tc.status)+" "+strconv.Itoa(tc.status), func(t *testing.T) {
			if got := (answer{status: tc.status}).ok(); got != tc.want {
				t.Errorf("ok() of %d = %t, want %t", tc.status, got, tc.want)
			}
		})
	}
}
