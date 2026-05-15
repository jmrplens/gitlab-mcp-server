package samplingtools

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/sampling"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// samplingRequirement is appended to every sampling tool description so users
// understand that the MCP client must support human-approved sampling.
const samplingRequirement = "Requires the MCP client to support the sampling capability (human-in-the-loop approval)."

// RegisterTools wires sampling-powered tools to the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	for _, spec := range ActionSpecs(client) {
		toolutil.RegisterSurfaceToolFromSpec(server, spec, toolutil.SurfaceToolRegisterOptions{Icons: toolutil.IconAnalytics, FormatResult: MetaMarkdownForResult})
	}
}

// samplingUnsupportedOutput is a sentinel type returned by wrapSamplingAction
// when the MCP client does not support the sampling capability.
type samplingUnsupportedOutput struct {
	ToolName string
}

// wrapSamplingAction wraps a sampling handler as an ActionFunc, converting
// sampling.ErrSamplingNotSupported into a sentinel so the meta handler returns
// an informational error result instead of a Go error.
func wrapSamplingAction[T any, R any](client *gitlabclient.Client, fn func(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input T) (R, error), toolName ...string) toolutil.ActionFunc {
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
func samplingRoute[T any, R any](client *gitlabclient.Client, fn func(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input T) (R, error), toolName ...string) toolutil.ActionRoute {
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

// registerLegacyMeta registers the pre-catalog gitlab_analyze meta-tool used by package-level parity tests.
func registerLegacyMeta(server *mcp.Server) {
	routes, err := toolutil.ActionSpecsToMapWithError(ActionSpecs(nil))
	if err != nil {
		panic(fmt.Sprintf("sampling action specs: %v", err))
	}

	toolutil.AddReadOnlyMetaTool(server, "gitlab_analyze", `LLM-assisted analysis of GitLab data via MCP sampling. Each action fetches data through GitLab APIs, then asks the connected LLM (the host's sampling capability) to summarize / analyze / classify it. Requires the client to advertise sampling capability — actions return SamplingUnsupportedResult otherwise (human-in-the-loop on the client side).
When to use: ask an LLM to interpret GitLab artifacts — MR diffs, issue threads, pipeline failures, CI configs, milestone progress, deployment history, technical-debt markers — and produce Markdown narratives, scopes, or release notes.
NOT for: raw data retrieval without LLM analysis (use gitlab_merge_request / gitlab_issue / gitlab_pipeline / gitlab_release / gitlab_repository); skipping explicit prerequisite inspection/list/compare steps requested by the user; long-form report generation outside the chat session; clients without sampling support (the action returns a `+"`SamplingUnsupportedResult`"+`).

Returns: each action returns action-specific JSON (typically identifiers + a text field plus model and truncated flags) and a Markdown summary suitable for direct display. Per-action text key:
- summary: issue_summary, mr_review
- analysis: mr_changes, pipeline_failure, ci_config, issue_scope, technical_debt, deployment_history
- review: mr_security
- report: milestone_report
- release_notes: release_notes
Alongside the resource identifiers (merge_request_iid, issue_iid, pipeline_id, milestone_iid, project_id) supplied as input.
Errors: 404 (hint: project_id, merge_request_iid, issue_iid, pipeline_id, milestone_iid must exist), 403 (hint: caller must have access to the underlying resource), `+"`SamplingUnsupportedResult`"+` when the client did not advertise sampling capability.

All actions need project_id*. Additional params per action:
- mr_changes: merge_request_iid*. Analyze MR code changes for quality, bugs, improvements.
- issue_summary: issue_iid*. Summarize discussion with key decisions and action items.
- release_notes: from*, to. Generate categorized release notes between refs. to defaults to HEAD. from_ref/to_ref aliases are accepted but from/to are canonical. If the user asks to inspect releases or compare refs first, call those tools before release_notes.
- pipeline_failure: pipeline_id*. Root cause analysis with fix suggestions.
- mr_review: merge_request_iid*. Summarize review feedback and unresolved threads.
- milestone_report: milestone_iid*. Progress report with metrics.
- ci_config: content_ref. Analyze CI/CD config for best practices and security.
- issue_scope: issue_iid*. Scope, complexity, and breakdown recommendations.
- mr_security: merge_request_iid*. OWASP Top 10, secrets, auth review.
- technical_debt: ref. Find TODO/FIXME/HACK markers.
- deployment_history: environment. Frequency, success rate, patterns.

See also: gitlab_merge_request (MR lifecycle), gitlab_issue (issue CRUD), gitlab_pipeline (raw pipelines and test reports), gitlab_release (release CRUD).`, routes, toolutil.IconAnalytics, metaMarkdownForResult)
}
