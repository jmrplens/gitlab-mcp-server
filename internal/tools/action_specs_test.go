package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/dynamic"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestCollectedActionSpecs_ProjectIntoActionCatalog covers CollectedActionSpecs with table-driven subtests for project into action catalog.
func TestCollectedActionSpecs_ProjectIntoActionCatalog(t *testing.T) {
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
			catalog, catalogErr := BuildActionCatalog(tc.client, ActionCatalogOptions{Enterprise: tc.enterprise})
			if catalogErr != nil {
				t.Fatalf("BuildActionCatalog() error = %v", catalogErr)
			}

			for _, specGroup := range CollectActionSpecs(tc.client, tc.enterprise) {
				t.Run(specGroup.ToolName, func(t *testing.T) {
					catalogGroup, ok := catalog.Group(specGroup.ToolName)
					if !ok {
						t.Fatalf("catalog missing group %s", specGroup.ToolName)
					}
					specRoutes, routeErr := toolutil.ActionSpecsToMapWithError(specGroup.Actions)
					if routeErr != nil {
						t.Fatalf("ActionSpecsToMapWithError() error = %v", routeErr)
					}
					assertActionRouteParity(t, specGroup.ToolName, specRoutes, catalogGroup.ActionMap())
					assertSpecProjectionParity(t, specGroup.ToolName, specGroup.Actions)
				})
			}
		})
	}
}

// TestCollectedActionSpecs_KnownGuidancePreserved covers CollectedActionSpecs with table-driven subtests for known guidance preserved.
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

// TestCollectedActionSpecs_DeclareCatalogOwnership verifies CollectedActionSpecs when declare catalog ownership.
func TestCollectedActionSpecs_DeclareCatalogOwnership(t *testing.T) {
	owners := sourceToolPackageNames(t)
	owners["tools"] = struct{}{}
	var missingGroupOwners []string
	var unknownGroupOwners []string
	var missingActionOwners []string
	var unknownActionOwners []string

	for _, group := range CollectActionSpecs(newGitLabDotComClient(t), true) {
		groupOwner := strings.TrimSpace(group.OwnerPackage)
		if groupOwner == "" {
			missingGroupOwners = append(missingGroupOwners, group.ToolName)
		} else if _, ok := owners[groupOwner]; !ok {
			unknownGroupOwners = append(unknownGroupOwners, fmt.Sprintf("%s owner %s", group.ToolName, groupOwner))
		}
		for _, spec := range group.Actions {
			actionOwner := strings.TrimSpace(spec.OwnerPackage)
			if actionOwner == "" {
				missingActionOwners = append(missingActionOwners, group.ToolName+"."+spec.Name)
				continue
			}
			if _, ok := owners[actionOwner]; !ok {
				unknownActionOwners = append(unknownActionOwners, fmt.Sprintf("%s.%s owner %s", group.ToolName, spec.Name, actionOwner))
			}
		}
	}

	if len(missingGroupOwners)+len(unknownGroupOwners)+len(missingActionOwners)+len(unknownActionOwners) > 0 {
		sort.Strings(missingGroupOwners)
		sort.Strings(unknownGroupOwners)
		sort.Strings(missingActionOwners)
		sort.Strings(unknownActionOwners)
		t.Fatalf("catalog ownership drift:\nmissing group owners: %v\nunknown group owners: %v\nmissing action owners: %v\nunknown action owners: %v", missingGroupOwners, unknownGroupOwners, missingActionOwners, unknownActionOwners)
	}
}

// TestCollectedActionSpecs_ClassifySamplingUtility verifies CollectedActionSpecs when classify sampling utility.
func TestCollectedActionSpecs_ClassifySamplingUtility(t *testing.T) {
	var analyzeGroup ActionSpecGroup
	for _, group := range CollectActionSpecs(nil, false) {
		if group.ToolName == "gitlab_analyze" {
			analyzeGroup = group
			break
		}
	}
	if analyzeGroup.ToolName == "" {
		t.Fatal("CollectActionSpecs() missing gitlab_analyze")
	}
	if analyzeGroup.SurfaceKind != actioncatalog.SurfaceKindSamplingUtility {
		t.Fatalf("gitlab_analyze surface kind = %q, want %q", analyzeGroup.SurfaceKind, actioncatalog.SurfaceKindSamplingUtility)
	}
	if !slices.Contains(analyzeGroup.CapabilityRequirements, "sampling") {
		t.Fatalf("gitlab_analyze capability requirements = %#v, want sampling", analyzeGroup.CapabilityRequirements)
	}
}

