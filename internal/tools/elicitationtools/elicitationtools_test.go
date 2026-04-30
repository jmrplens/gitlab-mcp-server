// elicitationtools_test.go contains unit tests for interactive MCP tool
// handlers powered by the elicitation capability. Tests cover helper
// functions, confirmation flows, input validation, elicitation-unsupported
// paths, and full end-to-end interactive creation flows for issues, merge
// requests, releases, and projects.
package elicitationtools

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/elicitation"
	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	fmtErrWantErr             = "error = %v, want %v"
	actionAccept              = "accept"
	keyConfirmed              = "confirmed"
	msgDeleteProject          = "Delete project?"
	keyProjectID              = "project_id"
	fmtErrWantProjectIDValErr = "error = %v, want project_id validation error"
	testIssueTitle            = "Test Issue"
	testMRFeatureTitle        = "feat: new feature"
	testRelease10Name         = "Release 1.0"
	testNewProjectName        = "new-project"
	testTagV100               = "v1.0.0"
)

// CancelledResult / UnsupportedResult tests.

// TestCancelledResult verifies that CancelledResult returns a non-error tool
// result containing a cancellation message.
func TestCancelledResult(t *testing.T) {
	result := CancelledResult("Operation canceled by user.")
	if result.IsError {
		t.Error("CancelledResult.IsError = true, want false")
	}
	if len(result.Content) != 1 {
		t.Fatalf("CancelledResult content len = %d, want 1", len(result.Content))
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("CancelledResult content[0] is not TextContent")
	}
	if !strings.Contains(tc.Text, "canceled") {
		t.Error("message missing 'cancelled'")
	}
}

// TestUnsupportedResult verifies that UnsupportedResult
// returns an error tool result containing the tool name and a reference to
// the elicitation capability.
func TestUnsupportedResult(t *testing.T) {
	result := UnsupportedResult("gitlab_interactive_issue_create")
	if !result.IsError {
		t.Error("UnsupportedResult.IsError = false, want true")
	}
	if len(result.Content) != 1 {
		t.Fatalf("content len = %d, want 1", len(result.Content))
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("content[0] is not TextContent")
	}
	if !strings.Contains(tc.Text, "gitlab_interactive_issue_create") {
		t.Error("error message missing tool name")
	}
	if !strings.Contains(tc.Text, "elicitation") {
		t.Error("error message missing 'elicitation' reference")
	}
	if !strings.Contains(tc.Text, "Alternatives") {
		t.Error("expected alternative tool suggestions in unsupported message")
	}
}

// ConfirmAction tests.

// TestConfirmAction_NoElicitationSupport verifies that ConfirmAction returns
// nil (allowing the operation to proceed) when the MCP client does not support
// elicitation, maintaining backward compatibility.
func TestConfirmAction_NoElicitationSupport(t *testing.T) {
	req := &mcp.CallToolRequest{}
	r := ConfirmAction(context.Background(), req, "Delete?")
	if r != nil {
		t.Error("ConfirmAction should return nil when elicitation not supported")
	}
}

// TestConfirmAction_UserConfirms verifies that ConfirmAction returns nil when
// the user accepts the confirmation prompt.
func TestConfirmAction_UserConfirms(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitationSession(t, ctx, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{
			Action:  actionAccept,
			Content: map[string]any{keyConfirmed: true},
		}, nil
	})
	defer cleanup()

	toolReq := &mcp.CallToolRequest{Session: ss}
	r := ConfirmAction(ctx, toolReq, msgDeleteProject)
	if r != nil {
		t.Error("ConfirmAction should return nil when user confirms")
	}
}

// TestConfirmAction_UserDeclines verifies that ConfirmAction returns a
// cancellation result when the user declines the confirmation prompt.
func TestConfirmAction_UserDeclines(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitationSession(t, ctx, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "decline"}, nil
	})
	defer cleanup()

	toolReq := &mcp.CallToolRequest{Session: ss}
	r := ConfirmAction(ctx, toolReq, msgDeleteProject)
	if r == nil {
		t.Fatal("ConfirmAction should return cancellation result when user declines")
	}
	tc, ok := r.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("content[0] is not TextContent")
	}
	if !strings.Contains(tc.Text, "canceled") {
		t.Error("message missing 'cancelled'")
	}
}

// TestConfirmAction_UserCancels verifies that ConfirmAction returns a
// cancellation result when the user explicitly cancels the prompt.
func TestConfirmAction_UserCancels(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitationSession(t, ctx, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "cancel"}, nil
	})
	defer cleanup()

	toolReq := &mcp.CallToolRequest{Session: ss}
	r := ConfirmAction(ctx, toolReq, msgDeleteProject)
	if r == nil {
		t.Fatal("ConfirmAction should return cancellation result when user cancels")
	}
}

// TestConfirmAction_UserNotConfirmed verifies that ConfirmAction returns a
// cancellation result when the user accepts but sets confirmed to false.
func TestConfirmAction_UserNotConfirmed(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitationSession(t, ctx, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{
			Action:  actionAccept,
			Content: map[string]any{keyConfirmed: false},
		}, nil
	})
	defer cleanup()

	toolReq := &mcp.CallToolRequest{Session: ss}
	r := ConfirmAction(ctx, toolReq, msgDeleteProject)
	if r == nil {
		t.Fatal("ConfirmAction should return cancellation when confirmed=false")
	}
}

// Interactive tool input validation tests.

// TestIssueCreate_EmptyProjectID verifies that IssueCreate
// returns a validation error when project_id is empty.
func TestIssueCreate_EmptyProjectID(t *testing.T) {
	_, err := IssueCreate(context.Background(), &mcp.CallToolRequest{}, nil, IssueInput{})
	if err == nil || !strings.Contains(err.Error(), keyProjectID) {
		t.Errorf(fmtErrWantProjectIDValErr, err)
	}
}

// TestMRCreate_EmptyProjectID verifies that MRCreate
// returns a validation error when project_id is empty.
func TestMRCreate_EmptyProjectID(t *testing.T) {
	_, err := MRCreate(context.Background(), &mcp.CallToolRequest{}, nil, MRInput{})
	if err == nil || !strings.Contains(err.Error(), keyProjectID) {
		t.Errorf(fmtErrWantProjectIDValErr, err)
	}
}

