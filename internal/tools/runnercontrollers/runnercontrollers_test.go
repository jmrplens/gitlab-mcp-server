// runnercontrollers_test.go contains unit tests for the runner controller MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.

package runnercontrollers

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	sampleControllerJSON = `{"id":1,"description":"ctrl-1","state":"enabled","created_at":"2026-01-15T10:00:00Z","updated_at":"2026-01-15T12:00:00Z"}`
	sampleDetailsJSON    = `{"id":1,"description":"ctrl-1","state":"enabled","connected":true,"created_at":"2026-01-15T10:00:00Z","updated_at":"2026-01-15T12:00:00Z"}`
	errUnexpected        = "unexpected error: %v"
	errExpValidation     = "expected validation error, got nil"
	errExpAPIErr         = "expected API error, got nil"
	errExpCtxCancel      = "expected context error, got nil"
	msgNotFound          = `{"message":"404 Not Found"}`
)

func nopHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {})
}

// TestList_Success verifies that List returns controllers with pagination.
func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[`+sampleControllerJSON+`]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, msgNotFound)
	}))

	out, err := List(context.Background(), client, ListInput{})
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
	if len(out.Controllers) != 1 {
		t.Fatalf("expected 1 controller, got %d", len(out.Controllers))
	}
	if out.Controllers[0].ID != 1 || out.Controllers[0].Description != "ctrl-1" {
		t.Errorf("controller mismatch: %+v", out.Controllers[0])
	}
	if out.Controllers[0].State != "enabled" {
		t.Errorf("state = %q, want enabled", out.Controllers[0].State)
	}
}

// TestList_WithPagination verifies that List passes pagination parameters.
func TestList_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") != "2" {
			t.Errorf("expected page=2, got %s", r.URL.Query().Get("page"))
		}
		if r.URL.Query().Get("per_page") != "10" {
			t.Errorf("expected per_page=10, got %s", r.URL.Query().Get("per_page"))
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
			testutil.PaginationHeaders{Page: "2", PerPage: "10", Total: "1", TotalPages: "1"})
	}))

	_, err := List(context.Background(), client, ListInput{
		PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 10},
	})
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
}

// TestList_Empty verifies that List handles empty results.
func TestList_Empty(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "0"})
	}))

	out, err := List(context.Background(), client, ListInput{})
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
	if len(out.Controllers) != 0 {
		t.Errorf("expected 0 controllers, got %d", len(out.Controllers))
	}
}

// TestList_APIError verifies that List propagates API errors.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal(errExpAPIErr)
	}
}

// TestList_ContextCancelled verifies that List respects context cancellation.
func TestList_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())
	ctx := testutil.CancelledCtx(t)

	_, err := List(ctx, client, ListInput{})
	if err == nil {
		t.Fatal(errExpCtxCancel)
	}
}

// TestGet_Success verifies that Get returns controller details.
func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, sampleDetailsJSON)
	}))

	out, err := Get(context.Background(), client, GetInput{ControllerID: 1})
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
	if out.ID != 1 || !out.Connected {
		t.Errorf("details mismatch: %+v", out)
	}
	if out.Description != "ctrl-1" {
		t.Errorf("description = %q, want ctrl-1", out.Description)
	}
}

// TestGet_MissingID verifies that Get rejects missing controller_id.
func TestGet_MissingID(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())

	_, err := Get(context.Background(), client, GetInput{})
	if err == nil {
		t.Fatal(errExpValidation)
	}
	if !strings.Contains(err.Error(), "controller_id") {
		t.Errorf("error should mention controller_id: %v", err)
	}
}

// TestGet_APIError verifies that Get propagates API errors.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, msgNotFound)
	}))

	_, err := Get(context.Background(), client, GetInput{ControllerID: 999})
	if err == nil {
		t.Fatal(errExpAPIErr)
	}
}

// TestGet_ContextCancelled verifies that Get respects context cancellation.
func TestGet_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())
	ctx := testutil.CancelledCtx(t)

	_, err := Get(ctx, client, GetInput{ControllerID: 1})
	if err == nil {
		t.Fatal(errExpCtxCancel)
	}
}

