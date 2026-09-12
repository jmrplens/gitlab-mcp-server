//go:build e2e

// record_test.go drives the recorder the way a run drives it: the real binary,
// on a real surface, against a stub GitLab, with the server's own spans coming
// back over the loopback receiver.
//
// One thing cannot be asserted from inside a test, and it shapes the file: a
// failing subtest fails its parent, so a test cannot let the flush fail and
// then check that it did. Every test that asks what the flush would report
// therefore calls finish with a reporter of its own, which is the same code
// the cleanup runs and differs only in where the failure goes.

package harness

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/e2ecalls"
)

// The two errors the outcome classification tells apart: a server that
// answered no, and a connection that went away before it could.
var (
	errUnknownPrompt = errors.New("prompts/get: unknown prompt")
	errBrokenPipe    = errors.New("write |1: broken pipe")
)

// capturedReporter is a test's stand-in for the testing.T a flush reports
// through, so an assertion about a failure does not have to produce one.
type capturedReporter struct {
	mu       sync.Mutex
	failures []string
}

// Errorf records one failure.
func (c *capturedReporter) Errorf(format string, args ...any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failures = append(c.failures, strings.TrimSpace(fmt.Sprintf(format, args...)))
}

// reported returns what was reported, joined for a message.
func (c *capturedReporter) reported() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return strings.Join(c.failures, "\n")
}

// count returns how many failures were reported.
func (c *capturedReporter) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.failures)
}

