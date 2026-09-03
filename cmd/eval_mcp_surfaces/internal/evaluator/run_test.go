// run_test.go covers the run-level helpers in run.go that orchestrate the
// evaluator CLI workflow.

package evaluator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestFatalInitialProviderError_DetectsUnavailableProvider verifies that
// fatalInitialProviderError surfaces a non-nil error when the first model
// turn fails before any tool calls are recorded and the provider returned
// HTTP 400/401/403/404.
//
// The test builds a [taskResult] with a 404 from the Google provider, no
// successful tool calls, and one model_error trace event. It asserts that
// the error string contains both the model label and the HTTP status so
// operators can quickly diagnose missing or retired providers.
func TestFatalInitialProviderError_DetectsUnavailableProvider(t *testing.T) {
	result := taskResult{
		Model:      "google:gemini-retired-preview",
		ModelCalls: 1,
		Notes:      []string{`google status 404: {"error":{"message":"model is no longer available"}}`},
		Trace: taskTrace{Events: []traceEvent{{
			Kind:     "model_error",
			IsError:  true,
			Provider: &modelProviderTrace{Provider: providerGoogle, ResponseStatus: http.StatusNotFound},
		}}},
	}

	err := fatalInitialProviderError(result)
	if err == nil {
		t.Fatal("fatalInitialProviderError() error = nil, want unavailable provider error")
	}
	if !strings.Contains(err.Error(), "google:gemini-retired-preview") || !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("fatalInitialProviderError() error = %q, want model and status", err)
	}
}

// TestFatalInitialProviderError_IgnoresModelBehaviorFailures verifies that
// fatalInitialProviderError returns nil when a 404 provider error coexists
// with successful tool calls and completed steps.
//
// The test reuses a Google 404 trace but reports one tool call and one
// completed step on the same task, mirroring a model that encountered a
// transient provider error after making progress. The assertion guards
// against the function aborting the run on a recoverable mid-evaluation
// provider hiccup.
func TestFatalInitialProviderError_IgnoresModelBehaviorFailures(t *testing.T) {
	result := taskResult{
		Model:          "google:gemini",
		ModelCalls:     1,
		ToolCalls:      1,
		CompletedSteps: 1,
		Notes:          []string{"step 1: missing required params: project_id"},
		Trace: taskTrace{Events: []traceEvent{{
			Kind:     "model_error",
			IsError:  true,
			Provider: &modelProviderTrace{Provider: providerGoogle, ResponseStatus: http.StatusNotFound},
		}}},
	}

	if err := fatalInitialProviderError(result); err != nil {
		t.Fatalf("fatalInitialProviderError() error = %v, want nil for task-level failures", err)
	}
}

// TestIsRetriableModelOutputFailure verifies that only malformed model tool-call
// output (empty/invalid arguments) is treated as retriable, while successes and
// genuine wrong-choice / execution failures are not.
func TestIsRetriableModelOutputFailure(t *testing.T) {
	withModelError := func(content string) taskResult {
		r := taskResult{}
		r.Trace.Events = []traceEvent{{Kind: "model_error", Content: content, IsError: true}}
		return r
	}
	cases := []struct {
		name   string
		result taskResult
		want   bool
	}{
		{"invalid args", withModelError("gitlab_execute_action tool call call_1 " + markerInvalidToolArgs + ": invalid character '<'"), true},
		{"empty args", withModelError("gitlab_execute_action tool call call_1 " + markerEmptyToolArgs), true},
		{"success not retried", taskResult{FinalSuccess: true}, false},
		{"wrong choice not retried", func() taskResult {
			r := taskResult{}
			r.Notes = []string{"expected action issue.create but model called issue.list"}
			r.Trace.Events = []traceEvent{{Kind: "validation", Content: "action mismatch"}}
			return r
		}(), false},
		{"gitlab execution error not retried", func() taskResult {
			r := taskResult{}
			r.Trace.Events = []traceEvent{{Kind: "tool_result", Content: "404 Project Not Found", IsError: true}}
			return r
		}(), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isRetriableModelOutputFailure(tc.result); got != tc.want {
				t.Fatalf("isRetriableModelOutputFailure() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestTaskAttemptPreparationErrorResult_RecordsReportableFailure verifies
// fixture setup failures become task rows instead of aborting a full preset.
func TestTaskAttemptPreparationErrorResult_RecordsReportableFailure(t *testing.T) {
	task := evalTask{ID: "MT-017", ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "merge_request.merge"}
	result := taskAttemptPreparationErrorResult(task, modelSpec{Provider: providerOpenAI, Model: "gpt-test"}, "dynamic", 2, errors.New("fixture timed out"))

	if result.FinalSuccess || result.FirstPass || !result.DestructiveSafe {
		t.Fatalf("result = %+v, want failed but destructive-safe fixture preparation result", result)
	}
	if result.Model != "openai:gpt-test" || result.Run != 2 || result.FirstAction != "merge_request.merge" || result.FinalAction != "merge_request.merge" {
		t.Fatalf("result = %+v, want model/run/action metadata preserved", result)
	}
	if len(result.Notes) != 1 || !strings.Contains(result.Notes[0], "fixture timed out") {
		t.Fatalf("notes = %#v, want fixture error note", result.Notes)
	}
	if result.Trace.Summary.FinalSuccess || result.Trace.Events[len(result.Trace.Events)-1].Kind != "fixture_error" {
		t.Fatalf("trace = %+v, want fixture_error trace summary", result.Trace)
	}
}

// TestPrepareTaskAttemptValue_PreservesNormalizedSteps verifies typed fixture
// preparation keeps catalog-normalized expectations for the selected surface.
func TestPrepareTaskAttemptValue_PreservesNormalizedSteps(t *testing.T) {
	task := taskFromCase(EvalCase{
		ID:     "MT-NORMALIZED",
		Prompt: "Merge the fixture merge request.",
		Steps: []ExpectedStep{{
			ExpectedTool:   "gitlab_merge_request",
			ExpectedAction: "merge",
		}},
		Fixtures: []CaseFixtureSpec{{
			Name:    "noop",
			Scope:   FixtureScopeAttempt,
			Outputs: []string{"project_id"},
			Ensure: func(context.Context, FixtureContext) (FixtureOutput, error) {
				return FixtureOutput{"project_id": "my-org/project"}, nil
			},
		}},
	})
	task.Steps = []evalStep{{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "merge_request.merge"}}

	attempt, err := prepareTaskAttemptValue(t.Context(), options{Execute: true, UseFixtures: true, ToolSurface: config.ToolSurfaceDynamic}, modelSpec{Provider: "fixture", Model: "smoke"}, 1, task, evaluationRuntime{}, "run")
	if err != nil {
		t.Fatalf("prepareTaskAttemptValue() error = %v", err)
	}
	steps := attempt.PreparedCase().Steps
	if len(steps) != 1 || steps[0].ExpectedTool != dynamicExecuteActionTool || steps[0].ExpectedAction != "merge_request.merge" {
		t.Fatalf("prepared steps = %+v, want dynamic execute action expectation", steps)
	}
}

// fakeProviderClient returns an *http.Client whose transport answers every
// provider request with the next scripted Anthropic-shaped response, so the
// evaluation loop can run end to end without a provider API. Requests beyond
// the script get the last response again, which keeps a retry loop from
// dying on an exhausted script.
func fakeProviderClient(t *testing.T, responses ...modelResponse) *http.Client {
	t.Helper()
	var index int
	return &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		response := responses[min(index, len(responses)-1)]
		index++
		body, err := json.Marshal(response)
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body))}, nil
	})}
}