// TestReleaseCreate_EmptyProjectID verifies that
// ReleaseCreate returns a validation error when project_id is empty.
func TestReleaseCreate_EmptyProjectID(t *testing.T) {
	_, err := ReleaseCreate(context.Background(), &mcp.CallToolRequest{}, nil, ReleaseInput{})
	if err == nil || !strings.Contains(err.Error(), keyProjectID) {
		t.Errorf(fmtErrWantProjectIDValErr, err)
	}
}

// Elicitation not supported tests.

// TestIssueCreate_ElicitationNotSupported verifies that
// IssueCreate returns [elicitation.ErrElicitationNotSupported]
// when the MCP client does not support elicitation.
func TestIssueCreate_ElicitationNotSupported(t *testing.T) {
	req := &mcp.CallToolRequest{}
	_, err := IssueCreate(context.Background(), req, nil, IssueInput{ProjectID: "42"})
	if !errors.Is(err, elicitation.ErrElicitationNotSupported) {
		t.Errorf(fmtErrWantErr, err, elicitation.ErrElicitationNotSupported)
	}
}

// TestMRCreate_ElicitationNotSupported verifies that
// MRCreate returns [elicitation.ErrElicitationNotSupported]
// when the MCP client does not support elicitation.
func TestMRCreate_ElicitationNotSupported(t *testing.T) {
	req := &mcp.CallToolRequest{}
	_, err := MRCreate(context.Background(), req, nil, MRInput{ProjectID: "42"})
	if !errors.Is(err, elicitation.ErrElicitationNotSupported) {
		t.Errorf(fmtErrWantErr, err, elicitation.ErrElicitationNotSupported)
	}
}

// TestReleaseCreate_ElicitationNotSupported verifies that
// ReleaseCreate returns [elicitation.ErrElicitationNotSupported]
// when the MCP client does not support elicitation.
func TestReleaseCreate_ElicitationNotSupported(t *testing.T) {
	req := &mcp.CallToolRequest{}
	_, err := ReleaseCreate(context.Background(), req, nil, ReleaseInput{ProjectID: "42"})
	if !errors.Is(err, elicitation.ErrElicitationNotSupported) {
		t.Errorf(fmtErrWantErr, err, elicitation.ErrElicitationNotSupported)
	}
}

// TestProjectCreate_ElicitationNotSupported verifies that
// ProjectCreate returns [elicitation.ErrElicitationNotSupported]
// when the MCP client does not support elicitation.
func TestProjectCreate_ElicitationNotSupported(t *testing.T) {
	req := &mcp.CallToolRequest{}
	_, err := ProjectCreate(context.Background(), req, nil, ProjectInput{})
	if !errors.Is(err, elicitation.ErrElicitationNotSupported) {
		t.Errorf(fmtErrWantErr, err, elicitation.ErrElicitationNotSupported)
	}
}

// Full flow tests.

// elicitationStep tracks a single interaction step in the sequential
// elicitation flow, holding the action (accept, decline, cancel) and
// optional content returned by the simulated user.
type elicitationStep struct {
	action  string
	content map[string]any
}

// stepHandler returns an elicitation handler that replays the given steps
// sequentially, returning decline for any requests beyond the defined steps.
func stepHandler(steps []elicitationStep) func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
	idx := 0
	return func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		if idx >= len(steps) {
			return &mcp.ElicitResult{Action: "decline"}, nil
		}
		step := steps[idx]
		idx++
		return &mcp.ElicitResult{
			Action:  step.action,
			Content: step.content,
		}, nil
	}
}

// TestIssueCreate_FullFlow verifies the complete interactive issue
// creation flow: collecting title, description, labels, and confidentiality
// via sequential elicitation steps, then confirming and creating the issue
// against a mocked GitLab API.
func TestIssueCreate_FullFlow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/issues", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{
			"id": 100, "iid": 1, "title": "Test Issue",
			"description": "A test issue", "state": "opened",
			"author": {"username": "alice"},
			"web_url": "https://gitlab.example.com/issues/1"
		}`)
	})
	client := testutil.NewTestClient(t, mux)

	steps := []elicitationStep{
		{action: actionAccept, content: map[string]any{"title": testIssueTitle}},
		{action: actionAccept, content: map[string]any{"description": "A test issue"}},
		{action: actionAccept, content: map[string]any{"labels": "bug, feature"}},
		{action: actionAccept, content: map[string]any{keyConfirmed: false}}, // not confidential
		{action: actionAccept, content: map[string]any{keyConfirmed: true}},  // final confirmation
	}

	ctx := context.Background()
	_, ss, cleanup := setupElicitationSession(t, ctx, stepHandler(steps))
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	out, err := IssueCreate(ctx, req, client, IssueInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("IssueCreate() error = %v", err)
	}
	if out.IID != 1 {
		t.Errorf("out.IID = %d, want 1", out.IID)
	}
	if out.Title != testIssueTitle {
		t.Errorf("out.Title = %q, want %q", out.Title, testIssueTitle)
	}
}

// TestIssueCreate_UserCancelsTitle verifies that the interactive
// issue creation returns an error when the user cancels at the title prompt.
func TestIssueCreate_UserCancelsTitle(t *testing.T) {
	ctx := context.Background()
	steps := []elicitationStep{
		{action: "cancel", content: nil}, // cancel at title prompt
	}
	_, ss, cleanup := setupElicitationSession(t, ctx, stepHandler(steps))
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	_, err := IssueCreate(ctx, req, nil, IssueInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error when user cancels title")
	}
}

// TestMRCreate_FullFlow verifies the complete interactive merge
// request creation flow: collecting branches, title, description, labels, and
// merge options via sequential elicitation steps, then confirming and creating
// the MR against a mocked GitLab API.
func TestMRCreate_FullFlow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{
			"id": 200, "iid": 5, "title": "feat: new feature",
			"state": "opened", "source_branch": "feature/x",
			"target_branch": "main",
			"web_url": "https://gitlab.example.com/mr/5"
		}`)
	})
	client := testutil.NewTestClient(t, mux)

	steps := []elicitationStep{
		{action: actionAccept, content: map[string]any{"source_branch": "feature/x"}},
		{action: actionAccept, content: map[string]any{"target_branch": "main"}},
		{action: actionAccept, content: map[string]any{"title": testMRFeatureTitle}},
		{action: actionAccept, content: map[string]any{"description": "A new feature"}},
		{action: actionAccept, content: map[string]any{"labels": ""}},        // no labels
		{action: actionAccept, content: map[string]any{keyConfirmed: true}},  // remove source
		{action: actionAccept, content: map[string]any{keyConfirmed: false}}, // no squash
		{action: actionAccept, content: map[string]any{keyConfirmed: true}},  // final confirmation
	}

	ctx := context.Background()
	_, ss, cleanup := setupElicitationSession(t, ctx, stepHandler(steps))
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	out, err := MRCreate(ctx, req, client, MRInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("MRCreate() error = %v", err)
	}
	if out.IID != 5 {
		t.Errorf("out.IID = %d, want 5", out.IID)
	}
	if out.Title != testMRFeatureTitle {
		t.Errorf("out.Title = %q, want %q", out.Title, testMRFeatureTitle)
	}
}

