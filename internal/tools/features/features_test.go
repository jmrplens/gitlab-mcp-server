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

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const fmtUnexpPath = "unexpected path: %s"

const fmtUnexpErr = "unexpected error: %v"

// TestList_Success verifies that List handles the success scenario correctly.
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

// TestList_Error verifies that List handles the error scenario correctly.
func TestList_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	_, err := List(t.Context(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestListDefinitions_Success verifies that ListDefinitions handles the success scenario correctly.
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

// TestSet_Success verifies that Set handles the success scenario correctly.
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

// TestDelete_Success verifies that Delete handles the success scenario correctly.
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

// TestDelete_Error verifies that Delete handles the error scenario correctly.
func TestDelete_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	err := Delete(t.Context(), client, DeleteInput{Name: "no_flag"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestFormatListMarkdown verifies the behavior of format list markdown.
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

// TestFormatListMarkdown_Empty verifies that FormatListMarkdown handles the empty scenario correctly.
func TestFormatListMarkdown_Empty(t *testing.T) {
	result := FormatListMarkdown(ListOutput{})
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "No feature flags found") {
		t.Errorf("expected empty message, got: %s", text)
	}
}

// TestFormatListDefinitionsMarkdown verifies the behavior of format list definitions markdown.
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

// TestFormatFeatureMarkdown verifies the behavior of format feature markdown.
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

// TestSet_APIError verifies the behavior of set a p i error.
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

// TestSet_AllOptionalFields verifies the behavior of set all optional fields.
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

// TestListDefinitions_APIError verifies the behavior of list definitions a p i error.
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

// TestFormatListDefinitionsMarkdown_Empty verifies the behavior of format list definitions markdown empty.
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

// TestFormatFeatureMarkdown_NoDefinition verifies the behavior of format feature markdown no definition.
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
	session := newFeaturesMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list_features", "gitlab_list_features", map[string]any{}},
		{"list_feature_definitions", "gitlab_list_feature_definitions", map[string]any{}},
		{"set_feature_flag", "gitlab_set_feature_flag", map[string]any{"name": "flag1", "value": true}},
		{"delete_feature_flag", "gitlab_delete_feature_flag", map[string]any{"name": "flag1"}},
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

// newFeaturesMCPSession is an internal helper for the features package.
// TestMCPRoundTrip_ErrorPaths covers the error return paths in register.go
// handlers when the GitLab API returns an error.
func TestMCPRoundTrip_ErrorPaths(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})
	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)
	session, connectErr := mcpClient.Connect(ctx, ct, nil)
	if connectErr != nil {
		t.Fatalf("client connect: %v", connectErr)
	}
	t.Cleanup(func() { session.Close() })

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_list_features", map[string]any{}},
		{"gitlab_set_feature_flag", map[string]any{"name": "test_flag", "value": "true"}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("unexpected transport error: %v", err)
			}
			if result == nil || !result.IsError {
				t.Fatalf("expected error result for %s with 500 backend", tt.name)
			}
		})
	}
}

// TestMCPRoundTrip_DeleteConfirmDeclined covers the ConfirmAction early-return
// branch in delete_feature_flag when user declines.
func TestMCPRoundTrip_DeleteConfirmDeclined(t *testing.T) {
	handler := http.NewServeMux()
	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "decline"}, nil
		},
	})
	session, connectErr := mcpClient.Connect(ctx, ct, nil)
	if connectErr != nil {
		t.Fatalf("client connect: %v", connectErr)
	}
	t.Cleanup(func() { session.Close() })

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_delete_feature_flag",
		Arguments: map[string]any{"name": "test_flag"},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

func newFeaturesMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	featureJSON := `{"name":"flag1","state":"on","gates":[{"key":"boolean","value":true}]}`

	handler := http.NewServeMux()

	handler.HandleFunc("GET /api/v4/features", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+featureJSON+`]`)
	})

	handler.HandleFunc("GET /api/v4/features/definitions", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"name":"def1","type":"development","group":"group::ide","milestone":"15.0","default_enabled":true,"log_state_changes":false}]`)
	})

	handler.HandleFunc("POST /api/v4/features/flag1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, featureJSON)
	})

	handler.HandleFunc("DELETE /api/v4/features/flag1", func(w http.ResponseWriter, _ *http.Request) {
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
	session, connectErr := mcpClient.Connect(ctx, ct, nil)
	if connectErr != nil {
		t.Fatalf("client connect: %v", connectErr)
	}
	t.Cleanup(func() { session.Close() })
	return session
}
