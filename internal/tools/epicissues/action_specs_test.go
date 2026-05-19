// action_specs_test.go contains integration tests for the epic issue link tool
// closures in ActionSpecs routes with a mock GitLab API.
package epicissues

import (
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_Metadata verifies epic issue action spec metadata.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 4 {
		t.Fatalf("len(ActionSpecs) = %d, want 4", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "epicissues" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
	}
}

// TestActionSpecs_CallRoutes uses table-driven subtests to exercise each canonical route.
func TestActionSpecs_CallRoutes(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"WorkItemWidgetHierarchy": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, gqlChildrenData)
		},
		"workItem(iid": func(w http.ResponseWriter, r *http.Request) {
			vars, _ := testutil.ParseGraphQLVariables(r)
			fp, _ := vars["fullPath"].(string)
			if fp == "child-group/child-project" {
				testutil.RespondGraphQL(w, http.StatusOK, `{"namespace":{"workItem":{"id":"gid://gitlab/WorkItem/42"}}}`)
			} else {
				testutil.RespondGraphQL(w, http.StatusOK, gqlWorkItemGIDData)
			}
		},
		"workItemUpdate(": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, gqlAddChildData)
		},
	})

	client := testutil.NewTestClient(t, handler)
	specs := ActionSpecs(client)
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_epic_issue_list", map[string]any{"full_path": testFullPath, "epic_iid": 1}},
		{"gitlab_epic_issue_assign", map[string]any{"full_path": testFullPath, "epic_iid": 1, "child_project_path": "child-group/child-project", "child_iid": 42}},
		{"gitlab_epic_issue_remove", map[string]any{"full_path": testFullPath, "epic_iid": 1, "child_project_path": "child-group/child-project", "child_iid": 42}},
		{"gitlab_epic_issue_update", map[string]any{"full_path": testFullPath, "epic_iid": 1, "child_id": "gid://gitlab/WorkItem/10", "adjacent_id": "gid://gitlab/WorkItem/20", "relative_position": "BEFORE"}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := specByTool[tt.name]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tt.name)
			}
			result, err := spec.Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil result", tt.name)
			}
		})
	}
}

// TestMarkdownInit_Registry verifies that markdown formatters for ListOutput and AssignOutput are registered in the shared markdown registry.
func TestMarkdownInit_Registry(t *testing.T) {
	out := toolutil.MarkdownForResult(ListOutput{})
	if out == nil {
		t.Fatal("expected non-nil result for ListOutput")
	}
	out2 := toolutil.MarkdownForResult(AssignOutput{})
	if out2 == nil {
		t.Fatal("expected non-nil result for AssignOutput")
	}
}
