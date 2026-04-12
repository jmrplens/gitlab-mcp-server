// broadcastmessages_test.go contains unit tests for the broadcast message MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package broadcastmessages

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const fmtUnexpErr = "unexpected error: %v"

const messageJSON = `{"id":1,"message":"System maintenance tonight","starts_at":"2025-01-01T00:00:00Z","ends_at":"2025-01-02T00:00:00Z","font":"","active":true,"target_access_levels":[],"target_path":"","broadcast_type":"banner","dismissable":true,"theme":"indigo"}`

const (
	pathBroadcastMessages = "/api/v4/broadcast_messages"
	pathBroadcastMessage1 = "/api/v4/broadcast_messages/1"
	testMessageText       = "System maintenance tonight"
	testBannerType        = "banner"
	testMessage           = "Test"
	fmtExpErrMentionID    = "expected error to mention 'id', got: %v"
)

// TestList_Success verifies the behavior of list success.
func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathBroadcastMessages && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `[`+messageJSON+`]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(t.Context(), client, ListInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(out.Messages))
	}
	if out.Messages[0].Message != testMessageText {
		t.Errorf("expected message text, got %q", out.Messages[0].Message)
	}
	if !out.Messages[0].Active {
		t.Error("expected active=true")
	}
}

// TestList_Error verifies the behavior of list error.
func TestList_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := List(t.Context(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestGet_Success verifies the behavior of get success.
func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathBroadcastMessage1 && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, messageJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(t.Context(), client, GetInput{ID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Message.ID != 1 {
		t.Errorf("expected ID 1, got %d", out.Message.ID)
	}
	if out.Message.BroadcastType != testBannerType {
		t.Errorf("expected type 'banner', got %q", out.Message.BroadcastType)
	}
}

// TestCreate_Success verifies the behavior of create success.
func TestCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathBroadcastMessages && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, messageJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(t.Context(), client, CreateInput{
		Message:       testMessageText,
		BroadcastType: testBannerType,
		Theme:         "indigo",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Message.Message != testMessageText {
		t.Errorf("unexpected message: %q", out.Message.Message)
	}
}

// TestCreate_WithTimes verifies the behavior of create with times.
func TestCreate_WithTimes(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathBroadcastMessages && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, messageJSON)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := Create(t.Context(), client, CreateInput{
		Message:  testMessage,
		StartsAt: "2025-01-01T00:00:00Z",
		EndsAt:   "2025-01-02T00:00:00Z",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestCreate_InvalidStartsAt verifies the behavior of create invalid starts at.
func TestCreate_InvalidStartsAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Create(t.Context(), client, CreateInput{
		Message:  testMessage,
		StartsAt: "not-a-date",
	})
	if err == nil {
		t.Fatal("expected error for invalid starts_at")
	}
}

// TestUpdate_Success verifies the behavior of update success.
func TestUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathBroadcastMessage1 && r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, messageJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Update(t.Context(), client, UpdateInput{
		ID:      1,
		Message: "Updated message",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Message.ID != 1 {
		t.Errorf("expected ID 1, got %d", out.Message.ID)
	}
}

// TestDelete_Success verifies the behavior of delete success.
func TestDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathBroadcastMessage1 && r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := Delete(t.Context(), client, DeleteInput{ID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDelete_Error verifies the behavior of delete error.
func TestDelete_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	err := Delete(t.Context(), client, DeleteInput{ID: 1})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestFormatListMarkdown verifies the behavior of format list markdown.
func TestFormatListMarkdown(t *testing.T) {
	out := ListOutput{
		Messages: []MessageItem{
			{ID: 1, Message: testMessage, BroadcastType: testBannerType, Active: true},
		},
	}
	result := FormatListMarkdown(out)
	content := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(content, "Broadcast Messages") {
		t.Error("expected 'Broadcast Messages' header")
	}
	if !strings.Contains(content, testBannerType) {
		t.Error("expected broadcast type in markdown")
	}
}

// TestFormatListMarkdown_Empty verifies the behavior of format list markdown empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	out := ListOutput{Messages: []MessageItem{}}
	result := FormatListMarkdown(out)
	content := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(content, "No broadcast messages") {
		t.Error("expected empty state message")
	}
}

// TestFormatMessageMarkdown verifies the behavior of format message markdown.
func TestFormatMessageMarkdown(t *testing.T) {
	item := MessageItem{
		ID: 1, Message: testMessage, BroadcastType: testBannerType,
		Active: true, Theme: "indigo", StartsAt: "2025-01-01T00:00:00Z",
	}
	result := FormatMessageMarkdown(item)
	content := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(content, "#1") {
		t.Error("expected message ID in header")
	}
	if !strings.Contains(content, "indigo") {
		t.Error("expected theme in markdown")
	}
}

// TestGet_InvalidID verifies the behavior of get invalid i d.
func TestGet_InvalidID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Get(t.Context(), client, GetInput{ID: 0})
	if err == nil {
		t.Fatal("expected error for zero ID")
	}
	if !strings.Contains(err.Error(), "id") {
		t.Errorf(fmtExpErrMentionID, err)
	}
}

// TestUpdate_InvalidID verifies the behavior of update invalid i d.
func TestUpdate_InvalidID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Update(t.Context(), client, UpdateInput{ID: -1, Message: "test"})
	if err == nil {
		t.Fatal("expected error for negative ID")
	}
	if !strings.Contains(err.Error(), "id") {
		t.Errorf(fmtExpErrMentionID, err)
	}
}

// TestDelete_InvalidID verifies the behavior of delete invalid i d.
func TestDelete_InvalidID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := Delete(t.Context(), client, DeleteInput{ID: 0})
	if err == nil {
		t.Fatal("expected error for zero ID")
	}
	if !strings.Contains(err.Error(), "id") {
		t.Errorf(fmtExpErrMentionID, err)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

const errExpectedAPI = "expected API error, got nil"

// ---------------------------------------------------------------------------
// Get — API error
// ---------------------------------------------------------------------------.

// TestGet_APIError verifies the behavior of get a p i error.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := Get(context.Background(), client, GetInput{ID: 999})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// Create — API error, invalid ends_at
// ---------------------------------------------------------------------------.

// TestCreate_APIError verifies the behavior of create a p i error.
func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{Message: "test"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestCreate_InvalidEndsAt verifies the behavior of create invalid ends at.
func TestCreate_InvalidEndsAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(context.Background(), client, CreateInput{
		Message: "Test",
		EndsAt:  "not-a-date",
	})
	if err == nil {
		t.Fatal("expected error for invalid ends_at")
	}
}

// TestCreate_WithAllOptionalFields verifies the behavior of create with all optional fields.
func TestCreate_WithAllOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":2,"message":"Full test","starts_at":"2025-01-01T00:00:00Z","ends_at":"2025-01-02T00:00:00Z","font":"serif","active":true,"target_access_levels":[30],"target_path":"/dashboard","broadcast_type":"notification","dismissable":true,"theme":"blue"}`)
			return
		}
		http.NotFound(w, r)
	}))

	dismiss := true
	out, err := Create(context.Background(), client, CreateInput{
		Message:            "Full test",
		StartsAt:           "2025-01-01T00:00:00Z",
		EndsAt:             "2025-01-02T00:00:00Z",
		Font:               "serif",
		TargetAccessLevels: []int64{30},
		TargetPath:         "/dashboard",
		BroadcastType:      "notification",
		Dismissable:        &dismiss,
		Theme:              "blue",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Message.BroadcastType != "notification" {
		t.Errorf("expected notification, got %s", out.Message.BroadcastType)
	}
}

// ---------------------------------------------------------------------------
// Update — API error, invalid starts_at, invalid ends_at, all optional fields
// ---------------------------------------------------------------------------.

// TestUpdate_APIError verifies the behavior of update a p i error.
func TestUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{ID: 1, Message: "upd"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestUpdate_InvalidStartsAt verifies the behavior of update invalid starts at.
func TestUpdate_InvalidStartsAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Update(context.Background(), client, UpdateInput{
		ID:       1,
		StartsAt: "bad-date",
	})
	if err == nil {
		t.Fatal("expected error for invalid starts_at")
	}
}

// TestUpdate_InvalidEndsAt verifies the behavior of update invalid ends at.
func TestUpdate_InvalidEndsAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Update(context.Background(), client, UpdateInput{
		ID:     1,
		EndsAt: "bad-date",
	})
	if err == nil {
		t.Fatal("expected error for invalid ends_at")
	}
}

