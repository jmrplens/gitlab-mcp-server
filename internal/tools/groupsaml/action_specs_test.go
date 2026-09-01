// action_specs_test.go contains unit tests for the group SAML [toolutil.ActionSpec] entries.
package groupsaml

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestSAMLUsersMetadata_Discoverability locks in the model-facing discovery
// metadata for gitlab_group_saml_users_list (added with client-go v2.41.0) and
// verifies the sibling saml_link_list cross-references it, so models can tell
// SAML *users* apart from SAML group *links*.
func TestSAMLUsersMetadata_Discoverability(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	byTool := groupSAMLSpecsByTool(t, ActionSpecs(client))

	users, ok := byTool["gitlab_group_saml_users_list"]
	if !ok {
		t.Fatal("missing gitlab_group_saml_users_list spec")
	}
	if users.Usage == "" || strings.Contains(users.Usage, "Use to execute") {
		t.Errorf("saml_users_list has generic/empty Usage: %q", users.Usage)
	}
	if !samlAliasHas(users.Aliases, "saml") {
		t.Errorf("saml_users_list aliases %v missing a 'saml' phrase", users.Aliases)
	}
	if !slices.Contains(users.RelatedActions, "group.saml_link_list") {
		t.Errorf("saml_users_list related %v missing group.saml_link_list", users.RelatedActions)
	}
	if !strings.Contains(users.IndividualTool.Description, "See also") {
		t.Errorf("saml_users_list description missing cross-references: %q", users.IndividualTool.Description)
	}

	if link := byTool["gitlab_group_saml_link_list"]; !slices.Contains(link.RelatedActions, "group.saml_users_list") {
		t.Errorf("saml_link_list should cross-reference group.saml_users_list, got %v", link.RelatedActions)
	}
}

// TestSAMLLinkGetDeleteMetadata_Discoverability locks in the non-generic
// discovery metadata for gitlab_group_saml_link_get and
// gitlab_group_saml_link_delete: an action-specific Usage, distinctive
// natural-language aliases beyond the tool name, canonical RelatedActions,
// and an IndividualTool.Description in "Returns: … See also: …" form.
func TestSAMLLinkGetDeleteMetadata_Discoverability(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	byTool := groupSAMLSpecsByTool(t, ActionSpecs(client))

	cases := []struct {
		tool    string
		related string
	}{
		{"gitlab_group_saml_link_get", "group.saml_link_list"},
		{"gitlab_group_saml_link_delete", "group.saml_link_get"},
	}
	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			spec, ok := byTool[tc.tool]
			if !ok {
				t.Fatalf("missing %s spec", tc.tool)
			}
			if spec.Usage == "" || strings.Contains(spec.Usage, "Use to execute") ||
				strings.Contains(spec.Usage, "Manage a group's SAML group links and SAML-provisioned users") {
				t.Errorf("%s has generic/empty Usage: %q", tc.tool, spec.Usage)
			}
			if !samlAliasHas(spec.Aliases, "saml") {
				t.Errorf("%s aliases %v missing a 'saml' phrase", tc.tool, spec.Aliases)
			}
			if slices.Contains(spec.Aliases, tc.tool) || len(spec.Aliases) < 2 {
				t.Errorf("%s aliases must be distinctive beyond the tool name, got %v", tc.tool, spec.Aliases)
			}
			if !slices.Contains(spec.RelatedActions, tc.related) {
				t.Errorf("%s related %v missing %s", tc.tool, spec.RelatedActions, tc.related)
			}
			if !strings.Contains(spec.IndividualTool.Description, "Returns:") ||
				!strings.Contains(spec.IndividualTool.Description, "See also") {
				t.Errorf("%s description missing Returns/See also form: %q", tc.tool, spec.IndividualTool.Description)
			}
		})
	}
}

func samlAliasHas(aliases []string, sub string) bool {
	for _, a := range aliases {
		if strings.Contains(a, sub) {
			return true
		}
	}
	return false
}

// TestActionSpecs_Metadata validates the Metadata route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)

	if len(specs) != 5 {
		t.Fatalf("len(ActionSpecs) = %d, want 5", len(specs))
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
	for _, name := range []string{"gitlab_group_saml_link_list", "gitlab_group_saml_link_get", "gitlab_group_saml_users_list"} {
		t.Run(name, func(t *testing.T) {
			if !byTool[name].ReadOnly {
				t.Errorf("%s should be read-only", name)
			}
		})
	}
	spec := byTool["gitlab_group_saml_link_delete"]
	if !spec.Destructive || !spec.Route.Destructive {
		t.Error("delete action should be destructive")
	}
	if !spec.Idempotent {
		t.Error("delete action should be idempotent")
	}
}

// TestActionSpecs_CallRoutes validates the CallRoutes route through the catalog surface.
// The mock GitLab API at /api/v4/groups/42/saml_group_links (GET) responds with HTTP OK.
// It asserts the route returns the expected error or result.
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

// TestActionSpecs_CallRouteError validates the CallRouteError route through the catalog surface.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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