// TestReleaseCreate_FullFlow verifies the complete interactive
// release creation flow: collecting tag name, release name, and description
// via sequential elicitation steps, then confirming and creating the release
// against a mocked GitLab API.
func TestReleaseCreate_FullFlow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/releases", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{
			"tag_name": "v1.0.0", "name": "Release 1.0",
			"description": "First release",
			"created_at": "2026-01-15T10:00:00Z"
		}`)
	})
	client := testutil.NewTestClient(t, mux)

	steps := []elicitationStep{
		{action: actionAccept, content: map[string]any{"tag_name": testTagV100}},
		{action: actionAccept, content: map[string]any{"name": testRelease10Name}},
		{action: actionAccept, content: map[string]any{"description": "First release"}},
		{action: actionAccept, content: map[string]any{keyConfirmed: true}}, // final confirmation
	}

	ctx := context.Background()
	_, ss, cleanup := setupElicitationSession(t, ctx, stepHandler(steps))
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	out, err := ReleaseCreate(ctx, req, client, ReleaseInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("ReleaseCreate() error = %v", err)
	}
	if out.TagName != testTagV100 {
		t.Errorf("out.TagName = %q, want %q", out.TagName, testTagV100)
	}
	if out.Name != testRelease10Name {
		t.Errorf("out.Name = %q, want %q", out.Name, testRelease10Name)
	}
}

// TestProjectCreate_FullFlow verifies the complete interactive
// project creation flow: collecting name, description, visibility, README
// preference, and default branch via sequential elicitation steps, then
// confirming and creating the project against a mocked GitLab API.
func TestProjectCreate_FullFlow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{
			"id": 300, "name": "new-project",
			"description": "A new project",
			"visibility": "private",
			"path_with_namespace": "alice/new-project",
			"web_url": "https://gitlab.example.com/alice/new-project",
			"default_branch": "main"
		}`)
	})
	client := testutil.NewTestClient(t, mux)

	steps := []elicitationStep{
		{action: actionAccept, content: map[string]any{"name": testNewProjectName}},
		{action: actionAccept, content: map[string]any{"description": "A new project"}},
		{action: actionAccept, content: map[string]any{"selection": "private"}},
		{action: actionAccept, content: map[string]any{keyConfirmed: true}}, // init README
		{action: actionAccept, content: map[string]any{"default_branch": "main"}},
		{action: actionAccept, content: map[string]any{keyConfirmed: true}}, // final confirmation
	}

	ctx := context.Background()
	_, ss, cleanup := setupElicitationSession(t, ctx, stepHandler(steps))
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	out, err := ProjectCreate(ctx, req, client, ProjectInput{})
	if err != nil {
		t.Fatalf("ProjectCreate() error = %v", err)
	}
	if out.Name != testNewProjectName {
		t.Errorf("out.Name = %q, want %q", out.Name, testNewProjectName)
	}
	if out.Visibility != "private" {
		t.Errorf("out.Visibility = %q, want %q", out.Visibility, "private")
	}
}

// TestProjectCreate_UserDeclinesConfirmation verifies that
// ProjectCreate returns a cancellation error when the user
// declines the final confirmation prompt.
func TestProjectCreate_UserDeclinesConfirmation(t *testing.T) {
	steps := []elicitationStep{
		{action: actionAccept, content: map[string]any{"name": testNewProjectName}},
		{action: actionAccept, content: map[string]any{"description": ""}},
		{action: actionAccept, content: map[string]any{"selection": "private"}},
		{action: actionAccept, content: map[string]any{keyConfirmed: false}}, // no README
		{action: actionAccept, content: map[string]any{"default_branch": ""}},
		{action: actionAccept, content: map[string]any{keyConfirmed: false}}, // decline final confirmation
	}

	ctx := context.Background()
	_, ss, cleanup := setupElicitationSession(t, ctx, stepHandler(steps))
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	_, err := ProjectCreate(ctx, req, nil, ProjectInput{})
	if err == nil || !strings.Contains(err.Error(), "canceled") {
		t.Errorf("error = %v, want cancellation error", err)
	}
}

// setupElicitationSession creates a connected MCP server+client pair where
// the client supports elicitation. Returns the server, server session, and a
// cleanup function. The server session can be used to construct CallToolRequests
// with elicitation support.
func setupElicitationSession(t *testing.T, ctx context.Context, handler func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error)) (*mcp.Server, *mcp.ServerSession, func()) {
	t.Helper()

	impl := &mcp.Implementation{Name: "test", Version: "1.0.0"}
	server := mcp.NewServer(impl, nil)
	client := mcp.NewClient(impl, &mcp.ClientOptions{
		ElicitationHandler: handler,
	})

	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		ss.Close()
		t.Fatalf("client connect: %v", err)
	}

	cleanup := func() {
		cs.Close()
		ss.Close()
	}
	return server, ss, cleanup
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// parseCSVLabels — edge cases
// ---------------------------------------------------------------------------.

// TestParseCSVLabels_Empty verifies the behavior of cov parse c s v labels empty.
func TestParseCSVLabels_Empty(t *testing.T) {
	got := parseCSVLabels("")
	if got != nil {
		t.Errorf("parseCSVLabels(\"\") = %v, want nil", got)
	}
}