// startLicensedStubGitLab serves what an Ultimate instance answers at startup.
//
// The license is what this stub adds: the alias rewrite that turns
// gitlab_environment get into protected_get only fires when the catalog has
// protected_get, and that route is licensed. Without a license the rewrite
// cannot happen and there is nothing for the dispatch assertion to see.
func startLicensedStubGitLab(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		writeStubJSON(w, map[string]any{"version": "18.0.0", "revision": "abcdef", "enterprise": true})
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		writeStubJSON(w, map[string]any{"id": 7, "username": "harness", "name": "Harness", "is_admin": true})
	})
	mux.HandleFunc("/api/v4/license", func(w http.ResponseWriter, _ *http.Request) {
		writeStubJSON(w, map[string]any{"id": 1, "plan": "ultimate"})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

// offlineInstance builds an instance with no GitLab behind it, for the tests
// whose subject is the record rather than a call.
func offlineInstance() *instance {
	return &instance{
		settings:    testSettings(map[string]string{envGitLabURL: "http://gitlab.test", envGitLabToken: stubToken}),
		requirement: Any,
		pkg:         "harness",
		runID:       "run-offline",
		facts:       runtimeFacts{URL: "http://gitlab.test", Version: "18.0.0", Tier: edition.Free},
	}
}

// recordInto points this test's records at a directory of its own and returns
// it.
//
// Release is registered straight after the directory is created, so the shard
// is closed before the directory is removed: on Windows a directory holding an
// open file cannot be removed, and a test would then fail in cleanup with
// every one of its own assertions passed.
func recordInto(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	t.Cleanup(e2ecalls.Release)
	t.Setenv(e2ecalls.DirEnv, dir)
	return dir
}

// TestRecorder_MetaAliasRewrite_RecordsTheRouteThatRanAndFailsAnUndeclaredCall
// is the assertion the whole recorder exists to make.
//
// gitlab_environment with action get and an environment named rather than
// numbered runs protected_get. A record crediting what the test asked for
// would credit environment.get for a call that never ran it, which is exactly
// the class of false coverage this rebuild replaces. The call is therefore
// recorded with both halves, and a test that did not declare the rewrite is
// failed.
func TestRecorder_MetaAliasRewrite_RecordsTheRouteThatRanAndFailsAnUndeclaredCall(t *testing.T) {
	inst := instanceForStub(t, startLicensedStubGitLab(t))
	params := map[string]any{"project_id": "group/project", "environment_id": "production"}

	t.Run("undeclared", func(t *testing.T) {
		env := newEnv(t, inst)
		session := env.Session(ServerConfig{Surface: SurfaceMeta, Private: true})

		_, _ = Try[map[string]any](session, "environment.get", params)

		reporter := &capturedReporter{}
		lines := env.recorder.finish(reporter, e2ecalls.StatusPassed)

		call := callLineFor(t, lines, "environment.get")
		if call.Dispatched != "environment.protected_get" {
			t.Errorf("dispatched = %q, want environment.protected_get; the server rewrote the action and the "+
				"record has to say so", call.Dispatched)
		}
		if !strings.Contains(reporter.reported(), "environment.protected_get") {
			t.Errorf("the flush reported %q, want it to fail the test and name the route that ran",
				reporter.reported())
		}
	})

	t.Run("declared", func(t *testing.T) {
		env := newEnv(t, inst)
		session := env.Session(ServerConfig{Surface: SurfaceMeta, Private: true})

		_, _ = Try[map[string]any](session, "environment.get", params, ExpectDispatch("environment.protected_get"))

		reporter := &capturedReporter{}
		lines := env.recorder.finish(reporter, e2ecalls.StatusPassed)

		call := callLineFor(t, lines, "environment.get")
		if call.Dispatched != "environment.protected_get" {
			t.Errorf("dispatched = %q, want environment.protected_get", call.Dispatched)
		}
		if reporter.count() != 0 {
			t.Errorf("the flush failed a test that declared the rewrite: %s", reporter.reported())
		}
	})
}

// callLineFor returns the one call line naming an action, failing when the
// record holds no such call.
func callLineFor(t *testing.T, lines []e2ecalls.Line, action string) *e2ecalls.Call {
	t.Helper()

	for _, line := range lines {
		call, isCall := line.(*e2ecalls.Call)
		if isCall && call.Action == action {
			return call
		}
	}
	t.Fatalf("the record holds no call of %s: %s", action, describeLines(lines))
	return nil
}

// describeLines summarizes a record for a failure message.
func describeLines(lines []e2ecalls.Line) string {
	described := make([]string, 0, len(lines))
	for _, line := range lines {
		switch typed := line.(type) {
		case *e2ecalls.Call:
			described = append(described, fmt.Sprintf("call %s/%s %s", typed.Method, typed.Action, typed.Outcome))
		case *e2ecalls.Dispatch:
			described = append(described, fmt.Sprintf("dispatch %s %s", typed.Action, typed.RefusalReason))
		case *e2ecalls.Skip:
			described = append(described, fmt.Sprintf("skip %s: %s", typed.Test, typed.Reason))
		default:
			described = append(described, fmt.Sprintf("%T", line))
		}
	}
	return strings.Join(described, "; ")
}

// TestRecorder_IndividualSafeMode_RecordsAPreviewAndTheServersRefusal covers
// the outcome no client-side classification can produce on its own.
//
// A safe-mode preview is a successful result carrying a description of the
// mutation that did not happen, and the server records it as a refusal with
// the reason safe_mode. The record has to carry both: the client's outcome, so
// a report knows the mutation never ran, and the server's reason, so it knows
// why.
func TestRecorder_IndividualSafeMode_RecordsAPreviewAndTheServersRefusal(t *testing.T) {
	inst := stubInstance(t)
	env := newEnv(t, inst)
	session := env.Session(ServerConfig{Surface: SurfaceIndividual, Mode: ModeSafe, Private: true})

	_, _ = Try[map[string]any](session, "project.create", map[string]any{"name": env.Name("safe-mode")})

	reporter := &capturedReporter{}
	lines := env.recorder.finish(reporter, e2ecalls.StatusPassed)

	call := callLineFor(t, lines, "project.create")
	if call.Outcome != e2ecalls.OutcomePreview {
		t.Errorf("outcome = %q, want %q: safe mode answered with a preview and the record calls it a success",
			call.Outcome, e2ecalls.OutcomePreview)
	}
	if reason := dispatchReasonFor(lines, call.TraceID); reason != "safe_mode" {
		t.Errorf("the server's refusal reason = %q, want safe_mode; the record cannot say why the mutation did "+
			"not run", reason)
	}
	if reporter.count() != 0 {
		t.Errorf("the flush failed a call that dispatched what it asked for: %s", reporter.reported())
	}
}

// dispatchReasonFor returns the refusal reason the server reported for one
// trace, or the empty string when no span carried one.
func dispatchReasonFor(lines []e2ecalls.Line, traceID string) string {
	for _, line := range lines {
		dispatch, isDispatch := line.(*e2ecalls.Dispatch)
		if isDispatch && dispatch.TraceID == traceID {
			return dispatch.RefusalReason
		}
	}
	return ""
}

// TestRecorder_ShardDirectoryUnset_WritesNothing pins that recording costs an
// ordinary run nothing.
//
// The harness records unconditionally and never branches on the variable: a
// writer for a directory nobody named is nil, and a nil writer's Write does
// nothing. This is what makes that safe to rely on.
func TestRecorder_ShardDirectoryUnset_WritesNothing(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(e2ecalls.Release)
	t.Setenv(e2ecalls.DirEnv, "")

	env := newEnv(t, offlineInstance())
	env.recorder.record(fabricatedCall(env, "issue.list"))
	reporter := &capturedReporter{}

	env.recorder.finish(reporter, e2ecalls.StatusPassed)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading the directory nothing should have written to: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("the record wrote %d files with no directory named", len(entries))
	}
	if reporter.count() != 0 {
		t.Errorf("a run with recording off reported %s", reporter.reported())
	}
}

// TestRecorder_ShardDirectoryNamed_WritesTheCallLine is the other half: with a
// directory named, the line reaches the shard the coverage audit reads.
func TestRecorder_ShardDirectoryNamed_WritesTheCallLine(t *testing.T) {
	dir := recordInto(t)

	env := newEnv(t, offlineInstance())
	env.recorder.record(fabricatedCall(env, "issue.list"))

	env.recorder.finish(&capturedReporter{}, e2ecalls.StatusPassed)
	e2ecalls.Release()

	records, err := e2ecalls.Read(dir)
	if err != nil {
		t.Fatalf("reading the shard back: %v", err)
	}
	found := false
	for _, record := range records {
		if record.Call != nil && record.Call.Action == "issue.list" {
			found = true
			if record.Call.TestStatus != e2ecalls.StatusPassed {
				t.Errorf("test_status = %q, want passed", record.Call.TestStatus)
			}
		}
	}
	if !found {
		t.Errorf("the shard holds no call of issue.list; %d records were written", len(records))
	}
}

// TestRecorder_RelativeShardDirectory_IsRefusedWithTheReason checks that a
// directory that would scatter shards nothing can find is refused rather than
// resolved.
//
// A test binary runs in its own package directory, so a relative path writes
// one shard under each of the suite's packages and the merge finds none of
// them. The refusal has to say that, because the alternative is a coverage
// report that is silently empty.
func TestRecorder_RelativeShardDirectory_IsRefusedWithTheReason(t *testing.T) {
	t.Cleanup(e2ecalls.Release)
	t.Setenv(e2ecalls.DirEnv, filepath.Join("dist", "e2e-calls"))

	env := newEnv(t, offlineInstance())
	env.recorder.record(fabricatedCall(env, "issue.list"))
	reporter := &capturedReporter{}

	env.recorder.finish(reporter, e2ecalls.StatusPassed)

	if !strings.Contains(reporter.reported(), "absolute path") {
		t.Errorf("the refusal is %q, want it to say the directory must be absolute", reporter.reported())
	}
}

// TestRecorder_SkippedSubtest_WritesASkipLineWithItsReason covers the whole
// skip path, from the need that was not provided to the shard.
//
// A skip is the honest third answer beside covered and absent: an action whose
// only scenario skips for want of a runner is not covered, and a report that
// said why is worth more than an empty cell. The testing package keeps a skip
// message to itself, so the harness has to record the reason before it skips,
// and this is what proves the two are wired together.
func TestRecorder_SkippedSubtest_WritesASkipLineWithItsReason(t *testing.T) {
	dir := recordInto(t)
	inst := offlineInstance()

	// A skipped subtest does not fail its parent, so the skip can be provoked
	// for real rather than simulated.
	t.Run("needs a runner", func(t *testing.T) {
		newEnv(t, inst, Needs(NeedRunner))
		t.Error("the test was not skipped, and this run has no CI runner")
	})
	e2ecalls.Release()

	records, err := e2ecalls.Read(dir)
	if err != nil {
		t.Fatalf("reading the shard back: %v", err)
	}
	for _, record := range records {
		if record.Skip == nil {
			continue
		}
		if !strings.Contains(record.Skip.Test, "needs_a_runner") {
			continue
		}
		if !strings.Contains(record.Skip.Reason, "runner") {
			t.Errorf("the skip reason is %q, want it to name the requirement", record.Skip.Reason)
		}
		return
	}
	t.Errorf("the shard holds no skip line for the subtest; %d records were written", len(records))
}

// fabricatedCall builds one recorded call without making one, for the tests
// whose subject is the writing rather than the calling.
//
// It carries no trace, which is the shape of a call the harness could not
// stamp: the flush then has no span to wait for and these tests do not pay the
// dispatch budget for one that was never issued.
func fabricatedCall(env *Env, action ActionID) *pendingCall {
	return &pendingCall{line: &e2ecalls.Call{
		Test:        env.T.Name(),
		Purpose:     string(PurposeTest),
		Expectation: ExpectationOK,
		Session:     "dynamic-default-full",
		Surface:     string(SurfaceDynamic),
		Mode:        string(ModeDefault),
		Method:      methodCallTool,
		Tool:        "gitlab_execute_action",
		Action:      string(action),
		Outcome:     e2ecalls.OutcomeOK,
	}}
}

// TestCallOptions_Expecting_IsTheVerbsOwnAndNotTheCallers pins where the
// expectation comes from.
//
// The verb is the assertion: Do expects success, Refused expects a class,
// Try constrains nothing. A caller cannot override it, because a record whose
// expectation disagreed with the assertion the test made would classify the
// call under a state no test ever asked for.
func TestCallOptions_Expecting_IsTheVerbsOwnAndNotTheCallers(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{name: "ok", want: ExpectationOK},
		{name: "any", want: ExpectationAny},
		{name: "a failure class", want: string(FailureNeedsConfirmation)},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			resolved := resolveCallOptions(nil).expecting(testCase.want)
			if resolved.expectation != testCase.want {
				t.Errorf("expectation = %q, want %q", resolved.expectation, testCase.want)
			}
		})
	}

	if defaults := resolveCallOptions(nil); defaults.expectation != ExpectationOK {
		t.Errorf("a call with no verb applied expects %q, want %q", defaults.expectation, ExpectationOK)
	}
}

