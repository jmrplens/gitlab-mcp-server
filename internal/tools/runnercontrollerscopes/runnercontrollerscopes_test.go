// runnercontrollerscopes_test.go contains unit tests for the runner controller scope MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package runnercontrollerscopes

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const (
	sampleScopesJSON        = `{"instance_level_scopings":[{"created_at":"2026-01-15T10:00:00Z","updated_at":"2026-01-15T12:00:00Z"}],"runner_level_scopings":[{"runner_id":42,"created_at":"2026-01-15T10:00:00Z","updated_at":"2026-01-15T12:00:00Z"}]}`
	sampleInstanceScopeJSON = `{"created_at":"2026-01-15T10:00:00Z","updated_at":"2026-01-15T12:00:00Z"}`
	sampleRunnerScopeJSON   = `{"runner_id":42,"created_at":"2026-01-15T10:00:00Z","updated_at":"2026-01-15T12:00:00Z"}`
	errUnexpected           = "unexpected error: %v"
	errExpValid             = "expected validation error, got nil"
	errExpAPIErr            = "expected API error, got nil"
	errExpCtxCancel         = "expected context error, got nil"
)

func nopHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {})
}

// TestList_Success verifies that List returns scopes for a controller.
func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, sampleScopesJSON)
	}))

	out, err := List(context.Background(), client, ListInput{ControllerID: 1})
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
	if len(out.InstanceLevelScopings) != 1 {
		t.Errorf("expected 1 instance scope, got %d", len(out.InstanceLevelScopings))
	}
	if len(out.RunnerLevelScopings) != 1 {
		t.Errorf("expected 1 runner scope, got %d", len(out.RunnerLevelScopings))
	}
	if out.RunnerLevelScopings[0].RunnerID != 42 {
		t.Errorf("runner_id = %d, want 42", out.RunnerLevelScopings[0].RunnerID)
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

// TestAddInstanceScope_Success verifies successful instance scope addition.
func TestAddInstanceScope_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, sampleInstanceScopeJSON)
	}))

	out, err := AddInstanceScope(context.Background(), client, AddInstanceScopeInput{ControllerID: 1})
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
	if out.CreatedAt == "" {
		t.Error("expected non-empty CreatedAt")
	}
}

// TestAddInstanceScope_MissingControllerID verifies rejection of missing controller_id.
func TestAddInstanceScope_MissingControllerID(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())

	_, err := AddInstanceScope(context.Background(), client, AddInstanceScopeInput{})
	if err == nil {
		t.Fatal(errExpValid)
	}
	if !strings.Contains(err.Error(), "controller_id") {
		t.Errorf("error should mention controller_id: %v", err)
	}
}

// TestAddInstanceScope_APIError verifies API error propagation.
func TestAddInstanceScope_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))

	_, err := AddInstanceScope(context.Background(), client, AddInstanceScopeInput{ControllerID: 1})
	if err == nil {
		t.Fatal(errExpAPIErr)
	}
}

// TestAddInstanceScope_ContextCancelled verifies context cancellation.
func TestAddInstanceScope_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := AddInstanceScope(ctx, client, AddInstanceScopeInput{ControllerID: 1})
	if err == nil {
		t.Fatal(errExpCtxCancel)
	}
}

// TestRemoveInstanceScope_Success verifies successful instance scope removal.
func TestRemoveInstanceScope_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	err := RemoveInstanceScope(context.Background(), client, RemoveInstanceScopeInput{ControllerID: 1})
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
}

// TestRemoveInstanceScope_MissingControllerID verifies rejection of missing controller_id.
func TestRemoveInstanceScope_MissingControllerID(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())

	err := RemoveInstanceScope(context.Background(), client, RemoveInstanceScopeInput{})
	if err == nil {
		t.Fatal(errExpValid)
	}
	if !strings.Contains(err.Error(), "controller_id") {
		t.Errorf("error should mention controller_id: %v", err)
	}
}

// TestRemoveInstanceScope_APIError verifies API error propagation.
func TestRemoveInstanceScope_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))

	err := RemoveInstanceScope(context.Background(), client, RemoveInstanceScopeInput{ControllerID: 1})
	if err == nil {
		t.Fatal(errExpAPIErr)
	}
}

// TestRemoveInstanceScope_ContextCancelled verifies context cancellation.
func TestRemoveInstanceScope_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := RemoveInstanceScope(ctx, client, RemoveInstanceScopeInput{ControllerID: 1})
	if err == nil {
		t.Fatal(errExpCtxCancel)
	}
}

// TestAddRunnerScope_Success verifies successful runner scope addition.
func TestAddRunnerScope_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, sampleRunnerScopeJSON)
	}))

	out, err := AddRunnerScope(context.Background(), client, AddRunnerScopeInput{ControllerID: 1, RunnerID: 42})
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
	if out.RunnerID != 42 {
		t.Errorf("runner_id = %d, want 42", out.RunnerID)
	}
}

