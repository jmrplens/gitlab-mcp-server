// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// action_specs_test.go contains unit tests for the group LDAP [toolutil.ActionSpec] entries.
package groupldap

import (
	"encoding/json"
	"maps"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
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

// TestLDAPLinkSelectors_Schema_StatesExactlyOneOfCNOrFilter verifies that the
// two actions GitLab declares with exactly_one_of :cn, :filter (the add and
// the delete by CN or filter, ee/lib/api/ldap_group_links.rb) publish that
// constraint where a caller reads it: a oneOf of the two in the input schema,
// which is what find's example and the individual tool's validation read, and
// the words exactly one and not both in the Usage, the individual tool's
// description and the description of each of the two parameters.
//
// The schema used to publish both as optional and say neither the choice nor
// the constraint, so find's example sent neither and GitLab refused the call
// it built (issue 1270).
func TestLDAPLinkSelectors_Schema_StatesExactlyOneOfCNOrFilter(t *testing.T) {
	byTool := groupLDAPSpecsByTool(t, ActionSpecs(okClient(t)))
	wantOneOf := []any{
		map[string]any{"required": []string{"cn"}, "properties": map[string]any{"cn": map[string]any{"minLength": 1}}, "additionalProperties": true},
		map[string]any{"required": []string{"filter"}, "properties": map[string]any{"filter": map[string]any{"minLength": 1}}, "additionalProperties": true},
	}

	for _, name := range []string{"gitlab_group_ldap_link_add", "gitlab_group_ldap_link_delete"} {
		t.Run(name, func(t *testing.T) {
			spec := byTool[name]
			if err := spec.Validate(); err != nil {
				t.Fatalf("%s: Validate() error = %v", name, err)
			}
			if got := spec.Route.InputSchema["oneOf"]; !reflect.DeepEqual(got, wantOneOf) {
				t.Errorf("%s: input schema oneOf = %#v, want %#v", name, got, wantOneOf)
			}
			for where, text := range selectorTexts(spec) {
				for _, phrase := range []string{"exactly one of cn or filter", "not both"} {
					if !strings.Contains(text, phrase) {
						t.Errorf("%s: %s = %q, want it to say %q", name, where, text, phrase)
					}
				}
			}
		})
	}
}

// TestLDAPLinkSelectors_ActionSchema_CountsASelectorWithAValue verifies the
// catalog's input schema of the add and the delete by CN or filter, as the
// meta tool's full parameter-schema mode holds a call's params to it: one
// selector passes, both or neither are refused, and a selector with an empty
// value counts as none, as the handler counts it. It also verifies that the
// individual tools are served that schema without its oneOf.
//
// The oneOf counted a selector by its key alone, so the individual tool
// refused a common name beside an empty filter as naming both, a call the
// handler serves by sending the common name, and the meta and dynamic
// surfaces served it: one call had two answers. The individual tool now
// serves no oneOf at all, since the Anthropic Messages API refuses a tool
// list holding a root oneOf, and its calls are answered by the handler as the
// other surfaces' are.
func TestLDAPLinkSelectors_ActionSchema_CountsASelectorWithAValue(t *testing.T) {
	byTool := groupLDAPSpecsByTool(t, ActionSpecs(okClient(t)))
	calls := []struct {
		name      string
		selectors map[string]any
		wantValid bool
	}{
		{name: "the common name alone", selectors: map[string]any{"cn": "engineers"}, wantValid: true},
		{name: "the filter alone", selectors: map[string]any{"filter": "(dept=eng)"}, wantValid: true},
		{name: "the common name beside an empty filter", selectors: map[string]any{"cn": "engineers", "filter": ""}, wantValid: true},
		{name: "the filter beside an empty common name", selectors: map[string]any{"cn": "", "filter": "(dept=eng)"}, wantValid: true},
		{name: "both", selectors: map[string]any{"cn": "engineers", "filter": "(dept=eng)"}},
		{name: "neither", selectors: map[string]any{}},
		{name: "an empty common name alone", selectors: map[string]any{"cn": ""}},
	}
	for _, tool := range []struct {
		name string
		base map[string]any
	}{
		{name: "gitlab_group_ldap_link_add", base: map[string]any{"group_id": "mygroup", "group_access": 30, "provider": "main"}},
		{name: "gitlab_group_ldap_link_delete", base: map[string]any{"group_id": "mygroup", "provider": "main"}},
	} {
		t.Run(tool.name, func(t *testing.T) {
			projected, err := toolutil.IndividualToolFromActionSpec(byTool[tool.name], toolutil.IndividualToolProjectionOptions{})
			if err != nil {
				t.Fatalf("IndividualToolFromActionSpec(%s) error = %v", tool.name, err)
			}
			if served := toolutil.RootCombinators(projected.InputSchema); len(served) > 0 {
				t.Errorf("%s is served %v at the root of its input schema", tool.name, served)
			}
			resolved := resolvedInputSchema(t, toolutil.MetaActionSchema(byTool[tool.name].Route))
			for _, call := range calls {
				t.Run(call.name, func(t *testing.T) {
					arguments := maps.Clone(tool.base)
					maps.Copy(arguments, call.selectors)
					if validateErr := resolved.Validate(arguments); (validateErr == nil) != call.wantValid {
						t.Errorf("Validate(%v) error = %v, want valid %t", arguments, validateErr, call.wantValid)
					}
				})
			}
		})
	}
}

// resolvedInputSchema decodes a served input schema into a resolved
// jsonschema-go schema, the validator the SDK puts a tool's arguments to.
func resolvedInputSchema(t *testing.T, schema any) *jsonschema.Resolved {
	t.Helper()
	data, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal input schema: %v", err)
	}
	var decoded jsonschema.Schema
	if err = json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode input schema: %v", err)
	}
	resolved, err := decoded.Resolve(nil)
	if err != nil {
		t.Fatalf("resolve input schema: %v", err)
	}
	return resolved
}

// TestLDAPLinkActions_WithoutASelectorChoice_CarryNoOneOf verifies that the
// three LDAP actions that take no selector, or a common name alone, publish
// no oneOf: a constraint on a choice they do not offer would refuse calls
// GitLab serves.
func TestLDAPLinkActions_WithoutASelectorChoice_CarryNoOneOf(t *testing.T) {
	byTool := groupLDAPSpecsByTool(t, ActionSpecs(okClient(t)))
	for _, name := range []string{"gitlab_group_ldap_link_list", "gitlab_group_ldap_sync", "gitlab_group_ldap_link_delete_for_provider"} {
		t.Run(name, func(t *testing.T) {
			if got, has := byTool[name].Route.InputSchema["oneOf"]; has {
				t.Errorf("%s: input schema oneOf = %#v, want none", name, got)
			}
		})
	}
}

// selectorTexts returns the four places a selector action states its choice,
// keyed by where each is read: its Usage, its individual tool description and
// the schema descriptions of cn and filter.
func selectorTexts(spec toolutil.ActionSpec) map[string]string {
	texts := map[string]string{"Usage": spec.Usage, "IndividualTool.Description": spec.IndividualTool.Description}
	properties, _ := spec.Route.InputSchema["properties"].(map[string]any)
	for _, param := range []string{"cn", "filter"} {
		property, _ := properties[param].(map[string]any)
		texts[param+" description"], _ = property["description"].(string)
	}
	return texts
}

// okClient returns a client whose mock GitLab answers every request with 200
// and no body, for a test that builds the specs and sends nothing.
func okClient(t *testing.T) *gitlabclient.Client {
	t.Helper()
	return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
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