// TestExpectDispatch_DeclaresTheRouteTheCallWillRun checks the option the
// dispatch assertion reads.
func TestExpectDispatch_DeclaresTheRouteTheCallWillRun(t *testing.T) {
	resolved := resolveCallOptions([]CallOption{ExpectDispatch("environment.protected_get")})

	if resolved.dispatch != "environment.protected_get" {
		t.Errorf("dispatch = %q, want environment.protected_get", resolved.dispatch)
	}
}

// TestNewTraceParent_IsAFreshSampledTraceEveryTime checks the value the server
// reads the trace off.
//
// Sampled, because a span that was not recorded is a call whose dispatch
// nothing can report. Fresh, because two calls sharing a trace id would join
// to each other's spans, and a retry is a call of its own.
func TestNewTraceParent_IsAFreshSampledTraceEveryTime(t *testing.T) {
	traceID, traceParent := newTraceParent()
	other, _ := newTraceParent()

	if len(traceID) != 32 {
		t.Errorf("trace id %q is %d characters, want 32", traceID, len(traceID))
	}
	if traceID == other {
		t.Errorf("two calls were given the same trace id %q", traceID)
	}
	if want := "00-" + traceID + "-"; !strings.HasPrefix(traceParent, want) {
		t.Errorf("traceparent = %q, want it to start with %q", traceParent, want)
	}
	if !strings.HasSuffix(traceParent, "-01") {
		t.Errorf("traceparent = %q, want it to end sampled", traceParent)
	}
}

