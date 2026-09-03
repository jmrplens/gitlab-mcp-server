// elicitation_test.go contains unit and integration tests for the elicitation package.
// Unit tests verify nil-safety, unsupported-client error paths, and context cancellation.
// Integration tests use in-memory MCP transports to validate [Client.Confirm],
// [Client.PromptText], [Client.SelectOne], and [Client.GatherData] against a
// real elicitation handler.
package elicitation

import (
	"context"
	"errors"
	"math"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

// testImpl is a shared MCP implementation descriptor used across tests.
var testImpl = &mcp.Implementation{Name: "test", Version: "1.0.0"}

// Test fixtures shared across elicitation tests.
const (
	testConfirmMsg       = "Are you sure?"
	testIssueTitle       = "My Issue Title"
	testSomeText         = "some text"
	testEnterTextMsg     = "Enter text"
	testProjectName      = "test-project"
	testDeleteProjectMsg = "Delete project 'my-project'?"
)

// FromRequest tests.

// TestFromRequest_Nil verifies that [FromRequest] returns an unsupported
// [Client] when given a nil request.
func TestFromRequest_Nil(t *testing.T) {
	c := FromRequest(nil)
	if c.IsSupported() {
		t.Error("FromRequest(nil).IsSupported() = true, want false")
	}
}

// TestFromRequest_NilSession verifies that [FromRequest] returns an unsupported
// [Client] when the request has no associated server session.
func TestFromRequest_NilSession(t *testing.T) {
	c := FromRequest(&mcp.CallToolRequest{})
	if c.IsSupported() {
		t.Error("FromRequest(no session).IsSupported() = true, want false")
	}
}

// TestZeroValue_Client verifies that a zero-value [Client] reports itself
// as unsupported.
func TestZeroValue_Client(t *testing.T) {
	var c Client
	if c.IsSupported() {
		t.Error("zero-value Client.IsSupported() = true, want false")
	}
}

// Unsupported client tests.

// TestConfirm_NotSupported verifies that [Client.Confirm] returns
// [ErrElicitationNotSupported] on an unsupported (zero-value) client.
func TestConfirm_NotSupported(t *testing.T) {
	var c Client
	_, err := c.Confirm(context.Background(), "test?")
	if !errors.Is(err, ErrElicitationNotSupported) {
		t.Errorf("Confirm() error = %v, want %v", err, ErrElicitationNotSupported)
	}
}

// TestPromptText_NotSupported verifies that [Client.PromptText] returns
// [ErrElicitationNotSupported] on an unsupported client.
func TestPromptText_NotSupported(t *testing.T) {
	var c Client
	_, err := c.PromptText(context.Background(), "enter text", "value")
	if !errors.Is(err, ErrElicitationNotSupported) {
		t.Errorf("PromptText() error = %v, want %v", err, ErrElicitationNotSupported)
	}
}

// TestSelectOne_NotSupported verifies that [Client.SelectOne] returns
// [ErrElicitationNotSupported] on an unsupported client.
func TestSelectOne_NotSupported(t *testing.T) {
	var c Client
	_, err := c.SelectOne(context.Background(), "choose", []string{"a", "b"})
	if !errors.Is(err, ErrElicitationNotSupported) {
		t.Errorf("SelectOne() error = %v, want %v", err, ErrElicitationNotSupported)
	}
}

// TestGatherData_NotSupported verifies that [Client.GatherData] returns
// [ErrElicitationNotSupported] on an unsupported client.
func TestGatherData_NotSupported(t *testing.T) {
	var c Client
	_, err := c.GatherData(context.Background(), "fill form", map[string]any{"type": "object"})
	if !errors.Is(err, ErrElicitationNotSupported) {
		t.Errorf("GatherData() error = %v, want %v", err, ErrElicitationNotSupported)
	}
}

// TestSelectOne_EmptyOptions verifies that [Client.SelectOne] validates
// the options slice and returns an error when it is empty.
func TestSelectOne_EmptyOptions(t *testing.T) {
	c := Client{session: &mcp.ServerSession{}} // non-nil but we'll check the validation before it reaches session
	_, err := c.SelectOne(context.Background(), "choose", []string{})
	if err == nil || !strings.Contains(err.Error(), "options list must not be empty") {
		t.Errorf("SelectOne(empty) error = %v, want 'options list must not be empty'", err)
	}
}

// Context cancellation.

// TestElicit_ContextCancelled verifies that the internal elicit method
// returns [context.Canceled] when the context is already canceled.
func TestElicit_ContextCancelled(t *testing.T) {
	ctx := testutil.CancelledCtx(t)

	c := Client{session: &mcp.ServerSession{}}
	_, err := c.elicit(ctx, "test", map[string]any{"type": "object"})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("elicit(canceled) error = %v, want context.Canceled", err)
	}
}

// Integration tests using InMemoryTransports.

// setupElicitSession creates an in-memory MCP server/client pair with the
// given elicitation handler. The client performs the legacy 2025-11-25
// handshake (via testutil.ConnectLegacyElicitationClient) because the
// synchronous [Client] path only exists on sessions negotiated below
// protocol 2026-07-28. Returns the server, its session, and a cleanup
// function (a no-op; teardown is registered via t.Cleanup).
func setupElicitSession(t *testing.T, ctx context.Context, handler func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error)) (*mcp.Server, *mcp.ServerSession, func()) {
	t.Helper()

	server := mcp.NewServer(testImpl, nil)
	ss := testutil.ConnectLegacyElicitationClient(ctx, t, server, func(ctx context.Context, params *mcp.ElicitParams) (*mcp.ElicitResult, error) {
		return handler(ctx, &mcp.ElicitRequest{Params: params})
	}, testutil.LegacyClientOptions{})
	return server, ss, func() {}
}

// TestConfirm_Accept verifies that [Client.Confirm] returns true when the
// elicitation handler accepts with confirmed=true.
func TestConfirm_Accept(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{
			Action:  "accept",
			Content: map[string]any{"confirmed": true},
		}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	confirmed, err := c.Confirm(ctx, testConfirmMsg)
	if err != nil {
		t.Fatalf("Confirm() error = %v", err)
	}
	if !confirmed {
		t.Error("Confirm() = false, want true")
	}
}

// TestConfirm_AcceptFalse verifies that [Client.Confirm] returns false when
// the handler accepts but the confirmed field is false.
func TestConfirm_AcceptFalse(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{
			Action:  "accept",
			Content: map[string]any{"confirmed": false},
		}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	confirmed, err := c.Confirm(ctx, testConfirmMsg)
	if err != nil {
		t.Fatalf("Confirm() error = %v", err)
	}
	if confirmed {
		t.Error("Confirm() = true, want false (user accepted but confirmed=false)")
	}
}

