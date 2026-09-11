// runner_controller_scopes_test.go contains unit tests for the runner controller scope MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package runnercontrollerscopes

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
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

// scopesHints, instanceHints and runnerHints are the guidance sections the
// three scope renderings close with.
const (
	scopesHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'runner.controller_scope_add_instance' to grant this controller the instance-level scope\n" +
		"- Use action 'runner.controller_scope_add_runner' to scope this controller to one more runner\n"
	instanceHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'runner.controller_scope_list' to see every scope this controller holds\n" +
		"- Use action 'runner.controller_scope_remove_instance' to revoke the instance-level scope\n"
	runnerHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'runner.controller_scope_list' to see every scope this controller holds\n" +
		"- Use action 'runner.controller_scope_remove_runner' to remove this runner from the controller's scope\n"
)

// TestFormatScopesMarkdown_BothKinds pins the whole card of a controller that
// holds both kinds of scope: each collection as a table of its own, under a
// heading that counts it.
func TestFormatScopesMarkdown_BothKinds(t *testing.T) {
	got := FormatScopesMarkdown(ScopesOutput{
		InstanceLevelScopings: []InstanceScopeItem{
			{CreatedAt: "2026-01-15T10:00:00Z", UpdatedAt: "2026-01-15T12:00:00Z"},
		},
		RunnerLevelScopings: []RunnerScopeItem{
			{RunnerID: 42, CreatedAt: "2026-01-15T10:00:00Z", UpdatedAt: "2026-01-15T12:00:00Z"},
		},
	})

	want := "## Runner Controller Scopes\n\n" +
		"### Instance-Level Scopes (1)\n\n" +
		"| Created | Updated |\n| --- | --- |\n" +
		"| 15 Jan 2026 10:00 UTC | 15 Jan 2026 12:00 UTC |\n" +
		"\n### Runner-Level Scopes (1)\n\n" +
		"| Runner ID | Created | Updated |\n| --- | --- | --- |\n" +
		"| 42 | 15 Jan 2026 10:00 UTC | 15 Jan 2026 12:00 UTC |\n" +
		scopesHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatScopesMarkdown_EmptyKinds pins the card of a controller with no
// scope of one kind or of either: the section says so in a sentence instead of
// opening an empty table.
func TestFormatScopesMarkdown_EmptyKinds(t *testing.T) {
	noInstance := FormatScopesMarkdown(ScopesOutput{
		RunnerLevelScopings: []RunnerScopeItem{{RunnerID: 42, CreatedAt: "2026-01-15T10:00:00Z"}},
	})

	wantNoInstance := "## Runner Controller Scopes\n\n" +
		"### Instance-Level Scopes (0)\n\n" +
		"No instance-level scopes configured.\n" +
		"\n### Runner-Level Scopes (1)\n\n" +
		"| Runner ID | Created | Updated |\n| --- | --- | --- |\n" +
		"| 42 | 15 Jan 2026 10:00 UTC |  |\n" +
		scopesHints

	if noInstance != wantNoInstance {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", noInstance, wantNoInstance)
	}

	neither := FormatScopesMarkdown(ScopesOutput{})

	wantNeither := "## Runner Controller Scopes\n\n" +
		"### Instance-Level Scopes (0)\n\n" +
		"No instance-level scopes configured.\n" +
		"\n### Runner-Level Scopes (0)\n\n" +
		"No runner-level scopes configured.\n" +
		scopesHints

	if neither != wantNeither {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", neither, wantNeither)
	}
}

// TestFormatInstanceScopeMarkdown pins the whole card of an instance-level
// scope, with and without the timestamps GitLab may omit.
func TestFormatInstanceScopeMarkdown(t *testing.T) {
	got := FormatInstanceScopeMarkdown(InstanceScopeOutput{
		CreatedAt: "2026-01-15T10:00:00Z",
		UpdatedAt: "2026-01-15T12:00:00Z",
	})

	want := "## Instance-Level Scope\n\n" +
		"- **Created**: 15 Jan 2026 10:00 UTC\n" +
		"- **Updated**: 15 Jan 2026 12:00 UTC\n" +
		instanceHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}

	bare := FormatInstanceScopeMarkdown(InstanceScopeOutput{})

	if wantBare := "## Instance-Level Scope\n" + instanceHints; bare != wantBare {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", bare, wantBare)
	}
}

// TestFormatRunnerScopeMarkdown pins the whole card of a runner-level scope,
// with and without the timestamps GitLab may omit.
func TestFormatRunnerScopeMarkdown(t *testing.T) {
	got := FormatRunnerScopeMarkdown(RunnerScopeOutput{
		RunnerID:  42,
		CreatedAt: "2026-01-15T10:00:00Z",
		UpdatedAt: "2026-01-15T12:00:00Z",
	})

	want := "## Runner Scope (Runner #42)\n\n" +
		"- **Runner ID**: 42\n" +
		"- **Created**: 15 Jan 2026 10:00 UTC\n" +
		"- **Updated**: 15 Jan 2026 12:00 UTC\n" +
		runnerHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}

	bare := FormatRunnerScopeMarkdown(RunnerScopeOutput{RunnerID: 42})

	wantBare := "## Runner Scope (Runner #42)\n\n" +
		"- **Runner ID**: 42\n" +
		runnerHints

	if bare != wantBare {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", bare, wantBare)
	}
}

// TestFormatScopesResult verifies FormatScopesResult returns a non-nil result.
func TestFormatScopesResult(t *testing.T) {
	result := FormatScopesResult(ScopesOutput{})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}
