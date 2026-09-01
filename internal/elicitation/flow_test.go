// flow_test.go contains unit and integration tests for the multi round-trip
// (MRTR, SEP-2322) elicitation flow. Unit tests construct Flow values
// directly to validate pending-request queueing, answer replay, action
// mapping, and state encoding. Integration tests register real tools and
// drive them through a protocol 2026-07-28 client whose SDK middleware
// fulfills input requests and retries the call, validating the full loop
// including RequestState echo across rounds.
package elicitation

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// newMRTRFlow builds a Flow in multi round-trip mode with the given
// preloaded answers, bypassing session negotiation for unit tests.
func newMRTRFlow(answers map[string]answerRecord) *Flow {
	if answers == nil {
		answers = map[string]answerRecord{}
	}
	return &Flow{legacy: Client{session: &mcp.ServerSession{}, caps: formCapabilities()}, mrtr: true, answers: answers}
}

// TestFlow_Pending_QueuesInputRequest verifies that an unanswered prompt on
// the MRTR path returns ErrInputPending and that InputRequiredResult carries
// the queued elicitation request plus versioned request state.
func TestFlow_Pending_QueuesInputRequest(t *testing.T) {
	f := newMRTRFlow(nil)
	_, err := f.PromptText(context.Background(), "title", "Enter title", "title")
	if !errors.Is(err, ErrInputPending) {
		t.Fatalf("PromptText(no answer) error = %v, want ErrInputPending", err)
	}
	result := f.InputRequiredResult()
	if result.IsError {
		t.Fatal("InputRequiredResult().IsError = true, want false")
	}
	if len(result.Content) != 0 {
		t.Fatalf("InputRequiredResult() has %d content entries, want 0 (content and inputRequests are mutually exclusive)", len(result.Content))
	}
	req, ok := result.InputRequests["title"].(*mcp.ElicitParams)
	if !ok {
		t.Fatalf("InputRequests[title] = %T, want *mcp.ElicitParams", result.InputRequests["title"])
	}
	if req.Message != "Enter title" {
		t.Errorf("queued message = %q, want 'Enter title'", req.Message)
	}
	// The state is signed now, so its version is readable from the prefix
	// rather than from the JSON: nothing inside can be trusted before the MAC
	// has been checked, and the version has to be legible before that.
	if !strings.HasPrefix(result.RequestState, "v1.") {
		t.Errorf("RequestState = %q, want state tagged with its version", result.RequestState)
	}
}

// TestFlow_AcceptedAnswer_ReturnsValue verifies that a recorded accepted
// answer is replayed without queueing a new request.
func TestFlow_AcceptedAnswer_ReturnsValue(t *testing.T) {
	f := newMRTRFlow(map[string]answerRecord{
		"title": {Action: "accept", Content: map[string]any{"title": "my title"}},
	})
	got, err := f.PromptText(context.Background(), "title", "Enter title", "title")
	if err != nil {
		t.Fatalf("PromptText(answered) error = %v", err)
	}
	if got != "my title" {
		t.Errorf("PromptText(answered) = %q, want 'my title'", got)
	}
}

// TestFlow_DeclinedAnswer_ReturnsErrDeclined verifies decline mapping on the
// MRTR path.
func TestFlow_DeclinedAnswer_ReturnsErrDeclined(t *testing.T) {
	f := newMRTRFlow(map[string]answerRecord{"q": {Action: "decline"}})
	_, err := f.Confirm(context.Background(), "q", "Sure?")
	if !errors.Is(err, ErrDeclined) {
		t.Errorf("Confirm(declined) error = %v, want ErrDeclined", err)
	}
}

// TestFlow_CancelledAnswer_ReturnsErrCancelled verifies cancel mapping on
// the MRTR path.
func TestFlow_CancelledAnswer_ReturnsErrCancelled(t *testing.T) {
	f := newMRTRFlow(map[string]answerRecord{"q": {Action: "cancel"}})
	_, err := f.Confirm(context.Background(), "q", "Sure?")
	if !errors.Is(err, ErrCancelled) {
		t.Errorf("Confirm(cancelled) error = %v, want ErrCancelled", err)
	}
}

// TestFlow_UnknownAction_ReturnsError verifies that an unrecognized recorded
// action surfaces as an error rather than a value.
func TestFlow_UnknownAction_ReturnsError(t *testing.T) {
	f := newMRTRFlow(map[string]answerRecord{"q": {Action: "weird"}})
	_, err := f.Confirm(context.Background(), "q", "Sure?")
	if err == nil || !strings.Contains(err.Error(), "unknown action") {
		t.Errorf("Confirm(unknown action) error = %v, want 'unknown action'", err)
	}
}

// TestFlow_SelectOne_RejectsUnknownOption verifies defense-in-depth option
// validation of replayed answers.
func TestFlow_SelectOne_RejectsUnknownOption(t *testing.T) {
	f := newMRTRFlow(map[string]answerRecord{
		"vis": {Action: "accept", Content: map[string]any{"selection": "hacked"}},
	})
	_, err := f.SelectOne(context.Background(), "vis", "Pick", []string{"private", "public"})
	if err == nil || !strings.Contains(err.Error(), "not in the allowed options") {
		t.Errorf("SelectOne(invalid option) error = %v, want allowed-options validation error", err)
	}
}

