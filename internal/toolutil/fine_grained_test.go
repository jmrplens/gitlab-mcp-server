package toolutil

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/mcpotel"
)

// The actions of the table these tests decide by.
const (
	fgDenied  = "demo.denied"
	fgAllowed = "demo.allowed"
	fgGraphQL = "demo.graphql"
)

// fineGrainedTable is a table with one action no fine-grained token runs, one
// served over REST, and one served over GraphQL with a part always empty and
// a list for an answer.
func fineGrainedTable() *finegrained.Table {
	return &finegrained.Table{
		Version: "19.4.1-ee",
		Bucket:  "19.4",
		Elements: []finegrained.Element{
			{Path: "project.issueLinks.nodes", Type: "VulnerabilityIssueLink", Undeclared: true, Effect: finegrained.EffectRemoved},
		},
		Actions: []finegrained.Requirement{
			{ID: fgAllowed},
			{ID: fgDenied, Denied: &finegrained.Denial{Cause: finegrained.CauseTypeUndeclared, Element: "Namespace", Effect: finegrained.EffectNull}},
			{ID: fgGraphQL, Degraded: []uint32{0}, GraphQL: true, Collection: true},
		},
	}
}

// fineGrainedContext is a request context bound to a client that carries a
// phase A authority over [fineGrainedTable], which is what a fine-grained
// session's requests carry.
func fineGrainedContext() context.Context {
	client := gitlabclient.NewUnboundClient("https://gitlab.example.com")
	client.SetAuthority(finegrained.Unevaluated(fineGrainedTable(), finegrained.FallbackNone, ""))
	return gitlabclient.WithClient(context.Background(), client)
}

// textOf is the text of a result's first text block.
func textOf(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if result == nil {
		t.Fatal("result is nil")
	}
	return resultText(result)
}

// TestFineGrainedRefusal_WithholdsOnlyWhatTheSessionMayNotRun verifies the
// check every dispatcher makes answers an action a fine-grained session may
// not run with the reason, prefixed as the caller asks, and lets through a
// classic session, an allowed action, an action the table has no row for and
// a route no catalog named.
func TestFineGrainedRefusal_WithholdsOnlyWhatTheSessionMayNotRun(t *testing.T) {
	session := fineGrainedContext()
	cases := []struct {
		name     string
		ctx      context.Context
		actionID string
		withheld bool
	}{
		{name: "a classic session", ctx: context.Background(), actionID: fgDenied},
		{name: "an allowed action", ctx: session, actionID: fgAllowed},
		{name: "an action with no row", ctx: session, actionID: "demo.unknown"},
		{name: "a route no catalog named", ctx: session, actionID: ""},
		{name: "a withheld action", ctx: session, actionID: fgDenied, withheld: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FineGrainedRefusal(tc.ctx, nil, "gitlab_demo", tc.actionID, "prefix: ")
			if (got != nil) != tc.withheld {
				t.Fatalf("FineGrainedRefusal = %+v, want withheld %v", got, tc.withheld)
			}
			if !tc.withheld {
				return
			}
			text := textOf(t, got)
			if !got.IsError || !strings.HasPrefix(text, `prefix: action "demo.denied" exists but is not available to a fine-grained personal access token: `) {
				t.Errorf("FineGrainedRefusal = %+v, text %q", got, text)
			}
		})
	}
}

// TestFineGrainedRefusal_RecordsTheRefusalAsFineGrained verifies a withheld
// call is recorded under the reason fine_grained on the span and in the INFO
// line, which the telemetry guide's table and the model evaluation read, and
// that the DEBUG line names the action and its cause.
func TestFineGrainedRefusal_RecordsTheRefusalAsFineGrained(t *testing.T) {
	var buf bytes.Buffer
	original := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(original) })
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	ctx, span := tp.Tracer("test").Start(fineGrainedContext(), "tools/call")
	refused := FineGrainedRefusal(ctx, nil, "gitlab_demo/denied", fgDenied, "")
	span.End()
	if refused == nil {
		t.Fatal("FineGrainedRefusal let a withheld action through")
	}

	// The value is spelled out rather than read from the constant, because the
	// value is what the guide and the evaluation filter on.
	out := buf.String()
	for _, want := range []string{
		`"msg":"tool call refused"`, `"tool":"gitlab_demo/denied"`, `"reason":"fine_grained"`,
		`"msg":"fine-grained session withheld an action"`, `"action":"demo.denied"`, `"cause":"graphql-type-undeclared"`,
	} {
		t.Run(want, func(t *testing.T) { assertContains(t, out, want) })
	}
	marked := 0
	for _, ended := range recorder.Ended() {
		for _, attr := range ended.Attributes() {
			if attr.Key == mcpotel.AttrRefusalReason && attr.Value.AsString() == "fine_grained" {
				marked++
			}
		}
	}
	if marked != 1 {
		t.Errorf("%d spans carry the refusal reason fine_grained, want the call's one", marked)
	}
}

