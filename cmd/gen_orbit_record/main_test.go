// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/orbitrecord"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
)

// recordedOn is the day every fixture recording is stamped with.
var recordedOn = time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)

// fakeAnswer is what the fake GitLab.com answers one request with.
type fakeAnswer struct {
	status      int
	contentType string
	body        string
}

// fakeGrammar is the description the fake query command answers with, and
// fakeGrammarStamp what a recording names it by: the stamp of that
// description beside the input schema the fake answers raw.
const fakeGrammar = "Read-only openCypher 9-based syntax (MATCH ... RETURN)."

// fakeGrammarStamp is the stamp the fake raw answer to orbit.dsl records.
func fakeGrammarStamp() string {
	return orbitrecord.GrammarStamp(fakeGrammar, map[string]any{
		"type":       "object",
		"required":   []any{"query"},
		"properties": map[string]any{"query": map[string]any{"type": "string"}},
		"$defs":      map[string]any{},
	})
}

// agentCommands is the fake raw answer to orbit.dsl, the query command with
// the given description.
func agentCommands(description string) string {
	return `[{"name":"query_graph","description":"` + description + `","parameters":{"type":"object","required":["query"],"properties":{"query":{"type":"string"}},"$defs":{}}}]`
}

// The keys of the fake's answers for the sources the handlers read since
// GitLab.com removed five of the six Orbit REST routes: a GraphQL document by
// its operation name, and an MCP request by its method, its tool and the
// format it asked for.
const (
	keyStatus         = "POST /api/graphql OrbitServiceStatus"
	keySchema         = "POST /api/graphql OrbitSchema"
	keyGraphStatus    = "POST /api/graphql OrbitGraphStatusByPath"
	keyTools          = "POST /orbit/mcp tools/list"
	keySchemaText     = "POST /orbit/mcp tools/call invoke_command llm"
	keyCommandsRaw    = "POST /orbit/mcp tools/call list_commands raw"
	keyCommandsLLM    = "POST /orbit/mcp tools/call list_commands llm"
	keyQueryRaw       = "POST /orbit/query raw"
	keyQueryLLM       = "POST /orbit/query llm"
	keyQueryGQL       = "POST /orbit/query gql"
	graphStatusFields = `"enabled":true,"enabled_at":"2026-06-11T23:19:22Z","access":null,"trial":null,"indexing":{"state":"READY","last_completed_at":"t","projects":{"indexed":2,"total_known":2,"gaps":0,"not_indexed":0,"total":2},"areas":[{"name":"core","state":"READY"}]},"itemCounts":{"domains":[{"name":"core","items":[{"name":"Project","count":"2"}]}]}`
)

// toolTextAnswer is an MCP tools/call answer carrying text. A string always
// encodes, which this repository says at the leaf.
func toolTextAnswer(text string) string {
	return `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":` + string(cmdutil.Must(json.Marshal(text))) + `}],"isError":false}}`
}

// orbitAnswers are the answers of a healthy GitLab.com, keyed by source,
// shaped like the ones GitLab.com gave on 2026-10-29 and carrying keys a
// verbatim path must keep out of the record ($defs, a JSON Schema's own
// property names) so a test sees the rule work: without the verbatim path the
// recording would refuse the key.
func orbitAnswers() map[string]fakeAnswer {
	ok := func(body string) fakeAnswer {
		return fakeAnswer{status: 200, contentType: "application/json", body: body}
	}
	return map[string]fakeAnswer{
		keyStatus: ok(`{"data":{"orbit":{"serviceStatus":{"status":"HEALTHY","version":"0.130.0","components":[{"name":"nats","status":"HEALTHY"}]}}}}`),
		keySchema: ok(`{"data":{"orbit":{"schema":{` +
			`"nodes":{"pageInfo":{"hasNextPage":false,"endCursor":"MQ"},"nodes":[{"name":"User","description":"d","primary_key":"id","label_field":"username","domain":{"name":"core"},"display":{"color":"#000","size":1},` +
			`"properties":{"pageInfo":{"hasNextPage":false},"nodes":[{"name":"id","data_type":"Int","nullable":false,"enum_values":[]}]}}]},` +
			`"edges":{"pageInfo":{"hasNextPage":false,"endCursor":"MQ"},"nodes":[{"name":"AUTHORED","description":"d","variants":{"pageInfo":{"hasNextPage":false},"nodes":[{"source":{"name":"User"},"target":{"name":"Issue"}}]}}]}}}}}`),
		keySchemaText:  ok(toolTextAnswer("domains[1]:\n  - name: core")),
		keyTools:       ok(`{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"invoke_command","description":"d","inputSchema":{"type":"object","properties":{"$ref":{}}},"trusted":true}]}}`),
		keyCommandsRaw: ok(toolTextAnswer(agentCommands(fakeGrammar))),
		keyCommandsLLM: ok(toolTextAnswer("commands[1]: query_graph, MATCH ... RETURN")),
		keyQueryRaw:    ok(`{"result":{"format_version":"5.0.3","nodes":[{"id":"1","full_path":"plens1/kg-fixtures"}],"edges":[]},"query_type":"traversal","row_count":1}`),
		keyQueryLLM:    {status: 200, contentType: "text/plain; charset=utf-8", body: "@header\nquery_type:traversal\n"},
		keyQueryGQL:    {status: 200, contentType: "text/plain; charset=utf-8", body: "+---+\n| p |\n+---+\n\n1 row\n"},
		keyGraphStatus: ok(`{"data":{"group":{"orbit":{` + graphStatusFields + `}},"project":null}}`),
	}
}

// fakeRequest is what the fake reads of a request body to key its answer.
type fakeRequest struct {
	ResponseFormat string `json:"response_format"`
	Query          string `json:"query"`
	Method         string `json:"method"`
	Params         struct {
		Name      string `json:"name"`
		Arguments struct {
			Format     string `json:"format"`
			Parameters struct {
				Format string `json:"format"`
			} `json:"parameters"`
		} `json:"arguments"`
	} `json:"params"`
}

// fakeKey names the answer one request gets: the method and the route below
// /api/v4, then the GraphQL operation, or the MCP method, tool and format, or
// the REST response format.
func fakeKey(req *http.Request, body []byte) string {
	route := strings.TrimPrefix(req.URL.Path, "/api/v4")
	var fields fakeRequest
	_ = json.Unmarshal(body, &fields)
	if match := graphQLOperationPattern.FindStringSubmatch(fields.Query); match != nil {
		return req.Method + " " + route + " " + match[2]
	}
	if fields.Method != "" {
		key := req.Method + " " + route + " " + fields.Method
		if fields.Params.Name != "" {
			key += " " + fields.Params.Name
		}
		if format := firstNonEmpty(fields.Params.Arguments.Format, fields.Params.Arguments.Parameters.Format); format != "" {
			key += " " + format
		}
		return key
	}
	return req.Method + " " + route + " " + firstNonEmpty(fields.ResponseFormat, req.URL.Query().Get("response_format"))
}

