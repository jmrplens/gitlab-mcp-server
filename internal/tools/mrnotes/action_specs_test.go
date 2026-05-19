// action_specs_test.go contains canonical-route tests for merge request note actions.
package mrnotes

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const mrNoteJSON = `{"id":200,"body":"comment","author":{"id":1,"username":"jmrplens"},"created_at":"2026-03-02T12:00:00Z","updated_at":"2026-03-02T12:00:00Z","system":false}`

// TestActionSpecs_CallAllRoutes exercises every merge request note tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := mrNoteSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, mrNotesActionHandler())))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_mr_note_create", map[string]any{"project_id": testProjectID, "merge_request_iid": 1, "body": "comment"}},
		{"gitlab_mr_notes_list", map[string]any{"project_id": testProjectID, "merge_request_iid": 1}},
		{"gitlab_mr_note_update", map[string]any{"project_id": testProjectID, "merge_request_iid": 1, "note_id": 200, "body": "updated"}},
		{"gitlab_mr_note_get", map[string]any{"project_id": testProjectID, "merge_request_iid": 1, "note_id": 200}},
		{"gitlab_mr_note_delete", map[string]any{"project_id": testProjectID, "merge_request_iid": 1, "note_id": 200}},
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

// TestActionSpecs_ErrorPaths verifies canonical routes propagate backend errors.
func TestActionSpecs_ErrorPaths(t *testing.T) {
	byTool := mrNoteSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_mr_notes_list", map[string]any{"project_id": testProjectID, "merge_request_iid": 1}},
		{"gitlab_mr_note_get", map[string]any{"project_id": testProjectID, "merge_request_iid": 1, "note_id": 1}},
		{"gitlab_mr_note_create", map[string]any{"project_id": testProjectID, "merge_request_iid": 1, "body": "x"}},
		{"gitlab_mr_note_update", map[string]any{"project_id": testProjectID, "merge_request_iid": 1, "note_id": 1, "body": "x"}},
		{"gitlab_mr_note_delete", map[string]any{"project_id": testProjectID, "merge_request_iid": 1, "note_id": 1}},
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

// TestActionSpecs_GetNotFound verifies the get route reports missing notes as errors.
func TestActionSpecs_GetNotFound(t *testing.T) {
	byTool := mrNoteSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))))

	_, err := byTool["gitlab_mr_note_get"].Route.Handler(t.Context(), map[string]any{
		"project_id": testProjectID, "merge_request_iid": 1, "note_id": 999,
	})
	if err == nil {
		t.Fatal("expected route error for missing note")
	}
}

// TestActionSpecs_DeleteOutput verifies the delete route preserves its success message.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	byTool := mrNoteSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, mrNotesActionHandler())))

	result, err := byTool["gitlab_mr_note_delete"].Route.Handler(t.Context(), map[string]any{
		"project_id": testProjectID, "merge_request_iid": 1, "note_id": 200,
	})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_mr_note_delete) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_mr_note_delete) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted note 200 from MR !1 in project 42." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API when confirm is declined")
	}))
	byTool := mrNoteSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_mr_note_delete"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test MR note destructive confirmation.",
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
		Name:      "gitlab_mr_note_delete",
		Arguments: map[string]any{"project_id": testProjectID, "merge_request_iid": 1, "note_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

// TestToOutput_ResolvedByAndTimestamps covers optional note fields set by GitLab.
func TestToOutput_ResolvedByAndTimestamps(t *testing.T) {
	now := time.Now()
	note := &gl.Note{
		ID:         1,
		Body:       "test",
		Author:     gl.NoteAuthor{Username: "author"},
		ResolvedBy: gl.NoteResolvedBy{Username: "resolver"},
		ResolvedAt: &now,
		CreatedAt:  &now,
		UpdatedAt:  &now,
	}
	out := ToOutput(note)
	if out.ResolvedBy != "resolver" {
		t.Errorf("ResolvedBy = %q, want %q", out.ResolvedBy, "resolver")
	}
	if out.ResolvedAt == "" {
		t.Error("expected non-empty ResolvedAt")
	}
	if out.CreatedAt == "" {
		t.Error("expected non-empty CreatedAt")
	}
	if out.UpdatedAt == "" {
		t.Error("expected non-empty UpdatedAt")
	}
}

func mrNotesActionHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/notes"):
			testutil.RespondJSON(w, http.StatusCreated, mrNoteJSON)
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/notes"):
			testutil.RespondJSON(w, http.StatusOK, "["+mrNoteJSON+"]")
		case r.Method == http.MethodPut && strings.Contains(path, "/notes/"):
			testutil.RespondJSON(w, http.StatusOK, mrNoteJSON)
		case r.Method == http.MethodGet && strings.Contains(path, "/notes/"):
			testutil.RespondJSON(w, http.StatusOK, mrNoteJSON)
		case r.Method == http.MethodDelete && strings.Contains(path, "/notes/"):
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
}

func mrNoteSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
