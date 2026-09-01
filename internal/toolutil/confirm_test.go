// confirm_test.go contains unit tests for user confirmation helpers.
package toolutil

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

const testConfirmPrompt = "Delete project?"

// TestIsTruthy verifies that isTruthy correctly parses boolean-like string
// values ("true", "1", "yes" and variants) as true, and everything else
// as false, using table-driven subtests.
func TestIsTruthy(t *testing.T) {
	tests := []struct {
		name string
		val  string
		want bool
	}{
		{"empty", "", false},
		{"1", "1", true},
		{"true", "true", true},
		{"TRUE", "TRUE", true},
		{"True", "True", true},
		{"yes", "yes", true},
		{"YES", "YES", true},
		{"Yes", "Yes", true},
		{"0", "0", false},
		{"false", "false", false},
		{"no", "no", false},
		{"random", "random", false},
		{"true with spaces", "  true  ", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTruthy(tt.val); got != tt.want {
				t.Errorf("isTruthy(%q) = %v, want %v", tt.val, got, tt.want)
			}
		})
	}
}

// TestIsYOLOMode verifies that [IsYOLOMode] returns true when either
// YOLO_MODE or AUTOPILOT environment variables are set to a truthy value.
func TestIsYOLOMode(t *testing.T) {
	tests := []struct {
		name      string
		yolo      string
		autopilot string
		want      bool
	}{
		{"neither set", "", "", false},
		{"YOLO_MODE=true", "true", "", true},
		{"YOLO_MODE=1", "1", "", true},
		{"AUTOPILOT=true", "", "true", true},
		{"AUTOPILOT=yes", "", "yes", true},
		{"both set", "true", "true", true},
		{"YOLO_MODE=false", "false", "", false},
		{"AUTOPILOT=0", "", "0", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("YOLO_MODE", tt.yolo)
			t.Setenv("AUTOPILOT", tt.autopilot)

			if got := IsYOLOMode(); got != tt.want {
				t.Errorf("IsYOLOMode() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestHasExplicitConfirm verifies that hasExplicitConfirm detects truthy
// values in the "confirm" key of the tool parameters map.
func TestHasExplicitConfirm(t *testing.T) {
	tests := []struct {
		name   string
		params map[string]any
		want   bool
	}{
		{"nil params", nil, false},
		{"empty params", map[string]any{}, false},
		{"no confirm key", map[string]any{"action": "delete"}, false},
		{"confirm true bool", map[string]any{"confirm": true}, true},
		{"confirm false bool", map[string]any{"confirm": false}, false},
		{"confirm true string", map[string]any{"confirm": "true"}, true},
		{"confirm yes string", map[string]any{"confirm": "yes"}, true},
		{"confirm 1 string", map[string]any{"confirm": "1"}, true},
		{"confirm false string", map[string]any{"confirm": "false"}, false},
		{"confirm number", map[string]any{"confirm": 1}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasExplicitConfirm(tt.params); got != tt.want {
				t.Errorf("hasExplicitConfirm(%v) = %v, want %v", tt.params, got, tt.want)
			}
		})
	}
}

// TestConfirmDestructiveAction_YOLOMode verifies that [ConfirmDestructiveAction]
// returns nil (proceed) when YOLO_MODE is enabled.
func TestConfirmDestructiveAction_YOLOMode(t *testing.T) {
	t.Setenv("YOLO_MODE", "true")

	result, guardErr := ConfirmDestructiveAction(context.Background(), nil, nil, testConfirmPrompt)
	if guardErr != nil {
		t.Fatalf("unexpected protocol error: %v", guardErr)
	}
	if result != nil {
		t.Errorf("expected nil (proceed) in YOLO_MODE, got result")
	}
}

// TestConfirmDestructiveAction_ExplicitConfirm verifies that
// [ConfirmDestructiveAction] returns nil (proceed) when the request
// parameters contain confirm:true.
func TestConfirmDestructiveAction_ExplicitConfirm(t *testing.T) {
	params := map[string]any{"confirm": true}
	result, guardErr := ConfirmDestructiveAction(context.Background(), nil, params, testConfirmPrompt)
	if guardErr != nil {
		t.Fatalf("unexpected protocol error: %v", guardErr)
	}
	if result != nil {
		t.Errorf("expected nil (proceed) with confirm:true, got result")
	}
}

// TestConfirmDestructiveAction_NoElicitation verifies that
// [ConfirmDestructiveAction] fails closed when the MCP request is nil
// (elicitation unsupported): the action must not proceed, and the error
// result must tell the caller how to confirm explicitly.
func TestConfirmDestructiveAction_NoElicitation(t *testing.T) {
	result, guardErr := ConfirmDestructiveAction(context.Background(), nil, nil, testConfirmPrompt)
	if guardErr != nil {
		t.Fatalf("unexpected protocol error: %v", guardErr)
	}
	if result == nil {
		t.Fatal("expected error result when elicitation unsupported, got nil (proceed)")
	}
	if !result.IsError {
		t.Errorf("result.IsError = false, want true")
	}
	assertConfirmGuardText(t, result)
}

// TestConfirmDestructiveAction_RequestWithoutElicitation verifies named tool
// requests fail closed when the client does not support elicitation.
func TestConfirmDestructiveAction_RequestWithoutElicitation(t *testing.T) {
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "delete_project"}}
	result, guardErr := ConfirmDestructiveAction(context.Background(), req, nil, testConfirmPrompt)
	if guardErr != nil {
		t.Fatalf("unexpected protocol error: %v", guardErr)
	}
	if result == nil {
		t.Fatal("ConfirmDestructiveAction() = nil, want fail-closed error result without elicitation support")
	}
	if !result.IsError {
		t.Errorf("result.IsError = false, want true")
	}
	assertConfirmGuardText(t, result)
}

// confirmGuardServer builds an in-memory server with two destructive-guarded
// tools: "guarded" runs the normal ConfirmDestructiveAction flow, and
// "corrupt" corrupts the multi round-trip request state first so the guard's
// invalid-state branch is exercised through a real tools/call.
func confirmGuardServer(t *testing.T) *mcp.ClientSession {
	t.Helper()
	server := mcp.NewServer(confirmTestImpl, nil)
	schema := map[string]any{"type": "object", "properties": map[string]any{}}
	executed := func() *mcp.CallToolResult {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "executed"}}}
	}
	server.AddTool(&mcp.Tool{Name: "guarded", Description: "guarded", InputSchema: schema},
		func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			result, err := ConfirmDestructiveAction(ctx, req, nil, testConfirmPrompt)
			if err != nil {
				return nil, err
			}
			if result != nil {
				return result, nil
			}
			return executed(), nil
		})
	server.AddTool(&mcp.Tool{Name: "corrupt", Description: "corrupt", InputSchema: schema},
		func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			req.Params.RequestState = "{not json"
			result, err := ConfirmDestructiveAction(ctx, req, nil, testConfirmPrompt)
			if err != nil {
				return nil, err
			}
			if result != nil {
				return result, nil
			}
			return executed(), nil
		})
	server.AddTool(&mcp.Tool{Name: "corrupt_confirm", Description: "corrupt confirm", InputSchema: schema},
		func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			req.Params.RequestState = "{not json"
			result, err := ConfirmAction(ctx, req, testConfirmPrompt)
			if err != nil {
				return nil, err
			}
			if result != nil {
				return result, nil
			}
			return executed(), nil
		})

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(t.Context(), serverTransport, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "confirm-mrtr-client", Version: "1.0.0"}, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirmed": true}}, nil
		},
	})
	session, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// TestConfirmDestructiveAction_MRTRRoundTrip verifies the multi round-trip
