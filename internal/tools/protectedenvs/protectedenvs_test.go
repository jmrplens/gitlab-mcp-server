// protectedenvs_test.go contains unit tests for the protected environment MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package protectedenvs

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const (
	pathProtectedEnvs = "/api/v4/projects/42/protected_environments"
	pathProtectedEnv1 = "/api/v4/projects/42/protected_environments/production"
	envJSON           = `{
		"name": "production",
		"deploy_access_levels": [
			{"id": 1, "access_level": 40, "access_level_description": "Maintainers", "user_id": 0, "group_id": 0}
		],
		"required_approval_count": 2,
		"approval_rules": [
			{"id": 10, "user_id": 5, "group_id": 0, "access_level": 40, "access_level_description": "Maintainers", "required_approvals": 1}
		]
	}`
)

// ---------- List ----------.

// TestList_Success verifies the behavior of list success.
func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProtectedEnvs {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+envJSON+`]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Environments) != 1 {
		t.Fatalf("len(Environments) = %d, want 1", len(out.Environments))
	}
	if out.Environments[0].Name != "production" {
		t.Errorf("Name = %q, want %q", out.Environments[0].Name, "production")
	}
	if out.Environments[0].RequiredApprovalCount != 2 {
		t.Errorf("RequiredApprovalCount = %d, want 2", out.Environments[0].RequiredApprovalCount)
	}
}

// TestList_MissingProjectID verifies the behavior of list missing project i d.
func TestList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("List() expected error for missing project_id")
	}
}

// ---------- Get ----------.

// TestGet_Success verifies the behavior of get success.
func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProtectedEnv1 {
			testutil.RespondJSON(w, http.StatusOK, envJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", Environment: "production"})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.Name != "production" {
		t.Errorf("Name = %q, want %q", out.Name, "production")
	}
	if len(out.DeployAccessLevels) != 1 {
		t.Fatalf("len(DeployAccessLevels) = %d, want 1", len(out.DeployAccessLevels))
	}
	if out.DeployAccessLevels[0].AccessLevel != 40 {
		t.Errorf("AccessLevel = %d, want 40", out.DeployAccessLevels[0].AccessLevel)
	}
	if len(out.ApprovalRules) != 1 {
		t.Fatalf("len(ApprovalRules) = %d, want 1", len(out.ApprovalRules))
	}
	if out.ApprovalRules[0].RequiredApprovalCount != 1 {
		t.Errorf("RequiredApprovalCount = %d, want 1", out.ApprovalRules[0].RequiredApprovalCount)
	}
}

// TestGet_MissingProjectID verifies the behavior of get missing project i d.
func TestGet_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Get(context.Background(), client, GetInput{Environment: "production"})
	if err == nil {
		t.Fatal("Get() expected error for missing project_id")
	}
}

// TestGet_MissingEnvironment verifies the behavior of get missing environment.
func TestGet_MissingEnvironment(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Get(context.Background(), client, GetInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("Get() expected error for missing environment")
	}
}

// ---------- Protect ----------.

// TestProtect_Success verifies the behavior of protect success.
func TestProtect_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProtectedEnvs {
			testutil.RespondJSON(w, http.StatusCreated, envJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Protect(context.Background(), client, ProtectInput{
		ProjectID: "42",
		Name:      "production",
	})
	if err != nil {
		t.Fatalf("Protect() unexpected error: %v", err)
	}
	if out.Name != "production" {
		t.Errorf("Name = %q, want %q", out.Name, "production")
	}
}

// TestProtect_WithAccessLevels verifies the behavior of protect with access levels.
func TestProtect_WithAccessLevels(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProtectedEnvs {
			testutil.RespondJSON(w, http.StatusCreated, envJSON)
			return
		}
		http.NotFound(w, r)
	}))

	al := 40
	_, err := Protect(context.Background(), client, ProtectInput{
		ProjectID: "42",
		Name:      "production",
		DeployAccessLevels: []DeployAccessLevelInput{
			{AccessLevel: &al},
		},
		ApprovalRules: []ApprovalRuleInput{
			{AccessLevel: &al, RequiredApprovalCount: new(int64(1))},
		},
	})
	if err != nil {
		t.Fatalf("Protect() unexpected error: %v", err)
	}
}

// TestProtect_WithAllOptionalFields validates the Protect function covers all
// optional field branches: UserID, GroupID, GroupInheritanceType in both
// DeployAccessLevels and ApprovalRules.
func TestProtect_WithAllOptionalFields(t *testing.T) {
	var capturedBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProtectedEnvs {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			capturedBody = string(body)
			testutil.RespondJSON(w, http.StatusCreated, envJSON)
			return
		}
		http.NotFound(w, r)
	}))

	al := 30
	uid := int64(5)
	gid := int64(10)
	git := int64(0)

	_, err := Protect(context.Background(), client, ProtectInput{
		ProjectID: "42",
		Name:      "staging",
		DeployAccessLevels: []DeployAccessLevelInput{
			{UserID: &uid, GroupID: &gid, AccessLevel: &al, GroupInheritanceType: &git},
		},
		ApprovalRules: []ApprovalRuleInput{
			{UserID: &uid, GroupID: &gid, AccessLevel: &al, RequiredApprovalCount: new(int64(2)), GroupInheritanceType: &git},
		},
	})
	if err != nil {
		t.Fatalf("Protect() unexpected error: %v", err)
	}
	for _, want := range []string{"deploy_access_levels", "user_id", "group_id", "group_inheritance_type", "approval_rules", "required_approvals"} {
		if !strings.Contains(capturedBody, want) {
			t.Errorf("request body missing field %q", want)
		}
	}
}

// TestGet_CancelledContext validates that Get returns an error when the
// context is cancelled before the API call.
func TestGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, envJSON)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Get(ctx, client, GetInput{ProjectID: "42", Environment: "prod"})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// TestProtect_CancelledContext validates that Protect returns an error when the
// context is cancelled before the API call.
func TestProtect_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, envJSON)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Protect(ctx, client, ProtectInput{ProjectID: "42", Name: "prod"})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// TestUnprotect_CancelledContext validates that Unprotect returns an error when
// the context is cancelled before the API call.
func TestUnprotect_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	ctx := testutil.CancelledCtx(t)

	err := Unprotect(ctx, client, UnprotectInput{ProjectID: "42", Environment: "prod"})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// TestMCPRoundTrip_ErrorAndNotFound validates register.go error and NotFound
// paths via MCP round-trip.
func TestMCPRoundTrip_ErrorAndNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404 Not Found"}`))
	})

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, mux)
	RegisterTools(server, client)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	t.Run("get_404", func(t *testing.T) {
		res, callErr := session.CallTool(ctx, &mcp.CallToolParams{
			Name:      "gitlab_protected_environment_get",
			Arguments: map[string]any{"project_id": "42", "environment": "prod"},
		})
		if callErr != nil {
			t.Fatalf("CallTool: %v", callErr)
		}
		if !res.IsError {
			t.Error("expected IsError=true for get 404")
		}
	})

	t.Run("unprotect_404", func(t *testing.T) {
		res, callErr := session.CallTool(ctx, &mcp.CallToolParams{
			Name:      "gitlab_protected_environment_unprotect",
			Arguments: map[string]any{"project_id": "42", "environment": "prod"},
		})
		if callErr != nil {
			t.Fatalf("CallTool: %v", callErr)
		}
		if !res.IsError {
			t.Error("expected IsError=true for unprotect 404")
		}
	})
}

