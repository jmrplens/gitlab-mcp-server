package dynamic

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/actionregistry"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestSearch_RanksMatchingActions verifies that Search prioritizes the most
// specific destructive action when query terms match both the domain and action.
func TestSearch_RanksMatchingActions(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, output, err := registry.Search(t.Context(), nil, SearchInput{Query: "project delete", Limit: 5})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Search() result = %+v, want non-error", result)
	}
	if output.Count == 0 {
		t.Fatal("Search() returned no matches")
	}
	if output.Results[0].ID != "project.delete" {
		t.Fatalf("top result ID = %q, want project.delete", output.Results[0].ID)
	}
	if !output.Results[0].Destructive {
		t.Fatal("top result Destructive = false, want true")
	}
}

// TestSearch_RequiresQuery verifies that Search returns an MCP tool error when
// the caller omits the query text.
func TestSearch_RequiresQuery(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, _, err := registry.Search(t.Context(), nil, SearchInput{})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("Search() result = %+v, want tool error", result)
	}
}

// TestSearch_RanksAliasMatches verifies that human-friendly aliases such as
// "webhook create" rank the canonical project hook action first.
func TestSearch_RanksAliasMatches(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, output, err := registry.Search(t.Context(), nil, SearchInput{Query: "webhook create", Limit: 3})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Search() result = %+v, want non-error", result)
	}
	if output.Count == 0 || output.Results[0].ID != "project.hook_add" {
		t.Fatalf("top result = %+v, want project.hook_add", output.Results)
	}
}

// TestSearch_UsesIntentSynonymsAndTags verifies that Search expands common
// intent words and tags before ranking dynamic actions.
func TestSearch_UsesIntentSynonymsAndTags(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	tests := []struct {
		name  string
		query string
		want  string
	}{
		{name: "merge request abbreviation", query: "mr approve", want: "merge_request.approve"},
		{name: "issue close intent", query: "close issue", want: "issue.update"},
		{name: "ci secret intent", query: "ci secret", want: "ci_variable.create"},
		{name: "project metadata intent", query: "project metadata", want: "project.get"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, output, err := registry.Search(t.Context(), nil, SearchInput{Query: tt.query, Limit: 3})
			if err != nil {
				t.Fatalf("Search() error = %v", err)
			}
			if result == nil || result.IsError {
				t.Fatalf("Search() result = %+v, want non-error", result)
			}
			if output.Count == 0 || output.Results[0].ID != tt.want {
				t.Fatalf("top result = %+v, want %s", output.Results, tt.want)
			}
		})
	}
}

// TestSearch_ExactCanonicalIDBeatsBroadText verifies that an exact canonical
// action ID outranks broader textual matches for the same domain.
func TestSearch_ExactCanonicalIDBeatsBroadText(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, output, err := registry.Search(t.Context(), nil, SearchInput{Query: "project.list", Limit: 3})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Search() result = %+v, want non-error", result)
	}
	if output.Count == 0 || output.Results[0].ID != "project.list" {
		t.Fatalf("top result = %+v, want project.list", output.Results)
	}
}

// TestAddStandaloneRoutes_AddsDynamicActions verifies that standalone dynamic
// routes are indexed alongside captured meta-tool routes.
func TestAddStandaloneRoutes_AddsDynamicActions(t *testing.T) {
	routes, err := AddStandaloneRoutes(nil, nil, StandaloneOptions{})
	if err != nil {
		t.Fatalf("AddStandaloneRoutes() error = %v", err)
	}
	registry := NewRegistry(routes)

	tests := []string{
		"discover_project.resolve",
		"interactive.issue_create",
		"interactive.mr_create",
		"interactive.project_create",
		"interactive.release_create",
	}
	for _, actionID := range tests {
		t.Run(actionID, func(t *testing.T) {
			if _, ok := registry.resolveAction(actionID); !ok {
				t.Fatalf("resolveAction(%q) = false, want true", actionID)
			}
		})
	}
}

// TestAddStandaloneRoutes_HonorsReadOnlyAndExclusions verifies that standalone
// route registration respects read-only mode and explicit tool exclusions.
func TestAddStandaloneRoutes_HonorsReadOnlyAndExclusions(t *testing.T) {
	routes, err := AddStandaloneRoutes(nil, nil, StandaloneOptions{
		ReadOnly:     true,
		ExcludeTools: []string{"gitlab_discover_project"},
	})
	if err != nil {
		t.Fatalf("AddStandaloneRoutes() error = %v", err)
	}
	registry := NewRegistry(routes)

	if _, ok := registry.resolveAction("discover_project.resolve"); ok {
		t.Fatal("discover_project.resolve is present, want excluded")
	}
	if _, ok := registry.resolveAction("interactive.issue_create"); ok {
		t.Fatal("interactive.issue_create is present in read-only mode")
	}
}

// TestAddStandaloneCatalog_MatchesRouteCompatibilityWrapper verifies that the
// catalog-native standalone builder preserves the old route-map wrapper output.
func TestAddStandaloneCatalog_MatchesRouteCompatibilityWrapper(t *testing.T) {
	routes := testRoutes(t)
	standaloneRoutes, err := AddStandaloneRoutes(routes, nil, StandaloneOptions{})
	if err != nil {
		t.Fatalf("AddStandaloneRoutes() error = %v", err)
	}
	standaloneCatalog, err := AddStandaloneCatalog(actionregistry.FromActionMaps(routes), nil, StandaloneOptions{})
	if err != nil {
		t.Fatalf("AddStandaloneCatalog() error = %v", err)
	}
	fromRoutes := NewRegistry(standaloneRoutes)
	fromCatalog := NewRegistryFromCatalog(standaloneCatalog)

	for _, actionID := range []string{"project.list", "discover_project.resolve", "interactive.issue_create"} {
		if _, ok := fromRoutes.resolveAction(actionID); !ok {
			t.Fatalf("route wrapper registry missing %s", actionID)
		}
		if _, ok := fromCatalog.resolveAction(actionID); !ok {
			t.Fatalf("catalog registry missing %s", actionID)
		}
	}
}

// TestAddStandaloneCatalog_NilCatalogWithExcludedInteractiveActions verifies
// nil catalogs are supported and no empty interactive group is added.
func TestAddStandaloneCatalog_NilCatalogWithExcludedInteractiveActions(t *testing.T) {
	catalog, err := AddStandaloneCatalog(nil, nil, StandaloneOptions{ExcludeTools: []string{
		"gitlab_interactive_issue_create",
		"gitlab_interactive_mr_create",
		"gitlab_interactive_project_create",
		"gitlab_interactive_release_create",
	}})
	if err != nil {
		t.Fatalf("AddStandaloneCatalog() error = %v", err)
	}
	registry := NewRegistryFromCatalog(catalog)

	if _, ok := registry.resolveAction("discover_project.resolve"); !ok {
		t.Fatal("discover_project.resolve missing")
	}
	if _, ok := registry.resolveAction("interactive.issue_create"); ok {
		t.Fatal("interactive.issue_create present, want excluded")
	}
}

