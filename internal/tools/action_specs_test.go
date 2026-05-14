package tools

import (
	"sort"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

func TestCollectedActionSpecs_MigratedMetaToolParity(t *testing.T) {
	captured := toolutil.CaptureMetaToolDefinitions(func() {
		registerAccessMeta(nil, nil)
		registerBranchMeta(nil, nil)
		registerCICatalogMeta(nil, nil)
		registerCIVariableMeta(nil, nil)
		registerCustomEmojiMeta(nil, nil)
		registerEnvironmentMeta(nil, nil)
		registerFeatureFlagsMeta(nil, nil)
		registerJobMeta(nil, nil)
		registerModelRegistryMeta(nil, nil)
		registerMRReviewMeta(nil, nil)
		registerPackageMeta(nil, nil)
		registerPipelineMeta(nil, nil)
		registerProjectMeta(nil, nil, false)
		registerRepositoryMeta(nil, nil)
		registerReleaseMeta(nil, nil)
		registerTagMeta(nil, nil)
		registerSnippetMeta(nil, nil)
		registerTemplateMeta(nil, nil)
		registerWikiMeta(nil, nil)
	})
	capturedByTool := make(map[string]toolutil.MetaToolDefinition, len(captured))
	for _, definition := range captured {
		capturedByTool[definition.Name] = definition
	}

	specsByTool, err := actionSpecGroupsByTool(CollectActionSpecs(nil, false))
	if err != nil {
		t.Fatalf("actionSpecGroupsByTool() error = %v", err)
	}

	for _, toolName := range []string{"gitlab_access", "gitlab_branch", "gitlab_ci_catalog", "gitlab_ci_variable", "gitlab_custom_emoji", "gitlab_environment", "gitlab_feature_flags", "gitlab_job", "gitlab_model_registry", "gitlab_mr_review", "gitlab_package", "gitlab_pipeline", "gitlab_project", "gitlab_release", "gitlab_repository", "gitlab_snippet", "gitlab_tag", "gitlab_template", "gitlab_wiki"} {
		t.Run(toolName, func(t *testing.T) {
			definition, ok := capturedByTool[toolName]
			if !ok {
				t.Fatalf("captured meta definitions missing %s", toolName)
			}
			specRoutes, routeErr := toolutil.ActionSpecsToMapWithError(specsByTool[toolName])
			if routeErr != nil {
				t.Fatalf("ActionSpecsToMapWithError() error = %v", routeErr)
			}
			assertActionRouteParity(t, toolName, definition.Routes, specRoutes)
		})
	}
}

func assertActionRouteParity(t *testing.T, toolName string, captured, specRoutes toolutil.ActionMap) {
	t.Helper()
	if len(specRoutes) != len(captured) {
		t.Fatalf("%s specs route count = %d, want %d; missing: %v", toolName, len(specRoutes), len(captured), missingRouteNames(captured, specRoutes))
	}
	for actionName, capturedRoute := range captured {
		specRoute, ok := specRoutes[actionName]
		if !ok {
			t.Fatalf("%s spec routes missing action %q", toolName, actionName)
		}
		if specRoute.Destructive != capturedRoute.Destructive {
			t.Fatalf("%s.%s destructive = %t, want %t", toolName, actionName, specRoute.Destructive, capturedRoute.Destructive)
		}
		if specRoute.InputSchema == nil {
			t.Fatalf("%s.%s missing input schema", toolName, actionName)
		}
		if specRoute.OutputSchema == nil {
			t.Fatalf("%s.%s missing output schema", toolName, actionName)
		}
	}
}

func missingRouteNames(want, got toolutil.ActionMap) []string {
	missing := make([]string, 0)
	for actionName := range want {
		if _, ok := got[actionName]; !ok {
			missing = append(missing, actionName)
		}
	}
	sort.Strings(missing)
	return missing
}
