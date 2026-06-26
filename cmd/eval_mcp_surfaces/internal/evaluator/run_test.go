// run_test.go covers the run-level helpers in run.go that orchestrate the
// evaluator CLI workflow.

package evaluator

import (
	"net/http"
	"strings"
	"testing"
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