// confirmation on a modern session: the first pass returns input-required,
// the SDK client answers through its elicitation handler, and the retried
// call executes the destructive handler.
func TestConfirmDestructiveAction_MRTRRoundTrip(t *testing.T) {
	session := confirmGuardServer(t)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "guarded", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok || tc.Text != "executed" {
		t.Fatalf("result = %+v, want confirmed execution", result.Content[0])
	}
}

// TestInvalidRequestState_FailsClosedAsAProtocolError covers both entry points
// into the confirmation guard with a corrupted client-echoed request state.
//
// The failing-closed half must never change: a state this server did not issue
// stops the destructive action. The shape did change. It used to be an isError
// tool result, which told a model that a field its own client mangled was
// something it could correct; it is invalid params, and the assertion follows
// the wire — a client sees an error response, not a successful call carrying a
// complaint.
//
// Both paths are here because ConfirmAction has its own defensive check,
// reached when a caller invokes it without the ConfirmDestructiveAction
// pre-check, and the two must classify the same input the same way.
func TestInvalidRequestState_FailsClosedAsAProtocolError(t *testing.T) {
	tests := []struct {
		name string
		tool string
		// namesTheField is true where the message should still identify what
		// the client got wrong.
		namesTheField bool
	}{
		{name: "through the destructive guard", tool: "corrupt", namesTheField: true},
		{name: "through ConfirmAction directly", tool: "corrupt_confirm"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := confirmGuardServer(t)
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
				Name:      tt.tool,
				Arguments: map[string]any{},
			})
			if err == nil {
				t.Fatalf("result = %+v, want a protocol error for an invalid request state", result)
			}
			if tt.namesTheField && !strings.Contains(err.Error(), "requestState") {
				t.Errorf("err = %v, want it to name the offending field", err)
			}

			var rpcErr *jsonrpc.Error
			if !errors.As(err, &rpcErr) {
				t.Fatalf("the error carries no JSON-RPC code: %v", err)
			}
			if rpcErr.Code != jsonrpc.CodeInvalidParams {
				t.Errorf("code = %d, want %d (invalid params)", rpcErr.Code, jsonrpc.CodeInvalidParams)
			}
		})
	}
}

