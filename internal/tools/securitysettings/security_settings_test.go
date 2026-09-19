// security_settings_test.go contains unit tests for GitLab project security
// settings operations. Tests use httptest to mock the GitLab API.
package securitysettings

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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

// TestGetProject_Success verifies that GetProject returns the expected output when the GitLab API responds successfully.
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

// TestGetProject_MissingProjectID verifies that GetProject returns a validation error when project_id is missing.
func TestGetProject_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := GetProject(context.Background(), client, GetProjectInput{})
	if err == nil {
		t.Fatal("expected error for empty project_id, got nil")
	}
}

// TestGetProject_CancelledContext verifies that GetProject returns an error when the context is already cancelled.
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

// TestGetProject_APIError verifies that GetProject returns an error when the GitLab API responds with a failure status.
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

// TestUpdateProject_Success verifies that UpdateProject returns the expected output when the GitLab API responds successfully.
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

// TestUpdateProject_MissingProjectID verifies that UpdateProject returns a validation error when project_id is missing.
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

// TestUpdateProject_CancelledContext verifies that UpdateProject returns an error when the context is already cancelled.
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

// TestUpdateProject_APIError verifies that UpdateProject returns an error when the GitLab API responds with a failure status.
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

// TestUpdateGroup_Success verifies that UpdateGroup returns the expected output when the GitLab API responds successfully.
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

// TestUpdateGroup_WithExclusions verifies that UpdateGroup forwards the exclusions parameters to the GitLab API.
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

// TestUpdateGroup_MissingGroupID verifies that UpdateGroup returns a validation error when group_id is missing.
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

// TestUpdateGroup_CancelledContext verifies that UpdateGroup returns an error when the context is already cancelled.
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

// TestUpdateGroup_APIError verifies that UpdateGroup returns an error when the GitLab API responds with a failure status.
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

// TestUpdateProject_RequestBody_CarriesTheValueTheCallerAskedFor validates
// that the PUT body sends secret_push_protection_enabled as the caller set it,
// both when switching protection on and when switching it off.
//
// The response is what GitLab now holds rather than what we asked for, so a
// flag that arrived inverted or never arrived at all would leave every
// assertion about the output passing while the project was left carrying the
// opposite of the request.
func TestUpdateProject_RequestBody_CarriesTheValueTheCallerAskedFor(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{name: "enable", want: true},
		{name: "disable", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPut)
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decoding the request body: %v", err)
				}
				if got, sent := body["secret_push_protection_enabled"]; !sent || got != tc.want {
					t.Errorf("secret_push_protection_enabled sent as %v (present %t), want %t (body %v)", got, sent, tc.want, body)
				}
				testutil.RespondJSON(w, http.StatusOK, projectSecurityJSON)
			}))

			if _, err := UpdateProject(context.Background(), client, UpdateProjectInput{
				ProjectID:                   toolutil.StringOrInt("42"),
				SecretPushProtectionEnabled: tc.want,
			}); err != nil {
				t.Fatalf("UpdateProject() error: %v", err)
			}
		})
	}
}

// TestUpdateGroup_RequestBody_ExcludesOnlyTheProjectsTheCallerNamed validates
// that projects_to_exclude reaches GitLab exactly when the caller supplied
// ids, carries them as given, and that the flag beside it is the caller's own.
//
// GitLab answers the same body whether the exclusion list was sent or not, so
// the guard around it cannot be seen in the output: a list dropped on the way
// out would enforce protection on the very projects the caller asked to leave
// alone, and an empty one sent in its place is a request we never meant to
// make.
func TestUpdateGroup_RequestBody_ExcludesOnlyTheProjectsTheCallerNamed(t *testing.T) {
	cases := []struct {
		name     string
		exclude  []int64
		enabled  bool
		wantSent bool
		want     []any
	}{
		{name: "none named", exclude: nil, enabled: true},
		{name: "empty list", exclude: []int64{}, enabled: false},
		{name: "two named", exclude: []int64{7, 11}, enabled: true, wantSent: true, want: []any{float64(7), float64(11)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPut)
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decoding the request body: %v", err)
				}
				if got, sent := body["secret_push_protection_enabled"]; !sent || got != tc.enabled {
					t.Errorf("secret_push_protection_enabled sent as %v (present %t), want %t (body %v)", got, sent, tc.enabled, body)
				}
				got, sent := body["projects_to_exclude"]
				switch {
				case sent != tc.wantSent:
					t.Errorf("projects_to_exclude present in the request = %t, want %t (body %v)", sent, tc.wantSent, body)
				case sent && !reflect.DeepEqual(got, tc.want):
					t.Errorf("projects_to_exclude sent as %v, want %v", got, tc.want)
				}
				testutil.RespondJSON(w, http.StatusOK, `{"secret_push_protection_enabled":true}`)
			}))

			if _, err := UpdateGroup(context.Background(), client, UpdateGroupInput{
				GroupID:                     toolutil.StringOrInt("mygroup"),
				SecretPushProtectionEnabled: tc.enabled,
				ProjectsToExclude:           tc.exclude,
			}); err != nil {
				t.Fatalf("UpdateGroup() error: %v", err)
			}
		})
	}
}

// TestGetProject_OneFlagAtATime_EachSettingReadsItsOwnField validates that
// every boolean GitLab sends lands on the field named after it, by driving one
// flag at a time and comparing the whole output against one carrying only that
// field.
//
// A block of flags has no fixture where no two values agree, so a converter
// reading a neighbour's key passes any response whose flags happen to match;
// the two fixtures above set auto_fix_dast and auto_fix_sast alike, and
// continuous scans alongside secret push protection, so four of the seven
// could be swapped in pairs with nothing failing.
func TestGetProject_OneFlagAtATime_EachSettingReadsItsOwnField(t *testing.T) {
	cases := []struct {
		key  string
		want ProjectOutput
	}{
		{key: "auto_fix_container_scanning", want: ProjectOutput{ProjectID: 7, AutoFixContainerScanning: true}},
		{key: "auto_fix_dast", want: ProjectOutput{ProjectID: 7, AutoFixDAST: true}},
		{key: "auto_fix_dependency_scanning", want: ProjectOutput{ProjectID: 7, AutoFixDependencyScanning: true}},
		{key: "auto_fix_sast", want: ProjectOutput{ProjectID: 7, AutoFixSAST: true}},
		{key: "continuous_vulnerability_scans_enabled", want: ProjectOutput{ProjectID: 7, ContinuousVulnerabilityScansEnabled: true}},
		{key: "container_scanning_for_registry_enabled", want: ProjectOutput{ProjectID: 7, ContainerScanningForRegistryEnabled: true}},
		{key: "secret_push_protection_enabled", want: ProjectOutput{ProjectID: 7, SecretPushProtectionEnabled: true}},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			body := `{"project_id":7,"` + tc.key + `":true}`
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestPath(t, r, "/api/v4/projects/7/security_settings")
				testutil.RespondJSON(w, http.StatusOK, body)
			}))

			out, err := GetProject(context.Background(), client, GetProjectInput{
				ProjectID: toolutil.StringOrInt("7"),
			})
			if err != nil {
				t.Fatalf("GetProject() error: %v", err)
			}
			if !reflect.DeepEqual(out, tc.want) {
				t.Errorf("GetProject() = %+v, want %+v", out, tc.want)
			}
		})
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
