// broadcastmessages_test.go contains unit tests for the broadcast message MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package broadcastmessages

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// messageJSON identifies the message JSON constant used by this package.
const messageJSON = `{"id":1,"message":"System maintenance tonight","starts_at":"2026-01-01T00:00:00Z","ends_at":"2026-01-02T00:00:00Z","font":"","active":true,"target_access_levels":[],"target_path":"","broadcast_type":"banner","dismissable":true,"theme":"indigo"}`

const (
	// pathBroadcastMessages identifies the path broadcast messages constant used by this package.
	pathBroadcastMessages = "/api/v4/broadcast_messages"
	// pathBroadcastMessage1 identifies the path broadcast message 1 constant used by this package.
	pathBroadcastMessage1 = "/api/v4/broadcast_messages/1"
	// testMessageText identifies the test message text constant used by this package.
	testMessageText = "System maintenance tonight"
	// testBannerType identifies the test banner type constant used by this package.
	testBannerType = "banner"
	// testMessage identifies the test message constant used by this package.
	testMessage = "Test"
	// fmtExpErrMentionID identifies the fmt exp err mention ID constant used by this package.
	fmtExpErrMentionID = "expected error to mention 'id', got: %v"
)

// TestList_Success verifies List when success.
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

// TestList_Error verifies List when error.
func TestList_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := List(t.Context(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestGet_Success verifies Get when success.
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

// TestCreate_Success verifies Create when success.
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

// TestCreate_WithTimes verifies Create when with times.
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
		StartsAt: "2026-01-01T00:00:00Z",
		EndsAt:   "2026-01-02T00:00:00Z",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestCreate_InvalidStartsAt verifies Create when invalid starts at.
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

// TestUpdate_Success verifies Update when success.
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

// TestDelete_Success verifies Delete when success.
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

// TestDelete_Error verifies Delete when error.
func TestDelete_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	err := Delete(t.Context(), client, DeleteInput{ID: 1})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestFormatListMarkdown verifies FormatListMarkdown.
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

// TestFormatListMarkdown_Empty verifies FormatListMarkdown when empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	out := ListOutput{Messages: []MessageItem{}}
	result := FormatListMarkdown(out)
	content := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(content, "No broadcast messages") {
		t.Error("expected empty state message")
	}
}

// TestFormatMessageMarkdown verifies FormatMessageMarkdown.
func TestFormatMessageMarkdown(t *testing.T) {
	item := MessageItem{
		ID: 1, Message: testMessage, BroadcastType: testBannerType,
		Active: true, Theme: "indigo", StartsAt: "2026-01-01T00:00:00Z",
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

// TestGet_InvalidID verifies Get when invalid ID.
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

// TestUpdate_InvalidID verifies Update when invalid ID.
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

// TestDelete_InvalidID verifies Delete when invalid ID.
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

// errExpectedAPI identifies the err expected API constant used by this package.
const errExpectedAPI = "expected API error, got nil"

// ---------------------------------------------------------------------------
// Get — API error
// ---------------------------------------------------------------------------.

// TestGet_APIError verifies Get when API error.
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

// TestCreate_APIError verifies Create when API error.
func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{Message: "test"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestCreate_InvalidEndsAt verifies Create when invalid ends at.
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

// TestCreate_WithAllOptionalFields verifies Create when with all optional fields.
func TestCreate_WithAllOptionalFields(t *testing.T) {
	var capturedBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			capturedBody = string(body)
			testutil.RespondJSON(w, http.StatusCreated, `{"id":2,"message":"Full test","starts_at":"2026-01-01T00:00:00Z","ends_at":"2026-01-02T00:00:00Z","font":"serif","active":true,"target_access_levels":[30],"target_path":"/dashboard","broadcast_type":"notification","dismissable":true,"theme":"blue"}`)
			return
		}
		http.NotFound(w, r)
	}))

	dismiss := true
	out, err := Create(context.Background(), client, CreateInput{
		Message:            "Full test",
		StartsAt:           "2026-01-01T00:00:00Z",
		EndsAt:             "2026-01-02T00:00:00Z",
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
	for _, want := range []string{"starts_at", "ends_at", "font", "target_access_levels", "target_path", "broadcast_type", "dismissable", "theme"} {
		if !strings.Contains(capturedBody, want) {
			t.Errorf("request body missing field %q", want)
		}
	}
}

// ---------------------------------------------------------------------------
// Update — API error, invalid starts_at, invalid ends_at, all optional fields
// ---------------------------------------------------------------------------.

// TestUpdate_APIError verifies Update when API error.
func TestUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{ID: 1, Message: "upd"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestUpdate_InvalidStartsAt verifies Update when invalid starts at.
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

// TestUpdate_InvalidEndsAt verifies Update when invalid ends at.
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

// TestUpdate_AllOptionalFields verifies Update when all optional fields.
func TestUpdate_AllOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"message":"Updated all","starts_at":"2026-06-01T00:00:00Z","ends_at":"2026-06-02T00:00:00Z","font":"mono","active":true,"target_access_levels":[40],"target_path":"/admin","broadcast_type":"banner","dismissable":false,"theme":"red"}`)
			return
		}
		http.NotFound(w, r)
	}))

	dismiss := false
	out, err := Update(context.Background(), client, UpdateInput{
		ID:                 1,
		Message:            "Updated all",
		StartsAt:           "2026-06-01T00:00:00Z",
		EndsAt:             "2026-06-02T00:00:00Z",
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

// TestFormatMessageMarkdown_WithOptionalFields verifies FormatMessageMarkdown when with optional fields.
func TestFormatMessageMarkdown_WithOptionalFields(t *testing.T) {
	item := MessageItem{
		ID:            2,
		Message:       "Maintenance",
		BroadcastType: "notification",
		Active:        true,
		Dismissable:   true,
		StartsAt:      "2026-01-01T00:00:00Z",
		EndsAt:        "2026-01-02T00:00:00Z",
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

// TestMarkdownRegistry_MessageOutputTypes verifies broadcast message output
// wrappers are registered and render through the message formatter.
func TestMarkdownRegistry_MessageOutputTypes(t *testing.T) {
	message := MessageItem{ID: 7, Message: "Registry check", BroadcastType: testBannerType, Active: true}
	tests := []struct {
		name   string
		output any
	}{
		{name: "get", output: GetOutput{Message: message}},
		{name: "create", output: CreateOutput{Message: message}},
		{name: "update", output: UpdateOutput{Message: message}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toolutil.MarkdownForResult(tt.output)
			if result == nil {
				t.Fatal("expected non-nil markdown result")
			}
			content, ok := result.Content[0].(*mcp.TextContent)
			if !ok {
				t.Fatalf("content type = %T, want TextContent", result.Content[0])
			}
			for _, want := range []string{"Broadcast Message #7", "Registry check", testBannerType} {
				if !strings.Contains(content.Text, want) {
					t.Fatalf("markdown missing %q:\n%s", want, content.Text)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ActionSpecs — metadata
// ---------------------------------------------------------------------------.

// TestActionSpecs_Metadata verifies canonical metadata for broadcast message actions.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	specs := ActionSpecs(client)
	byTool := broadcastMessageSpecsByTool(t, specs)

	if len(specs) != 5 {
		t.Fatalf("len(ActionSpecs) = %d, want 5", len(specs))
	}
	if len(byTool) != len(specs) {
		t.Fatalf("unique individual tools = %d, want %d", len(byTool), len(specs))
	}
	if !byTool["gitlab_delete_broadcast_message"].Route.Destructive {
		t.Fatal("gitlab_delete_broadcast_message should be destructive")
	}
	if byTool["gitlab_list_broadcast_messages"].Usage == "" {
		t.Fatal("gitlab_list_broadcast_messages should define usage")
	}
	if len(byTool["gitlab_get_broadcast_message"].Aliases) == 0 {
		t.Fatal("gitlab_get_broadcast_message should define aliases")
	}
	if byTool["gitlab_update_broadcast_message"].ParameterGuidance["id"].SemanticRole == "" {
		t.Fatal("gitlab_update_broadcast_message should define id parameter guidance")
	}
}

// ---------------------------------------------------------------------------
// ActionSpecs — all routes
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallAllRoutes validates broadcast message routes through canonical specs.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := newBroadcastRouteSpecs(t)

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
			result, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helper: route spec factory
// ---------------------------------------------------------------------------.

// newBroadcastRouteSpecs constructs broadcast route specs test fixtures.
func newBroadcastRouteSpecs(t *testing.T) map[string]toolutil.ActionSpec {
	t.Helper()

	msgJSON := `{"id":1,"message":"Hello","starts_at":"2026-01-01T00:00:00Z","ends_at":"2026-01-02T00:00:00Z","active":true,"broadcast_type":"banner","dismissable":true,"theme":"indigo"}`

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

	return broadcastMessageSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, handler)))
}

// broadcastMessageSpecsByTool supports broadcast message specs by tool assertions in broadcastmessages tests.
func broadcastMessageSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		toolName := spec.IndividualTool.Name
		if toolName == "" {
			t.Fatalf("spec %s missing IndividualTool.Name", spec.Name)
		}
		if _, exists := byTool[toolName]; exists {
			t.Fatalf("duplicate individual tool %q", toolName)
		}
		byTool[toolName] = spec
	}
	return byTool
}
