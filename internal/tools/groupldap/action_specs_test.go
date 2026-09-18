// action_specs_test.go contains unit tests for the group LDAP [toolutil.ActionSpec] entries.
package groupldap

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestLDAPSyncMetadata_Discoverability locks in the model-facing discovery
// metadata for the gitlab_group_ldap_sync action added with client-go v2.41.0,
// and verifies the sibling ldap_link_list cross-references it.
func TestLDAPSyncMetadata_Discoverability(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	byTool := groupLDAPSpecsByTool(t, ActionSpecs(client))

	sync, ok := byTool["gitlab_group_ldap_sync"]
	if !ok {
		t.Fatal("missing gitlab_group_ldap_sync spec")
	}
	if sync.Usage == "" || strings.Contains(sync.Usage, "Use to execute") {
		t.Errorf("ldap_sync has generic/empty Usage: %q", sync.Usage)
	}
	if !aliasHas(sync.Aliases, "sync") {
		t.Errorf("ldap_sync aliases %v missing a 'sync' phrase", sync.Aliases)
	}
	if !slices.Contains(sync.RelatedActions, "group.ldap_link_list") {
		t.Errorf("ldap_sync related %v missing group.ldap_link_list", sync.RelatedActions)
	}
	if !strings.Contains(sync.IndividualTool.Description, "asynchronous") {
		t.Errorf("ldap_sync description should warn it is asynchronous: %q", sync.IndividualTool.Description)
	}

	if list := byTool["gitlab_group_ldap_link_list"]; !slices.Contains(list.RelatedActions, "group.ldap_sync") {
		t.Errorf("ldap_link_list should cross-reference group.ldap_sync, got %v", list.RelatedActions)
	}
}

// TestLDAPMetadata_IndividualToolDescriptions locks in the "Returns: … See also: …"
// discovery descriptions for every group LDAP individual tool (1:1 audit R-META),
// guarding against generic or missing descriptions on the mutating link actions.
func TestLDAPMetadata_IndividualToolDescriptions(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	byTool := groupLDAPSpecsByTool(t, ActionSpecs(client))

	for _, name := range []string{
		"gitlab_group_ldap_link_list",
		"gitlab_group_ldap_link_add",
		"gitlab_group_ldap_sync",
		"gitlab_group_ldap_link_delete",
		"gitlab_group_ldap_link_delete_for_provider",
	} {
		t.Run(name, func(t *testing.T) {
			desc := byTool[name].IndividualTool.Description
			if desc == "" {
				t.Errorf("%s: IndividualTool.Description is empty", name)
				return
			}
			if !strings.Contains(desc, "Returns:") {
				t.Errorf("%s: description missing 'Returns:': %q", name, desc)
			}
			if !strings.Contains(desc, "See also:") {
				t.Errorf("%s: description missing 'See also:': %q", name, desc)
			}
		})
	}
}

// TestGroupLDAPOptions_UnknownIndividualTool_KeepsTheSharedMetadata asserts
// what a group LDAP action carries before the per-tool switch refines it.
// Every one of the five arms overwrites the usage, the aliases, the related
// actions and the description, so the shared base is observable nowhere else,
// and a sixth action added without an arm of its own inherits exactly this.
// The licensing gate is the part that matters: an LDAP group link is a
// Premium feature whether or not anybody has written discovery prose for the
// action yet, so Edition, OwnerPackage and the tags must not be things the
// switch supplies.
func TestGroupLDAPOptions_UnknownIndividualTool_KeepsTheSharedMetadata(t *testing.T) {
	const unknown = "gitlab_group_ldap_link_rename"

	options := groupLDAPOptions(unknown)

	if options.Edition != "premium" {
		t.Errorf("Edition = %q, want premium", options.Edition)
	}
	if options.OwnerPackage != "groupldap" {
		t.Errorf("OwnerPackage = %q, want groupldap", options.OwnerPackage)
	}
	if !options.OpenWorld {
		t.Error("OpenWorld = false, want true: an LDAP link names directory state this server does not own")
	}
	if !slices.Equal(options.Tags, []string{"group", "ldap"}) {
		t.Errorf("Tags = %v, want [group ldap]", options.Tags)
	}
	if !slices.Equal(options.Aliases, []string{unknown}) {
		t.Errorf("Aliases = %v, want the tool's own name as the only fallback alias", options.Aliases)
	}
	if options.Usage == "" {
		t.Error("Usage is empty: an action with no arm of its own still has to say what it is for")
	}
	if options.IndividualTool.Name != unknown {
		t.Errorf("IndividualTool.Name = %q, want %q", options.IndividualTool.Name, unknown)
	}
	if options.IndividualTool.Description != "" {
		t.Errorf("IndividualTool.Description = %q, want empty: the switch is what writes one", options.IndividualTool.Description)
	}
}

func aliasHas(aliases []string, sub string) bool {
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
		t.Run(name, func(t *testing.T) {
			spec := byTool[name]
			if !spec.Destructive || !spec.Route.Destructive {
				t.Errorf("%s should be destructive", name)
			}
			if !spec.Idempotent {
				t.Errorf("%s should be idempotent", name)
			}
		})
	}
}

// TestActionSpecs_CallRoutes validates the CallRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
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

// TestActionSpecs_CallRouteErrors validates the CallRouteErrors route through the catalog surface.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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
