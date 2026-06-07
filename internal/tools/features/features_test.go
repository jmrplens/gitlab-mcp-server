// features_test.go contains unit tests for the GitLab feature MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package features

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

// fmtUnexpPath identifies the fmt unexp path constant used by this package.
const fmtUnexpPath = "unexpected path: %s"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// TestList_Success verifies List when success.
func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/features" {
			t.Errorf(fmtUnexpPath, r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK, `[
			{"name":"flag1","state":"on","gates":[{"key":"boolean","value":true}],"definition":null},
			{"name":"flag2","state":"off","gates":[],"definition":{"name":"flag2","type":"development","group":"group::ide","milestone":"15.0","default_enabled":false,"log_state_changes":false}}
		]`)
	}))

	out, err := List(t.Context(), client, ListInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Features) != 2 {
		t.Fatalf("expected 2 features, got %d", len(out.Features))
	}
	if out.Features[0].Name != "flag1" {
		t.Errorf("expected flag1, got %s", out.Features[0].Name)
	}
	if out.Features[0].State != "on" {
		t.Errorf("expected on, got %s", out.Features[0].State)
	}
	if len(out.Features[0].Gates) != 1 {
		t.Errorf("expected 1 gate, got %d", len(out.Features[0].Gates))
	}
	if out.Features[1].Definition == nil {
		t.Fatal("expected definition for flag2")
	}
	if out.Features[1].Definition.Type != "development" {
		t.Errorf("expected development, got %s", out.Features[1].Definition.Type)
	}
}

// TestList_Error verifies List when error.
func TestList_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	_, err := List(t.Context(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestListDefinitions_Success verifies ListDefinitions when success.
func TestListDefinitions_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/features/definitions" {
			t.Errorf(fmtUnexpPath, r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK, `[
			{"name":"def1","introduced_by_url":"https://example.com","type":"development","group":"group::ide","milestone":"15.0","default_enabled":true,"log_state_changes":false,"rollout_issue_url":""},
			{"name":"def2","introduced_by_url":"","type":"ops","group":"group::ops","milestone":"16.0","default_enabled":false,"log_state_changes":true,"rollout_issue_url":"https://rollout.example.com"}
		]`)
	}))

	out, err := ListDefinitions(t.Context(), client, ListDefinitionsInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Definitions) != 2 {
		t.Fatalf("expected 2 definitions, got %d", len(out.Definitions))
	}
	if out.Definitions[0].Name != "def1" {
		t.Errorf("expected def1, got %s", out.Definitions[0].Name)
	}
	if !out.Definitions[0].DefaultEnabled {
		t.Error("expected default_enabled true")
	}
}

// TestSet_Success verifies Set when success.
func TestSet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v4/features/my_flag" {
			t.Errorf(fmtUnexpPath, r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusCreated, `{"name":"my_flag","state":"on","gates":[{"key":"boolean","value":true}],"definition":null}`)
	}))

	out, err := Set(t.Context(), client, SetInput{Name: "my_flag", Value: true})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Feature.Name != "my_flag" {
		t.Errorf("expected my_flag, got %s", out.Feature.Name)
	}
	if out.Feature.State != "on" {
		t.Errorf("expected on, got %s", out.Feature.State)
	}
}