// TestNewRegistryFromCatalog_UsesCatalogAliasesAndTags verifies that dynamic
// mode can consume registry-native action metadata without rebuilding it from
// legacy route maps.
func TestNewRegistryFromCatalog_UsesCatalogAliasesAndTags(t *testing.T) {
	catalog := actionregistry.NewCatalog()
	group := actionregistry.NewGroup(actionregistry.GroupOptions{ToolName: "gitlab_custom"})
	group.SetAction(actionregistry.Action{
		Name:    "inspect",
		Aliases: []string{"custom.lookup"},
		Tags:    []string{"bespoke"},
		Route: toolutil.ActionRoute{
			Handler: func(_ context.Context, params map[string]any) (any, error) {
				return map[string]any{"target": params["target"]}, nil
			},
			InputSchema: map[string]any{
				"type":     "object",
				"required": []any{"target"},
				"properties": map[string]any{
					"target": map[string]any{"type": "string"},
				},
			},
		},
	})
	if err := catalog.AddGroup(group); err != nil {
		t.Fatalf("AddGroup() error = %v", err)
	}

	registry := NewRegistryFromCatalog(catalog)
	result, output, err := registry.Search(t.Context(), nil, SearchInput{Query: "bespoke", Limit: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Search() result = %+v, want non-error", result)
	}
	if output.Count != 1 || output.Results[0].ID != "custom.inspect" {
		t.Fatalf("Search() output = %+v, want custom.inspect", output)
	}

	result, described, err := registry.Describe(t.Context(), nil, DescribeInput{Action: "custom.lookup"})
	if err != nil {
		t.Fatalf("Describe() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Describe() result = %+v, want non-error", result)
	}
	if described.Count != 1 || described.Actions[0].ID != "custom.inspect" {
		t.Fatalf("Describe() output = %+v, want custom.inspect", described)
	}
}

// TestNewRegistryFromCatalog_NilCatalog verifies callers can pass a nil catalog
// during transitional setup without panicking.
func TestNewRegistryFromCatalog_NilCatalog(t *testing.T) {
	registry := NewRegistryFromCatalog(nil)

	result, output, err := registry.Search(t.Context(), nil, SearchInput{Query: "project", Limit: 3})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Search() result = %+v, want non-error empty result", result)
	}
	if output.Count != 0 {
		t.Fatalf("Search() Count = %d, want 0", output.Count)
	}
}

// TestDescribe_CanonicalizesStandaloneAlias verifies that Describe resolves a
// standalone MCP tool name to its canonical dynamic action ID.
func TestDescribe_CanonicalizesStandaloneAlias(t *testing.T) {
	routes, err := AddStandaloneRoutes(nil, nil, StandaloneOptions{})
	if err != nil {
		t.Fatalf("AddStandaloneRoutes() error = %v", err)
	}
	registry := NewRegistry(routes)

	result, output, err := registry.Describe(t.Context(), nil, DescribeInput{Action: "gitlab_interactive_issue_create"})
	if err != nil {
		t.Fatalf("Describe() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Describe() result = %+v, want non-error", result)
	}
	if output.Count != 1 || output.Actions[0].ID != "interactive.issue_create" {
		t.Fatalf("actions = %+v, want canonical interactive.issue_create", output.Actions)
	}
}

// TestDescribe_ReturnsSchemaAndExample verifies that Describe returns action
// metadata, destructive hints, input schema, and an executable example.
func TestDescribe_ReturnsSchemaAndExample(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, output, err := registry.Describe(t.Context(), nil, DescribeInput{Action: "project.delete"})
	if err != nil {
		t.Fatalf("Describe() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Describe() result = %+v, want non-error", result)
	}
	if output.Count != 1 {
		t.Fatalf("Describe() Count = %d, want 1", output.Count)
	}
	action := output.Actions[0]
	if action.ID != "project.delete" || !action.Destructive {
		t.Fatalf("action = %+v, want project.delete destructive", action)
	}
	if _, ok := action.InputSchema["x_destructive"]; !ok {
		t.Fatalf("InputSchema missing x_destructive: %+v", action.InputSchema)
	}
	if action.Example.Arguments["confirm"] != true {
		t.Fatalf("example missing confirm param: %+v", action.Example)
	}
}

// TestDescribe_IncludesOutputSchema verifies that dynamic descriptions expose
// the action result schema when the backing catalog route has one.
func TestDescribe_IncludesOutputSchema(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, output, err := registry.Describe(t.Context(), nil, DescribeInput{Action: "project.get"})
	if err != nil {
		t.Fatalf("Describe() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Describe() result = %+v, want non-error", result)
	}
	description := output.Actions[0]
	properties := schemaProperties(description.OutputSchema)
	if _, ok := properties["project_id"]; !ok {
		t.Fatalf("OutputSchema properties = %v, want project_id", properties)
	}
}

// TestDescribe_MetaCatalogSchemas verifies that Describe returns input schemas
// and includes output schemas when route metadata provides them.
func TestDescribe_MetaCatalogSchemas(t *testing.T) {
	registry := realCatalogRegistry(t)

	result, output, err := registry.Describe(t.Context(), nil, DescribeInput{Actions: []string{
		"project.list",
		"merge_request.list",
		"user.current_user_status",
		"user.list",
	}})
	if err != nil {
		t.Fatalf("Describe() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Describe() result = %+v, want non-error", result)
	}
	if output.Count != 4 {
		t.Fatalf("Describe() Count = %d, want 4", output.Count)
	}
	structured, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("json.Marshal(DescribeOutput) error = %v", err)
	}
	if !strings.Contains(string(structured), "output_schema") {
		t.Fatalf("DescribeOutput JSON missing output_schema: %s", structured)
	}
	markdown := textContent(result)
	for _, notWant := range []string{"input_schema", "output_schema"} {
		if strings.Contains(markdown, notWant) {
			t.Fatalf("Describe() markdown contains %q: %s", notWant, markdown)
		}
	}
	if !strings.Contains(markdown, "**Input schema**") || !strings.Contains(markdown, "```json") || !strings.Contains(markdown, "properties") {
		t.Fatalf("Describe() markdown missing compact input schema: %s", markdown)
	}

	projectList := actionDescriptionByID(t, output, "project.list")
	assertSchemaHasProperties(t, projectList.InputSchema, "search", "owned", "per_page")
	if projectList.OutputSchema == nil {
		t.Fatal("project.list OutputSchema is nil")
	}
	if len(projectList.RequiredParams) != 0 {
		t.Fatalf("project.list RequiredParams = %v, want none", projectList.RequiredParams)
	}

	mergeRequestList := actionDescriptionByID(t, output, "merge_request.list")
	assertSchemaHasProperties(t, mergeRequestList.InputSchema, "project_id", "state", "author_username", "scope")
	if mergeRequestList.OutputSchema == nil {
		t.Fatal("merge_request.list OutputSchema is nil")
	}
	if !slices.Contains(mergeRequestList.RequiredParams, "project_id") {
		t.Fatalf("merge_request.list RequiredParams = %v, want project_id", mergeRequestList.RequiredParams)
	}
	if got := mergeRequestList.Example.Arguments["params"].(map[string]any)["project_id"]; got != "group/project" {
		t.Fatalf("merge_request.list example project_id = %v, want group/project", got)
	}

	currentUserStatus := actionDescriptionByID(t, output, "user.current_user_status")
	if len(schemaProperties(currentUserStatus.InputSchema)) != 0 {
		t.Fatalf("user.current_user_status input properties = %v, want none", schemaProperties(currentUserStatus.InputSchema))
	}
	if currentUserStatus.OutputSchema == nil {
		t.Fatal("user.current_user_status OutputSchema is nil")
	}

	userList := actionDescriptionByID(t, output, "user.list")
	assertSchemaHasProperties(t, userList.InputSchema, "search", "username", "per_page")
	if userList.OutputSchema == nil {
		t.Fatal("user.list OutputSchema is nil")
	}
}

// TestFind_ReturnsSchemaAndExecuteExample verifies that Find combines search
// ranking with the input schema and execute example needed to call an action.
func TestFind_ReturnsSchemaAndExecuteExample(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, output, err := registry.Find(t.Context(), nil, FindInput{Query: "project delete", Limit: 3})
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Find() result = %+v, want non-error", result)
	}
	if output.Count == 0 || output.Results[0].ID != "project.delete" {
		t.Fatalf("top result = %+v, want project.delete", output.Results)
	}
	found := output.Results[0]
	if !found.Destructive || found.InputSchema == nil {
		t.Fatalf("found result = %+v, want destructive action with schema", found)
	}
	if found.OutputSchema != nil {
		t.Fatalf("found OutputSchema = %v, want nil for route without output schema", found.OutputSchema)
	}
	if found.Example.Tool != "gitlab_execute_tool" || found.Example.Arguments["confirm"] != true {
		t.Fatalf("example = %+v, want execute example with confirm", found.Example)
	}
}