// TestStampTraceParent_WritesTheOneKeyTheServerReads pins the client half of
// the join.
//
// MCP reserves traceparent unprefixed at the top level of _meta, and this
// server already reads it there. Nothing else is written: a key the server
// does not read would be noise on every call of the run.
func TestStampTraceParent_WritesTheOneKeyTheServerReads(t *testing.T) {
	params := &mcp.CallToolParams{Name: "gitlab_issue_list", Arguments: map[string]any{"project_id": "a/b"}}
	req := &mcp.ClientRequest[*mcp.CallToolParams]{Params: params}

	if !stampTraceParent(req, "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01") {
		t.Fatal("the traceparent was not stamped onto a request that carries params")
	}
	meta := params.GetMeta()
	if meta[traceParentKey] != "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01" {
		t.Errorf("_meta[%q] = %v, want the traceparent", traceParentKey, meta[traceParentKey])
	}
	if len(meta) != 1 {
		t.Errorf("_meta carries %d keys, want only the traceparent: %v", len(meta), meta)
	}
}

// TestStampTraceParent_TypedNilParams_StampsNothing covers the shape that
// panics if it is trusted.
//
// A method whose params may be missing is handed to a middleware as a typed
// nil pointer inside a non-nil interface. Comparing the interface against nil
// says false, and the first method call on it dereferences the pointer, which
// is a panic in the middle of every list request.
func TestStampTraceParent_TypedNilParams_StampsNothing(t *testing.T) {
	req := &mcp.ClientRequest[*mcp.ListToolsParams]{Params: nil}

	if stampTraceParent(req, "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01") {
		t.Error("a request carrying no params was reported as stamped")
	}
}

