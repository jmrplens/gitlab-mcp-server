// protected_packages_test.go contains unit tests for GitLab protected package
// operations. Tests use httptest to mock the GitLab Protected Packages API.
package protectedpackages

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const (
	testProjectID = "myproject"
	pathRules     = "/api/v4/projects/myproject/packages/protection/rules"
	pathRule1     = "/api/v4/projects/myproject/packages/protection/rules/1"

	ruleJSON = `{
		"id": 1,
		"project_id": 42,
		"package_name_pattern": "@scope/pkg*",
		"package_type": "npm",
		"minimum_access_level_for_push": "maintainer",
		"minimum_access_level_for_delete": "owner"
	}`
)

// List tests.

func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathRules {
			testutil.RespondJSON(w, http.StatusOK, "["+ruleJSON+"]")
			return
		}
		http.NotFound(w, r)
	}))
	out, err := List(context.Background(), client, ListInput{ProjectID: testProjectID})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(out.Rules) != 1 {
		t.Fatalf("len(Rules) = %d, want 1", len(out.Rules))
	}
	if out.Rules[0].ID != 1 {
		t.Errorf("ID = %d, want 1", out.Rules[0].ID)
	}
	if out.Rules[0].ProjectID != 42 {
		t.Errorf("ProjectID = %d, want 42", out.Rules[0].ProjectID)
	}
	if out.Rules[0].PackageNamePattern != "@scope/pkg*" {
		t.Errorf("PackageNamePattern = %q", out.Rules[0].PackageNamePattern)
	}
	if out.Rules[0].PackageType != "npm" {
		t.Errorf("PackageType = %q", out.Rules[0].PackageType)
	}
	if out.Rules[0].MinimumAccessLevelForPush != "maintainer" {
		t.Errorf("MinPush = %q", out.Rules[0].MinimumAccessLevelForPush)
	}
	if out.Rules[0].MinimumAccessLevelForDelete != "owner" {
		t.Errorf("MinDelete = %q", out.Rules[0].MinimumAccessLevelForDelete)
	}
}

func TestList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{ProjectID: testProjectID})
	if err == nil {
		t.Fatal("expected context error")
	}
}

func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: testProjectID})
	if err == nil {
		t.Fatal("expected error for 403")
	}
}

func TestList_Pagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") != "2" {
			t.Errorf("page = %q, want 2", r.URL.Query().Get("page"))
		}
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	_, err := List(context.Background(), client, ListInput{
		ProjectID:       testProjectID,
		PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 10},
	})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
}

// Create tests.

func TestCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathRules {
			testutil.RespondJSON(w, http.StatusCreated, ruleJSON)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:                   testProjectID,
		PackageNamePattern:          "@scope/pkg*",
		PackageType:                 "npm",
		MinimumAccessLevelForPush:   "maintainer",
		MinimumAccessLevelForDelete: "owner",
	})
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("ID = %d, want 1", out.ID)
	}
	if out.MinimumAccessLevelForPush != "maintainer" {
		t.Errorf("MinPush = %q, want maintainer", out.MinimumAccessLevelForPush)
	}
}

func TestCreate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(context.Background(), client, CreateInput{
		PackageNamePattern: "@scope/pkg*",
		PackageType:        "npm",
	})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

func TestCreate_MissingPattern(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(context.Background(), client, CreateInput{
		ProjectID:   testProjectID,
		PackageType: "npm",
	})
	if err == nil {
		t.Fatal("expected error for missing package_name_pattern")
	}
}

func TestCreate_MissingPackageType(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(context.Background(), client, CreateInput{
		ProjectID:          testProjectID,
		PackageNamePattern: "@scope/pkg*",
	})
	if err == nil {
		t.Fatal("expected error for missing package_type")
	}
}

func TestCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Create(ctx, client, CreateInput{
		ProjectID:          testProjectID,
		PackageNamePattern: "@scope/pkg*",
		PackageType:        "npm",
	})
	if err == nil {
		t.Fatal("expected context error")
	}
}

