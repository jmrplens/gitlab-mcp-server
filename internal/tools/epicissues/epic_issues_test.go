// epic_issues_test.go validates all epic-issue tool handlers via the Work Items
// GraphQL API: List, Assign, Remove, and UpdateOrder. Covers success paths,
// input validation, API errors, mutation errors, context cancellation,
// pagination, empty results, dual GID resolution, and markdown formatters.
package epicissues

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const (
	testFullPath     = "my-group"
	testChildProject = "my-group/my-project"
)

// --- GraphQL response fixtures ---

const gqlChildrenData = `{
  "namespace": {
    "workItem": {
      "id": "gid://gitlab/WorkItem/1",
      "widgets": [{
        "children": {
          "pageInfo": {"hasNextPage": false, "hasPreviousPage": false, "endCursor": "abc", "startCursor": "xyz"},
          "nodes": [{
            "id": "gid://gitlab/WorkItem/10",
            "iid": "10",
            "title": "Fix login bug",
            "state": "OPEN",
            "webUrl": "https://gitlab.example.com/my-group/my-project/-/issues/10",
            "createdAt": "2026-01-15T10:00:00Z",
            "updatedAt": "2026-01-16T10:00:00Z",
            "author": {"username": "alice"},
            "widgets": [{"labels": {"nodes": [{"title": "bug"}, {"title": "critical"}]}}]
          }]
        }
      }]
    }
  }
}`

const gqlChildrenMultiple = `{
  "namespace": {
    "workItem": {
      "id": "gid://gitlab/WorkItem/1",
      "widgets": [{
        "children": {
          "pageInfo": {"hasNextPage": true, "hasPreviousPage": false, "endCursor": "cursor2", "startCursor": "cursor1"},
          "nodes": [
            {
              "id": "gid://gitlab/WorkItem/10",
              "iid": "10",
              "title": "Fix login bug",
              "state": "OPEN",
              "webUrl": "https://gitlab.example.com/my-group/my-project/-/issues/10",
              "createdAt": "2026-01-15T10:00:00Z",
              "updatedAt": "2026-01-16T10:00:00Z",
              "author": {"username": "alice"},
              "widgets": [{"labels": {"nodes": [{"title": "bug"}]}}]
            },
            {
              "id": "gid://gitlab/WorkItem/20",
              "iid": "20",
              "title": "Add feature",
              "state": "CLOSED",
              "webUrl": "https://gitlab.example.com/my-group/my-project/-/issues/20",
              "createdAt": "2026-02-01T12:00:00Z",
              "updatedAt": "2026-02-02T12:00:00Z",
              "author": {"username": "bob"},
              "widgets": [{"labels": {"nodes": []}}]
            }
          ]
        }
      }]
    }
  }
}`

const gqlChildrenEmpty = `{
  "namespace": {
    "workItem": {
      "id": "gid://gitlab/WorkItem/1",
      "widgets": [{
        "children": {
          "pageInfo": {"hasNextPage": false, "hasPreviousPage": false, "endCursor": "", "startCursor": ""},
          "nodes": []
        }
      }]
    }
  }
}`

const gqlNamespaceNull = `{"namespace": null}`

const gqlWorkItemGIDData = `{
  "namespace": {
    "workItem": {"id": "gid://gitlab/WorkItem/1"}
  }
}`

const gqlChildWorkItemGIDData = `{
  "namespace": {
    "workItem": {"id": "gid://gitlab/WorkItem/10"}
  }
}`

const gqlAddChildData = `{
  "workItemUpdate": {
    "workItem": {"id": "gid://gitlab/WorkItem/1"},
    "errors": []
  }
}`

const gqlRemoveParentData = `{
  "workItemUpdate": {
    "workItem": {"id": "gid://gitlab/WorkItem/10"},
    "errors": []
  }
}`

const gqlReorderData = `{
  "workItemUpdate": {
    "workItem": {
      "id": "gid://gitlab/WorkItem/1",
      "widgets": [{
        "children": {
          "nodes": [
            {
              "id": "gid://gitlab/WorkItem/20",
              "iid": "20",
              "title": "Add feature",
              "state": "CLOSED",
              "webUrl": "https://gitlab.example.com/my-group/my-project/-/issues/20",
              "createdAt": "2026-02-01T12:00:00Z",
              "updatedAt": "2026-02-02T12:00:00Z",
              "author": {"username": "bob"},
              "widgets": [{"labels": {"nodes": []}}]
            },
            {
              "id": "gid://gitlab/WorkItem/10",
              "iid": "10",
              "title": "Fix login bug",
              "state": "OPEN",
              "webUrl": "https://gitlab.example.com/my-group/my-project/-/issues/10",
              "createdAt": "2026-01-15T10:00:00Z",
              "updatedAt": "2026-01-16T10:00:00Z",
              "author": {"username": "alice"},
              "widgets": [{"labels": {"nodes": [{"title": "bug"}]}}]
            }
          ]
        }
      }]
    },
    "errors": []
  }
}`

