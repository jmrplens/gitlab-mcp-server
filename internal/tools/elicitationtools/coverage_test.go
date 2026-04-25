// coverage_test.go fills the remaining coverage gaps in the elicitationtools
// package: the `if err != nil` branch after the final ec.Confirm in each of
// the four interactive create tool handlers, and the cancelled/declined arm
// of the registered MCP tool wrappers in register.go.

package elicitationtools

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/elicitation"
	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// ---------------------------------------------------------------------------
// In-process Confirm error branch (lines 154, 234, 354, 429 in elicitationtools.go)
// ---------------------------------------------------------------------------

// stepHandlerCancelOnConfirm returns a handler that accepts the first
// `acceptCount` requests using the supplied content list, then cancels every
// subsequent request. It exercises the path where ec.Confirm returns an
// error (ErrCancelled) instead of (false, nil).
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
		// next request → handler returns "cancel"
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
		// next request → handler returns "cancel"
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
		// next request → handler returns "cancel"
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
		// next request → handler returns "cancel"
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

// ---------------------------------------------------------------------------
// MCP round-trip cancellation (register.go lines 40, 61, 82, 103)
// ---------------------------------------------------------------------------

// roundTripCancelCase describes one tool registration's cancellation flow.
type roundTripCancelCase struct {
	tool       string
	args       map[string]any
	wantInBody string // substring that must appear in the CancelledResult text
}

// runRoundTripCancelCase calls the named registered MCP tool through an
// in-memory client whose elicitation handler immediately cancels. The tool
// handler returns a wrapped ErrCancelled, and the registered closure should
// translate that into a non-error CancelledResult — the path exercised here.
func runRoundTripCancelCase(t *testing.T, tc roundTripCancelCase) {
	t.Helper()

	gitlabClient := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, gitlabClient)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	go func() { _, _ = server.Connect(ctx, st, nil) }()

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "cancel"}, nil
		},
	})
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tc.tool, Arguments: tc.args})
	if err != nil {
		t.Fatalf("CallTool %s: %v", tc.tool, err)
	}
	if res == nil {
		t.Fatalf("nil result for %s", tc.tool)
	}
	if res.IsError {
		t.Errorf("%s should return IsError=false (cancellation is not an error)", tc.tool)
	}

	body := contentText(res)
	if !strings.Contains(body, tc.wantInBody) {
		t.Errorf("%s body = %q, want substring %q", tc.tool, body, tc.wantInBody)
	}
}

// contentText concatenates the text content blocks of a tool result so tests
// can assert on the message returned by CancelledResult.
func contentText(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

// TestRegisterTools_IssueCancelRoundTrip verifies that the registered
// gitlab_interactive_issue_create tool returns a non-error CancelledResult
// when elicitation is cancelled mid-flow.
func TestRegisterTools_IssueCancelRoundTrip(t *testing.T) {
	runRoundTripCancelCase(t, roundTripCancelCase{
		tool:       "gitlab_interactive_issue_create",
		args:       map[string]any{keyProjectID: "42"},
		wantInBody: "Issue creation cancelled",
	})
}

// TestRegisterTools_MRCancelRoundTrip verifies the MR registration's
// cancellation wrapper.
func TestRegisterTools_MRCancelRoundTrip(t *testing.T) {
	runRoundTripCancelCase(t, roundTripCancelCase{
		tool:       "gitlab_interactive_mr_create",
		args:       map[string]any{keyProjectID: "42"},
		wantInBody: "Merge request creation cancelled",
	})
}

// TestRegisterTools_ReleaseCancelRoundTrip verifies the release registration's
// cancellation wrapper.
func TestRegisterTools_ReleaseCancelRoundTrip(t *testing.T) {
	runRoundTripCancelCase(t, roundTripCancelCase{
		tool:       "gitlab_interactive_release_create",
		args:       map[string]any{keyProjectID: "42"},
		wantInBody: "Release creation cancelled",
	})
}

// TestRegisterTools_ProjectCancelRoundTrip verifies the project registration's
// cancellation wrapper.
func TestRegisterTools_ProjectCancelRoundTrip(t *testing.T) {
	runRoundTripCancelCase(t, roundTripCancelCase{
		tool:       "gitlab_interactive_project_create",
		args:       map[string]any{},
		wantInBody: "Project creation cancelled",
	})
}