// TestAddRunnerScope_MissingControllerID verifies rejection of missing controller_id.
func TestAddRunnerScope_MissingControllerID(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())

	_, err := AddRunnerScope(context.Background(), client, AddRunnerScopeInput{RunnerID: 42})
	if err == nil {
		t.Fatal(errExpValid)
	}
	if !strings.Contains(err.Error(), "controller_id") {
		t.Errorf("error should mention controller_id: %v", err)
	}
}

// TestAddRunnerScope_MissingRunnerID verifies rejection of missing runner_id.
func TestAddRunnerScope_MissingRunnerID(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())

	_, err := AddRunnerScope(context.Background(), client, AddRunnerScopeInput{ControllerID: 1})
	if err == nil {
		t.Fatal(errExpValid)
	}
	if !strings.Contains(err.Error(), "runner_id") {
		t.Errorf("error should mention runner_id: %v", err)
	}
}

// TestAddRunnerScope_APIError verifies API error propagation.
func TestAddRunnerScope_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))

	_, err := AddRunnerScope(context.Background(), client, AddRunnerScopeInput{ControllerID: 1, RunnerID: 42})
	if err == nil {
		t.Fatal(errExpAPIErr)
	}
}

// TestAddRunnerScope_ContextCancelled verifies context cancellation.
func TestAddRunnerScope_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := AddRunnerScope(ctx, client, AddRunnerScopeInput{ControllerID: 1, RunnerID: 42})
	if err == nil {
		t.Fatal(errExpCtxCancel)
	}
}

// TestRemoveRunnerScope_Success verifies successful runner scope removal.
func TestRemoveRunnerScope_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	err := RemoveRunnerScope(context.Background(), client, RemoveRunnerScopeInput{ControllerID: 1, RunnerID: 42})
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
}

// TestRemoveRunnerScope_MissingControllerID verifies rejection of missing controller_id.
func TestRemoveRunnerScope_MissingControllerID(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())

	err := RemoveRunnerScope(context.Background(), client, RemoveRunnerScopeInput{RunnerID: 42})
	if err == nil {
		t.Fatal(errExpValid)
	}
	if !strings.Contains(err.Error(), "controller_id") {
		t.Errorf("error should mention controller_id: %v", err)
	}
}

// TestRemoveRunnerScope_MissingRunnerID verifies rejection of missing runner_id.
func TestRemoveRunnerScope_MissingRunnerID(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())

	err := RemoveRunnerScope(context.Background(), client, RemoveRunnerScopeInput{ControllerID: 1})
	if err == nil {
		t.Fatal(errExpValid)
	}
	if !strings.Contains(err.Error(), "runner_id") {
		t.Errorf("error should mention runner_id: %v", err)
	}
}

// TestRemoveRunnerScope_APIError verifies API error propagation.
func TestRemoveRunnerScope_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))

	err := RemoveRunnerScope(context.Background(), client, RemoveRunnerScopeInput{ControllerID: 1, RunnerID: 42})
	if err == nil {
		t.Fatal(errExpAPIErr)
	}
}

// TestRemoveRunnerScope_ContextCancelled verifies context cancellation.
func TestRemoveRunnerScope_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := RemoveRunnerScope(ctx, client, RemoveRunnerScopeInput{ControllerID: 1, RunnerID: 42})
	if err == nil {
		t.Fatal(errExpCtxCancel)
	}
}

// TestFormatScopesMarkdown verifies Markdown for scopes with various combinations.
func TestFormatScopesMarkdown(t *testing.T) {
	// Both instance and runner scopes
	out := ScopesOutput{
		InstanceLevelScopings: []InstanceScopeItem{
			{CreatedAt: "2026-01-15T10:00:00Z", UpdatedAt: "2026-01-15T12:00:00Z"},
		},
		RunnerLevelScopings: []RunnerScopeItem{
			{RunnerID: 42, CreatedAt: "2026-01-15T10:00:00Z", UpdatedAt: "2026-01-15T12:00:00Z"},
		},
	}

	md := FormatScopesMarkdown(out)
	for _, want := range []string{"Instance-Level", "Runner-Level", "42"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q: %s", want, md)
		}
	}

	// Empty instance scopes
	out.InstanceLevelScopings = nil
	md = FormatScopesMarkdown(out)
	if !strings.Contains(md, "No instance-level scopes") {
		t.Errorf("expected empty instance message: %s", md)
	}

	// Empty runner scopes
	out.RunnerLevelScopings = nil
	out.InstanceLevelScopings = []InstanceScopeItem{{CreatedAt: "2026-01-15T10:00:00Z"}}
	md = FormatScopesMarkdown(out)
	if !strings.Contains(md, "No runner-level scopes") {
		t.Errorf("expected empty runner message: %s", md)
	}
}