// fakeGitLab answers for GitLab.com in-process: it is the transport the
// recording proxy's client uses, so a test records without a network. It
// keeps what it was asked, under a lock because the proxy calls it from the
// server's goroutines, and a test reads that only after the run returns.
type fakeGitLab struct {
	answers map[string]fakeAnswer

	mu    sync.Mutex
	asked []string
	token []string
}

// RoundTrip answers one forwarded request. A route it has no answer for is
// answered 404, which the handler reports as its own failure.
func (f *fakeGitLab) RoundTrip(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
	}
	key := fakeKey(req, body)
	f.mu.Lock()
	f.asked = append(f.asked, req.URL.Scheme+"://"+req.URL.Host+" "+key+" "+req.URL.RawQuery+" "+string(body))
	f.token = append(f.token, req.Header.Get("Private-Token"))
	f.mu.Unlock()
	answer, found := f.answers[key]
	if !found {
		answer = fakeAnswer{status: 404, contentType: "application/json", body: `{"message":"404 Not Found"}`}
	}
	header := http.Header{}
	header.Set("Content-Type", answer.contentType)
	return &http.Response{
		StatusCode: answer.status,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(answer.body)),
		Request:    req,
	}, nil
}

// snapshot copies what the fake was asked, under its lock: the proxy served
// those requests on its own goroutines, and a socket between them is no
// happens-before edge the race detector can see.
func (f *fakeGitLab) snapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.asked)
}

// tokens copies the tokens the fake was sent, under its lock.
func (f *fakeGitLab) tokens() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.token)
}

// testRun is a run that records into dir against the fake GitLab.com, in a
// repository whose HEAD holds no record yet.
func testRun(dir string, fake *fakeGitLab) genRun {
	return genRun{
		dir:       dir,
		token:     "glpat-test",
		namespace: "plens1",
		upstream:  orbitrecord.Instance,
		client:    &http.Client{Transport: fake},
		now:       func() time.Time { return recordedOn },
		timeout:   time.Minute,
		git:       fakeHead{dir: dir + "-never-committed"}.git,
	}
}

// fakeHead stands for the repository's HEAD: it holds what a test committed
// into its directory, and a record nobody committed is not there, which git
// answers the way `git show` does.
type fakeHead struct{ dir string }

// git answers the one git command a recording runs.
func (h fakeHead) git(_ context.Context, _ string, args ...string) ([]byte, error) {
	if !slices.Equal(args, []string{"show", committedRevision()}) {
		return nil, fmt.Errorf("unexpected git %q", args)
	}
	raw, err := os.ReadFile(orbitrecord.Path(h.dir))
	if err != nil {
		return nil, fmt.Errorf("git show %s: fatal: path does not exist in 'HEAD'", committedRevision())
	}
	return raw, nil
}