// TestOutcomeOf_ClassifiesEveryMethodInTheRecordsVocabulary checks that a
// record and a failure message can never disagree.
//
// A tool call goes through the same classify the assertions use; every other
// method has two outcomes, and the one that matters is the difference between
// a server that said no and a connection that went away.
func TestOutcomeOf_ClassifiesEveryMethodInTheRecordsVocabulary(t *testing.T) {
	cases := []struct {
		name   string
		method string
		result mcp.Result
		err    error
		want   string
	}{
		{
			name:   "a tool call that ran",
			method: methodCallTool,
			result: &mcp.CallToolResult{StructuredContent: map[string]any{"id": 1}},
			want:   e2ecalls.OutcomeOK,
		},
		{
			name:   "a tool call the handler refused",
			method: methodCallTool,
			result: errorResult("something went wrong"),
			want:   e2ecalls.OutcomeToolError,
		},
		{
			name:   "a resource read that answered",
			method: methodReadResource,
			result: &mcp.ReadResourceResult{},
			want:   e2ecalls.OutcomeOK,
		},
		{
			name:   "a prompt the server refused",
			method: methodGetPrompt,
			err:    errUnknownPrompt,
			want:   e2ecalls.OutcomeProtocolError,
		},
		{
			name:   "a call whose connection went away",
			method: methodComplete,
			err:    errBrokenPipe,
			want:   e2ecalls.OutcomeTransportError,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := outcomeOf(testCase.method, testCase.result, testCase.err); got != testCase.want {
				t.Errorf("outcome = %q, want %q", got, testCase.want)
			}
		})
	}
}

// TestDescribeToolRequest_NamesTheToolAndItsArgumentNames pins what a call
// line carries about the arguments.
//
// The names and not the values: a value is a fixture and belongs to one run,
// while a name is part of the call and is what a coverage report compares
// against a schema.
func TestDescribeToolRequest_NamesTheToolAndItsArgumentNames(t *testing.T) {
	req := &mcp.ClientRequest[*mcp.CallToolParams]{Params: &mcp.CallToolParams{
		Name:      "gitlab_issue_list",
		Arguments: map[string]any{"project_id": "a/b", "state": "opened"},
	}}

	tool, arguments := describeToolRequest(req)

	if tool != "gitlab_issue_list" {
		t.Errorf("tool = %q, want gitlab_issue_list", tool)
	}
	if strings.Join(arguments, ",") != "project_id,state" {
		t.Errorf("arguments = %v, want the sorted names", arguments)
	}
	if strings.Contains(strings.Join(arguments, ","), "a/b") {
		t.Errorf("arguments = %v, and a record must carry no values", arguments)
	}
}