// TestParseCSVLabels_SingleLabel verifies the behavior of cov parse c s v labels single label.
func TestParseCSVLabels_SingleLabel(t *testing.T) {
	got := parseCSVLabels("bug")
	if len(got) != 1 || got[0] != "bug" {
		t.Errorf("parseCSVLabels(\"bug\") = %v", got)
	}
}

// TestParseCSVLabels_Multiple verifies the behavior of cov parse c s v labels multiple.
func TestParseCSVLabels_Multiple(t *testing.T) {
	got := parseCSVLabels("bug, feature , docs")
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0] != "bug" || got[1] != "feature" || got[2] != "docs" {
		t.Errorf("got %v, want [bug feature docs]", got)
	}
}

// TestParseCSVLabels_TrailingComma verifies the behavior of cov parse c s v labels trailing comma.
func TestParseCSVLabels_TrailingComma(t *testing.T) {
	got := parseCSVLabels("bug, ,, feature,")
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0] != "bug" || got[1] != "feature" {
		t.Errorf("got %v", got)
	}
}

// ---------------------------------------------------------------------------
// buildMRSummary — all options
// ---------------------------------------------------------------------------.

// TestBuildMRSummary_Full validates cov build m r summary full across multiple scenarios using table-driven subtests.
func TestBuildMRSummary_Full(t *testing.T) {
	removeSource := true
	squash := true
	s := buildMRSummary(mrSummaryParams{
		ProjectID:    "42",
		Title:        "feat: new",
		SourceBranch: "feature/x",
		TargetBranch: "main",
		Description:  "Full description text here",
		Labels:       []string{"bug", "feature"},
		RemoveSource: &removeSource,
		Squash:       &squash,
	})
	checks := []struct {
		name, want string
	}{
		{"project", "project 42"},
		{"title", "feat: new"},
		{"source", "feature/x"},
		{"target", "main"},
		{"description", "Full description text here"},
		{"labels", "bug, feature"},
		{"remove source", "Remove source branch"},
		{"squash", "Squash commits"},
	}
	for _, c := range checks {
		if !strings.Contains(s, c.want) {
			t.Errorf("buildMRSummary missing %s: want %q", c.name, c.want)
		}
	}
}

// TestBuildMRSummary_Minimal verifies the behavior of cov build m r summary minimal.
func TestBuildMRSummary_Minimal(t *testing.T) {
	s := buildMRSummary(mrSummaryParams{
		ProjectID:    "42",
		Title:        "fix: bug",
		SourceBranch: "fix/bug",
		TargetBranch: "main",
	})
	if strings.Contains(s, "Labels") {
		t.Error("minimal summary should not contain Labels")
	}
	if strings.Contains(s, "Remove source") {
		t.Error("minimal summary should not contain Remove source branch")
	}
	if strings.Contains(s, "Squash") {
		t.Error("minimal summary should not contain Squash")
	}
	if strings.Contains(s, "Description") {
		t.Error("minimal summary should not contain Description")
	}
}

// TestBuildMRSummary_RemoveSourceFalse verifies the behavior of cov build m r summary remove source false.
func TestBuildMRSummary_RemoveSourceFalse(t *testing.T) {
	removeSource := false
	squash := false
	s := buildMRSummary(mrSummaryParams{
		ProjectID:    "42",
		Title:        "fix",
		SourceBranch: "a",
		TargetBranch: "b",
		RemoveSource: &removeSource,
		Squash:       &squash,
	})
	if strings.Contains(s, "Remove source branch") {
		t.Error("remove source = false should not appear")
	}
	if strings.Contains(s, "Squash commits") {
		t.Error("squash = false should not appear")
	}
}

// ---------------------------------------------------------------------------
// RegisterTools no-panic
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of cov register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	RegisterTools(server, client)
}

// ---------------------------------------------------------------------------
// MCP round-trip — without elicitation (covers unsupported path in register.go)
// ---------------------------------------------------------------------------.

