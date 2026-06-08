// confirm_test.go contains unit tests for user confirmation helpers.
package toolutil

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
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

	result := ConfirmDestructiveAction(context.Background(), nil, nil, testConfirmPrompt)
	if result != nil {
		t.Errorf("expected nil (proceed) in YOLO_MODE, got result")
	}
}

// TestConfirmDestructiveAction_ExplicitConfirm verifies that
// [ConfirmDestructiveAction] returns nil (proceed) when the request
// parameters contain confirm:true.
func TestConfirmDestructiveAction_ExplicitConfirm(t *testing.T) {
	params := map[string]any{"confirm": true}
	result := ConfirmDestructiveAction(context.Background(), nil, params, testConfirmPrompt)
	if result != nil {
		t.Errorf("expected nil (proceed) with confirm:true, got result")
	}
}

// TestConfirmDestructiveAction_NoElicitation verifies that
// [ConfirmDestructiveAction] returns nil (proceed) when the MCP request
// is nil, indicating elicitation is not supported by the client.
func TestConfirmDestructiveAction_NoElicitation(t *testing.T) {
	// No elicitation support (req is nil) and no confirm param → proceeds (nil)
	result := ConfirmDestructiveAction(context.Background(), nil, nil, testConfirmPrompt)
	if result != nil {
		t.Errorf("expected nil (proceed) when elicitation unsupported, got result")
	}
}

// TestConfirmDestructiveAction_RequestWithoutElicitation verifies named tool
// requests still proceed when the client does not support elicitation.
func TestConfirmDestructiveAction_RequestWithoutElicitation(t *testing.T) {
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "delete_project"}}
	result := ConfirmDestructiveAction(context.Background(), req, nil, testConfirmPrompt)
	if result != nil {
		t.Fatalf("ConfirmDestructiveAction() = %+v, want nil without elicitation support", result)
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

// newConfirmSession wires a server and client with the supplied elicitation
// handler so tests can drive the user confirmation path end-to-end.
func newConfirmSession(t *testing.T, handler func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error)) *mcp.ServerSession {
	t.Helper()

	server := mcp.NewServer(confirmTestImpl, nil)
	client := mcp.NewClient(confirmTestImpl, &mcp.ClientOptions{ElicitationHandler: handler})

	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(context.Background(), st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	cs, err := client.Connect(context.Background(), ct, nil)
	if err != nil {
		_ = ss.Close()
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		_ = cs.Close()
		_ = ss.Close()
	})
	return ss
}

// TestConfirmAction_UnsupportedProceeds verifies that [ConfirmAction] returns
// nil (proceed) when the client does not support elicitation.
func TestConfirmAction_UnsupportedProceeds(t *testing.T) {
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "noop"}}
	if got := ConfirmAction(context.Background(), req, "Proceed?"); got != nil {
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
	if got := ConfirmAction(context.Background(), req, "Delete?"); got != nil {
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
	got := ConfirmAction(context.Background(), req, "Delete?")
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
	got := ConfirmAction(context.Background(), req, "Delete?")
	if got == nil {
		t.Fatal("ConfirmAction(declined) = nil, want CancelledResult")
	}
}

// TestConfirmAction_NilReqProceeds verifies that a nil request causes
// [ConfirmAction] to skip elicitation and return nil.
func TestConfirmAction_NilReqProceeds(t *testing.T) {
	if got := ConfirmAction(context.Background(), nil, "Proceed?"); got != nil {
		t.Errorf("ConfirmAction(nil req) = %+v, want nil", got)
	}
}

// TestConfirmAction_UnknownActionProceeds verifies that elicitation errors
// other than ErrDeclined/ErrCancelled are treated as a fallback to proceed
// (returning nil) so the destructive action can still run.
func TestConfirmAction_UnknownActionProceeds(t *testing.T) {
	ss := newConfirmSession(t, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "weird-action"}, nil
	})

	req := &mcp.CallToolRequest{Session: ss, Params: &mcp.CallToolParamsRaw{Name: "delete"}}
	if got := ConfirmAction(context.Background(), req, "Delete?"); got != nil {
		t.Errorf("ConfirmAction(unknown action) = %+v, want nil", got)
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
