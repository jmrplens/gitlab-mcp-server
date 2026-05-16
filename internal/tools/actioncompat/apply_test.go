package actioncompat

import (
	"context"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestApplyToGroupSpecs_EmptyInputReturnsNil verifies empty catalog groups stay nil.
func TestApplyToGroupSpecs_EmptyInputReturnsNil(t *testing.T) {
	if groups := ApplyToGroupSpecs(nil); groups != nil {
		t.Fatalf("ApplyToGroupSpecs(nil) = %+v, want nil", groups)
	}
}

// TestApplyToGroupSpec_ClonesAndUsesToolNameDomain verifies group projection clones inputs and falls back to the tool name domain.
func TestApplyToGroupSpec_ClonesAndUsesToolNameDomain(t *testing.T) {
	route := toolutil.ActionRoute{
		Handler:     func(_ context.Context, _ map[string]any) (any, error) { return nil, nil },
		InputSchema: map[string]any{"properties": map[string]any{"scope": map[string]any{}}},
	}
	original := actioncatalog.CatalogGroupSpec{
		ToolName: "gitlab_job",
		Actions:  []toolutil.ActionSpec{toolutil.NewActionSpec("list", route, toolutil.ActionSpecOptions{})},
	}

	projected := ApplyToGroupSpec(original)
	if len(projected.Actions) != 1 {
		t.Fatalf("projected actions = %d, want 1", len(projected.Actions))
	}
	if len(projected.Actions[0].Compatibility.ActionAliases) != 1 {
		t.Fatalf("action aliases = %+v, want pipeline.jobs compatibility alias", projected.Actions[0].Compatibility.ActionAliases)
	}
	if len(original.Actions[0].Compatibility.ActionAliases) != 0 {
		t.Fatalf("original action aliases = %+v, want original group unchanged", original.Actions[0].Compatibility.ActionAliases)
	}
}

// TestApplyToActionSpecs_ProjectsCompatibilityMetadata verifies ApplyToActionSpecs projects compatibility metadata.
func TestApplyToActionSpecs_ProjectsCompatibilityMetadata(t *testing.T) {
	route := toolutil.ActionRoute{
		Handler: func(_ context.Context, _ map[string]any) (any, error) { return nil, nil },
		InputSchema: map[string]any{"properties": map[string]any{
			"project_id": map[string]any{},
			"scope":      map[string]any{},
		}},
	}
	specs := ApplyToActionSpecs("gitlab_job", "job", []toolutil.ActionSpec{
		toolutil.NewActionSpec("list", route, toolutil.ActionSpecOptions{}),
	})
	if len(specs) != 1 {
		t.Fatalf("ApplyToActionSpecs() returned %d specs, want 1", len(specs))
	}
	compatibility := specs[0].Compatibility
	if len(compatibility.ActionAliases) != 1 || compatibility.ActionAliases[0].Alias != "pipeline.jobs" || compatibility.ActionAliases[0].Target != "list" {
		t.Fatalf("action aliases = %+v, want pipeline.jobs -> list", compatibility.ActionAliases)
	}
	if len(compatibility.ParameterAliases) != 1 || compatibility.ParameterAliases[0].Alias != "status" || compatibility.ParameterAliases[0].Target != "scope" {
		t.Fatalf("parameter aliases = %+v, want status -> scope", compatibility.ParameterAliases)
	}
}

// TestApplyToActionSpecs_EmptyInputReturnsNil verifies action projection keeps empty specs nil.
func TestApplyToActionSpecs_EmptyInputReturnsNil(t *testing.T) {
	if specs := ApplyToActionSpecs("gitlab_job", "job", nil); specs != nil {
		t.Fatalf("ApplyToActionSpecs(nil) = %+v, want nil", specs)
	}
}

// TestApplyToActionSpecs_PreservesUnsearchableActionAlias verifies ApplyToActionSpecs preserves unsearchable action alias.
func TestApplyToActionSpecs_PreservesUnsearchableActionAlias(t *testing.T) {
	specs := ApplyToActionSpecs("gitlab_repository", "repository", []toolutil.ActionSpec{
		toolutil.NewActionSpec("tree", toolutil.ActionRoute{}, toolutil.ActionSpecOptions{}),
	})
	aliases := specs[0].Compatibility.ActionAliases
	if len(aliases) != 2 {
		t.Fatalf("action aliases = %+v, want repository_tree aliases", aliases)
	}
	for _, alias := range aliases {
		if alias.Searchable {
			t.Fatalf("alias = %+v, want unsearchable repository tree compatibility alias", alias)
		}
		if alias.Reason == "" {
			t.Fatalf("alias = %+v, want reason", alias)
		}
	}
}

// TestNormalizeActionAlias_UsesCompatibilityPolicy verifies NormalizeActionAlias uses compatibility policy.
func TestNormalizeActionAlias_UsesCompatibilityPolicy(t *testing.T) {
	canonical, ok := NormalizeActionAlias(" FEATURE_FLAG_USER_LIST.CREATE ")
	if !ok || canonical != "feature_flags.ff_user_list_create" {
		t.Fatalf("NormalizeActionAlias() = %q, %t; want feature_flags.ff_user_list_create, true", canonical, ok)
	}
	unchanged, aliasOK := NormalizeActionAlias("project.get")
	if aliasOK || unchanged != "project.get" {
		t.Fatalf("NormalizeActionAlias(project.get) = %q, %t; want unchanged false", unchanged, aliasOK)
	}
}

// TestNormalizeParamsWithExplanation_AppliesActionScopedPolicy verifies NormalizeParamsWithExplanation applies action scoped policy.
func TestNormalizeParamsWithExplanation_AppliesActionScopedPolicy(t *testing.T) {
	schema := map[string]any{"properties": map[string]any{"project_id": map[string]any{}, "ref": map[string]any{}}}
	normalized, explanations := NormalizeParamsWithExplanation("repository.file_get", map[string]any{"project_id": 1, "branch": "main"}, schema)
	if normalized["ref"] != "main" {
		t.Fatalf("normalized params = %+v, want branch copied to ref", normalized)
	}
	if _, hasBranch := normalized["branch"]; hasBranch {
		t.Fatalf("normalized params = %+v, want branch removed", normalized)
	}
	if len(explanations) != 1 || explanations[0].Alias != "branch" || explanations[0].Canonical != "ref" {
		t.Fatalf("explanations = %+v, want branch -> ref", explanations)
	}
}
