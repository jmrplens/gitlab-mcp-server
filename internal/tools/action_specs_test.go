package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

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

func TestIndividualToolProjection_RepresentativeDomainParity(t *testing.T) {
	session := newMCPSession(t, auditHandler(), true)
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}
	toolsByName := make(map[string]*mcp.Tool, len(result.Tools))
	for _, tool := range result.Tools {
		toolsByName[tool.Name] = tool
	}

	specsByTool, err := actionSpecGroupsByTool(CollectActionSpecs(nil, true))
	if err != nil {
		t.Fatalf("actionSpecGroupsByTool() error = %v", err)
	}

	for _, toolName := range []string{"gitlab_project", "gitlab_issue", "gitlab_merge_request", "gitlab_job", "gitlab_group"} {
		t.Run(toolName, func(t *testing.T) {
			for _, spec := range specsByTool[toolName] {
				individualName := strings.TrimSpace(spec.IndividualTool.Name)
				actual, ok := toolsByName[individualName]
				if !ok {
					t.Fatalf("%s.%s individual tool %q is not registered", toolName, spec.Name, individualName)
				}
				projected, projectionErr := toolutil.IndividualToolFromActionSpec(spec, toolutil.IndividualToolProjectionOptions{
					Description: actual.Description,
					Icons:       actual.Icons,
				})
				if projectionErr != nil {
					t.Fatalf("%s.%s projection error = %v", toolName, spec.Name, projectionErr)
				}
				assertProjectedToolParity(t, toolName, spec.Name, actual, projected)
			}
		})
	}
}

func TestIndividualToolMetadata_CatalogBackedCoverage(t *testing.T) {
	session := newMCPSession(t, auditHandler(), true)
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}
	toolsByName := make(map[string]*mcp.Tool, len(result.Tools))
	for _, tool := range result.Tools {
		toolsByName[tool.Name] = tool
	}

	specNames := make(map[string]string)
	duplicateSpecNames := make([]string, 0)
	for _, group := range CollectActionSpecs(nil, true) {
		for _, spec := range group.Specs {
			name := strings.TrimSpace(spec.IndividualTool.Name)
			if name == "" {
				t.Fatalf("%s.%s missing individual tool name", group.ToolName, spec.Name)
			}
			if previous, exists := specNames[name]; exists {
				if _, ok := sharedIndividualToolSpecNames[name]; !ok {
					duplicateSpecNames = append(duplicateSpecNames, fmt.Sprintf("%s => %s, %s.%s", name, previous, group.ToolName, spec.Name))
				}
			} else {
				specNames[name] = group.ToolName + "." + spec.Name
			}
			if _, ok := toolsByName[name]; !ok {
				t.Fatalf("%s.%s references unregistered individual tool %q", group.ToolName, spec.Name, name)
			}
		}
	}
	if len(duplicateSpecNames) > 0 {
		sort.Strings(duplicateSpecNames)
		t.Fatalf("unexpected shared individual tool references: %v", duplicateSpecNames)
	}

	missingSpecs := make([]string, 0)
	for _, tool := range result.Tools {
		if _, ok := specNames[tool.Name]; ok {
			continue
		}
		if _, ok := standaloneIndividualToolExceptions[tool.Name]; ok {
			continue
		}
		missingSpecs = append(missingSpecs, tool.Name)
	}
	sort.Strings(missingSpecs)
	if len(missingSpecs) > 0 {
		t.Fatalf("individual tools missing ActionSpec metadata: %v", missingSpecs)
	}
}

var standaloneIndividualToolExceptions = map[string]string{
	"gitlab_discover_project":           "dynamic standalone project discovery helper",
	"gitlab_interactive_issue_create":   "elicitation standalone multi-step workflow",
	"gitlab_interactive_mr_create":      "elicitation standalone multi-step workflow",
	"gitlab_interactive_project_create": "elicitation standalone multi-step workflow",
	"gitlab_interactive_release_create": "elicitation standalone multi-step workflow",
	"gitlab_server_status":              "server diagnostic helper outside the GitLab API catalog",
}

var sharedIndividualToolSpecNames = map[string]string{
	"gitlab_commit_list":      "shared by gitlab_repository.commit_list and gitlab_repository.file_history",
	"gitlab_issue_list_group": "shared by gitlab_group.issues and gitlab_issue.list_group",
	"gitlab_user_current":     "shared by gitlab_user.current and gitlab_user.me",
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

func assertProjectedToolParity(t *testing.T, toolName, actionName string, actual, projected *mcp.Tool) {
	t.Helper()
	if projected.Name != actual.Name {
		t.Fatalf("%s.%s projected name = %q, want %q", toolName, actionName, projected.Name, actual.Name)
	}
	if projected.Title != actual.Title {
		t.Fatalf("%s.%s projected title = %q, want %q", toolName, actionName, projected.Title, actual.Title)
	}
	if projected.Description != actual.Description {
		t.Fatalf("%s.%s projected description drift", toolName, actionName)
	}
	if projected.InputSchema == nil {
		t.Fatalf("%s.%s projected input schema is nil", toolName, actionName)
	}
	if projected.OutputSchema == nil {
		t.Fatalf("%s.%s projected output schema is nil", toolName, actionName)
	}
	assertProjectedToolAnnotations(t, toolName, actionName, projected.Annotations)
	assertToolIconsParity(t, toolName, actionName, actual.Icons, projected.Icons)
}

func assertProjectedToolAnnotations(t *testing.T, toolName, actionName string, projected *mcp.ToolAnnotations) {
	t.Helper()
	if projected == nil {
		t.Fatalf("%s.%s projected annotations are nil", toolName, actionName)
	}
	if projected.DestructiveHint == nil {
		t.Fatalf("%s.%s projected destructive annotation is nil", toolName, actionName)
	}
	if projected.OpenWorldHint == nil {
		t.Fatalf("%s.%s projected open-world annotation is nil", toolName, actionName)
	}
	if projected.ReadOnlyHint && *projected.DestructiveHint {
		t.Fatalf("%s.%s projected annotations are both read-only and destructive", toolName, actionName)
	}
}

func assertToolIconsParity(t *testing.T, toolName, actionName string, actual, projected []mcp.Icon) {
	t.Helper()
	if len(projected) != len(actual) {
		t.Fatalf("%s.%s projected icon count = %d, want %d", toolName, actionName, len(projected), len(actual))
	}
	for i := range actual {
		if projected[i].Source != actual[i].Source || projected[i].MIMEType != actual[i].MIMEType || strings.Join(projected[i].Sizes, ",") != strings.Join(actual[i].Sizes, ",") || projected[i].Theme != actual[i].Theme {
			t.Fatalf("%s.%s projected icon[%d] = %+v, want %+v", toolName, actionName, i, projected[i], actual[i])
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
