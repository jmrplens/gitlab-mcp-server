// metatool_test.go contains unit tests for the meta-tool dispatch mechanism
// including action validation, parameter unmarshalling, and the wrapAction
// and wrapVoidAction generic helpers.
package tools

import (
	"context"
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/uploads"
)

const (
	// fmtUnexpectedErr is the format string used when an unexpected error occurs.
	fmtUnexpectedErr = "unexpected error: %v"
)

// TestMakeMetaHandler_EmptyAction verifies that makeMetaHandler returns a
// descriptive error when the action field is empty.
func TestMakeMetaHandler_EmptyAction(t *testing.T) {
	routes := map[string]actionFunc{
		"create": func(_ context.Context, _ map[string]any) (any, error) {
			return "created", nil
		},
	}

	handler := makeMetaHandler("test_tool", routes)
	input := MetaToolInput{Action: ""}

	_, _, err := handler(context.Background(), nil, input)
	if err == nil {
		t.Fatal("expected error for empty action")
	}
	if got := err.Error(); got != "test_tool: 'action' is required. Valid actions: create" {
		t.Errorf("unexpected error: %s", got)
	}
}

// TestMakeMetaHandler_UnknownAction verifies that makeMetaHandler returns a
// descriptive error listing valid actions when an unknown action is provided.
func TestMakeMetaHandler_UnknownAction(t *testing.T) {
	routes := map[string]actionFunc{
		"create": func(_ context.Context, _ map[string]any) (any, error) {
			return "created", nil
		},
		"list": func(_ context.Context, _ map[string]any) (any, error) {
			return "listed", nil
		},
	}

	handler := makeMetaHandler("test_tool", routes)
	input := MetaToolInput{Action: "destroy"}

	_, _, err := handler(context.Background(), nil, input)
	if err == nil {
		t.Fatal("expected error for unknown action")
	}
	if got := err.Error(); got != `test_tool: unknown action "destroy". Valid actions: create, list` {
		t.Errorf("unexpected error: %s", got)
	}
}

// TestMakeMetaHandler_ValidAction verifies that makeMetaHandler dispatches
// to the correct handler and returns its result.
func TestMakeMetaHandler_ValidAction(t *testing.T) {
	routes := map[string]actionFunc{
		"get": func(_ context.Context, params map[string]any) (any, error) {
			return params["id"], nil
		},
	}

	handler := makeMetaHandler("test_tool", routes)
	input := MetaToolInput{
		Action: "get",
		Params: map[string]any{"id": "42"},
	}

	_, result, err := handler(context.Background(), nil, input)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if result != "42" {
		t.Errorf("expected 42, got %v", result)
	}
}

// TestUnmarshalParams_ValidStruct verifies that unmarshalParams correctly
// deserializes a params map into the target struct type.
func TestUnmarshalParams_ValidStruct(t *testing.T) {
	params := map[string]any{
		"project_id": "group/repo",
	}

	input, err := unmarshalParams[projects.GetInput](params)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if input.ProjectID != "group/repo" {
		t.Errorf("expected group/repo, got %s", input.ProjectID)
	}
}

// TestUnmarshalParams_NilParams verifies that nil params unmarshal to a
// zero-value struct without error.
func TestUnmarshalParams_NilParams(t *testing.T) {
	_, err := unmarshalParams[projects.GetInput](nil)
	if err != nil {
		t.Fatalf("nil params should unmarshal to zero-value struct, got error: %v", err)
	}
}

// TestWrapAction_Integration verifies that wrapAction properly deserializes
// params and calls the underlying typed handler against a mock GitLab API.
func TestWrapAction_Integration(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		respondJSON(w, http.StatusOK, `{
			"id": 42,
			"name": "test-project",
			"path_with_namespace": "group/test-project",
			"visibility": "private",
			"default_branch": "main",
			"web_url": "https://gitlab.example.com/group/test-project",
			"description": "A test project"
		}`)
	})

	client := newTestClient(t, mux)
	action := wrapAction(client, projects.Get)

	result, err := action(context.Background(), map[string]any{
		"project_id": "42",
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	out, ok := result.(projects.Output)
	if !ok {
		t.Fatalf("expected projects.Output, got %T", result)
	}
	if out.Name != "test-project" {
		t.Errorf("expected test-project, got %s", out.Name)
	}
}

// TestWrapVoidAction_Integration verifies that wrapVoidAction properly
// deserializes params and returns nil result for void handlers.
func TestWrapVoidAction_Integration(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/uploads/5", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	client := newTestClient(t, mux)
	action := wrapVoidAction(client, uploads.Delete)

	result, err := action(context.Background(), map[string]any{
		"project_id": "42",
		"upload_id":  float64(5),
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if result != nil {
		t.Errorf("expected nil result for void action, got %v", result)
	}
}

// TestValidActions_StringSorted verifies that validActionsString returns
// action names in alphabetical order, separated by commas.
func TestValidActions_StringSorted(t *testing.T) {
	routes := map[string]actionFunc{
		"delete": nil,
		"create": nil,
		"list":   nil,
	}

	got := validActionsString(routes)
	expected := "create, delete, list"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}