// TestConfirm_Decline verifies that [Client.Confirm] returns [ErrDeclined]
// when the elicitation handler declines.
func TestConfirm_Decline(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "decline"}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	_, err := c.Confirm(ctx, testConfirmMsg)
	if !errors.Is(err, ErrDeclined) {
		t.Errorf("Confirm(decline) error = %v, want %v", err, ErrDeclined)
	}
}

// TestConfirm_Cancel verifies that [Client.Confirm] returns [ErrCancelled]
// when the elicitation handler cancels.
func TestConfirm_Cancel(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "cancel"}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	_, err := c.Confirm(ctx, testConfirmMsg)
	if !errors.Is(err, ErrCancelled) {
		t.Errorf("Confirm(cancel) error = %v, want %v", err, ErrCancelled)
	}
}

// TestPromptText_Accept verifies that [Client.PromptText] returns the text
// value from the accepted elicitation response using a custom field name.
func TestPromptText_Accept(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{
			Action:  "accept",
			Content: map[string]any{"title": testIssueTitle},
		}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	text, err := c.PromptText(ctx, "Enter issue title", "title")
	if err != nil {
		t.Fatalf("PromptText() error = %v", err)
	}
	if text != testIssueTitle {
		t.Errorf("PromptText() = %q, want %q", text, testIssueTitle)
	}
}

// TestPromptText_DefaultFieldName verifies that [Client.PromptText] defaults
// the field name to "value" when an empty string is provided.
func TestPromptText_DefaultFieldName(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{
			Action:  "accept",
			Content: map[string]any{"value": testSomeText},
		}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	text, err := c.PromptText(ctx, testEnterTextMsg, "")
	if err != nil {
		t.Fatalf("PromptText() error = %v", err)
	}
	if text != testSomeText {
		t.Errorf("PromptText() = %q, want %q", text, testSomeText)
	}
}

// TestPromptText_Decline verifies that [Client.PromptText] returns
// [ErrDeclined] when the elicitation handler declines.
func TestPromptText_Decline(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "decline"}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	_, err := c.PromptText(ctx, testEnterTextMsg, "value")
	if !errors.Is(err, ErrDeclined) {
		t.Errorf("PromptText(decline) error = %v, want %v", err, ErrDeclined)
	}
}

// TestSelectOne_Accept verifies that [Client.SelectOne] returns the selected
// option from a valid enum schema.
func TestSelectOne_Accept(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{
			Action:  "accept",
			Content: map[string]any{"selection": "bug"},
		}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	sel, err := c.SelectOne(ctx, "Select label", []string{"bug", "feature", "docs"})
	if err != nil {
		t.Fatalf("SelectOne() error = %v", err)
	}
	if sel != "bug" {
		t.Errorf("SelectOne() = %q, want %q", sel, "bug")
	}
}

// TestSelectOne_InvalidOption verifies that [Client.SelectOne] returns an
// error when the elicitation response contains a value not in the options list.
func TestSelectOne_InvalidOption(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{
			Action:  "accept",
			Content: map[string]any{"selection": "hacked_value"},
		}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	_, err := c.SelectOne(ctx, "Select label", []string{"bug", "feature"})
	// SDK validates enum schema before our defense-in-depth check runs
	if err == nil {
		t.Error("SelectOne(invalid option) should return an error")
	}
}

// TestSelectOne_Cancel verifies that [Client.SelectOne] returns [ErrCancelled]
// when the elicitation handler cancels.
func TestSelectOne_Cancel(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "cancel"}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	_, err := c.SelectOne(ctx, "Select", []string{"a", "b"})
	if !errors.Is(err, ErrCancelled) {
		t.Errorf("SelectOne(cancel) error = %v, want %v", err, ErrCancelled)
	}
}

// TestGatherData_Accept verifies that [Client.GatherData] returns the
// structured data map from an accepted elicitation response.
func TestGatherData_Accept(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{
			Action: "accept",
			Content: map[string]any{
				"name":        testProjectName,
				"description": "A test project",
				"visibility":  "private",
			},
		}, nil
	})
	defer cleanup()

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":        map[string]any{"type": "string"},
			"description": map[string]any{"type": "string"},
			"visibility":  map[string]any{"type": "string", "enum": []any{"private", "internal", "public"}},
		},
		"required": []string{"name"},
	}

	c := clientFromSession(ss)
	data, err := c.GatherData(ctx, "Fill project details", schema)
	if err != nil {
		t.Fatalf("GatherData() error = %v", err)
	}
	if data["name"] != testProjectName {
		t.Errorf("GatherData().name = %v, want %q", data["name"], testProjectName)
	}
	if data["visibility"] != "private" {
		t.Errorf("GatherData().visibility = %v, want %q", data["visibility"], "private")
	}
}

// TestGatherData_Decline verifies that [Client.GatherData] returns
// [ErrDeclined] when the elicitation handler declines.
func TestGatherData_Decline(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "decline"}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	_, err := c.GatherData(ctx, "Fill form", map[string]any{"type": "object"})
	if !errors.Is(err, ErrDeclined) {
		t.Errorf("GatherData(decline) error = %v, want %v", err, ErrDeclined)
	}
}

// TestElicit_UnknownAction verifies that the internal elicit method returns
// an error when the handler responds with an unrecognized action string.
func TestElicit_UnknownAction(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "unknown_action"}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	_, err := c.Confirm(ctx, "test?")
	if err == nil || !strings.Contains(err.Error(), "unknown action") {
		t.Errorf("elicit(unknown action) error = %v, want 'unknown action'", err)
	}
}

// TestPromptText_NonStringResponse verifies that [Client.PromptText] returns
// an error when the elicitation response contains a non-string value.
func TestPromptText_NonStringResponse(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{
			Action:  "accept",
			Content: map[string]any{"value": 42},
		}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	_, err := c.PromptText(ctx, testEnterTextMsg, "value")
	// SDK validates type schema before our type assertion runs
	if err == nil {
		t.Error("PromptText(non-string) should return an error")
	}
}

// TestSelectOne_NonStringResponse verifies that [Client.SelectOne] returns
// an error when the elicitation response contains a non-string selection.
func TestSelectOne_NonStringResponse(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{
			Action:  "accept",
			Content: map[string]any{"selection": 123},
		}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	_, err := c.SelectOne(ctx, "Select", []string{"a"})
	// SDK validates type schema before our type assertion runs
	if err == nil {
		t.Error("SelectOne(non-string) should return an error")
	}
}