// commit makes HEAD hold the record written under from.
func (h fakeHead) commit(t *testing.T, from string) {
	t.Helper()
	raw, err := os.ReadFile(orbitrecord.Path(from))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(h.dir, 0o750); err != nil {
		t.Fatal(err)
	}
	//#nosec G703 -- both halves of the path are this test's own: a t.TempDir and the record's file name
	if err = os.WriteFile(orbitrecord.Path(h.dir), raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

// hangingGitLab is a GitLab.com that never answers: it holds each request
// until the request's own context ends.
type hangingGitLab struct{}

// RoundTrip waits for the forwarded request to be abandoned.
func (hangingGitLab) RoundTrip(req *http.Request) (*http.Response, error) {
	<-req.Context().Done()
	return nil, req.Context().Err()
}

// runCapture runs cfg and returns its status and both streams.
func runCapture(cfg genRun) (status int, out, errOut string) {
	var stdout, stderr bytes.Buffer
	status = run(cfg, &stdout, &stderr)
	return status, stdout.String(), stderr.String()
}

// TestRecord_AgainstAHealthyGitLab_WritesARecordTheCheckAccepts verifies the
// whole recording: every expected call is made through its handler, through
// the proxy, to GitLab.com with the token; the record is stamped with the
// version the status call reported and written in canonical form; the offline
// check then accepts it; and once it is committed, a second recording of the
// same answers says the key tree is the committed one and succeeds.
func TestRecord_AgainstAHealthyGitLab_WritesARecordTheCheckAccepts(t *testing.T) {
	dir := t.TempDir()
	head := fakeHead{dir: t.TempDir()}
	fake := &fakeGitLab{answers: orbitAnswers()}
	cfg := testRun(dir, fake)
	cfg.git = head.git

	status, out, errOut := runCapture(cfg)
	if status != 0 {
		t.Fatalf("record status = %d, stderr:\n%s", status, errOut)
	}
	if !strings.Contains(out, "wrote "+orbitrecord.Path(dir)+": 11 calls, Orbit 0.130.0, query grammar "+fakeGrammarStamp()) || !strings.Contains(out, "nothing to compare this one with") {
		t.Errorf("stdout = %q", out)
	}
	if asked := fake.snapshot(); len(asked) != 11 || !strings.HasPrefix(asked[0], "https://gitlab.com "+keyStatus) {
		t.Errorf("asked = %q, want the eleven calls against gitlab.com", asked)
	}
	for _, token := range fake.tokens() {
		if token != "glpat-test" {
			t.Errorf("forwarded token = %q, want the run's", token)
		}
	}

	doc, err := orbitrecord.Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := orbitrecord.Source{Instance: orbitrecord.Instance, OrbitVersion: "0.130.0", QueryGrammar: fakeGrammarStamp(), Namespace: "plens1", RetrievedAt: "2026-09-27"}
	if doc.Source != want {
		t.Errorf("source = %+v, want %+v", doc.Source, want)
	}
	checkStatus, checkOut, checkErr := runCapture(genRun{dir: dir, check: true, now: func() time.Time { return recordedOn }})
	if checkStatus != 0 || !strings.Contains(checkOut, "11 calls recorded from https://gitlab.com, Orbit 0.130.0, query grammar "+fakeGrammarStamp()+", namespace plens1, on 2026-09-27") {
		t.Errorf("check = %d, %q, %q", checkStatus, checkOut, checkErr)
	}

	head.commit(t, dir)
	cfg.client = &http.Client{Transport: &fakeGitLab{answers: orbitAnswers()}}
	again, againOut, againErr := runCapture(cfg)
	if again != 0 || !strings.Contains(againOut, "the key tree and the query grammar are the ones committed at HEAD") {
		t.Errorf("second record = %d, %q, %q", again, againOut, againErr)
	}
}

// TestRecord_WhatTheRecordHolds_IsTheShapeAndNeverAValue verifies the calls
// the record keeps: each names the output type its handler returned, the
// parameter names the handler sent (the MCP tool and its arguments, the
// GraphQL operation and its variables, the query's body), the media type and
// the root its key tree is taken from, and keys with kinds only, with the
// data-keyed subtrees kept as their roots and GraphQL connections recorded as
// the lists they page.
func TestRecord_WhatTheRecordHolds_IsTheShapeAndNeverAValue(t *testing.T) {
	dir := t.TempDir()
	if status, _, errOut := runCapture(testRun(dir, &fakeGitLab{answers: orbitAnswers()})); status != 0 {
		t.Fatalf("record failed: %s", errOut)
	}
	raw, err := os.ReadFile(orbitrecord.Path(dir))
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"HEALTHY", "nats", "username", "#000", "plens1/kg-fixtures", "AUTHORED", "$defs", "$ref", "query_graph", "openCypher", "MATCH", "@header", "pageInfo", "hasNextPage"} {
		t.Run("no "+value, func(t *testing.T) {
			if bytes.Contains(raw, []byte(value)) {
				t.Errorf("the record holds %q, a value or a data key", value)
			}
		})
	}
	calls := recordedCalls(t, dir)
	verbatim := map[string]string{
		"orbit.tools (default)": "tools[].inputSchema",
		"orbit.dsl (raw)":       "[].parameters",
		"orbit.query (raw)":     "result.nodes[]",
	}
	for id, path := range verbatim {
		t.Run(id, func(t *testing.T) {
			assertKeptVerbatim(t, calls[id], path)
		})
	}
}

// recordedCalls reads the record under dir, its calls by identity.
func recordedCalls(t *testing.T, dir string) map[string]orbitrecord.Call {
	t.Helper()
	doc, err := orbitrecord.Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	calls := map[string]orbitrecord.Call{}
	for _, call := range doc.Calls {
		calls[call.ID().String()] = call
	}
	return calls
}

// TestRecord_EachKindOfCall_NamesItsRequestAndRoot verifies what the record
// says of each kind of call: an MCP tool call by its method, tool and argument
// names, its text recorded at the tool result's text; a GraphQL document by its
// operation and variables, its keys taken below data; and a REST route by its
// body names alone, recorded whole.
func TestRecord_EachKindOfCall_NamesItsRequestAndRoot(t *testing.T) {
	dir := t.TempDir()
	if status, _, errOut := runCapture(testRun(dir, &fakeGitLab{answers: orbitAnswers()})); status != 0 {
		t.Fatalf("record failed: %s", errOut)
	}
	calls := recordedCalls(t, dir)
	cases := []struct {
		id      string
		request orbitrecord.Request
		root    string
		kind    string
		hasKey  string
	}{
		{
			id:      "orbit.schema (llm)",
			request: orbitrecord.Request{Method: http.MethodPost, Path: "/orbit/mcp", Body: []string{"id", "jsonrpc", "method", "params"}, Operation: "tools/call invoke_command", Arguments: []string{"command_name", "parameters"}},
			root:    rootMCPText, kind: mediaText, hasKey: orbitrecord.Root,
		},
		{
			id:      "orbit.schema (expand)",
			request: orbitrecord.Request{Method: http.MethodPost, Path: orbitrecord.GraphQLPath, Body: []string{"query", "variables"}, Operation: "query OrbitSchema", Arguments: []string{"withProperties"}},
			root:    rootSchema, kind: mediaJSON, hasKey: "nodes[].properties[].data_type",
		},
		{
			id:      "orbit.status (default)",
			request: orbitrecord.Request{Method: http.MethodPost, Path: orbitrecord.GraphQLPath, Body: []string{"query"}, Operation: "query OrbitServiceStatus"},
			root:    rootServiceStatus, kind: mediaJSON, hasKey: "components[].status",
		},
		{
			id:      "orbit.query (llm)",
			request: orbitrecord.Request{Method: http.MethodPost, Path: "/orbit/query", Body: []string{"query", "response_format"}},
			hasKey:  orbitrecord.Root,
		},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			call := calls[tc.id]
			if !call.SameRequest(orbitrecord.Call{Request: tc.request}) || call.Request.Method != tc.request.Method {
				t.Errorf("request = %+v, want %+v", call.Request, tc.request)
			}
			if call.Response.Root != tc.root || call.Response.RootContentType != tc.kind {
				t.Errorf("root = %q (%q), want %q (%q)", call.Response.Root, call.Response.RootContentType, tc.root, tc.kind)
			}
			if !slices.ContainsFunc(call.Response.Keys, func(key orbitrecord.Key) bool { return key.Path == tc.hasKey }) {
				t.Errorf("keys = %+v, want %q among them", call.Response.Keys, tc.hasKey)
			}
		})
	}
}

// assertKeptVerbatim fails unless call records path as a verbatim root and
// nothing below it.
func assertKeptVerbatim(t *testing.T, call orbitrecord.Call, path string) {
	t.Helper()
	found := false
	for _, key := range call.Response.Keys {
		if key.Path == path {
			found = key.Verbatim
		}
		if strings.HasPrefix(key.Path, path+".") {
			t.Errorf("%s recorded %q below its verbatim %q", call.ID(), key.Path, path)
		}
	}
	if !found {
		t.Errorf("%s did not keep %q verbatim", call.ID(), path)
	}
}