// TestProtect_MissingName verifies the behavior of protect missing name.
func TestProtect_MissingName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Protect(context.Background(), client, ProtectInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("Protect() expected error for missing name")
	}
}

// ---------- Update ----------.

// TestUpdate_Success verifies the behavior of update success.
func TestUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathProtectedEnv1 {
			testutil.RespondJSON(w, http.StatusOK, envJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID:   "42",
		Environment: "production",
	})
	if err != nil {
		t.Fatalf("Update() unexpected error: %v", err)
	}
	if out.Name != "production" {
		t.Errorf("Name = %q, want %q", out.Name, "production")
	}
}

// TestUpdate_MissingProjectID verifies the behavior of update missing project i d.
func TestUpdate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Update(context.Background(), client, UpdateInput{Environment: "production"})
	if err == nil {
		t.Fatal("Update() expected error for missing project_id")
	}
}

// TestUpdate_MissingEnvironment verifies the behavior of update missing environment.
func TestUpdate_MissingEnvironment(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("Update() expected error for missing environment")
	}
}

// ---------- Unprotect ----------.

// TestUnprotect_Success verifies the behavior of unprotect success.
func TestUnprotect_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathProtectedEnv1 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := Unprotect(context.Background(), client, UnprotectInput{ProjectID: "42", Environment: "production"})
	if err != nil {
		t.Fatalf("Unprotect() unexpected error: %v", err)
	}
}

