// runnercontrollertokens_test.go contains unit tests for the runner controller token MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package runnercontrollertokens

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
	sampleTokenJSON = `{"id":10,"runner_controller_id":1,"description":"my-token","token":"glrt-abc123","last_used_at":"2026-01-15T10:00:00Z","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-15T10:00:00Z"}`
	errUnexpected   = "unexpected error: %v"
	errExpValid     = "expected validation error, got nil"
	errExpAPIErr    = "expected API error, got nil"
	errExpCtxCancel = "expected context error, got nil"
)

func nopHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {})
}

// TestList_Success verifies that List returns tokens with pagination.
func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[`+sampleTokenJSON+`]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	}))

	out, err := List(context.Background(), client, ListInput{ControllerID: 1})
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
	if len(out.Tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(out.Tokens))
	}
	if out.Tokens[0].ID != 10 || out.Tokens[0].Token != "glrt-abc123" {
		t.Errorf("token mismatch: %+v", out.Tokens[0])
	}
}

// TestList_WithPagination verifies List passes pagination parameters.
func TestList_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") != "2" {
			t.Errorf("expected page=2, got %s", r.URL.Query().Get("page"))
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
			testutil.PaginationHeaders{Page: "2", PerPage: "10", Total: "0", TotalPages: "0"})
	}))

	_, err := List(context.Background(), client, ListInput{
		ControllerID:    1,
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

	out, err := List(context.Background(), client, ListInput{ControllerID: 1})
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
	if len(out.Tokens) != 0 {
		t.Errorf("expected 0 tokens, got %d", len(out.Tokens))
	}
}

// TestList_MissingControllerID verifies that List rejects missing controller_id.
func TestList_MissingControllerID(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal(errExpValid)
	}
	if !strings.Contains(err.Error(), "controller_id") {
		t.Errorf("error should mention controller_id: %v", err)
	}
}

// TestList_APIError verifies that List propagates API errors.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))

	_, err := List(context.Background(), client, ListInput{ControllerID: 1})
	if err == nil {
		t.Fatal(errExpAPIErr)
	}
}

// TestList_ContextCancelled verifies that List respects context cancellation.
func TestList_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := List(ctx, client, ListInput{ControllerID: 1})
	if err == nil {
		t.Fatal(errExpCtxCancel)
	}
}

// TestGet_Success verifies that Get returns token details.
func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, sampleTokenJSON)
	}))

	out, err := Get(context.Background(), client, GetInput{ControllerID: 1, TokenID: 10})
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
	if out.ID != 10 || out.Token != "glrt-abc123" {
		t.Errorf("token mismatch: %+v", out)
	}
	if out.RunnerControllerID != 1 {
		t.Errorf("controller ID = %d, want 1", out.RunnerControllerID)
	}
}

// TestGet_MissingControllerID verifies that Get rejects missing controller_id.
func TestGet_MissingControllerID(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())

	_, err := Get(context.Background(), client, GetInput{TokenID: 10})
	if err == nil {
		t.Fatal(errExpValid)
	}
	if !strings.Contains(err.Error(), "controller_id") {
		t.Errorf("error should mention controller_id: %v", err)
	}
}

// TestGet_MissingTokenID verifies that Get rejects missing token_id.
func TestGet_MissingTokenID(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())

	_, err := Get(context.Background(), client, GetInput{ControllerID: 1})
	if err == nil {
		t.Fatal(errExpValid)
	}
	if !strings.Contains(err.Error(), "token_id") {
		t.Errorf("error should mention token_id: %v", err)
	}
}

// TestGet_APIError verifies that Get propagates API errors.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := Get(context.Background(), client, GetInput{ControllerID: 1, TokenID: 999})
	if err == nil {
		t.Fatal(errExpAPIErr)
	}
}

// TestGet_ContextCancelled verifies that Get respects context cancellation.
func TestGet_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Get(ctx, client, GetInput{ControllerID: 1, TokenID: 10})
	if err == nil {
		t.Fatal(errExpCtxCancel)
	}
}

// TestCreate_Success verifies that Create returns the new token.
func TestCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		testutil.RespondJSON(w, http.StatusCreated, sampleTokenJSON)
	}))

	out, err := Create(context.Background(), client, CreateInput{ControllerID: 1, Description: "my-token"})
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
	if out.ID != 10 || out.Description != "my-token" {
		t.Errorf("output mismatch: %+v", out)
	}
}