// TestFlow_TypedPrompts_ReplayAnswers verifies answer replay for the
// remaining typed prompt helpers (multi-select, integer select, number,
// arbitrary schema).
func TestFlow_TypedPrompts_ReplayAnswers(t *testing.T) {
	newFlow := func() *Flow {
		return newMRTRFlow(map[string]answerRecord{
			"langs":  {Action: "accept", Content: map[string]any{"selections": []any{"go", "rust"}}},
			"count":  {Action: "accept", Content: map[string]any{"selection": float64(3)}},
			"rating": {Action: "accept", Content: map[string]any{"rating": 4.5}},
			"form":   {Action: "accept", Content: map[string]any{"a": "b"}},
		})
	}
	tests := []struct {
		name  string
		check func(t *testing.T, f *Flow)
	}{
		{"SelectMulti replays selections", func(t *testing.T, f *Flow) {
			t.Helper()
			langs, err := f.SelectMulti(t.Context(), "langs", "Pick languages", []string{"go", "rust", "zig"}, 1, 0)
			if err != nil || len(langs) != 2 {
				t.Errorf("SelectMulti = (%v, %v), want 2 selections", langs, err)
			}
		}},
		{"SelectOneInt replays selection", func(t *testing.T, f *Flow) {
			t.Helper()
			count, err := f.SelectOneInt(t.Context(), "count", "Pick count", []int{1, 2, 3})
			if err != nil || count != 3 {
				t.Errorf("SelectOneInt = (%d, %v), want 3", count, err)
			}
		}},
		{"PromptNumber replays value", func(t *testing.T, f *Flow) {
			t.Helper()
			rating, err := f.PromptNumber(t.Context(), "rating", "Rate", "rating", 0, 5)
			if err != nil || rating != 4.5 {
				t.Errorf("PromptNumber = (%g, %v), want 4.5", rating, err)
			}
		}},
		{"GatherData replays content", func(t *testing.T, f *Flow) {
			t.Helper()
			form, err := f.GatherData(t.Context(), "form", "Fill", map[string]any{"type": "object"})
			if err != nil || form["a"] != "b" {
				t.Errorf("GatherData = (%v, %v), want map with a=b", form, err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check(t, newFlow())
		})
	}
}

// TestFlow_NotSupported_ReturnsSentinel verifies that a flow without
// elicitation support rejects every prompt with ErrElicitationNotSupported,
// on both the synchronous and the multi round-trip paths.
//
// Each method carries its own guard, so exercising one proves nothing about
// the other seven: a method that dropped its check would reach a nil session
// and panic instead of returning the sentinel every caller branches on.
// ElicitURL is the one that genuinely differs between the two paths — it
// dispatches on mrtr before checking support — which is why both are run.
func TestFlow_NotSupported_ReturnsSentinel(t *testing.T) {
	ctx := context.Background()
	prompts := []struct {
		name string
		call func(*Flow) error
	}{
		{"Confirm", func(f *Flow) error { _, err := f.Confirm(ctx, "q", "Sure?"); return err }},
		{"PromptText", func(f *Flow) error { _, err := f.PromptText(ctx, "q", "Title?", "title"); return err }},
		{"SelectOne", func(f *Flow) error { _, err := f.SelectOne(ctx, "q", "Pick", []string{"a"}); return err }},
		{"SelectMulti", func(f *Flow) error { _, err := f.SelectMulti(ctx, "q", "Pick", []string{"a"}, 1, 1); return err }},
		{"SelectOneInt", func(f *Flow) error { _, err := f.SelectOneInt(ctx, "q", "Pick", []int{1}); return err }},
		{"PromptNumber", func(f *Flow) error { _, err := f.PromptNumber(ctx, "q", "How many?", "n", 0, 10); return err }},
		{"GatherData", func(f *Flow) error {
			_, err := f.GatherData(ctx, "q", "Data", map[string]any{"type": "object"})
			return err
		}},
		{"ElicitURL", func(f *Flow) error {
			return f.ElicitURL(ctx, "q", "https://gitlab.example.com", "https://gitlab.example.com/x", "Open")
		}},
	}

	for _, path := range []struct {
		name string
		mrtr bool
	}{{"synchronous", false}, {"multi round-trip", true}} {
		for _, tt := range prompts {
			t.Run(path.name+"/"+tt.name, func(t *testing.T) {
				f := &Flow{mrtr: path.mrtr, answers: map[string]answerRecord{}}
				if err := tt.call(f); !errors.Is(err, ErrElicitationNotSupported) {
					t.Errorf("%s(no support) error = %v, want ErrElicitationNotSupported", tt.name, err)
				}
			})
		}
	}
}

// TestFlow_EmptyOptions_Rejected verifies that the three option-list prompts
// refuse an empty list instead of queueing it.
//
// An elicitation schema with an empty enum is one no answer can satisfy, so a
// client would be asked to choose from nothing and the call could never make
// progress. Failing at the call site names the caller's bug instead.
func TestFlow_EmptyOptions_Rejected(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		call func(*Flow) error
	}{
		{"SelectOne", func(f *Flow) error { _, err := f.SelectOne(ctx, "q", "Pick", nil); return err }},
		{"SelectMulti", func(f *Flow) error { _, err := f.SelectMulti(ctx, "q", "Pick", nil, 1, 1); return err }},
		{"SelectOneInt", func(f *Flow) error { _, err := f.SelectOneInt(ctx, "q", "Pick", nil); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newMRTRFlow(nil)
			err := tt.call(f)
			if err == nil || !strings.Contains(err.Error(), errOptionsEmpty) {
				t.Errorf("%s(no options) error = %v, want %q", tt.name, err, errOptionsEmpty)
			}
			if len(f.pending) != 0 {
				t.Errorf("%s queued %d input requests for an unanswerable prompt, want none", tt.name, len(f.pending))
			}
		})
	}
}

// TestFlow_TypedPrompts_QueueWhenUnanswered verifies that every typed prompt
// queues an input request and reports ErrInputPending on the first round, and
// that the two prompts taking a field name fall back to "value" when the
// caller passes none.
//
// The fallback is not cosmetic. The queued schema and the parser that reads
// the answer back derive the property name separately, so a prompt that
// queued "value" while its parser looked for "" would send the user a form,
// receive a valid answer, and then fail to find it.
func TestFlow_TypedPrompts_QueueWhenUnanswered(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		call func(*Flow) error
		// wantField is the schema property the queued request must declare,
		// or "" for prompts whose property name is not caller-supplied.
		wantField string
	}{
		{"PromptText", func(f *Flow) error { _, err := f.PromptText(ctx, "q", "Title?", ""); return err }, "value"},
		{"PromptNumber", func(f *Flow) error { _, err := f.PromptNumber(ctx, "q", "How many?", "", 0, 10); return err }, "value"},
		{"SelectOne", func(f *Flow) error { _, err := f.SelectOne(ctx, "q", "Pick", []string{"a", "b"}); return err }, ""},
		{"SelectMulti", func(f *Flow) error {
			_, err := f.SelectMulti(ctx, "q", "Pick", []string{"a", "b"}, 1, 2)
			return err
		}, ""},
		{"SelectOneInt", func(f *Flow) error { _, err := f.SelectOneInt(ctx, "q", "Pick", []int{1, 2}); return err }, ""},
		{"GatherData", func(f *Flow) error {
			_, err := f.GatherData(ctx, "q", "Data", map[string]any{"type": "object"})
			return err
		}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newMRTRFlow(nil)
			if err := tt.call(f); !errors.Is(err, ErrInputPending) {
				t.Fatalf("%s(unanswered) error = %v, want ErrInputPending", tt.name, err)
			}
			queued, ok := f.pending["q"].(*mcp.ElicitParams)
			if !ok {
				t.Fatalf("%s queued %T, want *mcp.ElicitParams", tt.name, f.pending["q"])
			}
			if tt.wantField == "" {
				return
			}
			schema, _ := queued.RequestedSchema.(map[string]any)
			props, _ := schema["properties"].(map[string]any)
			if _, declared := props[tt.wantField]; !declared {
				t.Errorf("%s queued schema properties = %v, want a %q property", tt.name, props, tt.wantField)
			}
		})
	}
}

