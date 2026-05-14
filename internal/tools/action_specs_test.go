package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/dynamic"
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

func TestActionSpecSurfacePolicy_MetadataProjectsPerSurface(t *testing.T) {
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
	if !handlerCalled {
		t.Fatal("meta route handler was not called")
	}
	if got := metaRoute.ParameterGuidance["project_id"].SemanticRole; got != "gitlab project id" {
		t.Fatalf("meta route guidance semantic role = %q", got)
	}
	if got := metaRoute.ParameterGuidance["project_id"].ValueSource; got != "prompt project reference" {
		t.Fatalf("meta route guidance value source = %q", got)
	}

	metaPrefix := toolutil.MetaToolDescriptionPrefix("gitlab_project", metaRoutes)
	if !strings.Contains(metaPrefix, "Action params schema: gitlab://schema/meta/gitlab_project/<action>.") {
		t.Fatalf("meta description prefix missing schema hint: %q", metaPrefix)
	}
	if !strings.Contains(metaPrefix, "delete.project_id: gitlab project id; source: prompt project reference") {
		t.Fatalf("meta description prefix missing parameter guidance: %q", metaPrefix)
	}
	if strings.Contains(metaPrefix, "Delete a GitLab project.") {
		t.Fatalf("meta description prefix leaked individual tool prose: %q", metaPrefix)
	}

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
	if searchResult.SchemaURI != "gitlab://schema/meta/gitlab_project/delete" {
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

var standaloneIndividualToolExceptions = map[string]string{
	"gitlab_discover_project":           "dynamic standalone project discovery helper",
	"gitlab_interactive_issue_create":   "elicitation standalone multi-step workflow",
	"gitlab_interactive_mr_create":      "elicitation standalone multi-step workflow",
	"gitlab_interactive_project_create": "elicitation standalone multi-step workflow",
	"gitlab_interactive_release_create": "elicitation standalone multi-step workflow",
	"gitlab_server_status":              "server diagnostic helper outside the GitLab API catalog",
}

var manualRegistrationExceptions = map[string]string{
	filepath.Join("dynamic", "register.go"):      "dynamic catalog search/describe/execute tools are generated from the canonical catalog surface, not individual GitLab API tools",
	filepath.Join("serverupdate", "register.go"): "server auto-update tools use *autoupdate.Updater and are registered from cmd/server/main.go outside RegisterAll",
}

var sharedIndividualToolSpecNames = map[string]string{
	"gitlab_commit_list":      "shared by gitlab_repository.commit_list and gitlab_repository.file_history",
	"gitlab_issue_list_group": "shared by gitlab_group.issues and gitlab_issue.list_group",
	"gitlab_user_current":     "shared by gitlab_user.current and gitlab_user.me",
}

func individualSpecsByToolNameMap(groups []ActionSpecGroup) map[string][]toolutil.ActionSpec {
	byName := make(map[string][]toolutil.ActionSpec)
	for _, group := range groups {
		for _, spec := range group.Specs {
			name := strings.TrimSpace(spec.IndividualTool.Name)
			if name != "" {
				byName[name] = append(byName[name], spec)
			}
		}
	}
	return byName
}

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

func appendStaleProjectionGapDiffs(diffs *[]string, kind string, known map[string]string, observed map[string]struct{}) {
	for name := range known {
		if _, ok := observed[name]; !ok {
			*diffs = append(*diffs, fmt.Sprintf("STALE %s gap allowlist: %s", kind, name))
		}
	}
}

func sortToolSnapshots(snapshots []toolSnapshot) {
	sort.SliceStable(snapshots, func(left, right int) bool {
		return snapshots[left].Name < snapshots[right].Name
	})
}

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

var knownIndividualProjectionAnnotationGaps = map[string]string{}

var knownIndividualProjectionSchemaGaps = map[string]string{}

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
