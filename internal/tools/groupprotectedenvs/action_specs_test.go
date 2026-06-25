// action_specs_test.go contains unit tests for the group protected environment [toolutil.ActionSpec] entries.
package groupprotectedenvs

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
		if !byTool[name].ReadOnly {
			t.Errorf("%s should be read-only", name)
		}
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
		desc := groupProtectedEnvDescription(name)
		if !strings.Contains(desc, "Returns:") || !strings.Contains(desc, "See also:") {
			t.Errorf("description for %s = %q, want Returns:/See also: form", name, desc)
		}
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
