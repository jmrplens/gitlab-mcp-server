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
	return &Flow{legacy: Client{session: &mcp.ServerSession{}}, mrtr: true, answers: answers}
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
	if !strings.Contains(result.RequestState, `"v":1`) {
		t.Errorf("RequestState = %q, want versioned state", result.RequestState)
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
	ctx := context.Background()
	f := newMRTRFlow(map[string]answerRecord{
		"langs":  {Action: "accept", Content: map[string]any{"selections": []any{"go", "rust"}}},
		"count":  {Action: "accept", Content: map[string]any{"selection": float64(3)}},
		"rating": {Action: "accept", Content: map[string]any{"rating": 4.5}},
		"form":   {Action: "accept", Content: map[string]any{"a": "b"}},
	})

	langs, err := f.SelectMulti(ctx, "langs", "Pick languages", []string{"go", "rust", "zig"}, 1, 0)
	if err != nil || len(langs) != 2 {
		t.Errorf("SelectMulti = (%v, %v), want 2 selections", langs, err)
	}
	count, err := f.SelectOneInt(ctx, "count", "Pick count", []int{1, 2, 3})
	if err != nil || count != 3 {
		t.Errorf("SelectOneInt = (%d, %v), want 3", count, err)
	}
	rating, err := f.PromptNumber(ctx, "rating", "Rate", "rating", 0, 5)
	if err != nil || rating != 4.5 {
		t.Errorf("PromptNumber = (%g, %v), want 4.5", rating, err)
	}
	form, err := f.GatherData(ctx, "form", "Fill", map[string]any{"type": "object"})
	if err != nil || form["a"] != "b" {
		t.Errorf("GatherData = (%v, %v), want map with a=b", form, err)
	}
}

// TestFlow_NotSupported_ReturnsSentinel verifies that a flow without
// elicitation support rejects every prompt with ErrElicitationNotSupported.
func TestFlow_NotSupported_ReturnsSentinel(t *testing.T) {
	f := &Flow{answers: map[string]answerRecord{}}
	if _, err := f.Confirm(context.Background(), "q", "Sure?"); !errors.Is(err, ErrElicitationNotSupported) {
		t.Errorf("Confirm(no support) error = %v, want ErrElicitationNotSupported", err)
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
		messages = append(messages, req.Params.Message)
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
	if len(messages) != len(want) {
		t.Fatalf("elicitation messages = %v, want %v", messages, want)
	}
	for i, msg := range want {
		if messages[i] != msg {
			t.Errorf("elicitation %d = %q, want %q", i, messages[i], msg)
		}
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