// TestActionSpecGroup_EmptySpecsReturnsNil verifies empty groups are omitted
// before catalog construction.
func TestActionSpecGroup_EmptySpecsReturnsNil(t *testing.T) {
	if groups := actionSpecGroup("gitlab_empty", nil); groups != nil {
		t.Fatalf("actionSpecGroup(empty) = %+v, want nil", groups)
	}
}

// TestActionSpecGroupsByTool_RejectsInvalidSpecs verifies grouping reports
// blank tool names, blank actions, duplicates, and still sorts valid specs.
func TestActionSpecGroupsByTool_RejectsInvalidSpecs(t *testing.T) {
	groups := []ActionSpecGroup{
		{ToolName: " ", Actions: []toolutil.ActionSpec{toolutil.NewActionSpec("ignored", testCatalogActionRoute(), toolutil.ActionSpecOptions{})}},
		{ToolName: "gitlab_test", Actions: []toolutil.ActionSpec{
			toolutil.NewActionSpec("zeta", testCatalogActionRoute(), toolutil.ActionSpecOptions{}),
			{Name: ""},
			toolutil.NewActionSpec("alpha", testCatalogActionRoute(), toolutil.ActionSpecOptions{}),
			toolutil.NewActionSpec("alpha", testCatalogActionRoute(), toolutil.ActionSpecOptions{}),
		}},
	}

	byTool, err := actionSpecGroupsByTool(groups)
	if err == nil {
		t.Fatal("expected grouped validation errors")
	}
	for _, want := range []string{"tool name", "action spec name is required", "duplicate action"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want %q", err.Error(), want)
		}
	}
	specs := byTool["gitlab_test"]
	if len(specs) != 4 {
		t.Fatalf("gitlab_test specs = %d, want 4", len(specs))
	}
	if specs[1].Name != "alpha" || specs[3].Name != "zeta" {
		t.Fatalf("sorted specs = %+v, want blank, alpha, alpha, zeta", specs)
	}
}

// TestSortedActionSpecGroups_EmptyReturnsNil verifies nil inputs are preserved.
func TestSortedActionSpecGroups_EmptyReturnsNil(t *testing.T) {
	if got := sortedActionSpecGroups(nil); got != nil {
		t.Fatalf("sortedActionSpecGroups(nil) = %+v, want nil", got)
	}
}

// sourceToolPackageNames supports source tool package names assertions in tools tests.
func sourceToolPackageNames(t *testing.T) map[string]struct{} {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	owners := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			owners[entry.Name()] = struct{}{}
		}
	}
	return owners
}

// TestActionSpecSurfacePolicy_MetadataProjectsPerSurface verifies ActionSpecSurfacePolicy when metadata projects per surface.
func TestActionSpecSurfacePolicy_MetadataProjectsPerSurface(t *testing.T) {
	spec, handlerCalled := surfacePolicyTestSpec(t)
	metaRoutes := assertSurfacePolicyMetaProjection(t, spec, handlerCalled)
	assertSurfacePolicyDynamicProjection(t, spec)
	assertSurfacePolicySchemaProjection(t, metaRoutes)
	assertSurfacePolicyIndividualProjection(t, spec)
}