// urlFlow builds a multi round-trip Flow whose session advertises URL-mode
// elicitation. The synchronous elicitation handler fails the test: on this
// path the flow must queue an input request, never send one, and routing a
// URL prompt through the legacy client is exactly the regression that would
// otherwise pass unnoticed.
func urlFlow(t *testing.T, answers map[string]answerRecord) *Flow {
	t.Helper()
	_, ss, cleanup := setupElicitURLSession(t, context.Background(),
		func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			t.Error("the multi round-trip path sent a synchronous elicitation request")
			return nil, errors.New("unexpected synchronous elicitation")
		})
	t.Cleanup(cleanup)
	if answers == nil {
		answers = map[string]answerRecord{}
	}
	return &Flow{legacy: Client{session: ss, caps: urlCapabilities()}, mrtr: true, answers: answers}
}

// elicitURLBase and elicitURLTarget are the instance and in-instance page the
// URL-mode tests prompt for.
const (
	elicitURLBase   = "https://gitlab.example.com"
	elicitURLTarget = elicitURLBase + "/group/project/-/issues/1"
)

// TestFlow_ElicitURL_RefusesWhatItCannotSend verifies the two checks that run
// before anything is queued: the client must actually support URL mode, and
// the target must belong to the instance the caller named.
//
// The host check is the security-relevant one. The URL is rendered to the user
// as somewhere their GitLab session will follow, so a prompt pointing off the
// instance turns an ordinary tool call into a link the user has been told to
// trust. Neither refusal may leave a queued request behind, or the next round
// would send the very prompt that was just rejected.
func TestFlow_ElicitURL_RefusesWhatItCannotSend(t *testing.T) {
	t.Run("client without URL capability", func(t *testing.T) {
		// newMRTRFlow's bare session advertises form elicitation but not URL.
		f := newMRTRFlow(nil)
		err := f.ElicitURL(context.Background(), "u", elicitURLBase, elicitURLTarget, "Open")
		if !errors.Is(err, ErrURLElicitationNotSupported) {
			t.Errorf("ElicitURL(form-only client) error = %v, want ErrURLElicitationNotSupported", err)
		}
		if len(f.pending) != 0 {
			t.Error("a prompt the client cannot render was still queued")
		}
	})

	t.Run("target outside the instance", func(t *testing.T) {
		f := urlFlow(t, nil)
		err := f.ElicitURL(context.Background(), "u", elicitURLBase, "https://elsewhere.example.net/x", "Open")
		if err == nil || !strings.Contains(err.Error(), "does not match GitLab instance") {
			t.Errorf("ElicitURL(foreign host) error = %v, want a host-mismatch error", err)
		}
		if len(f.pending) != 0 {
			t.Error("a rejected URL was still queued for the client")
		}
	})
}

