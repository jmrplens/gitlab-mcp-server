// action_specs_test.go contains canonical-route tests for issue discussion actions.
package issuediscussions

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_CallAllRoutes exercises every issue discussion tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := issueDiscussionSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, issueDiscussionsActionHandler())))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_list_issue_discussions", map[string]any{"project_id": "42", "issue_iid": 10}},
		{"gitlab_get_issue_discussion", map[string]any{"project_id": "42", "issue_iid": 10, "discussion_id": testDiscussionID}},
		{"gitlab_create_issue_discussion", map[string]any{"project_id": "42", "issue_iid": 10, "body": "New discussion"}},
		{"gitlab_add_issue_discussion_note", map[string]any{"project_id": "42", "issue_iid": 10, "discussion_id": testDiscussionID, "body": "Reply"}},
		{"gitlab_update_issue_discussion_note", map[string]any{"project_id": "42", "issue_iid": 10, "discussion_id": testDiscussionID, "note_id": 300, "body": "Updated"}},
		{"gitlab_delete_issue_discussion_note", map[string]any{"project_id": "42", "issue_iid": 10, "discussion_id": testDiscussionID, "note_id": 300}},
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

// TestActionSpecs_DeleteError verifies that the delete-note route propagates backend errors.
func TestActionSpecs_DeleteError(t *testing.T) {
	byTool := issueDiscussionSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))))

	_, err := byTool["gitlab_delete_issue_discussion_note"].Route.Handler(t.Context(), map[string]any{
		"project_id": "my-project", "issue_iid": 1, "discussion_id": testDiscussionID, "note_id": 1,
	})
	if err == nil {
		t.Fatal("expected error from delete with failing backend")
	}
}

// TestActionSpecs_DeleteOutput verifies the delete route preserves its success message.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	byTool := issueDiscussionSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, issueDiscussionsActionHandler())))

	result, err := byTool["gitlab_delete_issue_discussion_note"].Route.Handler(t.Context(), map[string]any{
		"project_id": "42", "issue_iid": 10, "discussion_id": testDiscussionID, "note_id": 300,
	})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_delete_issue_discussion_note) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_delete_issue_discussion_note) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted issue discussion note." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API when confirm is declined")
	}))
	byTool := issueDiscussionSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_delete_issue_discussion_note"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test issue discussion destructive confirmation.",
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
		Name:      "gitlab_delete_issue_discussion_note",
		Arguments: map[string]any{"project_id": "42", "issue_iid": 10, "discussion_id": testDiscussionID, "note_id": 300},
	})
	if err != nil {
		t.Fatalf("CallTool returned transport error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result when confirmation is declined")
	}
}

func issueDiscussionsActionHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/discussions"):
			testutil.RespondJSON(w, http.StatusCreated, discussionJSONCoverage)
		case r.Method == http.MethodPost && strings.Contains(path, "/discussions/") && strings.HasSuffix(path, "/notes"):
			testutil.RespondJSON(w, http.StatusCreated, noteJSONCoverage)
		case r.Method == http.MethodPut && strings.Contains(path, "/notes/"):
			testutil.RespondJSON(w, http.StatusOK, noteJSONCoverage)
		case r.Method == http.MethodDelete && strings.Contains(path, "/notes/"):
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && strings.Contains(path, "/discussions/"):
			testutil.RespondJSON(w, http.StatusOK, discussionJSONCoverage)
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/discussions"):
			testutil.RespondJSON(w, http.StatusOK, "["+discussionJSONCoverage+"]")
		default:
			http.NotFound(w, r)
		}
	})
}

func issueDiscussionSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
