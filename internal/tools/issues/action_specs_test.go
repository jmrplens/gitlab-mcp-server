// action_specs_test.go contains canonical-route tests for issue actions.
package issues

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_CallRoutes exercises issue actions through their canonical routes.
func TestActionSpecs_CallRoutes(t *testing.T) {
	byTool := issueSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))))
	pid := testProjectID

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_issue_create", map[string]any{"project_id": pid, "title": "Test"}},
		{"gitlab_issue_get", map[string]any{"project_id": pid, "issue_iid": 10}},
		{"gitlab_issue_get_by_id", map[string]any{"issue_id": 10}},
		{"gitlab_issue_list", map[string]any{"project_id": pid}},
		{"gitlab_issue_list_all", map[string]any{}},
		{"gitlab_issue_list_group", map[string]any{"group_id": "99"}},
		{"gitlab_issue_update", map[string]any{"project_id": pid, "issue_iid": 10, "title": "Updated"}},
		{"gitlab_issue_delete", map[string]any{"project_id": pid, "issue_iid": 10}},
		{"gitlab_issue_reorder", map[string]any{"project_id": pid, "issue_iid": 10, "move_after_id": 5}},
		{"gitlab_issue_move", map[string]any{"project_id": pid, "issue_iid": 10, "to_project_id": 99}},
		{"gitlab_issue_subscribe", map[string]any{"project_id": pid, "issue_iid": 10}},
		{"gitlab_issue_unsubscribe", map[string]any{"project_id": pid, "issue_iid": 10}},
		{"gitlab_issue_create_todo", map[string]any{"project_id": pid, "issue_iid": 10}},
		{"gitlab_issue_time_estimate_set", map[string]any{"project_id": pid, "issue_iid": 10, "duration": "3h"}},
		{"gitlab_issue_time_estimate_reset", map[string]any{"project_id": pid, "issue_iid": 10}},
		{"gitlab_issue_spent_time_add", map[string]any{"project_id": pid, "issue_iid": 10, "duration": "1h"}},
		{"gitlab_issue_spent_time_reset", map[string]any{"project_id": pid, "issue_iid": 10}},
		{"gitlab_issue_time_stats_get", map[string]any{"project_id": pid, "issue_iid": 10}},
		{"gitlab_issue_participants", map[string]any{"project_id": pid, "issue_iid": 10}},
		{"gitlab_issue_mrs_closing", map[string]any{"project_id": pid, "issue_iid": 10}},
		{"gitlab_issue_mrs_related", map[string]any{"project_id": pid, "issue_iid": 10}},
	}

	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			result, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.tool)
			}
		})
	}
}

// TestGroupActionSpecs_CallRoutes exercises issue actions exposed through the group meta-tool.
func TestGroupActionSpecs_CallRoutes(t *testing.T) {
	byTool := issueSpecsByTool(t, GroupActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))))

	result, err := byTool["gitlab_issue_list_group"].Route.Handler(t.Context(), map[string]any{"group_id": "99"})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_issue_list_group) error: %v", err)
	}
	if result == nil {
		t.Fatal("Route.Handler(gitlab_issue_list_group) returned nil")
	}
	spec := byTool["gitlab_issue_list_group"]
	if !spec.ReadOnly || !spec.Idempotent {
		t.Fatalf("group issue spec read-only/idempotent = %v/%v, want true/true", spec.ReadOnly, spec.Idempotent)
	}
}

// TestActionSpecs_UpdateStateEventGuidance verifies issue update exposes the
// state transition hints needed by meta and dynamic surfaces.
func TestActionSpecs_UpdateStateEventGuidance(t *testing.T) {
	byTool := issueSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))))
	spec := byTool["gitlab_issue_update"]

	if !strings.Contains(spec.Usage, "state_event") || !strings.Contains(spec.Usage, "issue.close") || !strings.Contains(spec.Usage, "issue.reopen") {
		t.Fatalf("Usage = %q, want state_event and lifecycle alias guidance", spec.Usage)
	}
	guidance, ok := spec.ParameterGuidance["state_event"]
	if !ok {
		t.Fatal("state_event parameter guidance missing")
	}
	if guidance.SemanticRole != "issue_state_transition" || !strings.Contains(guidance.ExampleBinding, "close") {
		t.Fatalf("state_event guidance = %+v, want transition role and close example", guidance)
	}

	properties, ok := spec.Route.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("InputSchema properties missing: %#v", spec.Route.InputSchema)
	}
	stateEventSchema, ok := properties["state_event"].(map[string]any)
	if !ok {
		t.Fatalf("state_event schema missing: %#v", properties["state_event"])
	}
	values, ok := stateEventSchema["enum"].([]any)
	if !ok || len(values) != 2 || values[0] != "close" || values[1] != "reopen" {
		t.Fatalf("state_event enum = %#v, want close/reopen", stateEventSchema["enum"])
	}
}