// TestConfirm_MessagePassedThrough verifies that [Client.Confirm] passes the
// message string to the elicitation handler without modification.
func TestConfirm_MessagePassedThrough(t *testing.T) {
	ctx := context.Background()
	var receivedMessage string
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		receivedMessage = req.Params.Message
		return &mcp.ElicitResult{
			Action:  "accept",
			Content: map[string]any{"confirmed": true},
		}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	_, _ = c.Confirm(ctx, testDeleteProjectMsg)
	if receivedMessage != testDeleteProjectMsg {
		t.Errorf("message = %q, want %q", receivedMessage, testDeleteProjectMsg)
	}
}

// TestFromRequest_NoElicitationCapability verifies that [FromRequest] returns an
// inactive [Client] when the MCP client does not advertise elicitation capability
// (covers the params.Capabilities.Elicitation == nil branch in FromRequest).
func TestFromRequest_NoElicitationCapability(t *testing.T) {
	ctx := context.Background()

	server := mcp.NewServer(testImpl, nil)
	// Client WITHOUT ElicitationHandler → no elicitation capability advertised
	client := mcp.NewClient(testImpl, nil)

	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() {
		cs.Close()
		ss.Close()
	}()

	// Create a fake CallToolRequest with the real session
	req := &mcp.CallToolRequest{
		Session: ss,
	}
	c := FromRequest(req)
	if c.IsSupported() {
		t.Error("FromRequest should return inactive client when elicitation not supported")
	}
}

// TestFromRequest_WithElicitationCapability verifies that [FromRequest] returns
// an active [Client] when the MCP client advertises elicitation capability.
func TestFromRequest_WithElicitationCapability(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{}}, nil
	})
	defer cleanup()

	req := &mcp.CallToolRequest{
		Session: ss,
	}
	c := FromRequest(req)
	if !c.IsSupported() {
		t.Error("FromRequest should return active client when elicitation is supported")
	}
}

// SelectMulti tests.

// TestSelectMulti_NotSupported verifies that [Client.SelectMulti] returns
// [ErrElicitationNotSupported] on an unsupported (zero-value) client.
func TestSelectMulti_NotSupported(t *testing.T) {
	var c Client
	_, err := c.SelectMulti(context.Background(), "choose", []string{"a"}, 0, 0)
	if !errors.Is(err, ErrElicitationNotSupported) {
		t.Errorf("SelectMulti() error = %v, want %v", err, ErrElicitationNotSupported)
	}
}

// TestSelectMulti_EmptyOptions verifies that [Client.SelectMulti] validates
// the options slice and returns an error when it is empty.
func TestSelectMulti_EmptyOptions(t *testing.T) {
	c := Client{session: &mcp.ServerSession{}}
	_, err := c.SelectMulti(context.Background(), "choose", []string{}, 0, 0)
	if err == nil || !strings.Contains(err.Error(), "options list must not be empty") {
		t.Errorf("SelectMulti(empty) error = %v, want 'options list must not be empty'", err)
	}
}

// TestSelectMulti_Accept verifies that [Client.SelectMulti] returns the
// selected options from a valid multi-select elicitation.
func TestSelectMulti_Accept(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{
			Action:  "accept",
			Content: map[string]any{"selections": []any{"bug", "docs"}},
		}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	sel, err := c.SelectMulti(ctx, "Select labels", []string{"bug", "feature", "docs"}, 1, 3)
	if err != nil {
		t.Fatalf("SelectMulti() error = %v", err)
	}
	if len(sel) != 2 || sel[0] != "bug" || sel[1] != "docs" {
		t.Errorf("SelectMulti() = %v, want [bug docs]", sel)
	}
}

// TestSelectMulti_InvalidOption verifies that [Client.SelectMulti] returns
// an error when a selected value is not in the allowed options.
func TestSelectMulti_InvalidOption(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{
			Action:  "accept",
			Content: map[string]any{"selections": []any{"hacked"}},
		}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	_, err := c.SelectMulti(ctx, "Select", []string{"a", "b"}, 0, 0)
	if err == nil {
		t.Error("SelectMulti(invalid) should return an error")
	}
}

// TestSelectMulti_Decline verifies that [Client.SelectMulti] returns
// [ErrDeclined] when the elicitation handler declines.
func TestSelectMulti_Decline(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "decline"}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	_, err := c.SelectMulti(ctx, "choose", []string{"a"}, 0, 0)
	if !errors.Is(err, ErrDeclined) {
		t.Errorf("SelectMulti(decline) error = %v, want %v", err, ErrDeclined)
	}
}

// SelectOneInt tests.

// TestSelectOneInt_NotSupported verifies that [Client.SelectOneInt] returns
// [ErrElicitationNotSupported] on an unsupported client.
func TestSelectOneInt_NotSupported(t *testing.T) {
	var c Client
	_, err := c.SelectOneInt(context.Background(), "choose", []int{1, 2})
	if !errors.Is(err, ErrElicitationNotSupported) {
		t.Errorf("SelectOneInt() error = %v, want %v", err, ErrElicitationNotSupported)
	}
}

// TestSelectOneInt_EmptyOptions verifies that [Client.SelectOneInt] validates
// the options slice and returns an error when it is empty.
func TestSelectOneInt_EmptyOptions(t *testing.T) {
	c := Client{session: &mcp.ServerSession{}}
	_, err := c.SelectOneInt(context.Background(), "choose", []int{})
	if err == nil || !strings.Contains(err.Error(), "options list must not be empty") {
		t.Errorf("SelectOneInt(empty) error = %v, want 'options list must not be empty'", err)
	}
}

// TestSelectOneInt_Accept verifies that [Client.SelectOneInt] returns the
// selected integer value from a valid enum elicitation.
func TestSelectOneInt_Accept(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{
			Action:  "accept",
			Content: map[string]any{"selection": "30"},
		}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	val, err := c.SelectOneInt(ctx, "Select access level", []int{10, 20, 30, 40})
	if err != nil {
		t.Fatalf("SelectOneInt() error = %v", err)
	}
	if val != 30 {
		t.Errorf("SelectOneInt() = %d, want 30", val)
	}
}

// TestSelectOneInt_InvalidValue verifies that [Client.SelectOneInt] returns
// an error when the selected value is not in the allowed options.
func TestSelectOneInt_InvalidValue(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{
			Action:  "accept",
			Content: map[string]any{"selection": "99"},
		}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	_, err := c.SelectOneInt(ctx, "Select", []int{10, 20})
	if err == nil {
		t.Error("SelectOneInt(invalid) should return an error")
	}
}

// PromptNumber tests.

// TestPromptNumber_NotSupported verifies that [Client.PromptNumber] returns
// [ErrElicitationNotSupported] on an unsupported client.
func TestPromptNumber_NotSupported(t *testing.T) {
	var c Client
	_, err := c.PromptNumber(context.Background(), "enter", "val", 0, 100)
	if !errors.Is(err, ErrElicitationNotSupported) {
		t.Errorf("PromptNumber() error = %v, want %v", err, ErrElicitationNotSupported)
	}
}