// TestFind_RequiresQuery verifies that Find returns an MCP tool error and empty
// output when the query is omitted.
func TestFind_RequiresQuery(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, output, err := registry.Find(t.Context(), nil, FindInput{})
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("Find() result = %+v, want tool error", result)
	}
	if output.Count != 0 || len(output.Results) != 0 {
		t.Fatalf("Find() output = %+v, want empty output", output)
	}
}

// TestRegisterCatalogFindExecuteTools_ExposesTwoDynamicTools verifies that the dynamic
// two-tool surface exposes only find and execute through an MCP session.
func TestRegisterCatalogFindExecuteTools_ExposesTwoDynamicTools(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "dynamic-test", Version: "0"}, nil)
	RegisterCatalogFindExecuteTools(server, actionregistry.FromActionMaps(testRoutes(t)))

	st, ct := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "dynamic-client", Version: "0"}, nil)
	session, err := client.Connect(t.Context(), ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	tools, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools.Tools) != 2 {
		t.Fatalf("tool count = %d, want 2", len(tools.Tools))
	}
	names := []string{tools.Tools[0].Name, tools.Tools[1].Name}
	if !slices.Contains(names, "gitlab_find_action") || !slices.Contains(names, "gitlab_execute_tool") {
		t.Fatalf("tools = %v, want find/execute", names)
	}
}

// TestDescribe_UnknownActionReturnsToolError verifies that Describe reports an
// MCP tool error for action IDs that are not present in the registry.
func TestDescribe_UnknownActionReturnsToolError(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, _, err := registry.Describe(t.Context(), nil, DescribeInput{Action: "project.missing"})
	if err != nil {
		t.Fatalf("Describe() error = %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("Describe() result = %+v, want tool error", result)
	}
}

// TestDescribe_CanonicalizesAlias verifies that Describe resolves compatibility
// aliases to the canonical action ID before returning metadata.
func TestDescribe_CanonicalizesAlias(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, output, err := registry.Describe(t.Context(), nil, DescribeInput{Action: "project_access_token.create"})
	if err != nil {
		t.Fatalf("Describe() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Describe() result = %+v, want non-error", result)
	}
	if output.Count != 1 || output.Actions[0].ID != "access.token_project_create" {
		t.Fatalf("Describe() output = %+v, want access.token_project_create", output)
	}
}

// TestRequiredParams_IncludesPreferredAlternative verifies that schemas using
// anyOf still produce a useful example branch for search and describe output.
func TestRequiredParams_IncludesPreferredAlternative(t *testing.T) {
	schema := map[string]any{
		"required": []any{"project_id", "title"},
		"anyOf": []any{
			map[string]any{"required": []any{"file_name", "content"}},
			map[string]any{"required": []any{"files"}},
		},
	}

	got := strings.Join(requiredParams(schema), ",")
	if got != "content,file_name,files,project_id,title" {
		t.Fatalf("requiredParams() = %q", got)
	}
}

// TestDescribe_CanonicalizesObservedModelAliases verifies aliases observed in
// model output so dynamic execution remains tolerant of alternate naming.
func TestDescribe_CanonicalizesObservedModelAliases(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	tests := map[string]string{
		"issue.notes":                               "issue.note_list",
		"issue.notes.list":                          "issue.note_list",
		"pipeline.jobs":                             "job.list",
		"project.schedule_storage_move":             "storage_move.schedule_project",
		"merge_request.changes":                     "mr_review.changes_get",
		"merge_request.accept":                      "merge_request.merge",
		"project.hooks.list":                        "project.hook_list",
		"project.status_check_list":                 "external_status_check.list_project",
		"project.status_checks.list":                "external_status_check.list_project",
		"ci_job_token_scope.inbound_allowlist.list": "job.token_scope_list_inbound",
		"package.files":                             "package.file_list",
		"group.audit_events":                        "audit_event.list_group",
		"project.releases.list":                     "release.list",
		"release.generate_notes":                    "analyze.release_notes",
		"deploy_token.create":                       "access.deploy_token_create_project",
		"deploy_key.create":                         "access.deploy_key_add",
		"branch.protected_list":                     "branch.get_protected",
		"branch.update_protection":                  "branch.update_protected",
		"merge_request.set_time_estimate":           "merge_request.time_estimate_set",
		"merge_request.time_estimate":               "merge_request.time_estimate_set",
		"merge_request.time_spent_add":              "merge_request.spent_time_add",
		"mr_review.draft_notes_publish":             "mr_review.draft_note_publish_all",
		"mr_review.publish":                         "mr_review.draft_note_publish_all",
		"package.list_generic":                      "package.list",
		"variable.create":                           "ci_variable.create",
		"group.variable.create":                     "ci_variable.group_create",
		"project_member.update":                     "project.member_edit",
		"project.member_remove":                     "project.member_delete",
		"project_member.remove":                     "project.member_delete",
		"webhook.add":                               "project.hook_add",
		"group.ldap_link_delete":                    "group.ldap_link_delete_for_provider",
		"release.create_link":                       "release.link_create",
		"package.list_project":                      "package.list",
	}
	for alias, want := range tests {
		t.Run(alias, func(t *testing.T) {
			result, output, err := registry.Describe(t.Context(), nil, DescribeInput{Action: alias})
			if err != nil {
				t.Fatalf("Describe() error = %v", err)
			}
			if result == nil || result.IsError {
				t.Fatalf("Describe() result = %+v, want non-error", result)
			}
			if output.Count != 1 || output.Actions[0].ID != want {
				t.Fatalf("Describe() output = %+v, want %s", output, want)
			}
		})
	}
}

// TestDescribe_CanonicalizesProviderSpecificAliases verifies alternate action
// IDs observed in provider output against the real action catalog.
func TestDescribe_CanonicalizesProviderSpecificAliases(t *testing.T) {
	registry := realCatalogRegistry(t)

	tests := map[string]string{
		"feature_flag_user_list.create":              "feature_flags.ff_user_list_create",
		"feature_flag_user_list.delete":              "feature_flags.ff_user_list_delete",
		"feature_flags.feature_flag_user_lists_list": "feature_flags.ff_user_list_list",
		"gitlab_issue.create":                        "issue.create",
		"gitlab_server.health_check":                 "server.health_check",
		"job.artifact_download":                      "job.download_single_artifact",
		"issue.link":                                 "issue.link_create",
		"issue.note.create":                          "issue.note_create",
		"issue_note.get":                             "issue.note_get",
		"issue_note.list":                            "issue.note_list",
		"repository_tree":                            "repository.tree",
		"repository_file.get":                        "repository.file_get",
		"repository_file.read":                       "repository.file_get",
		"repository_files.get_raw_file":              "repository.file_raw",
		"pipeline.schedule_variable_create":          "pipeline.schedule_create_variable",
		"pipeline.schedule_variable_update":          "pipeline.schedule_edit_variable",
		"project.badge_update":                       "project.badge_edit",
		"merge_request.time_spent_reset":             "merge_request.spent_time_reset",
		"merge_request.emoji_mr_award_create":        "merge_request.emoji_mr_create",
		"generic_package.list":                       "package.list",
		"issue_note.create":                          "issue.note_create",
		"release_link.link_list":                     "release.link_list",
		"wiki.show":                                  "wiki.get",
		"gitlab_interactive_issue.create":            "interactive.issue_create",
	}

	for alias, want := range tests {
		t.Run(alias, func(t *testing.T) {
			result, output, err := registry.Describe(t.Context(), nil, DescribeInput{Action: alias})
			if err != nil {
				t.Fatalf("Describe() error = %v", err)
			}
			if result == nil || result.IsError {
				t.Fatalf("Describe() result = %+v, want non-error", result)
			}
			if output.Count != 1 || output.Actions[0].ID != want {
				t.Fatalf("Describe() output = %+v, want %s", output, want)
			}
		})
	}
}

// TestDescribe_IncludesDisambiguationUsage verifies high-confusion actions carry
// usage notes that distinguish adjacent GitLab APIs.
func TestDescribe_IncludesDisambiguationUsage(t *testing.T) {
	registry := realCatalogRegistry(t)

	tests := map[string]string{
		"admin.settings_get":            "current instance/application settings",
		"job.download_single_artifact":  "one artifact file path",
		"package.list":                  "package registry packages",
		"runner.remove":                 "numeric runner_id",
		"repository.compare":            "params.from and params.to",
		"analyze.release_notes":         "after requested release/compare",
		"package.registry_list_project": "container registry image repositories",
	}

	for actionID, wantSubstring := range tests {
		t.Run(actionID, func(t *testing.T) {
			result, output, err := registry.Describe(t.Context(), nil, DescribeInput{Action: actionID})
			if err != nil {
				t.Fatalf("Describe() error = %v", err)
			}
			if result == nil || result.IsError {
				t.Fatalf("Describe() result = %+v, want non-error", result)
			}
			description := actionDescriptionByID(t, output, actionID)
			if !strings.Contains(description.Usage, wantSubstring) {
				t.Fatalf("usage = %q, want substring %q", description.Usage, wantSubstring)
			}
		})
	}
}

// TestDescribe_JobSingleArtifactRequiresArtifactPath verifies the dynamic
// schema exposes all values needed to download one artifact file.
func TestDescribe_JobSingleArtifactRequiresArtifactPath(t *testing.T) {
	registry := realCatalogRegistry(t)

	result, output, err := registry.Describe(t.Context(), nil, DescribeInput{Action: "job.download_single_artifact"})
	if err != nil {
		t.Fatalf("Describe() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Describe() result = %+v, want non-error", result)
	}
	description := actionDescriptionByID(t, output, "job.download_single_artifact")
	for _, required := range []string{"artifact_path", "job_id", "project_id"} {
		if !slices.Contains(description.RequiredParams, required) {
			t.Fatalf("required params = %v, want %s", description.RequiredParams, required)
		}
	}
	if params, ok := description.Example.Arguments["params"].(map[string]any); !ok || params["artifact_path"] == nil {
		t.Fatalf("example arguments = %#v, want artifact_path in params", description.Example.Arguments)
	}
}

// TestExecute_NormalizesCommonParameterAliases verifies that Execute rewrites
// common parameter aliases before dispatching to the canonical handler.
func TestExecute_NormalizesCommonParameterAliases(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, output, err := registry.Execute(t.Context(), nil, ExecuteInput{
		Action: "project.schedule_storage_move",
		Params: map[string]any{"project_id": 123, "shard": "default"},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Execute() result = %+v, want non-error", result)
	}
	data, ok := output.(map[string]any)
	if !ok {
		t.Fatalf("Execute() output type = %T, want map[string]any", output)
	}
	if data["destination_storage_name"] != "default" {
		t.Fatalf("destination_storage_name = %v, want default", data["destination_storage_name"])
	}
}

// TestExecute_DispatchesReadOnlyAction verifies that Execute forwards read-only
// action parameters to the registered route handler and returns its output.
func TestExecute_DispatchesReadOnlyAction(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, output, err := registry.Execute(t.Context(), nil, ExecuteInput{Action: "project.list", Params: map[string]any{"owned": true}})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Execute() result = %+v, want non-error", result)
	}
	data, ok := output.(map[string]any)
	if !ok {
		t.Fatalf("Execute() output type = %T, want map[string]any", output)
	}
	if data["owned"] != true {
		t.Fatalf("owned = %v, want true", data["owned"])
	}
}

// TestExecute_UsesCatalogFormatter verifies that dynamic execution preserves
// the formatter attached to the backing catalog group.
func TestExecute_UsesCatalogFormatter(t *testing.T) {
	catalog := actionregistry.NewCatalog()
	group := actionregistry.NewGroup(actionregistry.GroupOptions{
		ToolName: "gitlab_custom",
		FormatResult: func(any) *mcp.CallToolResult {
			return toolutil.ToolResultAnnotated("custom formatted result", toolutil.ContentDetail)
		},
	})
	group.SetAction(actionregistry.Action{
		Name: "get",
		Route: toolutil.Route(func(_ context.Context, _ map[string]any) (any, error) {
			return map[string]any{"ok": true}, nil
		}),
	})
	if err := catalog.AddGroup(group); err != nil {
		t.Fatalf("AddGroup() error = %v", err)
	}
	registry := NewRegistryFromCatalog(catalog)

	result, output, err := registry.Execute(t.Context(), nil, ExecuteInput{Action: "custom.get", Params: map[string]any{}})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Execute() result = %+v, want non-error", result)
	}
	if text := textContent(result); text != "custom formatted result" {
		t.Fatalf("Execute() text = %q, want custom formatter output", text)
	}
	if data, ok := output.(map[string]any); !ok || data["ok"] != true {
		t.Fatalf("Execute() output = %#v, want route output", output)
	}
}

