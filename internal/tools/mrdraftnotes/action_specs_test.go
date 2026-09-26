// action_specs_test.go contains canonical-route tests for merge request draft note actions.
package mrdraftnotes

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const (
	draftNoteActionJSON     = `{"id":10,"author_id":1,"merge_request_id":1,"note":"Draft comment","commit_id":"abc123","discussion_id":"disc1","resolve_discussion":false}`
	draftNoteActionListJSON = `[` + draftNoteActionJSON + `]`
)

// TestActionSpecs_CallAllRoutes exercises every draft note tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := mrDraftNoteSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, mrDraftNotesActionHandler())))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_mr_draft_note_list", map[string]any{"project_id": "42", "merge_request_iid": 1}},
		{"gitlab_mr_draft_note_get", map[string]any{"project_id": "42", "merge_request_iid": 1, "note_id": 10}},
		{"gitlab_mr_draft_note_create", map[string]any{"project_id": "42", "merge_request_iid": 1, "note": "new draft"}},
		{"gitlab_mr_draft_note_update", map[string]any{"project_id": "42", "merge_request_iid": 1, "note_id": 10, "note": "updated"}},
		{"gitlab_mr_draft_note_delete", map[string]any{"project_id": "42", "merge_request_iid": 1, "note_id": 10}},
		{"gitlab_mr_draft_note_publish", map[string]any{"project_id": "42", "merge_request_iid": 1, "note_id": 10}},
		{"gitlab_mr_draft_note_publish_all", map[string]any{"project_id": "42", "merge_request_iid": 1}},
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
	byTool := mrDraftNoteSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_mr_draft_note_list", map[string]any{"project_id": "42", "merge_request_iid": 1}},
		{"gitlab_mr_draft_note_create", map[string]any{"project_id": "42", "merge_request_iid": 1, "note": "x"}},
		{"gitlab_mr_draft_note_update", map[string]any{"project_id": "42", "merge_request_iid": 1, "note_id": 1, "note": "x"}},
		{"gitlab_mr_draft_note_delete", map[string]any{"project_id": "42", "merge_request_iid": 1, "note_id": 1}},
		{"gitlab_mr_draft_note_publish", map[string]any{"project_id": "42", "merge_request_iid": 1, "note_id": 1}},
		{"gitlab_mr_draft_note_publish_all", map[string]any{"project_id": "42", "merge_request_iid": 1}},
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

// TestActionSpecs_GetNotFound verifies the get route reports missing draft notes as errors.
func TestActionSpecs_GetNotFound(t *testing.T) {
	byTool := mrDraftNoteSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))))

	_, err := byTool["gitlab_mr_draft_note_get"].Route.Handler(t.Context(), map[string]any{
		"project_id": "42", "merge_request_iid": 1, "note_id": 999,
	})
	if err == nil {
		t.Fatal("expected route error for missing draft note")
	}
}

// TestActionSpecs_DeleteOutput verifies the delete route preserves its success message.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	byTool := mrDraftNoteSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, mrDraftNotesActionHandler())))

	result, err := byTool["gitlab_mr_draft_note_delete"].Route.Handler(t.Context(), map[string]any{
		"project_id": "42", "merge_request_iid": 1, "note_id": 10,
	})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_mr_draft_note_delete) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_mr_draft_note_delete) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted draft note 10 from MR !1 in project 42." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestActionSpecs_PublishOutputsIncludeStatus verifies publish routes return structured success status.
func TestActionSpecs_PublishOutputsIncludeStatus(t *testing.T) {
	byTool := mrDraftNoteSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, mrDraftNotesActionHandler())))

	tests := []struct {
		tool string
		args map[string]any
		want string
	}{
		{"gitlab_mr_draft_note_publish", map[string]any{"project_id": "42", "merge_request_iid": 1, "note_id": 10}, "Draft note 10 published on MR !1 in project 42"},
		{"gitlab_mr_draft_note_publish_all", map[string]any{"project_id": "42", "merge_request_iid": 1}, "All draft notes published on MR !1 in project 42"},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			result, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			out, ok := result.(toolutil.DeleteOutput)
			if !ok {
				t.Fatalf("Route.Handler(%s) returned %T, want toolutil.DeleteOutput", tt.tool, result)
			}
			if out.Status != "success" {
				t.Fatalf("Status = %q, want success", out.Status)
			}
			if out.Message != tt.want {
				t.Fatalf("Message = %q, want %q", out.Message, tt.want)
			}
		})
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	byTool := mrDraftNoteSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_mr_draft_note_delete"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test MR draft note destructive confirmation.",
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
		Name:      "gitlab_mr_draft_note_delete",
		Arguments: map[string]any{"project_id": "42", "merge_request_iid": 1, "note_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

// TestCreate_MissingBody covers Create validation when note is empty.
func TestCreate_MissingBody(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := Create(context.Background(), client, CreateInput{ProjectID: "1", MRIID: 1})
	if err == nil {
		t.Fatal("expected error for missing body")
	}
}

// TestUpdate_MissingBody covers Update validation when note is empty.
func TestUpdate_MissingBody(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "1", MRIID: 1, NoteID: 1})
	if err == nil {
		t.Fatal("expected error for missing body")
	}
}

// TestCreate_BadRequest verifies Create returns guidance for invalid draft note payloads.
func TestCreate_BadRequest(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: "1", MRIID: 1, Note: "bad"})
	if err == nil {
		t.Fatal("expected bad request error")
	}
	if !strings.Contains(err.Error(), "note body is required") {
		t.Fatalf("error = %v, want create payload hint", err)
	}
}