const gqlMutationErrors = `{
  "workItemUpdate": {
    "workItem": null,
    "errors": ["Something went wrong"]
  }
}`

// graphqlMux creates an http.Handler that routes GraphQL requests by query content.
func graphqlMux(handlers map[string]http.HandlerFunc) http.Handler {
	return testutil.GraphQLHandler(handlers)
}

// resolveHandler returns an http.HandlerFunc that resolves work item GIDs.
// When the query variables contain childPath, it returns the child GID;
// otherwise it returns the epic GID.
func resolveHandler(childPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars, _ := testutil.ParseGraphQLVariables(r)
		if fp, ok := vars["fullPath"].(string); ok && fp == childPath {
			testutil.RespondGraphQL(w, http.StatusOK, gqlChildWorkItemGIDData)
		} else {
			testutil.RespondGraphQL(w, http.StatusOK, gqlWorkItemGIDData)
		}
	}
}

// --------------------------------------------------------------------------
// List
// --------------------------------------------------------------------------

func TestList(t *testing.T) {
	tests := []struct {
		name    string
		input   ListInput
		handler http.Handler
		wantErr string
		check   func(t *testing.T, out ListOutput)
	}{
		{
			name:  "returns child issues with correct fields",
			input: ListInput{FullPath: testFullPath, IID: 1},
			handler: graphqlMux(map[string]http.HandlerFunc{"WorkItemWidgetHierarchy": func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondGraphQL(w, http.StatusOK, gqlChildrenData)
			}}),
			check: func(t *testing.T, out ListOutput) {
				t.Helper()
				if len(out.Issues) != 1 {
					t.Fatalf("got %d issues, want 1", len(out.Issues))
				}
				issue := out.Issues[0]
				if issue.ID != "gid://gitlab/WorkItem/10" {
					t.Errorf("ID = %q, want gid://gitlab/WorkItem/10", issue.ID)
				}
				if issue.IID != 10 {
					t.Errorf("IID = %d, want 10", issue.IID)
				}
				if issue.Title != "Fix login bug" {
					t.Errorf("Title = %q, want %q", issue.Title, "Fix login bug")
				}
				if issue.State != "opened" {
					t.Errorf("State = %q, want opened", issue.State)
				}
				if issue.Author != "alice" {
					t.Errorf("Author = %q, want alice", issue.Author)
				}
				if len(issue.Labels) != 2 || issue.Labels[0] != "bug" {
					t.Errorf("Labels = %v, want [bug critical]", issue.Labels)
				}
			},
		},
		{
			name:  "returns multiple issues with pagination",
			input: ListInput{FullPath: testFullPath, IID: 1},
			handler: graphqlMux(map[string]http.HandlerFunc{"WorkItemWidgetHierarchy": func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondGraphQL(w, http.StatusOK, gqlChildrenMultiple)
			}}),
			check: func(t *testing.T, out ListOutput) {
				t.Helper()
				if len(out.Issues) != 2 {
					t.Fatalf("got %d issues, want 2", len(out.Issues))
				}
				if out.Issues[1].State != "closed" {
					t.Errorf("Issues[1].State = %q, want closed", out.Issues[1].State)
				}
				if !out.Pagination.HasNextPage {
					t.Error("HasNextPage = false, want true")
				}
				if out.Pagination.EndCursor != "cursor2" {
					t.Errorf("EndCursor = %q, want cursor2", out.Pagination.EndCursor)
				}
			},
		},
		{
			name:  "passes pagination parameters",
			input: ListInput{FullPath: testFullPath, IID: 1, GraphQLPaginationInput: toolutil.GraphQLPaginationInput{First: new(5), After: "abc"}},
			handler: graphqlMux(map[string]http.HandlerFunc{"WorkItemWidgetHierarchy": func(w http.ResponseWriter, r *http.Request) {
				vars, _ := testutil.ParseGraphQLVariables(r)
				if first, ok := vars["first"].(float64); !ok || int(first) != 5 {
					t.Errorf("first = %v, want 5", vars["first"])
				}
				if after, ok := vars["after"].(string); !ok || after != "abc" {
					t.Errorf("after = %v, want abc", vars["after"])
				}
				testutil.RespondGraphQL(w, http.StatusOK, gqlChildrenData)
			}}),
			check: func(t *testing.T, out ListOutput) {
				t.Helper()
				if len(out.Issues) != 1 {
					t.Fatalf("got %d issues, want 1", len(out.Issues))
				}
			},
		},
		{
			name:  "returns empty list when no children",
			input: ListInput{FullPath: testFullPath, IID: 1},
			handler: graphqlMux(map[string]http.HandlerFunc{"WorkItemWidgetHierarchy": func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondGraphQL(w, http.StatusOK, gqlChildrenEmpty)
			}}),
			check: func(t *testing.T, out ListOutput) {
				t.Helper()
				if len(out.Issues) != 0 {
					t.Fatalf("got %d issues, want 0", len(out.Issues))
				}
			},
		},
		{
			name:  "returns error when epic not found",
			input: ListInput{FullPath: testFullPath, IID: 999},
			handler: graphqlMux(map[string]http.HandlerFunc{"WorkItemWidgetHierarchy": func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondGraphQL(w, http.StatusOK, gqlNamespaceNull)
			}}),
			wantErr: "epic not found",
		},
		{
			name:    "returns error when full_path is empty",
			input:   ListInput{IID: 1},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "full_path is required",
		},
		{
			name:    "returns error when iid is zero",
			input:   ListInput{FullPath: testFullPath, IID: 0},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "iid",
		},
		{
			name:    "returns error when iid is negative",
			input:   ListInput{FullPath: testFullPath, IID: -1},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "iid",
		},
		{
			name:  "returns error on API server error",
			input: ListInput{FullPath: testFullPath, IID: 1},
			handler: graphqlMux(map[string]http.HandlerFunc{"WorkItemWidgetHierarchy": func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "server error", http.StatusForbidden)
			}}),
			wantErr: "epicIssueList",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			out, err := List(context.Background(), client, tt.input)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("List() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("List() unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, out)
			}
		})
	}
}

