package groupprotectedenvs

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	registerProtEnvJSON     = `{"name":"production","deploy_access_levels":[{"access_level":40}]}`
	registerProtEnvListJSON = `[{"name":"production","deploy_access_levels":[{"access_level":40}]}]`
)

// TestActionSpecs_Metadata verifies canonical metadata for group protected environment actions.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)

	if len(specs) != 5 {
		t.Fatalf("len(ActionSpecs) = %d, want 5", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "groupprotectedenvs" {
			t.Errorf("OwnerPackage for %s = %q, want groupprotectedenvs", spec.Name, spec.OwnerPackage)
		}
		if spec.IndividualTool.Name == "" {
			t.Errorf("IndividualTool.Name for %s is empty", spec.Name)
		}
	}

	byTool := groupProtectedEnvSpecsByTool(t, specs)
	for _, name := range []string{"gitlab_group_protected_environment_list", "gitlab_group_protected_environment_get"} {
		if !byTool[name].ReadOnly {
			t.Errorf("%s should be read-only", name)
		}
	}
	spec := byTool["gitlab_group_protected_environment_unprotect"]
	if !spec.Destructive || !spec.Route.Destructive {
		t.Error("unprotect action should be destructive")
	}
	if !spec.Idempotent {
		t.Error("unprotect action should be idempotent")
	}
}

// TestActionSpecs_ProtectRequiresDeployAccessLevels verifies discovery schemas
// advertise the access rule required to create a group protected environment.
func TestActionSpecs_ProtectRequiresDeployAccessLevels(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	byTool := groupProtectedEnvSpecsByTool(t, ActionSpecs(client))
	schema := byTool["gitlab_group_protected_environment_protect"].Route.InputSchema
	if !schemaRequiredIncludes(schema, "deploy_access_levels") {
		t.Fatalf("protect required fields = %v, want deploy_access_levels", schema["required"])
	}
}

// TestActionSpecs_CallRoutes verifies all group protected environment routes execute through the catalog.
func TestActionSpecs_CallRoutes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/protected_environments"):
			testutil.RespondJSON(w, http.StatusOK, registerProtEnvListJSON)
		case r.Method == http.MethodGet:
			testutil.RespondJSON(w, http.StatusOK, registerProtEnvJSON)
		case r.Method == http.MethodPost:
			testutil.RespondJSON(w, http.StatusCreated, registerProtEnvJSON)
		case r.Method == http.MethodPut:
			testutil.RespondJSON(w, http.StatusOK, registerProtEnvJSON)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
	client := testutil.NewTestClient(t, mux)
	byTool := groupProtectedEnvSpecsByTool(t, ActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_group_protected_environment_list", map[string]any{"group_id": "mygroup"}},
		{"gitlab_group_protected_environment_get", map[string]any{"group_id": "mygroup", "environment": "production"}},
		{"gitlab_group_protected_environment_protect", map[string]any{"group_id": "mygroup", "name": "staging", "deploy_access_levels": []any{map[string]any{"access_level": 40}}}},
		{"gitlab_group_protected_environment_update", map[string]any{"group_id": "mygroup", "environment": "production", "deploy_access_levels": []any{map[string]any{"access_level": 30}}}},
		{"gitlab_group_protected_environment_unprotect", map[string]any{"group_id": "mygroup", "environment": "production"}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := byTool[tt.name].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.name)
			}
		})
	}
}

func groupProtectedEnvSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}

func schemaRequiredIncludes(schema map[string]any, name string) bool {
	switch required := schema["required"].(type) {
	case []any:
		for _, raw := range required {
			if field, ok := raw.(string); ok && field == name {
				return true
			}
		}
	case []string:
		return slices.Contains(required, name)
	}
	return false
}