// TestRunModelEvaluations_FakeProvider_ProducesOneResultPerTaskRunAndModel
// verifies the evaluation loop walks every model, run and task, labels each
// result with its model and run index, and fills the trace summary. It drives
// the loop through evaluationRuntime.providerClient with canned tool calls, so
// no provider API and no GitLab are involved.
func TestRunModelEvaluations_FakeProvider_ProducesOneResultPerTaskRunAndModel(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	tasks := []evalTask{
		{ID: "MT-1", Prompt: "Get project `my-org/app`.", Steps: []evalStep{{ExpectedTool: "gitlab_project", ExpectedAction: "get", RequiredParams: []string{"project_id"}}}},
		{ID: "MT-2", Prompt: "Get project `my-org/other`.", Steps: []evalStep{{ExpectedTool: "gitlab_project", ExpectedAction: "get", RequiredParams: []string{"project_id"}}}},
	}
	routes := map[string]toolutil.ActionMap{"gitlab_project": {"get": projectGetRoute()}}
	call := toolUseResponse("call", "gitlab_project", map[string]any{"action": "get", "params": map[string]any{"project_id": "my-org/app"}})
	runtime := evaluationRuntime{
		opts:           options{ToolSurface: config.ToolSurfaceMeta, Repeat: 2, MaxTokens: 64},
		providerClient: fakeProviderClient(t, call),
	}
	run := modelEvaluationRun{opts: runtime.opts, tasks: tasks, routes: routes, runtime: runtime}

	results, err := runModelEvaluations(t.Context(), run, []modelSpec{{Provider: providerAnthropic, Model: "one"}, {Provider: providerAnthropic, Model: "two"}})
	if err != nil {
		t.Fatalf("runModelEvaluations() error = %v", err)
	}
	if len(results) != 8 {
		t.Fatalf("results = %d, want 8 (2 models x 2 runs x 2 tasks)", len(results))
	}
	seen := map[string]int{}
	for _, result := range results {
		seen[result.Model]++
		if !result.FinalSuccess || result.Run < 1 || result.Run > 2 {
			t.Fatalf("result = %+v, want success with run index 1 or 2", result)
		}
		if result.Trace.Model != result.Model || !result.Trace.Summary.FinalSuccess {
			t.Fatalf("trace = %+v, want model label and summary", result.Trace)
		}
	}
	if seen["anthropic:one"] != 4 || seen["anthropic:two"] != 4 {
		t.Fatalf("results per model = %v, want four each", seen)
	}
}

// TestRunModelSpecEvaluations_MissingAPIKey_ReturnsCredentialError verifies
// the loop refuses to start when the provider credential is absent, before any
// task runs.
func TestRunModelSpecEvaluations_MissingAPIKey_ReturnsCredentialError(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	run := modelEvaluationRun{opts: options{Repeat: 1}, tasks: []evalTask{{ID: "MT-1"}}}
	_, err := runModelSpecEvaluations(t.Context(), run, modelSpec{Provider: providerAnthropic, Model: "x"})
	if err == nil || !strings.Contains(err.Error(), "ANTHROPIC_API_KEY is required") {
		t.Fatalf("runModelSpecEvaluations() error = %v, want missing credential error", err)
	}
}