func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, graphqlMux(map[string]http.HandlerFunc{}))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{FullPath: testFullPath, IID: 1})
	if err == nil {
		t.Fatal("List() expected context error, got nil")
	}
}

// --------------------------------------------------------------------------
// Assign
// --------------------------------------------------------------------------

func TestAssign(t *testing.T) {
	tests := []struct {
		name    string
		input   AssignInput
		handler http.Handler
		wantErr string
		check   func(t *testing.T, out AssignOutput)
	}{
		{
			name:  "assigns issue to epic successfully",
			input: AssignInput{FullPath: testFullPath, IID: 1, ChildProjectPath: testChildProject, ChildIID: 10},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"workItem(iid": resolveHandler(testChildProject),
				"workItemUpdate(": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlAddChildData)
				},
			}),
			check: func(t *testing.T, out AssignOutput) {
				t.Helper()
				if out.EpicGID != "gid://gitlab/WorkItem/1" {
					t.Errorf("EpicGID = %q, want gid://gitlab/WorkItem/1", out.EpicGID)
				}
				if out.ChildGID != "gid://gitlab/WorkItem/10" {
					t.Errorf("ChildGID = %q, want gid://gitlab/WorkItem/10", out.ChildGID)
				}
			},
		},
		{
			name:    "returns error when full_path is empty",
			input:   AssignInput{IID: 1, ChildProjectPath: testChildProject, ChildIID: 10},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "full_path is required",
		},
		{
			name:    "returns error when iid is zero",
			input:   AssignInput{FullPath: testFullPath, ChildProjectPath: testChildProject, ChildIID: 10},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "iid",
		},
		{
			name:    "returns error when iid is negative",
			input:   AssignInput{FullPath: testFullPath, IID: -5, ChildProjectPath: testChildProject, ChildIID: 10},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "iid",
		},
		{
			name:    "returns error when child_project_path is empty",
			input:   AssignInput{FullPath: testFullPath, IID: 1, ChildIID: 10},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "child_project_path is required",
		},
		{
			name:    "returns error when child_iid is zero",
			input:   AssignInput{FullPath: testFullPath, IID: 1, ChildProjectPath: testChildProject},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "child_iid",
		},
		{
			name:  "returns error when epic GID resolution fails",
			input: AssignInput{FullPath: testFullPath, IID: 1, ChildProjectPath: testChildProject, ChildIID: 10},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"workItem(iid": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlNamespaceNull)
				},
			}),
			wantErr: "work item not found",
		},
		{
			name:  "returns error on mutation GraphQL errors",
			input: AssignInput{FullPath: testFullPath, IID: 1, ChildProjectPath: testChildProject, ChildIID: 10},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"workItem(iid": resolveHandler(testChildProject),
				"workItemUpdate(": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlMutationErrors)
				},
			}),
			wantErr: "Something went wrong",
		},
		{
			name:  "returns error on API server error",
			input: AssignInput{FullPath: testFullPath, IID: 1, ChildProjectPath: testChildProject, ChildIID: 10},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"workItem(iid": func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "bad", http.StatusForbidden) },
			}),
			wantErr: "epicIssueAssign",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			out, err := Assign(context.Background(), client, tt.input)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Assign() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Assign() unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, out)
			}
		})
	}
}

