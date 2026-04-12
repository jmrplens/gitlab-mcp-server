// errortracking_test.go contains unit tests for the error tracking MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package errortracking

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const errExpectedErr = "expected error"

const fmtUnexpErr = "unexpected error: %v"

// TestGetSettings verifies the behavior of get settings.
func TestGetSettings(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/error_tracking/settings" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{"active":true,"project_name":"test","sentry_external_url":"https://sentry.io","api_url":"https://sentry.io/api","integrated":false}`)
	}))
	out, err := GetSettings(t.Context(), client, GetSettingsInput{ProjectID: "1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !out.Active {
		t.Error("expected active=true")
	}
	if out.ProjectName != "test" {
		t.Errorf("expected project_name=test, got %s", out.ProjectName)
	}
	if out.Integrated {
		t.Error("expected integrated=false")
	}
}

// TestGetSettings_Error verifies the behavior of get settings error.
func TestGetSettings_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))
	_, err := GetSettings(t.Context(), client, GetSettingsInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestEnableDisable verifies the behavior of enable disable.
func TestEnableDisable(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/error_tracking/settings" || r.Method != http.MethodPatch {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{"active":true,"project_name":"test","integrated":true}`)
	}))
	active := true
	integrated := true
	out, err := EnableDisable(t.Context(), client, EnableDisableInput{ProjectID: "1", Active: &active, Integrated: &integrated})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !out.Active || !out.Integrated {
		t.Error("expected active=true, integrated=true")
	}
}

// TestEnableDisable_Error verifies the behavior of enable disable error.
func TestEnableDisable_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"error"}`)
	}))
	_, err := EnableDisable(t.Context(), client, EnableDisableInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestListClientKeys verifies the behavior of list client keys.
func TestListClientKeys(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/error_tracking/client_keys" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"active":true,"public_key":"pk1","sentry_dsn":"dsn1"},{"id":2,"active":false,"public_key":"pk2","sentry_dsn":"dsn2"}]`)
	}))
	out, err := ListClientKeys(t.Context(), client, ListClientKeysInput{ProjectID: "1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(out.Keys))
	}
	if out.Keys[0].PublicKey != "pk1" {
		t.Errorf("expected pk1, got %s", out.Keys[0].PublicKey)
	}
}