// TestPromptNumber_Accept verifies that [Client.PromptNumber] returns the
// numeric value from an accepted elicitation response.
func TestPromptNumber_Accept(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{
			Action:  "accept",
			Content: map[string]any{"amount": float64(42.5)},
		}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	val, err := c.PromptNumber(ctx, "Enter amount", "amount", 0, 100)
	if err != nil {
		t.Fatalf("PromptNumber() error = %v", err)
	}
	if val != 42.5 {
		t.Errorf("PromptNumber() = %f, want 42.5", val)
	}
}

// TestPromptNumber_DefaultFieldName verifies that [Client.PromptNumber]
// defaults the field name to "value" when an empty string is provided.
func TestPromptNumber_DefaultFieldName(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{
			Action:  "accept",
			Content: map[string]any{"value": float64(10)},
		}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	val, err := c.PromptNumber(ctx, "Enter value", "", 0, 100)
	if err != nil {
		t.Fatalf("PromptNumber() error = %v", err)
	}
	if val != 10 {
		t.Errorf("PromptNumber() = %f, want 10", val)
	}
}

// TestPromptNumber_Decline verifies that [Client.PromptNumber] returns
// [ErrDeclined] when the elicitation handler declines.
func TestPromptNumber_Decline(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "decline"}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	_, err := c.PromptNumber(ctx, "Enter", "val", 0, 100)
	if !errors.Is(err, ErrDeclined) {
		t.Errorf("PromptNumber(decline) error = %v, want %v", err, ErrDeclined)
	}
}

// setupElicitURLSession creates an in-memory MCP client/server pair where
// the legacy client advertises both form and URL elicitation capabilities.
func setupElicitURLSession(t *testing.T, ctx context.Context, handler func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error)) (*mcp.Server, *mcp.ServerSession, func()) {
	t.Helper()

	server := mcp.NewServer(testImpl, nil)
	ss := testutil.ConnectLegacyElicitationClient(ctx, t, server, func(ctx context.Context, params *mcp.ElicitParams) (*mcp.ElicitResult, error) {
		return handler(ctx, &mcp.ElicitRequest{Params: params})
	}, testutil.LegacyClientOptions{URLElicitation: true})
	return server, ss, func() {}
}

// URL Mode Tests.

// TestIsURLSupported_WithCapability verifies that IsURLSupported returns true
// when the MCP server advertises URL-mode elicitation capability.
func TestIsURLSupported_WithCapability(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitURLSession(t, ctx, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "accept"}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	if !c.IsURLSupported() {
		t.Error("IsURLSupported() = false, want true")
	}
}

// TestIsURLSupported_WithoutCapability verifies that IsURLSupported returns
// false when the MCP server advertises form-only elicitation.
func TestIsURLSupported_WithoutCapability(t *testing.T) {
	ctx := context.Background()
	// Default setupElicitSession advertises form-only
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "accept"}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	if c.IsURLSupported() {
		t.Error("IsURLSupported() = true, want false (form-only client)")
	}
}

// TestIsURLSupported_ZeroClient verifies that IsURLSupported returns false
// on a zero-value Client with no session.
func TestIsURLSupported_ZeroClient(t *testing.T) {
	c := Client{}
	if c.IsURLSupported() {
		t.Error("IsURLSupported() = true on zero-value client")
	}
}

// TestElicitURL_Accept verifies that ElicitURL succeeds when the MCP client
// accepts the URL elicitation request with mode "url".
func TestElicitURL_Accept(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitURLSession(t, ctx, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		if req.Params.Mode != "url" {
			t.Errorf("mode = %q, want 'url'", req.Params.Mode)
		}
		if req.Params.URL != "https://gitlab.example.com/group/project/-/issues/1" {
			t.Errorf("URL = %q, want issues URL", req.Params.URL)
		}
		return &mcp.ElicitResult{Action: "accept"}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	err := c.ElicitURL(ctx, "https://gitlab.example.com", "https://gitlab.example.com/group/project/-/issues/1", "View issue")
	if err != nil {
		t.Errorf("ElicitURL(accept) error = %v", err)
	}
}

// TestElicitURL_Decline verifies that ElicitURL returns ErrDeclined when
// the MCP client declines the URL elicitation.
func TestElicitURL_Decline(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitURLSession(t, ctx, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "decline"}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	err := c.ElicitURL(ctx, "https://gitlab.example.com", "https://gitlab.example.com/test", "Open page")
	if !errors.Is(err, ErrDeclined) {
		t.Errorf("ElicitURL(decline) error = %v, want %v", err, ErrDeclined)
	}
}

// TestElicitURL_NotSupported verifies that ElicitURL returns
// ErrElicitationNotSupported on a zero-value Client with no session.
func TestElicitURL_NotSupported(t *testing.T) {
	c := Client{}
	err := c.ElicitURL(context.Background(), "https://gitlab.example.com", "https://gitlab.example.com/test", "Open page")
	if !errors.Is(err, ErrElicitationNotSupported) {
		t.Errorf("ElicitURL(no session) error = %v, want %v", err, ErrElicitationNotSupported)
	}
}

// TestElicitURL_URLModeNotSupported verifies that ElicitURL returns
// ErrURLElicitationNotSupported when the MCP server supports only form mode.
func TestElicitURL_URLModeNotSupported(t *testing.T) {
	ctx := context.Background()
	// Form-only client
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "accept"}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	err := c.ElicitURL(ctx, "https://gitlab.example.com", "https://gitlab.example.com/test", "Open page")
	if !errors.Is(err, ErrURLElicitationNotSupported) {
		t.Errorf("ElicitURL(form-only) error = %v, want %v", err, ErrURLElicitationNotSupported)
	}
}

// TestElicitURL_ExternalURLRejected verifies that ElicitURL rejects a target
// URL whose host does not match the GitLab base URL, preventing SSRF.
func TestElicitURL_ExternalURLRejected(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitURLSession(t, ctx, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "accept"}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	err := c.ElicitURL(ctx, "https://gitlab.example.com", "https://evil.com/exploit", "Click here")
	if err == nil {
		t.Error("ElicitURL(external URL) should return error")
	}
	if !strings.Contains(err.Error(), "does not match") {
		t.Errorf("ElicitURL error = %v, want host mismatch error", err)
	}
}

