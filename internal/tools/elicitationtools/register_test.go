// register_test.go validates that the four registered MCP tool wrappers in
// [register.go] correctly translate elicitation cancellation/decline errors
// returned by the underlying handlers into a non-error CancelledResult,
// rather than propagating the error to the client.

package elicitationtools

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

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