func TestCreate_WithoutAccessLevels(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathRules {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id": 2,
				"project_id": 42,
				"package_name_pattern": "mylib*",
				"package_type": "pypi"
			}`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:          testProjectID,
		PackageNamePattern: "mylib*",
		PackageType:        "pypi",
	})
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if out.ID != 2 {
		t.Errorf("ID = %d, want 2", out.ID)
	}
	if out.MinimumAccessLevelForPush != "" {
		t.Errorf("MinPush = %q, want empty", out.MinimumAccessLevelForPush)
	}
}

// Update tests.

func TestUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && r.URL.Path == pathRule1 {
			testutil.RespondJSON(w, http.StatusOK, ruleJSON)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID:                   testProjectID,
		RuleID:                      1,
		MinimumAccessLevelForPush:   "maintainer",
		MinimumAccessLevelForDelete: "admin",
	})
	if err != nil {
		t.Fatalf("Update() error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("ID = %d, want 1", out.ID)
	}
}

func TestUpdate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Update(context.Background(), client, UpdateInput{RuleID: 1})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

func TestUpdate_MissingRuleID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: testProjectID})
	if err == nil {
		t.Fatal("expected error for missing rule_id")
	}
}

func TestUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Update(ctx, client, UpdateInput{ProjectID: testProjectID, RuleID: 1})
	if err == nil {
		t.Fatal("expected context error")
	}
}

func TestUpdate_PartialFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && r.URL.Path == pathRule1 {
			testutil.RespondJSON(w, http.StatusOK, ruleJSON)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID:          testProjectID,
		RuleID:             1,
		PackageNamePattern: "@scope/new-pkg*",
		PackageType:        "maven",
	})
	if err != nil {
		t.Fatalf("Update() error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("ID = %d, want 1", out.ID)
	}
}

// Delete tests.

func TestDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathRule1 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: testProjectID, RuleID: 1})
	if err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
}

func TestDelete_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := Delete(context.Background(), client, DeleteInput{RuleID: 1})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

func TestDelete_MissingRuleID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: testProjectID})
	if err == nil {
		t.Fatal("expected error for missing rule_id")
	}
}

func TestDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx := testutil.CancelledCtx(t)
	err := Delete(ctx, client, DeleteInput{ProjectID: testProjectID, RuleID: 1})
	if err == nil {
		t.Fatal("expected context error")
	}
}

// Markdown tests.

func TestFormatOutputMarkdown_Basic(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		ID:                          1,
		PackageNamePattern:          "@scope/pkg*",
		PackageType:                 "npm",
		MinimumAccessLevelForPush:   "maintainer",
		MinimumAccessLevelForDelete: "owner",
	})
	if !contains(md, "## Package Protection Rule #1") {
		t.Error("missing header")
	}
	if !contains(md, "@scope/pkg*") {
		t.Error("missing pattern")
	}
	if !contains(md, "MinimumAccessLevelForPush") || !contains(md, "Min Push Level") {
		// check for at least one — implementation uses "Min Push Level"
		if !contains(md, "Min Push Level") {
			t.Error("missing push level")
		}
	}
}

func TestFormatOutputMarkdown_Empty(t *testing.T) {
	md := FormatOutputMarkdown(Output{})
	if md != "" {
		t.Errorf("expected empty string, got %q", md)
	}
}

func TestFormatOutputMarkdown_NoAccessLevels(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		ID:                 2,
		PackageNamePattern: "mylib*",
		PackageType:        "pypi",
	})
	if !contains(md, "## Package Protection Rule #2") {
		t.Error("missing header")
	}
	if contains(md, "Min Push Level") {
		t.Error("should not contain push level when empty")
	}
	if contains(md, "Min Delete Level") {
		t.Error("should not contain delete level when empty")
	}
}

func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if !contains(md, "No package protection rules found") {
		t.Error("missing empty message")
	}
}

func TestFormatListMarkdown_WithRules(t *testing.T) {
	md := FormatListMarkdown(ListOutput{
		Rules: []Output{
			{ID: 1, PackageNamePattern: "@scope/pkg*", PackageType: "npm", MinimumAccessLevelForPush: "maintainer"},
			{ID: 2, PackageNamePattern: "mylib*", PackageType: "pypi"},
		},
	})
	if !contains(md, "| 1 |") {
		t.Error("missing rule 1 row")
	}
	if !contains(md, "| 2 |") {
		t.Error("missing rule 2 row")
	}
	if !contains(md, "npm") {
		t.Error("missing npm type")
	}
}

// RegisterTools tests.

func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newProtectedPackagesMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_list_package_protection_rules", map[string]any{"project_id": testProjectID}},
		{"gitlab_create_package_protection_rule", map[string]any{"project_id": testProjectID, "package_name_pattern": "@scope/pkg*", "package_type": "npm"}},
		{"gitlab_update_package_protection_rule", map[string]any{"project_id": testProjectID, "rule_id": 1}},
		{"gitlab_delete_package_protection_rule", map[string]any{"project_id": testProjectID, "rule_id": 1}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.name,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.name, err)
			}
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(%s) returned error: %s", tt.name, tc.Text)
					}
				}
				t.Fatalf("CallTool(%s) returned IsError=true", tt.name)
			}
		})
	}
}

// TestCreate_APIError covers the API error path in Create.
func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: "1", PackageNamePattern: "pkg-*", PackageType: "npm"})
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

// TestUpdate_APIError covers the API error path in Update.
func TestUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "1", RuleID: 1})
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

// TestDelete_APIError covers the API error path in Delete.
func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: "1", RuleID: 1})
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

// TestMCPRoundTrip_DeleteConfirmDeclined covers the ConfirmAction early-return
// branch in delete_package_protection_rule when user declines.
func TestMCPRoundTrip_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
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
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_delete_package_protection_rule",
		Arguments: map[string]any{"project_id": "1", "rule_id": float64(1)},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

// TestMCPRoundTrip_DeleteError covers the delete error path through register.go
// when the backend returns a 500 after the user confirms the action.
func TestMCPRoundTrip_DeleteError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "accept"}, nil
		},
	})
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_delete_package_protection_rule",
		Arguments: map[string]any{"project_id": "1", "rule_id": float64(1)},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for 500 backend")
	}
}

func newProtectedPackagesMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && path == pathRules:
			testutil.RespondJSONWithPagination(w, http.StatusOK, "["+ruleJSON+"]",
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
		case r.Method == http.MethodPost && path == pathRules:
			testutil.RespondJSON(w, http.StatusCreated, ruleJSON)
		case r.Method == http.MethodPatch && path == pathRule1:
			testutil.RespondJSON(w, http.StatusOK, ruleJSON)
		case r.Method == http.MethodDelete && path == pathRule1:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))

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

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && containsSubstring(s, substr)
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