// TestMCPRound_TripNoElicitation validates cov m c p round trip no elicitation across multiple scenarios using table-driven subtests.
func TestMCPRound_TripNoElicitation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	go server.Connect(ctx, st, nil)

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}

	tests := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_interactive_issue_create", map[string]any{keyProjectID: "42"}},
		{"gitlab_interactive_mr_create", map[string]any{keyProjectID: "42"}},
		{"gitlab_interactive_release_create", map[string]any{keyProjectID: "42"}},
		{"gitlab_interactive_project_create", map[string]any{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var res *mcp.CallToolResult
			res, err = session.CallTool(ctx, &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
			if err != nil {
				t.Fatalf("CallTool %s: %v", tc.name, err)
			}
			if res == nil {
				t.Fatalf("nil result for %s", tc.name)
			}
			if !res.IsError {
				t.Errorf("%s should return IsError=true (elicitation unsupported)", tc.name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// MCP round-trip — with elicitation (covers success path for issue create)
// ---------------------------------------------------------------------------.

// TestMCPRoundTripIssueCreate_WithElicitation verifies the behavior of cov m c p round trip issue create with elicitation.
func TestMCPRoundTripIssueCreate_WithElicitation(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/issues", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{
			"id":100,"iid":1,"title":"Test Issue","description":"desc",
			"state":"opened","author":{"username":"alice"},
			"web_url":"https://gitlab.example.com/issues/1"
		}`)
	})
	gitlabClient := testutil.NewTestClient(t, mux)

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, gitlabClient)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	go server.Connect(ctx, st, nil)

	stepIdx := 0
	steps := []elicitationStep{
		{action: actionAccept, content: map[string]any{"title": testIssueTitle}},
		{action: actionAccept, content: map[string]any{"description": "desc"}},
		{action: actionAccept, content: map[string]any{"labels": ""}},
		{action: actionAccept, content: map[string]any{keyConfirmed: false}}, // not confidential
		{action: actionAccept, content: map[string]any{keyConfirmed: true}},  // final confirmation
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			if stepIdx >= len(steps) {
				return &mcp.ElicitResult{Action: "decline"}, nil
			}
			step := steps[stepIdx]
			stepIdx++
			return &mcp.ElicitResult{Action: step.action, Content: step.content}, nil
		},
	})
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_interactive_issue_create",
		Arguments: map[string]any{keyProjectID: "42"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res == nil {
		t.Fatal("nil result")
	}
	if res.IsError {
		t.Error("expected success, got IsError=true")
	}
}

// ---------------------------------------------------------------------------
// MCP round-trip — with elicitation, validation error (empty project_id)
// ---------------------------------------------------------------------------.

// TestMCPRoundTripIssueCreate_ValidationError verifies the behavior of cov m c p round trip issue create validation error.
func TestMCPRoundTripIssueCreate_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	go server.Connect(ctx, st, nil)

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_interactive_issue_create",
		Arguments: map[string]any{keyProjectID: ""},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res == nil {
		t.Fatal("nil result")
	}
	if !res.IsError {
		t.Error("expected error for empty project_id")
	}
}

// ---------------------------------------------------------------------------
// MR cancel at source branch prompt
// ---------------------------------------------------------------------------.

// TestMRCreate_UserCancelsSourceBranch verifies the behavior of cov m r create user cancels source branch.
func TestMRCreate_UserCancelsSourceBranch(t *testing.T) {
	ctx := context.Background()
	steps := []elicitationStep{
		{action: "cancel", content: nil},
	}
	_, ss, cleanup := setupElicitationSession(t, ctx, stepHandler(steps))
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	_, err := MRCreate(ctx, req, nil, MRInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error when user cancels source branch")
	}
	if !strings.Contains(err.Error(), "source branch") {
		t.Errorf("error = %q, want 'source branch' context", err)
	}
}

// ---------------------------------------------------------------------------
// Release cancel at tag name prompt
// ---------------------------------------------------------------------------.

// TestReleaseCreate_UserCancelsTagName verifies the behavior of cov release create user cancels tag name.
func TestReleaseCreate_UserCancelsTagName(t *testing.T) {
	ctx := context.Background()
	steps := []elicitationStep{
		{action: "cancel", content: nil},
	}
	_, ss, cleanup := setupElicitationSession(t, ctx, stepHandler(steps))
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	_, err := ReleaseCreate(ctx, req, nil, ReleaseInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error when user cancels tag name")
	}
	if !strings.Contains(err.Error(), "tag name") {
		t.Errorf("error = %q, want 'tag name' context", err)
	}
}

// ---------------------------------------------------------------------------
// Project cancel at name prompt
// ---------------------------------------------------------------------------.

// TestProjectCreate_UserCancelsName verifies the behavior of cov project create user cancels name.
func TestProjectCreate_UserCancelsName(t *testing.T) {
	ctx := context.Background()
	steps := []elicitationStep{
		{action: "cancel", content: nil},
	}
	_, ss, cleanup := setupElicitationSession(t, ctx, stepHandler(steps))
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	_, err := ProjectCreate(ctx, req, nil, ProjectInput{})
	if err == nil {
		t.Fatal("expected error when user cancels project name")
	}
}

// ---------------------------------------------------------------------------
// Issue user cancels at confirmation
// ---------------------------------------------------------------------------.

// TestIssueCreate_UserCancelsConfirmation verifies the behavior of cov issue create user cancels confirmation.
func TestIssueCreate_UserCancelsConfirmation(t *testing.T) {
	ctx := context.Background()
	steps := []elicitationStep{
		{action: actionAccept, content: map[string]any{"title": testIssueTitle}},
		{action: actionAccept, content: map[string]any{"description": "desc"}},
		{action: actionAccept, content: map[string]any{"labels": "bug"}},
		{action: actionAccept, content: map[string]any{keyConfirmed: false}},
		{action: actionAccept, content: map[string]any{keyConfirmed: false}}, // decline final confirm
	}
	_, ss, cleanup := setupElicitationSession(t, ctx, stepHandler(steps))
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	_, err := IssueCreate(ctx, req, nil, IssueInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if !strings.Contains(err.Error(), "canceled") {
		t.Errorf("error = %q, want cancellation", err)
	}
}

// ---------------------------------------------------------------------------
// MR user cancels at confirmation
// ---------------------------------------------------------------------------.

// TestMRCreate_UserCancelsConfirmation verifies the behavior of cov m r create user cancels confirmation.
func TestMRCreate_UserCancelsConfirmation(t *testing.T) {
	ctx := context.Background()
	steps := []elicitationStep{
		{action: actionAccept, content: map[string]any{"source_branch": "feature/x"}},
		{action: actionAccept, content: map[string]any{"target_branch": "main"}},
		{action: actionAccept, content: map[string]any{"title": testMRFeatureTitle}},
		{action: actionAccept, content: map[string]any{"description": ""}},
		{action: actionAccept, content: map[string]any{"labels": ""}},
		{action: actionAccept, content: map[string]any{keyConfirmed: true}},
		{action: actionAccept, content: map[string]any{keyConfirmed: false}},
		{action: actionAccept, content: map[string]any{keyConfirmed: false}}, // decline final
	}
	_, ss, cleanup := setupElicitationSession(t, ctx, stepHandler(steps))
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	_, err := MRCreate(ctx, req, nil, MRInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if !strings.Contains(err.Error(), "canceled") {
		t.Errorf("error = %q, want cancellation", err)
	}
}

// ---------------------------------------------------------------------------
// Release user cancels at confirmation
// ---------------------------------------------------------------------------.

// TestReleaseCreate_UserCancelsConfirmation verifies the behavior of cov release create user cancels confirmation.
func TestReleaseCreate_UserCancelsConfirmation(t *testing.T) {
	ctx := context.Background()
	steps := []elicitationStep{
		{action: actionAccept, content: map[string]any{"tag_name": testTagV100}},
		{action: actionAccept, content: map[string]any{"name": testRelease10Name}},
		{action: actionAccept, content: map[string]any{"description": "notes"}},
		{action: actionAccept, content: map[string]any{keyConfirmed: false}}, // decline final
	}
	_, ss, cleanup := setupElicitationSession(t, ctx, stepHandler(steps))
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	_, err := ReleaseCreate(ctx, req, nil, ReleaseInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if !strings.Contains(err.Error(), "canceled") {
		t.Errorf("error = %q, want cancellation", err)
	}
}

// ---------------------------------------------------------------------------
// Issue create with confidential=true in flow
// ---------------------------------------------------------------------------.

// TestIssueCreate_Confidential verifies the behavior of cov issue create confidential.
func TestIssueCreate_Confidential(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/issues", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{
			"id":101,"iid":2,"title":"Secret Issue","description":"confidential",
			"state":"opened","confidential":true,"author":{"username":"alice"},
			"web_url":"https://gitlab.example.com/issues/2"
		}`)
	})
	gitlabClient := testutil.NewTestClient(t, mux)

	ctx := context.Background()
	steps := []elicitationStep{
		{action: actionAccept, content: map[string]any{"title": "Secret Issue"}},
		{action: actionAccept, content: map[string]any{"description": "confidential"}},
		{action: actionAccept, content: map[string]any{"labels": "security"}},
		{action: actionAccept, content: map[string]any{keyConfirmed: true}}, // confidential=yes
		{action: actionAccept, content: map[string]any{keyConfirmed: true}}, // final confirm
	}
	_, ss, cleanup := setupElicitationSession(t, ctx, stepHandler(steps))
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	out, err := IssueCreate(ctx, req, gitlabClient, IssueInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("IssueCreate() error = %v", err)
	}
	if out.IID != 2 {
		t.Errorf("out.IID = %d, want 2", out.IID)
	}
}

// ConfirmAction — generic error (not Declined/Cancelled) → nil.

// TestConfirmAction_OtherError verifies that ConfirmAction returns nil when
// ec.Confirm returns a generic error (not ErrDeclined/ErrCancelled),
// maintaining backward compatibility by allowing the operation to proceed.
func TestConfirmAction_OtherError(t *testing.T) {
	ctx := context.Background()
	_, ss, cleanup := setupElicitationSession(t, ctx, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return nil, errors.New("unexpected transport failure")
	})
	defer cleanup()

	r := ConfirmAction(ctx, &mcp.CallToolRequest{Session: ss}, "Proceed?")
	if r != nil {
		t.Error("ConfirmAction should return nil on generic error (backward compat)")
	}
}

