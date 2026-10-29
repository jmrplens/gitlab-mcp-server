// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/orbitrecord"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/orbit"
)

// TestCallSpecs_AreTheExpectedCallsInOrder verifies the generator makes
// exactly the calls a whole record holds, in the order the record package
// names them, so the check and the recording cannot drift apart.
func TestCallSpecs_AreTheExpectedCallsInOrder(t *testing.T) {
	var got []orbitrecord.CallID
	for _, spec := range callSpecs() {
		got = append(got, spec.id)
	}
	if !slices.Equal(got, orbitrecord.ExpectedCalls()) {
		t.Errorf("callSpecs() = %v, want %v", got, orbitrecord.ExpectedCalls())
	}
}

// TestFixtureQuery_IsTheTraversalOfTheFixtureProject verifies the query the
// recording sends is written in GQL, the language GitLab.com reads every query
// in since 2026-10-15: the fixture project matched by its full path, three of
// its properties returned. GitLab.com answered this statement 200 against the
// fixture namespace on 2026-10-17.
func TestFixtureQuery_IsTheTraversalOfTheFixtureProject(t *testing.T) {
	want := "MATCH (p:Project {full_path: 'acme/kg-fixtures'}) RETURN p.id, p.full_path, p.name"
	if got := fixtureQuery("acme"); got != want {
		t.Errorf("fixtureQuery() = %q, want %q", got, want)
	}
}

