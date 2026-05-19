package groupwikis

import (
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const registerWikiJSON = `{
	"format": "markdown",
	"slug": "home",
	"title": "Home",
	"content": "# Welcome",
	"encoding": "UTF-8"
}`

// TestActionSpecs_Metadata verifies canonical metadata for group wiki actions.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)

	if len(specs) != 5 {
		t.Fatalf("len(ActionSpecs) = %d, want 5", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "groupwikis" {
			t.Errorf("OwnerPackage for %s = %q, want groupwikis", spec.Name, spec.OwnerPackage)
		}
		if spec.IndividualTool.Name == "" {
			t.Errorf("IndividualTool.Name for %s is empty", spec.Name)
		}
	}

	byTool := groupWikiSpecsByTool(t, specs)
	for _, name := range []string{"gitlab_group_wiki_list", "gitlab_group_wiki_get"} {
		if !byTool[name].ReadOnly {
			t.Errorf("%s should be read-only", name)
		}
	}
	spec := byTool["gitlab_group_wiki_delete"]
	if !spec.Destructive || !spec.Route.Destructive {
		t.Error("delete action should be destructive")
	}
	if !spec.Idempotent {
		t.Error("delete action should be idempotent")
	}
}

// TestActionSpecs_CallRoutes verifies all group wiki routes execute through the catalog.
func TestActionSpecs_CallRoutes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/groups/42/wikis":
			testutil.RespondJSON(w, http.StatusOK, `[`+registerWikiJSON+`]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/groups/42/wikis/home":
			testutil.RespondJSON(w, http.StatusOK, registerWikiJSON)
		case r.Method == http.MethodPost:
			testutil.RespondJSON(w, http.StatusCreated, registerWikiJSON)
		case r.Method == http.MethodPut:
			testutil.RespondJSON(w, http.StatusOK, registerWikiJSON)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
	client := testutil.NewTestClient(t, mux)
	byTool := groupWikiSpecsByTool(t, ActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_group_wiki_list", map[string]any{"group_id": "42"}},
		{"gitlab_group_wiki_get", map[string]any{"group_id": "42", "slug": "home"}},
		{"gitlab_group_wiki_create", map[string]any{"group_id": "42", "title": "Home", "content": "# Welcome"}},
		{"gitlab_group_wiki_edit", map[string]any{"group_id": "42", "slug": "home", "content": "# Updated"}},
		{"gitlab_group_wiki_delete", map[string]any{"group_id": "42", "slug": "home"}},
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
	spec := groupWikiSpecsByTool(t, ActionSpecs(client))["gitlab_group_wiki_delete"]

	result, err := spec.Route.Handler(t.Context(), map[string]any{"group_id": "42", "slug": "home"})
	if err == nil {
		t.Fatal("Route.Handler expected error, got nil")
	}
	if result != nil {
		t.Errorf("Route.Handler result = %#v, want nil", result)
	}
}

func groupWikiSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}
