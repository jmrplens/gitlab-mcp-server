// settings_test.go contains unit tests for the application settings MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.

package settings

import (
	"context"
	"math"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const fmtUnexpErr = "unexpected error: %v"

const settingsJSON = `{
	"id": 1,
	"signup_enabled": true,
	"default_project_visibility": "private",
	"default_group_visibility": "private",
	"default_snippet_visibility": "internal",
	"can_create_group": true,
	"auto_devops_enabled": false,
	"shared_runners_enabled": true,
	"max_artifacts_size": 100,
	"default_branch_name": "main",
	"password_authentication_enabled_for_web": true,
	"require_two_factor_authentication": false,
	"throttle_authenticated_api_enabled": false
}`

// TestGet_Success verifies that Get handles the success scenario correctly.
func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/application/settings" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, settingsJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(t.Context(), client, GetInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Settings == nil {
		t.Fatal("expected settings map, got nil")
	}
	if val, ok := out.Settings["signup_enabled"]; !ok || val != true {
		t.Errorf("expected signup_enabled=true, got %v", val)
	}
	if val, ok := out.Settings["default_project_visibility"]; !ok || val != "private" {
		t.Errorf("expected default_project_visibility=private, got %v", val)
	}
}

// TestGet_Error verifies that Get handles the error scenario correctly.
func TestGet_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := Get(t.Context(), client, GetInput{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestUpdate_Success verifies that Update handles the success scenario correctly.
func TestUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/application/settings" && r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, settingsJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Update(t.Context(), client, UpdateInput{
		Settings: map[string]any{
			"signup_enabled":             false,
			"default_project_visibility": "internal",
		},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Settings == nil {
		t.Fatal("expected settings map, got nil")
	}
}

// TestUpdate_Error verifies that Update handles the error scenario correctly.
func TestUpdate_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := Update(t.Context(), client, UpdateInput{
		Settings: map[string]any{"signup_enabled": false},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestUpdate_EmptySettings verifies that Update handles the empty settings scenario correctly.
func TestUpdate_EmptySettings(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/application/settings" && r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, settingsJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Update(t.Context(), client, UpdateInput{
		Settings: map[string]any{},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Settings == nil {
		t.Fatal("expected settings map, got nil")
	}
}

// TestFormatGetMarkdown verifies the behavior of format get markdown.
func TestFormatGetMarkdown(t *testing.T) {
	out := GetOutput{
		Settings: map[string]any{
			"signup_enabled":             true,
			"default_project_visibility": "private",
			"auto_devops_enabled":        false,
			"default_branch_name":        "main",
		},
	}
	result := FormatGetMarkdown(out)
	content := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(content, "Application Settings") {
		t.Error("expected 'Application Settings' header in markdown")
	}
	if !strings.Contains(content, "Total settings: 4") {
		t.Error("expected total settings count")
	}
}

// TestFormatUpdateMarkdown verifies the behavior of format update markdown.
func TestFormatUpdateMarkdown(t *testing.T) {
	out := UpdateOutput{
		Settings: map[string]any{
			"signup_enabled": false,
		},
	}
	result := FormatUpdateMarkdown(out)
	content := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(content, "Updated") {
		t.Error("expected 'Updated' in markdown")
	}
}

// TestUpdate_MarshalInputError verifies that Update returns an error when the
// input settings map contains a value that cannot be marshaled to JSON (e.g. NaN).
func TestUpdate_MarshalInputError(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())

	_, err := Update(t.Context(), client, UpdateInput{
		Settings: map[string]any{"bad_value": math.NaN()},
	})
	if err == nil {
		t.Fatal("expected error for unmarshalable input, got nil")
	}
	if !strings.Contains(err.Error(), "marshal input") {
		t.Errorf("expected 'marshal input' in error, got: %v", err)
	}
}

// TestMCPRound_Trip verifies that RegisterTools correctly wires both settings
// tools and that they can be called through the MCP protocol.
func TestMCPRound_Trip(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/application/settings", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, settingsJSON)
	})
	handler.HandleFunc("PUT /api/v4/application/settings", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, settingsJSON)
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

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"get", "gitlab_get_settings", nil},
		{"update", "gitlab_update_settings", map[string]any{"settings": map[string]any{"signup_enabled": false}}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			var result *mcp.CallToolResult
			result, err = session.CallTool(ctx, &mcp.CallToolParams{
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

// TestGet_APIError verifies that Get returns a wrapped error when the API fails.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := Get(context.Background(), client, GetInput{})
	if err == nil {
		t.Fatal("expected error for 403")
	}
}

// TestGet_MarshalError verifies that Get handles the json.Marshal step.
// Since gl.Settings always marshals fine, we test the unmarshal step
// by returning a valid response and verifying the full round-trip.
func TestGet_Success_FullRoundTrip(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"signup_enabled":true,"default_project_visibility":"private"}`)
	}))
	out, err := Get(context.Background(), client, GetInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Settings == nil {
		t.Fatal("expected non-nil Settings map")
	}
}

// TestUpdate_APIError verifies that Update returns a wrapped error when the API fails.
func TestUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{Settings: map[string]any{"signup_enabled": false}})
	if err == nil {
		t.Fatal("expected error for 403")
	}
}
