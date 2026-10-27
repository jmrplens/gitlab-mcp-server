// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// action_specs_test.go contains canonical-route tests for protected environment actions.
package protectedenvs

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestActionSpecs_CallAllRoutes exercises every protected environment tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := protectedEnvironmentSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, protectedEnvironmentsActionHandler())))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_protected_environment_list", map[string]any{"project_id": "42"}},
		{"gitlab_protected_environment_get", map[string]any{"project_id": "42", "environment": "production"}},
		{"gitlab_protected_environment_protect", map[string]any{"project_id": "42", "name": "staging", "deploy_access_levels": []any{map[string]any{"access_level": float64(40)}}}},
		{"gitlab_protected_environment_update", map[string]any{"project_id": "42", "environment": "production"}},
		{"gitlab_protected_environment_unprotect", map[string]any{"project_id": "42", "environment": "production"}},
	}

	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			result, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.tool)
			}
		})
	}
}

// TestActionSpecs_ErrorPaths verifies get and unprotect routes propagate 404s.
func TestActionSpecs_ErrorPaths(t *testing.T) {
	byTool := protectedEnvironmentSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404 Not Found"}`))
	}))))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_protected_environment_get", map[string]any{"project_id": "42", "environment": "prod"}},
		{"gitlab_protected_environment_unprotect", map[string]any{"project_id": "42", "environment": "prod"}},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			_, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err == nil {
				t.Fatal("expected route error")
			}
		})
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
	byTool := protectedEnvironmentSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		testutil.RespondJSON(w, http.StatusCreated, `{"name":"staging"}`)
	}))))
	maintainers := []any{map[string]any{"access_level": float64(40)}}
	const noGrantee = "approval_rules[0] names none of access_level, user_id or group_id"

	tests := []struct {
		name string
		tool string
		args map[string]any
	}{
		{name: "protect with a count alone", tool: "gitlab_protected_environment_protect", args: map[string]any{
			"project_id": "42", "name": "staging", "deploy_access_levels": maintainers,
			"approval_rules": []any{map[string]any{"required_approvals": float64(1)}},
		}},
		{name: "protect with a count under an alias", tool: "gitlab_protected_environment_protect", args: map[string]any{
			"project_id": "42", "name": "staging", "deploy_access_levels": maintainers,
			"approval_rules": []any{map[string]any{"approvals_required": float64(2)}},
		}},
		{name: "update of an existing rule with a count alone", tool: "gitlab_protected_environment_update", args: map[string]any{
			"project_id": "42", "environment": "production",
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

// TestActionSpecs_UnprotectOutput verifies the unprotect route preserves its success message.
func TestActionSpecs_UnprotectOutput(t *testing.T) {
	byTool := protectedEnvironmentSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, protectedEnvironmentsActionHandler())))

	result, err := byTool["gitlab_protected_environment_unprotect"].Route.Handler(t.Context(), map[string]any{
		"project_id":  "42",
		"environment": "production",
	})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_protected_environment_unprotect) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_protected_environment_unprotect) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted protected environment." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestActionSpecs_ProtectRequiresDeployAccessLevels verifies discovery schemas
// advertise the access rule required to create a protected environment.
func TestActionSpecs_ProtectRequiresDeployAccessLevels(t *testing.T) {
	byTool := protectedEnvironmentSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, protectedEnvironmentsActionHandler())))
	schema := byTool["gitlab_protected_environment_protect"].Route.InputSchema
	if !schemaRequiredIncludes(schema, "deploy_access_levels") {
		t.Fatalf("protect required fields = %v, want deploy_access_levels", schema["required"])
	}
}

