// systemhooks_test.go contains unit tests for the system hook MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package systemhooks

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const fmtUnexpPath = "unexpected path: %s"

const fmtUnexpErr = "unexpected error: %v"

const testHookURL = "https://example.com/hook"

const errExpectedErrZeroID = "expected error for zero ID, got nil"

const errAPINotCalledZeroID = "API should not be called when ID is 0"

const hookJSON = `{"id":1,"url":"https://example.com/hook","created_at":"2026-01-01T00:00:00Z","push_events":true,"tag_push_events":false,"merge_requests_events":true,"repository_update_events":false,"enable_ssl_verification":true}`

// TestList_Success verifies the behavior of list success.
func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/hooks" {
			t.Errorf(fmtUnexpPath, r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK, `[`+hookJSON+`]`)
	}))

	out, err := List(t.Context(), client, ListInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Hooks) != 1 {
		t.Fatalf("expected 1 hook, got %d", len(out.Hooks))
	}
	if out.Hooks[0].URL != testHookURL {
		t.Errorf("expected %s, got %s", testHookURL, out.Hooks[0].URL)
	}
}

// TestList_Error verifies the behavior of list error.
func TestList_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	_, err := List(t.Context(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestGet_Success verifies the behavior of get success.
func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/hooks/1" {
			t.Errorf(fmtUnexpPath, r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK, hookJSON)
	}))

	out, err := Get(t.Context(), client, GetInput{ID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Hook.ID != 1 {
		t.Errorf("expected ID 1, got %d", out.Hook.ID)
	}
	if !out.Hook.PushEvents {
		t.Error("expected push_events true")
	}
}

// TestAdd_Success verifies the behavior of add success.
func TestAdd_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		testutil.RespondJSON(w, http.StatusCreated, hookJSON)
	}))

	tr := true
	out, err := Add(t.Context(), client, AddInput{URL: testHookURL, PushEvents: &tr})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Hook.URL != testHookURL {
		t.Errorf("expected %s, got %s", testHookURL, out.Hook.URL)
	}
}

// TestTest_Success verifies the behavior of test success.
func TestTest_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/hooks/1" {
			t.Errorf(fmtUnexpPath, r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK, `{"event_name":"project_create","name":"test-proj","path":"test-proj","project_id":42,"owner_name":"admin","owner_email":"admin@example.com"}`)
	}))

	out, err := Test(t.Context(), client, TestInput{ID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Event.EventName != "project_create" {
		t.Errorf("expected project_create, got %s", out.Event.EventName)
	}
	if out.Event.ProjectID != 42 {
		t.Errorf("expected project_id 42, got %d", out.Event.ProjectID)
	}
}

// TestDelete_Success verifies the behavior of delete success.
func TestDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	err := Delete(t.Context(), client, DeleteInput{ID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDelete_Error verifies the behavior of delete error.
func TestDelete_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	err := Delete(t.Context(), client, DeleteInput{ID: 999})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestGet_ZeroID verifies the behavior of get zero i d.
func TestGet_ZeroID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errAPINotCalledZeroID)
	}))
	_, err := Get(t.Context(), client, GetInput{ID: 0})
	if err == nil {
		t.Fatal(errExpectedErrZeroID)
	}
}

// TestTest_ZeroID verifies the behavior of test zero i d.
func TestTest_ZeroID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errAPINotCalledZeroID)
	}))
	_, err := Test(t.Context(), client, TestInput{ID: 0})
	if err == nil {
		t.Fatal(errExpectedErrZeroID)
	}
}

// TestDelete_ZeroID verifies the behavior of delete zero i d.
func TestDelete_ZeroID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errAPINotCalledZeroID)
	}))
	err := Delete(t.Context(), client, DeleteInput{ID: 0})
	if err == nil {
		t.Fatal(errExpectedErrZeroID)
	}
}

// TestFormatListMarkdown verifies the behavior of format list markdown.
func TestFormatListMarkdown(t *testing.T) {
	result := FormatListMarkdown(ListOutput{
		Hooks: []HookItem{
			{ID: 1, URL: testHookURL, PushEvents: true, EnableSSLVerification: true},
		},
	})
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "example.com") {
		t.Errorf("expected URL in output, got: %s", text)
	}
}