// TestExecute_CanonicalizesAlias verifies that Execute resolves a compatibility
// alias before invoking the canonical action route.
func TestExecute_CanonicalizesAlias(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, output, err := registry.Execute(t.Context(), nil, ExecuteInput{Action: "repository_file.get", Params: map[string]any{"project_id": 123, "file_path": "README.md", "ref": "main"}})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Execute() result = %+v, want non-error", result)
	}
	data, ok := output.(map[string]any)
	if !ok {
		t.Fatalf("Execute() output type = %T, want map[string]any", output)
	}
	if data["action"] != "repository.file_get" {
		t.Fatalf("action = %v, want repository.file_get", data["action"])
	}
}

// TestExecute_NormalizesActionScopedParameterAliases verifies dynamic execute
// accepts ambiguous model aliases only for actions where the schema is clear.
func TestExecute_NormalizesActionScopedParameterAliases(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	tests := []struct {
		name   string
		input  ExecuteInput
		assert func(t *testing.T, output any)
	}{
		{
			name:  "job status to scope",
			input: ExecuteInput{Action: "job.list", Params: map[string]any{"project_id": 123, "pipeline_id": 456, "status": "failed"}},
			assert: func(t *testing.T, output any) {
				t.Helper()
				data := output.(map[string]any)
				if data["scope"] != "failed" {
					t.Fatalf("output = %#v, want scope failed", output)
				}
			},
		},
		{
			name:  "repository branch to ref",
			input: ExecuteInput{Action: "repository.file_get", Params: map[string]any{"project_id": 123, "file_path": "README.md", "branch": "main"}},
			assert: func(t *testing.T, output any) {
				t.Helper()
				data := output.(map[string]any)
				if data["ref"] != "main" {
					t.Fatalf("output = %#v, want ref main", output)
				}
			},
		},
		{
			name:  "project member role to numeric access level",
			input: ExecuteInput{Action: "project.member_add", Params: map[string]any{"project_id": 123, "user_id": 5, "access_level": "Reporter"}},
			assert: func(t *testing.T, output any) {
				t.Helper()
				data := output.(map[string]any)
				if data["access_level"] != 20 {
					t.Fatalf("output = %#v, want access_level 20", output)
				}
			},
		},
		{
			name:  "project member numeric string access level",
			input: ExecuteInput{Action: "project.member_edit", Params: map[string]any{"project_id": 123, "user_id": 5, "access_level": "30"}},
			assert: func(t *testing.T, output any) {
				t.Helper()
				data := output.(map[string]any)
				if data["access_level"] != 30 {
					t.Fatalf("output = %#v, want access_level 30", output)
				}
			},
		},
		{
			name:  "issue link aliases same project target",
			input: ExecuteInput{Action: "issue.link_create", Params: map[string]any{"project_id": 123, "issue_iid": 1, "linked_issue_iid": 2}},
			assert: func(t *testing.T, output any) {
				t.Helper()
				data := output.(map[string]any)
				if data["target_issue_iid"] != 2 || data["target_project_id"] != 123 {
					t.Fatalf("output = %#v, want target_issue_iid 2 and target_project_id 123", output)
				}
			},
		},
		{
			name:  "issue update closed state event",
			input: ExecuteInput{Action: "issue.update", Params: map[string]any{"project_id": 123, "issue_iid": 1, "state_event": "closed"}},
			assert: func(t *testing.T, output any) {
				t.Helper()
				data := output.(map[string]any)
				if data["state_event"] != "close" {
					t.Fatalf("output = %#v, want state_event close", output)
				}
			},
		},
		{
			name: "branch protect role access levels",
			input: ExecuteInput{Action: "branch.protect", Params: map[string]any{
				"project_id":         123,
				"branch_name":        "main",
				"push_access_level":  "maintainer",
				"merge_access_level": "maintainer",
				"allow_force_push":   false,
			}},
			assert: func(t *testing.T, output any) {
				t.Helper()
				data := output.(map[string]any)
				if data["push_access_level"] != 40 || data["merge_access_level"] != 40 {
					t.Fatalf("output = %#v, want access levels 40", output)
				}
			},
		},
		{
			name:  "group label update name alias",
			input: ExecuteInput{Action: "group.group_label_update", Params: map[string]any{"group_id": "my-org", "label_id": 31, "name": "next-label"}},
			assert: func(t *testing.T, output any) {
				t.Helper()
				data := output.(map[string]any)
				if data["new_name"] != "next-label" {
					t.Fatalf("output = %#v, want new_name next-label", output)
				}
				if _, ok := data["name"]; ok {
					t.Fatalf("output = %#v, want name alias removed", output)
				}
			},
		},
		{
			name:  "runner paused string to bool",
			input: ExecuteInput{Action: "runner.update", Params: map[string]any{"runner_id": 99, "paused": "true"}},
			assert: func(t *testing.T, output any) {
				t.Helper()
				data := output.(map[string]any)
				if data["paused"] != true {
					t.Fatalf("output = %#v, want paused true", output)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, output, err := registry.Execute(t.Context(), nil, tt.input)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if result == nil || result.IsError {
				t.Fatalf("Execute() result = %+v, want non-error", result)
			}
			tt.assert(t, output)
		})
	}
}

// TestExecute_UnknownActionSuggestsCanonicalIDs verifies that unknown actions
// return an MCP tool error with nearby canonical ID suggestions.
func TestExecute_UnknownActionSuggestsCanonicalIDs(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, output, err := registry.Execute(t.Context(), nil, ExecuteInput{Action: "project.destroy"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("Execute() result = %+v, want tool error", result)
	}
	if output != nil {
		t.Fatalf("Execute() output = %+v, want nil", output)
	}
	if !strings.Contains(textContent(result), "`project.delete`") {
		t.Fatalf("Execute() error text = %q, want project.delete suggestion", textContent(result))
	}
}

// TestExecute_RejectsAmbiguousAlias verifies that Execute refuses aliases that
// map to multiple canonical actions and reports the possible targets.
func TestExecute_RejectsAmbiguousAlias(t *testing.T) {
	registry := newRegistry(testRoutes(t), []actionAlias{
		{Alias: "danger.delete", Canonical: "project.delete"},
		{Alias: "danger.delete", Canonical: "package.delete"},
	})

	result, output, err := registry.Execute(t.Context(), nil, ExecuteInput{Action: "danger.delete", Params: map[string]any{"project_id": 123}, Confirm: true})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("Execute() result = %+v, want tool error", result)
	}
	if output != nil {
		t.Fatalf("Execute() output = %+v, want nil", output)
	}
	text := textContent(result)
	if !strings.Contains(text, "ambiguous") || !strings.Contains(text, "`project.delete`") || !strings.Contains(text, "`package.delete`") {
		t.Fatalf("Execute() error text = %q, want ambiguous canonical suggestions", text)
	}
}

// TestDescribe_RejectsAmbiguousAlias verifies that Describe reports ambiguous
// aliases instead of choosing one canonical action arbitrarily.
func TestDescribe_RejectsAmbiguousAlias(t *testing.T) {
	registry := newRegistry(testRoutes(t), []actionAlias{
		{Alias: "danger.delete", Canonical: "project.delete"},
		{Alias: "danger.delete", Canonical: "package.delete"},
	})

	result, output, err := registry.Describe(t.Context(), nil, DescribeInput{Action: "danger.delete"})
	if err != nil {
		t.Fatalf("Describe() error = %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("Describe() result = %+v, want tool error", result)
	}
	if output.Count != 0 || len(output.Actions) != 0 {
		t.Fatalf("Describe() output = %+v, want empty output", output)
	}
}

// TestExecute_DestructiveActionRequiresConfirm verifies that destructive actions
// are blocked until the caller explicitly sets confirm=true.
func TestExecute_DestructiveActionRequiresConfirm(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, output, err := registry.Execute(t.Context(), nil, ExecuteInput{Action: "project.delete", Params: map[string]any{"project_id": 123}})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("Execute() result = %+v, want tool error", result)
	}
	if output != nil {
		t.Fatalf("Execute() output = %+v, want nil", output)
	}
	if !strings.Contains(textContent(result), "confirm=true") {
		t.Fatalf("Execute() error text = %q, want confirm=true hint", textContent(result))
	}
}

// TestExecute_DestructiveActionExecutesWithConfirm verifies that destructive
// actions dispatch normally once the caller provides explicit confirmation.
func TestExecute_DestructiveActionExecutesWithConfirm(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, output, err := registry.Execute(t.Context(), nil, ExecuteInput{Action: "project.delete", Params: map[string]any{"project_id": 123}, Confirm: true})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Execute() result = %+v, want non-error", result)
	}
	data, ok := output.(map[string]any)
	if !ok {
		t.Fatalf("Execute() output type = %T, want map[string]any", output)
	}
	if data["confirm"] != true {
		t.Fatalf("confirm = %v, want true", data["confirm"])
	}
}

