// action_specs_test.go contains canonical-route tests for issue note actions.
package issuenotes

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_CallAllRoutes exercises every issue note tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := issueNoteSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, issueNotesActionHandler())))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_issue_note_create", map[string]any{"project_id": testProjectID, "issue_iid": 10, "body": "test note"}},
		{"gitlab_issue_note_list", map[string]any{"project_id": testProjectID, "issue_iid": 10}},
		{"gitlab_issue_note_get", map[string]any{"project_id": testProjectID, "issue_iid": 10, "note_id": 100}},
		{"gitlab_issue_note_update", map[string]any{"project_id": testProjectID, "issue_iid": 10, "note_id": 100, "body": "updated"}},
		{"gitlab_issue_note_delete", map[string]any{"project_id": testProjectID, "issue_iid": 10, "note_id": 100}},
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

// TestActionSpecs_MutationErrors verifies get-404 and delete backend error routes.
func TestActionSpecs_MutationErrors(t *testing.T) {
	byTool := issueNoteSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
		case http.MethodDelete:
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
		default:
			testutil.RespondJSON(w, http.StatusOK, `{}`)
		}
	}))))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_issue_note_get", map[string]any{"project_id": testProjectID, "issue_iid": 1, "note_id": 999}},
		{"gitlab_issue_note_delete", map[string]any{"project_id": testProjectID, "issue_iid": 1, "note_id": 1}},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			_, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err == nil {
				t.Fatal("expected route error")
			}
		})
	}
}

// TestActionSpecs_DeleteOutput verifies the delete route preserves its success message.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	byTool := issueNoteSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, issueNotesActionHandler())))

	result, err := byTool["gitlab_issue_note_delete"].Route.Handler(t.Context(), map[string]any{
		"project_id": testProjectID, "issue_iid": 10, "note_id": 100,
	})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_issue_note_delete) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_issue_note_delete) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted note 100 from issue #10 in project 42." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API when confirm is declined")
	}))
	byTool := issueNoteSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_issue_note_delete"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test issue note destructive confirmation.",
		Icons:       toolutil.IconDiscussion,
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

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_issue_note_delete",
		Arguments: map[string]any{"project_id": testProjectID, "issue_iid": 1, "note_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

func issueNotesActionHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodPost && path == pathIssueNotes:
			testutil.RespondJSON(w, http.StatusCreated, noteJSONSimple)
		case r.Method == http.MethodGet && path == pathIssueNotes:
			testutil.RespondJSONWithPagination(w, http.StatusOK, "["+noteJSONSimple+"]",
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
		case r.Method == http.MethodGet && path == pathIssueNote100:
			testutil.RespondJSON(w, http.StatusOK, noteJSONSimple)
		case r.Method == http.MethodPut && path == pathIssueNote100:
			testutil.RespondJSON(w, http.StatusOK, noteJSONSimple)
		case r.Method == http.MethodDelete && path == pathIssueNote100:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
}

func issueNoteSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