// TestMakeMetaHandler_FineGrained_WithholdsBeforeConfirmation verifies the
// meta dispatcher refuses an action a fine-grained session may not run before
// it asks for the confirmation a destructive action needs, so a withheld delete
// is never offered for confirmation, and that its route never runs.
func TestMakeMetaHandler_FineGrained_WithholdsBeforeConfirmation(t *testing.T) {
	t.Setenv("GITLAB_MCP_YOLO_MODE", "false")
	ran := false
	route := DestructiveRoute(func(_ context.Context, _ map[string]any) (any, error) {
		ran = true
		return map[string]string{"status": "deleted"}, nil
	})
	route.ActionID = fgDenied
	handler := MakeMetaHandler("gitlab_demo", ActionMap{"denied": route}, nil)

	result, out, err := handler(fineGrainedContext(), forgedRequestState("gitlab_demo"), MetaToolInput{Action: "denied"})
	if err != nil || out != nil {
		t.Fatalf("handler() = (%+v, %+v, %v), want the withheld answer and nothing else", result, out, err)
	}
	if text := textOf(t, result); !result.IsError || !strings.Contains(text, `action "demo.denied" exists but is not available`) {
		t.Errorf("handler() text = %q, want the withheld answer", text)
	}
	if ran {
		t.Error("the withheld route ran")
	}
}

// TestSurfaceToolHandler_FineGrained_WithholdsFirst verifies the standalone
// dispatcher refuses an action a fine-grained session may not run before
// anything else, its destructive confirmation included.
func TestSurfaceToolHandler_FineGrained_WithholdsFirst(t *testing.T) {
	t.Setenv("GITLAB_MCP_YOLO_MODE", "false")
	ran := false
	route := DestructiveRoute(func(_ context.Context, _ map[string]any) (any, error) {
		ran = true
		return "done", nil
	})
	route.ActionID = fgDenied
	handler := surfaceToolHandler("gitlab_demo_denied", route, MarkdownForResult)

	result, _, err := handler(fineGrainedContext(), forgedRequestState("gitlab_demo_denied"), map[string]any{})
	if err != nil || !result.IsError || !strings.Contains(textOf(t, result), `action "demo.denied" exists but`) {
		t.Errorf("handler() = (%+v, %v), want the withheld answer", result, err)
	}
	if ran {
		t.Error("the withheld route ran")
	}
}

// emptyList is a list output with nothing in it.
type emptyList struct {
	Items      []string `json:"items"`
	Pagination struct{} `json:"pagination"`
}

