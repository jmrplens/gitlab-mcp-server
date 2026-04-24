// security_settings_test.go contains unit tests for GitLab project security
// settings operations. Tests use httptest to mock the GitLab API.
package securitysettings

import (
	"context"
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const projectSecurityJSON = `{
	"project_id":42,
	"created_at":"2026-01-01T00:00:00Z",
	"updated_at":"2026-01-02T00:00:00Z",
	"auto_fix_container_scanning":true,
	"auto_fix_dast":false,
	"auto_fix_dependency_scanning":true,
	"auto_fix_sast":false,
	"continuous_vulnerability_scans_enabled":true,
	"container_scanning_for_registry_enabled":false,
	"secret_push_protection_enabled":true
}`

func TestGetProject_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/security_settings" {
			testutil.RespondJSON(w, http.StatusOK, projectSecurityJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetProject(context.Background(), client, GetProjectInput{
		ProjectID: toolutil.StringOrInt("42"),
	})
	if err != nil {
		t.Fatalf("GetProject() error: %v", err)
	}
	if out.ProjectID != 42 {
		t.Errorf("expected project_id 42, got %d", out.ProjectID)
	}
	if !out.SecretPushProtectionEnabled {
		t.Error("expected secret_push_protection_enabled to be true")
	}
	if !out.AutoFixContainerScanning {
		t.Error("expected auto_fix_container_scanning to be true")
	}
}

func TestGetProject_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := GetProject(context.Background(), client, GetProjectInput{})
	if err == nil {
		t.Fatal("expected error for empty project_id, got nil")
	}
}

func TestGetProject_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := GetProject(ctx, client, GetProjectInput{ProjectID: toolutil.StringOrInt("42")})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestGetProject_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/security_settings" {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := GetProject(context.Background(), client, GetProjectInput{
		ProjectID: toolutil.StringOrInt("42"),
	})
	if err == nil {
		t.Fatal("expected error for 403 response, got nil")
	}
}

func TestUpdateProject_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/projects/42/security_settings" {
			testutil.RespondJSON(w, http.StatusOK, projectSecurityJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := UpdateProject(context.Background(), client, UpdateProjectInput{
		ProjectID:                   toolutil.StringOrInt("42"),
		SecretPushProtectionEnabled: true,
	})
	if err != nil {
		t.Fatalf("UpdateProject() error: %v", err)
	}
	if out.ProjectID != 42 {
		t.Errorf("expected project_id 42, got %d", out.ProjectID)
	}
}

func TestUpdateProject_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := UpdateProject(context.Background(), client, UpdateProjectInput{
		SecretPushProtectionEnabled: true,
	})
	if err == nil {
		t.Fatal("expected error for empty project_id, got nil")
	}
}

func TestUpdateProject_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := UpdateProject(ctx, client, UpdateProjectInput{
		ProjectID:                   toolutil.StringOrInt("42"),
		SecretPushProtectionEnabled: true,
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestUpdateProject_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/security_settings" {
			testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"Bad Request"}`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := UpdateProject(context.Background(), client, UpdateProjectInput{
		ProjectID:                   toolutil.StringOrInt("42"),
		SecretPushProtectionEnabled: true,
	})
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

func TestUpdateGroup_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/groups/mygroup/security_settings" {
			testutil.RespondJSON(w, http.StatusOK, `{"secret_push_protection_enabled":true}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := UpdateGroup(context.Background(), client, UpdateGroupInput{
		GroupID:                     toolutil.StringOrInt("mygroup"),
		SecretPushProtectionEnabled: true,
	})
	if err != nil {
		t.Fatalf("UpdateGroup() error: %v", err)
	}
	if !out.SecretPushProtectionEnabled {
		t.Error("expected secret_push_protection_enabled to be true")
	}
}