// TestCreate_DefaultDescription verifies Create with empty description.
func TestCreate_DefaultDescription(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, sampleTokenJSON)
	}))

	out, err := Create(context.Background(), client, CreateInput{ControllerID: 1})
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
	if out.ID != 10 {
		t.Errorf("expected ID 10, got %d", out.ID)
	}
}

// TestCreate_MissingControllerID verifies Create rejects missing controller_id.
func TestCreate_MissingControllerID(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())

	_, err := Create(context.Background(), client, CreateInput{})
	if err == nil {
		t.Fatal(errExpValid)
	}
	if !strings.Contains(err.Error(), "controller_id") {
		t.Errorf("error should mention controller_id: %v", err)
	}
}

// TestCreate_APIError verifies that Create propagates API errors.
func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))

	_, err := Create(context.Background(), client, CreateInput{ControllerID: 1})
	if err == nil {
		t.Fatal(errExpAPIErr)
	}
}

// TestCreate_ContextCancelled verifies that Create respects context cancellation.
func TestCreate_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Create(ctx, client, CreateInput{ControllerID: 1})
	if err == nil {
		t.Fatal(errExpCtxCancel)
	}
}

// TestRotate_Success verifies that Rotate returns the rotated token.
func TestRotate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, sampleTokenJSON)
	}))

	out, err := Rotate(context.Background(), client, RotateInput{ControllerID: 1, TokenID: 10})
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
	if out.ID != 10 {
		t.Errorf("expected ID 10, got %d", out.ID)
	}
}

// TestRotate_MissingControllerID verifies Rotate rejects missing controller_id.
func TestRotate_MissingControllerID(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())

	_, err := Rotate(context.Background(), client, RotateInput{TokenID: 10})
	if err == nil {
		t.Fatal(errExpValid)
	}
	if !strings.Contains(err.Error(), "controller_id") {
		t.Errorf("error should mention controller_id: %v", err)
	}
}

// TestRotate_MissingTokenID verifies Rotate rejects missing token_id.
func TestRotate_MissingTokenID(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())

	_, err := Rotate(context.Background(), client, RotateInput{ControllerID: 1})
	if err == nil {
		t.Fatal(errExpValid)
	}
	if !strings.Contains(err.Error(), "token_id") {
		t.Errorf("error should mention token_id: %v", err)
	}
}

// TestRotate_APIError verifies that Rotate propagates API errors.
func TestRotate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))

	_, err := Rotate(context.Background(), client, RotateInput{ControllerID: 1, TokenID: 10})
	if err == nil {
		t.Fatal(errExpAPIErr)
	}
}

// TestRotate_ContextCancelled verifies that Rotate respects context cancellation.
func TestRotate_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Rotate(ctx, client, RotateInput{ControllerID: 1, TokenID: 10})
	if err == nil {
		t.Fatal(errExpCtxCancel)
	}
}

// TestRevoke_Success verifies that Revoke succeeds.
func TestRevoke_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	err := Revoke(context.Background(), client, RevokeInput{ControllerID: 1, TokenID: 10})
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
}

// TestRevoke_MissingControllerID verifies Revoke rejects missing controller_id.
func TestRevoke_MissingControllerID(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())

	err := Revoke(context.Background(), client, RevokeInput{TokenID: 10})
	if err == nil {
		t.Fatal(errExpValid)
	}
	if !strings.Contains(err.Error(), "controller_id") {
		t.Errorf("error should mention controller_id: %v", err)
	}
}

// TestRevoke_MissingTokenID verifies Revoke rejects missing token_id.
func TestRevoke_MissingTokenID(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())

	err := Revoke(context.Background(), client, RevokeInput{ControllerID: 1})
	if err == nil {
		t.Fatal(errExpValid)
	}
	if !strings.Contains(err.Error(), "token_id") {
		t.Errorf("error should mention token_id: %v", err)
	}
}

// TestRevoke_APIError verifies that Revoke propagates API errors.
func TestRevoke_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))

	err := Revoke(context.Background(), client, RevokeInput{ControllerID: 1, TokenID: 10})
	if err == nil {
		t.Fatal(errExpAPIErr)
	}
}