// TestUnprotect_MissingProjectID verifies the behavior of unprotect missing project i d.
func TestUnprotect_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	err := Unprotect(context.Background(), client, UnprotectInput{Environment: "production"})
	if err == nil {
		t.Fatal("Unprotect() expected error for missing project_id")
	}
}

// TestUnprotect_MissingEnvironment verifies the behavior of unprotect missing environment.
func TestUnprotect_MissingEnvironment(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	err := Unprotect(context.Background(), client, UnprotectInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("Unprotect() expected error for missing environment")
	}
}

// ---------- Formatters ----------.

// TestFormatOutputMarkdown verifies the behavior of format output markdown.
func TestFormatOutputMarkdown(t *testing.T) {
	pe := Output{
		Name:                  "production",
		RequiredApprovalCount: 2,
		DeployAccessLevels: []AccessLevelOutput{
			{ID: 1, AccessLevel: 40, AccessLevelDescription: "Maintainers"},
		},
		ApprovalRules: []ApprovalRuleOutput{
			{ID: 10, AccessLevel: 40, AccessLevelDescription: "Maintainers", RequiredApprovalCount: 1},
		},
	}
	md := FormatOutputMarkdown(pe)
	if !strings.Contains(md, "production") {
		t.Error("expected environment name in output")
	}
	if !strings.Contains(md, "Maintainers") {
		t.Error("expected access level description in output")
	}
	if !strings.Contains(md, "Deploy Access Levels") {
		t.Error("expected deploy access levels section")
	}
	if !strings.Contains(md, "Approval Rules") {
		t.Error("expected approval rules section")
	}
}

// TestFormatOutputMarkdown_Empty verifies the behavior of format output markdown empty.
func TestFormatOutputMarkdown_Empty(t *testing.T) {
	md := FormatOutputMarkdown(Output{})
	if md != "" {
		t.Errorf("FormatOutputMarkdown(empty) = %q, want empty", md)
	}
}

// TestFormatListMarkdown verifies the behavior of format list markdown.
func TestFormatListMarkdown(t *testing.T) {
	out := ListOutput{
		Environments: []Output{
			{Name: "production", RequiredApprovalCount: 2, DeployAccessLevels: []AccessLevelOutput{{ID: 1}}},
			{Name: "staging", RequiredApprovalCount: 0},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, TotalPages: 1, Page: 1, PerPage: 20},
	}
	md := FormatListMarkdown(out)
	if !strings.Contains(md, "production") {
		t.Error("expected production in list output")
	}
	if !strings.Contains(md, "staging") {
		t.Error("expected staging in list output")
	}
	if !strings.Contains(md, "Protected Environments (2)") {
		t.Error("expected header with count")
	}
}

// TestFormatListMarkdown_Empty verifies the behavior of format list markdown empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if !strings.Contains(md, "No protected environments found") {
		t.Errorf("FormatListMarkdown(empty) = %q, want no-results message", md)
	}
}

// TestUpdate_WithAllFields verifies Update maps all optional fields including
// DeployAccessLevels and ApprovalRules with all sub-fields populated.
func TestUpdate_WithAllFields(t *testing.T) {
	var capturedBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathProtectedEnv1 {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			capturedBody = string(body)
			testutil.RespondJSON(w, http.StatusOK, envJSON)
			return
		}
		http.NotFound(w, r)
	}))

	reqApproval := int64(2)
	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID:             "42",
		Environment:           "production",
		Name:                  "staging",
		RequiredApprovalCount: &reqApproval,
		DeployAccessLevels: []UpdateDeployAccessLevelInput{
			{
				ID:                   new(int64(1)),
				AccessLevel:          new(30),
				UserID:               new(int64(10)),
				GroupID:              new(int64(20)),
				GroupInheritanceType: new(int64(1)),
				Destroy:              new(false),
			},
		},
		ApprovalRules: []UpdateApprovalRuleInput{
			{
				ID:                    new(int64(2)),
				AccessLevel:           new(40),
				UserID:                new(int64(11)),
				GroupID:               new(int64(21)),
				RequiredApprovalCount: new(int64(1)),
				GroupInheritanceType:  new(int64(0)),
				Destroy:               new(true),
			},
		},
	})
	if err != nil {
		t.Fatalf("Update() unexpected error: %v", err)
	}
	if out.Name != "production" {
		t.Errorf("Name = %q, want %q", out.Name, "production")
	}
	for _, want := range []string{"deploy_access_levels", "user_id", "group_id", "group_inheritance_type", "approval_rules", "required_approvals", "_destroy"} {
		if !strings.Contains(capturedBody, want) {
			t.Errorf("request body missing field %q", want)
		}
	}
}