// assertConfirmGuardText checks the fail-closed guard result carries the
// original prompt plus the confirm=true instruction.
func assertConfirmGuardText(t *testing.T, result *mcp.CallToolResult) {
	t.Helper()
	if len(result.Content) != 1 {
		t.Fatalf("guard content length = %d, want 1", len(result.Content))
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("guard content is %T, want *mcp.TextContent", result.Content[0])
	}
	if !strings.Contains(tc.Text, testConfirmPrompt) || !strings.Contains(tc.Text, "confirm=true") {
		t.Errorf("guard text = %q, want prompt and confirm=true instruction", tc.Text)
	}
}

// TestCancelledResult verifies that [CancelledResult] returns a non-nil
// CallToolResult with a single TextContent entry matching the given message.
func TestCancelledResult(t *testing.T) {
	msg := "Operation canceled by user."
	result := CancelledResult(msg)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content entry, got %d", len(result.Content))
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}
	if tc.Text != msg {
		t.Errorf("expected %q, got %q", msg, tc.Text)
	}
}

// confirmTestImpl is a shared MCP implementation descriptor for in-memory
// confirm sessions.
var confirmTestImpl = &mcp.Implementation{Name: "confirm-test", Version: "1.0.0"}

// newConfirmSession wires a server and a legacy (protocol 2025-11-25) fake
// client with the supplied elicitation handler so direct guard invocations
// deterministically exercise the synchronous confirmation path. The multi
// round-trip confirmation path is covered by the surface/catalog tests that
// drive real tools/call round-trips.
func newConfirmSession(t *testing.T, handler func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error)) *mcp.ServerSession {
	t.Helper()

	server := mcp.NewServer(confirmTestImpl, nil)
	return testutil.ConnectLegacyElicitationClient(t.Context(), t, server, func(ctx context.Context, params *mcp.ElicitParams) (*mcp.ElicitResult, error) {
		return handler(ctx, &mcp.ElicitRequest{Params: params})
	}, testutil.LegacyClientOptions{})
}

// TestConfirmAction_UnsupportedProceeds verifies that [ConfirmAction] returns
// nil (proceed) when the client does not support elicitation.
func TestConfirmAction_UnsupportedProceeds(t *testing.T) {
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "noop"}}
	if got, guardErr := ConfirmAction(context.Background(), req, "Proceed?"); guardErr != nil || got != nil {
		t.Errorf("ConfirmAction(unsupported) = %+v, want nil", got)
	}
}

// TestConfirmAction_ConfirmedProceeds verifies that an accepted elicitation
// (confirmed=true) lets the destructive action proceed with a nil result.
func TestConfirmAction_ConfirmedProceeds(t *testing.T) {
	ss := newConfirmSession(t, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirmed": true}}, nil
	})

	req := &mcp.CallToolRequest{Session: ss, Params: &mcp.CallToolParamsRaw{Name: "delete"}}
	if got, guardErr := ConfirmAction(context.Background(), req, "Delete?"); guardErr != nil || got != nil {
		t.Errorf("ConfirmAction(confirmed) = %+v, want nil", got)
	}
}