// TestCreate_Success verifies that Create returns a new controller.
func TestCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		testutil.RespondJSON(w, http.StatusCreated, sampleControllerJSON)
	}))

	out, err := Create(context.Background(), client, CreateInput{Description: "ctrl-1", State: "enabled"})
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
	if out.ID != 1 || out.Description != "ctrl-1" {
		t.Errorf("output mismatch: %+v", out)
	}
}

// TestCreate_Defaults verifies that Create works with empty optional fields.
func TestCreate_Defaults(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, sampleControllerJSON)
	}))

	out, err := Create(context.Background(), client, CreateInput{})
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
	if out.ID != 1 {
		t.Errorf("expected ID 1, got %d", out.ID)
	}
}

// TestCreate_APIError verifies that Create propagates API errors.
func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))

	_, err := Create(context.Background(), client, CreateInput{Description: "x"})
	if err == nil {
		t.Fatal(errExpAPIErr)
	}
}

// TestCreate_ContextCancelled verifies that Create respects context cancellation.
func TestCreate_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())
	ctx := testutil.CancelledCtx(t)

	_, err := Create(ctx, client, CreateInput{Description: "x"})
	if err == nil {
		t.Fatal(errExpCtxCancel)
	}
}

// TestUpdate_Success verifies that Update returns the updated controller.
func TestUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		testutil.RespondJSON(w, http.StatusOK, sampleControllerJSON)
	}))

	out, err := Update(context.Background(), client, UpdateInput{ControllerID: 1, Description: "ctrl-1", State: "enabled"})
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
	if out.ID != 1 {
		t.Errorf("expected ID 1, got %d", out.ID)
	}
}

// TestUpdate_MissingID verifies that Update rejects missing controller_id.
func TestUpdate_MissingID(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())

	_, err := Update(context.Background(), client, UpdateInput{})
	if err == nil {
		t.Fatal(errExpValidation)
	}
	if !strings.Contains(err.Error(), "controller_id") {
		t.Errorf("error should mention controller_id: %v", err)
	}
}

// TestUpdate_APIError verifies that Update propagates API errors.
func TestUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))

	_, err := Update(context.Background(), client, UpdateInput{ControllerID: 1})
	if err == nil {
		t.Fatal(errExpAPIErr)
	}
}

// TestUpdate_ContextCancelled verifies that Update respects context cancellation.
func TestUpdate_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())
	ctx := testutil.CancelledCtx(t)

	_, err := Update(ctx, client, UpdateInput{ControllerID: 1})
	if err == nil {
		t.Fatal(errExpCtxCancel)
	}
}

// TestDelete_Success verifies that Delete succeeds.
func TestDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	err := Delete(context.Background(), client, DeleteInput{ControllerID: 1})
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
}

// TestDelete_MissingID verifies that Delete rejects missing controller_id.
func TestDelete_MissingID(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())

	err := Delete(context.Background(), client, DeleteInput{})
	if err == nil {
		t.Fatal(errExpValidation)
	}
	if !strings.Contains(err.Error(), "controller_id") {
		t.Errorf("error should mention controller_id: %v", err)
	}
}

// TestDelete_APIError verifies that Delete propagates API errors.
func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))

	err := Delete(context.Background(), client, DeleteInput{ControllerID: 1})
	if err == nil {
		t.Fatal(errExpAPIErr)
	}
}

// TestDelete_ContextCancelled verifies that Delete respects context cancellation.
func TestDelete_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())
	ctx := testutil.CancelledCtx(t)

	err := Delete(ctx, client, DeleteInput{ControllerID: 1})
	if err == nil {
		t.Fatal(errExpCtxCancel)
	}
}

// TestFormatOutputMarkdown verifies Markdown formatting with and without timestamps.
func TestFormatOutputMarkdown(t *testing.T) {
	out := Output{ID: 1, Description: "ctrl-1", State: "enabled",
		CreatedAt: "2026-01-15T10:00:00Z", UpdatedAt: "2026-01-15T12:00:00Z"}

	md := FormatOutputMarkdown(out)
	for _, want := range []string{"ctrl-1", "enabled", "Created At", "Updated At"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q: %s", want, md)
		}
	}

	// Without timestamps
	out.CreatedAt = ""
	out.UpdatedAt = ""
	md = FormatOutputMarkdown(out)
	if strings.Contains(md, "Created At") {
		t.Error("should not contain Created At when empty")
	}
}