// TestFlow_ElicitURL_QueuesThenReplays verifies the round trip itself: an
// unanswered prompt is queued in url mode, and a recorded answer resolves on
// the next round without prompting again.
//
// URL mode is the one prompt whose answer carries no data, only an action, so
// its replay path returns the action's error and nothing else. A regression
// there would not fail loudly — it would silently re-prompt the user on every
// round, and the call would never finish.
func TestFlow_ElicitURL_QueuesThenReplays(t *testing.T) {
	t.Run("unanswered queues a url-mode request", func(t *testing.T) {
		f := urlFlow(t, nil)
		err := f.ElicitURL(context.Background(), "u", elicitURLBase, elicitURLTarget, "Open the issue")
		if !errors.Is(err, ErrInputPending) {
			t.Fatalf("ElicitURL(unanswered) error = %v, want ErrInputPending", err)
		}
		queued, ok := f.pending["u"].(*mcp.ElicitParams)
		if !ok {
			t.Fatalf("ElicitURL queued %T, want *mcp.ElicitParams", f.pending["u"])
		}
		if queued.Mode != "url" {
			t.Errorf("queued Mode = %q, want %q", queued.Mode, "url")
		}
		if queued.URL != elicitURLTarget {
			t.Errorf("queued URL = %q, want %q", queued.URL, elicitURLTarget)
		}
		if queued.Message != "Open the issue" {
			t.Errorf("queued Message = %q, want the caller's message", queued.Message)
		}
	})

	t.Run("canceled context stops before queueing", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		f := urlFlow(t, nil)
		if err := f.ElicitURL(ctx, "u", elicitURLBase, elicitURLTarget, "Open"); !errors.Is(err, context.Canceled) {
			t.Errorf("ElicitURL(canceled ctx) error = %v, want context.Canceled", err)
		}
		if len(f.pending) != 0 {
			t.Error("a canceled call still queued an input request")
		}
	})

	replays := []struct {
		name    string
		action  string
		wantErr error
	}{
		{"accepted resolves", "accept", nil},
		{"declined surfaces ErrDeclined", "decline", ErrDeclined},
		{"cancelled surfaces ErrCancelled", "cancel", ErrCancelled},
	}
	for _, tt := range replays {
		t.Run(tt.name, func(t *testing.T) {
			f := urlFlow(t, map[string]answerRecord{"u": {Action: tt.action}})
			err := f.ElicitURL(context.Background(), "u", elicitURLBase, elicitURLTarget, "Open")
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("ElicitURL(%s) error = %v, want %v", tt.action, err, tt.wantErr)
			}
			if len(f.pending) != 0 {
				t.Error("an already-answered prompt was queued again")
			}
		})
	}
}

// TestFlow_ContextCancelled_StopsBeforeQueueing verifies that a canceled
// context aborts instead of queueing an input request.
func TestFlow_ContextCancelled_StopsBeforeQueueing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f := newMRTRFlow(nil)
	if _, err := f.Confirm(ctx, "q", "Sure?"); !errors.Is(err, context.Canceled) {
		t.Errorf("Confirm(canceled ctx) error = %v, want context.Canceled", err)
	}
}

// TestFlow_PendingError_CarriesResult verifies that PendingError wraps the
// input-required result for error-returning handlers and that surface code
// can recover it with errors.AsType.
func TestFlow_PendingError_CarriesResult(t *testing.T) {
	f := newMRTRFlow(nil)
	_, _ = f.PromptText(context.Background(), "title", "Enter title", "title")
	err := f.PendingError()
	inputErr, ok := errors.AsType[*InputRequiredError](err)
	if !ok {
		t.Fatalf("PendingError() type = %T, want *InputRequiredError", err)
	}
	if inputErr.Result() == nil || inputErr.Result().InputRequests == nil {
		t.Fatal("PendingError().Result() missing input requests")
	}
	if inputErr.Error() == "" {
		t.Error("InputRequiredError.Error() is empty")
	}
}

