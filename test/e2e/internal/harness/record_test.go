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
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
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

// TestAssertDispatch_RewriteFailsAScenarioButNotASweep checks the one
// exemption the dispatch assertion makes: a scenario call whose server span
// names another route fails, and a sweep call in the same shape does not.
//
// A sweep probes what a session serves with a non-constant id and is credited
// to the action the server said it ran, so a rewrite under it is credited
// correctly and asserts nothing false; failing it would only make a broad
// probe brittle against a rewrite it never claimed anything about.
func TestAssertDispatch_RewriteFailsAScenarioButNotASweep(t *testing.T) {
	ran := dispatchRecord{action: "environment.protected_get"}

	cases := []struct {
		purpose  Purpose
		wantFail bool
	}{
		{purpose: PurposeTest, wantFail: true},
		{purpose: PurposeSweep, wantFail: false},
	}
	for _, testCase := range cases {
		t.Run(string(testCase.purpose), func(t *testing.T) {
			reporter := &capturedReporter{}
			call := &pendingCall{line: &e2ecalls.Call{
				Action:  "environment.get",
				Surface: string(SurfaceMeta),
				Purpose: string(testCase.purpose),
			}}

			assertDispatch(reporter, call, ran)

			if failed := reporter.count() > 0; failed != testCase.wantFail {
				t.Errorf("assertDispatch reported %d failures (%q), want a failure = %t",
					reporter.count(), reporter.reported(), testCase.wantFail)
			}
		})
	}
}

// TestDispatchLine_CarriesTheRequestsTheTraceMade checks the one fact on this
// line that comes from a span other than the server's own.
//
// It is what lets a reader ask per action what the committed request inventory
// can only answer per package, so a line that dropped it would leave the
// question unanswerable while looking complete. The empty case is here beside
// it because the field is omitempty: an action that reached no GitLab must
// write no count rather than a zero that reads as a measurement.
func TestDispatchLine_CarriesTheRequestsTheTraceMade(t *testing.T) {
	cases := []struct {
		name string
		kept traceSpans
		want int
	}{
		{
			name: "a handler that called GitLab",
			kept: traceSpans{dispatch: dispatchRecord{action: "issue.list"}, requests: 3},
			want: 3,
		},
		{
			name: "a refusal that called nobody",
			kept: traceSpans{dispatch: dispatchRecord{action: "issue.delete", refusalReason: "safe_mode"}},
			want: 0,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			line := dispatchLine(testTraceID, testCase.kept)

			if line.Requests != testCase.want {
				t.Errorf("the dispatch line reports %d requests, want %d", line.Requests, testCase.want)
			}
			if line.Action != testCase.kept.dispatch.action {
				t.Errorf("the dispatch line names %q, want %q", line.Action, testCase.kept.dispatch.action)
			}
		})
	}
}