// TestRegisterCatalogTools_ExposesThreeDynamicTools verifies that the full dynamic
// surface exposes search, describe, and execute through an MCP session.
func TestRegisterCatalogTools_ExposesThreeDynamicTools(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "dynamic-test", Version: "0"}, nil)
	RegisterCatalogTools(server, actionregistry.FromActionMaps(testRoutes(t)))

	st, ct := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "dynamic-client", Version: "0"}, nil)
	session, err := client.Connect(t.Context(), ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	tools, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools.Tools) != 3 {
		t.Fatalf("tool count = %d, want 3", len(tools.Tools))
	}
	executeSchema := listedToolInputSchema(t, tools.Tools, "gitlab_execute_tool")
	if !slices.Contains(schemaRequired(executeSchema), "params") {
		t.Fatalf("gitlab_execute_tool required = %v, want params", schemaRequired(executeSchema))
	}
	assertSchemaHasProperties(t, executeSchema, "action", "params", "confirm")
}

// TestSearch_PartialMatchLongQuery verifies that incidental query terms do not
// suppress otherwise relevant merge request matches.
func TestSearch_PartialMatchLongQuery(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	// Simulate a realistic LLM query that includes incidental words ("open") that
	// do not map to any tool name but should not suppress relevant results.
	result, output, err := registry.Search(t.Context(), nil, SearchInput{Query: "merge request list open", Limit: 5})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Search() result = %+v, want non-error", result)
	}
	if output.Count == 0 {
		t.Fatal("Search() returned no matches for partial query, want at least one merge_request result")
	}
	found := slices.ContainsFunc(output.Results, func(r SearchResult) bool {
		return strings.HasPrefix(r.ID, "merge_request.")
	})
	if !found {
		t.Fatalf("Search() results = %+v, want at least one merge_request.* result", output.Results)
	}
}