// TestGrammarOf_StampsTheQueryCommandOrNothing verifies what the record names
// the query grammar by: the stamp of the description and input schema of the
// query command orbit.dsl answers with raw, and nothing at all for an answer
// that described no query command, so the record's own check refuses it
// rather than recording a grammar GitLab never described.
func TestGrammarOf_StampsTheQueryCommandOrNothing(t *testing.T) {
	parameters := map[string]any{"type": "object"}
	cases := []struct {
		name string
		dsl  orbit.DSLOutput
		want string
	}{
		{name: "the query command", dsl: orbit.DSLOutput{Name: "query_graph", Description: "MATCH ... RETURN", Parameters: parameters}, want: orbitrecord.GrammarStamp("MATCH ... RETURN", parameters)},
		{name: "no query command", dsl: orbit.DSLOutput{ResponseFormat: "raw"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := grammarOf(tc.dsl); got != tc.want {
				t.Errorf("grammarOf() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestRecordCalls_WithoutAProxyOrAClient_Fails verifies the two things a
// recording needs before it asks anything, a listener and a client, each
// failing the recording with the step named.
func TestRecordCalls_WithoutAProxyOrAClient_Fails(t *testing.T) {
	previousListen, previousClient := listen, newClient
	t.Cleanup(func() { listen, newClient = previousListen, previousClient })
	cfg := testRun(t.TempDir(), &fakeGitLab{answers: orbitAnswers()})

	listen = func(string, string) (net.Listener, error) { return nil, errors.New("address in use") }
	if _, err := recordCalls(context.Background(), cfg, nil); err == nil || !strings.Contains(err.Error(), "start the recording proxy: address in use") {
		t.Errorf("recordCalls() error = %v", err)
	}

	listen = previousListen
	newClient = func(string, string) (*gitlabclient.Client, error) { return nil, errors.New("bad base URL") }
	if _, err := recordCalls(context.Background(), cfg, nil); err == nil || !strings.Contains(err.Error(), "build the client the handlers call through: bad base URL") {
		t.Errorf("recordCalls() error = %v", err)
	}
}

// TestNewClient_PointsAtTheProxyWithoutRetries verifies the handlers' client
// is built against the address it is given, which is the proxy's.
func TestNewClient_PointsAtTheProxyWithoutRetries(t *testing.T) {
	client, err := newClient("http://127.0.0.1:1", "glpat-x")
	if err != nil {
		t.Fatalf("newClient() error: %v", err)
	}
	if got := client.GL().BaseURL().String(); got != "http://127.0.0.1:1/api/v4/" {
		t.Errorf("base URL = %q", got)
	}
}

// startProxy runs a recording proxy the way recordCalls does and returns it
// with a client pointed at it; both stop when the test ends.
func startProxy(t *testing.T, fake *fakeGitLab) (*recorder, *gitlabclient.Client) {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	proxy := newRecorder(orbitrecord.Instance, &http.Client{Transport: fake})
	server := &http.Server{Handler: proxy, ReadHeaderTimeout: time.Second}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	client, err := newClient("http://"+listener.Addr().String(), "glpat-x")
	if err != nil {
		t.Fatal(err)
	}
	return proxy, client
}

// TestMakeCall_RefusesACallItCannotRecordFromOneAnswer verifies the ways one
// call fails to become a record entry: the handler failed, it made no Orbit
// request or more than one, its answer does not decode, it holds nothing at
// the root the call is recorded from, its answer carries a data key no
// verbatim path covers (a tool's input schema recorded without the path that
// keeps it verbatim), and it returned a type no audit here could find.
func TestMakeCall_RefusesACallItCannotRecordFromOneAnswer(t *testing.T) {
	answers := orbitAnswers()
	// The llm answer to a query is the one a handler passes on as it came, so
	// it is the one whose undecodable JSON reaches the recording rather than
	// failing in the handler.
	answers[keyQueryLLM] = fakeAnswer{status: 200, contentType: "application/json", body: `{"broken"`}
	proxy, client := startProxy(t, &fakeGitLab{answers: answers})
	id := orbitrecord.CallID{Action: "orbit.test", Variant: "raw"}
	status := func(ctx context.Context, c *gitlabclient.Client) (orbit.StatusOutput, error) {
		return orbit.Status(ctx, c, orbit.StatusInput{})
	}
	cases := []struct {
		name     string
		root     string
		verbatim []string
		invoke   func(context.Context, *gitlabclient.Client, string) (any, error)
		want     string
	}{
		{name: "the handler failed", invoke: func(context.Context, *gitlabclient.Client, string) (any, error) {
			return nil, errors.New("refused")
		}, want: "orbit.test (raw) failed, so its answer cannot be recorded: refused"},
		{name: "no request", invoke: func(context.Context, *gitlabclient.Client, string) (any, error) {
			return orbit.StatusOutput{}, nil
		}, want: "orbit.test (raw) made 0 Orbit requests"},
		{name: "two requests", invoke: func(ctx context.Context, c *gitlabclient.Client, _ string) (any, error) {
			if _, err := status(ctx, c); err != nil {
				return nil, err
			}
			return status(ctx, c)
		}, want: "orbit.test (raw) made 2 Orbit requests"},
		{name: "an answer that does not decode", invoke: func(ctx context.Context, c *gitlabclient.Client, namespace string) (any, error) {
			return orbit.Query(ctx, c, orbit.QueryInput{Query: fixtureQuery(namespace), ResponseFormat: "llm"})
		}, want: "orbit.test (raw): POST /orbit/query answered 200: the answer is declared application/json and does not decode"},
		{name: "nothing at the root", root: "data.orbit.missing", invoke: func(ctx context.Context, c *gitlabclient.Client, _ string) (any, error) {
			return status(ctx, c)
		}, want: "the answer holds nothing at the root the call is recorded from (data.orbit.missing)"},
		{name: "a data key with no verbatim path", root: rootMCPResult, invoke: func(ctx context.Context, c *gitlabclient.Client, _ string) (any, error) {
			return orbit.Tools(ctx, c, orbit.ToolsInput{})
		}, want: `the answer has the key "$ref" under tools[].inputSchema.properties, which is not a key name`},
		{name: "a foreign type", invoke: func(ctx context.Context, c *gitlabclient.Client, _ string) (any, error) {
			if _, err := status(ctx, c); err != nil {
				return nil, err
			}
			return time.Time{}, nil
		}, want: "the handler returned a type declared outside this module: time.Time"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := makeCall(context.Background(), client, proxy, callSpec{id: id, root: tc.root, verbatim: tc.verbatim, invoke: tc.invoke}, "plens1")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("makeCall() error = %v, want %q", err, tc.want)
			}
		})
	}
}

// TestMakeCall_AnAccountGitLabDeclined_IsARefusal verifies the two failures a
// recording allowed to go without an answer records rather than stops on, and
// that they carry the request the handler sent: a 403, and a GraphQL answer
// with null where the handler reads. Any other failure is not one.
func TestMakeCall_AnAccountGitLabDeclined_IsARefusal(t *testing.T) {
	answers := orbitAnswers()
	answers[keyQueryRaw] = fakeAnswer{status: 403, contentType: "application/json", body: `{"message":"403 Forbidden"}`}
	answers[keyGraphStatus] = fakeAnswer{status: 200, contentType: "application/json", body: `{"data":{"group":{"orbit":null},"project":null}}`}
	answers[keyStatus] = fakeAnswer{status: 500, contentType: "application/json", body: `{"message":"500"}`}
	proxy, client := startProxy(t, &fakeGitLab{answers: answers})
	cases := []struct {
		name    string
		spec    callSpec
		refused bool
		want    string
	}{
		{name: "a 403", spec: callSpecs()[7], refused: true, want: "POST /orbit/query answered 403"},
		{name: "null where the handler reads", spec: callSpecs()[10], refused: true, want: "POST /api/graphql (query OrbitGraphStatusByPath) answered 200 with null"},
		{name: "a 500", spec: callSpecs()[0], want: "orbit.status (default) failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := makeCall(context.Background(), client, proxy, tc.spec, "plens1")
			refused, isRefusal := errors.AsType[*refusal](err)
			if isRefusal != tc.refused || err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("makeCall() error = %v, want refusal %t saying %q", err, tc.refused, tc.want)
			}
			if isRefusal && (refused.request.Request.Path == "" || refused.Unwrap() == nil) {
				t.Fatalf("refusal = %+v, want the request and the handler's error", refused)
			}
		})
	}
}

// TestMakeCall_ATopLevelGroupsAccess_IsARefusalUnlessItServes verifies the
// indexing status of a fixture namespace that is itself a top-level group:
// answered with an access GitLab.com serves no index under, as jmrp-oss was
// at 16:58 UTC on 2026-10-29, it is a refusal naming the access, carrying the
// request, never a key tree with no index in it; answered under a licensed
// access, it is recorded.
func TestMakeCall_ATopLevelGroupsAccess_IsARefusalUnlessItServes(t *testing.T) {
	cases := []struct {
		name    string
		orbit   string
		refused bool
	}{
		{
			name: "trial not eligible, no index",
			orbit: `{"enabled":false,"enabled_at":null,"access":{"state":"TRIAL_NOT_ELIGIBLE","reason":null,"turn_on_denial_reason":"PURCHASE_REQUIRED"},` +
				`"trial":{"state":"NOT_ELIGIBLE","expires_on":null,"credits":{"remaining":0.0,"total":null},"offer":null},"indexing":null,"itemCounts":null}`,
			refused: true,
		},
		{
			name:  "licensed, with its index",
			orbit: `{"access":{"state":"LICENSED","reason":"BETA","turn_on_denial_reason":null},"trial":null,` + strings.TrimPrefix(graphStatusFields, `"enabled":true,"enabled_at":"2026-06-11T23:19:22Z","access":null,"trial":null,`) + `}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			answers := orbitAnswers()
			answers[keyGraphStatus] = fakeAnswer{status: 200, contentType: "application/json", body: `{"data":{"group":{"orbit":` + tc.orbit + `},"project":null}}`}
			proxy, client := startProxy(t, &fakeGitLab{answers: answers})
			call, _, err := makeCall(context.Background(), client, proxy, callSpecs()[10], "jmrp-oss")
			refused, isRefusal := errors.AsType[*refusal](err)
			if isRefusal != tc.refused {
				t.Fatalf("makeCall() error = %v, want refusal %t", err, tc.refused)
			}
			if !tc.refused {
				if err != nil || call.Response.Root != rootGroupOrbit {
					t.Fatalf("makeCall() = %+v, %v, want the call recorded", call, err)
				}
				return
			}
			want := "POST /api/graphql (query OrbitGraphStatusByPath) answered 200 with the top-level group's Orbit access TRIAL_NOT_ELIGIBLE, under which GitLab.com serves no index"
			if !strings.Contains(err.Error(), want) || !errors.Is(err, errAccessDeclined) || refused.request.Request.Path == "" {
				t.Fatalf("refusal = %v (%+v), want %q with the request", err, refused, want)
			}
		})
	}
}

// TestIndexedAnswer_RefusesOnlyTheCountingAnswersOfAnUnindexedNamespace
// verifies which answers stop a recording: a raw traversal with no row and an
// indexing status with no project, or none, counted as indexed. The llm
// answers carry no count to judge, and every other output passes whatever it
// holds.
func TestIndexedAnswer_RefusesOnlyTheCountingAnswersOfAnUnindexedNamespace(t *testing.T) {
	raw := func(action string) orbitrecord.CallID { return orbitrecord.CallID{Action: action, Variant: "raw"} }
	byPath := orbitrecord.CallID{Action: "orbit.graph_status", Variant: "full_path"}
	cases := []struct {
		name    string
		id      orbitrecord.CallID
		output  any
		refused bool
	}{
		{name: "a query with rows", id: raw("orbit.query"), output: orbit.QueryOutput{RowCount: 1}},
		{name: "a query with no row", id: raw("orbit.query"), output: orbit.QueryOutput{}, refused: true},
		{name: "the llm query, which counts nothing", id: orbitrecord.CallID{Action: "orbit.query", Variant: "llm"}, output: orbit.QueryOutput{}},
		{name: "an indexed namespace", id: byPath, output: orbit.GraphStatusOutput{Projects: &orbit.GraphStatusProjects{Indexed: 2}}},
		{name: "no project indexed", id: byPath, output: orbit.GraphStatusOutput{Projects: &orbit.GraphStatusProjects{TotalKnown: 2}}, refused: true},
		{name: "no project count at all", id: byPath, output: orbit.GraphStatusOutput{}, refused: true},
		{name: "another output", id: orbitrecord.CallID{Action: "orbit.status", Variant: "default"}, output: orbit.StatusOutput{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := indexedAnswer(tc.id, tc.output)
			if errors.Is(err, errUnindexed) != tc.refused {
				t.Errorf("indexedAnswer() = %v, want refused %t", err, tc.refused)
			}
		})
	}
}

// TestOutputType_NamesATypeOfThisModuleRepositoryRelative verifies the name a
// call records its output under, which is how the audit finds the type.
func TestOutputType_NamesATypeOfThisModuleRepositoryRelative(t *testing.T) {
	got, err := outputType(orbit.SchemaOutput{})
	if err != nil || got != "internal/tools/orbit.SchemaOutput" {
		t.Errorf("outputType() = %q, %v", got, err)
	}
	if _, err = outputType(time.Time{}); !errors.Is(err, errForeignType) {
		t.Errorf("outputType(time.Time) error = %v, want errForeignType", err)
	}
}
