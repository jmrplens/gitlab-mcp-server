package groupldap

import (
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_Metadata verifies canonical metadata for group LDAP actions.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)

	if len(specs) != 4 {
		t.Fatalf("len(ActionSpecs) = %d, want 4", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "groupldap" {
			t.Errorf("OwnerPackage for %s = %q, want groupldap", spec.Name, spec.OwnerPackage)
		}
		if spec.IndividualTool.Name == "" {
			t.Errorf("IndividualTool.Name for %s is empty", spec.Name)
		}
	}

	byTool := groupLDAPSpecsByTool(t, specs)
	if !byTool["gitlab_group_ldap_link_list"].ReadOnly {
		t.Error("list action should be read-only")
	}
	for _, name := range []string{"gitlab_group_ldap_link_delete", "gitlab_group_ldap_link_delete_for_provider"} {
		spec := byTool[name]
		if !spec.Destructive || !spec.Route.Destructive {
			t.Errorf("%s should be destructive", name)
		}
		if !spec.Idempotent {
			t.Errorf("%s should be idempotent", name)
		}
	}
}

// TestActionSpecs_CallRoutes verifies all group LDAP routes execute through the catalog.
func TestActionSpecs_CallRoutes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			testutil.RespondJSON(w, http.StatusOK, `[{"cn":"admin-group","group_access":40,"provider":"ldapmain","filter":""}]`)
		case http.MethodPost:
			testutil.RespondJSON(w, http.StatusCreated, `{"cn":"new-group","group_access":30,"provider":"ldapmain","filter":""}`)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
	client := testutil.NewTestClient(t, mux)
	byTool := groupLDAPSpecsByTool(t, ActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_group_ldap_link_list", map[string]any{"group_id": "42"}},
		{"gitlab_group_ldap_link_add", map[string]any{"group_id": "42", "cn": "new-group", "provider": "ldapmain", "group_access": 30}},
		{"gitlab_group_ldap_link_delete", map[string]any{"group_id": "42", "cn": "admin-group", "provider": "ldapmain"}},
		{"gitlab_group_ldap_link_delete_for_provider", map[string]any{"group_id": "42", "provider": "ldapmain", "cn": "admin-group"}},
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

// TestActionSpecs_CallRouteErrors verifies delete route errors propagate directly.
func TestActionSpecs_CallRouteErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	})
	client := testutil.NewTestClient(t, mux)
	byTool := groupLDAPSpecsByTool(t, ActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_group_ldap_link_delete", map[string]any{"group_id": "42", "cn": "grp", "provider": "ldapmain"}},
		{"gitlab_group_ldap_link_delete_for_provider", map[string]any{"group_id": "42", "provider": "ldapmain", "cn": "grp"}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := byTool[tt.name].Route.Handler(t.Context(), tt.args)
			if err == nil {
				t.Fatalf("Route.Handler(%s) expected error, got nil", tt.name)
			}
			if result != nil {
				t.Errorf("Route.Handler(%s) result = %#v, want nil", tt.name, result)
			}
		})
	}
}

func groupLDAPSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}