// TestSearch_NaturalLLMQueriesReturnActions verifies natural-language queries
// observed from LLMs still return the intended dynamic actions.
func TestSearch_NaturalLLMQueriesReturnActions(t *testing.T) {
	routes, err := AddStandaloneRoutes(testRoutes(t), nil, StandaloneOptions{})
	if err != nil {
		t.Fatalf("AddStandaloneRoutes() error = %v", err)
	}
	registry := NewRegistry(routes)

	tests := []struct {
		name  string
		query string
		want  string
	}{
		{name: "discover project from remote url", query: "discover project from remote url", want: "discover_project.resolve"},
		{name: "merge request list open authored by me project", query: "merge request list open authored by me project", want: "merge_request.list"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, output, searchErr := registry.Search(t.Context(), nil, SearchInput{Query: tt.query, Limit: 5})
			if searchErr != nil {
				t.Fatalf("Search() error = %v", searchErr)
			}
			if result == nil || result.IsError {
				t.Fatalf("Search() result = %+v, want non-error", result)
			}
			if !slices.ContainsFunc(output.Results, func(r SearchResult) bool { return r.ID == tt.want }) {
				t.Fatalf("Search(%q) results = %+v, want %s", tt.query, output.Results, tt.want)
			}
		})
	}
}

// TestSearch_MultiIntentLongQuery_ReturnsSegmentMatches verifies that a long
// query containing multiple intents is segmented into actionable matches.
func TestSearch_MultiIntentLongQuery_ReturnsSegmentMatches(t *testing.T) {
	routes, err := AddStandaloneRoutes(testRoutes(t), nil, StandaloneOptions{})
	if err != nil {
		t.Fatalf("AddStandaloneRoutes() error = %v", err)
	}
	registry := NewRegistry(routes)

	result, output, err := registry.Search(t.Context(), nil, SearchInput{
		Query: "discover project from remote url merge request list current user open authored",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Search() result = %+v, want non-error", result)
	}
	for _, want := range []string{"discover_project.resolve", "merge_request.list"} {
		if !slices.ContainsFunc(output.Results, func(r SearchResult) bool { return r.ID == want }) {
			t.Fatalf("Search() results = %+v, want %s", output.Results, want)
		}
	}
}

// TestSearch_MultiIntentLongQueryOnMetaCatalog_ReturnsSegmentMatches verifies
// the observed long dynamic query against the real captured meta catalog.
//
// The full catalog already has global matches for the merge-request terms, so
// this test protects the segment merge path that keeps the standalone project
// discovery action in the first page of results.
func TestSearch_MultiIntentLongQueryOnMetaCatalog_ReturnsSegmentMatches(t *testing.T) {
	registry := realCatalogRegistry(t)

	result, output, err := registry.Search(t.Context(), nil, SearchInput{
		Query: "discover project from remote url merge request list current user open authored",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Search() result = %+v, want non-error", result)
	}
	assertSearchResultsContain(t, output.Results, "discover_project.resolve", "merge_request.list")
}

// TestSearch_QueryShapeMatrix_ReturnsExpectedActions verifies short, long,
// typo-heavy, alias-based, and mixed queries against expected action IDs.
func TestSearch_QueryShapeMatrix_ReturnsExpectedActions(t *testing.T) {
	routes, err := AddStandaloneRoutes(testRoutes(t), nil, StandaloneOptions{})
	if err != nil {
		t.Fatalf("AddStandaloneRoutes() error = %v", err)
	}
	registry := NewRegistry(routes)

	tests := []struct {
		name  string
		query string
		limit int
		want  []string
	}{
		{name: "short canonical action", query: "project list", want: []string{"project.list"}},
		{name: "short synonym intent", query: "project info", want: []string{"project.get"}},
		{name: "short alias intent", query: "deploy key", want: []string{"access.deploy_key_add"}},
		{name: "typo phrase", query: "merje requesy list", want: []string{"merge_request.list"}},
		{name: "long polite metadata phrase", query: "please find project metadata details using id", want: []string{"project.get"}},
		{name: "long repository content phrase", query: "download repository file content from project ref", want: []string{"repository.file_get"}},
		{name: "observed authored current user phrase", query: "current user open authored merge request list", want: []string{"merge_request.list"}},
		{name: "standalone discovery without verb", query: "project remote url lookup", want: []string{"discover_project.resolve"}},
		{name: "pipeline jobs alias", query: "pipeline jobs list", want: []string{"job.list"}},
		{name: "ci secret create", query: "create ci secret variable", want: []string{"ci_variable.create"}},
		{name: "package remove intent", query: "remove package", want: []string{"package.delete"}},
		{name: "release notes alias", query: "release generate notes", want: []string{"analyze.release_notes"}},
		{name: "project status checks alias", query: "project status checks list", want: []string{"external_status_check.list_project"}},
		{name: "group audit events alias", query: "group audit events", want: []string{"audit_event.list_group"}},
		{name: "mixed webhook and repository", query: "webhook create repository file read", limit: 10, want: []string{"project.hook_add", "repository.file_get"}},
		{name: "mixed deploy key and package", query: "deploy key create package delete", limit: 10, want: []string{"access.deploy_key_add", "package.delete"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, output, searchErr := registry.Search(t.Context(), nil, SearchInput{Query: tt.query, Limit: tt.limit})
			if searchErr != nil {
				t.Fatalf("Search() error = %v", searchErr)
			}
			if result == nil || result.IsError {
				t.Fatalf("Search() result = %+v, want non-error", result)
			}
			assertSearchResultsContain(t, output.Results, tt.want...)
		})
	}
}

// TestSearch_ProviderConfusionQueries_ReturnExpectedActions locks in the
// production catalog ranking for phrases that confused evaluated models.
func TestSearch_ProviderConfusionQueries_ReturnExpectedActions(t *testing.T) {
	registry := realCatalogRegistry(t)

	tests := []struct {
		name  string
		query string
		limit int
		want  []string
	}{
		{name: "single artifact by numeric job", query: "download coverage/report.xml single artifact file from numeric job id", want: []string{"job.download_single_artifact"}},
		{name: "current instance settings", query: "read current instance settings before creating broadcast message", want: []string{"admin.settings_get"}},
		{name: "release cleanup first steps", query: "verify tag release asset links before deleting release and tag", limit: 8, want: []string{"tag.get", "release.get", "release.link_list"}},
		{name: "compare refs before release notes", query: "list releases compare refs from v1.0.0 to main then generate release notes", limit: 8, want: []string{"release.list", "repository.compare", "analyze.release_notes"}},
		{name: "generic package list", query: "list package registry packages", want: []string{"package.list"}},
		{name: "runner removal by id", query: "remove runner by numeric runner_id", want: []string{"runner.remove"}},
		{name: "issue time tracking sequence", query: "issue time tracking set estimate add spent time reset spent time reset estimate", limit: 8, want: []string{"issue.time_estimate_set", "issue.spent_time_add", "issue.spent_time_reset", "issue.time_estimate_reset"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, output, err := registry.Search(t.Context(), nil, SearchInput{Query: tt.query, Limit: tt.limit})
			if err != nil {
				t.Fatalf("Search() error = %v", err)
			}
			if result == nil || result.IsError {
				t.Fatalf("Search() result = %+v, want non-error", result)
			}
			assertSearchResultsContain(t, output.Results, tt.want...)
		})
	}
}

// TestSearch_MixedQueriesWithTightLimit_ReturnExactActionSet verifies that mixed
// intent queries return the expected action set even when the limit is tight.
func TestSearch_MixedQueriesWithTightLimit_ReturnExactActionSet(t *testing.T) {
	routes, err := AddStandaloneRoutes(testRoutes(t), nil, StandaloneOptions{})
	if err != nil {
		t.Fatalf("AddStandaloneRoutes() error = %v", err)
	}
	registry := NewRegistry(routes)

	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{
			name:  "discover and merge request lookup",
			query: "discover project from remote url merge request list current user open authored",
			want:  []string{"merge_request.list", "discover_project.resolve"},
		},
		{
			name:  "webhook creation and repository read",
			query: "webhook create repository file read",
			want:  []string{"repository.file_get", "project.hook_add"},
		},
		{
			name:  "release link creation and package deletion",
			query: "release link create package remove",
			want:  []string{"release.link_create", "package.delete"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, output, searchErr := registry.Search(t.Context(), nil, SearchInput{Query: tt.query, Limit: len(tt.want)})
			if searchErr != nil {
				t.Fatalf("Search() error = %v", searchErr)
			}
			if result == nil || result.IsError {
				t.Fatalf("Search() result = %+v, want non-error", result)
			}
			assertSearchResultIDsEqual(t, output.Results, tt.want...)
		})
	}
}

// TestSearch_TypoQueryReturnsRelevantActions verifies that the fuzzy fallback
// recovers relevant merge request actions from typo-heavy query terms.
func TestSearch_TypoQueryReturnsRelevantActions(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	result, output, err := registry.Search(t.Context(), nil, SearchInput{Query: "merje requesy list", Limit: 5})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Search() result = %+v, want non-error", result)
	}
	if output.Count == 0 {
		t.Fatal("Search() returned no matches for typo query, want at least one merge_request result")
	}
	if !slices.ContainsFunc(output.Results, func(r SearchResult) bool {
		return strings.HasPrefix(r.ID, "merge_request.")
	}) {
		t.Fatalf("Search() results = %+v, want at least one merge_request.* result", output.Results)
	}
}