// TestFormatHookMarkdown verifies the behavior of format hook markdown.
func TestFormatHookMarkdown(t *testing.T) {
	result := FormatHookMarkdown(HookItem{ID: 1, URL: testHookURL, PushEvents: true})
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "System Hook #1") {
		t.Errorf("expected hook header, got: %s", text)
	}
}

// TestFormatTestMarkdown verifies the behavior of format test markdown.
func TestFormatTestMarkdown(t *testing.T) {
	result := FormatTestMarkdown(TestOutput{Event: HookEventItem{EventName: "project_create", Name: "test", ProjectID: 42}})
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "project_create") {
		t.Errorf("expected event name, got: %s", text)
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
// Add — API error, with all optional fields
// ---------------------------------------------------------------------------.

// TestAdd_APIError verifies the behavior of add a p i error.
func TestAdd_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := Add(context.Background(), client, AddInput{URL: "https://bad.example.com"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestAdd_AllOptionalFields verifies the behavior of add all optional fields.
func TestAdd_AllOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":2,"url":"https://example.com/hook2","created_at":"2026-01-01T00:00:00Z","push_events":false,"tag_push_events":true,"merge_requests_events":true,"repository_update_events":true,"enable_ssl_verification":false}`)
			return
		}
		http.NotFound(w, r)
	}))

	f, tr := false, true
	out, err := Add(context.Background(), client, AddInput{
		URL:                    "https://example.com/hook2",
		Token:                  "secret-token",
		PushEvents:             &f,
		TagPushEvents:          &tr,
		MergeRequestsEvents:    &tr,
		RepositoryUpdateEvents: &tr,
		EnableSSLVerification:  &f,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Hook.ID != 2 {
		t.Errorf("expected ID 2, got %d", out.Hook.ID)
	}
	if out.Hook.PushEvents {
		t.Error("expected push_events false")
	}
}

// ---------------------------------------------------------------------------
// Test — API error
// ---------------------------------------------------------------------------.

// TestTest_APIError verifies the behavior of test a p i error.
func TestTest_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := Test(context.Background(), client, TestInput{ID: 999})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// Formatters — empty list
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_Empty verifies the behavior of format list markdown empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	result := FormatListMarkdown(ListOutput{})
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "No system hooks found") {
		t.Errorf("expected empty message, got: %s", text)
	}
}

// ---------------------------------------------------------------------------
// Formatters — hook with created_at
// ---------------------------------------------------------------------------.

// TestFormatHookMarkdown_WithCreatedAt verifies the behavior of format hook markdown with created at.
func TestFormatHookMarkdown_WithCreatedAt(t *testing.T) {
	result := FormatHookMarkdown(HookItem{
		ID:        1,
		URL:       "https://example.com/hook",
		CreatedAt: "2026-01-01T00:00:00Z",
	})
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "1 Jan 2026 00:00 UTC") {
		t.Errorf("expected created_at in output, got: %s", text)
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
	session := newSystemHooksMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_list_system_hooks", map[string]any{}},
		{"get", "gitlab_get_system_hook", map[string]any{"id": float64(1)}},
		{"add", "gitlab_add_system_hook", map[string]any{"url": "https://example.com/hook"}},
		{"test", "gitlab_test_system_hook", map[string]any{"id": float64(1)}},
		{"delete", "gitlab_delete_system_hook", map[string]any{"id": float64(1)}},
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

// newSystemHooksMCPSession is an internal helper for the systemhooks package.
func newSystemHooksMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	hookJSON := `{"id":1,"url":"https://example.com/hook","created_at":"2026-01-01T00:00:00Z","push_events":true,"tag_push_events":false,"merge_requests_events":true,"repository_update_events":false,"enable_ssl_verification":true}`

	handler := http.NewServeMux()

	handler.HandleFunc("GET /api/v4/hooks", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+hookJSON+`]`)
	})

	handler.HandleFunc("GET /api/v4/hooks/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, hookJSON)
	})

	handler.HandleFunc("POST /api/v4/hooks", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, hookJSON)
	})

	// TestHook also uses GET /api/v4/hooks/1 — the GitLab API tests hooks via GET with special handling
	// Actually, the SDK uses /api/v4/hooks/:id — but the test endpoint shares the same path.
	// We need a separate handler for the test endpoint (it's actually a GET to a specific path).

	handler.HandleFunc("DELETE /api/v4/hooks/1", func(w http.ResponseWriter, _ *http.Request) {
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