// TestDispatchLinesOf_ATraceWithNoServerSpan_WritesNoLine covers the skip that
// keeps a record's two readings of one trace agreeing.
//
// A trace whose GitLab client spans landed and whose server span did not names
// no action. Written anyway, it is a dispatch line with an empty action: the
// coverage command counts it among its dispatch lines and then skips it when it
// joins, so its diagnostics and its joins disagree and nothing says why. The
// other trace here is what makes the assertion about the skip rather than about
// an empty receiver.
func TestDispatchLinesOf_ATraceWithNoServerSpan_WritesNoLine(t *testing.T) {
	lines := dispatchLinesOf(map[string]traceSpans{
		"4bf92f3577b34da6a3ce929d0e0e4731": {requests: 2},
		"4bf92f3577b34da6a3ce929d0e0e4732": {dispatch: dispatchRecord{action: "issue.list"}, requests: 1},
	})

	if len(lines) != 1 {
		t.Fatalf("dispatchLinesOf() wrote %d line(s), want only the trace the server spoke about", len(lines))
	}
	dispatch, isDispatch := lines[0].(*e2ecalls.Dispatch)
	if !isDispatch {
		t.Fatalf("dispatchLinesOf() wrote a %T, want a dispatch line", lines[0])
	}
	if dispatch.Action != "issue.list" || dispatch.Requests != 1 {
		t.Errorf("the line is %+v, want issue.list with its one request", dispatch)
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

// TestSubscriberIndex_OneWatcherPerURI_TurnsTheSecondAwayUntilReleased checks
// the index that says whose record a resource-updated notification belongs in,
// and that it holds one test per URI: the SDK keeps one subscription per URI
// and per session, so a second test would share the first one's.
func TestSubscriberIndex_OneWatcherPerURI_TurnsTheSecondAwayUntilReleased(t *testing.T) {
	env := newEnv(t, offlineInstance())
	other := newEnvRecorder(env)
	index := newSubscriberIndex()

	release, claimed := index.claim("gitlab://project/7", env.recorder)
	if !claimed {
		t.Fatal("the first claim on a URI nobody watches was turned away")
	}
	if _, secondClaimed := index.claim("gitlab://project/7", other); secondClaimed {
		t.Error("a second test was let watch a URI another test already watches on this session")
	}
	if rec, watched := index.recorderFor("gitlab://project/7"); !watched || rec != env.recorder {
		t.Errorf("recorderFor = (%p, %t), want the first test's recorder, which the refused claim must not replace", rec, watched)
	}
	if _, watched := index.recorderFor("gitlab://project/8"); watched {
		t.Error("a resource nobody watches has a watcher")
	}
	otherRelease, otherClaimed := index.claim("gitlab://project/8", other)
	if !otherClaimed {
		t.Fatal("a claim on another URI was turned away")
	}

	release()
	if _, watched := index.recorderFor("gitlab://project/7"); watched {
		t.Error("a released watcher is still recorded")
	}
	if rec, watched := index.recorderFor("gitlab://project/8"); !watched || rec != other {
		t.Error("releasing one URI dropped the watcher of another")
	}
	if _, reclaimed := index.claim("gitlab://project/7", other); !reclaimed {
		t.Error("a URI whose watcher was released cannot be claimed again")
	}
	otherRelease()
}

// TestRecordSubscribe_CreditsTheSubscribeToItsTest checks that the verb's own
// record names the resource, the test and what came of the subscribe, since
// the sending middleware cannot: the SDK opens the subscription on a
// background context under the current protocol, so the attribution never
// reaches it. The outcome is the caller's, which is what lets a refused
// subscribe be recorded as the refusal it was.
func TestRecordSubscribe_CreditsTheSubscribeToItsTest(t *testing.T) {
	env := newEnv(t, offlineInstance())
	conn := &sessionConn{
		label: "dynamic-default-full",
		inst:  env.inst,
		cfg:   ServerConfig{Surface: SurfaceDynamic, Mode: ModeDefault, Capabilities: CapabilitiesFull},
	}

	conn.recordSubscribe(env.recorder, "gitlab://project/7", ExpectationAny, e2ecalls.OutcomeProtocolError)

	lines := env.recorder.finish(&capturedReporter{}, e2ecalls.StatusPassed)
	var found *e2ecalls.Call
	for _, line := range lines {
		if record, isCall := line.(*e2ecalls.Call); isCall {
			found = record
		}
	}
	if found == nil {
		t.Fatalf("recordSubscribe wrote no call line; %d lines written", len(lines))
	}
	if found.Method != methodSubscribe || found.Target != "gitlab://project/7" {
		t.Errorf("subscribe line = method %q target %q, want %q and gitlab://project/7", found.Method, found.Target, methodSubscribe)
	}
	if found.Test != env.T.Name() || found.TestStatus != e2ecalls.StatusPassed || found.Purpose != string(PurposeTest) {
		t.Errorf("subscribe line credited test %q status %q purpose %q, want this test, passed, test",
			found.Test, found.TestStatus, found.Purpose)
	}
	if found.Outcome != e2ecalls.OutcomeProtocolError || found.Expectation != ExpectationAny {
		t.Errorf("subscribe line outcome %q expectation %q, want the caller's %q and %q",
			found.Outcome, found.Expectation, e2ecalls.OutcomeProtocolError, ExpectationAny)
	}
	if found.Session != "dynamic-default-full" || found.Surface != string(SurfaceDynamic) {
		t.Errorf("subscribe line names session %q on %q, want the session it was made on", found.Session, found.Surface)
	}
	if found.Requirement != Any.token() {
		t.Errorf("subscribe line carries requirement %q, want the instance's %q", found.Requirement, Any.token())
	}
}

// TestEnvRecorder_Record_NilCallOrRecorder_RecordsNothing checks the two
// guards on the buffer: a call that is not there is not buffered, and a test
// with no recorder takes a call without failing, since both reach it from the
// SDK's goroutines where a panic would take the whole run down.
func TestEnvRecorder_Record_NilCallOrRecorder_RecordsNothing(t *testing.T) {
	env := newEnv(t, offlineInstance())
	var none *envRecorder

	env.recorder.record(nil)
	none.record(&pendingCall{line: &e2ecalls.Call{Method: methodReadResource}})

	if lines := env.recorder.finish(&capturedReporter{}, e2ecalls.StatusPassed); len(lines) != 0 {
		t.Errorf("finish() wrote %d lines, want none from a nil call", len(lines))
	}
}

// TestRecordSending_AttributionWithoutARecorder_IsPassedThrough checks that a
// request attributed to no recorder is sent as it came: neither stamped with a
// trace nor recorded, since there is no test to file it under.
func TestRecordSending_AttributionWithoutARecorder_IsPassedThrough(t *testing.T) {
	conn := &sessionConn{}
	sent := 0
	send := conn.recordSending()(func(context.Context, string, mcp.Request) (mcp.Result, error) {
		sent++
		return &mcp.CallToolResult{}, nil
	})
	params := &mcp.CallToolParams{Name: "gitlab_issue_list"}
	ctx := withAttribution(t.Context(), callAttribution{purpose: PurposeTest})

	if _, err := send(ctx, methodCallTool, &mcp.ClientRequest[*mcp.CallToolParams]{Params: params}); err != nil {
		t.Fatalf("the middleware answered %v, want the call passed through", err)
	}
	if sent != 1 {
		t.Errorf("the call reached the transport %d times, want once", sent)
	}
	if _, stamped := params.GetMeta()[traceParentKey]; stamped {
		t.Error("a call attributed to no recorder was stamped with a trace nothing will join")
	}
}

// TestDescribeToolRequest_NotAToolCall_NamesNothing checks the requests that
// carry no tool: another method's params, and a tool call whose params are a
// typed nil, which the SDK hands a middleware and which must not be read.
func TestDescribeToolRequest_NotAToolCall_NamesNothing(t *testing.T) {
	cases := []struct {
		name string
		req  mcp.Request
	}{
		{name: "another method", req: &mcp.ClientRequest[*mcp.ReadResourceParams]{Params: &mcp.ReadResourceParams{URI: "gitlab://tools"}}},
		{name: "typed nil params", req: &mcp.ClientRequest[*mcp.CallToolParams]{}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if tool, arguments := describeToolRequest(testCase.req); tool != "" || arguments != nil {
				t.Errorf("describeToolRequest() = %q, %v; want nothing", tool, arguments)
			}
		})
	}
}

// TestRecordElicitation_NoParams_RecordsNothing checks that an elicitation
// whose params are a typed nil is passed over rather than read, with a call in
// flight to attribute it to.
func TestRecordElicitation_NoParams_RecordsNothing(t *testing.T) {
	env := newEnv(t, offlineInstance())
	conn := &sessionConn{label: "dynamic-default-full", inst: env.inst}
	release := conn.holdInFlight(callAttribution{rec: env.recorder, purpose: PurposeTest})
	defer release()

	conn.recordElicitation(&mcp.ElicitRequest{})

	if lines := env.recorder.finish(&capturedReporter{}, e2ecalls.StatusPassed); len(lines) != 0 {
		t.Errorf("recordElicitation() wrote %d lines for an elicitation with no params, want none", len(lines))
	}
}

// TestFixtureProfile_Settings_RecordWhatTheRuntimeHad checks each flag of the
// fixture profile against a runtime that had some fixtures and not others, so
// a flag read the wrong way round is one that disagrees.
func TestFixtureProfile_Settings_RecordWhatTheRuntimeHad(t *testing.T) {
	inst := offlineInstance()
	inst.settings = testSettings(map[string]string{
		envGitLabURL: "http://gitlab.test", envGitLabToken: stubToken,
		envFixtureURL: "http://fixtures.test", envGitHubToken: "ghp-test", envSeeds: "users, groups,users",
	})
	inst.runnerOnce.Do(func() {})

	got := fixtureProfile(inst)

	want := e2ecalls.FixtureProfile{FixtureService: true, GHToken: true, Seeds: []string{"groups", "users"}}
	if got.Runner != want.Runner || got.FixtureService != want.FixtureService || got.Bitbucket != want.Bitbucket ||
		got.GHToken != want.GHToken || !slices.Equal(got.Seeds, want.Seeds) {
		t.Errorf("fixtureProfile() = %+v, want %+v", got, want)
	}
}

// TestRecordReceiving_Acknowledgement_WakesOnlyTheURIsItNames checks the one
// place a subscribe's answer can be seen on protocol 2026-07-28: an
// acknowledgement wakes whoever waits on each URI it names and nobody else,
// the same payload under another method wakes nobody, and a payload of another
// shape or none is passed over, while the chain goes on to the SDK's own
// handler every time.
func TestRecordReceiving_Acknowledgement_WakesOnlyTheURIsItNames(t *testing.T) {
	conn := &sessionConn{acks: newUpdateNotifier(), notifier: newUpdateNotifier(), subscribers: newSubscriberIndex()}
	named := conn.acks.watch("gitlab://project/7")
	other := conn.acks.watch("gitlab://project/8")
	passed := 0
	receive := conn.recordReceiving()(func(context.Context, string, mcp.Request) (mcp.Result, error) {
		passed++
		return nil, nil //nolint:nilnil // a notification has no result, which is what the SDK's own handler answers it with
	})
	ack := &mcp.ClientRequest[*mcp.SubscriptionsAcknowledgedParams]{Params: &mcp.SubscriptionsAcknowledgedParams{
		Notifications: mcp.NotificationSubscriptions{ResourceSubscriptions: []string{"gitlab://project/7"}},
	}}

	_, _ = receive(t.Context(), methodSubscriptionsAcknowledged, ack)
	select {
	case <-named:
	default:
		t.Error("the URI the acknowledgement names was not woken")
	}
	select {
	case <-other:
		t.Error("a URI the acknowledgement does not name was woken")
	default:
	}

	quiet := []struct {
		name   string
		method string
		req    mcp.Request
	}{
		{name: "another method", method: methodResourceUpdated, req: ack},
		{
			name: "another shape", method: methodSubscriptionsAcknowledged,
			req: &mcp.ClientRequest[*mcp.ResourceUpdatedNotificationParams]{Params: &mcp.ResourceUpdatedNotificationParams{URI: "gitlab://project/7"}},
		},
		{name: "no params", method: methodSubscriptionsAcknowledged, req: &mcp.ClientRequest[*mcp.SubscriptionsAcknowledgedParams]{}},
	}
	for _, testCase := range quiet {
		t.Run(testCase.name, func(t *testing.T) {
			_, _ = receive(t.Context(), testCase.method, testCase.req)
			select {
			case <-named:
				t.Error("the URI was woken by something that is not an acknowledgement of it")
			default:
			}
		})
	}
	if passed != 1+len(quiet) {
		t.Errorf("the chain went on %d times, want every one of the %d notifications passed to the SDK", passed, 1+len(quiet))
	}
}

// TestRecordReceiving_ResourceUpdate_IsRecordedAgainstItsWatcher checks the
// delivery half of a subscription's record: an update for a URI a test watches
// is written against that test, and one for a URI nobody watches any more, or
// a payload of another shape, is written nowhere.
func TestRecordReceiving_ResourceUpdate_IsRecordedAgainstItsWatcher(t *testing.T) {
	env := newEnv(t, offlineInstance())
	conn := &sessionConn{
		label: "dynamic-default-full", inst: env.inst,
		cfg:      ServerConfig{Surface: SurfaceDynamic, Mode: ModeDefault, Capabilities: CapabilitiesFull},
		notifier: newUpdateNotifier(), acks: newUpdateNotifier(), subscribers: newSubscriberIndex(),
	}
	if _, claimed := conn.subscribers.claim("gitlab://project/7", env.recorder); !claimed {
		t.Fatal("the claim on an unwatched URI was turned away")
	}
	receive := conn.recordReceiving()(func(context.Context, string, mcp.Request) (mcp.Result, error) {
		return nil, nil //nolint:nilnil // a notification has no result, which is what the SDK's own handler answers it with
	})
	update := func(uri string) mcp.Request {
		return &mcp.ClientRequest[*mcp.ResourceUpdatedNotificationParams]{Params: &mcp.ResourceUpdatedNotificationParams{URI: uri}}
	}

	_, _ = receive(t.Context(), methodResourceUpdated, update("gitlab://project/7"))
	_, _ = receive(t.Context(), methodResourceUpdated, update("gitlab://project/8"))
	_, _ = receive(t.Context(), methodResourceUpdated, &mcp.ClientRequest[*mcp.SubscriptionsAcknowledgedParams]{})
	_, _ = receive(t.Context(), methodResourceUpdated, &mcp.ClientRequest[*mcp.ResourceUpdatedNotificationParams]{})

	var updates []*e2ecalls.Call
	for _, line := range env.recorder.finish(&capturedReporter{}, e2ecalls.StatusPassed) {
		if call, isCall := line.(*e2ecalls.Call); isCall {
			updates = append(updates, call)
		}
	}
	if len(updates) != 1 {
		t.Fatalf("recorded %d lines, want the one update of the watched resource", len(updates))
	}
	got := updates[0]
	if got.Method != methodResourceUpdated || got.Target != "gitlab://project/7" || got.Test != env.T.Name() ||
		got.Outcome != e2ecalls.OutcomeOK || got.Session != "dynamic-default-full" {
		t.Errorf("update line = %+v, want the watched resource's update credited to this test on its session", got)
	}
}

// TestSessionLines_Completions_AreSpelledAsTheCompletionCallsNameThem checks
// the completion denominator end to end against the real binary: a session
// line lists a reference for every prompt argument and template variable the
// session served, spelled exactly as a completion call's record names its
// target, so the coverage command can find each call in the list it divides
// by. The minimal surface serves one template and no prompt, so its list is
// that template's one variable.
func TestSessionLines_Completions_AreSpelledAsTheCompletionCallsNameThem(t *testing.T) {
	inst := stubInstance(t)

	t.Run("full", func(t *testing.T) {
		env := newEnv(t, inst)
		session := env.Session(ServerConfig{Surface: SurfaceDynamic, Private: true})
		completions := sessionLineCompletions(t, session)

		if want := listedCompletionReferences(session); !slices.Equal(completions, want) {
			t.Errorf("the session line lists %d completions, want the %d the session's listings name", len(completions), len(want))
		}

		prompt := firstPromptWithAnArgument(t, session)
		session.CompletePrompt(prompt.Name, prompt.Required[0], "")
		session.CompleteResource("gitlab://project/{project_id}", "project_id", "")
		calls := 0
		for _, line := range env.recorder.finish(&capturedReporter{}, e2ecalls.StatusPassed) {
			if call, isCall := line.(*e2ecalls.Call); isCall && call.Method == methodComplete {
				calls++
				assertCompletionLine(t, call, completions)
			}
		}
		if calls != 2 {
			t.Errorf("recorded %d completion calls, want the two this test made", calls)
		}
	})

	t.Run("minimal", func(t *testing.T) {
		env := newEnv(t, inst)
		session := env.Session(ServerConfig{Surface: SurfaceDynamic, Capabilities: CapabilitiesMinimal, Private: true})
		if got, want := sessionLineCompletions(t, session), []string{"gitlab://tools/{id} id"}; !slices.Equal(got, want) {
			t.Errorf("the minimal session line lists completions %q, want %q", got, want)
		}
	})
}

// listedCompletionReferences spells, independently of the harness's own
// helper, every reference a session's listings offer a completion for: each
// argument of each prompt and each variable of each template, sorted and
// listed once.
func listedCompletionReferences(session *Session) []string {
	var want []string
	for _, spec := range session.PromptSpecs() {
		for _, argument := range append(append([]string{}, spec.Required...), spec.Optional...) {
			want = append(want, spec.Name+" "+argument)
		}
	}
	for _, template := range session.ResourceTemplates() {
		for _, variable := range TemplateVariables(template) {
			want = append(want, template+" "+variable)
		}
	}
	slices.Sort(want)
	return slices.Compact(want)
}

// assertCompletionLine checks one recorded completion call against the list
// the session line counts it in, and that its duration is spelled in
// milliseconds: a completion against a stub takes a few of them, so a
// figure outside that range is a duration in another unit.
func assertCompletionLine(t *testing.T, call *e2ecalls.Call, completions []string) {
	t.Helper()
	if !slices.Contains(completions, call.Target) {
		t.Errorf("a completion was recorded as %q, which the session line's list does not name", call.Target)
	}
	if call.DurationMS <= 0 || call.DurationMS >= 60_000 {
		t.Errorf("the completion of %q was recorded as taking %v ms", call.Target, call.DurationMS)
	}
}

// TestSessionLines_SessionThatNeverStarted_WritesNoLine checks that a pool
// entry whose session never connected writes no session line: it served
// nothing, and a line for it would add an empty session to the denominator.
func TestSessionLines_SessionThatNeverStarted_WritesNoLine(t *testing.T) {
	before := len(sessionLines())
	sessions.Store("never-started", &sessionEntry{err: errors.New("the child did not start")})
	sessions.Store("not-an-entry", "a value the pool never stores")
	t.Cleanup(func() {
		sessions.Delete("never-started")
		sessions.Delete("not-an-entry")
	})

	if got := len(sessionLines()); got != before {
		t.Errorf("sessionLines() wrote %d lines, want the %d it wrote before a session that never started was pooled", got, before)
	}
}

// sessionLineCompletions returns the completion list the session line of one
// session carries.
func sessionLineCompletions(t *testing.T, session *Session) []string {
	t.Helper()
	for _, line := range sessionLines() {
		if recorded, isSession := line.(*e2ecalls.Session); isSession && recorded.Label == session.Label() {
			return recorded.Completions
		}
	}
	t.Fatalf("no session line names %s", session.Label())
	return nil
}

// firstPromptWithAnArgument returns a served prompt that requires an argument.
func firstPromptWithAnArgument(t *testing.T, session *Session) PromptSpec {
	t.Helper()
	for _, spec := range session.PromptSpecs() {
		if len(spec.Required) > 0 {
			return spec
		}
	}
	t.Fatal("the full capability session serves no prompt with a required argument")
	return PromptSpec{}
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

// TestRecordingElicitationHandler_RecordsBeforeAnsweringAndKeepsTheAnswer pins
// where an elicitation is written down.
//
// The recorder used to look for one in the receiving middleware, on method
// elicitation/create. The SDK never delivers it there: an elicitation reaches
// ClientOptions.ElicitationHandler instead, so the middleware's case never
// fired and every interactive flow ran uncounted. The coverage report read
// zero elicitations while the flows themselves passed, which is the worst
// shape a gap can take. The wrapper is the fix, and this holds it in place:
// the request is recorded, and the policy's own answer still reaches the
// server unchanged.
func TestRecordingElicitationHandler_RecordsBeforeAnsweringAndKeepsTheAnswer(t *testing.T) {
	conn := &sessionConn{}

	var seen int
	answered := &mcp.ElicitResult{Action: "accept", Content: map[string]any{"title": "from the policy"}}
	wrapped := conn.recordingElicitationHandler(func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		seen++
		return answered, nil
	})
	if wrapped == nil {
		t.Fatal("wrapping a policy handler produced nil, so the session would advertise no elicitation capability")
	}

	got, err := wrapped(t.Context(), &mcp.ElicitRequest{Params: &mcp.ElicitParams{Message: "confirm?"}})
	if err != nil {
		t.Fatalf("the wrapped handler answered an error: %v", err)
	}
	if seen != 1 {
		t.Errorf("the policy handler ran %d times, want exactly 1", seen)
	}
	if got != answered {
		t.Errorf("the wrapper answered %+v, want the policy's own result", got)
	}
}

// TestRecordingElicitationHandler_NoPolicy_StaysNil checks that the wrapper
// does not manufacture a handler.
//
// ElicitationNone means the client advertises no elicitation capability, which
// is what makes the server fail closed instead of prompting. A wrapper that
// returned a non-nil function for a nil policy would advertise the capability
// and quietly turn that scenario into a different one.
func TestRecordingElicitationHandler_NoPolicy_StaysNil(t *testing.T) {
	conn := &sessionConn{}
	if wrapped := conn.recordingElicitationHandler(nil); wrapped != nil {
		t.Error("wrapping no policy produced a handler, so the client would advertise elicitation it cannot serve")
	}
}

// TestAcceptElicitation_FillsWhatTheSchemaRequires pins the auto-accept
// policy against the four shapes this server asks for.
//
// The policy answered with empty content until 2026-09-14, which could never
// work: the SDK validates the accepted content against the requested schema
// before it applies the schema's defaults, and every schema the server sends
// marks its one property required, so an empty accept failed the call with
// InvalidParams instead of approving anything. The cases below are the real
// schemas from internal/elicitation/schemas.go.
func TestAcceptElicitation_FillsWhatTheSchemaRequires(t *testing.T) {
	cases := []struct {
		name   string
		schema any
		want   map[string]any
	}{
		{
			name: "the destructive-action confirmation",
			schema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"confirmed": map[string]any{"type": "boolean", "default": false}},
				"required":   []any{"confirmed"},
			},
			want: map[string]any{"confirmed": true},
		},
		{
			name: "free text",
			schema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"title": map[string]any{"type": "string"}},
				"required":   []any{"title"},
			},
			want: map[string]any{"title": ""},
		},
		{
			name: "one of an enum",
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"selection": map[string]any{"type": "string", "enum": []any{"first", "second"}},
				},
				"required": []any{"selection"},
			},
			want: map[string]any{"selection": "first"},
		},
		{
			// The specification puts a multi-select's values on the item
			// schema, so this is the shape a server that follows it sends.
			name: "many of an enum",
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"selections": map[string]any{
						"type":  "array",
						"items": map[string]any{"type": "string", "enum": []any{"first", "second"}},
					},
				},
				"required": []any{"selections"},
			},
			want: map[string]any{"selections": []any{"first"}},
		},
		{
			// And the shape that puts them on the array itself is answered
			// too rather than declined, since a selection this policy can
			// read is better answered than left empty.
			name: "many of an enum written on the array",
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"selections": map[string]any{"type": "array", "enum": []any{"first", "second"}},
				},
				"required": []any{"selections"},
			},
			want: map[string]any{"selections": []any{"first"}},
		},
		{
			// Nothing to choose from anywhere is an empty selection, never
			// the first of nothing.
			name: "many of nothing",
			schema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"selections": map[string]any{"type": "array"}},
				"required":   []any{"selections"},
			},
			want: map[string]any{"selections": []any{}},
		},
		{
			// An item schema that lists no values says nothing about them, so
			// the array's own list is the one read.
			name: "an empty item list defers to the array's own",
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"selections": map[string]any{
						"type":  "array",
						"enum":  []any{"first"},
						"items": map[string]any{"type": "string", "enum": []any{}},
					},
				},
				"required": []any{"selections"},
			},
			want: map[string]any{"selections": []any{"first"}},
		},
		{
			name:   "a schema this policy cannot read",
			schema: "not an object",
			want:   map[string]any{},
		},
		{
			name: "a property the schema does not describe",
			schema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
				"required":   []any{"mystery"},
			},
			want: map[string]any{"mystery": ""},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			result, err := acceptElicitation(t.Context(), &mcp.ElicitRequest{
				Params: &mcp.ElicitParams{RequestedSchema: testCase.schema},
			})
			if err != nil {
				t.Fatalf("acceptElicitation() error = %v, want nil", err)
			}
			if result.Action != "accept" {
				t.Errorf("action = %q, want accept", result.Action)
			}
			if fmt.Sprint(result.Content) != fmt.Sprint(testCase.want) {
				t.Errorf("content = %v, want %v", result.Content, testCase.want)
			}
		})
	}
}