// TestCurrentInFlight_AnswersOnlyWhenOneCallIsInFlight pins how an elicitation
// is attributed.
//
// An elicitation arrives on the SDK's own receiving goroutine, inside a call
// that is still waiting. With one call in flight the attribution is exact;
// with several the harness declines to guess, because a line attributed to the
// wrong test is worse than a missing one.
func TestCurrentInFlight_AnswersOnlyWhenOneCallIsInFlight(t *testing.T) {
	env := newEnv(t, offlineInstance())
	conn := &sessionConn{label: "dynamic-default-full", inst: env.inst}

	if _, known := conn.currentInFlight(); known {
		t.Error("a session serving nothing reported a call in flight")
	}

	releaseFirst := conn.holdInFlight(callAttribution{rec: env.recorder, action: "issue.list"})
	attr, known := conn.currentInFlight()
	if !known || attr.action != "issue.list" {
		t.Errorf("one call in flight was reported as %q (known=%t), want issue.list", attr.action, known)
	}

	releaseSecond := conn.holdInFlight(callAttribution{rec: env.recorder, action: "issue.get"})
	if _, known = conn.currentInFlight(); known {
		t.Error("two calls in flight were attributed to one of them")
	}

	releaseSecond()
	releaseFirst()
	if _, known = conn.currentInFlight(); known {
		t.Error("a released call is still reported as in flight")
	}
}

// TestSubscriberIndex_RecordsEveryWatcherAndForgetsTheReleasedOne checks the
// index that says whose record a resource-updated notification belongs in.
func TestSubscriberIndex_RecordsEveryWatcherAndForgetsTheReleasedOne(t *testing.T) {
	env := newEnv(t, offlineInstance())
	other := newEnvRecorder(env)
	index := newSubscriberIndex()

	releaseFirst := index.add("gitlab://project/7", env.recorder)
	releaseSecond := index.add("gitlab://project/7", other)

	if watchers := index.recordersFor("gitlab://project/7"); len(watchers) != 2 {
		t.Errorf("the index holds %d watchers, want both tests watching one resource", len(watchers))
	}
	if watchers := index.recordersFor("gitlab://project/8"); len(watchers) != 0 {
		t.Errorf("a resource nobody watches has %d watchers", len(watchers))
	}

	releaseSecond()
	if watchers := index.recordersFor("gitlab://project/7"); len(watchers) != 1 || watchers[0] != env.recorder {
		t.Errorf("the wrong watcher was dropped: %d left", len(watchers))
	}
	releaseFirst()
	if watchers := index.recordersFor("gitlab://project/7"); len(watchers) != 0 {
		t.Errorf("the last watcher was not dropped: %d left", len(watchers))
	}
}

// TestElicitationKeys_NamesTheFieldsAskedFor covers what an elicitation record
// carries about the request.
func TestElicitationKeys_NamesTheFieldsAskedFor(t *testing.T) {
	cases := []struct {
		name   string
		schema any
		want   string
	}{
		{
			name:   "a form schema",
			schema: map[string]any{"properties": map[string]any{"title": map[string]any{}, "confirm": map[string]any{}}},
			want:   "confirm,title",
		},
		{name: "a schema with no properties", schema: map[string]any{"type": "object"}, want: ""},
		{name: "no schema at all", schema: nil, want: ""},
		{name: "something that is not a schema", schema: "confirm?", want: ""},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := strings.Join(elicitationKeys(testCase.schema), ","); got != testCase.want {
				t.Errorf("keys = %q, want %q", got, testCase.want)
			}
		})
	}
}