// TestUpdate_Forbidden verifies Update returns guidance for author-only draft notes.
func TestUpdate_Forbidden(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"forbidden"}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "1", MRIID: 1, NoteID: 1, Note: "updated"})
	if err == nil {
		t.Fatal("expected forbidden error")
	}
	if !strings.Contains(err.Error(), "only the draft author can update") {
		t.Fatalf("error = %v, want author-only hint", err)
	}
}

// TestCreateUpdate_InvalidPosition verifies inline draft notes reject files outside the MR diff.
func TestCreateUpdate_InvalidPosition(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"new_path":"other.go","old_path":"other.go","diff":"@@ -1 +1 @@\n-old\n+new"}]`)
	}))
	position := &DiffPosition{BaseSHA: "base", StartSHA: "start", HeadSHA: "head", NewPath: "missing.go", NewLine: 1}

	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "create",
			run: func() error {
				_, err := Create(context.Background(), client, CreateInput{ProjectID: "1", MRIID: 1, Note: "inline", Position: position})
				return err
			},
		},
		{
			name: "update",
			run: func() error {
				_, err := Update(context.Background(), client, UpdateInput{ProjectID: "1", MRIID: 1, NoteID: 1, Note: "inline", Position: position})
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			if err == nil {
				t.Fatal("expected invalid position error")
			}
			if !strings.Contains(err.Error(), "missing.go") {
				t.Fatalf("error = %v, want missing file hint", err)
			}
		})
	}
}

func mrDraftNotesActionHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/draft_notes"):
			testutil.RespondJSON(w, http.StatusOK, draftNoteActionListJSON)
		case r.Method == http.MethodGet && strings.Contains(path, "/draft_notes/"):
			testutil.RespondJSON(w, http.StatusOK, draftNoteActionJSON)
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/draft_notes"):
			testutil.RespondJSON(w, http.StatusCreated, draftNoteActionJSON)
		case r.Method == http.MethodPut && strings.HasSuffix(path, "/publish"):
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPut && strings.Contains(path, "/draft_notes/"):
			testutil.RespondJSON(w, http.StatusOK, draftNoteActionJSON)
		case r.Method == http.MethodDelete && strings.Contains(path, "/draft_notes/"):
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/bulk_publish"):
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
}

func mrDraftNoteSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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

// TestActionSpecs_PublishAll_ReviewerStateIsAnEnumOfGitLabsTwoValues verifies
// that the bulk publish action publishes reviewer_state as the two values
// GitLab's route declares, and that no other draft note action carries a
// schema override, since none of them takes the parameter.
func TestActionSpecs_PublishAll_ReviewerStateIsAnEnumOfGitLabsTwoValues(t *testing.T) {
	byTool := mrDraftNoteSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.NewServeMux())))
	want := []toolutil.InputSchemaOverride{
		{PropertyPath: "reviewer_state", Values: map[string]any{"enum": []any{"requested_changes", "reviewed"}}},
	}
	for tool, spec := range byTool {
		t.Run(tool, func(t *testing.T) {
			if tool == "gitlab_mr_draft_note_publish_all" {
				if !reflect.DeepEqual(spec.InputSchemaOverrides, want) {
					t.Errorf("input schema overrides =\n got %#v\nwant %#v", spec.InputSchemaOverrides, want)
				}
				return
			}
			if len(spec.InputSchemaOverrides) != 0 {
				t.Errorf("input schema overrides = %#v, want none on an action that takes no reviewer_state", spec.InputSchemaOverrides)
			}
		})
	}
}

// TestActionSpecs_PublishAll_RouteSendsTheReviewOptions verifies that the
// canonical route decodes note, internal and reviewer_state from the caller's
// arguments and that each reaches the bulk_publish body with its value, so the
// schema a model reads and the request GitLab receives name the same three.
func TestActionSpecs_PublishAll_RouteSendsTheReviewOptions(t *testing.T) {
	var body []byte
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/bulk_publish") {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		read, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			http.Error(w, "read body", http.StatusInternalServerError)
			return
		}
		body = read
		w.WriteHeader(http.StatusNoContent)
	}))
	byTool := mrDraftNoteSpecsByTool(t, ActionSpecs(client))

	if _, err := byTool["gitlab_mr_draft_note_publish_all"].Route.Handler(t.Context(), map[string]any{
		"project_id": "42", "merge_request_iid": 1,
		"note": "Summary", "internal": true, "reviewer_state": "requested_changes",
	}); err != nil {
		t.Fatalf("Route.Handler(gitlab_mr_draft_note_publish_all) error: %v", err)
	}
	var sent map[string]any
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatalf("decode request body %q: %v", body, err)
	}
	want := map[string]any{"note": "Summary", "internal": true, "reviewer_state": "requested_changes"}
	if !reflect.DeepEqual(sent, want) {
		t.Errorf("bulk_publish body = %#v, want %#v", sent, want)
	}
}

// TestDecorateDraftNoteMeta_UnknownTool verifies the metadata decorator leaves
// options unchanged for a tool name that is not in the metadata map.
func TestDecorateDraftNoteMeta_UnknownTool(t *testing.T) {
	options := mrDraftNoteOptions("gitlab_unknown_tool")
	before := options.Usage
	toolutil.ApplyActionMeta(&options, draftNoteActionMeta["gitlab_unknown_tool"])
	if options.Usage != before {
		t.Errorf("expected usage unchanged for unknown tool, got %q", options.Usage)
	}
	if options.IndividualTool.Description != "" {
		t.Errorf("expected empty description for unknown tool, got %q", options.IndividualTool.Description)
	}
}
