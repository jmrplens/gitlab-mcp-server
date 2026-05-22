package samplingtools

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/sampling"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// samplingRequirement is appended to every sampling tool description so users
// understand that the MCP client must support human-approved sampling.
const samplingRequirement = "Requires the MCP client to support the sampling capability (human-in-the-loop approval)."

// samplingUnsupportedOutput is a sentinel type returned by wrapSamplingAction
// when the MCP client does not support the sampling capability.
type samplingUnsupportedOutput struct {
	ToolName string
}

// wrapSamplingAction wraps a sampling handler as an ActionFunc, converting
// sampling.ErrSamplingNotSupported into a sentinel so the meta handler returns
// an informational error result instead of a Go error.
func wrapSamplingAction[T, R any](client *gitlabclient.Client, fn func(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input T) (R, error), toolName ...string) toolutil.ActionFunc {
	return func(ctx context.Context, params map[string]any) (any, error) {
		input, err := toolutil.UnmarshalParams[T](params)
		if err != nil {
			return nil, err
		}
		result, err := fn(ctx, toolutil.RequestFromContext(ctx), client, input)
		if errors.Is(err, sampling.ErrSamplingNotSupported) {
			return samplingUnsupportedOutput{ToolName: samplingToolName(toolName...)}, nil
		}
		return result, err
	}
}

func samplingToolName(toolName ...string) string {
	if len(toolName) == 0 || toolName[0] == "" {
		return "gitlab_analyze"
	}
	return toolName[0]
}

// samplingRoute preserves the sampling-specific unsupported-capability handling
// while still attaching the typed input/output schemas expected by meta-route
// schema resources and audits.
func samplingRoute[T, R any](client *gitlabclient.Client, fn func(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input T) (R, error), toolName ...string) toolutil.ActionRoute {
	route := toolutil.RouteActionWithRequest(client, fn)
	route.Handler = wrapSamplingAction[T, R](client, fn, toolName...)
	return route
}

// MetaMarkdownForResult dispatches sampling output types to their Markdown formatters.
func MetaMarkdownForResult(result any) *mcp.CallToolResult {
	switch v := result.(type) {
	case samplingUnsupportedOutput:
		toolName := v.ToolName
		if toolName == "" {
			toolName = "gitlab_analyze"
		}
		return SamplingUnsupportedResult(toolName)
	case AnalyzeMRChangesOutput:
		return toolutil.ToolResultWithMarkdown(FormatAnalyzeMRChangesMarkdown(v))
	case SummarizeIssueOutput:
		return toolutil.ToolResultWithMarkdown(FormatSummarizeIssueMarkdown(v))
	case GenerateReleaseNotesOutput:
		return toolutil.ToolResultWithMarkdown(FormatGenerateReleaseNotesMarkdown(v))
	case AnalyzePipelineFailureOutput:
		return toolutil.ToolResultWithMarkdown(FormatAnalyzePipelineFailureMarkdown(v))
	case SummarizeMRReviewOutput:
		return toolutil.ToolResultWithMarkdown(FormatSummarizeMRReviewMarkdown(v))
	case GenerateMilestoneReportOutput:
		return toolutil.ToolResultWithMarkdown(FormatGenerateMilestoneReportMarkdown(v))
	case AnalyzeCIConfigOutput:
		return toolutil.ToolResultWithMarkdown(FormatAnalyzeCIConfigMarkdown(v))
	case AnalyzeIssueScopeOutput:
		return toolutil.ToolResultWithMarkdown(FormatAnalyzeIssueScopeMarkdown(v))
	case ReviewMRSecurityOutput:
		return toolutil.ToolResultWithMarkdown(FormatReviewMRSecurityMarkdown(v))
	case FindTechnicalDebtOutput:
		return toolutil.ToolResultWithMarkdown(FormatFindTechnicalDebtMarkdown(v))
	case AnalyzeDeploymentHistoryOutput:
		return toolutil.ToolResultWithMarkdown(FormatAnalyzeDeploymentHistoryMarkdown(v))
	default:
		return toolutil.ToolResultWithMarkdown(fmt.Sprintf("Unknown sampling output type: %T", result))
	}
}

func metaMarkdownForResult(result any) *mcp.CallToolResult {
	return MetaMarkdownForResult(result)
}
