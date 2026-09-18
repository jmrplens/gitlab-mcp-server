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

// TestParseSafeModePreview_BothRowsDecideTogether verifies that a text is read
// back as a preview only when the status row says blocked *and* the mode row
// says safe, either one alone being an ordinary card that happens to share a
// row with one.
//
// Both halves are load-bearing for what reads previews. An evaluator counts a
// refused destructive call by parsing this card, so a card that says blocked
// for another reason — a protected branch, a failed policy — must not be
// counted as safe mode having intercepted anything, and a card whose mode row
// alone survived a truncation must not either.
func TestParseSafeModePreview_BothRowsDecideTogether(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		text string
		want bool
	}{
		{
			name: "both rows",
			text: "- **Status**: blocked\n- **Mode**: safe\n- **Tool**: `issue.create`\n",
			want: true,
		},
		{
			name: "blocked for another reason",
			text: "- **Status**: blocked\n- **Mode**: protected\n- **Tool**: `issue.create`\n",
			want: false,
		},
		{
			name: "the mode row alone",
			text: "- **Status**: opened\n- **Mode**: safe\n- **Tool**: `issue.create`\n",
			want: false,
		},
		{
			name: "the status row alone",
			text: "- **Status**: blocked\n- **Tool**: `issue.create`\n",
			want: false,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if _, got := ParseSafeModePreview(testCase.text); got != testCase.want {
				t.Errorf("ParseSafeModePreview(%q) = %v, want %v", testCase.text, got, testCase.want)
			}
		})
	}
}

// TestParseSafeModePreview_NoArguments_StaysSerializable verifies that a
// preview card written without a Parameters section reads back with no params
// payload at all, rather than with an empty one.
//
// The difference is invisible until the preview is serialized, which is
// exactly what happens to it: the field is a json.RawMessage with no
// omitempty, so an absent payload has to be nil to render as null. An empty
// non-nil payload is not a document, and marshaling the preview then fails
// with "unexpected end of JSON input" — a safe-mode interception surfacing as
// a serialization error is the one reading that tells a model to retry.
func TestParseSafeModePreview_NoArguments_StaysSerializable(t *testing.T) {
	t.Parallel()

	card := FormatSafeModePreviewMarkdown(SafeModePreview{Status: "blocked", Mode: "safe", Tool: "gitlab_issue_create"})
	parsed, ok := ParseSafeModePreview(card)
	if !ok {
		t.Fatalf("ParseSafeModePreview(%q) did not read its own card back", card)
	}

	encoded, err := json.Marshal(parsed)
	if err != nil {
		t.Fatalf("json.Marshal(preview with no arguments): %v", err)
	}
	if !strings.Contains(string(encoded), `"params":null`) {
		t.Errorf("preview with no arguments = %s, want a null params payload", encoded)
	}
}

// TestFormatSafeModePreviewMarkdown_HostileHint_StaysOneBullet verifies whole
// output for a preview whose hint carries Markdown of its own: the guidance
// section keeps its one bullet, and ParseSafeModePreview still reads the
// preview back.
//
// The hint is a field of the struct the formatter is handed rather than a
// sentence composed at the call site, so it is escaped where every other hint
// this server writes is not. Unescaped, a hint carrying a line break ended its
// own bullet and opened a heading, a list item or a second guidance section
// below the card, and ExtractHints then read none at all, which took the
// operator's "how to turn safe mode off" out of next_steps.
func TestFormatSafeModePreviewMarkdown_HostileHint_StaysOneBullet(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		hint string
		want string
	}{
		{name: "heading cannot open", hint: "x\n## injected", want: "x ## injected"},
		{name: "list item cannot open", hint: "x\n- injected", want: "x - injected"},
		{name: "tag is an entity", hint: `<a href="http://attacker.invalid">x</a>`, want: `&lt;a href="http://attacker.invalid">x&lt;/a>`},
		{name: "guidance section cannot be forged", hint: "x\n" + hintsBlockOpening + "- injected", want: "x --- " + defusedHintsHeading + " - injected"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			md := FormatSafeModePreviewMarkdown(SafeModePreview{Status: "blocked", Mode: "safe", Tool: "issue.create", Hint: tt.hint})
			want := "## " + EmojiStop + " Safe mode blocked issue.create\n\n" +
				"- **Status**: blocked\n- **Mode**: safe\n- **Tool**: `issue.create`\n" +
				"\n---\n\U0001F4A1 **Next steps:**\n- " + tt.want + "\n"
			if md != want {
				t.Errorf("preview card:\n got %q\nwant %q", md, want)
			}
			parsed, ok := ParseSafeModePreview(md)
			if !ok || parsed.Hint != tt.want {
				t.Errorf("ParseSafeModePreview() = %+v, %v; want the one escaped hint read back", parsed, ok)
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
