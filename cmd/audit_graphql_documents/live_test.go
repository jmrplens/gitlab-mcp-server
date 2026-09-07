package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/graphqlintrospect"
)

// queryOnly is an introspection payload carrying one object type that is also
// the query root, which is the smallest schema a document can be judged by.
const queryOnly = `{"kind":"OBJECT","name":"Query","fields":[{"name":"ok","args":[],"type":{"kind":"SCALAR","name":"Boolean"}}]}`

// brokenType is a type whose field has no type at all, which renders to SDL
// that does not parse. It is how the conversion failure is reached without
// waiting for a GitLab release to serve something unrenderable.
const brokenType = `{"kind":"OBJECT","name":"Broken","fields":[{"name":"broken","args":[],"type":null}]}`

// introspectionAnswer builds an introspection payload carrying the given types
// plus enough filler scalars to clear the floor a whole GitLab schema has to
// reach, so a test can exercise what happens after that check rather than
// stopping at it.
func introspectionAnswer(types ...string) string {
	all := append([]string(nil), types...)
	for i := len(all); i < graphqlintrospect.MinimumTypes; i++ {
		all = append(all, fmt.Sprintf(`{"kind":"SCALAR","name":"Filler%04d"}`, i))
	}
	return `{"data":{"__schema":{"queryType":{"name":"Query"},"types":[` + strings.Join(all, ",") + `]}}}`
}

// answeringInstance returns the URL of a server that answers introspection with
// body and the metadata query with what an anonymous caller gets.
func answeringInstance(t *testing.T, body string) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(payload)
		if strings.Contains(string(payload), "metadata") {
			_, _ = w.Write([]byte(`{"data":{"metadata":null}}`))
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server.URL
}

// TestLiveSchema_AnInstanceThatAnswersInFull_IsWhatJudgesTheDocuments verifies
// the fetch this whole mode exists for, including that the provenance line says
// the pin was not consulted and names the version the instance reported, which
// is unknown for the anonymous call an unlicensed instance is asked with.
func TestLiveSchema_AnInstanceThatAnswersInFull_IsWhatJudgesTheDocuments(t *testing.T) {
	endpoint := answeringInstance(t, introspectionAnswer(queryOnly))

	schema, provenance, err := liveSchema(context.Background(), endpoint, "")
	if err != nil {
		t.Fatalf("liveSchema() error = %v, want nil", err)
	}

	if schema.Query == nil || schema.Query.Fields.ForName("ok") == nil {
		t.Errorf("the schema does not carry the instance's query root: %+v", schema.Query)
	}
	for _, want := range []string{"fetched now", "not the pinned schema", "GitLab unknown"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(provenance, want) {
				t.Errorf("the provenance line %q does not contain %q", provenance, want)
			}
		})
	}
}

// TestLiveSchema_AnInstanceThatCannotBeJudgedBy_IsRefused verifies the two ways
// a reachable instance still cannot answer the question.
//
// The truncated answer is the one that matters. An instance that boots and
// serves a fragment of its schema would let every document validate against the
// little that arrived, and the run would report success for a question nobody
// asked, which is worse than not running at all.
func TestLiveSchema_AnInstanceThatCannotBeJudgedBy_IsRefused(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "an answer too short to be a GitLab schema",
			body: `{"data":{"__schema":{"queryType":{"name":"Query"},"types":[` + queryOnly + `]}}}`,
			want: "the introspection was truncated",
		},
		{
			name: "an answer that converts to SDL nothing can parse",
			body: introspectionAnswer(queryOnly, brokenType),
			want: "the converted schema does not parse",
		},
		{
			name: "no schema at all",
			body: `{"data":{}}`,
			want: "answered introspection with no types",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			schema, provenance, err := liveSchema(context.Background(), answeringInstance(t, testCase.body), "")

			if err == nil {
				t.Fatalf("liveSchema() error = nil, want one naming %q", testCase.want)
			}
			if schema != nil || provenance != "" {
				t.Errorf("liveSchema() returned %v and %q, want nothing to judge by on failure", schema, provenance)
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("liveSchema() error = %q, want it to name %q", err, testCase.want)
			}
		})
	}
}

// TestLiveSchema_ATokenIsOffered_ReachesTheInstance verifies that a run given a
// credential sends it, since the version an instance names is the difference
// between a report that says which GitLab judged the documents and one that
// says unknown.
func TestLiveSchema_ATokenIsOffered_ReachesTheInstance(t *testing.T) {
	var seen string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Authorization")
		payload := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(payload)
		if strings.Contains(string(payload), "metadata") {
			_, _ = w.Write([]byte(`{"data":{"metadata":{"version":"19.4.0-ee","revision":"abc1234"}}}`))
			return
		}
		_, _ = w.Write([]byte(introspectionAnswer(queryOnly)))
	}))
	t.Cleanup(server.Close)

	_, provenance, err := liveSchema(context.Background(), server.URL, "secret")
	if err != nil {
		t.Fatalf("liveSchema() error = %v, want nil", err)
	}

	if seen != "Bearer secret" {
		t.Errorf("Authorization = %q, want the bearer credential", seen)
	}
	if !strings.Contains(provenance, "GitLab 19.4.0-ee") {
		t.Errorf("the provenance line %q does not name the version the instance reported", provenance)
	}
}
