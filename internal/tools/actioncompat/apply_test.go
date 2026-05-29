package actioncompat

import (
	"context"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestApplyToGroupSpecs_EmptyInputReturnsNil verifies empty catalog groups stay nil.
func TestApplyToGroupSpecs_EmptyInputReturnsNil(t *testing.T) {
	if groups := ApplyToGroupSpecs(nil); groups != nil {
		t.Fatalf("ApplyToGroupSpecs(nil) = %+v, want nil", groups)
	}
}

// TestApplyToGroupSpecs_ProjectsAllGroups verifies slice-level projection
// forwards each group through ApplyToGroupSpec.
func TestApplyToGroupSpecs_ProjectsAllGroups(t *testing.T) {
	groups := ApplyToGroupSpecs([]actioncatalog.CatalogGroupSpec{
		{ToolName: "gitlab_job", Actions: []toolutil.ActionSpec{toolutil.NewActionSpec("list", testCompatRoute(), toolutil.ActionSpecOptions{})}},
	})
	if len(groups) != 1 {
		t.Fatalf("ApplyToGroupSpecs() returned %d groups, want 1", len(groups))
	}
	if len(groups[0].Actions[0].Compatibility.ActionAliases) != 1 {
		t.Fatalf("action aliases = %+v, want projected alias", groups[0].Actions[0].Compatibility.ActionAliases)
	}
}

// TestApplyToGroupSpec_ClonesAndUsesToolNameDomain verifies group projection clones inputs and falls back to the tool name domain.
func TestApplyToGroupSpec_ClonesAndUsesToolNameDomain(t *testing.T) {
	route := toolutil.ActionRoute{
		Handler:     func(_ context.Context, _ map[string]any) (any, error) { return struct{}{}, nil },
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
		Handler: func(_ context.Context, _ map[string]any) (any, error) { return struct{}{}, nil },
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

// TestApplyToActionSpecs_ProjectsPackageReleaseWorkflowAliases verifies aliases
// for common package-to-release workflow action names are projected into specs.
func TestApplyToActionSpecs_ProjectsPackageReleaseWorkflowAliases(t *testing.T) {
	hasAlias := func(aliases []toolutil.ActionAliasSpec, alias, target string) bool {
		for _, candidate := range aliases {
			if candidate.Alias == alias && candidate.Target == target && candidate.Searchable {
				return true
			}
		}
		return false
	}

	packageSpecs := ApplyToActionSpecs("gitlab_package", "package", []toolutil.ActionSpec{
		toolutil.NewActionSpec("publish_directory", toolutil.ActionRoute{}, toolutil.ActionSpecOptions{}),
	})
	packageAliases := packageSpecs[0].Compatibility.ActionAliases
	for _, alias := range []string{"generic_packages.publish_directory", "gitlab_package/publish_directory"} {
		if !hasAlias(packageAliases, alias, "publish_directory") {
			t.Fatalf("package aliases = %+v, want %s -> publish_directory", packageAliases, alias)
		}
	}

	releaseSpecs := ApplyToActionSpecs("gitlab_release_link", "release", []toolutil.ActionSpec{
		toolutil.NewActionSpec("link_create_batch", toolutil.ActionRoute{}, toolutil.ActionSpecOptions{}),
	})
	releaseAliases := releaseSpecs[0].Compatibility.ActionAliases
	for _, alias := range []string{"release_link.create_batch", "release_link.link_create_batch", "gitlab_release/link_create_batch", "gitlab_release_link/link_create_batch"} {
		if !hasAlias(releaseAliases, alias, "link_create_batch") {
			t.Fatalf("release aliases = %+v, want %s -> link_create_batch", releaseAliases, alias)
		}
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

// TestActionAliasSpecsForAction_Empty verifies no metadata is emitted for
// actions without compatibility aliases.
func TestActionAliasSpecsForAction_Empty(t *testing.T) {
	if aliases := actionAliasSpecsForAction("get", nil); aliases != nil {
		t.Fatalf("actionAliasSpecsForAction(nil) = %+v, want nil", aliases)
	}
}

// TestNormalizeActionAlias_UsesCompatibilityPolicy verifies NormalizeActionAlias uses compatibility policy.
func TestNormalizeActionAlias_UsesCompatibilityPolicy(t *testing.T) {
	if normalized, ok := NormalizeActionAlias(" "); ok || normalized != "" {
		t.Fatalf("NormalizeActionAlias(empty) = %q, %t; want empty false", normalized, ok)
	}
	canonical, ok := NormalizeActionAlias(" FEATURE_FLAG_USER_LIST.CREATE ")
	if !ok || canonical != "feature_flags.ff_user_list_create" {
		t.Fatalf("NormalizeActionAlias() = %q, %t; want feature_flags.ff_user_list_create, true", canonical, ok)
	}
	canonical, ok = NormalizeActionAlias("project.hook_create")
	if !ok || canonical != "project.hook_add" {
		t.Fatalf("NormalizeActionAlias(project.hook_create) = %q, %t; want project.hook_add, true", canonical, ok)
	}
	for alias, want := range map[string]string{
		"generic_packages.publish_directory":    "package.publish_directory",
		"gitlab_package/publish_directory":      "package.publish_directory",
		"gitlab_release/create":                 "release.create",
		"gitlab_release_link/link_create_batch": "release.link_create_batch",
		"group.group_board_list":                "group.epic_board_list",
		"group.epic_discussion_note_update":     "group.epic_discussion_update_note",
		"group.epic_discussion_note_delete":     "group.epic_discussion_delete_note",
		"service_account.delete":                "group.service_account_delete",
		"service_account_pat.revoke":            "group.service_account_pat_revoke",
		"group_service_account.pat_revoke":      "group.service_account_pat_revoke",
		"group_service_account.revoke_pat":      "group.service_account_pat_revoke",
		"merge_request.award_emoji_add":         "merge_request.emoji_mr_create",
		"merge_request.time_spent_set":          "merge_request.spent_time_add",
		"project_service_account.pat_revoke":    "project.service_account_pat_revoke",
		"project_service_account.update":        "project.service_account_update",
		"release_link.create_batch":             "release.link_create_batch",
		"release_link.link_create_batch":        "release.link_create_batch",
		"terraform_state.unlock":                "admin.terraform_state_unlock",
	} {
		canonical, ok = NormalizeActionAlias(alias)
		if !ok || canonical != want {
			t.Fatalf("NormalizeActionAlias(%s) = %q, %t; want %s, true", alias, canonical, ok, want)
		}
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

func testCompatRoute() toolutil.ActionRoute {
	return toolutil.ActionRoute{
		Handler:     func(context.Context, map[string]any) (any, error) { return nil, nil },
		InputSchema: map[string]any{"properties": map[string]any{"scope": map[string]any{}}},
	}
}
