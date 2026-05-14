package tools

import (
	"sort"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

func TestCollectedActionSpecs_MigratedMetaToolParity(t *testing.T) {
	testCases := []struct {
		name       string
		client     *gitlabclient.Client
		enterprise bool
	}{
		{name: "base"},
		{name: "self-managed enterprise", enterprise: true},
		{name: "gitlab.com enterprise", client: newGitLabDotComClient(t), enterprise: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			captured := toolutil.CaptureMetaToolDefinitions(func() {
				registerAllMetaGroups(nil, tc.client, tc.enterprise)
			})
			capturedByTool := make(map[string]toolutil.MetaToolDefinition, len(captured))
			for _, definition := range captured {
				capturedByTool[definition.Name] = definition
			}

			specsByTool, err := actionSpecGroupsByTool(CollectActionSpecs(tc.client, tc.enterprise))
			if err != nil {
				t.Fatalf("actionSpecGroupsByTool() error = %v", err)
			}

			toolNames := make([]string, 0, len(capturedByTool))
			for toolName := range capturedByTool {
				toolNames = append(toolNames, toolName)
			}
			sort.Strings(toolNames)

			for _, toolName := range toolNames {
				t.Run(toolName, func(t *testing.T) {
					definition := capturedByTool[toolName]
					specs, ok := specsByTool[toolName]
					if !ok {
						t.Fatalf("collected action specs missing %s", toolName)
					}
					specRoutes, routeErr := toolutil.ActionSpecsToMapWithError(specs)
					if routeErr != nil {
						t.Fatalf("ActionSpecsToMapWithError() error = %v", routeErr)
					}
					assertActionRouteParity(t, toolName, definition.Routes, specRoutes)
					assertSpecProjectionParity(t, toolName, specs)
				})
			}
		})
	}
}

func TestCollectedActionSpecs_KnownGuidancePreserved(t *testing.T) {
	specsByTool, err := actionSpecGroupsByTool(CollectActionSpecs(newGitLabDotComClient(t), true))
	if err != nil {
		t.Fatalf("actionSpecGroupsByTool() error = %v", err)
	}

	testCases := []struct {
		toolName string
		action   string
		keys     []string
	}{
		{toolName: "gitlab_merge_request", action: "create", keys: []string{"source_branch", "target_branch"}},
		{toolName: "gitlab_issue", action: "link_create", keys: []string{"project_id", "issue_iid", "target_project_id", "target_issue_iid"}},
		{toolName: "gitlab_group", action: "epic_issue_assign", keys: []string{"full_path", "child_project_path", "child_iid"}},
		{toolName: "gitlab_job", action: "token_scope_remove_project", keys: []string{"project_id", "target_project_id"}},
		{toolName: "gitlab_access", action: "deploy_token_delete_project", keys: []string{"project_id", "deploy_token_id"}},
	}

	for _, tc := range testCases {
		t.Run(tc.toolName+"/"+tc.action, func(t *testing.T) {
			routes, routeErr := toolutil.ActionSpecsToMapWithError(specsByTool[tc.toolName])
			if routeErr != nil {
				t.Fatalf("ActionSpecsToMapWithError() error = %v", routeErr)
			}
			route, ok := routes[tc.action]
			if !ok {
				t.Fatalf("%s specs missing action %q", tc.toolName, tc.action)
			}
			assertGuidanceKeys(t, tc.toolName, tc.action, route.ParameterGuidance, tc.keys)
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

func assertSpecProjectionParity(t *testing.T, toolName string, specs []toolutil.ActionSpec) {
	t.Helper()
	group, err := actioncatalog.GroupFromSpecs(actioncatalog.GroupOptions{ToolName: toolName}, specs)
	if err != nil {
		t.Fatalf("GroupFromSpecs() error = %v", err)
	}
	if len(group.Actions) != len(specs) {
		t.Fatalf("%s projected action count = %d, want %d", toolName, len(group.Actions), len(specs))
	}
	for _, spec := range specs {
		action, ok := group.Actions[spec.Name]
		if !ok {
			t.Fatalf("%s projection missing action %q", toolName, spec.Name)
		}
		if !action.SpecBacked {
			t.Fatalf("%s.%s projection is not spec-backed", toolName, spec.Name)
		}
		if action.ReadOnly != spec.ReadOnly {
			t.Fatalf("%s.%s read-only = %t, want %t", toolName, spec.Name, action.ReadOnly, spec.ReadOnly)
		}
		if strings.TrimSpace(spec.IndividualTool.Name) == "" {
			t.Fatalf("%s.%s missing individual tool metadata", toolName, spec.Name)
		}
	}
}

func assertGuidanceKeys(t *testing.T, toolName, actionName string, guidance map[string]toolutil.ParameterGuidance, want []string) {
	t.Helper()
	got := make([]string, 0, len(guidance))
	for key := range guidance {
		got = append(got, key)
	}
	sort.Strings(got)
	want = append([]string(nil), want...)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("%s.%s guidance keys = %v, want %v", toolName, actionName, got, want)
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