// TestRunModelEvaluationRound_MalformedOutput_RetriesThenReportsRecovery
// verifies a task that failed only because the model emitted unparseable tool
// arguments is re-run, and the recovery is recorded in the notes.
func TestRunModelEvaluationRound_MalformedOutput_RetriesThenReportsRecovery(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	task := evalTask{ID: "MT-RETRY", Prompt: "Get project `my-org/app`.", Steps: []evalStep{{ExpectedTool: "gitlab_project", ExpectedAction: "get", RequiredParams: []string{"project_id"}}}}
	routes := map[string]toolutil.ActionMap{"gitlab_project": {"get": projectGetRoute()}}
	good := toolUseResponse("call", "gitlab_project", map[string]any{"action": "get", "params": map[string]any{"project_id": "my-org/app"}})
	var attempts int
	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"content":[],"error":{"type":"invalid_request","message":"` + markerInvalidToolArgs + `"}}`))}, nil
		}
		body, err := json.Marshal(good)
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body))}, nil
	})}
	opts := options{ToolSurface: config.ToolSurfaceMeta, Repeat: 1, MaxOutputRetries: 2, MaxTokens: 64}
	runtime := evaluationRuntime{opts: opts, providerClient: client}
	runner, err := newModelRunner(opts, modelSpec{Provider: providerAnthropic, Model: "x"}, runtime)
	if err != nil {
		t.Fatalf("newModelRunner() error = %v", err)
	}
	run := modelEvaluationRun{opts: opts, tasks: []evalTask{task}, routes: routes, runtime: runtime}

	results, err := runModelEvaluationRound(t.Context(), run, modelSpec{Provider: providerAnthropic, Model: "x"}, 1, runner)
	if err != nil {
		t.Fatalf("runModelEvaluationRound() error = %v", err)
	}
	if len(results) != 1 || !results[0].FinalSuccess {
		t.Fatalf("results = %+v, want one recovered result", results)
	}
	if !strings.Contains(strings.Join(results[0].Notes, "; "), "malformed-output retry: 1 attempt(s), recovered") {
		t.Fatalf("notes = %v, want recovery note", results[0].Notes)
	}
}

// TestRunModelEvaluationRound_FirstTaskProviderRejection_AbortsRun verifies a
// provider 401 on the very first task stops the whole round rather than
// filling the report with credential failures.
func TestRunModelEvaluationRound_FirstTaskProviderRejection_AbortsRun(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusUnauthorized, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":{"type":"authentication_error","message":"invalid x-api-key"}}`))}, nil
	})}
	opts := options{ToolSurface: config.ToolSurfaceMeta, Repeat: 1, MaxTokens: 64}
	runtime := evaluationRuntime{opts: opts, providerClient: client}
	runner, err := newModelRunner(opts, modelSpec{Provider: providerAnthropic, Model: "x"}, runtime)
	if err != nil {
		t.Fatalf("newModelRunner() error = %v", err)
	}
	task := evalTask{ID: "MT-1", Steps: []evalStep{{ExpectedTool: "gitlab_project", ExpectedAction: "get"}}}
	run := modelEvaluationRun{opts: opts, tasks: []evalTask{task}, runtime: runtime}

	_, err = runModelEvaluationRound(t.Context(), run, modelSpec{Provider: providerAnthropic, Model: "x"}, 1, runner)
	if err == nil || !strings.Contains(err.Error(), "failed before tool execution with HTTP 401") {
		t.Fatalf("runModelEvaluationRound() error = %v, want fatal provider error", err)
	}
}

// TestEvaluateModelTaskAttempt_FixturePreparationFails_ReturnsReportableResult
// verifies a fixture failure becomes a failed task row rather than aborting
// the run, without ever calling the provider.
func TestEvaluateModelTaskAttempt_FixturePreparationFails_ReturnsReportableResult(t *testing.T) {
	task := taskFromCase(EvalCase{
		ID:     "MT-FIXTURE-FAIL",
		Prompt: "Get project.",
		Steps:  []ExpectedStep{{ExpectedTool: "gitlab_project", ExpectedAction: "get"}},
		Fixtures: []CaseFixtureSpec{{
			Name:    "broken",
			Scope:   FixtureScopeAttempt,
			Outputs: []string{"project_id"},
			Ensure: func(context.Context, FixtureContext) (FixtureOutput, error) {
				return nil, errors.New("fixture exploded")
			},
		}},
	})
	opts := options{Execute: true, UseFixtures: true, ToolSurface: config.ToolSurfaceMeta}
	run := modelEvaluationRun{opts: opts, tasks: []evalTask{task}, runtime: evaluationRuntime{opts: opts}}

	result := evaluateModelTaskAttempt(t.Context(), run, modelSpec{Provider: providerAnthropic, Model: "x"}, 3, task, nil)

	if result.FinalSuccess || result.Run != 3 || !strings.Contains(strings.Join(result.Notes, "; "), "fixture exploded") {
		t.Fatalf("result = %+v, want failed fixture preparation row", result)
	}
	if result.Trace.Events[len(result.Trace.Events)-1].Kind != "fixture_error" {
		t.Fatalf("trace = %+v, want fixture_error event", result.Trace)
	}
}

// TestNewModelRunner_UsesRuntimeProviderClientAndOptions verifies the runner
// inherits the runtime's provider client when one is set and otherwise builds
// its own timed client, and that it carries the run options through.
func TestNewModelRunner_UsesRuntimeProviderClientAndOptions(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	opts := options{ToolSurface: config.ToolSurfaceDynamic, MaxTokens: 42, Retries: 5, RetryWait: time.Second, TraceProviderBodies: true}
	injected := &http.Client{}
	cases := []struct {
		name    string
		runtime evaluationRuntime
		want    *http.Client
	}{
		{name: "injected client", runtime: evaluationRuntime{providerClient: injected}, want: injected},
		{name: "default client", runtime: evaluationRuntime{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner, err := newModelRunner(opts, modelSpec{Provider: providerAnthropic, Model: "m"}, tc.runtime)
			if err != nil {
				t.Fatalf("newModelRunner() error = %v", err)
			}
			if tc.want != nil && runner.client != tc.want {
				t.Fatalf("runner.client = %p, want injected client %p", runner.client, tc.want)
			}
			if tc.want == nil && runner.client.Timeout != 60*time.Second {
				t.Fatalf("runner.client.Timeout = %s, want 60s default", runner.client.Timeout)
			}
			if runner.modelLabel != "anthropic:m" || runner.maxTokens != 42 || runner.retries != 5 || runner.toolSurface != config.ToolSurfaceDynamic || !runner.traceBodies {
				t.Fatalf("runner = %+v, want options carried through", runner)
			}
		})
	}
}