// TestDispatchers_FineGrained_NoteWhatGitLabLeftEmpty verifies the meta and
// standalone dispatchers add, for a fine-grained session only, the notes on
// what GitLab leaves empty to a served answer, and the null note to a GraphQL
// action's not-found error.
func TestDispatchers_FineGrained_NoteWhatGitLabLeftEmpty(t *testing.T) {
	served := Route(func(_ context.Context, _ map[string]any) (any, error) { return emptyList{}, nil })
	served.ActionID = fgGraphQL
	missing := Route(func(_ context.Context, _ map[string]any) (any, error) {
		return nil, errors.New("get_thing: thing \"7\" not found")
	})
	missing.ActionID = fgGraphQL
	format := func(any) *mcp.CallToolResult {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "No items."}}}
	}
	meta := MakeMetaHandler("gitlab_demo", ActionMap{"list": served, "get": missing}, format)
	standalone := surfaceToolHandler("gitlab_demo_list", served, format)
	standaloneMissing := surfaceToolHandler("gitlab_demo_get", missing, format)
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "gitlab_demo"}}

	callers := map[string]func(context.Context) (*mcp.CallToolResult, error){
		"meta": func(ctx context.Context) (*mcp.CallToolResult, error) {
			result, _, err := meta(ctx, req, MetaToolInput{Action: "list"})
			return result, err
		},
		"standalone": func(ctx context.Context) (*mcp.CallToolResult, error) {
			result, _, err := standalone(ctx, req, map[string]any{})
			return result, err
		},
	}
	for name, call := range callers {
		t.Run(name+" served", func(t *testing.T) {
			result, err := call(fineGrainedContext())
			if err != nil {
				t.Fatal(err)
			}
			text := textOf(t, result)
			for _, want := range []string{"leaves part of this answer empty", "an empty answer may mean this token cannot see them"} {
				if !strings.Contains(text, want) {
					t.Errorf("text %q does not carry %q", text, want)
				}
			}
			classic, _ := call(context.Background())
			if got := textOf(t, classic); got != "No items." {
				t.Errorf("a classic session's text = %q, want the answer untouched", got)
			}
		})
	}
	errorCallers := map[string]func(context.Context) error{
		"meta": func(ctx context.Context) error {
			_, _, err := meta(ctx, req, MetaToolInput{Action: "get"})
			return err
		},
		"standalone": func(ctx context.Context) error {
			_, _, err := standaloneMissing(ctx, req, map[string]any{})
			return err
		},
	}
	for name, call := range errorCallers {
		t.Run(name+" not found", func(t *testing.T) {
			err := call(fineGrainedContext())
			if err == nil || !strings.Contains(err.Error(), "not found may mean this token cannot see it") {
				t.Errorf("error = %v, want the null note after it", err)
			}
		})
	}
}

// TestMakeMetaHandler_FineGrained_SafeModePreviewCarriesNoNote verifies the
// meta dispatcher, which the dynamic surface also runs through, answers a
// fine-grained session's write in safe mode with the preview alone: the call
// was never sent to GitLab, so the note on what GitLab leaves empty in its
// answer would describe an answer that does not exist. The same action served
// carries the note, which is what makes its absence from the preview about the
// preview.
func TestMakeMetaHandler_FineGrained_SafeModePreviewCarriesNoNote(t *testing.T) {
	served := Route(func(_ context.Context, _ map[string]any) (any, error) {
		return map[string]string{"state": "confirmed"}, nil
	})
	served.ActionID = fgGraphQL
	previewed := Route(SafeModeActionFunc(fgGraphQL))
	previewed.ActionID = fgGraphQL
	format := func(result any) *mcp.CallToolResult {
		if _, preview := result.(SafeModePreview); preview {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "Safe mode blocked " + fgGraphQL}}}
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "Confirmed."}}}
	}
	handler := MakeMetaHandler("gitlab_demo", ActionMap{"served": served, "previewed": previewed}, format)
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "gitlab_demo"}}

	result, _, err := handler(fineGrainedContext(), req, MetaToolInput{Action: "served"})
	if err != nil || !strings.Contains(textOf(t, result), "leaves part of this answer empty") {
		t.Fatalf("the served write = (%+v, %v), want the degraded note: the control is broken", result, err)
	}
	result, _, err = handler(fineGrainedContext(), req, MetaToolInput{Action: "previewed"})
	if err != nil {
		t.Fatal(err)
	}
	if text := textOf(t, result); text != "Safe mode blocked "+fgGraphQL {
		t.Errorf("the previewed write = %q, want the preview with no note", text)
	}
}