func TestAssign_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, graphqlMux(map[string]http.HandlerFunc{}))
	ctx := testutil.CancelledCtx(t)
	_, err := Assign(ctx, client, AssignInput{FullPath: testFullPath, IID: 1, ChildProjectPath: testChildProject, ChildIID: 10})
	if err == nil {
		t.Fatal("Assign() expected context error, got nil")
	}
}

// --------------------------------------------------------------------------
// Remove
// --------------------------------------------------------------------------

func TestRemove(t *testing.T) {
	tests := []struct {
		name    string
		input   RemoveInput
		handler http.Handler
		wantErr string
		check   func(t *testing.T, out AssignOutput)
	}{
		{
			name:  "removes issue from epic successfully",
			input: RemoveInput{FullPath: testFullPath, IID: 1, ChildProjectPath: testChildProject, ChildIID: 10},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"workItem(iid": resolveHandler(testChildProject),
				"workItemUpdate(": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlRemoveParentData)
				},
			}),
			check: func(t *testing.T, out AssignOutput) {
				t.Helper()
				if out.EpicGID != "gid://gitlab/WorkItem/1" {
					t.Errorf("EpicGID = %q, want gid://gitlab/WorkItem/1", out.EpicGID)
				}
				if out.ChildGID != "gid://gitlab/WorkItem/10" {
					t.Errorf("ChildGID = %q, want gid://gitlab/WorkItem/10", out.ChildGID)
				}
			},
		},
		{
			name:    "returns error when full_path is empty",
			input:   RemoveInput{IID: 1, ChildProjectPath: testChildProject, ChildIID: 10},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "full_path is required",
		},
		{
			name:    "returns error when iid is zero",
			input:   RemoveInput{FullPath: testFullPath, ChildProjectPath: testChildProject, ChildIID: 10},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "iid",
		},
		{
			name:    "returns error when iid is negative",
			input:   RemoveInput{FullPath: testFullPath, IID: -3, ChildProjectPath: testChildProject, ChildIID: 10},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "iid",
		},
		{
			name:    "returns error when child_project_path is empty",
			input:   RemoveInput{FullPath: testFullPath, IID: 1, ChildIID: 10},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "child_project_path is required",
		},
		{
			name:    "returns error when child_iid is zero",
			input:   RemoveInput{FullPath: testFullPath, IID: 1, ChildProjectPath: testChildProject},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "child_iid",
		},
		{
			name:  "returns error when child GID resolution fails",
			input: RemoveInput{FullPath: testFullPath, IID: 1, ChildProjectPath: testChildProject, ChildIID: 10},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"workItem(iid": func(w http.ResponseWriter, r *http.Request) {
					vars, _ := testutil.ParseGraphQLVariables(r)
					if fp, ok := vars["fullPath"].(string); ok && fp == testChildProject {
						testutil.RespondGraphQL(w, http.StatusOK, gqlNamespaceNull)
					} else {
						testutil.RespondGraphQL(w, http.StatusOK, gqlWorkItemGIDData)
					}
				},
			}),
			wantErr: "work item not found",
		},
		{
			name:  "returns error on mutation GraphQL errors",
			input: RemoveInput{FullPath: testFullPath, IID: 1, ChildProjectPath: testChildProject, ChildIID: 10},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"workItem(iid": resolveHandler(testChildProject),
				"workItemUpdate(": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlMutationErrors)
				},
			}),
			wantErr: "Something went wrong",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			out, err := Remove(context.Background(), client, tt.input)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Remove() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Remove() unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, out)
			}
		})
	}
}