func surfacePolicyTestSpec(t *testing.T) (toolutil.ActionSpec, *bool) {
	t.Helper()
	openWorldOverride := false
	handlerCalled := false
	route := toolutil.ActionRoute{
		Handler: func(_ context.Context, params map[string]any) (any, error) {
			handlerCalled = true
			if params["project_id"] != "123" {
				t.Fatalf("handler project_id = %v, want 123", params["project_id"])
			}
			return map[string]any{"ok": true}, nil
		},
		Destructive: true,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id": map[string]any{"type": "string", "description": "GitLab project ID,required"},
			},
			"required":             []any{"project_id"},
			"additionalProperties": false,
		},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ok": map[string]any{"type": "boolean"},
			},
		},
		ParameterGuidance: map[string]toolutil.ParameterGuidance{
			"project_id": {SemanticRole: "gitlab project id"},
		},
	}
	spec := toolutil.NewActionSpec("delete", route, toolutil.ActionSpecOptions{
		Aliases:        []string{"remove repository"},
		Tags:           []string{"project", "destructive"},
		Usage:          "Delete a project permanently; use project.archive for reversible changes.",
		RelatedActions: []string{"project.archive"},
		ParameterGuidance: map[string]toolutil.ParameterGuidance{
			"project_id": {
				ValueSource:      "prompt project reference",
				CommonConfusions: []string{"target_project_id belongs to project sharing actions"},
			},
		},
		Destructive:  true,
		Idempotent:   true,
		OpenWorld:    true,
		OwnerPackage: "projects",
		IndividualTool: toolutil.IndividualToolSpec{
			Name:        "gitlab_project_delete",
			Title:       "Delete Project",
			Description: "Delete a GitLab project.",
			AnnotationOverrides: toolutil.IndividualToolAnnotationOverrides{
				OpenWorld: &openWorldOverride,
			},
		},
	})
	return spec, &handlerCalled
}

func assertSurfacePolicyMetaProjection(t *testing.T, spec toolutil.ActionSpec, handlerCalled *bool) toolutil.ActionMap {
	t.Helper()
	metaRoutes, err := toolutil.ActionSpecsToMapWithError([]toolutil.ActionSpec{spec})
	if err != nil {
		t.Fatalf("ActionSpecsToMapWithError() error = %v", err)
	}
	metaRoute := metaRoutes["delete"]
	if metaRoute.Handler == nil {
		t.Fatal("meta route lost handler")
	}
	if _, handlerErr := metaRoute.Handler(context.Background(), map[string]any{"project_id": "123"}); handlerErr != nil {
		t.Fatalf("meta route handler error = %v", handlerErr)
	}
	if !*handlerCalled {
		t.Fatal("meta route handler was not called")
	}
	if got := metaRoute.ParameterGuidance["project_id"].SemanticRole; got != "gitlab project id" {
		t.Fatalf("meta route guidance semantic role = %q", got)
	}
	if got := metaRoute.ParameterGuidance["project_id"].ValueSource; got != "prompt project reference" {
		t.Fatalf("meta route guidance value source = %q", got)
	}

	metaPrefix := toolutil.MetaToolDescriptionPrefix("gitlab_project", metaRoutes)
	if !strings.Contains(metaPrefix, "Action params schema: gitlab://tools/gitlab_project.<action>.") {
		t.Fatalf("meta description prefix missing schema hint: %q", metaPrefix)
	}
	if !strings.Contains(metaPrefix, "delete.project_id: gitlab project id; source: prompt project reference") {
		t.Fatalf("meta description prefix missing parameter guidance: %q", metaPrefix)
	}
	if strings.Contains(metaPrefix, "Delete a GitLab project.") {
		t.Fatalf("meta description prefix leaked individual tool prose: %q", metaPrefix)
	}
	return metaRoutes
}

