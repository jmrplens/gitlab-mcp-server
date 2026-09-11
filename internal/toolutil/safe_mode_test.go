// safe_mode_test.go verifies the preview safe mode returns instead of executing
// a mutating action.
package toolutil

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
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

// TestFormatSafeModePreviewMarkdown_WholeOutput_AndRoundTrip verifies the
// preview card byte for byte, the way every refusal is rendered, and that
// ParseSafeModePreview reads the same preview back out of it, which is what
// keeps the tests on every surface from parsing the card by hand. A preview
// with no arguments writes no fence, and the individual surface's result is
// the card in the refusal envelope.
func TestFormatSafeModePreviewMarkdown_WholeOutput_AndRoundTrip(t *testing.T) {
	t.Parallel()

	preview := NewSafeModePreview("issue.create", map[string]any{"project_id": "42", "title": "a|b"})
	md := FormatSafeModePreviewMarkdown(preview)
	want := "## " + EmojiStop + " Safe mode blocked issue.create\n\n" +
		"- **Status**: blocked\n" +
		"- **Mode**: safe\n" +
		"- **Tool**: `issue.create`\n" +
		"\n### Parameters\n\n" +
		"```json\n{\"project_id\":\"42\",\"title\":\"a|b\"}\n```\n" +
		"\n---\n\U0001F4A1 **Next steps:**\n- " + SafeModeHint + "\n"
	if md != want {
		t.Errorf("preview card:\n got %q\nwant %q", md, want)
	}

	parsed, ok := ParseSafeModePreview(md)
	if !ok || parsed.Status != "blocked" || parsed.Mode != "safe" || parsed.Tool != "issue.create" || parsed.Hint != SafeModeHint {
		t.Errorf("ParseSafeModePreview() = %+v, %v; want the preview read back", parsed, ok)
	}
	if string(parsed.Params) != `{"project_id":"42","title":"a|b"}` {
		t.Errorf("parsed params = %s, want the fenced arguments", parsed.Params)
	}

	bare := FormatSafeModePreviewMarkdown(SafeModePreview{Status: "blocked", Mode: "safe", Tool: "gitlab_issue_create"})
	wantBare := "## " + EmojiStop + " Safe mode blocked gitlab_issue_create\n\n" +
		"- **Status**: blocked\n- **Mode**: safe\n- **Tool**: `gitlab_issue_create`\n"
	if bare != wantBare {
		t.Errorf("preview card without arguments:\n got %q\nwant %q", bare, wantBare)
	}
	if _, isPreview := ParseSafeModePreview("## Issue #1\n\n- **Status**: opened\n"); isPreview {
		t.Error("ParseSafeModePreview() read an issue card as a preview")
	}

	result := SafeModePreviewResult(preview)
	if !result.IsError || result.Content[0].(*mcp.TextContent).Annotations != ContentMutate || result.Content[0].(*mcp.TextContent).Text != md {
		t.Errorf("SafeModePreviewResult() = %+v, want the card in the refusal envelope", result)
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
