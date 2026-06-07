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

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// settingsJSON identifies the settings JSON constant used by this package.
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

// TestGet_Success verifies Get when success.
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

// TestGet_Error verifies Get when error.
func TestGet_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := Get(t.Context(), client, GetInput{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestUpdate_Success verifies Update when success.
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

// TestUpdate_Error verifies Update when error.
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

// TestUpdate_EmptySettings verifies Update when empty settings.
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

// TestFormatGetMarkdown verifies FormatGetMarkdown.
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

// TestFormatUpdateMarkdown verifies FormatUpdateMarkdown.
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

// TestUpdate_UnmarshalOptionsError verifies Update rejects values that marshal
// to JSON but cannot be decoded into GitLab update option field types.
func TestUpdate_UnmarshalOptionsError(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())

	_, err := Update(t.Context(), client, UpdateInput{
		Settings: map[string]any{"signup_enabled": "not-a-bool"},
	})
	if err == nil {
		t.Fatal("expected error for invalid option field type")
	}
	if !strings.Contains(err.Error(), "unmarshal to options") {
		t.Errorf("expected 'unmarshal to options' in error, got: %v", err)
	}
}

// TestActionSpecs_Metadata verifies canonical metadata for settings actions.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, settingsJSON)
	}))
	specs := ActionSpecs(client)
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}

	if len(specs) != 2 {
		t.Fatalf("len(ActionSpecs) = %d, want 2", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "settings" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
		if spec.Usage == "" {
			t.Fatalf("Usage for %s should not be empty", spec.Name)
		}
		if len(spec.Aliases) == 0 {
			t.Fatalf("Aliases for %s should not be empty", spec.Name)
		}
	}
	if specByTool["gitlab_update_settings"].ParameterGuidance["settings"].SemanticRole == "" {
		t.Fatal("gitlab_update_settings should define settings parameter guidance")
	}
}

// TestActionSpecs_CallRoutes verifies that both settings canonical routes
// execute successfully through ActionSpecs.
func TestActionSpecs_CallRoutes(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/application/settings", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, settingsJSON)
	})
	handler.HandleFunc("PUT /api/v4/application/settings", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, settingsJSON)
	})

	client := testutil.NewTestClient(t, handler)
	specs := ActionSpecs(client)
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}

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
			spec, ok := specByTool[tt.tool]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tt.tool)
			}
			if spec.OwnerPackage != "settings" || !spec.Idempotent || !spec.OpenWorld {
				t.Fatalf("unexpected ActionSpec semantics for %s: %+v", tt.tool, spec)
			}
			result, err := spec.Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.tool)
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

// TestGet_Success_FullRoundTrip verifies that Get handles a complete settings response.
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

// TestUpdate_BadRequest verifies Update includes the settings-key guidance for
// GitLab validation errors.
func TestUpdate_BadRequest(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"unknown setting"}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{Settings: map[string]any{"unknown_setting": true}})
	if err == nil {
		t.Fatal("expected error for 400")
	}
	if !strings.Contains(err.Error(), "snake_case") {
		t.Fatalf("error missing settings guidance: %v", err)
	}
}

// TestGet_UnmarshalResponseError documents the contract for Get when the
// API returns a body that the GitLab SDK cannot decode into the Settings
// struct (e.g. a bare number). The SDK rejects it before our json.Unmarshal
// runs, so the in-package json.Unmarshal error branch (settings.go:40-42)
// is defense-in-depth. We assert that an error is returned, which is the
// externally observable contract regardless of which layer surfaces it.
func TestGet_UnmarshalResponseError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// A bare JSON number is invalid for the Settings object type.
		testutil.RespondJSON(w, http.StatusOK, `42`)
	}))
	_, err := Get(t.Context(), client, GetInput{})
	if err == nil {
		t.Fatal("expected error for non-object response, got nil")
	}
}

// TestUpdate_UnmarshalResponseError documents the contract for Update when
// the API returns a body that cannot be decoded into map[string]any. The
// SDK rejects the non-object body before our json.Unmarshal runs, so the
// in-package json.Unmarshal error branch (settings.go:89-91) is
// defense-in-depth. We assert that an error is returned, satisfying the
// contract regardless of which layer surfaces the failure.
func TestUpdate_UnmarshalResponseError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `42`)
	}))
	_, err := Update(t.Context(), client, UpdateInput{Settings: map[string]any{"signup_enabled": false}})
	if err == nil {
		t.Fatal("expected error for non-object response, got nil")
	}
}
