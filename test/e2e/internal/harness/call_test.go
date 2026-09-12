//go:build e2e

// call_test.go covers what a test learns from an answer: which class of
// failure it was, what the answer decodes into, and when a call is worth
// sending again.
//
// The classification is the part worth pinning hardest. A refusal reaches the
// client as text, so the only thing that separates a destructive call refused
// for want of a confirmation from a plain 404 is what the server wrote, and a
// harness that classified both as "tool error" would let a test assert the
// wrong refusal and pass.

package harness

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/e2ecalls"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// errorResult builds the shape every refusal this server makes arrives in: an
// error result whose first text block carries the reason.
func errorResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

// TestClassify_EveryRefusal_LandsInItsOwnClass pins how an answer is read.
//
// The markers are the server's own wording, so each case here is a sentence
// the product writes: the confirmation guard's instruction, the dispatcher's
// unknown-action message, its missing-parameter message, and GitLab's own
// statuses.
func TestClassify_EveryRefusal_LandsInItsOwnClass(t *testing.T) {
	cases := []struct {
		name        string
		text        string
		wantFailure Failure
		wantOutcome string
	}{
		{
			name:        "needs confirmation",
			text:        "Confirm gitlab_project_delete? This action may be irreversible. The connected client cannot prompt for confirmation. Re-send with confirm=true only after the user explicitly approves this operation.",
			wantFailure: FailureNeedsConfirmation,
			wantOutcome: e2ecalls.RefusedOutcome(toolutil.RefusalNeedsConfirmation),
		},
		{
			name:        "unknown action",
			text:        `gitlab_issue: unknown action "explode". Valid actions: create, get, list`,
			wantFailure: FailureUnknownAction,
			wantOutcome: e2ecalls.RefusedOutcome(toolutil.RefusalUnknownAction),
		},
		{
			name:        "missing params",
			text:        "gitlab_issue/get: missing required params: project_id, issue_iid. Put action-specific fields under params.",
			wantFailure: FailureInvalidParams,
			wantOutcome: e2ecalls.RefusedOutcome(toolutil.RefusalInvalidParams),
		},
		{
			name:        "params required",
			text:        "gitlab_issue/get: 'params' is required for this action. Required params: project_id.",
			wantFailure: FailureInvalidParams,
			wantOutcome: e2ecalls.RefusedOutcome(toolutil.RefusalInvalidParams),
		},
		{
			name:        "not found",
			text:        "get issue: 404 Not Found",
			wantFailure: FailureNotFound,
			wantOutcome: e2ecalls.RefusedOutcome(string(FailureNotFound)),
		},
		{
			name:        "forbidden",
			text:        "delete project: 403 Forbidden",
			wantFailure: FailureForbidden,
			wantOutcome: e2ecalls.RefusedOutcome(string(FailureForbidden)),
		},
		{
			name:        "anything else",
			text:        "create branch: branch already exists",
			wantFailure: FailureToolError,
			wantOutcome: e2ecalls.OutcomeToolError,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			answer := classify(errorResult(testCase.text), nil)

			if answer.failure != testCase.wantFailure {
				t.Errorf("failure = %q, want %q", answer.failure, testCase.wantFailure)
			}
			if answer.outcome != testCase.wantOutcome {
				t.Errorf("outcome = %q, want %q", answer.outcome, testCase.wantOutcome)
			}
			if answer.ok() {
				t.Error("a refused call reports itself as having run")
			}
		})
	}
}

// TestClassify_Success_IsOk checks that an ordinary answer is not classified
// as anything.
func TestClassify_Success_IsOk(t *testing.T) {
	answer := classify(&mcp.CallToolResult{StructuredContent: map[string]any{"id": 1}}, nil)

	if !answer.ok() {
		t.Fatalf("a successful call was classified %q: %s", answer.outcome, answer.describe())
	}
	if answer.outcome != e2ecalls.OutcomeOK {
		t.Errorf("outcome = %q, want %q", answer.outcome, e2ecalls.OutcomeOK)
	}
}