// TestSeedNames_ReadsTheListTheProvisioningScriptPublishes pins the one
// setting the fixture profile reads that the harness does not create itself.
func TestSeedNames_ReadsTheListTheProvisioningScriptPublishes(t *testing.T) {
	cases := []struct {
		name       string
		configured string
		want       string
	}{
		{name: "a list", configured: "registry-image,pending-user", want: "pending-user,registry-image"},
		{name: "padded and repeated", configured: " a , a ,b ", want: "a,b"},
		{name: "nothing provisioned", configured: "", want: ""},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := strings.Join(seedNames(testCase.configured), ","); got != testCase.want {
				t.Errorf("seeds = %q, want %q", got, testCase.want)
			}
		})
	}
}

// TestTestStatus_NamesHowATestEnded checks the field a report divides by: a
// call made by a test that failed is not coverage.
func TestTestStatus_NamesHowATestEnded(t *testing.T) {
	if got := testStatus(t); got != e2ecalls.StatusPassed {
		t.Errorf("a test that has not failed is %q, want passed", got)
	}

	t.Run("skipped", func(t *testing.T) {
		t.Cleanup(func() {
			if got := testStatus(t); got != e2ecalls.StatusSkipped {
				t.Errorf("a skipped test is %q, want skipped", got)
			}
		})
		t.Skip("on purpose, to read the status back")
	})
}

// TestRecorder_FlushedTwice_WritesOnce checks that a test whose record was
// read by an assertion is not written again by its own cleanup.
func TestRecorder_FlushedTwice_WritesOnce(t *testing.T) {
	env := newEnv(t, offlineInstance())
	env.recorder.record(fabricatedCall(env, "issue.list"))

	first := env.recorder.finish(&capturedReporter{}, e2ecalls.StatusPassed)
	second := env.recorder.finish(&capturedReporter{}, e2ecalls.StatusPassed)

	if len(first) == 0 {
		t.Fatal("the first flush wrote nothing")
	}
	if len(second) != 0 {
		t.Errorf("the second flush wrote %d lines, want none", len(second))
	}
}

// TestRecorder_LateArrival_IsDropped checks that a notification reaching a
// test that has already been written does not produce a line with a status
// nothing else of that test carries.
func TestRecorder_LateArrival_IsDropped(t *testing.T) {
	env := newEnv(t, offlineInstance())

	env.recorder.finish(&capturedReporter{}, e2ecalls.StatusPassed)
	env.recorder.record(fabricatedCall(env, "issue.list"))
	env.recorder.noteSkip("too late to matter")

	if lines := env.recorder.finish(&capturedReporter{}, e2ecalls.StatusPassed); len(lines) != 0 {
		t.Errorf("a record written after the flush produced %d lines", len(lines))
	}
}

// TestRunLine_RefusedRun_SaysSoWithItsReason pins that a package which refused
// its runtime is written down rather than left silent.
//
// A missing run line and a run that refused to start are different answers to
// "was this runtime exercised", and a release gate that could not tell them
// apart would pass on a shard nothing produced.
func TestRunLine_RefusedRun_SaysSoWithItsReason(t *testing.T) {
	refused := &runState{
		settings:    testSettings(map[string]string{envCommit: "abc1234"}),
		requirement: Licensed,
		pkg:         "ee",
		runID:       "run-1",
		kind:        refusalMismatch,
		refusal:     "this GitLab has no license",
	}

	run := runLine(refused)

	if run.Status != e2ecalls.RunRefused {
		t.Errorf("status = %q, want refused", run.Status)
	}
	if run.Reason != "this GitLab has no license" {
		t.Errorf("reason = %q, want the refusal", run.Reason)
	}
	if run.Requirement != "licensed" {
		t.Errorf("requirement = %q, want licensed", run.Requirement)
	}
	if run.Commit != "abc1234" {
		t.Errorf("commit = %q, want the revision under test", run.Commit)
	}
}

