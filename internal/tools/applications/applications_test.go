// applications_test.go contains unit tests for the OAuth application MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.

package applications

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const fmtUnexpPath = "unexpected path: %s"

const errExpectedNil = "expected error, got nil"

const fmtUnexpErr = "unexpected error: %v"

const fmtUnexpMethod = "unexpected method: %s"

// TestList verifies the behavior of list.
func TestList(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/applications" {
			t.Fatalf(fmtUnexpPath, r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Fatalf(fmtUnexpMethod, r.Method)
		}
		testutil.RespondJSON(w, http.StatusOK, `[
			{"id": 1, "application_id": "app-1", "application_name": "My App", "secret": "sec", "callback_url": "http://localhost", "confidential": true}
		]`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Applications) != 1 {
		t.Fatalf("len = %d, want 1", len(out.Applications))
	}
	if out.Applications[0].ApplicationName != "My App" {
		t.Errorf("Name = %q, want My App", out.Applications[0].ApplicationName)
	}
	if out.Applications[0].ID != 1 {
		t.Errorf("ID = %d, want 1", out.Applications[0].ID)
	}
}

// TestList_Error verifies the behavior of list error.
func TestList_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := List(t.Context(), client, ListInput{})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestCreate verifies the behavior of create.
func TestCreate(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/applications" {
			t.Fatalf(fmtUnexpPath, r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf(fmtUnexpMethod, r.Method)
		}
		testutil.RespondJSON(w, http.StatusCreated, `{
			"id": 2,
			"application_id": "app-2",
			"application_name": "New App",
			"secret": "newsecret",
			"callback_url": "http://example.com/callback",
			"confidential": false
		}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Create(t.Context(), client, CreateInput{
		Name:        "New App",
		RedirectURI: "http://example.com/callback",
		Scopes:      "api read_user",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 2 {
		t.Errorf("ID = %d, want 2", out.ID)
	}
	if out.ApplicationName != "New App" {
		t.Errorf("Name = %q, want New App", out.ApplicationName)
	}
	if out.Secret != "newsecret" {
		t.Errorf("Secret = %q, want newsecret", out.Secret)
	}
}

// TestCreate_Error verifies the behavior of create error.
func TestCreate_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := Create(t.Context(), client, CreateInput{Name: "x", RedirectURI: "y", Scopes: "z"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestDelete_ValidationError verifies the behavior of delete validation error.
func TestDelete_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called")
	}))
	for _, id := range []int64{0, -1} {
		err := Delete(t.Context(), client, DeleteInput{ID: id})
		if err == nil {
			t.Errorf("ID=%d: expected error, got nil", id)
		}
	}
}

// TestDelete verifies the behavior of delete.
func TestDelete(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/applications/3" {
			t.Fatalf(fmtUnexpPath, r.URL.Path)
		}
		if r.Method != http.MethodDelete {
			t.Fatalf(fmtUnexpMethod, r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, handler)
	err := Delete(t.Context(), client, DeleteInput{ID: 3})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDelete_Error verifies the behavior of delete error.
func TestDelete_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	client := testutil.NewTestClient(t, handler)
	err := Delete(t.Context(), client, DeleteInput{ID: 999})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestFormatListMarkdown verifies the behavior of format list markdown.
func TestFormatListMarkdown(t *testing.T) {
	out := ListOutput{
		Applications: []ApplicationItem{
			{ID: 1, ApplicationName: "App1", ApplicationID: "aid-1", CallbackURL: "http://localhost", Confidential: true},
		},
	}
	md := FormatListMarkdown(out)
	if !strings.Contains(md, "App1") {
		t.Error("missing app name")
	}
	if !strings.Contains(md, "aid-1") {
		t.Error("missing app id")
	}
}

// TestFormatListMarkdown_Empty verifies the behavior of format list markdown empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	out := ListOutput{Applications: nil}
	md := FormatListMarkdown(out)
	if !strings.Contains(md, "No applications found") {
		t.Error("missing empty message")
	}
}

// TestFormatCreateMarkdown verifies the behavior of format create markdown.
func TestFormatCreateMarkdown(t *testing.T) {
	out := CreateOutput{ApplicationItem: ApplicationItem{
		ID: 2, ApplicationName: "New", ApplicationID: "aid-2", Secret: "sec", CallbackURL: "http://cb", Confidential: false,
	}}
	md := FormatCreateMarkdown(out)
	if !strings.Contains(md, "New") {
		t.Error("missing app name")
	}
	if !strings.Contains(md, "sec") {
		t.Error("missing secret")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// List — with pagination
// ---------------------------------------------------------------------------.

// TestList_WithPagination verifies the behavior of list with pagination.
func TestList_WithPagination(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/applications" && r.Method == http.MethodGet {
			if r.URL.Query().Get("page") != "2" {
				t.Errorf("expected page=2, got %s", r.URL.Query().Get("page"))
			}
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id": 5, "application_id": "app-5", "application_name": "Paged", "secret": "s", "callback_url": "http://cb", "confidential": false}
			]`)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{Page: 2, PerPage: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Applications) != 1 {
		t.Fatalf("expected 1 app, got %d", len(out.Applications))
	}
}