// TestClassify_SafeModePreview_IsAPreviewOnBothSurfaceShapes checks that a
// blocked mutation is recognized however the surface answered it.
//
// The dispatchers answer with the preview as structured content and the
// individual surface answers with the card. A classifier that read one of them
// would count half the previews as ordinary successes, which is the worst
// possible answer: the test would believe the mutation happened.
func TestClassify_SafeModePreview_IsAPreviewOnBothSurfaceShapes(t *testing.T) {
	preview := toolutil.NewSafeModePreview("project.delete", map[string]any{"project_id": 7})
	structured := map[string]any{}
	encoded, err := json.Marshal(preview)
	if err != nil {
		t.Fatalf("encoding the preview: %v", err)
	}
	if err = json.Unmarshal(encoded, &structured); err != nil {
		t.Fatalf("decoding the preview into a map: %v", err)
	}

	cases := []struct {
		name   string
		result *mcp.CallToolResult
	}{
		{
			name:   "structured, as the dispatchers answer",
			result: &mcp.CallToolResult{StructuredContent: structured},
		},
		{
			name: "card, as the individual surface answers",
			result: &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: toolutil.FormatSafeModePreviewMarkdown(preview)}},
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			answer := classify(testCase.result, nil)

			if answer.failure != FailureSafeMode {
				t.Errorf("failure = %q, want %q", answer.failure, FailureSafeMode)
			}
			if answer.outcome != e2ecalls.OutcomePreview {
				t.Errorf("outcome = %q, want %q", answer.outcome, e2ecalls.OutcomePreview)
			}
			if !strings.Contains(answer.text, "project.delete") {
				t.Errorf("the preview does not name the action it blocked: %q", answer.text)
			}
		})
	}
}

// TestClassify_NoAnswer_IsAProtocolOrTransportFailure checks the two ways a
// call comes back without being a tool outcome at all.
//
// They are kept apart because they mean different things: a JSON-RPC error is
// the server answering, and a dropped connection is the server or GitLab
// having gone away, which is the only one worth sending again.
func TestClassify_NoAnswer_IsAProtocolOrTransportFailure(t *testing.T) {
	cases := []struct {
		name        string
		err         error
		result      *mcp.CallToolResult
		wantOutcome string
	}{
		{name: "json-rpc error", err: errors.New("method not found"), wantOutcome: e2ecalls.OutcomeProtocolError},
		{name: "dropped connection", err: errors.New("write |1: broken pipe"), wantOutcome: e2ecalls.OutcomeTransportError},
		{name: "nothing at all", wantOutcome: e2ecalls.OutcomeProtocolError},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			answer := classify(testCase.result, testCase.err)

			if answer.outcome != testCase.wantOutcome {
				t.Errorf("outcome = %q, want %q", answer.outcome, testCase.wantOutcome)
			}
			if answer.failure != FailureProtocolError {
				t.Errorf("failure = %q, want %q", answer.failure, FailureProtocolError)
			}
			if answer.err == nil {
				t.Error("an answer that never arrived carries no error")
			}
		})
	}
}

// TestDecodeResult_StructuredFirstThenText pins the decoding order.
//
// The fallback is not theoretical: a tool whose output schema the SDK could
// not derive answers with text alone, and a decoder that read only structured
// content would hand the test an empty object and no error.
func TestDecodeResult_StructuredFirstThenText(t *testing.T) {
	type answer struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}

	cases := []struct {
		name   string
		result *mcp.CallToolResult
		want   answer
	}{
		{
			name:   "structured content",
			result: &mcp.CallToolResult{StructuredContent: map[string]any{"id": 7, "name": "from structured"}},
			want:   answer{ID: 7, Name: "from structured"},
		},
		{
			name:   "text fallback",
			result: &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: `{"id":9,"name":"from text"}`}}},
			want:   answer{ID: 9, Name: "from text"},
		},
		{
			name: "structured wins over text",
			result: &mcp.CallToolResult{
				StructuredContent: map[string]any{"id": 1, "name": "structured"},
				Content:           []mcp.Content{&mcp.TextContent{Text: `{"id":2,"name":"text"}`}},
			},
			want: answer{ID: 1, Name: "structured"},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var decoded answer
			if err := decodeResult(testCase.result, &decoded); err != nil {
				t.Fatalf("decodeResult() error = %v", err)
			}
			if decoded != testCase.want {
				t.Errorf("decoded %+v, want %+v", decoded, testCase.want)
			}
		})
	}
}

// TestDecodeResult_NothingToDecode_IsReported checks that an answer carrying
// neither shape fails rather than leaving the test with a zero value.
func TestDecodeResult_NothingToDecode_IsReported(t *testing.T) {
	cases := []struct {
		name   string
		result *mcp.CallToolResult
	}{
		{name: "no result", result: nil},
		{name: "no content", result: &mcp.CallToolResult{}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var decoded map[string]any
			if err := decodeResult(testCase.result, &decoded); err == nil {
				t.Fatal("decodeResult() accepted a result with nothing in it")
			}
		})
	}
}