// TestRecord_AChangedAnswer_FailsEveryRunUntilItIsCommitted verifies what
// happens when GitLab.com adds and drops a key: the new record is written,
// every change is printed, and the run fails. A second run of the same
// answers fails the same way, because it compares with what HEAD holds and
// not with the file the first run wrote; once the record is committed, the
// next run passes.
func TestRecord_AChangedAnswer_FailsEveryRunUntilItIsCommitted(t *testing.T) {
	dir := t.TempDir()
	head := fakeHead{dir: t.TempDir()}
	first := testRun(dir, &fakeGitLab{answers: orbitAnswers()})
	first.git = head.git
	if status, _, errOut := runCapture(first); status != 0 {
		t.Fatalf("first record failed: %s", errOut)
	}
	head.commit(t, dir)
	changed := orbitAnswers()
	changed[keyGraphStatus] = fakeAnswer{status: 200, contentType: "application/json", body: `{"data":{"group":{"orbit":{"region":"eu",` + graphStatusFields + `}},"project":null}}`}
	changed[keyStatus] = fakeAnswer{status: 200, contentType: "application/json", body: `{"data":{"orbit":{"serviceStatus":{"status":"HEALTHY","version":"0.130.0","components":[{"status":"HEALTHY"}]}}}}`}
	const laterGrammar = "Read-only openCypher 9-based syntax (MATCH ... RETURN), now with OPTIONAL MATCH."
	changed[keyCommandsRaw] = fakeAnswer{status: 200, contentType: "application/json", body: toolTextAnswer(agentCommands(laterGrammar))}
	laterStamp := orbitrecord.GrammarStamp(laterGrammar, map[string]any{
		"type":       "object",
		"required":   []any{"query"},
		"properties": map[string]any{"query": map[string]any{"type": "string"}},
		"$defs":      map[string]any{},
	})
	rerun := func() (int, string) {
		cfg := testRun(dir, &fakeGitLab{answers: changed})
		cfg.git = head.git
		status, _, errOut := runCapture(cfg)
		return status, errOut
	}

	for _, run := range []string{"the run that notices the change", "a run before it is committed"} {
		t.Run(run, func(t *testing.T) {
			status, errOut := rerun()
			if status != 1 {
				t.Fatalf("status = %d, want 1; stderr:\n%s", status, errOut)
			}
			for _, line := range []string{
				"+ orbit.graph_status (full_path): region string",
				"- orbit.status (default): components[].name string",
				"~ query grammar: " + fakeGrammarStamp() + " -> " + laterStamp + ": read orbit.query's guidance against it",
				"the key tree or the query grammar differs from the record committed at HEAD in 3 places",
			} {
				if !strings.Contains(errOut, line) {
					t.Errorf("stderr lacks %q:\n%s", line, errOut)
				}
			}
		})
	}
	doc, _ := orbitrecord.Read(dir)
	if !strings.Contains(string(orbitrecord.Encode(doc)), `"region"`) {
		t.Error("the changed record was not written")
	}

	head.commit(t, dir)
	if status, errOut := rerun(); status != 0 {
		t.Errorf("status after the commit = %d, want 0; stderr:\n%s", status, errOut)
	}
}

// TestRecord_RefusesWhatItCannotRecordWhole verifies the recording's refusals:
// no token, a handler GitLab refuses, a fixture namespace the indexer has not
// reached, and a recording the check would refuse, none of which may replace
// the record already there.
func TestRecord_RefusesWhatItCannotRecordWhole(t *testing.T) {
	refused := orbitAnswers()
	delete(refused, keySchemaText)
	versionless := orbitAnswers()
	versionless[keyStatus] = fakeAnswer{status: 200, contentType: "application/json", body: `{"data":{"orbit":{"serviceStatus":{"status":"UNKNOWN","version":null,"components":[]}}}}`}
	noRows := orbitAnswers()
	noRows[keyQueryRaw] = fakeAnswer{status: 200, contentType: "application/json", body: `{"result":{"format_version":"5.0.3","nodes":[],"edges":[]},"query_type":"traversal","row_count":0}`}
	unindexed := orbitAnswers()
	unindexed[keyGraphStatus] = fakeAnswer{status: 200, contentType: "application/json", body: `{"data":{"group":{"orbit":{"enabled":true,"indexing":{"state":"INDEXING","projects":{"indexed":0,"total_known":2}},"itemCounts":null}},"project":null}}`}
	unstamped := orbitAnswers()
	unstamped[keyCommandsRaw] = fakeAnswer{status: 200, contentType: "application/json", body: toolTextAnswer(`[{"name":"get_graph_schema","description":"d","parameters":{"type":"object"}}]`)}
	forbidden := orbitAnswers()
	forbidden[keyQueryRaw] = fakeAnswer{status: 403, contentType: "application/json", body: `{"message":"403 Forbidden - No Orbit enabled namespaces available"}`}
	cases := []struct {
		name  string
		token string
		fake  *fakeGitLab
		want  string
	}{
		{name: "no token", fake: &fakeGitLab{answers: orbitAnswers()}, want: "GITLAB_COM_TOKEN is not set"},
		{name: "a refused call", token: "glpat-test", fake: &fakeGitLab{answers: refused}, want: "orbit.schema (llm) failed, so its answer cannot be recorded"},
		{name: "a query that finds no row", token: "glpat-test", fake: &fakeGitLab{answers: noRows}, want: "orbit.query (raw) found no row for the fixture project: the fixture namespace is not indexed"},
		{name: "a namespace with no indexed project", token: "glpat-test", fake: &fakeGitLab{answers: unindexed}, want: "orbit.graph_status (full_path) counts no indexed project"},
		{name: "a record the check refuses", token: "glpat-test", fake: &fakeGitLab{answers: versionless}, want: "names no Orbit version"},
		{name: "no query command described", token: "glpat-test", fake: &fakeGitLab{answers: unstamped}, want: "names no query grammar"},
		{name: "a refused account, not allowed", token: "glpat-test", fake: &fakeGitLab{answers: forbidden}, want: "orbit.query (raw) was not answered for this account (POST /orbit/query answered 403"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			cfg := testRun(dir, tc.fake)
			cfg.token = tc.token
			status, _, errOut := runCapture(cfg)
			if status != 1 || !strings.Contains(errOut, tc.want) {
				t.Errorf("status = %d, stderr = %q, want 1 and %q", status, errOut, tc.want)
			}
			if _, err := os.Stat(orbitrecord.Path(dir)); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("a refused recording wrote the record: %v", err)
			}
		})
	}
}