func TestRemove_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, graphqlMux(map[string]http.HandlerFunc{}))
	ctx := testutil.CancelledCtx(t)
	_, err := Remove(ctx, client, RemoveInput{FullPath: testFullPath, IID: 1, ChildProjectPath: testChildProject, ChildIID: 10})
	if err == nil {
		t.Fatal("Remove() expected context error, got nil")
	}
}

// --------------------------------------------------------------------------
// UpdateOrder
// --------------------------------------------------------------------------

func TestUpdateOrder(t *testing.T) {
	tests := []struct {
		name    string
		input   UpdateInput
		handler http.Handler
		wantErr string
		check   func(t *testing.T, out ListOutput)
	}{
		{
			name: "reorders child issue BEFORE another",
			input: UpdateInput{
				FullPath: testFullPath, IID: 1,
				ChildID: "gid://gitlab/WorkItem/10", AdjacentID: "gid://gitlab/WorkItem/20",
				RelativePosition: "BEFORE",
			},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"workItem(iid": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlWorkItemGIDData)
				},
				"workItemUpdate(": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlReorderData)
				},
			}),
			check: func(t *testing.T, out ListOutput) {
				t.Helper()
				if len(out.Issues) != 2 {
					t.Fatalf("got %d issues, want 2", len(out.Issues))
				}
				if out.Issues[0].IID != 20 {
					t.Errorf("Issues[0].IID = %d, want 20 (reordered first)", out.Issues[0].IID)
				}
				if out.Issues[1].IID != 10 {
					t.Errorf("Issues[1].IID = %d, want 10 (reordered second)", out.Issues[1].IID)
				}
			},
		},
		{
			name: "reorders child issue AFTER another",
			input: UpdateInput{
				FullPath: testFullPath, IID: 1,
				ChildID: "gid://gitlab/WorkItem/10", AdjacentID: "gid://gitlab/WorkItem/20",
				RelativePosition: "AFTER",
			},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"workItem(iid": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlWorkItemGIDData)
				},
				"workItemUpdate(": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlReorderData)
				},
			}),
			check: func(t *testing.T, out ListOutput) {
				t.Helper()
				if len(out.Issues) != 2 {
					t.Fatalf("got %d issues, want 2", len(out.Issues))
				}
			},
		},
		{
			name: "accepts lowercase relative position",
			input: UpdateInput{
				FullPath: testFullPath, IID: 1,
				ChildID: "gid://gitlab/WorkItem/10", AdjacentID: "gid://gitlab/WorkItem/20",
				RelativePosition: "before",
			},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"workItem(iid": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlWorkItemGIDData)
				},
				"workItemUpdate(": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlReorderData)
				},
			}),
			check: func(t *testing.T, out ListOutput) {
				t.Helper()
				if len(out.Issues) != 2 {
					t.Fatalf("got %d issues, want 2", len(out.Issues))
				}
			},
		},
		{
			name:    "returns error when full_path is empty",
			input:   UpdateInput{IID: 1, ChildID: "gid://gitlab/WorkItem/10", AdjacentID: "gid://gitlab/WorkItem/20", RelativePosition: "BEFORE"},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "full_path is required",
		},
		{
			name:    "returns error when iid is zero",
			input:   UpdateInput{FullPath: testFullPath, ChildID: "gid://gitlab/WorkItem/10", AdjacentID: "gid://gitlab/WorkItem/20", RelativePosition: "BEFORE"},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "iid",
		},
		{
			name:    "returns error when child_id is empty",
			input:   UpdateInput{FullPath: testFullPath, IID: 1, AdjacentID: "gid://gitlab/WorkItem/20", RelativePosition: "BEFORE"},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "child_id is required",
		},
		{
			name:    "returns error when adjacent_id is empty",
			input:   UpdateInput{FullPath: testFullPath, IID: 1, ChildID: "gid://gitlab/WorkItem/10", RelativePosition: "BEFORE"},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "adjacent_id is required",
		},
		{
			name:    "returns error when relative_position is empty",
			input:   UpdateInput{FullPath: testFullPath, IID: 1, ChildID: "gid://gitlab/WorkItem/10", AdjacentID: "gid://gitlab/WorkItem/20"},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "relative_position is required",
		},
		{
			name:    "returns error for invalid relative_position",
			input:   UpdateInput{FullPath: testFullPath, IID: 1, ChildID: "gid://gitlab/WorkItem/10", AdjacentID: "gid://gitlab/WorkItem/20", RelativePosition: "ABOVE"},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "BEFORE or AFTER",
		},
		{
			name: "returns error on mutation GraphQL errors",
			input: UpdateInput{
				FullPath: testFullPath, IID: 1,
				ChildID: "gid://gitlab/WorkItem/10", AdjacentID: "gid://gitlab/WorkItem/20",
				RelativePosition: "BEFORE",
			},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"workItem(iid": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlWorkItemGIDData)
				},
				"workItemUpdate(": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlMutationErrors)
				},
			}),
			wantErr: "Something went wrong",
		},
		{
			name: "returns error on API server error",
			input: UpdateInput{
				FullPath: testFullPath, IID: 1,
				ChildID: "gid://gitlab/WorkItem/10", AdjacentID: "gid://gitlab/WorkItem/20",
				RelativePosition: "BEFORE",
			},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"workItem(iid": func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "bad", http.StatusForbidden) },
			}),
			wantErr: "epicIssueUpdate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			out, err := UpdateOrder(context.Background(), client, tt.input)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("UpdateOrder() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("UpdateOrder() unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, out)
			}
		})
	}
}

