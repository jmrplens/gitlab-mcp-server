// Package samplingtools provides MCP tools that leverage the sampling capability
// to analyze GitLab data through LLM-assisted summarization and review.
package samplingtools

import (
	"context"
	"errors"
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