// Integration tests: full multi round-trip loop over a real 2026-07-28
// session, where the SDK client middleware fulfills input requests and
// retries the tools/call.

// flowTestTool registers a tool whose handler runs fn with a Flow built
// from the live request and connects a new-protocol client with the given
// elicitation handler. Returns the client session for CallTool round-trips.
func flowTestTool(t *testing.T, fn func(context.Context, *mcp.CallToolRequest, *Flow) (*mcp.CallToolResult, error), elicit func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error)) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()

	server := mcp.NewServer(testImpl, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "flow_tool",
		Description: "flow test tool",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
		fl, err := FlowFromRequest(req)
		if err != nil {
			//nolint:nilerr // the flow error is surfaced in-band as an error tool result
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}, IsError: true}, nil, nil
		}
		result, err := fn(ctx, req, fl)
		return result, nil, err
	})

	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(testImpl, &mcp.ClientOptions{ElicitationHandler: elicit})
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		_ = ss.Close()
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		_ = cs.Close()
		_ = ss.Close()
	})
	return cs
}

// textResult builds a plain text tool result.
func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

// resultText concatenates the text content of a tool result.
func resultText(result *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

// confirmFlowHandler is a tool handler that requires one confirmation.
func confirmFlowHandler(ctx context.Context, _ *mcp.CallToolRequest, fl *Flow) (*mcp.CallToolResult, error) {
	confirmed, err := fl.Confirm(ctx, "confirm", "Proceed?")
	switch {
	case errors.Is(err, ErrInputPending):
		return fl.InputRequiredResult(), nil
	case errors.Is(err, ErrDeclined), errors.Is(err, ErrCancelled):
		return textResult("canceled"), nil
	case err != nil:
		return nil, err
	}
	if !confirmed {
		return textResult("canceled"), nil
	}
	return textResult("confirmed"), nil
}

// TestFlow_MRTR_ConfirmAccepted verifies the full multi round-trip loop for
// an accepted confirmation: round one returns an input-required result, the
// client middleware fulfills it via the elicitation handler, and the retried
// call proceeds.
func TestFlow_MRTR_ConfirmAccepted(t *testing.T) {
	var elicitations atomic.Int32
	cs := flowTestTool(t, confirmFlowHandler, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		elicitations.Add(1)
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirmed": true}}, nil
	})

	result, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "flow_tool", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if got := resultText(result); got != "confirmed" {
		t.Errorf("result text = %q, want 'confirmed'", got)
	}
	if elicitations.Load() != 1 {
		t.Errorf("elicitations = %d, want 1", elicitations.Load())
	}
}

// TestFlow_MRTR_ConfirmDeclined verifies the full multi round-trip loop for
// a declined confirmation.
func TestFlow_MRTR_ConfirmDeclined(t *testing.T) {
	cs := flowTestTool(t, confirmFlowHandler, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "decline"}, nil
	})

	result, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "flow_tool", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if got := resultText(result); got != "canceled" {
		t.Errorf("result text = %q, want 'canceled'", got)
	}
}

// TestFlow_MRTR_MultiStepAccumulatesState verifies that a multi-step flow
// survives handler re-invocation: earlier answers are carried through the
// opaque RequestState and each round only elicits the next unanswered
// prompt, in order.
func TestFlow_MRTR_MultiStepAccumulatesState(t *testing.T) {
	// The elicitation handler runs on the client session's goroutine, so
	// guard the recorded messages against concurrent access.
	var mu sync.Mutex
	var messages []string
	answers := map[string]map[string]any{
		"Enter first":  {"first": "hello"},
		"Enter second": {"second": "world"},
		"Proceed?":     {"confirmed": true},
	}
	handler := func(ctx context.Context, req *mcp.CallToolRequest, fl *Flow) (*mcp.CallToolResult, error) {
		if !fl.UsesMultiRoundTrip() {
			return textResult("expected multi round-trip session"), nil
		}
		first, err := fl.PromptText(ctx, "first", "Enter first", "first")
		if errors.Is(err, ErrInputPending) {
			return fl.InputRequiredResult(), nil
		} else if err != nil {
			return nil, err
		}
		second, err := fl.PromptText(ctx, "second", "Enter second", "second")
		if errors.Is(err, ErrInputPending) {
			return fl.InputRequiredResult(), nil
		} else if err != nil {
			return nil, err
		}
		confirmed, err := fl.Confirm(ctx, "confirm", "Proceed?")
		if errors.Is(err, ErrInputPending) {
			return fl.InputRequiredResult(), nil
		} else if err != nil {
			return nil, err
		}
		if !confirmed {
			return textResult("canceled"), nil
		}
		return textResult(first + "|" + second), nil
	}

	cs := flowTestTool(t, handler, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		mu.Lock()
		messages = append(messages, req.Params.Message)
		mu.Unlock()
		content, ok := answers[req.Params.Message]
		if !ok {
			return nil, errors.New("unexpected elicitation: " + req.Params.Message)
		}
		return &mcp.ElicitResult{Action: "accept", Content: content}, nil
	})

	result, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "flow_tool", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if got := resultText(result); got != "hello|world" {
		t.Errorf("result text = %q, want 'hello|world'", got)
	}
	want := []string{"Enter first", "Enter second", "Proceed?"}
	mu.Lock()
	defer mu.Unlock()
	if len(messages) != len(want) {
		t.Fatalf("elicitation messages = %v, want %v", messages, want)
	}
	for i, msg := range want {
		t.Run(msg, func(t *testing.T) {
			if messages[i] != msg {
				t.Errorf("elicitation %d = %q, want %q", i, messages[i], msg)
			}
		})
	}
}

