package groupsaml

import (
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_Metadata verifies canonical metadata for group SAML actions.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)

	if len(specs) != 4 {
		t.Fatalf("len(ActionSpecs) = %d, want 4", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "groupsaml" {
			t.Errorf("OwnerPackage for %s = %q, want groupsaml", spec.Name, spec.OwnerPackage)
		}
		if spec.IndividualTool.Name == "" {
			t.Errorf("IndividualTool.Name for %s is empty", spec.Name)
		}
	}

	byTool := groupSAMLSpecsByTool(t, specs)
	for _, name := range []string{"gitlab_group_saml_link_list", "gitlab_group_saml_link_get"} {
		if !byTool[name].ReadOnly {
			t.Errorf("%s should be read-only", name)
		}
	}
	spec := byTool["gitlab_group_saml_link_delete"]
	if !spec.Destructive || !spec.Route.Destructive {
		t.Error("delete action should be destructive")
	}
	if !spec.Idempotent {
		t.Error("delete action should be idempotent")
	}
}

// TestActionSpecs_CallRoutes verifies all group SAML routes execute through the catalog.
func TestActionSpecs_CallRoutes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/groups/42/saml_group_links":
			testutil.RespondJSON(w, http.StatusOK, `[{"name":"saml-group","access_level":30,"member_role_id":null}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/groups/42/saml_group_links/saml-group":
			testutil.RespondJSON(w, http.StatusOK, `{"name":"saml-group","access_level":30,"member_role_id":null}`)
		case r.Method == http.MethodPost:
			testutil.RespondJSON(w, http.StatusCreated, `{"name":"new-saml","access_level":30,"member_role_id":null}`)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
	client := testutil.NewTestClient(t, mux)
	byTool := groupSAMLSpecsByTool(t, ActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_group_saml_link_list", map[string]any{"group_id": "42"}},
		{"gitlab_group_saml_link_get", map[string]any{"group_id": "42", "saml_group_name": "saml-group"}},
		{"gitlab_group_saml_link_add", map[string]any{"group_id": "42", "saml_group_name": "new-saml", "access_level": 30}},
		{"gitlab_group_saml_link_delete", map[string]any{"group_id": "42", "saml_group_name": "saml-group"}},
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

// TestActionSpecs_CallRouteError verifies delete route errors propagate directly.
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
	spec := groupSAMLSpecsByTool(t, ActionSpecs(client))["gitlab_group_saml_link_delete"]

	result, err := spec.Route.Handler(t.Context(), map[string]any{"group_id": "42", "saml_group_name": "bad"})
	if err == nil {
		t.Fatal("Route.Handler expected error, got nil")
	}
	if result != nil {
		t.Errorf("Route.Handler result = %#v, want nil", result)
	}
}

func groupSAMLSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}
