// routes_test.go contains integration tests for the sampling meta-tool
// closures in routes.go. Tests verify tool dispatch and error paths
// via an in-memory MCP session with a mock GitLab API.
package samplingtools

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/sampling"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestWrapSamplingAction_Success verifies that wrapSamplingAction invokes the wrapped handler and returns its typed output when no error occurs.
func TestWrapSamplingAction_Success(t *testing.T) {
	type testInput struct {
		Value string `json:"value"`
	}
	type testOutput struct {
		Result string
	}

	fn := func(_ context.Context, _ *mcp.CallToolRequest, _ *gitlabclient.Client, input testInput) (testOutput, error) {
		return testOutput{Result: input.Value}, nil
	}

	action := wrapSamplingAction[testInput, testOutput](nil, fn)

	ctx := toolutil.ContextWithRequest(context.Background(), &mcp.CallToolRequest{})
	result, err := action(ctx, map[string]any{"value": "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out, ok := result.(testOutput)
	if !ok {
		t.Fatalf("expected testOutput, got %T", result)
	}
	if out.Result != "hello" {
		t.Errorf("got %q, want %q", out.Result, "hello")
	}
}

// TestWrapSamplingAction_SamplingUnsupported verifies that wrapSamplingAction converts ErrSamplingNotSupported into a samplingUnsupportedOutput instead of propagating the error.
func TestWrapSamplingAction_SamplingUnsupported(t *testing.T) {
	type testInput struct{}
	type testOutput struct{}

	fn := func(_ context.Context, _ *mcp.CallToolRequest, _ *gitlabclient.Client, _ testInput) (testOutput, error) {
		return testOutput{}, sampling.ErrSamplingNotSupported
	}

	action := wrapSamplingAction[testInput, testOutput](nil, fn)

	ctx := toolutil.ContextWithRequest(context.Background(), &mcp.CallToolRequest{})
	result, err := action(ctx, map[string]any{})
	if err != nil {
		t.Fatalf("expected nil error for sampling unsupported, got: %v", err)
	}
	if _, ok := result.(samplingUnsupportedOutput); !ok {
		t.Fatalf("expected samplingUnsupportedOutput, got %T", result)
	}
}

// TestWrapSamplingAction_HandlerError verifies that wrapSamplingAction propagates arbitrary handler errors to the caller.
func TestWrapSamplingAction_HandlerError(t *testing.T) {
	type testInput struct{}
	type testOutput struct{}

	fn := func(_ context.Context, _ *mcp.CallToolRequest, _ *gitlabclient.Client, _ testInput) (testOutput, error) {
		return testOutput{}, errors.New("api error")
	}

	action := wrapSamplingAction[testInput, testOutput](nil, fn)

	ctx := toolutil.ContextWithRequest(context.Background(), &mcp.CallToolRequest{})
	_, err := action(ctx, map[string]any{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "api error") {
		t.Errorf("error = %q, want 'api error' substring", err.Error())
	}
}

// TestWrapSamplingAction_InvalidParams verifies that wrapSamplingAction does not panic when the caller supplies arguments that fail unmarshaling.
func TestWrapSamplingAction_InvalidParams(t *testing.T) {
	type testInput struct {
		Required int `json:"required"`
	}
	type testOutput struct{}

	fn := func(_ context.Context, _ *mcp.CallToolRequest, _ *gitlabclient.Client, _ testInput) (testOutput, error) {
		return testOutput{}, nil
	}

	action := wrapSamplingAction[testInput, testOutput](nil, fn)

	ctx := toolutil.ContextWithRequest(context.Background(), &mcp.CallToolRequest{})
	// Pass invalid type that will fail unmarshal
	_, err := action(ctx, map[string]any{"required": "not-a-number"})
	// numeric string coercion may handle this; either way the function should not panic
	if err != nil {
		return // unmarshal error is acceptable
	}
}

// TestSamplingRoute_AttachesInputAndOutputSchemas verifies that sampling routes
// retain reflected input and output schemas for meta-tool registration.
func TestSamplingRoute_AttachesInputAndOutputSchemas(t *testing.T) {
	t.Parallel()

	type testInput struct {
		Value string `json:"value"`
	}
	type testOutput struct {
		Result string `json:"result"`
	}
	fn := func(_ context.Context, _ *mcp.CallToolRequest, _ *gitlabclient.Client, input testInput) (testOutput, error) {
		return testOutput{Result: input.Value}, nil
	}

	route := samplingRoute[testInput, testOutput](nil, fn)
	if route.InputSchema == nil {
		t.Fatal("InputSchema is nil")
	}
	if route.OutputSchema == nil {
		t.Fatal("OutputSchema is nil")
	}
	if route.Destructive {
		t.Fatal("samplingRoute marked route destructive")
	}
}

// TestRegisterMeta_AnalyzeRoutesDeclareOutputSchemas verifies that every
// registered gitlab_analyze route declares an output schema.
func TestRegisterMeta_AnalyzeRoutesDeclareOutputSchemas(t *testing.T) {
	routes := analyzeActionSpecRoutes(t)
	if len(routes) == 0 {
		t.Fatal("gitlab_analyze routes were not registered")
	}
	for action, route := range routes {
		if route.OutputSchema == nil {
			t.Fatalf("route %q OutputSchema is nil", action)
		}
	}
}

// TestRegisterMeta_UsesActionSpecs verifies that gitlab_analyze meta routes
// are projected from canonical ActionSpec definitions while preserving schemas.
func TestRegisterMeta_UsesActionSpecs(t *testing.T) {
	got := analyzeActionSpecRoutes(t)
	want, err := toolutil.ActionSpecsToMapWithError(ActionSpecs(nil))
	if err != nil {
		t.Fatalf("ActionSpecsToMapWithError() error = %v", err)
	}

	if len(got) != len(want) {
		t.Fatalf("registered analyze route count = %d, want %d", len(got), len(want))
	}
	for actionName, wantRoute := range want {
		t.Run(actionName, func(t *testing.T) {
			gotRoute, ok := got[actionName]
			if !ok {
				t.Fatalf("registered meta routes missing %q", actionName)
			}
			if gotRoute.Destructive != wantRoute.Destructive {
				t.Fatalf("destructive = %t, want %t", gotRoute.Destructive, wantRoute.Destructive)
			}
			if !reflect.DeepEqual(gotRoute.InputSchema, wantRoute.InputSchema) {
				t.Fatal("input schema differs from ActionSpec projection")
			}
			if !reflect.DeepEqual(gotRoute.OutputSchema, wantRoute.OutputSchema) {
				t.Fatal("output schema differs from ActionSpec projection")
			}
		})
	}
}

// TestRegisterMeta_SamplingUnsupportedOmitsStructuredContent verifies that a
// sampling capability failure returns an error result without structured output.
func TestRegisterMeta_SamplingUnsupportedOmitsStructuredContent(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	registerAnalyzeMetaForTest(t, server)
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "gitlab_analyze",
		Arguments: map[string]any{
			"action": "mr_changes",
			"params": map[string]any{
				"project_id":        "group/project",
				"merge_request_iid": 1,
			},
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError result when sampling is unsupported")
	}
	if result.StructuredContent != nil {
		t.Fatalf("StructuredContent = %#v, want nil for IsError result", result.StructuredContent)
	}
}

func analyzeActionSpecRoutes(t *testing.T) toolutil.ActionMap {
	t.Helper()
	routes, err := toolutil.ActionSpecsToMapWithError(ActionSpecs(nil))
	if err != nil {
		t.Fatalf("ActionSpecsToMapWithError() error = %v", err)
	}
	return routes
}

// TestMetaMarkdownForResult_SamplingUnsupported verifies that metaMarkdownForResult renders a user-facing message when the sampling capability is unavailable.
func TestMetaMarkdownForResult_SamplingUnsupported(t *testing.T) {
	result := metaMarkdownForResult(samplingUnsupportedOutput{})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "does not support sampling") {
		t.Errorf("result text = %q, want 'does not support sampling' substring", text)
	}
}

// TestMetaMarkdownForResult_AllOutputTypes uses table-driven subtests to verify that metaMarkdownForResult produces non-empty content for every sampling output type.
func TestMetaMarkdownForResult_AllOutputTypes(t *testing.T) {
	tests := []struct {
		name   string
		output any
	}{
		{"AnalyzeMRChanges", AnalyzeMRChangesOutput{}},
		{"SummarizeIssue", SummarizeIssueOutput{}},
		{"GenerateReleaseNotes", GenerateReleaseNotesOutput{}},
		{"AnalyzePipelineFailure", AnalyzePipelineFailureOutput{}},
		{"SummarizeMRReview", SummarizeMRReviewOutput{}},
		{"GenerateMilestoneReport", GenerateMilestoneReportOutput{}},
		{"AnalyzeCIConfig", AnalyzeCIConfigOutput{}},
		{"AnalyzeIssueScope", AnalyzeIssueScopeOutput{}},
		{"ReviewMRSecurity", ReviewMRSecurityOutput{}},
		{"FindTechnicalDebt", FindTechnicalDebtOutput{}},
		{"AnalyzeDeploymentHistory", AnalyzeDeploymentHistoryOutput{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := metaMarkdownForResult(tc.output)
			if result == nil {
				t.Fatal("expected non-nil result")
			}
			if len(result.Content) == 0 {
				t.Fatal("expected non-empty content")
			}
		})
	}
}

// TestMetaMarkdownForResult_DefaultCase verifies that metaMarkdownForResult returns a fallback message for unknown output types.
func TestMetaMarkdownForResult_DefaultCase(t *testing.T) {
	result := metaMarkdownForResult("unexpected-type")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "Unknown sampling output type") {
		t.Errorf("result text = %q, want 'Unknown sampling output type' substring", text)
	}
}

// TestRegisterMeta_RegistersTool verifies that RegisterMeta registers the gitlab_analyze meta-tool and it is discoverable via ListTools over an in-memory MCP session.
func TestRegisterMeta_RegistersTool(t *testing.T) {
	impl := &mcp.Implementation{Name: "test", Version: "1.0.0"}
	server := mcp.NewServer(impl, nil)

	registerAnalyzeMetaForTest(t, server)

	// Connect a client to verify tool registration
	client := mcp.NewClient(impl, nil)
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer ss.Close()
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer cs.Close()

	result, err := cs.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools error: %v", err)
	}
	found := false
	for _, tool := range result.Tools {
		if tool.Name == "gitlab_analyze" {
			found = true
			break
		}
	}
	if !found {
		t.Error("gitlab_analyze tool not registered")
	}
}

func registerAnalyzeMetaForTest(t *testing.T, server *mcp.Server) {
	t.Helper()
	toolutil.AddReadOnlyMetaTool(server, "gitlab_analyze", "Ask the host model to analyze GitLab resources.", analyzeActionSpecRoutes(t), toolutil.IconAnalytics, metaMarkdownForResult)
}
