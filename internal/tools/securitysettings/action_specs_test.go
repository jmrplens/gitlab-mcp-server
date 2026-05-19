// action_specs_test.go contains integration tests for the security settings tool
// closures in ActionSpecs routes with a mock GitLab API.
package securitysettings

import (
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const registerProjectSecJSON = `{
	"project_id": 42,
	"auto_fix_container_scanning": false,
	"auto_fix_dast": false,
	"auto_fix_dependency_scanning": true,
	"auto_fix_sast": true,
	"continuous_vulnerability_scans_enabled": false,
	"container_scanning_for_registry_enabled": true,
	"secret_push_protection_enabled": true
}`

const registerGroupSecJSON = `{
	"secret_push_protection_enabled": true
}`

// TestActionSpecs_Metadata verifies security settings action spec metadata.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := append(ProjectActionSpecs(client), GroupActionSpecs(client)...)
	if len(specs) != 3 {
		t.Fatalf("len(ActionSpecs) = %d, want 3", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "securitysettings" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
	}
}

// TestActionSpecs_CallRoutes verifies all registered security settings routes execute successfully.
func TestActionSpecs_CallRoutes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			testutil.RespondJSON(w, http.StatusOK, registerProjectSecJSON)
		case http.MethodPatch, http.MethodPut:
			testutil.RespondJSON(w, http.StatusOK, registerGroupSecJSON)
		default:
			http.NotFound(w, r)
		}
	})
	client := testutil.NewTestClient(t, mux)
	specs := append(ProjectActionSpecs(client), GroupActionSpecs(client)...)
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_get_project_security_settings", map[string]any{"project_id": "42"}},
		{"gitlab_update_project_secret_push_protection", map[string]any{"project_id": "42", "secret_push_protection_enabled": true}},
		{"gitlab_update_group_secret_push_protection", map[string]any{"group_id": "my-group", "secret_push_protection_enabled": true}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := specByTool[tt.name]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tt.name)
			}
			result, err := spec.Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.name)
			}
		})
	}
}

// TestToProjectOutput_Nil verifies that toProjectOutput handles nil input
// gracefully by returning a zero-value ProjectOutput.
func TestToProjectOutput_Nil(t *testing.T) {
	out := toProjectOutput(nil)
	if out.ProjectID != 0 {
		t.Errorf("expected zero ProjectID for nil input, got %d", out.ProjectID)
	}
}

// TestToGroupOutput_Nil verifies that toGroupOutput handles nil input
// gracefully by returning a zero-value GroupOutput.
func TestToGroupOutput_Nil(t *testing.T) {
	out := toGroupOutput(nil)
	if out.SecretPushProtectionEnabled {
		t.Error("expected false SecretPushProtectionEnabled for nil input")
	}
	if len(out.Errors) != 0 {
		t.Errorf("expected empty errors for nil input, got %d", len(out.Errors))
	}
}
