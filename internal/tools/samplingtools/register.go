// Package samplingtools provides MCP tools that leverage the sampling capability
// to analyze GitLab data through LLM-assisted summarization and review.
package samplingtools

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/sampling"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const samplingRequirement = "Requires the MCP client to support the sampling capability (human-in-the-loop approval)."

// RegisterTools wires sampling-powered tools to the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_analyze_mr_changes",
		Title: toolutil.TitleFromName("gitlab_analyze_mr_changes"),
		Description: "Analyze a GitLab merge request using LLM-assisted code review via MCP sampling. " +
			"Fetches MR details and diffs, then requests LLM analysis for code quality, bugs, and improvements. " +
			samplingRequirement +
			"\n\nReturns: Markdown analysis of merge request changes including code quality, bugs, and improvement recommendations.\n\nSee also: gitlab_review_mr_security, gitlab_summarize_mr_review",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconAnalytics,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AnalyzeMRChangesInput) (*mcp.CallToolResult, AnalyzeMRChangesOutput, error) {
		start := time.Now()
		out, err := AnalyzeMRChanges(ctx, req, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_analyze_mr_changes", start, err)
		if errors.Is(err, sampling.ErrSamplingNotSupported) {
			return SamplingUnsupportedResult("gitlab_analyze_mr_changes"), AnalyzeMRChangesOutput{}, nil
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatAnalyzeMRChangesMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_summarize_issue",
		Title: toolutil.TitleFromName("gitlab_summarize_issue"),
		Description: "Summarize a GitLab issue discussion using LLM-assisted analysis via MCP sampling. " +
			"Fetches issue details and all notes, then requests LLM summary of key decisions and action items. " +
			samplingRequirement +
			"\n\nReturns: Markdown summary of the issue with key decisions and action items.\n\nSee also: gitlab_analyze_issue_scope, gitlab_list_issues",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconAnalytics,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SummarizeIssueInput) (*mcp.CallToolResult, SummarizeIssueOutput, error) {
		start := time.Now()
		out, err := SummarizeIssue(ctx, req, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_summarize_issue", start, err)
		if errors.Is(err, sampling.ErrSamplingNotSupported) {
			return SamplingUnsupportedResult("gitlab_summarize_issue"), SummarizeIssueOutput{}, nil
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatSummarizeIssueMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_generate_release_notes",
		Title: toolutil.TitleFromName("gitlab_generate_release_notes"),
		Description: "Generate polished release notes using LLM-assisted analysis via MCP sampling. " +
			"Compares two Git refs, fetches commits and merged MRs with labels, then requests LLM to produce " +
			"categorized release notes (Features, Bug Fixes, Improvements, Breaking Changes). " +
			samplingRequirement +
			"\n\nReturns: Markdown release notes categorized by Features, Bug Fixes, Improvements, and Breaking Changes.\n\nSee also: gitlab_create_release, gitlab_list_commits",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconAnalytics,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GenerateReleaseNotesInput) (*mcp.CallToolResult, GenerateReleaseNotesOutput, error) {
		start := time.Now()
		out, err := GenerateReleaseNotes(ctx, req, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_generate_release_notes", start, err)
		if errors.Is(err, sampling.ErrSamplingNotSupported) {
			return SamplingUnsupportedResult("gitlab_generate_release_notes"), GenerateReleaseNotesOutput{}, nil
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatGenerateReleaseNotesMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_analyze_pipeline_failure",
		Title: toolutil.TitleFromName("gitlab_analyze_pipeline_failure"),
		Description: "Analyze a GitLab pipeline failure using LLM-assisted root cause analysis via MCP sampling. " +
			"Fetches pipeline details, failed jobs and their traces, then requests LLM analysis for root cause, " +
			"fix suggestions, and impact assessment. " +
			samplingRequirement +
			"\n\nReturns: Markdown analysis of pipeline failure with root cause and suggested fixes.\n\nSee also: gitlab_get_pipeline, gitlab_get_job_trace",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconAnalytics,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AnalyzePipelineFailureInput) (*mcp.CallToolResult, AnalyzePipelineFailureOutput, error) {
		start := time.Now()
		out, err := AnalyzePipelineFailure(ctx, req, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_analyze_pipeline_failure", start, err)
		if errors.Is(err, sampling.ErrSamplingNotSupported) {
			return SamplingUnsupportedResult("gitlab_analyze_pipeline_failure"), AnalyzePipelineFailureOutput{}, nil
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatAnalyzePipelineFailureMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_summarize_mr_review",
		Title: toolutil.TitleFromName("gitlab_summarize_mr_review"),
		Description: "Summarize a GitLab merge request review using LLM-assisted analysis via MCP sampling. " +
			"Fetches MR details, discussions, and approval state, then requests LLM summary of reviewer feedback, " +
			"unresolved threads, and action items. " +
			samplingRequirement +
			"\n\nReturns: Markdown summary of reviewer feedback, unresolved threads, and action items.\n\nSee also: gitlab_analyze_mr_changes, gitlab_list_mr_discussions",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconAnalytics,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SummarizeMRReviewInput) (*mcp.CallToolResult, SummarizeMRReviewOutput, error) {
		start := time.Now()
		out, err := SummarizeMRReview(ctx, req, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_summarize_mr_review", start, err)
		if errors.Is(err, sampling.ErrSamplingNotSupported) {
			return SamplingUnsupportedResult("gitlab_summarize_mr_review"), SummarizeMRReviewOutput{}, nil
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatSummarizeMRReviewMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_generate_milestone_report",
		Title: toolutil.TitleFromName("gitlab_generate_milestone_report"),
		Description: "Generate a comprehensive milestone progress report using LLM-assisted analysis via MCP sampling. " +
			"Fetches milestone details, linked issues and merge requests, then requests LLM to produce " +
			"a data-driven progress report with metrics, risks, and recommendations. " +
			samplingRequirement +
			"\n\nReturns: Markdown progress report with metrics, risks, and recommendations.\n\nSee also: gitlab_get_milestone, gitlab_list_milestone_issues",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconAnalytics,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GenerateMilestoneReportInput) (*mcp.CallToolResult, GenerateMilestoneReportOutput, error) {
		start := time.Now()
		out, err := GenerateMilestoneReport(ctx, req, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_generate_milestone_report", start, err)
		if errors.Is(err, sampling.ErrSamplingNotSupported) {
			return SamplingUnsupportedResult("gitlab_generate_milestone_report"), GenerateMilestoneReportOutput{}, nil
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatGenerateMilestoneReportMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_analyze_ci_configuration",
		Title: toolutil.TitleFromName("gitlab_analyze_ci_configuration"),
		Description: "Analyze a GitLab project's CI/CD configuration using LLM-assisted analysis via MCP sampling. " +
			"Lints the CI config, fetches merged YAML and includes, then requests LLM analysis for " +
			"best practices, performance, security, and maintainability. " +
			samplingRequirement +
			"\n\nReturns: Markdown analysis of CI/CD configuration covering best practices, performance, security, and maintainability.\n\nSee also: gitlab_ci_lint_project, gitlab_list_pipelines",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconAnalytics,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AnalyzeCIConfigInput) (*mcp.CallToolResult, AnalyzeCIConfigOutput, error) {
		start := time.Now()
		out, err := AnalyzeCIConfig(ctx, req, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_analyze_ci_configuration", start, err)
		if errors.Is(err, sampling.ErrSamplingNotSupported) {
			return SamplingUnsupportedResult("gitlab_analyze_ci_configuration"), AnalyzeCIConfigOutput{}, nil
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatAnalyzeCIConfigMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_analyze_issue_scope",
		Title: toolutil.TitleFromName("gitlab_analyze_issue_scope"),
		Description: "Analyze a GitLab issue's scope and effort using LLM-assisted analysis via MCP sampling. " +
			"Fetches issue details, time stats, participants, related MRs, and discussion notes, then " +
			"requests LLM to assess scope, complexity, risks, and whether the issue should be broken down. " +
			samplingRequirement +
			"\n\nReturns: Markdown analysis of issue scope, complexity, risks, and breakdown recommendations.\n\nSee also: gitlab_summarize_issue, gitlab_get_issue",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconAnalytics,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AnalyzeIssueScopeInput) (*mcp.CallToolResult, AnalyzeIssueScopeOutput, error) {
		start := time.Now()
		out, err := AnalyzeIssueScope(ctx, req, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_analyze_issue_scope", start, err)
		if errors.Is(err, sampling.ErrSamplingNotSupported) {
			return SamplingUnsupportedResult("gitlab_analyze_issue_scope"), AnalyzeIssueScopeOutput{}, nil
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatAnalyzeIssueScopeMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_review_mr_security",
		Title: toolutil.TitleFromName("gitlab_review_mr_security"),
		Description: "Perform a security-focused review of a GitLab merge request using LLM-assisted analysis via MCP sampling. " +
			"Fetches MR details and code diffs, then requests LLM to identify injection vulnerabilities, " +
			"auth issues, exposed secrets, and OWASP Top 10 findings. " +
			samplingRequirement +
			"\n\nReturns: Markdown security review with vulnerability findings and OWASP Top 10 assessment.\n\nSee also: gitlab_analyze_mr_changes, gitlab_get_merge_request",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconAnalytics,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ReviewMRSecurityInput) (*mcp.CallToolResult, ReviewMRSecurityOutput, error) {
		start := time.Now()
		out, err := ReviewMRSecurity(ctx, req, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_review_mr_security", start, err)
		if errors.Is(err, sampling.ErrSamplingNotSupported) {
			return SamplingUnsupportedResult("gitlab_review_mr_security"), ReviewMRSecurityOutput{}, nil
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatReviewMRSecurityMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_find_technical_debt",
		Title: toolutil.TitleFromName("gitlab_find_technical_debt"),
		Description: "Find and analyze technical debt in a GitLab project using LLM-assisted analysis via MCP sampling. " +
			"Searches for TODO, FIXME, HACK, XXX, and DEPRECATED markers in source code, then requests LLM " +
			"to categorize, prioritize, and recommend a remediation strategy. " +
			samplingRequirement +
			"\n\nReturns: Markdown report of technical debt categorized by priority with remediation strategy.\n\nSee also: gitlab_search_code, gitlab_get_project",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconAnalytics,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input FindTechnicalDebtInput) (*mcp.CallToolResult, FindTechnicalDebtOutput, error) {
		start := time.Now()
		out, err := FindTechnicalDebt(ctx, req, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_find_technical_debt", start, err)
		if errors.Is(err, sampling.ErrSamplingNotSupported) {
			return SamplingUnsupportedResult("gitlab_find_technical_debt"), FindTechnicalDebtOutput{}, nil
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatFindTechnicalDebtMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_analyze_deployment_history",
		Title: toolutil.TitleFromName("gitlab_analyze_deployment_history"),
		Description: "Analyze deployment history and patterns for a GitLab project using LLM-assisted analysis via MCP sampling. " +
			"Fetches recent deployments, then requests LLM to assess deployment frequency, success rate, " +
			"rollback patterns, and suggest improvements. " +
			samplingRequirement +
			"\n\nReturns: Markdown analysis of deployment patterns with frequency, success rate, and improvement suggestions.\n\nSee also: gitlab_list_deployments, gitlab_list_environments",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconAnalytics,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AnalyzeDeploymentHistoryInput) (*mcp.CallToolResult, AnalyzeDeploymentHistoryOutput, error) {
		start := time.Now()
		out, err := AnalyzeDeploymentHistory(ctx, req, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_analyze_deployment_history", start, err)
		if errors.Is(err, sampling.ErrSamplingNotSupported) {
			return SamplingUnsupportedResult("gitlab_analyze_deployment_history"), AnalyzeDeploymentHistoryOutput{}, nil
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatAnalyzeDeploymentHistoryMarkdown(out)), out, err)
	})
}

// samplingUnsupportedOutput is a sentinel type returned by wrapSamplingAction
// when the MCP client does not support the sampling capability.
type samplingUnsupportedOutput struct{}

// wrapSamplingAction wraps a sampling handler as an ActionFunc, converting
// sampling.ErrSamplingNotSupported into a sentinel so the meta handler returns
// an informational error result instead of a Go error.
func wrapSamplingAction[T any, R any](client *gitlabclient.Client, fn func(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input T) (R, error)) toolutil.ActionFunc {
	return func(ctx context.Context, params map[string]any) (any, error) {
		input, err := toolutil.UnmarshalParams[T](params)
		if err != nil {
			return nil, err
		}
		result, err := fn(ctx, toolutil.RequestFromContext(ctx), client, input)
		if errors.Is(err, sampling.ErrSamplingNotSupported) {
			return samplingUnsupportedOutput{}, nil
		}
		return result, err
	}
}

// metaMarkdownForResult dispatches sampling output types to their Markdown formatters.
func metaMarkdownForResult(result any) *mcp.CallToolResult {
	switch v := result.(type) {
	case samplingUnsupportedOutput:
		return SamplingUnsupportedResult("gitlab_analyze")
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

// RegisterMeta registers a single gitlab_analyze meta-tool that consolidates
// all 11 sampling analysis tools under one action-dispatched interface.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := toolutil.ActionMap{
		"mr_changes":         toolutil.Route(wrapSamplingAction[AnalyzeMRChangesInput, AnalyzeMRChangesOutput](client, AnalyzeMRChanges)),
		"issue_summary":      toolutil.Route(wrapSamplingAction[SummarizeIssueInput, SummarizeIssueOutput](client, SummarizeIssue)),
		"release_notes":      toolutil.Route(wrapSamplingAction[GenerateReleaseNotesInput, GenerateReleaseNotesOutput](client, GenerateReleaseNotes)),
		"pipeline_failure":   toolutil.Route(wrapSamplingAction[AnalyzePipelineFailureInput, AnalyzePipelineFailureOutput](client, AnalyzePipelineFailure)),
		"mr_review":          toolutil.Route(wrapSamplingAction[SummarizeMRReviewInput, SummarizeMRReviewOutput](client, SummarizeMRReview)),
		"milestone_report":   toolutil.Route(wrapSamplingAction[GenerateMilestoneReportInput, GenerateMilestoneReportOutput](client, GenerateMilestoneReport)),
		"ci_config":          toolutil.Route(wrapSamplingAction[AnalyzeCIConfigInput, AnalyzeCIConfigOutput](client, AnalyzeCIConfig)),
		"issue_scope":        toolutil.Route(wrapSamplingAction[AnalyzeIssueScopeInput, AnalyzeIssueScopeOutput](client, AnalyzeIssueScope)),
		"mr_security":        toolutil.Route(wrapSamplingAction[ReviewMRSecurityInput, ReviewMRSecurityOutput](client, ReviewMRSecurity)),
		"technical_debt":     toolutil.Route(wrapSamplingAction[FindTechnicalDebtInput, FindTechnicalDebtOutput](client, FindTechnicalDebt)),
		"deployment_history": toolutil.Route(wrapSamplingAction[AnalyzeDeploymentHistoryInput, AnalyzeDeploymentHistoryOutput](client, AnalyzeDeploymentHistory)),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_analyze",
		Title: toolutil.TitleFromName("gitlab_analyze"),
		Description: `LLM-assisted analysis of GitLab data via MCP sampling. Each action fetches data through GitLab APIs, then asks the connected LLM (the host's sampling capability) to summarize / analyze / classify it. Requires the client to advertise sampling capability — actions return SamplingUnsupportedResult otherwise (human-in-the-loop on the client side).
Valid actions: ` + toolutil.ValidActionsString(routes) + `

When to use: ask an LLM to interpret GitLab artifacts — MR diffs, issue threads, pipeline failures, CI configs, milestone progress, deployment history, technical-debt markers — and produce Markdown narratives, scopes, or release notes.
NOT for: raw data retrieval without LLM analysis (use gitlab_merge_request / gitlab_issue / gitlab_pipeline / gitlab_release / gitlab_repository); long-form report generation outside the chat session; clients without sampling support (the action returns a ` + "`SamplingUnsupportedResult`" + `).

Returns: each action returns action-specific JSON (typically identifiers + analysis/review/release_notes text plus model and truncated flags) and a Markdown summary suitable for direct display. Common keys: {analysis|review|release_notes, model, truncated} alongside the resource identifiers (mr_iid, issue_iid, pipeline_id, milestone_iid, project_id) supplied as input.
Errors: 404 (hint: project_id, mr_iid, issue_iid, pipeline_id, milestone_iid must exist), 403 (hint: caller must have access to the underlying resource), ` + "`SamplingUnsupportedResult`" + ` when the client did not advertise sampling capability.

All actions need project_id*. Additional params per action:
- mr_changes: mr_iid*. Analyze MR code changes for quality, bugs, improvements.
- issue_summary: issue_iid*. Summarize discussion with key decisions and action items.
- release_notes: from_ref*, to_ref*. Generate categorized release notes between refs.
- pipeline_failure: pipeline_id*. Root cause analysis with fix suggestions.
- mr_review: mr_iid*. Summarize review feedback and unresolved threads.
- milestone_report: milestone_iid*. Progress report with metrics.
- ci_config: content_ref. Analyze CI/CD config for best practices and security.
- issue_scope: issue_iid*. Scope, complexity, and breakdown recommendations.
- mr_security: mr_iid*. OWASP Top 10, secrets, auth review.
- technical_debt: ref. Find TODO/FIXME/HACK markers.
- deployment_history: environment. Frequency, success rate, patterns.

See also: gitlab_merge_request (MR lifecycle), gitlab_issue (issue CRUD), gitlab_pipeline (raw pipelines and test reports), gitlab_release (release CRUD).`,
		Annotations: toolutil.ReadOnlyMetaAnnotationsWithTitle("gitlab_analyze"),
		Icons:       toolutil.IconAnalytics,
		InputSchema: toolutil.MetaToolSchema(routes),
	}, toolutil.MakeMetaHandler("gitlab_analyze", routes, metaMarkdownForResult))
}