// TestFineGrainedNotes_AddsEachNoteWhereItApplies verifies which note each
// answer gets: a served answer the parts GitLab leaves empty and, when it is an
// empty list of a GraphQL list action, the empty-list note; a not-found answer
// the null note; and an answer of a classic session, of no action, or of none
// at all, nothing.
func TestFineGrainedNotes_AddsEachNoteWhereItApplies(t *testing.T) {
	session := fineGrainedContext()
	text := func(s string) *mcp.CallToolResult {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
	}
	failed := func(s string) *mcp.CallToolResult {
		result := text(s)
		result.IsError = true
		return result
	}
	cases := []struct {
		name     string
		ctx      context.Context
		actionID string
		result   *mcp.CallToolResult
		output   any
		want     []string
		wantNot  []string
	}{
		{name: "a classic session", ctx: context.Background(), actionID: fgGraphQL, result: text("body"), output: emptyList{}, wantNot: []string{"Next steps"}},
		{name: "no action", ctx: session, result: text("body"), output: emptyList{}, wantNot: []string{"Next steps"}},
		{
			name: "an empty list", ctx: session, actionID: fgGraphQL, result: text("body"), output: emptyList{},
			want: []string{"leaves part of this answer empty", "an empty answer may mean"},
		},
		{
			name: "a list with items", ctx: session, actionID: fgGraphQL, result: text("body"), output: emptyList{Items: []string{"x"}},
			want: []string{"leaves part of this answer empty"}, wantNot: []string{"an empty answer may mean"},
		},
		{name: "a REST action", ctx: session, actionID: fgAllowed, result: text("body"), output: emptyList{}, wantNot: []string{"Next steps"}},
		{
			name: "a not-found answer", ctx: session, actionID: fgGraphQL, result: failed("## Thing Not Found"),
			want: []string{"not found may mean this token cannot see it"}, wantNot: []string{"leaves part"},
		},
		{name: "another error", ctx: session, actionID: fgGraphQL, result: failed("boom"), wantNot: []string{"Next steps"}},
		{
			name: "a safe-mode preview", ctx: session, actionID: fgGraphQL, result: text("body"),
			output: NewSafeModePreview(fgGraphQL, map[string]any{}), wantNot: []string{"Next steps"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := textOf(t, FineGrainedNotes(tc.ctx, tc.actionID, tc.result, tc.output))
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("text %q does not carry %q", got, want)
				}
			}
			for _, unwanted := range tc.wantNot {
				if strings.Contains(got, unwanted) {
					t.Errorf("text %q carries %q", got, unwanted)
				}
			}
		})
	}
	t.Run("no result", func(t *testing.T) {
		if got := FineGrainedNotes(session, fgGraphQL, nil, nil); got != nil {
			t.Errorf("FineGrainedNotes(nil) = %+v, want nil", got)
		}
	})
	t.Run("no text block", func(t *testing.T) {
		image := &mcp.CallToolResult{Content: []mcp.Content{&mcp.ImageContent{MIMEType: "image/png"}}}
		got := FineGrainedNotes(session, fgGraphQL, image, emptyList{})
		if len(got.Content) != 1 {
			t.Errorf("FineGrainedNotes added content to an answer with no text block: %+v", got.Content)
		}
	})
	t.Run("an error with no text block", func(t *testing.T) {
		image := &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.ImageContent{MIMEType: "image/png"}}}
		got := FineGrainedNotes(session, fgGraphQL, image, nil)
		if len(got.Content) != 1 || resultText(got) != "" {
			t.Errorf("FineGrainedNotes changed an error answer with no text block: %+v", got.Content)
		}
	})
}

// TestFineGrainedErrorNote_AddsTheNullNoteToANotFoundGraphQLError verifies a
// handler's not-found error of a GraphQL action comes back with the null note
// after it and still unwraps to itself, and that every other error comes back
// as it went in.
func TestFineGrainedErrorNote_AddsTheNullNoteToANotFoundGraphQLError(t *testing.T) {
	session := fineGrainedContext()
	notFound := errors.New(`get_thing: thing "7" not found`)
	cases := []struct {
		name     string
		ctx      context.Context
		actionID string
		err      error
		noted    bool
	}{
		{name: "no error", ctx: session, actionID: fgGraphQL},
		{name: "a classic session", ctx: context.Background(), actionID: fgGraphQL, err: notFound},
		{name: "another error", ctx: session, actionID: fgGraphQL, err: errors.New("boom")},
		{name: "a REST action", ctx: session, actionID: fgAllowed, err: notFound},
		{name: "a GraphQL not-found error", ctx: session, actionID: fgGraphQL, err: notFound, noted: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FineGrainedErrorNote(tc.ctx, tc.actionID, tc.err)
			if !tc.noted {
				if got != tc.err { //nolint:errorlint // the very error is what the case expects back
					t.Errorf("FineGrainedErrorNote = %v, want %v unchanged", got, tc.err)
				}
				return
			}
			if !errors.Is(got, tc.err) {
				t.Errorf("FineGrainedErrorNote does not unwrap to the handler's error: %v", got)
			}
			want := tc.err.Error() + ". Over GraphQL, GitLab answers null with no error"
			if !strings.HasPrefix(got.Error(), want) {
				t.Errorf("FineGrainedErrorNote = %q, want it to begin %q", got.Error(), want)
			}
		})
	}
}