// TestConfirmAction_ConfirmedFalseCancels verifies that an elicitation
// response with confirmed=false returns a CancelledResult.
func TestConfirmAction_ConfirmedFalseCancels(t *testing.T) {
	ss := newConfirmSession(t, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirmed": false}}, nil
	})

	req := &mcp.CallToolRequest{Session: ss, Params: &mcp.CallToolParamsRaw{Name: "delete"}}
	got, guardErr := ConfirmAction(context.Background(), req, "Delete?")
	if guardErr != nil {
		t.Fatalf("unexpected protocol error: %v", guardErr)
	}
	if got == nil {
		t.Fatal("ConfirmAction(denied) = nil, want CancelledResult")
	}
	if !strings.Contains(surfaceToolText(got), "canceled") {
		t.Errorf("canceled text = %q, want it to contain 'canceled'", surfaceToolText(got))
	}
}

// TestConfirmAction_DeclinedCancels verifies that a user-declined elicitation
// returns a CancelledResult and surfaces the user-canceled message.
func TestConfirmAction_DeclinedCancels(t *testing.T) {
	ss := newConfirmSession(t, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "decline"}, nil
	})

	req := &mcp.CallToolRequest{Session: ss, Params: &mcp.CallToolParamsRaw{Name: "delete"}}
	got, guardErr := ConfirmAction(context.Background(), req, "Delete?")
	if guardErr != nil {
		t.Fatalf("unexpected protocol error: %v", guardErr)
	}
	if got == nil {
		t.Fatal("ConfirmAction(declined) = nil, want CancelledResult")
	}
}

// TestConfirmAction_NilReqProceeds verifies that a nil request causes
// [ConfirmAction] to skip elicitation and return nil.
func TestConfirmAction_NilReqProceeds(t *testing.T) {
	if got, guardErr := ConfirmAction(context.Background(), nil, "Proceed?"); guardErr != nil || got != nil {
		t.Errorf("ConfirmAction(nil req) = %+v, want nil", got)
	}
}

// TestConfirmAction_UnknownActionFailsClosed verifies that a malformed
// elicitation answer (an unrecognized action value) fails closed with an
// error result instead of letting the destructive action proceed.
func TestConfirmAction_UnknownActionFailsClosed(t *testing.T) {
	ss := newConfirmSession(t, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "weird-action"}, nil
	})

	req := &mcp.CallToolRequest{Session: ss, Params: &mcp.CallToolParamsRaw{Name: "delete"}}
	got, guardErr := ConfirmAction(context.Background(), req, "Delete?")
	if guardErr != nil {
		t.Fatalf("unexpected protocol error: %v", guardErr)
	}
	if got == nil {
		t.Fatal("ConfirmAction(unknown action) = nil, want fail-closed error result")
	}
	if !got.IsError {
		t.Error("ConfirmAction(unknown action).IsError = false, want true")
	}
	if !strings.Contains(surfaceToolText(got), "Confirmation failed") {
		t.Errorf("fail-closed text = %q, want it to mention 'Confirmation failed'", surfaceToolText(got))
	}
}

// TestExplicitConfirmFromRequest verifies that ExplicitConfirmFromRequest
// detects the reserved confirm key on both call shapes it must support
// (flat on individual tools, nested under params on dispatcher surfaces),
// and fails closed (false) for a nil/malformed request instead of panicking
// or misreporting confirmation. This matters because clearguard.go gates a
// destructive workitems action on this return value: a false positive here
// would let a destructive call bypass confirmation, and a JSON-decode panic
// would crash the handler on any malformed client payload.
func TestExplicitConfirmFromRequest(t *testing.T) {
	tests := []struct {
		name string
		req  *mcp.CallToolRequest
		want bool
	}{
		{"nil request", nil, false},
		{"nil params", &mcp.CallToolRequest{Params: nil}, false},
		{"empty arguments", &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Arguments: nil}}, false},
		{
			name: "malformed JSON arguments",
			req: &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{
				Arguments: []byte("{not valid json"),
			}},
			want: false,
		},
		{
			name: "flat confirm true",
			req: &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{
				Arguments: []byte(`{"confirm":true}`),
			}},
			want: true,
		},
		{
			name: "flat confirm false",
			req: &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{
				Arguments: []byte(`{"confirm":false}`),
			}},
			want: false,
		},
		{
			name: "nested params confirm true",
			req: &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{
				Arguments: []byte(`{"action":"delete","params":{"confirm":true}}`),
			}},
			want: true,
		},
		{
			name: "no confirm anywhere",
			req: &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{
				Arguments: []byte(`{"action":"delete","params":{}}`),
			}},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExplicitConfirmFromRequest(tt.req); got != tt.want {
				t.Errorf("ExplicitConfirmFromRequest() = %v, want %v", got, tt.want)
			}
		})
	}
}