// TestRecord_AllowUnanswered_CarriesOrListsWhatGitLabRefused verifies a
// recording made while GitLab refuses the account, as it did from 09:22 UTC on
// 2026-10-29: a call refused 403 whose request the record at HEAD holds an
// answer to is carried from it, dated by that record; a call answered with
// null where the handler reads, which no earlier record answered, is listed as
// unrecorded with GitLab's answer and the time; the record written passes the
// check; and each is said on standard error.
func TestRecord_AllowUnanswered_CarriesOrListsWhatGitLabRefused(t *testing.T) {
	dir := t.TempDir()
	head := fakeHead{dir: t.TempDir()}
	first := testRun(dir, &fakeGitLab{answers: orbitAnswers()})
	first.git = head.git
	first.now = func() time.Time { return recordedOn.AddDate(0, 0, -3) }
	if status, _, errOut := runCapture(first); status != 0 {
		t.Fatalf("first record failed: %s", errOut)
	}
	head.commit(t, dir)

	refused := orbitAnswers()
	for _, key := range []string{keyQueryRaw, keyQueryLLM, keyQueryGQL} {
		refused[key] = fakeAnswer{status: 403, contentType: "application/json", body: `{"message":"403 Forbidden - No Orbit enabled namespaces available"}`}
	}
	refused[keyGraphStatus] = fakeAnswer{status: 200, contentType: "application/json", body: `{"data":{"group":{"orbit":{"enabled":true,"enabled_at":null,"access":null,"trial":null,"indexing":null,"itemCounts":null}},"project":null}}`}
	cfg := testRun(dir, &fakeGitLab{answers: refused})
	cfg.git = head.git
	cfg.allowUnanswered = true

	// Every refused request is one HEAD's record answered, so each is carried
	// and the key tree is HEAD's.
	status, _, errOut := runCapture(cfg)
	if status != 0 {
		t.Fatalf("status = %d, want 0; stderr:\n%s", status, errOut)
	}
	for _, line := range []string{
		"orbit.query (raw) was refused today and is carried from the recording of 2026-09-24, which sent the same request",
		"orbit.graph_status (full_path) was refused today and is carried from the recording of 2026-09-24",
	} {
		t.Run(line, func(t *testing.T) {
			if !strings.Contains(errOut, line) {
				t.Errorf("stderr lacks %q:\n%s", line, errOut)
			}
		})
	}
	doc, err := orbitrecord.Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Calls) != 11 || len(doc.Unrecorded) != 0 || doc.Source.RetrievedAt != "2026-09-27" {
		t.Errorf("record = %d calls, unrecorded %+v, source %+v", len(doc.Calls), doc.Unrecorded, doc.Source)
	}
	if problems := orbitrecord.Problems(doc, recordedOn); len(problems) != 0 {
		t.Errorf("Problems() = %q, want none", problems)
	}

	// With no record at HEAD there is nothing to carry, so each refused call
	// is listed as unrecorded, with GitLab's answer and the time.
	cfg.git = fakeHead{dir: t.TempDir()}.git
	cfg.dir = t.TempDir()
	if status, _, errOut = runCapture(cfg); status != 0 {
		t.Fatalf("status with nothing at HEAD = %d; stderr:\n%s", status, errOut)
	}
	if !strings.Contains(errOut, "orbit.graph_status (full_path) is not recorded: POST /api/graphql (query OrbitGraphStatusByPath) answered 200 with null where the handler reads") {
		t.Errorf("stderr lacks the unrecorded graph status:\n%s", errOut)
	}
	if doc, err = orbitrecord.Read(cfg.dir); err != nil || len(doc.Unrecorded) != 4 ||
		!strings.HasSuffix(doc.Unrecorded[0].Reason, "at 2026-09-27T10:00:00Z, and no earlier recording holds an answer to this request") {
		t.Errorf("record with nothing at HEAD = %+v, %v, want four unrecorded calls", doc.Unrecorded, err)
	}
}

// TestCarryOrSkip_CarriesOnlyTheSameRequest verifies what an earlier record
// can give a refused call: its answer when it sent the same request, keeping
// the day it was carried from if it was itself carried, and nothing when the
// request differs, which lists the call as unrecorded.
func TestCarryOrSkip_CarriesOnlyTheSameRequest(t *testing.T) {
	id := orbitrecord.CallID{Action: orbitrecord.QueryAction, Variant: "raw"}
	request := orbitrecord.Call{Action: id.Action, Variant: id.Variant, Request: orbitrecord.Request{Method: "POST", Path: "/orbit/query", Body: []string{"query"}}}
	refused := &refusal{id: id, request: request, reason: "POST /orbit/query answered 403"}
	carried := request
	carried.RetrievedAt = "2026-09-01"
	other := request
	other.Request.Path = "/orbit/query/name"
	previous := &orbitrecord.Document{Source: orbitrecord.Source{RetrievedAt: "2026-09-20"}, Calls: []orbitrecord.Call{other, carried}}

	var doc orbitrecord.Document
	carryOrSkip(&doc, previous, refused, recordedOn)
	if len(doc.Calls) != 1 || doc.Calls[0].RetrievedAt != "2026-09-01" {
		t.Errorf("carried = %+v, want the earlier answer with the day it was first recorded", doc.Calls)
	}

	doc = orbitrecord.Document{}
	carryOrSkip(&doc, &orbitrecord.Document{Calls: []orbitrecord.Call{other}}, refused, recordedOn)
	if len(doc.Calls) != 0 || len(doc.Unrecorded) != 1 || doc.Unrecorded[0].ID() != id {
		t.Errorf("record = %+v, want the call unrecorded since no earlier one sent its request", doc)
	}
}

// TestRecord_ADeadlineThatPasses_FailsAndWritesNothing verifies the run's
// timeout: a GitLab.com that does not answer ends the recording at the
// deadline, as a failure, with no record written.
func TestRecord_ADeadlineThatPasses_FailsAndWritesNothing(t *testing.T) {
	dir := t.TempDir()
	cfg := testRun(dir, nil)
	cfg.client = &http.Client{Transport: hangingGitLab{}}
	cfg.timeout = 50 * time.Millisecond
	started := time.Now()
	status, _, errOut := runCapture(cfg)
	if status != 1 || !strings.Contains(errOut, "context deadline exceeded") {
		t.Errorf("status = %d, stderr = %q", status, errOut)
	}
	if elapsed := time.Since(started); elapsed > 30*time.Second {
		t.Errorf("the run took %s, want it ended at its deadline", elapsed)
	}
	if _, err := os.Stat(orbitrecord.Path(dir)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("stat record = %v, want nothing written", err)
	}
}

// TestRecord_AnUnwritableDirectory_Fails verifies that a record the recording
// cannot write is a failure rather than a success with nothing on disk.
func TestRecord_AnUnwritableDirectory_Fails(t *testing.T) {
	blocker := t.TempDir() + "/file"
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	status, _, errOut := runCapture(testRun(blocker, &fakeGitLab{answers: orbitAnswers()}))
	if status != 1 || !strings.Contains(errOut, "create directory for") {
		t.Errorf("status = %d, stderr = %q", status, errOut)
	}
}