// TestAcceptElicitation_NoParams_AcceptsWithNothing checks the policy answers
// an acceptance rather than a nil result when there is no schema to read.
func TestAcceptElicitation_NoParams_AcceptsWithNothing(t *testing.T) {
	result, err := acceptElicitation(t.Context(), &mcp.ElicitRequest{})
	if err != nil {
		t.Fatalf("acceptElicitation() error = %v, want nil", err)
	}
	if result.Action != "accept" || len(result.Content) != 0 {
		t.Errorf("acceptElicitation() = %+v, want an empty acceptance", result)
	}
}

// TestProgressCollector_FilesOnlyWhatWasAskedFor pins the collector's rules.
//
// A session is shared by many calls and the server may report progress for one
// this harness never asked about, so a collector that kept everything would
// grow for the life of the session and hand one call another's notifications.
func TestProgressCollector_FilesOnlyWhatWasAskedFor(t *testing.T) {
	collector := newProgressCollector()
	collector.expect("wanted")

	collector.deliver(&mcp.ProgressNotificationParams{ProgressToken: "wanted", Progress: 1, Total: 3, Message: "first"})
	collector.deliver(&mcp.ProgressNotificationParams{ProgressToken: "unasked", Progress: 9})
	collector.deliver(&mcp.ProgressNotificationParams{ProgressToken: "wanted", Progress: 2, Total: 3})
	collector.deliver(&mcp.ProgressNotificationParams{ProgressToken: 42, Progress: 7})
	collector.deliver(nil)

	notes := collector.collect("wanted")
	if len(notes) != 2 {
		t.Fatalf("collected %d note(s), want the 2 filed under the token: %+v", len(notes), notes)
	}
	if notes[0].Progress != 1 || notes[0].Total != 3 || notes[0].Message != "first" {
		t.Errorf("first note = %+v, want the fields the server sent", notes[0])
	}
	if notes[1].Progress != 2 {
		t.Errorf("second note = %+v, want the second delivery", notes[1])
	}
	if again := collector.collect("wanted"); len(again) != 0 {
		t.Errorf("collecting twice answered %+v, want nothing: the token is done", again)
	}
	if unasked := collector.collect("unasked"); len(unasked) != 0 {
		t.Errorf("a token nothing expected collected %+v, want nothing", unasked)
	}
}

// TestProgressCollector_NoNotifications_CollectsEmpty checks that a call which
// asked for progress and got none reads as empty rather than as a failure.
//
// An action that reports no progress is not a broken action; what a scenario
// makes of the emptiness is its own business.
func TestProgressCollector_NoNotifications_CollectsEmpty(t *testing.T) {
	collector := newProgressCollector()
	collector.expect("quiet")
	if notes := collector.collect("quiet"); len(notes) != 0 {
		t.Errorf("collected %+v, want nothing", notes)
	}
}

// TestNewProgressToken_IsUniqueAndRecognisable checks the tokens two calls
// mint cannot collide, since a repeat would mix their notifications.
func TestNewProgressToken_IsUniqueAndRecognisable(t *testing.T) {
	seen := map[string]bool{}
	for range 64 {
		token := newProgressToken()
		if !strings.HasPrefix(token, "progress-") {
			t.Fatalf("token %q does not carry the prefix that identifies it in a record", token)
		}
		if seen[token] {
			t.Fatalf("token %q was minted twice", token)
		}
		seen[token] = true
	}
}