// TestNewEvaluationRuntime_WithoutExecution_ReturnsInertRuntime verifies a
// runtime built with neither execution nor resource exposure opens no session
// and its close is a no-op.
func TestNewEvaluationRuntime_WithoutExecution_ReturnsInertRuntime(t *testing.T) {
	catalog := []modelTool{{Name: "gitlab_project"}}
	runtime, err := newEvaluationRuntime(options{ToolSurface: config.ToolSurfaceMeta}, catalog)
	if err != nil {
		t.Fatalf("newEvaluationRuntime() error = %v", err)
	}
	if runtime.mcpSession != nil || runtime.executionClient != nil || len(runtime.catalog) != 1 || runtime.bridgeSupport.any() {
		t.Fatalf("runtime = %+v, want inert runtime", runtime)
	}
	runtime.close()
}

// TestNewEvaluationRuntime_ExposeResourcesWithToolsFile_SkipsSessionSetup
// verifies a snapshot-backed run does not open a resource lookup session, so
// the bridge stays inactive rather than reaching for a live GitLab.
func TestNewEvaluationRuntime_ExposeResourcesWithToolsFile_SkipsSessionSetup(t *testing.T) {
	runtime, err := newEvaluationRuntime(options{ToolSurface: config.ToolSurfaceMeta, ExposeResources: true, ToolsFile: "tools.json"}, nil)
	if err != nil {
		t.Fatalf("newEvaluationRuntime() error = %v", err)
	}
	defer runtime.close()
	if runtime.mcpSession != nil || runtime.opts.ResourceAccessActive {
		t.Fatalf("runtime = %+v, want no session and inactive resource access", runtime)
	}
}

// TestNewEvaluationRuntime_ExecutionRejected_ReturnsGuardError verifies the
// runtime refuses to build an execution session when the safety guards are
// unmet, so no live GitLab call is attempted.
func TestNewEvaluationRuntime_ExecutionRejected_ReturnsGuardError(t *testing.T) {
	t.Setenv("E2E_MODE", "")
	_, err := newEvaluationRuntime(options{ToolSurface: config.ToolSurfaceMeta, Execute: true, Backend: backendGitLab}, nil)
	if err == nil || !strings.Contains(err.Error(), "--execute-tools requires E2E_MODE=docker") {
		t.Fatalf("newEvaluationRuntime() error = %v, want execution guard error", err)
	}
}

// TestCloseRuntimeSessions_RunsEveryCloser verifies the composed close handle
// invokes each registered session closer, and the no-op close is safe.
func TestCloseRuntimeSessions_RunsEveryCloser(t *testing.T) {
	var closed []string
	closeRuntimeSessions([]func(){
		func() { closed = append(closed, "first") },
		func() { closed = append(closed, "second") },
	})()
	if strings.Join(closed, ",") != "first,second" {
		t.Fatalf("closed = %v, want both closers", closed)
	}
	noopCloseRuntime()
}

// TestNoopCloseTerminalOutput_ReturnsNil verifies the placeholder terminal
// closer used when logging is not configured reports no error.
func TestNoopCloseTerminalOutput_ReturnsNil(t *testing.T) {
	if err := noopCloseTerminalOutput(); err != nil {
		t.Fatalf("noopCloseTerminalOutput() error = %v, want nil", err)
	}
}

// TestPrepareRunFailureReport_WritesStartupPlaceholderAndErrorReport verifies
// the startup placeholder is written when an output path is configured, the
// deferred cleanup replaces it with the failure report when the run errored,
// and it leaves the final report alone when one was already written.
func TestPrepareRunFailureReport_WritesStartupPlaceholderAndErrorReport(t *testing.T) {
	cases := []struct {
		name        string
		runErr      error
		finalReport bool
		wantStatus  string
	}{
		{name: "failure replaces placeholder", runErr: errors.New("boom"), wantStatus: "failed"},
		{name: "final report kept", runErr: errors.New("boom"), finalReport: true, wantStatus: "running"},
		{name: "clean run keeps placeholder", wantStatus: "running"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "report.md")
			opts := options{Output: path, ToolSurface: config.ToolSurfaceMeta}
			var recorded error
			cleanup, err := prepareRunFailureReport(opts, func() error { return tc.runErr }, func(err error) { recorded = err }, func() bool { return tc.finalReport })
			if err != nil {
				t.Fatalf("prepareRunFailureReport() error = %v", err)
			}
			startup, readErr := os.ReadFile(path)
			if readErr != nil || !strings.Contains(string(startup), "Status: `running`") {
				t.Fatalf("startup report = %s, err = %v; want running placeholder", startup, readErr)
			}
			cleanup()
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatalf("read report: %v", readErr)
			}
			if !strings.Contains(string(content), "Status: `"+tc.wantStatus+"`") {
				t.Fatalf("report = %s, want status %s", content, tc.wantStatus)
			}
			if recorded != nil {
				t.Fatalf("setRunErr recorded %v, want nil", recorded)
			}
		})
	}
}

// TestPrepareRunFailureReport_NoOutputPath_SkipsReports verifies the failure
// report machinery stays inert when no report path is configured or the run
// only prepares fixtures.
func TestPrepareRunFailureReport_NoOutputPath_SkipsReports(t *testing.T) {
	cases := []struct {
		name string
		opts options
	}{
		{name: "no output", opts: options{}},
		{name: "fixtures only", opts: options{Output: "report.md", FixturesOnly: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cleanup, err := prepareRunFailureReport(tc.opts, func() error { return errors.New("boom") }, func(error) { t.Error("setRunErr called, want no report write") }, func() bool { return false })
			if err != nil {
				t.Fatalf("prepareRunFailureReport() error = %v", err)
			}
			cleanup()
		})
	}
}