// TestResolveCallOptions_DefaultsAndOverrides pins what a call is when nobody
// says otherwise, and what each option changes.
func TestResolveCallOptions_DefaultsAndOverrides(t *testing.T) {
	defaults := resolveCallOptions(nil)
	if defaults.purpose != PurposeTest {
		t.Errorf("purpose = %q, want %q", defaults.purpose, PurposeTest)
	}
	if !defaults.confirm {
		t.Error("a destructive call is unconfirmed by default, and the server then fails closed on every one of them")
	}

	overridden := resolveCallOptions([]CallOption{For(PurposeCleanup), WithoutConfirmation(), Within(time.Second)})
	if overridden.purpose != PurposeCleanup {
		t.Errorf("purpose = %q, want %q", overridden.purpose, PurposeCleanup)
	}
	if overridden.confirm {
		t.Error("WithoutConfirmation() left the confirmation on")
	}
	if overridden.timeout != time.Second {
		t.Errorf("timeout = %s, want 1s", overridden.timeout)
	}
}

// TestCallLabel_NamesThePurposeWhenItIsNotATestCall checks that a failure
// inside a cleanup says so, since a cleanup failing reads very differently
// from the test body failing.
func TestCallLabel_NamesThePurposeWhenItIsNotATestCall(t *testing.T) {
	cases := []struct {
		name    string
		purpose Purpose
		want    string
	}{
		{name: "test", purpose: PurposeTest, want: "issue.delete"},
		{name: "cleanup", purpose: PurposeCleanup, want: "issue.delete (cleanup)"},
		{name: "sweep", purpose: PurposeSweep, want: "issue.delete (sweep)"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			label := callLabel("issue.delete", callOptions{purpose: testCase.purpose})
			if label != testCase.want {
				t.Errorf("callLabel() = %q, want %q", label, testCase.want)
			}
		})
	}
}

// TestActionTierCeiling_FollowsThePackagesRequirement pins the ceiling a call
// is checked against.
//
// A package that declared no license runs Free actions only, whatever the
// instance turns out to be; a licensed package runs up to whatever the
// instance is licensed for.
func TestActionTierCeiling_FollowsThePackagesRequirement(t *testing.T) {
	cases := []struct {
		name        string
		requirement Requirement
		tier        edition.Tier
		want        edition.Tier
	}{
		{name: "any on a licensed instance", requirement: Any, tier: edition.Ultimate, want: edition.Free},
		{name: "free", requirement: Free, tier: edition.Free, want: edition.Free},
		{name: "licensed premium", requirement: Licensed, tier: edition.Premium, want: edition.Premium},
		{name: "licensed ultimate", requirement: Licensed, tier: edition.Ultimate, want: edition.Ultimate},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			inst := &instance{requirement: testCase.requirement, facts: runtimeFacts{Tier: testCase.tier}}
			if ceiling := inst.actionTierCeiling(); ceiling != testCase.want {
				t.Errorf("actionTierCeiling() = %s, want %s", ceiling, testCase.want)
			}
		})
	}
}

// TestFirstLines_TruncatesAndSaysSo checks that a server's answer can be
// quoted in a failure without burying the assertion that failed.
func TestFirstLines_TruncatesAndSaysSo(t *testing.T) {
	short := firstLines("one\ntwo", 6)
	if short != "one\ntwo" {
		t.Errorf("firstLines() = %q, want the whole text", short)
	}

	long := firstLines(strings.Repeat("line\n", 20), 3)
	if strings.Count(long, "\n") > 3 {
		t.Errorf("firstLines() kept %d lines, want 3 plus the note", strings.Count(long, "\n"))
	}
	if !strings.Contains(long, "more lines") {
		t.Errorf("firstLines() truncated without saying so: %q", long)
	}
}

// TestIsTransportError_OnlyTheDroppedConnections checks which errors are read
// as the connection having gone away.
func TestIsTransportError_OnlyTheDroppedConnections(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "eof", err: errors.New("unexpected EOF"), want: true},
		{name: "reset", err: errors.New("read tcp: connection reset by peer"), want: true},
		{name: "broken pipe", err: errors.New("write: broken pipe"), want: true},
		{name: "refused", err: errors.New("dial tcp: connection refused"), want: true},
		{name: "closed file", err: errors.New("file already closed"), want: true},
		{name: "method not found", err: errors.New("method not found"), want: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := isTransportError(testCase.err); got != testCase.want {
				t.Errorf("isTransportError(%v) = %t, want %t", testCase.err, got, testCase.want)
			}
		})
	}
}