// surfaceToolText concatenates text content of a CallToolResult for assertions.
func surfaceToolText(result *mcp.CallToolResult) string {
	if result == nil {
		return ""
	}
	var b strings.Builder
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

// TestConfirmAction_MalformedAnswerIsNotAUserDecision pins what a destructive
// confirmation reports when the client's answer cannot be read.
//
// "The server SHOULD treat [an invalid response] as if the input request had
// not been fulfilled." A client that accepts the confirmation but sends content
// failing the schema it was handed used to be reported as "Operation canceled
// by user." The action was correctly stopped; the explanation was invented.
// That reaches a model as a decision a person made, so the model does not
// retry, and it tells the user they declined something they were never shown.
//
// The guard is called directly rather than through a client session, because
// the SDK client cannot produce this input: it validates elicitation content
// against the requested schema and refuses to send a mismatch, and it fulfills
// input requests itself rather than handing the state back to the caller. With
// a go-sdk client the case is unreachable. It is reachable from any client that
// does not validate, which is where it was observed, so the test speaks that
// client's part directly.
//
// Both properties are asserted: the action still does not proceed, and the
// reason is attributed to the user only when a user actually decided.
func TestConfirmAction_MalformedAnswerIsNotAUserDecision(t *testing.T) {
	tests := []struct {
		name              string
		content           map[string]any
		wantsUserDecision bool
	}{
		{
			name:              "an explicit refusal is a user decision",
			content:           map[string]any{"confirmed": false},
			wantsUserDecision: true,
		},
		{
			name:              "an answer to a different question is not",
			content:           map[string]any{"title": "some unrelated field"},
			wantsUserDecision: false,
		},
		{
			name:              "a non-boolean confirmation is not",
			content:           map[string]any{"confirmed": "yes"},
			wantsUserDecision: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &mcp.CallToolRequest{
				Session: &mcp.ServerSession{},
				Params: &mcp.CallToolParamsRaw{
					Name: "guarded",
					Meta: mcp.Meta{
						"io.modelcontextprotocol/protocolVersion": "2026-07-28",
						"io.modelcontextprotocol/clientCapabilities": map[string]any{
							"elicitation": map[string]any{"form": map[string]any{}},
						},
					},
					InputResponses: mcp.InputResponseMap{
						"confirm": &mcp.ElicitResult{Action: "accept", Content: tt.content},
					},
				},
			}

			result, guardErr := ConfirmDestructiveAction(context.Background(), req, nil, testConfirmPrompt)
			if guardErr != nil {
				t.Fatalf("unexpected protocol error: %v", guardErr)
			}
			if result == nil {
				t.Fatal("the guard let the destructive action proceed")
			}

			var text string
			if len(result.Content) > 0 {
				if tc, isText := result.Content[0].(*mcp.TextContent); isText {
					text = tc.Text
				}
			}
			// A decision attributed to a person, however it is worded. The
			// failure this guards against is a client-side schema bug being
			// reported as something the user did.
			mentionsUser := strings.Contains(text, "user declined") ||
				strings.Contains(text, "canceled by user") ||
				strings.Contains(text, "cancelled by user")
			if mentionsUser != tt.wantsUserDecision {
				t.Errorf("result = %q; attributing this to the user = %v, want %v",
					text, mentionsUser, tt.wantsUserDecision)
			}
			if !result.IsError {
				t.Error("the result is not marked as an error, so a declared output schema goes unsatisfied")
			}
		})
	}
}