func assertSurfacePolicyDynamicProjection(t *testing.T, spec toolutil.ActionSpec) {
	t.Helper()
	group, err := actioncatalog.GroupFromSpecs(actioncatalog.GroupOptions{ToolName: "gitlab_project"}, []toolutil.ActionSpec{spec})
	if err != nil {
		t.Fatalf("GroupFromSpecs() error = %v", err)
	}
	catalog := actioncatalog.NewCatalog()
	if addErr := catalog.AddGroup(group); addErr != nil {
		t.Fatalf("AddGroup() error = %v", addErr)
	}
	registry := dynamic.NewRegistryFromCatalog(catalog)

	_, searchOutput, err := registry.Search(context.Background(), nil, dynamic.SearchInput{Query: "remove repository", Limit: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if searchOutput.Count != 1 || searchOutput.Results[0].ID != "project.delete" {
		t.Fatalf("search result = %+v, want project.delete", searchOutput.Results)
	}
	searchResult := searchOutput.Results[0]
	if searchResult.SchemaURI != "gitlab://tools/project.delete" {
		t.Fatalf("search schema URI = %q", searchResult.SchemaURI)
	}
	if searchResult.Usage != spec.Usage {
		t.Fatalf("search usage = %q, want spec usage", searchResult.Usage)
	}

	_, describeOutput, err := registry.Describe(context.Background(), nil, dynamic.DescribeInput{Action: "project.delete"})
	if err != nil {
		t.Fatalf("Describe() error = %v", err)
	}
	if describeOutput.Count != 1 {
		t.Fatalf("describe count = %d, want 1", describeOutput.Count)
	}
	description := describeOutput.Actions[0]
	if description.ParamGuidance["project_id"].ValueSource != "prompt project reference" {
		t.Fatalf("describe parameter guidance = %+v", description.ParamGuidance["project_id"])
	}
	if got := strings.Join(description.RelatedActions, ","); got != "project.archive" {
		t.Fatalf("describe related actions = %q", got)
	}
	if description.Example.Arguments["action"] != "project.delete" {
		t.Fatalf("describe example action = %v", description.Example.Arguments["action"])
	}
}

func assertSurfacePolicySchemaProjection(t *testing.T, metaRoutes toolutil.ActionMap) {
	t.Helper()
	schema, ok := toolutil.LookupMetaActionSchema(map[string]toolutil.ActionMap{"gitlab_project": metaRoutes}, "gitlab_project", "delete")
	if !ok {
		t.Fatal("schema resource lookup failed")
	}
	if schema["x_destructive"] != true {
		t.Fatalf("schema x_destructive = %v", schema["x_destructive"])
	}
	properties, _ := schema["properties"].(map[string]any)
	if _, hasConfirm := properties["confirm"]; !hasConfirm {
		t.Fatalf("schema properties missing confirm: %+v", properties)
	}
	xGuidance, _ := schema["x_parameter_guidance"].(map[string]any)
	projectGuidance, _ := xGuidance["project_id"].(map[string]any)
	if projectGuidance["semantic_role"] != "gitlab project id" || projectGuidance["value_source"] != "prompt project reference" {
		t.Fatalf("schema parameter guidance = %+v", projectGuidance)
	}
}

func assertSurfacePolicyIndividualProjection(t *testing.T, spec toolutil.ActionSpec) {
	t.Helper()
	individual, err := toolutil.IndividualToolFromActionSpec(spec, toolutil.IndividualToolProjectionOptions{Description: "fallback description", Icons: toolutil.IconProject})
	if err != nil {
		t.Fatalf("IndividualToolFromActionSpec() error = %v", err)
	}
	if individual.Name != "gitlab_project_delete" || individual.Title != "Delete Project" || individual.Description != "Delete a GitLab project." {
		t.Fatalf("individual projection = name %q title %q description %q", individual.Name, individual.Title, individual.Description)
	}
	if individual.Annotations == nil || individual.Annotations.OpenWorldHint == nil || *individual.Annotations.OpenWorldHint {
		t.Fatalf("individual open-world annotation = %+v, want override false", individual.Annotations)
	}
	individualInputSchema, ok := individual.InputSchema.(map[string]any)
	if !ok {
		t.Fatalf("individual input schema type = %T, want map[string]any", individual.InputSchema)
	}
	if individualInputSchema["x_parameter_guidance"] != nil {
		t.Fatalf("individual input schema leaked schema-resource guidance extension: %+v", individualInputSchema["x_parameter_guidance"])
	}
}

// TestIndividualToolProjection_RepresentativeDomainParity verifies IndividualToolProjection when representative domain parity.
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

// TestIndividualToolProjection_GoldenSnapshotParity verifies IndividualToolProjection when golden snapshot parity.
func TestIndividualToolProjection_GoldenSnapshotParity(t *testing.T) {
	goldenPath := filepath.Join("testdata", "tools_individual.json")
	goldenData, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden file %s: %v", goldenPath, err)
	}
	var golden []toolSnapshot
	if unmarshalErr := json.Unmarshal(goldenData, &golden); unmarshalErr != nil {
		t.Fatalf("parse golden file %s: %v", goldenPath, unmarshalErr)
	}
	goldenByName := make(map[string]toolSnapshot, len(golden))
	for _, snapshot := range golden {
		goldenByName[snapshot.Name] = snapshot
	}
	specsByIndividualName := individualSpecsByToolNameMap(CollectActionSpecs(nil, true))

	var projectedTools []*mcp.Tool
	missingSpecs := make([]string, 0)
	for _, snapshot := range golden {
		if _, ok := standaloneIndividualToolExceptions[snapshot.Name]; ok {
			continue
		}
		specs := specsByIndividualName[snapshot.Name]
		if len(specs) == 0 {
			missingSpecs = append(missingSpecs, snapshot.Name)
			continue
		}
		for _, spec := range specs {
			projected, projectionErr := toolutil.IndividualToolFromActionSpec(spec, toolutil.IndividualToolProjectionOptions{Description: snapshot.Description})
			if projectionErr != nil {
				t.Fatalf("project %s from ActionSpec: %v", snapshot.Name, projectionErr)
			}
			projectedTools = append(projectedTools, projected)
		}
	}
	if len(missingSpecs) > 0 {
		sort.Strings(missingSpecs)
		t.Fatalf("golden individual tools missing ActionSpec projections: %v", missingSpecs)
	}

	projectedSnapshots := buildSnapshots(t, projectedTools)
	var wantSnapshots []toolSnapshot
	for _, projected := range projectedSnapshots {
		want, ok := goldenByName[projected.Name]
		if !ok {
			t.Fatalf("projected individual tool %q missing from golden snapshot", projected.Name)
		}
		wantSnapshots = append(wantSnapshots, want)
	}
	compareSnapshotSlices(t, goldenPath, wantSnapshots, projectedSnapshots)
}

// TestIndividualToolMetadata_CatalogBackedCoverage verifies IndividualToolMetadata when catalog backed coverage.
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

	specNames := collectCatalogBackedIndividualToolNames(t, toolsByName)

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

func collectCatalogBackedIndividualToolNames(t *testing.T, toolsByName map[string]*mcp.Tool) map[string]string {
	t.Helper()
	specNames := make(map[string]string)
	duplicateSpecNames := make([]string, 0)
	for _, group := range CollectActionSpecs(nil, true) {
		duplicates := recordCatalogBackedGroupSpecs(t, specNames, toolsByName, group)
		duplicateSpecNames = append(duplicateSpecNames, duplicates...)
	}
	if len(duplicateSpecNames) > 0 {
		sort.Strings(duplicateSpecNames)
		t.Fatalf("unexpected shared individual tool references: %v", duplicateSpecNames)
	}
	return specNames
}

func recordCatalogBackedGroupSpecs(t *testing.T, specNames map[string]string, toolsByName map[string]*mcp.Tool, group ActionSpecGroup) []string {
	t.Helper()
	duplicates := make([]string, 0)
	for _, spec := range group.Actions {
		name := strings.TrimSpace(spec.IndividualTool.Name)
		if name == "" {
			t.Fatalf("%s.%s missing individual tool name", group.ToolName, spec.Name)
		}
		if duplicate := recordCatalogBackedSpecName(specNames, group, spec, name); duplicate != "" {
			duplicates = append(duplicates, duplicate)
		}
		if _, ok := toolsByName[name]; !ok {
			t.Fatalf("%s.%s references unregistered individual tool %q", group.ToolName, spec.Name, name)
		}
	}
	return duplicates
}

func recordCatalogBackedSpecName(specNames map[string]string, group ActionSpecGroup, spec toolutil.ActionSpec, name string) string {
	if previous, exists := specNames[name]; exists {
		if _, ok := sharedIndividualToolSpecNames[name]; !ok {
			return fmt.Sprintf("%s => %s, %s.%s", name, previous, group.ToolName, spec.Name)
		}
		return ""
	}
	specNames[name] = group.ToolName + "." + spec.Name
	return ""
}

// TestIndividualToolMetadata_SourceRegistrationUsesActionSpecProjection verifies IndividualToolMetadata when source registration uses action spec projection.
func TestIndividualToolMetadata_SourceRegistrationUsesActionSpecProjection(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	manualRegistrations := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(entry.Name(), "register.go")
		src, readErr := os.ReadFile(path)
		if os.IsNotExist(readErr) {
			continue
		}
		if readErr != nil {
			t.Fatalf("ReadFile %s: %v", path, readErr)
		}
		if !strings.Contains(string(src), "&mcp.Tool{") {
			continue
		}
		if reason, ok := manualRegistrationExceptions[path]; ok {
			t.Logf("allowing manual tool registration in %s: %s", path, reason)
			continue
		}
		manualRegistrations = append(manualRegistrations, path)
	}

	if len(manualRegistrations) > 0 {
		sort.Strings(manualRegistrations)
		t.Fatalf("tool register.go files must use ActionSpec individual projection instead of manual mcp.Tool metadata: %v", manualRegistrations)
	}
}