func TestUpdateOrder_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, graphqlMux(map[string]http.HandlerFunc{}))
	ctx := testutil.CancelledCtx(t)
	_, err := UpdateOrder(ctx, client, UpdateInput{
		FullPath: testFullPath, IID: 1,
		ChildID: "gid://gitlab/WorkItem/10", AdjacentID: "gid://gitlab/WorkItem/20",
		RelativePosition: "BEFORE",
	})
	if err == nil {
		t.Fatal("UpdateOrder() expected context error, got nil")
	}
}

// --------------------------------------------------------------------------
// normalizeState
// --------------------------------------------------------------------------

func TestNormalizeState(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"OPEN", "opened"},
		{"CLOSED", "closed"},
		{"open", "opened"},
		{"closed", "closed"},
		{"UNKNOWN", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := normalizeState(tt.input); got != tt.want {
				t.Errorf("normalizeState(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --------------------------------------------------------------------------
// Markdown formatters
// --------------------------------------------------------------------------

func TestFormatListMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    ListOutput
		contains []string
		excludes []string
	}{
		{
			name: "renders issues table with labels",
			input: ListOutput{
				Issues: []ChildOutput{
					{IID: 10, Title: "Fix login bug", State: "opened", Author: "alice", Labels: []string{"bug", "critical"}, CreatedAt: "2026-01-15T10:00:00Z"},
					{IID: 20, Title: "Add feature", State: "closed", Author: "bob", CreatedAt: "2026-02-01T12:00:00Z"},
				},
			},
			contains: []string{
				"## Epic Issues (2)",
				"| IID | Title | State | Author | Labels | Created |",
				"#10", "Fix login bug", "opened", "alice", "bug, critical",
				"#20", "Add feature", "closed", "bob",
			},
		},
		{
			name:  "renders empty list message",
			input: ListOutput{Issues: nil},
			contains: []string{
				"## Epic Issues",
				"No issues found in this epic",
			},
			excludes: []string{"| IID |"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := FormatListMarkdown(tt.input)
			for _, s := range tt.contains {
				if !strings.Contains(md, s) {
					t.Errorf("missing %q in:\n%s", s, md)
				}
			}
			for _, s := range tt.excludes {
				if strings.Contains(md, s) {
					t.Errorf("unexpected %q in:\n%s", s, md)
				}
			}
		})
	}
}

func TestFormatAssignMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		out      AssignOutput
		action   string
		contains []string
	}{
		{
			name:     "renders assigned action",
			out:      AssignOutput{EpicGID: "gid://gitlab/WorkItem/1", ChildGID: "gid://gitlab/WorkItem/10"},
			action:   "assigned",
			contains: []string{"## Epic Issue assigned", "gid://gitlab/WorkItem/1", "gid://gitlab/WorkItem/10"},
		},
		{
			name:     "renders removed action",
			out:      AssignOutput{EpicGID: "gid://gitlab/WorkItem/1", ChildGID: "gid://gitlab/WorkItem/10"},
			action:   "removed",
			contains: []string{"## Epic Issue removed", "gid://gitlab/WorkItem/1", "gid://gitlab/WorkItem/10"},
		},
		{
			name:     "handles empty GIDs",
			out:      AssignOutput{},
			action:   "assigned",
			contains: []string{"## Epic Issue assigned"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := FormatAssignMarkdown(tt.out, tt.action)
			for _, s := range tt.contains {
				if !strings.Contains(md, s) {
					t.Errorf("missing %q in:\n%s", s, md)
				}
			}
		})
	}
}
