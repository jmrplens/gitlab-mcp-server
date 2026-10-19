package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/freshness"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/dynamic"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/securityscanprofiles"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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

			// The catalog applies the central tier filter; align the raw collected
			// specs to the same effective tier so route parity is comparable.
			tier := edition.TierForEnterprise(tc.enterprise)
			for _, specGroup := range filterActionSpecGroupsByTier(CollectActionSpecs(tc.client), tier) {
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
	specsByTool, err := actionSpecGroupsByTool(CollectActionSpecs(newGitLabDotComClient(t)))
	if err != nil {
		t.Fatalf("actionSpecGroupsByTool() error = %v", err)
	}

	testCases := []struct {
		toolName string
		action   string
		keys     []string
	}{
		{toolName: "gitlab_merge_request", action: "create", keys: []string{"source_branch", "target_branch", "milestone_id", "project_id"}},
		{toolName: "gitlab_issue", action: "link_create", keys: []string{"project_id", "issue_iid", "target_project_id", "target_issue_iid", "link_type"}},
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

	for _, group := range CollectActionSpecs(newGitLabDotComClient(t)) {
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
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("error = %q, want %q", err.Error(), want)
			}
		})
	}
	specs := byTool["gitlab_test"]
	if len(specs) != 4 {
		t.Fatalf("gitlab_test specs = %d, want 4", len(specs))
	}
	if specs[1].Name != "alpha" || specs[3].Name != "zeta" {
		t.Fatalf("sorted specs = %+v, want blank, alpha, alpha, zeta", specs)
	}
}

// TestActionSpecGroupsByTool_DuplicateActionNames_KeepDeclarationOrder pins the
// stability of the per-tool sort.
//
// A duplicate action name is reported as an error and the specs are returned
// anyway, so the order decides which of the two a reader working from the
// returned slice sees first. The sort is stable for exactly that reason: a
// comparison that answered true for two equal names would reorder them, and the
// error would then name one spec while the slice handed back the other.
func TestActionSpecGroupsByTool_DuplicateActionNames_KeepDeclarationOrder(t *testing.T) {
	groups := []ActionSpecGroup{{ToolName: "gitlab_test", Actions: []toolutil.ActionSpec{
		toolutil.NewActionSpec("duplicate", testCatalogActionRoute(), toolutil.ActionSpecOptions{Usage: "declared first"}),
		toolutil.NewActionSpec("duplicate", testCatalogActionRoute(), toolutil.ActionSpecOptions{Usage: "declared second"}),
		toolutil.NewActionSpec("zeta", testCatalogActionRoute(), toolutil.ActionSpecOptions{Usage: "declared last"}),
	}}}

	byTool, err := actionSpecGroupsByTool(groups)
	if err == nil {
		t.Fatal("actionSpecGroupsByTool() error = nil, want the duplicate action name reported")
	}
	specs := byTool["gitlab_test"]
	if len(specs) != 3 {
		t.Fatalf("gitlab_test specs = %d, want all three kept for the caller to inspect", len(specs))
	}
	got := []string{specs[0].Usage, specs[1].Usage, specs[2].Usage}
	want := []string{"declared first", "declared second", "declared last"}
	if !slices.Equal(got, want) {
		t.Errorf("sorted specs = %v, want %v: two specs of one name must stay in declaration order", got, want)
	}
}

// TestSortedActionSpecGroups_EmptyReturnsNil verifies nil inputs are preserved.
func TestSortedActionSpecGroups_EmptyReturnsNil(t *testing.T) {
	if got := sortedActionSpecGroups(nil); got != nil {
		t.Fatalf("sortedActionSpecGroups(nil) = %+v, want nil", got)
	}
}

// TestSortedActionSpecGroups_SortsByToolNameAndKeepsDeclarationOrder covers
// both halves of the sort every collected catalog is assembled through.
//
// The order is what makes a catalog deterministic: the groups reach
// [BuildActionCatalog] in it, and the merge that folds overrides in reads it.
// Two groups contributing to one tool are ordered by declaration, so a domain
// that splits its specs across two builders keeps the order it wrote them in;
// a comparison that answered true for two equal tool names would swap those.
func TestSortedActionSpecGroups_SortsByToolNameAndKeepsDeclarationOrder(t *testing.T) {
	groups := []ActionSpecGroup{
		{ToolName: "gitlab_zeta", Title: "first zeta"},
		{ToolName: "gitlab_alpha", Title: "alpha"},
		{ToolName: "gitlab_zeta", Title: "second zeta"},
	}

	sorted := sortedActionSpecGroups(groups)

	got := []string{sorted[0].Title, sorted[1].Title, sorted[2].Title}
	want := []string{"alpha", "first zeta", "second zeta"}
	if !slices.Equal(got, want) {
		t.Errorf("sorted groups = %v, want %v", got, want)
	}
	if groups[0].Title != "first zeta" {
		t.Errorf("input group 0 = %q, want the caller's slice left untouched", groups[0].Title)
	}
}

// TestEnterpriseGroupBuilders_CarryTheirOwnDescription pins the three catalog
// groups that document their own actions instead of taking the curated
// snapshot text.
//
// The description is what a model reads to decide whether the group can do what
// it was asked, and these three describe GraphQL-only domains whose parameters
// appear nowhere else. It is set where the group is built, so a domain that
// contributed no specs yields no group rather than an assignment guarded
// against a slice that cannot be empty.
func TestEnterpriseGroupBuilders_CarryTheirOwnDescription(t *testing.T) {
	cases := []struct {
		name       string
		build      actionSpecGroupBuilder
		wantPrefix string
	}{
		{name: "gitlab_security_attribute", build: buildSecurityAttributeActionSpecs, wantPrefix: "Manage GitLab security attributes via GraphQL"},
		{name: "gitlab_security_category", build: buildSecurityCategoryActionSpecs, wantPrefix: "Manage GitLab security categories via GraphQL"},
		{name: "gitlab_security_scan_profile", build: buildSecurityScanProfileActionSpecs, wantPrefix: "Attach, detach, and inspect GitLab security scan profiles"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			groups := tc.build(nil)
			if len(groups) != 1 {
				t.Fatalf("%s builder returned %d groups, want exactly one", tc.name, len(groups))
			}
			if groups[0].ToolName != tc.name {
				t.Errorf("tool name = %q, want %q", groups[0].ToolName, tc.name)
			}
			if !strings.HasPrefix(groups[0].Description, tc.wantPrefix) {
				t.Errorf("description = %q, want it to start with %q", groups[0].Description, tc.wantPrefix)
			}
		})
	}
}