// TestDelete_Success verifies Delete when success.
func TestDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if r.URL.Path != "/api/v4/features/my_flag" {
			t.Errorf(fmtUnexpPath, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	err := Delete(t.Context(), client, DeleteInput{Name: "my_flag"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDelete_Error verifies Delete when error.
func TestDelete_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	err := Delete(t.Context(), client, DeleteInput{Name: "no_flag"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestFormatListMarkdown verifies FormatListMarkdown.
func TestFormatListMarkdown(t *testing.T) {
	result := FormatListMarkdown(ListOutput{
		Features: []FeatureItem{
			{Name: "flag1", State: "on", Gates: []GateItem{{Key: "boolean", Value: true}}},
			{Name: "flag2", State: "conditional", Gates: []GateItem{{Key: "percentage_of_time", Value: 50}}},
		},
	})
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "flag1") || !strings.Contains(text, "flag2") {
		t.Errorf("expected flags in output, got: %s", text)
	}
	if !strings.Contains(text, "boolean=true") {
		t.Errorf("expected gate info, got: %s", text)
	}
}

// TestFormatListMarkdown_Empty verifies FormatListMarkdown when empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	result := FormatListMarkdown(ListOutput{})
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "No feature flags found") {
		t.Errorf("expected empty message, got: %s", text)
	}
}

// TestFormatListDefinitionsMarkdown verifies FormatListDefinitionsMarkdown.
func TestFormatListDefinitionsMarkdown(t *testing.T) {
	result := FormatListDefinitionsMarkdown(ListDefinitionsOutput{
		Definitions: []DefinitionItem{
			{Name: "def1", Type: "development", Group: "group::ide", Milestone: "15.0", DefaultEnabled: true},
		},
	})
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "def1") || !strings.Contains(text, "development") {
		t.Errorf("expected definition info, got: %s", text)
	}
}

// TestFormatFeatureMarkdown verifies FormatFeatureMarkdown.
func TestFormatFeatureMarkdown(t *testing.T) {
	result := FormatFeatureMarkdown(SetOutput{
		Feature: FeatureItem{
			Name:  "my_flag",
			State: "on",
			Gates: []GateItem{{Key: "boolean", Value: true}},
			Definition: &DefinitionItem{
				Type:           "development",
				Group:          "group::ide",
				DefaultEnabled: false,
			},
		},
	})
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "my_flag") || !strings.Contains(text, "development") {
		t.Errorf("expected feature info, got: %s", text)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// Set — API error
// ---------------------------------------------------------------------------.

// TestSet_APIError verifies Set when API error.
func TestSet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	_, err := Set(context.Background(), client, SetInput{Name: "flag", Value: true})
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Set — with all optional fields
// ---------------------------------------------------------------------------.

// TestSet_AllOptionalFields verifies Set when all optional fields.
func TestSet_AllOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, `{"name":"flag","state":"conditional","gates":[{"key":"percentage_of_time","value":50}]}`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Set(context.Background(), client, SetInput{
		Name:         "flag",
		Value:        50,
		Key:          "percentage_of_time",
		FeatureGroup: "beta",
		User:         "admin",
		Group:        "mygroup",
		Namespace:    "myns",
		Project:      "myns/myproj",
		Repository:   "myns/myproj",
		Force:        true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Feature.State != "conditional" {
		t.Errorf("expected conditional, got %s", out.Feature.State)
	}
}

// ---------------------------------------------------------------------------
// ListDefinitions — API error
// ---------------------------------------------------------------------------.

// TestListDefinitions_APIError verifies ListDefinitions when API error.
func TestListDefinitions_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	_, err := ListDefinitions(context.Background(), client, ListDefinitionsInput{})
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Formatter — empty definitions
// ---------------------------------------------------------------------------.

// TestFormatListDefinitionsMarkdown_Empty verifies FormatListDefinitionsMarkdown when empty.
func TestFormatListDefinitionsMarkdown_Empty(t *testing.T) {
	result := FormatListDefinitionsMarkdown(ListDefinitionsOutput{})
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "No feature definitions found") {
		t.Errorf("expected empty message, got: %s", text)
	}
}

// ---------------------------------------------------------------------------
// Formatter — feature without definition
// ---------------------------------------------------------------------------.

// TestFormatFeatureMarkdown_NoDefinition verifies FormatFeatureMarkdown when no definition.
func TestFormatFeatureMarkdown_NoDefinition(t *testing.T) {
	result := FormatFeatureMarkdown(SetOutput{
		Feature: FeatureItem{
			Name:  "simple_flag",
			State: "on",
			Gates: []GateItem{},
		},
	})
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "simple_flag") {
		t.Errorf("expected flag name, got: %s", text)
	}
	if strings.Contains(text, "Type") {
		t.Errorf("should not contain Type when no definition: %s", text)
	}
}

// ---------------------------------------------------------------------------
// Set — NewRequest error when the body contains a non-JSON-serializable value
// ---------------------------------------------------------------------------.

// TestSet_NewRequestErrorOnUnserializableValue verifies that Set surfaces an
// error when the user-supplied value field cannot be marshaled to JSON (for
// example, a channel or function). GitLab's client-go NewRequest returns
// the marshal error directly, and the handler wraps it with the operation
// name so the LLM sees a meaningful diagnostic.
func TestSet_NewRequestErrorOnUnserializableValue(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API should not be called when body fails to marshal")
	}))

	_, err := Set(context.Background(), client, SetInput{Name: "flag1", Value: make(chan int)})
	if err == nil {
		t.Fatal("expected error for non-JSON-serializable value, got nil")
	}
	if !strings.Contains(err.Error(), "feature_set") {
		t.Errorf("error = %q, want wrapped with feature_set operation", err.Error())
	}
}