// TestActionSpecs_PrimaryMetadata verifies richer catalog metadata for the
// primary issue actions surfaced most often in meta and dynamic workflows.
func TestActionSpecs_PrimaryMetadata(t *testing.T) {
	byTool := issueSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))))

	createSpec := byTool["gitlab_issue_create"]
	if !strings.Contains(createSpec.Usage, "Create a new issue in a known project") {
		t.Fatalf("create Usage = %q", createSpec.Usage)
	}
	if !slices.Contains(createSpec.Aliases, "file issue") {
		t.Fatalf("create Aliases = %v, want file issue", createSpec.Aliases)
	}
	if guidance := createSpec.ParameterGuidance["title"]; guidance.SemanticRole != "issue_title" {
		t.Fatalf("create title guidance = %+v, want issue_title", guidance)
	}
	if !strings.Contains(createSpec.IndividualTool.Description, "Returns:") || !strings.Contains(createSpec.IndividualTool.Description, "See also:") {
		t.Fatalf("create description = %q, want Returns/See also sections", createSpec.IndividualTool.Description)
	}

	getSpec := byTool["gitlab_issue_get"]
	if !strings.Contains(getSpec.Usage, "project_id plus issue_iid") {
		t.Fatalf("get Usage = %q", getSpec.Usage)
	}
	if guidance := getSpec.ParameterGuidance["issue_iid"]; guidance.SemanticRole != "issue_iid" {
		t.Fatalf("get issue_iid guidance = %+v, want issue_iid", guidance)
	}
	if !slices.Contains(getSpec.RelatedActions, "issue.notes_list") {
		t.Fatalf("get RelatedActions = %v, want issue.notes_list", getSpec.RelatedActions)
	}

	listSpec := byTool["gitlab_issue_list"]
	if !slices.Contains(listSpec.Aliases, "list project issues") {
		t.Fatalf("list Aliases = %v, want list project issues", listSpec.Aliases)
	}
	if guidance := listSpec.ParameterGuidance["order_by"]; guidance.SemanticRole != "issue_list_sort_field" {
		t.Fatalf("list order_by guidance = %+v, want issue_list_sort_field", guidance)
	}

	allSpec := byTool["gitlab_issue_list_all"]
	if !strings.Contains(allSpec.Usage, "across all accessible projects") {
		t.Fatalf("list_all Usage = %q", allSpec.Usage)
	}

	groupSpecs := issueSpecsByTool(t, GroupActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))))
	groupSpec := groupSpecs["gitlab_issue_list_group"]
	if !slices.Contains(groupSpec.Aliases, "list group issues") {
		t.Fatalf("group list Aliases = %v, want list group issues", groupSpec.Aliases)
	}
	if guidance := groupSpec.ParameterGuidance["group_id"]; guidance.SemanticRole != "scope_group" {
		t.Fatalf("group_id guidance = %+v, want scope_group", guidance)
	}
}

// TestActionSpecs_DeleteOutput verifies issue delete preserves its success message.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	byTool := issueSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))))

	result, err := byTool["gitlab_issue_delete"].Route.Handler(t.Context(), map[string]any{"project_id": testProjectID, "issue_iid": 10})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_issue_delete) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_issue_delete) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted issue #10 from project 42." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestActionSpecs_DeleteErrorPropagates verifies issue delete backend errors propagate.
func TestActionSpecs_DeleteErrorPropagates(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		http.NotFound(w, r)
	}))
	byTool := issueSpecsByTool(t, ActionSpecs(client))

	_, err := byTool["gitlab_issue_delete"].Route.Handler(t.Context(), map[string]any{"project_id": testProjectID, "issue_iid": 10})
	if err == nil {
		t.Fatal("Route.Handler(gitlab_issue_delete) expected error")
	}
}

// TestActionSpecs_GetEmbedsCanonicalResource preserves the rich individual
// result shape that gitlab_issue_get exposed before catalog projection.
func TestActionSpecs_GetEmbedsCanonicalResource(t *testing.T) {
	byTool := issueSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))))

	raw, err := byTool["gitlab_issue_get"].Route.Handler(t.Context(), map[string]any{"project_id": testProjectID, "issue_iid": 10})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_issue_get) error: %v", err)
	}
	out, ok := raw.(getOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_issue_get) returned %T, want getOutput", raw)
	}
	result := toolutil.MarkdownForResult(out)
	if result == nil {
		t.Fatal("MarkdownForResult(getOutput) returned nil")
	}
	var found *mcp.EmbeddedResource
	for _, content := range result.Content {
		if embedded, isEmbedded := content.(*mcp.EmbeddedResource); isEmbedded {
			found = embedded
			break
		}
	}
	if found == nil || found.Resource == nil {
		t.Fatal("expected EmbeddedResource in gitlab_issue_get markdown result")
	}
	if found.Resource.URI != "gitlab://project/42/issue/10" {
		t.Fatalf("embedded URI = %q, want gitlab://project/42/issue/10", found.Resource.URI)
	}
	if found.Resource.MIMEType != "application/json" {
		t.Fatalf("embedded MIME = %q, want application/json", found.Resource.MIMEType)
	}
	if found.Resource.Text == "" {
		t.Fatal("embedded Text payload empty")
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API when confirm is declined")
	}))
	byTool := issueSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_issue_delete"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test issue destructive confirmation.",
		Icons:       toolutil.IconIssue,
	})

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "decline"}, nil
		},
	})
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		session.Close()
		_ = serverSession.Wait()
	})

	result, callErr := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_issue_delete",
		Arguments: map[string]any{"project_id": testProjectID, "issue_iid": 10},
	})
	if callErr != nil {
		t.Fatalf("CallTool error: %v", callErr)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

func issueSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		toolName := spec.IndividualTool.Name
		if toolName == "" {
			t.Fatalf("spec %s missing IndividualTool.Name", spec.Name)
		}
		if _, exists := byTool[toolName]; exists {
			t.Fatalf("duplicate individual tool %q", toolName)
		}
		byTool[toolName] = spec
	}
	return byTool
}