// TestPrepareRunFailureReport_UnwritableOutput_ReturnsError verifies a startup
// report that cannot be written aborts the run before any evaluation begins.
func TestPrepareRunFailureReport_UnwritableOutput_ReturnsError(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	_, err := prepareRunFailureReport(options{Output: filepath.Join(blocker, "report.md")}, func() error { return nil }, func(error) {}, func() bool { return false })
	if err == nil || !strings.Contains(err.Error(), "create report directory") {
		t.Fatalf("prepareRunFailureReport() error = %v, want report directory error", err)
	}
}

// TestPrepareRunFailureReport_ErrorReportFails_RecordsJoinedError verifies a
// failure while writing the error report is joined onto the original run error
// rather than lost.
func TestPrepareRunFailureReport_ErrorReportFails_RecordsJoinedError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.md")
	runErr := errors.New("original failure")
	var recorded error
	cleanup, err := prepareRunFailureReport(options{Output: path}, func() error { return runErr }, func(err error) { recorded = err }, func() bool { return false })
	if err != nil {
		t.Fatalf("prepareRunFailureReport() error = %v", err)
	}
	if removeErr := os.RemoveAll(dir); removeErr != nil {
		t.Fatalf("remove report dir: %v", removeErr)
	}
	if writeErr := os.WriteFile(dir, []byte("x"), 0o600); writeErr != nil {
		t.Fatalf("replace dir with file: %v", writeErr)
	}
	cleanup()
	if recorded == nil || !errors.Is(recorded, runErr) || !strings.Contains(recorded.Error(), "create report directory") {
		t.Fatalf("recorded = %v, want run error joined with report failure", recorded)
	}
}