// TestSearch_TypoQueryReturnsResultsOnMetaCatalog verifies that fuzzy matching
// works against the real captured meta-tool catalog, not only test fixtures.
func TestSearch_TypoQueryReturnsResultsOnMetaCatalog(t *testing.T) {
	registry := realCatalogRegistry(t)
	result, output, err := registry.Search(t.Context(), nil, SearchInput{Query: "merje requesy", Limit: 5})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("Search() result = %+v, want non-error", result)
	}
	if output.Count == 0 {
		t.Fatal("Search() returned no matches for typo query on meta catalog")
	}
}

func actionDescriptionByID(t *testing.T, output DescribeOutput, id string) ActionDescription {
	t.Helper()
	for _, action := range output.Actions {
		if action.ID == id {
			return action
		}
	}
	t.Fatalf("DescribeOutput missing action %q: %+v", id, output.Actions)
	return ActionDescription{}
}

func realCatalogRegistry(t *testing.T) *Registry {
	t.Helper()
	catalog, err := tools.BuildActionCatalog(nil, tools.ActionCatalogOptions{IncludeMCP: true})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}
	catalog, err = AddStandaloneCatalog(catalog, nil, StandaloneOptions{})
	if err != nil {
		t.Fatalf("AddStandaloneCatalog() error = %v", err)
	}
	return NewRegistryFromCatalog(catalog)
}

func assertSearchResultsContain(t *testing.T, results []SearchResult, want ...string) {
	t.Helper()
	for _, actionID := range want {
		if slices.ContainsFunc(results, func(result SearchResult) bool { return result.ID == actionID }) {
			continue
		}
		t.Fatalf("Search() results = %+v, want %s", results, actionID)
	}
}

func assertSearchResultIDsEqual(t *testing.T, results []SearchResult, want ...string) {
	t.Helper()
	if len(results) != len(want) {
		t.Fatalf("Search() results = %+v, want exactly %v", results, want)
	}
	gotIDs := make([]string, 0, len(results))
	for _, result := range results {
		gotIDs = append(gotIDs, result.ID)
	}
	slices.Sort(gotIDs)
	wantIDs := append([]string(nil), want...)
	slices.Sort(wantIDs)
	if !slices.Equal(gotIDs, wantIDs) {
		t.Fatalf("Search() result IDs = %v, want exactly %v", gotIDs, wantIDs)
	}
}

func assertSchemaHasProperties(t *testing.T, schema map[string]any, names ...string) {
	t.Helper()
	properties := schemaProperties(schema)
	for _, name := range names {
		if _, ok := properties[name]; !ok {
			t.Fatalf("schema properties = %v, want %q", sortedPropertyNames(properties), name)
		}
	}
}

func schemaProperties(schema map[string]any) map[string]any {
	properties, _ := schema["properties"].(map[string]any)
	if properties == nil {
		return map[string]any{}
	}
	return properties
}

func schemaRequired(schema map[string]any) []string {
	var required []string
	switch values := schema["required"].(type) {
	case []any:
		for _, value := range values {
			if name, ok := value.(string); ok {
				required = append(required, name)
			}
		}
	case []string:
		required = append(required, values...)
	}
	slices.Sort(required)
	return required
}

func listedToolInputSchema(t *testing.T, tools []*mcp.Tool, name string) map[string]any {
	t.Helper()
	for _, tool := range tools {
		if tool.Name != name {
			continue
		}
		data, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatalf("marshal %s input schema: %v", name, err)
		}
		var schema map[string]any
		if unmarshalErr := json.Unmarshal(data, &schema); unmarshalErr != nil {
			t.Fatalf("unmarshal %s input schema: %v", name, unmarshalErr)
		}
		return schema
	}
	t.Fatalf("tool %s not listed", name)
	return nil
}

