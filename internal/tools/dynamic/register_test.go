package dynamic

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

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

func TestAddStandaloneRoutes_AddsDynamicActions(t *testing.T) {
	routes := AddStandaloneRoutes(nil, nil, StandaloneOptions{})
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

func TestAddStandaloneRoutes_HonorsReadOnlyAndExclusions(t *testing.T) {
	routes := AddStandaloneRoutes(nil, nil, StandaloneOptions{
		ReadOnly:     true,
		ExcludeTools: []string{"gitlab_discover_project"},
	})
	registry := NewRegistry(routes)

	if _, ok := registry.resolveAction("discover_project.resolve"); ok {
		t.Fatal("discover_project.resolve is present, want excluded")
	}
	if _, ok := registry.resolveAction("interactive.issue_create"); ok {
		t.Fatal("interactive.issue_create is present in read-only mode")
	}
}

func TestDescribe_CanonicalizesStandaloneAlias(t *testing.T) {
	registry := NewRegistry(AddStandaloneRoutes(nil, nil, StandaloneOptions{}))

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
	if found.Example.Tool != "gitlab_execute_tool" || found.Example.Arguments["confirm"] != true {
		t.Fatalf("example = %+v, want execute example with confirm", found.Example)
	}
}

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

func TestRegisterFindExecuteTools_ExposesTwoDynamicTools(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "dynamic-test", Version: "0"}, nil)
	RegisterFindExecuteTools(server, testRoutes(t))

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

func TestDescribe_CanonicalizesObservedModelAliases(t *testing.T) {
	registry := NewRegistry(testRoutes(t))

	tests := map[string]string{
		"project.schedule_storage_move":   "storage_move.schedule_project",
		"merge_request.changes":           "mr_review.changes_get",
		"project.hooks.list":              "project.hook_list",
		"project.status_check_list":       "external_status_check.list_project",
		"deploy_token.create":             "access.deploy_token_create_project",
		"merge_request.set_time_estimate": "merge_request.time_estimate_set",
		"mr_review.draft_notes_publish":   "mr_review.draft_note_publish_all",
		"mr_review.publish":               "mr_review.draft_note_publish_all",
		"package.list_generic":            "package.list",
		"project_member.update":           "project.member_edit",
		"project.member_remove":           "project.member_delete",
		"project_member.remove":           "project.member_delete",
		"webhook.add":                     "project.hook_add",
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

func TestRegisterTools_ExposesThreeDynamicTools(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "dynamic-test", Version: "0"}, nil)
	RegisterTools(server, testRoutes(t))

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
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"member": "edited"}, nil
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
			"approve": {
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"approved": true}, nil
				},
			},
			"time_estimate_set": {
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"time": "set"}, nil
				},
			},
		},
		"gitlab_issue": {
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
		},
		"gitlab_repository": {
			"file_get": {
				Handler: func(_ context.Context, params map[string]any) (any, error) {
					return map[string]any{"action": "repository.file_get", "file_path": params["file_path"]}, nil
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
			"delete": {
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"deleted": true}, nil
				},
				Destructive: true,
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