// TestRunImmediateMode_DispatchesCheckOnlyModes verifies each immediate CLI
// mode is claimed by runImmediateMode and an ordinary evaluation run is not.
func TestRunImmediateMode_DispatchesCheckOnlyModes(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.md")
	cases := []struct {
		name        string
		opts        options
		wantHandled bool
		wantErr     string
	}{
		{name: "check docs", opts: options{CheckDocs: true}, wantHandled: true, wantErr: "--publish-docs and --check-docs require"},
		{name: "check efficiency", opts: options{CheckEfficiency: stringList{missing}}, wantHandled: true, wantErr: "stat trace path"},
		{name: "check report clean", opts: options{CheckReportClean: stringList{missing}}, wantHandled: true, wantErr: "read report"},
		{name: "compare traces", opts: options{CompareTraces: stringList{missing}}, wantHandled: true, wantErr: "--compare-traces"},
		{name: "compare reports", opts: options{CompareReports: stringList{missing}}, wantHandled: true, wantErr: "--compare requires"},
		{name: "evaluation run", opts: options{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handled, err := runImmediateMode(tc.opts)
			if handled != tc.wantHandled {
				t.Fatalf("runImmediateMode() handled = %t, want %t", handled, tc.wantHandled)
			}
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("runImmediateMode() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("runImmediateMode() error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

// TestResolveRunModels_DryRunAndModelBackedDefaults verifies a dry run needs
// no provider credential and labels the model "none", a model-backed run
// resolves its specs and label, and both derive the default output path (and,
// for a model-backed run, the trace directory beside it).
func TestResolveRunModels_DryRunAndModelBackedDefaults(t *testing.T) {
	t.Setenv("EVAL_MODELS", "")
	cases := []struct {
		name          string
		opts          options
		wantModel     string
		wantSpecs     int
		wantTraceDir  bool
		wantOutputHas string
	}{
		{name: "dry run", opts: options{DryRun: true}, wantModel: "none", wantOutputHas: "model-"},
		{name: "model backed", opts: options{Models: "openai:gpt-x,anthropic:claude-y"}, wantModel: "openai:gpt-x,anthropic:claude-y", wantSpecs: 2, wantTraceDir: true, wantOutputHas: "multi-model"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts, specs, err := resolveRunModels(tc.opts)
			if err != nil {
				t.Fatalf("resolveRunModels() error = %v", err)
			}
			if opts.Model != tc.wantModel || len(specs) != tc.wantSpecs {
				t.Fatalf("resolveRunModels() = model %q with %d specs, want %q with %d", opts.Model, len(specs), tc.wantModel, tc.wantSpecs)
			}
			if !strings.Contains(opts.Output, tc.wantOutputHas) {
				t.Fatalf("output = %q, want %q", opts.Output, tc.wantOutputHas)
			}
			if tc.wantTraceDir != (opts.TraceDir != "") {
				t.Fatalf("trace dir = %q, want set = %t", opts.TraceDir, tc.wantTraceDir)
			}
		})
	}
}

// TestResolveRunModels_UnsupportedProvider_ReturnsError verifies an unknown
// provider in --models aborts before any evaluation.
func TestResolveRunModels_UnsupportedProvider_ReturnsError(t *testing.T) {
	_, _, err := resolveRunModels(options{Models: "bogus:model"})
	if err == nil || !strings.Contains(err.Error(), "unsupported model provider") {
		t.Fatalf("resolveRunModels() error = %v, want unsupported provider error", err)
	}
}

// TestPrepareRunEnvironment_MissingGitLabEnvFile_ReturnsError verifies a
// missing --gitlab-env-file aborts the run with the path in the message. Docker
// auto-start is off, so nothing is started.
func TestPrepareRunEnvironment_MissingGitLabEnvFile_ReturnsError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent.env")
	err := prepareRunEnvironment(options{GitLabEnv: missing})
	if err == nil || !strings.Contains(err.Error(), "load gitlab env file "+missing) {
		t.Fatalf("prepareRunEnvironment() error = %v, want env file error", err)
	}
}

// TestPrepareRunTasks_FiltersAndValidatesTaskSelection verifies task selection
// reports the failure modes a run can hit before any provider call: no tasks
// selected, conflicting destructive filters, and a repeat count below one.
func TestPrepareRunTasks_FiltersAndValidatesTaskSelection(t *testing.T) {
	cases := []struct {
		name string
		opts options
		want string
	}{
		{name: "no tasks selected", opts: options{OnlyIDs: "MT-DOES-NOT-EXIST", Repeat: 1}, want: "no tasks selected"},
		{name: "conflicting destructive flags", opts: options{SkipDestructive: true, OnlyDestructive: true, Repeat: 1}, want: "destructive"},
		{name: "conflicting mutation flags", opts: options{SkipMutating: true, OnlyMutating: true, Repeat: 1}, want: "mutating"},
		{name: "repeat below one", opts: options{OnlyIDs: "MT-001", Repeat: 0}, want: "repeat must be >= 1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := prepareRunTasks(tc.opts)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("prepareRunTasks() error = %v, want %q", err, tc.want)
			}
		})
	}
}

// TestPrepareRunTasks_DeprecatedTasksFile_ReturnsMigrationError verifies the
// removed Markdown task loader reports the typed-case migration instead of
// silently ignoring the flag.
func TestPrepareRunTasks_DeprecatedTasksFile_ReturnsMigrationError(t *testing.T) {
	_, _, err := prepareRunTasks(options{TasksPath: "tasks.md", Repeat: 1})
	if err == nil || !strings.Contains(err.Error(), "custom --tasks Markdown files are deprecated") {
		t.Fatalf("prepareRunTasks() error = %v, want deprecation error", err)
	}
}

// TestPrepareRunTasks_SelectedTask_ReturnsTypedTask verifies a run that names
// one catalog task gets exactly that task back with its typed case attached.
func TestPrepareRunTasks_SelectedTask_ReturnsTypedTask(t *testing.T) {
	tasks, fixtures, err := prepareRunTasks(options{OnlyIDs: "MT-001", Repeat: 1})
	if err != nil {
		t.Fatalf("prepareRunTasks() error = %v", err)
	}
	if fixtures != nil {
		t.Fatalf("fixtures = %+v, want nil without --prepare-fixtures", fixtures)
	}
	if len(tasks) != 1 || tasks[0].ID != "MT-001" || tasks[0].Case == nil {
		t.Fatalf("tasks = %+v, want one typed MT-001 task", tasks)
	}
}

// TestPrepareRunCatalog_FiltersTasksAgainstMockCatalog verifies the catalog
// stage loads the mock catalog, normalizes the selected task onto real routes,
// and applies the max-tasks cap.
func TestPrepareRunCatalog_FiltersTasksAgainstMockCatalog(t *testing.T) {
	tasks, _, err := prepareRunTasks(options{Repeat: 1, SkipMutating: true, SkipDestructive: true})
	if err != nil {
		t.Fatalf("prepareRunTasks() error = %v", err)
	}
	opts := options{ToolSurface: config.ToolSurfaceMeta, Edition: editionCE, MaxTasks: 3, SkipUnavailable: true}
	catalog, routes, filtered, err := prepareRunCatalog(opts, tasks, nil)
	if err != nil {
		t.Fatalf("prepareRunCatalog() error = %v", err)
	}
	if len(catalog) == 0 || len(routes) == 0 {
		t.Fatalf("catalog = %d tools, routes = %d, want a populated mock catalog", len(catalog), len(routes))
	}
	if len(filtered) != 3 {
		t.Fatalf("filtered tasks = %d, want 3 after --max-tasks", len(filtered))
	}
}

// TestPrepareRunCatalog_UnknownBackend_ReturnsError verifies an unrecognized
// backend aborts before any catalog is built.
func TestPrepareRunCatalog_UnknownBackend_ReturnsError(t *testing.T) {
	_, _, _, err := prepareRunCatalog(options{ToolSurface: config.ToolSurfaceMeta, Backend: "bogus"}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "unknown backend") {
		t.Fatalf("prepareRunCatalog() error = %v, want unknown backend error", err)
	}
}

// TestRunCatalogFilters_EmptySelection_ReturnError verifies each catalog-stage
// filter reports which flag emptied the task list, and that a filter left
// unset passes tasks through untouched.
func TestRunCatalogFilters_EmptySelection_ReturnError(t *testing.T) {
	ceTask := evalTask{ID: "MT-CE", Case: &EvalCase{ID: "MT-CE", Edition: EvalCaseEdition(editionCE), Partition: EvalPartition(partitionBaseRead), Presets: []EvalPreset{EvalPreset(presetDockerRead)}}}
	cases := []struct {
		name  string
		apply func() ([]evalTask, error)
		want  string
	}{
		{name: "edition", apply: func() ([]evalTask, error) { return applyEditionFilter([]evalTask{ceTask}, editionEnterprise) }, want: "no tasks selected after --edition=enterprise"},
		{name: "partition", apply: func() ([]evalTask, error) {
			return applyPartitionFilter([]evalTask{ceTask}, partitionEnterpriseRead)
		}, want: "no tasks selected after --partition=enterprise-read"},
		{name: "preset", apply: func() ([]evalTask, error) {
			return applyPresetFilter([]evalTask{ceTask}, presetDockerEnterpriseRead)
		}, want: "no tasks selected after --preset=docker-enterprise-read"},
		{name: "availability", apply: func() ([]evalTask, error) {
			return applyAvailabilityFilter([]evalTask{{ID: "MT-X", Steps: []evalStep{{ExpectedTool: "gitlab_missing", ExpectedAction: "get"}}}}, nil, false, nil, true)
		}, want: "no tasks selected after --skip-unavailable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.apply(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("filter error = %v, want %q", err, tc.want)
			}
		})
	}
	passthrough := []evalTask{ceTask}
	for _, tc := range []struct {
		name  string
		apply func() ([]evalTask, error)
	}{
		{name: "partition unset", apply: func() ([]evalTask, error) { return applyPartitionFilter(passthrough, "") }},
		{name: "preset unset", apply: func() ([]evalTask, error) { return applyPresetFilter(passthrough, "") }},
		{name: "availability off", apply: func() ([]evalTask, error) { return applyAvailabilityFilter(passthrough, nil, false, nil, false) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.apply()
			if err != nil || len(got) != 1 {
				t.Fatalf("filter = %+v, %v; want passthrough", got, err)
			}
		})
	}
}

