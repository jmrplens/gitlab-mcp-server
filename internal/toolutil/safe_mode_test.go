// safe_mode_test.go verifies the preview safe mode returns instead of executing
// a mutating action.
package toolutil

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestNewSafeModePreview_ParamsThatCannotBeEncoded_StillReportTheBlock covers
// the defensive marshal in the preview builder.
//
// The preview is what a model reads instead of a result, so it must name the
// operation that was blocked even when the arguments cannot be rendered.
// Failing the call instead would present a safe-mode interception as a tool
// error, which is the one reading that tells a model to retry.
func TestNewSafeModePreview_ParamsThatCannotBeEncoded_StillReportTheBlock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		params     any
		wantParams string
	}{
		{name: "ordinary arguments", params: map[string]any{"project_id": "42"}, wantParams: `{"project_id":"42"}`},
		{name: "arguments the encoder refuses", params: map[string]any{"stream": make(chan int)}, wantParams: "null"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			preview := NewSafeModePreview("gitlab_issue_delete", tt.params)

			if preview.Status != "blocked" || preview.Mode != "safe" {
				t.Errorf("preview = %+v, want it to read as a safe-mode block", preview)
			}
			if preview.Tool != "gitlab_issue_delete" {
				t.Errorf("tool = %q, want the blocked operation named", preview.Tool)
			}
			if string(preview.Params) != tt.wantParams {
				t.Errorf("params = %s, want %s", preview.Params, tt.wantParams)
			}
			if !strings.Contains(preview.Hint, "GITLAB_MCP_SAFE_MODE") {
				t.Errorf("hint = %q, want it to say how to turn safe mode off", preview.Hint)
			}
		})
	}
}

// TestSafeModeActionFunc_ReturnsAPreviewInsteadOfExecuting covers the action
// wrapper the catalog installs over every mutating route in safe mode.
//
// It answers successfully, because a preview is a result rather than a failure,
// and it carries the arguments it was called with so the reader can see what
// would have happened.
func TestSafeModeActionFunc_ReturnsAPreviewInsteadOfExecuting(t *testing.T) {
	t.Parallel()

	action := SafeModeActionFunc("gitlab_branch_delete")

	result, err := action(context.Background(), map[string]any{"branch": "main"})
	if err != nil {
		t.Fatalf("a safe-mode preview returned an error: %v", err)
	}
	preview, ok := result.(SafeModePreview)
	if !ok {
		t.Fatalf("result = %T, want a SafeModePreview", result)
	}
	if preview.Tool != "gitlab_branch_delete" {
		t.Errorf("tool = %q, want the intercepted action named", preview.Tool)
	}
	var params map[string]any
	if unmarshalErr := json.Unmarshal(preview.Params, &params); unmarshalErr != nil {
		t.Fatalf("preview params are not JSON: %v", unmarshalErr)
	}
	if params["branch"] != "main" {
		t.Errorf("params = %v, want the call's own arguments", params)
	}
}