// TestListClientKeys_Error verifies the behavior of list client keys error.
func TestListClientKeys_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"error"}`)
	}))
	_, err := ListClientKeys(t.Context(), client, ListClientKeysInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestCreateClientKey verifies the behavior of create client key.
func TestCreateClientKey(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/error_tracking/client_keys" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusCreated, `{"id":10,"active":true,"public_key":"newpk","sentry_dsn":"newdsn"}`)
	}))
	out, err := CreateClientKey(t.Context(), client, CreateClientKeyInput{ProjectID: "1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 10 || out.PublicKey != "newpk" {
		t.Errorf("unexpected key: %+v", out)
	}
}

// TestCreateClientKey_Error verifies the behavior of create client key error.
func TestCreateClientKey_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"error"}`)
	}))
	_, err := CreateClientKey(t.Context(), client, CreateClientKeyInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestDeleteClientKey verifies the behavior of delete client key.
func TestDeleteClientKey(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/error_tracking/client_keys/10" || r.Method != http.MethodDelete {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	err := DeleteClientKey(t.Context(), client, DeleteClientKeyInput{ProjectID: "1", KeyID: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDeleteClientKey_Error verifies the behavior of delete client key error.
func TestDeleteClientKey_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"error"}`)
	}))
	err := DeleteClientKey(t.Context(), client, DeleteClientKeyInput{ProjectID: "1", KeyID: 10})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestDeleteClientKey_InvalidKeyID verifies the behavior of delete client key invalid key i d.
func TestDeleteClientKey_InvalidKeyID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API")
	}))
	err := DeleteClientKey(t.Context(), client, DeleteClientKeyInput{ProjectID: "1", KeyID: 0})
	if err == nil {
		t.Fatal("expected error for zero key_id")
	}
	if !strings.Contains(err.Error(), "key_id") {
		t.Errorf("expected error to mention key_id, got %q", err)
	}
	err = DeleteClientKey(t.Context(), client, DeleteClientKeyInput{ProjectID: "1", KeyID: -1})
	if err == nil {
		t.Fatal("expected error for negative key_id")
	}
}

// TestFormatSettingsMarkdown verifies the behavior of format settings markdown.
func TestFormatSettingsMarkdown(t *testing.T) {
	out := SettingsOutput{Active: true, ProjectName: "test", SentryExternalURL: "https://sentry.io", Integrated: false}
	md := FormatSettingsMarkdown(out)
	if md == "" {
		t.Error("expected non-empty markdown")
	}
}

// TestFormatListKeysMarkdown verifies the behavior of format list keys markdown.
func TestFormatListKeysMarkdown(t *testing.T) {
	out := ListClientKeysOutput{Keys: []ClientKeyItem{{ID: 1, Active: true, PublicKey: "pk", SentryDsn: "dsn"}}}
	md := FormatListKeysMarkdown(out)
	if md == "" {
		t.Error("expected non-empty markdown")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// Constants & fixtures
// ---------------------------------------------------------------------------.

const (
	covSettingsJSON = `{"active":true,"project_name":"proj","sentry_external_url":"https://sentry.io","api_url":"https://sentry.io/api","integrated":true}`
	covKeyJSON      = `{"id":1,"active":true,"public_key":"pk-abc","sentry_dsn":"https://dsn"}`
)

// ---------------------------------------------------------------------------
// ListClientKeys — pagination branch (Page > 0, PerPage > 0)
// ---------------------------------------------------------------------------.

// TestListClientKeys_WithPagination verifies the behavior of cov list client keys with pagination.
func TestListClientKeys_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/1/error_tracking/client_keys" && r.Method == http.MethodGet {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+covKeyJSON+`]`,
				testutil.PaginationHeaders{Page: "2", PerPage: "1", Total: "3", TotalPages: "3", NextPage: "3", PrevPage: "1"})
			return
		}
		http.NotFound(w, r)
	}))
	out, err := ListClientKeys(t.Context(), client, ListClientKeysInput{ProjectID: "1", Page: 2, PerPage: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(out.Keys))
	}
	if out.Pagination.TotalPages != 3 {
		t.Errorf("TotalPages = %d, want 3", out.Pagination.TotalPages)
	}
}

// ---------------------------------------------------------------------------
// FormatKeyMarkdown
// ---------------------------------------------------------------------------.

// TestFormatKeyMarkdown verifies the behavior of cov format key markdown.
func TestFormatKeyMarkdown(t *testing.T) {
	md := FormatKeyMarkdown(ClientKeyItem{ID: 42, Active: true, PublicKey: "pk-123", SentryDsn: "https://dsn.example.com"})
	for _, want := range []string{
		"## Error Tracking Client Key",
		"**ID**: 42",
		"**Active**: true",
		"**Public Key**: pk-123",
		"**Sentry DSN**: https://dsn.example.com",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatKeyMarkdown_Inactive verifies the behavior of cov format key markdown inactive.
func TestFormatKeyMarkdown_Inactive(t *testing.T) {
	md := FormatKeyMarkdown(ClientKeyItem{ID: 7, Active: false, PublicKey: "pk-xyz", SentryDsn: "dsn2"})
	if !strings.Contains(md, "**Active**: false") {
		t.Errorf("expected Active=false in markdown:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// FormatListKeysMarkdown — empty keys branch
// ---------------------------------------------------------------------------.

// TestFormatListKeysMarkdown_Empty verifies the behavior of cov format list keys markdown empty.
func TestFormatListKeysMarkdown_Empty(t *testing.T) {
	md := FormatListKeysMarkdown(ListClientKeysOutput{Keys: []ClientKeyItem{}})
	if !strings.Contains(md, "No client keys found") {
		t.Errorf("expected empty-keys message:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table header when empty")
	}
}

// TestFormatListKeysMarkdown_NilKeys verifies the behavior of cov format list keys markdown nil keys.
func TestFormatListKeysMarkdown_NilKeys(t *testing.T) {
	md := FormatListKeysMarkdown(ListClientKeysOutput{})
	if !strings.Contains(md, "No client keys found") {
		t.Errorf("expected empty-keys message:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// FormatSettingsMarkdown — minimal fields (no ProjectName, no SentryExternalURL)
// ---------------------------------------------------------------------------.

// TestFormatSettingsMarkdown_MinimalFields verifies the behavior of cov format settings markdown minimal fields.
func TestFormatSettingsMarkdown_MinimalFields(t *testing.T) {
	md := FormatSettingsMarkdown(SettingsOutput{Active: false, Integrated: true})
	if !strings.Contains(md, "**Active**: false") {
		t.Errorf("missing Active:\n%s", md)
	}
	if strings.Contains(md, "**Project Name**") {
		t.Error("should not contain Project Name when empty")
	}
	if strings.Contains(md, "**Sentry URL**") {
		t.Error("should not contain Sentry URL when empty")
	}
}

// ---------------------------------------------------------------------------
// RegisterTools — no panic
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of cov register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// ---------------------------------------------------------------------------
// RegisterTools — MCP round-trip for all 5 tools (success paths)
// ---------------------------------------------------------------------------.

// TestRegisterTools_CallAllThroughMCP validates cov register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := covNewErrorTrackingMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"get_settings", "gitlab_get_error_tracking_settings", map[string]any{"project_id": "1"}},
		{"enable_disable", "gitlab_enable_disable_error_tracking", map[string]any{"project_id": "1", "active": true, "integrated": true}},
		{"list_client_keys", "gitlab_list_error_tracking_client_keys", map[string]any{"project_id": "1", "page": 1, "per_page": 20}},
		{"create_client_key", "gitlab_create_error_tracking_client_key", map[string]any{"project_id": "1"}},
		{"delete_client_key", "gitlab_delete_error_tracking_client_key", map[string]any{"project_id": "1", "key_id": 10}},
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
// RegisterTools — MCP round-trip for all 5 tools (error paths)
// ---------------------------------------------------------------------------.

// TestRegisterTools_ErrorPathsThroughMCP validates cov register tools error paths through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_ErrorPathsThroughMCP(t *testing.T) {
	session := covNewErrorTrackingErrorMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"get_settings_err", "gitlab_get_error_tracking_settings", map[string]any{"project_id": "1"}},
		{"enable_disable_err", "gitlab_enable_disable_error_tracking", map[string]any{"project_id": "1", "active": true, "integrated": true}},
		{"list_client_keys_err", "gitlab_list_error_tracking_client_keys", map[string]any{"project_id": "1", "page": 1, "per_page": 20}},
		{"create_client_key_err", "gitlab_create_error_tracking_client_key", map[string]any{"project_id": "1"}},
		{"delete_client_key_err", "gitlab_delete_error_tracking_client_key", map[string]any{"project_id": "1", "key_id": 1}},
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

// ---------------------------------------------------------------------------
// RegisterMeta — no panic
// ---------------------------------------------------------------------------.

// TestRegisterMeta_NoPanic verifies the behavior of cov register meta no panic.
func TestRegisterMeta_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)
}

// ---------------------------------------------------------------------------
// RegisterMeta — MCP round-trip for all 5 actions
// ---------------------------------------------------------------------------.

// TestRegisterMeta_CallAllThroughMCP validates cov register meta call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterMeta_CallAllThroughMCP(t *testing.T) {
	session := covNewErrorTrackingMetaMCPSession(t)
	ctx := context.Background()

	actions := []struct {
		name   string
		action string
		params map[string]any
	}{
		{"get_settings", "get_settings", map[string]any{"project_id": "1"}},
		{"enable_disable", "enable_disable", map[string]any{"project_id": "1", "active": true, "integrated": true}},
		{"list_client_keys", "list_client_keys", map[string]any{"project_id": "1"}},
		{"create_client_key", "create_client_key", map[string]any{"project_id": "1"}},
		{"delete_client_key", "delete_client_key", map[string]any{"project_id": "1", "key_id": 10}},
	}

	for _, tt := range actions {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name: "gitlab_error_tracking",
				Arguments: map[string]any{
					"action": tt.action,
					"params": tt.params,
				},
			})
			if err != nil {
				t.Fatalf("CallTool(action=%s) error: %v", tt.action, err)
			}
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(action=%s) returned error: %s", tt.action, tc.Text)
					}
				}
				t.Fatalf("CallTool(action=%s) returned IsError=true", tt.action)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helper: MCP session for RegisterTools (success)
// ---------------------------------------------------------------------------.

// covNewErrorTrackingMCPSession is an internal helper for the errortracking package.
func covNewErrorTrackingMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	handler := http.NewServeMux()

	handler.HandleFunc("GET /api/v4/projects/1/error_tracking/settings", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covSettingsJSON)
	})

	handler.HandleFunc("PATCH /api/v4/projects/1/error_tracking/settings", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covSettingsJSON)
	})

	handler.HandleFunc("GET /api/v4/projects/1/error_tracking/client_keys", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+covKeyJSON+`]`)
	})

	handler.HandleFunc("POST /api/v4/projects/1/error_tracking/client_keys", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, covKeyJSON)
	})

	handler.HandleFunc("DELETE /api/v4/projects/1/error_tracking/client_keys/10", func(w http.ResponseWriter, _ *http.Request) {
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

// ---------------------------------------------------------------------------
// Helper: MCP session for RegisterTools (error paths)
// ---------------------------------------------------------------------------.

// covNewErrorTrackingErrorMCPSession is an internal helper for the errortracking package.
func covNewErrorTrackingErrorMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
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

// ---------------------------------------------------------------------------
// Helper: MCP session for RegisterMeta
// ---------------------------------------------------------------------------.

// covNewErrorTrackingMetaMCPSession is an internal helper for the errortracking package.
func covNewErrorTrackingMetaMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	handler := http.NewServeMux()

	handler.HandleFunc("GET /api/v4/projects/1/error_tracking/settings", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covSettingsJSON)
	})

	handler.HandleFunc("PATCH /api/v4/projects/1/error_tracking/settings", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covSettingsJSON)
	})

	handler.HandleFunc("GET /api/v4/projects/1/error_tracking/client_keys", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+covKeyJSON+`]`)
	})

	handler.HandleFunc("POST /api/v4/projects/1/error_tracking/client_keys", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, covKeyJSON)
	})

	handler.HandleFunc("DELETE /api/v4/projects/1/error_tracking/client_keys/10", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)

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