// TestApplyEditionFilter_InvalidEdition_ReturnsParseError verifies an
// unsupported edition selector is refused by the filter itself.
func TestApplyEditionFilter_InvalidEdition_ReturnsParseError(t *testing.T) {
	if _, err := applyEditionFilter(nil, "premium"); err == nil {
		t.Fatal("applyEditionFilter(premium) error = nil, want rejection")
	}
}

// TestRunDryRunEvaluation_WritesStaticValidationReport verifies the dry-run
// path validates the selected tasks against the catalog, repeats the run, and
// writes both the Markdown report and the route coverage report without
// contacting a provider.
func TestRunDryRunEvaluation_WritesStaticValidationReport(t *testing.T) {
	dir := t.TempDir()
	opts := options{
		ToolSurface:    config.ToolSurfaceMeta,
		Output:         filepath.Join(dir, "report.md"),
		CoverageReport: filepath.Join(dir, "coverage.md"),
		Repeat:         2,
		Model:          "none",
		DryRun:         true,
	}
	tasks := []evalTask{{ID: "MT-1", Steps: []evalStep{{ExpectedTool: "gitlab_project", ExpectedAction: "get"}}}}
	catalog := []modelTool{{Name: "gitlab_project"}}
	routes := map[string]toolutil.ActionMap{"gitlab_project": {"get": projectGetRoute(), "delete": {}}}

	if err := runDryRunEvaluation(t.Context(), opts, tasks, catalog, routes); err != nil {
		t.Fatalf("runDryRunEvaluation() error = %v", err)
	}
	report, err := os.ReadFile(opts.Output)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	for _, want := range []string{"static route/schema validation", "Task attempts: 2"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(string(report), want) {
				t.Fatalf("report = %s, want %q", report, want)
			}
		})
	}
	coverage, err := os.ReadFile(opts.CoverageReport)
	if err != nil {
		t.Fatalf("read coverage: %v", err)
	}
	if !strings.Contains(string(coverage), "gitlab_project/delete") {
		t.Fatalf("coverage = %s, want uncovered delete route", coverage)
	}
}

// TestRunDryRunEvaluation_ExposeResources_AddsBridgeToolsToCatalog verifies a
// dry run with --expose-resources advertises the capability bridge tools and
// records every access channel as active in the report header.
func TestRunDryRunEvaluation_ExposeResources_AddsBridgeToolsToCatalog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.md")
	opts := options{ToolSurface: config.ToolSurfaceDynamic, Output: path, Repeat: 1, DryRun: true, ExposeResources: true}
	tasks := []evalTask{{ID: "MS-1", Steps: []evalStep{{ExpectedTool: resourceListTool}}}}

	if err := runDryRunEvaluation(t.Context(), opts, tasks, nil, nil); err != nil {
		t.Fatalf("runDryRunEvaluation() error = %v", err)
	}
	report, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	for _, want := range []string{"MCP capability bridge: `enabled`", "Resource access: `enabled`", "Prompt access: `enabled`", "Completion access: `enabled`", "Catalog tools: 6"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(string(report), want) {
				t.Fatalf("report = %s, want %q", report, want)
			}
		})
	}
}

// TestRunFixtureSmokeEvaluation_RequiresExecutionAndFixtures verifies the
// fixture-smoke mode refuses to run without both --execute-tools and
// --use-fixtures, so it never runs against a catalog it cannot exercise.
func TestRunFixtureSmokeEvaluation_RequiresExecutionAndFixtures(t *testing.T) {
	cases := []struct {
		name string
		opts options
	}{
		{name: "no execute", opts: options{FixtureSmoke: true, UseFixtures: true, DryRun: true}},
		{name: "no fixtures", opts: options{FixtureSmoke: true, Execute: true, DryRun: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := runDryRunEvaluation(t.Context(), tc.opts, nil, nil, nil)
			if err == nil || !strings.Contains(err.Error(), "--fixture-smoke requires --execute-tools and --use-fixtures") {
				t.Fatalf("runDryRunEvaluation() error = %v, want fixture-smoke guard", err)
			}
		})
	}
}

// TestRunFixtureSmokeAttempts_PreparesEveryTaskWithoutProvider verifies the
// fixture smoke loop prepares each task's typed fixtures per run and reports a
// success row naming the fixture smoke, with no provider or GitLab client.
func TestRunFixtureSmokeAttempts_PreparesEveryTaskWithoutProvider(t *testing.T) {
	var ensured int
	task := taskFromCase(EvalCase{
		ID:             "MT-SMOKE",
		PromptTemplate: CasePromptTemplate{Text: "Get project {{ .Project.Path }}."},
		Steps:          []ExpectedStep{{ExpectedTool: "gitlab_project", ExpectedAction: "get"}},
		Fixtures: []CaseFixtureSpec{{
			Name:    "project",
			Scope:   FixtureScopeAttempt,
			Outputs: []string{"project_path"},
			Ensure: func(context.Context, FixtureContext) (FixtureOutput, error) {
				ensured++
				return FixtureOutput{"project_path": "my-org/app"}, nil
			},
		}},
	})
	opts := options{Execute: true, UseFixtures: true, ToolSurface: config.ToolSurfaceMeta, Repeat: 2}
	run := modelEvaluationRun{opts: opts, tasks: []evalTask{task}, runtime: evaluationRuntime{opts: opts}, liveAttemptRunSuffix: "smoke"}

	results, err := runFixtureSmokeAttempts(t.Context(), run, modelSpec{Model: "fixture-smoke"})
	if err != nil {
		t.Fatalf("runFixtureSmokeAttempts() error = %v", err)
	}
	if ensured != 2 || len(results) != 2 {
		t.Fatalf("ensured = %d, results = %d; want one prepared fixture per run", ensured, len(results))
	}
	for _, result := range results {
		if !result.FinalSuccess || !result.FirstPass || !result.DestructiveSafe || result.CompletedSteps != 1 {
			t.Fatalf("result = %+v, want fixture smoke success", result)
		}
		if strings.Join(result.Notes, "") != "live fixture smoke prepared resources" {
			t.Fatalf("notes = %v, want fixture smoke note", result.Notes)
		}
	}
}