// TestFlow_MRTR_InvalidRequestState verifies that a corrupted client-echoed
// RequestState is rejected as an error instead of silently restarting the
// flow.
func TestFlow_MRTR_InvalidRequestState(t *testing.T) {
	handler := func(ctx context.Context, req *mcp.CallToolRequest, _ *Flow) (*mcp.CallToolResult, error) {
		req.Params.RequestState = "{not json"
		if _, err := FlowFromRequest(req); err != nil {
			//nolint:nilerr // the rejection is asserted in-band via the result text
			return textResult("state rejected: " + err.Error()), nil
		}
		return textResult("state accepted"), nil
	}
	cs := flowTestTool(t, handler, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirmed": true}}, nil
	})

	result, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "flow_tool", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if got := resultText(result); !strings.Contains(got, "state rejected") {
		t.Errorf("result text = %q, want state rejection", got)
	}
}

// TestFlow_MRTR_UnsupportedStateVersion verifies that well-formed request
// state carrying a version this build does not know is rejected.
//
// The state is opaque to the client, which echoes back whatever it was given,
// so a version bump means a client mid-conversation can hand back a payload
// written by the previous build. Decoding it into the current shape would
// silently misread the accumulated answers; refusing it restarts the flow,
// which costs one round trip and cannot corrupt anything.
func TestFlow_MRTR_UnsupportedStateVersion(t *testing.T) {
	handler := func(_ context.Context, req *mcp.CallToolRequest, _ *Flow) (*mcp.CallToolResult, error) {
		// Shaped as this build emits state, with a version it does not know.
		// The point of the prefix is exactly this: the version is legible
		// before the signature, so a payload from another build is refused for
		// what it is rather than for failing a check it was never given.
		req.Params.RequestState = "v99.eyJ2Ijo5OX0.bm90LWEtdmFsaWQtbWFj"
		if _, err := FlowFromRequest(req); err != nil {
			//nolint:nilerr // the rejection is asserted in-band via the result text
			return textResult("state rejected: " + err.Error()), nil
		}
		return textResult("state accepted"), nil
	}
	cs := flowTestTool(t, handler, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "accept"}, nil
	})

	result, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "flow_tool", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	got := resultText(result)
	if !strings.Contains(got, "state rejected") {
		t.Fatalf("result text = %q, want the state to be rejected", got)
	}
	if !strings.Contains(got, `unsupported requestState version "v99"`) {
		t.Errorf("result text = %q, want the offending version named", got)
	}
}

// TestFlow_MRTR_NoParams_StillUsesMultiRoundTrip verifies that a request
// carrying no params still yields a multi round-trip flow.
//
// The mechanism is chosen by the negotiated protocol version, not by whether
// this particular call happens to carry state. Falling back to the
// synchronous path here would send a server-initiated elicitation request on
// a session where the specification forbids one.
func TestFlow_MRTR_NoParams_StillUsesMultiRoundTrip(t *testing.T) {
	handler := func(_ context.Context, req *mcp.CallToolRequest, _ *Flow) (*mcp.CallToolResult, error) {
		req.Params = nil
		fl, err := FlowFromRequest(req)
		if err != nil {
			//nolint:nilerr // the outcome is asserted in-band via the result text
			return textResult("unexpected error: " + err.Error()), nil
		}
		if !fl.UsesMultiRoundTrip() {
			return textResult("downgraded to the synchronous path"), nil
		}
		return textResult("multi round-trip"), nil
	}
	cs := flowTestTool(t, handler, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "accept"}, nil
	})

	result, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "flow_tool", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if got := resultText(result); got != "multi round-trip" {
		t.Errorf("result text = %q, want %q", got, "multi round-trip")
	}
}

// TestFlow_MRTR_UnexpectedResponseType verifies that an input response of a
// type this flow never requests (anything but *mcp.ElicitResult) is rejected
// as an error instead of being silently dropped and re-queued forever.
func TestFlow_MRTR_UnexpectedResponseType(t *testing.T) {
	handler := func(_ context.Context, req *mcp.CallToolRequest, _ *Flow) (*mcp.CallToolResult, error) {
		//nolint:staticcheck // deliberately uses a non-elicitation InputResponse type to exercise the mismatch rejection; all such types are deprecated upstream
		req.Params.InputResponses = mcp.InputResponseMap{"confirm": &mcp.ListRootsResult{}}
		if _, err := FlowFromRequest(req); err != nil {
			//nolint:nilerr // the rejection is asserted in-band via the result text
			return textResult("response rejected: " + err.Error()), nil
		}
		return textResult("response accepted"), nil
	}
	cs := flowTestTool(t, handler, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirmed": true}}, nil
	})

	result, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "flow_tool", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if got := resultText(result); !strings.Contains(got, "response rejected") {
		t.Errorf("result text = %q, want unexpected-type rejection", got)
	}
}