// TestSecurityScanProfileGroup_Description_OffersTheNamesTheirReleasesAndTheRefusals
// verifies that the meta group description, which a model on the meta surface
// reads where the individual surface reads the attach usage line, carries the
// same three sentences: the default profile names attach takes, the release
// each needs, and the scan types attach refuses by name. It used to offer the
// names alone, so a meta caller was never told container_scanning would be
// refused.
func TestSecurityScanProfileGroup_Description_OffersTheNamesTheirReleasesAndTheRefusals(t *testing.T) {
	groups := buildSecurityScanProfileActionSpecs(nil)
	if len(groups) != 1 {
		t.Fatalf("builder returned %d groups, want exactly one", len(groups))
	}
	sentences := map[string]string{
		"names":    securityscanprofiles.DefaultProfileNames,
		"releases": securityscanprofiles.DefaultProfileFloors,
		"refusals": securityscanprofiles.RefusedByName,
	}
	for name, want := range sentences {
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(groups[0].Description, want) {
				t.Errorf("description = %q, want it to carry %q", groups[0].Description, want)
			}
		})
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
				t.Errorf("handler project_id = %v, want 123", params["project_id"])
				return nil, fmt.Errorf("unexpected project_id %v", params["project_id"])
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
	if !strings.Contains(metaPrefix, "delete.project_id: gitlab project id. Source: prompt project reference") {
		t.Fatalf("meta description prefix missing parameter guidance: %q", metaPrefix)
	}
	if strings.Contains(metaPrefix, "Delete a GitLab project.") {
		t.Fatalf("meta description prefix leaked individual tool prose: %q", metaPrefix)
	}
	return metaRoutes
}

// surfacePolicyRelatedSpec is the action the spec under test cross-links to.
//
// It belongs in the group because the dynamic registry publishes a cross-link
// only when the session it serves can run it: a related ID the catalog does
// not hold is dropped rather than handed to a model as a next step it would be
// told is unknown. Without this the projection assertion would be checking
// that a dead link survives.
func surfacePolicyRelatedSpec() toolutil.ActionSpec {
	return toolutil.NewActionSpec("archive", toolutil.ActionRoute{
		Handler:     func(context.Context, map[string]any) (any, error) { return map[string]any{"ok": true}, nil },
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
	}, toolutil.ActionSpecOptions{
		Usage:          "Archive a project reversibly.",
		OwnerPackage:   "projects",
		IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_project_archive", Title: "Archive Project"},
	})
}