// TestCheckRecord_ARecordPinningTheJSONQueryDSL_IsRefused verifies the gate
// on the case issue 1414 is about, a record that still pins the JSON query
// DSL GitLab.com stopped reading on 2026-10-15, in both shapes such a record
// can take: one of schema version 2, which is every record a recording wrote
// before the change, is refused by its version with the command that
// re-records it, and one of version 3 that carries the JSON DSL's $id and
// version beside its grammar is refused as not the form the generator
// writes, since version 3 has no place for either key.
func TestCheckRecord_ARecordPinningTheJSONQueryDSL_IsRefused(t *testing.T) {
	const pins = `"dsl_schema": "https://gitlab.com/gitlab-org/orbit/knowledge-graph/schemas/graph_query/v12",` + "\n    " + `"dsl_version": "12.1.10",` + "\n    "
	cases := []struct {
		name string
		edit func(record []byte) []byte
		want string
	}{
		{
			name: "a record of schema version 2",
			edit: func(record []byte) []byte {
				return bytes.Replace(record, []byte(`"schema_version": 4`), []byte(`"schema_version": 2`), 1)
			},
			want: "the Orbit response record is schema version 2 and this build reads version 4: regenerate it with make gen-orbit-record",
		},
		{
			name: "a record of version 4 that still names the JSON DSL",
			edit: func(record []byte) []byte {
				return bytes.Replace(record, []byte(`"namespace": `), []byte(pins+`"namespace": `), 1)
			},
			want: "the record is not in the form this command writes",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if status, _, errOut := runCapture(testRun(dir, &fakeGitLab{answers: orbitAnswers()})); status != 0 {
				t.Fatalf("record failed: %s", errOut)
			}
			record, err := os.ReadFile(orbitrecord.Path(dir))
			if err != nil {
				t.Fatal(err)
			}
			edited := tc.edit(record)
			if bytes.Equal(edited, record) {
				t.Fatalf("the edit changed nothing in %s", record)
			}
			if err = os.WriteFile(orbitrecord.Path(dir), edited, 0o600); err != nil {
				t.Fatal(err)
			}

			status, _, errOut := runCapture(genRun{dir: dir, check: true, now: func() time.Time { return recordedOn }})
			if status != 1 || !strings.Contains(errOut, tc.want) {
				t.Errorf("status = %d, stderr = %q, want 1 and %q", status, errOut, tc.want)
			}
		})
	}
}

// TestCheckRecord_RefusesWhatIsNotAWholeCanonicalRecord verifies the gate: a
// missing record, one the problems check refuses, and one in another form than
// the generator writes each fail with the command that fixes them.
func TestCheckRecord_RefusesWhatIsNotAWholeCanonicalRecord(t *testing.T) {
	dir := t.TempDir()
	if status, _, errOut := runCapture(testRun(dir, &fakeGitLab{answers: orbitAnswers()})); status != 0 {
		t.Fatalf("record failed: %s", errOut)
	}
	whole, err := os.ReadFile(orbitrecord.Path(dir))
	if err != nil {
		t.Fatal(err)
	}
	var doc orbitrecord.Document
	if err = json.Unmarshal(whole, &doc); err != nil {
		t.Fatal(err)
	}
	compact, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	doc.Source.Instance = "https://gitlab.example.com"
	foreign := orbitrecord.Encode(doc)

	cases := []struct {
		name    string
		content []byte
		want    string
	}{
		{name: "missing", want: "reading the Orbit response record"},
		{name: "refused", content: foreign, want: "not https://gitlab.com"},
		{name: "not canonical", content: compact, want: "not in the form this command writes"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caseDir := t.TempDir()
			if tc.content != nil {
				if writeErr := os.WriteFile(orbitrecord.Path(caseDir), tc.content, 0o600); writeErr != nil {
					t.Fatal(writeErr)
				}
			}
			status, _, errOut := runCapture(genRun{dir: caseDir, check: true, now: func() time.Time { return recordedOn }})
			if status != 1 || !strings.Contains(errOut, tc.want) {
				t.Errorf("status = %d, stderr = %q, want 1 and %q", status, errOut, tc.want)
			}
			if tc.content != nil && !strings.Contains(errOut, "re-record with `make gen-orbit-record`") {
				t.Errorf("stderr = %q, want the command that fixes it", errOut)
			}
		})
	}
}

// TestRunMain_ReadsTheFlagsAndTheEnvironment verifies what the command line
// and the environment decide: a bad flag is a usage error, -check judges the
// named directory, and the namespace comes from the flag, then from
// ORBIT_FIXTURES_NAMESPACE, then from the default.
func TestRunMain_ReadsTheFlagsAndTheEnvironment(t *testing.T) {
	var out, errOut bytes.Buffer
	if status := runMain([]string{"-nope"}, func(string) string { return "" }, &out, &errOut); status != 2 {
		t.Errorf("bad flag status = %d, want 2", status)
	}

	dir := t.TempDir()
	previous := upstreamTransport
	t.Cleanup(func() { upstreamTransport = previous })
	for _, tc := range []struct {
		name string
		args []string
		env  map[string]string
		want string
	}{
		{name: "the flag wins", args: []string{"-namespace", "acme"}, env: map[string]string{"ORBIT_FIXTURES_NAMESPACE": "other"}, want: "acme"},
		{name: "then the environment", env: map[string]string{"ORBIT_FIXTURES_NAMESPACE": "other"}, want: "other"},
		{name: "then the default", want: defaultNamespace},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeGitLab{answers: orbitAnswers()}
			upstreamTransport = fake
			env := map[string]string{"GITLAB_COM_TOKEN": "glpat-test"}
			maps.Copy(env, tc.env)
			var stdout, stderr bytes.Buffer
			args := append([]string{"-dir", dir}, tc.args...)
			if status := runMain(args, func(key string) string { return env[key] }, &stdout, &stderr); status != 0 {
				t.Fatalf("record status = %d, stderr:\n%s", status, stderr.String())
			}
			doc, _ := orbitrecord.Read(dir)
			if doc.Source.Namespace != tc.want {
				t.Errorf("namespace = %q, want %q", doc.Source.Namespace, tc.want)
			}
			// The indexing status asks about the namespace in its GraphQL
			// variables.
			if want := `"fullPath":"` + tc.want + `"`; !slices.ContainsFunc(fake.snapshot(), func(asked string) bool { return strings.Contains(asked, want) }) {
				t.Errorf("asked = %q, want the graph status asked about %s", fake.snapshot(), tc.want)
			}
		})
	}

	out.Reset()
	if status := runMain([]string{"-check", "-dir", dir}, func(string) string { return "" }, &out, &errOut); status != 0 {
		t.Errorf("-check status = %d: %s", status, errOut.String())
	}
}

// TestMain_HandsTheStatusToTheProcess verifies main passes what runMain
// decided to the process, through the seam over os.Exit.
func TestMain_HandsTheStatusToTheProcess(t *testing.T) {
	previousExit, previousArgs := osExit, os.Args
	t.Cleanup(func() { osExit, os.Args = previousExit, previousArgs })
	status := -1
	osExit = func(code int) { status = code }
	os.Args = []string{"gen_orbit_record", "-check", "-dir", t.TempDir()}
	main()
	if status != 1 {
		t.Errorf("main exit = %d, want 1 for a missing record", status)
	}
}

// TestFirstNonEmpty_TakesTheFirstThatIsSet verifies the precedence helper.
func TestFirstNonEmpty_TakesTheFirstThatIsSet(t *testing.T) {
	if got := firstNonEmpty("", "b", "c"); got != "b" {
		t.Errorf("firstNonEmpty = %q", got)
	}
	if got := firstNonEmpty("", ""); got != "" {
		t.Errorf("firstNonEmpty of nothing = %q", got)
	}
}

