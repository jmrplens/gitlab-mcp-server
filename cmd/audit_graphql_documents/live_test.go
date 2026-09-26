package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/graphqlintrospect"
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
		// Read to the end: one Read may stop short of "metadata" and send
		// the version query the introspection answer.
		payload, _ := io.ReadAll(r.Body)
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
//
// The line is compared whole because it carries three values, two of them
// strings: how many types arrived, the endpoint that sent them and the version
// it named. A line that printed the endpoint where the version goes would still
// contain every word a looser check looks for.
func TestLiveSchema_AnInstanceThatAnswersInFull_IsWhatJudgesTheDocuments(t *testing.T) {
	endpoint := answeringInstance(t, introspectionAnswer(queryOnly))

	schema, judgedBy, err := liveSchema(context.Background(), endpoint, "", "")
	if err != nil {
		t.Fatalf("liveSchema() error = %v, want nil", err)
	}

	if schema.Query == nil || schema.Query.Fields.ForName("ok") == nil {
		t.Errorf("the schema does not carry the instance's query root: %+v", schema.Query)
	}
	want := fmt.Sprintf("%d types from %s (GitLab unknown), fetched now, not the pinned schema",
		graphqlintrospect.MinimumTypes, endpoint)
	if judgedBy != want {
		t.Errorf("the provenance line = %q, want %q", judgedBy, want)
	}
}

// TestLiveSchema_ATruncatedAnswer_NamesWhatArrivedAgainstTheFloor verifies the
// whole refusal of an answer too short to be a GitLab schema. It carries two
// counts and a reader needs them the right way round: one type arrived, and a
// GitLab schema carries more than the floor. The same words with the numbers
// exchanged would say thousands arrived from an instance that sent one.
func TestLiveSchema_ATruncatedAnswer_NamesWhatArrivedAgainstTheFloor(t *testing.T) {
	endpoint := answeringInstance(t, `{"data":{"__schema":{"queryType":{"name":"Query"},"types":[`+queryOnly+`]}}}`)

	_, _, err := liveSchema(context.Background(), endpoint, "", "")

	want := fmt.Sprintf("%s answered with 1 types and a GitLab schema carries more than %d: "+
		"the introspection was truncated, or that instance is not the GitLab this server targets",
		endpoint, graphqlintrospect.MinimumTypes)
	if err == nil || err.Error() != want {
		t.Errorf("liveSchema() error = %v, want %q", err, want)
	}
}

// TestLiveSchema_AnInstanceThatCannotBeJudgedBy_IsRefused verifies the ways
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
			schema, judgedBy, err := liveSchema(context.Background(), answeringInstance(t, testCase.body), "", "")

			if err == nil {
				t.Fatalf("liveSchema() error = nil, want one naming %q", testCase.want)
			}
			if schema != nil || judgedBy != "" {
				t.Errorf("liveSchema() returned %v and %q, want nothing to judge by on failure", schema, judgedBy)
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
		// Read to the end: one Read may stop short of "metadata" and send
		// the version query the introspection answer.
		payload, _ := io.ReadAll(r.Body)
		if strings.Contains(string(payload), "metadata") {
			_, _ = w.Write([]byte(`{"data":{"metadata":{"version":"19.4.0-ee","revision":"abc1234"}}}`))
			return
		}
		_, _ = w.Write([]byte(introspectionAnswer(queryOnly)))
	}))
	t.Cleanup(server.Close)

	_, judgedBy, err := liveSchema(context.Background(), server.URL, "secret", "")
	if err != nil {
		t.Fatalf("liveSchema() error = %v, want nil", err)
	}

	if seen != "Bearer secret" {
		t.Errorf("Authorization = %q, want the bearer credential", seen)
	}
	if !strings.Contains(judgedBy, "GitLab 19.4.0-ee") {
		t.Errorf("the provenance line %q does not name the version the instance reported", judgedBy)
	}
}

// TestLiveSchema_ATokenIsWithheld_ReachesNobodyAndIsExplained verifies the
// other half of that decision. `-live` takes an arbitrary endpoint, so a token
// resolved away by [graphqlintrospect.CredentialFor] must reach no request at
// all, and the report must say the version is unknown because this run
// declined to hand over a credential rather than because the instance would
// not answer. Those two look identical in a report that only prints
// "GitLab unknown", and the first is the one somebody has to act on.
func TestLiveSchema_ATokenIsWithheld_ReachesNobodyAndIsExplained(t *testing.T) {
	var authorized bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			authorized = true
		}
		// Read to the end: one Read may stop short of "metadata" and send
		// the version query the introspection answer.
		payload, _ := io.ReadAll(r.Body)
		if strings.Contains(string(payload), "metadata") {
			_, _ = w.Write([]byte(`{"data":{"metadata":null}}`))
			return
		}
		_, _ = w.Write([]byte(introspectionAnswer(queryOnly)))
	}))
	t.Cleanup(server.Close)

	const reason = "GITLAB_TOKEN belongs to https://gitlab.com and this run asks elsewhere"
	_, judgedBy, err := liveSchema(context.Background(), server.URL, "", reason)
	if err != nil {
		t.Fatalf("liveSchema() error = %v, want nil: a withheld token must not stop the judgement", err)
	}

	if authorized {
		t.Error("the instance received an Authorization header for a run that withheld the token")
	}
	want := fmt.Sprintf("%d types from %s (GitLab unknown), fetched now, not the pinned schema; %s",
		graphqlintrospect.MinimumTypes, server.URL, reason)
	if judgedBy != want {
		t.Errorf("the provenance line = %q, want the version unknown and the reason after it: %q", judgedBy, want)
	}
}