func assertSurfacePolicyDynamicProjection(t *testing.T, spec toolutil.ActionSpec) {
	t.Helper()
	group, err := actioncatalog.GroupFromSpecs(actioncatalog.GroupOptions{ToolName: "gitlab_project"}, []toolutil.ActionSpec{spec, surfacePolicyRelatedSpec()})
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

	specsByTool, err := actionSpecGroupsByTool(CollectActionSpecs(nil))
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
	specsByIndividualName := individualSpecsByToolNameMap(CollectActionSpecs(nil))

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
	for _, group := range CollectActionSpecs(nil) {
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
	filepath.Join("dynamic", "register.go"): "dynamic catalog controller tools are generated from the canonical catalog surface, not individual GitLab API tools",
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
// reports only non-allowlisted drift. It compares nothing when the harness
// deferred the comparison ([freshness.SkipIfDeferred]).
func compareSnapshotSlices(t *testing.T, goldenPath string, want, got []toolSnapshot) {
	t.Helper()
	freshness.SkipIfDeferred(t)
	sortToolSnapshots(want)
	sortToolSnapshots(got)
	if len(want) != len(got) {
		diffs, err := diffSnapshots(want, got)
		if err != nil {
			t.Fatalf("diff snapshots: %v", err)
		}
		if len(diffs) > 0 {
			t.Errorf("Tool snapshots changed (%s). Found %d difference(s):\n%s\n\nRun with UPDATE_TOOLSNAPS=true to update golden files.",
				goldenPath, len(diffs), strings.Join(diffs, "\n"))
		}
		return
	}
	var diffs []string
	observed := projectionGapsObserved{schema: map[string]struct{}{}, annotation: map[string]struct{}{}}
	// The two slices are the same length by the guard above; the loop bound
	// repeats it so that the indexing is provably in range where it happens.
	for index := 0; index < len(want) && index < len(got); index++ {
		diffs = append(diffs, projectionDiffs(t, want[index], got[index], observed)...)
	}
	appendStaleProjectionGapDiffs(&diffs, "schema", knownIndividualProjectionSchemaGaps, observed.schema)
	appendStaleProjectionGapDiffs(&diffs, "annotation", knownIndividualProjectionAnnotationGaps, observed.annotation)
	if len(diffs) > 0 {
		sort.Strings(diffs)
		t.Fatalf("generated individual snapshot parity drift against %s:\n%s", goldenPath, strings.Join(diffs, "\n"))
	}
}

// projectionGapsObserved records which allowlisted projection gaps a
// comparison actually met, so an allowlist entry nothing meets any more can be
// reported as stale.
type projectionGapsObserved struct {
	schema     map[string]struct{}
	annotation map[string]struct{}
}

// projectionDiffs compares one expected snapshot with its projection and
// returns the drift, allowlisted gaps recorded in observed rather than
// reported.
func projectionDiffs(t *testing.T, wanted, projected toolSnapshot, observed projectionGapsObserved) []string {
	t.Helper()
	name := wanted.Name
	if name != projected.Name {
		return []string{fmt.Sprintf("%s projected name = %s", name, projected.Name)}
	}
	var diffs []string
	if wanted.Description != projected.Description {
		diffs = append(diffs, "CHANGED "+name+" description")
	}
	if !schemaJSONEqual(t, name, wanted.InputSchema, projected.InputSchema) {
		if _, ok := knownIndividualProjectionSchemaGaps[name]; !ok {
			diffs = append(diffs, schemaDiffMessage(t, name, wanted.InputSchema, projected.InputSchema))
		} else {
			observed.schema[name] = struct{}{}
		}
	}
	if !schemaJSONEqual(t, name, wanted.OutputSchema, projected.OutputSchema) {
		diffs = append(diffs, "CHANGED "+name+" outputSchema")
	}
	if !annotationsEqual(t, name, wanted.Annotations, projected.Annotations) {
		if _, ok := knownIndividualProjectionAnnotationGaps[name]; !ok {
			diffs = append(diffs, "CHANGED "+name+" annotations")
		} else {
			observed.annotation[name] = struct{}{}
		}
	}
	return diffs
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

// premiumGatingExempt lists individual tools whose model-facing text mentions
// "Premium/Ultimate" but which are genuine CE (Free-tier) features. For these
// the premium wording describes an OPTIONAL parameter, not the action itself,
// so the action must NOT be Edition-gated (gating would hide a CE feature from
// CE clients). Each entry carries a one-line rationale.
var premiumGatingExempt = map[string]string{
	// Adding a group member is CE; only the optional member_role_id custom role
	// is Premium/Ultimate. The tool itself works on Free.
	"gitlab_group_member_add": "CE feature; only the optional member_role_id custom role is Premium/Ultimate.",
	// Adding a project member is CE; only the optional member_role_id custom role
	// is Premium/Ultimate. The tool itself works on Free.
	"gitlab_project_member_add": "CE feature; only the optional member_role_id custom role is Premium/Ultimate.",
	// Sharing a group with a group is CE; only the optional member_role_id
	// custom role (Ultimate) and the Minimal access level (a Premium or
	// Ultimate top-level group) need a license. The tool itself works on Free.
	"gitlab_group_share": "CE feature; only the optional member_role_id custom role and the Minimal access level need a license.",
	// Feature flags are available on Free, Premium, and Ultimate per current
	// GitLab docs (docs.gitlab.com/operations/feature_flags). The
	// "Requires Premium/Ultimate" wording in the spec is legacy/over-conservative.
	"gitlab_feature_flag_list":   "Feature flags are a Free-tier feature; the Premium/Ultimate wording is legacy and inaccurate.",
	"gitlab_feature_flag_create": "Feature flags are a Free-tier feature; the Premium/Ultimate wording is legacy and inaccurate.",
	"gitlab_feature_flag_update": "Feature flags are a Free-tier feature; the Premium/Ultimate wording is legacy and inaccurate.",
	"gitlab_feature_flag_delete": "Feature flags are a Free-tier feature; the Premium/Ultimate wording is legacy and inaccurate.",
	// Group integration management is Free per doc/api/group_integrations.md
	// (page tier = Free, Premium, Ultimate); only specific integrations need a
	// paid tier. The wording references that sub-feature nuance, not the API tier.
	"gitlab_list_group_integrations":          "Group integration management API is Free per group_integrations.md; only some integrations need a paid tier.",
	"gitlab_set_group_integration":            "Group integration management API is Free per group_integrations.md; only some integrations need a paid tier.",
	"gitlab_get_group_datadog_integration":    "Group integration management API is Free per group_integrations.md; the Premium/Ultimate wording is over-stated.",
	"gitlab_set_group_datadog_integration":    "Group integration management API is Free per group_integrations.md; the Premium/Ultimate wording is over-stated.",
	"gitlab_delete_group_datadog_integration": "Group integration management API is Free per group_integrations.md; the Premium/Ultimate wording is over-stated.",
}

// TestPremiumDescribedActionsAreEditionGated guards the premium-tool gating
// contract: any action whose model-facing text advertises a GitLab
// Premium/Ultimate requirement MUST also set ActionSpec.Edition, because the
// individual catalog hides an action from non-enterprise (CE) clients only when
// Edition != "" (see individualCatalogActionEligible). A premium action with an
// empty Edition leaks into the CE surface, where it would only 403 at runtime.
//
// Actions in premiumGatingExempt are skipped: they are CE features whose
// Premium/Ultimate text describes only an optional parameter. An entry whose
// action no longer carries that text, or no longer exists ungated, excuses
// nothing and fails the test too, since it would only hide the wording if it
// came back: gitlab_add_project_mirror kept one after its usage stopped
// claiming Premium/Ultimate.
func TestPremiumDescribedActionsAreEditionGated(t *testing.T) {
	var leaks []string
	excused := make(map[string]bool, len(premiumGatingExempt))
	for _, group := range CollectActionSpecs(nil) {
		for _, spec := range group.Actions {
			if spec.Edition != "" {
				continue
			}
			text := strings.ToLower(spec.Usage + " " + spec.IndividualTool.Description)
			if strings.Contains(text, "premium/ultimate") ||
				strings.Contains(text, "premium or ultimate") ||
				strings.Contains(text, "ultimate only") {
				name := spec.IndividualTool.Name
				if name == "" {
					name = group.ToolName + "/" + spec.Name
				}
				if _, ok := premiumGatingExempt[name]; ok {
					excused[name] = true
					continue
				}
				leaks = append(leaks, name)
			}
		}
	}
	var stale []string
	for name := range premiumGatingExempt {
		if !excused[name] {
			stale = append(stale, name)
		}
	}
	if len(stale) > 0 {
		sort.Strings(stale)
		t.Errorf("premiumGatingExempt names %d action(s) that no longer claim Premium/Ultimate while ungated, so the entry excuses nothing; delete it:\n  %s",
			len(stale), strings.Join(stale, "\n  "))
	}
	if len(leaks) > 0 {
		sort.Strings(leaks)
		t.Fatalf("%d premium-described action(s) have empty Edition and leak into the CE surface; gate them via the package's premium spec helper (e.g. groupPremiumSpec/projectPremiumSpec) or set opts.Edition:\n  %s",
			len(leaks), strings.Join(leaks, "\n  "))
	}
}

// clearingCategory names why an update field's empty value is not sent to
// GitLab, for a field [TestUpdateActions_AnEmptyValueReachesGitLabOrIsDeclared]
// would otherwise report.
type clearingCategory string

// The reasons an empty value may stay unsent. There is deliberately no
// category for "GitLab clears it and we drop it": that is the finding.
const (
	// clearingRequired is a field GitLab requires a value for, so an empty
	// one would be refused and the handler reads it as unset.
	clearingRequired clearingCategory = "required"
	// clearingNotStored is a field that names an operation, a credential, a
	// file or a moment rather than a stored value an update can empty.
	clearingNotStored clearingCategory = "not-stored"
	// clearingOneOf is a field that takes one of a fixed set of values, of
	// which the empty string is none.
	clearingOneOf clearingCategory = "one-of"
	// clearingUnexercised is a field of an action whose write the probe's
	// generic answers cannot reach; its package's own tests hold its clears.
	clearingUnexercised clearingCategory = "unexercised"
	// clearingUnverified is a field whose handling of an empty value by GitLab
	// has not been established. The guard stays until it is, and the entry is
	// the record that the question is open.
	clearingUnverified clearingCategory = "unverified"
)

// clearingDeclaration answers one field the clearing gate would report.
type clearingDeclaration struct {
	category clearingCategory
	reason   string
}

// clearRequired declares a field GitLab requires a value for.
func clearRequired(reason string) clearingDeclaration {
	return clearingDeclaration{category: clearingRequired, reason: reason}
}

// clearNotStored declares a field that names something other than a stored
// value.
func clearNotStored(reason string) clearingDeclaration {
	return clearingDeclaration{category: clearingNotStored, reason: reason}
}

// clearOneOf declares a field that takes one of a fixed set of values.
func clearOneOf(reason string) clearingDeclaration {
	return clearingDeclaration{category: clearingOneOf, reason: reason}
}

// clearUnexercised declares a field of an action the probe cannot drive to its
// write.
func clearUnexercised(reason string) clearingDeclaration {
	return clearingDeclaration{category: clearingUnexercised, reason: reason}
}

// clearUnverified declares a field whose answer to an empty value nobody has
// established against GitLab. Issue 1306 stays open for every field declared
// so, and settling one, by GitLab's model and validators or by a probe of an
// instance, moves it to another category or out of the table.
var clearUnverified = clearingDeclaration{
	category: clearingUnverified,
	reason:   "whether GitLab empties this field when sent an empty value has not been checked against an instance, so the guard keeps reading an empty value as unset until it is; issue 1306 stays open for it",
}

// The reasons several declarations share.
const (
	reasonAccessLevelChoice = "one of disabled, private, enabled or public"
	reasonAccessRuleEntries = "each entry adds, changes or removes one access rule, so an empty list changes nothing and is not a clear"
	reasonAdminMoment       = "a moment only an administrator may set, not a value an update empties"
	reasonAvatarUpload      = "part of an image upload, not a stored value"
	reasonCommitMetadata    = "describes the commit the update makes, not a value stored on the object"
	reasonHookSecret        = "a secret GitLab only ever replaces"
	reasonLabelOperation    = "labels to add or remove, an operation rather than a stored list"
	reasonMilestoneTitle    = "a title lib/api/helpers/milestones_helpers.rb resolves to a milestone and skips when blank, so an empty one names nothing; milestone_id 0 is what unassigns the milestone"
	reasonEpicLookup        = "the handler reads the epic's global ID before it sends the mutation, which the probe's generic answer cannot give; the epics package's tests hold its clears (issue 1302)"
	reasonWorkItemLookup    = "the handler reads the work item's global ID, or asks for confirmation, before it sends the mutation, which the probe cannot give; the workitems package's tests hold its clears (issue 1303)"
	reasonWebhookURL        = "a webhook delivers to its URL, which GitLab requires"
	reasonBadgeURL          = "GitLab validates a badge's link and image URLs as addresses and an empty one is not (app/models/badge.rb)"
	reasonServiceAccount    = "a service account has a name and a username"
)

// clearingDeclarations answers, by "action.field", every optional update field
// whose empty value does not reach GitLab. A field that starts reaching GitLab
// makes its entry stale, and a stale entry fails the gate like a missing one.
var clearingDeclarations = map[string]clearingDeclaration{
	"access.deploy_key_update.title":                                 clearRequired("a deploy key has a title"),
	"achievement.update.avatar_content_base64":                       clearNotStored(reasonAvatarUpload),
	"achievement.update.avatar_content_type":                         clearNotStored(reasonAvatarUpload),
	"achievement.update.avatar_file_path":                            clearNotStored(reasonAvatarUpload),
	"achievement.update.avatar_filename":                             clearNotStored(reasonAvatarUpload),
	"achievement.update.name":                                        clearRequired("an achievement has a name"),
	"admin.appearance_update.description":                            clearUnverified,
	"admin.appearance_update.favicon":                                clearNotStored(reasonAvatarUpload),
	"admin.appearance_update.footer_message":                         clearUnverified,
	"admin.appearance_update.header_logo":                            clearNotStored(reasonAvatarUpload),
	"admin.appearance_update.header_message":                         clearUnverified,
	"admin.appearance_update.logo":                                   clearNotStored(reasonAvatarUpload),
	"admin.appearance_update.member_guidelines":                      clearUnverified,
	"admin.appearance_update.message_background_color":               clearUnverified,
	"admin.appearance_update.message_font_color":                     clearUnverified,
	"admin.appearance_update.new_project_guidelines":                 clearUnverified,
	"admin.appearance_update.profile_image_guidelines":               clearUnverified,
	"admin.appearance_update.pwa_description":                        clearUnverified,
	"admin.appearance_update.pwa_icon":                               clearNotStored(reasonAvatarUpload),
	"admin.appearance_update.pwa_name":                               clearUnverified,
	"admin.appearance_update.pwa_short_name":                         clearUnverified,
	"admin.appearance_update.site_name":                              clearUnverified,
	"admin.appearance_update.title":                                  clearUnverified,
	"admin.appearance_update.url":                                    clearUnverified,
	"admin.broadcast_message_update.color":                           clearUnverified,
	"admin.broadcast_message_update.ends_at":                         clearUnverified,
	"admin.broadcast_message_update.font":                            clearUnverified,
	"admin.broadcast_message_update.message":                         clearRequired("a broadcast message has a message"),
	"admin.broadcast_message_update.starts_at":                       clearUnverified,
	"admin.system_hook_edit.branch_filter_strategy":                  clearOneOf("one of wildcard, regex or all_branches"),
	"admin.system_hook_edit.name":                                    clearUnverified,
	"admin.system_hook_edit.signing_token":                           clearNotStored(reasonHookSecret),
	"admin.system_hook_edit.token":                                   clearNotStored(reasonHookSecret),
	"admin.system_hook_edit.url":                                     clearRequired(reasonWebhookURL),
	"admin.topic_update.name":                                        clearRequired("a topic has a name"),
	"admin.topic_update.title":                                       clearRequired("a topic has a title"),
	"branch.update_protected.allowed_to_merge":                       clearNotStored(reasonAccessRuleEntries),
	"branch.update_protected.allowed_to_push":                        clearNotStored(reasonAccessRuleEntries),
	"branch.update_protected.allowed_to_unprotect":                   clearNotStored(reasonAccessRuleEntries),
	"branch.update_protected.name":                                   clearRequired("a protected branch rule names a branch or a wildcard"),
	"ci_variable.group_update.environment_scope":                     clearRequired("a variable has an environment scope, * for every environment"),
	"ci_variable.update.environment_scope":                           clearRequired("a variable has an environment scope, * for every environment"),
	"environment.freeze_update.cron_timezone":                        clearUnverified,
	"environment.freeze_update.freeze_end":                           clearRequired("a freeze period has a cron end"),
	"environment.freeze_update.freeze_start":                         clearRequired("a freeze period has a cron start"),
	"environment.protected_update.approval_rules":                    clearNotStored(reasonAccessRuleEntries),
	"environment.protected_update.deploy_access_levels":              clearNotStored(reasonAccessRuleEntries),
	"environment.protected_update.name":                              clearRequired("a protected environment names an environment"),
	"environment.update.flux_resource_path":                          clearUnverified,
	"environment.update.kubernetes_namespace":                        clearUnverified,
	"environment.update.name":                                        clearRequired("an environment has a name"),
	"external_status_check.update_project.external_url":              clearRequired("a status check calls its external URL"),
	"external_status_check.update_project.name":                      clearRequired("a status check has a name"),
	"external_status_check.update_project.protected_branch_ids":      clearUnverified,
	"external_status_check.update_project.shared_secret":             clearNotStored(reasonHookSecret),
	"feature_flags.feature_flag_update.new_name":                     clearRequired("a feature flag has a name"),
	"feature_flags.feature_flag_update.strategies":                   clearNotStored("each entry changes or removes one strategy by its ID, so an empty list changes nothing"),
	"feature_flags.ff_user_list_update.name":                         clearRequired("a user list has a name"),
	"feature_flags.ff_user_list_update.user_xids":                    clearUnverified,
	"group.badge_edit.image_url":                                     clearRequired(reasonBadgeURL),
	"group.badge_edit.link_url":                                      clearRequired(reasonBadgeURL),
	"group.badge_edit.name":                                          clearUnverified,
	"group.epic_issue_update.adjacent_id":                            clearNotStored("the issue the moved one is placed beside, not a stored value"),
	"group.epic_update.add_label_ids":                                clearNotStored(reasonLabelOperation),
	"group.epic_update.assignee_ids":                                 clearUnexercised(reasonEpicLookup),
	"group.epic_update.color":                                        clearUnexercised(reasonEpicLookup),
	"group.epic_update.description":                                  clearUnexercised(reasonEpicLookup),
	"group.epic_update.due_date":                                     clearUnexercised(reasonEpicLookup),
	"group.epic_update.milestone_id":                                 clearUnexercised(reasonEpicLookup),
	"group.epic_update.parent_id":                                    clearUnexercised(reasonEpicLookup),
	"group.epic_update.remove_label_ids":                             clearNotStored(reasonLabelOperation),
	"group.epic_update.start_date":                                   clearUnexercised(reasonEpicLookup),
	"group.epic_update.title":                                        clearRequired("an epic has a title"),
	"group.epic_update.weight":                                       clearUnexercised(reasonEpicLookup),
	"group.group_board_update.name":                                  clearRequired("a board has a name"),
	"group.group_label_update.color":                                 clearRequired("a label has a color"),
	"group.group_label_update.new_name":                              clearRequired("a label has a name"),
	"group.group_member_edit.expires_at":                             clearUnverified,
	"group.group_milestone_update.title":                             clearRequired("a milestone has a title"),
	"group.hook_edit.custom_headers":                                 clearUnverified,
	"group.hook_edit.name":                                           clearUnverified,
	"group.hook_edit.signing_token":                                  clearNotStored(reasonHookSecret),
	"group.hook_edit.token":                                          clearNotStored(reasonHookSecret),
	"group.hook_edit.url":                                            clearRequired(reasonWebhookURL),
	"group.protected_branch_update.allowed_to_merge":                 clearNotStored(reasonAccessRuleEntries),
	"group.protected_branch_update.allowed_to_push":                  clearNotStored(reasonAccessRuleEntries),
	"group.protected_branch_update.allowed_to_unprotect":             clearNotStored(reasonAccessRuleEntries),
	"group.protected_branch_update.name":                             clearRequired("a protected branch rule names a branch or a wildcard"),
	"group.protected_env_update.approval_rules":                      clearNotStored(reasonAccessRuleEntries),
	"group.protected_env_update.deploy_access_levels":                clearNotStored(reasonAccessRuleEntries),
	"group.security_settings_update.projects_to_exclude":             clearUnverified,
	"group.service_account_update.email":                             clearUnverified,
	"group.service_account_update.name":                              clearRequired(reasonServiceAccount),
	"group.service_account_update.username":                          clearRequired(reasonServiceAccount),
	"group.update.allowed_email_domains_list":                        clearUnverified,
	"group.update.default_branch":                                    clearUnverified,
	"group.update.ip_restriction_ranges":                             clearUnverified,
	"group.update.name":                                              clearRequired("a group has a name"),
	"group.update.path":                                              clearRequired("a group has a path"),
	"group.update.step_up_auth_required_oauth_provider":              clearUnverified,
	"group.update.unique_project_download_limit_alertlist":           clearUnverified,
	"group.update.unique_project_download_limit_allowlist":           clearUnverified,
	"group.wiki_edit.content":                                        clearUnverified,
	"group.wiki_edit.title":                                          clearRequired("a wiki page has a title"),
	"issue.discussion_update_note.created_at":                        clearNotStored(reasonAdminMoment),
	"issue.update.add_labels":                                        clearNotStored(reasonLabelOperation),
	"issue.update.milestone":                                         clearNotStored(reasonMilestoneTitle),
	"issue.update.remove_labels":                                     clearNotStored(reasonLabelOperation),
	"issue.update.title":                                             clearRequired("an issue has a title"),
	"issue.update.updated_at":                                        clearNotStored(reasonAdminMoment),
	"issue.work_item_saved_view_update.description":                  clearUnverified,
	"issue.work_item_saved_view_update.name":                         clearRequired("a saved view has a name"),
	"issue.work_item_update.add_label_ids":                           clearNotStored(reasonLabelOperation),
	"issue.work_item_update.assignee_ids":                            clearUnexercised(reasonWorkItemLookup),
	"issue.work_item_update.color":                                   clearUnexercised(reasonWorkItemLookup),
	"issue.work_item_update.crm_contact_ids":                         clearUnexercised(reasonWorkItemLookup),
	"issue.work_item_update.description":                             clearUnexercised(reasonWorkItemLookup),
	"issue.work_item_update.due_date":                                clearUnexercised(reasonWorkItemLookup),
	"issue.work_item_update.iteration_id":                            clearUnexercised(reasonWorkItemLookup),
	"issue.work_item_update.milestone_id":                            clearUnexercised(reasonWorkItemLookup),
	"issue.work_item_update.parent_id":                               clearUnexercised(reasonWorkItemLookup),
	"issue.work_item_update.remove_label_ids":                        clearNotStored(reasonLabelOperation),
	"issue.work_item_update.start_date":                              clearUnexercised(reasonWorkItemLookup),
	"issue.work_item_update.title":                                   clearRequired("a work item has a title"),
	"issue.work_item_update.weight":                                  clearUnexercised(reasonWorkItemLookup),
	"merge_request.approval_rule_update.group_ids":                   clearUnverified,
	"merge_request.approval_rule_update.name":                        clearRequired("an approval rule has a name"),
	"merge_request.approval_rule_update.user_ids":                    clearUnverified,
	"merge_request.update.add_labels":                                clearNotStored(reasonLabelOperation),
	"merge_request.update.milestone":                                 clearNotStored(reasonMilestoneTitle),
	"merge_request.update.remove_labels":                             clearNotStored(reasonLabelOperation),
	"merge_request.update.target_branch":                             clearRequired("a merge request targets a branch"),
	"merge_request.update.title":                                     clearRequired("a merge request has a title"),
	"mr_review.discussion_note_update.body":                          clearRequired("a note has a body"),
	"mr_review.discussion_note_update.created_at":                    clearNotStored(reasonAdminMoment),
	"mr_review.draft_note_update.note":                               clearRequired("a draft note has a body"),
	"package.protection_rule_update.minimum_access_level_for_delete": clearUnverified,
	"package.protection_rule_update.minimum_access_level_for_push":   clearUnverified,
	"package.registry_rule_update.repository_path_pattern":           clearRequired("a protection rule matches a repository path pattern"),
	"package.registry_tag_rule_update.tag_name_pattern":              clearRequired("a tag protection rule matches a tag name pattern"),
	"pipeline.schedule_update.cron":                                  clearRequired("a schedule has a cron expression"),
	"pipeline.schedule_update.cron_timezone":                         clearUnverified,
	"pipeline.schedule_update.description":                           clearRequired("GitLab validates the presence of a schedule's description (app/models/ci/pipeline_schedule.rb)"),
	"pipeline.schedule_update.inputs":                                clearUnverified,
	"pipeline.schedule_update.ref":                                   clearRequired("a schedule runs on a ref"),
	"pipeline.trigger_update.description":                            clearUnverified,
	"project.approval_rule_update.group_ids":                         clearUnverified,
	"project.approval_rule_update.name":                              clearRequired("an approval rule has a name"),
	"project.approval_rule_update.protected_branch_ids":              clearUnverified,
	"project.approval_rule_update.user_ids":                          clearUnverified,
	"project.approval_rule_update.usernames":                         clearUnverified,
	"project.badge_edit.image_url":                                   clearRequired(reasonBadgeURL),
	"project.badge_edit.link_url":                                    clearRequired(reasonBadgeURL),
	"project.badge_edit.name":                                        clearUnverified,
	"project.board_update.name":                                      clearRequired("a board has a name"),
	"project.hook_edit.branch_filter_strategy":                       clearOneOf("one of wildcard, regex or all_branches"),
	"project.hook_edit.custom_headers":                               clearUnverified,
	"project.hook_edit.name":                                         clearUnverified,
	"project.hook_edit.signing_token":                                clearNotStored(reasonHookSecret),
	"project.hook_edit.token":                                        clearNotStored(reasonHookSecret),
	"project.hook_edit.url":                                          clearRequired(reasonWebhookURL),
	"project.label_update.color":                                     clearRequired("a label has a color"),
	"project.label_update.new_name":                                  clearRequired("a label has a name"),
	"project.member_edit.expires_at":                                 clearUnverified,
	"project.milestone_update.title":                                 clearRequired("a milestone has a title"),
	"project.mirror_edit.host_keys":                                  clearUnverified,
	"project.mirror_edit.mirror_branch_regex":                        clearUnverified,
	"project.pages_domain_update.certificate":                        clearUnverified,
	"project.pages_domain_update.key":                                clearUnverified,
	"project.service_account_update.email":                           clearUnverified,
	"project.service_account_update.name":                            clearRequired(reasonServiceAccount),
	"project.service_account_update.username":                        clearRequired(reasonServiceAccount),
	"project.update.analytics_access_level":                          clearOneOf(reasonAccessLevelChoice),
	"project.update.builds_access_level":                             clearOneOf(reasonAccessLevelChoice),
	"project.update.ci_id_token_sub_claim_components":                clearUnverified,
	"project.update.container_registry_access_level":                 clearOneOf(reasonAccessLevelChoice),
	"project.update.default_branch":                                  clearRequired("a project's default branch names a branch"),
	"project.update.environments_access_level":                       clearOneOf(reasonAccessLevelChoice),
	"project.update.feature_flags_access_level":                      clearOneOf(reasonAccessLevelChoice),
	"project.update.forking_access_level":                            clearOneOf(reasonAccessLevelChoice),
	"project.update.import_url":                                      clearNotStored("starts an import from the address, not a value an update empties"),
	"project.update.infrastructure_access_level":                     clearOneOf(reasonAccessLevelChoice),
	"project.update.issues_access_level":                             clearOneOf(reasonAccessLevelChoice),
	"project.update.issues_template":                                 clearUnverified,
	"project.update.merge_requests_access_level":                     clearOneOf(reasonAccessLevelChoice),
	"project.update.merge_requests_template":                         clearUnverified,
	"project.update.mirror_branch_regex":                             clearUnverified,
	"project.update.model_experiments_access_level":                  clearOneOf(reasonAccessLevelChoice),
	"project.update.model_registry_access_level":                     clearOneOf(reasonAccessLevelChoice),
	"project.update.monitor_access_level":                            clearOneOf(reasonAccessLevelChoice),
	"project.update.name":                                            clearRequired("a project has a name"),
	"project.update.package_registry_access_level":                   clearOneOf(reasonAccessLevelChoice),
	"project.update.pages_access_level":                              clearOneOf(reasonAccessLevelChoice),
	"project.update.path":                                            clearRequired("a project has a path"),
	"project.update.releases_access_level":                           clearOneOf(reasonAccessLevelChoice),
	"project.update.repository_access_level":                         clearOneOf(reasonAccessLevelChoice),
	"project.update.repository_storage":                              clearNotStored("moves the repository to another storage, not a value an update empties"),
	"project.update.requirements_access_level":                       clearOneOf(reasonAccessLevelChoice),
	"project.update.security_and_compliance_access_level":            clearOneOf(reasonAccessLevelChoice),
	"project.update.snippets_access_level":                           clearOneOf(reasonAccessLevelChoice),
	"project.update.wiki_access_level":                               clearOneOf(reasonAccessLevelChoice),
	"release.update.description":                                     clearUnverified,
	"release.update.name":                                            clearUnverified,
	"release.update.released_at":                                     clearUnverified,
	"repository.commit_discussion_update_note.created_at":            clearNotStored(reasonAdminMoment),
	"repository.file_update.author_email":                            clearNotStored(reasonCommitMetadata),
	"repository.file_update.author_name":                             clearNotStored(reasonCommitMetadata),
	"repository.file_update.last_commit_id":                          clearNotStored(reasonCommitMetadata),
	"repository.file_update.start_branch":                            clearNotStored(reasonCommitMetadata),
	"repository.update_submodule.commit_message":                     clearNotStored(reasonCommitMetadata),
	"runner.update.access_level":                                     clearOneOf("one of not_protected or ref_protected"),
	"runner.update.description":                                      clearUnverified,
	"runner.update.maintenance_note":                                 clearUnverified,
	"runner.update.tag_list":                                         clearUnverified,
	"snippet.discussion_update_note.created_at":                      clearNotStored(reasonAdminMoment),
	"snippet.project_update.content":                                 clearUnverified,
	"snippet.project_update.file_name":                               clearUnverified,
	"snippet.project_update.files":                                   clearNotStored("each entry is a file action (create, update, delete or move), so an empty list changes nothing"),
	"snippet.project_update.title":                                   clearRequired("a snippet has a title"),
	"snippet.update.content":                                         clearUnverified,
	"snippet.update.file_name":                                       clearUnverified,
	"snippet.update.files":                                           clearNotStored("each entry is a file action (create, update, delete or move), so an empty list changes nothing"),
	"snippet.update.title":                                           clearRequired("a snippet has a title"),
	"user.modify.commit_email":                                       clearUnverified,
	"user.modify.email":                                              clearRequired("a user has a primary email"),
	"user.modify.extern_uid":                                         clearNotStored("links an external identity, an operation rather than a stored value"),
	"user.modify.job_title":                                          clearUnverified,
	"user.modify.linkedin":                                           clearUnverified,
	"user.modify.location":                                           clearUnverified,
	"user.modify.name":                                               clearRequired("a user has a name"),
	"user.modify.note":                                               clearUnverified,
	"user.modify.organization":                                       clearUnverified,
	"user.modify.password":                                           clearNotStored(reasonHookSecret),
	"user.modify.provider":                                           clearNotStored("links an external identity, an operation rather than a stored value"),
	"user.modify.public_email":                                       clearUnverified,
	"user.modify.twitter":                                            clearUnverified,
	"user.modify.username":                                           clearRequired("a user has a username"),
	"user.notification_global_update.notification_email":             clearUnverified,
	"user.notification_group_update.notification_email":              clearUnverified,
	"user.notification_project_update.notification_email":            clearUnverified,
	"user.update_service_account.email":                              clearUnverified,
	"user.update_service_account.name":                               clearRequired(reasonServiceAccount),
	"user.update_service_account.username":                           clearRequired(reasonServiceAccount),
	"wiki.update.content":                                            clearUnverified,
	"wiki.update.title":                                              clearRequired("a wiki page has a title"),
}

// updateActionName selects the actions the clearing gate judges: the writes
// that change an object that already exists.
var updateActionName = regexp.MustCompile(`(^|_)(update|edit|modify)($|_)`)

// graphQLMutation tells a GraphQL write from a read in a request body's query.
var graphQLMutation = regexp.MustCompile(`\bmutation\b`)

// TestUpdateActions_AnEmptyValueReachesGitLabOrIsDeclared is the gate issue
// 1306 asked for. Update handlers across the catalog decided whether a field
// was sent by testing it for emptiness, so the one value that empties a field
// in GitLab was the one value that could never be sent: the caller was told
// the update succeeded, or that it sent nothing, and the field kept its value.
//
// The gate calls every update action of the Ultimate catalog once per optional
// text, list or integer field, with that field set to "", [] or 0 and the
// required fields filled, against a stand-in GitLab that records what it is
// sent. 0 is the integer's probe because it is the value that clears an ID
// reference, and the one value a guard reading "set" as "positive" drops. A
// field passes when some write carries it with that empty value or null, as a
// member at the top of a REST body, anywhere in a GraphQL request's variables
// (in camel case there), or as a query parameter
// ([clearingWrite.carriesEmpty]). Any other field must be answered in
// [clearingDeclarations], and an entry the probe no longer needs fails too.
//
// The stand-in answers with one generic object or list per request, so the
// probe sees the first write each handler makes and nothing about GitLab's
// answer. It is built by hand rather than with testutil.NewTestClient on
// purpose: the requests carry the probe's fill values, which are not what this
// server sends GitLab, and recording them would put every update route in the
// request inventory under this package.
func TestUpdateActions_AnEmptyValueReachesGitLabOrIsDeclared(t *testing.T) {
	probe := newClearingProbe(t)
	catalog, err := BuildActionCatalog(probe.client, ActionCatalogOptions{Tier: edition.Ultimate, IncludeMCP: true})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}

	judged := make(map[string]bool)
	var undeclared, stale []string
	for _, action := range catalog.Actions() {
		if action.ReadOnly || !updateActionName.MatchString(action.Name) {
			continue
		}
		base, fields := clearingProbeInput(action.Route.InputSchema)
		for _, field := range fields {
			key := string(action.ID) + "." + field.name
			judged[key] = true
			outcome := probe.send(t, action.Route.Handler, base, field)
			_, declared := clearingDeclarations[key]
			switch {
			case outcome.reached && declared:
				stale = append(stale, key+": the empty value now reaches GitLab")
			case !outcome.reached && !declared:
				undeclared = append(undeclared, key+": "+outcome.String())
			}
		}
	}
	for key := range clearingDeclarations {
		if !judged[key] {
			stale = append(stale, key+": no update action of the catalog has this optional field")
		}
	}

	sort.Strings(undeclared)
	sort.Strings(stale)
	if len(undeclared) > 0 {
		t.Errorf("%d update field(s) drop the empty value a caller sets to clear them; send it (nil decides presence, never emptiness) or declare why in clearingDeclarations:\n  %s",
			len(undeclared), strings.Join(undeclared, "\n  "))
	}
	if len(stale) > 0 {
		t.Errorf("%d clearing declaration(s) answer nothing; delete them:\n  %s", len(stale), strings.Join(stale, "\n  "))
	}
}

// TestClearingWrite_CarriesEmpty_HoldsTheValueAndThePlace pins what the gate
// counts as a field reaching GitLab empty: the empty value or null, where a
// field the caller sets lands. A write that re-sends the stored value, or that
// carries another object's member of the same name, is not a clear, and the
// gate counted both as one while it matched a key at any depth with any value.
func TestClearingWrite_CarriesEmpty_HoldsTheValueAndThePlace(t *testing.T) {
	text := clearingField{name: "description", empty: ""}
	list := clearingField{name: "labels", empty: []any{}}
	number := clearingField{name: "assignee_id", empty: 0}
	for _, tc := range []struct {
		name  string
		write clearingWrite
		field clearingField
		want  bool
	}{
		{"empty text at the top of the body", clearingWrite{body: map[string]any{"description": ""}}, text, true},
		{"null at the top of the body", clearingWrite{body: map[string]any{"description": nil}}, text, true},
		{"the stored text sent back", clearingWrite{body: map[string]any{"description": "kept"}}, text, false},
		{"another object's member of the same name", clearingWrite{body: map[string]any{"owner": map[string]any{"description": ""}}}, text, false},
		{"empty list", clearingWrite{body: map[string]any{"labels": []any{}}}, list, true},
		{"empty comma-separated list", clearingWrite{body: map[string]any{"labels": ""}}, list, true},
		{"a list that still holds a label", clearingWrite{body: map[string]any{"labels": []any{"bug"}}}, list, false},
		{"zero", clearingWrite{body: map[string]any{"assignee_id": float64(0)}}, number, true},
		{"the stored number sent back", clearingWrite{body: map[string]any{"assignee_id": float64(7)}}, number, false},
		{"zero in the query", clearingWrite{query: map[string][]string{"assignee_id": {"0"}}}, number, true},
		{"empty list in the query", clearingWrite{query: map[string][]string{"labels[]": {""}}}, list, true},
		{"a value in the query", clearingWrite{query: map[string][]string{"description": {"kept"}}}, text, false},
		{"camel case in GraphQL variables", clearingWrite{graphQL: true, body: map[string]any{"variables": map[string]any{"input": map[string]any{"assigneeId": nil}}}}, number, true},
		{"the stored value in GraphQL variables", clearingWrite{graphQL: true, body: map[string]any{"variables": map[string]any{"input": map[string]any{"description": "kept"}}}}, text, false},
		{"a list of inputs in GraphQL variables", clearingWrite{graphQL: true, body: map[string]any{"variables": map[string]any{"inputs": []any{map[string]any{"labels": []any{}}}}}}, list, true},
		{"a GraphQL member outside the variables", clearingWrite{graphQL: true, body: map[string]any{"description": ""}}, text, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.write.carriesEmpty(tc.field); got != tc.want {
				t.Errorf("carriesEmpty(%s) = %t, want %t for %+v", tc.field.name, got, tc.want, tc.write)
			}
		})
	}
}

// clearingField is one optional field the gate empties.
type clearingField struct {
	name  string
	empty any
}

// clearingOutcome is what one probe call did with the empty value.
type clearingOutcome struct {
	reached bool
	writes  int
	err     error
}

// String describes an outcome the gate reports.
func (o clearingOutcome) String() string {
	switch {
	case o.writes == 0 && o.err != nil:
		return "no write was sent: " + o.err.Error()
	case o.writes == 0:
		return "no write was sent"
	default:
		return fmt.Sprintf("none of the %d write(s) carried it", o.writes)
	}
}

// clearingProbeInput reads an input schema into the parameters every probe of
// the action carries (its required fields and the first set of any
// alternative identifiers, filled) and the optional fields to empty one at a
// time: the text and integer fields with no enumeration, and the lists.
func clearingProbeInput(schema map[string]any) (map[string]any, []clearingField) {
	properties := asMap(schema["properties"])
	required := schemaNames(schema["required"])
	identifiers := map[string]bool{}
	for _, keyword := range []string{"anyOf", "oneOf"} {
		branches, _ := schema[keyword].([]any)
		for index, branch := range branches {
			names := schemaNames(asMap(branch)["required"])
			for _, name := range names {
				identifiers[name] = true
			}
			if index == 0 {
				required = append(required, names...)
			}
		}
	}
	base := make(map[string]any, len(required))
	for _, name := range required {
		base[name] = clearingFill(asMap(properties[name]))
		identifiers[name] = true
	}
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	sort.Strings(names)
	var fields []clearingField
	for _, name := range names {
		if identifiers[name] {
			continue
		}
		property := asMap(properties[name])
		switch schemaTypeName(property) {
		case "string":
			if property["enum"] == nil {
				fields = append(fields, clearingField{name: name, empty: ""})
			}
		case "array":
			fields = append(fields, clearingField{name: name, empty: []any{}})
		case "integer":
			if property["enum"] == nil {
				fields = append(fields, clearingField{name: name, empty: 0})
			}
		}
	}
	return base, fields
}

// schemaNames reads a schema keyword holding a list of names, in either of the
// two shapes the catalog's schemas carry it.
func schemaNames(raw any) []string {
	switch names := raw.(type) {
	case []string:
		return slices.Clone(names)
	case []any:
		out := make([]string, 0, len(names))
		for _, name := range names {
			out = append(out, fmt.Sprint(name))
		}
		return out
	}
	return nil
}

// asMap reads a schema node as an object, or as an empty one.
func asMap(raw any) map[string]any {
	node, _ := raw.(map[string]any)
	return node
}

// schemaTypeName reads the JSON type of a schema property, skipping the null
// a nullable property lists beside it.
func schemaTypeName(property map[string]any) string {
	switch kind := property["type"].(type) {
	case string:
		return kind
	case []any:
		for _, each := range kind {
			if name, ok := each.(string); ok && name != "null" {
				return name
			}
		}
	}
	return ""
}

// clearingFill is the value the probe gives a required field: the first value
// of an enumeration, or a value of the field's type.
func clearingFill(property map[string]any) any {
	if values, ok := property["enum"].([]any); ok && len(values) > 0 {
		return values[0]
	}
	switch schemaTypeName(property) {
	case "integer", "number":
		return 1
	case "boolean":
		return true
	case "array":
		return []any{"1"}
	case "object":
		return map[string]any{}
	}
	return "1"
}

// clearingProbe is the stand-in GitLab the gate drives every handler against,
// and the record of the writes it received during one call.
type clearingProbe struct {
	client *gitlabclient.Client
	mu     sync.Mutex
	writes []clearingWrite
}

// clearingWrite is one write request as the probe judges it.
type clearingWrite struct {
	query   map[string][]string
	body    any
	graphQL bool
}

// carriesEmpty reports whether the write sends field with the value that
// empties it, or null. The value is read where a field the caller sets can
// land and nowhere else: a query parameter, a member at the top of a REST
// body, or a member anywhere in a GraphQL request's variables, which nest
// under an input object and spell the name in camel case. Holding the value
// and the place is what keeps the gate from counting a write that re-sends
// the stored value, or another object's member of the same name, as a clear.
func (w clearingWrite) carriesEmpty(field clearingField) bool {
	for _, key := range []string{field.name, field.name + "[]"} {
		if values, ok := w.query[key]; ok && emptyQueryValues(values, field.empty) {
			return true
		}
	}
	body := asMap(w.body)
	if !w.graphQL {
		value, ok := body[field.name]
		return ok && emptyJSONValue(value, field.empty)
	}
	return variablesCarryEmpty(body["variables"], field)
}

// variablesCarryEmpty reports whether a GraphQL request's variables hold the
// field, under its own name or its camel-case spelling, with the value that
// empties it, at any depth.
func variablesCarryEmpty(node any, field clearingField) bool {
	camel := camelCase(field.name)
	switch value := node.(type) {
	case map[string]any:
		for key, member := range value {
			if (key == field.name || key == camel) && emptyJSONValue(member, field.empty) {
				return true
			}
			if variablesCarryEmpty(member, field) {
				return true
			}
		}
	case []any:
		for _, member := range value {
			if variablesCarryEmpty(member, field) {
				return true
			}
		}
	}
	return false
}

// emptyJSONValue reports whether a decoded JSON value is null or the empty
// value the probe sent: "" for a text, 0 for an integer, and for a list [] or
// the "" client-go writes for an empty comma-separated one.
func emptyJSONValue(got, probe any) bool {
	if got == nil {
		return true
	}
	switch probe.(type) {
	case string:
		return got == ""
	case int:
		number, ok := got.(float64)
		return ok && number == 0
	default:
		list, isList := got.([]any)
		return got == "" || isList && len(list) == 0
	}
}

// emptyQueryValues reports whether every value a query parameter carries is
// the empty value the probe sent: "0" for an integer, "" otherwise.
func emptyQueryValues(values []string, probe any) bool {
	want := ""
	if _, ok := probe.(int); ok {
		want = "0"
	}
	for _, value := range values {
		if value != want {
			return false
		}
	}
	return len(values) > 0
}

// newClearingProbe starts the stand-in and a client on it.
func newClearingProbe(t *testing.T) *clearingProbe {
	t.Helper()
	probe := &clearingProbe{}
	srv := httptest.NewServer(http.HandlerFunc(probe.serve))
	t.Cleanup(srv.Close)
	client, err := gitlabclient.NewClient(&config.Config{GitLabURL: srv.URL, GitLabToken: "test-token", DisableRetries: true})
	if err != nil {
		t.Fatalf("create the probe's GitLab client: %v", err)
	}
	probe.client = client
	return probe
}

// serve records a write and answers every request with one generic object, or
// a list of one for a read of a collection.
func (p *clearingProbe) serve(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body any
	_ = json.Unmarshal(raw, &body)
	graphQL := r.URL.Path == "/api/graphql"
	write := r.Method != http.MethodGet && r.Method != http.MethodHead
	if graphQL {
		query, _ := asMap(body)["query"].(string)
		write = graphQLMutation.MatchString(query)
	}
	if write {
		p.mu.Lock()
		p.writes = append(p.writes, clearingWrite{query: r.URL.Query(), body: body, graphQL: graphQL})
		p.mu.Unlock()
	}
	w.Header().Set("Content-Type", "application/json")
	segments := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	last := segments[len(segments)-1]
	switch {
	case graphQL:
		_, _ = io.WriteString(w, `{"data":{}}`)
	case r.Method == http.MethodGet && strings.Trim(last, "0123456789") != "":
		_, _ = io.WriteString(w, `[{"id":1,"iid":1,"name":"x","key":"K"}]`)
	default:
		_, _ = io.WriteString(w, `{"id":1,"iid":1,"name":"x"}`)
	}
}

// send calls the handler with field emptied and reports whether a write
// carried it.
func (p *clearingProbe) send(t *testing.T, handler toolutil.ActionFunc, base map[string]any, field clearingField) clearingOutcome {
	t.Helper()
	params := make(map[string]any, len(base)+1)
	maps.Copy(params, base)
	params[field.name] = field.empty
	p.mu.Lock()
	p.writes = nil
	p.mu.Unlock()
	_, err := handler(t.Context(), params)
	p.mu.Lock()
	defer p.mu.Unlock()
	outcome := clearingOutcome{writes: len(p.writes), err: err}
	for _, write := range p.writes {
		if write.carriesEmpty(field) {
			outcome.reached = true
		}
	}
	return outcome
}

// camelCase spells a snake-case member name the way a GraphQL input does.
func camelCase(name string) string {
	words := strings.Split(name, "_")
	for index := 1; index < len(words); index++ {
		if words[index] != "" {
			words[index] = strings.ToUpper(words[index][:1]) + words[index][1:]
		}
	}
	return strings.Join(words, "")
}
