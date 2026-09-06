package testutil

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/graphqlschema"
)

// recordingReporter stands in for *testing.T so the failure paths can be
// exercised without failing the test that exercises them.
type recordingReporter struct {
	name     string
	messages []string
}

// Helper satisfies the reporter interface and does nothing: there is no test
// goroutine to attribute a line to.
func (r *recordingReporter) Helper() {}

// Name returns the test name the exemption lookup keys on.
func (r *recordingReporter) Name() string { return r.name }

// Errorf records what would have been reported.
func (r *recordingReporter) Errorf(format string, args ...any) {
	r.messages = append(r.messages, strings.TrimSpace(fmt.Sprintf(format, args...)))
}

// joined returns every recorded message as one string for substring assertions.
func (r *recordingReporter) joined() string { return strings.Join(r.messages, "\n") }

// failingBody is a request body whose read fails, which is the one way the
// validator can be handed a request it cannot inspect.
type failingBody struct{}

// Read always fails.
func (failingBody) Read([]byte) (int, error) { return 0, errors.New("body is gone") }

// Close satisfies io.ReadCloser.
func (failingBody) Close() error { return nil }

// graphQLRequest builds a POST to the GraphQL endpoint carrying body.
func graphQLRequest(t *testing.T, body string) *http.Request {
	t.Helper()
	return httptest.NewRequestWithContext(t.Context(), http.MethodPost, "http://gitlab.example.com"+graphQLPath, strings.NewReader(body))
}

// TestValidateGraphQLRequest_RefusedDocument_ReportsEveryReason verifies the
// core of the gate: a document GitLab would refuse is reported, named by its
// operation signature, with one line per reason.
func TestValidateGraphQLRequest_RefusedDocument_ReportsEveryReason(t *testing.T) {
	reporter := &recordingReporter{name: t.Name()}
	request := graphQLRequest(t, `{"query":"query($id: VulnerabilityID!) { vulnerability(id: $id) { hasSolutions } }","variables":{"id":"gid://gitlab/Vulnerability/1"}}`)

	validateGraphQLRequest(reporter, request)

	if len(reporter.messages) != 1 {
		t.Fatalf("recorded %d message(s), want 1: %v", len(reporter.messages), reporter.messages)
	}
	for _, want := range []string{"GitLab would refuse", "operation: query($id: VulnerabilityID!)", `Cannot query field "hasSolutions"`} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(reporter.joined(), want) {
				t.Errorf("report does not contain %q:\n%s", want, reporter.joined())
			}
		})
	}
}

// TestValidateGraphQLRequest_UndeclaredVariable_IsReported verifies the half a
// static audit cannot see: the document is impeccable and the request sends a
// variable it never declared, which is the defect that let eight domains
// advertise a backward pagination nothing asked GitLab for.
func TestValidateGraphQLRequest_UndeclaredVariable_IsReported(t *testing.T) {
	reporter := &recordingReporter{name: t.Name()}
	request := graphQLRequest(t, `{"query":"query($path: ID!, $first: Int) { project(fullPath: $path) { branchRules(first: $first) { nodes { name } } } }","variables":{"path":"g/p","first":20,"last":20,"before":"cursor"}}`)

	validateGraphQLRequest(reporter, request)

	for _, want := range []string{"variable $before is sent", "variable $last is sent"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(reporter.joined(), want) {
				t.Errorf("report does not contain %q:\n%s", want, reporter.joined())
			}
		})
	}
}

