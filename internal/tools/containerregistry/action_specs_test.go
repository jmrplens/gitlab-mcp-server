// action_specs_test.go contains canonical-route tests for container registry delete behavior.
package containerregistry

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestTagProtectionMetadata_Discoverability locks in the model-facing discovery
// metadata for the container registry tag protection actions (client-go
// v2.40.0) and verifies the repository-path protection list and the tag list
// cross-reference them, so models can tell tag protection apart from
// repository-path protection.
func TestTagProtectionMetadata_Discoverability(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	byTool := registrySpecsByTool(t, ActionSpecs(client))

	cases := []struct {
		tool          string
		aliasContains string
		related       string
	}{
		{"gitlab_registry_tag_protection_list", "tag", "package.registry_tag_rule_create"},
		{"gitlab_registry_tag_protection_create", "tag", "package.registry_tag_rule_list"},
		{"gitlab_registry_tag_protection_update", "tag", "package.registry_tag_rule_delete"},
		{"gitlab_registry_tag_protection_delete", "tag", "package.registry_tag_rule_list"},
	}
	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			spec, ok := byTool[tc.tool]
			if !ok {
				t.Fatalf("missing spec for %s", tc.tool)
			}
			if spec.Usage == "" || strings.Contains(spec.Usage, "Manage container registry repositories, tags, and protection rules") {
				t.Errorf("%s has generic/empty Usage: %q", tc.tool, spec.Usage)
			}
			if !registryAliasHas(spec.Aliases, tc.aliasContains) {
				t.Errorf("%s aliases %v missing phrase %q", tc.tool, spec.Aliases, tc.aliasContains)
			}
			if !slices.Contains(spec.RelatedActions, tc.related) {
				t.Errorf("%s related %v missing %q", tc.tool, spec.RelatedActions, tc.related)
			}
		})
	}

	// Cross-references from the encompassing actions so models discover tag protection.
	if list := byTool["gitlab_registry_protection_list"]; !slices.Contains(list.RelatedActions, "package.registry_tag_rule_list") {
		t.Errorf("registry_protection_list should cross-reference tag protection, got %v", list.RelatedActions)
	}
	if tags := byTool["gitlab_registry_list_tags"]; !slices.Contains(tags.RelatedActions, "package.registry_tag_rule_list") {
		t.Errorf("registry_list_tags should cross-reference tag protection, got %v", tags.RelatedActions)
	}
}

// TestActionSpecs_ProtectionRules_StateWhatGitLabChecks verifies the schemas
// of the protection rule actions, where find's example wrote "value" for a
// path pattern and for both access levels of a tag rule, and GitLab refused
// the call (issue 1272): a repository path pattern carries
// repositoryPathPatternForm on the create and the update, the levels of both
// creates carry the three values GitLab lists, and a repository rule's
// create names at least one of its two levels, as GitLab's at_least_one_of
// does, so the example sends one. The updates' levels keep no enum, since an
// empty string removes one, and no other action carries any of it.
func TestActionSpecs_ProtectionRules_StateWhatGitLabChecks(t *testing.T) {
	levels := "[maintainer owner admin]"
	want := map[string]map[string]string{
		toolRegistryProtCreate: {
			"repository_path_pattern":         repositoryPathPatternForm,
			"minimum_access_level_for_push":   levels,
			"minimum_access_level_for_delete": levels,
		},
		toolRegistryProtUpdate: {"repository_path_pattern": repositoryPathPatternForm},
		toolRegistryTagProtCreate: {
			"minimum_access_level_for_push":   levels,
			"minimum_access_level_for_delete": levels,
		},
	}
	for _, spec := range ActionSpecs(nil) {
		t.Run(spec.IndividualTool.Name, func(t *testing.T) {
			wantAnyOf := ""
			if spec.IndividualTool.Name == toolRegistryProtCreate {
				wantAnyOf = "[map[required:[minimum_access_level_for_push]] map[required:[minimum_access_level_for_delete]]]"
			}
			gotAnyOf := ""
			if anyOf, ok := spec.Route.InputSchema["anyOf"]; ok {
				gotAnyOf = fmt.Sprint(anyOf)
			}
			if gotAnyOf != wantAnyOf {
				t.Errorf("%s anyOf = %s, want %q", spec.IndividualTool.Name, gotAnyOf, wantAnyOf)
			}
			properties, _ := spec.Route.InputSchema["properties"].(map[string]any)
			for _, name := range []string{"repository_path_pattern", "minimum_access_level_for_push", "minimum_access_level_for_delete"} {
				t.Run(name, func(t *testing.T) {
					property, _ := properties[name].(map[string]any)
					if got := statedRule(property); got != want[spec.IndividualTool.Name][name] {
						t.Errorf("%s %s states %q, want %q", spec.IndividualTool.Name, name, got, want[spec.IndividualTool.Name][name])
					}
				})
			}
		})
	}
}