// TestFormatInstanceScopeMarkdown verifies instance scope Markdown formatting.
func TestFormatInstanceScopeMarkdown(t *testing.T) {
	out := InstanceScopeOutput{
		CreatedAt: "2026-01-15T10:00:00Z",
		UpdatedAt: "2026-01-15T12:00:00Z",
	}

	md := FormatInstanceScopeMarkdown(out)
	if !strings.Contains(md, "Created At") || !strings.Contains(md, "Updated At") {
		t.Errorf("markdown missing timestamps: %s", md)
	}

	// Without timestamps
	md = FormatInstanceScopeMarkdown(InstanceScopeOutput{})
	if strings.Contains(md, "Created At") {
		t.Error("should not contain Created At when empty")
	}
}

// TestFormatRunnerScopeMarkdown verifies runner scope Markdown formatting.
func TestFormatRunnerScopeMarkdown(t *testing.T) {
	out := RunnerScopeOutput{
		RunnerID:  42,
		CreatedAt: "2026-01-15T10:00:00Z",
		UpdatedAt: "2026-01-15T12:00:00Z",
	}

	md := FormatRunnerScopeMarkdown(out)
	if !strings.Contains(md, "42") || !strings.Contains(md, "Created At") {
		t.Errorf("markdown missing data: %s", md)
	}

	// Without timestamps
	out.CreatedAt = ""
	out.UpdatedAt = ""
	md = FormatRunnerScopeMarkdown(out)
	if strings.Contains(md, "Created At") {
		t.Error("should not contain Created At when empty")
	}
}

// TestFormatScopesResult verifies FormatScopesResult returns a non-nil result.
func TestFormatScopesResult(t *testing.T) {
	result := FormatScopesResult(ScopesOutput{})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// RegisterTools — MCP roundtrip.

// TestRegisterTools_CallAllThroughMCP validates that all runner controller scope
// tools are registered and callable through a full MCP session roundtrip.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newScopesMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_runner_controller_scope_list", map[string]any{"controller_id": 1}},
		{"add_instance", "gitlab_runner_controller_scope_add_instance", map[string]any{"controller_id": 1}},
		{"remove_instance", "gitlab_runner_controller_scope_remove_instance", map[string]any{"controller_id": 1}},
		{"add_runner", "gitlab_runner_controller_scope_add_runner", map[string]any{"controller_id": 1, "runner_id": 42}},
		{"remove_runner", "gitlab_runner_controller_scope_remove_runner", map[string]any{"controller_id": 1, "runner_id": 42}},
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

// TestMCPRoundTrip_ConfirmDeclined covers the ConfirmAction early-return
// branches in remove_instance and remove_runner when user declines.
func TestMCPRoundTrip_ConfirmDeclined(t *testing.T) {
	handler := http.NewServeMux()
	client := testutil.NewTestClient(t, handler)
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

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_runner_controller_scope_remove_instance", map[string]any{"controller_id": float64(1)}},
		{"gitlab_runner_controller_scope_remove_runner", map[string]any{"controller_id": float64(1), "runner_id": float64(42)}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatal("expected non-nil result for declined confirmation")
			}
		})
	}
}

// TestMCPRoundTrip_RemoveErrors covers the error paths in remove handlers
// after ConfirmAction succeeds.
func TestMCPRoundTrip_RemoveErrors(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"server error"}`)
	})
	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "accept"}, nil
		},
	})
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_runner_controller_scope_remove_instance", map[string]any{"controller_id": float64(1)}},
		{"gitlab_runner_controller_scope_remove_runner", map[string]any{"controller_id": float64(1), "runner_id": float64(42)}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("unexpected transport error: %v", err)
			}
			if result == nil || !result.IsError {
				t.Fatalf("expected error result for %s with 500 backend", tt.name)
			}
		})
	}
}

// TestRegisterMeta_NoPanic verifies RegisterMeta does not panic.
func TestRegisterMeta_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)
}

// newScopesMCPSession creates an MCP session with runner controller scope tools.
func newScopesMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/runner_controllers/1/scopes", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, sampleScopesJSON)
	})
	handler.HandleFunc("POST /api/v4/runner_controllers/1/scopes/instance", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, sampleInstanceScopeJSON)
	})
	handler.HandleFunc("DELETE /api/v4/runner_controllers/1/scopes/instance", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler.HandleFunc("POST /api/v4/runner_controllers/1/scopes/runners/42", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, sampleRunnerScopeJSON)
	})
	handler.HandleFunc("DELETE /api/v4/runner_controllers/1/scopes/runners/42", func(w http.ResponseWriter, _ *http.Request) {
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