// TestProtectedEnvironmentSchemas_CarryGitLabsEntryRules verifies that the
// protect and the update carry the overrides that state GitLab's rules about
// an entry, the protect's and the update's respectively, that the two read
// actions and the unprotect carry none, and that every spec still validates:
// each override reaches a property its schema has.
func TestProtectedEnvironmentSchemas_CarryGitLabsEntryRules(t *testing.T) {
	byTool := protectedEnvironmentSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, protectedEnvironmentsActionHandler())))
	want := map[string][]toolutil.InputSchemaOverride{
		"gitlab_protected_environment_list":      nil,
		"gitlab_protected_environment_get":       nil,
		"gitlab_protected_environment_protect":   toolutil.EnvironmentProtectSchemaOverrides(),
		"gitlab_protected_environment_update":    toolutil.EnvironmentUpdateSchemaOverrides(),
		"gitlab_protected_environment_unprotect": nil,
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

// TestProtectedEnvironmentSchemas_IndividualTool_HoldsEachEntry verifies the
// input schema the individual protect and update tools are served with, put
// to the validator the SDK holds their arguments to: an entry GitLab takes
// passes and one it refuses is refused, a null counting as a field left out.
func TestProtectedEnvironmentSchemas_IndividualTool_HoldsEachEntry(t *testing.T) {
	byTool := protectedEnvironmentSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, protectedEnvironmentsActionHandler())))
	maintainers := []any{map[string]any{"access_level": 40}}
	tools := []struct {
		tool  string
		base  map[string]any
		calls []schemaCall
	}{
		{
			tool: "gitlab_protected_environment_protect",
			base: map[string]any{"project_id": "42", "name": "production"},
			calls: []schemaCall{
				{name: "a role", params: map[string]any{"deploy_access_levels": maintainers}, wantValid: true},
				{name: "a role beside a null user", params: map[string]any{"deploy_access_levels": []any{map[string]any{"access_level": 30, "user_id": nil}}}, wantValid: true},
				{name: "a user with a null role", params: map[string]any{"deploy_access_levels": []any{map[string]any{"user_id": 5, "access_level": nil}}}, wantValid: true},
				{name: "a group with its inheritance", params: map[string]any{"deploy_access_levels": []any{map[string]any{"group_id": 7, "group_inheritance_type": 1}}}, wantValid: true},
				{name: "no deploy access level", params: map[string]any{"deploy_access_levels": []any{}}},
				{name: "a deploy entry naming nothing", params: map[string]any{"deploy_access_levels": []any{map[string]any{}}}},
				{name: "a deploy entry naming a role and a user", params: map[string]any{"deploy_access_levels": []any{map[string]any{"access_level": 40, "user_id": 5}}}},
				{name: "No access", params: map[string]any{"deploy_access_levels": []any{map[string]any{"access_level": 0}}}},
				{name: "an inheritance GitLab does not take", params: map[string]any{"deploy_access_levels": []any{map[string]any{"group_id": 7, "group_inheritance_type": 2}}}},
				{name: "an approval rule naming a user and two approvals", params: map[string]any{"deploy_access_levels": maintainers, "approval_rules": []any{map[string]any{"user_id": 5, "required_approvals": 2}}}, wantValid: true},
				{name: "an approval rule naming a role and a group", params: map[string]any{"deploy_access_levels": maintainers, "approval_rules": []any{map[string]any{"access_level": 30, "group_id": 7}}}, wantValid: true},
				{name: "an approval rule naming nothing", params: map[string]any{"deploy_access_levels": maintainers, "approval_rules": []any{map[string]any{"required_approvals": 2}}}},
				{name: "six approvals", params: map[string]any{"deploy_access_levels": maintainers, "approval_rules": []any{map[string]any{"access_level": 30, "required_approvals": 6}}}},
				{name: "no approval", params: map[string]any{"deploy_access_levels": maintainers, "approval_rules": []any{map[string]any{"access_level": 30, "required_approvals": 0}}}},
				{name: "an approval rule with a description GitLab drops", params: map[string]any{"deploy_access_levels": maintainers, "approval_rules": []any{map[string]any{"user_id": 5, "access_level_description": "Release managers"}}}},
				{name: "the deprecated count at 0", params: map[string]any{"deploy_access_levels": maintainers, "required_approval_count": 0}, wantValid: true},
				{name: "the deprecated count at 1", params: map[string]any{"deploy_access_levels": maintainers, "required_approval_count": 1}},
			},
		},
		{
			tool: "gitlab_protected_environment_update",
			base: map[string]any{"project_id": "42", "environment": "production"},
			calls: []schemaCall{
				{name: "nothing to change", params: map[string]any{}, wantValid: true},
				{name: "an existing entry by its id", params: map[string]any{"deploy_access_levels": []any{map[string]any{"id": 11, "_destroy": true}}}, wantValid: true},
				{name: "an existing entry given a role", params: map[string]any{"deploy_access_levels": []any{map[string]any{"id": 11, "access_level": 30}}}, wantValid: true},
				{name: "a new entry naming a group", params: map[string]any{"deploy_access_levels": []any{map[string]any{"group_id": 7}}}, wantValid: true},
				{name: "a new entry naming nothing", params: map[string]any{"deploy_access_levels": []any{map[string]any{"_destroy": true}}}},
				{name: "an existing entry naming a role and a group", params: map[string]any{"deploy_access_levels": []any{map[string]any{"id": 11, "access_level": 30, "group_id": 7}}}},
				{name: "an approval rule removed", params: map[string]any{"approval_rules": []any{map[string]any{"id": 21, "_destroy": true}}}, wantValid: true},
				{name: "an approval rule removed asking for no approval", params: map[string]any{"approval_rules": []any{map[string]any{"id": 21, "_destroy": true, "required_approvals": 0}}}, wantValid: true},
				{name: "an existing approval rule asking for five approvals", params: map[string]any{"approval_rules": []any{map[string]any{"id": 21, "user_id": 5, "required_approvals": 5}}}, wantValid: true},
				{name: "a new approval rule asking for one approval", params: map[string]any{"approval_rules": []any{map[string]any{"access_level": 40, "required_approvals": 1}}}, wantValid: true},
				{name: "an existing approval rule asking for no approval", params: map[string]any{"approval_rules": []any{map[string]any{"id": 21, "access_level": 30, "required_approvals": 0}}}},
				{name: "an approval rule kept with _destroy false asking for six approvals", params: map[string]any{"approval_rules": []any{map[string]any{"id": 21, "_destroy": false, "user_id": 5, "required_approvals": 6}}}},
				{name: "an approval rule with a null _destroy asking for six approvals", params: map[string]any{"approval_rules": []any{map[string]any{"id": 21, "_destroy": nil, "user_id": 5, "required_approvals": 6}}}},
				{name: "an existing approval rule naming nothing", params: map[string]any{"approval_rules": []any{map[string]any{"id": 21, "required_approvals": 2}}}},
				{name: "the deprecated count at 2", params: map[string]any{"required_approval_count": 2}},
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

// schemaCall is one call put to a served input schema: its parameters beside
// the ones every call of the tool carries, and whether the schema takes it.
type schemaCall struct {
	name      string
	params    map[string]any
	wantValid bool
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

// TestApprovalRuleInputs_OfferNoAccessLevelDescription verifies that neither
// approval rule shape offers access_level_description. client-go's options
// carry it, and neither the protect nor the update route declares it, so
// GitLab drops it and a caller who set it changed nothing (upstream register
// row 139). The deploy access levels never offered it.
func TestApprovalRuleInputs_OfferNoAccessLevelDescription(t *testing.T) {
	byTool := protectedEnvironmentSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, protectedEnvironmentsActionHandler())))
	for _, tool := range []string{"gitlab_protected_environment_protect", "gitlab_protected_environment_update"} {
		t.Run(tool, func(t *testing.T) {
			properties, _ := byTool[tool].Route.InputSchema["properties"].(map[string]any)
			rules, _ := properties["approval_rules"].(map[string]any)
			items, _ := rules["items"].(map[string]any)
			itemProperties, _ := items["properties"].(map[string]any)
			if len(itemProperties) == 0 {
				t.Fatalf("approval_rules items carry no properties: %v", rules)
			}
			if _, has := itemProperties["access_level_description"]; has {
				t.Errorf("approval_rules items offer access_level_description: %v", itemProperties)
			}
		})
	}
}

// TestCatalogSurface_UnprotectConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_UnprotectConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	byTool := protectedEnvironmentSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_protected_environment_unprotect"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test protected environment destructive confirmation.",
		Icons:       toolutil.IconShield,
	})

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "decline"}, nil
		},
	})
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		session.Close()
		_ = serverSession.Wait()
	})

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_protected_environment_unprotect",
		Arguments: map[string]any{"project_id": "42", "environment": "production"},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

func protectedEnvironmentsActionHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == pathProtectedEnvs:
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+envJSON+`]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
		case r.Method == http.MethodGet && r.URL.Path == pathProtectedEnv1:
			testutil.RespondJSON(w, http.StatusOK, envJSON)
		case r.Method == http.MethodPost && r.URL.Path == pathProtectedEnvs:
			testutil.RespondJSON(w, http.StatusCreated, envJSON)
		case r.Method == http.MethodPut && r.URL.Path == pathProtectedEnv1:
			testutil.RespondJSON(w, http.StatusOK, envJSON)
		case r.Method == http.MethodDelete && r.URL.Path == pathProtectedEnv1:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
}

func protectedEnvironmentSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		toolName := spec.IndividualTool.Name
		if toolName == "" {
			t.Fatalf("spec %s missing IndividualTool.Name", spec.Name)
		}
		if _, exists := byTool[toolName]; exists {
			t.Fatalf("duplicate individual tool %q", toolName)
		}
		byTool[toolName] = spec
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

// TestProtectedEnvironmentDescription_AllActions verifies each protected
// environment action carries a "Returns: … See also: …" individual-tool
// description (R-META) and that an unknown tool name yields an empty string.
func TestProtectedEnvironmentDescription_AllActions(t *testing.T) {
	tools := []string{
		"gitlab_protected_environment_list",
		"gitlab_protected_environment_get",
		"gitlab_protected_environment_protect",
		"gitlab_protected_environment_update",
		"gitlab_protected_environment_unprotect",
	}
	for _, name := range tools {
		t.Run(name, func(t *testing.T) {
			desc := protectedEnvironmentDescription(name)
			if !strings.Contains(desc, "Returns:") || !strings.Contains(desc, "See also:") {
				t.Errorf("%s description missing Returns/See also: %q", name, desc)
			}
		})
	}
	if got := protectedEnvironmentDescription("gitlab_unknown"); got != "" {
		t.Errorf("unknown tool description = %q, want empty", got)
	}
}

// TestProtectedEnvironmentMeta_PerToolDiscovery verifies every protected
// environment tool carries action-specific Usage, at least two
// natural-language Aliases beyond the bare tool name, and non-empty
// RelatedActions (R-META). It also covers decorateProtectedEnvironmentMeta's
// no-op path for an unknown tool name, which keeps the shared defaults.
func TestProtectedEnvironmentMeta_PerToolDiscovery(t *testing.T) {
	const sharedUsagePrefix = "Use project protected environment actions for project deployment gates."

	tools := []string{
		"gitlab_protected_environment_list",
		"gitlab_protected_environment_get",
		"gitlab_protected_environment_protect",
		"gitlab_protected_environment_update",
		"gitlab_protected_environment_unprotect",
	}
	for _, name := range tools {
		t.Run(name, func(t *testing.T) {
			opts := protectedEnvironmentOptions(name)
			if strings.HasPrefix(opts.Usage, sharedUsagePrefix) {
				t.Errorf("%s still uses generic shared Usage: %q", name, opts.Usage)
			}
			if len(opts.Aliases) < 2 {
				t.Errorf("%s has %d aliases, want >= 2", name, len(opts.Aliases))
			}
			for _, alias := range opts.Aliases {
				if alias == name {
					t.Errorf("%s alias list still contains the bare tool name", name)
				}
				if strings.Contains(alias, "group") {
					t.Errorf("%s alias %q overlaps group-level wording", name, alias)
				}
			}
			if len(opts.RelatedActions) == 0 {
				t.Errorf("%s has empty RelatedActions", name)
			}
		})
	}

	// Unknown tool: decorator is a no-op, so the shared defaults remain.
	def := protectedEnvironmentOptions("gitlab_unknown_tool")
	if !strings.HasPrefix(def.Usage, sharedUsagePrefix) {
		t.Errorf("unknown tool Usage = %q, want shared default", def.Usage)
	}
	if len(def.Aliases) != 1 || def.Aliases[0] != "gitlab_unknown_tool" {
		t.Errorf("unknown tool Aliases = %v, want only the tool name", def.Aliases)
	}
}

// TestProtectedEnvironmentActionMeta_EveryEntry_FillsAllThreeFields is the
// property decorateProtectedEnvironmentMeta assigns unconditionally on: it
// used to guard each field against being empty, and because no entry ever is,
// no test could tell a guard that fired from one that did not. Asserting the
// map directly puts that where it can fail: an entry added without aliases
// or without related actions is reported here instead of silently keeping the
// shared defaults that R-META exists to replace.
func TestProtectedEnvironmentActionMeta_EveryEntry_FillsAllThreeFields(t *testing.T) {
	for tool, meta := range protectedEnvironmentActionMeta {
		t.Run(tool, func(t *testing.T) {
			if meta.usage == "" {
				t.Error("empty usage")
			}
			if len(meta.aliases) < 2 {
				t.Errorf("aliases = %v, want at least two natural-language phrases", meta.aliases)
			}
			if len(meta.related) < 2 {
				t.Errorf("related = %v, want at least two cross-links", meta.related)
			}
		})
	}
}

// TestProtectedEnvironmentMeta_RelatedActions_AreCanonicalCatalogIDs pins the
// cross-links each tool publishes, spelled out rather than counted. Nothing in
// the repository checks a related action against the catalog, and these
// actions are merged into the gitlab_environment group, so a deployment action
// is environment.deployment_list and never deployment.list, which is what
// this list carried until a model following the hint would have been answered
// "unknown action".
func TestProtectedEnvironmentMeta_RelatedActions_AreCanonicalCatalogIDs(t *testing.T) {
	want := map[string][]string{
		"gitlab_protected_environment_list":      {"environment.protected_get", "environment.list", "environment.deployment_list"},
		"gitlab_protected_environment_get":       {"environment.protected_list", "environment.protected_update", "environment.protected_unprotect"},
		"gitlab_protected_environment_protect":   {"environment.protected_get", "environment.protected_update", "environment.protected_unprotect"},
		"gitlab_protected_environment_update":    {"environment.protected_get", "environment.protected_protect", "environment.protected_unprotect"},
		"gitlab_protected_environment_unprotect": {"environment.protected_list", "environment.protected_protect"},
	}
	for tool, related := range want {
		t.Run(tool, func(t *testing.T) {
			if got := protectedEnvironmentOptions(tool).RelatedActions; !slices.Equal(got, related) {
				t.Errorf("RelatedActions = %v, want %v", got, related)
			}
		})
	}
	// The shared default reaches only a tool the map does not name, and it
	// crosses the same group, so it is held to the same spelling.
	if got := protectedEnvironmentOptions("gitlab_unknown_tool").RelatedActions; !slices.Equal(got,
		[]string{"environment.list", "environment.get", "environment.deployment_list"}) {
		t.Errorf("default RelatedActions = %v", got)
	}
}

// TestProtectedEnvironmentMeta_HintsAndSpecsShareOneSetOfActionIDs holds the
// canonical IDs the card and the listing build their hints from to the same
// constants the specs publish. The two lived in separate blocks, one per file,
// which is how a spelling drifts on one side of a package only.
func TestProtectedEnvironmentMeta_HintsAndSpecsShareOneSetOfActionIDs(t *testing.T) {
	want := map[string]string{
		"list":      actionEnvProtectedList,
		"get":       actionEnvProtectedGet,
		"protect":   actionEnvProtectedProtect,
		"update":    actionEnvProtectedUpdate,
		"unprotect": actionEnvProtectedUnprotect,
	}
	specs := ActionSpecs(testutil.NewTestClient(t, protectedEnvironmentsActionHandler()))
	byName := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byName[spec.Name] = spec
	}
	for suffix, id := range want {
		t.Run(suffix, func(t *testing.T) {
			name := "protected_" + suffix
			if _, ok := byName[name]; !ok {
				t.Fatalf("no spec named %q", name)
			}
			if id != "environment."+name {
				t.Errorf("constant = %q, want %q", id, "environment."+name)
			}
		})
	}
}