// TestUpdate_APIError verifies Update returns error on API failure.
func TestUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	_, err := Update(context.Background(), client, UpdateInput{
		ProjectID:   "42",
		Environment: "production",
	})
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

// TestUpdate_CancelledContext verifies Update respects context cancellation.
func TestUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, envJSON)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Update(ctx, client, UpdateInput{
		ProjectID:   "42",
		Environment: "production",
	})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// TestList_APIError verifies List returns error on API failure.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

// TestList_CancelledContext verifies List respects context cancellation.
func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// TestGet_APIError verifies Get returns error on API failure.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: "42", Environment: "production"})
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

// TestProtect_MissingProjectID verifies Protect returns error for empty project_id.
func TestProtect_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := Protect(context.Background(), client, ProtectInput{Name: "staging"})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestProtect_APIError verifies Protect returns error on API failure.
func TestProtect_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := Protect(context.Background(), client, ProtectInput{ProjectID: "42", Name: "staging"})
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

// TestUnprotect_APIError verifies Unprotect returns error on API failure.
func TestUnprotect_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	err := Unprotect(context.Background(), client, UnprotectInput{ProjectID: "42", Environment: "production"})
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

// TestRegisterTools_NoPanic verifies that RegisterTools does not panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// TestRegisterMeta_NoPanic verifies that RegisterMeta does not panic.
func TestRegisterMeta_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)
}

// TestRegisterTools_CallThroughMCP verifies all registered tools can be called
// through MCP in-memory transport, covering the handler closures.
func TestRegisterTools_CallThroughMCP(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			testutil.RespondJSON(w, http.StatusOK,
				`[{"name":"production","deploy_access_levels":[{"access_level":40}],"approval_rules":[]}]`)
		case http.MethodPost:
			testutil.RespondJSON(w, http.StatusCreated, envJSON)
		case http.MethodPut:
			testutil.RespondJSON(w, http.StatusOK, envJSON)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
	client := testutil.NewTestClient(t, mux)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_protected_environment_list", map[string]any{"project_id": "42"}},
		{"gitlab_protected_environment_get", map[string]any{"project_id": "42", "environment": "production"}},
		{"gitlab_protected_environment_protect", map[string]any{"project_id": "42", "name": "staging"}},
		{"gitlab_protected_environment_update", map[string]any{"project_id": "42", "environment": "production"}},
		{"gitlab_protected_environment_unprotect", map[string]any{"project_id": "42", "environment": "production"}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			var result *mcp.CallToolResult
			result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("CallTool(%s) returned nil", tt.name)
			}
		})
	}
}

// TestProtect_WithRequiredApprovalCount verifies that Protect forwards the
// required_approval_count value to the GitLab API when input.RequiredApprovalCount
// is non-nil. This targets the optional-field branch at the top of Protect.
func TestProtect_WithRequiredApprovalCount(t *testing.T) {
	var capturedBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProtectedEnvs {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			capturedBody = string(body)
			testutil.RespondJSON(w, http.StatusCreated, envJSON)
			return
		}
		http.NotFound(w, r)
	}))
	count := int64(3)
	_, err := Protect(context.Background(), client, ProtectInput{
		ProjectID:             "42",
		Name:                  "production",
		RequiredApprovalCount: &count,
	})
	if err != nil {
		t.Fatalf("Protect() unexpected error: %v", err)
	}
	if !strings.Contains(capturedBody, "required_approval_count") {
		t.Errorf("request body missing required_approval_count; body=%q", capturedBody)
	}
}