// TestValidateGraphQLRequest_RequestsThatCarryNoDocument_AreLeftAlone verifies
// that the validator judges GraphQL and nothing else. A test posting something
// other than the JSON envelope to this path, or reaching another endpoint
// entirely, is testing something this gate has no opinion about.
func TestValidateGraphQLRequest_RequestsThatCarryNoDocument_AreLeftAlone(t *testing.T) {
	cases := []struct {
		name    string
		request func(*testing.T) *http.Request
	}{
		{
			name: "a GET",
			request: func(t *testing.T) *http.Request {
				t.Helper()
				return httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://gitlab.example.com"+graphQLPath, nil)
			},
		},
		{
			name: "a POST to the REST API",
			request: func(t *testing.T) *http.Request {
				t.Helper()
				return httptest.NewRequestWithContext(t.Context(), http.MethodPost, "http://gitlab.example.com/api/v4/projects", strings.NewReader(`{"name":"x"}`))
			},
		},
		{
			name: "a body that is not the JSON envelope",
			request: func(t *testing.T) *http.Request {
				t.Helper()
				return graphQLRequest(t, "--multipart--")
			},
		},
		{
			name: "an envelope with no query",
			request: func(t *testing.T) *http.Request {
				t.Helper()
				return graphQLRequest(t, `{"variables":{"a":1}}`)
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			reporter := &recordingReporter{name: t.Name()}

			validateGraphQLRequest(reporter, testCase.request(t))

			if len(reporter.messages) != 0 {
				t.Errorf("reported %v, want nothing", reporter.messages)
			}
		})
	}
}

// TestValidateGraphQLRequest_UnreadableBody_ReportsAndRestoresTheBody verifies
// that a body the validator could not read is reported rather than swallowed,
// and that the mock behind it is still handed a body it can read.
func TestValidateGraphQLRequest_UnreadableBody_ReportsAndRestoresTheBody(t *testing.T) {
	reporter := &recordingReporter{name: t.Name()}
	request := graphQLRequest(t, "")
	request.Body = failingBody{}

	validateGraphQLRequest(reporter, request)

	if !strings.Contains(reporter.joined(), "could not read the GraphQL request body") {
		t.Errorf("report = %q, want the unreadable-body message", reporter.joined())
	}
	if _, err := io.ReadAll(request.Body); err != nil {
		t.Errorf("the body handed on to the mock is unreadable: %v", err)
	}
}

// TestValidateGraphQLRequest_AcceptedDocument_ReportsNothing verifies the
// quiet path, which is what almost every GraphQL test in this repository takes.
func TestValidateGraphQLRequest_AcceptedDocument_ReportsNothing(t *testing.T) {
	reporter := &recordingReporter{name: t.Name()}
	request := graphQLRequest(t, `{"query":"query($id: VulnerabilityID!) { vulnerability(id: $id) { id title solution } }","variables":{"id":"gid://gitlab/Vulnerability/1"}}`)

	validateGraphQLRequest(reporter, request)

	if len(reporter.messages) != 0 {
		t.Errorf("reported %v, want nothing", reporter.messages)
	}
}

// TestAllowInvalidGraphQL_ExemptedTest_SendsWhateverItLikes verifies the escape
// hatch and its scope: the exemption covers the test that declared it and the
// subtests under it, and is released when that test ends.
func TestAllowInvalidGraphQL_ExemptedTest_SendsWhateverItLikes(t *testing.T) {
	var inner string
	t.Run("exempted", func(t *testing.T) {
		inner = t.Name()
		AllowInvalidGraphQL(t)

		reporter := &recordingReporter{name: t.Name()}
		validateGraphQLRequest(reporter, graphQLRequest(t, `{"query":"query { nonsense( }"}`))
		if len(reporter.messages) != 0 {
			t.Errorf("an exempted test was still reported: %v", reporter.messages)
		}

		t.Run("subtest", func(t *testing.T) {
			sub := &recordingReporter{name: t.Name()}
			validateGraphQLRequest(sub, graphQLRequest(t, `{"query":"query { nonsense( }"}`))
			if len(sub.messages) != 0 {
				t.Errorf("a subtest of an exempted test was reported: %v", sub.messages)
			}
		})
	})

	if invalidGraphQLAllowed(inner) {
		t.Errorf("the exemption for %s outlived the test that declared it", inner)
	}
}