// TestValidateGitLabURL_ValidPaths verifies that validateGitLabURL accepts
// URLs with the same host as the GitLab base URL across various path patterns.
func TestValidateGitLabURL_ValidPaths(t *testing.T) {
	base := "https://gitlab.example.com"
	valid := []string{
		"https://gitlab.example.com/group/project",
		"https://gitlab.example.com/group/project/-/issues/1",
		"https://gitlab.example.com/group/project/-/merge_requests/5",
		"https://gitlab.example.com/-/admin",
		"http://gitlab.example.com/test", // http allowed for self-hosted
	}
	for _, u := range valid {
		t.Run(u, func(t *testing.T) {
			if err := validateGitLabURL(base, u); err != nil {
				t.Errorf("validateGitLabURL(%q) = %v, want nil", u, err)
			}
		})
	}
}

// TestValidateGitLabURL_ExternalHost verifies that validateGitLabURL rejects
// URLs with a different host, non-HTTPS schemes, and javascript: URIs.
func TestValidateGitLabURL_ExternalHost(t *testing.T) {
	base := "https://gitlab.example.com"
	invalid := []string{
		"https://evil.com/test",
		"https://gitlab.evil.com/test",
		"ftp://gitlab.example.com/test",
		"javascript:alert(1)",
	}
	for _, u := range invalid {
		t.Run(u, func(t *testing.T) {
			if err := validateGitLabURL(base, u); err == nil {
				t.Errorf("validateGitLabURL(%q) = nil, want error", u)
			}
		})
	}
}

// TestValidateGitLabURL_InvalidBase verifies that validateGitLabURL returns
// an error when the base URL is malformed.
func TestValidateGitLabURL_InvalidBase(t *testing.T) {
	err := validateGitLabURL("://invalid", "https://test.com")
	if err == nil {
		t.Error("validateGitLabURL(invalid base) = nil, want error")
	}
}

// TestValidateGitLabURL_InvalidTarget verifies that validateGitLabURL returns
// an error when the target URL is malformed.
func TestValidateGitLabURL_InvalidTarget(t *testing.T) {
	err := validateGitLabURL("https://gitlab.example.com", "://invalid")
	if err == nil {
		t.Error("validateGitLabURL(invalid target) = nil, want error")
	}
}

// TestValidateGitLabURL_PortMismatch verifies that [validateGitLabURL] rejects
// a target whose hostname matches but port differs from the base URL.
func TestValidateGitLabURL_PortMismatch(t *testing.T) {
	tests := []struct {
		name   string
		base   string
		target string
	}{
		{"base_has_port_target_does_not", "https://gitlab.example.com:8443", "https://gitlab.example.com/test"},
		{"target_has_port_base_does_not", "https://gitlab.example.com", "https://gitlab.example.com:9999/test"},
		{"different_ports", "https://gitlab.example.com:8443", "https://gitlab.example.com:9999/test"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateGitLabURL(tt.base, tt.target); err == nil {
				t.Errorf("validateGitLabURL(%q, %q) = nil, want port mismatch error", tt.base, tt.target)
			}
		})
	}
}

// TestValidateGitLabURL_PortMatch verifies that [validateGitLabURL] accepts a
// target whose hostname and port both match the base URL.
func TestValidateGitLabURL_PortMatch(t *testing.T) {
	tests := []struct {
		name   string
		base   string
		target string
	}{
		{"both_same_port", "https://gitlab.example.com:8443", "https://gitlab.example.com:8443/group/project"},
		{"both_no_port", "https://gitlab.example.com", "https://gitlab.example.com/group/project"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateGitLabURL(tt.base, tt.target); err != nil {
				t.Errorf("validateGitLabURL(%q, %q) = %v, want nil", tt.base, tt.target, err)
			}
		})
	}
}

// TestSelectOneInt_FloatTruncation verifies that [Client.SelectOneInt] rejects
// a non-integer float64 that would be silently truncated by int() cast.
// The SDK validates the JSON Schema (type: integer), so float values are
// rejected before reaching our code. Either way, error must be returned.
func TestSelectOneInt_FloatTruncation(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{
			Action:  "accept",
			Content: map[string]any{"selection": float64(2.5)},
		}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	_, err := c.SelectOneInt(ctx, "Select", []int{2, 3})
	if err == nil {
		t.Error("SelectOneInt(2.5) should return error for non-integer value")
	}
}

// TestSelectOneInt_NotInOptions verifies that [Client.SelectOneInt] rejects
// a value that is a valid integer but not present in the allowed options.
func TestSelectOneInt_NotInOptions(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{
			Action:  "accept",
			Content: map[string]any{"selection": "99"},
		}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	_, err := c.SelectOneInt(ctx, "Select", []int{1, 2, 3})
	if err == nil {
		t.Error("SelectOneInt(99) should return error for value not in options")
	}
}

// TestPromptNumber_BelowMin verifies that [Client.PromptNumber] rejects values
// below the minimum bound.
func TestPromptNumber_BelowMin(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{
			Action:  "accept",
			Content: map[string]any{"amount": float64(-5)},
		}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	// With min=0, max=100, value -5 should be rejected by schema validation.
	_, err := c.PromptNumber(ctx, "Enter amount", "amount", 0, 100)
	if err == nil {
		t.Error("PromptNumber(-5) should return error for value below minimum")
	}
}

// TestPromptNumber_AboveMax verifies that [Client.PromptNumber] rejects values
// above the maximum bound.
func TestPromptNumber_AboveMax(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{
			Action:  "accept",
			Content: map[string]any{"amount": float64(999)},
		}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	_, err := c.PromptNumber(ctx, "Enter amount", "amount", 0, 100)
	if err == nil {
		t.Error("PromptNumber(999) should return error for value above maximum")
	}
}

// ---------------------------------------------------------------------------
// ElicitURL — additional action and error paths
// ---------------------------------------------------------------------------.

// TestElicitURL_Cancel verifies that [Client.ElicitURL] returns [ErrCancelled]
// when the user cancels the URL-mode elicitation dialog.
func TestElicitURL_Cancel(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitURLSession(t, ctx, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "cancel"}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	err := c.ElicitURL(ctx, "https://gitlab.example.com", "https://gitlab.example.com/test", "Open page")
	if !errors.Is(err, ErrCancelled) {
		t.Errorf("ElicitURL(cancel) error = %v, want %v", err, ErrCancelled)
	}
}

// TestElicitURL_UnknownAction verifies that [Client.ElicitURL] returns an
// error when the server responds with an unrecognized action string.
func TestElicitURL_UnknownAction(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitURLSession(t, ctx, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "bogus"}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	err := c.ElicitURL(ctx, "https://gitlab.example.com", "https://gitlab.example.com/test", "Open page")
	if err == nil {
		t.Error("ElicitURL(unknown action) should return error")
	}
	if !strings.Contains(err.Error(), "unknown action") {
		t.Errorf("ElicitURL error = %v, want 'unknown action' message", err)
	}
}