// standaloneIndividualToolExceptions lists manually registered tools that do not
// belong to a GitLab API action catalog group.
var standaloneIndividualToolExceptions = map[string]string{
	"gitlab_discover_project":           "dynamic standalone project discovery helper",
	"gitlab_interactive_issue_create":   "elicitation standalone multi-step workflow",
	"gitlab_interactive_mr_create":      "elicitation standalone multi-step workflow",
	"gitlab_interactive_project_create": "elicitation standalone multi-step workflow",
	"gitlab_interactive_release_create": "elicitation standalone multi-step workflow",
	"gitlab_server_status":              "server diagnostic helper outside the GitLab API catalog",
}

// manualRegistrationExceptions allows the few package-level registration files
// that intentionally stay outside ordinary GitLab API action projection.
var manualRegistrationExceptions = map[string]string{
	filepath.Join("dynamic", "register.go"):      "dynamic catalog controller tools are generated from the canonical catalog surface, not individual GitLab API tools",
	filepath.Join("serverupdate", "register.go"): "server auto-update tools use *autoupdate.Updater and are registered from cmd/server/main.go outside RegisterAll",
}

// sharedIndividualToolSpecNames records individual tool names that are projected
// from more than one canonical action for compatibility.
var sharedIndividualToolSpecNames = map[string]string{
	"gitlab_commit_list":      "shared by gitlab_repository.commit_list and gitlab_repository.file_history",
	"gitlab_issue_list_group": "shared by gitlab_group.issues and gitlab_issue.list_group",
	"gitlab_user_current":     "shared by gitlab_user.current and gitlab_user.me",
}