func TestUpdateGroup_WithExclusions(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/groups/mygroup/security_settings" {
			testutil.RespondJSON(w, http.StatusOK, `{"secret_push_protection_enabled":true}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := UpdateGroup(context.Background(), client, UpdateGroupInput{
		GroupID:                     toolutil.StringOrInt("mygroup"),
		SecretPushProtectionEnabled: true,
		ProjectsToExclude:           []int64{1, 2, 3},
	})
	if err != nil {
		t.Fatalf("UpdateGroup() error: %v", err)
	}
	if !out.SecretPushProtectionEnabled {
		t.Error("expected secret_push_protection_enabled to be true")
	}
}

func TestUpdateGroup_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := UpdateGroup(context.Background(), client, UpdateGroupInput{
		SecretPushProtectionEnabled: true,
	})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

func TestUpdateGroup_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := UpdateGroup(ctx, client, UpdateGroupInput{
		GroupID:                     toolutil.StringOrInt("mygroup"),
		SecretPushProtectionEnabled: true,
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestUpdateGroup_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/mygroup/security_settings" {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := UpdateGroup(context.Background(), client, UpdateGroupInput{
		GroupID:                     toolutil.StringOrInt("mygroup"),
		SecretPushProtectionEnabled: true,
	})
	if err == nil {
		t.Fatal("expected error for 403 response, got nil")
	}
}

// TestGetProject_NoDates validates that toProjectOutput handles a response
// where created_at and updated_at are absent (nil time pointers).
func TestGetProject_NoDates(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/security_settings" {
			testutil.RespondJSON(w, http.StatusOK, `{
				"project_id":42,
				"secret_push_protection_enabled":false
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetProject(context.Background(), client, GetProjectInput{
		ProjectID: toolutil.StringOrInt("42"),
	})
	if err != nil {
		t.Fatalf("GetProject() error: %v", err)
	}
	if out.ProjectID != 42 {
		t.Errorf("expected project_id 42, got %d", out.ProjectID)
	}
	if out.CreatedAt != "" {
		t.Errorf("expected empty CreatedAt, got %q", out.CreatedAt)
	}
	if out.UpdatedAt != "" {
		t.Errorf("expected empty UpdatedAt, got %q", out.UpdatedAt)
	}
	if out.SecretPushProtectionEnabled {
		t.Error("expected secret_push_protection_enabled to be false")
	}
}

// TestUpdateGroup_WithErrors validates that UpdateGroup correctly parses
// group security settings responses that include an errors array.
func TestUpdateGroup_WithErrors(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/groups/99/security_settings" {
			testutil.RespondJSON(w, http.StatusOK, `{
				"secret_push_protection_enabled":true,
				"errors":["project 5 not eligible","project 8 archived"]
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := UpdateGroup(context.Background(), client, UpdateGroupInput{
		GroupID:                     toolutil.StringOrInt("99"),
		SecretPushProtectionEnabled: true,
	})
	if err != nil {
		t.Fatalf("UpdateGroup() error: %v", err)
	}
	if !out.SecretPushProtectionEnabled {
		t.Error("expected secret_push_protection_enabled to be true")
	}
	if len(out.Errors) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(out.Errors))
	}
	if out.Errors[0] != "project 5 not eligible" {
		t.Errorf("expected first error %q, got %q", "project 5 not eligible", out.Errors[0])
	}
}

// TestGetProject_SuccessAllFields validates that all project security
// fields (auto-fix, scanning, dates) are correctly mapped from the API response.
func TestGetProject_SuccessAllFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/10/security_settings" {
			testutil.RespondJSON(w, http.StatusOK, `{
				"project_id":10,
				"created_at":"2026-06-01T10:00:00Z",
				"updated_at":"2026-06-15T14:30:00Z",
				"auto_fix_container_scanning":false,
				"auto_fix_dast":true,
				"auto_fix_dependency_scanning":false,
				"auto_fix_sast":true,
				"continuous_vulnerability_scans_enabled":false,
				"container_scanning_for_registry_enabled":true,
				"secret_push_protection_enabled":false
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetProject(context.Background(), client, GetProjectInput{
		ProjectID: toolutil.StringOrInt("10"),
	})
	if err != nil {
		t.Fatalf("GetProject() error: %v", err)
	}
	if out.ProjectID != 10 {
		t.Errorf("expected project_id 10, got %d", out.ProjectID)
	}
	if out.CreatedAt != "2026-06-01T10:00:00Z" {
		t.Errorf("CreatedAt = %q, want %q", out.CreatedAt, "2026-06-01T10:00:00Z")
	}
	if out.UpdatedAt != "2026-06-15T14:30:00Z" {
		t.Errorf("UpdatedAt = %q, want %q", out.UpdatedAt, "2026-06-15T14:30:00Z")
	}
	if !out.AutoFixDAST {
		t.Error("expected auto_fix_dast true")
	}
	if out.AutoFixContainerScanning {
		t.Error("expected auto_fix_container_scanning false")
	}
	if !out.AutoFixSAST {
		t.Error("expected auto_fix_sast true")
	}
	if out.AutoFixDependencyScanning {
		t.Error("expected auto_fix_dependency_scanning false")
	}
	if out.ContinuousVulnerabilityScansEnabled {
		t.Error("expected continuous_vulnerability_scans_enabled false")
	}
	if !out.ContainerScanningForRegistryEnabled {
		t.Error("expected container_scanning_for_registry_enabled true")
	}
	if out.SecretPushProtectionEnabled {
		t.Error("expected secret_push_protection_enabled false")
	}
}

// TestGetProject_NotFound validates that a 404 API response returns an error.
func TestGetProject_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Project Not Found"}`)
	}))

	_, err := GetProject(context.Background(), client, GetProjectInput{
		ProjectID: toolutil.StringOrInt("999"),
	})
	if err == nil {
		t.Fatal("expected error for 404 response, got nil")
	}
}

