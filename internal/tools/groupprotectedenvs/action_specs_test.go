// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// action_specs_test.go contains unit tests for the group protected environment [toolutil.ActionSpec] entries.
package groupprotectedenvs

import (
	"encoding/json"
	"maps"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const (
	registerProtEnvJSON     = `{"name":"production","deploy_access_levels":[{"access_level":40}]}`
	registerProtEnvListJSON = `[{"name":"production","deploy_access_levels":[{"access_level":40}]}]`
)

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
		if spec.OwnerPackage != "groupprotectedenvs" {
			t.Errorf("OwnerPackage for %s = %q, want groupprotectedenvs", spec.Name, spec.OwnerPackage)
		}
		if spec.IndividualTool.Name == "" {
			t.Errorf("IndividualTool.Name for %s is empty", spec.Name)
		}
	}

	byTool := groupProtectedEnvSpecsByTool(t, specs)
	for _, name := range []string{"gitlab_group_protected_environment_list", "gitlab_group_protected_environment_get"} {
		t.Run(name, func(t *testing.T) {
			if !byTool[name].ReadOnly {
				t.Errorf("%s should be read-only", name)
			}
		})
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

// TestActionSpecs_ApprovalRuleNamingNoGrantee_IsRefusedThroughTheRoute
// verifies that an approval rule naming a count and no grantee is refused when
// it comes through the canonical route, the one the meta and dynamic surfaces
// call, and that no request reaches GitLab.
//
// The route decodes its arguments with toolutil.UnmarshalParams before the
// handler checks them. Decoding used to give such a rule the Maintainer role,
// so the protect went to GitLab as a rule letting Maintainers approve and the
// update of an existing rule moved its grantee to them, while the individual
// tool's schema refused both calls.
func TestActionSpecs_ApprovalRuleNamingNoGrantee_IsRefusedThroughTheRoute(t *testing.T) {
	var requests atomic.Int32
	byTool := groupProtectedEnvSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		testutil.RespondJSON(w, http.StatusCreated, registerProtEnvJSON)
	}))))
	maintainers := []any{map[string]any{"access_level": float64(40)}}
	const noGrantee = "approval_rules[0] names none of access_level, user_id or group_id"

	tests := []struct {
		name string
		tool string
		args map[string]any
	}{
		{name: "protect with a count alone", tool: "gitlab_group_protected_environment_protect", args: map[string]any{
			"group_id": "mygroup", "name": "staging", "deploy_access_levels": maintainers,
			"approval_rules": []any{map[string]any{"required_approvals": float64(1)}},
		}},
		{name: "protect with a count under an alias", tool: "gitlab_group_protected_environment_protect", args: map[string]any{
			"group_id": "mygroup", "name": "staging", "deploy_access_levels": maintainers,
			"approval_rules": []any{map[string]any{"approvals_required": float64(2)}},
		}},
		{name: "update of an existing rule with a count alone", tool: "gitlab_group_protected_environment_update", args: map[string]any{
			"group_id": "mygroup", "environment": "production",
			"approval_rules": []any{map[string]any{"id": float64(7), "required_approvals": float64(2)}},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err == nil || !strings.Contains(err.Error(), noGrantee) {
				t.Fatalf("Route.Handler(%s) error = %v, want one containing %q", tt.tool, err, noGrantee)
			}
		})
	}
	if sent := requests.Load(); sent != 0 {
		t.Errorf("%d request(s) reached GitLab, want none", sent)
	}
}

// TestGroupProtectedEnvSchemas_CarryTheTiersAndGitLabsEntryRules verifies the
// overrides each action carries: the deployment tiers on every parameter that
// names one, and on the protect and the update the overrides that state
// GitLab's rules about an entry of deploy_access_levels and approval_rules,
// the same rules the handlers refuse a call by. Every spec still validates,
// since each override reaches a property its schema has.
func TestGroupProtectedEnvSchemas_CarryTheTiersAndGitLabsEntryRules(t *testing.T) {
	byTool := groupProtectedEnvSpecsByTool(t, ActionSpecs(okClient(t)))
	tiers := func(params ...string) []toolutil.InputSchemaOverride {
		overrides := make([]toolutil.InputSchemaOverride, 0, len(params))
		for _, param := range params {
			overrides = append(overrides, toolutil.SchemaEnumOverride(param, "production", "staging", "testing", "development", "other"))
		}
		return overrides
	}
	want := map[string][]toolutil.InputSchemaOverride{
		"gitlab_group_protected_environment_list":      nil,
		"gitlab_group_protected_environment_get":       tiers("environment"),
		"gitlab_group_protected_environment_protect":   append(tiers("name"), toolutil.EnvironmentProtectSchemaOverrides()...),
		"gitlab_group_protected_environment_update":    append(tiers("environment", "name"), toolutil.EnvironmentUpdateSchemaOverrides()...),
		"gitlab_group_protected_environment_unprotect": tiers("environment"),
	}
	for tool, overrides := range want {
		t.Run(tool, func(t *testing.T) {
			spec := byTool[tool]
			if err := spec.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if !reflect.DeepEqual(spec.InputSchemaOverrides, overrides) {
				t.Errorf("InputSchemaOverrides = %#v, want %#v", spec.InputSchemaOverrides, overrides)
			}
		})
	}
}