// individualSpecsByToolNameMap groups canonical specs by projected individual
// tool name.
func individualSpecsByToolNameMap(groups []ActionSpecGroup) map[string][]toolutil.ActionSpec {
	byName := make(map[string][]toolutil.ActionSpec)
	for _, group := range groups {
		for _, spec := range group.Actions {
			name := strings.TrimSpace(spec.IndividualTool.Name)
			if name != "" {
				byName[name] = append(byName[name], spec)
			}
		}
	}
	return byName
}

// compareSnapshotSlices compares expected and projected tool snapshots and
// reports only non-allowlisted drift.
func compareSnapshotSlices(t *testing.T, goldenPath string, want, got []toolSnapshot) {
	t.Helper()
	sortToolSnapshots(want)
	sortToolSnapshots(got)
	if len(want) != len(got) {
		reportDiff(t, goldenPath, want, got)
		return
	}
	var diffs []string
	observedSchemaGaps := make(map[string]struct{})
	observedAnnotationGaps := make(map[string]struct{})
	for index := range want {
		name := want[index].Name
		if name != got[index].Name {
			diffs = append(diffs, fmt.Sprintf("%s projected name = %s", name, got[index].Name))
			continue
		}
		if want[index].Description != got[index].Description {
			diffs = append(diffs, "CHANGED "+name+" description")
		}
		if !schemaJSONEqual(t, name, want[index].InputSchema, got[index].InputSchema) {
			if _, ok := knownIndividualProjectionSchemaGaps[name]; !ok {
				diffs = append(diffs, schemaDiffMessage(t, name, want[index].InputSchema, got[index].InputSchema))
			} else {
				observedSchemaGaps[name] = struct{}{}
			}
		}
		if !schemaJSONEqual(t, name, want[index].OutputSchema, got[index].OutputSchema) {
			diffs = append(diffs, "CHANGED "+name+" outputSchema")
		}
		if !annotationsEqual(t, name, want[index].Annotations, got[index].Annotations) {
			if _, ok := knownIndividualProjectionAnnotationGaps[name]; !ok {
				diffs = append(diffs, "CHANGED "+name+" annotations")
			} else {
				observedAnnotationGaps[name] = struct{}{}
			}
		}
	}
	appendStaleProjectionGapDiffs(&diffs, "schema", knownIndividualProjectionSchemaGaps, observedSchemaGaps)
	appendStaleProjectionGapDiffs(&diffs, "annotation", knownIndividualProjectionAnnotationGaps, observedAnnotationGaps)
	if len(diffs) > 0 {
		sort.Strings(diffs)
		t.Fatalf("generated individual snapshot parity drift against %s:\n%s", goldenPath, strings.Join(diffs, "\n"))
	}
}

