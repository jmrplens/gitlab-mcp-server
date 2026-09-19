// action_specs_test.go contains integration tests for the security settings tool
// closures in ActionSpecs routes with a mock GitLab API.
package securitysettings

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const registerProjectSecJSON = `{
	"project_id": 42,
	"auto_fix_container_scanning": false,
	"auto_fix_dast": false,
	"auto_fix_dependency_scanning": true,
	"auto_fix_sast": true,
	"continuous_vulnerability_scans_enabled": false,
	"container_scanning_for_registry_enabled": true,
	"secret_push_protection_enabled": true
}`

const registerGroupSecJSON = `{
	"secret_push_protection_enabled": true
}`

// TestActionSpecs_Metadata verifies security settings action spec metadata.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := append(ProjectActionSpecs(client), GroupActionSpecs(client)...)
	if len(specs) != 3 {
		t.Fatalf("len(ActionSpecs) = %d, want 3", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "securitysettings" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
	}
}

// TestActionSpecs_DiscoveryMetadata verifies the R-META discovery metadata for
// each security settings tool: action-specific Usage, distinctive
// natural-language Aliases beyond the tool name, canonical RelatedActions, and a
// "Returns: … See also: …" IndividualTool.Description.
func TestActionSpecs_DiscoveryMetadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := append(ProjectActionSpecs(client), GroupActionSpecs(client)...)
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}

	for _, tool := range []string{
		"gitlab_get_project_security_settings",
		"gitlab_update_project_secret_push_protection",
		"gitlab_update_group_secret_push_protection",
	} {
		t.Run(tool, func(t *testing.T) {
			spec, ok := specByTool[tool]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tool)
			}
			if strings.TrimSpace(spec.Usage) == "" {
				t.Errorf("%s: empty Usage", tool)
			}
			if len(spec.RelatedActions) == 0 {
				t.Errorf("%s: empty RelatedActions", tool)
			}
			distinct := false
			for _, a := range spec.Aliases {
				if a != tool {
					distinct = true
					break
				}
			}
			if !distinct {
				t.Errorf("%s: aliases only contain the tool name: %v", tool, spec.Aliases)
			}
			desc := spec.IndividualTool.Description
			if !strings.Contains(desc, "Returns:") || !strings.Contains(desc, "See also:") {
				t.Errorf("%s: description missing Returns:/See also: form: %q", tool, desc)
			}
		})
	}
}

// TestActionSpecs_RelatedActions_NameTheSecuritySettingsSiblings verifies each
// tool publishes the canonical action IDs a model should reach for next, and
// asserts the whole list rather than that one entry is present.
//
// Nothing in the repository checks that an ID here names an action the catalog
// holds: the discovery audit only counts an empty list, so a misspelling ships
// green and answers the model "unknown action" the moment it follows the hint.
func TestActionSpecs_RelatedActions_NameTheSecuritySettingsSiblings(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specByTool := securitySettingsSpecsByTool(append(ProjectActionSpecs(client), GroupActionSpecs(client)...))

	tests := []struct {
		tool    string
		related []string
	}{
		{"gitlab_get_project_security_settings", []string{"project.get", "project.security_settings_update"}},
		{"gitlab_update_project_secret_push_protection", []string{"project.security_settings_get"}},
		{"gitlab_update_group_secret_push_protection", []string{"group.get", "project.security_settings_get"}},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			if got := specByTool[tt.tool].RelatedActions; !slices.Equal(got, tt.related) {
				t.Errorf("RelatedActions = %v, want %v", got, tt.related)
			}
		})
	}
}

// TestMarkdownActionConstants_NameTheActionsThisPackageRegisters verifies the
// three IDs the cards build their hints from are the domain of the catalog
// group each spec joins plus the spec's own name.
//
// They are two blocks of literals — the constants in markdown.go and the
// action names in this file — with nothing between them, so renaming an action
// would leave a card pointing a model at an ID the catalog no longer holds.
// The domains stay literal because they are the group the aggregation appends
// each set to, gitlab_project and gitlab_group, which this package cannot see.
func TestMarkdownActionConstants_NameTheActionsThisPackageRegisters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specByTool := securitySettingsSpecsByTool(append(ProjectActionSpecs(client), GroupActionSpecs(client)...))

	tests := []struct {
		constant string
		domain   string
		tool     string
	}{
		{actionProjectGet, "project", "gitlab_get_project_security_settings"},
		{actionProjectUpdate, "project", "gitlab_update_project_secret_push_protection"},
		{actionGroupUpdate, "group", "gitlab_update_group_secret_push_protection"},
	}
	for _, tt := range tests {
		t.Run(tt.constant, func(t *testing.T) {
			spec, ok := specByTool[tt.tool]
			if !ok {
				t.Fatalf("no ActionSpec registers %s", tt.tool)
			}
			if want := tt.domain + "." + spec.Name; tt.constant != want {
				t.Errorf("markdown hint constant %q, want %q", tt.constant, want)
			}
		})
	}
}

// TestActionSpecs_Edition_EveryActionIsUltimate verifies both scopes declare
// the tier secret push protection actually needs. The catalog aggregation
// overwrites this field, so a wrong value here is invisible at runtime and
// stays wrong until the day that overwrite goes away.
func TestActionSpecs_Edition_EveryActionIsUltimate(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for tool, spec := range securitySettingsSpecsByTool(append(ProjectActionSpecs(client), GroupActionSpecs(client)...)) {
		t.Run(tool, func(t *testing.T) {
			if spec.Edition != "ultimate" {
				t.Errorf("Edition = %q, want %q", spec.Edition, "ultimate")
			}
		})
	}
}

// securitySettingsSpecsByTool indexes the package's specs by the individual
// tool name each projects.
func securitySettingsSpecsByTool(specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}

// TestActionSpecs_CallRoutes verifies all registered security settings routes execute successfully.
func TestActionSpecs_CallRoutes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			testutil.RespondJSON(w, http.StatusOK, registerProjectSecJSON)
		case http.MethodPatch, http.MethodPut:
			testutil.RespondJSON(w, http.StatusOK, registerGroupSecJSON)
		default:
			http.NotFound(w, r)
		}
	})
	client := testutil.NewTestClient(t, mux)
	specs := append(ProjectActionSpecs(client), GroupActionSpecs(client)...)
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_get_project_security_settings", map[string]any{"project_id": "42"}},
		{"gitlab_update_project_secret_push_protection", map[string]any{"project_id": "42", "secret_push_protection_enabled": true}},
		{"gitlab_update_group_secret_push_protection", map[string]any{"group_id": "my-group", "secret_push_protection_enabled": true}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := specByTool[tt.name]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tt.name)
			}
			result, err := spec.Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.name)
			}
		})
	}
}

// TestToProjectOutput_Nil verifies that toProjectOutput handles nil input
// gracefully by returning a zero-value ProjectOutput.
func TestToProjectOutput_Nil(t *testing.T) {
	out := toProjectOutput(nil)
	if out.ProjectID != 0 {
		t.Errorf("expected zero ProjectID for nil input, got %d", out.ProjectID)
	}
}

// TestToGroupOutput_Nil verifies that toGroupOutput handles nil input
// gracefully by returning a zero-value GroupOutput.
func TestToGroupOutput_Nil(t *testing.T) {
	out := toGroupOutput(nil)
	if out.SecretPushProtectionEnabled {
		t.Error("expected false SecretPushProtectionEnabled for nil input")
	}
	if len(out.Errors) != 0 {
		t.Errorf("expected empty errors for nil input, got %d", len(out.Errors))
	}
}