// TestRunLine_StartedRun_CarriesWhatTheRuntimeWas checks the denominator every
// other line is read against: an action means nothing without the edition and
// tier it was served at.
func TestRunLine_StartedRun_CarriesWhatTheRuntimeWas(t *testing.T) {
	inst := offlineInstance()
	inst.facts = runtimeFacts{
		URL: "http://gitlab.test", Version: "18.1.0", Enterprise: true,
		Tier: edition.Ultimate, TierConfirmed: true,
	}
	inst.runner = true
	inst.runnerOnce.Do(func() {})
	started := &runState{settings: inst.settings, requirement: Any, pkg: "common", runID: "run-2", inst: inst}

	run := runLine(started)

	if run.Status != e2ecalls.RunStarted {
		t.Errorf("status = %q, want started", run.Status)
	}
	if run.Edition != "enterprise" || run.Tier != edition.Ultimate.String() || !run.TierConfirmed {
		t.Errorf("runtime = %q/%q (confirmed=%t), want enterprise/ultimate confirmed",
			run.Edition, run.Tier, run.TierConfirmed)
	}
	if run.GitLabVersion != "18.1.0" {
		t.Errorf("version = %q, want 18.1.0", run.GitLabVersion)
	}
	if !run.Fixtures.Runner {
		t.Error("the fixture profile does not record the runner this run had")
	}
}

// TestAwaitDispatch_NothingToWaitFor_ReturnsAtOnce checks that a flush only
// waits when there is a span it could be waiting for.
//
// A test that made no traced call must not pay the dispatch budget, and
// neither must a call the harness could not stamp: both would add ten seconds
// per test to a suite of hundreds.
func TestAwaitDispatch_NothingToWaitFor_ReturnsAtOnce(t *testing.T) {
	cases := []struct {
		name  string
		calls []*pendingCall
	}{
		{name: "no calls at all", calls: nil},
		{name: "a call with no trace", calls: []*pendingCall{{line: &e2ecalls.Call{Action: "issue.list"}}}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			started := time.Now()

			awaitDispatch(testCase.calls)

			if waited := time.Since(started); waited > time.Second {
				t.Errorf("the flush waited %s with no span to wait for", waited)
			}
		})
	}
}

// TestSessionLines_NameWhatEachSessionServed covers the denominator of the
// whole report.
//
// An action no session served is absent rather than untested, and those are
// different findings about different things. The session line is where that
// distinction comes from, so it has to carry what the session actually listed
// rather than what the catalog says it might have.
func TestSessionLines_NameWhatEachSessionServed(t *testing.T) {
	inst := stubInstance(t)
	env := newEnv(t, inst)
	session := env.Session(ServerConfig{Surface: SurfaceDynamic, Private: true})

	for _, line := range sessionLines() {
		recorded, isSession := line.(*e2ecalls.Session)
		if !isSession || recorded.Label != session.Label() {
			continue
		}
		if recorded.Surface != string(SurfaceDynamic) || recorded.Mode != string(ModeDefault) {
			t.Errorf("the session line says %s/%s, want dynamic/default", recorded.Surface, recorded.Mode)
		}
		if recorded.Transport != string(TransportStdio) {
			t.Errorf("transport = %q, want stdio", recorded.Transport)
		}
		if len(recorded.Tools) == 0 {
			t.Error("the session line lists no tools, so nothing can be divided by it")
		}
		if len(recorded.Prompts) == 0 {
			t.Error("the session line lists no prompts, and the full capability surface serves them")
		}
		return
	}
	t.Errorf("no session line names %s; the sessions a run started are what its coverage is measured against",
		session.Label())
}

// TestFlushRunRecords_WritesTheLinesThatBelongToThePackage covers the one
// write that happens after the last test.
//
// The run line and the session lines are written there rather than per test,
// because a session's dispatch-observed flag has only settled once every test
// is done: writing one per test would write two versions of the same session,
// one before its first span arrived and one after.
func TestFlushRunRecords_WritesTheLinesThatBelongToThePackage(t *testing.T) {
	dir := recordInto(t)

	flushRunRecords()
	e2ecalls.Release()

	records, err := e2ecalls.Read(dir)
	if err != nil {
		t.Fatalf("reading the shard back: %v", err)
	}
	for _, record := range records {
		if record.Run != nil {
			return
		}
	}
	t.Errorf("the shard holds no run line; %d records were written", len(records))
}