// ---------------------------------------------------------------------------
// Create — with confidential flag
// ---------------------------------------------------------------------------.

// TestCreate_WithConfidential verifies the behavior of create with confidential.
func TestCreate_WithConfidential(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/applications" && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id": 10, "application_id": "app-10", "application_name": "Conf App",
				"secret": "csec", "callback_url": "http://cb", "confidential": true
			}`)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)
	conf := true
	out, err := Create(t.Context(), client, CreateInput{
		Name:         "Conf App",
		RedirectURI:  "http://cb",
		Scopes:       "api",
		Confidential: &conf,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Confidential {
		t.Error("expected confidential=true")
	}
}

// ---------------------------------------------------------------------------
// RegisterTools — no panic
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// ---------------------------------------------------------------------------
// MCP round-trip
// ---------------------------------------------------------------------------.

// TestMCPRound_Trip validates m c p round trip across multiple scenarios using table-driven subtests.
func TestMCPRound_Trip(t *testing.T) {
	session := newApplicationsMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_list_applications", map[string]any{}},
		{"create", "gitlab_create_application", map[string]any{
			"name": "Test App", "redirect_uri": "http://cb", "scopes": "api",
		}},
		{"delete", "gitlab_delete_application", map[string]any{"id": float64(1)}},
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
				t.Fatalf("CallTool(%s) returned IsError=true", tt.tool)
			}
		})
	}
}

// TestMCPRound_Trip_ErrorPaths verifies that API errors inside RegisterTools
// handlers are returned as IsError results via MCP. Each subtest calls a tool
// backed by a mock that returns 500, exercising the if err != nil branches.
func TestMCPRound_Trip_ErrorPaths(t *testing.T) {
	session := newErrorMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list_error", "gitlab_list_applications", map[string]any{}},
		{"create_error", "gitlab_create_application", map[string]any{
			"name": "X", "redirect_uri": "http://cb", "scopes": "api",
		}},
		{"delete_error", "gitlab_delete_application", map[string]any{"id": float64(99)}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.tool,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("CallTool(%s) transport error: %v", tt.tool, err)
			}
			if !result.IsError {
				t.Fatalf("CallTool(%s) expected IsError=true", tt.tool)
			}
		})
	}
}

func newErrorMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
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

// newApplicationsMCPSession is an internal helper for the applications package.
func newApplicationsMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/applications", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"application_id":"a1","application_name":"App1","secret":"s","callback_url":"http://cb","confidential":true}]`)
	})
	handler.HandleFunc("POST /api/v4/applications", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"id":2,"application_id":"a2","application_name":"Test App","secret":"s2","callback_url":"http://cb","confidential":false}`)
	})
	handler.HandleFunc("DELETE /api/v4/applications/1", func(w http.ResponseWriter, _ *http.Request) {
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
