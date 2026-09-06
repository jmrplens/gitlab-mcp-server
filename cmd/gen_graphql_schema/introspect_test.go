package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// answering returns a run configured against a server that replies with body
// for every request.
func answering(t *testing.T, status int, body string) genRun {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return genRun{endpoint: server.URL, client: server.Client()}
}

// tinySchema is the smallest introspection payload that renders to loadable
// SDL: one object type that is also the query root.
const tinySchema = `{"data":{"__schema":{
  "queryType":{"name":"Query"},
  "types":[{"kind":"OBJECT","name":"Query","fields":[{"name":"ok","args":[],"type":{"kind":"SCALAR","name":"Boolean"}}]}]
}}}`

// TestIntrospect_WellFormedAnswer_ReturnsTheSchema verifies the happy path and
// that the token, when there is one, is sent as a bearer credential.
func TestIntrospect_WellFormedAnswer_ReturnsTheSchema(t *testing.T) {
	var seenAuth, seenQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		var body struct {
			Query string `json:"query"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		seenQuery = body.Query
		_, _ = w.Write([]byte(tinySchema))
	}))
	t.Cleanup(server.Close)

	schema, err := introspect(context.Background(), genRun{endpoint: server.URL, token: "secret", client: server.Client()})
	if err != nil {
		t.Fatalf("introspect() error = %v, want nil", err)
	}

	if len(schema.Types) != 1 || schema.QueryType.Name != "Query" {
		t.Errorf("introspect() returned %+v, want the one-type fixture", schema)
	}
	if seenAuth != "Bearer secret" {
		t.Errorf("Authorization = %q, want the bearer credential", seenAuth)
	}
	if !strings.Contains(seenQuery, "__schema") {
		t.Errorf("the query sent does not ask for __schema:\n%s", seenQuery)
	}
}

// TestIntrospect_AnswersThatCarryNoSchema_AreRefused verifies that a reply
// which decodes but says nothing is reported rather than written out. An empty
// artifact would parse, load, and then accept every broken document silently,
// which is the one failure this whole gate must not have.
func TestIntrospect_AnswersThatCarryNoSchema_AreRefused(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{name: "no __schema member", body: `{"data":{}}`, want: "answered introspection with no types"},
		{name: "a schema with no types", body: `{"data":{"__schema":{"types":[]}}}`, want: "answered introspection with no types"},
		{name: "a data member that is not an object", body: `{"data":[1,2,3]}`, want: "decode the introspection payload"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			schema, err := introspect(context.Background(), answering(t, http.StatusOK, testCase.body))

			if err == nil {
				t.Fatalf("introspect() error = nil, want one naming %q", testCase.want)
			}
			if schema != nil {
				t.Errorf("introspect() schema = %+v, want nil on failure", schema)
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("introspect() error = %q, want it to name %q", err, testCase.want)
			}
		})
	}
}

// TestPost_TransportAndProtocolFailures_AreNamed verifies that each way one
// GraphQL round trip can fail says what happened and which endpoint it was
// asking, since the operator running this command chose that endpoint.
func TestPost_TransportAndProtocolFailures_AreNamed(t *testing.T) {
	unreachable := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	unreachable.Close()

	truncating := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// A body shorter than the length announced makes the client's read
		// fail after the status line has already been accepted.
		w.Header().Set("Content-Length", "4096")
		_, _ = w.Write([]byte("{"))
	}))
	t.Cleanup(truncating.Close)

	cases := []struct {
		name string
		cfg  genRun
		want string
	}{
		{
			name: "an endpoint that is not a URL",
			cfg:  genRun{endpoint: "://not a url", client: http.DefaultClient},
			want: "build the request",
		},
		{
			name: "nothing listening",
			cfg:  genRun{endpoint: unreachable.URL, client: unreachable.Client()},
			want: "ask ",
		},
		{
			name: "a body that ends early",
			cfg:  genRun{endpoint: truncating.URL, client: truncating.Client()},
			want: "read the answer",
		},
		{
			name: "a status that is not 200",
			cfg:  answering(t, http.StatusBadGateway, "<html>gateway</html>"),
			want: "502",
		},
		{
			name: "a body that is not JSON",
			cfg:  answering(t, http.StatusOK, "definitely not json"),
			want: "decode the answer",
		},
		{
			name: "an errors array",
			cfg:  answering(t, http.StatusOK, `{"errors":[{"message":"field not found"},{"message":"and another"}]}`),
			want: "refused the query: field not found; and another",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			raw, err := post(context.Background(), testCase.cfg, "query { ok }")

			if err == nil {
				t.Fatalf("post() error = nil, want one naming %q", testCase.want)
			}
			if raw != nil {
				t.Errorf("post() data = %s, want nil on failure", raw)
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("post() error = %q, want it to name %q", err, testCase.want)
			}
		})
	}
}

// TestInstanceVersion_WhateverTheInstanceSays_NeverStopsTheRun verifies that
// provenance is best effort. GitLab answers the metadata query with null to an
// anonymous caller, and the schema is what the command exists to fetch, so a
// version nobody would tell us is recorded as unknown rather than fatal.
func TestInstanceVersion_WhateverTheInstanceSays_NeverStopsTheRun(t *testing.T) {
	cases := []struct {
		name         string
		body         string
		status       int
		wantVersion  string
		wantRevision string
	}{
		{
			name:         "a version and a revision",
			body:         `{"data":{"metadata":{"version":"19.4.0-pre","revision":"e53e1e5c151"}}}`,
			status:       http.StatusOK,
			wantVersion:  "19.4.0-pre",
			wantRevision: "e53e1e5c151",
		},
		{name: "null, which is what an anonymous caller gets", body: `{"data":{"metadata":null}}`, status: http.StatusOK, wantVersion: unknownVersion},
		{name: "a data member of the wrong shape", body: `{"data":"nope"}`, status: http.StatusOK, wantVersion: unknownVersion},
		{name: "a refusal", body: `{"errors":[{"message":"no"}]}`, status: http.StatusOK, wantVersion: unknownVersion},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			version, revision := instanceVersion(context.Background(), answering(t, testCase.status, testCase.body))

			if version != testCase.wantVersion {
				t.Errorf("version = %q, want %q", version, testCase.wantVersion)
			}
			if revision != testCase.wantRevision {
				t.Errorf("revision = %q, want %q", revision, testCase.wantRevision)
			}
		})
	}
}

// TestSnippet_LongBody_IsShortened verifies that an HTML error page does not
// take the whole terminal with it when an instance answers one.
func TestSnippet_LongBody_IsShortened(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    string
	}{
		{name: "short enough to show whole", payload: "  gateway timeout  ", want: "gateway timeout"},
		{name: "longer than the limit", payload: strings.Repeat("x", 260), want: strings.Repeat("x", 200) + "..."},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := snippet([]byte(testCase.payload)); got != testCase.want {
				t.Errorf("snippet() = %q, want %q", got, testCase.want)
			}
		})
	}
}