// TestElicitURL_ContextCancelled verifies that [Client.ElicitURL] returns a
// context error when the context is already cancelled before the request.
func TestElicitURL_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	_, ss, cleanup := setupElicitURLSession(t, ctx, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "accept"}, nil
	})
	defer cleanup()
	cancel()

	c := clientFromSession(ss)
	err := c.ElicitURL(ctx, "https://gitlab.example.com", "https://gitlab.example.com/test", "Open page")
	if err == nil {
		t.Error("ElicitURL(cancelled) should return error")
	}
}

// ---------------------------------------------------------------------------
// SelectOneInt — additional edge cases
// ---------------------------------------------------------------------------.

// TestSelectOneInt_NonNumericResponse verifies that [Client.SelectOneInt]
// returns an error when the response field is not a number. The MCP SDK
// validates the integer schema before our code, so the error comes from
// schema validation rather than our type assertion.
func TestSelectOneInt_NonNumericResponse(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{
			Action:  "accept",
			Content: map[string]any{"selection": "not-a-number"},
		}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	_, err := c.SelectOneInt(ctx, "Select", []int{1, 2})
	if err == nil {
		t.Error("SelectOneInt(string) should return error")
	}
}

// ---------------------------------------------------------------------------
// PromptNumber — additional edge cases
// ---------------------------------------------------------------------------.

// TestPromptNumber_NonNumericResponse verifies that [Client.PromptNumber]
// returns an error when the response field is not a number. The MCP SDK
// validates the number schema before our code runs.
func TestPromptNumber_NonNumericResponse(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{
			Action:  "accept",
			Content: map[string]any{"value": "text"},
		}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	_, err := c.PromptNumber(ctx, "Enter number", "", 0, 100)
	if err == nil {
		t.Error("PromptNumber(string) should return error")
	}
}

// ---------------------------------------------------------------------------
// SelectMulti — additional edge cases
// ---------------------------------------------------------------------------.

// TestSelectMulti_NonArrayResponse verifies that [Client.SelectMulti] returns
// an error when the response field is not an array.
func TestSelectMulti_NonArrayResponse(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{
			Action:  "accept",
			Content: map[string]any{"selections": "not-an-array"},
		}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	_, err := c.SelectMulti(ctx, "Select", []string{"a", "b"}, 0, 0)
	if err == nil {
		t.Error("SelectMulti(string) should return error")
	}
}

// TestSelectMulti_NonStringElement verifies that [Client.SelectMulti] returns
// an error when an array element is not a string.
func TestSelectMulti_NonStringElement(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{
			Action:  "accept",
			Content: map[string]any{"selections": []any{42}},
		}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	_, err := c.SelectMulti(ctx, "Select", []string{"a", "b"}, 0, 0)
	if err == nil {
		t.Error("SelectMulti(int element) should return error")
	}
}

// ConfirmAction tests.

// TestConfirmAction_NilRequest verifies that [ConfirmAction] returns nil
// when the request is nil (elicitation not supported — backward compatible).
func TestConfirmAction_NilRequest(t *testing.T) {
	result := ConfirmAction(context.Background(), nil, "Delete?")
	if result != nil {
		t.Error("expected nil result for nil request (not supported)")
	}
}

// TestConfirmAction_Confirmed verifies that [ConfirmAction] returns nil
// when the user confirms the action.
func TestConfirmAction_Confirmed(t *testing.T) {
	ctx := context.Background()
	server, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{
			Action:  "accept",
			Content: map[string]any{"confirmed": true},
		}, nil
	})
	defer cleanup()
	_ = server

	req := &mcp.CallToolRequest{}
	req.Session = ss
	result := ConfirmAction(ctx, req, "Delete project?")
	if result != nil {
		t.Error("expected nil result when user confirms")
	}
}

// TestConfirmAction_MalformedRequestState_FailsClosed verifies that
// [ConfirmAction] refuses the action when the echoed request state cannot be
// decoded, rather than treating the undecodable state as "no answer yet".
//
// This is the fail-closed edge of a destructive-action guard. Reading the
// state is how the second round learns the user already answered; if that
// read fails and the failure were ignored, the guard would either re-prompt
// forever or, worse, fall through to a path that never asked at all.
func TestConfirmAction_MalformedRequestState_FailsClosed(t *testing.T) {
	handler := func(ctx context.Context, req *mcp.CallToolRequest, _ *Flow) (*mcp.CallToolResult, error) {
		req.Params.RequestState = "{not json"
		if res := ConfirmAction(ctx, req, "Delete project?"); res != nil {
			return res, nil
		}
		return textResult("confirmation was skipped"), nil
	}
	cs := flowTestTool(t, handler, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		t.Error("a request whose state cannot be decoded must not reach the user")
		return nil, errors.New("unexpected elicitation")
	})

	result, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "flow_tool", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if !result.IsError {
		t.Error("IsError = false, want true: a failed confirmation must not read as success")
	}
	if got := resultText(result); !strings.Contains(got, "Confirmation failed") {
		t.Errorf("result text = %q, want a confirmation failure", got)
	}
}

// TestConfirmAction_MultiRoundTrip_QueuesThenConfirms verifies that on a
// protocol 2026-07-28 session [ConfirmAction] answers the first round with an
// input-required result and completes on the retry.
//
// Sending a server-initiated elicitation request here is forbidden by the
// specification, so the guard has to suspend the call instead of blocking on
// an answer. Only driving both rounds shows that it resumes.
func TestConfirmAction_MultiRoundTrip_QueuesThenConfirms(t *testing.T) {
	var rounds atomic.Int32
	handler := func(ctx context.Context, req *mcp.CallToolRequest, _ *Flow) (*mcp.CallToolResult, error) {
		rounds.Add(1)
		if res := ConfirmAction(ctx, req, "Delete project?"); res != nil {
			return res, nil
		}
		return textResult("confirmed"), nil
	}
	cs := flowTestTool(t, handler, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirmed": true}}, nil
	})

	result, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "flow_tool", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if got := resultText(result); got != "confirmed" {
		t.Errorf("result text = %q, want %q", got, "confirmed")
	}
	// Two rounds is the mechanism itself: one that queued the request and one
	// that read the answer back. A single round would mean the guard answered
	// without ever asking.
	if got := rounds.Load(); got != 2 {
		t.Errorf("handler ran %d times, want 2 (queue then resume)", got)
	}
}

// TestConfirmAction_Declined verifies that [ConfirmAction] returns a
// non-nil cancellation result when the user declines.
func TestConfirmAction_Declined(t *testing.T) {
	ctx := context.Background()
	server, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "decline"}, nil
	})
	defer cleanup()
	_ = server

	req := &mcp.CallToolRequest{}
	req.Session = ss
	result := ConfirmAction(ctx, req, "Delete?")
	if result == nil {
		t.Fatal("expected non-nil result when user declines")
	}
}