// TestUpdate_AllOptionalFields verifies the behavior of update all optional fields.
func TestUpdate_AllOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"message":"Updated all","starts_at":"2025-06-01T00:00:00Z","ends_at":"2025-06-02T00:00:00Z","font":"mono","active":true,"target_access_levels":[40],"target_path":"/admin","broadcast_type":"banner","dismissable":false,"theme":"red"}`)
			return
		}
		http.NotFound(w, r)
	}))

	dismiss := false
	out, err := Update(context.Background(), client, UpdateInput{
		ID:                 1,
		Message:            "Updated all",
		StartsAt:           "2025-06-01T00:00:00Z",
		EndsAt:             "2025-06-02T00:00:00Z",
		Font:               "mono",
		TargetAccessLevels: []int64{40},
		TargetPath:         "/admin",
		BroadcastType:      "banner",
		Dismissable:        &dismiss,
		Theme:              "red",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Message.Theme != "red" {
		t.Errorf("expected red, got %s", out.Message.Theme)
	}
}

// ---------------------------------------------------------------------------
// Formatters — message with TargetPath and EndsAt
// ---------------------------------------------------------------------------.

// TestFormatMessageMarkdown_WithOptionalFields verifies the behavior of format message markdown with optional fields.
func TestFormatMessageMarkdown_WithOptionalFields(t *testing.T) {
	item := MessageItem{
		ID:            2,
		Message:       "Maintenance",
		BroadcastType: "notification",
		Active:        true,
		Dismissable:   true,
		StartsAt:      "2025-01-01T00:00:00Z",
		EndsAt:        "2025-01-02T00:00:00Z",
		Theme:         "blue",
		TargetPath:    "/admin",
	}
	result := FormatMessageMarkdown(item)
	content := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(content, "/admin") {
		t.Errorf("expected target_path in markdown, got: %s", content)
	}
	if !strings.Contains(content, "blue") {
		t.Errorf("expected theme in markdown, got: %s", content)
	}
}

// ---------------------------------------------------------------------------
// RegisterTools — no panic
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// ---------------------------------------------------------------------------
// MCP round-trip for all tools
// ---------------------------------------------------------------------------.

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newBroadcastMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_list_broadcast_messages", map[string]any{}},
		{"get", "gitlab_get_broadcast_message", map[string]any{"id": float64(1)}},
		{"create", "gitlab_create_broadcast_message", map[string]any{"message": "Hello"}},
		{"update", "gitlab_update_broadcast_message", map[string]any{"id": float64(1), "message": "Updated"}},
		{"delete", "gitlab_delete_broadcast_message", map[string]any{"id": float64(1)}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.tool,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.tool, err)
			}
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(%s) returned error: %s", tt.tool, tc.Text)
					}
				}
				t.Fatalf("CallTool(%s) returned IsError=true", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helper: MCP session factory
// ---------------------------------------------------------------------------.

// newBroadcastMCPSession is an internal helper for the broadcastmessages package.
func newBroadcastMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	msgJSON := `{"id":1,"message":"Hello","starts_at":"2025-01-01T00:00:00Z","ends_at":"2025-01-02T00:00:00Z","active":true,"broadcast_type":"banner","dismissable":true,"theme":"indigo"}`

	handler := http.NewServeMux()

	handler.HandleFunc("GET /api/v4/broadcast_messages", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+msgJSON+`]`)
	})

	handler.HandleFunc("GET /api/v4/broadcast_messages/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, msgJSON)
	})

	handler.HandleFunc("POST /api/v4/broadcast_messages", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, msgJSON)
	})

	handler.HandleFunc("PUT /api/v4/broadcast_messages/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, msgJSON)
	})

	handler.HandleFunc("DELETE /api/v4/broadcast_messages/1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}