// TestFlowFromRequest_LegacySession_UsesSynchronousPath verifies that a
// session negotiated below 2026-07-28 keeps the synchronous elicitation
// mechanism behind the Flow API.
func TestFlowFromRequest_LegacySession_UsesSynchronousPath(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirmed": true}}, nil
	})
	defer cleanup()

	fl, err := FlowFromRequest(&mcp.CallToolRequest{Session: ss})
	if err != nil {
		t.Fatalf("FlowFromRequest error: %v", err)
	}
	if fl.UsesMultiRoundTrip() {
		t.Fatal("UsesMultiRoundTrip() = true on a legacy session, want false")
	}
	confirmed, err := fl.Confirm(ctx, "confirm", "Proceed?")
	if err != nil {
		t.Fatalf("Confirm(legacy) error = %v", err)
	}
	if !confirmed {
		t.Error("Confirm(legacy) = false, want true")
	}
}

// formCapabilities and urlCapabilities are the two client shapes the flow tests
// need. They are stated explicitly because a Client now captures the
// capabilities of the request that built it rather than reading them from the
// session: from 2026-07-28 a client declares them per request, so a test that
// leaves them unset is describing a client that declared nothing.
func formCapabilities() *mcp.ClientCapabilities {
	return &mcp.ClientCapabilities{
		Elicitation: &mcp.ElicitationCapabilities{Form: &mcp.FormElicitationCapabilities{}},
	}
}

func urlCapabilities() *mcp.ClientCapabilities {
	return &mcp.ClientCapabilities{
		Elicitation: &mcp.ElicitationCapabilities{
			Form: &mcp.FormElicitationCapabilities{},
			URL:  &mcp.URLElicitationCapabilities{},
		},
	}
}

// TestFlow_MRTR_TwoRounds_CarriesTheAnswerForward is the end-to-end check that
// binding the state to its call did not break the flow that state exists for.
//
// This is the failure the binding could introduce, and it is worse than the one
// it fixes: if the digest computed in round two disagreed with round one — for
// any reason, a client re-serializing its own arguments included — the answers
// would be dropped, the same question re-queued, and the flow would never
// finish. Tests that only check that foreign state is refused cannot see it,
// because refusing everything passes them.
//
// The SDK client drives both rounds itself: it answers the queued input request
// through its ElicitationHandler and retries the call with the state echoed
// back. That is what makes this worth running here rather than hand-rolling the
// second call — the retry is built the way a real client builds it, not the way
// this test imagines it.
func TestFlow_MRTR_TwoRounds_CarriesTheAnswerForward(t *testing.T) {
	var rounds int
	handler := func(ctx context.Context, _ *mcp.CallToolRequest, f *Flow) (*mcp.CallToolResult, error) {
		rounds++
		title, err := f.PromptText(ctx, "title", "Enter title", "title")
		if err != nil {
			if errors.Is(err, ErrInputPending) {
				return f.InputRequiredResult(), nil
			}
			return nil, err
		}
		return textResult("got title: " + title), nil
	}
	cs := flowTestTool(t, handler, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"title": "the answer"}}, nil
	})

	result, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "flow_tool",
		Arguments: map[string]any{"project_id": "7", "title_hint": "something"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if got := resultText(result); !strings.Contains(got, "got title: the answer") {
		t.Fatalf("the flow did not carry the answer forward: %q", got)
	}
	if len(result.InputRequests) != 0 {
		t.Error("the final result still queues a question that was answered")
	}
	// Exactly two: one that queued the request and one that read the answer. A
	// larger number would mean the state was not being honored and the flow was
	// going round again.
	if rounds != 2 {
		t.Errorf("the handler ran %d times, want 2 (queue, then resolve)", rounds)
	}
}

