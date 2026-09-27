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
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/orbitrecord"
)

// recordedOn is the day every fixture recording is stamped with.
var recordedOn = time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)

// fakeAnswer is what the fake GitLab.com answers one request with.
type fakeAnswer struct {
	status      int
	contentType string
	body        string
}

// orbitAnswers are the answers of a healthy GitLab.com, keyed by route and
// response format, shaped like the ones recorded on 2026-09-27 and carrying
// keys a verbatim path must keep out of the record ($defs, a JSON Schema's
// own property names, a metric name that is no identifier) so a test sees the
// rule work: without the verbatim path the recording would refuse the key.
func orbitAnswers() map[string]fakeAnswer {
	ok := func(body string) fakeAnswer {
		return fakeAnswer{status: 200, contentType: "application/json", body: body}
	}
	return map[string]fakeAnswer{
		"GET /orbit/status raw":       ok(`{"user":{"available":true},"system":{"status":"healthy","timestamp":"t","version":"0.130.0","components":[{"name":"api","status":"healthy","replicas":{"ready":1,"desired":2},"metrics":{"kind":"deployment","query-latency.p99_ms":"12"}}]}}`),
		"GET /orbit/status llm":       ok(`{"user":{"available":true},"system":{"formatted_text":"status: healthy"}}`),
		"GET /orbit/schema raw":       ok(`{"schema_version":"1","domains":[{"name":"core","description":"d","node_names":["User"]}],"nodes":[{"name":"User","domain":"core"}],"edges":[{"name":"AUTHORED","description":"d","variants":[{"source_type":"User","target_type":"Issue"}]}]}`),
		"GET /orbit/schema llm":       ok(`{"formatted_text":"nodes: User"}`),
		"GET /orbit/tools ":           ok(`[{"name":"invoke_command","description":"d","parameters":{"type":"object","properties":{"$ref":{}}}}]`),
		"GET /orbit/schema/dsl raw":   ok(`{"$defs":{},"version":"12.1.8"}`),
		"GET /orbit/schema/dsl llm":   ok(`"query := node+"`),
		"POST /orbit/query raw":       ok(`{"result":{"format_version":"5.0.3","nodes":[{"id":"1","full_path":"plens1/kg-fixtures"}],"edges":[]},"query_type":"traversal","row_count":1}`),
		"POST /orbit/query llm":       {status: 200, contentType: "text/plain; charset=utf-8", body: "@header\nquery_type:traversal\n"},
		"GET /orbit/graph_status raw": ok(`{"projects":{"indexed":2,"total_known":2},"domains":[{"name":"SDLC","items":[{"name":"Project","count":2}]}],"indexing":{"state":"indexed","last_error":null}}`),
		"GET /orbit/graph_status llm": ok(`{"formatted_text":"indexed"}`),
	}
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
	format := req.URL.Query().Get("response_format")
	if req.Body != nil {
		body, _ := io.ReadAll(req.Body)
		var fields struct {
			ResponseFormat string `json:"response_format"`
		}
		if json.Unmarshal(body, &fields) == nil && fields.ResponseFormat != "" {
			format = fields.ResponseFormat
		}
	}
	key := req.Method + " " + strings.TrimPrefix(req.URL.Path, "/api/v4") + " " + format
	f.mu.Lock()
	f.asked = append(f.asked, req.URL.Scheme+"://"+req.URL.Host+" "+key+" "+req.URL.RawQuery)
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
	if !slices.Equal(args, []string{"show", committedRevision}) {
		return nil, fmt.Errorf("unexpected git %q", args)
	}
	raw, err := os.ReadFile(orbitrecord.Path(h.dir))
	if err != nil {
		return nil, fmt.Errorf("git show %s: fatal: path does not exist in 'HEAD'", committedRevision)
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
	if !strings.Contains(out, "wrote "+orbitrecord.Path(dir)+": 12 calls, Orbit 0.130.0") || !strings.Contains(out, "nothing to compare this one with") {
		t.Errorf("stdout = %q", out)
	}
	if asked := fake.snapshot(); len(asked) != 12 || !strings.HasPrefix(asked[0], "https://gitlab.com GET /orbit/status raw") {
		t.Errorf("asked = %q, want the twelve calls against gitlab.com", asked)
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
	if doc.Source != (orbitrecord.Source{Instance: orbitrecord.Instance, OrbitVersion: "0.130.0", Namespace: "plens1", RetrievedAt: "2026-09-27"}) {
		t.Errorf("source = %+v", doc.Source)
	}
	checkStatus, checkOut, checkErr := runCapture(genRun{dir: dir, check: true, now: func() time.Time { return recordedOn }})
	if checkStatus != 0 || !strings.Contains(checkOut, "12 calls recorded from https://gitlab.com, Orbit 0.130.0, namespace plens1, on 2026-09-27") {
		t.Errorf("check = %d, %q, %q", checkStatus, checkOut, checkErr)
	}

	head.commit(t, dir)
	cfg.client = &http.Client{Transport: &fakeGitLab{answers: orbitAnswers()}}
	again, againOut, againErr := runCapture(cfg)
	if again != 0 || !strings.Contains(againOut, "the key tree is the one committed at HEAD") {
		t.Errorf("second record = %d, %q, %q", again, againOut, againErr)
	}
}

// TestRecord_WhatTheRecordHolds_IsTheShapeAndNeverAValue verifies the calls
// the record keeps: each names the output type its handler returned, the
// parameter names the handler sent (the schema's format as response_format,
// the query's in its body), the media type, and keys with kinds only, with
// the data-keyed subtrees kept as their roots.
func TestRecord_WhatTheRecordHolds_IsTheShapeAndNeverAValue(t *testing.T) {
	dir := t.TempDir()
	if status, _, errOut := runCapture(testRun(dir, &fakeGitLab{answers: orbitAnswers()})); status != 0 {
		t.Fatalf("record failed: %s", errOut)
	}
	raw, err := os.ReadFile(orbitrecord.Path(dir))
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"healthy", "deployment", "query-latency", "metrics.kind", "plens1/kg-fixtures", "invoke_command", "AUTHORED", "$defs", "$ref", "query := node+", "@header"} {
		t.Run("no "+value, func(t *testing.T) {
			if bytes.Contains(raw, []byte(value)) {
				t.Errorf("the record holds %q, a value or a data key", value)
			}
		})
	}
	doc, _ := orbitrecord.Read(dir)
	calls := map[string]orbitrecord.Call{}
	for _, call := range doc.Calls {
		calls[call.ID().String()] = call
	}
	schema := calls["orbit.schema (llm)"]
	if schema.Output != "internal/tools/orbit.SchemaOutput" || !slices.Equal(schema.Request.Query, []string{"response_format"}) {
		t.Errorf("schema llm = %+v", schema)
	}
	query := calls["orbit.query (llm)"]
	if query.Request.Method != http.MethodPost || !slices.Equal(query.Request.Body, []string{"query", "response_format"}) ||
		query.Response.ContentType != "text/plain" || query.Response.Keys[0].Kinds[0] != orbitrecord.KindText {
		t.Errorf("query llm = %+v", query)
	}
	verbatim := map[string]string{
		"orbit.status (raw)":    "system.components[].metrics",
		"orbit.tools (default)": "[].parameters",
		"orbit.dsl (raw)":       orbitrecord.Root,
		"orbit.query (raw)":     "result.nodes[]",
	}
	for id, path := range verbatim {
		t.Run(id, func(t *testing.T) {
			assertKeptVerbatim(t, calls[id], path)
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
	changed["GET /orbit/graph_status llm"] = fakeAnswer{status: 200, contentType: "application/json", body: `{"formatted_text":"indexed","region":"eu"}`}
	changed["GET /orbit/schema llm"] = fakeAnswer{status: 200, contentType: "application/json", body: `{"text":"nodes"}`}
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
				"+ orbit.graph_status (llm): region string",
				"+ orbit.schema (llm): text string",
				"- orbit.schema (llm): formatted_text string",
				"the key tree differs from the one committed at HEAD in 3 places",
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
	delete(refused, "GET /orbit/schema llm")
	versionless := orbitAnswers()
	versionless["GET /orbit/status raw"] = fakeAnswer{status: 200, contentType: "application/json", body: `{"user":{"available":true},"system":{"status":"unknown"}}`}
	noRows := orbitAnswers()
	noRows["POST /orbit/query raw"] = fakeAnswer{status: 200, contentType: "application/json", body: `{"result":{"format_version":"5.0.3","nodes":[],"edges":[]},"query_type":"traversal","row_count":0}`}
	unindexed := orbitAnswers()
	unindexed["GET /orbit/graph_status raw"] = fakeAnswer{status: 200, contentType: "application/json", body: `{"projects":{"indexed":0,"total_known":2},"domains":[],"indexing":{"state":"pending"}}`}
	cases := []struct {
		name  string
		token string
		fake  *fakeGitLab
		want  string
	}{
		{name: "no token", fake: &fakeGitLab{answers: orbitAnswers()}, want: "GITLAB_COM_TOKEN is not set"},
		{name: "a refused call", token: "glpat-test", fake: &fakeGitLab{answers: refused}, want: "orbit.schema (llm) failed, so its answer cannot be recorded"},
		{name: "a query that finds no row", token: "glpat-test", fake: &fakeGitLab{answers: noRows}, want: "orbit.query (raw) found no row for the fixture project: the fixture namespace is not indexed"},
		{name: "a namespace with no indexed project", token: "glpat-test", fake: &fakeGitLab{answers: unindexed}, want: "orbit.graph_status (raw) counts no indexed project"},
		{name: "a record the check refuses", token: "glpat-test", fake: &fakeGitLab{answers: versionless}, want: "names no Orbit version"},
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
			if want := "full_path=" + tc.want; !slices.ContainsFunc(fake.snapshot(), func(asked string) bool { return strings.Contains(asked, want) }) {
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