// IssueCreate — cancel at description and labels prompts.

// TestIssueCreate_CancelAtDescription verifies IssueCreate returns an error
// when the user cancels the description prompt (non-ErrDeclined error path).
func TestIssueCreate_CancelAtDescription(t *testing.T) {
	ctx := context.Background()
	steps := []elicitationStep{
		{action: actionAccept, content: map[string]any{"title": testIssueTitle}},
		{action: "cancel", content: nil},
	}
	_, ss, cleanup := setupElicitationSession(t, ctx, stepHandler(steps))
	defer cleanup()

	_, err := IssueCreate(ctx, &mcp.CallToolRequest{Session: ss}, nil, IssueInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error when user cancels description")
	}
	if !strings.Contains(err.Error(), "description") {
		t.Errorf("error = %q, want 'description' context", err)
	}
}

// TestIssueCreate_CancelAtLabels verifies IssueCreate returns an error
// when the user cancels the labels prompt after declining the optional description.
func TestIssueCreate_CancelAtLabels(t *testing.T) {
	ctx := context.Background()
	steps := []elicitationStep{
		{action: actionAccept, content: map[string]any{"title": testIssueTitle}},
		{action: "decline", content: nil}, // decline description (optional)
		{action: "cancel", content: nil},  // cancel labels
	}
	_, ss, cleanup := setupElicitationSession(t, ctx, stepHandler(steps))
	defer cleanup()

	_, err := IssueCreate(ctx, &mcp.CallToolRequest{Session: ss}, nil, IssueInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error when user cancels labels")
	}
	if !strings.Contains(err.Error(), "labels") {
		t.Errorf("error = %q, want 'labels' context", err)
	}
}

// MRCreate — cancel at intermediate prompts (table-driven).

// TestMRCreate_CancelAtVariousSteps verifies that MRCreate returns an
// appropriate error when the user cancels at each intermediate prompt:
// target branch, title, description, and labels.
func TestMRCreate_CancelAtVariousSteps(t *testing.T) {
	tests := []struct {
		name      string
		steps     []elicitationStep
		wantError string
	}{
		{
			name: "cancel at target branch",
			steps: []elicitationStep{
				{action: actionAccept, content: map[string]any{"source_branch": "f/x"}},
				{action: "cancel", content: nil},
			},
			wantError: "target branch",
		},
		{
			name: "cancel at title",
			steps: []elicitationStep{
				{action: actionAccept, content: map[string]any{"source_branch": "f/x"}},
				{action: actionAccept, content: map[string]any{"target_branch": "main"}},
				{action: "cancel", content: nil},
			},
			wantError: "title",
		},
		{
			name: "cancel at description",
			steps: []elicitationStep{
				{action: actionAccept, content: map[string]any{"source_branch": "f/x"}},
				{action: actionAccept, content: map[string]any{"target_branch": "main"}},
				{action: actionAccept, content: map[string]any{"title": "feat: x"}},
				{action: "cancel", content: nil},
			},
			wantError: "description",
		},
		{
			name: "cancel at labels",
			steps: []elicitationStep{
				{action: actionAccept, content: map[string]any{"source_branch": "f/x"}},
				{action: actionAccept, content: map[string]any{"target_branch": "main"}},
				{action: actionAccept, content: map[string]any{"title": "feat: x"}},
				{action: "decline", content: nil}, // decline description
				{action: "cancel", content: nil},  // cancel labels
			},
			wantError: "labels",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			_, ss, cleanup := setupElicitationSession(t, ctx, stepHandler(tc.steps))
			defer cleanup()

			_, err := MRCreate(ctx, &mcp.CallToolRequest{Session: ss}, nil, MRInput{ProjectID: "42"})
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.wantError) {
				t.Errorf("error = %q, want to contain %q", err, tc.wantError)
			}
		})
	}
}

// ReleaseCreate — cancel at intermediate prompts.

// TestReleaseCreate_CancelAtName verifies ReleaseCreate returns an error
// when the user cancels the release name prompt.
func TestReleaseCreate_CancelAtName(t *testing.T) {
	ctx := context.Background()
	steps := []elicitationStep{
		{action: actionAccept, content: map[string]any{"tag_name": testTagV100}},
		{action: "cancel", content: nil},
	}
	_, ss, cleanup := setupElicitationSession(t, ctx, stepHandler(steps))
	defer cleanup()

	_, err := ReleaseCreate(ctx, &mcp.CallToolRequest{Session: ss}, nil, ReleaseInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error when user cancels release name")
	}
	if !strings.Contains(err.Error(), "release name") {
		t.Errorf("error = %q, want 'release name' context", err)
	}
}