// TestRunFixtureSmokeAttempts_FixtureFailure_AbortsWithTaskID verifies a
// fixture failure during smoke preparation names the task that failed.
func TestRunFixtureSmokeAttempts_FixtureFailure_AbortsWithTaskID(t *testing.T) {
	task := taskFromCase(EvalCase{
		ID:     "MT-SMOKE-FAIL",
		Prompt: "x",
		Steps:  []ExpectedStep{{ExpectedTool: "gitlab_project", ExpectedAction: "get"}},
		Fixtures: []CaseFixtureSpec{{
			Name:  "broken",
			Scope: FixtureScopeAttempt,
			Ensure: func(context.Context, FixtureContext) (FixtureOutput, error) {
				return nil, errors.New("fixture exploded")
			},
		}},
	})
	opts := options{Execute: true, UseFixtures: true, ToolSurface: config.ToolSurfaceMeta, Repeat: 1}
	run := modelEvaluationRun{opts: opts, tasks: []evalTask{task}, runtime: evaluationRuntime{opts: opts}}

	_, err := runFixtureSmokeAttempts(t.Context(), run, modelSpec{Model: "fixture-smoke"})
	if err == nil || !strings.Contains(err.Error(), "fixture smoke MT-SMOKE-FAIL") {
		t.Fatalf("runFixtureSmokeAttempts() error = %v, want task-scoped fixture error", err)
	}
}

// TestFixtureSmokeResult_UsesFirstAndLastStepMetadata verifies the synthetic
// smoke result reports the first and last expected calls and counts every
// scenario step as completed.
func TestFixtureSmokeResult_UsesFirstAndLastStepMetadata(t *testing.T) {
	task := evalTask{ID: "MS-1", Steps: []evalStep{
		{ExpectedTool: dynamicFindTool},
		{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: actionProjectGet},
	}}
	got := fixtureSmokeResult(task, modelSpec{Model: "fixture-smoke"}, config.ToolSurfaceDynamic, 4)
	if got.FirstTool != dynamicFindTool || got.FinalAction != actionProjectGet || got.CompletedSteps != 2 || got.Run != 4 {
		t.Fatalf("fixtureSmokeResult() = %+v, want first/last step metadata", got)
	}
}

// TestPreparedTaskAttempt_PreparedCase_FallsBackToTask verifies an attempt
// without a prepared case derives one from the task itself.
func TestPreparedTaskAttempt_PreparedCase_FallsBackToTask(t *testing.T) {
	task := evalTask{ID: "MT-1", Prompt: "Get project.", Steps: []evalStep{{ExpectedTool: "gitlab_project", ExpectedAction: "get"}}}
	prepared := preparedTaskAttempt{Task: task}.PreparedCase()
	if prepared.Prompt != "Get project." || string(prepared.Case.ID) != "MT-1" || len(prepared.Steps) != 1 {
		t.Fatalf("PreparedCase() = %+v, want case derived from task", prepared)
	}
	explicit := PreparedCase{Prompt: "explicit"}
	if got := (preparedTaskAttempt{Task: task, Prepared: &explicit}).PreparedCase(); got.Prompt != "explicit" {
		t.Fatalf("PreparedCase() = %+v, want the prepared case", got)
	}
}

// TestPrepareTaskAttempt_WithoutFixtures_ReturnsTaskUnchanged verifies a run
// that neither executes nor uses fixtures skips fixture preparation entirely.
func TestPrepareTaskAttempt_WithoutFixtures_ReturnsTaskUnchanged(t *testing.T) {
	task := evalTask{ID: "MT-1", Prompt: "Get project."}
	got, err := prepareTaskAttempt(t.Context(), options{}, modelSpec{Model: "m"}, 1, task, evaluationRuntime{}, "suffix")
	if err != nil || got.ID != "MT-1" || got.Prompt != "Get project." {
		t.Fatalf("prepareTaskAttempt() = %+v, %v; want unchanged task", got, err)
	}
}

// TestPrepareTaskAttempt_FixtureFailure_ReturnsOriginalTaskAndError verifies a
// failed typed fixture returns the original task alongside the error so the
// caller can still report the row.
func TestPrepareTaskAttempt_FixtureFailure_ReturnsOriginalTaskAndError(t *testing.T) {
	task := taskFromCase(EvalCase{
		ID:     "MT-FAIL",
		Prompt: "x",
		Steps:  []ExpectedStep{{ExpectedTool: "gitlab_project", ExpectedAction: "get"}},
		Fixtures: []CaseFixtureSpec{{
			Name:  "broken",
			Scope: FixtureScopeAttempt,
			Ensure: func(context.Context, FixtureContext) (FixtureOutput, error) {
				return nil, errors.New("fixture exploded")
			},
		}},
	})
	got, err := prepareTaskAttempt(t.Context(), options{Execute: true, UseFixtures: true}, modelSpec{Model: "m"}, 1, task, evaluationRuntime{}, "suffix")
	if err == nil || !strings.Contains(err.Error(), "fixture exploded") {
		t.Fatalf("prepareTaskAttempt() error = %v, want fixture failure", err)
	}
	if got.ID != "MT-FAIL" {
		t.Fatalf("prepareTaskAttempt() task = %+v, want original task", got)
	}
}