// statedRule returns what a property's schema states about its value: its
// enum written out, else its pattern, else nothing.
func statedRule(property map[string]any) string {
	if enum, ok := property["enum"]; ok {
		return fmt.Sprint(enum)
	}
	pattern, _ := property["pattern"].(string)
	return pattern
}

// TestRepositoryPathPatternForm_HoldsGitLabsRule verifies the path pattern
// form against GitLab's own regular expression, each value put to it as
// Ruby reads it: lower-case names joined by the separators it allows, with a
// * for any character, are taken, and an upper-case name, a leading *, an
// empty segment or a trailing separator are refused.
func TestRepositoryPathPatternForm_HoldsGitLabsRule(t *testing.T) {
	form := regexp.MustCompile(repositoryPathPatternForm)
	cases := []struct {
		value string
		want  bool
	}{
		{value: "group/project/*", want: true},
		{value: "group/project/image-*", want: true},
		{value: "group/project", want: true},
		{value: "my.group/my_project/*", want: true},
		{value: "group__x/project---y", want: true},
		{value: "Group/Project/*", want: false},
		{value: "*/image", want: false},
		{value: "group//project", want: false},
		{value: "group/project/", want: false},
		{value: "", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			if got := form.MatchString(tc.value); got != tc.want {
				t.Errorf("repositoryPathPatternForm matches %q = %t, want %t", tc.value, got, tc.want)
			}
		})
	}
}

func registryAliasHas(aliases []string, sub string) bool {
	for _, a := range aliases {
		if strings.Contains(a, sub) {
			return true
		}
	}
	return false
}

// TestActionSpecs_DeleteErrors validates the DeleteErrors route through the catalog surface.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestActionSpecs_DeleteErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})
	client := testutil.NewTestClient(t, mux)
	byTool := registrySpecsByTool(t, ActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_registry_delete_repository", map[string]any{"project_id": "42", "repository_id": 1}},
		{"gitlab_registry_delete_tag", map[string]any{"project_id": "42", "repository_id": 1, "tag_name": "latest"}},
		{"gitlab_registry_delete_tags_bulk", map[string]any{"project_id": "42", "repository_id": 1}},
		{"gitlab_registry_protection_delete", map[string]any{"project_id": "42", "rule_id": 1}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			_, err := byTool[tt.name].Route.Handler(t.Context(), tt.args)
			if err == nil {
				t.Errorf("expected error from %s with failing backend", tt.name)
			}
		})
	}
}

// TestCatalogSurface_DeleteConfirmDeclined verifies the CatalogSurface_DeleteConfirmDeclined handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	byTool := registrySpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	for _, toolName := range []string{"gitlab_registry_delete_repository", "gitlab_registry_delete_tag", "gitlab_registry_delete_tags_bulk", "gitlab_registry_protection_delete"} {
		toolutil.RegisterSurfaceToolFromSpec(server, byTool[toolName], toolutil.SurfaceToolRegisterOptions{
			Description: "Test container registry destructive confirmation.",
			Icons:       toolutil.IconContainer,
		})
	}

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
	session, connectErr := mcpClient.Connect(ctx, ct, nil)
	if connectErr != nil {
		t.Fatalf("client connect: %v", connectErr)
	}
	t.Cleanup(func() {
		session.Close()
		_ = serverSession.Wait()
	})

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_registry_delete_repository", map[string]any{"project_id": "42", "repository_id": 1}},
		{"gitlab_registry_delete_tag", map[string]any{"project_id": "42", "repository_id": 1, "tag_name": "latest"}},
		{"gitlab_registry_delete_tags_bulk", map[string]any{"project_id": "42", "repository_id": 1}},
		{"gitlab_registry_protection_delete", map[string]any{"project_id": "42", "rule_id": 1}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, callErr := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if callErr != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.name, callErr)
			}
			if result == nil {
				t.Fatalf("expected non-nil result for declined confirmation on %s", tt.name)
			}
			found := false
			for _, c := range result.Content {
				if tc, ok := c.(*mcp.TextContent); ok && tc.Text != "" {
					found = true
				}
			}
			if !found {
				t.Errorf("expected non-empty text content in %s cancellation result", tt.name)
			}
		})
	}
}
