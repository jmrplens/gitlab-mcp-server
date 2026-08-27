// run_test.go covers the run-level helpers in run.go that orchestrate the
// evaluator CLI workflow.

package evaluator

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
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
			r := taskResult{
				Notes: []string{"expected action issue.create but model called issue.list"},
			}
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
