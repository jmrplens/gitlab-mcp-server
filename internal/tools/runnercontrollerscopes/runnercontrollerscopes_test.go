// runnercontrollerscopes_test.go contains unit tests for the runner controller scope MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package runnercontrollerscopes

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
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
	ctx := testutil.CancelledCtx(t)

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

// TestAddInstanceScope_Conflict verifies already-assigned instance scope hints.
func TestAddInstanceScope_Conflict(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusConflict, `{"message":"conflict"}`)
	}))
	_, err := AddInstanceScope(context.Background(), client, AddInstanceScopeInput{ControllerID: 1})
	if err == nil {
		t.Fatal(errExpAPIErr)
	}
	if !strings.Contains(err.Error(), "already have instance scope") {
		t.Fatalf("error = %v, want instance scope hint", err)
	}
}

// TestAddInstanceScope_ContextCancelled verifies context cancellation.
func TestAddInstanceScope_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())
	ctx := testutil.CancelledCtx(t)

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

// TestRemoveInstanceScope_NotFound verifies missing instance scope hints.
func TestRemoveInstanceScope_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))
	err := RemoveInstanceScope(context.Background(), client, RemoveInstanceScopeInput{ControllerID: 1})
	if err == nil {
		t.Fatal(errExpAPIErr)
	}
	if !strings.Contains(err.Error(), "may not have instance scope") {
		t.Fatalf("error = %v, want missing instance scope hint", err)
	}
}

// TestRemoveInstanceScope_ContextCancelled verifies context cancellation.
func TestRemoveInstanceScope_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())
	ctx := testutil.CancelledCtx(t)

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

// TestAddRunnerScope_NotFound verifies controller or runner lookup hints.
func TestAddRunnerScope_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))
	_, err := AddRunnerScope(context.Background(), client, AddRunnerScopeInput{ControllerID: 1, RunnerID: 42})
	if err == nil {
		t.Fatal(errExpAPIErr)
	}
	if !strings.Contains(err.Error(), "controller_id and runner_id") {
		t.Fatalf("error = %v, want controller/runner hint", err)
	}
}

// TestAddRunnerScope_ContextCancelled verifies context cancellation.
func TestAddRunnerScope_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())
	ctx := testutil.CancelledCtx(t)

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

// TestRemoveRunnerScope_NotFound verifies missing runner scope hints.
func TestRemoveRunnerScope_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))
	err := RemoveRunnerScope(context.Background(), client, RemoveRunnerScopeInput{ControllerID: 1, RunnerID: 42})
	if err == nil {
		t.Fatal(errExpAPIErr)
	}
	if !strings.Contains(err.Error(), "may not be scoped") {
		t.Fatalf("error = %v, want missing runner scope hint", err)
	}
}

// TestRemoveRunnerScope_ContextCancelled verifies context cancellation.
func TestRemoveRunnerScope_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())
	ctx := testutil.CancelledCtx(t)

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
