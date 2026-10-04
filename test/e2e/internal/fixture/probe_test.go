//go:build e2e

// probe_test.go drives the probes' halves that take a client against the
// stub: a REST answer read as a status and a refusal's body, and a GraphQL
// answer whose errors are kept as an answer rather than a failure.

package fixture

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// TestProbeREST_StatusAndRefusalBody_AreWhatGitLabAnswered checks that a
// probe answers the status of a success with no body, and the status and body
// of a refusal, which is where GitLab names what it refused for.
func TestProbeREST_StatusAndRefusalBody_AreWhatGitLabAnswered(t *testing.T) {
	const path = "/api/v4/projects/7/repository/files/README.md/raw"
	cases := []struct {
		name   string
		answer scriptedAnswer
		want   ProbeAnswer
	}{
		{name: "served", answer: stubOK(map[string]any{"content": "x"}), want: ProbeAnswer{Status: http.StatusOK}},
		{
			name:   "refused",
			answer: stubRefusal(http.StatusForbidden, "insufficient_granular_scope"),
			want:   ProbeAnswer{Status: http.StatusForbidden, Body: `{"message":"insufficient_granular_scope"}`},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.answers(http.MethodGet, path, tc.answer)

			got, err := probeREST(t.Context(), client, http.MethodGet, "projects/7/repository/files/README.md/raw", struct {
				Ref string `url:"ref"`
			}{Ref: "main"})
			if err != nil {
				t.Fatalf("probeREST() error = %v", err)
			}
			if got.Status != tc.want.Status || strings.TrimSpace(got.Body) != tc.want.Body {
				t.Errorf("probeREST() = %+v, want %+v", got, tc.want)
			}
			if requests := stub.recordedRequests(); len(requests) != 1 || requests[0].Query.Get("ref") != "main" {
				t.Errorf("the probe sent %+v, want one request naming the ref", requests)
			}
		})
	}
}

// TestProbes_ThroughAnEnv_ReachTheRunsInstance checks the two halves that take
// an Env: a REST probe sent as another token, and a GraphQL probe sent as one
// and as the run's own, all reach the run's instance and answer what it said.
// Which credential a client built for another token carries is the harness's
// own test of Env.ClientFor.
func TestProbes_ThroughAnEnv_ReachTheRunsInstance(t *testing.T) {
	stub, e := detachedStub(t)
	stub.answers(http.MethodGet, "/api/v4/projects/7/repository/branches", stubOK([]any{}))
	stub.graphqlAnswers = []string{`{"data":{"currentUser":{"id":"gid://gitlab/User/1"}}}`}

	if got := ProbeREST(e, Token{Value: "glpat-probe"}, http.MethodGet, "projects/7/repository/branches", nil); got.Status != http.StatusOK {
		t.Errorf("ProbeREST() = %+v, want the stub's 200", got)
	}
	if got := ProbeGraphQL(e, Token{Value: "glpat-probe"}, `query { currentUser { id } }`, nil); len(got.Data["currentUser"]) == 0 {
		t.Errorf("ProbeGraphQL() as another token = %+v, want the stub's user", got)
	}
	if got := ProbeGraphQL(e, Token{}, `query { currentUser { id } }`, nil); len(got.Data["currentUser"]) == 0 {
		t.Errorf("ProbeGraphQL() as the run's own token = %+v, want the stub's user", got)
	}
	if sent := len(stub.graphqlDocuments); sent != 2 {
		t.Errorf("the stub was sent %d documents, want the two probes", sent)
	}
}

// TestProbeREST_NoAnswer_IsAnError checks that a probe that reached nobody
// says so rather than answering a status of zero.
func TestProbeREST_NoAnswer_IsAnError(t *testing.T) {
	_, client := newStubGitLab(t)

	if got, err := probeREST(cancelledContext(t), client, http.MethodHead, "projects/7", nil); err == nil {
		t.Errorf("probeREST() = %+v on a cancelled context, want an error", got)
	}
}

// cancelledContext is a context that ended before the probe was sent, which
// is the one way to have a request reach nobody without waiting out the
// client's retries.
func cancelledContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	return ctx
}

// TestProbeGraphQL_ErrorsAreAnAnswer checks that a document GitLab answers
// with data and errors comes back with both, the errors as their messages.
func TestProbeGraphQL_ErrorsAreAnAnswer(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.graphqlAnswers = []string{`{"data":{"workItemSubscribe":null},"errors":[{"message":"Access denied: [Work Item: Subscribe]"}]}`}

	got, err := probeGraphQL(t.Context(), client, `mutation { workItemSubscribe(input: {id: "x", subscribed: true}) { errors } }`, nil)
	if err != nil {
		t.Fatalf("probeGraphQL() error = %v", err)
	}
	if string(got.Data["workItemSubscribe"]) != "null" || len(got.Errors) != 1 || got.Errors[0] != "Access denied: [Work Item: Subscribe]" {
		t.Errorf("probeGraphQL() = data %s, errors %q", got.Data["workItemSubscribe"], got.Errors)
	}
}

// TestProbeGraphQL_RefusedRequest_IsAnError checks that a request GitLab
// refused outright, which carries no document answer at all, is a failure of
// the probe rather than an empty answer.
func TestProbeGraphQL_RefusedRequest_IsAnError(t *testing.T) {
	_, client := newStubGitLab(t)

	if got, err := probeGraphQL(cancelledContext(t), client, `query { currentUser { id } }`, nil); err == nil {
		t.Errorf("probeGraphQL() = %+v on a cancelled context, want an error", got)
	}
}