// TestGroupProtectedEnvSchemas_IndividualTool_HoldsEachEntry verifies the
// input schema the individual protect and update tools are served with, put
// to the validator the SDK holds their arguments to: an entry GitLab takes
// passes and one it refuses is refused, a null counting as a field left out.
func TestGroupProtectedEnvSchemas_IndividualTool_HoldsEachEntry(t *testing.T) {
	byTool := groupProtectedEnvSpecsByTool(t, ActionSpecs(okClient(t)))
	maintainers := []any{map[string]any{"access_level": 40}}
	type call struct {
		name      string
		params    map[string]any
		wantValid bool
	}
	tools := []struct {
		tool  string
		base  map[string]any
		calls []call
	}{
		{
			tool: "gitlab_group_protected_environment_protect",
			base: map[string]any{"group_id": "mygroup", "name": "production"},
			calls: []call{
				{name: "a role", params: map[string]any{"deploy_access_levels": maintainers}, wantValid: true},
				{name: "a role beside a null group", params: map[string]any{"deploy_access_levels": []any{map[string]any{"access_level": 30, "group_id": nil}}}, wantValid: true},
				{name: "a group with its inheritance", params: map[string]any{"deploy_access_levels": []any{map[string]any{"group_id": 7, "group_inheritance_type": 0}}}, wantValid: true},
				{name: "no deploy access level", params: map[string]any{"deploy_access_levels": []any{}}},
				{name: "a deploy entry naming nothing", params: map[string]any{"deploy_access_levels": []any{map[string]any{}}}},
				{name: "a deploy entry naming a user and a group", params: map[string]any{"deploy_access_levels": []any{map[string]any{"user_id": 5, "group_id": 7}}}},
				{name: "No access", params: map[string]any{"deploy_access_levels": []any{map[string]any{"access_level": 0}}}},
				{name: "an approval rule naming all three", params: map[string]any{"deploy_access_levels": maintainers, "approval_rules": []any{map[string]any{"access_level": 30, "user_id": 5, "group_id": 7, "required_approvals": 5}}}, wantValid: true},
				{name: "an approval rule naming nothing", params: map[string]any{"deploy_access_levels": maintainers, "approval_rules": []any{map[string]any{"required_approvals": 1}}}},
				{name: "an approval rule with a description GitLab drops", params: map[string]any{"deploy_access_levels": maintainers, "approval_rules": []any{map[string]any{"user_id": 5, "access_level_description": "Release managers"}}}},
				{name: "the deprecated count at 1", params: map[string]any{"deploy_access_levels": maintainers, "required_approval_count": 1}},
				{name: "a name outside the tiers", params: map[string]any{"name": "prod", "deploy_access_levels": maintainers}},
			},
		},
		{
			tool: "gitlab_group_protected_environment_update",
			base: map[string]any{"group_id": "mygroup", "environment": "production"},
			calls: []call{
				{name: "an existing entry by its id", params: map[string]any{"deploy_access_levels": []any{map[string]any{"id": 11}}}, wantValid: true},
				{name: "a new entry naming nothing", params: map[string]any{"deploy_access_levels": []any{map[string]any{"group_inheritance_type": 1}}}},
				{name: "an existing entry naming a user and a role", params: map[string]any{"deploy_access_levels": []any{map[string]any{"id": 11, "user_id": 5, "access_level": 40}}}},
				{name: "an approval rule removed", params: map[string]any{"approval_rules": []any{map[string]any{"id": 21, "_destroy": true}}}, wantValid: true},
				{name: "an approval rule removed asking for six approvals", params: map[string]any{"approval_rules": []any{map[string]any{"id": 21, "_destroy": true, "group_id": 7, "required_approvals": 6}}}, wantValid: true},
				{name: "an approval rule kept with _destroy false", params: map[string]any{"approval_rules": []any{map[string]any{"id": 21, "_destroy": false}}}},
				{name: "an approval rule given five approvals", params: map[string]any{"approval_rules": []any{map[string]any{"id": 21, "user_id": 5, "required_approvals": 5}}}, wantValid: true},
				{name: "a new approval rule asking for one approval", params: map[string]any{"approval_rules": []any{map[string]any{"group_id": 7, "required_approvals": 1}}}, wantValid: true},
				{name: "an approval rule given six approvals", params: map[string]any{"approval_rules": []any{map[string]any{"id": 21, "user_id": 5, "required_approvals": 6}}}},
				{name: "an approval rule kept with _destroy false asking for no approval", params: map[string]any{"approval_rules": []any{map[string]any{"id": 21, "_destroy": false, "access_level": 30, "required_approvals": 0}}}},
			},
		},
	}
	for _, tool := range tools {
		t.Run(tool.tool, func(t *testing.T) {
			projected, err := toolutil.IndividualToolFromActionSpec(byTool[tool.tool], toolutil.IndividualToolProjectionOptions{})
			if err != nil {
				t.Fatalf("IndividualToolFromActionSpec(%s) error = %v", tool.tool, err)
			}
			resolved := resolvedInputSchema(t, projected.InputSchema)
			for _, call := range tool.calls {
				t.Run(call.name, func(t *testing.T) {
					arguments := maps.Clone(tool.base)
					maps.Copy(arguments, call.params)
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

// okClient returns a client whose mock GitLab answers every request with 200
// and no body, for a test that builds the specs and sends nothing.
func okClient(t *testing.T) *gitlabclient.Client {
	t.Helper()
	return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
}

// TestActionSpecs_CallRoutes validates the CallRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
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

// TestGroupProtectedEnvDescription verifies each individual tool advertises a
// "Returns: … See also: …" description (R-META) and that the helper returns the
// empty string for an unknown tool name.
func TestGroupProtectedEnvDescription(t *testing.T) {
	tools := []string{
		"gitlab_group_protected_environment_list",
		"gitlab_group_protected_environment_get",
		"gitlab_group_protected_environment_protect",
		"gitlab_group_protected_environment_update",
		"gitlab_group_protected_environment_unprotect",
	}
	for _, name := range tools {
		t.Run(name, func(t *testing.T) {
			desc := groupProtectedEnvDescription(name)
			if !strings.Contains(desc, "Returns:") || !strings.Contains(desc, "See also:") {
				t.Errorf("description for %s = %q, want Returns:/See also: form", name, desc)
			}
		})
	}
	if got := groupProtectedEnvDescription("gitlab_unknown_tool"); got != "" {
		t.Errorf("description for unknown tool = %q, want empty", got)
	}
}

// TestGroupProtectedEnvActionMeta asserts each group-protected-environment
// action carries non-generic action-specific Usage, distinctive group-scoped
// natural-language Aliases beyond the tool name, and canonical RelatedActions,
// so none is flagged by the R-META metadata-completeness audit.
func TestGroupProtectedEnvActionMeta(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	byTool := groupProtectedEnvSpecsByTool(t, ActionSpecs(client))

	for tool := range groupProtectedEnvActionMeta {
		spec, ok := byTool[tool]
		if !ok {
			t.Fatalf("meta tool %q has no projected ActionSpec", tool)
		}

		if strings.TrimSpace(spec.Usage) == "" || strings.Contains(strings.ToLower(spec.Usage), "use group protected environment actions for group-level deployment gates") {
			t.Errorf("%s: Usage is empty or still the generic shared sentence: %q", tool, spec.Usage)
		}

		distinctive := 0
		for _, alias := range spec.Aliases {
			normalized := strings.ToLower(strings.TrimSpace(alias))
			if normalized == "" || normalized == tool || normalized == strings.ToLower(spec.Name) {
				continue
			}
			if !strings.Contains(normalized, "group") {
				t.Errorf("%s: alias %q is not group-scoped", tool, alias)
			}
			distinctive++
		}
		if distinctive < 2 || distinctive > 4 {
			t.Errorf("%s: want 2-4 distinctive aliases beyond the tool name, got %d (%v)", tool, distinctive, spec.Aliases)
		}

		if len(spec.RelatedActions) == 0 {
			t.Errorf("%s: RelatedActions is empty", tool)
		}
		for _, related := range spec.RelatedActions {
			if strings.TrimSpace(related) == "" {
				t.Errorf("%s: RelatedActions contains an empty entry", tool)
			}
		}
	}
}

// TestGroupProtectedEnvSpecs_PublishEachActionsOwnDiscoveryMetadata asserts the
// projected spec carries the table's own usage, aliases and related actions,
// entry for entry, rather than the shared defaults the options start with. The
// check beside this one asks only whether each list is non-empty and
// group-scoped, and the shared defaults satisfy both: a decorator that stopped
// copying the related actions would leave all five actions pointing at
// group.get alone, which is a plausible-looking answer and the reason a model
// would never find the sibling action it needs next.
func TestGroupProtectedEnvSpecs_PublishEachActionsOwnDiscoveryMetadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	byTool := groupProtectedEnvSpecsByTool(t, ActionSpecs(client))

	for tool, meta := range groupProtectedEnvActionMeta {
		t.Run(tool, func(t *testing.T) {
			spec, ok := byTool[tool]
			if !ok {
				t.Fatalf("meta tool %q has no projected ActionSpec", tool)
			}
			if spec.Usage != meta.usage {
				t.Errorf("Usage = %q, want the entry's own %q", spec.Usage, meta.usage)
			}
			if !slices.Equal(spec.Aliases, meta.aliases) {
				t.Errorf("Aliases = %v, want the entry's own %v", spec.Aliases, meta.aliases)
			}
			if !slices.Equal(spec.RelatedActions, meta.related) {
				t.Errorf("RelatedActions = %v, want the entry's own %v", spec.RelatedActions, meta.related)
			}
		})
	}
}

// TestDecorateGroupProtectedEnvMeta_PartialEntry_KeepsTheSharedDefaults asserts
// what each of the decorator's three guards is for: an entry that fills some
// fields leaves the rest at the shared defaults instead of blanking them. Every
// entry in the table happens to fill all three today, so the guards are
// unobservable through the table as it stands, and without this an entry added
// with only a usage would publish no aliases and no related actions at all --
// the R-META findings the table exists to answer, reintroduced by an omission
// rather than by an edit.
func TestDecorateGroupProtectedEnvMeta_PartialEntry_KeepsTheSharedDefaults(t *testing.T) {
	const tool = "gitlab_group_protected_environment_list"
	sharedAliases := []string{tool}
	sharedRelated := []string{"group.get"}

	tests := []struct {
		name        string
		entry       groupProtectedEnvActionMetaEntry
		wantUsage   string
		wantAliases []string
		wantRelated []string
	}{
		{
			name:        "an entry naming only a usage keeps the shared aliases and related actions",
			entry:       groupProtectedEnvActionMetaEntry{usage: "Only a usage."},
			wantUsage:   "Only a usage.",
			wantAliases: sharedAliases,
			wantRelated: sharedRelated,
		},
		{
			name: "an entry naming no usage keeps the shared sentence while taking its own lists",
			entry: groupProtectedEnvActionMetaEntry{
				aliases: []string{"list group deployment gates"},
				related: []string{actionGet},
			},
			wantAliases: []string{"list group deployment gates"},
			wantRelated: []string{actionGet},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := groupProtectedEnvActionMeta[tool]
			t.Cleanup(func() { groupProtectedEnvActionMeta[tool] = original })
			groupProtectedEnvActionMeta[tool] = tt.entry

			options := groupProtectedEnvOptions(tool)

			if tt.wantUsage == "" {
				if !strings.Contains(options.Usage, "Use group protected environment actions") {
					t.Errorf("Usage = %q, want the shared default sentence", options.Usage)
				}
			} else if options.Usage != tt.wantUsage {
				t.Errorf("Usage = %q, want the entry's own %q", options.Usage, tt.wantUsage)
			}
			if !slices.Equal(options.Aliases, tt.wantAliases) {
				t.Errorf("Aliases = %v, want %v", options.Aliases, tt.wantAliases)
			}
			if !slices.Equal(options.RelatedActions, tt.wantRelated) {
				t.Errorf("RelatedActions = %v, want %v", options.RelatedActions, tt.wantRelated)
			}
		})
	}
}

// TestDecorateGroupProtectedEnvMeta_UnknownToolNoOp verifies the decorator
// leaves the shared default Usage, Aliases, and RelatedActions untouched for a
// tool name absent from the metadata map.
func TestDecorateGroupProtectedEnvMeta_UnknownToolNoOp(t *testing.T) {
	options := groupProtectedEnvOptions("gitlab_unknown_tool")
	if len(options.Aliases) != 1 || options.Aliases[0] != "gitlab_unknown_tool" {
		t.Errorf("unknown tool Aliases = %v, want shared default [gitlab_unknown_tool]", options.Aliases)
	}
	if !strings.Contains(options.Usage, "Use group protected environment actions") {
		t.Errorf("unknown tool Usage = %q, want shared default", options.Usage)
	}
	if len(options.RelatedActions) != 1 || options.RelatedActions[0] != "group.get" {
		t.Errorf("unknown tool RelatedActions = %v, want shared default [group.get]", options.RelatedActions)
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