// orbitSetting is one fixture setting of the Orbit make targets and the value
// each layer of a case gives it: the Makefile's default, .env, the
// environment and the make command line. An empty default is a setting the
// Makefile leaves to the program it starts.
type orbitSetting struct {
	name                                        string
	byDefault, dotEnv, environment, commandLine string
}

// orbitSettings are the five settings the Orbit targets read, each with a
// value per layer that no other layer shares, so what a stand-in is handed
// names the layer it came from. ORBIT_FIXTURES_MIRROR alternates, since it
// is a switch: on in .env, off in the environment, on on the command line.
// ORBIT_FIXTURES_INDEXER_TIMEOUT is seen as the number of polls, one per 15
// seconds.
var orbitSettings = []orbitSetting{
	{name: "ORBIT_FIXTURES_NAMESPACE", byDefault: defaultNamespace, dotEnv: "dotenv-group", environment: "environment-group", commandLine: "command-line-group"},
	{name: "ORBIT_FIXTURES_GITLAB_URL", byDefault: "https://gitlab.com", dotEnv: "https://dotenv.example.test", environment: "https://environment.example.test", commandLine: "https://command-line.example.test"},
	{name: "ORBIT_FIXTURES_MIRROR", byDefault: "false", dotEnv: "true", environment: "false", commandLine: "true"},
	{name: "ORBIT_FIXTURES_INDEXER_TIMEOUT", byDefault: "600", dotEnv: "30", environment: "45", commandLine: "60"},
	{name: "ORBIT_FIXTURES_VISIBILITY", dotEnv: "private", environment: "internal", commandLine: "public"},
}

// TestOrbitTargets_FixtureSettings_CommandLineThenEnvironmentThenDotEnv
// renders each of the four Orbit make targets that read the fixture settings
// and runs it against stand-ins for what it starts, holding every setting it
// hands on to the order the Makefile documents: the make command line, then
// the environment, then .env, and only then the default.
//
// Each recipe sourced .env and then used make's own value, which is the
// default whenever the setting is not on the command line or in the
// environment, because the Makefile does not include .env: with .env naming
// a namespace of a contributor's own, the setup, the indexer wait, the live
// suite and the recorder all went to the default namespace without a word
// (issue 1535). The recipes are rendered with make -n rather than read as
// text, so what is judged is the command make would run, and each runs in a
// directory of its own, so the .env it sources is the one the case writes and
// never the repository's. Every stand-in records what it was handed and does
// nothing else: no GitLab is reached.
func TestOrbitTargets_FixtureSettings_CommandLineThenEnvironmentThenDotEnv(t *testing.T) {
	recipe := newOrbitRecipe(t)

	cases := []struct {
		name string
		set  orbitLayers
	}{
		{name: "set nowhere"},
		{name: "set in .env alone", set: orbitLayers{dotEnv: true}},
		{name: "set in the environment and in .env", set: orbitLayers{dotEnv: true, environment: true}},
		{name: "set on the command line, in the environment and in .env", set: orbitLayers{dotEnv: true, environment: true, commandLine: true}},
	}
	// targets are the four targets and the settings each hands on, as the
	// stand-ins report them.
	targets := []struct {
		target   string
		settings []string
	}{
		{target: "orbit-setup-fixtures", settings: []string{"ORBIT_FIXTURES_NAMESPACE", "ORBIT_FIXTURES_GITLAB_URL", "ORBIT_FIXTURES_MIRROR", "ORBIT_FIXTURES_VISIBILITY"}},
		{target: "orbit-wait-indexer", settings: []string{"ORBIT_FIXTURES_NAMESPACE", "ORBIT_FIXTURES_GITLAB_URL", "ORBIT_FIXTURES_INDEXER_TIMEOUT"}},
		{target: "orbit-run-live-tests", settings: []string{"ORBIT_FIXTURES_NAMESPACE"}},
		{target: "gen-orbit-record", settings: []string{"ORBIT_FIXTURES_NAMESPACE"}},
	}
	for _, testCase := range cases {
		for _, target := range targets {
			t.Run(testCase.name+"/"+target.target, func(t *testing.T) {
				environ, commandLine, dotEnv, want := testCase.set.apply()
				seen := recipe.run(t, recipe.render(t, target.target, environ, commandLine), environ, dotEnv)
				for _, name := range target.settings {
					if got, ok := seen[name]; !ok || got != want[name] {
						t.Errorf("%s was handed %s=%q (reported: %t), want %q; the stand-ins reported %v", target.target, name, got, ok, want[name], seen)
					}
				}
			})
		}
	}
}

// orbitLayers says which layers of a case set every Orbit setting: .env, the
// environment and the make command line.
type orbitLayers struct{ dotEnv, environment, commandLine bool }

// apply returns what the layers set: the environment make runs in, the
// variables on its command line, the .env the recipe sources (always holding
// the token orbit-ensure-token asks for), and the value each setting should
// be handed, the highest layer's or the default.
func (set orbitLayers) apply() (environ, commandLine []string, dotEnv string, want map[string]string) {
	dotEnv = "GITLAB_COM_TOKEN=glpat-stand-in\n"
	want = map[string]string{}
	environ = orbitRecipeEnviron()
	for _, setting := range orbitSettings {
		want[setting.name] = setting.byDefault
		if set.dotEnv {
			dotEnv += setting.name + "=" + setting.dotEnv + "\n"
			want[setting.name] = setting.dotEnv
		}
		if set.environment {
			environ = append(environ, setting.name+"="+setting.environment)
			want[setting.name] = setting.environment
		}
		if set.commandLine {
			commandLine = append(commandLine, setting.name+"="+setting.commandLine)
			want[setting.name] = setting.commandLine
		}
	}
	return environ, commandLine, dotEnv, want
}

// orbitRecipe is what rendering and running an Orbit recipe needs: make, the
// shell make runs a recipe line with, the repository the Makefile sits in,
// and a directory holding the stand-ins found on the PATH.
type orbitRecipe struct {
	makeBin, shell, root, stubs string
}