// TestGatherData_ValidatesTheAnswerAgainstItsSchema pins the one prompt where
// nothing else checks what came back.
//
// "Servers SHOULD validate received data matches the requested schema." The
// legacy path gets this from the SDK, inside Elicit. The multi round-trip path
// never calls Elicit, so on that path an answer reaches the handler exactly as
// the client sent it.
//
// The typed prompts each validate their own answers with better messages, so
// this is deliberately scoped to GatherData, which takes an arbitrary schema
// and returns the content untouched.
func TestGatherData_ValidatesTheAnswerAgainstItsSchema(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"count": map[string]any{"type": "number"},
			"name":  map[string]any{"type": "string"},
		},
		"required": []string{"count", "name"},
	}

	tests := []struct {
		name    string
		content map[string]any
		wantErr bool
	}{
		{
			name:    "an answer matching the schema",
			content: map[string]any{"count": 3.0, "name": "ok"},
		},
		{
			name:    "a field of the wrong type",
			content: map[string]any{"count": "three", "name": "ok"},
			wantErr: true,
		},
		{
			name:    "a required field missing",
			content: map[string]any{"count": 3.0},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newMRTRFlow(map[string]answerRecord{
				"data": {Action: "accept", Content: tt.content},
			})

			got, err := f.GatherData(context.Background(), "data", "Fill this in", schema)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("an answer that does not satisfy the schema was accepted: %v", got)
				}
				if !errors.Is(err, ErrMalformedAnswer) {
					t.Errorf("err = %v, want it classified as a malformed answer rather than a user decision", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("a conforming answer was refused: %v", err)
			}
		})
	}
}

// TestTypedPrompts_KeepTheirOwnValidation records the line that schema
// validation deliberately does not cross.
//
// Two typed prompts accept more than their schema advertises, on purpose. The
// 2026-07-28 elicitation subset has no integer enum — PrimitiveSchemaDefinition
// is String | Number | Boolean | Enum, and every enum variant is string-typed —
// so an integer choice is offered as decimal strings and parsed back, and a
// client that answers with the number instead is honored rather than refused.
//
// Validating those against their own schema would reject a client that got the
// answer right, in order to enforce a shape the protocol forced on us. This
// test exists so that a later pass which "finishes" schema validation has to
// decide that deliberately rather than by accident.
func TestTypedPrompts_KeepTheirOwnValidation(t *testing.T) {
	t.Run("an integer choice answered as a number is accepted", func(t *testing.T) {
		f := newMRTRFlow(map[string]answerRecord{
			"days": {Action: "accept", Content: map[string]any{"selection": 30.0}},
		})
		got, err := f.SelectOneInt(context.Background(), "days", "How many days?", []int{7, 30, 90})
		if err != nil {
			t.Fatalf("a number answer to a string-typed enum was refused: %v", err)
		}
		if got != 30 {
			t.Errorf("got %d, want 30", got)
		}
	})

	t.Run("an option outside the list is still refused, in its own words", func(t *testing.T) {
		f := newMRTRFlow(map[string]answerRecord{
			"vis": {Action: "accept", Content: map[string]any{"selection": "hacked"}},
		})
		_, err := f.SelectOne(context.Background(), "vis", "Visibility?", []string{"private", "public"})
		if err == nil {
			t.Fatal("an option outside the list was accepted")
		}
		if !strings.Contains(err.Error(), "allowed options") {
			t.Errorf("err = %v, want the prompt's own message rather than a schema path", err)
		}
	})
}

// TestConfirmSchema_DefaultsToDeclining pins the one default this package
// emits.
//
// "Clients that support defaults SHOULD pre-populate form fields with these
// values." For a destructive confirmation the safe starting point is obvious,
// and a client that pre-populates then opens the dialog on "no", so an
// accidental Enter declines rather than approves. The other builders carry no
// default on purpose: suggesting an answer to an open question is not a
// convenience.
func TestConfirmSchema_DefaultsToDeclining(t *testing.T) {
	props, ok := confirmSchema("Delete it?")["properties"].(map[string]any)
	if !ok {
		t.Fatal("confirmSchema has no properties")
	}
	field, ok := props["confirmed"].(map[string]any)
	if !ok {
		t.Fatal("confirmSchema has no 'confirmed' property")
	}
	value, present := field["default"]
	if !present {
		t.Fatal("the confirmation field carries no default, so a pre-populating client chooses for itself")
	}
	if value != false {
		t.Errorf("default = %v, want false: a destructive dialog must not open pre-approved", value)
	}

	others := []struct {
		name   string
		schema map[string]any
	}{
		{name: "text", schema: textSchema("m", "f")},
		{name: "select_one", schema: selectOneSchema("m", []string{"a"})},
		{name: "number", schema: numberSchema("m", "f", 0, 10)},
	}
	for _, tc := range others {
		t.Run(tc.name, func(t *testing.T) {
			otherProps, _ := tc.schema["properties"].(map[string]any)
			for name, raw := range otherProps {
				if otherField, isMap := raw.(map[string]any); isMap {
					if _, hasDefault := otherField["default"]; hasDefault {
						t.Errorf("%q carries a default; only the confirmation is meant to suggest an answer", name)
					}
				}
			}
		})
	}
}

// TestGatherData_AnAcceptWithNoContentStillValidates covers an accept that
// carries nothing against a schema that requires something.
//
// The validator skipped nil content entirely, so a client accepting without a
// body handed the handler nil despite required fields. An empty answer is an
// answer, and required fields have to fail it.
func TestGatherData_AnAcceptWithNoContentStillValidates(t *testing.T) {
	err := validateAgainstSchema(map[string]any{
		"type":     "object",
		"required": []any{"name"},
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
	}, nil)

	if err == nil {
		t.Error("an accept with no content passed a schema that requires a field")
	}
}
