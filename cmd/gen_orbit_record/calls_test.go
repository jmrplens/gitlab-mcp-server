package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"reflect"
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
// recording sends is written in the DSL GitLab.com serves now: a nodes list
// and a bare equality filter on the fixture project's full path.
func TestFixtureQuery_IsTheTraversalOfTheFixtureProject(t *testing.T) {
	want := map[string]any{
		"query_type": "traversal",
		"nodes": []any{map[string]any{
			"id": "p", "entity": "Project",
			"filters": map[string]any{"full_path": "acme/kg-fixtures"},
			"columns": []any{"id", "full_path", "name"},
		}},
	}
	if got := fixtureQuery("acme"); !reflect.DeepEqual(got, want) {
		t.Errorf("fixtureQuery() = %v, want %v", got, want)
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
	if _, err := recordCalls(context.Background(), cfg); err == nil || !strings.Contains(err.Error(), "start the recording proxy: address in use") {
		t.Errorf("recordCalls() error = %v", err)
	}

	listen = previousListen
	newClient = func(string, string) (*gitlabclient.Client, error) { return nil, errors.New("bad base URL") }
	if _, err := recordCalls(context.Background(), cfg); err == nil || !strings.Contains(err.Error(), "build the client the handlers call through: bad base URL") {
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

// TestMakeCall_RefusesACallItCannotRecordFromOneAnswer verifies the four ways
// one call fails to become a record entry: the handler failed, it made no
// Orbit request or more than one, its answer does not decode, and it returned
// a type no audit here could find.
func TestMakeCall_RefusesACallItCannotRecordFromOneAnswer(t *testing.T) {
	answers := orbitAnswers()
	answers["GET /orbit/schema/dsl raw"] = fakeAnswer{status: 200, contentType: "application/json", body: `{"broken"`}
	proxy, client := startProxy(t, &fakeGitLab{answers: answers})
	id := orbitrecord.CallID{Action: "orbit.test", Variant: "raw"}
	status := func(ctx context.Context, c *gitlabclient.Client) (orbit.StatusOutput, error) {
		return orbit.Status(ctx, c, orbit.StatusInput{ResponseFormatInput: formatted("raw")})
	}
	cases := []struct {
		name   string
		invoke func(context.Context, *gitlabclient.Client, string) (any, error)
		want   string
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
		{name: "an answer that does not decode", invoke: func(ctx context.Context, c *gitlabclient.Client, _ string) (any, error) {
			return orbit.DSL(ctx, c, orbit.DSLInput{ResponseFormatInput: formatted("raw")})
		}, want: "orbit.test (raw): GET /orbit/schema/dsl answered 200: the answer is declared application/json and does not decode"},
		{name: "a foreign type", invoke: func(ctx context.Context, c *gitlabclient.Client, _ string) (any, error) {
			if _, err := status(ctx, c); err != nil {
				return nil, err
			}
			return time.Time{}, nil
		}, want: "the handler returned a type declared outside this module: time.Time"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := makeCall(context.Background(), client, proxy, callSpec{id: id, invoke: tc.invoke}, "plens1")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("makeCall() error = %v, want %q", err, tc.want)
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
