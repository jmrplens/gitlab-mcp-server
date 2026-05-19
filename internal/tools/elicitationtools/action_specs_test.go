// action_specs_test.go validates that catalog-backed interactive tools translate
// elicitation cancellation into non-error CancelledResult responses.
package elicitationtools

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// roundTripCancelCase describes one tool registration's cancellation flow.
type roundTripCancelCase struct {
	tool       string
	args       map[string]any
	wantInBody string // substring that must appear in the CancelledResult text
}

// runRoundTripCancelCase calls the named catalog-backed MCP tool through an
// in-memory client whose elicitation handler immediately cancels. The tool
// route translates the wrapped ErrCancelled into a non-error CancelledResult.
func runRoundTripCancelCase(t *testing.T, tc roundTripCancelCase) {
	t.Helper()

	gitlabClient := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	byTool := elicitationSpecsByTool(t, ActionSpecs(gitlabClient))
	spec, ok := byTool[tc.tool]
	if !ok {
		t.Fatalf("missing spec for %s", tc.tool)
	}
	toolutil.RegisterSurfaceToolFromSpec(server, spec, toolutil.SurfaceToolRegisterOptions{
		Description:  spec.IndividualTool.Description,
		Icons:        toolutil.IconConfig,
		FormatResult: FormatResult,
	})

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "cancel"}, nil
		},
	})
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		session.Close()
		_ = serverSession.Wait()
	})

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

// TestCancelledOutput_SurfaceToolTextOnly verifies cancelledOutput exposes a
// text-only representation for catalog-backed surface tools.
//
// The method has no return value, so the test ensures it remains callable for
// the formatter path that converts elicitation cancellations into text content.
func TestCancelledOutput_SurfaceToolTextOnly(t *testing.T) {
	cancelledOutput{}.SurfaceToolTextOnly()
}

// TestFormatResult_DefaultUnknown verifies FormatResult ignores unsupported
// output types.
//
// Passing an anonymous struct should return nil, keeping the shared formatter
// from producing misleading content for unknown elicitation results.
func TestFormatResult_DefaultUnknown(t *testing.T) {
	if result := FormatResult(struct{}{}); result != nil {
		t.Fatalf("FormatResult() = %#v, want nil for unknown type", result)
	}
}

// TestElicitationRoute_UnmarshalError verifies catalog-backed elicitation routes
// return decode errors for malformed input parameters.
//
// The test calls the raw route handler with project_id as a string slice instead
// of a scalar. The expected error proves route decoding happens before the
// interactive flow begins.
func TestElicitationRoute_UnmarshalError(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	byTool := elicitationSpecsByTool(t, ActionSpecs(client))
	spec := byTool["gitlab_interactive_issue_create"]

	_, err := spec.Route.Handler(context.Background(), map[string]any{keyProjectID: []string{"bad"}})
	if err == nil {
		t.Fatal("expected unmarshal error")
	}
}

// TestCatalogSurface_IssueCancelRoundTrip verifies that the catalog-backed
// gitlab_interactive_issue_create tool returns a non-error CancelledResult
// when elicitation is cancelled mid-flow.
func TestCatalogSurface_IssueCancelRoundTrip(t *testing.T) {
	runRoundTripCancelCase(t, roundTripCancelCase{
		tool:       "gitlab_interactive_issue_create",
		args:       map[string]any{keyProjectID: "42"},
		wantInBody: "Issue creation cancelled",
	})
}

// TestCatalogSurface_MRCancelRoundTrip verifies the MR surface tool's
// cancellation wrapper.
func TestCatalogSurface_MRCancelRoundTrip(t *testing.T) {
	runRoundTripCancelCase(t, roundTripCancelCase{
		tool:       "gitlab_interactive_mr_create",
		args:       map[string]any{keyProjectID: "42"},
		wantInBody: "Merge request creation cancelled",
	})
}

// TestCatalogSurface_ReleaseCancelRoundTrip verifies the release surface tool's
// cancellation wrapper.
func TestCatalogSurface_ReleaseCancelRoundTrip(t *testing.T) {
	runRoundTripCancelCase(t, roundTripCancelCase{
		tool:       "gitlab_interactive_release_create",
		args:       map[string]any{keyProjectID: "42"},
		wantInBody: "Release creation cancelled",
	})
}

// TestCatalogSurface_ProjectCancelRoundTrip verifies the project surface tool's
// cancellation wrapper.
func TestCatalogSurface_ProjectCancelRoundTrip(t *testing.T) {
	runRoundTripCancelCase(t, roundTripCancelCase{
		tool:       "gitlab_interactive_project_create",
		args:       map[string]any{},
		wantInBody: "Project creation cancelled",
	})
}

func elicitationSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		toolName := spec.IndividualTool.Name
		if toolName == "" {
			t.Fatalf("spec %s missing IndividualTool.Name", spec.Name)
		}
		if _, exists := byTool[toolName]; exists {
			t.Fatalf("duplicate individual tool %q", toolName)
		}
		byTool[toolName] = spec
	}
	return byTool
}