// TestConfirmAction_NotConfirmed verifies that [ConfirmAction] returns
// a cancellation result when the user accepts but does not confirm.
func TestConfirmAction_NotConfirmed(t *testing.T) {
	ctx := context.Background()
	server, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{
			Action:  "accept",
			Content: map[string]any{"confirmed": false},
		}, nil
	})
	defer cleanup()
	_ = server

	req := &mcp.CallToolRequest{}
	req.Session = ss
	result := ConfirmAction(ctx, req, "Delete?")
	if result == nil {
		t.Fatal("expected non-nil result when user does not confirm")
	}
}

// TestConfirmAction_OtherError verifies that [ConfirmAction] fails closed
// when [Client.Confirm] returns an error that is neither [ErrDeclined] nor
// [ErrCancelled]: the guard returns an error result so the destructive
// action does not proceed on a failed confirmation exchange.
func TestConfirmAction_OtherError(t *testing.T) {
	ctx := context.Background()
	server, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return nil, errors.New("transport blew up")
	})
	defer cleanup()
	_ = server

	req := &mcp.CallToolRequest{}
	req.Session = ss
	result := ConfirmAction(ctx, req, "Delete?")
	if result == nil {
		t.Fatal("expected fail-closed error result on confirmation failure, got nil (action would proceed)")
	}
	if !result.IsError {
		t.Errorf("ConfirmAction(failure).IsError = false, want true")
	}
}

// CancelledResult tests.

// TestCancelledResult verifies that [CancelledResult] returns a
// CallToolResult with the given message as TextContent.
func TestCancelledResult(t *testing.T) {
	msg := "Operation canceled by user."
	result := CancelledResult(msg)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content element, got %d", len(result.Content))
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}
	if tc.Text != msg {
		t.Errorf("Text = %q, want %q", tc.Text, msg)
	}
}

// TestSelectOneInt_NaN verifies that [Client.SelectOneInt] returns an error
// when the elicitation response contains a non-numeric value.
func TestSelectOneInt_NaN(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{
			Action:  "accept",
			Content: map[string]any{"selection": "not_a_number"},
		}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	_, err := c.SelectOneInt(ctx, "Pick", []int{1, 2, 3})
	if err == nil {
		t.Error("expected error for non-numeric selection")
	}
}

// TestSelectOneInt_Inf verifies that SelectOneInt rejects math.Inf values.
func TestSelectOneInt_Inf(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{
			Action:  "accept",
			Content: map[string]any{"selection": math.Inf(1)},
		}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	_, err := c.SelectOneInt(ctx, "Pick", []int{1, 2, 3})
	if err == nil {
		t.Error("expected error for Inf selection")
	}
}

// TestSelectOne_FloatInsteadOfString verifies that SelectOne returns an error
// when the response field is a float instead of a string.
func TestSelectOne_FloatInsteadOfString(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{
			Action:  "accept",
			Content: map[string]any{"selection": 42.0},
		}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	_, err := c.SelectOne(ctx, "Pick", []string{"a", "b"})
	if err == nil {
		t.Error("expected error for non-string selection")
	}
}

// TestSelectMulti_StringInsteadOfArray verifies that SelectMulti returns an
// error when the response field is a string instead of an array.
func TestSelectMulti_StringInsteadOfArray(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{
			Action:  "accept",
			Content: map[string]any{"selections": "not-an-array"},
		}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	_, err := c.SelectMulti(ctx, "Pick", []string{"a", "b"}, 0, 0)
	if err == nil {
		t.Error("expected error for non-array selections")
	}
}

// TestConfirmAction_DeclineAction verifies that ConfirmAction returns a
// non-nil result when the elicitation is declined.
// Note: ConfirmAction uses FromRequest internally which requires a full
// MCP server session with elicitation capability. Testing this function
// at the unit level is not practical without heavy infrastructure.
// Coverage for the Confirm/Decline paths is provided by TestConfirm_* tests.

// TestPromptText_NonStringField verifies that PromptText returns an error
// when the response field is not a string.
func TestPromptText_NonStringField(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{
			Action:  "accept",
			Content: map[string]any{"value": 123.0},
		}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	_, err := c.PromptText(ctx, "Enter text", "")
	if err == nil {
		t.Error("expected error for non-string response")
	}
}

// TestPromptNumber_NaN verifies that [Client.PromptNumber] returns an error
// when the response is not a number.
func TestPromptNumber_NaN(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{
			Action:  "accept",
			Content: map[string]any{"value": "text"},
		}, nil
	})
	defer cleanup()

	c := clientFromSession(ss)
	_, err := c.PromptNumber(ctx, "Enter number", "", 0, 100)
	if err == nil {
		t.Error("expected error for non-numeric value")
	}
}

// TestIsURLSupported_NilParams verifies that [Client.IsURLSupported] returns
// false when the session's InitializeParams are nil.
func TestIsURLSupported_NilParams(t *testing.T) {
	c := Client{session: &mcp.ServerSession{}}
	if c.IsURLSupported() {
		t.Error("expected false for nil InitializeParams")
	}
}

// TestElicitURL_HandlerError verifies that [Client.ElicitURL] propagates
// errors from the underlying session Elicit call as a wrapped "URL request
// failed" error.
func TestElicitURL_HandlerError(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitURLSession(t, ctx, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return nil, errors.New("transport blew up")
	})
	defer cleanup()

	c := clientFromSession(ss)
	err := c.ElicitURL(ctx, "https://gitlab.example.com", "https://gitlab.example.com/test", "Open")
	if err == nil {
		t.Fatal("expected error when handler returns error")
	}
	if !strings.Contains(err.Error(), "URL request failed") {
		t.Errorf("err = %v, want substring %q", err, "URL request failed")
	}
}

// clientFromSession builds a Client the way the tests used to get one, by
// taking the capabilities from the session's initialize handshake.
//
// Production code no longer reads them there: FromRequest takes the
// capabilities of the request, because from 2026-07-28 a client declares them
// per request and a request can arrive on a session that never handshook at
// all. The tests still negotiate a real session through setupElicitSession or
// setupElicitURLSession, so deriving from it keeps each case describing the
// client it set up (form-only or url-capable) rather than one written out again
// at the call site.
func clientFromSession(ss *mcp.ServerSession) Client {
	var caps *mcp.ClientCapabilities
	if p := ss.InitializeParams(); p != nil {
		caps = p.Capabilities
	}
	return Client{session: ss, caps: caps}
}

