package groupprotectedbranches

import (
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const registerBranchJSON = `{
	"id": 1,
	"name": "main",
	"push_access_levels": [{"access_level": 40, "access_level_description": "Maintainers"}],
	"merge_access_levels": [{"access_level": 40, "access_level_description": "Maintainers"}],
	"allow_force_push": false,
	"code_owner_approval_required": false
}`

// TestActionSpecs_Metadata verifies canonical metadata for group protected branch actions.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)

	if len(specs) != 5 {
		t.Fatalf("len(ActionSpecs) = %d, want 5", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "groupprotectedbranches" {
			t.Errorf("OwnerPackage for %s = %q, want groupprotectedbranches", spec.Name, spec.OwnerPackage)
		}
		if spec.IndividualTool.Name == "" {
			t.Errorf("IndividualTool.Name for %s is empty", spec.Name)
		}
	}

	byTool := groupProtectedBranchSpecsByTool(t, specs)
	for _, name := range []string{"gitlab_group_protected_branch_list", "gitlab_group_protected_branch_get"} {
		if !byTool[name].ReadOnly {
			t.Errorf("%s should be read-only", name)
		}
	}
	spec := byTool["gitlab_group_protected_branch_unprotect"]
	if !spec.Destructive || !spec.Route.Destructive {
		t.Error("unprotect action should be destructive")
	}
	if !spec.Idempotent {
		t.Error("unprotect action should be idempotent")
	}
}

// TestActionSpecs_CallRoutes verifies all group protected branch routes execute through the catalog.
func TestActionSpecs_CallRoutes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/groups/{gid}/protected_branches", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+registerBranchJSON+`]`)
	})
	mux.HandleFunc("GET /api/v4/groups/{gid}/protected_branches/{name}", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, registerBranchJSON)
	})
	mux.HandleFunc("POST /api/v4/groups/{gid}/protected_branches", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, registerBranchJSON)
	})
	mux.HandleFunc("PATCH /api/v4/groups/{gid}/protected_branches/{name}", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, registerBranchJSON)
	})
	mux.HandleFunc("DELETE /api/v4/groups/{gid}/protected_branches/{name}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)
	byTool := groupProtectedBranchSpecsByTool(t, ActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_group_protected_branch_list", map[string]any{"group_id": "42"}},
		{"gitlab_group_protected_branch_get", map[string]any{"group_id": "42", "branch": "main"}},
		{"gitlab_group_protected_branch_protect", map[string]any{"group_id": "42", "name": "main"}},
		{"gitlab_group_protected_branch_update", map[string]any{"group_id": "42", "branch": "main"}},
		{"gitlab_group_protected_branch_unprotect", map[string]any{"group_id": "42", "branch": "main"}},
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

// TestActionSpecs_CallRouteError verifies unprotect route errors propagate directly.
func TestActionSpecs_CallRouteError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	client := testutil.NewTestClient(t, mux)
	spec := groupProtectedBranchSpecsByTool(t, ActionSpecs(client))["gitlab_group_protected_branch_unprotect"]

	result, err := spec.Route.Handler(t.Context(), map[string]any{"group_id": "42", "branch": "main"})
	if err == nil {
		t.Fatal("Route.Handler expected error, got nil")
	}
	if result != nil {
		t.Errorf("Route.Handler result = %#v, want nil", result)
	}
}

func groupProtectedBranchSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}
