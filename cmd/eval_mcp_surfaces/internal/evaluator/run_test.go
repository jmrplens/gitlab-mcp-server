package evaluator

import (
	"net/http"
	"strings"
	"testing"
)

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