// TestConfirmDestructiveAction_ProtocolFaultsAreJSONRPCErrors pins which
// failures belong in a tool result and which do not.
//
// "Protocol errors (malformed JSON, invalid schema, internal server errors)
// SHOULD return a JSON-RPC error response with an appropriate error code and
// message."
//
// Three faults used to come back as successful JSON-RPC responses carrying an
// isError tool result: a requestState this server did not issue, one carrying a
// version it does not know, and an inputResponses value of the wrong type. None
// of the three is something the model wrote, so putting them in tool output
// invites it to fix a field its own client mangled — and a client that cannot
// see them as protocol errors has no reason to stop resending them.
//
// The distinction is the point, and it cuts the other way just as firmly: a
// user declining, a client that cannot prompt, a GitLab refusal are all tool
// outcomes and stay in the result, which is where MCP asks for them. Both sides
// are asserted here so a later change cannot quietly move the line.
func TestConfirmDestructiveAction_ProtocolFaultsAreJSONRPCErrors(t *testing.T) {
	mrtrMeta := mcp.Meta{
		"io.modelcontextprotocol/protocolVersion": "2026-07-28",
		"io.modelcontextprotocol/clientCapabilities": map[string]any{
			"elicitation": map[string]any{"form": map[string]any{}},
		},
	}

	tests := []struct {
		name         string
		params       *mcp.CallToolParamsRaw
		wantRPCError bool
	}{
		{
			name: "a requestState this server did not issue",
			params: &mcp.CallToolParamsRaw{
				Name: "guarded", Meta: mrtrMeta,
				RequestState: "not-json-at-all",
			},
			wantRPCError: true,
		},
		{
			name: "a requestState from a version this build does not know",
			params: &mcp.CallToolParamsRaw{
				Name: "guarded", Meta: mrtrMeta,
				RequestState: "v99.eyJ2Ijo5OX0.bWFj",
			},
			wantRPCError: true,
		},
		{
			name: "an inputResponse of the wrong type",
			params: &mcp.CallToolParamsRaw{
				Name: "guarded", Meta: mrtrMeta,
				InputResponses: mcp.InputResponseMap{
					// Every InputResponse that is not an ElicitResult is
					// deprecated (sampling and roots, SEP-2577), so a wrong
					// type can only be spelled with one. That is the case
					// under test: whatever arrives, it is not what was asked
					// for.
					"confirm": &mcp.ListRootsResult{}, //nolint:staticcheck // see above
				},
			},
			wantRPCError: true,
		},
		{
			name: "a user declining is a tool outcome, not a protocol fault",
			params: &mcp.CallToolParamsRaw{
				Name: "guarded", Meta: mrtrMeta,
				InputResponses: mcp.InputResponseMap{
					"confirm": &mcp.ElicitResult{Action: "decline"},
				},
			},
			wantRPCError: false,
		},
		{
			name: "a client that cannot elicit is a tool outcome",
			params: &mcp.CallToolParamsRaw{
				Name: "guarded",
				Meta: mcp.Meta{
					"io.modelcontextprotocol/protocolVersion":    "2026-07-28",
					"io.modelcontextprotocol/clientCapabilities": map[string]any{},
				},
			},
			wantRPCError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &mcp.CallToolRequest{Session: &mcp.ServerSession{}, Params: tt.params}

			result, err := ConfirmDestructiveAction(context.Background(), req, nil, testConfirmPrompt)

			if !tt.wantRPCError {
				if err != nil {
					t.Fatalf("a tool outcome was reported as a protocol error: %v", err)
				}
				if result == nil {
					t.Fatal("the destructive action was allowed to proceed")
				}
				return
			}

			if err == nil {
				t.Fatalf("a protocol fault was reported as a tool result: %+v", result)
			}
			if result != nil {
				t.Errorf("a protocol error also produced a tool result: %+v", result)
			}

			var rpcErr *jsonrpc.Error
			if !errors.As(err, &rpcErr) {
				t.Fatalf("the error carries no JSON-RPC code: %v", err)
			}
			if rpcErr.Code != jsonrpc.CodeInvalidParams {
				t.Errorf("code = %d, want %d (invalid params)", rpcErr.Code, jsonrpc.CodeInvalidParams)
			}
		})
	}
}