// schemaDiffMessage formats a stable JSON schema diff for snapshot failures.
func schemaDiffMessage(t *testing.T, name string, want, got json.RawMessage) string {
	t.Helper()
	wantJSON, wantErr := normalizedSchemaJSON(want)
	if wantErr != nil {
		t.Fatalf("normalize want schema for %s: %v", name, wantErr)
	}
	gotJSON, gotErr := normalizedSchemaJSON(got)
	if gotErr != nil {
		t.Fatalf("normalize got schema for %s: %v", name, gotErr)
	}
	return "CHANGED " + name + " inputSchema:\n  old: " + string(wantJSON) + "\n  new: " + string(gotJSON)
}

// appendStaleProjectionGapDiffs reports allowlisted projection gaps that no
// longer appear in the generated snapshot.
func appendStaleProjectionGapDiffs(diffs *[]string, kind string, known map[string]string, observed map[string]struct{}) {
	for name := range known {
		if _, ok := observed[name]; !ok {
			*diffs = append(*diffs, fmt.Sprintf("STALE %s gap allowlist: %s", kind, name))
		}
	}
}

// sortToolSnapshots orders projected tool snapshots by name before comparison.
func sortToolSnapshots(snapshots []toolSnapshot) {
	sort.SliceStable(snapshots, func(left, right int) bool {
		return snapshots[left].Name < snapshots[right].Name
	})
}

// annotationsEqual reports whether projected MCP annotations match exactly.
func annotationsEqual(t *testing.T, name string, want, got *mcp.ToolAnnotations) bool {
	t.Helper()
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal want annotations for %s: %v", name, err)
	}
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal got annotations for %s: %v", name, err)
	}
	return string(wantJSON) == string(gotJSON)
}

// schemaJSONEqual reports whether two JSON schemas are equal after normalizing
// unstable field order.
func schemaJSONEqual(t *testing.T, name string, want, got json.RawMessage) bool {
	t.Helper()
	wantJSON, wantErr := normalizedSchemaJSON(want)
	if wantErr != nil {
		t.Fatalf("normalize want schema for %s: %v", name, wantErr)
	}
	gotJSON, gotErr := normalizedSchemaJSON(got)
	if gotErr != nil {
		t.Fatalf("normalize got schema for %s: %v", name, gotErr)
	}
	return string(wantJSON) == string(gotJSON)
}

// normalizedSchemaJSON normalizes schema JSON for stable test assertions.
func normalizedSchemaJSON(raw json.RawMessage) ([]byte, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	normalizeSchemaValue("", value)
	return json.Marshal(value)
}

// normalizeSchemaValue recursively sorts JSON Schema `required` arrays so
// comparisons ignore map iteration order.
func normalizeSchemaValue(key string, value any) {
	switch typed := value.(type) {
	case map[string]any:
		for childKey, childValue := range typed {
			normalizeSchemaValue(childKey, childValue)
		}
	case []any:
		if key == "required" {
			required := make([]string, 0, len(typed))
			for _, item := range typed {
				field, ok := item.(string)
				if !ok {
					return
				}
				required = append(required, field)
			}
			sort.Strings(required)
			for index, field := range required {
				typed[index] = field
			}
			return
		}
		for _, childValue := range typed {
			normalizeSchemaValue("", childValue)
		}
	}
}

// knownIndividualProjectionAnnotationGaps tracks accepted annotation parity gaps
// until their source packages are migrated.
var knownIndividualProjectionAnnotationGaps = map[string]string{}

// knownIndividualProjectionSchemaGaps tracks accepted schema parity gaps until
// their source packages are migrated.
var knownIndividualProjectionSchemaGaps = map[string]string{}

// assertActionRouteParity checks action route parity invariants for tests.
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

// assertSpecProjectionParity checks spec projection parity invariants for tests.
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

// assertGuidanceKeys checks guidance keys invariants for tests.
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

// assertProjectedToolParity checks projected tool parity invariants for tests.
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

// assertProjectedToolAnnotations checks projected tool annotations invariants for tests.
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

// assertToolIconsParity checks tool icons parity invariants for tests.
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

// missingRouteNames returns canonical action names absent from the projected
// route map.
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