// TestReleaseCreate_CancelAtDescription verifies ReleaseCreate returns an
// error when the user cancels the description prompt.
func TestReleaseCreate_CancelAtDescription(t *testing.T) {
	ctx := context.Background()
	steps := []elicitationStep{
		{action: actionAccept, content: map[string]any{"tag_name": testTagV100}},
		{action: actionAccept, content: map[string]any{"name": testRelease10Name}},
		{action: "cancel", content: nil},
	}
	_, ss, cleanup := setupElicitationSession(t, ctx, stepHandler(steps))
	defer cleanup()

	_, err := ReleaseCreate(ctx, &mcp.CallToolRequest{Session: ss}, nil, ReleaseInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error when user cancels description")
	}
	if !strings.Contains(err.Error(), "description") {
		t.Errorf("error = %q, want 'description' context", err)
	}
}

// ProjectCreate — cancel at intermediate prompts (table-driven).

// TestProjectCreate_CancelAtVariousSteps verifies that ProjectCreate returns
// an appropriate error when the user cancels at description, visibility, or
// default branch prompts.
func TestProjectCreate_CancelAtVariousSteps(t *testing.T) {
	tests := []struct {
		name      string
		steps     []elicitationStep
		wantError string
	}{
		{
			name: "cancel at description",
			steps: []elicitationStep{
				{action: actionAccept, content: map[string]any{"name": "proj"}},
				{action: "cancel", content: nil},
			},
			wantError: "description",
		},
		{
			name: "cancel at visibility",
			steps: []elicitationStep{
				{action: actionAccept, content: map[string]any{"name": "proj"}},
				{action: "decline", content: nil}, // decline description
				{action: "cancel", content: nil},  // cancel visibility
			},
			wantError: "visibility",
		},
		{
			name: "cancel at default branch",
			steps: []elicitationStep{
				{action: actionAccept, content: map[string]any{"name": "proj"}},
				{action: "decline", content: nil},                                       // decline description
				{action: actionAccept, content: map[string]any{"selection": "private"}}, // visibility
				{action: "decline", content: nil},                                       // decline readme
				{action: "cancel", content: nil},                                        // cancel default branch
			},
			wantError: "default branch",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			_, ss, cleanup := setupElicitationSession(t, ctx, stepHandler(tc.steps))
			defer cleanup()

			_, err := ProjectCreate(ctx, &mcp.CallToolRequest{Session: ss}, nil, ProjectInput{})
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.wantError) {
				t.Errorf("error = %q, want to contain %q", err, tc.wantError)
			}
		})
	}
}

// MCP round-trip — MR create with elicitation (RegisterTools success path).

// TestMCPRoundTripMRCreate_WithElicitation verifies the full MCP round-trip
// for interactive MR creation through RegisterTools, covering the success
// path in the RegisterTools closure.
func TestMCPRoundTripMRCreate_WithElicitation(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/merge_requests", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{
			"id":200,"iid":5,"title":"feat: new",
			"state":"opened","source_branch":"feature/x","target_branch":"main",
			"web_url":"https://gitlab.example.com/mr/5"
		}`)
	})
	gitlabClient := testutil.NewTestClient(t, mux)

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, gitlabClient)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	go server.Connect(ctx, st, nil)

	stepIdx := 0
	steps := []elicitationStep{
		{action: actionAccept, content: map[string]any{"source_branch": "feature/x"}},
		{action: actionAccept, content: map[string]any{"target_branch": "main"}},
		{action: actionAccept, content: map[string]any{"title": "feat: new"}},
		{action: actionAccept, content: map[string]any{"description": ""}},
		{action: actionAccept, content: map[string]any{"labels": ""}},
		{action: actionAccept, content: map[string]any{keyConfirmed: true}},  // remove source
		{action: actionAccept, content: map[string]any{keyConfirmed: false}}, // no squash
		{action: actionAccept, content: map[string]any{keyConfirmed: true}},  // final confirm
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			if stepIdx >= len(steps) {
				return &mcp.ElicitResult{Action: "decline"}, nil
			}
			step := steps[stepIdx]
			stepIdx++
			return &mcp.ElicitResult{Action: step.action, Content: step.content}, nil
		},
	})
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_interactive_mr_create",
		Arguments: map[string]any{keyProjectID: "42"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Error("expected success, got IsError=true")
	}
}

// MCP round-trip — Release create with elicitation (RegisterTools success path).

// TestMCPRoundTripReleaseCreate_WithElicitation verifies the full MCP
// round-trip for interactive release creation through RegisterTools.
func TestMCPRoundTripReleaseCreate_WithElicitation(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/releases", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{
			"tag_name":"v1.0.0","name":"Release 1.0",
			"description":"notes","created_at":"2026-01-15T10:00:00Z"
		}`)
	})
	gitlabClient := testutil.NewTestClient(t, mux)

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, gitlabClient)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	go server.Connect(ctx, st, nil)

	stepIdx := 0
	steps := []elicitationStep{
		{action: actionAccept, content: map[string]any{"tag_name": testTagV100}},
		{action: actionAccept, content: map[string]any{"name": testRelease10Name}},
		{action: actionAccept, content: map[string]any{"description": "notes"}},
		{action: actionAccept, content: map[string]any{keyConfirmed: true}}, // final confirm
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			if stepIdx >= len(steps) {
				return &mcp.ElicitResult{Action: "decline"}, nil
			}
			step := steps[stepIdx]
			stepIdx++
			return &mcp.ElicitResult{Action: step.action, Content: step.content}, nil
		},
	})
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_interactive_release_create",
		Arguments: map[string]any{keyProjectID: "42"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Error("expected success, got IsError=true")
	}
}

// MCP round-trip — Project create with elicitation (RegisterTools success path).