// TestFormatDetailsMarkdown verifies detailed Markdown formatting.
func TestFormatDetailsMarkdown(t *testing.T) {
	out := DetailsOutput{
		Output:    Output{ID: 1, Description: "ctrl-1", State: "enabled", CreatedAt: "2026-01-15T10:00:00Z", UpdatedAt: "2026-01-15T12:00:00Z"},
		Connected: true,
	}

	md := FormatDetailsMarkdown(out)
	for _, want := range []string{"ctrl-1", "Details", "true", "Created At"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q: %s", want, md)
		}
	}

	// Without timestamps
	out.CreatedAt = ""
	out.UpdatedAt = ""
	md = FormatDetailsMarkdown(out)
	if strings.Contains(md, "Created At") {
		t.Error("should not contain Created At when empty")
	}
}

// TestFormatListMarkdown verifies list Markdown with data and empty.
func TestFormatListMarkdown(t *testing.T) {
	out := ListOutput{
		Controllers: []Output{
			{ID: 1, Description: "ctrl-1", State: "enabled"},
			{ID: 2, Description: "ctrl-2", State: "disabled"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2},
	}

	md := FormatListMarkdown(out)
	for _, want := range []string{"ctrl-1", "ctrl-2", "enabled", "disabled"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q: %s", want, md)
		}
	}

	// Empty
	md = FormatListMarkdown(ListOutput{})
	if !strings.Contains(md, "No runner controllers found") {
		t.Errorf("expected empty message, got: %s", md)
	}
}

// TestFormatGetMarkdown verifies FormatGetMarkdown returns a non-nil result.
func TestFormatGetMarkdown(t *testing.T) {
	out := DetailsOutput{
		Output:    Output{ID: 1, Description: "ctrl-1", State: "enabled"},
		Connected: true,
	}
	result := FormatGetMarkdown(out)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// RegisterTools — MCP roundtrip.

// TestRegisterTools_CallAllThroughMCP validates that all runner controller tools
// are registered and callable through a full MCP session roundtrip.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newRunnerControllersMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_runner_controller_list", map[string]any{}},
		{"get", "gitlab_runner_controller_get", map[string]any{"controller_id": 1}},
		{"create", "gitlab_runner_controller_create", map[string]any{"description": "new"}},
		{"update", "gitlab_runner_controller_update", map[string]any{"controller_id": 1, "description": "updated"}},
		{"delete", "gitlab_runner_controller_delete", map[string]any{"controller_id": 1}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.tool,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.tool, err)
			}
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(%s) returned error: %s", tt.tool, tc.Text)
					}
				}
				t.Fatalf("CallTool(%s) returned IsError=true", tt.tool)
			}
		})
	}
}

// newRunnerControllersMCPSession creates an MCP session with runner controller tools.
func newRunnerControllersMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/runner_controllers", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+sampleControllerJSON+`]`)
	})
	handler.HandleFunc("GET /api/v4/runner_controllers/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, sampleDetailsJSON)
	})
	handler.HandleFunc("POST /api/v4/runner_controllers", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, sampleControllerJSON)
	})
	handler.HandleFunc("PUT /api/v4/runner_controllers/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, sampleControllerJSON)
	})
	handler.HandleFunc("DELETE /api/v4/runner_controllers/1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

// TestRegisterTools_DeleteConfirmDeclined covers the ConfirmAction early-return
// branch in the runner controller delete handler when the user declines.
func TestRegisterTools_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
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
	t.Cleanup(func() { session.Close() })

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_runner_controller_delete",
		Arguments: map[string]any{"controller_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

// TestRegisterTools_DeleteAPIError covers the error path in the delete handler
// after ConfirmAction succeeds.
func TestRegisterTools_DeleteAPIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})
	client := testutil.NewTestClient(t, mux)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_runner_controller_delete",
		Arguments: map[string]any{"controller_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for API failure")
	}
}

// TestRegisterMeta_NoPanic verifies that RegisterMeta does not panic.
func TestRegisterMeta_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)
}