// TestFromRequest_NilCapabilities_DoesNotPanic pins the crash that took the
// process down.
//
// InitializeParams.Capabilities is a pointer, and the SDK synthesizes
// InitializeParams containing only a protocol version for a request that
// arrives without a handshake, leaving that pointer nil. FromRequest read
// params.Capabilities.Elicitation with only a params == nil guard, so the
// dereference panicked in the SDK's jsonrpc2 goroutine, where nothing recovers:
// the whole server died.
//
// It needed no client misbehavior on HTTP. Under the default stateless
// transport each POST is its own session, so an ordinary pre-2026-07-28 client
// took the synthesized path on every single tools/call that could elicit.
//
// The nil session and nil request cases are asserted alongside it so the guard
// cannot be narrowed later to the one shape that was reported.
func TestFromRequest_NilCapabilities_DoesNotPanic(t *testing.T) {
	tests := []struct {
		name string
		req  *mcp.CallToolRequest
	}{
		{
			name: "a request whose params were synthesized without capabilities",
			req:  &mcp.CallToolRequest{Session: &mcp.ServerSession{}},
		},
		{
			name: "a request with no session",
			req:  &mcp.CallToolRequest{},
		},
		{
			name: "no request at all",
			req:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := FromRequest(tt.req)
			if c.IsSupported() {
				t.Error("IsSupported() = true for a client that declared no elicitation capability")
			}
			// Both mode predicates are reached on the same nil state.
			if c.IsURLSupported() {
				t.Error("IsURLSupported() = true for a client that declared nothing")
			}
			if c.IsFormSupported() {
				t.Error("IsFormSupported() = true for a client that declared nothing")
			}
		})
	}
}

// TestIsFormSupported_FollowsTheDeclaredModes pins the predicate the
// specification requires before a form request is queued.
//
// "Servers MUST NOT send elicitation requests with modes that are not supported
// by the client." A client declaring url and not form does not support form.
// One declaring neither is form-capable, because an empty capabilities object
// predates the modes and the page keeps it meaning form for compatibility;
// that is also the predicate the SDK enforces on send, and disagreeing with it
// would mean queueing requests the SDK then refuses.
func TestIsFormSupported_FollowsTheDeclaredModes(t *testing.T) {
	tests := []struct {
		name string
		caps *mcp.ClientCapabilities
		want bool
	}{
		{
			name: "form declared",
			caps: &mcp.ClientCapabilities{Elicitation: &mcp.ElicitationCapabilities{Form: &mcp.FormElicitationCapabilities{}}},
			want: true,
		},
		{
			name: "neither mode declared is form, for backwards compatibility",
			caps: &mcp.ClientCapabilities{Elicitation: &mcp.ElicitationCapabilities{}},
			want: true,
		},
		{
			name: "both modes declared",
			caps: urlCapabilities(),
			want: true,
		},
		{
			name: "url only is not form",
			caps: &mcp.ClientCapabilities{Elicitation: &mcp.ElicitationCapabilities{URL: &mcp.URLElicitationCapabilities{}}},
			want: false,
		},
		{
			name: "no elicitation capability at all",
			caps: &mcp.ClientCapabilities{},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Client{session: &mcp.ServerSession{}, caps: tt.caps}
			if got := c.IsFormSupported(); got != tt.want {
				t.Errorf("IsFormSupported() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestFromRequest_ReadsTheRequestsOwnCapabilities pins where the capabilities
// are read from.
//
// "Clients that support elicitation MUST declare the `elicitation` capability
// in `_meta.io.modelcontextprotocol/clientCapabilities` on each request."
//
// FromRequest used to read the session's InitializeParams instead, which is
// wrong in both directions on a stdio session: the first request's declaration
// became permanent, so a client that omitted capabilities on its first call
// lost elicitation for the rest of the session, and one that declared them
// there kept them even on a later call that declared none. It is also what
// crashed the server, since the SDK synthesizes InitializeParams with a nil
// Capabilities pointer for a request that never handshook.
//
// Both directions are asserted here against a session that has no handshake at
// all, which is the shape a stateless HTTP POST always has.
func TestFromRequest_ReadsTheRequestsOwnCapabilities(t *testing.T) {
	tests := []struct {
		name          string
		meta          mcp.Meta
		wantSupported bool
	}{
		{
			name:          "declared on this request",
			meta:          mcp.Meta{mcp.MetaKeyClientCapabilities: map[string]any{"elicitation": map[string]any{"form": map[string]any{}}}},
			wantSupported: true,
		},
		{
			name:          "declared empty on this request is still elicitation-capable",
			meta:          mcp.Meta{mcp.MetaKeyClientCapabilities: map[string]any{"elicitation": map[string]any{}}},
			wantSupported: true,
		},
		{
			name:          "this request declares capabilities without elicitation",
			meta:          mcp.Meta{mcp.MetaKeyClientCapabilities: map[string]any{}},
			wantSupported: false,
		},
		{
			name:          "this request declares nothing",
			meta:          nil,
			wantSupported: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &mcp.CallToolRequest{
				Session: &mcp.ServerSession{},
				Params:  &mcp.CallToolParamsRaw{Name: "whatever", Meta: tt.meta},
			}
			if got := FromRequest(req).IsSupported(); got != tt.wantSupported {
				t.Errorf("IsSupported() = %v, want %v", got, tt.wantSupported)
			}
		})
	}
}

// TestConfirm_AcceptWithoutTheAnswer_IsNotADecision covers an accept whose
// content does not carry the boolean the confirmation asked for.
//
// It is deliberately not read as "no". A declined dialog is a decision the user
// made, and a model is told to stop and offer something else; a malformed
// answer is a client that did not answer the question at all, and reporting it
// as a refusal would attribute to the user a choice nobody made. Failing closed
// here is what keeps the destructive action from running either way.
func TestConfirm_AcceptWithoutTheAnswer_IsNotADecision(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		content map[string]any
	}{
		// A wrong type is not among them: the SDK validates the answer against
		// the schema that was sent and refuses it before this code sees it.
		// What reaches here is an answer that satisfies the schema and still
		// does not carry the field, which the schema does not make required.
		{name: "the field is missing", content: map[string]any{}},
		{name: "no content at all", content: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ss, cleanup := setupElicitSession(t, ctx, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				return &mcp.ElicitResult{Action: "accept", Content: tt.content}, nil
			})
			defer cleanup()

			confirmed, err := clientFromSession(ss).Confirm(ctx, testConfirmMsg)

			if !errors.Is(err, ErrMalformedAnswer) {
				t.Errorf("Confirm() error = %v, want ErrMalformedAnswer", err)
			}
			if confirmed {
				t.Error("Confirm() = true for an answer that never said so")
			}
		})
	}
}