// TestMCPRoundTripProjectCreate_WithElicitation verifies the full MCP
// round-trip for interactive project creation through RegisterTools.
func TestMCPRoundTripProjectCreate_WithElicitation(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{
			"id":300,"name":"new-project","description":"desc",
			"visibility":"private","path_with_namespace":"alice/new-project",
			"web_url":"https://gitlab.example.com/alice/new-project",
			"default_branch":"main"
		}`)
	})
	gitlabClient := testutil.NewTestClient(t, mux)

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, gitlabClient)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	go server.Connect(ctx, st, nil)

	stepIdx := 0
	steps := []elicitationStep{
		{action: actionAccept, content: map[string]any{"name": testNewProjectName}},
		{action: actionAccept, content: map[string]any{"description": "desc"}},
		{action: actionAccept, content: map[string]any{"selection": "private"}},
		{action: actionAccept, content: map[string]any{keyConfirmed: true}}, // init README
		{action: actionAccept, content: map[string]any{"default_branch": "main"}},
		{action: actionAccept, content: map[string]any{keyConfirmed: true}}, // final confirm
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			if stepIdx >= len(steps) {
				return &mcp.ElicitResult{Action: "decline"}, nil
			}
			step := steps[stepIdx]
			stepIdx++
			return &mcp.ElicitResult{Action: step.action, Content: step.content}, nil
		},
	})
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_interactive_project_create",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Error("expected success, got IsError=true")
	}
}

// ---------------------------------------------------------------------------
// Final ec.Confirm error branch (lines 154, 234, 354, 429 in elicitationtools.go)
// ---------------------------------------------------------------------------

// stepHandlerCancelOnConfirm returns an elicitation handler that accepts the
// first len(accepts) requests using the supplied content list and then cancels
// every subsequent request. It exercises the path where ec.Confirm returns an
// error wrapping ErrCancelled instead of (false, nil).
func stepHandlerCancelOnConfirm(accepts []map[string]any) func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
	idx := 0
	return func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		if idx >= len(accepts) {
			return &mcp.ElicitResult{Action: "cancel"}, nil
		}
		c := accepts[idx]
		idx++
		return &mcp.ElicitResult{Action: actionAccept, Content: c}, nil
	}
}

// TestIssueCreate_FinalConfirmCancel verifies that IssueCreate returns a
// cancellation error wrapping ErrCancelled when the user cancels the final
// confirm prompt (action="cancel"), exercising the err != nil branch after
// ec.Confirm rather than the !confirmed branch.
func TestIssueCreate_FinalConfirmCancel(t *testing.T) {
	ctx := context.Background()
	accepts := []map[string]any{
		{"title": testIssueTitle},
		{"description": "desc"},
		{"labels": ""},
		{keyConfirmed: false}, // not confidential
	}
	_, ss, cleanup := setupElicitationSession(t, ctx, stepHandlerCancelOnConfirm(accepts))
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	_, err := IssueCreate(ctx, req, nil, IssueInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error when user cancels final confirmation")
	}
	if !errors.Is(err, elicitation.ErrCancelled) {
		t.Errorf("error chain = %v, want ErrCancelled in chain", err)
	}
	if !strings.Contains(err.Error(), "canceled") {
		t.Errorf("error = %q, want 'canceled' wrapper", err)
	}
}

// TestMRCreate_FinalConfirmCancel verifies that MRCreate returns a
// cancellation error wrapping ErrCancelled when the user cancels the final
// confirm prompt.
func TestMRCreate_FinalConfirmCancel(t *testing.T) {
	ctx := context.Background()
	accepts := []map[string]any{
		{"source_branch": "feature/x"},
		{"target_branch": "main"},
		{"title": testMRFeatureTitle},
		{"description": "desc"},
		{"labels": ""},
		{keyConfirmed: true}, // remove source
		{keyConfirmed: true}, // squash
	}
	_, ss, cleanup := setupElicitationSession(t, ctx, stepHandlerCancelOnConfirm(accepts))
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	_, err := MRCreate(ctx, req, nil, MRInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error when user cancels final confirmation")
	}
	if !errors.Is(err, elicitation.ErrCancelled) {
		t.Errorf("error chain = %v, want ErrCancelled in chain", err)
	}
	if !strings.Contains(err.Error(), "merge request creation canceled") {
		t.Errorf("error = %q, want 'merge request creation canceled' wrapper", err)
	}
}

// TestReleaseCreate_FinalConfirmCancel verifies that ReleaseCreate returns a
// cancellation error wrapping ErrCancelled when the user cancels the final
// confirm prompt.
func TestReleaseCreate_FinalConfirmCancel(t *testing.T) {
	ctx := context.Background()
	accepts := []map[string]any{
		{"tag_name": testTagV100},
		{"name": testRelease10Name},
		{"description": "release notes"},
	}
	_, ss, cleanup := setupElicitationSession(t, ctx, stepHandlerCancelOnConfirm(accepts))
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	_, err := ReleaseCreate(ctx, req, nil, ReleaseInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error when user cancels final confirmation")
	}
	if !errors.Is(err, elicitation.ErrCancelled) {
		t.Errorf("error chain = %v, want ErrCancelled in chain", err)
	}
	if !strings.Contains(err.Error(), "release creation canceled") {
		t.Errorf("error = %q, want 'release creation canceled' wrapper", err)
	}
}

// TestProjectCreate_FinalConfirmCancel verifies that ProjectCreate returns a
// cancellation error wrapping ErrCancelled when the user cancels the final
// confirm prompt. Distinct from TestProjectCreate_UserDeclinesConfirmation,
// which exercises the !confirmed (false-nil) branch instead.
func TestProjectCreate_FinalConfirmCancel(t *testing.T) {
	ctx := context.Background()
	accepts := []map[string]any{
		{"name": testNewProjectName},
		{"description": ""},
		{"selection": "private"},
		{keyConfirmed: false}, // no README
		{"default_branch": ""},
	}
	_, ss, cleanup := setupElicitationSession(t, ctx, stepHandlerCancelOnConfirm(accepts))
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	_, err := ProjectCreate(ctx, req, nil, ProjectInput{})
	if err == nil {
		t.Fatal("expected error when user cancels final confirmation")
	}
	if !errors.Is(err, elicitation.ErrCancelled) {
		t.Errorf("error chain = %v, want ErrCancelled in chain", err)
	}
	if !strings.Contains(err.Error(), "project creation canceled") {
		t.Errorf("error = %q, want 'project creation canceled' wrapper", err)
	}
}
