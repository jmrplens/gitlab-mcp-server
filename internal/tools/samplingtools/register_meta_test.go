package samplingtools

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/sampling"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

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

func TestWrapSamplingAction_HandlerError(t *testing.T) {
	type testInput struct{}
	type testOutput struct{}

	fn := func(_ context.Context, _ *mcp.CallToolRequest, _ *gitlabclient.Client, _ testInput) (testOutput, error) {
		return testOutput{}, fmt.Errorf("api error")
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

func TestRegisterMeta_RegistersTool(t *testing.T) {
	impl := &mcp.Implementation{Name: "test", Version: "1.0.0"}
	server := mcp.NewServer(impl, nil)

	RegisterMeta(server, nil)

	// Connect a client to verify tool registration
	client := mcp.NewClient(impl, nil)
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
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