// TestUpdateProject_NotFound validates that a 404 API response returns an error.
func TestUpdateProject_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Project Not Found"}`)
	}))

	_, err := UpdateProject(context.Background(), client, UpdateProjectInput{
		ProjectID:                   toolutil.StringOrInt("999"),
		SecretPushProtectionEnabled: true,
	})
	if err == nil {
		t.Fatal("expected error for 404 response, got nil")
	}
}

// TestUpdateGroup_NotFound validates that a 404 API response returns an error.
func TestUpdateGroup_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Group Not Found"}`)
	}))

	_, err := UpdateGroup(context.Background(), client, UpdateGroupInput{
		GroupID:                     toolutil.StringOrInt("nonexistent"),
		SecretPushProtectionEnabled: true,
	})
	if err == nil {
		t.Fatal("expected error for 404 response, got nil")
	}
}

// TestUpdateProject_DisableProtection validates that secret push protection
// can be disabled (false value) and the response reflects the new state.
func TestUpdateProject_DisableProtection(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodPut)
		testutil.AssertRequestPath(t, r, "/api/v4/projects/42/security_settings")
		testutil.RespondJSON(w, http.StatusOK, `{
			"project_id":42,
			"secret_push_protection_enabled":false
		}`)
	}))

	out, err := UpdateProject(context.Background(), client, UpdateProjectInput{
		ProjectID:                   toolutil.StringOrInt("42"),
		SecretPushProtectionEnabled: false,
	})
	if err != nil {
		t.Fatalf("UpdateProject() error: %v", err)
	}
	if out.SecretPushProtectionEnabled {
		t.Error("expected secret_push_protection_enabled to be false after disabling")
	}
}

// TestUpdateGroup_EmptyExclusions validates that UpdateGroup works correctly
// when ProjectsToExclude is explicitly empty (should not set the field).
func TestUpdateGroup_EmptyExclusions(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodPut)
		testutil.RespondJSON(w, http.StatusOK, `{"secret_push_protection_enabled":false}`)
	}))

	out, err := UpdateGroup(context.Background(), client, UpdateGroupInput{
		GroupID:                     toolutil.StringOrInt("5"),
		SecretPushProtectionEnabled: false,
		ProjectsToExclude:           []int64{},
	})
	if err != nil {
		t.Fatalf("UpdateGroup() error: %v", err)
	}
	if out.SecretPushProtectionEnabled {
		t.Error("expected secret_push_protection_enabled to be false")
	}
}