// TestEmptyCollection_IsAnEmptyListOutput verifies which outputs read as an
// empty list: an empty slice, and a struct whose one exported slice is empty,
// through any pointer or interface; never a struct with two lists or none, an
// unexported list, a nil, or a scalar.
func TestEmptyCollection_IsAnEmptyListOutput(t *testing.T) {
	type twoLists struct {
		A []string
		B []string
	}
	type hidden struct {
		items []string
	}
	var nilList *emptyList
	var boxed any = emptyList{}
	cases := []struct {
		name   string
		output any
		want   bool
	}{
		{name: "nil", output: nil},
		{name: "a nil pointer", output: nilList},
		{name: "an empty slice", output: []string{}, want: true},
		{name: "a slice with items", output: []string{"x"}},
		{name: "an empty list output", output: emptyList{}, want: true},
		{name: "a pointer to one", output: &emptyList{}, want: true},
		{name: "a pointer to an interface holding one", output: &boxed, want: true},
		{name: "a list output with items", output: emptyList{Items: []string{"x"}}},
		{name: "two lists", output: twoLists{}},
		{name: "an unexported list", output: hidden{items: []string{}}},
		{name: "a scalar", output: 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := emptyCollection(tc.output); got != tc.want {
				t.Errorf("emptyCollection(%#v) = %v, want %v", tc.output, got, tc.want)
			}
		})
	}
}

// TestAppendHints_ExtendsTheOneSectionTheServerReads verifies hints are added
// to the guidance section an answer already carries, leading or closing, and
// open a section of their own on an answer with none, so [ExtractHints] reads
// every one of them; blank hints add nothing.
func TestAppendHints_ExtendsTheOneSectionTheServerReads(t *testing.T) {
	var closing strings.Builder
	closing.WriteString("body")
	WriteHints(&closing, "first")
	var leading strings.Builder
	WriteHints(&leading, "first")
	leading.WriteString("\nbody\n")
	cases := []struct {
		name  string
		md    string
		hints []string
		want  []string
	}{
		{name: "no hints", md: "body", hints: []string{" ", ""}, want: nil},
		{name: "no section", md: "body", hints: []string{"added"}, want: []string{"added"}},
		{name: "a closing section", md: closing.String(), hints: []string{"added"}, want: []string{"first", "added"}},
		{name: "a leading section", md: leading.String(), hints: []string{"added"}, want: []string{"first", "added"}},
		{
			name: "a section that does not close the answer", md: closing.String() + "\nmore body",
			hints: []string{"added"}, want: []string{"added"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := AppendHints(tc.md, tc.hints...)
			if tc.want == nil && got != tc.md {
				t.Errorf("AppendHints = %q, want %q unchanged", got, tc.md)
			}
			if hints := ExtractHints(got); !equalStrings(hints, tc.want) {
				t.Errorf("ExtractHints(AppendHints) = %q, want %q\n%s", hints, tc.want, got)
			}
		})
	}
}

// equalStrings reports whether two lists hold the same strings in order, nil
// and empty alike.
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestBulletRunLength_CountsTheLeadingBulletLines verifies the length of a
// section's leading bullets counts each whole bullet line, and stops at the
// first line that is not one or has no newline.
func TestBulletRunLength_CountsTheLeadingBulletLines(t *testing.T) {
	cases := []struct {
		section string
		want    int
	}{
		{section: "", want: 0},
		{section: "- a\n- b\n\nbody", want: 8},
		{section: "- a\n- b", want: 4},
		{section: "body\n- a\n", want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.section, func(t *testing.T) {
			if got := bulletRunLength(tc.section); got != tc.want {
				t.Errorf("bulletRunLength(%q) = %d, want %d", tc.section, got, tc.want)
			}
		})
	}
}
