// action_specs_test.go contains catalog-surface and route tests for behavior that
// used to live in register.go: destructive confirmation, not-found formatting,
// and route error propagation.
package tags

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_SurfaceConfirmDeclined covers generic destructive
// confirmation for tag delete and unprotect when the user declines.
func TestActionSpecs_SurfaceConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	for _, spec := range ActionSpecs(client) {
		if spec.IndividualTool.Name == "gitlab_tag_delete" || spec.IndividualTool.Name == "gitlab_tag_unprotect" {
			toolutil.RegisterSurfaceToolFromSpec(server, spec, toolutil.SurfaceToolRegisterOptions{Description: "Test tag destructive confirmation.", Icons: toolutil.IconTag})
		}
	}

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
	session, connectErr := mcpClient.Connect(ctx, ct, nil)
	if connectErr != nil {
		t.Fatalf("client connect: %v", connectErr)
	}
	t.Cleanup(func() {
		session.Close()
		_ = serverSession.Wait()
	})

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_tag_delete", map[string]any{"project_id": "42", "tag_name": "v1"}},
		{"gitlab_tag_unprotect", map[string]any{"project_id": "42", "tag_name": "v*"}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, callErr := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if callErr != nil {
				t.Fatalf("CallTool error: %v", callErr)
			}
			if result == nil {
				t.Fatal("expected non-nil result for declined confirmation")
			}
		})
	}
}

// TestActionSpecs_GetNotFound covers the canonical gitlab_tag_get 404 output.
func TestActionSpecs_GetNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Tag Not Found"}`)
	})
	client := testutil.NewTestClient(t, mux)
	byTool := tagSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_tag_get"].Route.Handler(t.Context(), map[string]any{"project_id": "42", "tag_name": "nonexistent"})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	callResult := toolutil.MarkdownForResult(result)
	if callResult == nil || !callResult.IsError {
		t.Fatalf("MarkdownForResult() = %#v, want IsError result for 404", callResult)
	}
}

// TestTagCreate_AlreadyExists covers the "already exists" hint branch in Create.
func TestTagCreate_AlreadyExists(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusConflict, `{"message":"Tag already exists"}`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := Create(context.Background(), client, CreateInput{ProjectID: "1", TagName: "v1", Ref: "main"})
	if err == nil {
		t.Fatal("expected error for already-existing tag")
	}
}

// TestTagCreate_InvalidRef_Register covers the "is invalid" hint branch in Create
// via the register handler path.
func TestTagCreate_InvalidRef_Register(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"Target is invalid"}`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := Create(context.Background(), client, CreateInput{ProjectID: "1", TagName: "v1", Ref: "bad"})
	if err == nil {
		t.Fatal("expected error for invalid ref")
	}
}

// TestTagList_PagePerPage covers the page/per_page optional branches in List.
func TestTagList_PagePerPage(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := List(context.Background(), client, ListInput{ProjectID: "1", PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 10}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestProtectTag_Conflict covers the 409 Conflict hint branch in ProtectTag.
func TestProtectTag_Conflict(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusConflict, `{"message":"already exists"}`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := ProtectTag(context.Background(), client, ProtectTagInput{ProjectID: "1", TagName: "v*"})
	if err == nil {
		t.Fatal("expected error for conflict")
	}
}

// TestActionSpecs_ErrorPaths covers route error propagation for non-destructive
// tag routes against a failing server.
func TestActionSpecs_ErrorPaths(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})
	client := testutil.NewTestClient(t, mux)
	byTool := tagSpecsByTool(t, ActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_tag_list", map[string]any{"project_id": "1"}},
		{"gitlab_tag_get", map[string]any{"project_id": "1", "tag_name": "v1"}},
		{"gitlab_tag_create", map[string]any{"project_id": "1", "tag_name": "v1", "ref": "main"}},
		{"gitlab_tag_get_signature", map[string]any{"project_id": "1", "tag_name": "v1"}},
		{"gitlab_tag_list_protected", map[string]any{"project_id": "1"}},
		{"gitlab_tag_get_protected", map[string]any{"project_id": "1", "tag_name": "v*"}},
		{"gitlab_tag_protect", map[string]any{"project_id": "1", "tag_name": "v*"}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			_, err := byTool[tt.name].Route.Handler(t.Context(), tt.args)
			if err == nil {
				t.Errorf("Route.Handler(%s) error = nil, want error", tt.name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// tagSpec — branch coverage
// ---------------------------------------------------------------------------

// TestTagSpec_IdempotentNonDestructive covers the case where a spec is
// idempotent=true but route.Destructive=false: this branch (the `case
// idempotent:` arm in tagSpec's second switch) is not exercised by any
// production call site, so it is verified directly.
func TestTagSpec_IdempotentNonDestructive(t *testing.T) {
	route := toolutil.ActionRoute{
		Handler: func(_ context.Context, _ map[string]any) (any, error) {
			return nil, nil //nolint:nilnil // test fixture: handler is never invoked
		},
		Destructive: false,
	}
	spec := tagSpec("custom", route, "gitlab_tag_custom", false, true)
	if spec.Name != "custom" {
		t.Errorf("expected spec.Name = custom, got %q", spec.Name)
	}
	// The idempotent + non-destructive + non-readonly path produces an
	// update-style spec (not destructive, not create, not read).
	if spec.ReadOnly {
		t.Errorf("expected non-readonly spec")
	}
	if spec.Destructive {
		t.Errorf("expected non-destructive spec for idempotent non-destructive route")
	}
}