// orbitStandIns are the programs an Orbit recipe starts from the PATH,
// written as shell scripts that append what they were handed to
// $ORBIT_STUB_LOG, one NAME=value line per setting. curl answers the indexer
// wait with a namespace already indexed, so the wait ends on its first poll,
// having printed the number of polls its timeout allows. The setup script is
// not among them: a recipe starts it by a path relative to the directory it
// runs in, so it is written there.
var orbitStandIns = map[string]string{
	"go": `#!/bin/sh
printf 'ORBIT_FIXTURES_NAMESPACE=%s\n' "$ORBIT_FIXTURES_NAMESPACE" >> "$ORBIT_STUB_LOG"
`,
	"curl": `#!/bin/sh
url= body=
while [ $# -gt 0 ]; do
  case "$1" in
    -d) body=$2; shift 2 ;;
    -H) shift 2 ;;
    http*) url=$1; shift ;;
    *) shift ;;
  esac
done
printf 'ORBIT_FIXTURES_GITLAB_URL=%s\n' "${url%/api/graphql}" >> "$ORBIT_STUB_LOG"
namespace=${body##*\"fullPath\":\"}
printf 'ORBIT_FIXTURES_NAMESPACE=%s\n' "${namespace%%\"*}" >> "$ORBIT_STUB_LOG"
printf '%s' '{"data":{"group":{"orbit":{"indexing":{"state":"READY","projects":{"indexed":2,"notIndexed":0}}}}}}'
`,
}

// orbitSetupStandIn stands in for scripts/setup-orbit-fixtures.sh, which
// reads the settings from its environment and the mirror as a flag.
const orbitSetupStandIn = `#!/bin/sh
mirror=false
[ "$1" = "--mirror-cli" ] && mirror=true
printf 'ORBIT_FIXTURES_NAMESPACE=%s\nORBIT_FIXTURES_GITLAB_URL=%s\nORBIT_FIXTURES_MIRROR=%s\nORBIT_FIXTURES_VISIBILITY=%s\n' \
  "$ORBIT_FIXTURES_NAMESPACE" "$ORBIT_FIXTURES_GITLAB_URL" "$mirror" "$ORBIT_FIXTURES_VISIBILITY" >> "$ORBIT_STUB_LOG"
`

// newOrbitRecipe finds the tools the recipes need, skipping the test where
// one is missing, and writes the stand-ins.
func newOrbitRecipe(t *testing.T) orbitRecipe {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the recipes are shell and the stand-ins shell scripts, which Windows does not start as programs")
	}
	var recipe orbitRecipe
	var err error
	if recipe.makeBin, err = exec.LookPath("make"); err != nil {
		t.Skipf("make is not installed here, so the recipes cannot be rendered: %v", err)
	}
	if recipe.shell, err = exec.LookPath("sh"); err != nil {
		t.Skipf("sh is not installed here, so the rendered recipes cannot be run: %v", err)
	}
	// The indexer wait reads the GraphQL answer with python3.
	if _, err = exec.LookPath("python3"); err != nil {
		t.Skipf("python3 is not installed here, so the indexer wait cannot be run: %v", err)
	}
	if recipe.root, err = filepath.Abs(filepath.Join("..", "..")); err != nil {
		t.Fatalf("resolving the repository root: %v", err)
	}
	recipe.stubs = t.TempDir()
	for name, body := range orbitStandIns {
		//#nosec G306 -- a stand-in is a script the rendered recipe must execute
		if err = os.WriteFile(filepath.Join(recipe.stubs, name), []byte(body), 0o700); err != nil {
			t.Fatalf("writing the stand-in %s: %v", name, err)
		}
	}
	return recipe
}

// render asks make for the recipe of target it would run under environ, with
// commandLine as variables on its command line.
func (r orbitRecipe) render(t *testing.T, target string, environ, commandLine []string) string {
	t.Helper()
	args := append([]string{"-n", "-s", "--no-print-directory", target}, commandLine...)
	//#nosec G204 -- make from the PATH, with this file's own arguments and a case's values
	render := exec.CommandContext(t.Context(), r.makeBin, args...)
	render.Dir = r.root
	render.Env = environ
	recipe, err := render.Output()
	if err != nil {
		t.Fatalf("make -n %s: %v", target, err)
	}
	return string(recipe)
}

// run runs a rendered recipe the way make would, in a directory of its own
// holding dotEnv as .env and the setup stand-in, with the stand-ins first on
// the PATH, and returns the last value each setting was reported with.
func (r orbitRecipe) run(t *testing.T, recipe string, environ []string, dotEnv string) map[string]string {
	t.Helper()
	work := t.TempDir()
	if err := os.WriteFile(filepath.Join(work, ".env"), []byte(dotEnv), 0o600); err != nil {
		t.Fatalf("writing .env: %v", err)
	}
	if err := os.Mkdir(filepath.Join(work, "scripts"), 0o700); err != nil {
		t.Fatalf("creating the scripts directory: %v", err)
	}
	//#nosec G306 -- the stand-in is a script the rendered recipe must execute
	if err := os.WriteFile(filepath.Join(work, "scripts", "setup-orbit-fixtures.sh"), []byte(orbitSetupStandIn), 0o700); err != nil {
		t.Fatalf("writing the setup stand-in: %v", err)
	}
	log := filepath.Join(work, "stand-ins.log")
	//#nosec G204 -- the recipe is what make rendered from this repository's own Makefile
	run := exec.CommandContext(t.Context(), r.shell, "-c", recipe)
	run.Dir = work
	// The last PATH wins, which puts the stand-ins first.
	run.Env = append(slices.Clone(environ), "ORBIT_STUB_LOG="+log, "PATH="+r.stubs+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("running the rendered recipe: %v\n%s", err, out)
	}
	reported, err := os.ReadFile(log) //#nosec G304 -- a file of the test's own temporary directory
	if err != nil {
		t.Fatalf("no stand-in was started: %v\n%s", err, out)
	}
	seen := map[string]string{}
	for line := range strings.Lines(string(reported)) {
		name, value, _ := strings.Cut(strings.TrimSuffix(line, "\n"), "=")
		seen[name] = value
	}
	// The indexer wait reports its timeout as the number of polls it allows,
	// one every 15 seconds.
	if _, polls, found := strings.Cut(string(out), "[1/"); found {
		if count, _, ok := strings.Cut(polls, "]"); ok {
			if n, convErr := strconv.Atoi(count); convErr == nil {
				seen["ORBIT_FIXTURES_INDEXER_TIMEOUT"] = strconv.Itoa(n * 15)
			}
		}
	}
	return seen
}

// orbitRecipeEnviron is this process's environment with nothing in it that
// would decide a setting or the make invocation on the test's behalf: none of
// the settings, no token, and none of the variables a parent make hands a
// child, since a variable given on a parent's command line reaches a child
// make through MAKEFLAGS as a command-line variable of its own.
func orbitRecipeEnviron() []string {
	dropped := []string{"GITLAB_COM_TOKEN", "MAKEFLAGS", "MFLAGS", "GNUMAKEFLAGS", "MAKELEVEL", "MAKEOVERRIDES", "ORBIT_RECORD_FLAGS"}
	for _, setting := range orbitSettings {
		dropped = append(dropped, setting.name)
	}
	return slices.DeleteFunc(os.Environ(), func(entry string) bool {
		name, _, _ := strings.Cut(entry, "=")
		return slices.Contains(dropped, name)
	})
}