// TestRevoke_ContextCancelled verifies that Revoke respects context cancellation.
func TestRevoke_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := Revoke(ctx, client, RevokeInput{ControllerID: 1, TokenID: 10})
	if err == nil {
		t.Fatal(errExpCtxCancel)
	}
}

// TestFormatOutputMarkdown verifies Markdown with and without optional fields.
func TestFormatOutputMarkdown(t *testing.T) {
	out := Output{
		ID: 10, RunnerControllerID: 1, Description: "my-token",
		Token: "glrt-abc123", LastUsedAt: "2026-01-15T10:00:00Z",
		CreatedAt: "2026-01-01T00:00:00Z",
	}

	md := FormatOutputMarkdown(out)
	for _, want := range []string{"my-token", "glrt-abc123", "Last Used At", "Created At"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q: %s", want, md)
		}
	}

	// Without token and timestamps
	out.Token = ""
	out.LastUsedAt = ""
	out.CreatedAt = ""
	md = FormatOutputMarkdown(out)
	if strings.Contains(md, "glrt-abc123") {
		t.Error("should not contain token when empty")
	}
	if strings.Contains(md, "Last Used At") {
		t.Error("should not contain Last Used At when empty")
	}
}

// TestFormatListMarkdown verifies list Markdown with data and empty.
func TestFormatListMarkdown(t *testing.T) {
	out := ListOutput{
		Tokens: []Output{
			{ID: 10, RunnerControllerID: 1, Description: "tok-1"},
			{ID: 11, RunnerControllerID: 1, Description: "tok-2"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2},
	}

	md := FormatListMarkdown(out)
	for _, want := range []string{"tok-1", "tok-2"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q: %s", want, md)
		}
	}

	// Empty
	md = FormatListMarkdown(ListOutput{})
	if !strings.Contains(md, "No runner controller tokens found") {
		t.Errorf("expected empty message, got: %s", md)
	}
}

// TestFormatGetMarkdown verifies FormatGetMarkdown returns a non-nil result.
func TestFormatGetMarkdown(t *testing.T) {
	out := Output{ID: 10, RunnerControllerID: 1, Description: "my-token"}
	result := FormatGetMarkdown(out)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// RegisterTools — MCP roundtrip.

// TestRegisterTools_CallAllThroughMCP validates that all runner controller token
// tools are registered and callable through a full MCP session roundtrip.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newTokensMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_runner_controller_token_list", map[string]any{"controller_id": 1}},
		{"get", "gitlab_runner_controller_token_get", map[string]any{"controller_id": 1, "token_id": 10}},
		{"create", "gitlab_runner_controller_token_create", map[string]any{"controller_id": 1, "description": "new"}},
		{"rotate", "gitlab_runner_controller_token_rotate", map[string]any{"controller_id": 1, "token_id": 10}},
		{"revoke", "gitlab_runner_controller_token_revoke", map[string]any{"controller_id": 1, "token_id": 10}},
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

// newTokensMCPSession creates an MCP session with runner controller token tools.
func newTokensMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/runner_controllers/1/tokens", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+sampleTokenJSON+`]`)
	})
	handler.HandleFunc("GET /api/v4/runner_controllers/1/tokens/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, sampleTokenJSON)
	})
	handler.HandleFunc("POST /api/v4/runner_controllers/1/tokens", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, sampleTokenJSON)
	})
	handler.HandleFunc("POST /api/v4/runner_controllers/1/tokens/10/rotate", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, sampleTokenJSON)
	})
	handler.HandleFunc("DELETE /api/v4/runner_controllers/1/tokens/10", func(w http.ResponseWriter, _ *http.Request) {
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

// TestRegisterTools_RevokeConfirmDeclined covers the ConfirmAction early-return
// branch in the runner controller token revoke handler when the user declines.
func TestRegisterTools_RevokeConfirmDeclined(t *testing.T) {
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
		Name:      "gitlab_runner_controller_token_revoke",
		Arguments: map[string]any{"controller_id": 1, "token_id": 10},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

// TestRegisterTools_RevokeAPIError covers the error path in the revoke handler
// after ConfirmAction succeeds.
func TestRegisterTools_RevokeAPIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"server error"}`)
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
		Name:      "gitlab_runner_controller_token_revoke",
		Arguments: map[string]any{"controller_id": 1, "token_id": 10},
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