// TestInvalidGraphQLAllowed_UnrelatedTest_IsNotExempt verifies that the
// prefix walk stops at a name boundary rather than matching any test whose
// name happens to start with the same characters.
func TestInvalidGraphQLAllowed_UnrelatedTest_IsNotExempt(t *testing.T) {
	AllowInvalidGraphQL(t)

	cases := []struct {
		name  string
		probe string
		want  bool
	}{
		{name: "the test itself", probe: t.Name(), want: true},
		{name: "a subtest of it", probe: t.Name() + "/deep/deeper", want: true},
		{name: "a sibling", probe: "TestSomethingElse", want: false},
		{name: "a name that merely shares a prefix", probe: t.Name() + "AndMore", want: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := invalidGraphQLAllowed(testCase.probe); got != testCase.want {
				t.Errorf("invalidGraphQLAllowed(%q) = %v, want %v", testCase.probe, got, testCase.want)
			}
		})
	}
}

// TestReasonLines_NonRefusalError_IsStillReported verifies the branch taken
// when the pinned schema could not be loaded at all: there are no reasons to
// list, and the one line there is must not be dropped.
func TestReasonLines_NonRefusalError_IsStillReported(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{name: "a refusal", err: &graphqlschema.ValidationError{Reasons: []string{"first", "second"}}, want: "  - first\n  - second"},
		{name: "anything else", err: errors.New("the pin is corrupt"), want: "  the pin is corrupt"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := reasonLines(testCase.err); got != testCase.want {
				t.Errorf("reasonLines() = %q, want %q", got, testCase.want)
			}
		})
	}
}

// TestDocumentLabel_NamesTheOperation verifies the label a failure is filed
// under, including the two shapes that carry no usable first line.
func TestDocumentLabel_NamesTheOperation(t *testing.T) {
	cases := []struct {
		name     string
		document string
		want     string
	}{
		{name: "a leading newline is skipped", document: "\n\nquery($id: ID!) {\n  x\n}", want: "query($id: ID!) {"},
		{name: "nothing at all", document: "   \n  \n", want: "(empty document)"},
		{name: "a signature longer than the limit", document: strings.Repeat("q", labelLimit+10), want: strings.Repeat("q", labelLimit) + "..."},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := documentLabel(testCase.document); got != testCase.want {
				t.Errorf("documentLabel() = %q, want %q", got, testCase.want)
			}
		})
	}
}

// TestNewTestClient_GraphQLThroughTheClient_IsValidated verifies that the
// validation really is wired into the client every domain test builds, driving
// a document all the way through client-go's GraphQL transport rather than
// calling the validator directly.
func TestNewTestClient_GraphQLThroughTheClient_IsValidated(t *testing.T) {
	t.Run("a document GitLab accepts passes through", func(t *testing.T) {
		client := NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			RespondGraphQL(w, http.StatusOK, `{"vulnerability":{"id":"gid://gitlab/Vulnerability/1"}}`)
		}))

		var response struct {
			Data map[string]any `json:"data"`
		}
		_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
			Query:     `query($id: VulnerabilityID!) { vulnerability(id: $id) { id } }`,
			Variables: map[string]any{"id": "gid://gitlab/Vulnerability/1"},
		}, &response, gl.WithContext(context.Background()))
		if err != nil {
			t.Fatalf("GraphQL.Do() error = %v", err)
		}
	})

	// The refusal path cannot be asserted from inside a test the refusal would
	// fail, so the exemption is what makes the wiring observable: with it the
	// request goes through untouched, which is the behavior the hatch promises.
	t.Run("an exempted test may send a broken one", func(t *testing.T) {
		AllowInvalidGraphQL(t)
		client := NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			RespondGraphQL(w, http.StatusOK, `{"vulnerability":null}`)
		}))

		var response struct {
			Data map[string]any `json:"data"`
		}
		_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
			Query: `query($id: VulnerabilityID!) { vulnerability(id: $id) { hasSolutions } }`,
		}, &response, gl.WithContext(context.Background()))
		if err != nil {
			t.Fatalf("GraphQL.Do() error = %v", err)
		}
	})
}