func sortedPropertyNames(properties map[string]any) []string {
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func testRoutes(t *testing.T) map[string]toolutil.ActionMap {
	t.Helper()
	return map[string]toolutil.ActionMap{
		"gitlab_project": {
			"get": {
				Handler: func(_ context.Context, params map[string]any) (any, error) {
					return map[string]any{"project_id": params["project_id"]}, nil
				},
				InputSchema: map[string]any{
					"type":     "object",
					"required": []any{"project_id"},
					"properties": map[string]any{
						"project_id": map[string]any{"type": "integer"},
					},
				},
				OutputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"project_id": map[string]any{"type": "integer"},
					},
				},
			},
			"hook_list": {
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"hooks": true}, nil
				},
			},
			"hook_add": {
				Handler: func(_ context.Context, params map[string]any) (any, error) {
					return map[string]any{"url": params["url"]}, nil
				},
				InputSchema: map[string]any{
					"type":     "object",
					"required": []any{"project_id", "url"},
					"properties": map[string]any{
						"project_id": map[string]any{"type": "integer"},
						"url":        map[string]any{"type": "string"},
					},
				},
			},
			"member_edit": {
				Handler: func(_ context.Context, params map[string]any) (any, error) {
					return map[string]any{"member": "edited", "access_level": params["access_level"]}, nil
				},
				InputSchema: map[string]any{
					"type":     "object",
					"required": []any{"project_id", "user_id", "access_level"},
					"properties": map[string]any{
						"project_id":   map[string]any{"type": "integer"},
						"user_id":      map[string]any{"type": "integer"},
						"access_level": map[string]any{"type": "integer"},
					},
				},
			},
			"member_add": {
				Handler: func(_ context.Context, params map[string]any) (any, error) {
					return map[string]any{"member": "added", "access_level": params["access_level"]}, nil
				},
				InputSchema: map[string]any{
					"type":     "object",
					"required": []any{"project_id", "user_id", "access_level"},
					"properties": map[string]any{
						"project_id":   map[string]any{"type": "integer"},
						"user_id":      map[string]any{"type": "integer"},
						"access_level": map[string]any{"type": "integer"},
					},
				},
			},
			"member_delete": {
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"member": "deleted"}, nil
				},
			},
			"delete": {
				Handler: func(_ context.Context, params map[string]any) (any, error) {
					return map[string]any{"deleted": true, "confirm": params["confirm"]}, nil
				},
				Destructive: true,
				InputSchema: map[string]any{
					"type":     "object",
					"required": []any{"project_id"},
					"properties": map[string]any{
						"project_id": map[string]any{"type": "integer"},
					},
				},
			},
			"list": {
				Handler: func(_ context.Context, params map[string]any) (any, error) {
					return map[string]any{"owned": params["owned"]}, nil
				},
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"owned": map[string]any{"type": "boolean"},
					},
				},
			},
		},
		"gitlab_merge_request": {
			"list": {
				Handler: func(_ context.Context, params map[string]any) (any, error) {
					return map[string]any{"state": params["state"], "author_username": params["author_username"]}, nil
				},
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"project_id":      map[string]any{"type": "integer"},
						"state":           map[string]any{"type": "string"},
						"author_username": map[string]any{"type": "string"},
					},
				},
			},
			"approve": {
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"approved": true}, nil
				},
			},
			"merge": {
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"merged": true}, nil
				},
			},
			"time_estimate_set": {
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"time": "set"}, nil
				},
			},
			"spent_time_add": {
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"spent": "added"}, nil
				},
			},
		},
		"gitlab_issue": {
			"note_list": {
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"notes": true}, nil
				},
			},
			"link_create": {
				Handler: func(_ context.Context, params map[string]any) (any, error) {
					return map[string]any{"target_issue_iid": params["target_issue_iid"], "target_project_id": params["target_project_id"]}, nil
				},
				InputSchema: map[string]any{
					"type":     "object",
					"required": []any{"project_id", "issue_iid", "target_project_id", "target_issue_iid"},
					"properties": map[string]any{
						"project_id":        map[string]any{"type": "integer"},
						"issue_iid":         map[string]any{"type": "integer"},
						"target_project_id": map[string]any{"type": "integer"},
						"target_issue_iid":  map[string]any{"type": "integer"},
					},
				},
			},
			"update": {
				Handler: func(_ context.Context, params map[string]any) (any, error) {
					return map[string]any{"state_event": params["state_event"]}, nil
				},
				InputSchema: map[string]any{
					"type":     "object",
					"required": []any{"project_id", "issue_iid", "state_event"},
					"properties": map[string]any{
						"project_id":  map[string]any{"type": "integer"},
						"issue_iid":   map[string]any{"type": "integer"},
						"state_event": map[string]any{"type": "string"},
					},
				},
			},
		},
		"gitlab_ci_variable": {
			"create": {
				Handler: func(_ context.Context, params map[string]any) (any, error) {
					return map[string]any{"key": params["key"]}, nil
				},
				InputSchema: map[string]any{
					"type":     "object",
					"required": []any{"project_id", "key", "value"},
					"properties": map[string]any{
						"project_id": map[string]any{"type": "integer"},
						"key":        map[string]any{"type": "string"},
						"value":      map[string]any{"type": "string"},
					},
				},
			},
			"group_create": {
				Handler: func(_ context.Context, params map[string]any) (any, error) {
					return map[string]any{"key": params["key"]}, nil
				},
			},
		},
		"gitlab_branch": {
			"get_protected": {
				Handler: func(_ context.Context, params map[string]any) (any, error) {
					return map[string]any{"branch_name": params["branch_name"]}, nil
				},
				InputSchema: map[string]any{
					"type":     "object",
					"required": []any{"project_id", "branch_name"},
					"properties": map[string]any{
						"project_id":  map[string]any{"type": "integer"},
						"branch_name": map[string]any{"type": "string"},
					},
				},
			},
			"protect": {
				Handler: func(_ context.Context, params map[string]any) (any, error) {
					return map[string]any{
						"push_access_level":  params["push_access_level"],
						"merge_access_level": params["merge_access_level"],
					}, nil
				},
				InputSchema: map[string]any{
					"type":     "object",
					"required": []any{"project_id", "branch_name"},
					"properties": map[string]any{
						"project_id":         map[string]any{"type": "integer"},
						"branch_name":        map[string]any{"type": "string"},
						"push_access_level":  map[string]any{"type": "integer"},
						"merge_access_level": map[string]any{"type": "integer"},
						"allow_force_push":   map[string]any{"type": "boolean"},
					},
				},
			},
			"update_protected": {
				Handler: func(_ context.Context, params map[string]any) (any, error) {
					return map[string]any{"allow_force_push": params["allow_force_push"]}, nil
				},
				InputSchema: map[string]any{
					"type":     "object",
					"required": []any{"project_id", "branch_name"},
					"properties": map[string]any{
						"project_id":       map[string]any{"type": "integer"},
						"branch_name":      map[string]any{"type": "string"},
						"allow_force_push": map[string]any{"type": "boolean"},
					},
				},
			},
		},
		"gitlab_repository": {
			"file_get": {
				Handler: func(_ context.Context, params map[string]any) (any, error) {
					return map[string]any{"action": "repository.file_get", "file_path": params["file_path"], "ref": params["ref"]}, nil
				},
				InputSchema: map[string]any{
					"type":     "object",
					"required": []any{"project_id", "file_path", "ref"},
					"properties": map[string]any{
						"project_id": map[string]any{"type": "integer"},
						"file_path":  map[string]any{"type": "string"},
						"ref":        map[string]any{"type": "string"},
					},
				},
			},
		},
		"gitlab_access": {
			"deploy_key_add": {
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"deploy_key": "added"}, nil
				},
			},
			"token_project_create": {
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"token": "created"}, nil
				},
			},
			"deploy_token_create_project": {
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"deploy_token": "created"}, nil
				},
			},
		},
		"gitlab_runner": {
			"update": {
				Handler: func(_ context.Context, params map[string]any) (any, error) {
					return map[string]any{"paused": params["paused"]}, nil
				},
				InputSchema: map[string]any{
					"type":     "object",
					"required": []any{"runner_id", "paused"},
					"properties": map[string]any{
						"runner_id": map[string]any{"type": "integer"},
						"paused":    map[string]any{"type": "boolean"},
					},
				},
			},
		},
		"gitlab_group": {
			"group_label_update": {
				Handler: func(_ context.Context, params map[string]any) (any, error) {
					return params, nil
				},
			},
			"ldap_link_delete_for_provider": {
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"deleted": true}, nil
				},
			},
		},
		"gitlab_storage_move": {
			"schedule_project": {
				Handler: func(_ context.Context, params map[string]any) (any, error) {
					return map[string]any{"destination_storage_name": params["destination_storage_name"]}, nil
				},
				InputSchema: map[string]any{
					"type":     "object",
					"required": []any{"project_id"},
					"properties": map[string]any{
						"project_id":               map[string]any{"type": "integer"},
						"destination_storage_name": map[string]any{"type": "string"},
					},
				},
			},
		},
		"gitlab_mr_review": {
			"changes_get": {
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"changes": true}, nil
				},
			},
			"draft_note_publish_all": {
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"published": true}, nil
				},
			},
		},
		"gitlab_external_status_check": {
			"list_project": {
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"checks": true}, nil
				},
			},
		},
		"gitlab_package": {
			"list": {
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"packages": true}, nil
				},
			},
			"file_list": {
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"files": true}, nil
				},
			},
			"delete": {
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"deleted": true}, nil
				},
				Destructive: true,
			},
		},
		"gitlab_audit_event": {
			"list_group": {
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"events": true}, nil
				},
			},
		},
		"gitlab_job": {
			"list": {
				Handler: func(_ context.Context, params map[string]any) (any, error) {
					return map[string]any{"jobs": true, "scope": params["scope"]}, nil
				},
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"project_id":  map[string]any{"type": "integer"},
						"pipeline_id": map[string]any{"type": "integer"},
						"scope":       map[string]any{"type": "string"},
					},
				},
			},
			"token_scope_list_inbound": {
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"allowlist": true}, nil
				},
			},
		},
		"gitlab_release": {
			"list": {
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"releases": true}, nil
				},
			},
			"link_create": {
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"link": "created"}, nil
				},
			},
		},
		"gitlab_analyze": {
			"release_notes": {
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"release_notes": true}, nil
				},
			},
		},
	}
}

func textContent(result *mcp.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}
	text, _ := result.Content[0].(*mcp.TextContent)
	if text == nil {
		return ""
	}
	return text.Text
}
