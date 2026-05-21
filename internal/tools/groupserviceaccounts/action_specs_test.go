package groupserviceaccounts

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	registerAccountJSON  = `{"id":1,"name":"svc","username":"svc-user","email":"svc@test.com"}`
	registerAccountsJSON = `[{"id":1,"name":"svc","username":"svc-user","email":"svc@test.com"}]`
	registerPATJSON      = `{"id":10,"name":"tok","scopes":["api"],"active":true,"revoked":false}`
	registerPATsJSON     = `[{"id":10,"name":"tok","scopes":["api"],"active":true,"revoked":false}]`
)

// TestActionSpecs_Metadata verifies canonical metadata for group service account actions.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)

	if len(specs) != 7 {
		t.Fatalf("len(ActionSpecs) = %d, want 7", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "groupserviceaccounts" {
			t.Errorf("OwnerPackage for %s = %q, want groupserviceaccounts", spec.Name, spec.OwnerPackage)
		}
		if spec.IndividualTool.Name == "" {
			t.Errorf("IndividualTool.Name for %s is empty", spec.Name)
		}
	}

	byTool := groupServiceAccountSpecsByTool(t, specs)
	for _, name := range []string{"gitlab_group_service_account_list", "gitlab_group_service_account_pat_list"} {
		if !byTool[name].ReadOnly {
			t.Errorf("%s should be read-only", name)
		}
	}
	for _, name := range []string{"gitlab_group_service_account_delete", "gitlab_group_service_account_pat_revoke"} {
		spec := byTool[name]
		if !spec.Destructive || !spec.Route.Destructive {
			t.Errorf("%s should be destructive", name)
		}
		if !spec.Idempotent {
			t.Errorf("%s should be idempotent", name)
		}
	}
}

// TestActionSpecs_CallRoutes verifies all group service account routes execute through the catalog.
func TestActionSpecs_CallRoutes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/service_accounts"):
			testutil.RespondJSON(w, http.StatusOK, registerAccountsJSON)
		case r.Method == http.MethodGet && strings.Contains(path, "/personal_access_tokens"):
			testutil.RespondJSON(w, http.StatusOK, registerPATsJSON)
		case r.Method == http.MethodPost && strings.Contains(path, "/personal_access_tokens"):
			testutil.RespondJSON(w, http.StatusCreated, registerPATJSON)
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/service_accounts"):
			testutil.RespondJSON(w, http.StatusCreated, registerAccountJSON)
		case r.Method == http.MethodPatch:
			testutil.RespondJSON(w, http.StatusOK, registerAccountJSON)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
	client := testutil.NewTestClient(t, mux)
	byTool := groupServiceAccountSpecsByTool(t, ActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_group_service_account_list", map[string]any{"group_id": "mygroup"}},
		{"gitlab_group_service_account_create", map[string]any{"group_id": "mygroup", "name": "svc", "username": "svc-user"}},
		{"gitlab_group_service_account_update", map[string]any{"group_id": "mygroup", "service_account_id": 42, "name": "svc2"}},
		{"gitlab_group_service_account_delete", map[string]any{"group_id": "mygroup", "service_account_id": 42}},
		{"gitlab_group_service_account_pat_list", map[string]any{"group_id": "mygroup", "service_account_id": 42}},
		{"gitlab_group_service_account_pat_create", map[string]any{"group_id": "mygroup", "service_account_id": 42, "name": "tok", "scopes": []any{"api"}}},
		{"gitlab_group_service_account_pat_revoke", map[string]any{"group_id": "mygroup", "service_account_id": 42, "token_id": 10}},
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
		w.WriteHeader(http.StatusOK)
	})
	client := testutil.NewTestClient(t, mux)
	byTool := groupServiceAccountSpecsByTool(t, ActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_group_service_account_delete", map[string]any{"group_id": "mygroup", "service_account_id": 42}},
		{"gitlab_group_service_account_pat_revoke", map[string]any{"group_id": "mygroup", "service_account_id": 42, "token_id": 10}},
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

func groupServiceAccountSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}
